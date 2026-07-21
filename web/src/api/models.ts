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
