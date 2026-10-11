package main

import (
	"errors"
	"os"
	"strings"
)

// History rewriting. Reword / drop / fixup of an arbitrary commit are done with
// a non-interactive `git rebase -i`: Gitcord's own binary is registered as
// GIT_SEQUENCE_EDITOR / GIT_EDITOR and edits the todo list or message file.
// This needs no shell scripts or temp files and works on any platform.

const (
	todoEditorFlag = "--gc-todo" // --gc-todo <verb> <full-hash> <todo-file>
	msgEditorFlag  = "--gc-msg"  // --gc-msg <message-file> <target-file>
)

// shQuote quotes s for use inside the shell command git builds for editors.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// editTodo rewrites the verb of the single todo line that belongs to target.
func editTodo(verb, target, todo string) string {
	lines := strings.Split(todo, "\n")
	for i, l := range lines {
		f := strings.Fields(l)
		if len(f) < 2 || (f[0] != "pick" && f[0] != "p") {
			continue
		}
		// The todo list uses abbreviated hashes, so match by prefix
		if len(f[1]) >= 4 && strings.HasPrefix(target, f[1]) {
			lines[i] = verb + strings.TrimPrefix(l, f[0])
			break
		}
	}
	return strings.Join(lines, "\n")
}

// runEditor handles the editor invocations; it reports whether args were one.
func runEditor(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case todoEditorFlag:
		if len(args) != 4 {
			return true, errors.New("bad arguments")
		}
		b, err := os.ReadFile(args[3])
		if err != nil {
			return true, err
		}
		return true, os.WriteFile(args[3], []byte(editTodo(args[1], args[2], string(b))), 0o644)
	case msgEditorFlag:
		if len(args) != 3 {
			return true, errors.New("bad arguments")
		}
		b, err := os.ReadFile(args[1])
		if err != nil {
			return true, err
		}
		return true, os.WriteFile(args[2], b, 0o644)
	}
	return false, nil
}

// resolveCommit returns the full hash of rev, which must be a commit.
func resolveCommit(repo, rev string) (string, error) {
	if err := checkRev(rev); err != nil {
		return "", err
	}
	out, err := git(repo, "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", errors.New("コミットが見つかりません")
	}
	return strings.TrimSpace(out), nil
}

func isMerge(repo, hash string) bool {
	ps, _ := git(repo, "rev-list", "--parents", "-n", "1", hash)
	return len(strings.Fields(ps)) > 2
}

func hasParent(repo, hash string) bool {
	ps, _ := git(repo, "rev-list", "--parents", "-n", "1", hash)
	return len(strings.Fields(ps)) > 1
}

// rebaseEdit applies verb (reword, drop or fixup) to one commit of the current
// branch and replays the commits after it. On any failure the rebase is aborted
// so the repository is never left in the middle of an operation.
func rebaseEdit(repo, rev, verb, msg string) (string, error) {
	hash, err := resolveCommit(repo, rev)
	if err != nil {
		return "", err
	}
	if _, err := git(repo, "merge-base", "--is-ancestor", hash, "HEAD"); err != nil {
		return "", errors.New("現在のブランチの履歴にないコミットです")
	}
	if !hasParent(repo, hash) && verb != "reword" {
		return "", errors.New("最初のコミットには使えません")
	}
	if isMerge(repo, hash) {
		return "", errors.New("マージコミットには使えません")
	}
	// The todo list must start at the commit itself, except for fixup, which
	// needs its parent in the list as the commit to fold into.
	start := hash
	if verb == "fixup" {
		start = hash + "^"
		if isMerge(repo, start) {
			return "", errors.New("直前がマージコミットのため、まとめられません")
		}
	}
	root := !hasParent(repo, start)
	rng := start + "^..HEAD"
	if root {
		rng = "HEAD"
	}
	if m, _ := git(repo, "rev-list", "--merges", rng); strings.TrimSpace(m) != "" {
		return "", errors.New("対象以降にマージコミットがあるため、書き換えできません")
	}

	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	env := []string{
		"GIT_SEQUENCE_EDITOR=" + shQuote(exe) + " " + todoEditorFlag + " " + verb + " " + hash,
		"GIT_EDITOR=true",
	}
	if verb == "reword" {
		f, err := os.CreateTemp("", "gitcord-msg-*")
		if err != nil {
			return "", err
		}
		defer os.Remove(f.Name())
		f.WriteString(strings.TrimSpace(msg) + "\n")
		f.Close()
		env[1] = "GIT_EDITOR=" + shQuote(exe) + " " + msgEditorFlag + " " + shQuote(f.Name())
	}

	args := []string{"rebase", "-i", "--autostash"}
	if root {
		args = append(args, "--root")
	} else {
		args = append(args, start+"^")
	}
	out, err := gitEnv(repo, env, args...)
	if err != nil {
		git(repo, "rebase", "--abort")
		return "", errors.New("書き換えに失敗したため、元に戻しました:\n" + err.Error())
	}
	return out, nil
}

// revertCommit makes an undo commit (or only applies the undo when noCommit is set).
// Merge commits are reverted relative to their first parent. A conflict aborts the revert.
func revertCommit(repo, rev string, noCommit bool) (string, error) {
	hash, err := resolveCommit(repo, rev)
	if err != nil {
		return "", err
	}
	args := []string{"revert"}
	if noCommit {
		args = append(args, "--no-commit")
	} else {
		args = append(args, "--no-edit")
	}
	if isMerge(repo, hash) {
		args = append(args, "-m", "1")
	}
	out, err := git(repo, append(args, hash)...)
	if err != nil {
		git(repo, "revert", "--abort")
		return "", errors.New("打ち消しに失敗したため、元に戻しました:\n" + err.Error())
	}
	return out, nil
}

// reflogEntry is one position HEAD has been at.
type reflogEntry struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

const reflogFormat = "--format=%H%x1f%gs%x1f%ct"

func parseReflog(out string) []reflogEntry {
	res := []reflogEntry{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(l, "\x1f")
		if len(f) == 3 {
			res = append(res, reflogEntry{f[0], f[1], f[2]})
		}
	}
	return res
}
