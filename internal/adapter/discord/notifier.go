// Package discord implements a notifier.Notifier for Discord webhooks.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Strob0t/CodeForge/internal/port/notifier"
)

const providerName = "discord"

// Notifier sends notifications to Discord via incoming webhook.
type Notifier struct {
	webhookURL string
	httpClient *http.Client
}

// NewNotifier creates a Discord notifier with the given webhook URL.
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
		Threads:        true,
	}
}

// discordWebhook is the Discord webhook payload with embeds.
type discordWebhook struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Footer      *discordFooter `json:"footer,omitempty"`
}

type discordFooter struct {
	Text string `json:"text"`
}

func (n *Notifier) Send(ctx context.Context, notification notifier.Notification) error {
	if n.webhookURL == "" {
		return notifier.ErrNotConfigured
	}

	embed := discordEmbed{
		Title:       notification.Title,
		Description: notification.Message,
		Color:       levelColor(notification.Level),
	}

	if notification.Source != "" {
		embed.Footer = &discordFooter{Text: "Source: " + notification.Source}
	}

	msg := discordWebhook{
		Embeds: []discordEmbed{embed},
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("discord marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req) //nolint:gosec // webhook URL from trusted config
	if err != nil {
		return fmt.Errorf("discord send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Discord returns 204 on success
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord API %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// levelColor returns Discord embed color integers for notification levels.
func levelColor(level string) int {
	switch level {
	case "success":
		return 0x2ECC71 // green
	case "error":
		return 0xE74C3C // red
	case "warning":
		return 0xF39C12 // orange
	default:
		return 0x3498DB // blue (info)
	}
}
