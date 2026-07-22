// Model and Endpoint management API calls.
import { http } from './client'
import type { Model } from './types'

export interface CreateModelPayload {
  hub_id: number
  model_id: string
}

export async function listModels(): Promise<Model[]> {
  return http.get<Model[]>('/models')
}

export async function createModel(payload: CreateModelPayload): Promise<Model> {
  return http.post<Model>('/models', payload)
}

// Delete a manual model with its endpoints and their history. Discovered
// models are rejected by the backend with 409 (disable them instead).
export async function deleteModel(modelId: number): Promise<void> {
  await http.del<void>(`/models/${modelId}`)
}
