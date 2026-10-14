package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/baodq97/hatrack/internal/atomicfile"
	"github.com/baodq97/hatrack/internal/claude"
	"github.com/baodq97/hatrack/internal/notify"
	"github.com/baodq97/hatrack/internal/usage"
)

// hat statusline is Claude Code's status line command. Claude passes it, on stdin, the share of
// the subscription limits used so far; hat records it per account, warns once when a limit
// passes its alert, and prints "work · 5h 62% ↻21:00 · 7d 31%". A status line the user had
// before keeps running, with hat's part after it.

func hatDir(s *claude.Store) string { return filepath.Dir(s.Dir) }

func statusline(s *claude.Store, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "install":
			return statuslineInstall(s)
		case "uninstall":
			return statuslineUninstall(s)
		}
		return errors.New("usage: hat statusline [install|uninstall]")
	}
	in, _ := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
	// a status line must always print something; errors only cost hat's part of it
	var parts []string
	if prev := previousStatusline(s); prev != "" {
		if out := runPrevious(prev, in); out != "" {
			parts = append(parts, out)
		}
	}
	if ours := ourStatus(s, in); ours != "" {
		parts = append(parts, ours)
	}
	fmt.Println(strings.Join(parts, "  "))
	return nil
}

func ourStatus(s *claude.Store, in []byte) string {
	cur, err := s.Current()
	if err != nil {
		return ""
	}
	account := cur.Name
	if account == "" {
		account = cur.Email
	}
	now := time.Now()
	_, fresh, _ := usage.Record(hatDir(s), account, in, now)
	for _, w := range fresh {
		notify.Show("Claude usage: "+w.Kind+" limit at "+strconv.Itoa(int(w.Percent))+"%", w.String())
	}
	l, err := usage.LoadLimits(hatDir(s))
	if err != nil {
		return account
	}
	if line, _ := l.Line(account, now); line != "" {
		return account + " · " + line
	}
	return account
}

// statuslineState remembers the status line hat replaced, to run it and to put it back.
type statuslineState struct {
	Previous json.RawMessage `json:"previous,omitempty"` // settings.json statusLine as it was
}

func statuslineStatePath(s *claude.Store) string { return filepath.Join(hatDir(s), "statusline.json") }

func previousStatusline(s *claude.Store) string {
	var st statuslineState
	b, err := os.ReadFile(statuslineStatePath(s))
	if err != nil || json.Unmarshal(b, &st) != nil || len(st.Previous) == 0 {
		return ""
	}
	var sl struct {
		Command string `json:"command"`
	}
	json.Unmarshal(st.Previous, &sl)
	return sl.Command
}

// runPrevious runs the user's own status line command the way Claude does, through a shell.
func runPrevious(command string, in []byte) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS != "windows":
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	default:
		if bash, err := exec.LookPath("bash"); err == nil { // Claude on Windows uses Git Bash
			cmd = exec.CommandContext(ctx, bash, "-c", command)
		} else {
			cmd = exec.CommandContext(ctx, "cmd", "/c", command)
		}
	}
	cmd.Stdin = bytes.NewReader(in)
	out, _ := cmd.Output()
	return strings.TrimRight(string(out), "\r\n")
}

func settingsPath(s *claude.Store) string { return filepath.Join(s.ConfigDir, "settings.json") }

func ourCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	// forward slashes and quotes work in sh, Git Bash and cmd alike
	return `"` + filepath.ToSlash(exe) + `" statusline`, nil
}

func isOurs(raw json.RawMessage) bool {
	var sl struct {
		Command string `json:"command"`
	}
	return json.Unmarshal(raw, &sl) == nil && strings.HasSuffix(sl.Command, `" statusline`)
}

func statuslineInstall(s *claude.Store) error {
	cmd, err := ourCommand()
	if err != nil {
		return err
	}
	var prev json.RawMessage
	err = atomicfile.Edit(settingsPath(s), s.BackupDir(), 10, func(b []byte) ([]byte, error) {
		m := map[string]json.RawMessage{}
		if len(bytes.TrimSpace(b)) > 0 {
			if err := json.Unmarshal(b, &m); err != nil {
				return nil, fmt.Errorf("%s: %w", settingsPath(s), err)
			}
		}
		if old, ok := m["statusLine"]; ok && !isOurs(old) {
			prev = old
		}
		raw, err := json.Marshal(map[string]any{"type": "command", "command": cmd, "padding": 0})
		if err != nil {
			return nil, err
		}
		m["statusLine"] = raw
		return atomicfile.JSON(m)
	})
	if err != nil {
		return err
	}
	if prev != nil {
		b, _ := atomicfile.JSON(statuslineState{Previous: prev})
		if err := atomicfile.Write(statuslineStatePath(s), b); err != nil {
			return err
		}
		fmt.Println("Your status line keeps running; hat's part shows after it.")
	}
	fmt.Printf("Claude Code's status line now shows your usage limits (set in %s).\n", tildePath(settingsPath(s)))
	fmt.Println("It updates while Claude Code runs in a terminal. hat warns once per window when a limit")
	fmt.Printf("passes its alert (%d%% for 5 hours, %d%% for the week): see `hat usage alert`.\n", usage.DefaultFiveHour, usage.DefaultSevenDay)
	return nil
}

func statuslineUninstall(s *claude.Store) error {
	var st statuslineState
	if b, err := os.ReadFile(statuslineStatePath(s)); err == nil {
		json.Unmarshal(b, &st)
	}
	err := atomicfile.Edit(settingsPath(s), s.BackupDir(), 10, func(b []byte) ([]byte, error) {
		m := map[string]json.RawMessage{}
		if len(bytes.TrimSpace(b)) == 0 {
			return b, nil
		}
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		if old, ok := m["statusLine"]; !ok || !isOurs(old) {
			return nil, errors.New("hat's status line is not installed")
		}
		if len(st.Previous) > 0 {
			m["statusLine"] = st.Previous
		} else {
			delete(m, "statusLine")
		}
		return atomicfile.JSON(m)
	})
	if err != nil {
		return err
	}
	os.Remove(statuslineStatePath(s))
	if len(st.Previous) > 0 {
		fmt.Println("Removed hat's status line; your own is back.")
	} else {
		fmt.Println("Removed hat's status line.")
	}
	return nil
}

func usageAlert(s *claude.Store, args []string) error {
	dir := hatDir(s)
	if len(args) == 0 {
		l, err := usage.LoadLimits(dir)
		if err != nil {
			return err
		}
		if l.Alert.Off {
			fmt.Println("Usage alerts are off. `hat usage alert on` turns them back on.")
			return nil
		}
		five, week := l.Alert.FiveHour, l.Alert.SevenDay
		if five == 0 {
			five = usage.DefaultFiveHour
		}
		if week == 0 {
			week = usage.DefaultSevenDay
		}
		fmt.Printf("hat warns at %.0f%% of the 5-hour limit and %.0f%% of the weekly one.\n", five, week)
		if !statuslineInstalled(s) {
			fmt.Println("It learns how much is used from Claude Code's status line: run `hat statusline install`.")
		}
		return nil
	}
	return usage.EditLimits(dir, func(l *usage.Limits) error {
		switch {
		case args[0] == "off":
			l.Alert.Off = true
		case args[0] == "on":
			l.Alert.Off = false
		default:
			five, err := percent(args[0])
			if err != nil {
				return err
			}
			l.Alert.FiveHour, l.Alert.Off = five, false
			if len(args) == 3 && args[1] == "--weekly" {
				if l.Alert.SevenDay, err = percent(args[2]); err != nil {
					return err
				}
			} else if len(args) != 1 {
				return errors.New("usage: hat usage alert [<5-hour %> [--weekly <%>] | on | off]")
			}
		}
		fmt.Println("saved")
		return nil
	})
}

func percent(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
	if err != nil || v <= 0 || v > 100 {
		return 0, fmt.Errorf("bad percentage %q: give 1 to 100", s)
	}
	return v, nil
}

func statuslineInstalled(s *claude.Store) bool {
	b, err := os.ReadFile(settingsPath(s))
	if err != nil {
		return false
	}
	var m map[string]json.RawMessage
	return json.Unmarshal(b, &m) == nil && isOurs(m["statusLine"])
}
