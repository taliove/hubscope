<!-- PROTOTYPE — Variant A「管线指挥舱」: pipeline-first ops console.
     Primary affordance: watching a running batch move through the three
     async stages (probe gate → exam pool → judge pool → median aggregate). -->
<script setup lang="ts">
import {
  protoEvents,
  protoJudges,
  protoPipeline as p,
  protoSubjects,
} from './evalProtoMock'

function pct(done: number, total: number): number {
  return total === 0 ? 0 : Math.round((done / total) * 100)
}
</script>

<template>
  <div class="va">
    <!-- Stage flow: four nodes, connectors carry queue depth. -->
    <div class="flow">
      <div class="node done">
        <div class="node-title">刺探门控</div>
        <div class="node-big">7/8</div>
        <div class="node-sub">yi-lightning 不可达 → 跳过,未烧一题</div>
      </div>
      <div class="link" />
      <div class="node active">
        <div class="node-title">考试池</div>
        <div class="node-big">{{ p.examDone }}<span class="dim">/{{ p.examTotal }}</span></div>
        <div class="bar"><div class="bar-fill" :style="{ width: pct(p.examDone, p.examTotal) + '%' }" /></div>
        <div class="node-sub">队列 {{ p.examPending }} · 在飞 {{ p.examInflight }} · 熔断 {{ p.circuit }}/{{ p.circuitLimit }}</div>
      </div>
      <div class="link" />
      <div class="node active">
        <div class="node-title">裁判池 ×3 票</div>
        <div class="node-big">{{ p.judgeDone }}<span class="dim">/{{ p.judgeTotal }}</span></div>
        <div class="bar"><div class="bar-fill judge" :style="{ width: pct(p.judgeDone, p.judgeTotal) + '%' }" /></div>
        <div class="node-sub">积压 {{ p.judgePending }} · 在飞 {{ p.judgeInflight }} · 失败槽位记 null</div>
      </div>
      <div class="link" />
      <div class="node">
        <div class="node-title">中位数聚合</div>
        <div class="node-big">5<span class="dim">/7</span></div>
        <div class="node-sub">模型已出分 · 3→中位 / 2→均值 / 1→自身</div>
      </div>
    </div>

    <!-- Jury strip -->
    <div class="jury-strip">
      <span class="strip-label">裁判团(均衡策略):</span>
      <span v-for="j in protoJudges" :key="j.id" class="judge-chip">
        {{ j.id }}
        <em>{{ j.calls }} 票 / 失败 {{ j.fails }} / ${{ j.cost.toFixed(4) }}</em>
      </span>
      <span class="strip-note">被评模型已排除自我裁判</span>
    </div>

    <!-- Model monitor -->
    <div class="panel">
      <div class="panel-title">模型监控</div>
      <table>
        <thead>
          <tr><th>模型</th><th>刺探</th><th>考试</th><th>裁判</th><th>中位数分</th><th>TPS</th><th>成本</th></tr>
        </thead>
        <tbody>
          <tr v-for="m in protoSubjects" :key="m.id" :class="{ skipped: !m.probeOk }">
            <td class="mono">{{ m.id }}</td>
            <td>
              <span v-if="m.probeOk" class="ok">✓ {{ m.probeSucc }} · {{ m.probeTps }} tps</span>
              <span v-else class="bad">✗ {{ m.probeSucc }} 不可达,已跳过</span>
            </td>
            <td>
              <div class="cellbar"><div class="cellbar-fill" :style="{ width: pct(m.examDone, m.examTotal) + '%' }" /></div>
              <span class="cellnum">{{ m.examDone }}/{{ m.examTotal }}</span>
            </td>
            <td>
              <div class="cellbar"><div class="cellbar-fill judge" :style="{ width: pct(m.judgeDone, m.judgeTotal) + '%' }" /></div>
              <span class="cellnum">{{ m.judgeDone }}/{{ m.judgeTotal }}</span>
            </td>
            <td class="score">{{ m.score === null ? '—' : m.score.toFixed(2) }}</td>
            <td>{{ m.tps ?? '—' }}</td>
            <td>{{ m.examCost === null ? '未登记' : '$' + (m.examCost + (m.judgeCost ?? 0)).toFixed(4) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Live log -->
    <div class="panel">
      <div class="panel-title">任务日志</div>
      <div class="log">
        <div v-for="(e, i) in protoEvents" :key="i" class="log-line">{{ e }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.va { display: flex; flex-direction: column; gap: var(--hs-space-4); }

.flow { display: flex; align-items: stretch; gap: var(--hs-space-2); }
.node {
  flex: 1; background: #fff; border: 1px solid var(--hs-gray-100); border-radius: 12px;
  padding: var(--hs-space-4); display: flex; flex-direction: column; gap: 6px;
}
.node.active { border-color: var(--hs-blue-200); }
.node.done { border-color: var(--hs-success-base); }
.link { align-self: center; width: 28px; height: 2px; background: var(--hs-gray-200); position: relative; }
.link::after {
  content: ''; position: absolute; right: 0; top: -3px;
  border-left: 7px solid var(--hs-gray-200);
  border-top: 4px solid transparent; border-bottom: 4px solid transparent;
}
.node-title { font-size: 13px; color: var(--hs-gray-500); }
.node-big { font-size: 30px; font-weight: 700; color: var(--hs-gray-900); }
.dim { color: var(--hs-gray-400); font-size: 18px; font-weight: 500; }
.node-sub { font-size: 12px; color: var(--hs-gray-500); }
.bar { height: 6px; background: var(--hs-gray-100); border-radius: 3px; overflow: hidden; }
.bar-fill { height: 100%; background: var(--hs-blue-600); }
.bar-fill.judge { background: var(--hs-warning-base); }

.jury-strip {
  display: flex; align-items: center; flex-wrap: wrap; gap: var(--hs-space-2);
  background: #fff; border: 1px solid var(--hs-gray-100); border-radius: 12px; padding: var(--hs-space-3) var(--hs-space-4);
  font-size: 13px;
}
.strip-label { color: var(--hs-gray-700); font-weight: 600; }
.judge-chip {
  background: var(--hs-blue-50); color: var(--hs-gray-900); border-radius: 8px; padding: 4px 10px;
}
.judge-chip em { font-style: normal; color: var(--hs-gray-500); margin-left: 6px; font-size: 12px; }
.strip-note { margin-left: auto; color: var(--hs-gray-500); font-size: 12px; }

.panel { background: #fff; border: 1px solid var(--hs-gray-100); border-radius: 12px; padding: var(--hs-space-4); }
.panel-title { font-size: 15px; font-weight: 600; color: var(--hs-gray-900); margin-bottom: var(--hs-space-3); }

table { width: 100%; border-collapse: collapse; font-size: 13px; }
th { text-align: left; color: var(--hs-gray-500); font-weight: 500; padding: 6px 8px; border-bottom: 1px solid var(--hs-gray-100); }
td { padding: 8px; border-bottom: 1px solid var(--hs-gray-50); color: var(--hs-gray-800); }
tr.skipped td { color: var(--hs-gray-400); }
.mono { font-family: ui-monospace, monospace; }
.ok { color: var(--hs-success-text-base); }
.bad { color: var(--hs-danger-text-base); }
.score { font-weight: 700; color: var(--hs-gray-900); }
.cellbar { width: 90px; height: 5px; background: var(--hs-gray-100); border-radius: 3px; overflow: hidden; display: inline-block; vertical-align: middle; }
.cellbar-fill { height: 100%; background: var(--hs-blue-600); }
.cellbar-fill.judge { background: var(--hs-warning-base); }
.cellnum { margin-left: 6px; font-size: 12px; color: var(--hs-gray-500); }

.log { font-family: ui-monospace, monospace; font-size: 12px; color: var(--hs-gray-700); display: flex; flex-direction: column; gap: 4px; }
.log-line { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>
