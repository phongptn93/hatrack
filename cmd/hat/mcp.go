package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/baodq97/hatrack/internal/atomicfile"
	"github.com/baodq97/hatrack/internal/mcp"
	"golang.org/x/term"
)

const mcpUsage = `hat mcp - manage the MCP servers Claude Code loads

  hat mcp [ls]                     list servers for this folder (user, local, project scope)
  hat mcp catalog                  list ready-made servers: databases, Azure DevOps, GitHub…
  hat mcp add <name> --from <template> [--var K=V]…
  hat mcp add <name> [-e K=V]… -- <command> [args…]
  hat mcp add <name> --url <url> [--transport http|sse] [-H "Key: value"]…
  hat mcp show <name> [--reveal]   print a server's config, secrets hidden unless --reveal
  hat mcp off <name>               switch a server off; its config is kept
  hat mcp on <name>                switch it back on
  hat mcp rm <name>                delete a server
  hat mcp test [name]              start servers, run the MCP handshake, list their tools
  hat mcp doctor                   find servers that cannot start, without starting them
  hat mcp export [-o file]         write servers to a file, secrets included
  hat mcp import <file>|--desktop  add servers from a file or from Claude Desktop

  hat mcp set                      list sets: named groups of user-scope servers
  hat mcp set save <set> [names…]  save a set; no names means the servers on now
  hat mcp set use <set>            switch the set's servers on and the others off
  hat mcp set link <set> <account> use the set whenever 'hat use <account>' runs
  hat mcp set unlink <set>
  hat mcp set rm <set>

Most commands take -s user|local|project. add defaults to user scope (every folder);
local is this folder for you only, project is this folder's .mcp.json, shared with the team.
Restart Claude (VS Code: Reload Window) to pick up changes.`

func runMCP(args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	c, err := mcp.Default(wd)
	if err != nil {
		return err
	}
	cmd := "ls"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "ls", "list":
		return mcpList(c, args)
	case "catalog":
		return mcpCatalog(c)
	case "add":
		return mcpAdd(c, args)
	case "show":
		return mcpShow(c, args)
	case "on", "enable":
		return mcpToggle(c, args, true)
	case "off", "disable":
		return mcpToggle(c, args, false)
	case "rm", "remove":
		name, scope, err := nameAndScope("rm", args, nil)
		if err != nil {
			return err
		}
		if err := c.Remove(name, scope); err != nil {
			return err
		}
		fmt.Println("removed", name)
		return nil
	case "test":
		return mcpTest(c, args)
	case "doctor":
		return mcpDoctor(c)
	case "export":
		return mcpExport(c, args)
	case "import":
		return mcpImport(c, args)
	case "set", "sets":
		return mcpSet(c, args)
	case "-h", "--help", "help":
		fmt.Println(mcpUsage)
		return nil
	}
	return fmt.Errorf("unknown mcp command %q\n\n%s", cmd, mcpUsage)
}

// multi is a flag that can be given more than once.
type multi []string

func (m *multi) String() string     { return strings.Join(*m, ", ") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

type scopeFlag struct{ s mcp.Scope }

func (f *scopeFlag) String() string { return string(f.s) }
func (f *scopeFlag) Set(v string) (err error) {
	f.s, err = mcp.ParseScope(v)
	return err
}

func newFlags(name string, scope *scopeFlag) *flag.FlagSet {
	fs := flag.NewFlagSet("hat mcp "+name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if scope != nil {
		fs.Var(scope, "s", "scope: user, local or project")
		fs.Var(scope, "scope", "scope: user, local or project")
	}
	return fs
}

// parse reads flags given before or after the positional arguments. Everything after "--" is
// returned as rest, untouched, so a server's own flags are not taken for hat's.
func parse(fs *flag.FlagSet, args []string) (pos, rest []string, err error) {
	if i := slices.Index(args, "--"); i >= 0 {
		args, rest = args[:i], args[i+1:]
	}
	for {
		if err := fs.Parse(args); err != nil {
			return nil, nil, fmt.Errorf("%w\n\nsee `hat mcp help`", err)
		}
		if args = fs.Args(); len(args) == 0 {
			return pos, rest, nil
		}
		pos, args = append(pos, args[0]), args[1:]
	}
}

func nameAndScope(cmd string, args []string, setup func(*flag.FlagSet)) (string, mcp.Scope, error) {
	var scope scopeFlag
	fs := newFlags(cmd, &scope)
	if setup != nil {
		setup(fs)
	}
	pos, rest, err := parse(fs, args)
	if err != nil {
		return "", "", err
	}
	if len(pos) != 1 || len(rest) > 0 {
		return "", "", fmt.Errorf("usage: hat mcp %s <name> [-s scope]", cmd)
	}
	return pos[0], scope.s, nil
}

func mcpList(c *mcp.Config, args []string) error {
	var scope scopeFlag
	if _, _, err := parse(newFlags("ls", &scope), args); err != nil {
		return err
	}
	ss, err := c.List()
	if err != nil {
		return err
	}
	if len(ss) == 0 {
		fmt.Println("no MCP servers yet: see `hat mcp catalog`, then `hat mcp add <name> --from <template>`")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SCOPE\tNAME\tSTATE\tTYPE\tTARGET")
	for _, s := range ss {
		if scope.s != "" && s.Scope != scope.s {
			continue
		}
		state := "on"
		switch {
		case !s.Enabled:
			state = "off"
		case s.Shadowed:
			state = "hidden"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Scope, s.Name, state, s.Spec.Kind(), shorten(s.Spec.Redacted().Target(), 70))
	}
	w.Flush()
	sets, err := c.Sets()
	if err == nil && len(sets) > 0 {
		var names []string
		for _, s := range sets {
			names = append(names, s.Name)
		}
		fmt.Printf("\nsets: %s  (hat mcp set)\n", strings.Join(names, ", "))
	}
	return nil
}

func mcpCatalog(c *mcp.Config) error {
	ts, err := c.Catalog()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	cat := ""
	for _, t := range ts {
		if t.Category != cat {
			if cat != "" {
				fmt.Fprintln(w)
			}
			cat = t.Category
			fmt.Fprintln(w, strings.ToUpper(cat))
		}
		pub := t.Publisher
		if t.Custom {
			pub = "your catalog"
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", t.Name, t.Title, pub, t.Description)
	}
	w.Flush()
	fmt.Println("\nAdd one with: hat mcp add <name> --from <template>")
	fmt.Println("Add your team's own templates to ~/.hatrack/mcp/catalog.json (see the README).")
	return nil
}

func mcpAdd(c *mcp.Config, args []string) error {
	scope := scopeFlag{mcp.User}
	fs := newFlags("add", &scope)
	from := fs.String("from", "", "catalog template")
	url := fs.String("url", "", "server URL")
	transport := fs.String("transport", "http", "http or sse, for --url")
	force := fs.Bool("force", false, "replace a server of the same name")
	test := fs.Bool("test", false, "test the server once added")
	var vars, env, headers multi
	fs.Var(&vars, "var", "template value K=V")
	fs.Var(&env, "e", "environment variable K=V")
	fs.Var(&env, "env", "environment variable K=V")
	fs.Var(&headers, "H", `HTTP header "Key: value"`)
	fs.Var(&headers, "header", `HTTP header "Key: value"`)
	pos, command, err := parse(fs, args)
	if err != nil {
		return err
	}
	name := ""
	if len(pos) > 0 {
		name, pos = pos[0], pos[1:]
	}
	if len(command) == 0 {
		command = pos // hat mcp add name cmd arg: fine while no arg starts with "-"
	} else if len(pos) > 0 {
		return fmt.Errorf("unexpected %q before --", pos[0])
	}

	var sp mcp.Spec
	switch {
	case *from != "":
		if *url != "" || len(command) > 0 {
			return errors.New("--from cannot be combined with --url or a command")
		}
		t, err := c.Template(*from)
		if err != nil {
			return err
		}
		if name == "" {
			name = t.Name
		}
		vals, err := kv(vars, "=")
		if err != nil {
			return err
		}
		if err := ask(t, vals); err != nil {
			return err
		}
		if sp, err = t.Render(vals, runtime.GOOS); err != nil {
			return err
		}
		if len(env) > 0 || len(headers) > 0 {
			extraEnv, err := kv(env, "=")
			if err != nil {
				return err
			}
			extraHdr, err := kv(headers, ":")
			if err != nil {
				return err
			}
			sp.Env, sp.Headers = merge(sp.Env, extraEnv), merge(sp.Headers, extraHdr)
		}
		if t.Docs != "" {
			defer fmt.Println("docs:", t.Docs)
		}
	case *url != "":
		if len(command) > 0 {
			return errors.New("give --url or a command, not both")
		}
		if *transport != "http" && *transport != "sse" {
			return fmt.Errorf("bad --transport %q: http or sse", *transport)
		}
		h, err := kv(headers, ":")
		if err != nil {
			return err
		}
		sp = mcp.Spec{Type: *transport, URL: *url, Headers: h}
	case len(command) > 0:
		e, err := kv(env, "=")
		if err != nil {
			return err
		}
		sp = mcp.Spec{Type: "stdio", Command: command[0], Args: command[1:], Env: e}
	default:
		return errors.New("usage: hat mcp add <name> --from <template> | --url <url> | -- <command> [args…]")
	}
	if name == "" {
		return errors.New("name the server: hat mcp add <name> …")
	}
	if err := c.Add(name, scope.s, sp, *force); err != nil {
		return err
	}
	fmt.Printf("added %s-scope MCP server %s: %s\n", scope.s, name, sp.Redacted().Target())
	for _, issue := range mcp.Check(mcp.Server{Name: name, Scope: scope.s, Enabled: true, Spec: sp}) {
		fmt.Println("  warning:", issue)
	}
	if *test {
		return testOne(name, sp, 90*time.Second)
	}
	fmt.Printf("Check it starts with: hat mcp test %s\n", name)
	return nil
}

// ask prompts on the terminal for template values not given with --var.
func ask(t mcp.Template, vals map[string]string) error {
	interactive := term.IsTerminal(int(os.Stdin.Fd()))
	in := bufio.NewReader(os.Stdin)
	first := true
	for _, v := range t.Vars {
		if vals[v.Name] != "" || !interactive {
			continue
		}
		if first {
			fmt.Printf("%s needs a few values (or pass them with --var NAME=value):\n", t.Title)
			first = false
		}
		prompt := v.Prompt
		if v.Default != "" {
			prompt += " [" + v.Default + "]"
		}
		fmt.Printf("  %s: ", prompt)
		var line string
		if v.Secret {
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return err
			}
			line = string(b)
		} else {
			s, err := in.ReadString('\n')
			if err != nil && s == "" {
				return errors.New("cancelled")
			}
			line = s
		}
		vals[v.Name] = strings.TrimSpace(line)
	}
	return nil
}

func mcpShow(c *mcp.Config, args []string) error {
	var reveal bool
	name, scope, err := nameAndScope("show", args, func(fs *flag.FlagSet) {
		fs.BoolVar(&reveal, "reveal", false, "print secrets")
	})
	if err != nil {
		return err
	}
	s, err := c.Find(name, scope)
	if err != nil {
		return err
	}
	state := "on"
	if !s.Enabled {
		state = "off"
	}
	fmt.Printf("%s  (%s scope, %s)\n", s.Name, s.Scope, state)
	if reveal {
		b, err := atomicfile.JSON(s.Raw)
		if err != nil {
			return err
		}
		os.Stdout.Write(b)
	} else {
		b, err := atomicfile.JSON(s.Spec.Redacted())
		if err != nil {
			return err
		}
		os.Stdout.Write(b)
	}
	for _, issue := range mcp.Check(s) {
		fmt.Println("warning:", issue)
	}
	return nil
}

func mcpToggle(c *mcp.Config, args []string, on bool) error {
	cmd := map[bool]string{true: "on", false: "off"}[on]
	name, scope, err := nameAndScope(cmd, args, nil)
	if err != nil {
		return err
	}
	if on {
		err = c.Enable(name, scope)
	} else {
		err = c.Disable(name, scope)
	}
	if err != nil {
		return err
	}
	fmt.Printf("%s is %s. Restart Claude (VS Code: Reload Window) to pick it up.\n", name, cmd)
	return nil
}

func mcpTest(c *mcp.Config, args []string) error {
	var scope scopeFlag
	fs := newFlags("test", &scope)
	timeout := fs.Duration("timeout", 90*time.Second, "how long to wait for each server")
	pos, _, err := parse(fs, args)
	if err != nil {
		return err
	}
	var targets []mcp.Server
	if len(pos) > 0 {
		for _, n := range pos {
			s, err := c.Find(n, scope.s)
			if err != nil {
				return err
			}
			targets = append(targets, s)
		}
	} else {
		all, err := c.List()
		if err != nil {
			return err
		}
		for _, s := range all {
			if s.Enabled && !s.Shadowed && (scope.s == "" || s.Scope == scope.s) {
				targets = append(targets, s)
			}
		}
	}
	if len(targets) == 0 {
		fmt.Println("no MCP servers switched on")
		return nil
	}
	failed := 0
	for _, s := range targets {
		if err := testOne(s.Name, s.Spec, *timeout); err != nil {
			fmt.Fprintf(os.Stderr, "  %v\n", err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d servers failed", failed, len(targets))
	}
	return nil
}

func testOne(name string, sp mcp.Spec, timeout time.Duration) error {
	fmt.Printf("%s: starting %s …\n", name, shorten(sp.Redacted().Target(), 60))
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	info, err := mcp.Probe(ctx, sp)
	if err != nil {
		fmt.Printf("✗ %s failed\n", name)
		return err
	}
	fmt.Printf("✓ %s: %s %s, MCP %s, %d tools, %s\n", name, info.Server, info.Version, info.Protocol, len(info.Tools), time.Since(start).Round(100*time.Millisecond))
	for _, t := range info.Tools {
		fmt.Printf("    %-32s %s\n", t.Name, shorten(firstLine(t.Description), 80))
	}
	return nil
}

func mcpDoctor(c *mcp.Config) error {
	ss, err := c.List()
	if err != nil {
		return err
	}
	bad := 0
	for _, s := range ss {
		if !s.Enabled {
			continue
		}
		issues := mcp.Check(s)
		if len(issues) == 0 {
			fmt.Printf("✓ %s (%s)\n", s.Name, s.Scope)
			continue
		}
		bad++
		fmt.Printf("✗ %s (%s)\n", s.Name, s.Scope)
		for _, i := range issues {
			fmt.Println("    " + i)
		}
	}
	if bad > 0 {
		return fmt.Errorf("%d servers need attention", bad)
	}
	fmt.Println("No problems found. `hat mcp test` starts each server to be sure.")
	return nil
}

func mcpExport(c *mcp.Config, args []string) error {
	scope := scopeFlag{mcp.User}
	fs := newFlags("export", &scope)
	out := fs.String("o", "", "file to write; standard output if not given")
	if _, _, err := parse(fs, args); err != nil {
		return err
	}
	b, err := c.Export(scope.s)
	if err != nil {
		return err
	}
	if *out == "" {
		_, err = os.Stdout.Write(b)
		return err
	}
	if err := atomicfile.Write(*out, b); err != nil {
		return err
	}
	fmt.Printf("wrote %s-scope servers to %s. It holds their secrets: keep it private.\n", scope.s, *out)
	return nil
}

func mcpImport(c *mcp.Config, args []string) error {
	scope := scopeFlag{mcp.User}
	fs := newFlags("import", &scope)
	desktop := fs.Bool("desktop", false, "import from Claude Desktop's config")
	force := fs.Bool("force", false, "replace servers of the same name")
	pos, _, err := parse(fs, args)
	if err != nil {
		return err
	}
	var path string
	switch {
	case *desktop && len(pos) == 0:
		if path, err = mcp.DesktopConfig(); err != nil {
			return err
		}
	case !*desktop && len(pos) == 1:
		path = pos[0]
	default:
		return errors.New("usage: hat mcp import <file> | --desktop [-s scope] [--force]")
	}
	added, skipped, err := c.Import(path, scope.s, *force)
	for _, n := range added {
		fmt.Println("added", n)
	}
	for _, n := range skipped {
		fmt.Printf("skipped %s: already exists (--force replaces it)\n", n)
	}
	return err
}

func mcpSet(c *mcp.Config, args []string) error {
	cmd := "ls"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch {
	case cmd == "ls" || cmd == "list":
		sets, err := c.Sets()
		if err != nil {
			return err
		}
		if len(sets) == 0 {
			fmt.Println("no sets yet: switch on the servers you want together, then `hat mcp set save <set>`")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SET\tACCOUNT\tSERVERS")
		for _, s := range sets {
			acct := s.Account
			if acct == "" {
				acct = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, acct, strings.Join(s.Servers, ", "))
		}
		return w.Flush()
	case cmd == "save" && len(args) >= 1:
		if err := c.SaveSet(args[0], args[1:]); err != nil {
			return err
		}
		fmt.Println("saved set", args[0])
		return nil
	case cmd == "use" && len(args) == 1:
		return useSet(c, args[0])
	case (cmd == "rm" || cmd == "remove") && len(args) == 1:
		if err := c.RemoveSet(args[0]); err != nil {
			return err
		}
		fmt.Println("removed set", args[0])
		return nil
	case cmd == "link" && len(args) == 2:
		if err := c.LinkSet(args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("`hat use %s` now also uses set %s\n", args[1], args[0])
		return nil
	case cmd == "unlink" && len(args) == 1:
		if err := c.LinkSet(args[0], ""); err != nil {
			return err
		}
		fmt.Println("unlinked set", args[0])
		return nil
	}
	return fmt.Errorf("unknown set command\n\n%s", mcpUsage)
}

func useSet(c *mcp.Config, name string) error {
	changed, err := c.UseSet(name)
	if err != nil {
		return err
	}
	if len(changed) == 0 {
		fmt.Printf("MCP set %s: nothing to change\n", name)
		return nil
	}
	fmt.Printf("MCP set %s: %s\n", name, strings.Join(changed, " "))
	return nil
}

// useLinkedSet applies the MCP set linked to an account hat just switched to.
func useLinkedSet(account string) error {
	c, err := mcp.Default("")
	if err != nil {
		return err
	}
	set, ok, err := c.SetFor(account)
	if err != nil || !ok {
		return err
	}
	return useSet(c, set.Name)
}

func kv(items []string, sep string) (map[string]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	m := map[string]string{}
	for _, it := range items {
		k, v, ok := strings.Cut(it, sep)
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("bad value %q: want KEY%sVALUE", it, sep)
		}
		if sep == ":" {
			v = strings.TrimSpace(v)
		}
		m[k] = v
	}
	return m, nil
}

func merge(a, b map[string]string) map[string]string {
	if len(b) == 0 {
		return a
	}
	if a == nil {
		a = map[string]string{}
	}
	for k, v := range b {
		a[k] = v
	}
	return a
}

func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}
