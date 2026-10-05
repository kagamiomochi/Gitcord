package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// newTestServer returns a server backed by a temporary store and a repo with two commits.
func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")

	repo := filepath.Join(dir, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}} {
		if _, err := git(repo, args...); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "one\n")
	git(repo, "add", ".")
	git(repo, "commit", "-qm", "c1")
	write("a.txt", "one\ntwo\n")
	git(repo, "commit", "-qam", "c2")

	s := &server{
		repos: &repoStore{path: filepath.Join(dir, "repos.json")},
		ui:    fstest.MapFS{"index.html": {Data: []byte("ok")}},
	}
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	return ts, repo
}

func post(t *testing.T, ts *httptest.Server, path string, body any) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(ts.URL+"/api/"+path, "application/json", strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func get(t *testing.T, ts *httptest.Server, path string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(ts.URL + "/api/" + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func TestRepoLifecycle(t *testing.T) {
	ts, repo := newTestServer(t)
	if code, _ := post(t, ts, "add", map[string]string{"Path": repo}); code != 200 {
		t.Fatalf("add: %d", code)
	}
	post(t, ts, "add", map[string]string{"Path": repo}) // duplicate is a no-op
	_, b := get(t, ts, "repos")
	var repos []string
	json.Unmarshal(b, &repos)
	if len(repos) != 1 || repos[0] != repo {
		t.Fatalf("repos = %v", repos)
	}
	if code, _ := post(t, ts, "add", map[string]string{"Path": t.TempDir()}); code != 400 {
		t.Fatalf("adding a non-git dir should fail, got %d", code)
	}
	post(t, ts, "remove", map[string]string{"Path": repo})
	_, b = get(t, ts, "repos")
	if strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("repos after remove = %s", b)
	}
}

func TestLogAndChanges(t *testing.T) {
	ts, repo := newTestServer(t)
	_, b := get(t, ts, "log?repo="+repo)
	var log struct {
		Commits  []commit
		Unpushed []string
	}
	if err := json.Unmarshal(b, &log); err != nil || len(log.Commits) != 2 {
		t.Fatalf("log = %s (%v)", b, err)
	}
	if log.Commits[0].Subject != "c2" || len(log.Unpushed) != 2 {
		t.Fatalf("unexpected log: %+v", log)
	}
	_, b = get(t, ts, "changes?repo="+repo+"&rev="+log.Commits[0].Hash)
	var ch []fileChange
	json.Unmarshal(b, &ch)
	if len(ch) != 1 || ch[0].Path != "a.txt" || ch[0].Status != "M" {
		t.Fatalf("changes = %s", b)
	}
}

func TestStageAndCommit(t *testing.T) {
	ts, repo := newTestServer(t)
	os.WriteFile(filepath.Join(repo, "n.txt"), []byte("n\n"), 0o644)
	post(t, ts, "stage", map[string]any{"Repo": repo, "Paths": []string{"n.txt"}, "Stage": true})
	_, b := get(t, ts, "status?repo="+repo)
	var st []statusEntry
	json.Unmarshal(b, &st)
	if len(st) != 1 || !st[0].Staged {
		t.Fatalf("status = %s", b)
	}
	if code, _ := post(t, ts, "commit", map[string]string{"Repo": repo, "Message": "feat: n"}); code != 200 {
		t.Fatalf("commit failed: %d", code)
	}
	if _, b = get(t, ts, "status?repo="+repo); strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("tree should be clean, got %s", b)
	}
}

func TestRejectsOptionLikeInput(t *testing.T) {
	ts, repo := newTestServer(t)
	if code, _ := get(t, ts, "file?repo="+repo+"&rev=--output=/tmp/x&path=a"); code != 400 {
		t.Errorf("option-like rev should be rejected, got %d", code)
	}
	if code, _ := post(t, ts, "action", map[string]string{"Repo": repo, "Hash": "--hard", "Action": "revert"}); code != 400 {
		t.Errorf("option-like hash should be rejected, got %d", code)
	}
	code, _ := post(t, ts, "clone", map[string]string{"URL": "--upload-pack=touch /tmp/pwned", "Dir": t.TempDir(), "Name": "x"})
	if code != 400 {
		t.Errorf("option-like clone URL should fail, got %d", code)
	}
	if _, err := os.Stat("/tmp/pwned"); err == nil {
		t.Error("clone executed an injected command")
	}
}
