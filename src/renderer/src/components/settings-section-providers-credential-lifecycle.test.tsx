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
  normalizeAppSettings
} from '@shared/app-settings'
import type { ProviderRegistryRequest, ProviderRegistryResult } from '@shared/analytix-api'
import { PROVIDER_REGISTRY_FAILURE_MESSAGES_V1, providerRegistryRequestSchemaV1, providerRegistryResultSchemaV1 } from '../../../../packages/runtime/src/contracts/provider-registry.js'
import { ProvidersSettingsSection } from './settings-section-providers'
import { useChatStore } from '../store/chat-store'

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
        generation: String(Number(prior?.generation ?? 0) + 1),
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

async function mount(expectCredential = true) {
  const form = normalizeAppSettings({
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
      update: (patch: unknown) => patches.push(patch), selectControlClass: ''
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

async function unmount() {
  await act(async () => { root!.unmount() })
  root = null
}

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
  vi.unstubAllGlobals()
})

describe('mounted Provider credential lifecycle', () => {
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

  it.each(['selection', 'rename'] as const)('C3 invalidates a pending draft after %s without a rollback', async (transition) => {
    await readyDraft()
    await click('showSecret')
    const gate = registry.hold('connect')
    await click('modelProviderDraftConfirm')
    expect(registry.mutations()[0]?.operation).toBe('connect')
    await gate.entered.promise
    if (transition === 'selection') await selectAlpha()
    else await enter(draftIdInput(), 'provider-renamed')
    expect(keyInput().type).toBe('password')
    const callsBeforeCompletion = registry.calls.length
    await act(async () => { gate.release.resolve() })
    expect(registry.calls.length).toBe(callsBeforeCompletion)
    expect(registry.snapshot().selectedProviderId).toBe('provider-alpha')
    expect(registry.mutations().length).toBe(1)
    expect(patches.length).toBe(0)
    expect(notices.length).toBe(0)
  })

  it('C3 ignores an older submission after a newer save and subsequent editing', async () => {
    await enter(keyInput(), firstInput)
    const gate = registry.hold('credential-replace')
    await click('modelProviderSaveChanges')
    await gate.entered.promise
    await enter(keyInput(), secondInput)
    await click('modelProviderSaveChanges')
    expect(keyInput().value.length).toBe(0)
    await enter(keyInput(), firstInput)
    const callsBeforeCompletion = registry.calls.length
    await act(async () => { gate.release.resolve() })
    expect(keyInput().value === firstInput).toBe(true)
    expect(registry.calls.length).toBe(callsBeforeCompletion)
    expect(registry.mutations().length).toBe(2)
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
    expect(notices).toEqual([{ tone: 'error', message: 'Provider credential update failed.' }])
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
