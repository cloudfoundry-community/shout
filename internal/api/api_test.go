package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/cloudfoundry-community/shout/internal/notify"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		Port:       0,
		DBPath:     filepath.Join(dir, "test.db"),
		OpsCreds:   "ops:ops",
		AdminCreds: "admin:admin",
	}
	handlers := notify.NewRegistry()
	handlers.Register(notify.NewSlackHandler())
	handlers.Register(notify.NewWebhookHandler())

	srv, err := New(cfg, handlers)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return srv
}

func TestHandleInfo(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/info", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["version"] == "" {
		t.Error("expected version in response")
	}
}

func TestHandleEventRequiresAuth(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(`{"topic":"test","ok":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", w.Code)
	}
}

func TestHandleEventValidRequest(t *testing.T) {
	srv := newTestServer(t)

	body := `{"topic":"test/pipeline","ok":true,"message":"build passed"}`
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("ops", "ops")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %v", resp["ok"])
	}
}

func TestHandleEventMissingTopic(t *testing.T) {
	srv := newTestServer(t)

	body := `{"ok":true,"message":"no topic"}`
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("ops", "ops")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing topic, got %d", w.Code)
	}
}

func TestHandleEventInvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("ops", "ops")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestHandleAnnouncement(t *testing.T) {
	srv := newTestServer(t)

	body := `{"topic":"deploy","ok":false,"message":"deploying v2.0"}`
	req := httptest.NewRequest(http.MethodPost, "/announcements", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("ops", "ops")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleAnnouncementMissingTopic(t *testing.T) {
	srv := newTestServer(t)

	body := `{"ok":true,"message":"no topic"}`
	req := httptest.NewRequest(http.MethodPost, "/announcements", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("ops", "ops")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing topic, got %d", w.Code)
	}
}

func TestHandleGetState(t *testing.T) {
	srv := newTestServer(t)

	// Ingest an event first
	body := `{"topic":"test","ok":true,"message":"ok"}`
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("ops", "ops")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	// Get state
	req = httptest.NewRequest(http.MethodGet, "/state?topic=test", nil)
	req.SetBasicAuth("ops", "ops")
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetStateMissingParam(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/state", nil)
	req.SetBasicAuth("ops", "ops")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing topic param, got %d", w.Code)
	}
}

func TestHandleGetStateNotFound(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/state?topic=nonexistent", nil)
	req.SetBasicAuth("ops", "ops")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown topic, got %d", w.Code)
	}
}

func TestHandleGetStates(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/states", nil)
	req.SetBasicAuth("ops", "ops")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandlePostRules(t *testing.T) {
	srv := newTestServer(t)

	rules := `
rules:
  - for: "*"
    when:
      - match: "*"
        do:
          - slack:
              webhook: "http://example.com"
              text: "{{ .Topic }}"
`
	req := httptest.NewRequest(http.MethodPost, "/rules", bytes.NewBufferString(rules))
	req.Header.Set("Content-Type", "text/x-yaml")
	req.SetBasicAuth("admin", "admin")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePostRulesInvalid(t *testing.T) {
	srv := newTestServer(t)

	rules := `
rules:
  - for: "re:[invalid"
    when:
      - match: "*"
        do: []
`
	req := httptest.NewRequest(http.MethodPost, "/rules", bytes.NewBufferString(rules))
	req.Header.Set("Content-Type", "text/x-yaml")
	req.SetBasicAuth("admin", "admin")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid rules, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetRulesNoRules(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/rules", nil)
	req.SetBasicAuth("admin", "admin")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetRulesReturnsYAML(t *testing.T) {
	srv := newTestServer(t)

	// Load rules first
	rules := `rules:
  - for: "*"
    when:
      - match: "*"
        do:
          - slack:
              webhook: "http://example.com"
              text: "test"
`
	req := httptest.NewRequest(http.MethodPost, "/rules", bytes.NewBufferString(rules))
	req.SetBasicAuth("admin", "admin")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /rules failed: %d", w.Code)
	}

	// Get rules
	req = httptest.NewRequest(http.MethodGet, "/rules", nil)
	req.SetBasicAuth("admin", "admin")
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/x-yaml" {
		t.Errorf("expected Content-Type text/x-yaml, got %q", ct)
	}
	if w.Body.String() != rules {
		t.Errorf("expected rules source to be returned verbatim")
	}
}

func TestHandleRulesRequiresAdmin(t *testing.T) {
	srv := newTestServer(t)

	// Try with ops creds
	req := httptest.NewRequest(http.MethodGet, "/rules", nil)
	req.SetBasicAuth("ops", "ops")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with ops creds on admin endpoint, got %d", w.Code)
	}
}

func TestHandleEventTransitions(t *testing.T) {
	srv := newTestServer(t)

	post := func(body string) map[string]any {
		req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("ops", "ops")
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)
		var resp map[string]any
		json.NewDecoder(w.Body).Decode(&resp)
		return resp
	}

	// First working event — no transition
	resp := post(`{"topic":"t","ok":true,"message":"ok"}`)
	if resp["transition"] != "" {
		t.Errorf("expected no transition on first event, got %v", resp["transition"])
	}

	// Working → broken
	resp = post(`{"topic":"t","ok":false,"message":"fail"}`)
	if resp["transition"] != "broken" {
		t.Errorf("expected broken transition, got %v", resp["transition"])
	}

	// Still broken
	resp = post(`{"topic":"t","ok":false,"message":"still fail"}`)
	if resp["transition"] != "" {
		t.Errorf("expected no transition for still broken, got %v", resp["transition"])
	}

	// Broken → fixed
	resp = post(`{"topic":"t","ok":true,"message":"fixed"}`)
	if resp["transition"] != "fixed" {
		t.Errorf("expected fixed transition, got %v", resp["transition"])
	}
}

func TestSplitCreds(t *testing.T) {
	tests := []struct {
		input    string
		expected [2]string
	}{
		{"user:pass", [2]string{"user", "pass"}},
		{"user:pass:extra", [2]string{"user", "pass:extra"}},
		{"useronly", [2]string{"useronly", ""}},
		{":", [2]string{"", ""}},
	}

	for _, tt := range tests {
		result := splitCreds(tt.input)
		if result != tt.expected {
			t.Errorf("splitCreds(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}
