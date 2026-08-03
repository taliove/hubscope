// Vendor icon mapping (UI v2 reference replica, part 2; uniform-tile rework
// GH #136; icon-pack expansion GH #140): the model-list NAME cell renders a
// uniform 26x26 brand tile (moved from the vendor column, GH #139) when the
// family maps to a known vendor, else null and the caller falls back to the
// neutral initials tile (familyInitials, GH #131). Centralized as the SINGLE
// SOURCE so no component ever carries its own vendor mapping (role.ts
// precedent).
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
// kimi mark (official Moonshot asset, same viewBox) and the GH #140 pack
// (deepseek / glm / mistral / grok / hunyuan / minimax / wenxin / meta,
// verbatim from lobe-icons, same viewBox) — do not「optimize」or rewrite
// them.

// One glyph path of the mark plus its own fill — multi-color marks (kimi:
// blue dot + white K; the GH #140 multi-color pack) need per-path fills, so
// the fill is NOT a vendor-level field. A gradient fill references a defs
// gradient by id: `url(#<id>)`.
export interface VendorIconPath {
  d: string
  fill: string
}

// A full-circle ground shape of the mark itself (hunyuan: the disc IS the
// brand ground). Rendered BEFORE the paths.
export interface VendorIconCircle {
  cx: number
  cy: number
  r: number
  fill: string
}

export interface VendorIconGradientStop {
  offset: string
  color: string
}

// A linearGradient def. Ids are UNIQUE ACROSS THE WHOLE PACK (per-vendor
// prefix: glm-grad / minimax-grad / wenxin-grad / lobe-icons-meta-N-_R_0_) —
// the SVG inlines once per row, and url(#id) resolves to the first matching
// id in the document. Same-vendor duplicates are identical defs and render
// identically; a cross-vendor collision would corrupt the mark.
export interface VendorIconGradient {
  id: string
  x1: string
  y1: string
  x2: string
  y2: string
  stops: VendorIconGradientStop[]
}

// The three tile variants (GH #140, registered in ui-guidelines §5
// 「供应商图标」):
//   brand  — solid brand-color ground + white glyph (the GH #136 uniform
//            tile; single-color marks).
//   subtle — neutral light ground (--hs-bg-subtle) + ORIGINAL-COLOR mark
//            (multi-color marks: inverting them to white would destroy the
//            brand asset).
//   none   — no tile ground at all; the mark carries its own ground
//            (hunyuan's full-disc mark), the tile seat stays transparent.
export type VendorTileVariant = 'brand' | 'subtle' | 'none'

export interface VendorIcon {
  variant: VendorTileVariant
  // Solid ground color of the uniform 26x26 tile (brand hex). Meaningful
  // only for the 'brand' variant; '' for subtle/none (the ground comes from
  // vendorTileBackground). The solid tile replaced the GH #134「transparent
  // ground, mark stands alone」form: uniform tiles give every vendor the
  // same silhouette, and the dark-theme invisibility of near-black marks
  // (the old openai currentColor exception) dissolves on a solid ground
  // (GH #136).
  tile: string
  // Glyph paths painted over the tile, in document order.
  paths: VendorIconPath[]
  // Optional full-circle ground shapes (hunyuan), rendered before paths.
  circles?: VendorIconCircle[]
  // Optional gradient defs referenced by path fills.
  gradients?: VendorIconGradient[]
}

// vendorTileBackground resolves the tile ground CSS for a variant: the brand
// hex for 'brand', the semantic subtle ground for 'subtle' (a token
// reference, not a brand asset — the only non-brand ground in the pack),
// transparent for 'none' (the mark brings its own ground).
export function vendorTileBackground(icon: VendorIcon): string {
  if (icon.variant === 'brand') return icon.tile
  if (icon.variant === 'subtle') return 'var(--hs-bg-subtle)'
  return 'transparent'
}

// Shared glyph/tile literals of the uniform-tile system: marks render white
// on the brand ground; openai, moonshot and grok share the black tile.
const GLYPH_WHITE = '#fff'
const TILE_BLACK = '#000'

const ANTHROPIC: VendorIcon = {
  // Claude mark on the Anthropic coral tile. Tile #D97757 — deviation from
  // the simple-icons official hex #191919, registered: coral is the real
  // Anthropic web brand color and matches the reference mock.
  variant: 'brand',
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
  variant: 'brand',
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
  variant: 'brand',
  tile: TILE_BLACK,
  paths: [
    {
      d: 'M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.872zm16.5963 3.8558L13.1038 8.364 15.1192 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927-.6813zm1.0976-2.3654l2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997Z',
      fill: GLYPH_WHITE,
    },
  ],
}

const BYTEDANCE: VendorIcon = {
  // ByteDance mark (doubao family) on the official ByteDance blue tile
  // #3C8CFF — chosen over the reference mock's red disc: the brand asset
  // beats the mock's placeholder color (registered trade-off).
  variant: 'brand',
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
  variant: 'brand',
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
  // two-color mark — brand blue dot #1783FF + white K.
  variant: 'brand',
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

// --- GH #140 pack (lobe-icons official paths, verbatim) ----------------------

const DEEPSEEK: VendorIcon = {
  // DeepSeek whale mark (lobe-icons), white on the brand blue tile #4D6BFE —
  // the single-color form of the uniform tile.
  variant: 'brand',
  tile: '#4D6BFE',
  paths: [
    {
      d: 'M23.748 4.482c-.254-.124-.364.113-.512.234-.051.039-.094.09-.137.136-.372.397-.806.657-1.373.626-.829-.046-1.537.214-2.163.848-.133-.782-.575-1.248-1.247-1.548-.352-.156-.708-.311-.955-.65-.172-.241-.219-.51-.305-.774-.055-.16-.11-.323-.293-.35-.2-.031-.278.136-.356.276-.313.572-.434 1.202-.422 1.84.027 1.436.633 2.58 1.838 3.393.137.093.172.187.129.323-.082.28-.18.552-.266.833-.055.179-.137.217-.329.14a5.526 5.526 0 01-1.736-1.18c-.857-.828-1.631-1.742-2.597-2.458a11.365 11.365 0 00-.689-.471c-.985-.957.13-1.743.388-1.836.27-.098.093-.432-.779-.428-.872.004-1.67.295-2.687.684a3.055 3.055 0 01-.465.137 9.597 9.597 0 00-2.883-.102c-1.885.21-3.39 1.102-4.497 2.623C.082 8.606-.231 10.684.152 12.85c.403 2.284 1.569 4.175 3.36 5.653 1.858 1.533 3.997 2.284 6.438 2.14 1.482-.085 3.133-.284 4.994-1.86.47.234.962.327 1.78.397.63.059 1.236-.03 1.705-.128.735-.156.684-.837.419-.961-2.155-1.004-1.682-.595-2.113-.926 1.096-1.296 2.746-2.642 3.392-7.003.05-.347.007-.565 0-.845-.004-.17.035-.237.23-.256a4.173 4.173 0 001.545-.475c1.396-.763 1.96-2.015 2.093-3.517.02-.23-.004-.467-.247-.588zM11.581 18c-2.089-1.642-3.102-2.183-3.52-2.16-.392.024-.321.471-.235.763.09.288.207.486.371.739.114.167.192.416-.113.603-.673.416-1.842-.14-1.897-.167-1.361-.802-2.5-1.86-3.301-3.307-.774-1.393-1.224-2.887-1.298-4.482-.02-.386.093-.522.477-.592a4.696 4.696 0 011.529-.039c2.132.312 3.946 1.265 5.468 2.774.868.86 1.525 1.887 2.202 2.891.72 1.066 1.494 2.082 2.48 2.914.348.292.625.514.891.677-.802.09-2.14.11-3.054-.614zm1-6.44a.306.306 0 01.415-.287.302.302 0 01.2.288.306.306 0 01-.31.307.303.303 0 01-.304-.308zm3.11 1.596c-.2.081-.399.151-.59.16a1.245 1.245 0 01-.798-.254c-.274-.23-.47-.358-.552-.758a1.73 1.73 0 01.016-.588c.07-.327-.008-.537-.239-.727-.187-.156-.426-.199-.688-.199a.559.559 0 01-.254-.078c-.11-.054-.2-.19-.114-.358.028-.054.16-.186.192-.21.356-.202.767-.136 1.146.016.352.144.618.408 1.001.782.391.451.462.576.685.914.176.265.336.537.445.848.067.195-.019.354-.25.452z',
      fill: GLYPH_WHITE,
    },
  ],
}

const GLM: VendorIcon = {
  // Zhipu GLM mark (lobe-icons): a two-stop brand gradient (#504AF4 →
  // #3485FF) — multi-color marks keep their ORIGINAL colors on the subtle
  // ground (variant subtle, GH #140); inverting to white would destroy the
  // asset. Gradient direction (diagonal) is our registered approximation —
  // gradient geometry verbatim from the official color assets (chatglm-color.svg / minimax-color.svg / wenxin-color.svg, captured 2026-08-01).
  variant: 'subtle',
  tile: '',
  paths: [
    {
      d: 'M9.917 2c4.906 0 10.178 3.947 8.93 10.58-.014.07-.037.14-.057.21l-.003-.277c-.083-3-1.534-8.934-8.87-8.934-3.393 0-8.137 3.054-7.93 8.158-.04 4.778 3.555 8.4 7.95 8.332l.073-.001c1.2-.033 2.763-.429 3.1-1.657.063-.031.26.534.268.598.048.256.112.369.192.34.981-.348 2.286-1.222 1.952-2.38-.176-.61-1.775-.147-1.921-.347.418-.979 2.234-.926 3.153-.716.443.102.657.38 1.012.442.29.052.981-.2.96.242-1.5 3.042-4.893 5.41-8.808 5.41C3.654 22 0 16.574 0 11.737 0 5.947 4.959 2 9.917 2zM9.9 5.3c.484 0 1.125.225 1.38.585 3.669.145 4.313 2.686 4.694 5.444.255 1.838.315 2.3.182 1.387l.083.59c.068.448.554.737.982.516.144-.075.254-.231.328-.47a.2.2 0 01.258-.13l.625.22a.2.2 0 01.124.238 2.172 2.172 0 01-.51.92c-.878.917-2.757.664-3.08-.62-.14-.554-.055-.626-.345-1.242-.292-.621-1.238-.709-1.69-.295-.345.315-.407.805-.406 1.282L12.6 15.9a.9.9 0 01-.9.9h-1.4a.9.9 0 01-.9-.9v-.65a1.15 1.15 0 10-2.3 0v.65a.9.9 0 01-.9.9H4.8a.9.9 0 01-.9-.9l.035-3.239c.012-1.884.356-3.658 2.47-4.134.2-.045.252.13.29.342.025.154.043.252.053.294.701 3.058 1.75 4.299 3.144 3.722l.66-.331.254-.13c.158-.082.25-.131.276-.15.012-.01-.165-.206-.407-.464l-1.012-1.067a8.925 8.925 0 01-.199-.216c-.047-.034-.116.068-.208.306-.074.157-.251.252-.272.326-.013.058.108.298.362.72.164.288.22.508-.31.343-1.04-.8-1.518-2.273-1.684-3.725-.004-.035-.162-1.913-.162-1.913a1.2 1.2 0 011.113-1.281L9.9 5.3zm12.994 8.68c.037.697-.403.704-1.213.591l-1.783-.276c-.265-.053-.385-.099-.313-.147.47-.315 3.268-.93 3.31-.168zm-.915-.083l-.926.042c-.85.077-1.452.24.338.336l.103.003c.815.012 1.264-.359.485-.381zm1.667-3.601h.01c.79.398.067 1.03-.65 1.393-.14.07-.491.176-1.052.315-.241.04-.457.092-.333.16l.01.005c1.952.958-3.123 1.534-2.495 1.285l.38-.148c.68-.266 1.614-.682 1.666-1.337.038-.48 1.253-.442 1.493-.968.048-.106 0-.236-.144-.389-.05-.047-.094-.094-.107-.148-.073-.305.7-.431 1.222-.168zm-2.568-.474c-.135 1.198-2.479 4.192-1.949 2.863l.017-.042c.298-.717.376-2.221 1.337-3.221.25-.26.636.035.595.4zm-7.976-.253c.02-.694 1.002-.968 1.346-.347.01-1.274-1.941-.768-1.346.347z',
      fill: 'url(#glm-grad)',
    },
  ],
  gradients: [
    {
      id: 'glm-grad',
      x1: '-18.756%',
      y1: '49.371%',
      x2: '70.894%',
      y2: '90.944%',
      stops: [
        { offset: '0%', color: '#504AF4' },
        { offset: '100%', color: '#3485FF' },
      ],
    },
  ],
}

const MISTRAL: VendorIcon = {
  // Mistral five-row mark (lobe-icons), original colors on the subtle
  // ground: gold → #E10500 top to bottom.
  variant: 'subtle',
  tile: '',
  paths: [
    { d: 'M3.428 3.4h3.429v3.428H3.428V3.4zm13.714 0h3.43v3.428h-3.43V3.4z', fill: 'gold' },
    { d: 'M3.428 6.828h6.857v3.429H3.429V6.828zm10.286 0h6.857v3.429h-6.857V6.828z', fill: '#FFAF00' },
    { d: 'M3.428 10.258h17.144v3.428H3.428v-3.428z', fill: '#FF8205' },
    { d: 'M3.428 13.686h3.429v3.428H3.428v-3.428zm6.858 0h3.429v3.428h-3.429v-3.428zm6.856 0h3.43v3.428h-3.43v-3.428z', fill: '#FA500F' },
    { d: 'M0 17.114h10.286v3.429H0v-3.429zm13.714 0H24v3.429H13.714v-3.429z', fill: '#E10500' },
  ],
}

const GROK: VendorIcon = {
  // xAI Grok mark (lobe-icons; the source is currentColor) — white on the
  // black tile, the single-color uniform form (openai precedent).
  variant: 'brand',
  tile: TILE_BLACK,
  paths: [
    {
      d: 'M9.27 15.29l7.978-5.897c.391-.29.95-.177 1.137.272.98 2.369.542 5.215-1.41 7.169-1.951 1.954-4.667 2.382-7.149 1.406l-2.711 1.257c3.889 2.661 8.611 2.003 11.562-.953 2.341-2.344 3.066-5.539 2.388-8.42l.006.007c-.983-4.232.242-5.924 2.75-9.383.06-.082.12-.164.179-.248l-3.301 3.305v-.01L9.267 15.292M7.623 16.723c-2.792-2.67-2.31-6.801.071-9.184 1.761-1.763 4.647-2.483 7.166-1.425l2.705-1.25a7.808 7.808 0 00-1.829-1A8.975 8.975 0 005.984 5.83c-2.533 2.536-3.33 6.436-1.962 9.764 1.022 2.487-.653 4.246-2.34 6.022-.599.63-1.199 1.259-1.682 1.925l7.62-6.815',
      fill: GLYPH_WHITE,
    },
  ],
}

const HUNYUAN: VendorIcon = {
  // Tencent Hunyuan disc mark (lobe-icons): the full circle IS the brand
  // ground (#0055E9) — variant none: the tile seat stays transparent and
  // the disc renders inside the glyph seat itself.
  variant: 'none',
  tile: '',
  circles: [{ cx: 12, cy: 12, r: 12, fill: '#0055E9' }],
  paths: [
    {
      d: 'M12 0c.518 0 1.028.033 1.528.096A6.188 6.188 0 0112.12 12.28l-.12.001c-2.99 0-5.242 2.179-5.554 5.11-.223 2.086.353 4.412 2.242 6.146C3.672 22.1 0 17.479 0 12 0 5.373 5.373 0 12 0z',
      fill: '#A8DFF5',
    },
    {
      d: 'M5.286 5a2.438 2.438 0 01.682 3.38c-3.962 5.966-3.215 10.743 2.648 15.136C3.636 22.056 0 17.452 0 12c0-1.787.39-3.482 1.09-5.006.253-.435.525-.872.817-1.311A2.438 2.438 0 015.286 5z',
      fill: '#0055E9',
    },
    {
      d: 'M12.98.04c.272.021.543.053.81.093.583.106 1.117.254 1.538.44 6.638 2.927 8.07 10.052 1.748 15.642a4.125 4.125 0 01-5.822-.358c-1.51-1.706-1.3-4.184.357-5.822.858-.848 3.108-1.223 4.045-2.441 1.257-1.634 2.122-6.009-2.523-7.506L12.98.039z',
      fill: '#00BCFF',
    },
    {
      d: 'M13.528.096A6.187 6.187 0 0112 12.281a5.75 5.75 0 00-1.71.255c.147-.905.595-1.784 1.321-2.501.858-.848 3.108-1.223 4.045-2.441 1.27-1.651 2.14-6.104-2.676-7.554.184.014.367.033.548.056z',
      fill: '#ECECEE',
    },
  ],
}

const MINIMAX: VendorIcon = {
  // MiniMax wave mark (lobe-icons): brand gradient #E2167E → #FE603C on
  // the subtle ground (original-color variant). Gradient direction
  // (diagonal) is our registered approximation, same as glm.
  variant: 'subtle',
  tile: '',
  paths: [
    {
      d: 'M16.278 2c1.156 0 2.093.927 2.093 2.07v12.501a.74.74 0 00.744.709.74.74 0 00.743-.709V9.099a2.06 2.06 0 012.071-2.049A2.06 2.06 0 0124 9.1v6.561a.649.649 0 01-.652.645.649.649 0 01-.653-.645V9.1a.762.762 0 00-.766-.758.762.762 0 00-.766.758v7.472a2.037 2.037 0 01-2.048 2.026 2.037 2.037 0 01-2.048-2.026v-12.5a.785.785 0 00-.788-.753.785.785 0 00-.789.752l-.001 15.904A2.037 2.037 0 0113.441 22a2.037 2.037 0 01-2.048-2.026V18.04c0-.356.292-.645.652-.645.36 0 .652.289.652.645v1.934c0 .263.142.506.372.638.23.131.514.131.744 0a.734.734 0 00.372-.638V4.07c0-1.143.937-2.07 2.093-2.07zm-5.674 0c1.156 0 2.093.927 2.093 2.07v11.523a.648.648 0 01-.652.645.648.648 0 01-.652-.645V4.07a.785.785 0 00-.789-.78.785.785 0 00-.789.78v14.013a2.06 2.06 0 01-2.07 2.048 2.06 2.06 0 01-2.071-2.048V9.1a.762.762 0 00-.766-.758.762.762 0 00-.766.758v3.8a2.06 2.06 0 01-2.071 2.049A2.06 2.06 0 010 12.9v-1.378c0-.357.292-.646.652-.646.36 0 .653.29.653.646V12.9c0 .418.343.757.766.757s.766-.339.766-.757V9.099a2.06 2.06 0 012.07-2.048 2.06 2.06 0 012.071 2.048v8.984c0 .419.343.758.767.758.423 0 .766-.339.766-.758V4.07c0-1.143.937-2.07 2.093-2.07z',
      fill: 'url(#minimax-grad)',
    },
  ],
  gradients: [
    {
      id: 'minimax-grad',
      x1: '0%',
      y1: '50.057%',
      x2: '100.182%',
      y2: '50.057%',
      stops: [
        { offset: '0%', color: '#E2167E' },
        { offset: '100%', color: '#FE603C' },
      ],
    },
  ],
}

const WENXIN: VendorIcon = {
  // Baidu Wenxin (ERNIE) hexagon mark (lobe-icons): brand gradient
  // #0A51C3 → #23A4FB + the solid #012F8D inner mark, original colors on
  // the subtle ground. Gradient direction (vertical) is our registered
  // approximation.
  variant: 'subtle',
  tile: '',
  paths: [
    {
      d: 'M11.32 1.176a1.4 1.4 0 011.36 0l8.64 4.843c.421.234.68.67.68 1.141v9.68c0 .472-.259.908-.68 1.143l-8.64 4.84a1.4 1.4 0 01-1.36 0l-8.64-4.84A1.31 1.31 0 012 16.84V7.159c0-.471.259-.907.68-1.142l8.64-4.84zm7.42 13.839V8.227L12.002 12 12 19.551l6.059-3.394a1.31 1.31 0 00.68-1.142zM12.68 4.833a1.393 1.393 0 00-1.36 0L5.944 7.846c-.421.235-.68.67-.68 1.142v6.027c0 .47.259.905.68 1.142l2.795 1.566V11.09a1.546 1.546 0 00.221.79 1.527 1.527 0 01-.216-.834l.004-.094.02-.15.018-.084.017-.062.039-.117.062-.142.035-.065.081-.13.094-.122.084-.091.08-.075.125-.1.071-.048.134-.076 5.87-3.29-2.796-1.566z',
      fill: 'url(#wenxin-grad)',
    },
    {
      d: 'M12 11.088c0-.875-.73-1.584-1.631-1.584a1.66 1.66 0 00-.855.237c-.027.016-.055.033-.08.05a2.361 2.361 0 00-.123.093c-.022.02-.045.038-.066.059l-.048.045-.063.067c-.014.016-.028.031-.04.048a2.303 2.303 0 00-.094.125l-.042.069a1.7 1.7 0 00-.07.13l-.036.081a.764.764 0 00-.022.06c-.01.03-.02.058-.028.087l-.017.062a.883.883 0 00-.03.16c-.002.025-.007.05-.008.074a1.527 1.527 0 00.213.929c.302.508.85.792 1.414.792.277 0 .558-.068.814-.212l.815-.457v-.914L12 11.088z',
      fill: '#012F8D',
    },
  ],
  gradients: [
    {
      id: 'wenxin-grad',
      x1: '9.155%',
      y1: '75.177%',
      x2: '90.531%',
      y2: '25.028%',
      stops: [
        { offset: '0%', color: '#0A51C3' },
        { offset: '100%', color: '#23A4FB' },
      ],
    },
  ],
}

const META: VendorIcon = {
  // Meta infinity mark (lobe-icons, for the llama family): 15 paths over 13
  // brand-blue gradients — verbatim from the official lobe-icons SVG
  // (paths, gradient geometry and ids untouched). Original colors on the
  // subtle ground.
  variant: 'subtle',
  tile: '',
  paths: [
    { d: 'M6.897 4h-.024l-.031 2.615h.022c1.715 0 3.046 1.357 5.94 6.246l.175.297.012.02 1.62-2.438-.012-.019a48.763 48.763 0 00-1.098-1.716 28.01 28.01 0 00-1.175-1.629C10.413 4.932 8.812 4 6.896 4z', fill: 'url(#lobe-icons-meta-0-_R_0_)' },
    { d: 'M6.873 4C4.95 4.01 3.247 5.258 2.02 7.17a4.352 4.352 0 00-.01.017l2.254 1.231.011-.017c.718-1.083 1.61-1.774 2.568-1.785h.021L6.896 4h-.023z', fill: 'url(#lobe-icons-meta-1-_R_0_)' },
    { d: 'M2.019 7.17l-.011.017C1.2 8.447.598 9.995.274 11.664l-.005.022 2.534.6.004-.022c.27-1.467.786-2.828 1.456-3.845l.011-.017L2.02 7.17z', fill: 'url(#lobe-icons-meta-2-_R_0_)' },
    { d: 'M2.807 12.264l-2.533-.6-.005.022c-.177.918-.267 1.851-.269 2.786v.023l2.598.233v-.023a12.591 12.591 0 01.21-2.44z', fill: 'url(#lobe-icons-meta-3-_R_0_)' },
    { d: 'M2.677 15.537a5.462 5.462 0 01-.079-.813v-.022L0 14.468v.024a8.89 8.89 0 00.146 1.652l2.535-.585a4.106 4.106 0 01-.004-.022z', fill: 'url(#lobe-icons-meta-4-_R_0_)' },
    { d: 'M3.27 16.89c-.284-.31-.484-.756-.589-1.328l-.004-.021-2.535.585.004.021c.192 1.01.568 1.85 1.106 2.487l.014.017 2.018-1.745a2.106 2.106 0 01-.015-.016z', fill: 'url(#lobe-icons-meta-5-_R_0_)' },
    { d: 'M10.78 9.654c-1.528 2.35-2.454 3.825-2.454 3.825-2.035 3.2-2.739 3.917-3.871 3.917a1.545 1.545 0 01-1.186-.508l-2.017 1.744.014.017C2.01 19.518 3.058 20 4.356 20c1.963 0 3.374-.928 5.884-5.33l1.766-3.13a41.283 41.283 0 00-1.227-1.886z', fill: '#0082FB' },
    { d: 'M13.502 5.946l-.016.016c-.4.43-.786.908-1.16 1.416.378.483.768 1.024 1.175 1.63.48-.743.928-1.345 1.367-1.807l.016-.016-1.382-1.24z', fill: 'url(#lobe-icons-meta-6-_R_0_)' },
    { d: 'M20.918 5.713C19.853 4.633 18.583 4 17.225 4c-1.432 0-2.637.787-3.723 1.944l-.016.016 1.382 1.24.016-.017c.715-.747 1.408-1.12 2.176-1.12.826 0 1.6.39 2.27 1.075l.015.016 1.589-1.425-.016-.016z', fill: '#0082FB' },
    { d: 'M23.998 14.125c-.06-3.467-1.27-6.566-3.064-8.396l-.016-.016-1.588 1.424.015.016c1.35 1.392 2.277 3.98 2.361 6.971v.023h2.292v-.022z', fill: 'url(#lobe-icons-meta-7-_R_0_)' },
    { d: 'M23.998 14.15v-.023h-2.292v.022c.004.14.006.282.006.424 0 .815-.121 1.474-.368 1.95l-.011.022 1.708 1.782.013-.02c.62-.96.946-2.293.946-3.91 0-.083 0-.165-.002-.247z', fill: 'url(#lobe-icons-meta-8-_R_0_)' },
    { d: 'M21.344 16.52l-.011.02c-.214.402-.519.67-.917.787l.778 2.462a3.493 3.493 0 00.438-.182 3.558 3.558 0 001.366-1.218l.044-.065.012-.02-1.71-1.784z', fill: 'url(#lobe-icons-meta-9-_R_0_)' },
    { d: 'M19.92 17.393c-.262 0-.492-.039-.718-.14l-.798 2.522c.449.153.927.222 1.46.222.492 0 .943-.073 1.352-.215l-.78-2.462c-.167.05-.341.075-.517.073z', fill: 'url(#lobe-icons-meta-10-_R_0_)' },
    { d: 'M18.323 16.534l-.014-.017-1.836 1.914.016.017c.637.682 1.246 1.105 1.937 1.337l.797-2.52c-.291-.125-.573-.353-.9-.731z', fill: 'url(#lobe-icons-meta-11-_R_0_)' },
    { d: 'M18.309 16.515c-.55-.642-1.232-1.712-2.303-3.44l-1.396-2.336-.011-.02-1.62 2.438.012.02.989 1.668c.959 1.61 1.74 2.774 2.493 3.585l.016.016 1.834-1.914a2.353 2.353 0 01-.014-.017z', fill: 'url(#lobe-icons-meta-12-_R_0_)' },
  ],
  gradients: [
    { id: 'lobe-icons-meta-0-_R_0_', x1: '75.897%', y1: '89.199%', x2: '26.312%', y2: '12.194%', stops: [{ offset: '.06%', color: '#0867DF' }, { offset: '45.39%', color: '#0668E1' }, { offset: '85.91%', color: '#0064E0' }] },
    { id: 'lobe-icons-meta-1-_R_0_', x1: '21.67%', y1: '75.874%', x2: '97.068%', y2: '23.985%', stops: [{ offset: '13.23%', color: '#0064DF' }, { offset: '99.88%', color: '#0064E0' }] },
    { id: 'lobe-icons-meta-2-_R_0_', x1: '38.263%', y1: '89.127%', x2: '60.895%', y2: '16.131%', stops: [{ offset: '1.47%', color: '#0072EC' }, { offset: '68.81%', color: '#0064DF' }] },
    { id: 'lobe-icons-meta-3-_R_0_', x1: '47.032%', y1: '90.19%', x2: '52.15%', y2: '15.745%', stops: [{ offset: '7.31%', color: '#007CF6' }, { offset: '99.43%', color: '#0072EC' }] },
    { id: 'lobe-icons-meta-4-_R_0_', x1: '52.155%', y1: '58.301%', x2: '47.591%', y2: '37.004%', stops: [{ offset: '7.31%', color: '#007FF9' }, { offset: '100%', color: '#007CF6' }] },
    { id: 'lobe-icons-meta-5-_R_0_', x1: '37.689%', y1: '12.502%', x2: '61.961%', y2: '63.624%', stops: [{ offset: '7.31%', color: '#007FF9' }, { offset: '100%', color: '#0082FB' }] },
    { id: 'lobe-icons-meta-6-_R_0_', x1: '34.808%', y1: '68.859%', x2: '62.313%', y2: '23.174%', stops: [{ offset: '27.99%', color: '#007FF8' }, { offset: '91.41%', color: '#0082FB' }] },
    { id: 'lobe-icons-meta-7-_R_0_', x1: '43.762%', y1: '6.235%', x2: '57.602%', y2: '98.514%', stops: [{ offset: '0%', color: '#0082FB' }, { offset: '99.95%', color: '#0081FA' }] },
    { id: 'lobe-icons-meta-8-_R_0_', x1: '60.055%', y1: '4.661%', x2: '39.88%', y2: '69.077%', stops: [{ offset: '6.19%', color: '#0081FA' }, { offset: '100%', color: '#0080F9' }] },
    { id: 'lobe-icons-meta-9-_R_0_', x1: '30.282%', y1: '59.32%', x2: '61.081%', y2: '33.244%', stops: [{ offset: '0%', color: '#027AF3' }, { offset: '100%', color: '#0080F9' }] },
    { id: 'lobe-icons-meta-10-_R_0_', x1: '20.433%', y1: '50.001%', x2: '82.112%', y2: '50.001%', stops: [{ offset: '0%', color: '#0377EF' }, { offset: '99.94%', color: '#0279F1' }] },
    { id: 'lobe-icons-meta-11-_R_0_', x1: '40.303%', y1: '35.298%', x2: '72.394%', y2: '57.811%', stops: [{ offset: '.19%', color: '#0471E9' }, { offset: '100%', color: '#0377EF' }] },
    { id: 'lobe-icons-meta-12-_R_0_', x1: '32.254%', y1: '19.719%', x2: '68.003%', y2: '84.908%', stops: [{ offset: '27.65%', color: '#0867DF' }, { offset: '100%', color: '#0471E9' }] },
  ],
}

// Canonical key → icon. Lookup normalizes case/whitespace and aliases, so
// both the classifier's family values (claude / gpt / gemini / qwen /
// doubao / kimi / deepseek / glm / mistral / grok / hunyuan / minimax /
// ernie / llama) and vendor names (anthropic / openai / google / alibaba /
// bytedance / moonshot / zhipu / xai / tencent / baidu / meta) hit the same
// entry.
const ICONS: Record<string, VendorIcon> = {
  anthropic: ANTHROPIC,
  googlegemini: GOOGLE_GEMINI,
  openai: OPENAI,
  bytedance: BYTEDANCE,
  alibabacloud: ALIBABA_CLOUD,
  moonshot: MOONSHOT,
  deepseek: DEEPSEEK,
  glm: GLM,
  mistral: MISTRAL,
  grok: GROK,
  hunyuan: HUNYUAN,
  minimax: MINIMAX,
  wenxin: WENXIN,
  meta: META,
}

// Alias → canonical key. Covers the classifier's canonical family values
// (internal/classifier: gpt / claude / gemini / qwen / doubao / kimi /
// deepseek / glm / mistral / grok / hunyuan / minimax / ernie / llama) and
// common vendor spellings. Keys are the NORMALIZED form (trim + lowercase +
// strip non-alphanumerics):「deepseek-ai」normalizes to deepseekai.
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
  deepseekai: 'deepseek',
  zhipu: 'glm',
  chatglm: 'glm',
  mixtral: 'mistral',
  xai: 'grok',
  tencent: 'hunyuan',
  ernie: 'wenxin',
  baidu: 'wenxin',
  llama: 'meta',
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
