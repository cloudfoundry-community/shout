package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Store defines the interface for state persistence.
type Store interface {
	Load() (map[string]*TopicState, error)
	Save(states map[string]*TopicState) error
}

// JSONFileStore persists state to a JSON file, compatible with the
// original Lisp Shout! database format.
type JSONFileStore struct {
	Path string
}

// NewJSONFileStore creates a new file-backed store.
func NewJSONFileStore(path string) *JSONFileStore {
	return &JSONFileStore{Path: path}
}

// Load reads state from the JSON file. Returns an empty map if the file
// does not exist.
func (s *JSONFileStore) Load() (map[string]*TopicState, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return make(map[string]*TopicState), nil
		}
		return nil, fmt.Errorf("loading state: %w", err)
	}

	var states map[string]*TopicState
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	return states, nil
}

// Save writes state to the JSON file atomically via a temp file rename.
func (s *JSONFileStore) Save(states map[string]*TopicState) error {
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing temp state file: %w", err)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		return fmt.Errorf("renaming state file: %w", err)
	}
	return nil
}
