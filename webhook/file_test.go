package webhook

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/starlinglab/integrity-v2/config"
)

// pngBytes returns a minimal valid PNG. The content is irrelevant to the gating
// logic — util.GuessMediaType only needs the PNG signature to sniff image/png,
// and the mock Nectar server ignores the upload body — so a 1x1 image suffices.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("encoding png: %v", err)
	}
	return buf.Bytes()
}

// writePNG writes the minimal PNG to a temp file and returns its path, for
// callers that need a path rather than bytes.
func writePNG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(path, pngBytes(t), 0600); err != nil {
		t.Fatalf("writing png temp file: %v", err)
	}
	return path
}

// writeText writes a plaintext temp file (sniffs as text/plain, not an image).
func writeText(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("this is not an image, just some text\n"), 0600); err != nil {
		t.Fatalf("writing text temp file: %v", err)
	}
	return path
}

// pfpServer returns an httptest.Server that answers Nectar /pfps with the given
// status. On 200 it returns a well-formed PFP; otherwise an error body.
func pfpServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("nectar boom"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"blobs":[{"pfp":"pintegrationfingerprint"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// nectarConf builds a fresh config pointing Nectar at url (empty = disabled).
// Config is passed explicitly into the ingest path, so each case gets its own
// state with no global cache or reset.
func nectarConf(url string) *config.Config {
	conf := &config.Config{}
	conf.Nectar.Url = url
	return conf
}

// TestComputeImagePFP exercises every gating branch of computeImagePFP with a
// per-case config and an in-process httptest server — no network, no global.
func TestComputeImagePFP(t *testing.T) {
	ctx := context.Background()

	t.Run("disabled skips", func(t *testing.T) {
		pfp, ok, err := computeImagePFP(ctx, nectarConf(""), writePNG(t))
		if err != nil || ok || pfp != "" {
			t.Fatalf("disabled: got (%q, %v, %v), want (\"\", false, nil)", pfp, ok, err)
		}
	})

	t.Run("non-image skips", func(t *testing.T) {
		// A server that would fail if hit, proving non-images never call Nectar.
		srv := pfpServer(t, http.StatusInternalServerError)
		pfp, ok, err := computeImagePFP(ctx, nectarConf(srv.URL), writeText(t))
		if err != nil || ok || pfp != "" {
			t.Fatalf("non-image: got (%q, %v, %v), want (\"\", false, nil)", pfp, ok, err)
		}
	})

	t.Run("image yields pfp", func(t *testing.T) {
		srv := pfpServer(t, http.StatusOK)
		pfp, ok, err := computeImagePFP(ctx, nectarConf(srv.URL), writePNG(t))
		if err != nil {
			t.Fatalf("image: unexpected error: %v", err)
		}
		if !ok || pfp != "pintegrationfingerprint" {
			t.Fatalf("image: got (%q, %v), want (\"pintegrationfingerprint\", true)", pfp, ok)
		}
	})

	t.Run("image failure is strict", func(t *testing.T) {
		srv := pfpServer(t, http.StatusInternalServerError)
		pfp, ok, err := computeImagePFP(ctx, nectarConf(srv.URL), writePNG(t))
		if err == nil {
			t.Fatalf("image failure: expected strict error, got (%q, %v, nil)", pfp, ok)
		}
	})
}

// TestGetFileAttributesAndWriteToDest_PFP proves the PFP attribute wiring
// end-to-end through the ingest function: a configured Nectar sets fileAttributes["pfp"]
// on success, and a Nectar failure aborts the whole ingest (strict).
func TestGetFileAttributesAndWriteToDest_PFP(t *testing.T) {
	ctx := context.Background()
	png := pngBytes(t)

	cases := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"pfp attribute added on success", http.StatusOK, false},
		{"nectar failure aborts ingest", http.StatusInternalServerError, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := pfpServer(t, tc.status)
			dest, err := os.CreateTemp(t.TempDir(), "dest_")
			if err != nil {
				t.Fatal(err)
			}
			defer dest.Close()

			_, attrs, err := getFileAttributesAndWriteToDest(ctx, nectarConf(srv.URL), bytes.NewReader(png), dest)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected ingest to fail when Nectar errors on an image, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if attrs["pfp"] != "pintegrationfingerprint" {
				t.Fatalf("pfp attribute = %v, want %q", attrs["pfp"], "pintegrationfingerprint")
			}
		})
	}
}
