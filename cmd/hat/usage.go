package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/baodq97/hatrack/internal/claude"
	"github.com/baodq97/hatrack/internal/usage"
)

const usageUsage = `hat usage - tokens Claude Code used, read from its transcripts on this machine

  hat usage                  last 7 days, by day, and the current 5-hour window
  hat usage --days 30        a longer stretch
  hat usage --by project     by day, project, model, account or session

Accounts are known from the moment hat first saved or switched to them.`

const untracked = "(before hat)"

func runUsage(s *claude.Store, args []string) error {
	fs := flag.NewFlagSet("hat usage", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	days := fs.Int("days", 7, "how many days back, today included")
	by := fs.String("by", "day", "day, project, model, account or session")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, usageUsage)
	}
	key, ok := map[string]func(usage.Entry) string{
		"day":     func(e usage.Entry) string { return e.Time.Local().Format("2006-01-02 Mon") },
		"project": func(e usage.Entry) string { return tildePath(e.Project) },
		"model":   func(e usage.Entry) string { return e.Model },
		"account": func(e usage.Entry) string { return e.Account },
		"session": func(e usage.Entry) string { return e.Session },
	}[*by]
	if !ok || *days < 1 {
		return fmt.Errorf("bad flags\n\n%s", usageUsage)
	}
	now := time.Now()
	y, m, d := now.Date()
	since := time.Date(y, m, d-(*days-1), 0, 0, 0, 0, time.Local)
	// scan from a window earlier, so the current window's start is known
	es, err := usage.Scan(filepath.Join(s.ConfigDir, "projects"), minTime(since, now.Add(-2*usage.BlockLen)))
	if err != nil {
		return err
	}
	hist, err := s.Switches()
	if err != nil {
		return err
	}
	for i := range es {
		if es[i].Account = claude.AccountAt(hist, es[i].Time); es[i].Account == "" {
			es[i].Account = untracked
		}
	}
	var inRange []usage.Entry
	for _, e := range es {
		if !e.Time.Before(since) {
			inRange = append(inRange, e)
		}
	}
	if len(inRange) == 0 {
		fmt.Printf("no Claude Code usage since %s\n", since.Format("2006-01-02"))
		return nil
	}
	rows := usage.Group(inRange, key)
	if *by != "day" {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Total() > rows[j].Total() })
	}
	rows = append(rows, total(inRange))
	width := len(*by)
	for _, r := range rows {
		width = max(width, len([]rune(shorten(r.Key, 48))))
	}
	// numbers align right; the first column is padded so it reads left-aligned
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(w, "%-*s\tRESPONSES\tINPUT\tOUTPUT\tCACHE WRITE\tCACHE READ\tTOTAL\t\n", width, strings.ToUpper(*by))
	for _, r := range rows {
		fmt.Fprintf(w, "%-*s\t%d\t%s\t%s\t%s\t%s\t%s\t\n", width, shorten(r.Key, 48), r.Responses, n(r.Input), n(r.Output), n(r.CacheWrite), n(r.CacheRead), n(r.Total()))
	}
	w.Flush()

	// the window subscription limits are counted in, for each account that has an open one
	byAcct := map[string][]usage.Entry{}
	for _, e := range es {
		byAcct[e.Account] = append(byAcct[e.Account], e)
	}
	fmt.Println()
	open := false
	for _, acct := range sortedAccounts(byAcct) {
		bs := usage.Blocks(byAcct[acct])
		b := bs[len(bs)-1]
		if !b.Active(now) {
			continue
		}
		open = true
		fmt.Printf("5-hour window %s–%s (ends in %s) for %s: %s tokens, %s output, %s\n",
			b.Start.Local().Format("15:04"), b.End.Local().Format("15:04"), strings.TrimSuffix(b.End.Sub(now).Round(time.Minute).String(), "0s"),
			acct, n(b.Total()), n(b.Output), plural(b.Responses, "response"))
	}
	if !open {
		fmt.Println("No 5-hour usage window open now.")
	}
	fmt.Println("Counted from transcripts on this machine only. Anthropic's limits weigh tokens in ways that are not public.")
	return nil
}

func total(es []usage.Entry) usage.Row {
	r := usage.Sum(es)
	r.Key = "TOTAL"
	return r
}

func sortedAccounts(m map[string][]usage.Entry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// n formats a token count as 950, 12.3K or 4.56M.
func n(v int64) string {
	switch {
	case v >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(v)/1e9)
	case v >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(v)/1e6)
	case v >= 10_000:
		return fmt.Sprintf("%.1fK", float64(v)/1e3)
	}
	return fmt.Sprint(v)
}

func tildePath(p string) string {
	if p == "" {
		return "(unknown)"
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
