package mcp

import (
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Spec is the part of a server config hatrack reads. Writes keep the config Claude has.
type Spec struct {
	Type    string            `json:"type,omitempty"` // stdio, http or sse
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Kind is the transport, guessed the way Claude does when type is left out.
func (s Spec) Kind() string {
	if s.Type != "" {
		return s.Type
	}
	if s.URL != "" {
		return "http"
	}
	return "stdio"
}

// Target is a one-line summary: the command line or the URL.
func (s Spec) Target() string {
	if s.Kind() == "stdio" {
		return strings.TrimSpace(s.Command + " " + strings.Join(s.Args, " "))
	}
	return s.URL
}

// Redacted hides env and header values and URL passwords, for printing. ${VAR} references stay.
func (s Spec) Redacted() Spec {
	r := s
	r.Args = make([]string, len(s.Args))
	for i, a := range s.Args {
		r.Args[i] = redactURL(a)
	}
	r.URL = redactURL(s.URL)
	r.Env = redactMap(s.Env)
	r.Headers = redactMap(s.Headers)
	return r
}

const hidden = "****"

func redactMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		rest := strings.TrimSpace(strings.TrimPrefix(envRef.ReplaceAllString(v, ""), "Bearer"))
		switch {
		case rest == "": // empty, or only ${VAR} references: nothing secret to hide
			out[k] = v
		case !secretKey.MatchString(k):
			out[k] = redactURL(v) // a host, port or flag: worth seeing, bar a password in a URL
		case strings.HasPrefix(v, "Bearer "):
			out[k] = "Bearer " + hidden
		default:
			out[k] = hidden
		}
	}
	return out
}

// secretKey matches env and header names whose values are secrets, or may hold one.
var secretKey = regexp.MustCompile(`(?i)pass|pwd|secret|token|key|auth|cred|cookie|session|dsn|uri|url|conn`)

var userinfo = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*://[^:/@\s]*):([^/\s]+)@`) // the password runs to the last @

// redactURL hides the password in scheme://user:password@host, which connection strings carry.
func redactURL(s string) string {
	if m := userinfo.FindStringSubmatch(s); m != nil && !envRef.MatchString(m[2]) {
		return m[1] + ":" + hidden + "@" + s[len(m[0]):]
	}
	return s
}

// envRef matches ${VAR} and ${VAR:-default}, which Claude expands when it starts a server.
var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// Expand fills ${VAR} references from lookup, as Claude would.
func (s Spec) Expand(lookup func(string) (string, bool)) Spec {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	x := func(v string) string {
		return envRef.ReplaceAllStringFunc(v, func(ref string) string {
			m := envRef.FindStringSubmatch(ref)
			if val, ok := lookup(m[1]); ok && val != "" {
				return val
			}
			return m[2]
		})
	}
	return s.mapStrings(x)
}

// EnvRefs lists the ${VAR} names the spec reads that lookup cannot fill and that have no default.
func (s Spec) EnvRefs(lookup func(string) (string, bool)) []string {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	seen := map[string]bool{}
	s.mapStrings(func(v string) string {
		for _, m := range envRef.FindAllStringSubmatch(v, -1) {
			hasDefault := strings.Contains(m[0], ":-")
			if val, ok := lookup(m[1]); (!ok || val == "") && !hasDefault {
				seen[m[1]] = true
			}
		}
		return v
	})
	return sortedKeys(seen)
}

var placeholder = regexp.MustCompile(`\{\{([A-Za-z0-9_]+)\}\}`)

// Placeholders lists catalog {{VARS}} still left in the spec.
func (s Spec) Placeholders() []string {
	seen := map[string]bool{}
	s.mapStrings(func(v string) string {
		for _, m := range placeholder.FindAllStringSubmatch(v, -1) {
			seen[m[1]] = true
		}
		return v
	})
	return sortedKeys(seen)
}

func (s Spec) mapStrings(f func(string) string) Spec {
	r := s
	r.Command, r.URL = f(s.Command), f(s.URL)
	if s.Args != nil {
		r.Args = make([]string, len(s.Args))
		for i, a := range s.Args {
			r.Args[i] = f(a)
		}
	}
	mapMap := func(m map[string]string) map[string]string {
		if m == nil {
			return nil
		}
		out := make(map[string]string, len(m))
		for k, v := range m {
			out[k] = f(v)
		}
		return out
	}
	r.Env, r.Headers = mapMap(s.Env), mapMap(s.Headers)
	return r
}

func validURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
