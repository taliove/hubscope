// Small formatting helpers shared across admin components.

// Shared local-date parts for the time formatters below; null/undefined →
// null, unparseable → the raw value echoed back as-is (callers render it).
function localParts(value: string | null | undefined): { y: string; m: string; d: string; hh: string; mm: string; ss: string } | null {
  if (!value) return null
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return null
  const pad = (n: number) => String(n).padStart(2, '0')
  return {
    y: String(date.getFullYear()),
    m: pad(date.getMonth() + 1),
    d: pad(date.getDate()),
    hh: pad(date.getHours()),
    mm: pad(date.getMinutes()),
    ss: pad(date.getSeconds()),
  }
}

// Render an RFC3339 timestamp as a compact local datetime string.
export function formatTime(value: string | null): string {
  if (!value) return '-'
  const p = localParts(value)
  if (!p) return value
  return `${p.y}-${p.m}-${p.d} ${p.hh}:${p.mm}:${p.ss}`
}

// Render an RFC3339 timestamp as "YYYY-MM-DD HH:mm" — minute precision for
// batch/生成 timestamps (GH #57 board batch meta, share-card footers) where
// seconds are noise. Components must not slice formatTime output themselves.
export function formatTimeMinute(value: string | null | undefined): string {
  if (!value) return '-'
  const p = localParts(value)
  if (!p) return value as string
  return `${p.y}-${p.m}-${p.d} ${p.hh}:${p.mm}`
}

// Render an RFC3339 timestamp as a bare local clock reading "HH:mm:ss" —
// for dense, same-session feeds (issue #17 live feed) where the date is
// noise. Falls back to the raw value on an unparseable input.
export function formatClockTime(value: string | null): string {
  if (!value) return '-'
  const p = localParts(value)
  if (!p) return value
  return `${p.hh}:${p.mm}:${p.ss}`
}

// Render an RFC3339 timestamp as a bare "HH:mm" clock reading — same-session
// freshness labels (HealthBanner 更新于) where both date and seconds are
// noise. Components must not slice formatTime/formatClockTime output.
export function formatClockMinute(value: string | null | undefined): string {
  if (!value) return '-'
  const p = localParts(value)
  if (!p) return value as string
  return `${p.hh}:${p.mm}`
}

// Render a nullable numeric metric, falling back to a dash.
export function formatMetric(value: number | null): string {
  if (value === null || value === undefined) return '-'
  return String(value)
}

// Render a 0~1 ratio as a percentage with one decimal, dash when null.
export function formatPercent(value: number | null): string {
  if (value === null || value === undefined) return '-'
  return `${(value * 100).toFixed(1)}%`
}

// Render a 0~1 ratio as percentage digits without the % sign, for layouts
// that typeset the unit separately (e.g. the StatusCard big number).
export function formatPercentDigits(value: number | null): string {
  if (value === null || value === undefined) return '-'
  return (value * 100).toFixed(1)
}

// Render a 0-100 score with one decimal, dash when null. Every eval/report
// score renders through this; components must not self-format (toFixed).
export function formatScore(value: number | null): string {
  if (value === null || value === undefined) return '-'
  return value.toFixed(1)
}

// Render a score delta against a previous batch with an explicit sign
// (+3.5 / -2.1), dash when null. Zero is a real value but callers render
// it as a flat placeholder (ui-guidelines §3: no arrow on ties).
export function formatScoreDelta(value: number | null): string {
  if (value === null || value === undefined) return '-'
  const sign = value > 0 ? '+' : ''
  return `${sign}${value.toFixed(1)}`
}

// Render a millisecond latency: raw ms below 1s, seconds above; dash when null.
export function formatMs(value: number | null): string {
  if (value === null || value === undefined) return '-'
  if (value < 1000) return `${Math.round(value)}ms`
  return `${(value / 1000).toFixed(2)}s`
}

// Render an accumulated duration in milliseconds (GH #42, ui-guidelines §5
// 成本指标条): the batch-level caliber — sub-minute as one-decimal seconds,
// sub-hour as "M 分 S 秒", beyond as "H 小时 M 分". Dash when null.
export function formatDuration(ms: number | null): string {
  if (ms === null || ms === undefined) return '-'
  const seconds = ms / 1000
  if (seconds < 60) return `${seconds.toFixed(1)}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分 ${Math.round(seconds % 60)} 秒`
  return `${Math.floor(seconds / 3600)} 小时 ${Math.round((seconds % 3600) / 60)} 分`
}

// Render a token count (GH #42, same registration): raw below 1000, then
// one-decimal k/M abbreviations. Dash when null.
export function formatTokens(value: number | null): string {
  if (value === null || value === undefined) return '-'
  if (value < 1000) return String(value)
  if (value < 1_000_000) return `${(value / 1000).toFixed(1)}k`
  return `${(value / 1_000_000).toFixed(1)}M`
}

// Render an hour-aligned bucket timestamp as a compact local "MM-dd HH:mm"
// label for chart axes.
export function formatBucketTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:00`
}
