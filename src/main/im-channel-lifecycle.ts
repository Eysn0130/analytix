import { unlink } from 'node:fs/promises'
import { join } from 'node:path'
import { atomicWriteFile } from '../../packages/runtime/src/adapters/file/atomic-write.js'
import type {
  AccountCredentialDraftV1,
  AccountCredentialRequestV1,
  AccountCredentialResultV1,
  AccountCredentialScopeV1,
  AccountCredentialStateV1,
  ProviderRegistryExpectedStateV1
} from '../../packages/runtime/src/contracts/provider-registry.js'
import type { AppSettingsV1, ClawImChannelV1 } from '../shared/app-settings'
import {
  accountCredentialExpectedState,
  type MainAccountCredentialResolution
} from './ipc/provider-registry-ipc'
import type { JsonSettingsStore } from './settings-store'

type Journal = {
  schemaVersion: 1
  operation: 'connect' | 'disconnect'
  phase: 'prepared' | 'credential-mutated'
  channel: ClawImChannelV1
  scope: AccountCredentialScopeV1
  expected: ProviderRegistryExpectedStateV1
  committed?: ProviderRegistryExpectedStateV1
}

type ImChannelLifecycleDeps = {
  dataDir: string
  store: Pick<JsonSettingsStore, 'load' | 'patch'>
  requestAccountCredential: (request: AccountCredentialRequestV1) => Promise<AccountCredentialResultV1>
  resolveAccountCredential: (scope: AccountCredentialScopeV1) => Promise<MainAccountCredentialResolution>
  onCredentialInvalidated?: (scope: AccountCredentialScopeV1) => Promise<void>
  crash?: {
    afterCredentialPut?: () => Promise<void> | void
    afterCredentialMutation?: () => Promise<void> | void
    afterSettingsSave?: () => Promise<void> | void
  }
}

const JOURNAL_FILE = '.analytix-im-account-lifecycle-v1.json'

function sameFence(state: AccountCredentialStateV1, expected: ProviderRegistryExpectedStateV1): boolean {
  return state.registryIncarnation === expected.registryIncarnation &&
    state.providerRevision === expected.providerRevision &&
    state.providerGeneration === expected.providerGeneration &&
    state.providerIncarnation === expected.providerIncarnation &&
    (state.status === 'ready' ? state.scope.purpose : '') === expected.providerCredentialPurpose
}

function isFailure(
  result: AccountCredentialResultV1
): result is Extract<AccountCredentialResultV1, { error: unknown }> {
  return 'error' in result
}

function exactCredentialReadback(
  scope: AccountCredentialScopeV1,
  credential: AccountCredentialDraftV1,
  committed: AccountCredentialStateV1,
  resolution: MainAccountCredentialResolution
): boolean {
  return resolution.ok && sameFence(resolution.state, accountCredentialExpectedState(committed)) &&
    JSON.stringify(resolution.state.scope) === JSON.stringify(scope) &&
    JSON.stringify(resolution.credential) === JSON.stringify(credential)
}

function validJournal(value: unknown): value is Journal {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const journal = value as Journal
  return journal.schemaVersion === 1 && ['connect', 'disconnect'].includes(journal.operation) &&
    ['prepared', 'credential-mutated'].includes(journal.phase) && Boolean(journal.channel?.id) &&
    journal.scope?.owner === 'transport' && Boolean(journal.expected?.registryIncarnation)
}

export class ImChannelAccountLifecycle {
  private readonly journalPath: string
  private queue: Promise<unknown> = Promise.resolve()

  constructor(private readonly deps: ImChannelLifecycleDeps) {
    this.journalPath = join(deps.dataDir, JOURNAL_FILE)
  }

  private exclusive<T>(action: () => Promise<T>): Promise<T> {
    const run = this.queue.then(action, action)
    this.queue = run.then(() => undefined, () => undefined)
    return run
  }

  private async write(journal: Journal): Promise<void> {
    await atomicWriteFile(this.journalPath, `${JSON.stringify(journal, null, 2)}\n`, { mode: 0o600 })
  }

  private async clear(): Promise<void> {
    await unlink(this.journalPath).catch((error: NodeJS.ErrnoException) => {
      if (error.code !== 'ENOENT') throw error
    })
  }

  private async status(scope: AccountCredentialScopeV1): Promise<AccountCredentialStateV1> {
    const result = await this.deps.requestAccountCredential({ schemaVersion: 1, operation: 'status', scope })
    if (isFailure(result)) throw new Error('IM account lifecycle authority is unavailable.')
    return result
  }

  private async saveConnectedChannel(channel: ClawImChannelV1): Promise<AppSettingsV1> {
    const current = await this.deps.store.load()
    const duplicate = current.claw.channels.find((item) => item.id !== channel.id && item.provider === channel.provider)
    if (duplicate) throw new Error('An IM channel for this provider is already connected.')
    const channels = current.claw.channels.some((item) => item.id === channel.id)
      ? current.claw.channels.map((item) => item.id === channel.id ? channel : item)
      : [...current.claw.channels, channel]
    const saved = await this.deps.store.patch({
      claw: { enabled: true, im: { enabled: true, provider: channel.provider }, channels }
    })
    const verified = saved.claw.channels.find((item) => item.id === channel.id)
    if (!verified || JSON.stringify(verified.platformAccount) !== JSON.stringify(channel.platformAccount)) {
      throw new Error('IM channel settings could not be verified.')
    }
    return saved
  }

  async connect(input: {
    channel: ClawImChannelV1
    scope: AccountCredentialScopeV1
    credential: AccountCredentialDraftV1
  }): Promise<AppSettingsV1> {
    return this.exclusive(async () => {
      await this.recoverNoLock()
      const status = await this.status(input.scope)
      if (status.status !== 'absent') throw new Error('IM account credential is already configured.')
      let journal: Journal = {
        schemaVersion: 1, operation: 'connect', phase: 'prepared', channel: input.channel,
        scope: input.scope, expected: accountCredentialExpectedState(status)
      }
      await this.write(journal)
      const committed = await this.deps.requestAccountCredential({
        schemaVersion: 1, operation: 'put', scope: input.scope,
        expected: journal.expected, credential: input.credential
      })
      if (isFailure(committed) || committed.status !== 'ready') {
        await this.clear()
        throw new Error('IM account credential could not be committed.')
      }
      await this.deps.crash?.afterCredentialPut?.()
      const readback = await this.deps.resolveAccountCredential(input.scope)
      if (!exactCredentialReadback(input.scope, input.credential, committed, readback)) {
        const cleaned = await this.deps.requestAccountCredential({
          schemaVersion: 1, operation: 'delete', scope: input.scope,
          expected: accountCredentialExpectedState(committed)
        })
        if (!isFailure(cleaned) && cleaned.status === 'absent') await this.clear()
        throw new Error('IM account credential could not be verified.')
      }
      journal = { ...journal, phase: 'credential-mutated', committed: accountCredentialExpectedState(committed) }
      await this.write(journal)
      await this.deps.crash?.afterCredentialMutation?.()
      const saved = await this.saveConnectedChannel(input.channel)
      await this.deps.crash?.afterSettingsSave?.()
      await this.clear()
      return saved
    })
  }

  async disconnect(channelId: string, scope: AccountCredentialScopeV1): Promise<AppSettingsV1> {
    return this.exclusive(async () => {
      await this.recoverNoLock()
      const current = await this.deps.store.load()
      const channel = current.claw.channels.find((item) => item.id === channelId)
      if (!channel) return current
      const status = await this.status(scope)
      let journal: Journal = {
        schemaVersion: 1, operation: 'disconnect', phase: 'prepared', channel, scope,
        expected: accountCredentialExpectedState(status)
      }
      await this.write(journal)
      if (status.status !== 'absent') {
        const deleted = await this.deps.requestAccountCredential({
          schemaVersion: 1, operation: 'delete', scope, expected: journal.expected
        })
        if (isFailure(deleted) || deleted.status !== 'absent') {
          await this.clear()
          throw new Error('IM account credential could not be deleted.')
        }
      }
      await this.deps.onCredentialInvalidated?.(scope)
      journal = { ...journal, phase: 'credential-mutated' }
      await this.write(journal)
      await this.deps.crash?.afterCredentialMutation?.()
      const saved = await this.deps.store.patch({
        claw: { channels: current.claw.channels.filter((item) => item.id !== channelId) }
      })
      if (saved.claw.channels.some((item) => item.id === channelId)) {
        throw new Error('IM channel disconnect could not be verified.')
      }
      await this.deps.crash?.afterSettingsSave?.()
      await this.clear()
      return saved
    })
  }

  async recover(): Promise<void> {
    await this.exclusive(() => this.recoverNoLock())
  }

  private async recoverNoLock(): Promise<void> {
    let journal: Journal
    try {
      const { readFile } = await import('node:fs/promises')
      const parsed = JSON.parse(await readFile(this.journalPath, 'utf8')) as unknown
      if (!validJournal(parsed)) throw new Error()
      journal = parsed
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') return
      throw new Error('IM account lifecycle recovery failed.')
    }
    const status = await this.status(journal.scope)
    if (journal.operation === 'connect') {
      if (journal.phase === 'prepared') {
        if (status.status === 'absent') {
          await this.clear()
          return
        }
        const exactUnrecordedCandidate = status.status === 'ready' &&
          journal.expected.providerRevision === '0' && journal.expected.providerGeneration === '0' &&
          journal.expected.providerIncarnation === '' &&
          status.registryIncarnation === journal.expected.registryIncarnation &&
          status.providerRevision === '1' && status.providerGeneration === '1'
        if (!exactUnrecordedCandidate) {
          // A later explicit mutation owns the slot. Never delete or project
          // that winner while recovering an older unrecorded candidate.
          await this.clear()
          return
        }
        const cleaned = await this.deps.requestAccountCredential({
          schemaVersion: 1, operation: 'delete', scope: journal.scope,
          expected: accountCredentialExpectedState(status)
        })
        if (isFailure(cleaned) || cleaned.status !== 'absent') {
          throw new Error('IM account lifecycle recovery failed.')
        }
        await this.clear()
        return
      }
      if (!journal.committed || status.status !== 'ready' || !sameFence(status, journal.committed)) {
        await this.clear()
        return
      }
      const readback = await this.deps.resolveAccountCredential(journal.scope)
      if (!readback.ok || !sameFence(readback.state, journal.committed)) {
        throw new Error('IM account lifecycle recovery failed.')
      }
      await this.saveConnectedChannel(journal.channel)
      await this.clear()
      return
    }
    if (journal.phase === 'prepared' && status.status !== 'absent') {
      if (!sameFence(status, journal.expected)) {
        await this.clear()
        return
      }
      const deleted = await this.deps.requestAccountCredential({
        schemaVersion: 1, operation: 'delete', scope: journal.scope, expected: journal.expected
      })
      if (isFailure(deleted) || deleted.status !== 'absent') throw new Error('IM account lifecycle recovery failed.')
    }
    await this.deps.onCredentialInvalidated?.(journal.scope)
    const current = await this.deps.store.load()
    const saved = await this.deps.store.patch({
      claw: { channels: current.claw.channels.filter((item) => item.id !== journal.channel.id) }
    })
    if (saved.claw.channels.some((item) => item.id === journal.channel.id)) {
      throw new Error('IM account lifecycle recovery failed.')
    }
    await this.clear()
  }
}
