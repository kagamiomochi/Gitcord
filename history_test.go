package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The test binary is registered as git's editor during history rewrites, so it
// has to behave like main() when invoked that way.
func TestMain(m *testing.M) {
	if ok, err := runEditor(os.Args[1:]); ok {
		if err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestEditTodo(t *testing.T) {
	todo := "pick abc1234 first\npick def5678 second\n# comment\n"
	got := editTodo("fixup", "def56789aaaa", todo)
	want := "pick abc1234 first\nfixup def5678 second\n# comment\n"
	if got != want {
		t.Fatalf("got %q", got)
	}
	if editTodo("drop", "000000000", todo) != todo {
		t.Fatal("unmatched target must leave the todo untouched")
	}
}

// subjects returns the commit subjects, newest first.
func subjects(t *testing.T, repo string) []string {
	t.Helper()
	out, err := git(repo, "log", "--format=%s")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(out), "\n")
}

func hashOf(t *testing.T, repo, rev string) string {
	t.Helper()
	out, err := git(repo, "rev-parse", rev)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}

// addCommit commits a new file so later commits never conflict with each other.
func addCommit(t *testing.T, repo, name string) {
	t.Helper()
	os.WriteFile(filepath.Join(repo, name), []byte(name+"\n"), 0o644)
	git(repo, "add", name)
	if _, err := git(repo, "commit", "-qm", name); err != nil {
		t.Fatal(err)
	}
}

func act(t *testing.T, ts *httptest.Server, repo, hash, action, msg string) (int, string) {
	t.Helper()
	code, b := post(t, ts, "action", map[string]string{"Repo": repo, "Hash": hash, "Action": action, "Message": msg})
	return code, string(b)
}

func TestRewordHeadKeepsStaged(t *testing.T) {
	ts, repo := newTestServer(t)
	os.WriteFile(filepath.Join(repo, "staged.txt"), []byte("s\n"), 0o644)
	git(repo, "add", "staged.txt")
	if code, b := act(t, ts, repo, "HEAD", "reword", "renamed"); code != 200 {
		t.Fatalf("reword: %s", b)
	}
	if s := subjects(t, repo); s[0] != "renamed" || len(s) != 2 {
		t.Fatalf("subjects = %v", s)
	}
	// The staged file must still be staged, not folded into the amended commit
	if out, _ := git(repo, "diff", "--cached", "--name-only"); strings.TrimSpace(out) != "staged.txt" {
		t.Fatalf("staged changes lost: %q", out)
	}
}

func TestRewordOlderCommit(t *testing.T) {
	ts, repo := newTestServer(t)
	addCommit(t, repo, "c3")
	target := hashOf(t, repo, "HEAD~1") // c2
	if code, b := act(t, ts, repo, target, "reword", "feat: better c2"); code != 200 {
		t.Fatalf("reword: %s", b)
	}
	if s := subjects(t, repo); strings.Join(s, ",") != "c3,feat: better c2,c1" {
		t.Fatalf("subjects = %v", s)
	}
}

func TestRewordRootCommit(t *testing.T) {
	ts, repo := newTestServer(t)
	root := hashOf(t, repo, "HEAD~1")
	if code, b := act(t, ts, repo, root, "reword", "initial"); code != 200 {
		t.Fatalf("reword root: %s", b)
	}
	if s := subjects(t, repo); s[1] != "initial" || s[0] != "c2" {
		t.Fatalf("subjects = %v", s)
	}
}

func TestDropCommit(t *testing.T) {
	ts, repo := newTestServer(t)
	addCommit(t, repo, "c3")
	addCommit(t, repo, "c4")
	if code, b := act(t, ts, repo, hashOf(t, repo, "HEAD~1"), "drop", ""); code != 200 {
		t.Fatalf("drop: %s", b)
	}
	if s := subjects(t, repo); strings.Join(s, ",") != "c4,c2,c1" {
		t.Fatalf("subjects = %v", s)
	}
	if _, err := os.Stat(filepath.Join(repo, "c3")); err == nil {
		t.Fatal("dropped commit's file is still present")
	}
}

func TestDropKeepsDirtyTree(t *testing.T) {
	ts, repo := newTestServer(t)
	addCommit(t, repo, "c3")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\ntwo\nlocal\n"), 0o644)
	if code, b := act(t, ts, repo, hashOf(t, repo, "HEAD"), "drop", ""); code != 200 {
		t.Fatalf("drop: %s", b)
	}
	b, _ := os.ReadFile(filepath.Join(repo, "a.txt"))
	if string(b) != "one\ntwo\nlocal\n" {
		t.Fatalf("uncommitted edit lost: %q", b)
	}
}

func TestFixupCommit(t *testing.T) {
	ts, repo := newTestServer(t)
	addCommit(t, repo, "c3")
	addCommit(t, repo, "c4")
	if code, b := act(t, ts, repo, hashOf(t, repo, "HEAD~1"), "fixup", ""); code != 200 {
		t.Fatalf("fixup: %s", b)
	}
	if s := subjects(t, repo); strings.Join(s, ",") != "c4,c2,c1" {
		t.Fatalf("subjects = %v", s)
	}
	// c3's content must survive inside c2
	if _, err := os.Stat(filepath.Join(repo, "c3")); err != nil {
		t.Fatal("folded commit's file is gone")
	}
}

func TestRewriteGuards(t *testing.T) {
	ts, repo := newTestServer(t)
	root := hashOf(t, repo, "HEAD~1")
	if code, _ := act(t, ts, repo, root, "drop", ""); code != 400 {
		t.Error("dropping the root commit must be refused")
	}
	if code, _ := act(t, ts, repo, root, "fixup", ""); code != 400 {
		t.Error("fixup of the root commit must be refused")
	}
	if code, _ := act(t, ts, repo, "HEAD", "reword", "  "); code != 400 {
		t.Error("an empty message must be refused")
	}
	if code, _ := act(t, ts, repo, "nonexistent", "drop", ""); code != 400 {
		t.Error("an unknown commit must be refused")
	}
}

// A conflicting rewrite must leave the repository exactly as it was.
func TestConflictAbortsRebase(t *testing.T) {
	ts, repo := newTestServer(t)
	// c2 changed a.txt; c3 changes it again, so dropping c2 conflicts with c3
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\ntwo\nthree\n"), 0o644)
	git(repo, "commit", "-qam", "c3")
	before := hashOf(t, repo, "HEAD")
	code, b := act(t, ts, repo, hashOf(t, repo, "HEAD~1"), "drop", "")
	if code != 400 || !strings.Contains(b, "元に戻しました") {
		t.Fatalf("expected a rolled-back failure, got %d %s", code, b)
	}
	if hashOf(t, repo, "HEAD") != before {
		t.Fatal("HEAD moved after a failed rewrite")
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "rebase-merge")); err == nil {
		t.Fatal("rebase left in progress")
	}
}

func TestRevertConflictAborts(t *testing.T) {
	ts, repo := newTestServer(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\ntwo\nthree\n"), 0o644)
	git(repo, "commit", "-qam", "c3")
	before := hashOf(t, repo, "HEAD")
	if code, _ := act(t, ts, repo, hashOf(t, repo, "HEAD~1"), "revert", ""); code != 400 {
		t.Fatal("conflicting revert should fail")
	}
	if hashOf(t, repo, "HEAD") != before {
		t.Fatal("HEAD moved")
	}
	if out, _ := git(repo, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Fatalf("revert left the tree dirty:\n%s", out)
	}
}

func TestRevertMergeCommit(t *testing.T) {
	ts, repo := newTestServer(t)
	git(repo, "checkout", "-q", "-b", "side")
	addCommit(t, repo, "side.txt")
	git(repo, "checkout", "-q", "main")
	addCommit(t, repo, "main.txt")
	if _, err := git(repo, "merge", "--no-ff", "-qm", "merge side", "side"); err != nil {
		t.Fatal(err)
	}
	if code, b := act(t, ts, repo, "HEAD", "revert", ""); code != 200 {
		t.Fatalf("revert merge: %s", b)
	}
	if _, err := os.Stat(filepath.Join(repo, "side.txt")); err == nil {
		t.Fatal("merge was not undone")
	}
	if _, err := os.Stat(filepath.Join(repo, "main.txt")); err != nil {
		t.Fatal("first-parent side must be kept")
	}
}

func TestRevertNoCommit(t *testing.T) {
	ts, repo := newTestServer(t)
	before := hashOf(t, repo, "HEAD")
	if code, b := act(t, ts, repo, "HEAD", "revert-n", ""); code != 200 {
		t.Fatalf("revert-n: %s", b)
	}
	if hashOf(t, repo, "HEAD") != before {
		t.Fatal("revert-n must not create a commit")
	}
	if out, _ := git(repo, "diff", "--cached", "--name-only"); strings.TrimSpace(out) != "a.txt" {
		t.Fatalf("undo should be staged, got %q", out)
	}
}

// The reflog is the safety net: a hard reset can be walked back with restore.
func TestRestoreFromReflog(t *testing.T) {
	ts, repo := newTestServer(t)
	before := hashOf(t, repo, "HEAD")
	if code, b := act(t, ts, repo, "HEAD~1", "hard", ""); code != 200 {
		t.Fatalf("hard reset: %s", b)
	}
	_, b := get(t, ts, "reflog?repo="+repo)
	var entries []reflogEntry
	if err := json.Unmarshal(b, &entries); err != nil || len(entries) < 3 {
		t.Fatalf("reflog = %s (%v)", b, err)
	}
	if entries[0].Hash == before || entries[1].Hash != before {
		t.Fatalf("unexpected reflog order: %+v", entries)
	}
	if code, b := act(t, ts, repo, entries[1].Hash, "restore", ""); code != 200 {
		t.Fatalf("restore: %s", b)
	}
	if hashOf(t, repo, "HEAD") != before {
		t.Fatal("HEAD was not restored")
	}
}

func TestForcePush(t *testing.T) {
	ts, repo := newTestServer(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	if _, err := git("", "init", "-q", "--bare", remote); err != nil {
		t.Fatal(err)
	}
	git(repo, "remote", "add", "origin", remote)
	if code, b := post(t, ts, "push", map[string]any{"Repo": repo}); code == 200 {
		// no upstream yet, plain push should fail
		t.Fatalf("expected failure without upstream, got %s", b)
	}
	git(repo, "push", "-q", "-u", "origin", "main")
	if code, b := act(t, ts, repo, "HEAD", "reword", "rewritten"); code != 200 {
		t.Fatalf("reword: %s", b)
	}
	if code, _ := post(t, ts, "push", map[string]any{"Repo": repo}); code != 400 {
		t.Fatal("plain push after a rewrite must be rejected")
	}
	if code, b := post(t, ts, "push", map[string]any{"Repo": repo, "Force": true}); code != 200 {
		t.Fatalf("force push: %s", b)
	}
	if got := hashOf(t, remote, "main"); got != hashOf(t, repo, "HEAD") {
		t.Fatal("remote was not updated")
	}
}

func TestFixupIntoRootCommit(t *testing.T) {
	ts, repo := newTestServer(t)
	if code, b := act(t, ts, repo, "HEAD", "fixup", ""); code != 200 {
		t.Fatalf("fixup into root: %s", b)
	}
	if s := subjects(t, repo); strings.Join(s, ",") != "c1" {
		t.Fatalf("subjects = %v", s)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(b) != "one\ntwo\n" {
		t.Fatalf("folded content lost: %q", b)
	}
}

func TestMessageKeepsBody(t *testing.T) {
	ts, repo := newTestServer(t)
	if code, b := act(t, ts, repo, "HEAD", "reword", "subject\n\nbody line"); code != 200 {
		t.Fatalf("reword: %s", b)
	}
	_, b := get(t, ts, "message?repo="+repo+"&rev=HEAD")
	var msg string
	json.Unmarshal(b, &msg)
	if msg != "subject\n\nbody line" {
		t.Fatalf("message = %q", msg)
	}
	if code, _ := get(t, ts, "message?repo="+repo+"&rev=--output=x"); code != 400 {
		t.Error("option-like rev must be rejected")
	}
}
