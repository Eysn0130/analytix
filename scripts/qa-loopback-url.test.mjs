import assert from 'node:assert/strict'
import test from 'node:test'
import { qaLoopbackURL } from './lib/qa-loopback-url.mjs'

test('packaged QA retains explicit local API routes and query data', () => {
  assert.equal(qaLoopbackURL('http://127.0.0.1:45678', '/api/v1/tasks/synthetic?case_id=fixture').href,
    'http://127.0.0.1:45678/api/v1/tasks/synthetic?case_id=fixture')
})

test('packaged QA rejects external, credential-bearing, ambiguous and escaped URLs', () => {
  for (const base of ['https://example.invalid:443', 'http://localhost:45678', 'file:///tmp/fixture',
    'http://127.0.0.1', 'http://synthetic@127.0.0.1:45678', 'http://127.0.0.1:45678/base',
    'http://127.0.0.1:45678/?query=synthetic', 'http://127.0.0.1:45678/#fragment']) {
    assert.throws(() => qaLoopbackURL(base, '/api/v1/cases'))
  }
  for (const route of ['//example.invalid/api/v1/cases', '/api/v1/../../private', '/api/v1/\\escape',
    '/api/v1/%2e%2e/%2e%2e/private', 'https://example.invalid', '/api/v1/cases#fragment']) {
    assert.throws(() => qaLoopbackURL('http://127.0.0.1:45678', route))
  }
})
