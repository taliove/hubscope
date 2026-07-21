// Probe execution and history API calls.
import { http } from './client'
import type { ProbeRecord, ProbeRunResult } from './types'

export async function triggerProbe(endpointId: number): Promise<ProbeRunResult> {
  return http.post<ProbeRunResult>(`/endpoints/${endpointId}/probe`)
}

export async function listProbeHistory(
  endpointId: number,
  limit = 50
): Promise<ProbeRecord[]> {
  return http.get<ProbeRecord[]>(`/endpoints/${endpointId}/probes?limit=${limit}`)
}
