// Package slack implements a notifier.Notifier for Slack webhooks.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Strob0t/CodeForge/internal/port/notifier"
)

const providerName = "slack"

// Notifier sends notifications to Slack via incoming webhook.
type Notifier struct {
	webhookURL string
	httpClient *http.Client
}

// NewNotifier creates a Slack notifier with the given webhook URL.
func NewNotifier(webhookURL string) *Notifier {
	return &Notifier{
		webhookURL: webhookURL,
		httpClient: http.DefaultClient,
	}
}

func (n *Notifier) Name() string { return providerName }

func (n *Notifier) Capabilities() notifier.Capabilities {
	return notifier.Capabilities{
		RichFormatting: true,
		Threads:        false,
	}
}

// slackMessage is the Slack Block Kit message payload.
type slackMessage struct {
	Blocks []slackBlock `json:"blocks"`
}

type slackBlock struct {
	Type string     `json:"type"`
	Text *slackText `json:"text,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (n *Notifier) Send(ctx context.Context, notification notifier.Notification) error {
	if n.webhookURL == "" {
		return notifier.ErrNotConfigured
	}

	emoji := levelEmoji(notification.Level)
	headerText := fmt.Sprintf("%s %s", emoji, notification.Title)

	msg := slackMessage{
		Blocks: []slackBlock{
			{Type: "header", Text: &slackText{Type: "plain_text", Text: headerText}},
			{Type: "section", Text: &slackText{Type: "mrkdwn", Text: notification.Message}},
		},
	}

	if notification.Source != "" {
		msg.Blocks = append(msg.Blocks, slackBlock{
			Type: "context",
			Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf("_Source: %s_", notification.Source)},
		})
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("slack marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req) //nolint:gosec // webhook URL from trusted config
	if err != nil {
		return fmt.Errorf("slack send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack API %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func levelEmoji(level string) string {
	switch level {
	case "success":
		return "[OK]"
	case "error":
		return "[ERROR]"
	case "warning":
		return "[WARN]"
	default:
		return "[INFO]"
	}
}
