//go:build windows

// Command hatrack-tray puts hat in the Windows tray: click an account to switch to it.
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
	"unsafe"

	"fyne.io/systray"
	"github.com/baodq97/hatrack/internal/claude"
	"github.com/baodq97/hatrack/internal/mcp"
	"github.com/baodq97/hatrack/internal/traymenu"
	"github.com/baodq97/hatrack/internal/update"
	"github.com/baodq97/hatrack/internal/usage"
	"golang.org/x/sys/windows"
)

//go:embed icon.ico
var icon []byte

var version = "dev" // set by the release build

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
	exe, xerr := os.Executable()
	started, serr := os.Stat(exe)
	go func() { // pick up changes made by `hat` or by signing in from Claude itself
		for range time.Tick(3 * time.Second) {
			if xerr == nil && serr == nil && replaced(exe, started) {
				restart(exe)
				return
			}
			t.render()
		}
	}()
	go t.checkUpdates()
}

// replaced reports whether exe is no longer the file this tray started from: an update put a
// new one in its place.
func replaced(exe string, started os.FileInfo) bool {
	fi, err := os.Stat(exe)
	return err == nil && !os.SameFile(fi, started)
}

// restart starts the new tray and quits this one.
func restart(exe string) {
	if err := exec.Command(exe).Start(); err != nil {
		log.Println("restart after update:", err)
		return
	}
	systray.Quit()
}

// checkUpdates asks GitHub for the latest release once a day; the menu offers it when newer.
func (t *tray) checkUpdates() {
	home, err := os.UserHomeDir()
	if err != nil || update.Off() {
		return
	}
	cache := filepath.Join(home, ".hatrack", "update.json")
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		latest, err := update.Cached(ctx, cache, 24*time.Hour)
		cancel()
		if err == nil && update.Newer(version, latest) {
			t.mu.Lock()
			t.latest = latest
			t.mu.Unlock()
			t.render()
		}
		time.Sleep(time.Hour)
	}
}

// install puts the latest release in place of hat.exe and this tray; the tick then restarts it.
func (t *tray) install() error {
	t.mu.Lock()
	latest := t.latest
	t.mu.Unlock()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return update.Install(ctx, latest, map[string]string{
		exe: update.Asset("hatrack-tray", "windows", runtime.GOARCH),
		filepath.Join(filepath.Dir(exe), "hat.exe"): update.Asset("hat", "windows", runtime.GOARCH),
	})
}

type tray struct {
	mu sync.Mutex
	s  *claude.Store
	m  *mcp.Config

	usage  usage.MenuCache
	err    error  // last failed action, shown at the top of the menu
	latest string // a newer release, once one is out
	last   string
	done   chan struct{}
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
	var mst traymenu.MCPState
	if err == nil {
		mst, err = traymenu.LoadMCP(t.m)
	}
	today, warn, _ := t.usage.Line(t.s)
	key := fmt.Sprint(ps, err, mst, today, warn, t.latest)
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
	if t.latest != "" {
		t.on(systray.AddMenuItem("⬆ Update to "+t.latest, "Download it from GitHub; the tray restarts on its own"), t.install)
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
	if warn {
		tip = "⚠ " + today + "\n" + tip
	}
	if t.latest != "" {
		tip += "\n⬆ " + t.latest + " is out"
	}
	systray.SetTooltip(tip)
	if today != "" {
		systray.AddSeparator()
		traymenu.AddUsage(today)
	}
	systray.AddSeparator()
	t.on(systray.AddMenuItem("Add account…", "Sign in another account in a console window"), func() error { return openHat("add") })
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
	traymenu.AddMCP(t.m, mst, t.on, func() error { return openHat("mcp add") })
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "")
	go func() {
		if _, ok := <-quit.ClickedCh; ok { // ResetMenu closes the channel; that is not a click
			systray.Quit()
		}
	}()
}

// use switches account, then to the MCP set linked to it, if any.
func (t *tray) use(name string) error {
	_, _, err := mcp.SwitchAccount(t.s, name)
	return err
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

// openHat runs `hat <args>` in a console kept open, so its prompts and result stay readable.
// It calls CreateProcess itself: os/exec always hands the child std handles, NUL for a GUI
// parent, which leaves the new console blank and makes cmd exit at once.
func openHat(args string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// cmd keeps the quotes around a lone executable path, spaces and all
	line, err := windows.UTF16PtrFromString(`cmd.exe /k "` + filepath.Join(filepath.Dir(exe), "hat.exe") + `" ` + args)
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
