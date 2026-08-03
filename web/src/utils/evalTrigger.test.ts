import { describe, expect, it } from 'vitest'
import { buildTriggerBody, evalCandidates } from './evalTrigger'
import type { Model, Suite } from '@/api/types'

type SuiteStub = Pick<Suite, 'id' | 'enabled'>
type ModelStub = Pick<Model, 'id' | 'capability' | 'status' | 'eval_enabled'>

function suite(id: number, enabled = true): SuiteStub {
  return { id, enabled }
}

function model(id: number, over: Partial<ModelStub> = {}): ModelStub {
  return { id, capability: 'chat', status: 'active', eval_enabled: true, ...over }
}

const suites = [suite(1), suite(2), suite(3, false)]
const models = [
  model(11),
  model(12),
  model(13, { capability: 'non_chat' }),
  model(14, { status: 'retired' }),
  model(15, { eval_enabled: false }),
]

describe('evalCandidates', () => {
  it('offers only enabled suites and sweep-eligible models', () => {
    expect(evalCandidates(suites, models)).toEqual({ suiteIds: [1, 2], modelIds: [11, 12] })
  })

  it('excludes non-chat, retired and opted-out models', () => {
    const { modelIds } = evalCandidates([], models)
    expect(modelIds).not.toContain(13)
    expect(modelIds).not.toContain(14)
    expect(modelIds).not.toContain(15)
  })
})

describe('buildTriggerBody', () => {
  const candidates = { suiteIds: [1, 2], modelIds: [11, 12] }

  it('yields the empty body when everything is selected', () => {
    expect(buildTriggerBody([1, 2], [11, 12], candidates)).toEqual({})
  })

  it('treats selection order as irrelevant', () => {
    expect(buildTriggerBody([2, 1], [12, 11], candidates)).toEqual({})
  })

  it('sends only suite_ids when models stay fully selected', () => {
    expect(buildTriggerBody([1], [11, 12], candidates)).toEqual({ suite_ids: [1] })
  })

  it('sends only model_ids when suites stay fully selected', () => {
    expect(buildTriggerBody([1, 2], [11], candidates)).toEqual({ model_ids: [11] })
  })

  it('sends both lists when both dimensions are narrowed', () => {
    expect(buildTriggerBody([2], [12], candidates)).toEqual({ suite_ids: [2], model_ids: [12] })
  })
})
