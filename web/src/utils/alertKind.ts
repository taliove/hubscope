// Alert-event kind label + tag type mapping (GH #68, spec 0017 ticket 5;
// ui-guidelines §5 centralization principle, role.ts / protocol.ts
// precedent). The alert-history table never writes word literals in the
// component — this module is the single source of truth for the kind
// vocabulary.
//
// Vocabulary discipline (ui-guidelines §7): this is the alert-EVENT word
// table, a third table beside the endpoint status words (正常/降级/宕机/
// 告警) and the batch/run words (等待中/运行中/已完成/失败). The four spec
// 0017 words are frozen by the spec — 「厂商组告警」 intentionally reuses
// the character 告警 (the collision was adjudicated at spec level); do not
// "fix" it here and do not coin new words.
import type { TagProps } from 'element-plus'
import type { AlertKind } from '@/api/settings'

const ALERT_KIND_LABELS: Record<AlertKind, string> = {
  down: '故障',
  recovered: '恢复',
  score_drop: '分数大跌',
  score_drop_skipped: '对比跳过',
  test: '测试',
  // spec 0017 frozen words.
  group_down: '厂商组告警',
  group_recovered: '厂商组恢复',
  batch: '聚合发送',
  quiet_summary: '静默摘要',
  // spec 0018 T4 retirement words (GH #98).
  retire_pending: '待退役',
  retired: '已退役',
}

// el-tag `type` accepts 'primary' | 'success' | 'warning' | 'danger' |
// 'info'. Fault/recovery kinds keep the legacy status-color mapping; the
// spec 0017 delivery-form kinds (batch / quiet_summary) are neutral info —
// they record HOW alerts went out, not a health signal. The manual channel
// check (test) is likewise not a health signal. The spec 0018 T4 retirement
// kinds (GH #98): retire_pending is warning (needs attention, like failures),
// retired is info (delivery record, not a health signal).
const ALERT_KIND_TAG_TYPES: Record<AlertKind, TagProps['type']> = {
  down: 'danger',
  recovered: 'success',
  score_drop: 'warning',
  score_drop_skipped: 'warning',
  test: 'info',
  group_down: 'danger',
  group_recovered: 'success',
  batch: 'info',
  quiet_summary: 'info',
  retire_pending: 'warning',
  retired: 'info',
}

// ALERT_KINDS lists every kind in the union — the keys of the exhaustive
// Record above, so the type system keeps this list in sync automatically
// (PROTOCOLS precedent in protocol.ts, GH #38 single-source discipline).
// Filter dropdowns (the /alerts timeline type filter, GH #117) render from
// this single source instead of hand-listing values. Labels and tag types
// stay untouched — this is an additive export, not a vocabulary change.
export const ALERT_KINDS = Object.keys(ALERT_KIND_LABELS) as AlertKind[]

// alertKindLabel returns the Chinese display word for an alert event kind.
// Unknown/null kinds fall back to a placeholder (role.ts precedent) so the
// UI never renders a bare technical kind string to admin readers.
export function alertKindLabel(kind: string | null | undefined): string {
  if (kind && kind in ALERT_KIND_LABELS) {
    return ALERT_KIND_LABELS[kind as AlertKind]
  }
  return '未知类型'
}

// alertKindTagType returns the el-tag `type` for an alert event kind.
// Unknown/null falls back to 'info' (neutral) — ambiguous events never
// claim a status color.
export function alertKindTagType(kind: string | null | undefined): TagProps['type'] {
  if (kind && kind in ALERT_KIND_TAG_TYPES) {
    return ALERT_KIND_TAG_TYPES[kind as AlertKind]
  }
  return 'info'
}
