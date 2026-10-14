//go:build windows || darwin

// Package traymenu builds the menu items the Windows tray and the macOS menu bar share, so both
// show the same thing and the code is checked wherever either one builds.
package traymenu

import (
	"strings"

	"fyne.io/systray"
	"github.com/baodq97/hatrack/internal/mcp"
)

// OnClick runs fn each time item is clicked, until the menu is rebuilt; each tray brings its own,
// which also shows fn's error.
type OnClick func(item *systray.MenuItem, fn func() error)

// MCPState is what the MCP menu shows: user-scope servers and sets. Compare it with fmt.Sprint to
// tell when the menu needs rebuilding.
type MCPState struct {
	Servers []mcp.Server
	Sets    []mcp.Set
}

// LoadMCP reads the user-scope servers and sets. A menu has no folder, so local and project
// servers are left to `hat mcp`.
func LoadMCP(c *mcp.Config) (MCPState, error) {
	var st MCPState
	if c == nil {
		return st, nil
	}
	all, err := c.List()
	if err != nil {
		return st, err
	}
	for _, s := range all {
		if s.Scope == mcp.User {
			st.Servers = append(st.Servers, s)
		}
	}
	st.Sets, err = c.Sets()
	return st, err
}

// AddMCP adds an "MCP servers" submenu: a checkbox per server to switch it on and off, then the sets.
func AddMCP(c *mcp.Config, st MCPState, on OnClick) {
	menu := systray.AddMenuItem("MCP servers", "Switch Claude's user-scope MCP servers on and off")
	if c == nil {
		menu.Disable()
		return
	}
	if len(st.Servers) == 0 {
		menu.AddSubMenuItem("None yet: see `hat mcp catalog`", "").Disable()
	}
	for _, s := range st.Servers {
		item := menu.AddSubMenuItemCheckbox(s.Name, s.Spec.Redacted().Target(), s.Enabled)
		on(item, func() error {
			if s.Enabled {
				return c.Disable(s.Name, mcp.User)
			}
			return c.Enable(s.Name, mcp.User)
		})
	}
	if len(st.Sets) == 0 {
		return
	}
	menu.AddSeparator()
	for _, set := range st.Sets {
		tip := strings.Join(set.Servers, ", ")
		if set.Account != "" {
			tip += " (with " + set.Account + ")"
		}
		on(menu.AddSubMenuItem("Set: "+set.Name, tip), func() error {
			_, err := c.UseSet(set.Name)
			return err
		})
	}
}

// AddUsage adds today's usage as a line that cannot be clicked, if there is any.
func AddUsage(line string) {
	if line == "" {
		return
	}
	systray.AddMenuItem(line, "From Claude Code's transcripts on this computer; `hat usage` has more").Disable()
}
