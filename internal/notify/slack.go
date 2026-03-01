package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SlackHandler sends notifications via Slack incoming webhooks.
type SlackHandler struct{}

func NewSlackHandler() *SlackHandler {
	return &SlackHandler{}
}

func (s *SlackHandler) Name() string { return "slack" }

func (s *SlackHandler) Send(ctx context.Context, args map[string]string) error {
	webhook := args["webhook"]
	if webhook == "" {
		return fmt.Errorf("slack handler: missing webhook argument")
	}

	payload := map[string]any{}

	if text := args["text"]; text != "" {
		payload["text"] = text
	}
	if username := args["username"]; username != "" {
		payload["username"] = username
	}
	if icon := args["icon_url"]; icon != "" {
		payload["icon_url"] = icon
	}

	// Build attachment if color or attach text is provided.
	color := args["color"]
	attachText := args["attach"]
	if color != "" || attachText != "" {
		attachment := map[string]string{}
		if color != "" {
			attachment["color"] = color
		}
		if attachText != "" {
			attachment["text"] = attachText
		}
		payload["attachments"] = []map[string]string{attachment}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack handler: marshaling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack handler: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack handler: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack handler: unexpected status %d", resp.StatusCode)
	}
	return nil
}
