'use strict'

const assert = require('node:assert/strict')
const { createHash } = require('node:crypto')
const fs = require('node:fs')
const { tmpdir } = require('node:os')
const { dirname, join, resolve } = require('node:path')
const test = require('node:test')
const { validateQualification, verifyPrivateLocal, stagePrivateLocal } = require('./office-private-local-contract.cjs')
const layout = require('../packages/runtime-go/internal/adapters/outbound/officeengineassets/private-local-layout.json')

const REPO = resolve(__dirname, '..')
const MANIFEST_PATH = join(REPO, 'packages/runtime-go/internal/adapters/outbound/officeengineassets/manifest.json')
const SHA = value => createHash('sha256').update(value).digest('hex')
const EXPECTED = { sourceCommit: 'a'.repeat(40), worktreeSnapshotDigest: 'b'.repeat(64), targetKey: 'darwin-arm64' }
const PLUGINS = ['documents', 'spreadsheets', 'presentations']
const pluginFiles = kind => ['.analytix-plugin/package.json', '.codex-plugin/plugin.json',
  'assets/adapter.json', 'ui/editor.json', `skills/${kind}/SKILL.md`]

// Metadata fixture only. Production fixed hashes are never overridden, and these
// synthetic bytes cannot pass verifyPrivateLocal or be staged as an engine.
function fixture() {
  const manifestBytes = fs.readFileSync(MANIFEST_PATH), manifest = JSON.parse(manifestBytes)
  const files = [{ path: 'manifest.json', byteLength: manifestBytes.length, sha256: SHA(manifestBytes) }]
  for (const asset of manifest.assets) files.push({ path: `assets/${asset.path}`, byteLength: asset.bytes, sha256: asset.sha256 })
  for (const [path, byteLength, sha256] of [
    ['COPYING', 35147, '8ceb4b9ee5adedde47b31e975c1d90c73ad27b6b165a1dcd80c7c545eb65b903'],
    ['COPYING.LGPL', 7639, 'a853c2ffec17057872340eee242ae4d96cbf2b520ae27d903e1b2fef1a5f9d1c'],
    ['COPYING.MPL', 16725, '1f256ecad192880510e84ad60474eab7589218784b9a50bc7ceee34c2b91f1d5'],
    ['zetajs-LICENSE', 1098, 'a8c9b7d037ed112c3b9f85d5a1122aedd218947b21cf3f2c4200d13d7fe1a7d9'],
    ['Noto-OFL.txt', 4301, '6a73f9541c2de74158c0e7cf6b0a58ef774f5a780bf191f2d7ec9cc53efe2bf2'],
    ['FONT-NOTICE.txt', 348, 'fd50fa43c3afea9046773ea987118c56ff100ea64163e9d4918e8f0b07b04669'],
    ['LO-LICENSE.xml', 588864, '0fc511d0cdf0b1098b2000f186840629f4a4740a0efe9a8cf60753d7de0f6af8']
  ]) files.push({ path: `notices/${path}`, byteLength, sha256 })
  const dynamic = ['surface/office-surface.html', 'surface/office-surface.js', 'surface/office-worker.js',
    'preload/office.cjs', 'notices/zetajs-adapter-NOTICE.txt', 'notices/office-local-install-admission.md']
  for (const kind of PLUGINS) dynamic.push(...pluginFiles(kind).map(path => `plugins/analytix-${kind}/${path}`))
  for (const path of dynamic) files.push({ path, byteLength: 9, sha256: SHA('synthetic') })
  files.sort((a, b) => a.path < b.path ? -1 : a.path > b.path ? 1 : 0)
  return { schemaVersion: 1, contract: 'analytix.office-private-local/v1', usage: 'private-local',
    publishable: false, releaseEligible: false, targetKey: EXPECTED.targetKey,
    sourceCommit: EXPECTED.sourceCommit, worktreeSnapshotDigest: EXPECTED.worktreeSnapshotDigest,
    engineBuildId: manifest.buildId, engineManifestSha256: SHA(manifestBytes), files }
}
function serialize(body) {
  const qualificationDigest = createHash('sha256').update('AnalytixOfficePrivateLocalV1\0').update(JSON.stringify(body)).digest('hex')
  return Buffer.from(JSON.stringify({ ...body, qualificationDigest }) + '\n')
}
function temporary(t) {
  const root = fs.realpathSync(fs.mkdtempSync(join(tmpdir(), 'analytix-office-contract-')))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  return root
}
function tinyTree(t) {
  const root = temporary(t), body = fixture()
  for (const file of body.files) {
    const path = join(root, file.path)
    fs.mkdirSync(dirname(path), { recursive: true })
    fs.writeFileSync(path, 'synthetic', { mode: 0o644 })
  }
  fs.writeFileSync(join(root, 'qualification.json'), serialize(body), { mode: 0o644 })
  return root
}

test('canonical metadata binds the actual fixed manifest, all 35 paths and the cross-language digest', () => {
  const body = fixture(), bytes = serialize(body), result = validateQualification(bytes)
  assert.equal(result.files.length, 35)
  assert.equal(result.engineManifestSha256, '87f31a749b2cd71f60da5f96c8e2a6323bcb6a53d9fa8339bca383e66bcdf2c5')
  assert.equal(result.engineBuildId, 'efaf0670b4d055f838a2849becb10f08aa06a257')
  assert.equal(result.publishable, false)
  assert.ok(Object.isFrozen(result.files[0]))
  assert.deepEqual(Buffer.from(JSON.stringify(result) + '\n'), bytes)
})

test('shared source layout binds the manifest and exactly the necessary 35-file closure', () => {
  const body = fixture()
  assert.equal(layout.schemaVersion, 1)
  assert.equal(layout.contract, 'analytix.office-private-local-layout/v1')
  assert.equal(layout.buildId, body.engineBuildId)
  assert.equal(layout.manifestSha256, body.engineManifestSha256)
  assert.equal(layout.files.length, 35)
  assert.deepEqual(layout.files.map(file => file.path), body.files.map(file => file.path))
  for (const file of layout.files) {
    assert.deepEqual(Object.keys(file), ['path', 'source', 'owner', 'maximum', 'byteLength', 'sha256'])
    const expected = body.files.find(entry => entry.path === file.path)
    if (file.byteLength !== null) {
      assert.equal(file.byteLength, expected.byteLength)
      assert.equal(file.sha256, expected.sha256)
    } else {
      assert.equal(file.owner, 'repo')
      assert.equal(file.sha256, null)
    }
  }
  assert.equal(layout.files.find(file => file.path === 'manifest.json').source,
    'packages/runtime-go/internal/adapters/outbound/officeengineassets/manifest.json')
})

test('rejects false authority, wrong identity types, unknown keys and noncanonical encodings', () => {
  for (const change of [
    x => { x.publishable = true }, x => { x.releaseEligible = true }, x => { x.usage = 'release' },
    x => { x.targetKey = 'darwin-amd64' }, x => { x.sourceCommit = [EXPECTED.sourceCommit] },
    x => { x.worktreeSnapshotDigest = [EXPECTED.worktreeSnapshotDigest] }, x => { x.schemaVersion = 2 },
    x => { x.engineBuildId = 'c'.repeat(40) }, x => { x.engineManifestSha256 = 'c'.repeat(64) },
    x => { x.extra = true }, x => { delete x.publishable }
  ]) {
    const body = fixture(); change(body)
    assert.throws(() => validateQualification(serialize(body)), /office-private-local-/)
  }
  const bytes = serialize(fixture()), json = bytes.toString()
  for (const text of [json.trim(), '\ufeff' + json, ' ' + json, json + '\n',
    json.replace('"schemaVersion":1', '"schemaVersion":1,"schemaVersion":1'),
    json.replace('"schemaVersion":1', '"schemaVersion":1.0'),
    JSON.stringify(JSON.parse(json), null, 2) + '\n']) {
    assert.throws(() => validateQualification(Buffer.from(text)), /office-private-local-/)
  }
  assert.throws(() => validateQualification(Buffer.from([0xff])), /office-private-local-/)
  const changed = JSON.parse(json); changed.qualificationDigest = '0'.repeat(64)
  assert.throws(() => validateQualification(Buffer.from(JSON.stringify(changed) + '\n')), /invalid-digest/)
})

test('rehashing cannot admit replacement fixed assets or omit notices or add arbitrary payload', () => {
  for (const change of [
    x => { x.files.find(f => f.path === 'assets/soffice.wasm').sha256 = '0'.repeat(64) },
    x => { x.files.find(f => f.path === 'manifest.json').byteLength-- },
    x => { x.files.find(f => f.path === 'notices/LO-LICENSE.xml').sha256 = '0'.repeat(64) },
    x => { x.files = x.files.filter(f => f.path !== 'notices/Noto-OFL.txt') },
    x => { x.files.push({ path: 'qualification.json', byteLength: 1, sha256: '0'.repeat(64) }) },
    x => { x.files[0].path = '../outside' }, x => { x.files[0].path = '/absolute' },
    x => { x.files[0].path = 'assets\\soffice.data' }, x => { x.files.reverse() },
    x => { x.files[0].extra = true }, x => { x.files[0].byteLength = -1 },
    x => { x.files.find(f => f.path === 'preload/office.cjs').sha256 = ['0'.repeat(64)] },
    x => { x.files.find(f => f.path === 'preload/office.cjs').byteLength = 1024 * 1024 + 1 }
  ]) {
    const body = fixture(); change(body)
    assert.throws(() => validateQualification(serialize(body)), /office-private-local-/)
  }
})

test('a tiny complete-looking tree cannot impersonate the real engine', t => {
  const root = tinyTree(t)
  assert.throws(() => verifyPrivateLocal(root, EXPECTED), /content-mismatch/)
  assert.throws(() => verifyPrivateLocal(root, { ...EXPECTED, sourceCommit: 'c'.repeat(40) }), /identity-mismatch/)
})

test('rejects unexpected files, empty directories and hardlink aliases before reading engine bytes', t => {
  const root = tinyTree(t), extra = join(root, 'extra.js')
  fs.writeFileSync(extra, 'not admitted')
  assert.throws(() => verifyPrivateLocal(root, EXPECTED), /unexpected-payload/)
  fs.unlinkSync(extra)
  fs.mkdirSync(join(root, 'extra'))
  assert.throws(() => verifyPrivateLocal(root, EXPECTED), /unexpected-payload/)
  fs.rmdirSync(join(root, 'extra'))
  const outside = temporary(t), alias = join(outside, 'alias')
  fs.linkSync(join(root, 'manifest.json'), alias)
  assert.throws(() => verifyPrivateLocal(root, EXPECTED), /unexpected-payload/)
})

test('rejects symlinked root, qualification and intermediate directory', t => {
  const root = tinyTree(t), outside = temporary(t), alias = join(outside, 'root')
  fs.symlinkSync(root, alias)
  assert.throws(() => verifyPrivateLocal(alias, EXPECTED), /unsafe-directory/)
  fs.renameSync(join(root, 'qualification.json'), join(outside, 'qualification.json'))
  fs.symlinkSync(join(outside, 'qualification.json'), join(root, 'qualification.json'))
  assert.throws(() => verifyPrivateLocal(root, EXPECTED), /unsafe-file/)
  fs.unlinkSync(join(root, 'qualification.json'))
  fs.renameSync(join(outside, 'qualification.json'), join(root, 'qualification.json'))
  fs.renameSync(join(root, 'surface'), join(outside, 'surface'))
  fs.symlinkSync(join(outside, 'surface'), join(root, 'surface'))
  assert.throws(() => verifyPrivateLocal(root, EXPECTED), /unsafe-tree/)
})

test('rejects initially writable payload root and nested directories, permitting writable host ancestors', t => {
  for (const relative of ['.', 'assets/fonts', 'plugins/analytix-documents/skills/documents']) {
    for (const mode of [0o775, 0o777]) {
      const root = tinyTree(t)
      fs.chmodSync(join(root, relative), mode)
      assert.throws(() => verifyPrivateLocal(root, EXPECTED), /unsafe-directory/)
    }
  }
  const parent = temporary(t), previous = tinyTree(t), root = join(parent, 'office-private')
  fs.renameSync(previous, root)
  fs.chmodSync(parent, 0o777)
  // Permission validation succeeds; synthetic bytes still fail the fixed hash.
  assert.throws(() => verifyPrivateLocal(root, EXPECTED), /content-mismatch/)
})

for (const change of ['root-chmod', 'subdir-chmod', 'subdir-rename']) {
  test(`rejects ${change} after directory observation during verification`, t => {
    const root = tinyTree(t), original = fs.readdirSync
    const target = change === 'root-chmod' ? root : join(root, 'assets/fonts')
    let changed = false
    t.mock.method(fs, 'readdirSync', (path, ...args) => {
      const entries = original(path, ...args)
      if (!changed && path === target) {
        changed = true
        if (change === 'subdir-rename') {
          const outside = temporary(t)
          fs.renameSync(target, join(outside, 'old-fonts'))
          fs.mkdirSync(target)
          fs.writeFileSync(join(target, 'NotoSansCJKsc-Regular.otf'), 'synthetic')
        } else fs.chmodSync(target, 0o777)
      }
      return entries
    })
    assert.throws(() => verifyPrivateLocal(root, EXPECTED), /directory-changed/)
    assert.equal(changed, true)
  })
}

test('held-descriptor checks reject a link introduced during the qualification read', t => {
  const root = tinyTree(t), outside = temporary(t), original = fs.readSync
  let injected = false
  t.mock.method(fs, 'readSync', (...args) => {
    const count = original(...args)
    if (!injected && count > 0) {
      injected = true
      fs.linkSync(join(root, 'qualification.json'), join(outside, 'alias'))
    }
    return count
  })
  assert.throws(() => verifyPrivateLocal(root, EXPECTED), /source-changed/)
  assert.equal(injected, true)
})

test('stager preserves an existing destination and refuses overlapping or symlink roots', t => {
  const root = temporary(t), repoRoot = join(root, 'repo'), assetRoot = join(root, 'assets'), destination = join(root, 'office-private')
  for (const dir of [repoRoot, assetRoot, destination]) fs.mkdirSync(dir)
  fs.writeFileSync(join(destination, 'sentinel'), 'preserve')
  const worktreeSnapshot = { sourceCommit: EXPECTED.sourceCommit, snapshotDigest: EXPECTED.worktreeSnapshotDigest }
  assert.throws(() => stagePrivateLocal({ repoRoot, assetRoot, destination, worktreeSnapshot }), /destination-exists/)
  assert.equal(fs.readFileSync(join(destination, 'sentinel'), 'utf8'), 'preserve')
  assert.throws(() => stagePrivateLocal({ repoRoot, assetRoot, destination: join(repoRoot, 'office-private'), worktreeSnapshot }), /overlapping-roots/)
  fs.symlinkSync(repoRoot, join(root, 'repo-alias'))
  assert.throws(() => stagePrivateLocal({ repoRoot: join(root, 'repo-alias'), assetRoot, destination, worktreeSnapshot }), /unsafe-directory/)
})

test('production stager fails on a small fake asset before creating a candidate', t => {
  const root = temporary(t), repoRoot = join(root, 'repo'), assetRoot = join(root, 'assets'), destination = join(root, 'office-private')
  fs.mkdirSync(repoRoot); fs.mkdirSync(assetRoot)
  for (const kind of PLUGINS) for (const file of pluginFiles(kind)) {
    const path = `plugins/analytix-${kind}/${file}`
    fs.mkdirSync(dirname(join(repoRoot, path)), { recursive: true })
    fs.copyFileSync(join(REPO, path), join(repoRoot, path))
  }
  fs.mkdirSync(join(assetRoot, 'fonts'))
  fs.writeFileSync(join(assetRoot, 'fonts/NotoSansCJKsc-Regular.otf'), 'synthetic')
  assert.throws(() => stagePrivateLocal({ repoRoot, assetRoot, destination,
    worktreeSnapshot: { sourceCommit: EXPECTED.sourceCommit, snapshotDigest: EXPECTED.worktreeSnapshotDigest } }), /fixed-source-mismatch/)
  assert.equal(fs.existsSync(destination), false)
})
