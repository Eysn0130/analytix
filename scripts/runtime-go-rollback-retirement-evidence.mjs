#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import process from 'node:process'
import { runTypeScriptRetirementScan } from './runtime-go-ts-retirement-scan.mjs'

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const jsonOutput = args.has('--json') || args.has('--dry-run')
const skipCommands = args.has('--skip-commands') || args.has('--dry-run')
const reportOnly = args.has('--no-gate')

const commands = [
  {
    id: 'typescript-retired-backend',
    command: npmCommand,
    args: ['run', 'test', '--', 'src/main/runtime/analytix-adapter.test.ts', '--run']
  },
  {
    id: 'legacy-child-retired',
    command: npmCommand,
    args: ['run', 'test', '--', 'src/main/analytix-process.test.ts', '--run']
  },
  {
    id: 'analytix-serve-go-launcher',
    command: npmCommand,
    args: ['run', '--prefix', 'packages/runtime', 'test', '--', 'tests/serve-entry-go-launcher.test.ts', '--run']
  },
  {
    id: 'packaged-go-boundary',
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
      // Child validation commands may log before the final JSON object.
    }
    start = text.lastIndexOf('{', start - 1)
  }
  return { ok: false, reason: 'command did not emit a trailing JSON object' }
}

function runCheck(item) {
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-rollback-retirement-evidence] ${item.id}`)
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
    maxBuffer: 160 * 1024 * 1024
  })
  const exitStatus = result.status ?? (result.signal ? 1 : 0)
  const parsed = item.expectsJSON ? parseLastJSONObject(result.stdout || '') : { ok: true, value: undefined }
  const report = parsed.ok ? parsed.value : undefined
  const reportStatus = typeof report?.status === 'string' ? report.status : ''
  const passed = exitStatus === 0 && (!item.expectsJSON || reportStatus === 'passed')
  if (!passed && !jsonOutput) {
    if (result.stdout) process.stdout.write(result.stdout)
    if (result.stderr) process.stderr.write(result.stderr)
  }
  const check = {
    id: item.id,
    status: passed ? 'passed' : 'failed',
    command: commandText(item),
    durationMs: Date.now() - started,
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
  if (check.status !== 'passed') {
    failed = true
  }
}

const typeScriptRetirement = runTypeScriptRetirementScan(process.cwd())
checks.push({
  id: 'typescript-codepath-scan',
  status: typeScriptRetirement.passed ? 'passed' : 'failed',
  command: 'node scripts/runtime-go-ts-retirement-scan.mjs --json',
  durationMs: 0,
  exitStatus: typeScriptRetirement.passed ? 0 : 1,
  reportId: typeScriptRetirement.id,
  reportStatus: typeScriptRetirement.status,
  reason: typeScriptRetirement.passed ? '' : typeScriptRetirement.summary
})
if (!typeScriptRetirement.passed) failed = true

const cutover = reports['cutover-report'] || {}
const cutoverDetails = cutover.cutover && typeof cutover.cutover === 'object'
  ? cutover.cutover
  : {}
const passed = !skipCommands && !failed && checks.every((item) => item.status === 'passed')
const status = passed ? 'passed' : skipCommands ? 'skipped' : 'failed'
const report = {
  schemaVersion: 1,
  id: 'runtime-go-rollback-retirement-evidence',
  generatedAt: new Date().toISOString(),
  sourceCommit: currentGitCommit(),
  status,
  passed,
  rollback: {
    goDefaultCutoverReady: cutover.goRuntimeDefaultCutoverReady === true,
    runtimeHealthPassed: cutoverDetails.runtimeHealthPassed === true,
    runtimeHealthActualProcessStarted: cutoverDetails.runtimeHealthActualProcessStarted === true,
    runtimeHealthProductionRuntime: cutoverDetails.runtimeHealthProductionRuntime === true,
    runtimeHealthUsesTemporaryDataDir: cutoverDetails.runtimeHealthUsesTemporaryDataDir === true,
    runtimeHealthUsesRealProviderConfig: cutoverDetails.runtimeHealthUsesRealProviderConfig === true,
    runtimeHealthProviderConfigMode: typeof cutoverDetails.runtimeHealthProviderConfigMode === 'string'
      ? cutoverDetails.runtimeHealthProviderConfigMode
      : 'not-run',
    runtimeHealthUsesIsolatedEmptyProviderConfig: cutoverDetails.runtimeHealthUsesIsolatedEmptyProviderConfig === true,
    runtimeHealthRuntimeInfoOk: cutoverDetails.runtimeHealthRuntimeInfoOk === true,
    runtimeHealthRuntimeToolsOk: cutoverDetails.runtimeHealthRuntimeToolsOk === true,
    runtimeHealthProductionCapabilitiesOk: cutoverDetails.runtimeHealthProductionCapabilitiesOk === true,
    explicitTypeScriptOverrideRetained: false,
    typeScriptOverrideRetired: true,
    overrideEnv: 'ANALYTIX_RUNTIME_BACKEND=typescript',
    defaultBackendWhenUnset: 'go-runtime-default',
    defaultPathFallbackRetired: passed,
    typeScriptCodePathDeleted: typeScriptRetirement.typeScriptCodePathDeleted === true,
    typeScriptRuntimeSourceDeleted: typeScriptRetirement.typeScriptRuntimeSourceDeleted === true,
    typeScriptRetirementSummary: typeScriptRetirement.summary,
    productionForbiddenImportCount: typeScriptRetirement.productionForbiddenImportCount,
    packagedDistRuntimeModuleCount: typeScriptRetirement.packagedDistRuntimeModuleCount,
    packageExportViolationCount: typeScriptRetirement.packageExportViolationCount,
    buildConfigViolationCount: typeScriptRetirement.buildConfigViolationCount,
    sourceRuntimeImplementationCount: typeScriptRetirement.sourceRuntimeImplementationCount,
    productionForbiddenImports: typeScriptRetirement.productionForbiddenImports,
    packagedDistRuntimeModules: typeScriptRetirement.packagedDistRuntimeModules,
    packageExportViolations: typeScriptRetirement.packageExportViolations,
    buildConfigViolations: typeScriptRetirement.buildConfigViolations,
    sourceRuntimeImplementationSamples: typeScriptRetirement.sourceRuntimeImplementationSamples,
    productionGoSourceCount: cutoverDetails.productionGoSourceCount ?? null,
    retiredProductionMarkerViolationCount: cutoverDetails.retiredProductionMarkerViolationCount ?? null,
    legacyUpstreamFieldKeyViolationCount: cutoverDetails.legacyUpstreamFieldKeyViolationCount ?? null,
    retiredBackendCovered: checks.find((item) => item.id === 'typescript-retired-backend')?.status === 'passed',
    legacyChildRetiredCovered: checks.find((item) => item.id === 'legacy-child-retired')?.status === 'passed',
    analytixServeGoLauncherCovered: checks.find((item) => item.id === 'analytix-serve-go-launcher')?.status === 'passed',
    packagedGoBoundaryCovered: checks.find((item) => item.id === 'packaged-go-boundary')?.status === 'passed'
  },
  checks
}

if (jsonOutput) {
  console.log(JSON.stringify(report, null, 2))
} else {
  console.log(`${status.toUpperCase()} ${report.id}`)
  for (const item of checks.filter((check) => check.status !== 'passed')) {
    console.log(`FAILED ${item.id}: ${item.reason}`)
  }
}

if (!reportOnly && !passed) process.exitCode = 1
