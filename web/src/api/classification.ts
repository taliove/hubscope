// Classification-rule management API calls (ticket 13).
import { http } from './client'
import type { ClassificationRule } from './types'

export interface CreateRulePayload {
  dimension: 'capability' | 'family'
  keyword: string
  category: string
  priority?: number
}

export interface UpdateRulePayload {
  keyword?: string
  category?: string
  priority?: number
}

export async function listClassificationRules(): Promise<ClassificationRule[]> {
  return http.get<ClassificationRule[]>('/classification-rules')
}

export async function createClassificationRule(payload: CreateRulePayload): Promise<ClassificationRule> {
  return http.post<ClassificationRule>('/classification-rules', payload)
}

export async function updateClassificationRule(id: number, payload: UpdateRulePayload): Promise<ClassificationRule> {
  return http.patch<ClassificationRule>(`/classification-rules/${id}`, payload)
}

export async function deleteClassificationRule(id: number): Promise<void> {
  await http.del<void>(`/classification-rules/${id}`)
}
