// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsPatch, type AppSettingsV1 } from '@shared/app-settings'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { useChatStore, type SettingsRouteSection } from '../store/chat-store'
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

async function advanceFormerAutosaveWindow() {
  await act(async () => { await vi.advanceTimersByTimeAsync(450) })
}

async function update(patch: AppSettingsPatch) {
  await act(async () => { context().update(patch) })
}

function button(label: string, scope: ParentNode = host): HTMLButtonElement {
  const found = [...scope.querySelectorAll<HTMLButtonElement>('button')].find((node) =>
    node.textContent?.trim() === label || node.getAttribute('aria-label') === label)
  expect(found).toBeDefined()
  return found!
}

async function clickButton(label: string, scope: ParentNode = host) {
  await act(async () => {
    const target = button(label, scope)
    expect(target.disabled).toBe(false)
    target.click()
  })
}

async function savePreferences() {
  await clickButton(i18n.t('settingsSavePreferences', { ns: 'settings' }), host.querySelector('footer')!)
}

async function selectSection(section: SettingsRouteSection) {
  await act(async () => { useChatStore.getState().openSettings(section) })
}

async function beginPreferenceSave(patch = preferencePatch()) {
  let pending!: Promise<AppSettingsV1>
  await act(async () => {
    pending = context().saveModelPreferences(patch)
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
    route: 'settings',
    settingsReturnRoute: 'chat',
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

describe('SettingsView explicit settings and model preference save queue', () => {
  it('keeps ordinary edits local until footer Save, including after a model preference commit', async () => {
    const acceptedPrefix = stored.codePromptPrefix
    await update({ codePromptPrefix: 'Pending ordinary draft' })
    await advanceFormerAutosaveWindow()
    expect(writes).toHaveLength(0)

    const { pending } = await beginPreferenceSave()
    let saved!: AppSettingsV1
    await act(async () => { saved = await pending })
    expect(writes).toEqual([preferencePatch()])
    expect(saved.codePromptPrefix).toBe(acceptedPrefix)
    expect(saved.provider.modelPreferences).toEqual([preference])
    expect(context().form.codePromptPrefix).toBe('Pending ordinary draft')
    expect(context().form.provider.modelPreferences).toEqual([preference])
    expect(events).toEqual([saved])
    expect(await rendererRuntimeClient.getSettings({ forceRefresh: true })).toEqual(saved)
    await advanceFormerAutosaveWindow()
    expect(writes).toHaveLength(1)
    expect(host.textContent).toContain(i18n.t('settingsUnsavedChanges', { ns: 'settings' }))

    await savePreferences()
    expect(writes).toHaveLength(2)
    expect(writes[1]).toEqual({ codePromptPrefix: 'Pending ordinary draft' })
    expect(stored.codePromptPrefix).toBe('Pending ordinary draft')
    expect(stored.provider.modelPreferences).toEqual([preference])
    expect(button(i18n.t('settingsSavePreferences', { ns: 'settings' })).disabled).toBe(true)
  })

  it('drains an older explicit ordinary write before saving only the preference patch', async () => {
    const older = deferred()
    gates.set(1, older)
    await update({ codePromptPrefix: 'Issued ordinary draft' })
    await savePreferences()
    expect(writes).toHaveLength(1)
    expect(writes[0]).toEqual({ codePromptPrefix: 'Issued ordinary draft' })

    const { pending } = await beginPreferenceSave()
    let settled = false
    void pending.then(() => { settled = true }, () => { settled = true })
    // A newer draft created while the Host write is pending must remain local.
    await update({ codePromptPrefix: 'Latest settings draft' })
    expect(writes).toHaveLength(1)
    expect(settled).toBe(false)

    let saved!: AppSettingsV1
    await act(async () => {
      older.resolve()
      saved = await pending
    })
    expect(writes).toHaveLength(2)
    expect(writes[1]).toEqual(preferencePatch())
    expect(saved.codePromptPrefix).toBe('Issued ordinary draft')
    expect(saved.provider.modelPreferences).toEqual([preference])
    expect(context().form.codePromptPrefix).toBe('Latest settings draft')
    expect(context().form.provider.modelPreferences).toEqual([preference])
    expect(await rendererRuntimeClient.getSettings({ forceRefresh: true })).toEqual(saved)
    await advanceFormerAutosaveWindow()
    expect(writes).toHaveLength(2)

    await savePreferences()
    expect(writes).toHaveLength(3)
    expect(writes[2]).toEqual({ codePromptPrefix: 'Latest settings draft' })
    expect(stored.codePromptPrefix).toBe('Latest settings draft')
    expect(stored.provider.modelPreferences).toEqual([preference])
  })

  it('rejects a failed model preference write and allows a subsequent explicit retry', async () => {
    const acceptedPrefix = stored.codePromptPrefix
    await update({ codePromptPrefix: 'Retained ordinary draft' })
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
    expect(context().form.codePromptPrefix).toBe('Retained ordinary draft')

    const retry = await beginPreferenceSave()
    let saved!: AppSettingsV1
    await act(async () => { saved = await retry.pending })
    expect(writes).toEqual([preferencePatch(), preferencePatch()])
    expect(saved.codePromptPrefix).toBe(acceptedPrefix)
    expect(saved.provider.modelPreferences).toEqual([preference])
    expect(context().form.codePromptPrefix).toBe('Retained ordinary draft')
    expect((await rendererRuntimeClient.getSettings({ forceRefresh: true })).provider.modelPreferences)
      .toEqual([preference])
  })

  it('drains a failed ordinary write without poisoning the queued preference save or footer retry', async () => {
    const acceptedPrefix = stored.codePromptPrefix
    const older = { ...deferred(), failure: new Error('Synthetic older save failure') }
    gates.set(1, older)
    await update({ codePromptPrefix: 'Retain this ordinary edit' })
    await savePreferences()
    const { pending } = await beginPreferenceSave()
    expect(writes).toHaveLength(1)
    let saved!: AppSettingsV1
    await act(async () => {
      older.resolve()
      saved = await pending
    })
    expect(writes).toHaveLength(2)
    expect(writes[1]).toEqual(preferencePatch())
    expect(saved.codePromptPrefix).toBe(acceptedPrefix)
    expect(saved.provider.modelPreferences).toEqual([preference])
    expect(context().form.codePromptPrefix).toBe('Retain this ordinary edit')
    await savePreferences()
    expect(writes).toHaveLength(3)
    expect(writes[2]).toEqual({ codePromptPrefix: 'Retain this ordinary edit' })
    expect(stored.codePromptPrefix).toBe('Retain this ordinary edit')
    expect(stored.provider.modelPreferences).toEqual([preference])
  })

  it('discards ordinary drafts without an autosave or leave flush after Cancel', async () => {
    const acceptedPrefix = stored.codePromptPrefix
    await update({ codePromptPrefix: 'Discard this ordinary edit' })
    await clickButton(i18n.t('settingsCancel', { ns: 'settings' }), host.querySelector('footer')!)
    expect(useChatStore.getState().route).toBe('chat')
    expect(writes).toHaveLength(0)
    await unmount()
    await advanceFormerAutosaveWindow()
    expect(writes).toHaveLength(0)
    expect(stored.codePromptPrefix).toBe(acceptedPrefix)

    await selectSection('providers')
    await mount()
    expect(context().form.codePromptPrefix).toBe(acceptedPrefix)
    expect(events).toHaveLength(0)
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
      await pending
    })
    // A write already handed to Main may finish, but its old component must
    // not emit a settings-change event or replace the new entry's draft.
    expect(events).toHaveLength(before)
    expect(context().form.codePromptPrefix).toBe('New entry draft')
    expect(context().form.provider.modelPreferences ?? []).toEqual([])
  })

  it('retires an undispatched queued preference write when its settings entry unmounts', async () => {
    const older = deferred()
    gates.set(1, older)
    const first = await beginPreferenceSave()
    const queued = await beginPreferenceSave({ provider: { modelPreferences: [] } })
    const rejection = expect(queued.pending).rejects.toThrow('Settings entry is no longer current.')
    expect(writes).toHaveLength(1)
    await unmount()
    await mount()
    await update({ codePromptPrefix: 'Reopened local draft' })
    await act(async () => {
      older.resolve()
      await first.pending
      await rejection
    })
    await advanceFormerAutosaveWindow()
    expect(writes).toEqual([preferencePatch()])
    expect(stored.provider.modelPreferences).toEqual([preference])
    expect(context().form.codePromptPrefix).toBe('Reopened local draft')
    expect(context().form.provider.modelPreferences ?? []).toEqual([])
    expect(events).toHaveLength(0)
  })
})

it.each([false, true])('keeps unapplied model preferences local, guards departure, and retries footer Save %s', async (reenter) => {
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
  expect([...host.querySelectorAll('button')].some((node) =>
    node.textContent?.trim() === i18n.t('modelProviderSaveChanges', { ns: 'settings' }))).toBe(false)
  if (reenter) {
    await clickButton(i18n.t('settingsTabGeneral', { ns: 'settings' }))
    expect(useChatStore.getState().settingsSection).toBe('providers')
    await clickButton(i18n.t('settingsClose', { ns: 'settings' }))
    expect(host.textContent).toContain(i18n.t('settingsDiscardTitle', { ns: 'settings' }))
    expect(useChatStore.getState().route).toBe('settings')
    await clickButton(i18n.t('settingsKeepEditing', { ns: 'settings' }))
    await selectSection('agents')
    expect(useChatStore.getState().settingsSection).toBe('providers')
    expect(button(i18n.t('settingsSavePreferences', { ns: 'settings' })).disabled).toBe(false)
    expect(useChatStore.getState().showTopNotice).toHaveBeenCalledWith({
      tone: 'info', message: i18n.t('modelProviderUnsavedChanges', { ns: 'settings' })
    })
    expect(writes).toHaveLength(0)
  }
  const failed = { ...deferred(), failure: new Error('Synthetic first Provider Save failure') }; gates.set(1, failed)
  await savePreferences()
  await act(async () => { failed.resolve() })
  expect(writes).toHaveLength(1)
  expect(stored.provider.modelPreferences ?? []).toEqual([])
  await clickButton(editLabel)
  const toggle = [...host.querySelectorAll<HTMLButtonElement>('button[role="switch"]')].find((node) =>
    node.parentElement?.textContent?.startsWith(i18n.t('providerModelReasoningLabel', { ns: 'settings' })))!
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  await clickButton(i18n.t('providerModelCancel', { ns: 'settings' }))
  await savePreferences()
  expect(writes).toHaveLength(2)
  expect(stored.provider.modelPreferences?.[0].modelProfiles['model-alpha'].reasoning?.defaultEffort).toBe('high')
  await clickButton(i18n.t('settingsTabGeneral', { ns: 'settings' }))
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
  await advanceFormerAutosaveWindow()
  expect(writes).toHaveLength(0)
  await savePreferences()
  expect(writes).toHaveLength(1)
  expect(writes[0]).toEqual({ codePromptPrefix: 'Synthetic ordinary edit' })
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
    await selectSection('memory')
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
    await selectSection('general')
    await selectSection('memory')
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
