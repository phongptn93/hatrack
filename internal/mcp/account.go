package mcp

import (
	"github.com/baodq97/hatrack/internal/claude"
)

// SwitchAccount makes name the account Claude uses, then uses the MCP set linked to it, if any.
// Every way of switching (CLI, tray, menu bar, dashboard) goes through here. set is the linked
// set's name and changed what it switched, as UseSet reports it.
func SwitchAccount(s *claude.Store, name string) (set string, changed []string, err error) {
	if err := s.Use(name); err != nil {
		return "", nil, err
	}
	c, err := Default("")
	if err != nil {
		return "", nil, err
	}
	linked, ok, err := c.SetFor(name)
	if err != nil || !ok {
		return "", nil, err
	}
	changed, err = c.UseSet(linked.Name)
	return linked.Name, changed, err
}

// RenameAccount renames a saved account and moves its MCP set link along with it.
func RenameAccount(s *claude.Store, oldName, newName string) error {
	if err := s.Rename(oldName, newName); err != nil {
		return err
	}
	c, err := Default("")
	if err != nil {
		return err
	}
	if set, ok, err := c.SetFor(oldName); err != nil || !ok {
		return err
	} else {
		return c.LinkSet(set.Name, newName)
	}
}
