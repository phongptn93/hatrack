package mcp

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Template is a ready-made server config with {{VARS}} the user fills in when adding it.
type Template struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Category    string `json:"category,omitempty"`
	Publisher   string `json:"publisher,omitempty"` // who publishes the server: its vendor, or community
	Description string `json:"description,omitempty"`
	Docs        string `json:"docs,omitempty"`
	Server      Spec   `json:"server"`
	Vars        []Var  `json:"vars,omitempty"`
	Custom      bool   `json:"-"` // from the user's catalog.json
}

type Var struct {
	Name    string `json:"name"`
	Prompt  string `json:"prompt"`
	Default string `json:"default,omitempty"`
	Secret  bool   `json:"secret,omitempty"` // typed without echo, never printed
}

// builtin lists servers people commonly add, each checked against its maintainer's docs and run
// with `hat mcp test`. Each runs as its maintainer publishes it; hatrack only writes the config
// Claude starts it with. Prefer servers published by the vendor and still maintained.
var builtin = []Template{
	// databases
	{
		Name: "postgres", Title: "PostgreSQL", Category: "database", Publisher: "Google (MCP Toolbox)",
		Description: "Query, list tables, query plans, locks and health; give it a read-only login",
		Docs:        "https://mcp-toolbox.dev",
		Server: Spec{Type: "stdio", Command: "npx", Args: []string{"-y", "@toolbox-sdk/server", "--prebuilt", "postgres", "--stdio"},
			Env: map[string]string{"POSTGRES_HOST": "{{POSTGRES_HOST}}", "POSTGRES_PORT": "{{POSTGRES_PORT}}", "POSTGRES_DATABASE": "{{POSTGRES_DATABASE}}",
				"POSTGRES_USER": "{{POSTGRES_USER}}", "POSTGRES_PASSWORD": "{{POSTGRES_PASSWORD}}"}},
		Vars: []Var{
			{Name: "POSTGRES_HOST", Prompt: "Host", Default: "localhost"},
			{Name: "POSTGRES_PORT", Prompt: "Port", Default: "5432"},
			{Name: "POSTGRES_DATABASE", Prompt: "Database"},
			{Name: "POSTGRES_USER", Prompt: "User (best a read-only one)"},
			{Name: "POSTGRES_PASSWORD", Prompt: "Password", Secret: true},
		},
	},
	{
		Name: "mssql", Title: "SQL Server", Category: "database", Publisher: "community (BYMCS)",
		Description: "SQL Server or Azure SQL with a SQL login; it can run any SQL, so give it a read-only login",
		Docs:        "https://github.com/BYMCS/mssql-mcp",
		Server: Spec{Type: "stdio", Command: "npx", Args: []string{"-y", "mssql-mcp@latest"},
			Env: map[string]string{"DB_SERVER": "{{DB_SERVER}}", "DB_PORT": "{{DB_PORT}}", "DB_DATABASE": "{{DB_DATABASE}}",
				"DB_USER": "{{DB_USER}}", "DB_PASSWORD": "{{DB_PASSWORD}}", "DB_TRUST_SERVER_CERTIFICATE": "{{DB_TRUST_SERVER_CERTIFICATE}}"}},
		Vars: []Var{
			{Name: "DB_SERVER", Prompt: "Server, e.g. localhost or myserver.database.windows.net"},
			{Name: "DB_PORT", Prompt: "Port", Default: "1433"},
			{Name: "DB_DATABASE", Prompt: "Database"},
			{Name: "DB_USER", Prompt: "SQL login (best a read-only one)"},
			{Name: "DB_PASSWORD", Prompt: "Password", Secret: true},
			{Name: "DB_TRUST_SERVER_CERTIFICATE", Prompt: "Trust a self-signed server certificate? true/false", Default: "false"},
		},
	},
	{
		Name: "mysql", Title: "MySQL / MariaDB", Category: "database", Publisher: "community (benborla)",
		Description: "Query MySQL or MariaDB; read-only unless you allow writes",
		Docs:        "https://github.com/benborla/mcp-server-mysql",
		Server: Spec{Type: "stdio", Command: "npx", Args: []string{"-y", "@benborla29/mcp-server-mysql"},
			Env: map[string]string{"MYSQL_HOST": "{{MYSQL_HOST}}", "MYSQL_PORT": "{{MYSQL_PORT}}", "MYSQL_USER": "{{MYSQL_USER}}",
				"MYSQL_PASS": "{{MYSQL_PASS}}", "MYSQL_DB": "{{MYSQL_DB}}"}},
		Vars: []Var{
			{Name: "MYSQL_HOST", Prompt: "Host", Default: "127.0.0.1"},
			{Name: "MYSQL_PORT", Prompt: "Port", Default: "3306"},
			{Name: "MYSQL_USER", Prompt: "User"},
			{Name: "MYSQL_PASS", Prompt: "Password", Secret: true},
			{Name: "MYSQL_DB", Prompt: "Database"},
		},
	},
	{
		Name: "sqlite", Title: "SQLite", Category: "database", Publisher: "Google (MCP Toolbox)",
		Description: "Query a local SQLite file",
		Docs:        "https://mcp-toolbox.dev",
		Server: Spec{Type: "stdio", Command: "npx", Args: []string{"-y", "@toolbox-sdk/server", "--prebuilt", "sqlite", "--stdio"},
			Env: map[string]string{"SQLITE_DATABASE": "{{SQLITE_DATABASE}}"}},
		Vars: []Var{{Name: "SQLITE_DATABASE", Prompt: "Full path to the .db file"}},
	},
	{
		Name: "mongodb", Title: "MongoDB", Category: "database", Publisher: "MongoDB",
		Description: "Query a MongoDB database or Atlas cluster, read-only",
		Docs:        "https://github.com/mongodb-js/mongodb-mcp-server",
		Server: Spec{Type: "stdio", Command: "npx", Args: []string{"-y", "mongodb-mcp-server", "--readOnly"},
			Env: map[string]string{"MDB_MCP_CONNECTION_STRING": "{{CONNECTION_STRING}}"}},
		Vars: []Var{{Name: "CONNECTION_STRING", Prompt: "Connection string, e.g. mongodb://user:pass@localhost:27017/db", Secret: true}},
	},
	{
		Name: "redis", Title: "Redis", Category: "database", Publisher: "Redis",
		Description: "Read and write Redis keys and data structures (needs uv)",
		Docs:        "https://github.com/redis/mcp-redis",
		Server:      Spec{Type: "stdio", Command: "uvx", Args: []string{"--from", "redis-mcp-server@latest", "redis-mcp-server", "--url", "{{REDIS_URL}}"}},
		Vars:        []Var{{Name: "REDIS_URL", Prompt: "URL, e.g. redis://localhost:6379/0", Default: "redis://localhost:6379/0", Secret: true}},
	},
	{
		Name: "dbhub", Title: "Any SQL database by DSN (DBHub)", Category: "database", Publisher: "Bytebase",
		Description: "PostgreSQL, MySQL, MariaDB, SQL Server or SQLite from one DSN; URL-encode special characters in the password",
		Docs:        "https://github.com/bytebase/dbhub",
		Server:      Spec{Type: "stdio", Command: "npx", Args: []string{"-y", "@bytebase/dbhub", "--transport", "stdio", "--dsn", "{{DSN}}"}},
		Vars:        []Var{{Name: "DSN", Prompt: "DSN, e.g. postgres://user:pass@localhost:5432/db?sslmode=disable", Secret: true}},
	},
	// devops and work tracking
	{
		Name: "azure-devops", Title: "Azure DevOps", Category: "devops", Publisher: "Microsoft",
		Description: "Work items, repos, pull requests, pipelines, wikis; Microsoft's hosted server, sign in with /mcp in Claude",
		Docs:        "https://learn.microsoft.com/azure/devops/mcp-server/remote-mcp-server",
		Server:      Spec{Type: "http", URL: "https://mcp.dev.azure.com/{{ADO_ORG}}"},
		Vars:        []Var{{Name: "ADO_ORG", Prompt: "Organization, e.g. contoso for dev.azure.com/contoso"}},
	},
	{
		Name: "azure-devops-local", Title: "Azure DevOps (local server)", Category: "devops", Publisher: "Microsoft",
		Description: "The same, run on your machine; signs in through the browser. Linux/WSL needs libsecret (apt install libsecret-1-0)",
		Docs:        "https://github.com/microsoft/azure-devops-mcp",
		Server:      Spec{Type: "stdio", Command: "npx", Args: []string{"-y", "@azure-devops/mcp", "{{ADO_ORG}}"}},
		Vars:        []Var{{Name: "ADO_ORG", Prompt: "Organization, e.g. contoso for dev.azure.com/contoso"}},
	},
	{
		Name: "azure", Title: "Azure", Category: "devops", Publisher: "Microsoft",
		Description: "Azure resources (Storage, Cosmos DB, Key Vault, Monitor…) through your Azure CLI login",
		Docs:        "https://github.com/microsoft/mcp",
		Server:      Spec{Type: "stdio", Command: "npx", Args: []string{"-y", "@azure/mcp@latest", "server", "start"}},
	},
	{
		Name: "github", Title: "GitHub", Category: "devops", Publisher: "GitHub",
		Description: "Repos, issues, pull requests and Actions, through GitHub's hosted server",
		Docs:        "https://github.com/github/github-mcp-server",
		Server: Spec{Type: "http", URL: "https://api.githubcopilot.com/mcp/",
			Headers: map[string]string{"Authorization": "Bearer {{GITHUB_TOKEN}}"}},
		Vars: []Var{{Name: "GITHUB_TOKEN", Prompt: "Personal access token (or ${GITHUB_TOKEN} to read it from the environment)", Secret: true}},
	},
	{
		Name: "atlassian", Title: "Jira and Confluence", Category: "devops", Publisher: "Atlassian",
		Description: "Atlassian's hosted server; sign in with /mcp in Claude",
		Docs:        "https://github.com/atlassian/atlassian-mcp-server",
		Server:      Spec{Type: "http", URL: "https://mcp.atlassian.com/v2/mcp"},
	},
	{
		Name: "linear", Title: "Linear", Category: "devops", Publisher: "Linear",
		Description: "Linear issues and projects; sign in with /mcp in Claude",
		Docs:        "https://linear.app/docs/mcp",
		Server:      Spec{Type: "http", URL: "https://mcp.linear.app/mcp"},
	},
	{
		Name: "sentry", Title: "Sentry", Category: "devops", Publisher: "Sentry",
		Description: "Errors and performance issues; sign in with /mcp in Claude",
		Docs:        "https://github.com/getsentry/sentry-mcp",
		Server:      Spec{Type: "http", URL: "https://mcp.sentry.dev/mcp"},
	},
	// docs and knowledge
	{
		Name: "context7", Title: "Context7", Category: "docs", Publisher: "Upstash",
		Description: "Up-to-date library documentation for the code you write",
		Docs:        "https://github.com/upstash/context7",
		Server:      Spec{Type: "http", URL: "https://mcp.context7.com/mcp"},
	},
	{
		Name: "notion", Title: "Notion", Category: "docs", Publisher: "Notion",
		Description: "Notion pages and databases; sign in with /mcp in Claude",
		Docs:        "https://developers.notion.com/docs/mcp",
		Server:      Spec{Type: "http", URL: "https://mcp.notion.com/mcp"},
	},
	// local tools
	{
		Name: "playwright", Title: "Playwright browser", Category: "tools", Publisher: "Microsoft",
		Description: "Drive a real browser: open pages, click, fill forms, take screenshots",
		Docs:        "https://github.com/microsoft/playwright-mcp",
		Server:      Spec{Type: "stdio", Command: "npx", Args: []string{"-y", "@playwright/mcp@latest"}},
	},
	{
		Name: "filesystem", Title: "Filesystem", Category: "tools", Publisher: "MCP project",
		Description: "Read and write files under one folder",
		Docs:        "https://github.com/modelcontextprotocol/servers/tree/main/src/filesystem",
		Server:      Spec{Type: "stdio", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "{{DIR}}"}},
		Vars:        []Var{{Name: "DIR", Prompt: "Folder to allow"}},
	},
	{
		Name: "fetch", Title: "Fetch", Category: "tools", Publisher: "MCP project",
		Description: "Fetch web pages as Markdown (needs uv)",
		Docs:        "https://github.com/modelcontextprotocol/servers/tree/main/src/fetch",
		Server:      Spec{Type: "stdio", Command: "uvx", Args: []string{"mcp-server-fetch"}},
	},
	{
		Name: "memory", Title: "Memory", Category: "tools", Publisher: "MCP project",
		Description: "A local knowledge graph Claude can keep notes in",
		Docs:        "https://github.com/modelcontextprotocol/servers/tree/main/src/memory",
		Server:      Spec{Type: "stdio", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-memory"}},
	},
}

func (c *Config) catalogPath() string { return filepath.Join(c.Dir, "catalog.json") }

// Catalog is the built-in templates plus the user's ~/.hatrack/mcp/catalog.json, which wins on
// a name clash. A team can share its own servers (an internal database, its ADO org) that way.
func (c *Config) Catalog() ([]Template, error) {
	var custom []Template
	if err := readJSON(c.catalogPath(), &custom); err != nil {
		return nil, err
	}
	byName := map[string]Template{}
	for _, t := range builtin {
		byName[t.Name] = t
	}
	for _, t := range custom {
		if !validName.MatchString(t.Name) {
			return nil, fmt.Errorf("%s: bad template name %q", c.catalogPath(), t.Name)
		}
		t.Custom = true
		byName[t.Name] = t
	}
	out := make([]Template, 0, len(byName))
	for _, t := range byName {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (c *Config) Template(name string) (Template, error) {
	ts, err := c.Catalog()
	if err != nil {
		return Template{}, err
	}
	for _, t := range ts {
		if t.Name == name {
			return t, nil
		}
	}
	return Template{}, fmt.Errorf("no template %q: see `hat mcp catalog`", name)
}

// Render fills the template's {{VARS}}. vars missing a value fall back to the default; one with
// neither is an error. On Windows, npx is started through cmd /c, which Claude needs there.
func (t Template) Render(vars map[string]string, goos string) (Spec, error) {
	vals := map[string]string{}
	var missing []string
	for _, v := range t.Vars {
		val, ok := vars[v.Name]
		if !ok || val == "" {
			val = v.Default
		}
		if val == "" {
			missing = append(missing, v.Name)
		}
		vals[v.Name] = val
	}
	if len(missing) > 0 {
		return Spec{}, fmt.Errorf("template %q needs %s", t.Name, strings.Join(missing, ", "))
	}
	s := t.Server.mapStrings(func(s string) string {
		return placeholder.ReplaceAllStringFunc(s, func(p string) string {
			if v, ok := vals[p[2:len(p)-2]]; ok {
				return v
			}
			return p
		})
	})
	if left := s.Placeholders(); len(left) > 0 {
		return Spec{}, fmt.Errorf("template %q uses {{%s}} but does not declare it", t.Name, left[0])
	}
	if goos == "windows" && s.Command == "npx" {
		s.Command, s.Args = "cmd", append([]string{"/c", "npx"}, s.Args...)
	}
	return s, nil
}
