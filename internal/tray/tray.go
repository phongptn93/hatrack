//go:build darwin

// Package tray is the macOS menu bar for hatrack.
package tray

import (
	_ "embed"
	"fmt"
	"os"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/baodq97/hatrack/internal/claude"
	"github.com/baodq97/hatrack/internal/mcp"
	"github.com/baodq97/hatrack/internal/traymenu"
	"github.com/baodq97/hatrack/internal/usage"
)

//go:embed icon.ico
var icon []byte

type Tray struct {
	mu           sync.Mutex
	s            *claude.Store
	m            *mcp.Config // user-scope MCP servers; nil if their config could not be found
	usage        usage.MenuCache
	err          error
	last         string
	done         chan struct{}
	onOpenWindow func()
	onAdd        func() error
}

// Start runs the menu bar on the calling goroutine, which must be the main one.
func Start(s *claude.Store, onOpenWindow func(), onAdd func() error) {
	systray.Run(func() {
		systray.SetIcon(icon)
		systray.SetTitle("")
		systray.SetTooltip("Hatrack - Claude Account Switcher")

		m, merr := mcp.Default("") // a menu has no folder: user-scope servers only
		t := &Tray{s: s, m: m, err: merr, onOpenWindow: onOpenWindow, onAdd: onAdd}
		t.render()

		go func() {
			for range time.Tick(3 * time.Second) {
				t.render()
			}
		}()
	}, func() {})
}

func (t *Tray) render() {
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
	today, warn, title := t.usage.Line(t.s)
	key := fmt.Sprint(ps, err, mst, today, warn)
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

	tip := "Hatrack: No active account"
	if len(ps) == 0 {
		systray.AddMenuItem("No saved accounts yet", "").Disable()
	}

	for _, p := range ps {
		if p.Active {
			tip = "Hatrack: " + p.Name
		}
		title := label(p)
		item := systray.AddMenuItemCheckbox(title, p.Email, p.Active)
		profileName := p.Name
		t.on(item, func() error {
			_, _, err := mcp.SwitchAccount(t.s, profileName) // with its linked MCP set
			return err
		})
	}

	if warn {
		tip = title + " · " + tip
	}
	systray.SetTooltip(tip)
	systray.SetTitle(title) // "⚠ 86%" next to the hat while a limit is past its alert
	if today != "" {
		systray.AddSeparator()
		traymenu.AddUsage(today)
	}
	systray.AddSeparator()

	if t.onOpenWindow != nil {
		mOpen := systray.AddMenuItem("🖥 Open Full Dashboard", "Open main application window")
		t.on(mOpen, func() error {
			t.onOpenWindow()
			return nil
		})
	}

	if t.onAdd != nil {
		t.on(systray.AddMenuItem("Add Account…", "Sign in another account in Terminal"), t.onAdd)
	}

	mSave := systray.AddMenuItem("💾 Save Current Account", "Keep account Claude is signed in to now")
	t.on(mSave, func() error {
		_, err := t.s.Save("")
		return err
	})

	if len(ps) > 0 {
		mRemove := systray.AddMenuItem("Remove", "")
		for _, p := range ps {
			pName := p.Name
			t.on(mRemove.AddSubMenuItem(pName, ""), func() error {
				return t.s.Remove(pName)
			})
		}
	}

	systray.AddSeparator()
	traymenu.AddMCP(t.m, mst, t.on)

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit Hatrack", "Exit application")
	go func() {
		if _, ok := <-mQuit.ClickedCh; ok {
			systray.Quit()
			os.Exit(0)
		}
	}()
}

func (t *Tray) on(item *systray.MenuItem, fn func() error) {
	if t.s == nil {
		item.Disable()
		return
	}
	done := t.done
	go func() {
		for {
			select {
			case _, ok := <-item.ClickedCh:
				if !ok {
					return
				}
				err := fn()
				t.mu.Lock()
				t.err = err
				t.mu.Unlock()
				go t.render()
			case <-done:
				return
			}
		}
	}()
}

func label(p claude.Profile) string {
	prefix := "  "
	if p.Active {
		prefix = "✓ "
	}
	if p.Org == "" || p.Name == p.Org {
		return prefix + p.Name
	}
	return prefix + p.Name + " (" + p.Org + ")"
}
