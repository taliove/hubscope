// Image-param-rule management API calls (GH #33, spec 0014): cost-saving
// parameters appended to image probe request bodies by model-name match.
import { http } from './client'
import type { ImageParamRule } from './types'

export interface CreateImageParamRulePayload {
  keyword: string
  params: Record<string, string>
  priority?: number
}

export interface UpdateImageParamRulePayload {
  keyword?: string
  params?: Record<string, string>
  priority?: number
}

export async function listImageParamRules(): Promise<ImageParamRule[]> {
  return http.get<ImageParamRule[]>('/image-param-rules')
}

export async function createImageParamRule(payload: CreateImageParamRulePayload): Promise<ImageParamRule> {
  return http.post<ImageParamRule>('/image-param-rules', payload)
}

export async function updateImageParamRule(
  id: number,
  payload: UpdateImageParamRulePayload
): Promise<ImageParamRule> {
  return http.patch<ImageParamRule>(`/image-param-rules/${id}`, payload)
}

export async function deleteImageParamRule(id: number): Promise<void> {
  await http.del<void>(`/image-param-rules/${id}`)
}
