import assert from 'node:assert/strict'
import { mkdirSync, mkdtempSync, realpathSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { assertDevelopmentProfileReady, launchDevelopment, prepareDevelopmentProfile } from './dev-launcher.mjs'

test('Windows source profiles retain a shared DPAPI authority without Keychain interaction', { skip: process.platform !== 'win32' }, async t => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'analytix-windows-dev-')))
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
