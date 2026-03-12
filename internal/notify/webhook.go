package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// WebhookHandler sends notifications via generic HTTP POST webhooks.
type WebhookHandler struct{}

func NewWebhookHandler() *WebhookHandler {
	return &WebhookHandler{}
}

func (w *WebhookHandler) Name() string { return "webhook" }

func (w *WebhookHandler) Send(ctx context.Context, args map[string]string) error {
	url := args["url"]
	if url == "" {
		return fmt.Errorf("webhook handler: missing url argument")
	}

	body, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("webhook handler: marshaling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook handler: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook handler: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook handler: unexpected status %d", resp.StatusCode)
	}
	return nil
}
