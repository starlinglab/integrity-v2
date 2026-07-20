package pfp

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
)

// fakeAA is an in-memory stand-in for *aa.AuthAttrInstance. This drives computeAndSetPFP's
// guard logic directly.
type fakeAA struct {
	existing *aa.AttEntry // nil means "not found"
	getErr   error        // takes priority over existing, for non-ErrNotFound failures
	setErr   error

	getCalls int
	setCalls []aa.PostKV
}

func (f *fakeAA) GetAttestation(cid, attr string, opts aa.GetAttOpts) (*aa.AttEntry, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.existing == nil {
		return nil, aa.ErrNotFound
	}
	return f.existing, nil
}

func (f *fakeAA) SetAttestations(cid string, index bool, kvs []aa.PostKV) error {
	f.setCalls = append(f.setCalls, kvs...)
	return f.setErr
}

func attEntryWithValue(v string) *aa.AttEntry {
	ae := &aa.AttEntry{}
	ae.Attestation.Value = v
	return ae
}

// writeCidFile writes b to dir/cid (files are stored flat, keyed by CID with no extension) and
// returns cid, mirroring how upload/upload.go and c2pa/c2pa.go lay files out on disk.
func writeCidFile(t *testing.T, dir, cid string, b []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, cid), b, 0600); err != nil {
		t.Fatalf("writing cid file: %v", err)
	}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("encoding png: %v", err)
	}
	return buf.Bytes()
}

// pfpServer returns an httptest.Server that answers Nectar /pfps with a well-formed PFP,
// counting how many times it was hit.
func pfpServer(t *testing.T, pfp string) (srv *httptest.Server, calls *int) {
	t.Helper()
	calls = new(int)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"blobs":[{"pfp":"` + pfp + `"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, calls
}

func testConf(filesDir, nectarURL string) *config.Config {
	conf := &config.Config{}
	conf.Nectar.Url = nectarURL
	conf.Dirs.Files = filesDir
	return conf
}

func TestComputeAndSetPFP(t *testing.T) {
	ctx := context.Background()

	t.Run("nectar not configured", func(t *testing.T) {
		fa := &fakeAA{}
		_, err := computeAndSetPFP(ctx, testConf(t.TempDir(), ""), fa, "somecid", false)
		if err == nil {
			t.Fatal("expected an error when nectar is not configured")
		}
		if fa.getCalls != 0 || fa.setCalls != nil {
			t.Errorf("no AA calls should happen before the nectar check: get=%d, set=%v", fa.getCalls, fa.setCalls)
		}
	})

	t.Run("existing value short-circuits", func(t *testing.T) {
		srv, calls := pfpServer(t, "pfresh")
		fa := &fakeAA{existing: attEntryWithValue("pexisting")}

		dir := t.TempDir()
		got, err := computeAndSetPFP(ctx, testConf(dir, srv.URL), fa, "somecid", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "pexisting" {
			t.Errorf("got %q, want existing value %q", got, "pexisting")
		}
		if *calls != 0 {
			t.Errorf("nectar should not be called when a pfp already exists, got %d calls", *calls)
		}
		if fa.setCalls != nil {
			t.Errorf("AA should not be written to when a pfp already exists, got %v", fa.setCalls)
		}
	})

	t.Run("force recomputes and overwrites existing value", func(t *testing.T) {
		srv, calls := pfpServer(t, "pfresh")
		fa := &fakeAA{existing: attEntryWithValue("pexisting")}

		dir := t.TempDir()
		writeCidFile(t, dir, "somecid", pngBytes(t))

		got, err := computeAndSetPFP(ctx, testConf(dir, srv.URL), fa, "somecid", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "pfresh" {
			t.Errorf("got %q, want freshly computed value %q", got, "pfresh")
		}
		if *calls != 1 {
			t.Errorf("nectar should be called exactly once with force, got %d calls", *calls)
		}
		if fa.getCalls != 0 {
			t.Errorf("the existing-value check should be skipped entirely with force, got %d get calls", fa.getCalls)
		}
		if len(fa.setCalls) != 1 || fa.setCalls[0].Value != "pfresh" {
			t.Errorf("expected a single set of the fresh value, got %v", fa.setCalls)
		}
	})

	t.Run("no existing value computes and sets", func(t *testing.T) {
		srv, calls := pfpServer(t, "pnew")
		fa := &fakeAA{}

		dir := t.TempDir()
		writeCidFile(t, dir, "somecid", pngBytes(t))

		got, err := computeAndSetPFP(ctx, testConf(dir, srv.URL), fa, "somecid", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "pnew" {
			t.Errorf("got %q, want %q", got, "pnew")
		}
		if *calls != 1 {
			t.Errorf("expected exactly one nectar call, got %d", *calls)
		}
		if len(fa.setCalls) != 1 || fa.setCalls[0].Key != "pfp" || fa.setCalls[0].Value != "pnew" || fa.setCalls[0].Type != "str" {
			t.Errorf("unexpected attestation written: %+v", fa.setCalls)
		}
	})

	t.Run("aa get error other than not-found propagates", func(t *testing.T) {
		srv, calls := pfpServer(t, "pfresh")
		fa := &fakeAA{getErr: errors.New("aa unreachable")}

		_, err := computeAndSetPFP(ctx, testConf(t.TempDir(), srv.URL), fa, "somecid", false)
		if err == nil {
			t.Fatal("expected error to propagate")
		}
		if *calls != 0 {
			t.Errorf("nectar should not be called when the existence check fails, got %d calls", *calls)
		}
	})

	t.Run("missing file on disk", func(t *testing.T) {
		srv, _ := pfpServer(t, "pfresh")
		fa := &fakeAA{}
		_, err := computeAndSetPFP(ctx, testConf(t.TempDir(), srv.URL), fa, "nonexistent", false)
		if err == nil {
			t.Fatal("expected an error for a CID with no file on disk")
		}
	})

	t.Run("unsupported media type", func(t *testing.T) {
		srv, calls := pfpServer(t, "pfresh")
		fa := &fakeAA{}

		dir := t.TempDir()
		writeCidFile(t, dir, "somecid", []byte("just some text, not an image\n"))

		_, err := computeAndSetPFP(ctx, testConf(dir, srv.URL), fa, "somecid", false)
		if err == nil {
			t.Fatal("expected an error for an unsupported media type")
		}
		if *calls != 0 {
			t.Errorf("nectar should not be called for an unsupported media type, got %d calls", *calls)
		}
		if fa.setCalls != nil {
			t.Errorf("AA should not be written to for an unsupported media type, got %v", fa.setCalls)
		}
	})

	t.Run("aa set error propagates", func(t *testing.T) {
		srv, calls := pfpServer(t, "pfresh")
		fa := &fakeAA{setErr: errors.New("aa write failed")}

		dir := t.TempDir()
		writeCidFile(t, dir, "somecid", pngBytes(t))

		_, err := computeAndSetPFP(ctx, testConf(dir, srv.URL), fa, "somecid", false)
		if err == nil {
			t.Fatal("expected the AA write error to propagate")
		}
		if *calls != 1 {
			t.Errorf("nectar should still have been called once before the failed write, got %d", *calls)
		}
	})
}
