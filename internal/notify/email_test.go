package notify

import (
	"context"
	"testing"
)

func TestEmailHandlerName(t *testing.T) {
	h := NewEmailHandler()
	if h.Name() != "email" {
		t.Errorf("expected name=email, got %q", h.Name())
	}
}

func TestEmailHandlerMissingTo(t *testing.T) {
	h := NewEmailHandler()
	err := h.Send(context.Background(), map[string]string{
		"subject": "Test",
		"body":    "Hello",
	})
	if err == nil {
		t.Error("expected error for missing 'to' argument")
	}
}

func TestEmailHandlerDefaultSubject(t *testing.T) {
	// We can't easily test actual SMTP sending without a server,
	// but we can verify the handler doesn't panic on valid args.
	// The Send will fail because there's no SMTP server, but the
	// error should be from smtp.SendMail, not from our validation.
	h := &EmailHandler{host: "127.0.0.1", port: "0", from: "test@example.com"}
	err := h.Send(context.Background(), map[string]string{
		"to":   "user@example.com",
		"body": "Test body",
	})
	// Should fail with connection error, not validation error
	if err == nil {
		t.Error("expected error (no SMTP server)")
	}
	// Verify it's not a "missing 'to'" error
	if err.Error() == "email handler: missing 'to' argument" {
		t.Error("unexpected validation error — 'to' was provided")
	}
}

func TestEmailHandlerMultipleRecipients(t *testing.T) {
	h := &EmailHandler{host: "127.0.0.1", port: "0", from: "test@example.com"}
	err := h.Send(context.Background(), map[string]string{
		"to":      "a@example.com, b@example.com",
		"subject": "Multi",
		"body":    "Test",
	})
	// Should fail with connection error, not panic
	if err == nil {
		t.Error("expected error (no SMTP server)")
	}
}
