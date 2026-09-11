import { beforeEach, describe, expect, it, vi } from 'vitest'

const testMocks = vi.hoisted(() => ({
  handlers: new Map<string, (...args: unknown[]) => unknown>(),
  handle: vi.fn((channel: string, handler: (...args: unknown[]) => unknown) => {
    testMocks.handlers.set(channel, handler)
  })
}))

vi.mock('electron', () => ({
  ipcMain: {
    handle: testMocks.handle
  }
}))

vi.mock('./case-project-registry', () => ({
  ensureDataAnalysisWorkspaceCase: vi.fn()
}))

import { registerDataAnalysisIpcHandlers } from './ipc'

beforeEach(() => {
  testMocks.handlers.clear()
  testMocks.handle.mockClear()
})

function authorizedManager(overrides: Record<string, unknown> = {}) {
  return {
    onState: vi.fn(() => () => undefined),
    getAuthorizedRenderers: vi.fn(() => []),
    validateRenderer: vi.fn(() => true),
    getRendererAuthorityGeneration: vi.fn(() => 3),
    getState: vi.fn(() => ({ phase: 'running', generation: 7 })),
    ensureBackend: vi.fn(async () => ({
      phase: 'running', generation: 7, authority: 'go-native', terminal: false
    })),
    ...overrides
  }
}

function rendererEvent() {
  const mainFrame = { url: 'file:///app/out/renderer/index.html' }
  const sender = { id: 42, mainFrame }
  return { sender, senderFrame: mainFrame }
}

describe('data analysis IPC renderer authority', () => {
  it('broadcasts terminal endpoint clearing to every authorized renderer', () => {
    let stateListener: ((state: unknown) => void) | undefined
    const first = { send: vi.fn(), isDestroyed: () => false }
    const second = { send: vi.fn(), isDestroyed: () => false }
    const manager = {
      onState: vi.fn((listener: (state: unknown) => void) => {
        stateListener = listener
        return () => undefined
      }),
      getAuthorizedRenderers: vi.fn(() => [first, second]),
      validateRenderer: vi.fn(() => true),
      getRendererAuthorityGeneration: vi.fn(() => 1),
      getState: vi.fn(() => ({ generation: 0 }))
    }

    registerDataAnalysisIpcHandlers({
      manager: manager as never,
      getMainWindow: () => null
    })
    const terminal = { phase: 'stopped', generation: 7 }
    stateListener?.(terminal)

    expect(first.send).toHaveBeenCalledWith('data-analysis:backend-runtime-state', terminal)
    expect(second.send).toHaveBeenCalledWith('data-analysis:backend-runtime-state', terminal)
  })

  it('rejects IPC from a subframe or unregistered navigation generation', async () => {
    const mainFrame = { url: 'file:///app/out/renderer/index.html' }
    const sender = { mainFrame }
    const manager = authorizedManager()
    registerDataAnalysisIpcHandlers({ manager: manager as never, getMainWindow: () => null })
    const ensure = testMocks.handlers.get('data-analysis:ensure-backend')

    await expect(ensure?.({ sender, senderFrame: { url: 'file:///attacker.html' } })).rejects.toThrow(
      'unauthorized data analysis renderer'
    )
    expect(manager.ensureBackend).not.toHaveBeenCalled()

    manager.validateRenderer.mockReturnValueOnce(false)
    await expect(ensure?.({ sender, senderFrame: mainFrame })).rejects.toThrow(
      'unauthorized data analysis renderer'
    )
    expect(manager.ensureBackend).not.toHaveBeenCalled()
  })

  it('returns the fixed native boundary before reading an untrusted workspace payload', async () => {
    const manager = authorizedManager({
      ensureBackend: vi.fn(async () => ({
        phase: 'failed',
        generation: 0,
        authority: 'unavailable',
        terminal: true,
        blocker: 'data_analysis_native_authority_unavailable'
      }))
    })
    registerDataAnalysisIpcHandlers({ manager: manager as never, getMainWindow: () => null })
    const payload = Object.defineProperty({}, 'workspaceRoot', {
      get: () => {
        throw new Error('workspace payload must not be read')
      }
    })

    await expect(
      testMocks.handlers.get('data-analysis:ensure-workspace-case')?.(rendererEvent(), payload)
    ).resolves.toEqual({ ok: false, message: 'data_analysis_native_authority_unavailable' })
  })

  it('quarantines source selection before inspecting renderer options', async () => {
    const manager = authorizedManager()
    registerDataAnalysisIpcHandlers({ manager: manager as never, getMainWindow: () => null })
    const hostileOptions = Object.defineProperty({}, 'title', {
      get: () => {
        throw new Error('selection options must not be read')
      }
    })

    await expect(
      testMocks.handlers.get('data-analysis:pick-files')?.(rendererEvent(), hostileOptions)
    ).resolves.toEqual({ canceled: true, paths: [], filePaths: [] })
    await expect(
      testMocks.handlers.get('data-analysis:pick-directory')?.(rendererEvent())
    ).resolves.toEqual({ canceled: true, path: '', filePath: '' })
  })

  it('does not register renderer-owned publication or generic path-opening channels', () => {
    registerDataAnalysisIpcHandlers({
      manager: authorizedManager() as never,
      getMainWindow: () => null
    })

    for (const channel of [
      'data-analysis:pick-save-file',
      'data-analysis:write-file',
      'data-analysis:open-path'
    ]) {
      expect(testMocks.handlers.has(channel), `unexpected renderer-owned channel: ${channel}`).toBe(false)
    }
  })
})
