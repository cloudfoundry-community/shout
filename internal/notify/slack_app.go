package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// SlackAppHandler sends notifications via the Slack Web API (chat.postMessage).
// Unlike the webhook-based SlackHandler, this uses Bearer token authentication
// and requires a channel parameter.
type SlackAppHandler struct {
	defaultToken   string
	defaultChannel string
}

// NewSlackAppHandler creates a SlackAppHandler with defaults from environment variables.
func NewSlackAppHandler() *SlackAppHandler {
	return &SlackAppHandler{
		defaultToken:   os.Getenv("SHOUT_SLACK_TOKEN"),
		defaultChannel: os.Getenv("SHOUT_SLACK_CHANNEL"),
	}
}

func (s *SlackAppHandler) Name() string { return "slack-app" }

const slackAPIURL = "https://slack.com/api/chat.postMessage"

func (s *SlackAppHandler) Send(ctx context.Context, args map[string]string) error {
	return s.sendToURL(ctx, slackAPIURL, args)
}

func (s *SlackAppHandler) sendToURL(ctx context.Context, url string, args map[string]string) error {
	token := args["token"]
	if token == "" {
		token = s.defaultToken
	}
	if token == "" {
		return fmt.Errorf("slack-app handler: missing token (set SHOUT_SLACK_TOKEN or pass token arg)")
	}

	channel := args["channel"]
	if channel == "" {
		channel = s.defaultChannel
	}
	if channel == "" {
		return fmt.Errorf("slack-app handler: missing channel (set SHOUT_SLACK_CHANNEL or pass channel arg)")
	}

	payload := map[string]any{
		"channel": channel,
	}

	if text := args["text"]; text != "" {
		payload["text"] = text
	}
	if username := args["username"]; username != "" {
		payload["username"] = username
	}
	if icon := args["icon_url"]; icon != "" {
		payload["icon_url"] = icon
	}

	if attachments := buildAttachments(args); attachments != nil {
		payload["attachments"] = attachments
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack-app handler: marshaling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack-app handler: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack-app handler: sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("slack-app handler: reading response: %w", err)
	}

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("slack-app handler: parsing response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("slack-app handler: Slack API error: %s", result.Error)
	}

	return nil
}

// buildAttachments creates a Slack attachment array from color/attach args.
// Returns nil if neither color nor attach is set.
func buildAttachments(args map[string]string) []map[string]string {
	color := args["color"]
	attachText := args["attach"]
	if color == "" && attachText == "" {
		return nil
	}

	attachment := map[string]string{}
	if color != "" {
		attachment["color"] = color
	}
	if attachText != "" {
		attachment["text"] = attachText
	}
	return []map[string]string{attachment}
}
