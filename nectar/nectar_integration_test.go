package nectar

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
)

// nectarLiveOrSkip returns the live Nectar endpoint URL and access token from
// the environment, skipping the test unless BOTH are set. Nectar is
// access-gated, so a real /pfps call needs the token as well as the URL. This
// mirrors the Cardano integration tests' env-gated skip pattern
// (register/cardano_integration_test.go), so plain `go test ./...` stays offline.
func nectarLiveOrSkip(t *testing.T) (url, token string) {
	t.Helper()
	url = os.Getenv("NECTAR_URL")
	token = os.Getenv("NECTAR_TOKEN")
	if url == "" || token == "" {
		t.Skip("set NECTAR_URL and NECTAR_TOKEN to run the live Nectar integration test")
	}
	return url, token
}

// genPatternedPNG returns the bytes of a small non-uniform PNG. A patterned
// (rather than solid-color) image gives the perceptual-fingerprint service
// something real to hash.
func genPatternedPNG(t *testing.T) []byte {
	t.Helper()
	const size = 64
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			// An XOR texture: non-flat content for the service to fingerprint.
			v := uint8(x ^ y)
			img.Set(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding test png: %v", err)
	}
	return buf.Bytes()
}

// TestNectarLivePFP uploads a synthetic image to the real Nectar /pfps endpoint
// and asserts a well-formed DASL PFP comes back. It is gated on NECTAR_URL +
// NECTAR_TOKEN and skips otherwise. Run it with:
//
//	NECTAR_URL=https://<host> NECTAR_TOKEN=<token> go test -v -run Nectar ./nectar/
//
// If the live service rejects a synthetic image, replace genPatternedPNG with a
// real image fixture under nectar/testdata/.
func TestNectarLivePFP(t *testing.T) {
	url, token := nectarLiveOrSkip(t)

	pfp, err := computePFPFromReader(context.Background(),
		bytes.NewReader(genPatternedPNG(t)), "integration-test.png", url, token)
	if err != nil {
		t.Fatalf("live nectar /pfps call failed: %v", err)
	}
	if !strings.HasPrefix(pfp, "p") {
		t.Errorf("pfp = %q, want a DASL \"p...\" string", pfp)
	}
	t.Logf("live nectar pfp: %s", pfp)
}
