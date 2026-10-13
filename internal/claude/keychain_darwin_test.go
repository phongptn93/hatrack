package claude

import (
	"os"
	"strings"
	"testing"
)

// Uses the real login keychain, so it only runs when asked: HATRACK_KEYCHAIN_TEST=1 go test ./...
func TestMacKeychain(t *testing.T) {
	if os.Getenv("HATRACK_KEYCHAIN_TEST") == "" {
		t.Skip("set HATRACK_KEYCHAIN_TEST=1 to use the real keychain")
	}
	k := systemKeychain()
	service := keychainService(t.TempDir())
	t.Cleanup(func() { k.Delete(service) })

	if b, err := k.Get(service); err != nil || b != nil {
		t.Fatalf("missing item: %q %v", b, err)
	}
	for _, v := range []string{`{"claudeAiOauth":{"refreshToken":"r1"}}`, `{"x":"` + strings.Repeat("y", 3000) + `"}`} {
		must(t, k.Set(service, []byte(v)))
		b, err := k.Get(service)
		must(t, err)
		if string(b) != v {
			t.Fatalf("read back %q", b)
		}
	}
	must(t, k.Delete(service))
	must(t, k.Delete(service))
	if b, _ := k.Get(service); b != nil {
		t.Fatalf("deleted item still reads %q", b)
	}
}
