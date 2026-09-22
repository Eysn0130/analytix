import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync, mkdtempSync, mkdirSync, symlinkSync, rmSync, cpSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { artifactLegalObligationsTestInternals as readers, inspectExactArtifactLegalInventory } from './artifact-legal-obligations-audit.mjs'

const { loadCatalog, materializeLegalEvidence, verifyEvidence, sha256 } = createRequire(import.meta.url)('./lib/dependency-legal-evidence.cjs')
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
  assert.equal(f.record.materials.length, 1)
  assert.throws(()=>verify(f),/native_component_provenance_and_notices_unverified/)
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
