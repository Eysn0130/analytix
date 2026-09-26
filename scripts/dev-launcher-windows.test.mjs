import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import { mkdirSync, rmSync } from 'node:fs'
import { homedir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import { assertDevelopmentProfileReady, launchDevelopment, prepareDevelopmentProfile } from './dev-launcher.mjs'

test('Windows source profiles retain a shared DPAPI authority without Keychain interaction', { skip: process.platform !== 'win32' }, async t => {
  const root = join(homedir(), `analytix-windows-dev-${randomBytes(12).toString('hex')}`)
  const script = fileURLToPath(new URL('./windows-development-profile.ps1', import.meta.url))
  const created = spawnSync('powershell.exe', [
    '-NoProfile', '-NonInteractive', '-File', script, '-Directory', root, '-Create'
  ], { stdio: 'ignore', windowsHide: true, timeout: 10000 })
  assert.equal(created.status, 0, 'Windows owner-only fixture creation failed')
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const stateRoot = join(root, 'state')
  const first = prepareDevelopmentProfile({ root, stateRoot, env: { PATH: process.env.PATH } })
  assertDevelopmentProfileReady(first)
  const otherCheckout = join(root, 'other-checkout')
  mkdirSync(otherCheckout)
  const second = prepareDevelopmentProfile({ root: otherCheckout, stateRoot, env: { PATH: process.env.PATH } })
  assert.notEqual(first.taskRoot, second.taskRoot)
  assert.equal(first.env.ANALYTIX_DEVELOPMENT_PROVIDER_AUTHORITY_DIR, second.env.ANALYTIX_DEVELOPMENT_PROVIDER_AUTHORITY_DIR)
  assert.equal(first.env.USERPROFILE, first.homeRoot)
  assert.equal(first.env.APPDATA, join(first.homeRoot, 'AppData', 'Roaming'))
  assert.equal(prepareDevelopmentProfile({ root, stateRoot, env: { PATH: process.env.PATH } }).taskRoot, first.taskRoot)
  const events = []
  await launchDevelopment(second, { fast: true }, {
    runPrerequisite: script => events.push(script),
    promptPassword: () => { throw new Error('Windows requested a Keychain password') },
    prepareKeychain: () => { throw new Error('Windows requested a Keychain') },
    startApplication: () => { events.push('start'); return {} }
  })
  assert.deepEqual(events, ['doctor', 'start'])
})
