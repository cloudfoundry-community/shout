package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cloudfoundry-community/shout/internal/engine"
	"github.com/cloudfoundry-community/shout/internal/notify"
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
}

// New creates a new API server.
func New(cfg Config, handlers *notify.Registry) (*Server, error) {
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		if cfg.TLSCert == "" {
			return nil, fmt.Errorf("SHOUT_TLS_KEY is set but SHOUT_TLS_CERT is missing")
		}
		return nil, fmt.Errorf("SHOUT_TLS_CERT is set but SHOUT_TLS_KEY is missing")
	}

	store := state.NewJSONFileStore(cfg.DBPath)
	var opts []state.ManagerOption
	if cfg.Expiry > 0 {
		opts = append(opts, state.WithExpiry(cfg.Expiry))
	}
	mgr, err := state.NewManager(store, opts...)
	if err != nil {
		return nil, fmt.Errorf("initializing state: %w", err)
	}

	s := &Server{
		cfg:      cfg,
		states:   mgr,
		handlers: handlers,
		mux:      http.NewServeMux(),
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

// Run starts the HTTP server and the background scan loop.
func (s *Server) Run(ctx context.Context) error {
	go s.scanLoop(ctx)

	addr := fmt.Sprintf(":%d", s.cfg.Port)
	tls := s.cfg.TLSCert != "" && s.cfg.TLSKey != ""
	if tls {
		log.Printf("shout! v%s (%s) listening on %s (TLS)", version.Version(), version.Release, addr)
	} else {
		log.Printf("shout! v%s (%s) listening on %s", version.Version(), version.Release, addr)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	if tls {
		return srv.ListenAndServeTLS(s.cfg.TLSCert, s.cfg.TLSKey)
	}
	return srv.ListenAndServe()
}

func (s *Server) scanLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = s.states.Save()
			return
		case <-ticker.C:
			s.states.Expire()
			if err := s.states.Save(); err != nil {
				log.Printf("error saving state: %v", err)
			}
			for _, ts := range s.states.BrokenTopics() {
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
		if err := h.Send(context.Background(), r.Args); err != nil {
			log.Printf("handler %q error: %v", r.Handler, err)
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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
