// Package mcp manages the MCP servers Claude Code loads: list them across scopes, add them from
// a catalog, switch them off and on without losing their settings, group them into sets, and
// check that they start.
//
// Claude keeps MCP servers in three places, and this package edits them in place:
//   - user scope: mcpServers in ~/.claude.json, loaded in every folder
//   - local scope: projects[<folder>].mcpServers in ~/.claude.json, one folder, only you
//   - project scope: mcpServers in <folder>/.mcp.json, one folder, checked in for the team
//
// A user or local server switched off is moved to ~/.hatrack/mcp/disabled.json, so Claude no
// longer sees it but nothing is lost. A project server is switched off the way Claude does it,
// through disabledMcpjsonServers, since .mcp.json belongs to the team.
package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/baodq97/hatrack/internal/atomicfile"
)

type Scope string

const (
	User    Scope = "user"
	Local   Scope = "local"
	Project Scope = "project"
)

// rank orders scopes for listing; Claude's precedence is the reverse: local, then project, then user.
var rank = map[Scope]int{User: 0, Project: 1, Local: 2}

func ParseScope(s string) (Scope, error) {
	switch Scope(s) {
	case User, Local, Project:
		return Scope(s), nil
	}
	return "", fmt.Errorf("bad scope %q: use user, local or project", s)
}

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// Config knows where Claude keeps its MCP servers and where hatrack keeps its own state.
type Config struct {
	GlobalJSON string // ~/.claude.json
	Project    string // folder for local and project scope; empty means user scope only
	Dir        string // ~/.hatrack/mcp
}

// Default follows Claude Code's lookup (CLAUDE_CONFIG_DIR if set). project may be empty.
func Default(project string) (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	c := &Config{
		GlobalJSON: filepath.Join(home, ".claude.json"),
		Dir:        filepath.Join(home, ".hatrack", "mcp"),
	}
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		c.GlobalJSON = filepath.Join(d, ".claude.json")
	}
	if project != "" {
		if c.Project, err = filepath.Abs(project); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Server is one MCP server as Claude sees it.
type Server struct {
	Name     string
	Scope    Scope
	Enabled  bool
	Shadowed bool // a server of the same name in a narrower scope wins
	Spec     Spec
	Raw      json.RawMessage
}

// List returns every server in reach, switched on or off, user scope first.
func (c *Config) List() ([]Server, error) {
	var out []Server
	for _, sc := range c.scopes() {
		m, err := c.servers(sc)
		if err != nil {
			return nil, err
		}
		for name, raw := range m {
			out = append(out, newServer(name, sc, true, raw))
		}
	}
	st, err := c.readStash()
	if err != nil {
		return nil, err
	}
	for name, raw := range st.User {
		out = append(out, newServer(name, User, false, raw))
	}
	if c.Project != "" {
		for name, raw := range st.Local[c.Project] {
			out = append(out, newServer(name, Local, false, raw))
		}
		off, err := c.projectList("disabledMcpjsonServers")
		if err != nil {
			return nil, err
		}
		for i := range out {
			if out[i].Scope == Project && slices.Contains(off, out[i].Name) {
				out[i].Enabled = false
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return rank[out[i].Scope] < rank[out[j].Scope]
		}
		return out[i].Name < out[j].Name
	})
	for i := range out {
		for j := range out {
			if out[i].Enabled && out[j].Enabled && out[i].Name == out[j].Name && rank[out[j].Scope] > rank[out[i].Scope] {
				out[i].Shadowed = true
			}
		}
	}
	return out, nil
}

// Find returns the servers called name, in scope if one is given.
func (c *Config) Find(name string, scope Scope) (Server, error) {
	ss, err := c.List()
	if err != nil {
		return Server{}, err
	}
	var hits []Server
	for _, s := range ss {
		if s.Name == name && (scope == "" || s.Scope == scope) {
			hits = append(hits, s)
		}
	}
	switch len(hits) {
	case 0:
		if scope != "" {
			return Server{}, fmt.Errorf("no %s-scope MCP server %q", scope, name)
		}
		return Server{}, fmt.Errorf("no MCP server %q", name)
	case 1:
		return hits[0], nil
	}
	var scopes []string
	for _, h := range hits {
		scopes = append(scopes, string(h.Scope))
	}
	return Server{}, fmt.Errorf("%q is in more than one scope (%s): pick one with -s", name, strings.Join(scopes, ", "))
}

// Add writes a server, switched on. An existing server of that name and scope is replaced
// only when force is set.
func (c *Config) Add(name string, scope Scope, spec Spec, force bool) error {
	raw, err := marshal(spec)
	if err != nil {
		return err
	}
	return c.AddRaw(name, scope, raw, force)
}

// AddRaw is Add for a server config kept exactly as given, unknown keys and all.
func (c *Config) AddRaw(name string, scope Scope, raw json.RawMessage, force bool) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("bad server name %q: letters, digits, '.', '_' and '-' only", name)
	}
	if err := c.needProject(scope); err != nil {
		return err
	}
	if old, err := c.Find(name, scope); err == nil {
		if !force {
			return fmt.Errorf("%s-scope MCP server %q already exists; remove it first or use --force", scope, name)
		}
		if err := c.Remove(old.Name, old.Scope); err != nil {
			return err
		}
	}
	m, err := c.servers(scope)
	if err != nil {
		return err
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	m[name] = raw
	return c.saveServers(scope, m)
}

// Remove deletes a server, whether switched on or off.
func (c *Config) Remove(name string, scope Scope) error {
	s, err := c.Find(name, scope)
	if err != nil {
		return err
	}
	if !s.Enabled && s.Scope != Project {
		return c.editStash(func(st *stash) error {
			delete(st.bucket(s.Scope, c.Project), name)
			return nil
		})
	}
	m, err := c.servers(s.Scope)
	if err != nil {
		return err
	}
	delete(m, name)
	if err := c.saveServers(s.Scope, m); err != nil {
		return err
	}
	if s.Scope == Project {
		return c.editProjectLists(name, false, false)
	}
	return nil
}

// Disable switches a server off. Its settings are kept so Enable brings it back as it was.
func (c *Config) Disable(name string, scope Scope) error {
	s, err := c.Find(name, scope)
	if err != nil {
		return err
	}
	if !s.Enabled {
		return nil
	}
	if s.Scope == Project {
		return c.editProjectLists(name, false, true)
	}
	// stash first: a crash between the two writes leaves a copy, never nothing
	if err := c.editStash(func(st *stash) error {
		st.bucket(s.Scope, c.Project)[name] = s.Raw
		return nil
	}); err != nil {
		return err
	}
	m, err := c.servers(s.Scope)
	if err != nil {
		return err
	}
	delete(m, name)
	return c.saveServers(s.Scope, m)
}

// Enable switches a server back on.
func (c *Config) Enable(name string, scope Scope) error {
	s, err := c.Find(name, scope)
	if err != nil {
		return err
	}
	if s.Enabled {
		return nil
	}
	if s.Scope == Project {
		return c.editProjectLists(name, true, false)
	}
	m, err := c.servers(s.Scope)
	if err != nil {
		return err
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	m[name] = s.Raw
	if err := c.saveServers(s.Scope, m); err != nil {
		return err
	}
	return c.editStash(func(st *stash) error {
		delete(st.bucket(s.Scope, c.Project), name)
		return nil
	})
}

func newServer(name string, sc Scope, on bool, raw json.RawMessage) Server {
	s := Server{Name: name, Scope: sc, Enabled: on, Raw: raw}
	json.Unmarshal(raw, &s.Spec) // a server Claude would reject still lists, with what parsed
	return s
}

func (c *Config) scopes() []Scope {
	if c.Project == "" {
		return []Scope{User}
	}
	return []Scope{User, Local, Project}
}

func (c *Config) needProject(scope Scope) error {
	if scope != User && c.Project == "" {
		return fmt.Errorf("%s scope needs a project folder", scope)
	}
	return nil
}

func (c *Config) mcpJSON() string { return filepath.Join(c.Project, ".mcp.json") }

// servers reads the switched-on servers of one scope.
func (c *Config) servers(scope Scope) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	switch scope {
	case User:
		d, err := readDoc(c.GlobalJSON)
		if err != nil {
			return nil, err
		}
		return m, d.get("mcpServers", &m)
	case Local:
		pd, err := c.projectDoc()
		if err != nil {
			return nil, err
		}
		return m, pd.get("mcpServers", &m)
	case Project:
		d, err := readDoc(c.mcpJSON())
		if err != nil {
			return nil, err
		}
		return m, d.get("mcpServers", &m)
	}
	return nil, fmt.Errorf("bad scope %q", scope)
}

func (c *Config) saveServers(scope Scope, m map[string]json.RawMessage) error {
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	switch scope {
	case User:
		return editDoc(c.GlobalJSON, func(d doc) error { return d.set("mcpServers", m) })
	case Local:
		return c.editProject(func(pd doc) error { return pd.set("mcpServers", m) })
	case Project:
		return editDoc(c.mcpJSON(), func(d doc) error { return d.set("mcpServers", m) })
	}
	return fmt.Errorf("bad scope %q", scope)
}

// projectKey is the key Claude files this folder under in projects: the path as is, or with
// forward slashes, which is how Claude writes Windows paths.
func (c *Config) projectKey(projects map[string]json.RawMessage) string {
	slash := filepath.ToSlash(c.Project)
	for _, k := range []string{c.Project, slash} {
		if _, ok := projects[k]; ok {
			return k
		}
	}
	if runtime.GOOS == "windows" {
		return slash
	}
	return c.Project
}

func (c *Config) projectDoc() (doc, error) {
	d, err := readDoc(c.GlobalJSON)
	if err != nil {
		return nil, err
	}
	var projects map[string]json.RawMessage
	if err := d.get("projects", &projects); err != nil {
		return nil, err
	}
	pd := doc{}
	if raw, ok := projects[c.projectKey(projects)]; ok {
		if err := json.Unmarshal(raw, &pd); err != nil {
			return nil, fmt.Errorf("%s: project %s: %w", c.GlobalJSON, c.Project, err)
		}
	}
	return pd, nil
}

// editProject changes this folder's entry in projects and keeps every other key as it was.
func (c *Config) editProject(fn func(pd doc) error) error {
	return editDoc(c.GlobalJSON, func(d doc) error {
		var projects map[string]json.RawMessage
		if err := d.get("projects", &projects); err != nil {
			return err
		}
		if projects == nil {
			projects = map[string]json.RawMessage{}
		}
		key := c.projectKey(projects)
		pd := doc{}
		if raw, ok := projects[key]; ok {
			if err := json.Unmarshal(raw, &pd); err != nil {
				return err
			}
		}
		if err := fn(pd); err != nil {
			return err
		}
		raw, err := marshal(pd)
		if err != nil {
			return err
		}
		projects[key] = raw
		return d.set("projects", projects)
	})
}

func (c *Config) projectList(key string) ([]string, error) {
	pd, err := c.projectDoc()
	if err != nil {
		return nil, err
	}
	var l []string
	return l, pd.get(key, &l)
}

// editProjectLists files a .mcp.json server under Claude's approved or switched-off lists.
func (c *Config) editProjectLists(name string, on, off bool) error {
	return c.editProject(func(pd doc) error {
		for key, want := range map[string]bool{"enabledMcpjsonServers": on, "disabledMcpjsonServers": off} {
			var l []string
			if err := pd.get(key, &l); err != nil {
				return err
			}
			l = slices.DeleteFunc(l, func(n string) bool { return n == name })
			if want {
				l = append(l, name)
			}
			if l == nil {
				l = []string{}
			}
			if err := pd.set(key, l); err != nil {
				return err
			}
		}
		return nil
	})
}

// stash holds switched-off user and local servers, out of Claude's sight.
type stash struct {
	User  map[string]json.RawMessage            `json:"user,omitempty"`
	Local map[string]map[string]json.RawMessage `json:"local,omitempty"` // folder -> servers
}

func (st *stash) bucket(scope Scope, project string) map[string]json.RawMessage {
	if scope == User {
		if st.User == nil {
			st.User = map[string]json.RawMessage{}
		}
		return st.User
	}
	if st.Local == nil {
		st.Local = map[string]map[string]json.RawMessage{}
	}
	if st.Local[project] == nil {
		st.Local[project] = map[string]json.RawMessage{}
	}
	return st.Local[project]
}

func (c *Config) stashPath() string { return filepath.Join(c.Dir, "disabled.json") }

func (c *Config) readStash() (*stash, error) {
	st := &stash{}
	return st, readJSON(c.stashPath(), st)
}

func (c *Config) editStash(fn func(*stash) error) error {
	st, err := c.readStash()
	if err != nil {
		return err
	}
	if err := fn(st); err != nil {
		return err
	}
	for k, v := range st.Local {
		if len(v) == 0 {
			delete(st.Local, k)
		}
	}
	return writeJSON(c.stashPath(), st)
}

// doc is a JSON object edited one key at a time, so keys hatrack does not know survive.
type doc map[string]json.RawMessage

func readDoc(path string) (doc, error) {
	d := doc{}
	return d, readJSON(path, &d)
}

func editDoc(path string, fn func(doc) error) error {
	d, err := readDoc(path)
	if err != nil {
		return err
	}
	if err := fn(d); err != nil {
		return err
	}
	return writeJSON(path, d)
}

func (d doc) get(key string, v any) error {
	raw, ok := d[key]
	if !ok || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return nil
}

func (d doc) set(key string, v any) error {
	raw, err := marshal(v)
	if err != nil {
		return err
	}
	d[key] = raw
	return nil
}

// readJSON leaves v as is when path does not exist.
func readJSON(path string, v any) error {
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

func writeJSON(path string, v any) error {
	b, err := atomicfile.JSON(v)
	if err != nil {
		return err
	}
	return atomicfile.Write(path, b)
}

func marshal(v any) (json.RawMessage, error) {
	b, err := atomicfile.JSON(v)
	return json.RawMessage(strings.TrimSpace(string(b))), err
}
