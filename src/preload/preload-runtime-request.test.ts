import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AnalytixApi } from '../shared/analytix-api'
import {
  analytixThreadSteerPath,
  analytixUserInputPath
} from '../shared/analytix-endpoints'

const electronMock = vi.hoisted(() => {
  const exposedApi = new Map<string, unknown>()
  return {
    exposedApi,
    exposeInMainWorld: vi.fn((name: string, api: unknown) => {
      exposedApi.set(name, api)
    }),
    invoke: vi.fn(),
    on: vi.fn(),
    removeListener: vi.fn(),
    getPathForFile: vi.fn()
  }
})

vi.mock('electron', () => ({
  contextBridge: {
    exposeInMainWorld: electronMock.exposeInMainWorld
  },
  ipcRenderer: {
    invoke: electronMock.invoke,
    on: electronMock.on,
    removeListener: electronMock.removeListener
  },
  webUtils: {
    getPathForFile: electronMock.getPathForFile
  }
}))

async function loadPreloadApi(): Promise<AnalytixApi> {
  vi.resetModules()
  electronMock.exposedApi.clear()
  await import('./index')
  const api = electronMock.exposedApi.get('analytix')
  expect(electronMock.exposeInMainWorld).toHaveBeenCalledWith('analytix', api)
  return api as AnalytixApi
}

describe('preload runtime request bridge', () => {
  beforeEach(() => {
    electronMock.exposedApi.clear()
    electronMock.exposeInMainWorld.mockClear()
    electronMock.invoke.mockReset()
    electronMock.on.mockReset()
    electronMock.removeListener.mockReset()
    electronMock.getPathForFile.mockReset()
  })

  it('validates non-secret instance identity through the dedicated app channel', async () => {
    const api = await loadPreloadApi()
    const identity = { mode: 'development', profileId: 'abcdef123456', isolated: true, mock: true, version: '1.0.6' }
    electronMock.invoke.mockResolvedValue(identity)
    await expect(api.app.getEnvironment()).resolves.toEqual(identity)
    expect(electronMock.invoke).toHaveBeenCalledWith('app:environment')
    electronMock.invoke.mockResolvedValue({ ...identity, userData: '/private/canary' })
    await expect(api.app.getEnvironment()).rejects.toThrow()
  })

  it('passes runtime request paths through the analytix runtime facade unchanged', async () => {
    const response = { ok: true, status: 200, body: '{"ok":true}' }
    electronMock.invoke.mockResolvedValue(response)
    const api = await loadPreloadApi()
    const path = analytixThreadSteerPath(
      'thr/with space?x=1#frag',
      'turn/with space?x=1#frag'
    )

    await expect(api.runtime.runtimeRequest(path, 'POST', '{"action":"step"}')).resolves.toEqual(response)

    expect(path).toContain('%2F')
    expect(electronMock.invoke).toHaveBeenCalledWith('runtime:request', {
      path,
      method: 'POST',
      body: '{"action":"step"}'
    })
  })

  it('keeps diagnostics runtime requests on the same analytix IPC channel', async () => {
    const response = { ok: true, status: 202, body: '{"accepted":true}' }
    electronMock.invoke.mockResolvedValue(response)
    const api = await loadPreloadApi()
    const path = analytixUserInputPath('input/with space?x=1#frag')

    await expect(api.diagnostics.runtimeRequest(path, 'POST', '{}')).resolves.toEqual(response)

    expect(path).toContain('%2F')
    expect(electronMock.invoke).toHaveBeenCalledWith('runtime:request', {
      path,
      method: 'POST',
      body: '{}'
    })
  })

  it('uses only the dedicated typed Provider Registry channel without exposing runtime routing', async () => {
    const registryIncarnation = `inc_${'a'.repeat(43)}`
    const providerIncarnation = `inc_${'b'.repeat(43)}`
    const response = {
      schemaVersion: 1,
      registryRevision: '1',
      registryIncarnation,
      provider: {
        id: 'provider-alpha',
        kind: 'openai-compatible',
        endpoint: 'https://provider.invalid/v1',
        models: ['model-alpha'],
        mediaModels: [],
        selectedModel: 'model-alpha',
        selectedRoutes: ['primary'],
        credentialConfigured: true,
        credentialPurpose: 'provider-api-key',
        revision: '1',
        generation: '1',
        incarnation: providerIncarnation,
        tombstone: false
      }
    }
    electronMock.invoke.mockResolvedValue(response)
    const api = await loadPreloadApi()
    const syntheticSecretMarker = 'synthetic-preload-provider-secret'
    const request = {
      schemaVersion: 1 as const,
      operation: 'connect' as const,
      expected: {
        registryRevision: '0',
        registryIncarnation,
        providerRevision: '0',
        providerGeneration: '0',
        providerIncarnation: '',
        providerCredentialPurpose: ''
      },
      provider: {
        id: 'provider-alpha',
        kind: 'openai-compatible',
        endpoint: 'https://provider.invalid/v1',
        proxy: '',
        models: ['model-alpha'],
        mediaModels: [],
        selectedModel: 'model-alpha',
        selectedMediaModel: '',
        selectedRoutes: ['primary']
      },
      credential: {
        kind: 'set' as const,
        purpose: 'provider-api-key',
        valueBase64: Buffer.from(syntheticSecretMarker).toString('base64')
      }
    }

    const result = await api.providerRegistry.request(request)

    expect(result).toEqual(response)
    expect(JSON.stringify(result)).not.toContain(syntheticSecretMarker)
    expect(electronMock.invoke).toHaveBeenCalledWith('provider-registry:request', request)
    expect(electronMock.invoke).not.toHaveBeenCalledWith('runtime:request', expect.anything())
    expect(api.providerRegistry).not.toHaveProperty('runtimeRequest')
  })

  it('exposes protected recovery as four no-argument operation-specific actions', async () => {
    const result = { ok: true as const, status: 'request-created' as const }
    electronMock.invoke.mockResolvedValue(result)
    const api = await loadPreloadApi()

    await expect(api.providerCredentialRecovery.createDestinationRequest()).resolves.toEqual(result)

    expect(electronMock.invoke).toHaveBeenCalledWith(
      'provider-credential-recovery:create-destination-request'
    )
    expect(api.providerCredentialRecovery).not.toHaveProperty('runtimeRequest')
  })

  it('routes explicit runtime restarts through the analytix runtime IPC channel', async () => {
    electronMock.invoke.mockResolvedValue(undefined)
    const api = await loadPreloadApi()

    await expect(api.runtime.restartRuntime()).resolves.toBeUndefined()

    expect(electronMock.invoke).toHaveBeenCalledWith('runtime:restart')
  })

  it('exposes query cache invalidation broadcast on the app facade', async () => {
    electronMock.invoke.mockResolvedValue(true)
    const api = await loadPreloadApi()

    await expect(api.app.invalidateQueryCache({
      queryKey: ['settings'],
      sourceClientId: 'renderer-a'
    })).resolves.toBe(true)

    expect(electronMock.invoke).toHaveBeenCalledWith('query-cache:invalidate', {
      queryKey: ['settings'],
      sourceClientId: 'renderer-a'
    })

    const handler = vi.fn()
    const unsubscribe = api.app.onQueryCacheInvalidated(handler)
    const wrapped = electronMock.on.mock.calls.find(([channel]) => channel === 'query-cache:invalidate')?.[1]
    expect(wrapped).toBeTypeOf('function')
    wrapped?.({}, { queryKey: ['skills'], sourceClientId: 'renderer-b' })
    expect(handler).toHaveBeenCalledWith({ queryKey: ['skills'], sourceClientId: 'renderer-b' })
    unsubscribe()
    expect(electronMock.removeListener).toHaveBeenCalledWith('query-cache:invalidate', wrapped)
  })

  it('exposes Electron file paths for attachment localFilePath propagation', async () => {
    electronMock.getPathForFile.mockReturnValue('/tmp/picked/shot.png')
    const api = await loadPreloadApi()
    const file = { name: 'shot.png', type: 'image/png' } as File

    expect(api.files.getPathForFile(file)).toBe('/tmp/picked/shot.png')

    expect(electronMock.getPathForFile).toHaveBeenCalledWith(file)
  })

  it('exposes git checkpoint operations on the analytix workspace facade', async () => {
    electronMock.invoke.mockResolvedValue({ ok: true, checkpointId: 'gcp_1' })
    const api = await loadPreloadApi()

    await expect(api.workspace.createGitCheckpoint({
      workspaceRoot: '/tmp/workspace',
      threadId: 'thr_1'
    })).resolves.toEqual({ ok: true, checkpointId: 'gcp_1' })
    await expect(api.workspace.restoreGitCheckpoint({
      checkpointId: 'gcp_1'
    })).resolves.toEqual({ ok: true, checkpointId: 'gcp_1' })

    expect(electronMock.invoke).toHaveBeenCalledWith('git:checkpoint:create', {
      workspaceRoot: '/tmp/workspace',
      threadId: 'thr_1'
    })
    expect(electronMock.invoke).toHaveBeenCalledWith('git:checkpoint:restore', {
      checkpointId: 'gcp_1'
    })
  })

  it('exposes thread handoff operations on the analytix workspace facade', async () => {
    electronMock.invoke
      .mockResolvedValueOnce({ operation: { id: 'op-start' } })
      .mockResolvedValueOnce({ operation: { id: 'op-retry' } })
      .mockResolvedValueOnce({ operations: [{ id: 'op-get' }] })
      .mockResolvedValueOnce({ operation: { id: 'op-cancel' } })
      .mockResolvedValueOnce({ removed: true })
      .mockResolvedValueOnce({ operation: { id: 'op-complete' } })
      .mockResolvedValueOnce({ operation: { id: 'op-fail' } })
    const api = await loadPreloadApi()

    await expect(api.workspace.startThreadHandoff({
      direction: 'to-worktree',
      sourceThreadId: 'thr_1',
      sourceWorkspace: '/tmp/project',
      localBranch: 'main'
    })).resolves.toEqual({ operation: { id: 'op-start' } })
    await expect(api.workspace.retryThreadHandoff({ operationId: 'op_1' })).resolves.toEqual({
      operation: { id: 'op-retry' }
    })
    await expect(api.workspace.getThreadHandoffOperations({ operationId: 'op_1' })).resolves.toEqual({
      operations: [{ id: 'op-get' }]
    })
    await expect(api.workspace.cancelThreadHandoff({ operationId: 'op_1' })).resolves.toEqual({
      operation: { id: 'op-cancel' }
    })
    await expect(api.workspace.removeThreadHandoff({ operationId: 'op_1' })).resolves.toEqual({
      removed: true
    })
    await expect(api.workspace.completeThreadHandoffSwitch({
      operationId: 'op_1',
      targetThreadId: 'thr_next'
    })).resolves.toEqual({ operation: { id: 'op-complete' } })
    await expect(api.workspace.failThreadHandoffSwitch({
      operationId: 'op_1',
      message: 'renderer switch failed'
    })).resolves.toEqual({ operation: { id: 'op-fail' } })

    expect(electronMock.invoke).toHaveBeenCalledWith('thread-handoff:start', {
      direction: 'to-worktree',
      sourceThreadId: 'thr_1',
      sourceWorkspace: '/tmp/project',
      localBranch: 'main'
    })
    expect(electronMock.invoke).toHaveBeenCalledWith('thread-handoff:retry', { operationId: 'op_1' })
    expect(electronMock.invoke).toHaveBeenCalledWith('thread-handoff:get', { operationId: 'op_1' })
    expect(electronMock.invoke).toHaveBeenCalledWith('thread-handoff:cancel', { operationId: 'op_1' })
    expect(electronMock.invoke).toHaveBeenCalledWith('thread-handoff:remove', { operationId: 'op_1' })
    expect(electronMock.invoke).toHaveBeenCalledWith('thread-handoff:complete-switch', {
      operationId: 'op_1',
      targetThreadId: 'thr_next'
    })
    expect(electronMock.invoke).toHaveBeenCalledWith('thread-handoff:fail-switch', {
      operationId: 'op_1',
      message: 'renderer switch failed'
    })

    const eventHandler = vi.fn()
    const unsubscribe = api.workspace.onThreadHandoffEvent(eventHandler)
    const wrapped = electronMock.on.mock.calls.find(([channel]) => channel === 'thread-handoff:event')?.[1]
    expect(wrapped).toBeTypeOf('function')
    const event = { operation: { id: 'op-event' } }
    wrapped?.({}, event)
    expect(eventHandler).toHaveBeenCalledWith(event)
    unsubscribe()
    expect(electronMock.removeListener).toHaveBeenCalledWith('thread-handoff:event', wrapped)
  })

  it('exposes background task operations on the analytix background task facade', async () => {
    electronMock.invoke
      .mockResolvedValueOnce({ ok: true, task: { id: 'task-register' } })
      .mockResolvedValueOnce({ tasks: [{ id: 'task-list' }] })
      .mockResolvedValueOnce({ tasks: [{ id: 'task-snapshot' }] })
      .mockResolvedValueOnce({ ok: true, task: { id: 'task-kill' } })
      .mockResolvedValueOnce({ ok: true, task: { id: 'task-restart' } })
      .mockResolvedValueOnce({
        ok: true,
        availability: 'withheld',
        reasonCode: 'electron_background_tasks_retired',
        outputWithheld: true,
        canReadOutput: false,
        factAnswerAllowed: false,
        evidenceAuthority: false
      })
    const api = await loadPreloadApi()

    await expect(api.backgroundTasks.register({
      record: {
        id: 'task_1',
        threadId: 'thr_1',
        pid: 1234,
        processId: '1234',
        status: 'running',
        source: 'summary-command'
      }
    })).resolves.toEqual({ ok: true, task: { id: 'task-register' } })
    await expect(api.backgroundTasks.list({ threadId: 'thr_1' })).resolves.toEqual({
      tasks: [{ id: 'task-list' }]
    })
    await expect(api.backgroundTasks.snapshot({ threadId: 'thr_1' })).resolves.toEqual({
      tasks: [{ id: 'task-snapshot' }]
    })
    await expect(api.backgroundTasks.kill({
      threadId: 'thr_1',
      taskId: 'task_1'
    })).resolves.toEqual({ ok: true, task: { id: 'task-kill' } })
    await expect(api.backgroundTasks.restart({
      threadId: 'thr_1',
      taskId: 'task_1'
    })).resolves.toEqual({ ok: true, task: { id: 'task-restart' } })
    await expect(api.backgroundTasks.output({
      threadId: 'thr_1',
      taskId: 'task_1',
      offset: 0,
      limit: 1024
    })).resolves.toMatchObject({
      ok: true,
      availability: 'withheld',
      outputWithheld: true,
      canReadOutput: false
    })

    expect(electronMock.invoke).toHaveBeenCalledWith('background-task:register', {
      record: {
        id: 'task_1',
        threadId: 'thr_1',
        pid: 1234,
        processId: '1234',
        status: 'running',
        source: 'summary-command'
      }
    })
    expect(electronMock.invoke).toHaveBeenCalledWith('background-task:list', { threadId: 'thr_1' })
    expect(electronMock.invoke).toHaveBeenCalledWith('background-task:snapshot', { threadId: 'thr_1' })
    expect(electronMock.invoke).toHaveBeenCalledWith('background-task:kill', {
      threadId: 'thr_1',
      taskId: 'task_1'
    })
    expect(electronMock.invoke).toHaveBeenCalledWith('background-task:restart', {
      threadId: 'thr_1',
      taskId: 'task_1'
    })
    expect(electronMock.invoke).toHaveBeenCalledWith('background-task:output', {
      threadId: 'thr_1',
      taskId: 'task_1',
      offset: 0,
      limit: 1024
    })
  })
})
