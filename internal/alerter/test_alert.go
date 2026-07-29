package alerter

import (
	"context"
	"log/slog"

	"github.com/taliove/hubscope/internal/store"
)

// TestMessage is the fixed text sent by the manual channel check (ticket
// 100). A fixed wording keeps the SSRF surface (super_admin may point the
// sender at any http(s) URL) limited to one known payload.
const TestMessage = "【HubScope】测试消息:告警通道配置成功。"

// testMessage renders the fixed test message as a turquoise-header card
// (ticket 101); the Text member stays TestMessage verbatim so the recorded
// event is unchanged.
func testMessage() Message {
	return Message{
		Text:     TestMessage,
		Title:    "测试消息",
		Template: templateTurquoise,
		Fields:   []Field{{Label: "内容", Value: "告警通道配置成功。"}},
	}
}

// SendTest delivers the fixed test message to the given webhook URL and
// records the attempt as an alert event with kind="test" (endpoint_id NULL)
// so the admin can see the channel check in the alert history — the failure
// is recorded too (sent_ok=false), and the send error is returned so the
// caller can report the reason. The webhook address never appears in logs or
// in the returned error (LarkSender strips url.Error, W6).
//
// Like HandleLoginFailures this method takes no lock and touches no alerted
// state: it has no per-endpoint flag to protect, and the caller (an admin
// request) bounds the wait by the sender timeout. It is also independent of
// the alert_enabled switch and the saved webhook setting — the manual test
// exists precisely to verify an address before saving or enabling it.
func (e *Evaluator) SendTest(ctx context.Context, webhookURL string) error {
	sentOK := true
	sendErr := e.sender.Send(ctx, webhookURL, testMessage())
	if sendErr != nil {
		slog.Error("alerter: send test message", "error", sendErr)
		sentOK = false
	} else {
		slog.Info("alerter: test message sent")
	}

	if _, err := e.db.CreateAlertEvent(store.AlertEvent{
		Kind:    store.AlertKindTest,
		Message: TestMessage,
		SentOK:  sentOK,
	}); err != nil {
		slog.Error("alerter: record test alert event", "error", err)
	}
	return sendErr
}
