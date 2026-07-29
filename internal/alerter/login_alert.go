package alerter

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// LoginFailureSnapshot is the frozen view of the failed-login window handed
// to the alert sender at trigger time: the instance-wide failure count, the
// window it was counted over, and the top-3 usernames and source IPs. It
// carries audit-level information only — never passwords (W6).
type LoginFailureSnapshot struct {
	Count        int
	Window       time.Duration
	TopUsernames []string
	TopIPs       []string
}

// HandleLoginFailures sends one brute-force login alert for the given
// snapshot (spec 0011 decision 4). It never blocks the caller: the settings
// lookup and the webhook send run in a detached goroutine on
// context.Background() — this hook fires on the login request path, whose
// latency budget excludes alerting, and a request context dies with its
// handler. The sender's own client timeout bounds the attempt.
//
// Unlike HandleRound/HandleCampaign this method takes no lock and touches no
// alerted state: throttling is the caller's job (the server-side tracker
// consumes the cooldown before invoking this hook, whether or not a webhook
// is configured or the send later succeeds). The send is not retried on
// failure and nothing is recorded in alert_events — the persisted-event
// semantics exist to suppress repeat alerts across restarts, which the
// in-memory cooldown already covers here.
func (e *Evaluator) HandleLoginFailures(snapshot LoginFailureSnapshot) {
	go func() {
		webhook, err := e.db.GetSetting(store.SettingLarkWebhookURL, "")
		if err != nil {
			slog.Error("alerter: read webhook setting", "error", err)
			return
		}
		enabled, err := e.db.GetSettingBool(store.SettingAlertEnabled, store.DefaultAlertEnabled)
		if err != nil {
			slog.Error("alerter: read alert_enabled setting", "error", err)
			return
		}
		if webhook == "" || !enabled {
			slog.Debug("alerter: login brute-force alert skipped (webhook not configured or alerts disabled)", "count", snapshot.Count)
			return
		}

		if err := e.sender.Send(context.Background(), webhook, buildLoginAlertMessage(snapshot)); err != nil {
			slog.Error("alerter: send login brute-force alert", "count", snapshot.Count, "error", err)
		} else {
			slog.Info("alerter: login brute-force alert sent", "count", snapshot.Count)
		}
	}()
}

// buildLoginAlertMessage composes the alert — once, as a Message whose plain
// text mirrors the historical wording and whose fields render the card
// (ticket 101): the failure count over the window plus the most-attempted
// usernames and most frequent source IPs.
func buildLoginAlertMessage(s LoginFailureSnapshot) Message {
	var b strings.Builder
	fmt.Fprintf(&b, "【HubScope】登录爆破告警:最近 %s 内全站登录失败 %d 次,疑似暴力破解,请检查审计日志。",
		loginAlertWindowText(s.Window), s.Count)
	fmt.Fprintf(&b, "\n被尝试最多的用户名:%s", joinOrDash(s.TopUsernames))
	fmt.Fprintf(&b, "\n失败最多的来源 IP:%s", joinOrDash(s.TopIPs))
	return Message{
		Text:     b.String(),
		Title:    "登录爆破告警",
		Template: templateRed,
		Fields: []Field{
			{Label: "失败次数", Value: fmt.Sprintf("%d 次", s.Count)},
			{Label: "统计窗口", Value: loginAlertWindowText(s.Window)},
			{Label: "被尝试最多的用户名", Value: joinOrDash(s.TopUsernames)},
			{Label: "失败最多的来源 IP", Value: joinOrDash(s.TopIPs)},
		},
		Detail: "疑似暴力破解,请检查审计日志。",
	}
}

// loginAlertWindowText renders the window in whole minutes when possible
// ("10 分钟"), falling back to the duration string for sub-minute or
// fractional values (millisecond-scale test policies).
func loginAlertWindowText(d time.Duration) string {
	if d >= time.Minute && d%time.Minute == 0 {
		return fmt.Sprintf("%d 分钟", int(d/time.Minute))
	}
	return d.String()
}

// joinOrDash comma-joins the entries, or answers a placeholder when the
// window carried none (e.g., every attempt had an empty username).
func joinOrDash(entries []string) string {
	if len(entries) == 0 {
		return "-"
	}
	return strings.Join(entries, ", ")
}
