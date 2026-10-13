package main

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// opener is the command used to open a file with the desktop's default application.
// It is a variable so tests can replace it.
var opener = "xdg-open"

// openDir holds read-only copies of past file versions that were opened externally.
// It is wiped on every start (see cleanOpenDir), because the external app needs the
// file to outlive the request.
func openDir() string { return filepath.Join(os.TempDir(), "gitcord-open") }

func cleanOpenDir() { _ = os.RemoveAll(openDir()) }

// handleOpen opens a file with the default application (double-click in the UI).
// With an empty rev, the working-tree file is opened so edits are saved into the repo.
// If it no longer exists (e.g. deleted), or a rev is given, a read-only copy of that
// version (HEAD when rev is empty) is extracted to a temporary directory and opened instead.
func (s *server) handleOpen(r *http.Request) (any, error) {
	// A cross-origin "simple" request cannot set this header, so this blocks other
	// web pages from making this local server launch files.
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return nil, errors.New("invalid request")
	}
	req, err := decode[struct{ Repo, Path, Rev string }](r)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(s.repos.List(), req.Repo) {
		return nil, errors.New("登録されていないディレクトリです")
	}
	if err := checkRev(req.Rev); err != nil {
		return nil, err
	}
	// Reject absolute paths and any ".." escape from the repository
	if !filepath.IsLocal(filepath.FromSlash(req.Path)) {
		return nil, errors.New("invalid path")
	}

	target := filepath.Join(req.Repo, filepath.FromSlash(req.Path))
	if st, err := os.Stat(target); req.Rev != "" || err != nil || !st.Mode().IsRegular() {
		rev := req.Rev
		if rev == "" {
			rev = "HEAD"
		}
		if target, err = extractVersion(req.Repo, rev, req.Path); err != nil {
			return nil, err
		}
	}
	if err := launch(target); err != nil {
		return nil, err
	}
	return "ok", nil
}

// extractVersion writes the file as of rev into a fresh temporary directory
// (keeping the original file name and extension) and returns its path.
func extractVersion(repo, rev, path string) (string, error) {
	data, err := git(repo, "show", rev+":"+path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(openDir(), 0o700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(openDir(), "v-")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, filepath.Base(path))
	if err := os.WriteFile(out, []byte(data), 0o444); err != nil {
		return "", err
	}
	return out, nil
}

// launch starts the opener without waiting for the application to quit.
// If the opener fails quickly (e.g. no associated application), the error is reported.
func launch(path string) error {
	c := exec.Command(opener, path)
	c.Env = cleanEnv()
	var stderr bytes.Buffer
	c.Stderr = &stderr
	if err := c.Start(); err != nil {
		return errors.New("ファイルを開けませんでした: " + err.Error())
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return errors.New("ファイルを開けませんでした: " + msg)
		}
	case <-time.After(1500 * time.Millisecond):
		// Still running: the application has taken over
	}
	return nil
}
