// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type {
  UserInputLocalDisplayV1, UserInputLocalDraftSnapshotV1, UserInputLocalSaveDraftRequestV1,
  UserInputLocalSelectorV1
} from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import type { UserInputDraftSaveResultV1 } from '../../../../../../shared/analytix-api'
import { useUserInputPrivateDraft, type UserInputPrivateDraftProps } from './useUserInputPrivateDraft'

const requestId = 'input_' + 'a'.repeat(64)
const selector: UserInputLocalSelectorV1 = { threadId: 'thread-a', turnId: 'turn-a', itemId: 'item_' + requestId, requestId }
const revisionA = '11111111-1111-4111-8111-111111111111'
const revisionB = '22222222-2222-4222-8222-222222222222'
function snapshot(text: string, currentPage = 0): UserInputLocalDraftSnapshotV1 {
  return { currentPage, answers: [{ questionId: requestId + '_1', optionIds: [], customText: text, skipped: false }] }
}
function display(cursor: string | null = null, draft: UserInputLocalDraftSnapshotV1 | null = null,
  target = selector): UserInputLocalDisplayV1 {
  return { schemaVersion: 1, ...target, status: 'pending', answerState: 'pending', draftRevision: cursor, draft,
    questions: [1, 2, 3].map(index => ({ id: target.requestId + '_' + index, header: 'Synthetic header',
      question: 'Synthetic original question', multiSelect: true,
      options: [1, 2, 3].map(option => ({ id: target.requestId + '_' + index + '_option_' + option,
        label: 'Synthetic original option', description: '' })) })) }
}
function success(request: UserInputLocalSaveDraftRequestV1): UserInputDraftSaveResultV1 {
  return { ok: true, receipt: { schemaVersion: 1, threadId: request.threadId, turnId: request.turnId,
    itemId: request.itemId, requestId: request.requestId, status: 'pending', draftRevision: request.revision,
    draft: request.draft } }
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(finish => { resolve = finish })
  return { promise, resolve }
}
let root: Root, container: HTMLDivElement, mounted: boolean
let previousApi: PropertyDescriptor | undefined
let save: ReturnType<typeof vi.fn<(request: UserInputLocalSaveDraftRequestV1) => Promise<UserInputDraftSaveResultV1>>>
let refresh: ReturnType<typeof vi.fn<() => void>>
let restore: ReturnType<typeof vi.fn<(value: UserInputLocalDraftSnapshotV1) => void>>
let interruptionMetadata: Map<string, { revision: string | null; unsent: boolean }>
let props: UserInputPrivateDraftProps
let current: ReturnType<typeof useUserInputPrivateDraft>
function Probe(input: UserInputPrivateDraftProps) {
  current = useUserInputPrivateDraft(input)
  return createElement('p', { 'data-phase': current.state.phase }, current.state.code ?? '')
}
async function render(next: Partial<UserInputPrivateDraftProps> = {}) {
  props = { ...props, ...next }
  await act(async () => root.render(createElement(Probe, props)))
}
async function change(value: UserInputLocalDraftSnapshotV1) {
  await act(async () => current.change(value))
}
async function flush() { await act(async () => current.flush()) }
async function advance(milliseconds: number) {
  await act(async () => vi.advanceTimersByTimeAsync(milliseconds))
}
beforeEach(() => {
  vi.useFakeTimers(); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  previousApi = Object.getOwnPropertyDescriptor(window, 'analytix')
  save = vi.fn(async (request: UserInputLocalSaveDraftRequestV1): Promise<UserInputDraftSaveResultV1> => success(request))
  refresh = vi.fn<() => void>(); restore = vi.fn<(value: UserInputLocalDraftSnapshotV1) => void>()
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: { saveUserInputDraft: save } } })
  interruptionMetadata = new Map()
  props = { selector, display: display(), editable: true, presented: true, privacyRevision: 0, refresh, onRestore: restore,
    readInterruption: key => interruptionMetadata.get(key) ?? null,
    recordInterruption: (key, value) => { if (value === null) interruptionMetadata.delete(key); else interruptionMetadata.set(key, value) } }
  container = document.createElement('div'); document.body.append(container); root = createRoot(container); mounted = true
})
afterEach(async () => {
  if (mounted) await act(async () => root.unmount())
  container.remove(); vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals()
  if (previousApi) Object.defineProperty(window, 'analytix', previousApi)
  else Reflect.deleteProperty(window, 'analytix')
})

describe('private unfinished Question snapshot owner', () => {
  it('restores an actual empty draft and page without saving initial or older-producer content', async () => {
    const actual: UserInputLocalDraftSnapshotV1 = { currentPage: 2, answers: [] }
    await render({ display: display(revisionA, actual) })
    expect(restore).toHaveBeenCalledWith(actual)
    await advance(5000); expect(save).not.toHaveBeenCalled()
    const older = display(); Reflect.deleteProperty(older, 'draftRevision'); Reflect.deleteProperty(older, 'draft')
    await render({ display: older }); await change(snapshot('Synthetic unsupported edit')); await advance(5000)
    expect(save).not.toHaveBeenCalled(); expect(current.state).toEqual({ phase: 'idle', code: null, canRetry: false })
  })

  it('retains an independent non-null cursor after logical draft clearing and an empty flush', async () => {
    await render({ display: display(revisionA, null) }); await flush()
    await change(snapshot('  Synthetic unfinished text  ')); await advance(749)
    expect(save).toHaveBeenCalledOnce()
    expect(save.mock.calls[0][0]).toMatchObject({ ...selector, action: 'saveDraft', expectedRevision: revisionA,
      draft: snapshot('  Synthetic unfinished text  ') })
    expect(current.state.phase).toBe('saved')
  })

  it('admits the first edit promptly and coalesces later edits while one request is in flight', async () => {
    const first = deferred<UserInputDraftSaveResultV1>(); save.mockImplementationOnce(() => first.promise)
    await render(); await change(snapshot('Synthetic edit 0'))
    expect(save).toHaveBeenCalledOnce()
    const request = save.mock.calls[0][0]
    for (let index = 1; index <= 5; index++) { await advance(500); await change(snapshot('Synthetic edit ' + index)) }
    await advance(500); expect(save).toHaveBeenCalledOnce()
    await act(async () => first.resolve(success(request)))
    expect(save).toHaveBeenCalledTimes(2); expect(save.mock.calls[1][0].draft).toEqual(snapshot('Synthetic edit 5'))
    expect(save.mock.calls[1][0].expectedRevision).toBe(request.revision)
    await advance(5000); expect(save).toHaveBeenCalledTimes(2)
  })

  it('pauses a due visible edit throughout loading-null, then uses the fresh Host cursor', async () => {
    await render({ display: display(revisionA) }); await change(snapshot('Synthetic first saved'))
    await change(snapshot('Synthetic paused edit'))
    await render({ display: null }); await advance(5000); await flush()
    expect(save).toHaveBeenCalledOnce()
    await render({ display: display(revisionB, null) })
    expect(save).toHaveBeenCalledTimes(2)
    expect(save.mock.calls[1][0]).toMatchObject({ expectedRevision: revisionB, draft: snapshot('Synthetic paused edit') })
    expect(current.state.phase).toBe('saved')
  })

  it('keeps an unknown exact attempt paused while visible readback is null', async () => {
    save.mockResolvedValueOnce({ ok: false, code: 'unavailable' })
    await render({ display: display(revisionA) }); await change(snapshot('Synthetic unknown paused')); await flush()
    const attempt = save.mock.calls[0][0]
    await render({ display: null }); expect(current.state.canRetry).toBe(false)
    await act(async () => current.retry()); await advance(5000); expect(save).toHaveBeenCalledOnce()
    await render({ display: display(revisionB, null) }); await act(async () => current.retry())
    expect(save).toHaveBeenCalledTimes(2); expect(save.mock.calls[1][0]).toEqual(attempt)
  })

  it('flushes a named hide with loading-null without converting it into privacy retirement', async () => {
    await render({ display: display(revisionA) }); await change(snapshot('Synthetic hidden edit'))
    await render({ display: null, presented: false })
    expect(save).toHaveBeenCalledOnce()
    expect(save.mock.calls[0][0]).toMatchObject({ expectedRevision: revisionA, draft: snapshot('Synthetic hidden edit') })
  })

  it('holds one in-flight request and one copied coalesced snapshot with original answer and option order', async () => {
    const first = deferred<UserInputDraftSaveResultV1>(); save.mockImplementationOnce(() => first.promise)
    await render(); await change(snapshot('Synthetic first')); await flush()
    const firstRequest = save.mock.calls[0][0]
    await change(snapshot('Synthetic intermediate'))
    const latest: UserInputLocalDraftSnapshotV1 = { currentPage: 2, answers: [
      { questionId: requestId + '_3', optionIds: [], customText: '', skipped: true },
      { questionId: requestId + '_1', optionIds: [requestId + '_1_option_3', requestId + '_1_option_1'],
        customText: '  Synthetic final  ', skipped: false }
    ] }
    const expected = JSON.parse(JSON.stringify(latest)) as UserInputLocalDraftSnapshotV1
    await change(latest); latest.answers[1].customText = 'Synthetic caller mutation'; latest.answers[1].optionIds.reverse()
    await advance(750); expect(save).toHaveBeenCalledOnce()
    await act(async () => first.resolve(success(firstRequest)))
    expect(save).toHaveBeenCalledTimes(2)
    expect(save.mock.calls[1][0].draft).toEqual(expected)
    expect(save.mock.calls[1][0].expectedRevision).toBe(firstRequest.revision)
    expect(save.mock.calls[1][0].revision).not.toBe(firstRequest.revision)
    expect(current.state.phase).toBe('saved')
  })

  it('rereads after an unknown ACK and retries its exact UUID, parent and snapshot before the coalesced edit', async () => {
    save.mockResolvedValueOnce({ ok: false, code: 'unavailable' })
    await render({ display: display(revisionA) }); await change(snapshot('  Synthetic uncertain  ')); await flush()
    const uncertain = save.mock.calls[0][0]
    expect(refresh).toHaveBeenCalledOnce(); expect(current.state).toMatchObject({ code: 'unavailable', canRetry: true })
    await change(snapshot('Synthetic next')); await render({ display: null }); await advance(5000)
    expect(save).toHaveBeenCalledOnce()
    await render({ display: display(revisionA) }); await act(async () => current.retry())
    expect(save).toHaveBeenCalledTimes(3)
    expect(save.mock.calls[1][0]).toEqual(uncertain)
    expect(save.mock.calls[2][0].draft).toEqual(snapshot('Synthetic next'))
    expect(save.mock.calls[2][0].expectedRevision).toBe(uncertain.revision)
    expect(save.mock.calls[2][0].revision).not.toBe(uncertain.revision)
  })

  it('treats a foreign ACK as unavailable and keeps the exact attempt for manual retry', async () => {
    save.mockImplementationOnce(async request => {
      const result = success(request)
      if (result.ok) result.receipt.turnId = 'turn-foreign'
      return result
    })
    await render(); await change(snapshot('Synthetic original')); await flush()
    const original = save.mock.calls[0][0]
    expect(current.state.code).toBe('unavailable'); expect(refresh).toHaveBeenCalledOnce()
    await advance(5000); expect(save).toHaveBeenCalledOnce()
    await act(async () => current.retry())
    expect(save.mock.calls[1][0]).toEqual(original); expect(current.state.phase).toBe('saved')
  })

  it('retires a 404 UUID and waits for actual readback before a new edit uses a fresh UUID', async () => {
    save.mockResolvedValueOnce({ ok: false, code: 'retired' })
    await render(); await change(snapshot('Synthetic retired')); await flush()
    const retired = save.mock.calls[0][0]
    expect(refresh).toHaveBeenCalledOnce(); expect(current.state).toMatchObject({ code: 'retired', canRetry: false })
    await act(async () => current.retry()); await advance(5000); expect(save).toHaveBeenCalledOnce()
    await change(snapshot('Synthetic new causal edit')); await flush(); expect(save).toHaveBeenCalledOnce()
    await render({ display: display(revisionB, null) })
    expect(save).toHaveBeenCalledTimes(2)
    expect(save.mock.calls[1][0]).toMatchObject({ expectedRevision: revisionB, draft: snapshot('Synthetic new causal edit') })
    expect(save.mock.calls[1][0].revision).not.toBe(retired.revision)
  })

  it.each(['conflict', 'invalid', 'capacity'] as const)('requires new actual readback and explicit retry after %s', async code => {
    save.mockResolvedValueOnce({ ok: false, code })
    await render(); await change(snapshot('Synthetic rejected')); await flush()
    const rejected = save.mock.calls[0][0]
    expect(refresh).toHaveBeenCalledOnce(); expect(current.state).toMatchObject({ code, canRetry: false })
    await act(async () => current.retry()); await advance(5000); expect(save).toHaveBeenCalledOnce()
    await render({ display: display(revisionB, null) })
    expect(current.state.canRetry).toBe(true); expect(save).toHaveBeenCalledOnce()
    await act(async () => current.retry())
    expect(save).toHaveBeenCalledTimes(2)
    expect(save.mock.calls[1][0].expectedRevision).toBe(revisionB)
    expect(save.mock.calls[1][0].revision).not.toBe(rejected.revision)
    expect(save.mock.calls[1][0].draft).toEqual(rejected.draft)
  })

  it('does not impose an asynchronous save prerequisite when retiring for ordinary answer capture at capacity', async () => {
    save.mockResolvedValueOnce({ ok: false, code: 'capacity' })
    await render(); await change(snapshot('Synthetic capacity')); await flush()
    expect(current.state.code).toBe('capacity')
    await act(async () => { expect(current.retire()).toBeUndefined() })
    expect(current.state).toEqual({ phase: 'idle', code: null, canRetry: false })
    await advance(5000); expect(save).toHaveBeenCalledOnce()
  })

  it('does not let a pre-save read arriving after the signed ACK restore its old parent cursor or content', async () => {
    await render({ display: display(revisionA, snapshot('Synthetic old Host draft')) })
    await change(snapshot('Synthetic saved human draft')); await flush()
    const saved = save.mock.calls[0][0]
    restore.mockClear()
    await render({ display: display(revisionA, snapshot('Synthetic stale parent content')) })
    expect(restore).not.toHaveBeenCalled(); expect(refresh).toHaveBeenCalledOnce()
    await render({ display: display(revisionA, snapshot('Synthetic stale parent content')) })
    expect(refresh).toHaveBeenCalledOnce(); expect(restore).not.toHaveBeenCalled()
    await render({ display: display(saved.revision, saved.draft) })
    expect(restore).toHaveBeenCalledWith(saved.draft)
    await change(snapshot('Synthetic next saved edit')); await flush()
    expect(save.mock.calls[1][0].expectedRevision).toBe(saved.revision)
  })

  it('settles a superseded exact ACK without replacing a newer actual Host cursor or content', async () => {
    const first = deferred<UserInputDraftSaveResultV1>(); save.mockImplementationOnce(() => first.promise)
    await render({ display: display(revisionA) }); await change(snapshot('Synthetic old')); await flush()
    const old = save.mock.calls[0][0], actual = snapshot('Synthetic Host current', 1)
    await render({ display: display(revisionB, actual) }); expect(restore).not.toHaveBeenCalled()
    await act(async () => first.resolve(success(old)))
    expect(restore).toHaveBeenLastCalledWith(actual); expect(current.state.phase).toBe('idle')
    await change(snapshot('Synthetic following')); await flush()
    expect(save.mock.calls[1][0].expectedRevision).toBe(revisionB)
    expect(restore).not.toHaveBeenCalledWith(old.draft)
  })

  it('requires explicit retry for coalesced content after an ACK settles against a newer actual head', async () => {
    const first = deferred<UserInputDraftSaveResultV1>(); save.mockImplementationOnce(() => first.promise)
    await render(); await change(snapshot('Synthetic old')); await flush()
    const old = save.mock.calls[0][0]
    await change(snapshot('Synthetic coalesced')); await advance(750); await render({ display: display(revisionB) })
    await act(async () => first.resolve(success(old)))
    expect(save).toHaveBeenCalledOnce(); expect(current.state).toMatchObject({ code: 'conflict', canRetry: true })
    await act(async () => current.retry())
    expect(save.mock.calls[1][0]).toMatchObject({ expectedRevision: revisionB, draft: snapshot('Synthetic coalesced') })
  })

  it('flushes normal presentation hide while privacy retirement removes pending bytes and ignores its old ACK', async () => {
    await render(); await change(snapshot('Synthetic hide edit')); await render({ presented: false })
    expect(save).toHaveBeenCalledOnce()
    const old = deferred<UserInputDraftSaveResultV1>(); save.mockImplementationOnce(() => old.promise)
    await render({ presented: true }); await change(snapshot('Synthetic private old')); await flush()
    const oldRequest = save.mock.calls[1][0]
    await change(snapshot('Synthetic private queued'))
    await render({ privacyRevision: 1, display: null }); await advance(5000)
    await act(async () => old.resolve(success(oldRequest)))
    expect(save).toHaveBeenCalledTimes(2); expect(restore).not.toHaveBeenCalled()
    expect(current.state.phase).toBe('idle')
    await render({ display: display(revisionB) }); await change(snapshot('Synthetic private new')); await flush()
    expect(save.mock.calls[2][0].expectedRevision).toBe(revisionB)
  })

  it.each(['owner', 'capture', 'unmount'] as const)('retires pending work and ignores a late ACK on %s', async transition => {
    const old = deferred<UserInputDraftSaveResultV1>(); save.mockImplementationOnce(() => old.promise)
    await render(); await change(snapshot('Synthetic retiring')); await flush(); const oldRequest = save.mock.calls[0][0]
    await change(snapshot('Synthetic unsent'))
    if (transition === 'owner') {
      const target = { ...selector, threadId: 'thread-b' }
      await render({ selector: target, display: display(revisionB, null, target) })
    } else if (transition === 'capture') await render({ editable: false })
    else { await act(async () => root.unmount()); mounted = false }
    await advance(5000); await act(async () => old.resolve(success(oldRequest)))
    expect(save).toHaveBeenCalledOnce(); expect(restore).not.toHaveBeenCalled()
    if (transition === 'owner') {
      await change(snapshot('Synthetic new owner')); await flush()
      expect(save.mock.calls[1][0]).toMatchObject({ threadId: 'thread-b', expectedRevision: revisionB })
    }
  })

  it.each(['privacy', 'owner', 'capture', 'unmount', 'explicit'] as const)('drops the unsent debounce snapshot on %s', async transition => {
    await render(); await change(snapshot('Synthetic initial saved')); save.mockClear()
    await change(snapshot('Synthetic unsent private edit'))
    if (transition === 'privacy') await render({ privacyRevision: 1, display: null })
    else if (transition === 'owner') {
      const target = { ...selector, threadId: 'thread-b' }
      await render({ selector: target, display: display(null, null, target) })
    } else if (transition === 'capture') await render({ editable: false })
    else if (transition === 'explicit') await act(async () => current.retire())
    else { await act(async () => root.unmount()); mounted = false }
    await advance(5000); expect(save).not.toHaveBeenCalled()
  })

  it('rejects an invalid local snapshot without dispatch and permits a corrected actual human edit', async () => {
    await render(); await change({ ...snapshot('Synthetic invalid'), currentPage: 3 })
    expect(save).not.toHaveBeenCalled(); expect(current.state).toMatchObject({ code: 'invalid', canRetry: false })
    await change(snapshot('Synthetic corrected')); await advance(750)
    expect(save).toHaveBeenCalledOnce(); expect(current.state.phase).toBe('saved')
  })
})

describe('draft retirement recovery and named Hide', () => {
  it('clears background raw state, restores a fresh accepted cursor, and never applies the old ACK', async () => {
    const first = deferred<UserInputDraftSaveResultV1>(); save.mockImplementationOnce(() => first.promise)
    await render(); await change(snapshot('Synthetic immediately admitted'))
    expect(save).toHaveBeenCalledOnce()
    const request = save.mock.calls[0][0]
    await render({ privacyRevision: 1, retirementReason: 'background', display: null })
    expect(current.interrupted).toBe(true); expect(current.state.phase).toBe('idle')
    await act(async () => first.resolve(success(request)))
    expect(restore).not.toHaveBeenCalled()
    await render({ display: display(request.revision, request.draft) })
    expect(current.interrupted).toBe(false)
    expect(restore).toHaveBeenCalledWith(request.draft)
    expect(save).toHaveBeenCalledOnce()
  })
  it('keeps an explicit warning for a newer unadmitted edit after a fresh read succeeds', async () => {
    await render(); await change(snapshot('Synthetic saved'))
    const saved = save.mock.calls[0][0]
    await change(snapshot('Synthetic not yet admitted'))
    await render({ privacyRevision: 1, retirementReason: 'background', display: null })
    await render({ display: display(saved.revision, saved.draft) })
    expect(current.interrupted).toBe(true)
    expect(restore).toHaveBeenCalledWith(saved.draft)
    await advance(5000); expect(save).toHaveBeenCalledOnce()
    await render({ privacyRevision: 2, retirementReason: 'authority', display: null })
    expect(current.interrupted).toBe(false)
  })
  it('keeps the latest coalesced child chain in the same leaf after immediate panel Hide', async () => {
    const first = deferred<UserInputDraftSaveResultV1>(), second = deferred<UserInputDraftSaveResultV1>()
    save.mockImplementationOnce(() => first.promise).mockImplementationOnce(() => second.promise)
    await render(); await change(snapshot('Synthetic first'))
    const initial = save.mock.calls[0][0]
    await change(snapshot('Synthetic latest')); await flush()
    await render({ presented: false, display: null })
    expect(save).toHaveBeenCalledOnce(); expect(current.state.phase).toBe('saving')
    await act(async () => first.resolve(success(initial)))
    expect(save).toHaveBeenCalledTimes(2)
    const latest = save.mock.calls[1][0]
    expect(latest.draft).toEqual(snapshot('Synthetic latest'))
    expect(latest.expectedRevision).toBe(initial.revision)
    await act(async () => second.resolve(success(latest)))
    expect(current.state.phase).toBe('saved')
    await render({ presented: true, display: display(latest.revision, latest.draft) })
    expect(restore).toHaveBeenLastCalledWith(latest.draft)
  })
  it('keeps a hung save independent of panel dismissal until real background retirement', async () => {
    save.mockImplementationOnce(() => new Promise(() => undefined))
    await render(); await change(snapshot('Synthetic uncertain')); await flush()
    await render({ presented: false, display: null }); await advance(5000)
    expect(save).toHaveBeenCalledOnce(); expect(current.state.phase).toBe('saving')
    await render({ privacyRevision: 1, retirementReason: 'background' })
    expect(current.state.phase).toBe('idle'); expect(current.interrupted).toBe(true)
  })
})

it('retains real retired-save feedback and interrupted metadata across background', async () => {
  save.mockResolvedValueOnce({ ok: false, code: 'retired' })
  await render(); await change(snapshot('Synthetic rejected draft'))
  expect(current.state.code).toBe('retired')
  await render({ privacyRevision: 1, retirementReason: 'background', display: null })
  expect(current.interrupted).toBe(true)
})

it('keeps only unconfirmed metadata across unmount and a new leaf, until a current saved edit replaces it', async () => {
  await render(); await change(snapshot('Synthetic saved before detach'))
  const saved = save.mock.calls[0][0]
  await change(snapshot('Synthetic latest unsent at detach'))
  await act(async () => root.render(null))
  expect(interruptionMetadata.get(JSON.stringify(selector))).toEqual({ revision: null, unsent: true })
  expect(JSON.stringify([...interruptionMetadata])).not.toContain('Synthetic')
  await render({ display: display(saved.revision, saved.draft) })
  expect(current.interrupted).toBe(true); expect(restore).toHaveBeenLastCalledWith(saved.draft)
  await change(snapshot('Synthetic replacement saved'))
  expect(current.interrupted).toBe(false); expect(interruptionMetadata.size).toBe(0)
})

it('reconciles only a matching actual Host revision after an admitted save outlives the old leaf', async () => {
  const pending = deferred<UserInputDraftSaveResultV1>(); save.mockImplementationOnce(() => pending.promise)
  await render(); await change(snapshot('Synthetic admitted before detach'))
  const admitted = save.mock.calls[0][0]
  await act(async () => root.render(null))
  expect(interruptionMetadata.get(JSON.stringify(selector))).toEqual({ revision: admitted.revision, unsent: false })
  await act(async () => pending.resolve(success(admitted)))
  expect(refresh).not.toHaveBeenCalled(); expect(restore).not.toHaveBeenCalled()
  await render({ display: display(revisionA, snapshot('Synthetic older Host draft')) })
  expect(current.interrupted).toBe(true)
  await render({ display: display(admitted.revision, admitted.draft) })
  expect(current.interrupted).toBe(false); expect(interruptionMetadata.size).toBe(0)
})

it('does not recreate interruption metadata when a live native revoke and unmount share one commit', async () => {
  let revoked = false
  await render({ isAuthorityRetired: () => revoked }); await change(snapshot('Synthetic first saved'))
  await change(snapshot('Synthetic unsent at actual revoke'))
  revoked = true
  await act(async () => root.render(null))
  expect(interruptionMetadata.size).toBe(0)
  await render({ display: display(revisionA, snapshot('Synthetic fresh Host only')), isAuthorityRetired: () => false })
  expect(current.interrupted).toBe(false)
})


it.each(['cancelled', 'submitted', 'continued'] as const)('does not create a draft writer for Host readOnly %s even when a caller requests editing', async status => {
  const historical: UserInputLocalDisplayV1 = { ...display(), readOnly: true, status,
    answerState: status === 'cancelled' ? 'cancelled' : 'unavailable', timedWait: null, continuedReason: null, lateReply: null }
  await render({ display: historical, editable: true })
  await change(snapshot('Synthetic forbidden historical edit')); await flush(); await advance(5000)
  await act(async () => current.retry())
  await render({ presented: false }); await flush()
  expect(save).not.toHaveBeenCalled(); expect(restore).not.toHaveBeenCalled()
  expect(current.state.canRetry).toBe(false)
})
