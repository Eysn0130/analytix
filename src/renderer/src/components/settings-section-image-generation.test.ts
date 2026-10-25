import { describe, expect, it } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { defaultAnalytixRuntimeSettings } from '@shared/app-settings'
import { SettingsSidebar } from './SettingsSidebar'
import { ImageGenerationSettingsSection } from './settings-section-image-generation'

const labels: Record<string, string> = {
  back: 'Back',
  general: 'General',
  providers: 'Providers',
  speechToText: 'Speech to text',
  settingsTabGeneral: 'General',
  settingsTabModels: 'Models',
  settingsTabWorkspace: 'Workspace',
  settingsTabTools: 'Tools & permissions',
  settingsSectionLabel: 'Settings section',
  write: 'Write',
  agents: 'AI assistant',
  keyboardShortcuts: 'Keyboard shortcuts',
  claw: 'Connect phone',
  settingsFooter: 'Settings',
  imageGen: 'Image generation',
  imageGenEnabled: 'Enable image generation',
  imageGenEnabledDesc: 'Enables agent chats and Write infographics',
  imageGenBaseUrl: 'API base URL',
  imageGenBaseUrlDesc: 'OpenAI-compatible endpoint root',
  imageGenBaseUrlPlaceholder: 'https://api.example.com/v1',
  imageGenApiKey: 'API key',
  imageGenApiKeyDesc: 'Independent image provider key',
  imageGenModel: 'Image model',
  imageGenModelDesc: 'Model id sent to the provider',
  imageGenModelQualityHint: 'Prefer GPT Image or Gemini image models for design drafts and infographics',
  imageGenModelPlaceholder: 'gpt-image-1',
  imageGenDefaultSize: 'Default size',
  imageGenDefaultSizeDesc: 'Default size description',
  imageGenTimeout: 'Timeout (ms)',
  imageGenTimeoutDesc: 'Timeout description',
  showSecret: 'Show',
  hideSecret: 'Hide'
}

function t(key: string): string {
  return labels[key] ?? key
}

describe('ImageGenerationSettingsSection', () => {
  it('renders image generation as a standalone shared settings section', () => {
    const legacyCredentialCanary = 'legacy-untrusted-image-credential-canary'
    const html = renderToStaticMarkup(createElement(ImageGenerationSettingsSection, {
      ctx: {
        t,
        analytix: {
          ...defaultAnalytixRuntimeSettings(),
          imageGeneration: {
            enabled: true,
            baseUrl: 'https://images.example.com/v1',
            apiKey: legacyCredentialCanary,
            model: 'image-model',
            defaultSize: '1536x1024',
            timeoutMs: 240000
          }
        },
        updateAnalytix: () => undefined
      }
    }))

    expect(html).toContain('Image generation')
    expect(html).toContain('Enables agent chats and Write infographics')
    expect(html).toContain('Prefer GPT Image or Gemini image models for design drafts and infographics')
    expect(html).toContain('value="https://images.example.com/v1"')
    expect(html).not.toContain(legacyCredentialCanary)
    expect(html).toContain('value="image-model"')
    expect(html).toContain('value="1536x1024"')
    expect(html).toContain('value="240000"')
  })

  it('selects image generation alongside Providers and transcription in Models', () => {
    const html = renderToStaticMarkup(createElement(SettingsSidebar, {
      category: 'imageGeneration',
      goBack: () => undefined,
      setCategory: () => undefined,
      t
    }))

    expect(html.match(/<button[^>]*aria-selected="true"[^>]*>([^<]*)<\/button>/)?.[1]).toBe('Models')
    expect([...html.matchAll(/<option value="([^"]+)"/g)].map(match => match[1])).toEqual([
      'providers', 'imageGeneration', 'speechToText'
    ])
    expect(html.match(/<option[^>]*selected=""[^>]*>([^<]*)<\/option>/)?.[1]).toBe('Image generation')
    expect(html).toContain('Providers')
    expect(html).toContain('Speech to text')
  })
})
