package c2pa

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// Network-gated: set TRUFO_TEST_API_KEY (a c2pa-sign-test key) to run.
func TestSignWithTrufo(t *testing.T) {
	apiKey := os.Getenv("TRUFO_TEST_API_KEY")
	if apiKey == "" {
		t.Skip("set TRUFO_TEST_API_KEY to run")
	}

	// A 1x1 PNG makes Trufo 500, so use a non-trivial image.
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	assertions := []any{
		[]any{"cawg_identity", map[string]any{"cawg_identity_id": "test"}},
	}
	signed, err := signWithTrufo(trufoDefaultBaseURL, apiKey, true, buf.Bytes(), nil, assertions)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(signed, []byte("c2pa.claim")) {
		t.Fatal("signed output missing C2PA manifest")
	}
}
