package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSlackHandlerName(t *testing.T) {
	h := NewSlackHandler()
	if h.Name() != "slack" {
		t.Errorf("expected name=slack, got %q", h.Name())
	}
}

func TestSlackHandlerMissingWebhook(t *testing.T) {
	h := NewSlackHandler()
	err := h.Send(context.Background(), map[string]string{})
	if err == nil {
		t.Error("expected error for missing webhook")
	}
}

func TestSlackHandlerSendsCorrectPayload(t *testing.T) {
	var received map[string]any

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

	h := NewSlackHandler()
	err := h.Send(context.Background(), map[string]string{
		"webhook":  srv.URL,
		"text":     "test message",
		"username": "shout-bot",
		"icon_url": "https://example.com/icon.png",
		"color":    "#ff0000",
		"attach":   "attachment text",
	})
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if received["text"] != "test message" {
		t.Errorf("expected text='test message', got %v", received["text"])
	}
	if received["username"] != "shout-bot" {
		t.Errorf("expected username='shout-bot', got %v", received["username"])
	}
	if received["icon_url"] != "https://example.com/icon.png" {
		t.Errorf("expected icon_url, got %v", received["icon_url"])
	}

	attachments, ok := received["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %v", received["attachments"])
	}
	att := attachments[0].(map[string]any)
	if att["color"] != "#ff0000" {
		t.Errorf("expected color=#ff0000, got %v", att["color"])
	}
	if att["text"] != "attachment text" {
		t.Errorf("expected attachment text, got %v", att["text"])
	}
}

func TestSlackHandlerNon2xxResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := NewSlackHandler()
	err := h.Send(context.Background(), map[string]string{"webhook": srv.URL, "text": "test"})
	if err == nil {
		t.Error("expected error for non-2xx response")
	}
}

func TestSlackHandlerTextOnly(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := NewSlackHandler()
	err := h.Send(context.Background(), map[string]string{
		"webhook": srv.URL,
		"text":    "just text",
	})
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if received["text"] != "just text" {
		t.Errorf("expected text='just text', got %v", received["text"])
	}
	if _, ok := received["attachments"]; ok {
		t.Error("expected no attachments when no color/attach args")
	}
}
