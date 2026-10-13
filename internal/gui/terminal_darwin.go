package gui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OpenAddInTerminal runs `hat add [name]` in a new Terminal window. It goes through a
// .command file, which Terminal opens on its own, so macOS asks for no automation rights.
func OpenAddInTerminal(name string) error {
	hat, err := findHat()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "hatrack-add-")
	if err != nil {
		return err
	}
	script := filepath.Join(dir, "add.command")
	cmd := shellQuote(hat) + " add"
	if name != "" {
		cmd += " " + shellQuote(name)
	}
	body := "#!/bin/sh\nrm -rf " + shellQuote(dir) + "\nclear\n" + cmd + "\n" +
		`printf '\nPress Enter to close this window.'; read _` + "\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		os.RemoveAll(dir)
		return err
	}
	if err := exec.Command("open", "-a", "Terminal", script).Run(); err != nil {
		os.RemoveAll(dir)
		return err
	}
	return nil
}

// findHat prefers the hat shipped next to this binary (inside Hatrack.app), since an app
// started from Finder does not get the shell's PATH.
func findHat() (string, error) {
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "hat")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("hat"); err == nil {
		return p, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".local", "bin", "hat")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("hat not found: install it, or run `hat add` in a terminal")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
