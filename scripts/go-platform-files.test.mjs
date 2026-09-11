import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const runtime = resolve(dirname(fileURLToPath(import.meta.url)), '../packages/runtime-go')
function selected(goos, pkg) {
  return JSON.parse(execFileSync('go', ['list', '-e', '-json', pkg], {
    cwd: runtime, encoding: 'utf8', timeout: 120000,
    env: { ...process.env, GOOS: goos, GOARCH: 'amd64', GOTOOLCHAIN: 'local', CGO_ENABLED: '0' },
    stdio: ['ignore', 'pipe', 'pipe']
  })).GoFiles
}

test('Linux selects the intended persistence and private-exact fallback implementations', () => {
  const files = selected('linux', './internal/adapters/outbound/persistencefs')
  assert.ok(files.includes('path_case_other_unix.go'))
  assert.ok(files.includes('secure_absolute_open_linux.go'))
  assert.ok(selected('linux', './internal/adapters/outbound/finalauthority').includes('private_exact_open_other.go'))
})

test('Darwin keeps its hardened implementation and never selects Linux fallbacks', () => {
  const files = selected('darwin', './internal/adapters/outbound/persistencefs')
  assert.ok(files.includes('path_case_darwin.go'))
  assert.ok(files.includes('secure_absolute_open_darwin.go'))
  assert.equal(files.includes('path_case_other_unix.go'), false)
  assert.equal(files.includes('secure_absolute_open_linux.go'), false)
  assert.equal(selected('darwin', './internal/adapters/outbound/finalauthority').includes('private_exact_open_other.go'), false)
})
