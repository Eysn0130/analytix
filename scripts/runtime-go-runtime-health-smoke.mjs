#!/usr/bin/env node

import { spawn, spawnSync } from 'node:child_process'
import http from 'node:http'
import { chmodSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import process from 'node:process'

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const jsonOutput = args.has('--json') || args.has('--dry-run')
const dryRun = args.has('--dry-run')
const reportOnly = args.has('--no-gate')
const goCommand = process.env.GO || 'go'
const timeoutMs = Number.parseInt(argValue('--timeout-ms', '30000'), 10) || 30000
const buildTimeoutMs = Number.parseInt(argValue('--build-timeout-ms', '120000'), 10) || 120000
const readyPrefix = 'ANALYTIX_RUNTIME_SERVER_READY '
const sourceUnavailableBoundaryText = '当前案件资金分析来源在本轮未通过可用性核验，因此无法核验或发布所请求的案件事实。请重新加载资金分析插件或运行时后重试。'
const ordinaryPromptMarker = 'Run the runtime health product chain smoke.'
const ordinaryProviderResponseText = 'health smoke product chain ok'
const contractProviderApiKey = 'sk-healthSmokeContractProviderKey'

function argValue(name, fallback = '') {
  const prefix = `${name}=`
  const inline = rawArgs.find((item) => item.startsWith(prefix))
  if (inline) return inline.slice(prefix.length)
  const index = rawArgs.indexOf(name)
  return index >= 0 ? rawArgs[index + 1] || '' : fallback
}

function currentGitCommit() {
  const result = spawnSync('git', ['rev-parse', 'HEAD'], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  return result.status === 0 ? String(result.stdout || '').trim() : ''
}

function runtimeEnv(overrides = {}) {
  return {
    ...process.env,
    ANALYTIX_API_KEY: '',
    ANALYTIX_MODEL_PROVIDERS: '',
    ANALYTIX_MCP_CONFIG_PATH: '',
    ANALYTIX_MCP_CONFIG_JSON: '',
    ...overrides
  }
}

function redact(value) {
  return String(value || '')
    .replace(/\bsk-[A-Za-z0-9_-]{12,}\b/g, 'sk-<redacted>')
    .replace(/Bearer\s+[A-Za-z0-9._~+/-]+=*/g, 'Bearer <redacted>')
    .replace(/(api[_-]?key|authorization|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|secret|password|signature|sig|auth|credential|jwt)\s*[:=]\s*[^,\s]+/gi, '$1=<redacted>')
}

function httpText(url, options = {}) {
  return new Promise((resolve, reject) => {
    const target = new URL(url)
    const requestBody = options.body === undefined ? undefined : String(options.body)
    const req = http.request({
      hostname: target.hostname,
      port: target.port,
      path: `${target.pathname}${target.search}`,
      protocol: target.protocol,
      method: options.method || 'GET',
      timeout: 5000,
      headers: {
        ...(requestBody === undefined ? {} : {
          'content-type': 'application/json',
          'content-length': Buffer.byteLength(requestBody)
        }),
        ...(options.headers || {})
      }
    }, (res) => {
      let responseBody = ''
      res.setEncoding('utf8')
      res.on('data', (chunk) => { responseBody += chunk })
      res.on('end', () => {
        if ((res.statusCode || 0) < 200 || (res.statusCode || 0) >= 300) {
          reject(new Error(`HTTP ${res.statusCode}: ${responseBody.slice(0, 240)}`))
          return
        }
        resolve(responseBody)
      })
    })
    req.on('timeout', () => {
      req.destroy(new Error('request timed out'))
    })
    req.on('error', reject)
    if (requestBody !== undefined) req.write(requestBody)
    req.end()
  })
}

async function httpJSON(url, options = {}) {
  const body = await httpText(url, options)
  return JSON.parse(body)
}

function prepareSyntheticProviderRegistryMasterKey(dataDir) {
  const privateDir = join(dataDir, 'private')
  const providerSecretsDir = join(privateDir, 'provider-secrets')
  const masterKeyDir = join(providerSecretsDir, 'master-key')
  for (const directory of [dataDir, privateDir, providerSecretsDir, masterKeyDir]) {
    mkdirSync(directory, { recursive: true, mode: 0o700 })
    chmodSync(directory, 0o700)
  }
  const masterKey = Buffer.alloc(32, 0x6a)
  try {
    const masterKeyPath = join(masterKeyDir, 'master.key')
    writeFileSync(masterKeyPath, masterKey, { flag: 'wx', mode: 0o600 })
    chmodSync(masterKeyPath, 0o600)
  } finally {
    masterKey.fill(0)
  }
  if (process.platform === 'darwin') {
    const authorityPath = join(masterKeyDir, 'authority.v1')
    writeFileSync(authorityPath, 'analytix-master-key-authority:v1:fallback\n', { flag: 'wx', mode: 0o600 })
    chmodSync(authorityPath, 0o600)
  }
}

function providerRegistryProjectionContainsForbiddenValue(projection, forbiddenValues) {
  const encoded = JSON.stringify(projection)
  return forbiddenValues.some((value) => encoded.includes(value))
}

function providerRegistryProjectionMatches(provider, expected) {
  return provider && typeof provider === 'object' && !Array.isArray(provider) &&
    provider.id === expected.id &&
    provider.kind === 'openai-compatible' &&
    provider.endpoint === expected.endpoint &&
    provider.selectedModel === expected.model &&
    Array.isArray(provider.models) && provider.models.length === 1 && provider.models[0] === expected.model &&
    Array.isArray(provider.selectedRoutes) && provider.selectedRoutes.length === 1 &&
    provider.selectedRoutes[0] === 'primary' && provider.credentialConfigured === true
}

async function establishExplicitProviderRegistryWinner(baseUrl, contractProvider) {
  const providerID = 'health-smoke-openai-compatible'
  const model = 'health-smoke-model'
  const endpoint = `${contractProvider.url}/v1`
  let initialRegistry
  try {
    initialRegistry = await httpJSON(`${baseUrl}/v1/provider-registry`)
  } catch {
    return { ok: false, phase: 'initial_get', resultClass: 'request_failed' }
  }
  if (!Array.isArray(initialRegistry.providers) || initialRegistry.providers.length !== 0 ||
      String(initialRegistry.selectedProviderId || '') !== '') {
    return { ok: false, phase: 'initial_get', resultClass: 'initial_state_mismatch' }
  }
  const registryRevision = String(initialRegistry.registryRevision || '')
  const registryIncarnation = String(initialRegistry.registryIncarnation || '')
  if (!registryRevision || !registryIncarnation) {
    return { ok: false, phase: 'initial_get', resultClass: 'cas_missing' }
  }

  const credentialBytes = Buffer.from(contractProviderApiKey, 'utf8')
  let encodedCredential = credentialBytes.toString('base64')
  let connectBody = JSON.stringify({
    schemaVersion: 1,
    expected: {
      registryRevision,
      registryIncarnation,
      providerRevision: '0',
      providerGeneration: '0',
      providerIncarnation: '',
      providerCredentialPurpose: ''
    },
    provider: {
      id: providerID,
      kind: 'openai-compatible',
      endpoint,
      proxy: '',
      models: [model],
      mediaModels: [],
      selectedModel: model,
      selectedMediaModel: '',
      selectedRoutes: ['primary']
    },
    credential: {
      kind: 'set',
      purpose: 'provider-api-key',
      valueBase64: encodedCredential
    }
  })
  const forbiddenValues = [
    contractProviderApiKey,
    encodedCredential,
    'credentialRef',
    'ciphertext',
    'nonce',
    'masterKey',
    'master-key',
    'valueBase64',
    'rawBody',
    'raw_body'
  ]
  try {
    let connectedRegistry
    try {
      connectedRegistry = await httpJSON(`${baseUrl}/v1/provider-registry`, {
        method: 'POST',
        body: connectBody
      })
    } catch {
      return { ok: false, phase: 'post_connect', resultClass: 'request_failed' }
    }
    if (providerRegistryProjectionContainsForbiddenValue(connectedRegistry, forbiddenValues)) {
      return { ok: false, phase: 'post_connect', resultClass: 'private_projection' }
    }
    if (!providerRegistryProjectionMatches(connectedRegistry.provider, { id: providerID, endpoint, model })) {
      return { ok: false, phase: 'post_connect', resultClass: 'provider_mismatch' }
    }

    let registryReadback
    try {
      registryReadback = await httpJSON(`${baseUrl}/v1/provider-registry`)
    } catch {
      return { ok: false, phase: 'readback', resultClass: 'request_failed' }
    }
    if (providerRegistryProjectionContainsForbiddenValue(registryReadback, forbiddenValues)) {
      return { ok: false, phase: 'readback', resultClass: 'private_projection' }
    }
    if (!Array.isArray(registryReadback.providers) || registryReadback.providers.length !== 1 ||
        registryReadback.selectedProviderId !== providerID ||
        !providerRegistryProjectionMatches(registryReadback.providers[0], { id: providerID, endpoint, model })) {
      return { ok: false, phase: 'readback', resultClass: 'winner_mismatch' }
    }
    return { ok: true, phase: 'readback', resultClass: 'winner_established' }
  } finally {
    credentialBytes.fill(0)
    encodedCredential = ''
    connectBody = ''
  }
}

async function waitForText(url, tokens) {
  const deadline = Date.now() + Math.min(timeoutMs, 15000)
  let lastText = ''
  while (Date.now() < deadline) {
    lastText = await httpText(url)
    if (tokens.every((token) => lastText.includes(token))) return lastText
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  const missing = tokens.filter((token) => !lastText.includes(token))
  throw new Error(`timed out waiting for runtime text tokens; missing ${missing.join(', ')} in ${redact(lastText.slice(0, 8000))}`)
}

function startContractProvider() {
  const requests = []
  const server = http.createServer((req, res) => {
    if (req.method !== 'POST' || req.url !== '/v1/chat/completions') {
      res.writeHead(404, { 'content-type': 'application/json' })
      res.end(JSON.stringify({ error: { message: 'contract provider route not found' } }))
      return
    }
    let raw = ''
    req.setEncoding('utf8')
    req.on('data', (chunk) => { raw += chunk })
    req.on('end', () => {
      let body = {}
      try {
        body = JSON.parse(raw)
      } catch {
        // The runtime must still receive a provider response so the smoke can
        // report the malformed request through its ordinary public boundary.
      }
      const messages = Array.isArray(body.messages) ? body.messages : []
      requests.push({
        authorizationConfigured: req.headers.authorization === `Bearer ${contractProviderApiKey}`,
        promptMarkerSeen: messages.some((message) =>
          message && message.role === 'user' && message.content === ordinaryPromptMarker)
      })
      res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
      res.end([
        `data: {"choices":[{"delta":{"content":"${ordinaryProviderResponseText}"},"finish_reason":"stop"}]}`,
        'data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15,"prompt_cache_hit_tokens":4,"prompt_cache_miss_tokens":8}}',
        'data: [DONE]'
      ].join('\n\n'))
    })
  })
  return new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      if (!address || typeof address === 'string') {
        reject(new Error('contract provider did not bind a TCP port'))
        return
      }
      server.off('error', reject)
      resolve({
        url: `http://127.0.0.1:${address.port}`,
        requests,
        close: () => new Promise((done) => server.close(() => done()))
      })
    })
  })
}

function objectField(record, field) {
  const value = record?.[field]
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {}
}

function boolField(record, field) {
  return record?.[field] === true
}

function stringField(record, field) {
  const value = record?.[field]
  return typeof value === 'string' ? value : ''
}

function numberField(record, field) {
  const value = Number(record?.[field])
  return Number.isFinite(value) ? value : 0
}

function summarizeRuntimeInfo(info) {
  const capabilities = objectField(info, 'capabilities')
  const mcp = objectField(capabilities, 'mcp')
  const mcpSearch = objectField(mcp, 'search')
  const mcpCatalog = objectField(mcp, 'catalog')
  const attachments = objectField(capabilities, 'attachments')
  const subagents = objectField(capabilities, 'subagents')
  const summary = {
    insecure: boolField(info, 'insecure'),
    mcpStatus: stringField(mcp, 'status'),
    mcpAvailable: boolField(mcp, 'available'),
    mcpEnabled: boolField(mcp, 'enabled'),
    mcpConfiguredServers: numberField(mcp, 'configuredServers'),
    mcpConnectedServers: numberField(mcp, 'connectedServers'),
    mcpToolCount: numberField(mcp, 'toolCount'),
    mcpPromptCount: numberField(mcp, 'promptCount'),
    mcpResourceCount: numberField(mcp, 'resourceCount'),
    mcpCatalogStatus: stringField(mcpCatalog, 'status'),
    mcpCatalogToolCount: numberField(mcpCatalog, 'toolCount'),
    mcpCatalogPromptCount: numberField(mcpCatalog, 'promptCount'),
    mcpCatalogResourceCount: numberField(mcpCatalog, 'resourceCount'),
    mcpSearchActive: boolField(mcpSearch, 'active'),
    attachmentsAvailable: boolField(attachments, 'available'),
    subagentsAvailable: boolField(subagents, 'available'),
    subagentsTaskToolAvailable: boolField(subagents, 'taskToolAvailable'),
    subagentsParallelTasksToolAvailable: boolField(subagents, 'parallelTasksToolAvailable'),
    subagentsDurableChildRunStore: boolField(subagents, 'durableChildRunStore'),
    subagentsTopLevelRouteExposed: boolField(subagents, 'topLevelRouteExposed')
  }
  return {
    ...summary,
    ok: summary.insecure &&
      summary.mcpStatus === 'disabled' &&
      !summary.mcpAvailable &&
      !summary.mcpEnabled &&
      summary.mcpConfiguredServers === 0 &&
      summary.mcpConnectedServers === 0 &&
      summary.mcpToolCount === 0 &&
      summary.mcpPromptCount === 0 &&
      summary.mcpResourceCount === 0 &&
      summary.mcpCatalogStatus === 'unavailable' &&
      summary.mcpCatalogToolCount === 0 &&
      summary.mcpCatalogPromptCount === 0 &&
      summary.mcpCatalogResourceCount === 0 &&
      !summary.mcpSearchActive &&
      summary.attachmentsAvailable &&
      summary.subagentsAvailable &&
      summary.subagentsTaskToolAvailable &&
      summary.subagentsParallelTasksToolAvailable &&
      summary.subagentsDurableChildRunStore &&
      !summary.subagentsTopLevelRouteExposed
  }
}

function summarizeRuntimeTools(tools) {
  const mcpSearch = objectField(tools, 'mcpSearch')
  const subagents = objectField(tools, 'subagents')
  const mcpPrompts = Array.isArray(tools?.mcpPrompts) ? tools.mcpPrompts : []
  const mcpResources = Array.isArray(tools?.mcpResources) ? tools.mcpResources : []
  const summary = {
    mcpSearchEnabled: boolField(mcpSearch, 'enabled'),
    mcpSearchMode: stringField(mcpSearch, 'mode'),
    mcpSearchActive: boolField(mcpSearch, 'active'),
    mcpSearchAvailable: boolField(mcpSearch, 'available'),
    mcpIndexedToolCount: numberField(mcpSearch, 'indexedToolCount'),
    mcpAdvertisedToolCount: numberField(mcpSearch, 'advertisedToolCount'),
    mcpPromptCount: mcpPrompts.length,
    mcpResourceCount: mcpResources.length,
    subagentsAvailable: boolField(subagents, 'available'),
    subagentsTaskToolAvailable: boolField(subagents, 'taskToolAvailable'),
    subagentsParallelTasksToolAvailable: boolField(subagents, 'parallelTasksToolAvailable'),
    subagentsTopLevelRouteExposed: boolField(subagents, 'topLevelRouteExposed')
  }
  return {
    ...summary,
    ok: !summary.mcpSearchEnabled &&
      summary.mcpSearchMode === 'auto' &&
      !summary.mcpSearchActive &&
      !summary.mcpSearchAvailable &&
      summary.mcpIndexedToolCount === 0 &&
      summary.mcpAdvertisedToolCount === 0 &&
      summary.mcpPromptCount === 0 &&
      summary.mcpResourceCount === 0 &&
      summary.subagentsAvailable &&
      summary.subagentsTaskToolAvailable &&
      summary.subagentsParallelTasksToolAvailable &&
      !summary.subagentsTopLevelRouteExposed
  }
}

async function waitForReady(child, stdoutChunks, stderrChunks, startedAt) {
  while (Date.now() - startedAt < timeoutMs) {
    const stdout = stdoutChunks.join('')
    const readyLine = stdout.split(/\r?\n/).find((line) => line.startsWith(readyPrefix))
    if (readyLine) {
      return JSON.parse(readyLine.slice(readyPrefix.length))
    }
    if (child.exitCode !== null) {
      throw new Error(`runtime-server exited before ready: ${redact(stderrChunks.join('').slice(-2000))}`)
    }
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  throw new Error(`runtime-server did not become ready within ${timeoutMs}ms`)
}

async function stopChild(child) {
  if (!child || child.exitCode !== null) return true
  child.kill('SIGTERM')
  const stopped = await Promise.race([
    new Promise((resolve) => child.once('close', () => resolve(true))),
    new Promise((resolve) => setTimeout(() => resolve(false), 3000))
  ])
  if (!stopped && child.exitCode === null) {
    child.kill('SIGKILL')
    await new Promise((resolve) => child.once('close', resolve))
  }
  return true
}

function skippedReport() {
  return {
    schemaVersion: 2,
    id: 'runtime-go-runtime-health-smoke',
    generatedAt: new Date().toISOString(),
    sourceCommit: currentGitCommit(),
    status: 'skipped',
    passed: false,
    smoke: {
      actualRuntimeProcessStarted: false,
      productionRuntime: false,
      durableRootConfigured: false,
      dataDirScope: 'temporary',
      usesRealProviderConfig: false,
      providerConfigMode: 'isolated-empty-provider-config',
      productChainMode: 'not-run',
      productChainOk: false,
      authorityBoundaryOk: false,
      hostAcceptedFinalOk: false,
      providerPositiveChainOk: false,
      providerAgentLoopReady: false,
      providerAgentLoopCoverage: 'not-run',
      providerExecutionExpected: false,
      providerExecutionBlocked: false,
      protectedTurnCreateOk: false,
      ordinaryTurnCreateOk: false,
      protectedSseReplayOk: false,
      ordinarySseReplayOk: false,
      ordinaryThreadDistinct: false,
      ordinaryPublicFinalOk: false,
      usageOk: false,
      providerUsageObserved: false,
      providerUsageAbsentOk: false,
      protectedProviderDispatchAbsent: false,
      protectedProviderRequestCount: null,
      contractProviderRequestCount: null,
      contractProviderPromptSeen: false,
      providerAuthorizationConfigured: false,
      actualPackagedAppLaunched: false,
      healthOk: false,
      runtimeInfoOk: false,
      runtimeToolsOk: false,
      productionCapabilitiesOk: false,
      secretsPrinted: false
    },
    checks: [
      {
        id: 'runtime-server-ready',
        status: 'skipped',
        durationMs: 0
      }
    ]
  }
}

async function runSmoke() {
  const startedAt = Date.now()
  const tempRoot = mkdtempSync(join(tmpdir(), 'analytix-runtime-health-'))
  const dataDir = join(tempRoot, 'data')
  const durableRoot = join(tempRoot, 'durable')
  const runtimeServerBinary = join(tempRoot, process.platform === 'win32' ? 'runtime-server.exe' : 'runtime-server')
  const stdoutChunks = []
  const stderrChunks = []
  let child
  let contractProvider
  let runtimeInfo = { ok: false }
  let runtimeTools = { ok: false }
  let productChain = { ok: false, mode: 'not-run' }
  const checks = []
  try {
    const masterKeyStartedAt = Date.now()
    try {
      prepareSyntheticProviderRegistryMasterKey(dataDir)
    } catch {
      checks.push({
        id: 'runtime-provider-registry-fixture',
        status: 'failed',
        durationMs: Date.now() - masterKeyStartedAt,
        phase: 'k1',
        resultClass: 'filesystem'
      })
      throw new Error('runtime Provider Registry fixture failed: phase=k1 class=filesystem')
    }
    contractProvider = await startContractProvider()
    const buildStartedAt = Date.now()
    const buildResult = spawnSync(goCommand, [
      'build',
      '-tags',
      'analytix_prod',
      '-o',
      runtimeServerBinary,
      './cmd/runtime-server'
    ], {
      cwd: join(process.cwd(), 'packages/runtime-go'),
      env: runtimeEnv(),
      encoding: 'utf8',
      stdio: 'pipe',
      timeout: buildTimeoutMs
    })
    const buildExitStatus = buildResult.status ?? (buildResult.signal ? 1 : 0)
    checks.push({
      id: 'runtime-server-build',
      status: buildExitStatus === 0 ? 'passed' : 'failed',
      durationMs: Date.now() - buildStartedAt,
      reason: buildExitStatus === 0 ? '' : redact(buildResult.stderr || buildResult.stdout || buildResult.error?.message || `go build exited with status ${buildExitStatus}`)
    })
    if (buildExitStatus !== 0 || buildResult.error) {
      throw new Error(checks[checks.length - 1].reason || 'go build failed')
    }

    child = spawn(runtimeServerBinary, [
      '-insecure',
      '-data-dir',
      dataDir,
      '-durable-root',
      durableRoot,
      '-provider-id',
      'health-smoke-openai-compatible',
      '-base-url',
      `${contractProvider.url}/v1`,
      '-model',
      'health-smoke-model',
      '-endpoint-format',
      'chat_completions'
    ], {
      cwd: join(process.cwd(), 'packages/runtime-go'),
      env: runtimeEnv({
        ANALYTIX_API_KEY: contractProviderApiKey
      }),
      stdio: ['ignore', 'pipe', 'pipe']
    })
    child.stdout.on('data', (chunk) => { stdoutChunks.push(String(chunk)) })
    child.stderr.on('data', (chunk) => { stderrChunks.push(String(chunk)) })

    const readinessStartedAt = Date.now()
    const ready = await waitForReady(child, stdoutChunks, stderrChunks, readinessStartedAt)
    const readyDuration = Date.now() - readinessStartedAt
    checks.push({
      id: 'runtime-server-ready',
      status: ready.productionRuntime === true ? 'passed' : 'failed',
      durationMs: readyDuration
    })
    const registryFixtureStartedAt = Date.now()
    const registryFixture = await establishExplicitProviderRegistryWinner(ready.url, contractProvider)
    checks.push({
      id: 'runtime-provider-registry-fixture',
      status: registryFixture.ok ? 'passed' : 'failed',
      durationMs: Date.now() - registryFixtureStartedAt,
      phase: registryFixture.phase,
      resultClass: registryFixture.resultClass
    })
    if (!registryFixture.ok) {
      throw new Error(`runtime Provider Registry fixture failed: phase=${registryFixture.phase} class=${registryFixture.resultClass}`)
    }
    const healthStartedAt = Date.now()
    const health = await httpJSON(`${ready.url}/health`)
    const healthOk = health.service === 'analytix' && health.mode === 'serve' && health.status === 'ok'
    checks.push({
      id: 'runtime-health',
      status: healthOk ? 'passed' : 'failed',
      durationMs: Date.now() - healthStartedAt
    })
    const infoStartedAt = Date.now()
    runtimeInfo = summarizeRuntimeInfo(await httpJSON(`${ready.url}/v1/runtime/info`))
    checks.push({
      id: 'runtime-info',
      status: runtimeInfo.ok ? 'passed' : 'failed',
      durationMs: Date.now() - infoStartedAt,
      reason: runtimeInfo.ok ? '' : `unexpected runtime info: ${JSON.stringify(runtimeInfo)}`
    })
    const toolsStartedAt = Date.now()
    runtimeTools = summarizeRuntimeTools(await httpJSON(`${ready.url}/v1/runtime/tools?refresh=1`))
    checks.push({
      id: 'runtime-tools',
      status: runtimeTools.ok ? 'passed' : 'failed',
      durationMs: Date.now() - toolsStartedAt,
      reason: runtimeTools.ok ? '' : `unexpected runtime tools: ${JSON.stringify(runtimeTools)}`
    })
    const productStartedAt = Date.now()
    productChain = await runProductChainSmoke({
      baseUrl: ready.url,
      dataDir,
      contractProvider
    })
    checks.push({
      id: 'runtime-authority-boundary',
      status: productChain.protectedBoundaryOk ? 'passed' : 'failed',
      durationMs: Date.now() - productStartedAt,
      reason: productChain.protectedBoundaryOk ? '' : `unexpected protected boundary: ${redact(JSON.stringify(productChain).slice(0, 1000))}`
    })
    checks.push({
      id: 'runtime-provider-agent-chain',
      status: productChain.providerPositiveChainOk ? 'passed' : 'failed',
      durationMs: Date.now() - productStartedAt,
      reason: productChain.providerPositiveChainOk ? '' : `unexpected provider Agent chain: ${redact(JSON.stringify(productChain).slice(0, 1000))}`
    })
    const cleanupStartedAt = Date.now()
    await stopChild(child)
    if (contractProvider) await contractProvider.close()
    checks.push({
      id: 'runtime-server-cleanup',
      status: 'passed',
      durationMs: Date.now() - cleanupStartedAt
    })
    const passed = checks.every((item) => item.status === 'passed')
    return {
      schemaVersion: 2,
      id: 'runtime-go-runtime-health-smoke',
      generatedAt: new Date().toISOString(),
      sourceCommit: currentGitCommit(),
      status: passed ? 'passed' : 'failed',
      passed,
      smoke: {
        actualRuntimeProcessStarted: true,
        productionRuntime: ready.productionRuntime === true,
        durableRootConfigured: ready.persistenceRootsConfigured === true,
        dataDirScope: 'temporary',
        usesRealProviderConfig: false,
        providerConfigMode: 'isolated-contract-provider-config',
        productChainMode: productChain.mode || 'not-run',
        productChainOk: productChain.providerPositiveChainOk === true,
        authorityBoundaryOk: productChain.authorityBoundaryOk === true,
        hostAcceptedFinalOk: productChain.hostAcceptedFinalOk === true,
        providerPositiveChainOk: productChain.providerPositiveChainOk === true,
        providerAgentLoopReady: productChain.providerAgentLoopReady === true,
        providerAgentLoopCoverage: productChain.providerAgentLoopCoverage || 'blocked-before-dispatch',
        providerExecutionExpected: productChain.providerExecutionExpected === true,
        providerExecutionBlocked: productChain.providerExecutionBlocked === true,
        protectedTurnCreateOk: productChain.protectedTurnCreateOk === true,
        ordinaryTurnCreateOk: productChain.ordinaryTurnCreateOk === true,
        protectedSseReplayOk: productChain.protectedSseReplayOk === true,
        ordinarySseReplayOk: productChain.ordinarySseReplayOk === true,
        ordinaryThreadDistinct: productChain.ordinaryThreadDistinct === true,
        ordinaryPublicFinalOk: productChain.ordinaryPublicFinalOk === true,
        turnCreateOk: productChain.turnCreateOk === true,
        sseReplayOk: productChain.sseReplayOk === true,
        usageOk: productChain.usageOk === true,
        providerUsageObserved: productChain.providerUsageObserved === true,
        providerUsageAbsentOk: productChain.providerUsageAbsentOk === true,
        protectedProviderDispatchAbsent: productChain.protectedProviderDispatchAbsent === true,
        protectedProviderRequestCount: Number.isFinite(productChain.protectedProviderRequestCount)
          ? productChain.protectedProviderRequestCount
          : null,
        attachmentOk: productChain.attachmentOk === true,
        providerDraftWithheld: productChain.providerDraftWithheld === true,
        contractProviderRequestCount: Number.isFinite(productChain.contractProviderRequestCount)
          ? productChain.contractProviderRequestCount
          : null,
        contractProviderPromptSeen: productChain.contractProviderPromptSeen === true,
        providerAuthorizationConfigured: productChain.providerAuthorizationConfigured === true,
        actualPackagedAppLaunched: false,
        healthOk,
        runtimeInfoOk: runtimeInfo.ok,
        runtimeToolsOk: runtimeTools.ok,
        productionCapabilitiesOk: runtimeInfo.ok && runtimeTools.ok,
        runtimeInfo,
        runtimeTools,
        health,
        secretsPrinted: false
      },
      checks
    }
  } catch (error) {
    if (child) await stopChild(child)
    if (contractProvider) await contractProvider.close()
    checks.push({
      id: checks.length === 0 ? 'runtime-server-ready' : 'runtime-health',
      status: 'failed',
      durationMs: Date.now() - startedAt,
      reason: error instanceof Error ? redact(error.message) : redact(String(error))
    })
    return {
      schemaVersion: 2,
      id: 'runtime-go-runtime-health-smoke',
      generatedAt: new Date().toISOString(),
      sourceCommit: currentGitCommit(),
      status: 'failed',
      passed: false,
      smoke: {
        actualRuntimeProcessStarted: Boolean(child),
        productionRuntime: false,
        durableRootConfigured: false,
        dataDirScope: 'temporary',
        usesRealProviderConfig: false,
        providerConfigMode: contractProvider
          ? 'isolated-contract-provider-config'
          : 'isolated-empty-provider-config',
        productChainMode: productChain.mode || 'not-run',
        productChainOk: productChain.providerPositiveChainOk === true,
        authorityBoundaryOk: productChain.authorityBoundaryOk === true,
        hostAcceptedFinalOk: productChain.hostAcceptedFinalOk === true,
        providerPositiveChainOk: productChain.providerPositiveChainOk === true,
        providerAgentLoopReady: productChain.providerAgentLoopReady === true,
        providerAgentLoopCoverage: productChain.providerAgentLoopCoverage || 'not-completed',
        providerExecutionExpected: productChain.providerExecutionExpected === true,
        providerExecutionBlocked: productChain.providerExecutionBlocked === true,
        protectedTurnCreateOk: productChain.protectedTurnCreateOk === true,
        ordinaryTurnCreateOk: productChain.ordinaryTurnCreateOk === true,
        protectedSseReplayOk: productChain.protectedSseReplayOk === true,
        ordinarySseReplayOk: productChain.ordinarySseReplayOk === true,
        ordinaryThreadDistinct: productChain.ordinaryThreadDistinct === true,
        ordinaryPublicFinalOk: productChain.ordinaryPublicFinalOk === true,
        turnCreateOk: productChain.turnCreateOk === true,
        sseReplayOk: productChain.sseReplayOk === true,
        usageOk: productChain.usageOk === true,
        providerUsageObserved: productChain.providerUsageObserved === true,
        providerUsageAbsentOk: productChain.providerUsageAbsentOk === true,
        protectedProviderDispatchAbsent: productChain.protectedProviderDispatchAbsent === true,
        protectedProviderRequestCount: Number.isFinite(productChain.protectedProviderRequestCount)
          ? productChain.protectedProviderRequestCount
          : null,
        attachmentOk: productChain.attachmentOk === true,
        contractProviderRequestCount: Number.isFinite(productChain.contractProviderRequestCount)
          ? productChain.contractProviderRequestCount
          : null,
        contractProviderPromptSeen: productChain.contractProviderPromptSeen === true,
        providerAuthorizationConfigured: productChain.providerAuthorizationConfigured === true,
        actualPackagedAppLaunched: false,
        healthOk: false,
        runtimeInfoOk: runtimeInfo.ok === true,
        runtimeToolsOk: runtimeTools.ok === true,
        productionCapabilitiesOk: false,
        runtimeInfo,
        runtimeTools,
        secretsPrinted: false,
        stderrTail: redact(stderrChunks.join('').slice(-2000))
      },
      checks
    }
  } finally {
    rmSync(tempRoot, { recursive: true, force: true })
  }
}

async function runProductChainSmoke({ baseUrl, dataDir, contractProvider }) {
  const protectedThread = await httpJSON(`${baseUrl}/v1/threads`, {
    method: 'POST',
    body: JSON.stringify({
      title: 'Runtime Health Protected Boundary',
      workspace: dataDir,
      providerId: 'health-smoke-openai-compatible',
      model: 'health-smoke-model',
      mode: 'agent'
    })
  })
  const protectedThreadId = stringField(protectedThread, 'id')
  if (!protectedThreadId) return { ok: false, threadCreateOk: false }

  const attachmentThread = await httpJSON(`${baseUrl}/v1/threads`, {
    method: 'POST',
    body: JSON.stringify({
      title: 'Runtime Health Attachment',
      workspace: dataDir,
      mode: 'agent'
    })
  })
  const attachmentThreadId = stringField(attachmentThread, 'id')
  if (!attachmentThreadId) return { ok: false, threadCreateOk: true, attachmentOk: false }

  const attachment = await httpJSON(`${baseUrl}/v1/attachments`, {
    method: 'POST',
    body: JSON.stringify({
      name: 'health-smoke.txt',
      mimeType: 'text/plain',
      dataBase64: 'aGVhbHRo',
      threadId: attachmentThreadId,
      workspace: dataDir
    })
  })
  const attachmentId = stringField(objectField(attachment, 'attachment'), 'id')
  if (!attachmentId) return { ok: false, threadCreateOk: true, attachmentOk: false }

  const attachmentContent = await httpJSON(`${baseUrl}/v1/attachments/${encodeURIComponent(attachmentId)}/content?thread_id=${encodeURIComponent(attachmentThreadId)}&workspace=${encodeURIComponent(dataDir)}`)
  const attachmentOk = attachmentContent.dataBase64 === 'aGVhbHRo'

  const protectedTurn = await httpJSON(`${baseUrl}/v1/threads/${encodeURIComponent(protectedThreadId)}/turns`, {
    method: 'POST',
    body: JSON.stringify({
      prompt: '请核实案件中的银行账号 6222020000000000000 与交易金额 100000 元。',
      providerId: 'health-smoke-openai-compatible',
      model: 'health-smoke-model',
      approvalPolicy: 'never',
      sandboxMode: 'read-only',
      disableUserInput: true
    })
  })
  const protectedTurnId = stringField(protectedTurn, 'turnId')
  const protectedTurnCreateOk = Boolean(protectedTurnId &&
    protectedTurn.threadId === protectedThreadId && stringField(protectedTurn, 'userMessageItemId'))
  const protectedReplay = await waitForText(`${baseUrl}/v1/threads/${encodeURIComponent(protectedThreadId)}/events?since_seq=0`, [
    'event: turn_started',
    'event: accepted_final_batch',
    '"kind":"turn_completed"',
    '"variant":"SourceUnavailableAnswer"',
    '"coverageStatus":"unavailable"',
    '"blockerCode":"current_case_source_unavailable"',
    '"receiptMetadata":{"citations":[],"count":0',
    sourceUnavailableBoundaryText
  ])
  const protectedSseReplayOk = Boolean(protectedTurnId) &&
    (protectedReplay.includes(`"turnId":"${protectedTurnId}"`) || protectedReplay.includes(`"turn_id":"${protectedTurnId}"`))
  const providerDraftWithheld = !protectedReplay.includes(ordinaryProviderResponseText) &&
    !protectedReplay.includes('event: assistant_text_delta')
  const authorityBoundaryOk = protectedReplay.includes(sourceUnavailableBoundaryText) &&
    protectedReplay.includes('event: accepted_final_batch') &&
    protectedReplay.includes('"variant":"SourceUnavailableAnswer"') &&
    protectedReplay.includes('"coverageStatus":"unavailable"') &&
    protectedReplay.includes('"blockerCode":"current_case_source_unavailable"') &&
    protectedReplay.includes('"receiptMetadata":{"citations":[],"count":0')
  const protectedProviderRequestCount = contractProvider.requests.length
  const protectedProviderDispatchAbsent = protectedProviderRequestCount === 0
  const protectedUsage = await httpJSON(`${baseUrl}/v1/usage?group_by=thread&thread_id=${encodeURIComponent(protectedThreadId)}`)
  const protectedUsageBuckets = Array.isArray(protectedUsage.buckets) ? protectedUsage.buckets : []
  const providerUsageAbsentOk = !protectedUsageBuckets.some((bucket) => {
    const record = objectField({ bucket }, 'bucket')
    return record.thread_id === protectedThreadId && Number(record.total_tokens || record.totalTokens || 0) > 0
  })
  const hostAcceptedFinalOk = authorityBoundaryOk

  const ordinaryThread = await httpJSON(`${baseUrl}/v1/threads`, {
    method: 'POST',
    body: JSON.stringify({
      title: 'Runtime Health Ordinary Provider Chain',
      workspace: dataDir,
      providerId: 'health-smoke-openai-compatible',
      model: 'health-smoke-model',
      mode: 'agent'
    })
  })
  const ordinaryThreadId = stringField(ordinaryThread, 'id')
  const ordinaryThreadDistinct = Boolean(ordinaryThreadId && ordinaryThreadId !== protectedThreadId)
  if (!ordinaryThreadDistinct) {
    return {
      ok: false,
      protectedBoundaryOk: authorityBoundaryOk && protectedTurnCreateOk && protectedSseReplayOk &&
        providerUsageAbsentOk && providerDraftWithheld && protectedProviderDispatchAbsent,
      authorityBoundaryOk,
      protectedTurnCreateOk,
      protectedSseReplayOk,
      providerUsageAbsentOk,
      providerDraftWithheld,
      protectedProviderDispatchAbsent,
      protectedProviderRequestCount,
      ordinaryThreadDistinct: false
    }
  }

  const ordinaryTurn = await httpJSON(`${baseUrl}/v1/threads/${encodeURIComponent(ordinaryThreadId)}/turns`, {
    method: 'POST',
    body: JSON.stringify({
      prompt: ordinaryPromptMarker,
      providerId: 'health-smoke-openai-compatible',
      model: 'health-smoke-model',
      approvalPolicy: 'never',
      sandboxMode: 'read-only',
      disableUserInput: true
    })
  })
  const ordinaryTurnId = stringField(ordinaryTurn, 'turnId')
  const ordinaryTurnCreateOk = Boolean(ordinaryTurnId &&
    ordinaryTurn.threadId === ordinaryThreadId && stringField(ordinaryTurn, 'userMessageItemId'))
  const ordinaryReplay = await waitForText(`${baseUrl}/v1/threads/${encodeURIComponent(ordinaryThreadId)}/events?since_seq=0`, [
    'event: turn_started',
    'event: general_terminal_batch',
    '"kind":"item_completed"',
    '"kind":"usage"',
    '"kind":"turn_completed"',
    '"ordinaryResult"',
    '"candidateOrigin":"provider_ordinary_only"',
    ordinaryProviderResponseText
  ])
  const ordinarySseReplayOk = Boolean(ordinaryTurnId) &&
    (ordinaryReplay.includes(`"turnId":"${ordinaryTurnId}"`) || ordinaryReplay.includes(`"turn_id":"${ordinaryTurnId}"`))
  const ordinaryPublicFinalOk = ordinaryReplay.includes('event: general_terminal_batch') &&
    ordinaryReplay.includes('"kind":"item_completed"') &&
    ordinaryReplay.includes('"ordinaryResult"') &&
    ordinaryReplay.includes('"candidateOrigin":"provider_ordinary_only"') &&
    ordinaryReplay.includes(ordinaryProviderResponseText) &&
    !ordinaryReplay.includes('event: assistant_text_delta')
  const ordinaryUsage = await httpJSON(`${baseUrl}/v1/usage?group_by=thread&thread_id=${encodeURIComponent(ordinaryThreadId)}`)
  const ordinaryUsageBuckets = Array.isArray(ordinaryUsage.buckets) ? ordinaryUsage.buckets : []
  const providerUsageObserved = ordinaryUsageBuckets.some((bucket) => {
    const record = objectField({ bucket }, 'bucket')
    return record.thread_id === ordinaryThreadId && Number(record.total_tokens || record.totalTokens || 0) > 0
  })
  const contractProviderRequestCount = contractProvider.requests.length
  const contractProviderPromptSeen = contractProvider.requests.some((request) => request.promptMarkerSeen === true)
  const providerAuthorizationConfigured = contractProvider.requests.some((request) => request.authorizationConfigured === true)
  const protectedBoundaryOk = protectedTurnCreateOk && protectedSseReplayOk && providerUsageAbsentOk &&
    attachmentOk && providerDraftWithheld && authorityBoundaryOk && protectedProviderDispatchAbsent
  const providerPositiveChainOk = ordinaryTurnCreateOk && ordinarySseReplayOk && ordinaryPublicFinalOk &&
    providerUsageObserved && contractProviderRequestCount === 1 && contractProviderPromptSeen &&
    providerAuthorizationConfigured
  return {
    mode: 'production-authority-enrolled-provider-agent-chain',
    ok: protectedBoundaryOk && providerPositiveChainOk,
    protectedBoundaryOk,
    threadCreateOk: true,
    attachmentOk,
    protectedTurnCreateOk,
    ordinaryTurnCreateOk,
    protectedSseReplayOk,
    ordinarySseReplayOk,
    ordinaryThreadDistinct,
    ordinaryPublicFinalOk,
    turnCreateOk: protectedTurnCreateOk && ordinaryTurnCreateOk,
    sseReplayOk: protectedSseReplayOk && ordinarySseReplayOk,
    usageOk: providerUsageObserved,
    providerUsageAbsentOk,
    providerDraftWithheld,
    authorityBoundaryOk,
    hostAcceptedFinalOk,
    providerPositiveChainOk,
    providerAgentLoopReady: providerPositiveChainOk,
    providerAgentLoopCoverage: 'ordinary-turn-with-protected-boundary',
    providerExecutionExpected: true,
    providerExecutionBlocked: contractProviderRequestCount === protectedProviderRequestCount,
    providerUsageObserved,
    protectedProviderDispatchAbsent,
    protectedProviderRequestCount,
    contractProviderRequestCount,
    contractProviderPromptSeen,
    providerAuthorizationConfigured,
    providerCredentialValueRecorded: false
  }
}

const report = dryRun ? skippedReport() : await runSmoke()
if (jsonOutput) {
  console.log(JSON.stringify(report, null, 2))
} else {
  console.log(`${String(report.status).toUpperCase()} ${report.id}`)
  for (const item of report.checks.filter((check) => check.status !== 'passed')) {
    console.log(`FAILED ${item.id}: ${item.reason || item.status}`)
  }
}

if (!reportOnly && report.status !== 'passed') process.exitCode = 1
