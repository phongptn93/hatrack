package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestMain doubles as a fake stdio MCP server when a probe test starts this binary.
func TestMain(m *testing.M) {
	if os.Getenv("HATRACK_FAKE_MCP") != "" {
		fakeServer()
		return
	}
	os.Exit(m.Run())
}

func fakeServer() {
	fmt.Println("a log line a sloppy server prints first")
	sc := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		json.Unmarshal(sc.Bytes(), &m)
		switch m.Method {
		case "initialize":
			out.Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{
				"protocolVersion": protocolVersion, "capabilities": map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]string{"name": "fake", "version": os.Getenv("FAKE_VERSION")}}})
		case "tools/list":
			out.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/message", "params": map[string]any{}})
			out.Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{
				"tools": []map[string]string{{"name": "query", "description": "run SQL"}}}})
		}
	}
}

func newConfig(t *testing.T) *Config {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	must(t, os.MkdirAll(proj, 0o700))
	c := &Config{GlobalJSON: filepath.Join(root, ".claude.json"), Project: proj, Dir: filepath.Join(root, ".hatrack", "mcp")}
	write(t, c.GlobalJSON, `{"numStartups":7,"oauthAccount":{"emailAddress":"a@x.com"},"tip":"<b>",
		"projects":{"`+filepath.ToSlash(proj)+`":{"allowedTools":["x"]}}}`)
	return c
}

func states(t *testing.T, c *Config) string {
	t.Helper()
	ss, err := c.List()
	must(t, err)
	var out []string
	for _, s := range ss {
		st := "on"
		if !s.Enabled {
			st = "off"
		} else if s.Shadowed {
			st = "hidden"
		}
		out = append(out, fmt.Sprintf("%s:%s:%s", s.Scope, s.Name, st))
	}
	return strings.Join(out, " ")
}

func TestScopesOnOffKeepOtherKeys(t *testing.T) {
	c := newConfig(t)
	db := Spec{Type: "stdio", Command: "uvx", Args: []string{"postgres-mcp"}, Env: map[string]string{"DATABASE_URI": "postgresql://u:secret@h/db"}}
	must(t, c.Add("db", User, db, false))
	must(t, c.Add("ado", User, Spec{Type: "stdio", Command: "npx", Args: []string{"-y", "@azure-devops/mcp", "contoso"}}, false))
	must(t, c.Add("db", Local, Spec{Type: "http", URL: "https://db.local/mcp"}, false))
	write(t, filepath.Join(c.Project, ".mcp.json"), `{"mcpServers":{"team":{"command":"team-mcp"}}}`)

	if got, want := states(t, c), "user:ado:on user:db:hidden project:team:on local:db:on"; got != want {
		t.Fatalf("list:\n got %s\nwant %s", got, want)
	}
	if err := c.Add("ado", User, Spec{Command: "x"}, false); err == nil {
		t.Fatal("add over an existing server without force")
	}
	if _, err := c.Find("db", ""); err == nil || !strings.Contains(err.Error(), "more than one scope") {
		t.Fatalf("ambiguous find: %v", err)
	}

	must(t, c.Disable("db", Local))
	must(t, c.Disable("ado", ""))
	must(t, c.Disable("team", ""))
	if got, want := states(t, c), "user:ado:off user:db:on project:team:off local:db:off"; got != want {
		t.Fatalf("after off:\n got %s\nwant %s", got, want)
	}
	// Claude no longer sees the switched-off user and local servers; the team's file is untouched
	g := readFile(t, c.GlobalJSON)
	if strings.Contains(g, "azure-devops") || strings.Contains(g, "db.local") {
		t.Fatalf("switched-off servers still in .claude.json:\n%s", g)
	}
	for _, keep := range []string{`"numStartups": 7`, `"tip": "<b>"`, `"allowedTools"`, `"emailAddress": "a@x.com"`, `"disabledMcpjsonServers": [`} {
		if !strings.Contains(g, keep) {
			t.Fatalf(".claude.json lost %s:\n%s", keep, g)
		}
	}
	if !strings.Contains(readFile(t, filepath.Join(c.Project, ".mcp.json")), "team-mcp") {
		t.Fatal(".mcp.json changed")
	}

	must(t, c.Enable("ado", User))
	must(t, c.Enable("db", Local))
	must(t, c.Enable("team", Project))
	if got, want := states(t, c), "user:ado:on user:db:hidden project:team:on local:db:on"; got != want {
		t.Fatalf("after on:\n got %s\nwant %s", got, want)
	}
	s, err := c.Find("ado", User)
	must(t, err)
	if s.Spec.Target() != "npx -y @azure-devops/mcp contoso" {
		t.Fatalf("ado came back as %q", s.Spec.Target())
	}

	must(t, c.Disable("ado", User))
	must(t, c.Remove("ado", User)) // removing a switched-off server empties the stash
	must(t, c.Remove("db", Local))
	if got, want := states(t, c), "user:db:on project:team:on"; got != want {
		t.Fatalf("after rm:\n got %s\nwant %s", got, want)
	}
}

func TestUserScopeOnlyWithoutProject(t *testing.T) {
	c := newConfig(t)
	must(t, c.Add("db", Local, Spec{Command: "x"}, false))
	c.Project = ""
	if got := states(t, c); got != "" {
		t.Fatalf("list without a project: %s", got)
	}
	if err := c.Add("db", Local, Spec{Command: "x"}, false); err == nil {
		t.Fatal("local add without a project")
	}
}

func TestCatalogRender(t *testing.T) {
	c := newConfig(t)
	ts, err := c.Catalog()
	must(t, err)
	for _, tp := range ts { // every template declares what it uses
		vals := map[string]string{}
		for _, v := range tp.Vars {
			vals[v.Name] = "v"
		}
		if _, err := tp.Render(vals, "linux"); err != nil {
			t.Errorf("%s: %v", tp.Name, err)
		}
	}

	ado, err := c.Template("azure-devops")
	must(t, err)
	if _, err := ado.Render(nil, "linux"); err == nil {
		t.Fatal("render without the org")
	}
	sp, err := ado.Render(map[string]string{"ADO_ORG": "contoso"}, "windows")
	must(t, err)
	if got := sp.Target(); got != "cmd /c npx -y @azure-devops/mcp contoso" {
		t.Fatalf("windows ado: %q", got)
	}
	my, err := c.Template("mysql")
	must(t, err)
	sp, err = my.Render(map[string]string{"MYSQL_USER": "u", "MYSQL_PASS": "p", "MYSQL_DB": "d"}, "linux")
	must(t, err)
	if sp.Env["MYSQL_PORT"] != "3306" || sp.Env["MYSQL_PASS"] != "p" {
		t.Fatalf("mysql env: %v", sp.Env)
	}

	// a team catalog adds templates and overrides built-ins
	write(t, filepath.Join(c.Dir, "catalog.json"), `[{"name":"azure-devops","title":"Our ADO","server":{"type":"stdio","command":"npx","args":["-y","@azure-devops/mcp","ourorg"]}}]`)
	ado, err = c.Template("azure-devops")
	must(t, err)
	if !ado.Custom || ado.Title != "Our ADO" {
		t.Fatalf("custom template: %+v", ado)
	}
}

func TestRedactAndExpand(t *testing.T) {
	sp := Spec{
		Args:    []string{"--dsn", "sqlserver://sa:P@ss@host:1433/db", "postgres://u:${PGPASS}@h/db"},
		Env:     map[string]string{"TOKEN": "abc123", "REF": "${MY_TOKEN}", "EMPTY": "", "DB_SERVER": "db1", "DB_PASSWORD": "pw", "LOG": "x://u:pw@h"},
		Headers: map[string]string{"Authorization": "Bearer ghp_xyz", "X-Ref": "Bearer ${GH}"},
	}
	r := sp.Redacted()
	if r.Args[1] != "sqlserver://sa:****@host:1433/db" || r.Args[2] != "postgres://u:${PGPASS}@h/db" {
		t.Fatalf("args: %q", r.Args)
	}
	if r.Env["DB_SERVER"] != "db1" || r.Env["DB_PASSWORD"] != "****" || r.Env["LOG"] != "x://u:****@h" {
		t.Fatalf("env: %v", r.Env)
	}
	if r.Env["TOKEN"] != "****" || r.Env["REF"] != "${MY_TOKEN}" || r.Headers["Authorization"] != "Bearer ****" || r.Headers["X-Ref"] != "Bearer ${GH}" {
		t.Fatalf("env %v headers %v", r.Env, r.Headers)
	}
	env := map[string]string{"PGPASS": "pw"}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	if got := (Spec{URL: "${HOST:-localhost}/${PGPASS}"}).Expand(lookup).URL; got != "localhost/pw" {
		t.Fatalf("expand: %q", got)
	}
	if got := sp.EnvRefs(lookup); !slices.Equal(got, []string{"GH", "MY_TOKEN"}) {
		t.Fatalf("missing refs: %v", got)
	}
}

func TestSetsAndAccountLink(t *testing.T) {
	c := newConfig(t)
	for _, n := range []string{"ado", "mssql", "github"} {
		must(t, c.Add(n, User, Spec{Command: n}, false))
	}
	must(t, c.SaveSet("work", []string{"ado", "mssql"}))
	must(t, c.SaveSet("personal", []string{"github"}))
	must(t, c.LinkSet("work", "me@company.com"))
	if err := c.SaveSet("bad", []string{"nope"}); err == nil {
		t.Fatal("set with an unknown server")
	}

	changed, err := c.UseSet("work")
	must(t, err)
	if !slices.Equal(changed, []string{"-github"}) {
		t.Fatalf("use work: %v", changed)
	}
	changed, err = c.UseSet("personal")
	must(t, err)
	if !slices.Equal(changed, []string{"-ado", "+github", "-mssql"}) {
		t.Fatalf("use personal: %v", changed)
	}
	set, ok, err := c.SetFor("me@company.com")
	must(t, err)
	if !ok || set.Name != "work" {
		t.Fatalf("linked set: %v %v", set, ok)
	}
	must(t, c.LinkSet("personal", "me@company.com")) // one set per account
	set, _, _ = c.SetFor("me@company.com")
	if set.Name != "personal" {
		t.Fatalf("relinked set: %v", set)
	}
	// saving with no names keeps what is on now
	must(t, c.SaveSet("now", nil))
	ss, _ := c.Sets()
	if i := slices.IndexFunc(ss, func(s Set) bool { return s.Name == "now" }); i < 0 || !slices.Equal(ss[i].Servers, []string{"github"}) {
		t.Fatalf("sets: %+v", ss)
	}
}

func TestImportExport(t *testing.T) {
	c := newConfig(t)
	desk := filepath.Join(t.TempDir(), "claude_desktop_config.json")
	write(t, desk, `{"globalShortcut":"x","mcpServers":{"fs":{"command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","/tmp"],"custom":1},"ado":{"command":"y"}}}`)
	must(t, c.Add("ado", User, Spec{Command: "mine"}, false))
	added, skipped, err := c.Import(desk, User, false)
	must(t, err)
	if !slices.Equal(added, []string{"fs"}) || !slices.Equal(skipped, []string{"ado"}) {
		t.Fatalf("import: added %v skipped %v", added, skipped)
	}
	must(t, c.Disable("fs", User))
	b, err := c.Export(User)
	must(t, err)
	if !strings.Contains(string(b), `"custom": 1`) || !strings.Contains(string(b), `"mine"`) {
		t.Fatalf("export lost servers or keys:\n%s", b)
	}
}

func TestCheck(t *testing.T) {
	old := lookPath
	defer func() { lookPath = old }()
	lookPath = func(f string) (string, error) {
		if f == "npx" {
			return "/usr/bin/npx", nil
		}
		return "", os.ErrNotExist
	}
	if got := Check(Server{Spec: Spec{Command: "cmd", Args: []string{"/c", "npx", "-y", "x"}}}); len(got) != 0 {
		t.Fatalf("cmd /c npx: %v", got)
	}
	got := Check(Server{Spec: Spec{Command: "uvx", Args: []string{"{{DSN}}"}}})
	if len(got) != 2 || !strings.Contains(got[1], "install uv") {
		t.Fatalf("uvx with a placeholder: %v", got)
	}
	if got := Check(Server{Spec: Spec{Type: "http", URL: "not a url"}}); len(got) != 1 {
		t.Fatalf("bad url: %v", got)
	}
}

func TestProbeStdio(t *testing.T) {
	exe, err := os.Executable()
	must(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	t.Setenv("FAKE_V", "1.2")
	info, err := Probe(ctx, Spec{Command: exe, Env: map[string]string{"HATRACK_FAKE_MCP": "1", "FAKE_VERSION": "${FAKE_V}"}})
	must(t, err)
	if info.Server != "fake" || info.Version != "1.2" || len(info.Tools) != 1 || info.Tools[0].Name != "query" {
		t.Fatalf("probe: %+v", info)
	}

	_, err = Probe(ctx, Spec{Command: exe, Args: []string{"-test.run=^$"}}) // exits without answering
	if err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("probe of a server that exits: %v", err)
	}
}

func TestProbeHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&m)
		switch m.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "s1")
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{
				"protocolVersion": protocolVersion, "capabilities": map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]string{"name": "remote", "version": "2"}}})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if r.Header.Get("Mcp-Session-Id") != "s1" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{
				"tools": []map[string]string{{"name": "wit_get_work_item"}, {"name": "repo_list"}}}})
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	info, err := Probe(ctx, Spec{Type: "http", URL: srv.URL, Headers: map[string]string{"Authorization": "Bearer good"}})
	must(t, err)
	if info.Server != "remote" || len(info.Tools) != 2 {
		t.Fatalf("probe: %+v", info)
	}
	if _, err := Probe(ctx, Spec{Type: "http", URL: srv.URL}); err == nil || !strings.Contains(err.Error(), "sign in") {
		t.Fatalf("unauthorized probe: %v", err)
	}
}

func write(t *testing.T, path, s string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o700))
	must(t, os.WriteFile(path, []byte(s), 0o600))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	must(t, err)
	return string(b)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
