package tui

import (
	"os"
	"strings"
	"testing"
)

// Every key in the table is documented in the user guide's key table, so
// the docs can't fall behind the code (Home and End once did).
func TestKeysDocumented(t *testing.T) {
	data, err := os.ReadFile("../../docs/user-guide.md")
	if err != nil {
		t.Fatal(err)
	}
	guide := string(data)
	sec := strings.Index(guide, "## The Interactive View")
	start := strings.Index(guide[max(sec, 0):], "| Key ")
	if sec < 0 || start < 0 {
		t.Fatal("no key table under The Interactive View in docs/user-guide.md")
	}
	start += sec
	end := strings.Index(guide[start:], "\n\n")
	table := guide[start : start+end]
	for _, b := range bindings() {
		for _, tok := range strings.Fields(b.show) {
			if !strings.Contains(table, "`"+tok+"`") {
				t.Errorf("key %q (%s) is missing from the user guide's key table", tok, b.label)
			}
		}
	}
}

// No key is bound twice.
func TestKeysUnique(t *testing.T) {
	seen := map[key]string{}
	for _, b := range bindings() {
		for _, k := range b.keys {
			if prev, ok := seen[k]; ok {
				t.Errorf("%v bound to both %q and %q", k, prev, b.label)
			}
			seen[k] = b.label
		}
	}
}
