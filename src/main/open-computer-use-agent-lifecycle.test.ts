import { mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { createServer, type Server } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { stopOwnedOpenComputerUseAppAgentV1 } from './open-computer-use-agent-lifecycle'

const roots: string[] = []
const servers: Server[] = []
const cleanBaseline = { state: 'absent', observedAtMs: 198_000 } as const
const presentSocket = (dev = 1n, ino = 2n) => ({
  state: 'present',
  identity: { dev, ino }
} as const)

afterEach(async () => {
  vi.restoreAllMocks()
  await Promise.all(servers.splice(0).map((server) => new Promise<void>((resolveClose) => {
    if (!server.listening) {
      resolveClose()
      return
    }
    server.close(() => resolveClose())
  })))
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true })
})

function commandFixture(): { command: string; bundleURL: string } {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'analytix-computer-use-agent-')))
  roots.push(root)
  const bundleURL = join(root, 'Analytix Computer Use.app')
  const command = join(bundleURL, 'Contents', 'MacOS', 'OpenComputerUse')
  mkdirSync(join(command, '..'), { recursive: true })
  writeFileSync(command, 'fixture', { mode: 0o700 })
  return { command, bundleURL }
}

function currentInfo(command: string, bundleURL: string, processStartTime = 200): Record<string, unknown> {
  return {
    bundleIdentifier: 'com.analytix.computer-use',
    bundleURL,
    executableURL: command,
    processStartTime
  }
}

describe('Open Computer Use app-agent lifecycle', () => {
  it('terminates only an exact agent started by the current app', async () => {
    const { command, bundleURL } = commandFixture()
    const request = vi.fn(async (payload: Readonly<Record<string, unknown>>) => (
      payload.kind === 'agentInfo' ? currentInfo(command, bundleURL) : { ok: true }
    ))
    const result = await stopOwnedOpenComputerUseAppAgentV1({
      command,
      appStartedAtMs: 199_000,
      baseline: cleanBaseline,
      platform: 'darwin',
      dependencies: {
        inspectSocket: async () => presentSocket(),
        openSession: async () => ({ request, close: vi.fn() }),
        waitForSocketRemoval: async () => true
      }
    })
    expect(result).toEqual({ status: 'terminated', code: 'agent_terminated' })
    expect(request).toHaveBeenNthCalledWith(1, { kind: 'agentInfo' })
    expect(request).toHaveBeenNthCalledWith(2, { kind: 'terminate' })
  })

  it('does not terminate a pre-existing or foreign agent', async () => {
    const { command, bundleURL } = commandFixture()
    for (const info of [
      currentInfo(command, bundleURL, 100),
      { ...currentInfo(command, bundleURL), bundleIdentifier: 'com.example.forged' },
      { ...currentInfo(command, bundleURL), executableURL: join(bundleURL, 'forged') },
      currentInfo(command, bundleURL, Date.now() / 1_000 + 60)
    ]) {
      const request = vi.fn(async () => info)
      const result = await stopOwnedOpenComputerUseAppAgentV1({
        command,
        appStartedAtMs: 199_000,
        baseline: cleanBaseline,
        platform: 'darwin',
        dependencies: {
          inspectSocket: async () => presentSocket(),
          openSession: async () => ({ request, close: vi.fn() }),
          waitForSocketRemoval: async () => true
        }
      })
      expect(result.status).toBe('not-owned')
      expect(request).toHaveBeenCalledTimes(1)
    }
  })

  it('fails closed for missing, unsafe, or replaced socket identity', async () => {
    const { command, bundleURL } = commandFixture()
    const missing = await stopOwnedOpenComputerUseAppAgentV1({
      command,
      appStartedAtMs: 199_000,
      baseline: cleanBaseline,
      platform: 'darwin',
      dependencies: {
        inspectSocket: async () => ({ state: 'absent' }),
        openSession: async () => ({
          request: async () => currentInfo(command, bundleURL),
          close: vi.fn()
        }),
        waitForSocketRemoval: async () => true
      }
    })
    expect(missing).toEqual({ status: 'not-present', code: 'agent_socket_absent' })

    const unsafe = await stopOwnedOpenComputerUseAppAgentV1({
      command,
      appStartedAtMs: 199_000,
      baseline: cleanBaseline,
      platform: 'darwin',
      dependencies: {
        inspectSocket: async () => ({ state: 'unsafe' }),
        openSession: async () => ({
          request: async () => currentInfo(command, bundleURL),
          close: vi.fn()
        }),
        waitForSocketRemoval: async () => true
      }
    })
    expect(unsafe).toEqual({ status: 'failed', code: 'agent_socket_untrusted' })

    let inspection = 0
    const replaced = await stopOwnedOpenComputerUseAppAgentV1({
      command,
      appStartedAtMs: 199_000,
      baseline: cleanBaseline,
      platform: 'darwin',
      dependencies: {
        inspectSocket: async () => (++inspection === 1 ? presentSocket() : presentSocket(1n, 3n)),
        openSession: async () => ({
          request: async () => currentInfo(command, bundleURL),
          close: vi.fn()
        }),
        waitForSocketRemoval: async () => true
      }
    })
    expect(replaced).toEqual({ status: 'failed', code: 'agent_socket_identity_changed' })
  })

  it('requires an observed socket removal before reporting termination', async () => {
    const { command, bundleURL } = commandFixture()
    const result = await stopOwnedOpenComputerUseAppAgentV1({
      command,
      appStartedAtMs: 199_000,
      baseline: cleanBaseline,
      platform: 'darwin',
      dependencies: {
        inspectSocket: async () => presentSocket(),
        openSession: async () => ({
          request: async (payload) => payload.kind === 'agentInfo'
            ? currentInfo(command, bundleURL)
            : { ok: true },
          close: vi.fn()
        }),
        waitForSocketRemoval: async () => false
      }
    })
    expect(result).toEqual({ status: 'failed', code: 'agent_termination_not_observed' })
  })

  it('refuses to terminate when a clean startup baseline was not observed', async () => {
    const { command } = commandFixture()
    const result = await stopOwnedOpenComputerUseAppAgentV1({
      command,
      appStartedAtMs: 199_000,
      baseline: { state: 'present', observedAtMs: 198_000 },
      platform: 'darwin'
    })
    expect(result).toEqual({ status: 'not-owned', code: 'agent_clean_start_not_observed' })
  })

  it('uses one socket connection for identity validation and termination', async () => {
    const { command, bundleURL } = commandFixture()
    const socketPath = join(roots.at(-1)!, 'agent.sock')
    let connectionCount = 0
    const server = createServer((socket) => {
      connectionCount += 1
      let pending = ''
      socket.on('data', (chunk) => {
        pending += chunk.toString('utf8')
        while (pending.includes('\n')) {
          const newline = pending.indexOf('\n')
          const request = JSON.parse(pending.slice(0, newline)) as { kind?: string }
          pending = pending.slice(newline + 1)
          socket.write(`${JSON.stringify(request.kind === 'agentInfo'
            ? currentInfo(command, bundleURL)
            : { ok: true })}\n`)
        }
      })
    })
    servers.push(server)
    await new Promise<void>((resolveListen, rejectListen) => {
      server.once('error', rejectListen)
      server.listen(socketPath, resolveListen)
    })

    const result = await stopOwnedOpenComputerUseAppAgentV1({
      command,
      appStartedAtMs: 199_000,
      baseline: cleanBaseline,
      platform: 'darwin',
      dependencies: {
        socketPath,
        inspectSocket: async () => presentSocket(),
        waitForSocketRemoval: async () => true
      }
    })
    expect(result).toEqual({ status: 'terminated', code: 'agent_terminated' })
    expect(connectionCount).toBe(1)
  })

  it('fails promptly when the peer closes without a complete response', async () => {
    const { command } = commandFixture()
    const socketPath = join(roots.at(-1)!, 'agent-close.sock')
    const server = createServer((socket) => {
      socket.once('data', () => socket.end())
    })
    servers.push(server)
    await new Promise<void>((resolveListen, rejectListen) => {
      server.once('error', rejectListen)
      server.listen(socketPath, resolveListen)
    })

    const result = await stopOwnedOpenComputerUseAppAgentV1({
      command,
      appStartedAtMs: 199_000,
      baseline: cleanBaseline,
      platform: 'darwin',
      dependencies: {
        socketPath,
        inspectSocket: async () => presentSocket(),
        waitForSocketRemoval: async () => true
      }
    })
    expect(result).toEqual({ status: 'failed', code: 'agent_cleanup_failed' })
  })
})
