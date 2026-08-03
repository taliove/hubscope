import { describe, expect, it } from 'vitest'
import { vendorIcon, vendorTileBackground } from '@/utils/vendorIcon'

describe('vendorIcon', () => {
  it('resolves the classifier canonical family values to their brand tiles', () => {
    // internal/classifier canonical families: claude / gpt / gemini / qwen /
    // doubao / kimi / deepseek / glm / mistral / grok / hunyuan / minimax
    // (+ ernie → wenxin, llama → meta via alias, GH #140)
    expect(vendorIcon('claude')?.tile).toBe('#D97757')
    expect(vendorIcon('gpt')?.tile).toBe('#000')
    expect(vendorIcon('gemini')?.tile).toBe('#4E86F5')
    expect(vendorIcon('qwen')?.tile).toBe('#FF6A00')
    expect(vendorIcon('doubao')?.tile).toBe('#3C8CFF')
    expect(vendorIcon('kimi')?.tile).toBe('#000')
    expect(vendorIcon('deepseek')?.tile).toBe('#4D6BFE')
    expect(vendorIcon('grok')?.tile).toBe('#000')
    expect(vendorIcon('glm')?.variant).toBe('subtle')
    expect(vendorIcon('mistral')?.variant).toBe('subtle')
    expect(vendorIcon('hunyuan')?.variant).toBe('none')
    expect(vendorIcon('minimax')?.variant).toBe('subtle')
    expect(vendorIcon('ernie')?.variant).toBe('subtle')
    expect(vendorIcon('llama')?.variant).toBe('subtle')
  })

  it('resolves vendor-name aliases to the same icons', () => {
    expect(vendorIcon('anthropic')).toEqual(vendorIcon('claude'))
    expect(vendorIcon('openai')).toEqual(vendorIcon('gpt'))
    expect(vendorIcon('google')).toEqual(vendorIcon('gemini'))
    expect(vendorIcon('googlegemini')).toEqual(vendorIcon('gemini'))
    expect(vendorIcon('alibaba')).toEqual(vendorIcon('qwen'))
    expect(vendorIcon('alibabacloud')).toEqual(vendorIcon('qwen'))
    expect(vendorIcon('bytedance')).toEqual(vendorIcon('doubao'))
    expect(vendorIcon('moonshot')).toEqual(vendorIcon('kimi'))
    expect(vendorIcon('moonshotai')).toEqual(vendorIcon('kimi'))
    // GH #140 icon-pack aliases
    expect(vendorIcon('deepseek-ai')).toEqual(vendorIcon('deepseek'))
    expect(vendorIcon('zhipu')).toEqual(vendorIcon('glm'))
    expect(vendorIcon('chatglm')).toEqual(vendorIcon('glm'))
    expect(vendorIcon('mixtral')).toEqual(vendorIcon('mistral'))
    expect(vendorIcon('xai')).toEqual(vendorIcon('grok'))
    expect(vendorIcon('tencent')).toEqual(vendorIcon('hunyuan'))
    expect(vendorIcon('baidu')).toEqual(vendorIcon('ernie'))
    expect(vendorIcon('wenxin')).toEqual(vendorIcon('ernie'))
    expect(vendorIcon('meta')).toEqual(vendorIcon('llama'))
  })

  it('normalizes case and whitespace', () => {
    expect(vendorIcon(' Claude ')).toEqual(vendorIcon('claude'))
    expect(vendorIcon('GPT')).toEqual(vendorIcon('gpt'))
    expect(vendorIcon('Gemini')).toEqual(vendorIcon('gemini'))
    expect(vendorIcon('Kimi')).toEqual(vendorIcon('kimi'))
    expect(vendorIcon('DeepSeek')).toEqual(vendorIcon('deepseek'))
    expect(vendorIcon('GLM')).toEqual(vendorIcon('glm'))
    expect(vendorIcon('Llama')).toEqual(vendorIcon('llama'))
  })

  it('returns null for unknown vendors (initials-chip fallback)', () => {
    expect(vendorIcon('baichuan')).toBeNull()
    expect(vendorIcon('cohere')).toBeNull()
    expect(vendorIcon('spark')).toBeNull()
    expect(vendorIcon('yi')).toBeNull()
    expect(vendorIcon('phi')).toBeNull()
    expect(vendorIcon('other')).toBeNull()
    expect(vendorIcon('')).toBeNull()
    expect(vendorIcon('   ')).toBeNull()
  })

  it('always carries at least one 24x24 glyph path (every variant)', () => {
    const families = [
      'claude', 'gpt', 'gemini', 'qwen', 'doubao', 'kimi',
      'deepseek', 'glm', 'mistral', 'grok', 'hunyuan', 'minimax', 'ernie', 'llama',
    ]
    for (const family of families) {
      const icon = vendorIcon(family)
      expect(icon).not.toBeNull()
      expect(icon!.paths.length).toBeGreaterThan(0)
      for (const p of icon!.paths) {
        expect(p.d.length).toBeGreaterThan(0)
        expect(p.fill.length).toBeGreaterThan(0)
      }
    }
  })

  it('brand-variant tiles carry a solid brand ground; subtle/none do not', () => {
    // The uniform-tile discipline (GH #136) keeps its three GH #140 variants
    // straight: brand = solid brand ground (white glyph); subtle = neutral
    // light ground (original-color mark); none = no ground (the mark brings
    // its own, hunyuan's full-disc mark).
    for (const family of ['claude', 'gpt', 'gemini', 'qwen', 'doubao', 'kimi', 'deepseek', 'grok']) {
      const icon = vendorIcon(family)!
      expect(icon.variant).toBe('brand')
      expect(icon.tile.length).toBeGreaterThan(0)
    }
    for (const family of ['glm', 'mistral', 'minimax', 'ernie', 'llama']) {
      expect(vendorIcon(family)!.variant).toBe('subtle')
    }
    expect(vendorIcon('hunyuan')!.variant).toBe('none')
  })

  it('renders single-color marks as one white glyph path over the brand tile', () => {
    // The uniform-tile discipline (GH #136): brand ground + white glyph.
    // openai is no longer currentColor — the solid tile solves the dark-theme
    // invisibility that motivated the currentColor exception. GH #140 adds
    // deepseek (brand blue) and grok (black tile) to the same form.
    for (const family of ['claude', 'gpt', 'gemini', 'qwen', 'doubao', 'deepseek', 'grok']) {
      const icon = vendorIcon(family)!
      expect(icon.paths).toHaveLength(1)
      expect(icon.paths[0]!.fill).toBe('#fff')
    }
  })

  it('renders kimi as the two-path mark: blue dot + white K over the black tile', () => {
    const icon = vendorIcon('kimi')!
    expect(icon.tile).toBe('#000')
    expect(icon.paths).toHaveLength(2)
    expect(icon.paths[0]!.fill).toBe('#1783FF')
    expect(icon.paths[1]!.fill).toBe('#fff')
  })

  it('renders glm / minimax / wenxin gradient marks with uniquified gradient ids', () => {
    // Multi-color marks keep their ORIGINAL colors on a subtle ground (GH
    // #140) — gradient fills reference defs ids that are unique across the
    // whole pack (per-vendor prefix), so inlined rows never collide.
    expect(vendorIcon('glm')!.paths[0]!.fill).toBe('url(#glm-grad)')
    expect(vendorIcon('glm')!.gradients?.map(g => g.id)).toEqual(['glm-grad'])
    expect(vendorIcon('minimax')!.paths[0]!.fill).toBe('url(#minimax-grad)')
    expect(vendorIcon('wenxin')!.paths[0]!.fill).toBe('url(#wenxin-grad)')
    expect(vendorIcon('wenxin')!.paths[1]!.fill).toBe('#012F8D')
  })

  it('renders mistral as the five-row original-color mark', () => {
    const icon = vendorIcon('mistral')!
    expect(icon.paths).toHaveLength(5)
    expect(icon.paths.map(p => p.fill)).toEqual(['gold', '#FFAF00', '#FF8205', '#FA500F', '#E10500'])
  })

  it('renders hunyuan as the self-grounded disc mark (no tile)', () => {
    // The full-circle mark carries its own #0055E9 ground — the tile seat
    // stays transparent (variant none), the disc renders inside the glyph
    // seat itself.
    const icon = vendorIcon('hunyuan')!
    expect(icon.variant).toBe('none')
    expect(icon.circles).toHaveLength(1)
    expect(icon.circles![0]!.fill).toBe('#0055E9')
    expect(icon.paths.map(p => p.fill)).toEqual(['#A8DFF5', '#0055E9', '#00BCFF', '#ECECEE'])
  })

  it('renders meta (llama) as the 15-path multi-gradient mark', () => {
    const icon = vendorIcon('llama')!
    expect(icon.variant).toBe('subtle')
    expect(icon.paths).toHaveLength(15)
    expect(icon.gradients).toHaveLength(13)
    // Every gradient reference resolves to a declared gradient id.
    const ids = new Set(icon.gradients!.map(g => g.id))
    for (const p of icon.paths) {
      const m = p.fill.match(/^url\(#(.+)\)$/)
      if (m) expect(ids.has(m[1]!)).toBe(true)
    }
  })

  it('vendorTileBackground maps the variant to the tile ground', () => {
    expect(vendorTileBackground(vendorIcon('claude')!)).toBe('#D97757')
    expect(vendorTileBackground(vendorIcon('glm')!)).toBe('var(--hs-bg-subtle)')
    expect(vendorTileBackground(vendorIcon('hunyuan')!)).toBe('transparent')
  })

  it('keeps every gradient id unique across the whole pack', () => {
    // Same-vendor duplicates across rows resolve to the first (identical)
    // def in the document; cross-vendor collisions would corrupt rendering.
    const families = ['glm', 'minimax', 'ernie', 'llama']
    const seen = new Set<string>()
    for (const family of families) {
      for (const g of vendorIcon(family)!.gradients ?? []) {
        expect(seen.has(g.id)).toBe(false)
        seen.add(g.id)
      }
    }
  })
})
