package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Store defines the interface for state persistence.
type Store interface {
	Load() (map[string]*TopicState, error)
	Save(states map[string]*TopicState) error
}

// ErrUnreadable reports a state file that is in neither the Go format nor
// the Lisp format. The server refuses to start on such a file rather than
// discard state it cannot read.
var ErrUnreadable = errors.New("state file is unreadable")

// Format names the layout of a state file.
type Format string

const (
	// FormatMissing means there is no state file yet.
	FormatMissing Format = "missing"
	// FormatGo is the Go server's layout, a JSON object keyed by topic.
	FormatGo Format = "go"
	// FormatLisp is the Lisp server's layout, a JSON list of topics, or
	// null when it had no topics.
	FormatLisp Format = "lisp"
)

// Inspection describes a state file without changing it.
type Inspection struct {
	Format Format
	// Topics is the number of topics the file holds, or for a Lisp file
	// the number that would be imported.
	Topics int
	// Skipped counts Lisp entries without a topic name, which cannot be
	// imported.
	Skipped int
}

// JSONFileStore persists state to a JSON file. It reads the Lisp Shout!
// state file as well, and converts it to the Go format on Load.
type JSONFileStore struct {
	Path string

	// ImportExpiry is the expiry, in seconds, given to topics imported from
	// a Lisp state file, since the Lisp server kept one global expiry
	// rather than one per topic. Zero leaves imported topics without one.
	ImportExpiry int64

	now  func() time.Time
	logf func(format string, args ...any)
}

// NewJSONFileStore creates a new file-backed store.
func NewJSONFileStore(path string) *JSONFileStore {
	return &JSONFileStore{Path: path, now: time.Now, logf: log.Printf}
}

// Inspect reads the state file and reports its format and size without
// writing anything. It returns an error wrapping ErrUnreadable when the
// file is in neither format.
func (s *JSONFileStore) Inspect() (Inspection, error) {
	_, insp, _, err := s.read()
	return insp, err
}

// Load reads state from the JSON file and returns an empty map if the file
// does not exist.
//
// A Lisp state file is converted. Load keeps the original bytes beside it
// as <path>.lisp-<unix seconds>, writes the converted state to path the
// same way Save does, and logs how many topics it imported. Any failure in
// that sequence leaves the original file in place and returns an error.
func (s *JSONFileStore) Load() (map[string]*TopicState, error) {
	states, insp, data, err := s.read()
	if err != nil {
		return nil, err
	}
	if insp.Format == FormatLisp {
		if err := s.importLisp(states, insp, data); err != nil {
			return nil, err
		}
	}
	return states, nil
}

// read parses the state file in whichever format it is in.
func (s *JSONFileStore) read() (map[string]*TopicState, Inspection, []byte, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return make(map[string]*TopicState), Inspection{Format: FormatMissing}, nil, nil
		}
		return nil, Inspection{}, nil, fmt.Errorf("loading state: %w", err)
	}

	trimmed := bytes.TrimSpace(data)
	switch {
	case len(trimmed) == 0:
		return nil, Inspection{}, nil, s.unreadable(errors.New("the file is empty"))

	case trimmed[0] == '{':
		var states map[string]*TopicState
		if err := json.Unmarshal(trimmed, &states); err != nil {
			return nil, Inspection{}, nil, s.unreadable(fmt.Errorf("not valid Go-format state: %w", err))
		}
		return states, Inspection{Format: FormatGo, Topics: len(states)}, data, nil

	case trimmed[0] == '[' || bytes.Equal(trimmed, []byte("null")):
		var expiry int64
		if s.ImportExpiry > 0 {
			expiry = s.ImportExpiry
		}
		states, skipped, err := parseLisp(trimmed, s.clock().Unix(), expiry)
		if err != nil {
			return nil, Inspection{}, nil, s.unreadable(fmt.Errorf("not a valid Lisp state list: %w", err))
		}
		return states, Inspection{Format: FormatLisp, Topics: len(states), Skipped: skipped}, data, nil

	default:
		return nil, Inspection{}, nil, s.unreadable(errors.New("expected a JSON object (Go format) or a JSON list (Lisp format)"))
	}
}

func (s *JSONFileStore) unreadable(cause error) error {
	return fmt.Errorf("parsing state file %s: %w: %w", s.Path, ErrUnreadable, cause)
}

// importLisp keeps the original Lisp file and replaces it with the
// converted state. The original is written first and synced, so at every
// step the state file holds either the Lisp state or the converted state.
func (s *JSONFileStore) importLisp(states map[string]*TopicState, insp Inspection, original []byte) error {
	backup := fmt.Sprintf("%s.lisp-%d", s.Path, s.clock().Unix())
	if err := keepOriginal(backup, original); err != nil {
		return fmt.Errorf("importing Lisp state file %s: %w", s.Path, err)
	}
	if err := s.Save(states); err != nil {
		return fmt.Errorf("importing Lisp state file %s: %w", s.Path, err)
	}

	skipped := ""
	if insp.Skipped > 0 {
		skipped = fmt.Sprintf(" and skipped %d without a topic name", insp.Skipped)
	}
	s.log("state: imported %d %s from the Lisp state file %s%s; the original is kept at %s",
		insp.Topics, plural(insp.Topics, "topic", "topics"), s.Path, skipped, backup)
	return nil
}

// keepOriginal writes data to path, which must not exist yet. A file that
// already holds exactly data counts as kept, so a restart within the same
// second after an interrupted import does not fail.
func keepOriginal(path string, data []byte) error {
	path = filepath.Clean(path)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			existing, readErr := os.ReadFile(path)
			if readErr == nil && bytes.Equal(existing, data) {
				return nil
			}
			return fmt.Errorf("keeping the original: %s already exists with other contents", path)
		}
		return fmt.Errorf("keeping the original: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("keeping the original: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("keeping the original: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("keeping the original: %w", err)
	}
	return nil
}

// Save writes state to the JSON file atomically via a temp file rename.
func (s *JSONFileStore) Save(states map[string]*TopicState) error {
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("writing temp state file: %w", err)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		return fmt.Errorf("renaming state file: %w", err)
	}
	return nil
}

func (s *JSONFileStore) clock() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

func (s *JSONFileStore) log(format string, args ...any) {
	if s.logf == nil {
		log.Printf(format, args...)
		return
	}
	s.logf(format, args...)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
