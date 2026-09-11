import assert from 'node:assert/strict'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { requiredBuildOutputs, verifySourceBuild } from './source-build-smoke.mjs'

test('source smoke requires every output and a resolvable renderer entry', t => {
  const root = mkdtempSync(join(tmpdir(), 'analytix-source-smoke-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  assert.equal(verifySourceBuild(root).passed, false)
  for (const name of [...requiredBuildOutputs, 'out/renderer/assets/index.js']) {
    mkdirSync(dirname(join(root, name)), { recursive: true })
    writeFileSync(join(root, name), name.endsWith('.html') ? '<script type="module" src="./assets/index.js"></script>' : '// synthetic build fixture')
  }
  assert.equal(verifySourceBuild(root).passed, true)
  rmSync(join(root, 'out/renderer/assets/index.js'))
  assert.equal(verifySourceBuild(root).passed, false)
})
