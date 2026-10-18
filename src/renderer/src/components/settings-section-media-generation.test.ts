import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { SettingsSidebar } from './SettingsSidebar'

const labels: Record<string, string> = {
  general: 'General',
  imageGen: 'Image generation',
  mediaGeneration: 'Media generation',
  speechToText: 'Speech to text'
}

describe('retired media generation navigation', () => {
  it('removes unsupported media generation while keeping image generation and transcription', () => {
    const html = renderToStaticMarkup(createElement(SettingsSidebar, {
      category: 'general',
      goBack: () => undefined,
      setCategory: () => undefined,
      t: key => labels[key] ?? key
    }))

    expect(html).not.toContain('Media generation')
    expect(html).toContain('Image generation')
    expect(html).toContain('Speech to text')
    expect(html).toContain('General')
  })
})
