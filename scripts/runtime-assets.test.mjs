import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, symlinkSync, readFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { prepareAssets, verifyAssets } from './runtime-assets.mjs'

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'analytix-assets-test-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const source = join(root, 'source'), destination = join(root, 'destination')
  mkdirSync(source); mkdirSync(destination)
  writeFileSync(join(source, 'helper'), 'synthetic native fixture')
  const data = readFileSync(join(source, 'helper'))
  const manifest = { targets: { fixture: { files: [{ path: 'helper', bytes: data.length, sha256: createHash('sha256').update(data).digest('hex'), executable: true }] } } }
  return { source, destination, manifest }
}

test('explicit preparation is verified and idempotent; absent assets never pass', t => {
  const f = fixture(t)
  assert.equal(verifyAssets(f.destination, 'fixture', f.manifest).passed, false)
  assert.equal(prepareAssets(f.source, f.destination, 'fixture', f.manifest).copied, 1)
  assert.equal(prepareAssets(f.source, f.destination, 'fixture', f.manifest).copied, 0)
  assert.equal(verifyAssets(f.destination, 'unknown', f.manifest).configured, false)
})

test('tampered input and existing unknown destination are preserved, not overwritten', t => {
  const f = fixture(t)
  writeFileSync(join(f.destination, 'helper'), 'keep user data')
  assert.throws(() => prepareAssets(f.source, f.destination, 'fixture', f.manifest), /Existing asset differs/)
  assert.equal(readFileSync(join(f.destination, 'helper'), 'utf8'), 'keep user data')
  writeFileSync(join(f.source, 'helper'), 'tampered')
  assert.throws(() => prepareAssets(f.source, f.destination, 'fixture', f.manifest), /incomplete/)
})

test('path traversal and symlinks cannot redirect asset operations', t => {
  const f = fixture(t)
  f.manifest.targets.fixture.files[0].path = '../escape'
  assert.throws(() => verifyAssets(f.destination, 'fixture', f.manifest), /Invalid asset path/)
  f.manifest.targets.fixture.files[0].path = 'redirect/helper'
  symlinkSync(f.source, join(f.destination, 'redirect'), process.platform === 'win32' ? 'junction' : 'dir')
  assert.throws(() => verifyAssets(f.destination, 'fixture', f.manifest), /symlinks/)
})

test('dangling symlinks and unfinished preparations are not overwritten', t => {
  const f = fixture(t)
  const temporary = join(f.destination, `helper.prepare-${process.pid}`)
  writeFileSync(temporary, 'unfinished')
  assert.throws(() => prepareAssets(f.source, f.destination, 'fixture', f.manifest), /unfinished/)
  assert.equal(readFileSync(temporary, 'utf8'), 'unfinished')
  if (process.platform !== 'win32') {
    symlinkSync(join(f.destination, 'absent'), join(f.destination, 'helper'))
    assert.throws(() => verifyAssets(f.destination, 'fixture', f.manifest), /symlinks/)
  }
})
