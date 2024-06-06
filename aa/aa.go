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
	Format         string
}

// Attestation as stored in the database in DAG-CBOR.
// See https://github.com/starlinglab/authenticated-attributes/blob/main/docs/database.md#schema
//
// This does not encode into the same CBOR it was decoded from, but that's okay
// as encoding this struct should be not required anywhere.
type AttEntry struct {
	Signature struct {
		PubKey [32]byte
		Sig    [64]byte
		Msg    CborCID
	}
	Timestamp struct {
		OTS struct {
			Proof    []byte
			Upgraded bool
			Msg      CborCID
		}
	}
	Attestation struct {
		CID       CborCID
		Value     any
		Encrypted bool
		Timestamp time.Time
	}
	Version string
}

// Attributes for uploading.
// See https://github.com/starlinglab/authenticated-attributes/blob/main/docs/http.md#post-ccid
type PostKV struct {
	Key    string   `cbor:"key"`
	Value  any      `cbor:"value"`
	Type   string   `cbor:"type,omitempty"`
	EncKey [32]byte `cbor:"encKey,omitempty"`
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

// GetAttestationRaw returns the raw bytes for the attribute from AA.
//
// If an encryption key was needed (to decrypt value for sig verify) but not provided
// a ErrNeedsKey is returned. ErrNotFound is returned if the CID-attribute pair doesn't
// exist in the database.
func GetAttestationRaw(cid, attr string, opts GetAttOpts) ([]byte, error) {
	url, err := urlpkg.Parse(fmt.Sprintf("%s/c/%s/%s", config.GetConfig().AA.Url, cid, attr))
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
	// Ignore format so CBOR is guaranteed
	opts.Format = ""

	data, err := GetAttestationRaw(cid, attr, opts)
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
	url, err := urlpkg.Parse(fmt.Sprintf("%s/c/%s", config.GetConfig().AA.Url, cid))
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
	url, err := urlpkg.Parse(fmt.Sprintf("%s/c/%s", config.GetConfig().AA.Url, cid))
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
	req.Header.Add("Authorization", "Bearer "+config.GetConfig().AA.Jwt)

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
	url, err := urlpkg.Parse(config.GetConfig().AA.Url + "/cids")
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
// See https://github.com/starlinglab/authenticated-attributes/blob/main/docs/http.md#post-ccidattr
func AppendAttestation(cid, attr string, val any) error {
	url, err := urlpkg.Parse(fmt.Sprintf("%s/c/%s/%s?append=1", config.GetConfig().AA.Url, cid, attr))
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
	req.Header.Add("Authorization", "Bearer "+config.GetConfig().AA.Jwt)

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
// https://github.com/starlinglab/authenticated-attributes/blob/main/docs/http.md#post-relcid
func AddRelationship(cid, relType, relationType, relCid string) error {
	url, err := urlpkg.Parse(fmt.Sprintf("%s/rel/%s", config.GetConfig().AA.Url, cid))
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
	req.Header.Add("Authorization", "Bearer "+config.GetConfig().AA.Jwt)

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
