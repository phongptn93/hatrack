package usage

import (
	"fmt"
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

// TodayCache recomputes Today at most once a minute, for menus redrawn every few seconds.
type TodayCache struct {
	mu   sync.Mutex
	line string
	at   time.Time
}

func (c *TodayCache) Line(s *claude.Store) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s != nil && time.Since(c.at) > time.Minute {
		c.line, c.at = Today(s, time.Now()), time.Now()
	}
	return c.line
}
