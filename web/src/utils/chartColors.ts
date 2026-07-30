// ECharts color mirrors — charts read colors from JS, not CSS, so the
// palette lives here as the single source consumed by every chart component
// (TimeSeriesChart, TrendChart, ModelTrendDialog). Light/dark pairs stay in
// sync with styles/semantics.css (ui-guidelines §3); chart components call
// useChartColors() and re-render on theme change.
import { computed, type ComputedRef } from 'vue'
import { useTheme } from './theme'

export interface ChartColors {
  brand: string
  success: string
  warning: string
  danger: string
  failing: string
  textRegular: string
  textSecondary: string
  placeholder: string
  border: string
  // Surface mirror for chart area fills (2026-07-30, ProbeLatencyChart
  // areaStyle) — the JS-side counterpart of LatencySparkline's solid
  // `fill: var(--hs-bg-hover)` (ui-guidelines §5 LatencySparkline 条:
  // 功能性面积强调,非装饰). Solid color, no opacity stacking, no gradient.
  bgHover: string
}

// Light theme: mirror of semantics.css :root values.
export const CHART_COLORS_LIGHT: ChartColors = {
  brand: '#0c8078', // --hs-brand (teal-600)
  success: '#059669', // --hs-success
  warning: '#a16207', // --hs-warning
  danger: '#dc2626', // --hs-danger
  failing: '#c2410c', // --hs-status-failing (orange-700)
  textRegular: '#324249', // --hs-text-regular (gray-700)
  textSecondary: '#617379', // --hs-text-secondary (gray-500)
  placeholder: '#91a3a8', // --hs-text-placeholder (gray-400)
  border: '#e0e8ea', // --hs-border (gray-200)
  bgHover: '#eff4f5', // --hs-bg-hover (gray-100) — LatencySparkline fill mirror
} as const

// Dark theme: mirror of semantics.css html.dark values.
export const CHART_COLORS_DARK: ChartColors = {
  brand: '#0faea2', // --hs-brand (teal-500)
  success: '#059669', // --hs-success (same value, double-encoded)
  warning: '#d97706', // --hs-warning
  danger: '#dc2626', // --hs-danger
  failing: '#fb923c', // --hs-status-failing (orange-400)
  textRegular: '#c3c7cd', // --hs-text-regular
  textSecondary: '#8a8f98', // --hs-text-secondary
  placeholder: '#5c616a', // --hs-text-placeholder
  border: '#2a2d33', // --hs-border
  bgHover: '#1f2227', // --hs-bg-hover — LatencySparkline fill mirror
} as const

// Series palette order shared by all charts: brand first, then the status
// ramp. Lines are assigned in declaration order.
export function seriesPalette(colors: ChartColors): string[] {
  return [colors.brand, colors.success, colors.warning, colors.danger, colors.failing]
}

// Reactive theme-aware palette. Components watch the returned ref (or the
// theme itself) and setOption again when it flips.
export function useChartColors(): ComputedRef<ChartColors> {
  const { dark } = useTheme()
  return computed(() => (dark.value ? CHART_COLORS_DARK : CHART_COLORS_LIGHT))
}
