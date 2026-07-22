// Package alerter watches probe rounds for endpoint up/down transitions and
// notifies a Lark group-bot webhook. Every attempted notification is recorded
// as an alert event so the history survives restarts and repeat alerts for
// one ongoing outage are suppressed.
package alerter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// sendTimeout bounds a single webhook call.
const sendTimeout = 10 * time.Second

// maxErrorBody caps how much of a failing webhook response is reported.
const maxErrorBody = 200

// LarkSender posts text messages to a Lark group-bot webhook.
type LarkSender struct {
	client *http.Client
}

// NewLarkSender creates a sender with a bounded request timeout.
func NewLarkSender() *LarkSender {
	return &LarkSender{client: &http.Client{Timeout: sendTimeout}}
}

// larkMessage is the group-bot text message envelope.
type larkMessage struct {
	MsgType string `json:"msg_type"`
	Content struct {
		Text string `json:"text"`
	} `json:"content"`
}

// Send delivers one text message to the given webhook URL. Any transport
// error or non-2xx response is returned as an error; the caller records it
// as sent_ok=false.
func (s *LarkSender) Send(ctx context.Context, webhookURL, text string) error {
	msg := larkMessage{MsgType: "text"}
	msg.Content.Text = text

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal lark message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build lark request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		// url.Error embeds the full webhook URL, which carries the bot
		// token. Log and return only the underlying cause.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return fmt.Errorf("post lark webhook: %w", urlErr.Err)
		}
		return fmt.Errorf("post lark webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return fmt.Errorf("lark webhook returned HTTP %d: %s", resp.StatusCode, string(snippet))
	}
	return nil
}
