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
