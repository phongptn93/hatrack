package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/baodq97/hatrack/internal/claude"
)

func report(five, week float64, fiveReset, weekReset time.Time) []byte {
	return []byte(fmt.Sprintf(`{"session_id":"s","model":{"id":"m"},"rate_limits":{"five_hour":{"used_percentage":%g,"resets_at":%d},"seven_day":{"used_percentage":%g,"resets_at":%d}}}`,
		five, fiveReset.Unix(), week, weekReset.Unix()))
}

func TestRecordWarnsOncePerWindow(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	r5, r7 := now.Add(2*time.Hour), now.Add(72*time.Hour)

	_, ws, err := Record(dir, "work", report(40, 20, r5, r7), now)
	if err != nil || len(ws) != 0 {
		t.Fatalf("below the alert: %v %v", ws, err)
	}
	_, ws, _ = Record(dir, "work", report(83.4, 20, r5, r7), now)
	if len(ws) != 1 || ws[0].Kind != "5-hour" || ws[0].Account != "work" {
		t.Fatalf("crossing 80%%: %v", ws)
	}
	_, ws, _ = Record(dir, "work", report(91, 95, r5, r7), now)
	if len(ws) != 1 || ws[0].Kind != "weekly" { // 5-hour already announced this window
		t.Fatalf("again: %v", ws)
	}
	// a new 5-hour window announces again
	_, ws, _ = Record(dir, "work", report(85, 95, r5.Add(5*time.Hour), r7), now.Add(3*time.Hour))
	if len(ws) != 1 || ws[0].Kind != "5-hour" {
		t.Fatalf("next window: %v", ws)
	}
	// other accounts are separate; no rate_limits (API key, first prompt) changes nothing
	if _, ws, _ = Record(dir, "personal", []byte(`{"model":{}}`), now); len(ws) != 0 {
		t.Fatalf("no limits: %v", ws)
	}

	l, err := LoadLimits(dir)
	if err != nil {
		t.Fatal(err)
	}
	line, warn := l.Line("work", now)
	if !warn || !strings.Contains(line, "⚠ 5h 85%") || !strings.Contains(line, "⚠ 7d 95%") {
		t.Fatalf("line: %q", line)
	}
	if line, _ := l.Line("work", now.Add(100*time.Hour)); line != "" {
		t.Fatalf("after every window reset: %q", line)
	}

	// raise the alert, then switch it off
	if err := EditLimits(dir, func(l *Limits) error { l.Alert.FiveHour = 90; return nil }); err != nil {
		t.Fatal(err)
	}
	l, _ = LoadLimits(dir)
	if ws := l.Warnings("work", now.Add(3*time.Hour)); len(ws) != 1 || ws[0].Kind != "weekly" {
		t.Fatalf("alert at 90: %v", ws)
	}
	EditLimits(dir, func(l *Limits) error { l.Alert.Off = true; return nil })
	l, _ = LoadLimits(dir)
	if ws := l.Warnings("work", now); len(ws) != 0 {
		t.Fatalf("alert off: %v", ws)
	}
}

func TestMenuLinePrefersLimits(t *testing.T) {
	root := t.TempDir()
	s := &claude.Store{ConfigDir: filepath.Join(root, ".claude"), GlobalJSON: filepath.Join(root, ".claude.json"), Dir: filepath.Join(root, ".hatrack", "claude")}
	os.WriteFile(s.GlobalJSON, []byte(`{"oauthAccount":{"accountUuid":"u","organizationUuid":"o","emailAddress":"w@corp.com"}}`), 0o600)
	var c MenuCache
	if line, warn, _ := c.Line(s); line != "" || warn {
		t.Fatalf("nothing known: %q", line)
	}
	now := time.Now()
	Record(filepath.Dir(s.Dir), "w@corp.com", report(86.7, 10, now.Add(time.Hour), now.Add(48*time.Hour)), now)
	line, warn, title := c.Line(s)
	if !warn || title != "⚠ 86%" || !strings.HasPrefix(line, "Limits: ⚠ 5h 86%") {
		t.Fatalf("got %q %v %q", line, warn, title)
	}
}
