package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/baodq97/hatrack/internal/atomicfile"
)

// Export returns the servers of one scope, switched on or off, as an mcpServers file in the
// format of .mcp.json. It holds secrets as they are stored.
func (c *Config) Export(scope Scope) ([]byte, error) {
	all, err := c.List()
	if err != nil {
		return nil, err
	}
	m := map[string]json.RawMessage{}
	for _, s := range all {
		if s.Scope == scope {
			m[s.Name] = s.Raw
		}
	}
	return atomicfile.JSON(map[string]any{"mcpServers": m})
}

// Import adds the servers of an mcpServers file: an export, a .mcp.json, or Claude Desktop's
// claude_desktop_config.json. A server whose name is taken is skipped unless force is set.
func (c *Config) Import(path string, scope Scope, force bool) (added, skipped []string, err error) {
	var f struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(f.MCPServers) == 0 {
		return nil, nil, fmt.Errorf("%s has no mcpServers", path)
	}
	names := sortedKeys(f.MCPServers)
	for _, name := range names {
		if _, err := c.Find(name, scope); err == nil && !force {
			skipped = append(skipped, name)
			continue
		}
		if err := c.AddRaw(name, scope, f.MCPServers[name], force); err != nil {
			return added, skipped, fmt.Errorf("%s: %w", name, err)
		}
		added = append(added, name)
	}
	sort.Strings(added)
	return added, skipped, nil
}

// DesktopConfig is where Claude Desktop keeps its MCP servers on this OS.
func DesktopConfig() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Claude", "claude_desktop_config.json"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), err
	}
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, "Claude", "claude_desktop_config.json"), err
}
