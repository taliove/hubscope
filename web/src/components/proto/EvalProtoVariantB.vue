<!-- PROTOTYPE — Variant B「裁判席」: jury-first scrutiny layout.
     Primary affordance: auditing judging quality — per-judge columns,
     median rule, and spread as a disagreement signal. -->
<script setup lang="ts">
import { protoCases, protoJudges, protoJuryPolicy, protoSubjects } from './evalProtoMock'

const policyNames: Record<string, string> = {
  balanced: '均衡', speed: '速度优先', iq: '智商优先', cost: '成本优先',
}

function scoreClass(v: number | null): string {
  if (v === null) return 'chip fail'
  if (v >= 0.8) return 'chip high'
  if (v >= 0.5) return 'chip mid'
  return 'chip low'
}
</script>

<template>
  <div class="vb">
    <!-- Left rail: the jury itself. -->
    <aside class="rail">
      <div class="rail-title">裁判团 <span class="policy">{{ policyNames[protoJuryPolicy] }}策略</span></div>
      <div v-for="j in protoJudges" :key="j.id" class="judge-card">
        <div class="judge-head">
          <span class="judge-name">{{ j.id }}</span>
          <span class="judge-cost">${{ j.cost.toFixed(4) }}</span>
        </div>
        <div class="reason">
          <span>智商</span>
          <div class="rbar"><div class="rbar-fill iq" :style="{ width: j.iq * 100 + '%' }" /></div>
        </div>
        <div class="reason">
          <span>速度</span>
          <div class="rbar"><div class="rbar-fill spd" :style="{ width: j.spd * 100 + '%' }" /></div>
        </div>
        <div class="reason">
          <span>便宜</span>
          <div class="rbar"><div class="rbar-fill chp" :style="{ width: j.chp * 100 + '%' }" /></div>
        </div>
        <div class="judge-foot">{{ j.calls }} 票 · 失败 {{ j.fails }}(记 null,不重试)</div>
      </div>
      <div class="rail-note">
        被评模型已排除自我裁判(备选 ≥3)。<br />
        中位数规则:3 票取中位 / 2 票取均值 / 1 票取自身 / 0 票不计分。
      </div>
    </aside>

    <!-- Main: case table with per-judge columns. -->
    <main class="main">
      <div class="subject-row">
        <span class="s-label">被评模型:</span>
        <span
          v-for="(m, i) in protoSubjects.filter((s) => s.probeOk)"
          :key="m.id"
          class="s-chip"
          :class="{ on: i === 0 }"
        >{{ m.id }}</span>
      </div>

      <div class="panel">
        <table>
          <thead>
            <tr>
              <th class="c-case">Case</th>
              <th v-for="j in protoJudges" :key="j.id" class="c-j">{{ j.id }}</th>
              <th class="c-med">中位数</th>
              <th class="c-spread">分歧</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="c in protoCases" :key="c.id">
              <tr class="case-row">
                <td>
                  <div class="case-title">{{ c.title }}</div>
                  <div class="case-cap">{{ c.capability }}</div>
                </td>
                <td :colspan="3" class="case-score-cell">case 分(样本中位数均值)</td>
                <td class="case-score-cell med">
                  <strong>{{ c.caseScore === null ? '判分中' : c.caseScore.toFixed(2) }}</strong>
                </td>
                <td />
              </tr>
              <tr v-for="s in c.samples" :key="s.no" class="sample-row">
                <td class="sample-no">样本 {{ s.no }}</td>
                <td v-for="(sc, k) in s.scores" :key="k">
                  <span :class="scoreClass(sc)">{{ sc === null ? 'FAIL' : sc.toFixed(2) }}</span>
                </td>
                <td class="med-cell">{{ s.median === null ? 'null' : s.median.toFixed(2) }}</td>
                <td>
                  <div class="spread">
                    <div
                      class="spread-fill"
                      :class="{ hot: s.spread > 0.12 }"
                      :style="{ width: Math.min(s.spread * 400, 100) + '%' }"
                    />
                  </div>
                  <span class="spread-num" :class="{ hot: s.spread > 0.12 }">{{ s.spread.toFixed(2) }}</span>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>

      <div class="legend">
        <span class="dot hot-dot" /> 分歧 &gt; 0.12:裁判意见不一致,题目可能模糊或模型作答有争议,值得人工抽查。
        <span class="dot fail-dot" /> FAIL:该裁判调用失败,槽位记 null,中位数按剩余票降级。
      </div>
    </main>
  </div>
</template>

<style scoped>
.vb { display: flex; gap: var(--hs-space-4); align-items: flex-start; }

.rail {
  width: 280px; flex-shrink: 0; display: flex; flex-direction: column; gap: var(--hs-space-3);
}
.rail-title { font-size: 16px; font-weight: 700; color: var(--hs-gray-900); }
.policy {
  font-size: 12px; font-weight: 500; color: var(--hs-blue-600);
  background: var(--hs-blue-50); border-radius: 6px; padding: 2px 8px; margin-left: 6px;
}
.judge-card {
  background: #fff; border: 1px solid var(--hs-gray-100); border-radius: 12px;
  padding: var(--hs-space-3); display: flex; flex-direction: column; gap: 6px;
}
.judge-head { display: flex; justify-content: space-between; align-items: baseline; }
.judge-name { font-weight: 600; color: var(--hs-gray-900); font-size: 14px; }
.judge-cost { color: var(--hs-gray-500); font-size: 12px; }
.reason { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--hs-gray-500); }
.reason span { width: 28px; }
.rbar { flex: 1; height: 5px; background: var(--hs-gray-100); border-radius: 3px; overflow: hidden; }
.rbar-fill { height: 100%; }
.rbar-fill.iq { background: var(--hs-blue-600); }
.rbar-fill.spd { background: var(--hs-success-base); }
.rbar-fill.chp { background: var(--hs-warning-base); }
.judge-foot { font-size: 12px; color: var(--hs-gray-500); }
.rail-note {
  font-size: 12px; color: var(--hs-gray-500); line-height: 1.7;
  background: var(--hs-gray-50); border-radius: 8px; padding: var(--hs-space-3);
}

.main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: var(--hs-space-3); }
.subject-row { display: flex; align-items: center; flex-wrap: wrap; gap: var(--hs-space-2); font-size: 13px; }
.s-label { color: var(--hs-gray-500); }
.s-chip {
  padding: 3px 10px; border-radius: 999px; border: 1px solid var(--hs-gray-200);
  color: var(--hs-gray-700); cursor: pointer;
}
.s-chip.on { background: var(--hs-gray-900); color: #fff; border-color: var(--hs-gray-900); }

.panel { background: #fff; border: 1px solid var(--hs-gray-100); border-radius: 12px; padding: var(--hs-space-3) var(--hs-space-4); }
table { width: 100%; border-collapse: collapse; font-size: 13px; }
th { text-align: left; color: var(--hs-gray-500); font-weight: 500; padding: 8px; border-bottom: 1px solid var(--hs-gray-100); font-size: 12px; }
td { padding: 6px 8px; }
.case-row td { background: var(--hs-gray-50); border-top: 1px solid var(--hs-gray-100); }
.case-title { font-weight: 600; color: var(--hs-gray-900); }
.case-cap { font-size: 12px; color: var(--hs-gray-500); }
.case-score-cell { color: var(--hs-gray-600); font-size: 12px; }
.case-score-cell.med strong { font-size: 15px; color: var(--hs-gray-900); }
.sample-no { color: var(--hs-gray-400); font-size: 12px; padding-left: 20px; }

.chip { display: inline-block; min-width: 44px; text-align: center; border-radius: 6px; padding: 2px 6px; font-weight: 600; }
.chip.high { background: #e6f7ec; color: var(--hs-success-text-base); }
.chip.mid { background: var(--hs-blue-50); color: var(--hs-gray-900); }
.chip.low { background: #fff3e6; color: var(--hs-warning-text-base); }
.chip.fail { background: #ffecea; color: var(--hs-danger-text-base); }
.med-cell { font-weight: 700; color: var(--hs-gray-900); }

.spread { width: 80px; height: 5px; background: var(--hs-gray-100); border-radius: 3px; overflow: hidden; display: inline-block; vertical-align: middle; }
.spread-fill { height: 100%; background: var(--hs-gray-300); }
.spread-fill.hot { background: var(--hs-danger-base); }
.spread-num { margin-left: 6px; font-size: 12px; color: var(--hs-gray-500); }
.spread-num.hot { color: var(--hs-danger-text-base); font-weight: 600; }

.legend { font-size: 12px; color: var(--hs-gray-500); line-height: 2; }
.dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin: 0 4px 0 12px; }
.hot-dot { background: var(--hs-danger-base); }
.fail-dot { background: #ffecea; border: 1px solid var(--hs-danger-base); }
</style>
