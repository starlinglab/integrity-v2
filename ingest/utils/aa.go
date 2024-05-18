package utils

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"os"

	"github.com/fxamacker/cbor/v2"
)

var client = &http.Client{}

var (
	ErrNeedsKey = errors.New("needs encryption key")
	ErrNotFound = errors.New("requested item not found")
)

type AttributeOptions struct {
	EncKey         []byte
	LeaveEncrypted bool
	Format         string
}

var aaUrl = os.Getenv("AA_URL")
var jwt = os.Getenv("AA_JWT")

// GetAttributeRaw returns the raw bytes for the attribute from AA.
// If an encryption key was needed (to decrypt value for sig verify) but not provided
// a ErrNeedsKey is returned. ErrNotFound is returned if the CID-attribute pair doesn't
// exist in the database.
func GetAttributeRaw(cid, attr string, opts AttributeOptions) ([]byte, error) {
	url, err := urlpkg.Parse(fmt.Sprintf("%s/c/%s/%s", aaUrl, cid, attr))
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

func GetAttribute(cid, attr string) (map[interface{}](interface{}), error) {
	bytes, err := GetAttributeRaw(cid, attr, AttributeOptions{})
	if err != nil {
		return nil, err
	}
	v := make(map[interface{}](interface{}))
	err = cbor.Unmarshal(bytes, &v)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func GetAllAttributes(cid string) (map[interface{}](interface{}), error) {
	url, err := urlpkg.Parse(fmt.Sprintf("%s/c/%s", aaUrl, cid))
	if err != nil {
		return nil, err
	}
	resp, err := client.Get(url.String())
	if err != nil {
		return nil, err
	}
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var value map[interface{}](interface{})
	err = cbor.Unmarshal(bytes, &value)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func PostNewAttribute(cid string, attributes []KeyValuePair) error {
	url, err := urlpkg.Parse(fmt.Sprintf("%s/c/%s", aaUrl, cid))
	if err != nil {
		return err
	}

	var encodedPayload []map[string]interface{}
	for _, a := range attributes {
		encodedPayload = append(encodedPayload, map[string]interface{}{"key": a.key, "value": a.value})
	}
	b, err := cbor.Marshal(encodedPayload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/cbor")
	req.Header.Add("Authorization", "Bearer "+jwt)

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
