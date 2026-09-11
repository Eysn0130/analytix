import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

export function uniqueStrings(values) {
  return [...new Set(values.map((value) => String(value || '').trim()).filter(Boolean))]
}

export const runtimeGoRelevantEvidencePaths = [
  '.github',
  '.github/workflows/release.yml',
  '.gitignore',
  'package.json',
  'package-lock.json',
  'vitest.config.ts',
  'packages/runtime/package.json',
  'scripts/after-pack.cjs',
  'scripts/cache-first-review-gate.mjs',
  'scripts/scan-product-sovereignty.cjs',
  'src/main/runtime/analytix-adapter.ts',
  'scripts/runtime-go-validation-command.mjs',
  'scripts/runtime-go-validation-delegate.mjs',
  'scripts/runtime-go-live-validation.mjs',
  'scripts/runtime-go-worktree-audit.mjs',
  'scripts/runtime-go-preflight.mjs',
  'scripts/runtime-go-default-readiness-report.mjs',
  'scripts/runtime-go-cutover-report.mjs',
  'scripts/runtime-go-engine-absorption-report.mjs',
  'scripts/runtime-go-approval-user-input-evidence.mjs',
  'scripts/runtime-go-live-evidence-collector.mjs',
  'scripts/runtime-go-local-validation.mjs',
  'scripts/runtime-go-runtime-health-smoke.mjs',
  'scripts/runtime-go-performance-check.mjs',
  'scripts/runtime-go-speed-cache-gate.mjs',
  'scripts/runtime-go-product-regression.mjs',
  'scripts/runtime-go-packaged-qa.mjs',
  'scripts/runtime-go-packaged-milestone-a.mjs',
  'scripts/runtime-go-packaged-session-soak.mjs',
  'scripts/runtime-go-packaged-gui-smoke.mjs',
  'scripts/runtime-go-rollback-retirement-evidence.mjs',
  'scripts/runtime-go-rollback-retirement-report.mjs',
  'scripts/runtime-go-ts-retirement-scan.mjs',
  'src/main/cache-first-review-gate.test.ts',
  'src/main/runtime-go-live-validation.test.ts',
  'src/main/runtime-go-packaged-contract-report.test.ts',
  'src/main/runtime-go-packaged-milestone-a.test.ts',
  'src/main/runtime-go-engine-absorption-report.test.ts',
  'src/main/runtime-go-worktree-audit.test.ts',
  'src/main/runtime/analytix-adapter.test.ts',
  'src/main/runtime/runtime-go-live-evidence-collector.test.ts',
  'src/main/packaging-config.test.ts',
  'docs/analytix/upstreams/go-runtime-retirement-checklist.md',
  'docs/analytix/upstreams/reasonix-integration-topology.md',
  'docs/analytix/upstreams/legacy-stage-evidence-archive.md',
  'docs/analytix/upstreams/runtime-go-live-evidence',
  'docs/model-provider-presets.md',
  'packages/runtime-go',
  'packages/runtime/src'
]

const classifiedDirtyPathReasons = new Map([
  [
    'backend/app/repositories/__pycache__/flow_repository.cpython-311.pyc',
    'pre-existing non-goal tracked generated artifact; do not modify from runtime-go goal work'
  ]
])

function classifiedDirtyReason(entry) {
  const mapped = classifiedDirtyPathReasons.get(entry.path)
  if (mapped) return mapped
  if (entry.path === 'output/' || entry.path.startsWith('output/')) {
    return 'local visual QA output artifact; do not modify from runtime-go goal work'
  }
  if (entry.status === '??' && (entry.path === 'openspec/changes/' || entry.path.startsWith('openspec/changes/'))) {
    return 'pre-existing non-goal OpenSpec change workspace; do not modify from runtime-go goal work'
  }
  if (entry.status === '??' && (entry.path === 'openspec/specs/' || entry.path.startsWith('openspec/specs/'))) {
    return 'pre-existing non-goal OpenSpec spec workspace; do not modify from runtime-go goal work'
  }
  if (entry.status !== '??' && entry.status.includes('D') && /(^|\/)__pycache__\/.*\.pyc$/i.test(entry.path)) {
    return 'release hygiene: tracked Python bytecode removed and ignored'
  }
  return ''
}

function decodeGitPath(path) {
  const trimmed = String(path || '').trim()
  if (!trimmed.startsWith('"')) return trimmed
  const body = trimmed.endsWith('"') ? trimmed.slice(1, -1) : trimmed.slice(1)
  const bytes = []
  for (let index = 0; index < body.length;) {
    const char = body[index]
    if (char === '\\') {
      const octal = body.slice(index + 1, index + 4)
      if (/^[0-7]{3}$/.test(octal)) {
        bytes.push(parseInt(octal, 8))
        index += 4
        continue
      }
      const escaped = body[index + 1]
      const simpleEscapes = {
        '"': '"'.charCodeAt(0),
        '\\': '\\'.charCodeAt(0),
        n: '\n'.charCodeAt(0),
        r: '\r'.charCodeAt(0),
        t: '\t'.charCodeAt(0),
        b: '\b'.charCodeAt(0),
        f: '\f'.charCodeAt(0)
      }
      if (escaped && Object.hasOwn(simpleEscapes, escaped)) {
        bytes.push(simpleEscapes[escaped])
        index += 2
        continue
      }
    }
    bytes.push(char.charCodeAt(0))
    index += 1
  }
  return Buffer.from(bytes).toString('utf8')
}

export function worktreeAudit() {
  const result = spawnSync('git', ['status', '--porcelain=v1'], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe',
    maxBuffer: 16 * 1024 * 1024
  })
  if (result.status !== 0) {
    return {
      status: 'unknown',
      reason: String(result.stderr || result.error?.message || 'git status failed').trim(),
      finalAcceptanceRequiresCleanOrClassifiedTree: true
    }
  }
  const entries = String(result.stdout || '')
    .split(/\r?\n/)
    .filter(Boolean)
    .map((line) => {
      const status = line.slice(0, 2)
      const path = decodeGitPath(line.slice(3))
      return { status, path }
    })
  const generatedArtifactPattern = /(^|\/)(__pycache__\/|.*\.pyc$|dist\/|node_modules\/|output\/|\.DS_Store$|.*\.log$|.*\.tmp$)/i
  const runtimeScopePattern = /^(?:\.github(?:\/|$)|docs\/analytix|docs\/model-provider-presets\.md|package\.json|package-lock\.json|vitest\.config\.ts|packages\/runtime|packages\/runtime-go|scripts\/runtime-go-|scripts\/cache-first-review-gate\.mjs|scripts\/scan-product-sovereignty\.cjs|scripts\/after-pack\.cjs|src\/main|src\/preload|src\/renderer|src\/shared|重构升级方案\.md)/
  const legacyRetirementPattern = /^scripts\/d0(?:24|25)[a-z0-9_-]*.*\.mjs$/i
  const releaseWorkflowPattern = /^\.github\/workflows\/release\.yml$/
  const repoHygienePattern = /^\.gitignore$/
  const relevantEvidenceEntries = entries.filter((entry) => isRuntimeGoRelevantEvidencePath(entry.path))
  const relevantEvidenceStagedEntries = relevantEvidenceEntries.filter((entry) => isStagedStatus(entry.status))
  const relevantEvidenceUnstagedEntries = relevantEvidenceEntries.filter((entry) => isUnstagedStatus(entry.status))
  const generatedArtifactDirtyEntries = entries
    .filter((entry) => generatedArtifactPattern.test(entry.path))
  const classifiedDirtyEntries = entries
    .filter((entry) => classifiedDirtyReason(entry))
  const trackedGeneratedArtifactDirtyEntries = generatedArtifactDirtyEntries
    .filter((entry) => entry.status !== '??')
  const untrackedGeneratedArtifactDirtyEntries = generatedArtifactDirtyEntries
    .filter((entry) => entry.status === '??')
  const runtimeScopeEntries = entries.filter((entry) =>
    runtimeScopePattern.test(entry.path) || legacyRetirementPattern.test(entry.path) || releaseWorkflowPattern.test(entry.path))
  const repoHygieneEntries = entries.filter((entry) => repoHygienePattern.test(entry.path))
  const goalScopeEntries = entries.filter((entry) =>
    runtimeScopePattern.test(entry.path) || legacyRetirementPattern.test(entry.path) || releaseWorkflowPattern.test(entry.path) || repoHygienePattern.test(entry.path))
  const nonGoalScopeEntries = entries.filter((entry) =>
    !runtimeScopePattern.test(entry.path) && !legacyRetirementPattern.test(entry.path) && !releaseWorkflowPattern.test(entry.path) && !repoHygienePattern.test(entry.path))
  const classifiedNonGoalScopeEntries = nonGoalScopeEntries
    .filter((entry) => classifiedDirtyReason(entry))
  const unclassifiedNonGoalScopeEntries = nonGoalScopeEntries
    .filter((entry) => !classifiedDirtyReason(entry))
  const classifiedGeneratedArtifactDirtyEntries = generatedArtifactDirtyEntries
    .filter((entry) => classifiedDirtyReason(entry))
  const unclassifiedGeneratedArtifactDirtyEntries = generatedArtifactDirtyEntries
    .filter((entry) => !classifiedDirtyReason(entry))
  const classifiedTrackedGeneratedArtifactDirtyEntries = trackedGeneratedArtifactDirtyEntries
    .filter((entry) => classifiedDirtyReason(entry))
  const unclassifiedTrackedGeneratedArtifactDirtyEntries = trackedGeneratedArtifactDirtyEntries
    .filter((entry) => !classifiedDirtyReason(entry))
  const classifiedDirtyFileReasons = classifiedDirtyEntries.slice(0, 20).map((entry) => ({
    status: entry.status,
    path: entry.path,
    reason: classifiedDirtyReason(entry)
  }))
  const sample = (items) => items.slice(0, 20).map((entry) => `${entry.status} ${entry.path}`)
  return {
    status: entries.length === 0 ? 'clean' : 'dirty',
    dirtyFileCount: entries.length,
    trackedDirtyFileCount: entries.filter((entry) => entry.status !== '??').length,
    untrackedFileCount: entries.filter((entry) => entry.status === '??').length,
    deletedFileCount: entries.filter((entry) => entry.status.includes('D')).length,
    runtimeScopeDirtyFileCount: runtimeScopeEntries.length,
    repoHygieneDirtyFileCount: repoHygieneEntries.length,
    goalScopeDirtyFileCount: goalScopeEntries.length,
    nonRuntimeScopeDirtyFileCount: entries.length - runtimeScopeEntries.length,
    nonGoalScopeDirtyFileCount: nonGoalScopeEntries.length,
    unclassifiedNonGoalScopeDirtyFileCount: unclassifiedNonGoalScopeEntries.length,
    classifiedNonGoalScopeDirtyFileCount: classifiedNonGoalScopeEntries.length,
    runtimeScopeDirtyFiles: sample(runtimeScopeEntries),
    runtimeScopeDirtyFilesTruncated: runtimeScopeEntries.length > 20,
    repoHygieneDirtyFiles: sample(repoHygieneEntries),
    repoHygieneDirtyFilesTruncated: repoHygieneEntries.length > 20,
    relevantEvidenceDirtyFileCount: relevantEvidenceEntries.length,
    relevantEvidenceDirtyFiles: sample(relevantEvidenceEntries),
    relevantEvidenceDirtyFilesTruncated: relevantEvidenceEntries.length > 20,
    relevantEvidenceStagedDirtyFileCount: relevantEvidenceStagedEntries.length,
    relevantEvidenceStagedDirtyFiles: sample(relevantEvidenceStagedEntries),
    relevantEvidenceStagedDirtyFilesTruncated: relevantEvidenceStagedEntries.length > 20,
    relevantEvidenceUnstagedDirtyFileCount: relevantEvidenceUnstagedEntries.length,
    relevantEvidenceUnstagedDirtyFiles: sample(relevantEvidenceUnstagedEntries),
    relevantEvidenceUnstagedDirtyFilesTruncated: relevantEvidenceUnstagedEntries.length > 20,
    relevantEvidenceIntentionallyStaged: relevantEvidenceEntries.length > 0 &&
      relevantEvidenceUnstagedEntries.length === 0 &&
      relevantEvidenceStagedEntries.length === relevantEvidenceEntries.length,
    nonGoalScopeDirtyFiles: sample(nonGoalScopeEntries),
    nonGoalScopeDirtyFilesTruncated: nonGoalScopeEntries.length > 20,
    unclassifiedNonGoalScopeDirtyFiles: sample(unclassifiedNonGoalScopeEntries),
    unclassifiedNonGoalScopeDirtyFilesTruncated: unclassifiedNonGoalScopeEntries.length > 20,
    classifiedNonGoalScopeDirtyFiles: sample(classifiedNonGoalScopeEntries),
    classifiedNonGoalScopeDirtyFilesTruncated: classifiedNonGoalScopeEntries.length > 20,
    classifiedDirtyFileReasons,
    classifiedDirtyFileReasonsTruncated: classifiedDirtyEntries.length > 20,
    generatedArtifactDirtyFileCount: generatedArtifactDirtyEntries.length,
    generatedArtifactDirtyFiles: sample(generatedArtifactDirtyEntries),
    generatedArtifactDirtyFilesTruncated: generatedArtifactDirtyEntries.length > 20,
    unclassifiedGeneratedArtifactDirtyFileCount: unclassifiedGeneratedArtifactDirtyEntries.length,
    unclassifiedGeneratedArtifactDirtyFiles: sample(unclassifiedGeneratedArtifactDirtyEntries),
    unclassifiedGeneratedArtifactDirtyFilesTruncated: unclassifiedGeneratedArtifactDirtyEntries.length > 20,
    classifiedGeneratedArtifactDirtyFileCount: classifiedGeneratedArtifactDirtyEntries.length,
    classifiedGeneratedArtifactDirtyFiles: sample(classifiedGeneratedArtifactDirtyEntries),
    classifiedGeneratedArtifactDirtyFilesTruncated: classifiedGeneratedArtifactDirtyEntries.length > 20,
    trackedGeneratedArtifactDirtyFileCount: trackedGeneratedArtifactDirtyEntries.length,
    trackedGeneratedArtifactDirtyFiles: sample(trackedGeneratedArtifactDirtyEntries),
    trackedGeneratedArtifactDirtyFilesTruncated: trackedGeneratedArtifactDirtyEntries.length > 20,
    unclassifiedTrackedGeneratedArtifactDirtyFileCount: unclassifiedTrackedGeneratedArtifactDirtyEntries.length,
    unclassifiedTrackedGeneratedArtifactDirtyFiles: sample(unclassifiedTrackedGeneratedArtifactDirtyEntries),
    unclassifiedTrackedGeneratedArtifactDirtyFilesTruncated: unclassifiedTrackedGeneratedArtifactDirtyEntries.length > 20,
    classifiedTrackedGeneratedArtifactDirtyFileCount: classifiedTrackedGeneratedArtifactDirtyEntries.length,
    classifiedTrackedGeneratedArtifactDirtyFiles: sample(classifiedTrackedGeneratedArtifactDirtyEntries),
    classifiedTrackedGeneratedArtifactDirtyFilesTruncated: classifiedTrackedGeneratedArtifactDirtyEntries.length > 20,
    untrackedGeneratedArtifactDirtyFileCount: untrackedGeneratedArtifactDirtyEntries.length,
    untrackedGeneratedArtifactDirtyFiles: sample(untrackedGeneratedArtifactDirtyEntries),
    untrackedGeneratedArtifactDirtyFilesTruncated: untrackedGeneratedArtifactDirtyEntries.length > 20,
    finalAcceptanceRequiresCleanOrClassifiedTree: true
  }
}

function isRuntimeGoRelevantEvidencePath(path) {
  const normalized = String(path || '').trim()
  return runtimeGoRelevantEvidencePaths.some((scope) =>
    normalized === scope || normalized.startsWith(`${scope}/`))
}

function isStagedStatus(status) {
  const x = String(status || '  ')[0]
  return Boolean(x && x !== ' ' && x !== '?')
}

function isUnstagedStatus(status) {
  const text = String(status || '  ')
  if (text === '??') return true
  const y = text[1]
  return Boolean(y && y !== ' ')
}

export function worktreeFinalGateBlockers(worktree) {
  const blockers = []
  const nonGoalDirtyFileCount = Object.hasOwn(worktree, 'unclassifiedNonGoalScopeDirtyFileCount')
    ? Number(worktree.unclassifiedNonGoalScopeDirtyFileCount || 0)
    : Number(worktree.nonGoalScopeDirtyFileCount || 0)
  const generatedArtifactDirtyFileCount = Object.hasOwn(worktree, 'unclassifiedGeneratedArtifactDirtyFileCount')
    ? Number(worktree.unclassifiedGeneratedArtifactDirtyFileCount || 0)
    : Number(worktree.generatedArtifactDirtyFileCount || 0)
  const trackedGeneratedArtifactDirtyFileCount = Object.hasOwn(worktree, 'unclassifiedTrackedGeneratedArtifactDirtyFileCount')
    ? Number(worktree.unclassifiedTrackedGeneratedArtifactDirtyFileCount || 0)
    : Number(worktree.trackedGeneratedArtifactDirtyFileCount || 0)
  if (nonGoalDirtyFileCount > 0) {
    blockers.push('worktree:non-goal-dirty-files')
  }
  if (generatedArtifactDirtyFileCount > 0) {
    blockers.push('worktree:generated-artifacts')
  }
  if (trackedGeneratedArtifactDirtyFileCount > 0) {
    blockers.push('worktree:tracked-generated-artifacts')
  }
  return blockers
}

export function relevantEvidenceFinalGateBlockers(worktree) {
  const dirtyCount = Number(worktree.relevantEvidenceDirtyFileCount || 0)
  const unstagedCount = Object.hasOwn(worktree, 'relevantEvidenceUnstagedDirtyFileCount')
    ? Number(worktree.relevantEvidenceUnstagedDirtyFileCount || 0)
    : dirtyCount
  if (unstagedCount > 0) {
    return ['evidence:relevant-clean-scope-dirty']
  }
  return []
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  const worktree = worktreeAudit()
  process.stdout.write(`${JSON.stringify({
    ...worktree,
    worktreeFinalGateBlockers: worktreeFinalGateBlockers(worktree),
    relevantEvidenceFinalGateBlockers: relevantEvidenceFinalGateBlockers(worktree)
  }, null, 2)}\n`)
}
