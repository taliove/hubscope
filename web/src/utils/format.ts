// Small formatting helpers shared across admin components.

// Render an RFC3339 timestamp as a compact local datetime string.
export function formatTime(value: string | null): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const pad = (n: number) => String(n).padStart(2, '0')
  const y = date.getFullYear()
  const m = pad(date.getMonth() + 1)
  const d = pad(date.getDate())
  const hh = pad(date.getHours())
  const mm = pad(date.getMinutes())
  const ss = pad(date.getSeconds())
  return `${y}-${m}-${d} ${hh}:${mm}:${ss}`
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

// Render an hour-aligned bucket timestamp as a compact local "MM-dd HH:mm"
// label for chart axes.
export function formatBucketTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:00`
}
