// Vendor icon mapping (UI v2 reference replica, part 2; uniform-tile rework
// GH #136): the model-list NAME cell renders a uniform 26x26 brand tile (moved from the vendor column, GH #139)
// — solid brand-color ground + white glyph — when the family maps to a known
// vendor, else null and the caller falls back to the neutral initials tile
// (familyInitials, GH #131). Centralized as the SINGLE SOURCE so no
// component ever carries its own vendor mapping (role.ts precedent).
//
// Token-discipline exemption (registered in ui-guidelines §5「供应商图标」):
// vendor brand colors are EXTERNAL BRAND ASSETS, not semantic expression —
// the BrandMark §2b precedent (graphic identity, not palette semantics).
// The hex literals below are the official brand colors; do NOT「tokenize」
// them. This file is on the check-tokens exemption list alongside
// BrandMark.vue and chartColors.ts; the impeccable detector exemptions live
// in .impeccable/config.json.
//
// SVG paths are verbatim from simple-icons (viewBox 0 0 24 24) except the
// kimi mark (official Moonshot asset, same viewBox) — do not「optimize」or
// rewrite them.

// One glyph path of the mark plus its own fill — multi-color marks (kimi:
// blue dot + white K) need per-path fills, so the fill is NOT a
// vendor-level field.
export interface VendorIconPath {
  d: string
  fill: string
}

export interface VendorIcon {
  // Solid ground color of the uniform 26x26 tile (brand hex). The solid
  // tile replaced the GH #134「transparent ground, mark stands alone」form:
  // uniform tiles give every vendor the same silhouette, and the dark-theme
  // invisibility of near-black marks (the old openai currentColor
  // exception) dissolves on a solid ground (GH #136).
  tile: string
  // Glyph paths painted over the tile, in document order.
  paths: VendorIconPath[]
}

// Shared glyph/tile literals of the uniform-tile system: marks render white
// on the brand ground; openai and moonshot share the black tile.
const GLYPH_WHITE = '#fff'
const TILE_BLACK = '#000'

const ANTHROPIC: VendorIcon = {
  // Claude mark on the Anthropic coral tile. Tile #D97757 — deviation from
  // the simple-icons official hex #191919, registered: coral is the real
  // Anthropic web brand color and matches the reference mock.
  tile: '#D97757',
  paths: [
    {
      d: 'M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z',
      fill: GLYPH_WHITE,
    },
  ],
}

const GOOGLE_GEMINI: VendorIcon = {
  // Gemini spark mark on the reference-blue tile #4E86F5 — deviation from
  // the simple-icons official hex #8E75B2 (purple), registered: the
  // reference mock renders the mark blue.
  tile: '#4E86F5',
  paths: [
    {
      d: 'M11.04 19.32Q12 21.51 12 24q0-2.49.93-4.68.96-2.19 2.58-3.81t3.81-2.55Q21.51 12 24 12q-2.49 0-4.68-.93a12.3 12.3 0 0 1-3.81-2.58 12.3 12.3 0 0 1-2.58-3.81Q12 2.49 12 0q0 2.49-.96 4.68-.93 2.19-2.55 3.81a12.3 12.3 0 0 1-3.81 2.58Q2.49 12 0 12q2.49 0 4.68.96 2.19.93 3.81 2.55t2.55 3.81',
      fill: GLYPH_WHITE,
    },
  ],
}

const OPENAI: VendorIcon = {
  // OpenAI knot mark, white on the black tile. The GH #134 currentColor
  // exception (near-black mark adapting to the chip ink) retires with the
  // uniform tile: a solid black ground + white glyph is visible on both
  // themes by construction (GH #136).
  tile: TILE_BLACK,
  paths: [
    {
      d: 'M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.872zm16.5963 3.8558L13.1038 8.364 15.1192 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927.6813zm1.0976-2.3654l2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997Z',
      fill: GLYPH_WHITE,
    },
  ],
}

const BYTEDANCE: VendorIcon = {
  // ByteDance mark (doubao family) on the official ByteDance blue tile
  // #3C8CFF — chosen over the reference mock's red disc: the brand asset
  // beats the mock's placeholder color (registered trade-off).
  tile: '#3C8CFF',
  paths: [
    {
      d: 'M19.8772 1.4685L24 2.5326v18.9426l-4.1228 1.0563V1.4685zm-13.3481 9.428l4.115 1.0641v8.9786l-4.115 1.0642v-11.107zM0 2.572l4.115 1.0642v16.7354L0 21.428V2.572zm17.4553 5.6205v11.107l-4.1228-1.0642V9.2568l4.1228-1.0642z',
      fill: GLYPH_WHITE,
    },
  ],
}

const ALIBABA_CLOUD: VendorIcon = {
  // Alibaba Cloud mark (qwen family) on the brand orange tile #FF6A00.
  tile: '#FF6A00',
  paths: [
    {
      d: 'M3.996 4.517h5.291L8.01 6.324 4.153 7.506a1.668 1.668 0 0 0-1.165 1.601v5.786a1.668 1.668 0 0 0 1.165 1.6l3.857 1.183 1.277 1.807H3.996A3.996 3.996 0 0 1 0 15.487V8.513a3.996 3.996 0 0 1 3.996-3.996m16.008 0h-5.291l1.277 1.807 3.857 1.182c.715.227 1.17.889 1.165 1.601v5.786a1.668 1.668 0 0 1-1.165 1.6l-3.857 1.183-1.277 1.807h5.291A3.996 3.996 0 0 0 24 15.487V8.513a3.996 3.996 0 0 0-3.996-3.996m-4.007 8.345H8.002v-1.804h7.995Z',
      fill: GLYPH_WHITE,
    },
  ],
}

const MOONSHOT: VendorIcon = {
  // Kimi mark (Moonshot AI, GH #136) on the black tile: the official
  // two-color mark — brand blue dot #1783FF + white K. This is the only
  // multi-color mark in the set; per-path fills carry its two colors.
  tile: TILE_BLACK,
  paths: [
    {
      d: 'M21.846 0a1.923 1.923 0 110 3.846H20.15a.226.226 0 01-.227-.226V1.923C19.923.861 20.784 0 21.846 0z',
      fill: '#1783FF',
    },
    {
      d: 'M11.065 11.199l7.257-7.2c.137-.136.06-.41-.116-.41H14.3a.164.164 0 00-.117.051l-7.82 7.756c-.122.12-.302.013-.302-.179V3.82c0-.127-.083-.23-.185-.23H3.186c-.103 0-.186.103-.186.23V19.77c0 .128.083.23.186.23h2.69c.103 0 .186-.102.186-.23v-3.25c0-.069.025-.135.069-.178l2.424-2.406a.158.158 0 01.205-.023l6.484 4.772a7.677 7.677 0 003.453 1.283c.108.012.2-.095.2-.23v-3.06c0-.117-.07-.212-.164-.227a5.028 5.028 0 01-2.027-.807l-5.613-4.064c-.117-.078-.132-.279-.028-.381z',
      fill: GLYPH_WHITE,
    },
  ],
}

// Canonical key → icon. Lookup normalizes case/whitespace and aliases, so
// both the classifier's family values (claude / gpt / gemini / qwen /
// doubao) and vendor names (anthropic / openai / google / alibaba /
// bytedance / moonshot) hit the same entry.
const ICONS: Record<string, VendorIcon> = {
  anthropic: ANTHROPIC,
  googlegemini: GOOGLE_GEMINI,
  openai: OPENAI,
  bytedance: BYTEDANCE,
  alibabacloud: ALIBABA_CLOUD,
  moonshot: MOONSHOT,
}

// Alias → canonical key. Covers the classifier's canonical family values
// (internal/classifier: gpt / claude / gemini / qwen / doubao) and common
// vendor spellings.
const ALIASES: Record<string, string> = {
  claude: 'anthropic',
  gpt: 'openai',
  gemini: 'googlegemini',
  google: 'googlegemini',
  qwen: 'alibabacloud',
  alibaba: 'alibabacloud',
  doubao: 'bytedance',
  kimi: 'moonshot',
  moonshotai: 'moonshot',
}

// Resolve a family string to its vendor icon, or null when the vendor is
// unknown (caller falls back to the initials tile). Normalization:
// trim + lowercase + strip non-alphanumerics (「Ali Cloud」→ alicloud-shaped
// input still misses unless aliased — unknown stays null by design).
export function vendorIcon(family: string): VendorIcon | null {
  const key = family.trim().toLowerCase().replace(/[^a-z0-9]+/g, '')
  if (!key) return null
  const canonical = ALIASES[key] ?? key
  return ICONS[canonical] ?? null
}
