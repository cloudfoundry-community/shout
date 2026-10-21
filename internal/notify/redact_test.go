package notify

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const hookSecret = "xoxbHookSecret0123456789"

// deadAddr returns a local address that refuses connections.
func deadAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// secretURLs are delivery targets whose failures must not reveal the
// secret: a Slack hook path, a query token, user information, a redirect
// to a Slack hook, and a URL that does not parse.
func secretURLs(t *testing.T) map[string]string {
	t.Helper()
	dead := deadAddr(t)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+dead+"/services/T9/B9/"+hookSecret, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)
	return map[string]string{
		"slack path": "http://" + dead + "/services/T000/B000/" + hookSecret,
		"query":      "http://" + dead + "/hook?token=" + hookSecret,
		"userinfo":   "http://bot:" + hookSecret + "@" + dead + "/hook",
		"redirect":   redirector.URL + "/start",
		"unparsable": "http://[::1/services/T/B/" + hookSecret,
	}
}

func assertNoSecret(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a delivery error")
	}
	if strings.Contains(err.Error(), hookSecret) {
		t.Fatalf("error reveals the secret: %v", err)
	}
	if !strings.Contains(err.Error(), "REDACTED") {
		t.Errorf("error does not show where the URL was masked: %v", err)
	}
}

func TestSlackHandlerErrorsHideWebhookSecret(t *testing.T) {
	for name, u := range secretURLs(t) {
		t.Run(name, func(t *testing.T) {
			err := NewSlackHandler().Send(context.Background(), map[string]string{"webhook": u, "text": "hi"})
			assertNoSecret(t, err)
		})
	}
}

func TestWebhookHandlerErrorsHideURLSecret(t *testing.T) {
	for name, u := range secretURLs(t) {
		t.Run(name, func(t *testing.T) {
			err := NewWebhookHandler().Send(context.Background(), map[string]string{"url": u})
			assertNoSecret(t, err)
		})
	}
}

func TestSlackAppHandlerErrorsHideURLSecret(t *testing.T) {
	for name, u := range secretURLs(t) {
		t.Run(name, func(t *testing.T) {
			h := &SlackAppHandler{defaultToken: "xoxb-token", defaultChannel: "#c", apiURL: u}
			err := h.Send(context.Background(), map[string]string{"text": "hi"})
			assertNoSecret(t, err)
			if strings.Contains(err.Error(), "xoxb-token") {
				t.Fatalf("error reveals the bot token: %v", err)
			}
		})
	}
}
