// Package usage reads the token counts Claude Code records in its transcripts
// (~/.claude/projects/<folder>/<session>.jsonl), so usage can be summed by day, folder, model,
// account and the 5-hour windows subscription limits are counted in. Nothing leaves the machine.
package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry is one model response.
type Entry struct {
	Time       time.Time
	Project    string // the folder Claude ran in
	Model      string
	Session    string
	Account    string // filled in by the caller from hat's switch history
	Input      int64
	Output     int64
	CacheWrite int64
	CacheRead  int64
}

func (e Entry) Total() int64 { return e.Input + e.Output + e.CacheWrite + e.CacheRead }

type line struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Cwd       string    `json:"cwd"`
	SessionID string    `json:"sessionId"`
	RequestID string    `json:"requestId"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// Scan reads every transcript under dir and returns the responses from since on, oldest first.
// A response Claude wrote more than once (one line per content block) is counted once.
func Scan(dir string, since time.Time) ([]Entry, error) {
	seen := map[string]bool{}
	var out []Entry
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if info, err := d.Info(); err != nil || info.ModTime().Before(since) {
			return nil // not written to since: nothing new in it
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		return scanFile(f, since, seen, &out)
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, err
}

var usageKey = []byte(`"usage"`)

func scanFile(r io.Reader, since time.Time, seen map[string]bool, out *[]Entry) error {
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		b, err := br.ReadBytes('\n')
		if len(b) > 0 && bytes.Contains(b, usageKey) {
			var l line
			if json.Unmarshal(b, &l) == nil && l.Type == "assistant" && l.Message.Usage != nil &&
				l.Message.Model != "<synthetic>" && !l.Timestamp.Before(since) {
				key := l.Message.ID + "/" + l.RequestID
				if l.Message.ID == "" || !seen[key] {
					seen[key] = true
					u := l.Message.Usage
					*out = append(*out, Entry{Time: l.Timestamp, Project: l.Cwd, Model: l.Message.Model, Session: l.SessionID,
						Input: u.Input, Output: u.Output, CacheWrite: u.CacheWrite, CacheRead: u.CacheRead})
				}
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// Row sums the entries that share a key.
type Row struct {
	Key        string
	Responses  int
	Input      int64
	Output     int64
	CacheWrite int64
	CacheRead  int64
}

func (r Row) Total() int64 { return r.Input + r.Output + r.CacheWrite + r.CacheRead }

func (r *Row) add(e Entry) {
	r.Responses++
	r.Input += e.Input
	r.Output += e.Output
	r.CacheWrite += e.CacheWrite
	r.CacheRead += e.CacheRead
}

// Group sums entries by key, sorted by key.
func Group(es []Entry, key func(Entry) string) []Row {
	m := map[string]*Row{}
	for _, e := range es {
		k := key(e)
		if m[k] == nil {
			m[k] = &Row{Key: k}
		}
		m[k].add(e)
	}
	out := make([]Row, 0, len(m))
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Sum adds up all entries.
func Sum(es []Entry) Row {
	var r Row
	for _, e := range es {
		r.add(e)
	}
	return r
}

// BlockLen is the window Claude subscriptions count usage limits in.
const BlockLen = 5 * time.Hour

// Block is one 5-hour usage window: it starts on the hour of its first response and ends 5 hours
// later, or earlier windows are closed by a 5-hour gap. This mirrors how the limits reset; the
// exact reset time is known only to Anthropic.
type Block struct {
	Start, End time.Time
	Last       time.Time // the latest response in it
	Row
}

// Active reports whether the window is still open at now.
func (b Block) Active(now time.Time) bool { return now.Before(b.End) && now.Sub(b.Last) < BlockLen }

// Blocks splits entries, oldest first, into 5-hour windows.
func Blocks(es []Entry) []Block {
	var out []Block
	for _, e := range es {
		if n := len(out); n == 0 || !e.Time.Before(out[n-1].End) || e.Time.Sub(out[n-1].Last) >= BlockLen {
			start := e.Time.Truncate(time.Hour)
			out = append(out, Block{Start: start, End: start.Add(BlockLen)})
		}
		b := &out[len(out)-1]
		b.Last = e.Time
		b.add(e)
	}
	return out
}
