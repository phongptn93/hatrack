package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Switch records that an account became the one Claude uses, so usage can be split by account.
type Switch struct {
	Time    time.Time `json:"time"`
	Account string    `json:"account"`
}

func (s *Store) historyPath() string { return filepath.Join(filepath.Dir(s.Dir), "switches.jsonl") }

// logSwitch appends to the history unless name is already the latest entry. It is a record,
// not part of the switch, so a failure to write it is ignored.
func (s *Store) logSwitch(name string) {
	if h, err := s.Switches(); err == nil && len(h) > 0 && h[len(h)-1].Account == name {
		return
	}
	b, err := json.Marshal(Switch{Time: time.Now().UTC(), Account: name})
	if err != nil {
		return
	}
	f, err := os.OpenFile(s.historyPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(b, '\n'))
}

// Switches returns the account history, oldest first.
func (s *Store) Switches() ([]Switch, error) {
	f, err := os.Open(s.historyPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Switch
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var w Switch
		if json.Unmarshal(sc.Bytes(), &w) == nil && w.Account != "" {
			out = append(out, w)
		}
	}
	return out, sc.Err()
}

// AccountAt returns which account was in use at t according to the history, or "".
func AccountAt(h []Switch, t time.Time) string {
	acct := ""
	for _, w := range h {
		if w.Time.After(t) {
			break
		}
		acct = w.Account
	}
	return acct
}

// renameInHistory rewrites the history so usage follows an account to its new name.
func (s *Store) renameInHistory(oldName, newName string) error {
	h, err := s.Switches()
	if err != nil || len(h) == 0 {
		return err
	}
	var buf []byte
	for _, w := range h {
		if w.Account == oldName {
			w.Account = newName
		}
		b, err := json.Marshal(w)
		if err != nil {
			return err
		}
		buf = append(append(buf, b...), '\n')
	}
	return writeAtomic(s.historyPath(), buf)
}
