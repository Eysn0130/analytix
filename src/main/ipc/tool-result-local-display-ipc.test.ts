import { describe, expect, it, vi } from 'vitest'
vi.mock('electron', () => ({ clipboard: {}, dialog: {}, ipcMain: {} }))
import { ToolResultLocalDisplayHostV1 } from './tool-result-local-display-ipc'
const selector = { threadId: 'thread-a', turnId: 'turn-a', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const request = { ...selector, viewId: '11111111-1111-4111-8111-111111111111' }
function fixture(body = 'ordinary\r\n') {
  const display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'bash', capture: { kind: 'shell', status: 'completed', body, label: 'printf', exitCode: 0, durationMs: 1, truncated: false, startLine: 0, endLine: 0, totalLines: 0 } }
  const transport = vi.fn(async (_path: string, _body: string) => ({ ok: true, status: 200, body: JSON.stringify(display) }))
  const notify = vi.fn()
  let current = true
  const release = vi.fn()
  const owner = { signal: new AbortController().signal, isCurrent: () => current, release }
  const host = new ToolResultLocalDisplayHostV1(transport, notify)
  return { host, transport, notify, display, owner, release, revoke: () => { current = false } }
}
const effectRequest = { ...request, snapshotDigest: 'c'.repeat(64) }
describe('Host fresh protected result effects', () => {
  it('uses selectors, masks copy and preserves original newline bytes', async () => {
    const f = fixture('ordinary\r\naccount: 6222020202020202020\n/Users/synthetic/private.go\n')
    expect((await f.host.open(request, f.owner)).ok).toBe(true)
    const copy = vi.fn(), save = vi.fn()
    expect(await f.host.effect(effectRequest, 'copy', () => true, async () => null, copy, save)).toEqual({ ok: true })
    expect(copy).toHaveBeenCalledWith('ordinary\r\naccount: [ACCOUNT]\n[PRIVATE_PATH]\n')
    expect(save).not.toHaveBeenCalled()
    expect(f.transport).toHaveBeenCalledTimes(3)
    for (const [path, body] of f.transport.mock.calls) {
      expect(path).toBe('/v1/local-display/tool-result-snapshot')
      expect(JSON.parse(body)).toEqual(selector)
    }
    f.host.close(request.viewId)
    expect(f.release).toHaveBeenCalledTimes(1)
  })
  it.each(['', 'package main\r\n'])('allows actual empty/code body %j without a case export grant', async body => {
    const f = fixture(body)
    await f.host.open(request, f.owner)
    const copy = vi.fn()
    expect(await f.host.effect(effectRequest, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: true })
    expect(copy).toHaveBeenCalledWith(body)
  })
  it.each(['cer1_' + 'a'.repeat(64), '<think>model private reasoning</think>'])('refuses a wholly withheld body with zero effects', async body => {
    const f = fixture(body)
    await f.host.open(request, f.owner)
    const copy = vi.fn(), save = vi.fn()
    expect(await f.host.effect(effectRequest, 'copy', () => true, async () => null, copy, save)).toEqual({ ok: false, code: 'unsupported' })
    expect(copy).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  })
  it('projects SGR-decorated text before masking split private identifiers', async () => {
    const f = fixture('ordinary\r\naccount: 622202\x1b[31m0202020202020\x1b[0m\n')
    await f.host.open(request, f.owner)
    const copy = vi.fn()
    expect(await f.host.effect(effectRequest, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: true })
    expect(copy).toHaveBeenCalledWith('ordinary\r\naccount: [ACCOUNT]\n')
  })
  it.each(['\x1b[2Jordinary', '\x1b]52;c;private\x07', '<th\x1b[31mink>private reasoning</think>'])('withholds unprojected control/reasoning text with zero effect', async body => {
    const f = fixture(body)
    await f.host.open(request, f.owner)
    const copy = vi.fn(), save = vi.fn()
    expect(await f.host.effect(effectRequest, 'copy', () => true, async () => null, copy, save)).toEqual({ ok: false, code: 'unsupported' })
    expect(copy).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  })
  it('saves only the masked frozen snippet and treats dialog cancel as zero effect', async () => {
    const f = fixture('ordinary\r\naccount: 6222020202020202020\n')
    await f.host.open(request, f.owner)
    const copy = vi.fn(), save = vi.fn(async () => undefined)
    expect(await f.host.effect(effectRequest, 'save', () => true, async () => '/synthetic-chosen.txt', copy, save)).toEqual({ ok: true })
    expect(save).toHaveBeenCalledWith('/synthetic-chosen.txt', 'ordinary\r\naccount: [ACCOUNT]\n')
    expect(copy).not.toHaveBeenCalled()
    save.mockClear()
    expect(await f.host.effect(effectRequest, 'save', () => true, async () => null, copy, save)).toEqual({ ok: false, code: 'canceled' })
    expect(save).not.toHaveBeenCalled()
  })
  it('rechecks after the dialog and rejects replaced authority before file writes', async () => {
    const f = fixture()
    await f.host.open(request, f.owner)
    const copy = vi.fn(), save = vi.fn()
    const pick = vi.fn(async () => { f.host.invalidate(); return '/synthetic-picked-file.txt' })
    expect(await f.host.effect(effectRequest, 'save', () => true, pick, copy, save)).toEqual({ ok: false, code: 'unavailable' })
    expect(pick).toHaveBeenCalledOnce(); expect(save).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled()
  })
  it('requires the exact immutable snapshot on the last fresh read', async () => {
    const f = fixture()
    await f.host.open(request, f.owner)
    f.transport.mockResolvedValueOnce({ ok: true, status: 200, body: JSON.stringify(f.display) })
    f.transport.mockResolvedValueOnce({ ok: true, status: 200, body: JSON.stringify({ ...f.display, snapshotDigest: 'd'.repeat(64) }) })
    const copy = vi.fn()
    expect(await f.host.effect(effectRequest, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unavailable' })
    expect(copy).not.toHaveBeenCalled(); expect(f.notify).toHaveBeenCalledWith(request.viewId)
  })
  it('retires every lease even if a stale document notification or release fails', async () => {
    const f = fixture()
    const host = new ToolResultLocalDisplayHostV1(f.transport, () => { throw new Error('closed document') })
    await host.open(request, { ...f.owner, release: () => { throw new Error('already detached') } })
    const otherRelease = vi.fn()
    await host.open({ ...request, viewId: '22222222-2222-4222-8222-222222222222' }, { ...f.owner, release: otherRelease })
    expect(() => host.invalidate()).not.toThrow()
    expect(otherRelease).toHaveBeenCalledOnce()
  })
  it('does not let an old failed request retire a newer view with the same ID', async () => {
    const f = fixture()
    let resolve!: (value: { ok: boolean; status: number; body: string }) => void
    f.transport.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    const older = f.host.open(request, f.owner)
    expect(await f.host.open(request, { ...f.owner, release: vi.fn() })).toMatchObject({ ok: true })
    resolve({ ok: false, status: 404, body: '{}' })
    expect(await older).toEqual({ ok: false, code: 'unavailable' })
    const copy = vi.fn()
    expect(await f.host.effect(effectRequest, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: true })
    expect(copy).toHaveBeenCalledWith('ordinary\r\n')
  })
  it('rejects a body revoked in the microtask after fresh validation but before open returns', async () => {
    const f = fixture()
    let complete!: (value: { ok: boolean; status: number; body: string }) => void
    f.transport.mockImplementationOnce(() => new Promise(resolve => { complete = resolve }))
    const result = f.host.open(request, f.owner)
    complete({ ok: true, status: 200, body: JSON.stringify(f.display) })
    // Resolving the transport queues fresh's continuation first. Its completed
    // validation then queues open behind this separately controlled revocation.
    let revoked = false
    queueMicrotask(() => { f.revoke(); revoked = true })
    expect(await result).toEqual({ ok: false, code: 'unavailable' })
    expect(revoked).toBe(true)
    expect(f.release).toHaveBeenCalledOnce()
  })
  it('rejects a replaced body in the fresh-to-open microtask without retiring its successor', async () => {
    const f = fixture()
    let complete!: (value: { ok: boolean; status: number; body: string }) => void
    f.transport.mockImplementationOnce(() => new Promise(resolve => { complete = resolve }))
    const older = f.host.open(request, f.owner)
    complete({ ok: true, status: 200, body: JSON.stringify(f.display) })
    const successorRelease = vi.fn()
    let successor!: ReturnType<typeof f.host.open>
    queueMicrotask(() => { successor = f.host.open(request, { ...f.owner, release: successorRelease }) })
    expect(await older).toEqual({ ok: false, code: 'unavailable' })
    expect(await successor).toMatchObject({ ok: true })
    expect(successorRelease).not.toHaveBeenCalled()
    expect(f.notify).not.toHaveBeenCalled()
    const copy = vi.fn()
    expect(await f.host.effect(effectRequest, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: true })
    expect(copy).toHaveBeenCalledWith('ordinary\r\n')
  })
  it('drops late body after close/document revoke and bounds invalid responses', async () => {
    const f = fixture()
    let resolve!: (value: { ok: boolean; status: number; body: string }) => void
    f.transport.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    const result = f.host.open(request, f.owner)
    f.host.close(request.viewId); f.revoke()
    resolve({ ok: true, status: 200, body: JSON.stringify(f.display) })
    expect(await result).toEqual({ ok: false, code: 'unavailable' })
    for (const body of ['x'.repeat(262145), JSON.stringify({ ...f.display, privatePath: '/hidden' })]) {
      const other = fixture()
      other.transport.mockResolvedValueOnce({ ok: true, status: 200, body })
      expect(await other.host.open(request, other.owner)).toEqual({ ok: false, code: 'unavailable' })
    }
  })
})
