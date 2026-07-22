// Evaluation center API calls (tickets 08/09).
import { http } from './client'
import type { CampaignDetail, EvalCase, EvalRun, EvalRunDetail, Suite, VerdictType, RuleConfig, Difficulty } from './types'

export async function listSuites(): Promise<Suite[]> {
  return http.get<Suite[]>('/suites')
}

export interface CasePayload {
  suite_id: number
  prompt: string
  verdict_type: VerdictType
  rule_config?: RuleConfig | null
  rubric?: string | null
  difficulty?: Difficulty
  sample_count?: number | null
  enabled?: boolean
}

export async function createCase(payload: CasePayload): Promise<EvalCase> {
  return http.post<EvalCase>('/cases', payload)
}

export async function patchCase(id: number, payload: Partial<CasePayload>): Promise<EvalCase> {
  return http.patch<EvalCase>(`/cases/${id}`, payload)
}

export async function listEvalRuns(): Promise<EvalRun[]> {
  return http.get<EvalRun[]>('/evals')
}

export async function getEvalRun(id: number): Promise<EvalRunDetail> {
  return http.get<EvalRunDetail>(`/evals/${id}`)
}

// Triggering an eval — single-suite or full sweep — creates a campaign; the
// response is the created campaign (its runs may still be pending creation).
export async function createEvalRun(suiteId: number, modelIds: number[]): Promise<CampaignDetail> {
  return http.post<CampaignDetail>('/evals', { suite_id: suiteId, model_ids: modelIds })
}

export async function createFullSweep(): Promise<CampaignDetail> {
  return http.post<CampaignDetail>('/evals', {})
}
