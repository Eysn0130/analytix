import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises'
import { join, dirname } from 'node:path'
import { createHash } from 'node:crypto'
import { tmpdir } from 'node:os'
const paths = [
  ['soffice.js', 'soffice.js', 'text/javascript'],
  ['soffice.wasm', 'soffice.wasm', 'application/wasm'],
  ['soffice.data', 'soffice.data', 'application/octet-stream'],
  ['soffice.data.js.metadata', 'soffice.data.js.metadata', 'application/json'],
  ['NotoSansCJKsc-Regular.otf', 'fonts/NotoSansCJKsc-Regular.otf', 'font/otf'],
  ['zeta.js', 'zetajs-57360bcb0e7726ffa0e66567c8041261b959f8dd/source/zeta.js', 'text/javascript']
]
export async function fixture() {
  const root = await mkdtemp(join(tmpdir(), 'analytix-office-fixture-'))
  const sourceRoot = join(root, 'source'), assetRoot = join(root, 'assets'), preloadPath = join(root, 'office-preload.js')
  const manifestPath = join(sourceRoot, 'packages/runtime-go/internal/adapters/outbound/officeengineassets/manifest.json')
  const manifest = { schemaVersion: 1, executionMode: 'source-experiment', publishable: false, buildId: 'efaf0670b4d055f838a2849becb10f08aa06a257', assets: [] as any[] }
  for (const [name, path, mime] of paths) {
    const bytes = Buffer.from('synthetic-asset-' + name)
    await mkdir(dirname(join(assetRoot, path)), { recursive: true }); await writeFile(join(assetRoot, path), bytes)
    manifest.assets.push({ name, path, bytes: bytes.length, sha256: createHash('sha256').update(bytes).digest('hex'), mime })
  }
  await mkdir(dirname(manifestPath), { recursive: true }); await writeFile(manifestPath, JSON.stringify(manifest, null, 2) + '\n')
  const surfaceRoot = join(sourceRoot, 'src/main/office/surface'); await mkdir(surfaceRoot, { recursive: true })
  for (const name of ['office-surface.html', 'office-surface.js', 'office-worker.js']) await writeFile(join(surfaceRoot, name), 'synthetic-surface-' + name)
  await writeFile(preloadPath, '// synthetic preload fixture')
  return { sourceRoot, assetRoot, preloadPath, manifestPath, manifest, root, cleanup: () => rm(root, { recursive: true, force: true }) }
}
