package nectar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestComputePFPFromReaderSuccess(t *testing.T) {
	const wantPFP = "pabcdefghijklmnop"
	var gotField bool
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if _, _, err := r.FormFile("image"); err == nil {
			gotField = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"blobs":[{"pfp":"` + wantPFP + `"}]}`))
	}))
	defer srv.Close()

	pfp, err := computePFPFromReader(context.Background(),
		strings.NewReader("fake image bytes"), "test.png", srv.URL, "secret-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pfp != wantPFP {
		t.Errorf("pfp = %q, want %q", pfp, wantPFP)
	}
	if !gotField {
		t.Error("server did not receive multipart field \"image\"")
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer secret-token")
	}
}

func TestComputePFPFromReaderNoTokenOmitsAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"blobs":[{"pfp":"pxyz"}]}`))
	}))
	defer srv.Close()

	if _, err := computePFPFromReader(context.Background(),
		strings.NewReader("x"), "t.png", srv.URL, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty when token unset", gotAuth)
	}
}

func TestComputePFPFromReaderErrors(t *testing.T) {
	cases := []struct {
		name          string
		status        int
		body          string
		wantErrSubstr string
	}{
		{"non-200", http.StatusBadRequest, "invalid or unsupported image", "status code 400"},
		{"malformed json", http.StatusOK, `{not json`, ""},
		{"no blobs", http.StatusOK, `{"blobs":[]}`, ""},
		{"empty pfp", http.StatusOK, `{"blobs":[{"pfp":""}]}`, ""},
		{"bad prefix", http.StatusOK, `{"blobs":[{"pfp":"xdeadbeef"}]}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			_, err := computePFPFromReader(context.Background(),
				strings.NewReader("x"), "t.png", srv.URL, "")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if tc.wantErrSubstr != "" && !strings.Contains(err.Error(), tc.wantErrSubstr) {
				t.Errorf("error = %v, want substring %q", err, tc.wantErrSubstr)
			}
		})
	}
}
