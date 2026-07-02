// Package nectar is a client for the Nectar API's perceptual-fingerprint
// (PFP) endpoint. It uploads an image to POST {nectar.url}/pfps and returns the
// DASL PFP string (see https://dasl.ing/pfp.html) computed by the service.
//
// PFPs are not computed locally; this package is a thin HTTP transport around
// Nectar. Media-type gating (only sending supported images) is the caller's
// responsibility.
package nectar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/starlinglab/integrity-v2/config"
)

var client = &http.Client{}

// ComputePFP uploads the image at imagePath to the configured Nectar API and
// returns its perceptual fingerprint (a DASL "p..." string). It returns an
// error if the image cannot be read, the request fails, or the response does
// not contain a valid PFP.
func ComputePFP(ctx context.Context, imagePath string) (string, error) {
	conf := config.GetConfig()
	if conf.Nectar.Url == "" {
		return "", fmt.Errorf("no nectar url set in the config")
	}

	f, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("opening image for pfp: %w", err)
	}
	defer f.Close()

	return computePFPFromReader(ctx, f, filepath.Base(imagePath), conf.Nectar.Url, conf.Nectar.Token)
}

// computePFPFromReader performs the multipart upload of r (named filename) to
// the Nectar /pfps endpoint at url and parses the resulting PFP. The bearer
// token is sent only when non-empty. It is separated from ComputePFP so it can
// be exercised against an httptest.Server without touching disk or global
// config.
func computePFPFromReader(ctx context.Context, r io.Reader, filename, url, token string) (string, error) {
	var buf bytes.Buffer
	mp := multipart.NewWriter(&buf)
	part, err := mp.CreateFormFile("image", filename)
	if err != nil {
		return "", fmt.Errorf("creating multipart image field: %w", err)
	}
	if _, err := io.Copy(part, r); err != nil {
		return "", fmt.Errorf("writing image to multipart body: %w", err)
	}
	if err := mp.Close(); err != nil {
		return "", fmt.Errorf("closing multipart body: %w", err)
	}

	endpoint := strings.TrimRight(url, "/") + "/pfps"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, &buf)
	if err != nil {
		return "", fmt.Errorf("building nectar request: %w", err)
	}
	req.Header.Set("Content-Type", mp.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error with nectar /pfps call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("nectar /pfps returned status code %d and body: %s",
			resp.StatusCode, body)
	}

	// Upload responses look like {"blobs":[{"pfp":"p..."}]}.
	var out struct {
		Blobs []struct {
			PFP string `json:"pfp"`
		} `json:"blobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decoding nectar /pfps response: %w", err)
	}

	if len(out.Blobs) == 0 {
		return "", fmt.Errorf("nectar /pfps response contained no blobs")
	}
	pfp := out.Blobs[0].PFP
	if !strings.HasPrefix(pfp, "p") {
		return "", fmt.Errorf("nectar returned unexpected pfp format: %q", pfp)
	}
	return pfp, nil
}
