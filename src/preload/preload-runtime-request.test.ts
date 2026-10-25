import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AnalytixApi } from '../shared/analytix-api'
import { userInputLocalDisplaySchemaV1 } from '../../packages/runtime/src/contracts/user-input-local-display'
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

it('exposes protected navigation only as a typed identity effect with a closed minimal result', async () => {
  const request = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64) }
  const result = { ok: true, target: { path: '/synthetic/actual.go', workspaceRoot: '/synthetic', line: 9 } }
  const api = await loadPreloadApi()
  electronMock.invoke.mockResolvedValue(result)
  await expect(api.runtime.navigateToolResultLocalDisplay(request)).resolves.toEqual(result)
  expect(electronMock.invoke).toHaveBeenCalledWith('tool-result-local:navigate', request)
  electronMock.invoke.mockClear()
  await expect(api.runtime.navigateToolResultLocalDisplay({ ...request, path: '/forged' } as never)).resolves.toEqual({ ok: false, code: 'unavailable' })
  expect(electronMock.invoke).not.toHaveBeenCalled()
  electronMock.invoke.mockResolvedValue({ ...result, target: { ...result.target, body: 'private' } })
  await expect(api.runtime.navigateToolResultLocalDisplay(request)).resolves.toEqual({ ok: false, code: 'unavailable' })
})

it('sends only protected references for source previews and refuses missing/forged references without ordinary fallback', async () => {
  const protectedRead = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64) }
  const api = await loadPreloadApi()
  electronMock.invoke.mockResolvedValue({ ok: false, message: 'unavailable' })
  await api.files.read({ path: '/forged-display-label', workspaceRoot: '/forged-workspace', line: 1, preview: 'source', protectedRead })
  expect(electronMock.invoke).toHaveBeenCalledWith('file:read-workspace', { protectedRead })
  electronMock.invoke.mockClear()
  await expect(api.files.read({ path: '/forged', preview: 'source' })).resolves.toMatchObject({ ok: false })
  await expect(api.files.read({ path: '/forged', preview: 'source', protectedRead: { ...protectedRead, path: '/forged' } } as never)).resolves.toMatchObject({ ok: false })
  expect(electronMock.invoke).not.toHaveBeenCalled()
  await api.files.read({ path: '/synthetic/ordinary.go', workspaceRoot: '/synthetic', line: 9 })
  expect(electronMock.invoke).toHaveBeenCalledWith('file:read-workspace', { path: '/synthetic/ordinary.go', workspaceRoot: '/synthetic', line: 9 })
})

it('bridges only the sealed Web link role and actual Inspect root-copy selector', async () => {
  const identity = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64) }
  const api = await loadPreloadApi()
  electronMock.invoke.mockResolvedValue({ ok: true })
  await expect(api.runtime.openToolResultLocalWebLink({ ...identity, targetRole: 'result' })).resolves.toEqual({ ok: true })
  expect(electronMock.invoke).toHaveBeenCalledWith('tool-result-local:web-link', { ...identity, targetRole: 'result' })
  electronMock.invoke.mockClear()
  await expect(api.runtime.openToolResultLocalWebLink({ ...identity, targetRole: 'result', url: 'https://forged.invalid' } as never)).resolves.toEqual({ ok: false, code: 'unavailable' })
  expect(electronMock.invoke).not.toHaveBeenCalled()
  await expect(api.runtime.copyToolResultLocalDisplay({ ...identity, section: 'payload' })).resolves.toEqual({ ok: true })
  expect(electronMock.invoke).toHaveBeenCalledWith('tool-result-local:copy', { ...identity, section: 'payload' })
  electronMock.invoke.mockResolvedValue({ ok: true, targetUrl: 'https://private.invalid' })
  await expect(api.runtime.openToolResultLocalWebLink({ ...identity, targetRole: 'request' })).resolves.toEqual({ ok: false, code: 'unavailable' })
})


const privateQuestionSelectorV1 = (suffix = 'a') => {
  const requestId = 'input_' + suffix.repeat(64)
  return { threadId: 'question-thread', turnId: 'question-turn', itemId: 'item_' + requestId, requestId }
}
const privateQuestionRequestV1 = { ...privateQuestionSelectorV1(), viewId: '11111111-1111-4111-8111-111111111111' }
function privateQuestionDisplayV1(selector = privateQuestionSelectorV1(), label = 'CURRENT') {
  return { schemaVersion: 1, ...selector, status: 'submitted', answerState: 'available',
    questions: [{ id: selector.requestId + '_1', header: 'Header', question: label + '_PRIVATE_QUESTION', multiSelect: false, options: [] }],
    submission: { schemaVersion: 1, format: 'structured', answers: [{ id: selector.requestId + '_1', kind: 'answer',
      optionIds: [], customText: label + '_PRIVATE_ANSWER' }] } }
}
describe('private Questions preload contract', () => {
  beforeEach(() => {
    electronMock.invoke.mockReset(); electronMock.on.mockReset(); electronMock.removeListener.mockReset()
  })
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })
  it('exposes only the six fixed runtime actions with a complete closed Core selector', async () => {
    const api = await loadPreloadApi(), display = privateQuestionDisplayV1()
    electronMock.invoke.mockResolvedValueOnce({ ok: true, display })
    expect(await api.runtime.openUserInputLocalDisplay(privateQuestionRequestV1)).toEqual({ ok: true, display })
    expect(electronMock.invoke).toHaveBeenCalledWith('user-input-local:open', privateQuestionRequestV1)
    electronMock.invoke.mockResolvedValueOnce(undefined)
    await api.runtime.closeUserInputLocalDisplay(privateQuestionRequestV1.viewId)
    expect(electronMock.invoke).toHaveBeenCalledWith('user-input-local:close', privateQuestionRequestV1.viewId)
    electronMock.invoke.mockResolvedValueOnce(undefined)
    await api.runtime.invalidateUserInputLocalDisplay()
    expect(electronMock.invoke).toHaveBeenCalledWith('user-input-local:invalidate')
    for (const name of ['openUserInputLocalDisplay', 'closeUserInputLocalDisplay', 'invalidateUserInputLocalDisplay', 'onUserInputLocalDisplayInvalidated', 'discardUserInputLateReply', 'saveUserInputDraft', 'performUserInputLocalEffect']) {
      expect(typeof api.runtime[name as keyof typeof api.runtime]).toBe('function')
      expect(api).not.toHaveProperty(name)
    }
    expect(electronMock.invoke).not.toHaveBeenCalledWith('runtime:request', expect.anything())
  })
  it('rejects malformed UUID/selector and injected authority without invoking Main', async () => {
    const api = await loadPreloadApi()
    for (const request of [
      { ...privateQuestionRequestV1, viewId: 'not-a-uuid' }, { ...privateQuestionRequestV1, requestId: 'invalid' },
      { ...privateQuestionRequestV1, itemId: 'item_input_' + 'b'.repeat(64) },
      { ...privateQuestionRequestV1, threadId: '../foreign' }, { ...privateQuestionRequestV1, questions: [] },
      { ...privateQuestionRequestV1, principal: 'foreign' }, { ...privateQuestionRequestV1, path: '/v1/arbitrary' }
    ]) expect(await api.runtime.openUserInputLocalDisplay(request as never)).toEqual({ ok: false, code: 'unavailable' })
    expect(electronMock.invoke).not.toHaveBeenCalled()
  })
  it('withholds malformed, private-error, foreign and rejected replies', async () => {
    const api = await loadPreloadApi(), display = privateQuestionDisplayV1()
    const foreign = privateQuestionDisplayV1(privateQuestionSelectorV1('b'), 'FOREIGN')
    expect(userInputLocalDisplaySchemaV1.safeParse(foreign).success).toBe(true)
    for (const reply of [
      { ok: false, code: 'unavailable', message: 'PRIVATE_ERROR_DETAIL' },
      { ok: true, display: { ...display, principal: 'PRIVATE_PRINCIPAL' } },
      { ok: true, display: { ...display, questions: [] } }, { ok: true, display: foreign }, 'PRIVATE_MALFORMED'
    ]) {
      electronMock.invoke.mockResolvedValueOnce(reply)
      expect(await api.runtime.openUserInputLocalDisplay(privateQuestionRequestV1)).toEqual({ ok: false, code: 'unavailable' })
    }
    electronMock.invoke.mockRejectedValueOnce(new Error('PRIVATE_REJECTED_IPC'))
    expect(await api.runtime.openUserInputLocalDisplay(privateQuestionRequestV1)).toEqual({ ok: false, code: 'unavailable' })
    electronMock.invoke.mockResolvedValueOnce({ ok: false, code: 'unavailable' })
    expect(await api.runtime.openUserInputLocalDisplay(privateQuestionRequestV1)).toEqual({ ok: false, code: 'unavailable' })
  })
  it('filters invalidation values and unsubscribes only the exact native listener', async () => {
    const api = await loadPreloadApi(), first = vi.fn(), second = vi.fn()
    const off = api.runtime.onUserInputLocalDisplayInvalidated(first)
    api.runtime.onUserInputLocalDisplayInvalidated(second)
    const listeners = electronMock.on.mock.calls.filter(([channel]) => channel === 'user-input-local:invalidated').map(([, handler]) => handler)
    expect(listeners).toHaveLength(2)
    for (const value of [{ question: 'PRIVATE_PAYLOAD' }, 'invalid', 7, undefined]) listeners[0]({ sender: 'PRIVATE_EVENT' }, value)
    expect(first).not.toHaveBeenCalled()
    listeners[0]({ sender: 'PRIVATE_EVENT' }, privateQuestionRequestV1.viewId)
    listeners[0]({ sender: 'PRIVATE_EVENT' }, null)
    listeners[0]({ sender: 'PRIVATE_EVENT' }, null, 'background')
    listeners[0]({ sender: 'PRIVATE_EVENT' }, null, { raw: 'PRIVATE_REASON' })
    expect(first.mock.calls).toEqual([[privateQuestionRequestV1.viewId, 'authority'], [null, 'authority'], [null, 'background'], [null, 'authority']])
    off()
    expect(electronMock.removeListener.mock.calls).toEqual([['user-input-local:invalidated', listeners[0]]])
    expect(electronMock.removeListener).not.toHaveBeenCalledWith('user-input-local:invalidated', listeners[1])
  })
})


describe('private Questions discard preload contract', () => {
  beforeEach(() => { electronMock.invoke.mockReset(); electronMock.on.mockReset(); electronMock.removeListener.mockReset() })
  const request = { ...privateQuestionSelectorV1(), action: 'discardLateReply' as const,
    clientUserMessageId: '67eeac24-98da-4a49-a4ee-1d5b54574999', expectedTurnId: 'turn-target' }
  const receipt = () => ({ schemaVersion: 1, ...privateQuestionSelectorV1(), status: 'continued',
    lateReply: { clientUserMessageId: request.clientUserMessageId, turnId: request.expectedTurnId, state: 'discarded' } })
  it('uses the dedicated fifth private action with independent original and target turns', async () => {
    const api = await loadPreloadApi(); electronMock.invoke.mockResolvedValueOnce(receipt())
    expect(await api.runtime.discardUserInputLateReply(request)).toEqual(receipt())
    expect(electronMock.invoke.mock.calls).toEqual([['user-input-local:discard-late-reply', request]])
    expect(api).not.toHaveProperty('discardUserInputLateReply')
  })
  it('rejects malformed identity and injected fields before invoking Main', async () => {
    const api = await loadPreloadApi()
    for (const invalid of [{ ...request, clientUserMessageId: 'invalid' }, { ...request, expectedTurnId: '' },
      { ...request, viewId: privateQuestionRequestV1.viewId }, { ...request, answers: [] }, { ...request, action: 'cancel' }]) {
      await expect(api.runtime.discardUserInputLateReply(invalid as never)).rejects.toThrow('Question discard unavailable.')
    }
    expect(electronMock.invoke).not.toHaveBeenCalled()
  })
  it('rejects every foreign binding, unknown field and private IPC error without reflection', async () => {
    const api = await loadPreloadApi(), original = receipt()
    const replies = [
      ...(['threadId', 'turnId', 'itemId', 'requestId'] as const).map(field => ({ ...original, [field]: field === 'requestId' ? 'input_' + 'b'.repeat(64) : 'foreign' })),
      { ...original, lateReply: { ...original.lateReply, clientUserMessageId: '77eeac24-98da-4a49-a4ee-1d5b54574999' } },
      { ...original, lateReply: { ...original.lateReply, turnId: 'foreign-target' } },
      { ...original, lateReply: { ...original.lateReply, state: 'accepted' } },
      { ...original, question: 'PRIVATE_RAW' }, { status: 409, message: 'PRIVATE_ERROR' }
    ]
    for (const reply of replies) {
      electronMock.invoke.mockResolvedValueOnce(reply)
      await expect(api.runtime.discardUserInputLateReply(request)).rejects.toThrow(/^Question discard unavailable\.$/)
    }
    electronMock.invoke.mockRejectedValueOnce(new Error('PRIVATE_IPC_REJECTION'))
    await expect(api.runtime.discardUserInputLateReply(request)).rejects.toThrow(/^Question discard unavailable\.$/)
    expect(electronMock.invoke.mock.calls.every(([channel]) => channel === 'user-input-local:discard-late-reply')).toBe(true)
  })
})


describe('private Questions draft-save preload contract', () => {
  beforeEach(() => { electronMock.invoke.mockReset(); electronMock.on.mockReset(); electronMock.removeListener.mockReset() })
  const request = { ...privateQuestionSelectorV1(), action: 'saveDraft' as const, revision: '11111111-1111-4111-8111-111111111111', expectedRevision: null,
    draft: { currentPage: 0, answers: [{ questionId: privateQuestionSelectorV1().requestId + '_1', optionIds: [], customText: '  unfinished\nraw  ', skipped: false }] } }
  const receipt = () => ({ schemaVersion: 1, ...privateQuestionSelectorV1(), status: 'pending', draftRevision: request.revision, draft: request.draft })
  it('exposes only the dedicated sixth private runtime action and preserves exact partial text', async () => {
    const api = await loadPreloadApi(); electronMock.invoke.mockResolvedValueOnce({ ok: true, receipt: receipt() })
    expect(await api.runtime.saveUserInputDraft(request)).toEqual({ ok: true, receipt: receipt() })
    expect(electronMock.invoke.mock.calls).toEqual([['user-input-local:save-draft', request]])
    expect(api).not.toHaveProperty('saveUserInputDraft')
  })
  it('rejects injected authority and invalid UUID/parent/snapshot before Main', async () => {
    const api = await loadPreloadApi()
    for (const invalid of [{ ...request, viewId: privateQuestionRequestV1.viewId }, { ...request, revision: 'invalid' },
      { ...request, expectedRevision: request.revision }, { ...request, draft: { ...request.draft, raw: 'PRIVATE_RAW' } }]) {
      expect(await api.runtime.saveUserInputDraft(invalid as never)).toEqual({ ok: false, code: 'invalid' })
    }
    expect(electronMock.invoke).not.toHaveBeenCalled()
  })
  it('rejects foreign bindings, changed whitespace, unknown outcomes and unsafe error objects', async () => {
    const api = await loadPreloadApi(), original = receipt()
    for (const reply of [
      ...(['threadId', 'turnId', 'itemId', 'requestId'] as const).map(field => ({ ok: true, receipt: { ...original, [field]: field === 'requestId' ? 'input_' + 'b'.repeat(64) : 'foreign' } })),
      { ok: true, receipt: { ...original, draftRevision: '22222222-2222-4222-8222-222222222222' } },
      { ok: true, receipt: { ...original, draft: { ...original.draft, answers: [{ ...original.draft.answers[0], customText: original.draft.answers[0].customText.trim() }] } } },
      { ok: true, receipt: { ...original, status: 'continued', accepted: true } },
      { ok: false, code: 'retired', message: 'PRIVATE_ERROR' }, { ok: false, code: 'private_custom_cause' }
    ]) {
      electronMock.invoke.mockResolvedValueOnce(reply)
      expect(await api.runtime.saveUserInputDraft(request)).toEqual({ ok: false, code: 'unavailable' })
    }
    electronMock.invoke.mockRejectedValueOnce(new Error('PRIVATE_IPC_REJECTION'))
    expect(await api.runtime.saveUserInputDraft(request)).toEqual({ ok: false, code: 'unavailable' })
  })
  it.each(['invalid', 'retired', 'conflict', 'capacity', 'unavailable'] as const)('preserves only the fixed %s failure classification', async code => {
    const api = await loadPreloadApi(); electronMock.invoke.mockResolvedValueOnce({ ok: false, code })
    expect(await api.runtime.saveUserInputDraft(request)).toEqual({ ok: false, code })
  })
})

describe('Question native effect preload boundary', () => {
  beforeEach(() => { electronMock.invoke.mockReset(); electronMock.on.mockReset(); electronMock.removeListener.mockReset() })
  const request = { selector: privateQuestionSelectorV1(), viewId: privateQuestionRequestV1.viewId,
    effect: { kind: 'copyCode' as const, questionId: privateQuestionSelectorV1().requestId + '_1', start: 0, end: 20 } }
  it('sends only the bound source metadata through the dedicated private action', async () => {
    const api = await loadPreloadApi(); electronMock.invoke.mockResolvedValueOnce({ ok: true })
    expect(await api.runtime.performUserInputLocalEffect(request)).toEqual({ ok: true })
    expect(electronMock.invoke.mock.calls).toEqual([['user-input-local:effect', request]])
  })
  it('rejects caller raw text/URL/extra authority and malformed acknowledgements', async () => {
    const api = await loadPreloadApi()
    for (const invalid of [{ ...request, raw: 'PRIVATE_RAW' }, { ...request, effect: { ...request.effect, text: 'PRIVATE_RAW' } },
      { ...request, effect: { ...request.effect, url: 'https://private.invalid' } }, { ...request, viewId: 'invalid' },
      { ...request, effect: { ...request.effect, end: 0 } }]) {
      expect(await api.runtime.performUserInputLocalEffect(invalid as never)).toEqual({ ok: false, code: 'unavailable' })
    }
    expect(electronMock.invoke).not.toHaveBeenCalled()
    for (const reply of [{ ok: true, body: 'PRIVATE_RAW' }, { ok: false, code: 'private_reason' }, null]) {
      electronMock.invoke.mockResolvedValueOnce(reply)
      expect(await api.runtime.performUserInputLocalEffect(request)).toEqual({ ok: false, code: 'unavailable' })
    }
    electronMock.invoke.mockRejectedValueOnce(new Error('PRIVATE_RAW'))
    expect(await api.runtime.performUserInputLocalEffect(request)).toEqual({ ok: false, code: 'unavailable' })
  })
})


it('maps closed schedule task operations to their desktop IPC channels', async () => {
  electronMock.invoke.mockResolvedValue({ kind: 'success', tasks: [] })
  const api = await loadPreloadApi()
  await api.schedule.listTasks()
  expect(electronMock.invoke).toHaveBeenLastCalledWith('schedule:tasks:list')
  const input = { title: 'New', prompt: 'Run', schedule: { kind: 'manual' as const } }
  await api.schedule.createTask(input)
  expect(electronMock.invoke).toHaveBeenLastCalledWith('schedule:task:create', input)
  const request = { taskId: 'task-1', patch: { title: 'New' }, expected: { title: 'Old' } }
  await api.schedule.updateTask(request)
  expect(electronMock.invoke).toHaveBeenLastCalledWith('schedule:task:update', request)
})


it('binds Present application effects and current previews to the original declaration index only', async () => {
  const identity = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64), index: 1 }
  const api = await loadPreloadApi()
  const apps = { ok: true, applications: [{ applicationId: 'org.synthetic.editor', label: 'Actual editor', isDefault: true }] }
  electronMock.invoke.mockResolvedValue(apps)
  await expect(api.runtime.listToolResultLocalPresentApplications(identity)).resolves.toEqual(apps)
  expect(electronMock.invoke).toHaveBeenLastCalledWith('tool-result-local:present-applications', identity)
  electronMock.invoke.mockResolvedValue({ ok: true })
  await expect(api.runtime.openToolResultLocalPresentFile({ ...identity, applicationId: 'org.synthetic.editor' })).resolves.toEqual({ ok: true })
  expect(electronMock.invoke).toHaveBeenLastCalledWith('tool-result-local:present-open', { ...identity, applicationId: 'org.synthetic.editor' })
  await expect(api.runtime.openToolResultLocalPresentFile({ ...identity, action: 'reveal' })).resolves.toEqual({ ok: true })
  expect(electronMock.invoke).toHaveBeenLastCalledWith('tool-result-local:present-open', { ...identity, action: 'reveal' })
  await api.files.read({ path: '/forged/display-name', workspaceRoot: '/forged/workspace', preview: 'source', protectedPresent: identity })
  expect(electronMock.invoke).toHaveBeenLastCalledWith('file:read-workspace', { protectedPresent: identity })
  electronMock.invoke.mockClear()
  for (const extra of [{ path: '/forged' }, { index: -1 }, { index: 8 }, { appPath: '/forged.app' },
    { action: 'reveal', applicationId: 'org.synthetic.editor' }, { action: 'unknown' }, { action: 'reveal', path: '/forged' }]) {
    await expect(api.runtime.openToolResultLocalPresentFile({ ...identity, ...extra } as never)).resolves.toMatchObject({ ok: false })
  }
  await expect(api.files.read({ path: '/forged', protectedPresent: identity, protectedRead: identity } as never)).resolves.toMatchObject({ ok: false })
  expect(electronMock.invoke).not.toHaveBeenCalled()
  electronMock.invoke.mockResolvedValue({ ...apps, applications: [{ ...apps.applications[0], appPath: '/private.app' }] })
  await expect(api.runtime.listToolResultLocalPresentApplications(identity)).resolves.toEqual({ ok: false, code: 'unavailable' })
})


it('keeps Present history and associations on their strict private channels', async () => {
  const api = await loadPreloadApi()
  const historyRequest = { kind: 'present_history' as const, threadId: 'thread-a', turnId: 'turn-a', viewId: '11111111-1111-4111-8111-111111111111' }
  const history = { schemaVersion: 1, kind: 'present_history', threadId: 'thread-a', turnId: 'turn-a', orderedItems: [] }
  electronMock.invoke.mockResolvedValue({ ok: true, history })
  expect(await api.runtime.openToolResultLocalPresentHistory(historyRequest)).toEqual({ ok: true, history })
  expect(electronMock.invoke).toHaveBeenLastCalledWith('tool-result-local:present-history', historyRequest)
  const { kind: _kind, ...owner } = historyRequest
  const association = { ...owner, callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64), index: 0, artifactIds: ['d'.repeat(64)] }
  electronMock.invoke.mockResolvedValue({ ok: true, fileIdentity: historyRequest.viewId, artifactIds: association.artifactIds })
  expect(await api.runtime.associateToolResultLocalPresentFile(association)).toMatchObject({ ok: true })
  expect(electronMock.invoke).toHaveBeenLastCalledWith('tool-result-local:present-associations', association)
  electronMock.invoke.mockClear()
  expect(await api.runtime.openToolResultLocalPresentHistory({ ...historyRequest, path: '/private' } as never)).toEqual({ ok: false, code: 'unavailable' })
  expect(await api.runtime.associateToolResultLocalPresentFile({ ...association, path: '/private' } as never)).toEqual({ ok: false, code: 'unavailable' })
  expect(electronMock.invoke).not.toHaveBeenCalled()
  electronMock.invoke.mockResolvedValue({ ok: true, history: { ...history, orderedItems: [], path: '/private' } })
  expect(await api.runtime.openToolResultLocalPresentHistory(historyRequest)).toEqual({ ok: false, code: 'unavailable' })
  electronMock.invoke.mockResolvedValue({ ok: true, fileIdentity: historyRequest.viewId, artifactIds: [], path: '/private' })
  expect(await api.runtime.associateToolResultLocalPresentFile(association)).toEqual({ ok: false, code: 'unavailable' })
})

it('passes only the exact task/cursor History read through the dedicated Schedule IPC facade', async () => {
  const api = await loadPreloadApi(), request = { taskId: 'task-a', limit: 20 as const, before: 'actual-attempt' }
  electronMock.invoke.mockResolvedValue({ kind: 'success', records: [], deliveryKind: 'legacy-headless' })
  await api.schedule.readHistory(request)
  expect(electronMock.invoke).toHaveBeenCalledWith('schedule:task:history', request)
})


it.each(['readImage', 'readPdf', 'readExcel'] as const)('Present %s keeps media on its opaque current index channel and denies Read fallthrough', async method => {
  const identity = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64), index: 1 }
  const api = await loadPreloadApi(), channel = { readImage: 'file:read-workspace-image',
    readPdf: 'file:read-workspace-pdf', readExcel: 'file:read-workspace-excel' }[method]
  electronMock.invoke.mockResolvedValue({ ok: false, message: 'unavailable' })
  await api.files[method]({ path: '/forged/display-name', workspaceRoot: '/forged/workspace', preview: 'source', protectedPresent: identity })
  expect(electronMock.invoke).toHaveBeenLastCalledWith(channel, { protectedPresent: identity })
  electronMock.invoke.mockClear()
  for (const options of [
    { path: '/forged', protectedPresent: { ...identity, path: '/forged' } },
    { path: '/forged', protectedPresent: { ...identity, index: 8 } },
    { path: '/forged', protectedPresent: null },
    { path: '/forged', protectedPresent: identity, protectedRead: identity },
    { path: '/forged', protectedRead: identity }, { path: '/forged', preview: 'source' }
  ]) await expect(api.files[method](options as never)).resolves.toMatchObject({ ok: false })
  expect(electronMock.invoke).not.toHaveBeenCalled()
  const ordinary = { path: method === 'readExcel' ? '/ordinary/file.csv' : '/ordinary/file.png', workspaceRoot: '/ordinary' }
  await api.files[method](ordinary); expect(electronMock.invoke).toHaveBeenLastCalledWith(channel, ordinary)
})


it('keeps protected spreadsheet Copy content inside Main and rejects forged rich authority', async () => {
  const selected = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64),
    index: 0, contentDigest: 'd'.repeat(64), selection: { sheetIndex: 0, sheetId: '1', ranges: [{ row: [0, 0], column: [0, 0] }] } }
  const api = await loadPreloadApi()
  electronMock.invoke.mockClear()
  for (const extra of [{ text: 'renderer text' }, { html: '<table>renderer HTML</table>' },
    { checkedHtml: '<table>forged checked HTML</table>' },
    { selection: { ...selected.selection, html: '<table>renderer HTML</table>' } }]) {
    await expect(api.runtime.copyToolResultLocalDisplay({ ...selected, ...extra } as never)).resolves.toEqual({ ok: false, code: 'unavailable' })
  }
  expect(electronMock.invoke).not.toHaveBeenCalled()
  electronMock.invoke.mockResolvedValueOnce({ ok: true })
  await expect(api.runtime.copyToolResultLocalDisplay(selected as never)).resolves.toEqual({ ok: true })
  expect(electronMock.invoke).toHaveBeenLastCalledWith('tool-result-local:copy', selected)
  for (const extra of [{ text: 'private text' }, { html: '<table>private HTML</table>' },
    { checkedHtml: '<table>private checked HTML</table>' }]) {
    electronMock.invoke.mockResolvedValueOnce({ ok: true, ...extra })
    await expect(api.runtime.copyToolResultLocalDisplay(selected as never)).resolves.toEqual({ ok: false, code: 'unavailable' })
  }
})


it('saves Present originals by closed file identity on the existing bridge and preserves text saves', async () => {
  const identity = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64) }
  const api = await loadPreloadApi(), indexed = { ...identity, index: 1 }
  electronMock.invoke.mockResolvedValue({ ok: true })
  for (const request of [identity, indexed]) {
    await expect(api.runtime.saveToolResultLocalDisplay(request)).resolves.toEqual({ ok: true })
    expect(electronMock.invoke).toHaveBeenLastCalledWith('tool-result-local:save', request)
  }
  electronMock.invoke.mockClear()
  for (const extra of [{ path: '/forged' }, { dataBase64: 'Zm9yZ2Vk' }, { index: -1 }, { index: 8 },
    { index: 1.5 }, { body: 'forged' }, { section: 'result' }, { threadId: '../other' }]) {
    await expect(api.runtime.saveToolResultLocalDisplay({ ...indexed, ...extra } as never)).resolves.toEqual({ ok: false, code: 'unavailable' })
  }
  expect(electronMock.invoke).not.toHaveBeenCalled()
  for (const result of [{ ok: false, code: 'canceled' }, { ok: false, code: 'unsupported' }, { ok: false, code: 'unavailable' }]) {
    electronMock.invoke.mockResolvedValue(result)
    await expect(api.runtime.saveToolResultLocalDisplay(indexed)).resolves.toEqual(result)
  }
  electronMock.invoke.mockResolvedValue({ ok: true, path: '/private/output', dataBase64: 'Zm9yZ2Vk' })
  await expect(api.runtime.saveToolResultLocalDisplay(indexed)).resolves.toEqual({ ok: false, code: 'unavailable' })
  electronMock.invoke.mockRejectedValue(new Error('synthetic transport failure'))
  await expect(api.runtime.saveToolResultLocalDisplay(indexed)).resolves.toEqual({ ok: false, code: 'unavailable' })
})
