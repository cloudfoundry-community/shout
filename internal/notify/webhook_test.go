package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookHandlerName(t *testing.T) {
	h := NewWebhookHandler()
	if h.Name() != "webhook" {
		t.Errorf("expected name=webhook, got %q", h.Name())
	}
}

func TestWebhookHandlerMissingURL(t *testing.T) {
	h := NewWebhookHandler()
	err := h.Send(context.Background(), map[string]string{})
	if err == nil {
		t.Error("expected error for missing url")
	}
}

func TestWebhookHandlerSendsAllArgs(t *testing.T) {
	var received map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := NewWebhookHandler()
	err := h.Send(context.Background(), map[string]string{
		"url":     srv.URL,
		"topic":   "test/pipeline",
		"status":  "broken",
		"message": "build failed",
	})
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if received["topic"] != "test/pipeline" {
		t.Errorf("expected topic=test/pipeline, got %q", received["topic"])
	}
	if received["status"] != "broken" {
		t.Errorf("expected status=broken, got %q", received["status"])
	}
}

func TestWebhookHandlerNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	h := NewWebhookHandler()
	err := h.Send(context.Background(), map[string]string{"url": srv.URL})
	if err == nil {
		t.Error("expected error for non-2xx response")
	}
}
