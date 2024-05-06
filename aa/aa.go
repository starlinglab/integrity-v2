// Package aa provides Go functions to access the Authenticated Attributes API.
package aa

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"

	"github.com/starlinglab/integrity-v2/config"
)

var client = &http.Client{}

var ErrNeedsKey = errors.New("needs encryption key")

type AttributeOptions struct {
	EncKey         []byte
	LeaveEncrypted bool
	Format         string
}

// GetAttributeRaw returns the raw bytes for the attribute from AA.
// If an encryption key was needed (to decrypt value for sig verify) but not provided
// a ErrNeedsKey is returned.
func GetAttributeRaw(cid, attr string, opts AttributeOptions) ([]byte, error) {
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
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
