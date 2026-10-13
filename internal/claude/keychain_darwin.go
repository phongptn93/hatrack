package claude

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
)

const (
	security       = "/usr/bin/security"
	errItemMissing = 44 // errSecItemNotFound
	stdinLineLimit = 4032
)

// macKeychain drives /usr/bin/security the way Claude does, so the items it writes stay
// readable by Claude without a keychain prompt.
type macKeychain struct{ account string }

func systemKeychain() Keychain { return macKeychain{account: keychainAccount()} }

// keychainAccount matches Claude's: $USER, else the login name, else a fixed fallback.
func keychainAccount() string {
	name := os.Getenv("USER")
	if name == "" {
		if u, err := user.Current(); err == nil {
			name = u.Username
		}
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9._-]+$`).MatchString(name) {
		return "claude-code-user"
	}
	return name
}

func (k macKeychain) Get(service string) ([]byte, error) {
	out, err := run(nil, "find-generic-password", "-a", k.account, "-s", service, "-w")
	if exitCode(err) == errItemMissing {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out = bytes.TrimRight(out, "\r\n")
	if len(out) == 0 {
		return nil, nil
	}
	// security prints data that is not plain text as hex
	if out[0] != '{' {
		if b, err := hex.DecodeString(string(out)); err == nil {
			out = b
		}
	}
	return out, nil
}

// Set writes through `security -i` so the tokens never show up in the process list.
func (k macKeychain) Set(service string, data []byte) error {
	args := []string{"add-generic-password", "-U", "-a", k.account, "-s", service, "-X", hex.EncodeToString(data)}
	line := strings.Join(quote(args), " ") + "\n"
	var err error
	if len(line) <= stdinLineLimit {
		_, err = run(strings.NewReader(line), "-i")
	} else {
		_, err = run(nil, args...) // too long for interactive mode; Claude does the same
	}
	if err != nil {
		return err
	}
	// security -i can exit 0 after a failed command, so read it back
	got, err := k.Get(service)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, data) {
		return fmt.Errorf("keychain: %q was not updated", service)
	}
	return nil
}

func (k macKeychain) Delete(service string) error {
	_, err := run(nil, "delete-generic-password", "-a", k.account, "-s", service)
	if exitCode(err) == errItemMissing {
		return nil
	}
	return err
}

func run(stdin *strings.Reader, args ...string) ([]byte, error) {
	c := exec.Command(security, args...)
	if stdin != nil {
		c.Stdin = stdin
	}
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("keychain %s: %s: %w", args[0], msg, err)
		}
		return nil, fmt.Errorf("keychain %s: %w", args[0], err)
	}
	return out, nil
}

func quote(args []string) []string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = `"` + a + `"`
	}
	return q
}

func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 0
}
