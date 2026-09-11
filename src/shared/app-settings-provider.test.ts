import { describe, expect, it } from 'vitest'
import {
  DEFAULT_DEEPSEEK_BASE_URL,
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultAnalytixRuntimeSettings,
  defaultMiniMaxMediaGenerationAnalytixPatch,
  defaultModelProviderSettings,
  getModelProviderIdForModel,
  getModelProviderPreset,
  isComposerChatModelId,
  isImageGenerationModelId,
  isMusicGenerationModelId,
  isSpeechToTextModelId,
  isTextToSpeechModelId,
  isVideoGenerationModelId,
  modelProviderPresetProfile,
  modelProviderTokenPlanProfile,
  defaultScheduleSettings,
  defaultWriteSettings,
  listMusicGenerationProviderProfiles,
  listSpeechToTextProviderProfiles,
  listTextToSpeechProviderProfiles,
  listVideoGenerationProviderProfiles,
  listModelProviderModelIds,
  modelSupportsImageInput,
  normalizeModelProviderModelIdForRequest,
  normalizeModelProviderSettings,
  analytixRuntimeSettingsEqual,
  buildAnalytixRuntimeSettingsKey,
  resolveAnalytixImageGenerationSettings,
  resolveAnalytixMusicGenerationSettings,
  resolveModelProviderBaseUrl,
  resolveAnalytixRuntimeSettings,
  resolveAnalytixSpeechToTextSettings,
  resolveAnalytixTextToSpeechSettings,
  resolveAnalytixVideoGenerationSettings,
  modelCapabilityProbeKey,
  resolveModelProviderEndpointFormat,
  type AppSettingsV1
} from './app-settings'

function settings(): AppSettingsV1 {
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    provider: {
      ...defaultModelProviderSettings(),
      providers: [
        ...defaultModelProviderSettings().providers,
        {
          id: 'custom',
          name: 'Custom Provider',
          baseUrl: 'https://custom.example/v1',
          endpointFormat: 'messages',
          models: ['custom-model'],
          modelProfiles: {}
        }
      ]
    },
    runtime: {
        ...defaultAnalytixRuntimeSettings(),
        providerId: 'custom',
        model: 'custom-model'
      },
    workspaceRoot: '/tmp/workspace',
    log: { enabled: false, retentionDays: 7 },
    notifications: { turnComplete: true },
    appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
    keyboardShortcuts: defaultKeyboardShortcuts(),
    write: defaultWriteSettings(),
    claw: defaultClawSettings(),
    schedule: defaultScheduleSettings(),
    guiUpdate: { channel: 'stable' },
    codePromptPrefix: '',
    disabledSkillIds: []
  }
}

describe('model provider settings', () => {
  it('keeps Vision Bridge disabled by default', () => {
    expect(defaultAnalytixRuntimeSettings().visionBridge.enabled).toBe(false)
  })

  it('resolves key-free Analytix runtime metadata from the selected provider', () => {
    const state = settings()
    state.runtime.baseUrl = 'https://stale-runtime.example/v1'
    const runtime = resolveAnalytixRuntimeSettings(state)

    expect(Object.hasOwn(runtime, 'apiKey')).toBe(false)
    expect(runtime.baseUrl).toBe('https://custom.example/v1')
    expect(runtime.endpointFormat).toBe('messages')
  })

  it('resolves DeepSeek provider metadata when runtime fields are intentionally blank', () => {
    const state = settings()
    state.provider = {
      activeProviderId: 'deepseek',
      baseUrl: 'https://api.deepseek.com',
      proxy: { enabled: false, url: '' },
      providers: [
        {
          id: 'deepseek',
          name: 'DeepSeek',
          baseUrl: 'https://api.deepseek.com',
          endpointFormat: 'chat_completions',
          models: ['deepseek-v4-pro'],
          modelProfiles: {}
        }
      ]
    }
    state.runtime.providerId = 'deepseek'
    state.runtime.baseUrl = ''
    state.runtime.model = 'deepseek-v4-pro'
    const runtime = resolveAnalytixRuntimeSettings(state)

    expect(Object.hasOwn(runtime, 'apiKey')).toBe(false)
    expect(runtime.baseUrl).toBe('https://api.deepseek.com')
    expect(runtime.endpointFormat).toBe('chat_completions')
  })

  it('repairs DeepSeek runtime requests that point at a MiMo model', () => {
    const state = settings()
    state.provider = defaultModelProviderSettings()
    state.runtime.providerId = 'deepseek'
    state.runtime.model = 'mimo-v2-pro'

    const runtime = resolveAnalytixRuntimeSettings(state)

    expect(runtime.providerId).toBe('deepseek')
    expect(runtime.model).toBe('deepseek-v4-flash')
    expect(normalizeModelProviderModelIdForRequest(state, 'mimo-v2-pro', 'deepseek')).toBe('deepseek-v4-flash')
  })

  it('preserves provider and model pricing for Go runtime usage cost', () => {
    const normalized = normalizeModelProviderSettings({
      providers: [{
        id: 'priced',
        name: 'Priced Provider',
        baseUrl: 'https://priced.example/v1',
        endpointFormat: 'chat_completions',
        models: ['priced-default', 'priced-special'],
        price: { input: 3, output: 9, currency: 'USD' },
        prices: {
          'priced-special': { cacheHit: 1, input: 4, output: 12, currency: 'USD' },
          'unknown-model': { input: 99, output: 99, currency: 'USD' }
        },
        modelProfiles: {
          'priced-special': {
            inputModalities: ['text'],
            outputModalities: ['text'],
            supportsToolCalling: true,
            messageParts: ['text'],
            price: { cacheHit: 0.5, input: 2, output: 8, currency: 'CNY' }
          }
        }
      }]
    })
    const provider = normalized.providers.find((item) => item.id === 'priced')

    expect(provider?.price).toEqual({ input: 3, output: 9, currency: 'USD' })
    expect(provider?.prices).toEqual({
      'priced-special': { cacheHit: 1, input: 4, output: 12, currency: 'USD' }
    })
    expect(provider?.modelProfiles['priced-special']?.price).toEqual({
      cacheHit: 0.5,
      input: 2,
      output: 8,
      currency: 'CNY'
    })
  })

  it('strips DeepSeek provider credentials from legacy provider input', () => {
    const canary = ['legacy', 'provider', 'credential'].join('-')
    const provider = normalizeModelProviderSettings({
      apiKey: '',
      baseUrl: '',
      providers: [
        {
          id: 'deepseek',
          name: 'DeepSeek',
          apiKey: canary,
          baseUrl: 'https://api.deepseek.com',
          endpointFormat: 'chat_completions',
          models: ['deepseek-v4-pro'],
          modelProfiles: {}
        }
      ]
    } as never)

    const deepseek = provider.providers.find((profile) => profile.id === 'deepseek')
    expect(deepseek?.baseUrl).toBe('https://api.deepseek.com')
    expect(JSON.stringify(provider).includes(canary)).toBe(false)
    expect(JSON.stringify(provider).includes('apiKey')).toBe(false)
  })

  it('resolves runtime credentials from provider.activeProviderId when runtime providerId is blank', () => {
    const state = settings()
    state.provider = {
      ...state.provider,
      activeProviderId: 'custom'
    }
    state.runtime.providerId = ''
    state.runtime.baseUrl = ''
    const runtime = resolveAnalytixRuntimeSettings(state)

    expect(runtime.providerId).toBe('custom')
    expect(Object.hasOwn(runtime, 'apiKey')).toBe(false)
    expect(runtime.baseUrl).toBe('https://custom.example/v1')
    expect(runtime.endpointFormat).toBe('messages')
  })

  it('lets runtime providerId override provider.activeProviderId for explicit routes', () => {
    const state = settings()
    state.provider = {
      ...state.provider,
      activeProviderId: 'custom'
    }
    state.runtime.providerId = 'deepseek'
    state.runtime.model = 'deepseek-v4-pro'
    const runtime = resolveAnalytixRuntimeSettings(state)

    expect(runtime.providerId).toBe('deepseek')
    expect(runtime.endpointFormat).toBe('chat_completions')
  })

  it('keys selected provider currentness into managed runtime rebuild decisions', () => {
    const base = settings()
    const uiOnly = settings()
    uiOnly.theme = 'dark'
    expect(analytixRuntimeSettingsEqual(base, uiOnly)).toBe(true)

    const endpointDrift = settings()
    endpointDrift.provider.providers = endpointDrift.provider.providers.map((provider) =>
      provider.id === 'custom'
        ? {
            ...provider,
            baseUrl: 'https://custom.example/responses',
            endpointFormat: 'responses' as const
          }
        : provider
    )
    expect(analytixRuntimeSettingsEqual(base, endpointDrift)).toBe(false)
    expect(buildAnalytixRuntimeSettingsKey(endpointDrift))
      .not.toBe(buildAnalytixRuntimeSettingsKey(base))

    const profileDrift = settings()
    profileDrift.provider.providers = profileDrift.provider.providers.map((provider) =>
      provider.id === 'custom'
        ? {
            ...provider,
            modelProfiles: {
              'custom-model': {
                contextWindowTokens: 256_000,
                inputModalities: ['text'],
                outputModalities: ['text'],
                supportsToolCalling: true,
                messageParts: ['text'],
                reasoning: {
                  supportedEfforts: ['off', 'high'],
                  defaultEffort: 'high',
                  requestProtocol: 'openai-responses'
                }
              }
            }
          }
        : provider
    )
    expect(analytixRuntimeSettingsEqual(base, profileDrift)).toBe(false)
  })

  it('uses the Kun 0.2.14 default context window for unknown provider models', () => {
    const state = settings()
    state.provider.providers = state.provider.providers.map((provider) =>
      provider.id === 'custom'
        ? {
            ...provider,
            modelProfiles: {
              'custom-model': {
                inputModalities: ['text'],
                outputModalities: ['text'],
                supportsToolCalling: true,
                messageParts: ['text']
              }
            }
          }
        : provider
    )

    expect(resolveAnalytixRuntimeSettings(state).modelProfiles['custom-model'])
      .toEqual(expect.objectContaining({ contextWindowTokens: 128_000 }))

    const explicit = normalizeModelProviderSettings({
      ...state.provider,
      providers: state.provider.providers.map((provider) =>
        provider.id === 'custom'
          ? {
              ...provider,
              modelProfiles: {
                'custom-model': { contextWindowTokens: 96_000 }
              }
            }
          : provider
      )
    })
    const custom = explicit.providers.find((provider) => provider.id === 'custom')
    expect(custom?.modelProfiles['custom-model']).toEqual(expect.objectContaining({
      contextWindowTokens: 96_000
    }))
  })

  it('strips legacy Analytix runtime credential overrides', () => {
    const state = settings()
    state.runtime.providerId = ''
    const canary = ['legacy', 'runtime', 'credential'].join('-')
    ;(state.runtime as unknown as Record<string, unknown>).apiKey = canary
    state.runtime.baseUrl = 'https://legacy-runtime.example/v1'
    const runtime = resolveAnalytixRuntimeSettings(state)

    expect(runtime.baseUrl).toBe('https://legacy-runtime.example/v1')
    expect(JSON.stringify(runtime).includes(canary)).toBe(false)
    expect(Object.hasOwn(runtime, 'apiKey')).toBe(false)
  })

  it('does not reintroduce a legacy runtime credential for a selected provider', () => {
    const state = settings()
    state.runtime.providerId = 'custom'
    const canary = ['runtime', 'fallback', 'credential'].join('-')
    ;(state.runtime as unknown as Record<string, unknown>).apiKey = canary
    const runtime = resolveAnalytixRuntimeSettings(state)

    expect(runtime.providerId).toBe('custom')
    expect(JSON.stringify(runtime).includes(canary)).toBe(false)
    expect(Object.hasOwn(runtime, 'apiKey')).toBe(false)
  })

  it('creates Xiaomi and MiniMax provider presets for Analytix runtime profiles', () => {
    const xiaomi = getModelProviderPreset('xiaomi')
    const minimax = getModelProviderPreset('minimax')

    expect(xiaomi && modelProviderPresetProfile(xiaomi)).toMatchObject({
      id: 'xiaomi',
      name: 'Xiaomi',
      baseUrl: 'https://api.xiaomimimo.com/v1',
      endpointFormat: 'chat_completions',
      models: expect.arrayContaining(['mimo-v2.5-pro']),
      modelProfiles: {
        'mimo-v2.5': expect.objectContaining({
          inputModalities: expect.arrayContaining(['image']),
          messageParts: expect.arrayContaining(['image_url']),
          reasoning: expect.objectContaining({
            supportedEfforts: ['off', 'low', 'medium', 'high'],
            defaultEffort: 'high',
            requestProtocol: 'mimo-chat-completions'
          })
        }),
        'mimo-v2-omni': expect.objectContaining({
          inputModalities: expect.arrayContaining(['image'])
        })
      }
    })
    expect(xiaomi && modelProviderPresetProfile(xiaomi).models).not.toContain(['mimo', 'v2', 'flash'].join('-'))
    expect(xiaomi && modelProviderPresetProfile(xiaomi).models).not.toContain('mimo-v2.5-pro-ultraspeed')
    expect(xiaomi && modelProviderPresetProfile(xiaomi).models.slice(0, 3)).toEqual([
      'mimo-v2.5-pro',
      'mimo-v2.5',
      'mimo-v2-pro'
    ])
    expect(modelProviderPresetProfile(xiaomi!).modelProfiles['mimo-v2.5-pro']?.aliases)
      .toContain('mimo-v2.5-pro-ultraspeed')
    expect(minimax && modelProviderPresetProfile(minimax)).toMatchObject({
      id: 'minimax',
      name: 'MiniMax',
      baseUrl: 'https://api.minimaxi.com/anthropic',
      endpointFormat: 'messages',
      models: expect.arrayContaining(['MiniMax-M2.5', 'MiniMax-M3']),
      image: {
        protocol: 'minimax-image',
        baseUrl: 'https://api.minimaxi.com',
        models: ['image-01', 'image-01-live']
      },
      textToSpeech: {
        protocol: 'minimax-t2a',
        baseUrl: 'https://api.minimax.io',
        models: ['speech-2.8-hd', 'speech-2.8-turbo']
      },
      music: {
        protocol: 'minimax-music',
        baseUrl: 'https://api.minimax.io',
        models: ['music-2.6', 'music-cover', 'music-2.6-free', 'music-cover-free']
      },
      video: {
        protocol: 'minimax-video',
        baseUrl: 'https://api.minimax.io',
        models: ['MiniMax-Hailuo-2.3', 'MiniMax-Hailuo-2.3-Fast']
      },
      modelProfiles: {
        'MiniMax-M3': expect.objectContaining({
          inputModalities: expect.arrayContaining(['image']),
          messageParts: expect.arrayContaining(['image_url']),
          reasoning: expect.objectContaining({
            supportedEfforts: ['auto', 'off'],
            defaultEffort: 'auto',
            requestProtocol: 'anthropic-thinking'
          })
        }),
        'MiniMax-M2.5': expect.objectContaining({
          reasoning: expect.objectContaining({
            supportedEfforts: ['auto'],
            defaultEffort: 'auto',
            requestProtocol: 'none'
          })
        })
      }
    })
  })

  it('resolves MiMo chat models back to the active Xiaomi provider profile', () => {
    const base = settings()
    const xiaomi = getModelProviderPreset('xiaomi')
    expect(xiaomi).not.toBeNull()
    const xiaomiProfile = modelProviderPresetProfile(xiaomi!)
    const xiaomiTokenPlanProfile = modelProviderTokenPlanProfile(xiaomi!)
    expect(xiaomiTokenPlanProfile).not.toBeNull()
    expect(xiaomiTokenPlanProfile?.modelProfiles['mimo-v2.5-pro']?.aliases)
      .toContain('mimo-v2.5-pro-ultraspeed')
    const configured = {
      ...base,
      provider: {
        ...base.provider,
        activeProviderId: xiaomiTokenPlanProfile!.id,
        providers: [
          ...base.provider.providers,
          xiaomiProfile,
          xiaomiTokenPlanProfile!
        ]
      }
    }

    expect(getModelProviderIdForModel(configured, 'mimo-v2.5-pro')).toBe('xiaomi-token-plan')
    expect(getModelProviderIdForModel(configured, 'mimo-v2.5-pro-ultraspeed')).toBe('xiaomi-token-plan')
    expect(normalizeModelProviderModelIdForRequest(configured, 'mimo-v2.5-pro-ultraspeed')).toBe('mimo-v2.5-pro')
    expect(getModelProviderIdForModel(configured, 'mimo-v2.5-pro', 'xiaomi')).toBe('xiaomi')

    const runtimeConfigured = {
      ...configured,
      provider: {
        ...configured.provider,
        activeProviderId: ''
      },
      runtime: {
        ...configured.runtime,
        providerId: 'xiaomi',
        model: 'mimo-v2.5-pro-ultraspeed'
      }
    }
    expect(resolveAnalytixRuntimeSettings(runtimeConfigured)).toMatchObject({
      providerId: 'xiaomi',
      model: 'mimo-v2.5-pro',
      endpointFormat: 'chat_completions'
    })
  })

  it('resolves MiniMax preset credentials through the selected provider', () => {
    const minimax = getModelProviderPreset('minimax')
    expect(minimax).not.toBeNull()
    const minimaxProfile = modelProviderPresetProfile(minimax!)
    const resolved = resolveAnalytixRuntimeSettings({
      ...settings(),
      provider: {
        ...defaultModelProviderSettings(),
        providers: [
          ...defaultModelProviderSettings().providers,
          minimaxProfile
        ]
      },
      runtime: {
          ...defaultAnalytixRuntimeSettings(),
          providerId: minimaxProfile.id,
          model: minimaxProfile.models[0]
        }
    })

    expect(resolved).toEqual(expect.objectContaining({
      baseUrl: 'https://api.minimaxi.com/anthropic',
      endpointFormat: 'messages',
      imageGeneration: expect.objectContaining({
        enabled: false,
        protocol: 'openai-images'
      }),
      model: 'MiniMax-M3',
      modelProfiles: expect.objectContaining({
        'minimax-m3': expect.objectContaining({
          inputModalities: expect.arrayContaining(['image'])
        })
      })
    }))
    expect(modelSupportsImageInput(resolved.modelProfiles['minimax-m3'])).toBe(true)
  })

  it('builds default media generation settings for configured MiniMax providers', () => {
    const minimax = getModelProviderPreset('minimax')
    expect(minimax).not.toBeNull()
    const minimaxProfile = modelProviderPresetProfile(minimax!)
    const patch = defaultMiniMaxMediaGenerationAnalytixPatch({
      providers: [
        ...defaultModelProviderSettings().providers,
        minimaxProfile
      ],
      currentAnalytix: defaultAnalytixRuntimeSettings()
    })

    expect(patch).toEqual(expect.objectContaining({
      textToSpeech: expect.objectContaining({
        enabled: true,
        providerId: 'minimax',
        protocol: 'minimax-t2a',
        model: 'speech-2.8-hd'
      }),
      musicGeneration: expect.objectContaining({
        enabled: true,
        providerId: 'minimax',
        protocol: 'minimax-music',
        model: 'music-2.6'
      }),
      videoGeneration: expect.objectContaining({
        enabled: true,
        providerId: 'minimax',
        protocol: 'minimax-video',
        model: 'MiniMax-Hailuo-2.3'
      })
    }))
  })

  it('prefers the active MiniMax token plan profile when backfilling media defaults', () => {
    const minimax = getModelProviderPreset('minimax')
    expect(minimax).not.toBeNull()
    const minimaxProfile = modelProviderPresetProfile(minimax!)
    const tokenPlanProfile = modelProviderTokenPlanProfile(minimax!)
    expect(tokenPlanProfile).not.toBeNull()
    const patch = defaultMiniMaxMediaGenerationAnalytixPatch({
      providers: [
        ...defaultModelProviderSettings().providers,
        minimaxProfile,
        tokenPlanProfile!
      ],
      currentAnalytix: {
        ...defaultAnalytixRuntimeSettings(),
        providerId: tokenPlanProfile!.id
      }
    })

    expect(patch).toEqual(expect.objectContaining({
      textToSpeech: expect.objectContaining({ providerId: 'minimax-token-plan' }),
      musicGeneration: expect.objectContaining({ providerId: 'minimax-token-plan' }),
      videoGeneration: expect.objectContaining({ providerId: 'minimax-token-plan' })
    }))
  })

  it('backfills MiniMax media defaults from presets without overriding explicit settings', () => {
    const staleMiniMax = {
      id: 'minimax',
      name: 'MiniMax',
      baseUrl: 'https://api.minimaxi.com/anthropic',
      endpointFormat: 'messages' as const,
      models: ['MiniMax-M3'],
      modelProfiles: {}
    }
    const patch = defaultMiniMaxMediaGenerationAnalytixPatch({
      providers: [
        ...defaultModelProviderSettings().providers,
        staleMiniMax
      ],
      currentAnalytix: {
        ...defaultAnalytixRuntimeSettings(),
        textToSpeech: {
          ...defaultAnalytixRuntimeSettings().textToSpeech,
          providerId: 'voice-lab'
        }
      },
      analytixPatch: {
        musicGeneration: { enabled: false }
      }
    })

    expect(patch).toEqual({
      videoGeneration: expect.objectContaining({
        enabled: true,
        providerId: 'minimax',
        protocol: 'minimax-video',
        model: 'MiniMax-Hailuo-2.3'
      })
    })
  })

  it('resolves media generation through stale MiniMax preset providers after capability backfill', () => {
    const staleMiniMax = {
      id: 'minimax',
      name: 'MiniMax',
      baseUrl: 'https://api.minimaxi.com/anthropic',
      endpointFormat: 'messages' as const,
      models: ['MiniMax-M3'],
      modelProfiles: {}
    }
    const state = {
      ...settings(),
      provider: {
        ...defaultModelProviderSettings(),
        providers: [
          ...defaultModelProviderSettings().providers,
          staleMiniMax
        ]
      },
      runtime: {
          ...defaultAnalytixRuntimeSettings(),
          textToSpeech: {
            ...defaultAnalytixRuntimeSettings().textToSpeech,
            enabled: true,
            providerId: 'minimax'
          },
          musicGeneration: {
            ...defaultAnalytixRuntimeSettings().musicGeneration,
            enabled: true,
            providerId: 'minimax'
          },
          videoGeneration: {
            ...defaultAnalytixRuntimeSettings().videoGeneration,
            enabled: true,
            providerId: 'minimax'
          }
        }
    }

    expect(listTextToSpeechProviderProfiles(state).map((profile) => profile.id)).toContain('minimax')
    expect(resolveAnalytixTextToSpeechSettings(state)).toEqual(expect.objectContaining({
      baseUrl: 'https://api.minimax.io',
      model: 'speech-2.8-hd'
    }))
    expect(resolveAnalytixMusicGenerationSettings(state)).toEqual(expect.objectContaining({
      baseUrl: 'https://api.minimax.io',
      model: 'music-2.6'
    }))
    expect(resolveAnalytixVideoGenerationSettings(state)).toEqual(expect.objectContaining({
      baseUrl: 'https://api.minimax.io',
      model: 'MiniMax-Hailuo-2.3'
    }))
  })

  it('resolves MiniMax image generation through provider image capability', () => {
    const minimax = getModelProviderPreset('minimax')
    expect(minimax).not.toBeNull()
    const minimaxProfile = modelProviderPresetProfile(minimax!)
    const resolved = resolveAnalytixImageGenerationSettings({
      ...settings(),
      provider: {
        ...defaultModelProviderSettings(),
        providers: [
          ...defaultModelProviderSettings().providers,
          minimaxProfile
        ]
      },
      runtime: {
          ...defaultAnalytixRuntimeSettings(),
          imageGeneration: {
            ...defaultAnalytixRuntimeSettings().imageGeneration,
            enabled: true,
            providerId: minimaxProfile.id,
            baseUrl: 'https://stale-image.example/v1',
            model: 'stale-image-model'
          }
        }
    })

    expect(resolved).toEqual(expect.objectContaining({
      enabled: true,
      providerId: 'minimax',
      protocol: 'minimax-image',
      baseUrl: 'https://api.minimaxi.com',
      model: 'image-01'
    }))
  })

  it('resolves MiniMax token plan image generation through provider image capability', () => {
    const minimax = getModelProviderPreset('minimax')
    expect(minimax).not.toBeNull()
    const minimaxTokenPlanProfile = modelProviderTokenPlanProfile(minimax!)
    expect(minimaxTokenPlanProfile).toMatchObject({
      id: 'minimax-token-plan',
      image: {
        protocol: 'minimax-image',
        baseUrl: 'https://api.minimaxi.com',
        models: ['image-01', 'image-01-live']
      }
    })
    const resolved = resolveAnalytixImageGenerationSettings({
      ...settings(),
      provider: {
        ...defaultModelProviderSettings(),
        providers: [
          ...defaultModelProviderSettings().providers,
          minimaxTokenPlanProfile!
        ]
      },
      runtime: {
          ...defaultAnalytixRuntimeSettings(),
          imageGeneration: {
            ...defaultAnalytixRuntimeSettings().imageGeneration,
            enabled: true,
            providerId: minimaxTokenPlanProfile!.id
          }
        }
    })

    expect(resolved).toEqual(expect.objectContaining({
      enabled: true,
      providerId: 'minimax-token-plan',
      protocol: 'minimax-image',
      baseUrl: 'https://api.minimaxi.com',
      model: 'image-01'
    }))
  })

  it('routes MiniMax token plan media capabilities through the selected region host', () => {
    const minimax = getModelProviderPreset('minimax')
    expect(minimax).not.toBeNull()
    const cnProfile = modelProviderTokenPlanProfile(minimax!, 'https://api.minimaxi.com/anthropic')
    const globalProfile = modelProviderTokenPlanProfile(minimax!, 'https://api.minimax.io/anthropic')
    expect(cnProfile).toMatchObject({
      image: { baseUrl: 'https://api.minimaxi.com' },
      textToSpeech: { baseUrl: 'https://api.minimaxi.com' },
      music: { baseUrl: 'https://api.minimaxi.com' },
      video: { baseUrl: 'https://api.minimaxi.com' }
    })
    expect(globalProfile).toMatchObject({
      image: { baseUrl: 'https://api.minimax.io' },
      textToSpeech: { baseUrl: 'https://api.minimax.io' },
      music: { baseUrl: 'https://api.minimax.io' },
      video: { baseUrl: 'https://api.minimax.io' }
    })

    const staleGlobalCapabilityOnCnProfile = {
      ...cnProfile!,
      image: { ...cnProfile!.image!, baseUrl: 'https://api.minimax.io' },
      textToSpeech: { ...cnProfile!.textToSpeech!, baseUrl: 'https://api.minimax.io' },
      music: { ...cnProfile!.music!, baseUrl: 'https://api.minimax.io' },
      video: { ...cnProfile!.video!, baseUrl: 'https://api.minimax.io' }
    }
    const state = {
      ...settings(),
      provider: {
        ...defaultModelProviderSettings(),
        providers: [
          ...defaultModelProviderSettings().providers,
          staleGlobalCapabilityOnCnProfile
        ]
      },
      runtime: {
          ...defaultAnalytixRuntimeSettings(),
          imageGeneration: {
            ...defaultAnalytixRuntimeSettings().imageGeneration,
            enabled: true,
            providerId: staleGlobalCapabilityOnCnProfile.id
          },
          textToSpeech: {
            ...defaultAnalytixRuntimeSettings().textToSpeech,
            enabled: true,
            providerId: staleGlobalCapabilityOnCnProfile.id
          },
          musicGeneration: {
            ...defaultAnalytixRuntimeSettings().musicGeneration,
            enabled: true,
            providerId: staleGlobalCapabilityOnCnProfile.id
          },
          videoGeneration: {
            ...defaultAnalytixRuntimeSettings().videoGeneration,
            enabled: true,
            providerId: staleGlobalCapabilityOnCnProfile.id
          }
        }
    }

    expect(resolveAnalytixImageGenerationSettings(state).baseUrl).toBe('https://api.minimaxi.com')
    expect(resolveAnalytixTextToSpeechSettings(state).baseUrl).toBe('https://api.minimaxi.com')
    expect(resolveAnalytixMusicGenerationSettings(state).baseUrl).toBe('https://api.minimaxi.com')
    expect(resolveAnalytixVideoGenerationSettings(state).baseUrl).toBe('https://api.minimaxi.com')
  })

  it('exposes the Xiaomi preset speech capability', () => {
    const xiaomi = getModelProviderPreset('xiaomi')
    expect(xiaomi && modelProviderPresetProfile(xiaomi)).toMatchObject({
      id: 'xiaomi',
      speech: {
        protocol: 'mimo-asr',
        baseUrl: 'https://api.xiaomimimo.com/v1',
        models: ['mimo-v2.5-asr']
      },
      textToSpeech: {
        protocol: 'mimo-tts',
        baseUrl: 'https://api.xiaomimimo.com/v1',
        models: ['mimo-v2.5-tts', 'mimo-v2.5-tts-voicedesign', 'mimo-v2.5-tts-voiceclone']
      }
    })
  })

  it('keeps speech-only models out of the composer model list', () => {
    const base = settings()
    const resolved = listModelProviderModelIds({
      ...base,
      provider: {
        ...base.provider,
        providers: [
          ...base.provider.providers,
          {
            id: 'voice-lab',
            name: 'Voice Lab',
            baseUrl: 'https://voice.example/v1',
            endpointFormat: 'chat_completions',
            models: ['voice-chat', 'mimo-v2.5-asr', 'whisper-1'],
            modelProfiles: {},
            speech: {
              protocol: 'openai-transcriptions',
              baseUrl: 'https://voice.example/v1',
              models: ['whisper-1']
            }
          }
        ]
      }
    })

    expect(resolved).toContain('voice-chat')
    expect(resolved).not.toContain('mimo-v2.5-asr')
    expect(resolved).not.toContain('whisper-1')
  })

  it('classifies speech and image model ids without treating TTS as ASR', () => {
    expect(isSpeechToTextModelId('mimo-v2.5-asr')).toBe(true)
    expect(isSpeechToTextModelId('whisper-1')).toBe(true)
    expect(isSpeechToTextModelId('mimo-v2.5-tts')).toBe(false)
    expect(isTextToSpeechModelId('mimo-v2.5-tts')).toBe(true)
    expect(isTextToSpeechModelId('speech-2.8-hd')).toBe(true)
    expect(isMusicGenerationModelId('music-cover')).toBe(true)
    expect(isVideoGenerationModelId('MiniMax-Hailuo-2.3')).toBe(true)
    expect(isComposerChatModelId('mimo-v2.5-tts')).toBe(false)
    expect(isComposerChatModelId('speech-2.8-hd')).toBe(false)
    expect(isComposerChatModelId('music-2.6')).toBe(false)
    expect(isComposerChatModelId('MiniMax-Hailuo-2.3')).toBe(false)
    expect(isImageGenerationModelId('gpt-image-1')).toBe(true)
    expect(isImageGenerationModelId('seedream-4-0-250828')).toBe(true)
    expect(isImageGenerationModelId('text-embedding-3-large')).toBe(false)
  })

  it('keeps image-generation and other non-text models out of the composer model list', () => {
    const base = settings()
    const resolved = listModelProviderModelIds({
      ...base,
      provider: {
        ...base.provider,
        providers: [
          ...base.provider.providers,
          {
            id: 'art-lab',
            name: 'Art Lab',
            baseUrl: 'https://art.example/v1',
            endpointFormat: 'chat_completions',
            models: [
              'art-chat',
              'paint-house',
              'banana-canvas',
              'seedream-4-0-250828',
              'text-embedding-3-large'
            ],
            modelProfiles: {
              'banana-canvas': {
                inputModalities: ['text'],
                outputModalities: ['image'],
                supportsToolCalling: false,
                messageParts: ['text']
              }
            },
            image: {
              protocol: 'openai-images',
              baseUrl: 'https://art.example/v1',
              models: ['paint-house']
            }
          }
        ]
      }
    })

    expect(resolved).toContain('art-chat')
    expect(resolved).not.toContain('paint-house')
    expect(resolved).not.toContain('banana-canvas')
    expect(resolved).not.toContain('seedream-4-0-250828')
    expect(resolved).not.toContain('text-embedding-3-large')
  })

  it('backfills preset model capabilities for stale stored providers', () => {
    const base = settings()
    const resolved = resolveAnalytixRuntimeSettings({
      ...base,
      provider: {
        ...base.provider,
        providers: [
          ...base.provider.providers,
          {
            id: 'xiaomi-token-plan',
            name: 'Xiaomi Token Plan',
            baseUrl: 'https://token-plan-cn.xiaomimimo.com/v1',
            endpointFormat: 'chat_completions',
            models: ['mimo-v2-omni', 'mimo-v2.5', 'mimo-v2.5-pro'],
            modelProfiles: {}
          }
        ]
      }
    })

    expect(modelSupportsImageInput(resolved.modelProfiles['mimo-v2.5'])).toBe(true)
    expect(modelSupportsImageInput(resolved.modelProfiles['mimo-v2-omni'])).toBe(true)
    expect(resolved.modelProfiles[['mimo', 'v2', 'flash'].join('-')]).toBeUndefined()
    expect(resolved.modelProfiles['mimo-v2.5-pro']).toBeDefined()
  })

  it('keeps mimo-v2.5-pro text-only until a capability probe verifies image support', () => {
    const base = settings()
    const provider = modelProviderPresetProfile(getModelProviderPreset('xiaomi')!)
    const state: AppSettingsV1 = {
      ...base,
      provider: {
        ...base.provider,
        activeProviderId: 'xiaomi',
        providers: [
          ...base.provider.providers,
          provider
        ]
      },
      runtime: {
        ...base.runtime,
        providerId: 'xiaomi',
        model: 'mimo-v2.5-pro'
      }
    }

    expect(modelSupportsImageInput(resolveAnalytixRuntimeSettings(state).modelProfiles['mimo-v2.5-pro'])).toBe(false)

    const key = modelCapabilityProbeKey({
      providerId: 'xiaomi',
      model: 'mimo-v2.5-pro',
      baseUrl: provider.baseUrl,
      endpointFormat: provider.endpointFormat
    })
    const supported = resolveAnalytixRuntimeSettings({
      ...state,
      runtime: {
        ...state.runtime,
        modelCapabilityProbes: {
          [key]: {
            key,
            providerId: 'xiaomi',
            model: 'mimo-v2.5-pro',
            endpointFormat: provider.endpointFormat,
            sanitizedBaseUrl: provider.baseUrl,
            probedAt: '2026-06-30T00:00:00.000Z',
            staleAfter: '2999-01-01T00:00:00.000Z',
            imageInput: 'supported',
            toolCalling: 'supported',
            toolResultImage: 'supported',
            status: 'supported'
          }
        }
      }
    })
    expect(modelSupportsImageInput(supported.modelProfiles['mimo-v2.5-pro'])).toBe(true)
    expect(supported.modelProfiles['mimo-v2.5-pro']?.supportsToolCalling).toBe(true)

    const failed = resolveAnalytixRuntimeSettings({
      ...state,
      runtime: {
        ...state.runtime,
        modelCapabilityProbes: {
          [key]: {
            key,
            providerId: 'xiaomi',
            model: 'mimo-v2.5-pro',
            endpointFormat: provider.endpointFormat,
            sanitizedBaseUrl: provider.baseUrl,
            probedAt: '2026-06-30T00:00:00.000Z',
            staleAfter: '2999-01-01T00:00:00.000Z',
            imageInput: 'unsupported',
            toolCalling: 'unsupported',
            toolResultImage: 'unsupported',
            status: 'unsupported'
          }
        }
      }
    })
    expect(modelSupportsImageInput(failed.modelProfiles['mimo-v2.5-pro'])).toBe(false)
    expect(failed.modelProfiles['mimo-v2.5-pro']?.supportsToolCalling).toBe(false)
  })

  it('resolves Xiaomi speech-to-text through provider speech capability', () => {
    const xiaomi = getModelProviderPreset('xiaomi')
    expect(xiaomi).not.toBeNull()
    const xiaomiProfile = modelProviderPresetProfile(xiaomi!)
    const base = {
      ...settings(),
      provider: {
        ...defaultModelProviderSettings(),
        providers: [
          ...defaultModelProviderSettings().providers,
          xiaomiProfile
        ]
      },
      runtime: {
          ...defaultAnalytixRuntimeSettings(),
          speechToText: {
            ...defaultAnalytixRuntimeSettings().speechToText,
            enabled: true,
            providerId: xiaomiProfile.id
          }
        }
    }

    expect(listSpeechToTextProviderProfiles(base).map((profile) => profile.id)).toEqual(['xiaomi'])
    expect(resolveAnalytixSpeechToTextSettings(base)).toEqual(expect.objectContaining({
      enabled: true,
      providerId: 'xiaomi',
      protocol: 'mimo-asr',
      baseUrl: 'https://api.xiaomimimo.com/v1',
      model: 'mimo-v2.5-asr'
    }))
  })

  it('resolves provider-backed speech, music and video generation settings', () => {
    const minimax = getModelProviderPreset('minimax')
    const xiaomi = getModelProviderPreset('xiaomi')
    expect(minimax).not.toBeNull()
    expect(xiaomi).not.toBeNull()
    const minimaxProfile = modelProviderPresetProfile(minimax!)
    const xiaomiProfile = modelProviderPresetProfile(xiaomi!)
    const base = {
      ...settings(),
      provider: {
        ...defaultModelProviderSettings(),
        providers: [
          ...defaultModelProviderSettings().providers,
          minimaxProfile,
          xiaomiProfile
        ]
      },
      runtime: {
          ...defaultAnalytixRuntimeSettings(),
          textToSpeech: {
            ...defaultAnalytixRuntimeSettings().textToSpeech,
            enabled: true,
            providerId: minimaxProfile.id,
            baseUrl: 'https://stale-tts.example/v1',
            model: 'stale-voice-model'
          },
          musicGeneration: {
            ...defaultAnalytixRuntimeSettings().musicGeneration,
            enabled: true,
            providerId: minimaxProfile.id,
            baseUrl: 'https://stale-music.example/v1',
            model: 'stale-music-model'
          },
          videoGeneration: {
            ...defaultAnalytixRuntimeSettings().videoGeneration,
            enabled: true,
            providerId: minimaxProfile.id,
            baseUrl: 'https://stale-video.example/v1',
            model: 'stale-video-model'
          }
        }
    }

    expect(listTextToSpeechProviderProfiles(base).map((profile) => profile.id)).toEqual(['minimax', 'xiaomi'])
    expect(listMusicGenerationProviderProfiles(base).map((profile) => profile.id)).toEqual(['minimax'])
    expect(listVideoGenerationProviderProfiles(base).map((profile) => profile.id)).toEqual(['minimax'])
    expect(resolveAnalytixTextToSpeechSettings(base)).toEqual(expect.objectContaining({
      enabled: true,
      providerId: 'minimax',
      protocol: 'minimax-t2a',
      baseUrl: 'https://api.minimax.io',
      model: 'speech-2.8-hd'
    }))
    expect(resolveAnalytixMusicGenerationSettings(base)).toEqual(expect.objectContaining({
      enabled: true,
      providerId: 'minimax',
      protocol: 'minimax-music',
      baseUrl: 'https://api.minimax.io',
      model: 'music-2.6'
    }))
    expect(resolveAnalytixVideoGenerationSettings(base)).toEqual(expect.objectContaining({
      enabled: true,
      providerId: 'minimax',
      protocol: 'minimax-video',
      baseUrl: 'https://api.minimax.io',
      model: 'MiniMax-Hailuo-2.3'
    }))
  })

  it('repairs stale Xiaomi token plan speech endpoint and TTS model overrides', () => {
    const xiaomi = getModelProviderPreset('xiaomi')
    expect(xiaomi).not.toBeNull()
    const xiaomiTokenPlanProfile = modelProviderTokenPlanProfile(xiaomi!)
    expect(xiaomiTokenPlanProfile).not.toBeNull()
    const staleTokenPlanProfile = {
      ...xiaomiTokenPlanProfile!,
      speech: {
        ...xiaomiTokenPlanProfile!.speech!,
        baseUrl: 'https://api.xiaomimimo.com/v1'
      }
    }
    const resolved = resolveAnalytixSpeechToTextSettings({
      ...settings(),
      provider: {
        ...defaultModelProviderSettings(),
        providers: [
          ...defaultModelProviderSettings().providers,
          staleTokenPlanProfile
        ]
      },
      runtime: {
          ...defaultAnalytixRuntimeSettings(),
          speechToText: {
            ...defaultAnalytixRuntimeSettings().speechToText,
            enabled: true,
            providerId: staleTokenPlanProfile.id,
            model: 'mimo-v2.5-tts'
          }
        }
    })

    expect(resolved).toEqual(expect.objectContaining({
      enabled: true,
      providerId: 'xiaomi-token-plan',
      protocol: 'mimo-asr',
      baseUrl: 'https://token-plan-cn.xiaomimimo.com/v1',
      model: 'mimo-v2.5-asr'
    }))
  })

  it('keeps custom speech-to-text settings when no provider is selected', () => {
    const resolved = resolveAnalytixSpeechToTextSettings({
      ...settings(),
      runtime: {
          ...defaultAnalytixRuntimeSettings(),
          speechToText: {
            enabled: true,
            providerId: '',
            protocol: 'openai-transcriptions',
            baseUrl: 'https://speech.example/v1',
            model: 'whisper-1',
            language: 'zh',
            timeoutMs: 30_000
          }
        }
    })

    expect(resolved).toEqual(expect.objectContaining({
      enabled: true,
      providerId: '',
      protocol: 'openai-transcriptions',
      baseUrl: 'https://speech.example/v1',
      model: 'whisper-1',
      language: 'zh'
    }))
  })

  it('preserves a cleared default base URL while resolving the official runtime endpoint', () => {
    const state = settings()
    const normalized = normalizeModelProviderSettings({
      ...state.provider,
      baseUrl: '',
      providers: state.provider.providers.map((provider) =>
        provider.id === 'deepseek'
          ? { ...provider, baseUrl: '' }
          : provider
      )
    })

    expect(normalized.baseUrl).toBe('')
    expect(normalized.providers.find((provider) => provider.id === 'deepseek')?.baseUrl).toBe('')
    expect(resolveModelProviderBaseUrl({ ...state, provider: normalized })).toBe(DEFAULT_DEEPSEEK_BASE_URL)
  })

  it('keeps deprecated DeepSeek models out of the default provider list', () => {
    const defaultProvider = defaultModelProviderSettings().providers[0]
    const defaultModels = defaultProvider.models

    expect(defaultModels).toEqual(['deepseek-v4-flash', 'deepseek-v4-pro'])
    expect(defaultModels).not.toContain('deepseek-chat')
    expect(defaultModels).not.toContain('deepseek-reasoner')
    expect(defaultProvider.modelProfiles['deepseek-v4-pro']?.reasoning).toEqual({
      supportedEfforts: ['off', 'high', 'max'],
      defaultEffort: 'high',
      requestProtocol: 'deepseek-chat-completions'
    })
  })
})

describe('provider presets', () => {
  it('includes a LiteLLM preset', () => {
    const litellm = getModelProviderPreset('litellm')
    expect(litellm).not.toBeNull()
    expect(litellm && modelProviderPresetProfile(litellm)).toMatchObject({
      id: 'litellm',
      name: 'LiteLLM',
      baseUrl: 'http://localhost:4000',
      endpointFormat: 'chat_completions',
      models: []
    })
  })

  it('includes Zhipu, Z.ai, Kimi Code, and Moonshot presets', () => {
    const zhipu = getModelProviderPreset('zhipu-coding-plan')
    const zai = getModelProviderPreset('zai-coding-plan')
    const kimiCode = getModelProviderPreset('kimi-code')
    const moonshotCn = getModelProviderPreset('moonshot-cn')
    const moonshotGlobal = getModelProviderPreset('moonshot-global')

    expect(zhipu && modelProviderPresetProfile(zhipu)).toMatchObject({
      id: 'zhipu-coding-plan',
      name: 'Zhipu Coding Plan',
      baseUrl: 'https://open.bigmodel.cn/api/coding/paas/v4/chat/completions',
      endpointFormat: 'custom_endpoint',
      models: ['glm-5.2', 'glm-5.1', 'glm-5-turbo', 'glm-4.7', 'glm-4.5-air'],
      modelProfiles: {
        'glm-5.2': expect.objectContaining({
          contextWindowTokens: 1_000_000,
          supportsToolCalling: true,
          inputModalities: ['text']
        }),
        'glm-5.1': expect.objectContaining({
          contextWindowTokens: 200_000,
          supportsToolCalling: true
        })
      }
    })
    expect(zhipu && modelProviderPresetProfile(zhipu).modelProfiles['glm-5.2'].reasoning)
      .toEqual({
        supportedEfforts: ['off', 'high', 'max'],
        defaultEffort: 'max',
        requestProtocol: 'glm-chat-completions'
      })

    expect(zai && modelProviderPresetProfile(zai)).toMatchObject({
      id: 'zai-coding-plan',
      name: 'Z.ai Coding Plan',
      baseUrl: 'https://api.z.ai/api/coding/paas/v4/chat/completions',
      endpointFormat: 'custom_endpoint',
      models: ['glm-5.1', 'glm-5', 'glm-5-turbo', 'glm-4.7', 'glm-4.5-air'],
      modelProfiles: {
        'glm-5': expect.objectContaining({
          contextWindowTokens: 200_000,
          supportsToolCalling: true,
          inputModalities: ['text']
        })
      }
    })
    expect(zai && modelProviderPresetProfile(zai).modelProfiles['glm-5'].reasoning)
      .toEqual({
        supportedEfforts: ['off', 'high', 'max'],
        defaultEffort: 'max',
        requestProtocol: 'glm-chat-completions'
      })

    expect(kimiCode && modelProviderPresetProfile(kimiCode)).toMatchObject({
      id: 'kimi-code',
      name: 'Kimi Code',
      baseUrl: 'https://api.kimi.com/coding/v1',
      endpointFormat: 'chat_completions',
      models: ['kimi-for-coding'],
      modelProfiles: {
        'kimi-for-coding': expect.objectContaining({
          supportsToolCalling: true,
          inputModalities: ['text']
        })
      }
    })

    for (const preset of [moonshotCn, moonshotGlobal]) {
      const profile = preset && modelProviderPresetProfile(preset)
      expect(profile).toMatchObject({
        endpointFormat: 'chat_completions',
        models: [
          'kimi-k2.7-code',
          'kimi-k2.6',
          'kimi-k2.5',
          'moonshot-v1-128k',
          'moonshot-v1-32k',
          'moonshot-v1-8k'
        ],
        modelProfiles: {
          'kimi-k2.7-code': expect.objectContaining({
            supportsToolCalling: true,
            inputModalities: ['text', 'image'],
            messageParts: ['text', 'image_url']
          }),
          'moonshot-v1-128k': expect.objectContaining({
            contextWindowTokens: 128_000,
            inputModalities: ['text']
          })
        }
      })
      expect(profile && modelSupportsImageInput(profile.modelProfiles['kimi-k2.7-code']))
        .toBe(true)
    }
    expect(moonshotCn && modelProviderPresetProfile(moonshotCn).baseUrl)
      .toBe('https://api.moonshot.cn/v1')
    expect(moonshotGlobal && modelProviderPresetProfile(moonshotGlobal).baseUrl)
      .toBe('https://api.moonshot.ai/v1')
  })

  it('resolves new OpenAI-compatible presets through the selected provider', () => {
    const cases = [
      ['zhipu-coding-plan', 'https://open.bigmodel.cn/api/coding/paas/v4/chat/completions', 'glm-5.2', 'custom_endpoint'],
      ['zai-coding-plan', 'https://api.z.ai/api/coding/paas/v4/chat/completions', 'glm-5.1', 'custom_endpoint'],
      ['kimi-code', 'https://api.kimi.com/coding/v1', 'kimi-for-coding', 'chat_completions'],
      ['moonshot-cn', 'https://api.moonshot.cn/v1', 'kimi-k2.7-code', 'chat_completions'],
      ['moonshot-global', 'https://api.moonshot.ai/v1', 'kimi-k2.7-code', 'chat_completions']
    ] as const

    for (const [presetId, baseUrl, model, endpointFormat] of cases) {
      const preset = getModelProviderPreset(presetId)
      expect(preset).not.toBeNull()
      const profile = modelProviderPresetProfile(preset!)
      const resolved = resolveAnalytixRuntimeSettings({
        ...settings(),
        provider: {
          ...defaultModelProviderSettings(),
          providers: [
            ...defaultModelProviderSettings().providers,
            profile
          ]
        },
        runtime: {
            ...defaultAnalytixRuntimeSettings(),
            providerId: profile.id,
            model
          }
      })

      expect(resolved).toEqual(expect.objectContaining({
        baseUrl,
        endpointFormat,
        model
      }))
      expect(resolved.modelProfiles[model]).toEqual(expect.objectContaining({
        supportsToolCalling: true
      }))
    }
  })

  it('keeps per-model endpointFormat overrides on the OpenCode Go preset', () => {
    const preset = getModelProviderPreset('opencode-go')
    expect(preset).not.toBeNull()
    const profile = modelProviderPresetProfile(preset!)
    // MiniMax / Qwen route over Anthropic Messages...
    expect(profile.modelProfiles['minimax-m3'].endpointFormat).toBe('messages')
    expect(profile.modelProfiles['qwen3.7-max'].endpointFormat).toBe('messages')
    expect(resolveModelProviderEndpointFormat(profile, 'minimax-m3')).toBe('messages')
    // ...while chat-completions models carry no override (they inherit).
    expect(profile.modelProfiles['glm-5.1'].endpointFormat).toBeUndefined()
    expect(profile.modelProfiles['kimi-k2.7'].endpointFormat).toBeUndefined()
    expect(resolveModelProviderEndpointFormat(profile, 'glm-5.1')).toBe('chat_completions')

    // The override survives the full settings normalization round-trip.
    const resolved = resolveAnalytixRuntimeSettings({
      ...settings(),
      provider: {
        ...defaultModelProviderSettings(),
        providers: [...defaultModelProviderSettings().providers, profile]
      },
      runtime: { ...defaultAnalytixRuntimeSettings(), providerId: profile.id, model: 'minimax-m3' }
    })
    expect(resolved.modelProfiles['minimax-m3'].endpointFormat).toBe('messages')
    expect(resolved.modelProfiles['glm-5.1'].endpointFormat).toBeUndefined()
  })
})
