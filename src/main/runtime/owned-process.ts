import { execFile } from 'node:child_process'
import type { ChildProcess } from 'node:child_process'
import { resolveWindowsSystemExecutable } from '../data-analysis/native-runtime-paths'

const WINDOWS_TREE_TERMINATION_TIMEOUT_MS = 5_000
const WINDOWS_TREE_TERMINATION_MAX_OUTPUT_BYTES = 64 << 10
const OWNED_PROCESS_POLL_INTERVAL_MS = 25

export type OwnedProcessExit = Readonly<{
  code: number | null
  signal: NodeJS.Signals | null
  spawnError: Error | null
}>

export type OwnedProcessStopResult = Readonly<{
  pid: number | null
  termination:
    | 'already_exited'
    | 'sigterm'
    | 'sigkill'
    | 'windows_tree'
    | 'windows_tree_forced'
  signals: readonly string[]
  exit: OwnedProcessExit
}>

export type OwnedProcessHandle = Readonly<{
  process: ChildProcess
  pid: number | null
  processGroupId: number | null
  platform: NodeJS.Platform
  exit: Promise<OwnedProcessExit>
}>

type ProcessKill = (pid: number, signal?: NodeJS.Signals | 0) => true
type WindowsTreeTerminator = (pid: number, force: boolean) => Promise<void>

type OwnedProcessState = {
  exitStatus: OwnedProcessExit | null
  resolveExit: (exit: OwnedProcessExit) => void
  stopPromise: Promise<OwnedProcessStopResult> | null
  processKill: ProcessKill
  windowsTreeTerminator: WindowsTreeTerminator
}

const ownedProcessStates = new WeakMap<OwnedProcessHandle, OwnedProcessState>()

export function ownSpawnedProcess(
  child: ChildProcess,
  options: {
    detached: boolean
    platform?: NodeJS.Platform
    processKill?: ProcessKill
    windowsTreeTerminator?: WindowsTreeTerminator
    env?: NodeJS.ProcessEnv
  }
): OwnedProcessHandle {
  const platform = options.platform ?? process.platform
  if (platform !== 'win32' && !options.detached) {
    throw new Error('Owned process launch requires an isolated detached process group.')
  }

  const pid = Number.isSafeInteger(child.pid) && Number(child.pid) > 0
    ? Number(child.pid)
    : null
  let resolveExit!: (exit: OwnedProcessExit) => void
  const handle: OwnedProcessHandle = {
    process: child,
    pid,
    processGroupId: platform !== 'win32' && options.detached && pid !== null ? pid : null,
    platform,
    exit: new Promise<OwnedProcessExit>((resolve) => {
      resolveExit = resolve
    })
  }
  const state: OwnedProcessState = {
    exitStatus: null,
    resolveExit,
    stopPromise: null,
    processKill: options.processKill ?? process.kill.bind(process),
    windowsTreeTerminator: options.windowsTreeTerminator ??
      ((ownedPid, force) => terminateWindowsProcessTree(
        ownedPid,
        force,
        options.env ?? process.env
      ))
  }
  ownedProcessStates.set(handle, state)

  const settle = (exit: OwnedProcessExit): void => {
    if (state.exitStatus !== null) return
    state.exitStatus = exit
    state.resolveExit(exit)
  }
  child.once('exit', (code, signal) => {
    settle({ code, signal, spawnError: null })
  })
  child.once('error', (error) => {
    // Once spawn returned a PID, an error notification is not proof that the
    // captured process or its descendants exited. Keep waiting for exit.
    if (pid === null) settle({ code: null, signal: null, spawnError: error })
  })
  if (child.exitCode !== null || child.signalCode !== null) {
    settle({
      code: child.exitCode,
      signal: child.signalCode,
      spawnError: null
    })
  }
  return handle
}

export function isOwnedProcessRunning(handle: OwnedProcessHandle | null): boolean {
  if (handle === null) return false
  const state = ownedProcessState(handle)
  return state.exitStatus === null &&
    handle.process.exitCode === null &&
    handle.process.signalCode === null
}

export function stopOwnedProcess(
  handle: OwnedProcessHandle,
  options: {
    graceMs?: number
    killWaitMs?: number
  } = {}
): Promise<OwnedProcessStopResult> {
  const state = ownedProcessState(handle)
  if (state.stopPromise === null) {
    state.stopPromise = terminateOwnedProcess(handle, {
      graceMs: Math.max(0, options.graceMs ?? 2_000),
      killWaitMs: Math.max(1, options.killWaitMs ?? 1_000)
    })
  }
  return state.stopPromise
}

async function terminateOwnedProcess(
  handle: OwnedProcessHandle,
  options: {
    graceMs: number
    killWaitMs: number
  }
): Promise<OwnedProcessStopResult> {
  const alreadyExited = await waitForOwnedProcessExit(handle, 0)
  if (alreadyExited !== null) {
    return stopResult(handle, 'already_exited', [], alreadyExited)
  }
  if (handle.pid === null) {
    throw new Error('Owned process has no captured PID and has not reported exit.')
  }

  if (handle.platform === 'win32') {
    return terminateOwnedWindowsProcess(handle, options)
  }

  const signals: NodeJS.Signals[] = []
  if (signalOwnedProcessGroup(handle, 'SIGTERM')) signals.push('SIGTERM')
  const gracefulExit = await waitForOwnedProcessExit(handle, options.graceMs)
  if (gracefulExit !== null) {
    return stopResult(handle, 'sigterm', signals, gracefulExit)
  }

  if (signalOwnedProcessGroup(handle, 'SIGKILL')) signals.push('SIGKILL')
  const forcedExit = await waitForOwnedProcessExit(handle, options.killWaitMs)
  if (forcedExit === null) {
    throw new Error(
      `Owned process group ${handle.processGroupId ?? 'unavailable'} did not fully exit after SIGKILL within ${options.killWaitMs}ms.`
    )
  }
  return stopResult(handle, 'sigkill', signals, forcedExit)
}

async function terminateOwnedWindowsProcess(
  handle: OwnedProcessHandle,
  options: {
    graceMs: number
    killWaitMs: number
  }
): Promise<OwnedProcessStopResult> {
  const state = ownedProcessState(handle)
  const pid = handle.pid
  if (pid === null) {
    throw new Error('Owned Windows process has no captured PID and has not reported exit.')
  }

  try {
    await state.windowsTreeTerminator(pid, false)
  } catch {
    const concurrentExit = await waitForOwnedProcessExit(handle, options.graceMs)
    if (concurrentExit !== null) {
      return stopResult(handle, 'already_exited', [], concurrentExit)
    }
    throw new Error(`Owned Windows process tree ${pid} could not be terminated safely.`)
  }
  const gracefulExit = await waitForOwnedProcessExit(handle, options.graceMs)
  if (gracefulExit !== null) {
    return stopResult(handle, 'windows_tree', ['WINDOWS_TREE'], gracefulExit)
  }

  try {
    await state.windowsTreeTerminator(pid, true)
  } catch {
    const concurrentExit = await waitForOwnedProcessExit(handle, options.killWaitMs)
    if (concurrentExit !== null) {
      return stopResult(handle, 'windows_tree', ['WINDOWS_TREE'], concurrentExit)
    }
    throw new Error(`Owned Windows process tree ${pid} could not be force-terminated safely.`)
  }
  const forcedExit = await waitForOwnedProcessExit(handle, options.killWaitMs)
  if (forcedExit === null) {
    throw new Error(
      `Owned Windows process tree ${pid} did not fully exit after forced termination within ${options.killWaitMs}ms.`
    )
  }
  return stopResult(
    handle,
    'windows_tree_forced',
    ['WINDOWS_TREE', 'WINDOWS_TREE_FORCE'],
    forcedExit
  )
}

function signalOwnedProcessGroup(
  handle: OwnedProcessHandle,
  signal: NodeJS.Signals
): boolean {
  const state = ownedProcessState(handle)
  if (ownedProcessExitIfComplete(handle) !== null) return false
  if (handle.processGroupId === null) {
    throw new Error('Owned process has no captured process-group identity.')
  }
  try {
    state.processKill(-handle.processGroupId, signal)
    return true
  } catch (error) {
    if (isNoSuchProcess(error)) return false
    throw error
  }
}

function ownedProcessExitIfComplete(handle: OwnedProcessHandle): OwnedProcessExit | null {
  const state = ownedProcessState(handle)
  if (state.exitStatus === null &&
    (handle.process.exitCode !== null || handle.process.signalCode !== null)) {
    state.exitStatus = {
      code: handle.process.exitCode,
      signal: handle.process.signalCode,
      spawnError: null
    }
    state.resolveExit(state.exitStatus)
  }
  if (state.exitStatus === null) return null
  if (handle.processGroupId !== null && ownedProcessGroupExists(handle)) return null
  return state.exitStatus
}

function ownedProcessGroupExists(handle: OwnedProcessHandle): boolean {
  const processGroupId = handle.processGroupId
  if (processGroupId === null) return false
  try {
    ownedProcessState(handle).processKill(-processGroupId, 0)
    return true
  } catch (error) {
    if (isNoSuchProcess(error)) return false
    if (isPermissionDenied(error)) return true
    throw error
  }
}

async function waitForOwnedProcessExit(
  handle: OwnedProcessHandle,
  timeoutMs: number
): Promise<OwnedProcessExit | null> {
  const deadline = Date.now() + Math.max(0, timeoutMs)
  while (true) {
    const complete = ownedProcessExitIfComplete(handle)
    if (complete !== null) return complete
    const remaining = deadline - Date.now()
    if (remaining <= 0) return null
    const state = ownedProcessState(handle)
    if (state.exitStatus === null) {
      await Promise.race([
        handle.exit,
        waitForTimer(Math.min(remaining, OWNED_PROCESS_POLL_INTERVAL_MS))
      ])
    } else {
      await waitForTimer(Math.min(remaining, OWNED_PROCESS_POLL_INTERVAL_MS))
    }
  }
}

function stopResult(
  handle: OwnedProcessHandle,
  termination: OwnedProcessStopResult['termination'],
  signals: readonly string[],
  exit: OwnedProcessExit
): OwnedProcessStopResult {
  return {
    pid: handle.pid,
    termination,
    signals,
    exit
  }
}

function ownedProcessState(handle: OwnedProcessHandle): OwnedProcessState {
  const state = ownedProcessStates.get(handle)
  if (!state) throw new Error('Unknown owned process handle.')
  return state
}

function waitForTimer(timeoutMs: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, timeoutMs)
  })
}

function isNoSuchProcess(error: unknown): boolean {
  return error instanceof Error && 'code' in error && error.code === 'ESRCH'
}

function isPermissionDenied(error: unknown): boolean {
  return error instanceof Error && 'code' in error && error.code === 'EPERM'
}

function terminateWindowsProcessTree(
  pid: number,
  force: boolean,
  env: NodeJS.ProcessEnv
): Promise<void> {
  const executable = resolveWindowsSystemExecutable(env, 'taskkill.exe')
  if (!executable) {
    return Promise.reject(new Error('Trusted Windows taskkill.exe is unavailable.'))
  }
  const systemRootEntry = Object.entries(env).find(
    ([key, value]) => key.trim().toUpperCase() === 'SYSTEMROOT' && value !== undefined
  )
  const childEnv = systemRootEntry
    ? { SystemRoot: String(systemRootEntry[1]) }
    : {}
  const args = ['/PID', String(pid), '/T', ...(force ? ['/F'] : [])]
  return new Promise((resolve, reject) => {
    execFile(executable, args, {
      encoding: 'buffer',
      env: childEnv,
      maxBuffer: WINDOWS_TREE_TERMINATION_MAX_OUTPUT_BYTES,
      timeout: WINDOWS_TREE_TERMINATION_TIMEOUT_MS,
      windowsHide: true
    }, (error) => {
      if (error) {
        reject(new Error(`Trusted Windows process-tree termination failed for PID ${pid}.`))
        return
      }
      resolve()
    })
  })
}
