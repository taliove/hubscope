// Evaluation center API calls (tickets 08/09).
import { http } from './client'
import type { CampaignDetail, EvalCase, EvalRun, EvalRunDetail, Suite, VerdictType, RuleConfig, Difficulty, ModelEvalSummary } from './types'

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

// Triggering an eval creates a campaign; the response is the created
// campaign (its runs may still be pending creation). TriggerEvalPayload is
// the sweep form: both lists omitted is the one-click full sweep, suite_ids
// narrows the enabled-suite rotation, model_ids overrides the eval_enabled
// candidate list (an omitted dimension takes its server default).
export interface TriggerEvalPayload {
  suite_ids?: number[]
  model_ids?: number[]
}

export async function triggerEval(payload: TriggerEvalPayload): Promise<CampaignDetail> {
  return http.post<CampaignDetail>('/evals', payload)
}

export async function createFullSweep(): Promise<CampaignDetail> {
  return http.post<CampaignDetail>('/evals', {})
}

// Get the latest evaluation summary for a specific model (ticket 60.1).
// Returns null when the model has never been evaluated.
export async function getModelEvalSummary(modelId: number): Promise<ModelEvalSummary | null> {
  return http.get<ModelEvalSummary | null>(`/models/${modelId}/eval-summary`)
}
