package usage

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/baodq97/hatrack/internal/claude"
)

func msg(ts, id, req, model string, in, out int) string {
	return `{"type":"assistant","timestamp":"` + ts + `","cwd":"/w/app","sessionId":"s1","requestId":"` + req +
		`","message":{"id":"` + id + `","model":"` + model + `","content":[{"type":"text","text":"x"}],"usage":{"input_tokens":` +
		strconv.Itoa(in) + `,"output_tokens":` + strconv.Itoa(out) + `,"cache_creation_input_tokens":10,"cache_read_input_tokens":100}}}`
}

func TestScanDedupesAndFilters(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		`{"type":"user","timestamp":"2026-10-01T08:00:00Z","message":{"content":"hi"}}`,
		msg("2026-10-01T08:00:05Z", "m1", "r1", "claude-sonnet-5-5", 5, 50),
		msg("2026-10-01T08:00:05Z", "m1", "r1", "claude-sonnet-5-5", 5, 50), // same response, second content block
		msg("2026-10-01T09:30:00Z", "m2", "r2", "claude-opus-5-5", 1, 20),
		msg("2026-10-01T09:31:00Z", "m3", "r3", "<synthetic>", 0, 0),
		`not json at all, with "usage" in it`,
		msg("2026-09-01T09:31:00Z", "m0", "r0", "claude-opus-5-5", 1, 1), // before since
	}
	p := filepath.Join(dir, "-w-app", "s1.jsonl")
	os.MkdirAll(filepath.Dir(p), 0o700)
	os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o600)

	es, err := Scan(dir, time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 {
		t.Fatalf("entries: %+v", es)
	}
	if s := Sum(es); s.Input != 6 || s.Output != 70 || s.CacheWrite != 20 || s.CacheRead != 200 || s.Total() != 296 {
		t.Fatalf("sum: %+v", s)
	}
	byModel := Group(es, func(e Entry) string { return e.Model })
	if len(byModel) != 2 || byModel[0].Key != "claude-opus-5-5" || byModel[1].Output != 50 {
		t.Fatalf("by model: %+v", byModel)
	}
}

func TestBlocks(t *testing.T) {
	at := func(s string) Entry {
		tm, _ := time.Parse(time.RFC3339, s)
		return Entry{Time: tm, Output: 1}
	}
	bs := Blocks([]Entry{
		at("2026-10-01T08:20:00Z"), at("2026-10-01T12:59:00Z"), // window 08:00-13:00
		at("2026-10-01T13:05:00Z"), // past the end: new window from 13:00
		at("2026-10-01T23:00:00Z"), // after a long gap
	})
	if len(bs) != 3 || bs[0].Responses != 2 || !bs[1].Start.Equal(at("2026-10-01T13:00:00Z").Time) || bs[2].Responses != 1 {
		t.Fatalf("blocks: %+v", bs)
	}
	if !bs[2].Active(at("2026-10-02T01:00:00Z").Time) || bs[2].Active(at("2026-10-02T04:00:00Z").Time) {
		t.Fatal("active window")
	}
}

func TestToday(t *testing.T) {
	root := t.TempDir()
	s := &claude.Store{ConfigDir: filepath.Join(root, ".claude"), GlobalJSON: filepath.Join(root, ".claude.json"), Dir: filepath.Join(root, ".hatrack", "claude")}
	now := time.Date(2026, 10, 13, 18, 30, 0, 0, time.UTC)
	ts := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	p := filepath.Join(s.ConfigDir, "projects", "-w", "s.jsonl")
	os.MkdirAll(filepath.Dir(p), 0o700)
	os.WriteFile(p, []byte(strings.Join([]string{
		msg(ts(30*time.Hour), "m0", "r0", "m", 1000, 1000), // yesterday: not counted
		msg(ts(2*time.Hour), "m1", "r1", "m", 1_000_000, 500_000),
		msg(ts(time.Hour), "m2", "r2", "m", 0, 89),
	}, "\n")), 0o600)
	// 1,500,000 + 89 + 2×(10+100) = 1,500,309; the window opened at 16:00
	if got := Today(s, now); got != "Today: 1.5M tokens · window resets 21:00" {
		t.Fatalf("got %q", got)
	}
	if got := Today(s, now.Add(48*time.Hour)); got != "" {
		t.Fatalf("a day without usage: %q", got)
	}
}
