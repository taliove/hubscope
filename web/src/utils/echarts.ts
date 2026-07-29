// Shared modular echarts entry (spec 0015 decision 6; review dedup GH #23):
// registers the union of what the chart components use, exactly once, so
// TimeSeriesChart and TrendChart can't drift into two registration copies —
// the same single-source precedent as utils/chartColors.ts. Every registered
// item maps to a setOption key used by one of the components:
//   LineChart        -> series[].type: 'line'
//   GridComponent    -> grid
//   TooltipComponent -> tooltip (its install also brings axisPointer)
//   LegendComponent  -> legend
//   MarkLineComponent-> series[].markLine (suite-version break markers)
//   CanvasRenderer   -> default renderer for echarts.init
// Full `import 'echarts'` would pull ~1MB into the bundle; keep this list in
// sync with the components' setOption configs when adding chart features.
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  MarkLineComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([
  LineChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  MarkLineComponent,
  CanvasRenderer,
])

export { echarts }
