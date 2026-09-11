import { cp, mkdir, readFile, realpath, rm, stat, writeFile } from 'node:fs/promises'
import { dirname, isAbsolute, join, normalize, resolve, sep } from 'node:path'
import { randomUUID } from 'node:crypto'
import { runGit, resolveGitCwd } from './git-service'
import { logInfo } from '../logger'
import type {
  GitCheckpointCreateResult,
  GitCheckpointRestoreResult
} from '../../shared/git-checkpoint'

const MAX_CHECKPOINT_UNTRACKED_FILES = 500
const MAX_CHECKPOINT_UNTRACKED_BYTES = 50 * 1024 * 1024
const MIN_HEAVY_CHECKPOINT_BUDGET_MS = 250

const checkpointCreateQueues = new Map<string, Promise<GitCheckpointCreateResult>>()

type GitCheckpointMetadata = {
  checkpointId: string
  threadId: string
  repositoryRoot: string
  head: string
  checkpointRef?: string | null
  currentBranch: string | null
  createdAt: string
  untrackedFiles: string[]
  skippedUntracked?: string[]
  partial?: boolean
}

class GitCheckpointTimeoutError extends Error {
  constructor(timeoutMs: number) {
    super(`Git checkpoint did not finish within ${timeoutMs}ms.`)
    this.name = 'GitCheckpointTimeoutError'
  }
}

function checkpointFailure(error: unknown): Extract<GitCheckpointCreateResult, { ok: false }> {
  const message = error instanceof Error ? error.message : String(error)
  if (error instanceof GitCheckpointTimeoutError || /timed out|timeout/i.test(message)) {
    return { ok: false, reason: 'timeout', message }
  }
  if (/not a git repository/i.test(message)) {
    return { ok: false, reason: 'not_git_repo', message: 'The working directory is not a Git repository.' }
  }
  if (/ENOENT/i.test(message) || /spawn git/i.test(message)) {
    return { ok: false, reason: 'git_unavailable', message: 'Git executable was not found.' }
  }
  return { ok: false, reason: 'error', message }
}

function restoreFailure(error: unknown): Extract<GitCheckpointRestoreResult, { ok: false }> {
  const message = error instanceof Error ? error.message : String(error)
  if (/not a git repository/i.test(message)) {
    return { ok: false, reason: 'not_git_repo', message: 'The working directory is not a Git repository.' }
  }
  if (/ENOENT/i.test(message) || /spawn git/i.test(message)) {
    return { ok: false, reason: 'git_unavailable', message: 'Git executable was not found.' }
  }
  return { ok: false, reason: 'error', message }
}

function checkpointDir(dataDir: string, checkpointId: string): string {
  return join(resolve(dataDir), 'git-checkpoints', checkpointId)
}

function checkpointHeadBundlePath(dataDir: string, checkpointId: string): string {
  return join(checkpointDir(dataDir, checkpointId), 'head.bundle')
}

function metadataPath(dataDir: string, checkpointId: string): string {
  return join(checkpointDir(dataDir, checkpointId), 'metadata.json')
}

async function fileExists(path: string): Promise<boolean> {
  try {
    await stat(path)
    return true
  } catch {
    return false
  }
}

function splitNul(stdout: string): string[] {
  return stdout.split('\0').map((entry) => entry.trim()).filter(Boolean)
}

function remainingTimeoutMs(deadlineAt: number | null, fallbackMs: number): number {
  if (!deadlineAt) return fallbackMs
  return Math.max(100, Math.min(fallbackMs, deadlineAt - Date.now()))
}

function throwIfTimedOut(deadlineAt: number | null, timeoutMs?: number): void {
  if (!deadlineAt || Date.now() < deadlineAt) return
  throw new GitCheckpointTimeoutError(timeoutMs ?? 0)
}

function throwIfInsufficientHeavyBudget(deadlineAt: number | null, timeoutMs?: number): void {
  if (!deadlineAt) return
  if (deadlineAt - Date.now() >= MIN_HEAVY_CHECKPOINT_BUDGET_MS) return
  throw new GitCheckpointTimeoutError(timeoutMs ?? 0)
}

async function assertNoUnmerged(repositoryRoot: string, timeout = 10_000): Promise<void> {
  const { stdout } = await runGit(repositoryRoot, ['diff', '--name-only', '--diff-filter=U'], timeout)
  const conflicted = stdout.split('\n').map((line) => line.trim()).filter(Boolean)
  if (conflicted.length > 0) {
    throw new Error(`Cannot create or restore a checkpoint while ${conflicted.length} files have merge conflicts.`)
  }
}

async function readMetadata(dataDir: string, checkpointId: string): Promise<GitCheckpointMetadata | null> {
  try {
    const raw = await readFile(metadataPath(dataDir, checkpointId), 'utf-8')
    return JSON.parse(raw) as GitCheckpointMetadata
  } catch {
    return null
  }
}

async function writePatch(repositoryRoot: string, args: string[], path: string, timeout = 30_000): Promise<void> {
  const { stdout } = await runGit(repositoryRoot, args, timeout)
  await writeFile(path, stdout, 'utf-8')
}

async function applyPatchIfPresent(repositoryRoot: string, path: string, cached: boolean): Promise<void> {
  const info = await stat(path).catch(() => null)
  if (!info || info.size === 0) return
  await runGit(repositoryRoot, ['apply', '--binary', ...(cached ? ['--index'] : []), path], 30_000)
}

async function commitExists(repositoryRoot: string, rev: string): Promise<boolean> {
  if (!rev.trim()) return false
  try {
    await runGit(repositoryRoot, ['cat-file', '-e', `${rev}^{commit}`])
    return true
  } catch {
    return false
  }
}

async function writeHeadBundle(repositoryRoot: string, path: string, timeout = 30_000): Promise<void> {
  await runGit(repositoryRoot, ['bundle', 'create', path, 'HEAD'], timeout)
}

function assertSafeRelativeCheckpointPath(relativePath: string): void {
  if (!relativePath || relativePath === '.' || relativePath === '..' || isAbsolute(relativePath)) {
    throw new Error(`invalid untracked path: ${relativePath}`)
  }
  if (relativePath.includes('\0') || /^[a-zA-Z]:/.test(relativePath)) {
    throw new Error(`invalid untracked path: ${relativePath}`)
  }
}

async function safeRealpath(target: string): Promise<string | null> {
  try {
    return await realpath(target)
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code
    if (code === 'ENOENT' || code === 'ENOTDIR') return null
    throw error
  }
}

async function resolveSymlinkSafeTarget(lexicalPath: string): Promise<string> {
  const direct = await safeRealpath(lexicalPath)
  if (direct) return direct

  const missingSegments: string[] = []
  let current = lexicalPath
  for (let index = 0; index < 128 && current !== dirname(current); index += 1) {
    const resolved = await safeRealpath(current)
    if (resolved) {
      return missingSegments.length > 0 ? normalize(join(resolved, ...missingSegments)) : resolved
    }
    missingSegments.unshift(current.split(sep).pop() || '')
    current = dirname(current)
  }
  throw new Error(`cannot canonicalize path: ${lexicalPath}`)
}

async function resolvePathWithinRepository(
  repositoryRoot: string,
  relativePath: string
): Promise<string> {
  assertSafeRelativeCheckpointPath(relativePath)
  const repositoryReal = await realpath(repositoryRoot)
  const lexicalTarget = normalize(join(repositoryReal, relativePath))
  if (lexicalTarget !== repositoryReal && !lexicalTarget.startsWith(repositoryReal + sep)) {
    throw new Error(`untracked path escapes the repository root: ${relativePath}`)
  }
  const realTarget = await resolveSymlinkSafeTarget(lexicalTarget)
  if (realTarget !== repositoryReal && !realTarget.startsWith(repositoryReal + sep)) {
    throw new Error(`untracked path escapes the repository root: ${relativePath}`)
  }
  return lexicalTarget
}

async function resolveExistingPathWithinBase(
  baseRoot: string,
  relativePath: string
): Promise<string | null> {
  assertSafeRelativeCheckpointPath(relativePath)
  const baseReal = await realpath(baseRoot)
  const lexicalTarget = normalize(join(baseReal, relativePath))
  if (lexicalTarget !== baseReal && !lexicalTarget.startsWith(baseReal + sep)) {
    throw new Error(`checkpoint source path escapes its root: ${relativePath}`)
  }
  const realTarget = await safeRealpath(lexicalTarget)
  if (!realTarget) return null
  if (realTarget !== baseReal && !realTarget.startsWith(baseReal + sep)) {
    throw new Error(`checkpoint source path escapes its root: ${relativePath}`)
  }
  return realTarget
}

async function resolveRestorableUntrackedFiles(
  repositoryRoot: string,
  checkpointUntrackedRoot: string,
  relativePaths: string[]
): Promise<Array<{ from: string; to: string }>> {
  const files: Array<{ from: string; to: string }> = []
  for (const relativePath of relativePaths) {
    const from = await resolveExistingPathWithinBase(checkpointUntrackedRoot, relativePath)
    if (!from) continue
    files.push({
      from,
      to: await resolvePathWithinRepository(repositoryRoot, relativePath)
    })
  }
  return files
}

type CheckpointStageTimings = Record<string, number>

function markStage(timings: CheckpointStageTimings, stage: string, startedAt: number): void {
  timings[stage] = Date.now() - startedAt
}

function logCheckpointTiming(data: {
  threadId: string
  repositoryRoot?: string
  ok: boolean
  reason?: string
  timings: CheckpointStageTimings
  untrackedCount?: number
  untrackedBytes?: number
}): void {
  logInfo(
    'git-checkpoint',
    'checkpoint timing',
    {
      ok: data.ok,
      untrackedCount: data.untrackedCount ?? 0,
      untrackedBytes: data.untrackedBytes ?? 0
    }
  )
}

async function measureUntrackedFiles(repositoryRoot: string, files: string[]): Promise<{
  ok: true
  totalBytes: number
  includedFiles: string[]
  skippedFiles: string[]
}> {
  if (files.length > MAX_CHECKPOINT_UNTRACKED_FILES) {
    return {
      ok: true,
      totalBytes: 0,
      includedFiles: [],
      skippedFiles: files
    }
  }
  let totalBytes = 0
  const includedFiles: string[] = []
  const skippedFiles: string[] = []
  for (const relativePath of files) {
    const info = await stat(join(repositoryRoot, relativePath)).catch(() => null)
    if (!info) continue
    if (totalBytes + info.size > MAX_CHECKPOINT_UNTRACKED_BYTES) {
      skippedFiles.push(relativePath)
      continue
    }
    totalBytes += info.size
    includedFiles.push(relativePath)
  }
  return { ok: true, totalBytes, includedFiles, skippedFiles }
}

async function resolveCheckpointTarget(
  repositoryRoot: string,
  dataDir: string,
  metadata: GitCheckpointMetadata
): Promise<string> {
  const head = metadata.head.trim()
  if (await commitExists(repositoryRoot, head)) return head

  const bundlePath = checkpointHeadBundlePath(dataDir, metadata.checkpointId)
  if (await fileExists(bundlePath)) {
    await runGit(repositoryRoot, ['bundle', 'unbundle', bundlePath], 30_000)
    if (await commitExists(repositoryRoot, head)) return head
  }

  const legacyRef = metadata.checkpointRef?.trim() ?? ''
  if (await commitExists(repositoryRoot, legacyRef)) return legacyRef

  throw new Error(`Git checkpoint target commit is unavailable: ${head || metadata.checkpointId}`)
}

async function resolveRepositoryRoot(workspaceRoot: string, timeout = 10_000): Promise<string | null> {
  const cwd = await resolveGitCwd(workspaceRoot)
  if (!cwd) return null
  const { stdout } = await runGit(cwd, ['rev-parse', '--show-toplevel'], timeout)
  return stdout.trim()
}

function queueCheckpointCreate(
  repositoryRoot: string,
  run: () => Promise<GitCheckpointCreateResult>
): Promise<GitCheckpointCreateResult> {
  const previous = checkpointCreateQueues.get(repositoryRoot)
  let current: Promise<GitCheckpointCreateResult>
  current = (previous ? previous.catch(() => undefined).then(run) : run()).finally(() => {
    if (checkpointCreateQueues.get(repositoryRoot) === current) {
      checkpointCreateQueues.delete(repositoryRoot)
    }
  })
  checkpointCreateQueues.set(repositoryRoot, current)
  return current
}

export async function createGitCheckpoint(params: {
  dataDir: string
  workspaceRoot: string
  threadId: string
  checkpointId?: string
  timeoutMs?: number
}): Promise<GitCheckpointCreateResult> {
  const workspaceRoot = params.workspaceRoot.trim()
  const timeoutMs = params.timeoutMs
  const deadlineAt = timeoutMs ? Date.now() + timeoutMs : null
  const timings: CheckpointStageTimings = {}
  let repositoryRootForLog: string | undefined
  let untrackedCountForLog = 0
  let untrackedBytesForLog = 0
  let checkpointDirForCleanup: string | null = null
  if (!workspaceRoot) {
    return { ok: false, reason: 'no_workspace', message: 'No working directory selected.' }
  }
  try {
    throwIfTimedOut(deadlineAt, timeoutMs)
    let stageStartedAt = Date.now()
    const repositoryRoot = await resolveRepositoryRoot(
      workspaceRoot,
      remainingTimeoutMs(deadlineAt, 10_000)
    )
    markStage(timings, 'resolveRepositoryRootMs', stageStartedAt)
    if (!repositoryRoot) {
      return { ok: false, reason: 'no_workspace', message: 'No working directory selected.' }
    }
    repositoryRootForLog = repositoryRoot
    return await queueCheckpointCreate(repositoryRoot, async () => {
      throwIfTimedOut(deadlineAt, timeoutMs)
      stageStartedAt = Date.now()
      await assertNoUnmerged(repositoryRoot, remainingTimeoutMs(deadlineAt, 10_000))
      markStage(timings, 'assertNoUnmergedMs', stageStartedAt)

      const checkpointId = params.checkpointId?.trim() || `gcp_${Date.now()}_${randomUUID()}`
      throwIfTimedOut(deadlineAt, timeoutMs)
      stageStartedAt = Date.now()
      const head = (await runGit(repositoryRoot, ['rev-parse', 'HEAD'], remainingTimeoutMs(deadlineAt, 10_000))).stdout.trim()
      markStage(timings, 'revParseHeadMs', stageStartedAt)
      throwIfTimedOut(deadlineAt, timeoutMs)
      stageStartedAt = Date.now()
      const currentBranchRaw = (await runGit(repositoryRoot, ['branch', '--show-current'], remainingTimeoutMs(deadlineAt, 10_000))).stdout.trim()
      markStage(timings, 'branchMs', stageStartedAt)
      const currentBranch = currentBranchRaw || null
      throwIfTimedOut(deadlineAt, timeoutMs)
      stageStartedAt = Date.now()
      const untrackedFiles = splitNul(
        (await runGit(
          repositoryRoot,
          ['ls-files', '--others', '--exclude-standard', '-z'],
          remainingTimeoutMs(deadlineAt, 10_000)
        )).stdout
      )
      untrackedCountForLog = untrackedFiles.length
      markStage(timings, 'listUntrackedMs', stageStartedAt)

      throwIfTimedOut(deadlineAt, timeoutMs)
      stageStartedAt = Date.now()
      const untrackedMeasure = await measureUntrackedFiles(repositoryRoot, untrackedFiles)
      untrackedBytesForLog = untrackedMeasure.totalBytes
      markStage(timings, 'measureUntrackedMs', stageStartedAt)

      throwIfTimedOut(deadlineAt, timeoutMs)
      throwIfInsufficientHeavyBudget(deadlineAt, timeoutMs)
      const dir = checkpointDir(params.dataDir, checkpointId)
      checkpointDirForCleanup = dir
      await rm(dir, { recursive: true, force: true })
      await mkdir(join(dir, 'untracked'), { recursive: true })

      throwIfTimedOut(deadlineAt, timeoutMs)
      throwIfInsufficientHeavyBudget(deadlineAt, timeoutMs)
      stageStartedAt = Date.now()
      await writeHeadBundle(
        repositoryRoot,
        checkpointHeadBundlePath(params.dataDir, checkpointId),
        remainingTimeoutMs(deadlineAt, 30_000)
      )
      markStage(timings, 'headBundleMs', stageStartedAt)
      throwIfTimedOut(deadlineAt, timeoutMs)
      throwIfInsufficientHeavyBudget(deadlineAt, timeoutMs)
      stageStartedAt = Date.now()
      await writePatch(
        repositoryRoot,
        ['diff', '--binary'],
        join(dir, 'unstaged.patch'),
        remainingTimeoutMs(deadlineAt, 30_000)
      )
      markStage(timings, 'unstagedDiffMs', stageStartedAt)
      throwIfTimedOut(deadlineAt, timeoutMs)
      stageStartedAt = Date.now()
      await writePatch(
        repositoryRoot,
        ['diff', '--cached', '--binary'],
        join(dir, 'staged.patch'),
        remainingTimeoutMs(deadlineAt, 30_000)
      )
      markStage(timings, 'stagedDiffMs', stageStartedAt)

      throwIfTimedOut(deadlineAt, timeoutMs)
      throwIfInsufficientHeavyBudget(deadlineAt, timeoutMs)
      stageStartedAt = Date.now()
      for (const relativePath of untrackedMeasure.includedFiles) {
        throwIfTimedOut(deadlineAt, timeoutMs)
        const from = join(repositoryRoot, relativePath)
        const to = join(dir, 'untracked', relativePath)
        await mkdir(dirname(to), { recursive: true })
        await cp(from, to, { recursive: true, force: true, errorOnExist: false })
      }
      markStage(timings, 'copyUntrackedMs', stageStartedAt)

      const metadata: GitCheckpointMetadata = {
        checkpointId,
        threadId: params.threadId,
        repositoryRoot,
        head,
        currentBranch,
        createdAt: new Date().toISOString(),
        untrackedFiles: untrackedMeasure.includedFiles,
        skippedUntracked: untrackedMeasure.skippedFiles,
        partial: untrackedMeasure.skippedFiles.length > 0
      }
      throwIfTimedOut(deadlineAt, timeoutMs)
      stageStartedAt = Date.now()
      await writeFile(join(dir, 'metadata.json'), JSON.stringify(metadata, null, 2), 'utf-8')
      markStage(timings, 'writeMetadataMs', stageStartedAt)
      logCheckpointTiming({
        threadId: params.threadId,
        repositoryRoot,
        ok: true,
        reason: metadata.partial ? 'partial' : undefined,
        timings,
        untrackedCount: untrackedFiles.length,
        untrackedBytes: untrackedMeasure.totalBytes
      })
      return {
        ok: true,
        checkpointId,
        repositoryRoot,
        head,
        currentBranch,
        partial: metadata.partial,
        skippedUntracked: metadata.skippedUntracked
      }
    })
  } catch (error) {
    const failure = checkpointFailure(error)
    if (/merge conflicts/i.test(failure.message)) {
      const conflictFailure = { ...failure, reason: 'conflict' as const }
      logCheckpointTiming({
        threadId: params.threadId,
        repositoryRoot: repositoryRootForLog,
        ok: false,
        reason: conflictFailure.reason,
        timings,
        untrackedCount: untrackedCountForLog,
        untrackedBytes: untrackedBytesForLog
      })
      return conflictFailure
    }
    logCheckpointTiming({
      threadId: params.threadId,
      repositoryRoot: repositoryRootForLog,
      ok: false,
      reason: failure.reason,
      timings,
      untrackedCount: untrackedCountForLog,
      untrackedBytes: untrackedBytesForLog
    })
    if (failure.reason === 'timeout' && checkpointDirForCleanup) {
      await rm(checkpointDirForCleanup, { recursive: true, force: true }).catch(() => undefined)
    }
    return failure
  }
}

export async function restoreGitCheckpoint(params: {
  dataDir: string
  checkpointId: string
  allowPartialRestore?: boolean
}): Promise<GitCheckpointRestoreResult> {
  const checkpointId = params.checkpointId.trim()
  const metadata = await readMetadata(params.dataDir, checkpointId)
  if (!metadata) {
    return { ok: false, reason: 'not_found', message: `Git checkpoint not found: ${checkpointId}` }
  }
  const skippedUntracked = metadata.skippedUntracked ?? []
  if (metadata.partial === true && params.allowPartialRestore !== true) {
    return {
      ok: false,
      reason: 'partial',
      message: `Git checkpoint ${checkpointId} is partial and skipped ${skippedUntracked.length} untracked files. Confirm partial restore before applying it.`,
      skippedUntracked
    }
  }
  try {
    const repositoryRoot = metadata.repositoryRoot
    await assertNoUnmerged(repositoryRoot)
    const targetRef = await resolveCheckpointTarget(repositoryRoot, params.dataDir, metadata)
    const dir = checkpointDir(params.dataDir, checkpointId)
    const restorableUntrackedFiles = await resolveRestorableUntrackedFiles(
      repositoryRoot,
      join(dir, 'untracked'),
      metadata.untrackedFiles
    )

    const rescue = await createGitCheckpoint({
      dataDir: params.dataDir,
      workspaceRoot: repositoryRoot,
      threadId: `${metadata.threadId}:rollback-rescue`
    })
    const rescueCheckpointId = rescue.ok ? rescue.checkpointId : null

    await runGit(repositoryRoot, ['reset', '--hard'], 30_000)
    await runGit(repositoryRoot, ['clean', '-fd'], 30_000)
    if (metadata.currentBranch) {
      await runGit(repositoryRoot, ['checkout', '-B', metadata.currentBranch, targetRef], 30_000)
    } else {
      await runGit(repositoryRoot, ['checkout', '--detach', targetRef], 30_000)
    }
    await runGit(repositoryRoot, ['reset', '--hard', targetRef], 30_000)
    await runGit(repositoryRoot, ['clean', '-fd'], 30_000)

    await applyPatchIfPresent(repositoryRoot, join(dir, 'staged.patch'), true)
    await applyPatchIfPresent(repositoryRoot, join(dir, 'unstaged.patch'), false)

    for (const file of restorableUntrackedFiles) {
      await mkdir(dirname(file.to), { recursive: true })
      await cp(file.from, file.to, { recursive: true, force: true, errorOnExist: false })
    }

    return {
      ok: true,
      checkpointId,
      repositoryRoot,
      head: metadata.head,
      currentBranch: metadata.currentBranch,
      rescueCheckpointId,
      partial: metadata.partial === true,
      skippedUntracked
    }
  } catch (error) {
    const failure = restoreFailure(error)
    if (/merge conflicts/i.test(failure.message)) {
      return { ...failure, reason: 'conflict' }
    }
    return failure
  }
}
