// Endpoint detail and time-series API calls (ticket 04).
import { http } from './client'
import type { EndpointDetail, SeriesBucket, SeriesStreaming } from './types'

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
