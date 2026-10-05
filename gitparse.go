package main

import "strings"

// Parsers for git's machine-readable output. They are pure functions so they
// can be unit-tested without running git.

// commit is one entry of the history. The JSON keys keep their capitalized
// names because the UI reads them as-is.
type commit struct{ Hash, Author, Time, Subject string }

// logFormat must stay in sync with parseLog.
const logFormat = "--format=%H%x1f%an%x1f%at%x1f%s"

func parseLog(out string) []commit {
	cs := []commit{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(l, "\x1f")
		if len(f) == 4 {
			cs = append(cs, commit{f[0], f[1], f[2], f[3]})
		}
	}
	return cs
}

type fileChange struct {
	Path   string `json:"path"`
	Old    string `json:"old"`
	Status string `json:"status"`
}

// parseNameStatus parses `--name-status -z` output.
// Renames and copies carry a second path (the new one).
func parseNameStatus(out string) []fileChange {
	res := []fileChange{}
	parts := strings.Split(out, "\x00")
	for i := 0; i < len(parts); i++ {
		st := parts[i]
		if st == "" || i+1 >= len(parts) {
			continue
		}
		f := fileChange{Status: st[:1], Path: parts[i+1]}
		i++
		if (st[0] == 'R' || st[0] == 'C') && i+1 < len(parts) {
			f.Old, f.Path = f.Path, parts[i+1]
			i++
		}
		res = append(res, f)
	}
	return res
}

type statusEntry struct {
	Path   string `json:"path"`
	X      string `json:"x"`
	Y      string `json:"y"`
	Staged bool   `json:"staged"`
}

// parseStatus parses `status --porcelain=v1 -z` output.
func parseStatus(out string) []statusEntry {
	res := []statusEntry{}
	parts := strings.Split(out, "\x00")
	for i := 0; i < len(parts); i++ {
		e := parts[i]
		if len(e) < 4 {
			continue
		}
		x, y := string(e[0]), string(e[1])
		// A rename entry is followed by the original path; skip it
		if x == "R" || x == "C" {
			i++
		}
		res = append(res, statusEntry{Path: e[3:], X: x, Y: y, Staged: x != " " && x != "?"})
	}
	return res
}

// splitNul splits NUL-separated output (-z) and drops empty items.
func splitNul(out string) []string {
	res := []string{}
	for _, s := range strings.Split(out, "\x00") {
		if s != "" {
			res = append(res, s)
		}
	}
	return res
}
