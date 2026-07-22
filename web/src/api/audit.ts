// Audit log API calls (ticket 14).
import { http } from './client'
import type { AuditLogPage } from './types'

export interface AuditLogQuery {
  page?: number
  page_size?: number
  action?: string
}

export async function listAuditLogs(query: AuditLogQuery): Promise<AuditLogPage> {
  const params = new URLSearchParams()
  if (query.page) params.set('page', String(query.page))
  if (query.page_size) params.set('page_size', String(query.page_size))
  if (query.action) params.set('action', query.action)
  const suffix = params.toString() ? `?${params}` : ''
  return http.get<AuditLogPage>(`/audit-logs${suffix}`)
}

export async function listAuditActions(): Promise<string[]> {
  return http.get<string[]>('/audit-logs/actions')
}
