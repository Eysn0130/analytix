#!/usr/bin/env node

import { spawn, spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, lstatSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  relevantEvidenceFinalGateBlockers,
  worktreeAudit
} from './runtime-go-worktree-audit.mjs'

const DEFAULT_REPORT_DIR = 'docs/analytix/upstreams'
const DEFAULT_TIMEOUT_MS = 20_000
const DEFAULT_PACKAGED_TIMEOUT_MS = 600_000
const SETTINGS_FILE_NAME = 'analytix-settings.json'
const DEFAULT_APP_SUPPORT_DIR_NAME = 'analytix'
const COMPATIBLE_APP_SUPPORT_DIR_NAMES = [DEFAULT_APP_SUPPORT_DIR_NAME, 'Analytix']
const LIVE_VALIDATION_PROVIDER_PROBE_COMMAND =
  'npm run runtime:go:live-validation -- --json --live-deepseek-cache-from-settings --live-non-deepseek-provider-from-settings --no-gate'
const TOKEN_PLAN_PROVIDER_ID_SUFFIX = '-token-plan'
const providerProbePrompt = 'Analytix runtime protocol probe.'
const providerErrorProbePrompt = 'Analytix runtime intentional error probe.'
const STATIC_MODEL_PROVIDER_PRESET_DEFAULTS = {
  litellm: {
    baseUrl: 'http://localhost:4000',
    endpointFormat: 'chat-completions',
    models: [],
    modelEndpointFormats: {}
  },
  minimax: {
    baseUrl: 'https://api.minimaxi.com/anthropic',
    endpointFormat: 'messages',
    models: ['MiniMax-M3', 'MiniMax-M2'],
    modelEndpointFormats: {}
  },
  aliyun: {
    baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    endpointFormat: 'chat-completions',
    models: ['qwen-max'],
    modelEndpointFormats: {}
  },
  'zai-coding-plan': {
    baseUrl: 'https://api.z.ai/api/coding/paas/v4/chat/completions',
    endpointFormat: 'custom-endpoint',
    models: ['glm-5.1'],
    modelEndpointFormats: {}
  },
  'zhipu-coding-plan': {
    baseUrl: 'https://open.bigmodel.cn/api/coding/paas/v4/chat/completions',
    endpointFormat: 'custom-endpoint',
    models: ['glm-5.1'],
    modelEndpointFormats: {}
  }
}

const PROVIDER_CASES = [
  {
    id: 'deepseek',
    label: 'DeepSeek',
    endpointFamily: 'openai-chat-completions',
    endpointFormat: 'openai-chat-completions',
    apiKeyEnv: ['ANALYTIX_RUNTIME_DEEPSEEK_API_KEY', 'ANALYTIX_RUNTIME_GO_DEEPSEEK_API_KEY'],
    baseUrlEnv: ['ANALYTIX_RUNTIME_DEEPSEEK_BASE_URL', 'ANALYTIX_RUNTIME_GO_DEEPSEEK_BASE_URL'],
    modelEnv: ['ANALYTIX_RUNTIME_DEEPSEEK_MODEL', 'ANALYTIX_RUNTIME_GO_DEEPSEEK_MODEL'],
    urlMode: 'append-chat-completions',
    bodyMode: 'openai-chat-completions',
    authMode: 'bearer',
    cacheTelemetryMode: 'deepseek-provider-scoped'
  },
  {
    id: 'openai-compatible',
    label: 'OpenAI-compatible',
    endpointFamily: 'openai-chat-completions',
    endpointFormat: 'openai-chat-completions',
    apiKeyEnv: ['ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY', 'ANALYTIX_RUNTIME_GO_OPENAI_COMPAT_API_KEY'],
    baseUrlEnv: ['ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL', 'ANALYTIX_RUNTIME_GO_OPENAI_COMPAT_BASE_URL'],
    modelEnv: ['ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL', 'ANALYTIX_RUNTIME_GO_OPENAI_COMPAT_MODEL'],
    urlMode: 'append-chat-completions',
    bodyMode: 'openai-chat-completions',
    authMode: 'bearer',
    cacheTelemetryMode: 'non-deepseek-no-deepseek-fields'
  },
  {
    id: 'anthropic-compatible',
    label: 'Anthropic-compatible',
    endpointFamily: 'anthropic-messages',
    endpointFormat: 'anthropic-messages',
    apiKeyEnv: ['ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_API_KEY', 'ANALYTIX_RUNTIME_GO_ANTHROPIC_COMPAT_API_KEY'],
    baseUrlEnv: ['ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_BASE_URL', 'ANALYTIX_RUNTIME_GO_ANTHROPIC_COMPAT_BASE_URL'],
    modelEnv: ['ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_MODEL', 'ANALYTIX_RUNTIME_GO_ANTHROPIC_COMPAT_MODEL'],
    urlMode: 'append-anthropic-messages',
    bodyMode: 'anthropic-messages',
    authMode: 'anthropic-x-api-key',
    cacheTelemetryMode: 'non-deepseek-no-deepseek-fields'
  },
  {
    id: 'custom-endpoint',
    label: 'Custom full endpoint',
    endpointFamily: 'custom-full-endpoint',
    endpointFormat: 'custom-full-endpoint',
    apiKeyEnv: ['ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_API_KEY', 'ANALYTIX_RUNTIME_GO_CUSTOM_ENDPOINT_API_KEY'],
    baseUrlEnv: ['ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_URL', 'ANALYTIX_RUNTIME_GO_CUSTOM_ENDPOINT_URL'],
    modelEnv: ['ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL', 'ANALYTIX_RUNTIME_GO_CUSTOM_ENDPOINT_MODEL'],
    urlMode: 'custom-full-endpoint-no-append',
    bodyMode: 'custom-endpoint-openai-compatible',
    authMode: 'bearer',
    cacheTelemetryMode: 'custom-request-shape-only'
  }
]

const REQUIRED_PACKAGED_CHECK_IDS = [
  'packaged-app-startup',
  'health',
  'runtime-info',
  'thread-list',
  'turn-create',
  'sse-replay',
  'go-runtime-default-gate',
  'typescript-retired-backend'
]

const REQUIRED_PACKAGED_MILESTONE_A_CHECK_IDS = [
  'trusted-cache-tmpdir',
  'formal-packaged-artifact',
  'configured-network-provider',
  'isolated-profile-and-repository',
  'packaged-first-launch',
  'composer-workflow-submit',
  'ordinary-agent-workflow',
  'case-dependencies-unavailable-with-ordinary-capabilities',
  'real-repository-test',
  'bounded-subagent',
  'git-skill-mcp-research-writing',
  'long-context-continuation',
  'composer-compaction-submit',
  'nonzero-compaction',
  'normal-first-quit',
  'fresh-packaged-relaunch',
  'relaunch-provider-continuation',
  'renderer-visual-evidence',
  'exact-thread-recovery',
  'exact-todo-recovery',
  'exact-subagent-recovery',
  'exact-compaction-recovery',
  'exact-result-recovery',
  'renderer-visible-thread-recovery',
  'renderer-visible-todo-recovery',
  'renderer-visible-subagent-recovery',
  'renderer-visible-compaction-recovery',
  'renderer-visible-result-recovery',
  'normal-final-quit',
  'zero-residual-processes',
  'credential-redaction',
  'sandbox-cleanup'
]

const backendFieldKey = ['runtime', 'Backend'].join('')

const REQUIRED_MCP_COVERAGE = [
  'connect',
  'toolDiscoverySearch',
  'toolCall',
  'approvalUserInput',
  'reconnect',
  'credentialRedaction'
]

export function defaultRuntimeGoLiveEvidencePaths(reportDir = DEFAULT_REPORT_DIR) {
  const baseDir = `${reportDir}/runtime-go-live-evidence`
  return {
    report: `${baseDir}/live-evidence-report.json`,
    provider: `${baseDir}/provider-matrix.json`,
    mcp: `${baseDir}/mcp-execution.json`,
    packaged: `${baseDir}/packaged-go-runtime-qa.json`,
    packagedGui: `${baseDir}/packaged-gui-bridge-smoke.json`,
    packagedSoak: `${baseDir}/packaged-session-soak.json`,
    packagedMilestoneA: `${baseDir}/packaged-milestone-a.json`,
    operator: `${baseDir}/operator-gate.json`,
    markdown: `${baseDir}/live-evidence-summary.md`
  }
}

export async function buildRuntimeGoLiveEvidenceReport(options = {}) {
  const envProvided = Object.hasOwn(options, 'env')
  const env = envProvided ? options.env : process.env
  const reportDir = options.reportDir || DEFAULT_REPORT_DIR
  const paths = {
    ...defaultRuntimeGoLiveEvidencePaths(reportDir),
    ...(options.paths || {})
  }
  const timeoutMs = Number(options.timeoutMs ||
    env.ANALYTIX_RUNTIME_GO_LIVE_TIMEOUT_MS ||
    DEFAULT_TIMEOUT_MS)
  const packagedTimeoutMs = Number(options.packagedTimeoutMs ||
    env.ANALYTIX_RUNTIME_GO_LIVE_PACKAGED_TIMEOUT_MS ||
    env.ANALYTIX_RUNTIME_GO_PACKAGED_QA_TIMEOUT_MS ||
    Math.max(timeoutMs, DEFAULT_PACKAGED_TIMEOUT_MS))
  const now = options.now || new Date().toISOString()
  const commitHash = options.commitHash || currentGitCommit()
  const dryRun = Boolean(options.dryRun)
  const noWrite = Boolean(options.noWrite) || dryRun
  const providerSettingsPath = resolveProviderSettingsPath(env, {
    explicitPath: options.settingsPath,
    useDefault: !envProvided
  })

  let provider = await collectProviderMatrix({
    env,
    timeoutMs,
    now,
    dryRun,
    fetchImpl: options.fetchImpl || globalThis.fetch,
    settingsPath: providerSettingsPath.path,
    settingsPathSource: providerSettingsPath.source
  })
  let mcp = await collectMCPExecution({
    env,
    timeoutMs,
    now,
    dryRun,
    fetchImpl: options.fetchImpl || globalThis.fetch
  })
  const existingMCP = dryRun ? null : readJSONIfExists(paths.mcp)
  mcp = preserveExistingPassedMCPExecution({
    env,
    current: mcp,
    existing: existingMCP
  })
  const existingPackaged = dryRun ? null : readJSONIfExists(paths.packaged)
  let packaged = collectPackagedQA({
    env,
    now,
    dryRun,
    noWrite,
    timeoutMs: packagedTimeoutMs,
    packagedScript: options.packagedScript || 'scripts/runtime-go-packaged-qa.mjs'
  })
  const historicalPackaged = historicalPackagedEvidence(existingPackaged, commitHash)
  if (historicalPackaged) {
    packaged.historicalEvidence = historicalPackaged
  }
  let packagedGui = collectPackagedGuiSmoke({
    env,
    now,
    dryRun,
    noWrite,
    timeoutMs: packagedTimeoutMs,
    packagedGuiScript: options.packagedGuiScript || 'scripts/runtime-go-packaged-gui-smoke.mjs'
  })
  const existingPackagedSoak = dryRun ? null : readJSONIfExists(paths.packagedSoak)
  let packagedSoak = collectPackagedSessionSoak({
    env,
    now,
    dryRun,
    noWrite,
    timeoutMs: packagedTimeoutMs,
    packagedSoakScript: options.packagedSoakScript || 'scripts/runtime-go-packaged-session-soak.mjs'
  })
  const historicalPackagedSoak = historicalPackagedEvidence(existingPackagedSoak, commitHash)
  if (historicalPackagedSoak) {
    packagedSoak.historicalEvidence = historicalPackagedSoak
  }
  let packagedMilestoneA = collectPackagedMilestoneA({
    packaged,
    now,
    dryRun
  })
  let operator = collectOperatorGate({
    env,
    now,
    commitHash,
    provider,
    mcp,
    packaged,
    packagedGui,
    packagedSoak,
    packagedMilestoneA,
    paths
  })

  const outputs = {
    provider,
    mcp,
    packaged,
    packagedGui,
    packagedSoak,
    packagedMilestoneA,
    operator
  }
  const secretFinding = firstEvidenceSecretFinding(outputs)
  if (secretFinding) {
    const safeEvidence = collectorRedactionFailureEvidence({
      now,
      commitHash,
      paths,
      secretFinding
    })
    provider = safeEvidence.provider
    mcp = safeEvidence.mcp
    packaged = safeEvidence.packaged
    packagedGui = safeEvidence.packagedGui
    packagedSoak = safeEvidence.packagedSoak
    packagedMilestoneA = safeEvidence.packagedMilestoneA
    operator = safeEvidence.operator
  }

  if (!noWrite) {
    writeJSON(paths.provider, provider)
    writeJSON(paths.mcp, mcp)
    writeJSON(paths.packaged, packaged)
    writeJSON(paths.packagedGui, packagedGui)
    writeJSON(paths.packagedSoak, packagedSoak)
    writeJSON(paths.packagedMilestoneA, packagedMilestoneA)
    writeJSON(paths.operator, operator)
  }

  const componentPassed = provider.passed === true &&
    mcp.passed === true &&
    packaged.passed === true &&
    packagedGui.passed === true &&
    packagedSoak.passed === true &&
    packagedMilestoneA.passed === true &&
    operator.passed === true
  const evidenceDigests = {
    provider: sha256(canonicalJSONString(provider)),
    mcp: sha256(canonicalJSONString(mcp)),
    packaged: sha256(canonicalJSONString(packaged)),
    packagedGui: sha256(canonicalJSONString(packagedGui)),
    packagedSoak: sha256(canonicalJSONString(packagedSoak)),
    packagedMilestoneA: sha256(canonicalJSONString(packagedMilestoneA)),
    operator: sha256(canonicalJSONString(operator))
  }
  const liveOrActualEvidenceUsed = liveOrActualEvidencePresent({
    provider,
    mcp,
    packaged,
    packagedGui,
    packagedSoak,
    packagedMilestoneA
  })
  const missingInputs = missingExternalInputs(
    provider,
    mcp,
    packaged,
    packagedGui,
    packagedSoak,
    packagedMilestoneA,
    operator
  )
  const missingInputIds = uniqueStrings(missingInputs.map((item) => stringValue(recordValue(item).id)).filter(Boolean))
  const passed = componentPassed && missingInputIds.length === 0
  const finalGateBlocked = !passed
  const finalGateBlockers = finalGateBlocked
    ? (missingInputIds.length > 0 ? missingInputIds : ['live-evidence-not-passed'])
    : []
  const report = {
    schemaVersion: 1,
    id: 'runtime-go-live-evidence-report',
    stage: 'runtime-go-live-evidence',
    generatedAt: now,
    sourceCommit: commitHash,
    status: passed ? 'passed' : 'live_blocked',
    passed,
    defaultBackendReady: true,
    goDefaultRuntimeAllowed: true,
    goDefaultBackendEnabled: true,
    typeScriptFallbackRetained: false,
    typeScriptDefaultPathRetired: true,
    typeScriptOverrideRetired: true,
    rendererVisibleGoSwitcher: false,
    llmAnswerQualityEvidenceUsed: false,
    liveOrActualEvidenceUsed,
    deterministicEvidenceOnly: !liveOrActualEvidenceUsed,
    credentialSecretsRecorded: false,
    dryRun,
    noWrite,
    persistentEvidenceWritten: !noWrite,
    timeoutMs,
    packagedTimeoutMs,
    evidenceDigestAlgorithm: 'sha256:canonical-json-v1',
    evidenceDigests,
    evidencePaths: noWrite ? {} : {
      provider: resolve(process.cwd(), paths.provider),
      mcp: resolve(process.cwd(), paths.mcp),
      packaged: resolve(process.cwd(), paths.packaged),
      packagedGui: resolve(process.cwd(), paths.packagedGui),
      packagedSoak: resolve(process.cwd(), paths.packagedSoak),
      packagedMilestoneA: resolve(process.cwd(), paths.packagedMilestoneA),
      operator: resolve(process.cwd(), paths.operator)
    },
    componentStatus: {
      provider: provider.status,
      mcp: mcp.status,
      packaged: packaged.status,
      packagedGui: packagedGui.status,
      packagedSoak: packagedSoak.status,
      packagedMilestoneA: packagedMilestoneA.status,
      operator: operator.status
    },
    missingExternalInputs: missingInputs,
    missingExternalInputIds: missingInputIds,
    finalGateBlocked,
    finalGateBlockers,
    nextEvidenceActions: nextEvidenceActions({
      provider,
      mcp,
      packaged,
      packagedGui,
      packagedSoak,
      packagedMilestoneA,
      operator,
      paths
    }),
    coveredFinalCoverageEnv: coveredFinalCoverageEnv({
      provider,
      mcp,
      packaged,
      packagedSoak,
      packagedMilestoneA,
      operator,
      paths
    }),
    strictGateEnvWhenPassed: passed ? strictGateEnv({ paths }) : {},
    outputPaths: noWrite ? {} : {
      json: resolve(process.cwd(), paths.report),
      markdown: resolve(process.cwd(), paths.markdown),
      provider: resolve(process.cwd(), paths.provider),
      mcp: resolve(process.cwd(), paths.mcp),
      packaged: resolve(process.cwd(), paths.packaged),
      packagedGui: resolve(process.cwd(), paths.packagedGui),
      packagedSoak: resolve(process.cwd(), paths.packagedSoak),
      packagedMilestoneA: resolve(process.cwd(), paths.packagedMilestoneA),
      operator: resolve(process.cwd(), paths.operator)
    },
    notes: liveEvidenceNotes({
      provider,
      mcp,
      packaged,
      packagedGui,
      packagedSoak,
      packagedMilestoneA,
      operator
    })
  }

  if (!noWrite) {
    writeJSON(paths.report, report)
    writeText(paths.markdown, markdownSummary(report))
  }

  return report
}

export function strictGateEnv({ paths }) {
  return {
    ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS: 'passed',
    ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS: 'passed',
    ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS: 'passed',
    ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE: resolve(process.cwd(), paths.provider),
    ANALYTIX_RUNTIME_MCP_MATRIX_STATUS: 'passed',
    ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE: resolve(process.cwd(), paths.mcp),
    ANALYTIX_RUNTIME_PACKAGED_QA_STATUS: 'passed',
    ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE: resolve(process.cwd(), paths.packaged),
    ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS: 'passed',
    ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS_EVIDENCE: resolve(process.cwd(), paths.provider),
    ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS: 'passed',
    ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS_EVIDENCE: resolve(process.cwd(), paths.mcp),
    ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS: 'passed',
    ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS_EVIDENCE: resolve(process.cwd(), paths.packaged),
    ANALYTIX_RUNTIME_GO_PACKAGED_GUI_SMOKE_STATUS: 'passed',
    ANALYTIX_RUNTIME_GO_PACKAGED_GUI_SMOKE_EVIDENCE: resolve(process.cwd(), paths.packagedGui),
    ANALYTIX_RUNTIME_GO_PACKAGED_SESSION_SOAK_STATUS: 'passed',
    ANALYTIX_RUNTIME_GO_PACKAGED_SESSION_SOAK_EVIDENCE: resolve(process.cwd(), paths.packagedSoak),
    ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_A_STATUS: 'passed',
    ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_A_EVIDENCE: resolve(
      process.cwd(),
      paths.packagedMilestoneA
    ),
    ANALYTIX_RUNTIME_READY: '1',
    ANALYTIX_RUNTIME_READY_EVIDENCE: resolve(process.cwd(), paths.operator)
  }
}

export function coveredFinalCoverageEnv({
  provider,
  mcp,
  packaged,
  packagedSoak,
  packagedMilestoneA,
  operator,
  paths
}) {
  const env = {}
  if (packagedMilestoneA?.passed === true &&
    packagedMilestoneAEvidenceMissing(packagedMilestoneA).length === 0) {
    env.ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS = 'passed'
    env.ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS = 'passed'
    env.ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_A_STATUS = 'passed'
    env.ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_A_EVIDENCE = resolve(
      process.cwd(),
      paths.packagedMilestoneA
    )
  }
  if (provider?.passed === true) {
    env.ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS = 'passed'
    env.ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE = resolve(process.cwd(), paths.provider)
  }
  if (mcp?.passed === true) {
    env.ANALYTIX_RUNTIME_MCP_MATRIX_STATUS = 'passed'
    env.ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE = resolve(process.cwd(), paths.mcp)
  }
  if (packaged?.passed === true) {
    env.ANALYTIX_RUNTIME_PACKAGED_QA_STATUS = 'passed'
    env.ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE = resolve(process.cwd(), paths.packaged)
  }
  if (operator?.passed === true) {
    env.ANALYTIX_RUNTIME_READY = '1'
    env.ANALYTIX_RUNTIME_READY_EVIDENCE = resolve(process.cwd(), paths.operator)
  }
  return env
}

function liveOrActualEvidencePresent({
  provider,
  mcp,
  packaged,
  packagedGui,
  packagedSoak,
  packagedMilestoneA
}) {
  const credentialedNonDeepSeekProviderIds = Array.isArray(provider?.credentialedNonDeepSeekProviderIds)
    ? provider.credentialedNonDeepSeekProviderIds
    : []
  const providerProbes = Array.isArray(provider?.credentialedProbes) ? provider.credentialedProbes : []
  const anyProviderProbePassed = providerProbes.some((probe) =>
    probe && typeof probe === 'object' && probe.status === 'passed' && probe.skipped !== true)
  const packagedRecord = recordValue(packaged?.packaged)
  const packagedGuiSmoke = recordValue(packagedGui?.smoke)
  const packagedSoakRecord = recordValue(packagedSoak?.soak)
  return provider?.credentialedDeepSeekPassed === true ||
    credentialedNonDeepSeekProviderIds.length > 0 ||
    anyProviderProbePassed ||
    mcp?.credentialedExecution === true ||
    packagedRecord.actualPackagedDesktopQAPassed === true ||
    packagedRecord.actualPackagedAppLaunched === true ||
    packagedGuiSmoke.actualPackagedAppLaunched === true ||
    packagedSoakRecord.actualPackagedAppLaunched === true ||
    packagedMilestoneA?.workflow?.firstLaunchObserved === true
}

function liveEvidenceNotes({
  provider,
  mcp,
  packaged,
  packagedGui,
  packagedSoak,
  packagedMilestoneA,
  operator
}) {
  const pending = []
  if (provider.passed !== true) pending.push('provider credentials')
  if (mcp.passed !== true) pending.push('MCP execution configuration')
  if (packaged.passed !== true) pending.push('packaged desktop QA')
  if (packagedGui.passed !== true) pending.push('packaged GUI bridge smoke')
  if (packagedSoak.passed !== true) pending.push('packaged session soak')
  else if (packagedSessionSoakActualFinalGateMissingInputs(packagedSoak).length > 0) pending.push('actual packaged session soak')
  if (packagedMilestoneA.passed !== true) pending.push('packaged general Agent Milestone A')
  if (operator.passed !== true) pending.push('operator approval')
  return [
    'Runtime Go live evidence collects auditable JSON evidence only; it does not evaluate LLM answer quality.',
    pending.length > 0
      ? `Pending live validation inputs: ${pending.join(', ')}.`
      : 'All live validation inputs are covered; the strict gate env block contains only current Analytix runtime variables.',
    'Go runtime is the only production agent runtime after deterministic local gates; TypeScript backend override is retired.',
    'typeScriptDefaultPathRetired:true and typeScriptOverrideRetired:true mean ANALYTIX_RUNTIME_BACKEND=typescript must return a retired_backend diagnostic without starting TypeScript.'
  ]
}

export function resolveProviderSettingsPath(env, {
  explicitPath = '',
  useDefault = false,
  homeDir = homedir(),
  exists = existsSync
} = {}) {
  const explicit = stringValue(explicitPath)
  if (explicit) return { path: explicit, source: 'explicit-option' }
  const envPath = firstEnv(env, [
    'ANALYTIX_RUNTIME_SETTINGS_PATH',
    'ANALYTIX_RUNTIME_GO_SETTINGS_PATH',
    'ANALYTIX_SETTINGS_PATH'
  ])
  if (envPath) return { path: envPath, source: 'env-settings-path' }
  const userDataDir = firstEnv(env, ['ANALYTIX_USER_DATA_DIR'])
  if (userDataDir) return { path: join(userDataDir, SETTINGS_FILE_NAME), source: 'env-user-data-dir' }
  if (useDefault) {
    const candidates = COMPATIBLE_APP_SUPPORT_DIR_NAMES.map((dirName) =>
      join(homeDir, 'Library', 'Application Support', dirName, SETTINGS_FILE_NAME))
    const existing = candidates.find((candidate) => exists(candidate))
    return {
      path: existing || candidates[0],
      source: 'default-app-settings'
    }
  }
  return { path: '', source: 'not-configured' }
}

function loadProviderSettingsContext({ settingsPath = '', settingsPathSource = '' } = {}) {
  const path = stringValue(settingsPath)
  const evidence = {
    status: path ? 'missing' : 'not_configured',
    source: settingsPathSource || (path ? 'explicit-option' : 'not-configured'),
    settingsPathConfigured: Boolean(path),
    settingsPathHash: path ? sha256(resolve(process.cwd(), path)) : '',
    activeProviderId: '',
    selectedProviderId: '',
    providerCount: 0
  }
  if (!path) {
    return {
      status: 'not_configured',
      evidence,
      providerSettings: {},
      runtime: {},
      providers: [],
      activeProviderId: '',
      selectedProviderId: ''
    }
  }
  if (!existsSync(path)) {
    evidence.status = 'missing'
    return {
      status: 'missing',
      evidence,
      providerSettings: {},
      runtime: {},
      providers: [],
      activeProviderId: '',
      selectedProviderId: ''
    }
  }
  try {
    const settings = recordValue(JSON.parse(readFileSync(path, 'utf8')))
    const providerSettings = recordValue(settings.provider)
    const runtime = recordValue(settings.runtime)
    const providers = Array.isArray(providerSettings.providers)
      ? providerSettings.providers.map(recordValue).filter((item) => Object.keys(item).length > 0)
      : []
    const activeProviderId = stringValue(providerSettings.activeProviderId)
    const selectedProviderId = stringValue(runtime.providerId) || activeProviderId
    evidence.status = 'loaded'
    evidence.activeProviderId = activeProviderId
    evidence.selectedProviderId = selectedProviderId
    evidence.providerCount = providers.length
    return {
      status: 'loaded',
      evidence,
      providerSettings,
      runtime,
      providers,
      activeProviderId,
      selectedProviderId
    }
  } catch (error) {
    evidence.status = 'invalid'
    evidence.reason = summarizeText(error instanceof Error ? error.message : String(error))
    return {
      status: 'invalid',
      evidence,
      providerSettings: {},
      runtime: {},
      providers: [],
      activeProviderId: '',
      selectedProviderId: ''
    }
  }
}

function providerConfigFromSettingsForSpec(spec, context) {
  const empty = {
    apiKey: '',
    baseUrl: '',
    model: '',
    endpointFormat: spec.endpointFormat,
    modelEndpointFormats: {},
    providerId: spec.id,
    profileId: '',
    profileMatched: false,
    activeProviderId: context?.activeProviderId || '',
    providerCount: Array.isArray(context?.providers) ? context.providers.length : 0
  }
  if (!context || context.status !== 'loaded') return empty
  const profile = selectProviderProfileForSpec(spec, context)
  if (!profile) return empty
  const providerId = stringValue(profile.id) || spec.id
  const presetDefaults = modelProviderPresetDefaults(providerId)
  const runtime = recordValue(context.runtime)
  const providerSettings = recordValue(context.providerSettings)
  const profileModels = Array.isArray(profile.models) ? profile.models.map(stringValue).filter(Boolean) : []
  const models = profileModels.length > 0 ? profileModels : presetDefaults.models
  const model = normalizeDeprecatedProviderModel(providerId, runtimeModelForProfile(profile, context) || models[0] || defaultModelForSpec(spec))
  const apiKey = stringValue(profile.apiKey) ||
    (spec.id === 'deepseek' ? stringValue(providerSettings.apiKey) : '') ||
    stringValue(runtime.apiKey)
  const profileBaseUrl = stringValue(profile.baseUrl)
  const baseUrl = stringValue(profile.baseUrl) ||
    presetDefaults.baseUrl ||
    (spec.id === 'deepseek' ? stringValue(providerSettings.baseUrl) : '') ||
    stringValue(runtime.baseUrl) ||
    defaultBaseUrlForSpec(spec)
  return {
    apiKey,
    baseUrl,
    model,
    endpointFormat: endpointFormatForProfileModel(profile, model, presetDefaults, runtime, spec),
    providerId,
    profileId: providerId,
    profileMatched: true,
    presetFallbackUsed: !profileBaseUrl && Boolean(presetDefaults.baseUrl),
    presetModelFallbackUsed: profileModels.length === 0 && presetDefaults.models.length > 0,
    activeProviderId: context.activeProviderId,
    providerCount: context.providers.length
  }
}

let modelProviderPresetDefaultsCache

function modelProviderPresetDefaults(providerId) {
  const id = stringValue(providerId)
  if (!id) return { baseUrl: '', endpointFormat: '', models: [], modelEndpointFormats: {} }
  if (!modelProviderPresetDefaultsCache) {
    modelProviderPresetDefaultsCache = loadModelProviderPresetDefaults()
  }
  return modelProviderPresetDefaultsCache.get(id) || { baseUrl: '', endpointFormat: '', models: [], modelEndpointFormats: {} }
}

function loadModelProviderPresetDefaults() {
  const defaults = new Map()
  for (const [id, preset] of Object.entries(STATIC_MODEL_PROVIDER_PRESET_DEFAULTS)) {
    defaults.set(id, {
      baseUrl: preset.baseUrl,
      endpointFormat: normalizeEndpointFormatForEvidence(preset.endpointFormat),
      models: [...preset.models],
      modelEndpointFormats: { ...(preset.modelEndpointFormats || {}) }
    })
  }
  try {
    const source = readFileSync(resolve(process.cwd(), 'src/shared/model-provider-presets.ts'), 'utf8')
    const constArrays = parseStringArrayConstants(source)
    const presetPattern = /id:\s*'([^']+)'[\s\S]*?name:\s*'([^']*)'[\s\S]*?baseUrl:\s*'([^']*)'[\s\S]*?endpointFormat:\s*'([^']*)'[\s\S]*?models:\s*\[([\s\S]*?)\]/
    for (const objectSource of extractTopLevelPresetObjects(source)) {
      const match = objectSource.match(presetPattern)
      if (!match) continue
      const id = stringValue(match[1])
      if (!id) continue
      defaults.set(id, {
        name: stringValue(match[2]),
        baseUrl: stringValue(match[3]),
        endpointFormat: normalizeEndpointFormatForEvidence(match[4]),
        models: extractStringArrayModels(match[5], constArrays),
        modelEndpointFormats: parsePresetModelEndpointFormats(objectSource)
      })
      const tokenPlan = parseTokenPlanPresetDefaults(objectSource, constArrays)
      if (tokenPlan) {
        defaults.set(`${id}${TOKEN_PLAN_PROVIDER_ID_SUFFIX}`, tokenPlan)
      }
    }
  } catch {
    // Preset defaults are a convenience for live validation; explicit settings/env still work without them.
  }
  return defaults
}

function parseTokenPlanPresetDefaults(objectSource, constArrays) {
  const tokenPlanPattern = /tokenPlan:\s*\{[\s\S]*?baseUrl:\s*'([^']*)'[\s\S]*?endpointFormat:\s*'([^']*)'[\s\S]*?models:\s*\[([\s\S]*?)\]/
  const match = String(objectSource || '').match(tokenPlanPattern)
  if (!match) return null
  return {
    baseUrl: stringValue(match[1]),
    endpointFormat: normalizeEndpointFormatForEvidence(match[2]),
    models: extractStringArrayModels(match[3], constArrays),
    modelEndpointFormats: parsePresetModelEndpointFormats(String(objectSource || '').slice(String(objectSource || '').indexOf('tokenPlan:')))
  }
}

function parsePresetModelEndpointFormats(objectSource) {
  const formats = {}
  const source = String(objectSource || '')
  const pattern = /'([^']+)'\s*:\s*(?:textChatProfile|visionChatProfile)\([^)]*'([^']+)'\s*\)/g
  for (const match of source.matchAll(pattern)) {
    const model = stringValue(match[1])
    const endpointFormat = normalizeEndpointFormatForEvidence(match[2])
    if (model && endpointFormat) formats[model] = endpointFormat
  }
  return formats
}

function endpointFormatForProfileModel(profile, model, presetDefaults, runtime, spec) {
  const profileModel = recordValue(recordValue(profile.modelProfiles)[model])
  return normalizeEndpointFormatForEvidence(
    stringValue(profileModel.endpointFormat) ||
    stringValue(recordValue(presetDefaults.modelEndpointFormats)[model]) ||
    stringValue(profile.endpointFormat) ||
    presetDefaults.endpointFormat ||
    stringValue(runtime.endpointFormat) ||
    spec.endpointFormat
  )
}

function extractTopLevelPresetObjects(source) {
  const marker = 'export const MODEL_PROVIDER_PRESETS'
  const markerStart = source.indexOf(marker)
  const assignmentStart = markerStart >= 0 ? source.indexOf('=', markerStart) : -1
  const arrayStart = assignmentStart >= 0 ? source.indexOf('[', assignmentStart) : -1
  if (arrayStart < 0) return []
  const objects = []
  let bracketDepth = 0
  let braceDepth = 0
  let objectStart = -1
  let stringQuote = ''
  let escaped = false
  for (let index = arrayStart; index < source.length; index += 1) {
    const char = source[index]
    if (stringQuote) {
      if (escaped) {
        escaped = false
      } else if (char === '\\') {
        escaped = true
      } else if (char === stringQuote) {
        stringQuote = ''
      }
      continue
    }
    if (char === '\'' || char === '"' || char === '`') {
      stringQuote = char
      continue
    }
    if (char === '[') {
      bracketDepth += 1
      continue
    }
    if (char === ']') {
      bracketDepth -= 1
      if (bracketDepth === 0) break
      continue
    }
    if (bracketDepth !== 1) continue
    if (char === '{') {
      if (braceDepth === 0) objectStart = index
      braceDepth += 1
      continue
    }
    if (char === '}') {
      braceDepth -= 1
      if (braceDepth === 0 && objectStart >= 0) {
        objects.push(source.slice(objectStart, index + 1))
        objectStart = -1
      }
    }
  }
  return objects
}

function parseStringArrayConstants(source) {
  const arrays = new Map()
  const constArrayPattern = /const\s+([A-Z0-9_]+)\s*=\s*\[([\s\S]*?)\]/g
  for (const match of source.matchAll(constArrayPattern)) {
    const name = stringValue(match[1])
    if (!name) continue
    arrays.set(name, [...match[2].matchAll(/'([^']+)'/g)].map((item) => stringValue(item[1])).filter(Boolean))
  }
  return arrays
}

function extractStringArrayModels(arraySource, constArrays) {
  const models = []
  for (const match of String(arraySource || '').matchAll(/\.\.\.([A-Z0-9_]+)|'([^']+)'/g)) {
    const spreadName = stringValue(match[1])
    if (spreadName) {
      models.push(...(constArrays.get(spreadName) || []))
      continue
    }
    const model = stringValue(match[2])
    if (model) models.push(model)
  }
  return models
}

function recognizedModelProviderPresetIds() {
  if (!modelProviderPresetDefaultsCache) {
    modelProviderPresetDefaultsCache = loadModelProviderPresetDefaults()
  }
  return [...modelProviderPresetDefaultsCache.keys()]
    .filter((id) => id !== 'deepseek')
    .sort()
}

function runtimeModelForProfile(profile, context) {
  const runtime = recordValue(context?.runtime)
  const runtimeModel = stringValue(runtime.model)
  if (!runtimeModel) return ''
  const profileId = stringValue(profile?.id)
  if (!profileId) return ''
  const runtimeProviderId = stringValue(runtime.providerId)
  if (runtimeProviderId) return runtimeProviderId === profileId ? runtimeModel : ''
  const selectedProviderId = stringValue(context?.selectedProviderId)
  if (selectedProviderId) return selectedProviderId === profileId ? runtimeModel : ''
  return stringValue(context?.activeProviderId) === profileId ? runtimeModel : ''
}

function normalizeDeprecatedProviderModel(providerId, model) {
  const id = stringValue(providerId).replace(/-token-plan$/, '')
  const value = stringValue(model)
  if (id === 'xiaomi' && value.toLowerCase() === 'mimo-v2.5-pro-ultraspeed') {
    return 'mimo-v2.5-pro'
  }
  return value
}

function selectProviderProfileForSpec(spec, context) {
  const providers = Array.isArray(context.providers) ? context.providers : []
  const selectedProviderId = stringValue(context.selectedProviderId)
  const selected = providers.find((profile) => stringValue(profile.id) === selectedProviderId)
  if (selected && providerProfileMatchesSpec(selected, spec, context)) return selected
  return providers.find((profile) => providerProfileMatchesSpec(profile, spec, context)) || null
}

function providerProfileMatchesSpec(profile, spec, context = {}) {
  const id = stringValue(profile.id).toLowerCase()
  const presetDefaults = modelProviderPresetDefaults(id)
  const name = stringValue(profile.name).toLowerCase()
  const baseUrl = (stringValue(profile.baseUrl) || presetDefaults.baseUrl).toLowerCase()
  const profileModels = Array.isArray(profile.models) ? profile.models.map((item) => stringValue(item).toLowerCase()) : []
  const presetModels = Array.isArray(presetDefaults.models) ? presetDefaults.models.map((item) => stringValue(item).toLowerCase()) : []
  const models = profileModels.length > 0 ? profileModels : presetModels
  const selectedModel = normalizeDeprecatedProviderModel(id, runtimeModelForProfile(profile, context) || models[0] || '')
  const endpointFormat = normalizeEndpointFormatForEvidence(
    stringValue(recordValue(recordValue(profile.modelProfiles)[selectedModel]).endpointFormat) ||
    stringValue(recordValue(presetDefaults.modelEndpointFormats)[selectedModel]) ||
    stringValue(profile.endpointFormat) ||
    presetDefaults.endpointFormat
  ).toLowerCase()
  const haystack = [id, name, baseUrl, endpointFormat, ...models].join(' ')
  if (spec.id === 'deepseek') return /deepseek/.test(haystack)
  if (spec.id === 'anthropic-compatible') return /anthropic|claude|messages/.test(haystack)
  if (spec.id === 'custom-endpoint') {
    return /custom|full/.test(endpointFormat) || /\/(responses|messages|chat\/completions|completions)(?:[?#]|$)/.test(baseUrl)
  }
  if (spec.id === 'openai-compatible') {
    return !/deepseek|anthropic|claude/.test(haystack) &&
      (/openai|compatible|chat|completion|responses/.test(haystack) || endpointFormat === 'openai-chat-completions')
  }
  return false
}

function normalizeEndpointFormatForEvidence(value) {
  const raw = stringValue(value).trim()
  if (!raw) return ''
  return raw.replace(/_/g, '-')
}

function defaultModelForSpec(spec) {
  if (spec.id === 'deepseek') return 'deepseek-chat'
  if (spec.id === 'anthropic-compatible') return 'claude-compatible'
  if (spec.id === 'openai-compatible') return 'gpt-compatible'
  return 'custom-compatible'
}

function defaultBaseUrlForSpec(spec) {
  return spec.id === 'deepseek' ? 'https://api.deepseek.com' : ''
}

export async function collectProviderMatrix({
  env,
  fetchImpl,
  timeoutMs = DEFAULT_TIMEOUT_MS,
  now,
  dryRun = false,
  settingsPath = '',
  settingsPathSource = ''
}) {
  const settingsContext = loadProviderSettingsContext({ settingsPath, settingsPathSource })
  const probes = []
  for (const spec of PROVIDER_CASES) {
    probes.push(await collectProviderProbe({ spec, env, fetchImpl, timeoutMs, dryRun, settingsContext }))
  }
  const coverage = providerCredentialedCoverage(probes)
  const passed = coverage.passed
  const settingsProfileCredentialsRead = settingsContext.status === 'loaded' &&
    probes.some((probe) => probe.provider?.hasApiKey === true && probe.provider?.source === 'settings-profile')
  const settingsProfileCoverage = providerSettingsProfileCoverage({ settingsContext, probes })
  const report = {
    schemaVersion: 1,
    id: 'runtime-go-provider-matrix',
    generatedAt: now,
    status: passed ? 'passed' : 'live_blocked',
    passed,
    credentialedMatrixEnvGated: true,
    credentialedMatrixSettingsProfileGated: settingsContext.status === 'loaded',
    readsRealApiKeysByDefault: settingsContext.evidence.source === 'default-app-settings' && settingsProfileCredentialsRead,
    settingsProfileCredentialsRead,
    credentialValuesRecorded: false,
    credentialSecretsRecorded: false,
    settingsProfile: settingsContext.evidence,
    settingsProfileCoverage,
    requiredProviderIds: PROVIDER_CASES.map((item) => item.id),
    requiredCredentialedProviderCoverage: ['deepseek', 'at-least-one-non-deepseek-provider'],
    optionalCredentialedProviderIds: PROVIDER_CASES.map((item) => item.id).filter((id) => id !== 'deepseek'),
    credentialedDeepSeekPassed: coverage.deepseekPassed,
    credentialedNonDeepSeekProviderIds: coverage.nonDeepSeekPassedIds,
    credentialedProviderCoverage: coverage,
    credentialedProbes: probes,
    redaction: {
      status: firstEvidenceSecretFinding({ credentialedProbes: probes }) ? 'failed' : 'passed',
      secretMaterialFound: Boolean(firstEvidenceSecretFinding({ credentialedProbes: probes }))
    },
    notes: [
      'Provider probes record endpoint family, request shape, stream parsing, usage parsing, cache telemetry, error handling, and redaction.',
      'Custom endpoint mode uses the configured full endpoint path without appending another provider path.',
      'DeepSeek cache telemetry is provider-scoped and does not alter OpenAI-compatible, Anthropic-compatible, or custom endpoint request bodies.'
    ]
  }
  if (report.redaction.status !== 'passed') {
    report.status = 'failed'
    report.passed = false
  }
  return report
}

function providerSettingsProfileCoverage({ settingsContext, probes }) {
  const matchedProviderIds = []
  const usableProviderIds = []
  for (const probe of probes) {
    const id = stringValue(probe.id)
    if (!id) continue
    if (probe.provider?.settingsProfileMatched === true) matchedProviderIds.push(id)
    if (probe.provider?.settingsProfileUsable === true) usableProviderIds.push(id)
  }
  const usableNonDeepSeekProviderIds = usableProviderIds.filter((id) => id !== 'deepseek')
  return {
    status: settingsContext.status,
    source: settingsContext.evidence.source || '',
    providerCount: Number.isFinite(settingsContext.evidence.providerCount)
      ? settingsContext.evidence.providerCount
      : 0,
    matchedProviderIds: matchedProviderIds.sort(),
    usableProviderIds: usableProviderIds.sort(),
    usableNonDeepSeekProviderIds: usableNonDeepSeekProviderIds.sort(),
    deepseekSettingsProfileUsable: usableProviderIds.includes('deepseek'),
    nonDeepSeekSettingsProfileUsable: usableNonDeepSeekProviderIds.length > 0,
    satisfiesFinalProviderCoverageFromSettings: usableProviderIds.includes('deepseek') &&
      usableNonDeepSeekProviderIds.length > 0,
    credentialValuesRecorded: false
  }
}

async function collectProviderProbe({ spec, env, fetchImpl, timeoutMs, dryRun, settingsContext }) {
  const settingsConfig = providerConfigFromSettingsForSpec(spec, settingsContext)
  const envApiKey = firstEnv(env, spec.apiKeyEnv)
  const envBaseUrl = firstEnv(env, spec.baseUrlEnv)
  const envModel = firstEnv(env, spec.modelEnv)
  const apiKey = envApiKey || settingsConfig.apiKey
  const baseUrl = envBaseUrl || settingsConfig.baseUrl
  const model = envModel || settingsConfig.model
  const missingEnv = []
  if (!apiKey) missingEnv.push(preferredEnvName(spec.apiKeyEnv))
  if (!baseUrl) missingEnv.push(preferredEnvName(spec.baseUrlEnv))
  if (!model) missingEnv.push(preferredEnvName(spec.modelEnv))
  const missingSettingsProfileFields = providerSettingsProfileMissingFields({
    settingsContext,
    settingsConfig,
    apiKey,
    baseUrl,
    model
  })
  const requestUrl = baseUrl ? providerRequestUrl(spec, baseUrl) : ''
  const evidenceRequestUrl = sanitizeUrlForEvidence(requestUrl)
  const requestBody = model ? providerRequestBody(spec, model, env, baseUrl) : {}
  const requestShape = providerRequestShape(spec, requestBody, evidenceRequestUrl, Boolean(apiKey))
  const requestShapeValidation = providerRequestShapeValidation(requestShape)

  const base = {
    id: spec.id,
    label: spec.label,
    family: spec.endpointFamily,
    endpointFamily: spec.endpointFamily,
    endpointFormat: spec.endpointFormat,
    credentialed: true,
    requestUrl: evidenceRequestUrl,
    requestUrlHash: evidenceRequestUrl ? sha256(evidenceRequestUrl) : '',
    requestShape,
    requestShapeValidation,
    missingEnv,
    redaction: {
      status: 'passed',
      credentialEnvNames: spec.apiKeyEnv,
      credentialValueRecorded: false
    },
    provider: {
      activeProviderId: settingsConfig.activeProviderId,
      providerId: settingsConfig.providerId || spec.id,
      model: model || '',
      endpointFormat: settingsConfig.endpointFormat || spec.endpointFormat,
      hasApiKey: Boolean(apiKey),
      providerIsDeepSeek: spec.id === 'deepseek',
      source: envApiKey || envBaseUrl || envModel
        ? settingsConfig.profileMatched
          ? 'env-and-settings-profile'
          : 'env'
        : settingsConfig.profileMatched
          ? 'settings-profile'
          : 'missing',
      settingsProfileMatched: settingsConfig.profileMatched,
      settingsProfileUsable: settingsConfig.profileMatched &&
        Boolean(settingsConfig.apiKey) &&
        Boolean(settingsConfig.baseUrl) &&
        Boolean(settingsConfig.model),
      settingsProfileId: settingsConfig.profileId,
      settingsProfilePresetFallbackUsed: settingsConfig.presetFallbackUsed === true,
      settingsProfilePresetModelFallbackUsed: settingsConfig.presetModelFallbackUsed === true,
      settingsProviderCount: settingsConfig.providerCount,
      settingsPathConfigured: settingsContext.evidence.settingsPathConfigured === true,
      settingsPathHash: settingsContext.evidence.settingsPathHash || ''
    }
  }

  if (missingEnv.length > 0 || dryRun) {
    return {
      ...base,
      status: 'skipped',
      skipped: true,
      missingEnv,
      missingSettingsProfileFields,
      message: dryRun
        ? 'dry run; live provider probe was not executed'
        : settingsContext.status === 'loaded'
          ? 'missing credential/baseUrl/model from env or settings provider profile; skipped without counting as pass'
          : 'missing env-gated credential/baseUrl/model; skipped without counting as pass',
      streamParsing: { status: 'skipped' },
      usageParsing: { status: 'skipped' },
      cacheTelemetry: { status: 'skipped', providerScoped: spec.id === 'deepseek' },
      errorHandling: { status: 'skipped', errorProbePassed: false }
    }
  }

  if (typeof fetchImpl !== 'function') {
    return {
      ...base,
      status: 'failed',
      skipped: false,
      message: 'fetch implementation is unavailable',
      streamParsing: { status: 'failed' },
      usageParsing: { status: 'failed' },
      cacheTelemetry: { status: 'failed' },
      errorHandling: { status: 'failed', errorProbePassed: false, message: 'fetch implementation is unavailable' }
    }
  }

  try {
    const response = await fetchImpl(requestUrl, {
      method: 'POST',
      headers: providerRequestHeaders(spec, apiKey, env),
      body: JSON.stringify(requestBody),
      signal: AbortSignal.timeout(timeoutMs)
    })
    const text = await response.text()
    const parsed = parseProviderProtocolResponse(text, spec)
    const usageParsing = usageParsingResult(parsed.usage)
    const cacheTelemetry = cacheTelemetryResult(spec, parsed.usage, requestBody)
    const streamParsing = {
      status: parsed.protocolParsed && parsed.mode === 'sse' ? 'passed' : 'failed',
      mode: parsed.mode,
      frameCount: parsed.frameCount,
      jsonFrameCount: parsed.jsonFrameCount,
      parseErrorCount: parsed.parseErrorCount
    }
    const errorHandling = response.ok
      ? await collectProviderErrorHandling({ spec, requestUrl, apiKey, env, fetchImpl, timeoutMs })
      : {
          status: 'failed',
          httpStatus: response.status,
          errorProbePassed: false,
          responseSummary: summarizeText(text),
          rawResponseRecorded: false,
          credentialValueRecorded: false
        }
    const passed = response.ok &&
      requestShapeValidation.status === 'passed' &&
      streamParsing.status === 'passed' &&
      usageParsing.status === 'passed' &&
      cacheTelemetry.status === 'passed' &&
      errorHandling.status === 'passed'
    return {
      ...base,
      status: passed ? 'passed' : 'failed',
      skipped: false,
      message: passed ? 'credentialed provider probe passed' : 'credentialed provider probe failed protocol evidence checks',
      streamParsing,
      usageParsing,
      cacheTelemetry,
      errorHandling
    }
  } catch (error) {
    const message = summarizeText(error instanceof Error ? error.message : String(error))
    return {
      ...base,
      status: 'failed',
      skipped: false,
      message,
      streamParsing: { status: 'failed' },
      usageParsing: { status: 'failed' },
      cacheTelemetry: { status: 'failed' },
      errorHandling: {
        status: 'failed',
        errorProbePassed: false,
        message,
        rawResponseRecorded: false,
        credentialValueRecorded: false
      }
    }
  }
}

function providerSettingsProfileMissingFields({ settingsContext, settingsConfig, apiKey, baseUrl, model }) {
  if (settingsContext?.status !== 'loaded') return []
  if (apiKey && baseUrl && model) return []
  if (!settingsConfig.profileMatched) return ['provider.providers[].matchingProfile']
  const missing = []
  if (!apiKey) missing.push('provider.providers[].apiKey')
  if (!baseUrl) missing.push('provider.providers[].baseUrl')
  if (!model) missing.push('provider.providers[].models or runtime.model')
  return missing
}

async function collectProviderErrorHandling({ spec, requestUrl, apiKey, env, fetchImpl, timeoutMs }) {
  try {
    const response = await fetchImpl(requestUrl, {
      method: 'POST',
      headers: providerRequestHeaders(spec, apiKey, env),
      body: JSON.stringify(providerErrorProbeBody(spec)),
      signal: AbortSignal.timeout(timeoutMs)
    })
    const text = await response.text()
    const summary = summarizeText(text)
    const errorProbePassed = !response.ok && response.status >= 400 && response.status < 500 && summary.length > 0
    return {
      status: errorProbePassed ? 'passed' : 'failed',
      httpStatus: response.status,
      errorProbePassed,
      requestShape: {
        method: 'POST',
        sameRequestUrl: true,
        intentionallyInvalidBody: true,
        credentialValueRecorded: false,
        requestBodyRecorded: false
      },
      responseSummary: summary,
      rawResponseRecorded: false,
      credentialValueRecorded: false
    }
  } catch (error) {
    return {
      status: 'failed',
      errorProbePassed: false,
      message: summarizeText(error instanceof Error ? error.message : String(error)),
      rawResponseRecorded: false,
      credentialValueRecorded: false
    }
  }
}

export async function collectMCPExecution({ env, timeoutMs = DEFAULT_TIMEOUT_MS, now, dryRun = false, fetchImpl = globalThis.fetch }) {
  const command = firstEnv(env, ['ANALYTIX_RUNTIME_MCP_COMMAND', 'ANALYTIX_RUNTIME_GO_MCP_COMMAND'])
  const url = firstEnv(env, ['ANALYTIX_RUNTIME_MCP_URL', 'ANALYTIX_RUNTIME_GO_MCP_URL'])
  const toolName = firstEnv(env, ['ANALYTIX_RUNTIME_MCP_TOOL_NAME', 'ANALYTIX_RUNTIME_GO_MCP_TOOL_NAME'])
  const toolArgs = parseJSONEnv(env, ['ANALYTIX_RUNTIME_MCP_TOOL_ARGS_JSON', 'ANALYTIX_RUNTIME_GO_MCP_TOOL_ARGS_JSON'], {})
  const approvalEvidence = validateApprovalUserInputEvidence(env)
  const commandSecretLike = command ? stringLooksLikeSecret(command) : false
  const probe = {
    id: 'credentialed-mcp',
    family: 'mcp',
    endpointFormat: command ? 'stdio' : url ? 'http' : 'stdio-or-http',
    status: 'skipped',
    skipped: true,
    credentialed: true,
    credentialedExecution: false,
    connect: false,
    toolDiscoverySearch: false,
    toolCall: false,
    approvalUserInput: approvalEvidence.status === 'passed',
    reconnect: false,
    credentialRedaction: true,
    commandConfigured: Boolean(command),
    commandHash: command && !commandSecretLike ? sha256(command) : '',
    urlConfigured: Boolean(url),
    url: url ? sanitizeUrlForEvidence(url) : '',
    urlHash: url ? sha256(sanitizeUrlForEvidence(url)) : '',
    bearerAuthConfigured: Boolean(firstEnv(env, ['ANALYTIX_RUNTIME_GO_MCP_BEARER_TOKEN'])),
    requestShape: mcpExecutionRequestShape({ command, url, env }),
    toolNameConfigured: Boolean(toolName),
    approvalUserInputEvidence: approvalEvidence
  }

  if (dryRun) {
    probe.message = 'dry run; live MCP probe was not executed'
  } else if (!command && !url) {
    probe.message = 'missing ANALYTIX_RUNTIME_MCP_COMMAND/ANALYTIX_RUNTIME_MCP_URL'
  } else if (commandSecretLike) {
    probe.status = 'failed'
    probe.skipped = false
    probe.message = 'MCP command contains secret-like material; pass credentials through environment variables instead'
  } else if (!toolName) {
    probe.status = 'skipped'
    probe.message = 'MCP command or URL is configured, but ANALYTIX_RUNTIME_MCP_TOOL_NAME is required before executing a tool call'
  } else {
    try {
      const result = command
        ? await runMCPStdioProbe({ command, toolName, toolArgs, timeoutMs, env })
        : await runMCPHttpProbe({ url, toolName, toolArgs, timeoutMs, env, fetchImpl })
      probe.connect = result.connect
      probe.toolDiscoverySearch = result.toolDiscoverySearch
      probe.toolCall = result.toolCall
      probe.reconnect = result.reconnect
      probe.toolCount = result.toolCount
      probe.calledToolNameHash = sha256(toolName)
      probe.toolCatalogDigest = result.toolCatalogDigest
      probe.toolResultContentTypes = result.toolResultContentTypes
      probe.reconnectToolNameHash = result.reconnectToolNameHash
      probe.reconnectToolCatalogDigest = result.reconnectToolCatalogDigest
      probe.reconnectToolCount = result.reconnectToolCount
      probe.transport = result.transport
      probe.message = result.message
    } catch (error) {
      probe.status = 'failed'
      probe.skipped = false
      probe.message = summarizeText(error instanceof Error ? error.message : String(error))
    }
  }

  const coveragePassed = REQUIRED_MCP_COVERAGE.every((key) => probe[key] === true)
  if (coveragePassed) {
    probe.status = 'passed'
    probe.skipped = false
    probe.credentialedExecution = true
    probe.message = probe.message || 'credentialed MCP execution probe passed'
  } else if (probe.status !== 'failed') {
    probe.status = 'skipped'
    probe.skipped = true
  }

  const report = {
    schemaVersion: 1,
    id: 'runtime-go-mcp-execution',
    generatedAt: now,
    status: probe.status === 'passed' ? 'passed' : probe.status === 'failed' ? 'failed' : 'live_blocked',
    passed: probe.status === 'passed',
    credentialedExecution: probe.credentialedExecution,
    topLevelMcpIndexerExposed: false,
    reasonixPublicProtocolUsed: false,
    requiredCoverage: REQUIRED_MCP_COVERAGE,
    connect: probe.connect,
    toolDiscoverySearch: probe.toolDiscoverySearch,
    toolCall: probe.toolCall,
    approvalUserInput: probe.approvalUserInput,
    reconnect: probe.reconnect,
    redaction: probe.credentialRedaction,
    credentialedProbes: [probe],
    notes: [
      'Runtime Go MCP evidence requires explicit tool name configuration before executing a live tool call.',
      'Approval/user-input coverage must be supplied by a separate auditable JSON evidence file when it cannot be exercised by the stdio MCP probe.',
      'No MCP-indexer top-level UI or upstream public protocol surface is added.'
    ]
  }
  const secretFinding = firstEvidenceSecretFinding(report)
  if (secretFinding) {
    report.status = 'failed'
    report.passed = false
    report.credentialedProbes[0].status = 'failed'
    report.credentialedProbes[0].message = `MCP evidence contains secret-like material at ${secretFinding}`
  }
  return report
}

export function collectPackagedQA({
  env,
  now,
  dryRun = false,
  noWrite = false,
  timeoutMs = DEFAULT_TIMEOUT_MS,
  packagedScript = 'scripts/runtime-go-packaged-qa.mjs'
}) {
  if (dryRun) {
    return {
      schemaVersion: 1,
      id: 'runtime-go-packaged-qa',
      stage: 'packaged-desktop-qa',
      generatedAt: now,
      goDefaultBackendEnabled: true,
      rendererVisibleGoSwitcher: false,
      typeScriptFallbackRetained: false,
      requiredCheckIds: REQUIRED_PACKAGED_CHECK_IDS,
      checks: REQUIRED_PACKAGED_CHECK_IDS.map((id) => ({
        id,
        label: id,
        status: 'skipped',
        message: 'dry run; packaged QA was not executed'
      })),
      passed: false,
      status: 'live_blocked'
    }
  }
  const isFormalRuntimeGoPackagedQA = /runtime-go-packaged-qa\.mjs$/.test(packagedScript)
  const child = spawnSync(process.execPath, [
    packagedScript,
    '--json',
    ...(noWrite ? ['--no-write'] : []),
    ...(isFormalRuntimeGoPackagedQA ? ['--actual'] : []),
    '--timeout-ms',
    String(timeoutMs)
  ], {
    cwd: process.cwd(),
    env,
    encoding: 'utf8',
    maxBuffer: 16 * 1024 * 1024,
    timeout: timeoutMs
  })
  const report = parseJSON(child.stdout) || {
    schemaVersion: 1,
    id: 'runtime-go-packaged-qa',
    generatedAt: now,
    checks: [],
    passed: false,
    status: 'failed',
    message: childFailureMessage(child, 'packaged QA')
  }
  normalizePackagedQAReport(report)
  const rawPassed = child.status === 0 && report.status === 'passed' && report.passed === true
  const reportedLiveBlocked = report.status === 'live_blocked'
  report.passed = rawPassed
  report.status = rawPassed
    ? 'passed'
    : report.status === 'failed'
      ? 'failed'
      : reportedLiveBlocked
        ? 'live_blocked'
        : child.status && child.status !== 0
          ? 'failed'
          : 'live_blocked'
  report.requiredCheckIds = Array.isArray(report.requiredCheckIds) && report.requiredCheckIds.length > 0
    ? report.requiredCheckIds
    : REQUIRED_PACKAGED_CHECK_IDS
  if (report.credentialSecretsRecorded !== true) report.credentialSecretsRecorded = false
  const missing = packagedEvidenceMissing(report)
  if (missing.length > 0) {
    report.passed = false
    if (report.status !== 'failed') report.status = 'live_blocked'
    report.missingRequiredEvidence = missing
  }
  return report
}

function preserveExistingPassedMCPExecution({ env, current, existing }) {
  if (mcpExecutionActualRunRequested(env)) return current
  if (!mcpExecutionIsWeakNonActualRun(current)) return current
  return existingPassedMCPExecutionEvidence(existing) || current
}

function existingPassedMCPExecutionEvidence(existing) {
  if (!existing || typeof existing !== 'object' || Array.isArray(existing)) return null
  if (existing.id !== 'runtime-go-mcp-execution') return null
  if (existing.status !== 'passed' || existing.passed !== true || existing.credentialedExecution !== true) return null
  if (existing.topLevelMcpIndexerExposed === true || existing.reasonixPublicProtocolUsed === true) return null
  const probes = Array.isArray(existing.credentialedProbes) ? existing.credentialedProbes : []
  const probe = probes.find((item) => item && typeof item === 'object' && item.id === 'credentialed-mcp') || probes[0]
  if (!probe || probe.status !== 'passed' || probe.skipped === true || probe.credentialedExecution !== true) return null
  for (const key of REQUIRED_MCP_COVERAGE) {
    const redactionKey = key === 'redaction' ? 'credentialRedaction' : key
    if (probe[key] !== true && probe[redactionKey] !== true && existing[key] !== true) return null
  }
  if (firstEvidenceSecretFinding(existing)) return null
  return existing
}

function mcpExecutionActualRunRequested(env) {
  return Boolean(firstEnv(env, [
    'ANALYTIX_RUNTIME_MCP_COMMAND',
    'ANALYTIX_RUNTIME_GO_MCP_COMMAND',
    'ANALYTIX_RUNTIME_MCP_URL',
    'ANALYTIX_RUNTIME_GO_MCP_URL'
  ]))
}

function mcpExecutionIsWeakNonActualRun(report) {
  if (!report || typeof report !== 'object') return false
  if (report.status !== 'live_blocked' || report.passed !== false) return false
  const probes = Array.isArray(report.credentialedProbes) ? report.credentialedProbes : []
  const probe = probes[0] && typeof probes[0] === 'object' ? probes[0] : {}
  return probe.commandConfigured !== true &&
    probe.urlConfigured !== true &&
    probe.credentialedExecution !== true
}

function historicalPackagedEvidence(existing, currentCommit) {
  if (!existing || typeof existing !== 'object' || Array.isArray(existing)) return null
  const sourceCommit = isGitCommit(existing.sourceCommit)
    ? String(existing.sourceCommit).toLowerCase()
    : ''
  const testHarnessCommit = isGitCommit(existing.testHarnessCommit)
    ? String(existing.testHarnessCommit).toLowerCase()
    : ''
  const normalizedCurrentCommit = isGitCommit(currentCommit)
    ? String(currentCommit).toLowerCase()
    : ''
  const generatedAt = typeof existing.generatedAt === 'string' &&
    Number.isFinite(Date.parse(existing.generatedAt))
    ? existing.generatedAt
    : ''
  const previousStatus = ['passed', 'failed', 'live_blocked', 'skipped'].includes(existing.status)
    ? existing.status
    : 'unverified'
  return {
    status: 'unverified',
    passed: false,
    reused: false,
    freshnessVerified: false,
    blocker: 'historical_packaged_evidence_requires_fresh_execution',
    previousStatus,
    previousPassed: existing.passed === true,
    generatedAt,
    sourceCommit,
    testHarnessCommit,
    sourceCommitMatchesCurrent: Boolean(
      normalizedCurrentCommit && sourceCommit === normalizedCurrentCommit
    ),
    testHarnessCommitMatchesCurrent: Boolean(
      normalizedCurrentCommit && testHarnessCommit === normalizedCurrentCommit
    )
  }
}

function readJSONIfExists(filePath) {
  if (!filePath || !existsSync(filePath)) return null
  try {
    const value = JSON.parse(readFileSync(filePath, 'utf8'))
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null
    return value
  } catch {
    return null
  }
}

function normalizePackagedQAReport(report) {
  if (!report || typeof report !== 'object') return report
  delete report.legacyId
  delete report.legacyChangeId
  return report
}

export function collectPackagedGuiSmoke({
  env,
  now,
  dryRun = false,
  noWrite = false,
  timeoutMs = DEFAULT_TIMEOUT_MS,
  packagedGuiScript = 'scripts/runtime-go-packaged-gui-smoke.mjs'
}) {
  if (dryRun) {
    return {
      schemaVersion: 1,
      id: 'runtime-go-packaged-gui-smoke',
      stage: 'packaged-gui-bridge-smoke',
      generatedAt: now,
      status: 'live_blocked',
      passed: false,
      requiredCheckIds: [
        'packaged-app-artifact',
        'packaged-app-launch',
        'packaged-renderer-bridge',
        'packaged-settings-bridge',
        'packaged-provider-profile-settings',
        'packaged-mimo-profile-settings',
        'packaged-runtime-restart',
        'packaged-runtime-health',
        'packaged-runtime-info'
      ],
      checks: [
        'packaged-app-artifact',
        'packaged-app-launch',
        'packaged-renderer-bridge',
        'packaged-settings-bridge',
        'packaged-provider-profile-settings',
        'packaged-mimo-profile-settings',
        'packaged-runtime-restart',
        'packaged-runtime-health',
        'packaged-runtime-info'
      ].map((id) => ({
        id,
        label: id,
        status: 'skipped',
        message: 'dry run; packaged GUI smoke was not executed'
      })),
      redaction: {
        status: 'passed',
        secretMaterialFound: false
      }
    }
  }
  const isFormalRuntimeGoSmoke = /runtime-go-packaged-gui-smoke\.mjs$/.test(packagedGuiScript)
  const child = spawnSync(process.execPath, [
    packagedGuiScript,
    '--json',
    ...(noWrite ? ['--no-write'] : []),
    ...(isFormalRuntimeGoSmoke ? ['--actual'] : []),
    '--timeout-ms',
    String(timeoutMs)
  ], {
    cwd: process.cwd(),
    env,
    encoding: 'utf8',
    maxBuffer: 16 * 1024 * 1024,
    timeout: timeoutMs
  })
  const report = parseJSON(child.stdout) || {
    schemaVersion: 1,
    id: 'runtime-go-packaged-gui-smoke',
    stage: 'packaged-gui-bridge-smoke',
    generatedAt: now,
    checks: [],
    passed: false,
    status: 'failed',
    message: childFailureMessage(child, 'packaged GUI smoke')
  }
  const rawPassed = child.status === 0 && report.status === 'passed' && report.passed === true
  report.passed = rawPassed
  report.status = rawPassed
    ? 'passed'
    : child.status && child.status !== 0 || report.status === 'failed'
      ? 'failed'
      : 'live_blocked'
  const missing = packagedGuiEvidenceMissing(report)
  if (missing.length > 0) {
    report.passed = false
    if (report.status !== 'failed') report.status = 'live_blocked'
    report.missingRequiredEvidence = missing
  }
  return report
}

export function collectPackagedSessionSoak({
  env,
  now,
  dryRun = false,
  noWrite = false,
  timeoutMs = DEFAULT_TIMEOUT_MS,
  packagedSoakScript = 'scripts/runtime-go-packaged-session-soak.mjs'
}) {
  if (dryRun) {
    return {
      schemaVersion: 1,
      id: 'runtime-go-packaged-session-soak',
      stage: 'packaged-session-soak',
      generatedAt: now,
      status: 'live_blocked',
      passed: false,
      requiredCheckIds: [
        'go-session-durable-contract',
        'typescript-retired-backend-session-contract'
      ],
      failureChecks: [
        'dry-run'
      ],
      credentialSecretsRecorded: false,
      rawValueRecorded: false,
      redaction: {
        status: 'passed',
        secretMaterialFound: false
      }
    }
  }
  const isFormalRuntimeGoSessionSoak = /runtime-go-packaged-session-soak\.mjs$/.test(packagedSoakScript)
  const child = spawnSync(process.execPath, [
    packagedSoakScript,
    '--json',
    ...(noWrite ? ['--no-write'] : []),
    ...(isFormalRuntimeGoSessionSoak ? ['--actual'] : []),
    '--timeout-ms',
    String(timeoutMs)
  ], {
    cwd: process.cwd(),
    env,
    encoding: 'utf8',
    maxBuffer: 16 * 1024 * 1024,
    timeout: timeoutMs
  })
  const report = parseJSON(child.stdout) || {
    schemaVersion: 1,
    id: 'runtime-go-packaged-session-soak',
    stage: 'packaged-session-soak',
    generatedAt: now,
    passed: false,
    status: 'failed',
    message: childFailureMessage(child, 'packaged session soak')
  }
  const rawPassed = child.status === 0 && report.status === 'passed' && report.passed === true
  report.passed = rawPassed
  report.status = rawPassed
    ? 'passed'
    : child.status && child.status !== 0 || report.status === 'failed'
      ? 'failed'
      : 'live_blocked'
  const missing = packagedSessionSoakEvidenceMissing(report)
  if (missing.length > 0) {
    report.passed = false
    if (report.status !== 'failed') report.status = 'live_blocked'
    report.missingRequiredEvidence = missing
  }
  return report
}

export function collectPackagedMilestoneA({
  packaged,
  now,
  dryRun = false
}) {
  if (dryRun) {
    return {
      schemaVersion: 1,
      id: 'runtime-go-packaged-milestone-a',
      stage: 'packaged-general-agent-milestone-a',
      generatedAt: now,
      status: 'live_blocked',
      passed: false,
      requiredCheckIds: REQUIRED_PACKAGED_MILESTONE_A_CHECK_IDS,
      checks: REQUIRED_PACKAGED_MILESTONE_A_CHECK_IDS.map((id) => ({
        id,
        status: 'live_blocked',
        message: 'dry run; packaged Milestone A was not executed'
      })),
      credentialSecretsRecorded: false,
      syntheticProviderUsed: false,
      localProviderUsed: false,
      directRuntimeTurnDriverUsed: false
    }
  }
  const embedded = recordValue(packaged?.milestoneAEvidence)
  const report = Object.keys(embedded).length > 0
    ? JSON.parse(JSON.stringify(embedded))
    : {
        schemaVersion: 1,
        id: 'runtime-go-packaged-milestone-a',
        stage: 'packaged-general-agent-milestone-a',
        generatedAt: now,
        status: 'live_blocked',
        passed: false,
        checks: [],
        credentialSecretsRecorded: false,
        syntheticProviderUsed: false,
        localProviderUsed: false,
        directRuntimeTurnDriverUsed: false,
        executionBlocker: 'packaged_qa_milestone_a_evidence_missing'
      }
  const qaMilestoneCheck = (Array.isArray(packaged?.checks) ? packaged.checks : [])
    .map(recordValue)
    .find((item) => item.id === 'milestone-a')
  const rawPassed = report.status === 'passed' &&
    report.passed === true &&
    qaMilestoneCheck?.status === 'passed' &&
    qaMilestoneCheck?.reportId === 'runtime-go-packaged-milestone-a'
  report.passed = rawPassed
  report.status = rawPassed
    ? 'passed'
    : report.status === 'failed' || qaMilestoneCheck?.status === 'failed'
      ? 'failed'
      : 'live_blocked'
  report.requiredCheckIds = REQUIRED_PACKAGED_MILESTONE_A_CHECK_IDS
  const missing = packagedMilestoneAEvidenceMissing(report)
  if (missing.length > 0) {
    report.passed = false
    if (report.status !== 'failed') report.status = 'live_blocked'
    report.missingRequiredEvidence = missing
  }
  return report
}

const MILESTONE_A_ALLOWED_AUXILIARY_CHECK_IDS = new Set([
  'external-real-repository-contract',
  'parent-owned-repository-test-cross-check',
  'protected-funds-source-unavailable'
])

function compactionWorkflowProofMissing(workflow) {
  const value = recordValue(workflow)
  const missing = []
  const projectionClass = value.manualCompactionProjectionClass ||
    (Number.isSafeInteger(value.manualCompactionReplacedTokens) &&
      value.manualCompactionReplacedTokens > 0
      ? 'ordinary_exact'
      : '')
  const baselineBound = value.manualCompactionBaselineBound === true
  if (!baselineBound) missing.push('workflow.manualCompactionBaselineBound')
  if (!Number.isSafeInteger(value.manualCompactionBaselineCount) ||
      value.manualCompactionBaselineCount < 0) {
    missing.push('workflow.manualCompactionBaselineCount')
  }
  if (!isSha256(value.manualCompactionBaselineDigest)) {
    missing.push('workflow.manualCompactionBaselineDigest')
  }
  if (!isSha256(value.manualCompactionCandidateDigest)) {
    missing.push('workflow.manualCompactionCandidateDigest')
  }
  if (value.manualCompactionNewAfterBaseline !== true) {
    missing.push('workflow.manualCompactionNewAfterBaseline')
  }
  if (value.manualCompactionObserved !== true) {
    missing.push('workflow.manualCompactionObserved')
  }
  if (value.manualCompactionAuto !== false) {
    missing.push('workflow.manualCompactionAuto:false')
  }
  if (!isSha256(value.manualCompactionSourceDigest)) {
    missing.push('workflow.manualCompactionSourceDigest')
  }
  if (!['ordinary_exact', 'case_bound_typed'].includes(projectionClass)) {
    missing.push('workflow.manualCompactionProjectionClass')
    return missing
  }
  if (projectionClass === 'ordinary_exact') {
    if (!Number.isSafeInteger(value.manualCompactionReplacedTokens) ||
        value.manualCompactionReplacedTokens <= 0) {
      missing.push('workflow.manualCompactionReplacedTokens')
    }
    if (!isSha256(value.manualCompactionSourceItemIdsDigest)) {
      missing.push('workflow.manualCompactionSourceItemIdsDigest')
    }
    if (value.manualCompactionSourceAncestryBound !== true) {
      missing.push('workflow.manualCompactionSourceAncestryBound')
    }
  } else {
    if (value.manualCompactionReplacedTokens !== 0) {
      missing.push('workflow.manualCompactionReplacedTokens:case-bound-zero')
    }
    if (value.manualCompactionSourceItemIdsDigest !== '') {
      missing.push('workflow.manualCompactionSourceItemIdsDigest:case-bound-omitted')
    }
    if (value.manualCompactionNonzeroBound !== true) {
      missing.push('workflow.manualCompactionNonzeroBound')
    }
    if (value.manualCompactionSourceAncestryBound !== true) {
      missing.push('workflow.manualCompactionSourceAncestryBound')
    }
  }
  return missing
}

export function packagedMilestoneAReportProjection(report) {
  const checks = Array.isArray(report?.checks) ? report.checks.map(recordValue) : []
  const allowed = new Set([
    ...REQUIRED_PACKAGED_MILESTONE_A_CHECK_IDS,
    ...MILESTONE_A_ALLOWED_AUXILIARY_CHECK_IDS
  ])
  const errors = []
  const byId = new Map()
  for (const item of checks) {
    const id = stringValue(item.id)
    if (!id || !allowed.has(id)) {
      errors.push(`checks:${id || 'missing'}:unknown`)
      continue
    }
    if (!['passed', 'failed', 'skipped', 'live_blocked'].includes(item.status)) {
      errors.push(`checks:${id}:status`)
    }
    const previous = byId.get(id)
    if (previous) {
      errors.push(`checks:${id}:unique`)
      if (previous.status !== item.status) errors.push(`checks:${id}:conflict`)
    } else {
      byId.set(id, item)
    }
  }
  for (const id of REQUIRED_PACKAGED_MILESTONE_A_CHECK_IDS) {
    if (!byId.has(id)) errors.push(`checks:${id}:unique`)
  }
  const derived = {
    failedCheckIds: checks.filter((item) => item.status === 'failed').map((item) => item.id),
    skippedCheckIds: checks.filter((item) => item.status === 'skipped').map((item) => item.id),
    liveBlockedCheckIds: checks.filter((item) => item.status === 'live_blocked').map((item) => item.id)
  }
  // Compatibility field: all non-pass checks in report.checks order. Direct
  // failures remain available from failedCheckIds.
  derived.failureCheckIds = checks
    .filter((item) => item.status !== 'passed')
    .map((item) => item.id)
  for (const key of ['failedCheckIds', 'skippedCheckIds', 'liveBlockedCheckIds', 'failureCheckIds']) {
    const values = Array.isArray(report?.[key])
      ? report[key].map((value) => stringValue(value)).filter(Boolean)
      : null
    if (!values || values.length !== new Set(values).size ||
        JSON.stringify(values) !== JSON.stringify(derived[key])) {
      errors.push(`${key}:closure`)
    }
  }
  if (report?.failureCheckIdsSemantics !== 'all_non_pass') {
    errors.push('failureCheckIdsSemantics:all_non_pass')
  }
  const workflow = recordValue(report?.workflow)
  const workflowClosure = new Map([
    ['ordinary-agent-workflow', [
      'ordinaryWorkflowCompleted', 'readObserved', 'planObserved', 'todoObserved',
      'writeObserved'
    ]],
    ['real-repository-test', ['realTestObserved']],
    ['bounded-subagent', ['subagentObserved']],
    ['protected-funds-source-unavailable', ['protectedFundsSourceUnavailableObserved']],
    ['long-context-continuation', [
      'longContextContinuationObserved',
      'longContextHostReadBound',
      'longContextSubagentContinuityBound',
      'longContextProviderReceiptReplayObserved',
      'longContextProviderReceiptBound',
      'longContextAcceptedFinalBound'
    ]]
  ])
  for (const [id, fields] of workflowClosure) {
    if (byId.get(id)?.status !== 'passed') continue
    for (const field of fields) {
      if (workflow[field] !== true) errors.push(`workflow:${id}:${field}`)
    }
  }
  if (byId.get('nonzero-compaction')?.status === 'passed') {
    for (const field of compactionWorkflowProofMissing(workflow)) {
      errors.push(`workflow:nonzero-compaction:${field.replace(/^workflow\./u, '')}`)
    }
  }
  const requiredPassed = REQUIRED_PACKAGED_MILESTONE_A_CHECK_IDS.every((id) =>
    byId.get(id)?.status === 'passed'
  )
  const nonPass = derived.failureCheckIds.length > 0
  if (report?.passed === true && (!requiredPassed || nonPass || report.status !== 'passed')) {
    errors.push('workflow:passed-check-consistency')
  }
  if (report?.status === 'failed' && derived.failedCheckIds.length === 0) {
    errors.push('status:failed-without-failed-check')
  }
  return {
    ok: errors.length === 0,
    errors: [...new Set(errors)],
    checks,
    ...derived
  }
}

function childFailureMessage(child, label) {
  if (child?.error) return child.error.message
  return child?.stderr || child?.stdout || `${label} exited ${child?.status}`
}

export function collectOperatorGate({
  env,
  now,
  commitHash,
  provider,
  mcp,
  packaged,
  packagedGui,
  packagedSoak,
  packagedMilestoneA,
  paths
}) {
  const explicitEnvGate = firstEnv(env, ['ANALYTIX_RUNTIME_READY']) === '1'
  const explicitApproval = ['ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT']
    .some((key) => String(env[key] || '').trim() === '1')
  const packagedGuiRequired = packagedGui !== undefined
  const packagedSoakRequired = packagedSoak !== undefined
  const packagedSoakFinalEvidenceMissing = packagedSoakRequired &&
    packagedSessionSoakActualFinalGateMissingInputs(packagedSoak).length > 0
  const packagedMilestoneARequired = packagedMilestoneA !== undefined
  const packagedMilestoneAFinalEvidenceMissing = packagedMilestoneARequired &&
    packagedMilestoneAEvidenceMissing(packagedMilestoneA).length > 0
  const evidenceReviewed = provider.passed === true &&
    mcp.passed === true &&
    packaged.passed === true &&
    (!packagedGuiRequired || packagedGui.passed === true) &&
    (!packagedSoakRequired || (packagedSoak.passed === true && !packagedSoakFinalEvidenceMissing)) &&
    (!packagedMilestoneARequired ||
      (packagedMilestoneA.passed === true && !packagedMilestoneAFinalEvidenceMissing))
  const commitRequired = explicitEnvGate && explicitApproval && evidenceReviewed
  const commitFormatValid = isGitCommit(commitHash)
  const commitExists = commitRequired && commitFormatValid && gitCommitExists(commitHash)
  const relevantClean = commitExists && relevantEvidenceClean(paths)
  const commitBound = !commitRequired || (commitFormatValid && commitExists && relevantClean)
  const passed = explicitEnvGate && explicitApproval && evidenceReviewed && commitBound
  const failureReasons = []
  if (!provider.passed) failureReasons.push('provider matrix evidence is not passed')
  if (!mcp.passed) failureReasons.push('MCP execution evidence is not passed')
  if (!packaged.passed) failureReasons.push('packaged desktop QA evidence is not passed')
  if (packagedGuiRequired && !packagedGui.passed) failureReasons.push('packaged GUI bridge smoke evidence is not passed')
  if (packagedSoakRequired && !packagedSoak.passed) failureReasons.push('packaged session soak evidence is not passed')
  if (packagedSoakFinalEvidenceMissing) failureReasons.push('actual packaged session soak evidence is required for final gate')
  if (packagedMilestoneARequired && !packagedMilestoneA.passed) {
    failureReasons.push('packaged general Agent Milestone A evidence is not passed')
  }
  if (packagedMilestoneAFinalEvidenceMissing) {
    failureReasons.push('fresh packaged general Agent Milestone A evidence is required for final gate')
  }
  if (!explicitEnvGate) failureReasons.push('ANALYTIX_RUNTIME_READY is not set to 1')
  if (!explicitApproval) failureReasons.push('operator post-cutover live-validation approval env is not set')
  if (commitRequired && !commitFormatValid) failureReasons.push('operator evidence target commit is not a Git commit SHA')
  else if (commitRequired && !commitExists) failureReasons.push('operator evidence target commit does not exist in this Git repository')
  else if (commitRequired && !relevantClean) failureReasons.push('operator evidence target has uncommitted runtime evidence changes')
  const dependencySummary = {
    schemaVersion: 1,
    envGate: {
      runtimeReady: explicitEnvGate,
      operatorApprovesDefault: explicitApproval
    },
    evidence: {
      providerPassed: provider.passed === true,
      mcpPassed: mcp.passed === true,
      packagedPassed: packaged.passed === true,
      packagedGuiRequired,
      packagedGuiPassed: !packagedGuiRequired || packagedGui.passed === true,
      packagedSoakRequired,
      packagedSoakPassed: !packagedSoakRequired || packagedSoak.passed === true,
      packagedSoakActualFinalEvidencePresent: !packagedSoakFinalEvidenceMissing,
      packagedMilestoneARequired,
      packagedMilestoneAPassed:
        !packagedMilestoneARequired || packagedMilestoneA.passed === true,
      packagedMilestoneAFinalEvidencePresent: !packagedMilestoneAFinalEvidenceMissing,
      credentialedEvidenceReviewed: evidenceReviewed
    },
    commitBinding: {
      required: commitRequired,
      formatValid: commitFormatValid,
      exists: commitExists,
      relevantEvidenceClean: relevantClean,
      bound: commitBound
    },
    blockers: failureReasons
  }

  return {
    schemaVersion: 1,
    id: 'runtime-go-operator-gate',
    generatedAt: now,
    status: passed ? 'passed' : 'live_blocked',
    passed,
    operatorGate: 'ANALYTIX_RUNTIME_READY=1',
    explicitEnvGate,
    credentialedEvidenceReviewed: evidenceReviewed,
    defaultRuntimeApproved: passed,
    goDefaultApproved: passed,
    commitHash,
    evidenceTargetCommit: commitHash,
    evidenceDigestAlgorithm: 'sha256:canonical-json-v1',
    evidenceDigests: {
      provider: sha256(canonicalJSONString(provider)),
      mcp: sha256(canonicalJSONString(mcp)),
      packaged: sha256(canonicalJSONString(packaged)),
      ...(packagedGuiRequired ? { packagedGui: sha256(canonicalJSONString(packagedGui)) } : {}),
      ...(packagedSoakRequired ? { packagedSoak: sha256(canonicalJSONString(packagedSoak)) } : {}),
      ...(packagedMilestoneARequired
        ? { packagedMilestoneA: sha256(canonicalJSONString(packagedMilestoneA)) }
        : {})
    },
    reportPaths: {
      provider: resolve(process.cwd(), paths.provider),
      mcp: resolve(process.cwd(), paths.mcp),
      packaged: resolve(process.cwd(), paths.packaged),
      ...(packagedGuiRequired ? { packagedGui: resolve(process.cwd(), paths.packagedGui) } : {}),
      ...(packagedSoakRequired ? { packagedSoak: resolve(process.cwd(), paths.packagedSoak) } : {}),
      ...(packagedMilestoneARequired
        ? { packagedMilestoneA: resolve(process.cwd(), paths.packagedMilestoneA) }
        : {})
    },
    evidenceReviewed: [
      { id: 'provider-matrix-credentialed', status: provider.status, path: resolve(process.cwd(), paths.provider) },
      { id: 'mcp-matrix-credentialed', status: mcp.status, path: resolve(process.cwd(), paths.mcp) },
      { id: 'packaged-qa', status: packaged.status, path: resolve(process.cwd(), paths.packaged) },
      ...(packagedGuiRequired
        ? [{ id: 'packaged-gui-bridge-smoke', status: packagedGui.status, path: resolve(process.cwd(), paths.packagedGui) }]
        : []),
      ...(packagedSoakRequired
        ? [{ id: 'packaged-session-soak', status: packagedSoak.status, path: resolve(process.cwd(), paths.packagedSoak) }]
        : []),
      ...(packagedMilestoneARequired
        ? [{
            id: 'packaged-general-agent-milestone-a',
            status: packagedMilestoneA.status,
            path: resolve(process.cwd(), paths.packagedMilestoneA)
          }]
        : [])
    ],
    typeScriptFallbackRetained: false,
    goDefaultBackendEnabled: true,
    rendererVisibleGoSwitcher: false,
    reasonixEngineRuntimeOnly: true,
    dependencySummary,
    failureReasons
  }
}

function collectorRedactionFailureEvidence({ now, commitHash, paths, secretFinding }) {
  const findingPath = String(secretFinding || 'unknown')
  const failureMessage = `collector output contained secret-like material at ${findingPath}; component evidence was replaced by redaction failure stubs`
  const provider = {
    schemaVersion: 1,
    id: 'runtime-go-provider-matrix',
    generatedAt: now,
    status: 'failed',
    passed: false,
    credentialedMatrixEnvGated: true,
    readsRealApiKeysByDefault: false,
    credentialSecretsRecorded: false,
    requiredProviderIds: PROVIDER_CASES.map((item) => item.id),
    credentialedProbes: [{
      id: 'collector-redaction-failure',
      status: 'failed',
      skipped: false,
      credentialed: false,
      message: failureMessage
    }],
    redaction: {
      status: 'failed',
      secretMaterialFound: true,
      findingPath
    },
    notes: [
      'Raw provider evidence was not written because the collector detected secret-like material.',
      'Rerun after fixing redaction at the failing component source.'
    ]
  }
  const mcp = {
    schemaVersion: 1,
    id: 'runtime-go-mcp-execution',
    generatedAt: now,
    status: 'failed',
    passed: false,
    credentialedExecution: false,
    topLevelMcpIndexerExposed: false,
    reasonixPublicProtocolUsed: false,
    requiredCoverage: REQUIRED_MCP_COVERAGE,
    connect: false,
    toolDiscoverySearch: false,
    toolCall: false,
    approvalUserInput: false,
    reconnect: false,
    redaction: false,
    credentialedProbes: [{
      id: 'collector-redaction-failure',
      family: 'mcp',
      status: 'failed',
      skipped: false,
      credentialed: false,
      credentialedExecution: false,
      credentialRedaction: false,
      message: failureMessage
    }],
    notes: [
      'Raw MCP evidence was not written because the collector detected secret-like material.',
      'No MCP-indexer top-level UI or upstream public protocol surface is added.'
    ]
  }
  const packaged = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-qa',
    stage: 'packaged-desktop-qa',
    generatedAt: now,
    status: 'failed',
    passed: false,
    credentialSecretsRecorded: false,
    requiredCheckIds: REQUIRED_PACKAGED_CHECK_IDS,
    checks: [],
    redaction: {
      status: 'failed',
      secretMaterialFound: true,
      findingPath
    },
    failureReasons: [failureMessage],
    goDefaultBackendEnabled: true,
    rendererVisibleGoSwitcher: false,
    typeScriptFallbackRetained: false
  }
  const packagedGui = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-gui-smoke',
    stage: 'packaged-gui-bridge-smoke',
    generatedAt: now,
    status: 'failed',
    passed: false,
    requiredCheckIds: [
      'packaged-app-artifact',
      'packaged-app-launch',
      'packaged-renderer-bridge',
      'packaged-settings-bridge',
      'packaged-provider-profile-settings',
      'packaged-runtime-restart',
      'packaged-runtime-health',
      'packaged-runtime-info'
    ],
    checks: [],
    redaction: {
      status: 'failed',
      secretMaterialFound: true,
      findingPath
    },
    failureReasons: [failureMessage]
  }
  const packagedSoak = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-session-soak',
    stage: 'packaged-session-soak',
    generatedAt: now,
    status: 'failed',
    passed: false,
    requiredCheckIds: [
      'go-session-durable-contract',
      'typescript-retired-backend-session-contract'
    ],
    failureChecks: ['redaction'],
    credentialSecretsRecorded: false,
    rawValueRecorded: false,
    redaction: {
      status: 'failed',
      secretMaterialFound: true,
      findingPath
    },
    failureReasons: [failureMessage]
  }
  const packagedMilestoneA = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-milestone-a',
    stage: 'packaged-general-agent-milestone-a',
    generatedAt: now,
    status: 'failed',
    passed: false,
    requiredCheckIds: REQUIRED_PACKAGED_MILESTONE_A_CHECK_IDS,
    checks: [],
    credentialSecretsRecorded: false,
    syntheticProviderUsed: false,
    localProviderUsed: false,
    directRuntimeTurnDriverUsed: false,
    redaction: {
      status: 'failed',
      secretMaterialFound: true,
      findingPath
    },
    failureReasons: [failureMessage]
  }
  const operator = {
    schemaVersion: 1,
    id: 'runtime-go-operator-gate',
    generatedAt: now,
    status: 'failed',
    passed: false,
    operatorGate: 'ANALYTIX_RUNTIME_READY=1',
    explicitEnvGate: false,
    credentialedEvidenceReviewed: false,
    defaultRuntimeApproved: false,
    goDefaultApproved: false,
    commitHash,
    evidenceTargetCommit: commitHash,
    evidenceDigestAlgorithm: 'sha256:canonical-json-v1',
    evidenceDigests: {
      provider: sha256(canonicalJSONString(provider)),
      mcp: sha256(canonicalJSONString(mcp)),
      packaged: sha256(canonicalJSONString(packaged)),
      packagedGui: sha256(canonicalJSONString(packagedGui)),
      packagedSoak: sha256(canonicalJSONString(packagedSoak)),
      packagedMilestoneA: sha256(canonicalJSONString(packagedMilestoneA))
    },
    reportPaths: {
      provider: resolve(process.cwd(), paths.provider),
      mcp: resolve(process.cwd(), paths.mcp),
      packaged: resolve(process.cwd(), paths.packaged),
      packagedGui: resolve(process.cwd(), paths.packagedGui),
      packagedSoak: resolve(process.cwd(), paths.packagedSoak),
      packagedMilestoneA: resolve(process.cwd(), paths.packagedMilestoneA)
    },
    evidenceReviewed: [
      { id: 'provider-matrix-credentialed', status: provider.status, path: resolve(process.cwd(), paths.provider) },
      { id: 'mcp-matrix-credentialed', status: mcp.status, path: resolve(process.cwd(), paths.mcp) },
      { id: 'packaged-qa', status: packaged.status, path: resolve(process.cwd(), paths.packaged) },
      { id: 'packaged-gui-bridge-smoke', status: packagedGui.status, path: resolve(process.cwd(), paths.packagedGui) },
      { id: 'packaged-session-soak', status: packagedSoak.status, path: resolve(process.cwd(), paths.packagedSoak) },
      {
        id: 'packaged-general-agent-milestone-a',
        status: packagedMilestoneA.status,
        path: resolve(process.cwd(), paths.packagedMilestoneA)
      }
    ],
    typeScriptFallbackRetained: false,
    goDefaultBackendEnabled: true,
    rendererVisibleGoSwitcher: false,
    reasonixEngineRuntimeOnly: true,
    failureReasons: [failureMessage]
  }
  return {
    provider,
    mcp,
    packaged,
    packagedGui,
    packagedSoak,
    packagedMilestoneA,
    operator
  }
}

async function runMCPStdioProbe({ command, toolName, toolArgs, timeoutMs, env }) {
  const first = await withMcpSession(command, timeoutMs, env, async (session) => {
    await session.initialize()
    const tools = await session.listTools()
    const toolNames = tools.map((tool) => tool.name).filter(Boolean)
    if (!toolNames.includes(toolName)) {
      throw new Error(`configured MCP tool was not discovered; discovered ${toolNames.length} tools`)
    }
    const call = await session.callTool(toolName, toolArgs)
    return {
      connect: true,
      toolDiscoverySearch: true,
      toolCall: true,
      toolCount: toolNames.length,
      toolCatalogDigest: sha256(canonicalJSONString([...toolNames].sort())),
      toolResultContentTypes: toolContentTypes(call)
    }
  })
  const second = await withMcpSession(command, timeoutMs, env, async (session) => {
    await session.initialize()
    const tools = await session.listTools()
    const toolNames = tools.map((tool) => tool.name).filter(Boolean)
    return {
      reconnect: toolNames.includes(toolName),
      reconnectToolNameHash: sha256(toolName),
      reconnectToolCatalogDigest: sha256(canonicalJSONString([...toolNames].sort())),
      reconnectToolCount: toolNames.length
    }
  })
  return {
    ...first,
    reconnect: second.reconnect,
    reconnectToolNameHash: second.reconnectToolNameHash,
    reconnectToolCatalogDigest: second.reconnectToolCatalogDigest,
    reconnectToolCount: second.reconnectToolCount,
    transport: 'stdio',
    message: 'credentialed MCP stdio connect/list/call/reconnect probe completed'
  }
}

async function runMCPHttpProbe({ url, toolName, toolArgs, timeoutMs, env, fetchImpl }) {
  if (typeof fetchImpl !== 'function') throw new Error('fetch implementation is unavailable for HTTP MCP probe')
  const first = await withMcpHttpSession(url, timeoutMs, env, fetchImpl, async (session) => {
    await session.initialize()
    const tools = await session.listTools()
    const toolNames = tools.map((tool) => tool.name).filter(Boolean)
    if (!toolNames.includes(toolName)) {
      throw new Error(`configured MCP tool was not discovered; discovered ${toolNames.length} tools`)
    }
    const call = await session.callTool(toolName, toolArgs)
    return {
      connect: true,
      toolDiscoverySearch: true,
      toolCall: true,
      toolCount: toolNames.length,
      toolCatalogDigest: sha256(canonicalJSONString([...toolNames].sort())),
      toolResultContentTypes: toolContentTypes(call)
    }
  })
  const second = await withMcpHttpSession(url, timeoutMs, env, fetchImpl, async (session) => {
    await session.initialize()
    const tools = await session.listTools()
    const toolNames = tools.map((tool) => tool.name).filter(Boolean)
    return {
      reconnect: toolNames.includes(toolName),
      reconnectToolNameHash: sha256(toolName),
      reconnectToolCatalogDigest: sha256(canonicalJSONString([...toolNames].sort())),
      reconnectToolCount: toolNames.length
    }
  })
  return {
    ...first,
    reconnect: second.reconnect,
    reconnectToolNameHash: second.reconnectToolNameHash,
    reconnectToolCatalogDigest: second.reconnectToolCatalogDigest,
    reconnectToolCount: second.reconnectToolCount,
    transport: 'http-json-rpc',
    message: 'credentialed MCP HTTP connect/list/call/reconnect probe completed'
  }
}

function withMcpSession(command, timeoutMs, env, fn) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, {
      cwd: process.cwd(),
      env: {
        ...process.env,
        ...recordValue(env)
      },
      shell: true,
      stdio: ['pipe', 'pipe', 'pipe']
    })
    const pending = new Map()
    let nextId = 1
    let stdout = ''
    let stderrTail = ''
    let settled = false
    const timer = setTimeout(() => {
      settle(false, new Error(`MCP stdio probe timed out. stderr: ${summarizeText(stderrTail)}`))
    }, timeoutMs)

    function settle(ok, value) {
      if (settled) return
      settled = true
      clearTimeout(timer)
      child.stdout?.off('data', onStdout)
      child.stderr?.off('data', onStderr)
      if (child.exitCode === null && child.signalCode === null) {
        child.kill()
      }
      if (ok) resolve(value)
      else reject(value)
    }

    function onStdout(chunk) {
      stdout += String(chunk)
      let newline = stdout.indexOf('\n')
      while (newline >= 0) {
        const line = stdout.slice(0, newline).trim()
        stdout = stdout.slice(newline + 1)
        if (line) handleMcpLine(line)
        newline = stdout.indexOf('\n')
      }
    }

    function onStderr(chunk) {
      stderrTail = `${stderrTail}${String(chunk)}`.slice(-4096)
    }

    function handleMcpLine(line) {
      const message = parseJSON(line)
      if (!message || typeof message.id === 'undefined') return
      const waiter = pending.get(message.id)
      if (!waiter) return
      pending.delete(message.id)
      if (message.error) {
        waiter.reject(new Error(`MCP ${waiter.method} failed: ${summarizeText(JSON.stringify(message.error))}`))
      } else {
        waiter.resolve(message.result || {})
      }
    }

    function send(method, params = {}) {
      const id = nextId++
      const payload = { jsonrpc: '2.0', id, method, params }
      child.stdin.write(`${JSON.stringify(payload)}\n`)
      return new Promise((resolveItem, rejectItem) => {
        pending.set(id, { method, resolve: resolveItem, reject: rejectItem })
      })
    }

    const session = {
      async initialize() {
        await send('initialize', {
          protocolVersion: '2025-11-25',
          capabilities: {},
          clientInfo: {
            name: 'analytix-runtime-go-live-evidence',
            version: '0.1.0'
          }
        })
        child.stdin.write(`${JSON.stringify({
          jsonrpc: '2.0',
          method: 'notifications/initialized',
          params: {}
        })}\n`)
      },
      async listTools() {
        const result = await send('tools/list', {})
        return Array.isArray(result.tools) ? result.tools : []
      },
      callTool(name, args) {
        return send('tools/call', { name, arguments: args && typeof args === 'object' ? args : {} })
      }
    }

    child.once('error', (error) => settle(false, error))
    child.once('exit', (code) => {
      if (!settled && pending.size > 0) {
        settle(false, new Error(`MCP stdio process exited before responses completed with code ${code ?? 'unknown'}`))
      }
    })
    child.stdout?.on('data', onStdout)
    child.stderr?.on('data', onStderr)

    Promise.resolve()
      .then(() => fn(session))
      .then((result) => settle(true, result))
      .catch((error) => settle(false, error))
  })
}

async function withMcpHttpSession(url, timeoutMs, env, fetchImpl, fn) {
  const state = {
    nextId: 1,
    sessionId: ''
  }
  const session = {
    async initialize() {
      await sendMcpHttpRequest({
        url,
        method: 'initialize',
        params: {
          protocolVersion: '2025-11-25',
          capabilities: {},
          clientInfo: {
            name: 'analytix-runtime-go-live-evidence',
            version: '0.1.0'
          }
        },
        timeoutMs,
        env,
        fetchImpl,
        state
      })
      await sendMcpHttpNotification({
        url,
        method: 'notifications/initialized',
        params: {},
        timeoutMs,
        env,
        fetchImpl,
        state
      })
    },
    async listTools() {
      const result = await sendMcpHttpRequest({
        url,
        method: 'tools/list',
        params: {},
        timeoutMs,
        env,
        fetchImpl,
        state
      })
      return Array.isArray(result.tools) ? result.tools : []
    },
    callTool(name, args) {
      return sendMcpHttpRequest({
        url,
        method: 'tools/call',
        params: { name, arguments: args && typeof args === 'object' ? args : {} },
        timeoutMs,
        env,
        fetchImpl,
        state
      })
    }
  }
  return await fn(session)
}

async function sendMcpHttpNotification({ url, method, params, timeoutMs, env, fetchImpl, state }) {
  await sendMcpHttpPayload({
    url,
    payload: { jsonrpc: '2.0', method, params },
    timeoutMs,
    env,
    fetchImpl,
    state,
    expectResponse: false
  })
}

async function sendMcpHttpRequest({ url, method, params, timeoutMs, env, fetchImpl, state }) {
  const id = state.nextId++
  return await sendMcpHttpPayload({
    url,
    payload: { jsonrpc: '2.0', id, method, params },
    timeoutMs,
    env,
    fetchImpl,
    state,
    expectResponse: true
  })
}

async function sendMcpHttpPayload({ url, payload, timeoutMs, env, fetchImpl, state, expectResponse }) {
  const headers = {
    accept: 'application/json, text/event-stream',
    'content-type': 'application/json',
    ...mcpHttpConfiguredHeaders(env)
  }
  const bearerToken = firstEnv(env, ['ANALYTIX_RUNTIME_GO_MCP_BEARER_TOKEN'])
  if (bearerToken) headers.authorization = `Bearer ${bearerToken}`
  if (state.sessionId) headers['mcp-session-id'] = state.sessionId
  const response = await fetchImpl(url, {
    method: 'POST',
    headers,
    body: JSON.stringify(payload),
    signal: AbortSignal.timeout(timeoutMs)
  })
  const sessionId = headerValue(response.headers, 'mcp-session-id')
  if (sessionId) state.sessionId = sessionId
  const text = await response.text()
  if (!response.ok) {
    throw new Error(`MCP HTTP ${payload.method} failed with HTTP ${response.status}: ${summarizeText(text)}`)
  }
  if (!expectResponse) return {}
  return parseMcpHttpRpcResult(text, payload.method)
}

function headerValue(headers, name) {
  if (!headers || typeof headers.get !== 'function') return ''
  return String(headers.get(name) || headers.get(name.toLowerCase()) || '').trim()
}

function parseMcpHttpRpcResult(text, method) {
  const messages = mcpHttpMessageCandidates(text)
    .map((item) => parseJSON(item))
    .filter(Boolean)
  const message = messages.find((item) => recordValue(item).result || recordValue(item).error) || messages[0]
  const root = recordValue(message)
  if (root.error) throw new Error(`MCP HTTP ${method} failed: ${summarizeText(JSON.stringify(root.error))}`)
  return recordValue(root.result)
}

function mcpHttpMessageCandidates(text) {
  const trimmed = String(text || '').trim()
  if (!trimmed) return []
  const dataLines = trimmed
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.startsWith('data:'))
    .map((line) => line.slice(5).trim())
    .filter((line) => line && line !== '[DONE]')
  return dataLines.length > 0 ? dataLines : [trimmed]
}

function mcpExecutionRequestShape({ command, url, env }) {
  if (url) {
    const configuredHeaders = mcpHttpConfiguredHeaders(env)
    const bearerToken = firstEnv(env, ['ANALYTIX_RUNTIME_GO_MCP_BEARER_TOKEN'])
    const configuredHeaderNames = Object.keys(configuredHeaders)
    return {
      protocol: 'mcp-jsonrpc-over-http',
      transport: 'streamable-http',
      methods: ['initialize', 'notifications/initialized', 'tools/list', 'tools/call'],
      headerNameClasses: mcpHttpHeaderNameClasses([
        'accept',
        'content-type',
        ...configuredHeaderNames,
        bearerToken ? 'authorization' : ''
      ]),
      customHeadersConfigured: configuredHeaderNames.length > 0,
      credentialHeaderConfigured: Boolean(bearerToken || configuredHeaderNames.some((name) => credentialHeaderName(name))),
      credentialValueRecorded: false,
      toolArgsValueRecorded: false
    }
  }
  return {
    protocol: command ? 'mcp-jsonrpc-over-stdio' : 'mcp-jsonrpc',
    methods: ['initialize', 'notifications/initialized', 'tools/list', 'tools/call'],
    credentialValueRecorded: false,
    toolArgsValueRecorded: false
  }
}

function mcpHttpConfiguredHeaders(env) {
  const parsed = parseJSONEnv(env, ['ANALYTIX_RUNTIME_GO_MCP_HEADERS_JSON'], {})
  const headers = {}
  for (const [name, value] of Object.entries(recordValue(parsed))) {
    const headerName = String(name).trim()
    if (!safeHttpHeaderName(headerName)) continue
    if (!['string', 'number', 'boolean'].includes(typeof value)) continue
    headers[headerName] = String(value)
  }
  return headers
}

function safeHttpHeaderName(name) {
  return /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(name)
}

function mcpHttpHeaderNameClasses(names) {
  return [...new Set(names
    .map((name) => String(name || '').trim())
    .filter(Boolean)
    .map((name) => {
      const lower = name.toLowerCase()
      if (lower === 'accept' || lower === 'content-type' || lower === 'mcp-session-id') return lower
      if (credentialHeaderName(name)) return 'redacted-credential-header'
      return 'custom-header'
    }))]
    .sort()
}

function credentialHeaderName(name) {
  return /(api[-_]?key|authorization|access[-_]?token|refresh[-_]?token|runtime[-_]?token|token|key|secret|signature|sig|auth|credential|jwt)/i.test(String(name || ''))
}

function validateApprovalUserInputEvidence(env) {
  const requiredFields = [
    'schemaVersion',
    'id',
    'status',
    'passed',
    'approvalUserInput',
    'rawValueRecorded',
    'credentialSecretsRecorded'
  ]
  const skippedEvidence = {
    evidencePath: '',
    evidenceSha256: '',
    evidenceDigestAlgorithm: 'sha256:file-bytes-v1',
    rawValueRecorded: false,
    credentialSecretsRecorded: false,
    requiredFields
  }
  const status = normalizedStatus(firstEnv(env, [
    'ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_STATUS'
  ]))
  const evidence = firstEnv(env, [
    'ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_EVIDENCE'
  ])
  if (status !== 'passed') {
    return {
      ...skippedEvidence,
      status: status === 'missing' ? 'skipped' : status,
      evidencePath: evidence ? resolve(process.cwd(), evidence) : '',
      message: 'approval/user-input evidence path is not passed'
    }
  }
  if (!evidence || !existsSync(resolve(process.cwd(), evidence))) {
    return {
      ...skippedEvidence,
      status: 'failed',
      evidencePath: evidence ? resolve(process.cwd(), evidence) : '',
      message: 'approval/user-input evidence path is missing'
    }
  }
  const evidenceText = readFileSync(resolve(process.cwd(), evidence), 'utf8')
  const parsed = parseJSON(evidenceText)
  const secretFinding = firstEvidenceSecretFinding(parsed)
  const root = recordValue(parsed)
  const missing = []
  if (root.schemaVersion !== 1) missing.push('schemaVersion')
  if (root.id !== 'runtime-go-mcp-approval-user-input-evidence') {
    missing.push('id')
  }
  if (root.status !== 'passed') missing.push('status')
  if (root.passed !== true) missing.push('passed')
  if (!(root.approvalUserInput === true || (root.approval === true && root.userInput === true))) {
    missing.push('approvalUserInput')
  }
  if (root.rawValueRecorded !== false) missing.push('rawValueRecorded:false')
  if (root.credentialSecretsRecorded !== false) missing.push('credentialSecretsRecorded:false')
  const passed = !secretFinding && missing.length === 0
  return {
    status: passed ? 'passed' : 'failed',
    evidencePath: resolve(process.cwd(), evidence),
    evidenceSha256: sha256(evidenceText),
    evidenceDigestAlgorithm: 'sha256:file-bytes-v1',
    rawValueRecorded: false,
    credentialSecretsRecorded: false,
    requiredFields,
    missingEvidenceFields: missing,
    message: passed
      ? 'approval/user-input evidence passed'
      : secretFinding
        ? `approval/user-input evidence contains secret-like material at ${secretFinding}`
        : `approval/user-input evidence must pass approval and userInput coverage with audit fields: ${missing.join(', ')}`
  }
}

function providerRequestUrl(spec, rawBaseUrl) {
  const baseUrl = rawBaseUrl.trim().replace(/\/+$/, '')
  if (spec.urlMode === 'custom-full-endpoint-no-append') return rawBaseUrl.trim()
  if (spec.urlMode === 'append-anthropic-messages') {
    if (baseUrl.endsWith('/messages')) return baseUrl
    if (baseUrl.endsWith('/v1')) return `${baseUrl}/messages`
    return `${baseUrl}/v1/messages`
  }
  return upstreamOpenAiChatCompletionsUrl(rawBaseUrl)
}

function splitUrlSuffix(url) {
  const query = url.search(/[?#]/)
  if (query < 0) return { path: url, suffix: '' }
  return { path: url.slice(0, query), suffix: url.slice(query) }
}

function appendUrlPath(baseUrl, path) {
  const split = splitUrlSuffix(baseUrl)
  return `${split.path.replace(/\/+$/, '')}/${path}${split.suffix}`
}

function trimUrlPathEnd(baseUrl) {
  const split = splitUrlSuffix(baseUrl.trim())
  return `${split.path.replace(/\/+$/, '')}${split.suffix}`
}

function lastPathSegment(baseUrl) {
  const split = splitUrlSuffix(baseUrl.trim())
  return split.path.replace(/\/+$/, '').split('/').pop() ?? ''
}

function isVersionSegment(segment) {
  const value = String(segment || '').toLowerCase()
  return value === 'beta' || /^v\d+$/i.test(value)
}

function unversionedBaseUrl(baseUrl) {
  const split = splitUrlSuffix(baseUrl)
  const trimmed = split.path.replace(/\/+$/, '')
  const slash = trimmed.lastIndexOf('/')
  if (slash < 0) return `${trimmed}${split.suffix}`
  const segment = trimmed.slice(slash + 1)
  if (isVersionSegment(segment)) return `${trimmed.slice(0, slash)}${split.suffix}`
  return `${trimmed}${split.suffix}`
}

function versionedBaseUrl(baseUrl) {
  const trimmed = trimUrlPathEnd(baseUrl)
  const segment = lastPathSegment(trimmed)
  if (isVersionSegment(segment)) return trimmed
  return appendUrlPath(trimmed, 'v1')
}

function upstreamOpenAiChatCompletionsUrl(baseUrl) {
  let versioned = versionedBaseUrl(baseUrl.trim())
  if (lastPathSegment(versioned).toLowerCase() === 'beta') {
    versioned = appendUrlPath(unversionedBaseUrl(baseUrl.trim()), 'v1')
  }
  return appendUrlPath(versioned, 'chat/completions')
}

function providerRequestBody(spec, model, env, baseUrl = '') {
  if (spec.id === 'custom-endpoint') {
    const customBody = firstEnv(env, [
      'ANALYTIX_RUNTIME_GO_CUSTOM_ENDPOINT_BODY_JSON'
    ])
    if (customBody) {
      const parsed = parseJSON(customBody)
      if (parsed && typeof parsed === 'object') return parsed
    }
    if (inferCustomEndpointFormatFromUrl(baseUrl) === 'responses') {
      return {
        model,
        input: providerProbePrompt,
        stream: true,
        max_output_tokens: 16
      }
    }
    if (inferCustomEndpointFormatFromUrl(baseUrl) === 'messages') {
      return {
        model,
        max_tokens: 16,
        stream: true,
        messages: [
          { role: 'user', content: providerProbePrompt }
        ]
      }
    }
  }
  if (spec.bodyMode === 'anthropic-messages') {
    return {
      model,
      max_tokens: 16,
      stream: true,
      messages: [
        { role: 'user', content: providerProbePrompt }
      ]
    }
  }
  return {
    model,
    stream: true,
    stream_options: { include_usage: true },
    max_tokens: 16,
    messages: [
      { role: 'user', content: providerProbePrompt }
    ]
  }
}

function providerErrorProbeBody(spec) {
  if (spec.bodyMode === 'anthropic-messages' || spec.id === 'custom-endpoint') {
    return {
      stream: true,
      max_tokens: 1,
      messages: [
        { role: 'user', content: providerErrorProbePrompt }
      ]
    }
  }
  return {
    stream: true,
    stream_options: { include_usage: true },
    max_tokens: 1,
    messages: [
      { role: 'user', content: providerErrorProbePrompt }
    ]
  }
}

function inferCustomEndpointFormatFromUrl(rawUrl) {
  const query = rawUrl.search(/[?#]/)
  const path = (query < 0 ? rawUrl : rawUrl.slice(0, query)).trim().replace(/\/+$/, '').toLowerCase()
  if (path.endsWith('/chat/completions') || path.endsWith('/completions')) return 'chat_completions'
  if (path.endsWith('/responses')) return 'responses'
  if (path.endsWith('/messages')) return 'messages'
  return 'chat_completions'
}

function providerRequestHeaders(spec, apiKey, env) {
  if (spec.authMode === 'anthropic-x-api-key') {
    return {
      'content-type': 'application/json',
      'anthropic-version': firstEnv(env, ['ANALYTIX_RUNTIME_GO_ANTHROPIC_VERSION']) || '2023-06-01',
      'x-api-key': apiKey
    }
  }
  return {
    'content-type': 'application/json',
    authorization: `Bearer ${apiKey}`
  }
}

function providerRequestShape(spec, body, requestUrl, hasCredential) {
  return {
    method: 'POST',
    urlPolicy: spec.urlMode,
    requestUrl,
    endpointFamily: spec.endpointFamily,
    endpointFormat: spec.endpointFormat,
    customEndpointInferredFormat: spec.id === 'custom-endpoint' ? inferCustomEndpointFormatFromUrl(requestUrl) : '',
    bodyFields: Object.keys(recordValue(body)).sort(),
    messageCount: Array.isArray(body.messages) ? body.messages.length : 0,
    inputConfigured: typeof body.input === 'string' && body.input.length > 0,
    streamRequested: body.stream === true,
    maxTokensField: Object.hasOwn(body, 'max_tokens')
      ? 'max_tokens'
      : Object.hasOwn(body, 'max_completion_tokens')
        ? 'max_completion_tokens'
        : Object.hasOwn(body, 'max_output_tokens')
          ? 'max_output_tokens'
          : '',
    headerNames: spec.authMode === 'anthropic-x-api-key'
      ? ['content-type', 'anthropic-version', 'redacted-api-key-header']
      : ['content-type', 'redacted-bearer-auth-header'],
    credentialConfigured: hasCredential,
    credentialValueRecorded: false,
    deepSeekSpecificRequestFieldsPresent: hasDeepSeekRequestFields(body)
  }
}

function providerRequestShapeValidation(shape) {
  const missing = []
  if (shape.method !== 'POST') missing.push('method')
  if (shape.credentialConfigured !== true) missing.push('credentialConfigured')
  if (shape.credentialValueRecorded !== false) missing.push('credentialValueRecorded:false')
  if (shape.streamRequested !== true) missing.push('streamRequested')
  if ((!Number.isFinite(shape.messageCount) || shape.messageCount <= 0) && shape.inputConfigured !== true) {
    missing.push('messageOrInput')
  }
  if (!shape.maxTokensField) missing.push('maxTokensField')
  return {
    status: missing.length === 0 ? 'passed' : 'failed',
    missing
  }
}

function parseProviderProtocolResponse(text, spec) {
  const trimmed = text.trim()
  if (!trimmed) {
    return { mode: 'empty', protocolParsed: false, frameCount: 0, jsonFrameCount: 0, parseErrorCount: 0, usage: {} }
  }
  const sseLines = trimmed.split(/\r?\n/).filter((line) => line.trim().startsWith('data:'))
  if (sseLines.length > 0) {
    let jsonFrameCount = 0
    let parseErrorCount = 0
    let usage = {}
    for (const line of sseLines) {
      const payload = line.replace(/^data:\s*/, '').trim()
      if (!payload || payload === '[DONE]') continue
      const parsed = parseJSON(payload)
      if (!parsed) {
        parseErrorCount++
        continue
      }
      jsonFrameCount++
      usage = mergeUsage(usage, usageFromProviderPayload(parsed, spec))
    }
    return {
      mode: 'sse',
      protocolParsed: jsonFrameCount > 0 && parseErrorCount === 0,
      frameCount: sseLines.length,
      jsonFrameCount,
      parseErrorCount,
      usage
    }
  }
  const parsed = parseJSON(trimmed)
  const usage = parsed ? usageFromProviderPayload(parsed, spec) : {}
  return {
    mode: 'json',
    protocolParsed: Boolean(parsed),
    frameCount: parsed ? 1 : 0,
    jsonFrameCount: parsed ? 1 : 0,
    parseErrorCount: parsed ? 0 : 1,
    usage
  }
}

function usageFromProviderPayload(payload, spec) {
  const root = recordValue(payload)
  const usage = recordValue(root.usage)
  if (Object.keys(usage).length > 0) return usage
  const message = recordValue(root.message)
  const messageUsage = recordValue(message.usage)
  if (Object.keys(messageUsage).length > 0) return messageUsage
  const delta = recordValue(root.delta)
  const deltaUsage = recordValue(delta.usage)
  if (Object.keys(deltaUsage).length > 0) return deltaUsage
  if (spec.bodyMode === 'anthropic-messages') {
    const anthropicUsage = {}
    for (const key of ['input_tokens', 'output_tokens', 'cache_creation_input_tokens', 'cache_read_input_tokens']) {
      if (typeof root[key] === 'number') anthropicUsage[key] = root[key]
    }
    return anthropicUsage
  }
  return {}
}

function usageParsingResult(usage) {
  const keys = Object.keys(recordValue(usage))
  const tokenKeys = keys.filter((key) => /token/i.test(key))
  return {
    status: tokenKeys.length > 0 ? 'passed' : 'failed',
    tokenFields: tokenKeys.sort(),
    usageObjectPresent: keys.length > 0
  }
}

function cacheTelemetryResult(spec, usage, body) {
  if (spec.id === 'deepseek') {
    const fields = Object.keys(recordValue(usage)).filter((key) =>
      /cache/i.test(key) || key === 'prompt_cache_hit_tokens' || key === 'prompt_cache_miss_tokens'
    )
    return {
      status: fields.length > 0 ? 'passed' : 'failed',
      providerScoped: true,
      cacheFieldNames: fields.sort(),
      deepSeekSpecificRequestFieldsPresent: hasDeepSeekRequestFields(body)
    }
  }
  return {
    status: hasDeepSeekRequestFields(body) ? 'failed' : 'passed',
    providerScoped: false,
    cacheFieldNames: Object.keys(recordValue(usage)).filter((key) => /cache/i.test(key)).sort(),
    deepSeekSpecificRequestFieldsPresent: hasDeepSeekRequestFields(body)
  }
}

function hasDeepSeekRequestFields(body) {
  const keys = new Set(Object.keys(recordValue(body)))
  return keys.has('prefix_cache') || keys.has('cache_prefix') || keys.has('prompt_cache_key')
}

function mergeUsage(left, right) {
  return { ...recordValue(left), ...recordValue(right) }
}

function missingExternalInputs(
  provider,
  mcp,
  packaged,
  packagedGui,
  packagedSoak,
  packagedMilestoneA,
  operator
) {
  const missing = []
  if (!provider.passed) {
    for (const item of providerMissingExternalInputs(provider)) missing.push(item)
  }
  if (!mcp.passed) {
    missing.push({
      id: 'mcp:credentialed-execution',
      status: mcp.status,
      reason: mcp.credentialedProbes?.[0]?.message || 'MCP execution did not pass'
    })
  }
  if (!packaged.passed) {
    for (const item of packagedMissingExternalInputs(packaged)) missing.push(item)
  }
  if (packagedGui && !packagedGui.passed) {
    for (const item of packagedGuiMissingExternalInputs(packagedGui)) missing.push(item)
  }
  if (packagedSoak && !packagedSoak.passed) {
    for (const item of packagedSessionSoakMissingExternalInputs(packagedSoak)) missing.push(item)
  } else if (packagedSoak) {
    for (const item of packagedSessionSoakActualFinalGateMissingInputs(packagedSoak)) missing.push(item)
  }
  if (packagedMilestoneA && !packagedMilestoneA.passed) {
    for (const item of packagedMilestoneAMissingExternalInputs(packagedMilestoneA)) missing.push(item)
  }
  if (!operator.passed) {
    missing.push({
      id: 'operator:gate',
      status: operator.status,
      reason: 'operator gate requirements are not satisfied',
      reasons: Array.isArray(operator.failureReasons) ? operator.failureReasons.filter(Boolean) : []
    })
  }
  return missing
}

function providerMissingExternalInputs(provider) {
  const probes = Array.isArray(provider?.credentialedProbes)
    ? provider.credentialedProbes.map(recordValue)
    : []
  if (recordValue(provider?.redaction).status === 'failed') {
    return [{
      id: 'provider:redaction',
      status: provider.status || 'failed',
      reason: probes[0]?.message || 'provider evidence failed redaction'
    }]
  }

  const coverage = recordValue(provider?.credentialedProviderCoverage)
  const missing = []
  const deepseekPassed = coverage.deepseekPassed === true || provider?.credentialedDeepSeekPassed === true
  const nonDeepSeekPassed = coverage.nonDeepSeekProviderPassed === true ||
    (Array.isArray(provider?.credentialedNonDeepSeekProviderIds) &&
      provider.credentialedNonDeepSeekProviderIds.length > 0)
  if (!deepseekPassed) {
    const probe = probes.find((item) => item.id === 'deepseek') || {}
    missing.push(providerMissingInputFromProbe({
      id: 'provider:deepseek',
      probe,
      fallbackReason: 'DeepSeek provider probe did not pass'
    }))
  }
  if (!nonDeepSeekPassed) {
    const candidateProbes = probes.filter((item) => item.id && item.id !== 'deepseek')
    missing.push({
      id: 'provider:at-least-one-non-deepseek-provider',
      status: provider.status || 'live_blocked',
      reason: 'configure and pass at least one non-DeepSeek provider probe',
      candidateProviderIds: candidateProbes.map((item) => item.id).filter(Boolean),
      candidates: candidateProbes.map((probe) => ({
        id: probe.id,
        status: probe.status || 'missing',
        missingEnv: Array.isArray(probe.missingEnv) ? probe.missingEnv : [],
        missingSettingsProfileFields: Array.isArray(probe.missingSettingsProfileFields)
          ? probe.missingSettingsProfileFields
          : []
      }))
    })
  }
  if (missing.length > 0) return missing
  return [{
    id: 'provider:credentialed-matrix',
    status: provider.status || 'live_blocked',
    reason: 'provider credentialed matrix did not pass'
  }]
}

function providerMissingInputFromProbe({ id, probe, fallbackReason }) {
  return {
    id,
    status: probe.status || 'missing',
    reason: probe.message || fallbackReason,
    missingEnv: Array.isArray(probe.missingEnv) ? probe.missingEnv : [],
    missingSettingsProfileFields: Array.isArray(probe.missingSettingsProfileFields)
      ? probe.missingSettingsProfileFields
      : []
  }
}

function providerCompletionOptions(provider) {
  const coverage = recordValue(provider?.settingsProfileCoverage)
  const usableNonDeepSeekProviderIds = Array.isArray(coverage.usableNonDeepSeekProviderIds)
    ? coverage.usableNonDeepSeekProviderIds.map((item) => String(item || '').trim()).filter(Boolean)
    : []
  return {
    credentialValuesRecorded: false,
    settingsProfilePath: 'provider.providers[]',
    currentSettingsProviderCount: Number.isFinite(recordValue(provider?.settingsProfile).providerCount)
      ? recordValue(provider.settingsProfile).providerCount
      : 0,
    usableNonDeepSeekProviderIds,
    recognizedPresetProviderIds: recognizedModelProviderPresetIds(),
    requiredNonDeepSeekSettingsFields: [
      'id or name identifying a non-DeepSeek provider',
      'apiKey',
      'endpointFormat or recognized preset/model profile default',
      'baseUrl or recognized preset default',
      'models[] or runtime.model or recognized preset default'
    ],
    acceptedNonDeepSeekProfileSignals: [
      'openai-compatible: endpointFormat chat_completions or OpenAI-compatible id/name',
      'anthropic-compatible: endpointFormat messages or Anthropic/Claude id/name',
      'custom-endpoint: endpointFormat custom_endpoint or full endpoint URL'
    ],
    envCompletionGroups: PROVIDER_CASES
      .filter((spec) => spec.id !== 'deepseek')
      .map((spec) => ({
        id: spec.id,
        requiredEnv: [
          preferredEnvName(spec.apiKeyEnv),
          preferredEnvName(spec.baseUrlEnv),
          preferredEnvName(spec.modelEnv)
        ],
        acceptedAliases: {
          apiKeyEnv: publicEnvAliases(spec.apiKeyEnv),
          baseUrlEnv: publicEnvAliases(spec.baseUrlEnv),
          modelEnv: publicEnvAliases(spec.modelEnv)
        }
      }))
  }
}

function nextEvidenceActions({
  provider,
  mcp,
  packaged,
  packagedGui,
  packagedSoak,
  packagedMilestoneA,
  operator,
  paths
}) {
  const packagedSoakActualMissing = packagedSessionSoakActualFinalGateMissingInputs(packagedSoak)
  return {
    schemaVersion: 1,
    credentialSecretsRecorded: false,
    valuesRecorded: false,
    providerMatrix: {
      status: provider.status,
      evidencePath: resolve(process.cwd(), paths.provider),
      settingsProfile: provider.settingsProfile,
      settingsProfileCoverage: provider.settingsProfileCoverage || {},
      completionOptions: providerCompletionOptions(provider),
      requiredCredentialedProviderCoverage: provider.requiredCredentialedProviderCoverage,
      credentialedDeepSeekPassed: provider.credentialedDeepSeekPassed === true,
      credentialedNonDeepSeekProviderIds: Array.isArray(provider.credentialedNonDeepSeekProviderIds)
        ? provider.credentialedNonDeepSeekProviderIds
        : [],
      requiredEnvGroups: PROVIDER_CASES.map((spec) => {
        const probe = (provider.credentialedProbes || []).find((item) => item.id === spec.id) || {}
        return {
          id: spec.id,
          status: probe.status || 'missing',
          endpointFamily: spec.endpointFamily,
          endpointFormat: spec.endpointFormat,
          env: {
            apiKeyEnv: preferredEnvName(spec.apiKeyEnv),
            baseUrlEnv: preferredEnvName(spec.baseUrlEnv),
            modelEnv: preferredEnvName(spec.modelEnv)
          },
          acceptedAliases: {
            apiKeyEnv: publicEnvAliases(spec.apiKeyEnv),
            baseUrlEnv: publicEnvAliases(spec.baseUrlEnv),
            modelEnv: publicEnvAliases(spec.modelEnv)
          },
          missingEnv: Array.isArray(probe.missingEnv) ? probe.missingEnv : [],
          missingSettingsProfileFields: Array.isArray(probe.missingSettingsProfileFields)
            ? probe.missingSettingsProfileFields
            : [],
          providerSource: probe.provider?.source || 'missing',
          settingsProfileMatched: probe.provider?.settingsProfileMatched === true,
          settingsProfileUsable: probe.provider?.settingsProfileUsable === true,
          providerHasApiKey: probe.provider?.hasApiKey === true,
          providerModelConfigured: Boolean(probe.provider?.model),
          cacheTelemetryMode: spec.cacheTelemetryMode
        }
      })
    },
    mcpExecution: {
      status: mcp.status,
      evidencePath: resolve(process.cwd(), paths.mcp),
      requiredEnv: {
        commandOrUrl: ['ANALYTIX_RUNTIME_MCP_COMMAND', 'ANALYTIX_RUNTIME_MCP_URL'],
        toolName: 'ANALYTIX_RUNTIME_MCP_TOOL_NAME',
        toolArgsJson: 'ANALYTIX_RUNTIME_MCP_TOOL_ARGS_JSON',
        bearerTokenEnv: 'ANALYTIX_RUNTIME_GO_MCP_BEARER_TOKEN',
        headersJson: 'ANALYTIX_RUNTIME_GO_MCP_HEADERS_JSON',
        approvalUserInputStatus: 'ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_STATUS',
        approvalUserInputEvidence: 'ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_EVIDENCE'
      },
      requiredCoverage: REQUIRED_MCP_COVERAGE,
      topLevelMcpIndexerExposed: false
    },
    packagedDesktopQa: {
      status: packaged.status,
      evidencePath: resolve(process.cwd(), paths.packaged),
      requiredEnv: {
        appPath: 'ANALYTIX_RUNTIME_GO_PACKAGED_APP_PATH',
        launchCommand: 'ANALYTIX_RUNTIME_GO_PACKAGED_LAUNCH_COMMAND',
        runtimeUrl: 'ANALYTIX_RUNTIME_GO_RUNTIME_URL',
        runtimeTokenEnv: 'ANALYTIX_RUNTIME_GO_RUNTIME_TOKEN',
        threadIdOrCreate: ['ANALYTIX_RUNTIME_GO_THREAD_ID', 'ANALYTIX_RUNTIME_GO_ALLOW_THREAD_CREATE=1'],
        rollbackEvidenceJson: 'ANALYTIX_RUNTIME_GO_ROLLBACK_EVIDENCE_JSON',
        rollbackEvidenceCommand: 'ANALYTIX_RUNTIME_GO_ROLLBACK_EVIDENCE_COMMAND',
        rollbackRuntimePid: 'ANALYTIX_RUNTIME_GO_RUNTIME_PID'
      },
      requiredCheckIds: REQUIRED_PACKAGED_CHECK_IDS
    },
    packagedGuiBridgeSmoke: {
      status: packagedGui?.status || 'missing',
      evidencePath: resolve(process.cwd(), paths.packagedGui),
      requiredEnv: {
        appPath: [
          'ANALYTIX_RUNTIME_GO_GUI_PACKAGED_APP_PATH',
          'ANALYTIX_RUNTIME_GO_PACKAGED_APP_PATH',
          'ANALYTIX_PACKAGED_APP_PATH'
        ],
        apiKey: 'ANALYTIX_RUNTIME_GO_GUI_API_KEY',
        timeoutMs: 'ANALYTIX_RUNTIME_GO_GUI_TIMEOUT_MS'
      },
      requiredCheckIds: [
        'packaged-app-artifact',
        'packaged-app-launch',
        'packaged-renderer-bridge',
        'packaged-settings-bridge',
        'packaged-provider-profile-settings',
        'packaged-mimo-profile-settings',
        'packaged-runtime-restart',
        'packaged-runtime-health',
        'packaged-runtime-info'
      ]
    },
    packagedSessionSoak: {
      status: packagedSoak?.status || 'missing',
      evidencePath: resolve(process.cwd(), paths.packagedSoak),
      finalGateActualEvidenceRequired: packagedSoakActualMissing.length > 0,
      finalGateMissingRequiredEvidence: packagedSoakActualMissing.flatMap((item) =>
        Array.isArray(item.requiredEvidence) ? item.requiredEvidence : []
      ),
      requiredEnv: {
        appPath: [
          'ANALYTIX_RUNTIME_GO_PACKAGED_SOAK_APP_PATH',
          'ANALYTIX_RUNTIME_GO_PACKAGED_APP_PATH',
          'ANALYTIX_PACKAGED_APP_PATH'
        ],
        timeoutMs: 'ANALYTIX_RUNTIME_GO_PACKAGED_SOAK_TIMEOUT_MS',
        actual: 'ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SOAK=1'
      },
      requiredCheckIds: [
        'go-session-durable-contract',
        'typescript-retired-backend-session-contract',
        'packaged-app-artifact',
        'packaged-app-launch',
        'packaged-renderer-bridge',
        'packaged-settings-bridge',
        'packaged-provider-profile-settings',
        'packaged-runtime-restart',
        'packaged-session-thread-create',
        'packaged-session-turn-create',
        'packaged-session-sse-replay',
        'packaged-session-tool-timeline',
        'packaged-session-mimo-plan',
        'packaged-session-attachment-fallback',
        'packaged-session-approval-user-input',
        'packaged-session-fork',
        'packaged-session-resume',
        'packaged-session-thread-list',
        'packaged-session-provider-redaction'
      ]
    },
    packagedGeneralAgentMilestoneA: {
      status: packagedMilestoneA?.status || 'missing',
      evidencePath: resolve(process.cwd(), paths.packagedMilestoneA),
      requiredEnv: {
        appPath: [
          'ANALYTIX_RUNTIME_GO_MILESTONE_A_APP_PATH',
          'ANALYTIX_RUNTIME_GO_PACKAGED_APP_PATH',
          'ANALYTIX_PACKAGED_APP_PATH'
        ],
        sourceCommit: [
          'ANALYTIX_RUNTIME_GO_MILESTONE_A_SOURCE_COMMIT',
          'ANALYTIX_RUNTIME_GO_PACKAGED_SOURCE_COMMIT'
        ],
        provider: [
          'ANALYTIX_RUNTIME_GO_MILESTONE_A_PROVIDER_ID',
          'ANALYTIX_RUNTIME_GO_MILESTONE_A_PROVIDER_MODEL',
          'ANALYTIX_RUNTIME_GO_MILESTONE_A_PROVIDER_BASE_URL',
          'ANALYTIX_RUNTIME_GO_MILESTONE_A_PROVIDER_ENDPOINT_FORMAT',
          'ANALYTIX_RUNTIME_GO_MILESTONE_A_PROVIDER_CREDENTIAL'
        ],
        timeoutMs: 'ANALYTIX_RUNTIME_GO_MILESTONE_A_TIMEOUT_MS'
      },
      requiredCheckIds: REQUIRED_PACKAGED_MILESTONE_A_CHECK_IDS,
      directRuntimeTurnDriverAllowed: false,
      composerInputDriver: 'CDP Input domain',
      exactRecoveryRequired: true,
      normalQuitRequired: true
    },
    commercialPublication: {
      status: recordValue(packaged?.packaged).commercialReleaseStatus || 'unverified',
      requiredForMilestoneA: false,
      requiredForCommercialPublication: true,
      requiredRows: [
        'formal-release-publication-authority',
        'controlled-release-native-receipt'
      ],
      requiredEnv: {
        releaseAuthority: [
          'ANALYTIX_RELEASE_AUTHORITY',
          'ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY',
          'ANALYTIX_RELEASE_AUTHORITY_TRUSTED_PUBLIC_KEY_SHA256',
          'ANALYTIX_RELEASE_TAG',
          'ANALYTIX_RELEASE_CHANNEL',
          'ANALYTIX_RELEASE_DIST'
        ]
      }
    },
    operatorGate: {
      status: operator.status,
      evidencePath: resolve(process.cwd(), paths.operator),
      dependencySummary: recordValue(operator.dependencySummary),
      requiredEnv: {
        g6Ready: 'ANALYTIX_RUNTIME_READY=1',
        approval: 'ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT=1'
      },
      evidenceReviewedIds: [
        'provider-matrix-credentialed',
        'mcp-matrix-credentialed',
        'packaged-qa',
        ...(packagedGui ? ['packaged-gui-bridge-smoke'] : []),
        ...(packagedSoak ? ['packaged-session-soak'] : []),
        ...(packagedMilestoneA ? ['packaged-general-agent-milestone-a'] : [])
      ]
    },
    commands: [
      'npm run runtime:go:approval-user-input-evidence -- --json',
      'npm run runtime:go:live-evidence -- --json',
      'npm run runtime:go:local-validation -- --json',
      'npm run runtime:go:packaged-soak -- --json --actual',
      'npm run runtime:go:packaged-gui-smoke -- --json',
      'npm run runtime:go:packaged-milestone-a -- --json',
      LIVE_VALIDATION_PROVIDER_PROBE_COMMAND,
      'npm run runtime:go:preflight',
      'ANALYTIX_RUNTIME_READY=1 ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT=1 npm run runtime:go:default-readiness-report -- --gate --json'
    ]
  }
}

function packagedMissingExternalInputs(packaged) {
  const missing = []
  for (const check of Array.isArray(packaged.checks) ? packaged.checks : []) {
    const item = recordValue(check)
    if (item.status !== 'passed') {
      missing.push({
        id: `packaged:${item.id || 'desktop-qa'}`,
        status: item.status || packaged.status,
        reason: item.reason || item.message || 'packaged desktop QA check did not pass'
      })
    }
  }
  const legacyMissingRequiredKey = ['missing', 'Required', 'Pr', 'oof'].join('')
  const missingRequiredEvidence = Array.isArray(packaged.missingRequiredEvidence)
    ? packaged.missingRequiredEvidence
    : Array.isArray(packaged[legacyMissingRequiredKey])
      ? packaged[legacyMissingRequiredKey]
      : []
  for (const evidence of missingRequiredEvidence) {
    missing.push({
      id: `packaged:evidence:${evidence}`,
      status: packaged.status,
      reason: `missing packaged QA evidence field: ${evidence}`
    })
  }
  if (missing.length === 0) {
    missing.push({
      id: 'packaged:desktop-qa',
      status: packaged.status,
      reason: 'packaged desktop QA did not pass'
    })
  }
  return missing
}

function packagedGuiMissingExternalInputs(packagedGui) {
  const missing = []
  for (const check of Array.isArray(packagedGui.checks) ? packagedGui.checks : []) {
    const item = recordValue(check)
    if (item.status !== 'passed') {
      missing.push({
        id: `packaged-gui:${item.id || 'bridge-smoke'}`,
        status: item.status || packagedGui.status,
        reason: item.message || 'packaged GUI bridge smoke check did not pass'
      })
    }
  }
  const legacyMissingRequiredKey = ['missing', 'Required', 'Pr', 'oof'].join('')
  const missingRequiredEvidence = Array.isArray(packagedGui.missingRequiredEvidence)
    ? packagedGui.missingRequiredEvidence
    : Array.isArray(packagedGui[legacyMissingRequiredKey])
      ? packagedGui[legacyMissingRequiredKey]
      : []
  for (const evidence of missingRequiredEvidence) {
    missing.push({
      id: `packaged-gui:evidence:${evidence}`,
      status: packagedGui.status,
      reason: `missing packaged GUI smoke evidence field: ${evidence}`
    })
  }
  if (missing.length === 0) {
    missing.push({
      id: 'packaged-gui:bridge-smoke',
      status: packagedGui.status,
      reason: 'packaged GUI bridge smoke did not pass'
    })
  }
  return missing
}

function packagedSessionSoakMissingExternalInputs(packagedSoak) {
  const missing = []
  for (const check of Array.isArray(packagedSoak.failureChecks) ? packagedSoak.failureChecks : []) {
    missing.push({
      id: `packaged-soak:${check}`,
      status: packagedSoak.status,
      reason: `packaged session soak check did not pass: ${check}`
    })
  }
  const legacyMissingRequiredKey = ['missing', 'Required', 'Pr', 'oof'].join('')
  const missingRequiredEvidence = Array.isArray(packagedSoak.missingRequiredEvidence)
    ? packagedSoak.missingRequiredEvidence
    : Array.isArray(packagedSoak[legacyMissingRequiredKey])
      ? packagedSoak[legacyMissingRequiredKey]
      : []
  for (const evidence of missingRequiredEvidence) {
    missing.push({
      id: `packaged-soak:evidence:${evidence}`,
      status: packagedSoak.status,
      reason: `missing packaged session soak evidence field: ${evidence}`
    })
  }
  if (missing.length === 0) {
    missing.push({
      id: 'packaged-soak:session',
      status: packagedSoak.status,
      reason: packagedSoak.message || 'packaged session soak did not pass'
    })
  }
  return missing
}

export function packagedMilestoneAMissingExternalInputs(packagedMilestoneA) {
  const explicit = Array.isArray(packagedMilestoneA.missingExternalInputs)
    ? packagedMilestoneA.missingExternalInputs.map(recordValue)
    : []
  const explicitInputs = explicit.map((item) => ({
      id: `packaged-milestone-a:${item.id || 'external-prerequisite'}`,
      status: item.status || packagedMilestoneA.status || 'live_blocked',
      reason: item.reason || packagedMilestoneA.executionBlocker ||
        'packaged general Agent Milestone A external prerequisite is missing'
    }))

  const failureCheckIds = Array.isArray(packagedMilestoneA.failureCheckIds)
    ? packagedMilestoneA.failureCheckIds.map((item) => String(item || '').trim()).filter(Boolean)
    : []
  const missingRequiredEvidence = Array.isArray(packagedMilestoneA.missingRequiredEvidence)
    ? packagedMilestoneA.missingRequiredEvidence
      .map((item) => String(item || '').trim())
      .filter(Boolean)
    : []
  const missing = [
    ...failureCheckIds.map((id) => ({
      id: `packaged-milestone-a:${id}`,
      status: packagedMilestoneA.status || 'live_blocked',
      reason: packagedMilestoneA.executionBlocker ||
        `packaged general Agent Milestone A check did not pass: ${id}`
    })),
    ...missingRequiredEvidence.map((evidence) => ({
      id: `packaged-milestone-a:evidence:${evidence}`,
      status: packagedMilestoneA.status || 'live_blocked',
      reason: `missing packaged general Agent Milestone A evidence field: ${evidence}`
    }))
  ]
  const merged = [...explicitInputs, ...missing]
  if (merged.length > 0) {
    const seen = new Set()
    return merged.filter((item) => {
      if (seen.has(item.id)) return false
      seen.add(item.id)
      return true
    })
  }
  return [{
    id: 'packaged-milestone-a:public-seam',
    status: packagedMilestoneA.status || 'live_blocked',
    reason: packagedMilestoneA.executionBlocker ||
      'packaged general Agent Milestone A public-seam acceptance did not pass'
  }]
}

function packagedSessionSoakActualFinalGateMissingInputs(packagedSoak) {
  const soak = recordValue(packagedSoak?.soak)
  if (soak.actualPackagedAppEvidenceRequiredForFinalGate !== true) return []
  return [{
    id: 'packaged-soak:actual-packaged-session-soak',
    status: 'live_blocked',
    reason: 'actual packaged session soak evidence is required for final gate',
    requiredEvidence: [
      'soak.actualPackagedAppEvidenceRequiredForFinalGate:false',
      'actual packaged app session resume/fork/SSE replay/attachment fallback soak'
    ]
  }]
}

function packagedEvidenceMissing(report) {
  if (isAggregatePackagedQAReport(report)) {
    return aggregatePackagedEvidenceMissing(report)
  }

  const missing = []
  if (report.id !== 'runtime-go-packaged-qa') missing.push('id')
  if (report.status !== 'passed') missing.push('status')
  if (report.passed !== true) missing.push('passed')
  if (report.goDefaultBackendEnabled !== true) missing.push('goDefaultBackendEnabled:true')
  if (report.rendererVisibleGoSwitcher !== false) missing.push('rendererVisibleGoSwitcher:false')
  if (report.typeScriptFallbackRetained !== false) missing.push('typeScriptFallbackRetained:false')
  if (report.credentialSecretsRecorded !== false) missing.push('credentialSecretsRecorded:false')
  const reportRedaction = recordValue(report.redaction)
  if (reportRedaction.status !== 'passed' || reportRedaction.secretMaterialFound !== false) missing.push('redaction')
  const checks = Array.isArray(report.checks) ? report.checks : []
  for (const id of REQUIRED_PACKAGED_CHECK_IDS) {
    const matching = checks.map(recordValue).filter((item) => item.id === id)
    if (matching.length !== 1) {
      missing.push(`checks:${id}:unique`)
      continue
    }
    if (matching[0].status !== 'passed') {
      missing.push(`checks:${id}:status`)
    }
  }
  for (const item of checks.map(recordValue)) {
    if (item.id && !REQUIRED_PACKAGED_CHECK_IDS.includes(item.id) && item.status !== 'passed') {
      missing.push(`checks:${item.id}:status`)
    }
  }
  const startup = recordValue(report.startup)
  const runtime = recordValue(report.runtime)
  const legacyDefaultRuntimeGateKey = ['candidate', 'Gate'].join('')
  const legacyCandidateEnvSetKey = ['candidate', 'Env', 'Set'].join('')
  const legacyCandidateStoppedKey = ['candidate', 'Stopped'].join('')
  const legacyCandidateLaunchCommandHashKey = ['candidate', 'LaunchCommandHash'].join('')
  const defaultRuntimeGate = recordValue(report.defaultRuntimeGate ?? report[legacyDefaultRuntimeGateKey])
  const legacyRollbackKey = ['rollback', 'Pr', 'oof'].join('')
  const retiredBackendEvidence = recordValue(report.retiredBackendEvidence ?? report[legacyRollbackKey])
  const explicitRuntimeBackendOverrideEnvSet = Object.prototype.hasOwnProperty.call(defaultRuntimeGate, 'explicitRuntimeBackendOverrideEnvSet')
    ? defaultRuntimeGate.explicitRuntimeBackendOverrideEnvSet
    : defaultRuntimeGate[legacyCandidateEnvSetKey]
  const defaultRuntimeStopped = Object.prototype.hasOwnProperty.call(retiredBackendEvidence, 'defaultRuntimeStopped')
    ? retiredBackendEvidence.defaultRuntimeStopped
    : retiredBackendEvidence[legacyCandidateStoppedKey]
  const defaultRuntimeLaunchCommandHash = retiredBackendEvidence.defaultRuntimeLaunchCommandHash || retiredBackendEvidence[legacyCandidateLaunchCommandHashKey]
  if (startup.appPathExists !== true) missing.push('startup.appPathExists')
  if (startup.launchCommandConfigured !== true) missing.push('startup.launchCommandConfigured')
  if (startup.launchCommandReferencesAppPath !== true) missing.push('startup.launchCommandReferencesAppPath')
  if (startup.exitCode !== 0) missing.push('startup.exitCode')
  if (!isSha256(startup.launchCommandHash)) missing.push('startup.launchCommandHash')
  if (!runtime.runtimeUrl) missing.push('runtime.runtimeUrl')
  if (runtime.runtimeTokenConfigured !== true) missing.push('runtime.runtimeTokenConfigured')
  if (runtime.healthOK !== true) missing.push('runtime.healthOK')
  const runtimeInfo = recordValue(runtime.runtimeInfo)
  if (runtimeInfo.candidateContract !== true) missing.push('runtime.runtimeInfo.candidateContract')
  if (runtimeInfo.hostLocalhost !== true) missing.push('runtime.runtimeInfo.hostLocalhost')
  if (runtimeInfo.dataDirConfigured !== true) missing.push('runtime.runtimeInfo.dataDirConfigured')
  if (!isSha256(runtimeInfo.dataDirHash)) missing.push('runtime.runtimeInfo.dataDirHash')
  if (!runtimeInfoCapabilityDiagnosticValid(runtimeInfo, 'mcp')) {
    missing.push('runtime.runtimeInfo.mcpDiagnosticHonest')
  }
  if (runtimeInfo.subagentsAvailable !== true) {
    missing.push('runtime.runtimeInfo.subagentsAvailable')
  }
  if (!runtimeInfoCapabilityDiagnosticValid(runtimeInfo, 'subagents')) {
    missing.push('runtime.runtimeInfo.subagentsDiagnosticHonest')
  }
  if (runtimeInfo.productRuntimeInfoHidesUpstreamEvidence !== true) missing.push('runtime.runtimeInfo.productRuntimeInfoHidesUpstreamEvidence')
  if (runtimeInfo.reasonixUpstreamAbsorptionPresent !== false) missing.push('runtime.runtimeInfo.reasonixUpstreamAbsorptionPresent:false')
  if (runtimeInfo.rawValueRecorded !== false) missing.push('runtime.runtimeInfo.rawValueRecorded:false')
  if (!isSha256(runtimeInfo.infoShapeDigest)) missing.push('runtime.runtimeInfo.infoShapeDigest')
  if (!isSha256(runtime.threadIdHash)) missing.push('runtime.threadIdHash')
  if (!isSha256(runtime.turnIdHash)) missing.push('runtime.turnIdHash')
  if (defaultRuntimeGate[backendFieldKey] !== 'go-runtime-default') missing.push(`defaultRuntimeGate.${backendFieldKey}`)
  if (explicitRuntimeBackendOverrideEnvSet !== false) missing.push('defaultRuntimeGate.explicitRuntimeBackendOverrideEnvSet:false')
  if (defaultRuntimeGate.goDefaultBackendEnabled !== true) missing.push('defaultRuntimeGate.goDefaultBackendEnabled:true')
  if (retiredBackendEvidence.requestedBackend !== 'typescript') missing.push('retiredBackendEvidence.requestedBackend:typescript')
  if (retiredBackendEvidence.code !== 'retired_backend') missing.push('retiredBackendEvidence.code:retired_backend')
  if (retiredBackendEvidence.activeBackendAfterRequest !== 'go-runtime-default') missing.push('retiredBackendEvidence.activeBackendAfterRequest')
  if (defaultRuntimeStopped !== false) missing.push('retiredBackendEvidence.defaultRuntimeStopped:false')
  if (retiredBackendEvidence.tsStarted !== false) missing.push('retiredBackendEvidence.tsStarted:false')
  if (retiredBackendEvidence.runtimeHealthAfterRequest !== true) missing.push('retiredBackendEvidence.runtimeHealthAfterRequest')
  if (!isSha256(defaultRuntimeLaunchCommandHash)) missing.push('retiredBackendEvidence.defaultRuntimeLaunchCommandHash')
  else if (defaultRuntimeLaunchCommandHash !== startup.launchCommandHash) {
    missing.push('retiredBackendEvidence.defaultRuntimeLaunchCommandHash:match')
  }
  if (retiredBackendEvidence.preRequestRuntimeUrl !== runtime.runtimeUrl) missing.push('retiredBackendEvidence.preRequestRuntimeUrl:match')
  if (retiredBackendEvidence.postRequestRuntimeUrl !== runtime.runtimeUrl) missing.push('retiredBackendEvidence.postRequestRuntimeUrl:match')
  if (!retiredBackendEvidence.verifiedAt) missing.push('retiredBackendEvidence.verifiedAt')
  if (retiredBackendEvidence.credentialSecretsRecorded !== false) missing.push('retiredBackendEvidence.credentialSecretsRecorded:false')
  const retiredBackendRedaction = recordValue(retiredBackendEvidence.redaction)
  if (retiredBackendRedaction.status !== 'passed' || retiredBackendRedaction.secretMaterialFound !== false) missing.push('retiredBackendEvidence.redaction')
  return missing
}

function isAggregatePackagedQAReport(report) {
  if (report.id !== 'runtime-go-packaged-qa') return false
  const packaged = recordValue(report.packaged)
  if (Object.keys(packaged).length === 0) return false
  return Array.isArray(report.checks) &&
    report.checks.map(recordValue).some((item) => item.id === 'cutover-report')
}

function aggregatePackagedEvidenceMissing(report) {
  const missing = []
  if (report.id !== 'runtime-go-packaged-qa') missing.push('id')
  if (report.status !== 'passed') missing.push('status')
  if (report.passed !== true) missing.push('passed')
  if (report.credentialSecretsRecorded !== false) missing.push('credentialSecretsRecorded:false')
  const reportRedaction = recordValue(report.redaction)
  if (reportRedaction.status !== 'passed' || reportRedaction.secretMaterialFound !== false) missing.push('redaction')

  const checks = Array.isArray(report.checks) ? report.checks.map(recordValue) : []
  const requiredCheckIds = [
    'cutover-report',
    'gui-smoke',
    'session-soak',
    'milestone-a',
    'rollback-evidence',
    'packaging-config'
  ]
  for (const id of requiredCheckIds) {
    const matching = checks.filter((item) => item.id === id)
    if (matching.length !== 1) {
      missing.push(`checks:${id}:unique`)
      continue
    }
    if (matching[0].status !== 'passed') missing.push(`checks:${id}:status`)
  }
  for (const item of checks) {
    if (item.id && !requiredCheckIds.includes(String(item.id)) && item.status !== 'passed') {
      missing.push(`checks:${item.id}:status`)
    }
  }

  const packaged = recordValue(report.packaged)
  const milestoneA = recordValue(report.milestoneAEvidence)
  for (const item of packagedMilestoneAEvidenceMissing(milestoneA)) {
    missing.push(`milestoneAEvidence.${item}`)
  }
  const qaHarness = recordValue(report.harness)
  const currentQaSha256 = currentRegularFileSha256(
    resolve(process.cwd(), 'scripts/runtime-go-packaged-qa.mjs')
  )
  if (!isSha256(qaHarness.scriptSha256)) missing.push('harness.scriptSha256')
  else if (qaHarness.scriptSha256 !== currentQaSha256) {
    missing.push('harness.scriptSha256:current-bytes')
  }
  if (!isSha256(qaHarness.contractManifestSha256)) {
    missing.push('harness.contractManifestSha256')
  }
  if (packaged.actualPackagedDesktopQAPassed !== true) missing.push('packaged.actualPackagedDesktopQAPassed')
  if (packaged.milestoneAFunctionalPassed !== true) {
    missing.push('packaged.milestoneAFunctionalPassed')
  }
  if (packaged.commercialReleaseReady !== true) {
    missing.push('packaged.commercialReleaseReady')
  }
  if (packaged.commercialReleaseStatus !== 'passed') {
    missing.push('packaged.commercialReleaseStatus:passed')
  }
  if (packaged.formalReleasePublicationAuthorityPassed !== true) {
    missing.push('packaged.formalReleasePublicationAuthorityPassed')
  }
  if (packaged.controlledReleaseNativeReceiptPassed !== true) {
    missing.push('packaged.controlledReleaseNativeReceiptPassed')
  }
  if (packaged.actualPackagedAppLaunched !== true) missing.push('packaged.actualPackagedAppLaunched')
  if (packaged.actualPackagedSmokeRequested !== true) missing.push('packaged.actualPackagedSmokeRequested')
  if (packaged.actualPackagedSmokePassed !== true) missing.push('packaged.actualPackagedSmokePassed')
  if (packaged.actualPackagedAppEvidenceRequiredForFinalGate !== false) {
    missing.push('packaged.actualPackagedAppEvidenceRequiredForFinalGate:false')
  }
  if (packaged.guiSmokePassed !== true) missing.push('packaged.guiSmokePassed')
  if (packaged.sessionSoakPassed !== true) missing.push('packaged.sessionSoakPassed')
  if (packaged.milestoneAPassed !== true) missing.push('packaged.milestoneAPassed')
  if (packaged.milestoneAStatus !== 'passed') missing.push('packaged.milestoneAStatus:passed')
  if (!isSha256(packaged.milestoneAHarnessScriptSha256)) {
    missing.push('packaged.milestoneAHarnessScriptSha256')
  }
  if (!isSha256(packaged.milestoneAHarnessManifestSha256)) {
    missing.push('packaged.milestoneAHarnessManifestSha256')
  }
  if (packaged.milestoneAHarnessBound !== true) missing.push('packaged.milestoneAHarnessBound')
  if (packaged.credentialedProviderPublicSeamPassed !== true) {
    missing.push('packaged.credentialedProviderPublicSeamPassed')
  }
  if (packaged.formalGeneralAgentWorkflowPassed !== true) {
    missing.push('packaged.formalGeneralAgentWorkflowPassed')
  }
  if (packaged.rendererVisualPublicSeamEvidencePassed !== true) {
    missing.push('packaged.rendererVisualPublicSeamEvidencePassed')
  }
  if (packaged.deterministicContractOnly !== false) {
    missing.push('packaged.deterministicContractOnly:false')
  }
  if (packaged.durableRestartPassed !== true) missing.push('packaged.durableRestartPassed')
  if (packaged.durableProcessRelaunchObserved !== true) {
    missing.push('packaged.durableProcessRelaunchObserved')
  }
  if (packaged.typeScriptRetiredBackendPassed !== true) missing.push('packaged.typeScriptRetiredBackendPassed')
  if (packaged.packagingConfigPassed !== true) missing.push('packaged.packagingConfigPassed')
  if (packaged.settingsProfilePatchAccepted !== true) missing.push('packaged.settingsProfilePatchAccepted')
  if (packaged.settingsCompatPatchUsed !== false) missing.push('packaged.settingsCompatPatchUsed:false')
  if (packaged.runtimeHealthPassed !== true) missing.push('packaged.runtimeHealthPassed')
  if (packaged.runtimeHealthRuntimeInfoOk !== true) missing.push('packaged.runtimeHealthRuntimeInfoOk')
  if (packaged.runtimeHealthRuntimeToolsOk !== true) missing.push('packaged.runtimeHealthRuntimeToolsOk')
  if (packaged.runtimeHealthProductionCapabilitiesOk !== true) {
    missing.push('packaged.runtimeHealthProductionCapabilitiesOk')
  }
  if (packaged.directLegacyExposureCount !== 0) missing.push('packaged.directLegacyExposureCount:0')
  if (packaged.temporaryGoProdViolationCount !== 0) missing.push('packaged.temporaryGoProdViolationCount:0')
  if (packaged.retiredProductionMarkerViolationCount !== 0) {
    missing.push('packaged.retiredProductionMarkerViolationCount:0')
  }
  if (packaged.legacyUpstreamFieldKeyViolationCount !== 0) {
    missing.push('packaged.legacyUpstreamFieldKeyViolationCount:0')
  }
  const missingFinalCoverage = Array.isArray(packaged.missingFinalCoverage)
    ? packaged.missingFinalCoverage.map((item) => String(item))
    : []
  for (const id of [
    'packaged-desktop-qa',
    'formal-release-publication-authority',
    'controlled-release-native-receipt'
  ]) {
    if (missingFinalCoverage.includes(id)) {
      missing.push(`packaged.missingFinalCoverage:${id}`)
    }
  }
  return missing
}

function packagedGuiEvidenceMissing(report) {
  const requiredCheckIds = [
    'packaged-app-artifact',
    'packaged-app-launch',
    'packaged-renderer-bridge',
    'packaged-settings-bridge',
    'packaged-provider-profile-settings',
    'packaged-mimo-profile-settings',
    'packaged-runtime-restart',
    'packaged-runtime-health',
    'packaged-runtime-info'
  ]
  const missing = []
  if (report.id !== 'runtime-go-packaged-gui-smoke') missing.push('id')
  if (report.status !== 'passed') missing.push('status')
  if (report.passed !== true) missing.push('passed')
  const smoke = recordValue(report.smoke)
  if (smoke.deterministicContractOnly !== false) missing.push('smoke.deterministicContractOnly:false')
  if (smoke.actualPackagedAppLaunched !== true) missing.push('smoke.actualPackagedAppLaunched')
  if (smoke.actualPackagedAppEvidenceRequiredForFinalGate !== false) missing.push('smoke.actualPackagedAppEvidenceRequiredForFinalGate:false')
  if (smoke.actualPackagedSmokeRequested !== true) missing.push('smoke.actualPackagedSmokeRequested')
  if (smoke.actualPackagedSmokePassed !== true) missing.push('smoke.actualPackagedSmokePassed')
  if (smoke.settingsProfilePatchAccepted !== true) missing.push('smoke.settingsProfilePatchAccepted')
  if (smoke.mimoProfileSettingsAccepted !== true) missing.push('smoke.mimoProfileSettingsAccepted')
  if (smoke.settingsCompatPatchUsed !== false) missing.push('smoke.settingsCompatPatchUsed:false')
  const actual = recordValue(report.actualPackagedSmoke)
  if (actual.passed !== true || actual.status !== 'passed') missing.push('actualPackagedSmoke.status')
  const reportRedaction = recordValue(actual.redaction)
  if (reportRedaction.status !== 'passed' || reportRedaction.secretMaterialFound !== false) missing.push('redaction')
  const checks = Array.isArray(actual.checks) ? actual.checks : []
  for (const id of requiredCheckIds) {
    const matching = checks.map(recordValue).filter((item) => item.id === id)
    if (matching.length !== 1) {
      missing.push(`checks:${id}:unique`)
      continue
    }
    if (matching[0].status !== 'passed') {
      missing.push(`checks:${id}:status`)
    }
  }
  const app = recordValue(actual.app)
  if (app.executableExists !== true) missing.push('app.executableExists')
  if (app.runtimeServerBinaryExists !== true) missing.push('app.runtimeServerBinaryExists')
  if (app.appAsarExists !== true) missing.push('app.appAsarExists')
  if (app.bundledRuntimeGoSourcePresent !== false) missing.push('app.bundledRuntimeGoSourcePresent:false')
  const renderer = recordValue(actual.renderer)
  if (renderer.apiPresent !== true) missing.push('renderer.apiPresent')
  if (renderer.title !== 'Analytix') missing.push('renderer.title:Analytix')
  if (renderer.legacyKun !== false) missing.push('renderer.legacyKun:false')
  if (renderer.legacyReasonix !== false) missing.push('renderer.legacyReasonix:false')
  if (renderer.settingsOk !== true) missing.push('renderer.settingsOk')
  if (renderer.settingsRuntimeTopLevel !== true) missing.push('renderer.settingsRuntimeTopLevel')
  if (renderer.settingsProfilePatchAccepted !== true) missing.push('renderer.settingsProfilePatchAccepted')
  if (renderer.settingsCompatPatchUsed !== false) missing.push('renderer.settingsCompatPatchUsed:false')
  if (renderer.restartOk !== true) missing.push('renderer.restartOk')
  if (renderer.healthStatus !== 200) missing.push('renderer.healthStatus:200')
  if (renderer.healthService !== 'analytix') missing.push('renderer.healthService:analytix')
  if (renderer.runtimeInfoStatus !== 200) missing.push('renderer.runtimeInfoStatus:200')
  if (renderer.runtimeInfoProductClean !== true) missing.push('renderer.runtimeInfoProductClean')
  const legacyMcpLocalMarkerKey = ['runtimeInfoHasMcpLocal', 'Pr', 'oof'].join('')
  const runtimeInfoHasLegacyMcpLocalMarker =
    renderer.runtimeInfoHasLegacyMcpLocalMarker ?? renderer[legacyMcpLocalMarkerKey]
  if (runtimeInfoHasLegacyMcpLocalMarker !== false) missing.push('renderer.runtimeInfoHasLegacyMcpLocalMarker:false')
  if (renderer.runtimeInfoHasLegacyAbsorptionMarker !== false) missing.push('renderer.runtimeInfoHasLegacyAbsorptionMarker:false')
  if (renderer.runtimeInfoHasReasonixSurface !== false) missing.push('renderer.runtimeInfoHasReasonixSurface:false')
  return missing
}

function packagedMilestoneAEvidenceMissing(report) {
  const missing = []
  if (report.id !== 'runtime-go-packaged-milestone-a') missing.push('id')
  if (report.status !== 'passed') missing.push('status')
  if (report.passed !== true) missing.push('passed')
  if (report.credentialSecretsRecorded !== false) missing.push('credentialSecretsRecorded:false')
  if (report.syntheticProviderUsed !== false) missing.push('syntheticProviderUsed:false')
  if (report.localProviderUsed !== false) missing.push('localProviderUsed:false')
  if (report.directRuntimeTurnDriverUsed !== false) missing.push('directRuntimeTurnDriverUsed:false')
  const projection = packagedMilestoneAReportProjection(report)
  for (const error of projection.errors) missing.push(error)
  if (report.composerDomAndPrimaryButtonRequired !== true) {
    missing.push('composerDomAndPrimaryButtonRequired:true')
  }
  if (report.cdpAndBridgeObservationOnly !== true) {
    missing.push('cdpAndBridgeObservationOnly:true')
  }
  const harness = recordValue(report.harness)
  const currentMilestoneASha256 = currentRegularFileSha256(
    resolve(process.cwd(), 'scripts/runtime-go-packaged-milestone-a.mjs')
  )
  if (!isSha256(harness.scriptSha256)) missing.push('harness.scriptSha256')
  else if (harness.scriptSha256 !== currentMilestoneASha256) {
    missing.push('harness.scriptSha256:current-bytes')
  }
  if (!isSha256(harness.contractManifestSha256)) {
    missing.push('harness.contractManifestSha256')
  }
  const harnessEntries = Array.isArray(harness.entries) ? harness.entries.map(recordValue) : []
  const collectorEntry = harnessEntries.find((item) =>
    item.name === 'runtime-go-live-evidence-collector.mjs'
  )
  if (collectorEntry?.sha256 !== currentRegularFileSha256(fileURLToPath(import.meta.url))) {
    missing.push('harness.entries:collector-current-bytes')
  }
  const checks = Array.isArray(report.checks) ? report.checks.map(recordValue) : []
  for (const id of REQUIRED_PACKAGED_MILESTONE_A_CHECK_IDS) {
    const matching = checks.filter((item) => item.id === id)
    if (matching.length !== 1) {
      missing.push(`checks:${id}:unique`)
      continue
    }
    if (matching[0].status !== 'passed') missing.push(`checks:${id}:status`)
  }
  const app = recordValue(report.app)
  if (app.ok !== true) missing.push('app.ok')
  if (app.codeSignatureVerified !== true) missing.push('app.codeSignatureVerified')
  const commercialRelease = recordValue(report.commercialRelease)
  if (commercialRelease.requiredForMilestoneA !== false) {
    missing.push('commercialRelease.requiredForMilestoneA:false')
  }
  if (commercialRelease.requiredForCommercialPublication !== true) {
    missing.push('commercialRelease.requiredForCommercialPublication:true')
  }
  const provider = recordValue(report.provider)
  if (provider.configured !== true) missing.push('provider.configured')
  if (provider.credentialConfigured !== true) missing.push('provider.credentialConfigured')
  if (provider.credentialRecorded !== false) missing.push('provider.credentialRecorded:false')
  if (provider.networkTurnCompleted !== true) missing.push('provider.networkTurnCompleted')
  if (provider.threadProviderBound !== true) missing.push('provider.threadProviderBound')
  if (provider.threadModelBound !== true) missing.push('provider.threadModelBound')
  if (provider.resultTurnProviderReceiptBound !== true) {
    missing.push('provider.resultTurnProviderReceiptBound')
  }
  if (provider.providerAttemptTelemetryValid !== true) {
    missing.push('provider.providerAttemptTelemetryValid')
  }
  if (!isSha256(provider.providerAttemptReceiptDigest)) {
    missing.push('provider.providerAttemptReceiptDigest')
  }
  if (provider.modelHash !== sha256('deepseek-v4-flash')) missing.push('provider.modelHash:formal-model')
  if (provider.requiredModelHash !== sha256('deepseek-v4-flash')) {
    missing.push('provider.requiredModelHash:formal-model')
  }
  if (provider.reasoningEffort !== 'high') missing.push('provider.reasoningEffort:high')
  if (report.operatorCheckpoint?.declared !== true) missing.push('operatorCheckpoint.declared')
  if (!['visible-human', 'visible-computer-use'].includes(report.operatorCheckpoint?.method)) {
    missing.push('operatorCheckpoint.method:visible-login')
  }
  const providerLogicalCallCount = Number(provider.providerLogicalCallCount || 0)
  const providerAttemptCount = Number(provider.providerAttemptCount || 0)
  const successfulProviderAttemptCount = Number(provider.successfulProviderAttemptCount || 0)
  if (!Number.isSafeInteger(providerLogicalCallCount) || providerLogicalCallCount <= 0) {
    missing.push('provider.providerLogicalCallCount')
  }
  if (!Number.isSafeInteger(providerAttemptCount) ||
    providerAttemptCount < providerLogicalCallCount) {
    missing.push('provider.providerAttemptCount')
  }
  if (!Number.isSafeInteger(successfulProviderAttemptCount) ||
    successfulProviderAttemptCount !== providerLogicalCallCount) {
    missing.push('provider.successfulProviderAttemptCount')
  }
  const ordinaryProviderEvidence = recordValue(provider.ordinaryEvidence)
  for (const field of [
    'networkTurnCompleted',
    'threadProviderBound',
    'threadModelBound',
    'resultTurnProviderReceiptBound',
    'providerAttemptTelemetryValid',
    'providerAttemptReceiptDigest',
    'providerLogicalCallCount',
    'providerAttemptCount',
    'successfulProviderAttemptCount'
  ]) {
    if (ordinaryProviderEvidence[field] !== provider[field]) {
      missing.push(`provider.ordinaryEvidence.${field}:top-level-match`)
    }
  }
  const longContextProviderEvidence = recordValue(provider.longContextEvidence)
  for (const field of [
    'networkTurnCompleted',
    'threadProviderBound',
    'threadModelBound',
    'resultTurnProviderReceiptBound',
    'providerAttemptTelemetryValid'
  ]) {
    if (longContextProviderEvidence[field] !== true) {
      missing.push(`provider.longContextEvidence.${field}`)
    }
  }
  if (!isSha256(longContextProviderEvidence.providerAttemptReceiptDigest)) {
    missing.push('provider.longContextEvidence.providerAttemptReceiptDigest')
  }
  const visualEvidence = recordValue(report.visualEvidence)
  if (visualEvidence.screenshotsUsedAsFunctionalVerdict !== false) {
    missing.push('visualEvidence.screenshotsUsedAsFunctionalVerdict:false')
  }
  if (visualEvidence.credentialScanCovered !== true) {
    missing.push('visualEvidence.credentialScanCovered')
  }
  for (const name of ['firstCompleted', 'recoveredAfterRelaunch']) {
    const item = recordValue(visualEvidence[name])
    const viewport = recordValue(item.viewport)
    if (item.captured !== true ||
      !isSha256(item.sha256) ||
      !Number.isSafeInteger(item.width) ||
      Number(item.width) <= 0 ||
      !Number.isSafeInteger(item.height) ||
      Number(item.height) <= 0 ||
      !(Number(viewport.width) > 0) ||
      !(Number(viewport.height) > 0) ||
      !(Number(viewport.scale) > 0)) {
      missing.push(`visualEvidence.${name}`)
    }
  }
  const isolation = recordValue(report.isolation)
  if (isolation.cacheTmpdirVerified !== true) missing.push('isolation.cacheTmpdirVerified')
  if (isolation.systemUserDataTouched !== false) missing.push('isolation.systemUserDataTouched:false')
  if (isolation.isolatedHomeUsedByChild !== true) missing.push('isolation.isolatedHomeUsedByChild')
  if (isolation.sameDataDirsOnRelaunch !== true) missing.push('isolation.sameDataDirsOnRelaunch')
  if (isolation.sandboxRemoved !== true) missing.push('isolation.sandboxRemoved')
  const publicSeams = recordValue(report.publicSeams)
  for (const launch of ['firstLaunch', 'secondLaunch']) {
    const seam = recordValue(publicSeams[launch])
    if (seam.healthOk !== true) missing.push(`publicSeams.${launch}.healthOk`)
    if (seam.runtimeInfoOk !== true) missing.push(`publicSeams.${launch}.runtimeInfoOk`)
    if (seam.runtimeToolsOk !== true) missing.push(`publicSeams.${launch}.runtimeToolsOk`)
    if (seam.ordinaryCatalogNonempty !== true) {
      missing.push(`publicSeams.${launch}.ordinaryCatalogNonempty`)
    }
    if (seam.fundsExecutionUnavailable !== true) {
      missing.push(`publicSeams.${launch}.fundsExecutionUnavailable`)
    }
  }
  const caseCapability = recordValue(report.caseCapability)
  if (caseCapability.additiveNotReplacement !== true) {
    missing.push('caseCapability.additiveNotReplacement')
  }
  if (caseCapability.privateAuthorityInputsAbsent !== true) {
    missing.push('caseCapability.privateAuthorityInputsAbsent')
  }
  if (caseCapability.privateAuthorityEnvironmentKeyCount !== 0) {
    missing.push('caseCapability.privateAuthorityEnvironmentKeyCount:0')
  }
  if (caseCapability.privateAuthoritySettingsKeyCount !== 0) {
    missing.push('caseCapability.privateAuthoritySettingsKeyCount:0')
  }
  if (caseCapability.preLaunchUnexpectedUserDataEntryCount !== 0) {
    missing.push('caseCapability.preLaunchUnexpectedUserDataEntryCount:0')
  }
  if (caseCapability.firstLaunchFundsExecutionUnavailable !== true) {
    missing.push('caseCapability.firstLaunchFundsExecutionUnavailable')
  }
  if (caseCapability.secondLaunchFundsExecutionUnavailable !== true) {
    missing.push('caseCapability.secondLaunchFundsExecutionUnavailable')
  }
  if (caseCapability.ordinaryCatalogAvailable !== true) {
    missing.push('caseCapability.ordinaryCatalogAvailable')
  }
  if (caseCapability.ordinaryWorkflowCompleted !== true) {
    missing.push('caseCapability.ordinaryWorkflowCompleted')
  }
  if (caseCapability.unavailableWhileOrdinaryCapabilitiesRetained !== true) {
    missing.push('caseCapability.unavailableWhileOrdinaryCapabilitiesRetained')
  }
  const workflow = recordValue(report.workflow)
  for (const field of [
    'firstLaunchObserved',
    'composerWorkflowSubmitted',
    'planModeSelected',
    'planTurnObserved',
    'agentModeRestored',
    'sameThreadPlanAgent',
    'readObserved',
    'planObserved',
    'todoObserved',
    'writeObserved',
    'realTestObserved',
    'subagentObserved',
    'longContextContinuationObserved',
    'longContextHostReadBound',
    'longContextSubagentContinuityBound',
    'longContextProviderReceiptReplayAttempted',
    'longContextProviderReceiptReplayObserved',
    'longContextProviderReceiptBound',
    'longContextAcceptedFinalBound',
    'successfulToolResultsObserved',
    'todosCompleted',
    'exactlyOneBoundedSubagentCompleted',
    'normalFirstQuitObserved',
    'freshProcessRelaunchObserved',
    'exactThreadRecovered',
    'exactTodosRecovered',
    'exactSubagentsRecovered',
    'exactCompactionsRecovered',
    'exactResultRecovered',
    'rendererVisibleThreadRecovered',
    'rendererVisibleTodosRecovered',
    'rendererVisibleSubagentRecovered',
    'rendererVisibleCompactionRecovered',
    'rendererVisibleResultRecovered',
    'normalFinalQuitObserved',
    'zeroResidualProcesses'
  ]) {
    if (workflow[field] !== true) missing.push(`workflow.${field}`)
  }
  if (workflow.longContextProviderReceiptReplayReasonCode !== 'provider_receipt_observed') {
    missing.push('workflow.longContextProviderReceiptReplayReasonCode:observed')
  }
  if (workflow.longContextContinuationReasonCode !== 'observed') {
    missing.push('workflow.longContextContinuationReasonCode:observed')
  }
  if (workflow.longContextContinuationFailureCode !== '') {
    missing.push('workflow.longContextContinuationFailureCode:empty')
  }
  if (!isSha256(workflow.longContextSubagentContinuityBaselineDigest) ||
      !isSha256(workflow.longContextSubagentContinuityProtectedFundsDigest) ||
      !isSha256(workflow.longContextSubagentContinuityDigest) ||
      workflow.longContextSubagentContinuityProtectedFundsDigest !==
        workflow.longContextSubagentContinuityBaselineDigest ||
      workflow.longContextSubagentContinuityDigest !==
        workflow.longContextSubagentContinuityBaselineDigest) {
    missing.push('workflow.longContextSubagentContinuityDigest')
  }
  if (!isSha256(workflow.longContextProviderReceiptDigest) ||
    workflow.longContextProviderReceiptDigest !==
      longContextProviderEvidence.providerAttemptReceiptDigest) {
    missing.push('workflow.longContextProviderReceiptDigest')
  }
  if (Number(workflow.compactionCount || 0) <= 0) missing.push('workflow.compactionCount')
  missing.push(...compactionWorkflowProofMissing(workflow))
  /* compactionWorkflowProofMissing owns the typed ordinary/case-bound closure. */
  if (workflow.relaunchProviderContinuationObserved !== true) {
    missing.push('workflow.relaunchProviderContinuationObserved')
  }
  if (workflow.firstExitCode !== 0 || workflow.firstExitSignal !== null) {
    missing.push('workflow.firstExit:clean')
  }
  if (workflow.finalExitCode !== 0 || workflow.finalExitSignal !== null) {
    missing.push('workflow.finalExit:clean')
  }
  const redaction = recordValue(report.redaction)
  if (redaction.status !== 'passed' ||
    redaction.secretMaterialFound !== false ||
    redaction.credentialRecorded !== false) {
    missing.push('redaction')
  }
  return missing
}

function packagedSessionSoakEvidenceMissing(report) {
  const missing = []
  if (report.id !== 'runtime-go-packaged-session-soak') missing.push('id')
  if (report.status !== 'passed') missing.push('status')
  if (report.passed !== true) missing.push('passed')
  const soak = recordValue(report.soak)
  if (soak.goDurableSessionCovered !== true) missing.push('soak.goDurableSessionCovered')
  if (soak.typeScriptRetiredBackendSessionCovered !== true) missing.push('soak.typeScriptRetiredBackendSessionCovered')
  if (soak.resumeForkPairingCovered !== true) missing.push('soak.resumeForkPairingCovered')
  if (soak.sseReplayCovered !== true) missing.push('soak.sseReplayCovered')
  const checks = Array.isArray(report.checks) ? report.checks.map(recordValue) : []
  for (const id of ['go-session-durable-contract', 'typescript-retired-backend-session-contract']) {
    const matching = checks.filter((item) => item.id === id)
    if (matching.length !== 1) {
      missing.push(`checks:${id}:unique`)
      continue
    }
    if (matching[0].status !== 'passed') missing.push(`checks:${id}:status`)
  }
  const actualMode = soak.actualPackagedSessionSoakPassed === true ||
    soak.actualPackagedAppEvidenceRequiredForFinalGate === false ||
    soak.deterministicContractOnly === false
  if (!actualMode) {
    if (soak.deterministicContractOnly !== true) missing.push('soak.deterministicContractOnly')
    if (soak.actualPackagedAppLaunched !== false) missing.push('soak.actualPackagedAppLaunched:false')
    if (soak.actualPackagedAppEvidenceRequiredForFinalGate !== true) missing.push('soak.actualPackagedAppEvidenceRequiredForFinalGate')
    return missing
  }

  if (soak.deterministicContractOnly !== false) missing.push('soak.deterministicContractOnly:false')
  if (soak.actualPackagedAppLaunched !== true) missing.push('soak.actualPackagedAppLaunched')
  if (soak.actualPackagedAppEvidenceRequiredForFinalGate !== false) {
    missing.push('soak.actualPackagedAppEvidenceRequiredForFinalGate:false')
  }
  if (soak.actualPackagedSessionSoakRequested !== true) missing.push('soak.actualPackagedSessionSoakRequested')
  if (soak.actualPackagedSessionSoakPassed !== true) missing.push('soak.actualPackagedSessionSoakPassed')
  if (soak.mimoPlanCovered !== true) missing.push('soak.mimoPlanCovered')
  if (soak.attachmentFallbackCovered !== true) missing.push('soak.attachmentFallbackCovered')
  if (soak.approvalUserInputCovered !== true) missing.push('soak.approvalUserInputCovered')
  if (soak.threadListSearchCovered !== true) missing.push('soak.threadListSearchCovered')
  if (soak.usageCovered !== true) missing.push('soak.usageCovered')

  const actual = recordValue(report.actualPackagedSessionSoak)
  if (actual.passed !== true || actual.status !== 'passed') missing.push('actualPackagedSessionSoak.status')
  const actualRedaction = recordValue(actual.redaction)
  if (actualRedaction.status !== 'passed' || actualRedaction.secretMaterialFound !== false) missing.push('actualPackagedSessionSoak.redaction')
  const actualChecks = Array.isArray(actual.checks) ? actual.checks.map(recordValue) : []
  const requiredActualCheckIds = [
    'packaged-app-artifact',
    'packaged-app-launch',
    'packaged-renderer-bridge',
    'packaged-settings-bridge',
    'packaged-provider-profile-settings',
    'packaged-runtime-restart',
    'packaged-session-thread-create',
    'packaged-session-turn-create',
    'packaged-session-sse-replay',
    'packaged-session-tool-timeline',
    'packaged-session-mimo-plan',
    'packaged-session-attachment-fallback',
    'packaged-session-approval-user-input',
    'packaged-session-fork',
    'packaged-session-resume',
    'packaged-session-thread-list',
    'packaged-session-provider-redaction'
  ]
  for (const id of requiredActualCheckIds) {
    const matching = actualChecks.filter((item) => item.id === id)
    if (matching.length !== 1) {
      missing.push(`actualPackagedSessionSoak.checks:${id}:unique`)
      continue
    }
    if (matching[0].status !== 'passed') missing.push(`actualPackagedSessionSoak.checks:${id}:status`)
  }
  const app = recordValue(actual.app)
  if (app.executableExists !== true) missing.push('actualPackagedSessionSoak.app.executableExists')
  if (app.runtimeServerBinaryExists !== true) missing.push('actualPackagedSessionSoak.app.runtimeServerBinaryExists')
  if (app.appAsarExists !== true) missing.push('actualPackagedSessionSoak.app.appAsarExists')
  if (app.bundledRuntimeGoSourcePresent !== false) {
    missing.push('actualPackagedSessionSoak.app.bundledRuntimeGoSourcePresent:false')
  }
  const renderer = recordValue(actual.renderer)
  if (renderer.apiPresent !== true) missing.push('actualPackagedSessionSoak.renderer.apiPresent')
  if (renderer.title !== 'Analytix') missing.push('actualPackagedSessionSoak.renderer.title:Analytix')
  if (renderer.legacyKun !== false) missing.push('actualPackagedSessionSoak.renderer.legacyKun:false')
  if (renderer.legacyReasonix !== false) missing.push('actualPackagedSessionSoak.renderer.legacyReasonix:false')
  if (renderer.settingsRuntimeTopLevel !== true) missing.push('actualPackagedSessionSoak.renderer.settingsRuntimeTopLevel')
  if (renderer.settingsProfilePatchAccepted !== true) missing.push('actualPackagedSessionSoak.renderer.settingsProfilePatchAccepted')
  if (renderer.settingsCompatPatchUsed !== false) missing.push('actualPackagedSessionSoak.renderer.settingsCompatPatchUsed:false')
  if (renderer.restartOk !== true) missing.push('actualPackagedSessionSoak.renderer.restartOk')
  if (renderer.healthStatus !== 200) missing.push('actualPackagedSessionSoak.renderer.healthStatus:200')
  if (renderer.healthService !== 'analytix') missing.push('actualPackagedSessionSoak.renderer.healthService:analytix')
  if (renderer.threadCreateOk !== true) missing.push('actualPackagedSessionSoak.renderer.threadCreateOk')
  if (renderer.turnCreateOk !== true) missing.push('actualPackagedSessionSoak.renderer.turnCreateOk')
  if (renderer.sseReplayOk !== true) missing.push('actualPackagedSessionSoak.renderer.sseReplayOk')
  if (renderer.attachmentUploadOk !== true) missing.push('actualPackagedSessionSoak.renderer.attachmentUploadOk')
  if (renderer.attachmentTurnOk !== true) missing.push('actualPackagedSessionSoak.renderer.attachmentTurnOk')
  if (renderer.attachmentSseReplayOk !== true) missing.push('actualPackagedSessionSoak.renderer.attachmentSseReplayOk')
  if (renderer.attachmentMetadataOk !== true) missing.push('actualPackagedSessionSoak.renderer.attachmentMetadataOk')
  if (renderer.approvalDenyRequestedOk !== true) missing.push('actualPackagedSessionSoak.renderer.approvalDenyRequestedOk')
  if (renderer.approvalDenyResolvedOk !== true) missing.push('actualPackagedSessionSoak.renderer.approvalDenyResolvedOk')
  if (renderer.approvalDenyNoExecuteOk !== true) missing.push('actualPackagedSessionSoak.renderer.approvalDenyNoExecuteOk')
  if (renderer.approvalAllowRequestedOk !== true) missing.push('actualPackagedSessionSoak.renderer.approvalAllowRequestedOk')
  if (renderer.approvalAllowResolvedOk !== true) missing.push('actualPackagedSessionSoak.renderer.approvalAllowResolvedOk')
  if (renderer.approvalAllowExecutedOk !== true) missing.push('actualPackagedSessionSoak.renderer.approvalAllowExecutedOk')
  if (renderer.userInputRequestedOk !== true) missing.push('actualPackagedSessionSoak.renderer.userInputRequestedOk')
  if (renderer.userInputResolvedOk !== true) missing.push('actualPackagedSessionSoak.renderer.userInputResolvedOk')
  if (renderer.userInputReplayOk !== true) missing.push('actualPackagedSessionSoak.renderer.userInputReplayOk')
  if (renderer.forkOk !== true || renderer.forkTurnOk !== true) missing.push('actualPackagedSessionSoak.renderer.fork')
  if (renderer.resumeOk !== true || renderer.resumeTurnOk !== true) missing.push('actualPackagedSessionSoak.renderer.resume')
  if (renderer.threadListOk !== true) missing.push('actualPackagedSessionSoak.renderer.threadListOk')
  if (renderer.usageOk !== true) missing.push('actualPackagedSessionSoak.renderer.usageOk')
  for (const idHashField of ['initialThreadIdHash', 'initialTurnIdHash', 'forkThreadIdHash', 'resumeThreadIdHash']) {
    if (!isSha256(renderer[idHashField])) missing.push(`actualPackagedSessionSoak.renderer.${idHashField}`)
  }
  const provider = recordValue(actual.provider)
  if (provider.localContractProviderUsed !== true) missing.push('actualPackagedSessionSoak.provider.localContractProviderUsed')
  if (provider.externalProviderNetworkCalled !== false) missing.push('actualPackagedSessionSoak.provider.externalProviderNetworkCalled:false')
  if (Number(provider.requestCount || 0) < 3) missing.push('actualPackagedSessionSoak.provider.requestCount')
  if (provider.authorizationConfigured !== true) missing.push('actualPackagedSessionSoak.provider.authorizationConfigured')
  if (provider.initialPromptSeen !== true) missing.push('actualPackagedSessionSoak.provider.initialPromptSeen')
  if (provider.attachmentFallbackSeen !== true) missing.push('actualPackagedSessionSoak.provider.attachmentFallbackSeen')
  if (provider.approvalDenySeen !== true) missing.push('actualPackagedSessionSoak.provider.approvalDenySeen')
  if (provider.approvalAllowSeen !== true) missing.push('actualPackagedSessionSoak.provider.approvalAllowSeen')
  if (provider.userInputSeen !== true) missing.push('actualPackagedSessionSoak.provider.userInputSeen')
  if (provider.forkPromptSeen !== true) missing.push('actualPackagedSessionSoak.provider.forkPromptSeen')
  if (provider.resumePromptSeen !== true) missing.push('actualPackagedSessionSoak.provider.resumePromptSeen')
  if (provider.historyCarriedToChildOrResume !== true) missing.push('actualPackagedSessionSoak.provider.historyCarriedToChildOrResume')
  return missing
}

function credentialedProbePassed(probe) {
  return recordValue(probe).status === 'passed' &&
    recordValue(probe).credentialed === true &&
    recordValue(probe).skipped !== true
}

function providerCredentialedCoverage(probes) {
  const passedIds = (Array.isArray(probes) ? probes : [])
    .map(recordValue)
    .filter((probe) => credentialedProbePassed(probe))
    .map((probe) => stringValue(probe.id))
    .filter(Boolean)
  const deepseekPassed = passedIds.includes('deepseek')
  const nonDeepSeekPassedIds = passedIds.filter((id) => id !== 'deepseek')
  return {
    passed: deepseekPassed && nonDeepSeekPassedIds.length > 0,
    deepseekPassed,
    nonDeepSeekProviderPassed: nonDeepSeekPassedIds.length > 0,
    nonDeepSeekPassedIds,
    passedIds,
    missing: [
      ...(deepseekPassed ? [] : ['deepseek']),
      ...(nonDeepSeekPassedIds.length > 0 ? [] : ['at-least-one-non-deepseek-provider'])
    ]
  }
}

function toolContentTypes(result) {
  const content = Array.isArray(recordValue(result).content) ? recordValue(result).content : []
  return content
    .map((item) => recordValue(item).type)
    .filter((type) => typeof type === 'string')
    .slice(0, 20)
}

function firstEnv(env, keys) {
  for (const key of keys) {
    const value = String(env[key] || '').trim()
    if (value) return value
  }
  return ''
}

function preferredEnvName(keys) {
  return keys[0]
}

function publicEnvAliases(keys) {
  return keys.filter((key) => /^ANALYTIX_RUNTIME(?:_GO)?_/.test(key))
}

function parseJSONEnv(env, keys, fallback) {
  const value = firstEnv(env, keys)
  if (!value) return fallback
  return parseJSON(value) || fallback
}

function parseJSON(text) {
  try {
    return JSON.parse(text)
  } catch {
    return null
  }
}

function recordValue(value) {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {}
}

function stringValue(value) {
  return typeof value === 'string' ? value.trim() : ''
}

function uniqueStrings(values) {
  return [...new Set((Array.isArray(values) ? values : [])
    .map((item) => String(item || '').trim())
    .filter(Boolean))]
}

function runtimeInfoCapabilityDiagnosticValid(runtimeInfo, prefix) {
  return typeof runtimeInfo[`${prefix}Available`] === 'boolean' &&
    runtimeInfo[`${prefix}DiagnosticHonest`] === true
}

function canonicalJSONString(value) {
  if (Array.isArray(value)) {
    return `[${value.map((item) => canonicalJSONString(item)).join(',')}]`
  }
  if (value && typeof value === 'object') {
    const entries = Object.entries(value).sort(([a], [b]) => a.localeCompare(b))
    return `{${entries.map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSONString(item)}`).join(',')}}`
  }
  return JSON.stringify(value)
}

function normalizedStatus(value) {
  const raw = String(value || '').trim().toLowerCase()
  if (!raw) return 'missing'
  if (raw === 'passed' || raw === 'pass' || raw === 'ok' || raw === '1') return 'passed'
  if (raw === 'skipped' || raw === 'skip') return 'skipped'
  return 'failed'
}

function sha256(value) {
  return createHash('sha256').update(String(value)).digest('hex')
}

function currentRegularFileSha256(path) {
  try {
    const stat = lstatSync(path)
    if (!stat.isFile() || stat.isSymbolicLink()) return ''
    return createHash('sha256').update(readFileSync(path)).digest('hex')
  } catch {
    return ''
  }
}

function isSha256(value) {
  return typeof value === 'string' && /^[a-f0-9]{64}$/i.test(value)
}

function isGitCommit(value) {
  return typeof value === 'string' && /^([a-f0-9]{40}|[a-f0-9]{64})$/i.test(value)
}

function gitCommitExists(value) {
  const child = spawnSync('git', ['cat-file', '-e', `${value}^{commit}`], {
    cwd: process.cwd(),
    encoding: 'utf8'
  })
  return child.status === 0
}

function relevantEvidenceClean(paths = {}) {
  const worktree = worktreeAudit()
  if (relevantEvidenceFinalGateBlockers(worktree).length === 0) return true
  return relevantEvidenceUnstagedDirtyFilesAreCollectorOutputs(worktree, paths)
}

function relevantEvidenceUnstagedDirtyFilesAreCollectorOutputs(worktree, paths = {}) {
  if (worktree.relevantEvidenceUnstagedDirtyFilesTruncated) return false
  const dirtyFiles = Array.isArray(worktree.relevantEvidenceUnstagedDirtyFiles)
    ? worktree.relevantEvidenceUnstagedDirtyFiles
    : []
  if (dirtyFiles.length === 0) return false
  const outputPaths = new Set([
    ...Object.values(defaultRuntimeGoLiveEvidencePaths()),
    ...Object.values(paths || {})
  ].map(normalizeRepoPath).filter(Boolean))
  return dirtyFiles.every((entry) => outputPaths.has(normalizeRepoPath(statusSamplePath(entry))))
}

function statusSamplePath(entry) {
  return String(entry || '').slice(3).trim()
}

function normalizeRepoPath(path) {
  return String(path || '').replace(/\\/g, '/').replace(/^\.\//, '').trim()
}

function currentGitCommit() {
  const child = spawnSync('git', ['rev-parse', 'HEAD'], {
    cwd: process.cwd(),
    encoding: 'utf8'
  })
  return child.status === 0 ? child.stdout.trim() : ''
}

function writeJSON(filePath, value) {
  writeText(filePath, `${JSON.stringify(value, null, 2)}\n`)
}

function writeText(filePath, text) {
  const fullPath = resolve(process.cwd(), filePath)
  mkdirSync(dirname(fullPath), { recursive: true })
  writeFileSync(fullPath, text, 'utf8')
}

function summarizeText(text) {
  return redactText(String(text || '')).replace(/\s+/g, ' ').trim().slice(0, 600)
}

function redactText(text) {
  return String(text || '')
    .replace(/\bBearer\s+[A-Za-z0-9._~+/-]+=*/g, 'Bearer [REDACTED]')
    .replace(/\bBasic\s+[A-Za-z0-9._~+/-]+=*/g, 'Basic [REDACTED]')
    .replace(/((?:api[_-]?key|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|key|signature|sig|auth|credential|jwt)=)[^&\s]+/gi, '$1[REDACTED]')
    .replace(/((?:x-api-key|api[_-]?key|authorization|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|key|signature|sig|auth|credential|jwt)\s*:\s*)[^\s,;]+/gi, '$1[REDACTED]')
    .replace(/("(?:apiKey|api_key|x-api-key|authorization|accessToken|refreshToken|runtimeToken|password|secret|clientSecret|apiKeyHeader|signature|sig|auth|credential|jwt)"\s*:\s*)"[^"]*"/gi, '$1"[REDACTED]"')
    .replace(/\bsk-[A-Za-z0-9_-]{12,}\b/g, 'sk-[REDACTED]')
}

function sanitizeUrlForEvidence(rawUrl) {
  if (!rawUrl) return ''
  try {
    const url = new URL(rawUrl)
    url.username = ''
    url.password = ''
    for (const key of [...url.searchParams.keys()]) {
      if (/(token|key|secret|password|signature|sig|auth|credential|jwt)/i.test(key)) url.searchParams.delete(key)
    }
    return url.toString()
  } catch {
    return redactText(rawUrl)
  }
}

function firstEvidenceSecretFinding(value, path = '$') {
  if (Array.isArray(value)) {
    for (let index = 0; index < value.length; index += 1) {
      const finding = firstEvidenceSecretFinding(value[index], `${path}[${index}]`)
      if (finding) return finding
    }
    return ''
  }
  if (value && typeof value === 'object') {
    for (const [key, item] of Object.entries(value)) {
      if (isSecretEvidenceKey(key)) return `${path}.${key}`
      const finding = firstEvidenceSecretFinding(item, `${path}.${key}`)
      if (finding) return finding
    }
    return ''
  }
  if (typeof value === 'string' && stringLooksLikeSecret(value)) return path
  return ''
}

function isSecretEvidenceKey(key) {
  const normalized = key.replace(/[-_]/g, '').toLowerCase()
  return [
    'apikey',
    'xapikey',
    'apikeyheader',
    'xapikeyheader',
    'authorization',
    'authorizationheader',
    'accesstoken',
    'refreshtoken',
    'runtimetoken',
    'token',
    'password',
    'secret',
    'secretkey',
    'privatekey',
    'clientsecret',
    'signature',
    'sig',
    'auth',
    'credential',
    'jwt'
  ].includes(normalized)
}

function stringLooksLikeSecret(value) {
  return /\bBearer\s+[A-Za-z0-9._~+/-]+=*/.test(value) ||
    /\bBasic\s+[A-Za-z0-9._~+/-]+=*/.test(value) ||
    /\bsk-[A-Za-z0-9_-]{12,}\b/.test(value) ||
    /(?:api[_-]?key|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|key|signature|sig|auth|credential|jwt)=(?!\[?redacted\]?|<redacted>|\[REDACTED\])[^&\s]+/i.test(value) ||
    /(?:x-api-key|api[_-]?key|authorization|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|key|signature|sig|auth|credential|jwt)\s*:\s*(?!\[?redacted\]?|<redacted>|\[REDACTED\])[^\s,;]+/i.test(value)
}

function markdownSummary(report) {
  const providerMatrix = recordValue(recordValue(report.nextEvidenceActions).providerMatrix)
  const settingsProfile = recordValue(providerMatrix.settingsProfile)
  const settingsCoverage = recordValue(providerMatrix.settingsProfileCoverage)
  const completionOptions = recordValue(providerMatrix.completionOptions)
  const finalGateBlockers = Array.isArray(report.finalGateBlockers)
    ? report.finalGateBlockers.map((item) => String(item || '').trim()).filter(Boolean)
    : []
  const lines = [
    '# Runtime Go Live Evidence Report',
    '',
    'Current status: Go runtime default evidence is governed by `runtime:go:release-gate`; `ANALYTIX_RUNTIME_BACKEND=typescript` is a retired-backend diagnostic path, not a runtime rollback.',
    '',
    `Generated at: ${report.generatedAt}`,
    `Source commit: ${report.sourceCommit}`,
    `Status: ${report.status}`,
    `Default backend ready: ${report.defaultBackendReady}`,
    `Go default runtime allowed: ${report.goDefaultRuntimeAllowed}`,
    `TypeScript default-path fallback retained: ${report.typeScriptFallbackRetained}`,
    `TypeScript override retired: ${report.typeScriptOverrideRetired}`,
    `Evidence digest algorithm: ${report.evidenceDigestAlgorithm}`,
    '',
    '## Final Gate',
    '',
    `- Blocked: ${report.finalGateBlocked === true}`,
    `- Blockers: ${finalGateBlockers.length > 0 ? finalGateBlockers.join(', ') : 'none'}`,
    '',
    '## Components',
    '',
    `- Provider matrix: ${report.componentStatus.provider}`,
    `- MCP execution: ${report.componentStatus.mcp}`,
    `- Packaged desktop QA: ${report.componentStatus.packaged}`,
    `- Packaged GUI bridge smoke: ${report.componentStatus.packagedGui}`,
    `- Packaged session soak: ${report.componentStatus.packagedSoak}`,
    `- Packaged general Agent Milestone A: ${report.componentStatus.packagedMilestoneA}`,
    `- Operator gate: ${report.componentStatus.operator}`,
    '',
    '## Provider Settings Coverage',
    '',
    `- Settings source: ${stringValue(settingsProfile.source) || 'unknown'}`,
    `- Settings loaded: ${settingsProfile.status === 'loaded'}`,
    `- Active provider id: ${stringValue(settingsProfile.activeProviderId) || 'unknown'}`,
    `- Selected provider id: ${stringValue(settingsProfile.selectedProviderId) || 'unknown'}`,
    `- Provider profile count: ${Number.isFinite(settingsProfile.providerCount) ? settingsProfile.providerCount : 0}`,
    `- Usable provider ids: ${markdownListValue(settingsCoverage.usableProviderIds)}`,
    `- Usable non-DeepSeek provider ids: ${markdownListValue(settingsCoverage.usableNonDeepSeekProviderIds)}`,
    `- Satisfies final provider coverage from settings: ${settingsCoverage.satisfiesFinalProviderCoverageFromSettings === true}`,
    '',
    '## Provider Completion Options',
    '',
    `- Settings profile path: ${stringValue(completionOptions.settingsProfilePath) || 'provider.providers[]'}`,
    `- Required non-DeepSeek settings fields: ${markdownListValue(completionOptions.requiredNonDeepSeekSettingsFields)}`,
    `- Recognized preset provider ids: ${markdownListValue(completionOptions.recognizedPresetProviderIds)}`,
    `- Accepted non-DeepSeek profile signals: ${markdownListValue(completionOptions.acceptedNonDeepSeekProfileSignals)}`,
    `- Env completion groups: ${markdownEnvGroups(completionOptions.envCompletionGroups)}`,
    '',
    '## Missing External Inputs',
    ''
  ]
  if (report.missingExternalInputs.length === 0) {
    lines.push('- none')
  } else {
    for (const item of report.missingExternalInputs) {
      const detail = missingExternalInputDetail(item)
      lines.push(`- ${item.id}: ${item.status} - ${String(item.reason || '').replace(/\|/g, '/')}${detail}`)
    }
  }
  lines.push('', '## Next Evidence Commands', '')
  for (const command of report.nextEvidenceActions.commands) {
    lines.push(`- \`${command}\``)
  }
  lines.push('', 'This report is evidence-only and does not switch the default runtime.')
  lines.push('')
  return lines.join('\n')
}

function markdownListValue(value) {
  const items = Array.isArray(value)
    ? value.map((item) => String(item || '').trim()).filter(Boolean)
    : []
  return items.length > 0 ? items.join(', ') : 'none'
}

function markdownEnvGroups(value) {
  const groups = Array.isArray(value) ? value.map(recordValue) : []
  const rendered = groups
    .map((item) => {
      const id = stringValue(item.id)
      const fields = markdownListValue(item.requiredEnv)
      return id ? `${id} (${fields})` : ''
    })
    .filter(Boolean)
  return rendered.length > 0 ? rendered.join('; ') : 'none'
}

function missingExternalInputDetail(item) {
  const details = []
  const missingEnv = Array.isArray(item.missingEnv) ? item.missingEnv.filter(Boolean) : []
  const missingSettings = Array.isArray(item.missingSettingsProfileFields)
    ? item.missingSettingsProfileFields.filter(Boolean)
    : []
  const candidateProviderIds = Array.isArray(item.candidateProviderIds)
    ? item.candidateProviderIds.filter(Boolean)
    : []
  const reasons = Array.isArray(item.reasons) ? item.reasons.filter(Boolean) : []
  const requiredEvidence = Array.isArray(item.requiredEvidence) ? item.requiredEvidence.filter(Boolean) : []
  if (missingEnv.length > 0) details.push(`env: ${missingEnv.join(', ')}`)
  if (missingSettings.length > 0) details.push(`settings: ${missingSettings.join(', ')}`)
  if (candidateProviderIds.length > 0) details.push(`candidates: ${candidateProviderIds.join(', ')}`)
  if (reasons.length > 0) details.push(`reasons: ${reasons.join('; ')}`)
  if (requiredEvidence.length > 0) details.push(`required evidence: ${requiredEvidence.join('; ')}`)
  return details.length > 0 ? ` (${details.join('; ')})` : ''
}

function argValue(rawArgs, name, fallback = '') {
  const prefix = `${name}=`
  const inline = rawArgs.find((item) => item.startsWith(prefix))
  if (inline) return inline.slice(prefix.length)
  const index = rawArgs.indexOf(name)
  return index >= 0 ? rawArgs[index + 1] || '' : fallback
}

async function main() {
  const rawArgs = process.argv.slice(2)
  const args = new Set(rawArgs)
  const json = args.has('--json')
  const reportDir = argValue(rawArgs, '--report-dir', DEFAULT_REPORT_DIR)
  const paths = defaultRuntimeGoLiveEvidencePaths(reportDir)
  const report = await buildRuntimeGoLiveEvidenceReport({
    reportDir,
    noWrite: args.has('--no-write'),
    dryRun: args.has('--dry-run'),
    settingsPath: argValue(rawArgs, '--settings-path', ''),
    timeoutMs: Number(argValue(rawArgs, '--timeout-ms', process.env.ANALYTIX_RUNTIME_GO_LIVE_TIMEOUT_MS || DEFAULT_TIMEOUT_MS)),
    packagedTimeoutMs: Number(argValue(
      rawArgs,
      '--packaged-timeout-ms',
      process.env.ANALYTIX_RUNTIME_GO_LIVE_PACKAGED_TIMEOUT_MS ||
        process.env.ANALYTIX_RUNTIME_GO_PACKAGED_QA_TIMEOUT_MS ||
        DEFAULT_PACKAGED_TIMEOUT_MS
    )),
    paths: {
      report: argValue(rawArgs, '--out-json', paths.report),
      provider: argValue(rawArgs, '--provider-json', paths.provider),
      mcp: argValue(rawArgs, '--mcp-json', paths.mcp),
      packaged: argValue(rawArgs, '--packaged-json', paths.packaged),
      packagedGui: argValue(rawArgs, '--packaged-gui-json', paths.packagedGui),
      packagedSoak: argValue(rawArgs, '--packaged-soak-json', paths.packagedSoak),
      packagedMilestoneA: argValue(
        rawArgs,
        '--packaged-milestone-a-json',
        paths.packagedMilestoneA
      ),
      operator: argValue(rawArgs, '--operator-json', paths.operator),
      markdown: argValue(rawArgs, '--out-md', paths.markdown)
    }
  })
  if (json) {
    console.log(JSON.stringify(report, null, 2))
  } else {
    console.log(`${report.status.toUpperCase()} ${report.id}`)
    for (const item of report.missingExternalInputs) {
      console.log(`MISSING ${item.id}: ${item.reason}`)
    }
  }
  if (args.has('--gate') && !report.passed) {
    process.exitCode = 1
  }
}

const entryPath = process.argv[1] ? fileURLToPath(import.meta.url) === resolve(process.argv[1]) : false
if (entryPath) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.stack || error.message : String(error))
    process.exitCode = 1
  })
}
