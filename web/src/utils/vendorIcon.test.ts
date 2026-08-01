import { describe, expect, it } from 'vitest'
import { vendorIcon } from '@/utils/vendorIcon'

describe('vendorIcon', () => {
  it('resolves the classifier canonical family values to their brand tiles', () => {
    // internal/classifier canonical families: claude / gpt / gemini / qwen /
    // doubao (+ kimi / moonshot, GH #136)
    expect(vendorIcon('claude')?.tile).toBe('#D97757')
    expect(vendorIcon('gpt')?.tile).toBe('#000')
    expect(vendorIcon('gemini')?.tile).toBe('#4E86F5')
    expect(vendorIcon('qwen')?.tile).toBe('#FF6A00')
    expect(vendorIcon('doubao')?.tile).toBe('#3C8CFF')
    expect(vendorIcon('kimi')?.tile).toBe('#000')
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
  })

  it('normalizes case and whitespace', () => {
    expect(vendorIcon(' Claude ')).toEqual(vendorIcon('claude'))
    expect(vendorIcon('GPT')).toEqual(vendorIcon('gpt'))
    expect(vendorIcon('Gemini')).toEqual(vendorIcon('gemini'))
    expect(vendorIcon('Kimi')).toEqual(vendorIcon('kimi'))
  })

  it('returns null for unknown vendors (initials-chip fallback)', () => {
    expect(vendorIcon('deepseek')).toBeNull()
    expect(vendorIcon('glm')).toBeNull()
    expect(vendorIcon('llama')).toBeNull()
    expect(vendorIcon('other')).toBeNull()
    expect(vendorIcon('')).toBeNull()
    expect(vendorIcon('   ')).toBeNull()
  })

  it('always carries a brand tile and at least one 24x24 glyph path', () => {
    for (const family of ['claude', 'gpt', 'gemini', 'qwen', 'doubao', 'kimi']) {
      const icon = vendorIcon(family)
      expect(icon).not.toBeNull()
      expect(icon!.tile.length).toBeGreaterThan(0)
      expect(icon!.paths.length).toBeGreaterThan(0)
      for (const p of icon!.paths) {
        expect(p.d.length).toBeGreaterThan(0)
        expect(p.fill.length).toBeGreaterThan(0)
      }
    }
  })

  it('renders single-color marks as one white glyph path over the brand tile', () => {
    // The uniform-tile discipline (GH #136): brand ground + white glyph.
    // openai is no longer currentColor — the solid tile solves the dark-theme
    // invisibility that motivated the currentColor exception.
    for (const family of ['claude', 'gpt', 'gemini', 'qwen', 'doubao']) {
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
})
