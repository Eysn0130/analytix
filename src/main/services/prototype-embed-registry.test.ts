import { createHash } from 'node:crypto'
import { runInNewContext } from 'node:vm'
import { mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync, symlinkSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  authorizePrototypePath, authorizePrototypeSnapshot, isPrototypeGrantCurrent, installProtectedPrototypeGuestGuards,
  clearAuthorizedPrototypes,
  isAuthorizedPrototypeUrl, setPrototypeSessionFactory, reservePrototypeGuest, bindPrototypeGuest,
  revokePrototypeGrant, isPrototypeRequestAllowed, readPrototypeResource
} from './prototype-embed-registry'

let workspace: string
let active = true
const release = vi.fn(), dispose = vi.fn(), closeGuest = vi.fn()
const sessions: object[] = []
const owner = { id: 12, isCurrent: () => active, release }
const authorize = (path: string, mode: 'write-prototype' | 'workspace-html' = 'write-prototype') =>
  authorizePrototypePath(join(workspace, path), workspace, owner, mode)

describe('prototype embed registry', () => {
  beforeEach(() => {
    active = true; release.mockClear(); dispose.mockClear(); closeGuest.mockClear(); sessions.length = 0
    setPrototypeSessionFactory(() => { const session = {}; sessions.push(session); return { session, dispose } })
    workspace = realpathSync(mkdtempSync(join(tmpdir(), 'proto-registry-')))
    mkdirSync(join(workspace, '.analytixsdd', 'proto'), { recursive: true })
    writeFileSync(join(workspace, '.analytixsdd', 'proto', 'page.html'), '<html></html>')
  })

  afterEach(() => {
    clearAuthorizedPrototypes()
    rmSync(workspace, { recursive: true, force: true })
  })

  function sealedSnapshot(name: string) {
    const path = join(workspace, name), bytes = Buffer.from('<html>SEALED_' + name + '</html>')
    writeFileSync(path, bytes)
    const controller = new AbortController(), guard = vi.fn(async () => true)
    return { controller, guard, snapshot: { path, workspaceRoot: workspace, bytes,
      digest: createHash('sha256').update(bytes).digest('hex'), guard,
      isCurrent: () => active && !controller.signal.aborted, signal: controller.signal } }
  }
  it('permanently retires only the sealed grant whose resource pre/post validation fails', async () => {
    const a = sealedSnapshot('before.htm'), b = sealedSnapshot('after.htm')
    const first = await authorizePrototypeSnapshot(a.snapshot, owner), second = await authorizePrototypeSnapshot(b.snapshot, owner)
    if (!first.ok || !second.ok) throw Error('Expected two sealed grants')
    a.guard.mockResolvedValue(false)
    expect(await readPrototypeResource(first.grantId, first.fileUrl, 'GET')).toEqual({ ok: false })
    expect(isPrototypeGrantCurrent(first.grantId)).toBe(false); expect(isPrototypeGrantCurrent(second.grantId)).toBe(true)
    b.guard.mockReset(); b.guard.mockResolvedValueOnce(true).mockResolvedValue(false)
    expect(await readPrototypeResource(second.grantId, second.fileUrl, 'GET')).toEqual({ ok: false })
    expect(b.guard).toHaveBeenCalledTimes(2); expect(isPrototypeGrantCurrent(second.grantId)).toBe(false)
    a.guard.mockResolvedValue(true); b.guard.mockResolvedValue(true)
    expect(await readPrototypeResource(first.grantId, first.fileUrl, 'GET')).toEqual({ ok: false })
    expect(await readPrototypeResource(second.grantId, second.fileUrl, 'GET')).toEqual({ ok: false })
  })
  it('aborts a bound sealed grant during validation without retiring its sibling', async () => {
    const a = sealedSnapshot('a.htm'), b = sealedSnapshot('b.htm')
    const first = await authorizePrototypeSnapshot(a.snapshot, owner), second = await authorizePrototypeSnapshot(b.snapshot, owner)
    if (!first.ok || !second.ok) throw Error('Expected two sealed grants')
    expect(reservePrototypeGuest(first.fileUrl, owner.id, first.partition)).toBe(true)
    expect(bindPrototypeGuest(owner.id, sessions[0], 701, closeGuest)).not.toBeNull()
    let finish!: (value: boolean) => void, reached!: () => void
    const started = new Promise<void>(resolve => { reached = resolve })
    a.guard.mockImplementationOnce(() => { reached(); return new Promise<boolean>(resolve => { finish = resolve }) })
    const reading = readPrototypeResource(first.grantId, first.fileUrl, 'GET'); await started
    a.controller.abort()
    expect(isPrototypeGrantCurrent(first.grantId)).toBe(false); expect(closeGuest).toHaveBeenCalledOnce()
    expect(dispose).toHaveBeenCalledOnce(); expect(isPrototypeGrantCurrent(second.grantId)).toBe(true)
    finish(true); expect(await reading).toEqual({ ok: false })
    expect(await readPrototypeResource(second.grantId, second.fileUrl, 'GET')).toMatchObject({ ok: true, bytes: b.snapshot.bytes })
  })
  it('protects only Main-sealed guests while retaining ordinary button interaction', async () => {
    const sealed = sealedSnapshot('guarded.htm'), grant = await authorizePrototypeSnapshot(sealed.snapshot, owner)
    if (!grant.ok) throw Error('Expected protected grant')
    const handlers = new Map<string, (...args: unknown[]) => void>()
    const execute = vi.fn(async (_world: number, _scripts: { code: string }[]) => undefined), ignore = vi.fn()
    const guest = { on: (name: string, handler: (...args: unknown[]) => void) => { handlers.set(name, handler) },
      setIgnoreMenuShortcuts: ignore, executeJavaScriptInIsolatedWorld: execute }
    installProtectedPrototypeGuestGuards(grant.grantId, guest as never)
    expect(ignore).toHaveBeenCalledWith(true)
    const blocked = { preventDefault: vi.fn() }
    handlers.get('before-input-event')!(blocked, { control: true, meta: false, alt: false, key: 'c' })
    expect(blocked.preventDefault).toHaveBeenCalledOnce()
    const movement = { preventDefault: vi.fn() }
    handlers.get('before-input-event')!(movement, { control: false, meta: false, alt: false, key: 'ArrowDown' })
    expect(movement.preventDefault).not.toHaveBeenCalled()
    handlers.get('dom-ready')!()
    expect(execute).toHaveBeenCalledOnce(); expect(execute.mock.calls[0][0]).toBe(1004)
    const transfers = new Map<string, (event: FakeEvent) => void>()
    class Transfer { cleared = false; clearData() { this.cleared = true } }
    class FakeEvent {
      prevented = false; stopped = false; clipboardData = new Transfer(); dataTransfer = new Transfer()
      preventDefault() { this.prevented = true }; stopImmediatePropagation() { this.stopped = true }
    }
    runInNewContext(execute.mock.calls[0][1][0].code, { window: { addEventListener: (name: string, listener: (event: FakeEvent) => void) => transfers.set(name, listener) }, Event: FakeEvent, DataTransfer: Transfer })
    expect([...transfers.keys()]).toEqual(['copy', 'cut', 'dragstart'])
    for (const listener of transfers.values()) {
      const event = new FakeEvent(); listener(event)
      expect(event.prevented && event.stopped && event.clipboardData.cleared && event.dataTransfer.cleared).toBe(true)
    }
    sealed.controller.abort(); handlers.get('dom-ready')!(); expect(execute).toHaveBeenCalledOnce()
    const ordinary = await authorize('.analytixsdd/proto/page.html')
    if (!ordinary.ok) throw Error('Expected ordinary grant')
    const unchanged = { on: vi.fn(), setIgnoreMenuShortcuts: vi.fn(), executeJavaScriptInIsolatedWorld: vi.fn() }
    installProtectedPrototypeGuestGuards(ordinary.grantId, unchanged as never)
    expect(unchanged.on).not.toHaveBeenCalled(); expect(unchanged.setIgnoreMenuShortcuts).not.toHaveBeenCalled()
  })
  it('authorizes a contained prototype and allow-lists its file url', async () => {
    const result = await authorizePrototypePath(
      join(workspace, '.analytixsdd', 'proto', 'page.html'),
      workspace, owner
    )
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.fileUrl.startsWith('analytix-prototype://')).toBe(true)
    expect(isAuthorizedPrototypeUrl(result.fileUrl, owner.id, result.partition)).toBe(true)
  })

  it('rejects non-html files and files outside a proto directory', async () => {
    writeFileSync(join(workspace, '.analytixsdd', 'proto', 'notes.txt'), 'x')
    writeFileSync(join(workspace, 'loose.html'), '<html></html>')

    const txt = await authorizePrototypePath(join(workspace, '.analytixsdd', 'proto', 'notes.txt'), workspace, owner)
    expect(txt.ok).toBe(false)

    const loose = await authorizePrototypePath(join(workspace, 'loose.html'), workspace, owner)
    expect(loose).toMatchObject({ ok: false, message: expect.stringContaining('proto directory') })
  })

  it('rejects paths escaping the workspace and missing files', async () => {
    const escaped = await authorizePrototypePath('/tmp/.analytixsdd/proto/evil.html', workspace, owner)
    expect(escaped.ok).toBe(false)

    const missing = await authorizePrototypePath(
      join(workspace, '.analytixsdd', 'proto', 'gone.html'),
      workspace, owner
    )
    expect(missing).toMatchObject({ ok: false, message: expect.stringContaining('unavailable') })
  })

  it('only admits exact previously-authorized file urls', () => {
    expect(isAuthorizedPrototypeUrl('file:///anything.html', owner.id, '')).toBe(false)
    expect(isAuthorizedPrototypeUrl('https://example.com', owner.id, '')).toBe(false)
  })
})


describe('isolated HTML grants', () => {
  beforeEach(() => {
    active = true; release.mockClear(); dispose.mockClear(); closeGuest.mockClear(); sessions.length = 0
    setPrototypeSessionFactory(() => { const session = {}; sessions.push(session); return { session, dispose } })
    workspace = realpathSync(mkdtempSync(join(tmpdir(), 'html-grants-')))
    writeFileSync(join(workspace, 'page.htm'), '<html></html>')
  })
  afterEach(() => { clearAuthorizedPrototypes(); rmSync(workspace, { recursive: true, force: true }) })
  it('admits ordinary HTML only in explicit workspace mode', async () => {
    expect((await authorize('page.htm')).ok).toBe(false)
    expect((await authorize('page.htm', 'workspace-html')).ok).toBe(true)
  })
  it('binds unique nonpersistent sessions to the exact owner and a single guest', async () => {
    const first = await authorize('page.htm', 'workspace-html'), second = await authorize('page.htm', 'workspace-html')
    if (!first.ok || !second.ok) throw Error('fixture admission')
    expect(first.partition).not.toBe(second.partition); expect(first.partition.startsWith('persist:')).toBe(false)
    expect(isAuthorizedPrototypeUrl(first.fileUrl, 99, first.partition)).toBe(false)
    expect(reservePrototypeGuest(first.fileUrl, 12, second.partition)).toBe(false)
    expect(reservePrototypeGuest(first.fileUrl, 12, first.partition)).toBe(true)
    expect(reservePrototypeGuest(first.fileUrl, 12, first.partition)).toBe(false)
    expect(bindPrototypeGuest(99, sessions[0], 7, closeGuest)).toBeNull()
    expect(bindPrototypeGuest(12, sessions[0], 7, closeGuest)?.grantId).toBe(first.grantId)
    expect(bindPrototypeGuest(12, sessions[0], 8, closeGuest)).toBeNull()
    expect(revokePrototypeGrant(first.grantId, 99)).toBe(false)
    expect(revokePrototypeGrant(first.grantId, 12)).toBe(true)
    expect(closeGuest).toHaveBeenCalledOnce(); expect(dispose).toHaveBeenCalledOnce()
    expect(isPrototypeRequestAllowed(first.grantId, first.fileUrl, 'GET')).toBe(false)
    expect(isPrototypeRequestAllowed(second.grantId, second.fileUrl, 'GET')).toBe(true)
  })
  it('rejects realpath escapes, hidden/configuration files and non-HTML aliases', async () => {
    const outside = realpathSync(mkdtempSync(join(tmpdir(), 'html-outside-')))
    try {
      writeFileSync(join(outside, 'escape.html'), 'outside'); symlinkSync(join(outside, 'escape.html'), join(workspace, 'escape.html'))
      writeFileSync(join(workspace, 'config.html'), 'private'); writeFileSync(join(workspace, 'credential.txt'), 'secret')
      symlinkSync(join(workspace, 'credential.txt'), join(workspace, 'alias.html'))
      mkdirSync(join(workspace, '.private')); writeFileSync(join(workspace, '.private', 'page.html'), 'private')
      for (const path of ['gone.html', 'escape.html', 'config.html', '.private/page.html', 'alias.html']) {
        expect((await authorize(path, 'workspace-html')).ok, path).toBe(false)
      }
    } finally { rmSync(outside, { recursive: true, force: true }) }
  })
  it('serves only bounded child web assets and the unchanged primary page', async () => {
    const page = await authorize('page.htm', 'workspace-html'); if (!page.ok) throw Error('fixture admission')
    const base = new URL('.', page.fileUrl).href
    mkdirSync(join(workspace, 'assets')); writeFileSync(join(workspace, 'assets', 'style.css'), 'body {}')
    writeFileSync(join(workspace, 'other.html'), '<html>other</html>')
    writeFileSync(join(workspace, 'config.js'), 'SECRET'); writeFileSync(join(workspace, 'data.json'), 'SECRET')
    expect(await readPrototypeResource(page.grantId, base + 'assets/style.css', 'GET')).toMatchObject({ ok: true, mimeType: 'text/css' })
    // URL dot segments normalize within the grant origin, never the host filesystem.
    expect(await readPrototypeResource(page.grantId, base + '../page.htm', 'GET')).toMatchObject({ ok: true, mimeType: 'text/html' })
    expect(await readPrototypeResource(page.grantId, page.fileUrl, 'HEAD')).toMatchObject({ ok: true, bytes: Buffer.alloc(0) })
    for (const name of ['other.html', 'config.js', 'data.json', '%2e%2e%2fescape.js']) {
      expect(await readPrototypeResource(page.grantId, base + name, 'GET'), name).toEqual({ ok: false })
    }
    for (const url of ['file:///etc/passwd', 'https://example.test/a.js', 'http://127.0.0.1/a.js', 'ws://127.0.0.1', 'analytix-prototype://different/a.js']) {
      expect(isPrototypeRequestAllowed(page.grantId, url, 'GET')).toBe(false)
    }
    const outside = realpathSync(mkdtempSync(join(tmpdir(), 'html-resource-outside-')))
    try {
      writeFileSync(join(outside, 'external.js'), 'SECRET')
      symlinkSync(join(outside, 'external.js'), join(workspace, 'external.js'))
      expect(await readPrototypeResource(page.grantId, base + 'external.js', 'GET')).toEqual({ ok: false })
    } finally { rmSync(outside, { recursive: true, force: true }) }
    expect(isPrototypeRequestAllowed(page.grantId, page.fileUrl, 'POST')).toBe(false)
    writeFileSync(join(workspace, 'big.js'), Buffer.alloc((8 << 20) + 1))
    expect(await readPrototypeResource(page.grantId, base + 'big.js', 'GET')).toEqual({ ok: false })
    writeFileSync(join(workspace, 'page.htm'), '<html>changed</html>')
    expect(await readPrototypeResource(page.grantId, page.fileUrl, 'GET')).toEqual({ ok: false })
  })
  it('irreversibly retires an exited document and expires unused grants', async () => {
    const page = await authorize('page.htm', 'workspace-html'); if (!page.ok) throw Error('fixture admission')
    active = false; expect(isPrototypeRequestAllowed(page.grantId, page.fileUrl, 'GET')).toBe(false)
    active = true; expect(isPrototypeRequestAllowed(page.grantId, page.fileUrl, 'GET')).toBe(false)
    expect(dispose).toHaveBeenCalledOnce(); expect(release).toHaveBeenCalledOnce()
    vi.useFakeTimers()
    try {
      const next = await authorize('page.htm', 'workspace-html'); if (!next.ok) throw Error('fixture admission')
      await vi.advanceTimersByTimeAsync(60_000)
      expect(isPrototypeRequestAllowed(next.grantId, next.fileUrl, 'GET')).toBe(false)
    } finally { vi.useRealTimers() }
  })
})
