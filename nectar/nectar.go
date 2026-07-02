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
	"slices"
	"strings"
	"time"
)

// client bounds every /pfps call with a timeout so a hung or slow Nectar cannot
// stall an image ingest indefinitely: computeImagePFP runs on the webhook
// request path and its only other cancellation source is the inbound request
// context, which has no deadline of its own.
var client = &http.Client{Timeout: 60 * time.Second}

// supportedImageTypes are the media types the Nectar /pfps endpoint accepts.
var supportedImageTypes = []string{"image/jpeg", "image/png", "image/webp", "image/gif"}

// SupportsMediaType reports whether the Nectar /pfps endpoint can fingerprint
// the given media type, as returned by util.GuessMediaType / http.DetectContentType.
func SupportsMediaType(mediaType string) bool {
	return slices.Contains(supportedImageTypes, mediaType)
}

// ComputePFP uploads the image at imagePath to the Nectar API at url and returns
// its perceptual fingerprint (a DASL "p..." string). The bearer token is sent
// only when non-empty. It returns an error if the image cannot be read, the
// request fails, or the response does not contain a valid PFP.
//
// This package is config-free by design: callers own where url and token come
// from (in production, config.GetConfig().Nectar), so it stays a pure transport
// that tests can drive with any endpoint.
func ComputePFP(ctx context.Context, url, token, imagePath string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("no nectar url provided")
	}

	f, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("opening image for pfp: %w", err)
	}
	defer f.Close()

	return computePFPFromReader(ctx, f, filepath.Base(imagePath), url, token)
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("nectar /pfps returned status code %d and body: %s",
			resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading nectar /pfps response: %w", err)
	}

	// Upload responses look like {"blobs":[{"pfp":"p..."}]}.
	var out struct {
		Blobs []struct {
			PFP string `json:"pfp"`
		} `json:"blobs"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
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
