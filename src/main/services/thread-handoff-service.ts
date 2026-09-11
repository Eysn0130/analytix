import { execFile } from 'node:child_process'
import { randomUUID } from 'node:crypto'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { promisify } from 'node:util'
import {
  threadHandoffInitialStepIds,
  type ThreadHandoffCompleteSwitchRequest,
  type ThreadHandoffEvent,
  type ThreadHandoffExecOutput,
  type ThreadHandoffOperation,
  type ThreadHandoffRequest,
  type ThreadHandoffStep,
  type ThreadHandoffStepId
} from '../../shared/thread-handoff'
import { parseWorktreeHasChangesError, type WorktreeInfo } from '../../shared/worktree'
import { runGit } from './git-service'
import {
  acquireWorktree,
  findAvailablePoolIndex,
  getWorktreeChanges,
  releaseWorktree
} from './worktree-service'

const execFileAsync = promisify(execFile)

type InternalOperation = {
  operation: ThreadHandoffOperation
  sourceStashRef: string | null
  sourceStashCwd: string | null
  sourceStashDropped: boolean
}

type ThreadHandoffListener = (event: ThreadHandoffEvent) => void

function nowIso(): string {
  return new Date().toISOString()
}

function initialOperation(request: ThreadHandoffRequest): InternalOperation {
  const timestamp = nowIso()
  const steps = threadHandoffInitialStepIds(request).map((id): ThreadHandoffStep => ({
    id,
    status: 'pending'
  }))
  return {
    operation: {
      id: randomUUID(),
      direction: request.direction,
      status: 'queued',
      sourceThreadId: request.sourceThreadId,
      targetThreadId: null,
      sourceWorkspace: request.sourceWorkspace,
      targetWorkspace: null,
      sourceBranch: request.sourceBranch?.trim() || null,
      localBranch: request.direction === 'to-local' || request.direction === 'to-worktree'
        ? request.localBranch?.trim() || request.sourceBranch?.trim() || null
        : null,
      worktreeBranch: request.direction === 'to-worktree' || request.direction === 'to-host-worktree'
        ? request.worktreeBranch?.trim() || null
        : null,
      request,
      steps,
      errorMessage: null,
      warningMessage: null,
      execOutput: null,
      hasUnseenTerminalState: false,
      worktree: null,
      createdAt: timestamp,
      updatedAt: timestamp
    },
    sourceStashRef: null,
    sourceStashCwd: null,
    sourceStashDropped: false
  }
}

function cloneOperation(internal: InternalOperation): ThreadHandoffOperation {
  return {
    ...internal.operation,
    steps: internal.operation.steps.map((step) => ({ ...step })),
    request: { ...internal.operation.request } as ThreadHandoffRequest,
    execOutput: internal.operation.execOutput ? { ...internal.operation.execOutput } : null,
    worktree: internal.operation.worktree ? { ...internal.operation.worktree } : null
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function commandOutput(command: string, error: unknown): ThreadHandoffExecOutput {
  const detail = error instanceof Error ? error.message : String(error)
  return { command, output: detail }
}

async function git(cwd: string, args: string[], timeout = 20_000): Promise<{ stdout: string; stderr: string }> {
  return runGit(cwd, args, timeout)
}

async function gitApplyPatch(cwd: string, patch: string): Promise<void> {
  if (!patch.trim()) return
  const dir = await mkdtemp(join(tmpdir(), 'analytix-handoff-'))
  const patchPath = join(dir, 'changes.patch')
  try {
    await writeFile(patchPath, patch, 'utf8')
    await git(cwd, ['apply', '--3way', '--binary', patchPath], 30_000)
  } finally {
    await rm(dir, { recursive: true, force: true })
  }
}

async function currentBranch(cwd: string): Promise<string | null> {
  const { stdout } = await git(cwd, ['branch', '--show-current'])
  return stdout.trim() || null
}

async function stashList(cwd: string): Promise<string[]> {
  const { stdout } = await git(cwd, ['stash', 'list', '--format=%gd%x00%s'])
  return stdout.split('\n').map((line) => line.trim()).filter(Boolean)
}

async function stashChanges(cwd: string, operationId: string): Promise<string | null> {
  const before = new Set(await stashList(cwd))
  const message = `analytix-handoff:${operationId}`
  const result = await git(cwd, ['stash', 'push', '--include-untracked', '-m', message], 30_000)
  if (/No local changes to save/i.test(`${result.stdout}\n${result.stderr}`)) return null
  const after = await stashList(cwd)
  const created = after.find((line) => line.includes(message) && !before.has(line))
    ?? after.find((line) => line.includes(message))
  if (!created) {
    throw new Error('Git reported a stash was created, but the new stash entry could not be found.')
  }
  return created.split('\0')[0] || 'stash@{0}'
}

async function stashPatch(cwd: string, stashRef: string): Promise<string> {
  const { stdout } = await git(cwd, ['stash', 'show', '--include-untracked', '--binary', '-p', stashRef], 30_000)
  return stdout
}

async function dropStash(cwd: string, stashRef: string): Promise<void> {
  await git(cwd, ['stash', 'drop', stashRef], 30_000)
}

async function restoreStash(cwd: string, stashRef: string): Promise<void> {
  await git(cwd, ['stash', 'apply', stashRef], 30_000)
  await dropStash(cwd, stashRef)
}

async function switchBranch(cwd: string, branch: string | null, createIfMissing = true): Promise<void> {
  const target = branch?.trim()
  if (!target) return
  await git(cwd, ['check-ref-format', '--branch', target])
  try {
    await git(cwd, ['switch', target], 30_000)
  } catch {
    if (!createIfMissing) throw new Error(`Branch not found: ${target}`)
    await git(cwd, ['switch', '-c', target], 30_000)
  }
}

export class ThreadHandoffService {
  private readonly operations = new Map<string, InternalOperation>()
  private readonly listeners = new Set<ThreadHandoffListener>()

  subscribe(listener: ThreadHandoffListener): () => void {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  get(operationId?: string): ThreadHandoffOperation[] {
    if (operationId) {
      const operation = this.operations.get(operationId)
      return operation ? [cloneOperation(operation)] : []
    }
    return [...this.operations.values()].map(cloneOperation)
  }

  async start(request: ThreadHandoffRequest): Promise<ThreadHandoffOperation> {
    if (request.direction === 'to-host-worktree') {
      throw new Error('Cross-host thread handoff is not available in analytix yet.')
    }
    for (const existing of this.operations.values()) {
      if (
        existing.operation.sourceThreadId === request.sourceThreadId &&
        (existing.operation.status === 'queued' || existing.operation.status === 'running')
      ) {
        throw new Error('A thread handoff is already running for this thread.')
      }
    }
    const internal = initialOperation(request)
    this.operations.set(internal.operation.id, internal)
    this.emit(internal)
    queueMicrotask(() => {
      void this.run(internal.operation.id)
    })
    return cloneOperation(internal)
  }

  async retry(operationId: string): Promise<ThreadHandoffOperation> {
    const existing = this.operations.get(operationId)
    if (!existing) throw new Error(`Thread handoff operation not found: ${operationId}`)
    const next = initialOperation(existing.operation.request)
    next.operation.id = existing.operation.id
    next.operation.createdAt = existing.operation.createdAt
    this.operations.set(next.operation.id, next)
    this.emit(next)
    queueMicrotask(() => {
      void this.run(next.operation.id)
    })
    return cloneOperation(next)
  }

  async completeSwitch(request: ThreadHandoffCompleteSwitchRequest): Promise<ThreadHandoffOperation> {
    const internal = this.requireOperation(request.operationId)
    if (internal.operation.status !== 'running') return cloneOperation(internal)
    const switching = internal.operation.steps.find((step) => step.id === 'switching-thread')
    if (switching) switching.status = 'done'
    if (request.targetThreadId?.trim()) internal.operation.targetThreadId = request.targetThreadId.trim()
    if (internal.sourceStashRef && internal.sourceStashCwd && !internal.sourceStashDropped) {
      await dropStash(internal.sourceStashCwd, internal.sourceStashRef)
      internal.sourceStashDropped = true
    }
    internal.operation.status = 'success'
    internal.operation.updatedAt = nowIso()
    this.emit(internal)
    return cloneOperation(internal)
  }

  async failSwitch(operationId: string, message: string): Promise<ThreadHandoffOperation> {
    const internal = this.requireOperation(operationId)
    const switching = internal.operation.steps.find((step) => step.id === 'switching-thread')
    if (switching) switching.status = 'failed'
    await this.rollbackSourceStash(internal)
    internal.operation.status = 'error'
    internal.operation.errorMessage = message
    internal.operation.execOutput = { output: message }
    internal.operation.hasUnseenTerminalState = true
    internal.operation.updatedAt = nowIso()
    this.emit(internal)
    return cloneOperation(internal)
  }

  cancel(operationId: string): ThreadHandoffOperation {
    const internal = this.requireOperation(operationId)
    if (internal.operation.status === 'running' || internal.operation.status === 'queued') {
      internal.operation.status = 'warning'
      internal.operation.warningMessage = 'Thread handoff cancellation is requested. Any completed git steps were left in place.'
      internal.operation.hasUnseenTerminalState = true
      internal.operation.updatedAt = nowIso()
      this.emit(internal)
    }
    return cloneOperation(internal)
  }

  remove(operationId: string): boolean {
    return this.operations.delete(operationId)
  }

  private requireOperation(operationId: string): InternalOperation {
    const operation = this.operations.get(operationId)
    if (!operation) throw new Error(`Thread handoff operation not found: ${operationId}`)
    return operation
  }

  private emit(internal: InternalOperation): void {
    const event = { operation: cloneOperation(internal) }
    for (const listener of this.listeners) listener(event)
  }

  private setStatus(internal: InternalOperation, status: ThreadHandoffOperation['status']): void {
    internal.operation.status = status
    internal.operation.updatedAt = nowIso()
    this.emit(internal)
  }

  private step(internal: InternalOperation, id: ThreadHandoffStepId, status: ThreadHandoffStep['status']): void {
    const step = internal.operation.steps.find((candidate) => candidate.id === id)
    if (!step) return
    step.status = status
    internal.operation.updatedAt = nowIso()
    this.emit(internal)
  }

  private async run(operationId: string): Promise<void> {
    const internal = this.requireOperation(operationId)
    this.setStatus(internal, 'running')
    try {
      if (internal.operation.direction === 'to-worktree') {
        await this.runToWorktree(internal)
      } else if (internal.operation.direction === 'to-local') {
        await this.runToLocal(internal)
      } else {
        throw new Error('Cross-host thread handoff is not available in analytix yet.')
      }
      this.step(internal, 'switching-thread', 'running')
      this.setStatus(internal, 'running')
    } catch (error) {
      await this.markError(internal, error)
    }
  }

  private async runToWorktree(internal: InternalOperation): Promise<void> {
    const request = internal.operation.request
    if (request.direction !== 'to-worktree') return
    const sourceWorkspace = request.sourceWorkspace
    const sourceBranch = request.sourceBranch?.trim() || await currentBranch(sourceWorkspace)
    internal.operation.sourceBranch = sourceBranch
    internal.operation.localBranch = request.localBranch?.trim() || sourceBranch
    const requestedWorktreeBranch = request.worktreeBranch?.trim() || null
    internal.operation.worktreeBranch = requestedWorktreeBranch

    const poolIndex = await findAvailablePoolIndex({
      projectPath: sourceWorkspace,
      worktreeRoot: request.worktreeRoot
    })
    if (poolIndex === null) throw new Error('No available worktree pool slot.')

    let worktree: WorktreeInfo
    try {
      this.step(internal, 'create-new-worktree', 'running')
      worktree = await acquireWorktree({
        projectPath: sourceWorkspace,
        poolIndex,
        taskId: internal.operation.id,
        worktreeRoot: request.worktreeRoot
      })
      this.step(internal, 'create-new-worktree', 'done')
    } catch (error) {
      if (parseWorktreeHasChangesError(errorMessage(error))) {
        this.step(internal, 'create-new-worktree', 'failed')
      }
      throw error
    }

    internal.operation.worktree = worktree
    internal.operation.targetWorkspace = worktree.path
    const worktreeBranch = requestedWorktreeBranch || worktree.branch
    internal.operation.worktreeBranch = worktreeBranch

    const patch = await this.stashSourceForPatch(internal, sourceWorkspace, 'stash-source-changes')
    this.step(internal, 'checkout-local-branch', 'running')
    await switchBranch(sourceWorkspace, internal.operation.localBranch)
    this.step(internal, 'checkout-local-branch', 'done')
    this.step(internal, 'stash-target-worktree-changes', 'running')
    const targetChanges = await getWorktreeChanges({ worktreePath: worktree.path })
    if (targetChanges.hasUncommittedChanges) {
      await stashChanges(worktree.path, `${internal.operation.id}:target`)
    }
    this.step(internal, 'stash-target-worktree-changes', 'done')
    this.step(internal, 'checkout-worktree-branch', 'running')
    await switchBranch(worktree.path, worktreeBranch)
    this.step(internal, 'checkout-worktree-branch', 'done')
    this.step(internal, 'apply-changes-to-worktree', 'running')
    await gitApplyPatch(worktree.path, patch)
    this.step(internal, 'apply-changes-to-worktree', 'done')
  }

  private async runToLocal(internal: InternalOperation): Promise<void> {
    const request = internal.operation.request
    if (request.direction !== 'to-local') return
    const sourceWorkspace = request.sourceWorkspace
    const targetWorkspace = request.localWorkspace
    internal.operation.targetWorkspace = targetWorkspace
    internal.operation.sourceBranch = request.sourceBranch?.trim() || await currentBranch(sourceWorkspace)
    internal.operation.localBranch = request.localBranch?.trim()
      || internal.operation.sourceBranch
      || `analytix-handoff-${internal.operation.id.slice(0, 8)}`

    const patch = await this.stashSourceForPatch(internal, sourceWorkspace, 'stash-source-changes')
    this.step(internal, 'detach-worktree-branch', 'running')
    await git(sourceWorkspace, ['switch', '--detach', 'HEAD'], 30_000)
    this.step(internal, 'detach-worktree-branch', 'done')
    this.step(internal, 'checkout-local-branch', 'running')
    await switchBranch(targetWorkspace, internal.operation.localBranch)
    this.step(internal, 'checkout-local-branch', 'done')
    this.step(internal, 'apply-changes-to-local', 'running')
    await gitApplyPatch(targetWorkspace, patch)
    this.step(internal, 'apply-changes-to-local', 'done')
    if (typeof request.poolIndex === 'number') {
      await releaseWorktree({ projectPath: request.projectPath, poolIndex: request.poolIndex })
    }
  }

  private async stashSourceForPatch(
    internal: InternalOperation,
    cwd: string,
    stepId: ThreadHandoffStepId
  ): Promise<string> {
    this.step(internal, stepId, 'running')
    const stashRef = await stashChanges(cwd, internal.operation.id)
    internal.sourceStashRef = stashRef
    internal.sourceStashCwd = stashRef ? cwd : null
    const patch = stashRef ? await stashPatch(cwd, stashRef) : ''
    this.step(internal, stepId, 'done')
    return patch
  }

  private async rollbackSourceStash(internal: InternalOperation): Promise<void> {
    if (!internal.sourceStashRef || !internal.sourceStashCwd || internal.sourceStashDropped) return
    try {
      await restoreStash(internal.sourceStashCwd, internal.sourceStashRef)
      internal.sourceStashDropped = true
    } catch (error) {
      internal.operation.warningMessage = `Rollback failed: ${errorMessage(error)}`
    }
  }

  private async markError(internal: InternalOperation, error: unknown): Promise<void> {
    const active = internal.operation.steps.find((step) => step.status === 'running')
    if (active) active.status = 'failed'
    await this.rollbackSourceStash(internal)
    internal.operation.status = internal.operation.warningMessage ? 'warning' : 'error'
    internal.operation.errorMessage = errorMessage(error)
    internal.operation.execOutput = commandOutput('thread-handoff', error)
    internal.operation.hasUnseenTerminalState = true
    internal.operation.updatedAt = nowIso()
    this.emit(internal)
  }
}

export const threadHandoffService = new ThreadHandoffService()

export async function gitVersionForThreadHandoffSmoke(): Promise<string> {
  const { stdout } = await execFileAsync('git', ['--version'])
  return String(stdout).trim()
}
