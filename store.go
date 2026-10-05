package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// repoStore persists the list of registered repositories as a JSON array.
type repoStore struct {
	mu   sync.Mutex
	path string
}

func newRepoStore() *repoStore {
	home, _ := os.UserHomeDir()
	return &repoStore{path: filepath.Join(home, ".config", "gitcord", "repos.json")}
}

// List never returns nil, so it always encodes as a JSON array.
func (s *repoStore) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Add registers p; adding an already registered path is a no-op.
func (s *repoStore) Add(p string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.load()
	for _, x := range r {
		if x == p {
			return nil
		}
	}
	return s.save(append(r, p))
}

func (s *repoStore) Remove(p string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := []string{}
	for _, x := range s.load() {
		if x != p {
			kept = append(kept, x)
		}
	}
	return s.save(kept)
}

// load and save expect s.mu to be held.
func (s *repoStore) load() []string {
	r := []string{}
	if b, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(b, &r)
	}
	if r == nil { // the file may contain a literal "null"
		r = []string{}
	}
	return r
}

func (s *repoStore) save(r []string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}
