import { ApiError } from '@/api/client'
import type { Model, Suite } from '@/api/types'

// Trigger-dialog selection helpers: the dialog offers every suite in the
// evaluation rotation (enabled; retired suites stay in the library for
// history, ADR 0010) and every model that joins sweeps by default (active,
// chat-capable, "join evaluations" switch on — GH #170). Non-chat, retired
// and opted-out models are not listed at all.

export interface EvalCandidates {
  suiteIds: number[]
  modelIds: number[]
}

// Compute the default selection: all enabled suites and all sweep-eligible
// models, in stable id order.
export function evalCandidates(
  suites: ReadonlyArray<Pick<Suite, 'id' | 'enabled'>>,
  models: ReadonlyArray<Pick<Model, 'id' | 'capability' | 'status' | 'eval_enabled'>>,
): EvalCandidates {
  return {
    suiteIds: suites.filter(s => s.enabled).map(s => s.id),
    modelIds: models
      .filter(m => m.capability === 'chat' && m.status === 'active' && m.eval_enabled)
      .map(m => m.id),
  }
}

export interface TriggerEvalBody {
  suite_ids?: number[]
  model_ids?: number[]
}

// Build the POST /api/evals body for a selection. A fully selected
// dimension is the server default and is omitted — both fully selected
// yields the empty body, the legacy one-click full sweep. A partially
// selected dimension rides as an explicit id list (suite_ids narrows the
// rotation, model_ids overrides the eval_enabled candidate list).
export function buildTriggerBody(
  selectedSuiteIds: readonly number[],
  selectedModelIds: readonly number[],
  candidates: EvalCandidates,
): TriggerEvalBody {
  const body: TriggerEvalBody = {}
  // Order-insensitive set equality: fully selected means the server default.
  if (
    selectedSuiteIds.length !== candidates.suiteIds.length ||
    selectedSuiteIds.some(id => !candidates.suiteIds.includes(id))
  ) {
    body.suite_ids = [...selectedSuiteIds]
  }
  if (
    selectedModelIds.length !== candidates.modelIds.length ||
    selectedModelIds.some(id => !candidates.modelIds.includes(id))
  ) {
    body.model_ids = [...selectedModelIds]
  }
  return body
}

// Translate trigger failures into operator-facing wording: the 409
// cross-campaign mutex (GH #153) speaks server English.
export function friendlyTriggerError(err: unknown): string {
  if (err instanceof ApiError && err.status === 409) {
    return '已有评估批次进行中,请等待其完成或取消后再试'
  }
  return err instanceof Error ? err.message : String(err)
}
