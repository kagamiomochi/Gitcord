package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// maxFileBytes caps the amount of file text sent to the UI to keep it responsive.
const maxFileBytes = 1 << 20

type server struct {
	repos *repoStore
	ui    fs.FS
}

// apiFunc is a JSON endpoint. A returned error becomes a 400 {"error": ...}.
type apiFunc func(r *http.Request) (any, error)

func jsonHandler(f apiFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := f(r)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(v)
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	api := func(name string, f apiFunc) { mux.HandleFunc("/api/"+name, jsonHandler(f)) }

	api("repos", s.handleRepos)
	api("ls", s.handleLs)
	api("add", s.handleAdd)
	api("remove", s.handleRemove)
	api("clone", s.handleClone)
	api("log", s.handleLog)
	api("tree", s.handleTree)
	api("file", s.handleFile)
	api("diff", s.handleDiff)
	api("changes", s.handleChanges)
	api("status", s.handleStatus)
	api("stage", s.handleStage)
	api("commit", s.handleCommit)
	api("push", s.handlePush)
	api("action", s.handleAction)
	api("translate", s.handleTranslate)

	mux.Handle("/", http.FileServerFS(s.ui))
	return mux
}

// decode reads the JSON request body into T.
func decode[T any](r *http.Request) (T, error) {
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return v, errors.New("invalid request body")
	}
	return v, nil
}

// query returns a URL query parameter.
func query(r *http.Request, k string) string { return r.URL.Query().Get(k) }

// revParam returns the "rev" query parameter, defaulting to HEAD when def is set.
func revParam(r *http.Request, def bool) (string, error) {
	rev := query(r, "rev")
	if rev == "" && def {
		rev = "HEAD"
	}
	return rev, checkRev(rev)
}

func isGit(p string) bool {
	_, err := os.Stat(filepath.Join(p, ".git"))
	return err == nil
}

// addRepo registers p after checking that it is a git directory.
func (s *server) addRepo(p string) error {
	if _, err := git(p, "rev-parse", "--git-dir"); err != nil {
		return errors.New("Gitディレクトリではありません")
	}
	return s.repos.Add(p)
}

// ---- repository list ----

func (s *server) handleRepos(r *http.Request) (any, error) { return s.repos.List(), nil }

func (s *server) handleLs(r *http.Request) (any, error) {
	p := query(r, "path")
	if p == "" {
		p, _ = os.UserHomeDir()
	}
	ents, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	type entry struct {
		Name string `json:"name"`
		Git  bool   `json:"git"`
	}
	out := []entry{}
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, entry{e.Name(), isGit(filepath.Join(p, e.Name()))})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return map[string]any{"path": p, "parent": filepath.Dir(p), "git": isGit(p), "entries": out}, nil
}

func (s *server) handleAdd(r *http.Request) (any, error) {
	in, err := decode[struct{ Path string }](r)
	if err != nil {
		return nil, err
	}
	return "ok", s.addRepo(in.Path)
}

func (s *server) handleRemove(r *http.Request) (any, error) {
	in, err := decode[struct{ Path string }](r)
	if err != nil {
		return nil, err
	}
	return "ok", s.repos.Remove(in.Path)
}

func (s *server) handleClone(r *http.Request) (any, error) {
	in, err := decode[struct{ URL, Dir, Name string }](r)
	if err != nil {
		return nil, err
	}
	dest := filepath.Join(in.Dir, in.Name)
	// "--" keeps a URL such as "--upload-pack=..." from being parsed as an option
	if _, err := git("", "clone", "--", in.URL, dest); err != nil {
		return nil, err
	}
	return dest, s.addRepo(dest)
}

// ---- history and file browsing (read-only) ----

func (s *server) handleLog(r *http.Request) (any, error) {
	repo := query(r, "repo")
	out, err := git(repo, "log", "-n", "5000", logFormat)
	if err != nil {
		return map[string]any{"commits": []commit{}, "unpushed": []string{}}, nil
	}
	up, _ := git(repo, "rev-list", "HEAD", "--not", "--remotes")
	return map[string]any{"commits": parseLog(out), "unpushed": strings.Fields(up)}, nil
}

func (s *server) handleTree(r *http.Request) (any, error) {
	rev, err := revParam(r, true)
	if err != nil {
		return nil, err
	}
	// -z keeps non-ASCII file names unquoted
	out, err := git(query(r, "repo"), "ls-tree", "-r", "-z", "--name-only", rev)
	if err != nil {
		return []string{}, nil
	}
	return splitNul(out), nil
}

func (s *server) handleFile(r *http.Request) (any, error) {
	rev, err := revParam(r, true)
	if err != nil {
		return nil, err
	}
	out, err := git(query(r, "repo"), "show", rev+":"+query(r, "path"))
	if err != nil {
		return nil, err
	}
	// Treat NUL bytes or invalid UTF-8 as binary content
	if strings.ContainsRune(out, 0) || !utf8.ValidString(out) {
		return "(バイナリファイルは表示できません)", nil
	}
	if len(out) > maxFileBytes {
		out = strings.ToValidUTF8(out[:maxFileBytes], "") + "\n\n(1MBを超えるため、以降は省略されました)"
	}
	return out, nil
}

func (s *server) handleDiff(r *http.Request) (any, error) {
	repo, p := query(r, "repo"), query(r, "path")
	rev, err := revParam(r, false)
	if err != nil {
		return nil, err
	}
	var out string
	switch {
	case rev != "":
		// Include the old path so renames are shown as renames, not as a whole-file add
		paths := []string{p}
		if o := query(r, "old"); o != "" && o != p {
			paths = append(paths, o)
		}
		// -m --first-parent makes merge commits show a diff against their first parent
		args := append([]string{"show", "--format=", "-m", "--first-parent", "-M", rev, "--"}, paths...)
		out, _ = git(repo, args...)
		if strings.TrimSpace(out) == "" {
			out, _ = git(repo, "show", rev+":"+p)
			out = "(このコミットでは変更なし。ファイル内容)\n" + out
		}
	case query(r, "staged") == "1":
		out, _ = git(repo, "diff", "--cached", "--", p)
	default:
		out, _ = git(repo, "diff", "--", p)
		if out == "" { // untracked file: diff it against /dev/null
			out, _ = git(repo, "diff", "--no-index", "/dev/null", filepath.Join(repo, p))
		}
	}
	return out, nil
}

// handleChanges lists the files touched by a commit.
func (s *server) handleChanges(r *http.Request) (any, error) {
	repo := query(r, "repo")
	rev, err := revParam(r, false)
	if err != nil {
		return nil, err
	}
	// diff-tree prints nothing useful for merges without -m, so diff against the first parent instead
	var out string
	if ps, _ := git(repo, "rev-list", "--parents", "-n", "1", rev); len(strings.Fields(ps)) > 2 {
		out, err = git(repo, "diff", "--name-status", "-M", "-z", rev+"^1", rev)
	} else {
		out, err = git(repo, "diff-tree", "--no-commit-id", "--name-status", "-r", "-M", "-z", "--root", rev)
	}
	if err != nil {
		return nil, err
	}
	return parseNameStatus(out), nil
}

// ---- working tree ----

func (s *server) handleStatus(r *http.Request) (any, error) {
	out, err := git(query(r, "repo"), "status", "--porcelain=v1", "-uall", "-z")
	if err != nil {
		return nil, err
	}
	return parseStatus(out), nil
}

func (s *server) handleStage(r *http.Request) (any, error) {
	in, err := decode[struct {
		Repo  string
		Paths []string
		Stage bool
	}](r)
	if err != nil {
		return nil, err
	}
	args := []string{"add", "--"}
	if !in.Stage {
		args = []string{"reset", "-q", "--"}
	}
	_, err = git(in.Repo, append(args, in.Paths...)...)
	return "ok", err
}

func (s *server) handleCommit(r *http.Request) (any, error) {
	in, err := decode[struct{ Repo, Message string }](r)
	if err != nil {
		return nil, err
	}
	_, err = git(in.Repo, "commit", "-m", in.Message)
	return "ok", err
}

func (s *server) handlePush(r *http.Request) (any, error) {
	in, err := decode[struct{ Repo string }](r)
	if err != nil {
		return nil, err
	}
	return git(in.Repo, "push")
}

// handleAction rewrites history: reset (soft/mixed/hard) or revert.
func (s *server) handleAction(r *http.Request) (any, error) {
	in, err := decode[struct{ Repo, Hash, Action string }](r)
	if err != nil {
		return nil, err
	}
	if err := checkRev(in.Hash); err != nil {
		return nil, err
	}
	switch in.Action {
	case "soft", "mixed", "hard":
		return git(in.Repo, "reset", "--"+in.Action, in.Hash)
	case "revert":
		return git(in.Repo, "revert", "--no-edit", in.Hash)
	}
	return nil, errors.New("unknown action")
}

func (s *server) handleTranslate(r *http.Request) (any, error) {
	in, err := decode[struct{ Text string }](r)
	if err != nil {
		return nil, err
	}
	return translate(in.Text)
}
