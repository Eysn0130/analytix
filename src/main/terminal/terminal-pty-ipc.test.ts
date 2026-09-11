import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync
} from 'node:fs'
import { dirname, join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  killTerminalPtyIfRunning,
  packagedDarwinNodePtyEntryPath,
  registerTerminalPtyIpc
} from './terminal-pty-ipc'
import { TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE } from './terminal-process-policy'

const taskRoots: string[] = []

function taskRoot(): string {
  const trustedTemporaryRoot = realpathSync(String(process.env.TMPDIR || ''))
  const root = mkdtempSync(join(trustedTemporaryRoot, 'analytix-terminal-ipc-'))
  taskRoots.push(root)
  return root
}

afterEach(() => {
  while (taskRoots.length > 0) {
    const root = taskRoots.pop()
    if (root) rmSync(root, { recursive: true, force: true })
  }
})

describe('packaged Darwin node-pty entry', () => {
  it('resolves the relocated module beside the packaged executable', () => {
    const executablePath = join(
      '/Applications',
      'analytix.app',
      'Contents',
      'MacOS',
      'analytix'
    )
    const pathExists = vi.fn(() => true)

    expect(packagedDarwinNodePtyEntryPath(
      'darwin',
      executablePath,
      pathExists
    )).toBe(join(
      '/Applications',
      'analytix.app',
      'Contents',
      'MacOS',
      'analytix-node-pty',
      'lib',
      'index.js'
    ))
    expect(pathExists).toHaveBeenCalledOnce()
  })

  it('falls back to the ordinary dependency when the relocated entry is absent', () => {
    expect(packagedDarwinNodePtyEntryPath(
      'darwin',
      '/Applications/analytix.app/Contents/MacOS/analytix',
      () => false
    )).toBe('')
  })

  it('does not use the Darwin relocation on other platforms', () => {
    const pathExists = vi.fn(() => true)
    expect(packagedDarwinNodePtyEntryPath(
      'win32',
      'C:\\analytix\\analytix.exe',
      pathExists
    )).toBe('')
    expect(pathExists).not.toHaveBeenCalled()
  })
})

describe('terminal PTY disposal', () => {
  it('kills a running PTY before disposal', () => {
    const kill = vi.fn()

    killTerminalPtyIfRunning({ kill }, false)

    expect(kill).toHaveBeenCalledOnce()
  })

  it('does not signal a PID retained after natural PTY exit', () => {
    const kill = vi.fn()

    killTerminalPtyIfRunning({ kill }, true)

    expect(kill).not.toHaveBeenCalled()
  })
})

type FakeSender = ReturnType<typeof fakeSender>

function fakeSender(id: number) {
  const listeners = new Map<string, Array<(...args: any[]) => void>>()
  const mainFrame = { url: 'analytix-app://renderer/index.html' }
  return {
    id,
    mainFrame,
    send: vi.fn(),
    isDestroyed: vi.fn(() => false),
    once: vi.fn((name: string, listener: (...args: any[]) => void) => {
      listeners.set(name, [...(listeners.get(name) ?? []), listener])
    }),
    on: vi.fn((name: string, listener: (...args: any[]) => void) => {
      listeners.set(name, [...(listeners.get(name) ?? []), listener])
    }),
    emit(name: string, ...args: any[]) {
      for (const listener of listeners.get(name) ?? []) listener(...args)
    }
  }
}

function eventFor(sender: FakeSender) {
  return { sender, senderFrame: sender.mainFrame }
}

function fakePty() {
  let dataListener: ((data: string) => void) | undefined
  let exitListener: ((event: { exitCode: number }) => void) | undefined
  return {
    kill: vi.fn(),
    write: vi.fn(),
    resize: vi.fn(),
    onData: vi.fn((listener: (data: string) => void) => {
      dataListener = listener
      return { dispose: vi.fn() }
    }),
    onExit: vi.fn((listener: (event: { exitCode: number }) => void) => {
      exitListener = listener
      return { dispose: vi.fn() }
    }),
    emitData(data: string) {
      dataListener?.(data)
    },
    emitExit(exitCode: number) {
      exitListener?.({ exitCode })
    }
  }
}

function terminalHarness(options: { deferPtyLoad?: boolean } = {}) {
  const handlers = new Map<string, (event: any, args: any) => Promise<any>>()
  const ipcMain = {
    handle: vi.fn((channel: string, handler: (event: any, args: any) => Promise<any>) => {
      handlers.set(channel, handler)
    })
  }
  const sender = fakeSender(1)
  const otherSender = fakeSender(2)
  const trusted = new Set([sender])
  const ptys: ReturnType<typeof fakePty>[] = []
  const spawn = vi.fn((_file: string, _args: string[], _options: Record<string, any>) => {
    const pty = fakePty()
    ptys.push(pty)
    return pty
  })
  const ptyModule = { spawn } as never
  let releasePtyLoad: (() => void) | undefined
  const loadPtyModule = options.deferPtyLoad
    ? vi.fn(() => new Promise<typeof import('node-pty') | null>((resolve) => {
        releasePtyLoad = () => resolve(ptyModule)
      }))
    : vi.fn(async () => ptyModule)
  const root = taskRoot()
  const protectedRoot = join(root, 'protected')
  const ordinaryRoot = join(root, 'ordinary')
  mkdirSync(protectedRoot, { mode: 0o700 })
  mkdirSync(ordinaryRoot, { mode: 0o700 })
  const getProtectedRoots = vi.fn(async () => [protectedRoot])
  const logError = vi.fn()
  let beforeQuit: (() => void) | undefined
  const controller = registerTerminalPtyIpc({
    ipcMain: ipcMain as never,
    getMainWindow: () => null,
    getProtectedRoots,
    isTrustedSender: (event) => trusted.has(event.sender as unknown as FakeSender),
    loadPtyModule,
    registerBeforeQuit: (listener) => {
      beforeQuit = listener
    },
    platform: 'darwin',
    env: {
      HOME: process.env.HOME,
      PATH: process.env.PATH,
      ANALYTIX_RUNTIME_TOKEN: 'ambient-authority-sentinel'
    },
    logError
  })
  return {
    handlers,
    sender,
    otherSender,
    trusted,
    spawn,
    ptys,
    protectedRoot,
    ordinaryRoot,
    getProtectedRoots,
    logError,
    controller,
    beforeQuit: () => beforeQuit?.(),
    releasePtyLoad: () => releasePtyLoad?.()
  }
}

describe('terminal PTY IPC authority', () => {
  it('revokes old PTYs before a runtime-root settings patch can persist', () => {
    const source = readFileSync(join(process.cwd(), 'src', 'main', 'index.ts'), 'utf8')
    const applyStart = source.indexOf(
      'const applySettingsPatch = async (partial: AppSettingsPatch)'
    )
    const applyEnd = source.indexOf(
      'const hubAccountService = createHubAccountService',
      applyStart
    )
    const applySource = source.slice(applyStart, applyEnd)
    const revoke = applySource.indexOf(
      'terminalPtyController?.suspendCreates()'
    )
    const persist = applySource.indexOf('saved = await store.patch(partial)')
    const handover = applySource.indexOf(
      'const runtimeApply = queueRuntimeSettingsApply(prev, saved)'
    )

    expect(applyStart).toBeGreaterThanOrEqual(0)
    expect(revoke).toBeGreaterThanOrEqual(0)
    expect(persist).toBeGreaterThan(revoke)
    expect(handover).toBeGreaterThan(persist)
    expect(applySource).toContain('void runtimeApply.then(release, release)')
    expect(applySource.slice(0, revoke)).toContain(
      'retainTerminalRuntimeProtectedRoots(prev)'
    )
    expect(applySource.slice(0, revoke)).toContain(
      'retainTerminalRuntimeProtectedRoots(next)'
    )
    expect(source).toContain(
      'function queueRuntimeSettingsApply(\n' +
      '  prev: AppSettingsV1,\n' +
      '  next: AppSettingsV1\n' +
      '): Promise<void> | null'
    )
    expect(source).toContain(
      'function handleUnexpectedAnalytixExit(info: AnalytixUnexpectedExitInfo): void {\n' +
      '  terminalPtyController?.disposeAll()'
    )
    expect(source).toContain(
      'async function stopManagedRuntimes(): Promise<void> {'
    )
    expect(source).toContain(
      'async function restartManagedRuntimeForMcpConfigChange(settings: AppSettingsV1)'
    )
    expect(source).toContain('isTrustedSender: isTrustedDesktopRenderer')
    expect(source).toContain(
      'const saveSettingsPatch = async (partial: AppSettingsPatch): Promise<AppSettingsV1> => {\n' +
      '    if (!silentSettingsPatchCanPersistWithoutRuntimeApply(partial)) {\n' +
      '      return applySettingsPatch(partial)\n' +
      '    }\n' +
      '    return store.patch(partial)\n' +
      '  }'
    )
    expect(source).toContain(
      "Object.keys(partial).some((key) => key !== 'runtime' && key !== 'write')"
    )
    expect(source).toContain("key !== 'modelCapabilityProbes'")
    expect(source).toContain(
      "Object.keys(partial.write ?? {}).some((key) => key !== 'typography')"
    )
    expect(source).toContain('retainTerminalRuntimeProtectedRoots(initial)')
    expect(source).not.toContain('saveSettingsPatch: applySettingsPatch')
    expect(source).toContain('...defaultDarwinTerminalProtectedRoots(desktopStateHomeRoot)')
    expect(source).toContain('...retainedTerminalRuntimeProtectedRoots')
    expect(source).toContain("join(dataDir, 'private')")
    expect(source).toContain("join(dataDir, 'child-runs')")
    expect(source).not.toContain(
      'isTrustedSender: (event: IpcMainInvokeEvent) => Boolean(\n' +
      '      dataAnalysisBackendManager'
    )
  })

  it('binds create, replay, write, resize, and dispose to one trusted main frame', async () => {
    if (process.platform !== 'darwin') return
    const harness = terminalHarness()
    const create = harness.handlers.get('terminal:create')!
    const write = harness.handlers.get('terminal:write')!
    const resize = harness.handlers.get('terminal:resize')!
    const dispose = harness.handlers.get('terminal:dispose')!
    const ownerEvent = eventFor(harness.sender)
    const otherEvent = eventFor(harness.otherSender)

    await expect(create(ownerEvent, {
      sessionId: 'main',
      cwd: harness.ordinaryRoot,
      cols: 90,
      rows: 30
    })).resolves.toEqual({ ok: true, sessionId: 'main' })
    expect(harness.spawn).toHaveBeenCalledOnce()
    const [file, args, options] = harness.spawn.mock.calls[0]!
    expect(file).toBe('/usr/bin/sandbox-exec')
    expect(args[0]).toBe('-p')
    expect(args[1]).toContain(harness.protectedRoot)
    expect(options).toEqual(expect.objectContaining({
      cwd: harness.ordinaryRoot,
      cols: 90,
      rows: 30
    }))
    expect(options.env.ANALYTIX_RUNTIME_TOKEN).toBeUndefined()
    expect(options.env.HISTFILE).toBe('/dev/null')
    expect(args.slice(-2)).toEqual(['/bin/zsh', '-f'])

    harness.ptys[0]!.emitData('owner-only-output')
    expect(harness.sender.send).toHaveBeenCalledWith('terminal:data', {
      sessionId: 'main',
      data: 'owner-only-output'
    })
    harness.sender.send.mockClear()

    await expect(create(otherEvent, {
      sessionId: 'main',
      cwd: harness.ordinaryRoot
    })).resolves.toEqual({
      ok: false,
      message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
    })
    expect(harness.otherSender.send).not.toHaveBeenCalled()
    await expect(write(otherEvent, {
      sessionId: 'main',
      data: 'unauthorized'
    })).resolves.toBe(false)
    await expect(resize(otherEvent, {
      sessionId: 'main',
      cols: 100,
      rows: 40
    })).resolves.toBe(false)
    await expect(dispose(otherEvent, 'main')).resolves.toBe(false)
    expect(harness.ptys[0]!.write).not.toHaveBeenCalled()
    expect(harness.ptys[0]!.resize).not.toHaveBeenCalled()
    expect(harness.ptys[0]!.kill).not.toHaveBeenCalled()

    await expect(create(ownerEvent, {
      sessionId: 'main',
      cwd: harness.ordinaryRoot
    })).resolves.toEqual({ ok: true, sessionId: 'main', replayed: true })
    expect(harness.sender.send).toHaveBeenCalledWith('terminal:data', {
      sessionId: 'main',
      data: 'owner-only-output'
    })
    await expect(write(ownerEvent, {
      sessionId: 'main',
      data: 'echo ordinary\n'
    })).resolves.toBe(true)
    await expect(resize(ownerEvent, {
      sessionId: 'main',
      cols: 100,
      rows: 40
    })).resolves.toBe(true)
    expect(harness.ptys[0]!.write).toHaveBeenCalledWith('echo ordinary\n')
    expect(harness.ptys[0]!.resize).toHaveBeenCalledWith(100, 40)
    await expect(dispose(ownerEvent, 'main')).resolves.toBe(true)
    expect(harness.ptys[0]!.kill).toHaveBeenCalledOnce()
  })

  it('rejects untrusted and stale frames without starting or revealing a session', async () => {
    if (process.platform !== 'darwin') return
    const harness = terminalHarness()
    const create = harness.handlers.get('terminal:create')!
    harness.trusted.delete(harness.sender)

    await expect(create(eventFor(harness.sender), {
      sessionId: 'main',
      cwd: harness.ordinaryRoot
    })).resolves.toEqual({
      ok: false,
      message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
    })
    await expect(create({
      sender: harness.sender,
      senderFrame: { url: 'analytix-app://renderer/index.html' }
    }, {
      sessionId: 'main',
      cwd: harness.ordinaryRoot
    })).resolves.toEqual({
      ok: false,
      message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
    })
    expect(harness.spawn).not.toHaveBeenCalled()
  })

  it('isolates the same session name across two trusted desktop windows', async () => {
    if (process.platform !== 'darwin') return
    const harness = terminalHarness()
    const create = harness.handlers.get('terminal:create')!
    harness.trusted.add(harness.otherSender)

    await expect(create(eventFor(harness.sender), {
      sessionId: 'main',
      cwd: harness.ordinaryRoot
    })).resolves.toEqual({ ok: true, sessionId: 'main' })
    await expect(create(eventFor(harness.otherSender), {
      sessionId: 'main',
      cwd: harness.ordinaryRoot
    })).resolves.toEqual({ ok: true, sessionId: 'main' })
    expect(harness.spawn).toHaveBeenCalledTimes(2)

    harness.ptys[0]!.emitData('first-window-output')
    harness.ptys[1]!.emitData('second-window-output')
    expect(harness.sender.send).toHaveBeenCalledWith('terminal:data', {
      sessionId: 'main',
      data: 'first-window-output'
    })
    expect(harness.sender.send).not.toHaveBeenCalledWith('terminal:data', {
      sessionId: 'main',
      data: 'second-window-output'
    })
    expect(harness.otherSender.send).toHaveBeenCalledWith('terminal:data', {
      sessionId: 'main',
      data: 'second-window-output'
    })
    expect(harness.otherSender.send).not.toHaveBeenCalledWith('terminal:data', {
      sessionId: 'main',
      data: 'first-window-output'
    })
  })

  it('reserves an async create and rejects a concurrent duplicate', async () => {
    if (process.platform !== 'darwin') return
    const harness = terminalHarness({ deferPtyLoad: true })
    const create = harness.handlers.get('terminal:create')!
    const event = eventFor(harness.sender)
    const first = create(event, {
      sessionId: 'main',
      cwd: harness.ordinaryRoot
    })
    await expect(create(event, {
      sessionId: 'main',
      cwd: harness.ordinaryRoot
    })).resolves.toEqual({
      ok: false,
      message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
    })
    harness.releasePtyLoad()
    await expect(first).resolves.toEqual({ ok: true, sessionId: 'main' })
    expect(harness.spawn).toHaveBeenCalledOnce()
  })

  it('prevents a pending create from spawning after navigation or invalidation', async () => {
    if (process.platform !== 'darwin') return
    const navigationHarness = terminalHarness({ deferPtyLoad: true })
    const navigationCreate = navigationHarness.handlers.get('terminal:create')!
    const navigationPending = navigationCreate(eventFor(navigationHarness.sender), {
      sessionId: 'main',
      cwd: navigationHarness.ordinaryRoot
    })
    navigationHarness.sender.emit('did-start-navigation', {
      isMainFrame: true,
      isSameDocument: false
    })
    navigationHarness.releasePtyLoad()
    await expect(navigationPending).resolves.toEqual({
      ok: false,
      message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
    })
    expect(navigationHarness.spawn).not.toHaveBeenCalled()

    const invalidationHarness = terminalHarness({ deferPtyLoad: true })
    const invalidationCreate = invalidationHarness.handlers.get('terminal:create')!
    const invalidationPending = invalidationCreate(eventFor(invalidationHarness.sender), {
      sessionId: 'main',
      cwd: invalidationHarness.ordinaryRoot
    })
    invalidationHarness.controller.disposeAll()
    invalidationHarness.releasePtyLoad()
    await expect(invalidationPending).resolves.toEqual({
      ok: false,
      message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
    })
    expect(invalidationHarness.spawn).not.toHaveBeenCalled()
  })

  it('suspends creates across an atomic protected-root settings transition', async () => {
    if (process.platform !== 'darwin') return
    const harness = terminalHarness()
    const create = harness.handlers.get('terminal:create')!
    const event = eventFor(harness.sender)
    await create(event, {
      sessionId: 'before-transition',
      cwd: harness.ordinaryRoot
    })
    const previousPty = harness.ptys[0]!
    let releaseTransition: (() => void) | undefined
    const transitionGate = new Promise<void>((resolve) => {
      releaseTransition = resolve
    })
    const transition = harness.controller.withCreatesSuspended(async () => {
      expect(previousPty.kill).toHaveBeenCalledOnce()
      await transitionGate
      return 'persisted'
    })

    await expect(create(event, {
      sessionId: 'during-transition',
      cwd: harness.ordinaryRoot
    })).resolves.toEqual({
      ok: false,
      message: TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
    })
    expect(harness.spawn).toHaveBeenCalledOnce()
    releaseTransition?.()
    await expect(transition).resolves.toBe('persisted')
    await expect(create(event, {
      sessionId: 'after-transition',
      cwd: harness.ordinaryRoot
    })).resolves.toEqual({ ok: true, sessionId: 'after-transition' })
    expect(harness.spawn).toHaveBeenCalledTimes(2)
  })

  it('kills sessions before navigation, runtime invalidation, and normal quit', async () => {
    if (process.platform !== 'darwin') return
    const harness = terminalHarness()
    const create = harness.handlers.get('terminal:create')!
    await create(eventFor(harness.sender), {
      sessionId: 'first',
      cwd: harness.ordinaryRoot
    })
    const first = harness.ptys[0]!
    harness.sender.emit('did-start-navigation', {
      isMainFrame: true,
      isSameDocument: false
    })
    expect(first.kill).toHaveBeenCalledOnce()
    first.emitData('stale-output')
    expect(harness.sender.send).not.toHaveBeenCalledWith('terminal:data', {
      sessionId: 'first',
      data: 'stale-output'
    })

    await create(eventFor(harness.sender), {
      sessionId: 'second',
      cwd: harness.ordinaryRoot
    })
    harness.controller.disposeAll()
    harness.controller.disposeAll()
    expect(harness.ptys[1]!.kill).toHaveBeenCalledOnce()
    expect(harness.sender.send).toHaveBeenCalledWith('terminal:exit', {
      sessionId: 'second',
      exitCode: null
    })

    await create(eventFor(harness.sender), {
      sessionId: 'third',
      cwd: harness.ordinaryRoot
    })
    harness.beforeQuit()
    expect(harness.ptys[2]!.kill).toHaveBeenCalledOnce()
  })

  it('resolves a fresh protected-root set for every new PTY', async () => {
    if (process.platform !== 'darwin') return
    const harness = terminalHarness()
    const create = harness.handlers.get('terminal:create')!
    const dispose = harness.handlers.get('terminal:dispose')!
    const event = eventFor(harness.sender)
    const nextRoot = join(dirname(harness.protectedRoot), 'next-protected')
    mkdirSync(nextRoot, { mode: 0o700 })

    await create(event, { sessionId: 'first', cwd: harness.ordinaryRoot })
    await dispose(event, 'first')
    harness.getProtectedRoots.mockResolvedValueOnce([nextRoot])
    await create(event, { sessionId: 'second', cwd: harness.ordinaryRoot })

    expect(harness.getProtectedRoots).toHaveBeenCalledTimes(2)
    expect(harness.spawn.mock.calls[0]![1][1]).toContain(harness.protectedRoot)
    expect(harness.spawn.mock.calls[1]![1][1]).toContain(nextRoot)
    expect(harness.spawn.mock.calls[1]![1][1]).not.toContain(harness.protectedRoot)
  })

  it('keeps renderer identifiers and native error text out of logs', async () => {
    if (process.platform !== 'darwin') return
    const harness = terminalHarness()
    const create = harness.handlers.get('terminal:create')!
    const write = harness.handlers.get('terminal:write')!
    const rendererIdentifier = 'account-6222021234567890123'
    const nativeError = 'native-error-private-path-/private/runtime/evidence'
    await create(eventFor(harness.sender), {
      sessionId: rendererIdentifier,
      cwd: harness.ordinaryRoot
    })
    harness.ptys[0]!.write.mockImplementationOnce(() => {
      throw new Error(nativeError)
    })
    await expect(write(eventFor(harness.sender), {
      sessionId: rendererIdentifier,
      data: 'ordinary input'
    })).resolves.toBe(false)

    const serialized = JSON.stringify(harness.logError.mock.calls)
    expect(serialized).not.toContain(rendererIdentifier)
    expect(serialized).not.toContain(nativeError)
    expect(serialized).toContain('terminal_pty_write_failed')
  })
})
