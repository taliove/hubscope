// PROTOTYPE — throwaway mock data for the spec-0020 eval-center UI variants.
// A static mid-run snapshot: probe gate passed for 7/8 models, jury picked
// under the balanced policy, exam/judge queues in flight, partial scores.

export interface ProtoJudge {
  id: string
  slot: number
  iq: number // 0..1 normalized reason scores
  spd: number
  chp: number
  calls: number
  fails: number
  cost: number
}

export interface ProtoSubject {
  id: string
  probeOk: boolean
  probeSucc: string // e.g. "3/3"
  probeTps: number | null
  examDone: number
  examTotal: number
  judgeDone: number
  judgeTotal: number
  score: number | null // running median-based score, null until first case settles
  tps: number | null
  examCost: number | null // null = price not registered
  judgeCost: number | null
}

export interface ProtoSample {
  no: number
  scores: (number | null)[] // per jury slot; null = judge call failed
  median: number | null
  spread: number
}

export interface ProtoCase {
  id: number
  title: string
  capability: string
  samples: ProtoSample[]
  caseScore: number | null
}

export const protoPhase = 'RUN' as const

export const protoJuryPolicy = 'balanced'

export const protoJudges: ProtoJudge[] = [
  { id: 'qwen3-235b', slot: 0, iq: 0.9, spd: 0.38, chp: 0.0, calls: 78, fails: 1, cost: 0.4212 },
  { id: 'deepseek-v3', slot: 1, iq: 0.85, spd: 0.5, chp: 0.72, calls: 78, fails: 0, cost: 0.2964 },
  { id: 'qwen3-30b-a3b', slot: 2, iq: 0.75, spd: 0.83, chp: 0.87, calls: 77, fails: 2, cost: 0.1956 },
]

export const protoPipeline = {
  examDone: 89,
  examTotal: 96,
  examPending: 7,
  examInflight: 2,
  judgeDone: 231,
  judgeTotal: 267,
  judgePending: 34,
  judgeInflight: 2,
  circuit: 0,
  circuitLimit: 5,
}

// 8 chat models on the hub; yi-lightning failed the probe gate and was
// skipped without burning a single case (no dead rows).
export const protoSubjects: ProtoSubject[] = [
  { id: 'llama-3.1-8b', probeOk: true, probeSucc: '2/3', probeTps: 118, examDone: 16, examTotal: 16, judgeDone: 45, judgeTotal: 48, score: 0.57, tps: 109, examCost: 0, judgeCost: 0.1521 },
  { id: 'gpt-4o-mini', probeOk: true, probeSucc: '3/3', probeTps: 88, examDone: 16, examTotal: 16, judgeDone: 44, judgeTotal: 48, score: 0.71, tps: 84, examCost: 0.0307, judgeCost: 0.1521 },
  { id: 'glm-4-flash', probeOk: true, probeSucc: '3/3', probeTps: 105, examDone: 16, examTotal: 16, judgeDone: 46, judgeTotal: 48, score: 0.63, tps: 101, examCost: 0.0064, judgeCost: 0.1521 },
  { id: 'mistral-large', probeOk: true, probeSucc: '3/3', probeTps: 52, examDone: 15, examTotal: 16, judgeDone: 41, judgeTotal: 45, score: 0.78, tps: 49, examCost: 0.6144, judgeCost: 0.1425 },
  { id: 'qwen3-32b', probeOk: true, probeSucc: '3/3', probeTps: 95, examDone: 14, examTotal: 16, judgeDone: 39, judgeTotal: 42, score: 0.74, tps: 91, examCost: null, judgeCost: null },
  { id: 'claude-haiku', probeOk: true, probeSucc: '3/3', probeTps: 76, examDone: 12, examTotal: 16, judgeDone: 16, judgeTotal: 36, score: null, tps: 72, examCost: 0.2016, judgeCost: 0.0507 },
  { id: 'yi-lightning', probeOk: false, probeSucc: '0/3', probeTps: null, examDone: 0, examTotal: 16, judgeDone: 0, judgeTotal: 0, score: null, tps: null, examCost: 0, judgeCost: 0 },
]

// Per-case jury detail for the selected subject (llama-3.1-8b), samples=2.
export const protoCases: ProtoCase[] = [
  {
    id: 101, title: '抽取发票金额并输出 JSON', capability: '结构化抽取', caseScore: 0.64,
    samples: [
      { no: 1, scores: [0.65, 0.65, 0.62], median: 0.65, spread: 0.03 },
      { no: 2, scores: [0.54, 0.63, 0.65], median: 0.63, spread: 0.11 },
    ],
  },
  {
    id: 102, title: '三段论推理:结论是否成立', capability: '推理', caseScore: 0.51,
    samples: [
      { no: 1, scores: [0.47, 0.55, 0.57], median: 0.55, spread: 0.1 },
      { no: 2, scores: [null, 0.47, 0.47], median: 0.47, spread: 0 },
    ],
  },
  {
    id: 103, title: '阅读函数,说明边界条件缺陷', capability: '代码理解', caseScore: 0.61,
    samples: [
      { no: 1, scores: [0.61, 0.61, 0.58], median: 0.61, spread: 0.03 },
      { no: 2, scores: [0.64, 0.62, 0.5], median: 0.62, spread: 0.14 },
    ],
  },
  {
    id: 104, title: '长文本摘要(800 字 → 100 字)', capability: '语言理解与生成', caseScore: 0.62,
    samples: [
      { no: 1, scores: [0.6, 0.55, 0.67], median: 0.6, spread: 0.12 },
      { no: 2, scores: [null, 0.66, 0.6], median: 0.63, spread: 0.06 },
    ],
  },
  {
    id: 105, title: '指令遵循:仅输出三个关键词', capability: '指令遵循', caseScore: 0.46,
    samples: [
      { no: 1, scores: [0.5, 0.43, 0.5], median: 0.5, spread: 0.07 },
      { no: 2, scores: [0.5, 0.41, 0.41], median: 0.41, spread: 0.09 },
    ],
  },
  {
    id: 106, title: '多跳知识问答', capability: '知识问答', caseScore: null,
    samples: [
      { no: 1, scores: [0.58, 0.52, null], median: 0.55, spread: 0.06 },
      { no: 2, scores: [null, null, null], median: null, spread: 0 },
    ],
  },
]

export const protoTotals = {
  examCost: 0.8531,
  judgeCost: 0.9459,
  avgTps: 86,
  estTotal: 2.4,
  priceUnknownModels: ['qwen3-32b'],
}

export const protoEvents = [
  '[10:02:11] 刺探完成:8 模型,7 可达;yi-lightning 0/3 不可达 → 跳过,不烧题',
  '[10:02:11] 裁判团(均衡)= qwen3-235b / deepseek-v3 / qwen3-30b-a3b;被评模型已排除自我裁判',
  '[10:02:12] 开跑:8 cases × 2 samples × 7 模型 = 112 答题,裁判 3 票/答案',
  '[10:14:37] judge qwen3-235b 调用失败(case 102 sample 2,槽位记 null,不重试)',
  '[10:21:05] judge qwen3-30b-a3b 调用失败(case 104 sample 2,槽位记 null)',
  '[10:23:44] claude-haiku 考试中(12/16),裁判队列积压 34',
]

export function fmtCost(v: number | null): string {
  return v === null ? '价格未登记' : `$${v.toFixed(4)}`
}
