import { spawnSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { USER_INPUT_LOCAL_DISPLAY_MAX_BYTES_V1, userInputLocalDisplaySchemaV1 } from '../../../packages/runtime/src/contracts/user-input-local-display'
import { EventEmitter } from 'node:events'
import { describe, expect, it, vi } from 'vitest'
vi.mock('electron', () => ({ ipcMain: { handle: vi.fn() } }))
vi.mock('../logger', () => ({ logUserInputLocalReadFailure: vi.fn() }))
import { logUserInputLocalReadFailure } from '../logger'
import { ipcMain, type BrowserWindow, type IpcMainInvokeEvent } from 'electron'
import { registerUserInputLocalDisplayIpcV1, UserInputLocalDisplayHostV1 } from './user-input-local-display-ipc'

const requestId = 'input_' + 'a'.repeat(64)
const selector = { threadId: 'thread', turnId: 'turn', requestId, itemId: 'item_' + requestId }
const request = { ...selector, viewId: '67eeac24-98da-4a49-a4ee-1d5b54574999' }
const display = { schemaVersion: 1, ...selector, status: 'submitted', answerState: 'available',
  questions: [{ id: requestId + '_1', header: 'Header', question: 'Original question', detail: '**Original detail**', multiSelect: false, options: [] }],
  submission: { schemaVersion: 1, format: 'structured', answers: [{ id: requestId + '_1', kind: 'answer', optionIds: [], customText: '  original\nanswer  ' }] } }
const response = () => ({ ok: true, status: 200, body: JSON.stringify(display) })
const owner = () => { const controller = new AbortController(); return { isCurrent: () => true, signal: controller.signal,
  release: vi.fn(() => controller.abort()) } }

it('keeps an opened inherited view readonly before draft/discard transport', async () => {
  const historical = { ...display, readOnly: true, status: 'continued', answerState: 'unavailable', submission: undefined,
    timedWait: null, continuedReason: null, draft: null, draftRevision: null, lateReply: null }
  const transport = vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify(historical) }))
  const host = new UserInputLocalDisplayHostV1(transport, vi.fn())
  expect((await host.open(request, owner())).ok).toBe(true)
  const revision = '00000000-0000-4000-8000-000000000001'
  expect(await host.saveDraft({ ...selector, action: 'saveDraft', revision, expectedRevision: null,
    draft: { currentPage: 0, answers: [] } }, owner())).toEqual({ ok: false, code: 'unavailable' })
  await expect(host.discard({ ...selector, action: 'discardLateReply', clientUserMessageId: revision, expectedTurnId: 'current-turn' }, owner())).rejects.toThrow('Question discard unavailable.')
  expect(transport).toHaveBeenCalledOnce()
  host.close(request.viewId)
})

describe('private user input native host', () => {
  it('returns exact native display data through the fixed private route', async () => {
    const transport = vi.fn(async (_path: string, _body: string, _signal?: AbortSignal) => response()), document = owner()
    const host = new UserInputLocalDisplayHostV1(transport, vi.fn())
    expect(await host.open(request, document)).toEqual({ ok: true, display })
    expect(transport.mock.calls[0][0]).toBe('/v1/local-display/user-input')
    expect(JSON.parse(transport.mock.calls[0][1])).toEqual(selector)
    host.close(request.viewId)
    expect(document.release).toHaveBeenCalledOnce()
  })
  it.each(['threadId', 'turnId', 'itemId', 'requestId'] as const)('rejects a response bound to a different %s', async field => {
    const body = JSON.stringify({ ...display, [field]: field === 'requestId' ? 'input_' + 'b'.repeat(64) : 'other' })
    const document = owner(), host = new UserInputLocalDisplayHostV1(async () => ({ ok: true, status: 200, body }), vi.fn())
    expect(await host.open(request, document)).toEqual({ ok: false, code: 'unavailable' })
    expect(document.release).toHaveBeenCalledOnce()
  })
  it('retires an in-flight native request when its document is no longer current', async () => {
    let resolve!: (value: ReturnType<typeof response>) => void
    let current = true
    const document = { ...owner(), isCurrent: () => current }
    const host = new UserInputLocalDisplayHostV1(() => new Promise(done => { resolve = done }), vi.fn())
    const pending = host.open(request, document)
    current = false; resolve(response())
    expect(await pending).toEqual({ ok: false, code: 'unavailable' })
    expect(document.release).toHaveBeenCalledOnce()
  })
  it('does not let an earlier open replace a later document lease using the same view ID', async () => {
    let resolve!: (value: ReturnType<typeof response>) => void
    const transport = vi.fn().mockImplementationOnce(() => new Promise(done => { resolve = done })).mockResolvedValue(response())
    const host = new UserInputLocalDisplayHostV1(transport, vi.fn())
    const first = owner(), second = owner()
    const old = host.open(request, first)
    expect(await host.open(request, second)).toEqual({ ok: true, display })
    resolve(response())
    expect(await old).toEqual({ ok: false, code: 'unavailable' })
    expect(first.release).toHaveBeenCalledOnce()
    expect(second.release).not.toHaveBeenCalled()
    host.close(request.viewId)
  })
  it('clears all native leases on authority invalidation and suppresses late payloads', async () => {
    let resolve!: (value: ReturnType<typeof response>) => void
    const notify = vi.fn(), document = owner()
    const host = new UserInputLocalDisplayHostV1(() => new Promise(done => { resolve = done }), notify)
    const pending = host.open(request, document)
    host.invalidate(); resolve(response())
    expect(await pending).toEqual({ ok: false, code: 'unavailable' })
    expect(document.release).toHaveBeenCalledOnce()
    expect(notify).toHaveBeenCalledWith(null, 'authority')
  })
  it('does not read invalid client selectors or reflect malformed private error bodies', async () => {
    const transport = vi.fn(async () => ({ ok: false, status: 500, body: 'private sensitive failure' }))
    const host = new UserInputLocalDisplayHostV1(transport, vi.fn())
    expect(await host.open({ ...request, body: 'injected' }, owner())).toEqual({ ok: false, code: 'unavailable' })
    expect(transport).not.toHaveBeenCalled()
    expect(await host.open(request, owner())).toEqual({ ok: false, code: 'unavailable' })
  })
})


describe('private native close confirmation', () => {
  it('rejects unauthorized or malformed close instead of acknowledging an ignored operation', () => {
    vi.mocked(ipcMain.handle).mockClear()
    const frame = {}, send = vi.fn(), contents = { id: 651, mainFrame: frame, isDestroyed: () => false, send }
    let visible = true
    const window = { webContents: contents, isDestroyed: () => false, isVisible: () => visible, isMinimized: () => false }
    registerUserInputLocalDisplayIpcV1(() => window as unknown as BrowserWindow, vi.fn())
    const handler = vi.mocked(ipcMain.handle).mock.calls.find(call => call[0] === 'user-input-local:close')![1]
    const event = (senderFrame: unknown = frame) => ({ sender: contents, senderFrame }) as unknown as IpcMainInvokeEvent
    expect(() => handler(event({}), request.viewId)).toThrow('Question display authority unavailable.')
    expect(() => handler(event(), null)).toThrow('Question display authority unavailable.')
    visible = false
    expect(() => handler(event(), request.viewId)).toThrow('Question display authority unavailable.')
    expect(send).not.toHaveBeenCalled()
    visible = true
    expect(handler(event(), request.viewId)).toBeUndefined()
    expect(send).toHaveBeenCalledWith('user-input-local:invalidated', request.viewId)
  })
})


const discardRequest = { ...selector, action: 'discardLateReply' as const,
  clientUserMessageId: '67eeac24-98da-4a49-a4ee-1d5b54574999', expectedTurnId: 'turn-target' }
const discarded = () => ({ schemaVersion: 1, ...selector, status: 'continued',
  lateReply: { clientUserMessageId: discardRequest.clientUserMessageId, turnId: discardRequest.expectedTurnId, state: 'discarded' } })
const discardResponse = () => ({ ok: true, status: 200, body: JSON.stringify(discarded()) })
describe('private queued-answer discard native host', () => {
  it('sends only the original selector, queued UUID and independent target through the private route', async () => {
    const transport = vi.fn(async (_path: string, _body: string, _signal?: AbortSignal) => discardResponse())
    const document = owner(), host = new UserInputLocalDisplayHostV1(transport, vi.fn())
    expect(await host.discard(discardRequest, document)).toEqual(discarded())
    expect(transport.mock.calls[0][0]).toBe('/v1/local-display/user-input')
    expect(JSON.parse(transport.mock.calls[0][1])).toEqual(discardRequest)
    expect(transport.mock.calls[0][2]).toBe(document.signal)
    expect(discardRequest.turnId).not.toBe(discardRequest.expectedTurnId)
    expect(document.release).toHaveBeenCalledOnce()
  })
  it('rejects injected authority before transport', async () => {
    const transport = vi.fn(async () => discardResponse()), document = owner()
    const host = new UserInputLocalDisplayHostV1(transport, vi.fn())
    await expect(host.discard({ ...discardRequest, viewId: request.viewId }, document)).rejects.toThrow('Question discard unavailable.')
    expect(transport).not.toHaveBeenCalled(); expect(document.release).toHaveBeenCalledOnce()
  })
  it.each(['threadId', 'turnId', 'itemId', 'requestId', 'clientUserMessageId', 'targetTurnId', 'queued', 'accepted', 'raw'] as const)('withholds an unbound or non-discarded %s receipt', async field => {
    const receipt = discarded()
    const body = field === 'clientUserMessageId' ? { ...receipt, lateReply: { ...receipt.lateReply, clientUserMessageId: '77eeac24-98da-4a49-a4ee-1d5b54574999' } }
      : field === 'targetTurnId' ? { ...receipt, lateReply: { ...receipt.lateReply, turnId: 'different-target' } }
      : field === 'queued' || field === 'accepted' ? { ...receipt, lateReply: { ...receipt.lateReply, state: field } }
      : field === 'raw' ? { ...receipt, submission: 'PRIVATE_RESPONSE' }
      : { ...receipt, [field]: field === 'requestId' ? 'input_' + 'b'.repeat(64) : 'other' }
    const document = owner(), host = new UserInputLocalDisplayHostV1(async () => ({ ok: true, status: 200, body: JSON.stringify(body) }), vi.fn())
    await expect(host.discard(discardRequest, document)).rejects.toThrow('Question discard unavailable.')
    expect(document.release).toHaveBeenCalledOnce()
  })
  it.each([400, 404, 409, 413, 500, 202])('does not reflect a private %s error body', async status => {
    const host = new UserInputLocalDisplayHostV1(async () => ({ ok: status < 400, status, body: 'PRIVATE_SENSITIVE_ERROR' }), vi.fn())
    await expect(host.discard(discardRequest, owner())).rejects.toThrow(/^Question discard unavailable\.$/)
  })
  it('aborts an invalidated command immediately without allowing its late completion to retire a successor raw view', async () => {
    let finish!: (response: ReturnType<typeof discardResponse>) => void
    const transport = vi.fn().mockImplementationOnce(() => new Promise(done => { finish = done })).mockResolvedValue(response())
    const host = new UserInputLocalDisplayHostV1(transport, vi.fn()), old = owner(), successor = owner()
    const pending = host.discard(discardRequest, old)
    const failed = expect(pending).rejects.toThrow('Question discard unavailable.')
    host.invalidate(); expect(old.signal.aborted).toBe(true); expect(old.release).toHaveBeenCalledOnce()
    expect(await host.open(request, successor)).toEqual({ ok: true, display })
    finish(discardResponse()); await failed
    expect(successor.release).not.toHaveBeenCalled(); expect(old.release).toHaveBeenCalledOnce()
    host.close(request.viewId); expect(successor.release).toHaveBeenCalledOnce()
  })
  it('bounds in-flight command owners independently of raw views', async () => {
    let finish!: (response: ReturnType<typeof discardResponse>) => void
    const held = new Promise<ReturnType<typeof discardResponse>>(done => { finish = done })
    const transport = vi.fn(async () => held), host = new UserInputLocalDisplayHostV1(transport, vi.fn())
    const owners = Array.from({ length: 32 }, () => owner())
    const pending = owners.map(document => host.discard(discardRequest, document).catch(error => error.message))
    const overflow = owner()
    await expect(host.discard(discardRequest, overflow)).rejects.toThrow('Question discard unavailable.')
    expect(transport).toHaveBeenCalledTimes(32); expect(overflow.signal.aborted).toBe(true)
    host.invalidate(); expect(owners.every(document => document.signal.aborted)).toBe(true)
    finish(discardResponse()); expect(await Promise.all(pending)).toEqual(Array(32).fill('Question discard unavailable.'))
  })
})


const saveDraftRequest = { ...selector, action: 'saveDraft' as const, revision: '11111111-1111-4111-8111-111111111111', expectedRevision: null,
  draft: { currentPage: 0, answers: [{ questionId: requestId + '_1', optionIds: [], customText: '  unfinished\noriginal  ', skipped: false }] } }
const saveDraftReceipt = () => ({ schemaVersion: 1, ...selector, status: 'pending', draftRevision: saveDraftRequest.revision, draft: saveDraftRequest.draft })
const saveDraftResponse = () => ({ ok: true, status: 200, body: JSON.stringify(saveDraftReceipt()) })
describe('private unfinished-draft native host', () => {
  it('uses only the private route and exact original selector/revision/unfinished snapshot', async () => {
    const transport = vi.fn(async (_path: string, _body: string, _signal?: AbortSignal) => saveDraftResponse())
    const document = owner(), host = new UserInputLocalDisplayHostV1(transport, vi.fn())
    expect(await host.saveDraft(saveDraftRequest, document)).toEqual({ ok: true, receipt: saveDraftReceipt() })
    expect(transport).toHaveBeenCalledOnce()
    expect(transport.mock.calls[0][0]).toBe('/v1/local-display/user-input')
    expect(JSON.parse(transport.mock.calls[0][1])).toEqual(saveDraftRequest)
    expect(transport.mock.calls[0][2]).toBe(document.signal)
    expect(document.release).toHaveBeenCalledOnce()
  })
  it.each(['unknown', 'view', 'parent', 'revision', 'answer'] as const)('rejects malformed %s before private transport', async invalid => {
    const payload = invalid === 'unknown' ? { ...saveDraftRequest, authorization: 'PRIVATE_AUTHORITY' }
      : invalid === 'view' ? { ...saveDraftRequest, viewId: request.viewId }
      : invalid === 'parent' ? { ...saveDraftRequest, expectedRevision: saveDraftRequest.revision }
      : invalid === 'revision' ? { ...saveDraftRequest, revision: 'not-a-uuid' }
      : { ...saveDraftRequest, draft: { ...saveDraftRequest.draft, answers: [{ ...saveDraftRequest.draft.answers[0], questionId: 'foreign' }] } }
    const transport = vi.fn(), document = owner(), host = new UserInputLocalDisplayHostV1(transport, vi.fn())
    expect(await host.saveDraft(payload, document)).toEqual({ ok: false, code: 'invalid' })
    expect(transport).not.toHaveBeenCalled(); expect(document.release).toHaveBeenCalledOnce()
  })
  it.each(['threadId', 'turnId', 'itemId', 'requestId', 'revision', 'snapshot', 'raw', 'outcome'] as const)('withholds an unbound %s ACK', async invalid => {
    const receipt = saveDraftReceipt()
    const value = invalid === 'revision' ? { ...receipt, draftRevision: '22222222-2222-4222-8222-222222222222' }
      : invalid === 'snapshot' ? { ...receipt, draft: { ...receipt.draft, answers: [{ ...receipt.draft.answers[0], customText: receipt.draft.answers[0].customText.trim() }] } }
      : invalid === 'raw' ? { ...receipt, privateError: 'PRIVATE_RAW' }
      : invalid === 'outcome' ? { ...receipt, status: 'submitted' }
      : { ...receipt, [invalid]: invalid === 'requestId' ? 'input_' + 'b'.repeat(64) : 'foreign' }
    const document = owner(), host = new UserInputLocalDisplayHostV1(async () => ({ ok: true, status: 200, body: JSON.stringify(value) }), vi.fn())
    expect(await host.saveDraft(saveDraftRequest, document)).toEqual({ ok: false, code: 'unavailable' }); expect(document.release).toHaveBeenCalledOnce()
  })
  it.each([[400, 'invalid'], [404, 'retired'], [409, 'conflict'], [413, 'capacity'], [500, 'unavailable']] as const)('projects HTTP %s only as fixed %s without private body', async (status, code) => {
    const host = new UserInputLocalDisplayHostV1(async () => ({ ok: false, status, body: 'PRIVATE_ERROR_AND_RAW_DRAFT' }), vi.fn())
    expect(await host.saveDraft(saveDraftRequest, owner())).toEqual({ ok: false, code })
  })
  it.each([201, 202, 204])('does not acknowledge non-200 HTTP %s as a saved draft', async status => {
    const host = new UserInputLocalDisplayHostV1(async () => ({ ...saveDraftResponse(), status }), vi.fn())
    expect(await host.saveDraft(saveDraftRequest, owner())).toEqual({ ok: false, code: 'unavailable' })
  })
  it('retires an in-flight save without releasing a successor raw view twice', async () => {
    let finish!: (response: ReturnType<typeof saveDraftResponse>) => void
    const transport = vi.fn().mockImplementationOnce(() => new Promise(resolve => { finish = resolve })).mockResolvedValue(response())
    const host = new UserInputLocalDisplayHostV1(transport, vi.fn()), old = owner(), next = owner()
    const pending = host.saveDraft(saveDraftRequest, old)
    host.invalidate(); expect(old.signal.aborted).toBe(true)
    expect(await host.open(request, next)).toEqual({ ok: true, display })
    finish(saveDraftResponse()); expect(await pending).toEqual({ ok: false, code: 'unavailable' })
    expect(old.release).toHaveBeenCalledOnce(); expect(next.release).not.toHaveBeenCalled()
    host.close(request.viewId); expect(next.release).toHaveBeenCalledOnce()
  })
  it('shares the existing bounded command owners with discard without reserving raw views', async () => {
    let finish!: (response: ReturnType<typeof saveDraftResponse>) => void
    const held = new Promise<ReturnType<typeof saveDraftResponse>>(resolve => { finish = resolve })
    const transport = vi.fn(async () => held), host = new UserInputLocalDisplayHostV1(transport, vi.fn())
    const documents = Array.from({ length: 32 }, () => owner()), pending = documents.map(document => host.saveDraft(saveDraftRequest, document))
    expect(await host.saveDraft(saveDraftRequest, owner())).toEqual({ ok: false, code: 'unavailable' })
    await expect(host.discard(discardRequest, owner())).rejects.toThrow('Question discard unavailable.')
    expect(transport).toHaveBeenCalledTimes(32)
    host.invalidate(); finish(saveDraftResponse())
    expect(await Promise.all(pending)).toEqual(Array.from({ length: 32 }, () => ({ ok: false, code: 'unavailable' })))
    expect(documents.every(document => document.release.mock.calls.length === 1 && document.signal.aborted)).toBe(true)
  })
  it('admits only the current visible main document and aborts its save on real document navigation', async () => {
    vi.mocked(ipcMain.handle).mockClear()
    const contents = Object.assign(new EventEmitter(), { id: 1651, mainFrame: {}, isDestroyed: () => false, send: vi.fn() })
    let visible = true
    const window = Object.assign(new EventEmitter(), { webContents: contents, isDestroyed: () => false, isVisible: () => visible, isMinimized: () => false })
    let finish!: (response: ReturnType<typeof saveDraftResponse>) => void
    const transport = vi.fn((_path: string, _body: string, _signal?: AbortSignal) => new Promise<ReturnType<typeof saveDraftResponse>>(resolve => { finish = resolve }))
    registerUserInputLocalDisplayIpcV1(() => window as unknown as BrowserWindow, transport)
    const invoke = vi.mocked(ipcMain.handle).mock.calls.find(call => call[0] === 'user-input-local:save-draft')![1]
    const event = (senderFrame: unknown = contents.mainFrame) => ({ sender: contents, senderFrame }) as unknown as IpcMainInvokeEvent
    try {
      expect(await invoke(event({}), saveDraftRequest)).toEqual({ ok: false, code: 'unavailable' })
      visible = false; expect(await invoke(event(), saveDraftRequest)).toEqual({ ok: false, code: 'unavailable' }); visible = true
      expect(transport).not.toHaveBeenCalled()
      const pending = invoke(event(), saveDraftRequest)
      contents.emit('did-start-navigation', {}, 'https://synthetic.invalid', false, true)
      expect(transport.mock.calls[0][2]?.aborted).toBe(true)
      finish(saveDraftResponse()); expect(await pending).toEqual({ ok: false, code: 'unavailable' })
    } finally { contents.emit('destroyed'); window.emit('closed') }
  })
})

describe('Question native source-bound local effects', () => {
  const accepted = () => ({ ...display, callId: 'actual-call', status: 'continued', timedWait: { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' },
    lateReply: { clientUserMessageId: request.viewId, turnId: 'target-turn', state: 'accepted' } })
  const copyRequest = { selector, viewId: request.viewId, effect: { kind: 'copyReply' as const, locale: 'zh' as const,
    clientUserMessageId: request.viewId, expectedTurnId: 'target-turn' } }
  it('copies actual Reply text after two fresh reads without accepting Renderer raw strings', async () => {
    const transport = vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify(accepted()) }))
    const copy = vi.fn(), openLink = vi.fn(async () => undefined)
    const host = new UserInputLocalDisplayHostV1(transport, vi.fn(), { copy, openLink })
    await host.open(request, owner())
    expect(await host.effect(copyRequest, owner())).toEqual({ ok: true })
    expect(transport).toHaveBeenCalledTimes(3)
    expect(copy).toHaveBeenCalledWith('Header — Original question\n回答：original\nanswer')
    expect(openLink).not.toHaveBeenCalled()
    expect(await host.effect({ ...copyRequest, text: 'Renderer injected bytes' }, owner())).toEqual({ ok: false, code: 'unavailable' })
    expect(copy).toHaveBeenCalledOnce()
    host.invalidate()
  })
  it('does not copy a changed digest between authorization reads', async () => {
    let value = accepted()
    const transport = vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify(value) }))
    const copy = vi.fn(), host = new UserInputLocalDisplayHostV1(transport, vi.fn(), { copy, openLink: vi.fn() })
    await host.open(request, owner())
    transport.mockImplementationOnce(async () => ({ ok: true, status: 200, body: JSON.stringify(value) }))
      .mockImplementationOnce(async () => { value = { ...value, questions: [{ ...value.questions[0], question: 'Changed' }] }; return { ok: true, status: 200, body: JSON.stringify(value) } })
    expect(await host.effect(copyRequest, owner())).toEqual({ ok: false, code: 'unavailable' })
    expect(copy).not.toHaveBeenCalled(); host.invalidate()
  })
  it('rejects queued or foreign Reply association and unsafe/source-injected requests', async () => {
    for (const patch of [{ lateReply: { ...accepted().lateReply, state: 'queued' } }, { callId: null }]) {
      const value = { ...accepted(), ...patch }, copy = vi.fn()
      const host = new UserInputLocalDisplayHostV1(async () => ({ ok: true, status: 200, body: JSON.stringify(value) }), vi.fn(), { copy, openLink: vi.fn() })
      await host.open(request, owner())
      expect(await host.effect(copyRequest, owner())).toEqual({ ok: false, code: 'unavailable' })
      expect(copy).not.toHaveBeenCalled(); host.invalidate()
    }
  })
  it('returns unavailable after a link action loses its document while awaiting completion', async () => {
    const detail = '[Open](https://example.test/docs)', value = { ...display, questions: [{ ...display.questions[0], detail }] }
    let finish!: () => void
    const openLink = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
    const host = new UserInputLocalDisplayHostV1(async () => ({ ok: true, status: 200, body: JSON.stringify(value) }), vi.fn(), { copy: vi.fn(), openLink })
    await host.open(request, owner())
    const action = host.effect({ selector, viewId: request.viewId, effect: { kind: 'openDetailLink', questionId: value.questions[0].id, start: 0, end: detail.length } }, owner())
    for (let i = 0; i < 6 && !openLink.mock.calls.length; i++) await Promise.resolve()
    expect(openLink).toHaveBeenCalledWith('https://example.test/docs')
    host.invalidate(); finish()
    expect(await action).toEqual({ ok: false, code: 'unavailable' })
  })
})

describe('admitted private draft across ordinary background', () => {
  const draft = { currentPage: 0, answers: [{ questionId: requestId + '_1', optionIds: [], customText: 'Synthetic latest', skipped: false }] }
  const draftRequest = { ...selector, action: 'saveDraft', revision: request.viewId, expectedRevision: null, draft }
  const receipt = { schemaVersion: 1, ...selector, status: 'pending', draftRevision: request.viewId, draft }
  it.each(['background', 'authority'] as const)('preserves only admitted saves for %s retirement', async reason => {
    let finish!: (value: { ok: boolean; status: number; body: string }) => void
    const host = new UserInputLocalDisplayHostV1(() => new Promise(resolve => { finish = resolve }), vi.fn())
    const document = owner(), saving = host.saveDraft(draftRequest, document)
    host.invalidate(reason); finish({ ok: true, status: 200, body: JSON.stringify(receipt) })
    expect(await saving).toEqual(reason === 'background' ? { ok: true, receipt } : { ok: false, code: 'unavailable' })
    expect(document.release).toHaveBeenCalledOnce()
  })
})

describe('save admission and irreversible native document identity', () => {
  it.each(['before-hide', 'after-admission', 'navigation'] as const)('uses visible admission and current document checks for %s', async phase => {
    vi.mocked(ipcMain.handle).mockClear()
    let visible = true
    const frame = {}, contents = Object.assign(new EventEmitter(), { id: phase === 'before-hide' ? 9051 : phase === 'after-admission' ? 9052 : 9053,
      mainFrame: frame, isDestroyed: () => false, send: vi.fn() })
    const window = Object.assign(new EventEmitter(), { webContents: contents, isDestroyed: () => false,
      isVisible: () => visible, isMinimized: () => false })
    let finish!: (value: { ok: boolean; status: number; body: string }) => void
    const transport = vi.fn(() => new Promise<{ ok: boolean; status: number; body: string }>(resolve => { finish = resolve }))
    registerUserInputLocalDisplayIpcV1(() => window as unknown as BrowserWindow, transport)
    const handler = vi.mocked(ipcMain.handle).mock.calls.find(call => call[0] === 'user-input-local:save-draft')![1]
    const draft = { currentPage: 0, answers: [{ questionId: requestId + '_1', optionIds: [], customText: 'Synthetic admitted', skipped: false }] }
    const payload = { ...selector, action: 'saveDraft', revision: request.viewId, expectedRevision: null, draft }
    try {
      if (phase === 'before-hide') visible = false
      const pending = handler({ sender: contents, senderFrame: frame } as unknown as IpcMainInvokeEvent, payload)
      if (phase === 'before-hide') {
        expect(await pending).toEqual({ ok: false, code: 'unavailable' }); expect(transport).not.toHaveBeenCalled()
      } else {
        expect(transport).toHaveBeenCalledOnce()
        visible = false; window.emit('hide')
        if (phase === 'navigation') contents.emit('did-start-navigation', {}, 'about:blank', false, true)
        const receipt = { schemaVersion: 1, ...selector, status: 'pending', draftRevision: request.viewId, draft }
        finish({ ok: true, status: 200, body: JSON.stringify(receipt) })
        expect(await pending).toEqual(phase === 'after-admission' ? { ok: true, receipt } : { ok: false, code: 'unavailable' })
      }
    } finally { contents.emit('destroyed'); window.emit('closed') }
  })
})

describe('Question inline URL native eligibility', () => {
  it.each([
    { url: 'https:example.test/docs', destination: 'https://example.test/docs' },
    { url: 'HTTP:example.test/docs', destination: 'http://example.test/docs' },
    { url: 'https://example.test/docs', destination: 'https://example.test/docs' },
    { url: 'mailto:synthetic@example.test', destination: null },
    { url: '/fixture/file.md', destination: null },
    { url: 'javascript:alert(1)', destination: null }
  ])('derives only the eligible HTTP(S) destination from the Host source $url', async ({ url, destination }) => {
    const tick = String.fromCharCode(96), detail = tick + url + tick
    const value = { ...display, questions: [{ ...display.questions[0], detail }] }
    const openLink = vi.fn(async (_url: string) => undefined)
    const host = new UserInputLocalDisplayHostV1(async () => ({ ok: true, status: 200, body: JSON.stringify(value) }), vi.fn(), { copy: vi.fn(), openLink })
    try {
      await host.open(request, owner())
      expect(await host.effect({ selector, viewId: request.viewId, effect: { kind: 'openDetailLink', questionId: value.questions[0].id, start: 0, end: detail.length } }, owner()))
        .toEqual(destination === null ? { ok: false, code: 'unavailable' } : { ok: true })
      if (destination === null) expect(openLink).not.toHaveBeenCalled()
      else expect(openLink).toHaveBeenCalledWith(destination)
    } finally { host.invalidate() }
  })
})


describe('readonly accepted history native Copy', () => {
 const value = () => ({ ...display, callId: 'actual-call', status: 'continued', readOnly: true,
   historyReply: { itemId: 'actual-child-reply', clientUserMessageId: request.viewId, turnId: 'actual-reply-turn' } })
 const copyRequest = { selector, viewId: request.viewId, effect: { kind: 'copyReply' as const, locale: 'zh' as const,
   clientUserMessageId: request.viewId, expectedTurnId: 'actual-reply-turn' } }
 it('copies the same original format with two fresh document reads', async () => {
   const transport = vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify(value()) }))
   const copy = vi.fn(), host = new UserInputLocalDisplayHostV1(transport, vi.fn(), { copy, openLink: vi.fn() })
   await host.open(request, owner())
   expect(await host.effect(copyRequest, owner())).toEqual({ ok: true })
   expect(copy).toHaveBeenCalledWith('Header — Original question\n回答：original\nanswer')
   expect(transport).toHaveBeenCalledTimes(3); host.invalidate()
 })
 it.each(['itemId', 'clientUserMessageId', 'turnId'] as const)('rejects %s drift in either fresh read', async field => {
   for (const changeAt of [2, 3]) {
     let reads = 0
     const initial = value(), changed = { ...initial, historyReply: { ...initial.historyReply,
       [field]: field === 'clientUserMessageId' ? '00000000-0000-4000-8000-000000000001' : 'changed-valid-id' } }
     const transport = vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify(++reads >= changeAt ? changed : initial) }))
     const copy = vi.fn(), host = new UserInputLocalDisplayHostV1(transport, vi.fn(), { copy, openLink: vi.fn() })
     await host.open(request, owner())
     expect(await host.effect(copyRequest, owner())).toEqual({ ok: false, code: 'unavailable' })
     expect(copy).not.toHaveBeenCalled(); host.invalidate()
   }
 })
 it('requires the exact association even for a readonly view', async () => {
   const copy = vi.fn(), host = new UserInputLocalDisplayHostV1(async () => ({ ok: true, status: 200, body: JSON.stringify({ ...value(), historyReply: null }) }), vi.fn(), { copy, openLink: vi.fn() })
   await host.open(request, owner())
   expect(await host.effect(copyRequest, owner())).toEqual({ ok: false, code: 'unavailable' }); expect(copy).not.toHaveBeenCalled(); host.invalidate()
 })
})


it('consumes actual Go accepted history HTTP through Main Copy without widening writes', async () => {
 const temp = mkdtempSync(join(tmpdir(), 'analytix-history-main-')), witness = join(temp, 'display.json')
 try {
   const execution = spawnSync('go', ['test', '-mod=readonly', '-tags', 'analytix_prod', '-count=1', '-run', '^TestPrivateQuestionDiscardActualQueueAndPromotionConflict$', './internal/server'], {
     cwd: join(process.cwd(), 'packages/runtime-go'), env: { ...process.env, ANALYTIX_HISTORY_MAIN_WITNESS_FILE: witness }, encoding: 'utf8', timeout: 240000, maxBuffer: 1 << 20
   })
   expect(execution.status, `${execution.error ?? ''}${execution.stdout ?? ''}${execution.stderr ?? ''}`.slice(-4000)).toBe(0)
   expect(execution.signal).toBeNull()
   const body = readFileSync(witness, 'utf8'), actual = userInputLocalDisplaySchemaV1.parse(JSON.parse(body))
   expect(actual.readOnly).toBe(true); expect(actual.historyReply).not.toBeNull(); expect(actual.lateReply).toBeNull()
   const bound = { threadId: actual.threadId, turnId: actual.turnId, itemId: actual.itemId, requestId: actual.requestId }
   const opened = { ...bound, viewId: request.viewId }, copy = vi.fn()
   const transport = vi.fn(async () => ({ ok: true, status: 200, body }))
   const host = new UserInputLocalDisplayHostV1(transport, vi.fn(), { copy, openLink: vi.fn() })
   expect((await host.open(opened, owner())).ok).toBe(true)
   expect(await host.effect({ selector: bound, viewId: opened.viewId, effect: { kind: 'copyReply', locale: 'en', clientUserMessageId: actual.historyReply!.clientUserMessageId, expectedTurnId: actual.historyReply!.turnId } }, owner())).toEqual({ ok: true })
   expect(copy).toHaveBeenCalledWith('Question 1 — What should happen next?\nAnswer: ordinary synthetic answer')
   expect(transport).toHaveBeenCalledTimes(3)
   expect(await host.saveDraft({ ...bound, action: 'saveDraft', revision: '00000000-0000-4000-8000-000000000093', expectedRevision: null, draft: { currentPage: 0, answers: [] } }, owner())).toEqual({ ok: false, code: 'unavailable' })
   await expect(host.discard({ ...bound, action: 'discardLateReply', clientUserMessageId: actual.historyReply!.clientUserMessageId, expectedTurnId: actual.historyReply!.turnId }, owner())).rejects.toThrow('Question discard unavailable.')
   expect(transport).toHaveBeenCalledTimes(3); host.invalidate()
 } finally { rmSync(temp, { recursive: true, force: true }) }
}, 300000)


describe('private user input read diagnostics', () => {
  it('records rejected native authority without invoking the private transport',async()=>{
    vi.mocked(logUserInputLocalReadFailure).mockClear();vi.mocked(ipcMain.handle).mockClear()
    const frame={},contents={id:987,mainFrame:frame,isDestroyed:()=>false}
    const window={webContents:contents,isDestroyed:()=>false,isVisible:()=>false,isMinimized:()=>false}
    const transport=vi.fn(async()=>response())
    registerUserInputLocalDisplayIpcV1(()=>window as unknown as BrowserWindow,transport)
    const handler=vi.mocked(ipcMain.handle).mock.calls.find(call=>call[0]==='user-input-local:open')![1]
    expect(await handler({sender:contents,senderFrame:frame} as unknown as IpcMainInvokeEvent,request)).toEqual({ok:false,code:'unavailable'})
    expect(transport).not.toHaveBeenCalled()
    expect(logUserInputLocalReadFailure).toHaveBeenCalledExactlyOnceWith('ipc_authority',null)
  })
  it.each(['runtime_request','response_owner','response_http','response_limit','response_json','response_schema','response_selector','display_owner'] as const)('records only the first %s failure and preserves retirement', async stage => {
    vi.mocked(logUserInputLocalReadFailure).mockClear()
    let current=true,checks=0
    const document={...owner(),isCurrent:()=>stage==='display_owner'?++checks<3:current}
    const transport=vi.fn(async()=>{
      if(stage==='runtime_request') throw Error('PRIVATE_QUESTION_BODY_SENTINEL')
      if(stage==='response_owner') current=false
      if(stage==='response_http') return {ok:false,status:403,body:'PRIVATE_QUESTION_BODY_SENTINEL'}
      if(stage==='response_limit') return {ok:true,status:200,body:'x'.repeat(USER_INPUT_LOCAL_DISPLAY_MAX_BYTES_V1+1)}
      if(stage==='response_json') return {ok:true,status:200,body:'PRIVATE_QUESTION_BODY_SENTINEL'}
      if(stage==='response_schema') return {ok:true,status:200,body:JSON.stringify({schemaVersion:99,privateBody:'PRIVATE_QUESTION_BODY_SENTINEL'})}
      if(stage==='response_selector') return {ok:true,status:200,body:JSON.stringify({...display,threadId:'other'})}
      return response()
    })
    const notify=vi.fn(),host=new UserInputLocalDisplayHostV1(transport,notify)
    expect(await host.open(request,document)).toEqual({ok:false,code:'unavailable'})
    expect(logUserInputLocalReadFailure).toHaveBeenCalledExactlyOnceWith(stage,stage==='runtime_request'?null:stage==='response_http'?403:200)
    expect(document.release).toHaveBeenCalledOnce()
    expect(notify).toHaveBeenCalledWith(request.viewId)
    expect(JSON.stringify(vi.mocked(logUserInputLocalReadFailure).mock.calls)).not.toContain('PRIVATE_QUESTION_BODY_SENTINEL')
  })
  it('keeps invalid request and owner admission closed before transport',async()=>{
    vi.mocked(logUserInputLocalReadFailure).mockClear()
    const transport=vi.fn(async()=>response()),host=new UserInputLocalDisplayHostV1(transport,vi.fn())
    expect(await host.open({...request,privateBody:'PRIVATE_QUESTION_BODY_SENTINEL'},owner())).toEqual({ok:false,code:'unavailable'})
    expect(await host.open(request,{...owner(),isCurrent:()=>false})).toEqual({ok:false,code:'unavailable'})
    expect(transport).not.toHaveBeenCalled()
    expect(vi.mocked(logUserInputLocalReadFailure).mock.calls).toEqual([['request_admission',null],['owner_admission',null]])
  })
  it('emits no failure when the original question is admitted',async()=>{
    vi.mocked(logUserInputLocalReadFailure).mockClear()
    const document=owner(),host=new UserInputLocalDisplayHostV1(async()=>response(),vi.fn())
    expect(await host.open(request,document)).toEqual({ok:true,display})
    expect(logUserInputLocalReadFailure).not.toHaveBeenCalled()
    host.close(request.viewId)
  })
})
