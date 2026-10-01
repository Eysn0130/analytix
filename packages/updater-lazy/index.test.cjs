'use strict'

const assert = require('node:assert/strict')
const { createRequire } = require('node:module')
const test = require('node:test')
const { Lazy } = require('./index.cjs')

test('electron-updater resolves the Analytix implementation', () => {
  const fromUpdater = createRequire(require.resolve('electron-updater'))
  assert.equal(fromUpdater('lazy-val').Lazy, Lazy)
  assert.equal(fromUpdater('lazy-val/package.json').name, '@analytix/updater-lazy')
})

test('creates a promise only on first read and reuses it', async () => {
  let calls = 0
  const lazy = new Lazy(() => {
    calls++
    return Promise.resolve('ready')
  })
  assert.equal(lazy.hasValue, false)
  assert.equal(calls, 0)
  const first = lazy.value
  assert.equal(lazy.value, first)
  assert.equal(lazy.hasValue, true)
  assert.equal(calls, 1)
  assert.equal(await first, 'ready')
})

test('explicit replacement wins before and after a read', async () => {
  const lazy = new Lazy(() => Promise.resolve('old'))
  const replacement = Promise.resolve('new')
  lazy.value = replacement
  assert.equal(lazy.hasValue, true)
  assert.equal(lazy.value, replacement)
  assert.equal(await lazy.value, 'new')
})

test('retains a rejected promise and retries a synchronous factory throw', async () => {
  const failure = Promise.reject(new Error('unavailable'))
  let calls = 0
  const rejected = new Lazy(() => {
    calls++
    return failure
  })
  assert.equal(rejected.value, failure)
  await assert.rejects(rejected.value, /unavailable/)
  assert.equal(calls, 1)

  const thrown = new Lazy(() => {
    calls++
    if (calls === 2) throw new Error('retry')
    return Promise.resolve('recovered')
  })
  assert.throws(() => thrown.value, /retry/)
  assert.equal(thrown.hasValue, false)
  assert.equal(await thrown.value, 'recovered')
})
