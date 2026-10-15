// Package update finds the latest hatrack release on GitHub and swaps the installed binaries for
// it, checking each download against the release's checksums.txt.
package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/baodq97/hatrack/internal/atomicfile"
)

// Base is where releases are published; tests point it at a local server.
var Base = "https://github.com/phongptn93/hatrack/releases"

// Off reports whether the user turned automatic update checks off. `hat update` still works.
func Off() bool { return os.Getenv("HATRACK_NO_UPDATE_CHECK") != "" }

// Latest returns the newest release's tag, such as v0.2.0. It follows GitHub's releases/latest
// redirect rather than the API, which limits callers without a token to 60 requests an hour.
func Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, Base+"/latest", nil)
	if err != nil {
		return "", err
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if loc == "" || !strings.Contains(loc, "/tag/") {
		return "", fmt.Errorf("no release found (%s)", resp.Status)
	}
	return path.Base(loc), nil
}

// Newer reports whether latest is a later version than current. A dev build is never behind.
func Newer(current, latest string) bool {
	c, ok1 := parse(current)
	l, ok2 := parse(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := range 3 {
		if c.n[i] != l.n[i] {
			return l.n[i] > c.n[i]
		}
	}
	// v1.0.0-beta.1 comes before v1.0.0
	return c.pre != "" && (l.pre == "" || l.pre > c.pre)
}

type version struct {
	n   [3]int
	pre string
}

func parse(v string) (version, bool) {
	var out version
	s, ok := strings.CutPrefix(v, "v")
	if !ok {
		return out, false
	}
	s, out.pre, _ = strings.Cut(s, "-")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out.n[i] = n
	}
	return out, true
}

// cache keeps the last answer from GitHub, so a tray asking every hour asks GitHub once a day.
type cache struct {
	Checked time.Time `json:"checked"`
	Latest  string    `json:"latest"`
}

// Cached is Latest, answered from the file at cachePath when it is younger than maxAge.
func Cached(ctx context.Context, cachePath string, maxAge time.Duration) (string, error) {
	var c cache
	if b, err := os.ReadFile(cachePath); err == nil && json.Unmarshal(b, &c) == nil &&
		c.Latest != "" && time.Since(c.Checked) < maxAge {
		return c.Latest, nil
	}
	tag, err := Latest(ctx)
	if err != nil {
		return "", err
	}
	if b, err := json.Marshal(cache{Checked: time.Now(), Latest: tag}); err == nil {
		atomicfile.Write(cachePath, b) // only a cache: asking again next time is fine
	}
	return tag, nil
}

// Asset is the release file name for a binary, such as hat-windows-amd64.exe.
func Asset(name, goos, goarch string) string {
	a := name + "-" + goos + "-" + goarch
	if goos == "windows" {
		a += ".exe"
	}
	return a
}

// Install downloads tag's assets and puts each in place of its file: files maps the path to
// replace to the asset that replaces it. Every download is checked before any file is touched.
// A running Windows exe cannot be overwritten but can be renamed, so each old file is moved
// to <file>.old first; Install removes those left by the last run.
func Install(ctx context.Context, tag string, files map[string]string) error {
	sums, err := checksums(ctx, tag)
	if err != nil {
		return err
	}
	tmps := map[string]string{}
	defer func() {
		for _, tmp := range tmps {
			os.Remove(tmp)
		}
	}()
	for dst, asset := range files {
		want, ok := sums[asset]
		if !ok {
			return fmt.Errorf("%s is not in %s's checksums.txt", asset, tag)
		}
		if olds, _ := filepath.Glob(dst + "*.old"); len(olds) > 0 {
			for _, o := range olds {
				os.Remove(o) // fails while that old copy still runs; next time then
			}
		}
		tmp, err := download(ctx, tag, asset, dst, want)
		if err != nil {
			return err
		}
		tmps[dst] = tmp
	}
	for dst, tmp := range tmps {
		old, err := moveAside(dst)
		if err != nil {
			return err
		}
		if err := os.Rename(tmp, dst); err != nil {
			if old != "" {
				os.Rename(old, dst)
			}
			return err
		}
		delete(tmps, dst)
	}
	return nil
}

// moveAside renames dst to dst.old, or to a fresh name when an old copy still running holds that
// one, and returns where it went: "" when there was no dst.
func moveAside(dst string) (string, error) {
	old := dst + ".old"
	err := os.Rename(dst, old)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		old = fmt.Sprintf("%s.%d.old", dst, time.Now().UnixNano())
		err = os.Rename(dst, old)
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return old, err
}

func get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

// checksums reads checksums.txt: sha256sum's "<hex>  <name>" per line.
func checksums(ctx context.Context, tag string) (map[string]string, error) {
	body, err := get(ctx, Base+"/download/"+tag+"/checksums.txt")
	if err != nil {
		return nil, err
	}
	defer body.Close()
	sums := map[string]string{}
	sc := bufio.NewScanner(body)
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) == 2 {
			sums[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
		}
	}
	return sums, sc.Err()
}

// download writes asset next to dst, so the final rename stays on one volume, and returns the
// temporary file once its checksum matches.
func download(ctx context.Context, tag, asset, dst, want string) (string, error) {
	body, err := get(ctx, Base+"/download/"+tag+"/"+asset)
	if err != nil {
		return "", err
	}
	defer body.Close()
	f, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".new-")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && hex.EncodeToString(h.Sum(nil)) != want {
		err = fmt.Errorf("%s: checksum does not match checksums.txt", asset)
	}
	if err == nil {
		err = os.Chmod(f.Name(), 0o755)
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
