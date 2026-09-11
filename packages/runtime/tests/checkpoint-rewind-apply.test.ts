import { execFile } from 'node:child_process'
import { createHash } from 'node:crypto'
import { access, chmod, mkdir, mkdtemp, readFile, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { promisify } from 'node:util'
import { afterEach, describe, expect, it } from 'vitest'
import type { CheckpointChangedFile, CheckpointMetadata } from '../src/contracts/checkpoints.js'
import type { RuntimeEvent } from '../src/contracts/events.js'
import { InMemoryEventBus } from '../src/adapters/in-memory-event-bus.js'
import { InMemorySessionStore } from '../src/adapters/in-memory-session-store.js'
import { createAnalytixCheckpointMetadata } from '../src/domain/checkpoint-rewind-contract.js'
import { RuntimeEventRecorder } from '../src/services-test-support/runtime-event-recorder.js'
import { CheckpointRewindService } from '../src/services-test-support/checkpoint-rewind-service.js'
import { applyCheckpointRewindPlan } from '../src/server-test-support/routes/checkpoints.js'
import type { ThreadService } from '../src/services-test-support/thread-service.js'

const execFileAsync = promisify(execFile)
const timestamp = '2026-06-20T12:00:00.000Z'
const threadId = 'thread-p4c'

const tempDirs: string[] = []

afterEach(async () => {
  await Promise.all(tempDirs.splice(0).map((dir) => rm(dir, { recursive: true, force: true })))
})

function hashUtf8(content: string): string {
  return `sha256:${createHash('sha256').update(content, 'utf8').digest('hex')}`
}

async function tempWorkspace(): Promise<string> {
  const dir = await mkdtemp(join(tmpdir(), 'analytix-p4c-'))
  tempDirs.push(dir)
  return dir
}

async function writeWorkspaceFile(workspace: string, relativePath: string, content: string): Promise<void> {
  const target = join(workspace, relativePath)
  await mkdir(dirname(target), { recursive: true })
  await writeFile(target, content, 'utf8')
}

async function exists(path: string): Promise<boolean> {
  try {
    await access(path)
    return true
  } catch {
    return false
  }
}

function baseEvent(overrides: Partial<RuntimeEvent> & Pick<RuntimeEvent, 'kind'>): RuntimeEvent {
  return {
    seq: overrides.seq ?? 1,
    timestamp: overrides.timestamp ?? timestamp,
    threadId: overrides.threadId ?? threadId,
    ...overrides
  } as RuntimeEvent
}

function eventsForCheckpoint(checkpoint: CheckpointMetadata): RuntimeEvent[] {
  return [
    baseEvent({ kind: 'thread_created', seq: 1, title: 'P4C thread' }),
    baseEvent({ kind: 'turn_started', seq: 2, turnId: 'turn-1' }),
    baseEvent({
      kind: 'item_created',
      seq: 3,
      turnId: 'turn-1',
      itemId: 'item-user-1',
      item: {
        id: 'item-user-1',
        turnId: 'turn-1',
        threadId,
        role: 'user',
        status: 'completed',
        createdAt: timestamp,
        finishedAt: timestamp,
        kind: 'user_message',
        text: 'first prompt'
      }
    }),
    baseEvent({ kind: 'turn_completed', seq: 4, turnId: 'turn-1' }),
    baseEvent({ kind: 'checkpoint_captured', seq: 5, turnId: 'turn-2', checkpoint }),
    baseEvent({ kind: 'turn_started', seq: 6, turnId: 'turn-2' }),
    baseEvent({
      kind: 'item_created',
      seq: 7,
      turnId: 'turn-2',
      itemId: 'item-user-2',
      item: {
        id: 'item-user-2',
        turnId: 'turn-2',
        threadId,
        role: 'user',
        status: 'completed',
        createdAt: timestamp,
        finishedAt: timestamp,
        kind: 'user_message',
        text: 'second prompt'
      }
    }),
    baseEvent({ kind: 'turn_completed', seq: 8, turnId: 'turn-2' })
  ]
}

function checkpoint(workspace: string, changedFiles: CheckpointChangedFile[]): CheckpointMetadata {
  return createAnalytixCheckpointMetadata({
    workspace,
    threadId,
    turnId: 'turn-2',
    createdAt: timestamp,
    changedFiles
  })
}

async function harness(workspace: string): Promise<{
  service: CheckpointRewindService
  sessionStore: InMemorySessionStore
}> {
  const sessionStore = new InMemorySessionStore()
  const eventBus = new InMemoryEventBus()
  const nowIso = () => timestamp
  const events = new RuntimeEventRecorder({
    eventBus,
    sessionStore,
    allocateSeq: (id) => eventBus.allocateSeq(id),
    nowIso
  })
  const threadService = {
    get: async (id: string) => ({ id, title: 'P4C thread', workspace })
  } as unknown as ThreadService
  return {
    service: new CheckpointRewindService({ threadService, sessionStore, events, nowIso }),
    sessionStore
  }
}

async function seedCheckpoint(
  sessionStore: InMemorySessionStore,
  metadata: CheckpointMetadata
): Promise<void> {
  for (const event of eventsForCheckpoint(metadata)) {
    await sessionStore.appendEvent(threadId, event)
  }
}

async function createPlan(
  service: CheckpointRewindService,
  metadata: CheckpointMetadata,
  scope: 'code' | 'conversation' | 'combined' = 'code'
) {
  const result = await service.createPlan({ threadId, checkpointId: metadata.checkpointId, scope })
  expect(result.ok).toBe(true)
  if (!result.ok) throw new Error(result.message)
  return result.plan
}

async function applyPlan(
  service: CheckpointRewindService,
  plan: Awaited<ReturnType<typeof createPlan>>,
  hostPrivateSnapshots: Array<{
    relativePath: string
    before?: { hash: string; content: string; encoding: 'utf8' }
    after?: { hash: string; content: string; encoding: 'utf8' }
  }> = []
) {
  return service.applyPlan({
    threadId,
    checkpointId: plan.checkpointId,
    request: {
      plan,
      confirmation: {
        confirmed: true,
        destructive: true,
        phrase: 'APPLY_CHECKPOINT_REWIND'
      }
    },
    hostPrivateSnapshots
  })
}

async function git(workspace: string, args: string[]): Promise<void> {
  await execFileAsync('git', ['-C', workspace, ...args])
}

async function initGit(workspace: string): Promise<void> {
  await execFileAsync('git', ['-C', workspace, 'init'])
  await git(workspace, ['config', 'user.email', 'analytix@example.invalid'])
  await git(workspace, ['config', 'user.name', 'Analytix Test'])
}

describe('P4C confirmed checkpoint rewind apply', () => {
  it('applies a modified-file restore with host-private snapshot evidence and rescue audit', async () => {
    const workspace = await tempWorkspace()
    const before = 'before\n'
    const after = 'after\n'
    await writeWorkspaceFile(workspace, 'src/app.ts', after)
    const metadata = checkpoint(workspace, [{
      relativePath: 'src/app.ts',
      changeKind: 'modified',
      beforeHash: hashUtf8(before),
      afterHash: hashUtf8(after)
    }])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'code')

    const result = await applyPlan(service, plan, [{
      relativePath: 'src/app.ts',
      before: { hash: hashUtf8(before), content: before, encoding: 'utf8' }
    }])

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.apply.status).toBe('applied')
    expect(result.apply.rescue?.rescueId).toMatch(/^axrr_/)
    expect(result.apply.files).toMatchObject([
      { relativePath: 'src/app.ts', action: 'restore_previous_version', status: 'applied' }
    ])
    expect(await readFile(join(workspace, 'src/app.ts'), 'utf8')).toBe(before)
    const events = await sessionStore.loadEventsSince(threadId, 0)
    const rescueIndex = events.findIndex((event) => event.kind === 'checkpoint_rewind_rescue_created')
    const applyIndex = events.findIndex((event) => event.kind === 'checkpoint_rewind_applied')
    expect(rescueIndex).toBeGreaterThan(-1)
    expect(applyIndex).toBeGreaterThan(rescueIndex)
    const rescue = events.find((event) => event.kind === 'checkpoint_rewind_rescue_created')
    expect(rescue?.kind === 'checkpoint_rewind_rescue_created' ? rescue.rescue.files[0]?.content : undefined).toBe(after)
  })

  it('blocks ready-looking restores that lack required snapshot evidence', async () => {
    const workspace = await tempWorkspace()
    const before = 'before\n'
    const after = 'after\n'
    await writeWorkspaceFile(workspace, 'src/app.ts', after)
    const metadata = checkpoint(workspace, [{
      relativePath: 'src/app.ts',
      changeKind: 'modified',
      beforeHash: hashUtf8(before),
      afterHash: hashUtf8(after)
    }])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'code')

    const result = await applyPlan(service, plan)

    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.code).toBe('apply_blocked')
    expect(result.apply?.files[0]).toMatchObject({
      status: 'blocked',
      reason: 'restore_previous_version requires before snapshot evidence'
    })
    expect(await readFile(join(workspace, 'src/app.ts'), 'utf8')).toBe(after)
  })

  it('blocks snapshot evidence whose content does not match its declared hash', async () => {
    const workspace = await tempWorkspace()
    const before = 'before\n'
    const after = 'after\n'
    await writeWorkspaceFile(workspace, 'src/app.ts', after)
    const metadata = checkpoint(workspace, [{
      relativePath: 'src/app.ts',
      changeKind: 'modified',
      beforeHash: hashUtf8(before),
      afterHash: hashUtf8(after)
    }])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'code')

    const result = await applyPlan(service, plan, [{
      relativePath: 'src/app.ts',
      before: { hash: hashUtf8(before), content: 'malicious\n', encoding: 'utf8' }
    }])

    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.apply?.files[0]).toMatchObject({
      status: 'blocked',
      reason: 'before snapshot content hash does not match snapshot hash'
    })
    expect(await readFile(join(workspace, 'src/app.ts'), 'utf8')).toBe(after)
  })

  it('keeps manual-review plans blocked without mutating ready files', async () => {
    const workspace = await tempWorkspace()
    await writeWorkspaceFile(workspace, 'src/app.ts', 'after\n')
    await writeWorkspaceFile(workspace, 'src/unknown.ts', 'unknown\n')
    const metadata = checkpoint(workspace, [
      {
        relativePath: 'src/app.ts',
        changeKind: 'modified',
        beforeHash: hashUtf8('before\n'),
        afterHash: hashUtf8('after\n')
      },
      { relativePath: 'src/unknown.ts', changeKind: 'unknown' }
    ])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'code')

    const result = await applyPlan(service, plan, [{
      relativePath: 'src/app.ts',
      before: { hash: hashUtf8('before\n'), content: 'before\n', encoding: 'utf8' }
    }])

    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.apply?.files.find((file) => file.relativePath === 'src/unknown.ts')?.status).toBe('manual_review')
    expect(result.apply?.files.find((file) => file.relativePath === 'src/app.ts')?.status).toBe('blocked')
    expect(await readFile(join(workspace, 'src/app.ts'), 'utf8')).toBe('after\n')
  })

  it('blocks path escape and absolute path restore plans', async () => {
    const workspace = await tempWorkspace()
    const metadata: CheckpointMetadata = {
      schemaVersion: 1,
      checkpointId: 'axcp_p4c_escape',
      threadId,
      turnId: 'turn-2',
      workspace,
      createdAt: timestamp,
      status: 'captured',
      changedFiles: [
        { relativePath: '../outside.ts', changeKind: 'modified', beforeHash: hashUtf8('x'), afterHash: hashUtf8('y') },
        { relativePath: '/tmp/outside.ts', changeKind: 'modified', beforeHash: hashUtf8('x'), afterHash: hashUtf8('y') }
      ]
    }
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'code')

    const result = await applyPlan(service, plan, [
      { relativePath: '../outside.ts', before: { hash: hashUtf8('x'), content: 'x', encoding: 'utf8' } },
      { relativePath: '/tmp/outside.ts', before: { hash: hashUtf8('x'), content: 'x', encoding: 'utf8' } }
    ])

    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.apply?.files.map((file) => file.status)).toEqual(['blocked', 'blocked'])
  })

  it('blocks symlink restore targets', async () => {
    const workspace = await tempWorkspace()
    await writeWorkspaceFile(workspace, 'real.ts', 'after\n')
    await symlink(join(workspace, 'real.ts'), join(workspace, 'link.ts'))
    const metadata = checkpoint(workspace, [{
      relativePath: 'link.ts',
      changeKind: 'modified',
      beforeHash: hashUtf8('before\n'),
      afterHash: hashUtf8('after\n')
    }])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'code')

    const result = await applyPlan(service, plan, [{
      relativePath: 'link.ts',
      before: { hash: hashUtf8('before\n'), content: 'before\n', encoding: 'utf8' }
    }])

    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.apply?.files[0]).toMatchObject({ status: 'blocked' })
    expect(await readFile(join(workspace, 'real.ts'), 'utf8')).toBe('after\n')
  })

  it('blocks restore paths that escape through symlink parent directories', async () => {
    const workspace = await tempWorkspace()
    const outside = await tempWorkspace()
    const before = 'before\n'
    const after = 'after\n'
    await writeWorkspaceFile(outside, 'app.ts', after)
    await symlink(outside, join(workspace, 'linked'))
    const metadata = checkpoint(workspace, [{
      relativePath: 'linked/app.ts',
      changeKind: 'modified',
      beforeHash: hashUtf8(before),
      afterHash: hashUtf8(after)
    }])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'code')

    const result = await applyPlan(service, plan, [{
      relativePath: 'linked/app.ts',
      before: { hash: hashUtf8(before), content: before, encoding: 'utf8' }
    }])

    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.apply?.files[0]).toMatchObject({
      status: 'blocked',
      reason: 'checkpoint apply path contains symlink ancestor: linked'
    })
    expect(await readFile(join(outside, 'app.ts'), 'utf8')).toBe(after)
  })

  it('blocks staged and untracked git conflicts', async () => {
    const workspace = await tempWorkspace()
    await initGit(workspace)
    await writeWorkspaceFile(workspace, 'src/staged.ts', 'after\n')
    await writeWorkspaceFile(workspace, 'src/untracked.ts', 'after\n')
    await git(workspace, ['add', 'src/staged.ts'])
    const metadata = checkpoint(workspace, [
      {
        relativePath: 'src/staged.ts',
        changeKind: 'modified',
        beforeHash: hashUtf8('before\n'),
        afterHash: hashUtf8('after\n')
      },
      {
        relativePath: 'src/untracked.ts',
        changeKind: 'modified',
        beforeHash: hashUtf8('before\n'),
        afterHash: hashUtf8('after\n')
      }
    ])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'code')

    const result = await applyPlan(service, plan, [
      { relativePath: 'src/staged.ts', before: { hash: hashUtf8('before\n'), content: 'before\n', encoding: 'utf8' } },
      { relativePath: 'src/untracked.ts', before: { hash: hashUtf8('before\n'), content: 'before\n', encoding: 'utf8' } }
    ])

    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.apply?.files.find((file) => file.relativePath === 'src/staged.ts')?.reason).toMatch(/staged/)
    expect(result.apply?.files.find((file) => file.relativePath === 'src/untracked.ts')?.reason).toMatch(/untracked/)
  })

  it('deletes checkpoint-created files and treats missing created files as noop', async () => {
    const workspace = await tempWorkspace()
    const created = 'created\n'
    await writeWorkspaceFile(workspace, 'src/new.ts', created)
    const metadata = checkpoint(workspace, [
      { relativePath: 'src/new.ts', changeKind: 'created', afterHash: hashUtf8(created) },
      { relativePath: 'src/already-gone.ts', changeKind: 'created', afterHash: hashUtf8('gone\n') }
    ])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'code')

    const result = await applyPlan(service, plan)

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(await exists(join(workspace, 'src/new.ts'))).toBe(false)
    expect(result.apply.files.map((file) => [file.relativePath, file.status])).toEqual([
      ['src/new.ts', 'applied'],
      ['src/already-gone.ts', 'noop']
    ])
  })

  it('restores deleted files from before snapshot evidence', async () => {
    const workspace = await tempWorkspace()
    const before = 'deleted before\n'
    const metadata = checkpoint(workspace, [{
      relativePath: 'src/deleted.ts',
      changeKind: 'deleted',
      beforeHash: hashUtf8(before)
    }])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'code')
    expect(plan.files[0]?.action).toBe('restore_deleted_file')
    expect(plan.files[0]?.status).toBe('ready')

    const result = await applyPlan(service, plan, [{
      relativePath: 'src/deleted.ts',
      before: { hash: hashUtf8(before), content: before, encoding: 'utf8' }
    }])

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(await readFile(join(workspace, 'src/deleted.ts'), 'utf8')).toBe(before)
    expect(result.apply.files[0]).toMatchObject({ action: 'restore_deleted_file', status: 'applied' })
  })

  it('records conversation-only apply as append-only audit without rewriting history', async () => {
    const workspace = await tempWorkspace()
    const metadata = checkpoint(workspace, [{
      relativePath: 'src/app.ts',
      changeKind: 'modified',
      beforeHash: hashUtf8('before\n'),
      afterHash: hashUtf8('after\n')
    }])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const beforeItems = await sessionStore.loadItems(threadId)
    const plan = await createPlan(service, metadata, 'conversation')

    const result = await applyPlan(service, plan)

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.apply.files).toEqual([])
    expect(result.apply.conversation).toMatchObject({ status: 'audit_recorded', removedTurnIds: ['turn-2'] })
    expect(await sessionStore.loadItems(threadId)).toEqual(beforeItems)
    const events = await sessionStore.loadEventsSince(threadId, 0)
    expect(events.at(-1)?.kind).toBe('checkpoint_rewind_applied')
  })

  it('applies combined code and conversation restore inside one audited operation', async () => {
    const workspace = await tempWorkspace()
    const before = 'before\n'
    const after = 'after\n'
    await writeWorkspaceFile(workspace, 'src/app.ts', after)
    const metadata = checkpoint(workspace, [{
      relativePath: 'src/app.ts',
      changeKind: 'modified',
      beforeHash: hashUtf8(before),
      afterHash: hashUtf8(after)
    }])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'combined')

    const result = await applyPlan(service, plan, [{
      relativePath: 'src/app.ts',
      before: { hash: hashUtf8(before), content: before, encoding: 'utf8' }
    }])

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(await readFile(join(workspace, 'src/app.ts'), 'utf8')).toBe(before)
    expect(result.apply.files[0]?.status).toBe('applied')
    expect(result.apply.conversation.status).toBe('audit_recorded')
  })

  it('keeps already-applied file results when a later mutation fails', async () => {
    const workspace = await tempWorkspace()
    const beforeOne = 'before one\n'
    const afterOne = 'after one\n'
    const beforeTwo = 'before two\n'
    const afterTwo = 'after two\n'
    await writeWorkspaceFile(workspace, 'src/one.ts', afterOne)
    await writeWorkspaceFile(workspace, 'src/two.ts', afterTwo)
    await chmod(join(workspace, 'src/two.ts'), 0o444)
    const metadata = checkpoint(workspace, [
      {
        relativePath: 'src/one.ts',
        changeKind: 'modified',
        beforeHash: hashUtf8(beforeOne),
        afterHash: hashUtf8(afterOne)
      },
      {
        relativePath: 'src/two.ts',
        changeKind: 'modified',
        beforeHash: hashUtf8(beforeTwo),
        afterHash: hashUtf8(afterTwo)
      }
    ])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'code')

    const result = await applyPlan(service, plan, [
      { relativePath: 'src/one.ts', before: { hash: hashUtf8(beforeOne), content: beforeOne, encoding: 'utf8' } },
      { relativePath: 'src/two.ts', before: { hash: hashUtf8(beforeTwo), content: beforeTwo, encoding: 'utf8' } }
    ])

    await chmod(join(workspace, 'src/two.ts'), 0o644).catch(() => undefined)
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.apply?.status).toBe('failed')
    expect(result.apply?.files.find((file) => file.relativePath === 'src/one.ts')).toMatchObject({
      status: 'applied'
    })
    expect(result.apply?.files.find((file) => file.relativePath === 'src/two.ts')?.status).toBe('failed')
    expect(result.apply?.summary.fileAppliedCount).toBe(1)
    expect(await readFile(join(workspace, 'src/one.ts'), 'utf8')).toBe(beforeOne)
  })

  it('is idempotent for already applied plans', async () => {
    const workspace = await tempWorkspace()
    const before = 'before\n'
    const after = 'after\n'
    await writeWorkspaceFile(workspace, 'src/app.ts', after)
    const metadata = checkpoint(workspace, [{
      relativePath: 'src/app.ts',
      changeKind: 'modified',
      beforeHash: hashUtf8(before),
      afterHash: hashUtf8(after)
    }])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'code')
    const snapshots = [{
      relativePath: 'src/app.ts',
      before: { hash: hashUtf8(before), content: before, encoding: 'utf8' as const }
    }]

    const first = await applyPlan(service, plan, snapshots)
    const second = await applyPlan(service, plan, snapshots)

    expect(first.ok).toBe(true)
    expect(second.ok).toBe(true)
    if (!second.ok) return
    expect(second.apply.status).toBe('already_applied')
    expect(await readFile(join(workspace, 'src/app.ts'), 'utf8')).toBe(before)
    const events = await sessionStore.loadEventsSince(threadId, 0)
    expect(events.filter((event) => event.kind === 'checkpoint_rewind_rescue_created')).toHaveLength(1)
    expect(events.filter((event) => event.kind === 'checkpoint_rewind_applied')).toHaveLength(1)
  })

  it('exposes apply through the confirmed runtime route', async () => {
    const workspace = await tempWorkspace()
    const metadata = checkpoint(workspace, [])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'conversation')

    const response = await applyCheckpointRewindPlan(
      service,
      threadId,
      metadata.checkpointId,
      new Request('http://analytix.local/v1/rewind-apply', {
        method: 'POST',
        body: JSON.stringify({
          plan,
          confirmation: {
            confirmed: true,
            destructive: true,
            phrase: 'APPLY_CHECKPOINT_REWIND'
          }
        })
      })
    )

    expect(response.status).toBe(200)
    if (typeof response.body !== 'string') throw new Error('expected JSON route response body')
    const payload = JSON.parse(response.body) as { apply: { status: string } }
    expect(payload.apply.status).toBe('applied')
  })

  it('rejects client-provided snapshot bytes at the public apply route', async () => {
    const workspace = await tempWorkspace()
    const metadata = checkpoint(workspace, [])
    const { service, sessionStore } = await harness(workspace)
    await seedCheckpoint(sessionStore, metadata)
    const plan = await createPlan(service, metadata, 'conversation')

    const response = await applyCheckpointRewindPlan(
      service,
      threadId,
      metadata.checkpointId,
      new Request('http://analytix.local/v1/rewind-apply', {
        method: 'POST',
        body: JSON.stringify({
          plan,
          confirmation: {
            confirmed: true,
            destructive: true,
            phrase: 'APPLY_CHECKPOINT_REWIND'
          },
          snapshots: [{
            relativePath: 'src/app.ts',
            before: {
              hash: hashUtf8('forged\n'),
              content: 'forged\n',
              encoding: 'utf8'
            }
          }]
        })
      })
    )

    expect(response.status).toBe(400)
  })
})
