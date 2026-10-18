// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsPatch, type AppSettingsV1 } from '@shared/app-settings'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { useChatStore } from '../store/chat-store'
import { SETTINGS_CHANGED_EVENT } from '../lib/keyboard-shortcut-settings'
import { mergeSettings } from './settings-utils'
import i18n from '../i18n'
import { settingsPatchSchema } from '../../../main/ipc/app-ipc-schemas'
import * as registry from '../agent/registry'
import type { AgentProvider } from '../agent/types'
import type { CoreMemoryRecordJson } from '../agent/analytix-contract'

type SectionContext = {
  form: AppSettingsV1
  update: (partial: AppSettingsPatch) => void
  saveModelPreferences: (partial: AppSettingsPatch) => Promise<AppSettingsV1>
  memoryRecords: CoreMemoryRecordJson[]
  createMemoryRecord: (input: { content: string; scope?: 'user' | 'workspace' | 'project' }) => Promise<boolean>
  refreshAnalytixDiagnostics: () => Promise<void>
}

const capture = vi.hoisted(() => ({ current: null as SectionContext | null, realProviders: false }))

vi.mock('./settings-sections', async () => {
  const { createElement: h } = await import('react')
  const { ProvidersSettingsSection } = await import('./settings-section-providers')
  const section = ({ ctx }: { ctx: SectionContext }) => {
    capture.current = ctx
    return h('div', { 'data-test-settings-form': ctx.form.codePromptPrefix })
  }
  const sections = Object.fromEntries([
    'Agents', 'ArchivedThreads', 'Browser', 'Claw', 'ComputerUse', 'General',
    'ImageGeneration', 'KeyboardShortcuts', 'LlmDebug', 'Worktree',
    'MediaGeneration', 'Memory', 'Permissions', 'Providers', 'SpeechToText',
    'Updates', 'Write'
  ].map((name) => [`${name}SettingsSection`, section]))
  return { ...sections, ProvidersSettingsSection: ({ ctx }: { ctx: SectionContext }) => {
    capture.current = ctx
    return capture.realProviders ? h(ProvidersSettingsSection, { ctx }) : section({ ctx })
  } }
})

import { SettingsView } from './SettingsView'

const preference = {
  id: 'provider-alpha',
  baseUrl: 'https://provider.invalid/v1',
  endpointFormat: 'chat_completions',
  modelProfiles: {
    'model-alpha': {
      aliases: [],
      reasoning: {
        supportedEfforts: ['off', 'high'],
        defaultEffort: 'high',
        requestProtocol: 'deepseek-chat-completions'
      }
    }
  }
} satisfies NonNullable<AppSettingsV1['provider']['modelPreferences']>[number]

function preferencePatch(): AppSettingsPatch {
  return { provider: { modelPreferences: [structuredClone(preference)] } }
}

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>((done) => { resolve = done })
  return { promise, resolve }
}

type WriteGate = ReturnType<typeof deferred> & { failure?: Error }

const initialStore = useChatStore.getState()
let root: Root | null = null
let host: HTMLDivElement
let originalBridge: Window['analytix'] | undefined
let stored: AppSettingsV1
let writes: AppSettingsPatch[]
let gates: Map<number, WriteGate>
let events: AppSettingsV1[]
let removeEventListener: () => void

function context(): SectionContext {
  expect(capture.current).not.toBeNull()
  return capture.current!
}

async function mount() {
  capture.current = null
  root = createRoot(host)
  await act(async () => { root!.render(createElement(SettingsView)) })
  expect(typeof context().saveModelPreferences).toBe('function')
}

async function unmount() {
  if (!root) return
  await act(async () => { root!.unmount() })
  root = null
}

async function advanceDebounce() {
  await act(async () => { await vi.advanceTimersByTimeAsync(450) })
}

async function update(patch: AppSettingsPatch) {
  await act(async () => { context().update(patch) })
}

async function beginPreferenceSave() {
  let pending!: Promise<AppSettingsV1>
  await act(async () => {
    pending = context().saveModelPreferences(preferencePatch())
    // Attach immediately so intentional failure tests cannot produce an
    // unhandled rejection before their rejection assertion runs.
    void pending.catch(() => undefined)
  })
  return { pending }
}

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('fetch', () => { throw new Error('Unexpected fixture network') })
  vi.stubGlobal('WebSocket', class { constructor() { throw new Error('Unexpected fixture network') } })
  await i18n.changeLanguage('en')
  vi.useFakeTimers()
  stored = normalizeAppSettings({ workspaceRoot: '/synthetic/settings-queue' } as AppSettingsV1)
  writes = []
  gates = new Map()
  events = []
  originalBridge = window.analytix
  window.analytix = {
    settings: {
      getSettings: vi.fn(async () => structuredClone(stored)),
      setSettings: vi.fn(async (partial: AppSettingsPatch) => {
        writes.push(structuredClone(partial))
        const gate = gates.get(writes.length)
        if (gate) {
          await gate.promise
          if (gate.failure) throw gate.failure
        }
        stored = mergeSettings(stored, settingsPatchSchema.parse(partial) as AppSettingsPatch)
        return structuredClone(stored)
      })
    },
    app: { invalidateQueryCache: vi.fn(async () => undefined) },
    write: {},
    updates: {},
    logs: { error: vi.fn(async () => undefined) }
  } as unknown as Window['analytix']
  rendererRuntimeClient.invalidateSettings()
  useChatStore.setState({
    settingsSection: 'providers',
    applyI18nFromSettings: vi.fn(async () => undefined),
    reloadUiSettings: vi.fn(async () => undefined),
    probeRuntime: vi.fn(async () => undefined),
    showTopNotice: vi.fn()
  })
  const onChanged = (event: Event) => {
    events.push((event as CustomEvent<AppSettingsV1>).detail)
  }
  window.addEventListener(SETTINGS_CHANGED_EVENT, onChanged)
  removeEventListener = () => window.removeEventListener(SETTINGS_CHANGED_EVENT, onChanged)
  host = document.createElement('div')
  document.body.append(host)
  await mount()
})

afterEach(async () => {
  await unmount()
  removeEventListener()
  host.remove()
  rendererRuntimeClient.invalidateSettings()
  if (originalBridge) window.analytix = originalBridge
  else Reflect.deleteProperty(window, 'analytix')
  useChatStore.setState(initialStore)
  capture.current = null
  capture.realProviders = false
  vi.clearAllTimers()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('SettingsView normal model preference save queue', () => {
  it('drains the actual older write before saving preferences against the latest form', async () => {
    const older = deferred()
    gates.set(1, older)
    await update({ codePromptPrefix: 'Older scheduled draft' })
    await advanceDebounce()
    expect(writes).toHaveLength(1)
    expect(writes[0]?.codePromptPrefix).toBe('Older scheduled draft')

    const { pending } = await beginPreferenceSave()
    let settled = false
    void pending.then(() => { settled = true }, () => { settled = true })
    // This ordinary edit happens while the explicit save waits for the older
    // bridge write. It must remain in the form and in the eventual readback.
    await update({ codePromptPrefix: 'Latest settings draft' })
    expect(writes).toHaveLength(1)
    expect(settled).toBe(false)

    let saved!: AppSettingsV1
    await act(async () => {
      older.resolve()
      saved = await pending
    })
    expect(writes).toHaveLength(2)
    expect(stored.codePromptPrefix).toBe('Latest settings draft')
    expect(saved.provider.modelPreferences).toEqual([preference])
    expect(context().form.codePromptPrefix).toBe('Latest settings draft')
    expect(context().form.provider.modelPreferences).toEqual([preference])
    expect(await rendererRuntimeClient.getSettings({ forceRefresh: true })).toEqual(saved)
  })

  it('cancels an older scheduled snapshot before it can overwrite the committed preference', async () => {
    await update({ codePromptPrefix: 'Pending ordinary draft' })
    const { pending } = await beginPreferenceSave()
    let saved!: AppSettingsV1
    await act(async () => { saved = await pending })
    expect(writes).toHaveLength(1)
    await advanceDebounce()
    expect(writes).toHaveLength(1)
    expect(saved.codePromptPrefix).toBe('Pending ordinary draft')
    expect((await rendererRuntimeClient.getSettings({ forceRefresh: true })).provider.modelPreferences)
      .toEqual([preference])
  })

  it('rejects a failed normal write and allows a subsequent explicit Save to commit', async () => {
    const failed = { ...deferred(), failure: new Error('Synthetic settings persistence failure') }
    gates.set(1, failed)
    const { pending } = await beginPreferenceSave()
    const rejection = expect(pending).rejects.toThrow('Synthetic settings persistence failure')
    await act(async () => {
      failed.resolve()
      await rejection
    })
    expect(stored.provider.modelPreferences ?? []).toEqual([])
    expect(events).toHaveLength(0)

    const retry = await beginPreferenceSave()
    let saved!: AppSettingsV1
    await act(async () => { saved = await retry.pending })
    expect(writes).toHaveLength(2)
    expect(saved.provider.modelPreferences).toEqual([preference])
    expect((await rendererRuntimeClient.getSettings({ forceRefresh: true })).provider.modelPreferences)
      .toEqual([preference])
  })

  it('drains an older failed scheduled write without poisoning the newer explicit Save', async () => {
    const older = { ...deferred(), failure: new Error('Synthetic older save failure') }
    gates.set(1, older)
    await update({ codePromptPrefix: 'Retain this ordinary edit' })
    await advanceDebounce()
    const { pending } = await beginPreferenceSave()
    expect(writes).toHaveLength(1)
    let saved!: AppSettingsV1
    await act(async () => {
      older.resolve()
      saved = await pending
    })
    expect(writes).toHaveLength(2)
    expect(saved.codePromptPrefix).toBe('Retain this ordinary edit')
    expect(saved.provider.modelPreferences).toEqual([preference])
  })

  it('does not publish an older entry completion into a reopened settings form', async () => {
    const older = deferred()
    gates.set(1, older)
    const { pending } = await beginPreferenceSave()
    expect(writes).toHaveLength(1)
    await unmount()
    await mount()
    await update({ codePromptPrefix: 'New entry draft' })
    const before = events.length

    await act(async () => {
      older.resolve()
      await pending.catch(() => undefined)
    })
    // A write already handed to Main may finish, but its old component must
    // not emit a settings-change event or replace the new entry's draft.
    expect(events).toHaveLength(before)
    expect(context().form.codePromptPrefix).toBe('New entry draft')
    expect(context().form.provider.modelPreferences ?? []).toEqual([])
  })
})

it('drops already-queued stale snapshots after a newer explicit Save drains the older write', async () => {
  const older = deferred(); gates.set(1, older)
  await update({ codePromptPrefix: 'Issued older write' }); await advanceDebounce()
  const { pending } = await beginPreferenceSave()
  await update({ codePromptPrefix: 'Queued stale draft' }); await advanceDebounce()
  await update({ codePromptPrefix: 'Latest retained draft' })
  await act(async () => { older.resolve(); await pending })
  await advanceDebounce()
  expect(writes).toHaveLength(2)
  expect(stored.codePromptPrefix).toBe('Latest retained draft')
  expect((await rendererRuntimeClient.getSettings({ forceRefresh: true })).provider.modelPreferences).toEqual([preference])
})

it.each([false, true])('keeps unapplied model preferences local, blocks departure, and retries final Provider Save %s', async (reenter) => {
  await unmount()
  const registryProvider = { id: 'provider-alpha', kind: 'openai-compatible', endpoint: 'https://provider.invalid/v1',
    models: ['model-alpha'], mediaModels: [], selectedModel: 'model-alpha', selectedRoutes: ['provider:provider-alpha'],
    credentialConfigured: true, credentialPurpose: 'provider-api-key', revision: '1', generation: '1',
    incarnation: `inc_${'b'.repeat(43)}`, tombstone: false }
  Object.assign(window.analytix, { providerRegistry: { request: async (request: { operation: string }) => {
    if (request.operation === 'credential-check') return { schemaVersion: 1, registryRevision: '1',
      registryIncarnation: `inc_${'a'.repeat(43)}`, providerId: registryProvider.id, providerRevision: '1',
      providerGeneration: '1', providerIncarnation: registryProvider.incarnation, credentialAvailable: true }
    if (request.operation !== 'list') throw new Error('Unexpected Registry mutation')
    return { schemaVersion: 1, registryRevision: '1', registryIncarnation: `inc_${'a'.repeat(43)}`,
      selectedProviderId: registryProvider.id, providers: [registryProvider] }
  } } })
  capture.realProviders = true
  await mount()
  const editLabel = i18n.t('providerModelEditAction', { ns: 'settings', model: 'model-alpha' })
  const clickButton = async (label: string) => act(async () => {
    const button = [...host.querySelectorAll('button')].find((node) => node.textContent?.trim() === label || node.getAttribute('aria-label') === label)
    expect(button).toBeDefined(); expect(button!.disabled).toBe(false); button!.click()
  })
  const editHighReasoning = async () => {
    await clickButton(editLabel)
    await act(async () => {
      const toggle = [...host.querySelectorAll<HTMLButtonElement>('button[role="switch"]')].find((node) =>
        node.parentElement?.textContent?.startsWith(i18n.t('providerModelReasoningLabel', { ns: 'settings' })))!
      expect(toggle).toBeDefined(); toggle.click()
    })
    await act(async () => {
      const label = [...host.querySelectorAll('label')].find((node) => node.textContent?.startsWith(i18n.t('providerModelReasoningDefault', { ns: 'settings' })))!
      const select = label.querySelector('select')!; select.value = 'high'; select.dispatchEvent(new Event('change', { bubbles: true }))
    })
  }
  await editHighReasoning()
  await clickButton(i18n.t('providerModelSave', { ns: 'settings' }))
  expect(writes).toHaveLength(0)
  expect(stored.provider.modelPreferences ?? []).toEqual([])
  expect(host.textContent).toContain(i18n.t('modelProviderManualSaveHint', { ns: 'settings' }))
  expect(host.textContent).not.toContain(i18n.t('autoApplyHint', { ns: 'settings' }))
  if (reenter) {
    await clickButton(i18n.t('general', { ns: 'settings' }))
    await clickButton(i18n.t('back', { ns: 'settings' }))
    await act(async () => { useChatStore.setState({ settingsSection: 'agents' }) })
    expect(host.textContent).toContain(i18n.t('modelProviderSaveChanges', { ns: 'settings' }))
    expect(useChatStore.getState().showTopNotice).toHaveBeenCalledWith({
      tone: 'info', message: i18n.t('modelProviderUnsavedChanges', { ns: 'settings' })
    })
    expect(writes).toHaveLength(0)
  }
  const failed = { ...deferred(), failure: new Error('Synthetic first Provider Save failure') }; gates.set(1, failed)
  await clickButton(i18n.t('modelProviderSaveChanges', { ns: 'settings' }))
  await act(async () => { failed.resolve() })
  expect(writes).toHaveLength(1)
  expect(stored.provider.modelPreferences ?? []).toEqual([])
  await clickButton(editLabel)
  const toggle = [...host.querySelectorAll<HTMLButtonElement>('button[role="switch"]')].find((node) =>
    node.parentElement?.textContent?.startsWith(i18n.t('providerModelReasoningLabel', { ns: 'settings' })))!
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  await clickButton(i18n.t('providerModelCancel', { ns: 'settings' }))
  await clickButton(i18n.t('modelProviderSaveChanges', { ns: 'settings' }))
  expect(writes).toHaveLength(2)
  expect(stored.provider.modelPreferences?.[0].modelProfiles['model-alpha'].reasoning?.defaultEffort).toBe('high')
  await clickButton(i18n.t('general', { ns: 'settings' }))
  expect(host.querySelector('[data-test-settings-form]')).not.toBeNull()
  if (reenter) {
    await act(async () => { await i18n.changeLanguage('zh') })
    expect(host.querySelector('[data-test-settings-form]')).not.toBeNull()
    await act(async () => { await i18n.changeLanguage('en') })
    expect(host.querySelector('[data-test-settings-form]')).not.toBeNull()
  }
})


it('does not write a stale task catalog while saving an ordinary settings form', async () => {
  await unmount()
  stored = normalizeAppSettings({ ...stored, schedule: { ...stored.schedule, tasks: [{ id: 'host-task', title: 'Host-owned' }] } } as AppSettingsV1)
  rendererRuntimeClient.invalidateSettings()
  await mount()
  await update({ codePromptPrefix: 'Synthetic ordinary edit' })
  await advanceDebounce()
  expect(writes).toHaveLength(1)
  expect(writes[0].schedule).not.toHaveProperty('tasks')
  expect(stored.schedule.tasks[0].id).toBe('host-task')
})

describe('SettingsView Memory workspace ownership', () => {
  let records: CoreMemoryRecordJson[]
  let list: ReturnType<typeof vi.fn>
  let create: ReturnType<typeof vi.fn>
  const memory = (id: string, workspace: string): CoreMemoryRecordJson => ({
    id, content: id, workspace, scope: 'workspace', tags: [], confidence: 1,
    createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z'
  })
  beforeEach(async () => {
    records = []
    list = vi.fn(async ({ workspace }: { workspace?: string }) => records.filter(record => !workspace || record.workspace === workspace))
    create = vi.fn(async (input: { content: string; workspace?: string; scope?: 'user' | 'workspace' | 'project' }) => {
      const record = { ...memory('created', input.workspace ?? ''), content: input.content, scope: input.scope ?? 'workspace' }
      records.push(record); return record
    })
    vi.spyOn(registry, 'getProvider').mockReturnValue({ listMemories: list, createMemory: create } as unknown as AgentProvider)
    await act(async () => useChatStore.setState({ settingsSection: 'memory' }))
  })
  async function workspace(path: string) {
    stored = { ...stored, workspaceRoot: path }
    await act(async () => window.dispatchEvent(new CustomEvent(SETTINGS_CHANGED_EVENT, { detail: structuredClone(stored) })))
  }
  it.each(['user', 'workspace', 'project'] as const)('binds new %s records to the list workspace and retains them after refresh', async scope => {
    let accepted!: boolean
    await act(async () => { accepted = await context().createMemoryRecord({ content: 'synthetic memory', scope }) })
    expect(accepted).toBe(true)
    expect(create).toHaveBeenCalledExactlyOnceWith({ content: 'synthetic memory', scope, workspace: '/synthetic/settings-queue' })
    await act(async () => context().refreshAnalytixDiagnostics())
    expect(context().memoryRecords).toEqual([expect.objectContaining({ content: 'synthetic memory', workspace: '/synthetic/settings-queue', scope })])
    expect(list).toHaveBeenLastCalledWith({ workspace: '/synthetic/settings-queue', includeDeleted: false })
  })
  it('rejects an old list across workspace A to B to A even when the path matches again', async () => {
    const held = deferred()
    list.mockImplementationOnce(async () => { await held.promise; return [memory('old A', '/synthetic/settings-queue')] })
    let pending!: Promise<void>
    await act(async () => { pending = context().refreshAnalytixDiagnostics() })
    records = [memory('new A', '/synthetic/settings-queue'), memory('B', '/synthetic/b')]
    await workspace('/synthetic/b'); expect(context().memoryRecords.map(record => record.id)).toEqual(['B'])
    await workspace('/synthetic/settings-queue'); expect(context().memoryRecords.map(record => record.id)).toEqual(['new A'])
    await act(async () => { held.resolve(); await pending })
    expect(context().memoryRecords.map(record => record.id)).toEqual(['new A'])
  })
  it('keeps an accepted creation when an older same-workspace list returns empty', async () => {
    const held = deferred()
    list.mockImplementationOnce(async () => { await held.promise; return [] })
    let pending!: Promise<void>
    await act(async () => { pending = context().refreshAnalytixDiagnostics() })
    await act(async () => { await context().createMemoryRecord({ content: 'accepted current' }) })
    await act(async () => { held.resolve(); await pending })
    expect(context().memoryRecords).toEqual([expect.objectContaining({ content: 'accepted current' })])
  })
  it('does not inject an accepted old creation into a new workspace', async () => {
    const held = deferred()
    create.mockImplementationOnce(async () => { await held.promise; return memory('accepted A', '/synthetic/settings-queue') })
    let pending!: Promise<boolean>
    await act(async () => { pending = context().createMemoryRecord({ content: 'held' }) })
    records = [memory('B', '/synthetic/b')]; await workspace('/synthetic/b')
    let accepted!: boolean
    await act(async () => { held.resolve(); accepted = await pending })
    expect(accepted).toBe(true)
    expect(context().memoryRecords.map(record => record.id)).toEqual(['B'])
  })
  it('does not surface a retired workspace Memory list error on the current entry', async () => {
    const held = deferred()
    list.mockImplementationOnce(async () => { await held.promise; throw new Error('retired Memory failure') })
    let pending!: Promise<void>
    await act(async () => { pending = context().refreshAnalytixDiagnostics() })
    records = [memory('B', '/synthetic/b')]; await workspace('/synthetic/b')
    await act(async () => { held.resolve(); await pending })
    expect(context().memoryRecords.map(record => record.id)).toEqual(['B'])
    expect(useChatStore.getState().showTopNotice).not.toHaveBeenCalledWith(expect.objectContaining({ message: expect.stringContaining('retired Memory failure') }))
  })
  it('retires a creation when the Memory entry leaves and re-enters', async () => {
    const held = deferred()
    create.mockImplementationOnce(async () => { await held.promise; return memory('retired', '/synthetic/settings-queue') })
    let pending!: Promise<boolean>
    await act(async () => { pending = context().createMemoryRecord({ content: 'held' }) })
    await act(async () => useChatStore.setState({ settingsSection: 'general' }))
    await act(async () => useChatStore.setState({ settingsSection: 'memory' }))
    await act(async () => { held.resolve(); await pending })
    expect(context().memoryRecords).toEqual([])
  })
  it('keeps the existing global failure notice and reports failed creation', async () => {
    create.mockRejectedValueOnce(new Error('synthetic memory unavailable'))
    let accepted!: boolean
    await act(async () => { accepted = await context().createMemoryRecord({ content: 'held' }) })
    expect(accepted).toBe(false)
    expect(useChatStore.getState().showTopNotice).toHaveBeenCalledWith({ tone: 'error', message: 'synthetic memory unavailable' })
    expect(context().memoryRecords).toEqual([])
  })
})
