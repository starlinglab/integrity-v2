// Package aa provides Go functions to access the Authenticated Attributes API.
package aa

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/config"
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

// GetAttributeRaw returns the raw bytes for the attribute from AA.
// If an encryption key was needed (to decrypt value for sig verify) but not provided
// a ErrNeedsKey is returned. ErrNotFound is returned if the CID-attribute pair doesn't
// exist in the database.
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
	if resp.StatusCode == 404 {
		return nil, ErrNotFound
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// GetCIDFromPath returns the CID for the given relative file path.
// ErrNotFound is returned if no CID is known for that file path.
func GetCIDFromPath(path string) (string, error) {
	resp, err := client.Get(
		fmt.Sprintf(
			"%s/path?p=%s",
			config.GetConfig().AA.Url,
			urlpkg.QueryEscape(filepath.Join(config.GetConfig().Dirs.Files, path)),
		),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", ErrNotFound
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
