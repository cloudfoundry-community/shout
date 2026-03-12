package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONFileStoreRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store := NewJSONFileStore(path)

	states := map[string]*TopicState{
		"test/pipeline": {
			Name:   "test/pipeline",
			Status: "broken",
			LastEvent: &Event{
				Topic:   "test/pipeline",
				OK:      false,
				Message: "build failed",
			},
		},
	}

	if err := store.Save(states); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	ts, ok := loaded["test/pipeline"]
	if !ok {
		t.Fatal("expected test/pipeline in loaded state")
	}
	if ts.Status != "broken" {
		t.Errorf("expected status=broken, got %q", ts.Status)
	}
	if ts.LastEvent == nil || ts.LastEvent.Message != "build failed" {
		t.Error("expected last event with message 'build failed'")
	}
}

func TestJSONFileStoreLoadNonexistent(t *testing.T) {
	store := NewJSONFileStore("/tmp/does-not-exist-shout-test.json")
	states, err := store.Load()
	if err != nil {
		t.Fatalf("Load() should not error for missing file, got: %v", err)
	}
	if states == nil {
		t.Error("expected non-nil empty map")
	}
	if len(states) != 0 {
		t.Errorf("expected empty map, got %d entries", len(states))
	}
}

func TestJSONFileStoreFieldNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store := NewJSONFileStore(path)

	states := map[string]*TopicState{
		"test": {
			Name:           "test",
			Status:         "working",
			LastNotifiedAt: 12345,
			RemindEvery:    300,
		},
	}

	if err := store.Save(states); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	content := string(data)
	// Verify JSON field names match the Lisp version
	for _, field := range []string{`"name"`, `"state"`, `"notified"`, `"reminder"`} {
		if !contains(content, field) {
			t.Errorf("expected JSON to contain field %s", field)
		}
	}
}

func TestJSONFileStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store := NewJSONFileStore(path)

	states := map[string]*TopicState{
		"test": {Name: "test", Status: "working"},
	}

	if err := store.Save(states); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify the temp file was cleaned up
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("expected temp file to be cleaned up after atomic rename")
	}
}

func TestJSONFileStoreCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub", "dir", "state.json")
	store := NewJSONFileStore(nested)

	states := map[string]*TopicState{
		"test": {Name: "test", Status: "working"},
	}

	if err := store.Save(states); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if _, err := os.Stat(nested); os.IsNotExist(err) {
		t.Error("expected file to be created in nested directory")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
