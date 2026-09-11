import { mkdtemp, readFile, rename, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import type {
  AccountCredentialDraftV1,
  AccountCredentialRequestV1,
  AccountCredentialResultV1,
  AccountCredentialScopeV1,
  AccountCredentialStateV1
} from '../../packages/runtime/src/contracts/provider-registry.js'
import { migrateLegacyImAccountCredentials } from './im-account-lifecycle'
import { ImChannelAccountLifecycle } from './im-channel-lifecycle'
import { JsonSettingsStore } from './settings-store'

const temporaryDirs: string[] = []
const registryIncarnation = `inc_${'i'.repeat(43)}`

function fakeAuthority() {
  let revision = 0
  const entries = new Map<string, { credential: AccountCredentialDraftV1; generation: number; incarnation: string }>()
  const key = (scope: AccountCredentialScopeV1) => JSON.stringify(scope)
  const state = (scope: AccountCredentialScopeV1): AccountCredentialStateV1 => {
    const entry = entries.get(key(scope))
    return {
      schemaVersion: 1, scope, status: entry ? 'ready' : 'absent', registryRevision: String(revision),
      registryIncarnation, providerRevision: entry ? String(entry.generation) : '0',
      providerGeneration: entry ? String(entry.generation) : '0', providerIncarnation: entry?.incarnation ?? ''
    }
  }
  const request = async (input: AccountCredentialRequestV1): Promise<AccountCredentialResultV1> => {
    const current = state(input.scope)
    if (input.operation === 'status') return current
    if (input.expected.registryRevision !== current.registryRevision ||
      input.expected.providerGeneration !== current.providerGeneration) {
      return { schemaVersion: 1, error: { code: 'conflict', message: 'The provider registry state has changed.' } }
    }
    if (input.operation !== 'put') {
      entries.delete(key(input.scope))
      revision++
      return state(input.scope)
    }
    const prior = entries.get(key(input.scope))
    revision++
    entries.set(key(input.scope), {
      credential: structuredClone(input.credential), generation: (prior?.generation ?? 0) + 1,
      incarnation: prior?.incarnation ?? `inc_${'j'.repeat(43)}`
    })
    return state(input.scope)
  }
  return {
    request,
    resolve: async (scope: AccountCredentialScopeV1) => {
      const entry = entries.get(key(scope))
      return entry
        ? { ok: true as const, state: state(scope), credential: structuredClone(entry.credential) }
        : { ok: false as const, code: 'not_found' as const }
    },
    state
  }
}

describe('legacy IM account migration', () => {
  afterEach(async () => {
    await Promise.all(temporaryDirs.splice(0).map((path) => rm(path, { recursive: true, force: true })))
  })

  it('preserves the exact legacy source on interruption and deterministically finalizes after verified K1/K2 readback', async () => {
    const dataDir = await mkdtemp(join(tmpdir(), 'analytix-im-migration-'))
    temporaryDirs.push(dataDir)
    const settingsPath = join(dataDir, 'analytix-settings.json')
    const seed = new JsonSettingsStore(dataDir)
    const defaults = await seed.load()
    const token = 'synthetic-telegram-legacy-token'
    const raw = JSON.parse(await readFile(settingsPath, 'utf8')) as Record<string, any>
    raw.claw.channels = [{
      id: 'legacy-telegram-channel', provider: 'telegram', label: 'Telegram', enabled: true,
      model: 'auto', threadId: '', workspaceRoot: '', agentProfile: {
        name: 'analytix', description: '', identity: '', personality: '', userContext: '', replyRules: ''
      }, conversations: [], createdAt: '2026-08-30T00:00:00.000Z', updatedAt: '2026-08-30T00:00:00.000Z',
      platformCredential: {
        kind: 'telegram', botToken: token, allowedChatIds: '1001',
        credentialRef: 'must-not-survive', createdAt: '2026-08-30T00:00:00.000Z'
      }
    }]
    const source = `${JSON.stringify(raw, null, 2)}\n`
    await writeFile(settingsPath, source, { mode: 0o600 })
    const authority = fakeAuthority()
    await expect(migrateLegacyImAccountCredentials({
      store: new JsonSettingsStore(dataDir),
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      crash: { afterCredentialCommit: () => { throw new Error('synthetic crash') } }
    })).rejects.toThrow('synthetic crash')
    expect(await readFile(settingsPath, 'utf8')).toBe(source)

    await migrateLegacyImAccountCredentials({
      store: new JsonSettingsStore(dataDir),
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve
    })
    const persisted = await readFile(settingsPath, 'utf8')
    const snapshot = await new JsonSettingsStore(dataDir).load()
    expect(persisted).not.toContain(token)
    expect(persisted).not.toContain('credentialRef')
    expect(snapshot.claw.channels[0]?.platformAccount).toMatchObject({
      kind: 'telegram', accountId: 'legacy-telegram-channel', allowedChatIds: '1001'
    })
    expect(defaults.claw.channels).toEqual([])
  })

  it('rejects an identical-byte replacement of the authenticated canonical legacy source', async () => {
    const dataDir = await mkdtemp(join(tmpdir(), 'analytix-im-migration-source-identity-'))
    temporaryDirs.push(dataDir)
    const settingsPath = join(dataDir, 'analytix-settings.json')
    const store = new JsonSettingsStore(dataDir)
    await store.load()
    const raw = JSON.parse(await readFile(settingsPath, 'utf8')) as Record<string, any>
    raw.claw.channels = [{
      id: 'legacy-source-channel', provider: 'telegram', label: 'Telegram', enabled: true,
      model: 'auto', threadId: '', workspaceRoot: '', agentProfile: {
        name: 'analytix', description: '', identity: '', personality: '', userContext: '', replyRules: ''
      }, conversations: [], createdAt: '2026-08-30T00:00:00.000Z', updatedAt: '2026-08-30T00:00:00.000Z',
      platformCredential: {
        kind: 'telegram', botToken: 'synthetic-source-identity-token', allowedChatIds: '',
        createdAt: '2026-08-30T00:00:00.000Z'
      }
    }]
    const source = `${JSON.stringify(raw, null, 2)}\n`
    await writeFile(settingsPath, source, { mode: 0o600 })
    const inspection = await new JsonSettingsStore(dataDir).inspectLegacyImAccountCredentials()
    expect(inspection).not.toBeNull()
    const replacementPath = join(dataDir, 'replacement-settings.json')
    await writeFile(replacementPath, source, { mode: 0o600 })
    await rename(replacementPath, settingsPath)

    await expect(new JsonSettingsStore(dataDir).finalizeLegacyImAccountCredentialMigration(inspection!))
      .rejects.toThrow('Legacy IM plaintext persistence is not permitted.')
    expect(await readFile(settingsPath, 'utf8')).toBe(source)
  })

  it('recovers connect and disconnect at one documented settings/credential terminal state', async () => {
    const dataDir = await mkdtemp(join(tmpdir(), 'analytix-im-channel-lifecycle-'))
    temporaryDirs.push(dataDir)
    const store = new JsonSettingsStore(dataDir)
    await store.load()
    const authority = fakeAuthority()
    const scope = {
      owner: 'transport' as const, provider: 'telegram', accountId: 'account-a',
      channelId: 'channel-a', purpose: 'transport-telegram-bot-token' as const
    }
    const channel = {
      id: 'channel-a', provider: 'telegram' as const, label: 'Telegram', enabled: true,
      model: 'auto' as const, threadId: '', workspaceRoot: '', agentProfile: {
        name: 'analytix', description: '', identity: '', personality: '', userContext: '', replyRules: ''
      }, conversations: [], platformAccount: {
        kind: 'telegram' as const, accountId: 'account-a', allowedChatIds: '',
        createdAt: '2026-08-30T00:00:00.000Z'
      }, createdAt: '2026-08-30T00:00:00.000Z', updatedAt: '2026-08-30T00:00:00.000Z'
    }
    const interruptedConnect = new ImChannelAccountLifecycle({
      dataDir, store, requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      crash: { afterCredentialMutation: () => { throw new Error('connect crash') } }
    })
    await expect(interruptedConnect.connect({
      channel, scope, credential: { kind: 'telegram', botToken: 'synthetic-connect-token', allowedChatIds: '' }
    })).rejects.toThrow('connect crash')
    expect((await store.load()).claw.channels).toEqual([])
    expect(await readFile(join(dataDir, '.analytix-im-account-lifecycle-v1.json'), 'utf8'))
      .not.toMatch(/synthetic-connect-token|credentialRef|botToken/)

    const restarted = new ImChannelAccountLifecycle({
      dataDir, store, requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve
    })
    await restarted.recover()
    expect((await store.load()).claw.channels.map((item) => item.id)).toEqual(['channel-a'])
    await restarted.recover()

    const interruptedDisconnect = new ImChannelAccountLifecycle({
      dataDir, store, requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      crash: { afterCredentialMutation: () => { throw new Error('disconnect crash') } }
    })
    await expect(interruptedDisconnect.disconnect('channel-a', scope)).rejects.toThrow('disconnect crash')
    expect((await store.load()).claw.channels.map((item) => item.id)).toEqual(['channel-a'])
    await restarted.recover()
    expect((await store.load()).claw.channels).toEqual([])
    await restarted.recover()

    const unrecordedScope = { ...scope, accountId: 'account-b', channelId: 'channel-b' }
    const unrecordedChannel = {
      ...channel, id: 'channel-b',
      platformAccount: { ...channel.platformAccount, accountId: 'account-b' }
    }
    const interruptedBeforeFence = new ImChannelAccountLifecycle({
      dataDir, store, requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      crash: { afterCredentialPut: () => { throw new Error('pre-fence crash') } }
    })
    await expect(interruptedBeforeFence.connect({
      channel: unrecordedChannel, scope: unrecordedScope,
      credential: { kind: 'telegram', botToken: 'synthetic-unrecorded-token', allowedChatIds: '' }
    })).rejects.toThrow('pre-fence crash')
    expect(authority.state(unrecordedScope).status).toBe('ready')
    expect((await store.load()).claw.channels).toEqual([])
    const preFenceRestart = new ImChannelAccountLifecycle({
      dataDir, store, requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve
    })
    await preFenceRestart.recover()
    expect(authority.state(unrecordedScope).status).toBe('absent')
    expect((await store.load()).claw.channels).toEqual([])
    await preFenceRestart.recover()

    const winnerScope = { ...scope, accountId: 'account-c', channelId: 'channel-c' }
    const winnerChannel = {
      ...channel, id: 'channel-c',
      platformAccount: { ...channel.platformAccount, accountId: 'account-c' }
    }
    const interruptedBeforeWinner = new ImChannelAccountLifecycle({
      dataDir, store, requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      crash: { afterCredentialPut: () => { throw new Error('pre-winner crash') } }
    })
    await expect(interruptedBeforeWinner.connect({
      channel: winnerChannel, scope: winnerScope,
      credential: { kind: 'telegram', botToken: 'synthetic-old-candidate', allowedChatIds: '' }
    })).rejects.toThrow('pre-winner crash')
    const oldCandidate = authority.state(winnerScope)
    await authority.request({
      schemaVersion: 1, operation: 'put', scope: winnerScope,
      expected: {
        registryRevision: oldCandidate.registryRevision,
        registryIncarnation: oldCandidate.registryIncarnation,
        providerRevision: oldCandidate.providerRevision,
        providerGeneration: oldCandidate.providerGeneration,
        providerIncarnation: oldCandidate.providerIncarnation,
        providerCredentialPurpose: winnerScope.purpose
      },
      credential: { kind: 'telegram', botToken: 'synthetic-newer-winner', allowedChatIds: '' }
    })
    await preFenceRestart.recover()
    expect(authority.state(winnerScope)).toMatchObject({ status: 'ready', providerGeneration: '2' })
    expect((await store.load()).claw.channels).toEqual([])
  })
})
