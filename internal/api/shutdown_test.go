package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudfoundry-community/shout/internal/notify"
	"github.com/cloudfoundry-community/shout/internal/state"
)

// blockingHandler is a notification handler whose Send blocks until it is
// released or its context is cancelled.
type blockingHandler struct {
	started   chan struct{}
	release   chan struct{}
	cancelled chan struct{}
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{
		started:   make(chan struct{}, 1),
		release:   make(chan struct{}),
		cancelled: make(chan struct{}),
	}
}

func (h *blockingHandler) Name() string { return "block" }

func (h *blockingHandler) Send(ctx context.Context, _ map[string]string) error {
	h.started <- struct{}{}
	select {
	case <-h.release:
		return nil
	case <-ctx.Done():
		close(h.cancelled)
		return ctx.Err()
	}
}

const blockingRules = `
rules:
  - for: "*"
    when:
      - match: "*"
        do:
          - block:
              note: "{{ .Topic }}"
`

// startServer runs srv.Serve on a loopback listener and returns its base
// URL, a function that cancels its context, and a channel that receives
// Serve's result.
func startServer(t *testing.T, srv *Server) (string, context.CancelFunc, <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		done <- srv.Serve(ctx, ln)
		close(finished)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(10 * time.Second):
			t.Error("Serve did not return during cleanup")
		}
	})
	return "http://" + ln.Addr().String(), cancel, done
}

// postEvent is safe to call from any goroutine; it reports failures by
// returning them rather than through a testing.T.
func postEvent(base, body string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, base+"/events", bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("ops", "ops")
	return http.DefaultClient.Do(req)
}

func waitServe(t *testing.T, done <-chan error, within time.Duration) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(within):
		t.Fatalf("Serve did not return within %s", within)
		return nil
	}
}

func readStateFile(t *testing.T, path string) map[string]*state.TopicState {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading state file: %v", err)
	}
	var states map[string]*state.TopicState
	if err := json.Unmarshal(data, &states); err != nil {
		t.Fatalf("parsing state file: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temp state file left behind: %v", err)
	}
	return states
}

func TestServeSavesStateOnShutdown(t *testing.T) {
	srv := newTestServer(t)
	base, cancel, done := startServer(t, srv)

	resp, err := postEvent(base, `{"topic":"ci/build","ok":false,"message":"failed"}`)
	if err != nil {
		t.Fatalf("POST /events: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /events status %d", resp.StatusCode)
	}

	if _, err := os.Stat(srv.cfg.DBPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state file exists before shutdown (err=%v); the test would prove nothing", err)
	}

	cancel()
	if err := waitServe(t, done, 5*time.Second); err != nil {
		t.Fatalf("Serve returned %v, want nil", err)
	}

	states := readStateFile(t, srv.cfg.DBPath)
	ts, ok := states["ci/build"]
	if !ok {
		t.Fatalf("topic missing from saved state: %v", states)
	}
	if ts.Status != "broken" {
		t.Errorf("saved status %q, want broken", ts.Status)
	}
}

func TestServeDrainsInFlightRequestBeforeSaving(t *testing.T) {
	srv := newTestServer(t)
	h := newBlockingHandler()
	srv.handlers.Register(h)
	if err := srv.loadRules([]byte(blockingRules)); err != nil {
		t.Fatalf("loading rules: %v", err)
	}
	base, cancel, done := startServer(t, srv)

	type result struct {
		code int
		err  error
	}
	reqDone := make(chan result, 1)
	go func() {
		resp, err := postEvent(base, `{"topic":"ci/deploy","ok":false,"message":"failed"}`)
		if err != nil {
			reqDone <- result{err: err}
			return
		}
		resp.Body.Close()
		reqDone <- result{code: resp.StatusCode}
	}()

	select {
	case <-h.started:
	case <-time.After(5 * time.Second):
		t.Fatal("notification never started")
	}

	cancel()
	select {
	case err := <-done:
		t.Fatalf("Serve returned %v while a request was in flight", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(h.release)
	r := <-reqDone
	if r.err != nil || r.code != http.StatusOK {
		t.Fatalf("in-flight request: code %d, err %v; want 200", r.code, r.err)
	}
	if err := waitServe(t, done, 5*time.Second); err != nil {
		t.Fatalf("Serve returned %v, want nil", err)
	}

	states := readStateFile(t, srv.cfg.DBPath)
	if ts, ok := states["ci/deploy"]; !ok || ts.Status != "broken" {
		t.Fatalf("saved state for ci/deploy = %+v, want broken", ts)
	}
}

func TestServeCancelsStuckNotificationAfterDrainTimeout(t *testing.T) {
	srv := newTestServer(t)
	srv.drainTimeout = 100 * time.Millisecond
	srv.cancelGrace = 2 * time.Second
	h := newBlockingHandler()
	srv.handlers.Register(h)
	if err := srv.loadRules([]byte(blockingRules)); err != nil {
		t.Fatalf("loading rules: %v", err)
	}
	base, cancel, done := startServer(t, srv)

	go func() {
		resp, err := postEvent(base, `{"topic":"ci/stuck","ok":false,"message":"failed"}`)
		if err == nil {
			resp.Body.Close()
		}
	}()
	select {
	case <-h.started:
	case <-time.After(5 * time.Second):
		t.Fatal("notification never started")
	}

	start := time.Now()
	cancel()
	if err := waitServe(t, done, 5*time.Second); err != nil {
		t.Fatalf("Serve returned %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > srv.drainTimeout+srv.cancelGrace {
		t.Errorf("shutdown took %s, longer than drain plus grace", elapsed)
	}
	select {
	case <-h.cancelled:
	default:
		t.Error("stuck notification was not cancelled")
	}

	states := readStateFile(t, srv.cfg.DBPath)
	if _, ok := states["ci/stuck"]; !ok {
		t.Fatalf("topic ingested before shutdown missing from saved state: %v", states)
	}
}

func TestRunReturnsListenError(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	srv := newTestServer(t)
	srv.cfg.Port = ln.Addr().(*net.TCPAddr).Port
	if err := srv.Run(context.Background()); err == nil {
		t.Fatal("Run on a port in use returned nil, want an error")
	}
}

func TestServeReturnsErrorWhenTLSKeyPairIsUnreadable(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		DBPath:     filepath.Join(dir, "test.db"),
		OpsCreds:   "ops:ops",
		AdminCreds: "admin:admin",
		TLSCert:    filepath.Join(dir, "missing.crt"),
		TLSKey:     filepath.Join(dir, "missing.key"),
	}
	srv, err := New(cfg, notify.NewRegistry())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background(), ln) }()
	if err := waitServe(t, done, 5*time.Second); err == nil {
		t.Fatal("Serve with a missing TLS key pair returned nil, want an error")
	}
}
