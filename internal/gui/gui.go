// Package gui serves the hatrack dashboard on 127.0.0.1. Every API call must carry the token
// handed to the window that hatrack opens, so other pages in the browser cannot drive it.
package gui

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/baodq97/hatrack/internal/claude"
	"github.com/baodq97/hatrack/internal/mcp"
)

//go:embed assets/*
var assetsEmbed embed.FS

const tokenHeader = "X-Hatrack-Token"

// csp keeps the page to its own files: no fonts, scripts or requests from anywhere else.
const csp = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

type Server struct {
	Store    *claude.Store
	Listener net.Listener
	Addr     string // http://127.0.0.1:port
	host     string // 127.0.0.1:port, the only Host header accepted
	token    string
	// AddAccount runs `hat add [name]` where the user can see it and answer its prompts.
	AddAccount func(name string) error
}

func NewServer(s *claude.Store, preferredPort int) (*Server, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferredPort))
	if err != nil {
		if ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			return nil, fmt.Errorf("listen: %w", err)
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		ln.Close()
		return nil, err
	}
	host := ln.Addr().String()
	return &Server{
		Store:      s,
		Listener:   ln,
		Addr:       "http://" + host,
		host:       host,
		token:      hex.EncodeToString(b),
		AddAccount: OpenAddInTerminal,
	}, nil
}

// URL opens the dashboard. The token rides in the fragment, which the browser never sends
// to a server or puts in a Referer.
func (srv *Server) URL() string { return srv.Addr + "/#t=" + srv.token }

func (srv *Server) Start() error { return http.Serve(srv.Listener, srv.Handler()) }

func (srv *Server) Handler() http.Handler {
	sub, err := fs.Sub(assetsEmbed, "assets")
	if err != nil {
		panic(err)
	}
	api := http.NewServeMux()
	api.HandleFunc("GET /api/profiles", srv.handleListProfiles)
	api.HandleFunc("POST /api/use", srv.handleUseProfile)
	api.HandleFunc("POST /api/save", srv.handleSaveProfile)
	api.HandleFunc("POST /api/add", srv.handleAddProfile)
	api.HandleFunc("POST /api/rename", srv.handleRenameProfile)
	api.HandleFunc("POST /api/remove", srv.handleRemoveProfile)

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.Handle("/api/", srv.guard(api))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		// a name that resolves to 127.0.0.1 (DNS rebinding) must not reach the dashboard
		if r.Host != srv.host {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// guard lets an API call through only from the dashboard itself.
func (srv *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && o != srv.Addr {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(tokenHeader)), []byte(srv.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// a page elsewhere can only send JSON after a CORS preflight, which this server never answers
		if r.Method != http.MethodGet {
			if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
				http.Error(w, "want application/json", http.StatusUnsupportedMediaType)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// OpenAppWindow opens the dashboard, as an app window when Chrome is installed.
func (srv *Server) OpenAppWindow() error {
	url := srv.URL()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		chrome := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		profile, err := os.UserCacheDir()
		if _, serr := os.Stat(chrome); serr == nil && err == nil {
			profile = filepath.Join(profile, "hatrack", "chrome-app")
			cmd = exec.Command(chrome, "--app="+url, "--user-data-dir="+profile)
		} else {
			cmd = exec.Command("open", url)
		}
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

type nameReq struct {
	Name string `json:"name"`
}

func (srv *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	ps, err := srv.Store.List()
	if err != nil {
		fail(w, err, http.StatusInternalServerError)
		return
	}
	jsonResp(w, map[string]any{"status": "ok", "profiles": ps}, http.StatusOK)
}

func (srv *Server) handleUseProfile(w http.ResponseWriter, r *http.Request) {
	var req nameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		fail(w, errors.New("invalid name"), http.StatusBadRequest)
		return
	}
	if _, _, err := mcp.SwitchAccount(srv.Store, req.Name); err != nil { // with its linked MCP set
		fail(w, err, http.StatusInternalServerError)
		return
	}
	jsonResp(w, map[string]any{"status": "ok"}, http.StatusOK)
}

func (srv *Server) handleSaveProfile(w http.ResponseWriter, r *http.Request) {
	var req nameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	name, err := srv.Store.Save(req.Name)
	if err != nil {
		fail(w, err, http.StatusInternalServerError)
		return
	}
	jsonResp(w, map[string]any{"status": "ok", "name": name}, http.StatusOK)
}

// handleAddProfile hands sign-in to `hat add` in a terminal: it asks how to open the
// sign-in page and may need a pasted code, which a background process cannot do.
func (srv *Server) handleAddProfile(w http.ResponseWriter, r *http.Request) {
	var req nameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	if req.Name != "" && !claude.ValidName(req.Name) {
		fail(w, fmt.Errorf("bad profile name %q", req.Name), http.StatusBadRequest)
		return
	}
	if err := srv.AddAccount(req.Name); err != nil {
		fail(w, err, http.StatusInternalServerError)
		return
	}
	jsonResp(w, map[string]any{"status": "ok"}, http.StatusOK)
}

func (srv *Server) handleRenameProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldName string `json:"oldName"`
		NewName string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	if err := mcp.RenameAccount(srv.Store, req.OldName, req.NewName); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	jsonResp(w, map[string]any{"status": "ok"}, http.StatusOK)
}

func (srv *Server) handleRemoveProfile(w http.ResponseWriter, r *http.Request) {
	var req nameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		fail(w, errors.New("invalid name"), http.StatusBadRequest)
		return
	}
	if err := srv.Store.Remove(req.Name); err != nil {
		fail(w, err, http.StatusInternalServerError)
		return
	}
	jsonResp(w, map[string]any{"status": "ok"}, http.StatusOK)
}

func fail(w http.ResponseWriter, err error, code int) {
	jsonResp(w, map[string]any{"status": "error", "error": err.Error()}, code)
}

func jsonResp(w http.ResponseWriter, data map[string]any, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("encode response: %v", err)
	}
}
