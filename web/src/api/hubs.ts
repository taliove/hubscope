// Hub management API calls.
import { http } from './client'
import type { Hub } from './types'

export interface CreateHubPayload {
  name: string
  base_url: string
  token: string
}

export interface UpdateHubPayload {
  name?: string
  base_url?: string
  token?: string // omit to keep existing token unchanged
}

export async function listHubs(): Promise<Hub[]> {
  return http.get<Hub[]>('/hubs')
}

export async function createHub(payload: CreateHubPayload): Promise<void> {
  await http.post<void>('/hubs', payload)
}

export async function updateHub(id: number, payload: UpdateHubPayload): Promise<Hub> {
  return http.put<Hub>(`/hubs/${id}`, payload)
}

export async function deleteHub(id: number): Promise<void> {
  await http.del<void>(`/hubs/${id}`)
}
