#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import {
  relevantEvidenceFinalGateBlockers,
  uniqueStrings,
  worktreeAudit,
  worktreeFinalGateBlockers
} from './runtime-go-worktree-audit.mjs'

const rawArgs = process.argv.slice(2)
const json = rawArgs.includes('--json') || rawArgs.includes('--dry-run')
const skipCommands = rawArgs.includes('--skip-commands') || rawArgs.includes('--dry-run')
const strictGate = rawArgs.includes('--gate')
const rcControlPlane = rawArgs.includes('--rc-control-plane')
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const defaultLiveEvidencePath = 'docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json'

const standaloneCommands = [
  {
    id: 'product-regression',
    command: npmCommand,
    args: ['run', 'runtime:go:product-regression']
  },
  {
    id: 'speed-cache-gate',
    command: npmCommand,
    args: ['run', 'runtime:go:speed-cache-gate']
  },
  {
    id: 'typecheck',
    command: npmCommand,
    args: ['run', 'typecheck']
  },
  {
    id: 'build-runtime',
    command: npmCommand,
    args: ['run', 'build:runtime']
  },
  {
    id: 'diff-check',
    command: 'git',
    args: ['diff', '--check']
  }
]
const commands = rcControlPlane
  ? standaloneCommands.filter((item) => item.id === 'diff-check')
  : standaloneCommands

function commandText(item) {
  return [item.command, ...item.args].join(' ')
}

function localCheckEnv() {
  const env = { ...process.env }
  for (const key of [
    'ANALYTIX_RUNTIME_READY',
    'ANALYTIX_RUNTIME_READY_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT',
    'ANALYTIX_RUNTIME_GO_OPERATOR_GATE_EVIDENCE',
    'ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS',
    'ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS',
    'ANALYTIX_RUNTIME_DURABLE_RESTART_EVIDENCE',
    'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS',
    'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS',
    'ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS',
    'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS',
    'ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS',
    'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS',
    'ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_PACKAGED_GUI_SMOKE_STATUS',
    'ANALYTIX_RUNTIME_GO_PACKAGED_GUI_SMOKE_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_PACKAGED_SESSION_SOAK_STATUS',
    'ANALYTIX_RUNTIME_GO_PACKAGED_SESSION_SOAK_EVIDENCE'
  ]) {
    delete env[key]
  }
  return env
}

function optionValue(name) {
  const prefix = `${name}=`
  for (let index = 0; index < rawArgs.length; index += 1) {
    const item = rawArgs[index]
    if (item.startsWith(prefix)) return item.slice(prefix.length)
    if (item === name && rawArgs[index + 1]) return rawArgs[index + 1]
  }
  return ''
}

function recordValue(value) {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {}
}

function stringList(values) {
  return Array.isArray(values)
    ? uniqueStrings(values.map((value) => String(value || '').trim()))
    : []
}

function operatorCurrentEnvGate() {
  return {
    runtimeReady: String(process.env.ANALYTIX_RUNTIME_READY || '').trim() === '1',
    operatorApprovesDefault: String(process.env.ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT || '').trim() === '1'
  }
}

function canonicalJSONString(value) {
  if (value === null || typeof value !== 'object') return JSON.stringify(value)
  if (Array.isArray(value)) return `[${value.map((item) => canonicalJSONString(item)).join(',')}]`
  return `{${Object.keys(value)
    .sort((a, b) => a.localeCompare(b))
    .map((key) => `${JSON.stringify(key)}:${canonicalJSONString(value[key])}`)
    .join(',')}}`
}

function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}

function currentGitCommit() {
  const result = spawnSync('git', ['rev-parse', 'HEAD'], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  return result.status === 0 ? String(result.stdout || '').trim() : ''
}

function normalizeRepoPath(pathValue) {
  return String(pathValue || '')
    .replace(/\\/g, '/')
    .replace(/^\.\//, '')
}

function relativeEvidencePath(pathValue) {
  const normalized = normalizeRepoPath(pathValue)
  const cwd = normalizeRepoPath(process.cwd())
  return normalized.startsWith(`${cwd}/`) ? normalized.slice(cwd.length + 1) : normalized
}

function isGitObjectId(value) {
  return /^[0-9a-f]{7,40}$/i.test(String(value || '').trim())
}

function liveEvidenceOutputPathSet(report) {
  const evidencePaths = recordValue(report.evidencePaths)
  const paths = new Set([
    defaultLiveEvidencePath,
    'docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-summary.md'
  ])
  for (const pathValue of Object.values(evidencePaths)) {
    if (typeof pathValue === 'string' && pathValue.trim()) {
      paths.add(relativeEvidencePath(pathValue))
    }
  }
  return paths
}

function sourceCommitCoveredByEvidenceOnlyCommit(reportCommit, gitCommit, report) {
  if (!isGitObjectId(reportCommit) || !isGitObjectId(gitCommit)) return false
  const ancestor = spawnSync('git', ['merge-base', '--is-ancestor', reportCommit, gitCommit], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  if (ancestor.status !== 0) return false

  const diff = spawnSync('git', ['diff', '--name-only', `${reportCommit}..${gitCommit}`], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  if (diff.status !== 0) return false

  const changedPaths = String(diff.stdout || '')
    .split(/\r?\n/)
    .map((item) => normalizeRepoPath(item.trim()))
    .filter(Boolean)
  if (changedPaths.length === 0) return false

  const evidencePaths = liveEvidenceOutputPathSet(report)
  return changedPaths.every((pathValue) => evidencePaths.has(pathValue))
}

function liveEvidenceFreshness(report) {
  const blockers = []
  const evidencePaths = recordValue(report.evidencePaths)
  const evidenceDigests = recordValue(report.evidenceDigests)
  const digestEntries = Object.entries(evidenceDigests)
    .filter(([, expected]) => typeof expected === 'string' && expected.length > 0)
  const digestMismatches = []
  const digestMissing = []
  for (const [id, expected] of digestEntries) {
    const pathValue = typeof evidencePaths[id] === 'string' ? evidencePaths[id] : ''
    if (!pathValue || !existsSync(pathValue)) {
      digestMissing.push(id)
      continue
    }
    try {
      const actual = sha256(canonicalJSONString(JSON.parse(readFileSync(pathValue, 'utf8'))))
      if (actual !== expected) digestMismatches.push(id)
    } catch {
      digestMismatches.push(id)
    }
  }
  if (digestEntries.length > 0 && (digestMissing.length > 0 || digestMismatches.length > 0)) {
    blockers.push('live-evidence-digest-mismatch')
  }

  const reportCommit = typeof report.sourceCommit === 'string' ? report.sourceCommit.trim() : ''
  const gitCommit = reportCommit ? currentGitCommit() : ''
  const sourceCommitStatus = reportCommit && gitCommit
    ? reportCommit === gitCommit
        ? 'passed'
        : sourceCommitCoveredByEvidenceOnlyCommit(reportCommit, gitCommit, report)
          ? 'evidence-only-commit'
          : 'stale'
    : reportCommit ? 'unknown' : 'skipped'
  if (sourceCommitStatus === 'stale') blockers.push('live-evidence-source-commit-stale')

  return {
    status: blockers.length > 0 ? 'failed' : 'passed',
    blockers,
    digestStatus: digestEntries.length === 0
      ? 'skipped'
      : digestMissing.length > 0 || digestMismatches.length > 0
        ? 'failed'
        : 'passed',
    digestCheckedIds: digestEntries.map(([id]) => id),
    digestMissing,
    digestMismatches,
    sourceCommitStatus,
    sourceCommit: reportCommit,
    currentCommit: sourceCommitStatus === 'skipped' ? '' : gitCommit
  }
}

function liveEvidenceCompleteness(report) {
  if (report.status !== 'passed' || report.passed !== true) {
    return {
      status: 'skipped',
      blockers: [],
      missingComponents: [],
      digestCount: 0,
      sourceCommit: typeof report.sourceCommit === 'string' ? report.sourceCommit.trim() : ''
    }
  }
  const blockers = []
  const componentStatus = recordValue(report.componentStatus)
  const evidenceDigests = recordValue(report.evidenceDigests)
  const sourceCommit = typeof report.sourceCommit === 'string' ? report.sourceCommit.trim() : ''
  const requiredComponents = ['provider', 'mcp', 'packaged', 'packagedGui', 'packagedSoak', 'operator']
  const missingComponents = requiredComponents.filter((id) => String(componentStatus[id] || '').trim() !== 'passed')
  const digestEntries = Object.entries(evidenceDigests)
    .filter(([, expected]) => typeof expected === 'string' && expected.length > 0)

  if (sourceCommit === '') blockers.push('sourceCommit')
  if (digestEntries.length === 0) blockers.push('evidenceDigests')
  if (report.liveOrActualEvidenceUsed !== true) blockers.push('liveOrActualEvidenceUsed')
  if (report.deterministicEvidenceOnly === true) blockers.push('deterministicEvidenceOnly')
  if (missingComponents.length > 0) blockers.push(...missingComponents.map((id) => `componentStatus.${id}`))

  return {
    status: blockers.length > 0 ? 'failed' : 'passed',
    blockers,
    missingComponents,
    digestCount: digestEntries.length,
    sourceCommit
  }
}

function liveEvidencePath() {
  return resolve(process.cwd(),
    optionValue('--live-evidence-json') ||
      process.env.ANALYTIX_RUNTIME_GO_LIVE_EVIDENCE_JSON ||
      process.env.ANALYTIX_RUNTIME_LIVE_EVIDENCE_JSON ||
      defaultLiveEvidencePath)
}

function summarizeNextEvidenceActions(report, missingExternalInputs) {
  return summarizeNextEvidenceActionsWithOperatorSummary(report, missingExternalInputs, operatorDependencySummaryFromReport(report))
}

function summarizeNextEvidenceActionsWithOperatorSummary(report, missingExternalInputs, operatorDependencySummary) {
  const currentEnvGate = operatorCurrentEnvGate()
  const source = recordValue(report.nextEvidenceActions)
  const providerMatrix = recordValue(source.providerMatrix)
  const operatorGate = recordValue(source.operatorGate)
  const commands = stringList(source.commands)
  const candidateProviderIds = uniqueStrings(missingExternalInputs
    .flatMap((input) => stringList(input.candidateProviderIds)))
  const actions = []

  for (const input of missingExternalInputs) {
    const id = String(input.id || '').trim()
    if (!id) continue
    const base = {
      id,
      status: String(input.status || 'required').trim(),
      reason: String(input.reason || '').trim()
    }
    if (id === 'provider:at-least-one-non-deepseek-provider') {
      actions.push({
        ...base,
        evidencePath: typeof providerMatrix.evidencePath === 'string' ? providerMatrix.evidencePath : '',
        candidateProviderIds,
        candidates: Array.isArray(input.candidates)
          ? input.candidates.map((candidate) => {
            const item = recordValue(candidate)
            return {
              id: String(item.id || '').trim(),
              status: String(item.status || '').trim(),
              missingEnv: stringList(item.missingEnv),
              missingSettingsProfileFields: stringList(item.missingSettingsProfileFields)
            }
          })
          : [],
        acceptedSettingsProfilePath: String(recordValue(providerMatrix.completionOptions).settingsProfilePath || '').trim(),
        settingsProfileCoverage: providerGateSettingsCoverage(providerMatrix.settingsProfileCoverage),
        credentialedDeepSeekPassed: providerMatrix.credentialedDeepSeekPassed === true,
        credentialedNonDeepSeekProviderIds: stringList(providerMatrix.credentialedNonDeepSeekProviderIds),
        command: commands.find((command) => command.includes('runtime:go:live-validation') &&
          command.includes('--live-non-deepseek-provider-from-settings')) ||
          'npm run runtime:go:live-validation -- --json --live-deepseek-cache-from-settings --live-non-deepseek-provider-from-settings --no-gate'
      })
      continue
    }
    if (id === 'operator:gate') {
      const dependencySummary = Object.keys(recordValue(operatorGate.dependencySummary)).length > 0
        ? recordValue(operatorGate.dependencySummary)
        : operatorDependencySummary
      const evidenceEnvGate = recordValue(dependencySummary.envGate)
      const evidenceRefreshRequired = evidenceEnvGate.runtimeReady !== true ||
        evidenceEnvGate.operatorApprovesDefault !== true
      const evidenceRefreshCommand = 'ANALYTIX_RUNTIME_READY=1 ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT=1 npm run runtime:go:live-evidence -- --json'
      const defaultReadinessCommand = 'ANALYTIX_RUNTIME_READY=1 ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT=1 npm run runtime:go:default-readiness-report -- --gate --json'
      actions.push({
        ...base,
        evidencePath: typeof operatorGate.evidencePath === 'string' ? operatorGate.evidencePath : '',
        dependencySummary,
        evidenceEnvGate,
        currentEnvGate,
        evidenceRefreshRequired,
        evidenceRefreshCommand,
        defaultReadinessCommand,
        requiredEnv: recordValue(operatorGate.requiredEnv),
        evidenceReviewedIds: stringList(operatorGate.evidenceReviewedIds),
        command: evidenceRefreshRequired
          ? evidenceRefreshCommand
          : commands.find((command) => command.includes('runtime:go:default-readiness-report') && command.includes('--gate')) ||
            defaultReadinessCommand
      })
      continue
    }
    actions.push(base)
  }

  return {
    schemaVersion: 1,
    source: 'live-evidence-report',
    credentialSecretsRecorded: false,
    valuesRecorded: false,
    status: actions.length > 0 ? 'blocked' : 'passed',
    actions,
    commands
  }
}

function providerGateSettingsCoverage(value) {
  const coverage = recordValue(value)
  return {
    status: String(coverage.status || '').trim(),
    source: String(coverage.source || '').trim(),
    providerCount: Number.isFinite(coverage.providerCount) ? coverage.providerCount : 0,
    matchedProviderIds: stringList(coverage.matchedProviderIds),
    usableProviderIds: stringList(coverage.usableProviderIds),
    usableNonDeepSeekProviderIds: stringList(coverage.usableNonDeepSeekProviderIds),
    deepseekSettingsProfileUsable: coverage.deepseekSettingsProfileUsable === true,
    nonDeepSeekSettingsProfileUsable: coverage.nonDeepSeekSettingsProfileUsable === true,
    satisfiesFinalProviderCoverageFromSettings: coverage.satisfiesFinalProviderCoverageFromSettings === true,
    credentialValuesRecorded: coverage.credentialValuesRecorded === true
  }
}

function providerGateDetail(finalGateBlockers, value) {
  const present = finalGateBlockers.includes('provider:at-least-one-non-deepseek-provider')
  const source = recordValue(value)
  const candidates = Array.isArray(source.candidates)
    ? source.candidates.map((candidate) => {
      const item = recordValue(candidate)
      return {
        id: String(item.id || '').trim(),
        status: String(item.status || '').trim(),
        missingEnv: stringList(item.missingEnv),
        missingSettingsProfileFields: stringList(item.missingSettingsProfileFields)
      }
    }).filter((item) => item.id)
    : []
  const credentialedNonDeepSeekProviderIds = stringList(source.credentialedNonDeepSeekProviderIds)
  return {
    present,
    evidencePath: typeof source.evidencePath === 'string' ? source.evidencePath : '',
    candidateProviderIds: stringList(source.candidateProviderIds),
    candidates,
    acceptedSettingsProfilePath: String(source.acceptedSettingsProfilePath || '').trim(),
    settingsProfileCoverage: providerGateSettingsCoverage(source.settingsProfileCoverage),
    credentialedDeepSeekPassed: source.credentialedDeepSeekPassed === true,
    credentialedNonDeepSeekProviderIds,
    requiresNonDeepSeekProvider: present && credentialedNonDeepSeekProviderIds.length === 0
  }
}

function operatorDependencySummaryFromReport(report) {
  const source = recordValue(report.nextEvidenceActions)
  const operatorGate = recordValue(source.operatorGate)
  const fromAction = recordValue(operatorGate.dependencySummary)
  if (Object.keys(fromAction).length > 0) return fromAction

  const evidencePaths = recordValue(report.evidencePaths)
  const operatorEvidencePath = String(
    evidencePaths.operator ||
    operatorGate.evidencePath ||
    ''
  ).trim()
  if (!operatorEvidencePath) return {}
  try {
    const path = operatorEvidencePath.startsWith('/')
      ? operatorEvidencePath
      : resolve(process.cwd(), operatorEvidencePath)
    const operatorEvidence = JSON.parse(readFileSync(path, 'utf8'))
    return recordValue(operatorEvidence.dependencySummary)
  } catch {
    return {}
  }
}

function readLiveEvidenceSummary() {
  const path = liveEvidencePath()
  if (!existsSync(path)) {
    return {
      path,
      status: 'missing',
      passed: false,
      missingExternalInputIds: ['live-evidence-report'],
      componentStatus: {},
      finalGateBlocked: true,
      finalGateBlockers: ['live-evidence-report-missing']
    }
  }
  try {
    const report = JSON.parse(readFileSync(path, 'utf8'))
    const operatorDependencySummary = operatorDependencySummaryFromReport(report)
    const missingExternalInputs = Array.isArray(report.missingExternalInputs)
      ? report.missingExternalInputs.map(recordValue)
      : []
    const missingExternalInputIds = uniqueStrings(missingExternalInputs
      .map((item) => String(item.id || '').trim())
      .filter(Boolean))
    const completeness = liveEvidenceCompleteness(report)
    const freshness = liveEvidenceFreshness(report)
    const completenessFailed = completeness.status === 'failed'
    const finalGateBlocked = report.status !== 'passed' ||
      report.passed !== true ||
      completenessFailed ||
      freshness.status !== 'passed'
    return {
      path,
      status: typeof report.status === 'string' ? report.status : 'invalid',
      passed: report.passed === true,
      liveOrActualEvidenceUsed: report.liveOrActualEvidenceUsed === true,
      deterministicEvidenceOnly: report.deterministicEvidenceOnly === true,
      llmAnswerQualityEvidenceUsed: report.llmAnswerQualityEvidenceUsed === true,
      componentStatus: recordValue(report.componentStatus),
      missingExternalInputIds,
      operatorDependencySummary,
      nextEvidenceActions: summarizeNextEvidenceActionsWithOperatorSummary(report, missingExternalInputs, operatorDependencySummary),
      completeness,
      freshness,
      finalGateBlocked,
      finalGateBlockers: finalGateBlocked
        ? uniqueStrings([
          ...missingExternalInputIds,
          ...(completenessFailed ? ['live-evidence-report-incomplete'] : []),
          ...freshness.blockers,
          ...(missingExternalInputIds.length === 0 && !completenessFailed && freshness.blockers.length === 0
            ? ['live-evidence-not-passed']
            : [])
        ])
        : []
    }
  } catch (error) {
    return {
      path,
      status: 'invalid',
      passed: false,
      missingExternalInputIds: ['live-evidence-report-invalid'],
      componentStatus: {},
      finalGateBlocked: true,
      finalGateBlockers: ['live-evidence-report-invalid'],
      reason: error instanceof Error ? error.message : String(error)
    }
  }
}

function finalGateNextActions(finalGateBlockers, liveEvidence, worktree, skipCommands) {
  const actions = []
  if (skipCommands || finalGateBlockers.includes('dry-run-cannot-authorize-cutover')) {
    actions.push({
      id: 'dry-run-cannot-authorize-cutover',
      status: 'required',
      reason: 'dry-run reports cannot authorize the final Go runtime gate',
      command: 'npm run runtime:go:preflight -- --json --gate'
    })
  }
  const liveActions = recordValue(liveEvidence.nextEvidenceActions).actions
  for (const item of Array.isArray(liveActions) ? liveActions : []) {
    actions.push(item)
  }
  if (String(recordValue(liveEvidence.freshness).status || '') === 'failed') {
    actions.push({
      id: 'live-evidence:freshness',
      status: 'required',
      reason: 'live evidence digests or source commit are stale',
      command: 'npm run runtime:go:live-evidence -- --json'
    })
  }
  if (finalGateBlockers.includes('evidence:relevant-clean-scope-dirty')) {
    actions.push({
      id: 'evidence:relevant-clean-scope-dirty',
      status: 'required',
      reason: 'operator/live evidence cannot authorize final acceptance while hard gate scripts or evidence tests are dirty',
      remediation: {
        requiresOperatorReview: true,
        destructiveCleanupSuggested: false,
        guidance: 'Commit, intentionally stage, or re-run live evidence after the listed hard gate scripts/tests are clean.'
      },
      relevantEvidenceDirtyFileCount: Number(worktree.relevantEvidenceDirtyFileCount || 0),
      relevantEvidenceDirtyFiles: Array.isArray(worktree.relevantEvidenceDirtyFiles)
        ? worktree.relevantEvidenceDirtyFiles
        : [],
      relevantEvidenceDirtyFilesTruncated: Boolean(worktree.relevantEvidenceDirtyFilesTruncated),
      relevantEvidenceUnstagedDirtyFileCount: Number(worktree.relevantEvidenceUnstagedDirtyFileCount || 0),
      relevantEvidenceUnstagedDirtyFiles: Array.isArray(worktree.relevantEvidenceUnstagedDirtyFiles)
        ? worktree.relevantEvidenceUnstagedDirtyFiles
        : [],
      relevantEvidenceUnstagedDirtyFilesTruncated: Boolean(worktree.relevantEvidenceUnstagedDirtyFilesTruncated),
      relevantEvidenceStagedDirtyFileCount: Number(worktree.relevantEvidenceStagedDirtyFileCount || 0),
      relevantEvidenceIntentionallyStaged: Boolean(worktree.relevantEvidenceIntentionallyStaged),
      command: 'npm run runtime:go:live-evidence -- --json'
    })
  }
  if (recordValue(worktree).status === 'dirty') {
    const blockerIds = finalGateBlockers.filter((id) => id.startsWith('worktree:'))
    actions.push({
      id: 'worktree:clean-or-classify',
      status: blockerIds.length > 0 ? 'required' : 'advisory',
      reason: 'final acceptance requires a clean or fully classified worktree',
      blockerIds,
      remediation: {
        requiresOperatorReview: blockerIds.length > 0,
        destructiveCleanupSuggested: false,
        guidance: blockerIds.length > 0
          ? 'Classify, intentionally keep, or remove the listed non-goal/generated dirty files before final acceptance.'
          : 'Review the dirty worktree before final acceptance.'
      },
      generatedArtifactDirtyFileCount: Number(worktree.generatedArtifactDirtyFileCount || 0),
      generatedArtifactDirtyFiles: Array.isArray(worktree.generatedArtifactDirtyFiles)
        ? worktree.generatedArtifactDirtyFiles
        : [],
      generatedArtifactDirtyFilesTruncated: Boolean(worktree.generatedArtifactDirtyFilesTruncated),
      unclassifiedGeneratedArtifactDirtyFileCount: Number(worktree.unclassifiedGeneratedArtifactDirtyFileCount || 0),
      unclassifiedGeneratedArtifactDirtyFiles: Array.isArray(worktree.unclassifiedGeneratedArtifactDirtyFiles)
        ? worktree.unclassifiedGeneratedArtifactDirtyFiles
        : [],
      unclassifiedGeneratedArtifactDirtyFilesTruncated: Boolean(worktree.unclassifiedGeneratedArtifactDirtyFilesTruncated),
      classifiedGeneratedArtifactDirtyFileCount: Number(worktree.classifiedGeneratedArtifactDirtyFileCount || 0),
      classifiedGeneratedArtifactDirtyFiles: Array.isArray(worktree.classifiedGeneratedArtifactDirtyFiles)
        ? worktree.classifiedGeneratedArtifactDirtyFiles
        : [],
      classifiedGeneratedArtifactDirtyFilesTruncated: Boolean(worktree.classifiedGeneratedArtifactDirtyFilesTruncated),
      trackedGeneratedArtifactDirtyFileCount: Number(worktree.trackedGeneratedArtifactDirtyFileCount || 0),
      trackedGeneratedArtifactDirtyFiles: Array.isArray(worktree.trackedGeneratedArtifactDirtyFiles)
        ? worktree.trackedGeneratedArtifactDirtyFiles
        : [],
      trackedGeneratedArtifactDirtyFilesTruncated: Boolean(worktree.trackedGeneratedArtifactDirtyFilesTruncated),
      unclassifiedTrackedGeneratedArtifactDirtyFileCount: Number(worktree.unclassifiedTrackedGeneratedArtifactDirtyFileCount || 0),
      unclassifiedTrackedGeneratedArtifactDirtyFiles: Array.isArray(worktree.unclassifiedTrackedGeneratedArtifactDirtyFiles)
        ? worktree.unclassifiedTrackedGeneratedArtifactDirtyFiles
        : [],
      unclassifiedTrackedGeneratedArtifactDirtyFilesTruncated: Boolean(worktree.unclassifiedTrackedGeneratedArtifactDirtyFilesTruncated),
      classifiedTrackedGeneratedArtifactDirtyFileCount: Number(worktree.classifiedTrackedGeneratedArtifactDirtyFileCount || 0),
      classifiedTrackedGeneratedArtifactDirtyFiles: Array.isArray(worktree.classifiedTrackedGeneratedArtifactDirtyFiles)
        ? worktree.classifiedTrackedGeneratedArtifactDirtyFiles
        : [],
      classifiedTrackedGeneratedArtifactDirtyFilesTruncated: Boolean(worktree.classifiedTrackedGeneratedArtifactDirtyFilesTruncated),
      untrackedGeneratedArtifactDirtyFileCount: Number(worktree.untrackedGeneratedArtifactDirtyFileCount || 0),
      untrackedGeneratedArtifactDirtyFiles: Array.isArray(worktree.untrackedGeneratedArtifactDirtyFiles)
        ? worktree.untrackedGeneratedArtifactDirtyFiles
        : [],
      untrackedGeneratedArtifactDirtyFilesTruncated: Boolean(worktree.untrackedGeneratedArtifactDirtyFilesTruncated),
      nonGoalScopeDirtyFileCount: Number(worktree.nonGoalScopeDirtyFileCount || 0),
      nonGoalScopeDirtyFiles: Array.isArray(worktree.nonGoalScopeDirtyFiles)
        ? worktree.nonGoalScopeDirtyFiles
        : [],
      nonGoalScopeDirtyFilesTruncated: Boolean(worktree.nonGoalScopeDirtyFilesTruncated),
      unclassifiedNonGoalScopeDirtyFileCount: Number(worktree.unclassifiedNonGoalScopeDirtyFileCount || 0),
      unclassifiedNonGoalScopeDirtyFiles: Array.isArray(worktree.unclassifiedNonGoalScopeDirtyFiles)
        ? worktree.unclassifiedNonGoalScopeDirtyFiles
        : [],
      unclassifiedNonGoalScopeDirtyFilesTruncated: Boolean(worktree.unclassifiedNonGoalScopeDirtyFilesTruncated),
      classifiedNonGoalScopeDirtyFileCount: Number(worktree.classifiedNonGoalScopeDirtyFileCount || 0),
      classifiedNonGoalScopeDirtyFiles: Array.isArray(worktree.classifiedNonGoalScopeDirtyFiles)
        ? worktree.classifiedNonGoalScopeDirtyFiles
        : [],
      classifiedNonGoalScopeDirtyFilesTruncated: Boolean(worktree.classifiedNonGoalScopeDirtyFilesTruncated),
      classifiedDirtyFileReasons: Array.isArray(worktree.classifiedDirtyFileReasons)
        ? worktree.classifiedDirtyFileReasons
        : [],
      classifiedDirtyFileReasonsTruncated: Boolean(worktree.classifiedDirtyFileReasonsTruncated)
    })
  }
  return {
    schemaVersion: 1,
    credentialSecretsRecorded: false,
    valuesRecorded: false,
    status: finalGateBlockers.length > 0 ? 'blocked' : 'passed',
    actions
  }
}

function operatorGateDetail(finalGateBlockers, operatorDependencySummary, operatorCurrentEnvGate) {
  const present = finalGateBlockers.includes('operator:gate')
  const evidenceEnvGate = recordValue(recordValue(operatorDependencySummary).envGate)
  const currentEnvGate = recordValue(operatorCurrentEnvGate)
  const dependencyBlockers = stringList(recordValue(operatorDependencySummary).blockers)
  const evidenceEnvSatisfied = evidenceEnvGate.runtimeReady === true &&
    evidenceEnvGate.operatorApprovesDefault === true
  const currentEnvSatisfied = currentEnvGate.runtimeReady === true &&
    currentEnvGate.operatorApprovesDefault === true
  return {
    present,
    evidenceEnvGate,
    currentEnvGate,
    dependencyBlockers,
    requiresOperatorApproval: present && !evidenceEnvSatisfied && !currentEnvSatisfied,
    requiresOperatorEvidenceRefresh: present && !evidenceEnvSatisfied && currentEnvSatisfied,
    blockedByDependencies: present && evidenceEnvSatisfied && dependencyBlockers.length > 0
  }
}

function finalGateBlockerSummary(finalGateBlockers, context = {}) {
  const operatorDetail = operatorGateDetail(
    finalGateBlockers,
    context.operatorDependencySummary,
    context.operatorCurrentEnvGate
  )
  const providerDetail = providerGateDetail(finalGateBlockers, context.providerGateDetail || context.providerAction)
  const summary = {
    schemaVersion: 1,
    dryRun: [],
    externalInput: [],
    operator: [],
    evidenceHygiene: [],
    worktreeHygiene: [],
    deterministic: [],
    other: [],
    requiresExternalInput: false,
    requiresNonDeepSeekProvider: false,
    providerGateDetail: providerDetail,
    requiresOperatorApproval: false,
    requiresOperatorEvidenceRefresh: false,
    operatorBlockedByDependencies: false,
    operatorDependencyBlockers: [],
    operatorGateDetail: operatorDetail,
    requiresLocalHygiene: false,
    requiresDeterministicFix: false
  }
  for (const blocker of finalGateBlockers) {
    if (blocker === 'dry-run-cannot-authorize-cutover') {
      summary.dryRun.push(blocker)
    } else if (blocker === 'operator:gate') {
      summary.operator.push(blocker)
    } else if (blocker.startsWith('provider:') || blocker.startsWith('mcp:')) {
      summary.externalInput.push(blocker)
    } else if (blocker.startsWith('evidence:') || blocker.startsWith('live-evidence')) {
      summary.evidenceHygiene.push(blocker)
    } else if (blocker.startsWith('worktree:')) {
      summary.worktreeHygiene.push(blocker)
    } else if (blocker.startsWith('deterministic-') || blocker.startsWith('deterministic-check:')) {
      summary.deterministic.push(blocker)
    } else {
      summary.other.push(blocker)
    }
  }
  summary.requiresExternalInput = summary.externalInput.length > 0
  summary.requiresNonDeepSeekProvider = providerDetail.requiresNonDeepSeekProvider
  summary.requiresOperatorApproval = operatorDetail.requiresOperatorApproval
  summary.requiresOperatorEvidenceRefresh = operatorDetail.requiresOperatorEvidenceRefresh
  summary.operatorBlockedByDependencies = operatorDetail.blockedByDependencies
  summary.operatorDependencyBlockers = operatorDetail.dependencyBlockers
  summary.requiresLocalHygiene = summary.evidenceHygiene.length > 0 || summary.worktreeHygiene.length > 0
  summary.requiresDeterministicFix = summary.deterministic.length > 0
  return summary
}

const checks = []
let failed = false

for (const item of commands) {
  const started = Date.now()
  if (!json) console.log(`[runtime-go-preflight] ${item.id}`)
  if (skipCommands) {
    checks.push({
      id: item.id,
      status: 'skipped',
      command: commandText(item),
      durationMs: 0
    })
    continue
  }
  const result = spawnSync(item.command, item.args, {
    cwd: process.cwd(),
    env: localCheckEnv(),
    stdio: json ? 'pipe' : 'inherit',
    encoding: 'utf8',
    maxBuffer: 64 * 1024 * 1024
  })
  const status = result.status ?? (result.signal ? 1 : 0)
  if (json && status !== 0) {
    if (result.stdout) process.stdout.write(result.stdout)
    if (result.stderr) process.stderr.write(result.stderr)
  }
  checks.push({
    id: item.id,
    status: status === 0 ? 'passed' : 'failed',
    command: commandText(item),
    durationMs: Date.now() - started
  })
  if (result.error) {
    console.error(result.error.message)
    failed = true
    break
  }
  if (status !== 0) {
    failed = true
    break
  }
}

const liveEvidence = readLiveEvidenceSummary()
const worktree = worktreeAudit()
const worktreeBlockers = worktreeFinalGateBlockers(worktree)
const relevantEvidenceBlockers = relevantEvidenceFinalGateBlockers(worktree)
const finalGateBlockers = uniqueStrings([
  ...(skipCommands ? ['dry-run-cannot-authorize-cutover'] : []),
  ...liveEvidence.finalGateBlockers,
  ...relevantEvidenceBlockers,
  ...worktreeBlockers
])
const finalGateBlocked = finalGateBlockers.length > 0
const nextActions = finalGateNextActions(finalGateBlockers, liveEvidence, worktree, skipCommands)
const currentOperatorEnvGate = operatorCurrentEnvGate()
const providerAction = Array.isArray(recordValue(nextActions).actions)
  ? nextActions.actions.find((item) => recordValue(item).id === 'provider:at-least-one-non-deepseek-provider')
  : undefined
const blockerSummary = finalGateBlockerSummary(finalGateBlockers, {
  providerAction,
  operatorDependencySummary: liveEvidence.operatorDependencySummary,
  operatorCurrentEnvGate: currentOperatorEnvGate
})
const localChecksStatus = failed ? 'failed' : skipCommands ? 'skipped' : 'passed'
const status = failed ? 'failed' : strictGate && finalGateBlocked ? 'live_blocked' : localChecksStatus

const report = {
  schemaVersion: 1,
  id: 'runtime-go-preflight',
  status,
  localChecksStatus,
  gateMode: strictGate,
  rcControlPlane,
  heavyweightChildrenExecuted: rcControlPlane ? 0 : 4,
  dryRunCannotAuthorizeCutover: skipCommands,
  expectedBlocked: skipCommands,
  finalGateBlocked,
  finalGateBlockers,
  finalGateBlockerSummary: blockerSummary,
  finalGateNextActions: nextActions,
  operatorCurrentEnvGate: currentOperatorEnvGate,
  operatorDependencySummary: liveEvidence.operatorDependencySummary,
  liveEvidence,
  worktree,
  rendererVisibleGoSwitcher: false,
  retiredGateIds: [
    'runtime-go-product-regression',
    'runtime-go-speed-cache-gate',
    'runtime-go-preflight'
  ],
  credentialedGateIds: [
    'provider-matrix-credentialed',
    'mcp-matrix-credentialed'
  ],
  checks
}

if (json || failed) {
  console.log(JSON.stringify(report, null, 2))
}

process.exit(failed || (strictGate && finalGateBlocked) ? 1 : 0)
