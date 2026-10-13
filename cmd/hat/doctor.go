package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/baodq97/hatrack/internal/claude"
	"github.com/baodq97/hatrack/internal/mcp"
)

type report struct{ bad, warn int }

func (r *report) ok(format string, a ...any) { fmt.Printf("✓ "+format+"\n", a...) }
func (r *report) warning(format string, a ...any) {
	r.warn++
	fmt.Printf("! "+format+"\n", a...)
}
func (r *report) fail(format string, a ...any) {
	r.bad++
	fmt.Printf("✗ "+format+"\n", a...)
}

// doctor checks the whole Claude Code setup hat manages, changing nothing.
func doctor(s *claude.Store) error {
	r := &report{}

	// Claude Code itself
	if path, err := exec.LookPath("claude"); err != nil {
		r.warning("the claude command is not on PATH (fine if you only use the VS Code extension)")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		out, err := exec.CommandContext(ctx, path, "--version").Output()
		cancel()
		if err != nil {
			r.warning("claude is at %s but `claude --version` failed: %v", path, err)
		} else {
			r.ok("Claude Code %s", strings.TrimSpace(string(out)))
		}
	}

	// the login
	cur, err := s.Current()
	switch {
	case errors.Is(err, claude.ErrNotSignedIn):
		r.fail("Claude is not signed in: sign in, then `hat save`")
	case err != nil:
		r.fail("reading the login: %v", err)
	case cur.Name == "":
		r.warning("signed in as %s, which hat has not saved: `hat save` keeps it", cur.Email)
	default:
		plan := ""
		if cur.Plan != "" {
			plan = ", " + cur.Plan
		}
		r.ok("signed in as %s (saved as %s%s)", cur.Email, cur.Name, plan)
	}
	if runtime.GOOS != "windows" {
		for _, f := range []string{s.CredentialsPath(), s.Dir} {
			if st, err := os.Stat(f); err == nil && st.Mode().Perm()&0o077 != 0 {
				r.warning("%s can be read by other users (mode %o): chmod go-rwx %s", f, st.Mode().Perm(), f)
			}
		}
	}
	if ps, err := s.List(); err == nil {
		r.ok("%s", plural(len(ps), "saved account"))
	}

	// Claude's files
	if st, err := os.Stat(s.GlobalJSON); err == nil {
		var m map[string]json.RawMessage
		b, _ := os.ReadFile(s.GlobalJSON)
		if err := json.Unmarshal(b, &m); err != nil {
			r.fail("%s is not valid JSON: %v (hat keeps backups in %s)", s.GlobalJSON, err, s.BackupDir())
		} else {
			var projects map[string]json.RawMessage
			json.Unmarshal(m["projects"], &projects)
			msg := fmt.Sprintf("%s: %s, %s remembered", tildePath(s.GlobalJSON), size(st.Size()), plural(len(projects), "folder"))
			if st.Size() > 20<<20 {
				r.warning("%s; a file this large slows Claude down, see `hat clean`", msg)
			} else {
				r.ok("%s", msg)
			}
		}
	}
	settings := filepath.Join(s.ConfigDir, "settings.json")
	if b, err := os.ReadFile(settings); err == nil {
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil {
			r.fail("%s is not valid JSON, so Claude ignores it: %v", settings, err)
		} else {
			days := "30 (default)"
			if d, ok := v["cleanupPeriodDays"]; ok {
				days = fmt.Sprint(d)
			}
			r.ok("%s is valid; Claude keeps transcripts %s days", tildePath(settings), days)
		}
	}
	tdir := filepath.Join(s.ConfigDir, "projects")
	if _, err := os.Stat(tdir); err == nil {
		r.ok("transcripts: %s in %s", size(dirSize(tdir)), tildePath(tdir))
	}
	if gs, err := s.FindGone(); err == nil && len(gs) > 0 {
		r.warning("%s still remembered: `hat clean` shows them", plural(len(gs), "deleted folder"))
	}

	// MCP servers
	if wd, err := os.Getwd(); err == nil {
		if c, err := mcp.Default(wd); err == nil {
			if ss, err := c.List(); err != nil {
				r.fail("MCP servers: %v", err)
			} else {
				on, issues := 0, 0
				for _, sv := range ss {
					if sv.Enabled {
						on++
						if len(mcp.Check(sv)) > 0 {
							issues++
						}
					}
				}
				if issues > 0 {
					r.warning("MCP: %d of %d servers on have problems: `hat mcp doctor`", issues, on)
				} else {
					r.ok("MCP: %d servers on, %d off, no problems found", on, len(ss)-on)
				}
			}
		}
	}

	fmt.Println()
	switch {
	case r.bad > 0:
		return fmt.Errorf("%s, %s", plural(r.bad, "problem"), plural(r.warn, "warning"))
	case r.warn > 0:
		fmt.Printf("%s.\n", plural(r.warn, "warning"))
	default:
		fmt.Println("All good.")
	}
	return nil
}

func clean(s *claude.Store, args []string) error {
	apply, transcripts := false, false
	for _, a := range args {
		switch a {
		case "--apply":
			apply = true
		case "--transcripts":
			transcripts = true
		default:
			return fmt.Errorf("usage: hat clean [--transcripts] [--apply]")
		}
	}
	gs, err := s.FindGone()
	if err != nil {
		return err
	}
	if len(gs) == 0 {
		fmt.Println("Nothing to clean: every folder Claude remembers still exists.")
		return nil
	}
	var bytes int64
	fmt.Println("Folders Claude remembers that were deleted:")
	for _, g := range gs {
		notes := []string{}
		if g.MCPServers > 0 {
			notes = append(notes, plural(g.MCPServers, "local MCP server"))
		}
		if g.Transcripts != "" {
			notes = append(notes, "transcripts "+size(g.Bytes))
			bytes += g.Bytes
		}
		extra := ""
		if len(notes) > 0 {
			extra = "  (" + strings.Join(notes, ", ") + ")"
		}
		fmt.Printf("  %s%s\n", g.Path, extra)
	}
	if !apply {
		fmt.Println("\nNothing was changed. `hat clean --apply` forgets these folders in ~/.claude.json;")
		fmt.Printf("add --transcripts to also delete their transcripts (%s; they also hold their usage history).\n", size(bytes))
		return nil
	}
	if err := s.Forget(gs, transcripts); err != nil {
		return err
	}
	fmt.Printf("\nForgot %s. The previous ~/.claude.json is in %s.\n", plural(len(gs), "folder"), s.BackupDir())
	if transcripts {
		fmt.Printf("Freed %s of transcripts.\n", size(bytes))
	}
	return nil
}

func size(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%d B", b)
}

func dirSize(dir string) int64 {
	var n int64
	filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
