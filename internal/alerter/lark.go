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

// Card header colors (Lark template names), one per alert kind, mirroring the
// ui-guidelines §3 semantics: down and the login brute-force alert are the
// most urgent (red), a score drop is a warning (orange), a recovery is good
// news (green), and the manual channel check carries the brand color
// (turquoise).
const (
	templateRed       = "red"
	templateOrange    = "orange"
	templateGreen     = "green"
	templateTurquoise = "turquoise"
)

// Field is one labeled value rendered as a two-column cell on the card.
type Field struct {
	Label string
	Value string
}

// Message is one outbound alert, produced once per alert site so the two
// renderings can never drift apart (ticket 101 risk 4): Text is the plain
// text persisted to alert_events.message (the history table renders it
// unchanged), while Title/Template/Fields/Detail form the interactive card
// sent to Lark. Detail is an optional long-form lark_md block rendered
// between the fields and the note (e.g. per-case score-drop changes).
type Message struct {
	Text     string
	Title    string
	Template string
	Fields   []Field
	Detail   string
}

// LarkSender posts interactive-card messages to a Lark group-bot webhook.
type LarkSender struct {
	client *http.Client
}

// NewLarkSender creates a sender with a bounded request timeout.
func NewLarkSender() *LarkSender {
	return &LarkSender{client: &http.Client{Timeout: sendTimeout}}
}

// larkCardMessage is the group-bot interactive-card envelope. It uses the
// legacy card JSON schema (config/header/elements), the most compatible
// shape for custom bots (ticket 101 risk 1).
type larkCardMessage struct {
	MsgType string   `json:"msg_type"`
	Card    larkCard `json:"card"`
}

type larkCard struct {
	Config   larkCardConfig    `json:"config"`
	Header   larkCardHeader    `json:"header"`
	Elements []larkCardElement `json:"elements"`
}

type larkCardConfig struct {
	WideScreenMode bool `json:"wide_screen_mode"`
}

type larkCardHeader struct {
	Template string       `json:"template"`
	Title    larkCardText `json:"title"`
}

type larkCardText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

// larkCardElement covers the three element shapes one alert card uses: a div
// with two-column fields, a div with a long-form text block, an hr, and a
// note.
type larkCardElement struct {
	Tag      string          `json:"tag"`
	Fields   []larkCardField `json:"fields,omitempty"`
	Text     *larkCardText   `json:"text,omitempty"`
	Elements []larkCardText  `json:"elements,omitempty"`
}

type larkCardField struct {
	IsShort bool         `json:"is_short"`
	Text    larkCardText `json:"text"`
}

// newCardMessage renders one Message as an interactive card: a colored
// header (「{title} · HubScope」), the fields in two-column lark_md cells, an
// optional detail block, an hr, and a note with the send time and the
// service signature.
func newCardMessage(msg Message, now time.Time) larkCardMessage {
	elements := make([]larkCardElement, 0, 4)
	if len(msg.Fields) > 0 {
		fields := make([]larkCardField, 0, len(msg.Fields))
		for _, f := range msg.Fields {
			fields = append(fields, larkCardField{
				IsShort: true,
				Text:    larkCardText{Tag: "lark_md", Content: "**" + f.Label + "**\n" + f.Value},
			})
		}
		elements = append(elements, larkCardElement{Tag: "div", Fields: fields})
	}
	if msg.Detail != "" {
		elements = append(elements, larkCardElement{
			Tag:  "div",
			Text: &larkCardText{Tag: "lark_md", Content: msg.Detail},
		})
	}
	elements = append(elements, larkCardElement{Tag: "hr"})
	elements = append(elements, larkCardElement{
		Tag: "note",
		Elements: []larkCardText{{
			Tag:     "plain_text",
			Content: now.Format("2006-01-02 15:04:05") + " · HubScope 服务监控",
		}},
	})

	return larkCardMessage{
		MsgType: "interactive",
		Card: larkCard{
			Config: larkCardConfig{WideScreenMode: true},
			Header: larkCardHeader{
				Template: msg.Template,
				Title:    larkCardText{Tag: "plain_text", Content: msg.Title + " · HubScope"},
			},
			Elements: elements,
		},
	}
}

// Send delivers one message to the given webhook URL as an interactive card.
// Any transport error or non-2xx response is returned as an error; the
// caller records it as sent_ok=false.
func (s *LarkSender) Send(ctx context.Context, webhookURL string, msg Message) error {
	body, err := json.Marshal(newCardMessage(msg, time.Now()))
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
