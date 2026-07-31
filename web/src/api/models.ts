// Model and Endpoint management API calls.
import { http } from './client'
import type { Model, ModelTrialResult } from './types'

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

// Re-run the protocol trial for a model: an enabled endpoint is created per
// missing protocol that answers; failed trials create nothing. Mainly used
// to backfill endpoints for endpointless models.
export async function trialModel(modelId: number): Promise<ModelTrialResult> {
  return http.post<ModelTrialResult>(`/models/${modelId}/trial`)
}

// Update a model's capability (chat / image / video) and reconcile its
// endpoint set (GH #105, spec 0018 T7): missing protocols get created (chat
// via trial, image/video trial-free), surplus protocols get disabled with
// history preserved.
export async function updateModelCapability(modelId: number, capability: string): Promise<Model> {
  return http.patch<Model>(`/models/${modelId}`, { capability })
}
