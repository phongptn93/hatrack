package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindGoneAndForget(t *testing.T) {
	s := newStore(t)
	root := filepath.Dir(s.Dir)
	kept := filepath.ToSlash(filepath.Join(root, "kept"))
	deleted := filepath.ToSlash(filepath.Join(root, "deleted"))
	unmounted := filepath.ToSlash(filepath.Join(root, "nodrive", "proj")) // parent missing too
	must(t, os.MkdirAll(filepath.FromSlash(kept), 0o700))
	write(t, s.GlobalJSON, `{"numStartups":7,"oauthAccount":{"emailAddress":"a@x.com"},"projects":{
		"`+kept+`":{"a":1},"`+deleted+`":{"mcpServers":{"db":{"command":"x"}}},"`+unmounted+`":{"b":2}}}`)
	tdir := filepath.Join(s.ConfigDir, "projects", "-deleted")
	write(t, filepath.Join(tdir, "s.jsonl"), `{"type":"user","cwd":"`+strings.ReplaceAll(filepath.FromSlash(deleted), `\`, `\\`)+`"}`+"\n")

	gs, err := s.FindGone()
	must(t, err)
	// one folder, though .claude.json and the transcript spell it differently on Windows
	if len(gs) != 1 || gs[0].Path != deleted || gs[0].MCPServers != 1 || gs[0].Transcripts != tdir {
		t.Fatalf("gone: %+v", gs)
	}

	must(t, s.Forget(gs, true))
	b := readFile(t, s.GlobalJSON)
	if strings.Contains(b, `"`+deleted+`"`) || !strings.Contains(b, kept) || !strings.Contains(b, unmounted) || !strings.Contains(b, `"numStartups": 7`) {
		t.Fatalf("after forget:\n%s", b)
	}
	if _, err := os.Stat(tdir); !os.IsNotExist(err) {
		t.Fatal("transcripts kept")
	}
	if bk, _ := filepath.Glob(filepath.Join(s.BackupDir(), "claude.json.*")); len(bk) != 1 {
		t.Fatalf("backups: %v", bk)
	}
}

func TestSwitchHistory(t *testing.T) {
	s := newStore(t)
	signIn(t, filepath.Join(s.ConfigDir, credsFile), s.GlobalJSON, "a@x.com", "a1")
	_, err := s.Save("")
	must(t, err)
	_, err = s.Add("", func(dir string) error {
		signIn(t, filepath.Join(dir, credsFile), filepath.Join(dir, ".claude.json"), "b@x.com", "b1")
		return nil
	})
	must(t, err)
	must(t, s.Use("b@x.com"))
	must(t, s.Use("b@x.com")) // no second entry for the same account
	h, err := s.Switches()
	must(t, err)
	if len(h) != 2 || h[0].Account != "a@x.com" || h[1].Account != "b@x.com" {
		t.Fatalf("history: %+v", h)
	}
	if AccountAt(h, h[0].Time.Add(-time.Second)) != "" || AccountAt(h, h[1].Time) != "b@x.com" {
		t.Fatal("AccountAt")
	}
	cur, err := s.Current()
	must(t, err)
	if cur.Name != "b@x.com" {
		t.Fatalf("current: %+v", cur)
	}
	must(t, s.Rename("b@x.com", "work")) // usage follows the account to its new name
	if h, _ := s.Switches(); h[1].Account != "work" || h[0].Account != "a@x.com" {
		t.Fatalf("history after rename: %+v", h)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	must(t, err)
	return string(b)
}
