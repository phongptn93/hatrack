package usage

import (
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"time"

	"github.com/baodq97/hatrack/internal/claude"
)

// Today sums today's tokens and the open 5-hour window of the account in use, as one line
// for a menu: "Today: 15.5M tokens · window resets 21:00". It is empty when nothing was used.
func Today(s *claude.Store, now time.Time) string {
	y, mo, d := now.Date()
	today := time.Date(y, mo, d, 0, 0, 0, 0, now.Location())
	es, err := Scan(filepath.Join(s.ConfigDir, "projects"), today.Add(-BlockLen))
	if err != nil || len(es) == 0 {
		return ""
	}
	hist, _ := s.Switches()
	cur := claude.AccountAt(hist, now)
	var day, mine []Entry
	for _, e := range es {
		if !e.Time.Before(today) {
			day = append(day, e)
		}
		if claude.AccountAt(hist, e.Time) == cur {
			mine = append(mine, e)
		}
	}
	if len(day) == 0 {
		return ""
	}
	line := "Today: " + Short(Sum(day).Total()) + " tokens"
	if bs := Blocks(mine); len(bs) > 0 && bs[len(bs)-1].Active(now) {
		line += " · window resets " + bs[len(bs)-1].End.In(now.Location()).Format("15:04")
	}
	return line
}

// Short formats a token count for a menu: 950, 12K, 15.5M.
func Short(v int64) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(v)/1e6)
	case v >= 1_000:
		return fmt.Sprintf("%.0fK", float64(v)/1e3)
	}
	return fmt.Sprint(v)
}

// ActiveAccount names the account Claude uses now the way hat records it: its saved name, else
// its email.
func ActiveAccount(s *claude.Store) string {
	cur, err := s.Current()
	if err != nil {
		return ""
	}
	if cur.Name != "" {
		return cur.Name
	}
	return cur.Email
}

// MenuCache builds the usage line of the tray and the menu bar: the share of the limits used, as
// Claude Code last reported it, else today's tokens. Tokens are recounted once a minute; the
// limits, a small file, on every call.
type MenuCache struct {
	mu   sync.Mutex
	line string
	at   time.Time
}

// Line returns the line and whether a limit is past its alert, then title is its short form
// for the macOS menu bar ("⚠ 86%").
func (c *MenuCache) Line(s *claude.Store) (line string, warn bool, title string) {
	if s == nil {
		return "", false, ""
	}
	now := time.Now()
	if l, err := LoadLimits(filepath.Dir(s.Dir)); err == nil {
		acct := ActiveAccount(s)
		if line, warn := l.Line(acct, now); line != "" {
			if warn {
				max := 0.0
				for _, w := range l.Warnings(acct, now) {
					max = math.Max(max, w.Percent)
				}
				title = fmt.Sprintf("⚠ %.0f%%", math.Floor(max))
			}
			return "Limits: " + line, warn, title
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.at) > time.Minute {
		c.line, c.at = Today(s, now), now
	}
	return c.line, false, ""
}
