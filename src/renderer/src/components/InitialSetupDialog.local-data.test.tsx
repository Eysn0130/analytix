// @vitest-environment jsdom

import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_DEEPSEEK_BASE_URL, normalizeAppSettings, type AppSettingsPatch, type AppSettingsV1 } from '@shared/app-settings'
import type { ProviderRegistryRequest, ProviderRegistryResult } from '@shared/analytix-api'
import { providerRegistryRequestSchemaV1, providerRegistryResultSchemaV1 } from '../../../../packages/runtime/src/contracts/provider-registry.js'
import { rendererRuntimeClient } from '../agent/runtime-client'
import i18n from '../i18n'
import { useChatStore } from '../store/chat-store'
import { InitialSetupDialog } from './InitialSetupDialog'
import { persistProviderCredentialDraft, providerRegistryInput } from './initial-setup-save'
import { mergeSettings } from './settings-utils'

const initialChatState = useChatStore.getState()
let root: Root
let container: HTMLDivElement
let originalBridge: Window['analytix'] | undefined
let stored: AppSettingsV1
let publicWrites: AppSettingsPatch[]
type PublicProvider = Extract<ProviderRegistryResult, { providers: unknown[] }>['providers'][number]

function syntheticInput(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(24)), byte => byte.toString(16).padStart(2, '0')).join('')
}
const firstInput = syntheticInput()
const secondInput = syntheticInput()

function deferred() {
  let resolve!: () => void
  let reject!: (error: Error) => void
  const promise = new Promise<void>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

function setupRegistry(
  hold?: { operation: ProviderRegistryRequest['operation']; gate: ReturnType<typeof deferred> },
  initialProviders: PublicProvider[] = []
) {
  type Snapshot = Extract<ProviderRegistryResult, { providers: unknown[] }>
  let providers: Snapshot['providers'] = structuredClone(initialProviders).map(provider => ({
    ...provider, proxy: provider.proxy || undefined
  }))
  let selectedProviderId = ''
  let revision = 1
  const registryIncarnation = `inc_${'a'.repeat(43)}`
  const operations: ProviderRegistryRequest['operation'][] = []
  const entered = deferred()
  let held = false
  const request = async (input: ProviderRegistryRequest): Promise<ProviderRegistryResult> => {
    const parsed = providerRegistryRequestSchemaV1.safeParse(input)
    const diagnostic = parsed.success ? '' : [
      ...parsed.error.issues.map(issue => `${issue.path.join('.')}:${issue.code}`),
      `endpointBlank=${'provider' in input ? input.provider.endpoint.trim() === '' : false}`
    ].join(', ')
    expect(parsed.success, diagnostic).toBe(true)
    operations.push(input.operation)
    const base = () => ({ schemaVersion: 1 as const, registryRevision: String(revision), registryIncarnation })
    let result: ProviderRegistryResult
    if (input.operation === 'list') result = { ...base(), ...(selectedProviderId ? { selectedProviderId } : {}), providers: structuredClone(providers) }
    else if (input.operation === 'connect') {
      const next: Snapshot['providers'][number] = { ...input.provider, proxy: input.provider.proxy || undefined,
        selectedModel: input.provider.selectedModel || undefined,
        selectedMediaModel: input.provider.selectedMediaModel || undefined,
        credentialConfigured: true, credentialPurpose: 'provider-api-key', revision: '1', generation: '1',
        incarnation: `inc_${'b'.repeat(43)}`, tombstone: false }
      providers = [next]; revision += 1
      result = { ...base(), provider: structuredClone(next) }
    } else if (input.operation === 'probe') {
      const provider = providers.find((entry) => entry.id === input.providerId)!
      result = { ...base(), providerId: provider.id, providerRevision: provider.revision,
        providerGeneration: provider.generation, providerIncarnation: provider.incarnation,
        status: 'reachable', code: 200, modelCount: provider.models.length, latencyMs: 1 }
    } else if (input.operation === 'select') {
      selectedProviderId = input.providerId; revision += 1
      const provider = providers.find((entry) => entry.id === input.providerId)!
      provider.revision = String(Number(provider.revision) + 1)
      result = { ...base(), provider: structuredClone(provider) }
    } else throw new Error('Unexpected synthetic setup Registry operation')
    const parsedResult = providerRegistryResultSchemaV1.safeParse(result)
    const resultDiagnostic = parsedResult.success ? '' : parsedResult.error.issues
      .map(issue => `${issue.path.join('.')}:${issue.code}`).join(', ')
    expect(parsedResult.success, resultDiagnostic).toBe(true)
    if (!held && hold?.operation === input.operation) {
      held = true; entered.resolve(); await hold.gate.promise
    }
    return result
  }
  window.analytix = { providerRegistry: { request } } as unknown as Window['analytix']
  return { operations, entered, endpoints: () => providers.map(({ id, endpoint }) => ({ id, endpoint })) }
}

async function mount() { await act(async () => root.render(createElement(InitialSetupDialog))) }
function keyInput(): HTMLInputElement { return container.querySelector<HTMLInputElement>('input[id$="-api-key"]')! }
async function enter(input: HTMLInputElement, value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}
async function click(label: string) {
  const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent?.trim() === label)!
  expect(button).toBeDefined(); expect(button.disabled).toBe(false)
  await act(async () => button.click())
}
async function select(input: HTMLSelectElement, value: string) {
  await act(async () => { input.value = value; input.dispatchEvent(new Event('change', { bubbles: true })) })
}

describe('InitialSetupDialog local data path', () => {
  beforeEach(async () => {
    const actEnvironment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    await i18n.changeLanguage('en')
    container = document.createElement('div')
    document.body.append(container)
    root = createRoot(container)
    originalBridge = window.analytix
    stored = normalizeAppSettings({ workspaceRoot: '/workspace/analytix' } as AppSettingsV1)
    publicWrites = []
    vi.stubGlobal('fetch', () => { throw new Error('Unexpected fixture network') })
    vi.stubGlobal('WebSocket', class { constructor() { throw new Error('Unexpected fixture network') } })
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(
      stored
    )
    vi.spyOn(rendererRuntimeClient, 'setSettings').mockImplementation(async patch => {
      publicWrites.push(patch)
      stored = mergeSettings(stored, patch)
      return structuredClone(stored)
    })
    useChatStore.setState({
      ...initialChatState,
      initialSetupOpen: true,
      initialSetupMode: 'required',
      runtimeConnection: 'idle',
      reloadUiSettings: vi.fn(async () => undefined),
      probeRuntime: vi.fn(async () => { useChatStore.setState({ runtimeConnection: 'ready' }) }),
      applyI18nFromSettings: vi.fn(async () => undefined),
      openCode: vi.fn(async () => undefined),
      setComposerModel: vi.fn()
    })
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    useChatStore.setState(initialChatState)
    if (originalBridge) window.analytix = originalBridge
    else Reflect.deleteProperty(window, 'analytix')
    for (const value of [firstInput, secondInput]) {
      expect(JSON.stringify(publicWrites).includes(value)).toBe(false)
      expect(JSON.stringify(publicWrites).includes(btoa(value))).toBe(false)
    }
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    const actEnvironment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  it('ignores an initial settings read owned by an earlier setup mode', async () => {
    let oldResolve!: (value: AppSettingsV1) => void, currentResolve!: (value: AppSettingsV1) => void
    const old = new Promise<AppSettingsV1>(accept => { oldResolve = accept })
    const current = new Promise<AppSettingsV1>(accept => { currentResolve = accept })
    vi.mocked(rendererRuntimeClient.getSettings).mockReturnValueOnce(old).mockReturnValueOnce(current)
    await mount()
    await act(async () => useChatStore.setState({ initialSetupMode: 'preview' }))
    await act(async () => oldResolve(stored))
    expect(container.querySelector('input')).toBeNull()
    await act(async () => currentResolve(stored))
    expect(container.querySelector('input')).not.toBeNull()
  })

  it('shows a safe initial-read error with an uncached retry and the required local-data entry', async () => {
    vi.mocked(rendererRuntimeClient.getSettings).mockRejectedValueOnce(new Error('Synthetic private read detail'))
    const fresh = vi.spyOn(rendererRuntimeClient, 'readSettingsFromHost').mockResolvedValue(stored)
    await mount()
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(i18n.t('settings:settingsReadFailed'))
    expect(container.textContent).not.toContain('Synthetic private read detail')
    expect(container.textContent).toContain(i18n.t('settings:firstRunOpenLocalData'))
    await click(i18n.t('settings:settingsReadRetry'))
    expect(fresh).toHaveBeenCalledTimes(1)
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(container.querySelector('input')).not.toBeNull()
  })

  it('allows required local data after a read failure and ignores a retry from the departed setup entry', async () => {
    vi.mocked(rendererRuntimeClient.getSettings).mockRejectedValueOnce(new Error('Synthetic read failure'))
    let resolve!: (value: AppSettingsV1) => void
    const read = new Promise<AppSettingsV1>(accept => { resolve = accept })
    vi.spyOn(rendererRuntimeClient, 'readSettingsFromHost').mockReturnValue(read)
    await mount(); await click(i18n.t('settings:settingsReadRetry'))
    await click(i18n.t('settings:firstRunOpenLocalData'))
    expect(useChatStore.getState().initialSetupOpen).toBe(false)
    expect(useChatStore.getState().reloadUiSettings).toHaveBeenCalledTimes(1)
    expect(useChatStore.getState().openCode).not.toHaveBeenCalled()
    await act(async () => resolve(stored))
    expect(container.querySelector('input')).toBeNull()
    expect(publicWrites).toHaveLength(0)
  })

  it('dismisses required Provider setup explicitly while keeping model execution gated', async () => {
    await act(async () => {
      root.render(createElement(InitialSetupDialog))
      await Promise.resolve()
    })
    const entry = Array.from(container.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Open local data tools'
    )
    expect(entry).toBeDefined()
    expect(entry?.disabled).toBe(false)
    await act(async () => entry?.click())
    const state = useChatStore.getState()
    expect(state.initialSetupOpen).toBe(false)
    expect(state.runtimeConnection).toBe('idle')
    expect(state.reloadUiSettings).toHaveBeenCalledTimes(1)
    expect(state.probeRuntime).not.toHaveBeenCalled()
    expect(rendererRuntimeClient.setSettings).not.toHaveBeenCalled()
  })
  it('names protected fields and keeps required Escape gated without saving', async () => {
    await act(async () => root.render(createElement(InitialSetupDialog)))
    const modal = container.querySelector<HTMLElement>('[role="dialog"]')!
    expect(modal.contains(document.activeElement)).toBe(true)
    const key = container.querySelector<HTMLInputElement>('input[type="password"]')!
    expect(container.querySelector(`label[for="${key.id}"]`)?.textContent).toContain('API Key')
    const show = container.querySelector<HTMLButtonElement>('button[aria-pressed="false"][aria-label="Show value"]')!
    expect(show).not.toBeNull()
    await act(async () => show.click())
    expect(key.type).toBe('text'); expect(show.getAttribute('aria-label')).toBe('Hide value')
    await click('Advanced options')
    const base = container.querySelector<HTMLInputElement>('input[id$="-base-url"]')!
    expect(container.querySelector(`label[for="${base.id}"]`)?.textContent).toBeTruthy()
    await act(async () => modal.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
    expect(useChatStore.getState().initialSetupOpen).toBe(true)
    expect(rendererRuntimeClient.setSettings).not.toHaveBeenCalled()
    expect(useChatStore.getState().probeRuntime).not.toHaveBeenCalled()
  })
  it('restores the preview opener with Escape and handles loading to ready without losing the opener', async () => {
    let resolve!: (settings: AppSettingsV1) => void
    vi.mocked(rendererRuntimeClient.getSettings).mockImplementationOnce(() => new Promise(done => { resolve = done }))
    useChatStore.setState({ initialSetupMode: 'preview' })
    const opener = document.createElement('button'); document.body.append(opener); opener.focus()
    try {
      await act(async () => root.render(createElement(InitialSetupDialog)))
      const modal = container.querySelector<HTMLElement>('[role="dialog"]')!
      expect(modal.contains(document.activeElement)).toBe(true)
      expect(document.activeElement).toBe(modal.querySelector('button:not([disabled])'))
      await act(async () => resolve(normalizeAppSettings({ workspaceRoot: '/synthetic/a' } as AppSettingsV1)))
      expect(container.querySelector('[role="dialog"]')?.contains(document.activeElement)).toBe(true)
      await act(async () => document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
      expect(useChatStore.getState().initialSetupOpen).toBe(false)
      await act(async () => root.render(null)); expect(document.activeElement).toBe(opener)
      expect(rendererRuntimeClient.setSettings).not.toHaveBeenCalled()
    } finally { opener.remove() }
  })

  it('shows the preset and key first while keeping custom fields in the advanced entry', async () => {
    await mount()
    const provider = container.querySelector<HTMLSelectElement>('select[id$="-provider"]')!
    expect([...provider.options].map(option => option.value)).toEqual(['deepseek', 'xiaomi', 'minimax'])
    expect(keyInput()).not.toBeNull()
    expect(container.querySelector('input[id$="-base-url"]')).toBeNull()
    expect(container.querySelector('#initial-setup-model')).toBeNull()
    await click('Advanced options')
    expect(container.querySelector('input[id$="-base-url"]')).not.toBeNull()
    expect(container.querySelector('#initial-setup-model')).not.toBeNull()
    expect(container.querySelector('select[id$="-theme"]')).not.toBeNull()
    expect(container.querySelector('select[id$="-locale"]')).not.toBeNull()
  })

  it.each(['key', 'key-aba', 'endpoint', 'model', 'provider'] as const)('retains a newer %s edit after an older setup list settles', async (edit) => {
    const gate = deferred()
    const registry = setupRegistry({ operation: 'list', gate })
    await mount(); await enter(keyInput(), firstInput)
    await click('Save and continue')
    await registry.entered.promise
    if (edit === 'key' || edit === 'key-aba') {
      await enter(keyInput(), secondInput)
      if (edit === 'key-aba') await enter(keyInput(), firstInput)
    } else if (edit === 'provider') {
      await select(container.querySelector<HTMLSelectElement>('select[id$="-provider"]')!, 'xiaomi')
    } else {
      await click('Advanced options')
      const input = edit === 'endpoint'
        ? container.querySelector<HTMLInputElement>('input[id$="-base-url"]')!
        : container.querySelector<HTMLInputElement>('#initial-setup-model')!
      await enter(input, edit === 'endpoint' ? 'https://newer.invalid/v1' : 'newer-model')
    }
    await act(async () => { gate.resolve() })
    expect(registry.operations).toEqual(['list'])
    expect(publicWrites).toEqual([])
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(useChatStore.getState().initialSetupOpen).toBe(true)
    expect(useChatStore.getState().openCode).not.toHaveBeenCalled()
    expect(useChatStore.getState().probeRuntime).not.toHaveBeenCalled()
    if (edit === 'key' || edit === 'key-aba') expect(keyInput().value === (edit === 'key' ? secondInput : firstInput)).toBe(true)
    if (edit === 'endpoint') expect(container.querySelector<HTMLInputElement>('input[id$="-base-url"]')!.value).toBe('https://newer.invalid/v1')
    if (edit === 'model') expect(container.querySelector<HTMLInputElement>('#initial-setup-model')!.value).toBe('newer-model')
    if (edit === 'provider') {
      expect(container.querySelector<HTMLSelectElement>('select[id$="-provider"]')!.value).toBe('xiaomi')
      await select(container.querySelector<HTMLSelectElement>('select[id$="-provider"]')!, 'deepseek')
      expect(keyInput().value === firstInput).toBe(true)
    }
    const save = [...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent?.trim() === 'Save and continue')!
    expect(save.disabled).toBe(false)
    if (edit === 'key') {
      await click('Save and continue')
      expect(useChatStore.getState().initialSetupOpen).toBe(false)
      expect(useChatStore.getState().openCode).toHaveBeenCalledTimes(1)
    }
  })

  it('retains newer input after an already issued settings write completes without selecting or closing', async () => {
    const registry = setupRegistry()
    const gate = deferred(); const entered = deferred()
    vi.mocked(rendererRuntimeClient.setSettings).mockImplementationOnce(async patch => {
      publicWrites.push(patch); entered.resolve(); await gate.promise
      stored = mergeSettings(stored, patch); return structuredClone(stored)
    })
    await mount(); await enter(keyInput(), firstInput); await click('Save and continue')
    expect(container.querySelector('[role="alert"]')?.textContent ?? null).toBeNull()
    await entered.promise
    await enter(keyInput(), secondInput)
    await act(async () => { gate.resolve() })
    expect(registry.operations).toEqual(['list', 'connect', 'list', 'probe'])
    expect(publicWrites).toHaveLength(1)
    expect(keyInput().value === secondInput).toBe(true)
    expect(useChatStore.getState().initialSetupOpen).toBe(true)
    expect(useChatStore.getState().reloadUiSettings).not.toHaveBeenCalled()
    expect(useChatStore.getState().applyI18nFromSettings).not.toHaveBeenCalled()
    expect(container.querySelector('[role="alert"]')).toBeNull()
  })

  it('does not open Code or close required setup after newer input arrives during readiness', async () => {
    setupRegistry()
    const gate = deferred(); const entered = deferred()
    useChatStore.setState({ probeRuntime: vi.fn(async () => {
      entered.resolve(); await gate.promise; useChatStore.setState({ runtimeConnection: 'ready' })
    }) })
    await mount(); await enter(keyInput(), firstInput); await click('Save and continue')
    expect(container.querySelector('[role="alert"]')?.textContent ?? null).toBeNull()
    await entered.promise
    await enter(keyInput(), secondInput)
    await act(async () => { gate.resolve() })
    expect(keyInput().value === secondInput).toBe(true)
    expect(useChatStore.getState().openCode).not.toHaveBeenCalled()
    expect(useChatStore.getState().setComposerModel).not.toHaveBeenCalled()
    expect(useChatStore.getState().initialSetupOpen).toBe(true)
  })

  it.each([false, true])('retires a preview save when closed before its settlement (failure=%s)', async (failure) => {
    const gate = deferred()
    const registry = setupRegistry({ operation: 'list', gate })
    useChatStore.setState({ initialSetupMode: 'preview' })
    await mount(); await enter(keyInput(), firstInput); await click('Save and continue')
    await registry.entered.promise
    await click('Close')
    await act(async () => {
      if (failure) gate.reject(new Error('Synthetic retired save failure'))
      else gate.resolve()
    })
    expect(registry.operations).toEqual(['list'])
    expect(publicWrites).toEqual([])
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(useChatStore.getState().initialSetupOpen).toBe(false)
    expect(useChatStore.getState().probeRuntime).not.toHaveBeenCalled()
    expect(useChatStore.getState().openCode).not.toHaveBeenCalled()
  })

  it('completes a current required setup through persisted Registry selection and readiness', async () => {
    const registry = setupRegistry()
    await mount(); await enter(keyInput(), firstInput); await click('Save and continue')
    expect(container.querySelector('[role="alert"]')?.textContent ?? null).toBeNull()
    expect(registry.operations).toEqual(['list', 'connect', 'list', 'probe', 'select', 'list'])
    expect(registry.endpoints()).toEqual([{ id: 'deepseek', endpoint: DEFAULT_DEEPSEEK_BASE_URL }])
    expect(publicWrites).toHaveLength(1)
    expect(useChatStore.getState().probeRuntime).toHaveBeenCalledWith('user')
    expect(useChatStore.getState().openCode).toHaveBeenCalledTimes(1)
    expect(useChatStore.getState().setComposerModel).toHaveBeenCalledTimes(1)
    expect(useChatStore.getState().initialSetupOpen).toBe(false)
    expect(keyInput().value).toBe('')
  })

  it('keeps an explicit full endpoint when connecting a fresh DeepSeek Provider', async () => {
    const registry = setupRegistry()
    const endpoint = 'https://custom.invalid/provider/chat/completions'
    await mount(); await enter(keyInput(), firstInput); await click('Advanced options')
    await enter(container.querySelector<HTMLInputElement>('input[id$="-base-url"]')!, endpoint)
    await click('Save and continue')
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(registry.endpoints()).toEqual([{ id: 'deepseek', endpoint }])
    expect(useChatStore.getState().initialSetupOpen).toBe(false)
  })

  it('preserves an existing custom Registry endpoint when preview settings keep a blank URL', async () => {
    const profile = stored.provider.providers.find(provider => provider.id === 'deepseek')!
    expect(profile.baseUrl).toBe('')
    const endpoint = 'https://existing.invalid/provider/chat/completions'
    const registry = setupRegistry(undefined, [{
      ...providerRegistryInput(profile, stored.runtime.model, ''),
      endpoint, selectedMediaModel: undefined,
      credentialConfigured: true, credentialPurpose: 'provider-api-key',
      revision: '1', generation: '1', incarnation: `inc_${'b'.repeat(43)}`, tombstone: false
    }])
    useChatStore.setState({ initialSetupMode: 'preview' })
    await mount(); await click('Save and continue')
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(registry.operations).toEqual(['list', 'probe', 'select', 'list'])
    expect(registry.endpoints()).toEqual([{ id: 'deepseek', endpoint }])
    expect(useChatStore.getState().openCode).not.toHaveBeenCalled()
    expect(useChatStore.getState().initialSetupOpen).toBe(false)
  })

  it('does not assign the DeepSeek default to a blank endpoint for another Provider', async () => {
    const registry = setupRegistry()
    const profile = { ...stored.provider.providers[0], id: 'custom-provider', baseUrl: '' }
    await expect(persistProviderCredentialDraft({
      profile, selectedModel: profile.models[0], credentialDraft: firstInput,
      requestProviderRegistry: window.analytix.providerRegistry.request
    })).rejects.toThrow('endpointBlank=true')
    expect(registry.operations).toEqual(['list'])
    expect(registry.endpoints()).toEqual([])
  })

})
