import assert from 'node:assert/strict'
import test from 'node:test'
import { assertDevelopmentCISuccess, requiredDevelopmentJobs } from './development-ci-gate.mjs'

const success = () => Object.fromEntries(requiredDevelopmentJobs.map(name => [name, { result: 'success' }]))

test('all complete CI job families are required for the merge gate', () => {
  assert.equal(assertDevelopmentCISuccess(success()), true)
  for (const name of requiredDevelopmentJobs) {
    for (const result of ['failure', 'cancelled', 'skipped', 'pending', undefined]) {
      assert.throws(() => assertDevelopmentCISuccess({ ...success(), [name]: { result } }), /did not succeed/)
    }
  }
})

test('missing, malformed or unaccounted CI results cannot produce a green gate', () => {
  for (const input of [null, [], {}, 'success', { ...success(), unexpected: { result: 'success' } }]) {
    assert.throws(() => assertDevelopmentCISuccess(input), /job results/)
  }
  for (const name of requiredDevelopmentJobs) {
    const input = success()
    delete input[name]
    assert.throws(() => assertDevelopmentCISuccess(input), /job results/)
    assert.throws(() => assertDevelopmentCISuccess({ ...success(), [name]: null }), /did not succeed/)
  }
})
