package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/baodq97/hatrack/internal/mcp"
)

func TestPickTemplate(t *testing.T) {
	root := t.TempDir()
	c := &mcp.Config{GlobalJSON: filepath.Join(root, ".claude.json"), Dir: filepath.Join(root, ".hatrack", "mcp")}
	ts, err := c.Catalog()
	if err != nil || len(ts) < 2 {
		t.Fatalf("catalog: %d templates, %v", len(ts), err)
	}

	var out strings.Builder
	name, tmpl, err := pickTemplate(c, strings.NewReader("nope\n2\nmydb\n"), &out)
	if err != nil || name != "mydb" || tmpl != ts[1].Name {
		t.Fatalf("by number: %q %q %v", name, tmpl, err)
	}
	if !strings.Contains(out.String(), `no "nope"`) || !strings.Contains(out.String(), "  1. "+ts[0].Name) {
		t.Fatalf("output:\n%s", out.String())
	}

	name, tmpl, err = pickTemplate(c, strings.NewReader(strings.ToUpper(ts[0].Name)+"\n\n"), &out)
	if err != nil || name != ts[0].Name || tmpl != ts[0].Name {
		t.Fatalf("by name, default name: %q %q %v", name, tmpl, err)
	}

	if _, _, err = pickTemplate(c, strings.NewReader(""), &out); err == nil {
		t.Fatal("no input should cancel")
	}
}
