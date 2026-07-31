// Endpoint detail and time-series API calls (ticket 04).
import { http } from './client'
import type { EndpointDetail, SeriesBucket, SeriesStreaming, Endpoint } from './types'

export async function getEndpointDetail(endpointId: number): Promise<EndpointDetail> {
  return http.get<EndpointDetail>(`/endpoints/${endpointId}`)
}

export async function getEndpointSeries(
  endpointId: number,
  hours: number,
  streaming: SeriesStreaming
): Promise<SeriesBucket[]> {
  return http.get<SeriesBucket[]>(
    `/endpoints/${endpointId}/series?hours=${hours}&streaming=${streaming}`
  )
}

// Delete an endpoint together with its probe history and alert events.
export async function deleteEndpoint(endpointId: number): Promise<void> {
  await http.del<void>(`/endpoints/${endpointId}`)
}

// Prune every disabled endpoint that never had a successful probe (legacy
// failed-trial placeholders), with their history. Returns the count removed.
export async function pruneDeadEndpoints(): Promise<{ pruned: number }> {
  return http.post<{ pruned: number }>('/endpoints/prune-dead')
}

// Update an endpoint's enabled state or probe interval (GH #99).
export async function updateEndpoint(
  endpointId: number,
  payload: { enabled?: boolean; interval_seconds?: number | null }
): Promise<Endpoint> {
  return http.patch<Endpoint>(`/endpoints/${endpointId}`, payload)
}
