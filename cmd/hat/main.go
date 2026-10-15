// Command hat switches the account Claude Code is signed in to, without signing out, and
// manages the MCP servers Claude Code loads.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/baodq97/hatrack/internal/claude"
	"github.com/baodq97/hatrack/internal/mcp"
)

const helpText = `hat - switch Claude Code accounts and manage its MCP servers

  hat               list accounts (* = active)
  hat use <name>    switch to an account
  hat add [name]    sign in another account; the active one is untouched
  hat save [name]   keep the account Claude is signed in to now
  hat rm <name>     forget an account
  hat mcp …         manage MCP servers (databases, Azure DevOps…): hat mcp help
  hat usage         tokens used by day, project, model, account; the 5-hour window
  hat statusline install   show your limits in Claude Code and warn before you hit them
  hat doctor        check Claude Code's setup: login, files, MCP servers
  hat clean         forget deleted folders Claude still remembers (asks first)
  hat version       print the version, and say when a newer one is out
  hat update        update hat (and the Windows tray app) to the latest release

Names default to the account's email.`

// noBrowser marks hat standing in as claude's BROWSER when the user picks "show me the link":
// it opens nothing, so claude prints the link and asks for the code instead.
const noBrowser = "HATRACK_NO_BROWSER"

var version = "dev" // set by the release build

func main() {
	if os.Getenv(noBrowser) != "" {
		os.Exit(1) // "browser didn't open": claude falls back to printing the link
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "hat:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "mcp" {
		return runMCP(args[1:])
	}
	if len(args) > 0 && args[0] == "update" {
		return selfUpdate(args[1:])
	}
	s, err := claude.Default()
	if err != nil {
		return err
	}
	cmd, arg := "ls", ""
	if len(args) > 0 {
		cmd = args[0]
	}
	if len(args) > 1 {
		arg = args[1]
	}
	switch {
	case cmd == "ls" || cmd == "list":
		ps, err := s.List()
		if err != nil {
			return err
		}
		if len(ps) == 0 {
			fmt.Println("no accounts yet: run `hat save` to keep the current one, `hat add` for another")
		}
		for _, p := range ps {
			mark := " "
			if p.Active {
				mark = "*"
			}
			plan := ""
			if p.Plan != "" {
				plan = "  [" + p.Plan + "]"
			}
			fmt.Printf("%s %-30s %s  %s%s\n", mark, p.Name, p.Email, p.Org, plan)
		}
		return nil
	case cmd == "use" && arg != "":
		set, changed, err := mcp.SwitchAccount(s, arg)
		if err != nil && set == "" {
			return err
		}
		fmt.Printf("now using %s. If an open Claude session still shows the old account, restart it (VS Code: Reload Window).\n", arg)
		if err != nil {
			return fmt.Errorf("switched account, but MCP set %s: %w", set, err)
		}
		if set != "" {
			fmt.Printf("MCP set %s: %s\n", set, strings.Join(append([]string{}, changed...), " "))
		}
		return nil
	case cmd == "add":
		return add(s, arg)
	case cmd == "save":
		name, err := s.Save(arg)
		if err != nil {
			return err
		}
		fmt.Println("saved", name)
		return nil
	case (cmd == "rm" || cmd == "remove") && arg != "":
		if err := s.Remove(arg); err != nil {
			return err
		}
		fmt.Println("removed", arg)
		return nil
	case cmd == "statusline":
		return statusline(s, args[1:])
	case cmd == "usage":
		return runUsage(s, args[1:])
	case cmd == "doctor":
		return doctor(s)
	case cmd == "clean":
		return clean(s, args[1:])
	case cmd == "version" || cmd == "--version":
		return printVersion()
	case cmd == "-h" || cmd == "--help" || cmd == "help":
		fmt.Println(helpText)
		return nil
	}
	return fmt.Errorf("unknown command\n\n%s", helpText)
}

const addPrompt = `Add a Claude account

Sign-in happens in a separate folder; the account you use now stays signed in.

  [1] Open the sign-in page in my default browser
  [2] Show me the link - I'll open it in the right browser profile myself

Choose 1 or 2: `

func add(s *claude.Store, name string) error {
	fmt.Print(addPrompt)
	var choice string
	fmt.Scanln(&choice)
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, claude.SecureStorageEnv+"=") {
			env = append(env, kv)
		}
	}
	switch choice {
	case "1":
	case "2":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		env = append(env, "BROWSER="+exe, noBrowser+"=1")
		fmt.Println("\nOpen the link below in the browser profile of the account to add, sign in,\nthen paste the code it shows back here.")
	default:
		return errors.New("cancelled")
	}
	fmt.Println()
	ps, _ := s.List()
	added, err := s.Add(name, func(dir string) error {
		c := exec.Command("claude", "auth", "login")
		c.Env = append(env, "CLAUDE_CONFIG_DIR="+dir)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		return runLogin(c)
	})
	if err != nil {
		return err
	}
	fmt.Printf("\nAdded %s.\n", added)
	for _, p := range ps {
		if p.Active {
			fmt.Printf("You are still using %s. ", p.Name)
		}
	}
	fmt.Printf("Switch with: hat use %s\n", added)
	return nil
}
