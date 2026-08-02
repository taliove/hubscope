// Lucide-style line icons (GH #135, reference-design replica): inline SVG
// functional components — 24 viewBox, stroke=currentColor, stroke-width 2,
// round caps/joins, no fill. Hosts size them via CSS (width/height) and the
// glyph color follows the host text color. Path data mirrors the Lucide
// icon set one-to-one so the shell matches the reference design; these are
// static trusted literals, never user input.
import { h, type FunctionalComponent } from 'vue'

type IconChild = [tag: string, attrs: Record<string, string | number>]

function makeIcon(children: IconChild[]): FunctionalComponent {
  return () =>
    h(
      'svg',
      {
        viewBox: '0 0 24 24',
        fill: 'none',
        stroke: 'currentColor',
        'stroke-width': 2,
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round',
        'aria-hidden': 'true',
      },
      children.map(([tag, attrs]) => h(tag, attrs)),
    )
}

// lucide/activity
export const ActivityIcon = makeIcon([
  [
    'path',
    {
      d: 'M22 12h-2.48a2 2 0 0 0-1.93 1.46l-2.35 8.36a.25.25 0 0 1-.48 0L9.24 2.18a.25.25 0 0 0-.48 0l-2.35 8.36A2 2 0 0 1 4.49 12H2',
    },
  ],
])

// lucide/atom
export const AtomIcon = makeIcon([
  ['circle', { cx: 12, cy: 12, r: 1 }],
  [
    'path',
    {
      d: 'M20.2 20.2c2.04-2.03.02-7.36-4.5-11.9-4.54-4.52-9.87-6.54-11.9-4.5-2.04 2.03-.02 7.36 4.5 11.9 4.54 4.52 9.87 6.54 11.9 4.5Z',
    },
  ],
  [
    'path',
    {
      d: 'M15.7 15.7c4.52-4.54 6.54-9.87 4.5-11.9-2.03-2.04-7.36-.02-11.9 4.5-4.52 4.54-6.54 9.87-4.5 11.9 2.03 2.04 7.36.02 11.9-4.5Z',
    },
  ],
])

// lucide/clipboard-list
export const ClipboardListIcon = makeIcon([
  ['rect', { x: 8, y: 2, width: 8, height: 4, rx: 1 }],
  ['path', { d: 'M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2' }],
  ['path', { d: 'M12 11h4' }],
  ['path', { d: 'M12 16h4' }],
  ['path', { d: 'M8 11h.01' }],
  ['path', { d: 'M8 16h.01' }],
])

// lucide/file-text
export const FileTextIcon = makeIcon([
  [
    'path',
    {
      d: 'M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z',
    },
  ],
  ['path', { d: 'M14 2v5a1 1 0 0 0 1 1h5' }],
  ['path', { d: 'M10 9H8' }],
  ['path', { d: 'M16 13H8' }],
  ['path', { d: 'M16 17H8' }],
])

// lucide/triangle-alert
export const TriangleAlertIcon = makeIcon([
  ['path', { d: 'm21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3' }],
  ['path', { d: 'M12 9v4' }],
  ['path', { d: 'M12 17h.01' }],
])

// lucide/settings
export const SettingsIcon = makeIcon([
  [
    'path',
    {
      d: 'M9.671 4.136a2.34 2.34 0 0 1 4.659 0 2.34 2.34 0 0 0 3.319 1.915 2.34 2.34 0 0 1 2.33 4.033 2.34 2.34 0 0 0 0 3.831 2.34 2.34 0 0 1-2.33 4.033 2.34 2.34 0 0 0-3.319 1.915 2.34 2.34 0 0 1-4.659 0 2.34 2.34 0 0 0-3.32-1.915 2.34 2.34 0 0 1-2.33-4.033 2.34 2.34 0 0 1 0-3.831A2.34 2.34 0 0 1 6.35 6.051a2.34 2.34 0 0 0 3.319-1.915',
    },
  ],
  ['circle', { cx: 12, cy: 12, r: 3 }],
])

// lucide/bell — topbar alerts entry (pure navigation, no badge semantics).
export const BellIcon = makeIcon([
  ['path', { d: 'M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9' }],
  ['path', { d: 'M10.3 21a1.94 1.94 0 0 0 3.4 0' }],
])

// lucide/circle-user-round — avatar placeholder in the topbar user chip and
// the sidebar user card (no real avatars exist server-side).
export const CircleUserRoundIcon = makeIcon([
  ['path', { d: 'M18 20a6 6 0 0 0-12 0' }],
  ['circle', { cx: 12, cy: 10, r: 4 }],
  ['circle', { cx: 12, cy: 12, r: 10 }],
])

// lucide/log-out — the single item of the topbar user-chip dropdown.
export const LogOutIcon = makeIcon([
  ['path', { d: 'M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4' }],
  ['polyline', { points: '16 17 21 12 16 7' }],
  ['line', { x1: 21, x2: 9, y1: 12, y2: 12 }],
])

// lucide/menu — the topbar hamburger that opens the nav drawer on narrow
// viewports (2026-08-01 shell drawer batch).
export const MenuIcon = makeIcon([
  ['line', { x1: 4, x2: 20, y1: 6, y2: 6 }],
  ['line', { x1: 4, x2: 20, y1: 12, y2: 12 }],
  ['line', { x1: 4, x2: 20, y1: 18, y2: 18 }],
])

// lucide/x — the hamburger's open state (closes the nav drawer).
export const XIcon = makeIcon([
  ['path', { d: 'M18 6 6 18' }],
  ['path', { d: 'm6 6 12 12' }],
])

// Sidebar glyph per nav key (GH #135). The key set mirrors
// utils/sidebarNav.ts SIDEBAR_NAV_ITEMS one-to-one — the sync is
// vitest-covered in sidebarNav.test.ts.
export const SIDEBAR_ICONS: Record<string, FunctionalComponent> = {
  dashboard: ActivityIcon,
  benchmark: FileTextIcon,
  eval: ClipboardListIcon,
  models: AtomIcon,
  alerts: TriangleAlertIcon,
  settings: SettingsIcon,
}
