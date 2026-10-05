package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// git runs a git command, inside repo (via -C) when repo is not empty.
// stdout is returned even on failure, because some commands such as
// `diff --no-index` exit non-zero while still producing useful output.
func git(repo string, args ...string) (string, error) {
	if repo != "" {
		args = append([]string{"-C", repo}, args...)
	}
	c := exec.Command("git", args...)

	// Do not leak AppImage's bundled library path into system git
	env := make([]string, 0, len(os.Environ())+2)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "LD_LIBRARY_PATH=") || strings.HasPrefix(e, "LD_PRELOAD=") {
			continue
		}
		env = append(env, e)
	}
	c.Env = append(env, "GIT_TERMINAL_PROMPT=0", "LC_ALL=C.UTF-8")

	// Keep stdout and stderr separate so warnings never end up in parsed output
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), errors.New(msg)
	}
	return stdout.String(), nil
}

// checkRev rejects revisions that git would parse as command-line options.
func checkRev(rev string) error {
	if strings.HasPrefix(rev, "-") || strings.ContainsAny(rev, "\x00\n") {
		return errors.New("invalid revision")
	}
	return nil
}
