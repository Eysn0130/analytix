#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { homedir, tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import process from 'node:process'

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const json = args.has('--json')
const repoRoot = resolve(new URL('..', import.meta.url).pathname)
const defaultSettingsPath = join(homedir(), 'Library/Application Support/Analytix/analytix-settings.json')
const defaultReasonixRepo = '/Users/sun/Projects/_upstreams/DeepSeek-Reasonix'

function argValue(name, fallback = '') {
  const prefix = `${name}=`
  const inline = rawArgs.find((item) => item.startsWith(prefix))
  if (inline) return inline.slice(prefix.length)
  const index = rawArgs.indexOf(name)
  return index >= 0 ? rawArgs[index + 1] || '' : fallback
}

function strings(value) {
  return typeof value === 'string' ? value.trim() : ''
}

function sha256(value) {
  return createHash('sha256').update(String(value)).digest('hex')
}

function parseLastJSONObject(text) {
  let start = text.lastIndexOf('{')
  while (start >= 0) {
    const candidate = text.slice(start).trim()
    try {
      const value = JSON.parse(candidate)
      if (value && typeof value === 'object' && !Array.isArray(value)) return value
    } catch {
      // npm wrappers may print logs before the final report.
    }
    start = text.lastIndexOf('{', start - 1)
  }
  return null
}

function redact(text, secrets = []) {
  let out = String(text || '')
  for (const secret of secrets) {
    const trimmed = strings(secret)
    if (trimmed) out = out.split(trimmed).join('<redacted>')
  }
  out = out.replace(/sk-[A-Za-z0-9_-]{12,}/g, 'sk-<redacted>')
  out = out.replace(/Bearer\s+[A-Za-z0-9._~+/-]+/gi, 'Bearer <redacted>')
  return out
}

function looksDeepSeekProvider(provider) {
  const parts = [
    provider?.id,
    provider?.name,
    provider?.baseUrl,
    ...(Array.isArray(provider?.models) ? provider.models : [])
  ]
  return parts.map(strings).join(' ').toLowerCase().includes('deepseek')
}

function resolveDeepSeekSettings(settingsPath) {
  if (!settingsPath || !existsSync(settingsPath)) {
    return { status: 'skipped', reason: 'settings-not-found' }
  }
  const settings = JSON.parse(readFileSync(settingsPath, 'utf8'))
  const providerSettings = settings.provider && typeof settings.provider === 'object' ? settings.provider : {}
  const runtime = settings.runtime && typeof settings.runtime === 'object' ? settings.runtime : {}
  const providers = Array.isArray(providerSettings.providers) ? providerSettings.providers : []
  const selectedProvider = providers.filter(looksDeepSeekProvider).find((item) => strings(item?.apiKey)) ||
    (strings(providerSettings.apiKey) ? { id: 'deepseek', apiKey: providerSettings.apiKey } : null) ||
    (strings(runtime.apiKey) ? { id: 'deepseek', apiKey: runtime.apiKey } : null)
  const apiKey = strings(selectedProvider?.apiKey)
  if (!apiKey) return { status: 'skipped', reason: 'deepseek-api-key-not-configured' }
  const configuredModel = strings(
    Array.isArray(selectedProvider?.models) ? selectedProvider.models.find((item) => strings(item)) : ''
  ) || strings(runtime.model)
  return {
    status: 'configured',
    apiKey,
    publicProvider: {
      providerId: strings(selectedProvider?.id) || 'deepseek',
      model: configuredModel,
      hasApiKey: true
    }
  }
}

function runCommand(command, commandArgs, options = {}) {
  const started = Date.now()
  const result = spawnSync(command, commandArgs, {
    cwd: options.cwd || repoRoot,
    env: options.env || process.env,
    stdio: 'pipe',
    encoding: 'utf8',
    timeout: options.timeoutMs || 120_000,
    maxBuffer: options.maxBuffer || 128 * 1024 * 1024
  })
  return {
    command: [command, ...commandArgs].join(' '),
    code: result.status ?? (result.signal ? 1 : 0),
    signal: result.signal || null,
    elapsedMs: Date.now() - started,
    stdout: result.stdout || '',
    stderr: result.stderr || '',
    error: result.error ? result.error.message : ''
  }
}

function runAnalytixLiveProbe(timeoutMs) {
  const run = runCommand(process.execPath, [
    './scripts/runtime-go-performance-check.mjs',
    '--json',
    '--live-deepseek-cache-from-settings'
  ], { cwd: repoRoot, timeoutMs })
  const report = parseLastJSONObject(run.stdout)
  const probe = report?.liveDeepSeekCache || {}
  return {
    status: run.code === 0 && report?.status === 'passed' && probe.status === 'passed' ? 'passed' : 'failed',
    command: run.command,
    elapsedMs: run.elapsedMs,
    reportStatus: report?.status || '',
    model: strings(probe.model),
    cacheHitObserved: probe.cacheHitObserved === true,
    cacheHitRate: Number.isFinite(Number(probe.cacheHitRate)) ? Number(probe.cacheHitRate) : null,
    calls: Array.isArray(probe.calls)
      ? probe.calls.map((call) => ({
          index: Number(call?.index || 0),
          ok: call?.ok === true,
          status: Number(call?.status || 0),
          elapsedMs: Number(call?.elapsedMs || 0),
          usage: call?.usage || {}
        }))
      : [],
    redaction: {
      secretsPrinted: false
    },
    failure: run.code === 0 && report ? '' : redact(`${run.stderr}\n${run.stdout}`.slice(-4000))
  }
}

function goCacheEnv(reasonixRepo) {
  const result = spawnSync('go', ['env', 'GOMODCACHE', 'GOCACHE'], {
    cwd: reasonixRepo,
    stdio: 'pipe',
    encoding: 'utf8'
  })
  const [goModCache, goCache] = String(result.stdout || '').trim().split(/\r?\n/)
  return {
    GOMODCACHE: strings(goModCache),
    GOCACHE: strings(goCache)
  }
}

function runReasonixProviderProbe(reasonixRepo, apiKey, timeoutMs) {
  const env = {
    ...process.env,
    DEEPSEEK_API_KEY: apiKey
  }
  const run = runCommand('go', [
    'test',
    './internal/provider/openai/',
    '-run',
    'TestRealDeepSeekCacheProbe',
    '-v',
    '-count=1'
  ], { cwd: reasonixRepo, env, timeoutMs })
  const output = redact(`${run.stdout}\n${run.stderr}`, [apiKey])
  const warm = output.match(/call 2 \(warm\): prompt=(\d+) hit=(\d+) miss=(\d+).*rate=(\d+)%/)
  const delta = output.match(/prompt_tokens delta \(with - without\) = (-?\d+)/)
  const withReasoning = output.match(/WITH reasoning_content: prompt=(\d+) hit=(\d+) miss=(\d+).*rate=(\d+)%/)
  const withoutReasoning = output.match(/WITHOUT \(stripped\):\s+prompt=(\d+) hit=(\d+) miss=(\d+).*rate=(\d+)%/)
  return {
    status: run.code === 0 ? 'passed' : 'failed',
    command: run.command,
    elapsedMs: run.elapsedMs,
    model: 'deepseek-v4-flash',
    warmCache: warm
      ? {
          promptTokens: Number(warm[1]),
          hitTokens: Number(warm[2]),
          missTokens: Number(warm[3]),
          ratePct: Number(warm[4])
        }
      : null,
    reasoningRoundTrip: {
      promptTokenDelta: delta ? Number(delta[1]) : null,
      withReasoning: withReasoning
        ? { promptTokens: Number(withReasoning[1]), hitTokens: Number(withReasoning[2]), missTokens: Number(withReasoning[3]), ratePct: Number(withReasoning[4]) }
        : null,
      withoutReasoning: withoutReasoning
        ? { promptTokens: Number(withoutReasoning[1]), hitTokens: Number(withoutReasoning[2]), missTokens: Number(withoutReasoning[3]), ratePct: Number(withoutReasoning[4]) }
        : null
    },
    outputDigest: sha256(output),
    outputTail: output.split(/\r?\n/).filter((line) => /Probe|call [12]|WITH|WITHOUT|delta|PASS|FAIL|WARNING|REJECTED|saw reasoning|prompt=|rate=|ok\s+reasonix/.test(line)).join('\n').slice(-4000)
  }
}

function runReasonixCliSpeedProbe(reasonixRepo, apiKey, timeoutMs) {
  const tempHome = mkdtempSync(join(tmpdir(), 'reasonix-live-home-'))
  const reasonixHome = join(tempHome, '.reasonix')
  const workspace = mkdtempSync(join(tmpdir(), 'reasonix-live-workspace-'))
  const cacheEnv = goCacheEnv(reasonixRepo)
  mkdirSync(reasonixHome, { recursive: true })
  writeFileSync(join(reasonixHome, '.env'), `DEEPSEEK_API_KEY=${JSON.stringify(apiKey)}\n`, { mode: 0o600 })
  const env = {
    ...process.env,
    HOME: tempHome,
    [['REASONIX', 'HOME'].join('_')]: reasonixHome,
    DEEPSEEK_API_KEY: apiKey,
    ...(cacheEnv.GOMODCACHE ? { GOMODCACHE: cacheEnv.GOMODCACHE } : {}),
    ...(cacheEnv.GOCACHE ? { GOCACHE: cacheEnv.GOCACHE } : {})
  }

  try {
    const runs = []
    for (const index of [1, 2]) {
      const metricsPath = join(tempHome, `metrics-${index}.json`)
      const run = runCommand('go', [
        'run',
        './cmd/reasonix',
        'run',
        '--model',
        'deepseek-pro',
        '--max-steps',
        '1',
        '--metrics',
        metricsPath,
        '--dir',
        workspace,
        'Reply with exactly: ok'
      ], { cwd: reasonixRepo, env, timeoutMs })
      const metrics = existsSync(metricsPath) ? JSON.parse(readFileSync(metricsPath, 'utf8')) : null
      runs.push({
        index,
        status: run.code === 0 ? 'passed' : 'failed',
        code: run.code,
        elapsedMs: run.elapsedMs,
        metrics,
        outputDigest: sha256(redact(`${run.stdout}\n${run.stderr}`, [apiKey])),
        outputTail: redact(`${run.stdout}\n${run.stderr}`, [apiKey]).slice(-1600)
      })
    }
    return {
      status: runs.every((item) => item.status === 'passed') ? 'passed' : 'failed',
      command: 'go run ./cmd/reasonix run --model deepseek-pro --max-steps 1 --metrics <temp> --dir <temp> "Reply with exactly: ok"',
      model: 'deepseek-pro',
      resolvedModel: 'deepseek-v4-pro',
      runs
    }
  } finally {
    rmSync(tempHome, { recursive: true, force: true })
    rmSync(workspace, { recursive: true, force: true })
  }
}

async function main() {
  const settingsPath = argValue('--settings-path', process.env.ANALYTIX_SETTINGS_PATH || defaultSettingsPath)
  const reasonixRepo = argValue('--reasonix-repo', process.env.ANALYTIX_UPSTREAM_REPO_REASONIX || defaultReasonixRepo)
  const timeoutMs = Number(argValue('--timeout-ms', process.env.ANALYTIX_UPSTREAM_LIVE_PARITY_TIMEOUT_MS || '120000')) || 120_000
  const settings = resolveDeepSeekSettings(settingsPath)
  if (settings.status !== 'configured') {
    const report = {
      schemaVersion: 1,
      id: 'runtime-go-reasonix-live-parity',
      generatedAt: new Date().toISOString(),
      status: 'skipped',
      reason: settings.reason,
      settingsPathPresent: Boolean(settingsPath && existsSync(settingsPath))
    }
    console.log(JSON.stringify(report, null, 2))
    return
  }
  if (!existsSync(reasonixRepo)) {
    const report = {
      schemaVersion: 1,
      id: 'runtime-go-reasonix-live-parity',
      generatedAt: new Date().toISOString(),
      status: 'failed',
      reason: 'reasonix-repo-not-found',
      reasonixRepo
    }
    console.log(JSON.stringify(report, null, 2))
    process.exitCode = 1
    return
  }

  const analytix = runAnalytixLiveProbe(timeoutMs)
  const reasonixProvider = runReasonixProviderProbe(reasonixRepo, settings.apiKey, timeoutMs)
  const reasonixCli = runReasonixCliSpeedProbe(reasonixRepo, settings.apiKey, timeoutMs)
  const analytixWarm = analytix.calls.find((item) => item.index === 2) || analytix.calls.at(-1) || {}
  const reasonixWarm = reasonixCli.runs.find((item) => item.index === 2) || reasonixCli.runs.at(-1) || {}
  const reasonixWarmMetrics = reasonixWarm.metrics || {}
  const reasonixPrompt = Number(reasonixWarmMetrics.prompt_tokens || 0)
  const reasonixHit = Number(reasonixWarmMetrics.cache_hit_tokens || 0)
  const reasonixMiss = Number(reasonixWarmMetrics.cache_miss_tokens || 0)
  const reasonixHitRate = reasonixHit + reasonixMiss > 0 ? reasonixHit / (reasonixHit + reasonixMiss) : null
  const speedParity = Number(analytixWarm.elapsedMs || 0) > 0 &&
    Number(reasonixWarm.elapsedMs || 0) > 0 &&
    Number(analytixWarm.elapsedMs) <= Number(reasonixWarm.elapsedMs) * 1.5

  const report = {
    schemaVersion: 1,
    id: 'runtime-go-reasonix-live-parity',
    generatedAt: new Date().toISOString(),
    status: analytix.status === 'passed' &&
      reasonixProvider.status === 'passed' &&
      reasonixCli.status === 'passed' &&
      analytix.cacheHitObserved === true &&
      reasonixProvider.warmCache?.hitTokens > 0 &&
      reasonixHit > 0 &&
      speedParity
      ? 'passed'
      : 'failed',
    settingsProvider: settings.publicProvider,
    analytix: {
      status: analytix.status,
      model: analytix.model,
      warmElapsedMs: Number(analytixWarm.elapsedMs || 0),
      cacheHitRate: analytix.cacheHitRate,
      cacheHitObserved: analytix.cacheHitObserved,
      calls: analytix.calls
    },
    reasonix: {
      providerProbe: reasonixProvider,
      cliProbe: {
        status: reasonixCli.status,
        model: reasonixCli.resolvedModel,
        warmElapsedMs: Number(reasonixWarm.elapsedMs || 0),
        warmCacheHitRate: reasonixHitRate,
        warmPromptTokens: reasonixPrompt,
        runs: reasonixCli.runs
      }
    },
    comparison: {
      analytixWarmElapsedMs: Number(analytixWarm.elapsedMs || 0),
      reasonixWarmElapsedMs: Number(reasonixWarm.elapsedMs || 0),
      speedParityThreshold: 'analytix <= reasonix * 1.5',
      speedParity,
      analytixWarmCacheHitRate: analytix.cacheHitRate,
      reasonixCliWarmCacheHitRate: reasonixHitRate,
      reasonixProviderWarmCacheHitRate: reasonixProvider.warmCache ? reasonixProvider.warmCache.ratePct / 100 : null
    },
    redaction: {
      secretsPrinted: false
    }
  }

  console.log(JSON.stringify(report, null, 2))
  process.exitCode = report.status === 'passed' ? 0 : 1
}

main().catch((error) => {
  console.log(JSON.stringify({
    schemaVersion: 1,
    id: 'runtime-go-reasonix-live-parity',
    generatedAt: new Date().toISOString(),
    status: 'failed',
    message: error instanceof Error ? error.message : String(error),
    redaction: { secretsPrinted: false }
  }, null, 2))
  process.exitCode = 1
})
