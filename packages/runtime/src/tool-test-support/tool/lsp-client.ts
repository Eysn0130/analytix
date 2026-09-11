/**
 * Minimal LSP client for the `lsp` AI tool.
 *
 * Speaks JSON-RPC 2.0 over stdio to a language server process (currently
 * typescript-language-server). Implements only what the tool needs:
 * initialize, textDocument/didOpen, and a handful of textDocument/ and
 * workspace/ requests. No subscriptions, no diagnostics, no completion —
 * this is a query-only client, not a full editor integration.
 *
 * Sessions are keyed by workspace root and reference-counted; when the last
 * caller releases, the server is kept alive briefly (CLEANUP_DELAY) before
 * being killed, so back-to-back tool calls don't respawn the server.
 */

import { spawn, type ChildProcess } from 'node:child_process'
import { randomUUID } from 'node:crypto'
import { pathToFileURL } from 'node:url'

const CLEANUP_DELAY = 30_000
const REQUEST_TIMEOUT = 30_000
const SERVER_PROBE_TIMEOUT = 3_000

interface LspSession {
  process: ChildProcess
  workspaceRoot: string
  refCount: number
  cleanupTimer: NodeJS.Timeout | null
  /** Sequential buffer for stdout framing. */
  stdoutBuffer: string
  /** Pending JSON-RPC requests awaiting a response, keyed by id. */
  pending: Map<string, { resolve: (v: unknown) => void; reject: (e: Error) => void; timer: NodeJS.Timeout }>
  /** Monotonic request id counter. */
  nextId: number
  initialized: boolean
  initPromise: Promise<void> | null
}

/**
 * Map<workspaceRoot, Promise<LspSession>>.
 * Storing the Promise (not the resolved session) prevents a race where two
 * concurrent acquireLspSession calls both see sessions.get() === undefined
 * and each spawns their own server. The first caller writes its Promise
 * into the map before awaiting anything; the second caller awaits the same
 * Promise.
 */
const sessions = new Map<string, Promise<LspSession>>()

/**
 * Check whether a language server binary is available on PATH.
 * Returns the binary name if found, null otherwise.
 */
async function probeServerBinary(binary: string): Promise<string | null> {
  return new Promise((resolve) => {
    const child = spawn('which', [binary], { stdio: ['ignore', 'pipe', 'ignore'], timeout: SERVER_PROBE_TIMEOUT })
    let stdout = ''
    child.stdout?.on('data', (chunk) => { stdout += chunk })
    child.on('error', () => resolve(null))
    child.on('close', (code) => {
      resolve(code === 0 && stdout.trim() ? stdout.trim() : null)
    })
  })
}

/**
 * Resolve the typescript-language-server binary.
 * Returns the spawn command + args, or null if not available.
 */
export async function resolveTsLsCommand(workspaceRoot: string): Promise<{ command: string; args: string[] } | null> {
  // Prefer a project-local install.
  const localPath = `${workspaceRoot}/node_modules/.bin/typescript-language-server`
  try {
    const { access } = await import('node:fs/promises')
    await access(localPath)
    return { command: localPath, args: ['--stdio'] }
  } catch {
    // fall through to PATH lookup
  }
  const found = await probeServerBinary('typescript-language-server')
  if (found) return { command: 'typescript-language-server', args: ['--stdio'] }
  return null
}

function killSession(session: LspSession): void {
  if (session.cleanupTimer) {
    clearTimeout(session.cleanupTimer)
    session.cleanupTimer = null
  }
  for (const [id, entry] of session.pending) {
    clearTimeout(entry.timer)
    entry.reject(new Error('LSP session closed'))
    session.pending.delete(id)
  }
  try {
    session.process.kill('SIGTERM')
  } catch {
    // already dead
  }
  // Force-kill after grace period.
  setTimeout(() => {
    try { session.process.kill('SIGKILL') } catch { /* ignore */ }
  }, 2_000)
}

function sendMessage(session: LspSession, message: Record<string, unknown>): void {
  const body = JSON.stringify(message)
  const chunk = `Content-Length: ${Buffer.byteLength(body)}\r\n\r\n${body}`
  session.process.stdin?.write(chunk)
}

function handleResponse(session: LspSession, msg: Record<string, unknown>): void {
  const id = String(msg.id ?? '')
  const entry = session.pending.get(id)
  if (!entry) return
  clearTimeout(entry.timer)
  session.pending.delete(id)
  if (msg.error) {
    entry.reject(new Error(typeof msg.error === 'object' && msg.error !== null && 'message' in msg.error
      ? String((msg.error as { message: unknown }).message)
      : 'LSP request failed'))
  } else {
    entry.resolve(msg.result)
  }
}

function processBuffer(session: LspSession): void {
  while (true) {
    const headerEnd = session.stdoutBuffer.indexOf('\r\n\r\n')
    if (headerEnd === -1) return
    const header = session.stdoutBuffer.slice(0, headerEnd)
    const lengthMatch = header.match(/Content-Length:\s*(\d+)/i)
    if (!lengthMatch) {
      // Malformed; drop the header to resync.
      session.stdoutBuffer = session.stdoutBuffer.slice(headerEnd + 4)
      continue
    }
    const contentLength = Number(lengthMatch[1])
    const bodyStart = headerEnd + 4
    if (session.stdoutBuffer.length < bodyStart + contentLength) return // incomplete
    const body = session.stdoutBuffer.slice(bodyStart, bodyStart + contentLength)
    session.stdoutBuffer = session.stdoutBuffer.slice(bodyStart + contentLength)
    try {
      const msg = JSON.parse(body) as Record<string, unknown>
      if (msg.id !== undefined && (msg.result !== undefined || msg.error !== undefined)) {
        handleResponse(session, msg)
      }
      // Notifications (no id) are ignored — this client doesn't subscribe to anything.
    } catch {
      // Malformed JSON; skip.
    }
  }
}

async function initialize(session: LspSession): Promise<void> {
  if (session.initialized) return
  if (session.initPromise) return session.initPromise

  session.initPromise = (async () => {
    const initResult = await sendRequest(session, 'initialize', {
      processId: process.pid,
      rootUri: pathToFileURL(session.workspaceRoot).href,
      capabilities: {
        textDocument: {
          synchronization: { didOpen: true, didChange: false, willSave: false },
          hover: { contentFormat: ['markdown', 'plaintext'] },
          definition: { linkSupport: false },
          references: {},
          documentSymbol: { hierarchicalDocumentSymbolSupport: false },
          implementation: {},
          callHierarchy: { dynamicRegistration: false }
        },
        workspace: {
          symbol: {}
        }
      },
      workspaceFolders: null
    }).catch((err) => {
      throw new Error(`LSP initialize failed: ${err instanceof Error ? err.message : String(err)}`)
    })

    // ts_ls / typescript-language-server: pull out tsserver path from init result if present.
    void initResult
    sendNotification(session, 'initialized', {})
    session.initialized = true
  })()

  return session.initPromise
}

function sendRequest(session: LspSession, method: string, params: Record<string, unknown> | null): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const id = String(session.nextId++)
    const timer = setTimeout(() => {
      session.pending.delete(id)
      reject(new Error(`LSP request "${method}" timed out after ${REQUEST_TIMEOUT}ms`))
    }, REQUEST_TIMEOUT)
    session.pending.set(id, { resolve, reject, timer })
    sendMessage(session, { jsonrpc: '2.0', id, method, params: params ?? {} })
  })
}

function sendNotification(session: LspSession, method: string, params: Record<string, unknown>): void {
  sendMessage(session, { jsonrpc: '2.0', method, params })
}

/**
 * Acquire (or reuse) an LSP session for the given workspace.
 * The returned session is initialized and ready for requests.
 * Caller MUST call releaseLspSession when done.
 *
 * Race-safe: the in-flight Promise is written to the sessions map before
 * any await, so concurrent callers get the same session.
 */
export async function acquireLspSession(workspaceRoot: string): Promise<LspSession> {
  const existing = sessions.get(workspaceRoot)
  if (existing) {
    const session = await existing
    if (session.cleanupTimer) {
      clearTimeout(session.cleanupTimer)
      session.cleanupTimer = null
    }
    session.refCount += 1
    await initialize(session)
    return session
  }

  // Write the Promise immediately to prevent concurrent spawns.
  const sessionPromise = createSession(workspaceRoot)
  sessions.set(workspaceRoot, sessionPromise)

  try {
    const session = await sessionPromise
    return session
  } catch (err) {
    // If spawn/init fails, remove the placeholder so the next call can retry.
    sessions.delete(workspaceRoot)
    throw err
  }
}

async function createSession(workspaceRoot: string): Promise<LspSession> {
  const cmd = await resolveTsLsCommand(workspaceRoot)
  if (!cmd) {
    throw new Error(
      'typescript-language-server is not installed. Install it with `npm install -g typescript-language-server typescript` and try again.'
    )
  }

  const proc = spawn(cmd.command, cmd.args, {
    stdio: ['pipe', 'pipe', 'pipe'],
    cwd: workspaceRoot,
    env: process.env,
    windowsHide: true
  })

  const session: LspSession = {
    process: proc,
    workspaceRoot,
    refCount: 1,
    cleanupTimer: null,
    stdoutBuffer: '',
    pending: new Map(),
    nextId: 1,
    initialized: false,
    initPromise: null
  }

  proc.stdout?.on('data', (chunk: Buffer) => {
    session.stdoutBuffer += chunk.toString('utf-8')
    processBuffer(session)
  })

  proc.on('error', (err) => {
    // Kill + reject pending rather than throwing (throw in an event handler
    // becomes an uncaught exception).
    killSession(session)
    sessions.delete(workspaceRoot)
    void err
  })

  proc.on('exit', () => {
    // Reject all pending requests so callers don't hang for 30s.
    killSession(session)
    sessions.delete(workspaceRoot)
  })

  await initialize(session)
  return session
}

/**
 * Kill all active LSP sessions. Should be called on app quit / process exit
 * to prevent orphaned language-server processes.
 */
export function shutdownAllLspSessions(): void {
  for (const [workspaceRoot, sessionPromise] of sessions) {
    sessions.delete(workspaceRoot)
    sessionPromise
      .then((session) => killSession(session))
      .catch(() => { /* session failed to init — nothing to kill */ })
  }
}

export function releaseLspSession(workspaceRoot: string): void {
  const sessionPromise = sessions.get(workspaceRoot)
  if (!sessionPromise) return
  sessionPromise
    .then((session) => {
      session.refCount -= 1
      if (session.refCount <= 0) {
        session.cleanupTimer = setTimeout(() => {
          const sp = sessions.get(workspaceRoot)
          if (!sp) return
          sp.then((s) => {
            if (s.refCount <= 0) {
              killSession(s)
              sessions.delete(workspaceRoot)
            }
          }).catch(() => { /* ignore */ })
        }, CLEANUP_DELAY)
      }
    })
    .catch(() => { /* session failed to init — nothing to release */ })
  }

// --- LSP operations ---

function filePathToUri(filePath: string): string {
  return pathToFileURL(filePath).href
}

function languageIdForFile(filePath: string): string {
  if (filePath.endsWith('.ts')) return 'typescript'
  if (filePath.endsWith('.tsx')) return 'typescriptreact'
  if (filePath.endsWith('.js')) return 'javascript'
  if (filePath.endsWith('.jsx')) return 'javascriptreact'
  return 'typescript'
}

export async function lspOpenDocument(session: LspSession, filePath: string, content: string): Promise<void> {
  sendNotification(session, 'textDocument/didOpen', {
    textDocument: {
      uri: filePathToUri(filePath),
      languageId: languageIdForFile(filePath),
      version: 1,
      text: content
    }
  })
}

export async function lspCloseDocument(session: LspSession, filePath: string): Promise<void> {
  sendNotification(session, 'textDocument/didClose', {
    textDocument: { uri: filePathToUri(filePath) }
  })
}

export async function lspDefinition(
  session: LspSession,
  filePath: string,
  line: number,
  character: number
): Promise<unknown> {
  return sendRequest(session, 'textDocument/definition', {
    textDocument: { uri: filePathToUri(filePath) },
    position: { line, character }
  })
}

export async function lspReferences(
  session: LspSession,
  filePath: string,
  line: number,
  character: number
): Promise<unknown> {
  return sendRequest(session, 'textDocument/references', {
    textDocument: { uri: filePathToUri(filePath) },
    position: { line, character },
    context: { includeDeclaration: true }
  })
}

export async function lspHover(
  session: LspSession,
  filePath: string,
  line: number,
  character: number
): Promise<unknown> {
  return sendRequest(session, 'textDocument/hover', {
    textDocument: { uri: filePathToUri(filePath) },
    position: { line, character }
  })
}

export async function lspImplementation(
  session: LspSession,
  filePath: string,
  line: number,
  character: number
): Promise<unknown> {
  return sendRequest(session, 'textDocument/implementation', {
    textDocument: { uri: filePathToUri(filePath) },
    position: { line, character }
  })
}

export async function lspDocumentSymbol(session: LspSession, filePath: string): Promise<unknown> {
  return sendRequest(session, 'textDocument/documentSymbol', {
    textDocument: { uri: filePathToUri(filePath) }
  })
}

export async function lspWorkspaceSymbol(session: LspSession, query: string): Promise<unknown> {
  return sendRequest(session, 'workspace/symbol', { query })
}

/**
 * Synchronous last-resort cleanup on process exit. The exit handler can only
 * run synchronous code, so we SIGKILL immediately (no grace period). This
 * prevents orphaned typescript-language-server / tsserver processes when the
 * host process (Electron / analytix serve) terminates.
 */
function syncKillAll(): void {
  for (const [, sessionPromise] of sessions) {
    sessionPromise
      .then((session) => {
        try { session.process.kill('SIGKILL') } catch { /* already dead */ }
      })
      .catch(() => { /* init failed — no process to kill */ })
  }
  sessions.clear()
}

process.on('exit', syncKillAll)

export type { LspSession }
