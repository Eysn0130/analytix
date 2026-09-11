#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import process from 'node:process'

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const nodeCommand = process.execPath
const jsonOutput = args.has('--json') || args.has('--dry-run')
const skipCommands = args.has('--skip-commands') || args.has('--dry-run')
const reportOnly = args.has('--no-gate')
const liveDeepSeekProbeRequested = args.has('--live-p5-from-settings') ||
  args.has('--live-deepseek-cache-from-settings')
const liveNonDeepSeekProbeRequested = args.has('--live-p5-from-settings') ||
  args.has('--live-non-deepseek-provider-from-settings')
const liveProbeRequested = liveDeepSeekProbeRequested || liveNonDeepSeekProbeRequested

const forwardedPerformanceArgs = rawArgs.filter((item) => ![
  '--json',
  '--gate',
  '--no-gate',
  '--dry-run',
  '--skip-commands'
].includes(item))
const performanceGateArgs = reportOnly ? [] : ['--gate']

const commands = [
  {
    id: 'provider-settings-resolution',
    command: nodeCommand,
    args: ['./scripts/runtime-go-performance-check.mjs', '--self-test-settings-resolution', '--json', '--gate'],
    expectsJSON: true
  },
  {
    id: 'default-readiness-report',
    command: npmCommand,
    args: ['run', 'runtime:go:default-readiness-report', '--', '--json'],
    expectsJSON: true
  },
  ...(liveProbeRequested ? [{
    id: 'live-provider-probes-command',
    command: nodeCommand,
    args: ['./scripts/runtime-go-performance-check.mjs', '--json', ...performanceGateArgs, ...forwardedPerformanceArgs],
    expectsJSON: true,
    allowPartial: reportOnly
  }] : [])
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
      // Child commands may emit npm logs before the final JSON report.
    }
    start = text.lastIndexOf('{', start - 1)
  }
  return { ok: false, reason: 'command did not emit a trailing JSON object' }
}

function runCheck(item) {
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-live-validation] ${item.id}`)
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
    maxBuffer: 128 * 1024 * 1024
  })
  const exitStatus = result.status ?? (result.signal ? 1 : 0)
  const parsed = item.expectsJSON ? parseLastJSONObject(result.stdout || '') : { ok: true, value: undefined }
  const report = parsed.ok ? parsed.value : undefined
  const reportStatus = typeof report?.status === 'string' ? report.status : ''
  const passed = exitStatus === 0 && (!item.expectsJSON || reportStatus === 'passed')
  const collectedPartial = !passed && item.allowPartial === true && exitStatus === 0 && (!item.expectsJSON || parsed.ok)
  if (!passed && !collectedPartial && !jsonOutput) {
    if (result.stdout) process.stdout.write(result.stdout)
    if (result.stderr) process.stderr.write(result.stderr)
  }
  const check = {
    id: item.id,
    status: passed || collectedPartial ? 'passed' : 'failed',
    command: commandText(item),
    durationMs: Date.now() - started,
    exitStatus,
    reportId: typeof report?.id === 'string' ? report.id : '',
    reportStatus,
    reason: collectedPartial ? '' : failureReason(item, result, parsed, exitStatus, reportStatus)
  }
  if (collectedPartial) check.collectionOnly = true
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

function summarizeLiveDeepSeekCacheProbe(report) {
  const probe = report?.liveDeepSeekCache
  if (!probe || typeof probe !== 'object') return null
  return {
    status: typeof probe.status === 'string' ? probe.status : 'unknown',
    provider: {
      activeProviderId: typeof probe.provider?.activeProviderId === 'string' ? probe.provider.activeProviderId : '',
      providerId: typeof probe.provider?.providerId === 'string' ? probe.provider.providerId : '',
      model: typeof probe.provider?.model === 'string' ? probe.provider.model : '',
      endpointFormat: typeof probe.provider?.endpointFormat === 'string' ? probe.provider.endpointFormat : '',
      hasApiKey: probe.provider?.hasApiKey === true,
      providerIsDeepSeek: probe.provider?.providerIsDeepSeek === true
    },
    model: typeof probe.model === 'string' ? probe.model : '',
    systemHash: typeof probe.systemHash === 'string' ? probe.systemHash : '',
    toolsHash: typeof probe.toolsHash === 'string' ? probe.toolsHash : '',
    prefixHash: typeof probe.prefixHash === 'string' ? probe.prefixHash : '',
    cacheHitObserved: probe.cacheHitObserved === true,
    cacheHitRate: Number.isFinite(Number(probe.cacheHitRate)) ? Number(probe.cacheHitRate) : null,
    calls: Array.isArray(probe.calls)
      ? probe.calls.map((call) => ({
          index: Number(call?.index || 0),
          ok: call?.ok === true,
          status: Number(call?.status || 0),
          elapsedMs: Number(call?.elapsedMs || 0),
          usage: {
            prompt_tokens: Number(call?.usage?.prompt_tokens || 0),
            completion_tokens: Number(call?.usage?.completion_tokens || 0),
            total_tokens: Number(call?.usage?.total_tokens || 0),
            prompt_cache_hit_tokens: Number(call?.usage?.prompt_cache_hit_tokens || 0),
            prompt_cache_miss_tokens: Number(call?.usage?.prompt_cache_miss_tokens || 0)
          }
        }))
      : []
  }
}

function summarizeLiveNonDeepSeekProviderProbe(report) {
  const probe = report?.liveNonDeepSeekProvider
  if (!probe || typeof probe !== 'object') return null
  return {
    status: typeof probe.status === 'string' ? probe.status : 'unknown',
    reason: typeof probe.reason === 'string' ? probe.reason : '',
    provider: probe.provider && typeof probe.provider === 'object'
      ? {
          activeProviderId: typeof probe.provider.activeProviderId === 'string' ? probe.provider.activeProviderId : '',
          providerId: typeof probe.provider.providerId === 'string' ? probe.provider.providerId : '',
          model: typeof probe.provider.model === 'string' ? probe.provider.model : '',
          endpointFormat: typeof probe.provider.endpointFormat === 'string' ? probe.provider.endpointFormat : '',
          hasApiKey: probe.provider.hasApiKey === true,
          providerIsDeepSeek: probe.provider.providerIsDeepSeek === true
        }
      : null,
    providerResponded: probe.providerResponded === true,
    statusCode: Number(probe.statusCode || 0),
    elapsedMs: Number(probe.elapsedMs || 0),
    usage: probe.usage && typeof probe.usage === 'object'
      ? {
          prompt_tokens: Number(probe.usage.prompt_tokens || 0),
          completion_tokens: Number(probe.usage.completion_tokens || 0),
          total_tokens: Number(probe.usage.total_tokens || 0),
          cached_tokens: Number(probe.usage.cached_tokens || 0),
          cache_read_input_tokens: Number(probe.usage.cache_read_input_tokens || 0),
          cache_creation_input_tokens: Number(probe.usage.cache_creation_input_tokens || 0)
        }
      : null
  }
}

function liveProbePassed(summary) {
  if (!summary) return null
  if (summary.status === 'skipped') return null
  return summary.status === 'passed'
}

function derivedLiveProbeCheck(id, summary, requested) {
  if (!requested || skipCommands) return null
  const summaryStatus = typeof summary?.status === 'string' ? summary.status : ''
  const passed = summaryStatus === 'passed'
  const skipped = summaryStatus === 'skipped'
  const status = passed ? 'passed' : skipped ? 'skipped' : reportOnly ? 'partial' : 'failed'
  return {
    id,
    status,
    command: commandText(commands.find((item) => item.id === 'live-provider-probes-command') || {
      command: nodeCommand,
      args: ['./scripts/runtime-go-performance-check.mjs']
    }),
    durationMs: 0,
    reportStatus: summaryStatus,
    summary: summary || null,
    reason: passed || skipped ? '' : (summary?.reason || `${id} did not pass`)
  }
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
    break
  }
}

const candidate = reports['default-readiness-report'] || {}
const settingsResolution = reports['provider-settings-resolution'] || {}
const liveProbe = reports['live-provider-probes-command'] || undefined
const liveDeepSeekCacheSummary = summarizeLiveDeepSeekCacheProbe(liveProbe)
const liveNonDeepSeekProviderSummary = summarizeLiveNonDeepSeekProviderProbe(liveProbe)
const liveDeepSeekCacheProbePassed = liveProbePassed(liveDeepSeekCacheSummary)
const liveNonDeepSeekProviderProbePassed = liveProbePassed(liveNonDeepSeekProviderSummary)
for (const derived of [
  derivedLiveProbeCheck('live-deepseek-cache-probe', liveDeepSeekCacheSummary, liveDeepSeekProbeRequested),
  derivedLiveProbeCheck('live-non-deepseek-provider-probe', liveNonDeepSeekProviderSummary, liveNonDeepSeekProbeRequested)
]) {
  if (derived) checks.push(derived)
}
const failedAfterDerived = failed || checks.some((item) => item.status === 'failed')
const passed = !skipCommands && !failedAfterDerived && checks.every((item) => item.status === 'passed')
const status = passed ? 'passed' : skipCommands ? 'skipped' : failedAfterDerived ? 'failed' : 'partial'
const report = {
  schemaVersion: 1,
  id: 'runtime-go-live-validation',
  generatedAt: new Date().toISOString(),
  sourceCommit: currentGitCommit(),
  status,
  passed,
  live: {
    providerSettingsResolutionPassed: settingsResolution.status === 'passed',
    runtimeHealthPassed: candidate.runtimeHealth?.passed === true,
    runtimeHealthRuntimeInfoOk: candidate.runtimeHealth?.runtimeInfoOk === true,
    runtimeHealthRuntimeToolsOk: candidate.runtimeHealth?.runtimeToolsOk === true,
    runtimeHealthProductionCapabilitiesOk: candidate.runtimeHealth?.productionCapabilitiesOk === true,
    productRegressionPassed: candidate.productRegression?.passed === true,
    speedCacheGatePassed: candidate.speedCacheGate?.passed === true,
    liveProbeRequested,
    liveDeepSeekCacheProbePassed,
    liveDeepSeekCacheSummary,
    liveNonDeepSeekProviderProbePassed,
    liveNonDeepSeekProviderSummary,
    secretsPrinted: false
  },
  checks
}

if (jsonOutput) {
  console.log(JSON.stringify(report, null, 2))
} else {
  console.log(`${status.toUpperCase()} ${report.id}`)
  console.log(`live probe requested: ${liveProbeRequested}`)
  for (const item of checks.filter((check) => check.status !== 'passed')) {
    console.log(`FAILED ${item.id}: ${item.reason}`)
  }
}

if (!reportOnly && !passed) process.exitCode = 1
