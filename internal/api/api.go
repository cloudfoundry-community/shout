package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cloudfoundry-community/shout/internal/engine"
	"github.com/cloudfoundry-community/shout/internal/notify"
	"github.com/cloudfoundry-community/shout/internal/redact"
	"github.com/cloudfoundry-community/shout/internal/state"
	"github.com/cloudfoundry-community/shout/pkg/version"
	"gopkg.in/yaml.v3"
)

// Config holds the API server configuration.
type Config struct {
	Port       int
	DBPath     string
	RulesFile  string
	OpsCreds   string // "user:pass"
	AdminCreds string // "user:pass"
	Expiry     int64  // default state expiry in seconds (0 = no expiry)
	TLSCert    string // path to TLS certificate file
	TLSKey     string // path to TLS private key file
}

// Server is the Shout! HTTP API server.
type Server struct {
	cfg      Config
	states   *state.Manager
	handlers *notify.Registry
	mux      *http.ServeMux

	engineMu    sync.RWMutex
	engine      *engine.Engine
	rulesSource []byte

	// work is the context for notification sends. It outlives any single
	// request so that a client hanging up does not abort a notification,
	// and it is cancelled when a shutdown runs out of time to drain.
	work       context.Context
	cancelWork context.CancelFunc

	// drainTimeout bounds how long a shutdown waits for in-flight requests
	// and the scan loop before it cancels their notification sends, and
	// cancelGrace bounds how long it then waits for them to return.
	drainTimeout time.Duration
	cancelGrace  time.Duration

	// inflight counts running HTTP handlers so that the final state save
	// happens after the last of them has updated the state.
	inflightMu sync.Mutex
	inflight   int
	closing    bool
	idle       chan struct{}
}

// Shutdown timing defaults. The BOSH job sends SIGKILL 30 seconds after
// SIGTERM, so a shutdown must drain, cancel, and save well within that.
const (
	defaultDrainTimeout = 20 * time.Second
	defaultCancelGrace  = 5 * time.Second
)

// New creates a new API server.
func New(cfg Config, handlers *notify.Registry) (*Server, error) {
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		if cfg.TLSCert == "" {
			return nil, fmt.Errorf("SHOUT_TLS_KEY is set but SHOUT_TLS_CERT is missing")
		}
		return nil, fmt.Errorf("SHOUT_TLS_CERT is set but SHOUT_TLS_KEY is missing")
	}

	store := state.NewJSONFileStore(cfg.DBPath)
	store.ImportExpiry = cfg.Expiry
	var opts []state.ManagerOption
	if cfg.Expiry > 0 {
		opts = append(opts, state.WithExpiry(cfg.Expiry))
	}
	mgr, err := state.NewManager(store, opts...)
	if err != nil {
		return nil, fmt.Errorf("initializing state: %w", err)
	}

	work, cancelWork := context.WithCancel(context.Background())
	s := &Server{
		cfg:          cfg,
		states:       mgr,
		handlers:     handlers,
		mux:          http.NewServeMux(),
		work:         work,
		cancelWork:   cancelWork,
		drainTimeout: defaultDrainTimeout,
		cancelGrace:  defaultCancelGrace,
		idle:         make(chan struct{}),
	}

	if cfg.RulesFile != "" {
		if err := s.loadRulesFromFile(cfg.RulesFile); err != nil {
			return nil, err
		}
	}

	s.mux.HandleFunc("GET /info", s.handleInfo)
	s.mux.HandleFunc("POST /events", s.requireOps(s.handleEvent))
	s.mux.HandleFunc("POST /announcements", s.requireOps(s.handleAnnouncement))
	s.mux.HandleFunc("GET /state", s.requireOps(s.handleGetState))
	s.mux.HandleFunc("GET /states", s.requireOps(s.handleGetStates))
	s.mux.HandleFunc("GET /rules", s.requireAdmin(s.handleGetRules))
	s.mux.HandleFunc("POST /rules", s.requireAdmin(s.handlePostRules))

	return s, nil
}

// Run listens on the configured port and serves until ctx is cancelled.
// See Serve for the shutdown sequence.
func (s *Server) Run(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	return s.Serve(ctx, ln)
}

// Serve runs the HTTP server on ln and the background scan loop until ctx
// is cancelled, then shuts down gracefully. It stops accepting connections,
// waits up to the drain timeout for in-flight requests and the scan loop,
// cancels the notification sends of anything still running, and writes the
// state file. It returns nil after a clean shutdown, and an error if the
// server fails or the final save fails. Serve takes ownership of ln.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	defer s.cancelWork()

	tls := s.cfg.TLSCert != "" && s.cfg.TLSKey != ""
	if tls {
		log.Printf("shout! v%s (%s) listening on %s (TLS)", version.Version(), version.Release, ln.Addr())
	} else {
		log.Printf("shout! v%s (%s) listening on %s", version.Version(), version.Release, ln.Addr())
	}

	srv := &http.Server{
		Handler:           s.trackInflight(s.mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	scanCtx, stopScan := context.WithCancel(ctx)
	defer stopScan()
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		s.scanLoop(scanCtx)
	}()

	serveErr := make(chan error, 1)
	go func() {
		if tls {
			serveErr <- srv.ServeTLS(ln, s.cfg.TLSCert, s.cfg.TLSKey)
		} else {
			serveErr <- srv.Serve(ln)
		}
	}()

	var runErr error
	select {
	case err := <-serveErr:
		// The server stopped on its own, for example because the TLS
		// key pair could not be loaded. Save what we have and fail.
		runErr = fmt.Errorf("serving: %w", err)
		stopScan()
		s.cancelWork()
		_ = srv.Close()
	case <-ctx.Done():
		log.Printf("shutting down: %v", context.Cause(ctx))
	}

	if runErr == nil {
		drainCtx, cancelDrain := context.WithTimeout(context.Background(), s.drainTimeout)
		if err := srv.Shutdown(drainCtx); err != nil {
			log.Printf("shutdown: in-flight requests did not finish within %s: %v", s.drainTimeout, err)
		}
		select {
		case <-scanDone:
		case <-drainCtx.Done():
		}
		if drainCtx.Err() != nil {
			log.Printf("shutdown: cancelling in-flight notifications")
			s.cancelWork()
			_ = srv.Close()
		}
		cancelDrain()
		if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("shutdown: server error: %v", err)
		}
	}

	graceCtx, cancelGrace := context.WithTimeout(context.Background(), s.cancelGrace)
	defer cancelGrace()
	select {
	case <-s.beginClosing():
	case <-graceCtx.Done():
		log.Printf("shutdown: requests still running after %s; saving state without them", s.cancelGrace)
	}
	select {
	case <-scanDone:
	case <-graceCtx.Done():
		log.Printf("shutdown: scan loop still running; saving state without it")
	}

	if err := s.states.Save(); err != nil {
		saveErr := fmt.Errorf("saving state on shutdown: %w", err)
		if runErr != nil {
			return errors.Join(runErr, saveErr)
		}
		return saveErr
	}
	if runErr != nil {
		return runErr
	}
	log.Printf("graceful shutdown complete")
	return nil
}

// trackInflight counts running handlers so that a shutdown can wait for
// them. Once a shutdown has begun closing, new requests get a 503.
func (s *Server) trackInflight(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.inflightMu.Lock()
		if s.closing {
			s.inflightMu.Unlock()
			w.Header().Set("Connection", "close")
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "shutting down"})
			return
		}
		s.inflight++
		s.inflightMu.Unlock()

		defer func() {
			s.inflightMu.Lock()
			s.inflight--
			if s.closing && s.inflight == 0 {
				close(s.idle)
			}
			s.inflightMu.Unlock()
		}()
		next.ServeHTTP(w, r)
	})
}

// beginClosing refuses further requests and returns a channel that is
// closed once no handler is running.
func (s *Server) beginClosing() <-chan struct{} {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	if !s.closing {
		s.closing = true
		if s.inflight == 0 {
			close(s.idle)
		}
	}
	return s.idle
}

func (s *Server) scanLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Serve writes the final state once requests have drained.
			return
		case <-ticker.C:
			s.states.Expire()
			if err := s.states.Save(); err != nil {
				log.Printf("error saving state: %v", err)
			}
			for _, ts := range s.states.BrokenTopics() {
				if ctx.Err() != nil {
					return
				}
				s.fireRules(ts.LastEvent, "still broken")
				s.states.MarkNotified(ts.Name)
			}
		}
	}
}

func (s *Server) loadRulesFromFile(path string) error {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("reading rules file: %w", err)
	}
	return s.loadRules(data)
}

func (s *Server) loadRules(data []byte) error {
	var cfg engine.RulesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parsing rules: %w", err)
	}

	eng, err := engine.Compile(&cfg)
	if err != nil {
		return fmt.Errorf("compiling rules: %w", err)
	}

	s.engineMu.Lock()
	s.engine = eng
	s.rulesSource = make([]byte, len(data))
	copy(s.rulesSource, data)
	s.engineMu.Unlock()
	return nil
}

func (s *Server) fireRules(evt *state.Event, status string) {
	s.engineMu.RLock()
	eng := s.engine
	s.engineMu.RUnlock()

	if eng == nil {
		return
	}

	results := eng.Evaluate(evt.Topic, status, evt.Message, evt.Link, evt.OK, evt.Metadata)
	for _, r := range results {
		h, ok := s.handlers.Get(r.Handler)
		if !ok {
			log.Printf("unknown handler %q in rules", r.Handler)
			continue
		}
		if err := h.Send(s.work, r.Args); err != nil {
			// Handler errors can quote the target URL, and the URL holds
			// the webhook secret, so mask it before it reaches the log.
			log.Printf("handler %q error: %s", r.Handler, redact.String(err.Error()))
		}
		if r.Reminder > 0 {
			s.states.SetReminder(evt.Topic, int64(r.Reminder.Seconds()))
		}
	}
}

// --- HTTP Handlers ---

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"version": version.Version(),
		"release": version.Release,
	})
}

func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	var evt state.Event
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if evt.Topic == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing topic"})
		return
	}

	transition := s.states.Ingest(&evt)
	if transition != "" {
		status := "now " + transition
		s.fireRules(&evt, status)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         transition != "" || evt.OK,
		"transition": transition,
	})
}

func (s *Server) handleAnnouncement(w http.ResponseWriter, r *http.Request) {
	var evt state.Event
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if evt.Topic == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing topic"})
		return
	}

	// Announcements always fire rules, regardless of state.
	evt.OK = true
	s.fireRules(&evt, "announcement")

	writeJSON(w, http.StatusOK, map[string]string{"ok": "sent"})
}

func (s *Server) handleGetState(w http.ResponseWriter, r *http.Request) {
	topic := r.URL.Query().Get("topic")
	if topic == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing topic query parameter"})
		return
	}
	ts := s.states.Get(topic)
	if ts == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown topic"})
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

func (s *Server) handleGetStates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.states.All())
}

func (s *Server) handleGetRules(w http.ResponseWriter, r *http.Request) {
	s.engineMu.RLock()
	src := s.rulesSource
	s.engineMu.RUnlock()

	if src == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "no rules loaded"})
		return
	}
	w.Header().Set("Content-Type", "text/x-yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(src)
}

func (s *Server) handlePostRules(w http.ResponseWriter, r *http.Request) {
	data, err := readBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.loadRules(data); err != nil {
		// Parse errors can quote the offending rule, which may hold a
		// webhook URL, so mask URL secrets before answering.
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": redact.String(err.Error())})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Auth Middleware ---

func (s *Server) requireOps(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(s.cfg.OpsCreds, next)
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(s.cfg.AdminCreds, next)
}

func (s *Server) requireAuth(creds string, next http.HandlerFunc) http.HandlerFunc {
	if creds == "" {
		return next
	}
	parts := splitCreds(creds)
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != parts[0] || pass != parts[1] {
			w.Header().Set("WWW-Authenticate", `Basic realm="shout"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func splitCreds(creds string) [2]string {
	if i := len(creds); i > 0 {
		for j := 0; j < i; j++ {
			if creds[j] == ':' {
				return [2]string{creds[:j], creds[j+1:]}
			}
		}
	}
	return [2]string{creds, ""}
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, 1<<20))
}
