package notify

import (
	"context"
	"sort"
	"testing"
)

type fakeHandler struct {
	name string
}

func (f *fakeHandler) Name() string                                     { return f.name }
func (f *fakeHandler) Send(_ context.Context, _ map[string]string) error { return nil }

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeHandler{name: "test"})

	h, ok := r.Get("test")
	if !ok {
		t.Fatal("expected handler to be registered")
	}
	if h.Name() != "test" {
		t.Errorf("expected name=test, got %q", h.Name())
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected ok=false for missing handler")
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeHandler{name: "slack"})
	r.Register(&fakeHandler{name: "webhook"})
	r.Register(&fakeHandler{name: "email"})

	names := r.List()
	sort.Strings(names)

	expected := []string{"email", "slack", "webhook"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d handlers, got %d", len(expected), len(names))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("expected %q at index %d, got %q", expected[i], i, name)
		}
	}
}

func TestRegistryOverwrite(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeHandler{name: "test"})
	r.Register(&fakeHandler{name: "test"}) // overwrite

	names := r.List()
	if len(names) != 1 {
		t.Errorf("expected 1 handler after overwrite, got %d", len(names))
	}
}
