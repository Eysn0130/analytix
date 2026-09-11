import assert from 'node:assert/strict'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { candidateImage, checkCandidateLayout } from './package-candidate-smoke.mjs'

test('candidate selection rejects absent, empty or ambiguous installers', t => {
  const root = mkdtempSync(join(tmpdir(), 'analytix-package-fixture-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  assert.throws(() => candidateImage(root), /exactly one/)
  const file = join(root, 'analytix-1.0.6-mac-arm64.dmg')
  writeFileSync(file, '')
  assert.throws(() => candidateImage(root), /nonempty/)
  writeFileSync(file, 'synthetic image selection fixture, not an installer')
  assert.equal(candidateImage(root), file)
  writeFileSync(join(root, 'analytix-1.0.7-mac-arm64.dmg'), 'synthetic')
  assert.throws(() => candidateImage(root), /exactly one/)
})

test('layout validation cannot pass without the production Go runtime', t => {
  const root = mkdtempSync(join(tmpdir(), 'analytix-package-layout-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const files = ['Contents/Info.plist', 'Contents/MacOS/analytix', 'Contents/Resources/app.asar', 'Contents/Resources/runtime-go/bin/runtime-server']
  assert.throws(() => checkCandidateLayout(root))
  for (const file of files) {
    const target = join(root, 'Analytix.app', file)
    mkdirSync(dirname(target), { recursive: true })
    writeFileSync(target, 'synthetic layout fixture')
  }
  assert.equal(checkCandidateLayout(root), join(root, 'Analytix.app'))
  rmSync(join(root, 'Analytix.app', files[3]))
  assert.throws(() => checkCandidateLayout(root))
})
