import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { execFileSync } from 'node:child_process'
import { mkdtemp, readFile, rm, stat, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createGitCheckpoint, restoreGitCheckpoint } from './git-checkpoint-service'

let sandbox = ''
let repoRoot = ''
let dataDir = ''

beforeEach(async () => {
  sandbox = await mkdtemp(join(tmpdir(), 'analytix-git-checkpoint-'))
  repoRoot = join(sandbox, 'repo')
  dataDir = join(sandbox, 'data')
  execFileSync('git', ['init', '-b', 'main', repoRoot], { stdio: 'pipe' })
  execFileSync('git', ['-C', repoRoot, 'config', 'user.email', 'test@example.com'], { stdio: 'pipe' })
  execFileSync('git', ['-C', repoRoot, 'config', 'user.name', 'Test'], { stdio: 'pipe' })
  await writeFile(join(repoRoot, 'tracked.txt'), 'base\n')
  await writeFile(join(repoRoot, 'staged.txt'), 'staged base\n')
  execFileSync('git', ['-C', repoRoot, 'add', '.'], { stdio: 'pipe' })
  execFileSync('git', ['-C', repoRoot, 'commit', '-m', 'init'], { stdio: 'pipe' })
})

afterEach(async () => {
  if (!sandbox) return
  await rm(sandbox, { recursive: true, force: true })
  sandbox = ''
  repoRoot = ''
  dataDir = ''
})

describe('git checkpoint service', () => {
  it('stores checkpoint heads outside visible git refs', async () => {
    const checkpoint = await createGitCheckpoint({
      dataDir,
      workspaceRoot: repoRoot,
      threadId: 'thr_1'
    })
    expect(checkpoint.ok).toBe(true)
    if (!checkpoint.ok) throw new Error(checkpoint.message)

    const checkpointDir = join(dataDir, 'git-checkpoints', checkpoint.checkpointId)
    const metadata = JSON.parse(await readFile(join(checkpointDir, 'metadata.json'), 'utf-8')) as {
      checkpointRef?: string
    }
    expect(metadata.checkpointRef).toBeUndefined()
    await expect(stat(join(checkpointDir, 'head.bundle'))).resolves.toBeTruthy()

    const refs = execFileSync('git', ['-C', repoRoot, 'show-ref'], { encoding: 'utf-8' })
    expect(refs).not.toContain('refs/analytix/checkpoints')
  })

  it('restores staged, unstaged, and untracked files to the checkpoint state', async () => {
    await writeFile(join(repoRoot, 'tracked.txt'), 'checkpoint unstaged\n')
    await writeFile(join(repoRoot, 'staged.txt'), 'checkpoint staged\n')
    execFileSync('git', ['-C', repoRoot, 'add', 'staged.txt'], { stdio: 'pipe' })
    await writeFile(join(repoRoot, 'untracked.txt'), 'checkpoint untracked\n')

    const checkpoint = await createGitCheckpoint({
      dataDir,
      workspaceRoot: repoRoot,
      threadId: 'thr_1'
    })
    expect(checkpoint.ok).toBe(true)
    if (!checkpoint.ok) throw new Error(checkpoint.message)

    await writeFile(join(repoRoot, 'tracked.txt'), 'agent changed\n')
    await writeFile(join(repoRoot, 'staged.txt'), 'agent staged changed\n')
    execFileSync('git', ['-C', repoRoot, 'add', 'tracked.txt', 'staged.txt'], { stdio: 'pipe' })
    await writeFile(join(repoRoot, 'untracked.txt'), 'agent changed untracked\n')
    await writeFile(join(repoRoot, 'agent-new.txt'), 'agent new\n')

    const restored = await restoreGitCheckpoint({
      dataDir,
      checkpointId: checkpoint.checkpointId
    })
    expect(restored.ok).toBe(true)
    if (!restored.ok) throw new Error(restored.message)

    expect(await readFile(join(repoRoot, 'tracked.txt'), 'utf-8')).toBe('checkpoint unstaged\n')
    expect(await readFile(join(repoRoot, 'staged.txt'), 'utf-8')).toBe('checkpoint staged\n')
    expect(await readFile(join(repoRoot, 'untracked.txt'), 'utf-8')).toBe('checkpoint untracked\n')
    expect(execFileSync('git', ['-C', repoRoot, 'status', '--porcelain=v1'], { encoding: 'utf-8' })
      .split('\n')
      .filter(Boolean)
      .sort()).toEqual([' M tracked.txt', 'M  staged.txt', '?? untracked.txt'].sort())
  })

  it('rolls back commits created after the checkpoint', async () => {
    const checkpoint = await createGitCheckpoint({
      dataDir,
      workspaceRoot: repoRoot,
      threadId: 'thr_1'
    })
    expect(checkpoint.ok).toBe(true)
    if (!checkpoint.ok) throw new Error(checkpoint.message)

    await writeFile(join(repoRoot, 'tracked.txt'), 'committed by agent\n')
    execFileSync('git', ['-C', repoRoot, 'add', 'tracked.txt'], { stdio: 'pipe' })
    execFileSync('git', ['-C', repoRoot, 'commit', '-m', 'agent commit'], { stdio: 'pipe' })
    await writeFile(join(repoRoot, 'after-commit.txt'), 'uncommitted after commit\n')

    const restored = await restoreGitCheckpoint({
      dataDir,
      checkpointId: checkpoint.checkpointId
    })
    expect(restored.ok).toBe(true)
    if (!restored.ok) throw new Error(restored.message)

    expect(await readFile(join(repoRoot, 'tracked.txt'), 'utf-8')).toBe('base\n')
    expect(restored.rescueCheckpointId).toMatch(/^gcp_/)
    expect(execFileSync('git', ['-C', repoRoot, 'rev-parse', 'HEAD'], { encoding: 'utf-8' }).trim()).toBe(
      checkpoint.head
    )
    expect(execFileSync('git', ['-C', repoRoot, 'status', '--porcelain=v1'], { encoding: 'utf-8' }).trim()).toBe('')
  })

  it('creates partial checkpoints for oversized untracked snapshots and blocks restore by default', async () => {
    await writeFile(join(repoRoot, 'tracked.txt'), 'checkpoint unstaged\n')
    for (let index = 0; index < 501; index += 1) {
      await writeFile(join(repoRoot, `untracked-${index}.txt`), `file ${index}\n`)
    }

    const checkpoint = await createGitCheckpoint({
      dataDir,
      workspaceRoot: repoRoot,
      threadId: 'thr_partial'
    })
    expect(checkpoint.ok).toBe(true)
    if (!checkpoint.ok) throw new Error(checkpoint.message)
    expect(checkpoint.partial).toBe(true)
    expect(checkpoint.skippedUntracked?.length).toBe(501)

    const blocked = await restoreGitCheckpoint({
      dataDir,
      checkpointId: checkpoint.checkpointId
    })
    expect(blocked.ok).toBe(false)
    if (blocked.ok) throw new Error('partial restore should be blocked by default')
    expect(blocked.reason).toBe('partial')
    expect(blocked.skippedUntracked?.length).toBe(501)

    const restored = await restoreGitCheckpoint({
      dataDir,
      checkpointId: checkpoint.checkpointId,
      allowPartialRestore: true
    })
    expect(restored.ok).toBe(true)
    if (!restored.ok) throw new Error(restored.message)
    expect(restored.partial).toBe(true)
  })

  it('returns timeout when checkpoint creation exceeds the caller budget', async () => {
    const checkpoint = await createGitCheckpoint({
      dataDir,
      workspaceRoot: repoRoot,
      threadId: 'thr_timeout',
      timeoutMs: -1
    })

    expect(checkpoint.ok).toBe(false)
    if (checkpoint.ok) throw new Error('checkpoint should time out')
    expect(checkpoint.reason).toBe('timeout')
  })

  it('rejects unsafe untracked restore paths before mutating the repository', async () => {
    await writeFile(join(repoRoot, 'untracked.txt'), 'checkpoint untracked\n')
    const checkpoint = await createGitCheckpoint({
      dataDir,
      workspaceRoot: repoRoot,
      threadId: 'thr_unsafe_restore'
    })
    expect(checkpoint.ok).toBe(true)
    if (!checkpoint.ok) throw new Error(checkpoint.message)

    const checkpointDir = join(dataDir, 'git-checkpoints', checkpoint.checkpointId)
    const metadataPath = join(checkpointDir, 'metadata.json')
    const metadata = JSON.parse(await readFile(metadataPath, 'utf-8')) as {
      untrackedFiles: string[]
    }
    metadata.untrackedFiles = ['../outside.txt']
    await writeFile(metadataPath, JSON.stringify(metadata, null, 2), 'utf-8')
    await writeFile(join(repoRoot, 'tracked.txt'), 'agent changed before failed restore\n')

    const restored = await restoreGitCheckpoint({
      dataDir,
      checkpointId: checkpoint.checkpointId
    })

    expect(restored.ok).toBe(false)
    if (restored.ok) throw new Error('unsafe restore should fail')
    expect(restored.reason).toBe('error')
    expect(restored.message).toMatch(/invalid untracked path|escapes/)
    expect(await readFile(join(repoRoot, 'tracked.txt'), 'utf-8')).toBe('agent changed before failed restore\n')
  })
})
