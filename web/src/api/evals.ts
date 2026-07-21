// Evaluation center API calls (tickets 08/09).
import { http } from './client'
import type { EvalCase, EvalRun, EvalRunDetail, LatestScore, Suite, VerdictType, RuleConfig } from './types'

export async function listSuites(): Promise<Suite[]> {
  return http.get<Suite[]>('/suites')
}

export interface CasePayload {
  suite_id: number
  prompt: string
  verdict_type: VerdictType
  rule_config?: RuleConfig | null
  rubric?: string | null
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

export async function createEvalRun(suiteId: number, modelIds: number[]): Promise<EvalRun> {
  return http.post<EvalRun>('/evals', { suite_id: suiteId, model_ids: modelIds })
}

export async function listLatestScores(): Promise<LatestScore[]> {
  return http.get<LatestScore[]>('/evals/latest')
}
