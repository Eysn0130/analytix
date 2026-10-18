// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  defaultAnalytixRuntimeSettings,
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  getModelProviderPreset,
  normalizeAppSettings,
  type AppSettingsPatch,
  type AppSettingsV1
} from '@shared/app-settings'
import type { ProviderRegistryRequest, ProviderRegistryResult } from '@shared/analytix-api'
import { PROVIDER_REGISTRY_FAILURE_MESSAGES_V1, providerRegistryRequestSchemaV1, providerRegistryResultSchemaV1 } from '../../../../packages/runtime/src/contracts/provider-registry.js'
import { ProvidersSettingsSection, type ProviderSettingsActions } from './settings-section-providers'
import { useChatStore } from '../store/chat-store'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { mergeSettings } from './settings-utils'
import { settingsPatchSchema } from '../../../main/ipc/app-ipc-schemas'

type Snapshot = Extract<ProviderRegistryResult, { providers: unknown[] }>
type Provider = Snapshot['providers'][number]
const registryIncarnation = `inc_${'a'.repeat(43)}`
const providerIncarnation = `inc_${'b'.repeat(43)}`
function syntheticInput(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(24)), (byte) => byte.toString(16).padStart(2, '0')).join('')
}
const firstInput = syntheticInput()
const secondInput = syntheticInput()

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>((done) => { resolve = done })
  return { promise, resolve }
}

function provider(id = 'provider-alpha'): Provider {
  return {
    id, kind: 'openai-compatible', endpoint: 'https://provider.invalid/v1',
    models: ['model-alpha'], mediaModels: [], selectedModel: 'model-alpha',
    selectedRoutes: [`provider:${id}`], credentialConfigured: true,
    credentialPurpose: 'provider-api-key', revision: '1', generation: '1',
    incarnation: providerIncarnation, tombstone: false
  }
}

function registryFixture() {
  let providers = [provider()]
  let selectedProviderId = providers[0]!.id
  let revision = 1
  const calls: ProviderRegistryRequest[] = []
  let failure: ProviderRegistryRequest['operation'] | null = null
  let legacyReentryRequired = false
  const gates: Array<{ operation: ProviderRegistryRequest['operation']; entered: ReturnType<typeof deferred>; release: ReturnType<typeof deferred> }> = []
  const snapshot = (): Snapshot => ({
    schemaVersion: 1, registryRevision: String(revision), registryIncarnation,
    selectedProviderId, providers: structuredClone(providers)
  })
  const request = async (input: ProviderRegistryRequest): Promise<ProviderRegistryResult> => {
    expect(providerRegistryRequestSchemaV1.safeParse(input).success).toBe(true)
    calls.push(input)
    let result: ProviderRegistryResult
    if (failure === input.operation) {
      failure = null
      result = { schemaVersion: 1, error: { code: 'runtime_unavailable', message: 'The provider registry is unavailable.' } }
    } else if (input.operation === 'list') {
      result = snapshot()
    } else if (input.operation === 'credential-check') {
      if (legacyReentryRequired) {
        result = { schemaVersion: 1, error: {
          code: 'credential_reentry_required', message: PROVIDER_REGISTRY_FAILURE_MESSAGES_V1.credential_reentry_required
        } }
      } else {
        const current = providers.find((entry) => entry.id === input.providerId)!
        result = {
          schemaVersion: 1, registryRevision: String(revision), registryIncarnation,
          providerId: current.id, providerRevision: current.revision,
          providerGeneration: current.generation, providerIncarnation: current.incarnation,
          credentialAvailable: true
        }
      }
    } else if (input.operation === 'credential-replace' || input.operation === 'connect' || input.operation === 'update') {
      expect(input.expected.registryRevision === String(revision)).toBe(true)
      expect(input.expected.registryIncarnation === registryIncarnation).toBe(true)
      const id = input.operation === 'connect' ? input.provider.id : input.providerId
      const prior = providers.find((entry) => entry.id === id)
      // Core's transactional identity keeps kind immutable after connect.
      if (input.operation === 'update') expect(input.provider.kind).toBe(prior!.kind)
      revision += 1
      const next: Provider = {
        ...(prior ?? provider(id)),
        ...(input.operation === 'credential-replace' ? {} : {
          kind: input.provider.kind, endpoint: input.provider.endpoint,
          models: input.provider.models, mediaModels: input.provider.mediaModels,
          selectedModel: input.provider.selectedModel || undefined,
          selectedMediaModel: input.provider.selectedMediaModel || undefined,
          selectedRoutes: input.provider.selectedRoutes
        }),
        revision: String(Number(prior?.revision ?? 0) + 1),
        generation: input.operation === 'update' && input.credential.kind === 'keep'
          ? prior!.generation : String(Number(prior?.generation ?? 0) + 1),
        credentialConfigured: true, credentialPurpose: 'provider-api-key'
      }
      providers = [...providers.filter((entry) => entry.id !== id), next].sort((left, right) => left.id.localeCompare(right.id))
      if (input.operation === 'credential-replace') legacyReentryRequired = false
      result = { schemaVersion: 1, registryRevision: String(revision), registryIncarnation, provider: structuredClone(next) }
    } else if (input.operation === 'select') {
      expect(input.expected.registryRevision === String(revision)).toBe(true)
      selectedProviderId = input.providerId
      revision += 1
      const selected = providers.find((entry) => entry.id === input.providerId)!
      const next = { ...selected, revision: String(Number(selected.revision) + 1) }
      providers = providers.map((entry) => entry.id === next.id ? next : entry)
      result = { schemaVersion: 1, registryRevision: String(revision), registryIncarnation, provider: structuredClone(next) }
    } else {
      throw new Error('Unexpected fixture Registry operation')
    }
    expect(providerRegistryResultSchemaV1.safeParse(result).success).toBe(true)
    const gateIndex = gates.findIndex((gate) => gate.operation === input.operation)
    if (gateIndex >= 0) {
      const [gate] = gates.splice(gateIndex, 1)
      gate!.entered.resolve()
      await gate!.release.promise
    }
    return result
  }
  return {
    request, snapshot, calls,
    addProvider(id: string) { providers.push(provider(id)); providers.sort((left, right) => left.id.localeCompare(right.id)) },
    failNext(operation: ProviderRegistryRequest['operation']) { failure = operation },
    hold(operation: ProviderRegistryRequest['operation']) {
      const gate = { operation, entered: deferred(), release: deferred() }
      gates.push(gate)
      return gate
    },
    requireLegacyReentry() { legacyReentryRequired = true },
    mutations: () => calls.filter((call) => call.operation !== 'list' && call.operation !== 'credential-check')
  }
}

let root: Root | null
let container: HTMLDivElement
let registry: ReturnType<typeof registryFixture>
let notices: unknown[]
let patches: unknown[]
const originalNotice = useChatStore.getState().showTopNotice

function keyInput(): HTMLInputElement {
  const input = container.querySelector<HTMLInputElement>('input[placeholder="modelProviderApiKeyPlaceholder"], input[placeholder="modelProviderApiKeySavedPlaceholder"], input[placeholder="modelProviderApiKeyReentryPlaceholder"]')
  expect(Boolean(input)).toBe(true)
  return input!
}

async function click(label: string) {
  const button = [...container.querySelectorAll('button')].find((node) =>
    node.textContent?.trim() === label || node.getAttribute('aria-label') === label)
  expect(Boolean(button)).toBe(true)
  expect(button!.disabled).toBe(false)
  await act(async () => { button!.click() })
}

async function enter(input: HTMLInputElement, value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

async function mount(
  expectCredential = true,
  savedForm?: AppSettingsV1,
  saveModelPreferences?: (patch: AppSettingsPatch) => Promise<AppSettingsV1>,
  onProviderActionsChange?: (actions: ProviderSettingsActions | null) => void
) {
  const form = savedForm ?? normalizeAppSettings({
    version: 1, locale: 'en', theme: 'system', uiFontScale: 'small',
    provider: defaultModelProviderSettings(), runtime: defaultAnalytixRuntimeSettings(),
    workspaceRoot: '', log: { enabled: false, retentionDays: 7 },
    notifications: { turnComplete: true },
    appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
    keyboardShortcuts: defaultKeyboardShortcuts(), write: defaultWriteSettings(),
    claw: defaultClawSettings(), schedule: defaultScheduleSettings(),
    guiUpdate: { channel: 'stable' }, codePromptPrefix: '', disabledSkillIds: []
  })
  root = createRoot(container)
  await act(async () => {
    root!.render(<ProvidersSettingsSection ctx={{
      t: (key: string) => key, form, provider: form.provider, analytix: form.runtime,
      update: (patch: unknown) => patches.push(patch), saveModelPreferences,
      onProviderActionsChange, selectControlClass: ''
    }} />)
  })
  expect(registry.calls.some((call) => call.operation === 'list')).toBe(true)
  if (expectCredential) expect(keyInput().type).toBe('password')
}

async function addDraft() {
  await click('modelProviderAdd')
  await click('modelProviderAddMenuCustom')
}

function draftIdInput(): HTMLInputElement {
  const label = [...container.querySelectorAll('label')].find((node) =>
    node.textContent?.trim().startsWith('modelProviderId'))
  const input = label?.querySelector('input')
  expect(Boolean(input)).toBe(true)
  return input!
}

async function readyDraft() {
  await addDraft()
  await click('providerModelAdd')
  const modelInput = container.querySelector<HTMLInputElement>('input[placeholder="providerModelIdPlaceholder"]')
  expect(Boolean(modelInput)).toBe(true)
  await enter(modelInput!, 'model-alpha')
  await click('providerModelSave')
  await enter(keyInput(), firstInput)
}

async function selectAlpha() {
  const button = [...container.querySelectorAll<HTMLButtonElement>('button[aria-pressed]')].find((node) =>
    node.textContent?.includes('provider-alpha'))
  expect(Boolean(button)).toBe(true)
  await act(async () => { button!.click() })
}

function endpointInput(): HTMLInputElement {
  const label = [...container.querySelectorAll('label')].find((node) => node.textContent?.startsWith('modelProviderBaseUrl'))
  const input = label?.querySelector('input')
  expect(input).toBeDefined()
  return input!
}

async function mountExistingPreset() {
  const preset = getModelProviderPreset('moonshot-cn')!
  registry.addProvider(preset.id)
  await unmount(); await mount()
  const button = [...container.querySelectorAll<HTMLButtonElement>('button[aria-pressed]')].find((node) =>
    node.querySelector('span')?.textContent === preset.name)
  expect(button).toBeDefined()
  await act(async () => { button!.click() })
  expect(draftIdInput().value).toBe(preset.id)
  return preset
}

async function choosePreset(name: string) {
  await click('modelProviderAdd')
  const button = [...container.querySelectorAll<HTMLButtonElement>('button[role="menuitem"]')].find((node) =>
    node.querySelector('span')?.textContent === name)
  expect(button).toBeDefined()
  await act(async () => { button!.click() })
}

async function unmount() {
  await act(async () => { root!.unmount() })
  root = null
}

it('selects native Messages at creation and preserves the saved Registry protocol', async () => {
  const endpoint = (): HTMLSelectElement => {
    const label = [...container.querySelectorAll('label')].find(node => node.textContent?.startsWith('modelProviderEndpointFormat'))
    expect(label?.querySelector('select')).not.toBeNull()
    return label!.querySelector('select')!
  }
  expect(endpoint().disabled).toBe(true)
  expect(container.textContent).toContain('modelProviderEndpointFormatCommitted')
  await readyDraft()
  expect(endpoint().disabled).toBe(false)
  const draftId = draftIdInput().value
  await act(async () => {
    endpoint().value = 'deepseek-messages'
    endpoint().dispatchEvent(new Event('change', { bubbles: true }))
  })
  await click('modelProviderDraftConfirm')
  expect(registry.snapshot().providers.find(item => item.id === draftId)?.kind).toBe('deepseek-messages')
  expect(registry.calls.find(call => call.operation === 'connect')).toMatchObject({
    provider: { kind: 'deepseek-messages' }, credential: { kind: 'set' }
  })
  expect(patches.some(patch => Object.prototype.hasOwnProperty.call(patch, 'provider'))).toBe(false)
  await unmount()
  await mount()
  expect(endpoint().value).toBe('deepseek-messages')
  expect(endpoint().disabled).toBe(true)
  const generation = registry.snapshot().providers.find(item => item.id === draftId)?.generation
  await click('modelProviderSaveChanges')
  expect(registry.snapshot().providers.find(item => item.id === draftId)).toMatchObject({
    kind: 'deepseek-messages', generation
  })
})

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('fetch', () => { throw new Error('Unexpected fixture network') })
  vi.stubGlobal('WebSocket', class { constructor() { throw new Error('Unexpected fixture network') } })
  registry = registryFixture()
  notices = []
  patches = []
  useChatStore.setState({ showTopNotice: (notice) => { notices.push(notice) } })
  const unexpected = () => { throw new Error('Unexpected fixture bridge') }
  window.analytix = new Proxy({ providerRegistry: { request: registry.request } }, {
    get(target, name) { return name in target ? target[name as keyof typeof target] : unexpected() }
  }) as unknown as Window['analytix']
  container = document.createElement('div')
  document.body.append(container)
  root = null
  await mount()
})

afterEach(async () => {
  if (root) await act(async () => { root!.unmount() })
  root = null
  container.remove()
  const publicValues = JSON.stringify([notices, patches, registry.snapshot()])
  for (const value of [firstInput, secondInput]) {
    expect(publicValues.includes(value)).toBe(false)
    expect(publicValues.includes(btoa(value))).toBe(false)
  }
  useChatStore.setState({ showTopNotice: originalNotice })
  rendererRuntimeClient.invalidateSettings()
  vi.unstubAllGlobals()
})

describe('mounted Provider credential lifecycle', () => {
  it('keeps required custom fields visible while advanced and destructive controls start collapsed', () => {
    const advanced = [...container.querySelectorAll('details')].find(item => item.querySelector('summary')?.textContent === 'modelProviderAdvancedSettings')!
    const danger = [...container.querySelectorAll('details')].find(item => item.querySelector('summary')?.textContent === 'modelProviderSectionDanger')!
    expect(advanced.open).toBe(false)
    expect(danger.open).toBe(false)
    expect(keyInput().closest('details')).toBeNull()
    expect(endpointInput().closest('details')).toBeNull()
    const proxy = container.querySelector<HTMLInputElement>('input[placeholder="proxyUrlPlaceholder"]')!
    expect(proxy.closest('details')).toBe(advanced)
  })

  it('shows the required protocol and first model outside advanced settings for a new custom provider', async () => {
    await addDraft()
    const protocol = [...container.querySelectorAll('label')].find(item =>
      item.textContent?.startsWith('modelProviderEndpointFormat'))?.querySelector('select')
    const addModel = [...container.querySelectorAll('button')].find(item =>
      item.textContent?.trim() === 'providerModelAdd')
    expect(protocol).toBeDefined()
    expect(protocol!.disabled).toBe(false)
    expect(protocol!.closest('details')).toBeNull()
    expect(addModel).toBeDefined()
    expect(addModel!.closest('details')).toBeNull()
    expect(endpointInput().closest('details')).toBeNull()
  })

  it.each(['endpoint', 'model', 'key-aba'] as const)('preserves a newer %s draft when an older preset confirmation resolves', async (edit) => {
    const preset = await mountExistingPreset()
    if (edit === 'key-aba') await selectAlpha()
    const initialEndpoint = endpointInput().value
    const initialProviderId = draftIdInput().value
    const gate = deferred()
    const entered = deferred()
    const confirmDialog = vi.fn(async () => { entered.resolve(); await gate.promise; return true })
    Object.assign(window.analytix, { app: { confirmDialog } })
    await choosePreset(preset.name)
    await entered.promise
    if (edit === 'endpoint') await enter(endpointInput(), 'https://newer.invalid/v1')
    else if (edit === 'model') {
      await click('providerModelAdd')
      await enter(container.querySelector<HTMLInputElement>('input[placeholder="providerModelIdPlaceholder"]')!, 'newer-model')
      await click('providerModelSave')
    } else {
      await enter(keyInput(), firstInput)
      await enter(keyInput(), '')
    }
    await act(async () => { gate.resolve() })
    expect(confirmDialog).toHaveBeenCalledTimes(1)
    expect(draftIdInput().value).toBe(initialProviderId)
    expect(endpointInput().value).toBe(edit === 'endpoint' ? 'https://newer.invalid/v1' : initialEndpoint)
    if (edit === 'model') expect(container.textContent).toContain('newer-model')
    expect(registry.mutations()).toEqual([])
    expect(notices).toEqual([])
  })

  it('does not request preset confirmation over an already dirty Provider draft', async () => {
    const preset = await mountExistingPreset()
    const confirmDialog = vi.fn(async () => true)
    Object.assign(window.analytix, { app: { confirmDialog } })
    await enter(endpointInput(), 'https://newer.invalid/v1')
    await choosePreset(preset.name)
    expect(confirmDialog).not.toHaveBeenCalled()
    expect(endpointInput().value).toBe('https://newer.invalid/v1')
    expect(notices).toEqual([{ tone: 'info', message: 'modelProviderUnsavedChanges' }])
    expect(registry.mutations()).toEqual([])
  })

  it('keeps an unchanged explicit preset confirmation available as a local draft', async () => {
    const preset = await mountExistingPreset()
    await selectAlpha()
    const confirmDialog = vi.fn(async () => true)
    Object.assign(window.analytix, { app: { confirmDialog } })
    await choosePreset(preset.name)
    expect(confirmDialog).toHaveBeenCalledTimes(1)
    expect(draftIdInput().value).toBe(preset.id)
    expect(endpointInput().value).toBe(preset.baseUrl)
    expect(container.textContent).toContain(preset.models[0])
    expect(container.textContent).toContain('model-alpha')
    expect(registry.mutations()).toEqual([])
    expect(notices).toEqual([])
  })

  it('blocks snapshot-changing Registry actions while a Provider input is unsaved', async () => {
    await enter(keyInput(), firstInput)
    const before = registry.calls.length
    for (const label of ['modelProviderFetchModels', 'modelProviderDisconnect', 'modelProviderRemove']) {
      await click(label)
      expect(keyInput().value === firstInput).toBe(true)
    }
    expect(registry.calls.length).toBe(before)
    expect(notices).toEqual(Array.from({ length: 3 }, () => ({ tone: 'info', message: 'modelProviderUnsavedChanges' })))
  })

  it('retains input entered while another Provider selection snapshot is pending', async () => {
    registry.addProvider('provider-beta')
    await unmount(); await mount()
    const beta = [...container.querySelectorAll<HTMLButtonElement>('button[aria-pressed]')].find((node) => node.textContent?.includes('provider-beta'))!
    await act(async () => { beta.click() })
    const gate = registry.hold('select')
    await click('modelProviderSelect')
    await gate.entered.promise
    await enter(keyInput(), secondInput)
    await act(async () => { gate.release.resolve() })
    expect(registry.snapshot().selectedProviderId).toBe('provider-beta')
    expect(keyInput().value === secondInput).toBe(true)
    expect(draftIdInput().value).toBe('provider-beta')
    expect(notices).toEqual([])
    await click('modelProviderDiscardChanges')
    expect(keyInput().value).toBe('')
  })

  it('retains a newer endpoint draft after an older Provider update completes', async () => {
    const endpoint = [...container.querySelectorAll('label')].find((node) => node.textContent?.startsWith('modelProviderBaseUrl'))!.querySelector('input')!
    await enter(endpoint, 'https://older.invalid/v1')
    const gate = registry.hold('update')
    await click('modelProviderSaveChanges')
    await gate.entered.promise
    await enter(endpoint, 'https://newer.invalid/v1')
    const callsBeforeCompletion = registry.calls.length
    await act(async () => { gate.release.resolve() })
    expect(endpoint.value).toBe('https://newer.invalid/v1')
    expect(registry.snapshot().providers[0]!.endpoint).toBe('https://older.invalid/v1')
    expect(registry.calls.length).toBe(callsBeforeCompletion)
    expect(notices).toEqual([])
    await click('modelProviderSaveChanges')
    expect(registry.snapshot().providers[0]!.endpoint).toBe('https://newer.invalid/v1')
    expect(registry.mutations()).toHaveLength(2)
  })

  it('shows a key-free legacy re-entry notice and clears it after fenced replacement', async () => {
    registry.requireLegacyReentry()
    const beforeChecks = registry.calls.filter((call) => call.operation === 'credential-check').length
    await unmount()
    await mount()
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)) })
    expect(registry.calls.filter((call) => call.operation === 'credential-check').length).toBeGreaterThan(beforeChecks)
    expect(container.textContent).toContain('modelProviderApiKeyReentryHint')
    expect(keyInput().placeholder).toBe('modelProviderApiKeyReentryPlaceholder')
    expect(keyInput().value).toBe('')
    await enter(keyInput(), firstInput)
    await click('modelProviderSaveChanges')
    expect(registry.mutations().at(-1)?.operation).toBe('credential-replace')
    expect(container.textContent).not.toContain('modelProviderApiKeyReentryHint')
    expect(keyInput().value).toBe('')
  })

  it('shows committed credential state after remount without restoring the secret into the input', async () => {
    expect(keyInput().placeholder).toBe('modelProviderApiKeySavedPlaceholder')
    expect(container.textContent).toContain('modelProviderApiKeySavedHint')
    expect(keyInput().value).toBe('')
    await unmount()
    await mount()
    expect(keyInput().placeholder).toBe('modelProviderApiKeySavedPlaceholder')
    expect(keyInput().value).toBe('')
    await addDraft()
    expect(keyInput().placeholder).toBe('modelProviderApiKeyPlaceholder')
    expect(container.textContent).not.toContain('modelProviderApiKeySavedHint')
    expect(registry.mutations()).toHaveLength(0)
  })

  it('clears an unchanged successful input after a real helper write and readback', async () => {
    await enter(keyInput(), firstInput)
    expect(keyInput().value === firstInput).toBe(true)
    await click('modelProviderSaveChanges')
    expect(registry.mutations().length).toBe(1)
    expect(registry.mutations()[0]?.operation).toBe('credential-replace')
    expect(keyInput().value.length).toBe(0)
    expect(notices.length).toBe(0)
  })

  it('C1 discards input before recreating the same custom draft identity', async () => {
    await addDraft()
    await enter(keyInput(), firstInput)
    expect(keyInput().value === firstInput).toBe(true)
    await click('modelProviderDraftDiscard')
    await addDraft()
    expect(keyInput().value.length).toBe(0)
    expect(registry.mutations().length).toBe(0)
  })

  it('C1 ends the old credential identity when a draft ID is renamed', async () => {
    await addDraft()
    const originalId = draftIdInput().value
    await enter(keyInput(), firstInput)
    await click('showSecret')
    expect(keyInput().type).toBe('text')
    await enter(draftIdInput(), 'provider-renamed')
    expect(keyInput().type).toBe('password')
    await enter(draftIdInput(), originalId)
    expect(keyInput().value.length).toBe(0)
    expect(registry.mutations().length).toBe(0)
  })

  it.each([false, true])('C2 retains a newer input revision after an older completion (ABA=%s)', async (aba) => {
    await enter(keyInput(), firstInput)
    const gate = registry.hold('credential-replace')
    await click('modelProviderSaveChanges')
    expect(registry.mutations()[0]?.operation).toBe('credential-replace')
    await gate.entered.promise
    expect(registry.mutations().length).toBe(1)
    await enter(keyInput(), secondInput)
    if (aba) await enter(keyInput(), firstInput)
    await act(async () => { gate.release.resolve() })
    expect(keyInput().value === (aba ? firstInput : secondInput)).toBe(true)
    expect(notices.length).toBe(0)
    await click('modelProviderSaveChanges')
    expect(registry.mutations().length).toBe(2)
    expect(keyInput().value.length).toBe(0)
  })

  it('commits and selects a valid mounted custom draft through the real helper chain', async () => {
    await readyDraft()
    const id = draftIdInput().value
    await click('modelProviderDraftConfirm')
    expect(registry.mutations().map((call) => call.operation)).toEqual(['connect', 'select'])
    expect(registry.snapshot().selectedProviderId === id).toBe(true)
    expect(patches.length).toBe(1)
    expect(notices.length).toBe(0)
    expect(keyInput().value.length).toBe(0)
  })

  it('C3 stops a cancelled pre-write continuation before connecting', async () => {
    await readyDraft()
    const gate = registry.hold('list')
    await click('modelProviderDraftConfirm')
    await gate.entered.promise
    await click('modelProviderDraftDiscard')
    await act(async () => { gate.release.resolve() })
    expect(registry.mutations().length).toBe(0)
    expect(patches.length).toBe(0)
    expect(notices.length).toBe(0)
  })

  it('C3 preserves a new draft after the cancelled draft write completes', async () => {
    await readyDraft()
    const gate = registry.hold('connect')
    await click('modelProviderDraftConfirm')
    expect(registry.mutations()[0]?.operation).toBe('connect')
    await gate.entered.promise
    await click('modelProviderDraftDiscard')
    await addDraft()
    await enter(draftIdInput(), 'provider-next')
    await enter(keyInput(), secondInput)
    const callsBeforeCompletion = registry.calls.length
    await act(async () => { gate.release.resolve() })
    expect(draftIdInput().value).toBe('provider-next')
    expect(draftIdInput().disabled).toBe(false)
    expect(keyInput().value === secondInput).toBe(true)
    expect(registry.calls.length).toBe(callsBeforeCompletion)
    expect(registry.mutations().length).toBe(1)
    expect(patches.length).toBe(0)
    expect(notices.length).toBe(0)
  })

  it('blocks selecting another Provider until the pending dirty draft is explicitly cancelled', async () => {
    await readyDraft()
    const draftId = draftIdInput().value
    await click('showSecret')
    const gate = registry.hold('connect')
    await click('modelProviderDraftConfirm')
    await gate.entered.promise
    await selectAlpha()
    expect(draftIdInput().value).toBe(draftId)
    expect(keyInput().value === firstInput).toBe(true)
    expect(keyInput().type).toBe('text')
    expect(notices).toEqual([{ tone: 'info', message: 'modelProviderUnsavedChanges' }])
    await click('modelProviderDraftDiscard')
    expect(keyInput().type).toBe('password')
    expect(keyInput().value).toBe('')
    const callsBeforeCompletion = registry.calls.length
    await act(async () => { gate.release.resolve() })
    expect(registry.calls.length).toBe(callsBeforeCompletion)
    expect(registry.snapshot().selectedProviderId).toBe('provider-alpha')
    expect(registry.mutations().length).toBe(1)
    expect(patches.length).toBe(0)
  })

  it('C3 invalidates a pending draft after rename without a rollback', async () => {
    await readyDraft()
    await click('showSecret')
    const gate = registry.hold('connect')
    await click('modelProviderDraftConfirm')
    expect(registry.mutations()[0]?.operation).toBe('connect')
    await gate.entered.promise
    await enter(draftIdInput(), 'provider-renamed')
    expect(keyInput().type).toBe('password')
    const callsBeforeCompletion = registry.calls.length
    await act(async () => { gate.release.resolve() })
    expect(registry.calls.length).toBe(callsBeforeCompletion)
    expect(registry.snapshot().selectedProviderId).toBe('provider-alpha')
    expect(registry.mutations().length).toBe(1)
    expect(patches.length).toBe(0)
    expect(notices.length).toBe(0)
  })

  it('serializes Provider Save and preserves input entered while the older save is pending', async () => {
    await enter(keyInput(), firstInput)
    const gate = registry.hold('credential-replace')
    await click('modelProviderSaveChanges')
    await gate.entered.promise
    await enter(keyInput(), secondInput)
    const save = [...container.querySelectorAll('button')].find((node) => node.textContent?.trim() === 'modelProviderSaveChanges')!
    expect(save.disabled).toBe(true)
    await act(async () => { save.click() })
    expect(registry.mutations().length).toBe(1)
    const callsBeforeCompletion = registry.calls.length
    await act(async () => { gate.release.resolve() })
    expect(keyInput().value === secondInput).toBe(true)
    expect(registry.calls.slice(callsBeforeCompletion).filter((call) => call.operation === 'list')).toHaveLength(1)
    expect(registry.mutations().length).toBe(1)
    await click('modelProviderSaveChanges')
    expect(keyInput().value).toBe('')
    expect(registry.mutations().length).toBe(2)
    await enter(keyInput(), firstInput)
    expect(keyInput().value === firstInput).toBe(true)
    expect(notices.length).toBe(0)
  })

  it.each(['list', 'connect'] as const)('C4 ends the mounted continuation while %s is pending', async (operation) => {
    await readyDraft()
    const gate = registry.hold(operation)
    await click('modelProviderDraftConfirm')
    await gate.entered.promise
    const callsBeforeCompletion = registry.calls.length
    await unmount()
    await act(async () => { gate.release.resolve() })
    expect(registry.calls.length).toBe(callsBeforeCompletion)
    expect(registry.mutations().length).toBe(operation === 'list' ? 0 : 1)
    expect(patches.length).toBe(0)
    expect(notices.length).toBe(0)
    await mount()
    expect(keyInput().value.length).toBe(0)
  })

  it('keeps an active failed input available for retry with a fixed public failure', async () => {
    await enter(keyInput(), firstInput)
    registry.failNext('credential-replace')
    await click('modelProviderSaveChanges')
    expect(keyInput().value === firstInput).toBe(true)
    expect(notices).toEqual([{ tone: 'error', message: 'modelProviderUpdateFailed' }])
    await click('modelProviderSaveChanges')
    expect(keyInput().value.length).toBe(0)
    expect(registry.mutations().length).toBe(2)
  })

  it('suppresses a failed response delivered after unmount', async () => {
    await enter(keyInput(), firstInput)
    registry.failNext('credential-replace')
    const gate = registry.hold('credential-replace')
    await click('modelProviderSaveChanges')
    await gate.entered.promise
    await unmount()
    await act(async () => { gate.release.resolve() })
    expect(notices.length).toBe(0)
    expect(patches.length).toBe(0)
  })
})

async function mountWithFooterActions() {
  await unmount()
  let latest: ProviderSettingsActions | null = null
  const registrations: Array<ProviderSettingsActions | null> = []
  await mount(true, undefined, undefined, (actions) => {
    latest = actions
    registrations.push(actions)
  })
  return {
    registrations,
    current: (): ProviderSettingsActions => {
      expect(latest).not.toBeNull()
      return latest!
    }
  }
}

describe('modal footer Provider actions', () => {
  it('registers final Save and Cancel without duplicate Provider buttons and unregisters on close', async () => {
    const footer = await mountWithFooterActions()
    expect(footer.current()).toMatchObject({ dirty: false, busy: false })
    expect(container.textContent).not.toContain('modelProviderSaveChanges')
    await enter(keyInput(), firstInput)
    expect(footer.current().dirty).toBe(true)
    expect(container.textContent).not.toContain('modelProviderDiscardChanges')
    await act(async () => { expect(await footer.current().save()).toBe(true) })
    expect(keyInput().value).toBe('')
    expect(footer.current()).toMatchObject({ dirty: false, busy: false })
    expect(registry.mutations().map((call) => call.operation)).toEqual(['credential-replace'])
    const stale = footer.current()
    await unmount()
    expect(footer.registrations.at(-1)).toBeNull()
    expect(await stale.save()).toBe(false)
    await mount()
    expect(keyInput().value).toBe('')
    expect(keyInput().placeholder).toBe('modelProviderApiKeySavedPlaceholder')
  })

  it('keeps a failed footer Save draft available for explicit retry', async () => {
    const footer = await mountWithFooterActions()
    await enter(keyInput(), firstInput)
    registry.failNext('credential-replace')
    await act(async () => { expect(await footer.current().save()).toBe(false) })
    expect(keyInput().value === firstInput).toBe(true)
    expect(footer.current()).toMatchObject({ dirty: true, busy: false })
    expect(notices).toEqual([{ tone: 'error', message: 'modelProviderUpdateFailed' }])
    await act(async () => { expect(await footer.current().save()).toBe(true) })
    expect(keyInput().value).toBe('')
  })

  it('discards in-memory credentials, Provider drafts and an unfinished model form without clearing the saved key', async () => {
    const footer = await mountWithFooterActions()
    const baseline = registry.snapshot()
    await enter(keyInput(), firstInput)
    await click('showSecret')
    await click('providerModelEditAction')
    await act(async () => { expect(await footer.current().save()).toBe(false) })
    await act(async () => { footer.current().discard() })
    expect(keyInput().value).toBe('')
    expect(keyInput().type).toBe('password')
    expect(container.querySelector('input[placeholder="providerModelIdPlaceholder"]')).toBeNull()
    expect(footer.current().dirty).toBe(false)
    await readyDraft()
    expect(container.textContent).not.toContain('modelProviderDraftConfirm')
    expect(container.textContent).not.toContain('modelProviderDraftDiscard')
    await act(async () => { footer.current().discard() })
    expect(draftIdInput().value).toBe('provider-alpha')
    expect(keyInput().value).toBe('')
    expect(footer.current().dirty).toBe(false)
    expect(registry.snapshot()).toEqual(baseline)
    expect(registry.mutations()).toEqual([])
  })

  it('connects and selects a new Provider through final footer Save', async () => {
    const footer = await mountWithFooterActions()
    await readyDraft()
    const id = draftIdInput().value
    await act(async () => { expect(await footer.current().save()).toBe(true) })
    expect(registry.mutations().map((call) => call.operation)).toEqual(['connect', 'select'])
    expect(registry.snapshot().selectedProviderId).toBe(id)
    expect(footer.current()).toMatchObject({ dirty: false, busy: false })
    expect(keyInput().value).toBe('')
  })

  it('keeps a newer key draft open and serializes footer Save while an earlier write is pending', async () => {
    const footer = await mountWithFooterActions()
    await enter(keyInput(), firstInput)
    const gate = registry.hold('credential-replace')
    let pending!: Promise<boolean>
    await act(async () => { pending = footer.current().save() })
    await gate.entered.promise
    expect(footer.current().busy).toBe(true)
    expect(await footer.current().save()).toBe(false)
    await enter(keyInput(), secondInput)
    await act(async () => {
      gate.release.resolve()
      expect(await pending).toBe(false)
    })
    expect(keyInput().value === secondInput).toBe(true)
    expect(footer.current()).toMatchObject({ dirty: true, busy: false })
    expect(registry.mutations()).toHaveLength(1)
    await act(async () => { expect(await footer.current().save()).toBe(true) })
    expect(keyInput().value).toBe('')
  })

  it('does not acknowledge a model form opened during an earlier footer Save', async () => {
    const footer = await mountWithFooterActions()
    await enter(keyInput(), firstInput)
    const gate = registry.hold('credential-replace')
    let pending!: Promise<boolean>
    await act(async () => { pending = footer.current().save() })
    await gate.entered.promise
    await click('providerModelEditAction')
    await act(async () => { reasoningToggle().click() })
    await act(async () => {
      gate.release.resolve()
      expect(await pending).toBe(false)
    })
    expect(reasoningToggle().getAttribute('aria-checked')).toBe('true')
    expect(footer.current()).toMatchObject({ dirty: true, busy: false })
    await act(async () => { footer.current().discard() })
    expect(footer.current().dirty).toBe(false)
  })

  it('invalidates a discarded pre-write continuation before it can connect', async () => {
    const footer = await mountWithFooterActions()
    await readyDraft()
    const gate = registry.hold('list')
    let pending!: Promise<boolean>
    await act(async () => { pending = footer.current().save() })
    await gate.entered.promise
    await act(async () => { footer.current().discard() })
    await act(async () => {
      gate.release.resolve()
      expect(await pending).toBe(false)
    })
    expect(draftIdInput().value).toBe('provider-alpha')
    expect(keyInput().value).toBe('')
    expect(footer.current()).toMatchObject({ dirty: false, busy: false })
    expect(registry.mutations()).toEqual([])
    expect(patches).toEqual([])
  })
})

it('shows unavailable state and retries without replacing saved providers or credentials', async () => {
  await unmount()
  const before = registry.snapshot()
  registry.failNext('list')
  await mount(false)
  expect(container.querySelector('[role="alert"]')?.textContent).toContain('modelProviderRegistryUnavailable')
  expect(registry.snapshot()).toEqual(before)
  expect(registry.mutations()).toEqual([])
  expect(patches).toEqual([])
  await click('modelProviderRegistryRetry')
  expect(container.querySelector('[role="alert"]')).toBeNull()
  expect(keyInput().type).toBe('password')
  expect(keyInput().value).toBe('')
  expect(registry.snapshot()).toEqual(before)
  expect(registry.mutations()).toEqual([])
  expect(patches).toEqual([])
})

async function preferencePersistenceFixture() {
  await unmount()
  let stored = normalizeAppSettings({ workspaceRoot: '/synthetic/model-preference-ui' } as AppSettingsV1)
  let failure = false
  let gate: ReturnType<typeof deferred> | null = null
  const writes: AppSettingsPatch[] = []
  window.analytix = {
    providerRegistry: { request: registry.request },
    settings: {
      getSettings: async () => structuredClone(stored),
      setSettings: async (partial: AppSettingsPatch) => {
        const accepted = settingsPatchSchema.parse(partial) as AppSettingsPatch
        writes.push(accepted)
        if (gate) await gate.promise
        if (failure) { failure = false; throw new Error('Synthetic settings save failure') }
        stored = mergeSettings(stored, accepted)
        return structuredClone(stored)
      }
    },
    app: { invalidateQueryCache: async () => undefined }
  } as unknown as Window['analytix']
  rendererRuntimeClient.invalidateSettings()
  const reopen = async () => {
    if (root) await unmount()
    await mount(true, await rendererRuntimeClient.getSettings({ forceRefresh: true }),
      (patch) => rendererRuntimeClient.setSettings(patch))
  }
  await reopen()
  return {
    writes, reopen,
    readback: () => rendererRuntimeClient.getSettings({ forceRefresh: true }),
    failNext() { failure = true },
    holdNext() { gate = deferred(); return gate }
  }
}

function reasoningToggle(): HTMLButtonElement {
  const button = [...container.querySelectorAll<HTMLButtonElement>('button[role="switch"]')].find((node) =>
    node.parentElement?.textContent?.startsWith('providerModelReasoningLabel'))
  expect(button).toBeDefined()
  return button!
}

function modelSelect(label: string): HTMLSelectElement {
  const owner = [...container.querySelectorAll('label')].find((node) => node.textContent?.startsWith(label))
  expect(owner?.querySelector('select')).not.toBeNull()
  return owner!.querySelector('select')!
}

async function enableHighReasoning() {
  await click('providerModelEditAction')
  await act(async () => { reasoningToggle().click() })
  for (const [label, value] of [['providerModelReasoningDefault', 'high'], ['providerModelReasoningProtocol', 'deepseek-chat-completions']]) {
    await act(async () => {
      const select = modelSelect(label)
      select.value = value
      select.dispatchEvent(new Event('change', { bubbles: true }))
    })
  }
}

function expectHighReasoning() {
  expect(reasoningToggle().getAttribute('aria-checked')).toBe('true')
  expect(modelSelect('providerModelReasoningDefault').value).toBe('high')
  expect(modelSelect('providerModelReasoningProtocol').value).toBe('deepseek-chat-completions')
}

describe('model drafts and final Provider Save readback', () => {
  it('keeps model edits as drafts until Provider Save and restores the committed choice on reopen', async () => {
    const fixture = await preferencePersistenceFixture()
    const initial = registry.snapshot()
    await enableHighReasoning()
    await click('providerModelSave')
    expect(container.querySelector('input[placeholder="providerModelIdPlaceholder"]')).toBeNull()
    expect(fixture.writes).toHaveLength(0)
    expect((await fixture.readback()).provider.modelPreferences ?? []).toEqual([])
    await click('providerModelEditAction'); expectHighReasoning(); await click('providerModelCancel')
    await click('modelProviderSaveChanges')
    expect(fixture.writes).toHaveLength(1)
    expect((await fixture.readback()).provider.modelPreferences?.[0].modelProfiles['model-alpha'].reasoning?.defaultEffort).toBe('high')
    expect(registry.mutations()).toHaveLength(0)
    expect(registry.snapshot()).toEqual(initial)
    await click('providerModelEditAction'); expectHighReasoning(); await click('providerModelCancel')
    await fixture.reopen()
    await click('providerModelEditAction'); expectHighReasoning()
    expect(keyInput().value).toBe('')
  })
  it('retains the model draft after failed Provider persistence and allows final Save retry', async () => {
    const fixture = await preferencePersistenceFixture()
    await enableHighReasoning()
    await click('providerModelSave')
    fixture.failNext()
    await click('modelProviderSaveChanges')
    expect(notices).toEqual([{ tone: 'error', message: 'modelProviderUpdateFailed' }])
    expect((await fixture.readback()).provider.modelPreferences ?? []).toEqual([])
    await click('providerModelEditAction'); expectHighReasoning(); await click('providerModelCancel')
    await click('modelProviderSaveChanges')
    await fixture.reopen(); await click('providerModelEditAction'); expectHighReasoning()
  })
  it('keeps explicit off after Provider Save and ordinary settings reopen', async () => {
    const fixture = await preferencePersistenceFixture()
    await enableHighReasoning(); await click('providerModelSave')
    await click('modelProviderSaveChanges')
    await click('providerModelEditAction')
    await act(async () => { reasoningToggle().click() })
    await click('providerModelSave')
    expect((await fixture.readback()).provider.modelPreferences?.[0].modelProfiles['model-alpha'].reasoning?.defaultEffort).toBe('high')
    await click('modelProviderSaveChanges')
    expect((await fixture.readback()).provider.modelPreferences?.[0].modelProfiles['model-alpha'].reasoning).toBeNull()
    await fixture.reopen(); await click('providerModelEditAction')
    expect(reasoningToggle().getAttribute('aria-checked')).toBe('false')
    expect(container.textContent).not.toContain('providerModelReasoningDefault')
    expect(registry.mutations()).toHaveLength(0)
  })
  it('blocks replacing dirty Provider edits during Save and permits it after explicit discard', async () => {
    const fixture = await preferencePersistenceFixture()
    await enableHighReasoning()
    await click('providerModelSave')
    const held = fixture.holdNext()
    await click('modelProviderSaveChanges')
    const saveProvider = [...container.querySelectorAll('button')].find((node) => node.textContent?.trim() === 'modelProviderSaveChanges')!
    expect(saveProvider.disabled).toBe(true)
    await enter(keyInput(), secondInput)
    await addDraft()
    expect(draftIdInput().value).toBe('provider-alpha')
    expect(keyInput().value === secondInput).toBe(true)
    expect(notices).toEqual([{ tone: 'info', message: 'modelProviderUnsavedChanges' }])
    await act(async () => { held.resolve() })
    expect(draftIdInput().value).toBe('provider-alpha')
    expect(keyInput().value === secondInput).toBe(true)
    await click('providerModelEditAction'); expectHighReasoning(); await click('providerModelCancel')
    expect(fixture.writes).toHaveLength(1)
    await click('modelProviderDiscardChanges')
    expect(keyInput().value).toBe('')
    await addDraft()
    expect(draftIdInput().value).not.toBe('provider-alpha')
    expect(registry.mutations()).toHaveLength(0)
  })
})

it('discards an older Provider Save snapshot after a newer model draft is applied', async () => {
  const fixture = await preferencePersistenceFixture()
  const older = registry.hold('list')
  await click('modelProviderSaveChanges')
  await older.entered.promise
  await enableHighReasoning(); await click('providerModelSave')
  expect((await fixture.readback()).provider.modelPreferences ?? []).toEqual([])
  await act(async () => { older.release.resolve() })
  expect(fixture.writes).toHaveLength(0)
  await click('providerModelEditAction'); expectHighReasoning(); await click('providerModelCancel')
  await click('modelProviderSaveChanges')
  expect(fixture.writes).toHaveLength(1)
  await fixture.reopen(); await click('providerModelEditAction'); expectHighReasoning()
  expect(registry.mutations()).toHaveLength(0)
})

it('keeps unsupported capability and identity fields out of the actual editors', async () => {
  expect([...container.querySelectorAll('label')].some((node) => node.textContent?.startsWith('modelProviderName'))).toBe(false)
  expect(container.querySelector('[aria-label="modelProviderImageCapability"]')).toBeNull()
  expect(container.querySelector('[aria-label="modelProviderSpeechCapability"]')).toBeNull()
  await click('providerModelEditAction')
  for (const key of ['providerModelContextLabel', 'providerModelVisionLabel', 'providerModelToolsLabel', 'providerModelEndpointFormatLabel']) {
    expect(container.textContent).not.toContain(key)
  }
  expect(container.textContent).toContain('providerModelAliasesLabel')
  expect(container.textContent).toContain('providerModelReasoningLabel')
})
