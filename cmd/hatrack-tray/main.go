//go:build windows

// Command hatrack-tray puts hat in the Windows tray: click an account to switch to it.
package main

import (
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unsafe"

	"fyne.io/systray"
	"github.com/baodq97/hatrack/internal/claude"
	"github.com/baodq97/hatrack/internal/mcp"
	"github.com/baodq97/hatrack/internal/usage"
	"golang.org/x/sys/windows"
)

//go:embed icon.ico
var icon []byte

func main() {
	// a windowsgui app has no console: keep crashes and systray errors in ~/.hatrack/tray.log
	if home, err := os.UserHomeDir(); err == nil {
		os.MkdirAll(filepath.Join(home, ".hatrack"), 0o700)
		if f, err := os.OpenFile(filepath.Join(home, ".hatrack", "tray.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			log.SetOutput(f)
			debug.SetCrashOutput(f, debug.CrashOptions{})
		}
	}
	systray.Run(onReady, nil)
}

func onReady() {
	systray.SetIcon(icon)
	s, err := claude.Default()
	m, merr := mcp.Default("") // the tray has no folder: user-scope servers only
	t := &tray{s: s, m: m, err: errors.Join(err, merr)}
	t.render()
	go func() { // pick up changes made by `hat` or by signing in from Claude itself
		for range time.Tick(3 * time.Second) {
			t.render()
		}
	}()
}

type tray struct {
	mu sync.Mutex
	s  *claude.Store
	m  *mcp.Config

	usage   string // today's usage line, refreshed once a minute
	usageAt time.Time
	err     error // last failed action, shown at the top of the menu
	last    string
	done    chan struct{}
}

// render rebuilds the menu only when something changed, so an open menu is not torn down every tick.
func (t *tray) render() {
	t.mu.Lock()
	defer t.mu.Unlock()
	var ps []claude.Profile
	err := t.err
	if t.s != nil {
		var lerr error
		if ps, lerr = t.s.List(); err == nil {
			err = lerr
		}
	}
	var servers []mcp.Server
	var sets []mcp.Set
	if t.m != nil && err == nil {
		servers, err = t.m.List()
		if err == nil {
			sets, err = t.m.Sets()
		}
	}
	if time.Since(t.usageAt) > time.Minute && t.s != nil {
		t.usage, t.usageAt = usageLine(t.s), time.Now()
	}
	key := fmt.Sprint(ps, err, servers, sets, t.usage)
	if key == t.last {
		return
	}
	t.last = key
	if t.done != nil {
		close(t.done)
	}
	t.done = make(chan struct{})
	systray.ResetMenu()

	if err != nil {
		systray.AddMenuItem("⚠ "+err.Error(), "").Disable()
		systray.AddSeparator()
	}
	tip := "hatrack: no saved account active"
	if len(ps) == 0 {
		systray.AddMenuItem("No accounts yet", "").Disable()
	}
	for _, p := range ps {
		if p.Active {
			tip = "hatrack: " + p.Name
		}
		t.on(systray.AddMenuItemCheckbox(label(p), p.Email, p.Active), func() error { return t.use(p.Name) })
	}
	systray.SetTooltip(tip)
	if t.usage != "" {
		systray.AddSeparator()
		systray.AddMenuItem(t.usage, "From Claude Code's transcripts on this PC; `hat usage` has more").Disable()
	}
	systray.AddSeparator()
	t.on(systray.AddMenuItem("Add account…", "Sign in another account in a console window"), t.openAdd)
	t.on(systray.AddMenuItem("Save current account", "Keep the account Claude is signed in to now"), func() error {
		_, err := t.s.Save("")
		return err
	})
	if len(ps) > 0 {
		rm := systray.AddMenuItem("Remove", "")
		for _, p := range ps {
			t.on(rm.AddSubMenuItem(p.Name, ""), func() error { return t.s.Remove(p.Name) })
		}
	}
	systray.AddSeparator()
	t.mcpMenu(servers, sets)
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "")
	go func() {
		if _, ok := <-quit.ClickedCh; ok { // ResetMenu closes the channel; that is not a click
			systray.Quit()
		}
	}()
}

// usageLine sums today's tokens and the open 5-hour window of the account in use.
func usageLine(s *claude.Store) string {
	now := time.Now()
	y, mo, d := now.Date()
	today := time.Date(y, mo, d, 0, 0, 0, 0, time.Local)
	es, err := usage.Scan(filepath.Join(s.ConfigDir, "projects"), today.Add(-usage.BlockLen))
	if err != nil || len(es) == 0 {
		return ""
	}
	hist, _ := s.Switches()
	cur := claude.AccountAt(hist, now)
	var day, mine []usage.Entry
	for _, e := range es {
		if !e.Time.Before(today) {
			day = append(day, e)
		}
		if claude.AccountAt(hist, e.Time) == cur {
			mine = append(mine, e)
		}
	}
	line := "Today: " + tokens(usage.Sum(day).Total()) + " tokens"
	if bs := usage.Blocks(mine); len(bs) > 0 && bs[len(bs)-1].Active(now) {
		line += " · window resets " + bs[len(bs)-1].End.Local().Format("15:04")
	}
	return line
}

func tokens(v int64) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(v)/1e6)
	case v >= 1_000:
		return fmt.Sprintf("%.0fK", float64(v)/1e3)
	}
	return fmt.Sprint(v)
}

// use switches account, then to the MCP set linked to it, if any.
func (t *tray) use(name string) error {
	_, _, err := mcp.SwitchAccount(t.s, name)
	return err
}

// mcpMenu lists user-scope MCP servers to switch on and off, and the sets to switch to.
func (t *tray) mcpMenu(servers []mcp.Server, sets []mcp.Set) {
	menu := systray.AddMenuItem("MCP servers", "Switch Claude's user-scope MCP servers on and off")
	if len(servers) == 0 {
		menu.AddSubMenuItem("None yet: see `hat mcp catalog`", "").Disable()
	}
	for _, s := range servers {
		if s.Scope != mcp.User {
			continue
		}
		item := menu.AddSubMenuItemCheckbox(s.Name, s.Spec.Redacted().Target(), s.Enabled)
		t.on(item, func() error {
			if s.Enabled {
				return t.m.Disable(s.Name, mcp.User)
			}
			return t.m.Enable(s.Name, mcp.User)
		})
	}
	if len(sets) == 0 {
		return
	}
	menu.AddSeparator()
	for _, set := range sets {
		tip := strings.Join(set.Servers, ", ")
		if set.Account != "" {
			tip += " (with " + set.Account + ")"
		}
		t.on(menu.AddSubMenuItem("Set: "+set.Name, tip), func() error {
			_, err := t.m.UseSet(set.Name)
			return err
		})
	}
}

// on runs fn for each click until the menu is rebuilt, then shows its error, if any.
func (t *tray) on(item *systray.MenuItem, fn func() error) {
	if t.s == nil {
		item.Disable()
		return
	}
	done := t.done
	go func() {
		for {
			select {
			case _, ok := <-item.ClickedCh:
				if !ok { // closed by ResetMenu, not clicked
					return
				}
				err := fn()
				t.mu.Lock()
				t.err = err
				t.mu.Unlock()
				go t.render() // this goroutine ends when render closes done
			case <-done:
				return
			}
		}
	}()
}

// openAdd runs `hat add` in a console kept open, so the sign-in prompt and its result stay readable.
// It calls CreateProcess itself: os/exec always hands the child std handles, NUL for a GUI
// parent, which leaves the new console blank and makes cmd exit at once.
func (t *tray) openAdd() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// cmd keeps the quotes around a lone executable path, spaces and all
	line, err := windows.UTF16PtrFromString(`cmd.exe /k "` + filepath.Join(filepath.Dir(exe), "hat.exe") + `" add`)
	if err != nil {
		return err
	}
	si := &windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(nil, line, nil, nil, false, windows.CREATE_NEW_CONSOLE, nil, nil, si, &pi); err != nil {
		return err
	}
	windows.CloseHandle(pi.Thread)
	windows.CloseHandle(pi.Process)
	return nil
}

func label(p claude.Profile) string {
	if p.Org == "" || p.Name == p.Org {
		return p.Name
	}
	return p.Name + "  (" + p.Org + ")"
}
