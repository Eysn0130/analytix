#!/usr/bin/env node

import { localProviderPublicSeamPassed } from './lib/local-provider-acceptance.mjs'

import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { lstatSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const jsonOutput = args.has('--json') || args.has('--dry-run')
const skipCommands = args.has('--skip-commands') || args.has('--dry-run')
const reportOnly = args.has('--no-gate')
const actualPackagedSmoke = args.has('--actual') || process.env.ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SMOKE === '1'
const noWrite = args.has('--no-write') || skipCommands
const defaultEvidencePath = 'docs/analytix/upstreams/runtime-go-live-evidence/packaged-go-runtime-qa.json'
const REQUIRED_COMMERCIAL_RELEASE_CHECK_IDS = [
  'formal-release-publication-authority',
  'controlled-release-native-receipt'
]

function optionValue(name) {
  const inlinePrefix = `${name}=`
  for (let index = 0; index < rawArgs.length; index += 1) {
    const item = rawArgs[index]
    if (item.startsWith(inlinePrefix)) return item.slice(inlinePrefix.length)
    if (item === name && rawArgs[index + 1]) return rawArgs[index + 1]
  }
  return ''
}

const evidencePath = optionValue('--output') ||
  process.env.ANALYTIX_RUNTIME_GO_PACKAGED_QA_EVIDENCE ||
  defaultEvidencePath
const commandTimeoutMs = Number(optionValue('--timeout-ms') ||
  process.env.ANALYTIX_RUNTIME_GO_PACKAGED_QA_TIMEOUT_MS ||
  0)

function appendOptionPassThrough(target, name) {
  const inlinePrefix = `${name}=`
  for (let index = 0; index < rawArgs.length; index += 1) {
    const item = rawArgs[index]
    if (item.startsWith(inlinePrefix)) {
      target.push(item)
      continue
    }
    if (item === name && rawArgs[index + 1]) {
      target.push(item, rawArgs[index + 1])
      index += 1
    }
  }
}

const packagedGuiSmokeArgs = ['run', 'runtime:go:packaged-gui-smoke', '--', '--json']
if (actualPackagedSmoke) packagedGuiSmokeArgs.push('--actual')
appendOptionPassThrough(packagedGuiSmokeArgs, '--app-path')
appendOptionPassThrough(packagedGuiSmokeArgs, '--timeout-ms')

const packagedSessionSoakArgs = ['run', 'runtime:go:packaged-soak', '--', '--json', '--no-write']
if (actualPackagedSmoke) packagedSessionSoakArgs.push('--actual')
appendOptionPassThrough(packagedSessionSoakArgs, '--app-path')
appendOptionPassThrough(packagedSessionSoakArgs, '--timeout-ms')

const packagedMilestoneAArgs = [
  'run',
  'runtime:go:packaged-milestone-a',
  '--',
  '--json',
  '--no-write',
  '--no-gate'
]
if (!actualPackagedSmoke) packagedMilestoneAArgs.push('--dry-run')
for (const name of [
  '--app-path',
  '--timeout-ms',
  '--expected-source-commit',
  '--target-platform',
  '--target-arch',
  '--repository-path',
  '--repository-contract',
  '--repository-provenance',
  '--repository-owner-root',
  '--release-authority',
  '--release-authority-public-key',
  '--release-tag',
  '--release-channel',
  '--release-dist'
]) {
  appendOptionPassThrough(packagedMilestoneAArgs, name)
}

const commands = [
  {
    id: 'cutover-report',
    command: npmCommand,
    args: ['run', 'runtime:go:cutover-report', '--', '--json'],
    expectsJSON: true
  },
  {
    id: 'gui-smoke',
    command: npmCommand,
    args: packagedGuiSmokeArgs,
    expectsJSON: true
  },
  {
    id: 'session-soak',
    command: npmCommand,
    args: packagedSessionSoakArgs,
    expectsJSON: true
  },
  {
    id: 'milestone-a',
    command: npmCommand,
    args: packagedMilestoneAArgs,
    expectsJSON: true
  },
  {
    id: 'rollback-evidence',
    command: npmCommand,
    args: ['run', 'runtime:go:rollback-retirement-evidence', '--', '--json'],
    expectsJSON: true
  },
  {
    id: 'packaging-config',
    command: npmCommand,
    args: ['run', 'test', '--', 'src/main/packaging-config.test.ts', '--run']
  }
]

function commandText(item) {
  return [item.command, ...item.args].join(' ')
}

function currentGitCommit() {
  const result = spawnSync('git', ['rev-parse', 'HEAD'], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  return result.status === 0 ? String(result.stdout || '').trim() : ''
}

function fileSha256(path) {
  try {
    return createHash('sha256').update(readFileSync(path)).digest('hex')
  } catch {
    return ''
  }
}

function canonicalJSON(value) {
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`
  if (value && typeof value === 'object') {
    return `{${Object.entries(value)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSON(item)}`)
      .join(',')}}`
  }
  return JSON.stringify(value)
}

function harnessFileEvidence(name, path) {
  try {
    const stat = lstatSync(path)
    return {
      name,
      regular: stat.isFile() && !stat.isSymbolicLink(),
      byteLength: stat.size,
      sha256: fileSha256(path)
    }
  } catch {
    return { name, regular: false, byteLength: 0, sha256: '' }
  }
}

function harnessManifest() {
  const entries = [
    ['runtime-go-packaged-milestone-a.mjs', resolve(process.cwd(), 'scripts/runtime-go-packaged-milestone-a.mjs')],
    ['local-provider-acceptance.mjs', resolve(process.cwd(), 'scripts/lib/local-provider-acceptance.mjs')],
    ['local-provider-credential-scan.mjs', resolve(process.cwd(), 'scripts/lib/local-provider-credential-scan.mjs')],
    ['runtime-go-packaged-qa.mjs', fileURLToPath(import.meta.url)],
    ['runtime-go-live-evidence-collector.mjs', resolve(process.cwd(), 'scripts/runtime-go-live-evidence-collector.mjs')],
    ['runtime-go-validation-command.mjs', resolve(process.cwd(), 'scripts/runtime-go-validation-command.mjs')],
    ['runtime-go-cutover-report.mjs', resolve(process.cwd(), 'scripts/runtime-go-cutover-report.mjs')],
    ['runtime-go-packaged-gui-smoke.mjs', resolve(process.cwd(), 'scripts/runtime-go-packaged-gui-smoke.mjs')],
    ['runtime-go-packaged-session-soak.mjs', resolve(process.cwd(), 'scripts/runtime-go-packaged-session-soak.mjs')],
    ['runtime-go-rollback-retirement-evidence.mjs', resolve(process.cwd(), 'scripts/runtime-go-rollback-retirement-evidence.mjs')],
    ['packaging-config.test.ts', resolve(process.cwd(), 'src/main/packaging-config.test.ts')],
    ['after-pack.cjs', resolve(process.cwd(), 'scripts/after-pack.cjs')],
    ['packaged-release-publication-authority.mjs', resolve(process.cwd(), 'scripts/lib/packaged-release-publication-authority.mjs')],
    ['publish-r2.mjs', resolve(process.cwd(), 'scripts/publish-r2.mjs')],
    ['strict-json.cjs', resolve(process.cwd(), 'scripts/lib/strict-json.cjs')],
    ['macos-signing-policy.cjs', resolve(process.cwd(), 'scripts/macos-signing-policy.cjs')],
    ['macos-signing-policy.json', resolve(process.cwd(), 'scripts/macos-signing-policy.json')],
    ['entitlements.mac.native-helper.plist', resolve(process.cwd(), 'build/entitlements.mac.native-helper.plist')],
    ['package.json', resolve(process.cwd(), 'package.json')],
    ['package-lock.json', resolve(process.cwd(), 'package-lock.json')],
    ['scripts/use-analytix-cache.sh', resolve(process.cwd(), 'scripts/use-analytix-cache.sh')],
    ['scripts/analytix-cache-storage.zsh', resolve(process.cwd(), 'scripts/analytix-cache-storage.zsh')]
  ].map(([name, path]) => harnessFileEvidence(name, path))
  return {
    scriptSha256:
      entries.find((item) => item.name === 'runtime-go-packaged-qa.mjs')?.sha256 || '',
    contractManifestSha256: createHash('sha256')
      .update(canonicalJSON(entries))
      .digest('hex'),
    entries
  }
}

function parseLastJSONObject(text) {
  let start = text.lastIndexOf('{')
  while (start >= 0) {
    const candidate = text.slice(start).trim()
    try {
      const value = JSON.parse(candidate)
      if (value && typeof value === 'object' && !Array.isArray(value)) {
        return { ok: true, value }
      }
    } catch {
      // Child validation commands may write npm logs before the final JSON report.
    }
    start = text.lastIndexOf('{', start - 1)
  }
  return { ok: false, reason: 'command did not emit a trailing JSON object' }
}

function runCheck(item) {
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-packaged-qa] ${item.id}`)
  if (skipCommands) {
    return {
      check: {
        id: item.id,
        status: 'skipped',
        command: commandText(item),
        durationMs: 0
      }
    }
  }
  const result = spawnSync(item.command, item.args, {
    cwd: process.cwd(),
    env: process.env,
    stdio: 'pipe',
    encoding: 'utf8',
    maxBuffer: 160 * 1024 * 1024,
    ...(commandTimeoutMs > 0 ? { timeout: commandTimeoutMs } : {})
  })
  const exitStatus = result.status ?? (result.signal ? 1 : 0)
  const parsed = item.expectsJSON ? parseLastJSONObject(result.stdout || '') : { ok: true, value: undefined }
  const report = parsed.ok ? parsed.value : undefined
  const reportStatus = typeof report?.status === 'string' ? report.status : ''
  const passed = exitStatus === 0 && (!item.expectsJSON || reportStatus === 'passed')
  const liveBlocked =
    exitStatus === 0 && item.expectsJSON && reportStatus === 'live_blocked'
  if (!passed && !jsonOutput) {
    if (result.stdout) process.stdout.write(result.stdout)
    if (result.stderr) process.stderr.write(result.stderr)
  }
  const check = {
    id: item.id,
    status: passed ? 'passed' : liveBlocked ? 'live_blocked' : 'failed',
    command: commandText(item),
    durationMs: Date.now() - started,
    timeoutMs: commandTimeoutMs > 0 ? commandTimeoutMs : undefined,
    exitStatus,
    reportId: typeof report?.id === 'string' ? report.id : '',
    reportStatus,
    reason: failureReason(item, result, parsed, exitStatus, reportStatus)
  }
  if (result.signal) check.signal = result.signal
  return { check, report }
}

function failureReason(item, result, parsed, exitStatus, reportStatus) {
  if (result.error) return result.error.message
  if (exitStatus !== 0) return `command exited with status ${exitStatus}`
  if (item.expectsJSON && !parsed.ok) return parsed.reason
  if (item.expectsJSON && reportStatus !== 'passed') return `report status is ${reportStatus || 'missing'}`
  return ''
}

const checks = []
const reports = {}
let failed = false

for (const item of commands) {
  const { check, report } = runCheck(item)
  checks.push(check)
  if (report) reports[item.id] = report
  if (check.status === 'failed') {
    failed = true
  }
}

const cutover = reports['cutover-report'] || {}
const guiSmoke = reports['gui-smoke'] || {}
const sessionSoak = reports['session-soak'] || {}
const milestoneA = reports['milestone-a'] || {}
const milestoneACommercialChecks = Array.isArray(milestoneA.commercialRelease?.checks)
  ? milestoneA.commercialRelease.checks
  : []
const commercialReleaseChecks = REQUIRED_COMMERCIAL_RELEASE_CHECK_IDS.map((id) => {
  const row = milestoneACommercialChecks.find((item) => item?.id === id)
  const rowStatus = row?.status === 'passed'
    ? 'passed'
    : row?.status === 'failed'
      ? 'failed'
      : 'live_blocked'
  return {
    id: `commercial-release:${id}`,
    status: rowStatus,
    command: 'independent commercial publication evidence row',
    durationMs: 0,
    reportId: 'runtime-go-packaged-milestone-a',
    reportStatus: milestoneA.commercialRelease?.status || 'unverified',
    reason: row?.message || `commercial publication evidence row is unverified: ${id}`
  }
})
checks.push(...commercialReleaseChecks)
failed = checks.some((item) => item.status === 'failed')
const commercialReleaseReady = commercialReleaseChecks.every((item) => item.status === 'passed') &&
  milestoneA.commercialRelease?.requiredForMilestoneA === false &&
  milestoneA.commercialRelease?.requiredForCommercialPublication === true &&
  milestoneA.commercialRelease?.passed === true &&
  milestoneA.releasePublicationAuthority?.ok === true &&
  milestoneA.app?.controlledReleaseNativeReceipt === true
const qaHarness = harnessManifest()
const milestoneAHarnessEntries = Array.isArray(milestoneA.harness?.entries)
  ? milestoneA.harness.entries
  : []
const expectedMilestoneHarnessDigest = createHash('sha256')
  .update(canonicalJSON(qaHarness.entries))
  .digest('hex')
const milestoneAHarnessEntriesExactlyBound =
  milestoneAHarnessEntries.length === qaHarness.entries.length &&
  qaHarness.entries.every((expected, index) => {
    const observed = milestoneAHarnessEntries[index]
    return observed?.name === expected.name &&
      observed?.regular === true && expected.regular === true &&
      observed?.byteLength === expected.byteLength &&
      observed?.sha256 === expected.sha256 &&
      /^[0-9a-f]{64}$/.test(expected.sha256)
  })
const milestoneAHarnessBound =
  /^[0-9a-f]{64}$/.test(milestoneA.harness?.scriptSha256 || '') &&
  /^[0-9a-f]{64}$/.test(milestoneA.harness?.contractManifestSha256 || '') &&
  milestoneA.harness?.sourceClosureBound === true &&
  /^[0-9a-f]{64}$/.test(milestoneA.harness?.sourceClosureSnapshotDigest || '') &&
  milestoneA.harness.sourceClosureSnapshotDigest ===
    milestoneA.harness?.packagedSourceClosureSnapshotDigest &&
  milestoneA.harness.scriptSha256 ===
    fileSha256(resolve(process.cwd(), 'scripts/runtime-go-packaged-milestone-a.mjs')) &&
  milestoneA.harness.contractManifestSha256 === expectedMilestoneHarnessDigest &&
  milestoneAHarnessEntriesExactlyBound
const rollbackEvidence = reports['rollback-evidence'] || {}
const sessionActualEvidenceRequired = sessionSoak.soak?.actualPackagedAppEvidenceRequiredForFinalGate !== false
const packagingConfigPassed = checks.find((item) => item.id === 'packaging-config')?.status === 'passed'
const credentialedProviderPublicSeamPassed = localProviderPublicSeamPassed(milestoneA)
const formalGeneralAgentWorkflowPassed =
  milestoneA.passed === true &&
  milestoneA.workflow?.planModeSelected === true &&
  milestoneA.workflow?.planTurnObserved === true &&
  milestoneA.workflow?.planProviderReceiptBound === true &&
  milestoneA.workflow?.agentModeRestored === true &&
  milestoneA.workflow?.sameThreadPlanAgent === true &&
  milestoneA.workflow?.readObserved === true &&
  milestoneA.workflow?.planObserved === true &&
  milestoneA.workflow?.todoObserved === true &&
  milestoneA.workflow?.writeObserved === true &&
  milestoneA.workflow?.realTestObserved === true &&
  milestoneA.workflow?.subagentObserved === true &&
  milestoneA.workflow?.childSubagentProviderReceiptBound === true &&
  milestoneA.workflow?.gitCommandObserved === true &&
  milestoneA.workflow?.skillObserved === true &&
  milestoneA.workflow?.ordinaryMCPObserved === true &&
  milestoneA.workflow?.researchWritingObserved === true &&
  milestoneA.workflow?.relaunchProviderContinuationObserved === true &&
  milestoneA.publicSeams?.firstLaunch?.gitCommandAvailable === true &&
  milestoneA.publicSeams?.firstLaunch?.skillCatalogAvailable === true &&
  milestoneA.publicSeams?.firstLaunch?.ordinaryMCPAvailable === true &&
  milestoneA.publicSeams?.secondLaunch?.gitCommandAvailable === true &&
  milestoneA.publicSeams?.secondLaunch?.skillCatalogAvailable === true &&
  milestoneA.publicSeams?.secondLaunch?.ordinaryMCPAvailable === true &&
  milestoneA.workflow?.successfulToolResultsObserved === true &&
  milestoneA.workflow?.todosCompleted === true &&
  milestoneA.workflow?.exactlyOneBoundedSubagentCompleted === true &&
  milestoneA.caseCapability?.additiveNotReplacement === true &&
  milestoneA.caseCapability?.privateAuthorityInputsAbsent === true &&
  milestoneA.caseCapability?.unavailableWhileOrdinaryCapabilitiesRetained === true &&
  Number(milestoneA.workflow?.compactionCount || 0) > 0
const rendererVisualPublicSeamEvidencePassed =
  milestoneA.visualEvidence?.screenshotsUsedAsFunctionalVerdict === false &&
  milestoneA.visualEvidence?.credentialScanCovered === true &&
  [
    milestoneA.visualEvidence?.firstCompleted,
    milestoneA.visualEvidence?.recoveredAfterRelaunch
  ].every((item) =>
    item?.captured === true &&
    /^[0-9a-f]{64}$/.test(item.sha256 || '') &&
    Number(item.width || 0) > 0 &&
    Number(item.height || 0) > 0 &&
    Number(item.viewport?.width || 0) > 0 &&
    Number(item.viewport?.height || 0) > 0 &&
    Number(item.viewport?.scale || 0) > 0
  )
const actualPackagedDesktopQAPassed =
  milestoneA.passed === true &&
  milestoneA.status === 'passed' &&
  milestoneA.app?.ok === true &&
  milestoneA.directRuntimeTurnDriverUsed === false &&
  milestoneA.composerDomAndPrimaryButtonRequired === true &&
  milestoneA.cdpAndBridgeObservationOnly === true &&
  milestoneAHarnessBound &&
  packagingConfigPassed === true &&
  credentialedProviderPublicSeamPassed &&
  formalGeneralAgentWorkflowPassed &&
  rendererVisualPublicSeamEvidencePassed
const actualPackagedAppLaunched = guiSmoke.smoke?.actualPackagedAppLaunched === true ||
  sessionSoak.soak?.actualPackagedAppLaunched === true ||
  milestoneA.workflow?.firstLaunchObserved === true
const deterministicContractOnly = !actualPackagedDesktopQAPassed && (
  guiSmoke.smoke?.deterministicContractOnly === true ||
  sessionSoak.soak?.deterministicContractOnly === true ||
  !credentialedProviderPublicSeamPassed ||
  !formalGeneralAgentWorkflowPassed ||
  !rendererVisualPublicSeamEvidencePassed
)
const typeScriptRetiredBackendPassed = rollbackEvidence.passed === true &&
  rollbackEvidence.rollback?.typeScriptOverrideRetired === true &&
  rollbackEvidence.rollback?.typeScriptCodePathDeleted === true &&
  rollbackEvidence.rollback?.retiredBackendCovered === true &&
  rollbackEvidence.rollback?.legacyChildRetiredCovered === true &&
  rollbackEvidence.rollback?.analytixServeGoLauncherCovered === true &&
  rollbackEvidence.rollback?.packagedGoBoundaryCovered === true
const typeScriptRuntimeSourceDeleted = rollbackEvidence.rollback?.typeScriptRuntimeSourceDeleted === true
const durableProcessRelaunchObserved =
  milestoneA.workflow?.normalFirstQuitObserved === true &&
  milestoneA.workflow?.freshProcessRelaunchObserved === true &&
  milestoneA.workflow?.exactThreadRecovered === true &&
  milestoneA.workflow?.normalFinalQuitObserved === true &&
  milestoneA.workflow?.zeroResidualProcesses === true
const durableRestartPassed = milestoneA.passed === true &&
  milestoneA.workflow?.exactTodosRecovered === true &&
  milestoneA.workflow?.exactSubagentsRecovered === true &&
  milestoneA.workflow?.exactCompactionsRecovered === true &&
  milestoneA.workflow?.exactResultRecovered === true &&
  milestoneA.workflow?.rendererVisibleThreadRecovered === true &&
  milestoneA.workflow?.rendererVisibleTodosRecovered === true &&
  milestoneA.workflow?.rendererVisibleSubagentRecovered === true &&
  milestoneA.workflow?.rendererVisibleCompactionRecovered === true &&
  milestoneA.workflow?.rendererVisibleResultRecovered === true &&
  milestoneA.isolation?.sameDataDirsOnRelaunch === true &&
  milestoneA.isolation?.sandboxRemoved === true &&
  durableProcessRelaunchObserved
const actualAppEvidenceRequired = guiSmoke.smoke?.actualPackagedAppEvidenceRequiredForFinalGate !== false ||
  sessionActualEvidenceRequired ||
  milestoneA.passed !== true ||
  !actualPackagedDesktopQAPassed ||
  !durableRestartPassed
const missingFinalCoverage = new Set(Array.isArray(cutover.missingFinalCoverage) ? cutover.missingFinalCoverage : [])
if (actualPackagedDesktopQAPassed) {
  missingFinalCoverage.delete('packaged-desktop-qa')
} else {
  missingFinalCoverage.add('packaged-desktop-qa')
}
if (typeScriptRetiredBackendPassed) {
  missingFinalCoverage.delete('typescript-retired-backend')
}
if (typeScriptRuntimeSourceDeleted) {
  missingFinalCoverage.delete('typescript-runtime-source-deleted')
} else {
  missingFinalCoverage.add('typescript-runtime-source-deleted')
}
if (durableRestartPassed) {
  missingFinalCoverage.delete('durable-restart')
} else {
  missingFinalCoverage.add('durable-restart')
}
for (const id of REQUIRED_COMMERCIAL_RELEASE_CHECK_IDS) {
  const passed = commercialReleaseChecks.some((item) =>
    item.id === `commercial-release:${id}` && item.status === 'passed'
  )
  if (passed) missingFinalCoverage.delete(id)
  else missingFinalCoverage.add(id)
}
const passed = !skipCommands && !failed && checks.every((item) => item.status === 'passed')
const liveBlocked = checks.some((item) => item.status === 'live_blocked')
const status = passed
  ? 'passed'
  : skipCommands
    ? 'skipped'
    : failed
      ? 'failed'
      : liveBlocked
        ? 'live_blocked'
        : 'failed'
const testHarnessCommit = currentGitCommit()
const report = {
  schemaVersion: 1,
  id: 'runtime-go-packaged-qa',
  generatedAt: new Date().toISOString(),
  sourceCommit: testHarnessCommit,
  testHarnessCommit,
  harness: qaHarness,
  status,
  passed,
  credentialSecretsRecorded: false,
  redaction: {
    status: 'passed',
    secretMaterialFound: false
  },
  milestoneAEvidence: milestoneA,
  packaged: {
    cutoverReady: cutover.goRuntimeDefaultCutoverReady === true,
    cutoverFinalAcceptanceReady: cutover.finalAcceptanceReady === true,
    finalAcceptanceReady: passed && missingFinalCoverage.size === 0,
    missingFinalCoverage: Array.from(missingFinalCoverage),
    runtimeHealthPassed: cutover.cutover?.runtimeHealthPassed === true,
    runtimeHealthRuntimeInfoOk: cutover.cutover?.runtimeHealthRuntimeInfoOk === true,
    runtimeHealthRuntimeToolsOk: cutover.cutover?.runtimeHealthRuntimeToolsOk === true,
    runtimeHealthProductionCapabilitiesOk: cutover.cutover?.runtimeHealthProductionCapabilitiesOk === true,
    guiSmokePassed: guiSmoke.passed === true,
    sessionSoakPassed: sessionSoak.passed === true,
    milestoneAPassed: milestoneA.passed === true,
    milestoneAStatus: milestoneA.status || '',
    milestoneAHarnessScriptSha256: milestoneA.harness?.scriptSha256 || '',
    milestoneAHarnessManifestSha256: milestoneA.harness?.contractManifestSha256 || '',
    milestoneAHarnessBound,
    actualPackagedSmokeRequested: guiSmoke.smoke?.actualPackagedSmokeRequested === true,
    actualPackagedSmokePassed: guiSmoke.smoke?.actualPackagedSmokePassed === true,
    actualPackagedDesktopQAPassed,
    milestoneAFunctionalPassed: actualPackagedDesktopQAPassed,
    commercialReleaseReady,
    commercialReleaseStatus: milestoneA.commercialRelease?.status || 'unverified',
    formalReleasePublicationAuthorityPassed:
      milestoneA.releasePublicationAuthority?.ok === true,
    controlledReleaseNativeReceiptPassed:
      milestoneA.app?.controlledReleaseNativeReceipt === true,
    credentialedProviderPublicSeamPassed,
    formalGeneralAgentWorkflowPassed,
    rendererVisualPublicSeamEvidencePassed,
    deterministicContractOnly,
    sessionSoakDeterministicContractOnly: sessionSoak.soak?.deterministicContractOnly === true,
    actualPackagedSessionSoakRequested: sessionSoak.soak?.actualPackagedSessionSoakRequested === true,
    actualPackagedSessionSoakPassed: sessionSoak.soak?.actualPackagedSessionSoakPassed === true,
    sessionSoakActualEvidenceRequired: sessionActualEvidenceRequired,
    durableRestartPassed,
    durableProcessRelaunchObserved,
    actualPackagedAppLaunched,
    settingsProfilePatchAccepted: guiSmoke.smoke?.settingsProfilePatchAccepted === true,
    settingsCompatPatchUsed: guiSmoke.smoke?.settingsCompatPatchUsed === true,
    settingsPatchMode: guiSmoke.smoke?.settingsPatchMode || '',
    typeScriptRetiredBackendPassed,
    typeScriptCodePathDeleted: rollbackEvidence.rollback?.typeScriptCodePathDeleted === true,
    typeScriptRuntimeSourceDeleted,
    sourceRuntimeImplementationCount: rollbackEvidence.rollback?.sourceRuntimeImplementationCount ?? null,
    actualPackagedAppEvidenceRequiredForFinalGate: actualAppEvidenceRequired,
    packagingConfigPassed,
    directLegacyExposureCount: cutover.cutover?.directLegacyExposureCount ?? null,
    internalLegacyDelegateCount: cutover.cutover?.internalLegacyDelegateCount ?? null,
    temporaryGoProdViolationCount: cutover.cutover?.temporaryGoProdViolationCount ?? null,
    productionGoSourceCount: cutover.cutover?.productionGoSourceCount ?? null,
    retiredProductionMarkerViolationCount: cutover.cutover?.retiredProductionMarkerViolationCount ?? null,
    legacyUpstreamFieldKeyViolationCount: cutover.cutover?.legacyUpstreamFieldKeyViolationCount ?? null
  },
  checks
}

if (!noWrite) {
  const absoluteEvidencePath = resolve(process.cwd(), evidencePath)
  report.outputPath = absoluteEvidencePath
  mkdirSync(dirname(absoluteEvidencePath), { recursive: true })
  writeFileSync(absoluteEvidencePath, JSON.stringify(report, null, 2), 'utf8')
}

if (jsonOutput) {
  console.log(JSON.stringify(report, null, 2))
} else {
  console.log(`${status.toUpperCase()} ${report.id}`)
  for (const item of checks.filter((check) => check.status !== 'passed')) {
    console.log(`${item.status.toUpperCase()} ${item.id}: ${item.reason}`)
  }
}

if (!reportOnly && !passed) process.exitCode = 1
