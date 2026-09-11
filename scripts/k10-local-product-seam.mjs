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
import { spawn } from 'node:child_process'
import { isAbsolute, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createK10DarwinExplicitTaskKeychain } from './k10-darwin-explicit-keychain.mjs'

const repoRoot = resolve(fileURLToPath(new URL('..', import.meta.url)))
const lane = process.argv.includes('--packaged') ? 'packaged' : 'development'
const appPathIndex = process.argv.indexOf('--app-path')
const packagedAppPath = appPathIndex >= 0 ? process.argv[appPathIndex + 1] ?? '' : ''
const SAFE_PATH = /^\/[A-Za-z0-9._/-]+$/u
const MAX_PROVIDER_BODY_BYTES = 1 << 20
const MAX_SCAN_FILE_BYTES = 16 << 20
const MAX_CHILD_LINE_BYTES = 1 << 20
const WAIT_TIMEOUT_MS = 120_000
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
let settingsPath = ''
let keychainDatabasePath = ''
let providerBaseUrl = ''
let runtimeDataDirPath = ''
let replyObservedInRenderer = false
let lastChildFailureFacts = null
let lastUiFacts = null
let activeChildFailureSnapshot = null
const launchEvidence = []
const providerObservations = []
let denyProxyConnectionCount = 0

function output(value) {
  process.stdout.write(`${JSON.stringify(value)}\n`)
}

function sleep(ms) {
  return new Promise((resolvePromise) => setTimeout(resolvePromise, ms))
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
  if (!child?.pid) return
  try {
    process.kill(-child.pid, 'SIGTERM')
  } catch {
    child.kill('SIGTERM')
  }
}

function createProviderServer() {
  return http.createServer((request, response) => {
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
      const body = Buffer.concat(chunks)
      for (const item of chunks) item.fill(0)
      let parsed = null
      try {
        parsed = JSON.parse(body.toString('utf8'))
      } catch {
        parsed = null
      }
      const messages = Array.isArray(parsed?.messages) ? parsed.messages : []
      providerObservations.push(Object.freeze({
        authorizationMatched: request.headers.authorization === `Bearer ${credential}`,
        pathMatched: request.url === '/v1/chat/completions',
        streamRequested: parsed?.stream === true,
        modelConfigured: typeof parsed?.model === 'string' && parsed.model.length > 0,
        promptSeen: messages.some((message) =>
          message && message.role === 'user' && typeof message.content === 'string' &&
          message.content.includes(prompt)),
        bodyBytes: body.length
      }))
      body.fill(0)
      parsed = null
      response.writeHead(200, {
        'content-type': 'text/event-stream; charset=utf-8',
        'cache-control': 'no-store'
      })
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
    requestCount: providerObservations.length,
    allAuthorizationMatched: providerObservations.every((item) => item.authorizationMatched),
    allLoopbackPathMatched: providerObservations.every((item) => item.pathMatched),
    allStreaming: providerObservations.every((item) => item.streamRequested),
    allModelConfigured: providerObservations.every((item) => item.modelConfigured),
    promptObserved: providerObservations.some((item) => item.promptSeen),
    allBodiesNonEmpty: providerObservations.every((item) => item.bodyBytes > 0)
  }
}

function seedKeyFreeSettings(userDataDir, runtimeDataDir) {
  settingsPath = join(userDataDir, 'analytix-settings.json')
  const settings = {
    version: 1,
    locale: 'en',
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
  const classifyDiagnostic = (chunk) => {
    diagnosticTail = `${diagnosticTail}${chunk.toString('utf8')}`.slice(-65_536)
    const patterns = [
      ['desktop_external_state_invalid', /desktop external-state isolation configuration is invalid/iu],
      ['darwin_keychain_binding_unavailable', /Darwin Secret Store task Keychain binding is unavailable/iu],
      ['single_instance_unavailable', /another instance|single instance/iu],
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
    if (/gotSingleInstanceLock["']?\s*[:=]\s*false/iu.test(diagnosticTail)) {
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
  activeChild = child
  activeChildFailureSnapshot = () => ({
    exitObserved: false,
    exitCode: null,
    signaled: false,
    diagnosticKinds: [...diagnosticKinds].sort(),
    traceStages: [...traceStages],
    eventCounts: { ...eventCounts },
    stdoutBytes,
    stderrBytes
  })
  child.stdout.on('data', (chunk) => {
    stdoutBytes += chunk.length
    classifyDiagnostic(chunk)
    line += chunk.toString('utf8')
    if (Buffer.byteLength(line) > MAX_CHILD_LINE_BYTES) line = line.slice(-MAX_CHILD_LINE_BYTES)
    for (;;) {
      const index = line.indexOf('\n')
      if (index < 0) break
      const completedLine = line.slice(0, index)
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
    stderrBytes += chunk.length
    classifyDiagnostic(chunk)
  })
  const closed = new Promise((resolveClosed) => {
    child.once('close', (code, signal) => {
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
        stderrBytes
      }
      diagnosticTail = ''
      resolveClosed({ code, signal })
    })
  })
  return {
    child,
    closed,
    observation: () => ({ observations: [...observations], stdoutBytes, stderrBytes })
  }
}

async function waitForTarget(debugPort, timeoutMs = WAIT_TIMEOUT_MS) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
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

async function waitForTargetClosed(debugPort, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`http://127.0.0.1:${debugPort}/json/list`, {
        signal: AbortSignal.timeout(1000)
      })
      if (!response.ok) return true
      const targets = await response.json()
      if (!Array.isArray(targets) || !targets.some((target) => target?.type === 'page')) return true
    } catch {
      return true
    }
    await sleep(250)
  }
  return false
}

async function dispatchCdp(webSocketDebuggerUrl, commands, timeoutMs = 20_000) {
  const socket = new WebSocket(webSocketDebuggerUrl)
  await new Promise((resolveOpen, reject) => {
    socket.addEventListener('open', resolveOpen, { once: true })
    socket.addEventListener('error', reject, { once: true })
  })
  let nextId = 0
  try {
    const results = []
    for (const command of commands) {
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
      })
      results.push(result)
    }
    return results
  } finally {
    socket.close()
  }
}

async function evaluate(debugPort, expression, timeoutMs = 20_000) {
  const target = await waitForTarget(debugPort, timeoutMs)
  const [result] = await dispatchCdp(target.webSocketDebuggerUrl, [{
    method: 'Runtime.evaluate',
    params: { expression, awaitPromise: true, returnByValue: true, timeout: timeoutMs }
  }], timeoutMs)
  return result?.result?.value
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
    const normalized = (element) => (element.textContent || '').replace(/\s+/g, ' ').trim();
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
          'verification_failure', 'request_too_large', 'unauthorized', 'runtime_unavailable', 'invalid_response'
        ]);
        registryFailureKind = allowed.has(result.error.code) ? result.error.code : 'other';
      }
    } catch { registryFailureKind = 'invoke_rejected'; }
    const facts = {
      uiSchemaVersion: 1,
      ready: Boolean(credential && save),
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

async function normalQuit(debugPort, launched) {
  try {
    const target = await waitForTarget(debugPort, 10_000)
    await dispatchCdp(target.webSocketDebuggerUrl, [
      { method: 'Input.dispatchKeyEvent', params: { type: 'rawKeyDown', key: 'q', code: 'KeyQ', modifiers: 4 } },
      { method: 'Input.dispatchKeyEvent', params: { type: 'keyUp', key: 'q', code: 'KeyQ', modifiers: 4 } }
    ], 10_000)
  } catch {
    // Exact-child termination below is the bounded fallback.
  }
  let targetClosed = await waitForTargetClosed(debugPort, 15_000)
  if (!targetClosed) {
    terminateTaskProcessGroup(launched.child)
    targetClosed = await waitForTargetClosed(debugPort, 10_000)
  }
  if (!targetClosed) throw new Error('electron_task_process_group_did_not_close')
  if (activeChild === launched.child) activeChild = null
  return launched.observation()
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
  try {
    const startup = await Promise.race([
      waitForTarget(debugPort).then(() => 'target'),
      launched.closed.then(() => 'exit')
    ])
    if (startup !== 'target') throw new Error('electron_exited_before_cdp')
    if (enterCredential) {
      stage = 'credential_surface_ready'
      const credentialSurface = await waitFor(
        debugPort,
        credentialSurfaceExpression(),
        (value) => value?.ready === true && ['onboarding', 'recovery'].includes(value?.mode)
      )
      stage = 'credential_normal_ui_input'
      await normalUiInput(debugPort, credentialSurface.credential, credential)
      stage = 'credential_normal_ui_commit'
      await clickGeometry(debugPort, credentialSurface.save)
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
    stage = 'composer_ready'
    await waitFor(debugPort, readyUiExpression(), (value) => value?.modal === false && value?.composer === true)
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
      await clickGeometry(debugPort, sendGeometry)
      stage = 'ordinary_provider_reply'
      await waitFor(debugPort, readyUiExpression(reply), (value) => value?.replySeen === true)
      replyObservedInRenderer = true
    }
    stage = 'normal_quit'
    const processObservation = await normalQuit(debugPort, launched)
    return { registry, processObservation }
  } finally {
    if (activeChild === launched.child) {
      terminateTaskProcessGroup(launched.child)
      await waitForTargetClosed(debugPort, 10_000)
      activeChild = null
    }
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
  if (lane === 'packaged' && (!packagedAppPath || !isAbsolute(packagedAppPath) || !existsSync(packagedAppPath))) {
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
  prompt = `k10-${lane}-ordinary-${randomBytes(16).toString('hex')}`
  reply = `k10-${lane}-reply-${randomBytes(16).toString('hex')}`
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
    stderrBytes: first.processObservation.stderrBytes
  })

  if (lane === 'development') {
    stage = 'restart_launch'
    const second = await runLaunch({ userDataDir, unlock: true, enterCredential: false, sendPrompt: true })
    launchEvidence.push({
      registry: second.registry,
      hub: hubZeroEvidence(second.processObservation.observations),
      stdoutBytes: second.processObservation.stdoutBytes,
      stderrBytes: second.processObservation.stderrBytes
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
    providerObservations.some((item) => item.promptSeen)
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
    (lane !== 'development' || keychainEvidence.reusedForTwoLaunches)

  stage = 'complete'
  output({
    schemaVersion: 1,
    lane,
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
    }
  })
  if (!green) process.exitCode = 1
}

try {
  await main()
} catch {
  output({
    schemaVersion: 1,
    lane,
    classification: 'LOCAL_NONPUBLISHABLE_PRODUCT_SEAM_FAILED',
    green: false,
    failedStage: stage,
    child: lastChildFailureFacts ?? activeChildFailureSnapshot?.() ?? null,
    ui: lastUiFacts,
    provider: providerObservationFacts(),
    network: { denyProxyConnectionCount },
    runtimeMaterialization: runtimeFailureMaterializationFacts(),
    rawErrorRetained: false
  })
  process.exitCode = 1
} finally {
  if (activeChild) {
    terminateTaskProcessGroup(activeChild)
    activeChild = null
  }
  activeChildFailureSnapshot = null
  await closeServer(providerServer)
  await closeServer(denyProxy)
  keychain?.dispose()
  credential = ''
  prompt = ''
  reply = ''
  providerBaseUrl = ''
  runtimeDataDirPath = ''
  replyObservedInRenderer = false
  keychainDatabasePath = ''
  settingsPath = ''
  if (exactTaskRoot) {
    try {
      rmSync(exactTaskRoot, { recursive: true, force: false })
      cleanedExactTaskRoot = !existsSync(exactTaskRoot)
    } catch {
      cleanedExactTaskRoot = false
    }
  }
  output({ schemaVersion: 1, lane, cleanedExactTaskRoot })
}
