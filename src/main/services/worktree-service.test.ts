import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { execFileSync } from 'node:child_process'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { existsSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { acquireWorktree, listWorktrees, releaseWorktree } from './worktree-service'

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

beforeEach(async () => {
  sandbox = await mkdtemp(join(tmpdir(), 'analytix-worktree-service-'))
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

describe('worktree-service', () => {
  it('acquires, lists, and releases a real git worktree pool slot', async () => {
    const info = await acquireWorktree({
      projectPath: repoRoot,
      poolIndex: 0,
      taskId: 'task-1',
      worktreeRoot
    })

    expect(info).toMatchObject({
      poolIndex: 0,
      branch: 'analytix-pool-0',
      inUse: true,
      taskId: 'task-1',
      changesCount: 0
    })
    expect(existsSync(join(info.path, '.git'))).toBe(true)
    await expect(readFile(join(info.path, 'README.md'), 'utf8')).resolves.toBe('base\n')

    const occupied = await listWorktrees({ projectPath: repoRoot, worktreeRoot })
    expect(occupied.isGitRepo).toBe(true)
    expect(occupied.worktrees).toEqual([
      expect.objectContaining({ poolIndex: 0, inUse: true, taskId: 'task-1' })
    ])

    await releaseWorktree({ projectPath: repoRoot, poolIndex: 0 })
    const released = await listWorktrees({ projectPath: repoRoot, worktreeRoot })
    expect(released.worktrees).toEqual([
      expect.objectContaining({ poolIndex: 0, inUse: false, taskId: null })
    ])
  })
})
