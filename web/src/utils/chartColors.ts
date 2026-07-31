// ECharts color mirrors — charts read colors from JS, not CSS, so the
// palette lives here as the single source consumed by every chart component
// (TimeSeriesChart, TrendChart, ModelTrendDialog). Light/dark pairs stay in
// sync with styles/semantics.css; chart components call useChartColors() and
// re-render on theme change.
// v2.0 (GH #110, spec 0018): LIGHT rebuilt on the new palette (brand blue
// #007AFF, three-state functional colors, Apple neutrals); failing is
// merged into incident at the display layer (decision 3) = danger red, the
// field stays (seriesPalette's five-slot structure and mixed-period
// consumers); DARK keys are reserved with values temporarily mirroring
// LIGHT (dark deferred, decision 10 — a later dark spec fills the values).
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
  // Surface mirror for chart area fills — the JS-side counterpart of the
  // solid `fill: var(--hs-bg-hover)` discipline (functional area emphasis,
  // not decoration). Solid color, no opacity stacking, no gradient.
  bgHover: string
}

// Light theme: mirror of semantics.css :root values.
export const CHART_COLORS_LIGHT: ChartColors = {
  brand: '#007aff', // --hs-brand (blue-600, v2.0 brand anchor)
  success: '#34c759', // --hs-success (stable, graphic tier)
  warning: '#ff9500', // --hs-warning (degraded, graphic tier)
  danger: '#ff3b30', // --hs-danger (incident, graphic tier)
  failing: '#ff3b30', // --hs-status-failing — merged into incident at the display layer (spec 0018 decision 3)
  textRegular: '#3a3a3c', // --hs-text-regular (gray-800)
  textSecondary: '#86868b', // --hs-text-secondary (gray-500, v2.0 secondary-text anchor)
  placeholder: '#a1a1a6', // --hs-text-placeholder (gray-400)
  border: '#d2d2d7', // --hs-border (gray-200)
  bgHover: '#e8e8ed', // --hs-bg-hover (gray-100) — area-fill mirror
} as const

// Dark theme: dark deferred (spec 0018 decision 10) — every key reserved,
// values temporarily mirroring LIGHT; a later dark spec fills them per the
// semantics.css html.dark block, one by one.
export const CHART_COLORS_DARK: ChartColors = {
  brand: '#007aff',
  success: '#34c759',
  warning: '#ff9500',
  danger: '#ff3b30',
  failing: '#ff3b30',
  textRegular: '#3a3a3c',
  textSecondary: '#86868b',
  placeholder: '#a1a1a6',
  border: '#d2d2d7',
  bgHover: '#e8e8ed',
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
