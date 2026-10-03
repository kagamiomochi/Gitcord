package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed index.html
var ui embed.FS

var store = filepath.Join(os.Getenv("HOME"), ".config", "gitcord", "repos.json")

func git(repo string, args ...string) (string, error) {
	a := args
	if repo != "" {
		a = append([]string{"-C", repo}, args...)
	}
	c := exec.Command("git", a...)
	c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C.UTF-8")
	out, err := c.CombinedOutput()
	if err != nil {
		return string(out), errors.New(strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func repos() []string {
	var r []string
	if b, err := os.ReadFile(store); err == nil {
		json.Unmarshal(b, &r)
	}
	return r
}

func saveRepos(r []string) {
	os.MkdirAll(filepath.Dir(store), 0o755)
	b, _ := json.Marshal(r)
	os.WriteFile(store, b, 0o644)
}

func addRepo(p string) error {
	if _, err := git(p, "rev-parse", "--git-dir"); err != nil {
		return errors.New("Gitディレクトリではありません")
	}
	r := repos()
	for _, x := range r {
		if x == p {
			return nil
		}
	}
	saveRepos(append(r, p))
	return nil
}

func isGit(p string) bool {
	_, err := os.Stat(filepath.Join(p, ".git"))
	return err == nil
}

// Shared client with a timeout so a slow network never hangs the commit flow
var trClient = &http.Client{Timeout: 10 * time.Second}

// translate prefers DeepL when a key is configured, otherwise uses MyMemory
func translate(text string) (string, error) {
	if key := os.Getenv("GITCORD_DEEPL_KEY"); key != "" {
		return translateDeepL(text, key)
	}
	return translateMyMemory(text)
}

func translateDeepL(text, key string) (string, error) {
	form := url.Values{"text": {text}, "target_lang": {"EN"}}
	req, err := http.NewRequest("POST", "https://api-free.deepl.com/v2/translate", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "DeepL-Auth-Key "+key)
	resp, err := trClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DeepLがHTTP %dを返しました", resp.StatusCode)
	}
	var out struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil || len(out.Translations) == 0 {
		return "", errors.New("DeepLの応答を解析できませんでした")
	}
	return strings.TrimSpace(out.Translations[0].Text), nil
}

// translateMyMemory translates line by line to stay under the per-request size limit
func translateMyMemory(text string) (string, error) {
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		// Skip empty or ASCII-only lines, there is nothing to translate
		if strings.TrimSpace(ln) == "" || !hasNonASCII(ln) {
			continue
		}
		t, err := myMemoryLine(ln)
		if err != nil {
			return "", err
		}
		lines[i] = t
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func hasNonASCII(s string) bool {
	for _, r := range s {
		if r > 0x7f {
			return true
		}
	}
	return false
}

func myMemoryLine(s string) (string, error) {
	q := url.Values{"q": {s}, "langpair": {"ja|en"}}
	// Optional contact email raises the daily quota from 5k to 50k chars
	if mail := os.Getenv("GITCORD_MYMEMORY_EMAIL"); mail != "" {
		q.Set("de", mail)
	}
	resp, err := trClient.Get("https://api.mymemory.translated.net/get?" + q.Encode())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("MyMemoryがHTTP %dを返しました", resp.StatusCode)
	}
	var out struct {
		Data struct {
			Text string `json:"translatedText"`
		} `json:"responseData"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil || out.Data.Text == "" {
		return "", errors.New("MyMemoryの応答を解析できませんでした")
	}
	// MyMemory reports quota exhaustion inside a normal 200 response body
	if strings.HasPrefix(out.Data.Text, "MYMEMORY WARNING") {
		return "", errors.New("MyMemoryの無料枠を使い切りました")
	}
	return out.Data.Text, nil
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8484", "listen address")
	noOpen := flag.Bool("no-open", false, "do not open the browser")
	flag.Parse()

	h := func(p string, f func(r *http.Request) (any, error)) {
		http.HandleFunc("/api/"+p, func(w http.ResponseWriter, r *http.Request) {
			v, err := f(r)
			w.Header().Set("Content-Type", "application/json")
			if err != nil {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			json.NewEncoder(w).Encode(v)
		})
	}
	q := func(r *http.Request, k string) string { return r.URL.Query().Get(k) }
	body := func(r *http.Request) (m struct {
		Repo, Path, URL, Dir, Name, Hash, Action, Message, Text string
		Paths                                                  []string
		Stage                                                  bool
	}) {
		json.NewDecoder(r.Body).Decode(&m)
		return
	}

	h("repos", func(r *http.Request) (any, error) { return repos(), nil })
	h("ls", func(r *http.Request) (any, error) {
		p := q(r, "path")
		if p == "" {
			p, _ = os.UserHomeDir()
		}
		ents, err := os.ReadDir(p)
		if err != nil {
			return nil, err
		}
		type E struct {
			Name string `json:"name"`
			Git  bool   `json:"git"`
		}
		out := []E{}
		for _, e := range ents {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				out = append(out, E{e.Name(), isGit(filepath.Join(p, e.Name()))})
			}
		}
		sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
		return map[string]any{"path": p, "parent": filepath.Dir(p), "git": isGit(p), "entries": out}, nil
	})
	h("add", func(r *http.Request) (any, error) { return "ok", addRepo(body(r).Path) })
	h("remove", func(r *http.Request) (any, error) {
		p := body(r).Path
		var n []string
		for _, x := range repos() {
			if x != p {
				n = append(n, x)
			}
		}
		saveRepos(n)
		return "ok", nil
	})
	h("clone", func(r *http.Request) (any, error) {
		m := body(r)
		dest := filepath.Join(m.Dir, m.Name)
		if _, err := git("", "clone", m.URL, dest); err != nil {
			return nil, err
		}
		return dest, addRepo(dest)
	})
	h("log", func(r *http.Request) (any, error) {
		repo := q(r, "repo")
		out, err := git(repo, "log", "-n", "5000", "--format=%H%x1f%an%x1f%at%x1f%s")
		if err != nil {
			return map[string]any{"commits": []any{}, "unpushed": []string{}}, nil
		}
		type C struct{ Hash, Author, Time, Subject string }
		cs := []C{}
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			f := strings.Split(l, "\x1f")
			if len(f) == 4 {
				cs = append(cs, C{f[0], f[1], f[2], f[3]})
			}
		}
		up, _ := git(repo, "rev-list", "HEAD", "--not", "--remotes")
		return map[string]any{"commits": cs, "unpushed": strings.Fields(up)}, nil
	})
	h("tree", func(r *http.Request) (any, error) {
		rev := q(r, "rev")
		if rev == "" {
			rev = "HEAD"
		}
		out, err := git(q(r, "repo"), "ls-tree", "-r", "--name-only", rev)
		if err != nil {
			return []string{}, nil
		}
		return strings.Split(strings.TrimSpace(out), "\n"), nil
	})
	h("diff", func(r *http.Request) (any, error) {
		repo, rev, p := q(r, "repo"), q(r, "rev"), q(r, "path")
		var out string
		if rev != "" {
			out, _ = git(repo, "show", "--format=", rev, "--", p)
			if strings.TrimSpace(out) == "" {
				out, _ = git(repo, "show", rev+":"+p)
				out = "(このコミットでは変更なし。ファイル内容)\n" + out
			}
		} else if q(r, "staged") == "1" {
			out, _ = git(repo, "diff", "--cached", "--", p)
		} else {
			out, _ = git(repo, "diff", "--", p)
			if out == "" {
				out, _ = git(repo, "diff", "--no-index", "/dev/null", filepath.Join(repo, p))
			}
		}
		return out, nil
	})
	h("status", func(r *http.Request) (any, error) {
		out, err := git(q(r, "repo"), "status", "--porcelain=v1", "-uall", "-z")
		if err != nil {
			return nil, err
		}
		type S struct {
			Path   string `json:"path"`
			X      string `json:"x"`
			Y      string `json:"y"`
			Staged bool   `json:"staged"`
		}
		res := []S{}
		parts := strings.Split(out, "\x00")
		for i := 0; i < len(parts); i++ {
			e := parts[i]
			if len(e) < 4 {
				continue
			}
			x, y := string(e[0]), string(e[1])
			if x == "R" || x == "C" {
				i++
			}
			res = append(res, S{e[3:], x, y, x != " " && x != "?"})
		}
		return res, nil
	})
	h("stage", func(r *http.Request) (any, error) {
		m := body(r)
		args := []string{"add", "--"}
		if !m.Stage {
			args = []string{"reset", "-q", "--"}
		}
		_, err := git(m.Repo, append(args, m.Paths...)...)
		return "ok", err
	})
	h("commit", func(r *http.Request) (any, error) {
		m := body(r)
		_, err := git(m.Repo, "commit", "-m", m.Message)
		return "ok", err
	})
	h("push", func(r *http.Request) (any, error) {
		out, err := git(body(r).Repo, "push")
		return out, err
	})
	h("action", func(r *http.Request) (any, error) {
		m := body(r)
		switch m.Action {
		case "soft", "mixed", "hard":
			return git(m.Repo, "reset", "--"+m.Action, m.Hash)
		case "revert":
			return git(m.Repo, "revert", "--no-edit", m.Hash)
		}
		return nil, errors.New("unknown action")
	})
	h("translate", func(r *http.Request) (any, error) { return translate(body(r).Text) })

	sub := http.FileServerFS(ui)
	http.Handle("/", sub)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	u := "http://" + ln.Addr().String()
	fmt.Println("Gitcord: " + u)
	if !*noOpen {
		go exec.Command("xdg-open", u).Start()
	}
	http.Serve(ln, nil)
}
