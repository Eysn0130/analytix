// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { renderToStaticMarkup } from 'react-dom/server'
import type { ModelProviderProfileV1 } from '@shared/app-settings'
import { ProviderModelsManager } from './settings-section-provider-models'

const labels: Record<string, string> = {
  providerModelListDesc: 'Models list description',
  providerModelEmpty: 'No models yet',
  providerModelAdd: 'Add model',
  providerModelSave: 'Apply to draft',
  providerModelKindMedia: 'Media models',
  providerModelEditAction: 'Edit {{model}}',
  modelProviderVisionBadge: 'Vision',
  providerModelReasoningBadge: 'Reasoning',
  providerModelNoToolsBadge: 'No tool calling',
  providerModelDefaultProfileBadge: 'Default profile',
  providerModelContextBadge: '{{size}} context',
  providerModelKindChat: 'Text chat',
  providerModelKindImage: 'Image generation',
  providerModelKindSpeech: 'Speech to text',
  providerModelKindTts: 'Text to speech',
  providerModelKindMusic: 'Music generation',
  providerModelKindVideo: 'Video generation'
}

function t(key: string, params?: Record<string, unknown>): string {
  const template = labels[key] ?? key
  return template.replace(/\{\{(\w+)\}\}/g, (_, name: string) => String(params?.[name] ?? ''))
}

function provider(overrides: Partial<ModelProviderProfileV1> = {}): ModelProviderProfileV1 {
  return {
    id: 'custom-provider-1',
    name: 'Custom',
    baseUrl: 'https://api.example.com/v1',
    endpointFormat: 'chat_completions',
    models: [],
    modelProfiles: {},
    ...overrides
  }
}

function renderManager(target: ModelProviderProfileV1): string {
  return renderToStaticMarkup(createElement(ProviderModelsManager, {
    provider: target,
    t,
    selectControlClass: 'select',
    onChange: () => undefined
  }))
}

let mountedRoot: Root | null = null
let mountedHost: HTMLDivElement | null = null
const previousActEnvironment = Reflect.get(globalThis, 'IS_REACT_ACT_ENVIRONMENT')

afterEach(async () => {
  if (mountedRoot) await act(async () => { mountedRoot!.unmount() })
  mountedRoot = null
  mountedHost?.remove()
  mountedHost = null
  Reflect.set(globalThis, 'IS_REACT_ACT_ENVIRONMENT', previousActEnvironment)
})

async function mountManager(target: ModelProviderProfileV1, unclassifiedMediaModelIds: readonly string[] = []) {
  Reflect.set(globalThis, 'IS_REACT_ACT_ENVIRONMENT', true)
  mountedHost = document.createElement('div')
  document.body.append(mountedHost)
  mountedRoot = createRoot(mountedHost)
  let current = target
  const changes: ModelProviderProfileV1[] = []
  const render = () => mountedRoot!.render(createElement(ProviderModelsManager, {
    provider: current, t, selectControlClass: 'select', unclassifiedMediaModelIds,
    onChange: (next) => { current = next; changes.push(next); render() }
  }))
  await act(async () => { render() })
  const click = async (label: string) => act(async () => {
    const button = [...mountedHost!.querySelectorAll<HTMLButtonElement>('button')].find((node) =>
      node.textContent?.trim().startsWith(label) || node.getAttribute('aria-label') === label)
    expect(button).toBeDefined()
    button!.click()
  })
  const enter = async (placeholder: string, value: string) => act(async () => {
    const input = mountedHost!.querySelector<HTMLInputElement>(`input[placeholder="${placeholder}"]`)
    expect(input).not.toBeNull()
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, value)
    input!.dispatchEvent(new Event('input', { bubbles: true }))
  })
  return { click, enter, changes, readback: () => current, host: mountedHost }
}

describe('ProviderModelsManager', () => {
  it.each(['https://api.deepseek.com', 'http://127.0.0.1:5500', 'https://gateway.test'])('shows a stable model name and editable API ID at %s', (baseUrl) => {
    const html = renderManager(provider({
      baseUrl, models: ['deepseek-v4-flash']
    }))
    expect(html).toContain('>V4.1 Flash</span>')
    expect(html).toContain('>deepseek-v4-flash</span>')
  })
  it('renders the empty state with an add button', () => {
    const html = renderManager(provider())
    expect(html).toContain('No models yet')
    expect(html).toContain('Add model')
  })

  it('renders capability badges from the model profile', () => {
    const html = renderManager(provider({
      models: ['vision-thinker', 'bare-model'],
      modelProfiles: {
        'vision-thinker': {
          contextWindowTokens: 1_000_000,
          inputModalities: ['text', 'image'],
          outputModalities: ['text'],
          supportsToolCalling: false,
          messageParts: ['text', 'image_url'],
          reasoning: {
            supportedEfforts: ['off', 'high'],
            defaultEffort: 'high',
            requestProtocol: 'deepseek-chat-completions'
          }
        }
      }
    }))
    expect(html).toContain('vision-thinker')
    expect(html).toContain('1M context')
    expect(html).toContain('Vision')
    expect(html).toContain('Reasoning')
    expect(html).toContain('No tool calling')
    expect(html).toContain('Default profile')
  })

  it('keeps model names on a separate row from compact capability badges', () => {
    const html = renderManager(provider({
      models: ['deepseek-v4-pro'],
      modelProfiles: {
        'deepseek-v4-pro': {
          contextWindowTokens: 1_000_000,
          inputModalities: ['text', 'image'],
          outputModalities: ['text'],
          supportsToolCalling: true,
          messageParts: ['text', 'image_url'],
          reasoning: {
            supportedEfforts: ['off', 'high'],
            defaultEffort: 'high',
            requestProtocol: 'deepseek-chat-completions'
          }
        }
      }
    }))

    expect(html).toContain('grid min-w-0 flex-1 gap-1.5')
    expect(html).toContain('flex min-w-0 flex-wrap items-center gap-1')
    expect(html).toContain('text-[10.5px]')
  })

  it('exposes the complete model name on hover for truncated rows', () => {
    const longModelId = 'MiniMax-Text-01-very-long-model-name-with-extra-tags-and-context'
    const html = renderManager(provider({ models: [longModelId] }))

    expect(html).toContain(`title="${longModelId}"`)
    expect(html).toContain('group-hover/model-name:opacity-100')
  })

  it('renders image and speech capability models in the unified list', () => {
    const html = renderManager(provider({
      models: ['chat-model'],
      image: { protocol: 'openai-images', baseUrl: 'https://api.example.com/v1', models: ['image-01'] },
      speech: { protocol: 'mimo-asr', baseUrl: 'https://api.example.com/v1', models: ['mimo-v2.5-asr'] }
    }))

    expect(html).toContain('chat-model')
    expect(html).toContain('Text chat')
    expect(html).toContain('image-01')
    expect(html).toContain('Image generation')
    expect(html).toContain('mimo-v2.5-asr')
    expect(html).toContain('Speech to text')
  })

  it('renders media generation capability models in the unified list', () => {
    const html = renderManager(provider({
      textToSpeech: { protocol: 'mimo-tts', baseUrl: 'https://api.example.com/v1', models: ['mimo-v2.5-tts'] },
      music: { protocol: 'minimax-music', baseUrl: 'https://api.example.com/v1', models: ['music-2.6'] },
      video: { protocol: 'minimax-video', baseUrl: 'https://api.example.com/v1', models: ['MiniMax-Hailuo-2.3'] }
    }))

    expect(html).toContain('mimo-v2.5-tts')
    expect(html).toContain('Text to speech')
    expect(html).toContain('music-2.6')
    expect(html).toContain('Music generation')
    expect(html).toContain('MiniMax-Hailuo-2.3')
    expect(html).toContain('Video generation')
  })

  it('keeps catalog capabilities read-only while applying aliases to a model draft', async () => {
    const metadata = {
      contextWindowTokens: 1_000_000,
      inputModalities: ['text', 'image'] as Array<'text' | 'image'>,
      outputModalities: ['text', 'image'] as Array<'text' | 'image'>,
      supportsToolCalling: false,
      messageParts: ['text', 'image_url'] as Array<'text' | 'image_url'>,
      endpointFormat: 'responses' as const
    }
    const fixture = await mountManager(provider({ models: ['vision-model'], modelProfiles: { 'vision-model': metadata } }))
    await fixture.click('Edit vision-model')
    for (const key of ['providerModelContextLabel', 'providerModelVisionLabel', 'providerModelToolsLabel', 'providerModelEndpointFormatLabel']) {
      expect(fixture.host.textContent).not.toContain(key)
    }
    expect(fixture.host.textContent).toContain('1M context')
    expect(fixture.host.textContent).toContain('Vision')
    expect(fixture.host.textContent).toContain('No tool calling')
    await fixture.enter('providerModelAliasesPlaceholder', 'vision-alias')
    await fixture.click('Apply to draft')
    expect(fixture.changes).toHaveLength(1)
    expect(fixture.readback().modelProfiles['vision-model']).toMatchObject({ ...metadata, aliases: ['vision-alias'] })
  })

  it('adds a text model without inventing context or vision metadata', async () => {
    const fixture = await mountManager(provider())
    await fixture.click('Add model')
    await fixture.enter('providerModelIdPlaceholder', 'text-new')
    await fixture.click('Apply to draft')
    expect(fixture.readback().models).toEqual(['text-new'])
    expect(fixture.readback().modelProfiles).not.toHaveProperty('text-new')
  })

  it('adds generic media without losing catalog lists and keeps existing media edits in their own kind', async () => {
    const baseUrl = 'https://api.example.com/v1'
    const fixture = await mountManager(provider({
      image: { protocol: 'openai-images', baseUrl, models: ['image-old'] },
      speech: { protocol: 'mimo-asr', baseUrl, models: ['speech-old'] },
      textToSpeech: { protocol: 'mimo-tts', baseUrl, models: ['tts-old'] },
      music: { protocol: 'minimax-music', baseUrl, models: ['music-old'] },
      video: { protocol: 'minimax-video', baseUrl, models: ['video-old'] }
    }), ['media-new'])
    await fixture.click('Add model')
    await fixture.click('Media models')
    expect(fixture.host.querySelector('[aria-pressed="false"]')?.textContent).toContain('Text chat')
    await fixture.enter('providerModelIdPlaceholder', 'media-new')
    await fixture.click('Apply to draft')
    expect(fixture.readback()).toMatchObject({
      image: { models: ['image-old', 'media-new'] }, speech: { models: ['speech-old'] },
      textToSpeech: { models: ['tts-old'] }, music: { models: ['music-old'] }, video: { models: ['video-old'] }
    })
    const genericRow = [...fixture.host.querySelectorAll('li')].find((node) => node.textContent?.includes('media-new'))!
    expect(genericRow.textContent).toContain('Media models')
    expect(genericRow.textContent).not.toContain('Image generation')
    await fixture.click('Edit speech-old')
    await fixture.enter('providerModelIdPlaceholder', 'speech-new')
    await fixture.click('Apply to draft')
    expect(fixture.readback().speech?.models).toEqual(['speech-new'])
    expect(fixture.readback().image?.models).toEqual(['image-old', 'media-new'])
  })
  it('gates only native Messages custom controls and preserves hidden metadata when its model ID changes', async () => {
    const original = { inputModalities: ['text'] as const, outputModalities: ['text'] as const,
      messageParts: ['text'] as const, supportsToolCalling: true, aliases: ['retained-alias'],
      reasoning: { supportedEfforts: [], defaultEffort: 'high' as const, requestProtocol: 'deepseek-chat-completions' as const } }
    const fixture = await mountManager(provider({ registryKind: 'deepseek-messages', endpointFormat: 'messages',
      models: ['native-old'], modelProfiles: { 'native-old': { ...original,
        inputModalities: [...original.inputModalities], outputModalities: [...original.outputModalities], messageParts: [...original.messageParts] } } }))
    await fixture.click('Edit native-old')
    expect(fixture.host.textContent).toContain('providerModelNativeMessagesManaged')
    expect(fixture.host.querySelector('input[placeholder="providerModelAliasesPlaceholder"]')).toBeNull()
    expect(fixture.host.textContent).not.toContain('providerModelReasoningLabel')
    await fixture.enter('providerModelIdPlaceholder', 'native-new')
    await fixture.click('Apply to draft')
    expect(fixture.readback().models).toEqual(['native-new'])
    expect(fixture.readback().modelProfiles['native-new']).toEqual(original)
  })

  it('retains real custom reasoning and alias controls for ordinary Messages providers', async () => {
    const fixture = await mountManager(provider({ endpointFormat: 'messages', models: ['ordinary'] }))
    await fixture.click('Edit ordinary')
    expect(fixture.host.querySelector('input[placeholder="providerModelAliasesPlaceholder"]')).not.toBeNull()
    expect(fixture.host.textContent).toContain('providerModelReasoningLabel')
    expect(fixture.host.textContent).not.toContain('providerModelNativeMessagesManaged')
  })

})
