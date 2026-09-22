const { test } = require('node:test')
const assert = require('node:assert/strict')
const { mkdtempSync, mkdirSync, writeFileSync, symlinkSync, rmSync, readFileSync } = require('node:fs')
const { tmpdir } = require('node:os')
const { join, dirname } = require('node:path')
const profile = require('./core-package-profile.cjs')
const pack = require('./after-pack.cjs')._internals

test('core profile excludes professional resources while full retains them', () => {
  const config = () => ({ extraResources: [{ to: 'backend' }, { to: 'plugins/analytix-fund-analysis' }, { to: 'runtime' }, { to: 'LICENSE' }], artifactName: 'analytix-${version}.dmg' })
  assert.equal(profile.applyReleaseProfile(config(), 'full').extraResources.length, 4)
  const core = profile.applyReleaseProfile(config(), 'core')
  assert.deepEqual(core.extraResources.map(x => x.to), ['runtime', 'LICENSE'])
  assert.equal(core.extraMetadata.releaseProfile, 'core')
  assert.equal(core.artifactName, 'analytix-core-${version}.dmg')
  assert.throws(() => profile.applyReleaseProfile(config(), 'unknown'))
})

test('core closure rejects every excluded resource, including dangling symlinks', () => {
  const root = mkdtempSync(join(tmpdir(), 'analytix-core-closure-'))
  try {
    profile.assertCoreResourcesAbsent(root)
    for (const name of profile.EXCLUDED_RESOURCES) {
      const path = join(root, name)
      mkdirSync(dirname(path), { recursive: true })
      writeFileSync(path, 'unexpected')
      assert.throws(() => profile.assertCoreResourcesAbsent(root))
      rmSync(path)
      symlinkSync('missing', path)
      assert.throws(() => profile.assertCoreResourcesAbsent(root))
      rmSync(path)
    }
  } finally { rmSync(root, { recursive: true }) }
})

test('core authority binds absence and rejects full resources or target drift', () => {
  const original = JSON.parse(readFileSync(join(__dirname, '../packages/runtime-go/internal/domain/packagedbuildauthority/testdata/packaged-build-authority-v2-js-golden.json'), 'utf8'))
  const input = { ...original, nativeDisposition: { kind: profile.CORE_DISPOSITION, targetKey: 'darwin-arm64' }, artifacts: { ...original.artifacts, fundsPlugin: profile.ABSENT_FUNDS } }
  const core = pack.createPackagedBuildAuthorityV2(input)
  assert.equal(core.classification, 'development_clean_non_publishable')
  assert.equal(pack.isPackagedBuildAuthorityV2(core), true)
  const afterSign = require('./mac-notarize.cjs')._internals
  assert.equal(afterSign.assertNativeDispositionSigningBoundary(core), profile.CORE_DISPOSITION)
  assert.throws(() => afterSign.assertNativeDispositionSigningBoundary(core, { requireDeveloperID: true }), /cannot enter Developer ID or notarization/)
  assert.throws(() => pack.createPackagedBuildAuthorityV2({ ...input, artifacts: original.artifacts }))
  assert.throws(() => pack.createPackagedBuildAuthorityV2({ ...input, nativeDisposition: { ...input.nativeDisposition, targetKey: 'linux-x64' } }))
  assert.throws(() => pack.createPackagedBuildAuthorityV2({ ...original, artifacts: input.artifacts }))
})
