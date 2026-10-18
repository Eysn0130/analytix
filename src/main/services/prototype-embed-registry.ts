import type { WebContents } from 'electron'
import { randomBytes, createHash } from 'node:crypto'
import { constants } from 'node:fs'
import { open, realpath, stat } from 'node:fs/promises'
import { dirname, extname, isAbsolute, relative, resolve, sep } from 'node:path'

export const PROTOTYPE_PREVIEW_SCHEME = 'analytix-prototype'
export const PROTOTYPE_PREVIEW_PRIVILEGES = {
  standard: true, secure: true, supportFetchAPI: true, corsEnabled: true
} as const
const MAX_AUTHORIZED_PROTOTYPES = 256
const MAX_RESOURCE_BYTES = 8 << 20
const ATTACH_TIMEOUT_MS = 60_000
export type PrototypePreviewMode = 'write-prototype' | 'workspace-html'
export type PrototypePreviewOwner = {
  id: number; isCurrent: () => boolean; release: () => void
  validate?: () => Promise<boolean>; signal?: AbortSignal
}
export type PrototypePreviewSnapshot = {
  path: string; workspaceRoot: string; bytes: Buffer; digest: string
  guard: () => Promise<boolean>; isCurrent: () => boolean; signal: AbortSignal
}
export type PrototypePreviewGrant = {
  grantId: string; absolutePath: string; fileUrl: string; partition: string
}
type SessionControl = { session: object; dispose: () => void | Promise<void> }
type Grant = PrototypePreviewGrant & {
  owner: PrototypePreviewOwner; root: string; digest: string; protectedPresent: boolean
  session: SessionControl; reserved: boolean; guestId?: number; closeGuest?: () => void
  timer?: ReturnType<typeof setTimeout>; abort?: () => void
}
const grants = new Map<string, Grant>()
const prototypeSessions = new WeakSet<object>()
let sessionFactory: ((grant: PrototypePreviewGrant) => SessionControl) | null = null

const mimeTypes: Record<string, string> = {
  '.css': 'text/css', '.js': 'text/javascript', '.mjs': 'text/javascript',
  '.png': 'image/png', '.jpg': 'image/jpeg', '.jpeg': 'image/jpeg', '.gif': 'image/gif',
  '.webp': 'image/webp', '.svg': 'image/svg+xml', '.ico': 'image/x-icon', '.bmp': 'image/bmp',
  '.woff': 'font/woff', '.woff2': 'font/woff2', '.ttf': 'font/ttf', '.otf': 'font/otf'
}
export const PROTOTYPE_PREVIEW_CSP = [
  "default-src 'none'", "script-src 'self' 'unsafe-inline'", "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob:", "font-src 'self' data:", "connect-src 'none'",
  "frame-src 'none'", "object-src 'none'", "worker-src 'none'", "form-action 'none'", "base-uri 'none'"
].join('; ')

function within(root: string, path: string): boolean {
  const value = relative(root, path)
  return value === '' || value !== '..' && !value.startsWith('..' + sep) && !isAbsolute(value)
}
function deniedResourceName(path: string): boolean {
  return path.replaceAll('\\', '/').split('/').some(segment =>
    segment.startsWith('.') || segment === 'node_modules' ||
    /(?:^|[._-])(?:secrets?|credentials?|tokens?|config(?:uration)?)(?:[._-]|$)/i.test(segment))
}
function digest(bytes: Uint8Array): string { return createHash('sha256').update(bytes).digest('hex') }

async function readBoundedFile(path: string, root: string): Promise<Buffer> {
  const handle = await open(path, constants.O_RDONLY | (constants.O_NOFOLLOW ?? 0))
  try {
    const before = await handle.stat()
    if (!before.isFile() || before.size > MAX_RESOURCE_BYTES) throw Error('preview-resource-unavailable')
    const bytes = Buffer.alloc(before.size)
    let offset = 0
    while (offset < bytes.length) {
      const { bytesRead } = await handle.read(bytes, offset, bytes.length - offset, offset)
      if (!bytesRead) throw Error('preview-resource-unavailable')
      offset += bytesRead
    }
    const after = await handle.stat()
    if (after.size !== before.size || after.mtimeMs !== before.mtimeMs || after.ino !== before.ino) throw Error('preview-resource-unavailable')
    // O_NOFOLLOW covers the leaf. Recheck the named path and fd identity to
    // reject an ancestor redirected between realpath and open.
    const namedPath = await realpath(path)
    if (namedPath !== path || !within(root, namedPath)) throw Error('preview-resource-unavailable')
    const named = await stat(namedPath)
    if (named.dev !== after.dev || named.ino !== after.ino || named.size !== after.size || named.mtimeMs !== after.mtimeMs ||
      await realpath(path) !== namedPath) throw Error('preview-resource-unavailable')
    return bytes
  } finally { await handle.close() }
}

export async function resolvePrototypePath(path: string, workspaceRoot: string, mode: PrototypePreviewMode = 'write-prototype'):
Promise<{ ok: true; absolutePath: string; root: string; bytes: Buffer } | { ok: false; message: string }> {
  try {
    if (!/\.html?$/i.test(path.trim())) return { ok: false, message: 'Only HTML files can run in this preview.' }
    const workspace = await realpath(resolve(workspaceRoot))
    const requested = resolve(workspace, path)
    const absolutePath = await realpath(requested)
    if (!within(workspace, absolutePath) || !/\.html?$/i.test(absolutePath)) throw Error('outside-workspace')
    if (mode === 'write-prototype' && !absolutePath.replaceAll('\\', '/').split('/').includes('proto')) {
      return { ok: false, message: 'Prototypes must live in a proto directory.' }
    }
    if (deniedResourceName(relative(workspace, absolutePath).replaceAll('\\', '/').replace(/^\.analytixsdd\//, ''))) {
      return { ok: false, message: 'Private and configuration files cannot run in this preview.' }
    }
    const bytes = await readBoundedFile(absolutePath, workspace)
    if (bytes.includes(0)) return { ok: false, message: 'This file cannot run as HTML.' }
    return { ok: true, absolutePath, root: dirname(absolutePath), bytes }
  } catch { return { ok: false, message: 'The HTML file is unavailable within the selected workspace.' } }
}

export function setPrototypeSessionFactory(factory: (grant: PrototypePreviewGrant) => SessionControl): void {
  sessionFactory = factory
}
function current(id: string): Grant | undefined {
  const grant = grants.get(id)
  if (grant && (grant.owner.signal?.aborted || !grant.owner.isCurrent())) { revokePrototypeGrant(id, grant.owner.id); return undefined }
  return grant
}
function grantForUrl(src: string): Grant | undefined {
  try {
    const url = new URL(src)
    if (url.protocol !== PROTOTYPE_PREVIEW_SCHEME + ':' || url.username || url.password || url.port) return undefined
    return current(url.hostname)
  } catch { return undefined }
}
async function validateOwner(owner: PrototypePreviewOwner): Promise<boolean> {
  try {
    return !owner.signal?.aborted && owner.isCurrent() &&
      (!owner.validate || await owner.validate()) && !owner.signal?.aborted && owner.isCurrent()
  } catch { return false }
}
async function createPrototypeGrant(absolutePath: string, root: string, bytes: Buffer, owner: PrototypePreviewOwner, protectedPresent = false):
Promise<({ ok: true } & PrototypePreviewGrant) | { ok: false; message: string }> {
  const denied = { ok: false as const, message: 'HTML preview is unavailable.' }
  if (!await validateOwner(owner) || !sessionFactory) { owner.release(); return denied }
  const grantId = randomBytes(24).toString('hex')
  const info: PrototypePreviewGrant = { grantId, absolutePath,
    fileUrl: PROTOTYPE_PREVIEW_SCHEME + '://' + grantId + '/' + encodeURIComponent(relative(root, absolutePath)),
    partition: 'analytix-proto-' + grantId }
  let control: SessionControl
  try { control = sessionFactory(info) } catch { owner.release(); return denied }
  if (!await validateOwner(owner)) {
    owner.release(); void Promise.resolve(control.dispose()).catch(() => undefined); return denied
  }
  const grant: Grant = { ...info, owner, root, digest: digest(bytes), protectedPresent, session: control, reserved: false }
  grants.set(grantId, grant); prototypeSessions.add(control.session)
  grant.abort = () => { revokePrototypeGrant(grantId, owner.id) }
  owner.signal?.addEventListener('abort', grant.abort, { once: true })
  if (!current(grantId)) return denied
  grant.timer = setTimeout(() => revokePrototypeGrant(grantId, owner.id), ATTACH_TIMEOUT_MS)
  grant.timer.unref?.()
  if (grants.size > MAX_AUTHORIZED_PROTOTYPES) {
    const oldest = grants.values().next().value as Grant | undefined
    if (oldest) revokePrototypeGrant(oldest.grantId, oldest.owner.id)
  }
  return { ok: true, ...info }
}
export async function authorizePrototypePath(path: string, workspaceRoot: string,
  owner: PrototypePreviewOwner, mode: PrototypePreviewMode = 'write-prototype'):
Promise<({ ok: true } & PrototypePreviewGrant) | { ok: false; message: string }> {
  const resolved = await resolvePrototypePath(path, workspaceRoot, mode)
  if (!resolved.ok) { owner.release(); return resolved }
  return createPrototypeGrant(resolved.absolutePath, resolved.root, resolved.bytes, owner)
}
/** Only Main's complete sealed descriptor snapshot can create a Present grant. */
export async function authorizePrototypeSnapshot(snapshot: PrototypePreviewSnapshot, owner: PrototypePreviewOwner):
Promise<({ ok: true } & PrototypePreviewGrant) | { ok: false; message: string }> {
  const denied = { ok: false as const, message: 'Protected HTML preview is unavailable.' }
  const bound: PrototypePreviewOwner = { ...owner, signal: snapshot.signal,
    isCurrent: () => owner.isCurrent() && snapshot.isCurrent(),
    validate: async () => (!owner.validate || await owner.validate()) && await snapshot.guard() }
  try {
    const workspace = await realpath(snapshot.workspaceRoot)
    if (!isAbsolute(snapshot.path) || !within(workspace, snapshot.path) || !/\.html?$/i.test(snapshot.path) ||
      deniedResourceName(relative(workspace, snapshot.path).replaceAll('\\', '/').replace(/^\.analytixsdd\//, '')) ||
      snapshot.bytes.length > MAX_RESOURCE_BYTES || snapshot.bytes.includes(0) || digest(snapshot.bytes) !== snapshot.digest ||
      !await validateOwner(bound)) { owner.release(); return denied }
    return await createPrototypeGrant(snapshot.path, dirname(snapshot.path), snapshot.bytes, bound, true)
  } catch { owner.release(); return denied }
}
export function revokePrototypeGrant(grantId: string, ownerId: number): boolean {
  const grant = grants.get(grantId)
  if (!grant || grant.owner.id !== ownerId) return false
  grants.delete(grantId)
  if (grant.timer) clearTimeout(grant.timer)
  if (grant.abort) grant.owner.signal?.removeEventListener('abort', grant.abort)
  grant.owner.release()
  try { grant.closeGuest?.() } catch { /* Already retired guest. */ }
  try { void Promise.resolve(grant.session.dispose()).catch(() => undefined) } catch { /* Denial remains active. */ }
  return true
}
export function isAuthorizedPrototypeUrl(src: string, ownerId: number, partition: string): boolean {
  const grant = grantForUrl(src)
  return Boolean(grant && grant.owner.id === ownerId && grant.partition === partition && !grant.reserved &&
    src.split(/[?#]/, 1)[0] === grant.fileUrl)
}
export function reservePrototypeGuest(src: string, ownerId: number, partition: string): boolean {
  if (!isAuthorizedPrototypeUrl(src, ownerId, partition)) return false
  grantForUrl(src)!.reserved = true
  return true
}
export function isPrototypeSession(session: object): boolean { return prototypeSessions.has(session) }
export function bindPrototypeGuest(ownerId: number, session: object, guestId: number, closeGuest: () => void): PrototypePreviewGrant | null {
  for (const grant of grants.values()) {
    if (grant.owner.id !== ownerId || grant.session.session !== session || !grant.reserved || grant.guestId !== undefined || !current(grant.grantId)) continue
    grant.guestId = guestId; grant.closeGuest = closeGuest
    if (grant.timer) clearTimeout(grant.timer)
    return { grantId: grant.grantId, absolutePath: grant.absolutePath, fileUrl: grant.fileUrl, partition: grant.partition }
  }
  return null
}
export function isPrototypeRequestAllowed(grantId: string, url: string, method: string): boolean {
  return (method === 'GET' || method === 'HEAD') && grantForUrl(url)?.grantId === grantId
}
// Only a Main-sealed Present snapshot selects this guest policy. Ordinary
// previews retain their existing interaction behavior and no bridge is added.
const protectedTransferGuard = `(() => {
  const apply = Reflect.apply;
  const prevent = Event.prototype.preventDefault;
  const stop = Event.prototype.stopImmediatePropagation;
  const clear = DataTransfer.prototype.clearData;
  const block = event => {
    if (event.clipboardData) apply(clear, event.clipboardData, []);
    if (event.dataTransfer) apply(clear, event.dataTransfer, []);
    apply(prevent, event, []); apply(stop, event, []);
  };
  for (const type of ['copy', 'cut', 'dragstart']) window.addEventListener(type, block, true);
})()`
export function installProtectedPrototypeGuestGuards(grantId: string,
  guest: Pick<WebContents, 'on' | 'setIgnoreMenuShortcuts' | 'executeJavaScriptInIsolatedWorld'>): void {
  const grant = current(grantId)
  if (!grant?.protectedPresent) return
  guest.setIgnoreMenuShortcuts(true)
  guest.on('before-input-event', (event, input) => {
    if ((input.control || input.meta) && !input.alt && ['c', 'x'].includes(input.key.toLowerCase())) event.preventDefault()
  })
  guest.on('context-menu', event => event.preventDefault())
  guest.on('dom-ready', () => {
    if (current(grantId) !== grant) return
    // An isolated world keeps page script from replacing the guard intrinsics.
    void guest.executeJavaScriptInIsolatedWorld(1004, [{ code: protectedTransferGuard }])
      .catch(() => { if (grants.get(grantId) === grant) revokePrototypeGrant(grantId, grant.owner.id) })
  })
}
export function isPrototypeGrantCurrent(grantId: string): boolean { return current(grantId) !== undefined }
async function validateGrant(grantId: string, grant: Grant): Promise<boolean> {
  if (current(grantId) === grant && await validateOwner(grant.owner) && current(grantId) === grant) return true
  if (grants.get(grantId) === grant) revokePrototypeGrant(grantId, grant.owner.id)
  return false
}
export async function readPrototypeResource(grantId: string, src: string, method: string):
Promise<{ ok: true; bytes: Buffer; mimeType: string } | { ok: false }> {
  if (!isPrototypeRequestAllowed(grantId, src, method)) return { ok: false }
  const grant = current(grantId)
  if (!grant || !await validateGrant(grantId, grant)) return { ok: false }
  try {
    const url = new URL(src)
    const segments = url.pathname.split('/').filter(Boolean).map(segment => decodeURIComponent(segment))
    if (!segments.length || segments.some(segment => segment === '.' || segment === '..' || /[/\\:\0]/.test(segment)) ||
      deniedResourceName(segments.join('/'))) return { ok: false }
    const lexical = resolve(grant.root, ...segments)
    if (!within(grant.root, lexical)) return { ok: false }
    const path = await realpath(lexical)
    if (!within(grant.root, path) || deniedResourceName(relative(grant.root, path))) return { ok: false }
    const isDocument = path === grant.absolutePath
    const mimeType = isDocument ? 'text/html' : mimeTypes[extname(path).toLowerCase()]
    if (!mimeType) return { ok: false }
    const bytes = await readBoundedFile(path, grant.root)
    if (!await validateGrant(grantId, grant) || current(grantId) !== grant || isDocument && digest(bytes) !== grant.digest) { bytes.fill(0); return { ok: false } }
    return { ok: true, bytes: method === 'HEAD' ? Buffer.alloc(0) : bytes, mimeType }
  } catch { return { ok: false } }
}
export function clearAuthorizedPrototypes(): void {
  for (const grant of [...grants.values()]) revokePrototypeGrant(grant.grantId, grant.owner.id)
}
