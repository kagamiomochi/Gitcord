package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseTags(t *testing.T) {
	const sep = "\x1f"
	row := func(f ...string) string { return strings.Join(f, sep) }
	out := strings.Join([]string{
		// annotated tag pointing at a commit
		row("refs/tags/v2", "tag", "tagobj", "commit", "c2", "200", "release two"),
		// lightweight tag: the commit subject must not be mistaken for a tag message
		row("refs/tags/v1", "commit", "c1", "", "", "100", "first commit"),
		// annotated tag pointing at a tree has no commit to jump to
		row("refs/tags/tree", "tag", "tagobj2", "tree", "t1", "50", "odd"),
		"garbage",
	}, "\n")
	got := parseTags(out)
	want := []tagInfo{
		{Name: "v2", Hash: "c2", Annotated: true, Message: "release two", Time: "200"},
		{Name: "v1", Hash: "c1", Time: "100"},
		{Name: "tree", Annotated: true, Message: "odd", Time: "50"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseTags = %+v, want %+v", got, want)
	}
}

func TestParseRemoteTags(t *testing.T) {
	out := "aaa\trefs/tags/v1\nbbb\trefs/tags/release/2\n\nccc\trefs/heads/main\n"
	got := parseRemoteTags(out)
	if !reflect.DeepEqual(got, []string{"v1", "release/2"}) {
		t.Fatalf("parseRemoteTags = %v", got)
	}
}

// tagList fetches the tag list through the API.
func tagList(t *testing.T, repo string, get func(string) (int, []byte)) []tagInfo {
	t.Helper()
	code, b := get("tags?repo=" + repo)
	if code != 200 {
		t.Fatalf("tags: %d %s", code, b)
	}
	var ts []tagInfo
	if err := json.Unmarshal(b, &ts); err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestTagCreateListDelete(t *testing.T) {
	ts, repo := newTestServer(t)
	getf := func(p string) (int, []byte) { return get(t, ts, p) }
	head := hashOf(t, repo, "HEAD")
	first := hashOf(t, repo, "HEAD~1")

	// Lightweight tag on HEAD (empty hash defaults to HEAD)
	if code, b := post(t, ts, "tagadd", map[string]string{"Repo": repo, "Name": "v1"}); code != 200 {
		t.Fatalf("lightweight: %s", b)
	}
	// Annotated tag on an older commit
	if code, b := post(t, ts, "tagadd", map[string]string{"Repo": repo, "Name": "v0", "Hash": first, "Message": "first release"}); code != 200 {
		t.Fatalf("annotated: %s", b)
	}
	byName := map[string]tagInfo{}
	for _, tg := range tagList(t, repo, getf) {
		byName[tg.Name] = tg
	}
	if v := byName["v1"]; v.Hash != head || v.Annotated || v.Message != "" {
		t.Fatalf("v1 = %+v", v)
	}
	if v := byName["v0"]; v.Hash != first || !v.Annotated || v.Message != "first release" {
		t.Fatalf("v0 = %+v", v)
	}

	// The log carries the tags so the UI can badge commits
	_, b := get(t, ts, "log?repo="+repo)
	var log struct{ Tags map[string][]string }
	if err := json.Unmarshal(b, &log); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(log.Tags[head], []string{"v1"}) || !reflect.DeepEqual(log.Tags[first], []string{"v0"}) {
		t.Fatalf("log tags = %v", log.Tags)
	}

	// Duplicates and bad input are rejected
	for name, body := range map[string]map[string]string{
		"duplicate":   {"Repo": repo, "Name": "v1"},
		"empty":       {"Repo": repo, "Name": "  "},
		"option-like": {"Repo": repo, "Name": "--force"},
		"bad chars":   {"Repo": repo, "Name": "a b"},
		"bad dots":    {"Repo": repo, "Name": "a..b"},
		"bad target":  {"Repo": repo, "Name": "x", "Hash": "--hard"},
		"no commit":   {"Repo": repo, "Name": "x", "Hash": "deadbeef"},
	} {
		if code, _ := post(t, ts, "tagadd", body); code != 400 {
			t.Errorf("%s: expected 400, got %d", name, code)
		}
	}
	if n := len(tagList(t, repo, getf)); n != 2 {
		t.Fatalf("rejected requests must not create tags, have %d", n)
	}

	if code, b := post(t, ts, "tagdel", map[string]any{"Repo": repo, "Name": "v1"}); code != 200 {
		t.Fatalf("delete: %s", b)
	}
	if n := len(tagList(t, repo, getf)); n != 1 {
		t.Fatalf("expected 1 tag after delete, have %d", n)
	}
	if code, _ := post(t, ts, "tagdel", map[string]any{"Repo": repo, "Name": "--all"}); code != 400 {
		t.Error("option-like name must be rejected on delete")
	}
}

func TestTagPushAndRemoteDelete(t *testing.T) {
	ts, repo := newTestServer(t)
	getf := func(p string) (int, []byte) { return get(t, ts, p) }

	// Without a remote every network operation fails cleanly
	if code, _ := post(t, ts, "tagpush", map[string]string{"Repo": repo, "Name": "v1"}); code != 400 {
		t.Fatal("push without a remote must fail")
	}
	if code, _ := get(t, ts, "tagremote?repo="+repo); code != 400 {
		t.Fatal("remote listing without a remote must fail")
	}

	remote := filepath.Join(t.TempDir(), "remote.git")
	if _, err := git("", "init", "-q", "--bare", remote); err != nil {
		t.Fatal(err)
	}
	git(repo, "remote", "add", "origin", remote)
	post(t, ts, "tagadd", map[string]string{"Repo": repo, "Name": "v1"})
	post(t, ts, "tagadd", map[string]string{"Repo": repo, "Name": "v2", "Message": "two"})

	remoteTags := func() []string {
		code, b := get(t, ts, "tagremote?repo="+repo)
		if code != 200 {
			t.Fatalf("tagremote: %d %s", code, b)
		}
		var names []string
		json.Unmarshal(b, &names)
		return names
	}
	if n := remoteTags(); len(n) != 0 {
		t.Fatalf("remote should start empty, got %v", n)
	}

	// Push a single tag, then all of them
	if code, b := post(t, ts, "tagpush", map[string]string{"Repo": repo, "Name": "v1"}); code != 200 {
		t.Fatalf("push one: %s", b)
	}
	if n := remoteTags(); !reflect.DeepEqual(n, []string{"v1"}) {
		t.Fatalf("remote tags = %v", n)
	}
	if code, b := post(t, ts, "tagpush", map[string]string{"Repo": repo}); code != 200 {
		t.Fatalf("push all: %s", b)
	}
	if n := remoteTags(); len(n) != 2 {
		t.Fatalf("remote tags = %v", n)
	}

	// Remote delete leaves the local tag alone
	if code, b := post(t, ts, "tagdel", map[string]any{"Repo": repo, "Name": "v1", "Remote": true}); code != 200 {
		t.Fatalf("remote delete: %s", b)
	}
	if n := remoteTags(); !reflect.DeepEqual(n, []string{"v2"}) {
		t.Fatalf("remote tags after delete = %v", n)
	}
	if n := len(tagList(t, repo, getf)); n != 2 {
		t.Fatalf("local tags must be kept, have %d", n)
	}
}
