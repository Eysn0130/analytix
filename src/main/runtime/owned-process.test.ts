import type { ChildProcess } from 'node:child_process'
import { EventEmitter } from 'node:events'
import { describe, expect, it, vi } from 'vitest'
import {
  ownSpawnedProcess,
  stopOwnedProcess,
  type OwnedProcessHandle
} from './owned-process'

type FakeChild = {
  process: ChildProcess
  emitExit: (code: number | null, signal: NodeJS.Signals | null) => void
  kill: ReturnType<typeof vi.fn>
}

function fakeChild(pid: number | null = 4242): FakeChild {
  const emitter = new EventEmitter()
  const kill = vi.fn(() => true)
  const process = Object.assign(emitter, {
    pid: pid ?? undefined,
    exitCode: null as number | null,
    signalCode: null as NodeJS.Signals | null,
    kill
  }) as unknown as ChildProcess
  return {
    process,
    emitExit: (code, signal) => {
      Object.assign(process, { exitCode: code, signalCode: signal })
      emitter.emit('exit', code, signal)
    },
    kill
  }
}

function noSuchProcess(): Error & { code: string } {
  return Object.assign(new Error('no such process'), { code: 'ESRCH' })
}

function unixOwnedProcess(options: {
  onSignal: (signal: NodeJS.Signals, child: FakeChild) => void
}): {
  child: FakeChild
  handle: OwnedProcessHandle
  signals: NodeJS.Signals[]
  setGroupExists: (exists: boolean) => void
} {
  const child = fakeChild()
  const signals: NodeJS.Signals[] = []
  let groupExists = true
  const handle = ownSpawnedProcess(child.process, {
    detached: true,
    platform: 'darwin',
    processKill: (_pid, signal) => {
      if (signal === undefined) throw new Error('signal is required')
      if (signal === 0) {
        if (!groupExists) throw noSuchProcess()
        return true
      }
      signals.push(signal)
      options.onSignal(signal, child)
      return true
    }
  })
  return {
    child,
    handle,
    signals,
    setGroupExists: (exists) => {
      groupExists = exists
    }
  }
}

describe('owned production process lifecycle', () => {
  it('waits for both normal child exit and detached group disappearance', async () => {
    let fixture!: ReturnType<typeof unixOwnedProcess>
    fixture = unixOwnedProcess({
      onSignal: (signal, child) => {
        if (signal !== 'SIGTERM') return
        fixture.setGroupExists(false)
        child.emitExit(0, null)
      }
    })

    await expect(stopOwnedProcess(fixture.handle, {
      graceMs: 50,
      killWaitMs: 50
    })).resolves.toEqual({
      pid: 4242,
      termination: 'sigterm',
      signals: ['SIGTERM'],
      exit: { code: 0, signal: null, spawnError: null }
    })
    expect(fixture.signals).toEqual(['SIGTERM'])
    expect(fixture.child.kill).not.toHaveBeenCalled()
  })

  it('escalates the exact detached group and confirms no descendant remains', async () => {
    let fixture!: ReturnType<typeof unixOwnedProcess>
    fixture = unixOwnedProcess({
      onSignal: (signal, child) => {
        if (signal !== 'SIGKILL') return
        fixture.setGroupExists(false)
        child.emitExit(null, 'SIGKILL')
      }
    })

    await expect(stopOwnedProcess(fixture.handle, {
      graceMs: 1,
      killWaitMs: 50
    })).resolves.toEqual({
      pid: 4242,
      termination: 'sigkill',
      signals: ['SIGTERM', 'SIGKILL'],
      exit: { code: null, signal: 'SIGKILL', spawnError: null }
    })
    expect(fixture.signals).toEqual(['SIGTERM', 'SIGKILL'])
    expect(fixture.child.kill).not.toHaveBeenCalled()
  })

  it('fails closed when SIGKILL does not prove child and group disappearance', async () => {
    const fixture = unixOwnedProcess({
      onSignal: () => undefined
    })

    await expect(stopOwnedProcess(fixture.handle, {
      graceMs: 1,
      killWaitMs: 1
    })).rejects.toThrow('did not fully exit after SIGKILL')
    expect(fixture.signals).toEqual(['SIGTERM', 'SIGKILL'])
    expect(fixture.child.kill).not.toHaveBeenCalled()
  })

  it('shares one stop result across concurrent and repeated callers', async () => {
    let fixture!: ReturnType<typeof unixOwnedProcess>
    fixture = unixOwnedProcess({
      onSignal: (signal, child) => {
        if (signal !== 'SIGTERM') return
        fixture.setGroupExists(false)
        child.emitExit(0, null)
      }
    })

    const first = stopOwnedProcess(fixture.handle)
    const second = stopOwnedProcess(fixture.handle, {
      graceMs: 1,
      killWaitMs: 1
    })
    expect(second).toBe(first)
    const [firstResult, secondResult] = await Promise.all([first, second])
    const thirdResult = await stopOwnedProcess(fixture.handle)
    expect(secondResult).toBe(firstResult)
    expect(thirdResult).toBe(firstResult)
    expect(fixture.signals).toEqual(['SIGTERM'])
  })

  it('never falls back to a mutable PID when spawn did not capture one', async () => {
    const child = fakeChild(null)
    const handle = ownSpawnedProcess(child.process, {
      detached: true,
      platform: 'darwin'
    })

    await expect(stopOwnedProcess(handle, {
      graceMs: 1,
      killWaitMs: 1
    })).rejects.toThrow('no captured PID')
    expect(child.kill).not.toHaveBeenCalled()
  })

  it('settles a no-PID spawn failure without signalling an unrelated process', async () => {
    const child = fakeChild(null)
    const handle = ownSpawnedProcess(child.process, {
      detached: true,
      platform: 'darwin'
    })
    const spawnError = new Error('spawn failed')
    child.process.emit('error', spawnError)

    const result = await stopOwnedProcess(handle)
    expect(result).toMatchObject({
      pid: null,
      termination: 'already_exited',
      signals: [],
      exit: {
        code: null,
        signal: null
      }
    })
    expect(result.exit.spawnError).toBe(spawnError)
    expect(child.kill).not.toHaveBeenCalled()
  })

  it('uses only the captured Windows process tree and escalates through the trusted terminator', async () => {
    const child = fakeChild(7171)
    const calls: Array<{ pid: number; force: boolean }> = []
    const handle = ownSpawnedProcess(child.process, {
      detached: false,
      platform: 'win32',
      windowsTreeTerminator: async (pid, force) => {
        calls.push({ pid, force })
        if (force) child.emitExit(1, null)
      }
    })

    await expect(stopOwnedProcess(handle, {
      graceMs: 1,
      killWaitMs: 50
    })).resolves.toEqual({
      pid: 7171,
      termination: 'windows_tree_forced',
      signals: ['WINDOWS_TREE', 'WINDOWS_TREE_FORCE'],
      exit: { code: 1, signal: null, spawnError: null }
    })
    expect(calls).toEqual([
      { pid: 7171, force: false },
      { pid: 7171, force: true }
    ])
    expect(child.kill).not.toHaveBeenCalled()
  })

  it('does not target a reused Windows PID after the captured child has exited', async () => {
    const child = fakeChild(8181)
    const treeTerminator = vi.fn(async () => undefined)
    const handle = ownSpawnedProcess(child.process, {
      detached: false,
      platform: 'win32',
      windowsTreeTerminator: treeTerminator
    })
    child.emitExit(0, null)

    await expect(stopOwnedProcess(handle)).resolves.toEqual({
      pid: 8181,
      termination: 'already_exited',
      signals: [],
      exit: { code: 0, signal: null, spawnError: null }
    })
    expect(treeTerminator).not.toHaveBeenCalled()
    expect(child.kill).not.toHaveBeenCalled()
  })
})
