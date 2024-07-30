// Package aa provides Go functions to access the Authenticated Attributes API.
package aa

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"reflect"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/starlinglab/integrity-v2/config"
)

var client = &http.Client{}

var (
	ErrNeedsKey = errors.New("needs encryption key")
	ErrNotFound = errors.New("requested item not found")
)

type GetAttOpts struct {
	EncKey         []byte
	LeaveEncrypted bool
	Format         string // "cbor", "vc", or "" (cbor)
}

// Attestation as stored in the database in DAG-CBOR.
// See https://github.com/starlinglab/authenticated-attributes/blob/main/docs/database.md#schema
//
// This may not encode into the same CBOR it was decoded from, but that's okay
// as that should not be required anywhere.
type AttEntry struct {
	Signature struct {
		PubKey []byte  `json:"pubKey"`
		Sig    []byte  `json:"sig"`
		Msg    CborCID `json:"msg"`
	} `json:"signature"`
	Timestamp struct {
		OTS struct {
			Proof    []byte  `json:"proof"`
			Upgraded bool    `json:"upgraded"`
			Msg      CborCID `json:"msg"`
		} `json:"ots"`
	} `json:"timestamp"`
	Attestation struct {
		CID       CborCID   `json:"CID"`
		Value     any       `json:"value"`
		Attribute string    `json:"attribute"`
		Encrypted bool      `json:"encrypted"`
		Timestamp time.Time `json:"timestamp"`
	} `json:"attestation"`
	Version string `json:"version"`
}

// Attributes for uploading.
// See https://github.com/starlinglab/authenticated-attributes/blob/main/docs/http.md#post-v1ccid
type PostKV struct {
	Key    string `cbor:"key"`
	Value  any    `cbor:"value"`
	Type   string `cbor:"type,omitempty"`
	EncKey []byte `cbor:"encKey,omitempty"` // 32 bytes
}

var (
	dagCborDecMode cbor.DecMode
	dagCborEncMode cbor.EncMode
)

func init() {
	cborTags := cbor.NewTagSet()
	err := cborTags.Add(
		cbor.TagOptions{EncTag: cbor.EncTagRequired, DecTag: cbor.DecTagRequired},
		reflect.TypeOf(CborCID{}),
		42, // CIDs are type 42 in CBOR according to the DAG-CBOR spec
	)
	if err != nil {
		panic(err)
	}

	// Permissive decoder
	dagCborDecMode, err = cbor.DecOptions{
		// Easier to re-encode to JSON later
		DefaultMapType: reflect.TypeOf(map[string]any{}),
	}.DecModeWithSharedTags(cborTags)
	if err != nil {
		panic(err)
	}
	dagCborEncMode, err = cbor.EncOptions{
		// Following DAG-CBOR spec as much as possible
		// https://ipld.io/specs/codecs/dag-cbor/spec/
		Sort:          cbor.SortCanonical,
		ShortestFloat: cbor.ShortestFloatNone,
		Time:          cbor.TimeRFC3339, // Matches AA schema
		TimeTag:       cbor.EncTagNone,
		IndefLength:   cbor.IndefLengthForbidden,
		TagsMd:        cbor.TagsAllowed,      // Can't be forbidden since CID tag is needed
		OmitEmpty:     cbor.OmitEmptyGoValue, // Seems more intuitive
	}.EncModeWithSharedTags(cborTags)
	if err != nil {
		panic(err)
	}
}

type AuthAttrInstance struct {
	Url  string
	Jwt  string
	Mock bool // No network requests go through if true
}

var defaultInstance *AuthAttrInstance = nil

func GetAAInstanceFromConfig() *AuthAttrInstance {
	if defaultInstance != nil {
		return defaultInstance
	}
	defaultInstance = &AuthAttrInstance{
		Url: config.GetConfig().AA.Url,
		Jwt: config.GetConfig().AA.Jwt,
	}
	return defaultInstance
}

// GetAttestationRaw returns the raw bytes for the attribute from AA.
//
// If an encryption key was needed (to decrypt value for sig verify) but not provided
// a ErrNeedsKey is returned. ErrNotFound is returned if the CID-attribute pair doesn't
// exist in the database.
func GetAttestationRaw(cid, attr string, opts GetAttOpts) ([]byte, error) {
	return GetAAInstanceFromConfig().GetAttestationRaw(cid, attr, opts)
}

// GetAttestationRaw returns the raw bytes for the attribute from AA.
//
// If an encryption key was needed (to decrypt value for sig verify) but not provided
// a ErrNeedsKey is returned. ErrNotFound is returned if the CID-attribute pair doesn't
// exist in the database.
func (a *AuthAttrInstance) GetAttestationRaw(cid, attr string, opts GetAttOpts) ([]byte, error) {
	if a.Mock {
		return nil, nil
	}

	url, err := urlpkg.Parse(fmt.Sprintf("%s/v1/c/%s/%s",
		a.Url, urlpkg.PathEscape(cid), urlpkg.PathEscape(attr)))
	if err != nil {
		return nil, err
	}
	q := url.Query()
	if opts.EncKey != nil {
		q.Add("key", base64.URLEncoding.EncodeToString(opts.EncKey))
	}
	if opts.LeaveEncrypted {
		q.Add("decrypt", "0")
	}
	if opts.Format != "" {
		q.Add("format", opts.Format)
	}
	url.RawQuery = q.Encode()

	resp, err := client.Get(url.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 400 {
		return nil, ErrNeedsKey
	}
	if resp.StatusCode == 404 {
		return nil, ErrNotFound
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// GetAttestation returns the attestation for the provided attribute from AA.
//
// If an encryption key was needed (to decrypt value for sig verify) but not provided
// a ErrNeedsKey is returned. ErrNotFound is returned if the CID-attribute pair doesn't
// exist in the database.
//
// The Format fields of `opts` is ignored.
func GetAttestation(cid, attr string, opts GetAttOpts) (*AttEntry, error) {
	return GetAAInstanceFromConfig().GetAttestation(cid, attr, opts)
}

// GetAttestation returns the attestation for the provided attribute from AA.
//
// If an encryption key was needed (to decrypt value for sig verify) but not provided
// a ErrNeedsKey is returned. ErrNotFound is returned if the CID-attribute pair doesn't
// exist in the database.
//
// The Format fields of `opts` is ignored.
func (a *AuthAttrInstance) GetAttestation(cid, attr string, opts GetAttOpts) (*AttEntry, error) {
	if a.Mock {
		return nil, nil
	}

	// Ignore format so CBOR is guaranteed
	opts.Format = ""

	data, err := a.GetAttestationRaw(cid, attr, opts)
	if err != nil {
		return nil, err
	}

	var v AttEntry
	if err := dagCborDecMode.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// GetAttestations returns all attestations for the provided CID from AA.
func GetAttestations(cid string) (map[string]*AttEntry, error) {
	return GetAAInstanceFromConfig().GetAttestations(cid)
}

// GetAttestations returns all attestations for the provided CID from AA.
func (a *AuthAttrInstance) GetAttestations(cid string) (map[string]*AttEntry, error) {
	if a.Mock {
		return nil, nil
	}

	url, err := urlpkg.Parse(fmt.Sprintf("%s/v1/c/%s", a.Url, urlpkg.PathEscape(cid)))
	if err != nil {
		return nil, err
	}

	resp, err := client.Get(url.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, ErrNotFound
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var v map[string]*AttEntry
	if err := dagCborDecMode.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func SetAttestations(cid string, index bool, kvs []PostKV) error {
	return GetAAInstanceFromConfig().SetAttestations(cid, index, kvs)
}

func (a *AuthAttrInstance) SetAttestations(cid string, index bool, kvs []PostKV) error {
	if a.Mock {
		return nil
	}

	url, err := urlpkg.Parse(fmt.Sprintf("%s/v1/c/%s", a.Url, urlpkg.PathEscape(cid)))
	if err != nil {
		return err
	}
	if index {
		q := url.Query()
		q.Add("index", "1")
		url.RawQuery = q.Encode()
	}

	b, err := dagCborEncMode.Marshal(kvs)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url.String(), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/cbor")
	req.Header.Add("Authorization", "Bearer "+a.Jwt)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}
	return nil
}

// GetCIDs returns a slice of all the CIDs stored in the database, as strings.
func GetCIDs() ([]string, error) {
	return GetAAInstanceFromConfig().GetCIDs()
}

// GetCIDs returns a slice of all the CIDs stored in the database, as strings.
func (a *AuthAttrInstance) GetCIDs() ([]string, error) {
	if a.Mock {
		return nil, nil
	}

	url, err := urlpkg.Parse(a.Url + "/v1/cids")
	if err != nil {
		return nil, err
	}

	resp, err := client.Get(url.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var v []string
	if err := dagCborDecMode.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return v, nil
}

type singleSet[T bool | []byte] struct {
	Value  any `cbor:"value"`
	EncKey T   `cbor:"encKey"`
}

// AppendAttestation appends to an array stored at attr.
//
// See https://github.com/starlinglab/authenticated-attributes/blob/main/docs/http.md#post-v1ccidattr
func AppendAttestation(cid, attr string, val any) error {
	return GetAAInstanceFromConfig().AppendAttestation(cid, attr, val)
}

// AppendAttestation appends to an array stored at attr.
//
// See https://github.com/starlinglab/authenticated-attributes/blob/main/docs/http.md#post-v1ccidattr
func (a *AuthAttrInstance) AppendAttestation(cid, attr string, val any) error {
	if a.Mock {
		return nil
	}

	url, err := urlpkg.Parse(fmt.Sprintf("%s/v1/c/%s/%s?append=1",
		a.Url, urlpkg.PathEscape(cid), urlpkg.PathEscape(attr)))
	if err != nil {
		return err
	}

	b, err := dagCborEncMode.Marshal(singleSet[bool]{val, false})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url.String(), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/cbor")
	req.Header.Add("Authorization", "Bearer "+a.Jwt)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}
	return nil
}

type relBody struct {
	Type         string  `cbor:"type"`
	RelationType string  `cbor:"relation_type"`
	Cid          CborCID `cbor:"cid"`
}

// AddRelationship adds a relationship to the database.
//
// relType must be either "children" or "parents".
//
// relationType is the adjective to use, like "related".
//
// See AA docs for details:
// https://github.com/starlinglab/authenticated-attributes/blob/main/docs/http.md#post-v1relcid
func AddRelationship(cid, relType, relationType, relCid string) error {
	return GetAAInstanceFromConfig().AddRelationship(cid, relType, relationType, relCid)
}

// AddRelationship adds a relationship to the database.
//
// relType must be either "children" or "parents".
//
// relationType is the adjective to use, like "related".
//
// See AA docs for details:
// https://github.com/starlinglab/authenticated-attributes/blob/main/docs/http.md#post-v1relcid
func (a *AuthAttrInstance) AddRelationship(cid, relType, relationType, relCid string) error {
	if a.Mock {
		return nil
	}

	url, err := urlpkg.Parse(fmt.Sprintf("%s/v1/rel/%s", a.Url, urlpkg.PathEscape(cid)))
	if err != nil {
		return err
	}

	relCidCbor, err := NewCborCID(relCid)
	if err != nil {
		return fmt.Errorf("failed to parse relCid (%s): %v", relCid, err)
	}

	b, err := dagCborEncMode.Marshal(relBody{Type: relType, RelationType: relationType, Cid: relCidCbor})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url.String(), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/cbor")
	req.Header.Add("Authorization", "Bearer "+a.Jwt)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}
	return nil
}

// IndexMatchQuery queries the AA index for any CIDs with the provided attribute-value pair.
// See the API docs for more information:
// https://github.com/starlinglab/authenticated-attributes/blob/main/docs/http.md#get-v1i
func IndexMatchQuery(attr, val, valType string) ([]string, error) {
	return GetAAInstanceFromConfig().IndexMatchQuery(attr, val, valType)
}

// IndexMatchQuery queries the AA index for any CIDs with the provided attribute-value pair.
// See the API docs for more information:
// https://github.com/starlinglab/authenticated-attributes/blob/main/docs/http.md#get-v1i
func (a *AuthAttrInstance) IndexMatchQuery(attr, val, valType string) ([]string, error) {
	if a.Mock {
		return nil, nil
	}

	url, err := urlpkg.Parse(a.Url + "/v1/i")
	if err != nil {
		return nil, err
	}

	q := url.Query()
	q.Add("query", "match")
	q.Add("key", attr)
	q.Add("val", val)
	q.Add("type", valType)
	url.RawQuery = q.Encode()

	resp, err := client.Get(url.String())
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var cids []string
	err = dagCborDecMode.Unmarshal(data, &cids)
	if err != nil {
		return nil, err
	}
	return cids, nil
}

// IndexListQuery queries the AA index for any values that have been indexed for the
// given attribute.
func IndexListQuery(attr string) ([]string, error) {
	return GetAAInstanceFromConfig().IndexListQuery(attr)
}

// IndexListQuery queries the AA index for any values that have been indexed for the
// given attribute.
func (a *AuthAttrInstance) IndexListQuery(attr string) ([]string, error) {
	if a.Mock {
		return nil, nil
	}

	url, err := urlpkg.Parse(a.Url + "/v1/i")
	if err != nil {
		return nil, err
	}

	q := url.Query()
	q.Add("query", "list")
	q.Add("key", attr)
	url.RawQuery = q.Encode()

	resp, err := client.Get(url.String())
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var vals []string
	err = dagCborDecMode.Unmarshal(data, &vals)
	if err != nil {
		return nil, err
	}
	return vals, nil
}
