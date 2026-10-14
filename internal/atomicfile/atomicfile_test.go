package atomicfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditRedoesWhenChangedAndKeepsBackups(t *testing.T) {
	dir := t.TempDir()
	path, backups := filepath.Join(dir, "f.json"), filepath.Join(dir, "bk")
	os.WriteFile(path, []byte("v1"), 0o600)
	calls := 0
	err := Edit(path, backups, 2, func(old []byte) ([]byte, error) {
		calls++
		if calls == 1 { // another program writes while we work
			os.WriteFile(path, []byte("v2 from claude"), 0o600)
		}
		return append(old, " +edit"...), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "v2 from claude +edit" || calls != 2 {
		t.Fatalf("got %q after %d calls", b, calls)
	}
	for i := range 3 {
		if err := Edit(path, backups, 2, func(old []byte) ([]byte, error) { return append(old, byte('a'+i)), nil }); err != nil {
			t.Fatal(err)
		}
	}
	bk, _ := filepath.Glob(filepath.Join(backups, "f.json.*"))
	if len(bk) != 2 {
		t.Fatalf("backups: %v", bk)
	}
	if b, _ := os.ReadFile(bk[1]); !strings.HasSuffix(string(b), "+editab") {
		t.Fatalf("newest backup: %q", b)
	}
}
