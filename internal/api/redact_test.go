package api

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cloudfoundry-community/shout/internal/state"
)

const leakSecret = "T0PS3CR3TW3BH00K"

// lockedBuffer collects log output safely from any goroutine.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLog sends the standard logger to a buffer for the rest of the test.
func captureLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(buf)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return buf
}

// leakyHandler fails with an error that quotes a secret URL verbatim, the
// way a handler that forgot to mask its error would.
type leakyHandler struct{}

func (leakyHandler) Name() string { return "leaky" }
func (leakyHandler) Send(context.Context, map[string]string) error {
	return fmt.Errorf(`leaky handler: Post "https://hooks.slack.com/services/T1/B1/%s": connection refused`, leakSecret)
}

func postJSON(srv *Server, path, user, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, user)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	return w
}

func TestDeliveryFailureLogsHideWebhookSecrets(t *testing.T) {
	logs := captureLog(t)
	srv := newTestServer(t)
	srv.handlers.Register(leakyHandler{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := ln.Addr().String()
	_ = ln.Close()

	rules := fmt.Sprintf(`
rules:
  - for: "*"
    when:
      - match: "*"
        do:
          - slack:
              webhook: "http://%[1]s/services/T000/B000/%[2]s"
              text: "{{ .Topic }} is {{ .Status }}"
          - webhook:
              url: "http://hook:%[2]s@%[1]s/notify?token=%[2]s"
          - leaky:
              text: "x"
`, dead, leakSecret)
	if err := srv.loadRules([]byte(rules)); err != nil {
		t.Fatalf("loadRules: %v", err)
	}

	// A first failing event is a transition, so every rule fires and every
	// delivery fails. An announcement fires them again.
	for _, req := range []struct{ path, body string }{
		{"/events", `{"topic":"ci/build","ok":false,"message":"failed"}`},
		{"/announcements", `{"topic":"ci/build","message":"note"}`},
	} {
		w := postJSON(srv, req.path, "ops", req.body)
		if w.Code != http.StatusOK {
			t.Fatalf("POST %s = %d: %s", req.path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), leakSecret) {
			t.Fatalf("POST %s response reveals the secret: %s", req.path, w.Body.String())
		}
	}

	out := logs.String()
	for _, name := range []string{"slack", "webhook", "leaky"} {
		if strings.Count(out, fmt.Sprintf("handler %q error", name)) != 2 {
			t.Errorf("expected two %s delivery failures in the log:\n%s", name, out)
		}
	}
	if strings.Contains(out, leakSecret) {
		t.Fatalf("log reveals the webhook secret:\n%s", out)
	}
	if !strings.Contains(out, "/services/REDACTED") || !strings.Contains(out, "?REDACTED") {
		t.Errorf("log does not show the masked URLs:\n%s", out)
	}
}

func TestReminderFailureLogsHideWebhookSecrets(t *testing.T) {
	logs := captureLog(t)
	srv := newTestServer(t)
	srv.handlers.Register(leakyHandler{})
	if err := srv.loadRules([]byte(`
rules:
  - for: "*"
    when:
      - match: "*"
        do:
          - leaky:
              text: "x"
`)); err != nil {
		t.Fatal(err)
	}
	srv.states.Ingest(&state.Event{Topic: "ci/build", OK: false, Message: "failed"})
	srv.fireRules(srv.states.Get("ci/build").LastEvent, "still broken")

	if out := logs.String(); strings.Contains(out, leakSecret) || !strings.Contains(out, `handler "leaky" error`) {
		t.Fatalf("reminder log line is missing or reveals the secret:\n%s", out)
	}
}

func TestRulesErrorResponseHidesWebhookSecret(t *testing.T) {
	srv := newTestServer(t)
	rules := fmt.Sprintf(`
rules:
  - for: "*"
    when:
      - match: 'topic == "https://hooks.slack.com/services/T1/B1/%s" &&'
        do: []
`, leakSecret)

	// The premise: the raw compile error quotes the rule, secret included.
	if err := srv.loadRules([]byte(rules)); err == nil || !strings.Contains(err.Error(), leakSecret) {
		t.Fatalf("test premise failed, raw error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/rules", bytes.NewBufferString(rules))
	req.SetBasicAuth("admin", "admin")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST /rules = %d, want 400", w.Code)
	}
	if strings.Contains(w.Body.String(), leakSecret) {
		t.Fatalf("rules error response reveals the secret: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "/services/REDACTED") {
		t.Errorf("rules error response lost its detail: %s", w.Body.String())
	}
}
