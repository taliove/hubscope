// Probe execution and history API calls.
import { http } from './client'
import type { ProbeRecord, ProbeRunResult } from './types'

export async function triggerProbe(endpointId: number): Promise<ProbeRunResult> {
  return http.post<ProbeRunResult>(`/endpoints/${endpointId}/probe`)
}

export async function listProbeHistory(
  endpointId: number,
  limit = 50,
  ok?: boolean
): Promise<ProbeRecord[]> {
  let url = `/endpoints/${endpointId}/probes?limit=${limit}`
  // Optional ok filter, used by the recent-failures list on the detail page.
  if (ok !== undefined) url += `&ok=${ok}`
  return http.get<ProbeRecord[]>(url)
}
