import { afterEach, describe, expect, it, vi } from 'vitest'
vi.mock('electron', () => ({ clipboard: {}, dialog: {}, ipcMain: {}, shell: { openExternal: vi.fn(async () => undefined), showItemInFolder: vi.fn() } }))
vi.mock('node:fs/promises', async importOriginal => {
  const actual = await importOriginal<typeof import('node:fs/promises')>()
  return { ...actual, lstat: vi.fn(actual.lstat), open: vi.fn(actual.open) }
})
import { shell } from 'electron'
import { ToolResultLocalDisplayHostV1, invalidateToolResultLocalDisplaysV1, saveToolResultLocalFileV1 } from './tool-result-local-display-ipc'
import * as questionDisplayIpc from './user-input-local-display-ipc'
import { mkdtemp, realpath, writeFile, rm, symlink, unlink, rename, lstat, open, truncate, readFile, readdir, mkdir } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import type { GeneratedArtifactResponse } from '../../../packages/runtime/src/contracts/generated-artifact'
import { createHash } from 'node:crypto'
import type { ExcelCopyFrameV1, ExcelPreviewCopyReaderV1 } from '../services/excel-preview-copy-service'
import { formatExcelCopyHtmlV1 } from '../office/excel-preview-copy-html'
import type { Cell } from '@fortune-sheet/core'
import { TOOL_RESULT_LOCAL_PRESENT_SAVE_MAX_BYTES_V1 } from '../../../packages/runtime/src/contracts/tool-result-local-display'
const temporaryRoots: string[] = []
afterEach(async () => { vi.mocked(lstat).mockRestore(); vi.mocked(open).mockRestore(); vi.mocked(shell.showItemInFolder).mockClear(); await Promise.all(temporaryRoots.splice(0).map(path => rm(path, { recursive: true }))) })
const selector = { threadId: 'thread-a', turnId: 'turn-a', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const request = { ...selector, viewId: '11111111-1111-4111-8111-111111111111' }
function fixture(body = 'ordinary\r\n', readExcelCells?: ExcelPreviewCopyReaderV1) {
  const display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'bash', capture: { kind: 'shell', status: 'completed', body, label: 'printf', exitCode: 0, durationMs: 1, truncated: false, startLine: 0, endLine: 0, totalLines: 0 } }
  const transport = vi.fn(async (_path: string, _body: string) => ({ ok: true, status: 200, body: JSON.stringify(display) }))
  const notify = vi.fn()
  let current = true
  const release = vi.fn()
  const owner = { signal: new AbortController().signal, isCurrent: () => current, release }
  const host = new ToolResultLocalDisplayHostV1(transport, notify, readExcelCells)
  return { host, transport, notify, display, owner, release, revoke: () => { current = false } }
}
const effectRequest = { ...request, snapshotDigest: 'c'.repeat(64) }
const inspectFacts = { inputJSON: '{"command":"ACTUAL_INPUT"}', schemaJSON: '{"name":"bash","description":"Model request schema","parameters":{"type":"object"}}',
  inputTruncated: false, schemaTruncated: false, callEventAt: '2026-10-10T00:00:00Z', resultEventAt: '2026-10-10T00:00:00.125Z', durationMs: 125 }
it('opens a typed original file result while refusing all export and path effects before a dialog', async () => {
  const f = fixture()
  const source = { ...f.display, toolName: 'ls', inspect: { ...inspectFacts, schemaJSON: undefined }, capture: {
    kind: 'file_result', status: 'completed', body: '', label: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false,
    fileResult: { operation: 'ls', classification: 'source_exact_local', list: { path: '/synthetic/original', relativePath: '.',
      entries: [{ name: 'ACTUAL_ENTRY', displayName: 'ACTUAL_ENTRY', kind: 'file' }], names: ['ACTUAL_ENTRY'],
      limit: 200, producerTruncated: false, originalCount: 1 } }
  } }
  f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify(source) }))
  expect(await f.host.open(request, f.owner)).toMatchObject({ ok: true, display: { capture: source.capture } })
  const pick = vi.fn(async () => '/synthetic-unused'), copy = vi.fn(), save = vi.fn()
  for (const section of ['result', 'payload', 'schema'] as const) {
    expect(await f.host.effect({ ...effectRequest, section }, 'copy', () => true, pick, copy, save)).toEqual({ ok: false, code: 'unsupported' })
  }
  expect(await f.host.effect(effectRequest, 'save', () => true, pick, copy, save)).toEqual({ ok: false, code: 'unsupported' })
  expect(pick).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  f.revoke(); expect((await f.host.open(request, f.owner)).ok).toBe(false)
})
it('keeps a native Schedule receipt and Inspect in the fresh lease with no export effects', async () => {
  const f = fixture()
  const source = { ...f.display, toolName: 'mcp__gui_schedule__gui_schedule_delete', capture: {
    kind: 'schedule', status: 'completed', body: '', label: '', durationMs: 0, truncated: false, startLine: 0, endLine: 0, totalLines: 0,
    schedule: { operation: 'delete', status: 'success', message: 'Actual delete', taskId: 'task-1' }
  }, inspect: { ...inspectFacts, schemaJSON: undefined } }
  f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify(source) }))
  expect(await f.host.open(request, f.owner)).toMatchObject({ ok: true, display: { capture: source.capture, inspect: {
    inputJSON: inspectFacts.inputJSON, inputTruncated: false, schemaTruncated: false,
    callEventAt: inspectFacts.callEventAt, resultEventAt: inspectFacts.resultEventAt, durationMs: inspectFacts.durationMs
  } } })
  const pick = vi.fn(async () => '/synthetic-unused'), copy = vi.fn(), save = vi.fn()
  for (const section of ['result', 'payload', 'schema'] as const) {
    expect(await f.host.effect({ ...effectRequest, section }, 'copy', () => true, pick, copy, save)).toEqual({ ok: false, code: 'unsupported' })
  }
  expect(await f.host.effect(effectRequest, 'save', () => true, pick, copy, save)).toEqual({ ok: false, code: 'unsupported' })
  expect(pick).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  f.revoke()
  expect((await f.host.open(request, f.owner)).ok).toBe(false)
})
it('passes the original inspect facts through Main open while preserving existing result export only', async () => {
  const f = fixture(), source = { ...f.display, inspect: inspectFacts }
  f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify(source) }))
  expect(await f.host.open(request, f.owner)).toMatchObject({ ok: true, display: { inspect: inspectFacts } })
  const copy = vi.fn(), save = vi.fn(), pick = vi.fn(async () => '/synthetic-unused')
  for (const section of ['payload', 'schema'] as const) expect(await f.host.effect({ ...effectRequest, section }, 'copy', () => true, pick, copy, save)).toEqual({ ok: false, code: 'unsupported' })
  expect(copy).not.toHaveBeenCalled(); expect(pick).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  expect(await f.host.effect(effectRequest, 'copy', () => true, pick, copy, save)).toEqual({ ok: true })
  expect(copy).toHaveBeenCalledWith(f.display.capture.body)
})
it.each(['job', 'inspect'] as const)('refuses %s exports before destination selection and keeps facts in the existing lease', async kind => {
  const f = fixture(), reserved = { status: 'completed', body: '', label: '', durationMs: 0, truncated: false, startLine: 0, endLine: 0, totalLines: 0 }
  const source = kind === 'job' ? { ...f.display, toolName: 'bash_output', capture: { ...reserved, kind, job: {
    operation: 'output', jobs: [{ jobId: 'job-a', kind: 'bash', status: 'running', name: 'Actual name', label: 'Actual label',
      output: { availability: 'available', body: 'PRIVATE_OUTPUT', truncated: false } }], total: 1, truncated: false, receipt: null } } }
    : { ...f.display, inspect: inspectFacts, capture: { ...reserved, kind } }
  f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify(source) }))
  expect((await f.host.open(request, f.owner)).ok).toBe(true)
  const copy = vi.fn(), save = vi.fn(), pick = vi.fn(async () => '/synthetic-unused')
  for (const action of ['copy', 'save'] as const) expect(await f.host.effect(effectRequest, action, () => true, pick, copy, save)).toEqual({ ok: false, code: 'unsupported' })
  expect(pick).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  f.revoke(); expect((await f.host.open(request, f.owner)).ok).toBe(false)
})
it.each(['goal', 'todo', 'skill'] as const)('keeps %s read-only through existing fresh lease with zero export effects', async kind => {
  const f = fixture()
  const reserved = { kind, status: 'completed', body: '', label: '', durationMs: 0, truncated: false, startLine: 0, endLine: 0, totalLines: 0 }
  const captures = {
    goal: { ...reserved, goal: { goal: null, remainingTokens: null } },
    todo: { ...reserved, todo: { todos: { threadId: 'thread-a', items: [], updatedAt: '2026-10-10T00:00:00Z' }, baseline: { availability: 'known', todos: null } } },
    skill: { ...reserved, body: 'Actual instruction', skill: { requestedName: 'skill-a', resolved: { id: 'skill-a', name: 'Skill A', runAs: 'inline', packageDigest: 'a'.repeat(64) }, child: null, error: null } }
  }
  const toolNames = { goal: 'get_goal', todo: 'todo_write', skill: 'run_skill' }
  f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify({ ...f.display, toolName: toolNames[kind], capture: captures[kind] }) }))
  expect((await f.host.open(request, f.owner)).ok).toBe(true)
  const pick = vi.fn(async () => '/synthetic-unused'), copy = vi.fn(), save = vi.fn()
  for (const action of ['copy', 'save'] as const) {
    expect(await f.host.effect(effectRequest, action, () => true, pick, copy, save)).toEqual({ ok: false, code: 'unsupported' })
  }
  expect(pick).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  f.revoke()
  expect((await f.host.open(request, f.owner)).ok).toBe(false)
})
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
  it.each([
    ['\x1b[mordinary\r\n', 'ordinary\r\n'],
    ['ordi\x1b[;mna\x1b[000;031mry\x1b[0m\tline\r\n', 'ordinary\tline\r\n'],
    ['a\x1b[000;00mb\x1b[123456789;0mc', 'abc'],
    ['pair:🚀\x1b[31mplain\x1b[0m', 'pair:🚀plain'],
    ['account: 622202\x1b[m020202\x1b[;m0202020\n', 'account: [ACCOUNT]\n']
  ])('preserves original SGR stripping and pre-mask joining for %j', async (body, expected) => {
    const f = fixture(body); await f.host.open(request, f.owner)
    const copy = vi.fn(), save = vi.fn()
    expect(await f.host.effect(effectRequest, 'copy', () => true, async () => null, copy, save)).toEqual({ ok: true })
    expect(copy).toHaveBeenCalledWith(expected); expect(save).not.toHaveBeenCalled()
  })
  it.each([9, 10, 13, 32, 126, 160])('permits the original non-forbidden character boundary %i', async code => {
    const body = 'a' + String.fromCharCode(code) + 'b', f = fixture(body)
    await f.host.open(request, f.owner)
    const copy = vi.fn()
    expect(await f.host.effect(effectRequest, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: true })
    expect(copy).toHaveBeenCalledWith(body)
  })
  it.each([
    ...[0, 8, 11, 12, 14, 31, 127, 159].map(code => 'a' + String.fromCharCode(code) + 'b'),
    '\x1b[', '\x1b[31', '\x1b[31M', '\x1b[3:1m', '\x1b[ 31m', '\x1b[31 m', '\x1b[0;31\nm', 'x\x1b[12\x1b[0myz'
  ])('rejects control and malformed SGR boundaries %j with zero clipboard/file effects', async body => {
    for (const action of ['copy', 'save'] as const) {
      const f = fixture(body); await f.host.open(request, f.owner)
      const copy = vi.fn(), save = vi.fn(), pick = vi.fn(async () => '/synthetic-unused.txt')
      expect(await f.host.effect(effectRequest, action, () => true, pick, copy, save)).toEqual({ ok: false, code: 'unsupported' })
      expect(copy).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
      if (action === 'copy') expect(pick).not.toHaveBeenCalled()
    }
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

async function readFixture() {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'analytix-read-navigation-')))
  temporaryRoots.push(root)
  const path = join(root, 'actual.go'), body = 'first\r\nsecond\r\n'
  await writeFile(path, body)
  const display = { schemaVersion: 1, snapshotDigest: effectRequest.snapshotDigest, resultItemId: selector.resultItemId, toolName: 'read',
    capture: { kind: 'read', status: 'completed', body: 'second\r\n', label: '/misleading/truncated-label…', durationMs: 0, truncated: false, startLine: 2, endLine: 2, totalLines: 2 },
    readTarget: { path, workspaceRoot: root, startLine: 2, contentDigest: createHash('sha256').update(body).digest('hex') } }
  const transport = vi.fn(async (_path: string, _body: string) => ({ ok: true, status: 200, body: JSON.stringify(display) }))
  let current = true
  const release = vi.fn(), notify = vi.fn()
  const owner = { signal: new AbortController().signal, isCurrent: () => current, release }
  const host = new ToolResultLocalDisplayHostV1(transport, notify)
  return { root, path, body, display, host, transport, notify, owner, release, revoke: () => { current = false } }
}
describe('protected Read navigation with actual filesystem bytes', () => {
  it('removes the private target from display and returns the authoritative path and start after two fresh reads', async () => {
    const f = await readFixture(), opened = await f.host.open(request, f.owner)
    expect(opened).toMatchObject({ ok: true, display: { canNavigate: true } })
    expect(JSON.stringify(opened)).not.toContain(f.path)
    expect(JSON.stringify(opened)).not.toContain('contentDigest')
    expect(await f.host.navigate(effectRequest, () => true)).toEqual({ ok: true, target: { path: f.path, workspaceRoot: f.root, line: 2 } })
    expect(f.transport).toHaveBeenCalledTimes(3)
    for (const [, body] of f.transport.mock.calls) expect(JSON.parse(body)).toEqual(selector)
  })
  it.each(['external', 'symlink', 'utf16-bom', 'utf16-no-bom', 'invalid-utf8', 'nul'])('hides navigation for an unsupported %s target without losing the historical body', async kind => {
    const f = await readFixture()
    if (kind === 'external') { const other = await readFixture(); f.display.readTarget.path = other.path }
    else if (kind === 'symlink') { const other = join(f.root, 'other.go'); await writeFile(other, f.body); await unlink(f.path); await symlink(other, f.path) }
    else await writeFile(f.path, kind === 'utf16-bom' ? Buffer.from([0xff, 0xfe, 0x61, 0x00])
      : kind === 'utf16-no-bom' ? Buffer.from([0x61, 0x00, 0x62, 0x00])
      : kind === 'invalid-utf8' ? Buffer.from([0xff, 0xff]) : Buffer.from([0x61, 0x00]))
    expect(await f.host.open(request, f.owner)).toMatchObject({ ok: true, display: { canNavigate: false, capture: { body: 'second\r\n' } } })
    expect(f.transport).toHaveBeenCalledTimes(1)
  })
  it('qualifies only bounded preview bytes at expansion, retaining full digest validation for the click', async () => {
    const f = await readFixture()
    await writeFile(f.path, 'x'.repeat(1_500_000) + 'changed tail')
    // The frozen digest is intentionally different. Expansion can qualify the
    // bounded UTF-8 prefix, but navigation must reject the full changed bytes.
    expect(await f.host.open(request, f.owner)).toMatchObject({ ok: true, display: { canNavigate: true } })
    expect(await f.host.navigate(effectRequest, () => true)).toEqual({ ok: false, code: 'unavailable' })
  })
  it.each([{ path: '/forged' }, { line: 1 }, { principal: 'other' }, { threadId: 'other' }, { snapshotDigest: 'f'.repeat(64) }])('rejects forged navigation requests %j', async extra => {
    const f = await readFixture()
    await f.host.open(request, f.owner)
    expect(await f.host.navigate({ ...effectRequest, ...extra }, () => true)).toEqual({ ok: false, code: 'unavailable' })
    expect(f.transport).toHaveBeenCalledTimes(1)
  })
  it('keeps legacy targetless snapshots readable but never guesses their label as a target', async () => {
    const f = await readFixture(), { readTarget: _target, ...legacy } = f.display
    f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify(legacy) }))
    expect(await f.host.open(request, f.owner)).toMatchObject({ ok: true, display: { canNavigate: false } })
    expect(await f.host.navigate(effectRequest, () => true)).toEqual({ ok: false, code: 'unavailable' })
  })
  it('rejects changed content, removal and symlink substitution without changing the historical body', async () => {
    for (const change of ['content', 'remove', 'symlink']) {
      const f = await readFixture()
      await f.host.open(request, f.owner)
      if (change === 'content') await writeFile(f.path, 'replacement source')
      else {
        await unlink(f.path)
        if (change === 'symlink') { const other = join(f.root, 'other.go'); await writeFile(other, f.body); await symlink(other, f.path) }
      }
      expect(await f.host.navigate(effectRequest, () => true)).toEqual({ ok: false, code: 'unavailable' })
      expect(f.display.capture.body).toBe('second\r\n')
    }
  })
  it('rejects a forged/replaced target and a file modified while final authority is awaited', async () => {
    for (const change of ['target', 'digest', 'file']) {
      const f = await readFixture(); await f.host.open(request, f.owner)
      f.transport.mockResolvedValueOnce({ ok: true, status: 200, body: JSON.stringify(f.display) })
      f.transport.mockImplementationOnce(async () => {
        if (change === 'file') await writeFile(f.path, 'replacement source')
        const display = change === 'target' ? { ...f.display, readTarget: { ...f.display.readTarget, path: join(f.root, 'forged.go') } }
          : change === 'digest' ? { ...f.display, snapshotDigest: 'f'.repeat(64) } : f.display
        return { ok: true, status: 200, body: JSON.stringify(display) }
      })
      expect(await f.host.navigate(effectRequest, () => true)).toEqual({ ok: false, code: 'unavailable' })
    }
  })
  it.each(['document', 'workspace', 'authority'])('drops final late navigation after %s revocation', async reason => {
    const f = await readFixture(); await f.host.open(request, f.owner)
    f.transport.mockResolvedValueOnce({ ok: true, status: 200, body: JSON.stringify(f.display) })
    let finish!: (value: { ok: boolean; status: number; body: string }) => void
    f.transport.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const result = f.host.navigate(effectRequest, () => true)
    await vi.waitFor(() => expect(finish).toBeDefined())
    if (reason === 'document') f.revoke()
    else if (reason === 'workspace') f.host.invalidate()
    finish({ ok: reason !== 'authority', status: reason === 'authority' ? 404 : 200, body: JSON.stringify(f.display) })
    expect(await result).toEqual({ ok: false, code: 'unavailable' })
    expect(f.release).toHaveBeenCalledOnce()
  })
})

describe('same-owner protected preview read', () => {
  it('returns actual bounded original bytes and Host coordinates under fresh authority without a path request', async () => {
    const f = await readFixture(); await f.host.open(request, f.owner)
    expect(await f.host.preview(effectRequest, () => true)).toEqual({ ok: true, path: f.path, content: f.body, size: Buffer.byteLength(f.body), truncated: false, line: 2 })
    for (const [, body] of f.transport.mock.calls) expect(JSON.parse(body)).toEqual(selector)
    expect(await f.host.preview({ ...effectRequest, path: '/forged' }, () => true)).toEqual({ ok: false, message: 'Protected file preview unavailable.' })
  })
  it('rejects replaced actual bytes and late revoked preview responses', async () => {
    const f = await readFixture(); await f.host.open(request, f.owner)
    await writeFile(f.path, 'changed source')
    expect((await f.host.preview(effectRequest, () => true)).ok).toBe(false)
    const other = await readFixture(); await other.host.open(request, other.owner)
    other.transport.mockResolvedValueOnce({ ok: true, status: 200, body: JSON.stringify(other.display) })
    let finish!: (value: { ok: boolean; status: number; body: string }) => void
    other.transport.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const result = other.host.preview(effectRequest, () => true)
    await vi.waitFor(() => expect(finish).toBeDefined())
    other.host.invalidate()
    finish({ ok: true, status: 200, body: JSON.stringify(other.display) })
    expect(await result).toEqual({ ok: false, message: 'Protected file preview unavailable.' })
  })
  it('preserves the existing 1.5MB preview cap while verifying the digest of the entire file', async () => {
    const f = await readFixture(), body = 'a'.repeat(1_500_001)
    await writeFile(f.path, body)
    f.display.readTarget.contentDigest = createHash('sha256').update(body).digest('hex')
    await f.host.open(request, f.owner)
    const result = await f.host.preview(effectRequest, () => true)
    expect(result).toMatchObject({ ok: true, size: 1_500_001, truncated: true, line: 2 })
    if (result.ok) expect(Buffer.byteLength(result.content)).toBe(1_500_000)
  })
})

function webFixture(body = 'actual extracted text\n') {
  const f = fixture(), requestUrl = 'https://synthetic.invalid/request', finalUrl = 'https://synthetic.invalid/final'
  const display = { schemaVersion: 1, snapshotDigest: effectRequest.snapshotDigest, resultItemId: selector.resultItemId,
    toolName: 'web_fetch', capture: { kind: 'web_fetch', status: 'completed', body, label: 'Fetch', durationMs: 7,
      truncated: false, startLine: 0, endLine: 0, totalLines: 0, web: { requestUrl, finalUrl, requestUrlTruncated: false,
        finalUrlTruncated: false, inputTruncated: false, schemaTruncated: false, inputJSON: JSON.stringify({ url: requestUrl }),
        schemaJSON: '{"type":"object"}', responseComplete: true, toolIsError: true, statusCode: 404, fetchTruncated: false,
        startedAt: '2026-10-08T12:00:00Z' } }, webTargets: { requestUrl, resultUrl: finalUrl } }
  f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify(display) }))
  return { ...f, display }
}
describe('protected Web result links and Inspect root copy', () => {
  it('returns complete observed text above legacy budget and strips all Host targets', async () => {
    const f = webFixture('a'.repeat(1_000_000)), opened = await f.host.open(request, f.owner)
    expect(opened).toMatchObject({ ok: true, display: { canOpenWeb: { request: true, result: true }, capture: { body: f.display.capture.body } } })
    expect(opened).not.toHaveProperty('display.webTargets')
    expect(opened).not.toHaveProperty('display.capture.webTargets')
    const overflow = webFixture('a'.repeat(1_000_001))
    expect(await overflow.host.open(request, overflow.owner)).toEqual({ ok: false, code: 'unavailable' })
  })
  it.each(['request', 'result'] as const)('opens only sealed %s role after two fresh reads', async targetRole => {
    const f = webFixture(), openExternal = vi.fn(async () => undefined)
    await f.host.open(request, f.owner)
    expect(await f.host.openWebLink({ ...effectRequest, targetRole }, () => true, openExternal)).toEqual({ ok: true })
    expect(openExternal).toHaveBeenCalledWith(targetRole === 'request' ? f.display.webTargets.requestUrl : f.display.webTargets.resultUrl)
    expect(f.transport).toHaveBeenCalledTimes(3)
    for (const [, raw] of f.transport.mock.calls) expect(JSON.parse(raw)).toEqual(selector)
  })
  it.each(['url', 'digest', 'target', 'authority'] as const)('blocks %s substitution/revocation with zero OS effects', async reason => {
    const f = webFixture(), openExternal = vi.fn(async () => undefined)
    await f.host.open(request, f.owner)
    let payload: unknown = { ...effectRequest, targetRole: 'request' }
    if (reason === 'url') payload = { ...payload as object, url: 'https://forged.invalid' }
    else if (reason === 'digest') payload = { ...payload as object, snapshotDigest: 'f'.repeat(64) }
    else {
      f.transport.mockResolvedValueOnce({ ok: true, status: 200, body: JSON.stringify(f.display) })
      f.transport.mockImplementationOnce(async () => {
        if (reason === 'authority') f.revoke()
        return { ok: true, status: 200, body: JSON.stringify({ ...f.display,
          ...(reason === 'target' ? { webTargets: { requestUrl: 'https://replacement.invalid' } } : {}) }) }
      })
    }
    expect(await f.host.openWebLink(payload, () => true, openExternal)).toEqual({ ok: false, code: 'unavailable' })
    expect(openExternal).not.toHaveBeenCalled()
  })
  it('does not widen Shell envelopes to the Web maximum', async () => {
    const f = fixture('a'.repeat(32768)), display = JSON.stringify(f.display)
    f.transport.mockResolvedValueOnce({ ok: true, status: 200, body: display + ' '.repeat(262145) })
    expect(await f.host.open(request, f.owner)).toEqual({ ok: false, code: 'unavailable' })
  })
  it.each(['payload', 'schema', 'request-url', 'result-url'] as const)('copies the actual %s root through existing Host projection', async section => {
    const f = webFixture(), copy = vi.fn()
    await f.host.open(request, f.owner)
    expect(await f.host.effect({ ...effectRequest, section }, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: true })
    const expected = { payload: f.display.capture.web.inputJSON, schema: f.display.capture.web.schemaJSON,
      'request-url': f.display.capture.web.requestUrl, 'result-url': f.display.capture.web.finalUrl }
    expect(copy).toHaveBeenCalledWith(expected[section])
    expect(await f.host.effect({ ...effectRequest, section, body: 'forged' }, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unavailable' })
  })
  it('does not reconstruct omitted input or copy a schema from another snapshot', async () => {
    const f = webFixture(), copy = vi.fn()
    f.display.capture.web.inputTruncated = true
    delete (f.display.capture.web as { inputJSON?: string }).inputJSON
    await f.host.open(request, f.owner)
    expect(await f.host.effect({ ...effectRequest, section: 'payload' }, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unsupported' })
    f.transport.mockResolvedValueOnce({ ok: true, status: 200, body: JSON.stringify(f.display) })
    f.transport.mockResolvedValueOnce({ ok: true, status: 200, body: JSON.stringify({ ...f.display, snapshotDigest: 'f'.repeat(64) }) })
    expect(await f.host.effect({ ...effectRequest, section: 'schema' }, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unavailable' })
    expect(copy).not.toHaveBeenCalled()
  })
})

describe('Search uses the existing Host result lease', () => {
  const source = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'grep', capture: {
    kind: 'search', status: 'completed', body: '/synthetic/source.go\n2: SOURCE_ONE\n3: SOURCE_TWO', label: 'grep', durationMs: 1, truncated: false,
    startLine: 0, endLine: 0, totalLines: 0, search: { shape: 'matches', files: [{ path: '/synthetic/source.go', matches: [
      { lineNumber: 2, line: 'SOURCE_ONE', column: 1, contextBefore: [], contextAfter: [] },
      { lineNumber: 3, line: 'SOURCE_TWO', column: 1, contextBefore: [], contextAfter: [] }
    ] }], total: null, truncated: true, toolIsError: false, inputJSON: '{"pattern":"SOURCE"}', inputTruncated: false, startedAt: '2026-10-08T00:00:00Z' } } }
  it('copies all sealed result rows and real input through fresh reads, with no renderer body/path authority', async () => {
    const f = fixture(); f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify(source) }))
    const opened = await f.host.open(request, f.owner)
    expect(opened.ok).toBe(true)
    const copy = vi.fn()
    expect(await f.host.effect(effectRequest, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: true })
    expect(copy.mock.calls[0][0]).toContain('SOURCE_ONE\n3: SOURCE_TWO')
    expect(await f.host.effect({ ...effectRequest, section: 'payload' }, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: true })
    expect(copy).toHaveBeenLastCalledWith(source.capture.search.inputJSON)
    expect(await f.host.effect({ ...effectRequest, body: 'FORGED' }, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unavailable' })
  })
  it('refuses an expired immutable locator and never recovers a mutable source by path', async () => {
    const f = fixture(); f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify(source) }))
    await f.host.open(request, f.owner)
    f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify({ ...source, snapshotDigest: 'd'.repeat(64) }) }))
    const copy = vi.fn()
    expect(await f.host.effect(effectRequest, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unavailable' })
    expect(copy).not.toHaveBeenCalled()
  })
})

it('forwards ordinary ToolResult background separately from true Question authority retirement', () => {
  const invalidate = vi.spyOn(questionDisplayIpc, 'invalidateUserInputLocalDisplaysV1').mockImplementation(() => undefined)
  try {
    invalidateToolResultLocalDisplaysV1('background')
    expect(invalidate).toHaveBeenLastCalledWith('background')
    invalidateToolResultLocalDisplaysV1()
    expect(invalidate).toHaveBeenLastCalledWith('authority')
  } finally { invalidate.mockRestore() }
})


it('strips Host-only Present targets and refuses declaration exports under the original lease', async () => {
  const f = fixture()
  const source = { ...f.display, toolName: 'present', inspect: { ...inspectFacts, schemaJSON: undefined },
    capture: { kind: 'present', status: 'completed', body: '', label: '', durationMs: 0, truncated: false,
      startLine: 0, endLine: 0, totalLines: 0, present: { files: [{ index: 0, path: 'actual.md', description: '' }] } },
    presentTargets: { workspaceRoot: '/synthetic', files: [{ index: 0, path: '/synthetic/actual.md' }] } }
  f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify(source) }))
  const opened = await f.host.open(request, f.owner)
  expect(opened).toMatchObject({ ok: true, display: { capture: source.capture, inspect: JSON.parse(JSON.stringify(source.inspect)) } })
  expect(JSON.stringify(opened)).not.toContain('/synthetic')
  expect(opened.ok && 'presentTargets' in opened.display).toBe(false)
  expect(opened.ok && opened.display.inspect && 'schemaJSON' in opened.display.inspect).toBe(false)
  const pick = vi.fn(async () => '/synthetic-unused'), copy = vi.fn(), save = vi.fn()
  for (const section of ['result', 'payload', 'schema'] as const) {
    expect(await f.host.effect({ ...effectRequest, section }, 'copy', () => true, pick, copy, save)).toEqual({ ok: false, code: 'unsupported' })
  }
  expect(await f.host.effect(effectRequest, 'save', () => true, pick, copy, save)).toEqual({ ok: false, code: 'unsupported' })
  expect(pick).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  f.revoke()
  expect((await f.host.open(request, f.owner)).ok).toBe(false)
})

async function presentFixture(contents: string | Buffer = 'original content', name = 'current.md', readExcelCells?: ExcelPreviewCopyReaderV1) {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'analytix-present-host-')))
  temporaryRoots.push(root)
  const path = join(root, name)
  await writeFile(path, contents)
  const f = fixture(undefined, readExcelCells)
  const source = { ...f.display, toolName: 'present', inspect: { ...inspectFacts, schemaJSON: undefined },
    capture: { kind: 'present', status: 'completed', body: '', label: '', durationMs: 0, truncated: false,
      startLine: 0, endLine: 0, totalLines: 0, present: { files: [{ index: 0, path: name, description: '' }] } },
    presentTargets: { workspaceRoot: root, files: [{ index: 0, path }] } }
  f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify(source) }))
  expect((await f.host.open(request, f.owner)).ok).toBe(true)
  return { ...f, root, path, source, indexed: { ...effectRequest, index: 0 } }
}
it('Present previews the current edited bytes and passes only sealed file identity to native actions', async () => {
  const f = await presentFixture()
  await writeFile(f.path, 'edited after declaration')
  expect(await f.host.previewPresent(f.indexed, () => true)).toMatchObject({ ok: true, content: 'edited after declaration' })
  const list = vi.fn(async (_path: string, guard: () => Promise<boolean>) => ({ ok: true as const, applications: await guard() ? [{ applicationId: 'org.example.Real', label: 'Real app', isDefault: true }] : [] }))
  expect(await f.host.presentApplications(f.indexed, () => true, list)).toMatchObject({ ok: true })
  expect(list).toHaveBeenCalledWith(f.path, expect.any(Function))
  const launch = vi.fn(async (_path: string, _id: string | undefined, guard: () => Promise<boolean>) => await guard() ? { ok: true as const } : { ok: false as const, code: 'unavailable' as const })
  expect(await f.host.openPresent({ ...f.indexed, applicationId: 'org.example.Real' }, () => true, launch)).toEqual({ ok: true })
  expect(launch).toHaveBeenCalledWith(f.path, 'org.example.Real', expect.any(Function))
})
it.each(['path', 'applicationPath', 'index', 'digest'] as const)('Present rejects forged %s before native query or launch', async field => {
  const f = await presentFixture(), list = vi.fn(), launch = vi.fn()
  const forged = field === 'path' ? { ...f.indexed, path: f.path } : field === 'applicationPath' ? { ...f.indexed, applicationPath: '/Applications/Forged.app' }
    : field === 'index' ? { ...f.indexed, index: 1 } : { ...f.indexed, snapshotDigest: 'e'.repeat(64) }
  expect(await f.host.presentApplications(forged, () => true, list)).toEqual({ ok: false, code: 'unavailable' })
  expect(await f.host.openPresent(forged, () => true, launch)).toEqual({ ok: false, code: 'unavailable' })
  expect(list).not.toHaveBeenCalled(); expect(launch).not.toHaveBeenCalled()
})
it('Present rejects symlinks and masks late query/launch completion after authority retirement', async () => {
  const f = await presentFixture()
  const list = vi.fn(async () => { f.revoke(); return { ok: true as const, applications: [] } })
  expect(await f.host.presentApplications(f.indexed, () => true, list)).toEqual({ ok: false, code: 'unavailable' })
  const second = await presentFixture(), launch = vi.fn(async () => { second.host.invalidate(); return { ok: true as const } })
  expect(await second.host.openPresent(second.indexed, () => true, launch)).toEqual({ ok: false, code: 'unavailable' })
  const third = await presentFixture()
  await rename(third.path, join(third.root, 'real.md'))
  await symlink(join(third.root, 'real.md'), third.path)
  const forbiddenLaunch = vi.fn()
  expect(await third.host.openPresent(third.indexed, () => true, forbiddenLaunch)).toEqual({ ok: false, code: 'unavailable' })
  expect(forbiddenLaunch).not.toHaveBeenCalled()
})
it('Present rejects a replacement inode observed during the final fresh query guard', async () => {
  const f = await presentFixture()
  const list = vi.fn(async (_path: string, guard: () => Promise<boolean>) => {
    await rename(f.path, join(f.root, 'prior.md'))
    await writeFile(f.path, 'replacement object')
    expect(await guard()).toBe(false)
    return { ok: true as const, applications: [] }
  })
  expect(await f.host.presentApplications(f.indexed, () => true, list)).toEqual({ ok: false, code: 'unavailable' })
})
it('reveals only the sealed current Present file through the existing shell effect without querying applications', async () => {
  const f = await presentFixture(), launch = vi.fn()
  await writeFile(f.path, 'current edited content remains revealable')
  expect(await f.host.openPresent({ ...f.indexed, action: 'reveal' }, () => true, launch)).toEqual({ ok: true })
  expect(shell.showItemInFolder).toHaveBeenCalledExactlyOnceWith(f.path)
  expect(launch).not.toHaveBeenCalled()
  for (const [, body] of f.transport.mock.calls) expect(JSON.parse(body)).toEqual(selector)
})
it.each([
  { path: '/forged' }, { appPath: '/Applications/Forged.app' }, { applicationId: 'org.synthetic.Mixed' },
  { action: 'unknown' }, { index: -1 }, { index: 1 }, { snapshotDigest: 'e'.repeat(64) }, { threadId: 'other' }
])('rejects forged Reveal request %j without any native effect', async extra => {
  const f = await presentFixture(), launch = vi.fn(), reveal = vi.fn()
  expect(await f.host.openPresent({ ...f.indexed, action: 'reveal', ...extra }, () => true, launch, reveal)).toEqual({ ok: false, code: 'unavailable' })
  expect(launch).not.toHaveBeenCalled(); expect(reveal).not.toHaveBeenCalled(); expect(shell.showItemInFolder).not.toHaveBeenCalled()
})
it.each(['revoked', 'successor'] as const)('does not reveal after %s in the final-guard-to-effect microtask', async mode => {
  const f = await presentFixture(), launch = vi.fn(), reveal = vi.fn()
  const observed = await lstat(f.path, { bigint: true })
  let complete!: (value: typeof observed) => void, reached!: () => void
  const waiting = new Promise<void>(resolve => { reached = resolve })
  vi.mocked(lstat).mockResolvedValueOnce(observed as never).mockImplementationOnce(() => {
    reached(); return new Promise(resolve => { complete = resolve }) as never
  })
  const effect = f.host.openPresent({ ...f.indexed, action: 'reveal' }, () => true, launch, reveal)
  await waiting
  let successor: ReturnType<typeof f.host.open> | undefined
  const successorRelease = vi.fn()
  complete(observed)
  // The original guard sees its last stat while current. The queued retirement
  // then precedes the caller's continuation, requiring its synchronous check.
  queueMicrotask(() => {
    if (mode === 'revoked') f.revoke()
    else successor = f.host.open(request, { ...f.owner, release: successorRelease })
  })
  expect(await effect).toEqual({ ok: false, code: 'unavailable' })
  expect(launch).not.toHaveBeenCalled(); expect(reveal).not.toHaveBeenCalled()
  if (mode === 'successor') {
    expect(await successor).toMatchObject({ ok: true })
    expect(successorRelease).not.toHaveBeenCalled()
    expect(await f.host.openPresent({ ...f.indexed, action: 'reveal' }, () => true, launch, reveal)).toEqual({ ok: true })
    expect(reveal).toHaveBeenCalledExactlyOnceWith(f.path)
  }
})
it('rejects a replacement file during the final fresh Reveal guard', async () => {
  const f = await presentFixture(), launch = vi.fn(), reveal = vi.fn()
  f.transport.mockResolvedValueOnce({ ok: true, status: 200, body: JSON.stringify(f.source) })
    .mockImplementationOnce(async () => {
      await rename(f.path, join(f.root, 'before-reveal.md'))
      await writeFile(f.path, 'replacement object')
      return { ok: true, status: 200, body: JSON.stringify(f.source) }
    })
  expect(await f.host.openPresent({ ...f.indexed, action: 'reveal' }, () => true, launch, reveal)).toEqual({ ok: false, code: 'unavailable' })
  expect(launch).not.toHaveBeenCalled(); expect(reveal).not.toHaveBeenCalled()
})
it('reports an unavailable Reveal effect when the native shell rejects it', async () => {
  const f = await presentFixture(), launch = vi.fn(), reveal = vi.fn(() => { throw new Error('synthetic shell failure') })
  expect(await f.host.openPresent({ ...f.indexed, action: 'reveal' }, () => true, launch, reveal)).toEqual({ ok: false, code: 'unavailable' })
  expect(launch).not.toHaveBeenCalled(); expect(reveal).toHaveBeenCalledExactlyOnceWith(f.path)
})
it('Present reports binary and invalid internal UTF-8 as unsupported source preview', async () => {
  for (const bytes of [Buffer.from([65, 0, 66]), Buffer.from([65, 255, 66]), Buffer.concat([Buffer.from([255]), Buffer.alloc(1_500_001, 65)])]) {
    const f = await presentFixture(bytes)
    expect((await f.host.previewPresent(f.indexed, () => true)).ok).toBe(false)
  }
  const f = await presentFixture(Buffer.concat([Buffer.alloc(1_499_999, 65), Buffer.from('中')]))
  expect(await f.host.previewPresent(f.indexed, () => true)).toMatchObject({ ok: true, truncated: true })
})


describe('Present private history shares current document authority', () => {
  const historyRequest = { kind: 'present_history', threadId: 'thread-a', turnId: 'turn-a', viewId: request.viewId }
  const history = { schemaVersion: 1, kind: 'present_history', threadId: 'thread-a', turnId: 'turn-a', orderedItems: [
    { kind: 'tool_result', itemId: selector.resultItemId, callId: selector.callId, toolName: 'present' },
    { kind: 'assistant_text', itemId: 'closing' }
  ] }
  it('uses the existing private path with only identities and closes the same lease', async () => {
    const f = fixture()
    f.transport.mockResolvedValue({ ok: true, status: 200, body: JSON.stringify(history) })
    expect(await f.host.openPresentHistory(historyRequest, f.owner)).toEqual({ ok: true, history })
    expect(f.transport).toHaveBeenCalledWith('/v1/local-display/tool-result-snapshot', JSON.stringify({ kind: 'present_history', threadId: 'thread-a', turnId: 'turn-a' }), f.owner.signal)
    expect(await f.host.openPresent({ ...effectRequest, index: 0 }, () => true, vi.fn())).toEqual({ ok: false, code: 'unavailable' })
    f.host.close(historyRequest.viewId); expect(f.release).toHaveBeenCalledTimes(1)
  })
  it.each(['owner', 'overflow', 'duplicate', 'body', 'snapshot'] as const)('rejects %s history without retaining a lease', async kind => {
    const f = fixture()
    const body = kind === 'owner' ? JSON.stringify({ ...history, turnId: 'other' }) : kind === 'overflow' ? ' '.repeat((256 << 10) + 1)
      : kind === 'duplicate' ? JSON.stringify({ ...history, orderedItems: [history.orderedItems[0], history.orderedItems[0]] })
      : kind === 'body' ? JSON.stringify({ ...history, orderedItems: [{ ...history.orderedItems[1], text: 'PRIVATE' }] }) : JSON.stringify(f.display)
    f.transport.mockResolvedValue({ ok: true, status: 200, body })
    expect(await f.host.openPresentHistory(historyRequest, f.owner)).toEqual({ ok: false, code: 'unavailable' })
    expect(f.release).toHaveBeenCalledTimes(1)
  })
  it.each(['close', 'revoke', 'invalidate', 'abort'] as const)('masks a late read after %s', async kind => {
    const f = fixture(), controller = new AbortController()
    let resolve!: (value: { ok: boolean; status: number; body: string }) => void
    f.transport.mockImplementation(() => new Promise(value => { resolve = value }))
    const pending = f.host.openPresentHistory(historyRequest, { ...f.owner, signal: controller.signal })
    if (kind === 'close') f.host.close(historyRequest.viewId)
    else if (kind === 'revoke') f.revoke()
    else if (kind === 'invalidate') f.host.invalidate()
    else controller.abort()
    resolve({ ok: true, status: 200, body: JSON.stringify(history) })
    expect(await pending).toEqual({ ok: false, code: 'unavailable' })
  })
  it('does not let a failed old read retire a same-view successor', async () => {
    const f = fixture()
    let resolve!: (value: { ok: boolean; status: number; body: string }) => void
    f.transport.mockImplementationOnce(() => new Promise(value => { resolve = value }))
    const old = f.host.openPresentHistory(historyRequest, f.owner)
    f.transport.mockResolvedValue({ ok: true, status: 200, body: JSON.stringify(history) })
    expect(await f.host.openPresentHistory(historyRequest, f.owner)).toEqual({ ok: true, history })
    resolve({ ok: false, status: 403, body: '' }); expect((await old).ok).toBe(false)
    expect(f.release).toHaveBeenCalledTimes(2); expect(f.notify).not.toHaveBeenCalled()
    f.host.close(historyRequest.viewId); expect(f.release).toHaveBeenCalledTimes(2)
  })
})

it('associates only the same current Office object, preserving unknown candidates and hiding target paths', async () => {
  const f = await presentFixture(), actual = 'd'.repeat(64), wrong = 'e'.repeat(64), unknown = 'f'.repeat(64)
  const otherPath = join(f.root, 'another', 'current.md')
  const resolve = vi.fn(async (payload: unknown): Promise<GeneratedArtifactResponse> => {
    const { artifactId } = payload as { artifactId: string }
    if (artifactId === unknown) return { ok: false, code: 'artifact_unavailable' }
    return { ok: true, artifact: { threadId: selector.threadId, artifactId, path: artifactId === actual ? f.path : otherPath,
      workspace: f.root, kind: 'docx', revision: '1'.repeat(64), byteSize: 42, changed: true } }
  })
  const response = await f.host.associatePresent({ ...f.indexed, artifactIds: [actual, wrong, unknown] }, () => true, resolve)
  expect(response).toEqual({ ok: true, fileIdentity: expect.any(String), artifactIds: [actual] })
  expect(JSON.stringify(response)).not.toContain(f.root)
  expect(resolve.mock.calls.map(([payload]) => payload)).toEqual([actual, wrong, unknown].map(artifactId => ({ threadId: selector.threadId, artifactId })))
  expect(await f.host.associatePresent({ ...f.indexed, artifactIds: [actual] }, () => true)).toMatchObject({ ok: true, artifactIds: [] })
  expect(await f.host.associatePresent({ ...f.indexed, artifactIds: [actual] }, () => true, async () => { throw new Error('unknown') })).toMatchObject({ ok: true, artifactIds: [] })
})
it('association deduplicates a current native object across leases and clears identity when all leases close', async () => {
  const f = await presentFixture(), secondId = '22222222-2222-4222-8222-222222222222'
  expect((await f.host.open({ ...request, viewId: secondId }, f.owner)).ok).toBe(true)
  const first = await f.host.associatePresent({ ...f.indexed, artifactIds: [] }, () => true)
  const second = await f.host.associatePresent({ ...f.indexed, viewId: secondId, artifactIds: [] }, () => true)
  expect(first.ok && second.ok && first.fileIdentity === second.fileIdentity).toBe(true)
  f.host.close(request.viewId)
  expect(await f.host.associatePresent({ ...f.indexed, viewId: secondId, artifactIds: [] }, () => true)).toEqual(second)
  f.host.close(secondId)
  await f.host.open(request, f.owner)
  const renewed = await f.host.associatePresent({ ...f.indexed, artifactIds: [] }, () => true)
  expect(first.ok && renewed.ok && renewed.fileIdentity !== first.fileIdentity).toBe(true)
})
it.each(['revoke', 'replace', 'snapshot'] as const)('association masks a resolver completion after %s', async kind => {
  const f = await presentFixture()
  const resolve = vi.fn(async (): Promise<GeneratedArtifactResponse> => {
    if (kind === 'revoke') f.revoke()
    else if (kind === 'snapshot') f.source.snapshotDigest = 'e'.repeat(64)
    else { await rename(f.path, join(f.root, 'prior.md')); await writeFile(f.path, 'new object') }
    return { ok: false, code: 'artifact_unavailable' }
  })
  expect(await f.host.associatePresent({ ...f.indexed, artifactIds: ['d'.repeat(64)] }, () => true, resolve)).toEqual({ ok: false, code: 'unavailable' })
})
it('association checks the original declaration index and bounded candidate set before resolution', async () => {
  const f = await presentFixture(), resolve = vi.fn()
  for (const extra of [{ index: 1 }, { artifactIds: ['d'.repeat(64), 'd'.repeat(64)] },
    { artifactIds: Array.from({ length: 33 }, (_, i) => i.toString(16).padStart(64, '0')) }, { path: f.path }]) {
    expect(await f.host.associatePresent({ ...f.indexed, artifactIds: [], ...extra }, () => true, resolve)).toEqual({ ok: false, code: 'unavailable' })
  }
  expect(resolve).not.toHaveBeenCalled()
})


it('empty history discovery does not consume ordinary result display capacity', async () => {
  const f = fixture()
  f.transport.mockImplementation(async (_path, body) => JSON.parse(body).kind === 'present_history'
    ? { ok: true, status: 200, body: JSON.stringify({ schemaVersion: 1, kind: 'present_history', threadId: 'thread-a', turnId: JSON.parse(body).turnId,
      orderedItems: [{ kind: 'assistant_text', itemId: 'closing' }] }) }
    : { ok: true, status: 200, body: JSON.stringify(f.display) })
  for (let index = 0; index < 36; index++) {
    expect((await f.host.openPresentHistory({ kind: 'present_history', threadId: 'thread-a', turnId: 'turn-' + index,
      viewId: '11111111-1111-4111-8111-' + index.toString().padStart(12, '0') }, f.owner)).ok).toBe(true)
  }
  expect(f.release).toHaveBeenCalledTimes(36)
  expect((await f.host.open(request, f.owner)).ok).toBe(true)
})


it('resolves each original Present declaration index independently', async () => {
  const f = await presentFixture(), secondPath = join(f.root, 'second.md')
  await writeFile(secondPath, 'second object')
  f.source.capture.present.files.push({ index: 1, path: 'second.md', description: '' })
  f.source.presentTargets.files.push({ index: 1, path: secondPath })
  f.host.close(request.viewId); expect((await f.host.open(request, f.owner)).ok).toBe(true)
  const first = await f.host.associatePresent({ ...f.indexed, artifactIds: [] }, () => true)
  const second = await f.host.associatePresent({ ...f.indexed, index: 1, artifactIds: [] }, () => true)
  expect(first.ok && second.ok && first.fileIdentity !== second.fileIdentity).toBe(true)
  expect(await f.host.previewPresent({ ...f.indexed, index: 1 }, () => true)).toMatchObject({ ok: true, content: 'second object' })
})
it.each(['threadId', 'workspace', 'artifactId'] as const)('does not treat a foreign Office %s as a current object proof', async field => {
  const f = await presentFixture(), artifactId = 'd'.repeat(64)
  const resolve = async (): Promise<GeneratedArtifactResponse> => ({ ok: true, artifact: {
    threadId: field === 'threadId' ? 'other' : selector.threadId, artifactId: field === 'artifactId' ? 'e'.repeat(64) : artifactId,
    workspace: field === 'workspace' ? '/other' : f.root, path: f.path, kind: 'docx', revision: '1'.repeat(64), byteSize: 42, changed: false
  } })
  expect(await f.host.associatePresent({ ...f.indexed, artifactIds: [artifactId] }, () => true, resolve)).toMatchObject({ ok: true, artifactIds: [] })
})


describe('Present protected media bytes', () => {
  it.each([
    ['png', 'image/png'], ['jpg', 'image/jpeg'], ['jpeg', 'image/jpeg'], ['gif', 'image/gif'],
    ['webp', 'image/webp'], ['bmp', 'image/bmp'], ['ico', 'image/x-icon'], ['svg', 'image/svg+xml']
  ])('reads current complete %s bytes from the original sealed index', async (extension, mimeType) => {
    const bytes = Buffer.from([137, 0, 13, 10, 255]), f = await presentFixture(bytes, `current.${extension}`)
    const edited = Buffer.from([137, 0, 13, 10, 254, 9]); await writeFile(f.path, edited)
    expect(await f.host.previewPresentImage(f.indexed, () => true)).toEqual({ ok: true, path: f.path,
      dataUrl: `data:${mimeType};base64,${edited.toString('base64')}`, mimeType, size: edited.length })
    expect((await f.host.previewPresent(f.indexed, () => true)).ok).toBe(false)
  })
  it.each(['image', 'pdf'] as const)('admits the exact %s cap and rejects oversized media without truncating it', async format => {
    const cap = (format === 'image' ? 12 : 64) * 1024 * 1024
    const f = await presentFixture(Buffer.from([1, 0, 2]), format === 'image' ? 'current.png' : 'current.pdf')
    await truncate(f.path, cap)
    const preview = () => format === 'image' ? f.host.previewPresentImage(f.indexed, () => true) : f.host.previewPresentPdf(f.indexed, () => true)
    const value = await preview()
    expect(value).toMatchObject({ ok: true, path: f.path, size: cap })
    if (value.ok) {
      const encoded = 'dataUrl' in value ? value.dataUrl.split(',')[1] : value.dataBase64
      expect(Buffer.from(encoded, 'base64').length).toBe(cap)
      expect('truncated' in value).toBe(false)
    }
    await truncate(f.path, cap + 1)
    expect((await preview()).ok).toBe(false)
  })
  it.each(['image', 'pdf'] as const)('rejects forged %s selectors and wrong Host MIME instead of reading a label path', async format => {
    const f = await presentFixture('not media')
    const preview = (payload: unknown) => format === 'image' ? f.host.previewPresentImage(payload, () => true) : f.host.previewPresentPdf(payload, () => true)
    for (const payload of [f.indexed, { ...f.indexed, path: '/forged.png' }, { ...f.indexed, index: 8 },
      { ...f.indexed, snapshotDigest: 'f'.repeat(64) }]) expect((await preview(payload)).ok).toBe(false)
  })
  it.each(['changed', 'short-read', 'closed-revoke'] as const)('discards media after %s at the descriptor boundary', async change => {
    const f = await presentFixture(Buffer.from([137, 0, 13, 10]), 'current.png')
    const actual = await vi.importActual<typeof import('node:fs/promises')>('node:fs/promises')
    vi.mocked(open).mockImplementationOnce(async (...args) => {
      const handle = await actual.open(...args), read = handle.read.bind(handle), close = handle.close.bind(handle)
      if (change === 'closed-revoke') handle.close = async () => { await close(); f.revoke() }
      else handle.read = (async (buffer: Buffer, offset: number, length: number, position: number | null) => {
        if (change === 'short-read') return { bytesRead: 0, buffer }
        const result = await read(buffer, offset, length, position)
        await writeFile(f.path, Buffer.from([1, 0, 2, 3, 4, 5]))
        return result
      }) as typeof handle.read
      return handle
    })
    expect((await f.host.previewPresentImage(f.indexed, () => true)).ok).toBe(false)
  })
  it.each(['text', 'image', 'pdf', 'excel'] as const)('does not return %s bytes after final-guard microtask revoke or successor', async format => {
    for (const mode of ['revoked', 'successor'] as const) {
      const f = await presentFixture(format === 'text' ? 'current text' : Buffer.from([1, 0, 2]),
        format === 'image' ? 'current.png' : format === 'pdf' ? 'current.pdf' : format === 'excel' ? 'current.csv' : 'current.md')
      const observed = await lstat(f.path, { bigint: true })
      let complete!: (value: typeof observed) => void, reached!: () => void
      const waiting = new Promise<void>(resolve => { reached = resolve })
      // Complete media reads finish with an extra linked-version stat.
      for (let index = 0; index < (format === 'text' ? 3 : 4); index++) vi.mocked(lstat).mockResolvedValueOnce(observed as never)
      vi.mocked(lstat).mockImplementationOnce(() => {
        reached(); return new Promise(resolve => { complete = resolve }) as never
      })
      const preview = () => format === 'text' ? f.host.previewPresent(f.indexed, () => true)
        : format === 'image' ? f.host.previewPresentImage(f.indexed, () => true)
        : format === 'excel' ? f.host.previewPresentExcel(f.indexed, () => true) : f.host.previewPresentPdf(f.indexed, () => true)
      const pending = preview(); await waiting
      let successor: ReturnType<typeof f.host.open> | undefined
      const successorRelease = vi.fn()
      complete(observed)
      queueMicrotask(() => {
        if (mode === 'revoked') f.revoke()
        else successor = f.host.open(request, { ...f.owner, release: successorRelease })
      })
      expect((await pending).ok).toBe(false)
      if (mode === 'successor') {
        expect(await successor).toMatchObject({ ok: true }); expect(successorRelease).not.toHaveBeenCalled()
        expect((await preview()).ok).toBe(true)
      }
    }
  })
})


const excelSelectedRange = { sheetIndex: 0, sheetId: '1', ranges: [{ row: [0, 1] as [number, number], column: [0, 1] as [number, number] }] }
function excelCopyFrame(cells: ExcelCopyFrameV1['cells'] = [[0, 0, { m: 'Display', v: 'stored' }]], extra: Partial<ExcelCopyFrameV1> = {}): ExcelCopyFrameV1 {
  const frame: ExcelCopyFrameV1 = { schemaVersion: 1, op: 'excel-preview-copy-cells-v1', sheetIndex: 0, sheetId: '1',
    rowCount: 2, columnCount: 2, ranges: [{ row: [0, 1], column: [0, 1] }], ...extra, cells }
  frame.html = formatExcelCopyHtmlV1({ name: 'Synthetic Host fixture', id: frame.sheetId, hide: 0,
    row: frame.rowCount, column: frame.columnCount, celldata: cells.map(([r, c, item]) => {
      const v: Cell = { m: item.m, v: item.v, mc: item.mc }
      if (item.numberFormat !== undefined || item.cellType !== undefined) v.ct = { fa: item.numberFormat, t: item.cellType }
      return { r, c, v }
    }) }, { sheetIndex: frame.sheetIndex, sheetId: frame.sheetId, ranges: frame.ranges })
  return frame
}

it.each(['xlsx', 'xls', 'csv', 'tsv'] as const)('reads complete protected %s bytes with a Host-selected format and independent content digest', async format => {
  const bytes = Buffer.from([1, 0, 2, 3]), f = await presentFixture(bytes, `current.${format}`)
  expect(await f.host.previewPresentExcel(f.indexed, () => true)).toMatchObject({ ok: true, path: f.path,
    format, size: bytes.length, dataBase64: bytes.toString('base64'),
    contentDigest: createHash('sha256').update(bytes).digest('hex'),
    limits: { maxBytes: 16 << 20, maxUncompressedBytes: 64 << 20, maxCells: 250_000, timeoutMs: 15_000 } })
})

it('copies only Host-converted selected saved display text through the existing privacy projection', async () => {
  const source = 'synthetic saved workbook bytes', cells: ExcelCopyFrameV1['cells'] = [
    [0, 0, { m: 'Display', v: 'ignored stored value' }],
    [0, 1, { m: 'account: 6222020202020202020' }],
    [1, 0, { m: '0', v: 0 }], [1, 1, { m: 'line\n"quoted"' }]
  ]
  const read = vi.fn(async (bytes: Uint8Array<ArrayBuffer>, format: string, selected: unknown) => {
    expect(Buffer.from(bytes).toString()).toBe(source)
    expect(format).toBe('xlsx'); expect(selected).toEqual(excelSelectedRange)
    return excelCopyFrame(cells)
  })
  const f = await presentFixture(source, 'current.xlsx', read), copy = vi.fn(), save = vi.fn(), pick = vi.fn(async () => null)
  const contentDigest = createHash('sha256').update(source).digest('hex')
  expect(await f.host.effect({ ...f.indexed, contentDigest, selection: excelSelectedRange }, 'copy', () => true, pick, copy, save)).toEqual({ ok: true })
  expect(copy).toHaveBeenCalledExactlyOnceWith('Display\taccount: [ACCOUNT]\n0\t"line\n""quoted"""', expect.any(String))
  expect(pick).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled(); expect(read).toHaveBeenCalledOnce()
})

it('rejects a stale content digest before starting the converter', async () => {
  const read = vi.fn(async () => excelCopyFrame()), f = await presentFixture('current bytes', 'current.csv', read)
  const copy = vi.fn()
  expect(await f.host.effect({ ...f.indexed, contentDigest: 'd'.repeat(64), selection: excelSelectedRange }, 'copy', () => true,
    async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unavailable' })
  expect(read).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled()
})

it('rejects an edit while parsing instead of copying a prior file snapshot', async () => {
  let replace!: () => Promise<void>
  const read = vi.fn(async () => { await replace(); return excelCopyFrame() })
  const source = 'before edit', f = await presentFixture(source, 'current.csv', read), copy = vi.fn()
  replace = () => writeFile(f.path, 'after edit')
  expect(await f.host.effect({ ...f.indexed, contentDigest: createHash('sha256').update(source).digest('hex'), selection: excelSelectedRange },
    'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unavailable' })
  expect(read).toHaveBeenCalledOnce(); expect(copy).not.toHaveBeenCalled()
})

it('does not inherit a same-view successor after the converter await', async () => {
  let replace!: () => Promise<unknown>
  const read = vi.fn(async () => { await replace(); return excelCopyFrame() })
  const source = 'same saved bytes', f = await presentFixture(source, 'current.csv', read), copy = vi.fn()
  replace = () => f.host.open(request, { ...f.owner, release: vi.fn() })
  expect(await f.host.effect({ ...f.indexed, contentDigest: createHash('sha256').update(source).digest('hex'), selection: excelSelectedRange },
    'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unavailable' })
  expect(read).toHaveBeenCalledOnce(); expect(copy).not.toHaveBeenCalled()
})

it.each([
  { path: '/forged.csv' }, { format: 'csv' }, { body: 'renderer body' }, { section: 'result' },
  { text: 'renderer text' }, { html: '<table>renderer HTML</table>' }, { checkedHtml: '<table>forged checked HTML</table>' },
  { fileIndex: 0 }, { index: 1 }, { selection: { ...excelSelectedRange, range: excelSelectedRange.ranges[0] } },
  { selection: { ...excelSelectedRange, ranges: [{ row: [1, 0], column: [0, 1] }] } }
])('rejects forged or malformed selected Copy input %j with zero effects', async extra => {
  const read = vi.fn(async () => excelCopyFrame()), source = 'current bytes', f = await presentFixture(source, 'current.csv', read), copy = vi.fn()
  expect(await f.host.effect({ ...f.indexed, contentDigest: createHash('sha256').update(source).digest('hex'),
    selection: excelSelectedRange, ...extra }, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unavailable' })
  expect(read).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled()
})

it.each([['private', '<think>synthetic private reasoning</think>'], ['oversized', 'x'.repeat((1 << 20) + 1)]])('refuses %s selected output with zero clipboard effects', async (_label, value) => {
  const read = vi.fn(async () => { const frame = excelCopyFrame(); frame.cells = [[0, 0, { m: value }]]; return frame }), source = 'source bytes', f = await presentFixture(source, 'current.csv', read), copy = vi.fn()
  expect(await f.host.effect({ ...f.indexed, contentDigest: createHash('sha256').update(source).digest('hex'), selection: excelSelectedRange },
    'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unsupported' })
  expect(copy).not.toHaveBeenCalled()
})


const excelNumberFormatCases: Array<{ label: string; cell: ExcelCopyFrameV1['cells'][number][2]; expected: string }> = [
  { label: 'W uses stored value', cell: { numberFormat: 'W', m: 'saved display', v: 42 }, expected: '42' },
  { label: 'W0 uses stored value', cell: { numberFormat: 'W0', m: 'saved display', v: 42 }, expected: '42' },
  { label: 'W0.00 uses stored value', cell: { numberFormat: 'W0.00', m: 'saved display', v: 42 }, expected: '42' },
  { label: 'w uses stored value', cell: { numberFormat: 'w', m: 'saved display', v: 42 }, expected: '42' },
  { label: 'w0 uses stored value', cell: { numberFormat: 'w0', m: 'saved display', v: 42 }, expected: '42' },
  { label: 'w0.0 uses stored value', cell: { numberFormat: 'w0.0', m: 'saved display', v: 42 }, expected: '42' },
  { label: 'special format preserves zero', cell: { numberFormat: 'W0', m: 'not the zero value', v: 0 }, expected: '0' },
  { label: 'special format preserves false', cell: { numberFormat: 'w0.00', m: 'not the false value', v: false }, expected: 'false' },
  { label: 'special format missing cache stays empty', cell: { numberFormat: 'W', m: 'stale display without a stored cache' }, expected: '' },
  { label: 'General keeps saved display', cell: { numberFormat: 'General', m: 'saved display', v: 42 }, expected: 'saved display' },
  { label: 'ordinary numeric format keeps saved display', cell: { numberFormat: '#,##0.00', m: 'saved display', v: 42 }, expected: 'saved display' },
  { label: 'near-match W1 keeps saved display', cell: { numberFormat: 'W1', m: 'saved display', v: 42 }, expected: 'saved display' },
  { label: 'near-match W0.01 keeps saved display', cell: { numberFormat: 'W0.01', m: 'saved display', v: 42 }, expected: 'saved display' },
  { label: 'suffixed W0.00 keeps saved display', cell: { numberFormat: 'W0.00suffix', m: 'saved display', v: 42 }, expected: 'saved display' },
  { label: 'leading-space W keeps saved display', cell: { numberFormat: ' W', m: 'saved display', v: 42 }, expected: 'saved display' },
  { label: 'absent format keeps saved display', cell: { m: 'saved display', v: 42 }, expected: 'saved display' },
  { label: 'nil display falls back to zero', cell: { numberFormat: 'General', v: 0 }, expected: '0' },
  { label: 'nil display falls back to false', cell: { numberFormat: 'General', v: false }, expected: 'false' },
  { label: 'empty display is preserved', cell: { numberFormat: 'General', m: '', v: 42 }, expected: '' },
  { label: 'missing display and cache stays empty', cell: { numberFormat: 'General' }, expected: '' },
  { label: 'd cell keeps its saved date display', cell: { cellType: 'd', numberFormat: 'General', m: 'displayed date', v: 46500 }, expected: 'displayed date' },
  { label: 'd cell overrides W raw-value branch', cell: { cellType: 'd', numberFormat: 'W0.00', m: 'displayed date', v: 0 }, expected: 'displayed date' },
  { label: 'd cell with nil display does not fall back to numeric cache', cell: { cellType: 'd', numberFormat: 'General', v: 46500 }, expected: '' },
  { label: 'd cell with nil display does not fall back to zero', cell: { cellType: 'd', numberFormat: 'W', v: 0 }, expected: '' },
  { label: 'd cell with nil display does not fall back to false', cell: { cellType: 'd', numberFormat: 'General', v: false }, expected: '' },
  { label: 'd cell empty display is preserved', cell: { cellType: 'd', numberFormat: 'W0', m: '', v: 46500 }, expected: '' }
]
it.each(excelNumberFormatCases)('matches the admitted Copy value branch: $label', async ({ cell, expected }) => {
  const selected = { sheetIndex: 0, sheetId: '1', ranges: [{ row: [0, 0] as [number, number], column: [0, 0] as [number, number] }] }
  const read = vi.fn(async () => excelCopyFrame([[0, 0, cell]], { ranges: selected.ranges }))
  const source = 'synthetic saved workbook bytes', f = await presentFixture(source, 'current.xlsx', read), copy = vi.fn()
  expect(await f.host.effect({ ...f.indexed, contentDigest: createHash('sha256').update(source).digest('hex'), selection: selected },
    'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: true })
  expect(copy).toHaveBeenCalledExactlyOnceWith(expected, expect.any(String))
  expect(read).toHaveBeenCalledOnce()
})


const excelRangeOrderCases: Array<{ label: string; ranges: ExcelCopyFrameV1['ranges']; cells: ExcelCopyFrameV1['cells']; expected: string }> = [
  { label: 'horizontal first-seen columns', ranges: [{ row: [0, 0], column: [2, 2] }, { row: [0, 0], column: [0, 0] }],
    cells: [[0, 0, { m: 'Alpha' }], [0, 2, { m: 'Charlie' }]], expected: 'Charlie\tAlpha' },
  { label: 'vertical first-seen rows', ranges: [{ row: [2, 2], column: [0, 0] }, { row: [0, 0], column: [0, 0] }],
    cells: [[0, 0, { m: 'Alpha' }], [2, 0, { m: 'Charlie' }]], expected: 'Charlie\nAlpha' }
]
it.each(excelRangeOrderCases)('copies aligned non-overlapping ranges in original order: $label', async ({ ranges, cells, expected }) => {
  const selected = { sheetIndex: 0, sheetId: '1', ranges }
  const read = vi.fn(async () => excelCopyFrame(cells, { rowCount: 3, columnCount: 3, ranges }))
  const source = 'synthetic workbook bytes', f = await presentFixture(source, 'current.xlsx', read), copy = vi.fn()
  expect(await f.host.effect({ ...f.indexed, contentDigest: createHash('sha256').update(source).digest('hex'), selection: selected },
    'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: true })
  expect(copy).toHaveBeenCalledExactlyOnceWith(expected, expect.any(String))
})

it.each([
  { label: 'empty ranges', ranges: [] },
  { label: 'diagonal ranges', ranges: [{ row: [0, 0], column: [0, 0] }, { row: [1, 1], column: [1, 1] }] },
  { label: 'horizontal closed overlap', ranges: [{ row: [0, 1], column: [0, 1] }, { row: [0, 1], column: [1, 2] }] },
  { label: 'vertical closed overlap', ranges: [{ row: [0, 1], column: [0, 1] }, { row: [1, 2], column: [0, 1] }] },
  { label: 'Cartesian area above budget', ranges: [{ row: [0, 999], column: [0, 124] }, { row: [0, 999], column: [125, 250] }] }
])('rejects unsupported range shape before parsing: $label', async ({ ranges }) => {
  const read = vi.fn(async () => excelCopyFrame()), source = 'synthetic workbook bytes', f = await presentFixture(source, 'current.xlsx', read), copy = vi.fn()
  expect(await f.host.effect({ ...f.indexed, contentDigest: createHash('sha256').update(source).digest('hex'),
    selection: { sheetIndex: 0, sheetId: '1', ranges } }, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unavailable' })
  expect(read).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled()
})


it.each(['missing', 'mismatch', 'script', 'remote-style'] as const)('withholds both protected Excel outputs for %s HTML', async variant => {
  const value = excelCopyFrame()
  if (variant === 'missing') delete value.html
  else if (variant === 'mismatch') value.html = value.html!.replace('Display', 'different child body')
  else if (variant === 'script') value.html = value.html!.replace('Display</td>', '<script>Display</script></td>')
  else value.html = value.html!.replace('height:19px;', 'background:url(https://example.invalid);')
  const source = 'synthetic saved workbook bytes', read = vi.fn(async () => value)
  const f = await presentFixture(source, 'current.csv', read), copy = vi.fn()
  expect(await f.host.effect({ ...f.indexed, contentDigest: createHash('sha256').update(source).digest('hex'),
    selection: excelSelectedRange }, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unsupported' })
  expect(copy).not.toHaveBeenCalled()
})


it.each(['horizontal', 'vertical'] as const)('rejects split identifiers across %s cells with zero protected clipboard effects', async direction => {
  const source = direction === 'horizontal' ? '62220202,02020202\n,' : '62220202,\n02020202,'
  const cells: ExcelCopyFrameV1['cells'] = [[0, 0, { m: '62220202' }],
    [direction === 'vertical' ? 1 : 0, direction === 'horizontal' ? 1 : 0, { m: '02020202' }]]
  const read = vi.fn(async () => excelCopyFrame(cells)), f = await presentFixture(source, 'current.csv', read), copy = vi.fn()
  expect(await f.host.effect({ ...f.indexed, contentDigest: createHash('sha256').update(source).digest('hex'),
    selection: excelSelectedRange }, 'copy', () => true, async () => null, copy, vi.fn())).toEqual({ ok: false, code: 'unsupported' })
  expect(copy).not.toHaveBeenCalled()
})


describe('protected Present original file save', () => {
  it.each(['binary', 'empty'] as const)('saves the complete %s bytes at the user-selected destination', async kind => {
    const bytes = kind === 'binary' ? Buffer.concat([Buffer.alloc(1_500_001, 65), Buffer.from([0, 255, 13, 10, 137])]) : Buffer.alloc(0)
    const f = await presentFixture(bytes, 'original.bin'), destination = join(f.root, 'user-chosen.bin')
    const pick = vi.fn(async () => destination), copy = vi.fn(), save = vi.fn(saveToolResultLocalFileV1)
    expect(await f.host.effect(f.indexed, 'save', () => true, pick, copy, save)).toEqual({ ok: true })
    expect(pick).toHaveBeenCalledWith('original.bin')
    expect(save).toHaveBeenCalledWith(destination, bytes, { check: expect.any(Function), isCurrent: expect.any(Function) })
    expect(await readFile(destination)).toEqual(bytes)
    expect(copy).not.toHaveBeenCalled()
    expect((await readdir(f.root)).filter(name => name.startsWith('.analytix-save-'))).toEqual([])
  })
  it('cancels before reading or writing bytes and keeps the lease usable', async () => {
    const f = await presentFixture(), copy = vi.fn(), save = vi.fn()
    vi.mocked(open).mockClear()
    expect(await f.host.effect(f.indexed, 'save', () => true, async () => null, copy, save)).toEqual({ ok: false, code: 'canceled' })
    expect(open).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled()
    expect((await f.host.previewPresent(f.indexed, () => true)).ok).toBe(true)
  })
  it('rejects a file exceeding the complete-export cap before opening the dialog', async () => {
    const f = await presentFixture(), pick = vi.fn(), save = vi.fn()
    await truncate(f.path, TOOL_RESULT_LOCAL_PRESENT_SAVE_MAX_BYTES_V1 + 1)
    vi.mocked(open).mockClear()
    expect(await f.host.effect(f.indexed, 'save', () => true, pick, vi.fn(), save)).toEqual({ ok: false, code: 'unsupported' })
    expect(pick).not.toHaveBeenCalled(); expect(open).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  })
  it.each(['thread', 'turn', 'call', 'result', 'view', 'digest', 'index', 'path', 'bytes'] as const)('rejects forged %s before any dialog or write', async field => {
    const f = await presentFixture(), pick = vi.fn(), save = vi.fn()
    const changes = { thread: { threadId: 'foreign' }, turn: { turnId: 'foreign' }, call: { callId: 'call_host_' + 'e'.repeat(64) },
      result: { resultItemId: 'item_result_' + 'e'.repeat(64) }, view: { viewId: '22222222-2222-4222-8222-222222222222' },
      digest: { snapshotDigest: 'e'.repeat(64) }, index: { index: 1 }, path: { path: f.path }, bytes: { dataBase64: 'Zm9yZ2Vk' } }
    expect(await f.host.effect({ ...f.indexed, ...changes[field] }, 'save', () => true, pick, vi.fn(), save)).toEqual({ ok: false, code: 'unavailable' })
    expect(pick).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  })
  it('does not reinterpret an ordinary text capture as a Present file request', async () => {
    const f = fixture(), pick = vi.fn(), save = vi.fn()
    await f.host.open(request, f.owner)
    expect(await f.host.effect({ ...effectRequest, index: 0 }, 'save', () => true, pick, vi.fn(), save)).toEqual({ ok: false, code: 'unavailable' })
    expect(pick).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  })
  it.each(['edited', 'replaced', 'symlink', 'target', 'digest', 'revoked', 'successor'] as const)('refuses %s during the save dialog with zero destination effects', async change => {
    const f = await presentFixture(), save = vi.fn(), copy = vi.fn()
    const pick = vi.fn(async () => {
      if (change === 'edited') await writeFile(f.path, 'X'.repeat('original content'.length))
      else if (change === 'replaced' || change === 'symlink') {
        const original = join(f.root, 'before.md'); await rename(f.path, original)
        if (change === 'symlink') await symlink(original, f.path)
        else await writeFile(f.path, 'original content')
      } else if (change === 'target') {
        const other = join(f.root, 'other.md'); await writeFile(other, 'other content')
        f.source.presentTargets.files[0].path = other
      } else if (change === 'digest') f.source.snapshotDigest = 'e'.repeat(64)
      else if (change === 'revoked') f.revoke()
      else await f.host.open(request, { ...f.owner, release: vi.fn() })
      return join(f.root, 'destination.md')
    })
    expect(await f.host.effect(f.indexed, 'save', () => true, pick, copy, save)).toEqual({ ok: false, code: 'unavailable' })
    expect(pick).toHaveBeenCalledOnce(); expect(save).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled()
  })
  it.each(['short-read', 'edited', 'closed-revoke'] as const)('discards staged original bytes after %s', async change => {
    const f = await presentFixture(Buffer.from([0, 1, 255, 13, 10])), save = vi.fn()
    const actual = await vi.importActual<typeof import('node:fs/promises')>('node:fs/promises')
    vi.mocked(open).mockImplementationOnce(async (...args) => {
      const handle = await actual.open(...args), read = handle.read.bind(handle), close = handle.close.bind(handle)
      if (change === 'closed-revoke') handle.close = async () => { await close(); f.revoke() }
      else handle.read = (async (buffer: Buffer, offset: number, length: number, position: number | null) => {
        if (change === 'short-read') return { bytesRead: 0, buffer }
        const result = await read(buffer, offset, length, position)
        await writeFile(f.path, Buffer.from([0, 1, 254, 13, 10]))
        return result
      }) as typeof handle.read
      return handle
    })
    expect(await f.host.effect(f.indexed, 'save', () => true, async () => join(f.root, 'destination'), vi.fn(), save)).toEqual({ ok: false, code: 'unavailable' })
    expect(save).not.toHaveBeenCalled()
  })
  it.each(['revoked', 'successor'] as const)('rejects final-guard microtask %s before starting a write', async mode => {
    const f = await presentFixture(), save = vi.fn(), observed = await lstat(f.path, { bigint: true })
    let complete!: (value: typeof observed) => void, reached!: () => void
    const waiting = new Promise<void>(resolve => { reached = resolve })
    // Selection, pre-dialog, pre-read, post-read, post-close guard + version,
    // final guard + version. Revoke after that last stat resolves.
    for (let index = 0; index < 7; index++) vi.mocked(lstat).mockResolvedValueOnce(observed as never)
    vi.mocked(lstat).mockImplementationOnce(() => { reached(); return new Promise(resolve => { complete = resolve }) as never })
    const pending = f.host.effect(f.indexed, 'save', () => true, async () => join(f.root, 'destination'), vi.fn(), save)
    await waiting
    let successor: ReturnType<typeof f.host.open> | undefined
    complete(observed)
    queueMicrotask(() => { if (mode === 'revoked') f.revoke(); else successor = f.host.open(request, { ...f.owner, release: vi.fn() }) })
    expect(await pending).toEqual({ ok: false, code: 'unavailable' }); expect(save).not.toHaveBeenCalled()
    if (successor) expect((await successor).ok).toBe(true)
  })
  it.each(['revoked', 'edited'] as const)('keeps an existing destination intact when %s while staging the native write', async change => {
    const f = await presentFixture(Buffer.from([0, 1, 255])), destination = join(f.root, 'chosen.bin')
    await writeFile(destination, 'keep existing destination')
    const save = vi.fn(async (path: string, bytes: string | Uint8Array, guard?: Parameters<typeof saveToolResultLocalFileV1>[2]) => {
      let checks = 0
      await saveToolResultLocalFileV1(path, bytes, guard && { ...guard, check: async () => {
        if (++checks === 2) {
          if (change === 'revoked') f.revoke()
          else await writeFile(f.path, Buffer.from([0, 1, 254]))
        }
        return guard.check()
      } })
    })
    expect(await f.host.effect(f.indexed, 'save', () => true, async () => destination, vi.fn(), save)).toEqual({ ok: false, code: 'unavailable' })
    expect(await readFile(destination, 'utf8')).toBe('keep existing destination')
    expect((await readdir(f.root)).filter(name => name.startsWith('.analytix-save-'))).toEqual([])
  })
  it('cleans staging files on a failed destination commit', async () => {
    const f = await presentFixture(), guard = { check: async () => true, isCurrent: () => true }
    const destination = join(f.root, 'existing-directory'); await mkdir(destination)
    await expect(saveToolResultLocalFileV1(destination, Buffer.from('complete bytes'), guard)).rejects.toThrow()
    expect((await readdir(f.root)).filter(name => name.startsWith('.analytix-save-'))).toEqual([])
  })
})

it('resolves complete HTML beyond the source cap and binds its version and signal', async () => {
  const bytes = Buffer.from('<!doctype html><!--' + 'x'.repeat(1_600_000) + '--><p>COMPLETE_TAIL</p>')
  const f = await presentFixture(bytes, 'current.html'), controller = new AbortController()
  f.owner.signal = controller.signal
  const cap = await f.host.resolvePresentHtml(f.indexed, () => true)
  if (!cap) throw Error('Expected complete current HTML snapshot')
  expect(cap.path).toBe(f.path); expect(cap.workspaceRoot).toBe(f.root)
  expect(cap.bytes).toEqual(bytes); expect(cap.digest).toBe(createHash('sha256').update(bytes).digest('hex'))
  expect(await cap.guard()).toBe(true); expect(cap.isCurrent()).toBe(true)
  await writeFile(f.path, '<html>CHANGED_VERSION</html>')
  expect(await cap.guard()).toBe(false)
  controller.abort(); expect(cap.signal.aborted).toBe(true); expect(cap.isCurrent()).toBe(false)
})
it('discards HTML whose descriptor version changes during the complete read', async () => {
  const f = await presentFixture('<html>ORIGINAL</html>', 'current.html')
  const actual = await vi.importActual<typeof import('node:fs/promises')>('node:fs/promises')
  vi.mocked(open).mockImplementationOnce(async (...args) => {
    const handle = await actual.open(...args), read = handle.read.bind(handle)
    let changed = false
    handle.read = (async (buffer: Buffer, offset: number, length: number, position: number | null) => {
      const result = await read(buffer, offset, length, position)
      if (!changed) { changed = true; await writeFile(f.path, '<html>CHANGED_AND_LONGER</html>') }
      return result
    }) as typeof handle.read
    return handle
  })
  expect(await f.host.resolvePresentHtml(f.indexed, () => true)).toBeNull()
})
it('rejects forged and oversized Present HTML before a grant can exist', async () => {
  const f = await presentFixture('<html>current</html>', 'current.htm')
  expect(await f.host.resolvePresentHtml({ ...f.indexed, path: f.path }, () => true)).toBeNull()
  expect(await f.host.resolvePresentHtml({ ...f.indexed, snapshotDigest: 'd'.repeat(64) }, () => true)).toBeNull()
  await truncate(f.path, (8 << 20) + 1)
  expect(await f.host.resolvePresentHtml(f.indexed, () => true)).toBeNull()
})

it.each(['docx','pptx'])('resolves complete protected %s bytes and retires a changed file or original signal', async kind => {
  const bytes = Buffer.concat([Buffer.from([80,75,3,4,0]),Buffer.alloc(1_600_000,1)]), f = await presentFixture(bytes,`current.${kind}`)
  const controller = new AbortController(); f.owner.signal = controller.signal
  const cap = await f.host.resolvePresentOffice(f.indexed,() => true)
  if (!cap) throw Error('Expected complete current Office snapshot')
  expect(cap.kind).toBe(kind); expect(cap.selector).toEqual(f.indexed); expect(cap.path).toBe(f.path)
  expect(cap.bytes).toEqual(bytes); expect(cap.digest).toBe(createHash('sha256').update(bytes).digest('hex'))
  expect(await cap.guard()).toBe(true)
  await writeFile(f.path,Buffer.from([80,75,3,4,2])); expect(await cap.guard()).toBe(false)
  controller.abort(); expect(cap.signal.aborted).toBe(true); expect(cap.isCurrent()).toBe(false)
})
it('refuses forged, spreadsheet and oversized Office Present selectors', async () => {
  const f = await presentFixture(Buffer.from([80,75,3,4,0]),'current.docx')
  expect(await f.host.resolvePresentOffice({...f.indexed,path:f.path},() => true)).toBeNull()
  expect(await f.host.resolvePresentOffice({...f.indexed,snapshotDigest:'d'.repeat(64)},() => true)).toBeNull()
  await truncate(f.path,(16 << 20)+1); expect(await f.host.resolvePresentOffice(f.indexed,() => true)).toBeNull()
  const xlsx = await presentFixture(Buffer.from([80,75,3,4,0]),'current.xlsx')
  expect(await xlsx.host.resolvePresentOffice(xlsx.indexed,() => true)).toBeNull()
})


it('keeps generic success text inside the original Inspect readonly lease and refuses copy/save separately', async () => {
  const f = fixture(), source = { ...f.display, toolName: 'mcp__docs__lookup',
    capture: { kind: 'inspect', status: 'completed', body: '', label: '', durationMs: 0, truncated: false, startLine: 0, endLine: 0, totalLines: 0 },
    inspect: { ...inspectFacts, schemaJSON: undefined, outputText: { availability: 'available', body: 'actual readonly OUT', classification: 'source_exact_local', truncated: false } } }
  f.transport.mockImplementation(async () => ({ ok: true, status: 200, body: JSON.stringify(source) }))
  expect(await f.host.open(request, f.owner)).toMatchObject({ ok: true, display: { inspect: { outputText: source.inspect.outputText } } })
  const pick = vi.fn(async () => '/synthetic-unused'), copy = vi.fn(), save = vi.fn()
  for (const section of ['result', 'payload', 'schema'] as const) expect(await f.host.effect({ ...effectRequest, section }, 'copy', () => true, pick, copy, save)).toEqual({ ok: false, code: 'unsupported' })
  expect(await f.host.effect(effectRequest, 'save', () => true, pick, copy, save)).toEqual({ ok: false, code: 'unsupported' })
  expect(pick).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  expect(await f.host.effect({ ...effectRequest, snapshotDigest: 'd'.repeat(64) }, 'copy', () => true, pick, copy, save)).toEqual({ ok: false, code: 'unavailable' })
  f.revoke(); expect((await f.host.open(request, f.owner)).ok).toBe(false)
})
