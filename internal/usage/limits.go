package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/baodq97/hatrack/internal/atomicfile"
)

// Claude Code hands its status line the share of the subscription limits used so far, as the API
// reports it. `hat statusline` records it here per account, so hat can show it and warn before a
// limit is reached: ~/.hatrack/limits.json.

// Window is one limit as Claude Code reports it.
type Window struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"` // Unix seconds
}

func (w *Window) live(now time.Time) bool { return w != nil && time.Unix(w.ResetsAt, 0).After(now) }

// Resets is when the window resets.
func (w *Window) Resets() time.Time { return time.Unix(w.ResetsAt, 0) }

// Snapshot is the latest report for one account.
type Snapshot struct {
	Updated  time.Time `json:"updated"`
	FiveHour *Window   `json:"five_hour,omitempty"`
	SevenDay *Window   `json:"seven_day,omitempty"`
}

// Live drops windows that have reset since the report, which no longer say anything.
func (s Snapshot) Live(now time.Time) Snapshot {
	if !s.FiveHour.live(now) {
		s.FiveHour = nil
	}
	if !s.SevenDay.live(now) {
		s.SevenDay = nil
	}
	return s
}

// Alert holds the percentages at which hat warns. Zero means the default; Off silences it.
type Alert struct {
	FiveHour float64 `json:"five_hour,omitempty"`
	SevenDay float64 `json:"seven_day,omitempty"`
	Off      bool    `json:"off,omitempty"`
}

const (
	DefaultFiveHour = 80
	DefaultSevenDay = 90
)

func (a Alert) fiveHour() float64 { return orDefault(a.FiveHour, DefaultFiveHour) }
func (a Alert) sevenDay() float64 { return orDefault(a.SevenDay, DefaultSevenDay) }

func orDefault(v, d float64) float64 {
	if v <= 0 {
		return d
	}
	return v
}

// Limits is ~/.hatrack/limits.json.
type Limits struct {
	Alert    Alert               `json:"alert"`
	Accounts map[string]Snapshot `json:"accounts,omitempty"`
	Notified map[string]bool     `json:"notified,omitempty"` // warnings already shown, by Warning.Key
}

// Warning is a limit past its alert percentage.
type Warning struct {
	Account string
	Kind    string // "5-hour" or "weekly"
	Percent float64
	Resets  time.Time
}

// Key names the warning once per account, limit and window, so it is announced once.
func (w Warning) Key() string { return fmt.Sprintf("%s|%s|%d", w.Account, w.Kind, w.Resets.Unix()) }

func (w Warning) String() string {
	return fmt.Sprintf("%s: %.0f%% of the %s limit used; resets %s", w.Account, w.Percent, w.Kind, when(w.Resets))
}

func limitsPath(dir string) string { return filepath.Join(dir, "limits.json") }

// LoadLimits reads dir/limits.json; a missing file is empty.
func LoadLimits(dir string) (*Limits, error) {
	l := &Limits{}
	return l, readJSONFile(limitsPath(dir), l)
}

// EditLimits changes dir/limits.json.
func EditLimits(dir string, fn func(*Limits) error) error {
	return atomicfile.Edit(limitsPath(dir), "", 0, func(b []byte) ([]byte, error) {
		l := &Limits{}
		if len(strings.TrimSpace(string(b))) > 0 {
			if err := json.Unmarshal(b, l); err != nil {
				return nil, err
			}
		}
		if err := fn(l); err != nil {
			return nil, err
		}
		return atomicfile.JSON(l)
	})
}

// Warnings lists the account's limits past their alert, from its latest report.
func (l *Limits) Warnings(account string, now time.Time) []Warning {
	if l.Alert.Off {
		return nil
	}
	s := l.Accounts[account].Live(now)
	var out []Warning
	if w := s.FiveHour; w != nil && w.UsedPercentage >= l.Alert.fiveHour() {
		out = append(out, Warning{account, "5-hour", w.UsedPercentage, w.Resets()})
	}
	if w := s.SevenDay; w != nil && w.UsedPercentage >= l.Alert.sevenDay() {
		out = append(out, Warning{account, "weekly", w.UsedPercentage, w.Resets()})
	}
	return out
}

// Record stores a status line report for account and returns the warnings it raises that were not
// announced before, marking them announced. in is the JSON Claude Code passes its status line.
func Record(dir, account string, in []byte, now time.Time) (Snapshot, []Warning, error) {
	var payload struct {
		RateLimits *struct {
			FiveHour *Window `json:"five_hour"`
			SevenDay *Window `json:"seven_day"`
		} `json:"rate_limits"`
	}
	if err := json.Unmarshal(in, &payload); err != nil {
		return Snapshot{}, nil, err
	}
	if payload.RateLimits == nil || account == "" {
		return Snapshot{}, nil, nil // not a subscriber, or before the first response
	}
	snap := Snapshot{Updated: now.UTC(), FiveHour: payload.RateLimits.FiveHour, SevenDay: payload.RateLimits.SevenDay}
	var fresh []Warning
	err := EditLimits(dir, func(l *Limits) error {
		if l.Accounts == nil {
			l.Accounts = map[string]Snapshot{}
		}
		l.Accounts[account] = snap
		if l.Notified == nil {
			l.Notified = map[string]bool{}
		}
		for k := range l.Notified { // forget windows that have reset
			var ts int64
			if i := strings.LastIndex(k, "|"); i >= 0 {
				fmt.Sscan(k[i+1:], &ts)
			}
			if time.Unix(ts, 0).Before(now) {
				delete(l.Notified, k)
			}
		}
		for _, w := range l.Warnings(account, now) {
			if !l.Notified[w.Key()] {
				l.Notified[w.Key()] = true
				fresh = append(fresh, w)
			}
		}
		return nil
	})
	return snap, fresh, err
}

// Line sums a report up as "5h 62% ↻21:00 · 7d 31%", with ⚠ on a limit past its alert.
func (l *Limits) Line(account string, now time.Time) (string, bool) {
	s := l.Accounts[account].Live(now)
	var parts []string
	warn := false
	part := func(label string, w *Window, alert float64) {
		if w == nil {
			return
		}
		p := fmt.Sprintf("%s %.0f%%", label, math.Floor(w.UsedPercentage))
		if !l.Alert.Off && w.UsedPercentage >= alert {
			p, warn = "⚠ "+p, true
		}
		parts = append(parts, p+" ↻"+when(w.Resets()))
	}
	part("5h", s.FiveHour, l.Alert.fiveHour())
	part("7d", s.SevenDay, l.Alert.sevenDay())
	return strings.Join(parts, " · "), warn
}

// when is a reset time: the clock today, else the weekday too.
func when(t time.Time) string {
	t = t.Local()
	if y, m, d := t.Date(); y == time.Now().Year() && m == time.Now().Month() && d == time.Now().Day() {
		return t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}

func readJSONFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(strings.TrimSpace(string(b))) == 0) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
