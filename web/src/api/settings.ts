// Settings and alert-events API calls.
import { http } from './client'

export interface AppSettings {
  lark_webhook_url: string
  alert_enabled: boolean
  score_drop_alert_enabled: boolean
  judge_model: string
  default_sample_count: number
  // Leaderboard total-score weight per suite key (ticket 31); suites absent
  // from the map weigh 1 (equal weighting is the default).
  suite_weights: Record<string, number>
}

export type UpdateSettingsPayload = Partial<AppSettings>

export interface AlertEvent {
  id: number
  endpoint_id: number | null
  kind: 'down' | 'recovered' | 'score_drop'
  message: string
  sent_ok: boolean
  created_at: string
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
