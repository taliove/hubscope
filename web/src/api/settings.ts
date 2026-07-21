// Settings and alert-events API calls.
import { http } from './client'

export interface AppSettings {
  lark_webhook_url: string
  alert_enabled: boolean
  score_drop_alert_enabled: boolean
  judge_model: string
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
