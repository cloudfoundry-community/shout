package notify

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
)

// EmailHandler sends notifications via SMTP email.
type EmailHandler struct {
	host string
	port string
	from string
}

func NewEmailHandler() *EmailHandler {
	host := os.Getenv("SHOUT_SMTP_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("SHOUT_SMTP_PORT")
	if port == "" {
		port = "25"
	}
	from := os.Getenv("SHOUT_EMAIL_FROM")
	if from == "" {
		from = "shout@localhost"
	}
	return &EmailHandler{host: host, port: port, from: from}
}

func (e *EmailHandler) Name() string { return "email" }

func (e *EmailHandler) Send(_ context.Context, args map[string]string) error {
	to := args["to"]
	if to == "" {
		return fmt.Errorf("email handler: missing 'to' argument")
	}

	subject := args["subject"]
	if subject == "" {
		subject = "Shout! Notification"
	}

	body := args["body"]

	recipients := strings.Split(to, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		e.from,
		strings.Join(recipients, ", "),
		subject,
		body,
	)

	addr := net.JoinHostPort(e.host, e.port)
	if err := smtp.SendMail(addr, nil, e.from, recipients, []byte(msg)); err != nil {
		return fmt.Errorf("email handler: %w", err)
	}
	return nil
}
