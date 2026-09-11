#!/usr/bin/env node

import http from 'node:http'
import { spawn, spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { arch, cpus, homedir, platform, tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { performance } from 'node:perf_hooks'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const repoRoot = resolve(here, '..')
const READY_PREFIX = 'ANALYTIX_RUNTIME_SERVER_READY '
const DEFAULT_TIMEOUT_MS = 30_000
const DEFAULT_APP_PATH = join(repoRoot, 'dist/mac-arm64/analytix.app')
const MAIN_SSE_EVENT_BATCH_MS = 16
const RC_WARMUP_COUNT = 2
const RC_MEASURED_COUNT = 20
const RC_FIRST_EVENT_P95_MS = 250
const TOKEN_PLAN_PROVIDER_ID_SUFFIX = '-token-plan'
const STATIC_MODEL_PROVIDER_PRESET_DEFAULTS = {
  litellm: {
    baseUrl: 'http://localhost:4000',
    endpointFormat: 'chat_completions',
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
    endpointFormat: 'chat_completions',
    models: ['qwen-max'],
    modelEndpointFormats: {}
  },
  'moonshot-cn': {
    baseUrl: 'https://api.moonshot.cn/v1',
    endpointFormat: 'chat_completions',
    models: ['kimi-k2.7-code'],
    modelEndpointFormats: {}
  },
  'moonshot-global': {
    baseUrl: 'https://api.moonshot.ai/v1',
    endpointFormat: 'chat_completions',
    models: ['kimi-k2.7-code'],
    modelEndpointFormats: {}
  },
  'zai-coding-plan': {
    baseUrl: 'https://api.z.ai/api/coding/paas/v4/chat/completions',
    endpointFormat: 'custom_endpoint',
    models: ['glm-5.1'],
    modelEndpointFormats: {}
  },
  'zhipu-coding-plan': {
    baseUrl: 'https://open.bigmodel.cn/api/coding/paas/v4/chat/completions',
    endpointFormat: 'custom_endpoint',
    models: ['glm-5.1'],
    modelEndpointFormats: {}
  }
}

const args = new Set(process.argv.slice(2))
const json = args.has('--json')
const gate = args.has('--gate')

function argValue(name, fallback = '') {
  const rawArgs = process.argv.slice(2)
  const prefix = `${name}=`
  const inline = rawArgs.find((item) => item.startsWith(prefix))
  if (inline) return inline.slice(prefix.length)
  const index = rawArgs.indexOf(name)
  return index >= 0 ? rawArgs[index + 1] || '' : fallback
}

function sha256(value) {
  return createHash('sha256').update(String(value)).digest('hex')
}

function nowMs() {
  return performance.now()
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function runtimeServerBinaryPath(appPath) {
  return join(appPath, 'Contents', 'Resources', 'runtime-go', 'bin', 'runtime-server')
}

async function listen(server) {
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('server did not bind to TCP')
  return `http://127.0.0.1:${address.port}`
}

async function readJSONBody(req) {
  let body = ''
  for await (const chunk of req) body += String(chunk)
  try {
    return body.trim() ? JSON.parse(body) : {}
  } catch {
    return {}
  }
}

function bodySummary(body) {
  const fields = body && typeof body === 'object' ? Object.keys(body).sort() : []
  return {
    fields,
    bodyDigest: sha256(JSON.stringify(body)),
    model: typeof body.model === 'string' ? body.model : '',
    messageCount: Array.isArray(body.messages) ? body.messages.length : 0,
    stream: body?.stream === true,
    streamOptionsIncludeUsage: body?.stream_options?.include_usage === true,
    hasThinking: body && Object.prototype.hasOwnProperty.call(body, 'thinking'),
    hasReasoningEffort: body && Object.prototype.hasOwnProperty.call(body, 'reasoning_effort'),
    toolCount: Array.isArray(body?.tools) ? body.tools.length : 0
  }
}

function startStreamingProvider({ firstDelayMs, finishDelayMs }) {
  const requests = []
  const timings = {
    requestStartedAt: 0,
    firstFrameAt: 0,
    doneFrameAt: 0
  }
  const server = http.createServer(async (req, res) => {
    const body = await readJSONBody(req)
    timings.requestStartedAt = nowMs()
    requests.push({
      url: req.url || '',
      authorizationConfigured: Boolean(req.headers.authorization || req.headers['x-api-key']),
      ...bodySummary(body)
    })
    if (!body.model) {
      res.writeHead(400, { 'content-type': 'application/json' })
      res.end(JSON.stringify({ error: { message: 'intentional local performance check error' } }))
      return
    }
    res.writeHead(200, {
      'content-type': 'text/event-stream; charset=utf-8',
      'cache-control': 'no-cache, no-transform',
      connection: 'keep-alive'
    })
    res.flushHeaders?.()
    await sleep(firstDelayMs)
    timings.firstFrameAt = nowMs()
    res.write(`data: ${JSON.stringify({
      choices: [{ delta: { content: 'performance first token' }, finish_reason: null }]
    })}\n\n`)
    await sleep(finishDelayMs)
    res.write(`data: ${JSON.stringify({
      choices: [{ delta: { content: ' and final token' }, finish_reason: null }]
    })}\n\n`)
    res.write(`data: ${JSON.stringify({
      choices: [{ delta: {}, finish_reason: 'stop' }],
      usage: {
        prompt_tokens: 100,
        completion_tokens: 8,
        total_tokens: 108,
        prompt_cache_hit_tokens: 64,
        prompt_cache_miss_tokens: 36
      }
    })}\n\n`)
    timings.doneFrameAt = nowMs()
    res.write('data: [DONE]\n\n')
    res.end()
  })
  return { server, requests, timings }
}

function waitForReady(child, timeoutMs) {
  return new Promise((resolve, reject) => {
    let output = ''
    const timer = setTimeout(() => reject(new Error(`timeout waiting for runtime ready: ${output.slice(-1000)}`)), timeoutMs)
    const cleanup = () => {
      clearTimeout(timer)
      child.stdout?.off('data', onStdout)
      child.stderr?.off('data', onStderr)
      child.off('exit', onExit)
      child.off('error', onError)
    }
    const onStdout = (chunk) => {
      output += String(chunk)
      const index = output.indexOf(READY_PREFIX)
      if (index < 0) return
      const line = output.slice(index + READY_PREFIX.length).split(/\r?\n/)[0]
      cleanup()
      resolve(JSON.parse(line))
    }
    const onStderr = (chunk) => {
      output += String(chunk)
    }
    const onExit = (code, signal) => {
      cleanup()
      reject(new Error(`runtime exited before ready: ${code ?? signal}; ${output.slice(-1000)}`))
    }
    const onError = (error) => {
      cleanup()
      reject(new Error(`runtime failed to start: ${error instanceof Error ? error.message : String(error)}`))
    }
    child.stdout?.on('data', onStdout)
    child.stderr?.on('data', onStderr)
    child.once('exit', onExit)
    child.once('error', onError)
  })
}

function runtimeLaunchCommand({ appPath, runtimeBin, tempRoot, preferPackagedBinary = false, explicitRuntimeBin = '' }) {
  const explicitBin = strings(explicitRuntimeBin || argValue('--runtime-bin', process.env.ANALYTIX_RUNTIME_GO_PERF_BIN || ''))
  if (explicitBin) {
    return { mode: 'binary', command: explicitBin, cwd: repoRoot }
  }
  if (preferPackagedBinary && existsSync(runtimeBin)) {
    return { mode: 'packaged-binary', command: runtimeBin, cwd: repoRoot }
  }
  if (args.has('--no-go-run')) {
    throw new Error(`packaged runtime-server binary is missing: ${runtimeBin}`)
  }
  const runtimeGoRoot = join(repoRoot, 'packages/runtime-go')
  const builtRuntime = join(tempRoot, process.platform === 'win32' ? 'runtime-server.exe' : 'runtime-server')
  const build = spawnSync(
    process.env.GO || 'go',
    ['build', '-tags', 'analytix_prod', '-o', builtRuntime, './cmd/runtime-server'],
    { cwd: runtimeGoRoot, encoding: 'utf8' }
  )
  if (build.status !== 0) {
    const output = `${build.stdout || ''}\n${build.stderr || ''}`.trim()
    throw new Error(`failed to build runtime-server performance fixture: ${output.slice(-1200)}`)
  }
  if (!existsSync(builtRuntime)) {
    throw new Error('runtime-server performance fixture build succeeded without producing the binary')
  }
  return {
    mode: 'go-build-production-tag',
    command: builtRuntime,
    prefixArgs: [],
    cwd: runtimeGoRoot,
    appPath
  }
}

function strings(value) {
  return String(value || '').trim()
}

function isDeepSeekLikeProvider(provider = {}, model = '') {
  const joined = [
    provider.id,
    provider.name,
    provider.baseUrl,
    provider.endpointFormat,
    model,
    ...(Array.isArray(provider.models) ? provider.models : [])
  ].map((value) => String(value || '').toLowerCase()).join(' ')
  return joined.includes('deepseek') || joined.includes('api.deepseek.com')
}

function normalizeEndpointFormat(value) {
  const normalized = strings(value).toLowerCase().replaceAll('-', '_').replace(/^\/+|\/+$/g, '')
  switch (normalized) {
    case '':
    case 'chat':
    case 'chat_completions':
    case 'chat/completions':
    case 'v1/chat/completions':
      return 'chat_completions'
    case 'response':
    case 'responses':
    case 'v1/responses':
      return 'responses'
    case 'message':
    case 'messages':
    case 'v1/messages':
      return 'messages'
    case 'custom':
    case 'custom_endpoint':
    case 'custom_full_path':
    case 'full_path':
    case 'full_url':
      return 'custom_endpoint'
    default:
      return 'chat_completions'
  }
}

function readSettingsJSON(settingsPath) {
  return JSON.parse(readFileSync(settingsPath, 'utf8'))
}

function providerProfileEndpointFormat(provider, model, fallbackEndpointFormat = '', fallbackModelEndpointFormats = {}) {
  const endpointFormat = normalizeEndpointFormat(provider?.endpointFormat || fallbackEndpointFormat)
  const profile = provider?.modelProfiles && typeof provider.modelProfiles === 'object'
    ? provider.modelProfiles[model]
    : undefined
  return normalizeEndpointFormat(profile?.endpointFormat || fallbackModelEndpointFormats[model] || endpointFormat)
}

function normalizeDeprecatedProviderModel(providerID, model) {
  const id = strings(providerID).replace(/-token-plan$/, '')
  const normalized = strings(model)
  if (id === 'xiaomi' && normalized.toLowerCase() === 'mimo-v2.5-pro-ultraspeed') {
    return 'mimo-v2.5-pro'
  }
  return normalized
}

function modelForProvider(provider, runtime = {}, fallbackModels = []) {
  const providerID = strings(provider?.id)
  if (strings(runtime.providerId) === providerID && strings(runtime.model)) {
    return normalizeDeprecatedProviderModel(providerID, runtime.model)
  }
  return normalizeDeprecatedProviderModel(providerID, (Array.isArray(provider?.models) ? strings(provider.models.find((item) => strings(item))) : '') ||
    strings(fallbackModels.find((item) => strings(item))) ||
    strings(runtime.model) ||
    'deepseek-chat')
}

function providerConfigFromProfile(provider, runtime = {}, providerSettings = {}, selectedProviderId = '') {
  const providerID = strings(provider?.id || selectedProviderId || 'deepseek')
  const preset = modelProviderPresetDefaults(providerID)
  const model = modelForProvider(provider, runtime, preset.models)
  const apiKey = strings(provider?.apiKey || (providerID === 'deepseek' ? providerSettings.apiKey : '') || runtime.apiKey)
  const defaultBaseUrl = providerID === 'deepseek' ? 'https://api.deepseek.com' : ''
  const baseUrl = strings(provider?.baseUrl || preset.baseUrl || (providerID === 'deepseek' ? providerSettings.baseUrl : '') || runtime.baseUrl || defaultBaseUrl).replace(/\/+$/, '')
  return {
    apiKey,
    baseUrl,
    providerId: providerID,
    model,
    endpointFormat: providerProfileEndpointFormat(provider, model, preset.endpointFormat, preset.modelEndpointFormats),
    providerIsDeepSeek: isDeepSeekLikeProvider(provider, model),
    hasApiKey: Boolean(apiKey)
  }
}

let modelProviderPresetDefaultsCache

function modelProviderPresetDefaults(providerId) {
  const id = strings(providerId)
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
      endpointFormat: normalizeEndpointFormat(preset.endpointFormat),
      models: [...preset.models],
      modelEndpointFormats: { ...(preset.modelEndpointFormats || {}) }
    })
  }
  try {
    const source = readFileSync(resolve(repoRoot, 'src/shared/model-provider-presets.ts'), 'utf8')
    const constArrays = parseStringArrayConstants(source)
    const presetPattern = /id:\s*'([^']+)'[\s\S]*?name:\s*'([^']*)'[\s\S]*?baseUrl:\s*'([^']*)'[\s\S]*?endpointFormat:\s*'([^']*)'[\s\S]*?models:\s*\[([\s\S]*?)\]/
    for (const objectSource of extractTopLevelPresetObjects(source)) {
      const match = objectSource.match(presetPattern)
      if (!match) continue
      const id = strings(match[1])
      if (!id) continue
      defaults.set(id, {
        baseUrl: strings(match[3]),
        endpointFormat: normalizeEndpointFormat(match[4]),
        models: extractStringArrayModels(match[5], constArrays),
        modelEndpointFormats: parsePresetModelEndpointFormats(objectSource)
      })
      const tokenPlan = parseTokenPlanPresetDefaults(objectSource, constArrays)
      if (tokenPlan) {
        defaults.set(`${id}${TOKEN_PLAN_PROVIDER_ID_SUFFIX}`, tokenPlan)
      }
    }
  } catch {
    // Explicit env/settings still work if the product preset source is unavailable.
  }
  return defaults
}

function parseTokenPlanPresetDefaults(objectSource, constArrays) {
  const tokenPlanPattern = /tokenPlan:\s*\{[\s\S]*?baseUrl:\s*'([^']*)'[\s\S]*?endpointFormat:\s*'([^']*)'[\s\S]*?models:\s*\[([\s\S]*?)\]/
  const match = String(objectSource || '').match(tokenPlanPattern)
  if (!match) return null
  return {
    baseUrl: strings(match[1]),
    endpointFormat: normalizeEndpointFormat(match[2]),
    models: extractStringArrayModels(match[3], constArrays),
    modelEndpointFormats: parsePresetModelEndpointFormats(String(objectSource || '').slice(String(objectSource || '').indexOf('tokenPlan:')))
  }
}

function parsePresetModelEndpointFormats(objectSource) {
  const formats = {}
  const source = String(objectSource || '')
  const pattern = /'([^']+)'\s*:\s*(?:textChatProfile|visionChatProfile)\([^)]*'([^']+)'\s*\)/g
  for (const match of source.matchAll(pattern)) {
    const model = strings(match[1])
    const endpointFormat = normalizeEndpointFormat(match[2])
    if (model && endpointFormat) formats[model] = endpointFormat
  }
  return formats
}

function parseStringArrayConstants(source) {
  const arrays = new Map()
  const constArrayPattern = /const\s+([A-Z0-9_]+)\s*=\s*\[([\s\S]*?)\]/g
  for (const match of source.matchAll(constArrayPattern)) {
    const name = strings(match[1])
    if (!name) continue
    arrays.set(name, [...match[2].matchAll(/'([^']+)'/g)].map((item) => strings(item[1])).filter(Boolean))
  }
  return arrays
}

function extractStringArrayModels(arraySource, constArrays) {
  const models = []
  for (const match of String(arraySource || '').matchAll(/\.\.\.([A-Z0-9_]+)|'([^']+)'/g)) {
    const spreadName = strings(match[1])
    if (spreadName) {
      models.push(...(constArrays.get(spreadName) || []))
      continue
    }
    const model = strings(match[2])
    if (model) models.push(model)
  }
  return models
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
      if (escaped) escaped = false
      else if (char === '\\') escaped = true
      else if (char === stringQuote) stringQuote = ''
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

function resolveSettingsProviderContext(settingsPath) {
  const settings = readSettingsJSON(settingsPath)
  const providerSettings = settings.provider && typeof settings.provider === 'object' ? settings.provider : {}
  const runtime = settings.runtime && typeof settings.runtime === 'object' ? settings.runtime : {}
  const providers = Array.isArray(providerSettings.providers) ? providerSettings.providers : []
  const activeProviderId = strings(providerSettings.activeProviderId)
  const selectedProviderId = strings(runtime.providerId) || activeProviderId || 'deepseek'
  return { settings, providerSettings, runtime, providers, activeProviderId, selectedProviderId }
}

function resolveActiveProviderFromSettings(settingsPath) {
  const { providerSettings, runtime, providers, activeProviderId, selectedProviderId } = resolveSettingsProviderContext(settingsPath)
  const bySelectedId = providers.find((item) => strings(item?.id) === selectedProviderId)
  const fallbackDeepSeek = providers.find((item) => isDeepSeekLikeProvider(item, runtime.model))
  const provider = bySelectedId || fallbackDeepSeek || providers[0] || {
    id: 'deepseek',
    name: 'DeepSeek',
    apiKey: providerSettings.apiKey || runtime.apiKey || '',
    baseUrl: providerSettings.baseUrl || runtime.baseUrl || 'https://api.deepseek.com',
    endpointFormat: runtime.endpointFormat || 'chat_completions',
    models: runtime.model ? [runtime.model] : ['deepseek-chat'],
    modelProfiles: {}
  }
  const config = providerConfigFromProfile(provider, runtime, providerSettings, selectedProviderId)
  return {
    ...config,
    activeProviderId,
    providerCount: providers.length
  }
}

function resolveLiveNonDeepSeekProviderFromSettings(settingsPath) {
  if (!settingsPath || !existsSync(settingsPath)) {
    return { status: 'not_configured', reason: 'settings-not-found', provider: null }
  }
  const context = resolveSettingsProviderContext(settingsPath)
  const candidates = context.providers
    .map((provider) => ({
      provider,
      config: providerConfigFromProfile(provider, context.runtime, context.providerSettings, strings(provider?.id))
    }))
    .filter((item) => !item.config.providerIsDeepSeek)
  if (candidates.length === 0) {
    return { status: 'not_configured', reason: 'non-deepseek-provider-not-configured', provider: null }
  }
  const selected = candidates.find((item) => liveNonDeepSeekCandidateCanProbe(item.config)) ||
    candidates.find((item) => item.config.hasApiKey && strings(item.config.baseUrl) && strings(item.config.model)) ||
    candidates.find((item) => item.config.hasApiKey) ||
    candidates[0]
  return {
    status: selected.config.hasApiKey ? 'configured' : 'missing_api_key',
    reason: selected.config.hasApiKey ? '' : 'missing-api-key',
    provider: {
      ...selected.config,
      activeProviderId: context.activeProviderId || context.selectedProviderId,
      providerCount: context.providers.length
    }
  }
}

function resolveLiveDeepSeekProviderFromSettings(settingsPath) {
  if (!settingsPath || !existsSync(settingsPath)) {
    return { status: 'not_configured', reason: 'settings-not-found', provider: null }
  }
  const context = resolveSettingsProviderContext(settingsPath)
  const candidates = context.providers
    .map((provider) => ({
      provider,
      config: providerConfigFromProfile(provider, context.runtime, context.providerSettings, strings(provider?.id))
    }))
    .filter((item) => item.config.providerIsDeepSeek)
  if (candidates.length === 0) {
    return { status: 'not_configured', reason: 'deepseek-provider-not-configured', provider: null }
  }
  const selected = candidates.find((item) => liveDeepSeekCandidateCanProbe(item.config)) ||
    candidates.find((item) => item.config.hasApiKey && strings(item.config.baseUrl) && strings(item.config.model)) ||
    candidates.find((item) => item.config.hasApiKey) ||
    candidates[0]
  const configured = liveDeepSeekCandidateCanProbe(selected.config)
  return {
    status: configured
      ? 'configured'
      : selected.config.hasApiKey
        ? 'missing_base_url_or_model'
        : 'missing_api_key',
    reason: configured
      ? ''
      : selected.config.hasApiKey
        ? 'missing-base-url-or-model'
        : 'missing-api-key',
    provider: {
      ...selected.config,
      activeProviderId: context.activeProviderId || context.selectedProviderId,
      providerCount: context.providers.length
    }
  }
}

function liveNonDeepSeekCandidateCanProbe(config) {
  return config.hasApiKey === true &&
    Boolean(strings(config.baseUrl)) &&
    Boolean(strings(config.model)) &&
    liveProviderRequestConfig(config).supported === true
}

function liveDeepSeekCandidateCanProbe(config) {
  return config.hasApiKey === true &&
    Boolean(strings(config.baseUrl)) &&
    Boolean(strings(config.model)) &&
    config.providerIsDeepSeek === true
}

function publicProviderSummary(config) {
  const providerId = strings(config.providerId)
  return {
    activeProviderId: strings(config.activeProviderId) || providerId,
    providerId,
    model: config.model,
    endpointFormat: config.endpointFormat,
    hasApiKey: config.hasApiKey === true,
    providerIsDeepSeek: config.providerIsDeepSeek === true
  }
}

function buildSettingsResolutionSelfTestReport() {
  const tempRoot = mkdtempSync(join(tmpdir(), 'analytix-provider-settings-'))
  const settingsPath = join(tempRoot, 'settings.json')
  const secret = 'sk-self-test-settings-resolution'
  const overrideSecret = 'sk-self-test-runtime-provider-override'
  try {
    writeFileSync(settingsPath, JSON.stringify({
      provider: {
        activeProviderId: 'deepseek',
        apiKey: '',
        baseUrl: '',
        providers: [
          {
            id: 'openai-compatible',
            name: 'OpenAI compatible',
            apiKey: 'sk-other-provider',
            baseUrl: 'https://openai-compatible.example/v1',
            endpointFormat: 'chat_completions',
            models: ['gpt-compatible']
          },
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: secret,
            baseUrl: 'https://api.deepseek.com',
            endpointFormat: 'chat_completions',
            models: ['deepseek-chat']
          }
        ]
      },
      runtime: {
        providerId: '',
        model: 'deepseek-chat',
        apiKey: '',
        baseUrl: ''
      }
    }, null, 2))
    const config = resolveActiveProviderFromSettings(settingsPath)
    const summary = publicProviderSummary(config)
    const serialized = JSON.stringify(summary)
    const activeProviderPassed = config.apiKey === secret &&
      summary.hasApiKey === true &&
      summary.activeProviderId === 'deepseek' &&
      summary.providerId === 'deepseek' &&
      summary.model === 'deepseek-chat' &&
      summary.endpointFormat === 'chat_completions' &&
      !serialized.includes(secret) &&
      !serialized.includes('sk-other-provider')

    writeFileSync(settingsPath, JSON.stringify({
      provider: {
        activeProviderId: '',
        apiKey: '',
        baseUrl: '',
        providers: [
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: secret,
            baseUrl: 'https://api.deepseek.com',
            endpointFormat: 'chat_completions',
            models: ['deepseek-chat']
          }
        ]
      },
      runtime: {
        providerId: '',
        model: 'deepseek-chat',
        apiKey: '',
        baseUrl: ''
      }
    }, null, 2))
    const missingActiveSummary = publicProviderSummary(resolveActiveProviderFromSettings(settingsPath))
    const missingActiveFallbackPassed = missingActiveSummary.activeProviderId === 'deepseek' &&
      missingActiveSummary.providerId === 'deepseek' &&
      missingActiveSummary.hasApiKey === true

    writeFileSync(settingsPath, JSON.stringify({
      provider: {
        activeProviderId: 'custom',
        apiKey: '',
        baseUrl: '',
        providers: [
          {
            id: 'custom',
            name: 'Custom provider',
            apiKey: 'sk-custom-provider',
            baseUrl: 'https://custom.example/v1',
            endpointFormat: 'messages',
            models: ['custom-chat']
          },
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: overrideSecret,
            baseUrl: 'https://api.deepseek.com',
            endpointFormat: 'chat_completions',
            models: ['deepseek-chat']
          }
        ]
      },
      runtime: {
        providerId: 'deepseek',
        model: 'deepseek-chat',
        apiKey: '',
        baseUrl: ''
      }
    }, null, 2))
    const overrideConfig = resolveActiveProviderFromSettings(settingsPath)
    const overrideSummary = publicProviderSummary(overrideConfig)
    const overrideSerialized = JSON.stringify(overrideSummary)
    const runtimeOverridePassed = overrideConfig.apiKey === overrideSecret &&
      overrideSummary.hasApiKey === true &&
      overrideSummary.activeProviderId === 'custom' &&
      overrideSummary.providerId === 'deepseek' &&
      overrideSummary.model === 'deepseek-chat' &&
      overrideSummary.endpointFormat === 'chat_completions' &&
      !overrideSerialized.includes(overrideSecret) &&
      !overrideSerialized.includes('sk-custom-provider')
    const nonDeepSeek = resolveLiveNonDeepSeekProviderFromSettings(settingsPath)
    const nonDeepSeekSummary = nonDeepSeek.provider ? publicProviderSummary(nonDeepSeek.provider) : {}
    const nonDeepSeekSerialized = JSON.stringify(nonDeepSeekSummary)
    const nonDeepSeekPassed = nonDeepSeek.status === 'configured' &&
      nonDeepSeek.provider?.apiKey === 'sk-custom-provider' &&
      nonDeepSeekSummary.hasApiKey === true &&
      nonDeepSeekSummary.providerId === 'custom' &&
      nonDeepSeekSummary.model === 'custom-chat' &&
      nonDeepSeekSummary.endpointFormat === 'messages' &&
      nonDeepSeekSummary.providerIsDeepSeek === false &&
      !nonDeepSeekSerialized.includes('sk-custom-provider') &&
      !nonDeepSeekSerialized.includes(overrideSecret)

    writeFileSync(settingsPath, JSON.stringify({
      provider: {
        activeProviderId: 'moonshot-cn',
        apiKey: '',
        baseUrl: '',
        providers: [
          {
            id: 'moonshot-cn',
            name: 'Moonshot CN',
            apiKey: 'sk-moonshot-provider',
            baseUrl: '',
            endpointFormat: '',
            models: []
          },
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: secret,
            baseUrl: 'https://api.deepseek.com',
            endpointFormat: 'chat_completions',
            models: ['deepseek-chat']
          }
        ]
      },
      runtime: {
        providerId: 'moonshot-cn',
        model: '',
        apiKey: '',
        baseUrl: ''
      }
    }, null, 2))
    const presetNonDeepSeek = resolveLiveNonDeepSeekProviderFromSettings(settingsPath)
    const presetNonDeepSeekSummary = presetNonDeepSeek.provider ? publicProviderSummary(presetNonDeepSeek.provider) : {}
    const presetNonDeepSeekSerialized = JSON.stringify(presetNonDeepSeekSummary)
    const presetNonDeepSeekPassed = presetNonDeepSeek.status === 'configured' &&
      presetNonDeepSeek.provider?.apiKey === 'sk-moonshot-provider' &&
      presetNonDeepSeek.provider?.baseUrl === 'https://api.moonshot.cn/v1' &&
      presetNonDeepSeekSummary.hasApiKey === true &&
      presetNonDeepSeekSummary.providerId === 'moonshot-cn' &&
      presetNonDeepSeekSummary.model === 'kimi-k2.7-code' &&
      presetNonDeepSeekSummary.endpointFormat === 'chat_completions' &&
      presetNonDeepSeekSummary.providerIsDeepSeek === false &&
      !presetNonDeepSeekSerialized.includes('sk-moonshot-provider') &&
      !presetNonDeepSeekSerialized.includes(secret)

    writeFileSync(settingsPath, JSON.stringify({
      provider: {
        activeProviderId: 'stale-openai',
        apiKey: '',
        baseUrl: '',
        providers: [
          {
            id: 'stale-openai',
            name: 'Stale OpenAI compatible',
            apiKey: 'sk-stale-provider',
            baseUrl: '',
            endpointFormat: 'chat_completions',
            models: ['stale-model']
          },
          {
            id: 'moonshot-cn',
            name: 'Moonshot CN',
            apiKey: 'sk-moonshot-provider',
            baseUrl: '',
            endpointFormat: '',
            models: []
          },
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: secret,
            baseUrl: 'https://api.deepseek.com',
            endpointFormat: 'chat_completions',
            models: ['deepseek-chat']
          }
        ]
      },
      runtime: {
        providerId: 'stale-openai',
        model: '',
        apiKey: '',
        baseUrl: ''
      }
    }, null, 2))
    const multiNonDeepSeek = resolveLiveNonDeepSeekProviderFromSettings(settingsPath)
    const multiNonDeepSeekSummary = multiNonDeepSeek.provider ? publicProviderSummary(multiNonDeepSeek.provider) : {}
    const multiNonDeepSeekSerialized = JSON.stringify(multiNonDeepSeekSummary)
    const multiNonDeepSeekPassed = multiNonDeepSeek.status === 'configured' &&
      multiNonDeepSeek.provider?.apiKey === 'sk-moonshot-provider' &&
      multiNonDeepSeek.provider?.baseUrl === 'https://api.moonshot.cn/v1' &&
      multiNonDeepSeekSummary.hasApiKey === true &&
      multiNonDeepSeekSummary.providerId === 'moonshot-cn' &&
      multiNonDeepSeekSummary.model === 'kimi-k2.7-code' &&
      multiNonDeepSeekSummary.providerIsDeepSeek === false &&
      !multiNonDeepSeekSerialized.includes('sk-moonshot-provider') &&
      !multiNonDeepSeekSerialized.includes('sk-stale-provider') &&
      !multiNonDeepSeekSerialized.includes(secret)

    writeFileSync(settingsPath, JSON.stringify({
      provider: {
        activeProviderId: '',
        apiKey: '',
        baseUrl: '',
        providers: [
          {
            id: 'xiaomi-token-plan',
            name: 'Xiaomi Token Plan',
            apiKey: 'tp-xiaomi-provider',
            baseUrl: '',
            endpointFormat: '',
            models: []
          },
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: secret,
            baseUrl: 'https://api.deepseek.com',
            endpointFormat: 'chat_completions',
            models: ['deepseek-chat']
          }
        ]
      },
      runtime: {
        providerId: 'xiaomi-token-plan',
        model: '',
        apiKey: '',
        baseUrl: ''
      }
    }, null, 2))
    const tokenPlanNonDeepSeek = resolveLiveNonDeepSeekProviderFromSettings(settingsPath)
    const tokenPlanNonDeepSeekSummary = tokenPlanNonDeepSeek.provider ? publicProviderSummary(tokenPlanNonDeepSeek.provider) : {}
    const tokenPlanNonDeepSeekSerialized = JSON.stringify(tokenPlanNonDeepSeekSummary)
    const inactiveDeepSeek = resolveLiveDeepSeekProviderFromSettings(settingsPath)
    const inactiveDeepSeekSummary = inactiveDeepSeek.provider ? publicProviderSummary(inactiveDeepSeek.provider) : {}
    const inactiveDeepSeekSerialized = JSON.stringify(inactiveDeepSeekSummary)
    const tokenPlanNonDeepSeekPassed = tokenPlanNonDeepSeek.status === 'configured' &&
      tokenPlanNonDeepSeek.provider?.apiKey === 'tp-xiaomi-provider' &&
      tokenPlanNonDeepSeek.provider?.baseUrl === 'https://token-plan-cn.xiaomimimo.com/v1' &&
      tokenPlanNonDeepSeekSummary.hasApiKey === true &&
      tokenPlanNonDeepSeekSummary.providerId === 'xiaomi-token-plan' &&
      tokenPlanNonDeepSeekSummary.model === 'mimo-v2.5-pro' &&
      tokenPlanNonDeepSeekSummary.endpointFormat === 'chat_completions' &&
      tokenPlanNonDeepSeekSummary.providerIsDeepSeek === false &&
      !tokenPlanNonDeepSeekSerialized.includes('tp-xiaomi-provider') &&
      !tokenPlanNonDeepSeekSerialized.includes(secret)
    const inactiveDeepSeekPassed = inactiveDeepSeek.status === 'configured' &&
      inactiveDeepSeek.provider?.apiKey === secret &&
      inactiveDeepSeekSummary.activeProviderId === 'xiaomi-token-plan' &&
      inactiveDeepSeekSummary.providerId === 'deepseek' &&
      inactiveDeepSeekSummary.model === 'deepseek-chat' &&
      inactiveDeepSeekSummary.hasApiKey === true &&
      inactiveDeepSeekSummary.providerIsDeepSeek === true &&
      !inactiveDeepSeekSerialized.includes(secret) &&
      !inactiveDeepSeekSerialized.includes('tp-xiaomi-provider')

    writeFileSync(settingsPath, JSON.stringify({
      provider: {
        activeProviderId: 'opencode-go',
        apiKey: '',
        baseUrl: '',
        providers: [
          {
            id: 'opencode-go',
            name: 'OpenCode Go',
            apiKey: 'sk-opencode-provider',
            baseUrl: '',
            endpointFormat: '',
            models: []
          },
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: secret,
            baseUrl: 'https://api.deepseek.com',
            endpointFormat: 'chat_completions',
            models: ['deepseek-chat']
          }
        ]
      },
      runtime: {
        providerId: 'opencode-go',
        model: 'minimax-m3',
        apiKey: '',
        baseUrl: ''
      }
    }, null, 2))
    const modelOverrideNonDeepSeek = resolveLiveNonDeepSeekProviderFromSettings(settingsPath)
    const modelOverrideNonDeepSeekSummary = modelOverrideNonDeepSeek.provider ? publicProviderSummary(modelOverrideNonDeepSeek.provider) : {}
    const modelOverrideNonDeepSeekSerialized = JSON.stringify(modelOverrideNonDeepSeekSummary)
    const modelOverrideRequest = modelOverrideNonDeepSeek.provider ? liveProviderRequestConfig(modelOverrideNonDeepSeek.provider) : { supported: false }
    const modelOverrideNonDeepSeekPassed = modelOverrideNonDeepSeek.status === 'configured' &&
      modelOverrideNonDeepSeek.provider?.apiKey === 'sk-opencode-provider' &&
      modelOverrideNonDeepSeek.provider?.baseUrl === 'https://opencode.ai/zen/go/v1' &&
      modelOverrideNonDeepSeekSummary.hasApiKey === true &&
      modelOverrideNonDeepSeekSummary.providerId === 'opencode-go' &&
      modelOverrideNonDeepSeekSummary.model === 'minimax-m3' &&
      modelOverrideNonDeepSeekSummary.endpointFormat === 'messages' &&
      modelOverrideNonDeepSeekSummary.providerIsDeepSeek === false &&
      modelOverrideRequest.supported === true &&
      modelOverrideRequest.url === 'https://opencode.ai/zen/go/v1/messages' &&
      !modelOverrideNonDeepSeekSerialized.includes('sk-opencode-provider') &&
      !modelOverrideNonDeepSeekSerialized.includes(secret)

    const endpointAppendPassed =
      liveDeepSeekChatCompletionsURL({ baseUrl: 'https://api.deepseek.com/v1' }) === 'https://api.deepseek.com/v1/chat/completions' &&
      liveDeepSeekChatCompletionsURL({ baseUrl: 'https://api.deepseek.com/v1/chat/completions' }) === 'https://api.deepseek.com/v1/chat/completions'
    const passed = activeProviderPassed &&
      missingActiveFallbackPassed &&
      runtimeOverridePassed &&
      nonDeepSeekPassed &&
      presetNonDeepSeekPassed &&
      multiNonDeepSeekPassed &&
      tokenPlanNonDeepSeekPassed &&
      inactiveDeepSeekPassed &&
      modelOverrideNonDeepSeekPassed &&
      endpointAppendPassed
    return {
      schemaVersion: 1,
      id: 'runtime-go-provider-settings-resolution-self-test',
      status: passed ? 'passed' : 'failed',
      passed,
      provider: summary,
      runtimeProviderOverride: overrideSummary,
      nonDeepSeekProvider: nonDeepSeekSummary,
      presetNonDeepSeekProvider: {
        ...presetNonDeepSeekSummary,
        baseUrl: presetNonDeepSeek.provider?.baseUrl || ''
      },
      multiNonDeepSeekProvider: {
        ...multiNonDeepSeekSummary,
        baseUrl: multiNonDeepSeek.provider?.baseUrl || ''
      },
      tokenPlanNonDeepSeekProvider: {
        ...tokenPlanNonDeepSeekSummary,
        baseUrl: tokenPlanNonDeepSeek.provider?.baseUrl || ''
      },
      deepSeekProviderFromProfiles: inactiveDeepSeekSummary,
      modelOverrideNonDeepSeekProvider: {
        ...modelOverrideNonDeepSeekSummary,
        baseUrl: modelOverrideNonDeepSeek.provider?.baseUrl || '',
        requestUrl: modelOverrideRequest.url || ''
      },
      requestShape: {
        deepSeekChatCompletionsAppendIdempotent: endpointAppendPassed
      },
      redaction: {
        status: passed ? 'passed' : 'failed',
        credentialSecretsRecorded:
          serialized.includes(secret) ||
          serialized.includes('sk-other-provider') ||
          overrideSerialized.includes(overrideSecret) ||
          overrideSerialized.includes('sk-custom-provider') ||
          nonDeepSeekSerialized.includes('sk-custom-provider') ||
          nonDeepSeekSerialized.includes(overrideSecret) ||
          presetNonDeepSeekSerialized.includes('sk-moonshot-provider') ||
          presetNonDeepSeekSerialized.includes(secret) ||
          multiNonDeepSeekSerialized.includes('sk-moonshot-provider') ||
          multiNonDeepSeekSerialized.includes('sk-stale-provider') ||
          multiNonDeepSeekSerialized.includes(secret) ||
          tokenPlanNonDeepSeekSerialized.includes('tp-xiaomi-provider') ||
          inactiveDeepSeekSerialized.includes(secret) ||
          inactiveDeepSeekSerialized.includes('tp-xiaomi-provider') ||
          modelOverrideNonDeepSeekSerialized.includes('sk-opencode-provider') ||
          modelOverrideNonDeepSeekSerialized.includes(secret) ||
          tokenPlanNonDeepSeekSerialized.includes(secret)
      }
    }
  } finally {
    rmSync(tempRoot, { recursive: true, force: true })
  }
}

function spawnRuntime({ appPath, providerBaseUrl, tempRoot, timeoutMs, preferPackagedBinary, explicitRuntimeBin = '' }) {
  const runtimeBin = runtimeServerBinaryPath(appPath)
  const launch = runtimeLaunchCommand({
    appPath,
    runtimeBin,
    tempRoot,
    preferPackagedBinary,
    explicitRuntimeBin
  })
  const token = 'local-runtime-performance-credential'
  const providerId = 'deepseek-performance-local'
  const model = 'deepseek-chat'
  const dataDir = join(tempRoot, 'runtime-data')
  const modelProviders = JSON.stringify({
    defaultProviderId: providerId,
    providers: [{
      id: providerId,
      name: 'Analytix local performance provider',
      apiKey: 'local-performance-provider-credential',
      baseUrl: providerBaseUrl,
      endpointFormat: 'chat_completions',
      models: [model],
      prices: {
        [model]: { input: 0.14, output: 0.28, cacheHit: 0.014, currency: 'USD' }
      }
    }]
  })
  const runtimeArgs = [
    '-addr',
    '127.0.0.1:0',
    '-durable-root',
    dataDir,
    '-data-dir',
    dataDir,
    '-model',
    model
  ]
  const child = spawn(
    launch.command,
    [...(launch.prefixArgs || []), ...runtimeArgs],
    {
      cwd: launch.cwd,
      env: {
        ...process.env,
        ANALYTIX_RUNTIME_TOKEN: token,
        ANALYTIX_MODEL_PROVIDERS: modelProviders,
        ANALYTIX_MCP_CONFIG_JSON: JSON.stringify({ mcpServers: {} })
      },
      stdio: ['ignore', 'pipe', 'pipe']
    }
  )
  return {
    child,
    token,
    providerId,
    model,
    dataDir,
    runtimeBin: launch.command,
    launchMode: launch.mode,
    timeoutMs
  }
}

async function stopChild(child) {
  if (!child || child.exitCode !== null || child.signalCode !== null) return
  child.kill('SIGTERM')
  await new Promise((resolve) => {
    const timer = setTimeout(() => {
      if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL')
      resolve()
    }, 2_000)
    child.once('exit', () => {
      clearTimeout(timer)
      resolve()
    })
  })
}

async function closeServer(server) {
  await new Promise((resolve) => server.close(() => resolve()))
}

async function fetchText(url, token, init = {}, timeoutMs = DEFAULT_TIMEOUT_MS) {
  const headers = new Headers(init.headers || {})
  if (token) headers.set('Authorization', `Bearer ${token}`)
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const response = await fetch(url, {
    ...init,
    headers,
    signal: AbortSignal.timeout(timeoutMs)
  })
  const text = await response.text()
  let body = {}
  try {
    body = text.trim() ? JSON.parse(text) : {}
  } catch {
    body = {}
  }
  return { ok: response.ok, status: response.status, text, body }
}

function deferred() {
  let resolve
  let reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function parseSSEBlock(block) {
  const event = { id: '', event: 'message', data: '', receivedAtMs: nowMs(), json: null }
  for (const line of block.split(/\r?\n/)) {
    if (line.startsWith('id:')) event.id = line.slice(3).trim()
    else if (line.startsWith('event:')) event.event = line.slice(6).trim()
    else if (line.startsWith('data:')) event.data += line.slice(5).trim()
  }
  try {
    event.json = event.data ? JSON.parse(event.data) : null
  } catch {
    event.json = null
  }
  return event
}

async function collectLiveEvents(url, token, { signal, onEvent, onOpen }) {
  const response = await fetch(url, {
    headers: {
      Accept: 'text/event-stream',
      Authorization: `Bearer ${token}`
    },
    signal
  })
  if (!response.ok || !response.body) {
    throw new Error(`live SSE failed: ${response.status}`)
  }
  onOpen?.()
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      let index
      while ((index = buffer.indexOf('\n\n')) >= 0) {
        const block = buffer.slice(0, index).trim()
        buffer = buffer.slice(index + 2)
        if (!block || block.startsWith(':')) continue
        onEvent(parseSSEBlock(block))
      }
    }
  } catch (error) {
    if (error?.name !== 'AbortError') throw error
  }
}

function parseDurableEvents(text) {
  return text
    .split(/\n\n+/)
    .map((block) => block.trim())
    .filter(Boolean)
    .filter((block) => !block.startsWith(':'))
    .map(parseSSEBlock)
    .filter((event) => event.json)
}

function check(status, id, evidence = {}) {
  return { id, status: status ? 'passed' : 'failed', ...evidence }
}

function timed(promise, ms, message) {
  let timer
  return Promise.race([
    promise,
    new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error(message)), ms)
    })
  ]).finally(() => clearTimeout(timer))
}

function shouldRunLiveDeepSeekCache(options) {
  return options.liveDeepSeekCache === true ||
    options.liveP5 === true ||
    args.has('--live-p5-from-settings') ||
    args.has('--live-deepseek-cache-from-settings') ||
    process.env.ANALYTIX_RUNTIME_GO_PERF_LIVE_DEEPSEEK_CACHE === '1'
}

function shouldRunLiveNonDeepSeekProvider(options) {
  return options.liveNonDeepSeekProvider === true ||
    args.has('--live-non-deepseek-provider-from-settings') ||
    process.env.ANALYTIX_RUNTIME_GO_PERF_LIVE_NON_DEEPSEEK_PROVIDER === '1'
}

function redactSensitiveText(value, secrets = []) {
  let out = String(value || '')
  for (const secret of secrets) {
    const trimmed = strings(secret)
    if (trimmed) out = out.split(trimmed).join('<redacted>')
  }
  out = out.replace(/sk-[A-Za-z0-9_-]{12,}/g, 'sk-<redacted>')
  out = out.replace(/Bearer\s+[A-Za-z0-9._~+/-]+/gi, 'Bearer <redacted>')
  return out
}

function appendEndpointPath(baseUrl, path) {
  const cleanBase = strings(baseUrl).replace(/\/+$/, '')
  if (!cleanBase) return ''
  const cleanPath = path.replace(/^\/+/, '')
  if (cleanBase.toLowerCase().endsWith(`/${cleanPath.toLowerCase()}`)) return cleanBase
  return `${cleanBase}/${cleanPath}`
}

function liveProviderRequestConfig(config) {
  const endpointFormat = normalizeEndpointFormat(config.endpointFormat)
  const model = strings(config.model)
  if (endpointFormat === 'chat_completions') {
    return {
      supported: true,
      url: appendEndpointPath(config.baseUrl, 'chat/completions'),
      headers: {
        'content-type': 'application/json',
        authorization: `Bearer ${config.apiKey}`
      },
      body: {
        model,
        messages: [{ role: 'user', content: 'Return the single word OK.' }],
        max_tokens: 2,
        temperature: 0,
        stream: false
      }
    }
  }
  if (endpointFormat === 'responses') {
    return {
      supported: true,
      url: appendEndpointPath(config.baseUrl, 'responses'),
      headers: {
        'content-type': 'application/json',
        authorization: `Bearer ${config.apiKey}`
      },
      body: {
        model,
        input: 'Return the single word OK.',
        max_output_tokens: 8,
        temperature: 0,
        stream: false
      }
    }
  }
  if (endpointFormat === 'messages') {
    return {
      supported: true,
      url: appendEndpointPath(config.baseUrl, 'messages'),
      headers: {
        'content-type': 'application/json',
        'x-api-key': config.apiKey,
        'anthropic-version': '2023-06-01'
      },
      body: {
        model,
        max_tokens: 2,
        temperature: 0,
        messages: [{ role: 'user', content: 'Return the single word OK.' }]
      }
    }
  }
  return {
    supported: false,
    reason: `unsupported-endpoint-format:${endpointFormat}`
  }
}

function liveDeepSeekChatCompletionsURL(config) {
  return appendEndpointPath(config.baseUrl, 'chat/completions')
}

function liveProviderUsage(body, endpointFormat) {
  const usage = body?.usage && typeof body.usage === 'object' ? body.usage : {}
  if (endpointFormat === 'responses') {
    return {
      prompt_tokens: Number(usage.input_tokens || 0),
      completion_tokens: Number(usage.output_tokens || 0),
      total_tokens: Number(usage.total_tokens || Number(usage.input_tokens || 0) + Number(usage.output_tokens || 0)),
      cached_tokens: Number(usage.input_tokens_details?.cached_tokens || 0)
    }
  }
  if (endpointFormat === 'messages') {
    return {
      prompt_tokens: Number(usage.input_tokens || 0),
      completion_tokens: Number(usage.output_tokens || 0),
      total_tokens: Number(usage.input_tokens || 0) + Number(usage.output_tokens || 0),
      cache_read_input_tokens: Number(usage.cache_read_input_tokens || 0),
      cache_creation_input_tokens: Number(usage.cache_creation_input_tokens || 0)
    }
  }
  return {
    prompt_tokens: Number(usage.prompt_tokens || 0),
    completion_tokens: Number(usage.completion_tokens || 0),
    total_tokens: Number(usage.total_tokens || 0),
    cached_tokens: Number(usage.prompt_tokens_details?.cached_tokens || usage.cached_tokens || 0)
  }
}

async function runLiveNonDeepSeekProviderProbe({ settingsPath, timeoutMs, fetchFn = fetch }) {
  const resolved = resolveLiveNonDeepSeekProviderFromSettings(settingsPath)
  if (resolved.status !== 'configured') {
    return {
      status: resolved.status,
      reason: resolved.reason,
      provider: resolved.provider ? publicProviderSummary(resolved.provider) : null,
      providerResponded: false
    }
  }
  const config = resolved.provider
  if (!strings(config.baseUrl)) {
    return {
      status: 'missing_base_url',
      reason: 'missing-base-url',
      provider: publicProviderSummary(config),
      providerResponded: false
    }
  }
  const requestConfig = liveProviderRequestConfig(config)
  if (!requestConfig.supported) {
    return {
      status: 'unsupported',
      reason: requestConfig.reason,
      provider: publicProviderSummary(config),
      providerResponded: false
    }
  }
  const started = Date.now()
  const endpointFormat = normalizeEndpointFormat(config.endpointFormat)
  const response = await fetchFn(requestConfig.url, {
    method: 'POST',
    headers: requestConfig.headers,
    body: JSON.stringify(requestConfig.body),
    signal: AbortSignal.timeout(timeoutMs)
  })
  const text = await response.text()
  let body = {}
  try {
    body = text.trim() ? JSON.parse(text) : {}
  } catch {
    body = {}
  }
  if (!response.ok) {
    return {
      status: 'failed',
      reason: 'provider-http-error',
      provider: publicProviderSummary(config),
      providerResponded: true,
      statusCode: response.status,
      elapsedMs: Date.now() - started,
      error: redactSensitiveText(body.error?.message || text || '', [config.apiKey]).slice(0, 180)
    }
  }
  return {
    status: 'passed',
    provider: publicProviderSummary(config),
    providerResponded: true,
    statusCode: response.status,
    elapsedMs: Date.now() - started,
    usage: liveProviderUsage(body, endpointFormat),
    responseTextLength: String(
      body.choices?.[0]?.message?.content ||
      body.output_text ||
      body.content?.[0]?.text ||
      ''
    ).length
  }
}

async function runLiveDeepSeekCacheProbe({ settingsPath, timeoutMs, fetchFn = fetch }) {
  if (!settingsPath || !existsSync(settingsPath)) {
    return { status: 'skipped', reason: 'settings-not-found', cacheHitObserved: false }
  }
  const resolved = resolveLiveDeepSeekProviderFromSettings(settingsPath)
  if (resolved.status !== 'configured') {
    return {
      status: resolved.status === 'not_configured' ? 'skipped' : 'failed',
      reason: resolved.reason,
      provider: resolved.provider ? publicProviderSummary(resolved.provider) : null,
      cacheHitObserved: false
    }
  }
  const config = resolved.provider
  const model = strings(config.model) || 'deepseek-chat'
  const stablePrefix = Array.from({ length: 220 }, (_, index) =>
    `Analytix cache probe stable line ${String(index).padStart(3, '0')}: product-runtime-cache-prefix-validation.`
  ).join('\n')
  const toolsJSON = '[]'
  const systemHash = sha256(stablePrefix)
  const toolsHash = sha256(toolsJSON)
  const prefixHash = sha256(JSON.stringify({ system: stablePrefix, tools: toolsJSON }))
  async function call(index) {
    const started = Date.now()
    const response = await fetchFn(liveDeepSeekChatCompletionsURL(config), {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        authorization: `Bearer ${config.apiKey}`
      },
      body: JSON.stringify({
        model,
        messages: [
          { role: 'system', content: stablePrefix },
          { role: 'user', content: 'Return the single word OK.' }
        ],
        max_tokens: 2,
        temperature: 0,
        stream: false
      }),
      signal: AbortSignal.timeout(timeoutMs)
    })
    const text = await response.text()
    let body = {}
    try {
      body = text.trim() ? JSON.parse(text) : {}
    } catch {
      body = {}
    }
    if (!response.ok) {
      return {
        index,
        ok: false,
        status: response.status,
        elapsedMs: Date.now() - started,
        error: redactSensitiveText(body.error?.message || text || '', [config.apiKey]).slice(0, 180)
      }
    }
    const usage = body.usage || {}
    return {
      index,
      ok: true,
      status: response.status,
      elapsedMs: Date.now() - started,
      usage: {
        prompt_tokens: Number(usage.prompt_tokens || 0),
        completion_tokens: Number(usage.completion_tokens || 0),
        total_tokens: Number(usage.total_tokens || 0),
        prompt_cache_hit_tokens: Number(usage.prompt_cache_hit_tokens || 0),
        prompt_cache_miss_tokens: Number(usage.prompt_cache_miss_tokens || 0)
      },
      responseTextLength: String(body.choices?.[0]?.message?.content || '').length
    }
  }
  const first = await call(1)
  await sleep(1200)
  const second = await call(2)
  const cacheHitObserved =
    Number(first.usage?.prompt_cache_hit_tokens || 0) > 0 ||
    Number(second.usage?.prompt_cache_hit_tokens || 0) > 0
  const warmUsage = second.usage || {}
  const hit = Number(warmUsage.prompt_cache_hit_tokens || 0)
  const miss = Number(warmUsage.prompt_cache_miss_tokens || 0)
  return {
    status: first.ok && second.ok && cacheHitObserved ? 'passed' : 'not_confirmed',
    provider: publicProviderSummary(config),
    model,
    systemHash,
    toolsHash,
    prefixHash,
    cacheHitObserved,
    cacheHitRate: hit + miss > 0 ? hit / (hit + miss) : null,
    calls: [first, second]
  }
}

function spawnRuntimeWithProviderConfig({ appPath, providerConfig, tempRoot }) {
  const runtimeBin = runtimeServerBinaryPath(appPath)
  const launch = runtimeLaunchCommand({
    appPath,
    runtimeBin,
    tempRoot,
    preferPackagedBinary: true
  })
  const token = 'live-runtime-performance-credential'
  const dataDir = join(tempRoot, 'live-runtime-data')
  const modelProviders = JSON.stringify({
    defaultProviderId: providerConfig.providerId,
    providers: [{
      id: providerConfig.providerId,
      name: 'Analytix live performance provider',
      apiKey: providerConfig.apiKey,
      baseUrl: providerConfig.baseUrl,
      endpointFormat: providerConfig.endpointFormat,
      models: [providerConfig.model]
    }]
  })
  const runtimeArgs = [
    '-addr',
    '127.0.0.1:0',
    '-durable-root',
    dataDir,
    '-data-dir',
    dataDir,
    '-provider-id',
    providerConfig.providerId,
    '-model',
    providerConfig.model
  ]
  const child = spawn(launch.command, [...(launch.prefixArgs || []), ...runtimeArgs], {
    cwd: launch.cwd,
    env: {
      ...process.env,
      ANALYTIX_RUNTIME_TOKEN: token,
      ANALYTIX_MODEL_PROVIDERS: modelProviders,
      ANALYTIX_MCP_CONFIG_JSON: JSON.stringify({ mcpServers: {} })
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
  return { child, token, dataDir, runtimeBin, launchMode: launch.mode }
}

async function collectRuntimeSseEvents(url, token, { signal, onRendererBatch, onOpen }) {
  const response = await fetch(url, {
    headers: {
      Accept: 'text/event-stream',
      Authorization: `Bearer ${token}`
    },
    signal
  })
  if (!response.ok || !response.body) {
    throw new Error(`runtime SSE probe failed: ${response.status}`)
  }
  onOpen?.()
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let pendingEvents = []
  let throttleTimer
  let firstEventFlushed = false
  const flush = () => {
    if (throttleTimer) {
      clearTimeout(throttleTimer)
      throttleTimer = undefined
    }
    if (pendingEvents.length === 0) return
    const batch = pendingEvents
    pendingEvents = []
    onRendererBatch({ receivedAtMs: nowMs(), events: batch })
  }
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      let index
      let hasDeferredEvents = false
      while ((index = buffer.indexOf('\n\n')) >= 0) {
        const block = buffer.slice(0, index).trim()
        buffer = buffer.slice(index + 2)
        if (!block || block.startsWith(':')) continue
        const event = parseSSEBlock(block).json
        if (event?.kind === 'accepted_final_batch' || event?.kind === 'general_terminal_batch') {
          // Match the Electron main bridge: progress may be frame-batched, but
          // an indivisible terminal batch is delivered alone without waiting
          // for the 16 ms progress timer.
          flush()
          pendingEvents.push(event)
          firstEventFlushed = true
          hasDeferredEvents = false
          flush()
          continue
        }
        pendingEvents.push(event)
        if (!firstEventFlushed) {
          firstEventFlushed = true
          flush()
        } else {
          hasDeferredEvents = true
        }
      }
      if (hasDeferredEvents && !throttleTimer) {
        throttleTimer = setTimeout(flush, MAIN_SSE_EVENT_BATCH_MS)
      }
    }
    buffer += decoder.decode()
    const trailing = buffer.trim()
    if (trailing && !trailing.startsWith(':')) {
      pendingEvents.push(parseSSEBlock(trailing).json)
    }
  } catch (error) {
    if (error?.name !== 'AbortError') throw error
  } finally {
    flush()
  }
}

function usageFromEvent(event) {
  const usage = event?.usage || {}
  const hit = Number(usage.cacheHitTokens || usage.cachedTokens || 0)
  const miss = Number(usage.cacheMissTokens || 0)
  return {
    promptTokens: Number(usage.promptTokens || 0),
    completionTokens: Number(usage.completionTokens || 0),
    totalTokens: Number(usage.totalTokens || 0),
    cacheHitTokens: hit,
    cacheMissTokens: miss,
    cacheHitRate: hit + miss > 0 ? hit / (hit + miss) : null
  }
}

function flattenedPublicTerminalEvents(events) {
  const out = []
  for (const event of events) {
    if ((event?.kind === 'general_terminal_batch' || event?.kind === 'accepted_final_batch') &&
        Array.isArray(event.events)) {
      out.push(...event.events)
    } else {
      out.push(event)
    }
  }
  return out
}

function replayContainsProviderDraft(events, draftFragments) {
  return events.some((event) => {
    if (event?.kind === 'general_terminal_batch' || event?.kind === 'accepted_final_batch') {
      return false
    }
    if (event?.kind === 'assistant_text_delta') return true
    const encoded = JSON.stringify(event)
    return draftFragments.some((fragment) => encoded.includes(fragment))
  })
}

function tokenIntervals(times) {
  const out = []
  for (let index = 1; index < times.length; index += 1) {
    out.push(Math.round(times[index] - times[index - 1]))
  }
  return out
}

function runProductSseContractTests(timeoutMs) {
  const started = Date.now()
  const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
  const testFiles = [
    'src/main/runtime-sse-ipc.test.ts',
    'src/renderer/src/store/chat-store-runtime.test.ts',
    'src/renderer/src/thread/streaming/streaming-delta-scheduler.test.ts',
    'src/renderer/src/agent/analytix-mapper.test.ts'
  ]
  const result = spawnSync(
    npmCommand,
    [
      'run',
      'test',
      '--',
      ...testFiles,
      '--run'
    ],
    {
      cwd: repoRoot,
      env: process.env,
      encoding: 'utf8',
      timeout: Math.max(timeoutMs, 30_000)
    }
  )
  const output = `${result.stdout || ''}\n${result.stderr || ''}`
  return {
    status: result.status === 0 ? 'passed' : 'failed',
    command: `npm run test -- ${testFiles.join(' ')} --run`,
    testFiles,
    durationMs: Date.now() - started,
    timedOut: result.error?.code === 'ETIMEDOUT',
    outputTail: redactSensitiveText(output).slice(-1200)
  }
}

async function runAnalytixRuntimeLiveProbe({ appPath, providerConfig, prompt, timeoutMs }) {
  const tempRoot = mkdtempSync(join(tmpdir(), 'analytix-runtime-go-live-probe-'))
  let runtime = null
  let abort = null
  try {
    const spawned = spawnRuntimeWithProviderConfig({ appPath, providerConfig, tempRoot })
    runtime = spawned.child
    const ready = await waitForReady(runtime, timeoutMs)
    const created = await fetchText(`${ready.url}/v1/threads`, spawned.token, {
      method: 'POST',
      body: JSON.stringify({
        title: 'Analytix live performance probe',
        workspace: tempRoot,
        model: providerConfig.model,
        providerId: providerConfig.providerId,
        mode: 'agent'
      })
    }, timeoutMs)
    const threadId = created.body.id
    if (!created.ok || !threadId) throw new Error(`thread create failed: ${created.status} ${created.text}`)

    const rendererBatches = []
    const sseOpen = deferred()
    const firstSafeProgress = deferred()
    const atomicTerminal = deferred()
    abort = new AbortController()
    const bridgePromise = collectRuntimeSseEvents(
      `${ready.url}/v1/threads/${encodeURIComponent(threadId)}/events?since_seq=0&live=1`,
      spawned.token,
      {
        signal: abort.signal,
        onOpen: sseOpen.resolve,
        onRendererBatch(batch) {
          rendererBatches.push(batch)
          for (const event of batch.events) {
            if (event?.kind === 'turn_started' || event?.kind === 'pipeline_stage' ||
                event?.kind === 'tool_progress') {
              firstSafeProgress.resolve({ event, receivedAtMs: batch.receivedAtMs })
            }
            if (event?.kind === 'general_terminal_batch') {
              atomicTerminal.resolve({ event, receivedAtMs: batch.receivedAtMs })
            }
          }
        }
      }
    ).catch((error) => {
      if (error?.name !== 'AbortError') {
        firstSafeProgress.reject(error)
        atomicTerminal.reject(error)
      }
    })
    await timed(sseOpen.promise, timeoutMs, 'timeout opening Analytix runtime SSE probe')
    const startedAt = nowMs()
    const turnPromise = fetchText(`${ready.url}/v1/threads/${encodeURIComponent(threadId)}/turns`, spawned.token, {
      method: 'POST',
      body: JSON.stringify({
        prompt,
        model: providerConfig.model,
        providerId: providerConfig.providerId,
        disableUserInput: true,
        maxModelSteps: 1
      })
    }, timeoutMs)
    const [first, terminal, turn] = await Promise.all([
      timed(firstSafeProgress.promise, timeoutMs, 'timeout waiting for Analytix safe progress event'),
      timed(atomicTerminal.promise, timeoutMs, 'timeout waiting for Analytix atomic terminal batch'),
      turnPromise
    ])
    const completedAt = nowMs()
    const finalReplay = await fetchText(`${ready.url}/v1/threads/${encodeURIComponent(threadId)}/events?since_seq=0`, spawned.token, {
      headers: { Accept: 'text/event-stream' }
    }, timeoutMs)
    abort.abort()
    await bridgePromise
    const outerEvents = parseDurableEvents(finalReplay.text).map((event) => event.json)
    const events = flattenedPublicTerminalEvents(outerEvents)
    const usageEvent = events.find((event) => event?.kind === 'usage')
    const textTimes = []
    for (const batch of rendererBatches) {
      for (const event of batch.events) {
        if (event?.kind === 'assistant_text_delta') textTimes.push(batch.receivedAtMs)
      }
    }
    return {
      status: turn.ok ? 'passed' : 'failed',
      surface: 'analytix-go-runtime-live-sse-reader',
      provider: publicProviderSummary(providerConfig),
      firstSafeProgressMs: Math.round(first.receivedAtMs - startedAt),
      atomicTerminalMs: Math.round(terminal.receivedAtMs - startedAt),
      turnResponseMs: Math.round(completedAt - startedAt),
      draftDeltaCount: textTimes.length,
      textDeltaIntervalsMs: tokenIntervals(textTimes),
      usage: usageFromEvent(usageEvent),
      prefix: {
        systemHash: strings(usageEvent?.cacheDiagnostics?.systemHash),
        toolsHash: strings(usageEvent?.cacheDiagnostics?.toolsHash),
        prefixHash: strings(usageEvent?.cacheDiagnostics?.prefixHash),
        prefixItemsHash: strings(usageEvent?.cacheDiagnostics?.prefixItemsHash),
        toolSchemaTokens: Number(usageEvent?.cacheDiagnostics?.toolSchemaTokens || 0),
        toolCount: Number(usageEvent?.cacheDiagnostics?.toolCount || 0)
      }
    }
  } finally {
    abort?.abort()
    await stopChild(runtime)
    rmSync(tempRoot, { recursive: true, force: true })
  }
}

export async function buildRuntimeGoPerformanceReport(options = {}) {
  const timeoutMs = Number(options.timeoutMs || argValue('--timeout-ms', process.env.ANALYTIX_RUNTIME_GO_PERF_TIMEOUT_MS || DEFAULT_TIMEOUT_MS))
  const firstDelayMs = Number(options.firstDelayMs || argValue('--provider-first-delay-ms', '40'))
  const finishDelayMs = Number(options.finishDelayMs || argValue('--provider-finish-delay-ms', '220'))
  const explicitAppPath = strings(options.appPath) || strings(argValue('--app-path', process.env.ANALYTIX_RUNTIME_GO_PERF_APP_PATH || ''))
  const appPath = String(options.appPath || argValue('--app-path', process.env.ANALYTIX_RUNTIME_GO_PERF_APP_PATH || DEFAULT_APP_PATH))
  const preferPackagedBinary = Boolean(explicitAppPath) || args.has('--no-go-run')
  const settingsPath = String(options.settingsPath || argValue(
    '--settings-path',
    join(homedir(), 'Library/Application Support/Analytix/analytix-settings.json')
  ))
  const tempRoot = mkdtempSync(join(tmpdir(), 'analytix-runtime-go-perf-'))
  const provider = startStreamingProvider({ firstDelayMs, finishDelayMs })
  let runtime = null
  let liveAbort = null
  let report
  try {
    const providerBaseUrl = await listen(provider.server)
    const spawned = spawnRuntime({
      appPath,
      providerBaseUrl,
      tempRoot,
      timeoutMs,
      preferPackagedBinary,
      explicitRuntimeBin: strings(options.runtimeBin)
    })
    runtime = spawned.child
    const ready = await waitForReady(runtime, timeoutMs)
    const health = await fetchText(`${ready.url}/health`, '', {}, timeoutMs)
    const created = await fetchText(`${ready.url}/v1/threads`, spawned.token, {
      method: 'POST',
      body: JSON.stringify({
        title: 'Analytix Go performance check',
        workspace: tempRoot,
        model: spawned.model,
        providerId: spawned.providerId,
        mode: 'agent'
      })
    }, timeoutMs)
    const threadId = created.body.id
    if (!created.ok || !threadId) throw new Error(`thread create failed: ${created.status} ${created.text}`)

    const rendererBatches = []
    const sseOpen = deferred()
    const firstSafeProgress = deferred()
    const atomicTerminal = deferred()
    liveAbort = new AbortController()
    const livePromise = collectRuntimeSseEvents(
      `${ready.url}/v1/threads/${encodeURIComponent(threadId)}/events?since_seq=0&live=1`,
      spawned.token,
      {
        signal: liveAbort.signal,
        onOpen: sseOpen.resolve,
        onRendererBatch(batch) {
          rendererBatches.push(batch)
          for (const event of batch.events) {
            if (event?.kind === 'turn_started' || event?.kind === 'pipeline_stage' ||
                event?.kind === 'tool_progress') {
              firstSafeProgress.resolve({ event, receivedAtMs: batch.receivedAtMs })
            }
            if (event?.kind === 'general_terminal_batch' || event?.kind === 'accepted_final_batch') {
              atomicTerminal.resolve({ event, receivedAtMs: batch.receivedAtMs })
            }
          }
        }
      }
    ).catch((error) => {
      if (error?.name !== 'AbortError') {
        firstSafeProgress.reject(error)
        atomicTerminal.reject(error)
      }
    })
    await timed(sseOpen.promise, timeoutMs, 'timeout opening live SSE')

    const turnStartedAt = nowMs()
    const turnPromise = fetchText(`${ready.url}/v1/threads/${encodeURIComponent(threadId)}/turns`, spawned.token, {
      method: 'POST',
      body: JSON.stringify({
        prompt: 'Measure Analytix Go runtime streaming and cache telemetry.',
        model: spawned.model,
        providerId: spawned.providerId,
        disableUserInput: true
      })
    }, timeoutMs)

    const turnResultPromise = turnPromise.then((turn) => ({ turn, receivedAtMs: nowMs() }))
    const firstProgressEvent = await timed(firstSafeProgress.promise, timeoutMs, 'timeout waiting for first safe progress event')
    const immediateReplay = await fetchText(`${ready.url}/v1/threads/${encodeURIComponent(threadId)}/events?since_seq=0`, spawned.token, {
      headers: { Accept: 'text/event-stream' }
    }, timeoutMs)
    const turnResult = await turnResultPromise
    const turn = turnResult.turn
    const turnResponseAt = turnResult.receivedAtMs
    const atomicTerminalEvent = await timed(atomicTerminal.promise, timeoutMs, 'timeout waiting for atomic terminal batch')
    const finalReplay = await fetchText(`${ready.url}/v1/threads/${encodeURIComponent(threadId)}/events?since_seq=0`, spawned.token, {
      headers: { Accept: 'text/event-stream' }
    }, timeoutMs)
    const thread = await fetchText(`${ready.url}/v1/threads/${encodeURIComponent(threadId)}`, spawned.token, {}, timeoutMs)

    liveAbort.abort()
    await livePromise

    const finalOuterEvents = parseDurableEvents(finalReplay.text).map((event) => event.json)
    const finalEvents = flattenedPublicTerminalEvents(finalOuterEvents)
    const usageEvent = finalEvents.find((event) => event?.kind === 'usage')
    const usage = usageEvent?.usage || {}
    const cacheDiagnostics = usageEvent?.cacheDiagnostics || {}
    const firstSafeProgressMs = Math.max(0, firstProgressEvent.receivedAtMs - turnStartedAt)
    const atomicTerminalMs = Math.max(0, atomicTerminalEvent.receivedAtMs - turnStartedAt)
    const turnAcceptedMs = Math.max(0, turnResponseAt - turnStartedAt)
    const providerDraftFragments = ['performance first token', 'and final token']
    const immediateOuterEvents = parseDurableEvents(immediateReplay.text).map((event) => event.json)
    const immediateReplayContainsDraft = replayContainsProviderDraft(
      immediateOuterEvents,
      providerDraftFragments
    )
    const finalReplayContainsDraft = replayContainsProviderDraft(
      finalOuterEvents,
      providerDraftFragments
    )
    const replayContainsCompletion = finalReplay.text.includes('turn_completed')
    const replayContainsUsage = finalReplay.text.includes('"kind":"usage"') || finalReplay.text.includes('event: usage')
    const noReasoningLeak = !finalReplay.text.includes('assistant_reasoning')
    const costUsd = Number(usage.costUsd || usage.costUSD || 0)
    const cacheHitTokens = Number(usage.cacheHitTokens || usage.cachedTokens || 0)
    const cacheMissTokens = Number(usage.cacheMissTokens || 0)
    const providerRequest = provider.requests[0] || {}
    const acceptedFinalEvent = finalEvents.find((event) => event?.kind === 'item_completed' &&
      event?.item?.acceptedFinal?.publicView)
    const acceptedFinalView = acceptedFinalEvent?.item?.acceptedFinal?.publicView || {}
    const receiptMetadata = acceptedFinalView?.receiptMetadata || {}
    const authorityBoundaryMode = provider.requests.length === 0
    const authorityBoundaryOk = authorityBoundaryMode &&
      atomicTerminalEvent.event?.kind === 'accepted_final_batch' &&
      acceptedFinalView.variant === 'GeneralGuidanceAnswer' &&
      acceptedFinalView.coverageStatus === 'guidance_only' &&
      acceptedFinalView.publicationState === 'accepted' &&
      Number(acceptedFinalView.claimCount || 0) === 0 &&
      Number(receiptMetadata.count || 0) === 0 &&
      Array.isArray(receiptMetadata.citations) && receiptMetadata.citations.length === 0 &&
      Number(usage.totalTokens || 0) === 0 && usage.priceConfigured !== true &&
      !finalReplayContainsDraft
    const liveDeepSeekRequested = shouldRunLiveDeepSeekCache(options)
    const liveDeepSeekCache = liveDeepSeekRequested
      ? await runLiveDeepSeekCacheProbe({
          settingsPath,
          timeoutMs
        })
      : { status: 'skipped', reason: 'not-requested', cacheHitObserved: false }
    const liveNonDeepSeekRequested = shouldRunLiveNonDeepSeekProvider(options)
    const liveNonDeepSeekProvider = liveNonDeepSeekRequested
      ? await runLiveNonDeepSeekProviderProbe({
          settingsPath,
          timeoutMs
        })
      : { status: 'skipped', reason: 'not-requested', providerResponded: false }
    const runSseContractTests = options.runProductSseContractTests !== false
    const productSseContractTests = runSseContractTests
      ? runProductSseContractTests(timeoutMs)
      : {
          status: 'not_run',
          required: false,
          reason: 'benchmark sample reuses the structural suite result'
        }
    const checks = [
      check(health.ok, 'runtime-health', { statusCode: health.status }),
      check(turn.ok && turn.status === 202, 'turn-create-completes', { statusCode: turn.status }),
      check(finalReplay.ok &&
        (atomicTerminalEvent.event?.kind === 'general_terminal_batch' ||
          atomicTerminalEvent.event?.kind === 'accepted_final_batch'),
      'durable-before-publish', {
        evidence: 'the terminal publication was subsequently readable from the durable finite replay'
      }),
      check(firstSafeProgressMs <= atomicTerminalMs, 'safe-progress-precedes-atomic-terminal', {
        firstSafeProgressMs: Math.round(firstSafeProgressMs),
        atomicTerminalMs: Math.round(atomicTerminalMs)
      }),
      check(!immediateReplayContainsDraft && !finalReplayContainsDraft, 'provider-draft-never-published', {
        evidence: 'live and finite replay contained only host-safe progress plus the atomic terminal boundary'
      }),
      check(replayContainsCompletion && replayContainsUsage, 'final-replay-turn-completed-and-usage', {}),
      check(noReasoningLeak, 'ordinary-text-turn-no-reasoning-leak', {})
    ]
    if (authorityBoundaryMode) {
      checks.push(
        check(provider.requests.length === 0, 'production-authority-boundary-zero-provider-dispatch', {
          count: provider.requests.length
        }),
        check(authorityBoundaryOk, 'production-authority-boundary-host-final', {
          atomicTerminalKind: atomicTerminalEvent.event?.kind || '',
          variant: acceptedFinalView.variant || '',
          coverageStatus: acceptedFinalView.coverageStatus || '',
          publicationState: acceptedFinalView.publicationState || '',
          receiptCount: Number(receiptMetadata.count || 0)
        }),
        check(Number(usage.totalTokens || 0) === 0 && usage.priceConfigured !== true,
          'production-authority-boundary-zero-provider-usage', {
            totalTokens: Number(usage.totalTokens || 0),
            priceConfigured: usage.priceConfigured === true
          })
      )
    } else {
      checks.push(
        check(provider.requests.length === 1, 'single-provider-stream-request', { count: provider.requests.length }),
        check(providerRequest.stream === true && providerRequest.streamOptionsIncludeUsage === true,
          'provider-stream-usage-requested', {
            requestDigest: providerRequest.bodyDigest || ''
          }),
        check(cacheHitTokens === 64 && cacheMissTokens === 36, 'deepseek-cache-usage-parsed', {
          cacheHitTokens,
          cacheMissTokens
        }),
        check(costUsd > 0 && usage.priceConfigured === true, 'configured-pricing-cost-non-zero', {
          costUsd
        })
      )
    }
    if (runSseContractTests) {
      checks.push(check(productSseContractTests.status === 'passed', 'product-sse-main-and-renderer-contract-tests', {
        durationMs: productSseContractTests.durationMs
      }))
    }
    if (liveDeepSeekRequested) {
      checks.push(check(liveDeepSeekCache.status === 'passed', 'live-deepseek-provider-cache-hit-observed', {
        status: liveDeepSeekCache.status,
        cacheHitObserved: liveDeepSeekCache.cacheHitObserved === true
      }))
    }
    if (liveNonDeepSeekRequested) {
      checks.push(check(liveNonDeepSeekProvider.status === 'passed', 'live-non-deepseek-provider-observed', {
        status: liveNonDeepSeekProvider.status,
        providerResponded: liveNonDeepSeekProvider.providerResponded === true,
        providerId: liveNonDeepSeekProvider.provider?.providerId || '',
        endpointFormat: liveNonDeepSeekProvider.provider?.endpointFormat || ''
      }))
    }
    const localCheckIds = new Set(checks.map((item) => item.id))
    const p5RequiredChecks = {
      localRuntimeParsing: false,
      liveDeepSeekCache: liveDeepSeekCache.status === 'passed',
      runtimeLiveSse: firstSafeProgressMs <= atomicTerminalMs && !finalReplayContainsDraft,
      productSseContractTests: !runSseContractTests || productSseContractTests.status === 'passed'
    }
    const failedChecks = checks.filter((item) => item.status !== 'passed').map((item) => item.id)
    const externalGateIds = new Set(['live-deepseek-provider-cache-hit-observed', 'live-non-deepseek-provider-observed'])
    const localFailureChecks = checks
      .filter((item) => localCheckIds.has(item.id) && !externalGateIds.has(item.id) && item.status !== 'passed')
      .map((item) => item.id)
    p5RequiredChecks.localRuntimeParsing = localFailureChecks.length === 0
    const structuralGatePassed = localFailureChecks.length === 0
    const gatePassed = structuralGatePassed &&
      (!liveDeepSeekRequested || liveDeepSeekCache.status === 'passed') &&
      (!liveNonDeepSeekRequested || liveNonDeepSeekProvider.status === 'passed')
    const p5Complete = !authorityBoundaryMode && Object.values(p5RequiredChecks).every(Boolean)
    const reportStatus = gatePassed ? 'passed' : 'failed'
    report = {
      schemaVersion: 1,
      id: 'runtime-go-performance-check',
      stage: 'p5-performance-cache',
      gateKind: 'structural-correctness',
      latencySloApplied: false,
      generatedAt: new Date().toISOString(),
      status: reportStatus,
      passed: gatePassed,
      structuralGatePassed,
      measurementMode: authorityBoundaryMode ? 'production-authority-boundary' : 'provider-positive-fixture',
      providerPerformanceObserved: !authorityBoundaryMode,
      providerPerformanceClaimAllowed: false,
      authorityBoundary: {
        expected: authorityBoundaryMode,
        ok: authorityBoundaryOk,
        providerExecutionExpected: !authorityBoundaryMode,
        providerExecutionBlocked: authorityBoundaryMode && provider.requests.length === 0,
        hostAcceptedFinalOk: authorityBoundaryOk
      },
      liveDeepSeekRequested,
      liveNonDeepSeekRequested,
      p5RequiredChecks,
      p5CompletionClaimAllowed: p5Complete,
      completionStatus: p5Complete ? 'complete' : 'partial',
      localProviderOnly: liveDeepSeekCache.status !== 'passed' ||
        (liveNonDeepSeekRequested && liveNonDeepSeekProvider.status !== 'passed'),
      appPath,
      runtimeServerBinaryExists: existsSync(spawned.runtimeBin),
      runtimeLaunchMode: spawned.launchMode,
      packagedRuntimeSelected: spawned.launchMode === 'packaged-binary',
      productionTagUsed: options.productionTagUsed === true ||
        spawned.launchMode === 'go-build-production-tag' ||
        spawned.launchMode === 'packaged-binary',
      provider: {
        requestCount: provider.requests.length,
        requestShape: providerRequest
          ? {
              url: providerRequest.url || '',
              fields: providerRequest.fields || [],
              messageCount: providerRequest.messageCount || 0,
              stream: providerRequest.stream === true,
              streamOptionsIncludeUsage: providerRequest.streamOptionsIncludeUsage === true,
              hasThinking: providerRequest.hasThinking === true,
              hasReasoningEffort: providerRequest.hasReasoningEffort === true,
              toolCount: providerRequest.toolCount || 0,
              bodyDigest: providerRequest.bodyDigest || ''
            }
          : {}
      },
      timings: {
        providerFirstDelayMs: firstDelayMs,
        providerFinishDelayMs: finishDelayMs,
        turnToProviderRequestMs: provider.timings.requestStartedAt
          ? Math.max(0, Math.round(provider.timings.requestStartedAt - turnStartedAt))
          : 0,
        providerRequestToFirstFrameMs: provider.timings.firstFrameAt && provider.timings.requestStartedAt
          ? Math.round(provider.timings.firstFrameAt - provider.timings.requestStartedAt)
          : 0,
        providerRequestToDoneFrameMs: provider.timings.doneFrameAt && provider.timings.requestStartedAt
          ? Math.round(provider.timings.doneFrameAt - provider.timings.requestStartedAt)
          : 0,
        providerDoneToAtomicTerminalMs: provider.timings.doneFrameAt
          ? Math.max(0, Math.round(atomicTerminalEvent.receivedAtMs - provider.timings.doneFrameAt))
          : 0,
        firstSafeProgressMs: Math.round(firstSafeProgressMs),
        atomicTerminalMs: Math.round(atomicTerminalMs),
        turnAcceptedMs: Math.round(turnAcceptedMs)
      },
      runtimeSseProbe: {
        firstSafeProgressKind: firstProgressEvent.event?.kind || '',
        firstSafeProgressSeq: firstProgressEvent.event?.seq || 0,
        atomicTerminalKind: atomicTerminalEvent.event?.kind || '',
        atomicTerminalSeq: atomicTerminalEvent.event?.seq || 0,
        providerDraftPublished: finalReplayContainsDraft,
        rendererBatchCount: rendererBatches.length,
        batchWindowMs: MAIN_SSE_EVENT_BATCH_MS,
        prefix: {
          systemHash: strings(cacheDiagnostics.systemHash),
          toolsHash: strings(cacheDiagnostics.toolsHash),
          prefixHash: strings(cacheDiagnostics.prefixHash),
          prefixItemsHash: strings(cacheDiagnostics.prefixItemsHash),
          toolSchemaTokens: Number(cacheDiagnostics.toolSchemaTokens || 0),
          toolCount: Number(cacheDiagnostics.toolCount || 0)
        }
      },
      productSseContractTests,
      replay: {
        immediateReplayContainsProviderDraft: immediateReplayContainsDraft,
        finalReplayContainsProviderDraft: finalReplayContainsDraft,
        finalReplayContainsTurnCompleted: replayContainsCompletion,
        finalReplayContainsUsage: replayContainsUsage,
        finalReplayDigest: sha256(finalReplay.text)
      },
      usage: {
        promptTokens: Number(usage.promptTokens || 0),
        completionTokens: Number(usage.completionTokens || 0),
        totalTokens: Number(usage.totalTokens || 0),
        cacheHitTokens,
        cacheMissTokens,
        cacheHitRate: Number(usage.cacheHitRate || 0),
        costUsd,
        priceConfigured: usage.priceConfigured === true
      },
      liveDeepSeekCache,
      liveNonDeepSeekProvider,
      thread: {
        idHash: sha256(threadId),
        readable: thread.ok,
        usageCostUsd: Number(thread.body?.usage?.costUsd || thread.body?.usage?.costUSD || 0)
      },
      redaction: {
        status: 'passed',
        credentialSecretsRecorded: false,
        rawValueRecorded: false
      },
      checks,
      failureChecks: failedChecks
    }
  } catch (error) {
    report = {
      schemaVersion: 1,
      id: 'runtime-go-performance-check',
      stage: 'p5-performance-cache',
      generatedAt: new Date().toISOString(),
      status: 'failed',
      passed: false,
      message: error instanceof Error ? error.message : String(error),
      redaction: {
        status: 'passed',
        credentialSecretsRecorded: false,
        rawValueRecorded: false
      }
    }
  } finally {
    liveAbort?.abort()
    await stopChild(runtime)
    await closeServer(provider.server)
    if (argValue('--keep-temp', '') !== '1') {
      rmSync(tempRoot, { recursive: true, force: true })
    }
  }
  return report
}

function commandOutput(command, commandArgs, cwd = repoRoot) {
  const result = spawnSync(command, commandArgs, {
    cwd,
    env: process.env,
    encoding: 'utf8',
    stdio: 'pipe'
  })
  return result.status === 0 ? strings(result.stdout) : ''
}

export function nearestRank(values, percentile) {
  const sorted = values
    .map((value) => Number(value))
    .filter(Number.isFinite)
    .sort((left, right) => left - right)
  if (sorted.length === 0) return null
  const rank = Math.max(1, Math.ceil(Number(percentile) * sorted.length))
  return sorted[Math.min(rank, sorted.length) - 1]
}

function rcHostProfile() {
  const profile = {
    os: platform(),
    arch: arch(),
    machine: commandOutput('sysctl', ['-n', 'hw.model']),
    cpu: commandOutput('sysctl', ['-n', 'machdep.cpu.brand_string']),
    logicalCpuCount: cpus().length,
    osVersion: commandOutput('sw_vers', ['-productVersion'])
  }
  return {
    ...profile,
    supported: profile.os === 'darwin' &&
      profile.arch === 'arm64' &&
      profile.machine === 'Mac16,8' &&
      profile.cpu === 'Apple M4 Pro' &&
      profile.logicalCpuCount === 12,
    acceptedProfile: {
      os: 'darwin',
      arch: 'arm64',
      machine: 'Mac16,8',
      cpu: 'Apple M4 Pro',
      logicalCpuCount: 12
    }
  }
}

function rcSourceIdentity() {
  const statusResult = spawnSync('git', ['status', '--porcelain=v1', '--untracked-files=all'], {
    cwd: repoRoot,
    env: process.env,
    encoding: 'utf8',
    stdio: 'pipe'
  })
  const status = parseGitPorcelainEntries(statusResult.stdout)
  const blockingEntries = status.filter((entry) => entry !== ' M AGENTS.md')
  const head = commandOutput('git', ['rev-parse', 'HEAD'])
  const tree = commandOutput('git', ['rev-parse', 'HEAD^{tree}'])
  const identityValid = /^[0-9a-f]{40}$/.test(head) && /^[0-9a-f]{40}$/.test(tree)
  return {
    head,
    tree,
    identityValid,
    sourceBoundToHeadTree: statusResult.status === 0 && identityValid && blockingEntries.length === 0,
    allowedUserDirtyPaths: status.includes(' M AGENTS.md') ? ['AGENTS.md'] : [],
    blockingEntries
  }
}

export function parseGitPorcelainEntries(output) {
  return String(output || '').split(/\r?\n/).filter(Boolean)
}

function rcToolchain() {
  return {
    node: process.version,
    npm: commandOutput(process.platform === 'win32' ? 'npm.cmd' : 'npm', ['--version']),
    go: commandOutput(process.env.GO || 'go', ['version'])
  }
}

function sampleResult(report, ordinal, phase) {
  return {
    ordinal,
    phase,
    status: report?.passed === true ? 'passed' : 'failed',
    first_runtime_event_ms: Number(report?.timings?.firstSafeProgressMs),
    structuralFailureChecks: Array.isArray(report?.failureChecks) ? report.failureChecks : []
  }
}

function unavailablePhaseTimingEvidence() {
  return {
    status: 'unverified',
    optimizationAuthorized: false,
    reason: 'the public seam does not attribute the failed p95 to an internal phase',
    phases: {
      http_admission_ms: null,
      history_read_preflight_ms: null,
      durable_append_fsync_ms: null,
      sse_publish_ms: null
    }
  }
}

export async function buildRuntimeGoRcSuiteReport(options = {}) {
  const generatedAt = new Date().toISOString()
  const source = rcSourceIdentity()
  const host = rcHostProfile()
  const toolchain = rcToolchain()
  if (!host.supported) {
    return {
      schemaVersion: 1,
      id: 'runtime-go-rc-performance-suite',
      generatedAt,
      status: 'not_configured',
      passed: false,
      rcPerformanceAuthorization: false,
      releaseAuthorization: false,
      source,
      host,
      toolchain,
      structural: { status: 'unverified', passed: false },
      benchmark: {
        status: 'unverified',
        passed: false,
        warmupCount: RC_WARMUP_COUNT,
        measuredCount: RC_MEASURED_COUNT,
        statistic: 'nearest-rank-p95',
        thresholdMs: RC_FIRST_EVENT_P95_MS,
        samples: []
      }
    }
  }

  const suiteRoot = mkdtempSync(join(tmpdir(), 'analytix-runtime-go-rc-suite-'))
  const runtimeBin = join(suiteRoot, process.platform === 'win32' ? 'runtime-server.exe' : 'runtime-server')
  const buildCommand = [process.env.GO || 'go', 'build', '-tags', 'analytix_prod', '-o', runtimeBin, './cmd/runtime-server']
  try {
    const buildStarted = Date.now()
    const build = spawnSync(buildCommand[0], buildCommand.slice(1), {
      cwd: join(repoRoot, 'packages/runtime-go'),
      env: process.env,
      encoding: 'utf8',
      stdio: 'pipe'
    })
    if (build.status !== 0 || !existsSync(runtimeBin)) {
      throw new Error(`production runtime build failed: ${`${build.stdout || ''}\n${build.stderr || ''}`.trim().slice(-1200)}`)
    }
    const runtimeBinary = {
      buildCount: 1,
      buildCommand: buildCommand.join(' '),
      buildDurationMs: Date.now() - buildStarted,
      sha256: createHash('sha256').update(readFileSync(runtimeBin)).digest('hex'),
      productionTagUsed: true
    }
    const probeOptions = {
      ...options,
      runtimeBin,
      productionTagUsed: true
    }
    const structuralReport = await buildRuntimeGoPerformanceReport({
      ...probeOptions,
      runProductSseContractTests: false
    })

    const warmups = []
    for (let index = 0; index < RC_WARMUP_COUNT; index += 1) {
      const report = await buildRuntimeGoPerformanceReport({
        ...probeOptions,
        runProductSseContractTests: false
      })
      warmups.push(sampleResult(report, index + 1, 'warmup'))
    }

    const samples = []
    for (let index = 0; index < RC_MEASURED_COUNT; index += 1) {
      const report = await buildRuntimeGoPerformanceReport({
        ...probeOptions,
        runProductSseContractTests: false
      })
      samples.push(sampleResult(report, index + 1, 'measured'))
    }
    const measuredValues = samples.map((sample) => sample.first_runtime_event_ms)
    const warmupValues = warmups.map((sample) => sample.first_runtime_event_ms)
    const validMeasuredCount = measuredValues.filter(Number.isFinite).length
    const validWarmupCount = warmupValues.filter(Number.isFinite).length
    const p50 = nearestRank(measuredValues, 0.5)
    const p95 = nearestRank(measuredValues, 0.95)
    const max = measuredValues.length > 0 ? Math.max(...measuredValues) : null
    const allSamplesStructural = [...warmups, ...samples].every((sample) => sample.status === 'passed')
    const benchmarkPassed = allSamplesStructural &&
      validWarmupCount === RC_WARMUP_COUNT &&
      validMeasuredCount === RC_MEASURED_COUNT &&
      p95 !== null &&
      p95 <= RC_FIRST_EVENT_P95_MS
    const structuralPassed = structuralReport.passed === true
    const passed = structuralPassed && benchmarkPassed
    return {
      schemaVersion: 1,
      id: 'runtime-go-rc-performance-suite',
      generatedAt,
      status: passed ? 'passed' : 'failed',
      passed,
      rcPerformanceAuthorization: passed && source.sourceBoundToHeadTree,
      releaseAuthorization: false,
      source,
      host,
      toolchain,
      runtimeBinary,
      structural: {
        status: structuralPassed ? 'passed' : 'failed',
        passed: structuralPassed,
        latencySloApplied: false,
        productSseContractTestsDelegatedToParent: true,
        failureChecks: structuralReport.failureChecks || [],
        checks: structuralReport.checks || [],
        timings: structuralReport.timings || {},
        runtimeSseProbe: structuralReport.runtimeSseProbe || {},
        usage: structuralReport.usage || {},
        measurementMode: structuralReport.measurementMode || 'not-measured',
        authorityBoundary: structuralReport.authorityBoundary || {}
      },
      benchmark: {
        status: benchmarkPassed ? 'passed' : 'failed',
        passed: benchmarkPassed,
        isolatedDataAndThreadPerSample: true,
        cacheState: {
          productionBinary: 'one-build-reused-across-structural-warmup-and-measured-runs',
          binaryWarmup: 'two-unmeasured-runs',
          dataAndThread: 'fresh-isolated-cold-root-per-run'
        },
        warmupCount: RC_WARMUP_COUNT,
        measuredCount: RC_MEASURED_COUNT,
        validWarmupCount,
        validMeasuredCount,
        statistic: 'nearest-rank-p95',
        thresholdMs: RC_FIRST_EVENT_P95_MS,
        comparison: '<=',
        warmups,
        samples,
        p50,
        p95,
        max,
        maxIsGateStatistic: false,
        ...(benchmarkPassed ? {} : { phaseTimingEvidence: unavailablePhaseTimingEvidence() })
      }
    }
  } catch (error) {
    return {
      schemaVersion: 1,
      id: 'runtime-go-rc-performance-suite',
      generatedAt,
      status: 'failed',
      passed: false,
      rcPerformanceAuthorization: false,
      releaseAuthorization: false,
      source,
      host,
      toolchain,
      message: error instanceof Error ? error.message : String(error)
    }
  } finally {
    rmSync(suiteRoot, { recursive: true, force: true })
  }
}

async function main() {
  if (args.has('--self-test-rc-contract')) {
    const ordered = Array.from({ length: 20 }, (_, index) => index + 1)
    const porcelainPrefixPreserved = parseGitPorcelainEntries(' M AGENTS.md\n')[0] === ' M AGENTS.md'
    const report = {
      schemaVersion: 1,
      id: 'runtime-go-rc-performance-contract-self-test',
      status: nearestRank(ordered, 0.5) === 10 &&
        nearestRank(ordered, 0.95) === 19 &&
        porcelainPrefixPreserved
        ? 'passed'
        : 'failed',
      warmupCount: RC_WARMUP_COUNT,
      measuredCount: RC_MEASURED_COUNT,
      thresholdMs: RC_FIRST_EVENT_P95_MS,
      p50: nearestRank(ordered, 0.5),
      p95: nearestRank(ordered, 0.95),
      porcelainPrefixPreserved
    }
    report.passed = report.status === 'passed'
    console.log(JSON.stringify(report, null, 2))
    if (gate && !report.passed) process.exitCode = 1
    return
  }
  if (args.has('--self-test-settings-resolution')) {
    const report = buildSettingsResolutionSelfTestReport()
    if (json) {
      console.log(JSON.stringify(report, null, 2))
    } else {
      console.log(`${String(report.status || 'unknown').toUpperCase()} ${report.id}`)
      console.log(`Provider: ${report.provider.providerId} model=${report.provider.model} hasApiKey=${report.provider.hasApiKey}`)
    }
    if (gate && report.passed !== true) process.exitCode = 1
    return
  }
  const report = args.has('--rc-suite')
    ? await buildRuntimeGoRcSuiteReport()
    : await buildRuntimeGoPerformanceReport()
  if (json) {
    console.log(JSON.stringify(report, null, 2))
  } else {
    console.log(`${String(report.status || 'unknown').toUpperCase()} ${report.id}`)
    if (report.timings) {
      console.log(`First safe progress: ${report.timings.firstSafeProgressMs}ms`)
      console.log(`Atomic terminal: ${report.timings.atomicTerminalMs}ms`)
      console.log(`Turn accepted: ${report.timings.turnAcceptedMs}ms`)
    }
    if (Array.isArray(report.failureChecks) && report.failureChecks.length > 0) {
      console.log(`FAILED ${report.failureChecks.join(',')}`)
    }
  }
  if (gate && report.passed !== true) process.exitCode = 1
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.stack || error.message : String(error))
    process.exitCode = 1
  })
}
