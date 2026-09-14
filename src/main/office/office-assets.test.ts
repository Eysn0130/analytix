import { afterEach, expect, test } from 'vitest'
import { writeFile, unlink, symlink } from 'node:fs/promises'
import { join } from 'node:path'
import { loadOfficeBuffers, startOfficeAssetServer } from './office-assets'
import { fixture } from './test-fixture'
const cleanups: (() => Promise<void>)[] = []
afterEach(async () => { for (const c of cleanups.splice(0)) await c() })
async function setup() { const f = await fixture(); cleanups.push(f.cleanup); return f }
test('fixed routes serve verified held buffers after backing file changes', async () => {
  const f = await setup(), buffers = await loadOfficeBuffers(f.sourceRoot, f.assetRoot)
  expect(buffers.size).toBe(9)
  await writeFile(join(f.assetRoot, 'soffice.js'), 'mutated after verification')
  const server = await startOfficeAssetServer(buffers, '11111111-1111-1111-1111-111111111111')
  try {
    const response = await fetch(server.entryURL.replace('office-surface.html', 'assets/soffice.js'))
    expect(await response.text()).toBe('synthetic-asset-soffice.js')
    for (const [name, value] of [['cross-origin-opener-policy','same-origin'], ['cross-origin-embedder-policy','require-corp'], ['cross-origin-resource-policy','same-origin']]) expect(response.headers.get(name)).toBe(value)
    expect(response.headers.get('content-security-policy')).toContain("frame-ancestors 'none'")
    for (const url of [server.entryURL + '?x=1', server.origin + '/etc/passwd', server.entryURL.replace('office-surface.html','assets/not-allowed.js')]) expect((await fetch(url)).status).toBe(404)
    expect((await fetch(server.entryURL, { method: 'POST', body: 'no writes' })).status).toBe(404)
    expect((await fetch(server.entryURL, { headers: { Origin: 'https://outside.invalid' } })).status).toBe(404)
    expect(server.permits('https://outside.invalid', 'GET')).toBe(false)
    expect(server.permits(server.entryURL + '#x', 'GET')).toBe(false)
  } finally { server.destroy() }
  expect(buffers.size).toBe(0)
})
test.each(['digest', 'size', 'path', 'extra', 'duplicate', 'mode', 'symlink'])('rejects %s before admitting resources', async mode => {
  const f = await setup()
  if (mode === 'digest') f.manifest.assets[0].sha256 = '0'.repeat(64)
  if (mode === 'size') f.manifest.assets[0].bytes--
  if (mode === 'path') f.manifest.assets[0].path = '../outside'
  if (mode === 'extra') f.manifest.assets.push(f.manifest.assets[0])
  if (mode === 'mode') f.manifest.publishable = true
  if (mode === 'symlink') { const path = join(f.assetRoot, 'soffice.js'); await unlink(path); await symlink(f.preloadPath, path) }
  const raw = JSON.stringify(f.manifest, null, 2) + '\n'
  await writeFile(f.manifestPath, mode === 'duplicate' ? raw.replace('"schemaVersion": 1,', '"schemaVersion": 1,\n  "schemaVersion": 1,') : raw)
  await expect(loadOfficeBuffers(f.sourceRoot, f.assetRoot)).rejects.toThrow('office-assets-unavailable')
})
