// Package atomicfile writes files in one rename so a crash never leaves half a file.
package atomicfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Write replaces path with data, readable only by the user where the OS allows it.
func Write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// JSON encodes v indented, without escaping <, > and &, the way Claude writes its own files.
func JSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Edit rewrites path with fn's result. Claude rewrites its own files while it runs, so if path
// changes between the read and the write, the edit is redone on the new content rather than
// overwriting it. A missing file reads as empty. With backupDir set, the old content is copied
// there first, keeping the newest keep copies.
func Edit(path, backupDir string, keep int, fn func(old []byte) ([]byte, error)) error {
	for range 5 {
		old, before, err := read(path)
		if err != nil {
			return err
		}
		data, err := fn(old)
		if err != nil {
			return err
		}
		if _, after, err := read(path); err != nil {
			return err
		} else if after != before {
			continue // changed under us: start over from what is there now
		}
		if backupDir != "" && len(old) > 0 {
			if err := backup(path, backupDir, keep, old); err != nil {
				return fmt.Errorf("back up %s: %w", path, err)
			}
		}
		return Write(path, data)
	}
	return fmt.Errorf("%s keeps changing; close Claude and try again", path)
}

// read returns the file and a stamp that changes whenever the file does.
func read(path string) ([]byte, string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, "", err
	}
	return b, fmt.Sprintf("%d %d %x", st.Size(), st.ModTime().UnixNano(), sha256.Sum256(b)), nil
}

func backup(path, dir string, keep int, data []byte) error {
	base := strings.TrimPrefix(filepath.Base(path), ".") // a visible name: claude.json.<time>
	name := filepath.Join(dir, fmt.Sprintf("%s.%s", base, time.Now().Format("20060102-150405.000")))
	if err := Write(name, data); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(dir, base+".*"))
	sort.Strings(old) // timestamps sort oldest first
	for len(old) > keep {
		os.Remove(old[0])
		old = old[1:]
	}
	return nil
}
