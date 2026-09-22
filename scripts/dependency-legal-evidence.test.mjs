import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync, mkdtempSync, mkdirSync, symlinkSync, rmSync, cpSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { execFileSync } from 'node:child_process'
import { artifactLegalObligationsTestInternals as readers, inspectExactArtifactLegalInventory } from './artifact-legal-obligations-audit.mjs'

const { loadCatalog, materializeLegalEvidence, verifyEvidence, verifyPackage, verifyDistributionMaterials, sha256 } = createRequire(import.meta.url)('./lib/dependency-legal-evidence.cjs')
const root = resolve(fileURLToPath(new URL('..', import.meta.url)))
const loaded = loadCatalog(root)
const record = loaded.catalog.packages.find(record => record.name === 'exif-parser')

function fixture(name = 'exif-parser') {
  const record = loaded.catalog.packages.find(record => record.name === name)
  const entries = { 'dependency-legal/manifest.json': loaded.bytes }
  for (const lock of loaded.catalog.locks) entries[`dependency-legal/${lock.file}`] = readFileSync(join(root, lock.path))
  for (const material of record.materials) entries[`dependency-legal/${material.file}`] = readFileSync(join(loaded.root, material.file))
  const prefix = record.bindings.find(binding => binding.lock === 'package-lock.json').path
  entries[`${prefix}/package.json`] = readFileSync(join(root, prefix, 'package.json'))
  for (const pin of record.signingTransforms || []) {
    entries[`${prefix}/${pin.file}`] = readFileSync(join(root, prefix, pin.file))
  }
  // These are exact installed metadata and license bytes; no fake grants.
  assert.ok(record.packageJsonSha256.includes(sha256(entries[`${prefix}/package.json`])))
  return { entries, record, entry: `${prefix}/package.json` }
}

function verify(f) { return verifyEvidence(readers.createMemoryReader(f.entries), f.entry, f.record, loaded) }

test('missing upstream declaration is preserved while exact MIT evidence supplies the governing term', () => {
  const f = fixture()
  assert.equal(JSON.parse(f.entries[f.entry]).license, undefined)
  assert.equal(verify(f).selectedLicense, 'MIT')
  const dependency = inspectExactArtifactLegalInventory({artifact:{entries:f.entries}}).dependencyInstances.find(row => row.name === 'exif-parser')
  assert.equal(dependency.status, 'passed')
  assert.equal(dependency.declaredLicense, null)
  assert.equal(dependency.supplementalEvidence.evidenceId, record.id)
})

test('exact version, package metadata, content, instance path, lock and provenance changes fail closed', () => {
  for (const change of [
    f => { f.entries[f.entry] = JSON.stringify({...JSON.parse(f.entries[f.entry]),version:'0.1.13'}) },
    f => { f.entries[f.entry] = JSON.stringify({...JSON.parse(f.entries[f.entry]),name:'another-package'}) },
    f => { f.entries[f.entry] = JSON.stringify({...JSON.parse(f.entries[f.entry]),license:'MIT'}) },
    f => { f.entries['node_modules/exif-parser/index.js'] = 'tampered implementation' },
    f => { f.entry = 'node_modules/other/package.json'; f.entries[f.entry] = f.entries['node_modules/exif-parser/package.json'] },
    f => { f.entries['dependency-legal/locks/root.json'] = '{}' },
    f => { f.entries['dependency-legal/manifest.json'] = Buffer.from(loaded.bytes.toString().replace('979c143d', '00000000')) },
    f => { f.entries[`dependency-legal/${record.materials[0].file}`] = ' ' },
    f => { f.entries[`dependency-legal/${record.materials[0].file}`] = 'A different package MIT license' },
    f => { delete f.entries[`dependency-legal/${record.materials[0].file}`] },
    f => { f.entry = '../node_modules/exif-parser/package.json' }
  ]) {
    const f = fixture(); change(f); assert.throws(() => verify(f), /dependency_legal_/)
  }
})

test('OR selects the bound MIT branch and retains CC0; AND and WITH are not downgraded', () => {
  const f = fixture('type-fest')
  assert.equal(f.record.materials.length, 2)
  assert.equal(verify(f).selectedLicense, 'MIT')
  for (const license of ['MIT AND Apache-2.0', 'MIT WITH unknown-exception']) {
    const changed = fixture('type-fest')
    changed.entries[changed.entry] = JSON.stringify({...JSON.parse(changed.entries[changed.entry]),license})
    assert.throws(() => verify(changed), /metadata_changed/)
  }
})

test('fixed AWS and xml-naming sources supply only the exact reviewed license and instances', () => {
  for (const name of ['@aws-sdk/credential-provider-http', '@aws-sdk/credential-provider-login',
    '@aws-sdk/nested-clients', 'xml-naming', '@nodable/entities', 'html-parse-stringify']) {
    const f = fixture(name)
    const selected = name.startsWith('@aws-sdk/') ? 'Apache-2.0' : 'MIT'
    assert.equal(verify(f).selectedLicense, selected)
    const wrongOwner = {...f, entry: `packages/runtime/${f.entry}`}
    wrongOwner.entries[wrongOwner.entry] = f.entries[f.entry]
    assert.throws(() => verify(wrongOwner), /instance_not_bound/)
    for (const material of f.record.materials) {
      const changed = fixture(name)
      changed.entries[`dependency-legal/${material.file}`] = 'different source'
      assert.throws(() => verify(changed), /material_changed/)
    }
  }
})

test('root and runtime duplicate instances require independent exact owner bindings', () => {
  const f = fixture()
  const runtime = 'packages/runtime/node_modules/exif-parser/package.json'
  f.entries[runtime] = f.entries[f.entry]
  assert.equal(verify({...f,entry:runtime}).selectedLicense, 'MIT')
  const inventory = inspectExactArtifactLegalInventory({artifact:{entries:f.entries}})
  assert.equal(inventory.dependencyInstances.filter(row=>row.name==='exif-parser'&&row.status==='passed').length, 2)
  delete f.entries['dependency-legal/manifest.json']
  const missing = inspectExactArtifactLegalInventory({artifact:{entries:f.entries}})
  assert.equal(missing.dependencyInstances.filter(row=>row.name==='exif-parser'&&row.engineeringBlocking).length, 2)
})

test('reviewed computer-use wrappers bind all eight actual instances without admitting native dependencies', () => {
  for (const name of ['shared', 'provider-interfaces', 'libnut', 'default-clipboard-provider']) {
    const f = fixture(`@computer-use/${name}`)
    for (const binding of f.record.bindings) {
      const prefix = `${binding.lock === 'package-lock.json' ? '' : 'packages/runtime/'}${binding.path}`
      const entry = `${prefix}/package.json`
      f.entries[entry] = readFileSync(join(root, entry))
      for (const file of Object.keys(f.record.files)) f.entries[`${prefix}/${file}`] = readFileSync(join(root, prefix, file))
      assert.equal(verify({...f, entry}).selectedLicense, 'Apache-2.0')
    }
    const wrongVersion = {...f, entry: f.entry.replace(name, 'libnut-darwin')}
    f.entries[wrongVersion.entry] = f.entries[f.entry]
    assert.throws(() => verify(wrongVersion), /instance_not_bound/)
    delete f.entries['dependency-legal/materials/nut-tree-aaca4af3/CHANGES.txt']
    assert.throws(() => verify(f), /material_changed/)
  }
})

test('original-author buffer-equal supplement binds both installations without changing package metadata', () => {
  const f = fixture('buffer-equal')
  const runtime = `packages/runtime/${f.entry}`
  f.entries[runtime] = f.entries[f.entry]
  assert.equal(verify(f).selectedLicense, 'MIT')
  assert.equal(verify({...f, entry: runtime}).selectedLicense, 'MIT')
})

test('tr46 preserves separate Unicode data obligations for both exact installations', () => {
  const f = fixture('tr46')
  f.entries['node_modules/tr46/lib/mappingTable.json'] = readFileSync(join(root, 'node_modules/tr46/lib/mappingTable.json'))
  const runtime = `packages/runtime/${f.entry}`
  f.entries[runtime] = f.entries[f.entry]
  assert.equal(verify(f).governingExpression, 'MIT AND Unicode-3.0')
  assert.equal(verify({...f, entry: runtime}).governingExpression, 'MIT AND Unicode-3.0')
  delete f.entries['dependency-legal/materials/tr46-0.0.3/UNICODE-LICENSE.txt']
  assert.throws(() => verify(f), /material_changed/)
})

test('physical evidence belongs only to the actual ASAR component, never a suffix shadow', () => {
  const f = fixture()
  const physical = 'packaged-root/Contents/Resources/app.asar.unpacked/packages/runtime/node_modules/exif-parser/package.json'
  f.entries[physical] = f.entries[f.entry]
  const reader = {...readers.createMemoryReader(f.entries), components:[{artifactEntry:'Contents/Resources/app.asar'}]}
  assert.equal(verifyEvidence(reader, physical, record, loaded).selectedLicense, 'MIT')
  assert.throws(()=>verifyEvidence({...reader,components:[]},physical,record,loaded),/owner_invalid/)
  const shadow = physical.replace('Contents/Resources/app.asar', 'unrelated/app.asar')
  assert.throws(()=>verifyEvidence(reader,shadow,record,loaded),/owner_invalid/)
})

test('native root MIT text never closes unresolved embedded-component obligations', () => {
  const f = fixture('@napi-rs/canvas-darwin-arm64')
  assert.ok(f.record.materials.some(material => material.file.endsWith('/skia/icu/LICENSE')))
  assert.equal(f.record.componentSourceReview.status, 'source_materials_only_binary_build_provenance_unverified')
  assert.throws(()=>verify(f),/native_component_provenance_and_notices_unverified/)
})

test('exact native source survives real signing but code changes and re-signed changes are rejected',
  { skip: process.platform !== 'darwin' }, () => {
    const f = fixture('@napi-rs/canvas-darwin-arm64')
    const native = f.entry.replace('package.json', 'skia.darwin-arm64.node')
    const original = f.entries[native]
    assert.equal(sha256(original), f.record.files['skia.darwin-arm64.node'])
    const verifySource = (options = {}) => verifyPackage(readers.createMemoryReader(f.entries), f.entry, f.record, options)
    verifySource({ allowSigned: false })
    const tmp = mkdtempSync(join(tmpdir(), 'dependency-native-signing-'))
    const path = join(tmp, 'skia.darwin-arm64.node')
    const sign = bytes => {
      writeFileSync(path, bytes)
      execFileSync('/usr/bin/codesign', ['--force', '--sign', '-', '--options', 'runtime', '--timestamp=none', path], {stdio: 'pipe'})
      execFileSync('/usr/bin/codesign', ['--verify', '--strict', path], {stdio: 'pipe'})
      return readFileSync(path)
    }
    try {
      f.entries[native] = sign(original)
      assert.notEqual(sha256(f.entries[native]), sha256(original))
      verifySource()
      assert.throws(() => verifySource({ allowSigned: false }), /content_changed/)
      assert.throws(() => verify(f), /native_component_provenance_and_notices_unverified/)
      const changed = Buffer.from(original)
      changed[4096] ^= 1 // code, not a signature/load-command field
      f.entries[native] = changed
      assert.throws(() => verifySource(), /content_changed/)
      f.entries[native] = sign(changed) // valid signature is insufficient source evidence
      assert.throws(() => verifySource(), /content_changed/)
      const signed = sign(original)
      f.entries[native] = Buffer.from(signed)
      f.entries[native][4096] ^= 1
      assert.throws(() => verifySource(), /content_changed/)
      f.entries[native] = Buffer.from(signed)
      f.entries[native].writeUInt32LE(0x01000007, 4)
      assert.throws(() => verifySource(), /content_changed/)
      delete f.entries[native]
      assert.throws(() => verifySource(), /native_source_missing/)
      f.entries[native] = signed
      f.entries['dependency-legal/manifest.json'] = '{}'
      assert.throws(() => verify(f), /catalog_changed/)
    } finally { rmSync(tmp, { recursive: true, force: true }) }
  })

test('materialization copies canonical inputs before sealing and refuses replacement or symlink input', () => {
  const tmp = mkdtempSync(join(tmpdir(), 'dependency-legal-'))
  try {
    const resources = join(tmp,'resources');mkdirSync(resources)
    materializeLegalEvidence(root,resources)
    assert.deepEqual(readFileSync(join(resources,'dependency-legal/manifest.json')),loaded.bytes)
    assert.throws(()=>materializeLegalEvidence(root,resources),/EEXIST/)
    const source = join(tmp,'source');mkdirSync(join(source,'build'),{recursive:true})
    symlinkSync(loaded.root,join(source,'build/dependency-legal'))
    assert.throws(()=>loadCatalog(source),/symlink_rejected/)
    rmSync(join(source,'build/dependency-legal'))
    cpSync(loaded.root,join(source,'build/dependency-legal'),{recursive:true})
    writeFileSync(join(source,'build/dependency-legal',record.materials[0].file),'tampered')
    assert.throws(()=>loadCatalog(source),/material_changed/)
  } finally { rmSync(tmp,{recursive:true,force:true}) }
})

test('official Electron distribution notices are present and exact after materialization',
  { skip: process.platform !== 'darwin' || process.arch !== 'arm64' }, () => {
    const tmp = mkdtempSync(join(tmpdir(), 'electron-legal-'))
    try {
      materializeLegalEvidence(root, tmp)
      const entries = { 'dependency-legal/manifest.json': loaded.bytes }
      for (const material of loaded.catalog.distributionMaterials) {
        entries[`dependency-legal/${material.file}`] = readFileSync(join(tmp, 'dependency-legal', material.file))
      }
      const verify = () => verifyDistributionMaterials(readers.createMemoryReader(entries), loaded, { platform: 'darwin', arch: 'arm64' })
      verify()
      const path = `dependency-legal/${loaded.catalog.distributionMaterials[1].file}`
      const original = entries[path]
      entries[path] = Buffer.from('truncated or substituted notice')
      assert.throws(verify, /distribution_material_changed/)
      delete entries[path]
      assert.throws(verify, /distribution_material_changed/)
      entries[path] = original
      verify()
    } finally { rmSync(tmp, { recursive: true, force: true }) }
  })
