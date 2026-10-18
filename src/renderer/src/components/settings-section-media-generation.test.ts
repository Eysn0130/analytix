import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { SettingsSidebar } from './SettingsSidebar'

const labels: Record<string, string> = {
  general: 'General',
  providers: 'Providers',
  settingsTabGeneral: 'General',
  settingsTabModels: 'Models',
  settingsTabWorkspace: 'Workspace',
  settingsTabTools: 'Tools & permissions',
  settingsSectionLabel: 'Settings section',
  imageGen: 'Image generation',
  mediaGeneration: 'Media generation',
  speechToText: 'Speech to text'
}

describe('retired media generation navigation', () => {
  it('removes unsupported media generation while keeping image generation and transcription', () => {
    const navigation = (category: 'general' | 'providers' | 'imageGeneration' | 'speechToText' | 'write' | 'agents') => renderToStaticMarkup(createElement(SettingsSidebar, {
      category,
      goBack: () => undefined,
      setCategory: () => undefined,
      t: key => labels[key] ?? key
    }))

    for (const category of ['general', 'providers', 'write', 'agents'] as const) {
      const html = navigation(category)
      expect(html).not.toContain('Media generation')
      expect(html).not.toContain('value="mediaGeneration"')
    }
    expect(navigation('general').match(/<option[^>]*selected=""[^>]*>([^<]*)<\/option>/)?.[1]).toBe('General')
    for (const [category, label] of [['imageGeneration', 'Image generation'], ['speechToText', 'Speech to text']] as const) {
      const html = navigation(category)
      expect(html.match(/<button[^>]*aria-selected="true"[^>]*>([^<]*)<\/button>/)?.[1]).toBe('Models')
      expect([...html.matchAll(/<option value="([^"]+)"/g)].map(match => match[1])).toEqual([
        'providers', 'imageGeneration', 'speechToText'
      ])
      expect(html.match(/<option[^>]*selected=""[^>]*>([^<]*)<\/option>/)?.[1]).toBe(label)
    }
  })
})
