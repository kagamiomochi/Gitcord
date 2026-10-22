package main

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Tag management: list, create (lightweight or annotated), push and delete.

// tagInfo is one tag. Hash is the commit it points at ("" when the tag points
// at something else, such as a tree).
type tagInfo struct {
	Name      string `json:"name"`
	Hash      string `json:"hash"`
	Annotated bool   `json:"annotated"`
	Message   string `json:"message"`
	Time      string `json:"time"`
}

// tagFormat must stay in sync with parseTags. The *-prefixed fields describe
// the object an annotated tag points at, and are empty for lightweight tags.
const tagFormat = "--format=%(refname)%1f%(objecttype)%1f%(objectname)%1f%(*objecttype)%1f%(*objectname)%1f%(creatordate:unix)%1f%(contents:subject)"

// parseTags parses `for-each-ref refs/tags` output produced with tagFormat.
func parseTags(out string) []tagInfo {
	res := []tagInfo{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(l, "\x1f")
		if len(f) != 7 {
			continue
		}
		t := tagInfo{Name: strings.TrimPrefix(f[0], "refs/tags/"), Time: f[5]}
		kind, hash := f[1], f[2]
		if kind == "tag" {
			// Annotated: follow the tag object to what it points at
			t.Annotated = true
			t.Message = f[6]
			kind, hash = f[3], f[4]
		}
		if kind == "commit" {
			t.Hash = hash
		}
		res = append(res, t)
	}
	return res
}

// listTags returns the tags of repo, newest first.
func listTags(repo string) ([]tagInfo, error) {
	out, err := git(repo, "for-each-ref", "--sort=-creatordate", tagFormat, "refs/tags")
	if err != nil {
		return nil, err
	}
	return parseTags(out), nil
}

// tagsByCommit maps a commit hash to the sorted names of the tags pointing at it.
func tagsByCommit(repo string) map[string][]string {
	res := map[string][]string{}
	ts, err := listTags(repo)
	if err != nil {
		return res
	}
	for _, t := range ts {
		if t.Hash != "" {
			res[t.Hash] = append(res[t.Hash], t.Name)
		}
	}
	for _, names := range res {
		sort.Strings(names)
	}
	return res
}

// checkTagName rejects names git would refuse or parse as command-line options.
func checkTagName(repo, name string) error {
	if name == "" {
		return errors.New("タグ名を入力してください")
	}
	if strings.HasPrefix(name, "-") {
		return errors.New("タグ名の先頭に - は使えません")
	}
	if _, err := git(repo, "check-ref-format", "refs/tags/"+name); err != nil {
		return errors.New("タグ名として使えない文字や形式です(空白、~ ^ : ? * [ \\ や .. などは使えません)")
	}
	return nil
}

func tagExists(repo, name string) bool {
	_, err := git(repo, "rev-parse", "-q", "--verify", "refs/tags/"+name)
	return err == nil
}

// tagRemote picks the remote that tags are pushed to: origin if present, else the first one.
func tagRemote(repo string) (string, error) {
	out, _ := git(repo, "remote")
	remotes := strings.Fields(out)
	if len(remotes) == 0 {
		return "", errors.New("リモートが設定されていません")
	}
	for _, r := range remotes {
		if r == "origin" {
			return r, nil
		}
	}
	return remotes[0], nil
}

func (s *server) handleTags(r *http.Request) (any, error) {
	return listTags(query(r, "repo"))
}

// handleTagAdd creates a tag at Hash (default HEAD). A non-empty Message makes it an annotated tag.
func (s *server) handleTagAdd(r *http.Request) (any, error) {
	in, err := decode[struct{ Repo, Name, Hash, Message string }](r)
	if err != nil {
		return nil, err
	}
	name, msg := strings.TrimSpace(in.Name), strings.TrimSpace(in.Message)
	if err := checkTagName(in.Repo, name); err != nil {
		return nil, err
	}
	if tagExists(in.Repo, name) {
		return nil, errors.New("同じ名前のタグが既にあります")
	}
	rev := in.Hash
	if rev == "" {
		rev = "HEAD"
	}
	hash, err := resolveCommit(in.Repo, rev)
	if err != nil {
		return nil, err
	}
	args := []string{"tag"}
	if msg != "" {
		args = append(args, "-a", "-m", msg)
	}
	return git(in.Repo, append(args, name, hash)...)
}

// handleTagDel deletes a tag locally, or on the remote when Remote is set.
func (s *server) handleTagDel(r *http.Request) (any, error) {
	in, err := decode[struct {
		Repo, Name string
		Remote     bool
	}](r)
	if err != nil {
		return nil, err
	}
	if err := checkTagName(in.Repo, in.Name); err != nil {
		return nil, err
	}
	if !in.Remote {
		return git(in.Repo, "tag", "-d", in.Name)
	}
	remote, err := tagRemote(in.Repo)
	if err != nil {
		return nil, err
	}
	return git(in.Repo, "push", remote, "--delete", "refs/tags/"+in.Name)
}

// handleTagPush pushes one tag, or every tag when Name is empty.
func (s *server) handleTagPush(r *http.Request) (any, error) {
	in, err := decode[struct{ Repo, Name string }](r)
	if err != nil {
		return nil, err
	}
	remote, err := tagRemote(in.Repo)
	if err != nil {
		return nil, err
	}
	if in.Name == "" {
		return git(in.Repo, "push", remote, "--tags")
	}
	if err := checkTagName(in.Repo, in.Name); err != nil {
		return nil, err
	}
	return git(in.Repo, "push", remote, "refs/tags/"+in.Name)
}

// handleTagRemote lists the tag names that exist on the remote. It talks to
// the network, so it gives up after a timeout instead of hanging the UI.
func (s *server) handleTagRemote(r *http.Request) (any, error) {
	repo := query(r, "repo")
	remote, err := tagRemote(repo)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	out, err := gitEnvCtx(ctx, repo, nil, "ls-remote", "--tags", "--refs", remote)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("リモートへの接続がタイムアウトしました")
		}
		return nil, err
	}
	return parseRemoteTags(out), nil
}

// parseRemoteTags extracts tag names from `ls-remote --tags --refs` output ("<hash>\trefs/tags/<name>").
func parseRemoteTags(out string) []string {
	res := []string{}
	for _, l := range strings.Split(out, "\n") {
		if _, ref, ok := strings.Cut(l, "\t"); ok && strings.HasPrefix(ref, "refs/tags/") {
			res = append(res, strings.TrimPrefix(ref, "refs/tags/"))
		}
	}
	return res
}
