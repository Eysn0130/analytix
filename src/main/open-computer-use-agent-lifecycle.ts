import { createConnection } from 'node:net'
import { lstat, realpath } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'

const AGENT_SOCKET_NAME = 'open-computer-use-agent.sock'
const AGENT_BUNDLE_ID = 'com.analytix.computer-use'
const AGENT_EXECUTABLE_NAME = 'OpenComputerUse'
const MAX_RESPONSE_BYTES = 64 * 1024
const REQUEST_TIMEOUT_MS = 1_500
const TERMINATION_TIMEOUT_MS = 2_000

type AgentSocketIdentityV1 = Readonly<{
  dev: bigint
  ino: bigint
}>

type AgentSocketInspectionV1 =
  | Readonly<{ state: 'absent' }>
  | Readonly<{ state: 'unsafe' }>
  | Readonly<{ state: 'present'; identity: AgentSocketIdentityV1 }>

type AgentSocketSessionV1 = Readonly<{
  request: (request: Readonly<Record<string, unknown>>) => Promise<unknown>
  close: () => void
}>

type AgentInfoV1 = Readonly<{
  bundleIdentifier: string
  bundleURL: string
  executableURL: string
  processStartTime: number
}>

export type OpenComputerUseAgentStopStatusV1 =
  | 'terminated'
  | 'not-present'
  | 'not-owned'
  | 'invalid'
  | 'failed'

export type OpenComputerUseAgentStopResultV1 = Readonly<{
  status: OpenComputerUseAgentStopStatusV1
  code: string
}>

export type OpenComputerUseAgentBaselineV1 = Readonly<{
  state: 'absent' | 'present' | 'indeterminate'
  observedAtMs: number
}>

type AgentLifecycleDependenciesV1 = Readonly<{
  socketPath?: string
  inspectSocket?: (socketPath: string) => Promise<AgentSocketInspectionV1>
  openSession?: (socketPath: string) => Promise<AgentSocketSessionV1>
  waitForSocketRemoval?: (socketPath: string, timeoutMs: number) => Promise<boolean>
}>

export async function captureOpenComputerUseAgentBaselineV1(options: Readonly<{
  platform?: NodeJS.Platform
}> = {}): Promise<OpenComputerUseAgentBaselineV1> {
  const observedAtMs = Date.now()
  if ((options.platform ?? process.platform) !== 'darwin') {
    return { state: 'absent', observedAtMs }
  }
  try {
    await lstat(join(tmpdir(), AGENT_SOCKET_NAME))
    return { state: 'present', observedAtMs }
  } catch (error) {
    return {
      state: isNodeErrorCode(error, 'ENOENT') ? 'absent' : 'indeterminate',
      observedAtMs
    }
  }
}

export async function stopOwnedOpenComputerUseAppAgentV1(options: Readonly<{
  command: string | null
  appStartedAtMs: number
  baseline: OpenComputerUseAgentBaselineV1
  platform?: NodeJS.Platform
  dependencies?: AgentLifecycleDependenciesV1
}>): Promise<OpenComputerUseAgentStopResultV1> {
  if ((options.platform ?? process.platform) !== 'darwin') {
    return { status: 'not-present', code: 'platform_not_darwin' }
  }
  const expected = await expectedAgentIdentityV1(options.command)
  if (
    !expected ||
    !Number.isFinite(options.appStartedAtMs) || options.appStartedAtMs <= 0 ||
    !Number.isFinite(options.baseline.observedAtMs) || options.baseline.observedAtMs <= 0 ||
    options.appStartedAtMs > Date.now() + 2_000 ||
    options.baseline.observedAtMs > Date.now() + 2_000
  ) {
    return { status: 'invalid', code: 'agent_identity_unavailable' }
  }
  if (
    options.baseline.state !== 'absent' ||
    options.baseline.observedAtMs + 2_000 < options.appStartedAtMs
  ) {
    return { status: 'not-owned', code: 'agent_clean_start_not_observed' }
  }
  const socketPath = options.dependencies?.socketPath ?? join(tmpdir(), AGENT_SOCKET_NAME)
  const inspectSocket = options.dependencies?.inspectSocket ?? inspectPrivateSocketV1
  const openSession = options.dependencies?.openSession ?? openAgentSessionV1
  const waitForSocketRemoval = options.dependencies?.waitForSocketRemoval ?? waitForSocketRemovalV1
  let session: AgentSocketSessionV1 | null = null
  try {
    const before = await inspectSocket(socketPath)
    if (before.state === 'absent') return { status: 'not-present', code: 'agent_socket_absent' }
    if (before.state === 'unsafe') return { status: 'failed', code: 'agent_socket_untrusted' }
    session = await openSession(socketPath)
    const rawInfo = await session.request({ kind: 'agentInfo' })
    const info = parseAgentInfoV1(rawInfo)
    const agentStartedAtMs = info ? info.processStartTime * 1_000 : 0
    if (
      !info ||
      !sameAgentIdentityV1(info, expected) ||
      agentStartedAtMs + 2_000 < options.appStartedAtMs ||
      agentStartedAtMs + 2_000 < options.baseline.observedAtMs ||
      agentStartedAtMs > Date.now() + 2_000
    ) {
      return { status: 'not-owned', code: 'agent_not_owned_by_current_app' }
    }
    const stable = await inspectSocket(socketPath)
    if (
      stable.state !== 'present' ||
      stable.identity.dev !== before.identity.dev ||
      stable.identity.ino !== before.identity.ino
    ) {
      return { status: 'failed', code: 'agent_socket_identity_changed' }
    }
    const termination = objectOf(await session.request({ kind: 'terminate' }))
    if (termination.ok !== true) return { status: 'failed', code: 'agent_termination_rejected' }
    session.close()
    session = null
    if (!await waitForSocketRemoval(socketPath, TERMINATION_TIMEOUT_MS)) {
      return { status: 'failed', code: 'agent_termination_not_observed' }
    }
    return { status: 'terminated', code: 'agent_terminated' }
  } catch {
    return { status: 'failed', code: 'agent_cleanup_failed' }
  } finally {
    session?.close()
  }
}

async function expectedAgentIdentityV1(command: string | null): Promise<Readonly<{
  bundleURL: string
  executableURL: string
}> | null> {
  if (!command || !resolve(command).endsWith(`/Contents/MacOS/${AGENT_EXECUTABLE_NAME}`)) return null
  try {
    const executableURL = await realpath(resolve(command))
    if (executableURL !== resolve(command)) return null
    const bundleURL = dirname(dirname(dirname(executableURL)))
    if (!bundleURL.endsWith('/Analytix Computer Use.app')) return null
    if (await realpath(bundleURL) !== bundleURL) return null
    return { bundleURL, executableURL }
  } catch {
    return null
  }
}

function parseAgentInfoV1(value: unknown): AgentInfoV1 | null {
  const info = objectOf(value)
  if (
    info.bundleIdentifier !== AGENT_BUNDLE_ID ||
    typeof info.bundleURL !== 'string' || !info.bundleURL.startsWith('/') || resolve(info.bundleURL) !== info.bundleURL ||
    typeof info.executableURL !== 'string' || !info.executableURL.startsWith('/') || resolve(info.executableURL) !== info.executableURL ||
    typeof info.processStartTime !== 'number' || !Number.isFinite(info.processStartTime) || info.processStartTime <= 0
  ) return null
  return {
    bundleIdentifier: info.bundleIdentifier,
    bundleURL: info.bundleURL,
    executableURL: info.executableURL,
    processStartTime: info.processStartTime
  }
}

function sameAgentIdentityV1(
  info: AgentInfoV1,
  expected: Readonly<{ bundleURL: string; executableURL: string }>
): boolean {
  return info.bundleURL === expected.bundleURL && info.executableURL === expected.executableURL
}

async function inspectPrivateSocketV1(socketPath: string): Promise<AgentSocketInspectionV1> {
  try {
    const stat = await lstat(socketPath, { bigint: true })
    const currentUid = typeof process.getuid === 'function' ? BigInt(process.getuid()) : -1n
    if (!stat.isSocket() || stat.isSymbolicLink() || stat.uid !== currentUid || (stat.mode & 0o077n) !== 0n) {
      return { state: 'unsafe' }
    }
    return { state: 'present', identity: { dev: stat.dev, ino: stat.ino } }
  } catch (error) {
    return { state: isNodeErrorCode(error, 'ENOENT') ? 'absent' : 'unsafe' }
  }
}

async function openAgentSessionV1(socketPath: string): Promise<AgentSocketSessionV1> {
  return new Promise((resolveSession, rejectSession) => {
    const socket = createConnection({ path: socketPath })
    let opened = false
    let closed = false
    let response = Buffer.alloc(0)
    let pending: Readonly<{
      resolve: (value: unknown) => void
      reject: (error: Error) => void
      timer: NodeJS.Timeout
    }> | null = null
    const openTimer = setTimeout(() => fail(new Error('agent connection timeout')), REQUEST_TIMEOUT_MS)
    const clearResponse = (): void => {
      response.fill(0)
      response = Buffer.alloc(0)
    }
    const close = (): void => {
      if (closed) return
      closed = true
      socket.destroy()
      clearResponse()
    }
    const fail = (error: Error): void => {
      if (closed) return
      closed = true
      clearTimeout(openTimer)
      const active = pending
      pending = null
      if (active) {
        clearTimeout(active.timer)
        active.reject(error)
      } else if (!opened) {
        rejectSession(error)
      }
      socket.destroy()
      clearResponse()
    }
    const request = (payload: Readonly<Record<string, unknown>>): Promise<unknown> => {
      if (closed) return Promise.reject(new Error('agent connection closed'))
      if (pending) return Promise.reject(new Error('agent request already pending'))
      let encoded: string
      try {
        encoded = `${JSON.stringify(payload)}\n`
      } catch {
        return Promise.reject(new Error('agent request invalid'))
      }
      return new Promise((resolveRequest, rejectRequest) => {
        const timer = setTimeout(() => fail(new Error('agent request timeout')), REQUEST_TIMEOUT_MS)
        pending = { resolve: resolveRequest, reject: rejectRequest, timer }
        socket.write(encoded, (error) => {
          if (error) fail(new Error('agent request failed'))
        })
      })
    }
    socket.once('connect', () => {
      if (closed) return
      opened = true
      clearTimeout(openTimer)
      resolveSession({ request, close })
    })
    socket.once('error', () => fail(new Error('agent request failed')))
    socket.once('end', () => fail(new Error('agent connection ended')))
    socket.once('close', () => fail(new Error('agent connection closed')))
    socket.on('data', (chunk: Buffer) => {
      if (!pending) {
        fail(new Error('agent response without request'))
        return
      }
      const newline = chunk.indexOf(0x0a)
      const piece = newline >= 0 ? chunk.subarray(0, newline) : chunk
      if (response.length + piece.length > MAX_RESPONSE_BYTES) {
        fail(new Error('agent response too large'))
        return
      }
      const next = Buffer.concat([response, piece])
      response.fill(0)
      response = next
      if (newline < 0) return
      if (newline !== chunk.length - 1) {
        fail(new Error('agent response framing invalid'))
        return
      }
      const active = pending
      pending = null
      clearTimeout(active.timer)
      try {
        const parsed = JSON.parse(response.toString('utf8')) as unknown
        clearResponse()
        active.resolve(parsed)
      } catch {
        clearResponse()
        active.reject(new Error('agent response invalid'))
      }
    })
  })
}

async function waitForSocketRemovalV1(socketPath: string, timeoutMs: number): Promise<boolean> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      await lstat(socketPath)
    } catch (error) {
      return isNodeErrorCode(error, 'ENOENT')
    }
    await new Promise((resolveWait) => setTimeout(resolveWait, 25))
  }
  return false
}

function isNodeErrorCode(error: unknown, code: string): boolean {
  return error instanceof Error && 'code' in error && error.code === code
}

function objectOf(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}
