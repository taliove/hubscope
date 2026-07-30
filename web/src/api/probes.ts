// Probe execution and history API calls.
import { http } from './client'
import type { ProbeRecord, ProbeRunResult } from './types'

export async function triggerProbe(endpointId: number): Promise<ProbeRunResult> {
  return http.post<ProbeRunResult>(`/endpoints/${endpointId}/probe`)
}

export async function listProbeHistory(
  endpointId: number,
  limit = 50,
  ok?: boolean,
  hours?: number
): Promise<ProbeRecord[]> {
  // hours (2026-07-30, quick-view latency-detail curve) opens a time window
  // server-side (row cap 2000) and takes priority over limit per the contract
  // — don't send limit in window mode.
  let url =
    hours !== undefined
      ? `/endpoints/${endpointId}/probes?hours=${hours}`
      : `/endpoints/${endpointId}/probes?limit=${limit}`
  // Optional ok filter, used by the recent-failures list on the detail page.
  if (ok !== undefined) url += `&ok=${ok}`
  return http.get<ProbeRecord[]>(url)
}
