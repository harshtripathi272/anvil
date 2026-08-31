package main

import (
	"bytes"
	"testing"
)

func TestPrefixEnd(t *testing.T) {
	cases := []struct {
		prefix string
		want   []byte // nil means unbounded above
	}{
		{"user:", []byte("user;")},
		{"a", []byte("b")},
		{"az", []byte("a{")},
		{"\xff", nil},          // all 0xff has no successor
		{"\xff\xff", nil},      //
		{"a\xff", []byte("b")}, // carry past the trailing 0xff
	}
	for _, c := range cases {
		got := prefixEnd([]byte(c.prefix))
		if !bytes.Equal(got, c.want) {
			t.Errorf("prefixEnd(%q) = %q, want %q", c.prefix, got, c.want)
		}
	}
}

// TestPrefixEndBoundsScan checks the property the CLI actually relies on: every
// key carrying the prefix sorts before the returned end key, and the next key
// after the prefix range does not.
func TestPrefixEndBoundsScan(t *testing.T) {
	prefix := []byte("user:")
	end := prefixEnd(prefix)
	inside := [][]byte{[]byte("user:"), []byte("user:1"), []byte("user:\xff\xff")}
	outside := [][]byte{[]byte("user;"), []byte("usez"), []byte("v")}

	for _, k := range inside {
		if bytes.Compare(k, end) >= 0 {
			t.Errorf("%q should sort before end %q", k, end)
		}
	}
	for _, k := range outside {
		if bytes.Compare(k, end) < 0 {
			t.Errorf("%q should not sort before end %q", k, end)
		}
	}
}

// TestCommandTable guards the help output: every visible command needs a group
// and a summary, and no name may be registered twice.
func TestCommandTable(t *testing.T) {
	seen := map[string]bool{}
	groups := map[string]bool{groupData: true, groupVerify: true, groupBench: true}

	for _, c := range commands() {
		if c.name == "" {
			t.Fatal("command with empty name")
		}
		if seen[c.name] {
			t.Errorf("duplicate command %q", c.name)
		}
		seen[c.name] = true
		if c.run == nil {
			t.Errorf("command %q has no implementation", c.name)
		}
		if c.hidden {
			continue
		}
		if c.summary == "" {
			t.Errorf("command %q has no summary", c.name)
		}
		if !groups[c.group] {
			t.Errorf("command %q has unknown group %q", c.name, c.group)
		}
	}

	for _, required := range []string{"create", "put", "get", "delete", "scan", "info", "check", "verify", "torture", "bench"} {
		if !seen[required] {
			t.Errorf("missing command %q", required)
		}
	}
}
