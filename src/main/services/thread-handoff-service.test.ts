import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { execFileSync } from 'node:child_process'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { threadHandoffStepsWithRollback, type ThreadHandoffOperation } from '../../shared/thread-handoff'
import { ThreadHandoffService } from './thread-handoff-service'

let sandbox = ''
let repoRoot = ''
let worktreeRoot = ''

async function createRepo(): Promise<void> {
  execFileSync('git', ['init', '-b', 'main', repoRoot], { stdio: 'pipe' })
  execFileSync('git', ['-C', repoRoot, 'config', 'user.email', 'test@example.com'], { stdio: 'pipe' })
  execFileSync('git', ['-C', repoRoot, 'config', 'user.name', 'Test'], { stdio: 'pipe' })
  await writeFile(join(repoRoot, 'README.md'), 'base\n', 'utf8')
  execFileSync('git', ['-C', repoRoot, 'add', 'README.md'], { stdio: 'pipe' })
  execFileSync('git', ['-C', repoRoot, 'commit', '-m', 'init'], { stdio: 'pipe' })
}

function waitForOperation(
  service: ThreadHandoffService,
  predicate: (operation: ThreadHandoffOperation) => boolean
): Promise<ThreadHandoffOperation> {
  const current = service.get().find(predicate)
  if (current) return Promise.resolve(current)
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      unsubscribe()
      reject(new Error('Timed out waiting for thread handoff operation state.'))
    }, 8_000)
    const unsubscribe = service.subscribe((event) => {
      if (!predicate(event.operation)) return
      clearTimeout(timeout)
      unsubscribe()
      resolve(event.operation)
    })
  })
}

beforeEach(async () => {
  sandbox = await mkdtemp(join(tmpdir(), 'analytix-thread-handoff-'))
  repoRoot = join(sandbox, 'repo')
  worktreeRoot = join(sandbox, 'worktrees')
  await mkdir(repoRoot, { recursive: true })
  await createRepo()
})

afterEach(async () => {
  if (sandbox) {
    await rm(sandbox, { recursive: true, force: true })
    sandbox = ''
    repoRoot = ''
    worktreeRoot = ''
  }
})

describe('thread-handoff-service', () => {
  it('moves uncommitted changes to a worktree and completes after renderer switch', async () => {
    const service = new ThreadHandoffService()
    await writeFile(join(repoRoot, 'README.md'), 'handoff change\n', 'utf8')

    const started = await service.start({
      direction: 'to-worktree',
      sourceThreadId: 'thr_1',
      sourceWorkspace: repoRoot,
      worktreeRoot
    })

    expect(started.status).toBe('queued')
    const readyToSwitch = await waitForOperation(service, (operation) =>
      operation.id === started.id &&
      operation.status === 'running' &&
      operation.steps.some((step) => step.id === 'switching-thread' && step.status === 'running')
    )

    expect(readyToSwitch.targetWorkspace).toBeTruthy()
    expect(readyToSwitch.steps).toEqual(expect.arrayContaining([
      { id: 'create-new-worktree', status: 'done' },
      { id: 'apply-changes-to-worktree', status: 'done' },
      { id: 'switching-thread', status: 'running' }
    ]))
    await expect(readFile(join(readyToSwitch.targetWorkspace ?? '', 'README.md'), 'utf8')).resolves.toBe('handoff change\n')

    const completed = await service.completeSwitch({
      operationId: started.id,
      targetThreadId: 'thr_1'
    })

    expect(completed).toMatchObject({
      status: 'success',
      hasUnseenTerminalState: false,
      targetThreadId: 'thr_1'
    })
    expect(completed.steps.find((step) => step.id === 'switching-thread')?.status).toBe('done')
  })

  it('restores source changes when the renderer fails the final thread switch', async () => {
    const service = new ThreadHandoffService()
    await writeFile(join(repoRoot, 'README.md'), 'switch failure change\n', 'utf8')

    const started = await service.start({
      direction: 'to-worktree',
      sourceThreadId: 'thr_switch_fail',
      sourceWorkspace: repoRoot,
      worktreeRoot
    })
    const readyToSwitch = await waitForOperation(service, (operation) =>
      operation.id === started.id &&
      operation.status === 'running' &&
      operation.steps.some((step) => step.id === 'switching-thread' && step.status === 'running')
    )

    await expect(readFile(join(repoRoot, 'README.md'), 'utf8')).resolves.toBe('base\n')
    await expect(readFile(join(readyToSwitch.targetWorkspace ?? '', 'README.md'), 'utf8'))
      .resolves.toBe('switch failure change\n')

    const failed = await service.failSwitch(started.id, 'renderer workspace update failed')

    expect(failed).toMatchObject({
      status: 'error',
      errorMessage: 'renderer workspace update failed',
      hasUnseenTerminalState: true
    })
    expect(failed.steps.find((step) => step.id === 'switching-thread')?.status).toBe('failed')
    await expect(readFile(join(repoRoot, 'README.md'), 'utf8')).resolves.toBe('switch failure change\n')
  })

  it('rolls source changes back on failure and rebuilds failed steps on retry', async () => {
    const service = new ThreadHandoffService()
    await writeFile(join(repoRoot, 'README.md'), 'rollback me\n', 'utf8')

    const started = await service.start({
      direction: 'to-worktree',
      sourceThreadId: 'thr_rollback',
      sourceWorkspace: repoRoot,
      worktreeBranch: 'bad branch name',
      worktreeRoot
    })
    const failed = await waitForOperation(service, (operation) =>
      operation.id === started.id && operation.status === 'error'
    )

    expect(failed.errorMessage).toBeTruthy()
    expect(failed.execOutput?.output).toContain('bad branch name')
    expect(failed.steps.find((step) => step.id === 'checkout-worktree-branch')?.status).toBe('failed')
    expect(threadHandoffStepsWithRollback(failed.steps)).toEqual(expect.arrayContaining([
      { id: 'rolling-back-changes', status: 'running' }
    ]))
    await expect(readFile(join(repoRoot, 'README.md'), 'utf8')).resolves.toBe('rollback me\n')

    const retry = await service.retry(started.id)

    expect(retry.steps.some((step) => step.status === 'failed')).toBe(false)
    const failedAgain = await waitForOperation(service, (operation) =>
      operation.id === retry.id && operation.status === 'error' && operation.updatedAt !== failed.updatedAt
    )
    expect(failedAgain.steps.find((step) => step.id === 'checkout-worktree-branch')?.status).toBe('failed')
  })

  it('marks a running operation as warning when cancelled', async () => {
    const service = new ThreadHandoffService()
    await writeFile(join(repoRoot, 'README.md'), 'cancel me\n', 'utf8')

    const started = await service.start({
      direction: 'to-worktree',
      sourceThreadId: 'thr_cancel',
      sourceWorkspace: repoRoot,
      worktreeRoot
    })
    await waitForOperation(service, (operation) =>
      operation.id === started.id &&
      operation.status === 'running' &&
      operation.steps.some((step) => step.id === 'switching-thread' && step.status === 'running')
    )
    const warning = service.cancel(started.id)

    expect(warning.status).toBe('warning')
    expect(warning.warningMessage).toContain('cancellation')
    expect(warning.hasUnseenTerminalState).toBe(true)
  })
})
