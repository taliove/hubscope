import { describe, expect, it } from 'vitest'
import { vendorIcon } from '@/utils/vendorIcon'

describe('vendorIcon', () => {
  it('resolves the classifier canonical family values', () => {
    // internal/classifier canonical families: claude / gpt / gemini / qwen / doubao
    expect(vendorIcon('claude')?.color).toBe('#D97757')
    expect(vendorIcon('gpt')?.color).toBe('currentColor')
    expect(vendorIcon('gemini')?.color).toBe('#4E86F5')
    expect(vendorIcon('qwen')?.color).toBe('#FF6A00')
    expect(vendorIcon('doubao')?.color).toBe('#3C8CFF')
  })

  it('resolves vendor-name aliases to the same icons', () => {
    expect(vendorIcon('anthropic')).toEqual(vendorIcon('claude'))
    expect(vendorIcon('openai')).toEqual(vendorIcon('gpt'))
    expect(vendorIcon('google')).toEqual(vendorIcon('gemini'))
    expect(vendorIcon('googlegemini')).toEqual(vendorIcon('gemini'))
    expect(vendorIcon('alibaba')).toEqual(vendorIcon('qwen'))
    expect(vendorIcon('alibabacloud')).toEqual(vendorIcon('qwen'))
    expect(vendorIcon('bytedance')).toEqual(vendorIcon('doubao'))
  })

  it('normalizes case and whitespace', () => {
    expect(vendorIcon(' Claude ')).toEqual(vendorIcon('claude'))
    expect(vendorIcon('GPT')).toEqual(vendorIcon('gpt'))
    expect(vendorIcon('Gemini')).toEqual(vendorIcon('gemini'))
  })

  it('returns null for unknown vendors (initials-chip fallback)', () => {
    expect(vendorIcon('deepseek')).toBeNull()
    expect(vendorIcon('glm')).toBeNull()
    expect(vendorIcon('llama')).toBeNull()
    expect(vendorIcon('kimi')).toBeNull()
    expect(vendorIcon('other')).toBeNull()
    expect(vendorIcon('')).toBeNull()
    expect(vendorIcon('   ')).toBeNull()
  })

  it('always carries a non-empty 24x24 simple-icons path', () => {
    for (const family of ['claude', 'gpt', 'gemini', 'qwen', 'doubao']) {
      const icon = vendorIcon(family)
      expect(icon).not.toBeNull()
      expect(icon!.path.length).toBeGreaterThan(0)
      expect(icon!.color.length).toBeGreaterThan(0)
    }
  })
})
