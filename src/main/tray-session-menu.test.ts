import { describe, expect, it, vi } from 'vitest'
import {
  buildTrayMenuTemplate,
  currentTrayProviderObservation,
  parseTrayThreads,
  type TrayThreadSummary
} from './tray-session-menu'

describe('tray session menu', () => {
  it('parses, filters, and sorts active thread summaries', () => {
    expect(parseTrayThreads(JSON.stringify({ threads: [
      thread('old', 'idle', '2026-06-01T00:00:00.000Z'),
      thread('deleted', 'deleted', '2026-06-03T00:00:00.000Z'),
      thread('archived', 'archived', '2026-06-04T00:00:00.000Z'),
      thread('new', 'running', '2026-06-02T00:00:00.000Z')
    ] })).map((item) => item.id)).toEqual(['new', 'old'])

    expect(parseTrayThreads('not json')).toEqual([])
    expect(parseTrayThreads(JSON.stringify({ threads: [{ title: 'missing id' }] }))).toEqual([])
  })

  it('groups running and recent threads with workspace labels', () => {
    const actions = fakeActions()
    const menu = buildTrayMenuTemplate({
      locale: 'en',
      threads: [
        thread('run', 'running', '2026-06-02T00:00:00.000Z', 'Fix tests', 'C:\\work\\analytix'),
        thread('recent', 'idle', '2026-06-01T00:00:00.000Z', 'Review PR', '/work/docs')
      ],
      actions
    })

    expect(menu.map((item) => item.label).filter(Boolean)).toEqual([
      'Running',
      'Fix tests',
      'Recent',
      'Review PR',
      'New Chat',
      'Open Analytix',
      'Quit'
    ])
    expect(menu.find((item) => item.label === 'Fix tests')?.sublabel).toBe('analytix')
    expect(menu.find((item) => item.label === 'Review PR')?.sublabel).toBe('docs')
    menu.find((item) => item.label === 'Fix tests')?.click?.({} as never, undefined, {} as never)
    expect(actions.openThread).toHaveBeenCalledWith('run')
  })

  it('moves overflow sessions into More and localizes actions without legacy identity', () => {
    const actions = fakeActions()
    const threads = Array.from({ length: 7 }, (_, index) =>
      thread(`thread-${index}`, 'idle', `2026-06-${String(10 - index).padStart(2, '0')}T00:00:00.000Z`, '')
    )
    const menu = buildTrayMenuTemplate({ locale: 'zh', threads, actions })
    const more = menu.find((item) => item.label === '更多')
    const labels = JSON.stringify(menu)

    expect(Array.isArray(more?.submenu) ? more.submenu : []).toHaveLength(2)
    expect(menu.map((item) => item.label).filter(Boolean)).toEqual([
      '最近会话',
      '未命名会话',
      '未命名会话',
      '未命名会话',
      '未命名会话',
      '未命名会话',
      '更多',
      '新建会话',
      '打开 Analytix',
      '退出'
    ])
    expect(labels).not.toContain('Kun')
  })

  it('keeps the fallback menu useful when no sessions are available', () => {
    const menu = buildTrayMenuTemplate({ locale: 'en', threads: [], actions: fakeActions() })
    expect(menu.map((item) => item.label).filter(Boolean)).toEqual([
      'New Chat',
      'Open Analytix',
      'Quit'
    ])
  })

  it('shows only the bounded on-demand Provider observation without Hub identity or secret state', () => {
    const menu = buildTrayMenuTemplate({
      locale: 'en',
      threads: [],
      providerObservation: {
        providerId: 'provider-local',
        status: 'available',
        observedAt: '2026-08-31T00:00:00Z',
        expiresAt: '2026-08-31T00:05:00Z',
        quota: 1000,
        usage: 125,
        remaining: 875
      },
      actions: fakeActions()
    })
    expect(menu[0]).toEqual(expect.objectContaining({
      label: 'Provider quota: provider-local',
      sublabel: '875 / 1,000',
      enabled: false
    }))
    const serialized = JSON.stringify(menu)
    for (const forbidden of ['Hub', 'accountId', 'credentialRef', 'token', 'endpoint']) {
      expect(serialized).not.toContain(forbidden)
    }
  })

  it('publishes a tray observation only after a complete current readback and before expiry', () => {
    const binding = {
      schemaVersion: 1 as const,
      endpoint: 'https://quota.provider.invalid/observation',
      method: 'GET' as const,
      projection: 'normalized-quota-v1' as const
    }
    const provider = {
      id: 'provider-local', kind: 'openai-compatible', endpoint: 'https://provider.invalid/v1',
      models: ['model-local'], mediaModels: [], selectedModel: 'model-local', selectedRoutes: ['primary'],
      credentialConfigured: true, credentialPurpose: 'provider-api-key' as const,
      revision: '7', generation: '3', incarnation: `inc_${'b'.repeat(43)}`, tombstone: false,
      accountObservation: binding
    }
    const snapshot = {
      schemaVersion: 1 as const,
      registryRevision: '9', registryIncarnation: `inc_${'a'.repeat(43)}`,
      selectedProviderId: provider.id, providers: [provider]
    }
    const observation = {
      schemaVersion: 1 as const,
      registryRevision: snapshot.registryRevision,
      registryIncarnation: snapshot.registryIncarnation,
      providerId: provider.id,
      providerRevision: provider.revision,
      providerGeneration: provider.generation,
      providerIncarnation: provider.incarnation,
      providerCredentialPurpose: provider.credentialPurpose,
      status: 'available' as const,
      observedAt: '2026-08-31T00:00:00Z',
      expiresAt: '2026-08-31T00:05:00Z',
      quota: 100,
      remaining: 80
    }
    const current = () => currentTrayProviderObservation({
      expectedSnapshot: snapshot,
      expectedProvider: provider,
      observation,
      currentSnapshot: snapshot,
      nowMs: Date.parse('2026-08-31T00:01:00Z')
    })
    expect(current()).toEqual(expect.objectContaining({ providerId: provider.id, quota: 100, remaining: 80 }))
    expect(currentTrayProviderObservation({
      expectedSnapshot: snapshot,
      expectedProvider: provider,
      observation: { ...observation, providerCredentialPurpose: 'provider-oauth-token-bundle' },
      currentSnapshot: snapshot,
      nowMs: Date.parse('2026-08-31T00:01:00Z')
    })).toBeNull()

    for (const currentSnapshot of [
      { ...snapshot, selectedProviderId: 'provider-other' },
      { ...snapshot, registryRevision: '10' },
      { ...snapshot, registryIncarnation: `inc_${'c'.repeat(43)}` },
      { ...snapshot, providers: [{ ...provider, revision: '8' }] },
      { ...snapshot, providers: [{ ...provider, generation: '4' }] },
      { ...snapshot, providers: [{ ...provider, incarnation: `inc_${'d'.repeat(43)}` }] },
      { ...snapshot, providers: [{ ...provider, credentialPurpose: 'provider-oauth-token-bundle' as const }] },
      { ...snapshot, providers: [{ ...provider, tombstone: true, credentialConfigured: false }] },
      { ...snapshot, providers: [{ ...provider, accountObservation: { ...binding, endpoint: 'https://changed.invalid/quota' } }] }
    ]) {
      expect(currentTrayProviderObservation({
        expectedSnapshot: snapshot,
        expectedProvider: provider,
        observation,
        currentSnapshot,
        nowMs: Date.parse('2026-08-31T00:01:00Z')
      })).toBeNull()
    }
    expect(currentTrayProviderObservation({
      expectedSnapshot: snapshot,
      expectedProvider: provider,
      observation,
      currentSnapshot: snapshot,
      nowMs: Date.parse(observation.expiresAt)
    })).toBeNull()
  })
})

function thread(
  id: string,
  status: string,
  updatedAt: string,
  title = id,
  workspace = '/work/project'
): TrayThreadSummary {
  return { id, title, workspace, status, updatedAt }
}

function fakeActions() {
  return {
    openThread: vi.fn(),
    newChat: vi.fn(),
    openApp: vi.fn(),
    quit: vi.fn()
  }
}
