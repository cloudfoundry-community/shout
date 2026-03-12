package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSlackAppHandlerName(t *testing.T) {
	h := NewSlackAppHandler()
	if h.Name() != "slack-app" {
		t.Errorf("expected name=slack-app, got %q", h.Name())
	}
}

func TestSlackAppHandlerMissingToken(t *testing.T) {
	h := &SlackAppHandler{}
	err := h.Send(context.Background(), map[string]string{"channel": "#test"})
	if err == nil {
		t.Error("expected error for missing token")
	}
}

func TestSlackAppHandlerMissingChannel(t *testing.T) {
	h := &SlackAppHandler{defaultToken: "xoxb-test"}
	err := h.Send(context.Background(), map[string]string{})
	if err == nil {
		t.Error("expected error for missing channel")
	}
}

func TestSlackAppHandlerSendsCorrectPayload(t *testing.T) {
	var received map[string]any
	var authHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		authHeader = r.Header.Get("Authorization")
		if ct := r.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("expected Content-Type application/json; charset=utf-8, got %q", ct)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	// Use a handler that points to our test server instead of slack.com.
	h := &SlackAppHandler{}
	err := sendSlackApp(h, srv.URL, map[string]string{
		"token":    "xoxb-test-token",
		"channel":  "#ci-alerts",
		"text":     "test message",
		"username": "shout-bot",
		"icon_url": "https://example.com/icon.png",
		"color":    "#ff0000",
		"attach":   "attachment text",
	})
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if authHeader != "Bearer xoxb-test-token" {
		t.Errorf("expected Authorization=Bearer xoxb-test-token, got %q", authHeader)
	}
	if received["channel"] != "#ci-alerts" {
		t.Errorf("expected channel=#ci-alerts, got %v", received["channel"])
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

func TestSlackAppHandlerAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":false,"error":"channel_not_found"}`))
	}))
	defer srv.Close()

	h := &SlackAppHandler{}
	err := sendSlackApp(h, srv.URL, map[string]string{
		"token":   "xoxb-test",
		"channel": "#nonexistent",
		"text":    "test",
	})
	if err == nil {
		t.Error("expected error for Slack API error response")
	}
	if err != nil && err.Error() != "slack-app handler: Slack API error: channel_not_found" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSlackAppHandlerEnvVarDefaults(t *testing.T) {
	var authHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	h := &SlackAppHandler{
		defaultToken:   "xoxb-env-token",
		defaultChannel: "#env-channel",
	}
	err := sendSlackApp(h, srv.URL, map[string]string{
		"text": "test",
	})
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if authHeader != "Bearer xoxb-env-token" {
		t.Errorf("expected env var token, got %q", authHeader)
	}
}

func TestSlackAppHandlerArgOverridesEnv(t *testing.T) {
	var authHeader string
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&received)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	h := &SlackAppHandler{
		defaultToken:   "xoxb-env-token",
		defaultChannel: "#env-channel",
	}
	err := sendSlackApp(h, srv.URL, map[string]string{
		"token":   "xoxb-override-token",
		"channel": "#override-channel",
		"text":    "test",
	})
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if authHeader != "Bearer xoxb-override-token" {
		t.Errorf("expected override token, got %q", authHeader)
	}
	if received["channel"] != "#override-channel" {
		t.Errorf("expected override channel, got %v", received["channel"])
	}
}

func TestSlackAppHandlerTextOnly(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	h := &SlackAppHandler{defaultToken: "xoxb-test", defaultChannel: "#test"}
	err := sendSlackApp(h, srv.URL, map[string]string{
		"text": "just text",
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

// sendSlackApp is a test helper that sends via SlackAppHandler but overrides the URL
// to point at a test server instead of slack.com.
func sendSlackApp(h *SlackAppHandler, url string, args map[string]string) error {
	// Resolve token and channel using the handler's defaults
	token := args["token"]
	if token == "" {
		token = h.defaultToken
	}
	channel := args["channel"]
	if channel == "" {
		channel = h.defaultChannel
	}

	// Build the same args but inject the resolved values and use sendToURL
	resolved := make(map[string]string, len(args))
	for k, v := range args {
		resolved[k] = v
	}
	resolved["token"] = token
	resolved["channel"] = channel

	return h.sendToURL(context.Background(), url, resolved)
}
