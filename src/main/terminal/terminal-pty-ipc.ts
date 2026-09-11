/**
 * Main-process PTY lifecycle for the built-in terminal.
 *
 * Architecture mirrors `runtime-sse-ipc.ts`: the main process owns the real
 * resource (a node-pty pseudo-terminal), streams chunks to the renderer over
 * `terminal:data`, and reports exit via `terminal:exit`. node-pty is loaded
 * lazily so a missing/broken native build disables the terminal gracefully
 * instead of crashing app startup.
 *
 * Process containment is currently available on macOS. Other hosts reject
 * only this terminal effect; the rest of the general Agent remains usable.
 */
import { existsSync } from 'node:fs'
import { homedir } from 'node:os'
import { dirname, join } from 'node:path'
import { pathToFileURL } from 'node:url'
import type {
  BrowserWindow,
  IpcMain,
  IpcMainInvokeEvent,
  WebContents,
  WebFrameMain
} from 'electron'
import type { IPty } from 'node-pty'
import {
  TERMINAL_DEFAULT_COLS,
  TERMINAL_DEFAULT_ROWS,
  TERMINAL_MAX_SESSIONS,
  TERMINAL_RING_BUFFER_BYTES
} from '../../shared/terminal'
import {
  terminalCreatePayloadSchema,
  terminalResizePayloadSchema,
  terminalSessionIdSchema,
  terminalWritePayloadSchema
} from '../ipc/app-ipc-schemas'
import { publicConsoleWarn } from '../logger'
import {
  prepareTerminalProcessLaunch,
  TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
} from './terminal-process-policy'

type TerminalSession = {
  pty: IPty
  sender: WebContents
  senderFrame: WebFrameMain
  /** Last ~64KB of output, replayed when a panel re-attaches. */
  ringBuffer: string
  exited: boolean
}

type PendingTerminalSession = {
  senderFrame: WebFrameMain
  controllerGeneration: number
  senderGeneration: number
}

let nodePty: typeof import('node-pty') | null | undefined

export function packagedDarwinNodePtyEntryPath(
  platform = process.platform,
  executablePath = process.execPath,
  pathExists: (path: string) => boolean = existsSync
): string {
  if (platform !== 'darwin') return ''
  const entryPath = join(dirname(executablePath), 'analytix-node-pty', 'lib', 'index.js')
  return pathExists(entryPath) ? entryPath : ''
}

async function loadNodePty(): Promise<typeof import('node-pty') | null> {
  if (nodePty !== undefined) return nodePty
  try {
    // Dynamic import keeps the main bundle compiling even if the native
    // prebuild is missing on the current platform; failure surfaces as a
    // friendly message in the panel instead of a hard crash.
    const packagedEntryPath = packagedDarwinNodePtyEntryPath()
    const loaded = packagedEntryPath
      ? await import(/* @vite-ignore */ pathToFileURL(packagedEntryPath).href)
      : await import('node-pty')
    const candidate = (
      typeof loaded.spawn === 'function'
        ? loaded
        : loaded.default
    ) as typeof import('node-pty') | undefined
    if (!candidate || typeof candidate.spawn !== 'function') {
      throw new Error('node-pty module does not export spawn')
    }
    nodePty = candidate
  } catch {
    publicConsoleWarn(
      'terminal',
      'node-pty failed to load; built-in terminal disabled.',
      { code: 'terminal_backend_unavailable' }
    )
    nodePty = null
  }
  return nodePty
}

/**
 * Pick a default shell for the current platform.
 *
 * macOS: respects $SHELL (set by the OS for the user's default terminal),
 *        falling back to zsh which has shipped as the system default since
 *        Catalina.
 * Linux: respects $SHELL, falling back to bash (the de-facto standard).
 * Windows: PowerShell 7 (pwsh.exe) if installed, else Windows PowerShell,
 *        else the COMSPEC command interpreter (usually cmd.exe).
 */
function resolveDefaultShell(platform = process.platform): { file: string; args: string[] } {
  if (platform === 'darwin') {
    // A fixed no-rc shell prevents user startup files from restoring ambient
    // credentials or history after the main process has scrubbed the env.
    return { file: '/bin/zsh', args: ['-f'] }
  }
  return { file: '', args: [] }
}

function pushToRingBuffer(session: TerminalSession, chunk: string): void {
  session.ringBuffer += chunk
  if (session.ringBuffer.length > TERMINAL_RING_BUFFER_BYTES) {
    session.ringBuffer = session.ringBuffer.slice(-TERMINAL_RING_BUFFER_BYTES)
  }
}

function sendToSender(sender: WebContents, channel: string, payload: unknown): void {
  if (sender.isDestroyed()) return
  try {
    sender.send(channel, payload)
  } catch {
    // Renderer teardown races are handled by sender invalidation.
  }
}

export function killTerminalPtyIfRunning(
  pty: Pick<IPty, 'kill'>,
  exited: boolean
): void {
  if (!exited) pty.kill()
}

export type RegisterTerminalPtyIpcOptions = {
  ipcMain: IpcMain
  getMainWindow: () => BrowserWindow | null
  getProtectedRoots: () => Promise<readonly string[]>
  isTrustedSender: (event: IpcMainInvokeEvent) => boolean
  logError: (category: string, message: string, detail?: unknown) => void
  loadPtyModule?: () => Promise<typeof import('node-pty') | null>
  registerBeforeQuit?: (listener: () => void) => void
  platform?: NodeJS.Platform
  env?: NodeJS.ProcessEnv
}

export type TerminalPtyIpcController = Readonly<{
  disposeAll: () => void
  suspendCreates: () => () => void
  withCreatesSuspended: <T>(operation: () => Promise<T>) => Promise<T>
}>

export function registerTerminalPtyIpc(
  options: RegisterTerminalPtyIpcOptions
): TerminalPtyIpcController {
  const {
    ipcMain,
    getMainWindow,
    getProtectedRoots,
    isTrustedSender,
    logError
  } = options
  const sessions = new Map<WebContents, Map<string, TerminalSession>>()
  const pendingSessions = new Map<WebContents, Map<string, PendingTerminalSession>>()
  const observedSenders = new WeakSet<WebContents>()
  const senderGenerations = new WeakMap<WebContents, number>()
  let controllerGeneration = 0
  let createSuspensionDepth = 0

  const getSenderGeneration = (sender: WebContents): number =>
    senderGenerations.get(sender) ?? 0

  const invalidateSenderGeneration = (sender: WebContents): void => {
    senderGenerations.set(sender, getSenderGeneration(sender) + 1)
  }

  const sessionFor = (
    sender: WebContents,
    sessionId: string
  ): TerminalSession | undefined => sessions.get(sender)?.get(sessionId)

  const pendingFor = (
    sender: WebContents,
    sessionId: string
  ): PendingTerminalSession | undefined => pendingSessions.get(sender)?.get(sessionId)

  const setPending = (
    sender: WebContents,
    sessionId: string,
    pending: PendingTerminalSession
  ): void => {
    const senderPending = pendingSessions.get(sender) ?? new Map()
    senderPending.set(sessionId, pending)
    pendingSessions.set(sender, senderPending)
  }

  const deletePending = (
    sender: WebContents,
    sessionId: string,
    expected?: PendingTerminalSession
  ): boolean => {
    const senderPending = pendingSessions.get(sender)
    if (!senderPending) return false
    const current = senderPending.get(sessionId)
    if (!current || (expected && current !== expected)) return false
    senderPending.delete(sessionId)
    if (senderPending.size === 0) pendingSessions.delete(sender)
    return true
  }

  const totalSessionCount = (): number => {
    let count = 0
    for (const senderSessions of sessions.values()) count += senderSessions.size
    for (const senderPending of pendingSessions.values()) count += senderPending.size
    return count
  }

  const disposeSession = (
    sender: WebContents,
    sessionId: string,
    killedByClient: boolean
  ): boolean => {
    const senderSessions = sessions.get(sender)
    const session = senderSessions?.get(sessionId)
    if (!session) return false
    const alreadyExited = session.exited
    session.exited = true
    try {
      killTerminalPtyIfRunning(session.pty, alreadyExited)
    } catch {
      logError('terminal', 'Failed to kill PTY process', {
        code: 'terminal_pty_kill_failed'
      })
    }
    senderSessions!.delete(sessionId)
    if (senderSessions!.size === 0) sessions.delete(sender)
    if (!killedByClient && !session.sender.isDestroyed()) {
      sendToSender(session.sender, 'terminal:exit', { sessionId, exitCode: null })
    }
    return true
  }

  const disposeForSender = (sender: WebContents): void => {
    invalidateSenderGeneration(sender)
    pendingSessions.delete(sender)
    for (const sessionId of Array.from(sessions.get(sender)?.keys() ?? [])) {
      disposeSession(sender, sessionId, true)
    }
  }

  // When the renderer window closes, tear down any PTY it owned. Listening
  // on the main window's webContents covers the normal single-window case.
  const attachSenderCleanup = (sender: WebContents): void => {
    if (observedSenders.has(sender)) return
    observedSenders.add(sender)
    if (sender.isDestroyed()) {
      disposeForSender(sender)
      return
    }
    sender.once('destroyed', () => disposeForSender(sender))
    sender.on(
      'did-start-navigation',
      (event: Electron.Event<Electron.WebContentsDidStartNavigationEventParams>) => {
        if (event.isMainFrame && !event.isSameDocument) disposeForSender(sender)
      }
    )
  }

  ipcMain.handle('terminal:create', async (event, args: unknown) => {
    const request = terminalCreatePayloadSchema.parse(args)
    if (
      createSuspensionDepth > 0 ||
      !trustedTerminalEvent(event, isTrustedSender)
    ) {
      return {
        ok: false as const,
        message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
      }
    }

    attachSenderCleanup(event.sender)

    // Re-attach to an existing session: replay the ring buffer so reopening
    // the panel shows recent output instead of a blank screen.
    const existing = sessionFor(event.sender, request.sessionId)
    if (existing) {
      if (!ownsTerminalSession(existing, event)) {
        return {
          ok: false as const,
          message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
        }
      }
    }
    if (existing && !existing.exited) {
      if (existing.ringBuffer) {
        sendToSender(event.sender, 'terminal:data', {
          sessionId: request.sessionId,
          data: existing.ringBuffer
        })
      }
      return { ok: true as const, sessionId: request.sessionId, replayed: true }
    }
    if (existing && existing.exited) {
      disposeSession(event.sender, request.sessionId, true)
    }

    if (pendingFor(event.sender, request.sessionId)) {
      return {
        ok: false as const,
        message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
      }
    }
    if (totalSessionCount() >= TERMINAL_MAX_SESSIONS) {
      return {
        ok: false as const,
        message: `Too many terminal sessions (limit ${TERMINAL_MAX_SESSIONS}).`
      }
    }

    const { file, args: shellArgs } = resolveDefaultShell(options.platform)
    const cols = request.cols ?? TERMINAL_DEFAULT_COLS
    const rows = request.rows ?? TERMINAL_DEFAULT_ROWS
    const cwd = request.cwd && request.cwd.trim() ? request.cwd.trim() : homedir()
    const pending: PendingTerminalSession = {
      senderFrame: event.senderFrame!,
      controllerGeneration,
      senderGeneration: getSenderGeneration(event.sender)
    }
    setPending(event.sender, request.sessionId, pending)

    try {
      const ptyModule = await (options.loadPtyModule ?? loadNodePty)()
      if (!pendingTerminalSessionIsCurrent(
        event,
        pending,
        pendingFor(event.sender, request.sessionId),
        controllerGeneration,
        getSenderGeneration(event.sender),
        createSuspensionDepth === 0,
        isTrustedSender
      )) {
        return {
          ok: false as const,
          message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
        }
      }
      if (!ptyModule) {
        return {
          ok: false as const,
          message: 'The terminal backend (node-pty) is not available on this system.'
        }
      }
      const protectedRoots = await getProtectedRoots()
      if (!pendingTerminalSessionIsCurrent(
        event,
        pending,
        pendingFor(event.sender, request.sessionId),
        controllerGeneration,
        getSenderGeneration(event.sender),
        createSuspensionDepth === 0,
        isTrustedSender
      )) {
        return {
          ok: false as const,
          message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
        }
      }
      const launch = prepareTerminalProcessLaunch({
        shellFile: file,
        shellArgs,
        cwd,
        protectedRoots,
        platform: options.platform,
        env: options.env
      })
      const pty = ptyModule.spawn(launch.file, [...launch.args], {
        name: 'xterm-256color',
        cols,
        rows,
        cwd: launch.cwd,
        env: launch.env,
        // ConPTY on Windows, ignored elsewhere.
        useConpty: true
      })
      if (!pendingTerminalSessionIsCurrent(
        event,
        pending,
        pendingFor(event.sender, request.sessionId),
        controllerGeneration,
        getSenderGeneration(event.sender),
        createSuspensionDepth === 0,
        isTrustedSender
      )) {
        try {
          pty.kill()
        } catch {
          // The fixed unavailable result remains the only public projection.
        }
        return {
          ok: false as const,
          message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
        }
      }

      const session: TerminalSession = {
        pty,
        sender: event.sender,
        senderFrame: event.senderFrame!,
        ringBuffer: '',
        exited: false
      }
      const senderSessions = sessions.get(event.sender) ?? new Map()
      senderSessions.set(request.sessionId, session)
      sessions.set(event.sender, senderSessions)
      deletePending(event.sender, request.sessionId, pending)

      pty.onData((data) => {
        if (session.exited) return
        pushToRingBuffer(session, data)
        sendToSender(session.sender, 'terminal:data', { sessionId: request.sessionId, data })
      })

      pty.onExit(({ exitCode }) => {
        if (session.exited) return
        session.exited = true
        sendToSender(session.sender, 'terminal:exit', { sessionId: request.sessionId, exitCode })
        // Keep the entry briefly so a slow re-attach can still replay; the
        // next create disposes it. Full cleanup also happens on app quit.
      })

      return { ok: true as const, sessionId: request.sessionId }
    } catch {
      logError('terminal', 'Failed to spawn contained PTY', {
        code: 'terminal_process_containment_unavailable'
      })
      return {
        ok: false as const,
        message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
      }
    } finally {
      deletePending(event.sender, request.sessionId, pending)
    }
  })

  ipcMain.handle('terminal:write', async (event, args: unknown) => {
    const request = terminalWritePayloadSchema.parse(args)
    const session = sessionFor(event.sender, request.sessionId)
    if (
      !session ||
      session.exited ||
      !trustedTerminalEvent(event, isTrustedSender) ||
      !ownsTerminalSession(session, event)
    ) {
      return false
    }
    try {
      session.pty.write(request.data)
      return true
    } catch {
      logError('terminal', 'Failed to write to PTY', {
        code: 'terminal_pty_write_failed'
      })
      return false
    }
  })

  ipcMain.handle('terminal:resize', async (event, args: unknown) => {
    const request = terminalResizePayloadSchema.parse(args)
    const session = sessionFor(event.sender, request.sessionId)
    if (
      !session ||
      session.exited ||
      !trustedTerminalEvent(event, isTrustedSender) ||
      !ownsTerminalSession(session, event)
    ) {
      return false
    }
    try {
      session.pty.resize(request.cols, request.rows)
      return true
    } catch {
      logError('terminal', 'Failed to resize PTY', {
        code: 'terminal_pty_resize_failed'
      })
      return false
    }
  })

  ipcMain.handle('terminal:dispose', async (event, sessionId: unknown) => {
    const normalized = terminalSessionIdSchema.parse(sessionId)
    if (!trustedTerminalEvent(event, isTrustedSender)) {
      return false
    }
    const pending = pendingFor(event.sender, normalized)
    if (pending) {
      if (pending.senderFrame !== event.senderFrame) return false
      return deletePending(event.sender, normalized, pending)
    }
    const session = sessionFor(event.sender, normalized)
    if (!session || !ownsTerminalSession(session, event)) return false
    return disposeSession(event.sender, normalized, true)
  })

  const disposeAll = (): void => {
    controllerGeneration += 1
    pendingSessions.clear()
    for (const [sender, senderSessions] of Array.from(sessions.entries())) {
      for (const sessionId of Array.from(senderSessions.keys())) {
        disposeSession(sender, sessionId, false)
      }
    }
  }
  options.registerBeforeQuit?.(disposeAll)
  const suspendCreates = (): (() => void) => {
    createSuspensionDepth += 1
    disposeAll()
    let released = false
    return () => {
      if (released) return
      released = true
      disposeAll()
      createSuspensionDepth -= 1
    }
  }
  const withCreatesSuspended = async <T>(
    operation: () => Promise<T>
  ): Promise<T> => {
    const release = suspendCreates()
    try {
      return await operation()
    } finally {
      release()
    }
  }

  // If the main window is recreated (e.g. on macOS reactivation), make sure
  // stale sessions bound to a destroyed window are torn down.
  const mainWindow = getMainWindow()
  if (mainWindow && !mainWindow.isDestroyed()) {
    attachSenderCleanup(mainWindow.webContents)
  }
  return Object.freeze({ disposeAll, suspendCreates, withCreatesSuspended })
}

function trustedTerminalEvent(
  event: IpcMainInvokeEvent,
  isTrustedSender: (event: IpcMainInvokeEvent) => boolean
): boolean {
  try {
    return Boolean(
      event.senderFrame &&
      event.senderFrame === event.sender.mainFrame &&
      isTrustedSender(event)
    )
  } catch {
    return false
  }
}

function ownsTerminalSession(
  session: TerminalSession,
  event: IpcMainInvokeEvent
): boolean {
  return session.sender === event.sender && session.senderFrame === event.senderFrame
}

function pendingTerminalSessionIsCurrent(
  event: IpcMainInvokeEvent,
  expected: PendingTerminalSession,
  current: PendingTerminalSession | undefined,
  controllerGeneration: number,
  senderGeneration: number,
  createsAllowed: boolean,
  isTrustedSender: (event: IpcMainInvokeEvent) => boolean
): boolean {
  return createsAllowed &&
    current === expected &&
    expected.controllerGeneration === controllerGeneration &&
    expected.senderGeneration === senderGeneration &&
    expected.senderFrame === event.senderFrame &&
    !event.sender.isDestroyed() &&
    trustedTerminalEvent(event, isTrustedSender)
}
