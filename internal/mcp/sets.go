package mcp

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
)

// Set is a named group of user-scope servers, such as "work" (Azure DevOps, SQL Server) or
// "personal" (GitHub). Using a set switches its servers on and every other user server off.
// A set linked to an account is used whenever hat switches to that account.
type Set struct {
	Name    string   `json:"-"`
	Servers []string `json:"servers"`
	Account string   `json:"account,omitempty"`
}

func (c *Config) setsPath() string { return filepath.Join(c.Dir, "sets.json") }

func (c *Config) readSets() (map[string]Set, error) {
	m := map[string]Set{}
	if err := readJSON(c.setsPath(), &m); err != nil {
		return nil, err
	}
	for k, v := range m {
		v.Name = k
		m[k] = v
	}
	return m, nil
}

func (c *Config) editSets(fn func(map[string]Set) error) error {
	m, err := c.readSets()
	if err != nil {
		return err
	}
	if err := fn(m); err != nil {
		return err
	}
	return writeJSON(c.setsPath(), m)
}

func (c *Config) Sets() ([]Set, error) {
	m, err := c.readSets()
	if err != nil {
		return nil, err
	}
	out := make([]Set, 0, len(m))
	for _, s := range m {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// SaveSet stores a set. No servers means the user-scope servers switched on now.
func (c *Config) SaveSet(name string, servers []string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("bad set name %q", name)
	}
	user, err := c.userServers()
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		for _, s := range user {
			if s.Enabled {
				servers = append(servers, s.Name)
			}
		}
	}
	for _, n := range servers {
		if !slices.ContainsFunc(user, func(s Server) bool { return s.Name == n }) {
			return fmt.Errorf("no user-scope MCP server %q: sets hold user-scope servers only", n)
		}
	}
	slices.Sort(servers)
	servers = slices.Compact(servers)
	return c.editSets(func(m map[string]Set) error {
		s := m[name]
		s.Servers = servers
		if s.Servers == nil {
			s.Servers = []string{}
		}
		m[name] = s
		return nil
	})
}

func (c *Config) RemoveSet(name string) error {
	return c.editSets(func(m map[string]Set) error {
		if _, ok := m[name]; !ok {
			return fmt.Errorf("no set %q", name)
		}
		delete(m, name)
		return nil
	})
}

// LinkSet makes hat use the set when it switches to account. An empty account unlinks it.
// An account links to one set at most.
func (c *Config) LinkSet(name, account string) error {
	return c.editSets(func(m map[string]Set) error {
		s, ok := m[name]
		if !ok {
			return fmt.Errorf("no set %q", name)
		}
		if account != "" {
			for k, o := range m {
				if o.Account == account {
					o.Account = ""
					m[k] = o
				}
			}
		}
		s.Account = account
		m[name] = s
		return nil
	})
}

// SetFor returns the set linked to account.
func (c *Config) SetFor(account string) (Set, bool, error) {
	m, err := c.readSets()
	if err != nil {
		return Set{}, false, err
	}
	for _, s := range m {
		if s.Account != "" && s.Account == account {
			return s, true, nil
		}
	}
	return Set{}, false, nil
}

// UseSet switches the set's servers on and every other user-scope server off, and returns what
// it changed as "+name" and "-name".
func (c *Config) UseSet(name string) ([]string, error) {
	m, err := c.readSets()
	if err != nil {
		return nil, err
	}
	set, ok := m[name]
	if !ok {
		return nil, fmt.Errorf("no set %q", name)
	}
	user, err := c.userServers()
	if err != nil {
		return nil, err
	}
	for _, n := range set.Servers {
		if !slices.ContainsFunc(user, func(s Server) bool { return s.Name == n }) {
			return nil, fmt.Errorf("set %q needs MCP server %q, which is gone; edit the set with `hat mcp set save`", name, n)
		}
	}
	var changed []string
	for _, s := range user {
		want := slices.Contains(set.Servers, s.Name)
		switch {
		case want && !s.Enabled:
			if err := c.Enable(s.Name, User); err != nil {
				return changed, err
			}
			changed = append(changed, "+"+s.Name)
		case !want && s.Enabled:
			if err := c.Disable(s.Name, User); err != nil {
				return changed, err
			}
			changed = append(changed, "-"+s.Name)
		}
	}
	return changed, nil
}

func (c *Config) userServers() ([]Server, error) {
	all, err := c.List()
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(s Server) bool { return s.Scope != User }), nil
}
