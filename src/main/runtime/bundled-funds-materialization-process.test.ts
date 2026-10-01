import { EventEmitter } from 'node:events'
import { PassThrough } from 'node:stream'
import type { ChildProcess } from 'node:child_process'
import { afterEach, expect, it, vi } from 'vitest'

const spawnMock = vi.hoisted(() => vi.fn())
vi.mock('node:child_process', async (importOriginal) => ({
  ...await importOriginal<typeof import('node:child_process')>(),
  spawn: spawnMock
}))

import {
  BundledFundsMaterializationChildUnconfirmedError,
  materializeBundledFundsBeforeRuntimeV1
} from './bundled-funds-materialization'

afterEach(() => {
  vi.useRealTimers()
  spawnMock.mockReset()
})

it('blocks another materialization until the exact unconfirmed child exits', async () => {
  vi.useFakeTimers()
  const child = Object.assign(new EventEmitter(), {
    pid: 54321,
    exitCode: null as number | null,
    signalCode: null as NodeJS.Signals | null,
    stdout: new PassThrough(),
    stderr: new PassThrough(),
    kill: vi.fn(() => true)
  })
  spawnMock.mockReturnValue(child as unknown as ChildProcess)
  const options = {
    appIsPackaged: true,
    launchTarget: { command: '/tmp/runtime-server', argsPrefix: [], mode: 'bundled-binary' as const },
    dataDir: '/tmp/analytix-startup-owner-trace/data'
  }

  const first = materializeBundledFundsBeforeRuntimeV1(options).catch((error: unknown) => error)
  child.stderr.write(Buffer.alloc(600000, 0x78))
  await vi.advanceTimersByTimeAsync(3000)
  expect(await first).toBeInstanceOf(BundledFundsMaterializationChildUnconfirmedError)
  expect(child.kill).toHaveBeenCalledTimes(2)

  const runner = vi.fn(async () => ({
    exitCode: 1, signal: null, stdout: Buffer.alloc(0), stderr: Buffer.alloc(0),
    timedOut: false, overflow: false
  }))
  await expect(materializeBundledFundsBeforeRuntimeV1({ ...options, runner }))
    .rejects.toBeInstanceOf(BundledFundsMaterializationChildUnconfirmedError)
  expect(runner).not.toHaveBeenCalled()

  child.signalCode = 'SIGKILL'
  child.emit('exit', null, 'SIGKILL')
  await expect(materializeBundledFundsBeforeRuntimeV1({ ...options, runner }))
    .rejects.toThrow(/command failed/)
  expect(runner).toHaveBeenCalledTimes(1)
  child.stdout?.destroy()
  child.stderr?.destroy()
})
