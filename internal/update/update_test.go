package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		cur, latest string
		want        bool
	}{
		{"v0.1.0", "v0.1.1", true},
		{"v0.1.9", "v0.2.0", true},
		{"v0.10.0", "v0.9.0", false},
		{"v1.0.0", "v1.0.0", false},
		{"v1.0.0-beta.1", "v1.0.0", true},
		{"v1.0.0", "v1.0.1-beta.1", true},
		{"v1.0.0", "v1.0.0-beta.1", false},
		{"dev", "v9.0.0", false},
		{"v1.0.0", "garbage", false},
	} {
		if got := Newer(c.cur, c.latest); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.cur, c.latest, got)
		}
	}
}

func serve(t *testing.T, assets map[string]string, sums string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tag/v9.9.9", http.StatusFound)
	})
	mux.HandleFunc("/download/v9.9.9/checksums.txt", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(sums)) })
	for name, body := range assets {
		mux.HandleFunc("/download/v9.9.9/"+name, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	old := Base
	Base = srv.URL
	t.Cleanup(func() { Base = old })
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestLatestAndCached(t *testing.T) {
	serve(t, nil, "")
	ctx := context.Background()
	if tag, err := Latest(ctx); err != nil || tag != "v9.9.9" {
		t.Fatalf("latest: %q %v", tag, err)
	}
	p := filepath.Join(t.TempDir(), "update.json")
	if tag, err := Cached(ctx, p, time.Hour); err != nil || tag != "v9.9.9" {
		t.Fatalf("cached: %q %v", tag, err)
	}
	Base = "http://127.0.0.1:1" // nothing listens: only the cache can answer
	if tag, err := Cached(ctx, p, time.Hour); err != nil || tag != "v9.9.9" {
		t.Fatalf("from cache: %q %v", tag, err)
	}
	if _, err := Cached(ctx, p, 0); err == nil {
		t.Fatal("a stale cache should ask again")
	}
}

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	hat, tray := filepath.Join(dir, "hat.exe"), filepath.Join(dir, "hatrack-tray.exe")
	os.WriteFile(hat, []byte("old hat"), 0o755)
	os.WriteFile(hat+".old", []byte("from the last update"), 0o755)
	serve(t, map[string]string{"hat-windows-amd64.exe": "new hat", "hatrack-tray-windows-amd64.exe": "new tray", "bad.exe": "tampered"},
		sum("new hat")+"  hat-windows-amd64.exe\n"+sum("new tray")+" *hatrack-tray-windows-amd64.exe\n"+sum("original")+"  bad.exe\n")
	ctx := context.Background()

	if err := Install(ctx, "v9.9.9", map[string]string{hat: "bad.exe"}); err == nil {
		t.Fatal("a checksum mismatch should fail")
	}
	if err := Install(ctx, "v9.9.9", map[string]string{hat: "missing.exe"}); err == nil {
		t.Fatal("an asset missing from checksums.txt should fail")
	}
	if b, _ := os.ReadFile(hat); string(b) != "old hat" {
		t.Fatalf("a failed install changed hat.exe: %q", b)
	}

	if err := Install(ctx, "v9.9.9", map[string]string{hat: "hat-windows-amd64.exe", tray: "hatrack-tray-windows-amd64.exe"}); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{hat: "new hat", tray: "new tray", hat + ".old": "old hat"} {
		if b, _ := os.ReadFile(p); string(b) != want {
			t.Errorf("%s = %q, want %q", filepath.Base(p), b, want)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*")); len(left) > 0 {
		t.Errorf("temporary files left: %v", left)
	}
}
