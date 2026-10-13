// Package claude parks and swaps Claude Code logins so one machine can switch accounts
// without signing out. A login is the credentials file plus the oauthAccount block of
// .claude.json; everything else (settings, history, projects) stays shared.
package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/baodq97/hatrack/internal/atomicfile"
)

const (
	credsFile   = ".credentials.json"
	accountKey  = "oauthAccount"
	parkedCreds = "credentials.json"
	parkedAcct  = "account.json"
)

var (
	ErrNotSignedIn = errors.New("claude is not signed in")
	validName      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@+-]{0,63}$`)
)

// Store knows where Claude reads its login and where parked logins live.
type Store struct {
	ConfigDir  string // holds .credentials.json
	GlobalJSON string // .claude.json, holds oauthAccount
	Dir        string // parked logins, one folder per profile
}

type Profile struct {
	Name   string
	Email  string
	Org    string
	Active bool
}

type account struct {
	AccountUUID      string `json:"accountUuid"`
	OrganizationUUID string `json:"organizationUuid"`
	EmailAddress     string `json:"emailAddress"`
	OrganizationName string `json:"organizationName"`
}

func (a account) same(b account) bool {
	return a.AccountUUID != "" && a.AccountUUID == b.AccountUUID && a.OrganizationUUID == b.OrganizationUUID
}

// Default follows Claude Code's own lookup: CLAUDE_CONFIG_DIR if set, else ~/.claude and ~/.claude.json.
func Default() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	s := &Store{
		ConfigDir:  filepath.Join(home, ".claude"),
		GlobalJSON: filepath.Join(home, ".claude.json"),
		Dir:        filepath.Join(home, ".hatrack", "claude"),
	}
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		s.ConfigDir, s.GlobalJSON = d, filepath.Join(d, ".claude.json")
	}
	return s, nil
}

// List returns parked profiles sorted by name, marking the one Claude is signed in to.
func (s *Store) List() ([]Profile, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_, cur, curErr := readAccount(s.GlobalJSON)
	var out []Profile
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		a, err := s.parkedAccount(e.Name())
		if err != nil {
			continue // half-written or foreign folder
		}
		out = append(out, Profile{Name: e.Name(), Email: a.EmailAddress, Org: a.OrganizationName, Active: curErr == nil && a.same(cur)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Save parks the login Claude is signed in to now. An empty name means the account's email.
func (s *Store) Save(name string) (string, error) {
	return s.park(name, filepath.Join(s.ConfigDir, credsFile), s.GlobalJSON)
}

// Add signs a new account in through login, run with CLAUDE_CONFIG_DIR set to a scratch
// folder, so the active login is never replaced. An empty name means the account's email.
func (s *Store) Add(name string, login func(configDir string) error) (string, error) {
	if name != "" && !validName.MatchString(name) {
		return "", fmt.Errorf("bad profile name %q", name)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", err
	}
	// an add killed mid-login (window closed, Ctrl+C) skips its cleanup; sweep it now
	if old, err := filepath.Glob(filepath.Join(s.Dir, ".login-*")); err == nil {
		for _, d := range old {
			os.RemoveAll(d)
		}
	}
	tmp, err := os.MkdirTemp(s.Dir, ".login-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := login(tmp); err != nil {
		return "", fmt.Errorf("sign-in: %w", err)
	}
	return s.park(name, filepath.Join(tmp, credsFile), filepath.Join(tmp, ".claude.json"))
}

// Use makes name the login Claude reads. The current login is parked first, so tokens
// Claude refreshed since the last switch are kept and an unparked login is never lost.
func (s *Store) Use(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("bad profile name %q", name)
	}
	dir := filepath.Join(s.Dir, name)
	if _, err := s.parkedAccount(name); err != nil {
		return fmt.Errorf("profile %q: %w", name, err)
	}
	// park first: when name is the active login, this is what refreshes its parked tokens
	if err := s.parkCurrent(); err != nil {
		return fmt.Errorf("park current login: %w", err)
	}
	creds, err := os.ReadFile(filepath.Join(dir, parkedCreds))
	if err != nil {
		return fmt.Errorf("profile %q: %w", name, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, parkedAcct))
	if err != nil {
		return fmt.Errorf("profile %q: %w", name, err)
	}
	if err := writeAtomic(filepath.Join(s.ConfigDir, credsFile), creds); err != nil {
		return err
	}
	return setAccount(s.GlobalJSON, raw)
}

func (s *Store) Remove(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("bad profile name %q", name)
	}
	dir := filepath.Join(s.Dir, name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("profile %q: %w", name, err)
	}
	return os.RemoveAll(dir)
}

// parkCurrent writes the active login back into its profile, creating one if none matches.
func (s *Store) parkCurrent() error {
	_, cur, err := readAccount(s.GlobalJSON)
	if errors.Is(err, ErrNotSignedIn) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(s.ConfigDir, credsFile)); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	ps, err := s.List()
	if err != nil {
		return err
	}
	name := ""
	for _, p := range ps {
		if p.Active {
			name = p.Name
			break
		}
	}
	if name == "" {
		name = s.freeName(cur)
	}
	_, err = s.Save(name)
	return err
}

func (s *Store) park(name, credsPath, jsonPath string) (string, error) {
	creds, err := os.ReadFile(credsPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotSignedIn
	}
	if err != nil {
		return "", err
	}
	raw, acct, err := readAccount(jsonPath)
	if err != nil {
		return "", err
	}
	if name == "" {
		name = s.freeName(acct)
	}
	if !validName.MatchString(name) {
		return "", fmt.Errorf("bad profile name %q", name)
	}
	if old, err := s.parkedAccount(name); err == nil && !old.same(acct) {
		return "", fmt.Errorf("profile %q already holds %s; pick another name", name, old.EmailAddress)
	}
	// one copy per account: refresh tokens rotate, so a second copy would silently go stale
	if entries, err := os.ReadDir(s.Dir); err == nil {
		for _, e := range entries {
			if old, err := s.parkedAccount(e.Name()); err == nil && e.Name() != name && old.same(acct) {
				return "", fmt.Errorf("%s is already saved as %q", acct.EmailAddress, e.Name())
			}
		}
	}
	dir := filepath.Join(s.Dir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := writeAtomic(filepath.Join(dir, parkedCreds), creds); err != nil {
		return "", err
	}
	return name, writeAtomic(filepath.Join(dir, parkedAcct), raw)
}

// freeName is the account's email, suffixed when that name already holds another org's login.
func (s *Store) freeName(a account) string {
	name := a.EmailAddress
	if !validName.MatchString(name) {
		name = "account"
	}
	for i := 1; ; i++ {
		n := name
		if i > 1 {
			n = fmt.Sprintf("%s-%d", name, i)
		}
		old, err := s.parkedAccount(n)
		if err != nil || old.same(a) {
			return n
		}
	}
}

func (s *Store) parkedAccount(name string) (account, error) {
	var a account
	b, err := os.ReadFile(filepath.Join(s.Dir, name, parkedAcct))
	if err != nil {
		return a, err
	}
	return a, json.Unmarshal(b, &a)
}

func readAccount(path string) (json.RawMessage, account, error) {
	var a account
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, a, ErrNotSignedIn
	}
	if err != nil {
		return nil, a, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, a, fmt.Errorf("%s: %w", path, err)
	}
	raw, ok := m[accountKey]
	if !ok || string(raw) == "null" {
		return nil, a, ErrNotSignedIn
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, a, fmt.Errorf("%s: %w", path, err)
	}
	return raw, a, nil
}

// setAccount replaces oauthAccount in .claude.json and keeps every other key as it was.
func setAccount(path string, raw json.RawMessage) error {
	m := map[string]json.RawMessage{}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	m[accountKey] = raw
	b, err = atomicfile.JSON(m)
	if err != nil {
		return err
	}
	return writeAtomic(path, b)
}

var writeAtomic = atomicfile.Write
