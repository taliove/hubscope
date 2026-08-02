// Settings and alert-events API calls.
import { http } from './client'

export interface AppSettings {
  lark_webhook_url: string
  alert_enabled: boolean
  score_drop_alert_enabled: boolean
  judge_model: string
  default_sample_count: number
  // Eval worker-pool size: how many (suite × model) cells run at once
  // (GH #26); 1-16, default 4.
  eval_concurrency: number
  // Campaign wall-clock budget in minutes (GH #153): a batch outliving it
  // drops unstarted cells and settles failed; 0 disables. Default 120.
  eval_campaign_budget_minutes: number
  // Leaderboard total-score weight per suite key (ticket 31); suites absent
  // from the map weigh 1 (equal weighting is the default).
  suite_weights: Record<string, number>
  // Quiet hours (spec 0017 ticket 4): inside the daily window alert sends
  // are held and a summary goes out when the window ends. Hours are
  // integers 0–23 in the server's local timezone; start == end means "not
  // enabled" even when the switch is on.
  quiet_hours_enabled: boolean
  quiet_hours_start: number
  quiet_hours_end: number
}

export type UpdateSettingsPayload = Partial<AppSettings>

// Alert event kinds: the five legacy kinds plus the spec 0017 noise-
// reduction kinds — group_down / group_recovered (vendor group state
// transitions), batch (one aggregated window flush delivery), quiet_summary
// (quiet-hours end summary delivery) — plus the spec 0018 T4 retirement
// kinds — retire_pending (chat endpoint enters 72h-no-success pending state,
// warning level), retired (models disappeared from Hub listing causing
// retirement, info level, aggregated per sync batch).
export type AlertKind =
  | 'down'
  | 'recovered'
  | 'score_drop'
  | 'score_drop_skipped'
  | 'test'
  | 'group_down'
  | 'group_recovered'
  | 'batch'
  | 'quiet_summary'
  | 'retire_pending'
  | 'retired'

export interface AlertEvent {
  id: number
  endpoint_id: number | null
  kind: AlertKind
  message: string
  sent_ok: boolean
  created_at: string
  // Vendor family name; non-null only on group_down / group_recovered
  // (spec 0017 ticket 3). Null on every endpoint- or hub-scoped event.
  group_key: string | null
}

// Result of POST /api/settings/test-lark (ticket 100): error carries the
// send failure reason, never the webhook URL.
export interface TestLarkResult {
  sent_ok: boolean
  error: string | null
}

export async function getSettings(): Promise<AppSettings> {
  return http.get<AppSettings>('/settings')
}

export async function updateSettings(payload: UpdateSettingsPayload): Promise<AppSettings> {
  return http.put<AppSettings>('/settings', payload)
}

export async function listAlerts(limit = 50): Promise<AlertEvent[]> {
  return http.get<AlertEvent[]>(`/alerts?limit=${limit}`)
}

// Sends the fixed test message to the given webhook address (the form value,
// not the saved setting); every attempt records a kind="test" alert event.
export async function testLark(webhookUrl: string): Promise<TestLarkResult> {
  return http.post<TestLarkResult>('/settings/test-lark', { webhook_url: webhookUrl })
}
