#!/usr/bin/env node

import { randomBytes } from 'node:crypto'
import {
  chmodSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  realpathSync,
  rmSync,
  writeFileSync
} from 'node:fs'
import http from 'node:http'
import net from 'node:net'
import { execFileSync, spawn } from 'node:child_process'
import { isAbsolute, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createK10DarwinExplicitTaskKeychain } from './k10-darwin-explicit-keychain.mjs'
import {
  exactMainCommandMatches,
  exactTaskGroupMembers,
  observedSingleInstanceLockFailure,
  normalQuitFailureCode,
  parseRuntimeStartupNumericDiagnostic,
  parseRuntimeTransitionPhase,
  parseGoStartupPhase,
  residualMembersAlreadyPinned,
  processProbeFailureKind
} from './lib/k10-child-diagnostics.mjs'
import { createPackagedStartupTraceRecorder } from './runtime-go-packaged-milestone-a.mjs'

const repoRoot = resolve(fileURLToPath(new URL('..', import.meta.url)))
const lane = process.argv.includes('--packaged') ? 'packaged' : 'development'
const appPathIndex = process.argv.indexOf('--app-path')
const packagedAppPath = appPathIndex >= 0 ? process.argv[appPathIndex + 1] ?? '' : ''
const latencyObservation = process.argv.includes('--latency-observation')
const traceTransitions = process.argv.includes('--trace-transitions')
const ordinaryFileProbe = process.argv.includes('--ordinary-file-probe')
if (ordinaryFileProbe && (lane !== 'packaged' || !latencyObservation)) {
  throw new Error('ordinary_file_probe_requires_packaged_latency_observation')
}
const SAFE_PATH = /^\/[A-Za-z0-9._/-]+$/u
const MAX_PROVIDER_BODY_BYTES = 1 << 20
const MAX_SCAN_FILE_BYTES = 16 << 20
const MAX_CHILD_LINE_BYTES = 1 << 20
const WAIT_TIMEOUT_MS = 120_000
const STARTUP_TARGET_TIMEOUT_MS = latencyObservation ? 300_000 : WAIT_TIMEOUT_MS
const PROCESS_PROBE_TIMEOUT_MS = 2_000
const forbiddenPublicKeys = new Set([
  'apiKey', 'credential', 'credentialRef', 'keychainDBPath', 'ownerBinding',
  'password', 'path', 'secret', 'token'
])
const settingsSensitiveKeys = new Set([
  'apiKey', 'credential', 'credentialRef', 'keychainDBPath', 'ownerBinding',
  'password', 'token'
])
const settingsFilesystemPathKeys = new Set([
  'activeWorkspaceRoot', 'dataDir', 'defaultWorkspaceRoot', 'workspaceRoot'
])

let stage = 'preflight'
let exactTaskRoot = ''
let cleanedExactTaskRoot = false
let keychain = null
let activeChild = null
let providerServer = null
let denyProxy = null
let credential = ''
let prompt = ''
let reply = ''
let ordinaryFileReadToolName = ''
let settingsPath = ''
let keychainDatabasePath = ''
let providerBaseUrl = ''
let runtimeDataDirPath = ''
let replyObservedInRenderer = false
let lastChildFailureFacts = null
let lastUiFacts = null
let activeChildFailureSnapshot = null
let firstSubmittedProviderRequestAtMs = 0
let submittedProviderRequestArmed = false
let latencyFailed = false
let taskCleanupFailure = ''
let lastTaskGroupIdentityFailure = ''
let lastNormalQuitFacts = null
let partialLatencyEvidence = null
let lastTaskMainIdentity = null
let ownedGoIdentity = null
let ownedGroupMemberIdentities = new Map()
let goProbeUnknownCount = 0
let primaryFailureReasonCode = ''
const processProbe = { count: 0, elapsedMs: 0, timeoutCount: 0 }
const launchEvidence = []
const providerObservations = []
const providerProbeObservations = []
let denyProxyConnectionCount = 0

function output(value) {
  process.stdout.write(`${JSON.stringify(value)}\n`)
}

function sleep(ms) {
  return new Promise((resolvePromise) => setTimeout(resolvePromise, ms))
}

function boundedProcessProbe(command, args) {
  const startedAtMs = Date.now()
  processProbe.count += 1
  try {
    return execFileSync(command, args, {
      encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'],
      timeout: PROCESS_PROBE_TIMEOUT_MS
    })
  } catch (error) {
    if (error?.code === 'ETIMEDOUT') processProbe.timeoutCount += 1
    throw error
  } finally {
    processProbe.elapsedMs += Date.now() - startedAtMs
  }
}

function processProbeTimedOut(error) {
  return error?.code === 'ETIMEDOUT'
}

function exactOwnerOnlyDirectory(path) {
  try {
    const info = lstatSync(path)
    return info.isDirectory() && !info.isSymbolicLink() && info.uid === process.getuid() &&
      (info.mode & 0o077) === 0 && realpathSync(path) === path
  } catch {
    return false
  }
}

function strictDescendant(parent, candidate) {
  const value = relative(parent, candidate)
  return Boolean(value) && value !== '..' && !value.startsWith(`..${sep}`) && !isAbsolute(value)
}

function allocatePort() {
  return new Promise((resolvePort, reject) => {
    const server = net.createServer()
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      if (!address || typeof address === 'string') {
        server.close()
        reject(new Error('port_unavailable'))
        return
      }
      const port = address.port
      server.close((error) => error ? reject(error) : resolvePort(port))
    })
  })
}

function listen(server) {
  return new Promise((resolvePort, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      server.off('error', reject)
      const address = server.address()
      if (!address || typeof address === 'string') {
        reject(new Error('listen_failed'))
        return
      }
      resolvePort(address.port)
    })
  })
}

function closeServer(server) {
  if (!server?.listening) return Promise.resolve()
  return new Promise((resolveClose) => server.close(() => resolveClose()))
}

function terminateTaskProcessGroup(child) {
  if (!child?.pid || child.exitCode !== null || child.signalCode) return
  try {
    process.kill(-child.pid, 'SIGTERM')
  } catch {
    child.kill('SIGTERM')
  }
}

function processDefinitelyExited(pid) {
  try {
    process.kill(pid, 0)
    return false
  } catch (error) {
    return error?.code === 'ESRCH'
  }
}

async function pinExactPackagedMainBirth(child) {
  if (!latencyObservation) return
  if (!Number.isInteger(child?.pid) || child.pid <= 0) {
    throw new Error('exact_main_birth_unavailable')
  }
  const deadline = Date.now() + 10_000
  while (Date.now() < deadline) {
    if (child.exitCode !== null || child.signalCode || processDefinitelyExited(child.pid)) {
      throw new Error('exact_main_exited_before_birth_pin')
    }
    try {
      const command = boundedProcessProbe('ps', ['-p', String(child.pid), '-o', 'ppid=,pgid=,command='])
      const born = boundedProcessProbe('ps', ['-p', String(child.pid), '-o', 'lstart=']).trim()
      if (born && exactMainCommandMatches(command, process.pid, child.pid,
        packagedAppPath, child.analytixDebugPort) &&
        boundedProcessProbe('ps', ['-p', String(child.pid), '-o', 'lstart=']).trim() === born) {
        child.analytixMainBirth = born
        lastTaskMainIdentity = { pid: child.pid, born }
        return
      }
    } catch (error) {
      if (processProbeFailureKind(error, 'ps') === 'timeout') {
        throw new Error('process_probe_timeout')
      }
    }
    await sleep(100)
  }
  throw new Error('exact_main_birth_unavailable')
}

function exactPackagedTaskGroupState(child) {
  const untrusted = (reason) => {
    lastTaskGroupIdentityFailure = reason
    return 'untrusted'
  }
  if (!latencyObservation || !Number.isInteger(child?.pid) || child.pid <= 0 ||
      !Number.isInteger(child.analytixDebugPort) || !child.analytixMainBirth) {
    return untrusted('missing_task_identity')
  }
  if (child.exitCode !== null || child.signalCode || processDefinitelyExited(child.pid)) return 'gone'
  let main = ''
  let born = ''
  try {
    main = boundedProcessProbe('ps', ['-p', String(child.pid), '-o', 'ppid=,pgid=,command=']).trim()
    born = boundedProcessProbe('ps', ['-p', String(child.pid), '-o', 'lstart=']).trim()
  } catch (error) {
    return processDefinitelyExited(child.pid) ? 'gone' :
      untrusted(processProbeTimedOut(error) ? 'main_probe_timeout' : 'main_probe_unknown')
  }
  if (!born) return untrusted('main_birth_unavailable')
  if (child.analytixMainBirth !== born) {
    return untrusted('main_birth_changed')
  }
  if (!exactMainCommandMatches(main, process.pid, child.pid,
    packagedAppPath, child.analytixDebugPort)) return untrusted('main_identity_changed')
  const contentsRoot = resolve(packagedAppPath, '../..')
  let processes = ''
  try {
    processes = boundedProcessProbe('ps', ['-axo', 'pid=,pgid=,command='])
  } catch { return untrusted('group_probe_unavailable') }
  const group = exactTaskGroupMembers(processes, child.pid, contentsRoot)
  if (!group || group.length === 0 || !group.includes(child.pid)) {
    return untrusted('group_member_untrusted')
  }
  lastTaskGroupIdentityFailure = ''
  return 'matched'
}

async function stopExactPackagedTaskGroup(child) {
  let state = exactPackagedTaskGroupState(child)
  if (state === 'gone') return { signalSent: false, sigkillSent: false }
  if (state !== 'matched') throw new Error('exact_packaged_task_group_untrusted')
  pinExactTaskGroupMembers(child)
  process.kill(-child.pid, 'SIGTERM')
  for (let attempt = 0; attempt < 40; attempt += 1) {
    await sleep(250)
    state = exactPackagedTaskGroupState(child)
    if (state === 'gone') return { signalSent: true, sigkillSent: false }
  }
  if (state !== 'matched') throw new Error('exact_packaged_task_group_changed')
  process.kill(-child.pid, 'SIGKILL')
  for (let attempt = 0; attempt < 40; attempt += 1) {
    await sleep(250)
    state = exactPackagedTaskGroupState(child)
    if (state === 'gone') return { signalSent: true, sigkillSent: true }
  }
  throw new Error('exact_packaged_task_group_did_not_exit')
}

function exactGroupMemberIdentity(pid, groupPid) {
  if (!Number.isInteger(pid) || pid <= 0) return null
  try {
    const command = boundedProcessProbe('ps', ['-p', String(pid), '-o', 'pgid=,command=']).trim()
    const born = boundedProcessProbe('ps', ['-p', String(pid), '-o', 'lstart=']).trim()
    const match = command.match(/^(\d+)\s+(.+)$/u)
    return match && Number(match[1]) === groupPid && born &&
      match[2].startsWith(`${resolve(packagedAppPath, '../..')}/`)
      ? { pid, groupPid, born, command: match[2] } : null
  } catch { return null }
}

function sameExactGroupMember(identity) {
  const current = exactGroupMemberIdentity(identity.pid, identity.groupPid)
  return Boolean(current && current.born === identity.born &&
    current.command === identity.command)
}

function pinExactTaskGroupMembers(child) {
  if (exactPackagedTaskGroupState(child) !== 'matched') {
    throw new Error('residual_main_identity_unavailable')
  }
  let processes = ''
  try {
    processes = boundedProcessProbe('ps', ['-axo', 'pid=,pgid=,command='])
  } catch { throw new Error('residual_group_probe_unavailable') }
  const members = exactTaskGroupMembers(processes, child.pid, resolve(packagedAppPath, '../..'))
  if (!members || !members.includes(child.pid) || members.length > 24) {
    throw new Error('residual_group_untrusted')
  }
  const identities = members.map((pid) => exactGroupMemberIdentity(pid, child.pid))
  if (identities.some((value) => !value) || exactPackagedTaskGroupState(child) !== 'matched') {
    throw new Error('residual_member_identity_unavailable')
  }
  ownedGroupMemberIdentities = new Map(identities.map((value) => [value.pid, value]))
}

async function stopExactTaskOwnedResiduals() {
  if (!lastTaskMainIdentity) throw new Error('residual_main_identity_unavailable')
  let processes = ''
  try {
    processes = boundedProcessProbe('ps', ['-axo', 'pid=,pgid=,command='])
  } catch { throw new Error('residual_group_probe_unavailable') }
  const members = exactTaskGroupMembers(processes, lastTaskMainIdentity.pid,
    resolve(packagedAppPath, '../..'))
  if (!residualMembersAlreadyPinned(members, ownedGroupMemberIdentities)) {
    throw new Error('residual_group_untrusted')
  }
  const identities = members.map((pid) => ownedGroupMemberIdentities.get(pid))
  for (const identity of identities) {
    if (!sameExactGroupMember(identity)) continue
    try { process.kill(identity.pid, 'SIGTERM') } catch { /* exited */ }
  }
  for (let attempt = 0; attempt < 20 && identities.some(sameExactGroupMember); attempt += 1) {
    await sleep(100)
  }
  for (const identity of identities) {
    if (!sameExactGroupMember(identity)) continue
    try { process.kill(identity.pid, 'SIGKILL') } catch { /* exited */ }
  }
  for (let attempt = 0; attempt < 20 && identities.some(sameExactGroupMember); attempt += 1) {
    await sleep(100)
  }
  if (ownedGoIdentity && sameExactGoIdentity(ownedGoIdentity)) {
    try { process.kill(ownedGoIdentity.pid, 'SIGTERM') } catch { /* exited */ }
    for (let attempt = 0; attempt < 20 && sameExactGoIdentity(ownedGoIdentity); attempt += 1) {
      await sleep(100)
    }
    if (sameExactGoIdentity(ownedGoIdentity)) {
      try { process.kill(ownedGoIdentity.pid, 'SIGKILL') } catch { /* exited */ }
      for (let attempt = 0; attempt < 20 && sameExactGoIdentity(ownedGoIdentity); attempt += 1) {
        await sleep(100)
      }
    }
  }
  if (taskOwnedResidualProcessCount() !== 0) throw new Error('residual_cleanup_unverified')
}

function createProviderServer() {
  return http.createServer((request, response) => {
    if (request.method === 'GET' && request.url === '/v1/models') {
      const authorizationMatched = request.headers.authorization === `Bearer ${credential}`
      providerProbeObservations.push({ authorizationMatched })
      response.writeHead(authorizationMatched ? 200 : 401, {
        'content-type': 'application/json',
        'cache-control': 'no-store'
      })
      response.end(authorizationMatched
        ? '{"data":[{"id":"deepseek-v4-flash"}]}'
        : '{"error":{"message":"unauthorized"}}')
      return
    }
    if (request.method !== 'POST' || request.url !== '/v1/chat/completions') {
      response.writeHead(404, { 'content-type': 'application/json' })
      response.end('{"error":{"message":"not found"}}')
      return
    }
    const chunks = []
    let byteCount = 0
    let rejected = false
    request.on('data', (chunk) => {
      byteCount += chunk.length
      if (byteCount > MAX_PROVIDER_BODY_BYTES) {
        rejected = true
        request.destroy()
        for (const item of chunks) item.fill(0)
        chunks.length = 0
        return
      }
      chunks.push(Buffer.from(chunk))
    })
    request.on('end', () => {
      if (rejected) return
      if (submittedProviderRequestArmed && !firstSubmittedProviderRequestAtMs) {
        firstSubmittedProviderRequestAtMs = Date.now()
      }
      const body = Buffer.concat(chunks)
      for (const item of chunks) item.fill(0)
      let parsed = null
      try {
        parsed = JSON.parse(body.toString('utf8'))
      } catch {
        parsed = null
      }
      const messages = Array.isArray(parsed?.messages) ? parsed.messages : []
      const advertisedReadTool = Array.isArray(parsed?.tools) && parsed.tools.find((tool) =>
        tool?.type === 'function' && ['read_file', 'read'].includes(tool.function?.name))
      const readToolAdvertised = Boolean(advertisedReadTool)
      const readResultSeen = messages.some((message) => message?.role === 'tool' &&
        typeof message.content === 'string' && message.content.includes('2026-07-01') &&
        message.content.includes('1234.56') && message.content.includes('REF-17'))
      providerObservations.push(Object.freeze({
        authorizationMatched: request.headers.authorization === `Bearer ${credential}`,
        pathMatched: request.url === '/v1/chat/completions',
        streamRequested: parsed?.stream === true,
        modelConfigured: typeof parsed?.model === 'string' && parsed.model.length > 0,
        promptSeen: messages.some((message) =>
          message && message.role === 'user' && typeof message.content === 'string' &&
          message.content.includes(prompt)),
        bodyBytes: body.length,
        readToolAdvertised,
        readResultSeen
      }))
      body.fill(0)
      parsed = null
      if (ordinaryFileProbe && ((providerObservations.length === 1 && !readToolAdvertised) ||
        (providerObservations.length === 2 && !readResultSeen))) {
        response.writeHead(409, { 'content-type': 'application/json' })
        response.end('{"error":{"message":"synthetic read result missing"}}')
        return
      }
      response.writeHead(200, {
        'content-type': 'text/event-stream; charset=utf-8',
        'cache-control': 'no-store'
      })
      if (ordinaryFileProbe && providerObservations.length === 1) {
        ordinaryFileReadToolName = advertisedReadTool.function.name
        response.end([
          `data: ${JSON.stringify({ choices: [{ delta: { tool_calls: [{
            index: 0, id: 'n04_read_once', type: 'function',
            function: { name: ordinaryFileReadToolName, arguments: JSON.stringify({ path: '2026年资料/表单 42.txt' }) }
          }] }, finish_reason: 'tool_calls' }] })}`,
          'data: [DONE]'
        ].join('\n\n'))
        return
      }
      response.end([
        `data: ${JSON.stringify({ choices: [{ delta: { content: reply }, finish_reason: 'stop' }] })}`,
        'data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}',
        'data: [DONE]'
      ].join('\n\n'))
    })
  })
}

function createDenyProxy() {
  return net.createServer((socket) => {
    denyProxyConnectionCount += 1
    socket.destroy()
  })
}

function providerObservationFacts() {
  return {
    probeRequestCount: providerProbeObservations.length,
    probeAllAuthorizationMatched: providerProbeObservations.every((item) => item.authorizationMatched),
    requestCount: providerObservations.length,
    allAuthorizationMatched: providerObservations.every((item) => item.authorizationMatched),
    allLoopbackPathMatched: providerObservations.every((item) => item.pathMatched),
    allStreaming: providerObservations.every((item) => item.streamRequested),
    allModelConfigured: providerObservations.every((item) => item.modelConfigured),
    promptObserved: providerObservations.some((item) => item.promptSeen),
    allBodiesNonEmpty: providerObservations.every((item) => item.bodyBytes > 0),
    ordinaryFileReadToolAdvertised: providerObservations.some((item) => item.readToolAdvertised),
    ordinaryFileReadResultObserved: providerObservations.some((item) => item.readResultSeen)
  }
}

function seedKeyFreeSettings(userDataDir, runtimeDataDir) {
  settingsPath = join(userDataDir, 'analytix-settings.json')
  const settings = {
    version: 1,
    locale: 'en',
    // Make the isolated window-close choice explicit for the quit observation.
    appBehavior: { closeAction: 'quit' },
    provider: {
      activeProviderId: '',
      baseUrl: providerBaseUrl,
      proxy: { enabled: false, url: '' },
      providers: [{
        id: 'deepseek',
        name: 'DeepSeek',
        baseUrl: providerBaseUrl,
        endpointFormat: 'chat_completions',
        models: ['deepseek-v4-flash'],
        modelProfiles: {}
      }]
    },
    runtime: {
      autoStart: true,
      dataDir: runtimeDataDir,
      providerId: '',
      model: 'deepseek-v4-flash'
    },
    workspaceRoot: join(exactTaskRoot, 'workspace')
  }
  writeFileSync(settingsPath, `${JSON.stringify(settings, null, 2)}\n`, { mode: 0o600 })
}

function safeChildEnvironment(userDataDir, denyProxyPort) {
  const env = { ...process.env }
  for (const key of Object.keys(env)) {
    if (
      /(?:API_KEY|AUTH_TOKEN|CREDENTIAL|PASSWORD|PRIVATE_KEY|SECRET)$/u.test(key) ||
      /^(?:ANALYTIX_(?:HUB|AGENT_PLUGIN_MARKETPLACE|PROVIDER_AUDIT|TEST_BOOTSTRAP)|OPENAI_API_KEY|DEEPSEEK_API_KEY|ANTHROPIC_API_KEY)$/u.test(key)
    ) {
      delete env[key]
    }
  }
  delete env.ELECTRON_RUN_AS_NODE
  delete env.ANALYTIX_DATA_DIR
  delete env.ANALYTIX_RUNTIME_TOKEN
  env.ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE = 'isolated-local-v1'
  env.ANALYTIX_USER_DATA_DIR = userDataDir
  env.ANALYTIX_HUB_ACTIVITY_OBSERVATION = '1'
  env.ANALYTIX_STARTUP_TRACE = '1'
  if (latencyObservation) env.ANALYTIX_THREAD_TRACE = '1'
  env.HTTP_PROXY = `http://127.0.0.1:${denyProxyPort}`
  env.HTTPS_PROXY = env.HTTP_PROXY
  env.ALL_PROXY = env.HTTP_PROXY
  env.NO_PROXY = '127.0.0.1,localhost'
  env.no_proxy = env.NO_PROXY
  return env
}

function parseHubObservationLine(line, observations) {
  const marker = '[hub-activity-observation]'
  const markerIndex = line.indexOf(marker)
  if (markerIndex < 0) return
  const raw = line.slice(markerIndex + marker.length).trim()
  try {
    const value = JSON.parse(raw)
    if (
      value?.schemaVersion === 1 &&
      (value.stage === 'ordinary_startup_ready' || value.stage === 'activity_changed') &&
      ['moduleLoad', 'serviceInstance', 'refreshTimer', 'tokenRead', 'request', 'fallback']
        .every((key) => Number.isInteger(value[key]) && value[key] >= 0) &&
      typeof value.anyActivity === 'boolean'
    ) {
      observations.push(Object.freeze({
        stage: value.stage,
        moduleLoad: value.moduleLoad,
        serviceInstance: value.serviceInstance,
        refreshTimer: value.refreshTimer,
        tokenRead: value.tokenRead,
        request: value.request,
        fallback: value.fallback,
        anyActivity: value.anyActivity
      }))
    }
  } catch {
    // Non-contract child output is intentionally discarded.
  }
}

function startChild({ debugPort, userDataDir, denyProxyPort }) {
  const chromiumArgs = [
    `--proxy-server=http://127.0.0.1:${denyProxyPort}`,
    '--disable-background-networking',
    '--disable-component-update',
    '--no-default-browser-check',
    '--no-first-run'
  ]
  const electronViteBin = join(repoRoot, 'node_modules', 'electron-vite', 'bin', 'electron-vite.js')
  const command = lane === 'development' ? process.execPath : packagedAppPath
  const args = lane === 'development'
    ? [electronViteBin, 'dev', '--remoteDebuggingPort', String(debugPort), '--', ...chromiumArgs]
    : [`--remote-debugging-port=${debugPort}`, ...chromiumArgs]
  const observations = []
  let line = ''
  let stdoutBytes = 0
  let stderrBytes = 0
  let diagnosticTail = ''
  const diagnosticKinds = new Set()
  const traceStages = new Set()
  const eventCounts = {
    mainInfo: 0,
    mainWarn: 0,
    mainError: 0,
    launcherMainBuilt: 0,
    launcherPreloadBuilt: 0,
    launcherRendererReady: 0,
    launcherElectronStarted: 0
  }
  const startedAtMs = Date.now()
  const checkpointReceivedAtMs = Object.create(null)
  const startupNumericDiagnostics = {
    maxEventLoopLagMs: null,
    maxConcurrentWaitForHealthProbes: null
  }
  let goStartupAttemptCount = 0
  const goStartupAttempts = []
  const runtimeTransitions = []
  const startupTrace = latencyObservation
    ? createPackagedStartupTraceRecorder(({ checkpoint }) => {
      checkpointReceivedAtMs[checkpoint] = Date.now()
    })
    : null
  const classifyDiagnostic = (chunk) => {
    diagnosticTail = `${diagnosticTail}${chunk.toString('utf8')}`.slice(-65_536)
    const patterns = [
      ['desktop_external_state_invalid', /desktop external-state isolation configuration is invalid/iu],
      ['darwin_keychain_binding_unavailable', /Darwin Secret Store task Keychain binding is unavailable/iu],
      ['electron_vite_start_error', /error during start dev server and electron app/iu],
      ['address_in_use', /EADDRINUSE/iu],
      ['module_not_found', /ERR_MODULE_NOT_FOUND|Cannot find (?:package|module)/iu],
      ['missing_file', /ENOENT/iu],
      ['syntax_error', /SyntaxError/iu],
      ['type_error', /TypeError/iu]
    ]
    for (const [kind, pattern] of patterns) {
      if (pattern.test(diagnosticTail)) diagnosticKinds.add(kind)
    }
    if (observedSingleInstanceLockFailure(diagnosticTail)) {
      diagnosticKinds.add('single_instance_lock_false')
    }
    for (const label of [
      'main module evaluated', 'legacy data migration barrier ready', 'app icon loaded',
      'single instance lock checked', 'app.whenReady:start', 'settings load:start',
      'settings load:done', 'ipc registration:start', 'ipc registration:done',
      'createWindow:start', 'createWindow:load', 'createWindow:returned',
      'window:did-finish-load', 'window:did-fail-load'
    ]) {
      if (diagnosticTail.includes(label)) traceStages.add(label)
    }
  }
  const child = spawn(command, args, {
    cwd: repoRoot,
    env: safeChildEnvironment(userDataDir, denyProxyPort),
    detached: true,
    stdio: ['ignore', 'pipe', 'pipe']
  })
  child.analytixDebugPort = debugPort
  activeChild = child
  activeChildFailureSnapshot = () => ({
    exitObserved: false,
    exitCode: null,
    signaled: false,
    diagnosticKinds: [...diagnosticKinds].sort(),
    traceStages: [...traceStages],
    eventCounts: { ...eventCounts },
    stdoutBytes,
    stderrBytes,
    ...(startupTrace ? {
      launchElapsedMs: Date.now() - startedAtMs,
      startupTrace: startupTrace.evidence()
    } : {})
  })
  child.stdout.on('data', (chunk) => {
    startupTrace?.accept('stdout', chunk)
    stdoutBytes += chunk.length
    classifyDiagnostic(chunk)
    line += chunk.toString('utf8')
    if (Buffer.byteLength(line) > MAX_CHILD_LINE_BYTES) line = line.slice(-MAX_CHILD_LINE_BYTES)
    for (;;) {
      const index = line.indexOf('\n')
      if (index < 0) break
      const completedLine = line.slice(0, index)
      const phase = parseGoStartupPhase(completedLine)
      const transition = parseRuntimeTransitionPhase(completedLine)
      if (transition && runtimeTransitions.length < 32) runtimeTransitions.push(transition)
      if (phase?.phase === 'preflightBegin') {
        goStartupAttemptCount += 1
        if (goStartupAttempts.length < 8) goStartupAttempts.push({ preflightBegin: phase.elapsedMs })
      } else if (phase && goStartupAttempts.length > 0 && goStartupAttemptCount <= 8) {
        goStartupAttempts.at(-1)[phase.phase] = phase.elapsedMs
      }
      const numeric = parseRuntimeStartupNumericDiagnostic(completedLine)
      if (numeric) {
        startupNumericDiagnostics[numeric.key] = Math.max(
          startupNumericDiagnostics[numeric.key] ?? 0, numeric.value
        )
      }
      parseHubObservationLine(completedLine, observations)
      if (completedLine.includes('event=main_info')) eventCounts.mainInfo += 1
      if (completedLine.includes('event=main_warn')) eventCounts.mainWarn += 1
      if (completedLine.includes('event=main_error')) eventCounts.mainError += 1
      if (completedLine.includes('build the electron main process successfully')) eventCounts.launcherMainBuilt += 1
      if (completedLine.includes('build the electron preload files successfully')) eventCounts.launcherPreloadBuilt += 1
      if (completedLine.includes('dev server running for the electron renderer process')) eventCounts.launcherRendererReady += 1
      if (completedLine.includes('start electron app')) eventCounts.launcherElectronStarted += 1
      line = line.slice(index + 1)
    }
  })
  child.stderr.on('data', (chunk) => {
    startupTrace?.accept('stderr', chunk)
    stderrBytes += chunk.length
    classifyDiagnostic(chunk)
  })
  const closed = new Promise((resolveClosed) => {
    child.once('close', (code, signal) => {
      startupTrace?.finish('stdout')
      startupTrace?.finish('stderr')
      parseHubObservationLine(line, observations)
      line = ''
      lastChildFailureFacts = {
        exitObserved: true,
        exitCode: Number.isInteger(code) ? code : null,
        signaled: typeof signal === 'string' && signal.length > 0,
        diagnosticKinds: [...diagnosticKinds].sort(),
        traceStages: [...traceStages],
        eventCounts,
        stdoutBytes,
        stderrBytes,
        ...(startupTrace ? {
          launchElapsedMs: Date.now() - startedAtMs,
          startupTrace: startupTrace.evidence()
        } : {})
      }
      diagnosticTail = ''
      resolveClosed({ code, signal })
    })
  })
  return {
    child,
    closed,
    observation: () => ({ observations: [...observations], stdoutBytes, stderrBytes,
      ...((latencyObservation || traceTransitions) ? {
        goStartupAttemptCount,
        goStartupAttempts: goStartupAttempts.map((attempt) => ({ ...attempt })),
        runtimeTransitions: runtimeTransitions.map((transition) => ({ ...transition }))
      } : {}),
      ...(startupTrace ? {
        startupTrace: startupTrace.evidence(),
        checkpointReceivedAtMs: { ...checkpointReceivedAtMs },
        startupNumericDiagnostics: { ...startupNumericDiagnostics },
        startedAtMs
      } : {}) })
  }
}

async function waitForTarget(debugPort, timeoutMs = WAIT_TIMEOUT_MS) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (latencyObservation && !ownedDebugPortReady(debugPort)) {
      await sleep(250)
      continue
    }
    try {
      const response = await fetch(`http://127.0.0.1:${debugPort}/json/list`, {
        signal: AbortSignal.timeout(1000)
      })
      if (response.ok) {
        const targets = await response.json()
        const pages = Array.isArray(targets)
          ? targets.filter((target) => target?.type === 'page' && target?.webSocketDebuggerUrl)
          : []
        if (pages.length === 1) return pages[0]
      }
    } catch {
      // The exact child may still be starting or rebuilding.
    }
    await sleep(250)
  }
  throw new Error('cdp_target_timeout')
}

function ownedDebugPortReady(port) {
  if (!Number.isInteger(activeChild?.pid) || activeChild.pid <= 0 ||
      activeChild.exitCode !== null || activeChild.signalCode) {
    throw new Error('exact_main_process_unavailable')
  }
  let owners = []
  try {
    owners = [...new Set(boundedProcessProbe('lsof', ['-ti', `tcp:${port}`, '-sTCP:LISTEN'])
      .split(/\r?\n/u).map((value) => Number(value.trim()))
      .filter((value) => Number.isInteger(value) && value > 0))]
  } catch (error) {
    if (processProbeTimedOut(error)) throw new Error('process_probe_timeout')
    return false
  }
  if (owners.length === 0) return false
  if (owners.length !== 1 || owners[0] !== activeChild.pid) {
    throw new Error('exact_main_debug_port_owner_mismatch')
  }
  return true
}

async function waitForTargetClosed(debugPort, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  let emptyPageObservations = 0
  while (Date.now() < deadline) {
    if (activeChild && (activeChild.exitCode !== null || activeChild.signalCode ||
      processDefinitelyExited(activeChild.pid))) return true
    try {
      const response = await fetch(`http://127.0.0.1:${debugPort}/json/list`, {
        signal: AbortSignal.timeout(1000)
      })
      const targets = response.ok ? await response.json() : null
      if (Array.isArray(targets) && !targets.some((target) => target?.type === 'page')) {
        emptyPageObservations += 1
        if (emptyPageObservations >= 2) return true
      } else emptyPageObservations = 0
    } catch {
      emptyPageObservations = 0
    }
    await sleep(250)
  }
  return Boolean(activeChild && (activeChild.exitCode !== null || activeChild.signalCode ||
    processDefinitelyExited(activeChild.pid)))
}

async function dispatchCdp(webSocketDebuggerUrl, commands, timeoutMs = 20_000, onCommandSent = () => {}) {
  const endpoint = latencyObservation ? new URL(webSocketDebuggerUrl) : null
  const port = Number(endpoint?.port)
  if (latencyObservation && (
    endpoint.protocol !== 'ws:' || endpoint.hostname !== '127.0.0.1' ||
    !Number.isInteger(port) || port <= 0 ||
    !endpoint.pathname.startsWith('/devtools/page/') ||
    !ownedDebugPortReady(port))) throw new Error('exact_main_debug_target_untrusted')
  const socket = new WebSocket(webSocketDebuggerUrl)
  await new Promise((resolveOpen, reject) => {
    socket.addEventListener('open', resolveOpen, { once: true })
    socket.addEventListener('error', reject, { once: true })
  })
  let nextId = 0
  try {
    if (latencyObservation && !ownedDebugPortReady(port)) {
      throw new Error('exact_main_debug_port_owner_unavailable')
    }
    const results = []
    for (const command of commands) {
      if (latencyObservation && !ownedDebugPortReady(port)) {
        throw new Error('exact_main_debug_port_owner_unavailable')
      }
      const id = ++nextId
      const result = await new Promise((resolveResult, reject) => {
        const timer = setTimeout(() => reject(new Error('cdp_command_timeout')), timeoutMs)
        const listener = (event) => {
          let message
          try {
            message = JSON.parse(String(event.data))
          } catch {
            return
          }
          if (message.id !== id) return
          socket.removeEventListener('message', listener)
          clearTimeout(timer)
          if (message.error || message.result?.exceptionDetails) {
            reject(new Error('cdp_command_failed'))
            return
          }
          resolveResult(message.result)
        }
        socket.addEventListener('message', listener)
        socket.send(JSON.stringify({ id, ...command }))
        onCommandSent(command)
      })
      results.push(result)
    }
    return results
  } finally {
    socket.close()
  }
}

function taskOwnedResidualProcessCount() {
  if (!exactTaskRoot || (latencyObservation && !lastTaskMainIdentity)) return null
  let candidates = ''
  try {
    candidates = boundedProcessProbe('pgrep', ['-f', exactTaskRoot])
  } catch (error) {
    if (processProbeFailureKind(error, 'pgrep') !== 'no_match') return null
  }
  const found = new Set()
  for (const value of candidates.split(/\r?\n/u)) {
    const pid = Number(value.trim())
    if (!Number.isInteger(pid) || pid <= 0 || pid === process.pid) continue
    try {
      const command = boundedProcessProbe('ps', ['-p', String(pid), '-o', 'command='])
      if (command.includes(exactTaskRoot)) found.add(pid)
    } catch (error) {
      if (!processDefinitelyExited(pid)) return null
    }
  }
  if (latencyObservation) {
    let processes = ''
    try {
      processes = boundedProcessProbe('ps', ['-axo', 'pid=,pgid=,command='])
    } catch { return null }
    const group = exactTaskGroupMembers(processes, lastTaskMainIdentity.pid,
      resolve(packagedAppPath, '../..'))
    if (!group) return null
    for (const pid of group) found.add(pid)
  }
  return found.size
}

async function evaluate(debugPort, expression, timeoutMs = 20_000) {
  const target = await waitForTarget(debugPort, timeoutMs)
  const [result] = await dispatchCdp(target.webSocketDebuggerUrl, [{
    method: 'Runtime.evaluate',
    params: { expression, awaitPromise: true, returnByValue: true, timeout: timeoutMs }
  }], timeoutMs)
  return result?.result?.value
}

async function enableRendererThreadTrace(debugPort) {
  // Main inherits the opt-in environment variable; a packaged renderer may not.
  // Write once in this task's isolated profile, then inspect state if CDP did
  // not acknowledge the write. Never replay an ambiguous mutating expression.
  try {
    if (await evaluate(debugPort, `(() => {
      try {
        localStorage.setItem('ANALYTIX_THREAD_TRACE', '1');
        return localStorage.getItem('ANALYTIX_THREAD_TRACE') === '1';
      } catch { return false; }
    })()`)) return
  } catch {
    // A lost CDP response does not tell us whether the write took effect.
  }
  const observed = await evaluate(debugPort, `(() => {
    try { return localStorage.getItem('ANALYTIX_THREAD_TRACE') === '1'; }
    catch { return false; }
  })()`).catch(() => false)
  if (observed !== true) throw new Error('renderer_thread_trace_enable_unverified')
}

function geometryExpression(kind) {
  const selector = kind === 'credential'
    ? 'input[type="password"]'
    : kind === 'composer'
      ? '.ProseMirror[contenteditable="true"], [contenteditable="true"][role="textbox"]'
      : '.ds-composer-primary-action-button'
  return `(() => {
    const elements = Array.from(document.querySelectorAll(${JSON.stringify(selector)}));
    const element = elements.find((candidate) => {
      const rect = candidate.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0 && !candidate.disabled;
    });
    if (!element) return { found: false };
    const rect = element.getBoundingClientRect();
    return { found: true, x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
  })()`
}

function buttonGeometryExpression(labels) {
  return `(() => {
    const labels = ${JSON.stringify(labels)};
    const element = Array.from(document.querySelectorAll('button')).find((candidate) => {
      const text = (candidate.textContent || '').replace(/\\s+/g, ' ').trim();
      const rect = candidate.getBoundingClientRect();
      return labels.includes(text) && rect.width > 0 && rect.height > 0 && !candidate.disabled;
    });
    if (!element) return { found: false };
    const rect = element.getBoundingClientRect();
    return { found: true, x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
  })()`
}

function credentialSurfaceExpression() {
  return `(async () => {
    const visible = (element) => {
      const rect = element.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0 && !element.disabled;
    };
    const credentials = Array.from(document.querySelectorAll('input[type="password"]')).filter(visible);
    const credential = credentials[0];
    const buttons = Array.from(document.querySelectorAll('button')).filter(visible);
    const normalized = (element) => (element.textContent || '').replace(/\\s+/g, ' ').trim();
    const onboardingSave = buttons.find((button) => ['Save and continue', '保存并继续'].includes(normalized(button)));
    const recoverySave = buttons.find((button) => ['Save changes', '保存更改'].includes(normalized(button)));
    const save = onboardingSave || recoverySave;
    const settingsBack = buttons.some((button) => ['Back', '返回'].includes(normalized(button)));
    let registryListOk = false;
    let registryFailureKind = 'none';
    let registryProviderCount = 0;
    let registrySelected = false;
    let registryCredentialConfigured = false;
    try {
      const result = await window.analytix.providerRegistry.request({ schemaVersion: 1, operation: 'list' });
      if (result && !result.error && Array.isArray(result.providers)) {
        registryListOk = true;
        registryProviderCount = result.providers.length;
        const selected = result.providers.find((provider) => provider.id === result.selectedProviderId);
        registrySelected = Boolean(selected && !selected.tombstone);
        registryCredentialConfigured = selected?.credentialConfigured === true;
      } else if (result?.error?.code) {
        const allowed = new Set([
          'invalid_request', 'method_not_allowed', 'not_found', 'conflict', 'persistence_failure',
          'verification_failure', 'request_too_large', 'unauthorized', 'runtime_unavailable', 'invalid_response',
          'credential_unavailable', 'credential_reentry_required'
        ]);
        registryFailureKind = allowed.has(result.error.code) ? result.error.code : 'other';
      }
    } catch { registryFailureKind = 'invoke_rejected'; }
    const facts = {
      uiSchemaVersion: 1,
      ready: Boolean(credential && save),
      credentialValuePresent: Boolean(credential?.value),
      visiblePasswordCount: credentials.length,
      onboardingTitlePresent: Boolean(document.getElementById('initial-setup-title')),
      onboardingSavePresent: Boolean(onboardingSave),
      recoverySavePresent: Boolean(recoverySave),
      settingsBackPresent: settingsBack,
      hubLoginPasswordPresent: Boolean(document.getElementById('password')),
      registryListOk,
      registryFailureKind,
      registryProviderCount,
      registrySelected,
      registryCredentialConfigured
    };
    if (!credential || !save) return facts;
    const credentialRect = credential.getBoundingClientRect();
    const saveRect = save.getBoundingClientRect();
    return {
      ...facts,
      ready: true,
      mode: onboardingSave ? 'onboarding' : 'recovery',
      credential: { found: true, x: credentialRect.left + credentialRect.width / 2, y: credentialRect.top + credentialRect.height / 2 },
      save: { found: true, x: saveRect.left + saveRect.width / 2, y: saveRect.top + saveRect.height / 2 }
    };
  })()`
}

function onboardingSaveOutcomeExpression() {
  return `(async () => {
    const dialog = document.getElementById('initial-setup-title')?.closest('[role="dialog"]');
    const buttons = dialog ? Array.from(dialog.querySelectorAll('button')) : [];
    const normalized = (element) => (element.textContent || '').replace(/\\s+/g, ' ').trim();
    const saving = buttons.some((button) =>
      ['Saving configuration…', '正在保存配置…'].includes(normalized(button)));
    const errorVisible = Boolean(dialog && Array.from(dialog.querySelectorAll('div')).some((element) => {
      const rect = element.getBoundingClientRect();
      return typeof element.className === 'string' && element.className.includes('border-red-500') &&
        rect.width > 0 && rect.height > 0;
    }));
    let providerCount = 0;
    let selected = false;
    let credentialConfigured = false;
    let registryFailureKind = 'none';
    try {
      const result = await window.analytix.providerRegistry.request({ schemaVersion: 1, operation: 'list' });
      if (result && !result.error && Array.isArray(result.providers)) {
        providerCount = result.providers.length;
        const chosen = result.providers.find((provider) => provider.id === result.selectedProviderId);
        selected = Boolean(chosen && !chosen.tombstone);
        credentialConfigured = chosen?.credentialConfigured === true;
      } else if (result?.error?.code) {
        const allowed = new Set([
          'invalid_request', 'method_not_allowed', 'not_found', 'conflict', 'persistence_failure',
          'verification_failure', 'request_too_large', 'unauthorized', 'runtime_unavailable', 'invalid_response',
          'credential_unavailable', 'credential_reentry_required'
        ]);
        registryFailureKind = allowed.has(result.error.code) ? result.error.code : 'other';
      }
    } catch { registryFailureKind = 'invoke_rejected'; }
    return { uiSchemaVersion: 1, phase: 'after_onboarding_save', modalPresent: Boolean(dialog),
      saving, errorVisible, providerCount, selected, credentialConfigured, registryFailureKind };
  })()`
}

function providerCommitObservationExpression() {
  return `(async () => {
    try {
      const result = await window.analytix.providerRegistry.request({ schemaVersion: 1, operation: 'list' });
      if (!result || result.error || !Array.isArray(result.providers)) return { configured: false };
      const provider = result.providers.find((item) => item.id === 'deepseek' && !item.tombstone);
      const selectedReady = provider?.credentialConfigured === true && result.selectedProviderId === provider.id;
      const select = Array.from(document.querySelectorAll('button')).find((button) => {
        const text = (button.textContent || '').replace(/\s+/g, ' ').trim();
        const rect = button.getBoundingClientRect();
        return ['Select', '选择'].includes(text) && rect.width > 0 && rect.height > 0 && !button.disabled;
      });
      const rect = select?.getBoundingClientRect();
      return {
        configured: provider?.credentialConfigured === true,
        selectedReady,
        select: rect ? { found: true, x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 } : { found: false }
      };
    } catch { return { configured: false }; }
  })()`
}

async function clickGeometry(debugPort, geometry) {
  if (!geometry?.found) throw new Error('ui_geometry_missing')
  const target = await waitForTarget(debugPort)
  await dispatchCdp(target.webSocketDebuggerUrl, [
    { method: 'Input.dispatchMouseEvent', params: { type: 'mousePressed', x: geometry.x, y: geometry.y, button: 'left', clickCount: 1 } },
    { method: 'Input.dispatchMouseEvent', params: { type: 'mouseReleased', x: geometry.x, y: geometry.y, button: 'left', clickCount: 1 } }
  ])
}

async function normalUiInput(debugPort, geometry, text) {
  await clickGeometry(debugPort, geometry)
  const target = await waitForTarget(debugPort)
  await dispatchCdp(target.webSocketDebuggerUrl, [
    { method: 'Input.dispatchKeyEvent', params: { type: 'rawKeyDown', key: 'a', code: 'KeyA', modifiers: 4 } },
    { method: 'Input.dispatchKeyEvent', params: { type: 'keyUp', key: 'a', code: 'KeyA', modifiers: 4 } },
    { method: 'Input.dispatchKeyEvent', params: { type: 'rawKeyDown', key: 'Backspace', code: 'Backspace' } },
    { method: 'Input.dispatchKeyEvent', params: { type: 'keyUp', key: 'Backspace', code: 'Backspace' } },
    { method: 'Input.insertText', params: { text } }
  ])
}

async function waitFor(debugPort, expression, accepted, timeoutMs = WAIT_TIMEOUT_MS) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const value = await evaluate(debugPort, expression, Math.min(10_000, deadline - Date.now()))
      if (value?.uiSchemaVersion === 1) lastUiFacts = value
      if (accepted(value)) return value
    } catch {
      // A development target may reload while its bundles settle.
    }
    await sleep(300)
  }
  throw new Error('ui_wait_timeout')
}

function registryObservationExpression() {
  return `(async () => {
    try {
      const result = await window.analytix.providerRegistry.request({ schemaVersion: 1, operation: 'list' });
      if (!result || result.error || !Array.isArray(result.providers)) return { ready: false };
      const selected = result.providers.find((provider) => provider.id === result.selectedProviderId);
      const forbidden = new Set(${JSON.stringify([...forbiddenPublicKeys])});
      let forbiddenKeyCount = 0;
      const visit = (value) => {
        if (!value || typeof value !== 'object') return;
        if (Array.isArray(value)) { value.forEach(visit); return; }
        for (const [key, child] of Object.entries(value)) {
          if (forbidden.has(key)) forbiddenKeyCount += 1;
          visit(child);
        }
      };
      visit(result);
      return {
        ready: Boolean(selected && selected.credentialConfigured && !selected.tombstone),
        providerCount: result.providers.length,
        selectedDeepseek: selected?.id === 'deepseek',
        credentialConfigured: selected?.credentialConfigured === true,
        endpointLoopback: typeof selected?.endpoint === 'string' && selected.endpoint.startsWith('http://127.0.0.1:'),
        forbiddenKeyCount
      };
    } catch { return { ready: false }; }
  })()`
}

function readyUiExpression(replyText = '') {
  return `(() => {
    const modal = Boolean(document.getElementById('initial-setup-title'));
    const composer = Array.from(document.querySelectorAll('.ProseMirror[contenteditable="true"], [contenteditable="true"][role="textbox"]'))
      .some((element) => { const rect = element.getBoundingClientRect(); return rect.width > 0 && rect.height > 0; });
    const passwordVisible = Array.from(document.querySelectorAll('input[type="password"]'))
      .some((element) => { const rect = element.getBoundingClientRect(); return rect.width > 0 && rect.height > 0; });
    const replySeen = ${replyText ? `document.body.innerText.includes(${JSON.stringify(replyText)})` : 'false'};
    const root = document.getElementById('root');
    return {
      uiSchemaVersion: 1,
      readyState: ['loading', 'interactive', 'complete'].includes(document.readyState) ? document.readyState : 'unknown',
      bodyChildCount: document.body?.children?.length ?? 0,
      rootPresent: Boolean(root),
      rootChildCount: root?.children?.length ?? 0,
      bridgePresent: Boolean(window.analytix),
      dialogPresent: Boolean(document.querySelector('[role="dialog"]')),
      viteErrorOverlayPresent: Boolean(document.querySelector('vite-error-overlay')),
      modal,
      passwordVisible,
      composer,
      replySeen
    };
  })()`
}

function bridgeHealthExpression() {
  return `(async () => {
    try {
      const response = await window.analytix?.runtime?.runtimeRequest('/health', 'GET');
      if (response?.status !== 200) return { ready: false };
      return { ready: JSON.parse(response.body || '{}').service === 'analytix' };
    } catch { return { ready: false }; }
  })()`
}

function ownedGoPid(mainPid) {
  if (!Number.isInteger(mainPid) || mainPid <= 0 || !runtimeDataDirPath ||
      lastTaskMainIdentity?.pid !== mainPid) return null
  let children = ''
  try {
    children = boundedProcessProbe('pgrep', ['-P', String(mainPid)])
  } catch (error) {
    return processProbeFailureKind(error, 'pgrep') === 'no_match' ? 0 : null
  }
  for (const value of children.split(/\r?\n/u)) {
    const pid = Number(value.trim())
    if (!Number.isInteger(pid) || pid <= 0) continue
    try {
      const row = boundedProcessProbe('ps', ['-p', String(pid), '-o', 'ppid=,pgid=,command=']).trim()
      const match = row.match(/^(\d+)\s+(\d+)\s+(.+)$/u)
      const executable = `${resolve(packagedAppPath, '../..')}/Resources/runtime-go/bin/runtime-server`
      if (match && Number(match[1]) === mainPid &&
          match[3].startsWith(`${executable} `) &&
          (match[3].includes(`--data-dir ${runtimeDataDirPath} `) ||
            match[3].endsWith(`--data-dir ${runtimeDataDirPath}`))) {
        const born = boundedProcessProbe('ps', ['-p', String(pid), '-o', 'lstart=']).trim()
        if (born && boundedProcessProbe('ps', ['-p', String(pid), '-o', 'lstart=']).trim() === born) {
          ownedGoIdentity = { pid, pgid: Number(match[2]), born, command: match[3] }
          return pid
        }
      }
    } catch {
      if (!processDefinitelyExited(pid)) return null
    }
  }
  return 0
}

function sameExactGoIdentity(identity) {
  try {
    const row = boundedProcessProbe('ps', ['-p', String(identity.pid), '-o', 'pgid=,command=']).trim()
    const born = boundedProcessProbe('ps', ['-p', String(identity.pid), '-o', 'lstart=']).trim()
    const match = row.match(/^(\d+)\s+(.+)$/u)
    return Boolean(match && Number(match[1]) === identity.pgid &&
      match[2] === identity.command && born === identity.born)
  } catch { return false }
}

async function normalQuit(debugPort, launched) {
  let quitRequestOk = false
  let quitRequestAcknowledged = false
  let quitTargetObserved = false
  let fallbackUsed = false
  let fallbackSigkillSent = false
  if (latencyObservation) pinExactTaskGroupMembers(launched.child)
  try {
    const target = await waitForTarget(debugPort, 10_000)
    quitTargetObserved = true
    await dispatchCdp(target.webSocketDebuggerUrl, [
      { method: 'Input.dispatchKeyEvent', params: { type: 'rawKeyDown', key: 'F4', code: 'F4', modifiers: 1 } },
      { method: 'Input.dispatchKeyEvent', params: { type: 'keyUp', key: 'F4', code: 'F4', modifiers: 1 } }
    ], 10_000, (command) => {
      if (command.params.type === 'rawKeyDown') quitRequestOk = true
    })
    quitRequestAcknowledged = true
  } catch {
    // The app can exit and close CDP before acknowledging the key event.
    // Exact-child exit and residual checks below still determine success.
  }
  let targetClosed = await waitForTargetClosed(debugPort, 15_000)
  lastNormalQuitFacts = { quitTargetObserved, quitRequestSent: quitRequestOk,
    quitRequestAcknowledged, targetClosed,
    mainExitedBeforeFallback: launched.child.exitCode !== null || Boolean(launched.child.signalCode),
    fallbackSignalSent: false, fallbackSigkillSent: false,
    mainExitCode: launched.child.exitCode,
    mainSignaled: Boolean(launched.child.signalCode), residualProcessCount: null }
  if (!targetClosed) {
    try {
      if (latencyObservation) {
        const stopped = await stopExactPackagedTaskGroup(launched.child)
        fallbackUsed = stopped.signalSent
        fallbackSigkillSent = stopped.sigkillSent
      } else {
        terminateTaskProcessGroup(launched.child)
        fallbackUsed = true
      }
    } catch {
      taskCleanupFailure = 'exact_packaged_task_group_cleanup_failed'
      throw new Error(quitRequestOk
        ? 'normal_quit_target_stayed_open' : 'normal_quit_request_failed')
    }
    targetClosed = await waitForTargetClosed(debugPort, 10_000)
    lastNormalQuitFacts = { ...lastNormalQuitFacts, targetClosed,
      fallbackSignalSent: fallbackUsed, fallbackSigkillSent,
      mainExitCode: launched.child.exitCode,
      mainSignaled: Boolean(launched.child.signalCode) }
    if (latencyObservation) {
      throw new Error(quitRequestOk
        ? 'normal_quit_target_stayed_open' : 'normal_quit_request_failed')
    }
  }
  if (!targetClosed) throw new Error('electron_task_process_group_did_not_close')
  let mainExited = launched.child.exitCode !== null || Boolean(launched.child.signalCode)
  for (let attempt = 0; attempt < 40 && !mainExited; attempt += 1) {
    await sleep(250)
    mainExited = launched.child.exitCode !== null || Boolean(launched.child.signalCode)
  }
  const mainExitedBeforeFallback = mainExited
  if (!mainExited) {
    try {
      if (latencyObservation) {
        const stopped = await stopExactPackagedTaskGroup(launched.child)
        fallbackUsed ||= stopped.signalSent
        fallbackSigkillSent ||= stopped.sigkillSent
      } else {
        terminateTaskProcessGroup(launched.child)
        fallbackUsed = true
      }
    } catch {
      taskCleanupFailure = 'exact_packaged_task_group_cleanup_failed'
      throw new Error('exact_main_process_did_not_exit')
    }
    for (let attempt = 0; attempt < 40 && !mainExited; attempt += 1) {
      await sleep(250)
      mainExited = launched.child.exitCode !== null || Boolean(launched.child.signalCode)
    }
  }
  const residualProcessCount = latencyObservation ? taskOwnedResidualProcessCount() : 0
  lastNormalQuitFacts = { quitTargetObserved, quitRequestSent: quitRequestOk, quitRequestAcknowledged,
    targetClosed, mainExitedBeforeFallback, fallbackSignalSent: fallbackUsed,
    fallbackSigkillSent, mainExitCode: launched.child.exitCode,
    mainSignaled: Boolean(launched.child.signalCode), residualProcessCount }
  if (latencyObservation) {
    const failure = normalQuitFailureCode({ quitRequestOk, targetClosed, mainExited,
      mainExitCode: launched.child.exitCode, mainSignal: launched.child.signalCode,
      fallbackUsed, residualProcessCount })
    if (failure) throw new Error(failure)
  } else if (!mainExited) {
    throw new Error('exact_main_process_did_not_exit')
  }
  if (activeChild === launched.child) activeChild = null
  return { ...launched.observation(), residualProcessCount, normalQuit: lastNormalQuitFacts }
}

function hubZeroEvidence(observations) {
  const ready = observations.filter((item) => item.stage === 'ordinary_startup_ready')
  const changed = observations.filter((item) => item.stage === 'activity_changed')
  const activity = Object.fromEntries([
    'moduleLoad', 'serviceInstance', 'refreshTimer', 'tokenRead', 'request', 'fallback'
  ].map((key) => [key, observations.reduce((maximum, item) => Math.max(maximum, item[key]), 0)]))
  const zero = ready.length === 1 && changed.length === 0 && ready.every((item) =>
    item.moduleLoad === 0 && item.serviceInstance === 0 && item.refreshTimer === 0 &&
    item.tokenRead === 0 && item.request === 0 && item.fallback === 0 && item.anyActivity === false)
  return { readyCount: ready.length, activityChangedCount: changed.length, ...activity, allZero: zero }
}

async function runLaunch({ userDataDir, unlock, enterCredential, sendPrompt }) {
  stage = unlock ? 'keychain_unlock' : 'launch_prepare'
  if (unlock) await keychain.unlockForLaunch()
  const debugPort = await allocatePort()
  const denyProxyPort = denyProxy.address().port
  stage = 'electron_launch'
  const launched = startChild({ debugPort, userDataDir, denyProxyPort })
  ownedGoIdentity = null
  ownedGroupMemberIdentities = new Map()
  const timing = { goSpawnObservedAtMs: 0, bridgeHealth200AtMs: 0,
    credentialSurfaceReadyAtMs: 0, credentialSaveClickedAtMs: 0,
    credentialSaveOutcomeAtMs: 0, registryReadyAtMs: 0,
    composerReadyAtMs: 0, submitAtMs: 0, domReplyObservedAtMs: 0,
    rendererTraceEnabled: false, terminalDomTraceObservedBeforeQuit: false }
  const goProbe = latencyObservation ? setInterval(() => {
    const pid = ownedGoPid(launched.child.pid)
    if (pid === null && lastTaskMainIdentity?.pid === launched.child.pid) {
      goProbeUnknownCount += 1
    }
    if (!timing.goSpawnObservedAtMs && pid) {
      timing.goSpawnObservedAtMs = Date.now()
    }
  }, 250) : null
  goProbe?.unref?.()
  let launchError = null
  try {
    if (latencyObservation) {
      stage = 'main_birth_pin'
      await pinExactPackagedMainBirth(launched.child)
    }
    const startup = await Promise.race([
      waitForTarget(debugPort, STARTUP_TARGET_TIMEOUT_MS).then(() => 'target'),
      launched.closed.then(() => 'exit')
    ])
    if (startup !== 'target') throw new Error('electron_exited_before_cdp')
    if (latencyObservation) {
      stage = 'bridge_health_ready'
      await waitFor(
        debugPort, bridgeHealthExpression(), (value) => value?.ready === true,
        STARTUP_TARGET_TIMEOUT_MS
      )
      timing.bridgeHealth200AtMs = Date.now()
      stage = 'renderer_thread_trace_enable'
      await enableRendererThreadTrace(debugPort)
      timing.rendererTraceEnabled = true
    }
    if (enterCredential) {
      stage = 'credential_surface_ready'
      const credentialSurface = await waitFor(
        debugPort,
        credentialSurfaceExpression(),
        (value) => value?.ready === true && ['onboarding', 'recovery'].includes(value?.mode)
      )
      if (latencyObservation) timing.credentialSurfaceReadyAtMs = Date.now()
      stage = 'credential_normal_ui_input'
      await normalUiInput(debugPort, credentialSurface.credential, credential)
      const readyToSave = await waitFor(
        debugPort,
        credentialSurfaceExpression(),
        (value) => value?.ready === true && value?.credentialValuePresent === true &&
          value?.save?.found === true
      )
      stage = 'credential_normal_ui_commit'
      await clickGeometry(debugPort, readyToSave.save)
      if (latencyObservation) timing.credentialSaveClickedAtMs = Date.now()
      if (credentialSurface.mode === 'onboarding') {
        stage = 'credential_onboarding_save_outcome'
        const outcome = await waitFor(
          debugPort,
          onboardingSaveOutcomeExpression(),
          (value) => value?.providerCount > 0 || value?.errorVisible === true ||
            value?.modalPresent === false,
          30_000
        )
        if (outcome.errorVisible) throw new Error('credential_onboarding_save_error_visible')
        if (latencyObservation) timing.credentialSaveOutcomeAtMs = Date.now()
      }
      if (credentialSurface.mode === 'recovery') {
        stage = 'credential_registry_commit_ready'
        const committed = await waitFor(
          debugPort,
          providerCommitObservationExpression(),
          (value) => value?.configured === true
        )
        if (!committed.selectedReady) {
          const selectable = committed.select?.found
            ? committed
            : await waitFor(
                debugPort,
                providerCommitObservationExpression(),
                (value) => value?.configured === true && value?.select?.found === true
              )
          stage = 'provider_normal_ui_select'
          await clickGeometry(debugPort, selectable.select)
        }
        stage = 'provider_registry_selected'
        await waitFor(debugPort, registryObservationExpression(), (value) => value?.ready === true)
        const backGeometry = await waitFor(
          debugPort,
          buttonGeometryExpression(['Back', '返回']),
          (value) => value?.found === true
        )
        stage = 'settings_normal_ui_back'
        await clickGeometry(debugPort, backGeometry)
      }
    }
    stage = 'registry_ready'
    const registry = await waitFor(debugPort, registryObservationExpression(), (value) => value?.ready === true)
    if (latencyObservation) timing.registryReadyAtMs = Date.now()
    stage = 'composer_ready'
    await waitFor(debugPort, readyUiExpression(), (value) => value?.modal === false && value?.composer === true)
    if (latencyObservation) timing.composerReadyAtMs = Date.now()
    if (sendPrompt) {
      const composerGeometry = await waitFor(
        debugPort,
        geometryExpression('composer'),
        (value) => value?.found === true
      )
      stage = 'ordinary_prompt_normal_ui_input'
      await normalUiInput(debugPort, composerGeometry, prompt)
      const sendGeometry = await waitFor(
        debugPort,
        geometryExpression('send'),
        (value) => value?.found === true
      )
      stage = 'ordinary_prompt_normal_ui_commit'
      if (latencyObservation) {
        timing.submitAtMs = Date.now()
        submittedProviderRequestArmed = true
      }
      await clickGeometry(debugPort, sendGeometry)
      stage = 'ordinary_provider_reply'
      await waitFor(debugPort, readyUiExpression(reply), (value) => value?.replySeen === true)
      submittedProviderRequestArmed = false
      if (latencyObservation) timing.domReplyObservedAtMs = Date.now()
      replyObservedInRenderer = true
      if (latencyObservation) {
        stage = 'terminal_dom_trace_observation'
        timing.terminalDomTraceObservedBeforeQuit = await waitForTerminalDomTrace(userDataDir)
      }
    }
    stage = 'normal_quit'
    const processObservation = await normalQuit(debugPort, launched)
    return { registry, processObservation, timing }
  } catch (error) {
    launchError = error
    throw error
  } finally {
    submittedProviderRequestArmed = false
    if (goProbe) clearInterval(goProbe)
    if (activeChild === launched.child) {
      try {
        if (latencyObservation) await stopExactPackagedTaskGroup(launched.child)
        else terminateTaskProcessGroup(launched.child)
        await waitForTargetClosed(debugPort, 10_000)
        if (latencyObservation) await stopExactTaskOwnedResiduals()
      } catch (error) {
        taskCleanupFailure = 'exact_packaged_task_group_cleanup_failed'
        if (!launchError) throw error
      } finally {
        activeChild = null
      }
    }
    if (latencyObservation) {
      try {
        partialLatencyEvidence = packagedLatencyEvidence(userDataDir, {
          processObservation: {
            ...launched.observation(), residualProcessCount: taskOwnedResidualProcessCount()
          },
          timing
        })
      } catch {
        partialLatencyEvidence = { classification: 'development_only_partial_observation',
          latencyEvidenceError: 'partial_timing_unavailable' }
      }
    }
  }
}

function boundedElapsed(start, end) {
  return Number.isFinite(start) && Number.isFinite(end) && start > 0 && end >= start &&
    end - start <= 30 * 60 * 1000 ? Math.round((end - start) * 1000) / 1000 : null
}

function stageElapsed(start, end, stages) {
  if (!Object.hasOwn(stages, start) || !Object.hasOwn(stages, end)) return null
  const first = stages[start]
  const last = stages[end]
  return Number.isSafeInteger(first) && Number.isSafeInteger(last) &&
    first >= 0 && last >= first && last - first <= 30 * 60 * 1000
    ? last - first : null
}

function taskTraceTiming(userDataDir) {
  const dir = join(userDataDir, 'traces')
  const timing = { coreCommittedAtMs: 0, mainReceivedAtMs: 0,
    ipcSentAtMs: 0, rendererCommittedAtMs: 0, domCommittedAtMs: 0 }
  if (!existsSync(dir)) return timing
  for (const entry of readdirSync(dir)) {
    if (!/^thread-ref-[a-f0-9]{64}\.jsonl$/u.test(entry)) continue
    const path = join(dir, entry)
    const info = lstatSync(path)
    if (!info.isFile() || info.isSymbolicLink() || info.size > 1024 * 1024) continue
    for (const line of readFileSync(path, 'utf8').split('\n')) {
      if (!line || line.length > 8192) continue
      let event
      try { event = JSON.parse(line) } catch { continue }
      const data = event?.data
      if (!data || typeof data !== 'object' || Array.isArray(data)) continue
      const absolute = (origin, elapsed) => Number.isFinite(origin) &&
        Number.isFinite(elapsed) && origin > 0 && elapsed >= 0
        ? origin + elapsed : 0
      if (event.name === 'thread.terminal.core_committed' && !timing.coreCommittedAtMs) {
        timing.coreCommittedAtMs = absolute(data.timeOrigin, data.publicationCommittedMs)
        timing.mainReceivedAtMs = absolute(data.mainTimeOrigin, data.mainReceivedMonotonicMs)
      } else if (event.name === 'thread.terminal.ipc_sent' && !timing.ipcSentAtMs) {
        timing.ipcSentAtMs = absolute(data.timeOrigin, data.monotonicMs)
      } else if (event.name === 'thread.terminal.renderer_committed' && !timing.rendererCommittedAtMs) {
        timing.rendererCommittedAtMs = absolute(data.timeOrigin, data.monotonicMs)
      } else if (event.name === 'thread.terminal.dom_committed' && !timing.domCommittedAtMs) {
        timing.domCommittedAtMs = absolute(data.timeOrigin, data.monotonicMs)
      }
    }
  }
  return timing
}

async function waitForTerminalDomTrace(userDataDir) {
  const deadline = Date.now() + 6_000
  while (Date.now() < deadline) {
    try {
      if (taskTraceTiming(userDataDir).domCommittedAtMs > 0) return true
    } catch {
      // The diagnostic writer may rotate a trace file during this read.
    }
    await sleep(250)
  }
  try { return taskTraceTiming(userDataDir).domCommittedAtMs > 0 } catch { return false }
}

function packagedLatencyEvidence(userDataDir, launch) {
  const trace = launch.processObservation.startupTrace
  const received = launch.processObservation.checkpointReceivedAtMs || {}
  const at = launch.timing
  const spawnAt = launch.processObservation.startedAtMs
  const stages = trace?.checkpointElapsedMs || {}
  const terminal = taskTraceTiming(userDataDir)
  return {
    classification: 'development_only_single_run_observation',
    normalQuit: launch.processObservation.normalQuit ?? null,
    rendererTraceEnabled: at.rendererTraceEnabled,
    terminalDomTraceObservedBeforeQuit: at.terminalDomTraceObservedBeforeQuit,
    processProbe: { ...processProbe },
    goProbeUnknownCount,
    mainEventLoopLagMaxMs: launch.processObservation.startupNumericDiagnostics?.maxEventLoopLagMs ?? null,
    maxConcurrentWaitForHealthProbes: launch.processObservation.startupNumericDiagnostics?.maxConcurrentWaitForHealthProbes ?? null,
    goStartupAttemptCount: launch.processObservation.goStartupAttemptCount ?? 0,
    runtimeTransitions: (launch.processObservation.runtimeTransitions ?? []).map((item) => ({ ...item })),
    goStartupAttempts: (launch.processObservation.goStartupAttempts ?? []).map((attempt, index) => ({
      ordinal: index + 1,
      ...attempt,
      preflightToCapabilityMaterializationDoneMs: boundedElapsed(
        attempt.preflightBegin, attempt.capabilityMaterializationDone),
      processSpawnedToReadyLineReceivedMs: boundedElapsed(
        attempt.processSpawned, attempt.readyLineReceived)
    })),
    goPhaseAttribution: launch.processObservation.goStartupAttemptCount > 1
      ? 'multiple_go_attempts_bounded_phase_series' : 'single_or_unobserved_attempt',
    taskOwnedResidualProcessCount: launch.processObservation.residualProcessCount,
    traceCheckpointCount: trace?.checkpointCount || 0,
    launchToMainFirstLogReceivedMs: boundedElapsed(spawnAt, received.main_module_evaluated),
    mainModuleToWindowStartupSurfaceMs: stageElapsed(
      'main_module_evaluated', 'window_startup_surface_ready', stages),
    launchToGoSpawnObservedMs: boundedElapsed(spawnAt, at.goSpawnObservedAtMs),
    goSpawnObservedToMainBridgeHealth200Ms: boundedElapsed(
      at.goSpawnObservedAtMs, at.bridgeHealth200AtMs),
    launchToMainBridgeHealth200Ms: boundedElapsed(spawnAt, at.bridgeHealth200AtMs),
    bridgeHealthToCredentialSurfaceReadyMs: boundedElapsed(
      at.bridgeHealth200AtMs, at.credentialSurfaceReadyAtMs),
    credentialSurfaceToSaveClickMs: boundedElapsed(
      at.credentialSurfaceReadyAtMs, at.credentialSaveClickedAtMs),
    saveClickToSaveOutcomeMs: boundedElapsed(
      at.credentialSaveClickedAtMs, at.credentialSaveOutcomeAtMs),
    saveOutcomeToRegistryReadyMs: boundedElapsed(
      at.credentialSaveOutcomeAtMs, at.registryReadyAtMs),
    registryReadyToComposerReadyMs: boundedElapsed(
      at.registryReadyAtMs, at.composerReadyAtMs),
    bridgeHealthToComposerReadyMs: boundedElapsed(
      at.bridgeHealth200AtMs, at.composerReadyAtMs),
    launchToComposerReadyMs: boundedElapsed(spawnAt, at.composerReadyAtMs),
    submitToFirstLoopbackProviderBodyReceivedMs: boundedElapsed(
      at.submitAtMs, firstSubmittedProviderRequestAtMs),
    submitToCoreCommittedMs: boundedElapsed(at.submitAtMs, terminal.coreCommittedAtMs),
    coreCommittedToMainReceivedMs: boundedElapsed(terminal.coreCommittedAtMs, terminal.mainReceivedAtMs),
    mainReceivedToIpcSentMs: boundedElapsed(terminal.mainReceivedAtMs, terminal.ipcSentAtMs),
    ipcSentToRendererCommittedMs: boundedElapsed(terminal.ipcSentAtMs, terminal.rendererCommittedAtMs),
    rendererCommittedToDomCommittedMs: boundedElapsed(terminal.rendererCommittedAtMs, terminal.domCommittedAtMs),
    ipcSentToDomCommittedMs: boundedElapsed(terminal.ipcSentAtMs, terminal.domCommittedAtMs),
    submitToDomCommittedMs: boundedElapsed(at.submitAtMs, terminal.domCommittedAtMs),
    submitToVisibleReplyObservedMs: boundedElapsed(at.submitAtMs, at.domReplyObservedAtMs),
    terminalTrace: {
      coreCommitted: terminal.coreCommittedAtMs > 0,
      mainReceived: terminal.mainReceivedAtMs > 0,
      ipcSent: terminal.ipcSentAtMs > 0,
      rendererCommitted: terminal.rendererCommittedAtMs > 0,
      domCommitted: terminal.domCommittedAtMs > 0
    },
    limits: [
      'launch_to_main_includes_imports_and_top_level_initialization',
      'go_spawn_is_polled_and_main_bridge_health_bounds_verified_runtime_ready',
      'ui_phase_timings_include_cdp_and_process_probe_observer_overhead',
      'window_startup_surface_is_not_agent_host_ready',
      'dom_commit_is_not_compositor_presentation'
    ]
  }
}

function settingsPrivacyFacts(value) {
  const sensitiveKeyCounts = Object.fromEntries([...settingsSensitiveKeys].sort().map((key) => [key, 0]))
  let structuralPathKeyCount = 0
  let unexpectedStructuralPathValueCount = 0
  let structuralSecretKeyCount = 0
  let nonEmptyStructuralSecretValueCount = 0
  let taskOwnedFilesystemPathCount = 0
  let inertEmptyFilesystemPathCount = 0
  let outsideTaskOwnedFilesystemPathCount = 0
  const recordFilesystemPath = (path) => {
    taskOwnedFilesystemPathCount += 1
    if (
      typeof path !== 'string' || !isAbsolute(path) || resolve(path) !== path ||
      (path !== exactTaskRoot && !strictDescendant(exactTaskRoot, path))
    ) {
      outsideTaskOwnedFilesystemPathCount += 1
    }
  }
  const visit = (current, parents = []) => {
    if (!current || typeof current !== 'object') return
    if (Array.isArray(current)) {
      current.forEach((child, index) => visit(child, [...parents, String(index)]))
      return
    }
    for (const [key, child] of Object.entries(current)) {
      const fieldPath = [...parents, key].join('.')
      if (Object.hasOwn(sensitiveKeyCounts, key)) sensitiveKeyCounts[key] += 1
      if (key === 'path') {
        structuralPathKeyCount += 1
        if (child !== '/claw/im') unexpectedStructuralPathValueCount += 1
      }
      if (key === 'secret') {
        structuralSecretKeyCount += 1
        if (typeof child !== 'string' || child.length > 0) nonEmptyStructuralSecretValueCount += 1
      }
      if (settingsFilesystemPathKeys.has(key)) {
        if (
          child === '' &&
          (fieldPath === 'claw.im.workspaceRoot' || fieldPath === 'schedule.defaultWorkspaceRoot')
        ) {
          inertEmptyFilesystemPathCount += 1
        } else {
          recordFilesystemPath(child)
        }
      }
      if (key === 'workspaces') {
        if (!Array.isArray(child)) {
          outsideTaskOwnedFilesystemPathCount += 1
        } else {
          for (const path of child) recordFilesystemPath(path)
        }
      }
      visit(child, [...parents, key])
    }
  }
  visit(value)
  return {
    sensitiveKeyCounts,
    sensitiveKeyCount: Object.values(sensitiveKeyCounts).reduce((sum, count) => sum + count, 0),
    structuralPathKeyCount,
    unexpectedStructuralPathValueCount,
    structuralSecretKeyCount,
    nonEmptyStructuralSecretValueCount,
    taskOwnedFilesystemPathCount,
    inertEmptyFilesystemPathCount,
    outsideTaskOwnedFilesystemPathCount
  }
}

function scanTaskFiles(path, rawNeedle, base64Needle) {
  let scannedFileCount = 0
  let secretHitCount = 0
  let base64HitCount = 0
  const visit = (current) => {
    const info = lstatSync(current)
    if (info.isSymbolicLink()) return
    if (info.isDirectory()) {
      for (const entry of readdirSync(current)) visit(join(current, entry))
      return
    }
    if (!info.isFile() || info.size > MAX_SCAN_FILE_BYTES || current === keychainDatabasePath) return
    scannedFileCount += 1
    const bytes = readFileSync(current)
    if (bytes.includes(rawNeedle)) secretHitCount += 1
    if (bytes.includes(base64Needle)) base64HitCount += 1
    bytes.fill(0)
  }
  visit(path)
  return { scannedFileCount, secretHitCount, base64HitCount }
}

function runtimeFailureMaterializationFacts() {
  if (!runtimeDataDirPath || !existsSync(runtimeDataDirPath)) {
    return {
      runtimeDataDirectoryPresent: false,
      runtimeFileCount: 0,
      secretAuthoritySelectionPresent: false,
      explicitTaskBindingPresent: false,
      credentialStorePresent: false,
      providerRegistryPresent: false
    }
  }
  let runtimeFileCount = 0
  const countFiles = (path) => {
    const info = lstatSync(path)
    if (info.isSymbolicLink()) return
    if (info.isDirectory()) {
      for (const entry of readdirSync(path)) countFiles(join(path, entry))
      return
    }
    if (info.isFile()) runtimeFileCount += 1
  }
  countFiles(runtimeDataDirPath)
  const privateRoot = join(runtimeDataDirPath, 'private')
  const secretsRoot = join(privateRoot, 'provider-secrets')
  const masterKeyRoot = join(secretsRoot, 'master-key')
  return {
    runtimeDataDirectoryPresent: true,
    runtimeFileCount,
    secretAuthoritySelectionPresent: existsSync(join(masterKeyRoot, 'authority.v1')),
    explicitTaskBindingPresent: existsSync(join(masterKeyRoot, 'explicit-task-keychain-binding.v1')),
    credentialStorePresent: existsSync(join(secretsRoot, 'credentials.v1.json')),
    providerRegistryPresent: existsSync(join(privateRoot, 'provider-registry', 'registry.v1.json'))
  }
}

async function main() {
  if (process.platform !== 'darwin' || typeof process.getuid !== 'function') throw new Error('darwin_required')
  if (latencyObservation && lane !== 'packaged') throw new Error('packaged_latency_observation_required')
  if (lane === 'packaged' && (!packagedAppPath || !isAbsolute(packagedAppPath) ||
      !existsSync(packagedAppPath) || !lstatSync(packagedAppPath).isFile())) {
    throw new Error('packaged_app_invalid')
  }
  const cacheRoot = process.env.ANALYTIX_DEV_CACHE_ROOT ?? ''
  const cacheTmp = join(cacheRoot, 'tmp')
  if (!SAFE_PATH.test(cacheRoot) || resolve(cacheRoot) !== cacheRoot ||
    !exactOwnerOnlyDirectory(cacheRoot) || !exactOwnerOnlyDirectory(cacheTmp)) {
    throw new Error('cache_boundary_invalid')
  }
  exactTaskRoot = mkdtempSync(join(cacheTmp, lane === 'development' ? 'agk10c-' : 'agk10d-'))
  chmodSync(exactTaskRoot, 0o700)
  if (!exactOwnerOnlyDirectory(exactTaskRoot) || !strictDescendant(cacheTmp, exactTaskRoot)) {
    throw new Error('task_root_invalid')
  }
  const userDataDir = join(exactTaskRoot, 'profile')
  const runtimeDataDir = join(exactTaskRoot, 'runtime-data')
  runtimeDataDirPath = runtimeDataDir
  mkdirSync(userDataDir, { mode: 0o700 })
  chmodSync(userDataDir, 0o700)
  mkdirSync(runtimeDataDir, { mode: 0o700 })
  chmodSync(runtimeDataDir, 0o700)
  if (!exactOwnerOnlyDirectory(userDataDir) || !exactOwnerOnlyDirectory(runtimeDataDir)) {
    throw new Error('profile_or_runtime_data_invalid')
  }

  credential = `sk-k10-${lane}-${randomBytes(24).toString('hex')}`
  prompt = ordinaryFileProbe
    ? '请实际读取工作区相对路径「2026年资料/表单 42.txt」的内容，只列出文件中的日期、数量、金额和参考编号；不要猜测。标记 N04-QA-107d-ordinary。'
    : 'Please say hello in one short sentence.'
  reply = ordinaryFileProbe
    ? '日期：2026-07-01；数量：42；金额：1234.56 元；参考编号：REF-17。'
    : 'Hello from the local test provider.'
  if (ordinaryFileProbe) {
    const workspace = join(exactTaskRoot, 'workspace')
    const documentDirectory = join(workspace, '2026年资料')
    mkdirSync(documentDirectory, { recursive: true, mode: 0o700 })
    chmodSync(workspace, 0o700)
    chmodSync(documentDirectory, 0o700)
    writeFileSync(join(documentDirectory, '表单 42.txt'),
      '日期：2026-07-01\n数量：42\n金额：1234.56 元\n参考编号：REF-17\n',
      { mode: 0o600, flag: 'wx' })
  }
  providerServer = createProviderServer()
  denyProxy = createDenyProxy()
  const providerPort = await listen(providerServer)
  await listen(denyProxy)
  providerBaseUrl = `http://127.0.0.1:${providerPort}/v1`
  seedKeyFreeSettings(userDataDir, runtimeDataDir)

  stage = 'task_keychain_create'
  keychain = await createK10DarwinExplicitTaskKeychain({ isolationRoot: exactTaskRoot, cacheRoot })
  keychainDatabasePath = keychain.paths.databasePath

  stage = 'first_launch'
  const first = await runLaunch({
    userDataDir,
    unlock: true,
    enterCredential: true,
    sendPrompt: lane === 'packaged'
  })
  launchEvidence.push({
    registry: first.registry,
    hub: hubZeroEvidence(first.processObservation.observations),
    stdoutBytes: first.processObservation.stdoutBytes,
    stderrBytes: first.processObservation.stderrBytes,
    ...((latencyObservation || traceTransitions) ? {
      goStartupAttemptCount: first.processObservation.goStartupAttemptCount,
      goStartupAttempts: first.processObservation.goStartupAttempts,
      runtimeTransitions: first.processObservation.runtimeTransitions
    } : {})
  })
  const latency = latencyObservation
    ? { ...packagedLatencyEvidence(userDataDir, first), normalQuitObserved: true }
    : null

  if (lane === 'development') {
    stage = 'restart_launch'
    const second = await runLaunch({ userDataDir, unlock: true, enterCredential: false, sendPrompt: true })
    launchEvidence.push({
      registry: second.registry,
      hub: hubZeroEvidence(second.processObservation.observations),
      stdoutBytes: second.processObservation.stdoutBytes,
      stderrBytes: second.processObservation.stderrBytes,
      ...((latencyObservation || traceTransitions) ? {
        goStartupAttemptCount: second.processObservation.goStartupAttemptCount,
        goStartupAttempts: second.processObservation.goStartupAttempts,
        runtimeTransitions: second.processObservation.runtimeTransitions
      } : {})
    })
  }

  stage = 'privacy_scan'
  const rawNeedle = Buffer.from(credential, 'utf8')
  const base64Needle = Buffer.from(rawNeedle.toString('base64'), 'ascii')
  const scan = scanTaskFiles(exactTaskRoot, rawNeedle, base64Needle)
  rawNeedle.fill(0)
  base64Needle.fill(0)
  const settings = JSON.parse(readFileSync(settingsPath, 'utf8'))
  const settingsPrivacy = settingsPrivacyFacts(settings)
  const keychainEvidence = keychain.evidence()
  const providerGreen = providerObservations.length >= 1 && providerObservations.length <= 3 &&
    providerObservations.every((item) => item.authorizationMatched && item.pathMatched &&
      item.streamRequested && item.modelConfigured && item.bodyBytes > 0) &&
    providerObservations.some((item) => item.promptSeen) &&
    providerProbeObservations.length >= 1 &&
    providerProbeObservations.every((item) => item.authorizationMatched) &&
    (!ordinaryFileProbe || (providerObservations.length === 2 &&
      providerObservations[0].readToolAdvertised && providerObservations[1].readResultSeen))
  const registryGreen = launchEvidence.every((item) =>
    item.registry.ready && item.registry.providerCount === 1 && item.registry.selectedDeepseek &&
    item.registry.credentialConfigured && item.registry.endpointLoopback && item.registry.forbiddenKeyCount === 0)
  const hubGreen = launchEvidence.every((item) => item.hub.allZero)
  const privacyGreen = scan.secretHitCount === 0 && scan.base64HitCount === 0 &&
    settingsPrivacy.sensitiveKeyCount === 0 && settingsPrivacy.unexpectedStructuralPathValueCount === 0 &&
    settingsPrivacy.nonEmptyStructuralSecretValueCount === 0 &&
    settingsPrivacy.outsideTaskOwnedFilesystemPathCount === 0
  const launchCountExpected = lane === 'development' ? 2 : 1
  const green = launchEvidence.length === launchCountExpected && registryGreen && hubGreen && providerGreen &&
    replyObservedInRenderer && privacyGreen && denyProxyConnectionCount === 0 && keychainEvidence.ready &&
    (!latencyObservation || processProbe.timeoutCount === 0) &&
    (!latencyObservation || first.processObservation.residualProcessCount === 0) &&
    (lane !== 'development' || keychainEvidence.reusedForTwoLaunches)

  stage = 'complete'
  output({
    schemaVersion: 1,
    lane,
    ordinaryFileProbe,
    ...(traceTransitions ? { runtimeTransitionDiagnostic: launchEvidence.map((item) => ({
      goStartupAttemptCount: item.goStartupAttemptCount,
      goStartupAttempts: item.goStartupAttempts,
      runtimeTransitions: item.runtimeTransitions
    })) } : {}),
    classification: green ? 'LOCAL_NONPUBLISHABLE_PRODUCT_SEAM_GREEN' : 'LOCAL_NONPUBLISHABLE_PRODUCT_SEAM_FAILED',
    green,
    launchCount: launchEvidence.length,
    keychain: {
      ready: keychainEvidence.ready,
      unlockCount: keychainEvidence.unlockCount,
      reusedForTwoLaunches: keychainEvidence.reusedForTwoLaunches,
      passwordInArgv: keychainEvidence.passwordInArgv,
      passwordInEnvironment: keychainEvidence.passwordInEnvironment,
      passwordInFile: keychainEvidence.passwordInFile
    },
    registry: {
      allReady: registryGreen,
      providerCounts: launchEvidence.map((item) => item.registry.providerCount),
      selectedLocalCredentialConfigured: launchEvidence.every((item) => item.registry.credentialConfigured),
      endpointLoopback: launchEvidence.every((item) => item.registry.endpointLoopback),
      forbiddenPublicKeyCount: launchEvidence.reduce((sum, item) => sum + item.registry.forbiddenKeyCount, 0)
    },
    provider: {
      ...providerObservationFacts(),
      replyObservedInRenderer
    },
    hub: {
      ordinaryStartupReadyCounts: launchEvidence.map((item) => item.hub.readyCount),
      activityChangedCounts: launchEvidence.map((item) => item.hub.activityChangedCount),
      dimensions: Object.fromEntries([
        'moduleLoad', 'serviceInstance', 'refreshTimer', 'tokenRead', 'request', 'fallback'
      ].map((key) => [key, launchEvidence.map((item) => item.hub[key])])),
      allDimensionsZero: hubGreen
    },
    privacy: {
      scannedFileCount: scan.scannedFileCount,
      secretHitCount: scan.secretHitCount,
      base64SecretHitCount: scan.base64HitCount,
      settingsForbiddenKeyCount: settingsPrivacy.sensitiveKeyCount,
      settingsForbiddenKeyCounts: settingsPrivacy.sensitiveKeyCounts,
      structuralPathKeyCount: settingsPrivacy.structuralPathKeyCount,
      unexpectedStructuralPathValueCount: settingsPrivacy.unexpectedStructuralPathValueCount,
      structuralSecretKeyCount: settingsPrivacy.structuralSecretKeyCount,
      nonEmptyStructuralSecretValueCount: settingsPrivacy.nonEmptyStructuralSecretValueCount,
      taskOwnedFilesystemPathCount: settingsPrivacy.taskOwnedFilesystemPathCount,
      inertEmptyFilesystemPathCount: settingsPrivacy.inertEmptyFilesystemPathCount,
      outsideTaskOwnedFilesystemPathCount: settingsPrivacy.outsideTaskOwnedFilesystemPathCount,
      rawProviderBodiesRetained: false,
      rawChildOutputRetained: false
    },
    network: {
      externalProviderUsed: false,
      denyProxyConnectionCount
    },
    ...(latency ? { latency } : {})
  })
  if (!green) {
    latencyFailed = true
    process.exitCode = 1
  }
}

try {
  await main()
} catch (error) {
  latencyFailed = true
  const allowedFailureCodes = new Set([
    'normal_quit_target_stayed_open', 'normal_quit_request_failed',
    'normal_quit_main_abnormal_exit', 'normal_quit_required_fallback',
    'exact_main_process_did_not_exit',
    'electron_task_process_group_did_not_close',
    'exact_packaged_task_group_untrusted', 'exact_packaged_task_group_changed',
    'exact_packaged_task_group_did_not_exit',
    'exact_main_process_unavailable', 'exact_main_debug_port_owner_mismatch',
    'exact_main_debug_target_untrusted', 'exact_main_debug_port_owner_unavailable',
    'electron_exited_before_cdp', 'cdp_command_timeout', 'cdp_command_failed',
    'process_probe_timeout', 'exact_main_birth_unavailable',
    'exact_main_exited_before_birth_pin', 'task_owned_process_residual',
    'task_owned_process_residual_unknown',
    'credential_onboarding_save_error_visible', 'ui_wait_timeout',
    'cdp_target_timeout'
  ])
  primaryFailureReasonCode = error instanceof Error && allowedFailureCodes.has(error.message)
    ? error.message : 'other'
  output({
    schemaVersion: 1,
    lane,
    ordinaryFileProbe,
    classification: 'LOCAL_NONPUBLISHABLE_PRODUCT_SEAM_FAILED',
    green: false,
    failedStage: stage,
    failureReasonCode: primaryFailureReasonCode,
    child: lastChildFailureFacts ?? activeChildFailureSnapshot?.() ?? null,
    ...(latencyObservation ? { processProbe: { ...processProbe } } : {}),
    ...(latencyObservation && lastTaskGroupIdentityFailure
      ? { taskGroupIdentityFailure: lastTaskGroupIdentityFailure } : {}),
    ui: lastUiFacts,
    ...(lastNormalQuitFacts ? { normalQuit: lastNormalQuitFacts } : {}),
    keychain: keychain ? {
      ready: keychain.evidence().ready,
      unlockCount: keychain.evidence().unlockCount
    } : null,
    provider: providerObservationFacts(),
    network: { denyProxyConnectionCount },
    runtimeMaterialization: runtimeFailureMaterializationFacts(),
    ...(partialLatencyEvidence
      ? { latency: { ...partialLatencyEvidence, normalQuitObserved: false } }
      : {}),
    rawErrorRetained: false
  })
  process.exitCode = 1
} finally {
  if (activeChild) {
    try {
      if (latencyObservation) {
        await stopExactPackagedTaskGroup(activeChild)
        await stopExactTaskOwnedResiduals()
      } else terminateTaskProcessGroup(activeChild)
    } catch {
      taskCleanupFailure = 'exact_packaged_task_group_cleanup_failed'
      latencyFailed = true
    }
    activeChild = null
  }
  const finalTaskResidualProcessCount = latencyObservation && exactTaskRoot
    ? taskOwnedResidualProcessCount() : null
  if (latencyObservation && exactTaskRoot && finalTaskResidualProcessCount !== 0) {
    taskCleanupFailure ||= finalTaskResidualProcessCount === null
      ? 'final_task_residual_unknown' : 'final_task_process_residual'
    latencyFailed = true
    process.exitCode = 1
  }
  activeChildFailureSnapshot = null
  await closeServer(providerServer)
  await closeServer(denyProxy)
  keychain?.dispose()
  credential = ''
  prompt = ''
  reply = ''
  ordinaryFileReadToolName = ''
  providerBaseUrl = ''
  runtimeDataDirPath = ''
  replyObservedInRenderer = false
  keychainDatabasePath = ''
  settingsPath = ''
  if (exactTaskRoot) {
    if (!latencyObservation || !latencyFailed) {
      try {
        rmSync(exactTaskRoot, { recursive: true, force: false })
        cleanedExactTaskRoot = !existsSync(exactTaskRoot)
      } catch {
        cleanedExactTaskRoot = false
      }
    }
  }
  output({ schemaVersion: 1, lane, cleanedExactTaskRoot,
    ...(latencyObservation ? { finalTaskResidualProcessCount } : {}),
    ...(primaryFailureReasonCode ? { primaryFailureReasonCode } : {}),
    ...(taskCleanupFailure ? { taskCleanupFailure } : {}),
    ...(lastTaskGroupIdentityFailure ? { taskGroupIdentityFailure: lastTaskGroupIdentityFailure } : {}),
    ...(latencyObservation && !cleanedExactTaskRoot ? {
      retainedDiagnosticProfile: exactTaskRoot
    } : {}) })
}
