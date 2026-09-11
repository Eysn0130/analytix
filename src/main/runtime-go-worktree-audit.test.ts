import { spawnSync } from 'node:child_process'
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { delimiter, join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { afterEach, describe, expect, it } from 'vitest'

type WorktreeAuditModule = {
  worktreeAudit(): Record<string, unknown>
  worktreeFinalGateBlockers(worktree: Record<string, unknown>): string[]
  relevantEvidenceFinalGateBlockers(worktree: Record<string, unknown>): string[]
}

let tempDirs: string[] = []

afterEach(() => {
  for (const dir of tempDirs) rmSync(dir, { recursive: true, force: true })
  tempDirs = []
})

function tempDir(): string {
  const dir = mkdtempSync(join(tmpdir(), 'analytix-worktree-audit-'))
  tempDirs.push(dir)
  return dir
}

function writeExecutable(path: string, source: string): void {
  writeFileSync(path, source, { encoding: 'utf8', mode: 0o755 })
}

async function loadWorktreeAudit(): Promise<WorktreeAuditModule> {
  return await import(pathToFileURL(join(process.cwd(), 'scripts/runtime-go-worktree-audit.mjs')).href) as WorktreeAuditModule
}

describe('runtime Go worktree audit helper', () => {
  it('classifies decoded goal paths and generated non-goal artifacts consistently', async () => {
    const dir = tempDir()
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const originalPath = process.env.PATH
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'status' && args[1] === '--porcelain=v1') {
  console.log([
    ' M backend/app/repositories/__pycache__/flow_repository.cpython-311.pyc',
    '?? tmp/cache.pyc',
    ' M scripts/runtime-go-worktree-audit.mjs',
    ' M docs/model-provider-presets.md',
    ' M "\\\\351\\\\207\\\\215\\\\346\\\\236\\\\204\\\\345\\\\215\\\\207\\\\347\\\\272\\\\247\\\\346\\\\226\\\\271\\\\346\\\\241\\\\210.md"'
  ].join('\\n'))
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

    try {
      process.env.PATH = `${dir}${delimiter}${originalPath || ''}`
      const { relevantEvidenceFinalGateBlockers, worktreeAudit, worktreeFinalGateBlockers } = await loadWorktreeAudit()
      const audit = worktreeAudit()

      expect(audit).toEqual(expect.objectContaining({
        status: 'dirty',
        dirtyFileCount: 5,
        goalScopeDirtyFileCount: 3,
        nonGoalScopeDirtyFileCount: 2,
        unclassifiedNonGoalScopeDirtyFileCount: 1,
        classifiedNonGoalScopeDirtyFileCount: 1,
        generatedArtifactDirtyFileCount: 2,
        unclassifiedGeneratedArtifactDirtyFileCount: 1,
        classifiedGeneratedArtifactDirtyFileCount: 1,
        trackedGeneratedArtifactDirtyFileCount: 1,
        unclassifiedTrackedGeneratedArtifactDirtyFileCount: 0,
        classifiedTrackedGeneratedArtifactDirtyFileCount: 1,
        untrackedGeneratedArtifactDirtyFileCount: 1,
        relevantEvidenceDirtyFileCount: 2,
        relevantEvidenceStagedDirtyFileCount: 0,
        relevantEvidenceUnstagedDirtyFileCount: 2,
        relevantEvidenceIntentionallyStaged: false,
        finalAcceptanceRequiresCleanOrClassifiedTree: true
      }))
      expect(audit).toEqual(expect.objectContaining({
        runtimeScopeDirtyFiles: expect.arrayContaining([
          ' M scripts/runtime-go-worktree-audit.mjs',
          ' M docs/model-provider-presets.md'
        ]),
        relevantEvidenceDirtyFiles: expect.arrayContaining([
          ' M scripts/runtime-go-worktree-audit.mjs',
          ' M docs/model-provider-presets.md'
        ]),
        relevantEvidenceUnstagedDirtyFiles: expect.arrayContaining([
          ' M scripts/runtime-go-worktree-audit.mjs',
          ' M docs/model-provider-presets.md'
        ]),
        relevantEvidenceStagedDirtyFiles: [],
        nonGoalScopeDirtyFiles: [
          ' M backend/app/repositories/__pycache__/flow_repository.cpython-311.pyc',
          '?? tmp/cache.pyc'
        ],
        unclassifiedNonGoalScopeDirtyFiles: [
          '?? tmp/cache.pyc'
        ],
        classifiedNonGoalScopeDirtyFiles: [
          ' M backend/app/repositories/__pycache__/flow_repository.cpython-311.pyc'
        ],
        generatedArtifactDirtyFiles: [
          ' M backend/app/repositories/__pycache__/flow_repository.cpython-311.pyc',
          '?? tmp/cache.pyc'
        ],
        unclassifiedGeneratedArtifactDirtyFiles: [
          '?? tmp/cache.pyc'
        ],
        classifiedGeneratedArtifactDirtyFiles: [
          ' M backend/app/repositories/__pycache__/flow_repository.cpython-311.pyc'
        ],
        trackedGeneratedArtifactDirtyFiles: [
          ' M backend/app/repositories/__pycache__/flow_repository.cpython-311.pyc'
        ],
        unclassifiedTrackedGeneratedArtifactDirtyFiles: [],
        classifiedTrackedGeneratedArtifactDirtyFiles: [
          ' M backend/app/repositories/__pycache__/flow_repository.cpython-311.pyc'
        ],
        untrackedGeneratedArtifactDirtyFiles: [
          '?? tmp/cache.pyc'
        ]
      }))
      expect(audit.classifiedDirtyFileReasons).toEqual([
        expect.objectContaining({
          path: 'backend/app/repositories/__pycache__/flow_repository.cpython-311.pyc',
          reason: expect.stringContaining('pre-existing non-goal')
        })
      ])
      expect(JSON.stringify(audit)).toContain('重构升级方案.md')
      expect(worktreeFinalGateBlockers(audit)).toEqual([
        'worktree:non-goal-dirty-files',
        'worktree:generated-artifacts'
      ])
      expect(relevantEvidenceFinalGateBlockers(audit)).toEqual([
        'evidence:relevant-clean-scope-dirty'
      ])
    } finally {
      process.env.PATH = originalPath
    }
  })

  it('does not block final worktree gate for the classified tracked pyc alone', async () => {
    const dir = tempDir()
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const originalPath = process.env.PATH
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'status' && args[1] === '--porcelain=v1') {
  console.log(' M backend/app/repositories/__pycache__/flow_repository.cpython-311.pyc')
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

    try {
      process.env.PATH = `${dir}${delimiter}${originalPath || ''}`
      const { relevantEvidenceFinalGateBlockers, worktreeAudit, worktreeFinalGateBlockers } = await loadWorktreeAudit()
      const audit = worktreeAudit()

      expect(audit).toEqual(expect.objectContaining({
        status: 'dirty',
        dirtyFileCount: 1,
        nonGoalScopeDirtyFileCount: 1,
        unclassifiedNonGoalScopeDirtyFileCount: 0,
        classifiedNonGoalScopeDirtyFileCount: 1,
        generatedArtifactDirtyFileCount: 1,
        unclassifiedGeneratedArtifactDirtyFileCount: 0,
        classifiedGeneratedArtifactDirtyFileCount: 1,
        trackedGeneratedArtifactDirtyFileCount: 1,
        unclassifiedTrackedGeneratedArtifactDirtyFileCount: 0,
        classifiedTrackedGeneratedArtifactDirtyFileCount: 1,
        relevantEvidenceDirtyFileCount: 0,
        relevantEvidenceStagedDirtyFileCount: 0,
        relevantEvidenceUnstagedDirtyFileCount: 0,
        relevantEvidenceIntentionallyStaged: false
      }))
      expect(worktreeFinalGateBlockers(audit)).toEqual([])
      expect(relevantEvidenceFinalGateBlockers(audit)).toEqual([])
    } finally {
      process.env.PATH = originalPath
    }
  })

  it('classifies local OpenSpec workspaces as intentional non-goal dirty state', async () => {
    const dir = tempDir()
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const originalPath = process.env.PATH
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'status' && args[1] === '--porcelain=v1') {
  console.log([
    '?? openspec/changes/',
    '?? openspec/specs/'
  ].join('\\n'))
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

    try {
      process.env.PATH = `${dir}${delimiter}${originalPath || ''}`
      const { relevantEvidenceFinalGateBlockers, worktreeAudit, worktreeFinalGateBlockers } = await loadWorktreeAudit()
      const audit = worktreeAudit()

      expect(audit).toEqual(expect.objectContaining({
        status: 'dirty',
        dirtyFileCount: 2,
        nonGoalScopeDirtyFileCount: 2,
        unclassifiedNonGoalScopeDirtyFileCount: 0,
        classifiedNonGoalScopeDirtyFileCount: 2,
        generatedArtifactDirtyFileCount: 0,
        relevantEvidenceDirtyFileCount: 0
      }))
      expect(audit.classifiedNonGoalScopeDirtyFiles).toEqual([
        '?? openspec/changes/',
        '?? openspec/specs/'
      ])
      expect(audit.classifiedDirtyFileReasons).toEqual([
        expect.objectContaining({
          path: 'openspec/changes/',
          reason: expect.stringContaining('OpenSpec change workspace')
        }),
        expect.objectContaining({
          path: 'openspec/specs/',
          reason: expect.stringContaining('OpenSpec spec workspace')
        })
      ])
      expect(worktreeFinalGateBlockers(audit)).toEqual([])
      expect(relevantEvidenceFinalGateBlockers(audit)).toEqual([])
    } finally {
      process.env.PATH = originalPath
    }
  })

  it('tracks formal current evidence docs but not archived numbered evidence paths', async () => {
    const dir = tempDir()
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const originalPath = process.env.PATH
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'status' && args[1] === '--porcelain=v1') {
  console.log([
    ' M docs/analytix/upstreams/d0249-d0250-go-runtime-retirement-checklist.md',
    ' M docs/analytix/upstreams/go-runtime-retirement-checklist.md',
    ' M docs/analytix/upstreams/d0249-reasonix-integration-topology.md',
    ' M docs/analytix/upstreams/reasonix-integration-topology.md',
    ' M docs/analytix/upstreams/legacy-stage-evidence-archive.md'
  ].join('\\n'))
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

    try {
      process.env.PATH = `${dir}${delimiter}${originalPath || ''}`
      const { relevantEvidenceFinalGateBlockers, worktreeAudit, worktreeFinalGateBlockers } = await loadWorktreeAudit()
      const audit = worktreeAudit()

      expect(audit).toEqual(expect.objectContaining({
        status: 'dirty',
        dirtyFileCount: 5,
        goalScopeDirtyFileCount: 5,
        nonGoalScopeDirtyFileCount: 0,
        relevantEvidenceDirtyFileCount: 3,
        relevantEvidenceStagedDirtyFileCount: 0,
        relevantEvidenceUnstagedDirtyFileCount: 3,
        relevantEvidenceIntentionallyStaged: false
      }))
      expect(audit.relevantEvidenceDirtyFiles).toEqual([
        ' M docs/analytix/upstreams/go-runtime-retirement-checklist.md',
        ' M docs/analytix/upstreams/reasonix-integration-topology.md',
        ' M docs/analytix/upstreams/legacy-stage-evidence-archive.md'
      ])
      expect(worktreeFinalGateBlockers(audit)).toEqual([])
      expect(relevantEvidenceFinalGateBlockers(audit)).toEqual([
        'evidence:relevant-clean-scope-dirty'
      ])
    } finally {
      process.env.PATH = originalPath
    }
  })

  it('classifies the release workflow as runtime Go release-gate evidence', async () => {
    const dir = tempDir()
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const originalPath = process.env.PATH
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'status' && args[1] === '--porcelain=v1') {
  console.log(' M .github/workflows/release.yml')
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

    try {
      process.env.PATH = `${dir}${delimiter}${originalPath || ''}`
      const { relevantEvidenceFinalGateBlockers, worktreeAudit, worktreeFinalGateBlockers } = await loadWorktreeAudit()
      const audit = worktreeAudit()

      expect(audit).toEqual(expect.objectContaining({
        status: 'dirty',
        dirtyFileCount: 1,
        goalScopeDirtyFileCount: 1,
        nonGoalScopeDirtyFileCount: 0,
        relevantEvidenceDirtyFileCount: 1,
        relevantEvidenceUnstagedDirtyFileCount: 1
      }))
      expect(audit.relevantEvidenceDirtyFiles).toEqual([
        ' M .github/workflows/release.yml'
      ])
      expect(worktreeFinalGateBlockers(audit)).toEqual([])
      expect(relevantEvidenceFinalGateBlockers(audit)).toEqual([
        'evidence:relevant-clean-scope-dirty'
      ])
    } finally {
      process.env.PATH = originalPath
    }
  })

  it('classifies validation config and visual QA output without non-goal blockers', async () => {
    const dir = tempDir()
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const originalPath = process.env.PATH
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'status' && args[1] === '--porcelain=v1') {
  console.log([
    ' M package-lock.json',
    ' M vitest.config.ts',
    '?? .github/',
    '?? output/'
  ].join('\\n'))
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

    try {
      process.env.PATH = `${dir}${delimiter}${originalPath || ''}`
      const { relevantEvidenceFinalGateBlockers, worktreeAudit, worktreeFinalGateBlockers } = await loadWorktreeAudit()
      const audit = worktreeAudit()

      expect(audit).toEqual(expect.objectContaining({
        status: 'dirty',
        dirtyFileCount: 4,
        goalScopeDirtyFileCount: 3,
        nonGoalScopeDirtyFileCount: 1,
        unclassifiedNonGoalScopeDirtyFileCount: 0,
        classifiedNonGoalScopeDirtyFileCount: 1,
        generatedArtifactDirtyFileCount: 1,
        unclassifiedGeneratedArtifactDirtyFileCount: 0,
        classifiedGeneratedArtifactDirtyFileCount: 1,
        relevantEvidenceDirtyFileCount: 3,
        relevantEvidenceUnstagedDirtyFileCount: 3
      }))
      expect(audit.relevantEvidenceDirtyFiles).toEqual([
        ' M package-lock.json',
        ' M vitest.config.ts',
        '?? .github/'
      ])
      expect(audit.classifiedNonGoalScopeDirtyFiles).toEqual([
        '?? output/'
      ])
      expect(worktreeFinalGateBlockers(audit)).toEqual([])
      expect(relevantEvidenceFinalGateBlockers(audit)).toEqual([
        'evidence:relevant-clean-scope-dirty'
      ])
    } finally {
      process.env.PATH = originalPath
    }
  })

  it('classifies current live evidence reports as runtime Go release-gate evidence', async () => {
    const dir = tempDir()
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const originalPath = process.env.PATH
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'status' && args[1] === '--porcelain=v1') {
  console.log([
    ' M docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json',
    ' M docs/analytix/upstreams/runtime-go-live-evidence/operator-gate.json'
  ].join('\\n'))
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

    try {
      process.env.PATH = `${dir}${delimiter}${originalPath || ''}`
      const { relevantEvidenceFinalGateBlockers, worktreeAudit, worktreeFinalGateBlockers } = await loadWorktreeAudit()
      const audit = worktreeAudit()

      expect(audit).toEqual(expect.objectContaining({
        status: 'dirty',
        dirtyFileCount: 2,
        goalScopeDirtyFileCount: 2,
        nonGoalScopeDirtyFileCount: 0,
        relevantEvidenceDirtyFileCount: 2,
        relevantEvidenceUnstagedDirtyFileCount: 2
      }))
      expect(audit.relevantEvidenceDirtyFiles).toEqual([
        ' M docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json',
        ' M docs/analytix/upstreams/runtime-go-live-evidence/operator-gate.json'
      ])
      expect(worktreeFinalGateBlockers(audit)).toEqual([])
      expect(relevantEvidenceFinalGateBlockers(audit)).toEqual([
        'evidence:relevant-clean-scope-dirty'
      ])
    } finally {
      process.env.PATH = originalPath
    }
  })

  it('allows final evidence gate when every relevant evidence change is intentionally staged', async () => {
    const dir = tempDir()
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const originalPath = process.env.PATH
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'status' && args[1] === '--porcelain=v1') {
  console.log([
    'M  scripts/runtime-go-worktree-audit.mjs',
    'A  scripts/runtime-go-preflight.mjs',
    ' M backend/app/repositories/__pycache__/flow_repository.cpython-311.pyc'
  ].join('\\n'))
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

    try {
      process.env.PATH = `${dir}${delimiter}${originalPath || ''}`
      const { relevantEvidenceFinalGateBlockers, worktreeAudit, worktreeFinalGateBlockers } = await loadWorktreeAudit()
      const audit = worktreeAudit()

      expect(audit).toEqual(expect.objectContaining({
        status: 'dirty',
        dirtyFileCount: 3,
        relevantEvidenceDirtyFileCount: 2,
        relevantEvidenceStagedDirtyFileCount: 2,
        relevantEvidenceUnstagedDirtyFileCount: 0,
        relevantEvidenceIntentionallyStaged: true,
        nonGoalScopeDirtyFileCount: 1,
        unclassifiedNonGoalScopeDirtyFileCount: 0
      }))
      expect(audit.relevantEvidenceStagedDirtyFiles).toEqual([
        'M  scripts/runtime-go-worktree-audit.mjs',
        'A  scripts/runtime-go-preflight.mjs'
      ])
      expect(audit.relevantEvidenceUnstagedDirtyFiles).toEqual([])
      expect(worktreeFinalGateBlockers(audit)).toEqual([])
      expect(relevantEvidenceFinalGateBlockers(audit)).toEqual([])
    } finally {
      process.env.PATH = originalPath
    }
  })

  it('prints JSON with final gate blockers when executed as a CLI helper', () => {
    const dir = tempDir()
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const originalPath = process.env.PATH
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'status' && args[1] === '--porcelain=v1') {
  console.log([
    ' M backend/app/repositories/__pycache__/flow_repository.cpython-311.pyc',
    '?? tmp/cache.pyc'
  ].join('\\n'))
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

    try {
      process.env.PATH = `${dir}${delimiter}${originalPath || ''}`
      const result = spawnSync(process.execPath, ['scripts/runtime-go-worktree-audit.mjs', '--json'], {
        cwd: process.cwd(),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      expect(result.status).toBe(0)
      const audit = JSON.parse(result.stdout)

      expect(audit).toEqual(expect.objectContaining({
        status: 'dirty',
        dirtyFileCount: 2,
        unclassifiedNonGoalScopeDirtyFileCount: 1,
        classifiedNonGoalScopeDirtyFileCount: 1,
        worktreeFinalGateBlockers: [
          'worktree:non-goal-dirty-files',
          'worktree:generated-artifacts'
        ],
        relevantEvidenceFinalGateBlockers: []
      }))
      expect(result.stderr).toBe('')
    } finally {
      process.env.PATH = originalPath
    }
  })
})
