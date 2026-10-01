import { createHash } from 'node:crypto'
import privateLayout from '../../../packages/runtime-go/internal/adapters/outbound/officeengineassets/private-local-layout.json'
import engineManifest from '../../../packages/runtime-go/internal/adapters/outbound/officeengineassets/manifest.json'
import { constants } from 'node:fs'
import { lstat, open, realpath } from 'node:fs/promises'
import { isAbsolute, join, relative, resolve, sep } from 'node:path'
import { createServer, type Server } from 'node:http'

type Asset = { name: string; path: string; bytes: number; sha256: string; mime: string }
export type OfficeBuffer = { bytes: Buffer; mime: string }
const paths = {
  'soffice.js': ['soffice.js', 'text/javascript'],
  'soffice.wasm': ['soffice.wasm', 'application/wasm'],
  'soffice.data': ['soffice.data', 'application/octet-stream'],
  'soffice.data.js.metadata': ['soffice.data.js.metadata', 'application/json'],
  'NotoSansCJKsc-Regular.otf': ['fonts/NotoSansCJKsc-Regular.otf', 'font/otf'],
  'zeta.js': ['zetajs-57360bcb0e7726ffa0e66567c8041261b959f8dd/source/zeta.js', 'text/javascript']
} as const
export const officeAssetFailure = () => Error('office-assets-unavailable')
const hash = (b: Buffer) => createHash('sha256').update(b).digest('hex')
function exact(r: Record<string, unknown>, keys: string[]) { return Object.keys(r).sort().join(',') === keys.sort().join(',') }
function record(v: unknown): v is Record<string, unknown> { return !!v && typeof v === 'object' && !Array.isArray(v) }

// A descriptor provides the held bytes; pre/post identity checks reject changed paths.
// Engine bytes additionally must match the single source-owned manifest.
async function readHeld(root: string, name: string, max: number): Promise<Buffer> {
  if (!isAbsolute(root) || resolve(root) !== root || await realpath(root) !== root) throw officeAssetFailure()
  const target = join(root, name), rel = relative(root, target)
  if (!rel || rel.startsWith('..' + sep) || isAbsolute(rel)) throw officeAssetFailure()
  let current = root
  for (const part of rel.split(sep).slice(0, -1)) {
    current = join(current, part)
    const st = await lstat(current)
    if (!st.isDirectory() || st.isSymbolicLink()) throw officeAssetFailure()
  }
  const before = await lstat(target)
  if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1 || before.size < 1 || before.size > max) throw officeAssetFailure()
  const fd = await open(target, constants.O_RDONLY | (constants.O_NOFOLLOW ?? 0))
  try {
    const st = await fd.stat()
    if (!st.isFile() || st.dev !== before.dev || st.ino !== before.ino || st.size !== before.size || st.nlink !== 1 || st.mtimeMs !== before.mtimeMs || st.ctimeMs !== before.ctimeMs) throw officeAssetFailure()
    const buffer = Buffer.alloc(st.size + 1)
    let count = 0
    while (count < buffer.length) {
      const read = await fd.read(buffer, count, buffer.length - count, count)
      if (!read.bytesRead) break
      count += read.bytesRead
    }
    const after = await fd.stat(), pathAfter = await lstat(target)
    if (count !== st.size || after.nlink !== 1 || pathAfter.nlink !== 1 || after.size !== st.size || after.mtimeMs !== st.mtimeMs || after.ctimeMs !== st.ctimeMs || pathAfter.dev !== st.dev || pathAfter.ino !== st.ino || pathAfter.size !== st.size || pathAfter.mtimeMs !== st.mtimeMs || pathAfter.ctimeMs !== st.ctimeMs || pathAfter.isSymbolicLink() || await realpath(target) !== target) throw officeAssetFailure()
    return buffer.subarray(0, count)
  } finally { await fd.close() }
}
export async function loadOfficeBuffers(sourceRoot: string, assetRoot: string): Promise<Map<string, OfficeBuffer>> {
  try {
    const raw = await readHeld(sourceRoot, 'packages/runtime-go/internal/adapters/outbound/officeengineassets/manifest.json', 16384)
    const manifest: unknown = JSON.parse(raw.toString('utf8'))
    // The source manifest has one canonical encoding; this also rejects duplicate keys.
    if (JSON.stringify(manifest, null, 2) + '\n' !== raw.toString('utf8') || !record(manifest) || !exact(manifest, ['schemaVersion', 'executionMode', 'publishable', 'buildId', 'assets']) || manifest.schemaVersion !== 1 || manifest.executionMode !== 'source-experiment' || manifest.publishable !== false || !/^[a-f0-9]{40}$/.test(String(manifest.buildId)) || !Array.isArray(manifest.assets) || manifest.assets.length !== Object.keys(paths).length) throw officeAssetFailure()
    const buffers = new Map<string, OfficeBuffer>(), seen = new Set<string>()
    let total = 0
    for (const entry of manifest.assets) {
      if (!record(entry) || !exact(entry, ['name', 'path', 'bytes', 'sha256', 'mime']) || !Object.hasOwn(paths, String(entry.name))) throw officeAssetFailure()
      const asset = entry as Asset, expected = paths[asset.name as keyof typeof paths]
      if (seen.has(asset.name) || asset.path !== expected[0] || asset.mime !== expected[1] || !Number.isSafeInteger(asset.bytes) || asset.bytes <= 0 || asset.bytes > 192 * 1024 * 1024 || !/^[a-f0-9]{64}$/.test(asset.sha256)) throw officeAssetFailure()
      seen.add(asset.name); total += asset.bytes
      if (total > 300 * 1024 * 1024) throw officeAssetFailure()
      const bytes = await readHeld(assetRoot, asset.path, asset.bytes)
      if (bytes.length !== asset.bytes || hash(bytes) !== asset.sha256) throw officeAssetFailure()
      buffers.set('/assets/' + asset.name, { bytes, mime: asset.mime })
    }
    for (const name of ['office-surface.html', 'office-surface.js', 'office-worker.js']) {
      buffers.set('/' + name, { bytes: await readHeld(sourceRoot, 'src/main/office/surface/' + name, 1024 * 1024), mime: name.endsWith('.html') ? 'text/html; charset=utf-8' : 'text/javascript; charset=utf-8' })
    }
    return buffers
  } catch { throw officeAssetFailure() }
}
export type OfficeAssetServer = { server: Server; origin: string; entryURL: string; permits: (url: string, method: string) => boolean; destroy: () => void }
export async function startOfficeAssetServer(buffers: Map<string, OfficeBuffer>, nonce: string): Promise<OfficeAssetServer> {
  if (!/^[a-f0-9-]{36}$/.test(nonce)) throw officeAssetFailure()
  const prefix = '/' + nonce, routes = new Map([...buffers].map(([p, b]) => [prefix + p, b]))
  let origin = '', destroyed = false
  const permits = (url: string, method: string) => {
    try { const u = new URL(url); return !destroyed && method === 'GET' && u.origin === origin && !u.username && !u.password && !u.search && !u.hash && routes.has(u.pathname) && url === origin + u.pathname } catch { return false }
  }
  const server = createServer((req, res) => {
    const full = origin + req.url
    if (req.headers.host !== origin.slice(7) || (req.headers.origin && req.headers.origin !== origin) || !permits(full, req.method ?? '')) { res.writeHead(404).end(); return }
    const asset = routes.get(new URL(full).pathname)!
    res.writeHead(200, {
      'Content-Type': asset.mime, 'Content-Length': asset.bytes.length,
      'Cross-Origin-Opener-Policy': 'same-origin', 'Cross-Origin-Embedder-Policy': 'require-corp',
      'Cross-Origin-Resource-Policy': 'same-origin', 'X-Content-Type-Options': 'nosniff',
      'Cache-Control': 'no-store',
      'Content-Security-Policy': "default-src 'none'; script-src 'self' 'unsafe-eval'; worker-src 'self' blob:; connect-src 'self'; img-src 'self' data: blob:; style-src 'unsafe-inline'; font-src 'self' data:; frame-src 'none'; frame-ancestors 'none'; form-action 'none'; base-uri 'none'; object-src 'none'"
    })
    res.end(asset.bytes)
  })
  server.requestTimeout = 5000; server.headersTimeout = 5000; server.keepAliveTimeout = 1000
  await new Promise<void>((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', () => { server.removeListener('error', reject); resolve() }) })
  const address = server.address()
  if (!address || typeof address === 'string') { server.close(); throw officeAssetFailure() }
  origin = 'http://127.0.0.1:' + address.port
  return { server, origin, entryURL: origin + prefix + '/office-surface.html', permits, destroy() { if (destroyed) return; destroyed = true; server.closeAllConnections(); server.close(); routes.clear(); buffers.clear() } }
}

// These are source-owned, fixed package layout inputs; qualification.json is
// accepted only when its complete digest was witnessed by the current Core.
const privateDomain = Buffer.from('AnalytixOfficePrivateLocalV1\0')
const privateKeys = ['schemaVersion','contract','usage','publishable','releaseEligible','targetKey','sourceCommit','worktreeSnapshotDigest','engineBuildId','engineManifestSha256','files','qualificationDigest']
const hex = (value: unknown, length: number): value is string => typeof value === 'string' && new RegExp(`^[a-f0-9]{${length}}$`).test(value)
type PrivateFile = { path: string; byteLength: number; sha256: string }
export type PrivateOfficeBuffers = { buffers: Map<string, OfficeBuffer>; preloadPath: string }

export function parsePrivateOfficeQualification(raw:Buffer, qualificationDigest:string):ReadonlyMap<string,PrivateFile> {
  try {
    if(raw.length>64*1024 || !hex(qualificationDigest,64))throw officeAssetFailure()
    const q: unknown = JSON.parse(raw.toString('utf8'))
    if (!record(q) || !exact(q,privateKeys) || !Buffer.from(JSON.stringify(q)+'\n').equals(raw) ||
        q.schemaVersion !== 1 || q.contract !== 'analytix.office-private-local/v1' || q.usage !== 'private-local' || q.publishable !== false || q.releaseEligible !== false ||
        q.targetKey !== 'darwin-arm64' || !hex(q.sourceCommit,40) || !hex(q.worktreeSnapshotDigest,64) ||
        q.engineBuildId !== privateLayout.buildId || q.engineManifestSha256 !== privateLayout.manifestSha256 || q.qualificationDigest !== qualificationDigest ||
        !Array.isArray(q.files) || q.files.length !== 35 || privateLayout.files.length !== 35) throw officeAssetFailure()
    // Rebuild the canonical ordered body, so reordered fields or duplicate keys
    // cannot acquire a different interpretation under the same Core witness.
    const body = {schemaVersion:q.schemaVersion,contract:q.contract,usage:q.usage,publishable:q.publishable,releaseEligible:q.releaseEligible,targetKey:q.targetKey,
      sourceCommit:q.sourceCommit,worktreeSnapshotDigest:q.worktreeSnapshotDigest,engineBuildId:q.engineBuildId,engineManifestSha256:q.engineManifestSha256,files:q.files}
    if (!Buffer.from(JSON.stringify({...body,qualificationDigest})+'\n').equals(raw) ||
        createHash('sha256').update(privateDomain).update(JSON.stringify(body)).digest('hex') !== qualificationDigest) throw officeAssetFailure()
    const files = new Map<string,PrivateFile>()
    let total = 0, previous = ''
    for (let index=0;index<q.files.length;index++) {
      const file:unknown=q.files[index], expected=privateLayout.files[index]
      if (!record(file) || !exact(file,['path','byteLength','sha256']) || file.path !== expected.path || expected.path <= previous ||
          !Number.isSafeInteger(file.byteLength) || Number(file.byteLength)<=0 || Number(file.byteLength)>expected.maximum || !hex(file.sha256,64) ||
          (expected.byteLength !== null && file.byteLength !== expected.byteLength) || (expected.sha256 !== null && file.sha256 !== expected.sha256)) throw officeAssetFailure()
      const entry={path:expected.path,byteLength:Number(file.byteLength),sha256:file.sha256}
      if (JSON.stringify(file)!==JSON.stringify(entry)) throw officeAssetFailure()
      files.set(entry.path,entry);previous=entry.path;total+=entry.byteLength
    }
    if(total>320*1024*1024)throw officeAssetFailure()
    return files
  } catch { throw officeAssetFailure() }
}

export async function loadPrivateOfficeBuffers(root: string, qualificationDigest: string): Promise<PrivateOfficeBuffers> {
  const buffers = new Map<string, OfficeBuffer>()
  try {
    if (!hex(qualificationDigest,64)) throw officeAssetFailure()
    const raw = await readHeld(root,'qualification.json',64*1024)
    const files = parsePrivateOfficeQualification(raw,qualificationDigest)
    const read = async (path:string) => {
      const entry=files.get(path)
      if(!entry)throw officeAssetFailure()
      const bytes=await readHeld(root,path,entry.byteLength)
      if(bytes.length!==entry.byteLength || hash(bytes)!==entry.sha256)throw officeAssetFailure()
      return bytes
    }
    const manifest=await read('manifest.json')
    if(!manifest.equals(Buffer.from(JSON.stringify(engineManifest,null,2)+'\n')) || hash(manifest)!==privateLayout.manifestSha256 || engineManifest.buildId!==privateLayout.buildId)throw officeAssetFailure()
    for(const [name,[path,mime]] of Object.entries(paths))buffers.set('/assets/'+name,{bytes:await read('assets/'+path),mime})
    for(const name of ['office-surface.html','office-surface.js','office-worker.js'])buffers.set('/'+name,{bytes:await read('surface/'+name),mime:name.endsWith('.html')?'text/html; charset=utf-8':'text/javascript; charset=utf-8'})
    // Electron requires a preload pathname. Verify its exact bytes as part of
    // the same witness before the caller rechecks Core and creates the view.
    await read('preload/office.cjs')
    return {buffers,preloadPath:join(root,'preload/office.cjs')}
  } catch { buffers.clear(); throw officeAssetFailure() }
}
