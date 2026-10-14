package mcp

import (
	"fmt"
	"os/exec"
	"strings"
)

var lookPath = exec.LookPath

// Check lists what would keep a server from starting, without starting it.
func Check(s Server) []string {
	var issues []string
	sp := s.Spec
	if s.Shadowed {
		issues = append(issues, "hidden by a server of the same name in a narrower scope (local > project > user)")
	}
	for _, p := range sp.Placeholders() {
		issues = append(issues, fmt.Sprintf("{{%s}} was never filled in", p))
	}
	for _, v := range sp.EnvRefs(nil) {
		issues = append(issues, fmt.Sprintf("${%s} is not set here; Claude must be started with it set", v))
	}
	x := sp.Expand(nil)
	switch sp.Kind() {
	case "stdio":
		if x.Command == "" {
			issues = append(issues, "no command")
			break
		}
		cmd := x.Command
		if strings.EqualFold(cmd, "cmd") && len(x.Args) > 1 && strings.EqualFold(x.Args[0], "/c") {
			cmd = x.Args[1] // Windows: cmd /c npx …
		}
		if _, err := lookPath(cmd); err != nil {
			issues = append(issues, fmt.Sprintf("%q is not on PATH%s", cmd, installHint(cmd)))
		}
	case "http", "sse":
		if !validURL(x.URL) {
			issues = append(issues, fmt.Sprintf("bad URL %q", sp.URL))
		}
		if sp.Kind() == "sse" {
			issues = append(issues, "the sse transport is being retired (Atlassian and Linear have dropped it); ask the provider for its streamable HTTP URL, usually ending in /mcp")
		}
	default:
		issues = append(issues, fmt.Sprintf("unknown type %q", sp.Type))
	}
	return issues
}

func installHint(cmd string) string {
	switch strings.ToLower(strings.TrimSuffix(cmd, ".exe")) {
	case "npx", "node", "npm":
		return ": install Node.js (https://nodejs.org)"
	case "uvx", "uv":
		return ": install uv (https://docs.astral.sh/uv/)"
	case "docker":
		return ": install Docker"
	}
	return ""
}
