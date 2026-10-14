package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/baodq97/hatrack/internal/atomicfile"
)

// Gone is a folder Claude remembers that no longer exists.
type Gone struct {
	Path        string
	MCPServers  int    // local-scope MCP servers set up for it
	Transcripts string // its folder under ~/.claude/projects, if any
	Bytes       int64  // size of that folder
}

// FindGone lists folders that ~/.claude.json or the transcripts remember but that were deleted.
// A folder counts as deleted only when its parent still exists, so a drive or share that is
// just not mounted is left alone.
func (s *Store) FindGone() ([]Gone, error) {
	byPath := map[string]*Gone{}
	b, err := os.ReadFile(s.GlobalJSON)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(b) > 0 {
		var m struct {
			Projects map[string]struct {
				MCPServers map[string]json.RawMessage `json:"mcpServers"`
			} `json:"projects"`
		}
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", s.GlobalJSON, err)
		}
		for p, v := range m.Projects {
			if gone(p) {
				byPath[pathKey(p)] = &Gone{Path: p, MCPServers: len(v.MCPServers)}
			}
		}
	}
	dirs, _ := os.ReadDir(filepath.Join(s.ConfigDir, "projects"))
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(s.ConfigDir, "projects", d.Name())
		cwd := transcriptCwd(dir)
		if cwd == "" || !gone(cwd) {
			continue
		}
		g := byPath[pathKey(cwd)]
		if g == nil {
			g = &Gone{Path: cwd}
			byPath[pathKey(cwd)] = g
		}
		g.Transcripts, g.Bytes = dir, dirSize(dir)
	}
	out := make([]Gone, 0, len(byPath))
	for _, g := range byPath {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Forget removes the folders from ~/.claude.json, after backing it up, and with transcripts
// set also deletes their transcripts.
func (s *Store) Forget(gs []Gone, transcripts bool) error {
	drop := map[string]bool{}
	for _, g := range gs {
		drop[pathKey(g.Path)] = true
	}
	err := atomicfile.Edit(s.GlobalJSON, s.BackupDir(), backups, func(b []byte) ([]byte, error) {
		m := map[string]json.RawMessage{}
		if len(bytes.TrimSpace(b)) == 0 {
			return b, nil
		}
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		var projects map[string]json.RawMessage
		if raw, ok := m["projects"]; ok {
			if err := json.Unmarshal(raw, &projects); err != nil {
				return nil, err
			}
		}
		if projects == nil {
			return b, nil // only transcripts to delete
		}
		for p := range projects {
			if drop[pathKey(p)] && gone(p) { // checked again: it may have come back meanwhile
				delete(projects, p)
			}
		}
		raw, err := atomicfile.JSON(projects)
		if err != nil {
			return nil, err
		}
		m["projects"] = bytes.TrimSpace(raw)
		return atomicfile.JSON(m)
	})
	if err != nil || !transcripts {
		return err
	}
	root := filepath.Join(s.ConfigDir, "projects") + string(filepath.Separator)
	for _, g := range gs {
		if g.Transcripts != "" && strings.HasPrefix(g.Transcripts, root) && gone(g.Path) {
			if err := os.RemoveAll(g.Transcripts); err != nil {
				return err
			}
		}
	}
	return nil
}

// pathKey makes the spellings of one folder equal: Claude files Windows folders under C:/x in
// .claude.json but records C:\x in transcripts, and Windows ignores case.
func pathKey(p string) string {
	p = filepath.Clean(filepath.FromSlash(p))
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

func gone(p string) bool {
	p = filepath.FromSlash(p)
	if !filepath.IsAbs(p) {
		return false
	}
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		return false
	}
	_, err := os.Stat(filepath.Dir(p))
	return err == nil
}

// transcriptCwd finds the folder a transcript folder belongs to, from the cwd its lines record.
// Folder names are encoded lossily, so they cannot be decoded instead.
func transcriptCwd(dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 64<<10), 8<<20)
		for i := 0; i < 50 && sc.Scan(); i++ {
			var l struct {
				Cwd string `json:"cwd"`
			}
			if json.Unmarshal(sc.Bytes(), &l) == nil && l.Cwd != "" {
				fh.Close()
				return l.Cwd
			}
		}
		fh.Close()
	}
	return ""
}

func dirSize(dir string) int64 {
	var n int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}
