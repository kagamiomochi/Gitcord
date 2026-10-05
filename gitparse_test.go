package main

import (
	"reflect"
	"testing"
)

func TestParseLog(t *testing.T) {
	out := "aaa\x1fAlice\x1f100\x1ffeat: x\nbbb\x1fBob\x1f200\x1ffix: y\n"
	want := []commit{{"aaa", "Alice", "100", "feat: x"}, {"bbb", "Bob", "200", "fix: y"}}
	if got := parseLog(out); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := parseLog(""); len(got) != 0 {
		t.Fatalf("empty log should give no commits, got %v", got)
	}
}

func TestParseNameStatus(t *testing.T) {
	out := "M\x00a.txt\x00R100\x00old.txt\x00new.txt\x00A\x00dir/b.txt\x00"
	want := []fileChange{
		{Path: "a.txt", Status: "M"},
		{Path: "new.txt", Old: "old.txt", Status: "R"},
		{Path: "dir/b.txt", Status: "A"},
	}
	if got := parseNameStatus(out); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseStatus(t *testing.T) {
	out := " M a.txt\x00A  b.txt\x00?? c.txt\x00R  new.txt\x00old.txt\x00"
	want := []statusEntry{
		{"a.txt", " ", "M", false},
		{"b.txt", "A", " ", true},
		{"c.txt", "?", "?", false},
		{"new.txt", "R", " ", true},
	}
	if got := parseStatus(out); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestCheckRev(t *testing.T) {
	for _, ok := range []string{"HEAD", "main", "abc123", "HEAD~1"} {
		if checkRev(ok) != nil {
			t.Errorf("%q should be accepted", ok)
		}
	}
	for _, bad := range []string{"--output=/tmp/x", "-n", "a\nb"} {
		if checkRev(bad) == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}
