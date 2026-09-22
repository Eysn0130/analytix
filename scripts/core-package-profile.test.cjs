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

test('controlled Core candidate uses explicit signing qualification without professional receipts', () => {
  const original = JSON.parse(readFileSync(join(__dirname, '../packages/runtime-go/internal/domain/packagedbuildauthority/testdata/packaged-build-authority-v2-js-golden.json'), 'utf8'))
  const disposition = { kind: 'core_controlled_release', targetKey: 'darwin-arm64', signingPolicySha256: require('./macos-signing-policy.cjs').policyDigest, signingMode: 'developer-id', appleTeamIdentifier: 'TESTTEAM01' }
  const input = { ...original, nativeDisposition: disposition, artifacts: { ...original.artifacts, fundsPlugin: profile.ABSENT_FUNDS } }
  const authority = pack.createPackagedBuildAuthorityV2(input)
  assert.equal(authority.classification, 'controlled_release_clean_candidate_non_publishable')
  assert.equal(authority.publishable, false)
  assert.equal(require('./mac-notarize.cjs')._internals.assertNativeDispositionSigningBoundary(authority, { requireDeveloperID: true }), disposition.kind)
  for (const patch of [{ signingMode: 'ad-hoc' }, { appleTeamIdentifier: '' }, { targetKey: 'linux-x64' }, { unexpected: true }, { signingPolicySha256: '' }]) {
    assert.throws(() => pack.createPackagedBuildAuthorityV2({ ...input, nativeDisposition: { ...disposition, ...patch } }))
  }
  assert.throws(() => pack.createPackagedBuildAuthorityV2({ ...input, artifacts: original.artifacts }))
})

test('the existing signer accepts qualified Core only with its exact signing policy and team', () => {
  const vm = require('node:vm')
  const localRequire = require('node:module').createRequire(join(__dirname, 'mac-sign.cjs'))
  const realPolicy = localRequire('./macos-signing-policy.cjs')
  const moduleFixture = { exports: {} }
  vm.runInNewContext(readFileSync(join(__dirname, 'mac-sign.cjs'), 'utf8'), {
    module: moduleFixture, exports: moduleFixture.exports, __dirname,
    require: name => name === './macos-signing-policy.cjs'
      ? { ...realPolicy, requireOfficialTeamIdentifier: () => 'TESTTEAM01' }
      : localRequire(name)
  })
  const validate = moduleFixture.exports._internals.validateCoreSigningProfile
  const root = mkdtempSync(join(tmpdir(), 'core-signer-'))
  try {
    const app = join(root, 'analytix.app')
    mkdirSync(join(app, 'Contents', 'Resources'), {recursive:true})
    const original = JSON.parse(readFileSync(join(__dirname, '../packages/runtime-go/internal/domain/packagedbuildauthority/testdata/packaged-build-authority-v2-js-golden.json'), 'utf8'))
    const disposition = {kind:'core_controlled_release',targetKey:'darwin-arm64',signingPolicySha256:realPolicy.policyDigest,signingMode:'developer-id',appleTeamIdentifier:'TESTTEAM01'}
    const authority = pack.createPackagedBuildAuthorityV2({...original,nativeDisposition:disposition,artifacts:{...original.artifacts,fundsPlugin:profile.ABSENT_FUNDS}})
    const read = () => authority
    assert.equal(validate({app,identity:'Developer ID Application: Test'},'core',read),authority.authorityDigest)
    assert.throws(()=>validate({app,identity:'-'},'core',read))
    for (const patch of [{appleTeamIdentifier:'OTHERTEAM1'},{signingPolicySha256:'a'.repeat(64)},{signingMode:'ad-hoc'}]) {
      assert.throws(()=>validate({app,identity:'Developer ID Application: Test'},'core',()=>({...authority,nativeDisposition:{...disposition,...patch}})))
    }
    mkdirSync(join(app,'Contents','Resources','backend'))
    assert.throws(()=>validate({app,identity:'Developer ID Application: Test'},'core',read))
  } finally { rmSync(root,{recursive:true}) }
})


test('controlled Core packaging keeps its production route separate from private isolation', () => {
  const scripts = JSON.parse(readFileSync(join(__dirname,'../package.json'),'utf8')).scripts
  assert.match(scripts['dist:mac:arm64:core'], /isolated-local-v1/)
  const controlled = scripts['dist:mac:arm64:core:controlled']
  assert.match(controlled, /ANALYTIX_RELEASE_BUILD=1 MAC_SIGN=1/)
  assert.doesNotMatch(controlled, /isolated-local-v1|CSC_IDENTITY_AUTO_DISCOVERY=false/)
  assert.match(controlled, /--publish never --mac dmg zip --arm64 && node scripts\/core-update-metadata.cjs$/)
})
