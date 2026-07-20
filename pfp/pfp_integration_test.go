package pfp

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
)

// nectarLiveOrSkip returns the live Nectar endpoint URL and access token from the environment,
// skipping the test unless both are set. Mirrors nectar/nectar_integration_test.go's
// nectarLiveOrSkip, so plain `go test ./...` stays offline.
func nectarLiveOrSkip(t *testing.T) (url, token string) {
	t.Helper()
	url = os.Getenv("NECTAR_URL")
	token = os.Getenv("NECTAR_TOKEN")
	if url == "" || token == "" {
		t.Skip("set NECTAR_URL and NECTAR_TOKEN to run the live Nectar integration test")
	}
	return url, token
}

// genPatternedPNG returns the bytes of a small non-uniform PNG, matching
// nectar/nectar_integration_test.go's fixture: a patterned image gives the live service
// something real to fingerprint, unlike a flat single-color one.
func genPatternedPNG(t *testing.T) []byte {
	t.Helper()
	const size = 64
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
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

// TestLivePFPBackfill runs computeAndSetPFP against the real Nectar /pfps endpoint, exercising
// the full CLI code path (file lookup by CID, media-type gating, the Nectar call itself) rather
// than nectar.computePFPFromReader in isolation as nectar/nectar_integration_test.go does.
//
// AA is stubbed with Mock: true so this test depends on nothing but a live Nectar, and force is
// set so the (already unit-tested) existing-value guard is bypassed rather than needing a real
// AA to check against. Run it with:
//
//	NECTAR_URL=https://<host> NECTAR_TOKEN=<token> go test -v -run LivePFPBackfill ./pfp/
func TestLivePFPBackfill(t *testing.T) {
	url, token := nectarLiveOrSkip(t)

	dir := t.TempDir()
	const cid = "integration-test-cid"
	writeCidFile(t, dir, cid, genPatternedPNG(t))

	conf := &config.Config{}
	conf.Nectar.Url = url
	conf.Nectar.Token = token
	conf.Dirs.Files = dir

	pfp, err := computeAndSetPFP(context.Background(), conf, &aa.AuthAttrInstance{Mock: true}, cid, true)
	if err != nil {
		t.Fatalf("live pfp backfill failed: %v", err)
	}
	if !strings.HasPrefix(pfp, "p") {
		t.Errorf("pfp = %q, want a DASL \"p...\" string", pfp)
	}
	t.Logf("live pfp: %s", pfp)
}
