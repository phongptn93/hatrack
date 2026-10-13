package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baodq97/hatrack/internal/claude"
)

func newServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	store := &claude.Store{
		ConfigDir:  filepath.Join(dir, "config"),
		GlobalJSON: filepath.Join(dir, "global.json"),
		Dir:        filepath.Join(dir, "parked"),
	}
	if err := os.MkdirAll(store.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(store, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Listener.Close() })
	return srv
}

func (srv *Server) do(method, path, body string, set func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = srv.host
	r.Header.Set(tokenHeader, srv.token)
	if method == http.MethodPost {
		r.Header.Set("Content-Type", "application/json")
	}
	if set != nil {
		set(r)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	return w
}

func TestListProfiles(t *testing.T) {
	srv := newServer(t)
	w := srv.do(http.MethodGet, "/api/profiles", "", nil)
	var data struct{ Status string }
	if err := json.NewDecoder(w.Body).Decode(&data); err != nil || w.Code != http.StatusOK || data.Status != "ok" {
		t.Fatalf("code=%d status=%q err=%v", w.Code, data.Status, err)
	}
	if !strings.Contains(srv.URL(), "#t="+srv.token) {
		t.Fatalf("url %q does not carry the token", srv.URL())
	}
}

func TestOnlyTheDashboardCanCallTheAPI(t *testing.T) {
	srv := newServer(t)
	added := false
	srv.AddAccount = func(string) error { added = true; return nil }

	cases := map[string]func(*http.Request){
		"no token":      func(r *http.Request) { r.Header.Del(tokenHeader) },
		"wrong token":   func(r *http.Request) { r.Header.Set(tokenHeader, "x"+srv.token[1:]) },
		"other origin":  func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
		"rebound host":  func(r *http.Request) { r.Host = "evil.example:" + strings.Split(srv.host, ":")[1] },
		"form post":     func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") },
		"no media type": func(r *http.Request) { r.Header.Del("Content-Type") },
	}
	for name, set := range cases {
		if w := srv.do(http.MethodPost, "/api/add", `{}`, set); w.Code < 400 {
			t.Errorf("%s: got %d", name, w.Code)
		}
	}
	if added {
		t.Fatal("a refused request still ran hat add")
	}
	if w := srv.do(http.MethodPost, "/api/add", `{}`, func(r *http.Request) { r.Header.Set("Origin", srv.Addr) }); w.Code != http.StatusOK || !added {
		t.Fatalf("dashboard request refused: %d %s", w.Code, w.Body)
	}
	if w := srv.do(http.MethodPost, "/api/add", `{"name":"../x"}`, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("bad name reached hat add: %d", w.Code)
	}
}

func TestRenameRejectsPaths(t *testing.T) {
	srv := newServer(t)
	victim := filepath.Join(filepath.Dir(srv.Store.Dir), "victim")
	if err := os.MkdirAll(victim, 0o700); err != nil {
		t.Fatal(err)
	}
	w := srv.do(http.MethodPost, "/api/rename", `{"oldName":"../victim","newName":"stolen"}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d", w.Code)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("folder outside ~/.hatrack moved: %v", err)
	}
}

func TestPageHasNoOutsideResources(t *testing.T) {
	srv := newServer(t)
	w := srv.do(http.MethodGet, "/", "", func(r *http.Request) { r.Header.Del(tokenHeader) })
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "https://") {
		t.Fatalf("code=%d, page links outside resources", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatal("no CSP")
	}
}
