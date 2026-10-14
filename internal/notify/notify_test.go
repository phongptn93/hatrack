package notify

import (
	"strings"
	"testing"
)

func TestQuoting(t *testing.T) {
	if got := appleString(`a "b" \c`); got != `"a \"b\" \\c"` {
		t.Fatalf("apple: %s", got)
	}
	if got := psString("it's"); got != "'it''s'" {
		t.Fatalf("ps: %s", got)
	}
	c := command("darwin", `T"`, "B")
	if c == nil || !strings.Contains(c.Args[2], `with title "T\""`) {
		t.Fatalf("darwin: %v", c)
	}
	c = command("windows", "T", "it's 85%")
	if c == nil || !strings.Contains(c.Args[len(c.Args)-1], "'it''s 85%'") {
		t.Fatalf("windows: %v", c)
	}
}
