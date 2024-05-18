package utils

import (
	"bytes"
	"errors"
	"fmt"
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
