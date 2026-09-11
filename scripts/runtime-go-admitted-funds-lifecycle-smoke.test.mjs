import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

const source = readFileSync(new URL('./runtime-go-admitted-funds-lifecycle-smoke.mjs', import.meta.url), 'utf8')
const extract = (start, end) => {
  const begin = source.indexOf(start)
  const finish = source.indexOf(end, begin)
  assert.ok(begin >= 0 && finish > begin, `missing source boundary: ${start}`)
  return source.slice(begin, finish)
}
// The smoke entry launches native processes. Exercise its diagnostics with all I/O stubbed.
const helpers = extract('function exactKeys(', 'function isSHA256(') +
  extract('const publicTurnStatuses =', 'async function captureAccountFlowFailureStage(')
const api = vm.runInNewContext(helpers + '\n({classifyPublicDurableTurnStage, completeAccountFlowStage, isAccountFlowStage, unobservablePublicStage, publicSSEEvents})')
const turnID = 'synthetic-turn'
const sse = (kind, reason, id = turnID) => 'data: ' + JSON.stringify({kind, turnId: id, terminalReason: reason}) + '\n\n'
const thread = (status, reason, items = []) => ({turns: [{id: turnID, status, acceptedFinalView: {terminalReason: reason}, items}]})
const requests = [
  {valueSafe: true, authorizationConfigured: true},
  {valueSafe: true, authorizationConfigured: true, accountFlowSemantic: {synthetic: true}}
]

test('uses the public acceptedFinalView for failed and aborted turns', () => {
  for (const [status, reason] of [['failed', 'provider_failure'], ['aborted', 'cancel']]) {
    const result = api.classifyPublicDurableTurnStage(thread(status, reason), '', turnID)
    assert.equal(result.turnTerminalStatus, status)
    assert.equal(result.turnTerminalReason, reason)
  }
})

test('uses matching failed or aborted SSE when the durable view has no terminal reason', () => {
  for (const [status, reason] of [['failed', 'semantic_failure'], ['aborted', 'cancel']]) {
    const result = api.classifyPublicDurableTurnStage(thread(status),
      sse(`turn_${status}`, reason) + sse('turn_completed', 'success', 'unrelated-turn'), turnID)
    assert.equal(result.turnTerminalReason, reason)
  }
})

test('classifies public aborted items and source-unavailable host settlements', () => {
  const result = api.classifyPublicDurableTurnStage(thread('completed', 'source_unavailable', [
    {kind: 'tool_call', status: 'aborted'},
    {kind: 'tool_result', status: 'completed', output: {projectionKind: 'host_status', code: 'tool_source_unavailable'}}
  ]), '', turnID)
  assert.equal(result.toolCallStatuses.aborted, 1)
  assert.equal(result.toolResultSettlementReasons.tool_source_unavailable, 1)
})

test('preserves captured stage objects through the healthy-run error boundary', async () => {
  const captured = api.completeAccountFlowStage(api.classifyPublicDurableTurnStage(thread('failed', 'provider_failure'), '', turnID), requests)
  const runHealthy = vm.runInNewContext('(async () => {\n' +
    extract('    let healthy\n', '    const healthySourceProbe') + '\n})', {
    runFundsTurn: async () => { throw captured }, isAccountFlowStage: api.isAccountFlowStage,
    runtime: {stderr: []}, provider: {}, workspace: '', runtimeErrorCodes: () => []
  })
  await assert.rejects(runHealthy(), (error) => error === captured && api.isAccountFlowStage(error))
})

test('serializes only closed values and rejects extra fields in failure output', () => {
  const canary = 'SYNTHETIC-PRIVATE-VALUE-NOT-FOR-OUTPUT'
  const result = api.completeAccountFlowStage(api.classifyPublicDurableTurnStage(thread(canary, canary, [
    {kind: 'tool_call', status: canary, arguments: {content: canary}},
    {kind: 'tool_result', status: canary, output: {code: canary, content: canary}}
  ]), sse('turn_failed', canary), turnID), [{valueSafe: false, authorizationConfigured: true, accountFlowSemantic: {content: canary}}])
  assert.equal(api.isAccountFlowStage(result), true)
  assert.equal(JSON.stringify(result).includes(canary), false)
  assert.equal(result.providerValueSafe, false)
  assert.equal(api.isAccountFlowStage({...result, raw: canary}), false)
  assert.equal(api.isAccountFlowStage({...result, initialProviderRequestCount: 0.5}), false)
  assert.equal(api.isAccountFlowStage({...result, turnTerminalReason: canary}), false)
})

test('malformed SSE and an absent matching durable turn remain unobservable', () => {
  assert.equal(api.publicSSEEvents('data: not-json\ndata: []\ndata: null\n').length, 0)
  assert.equal(api.classifyPublicDurableTurnStage(thread('completed', 'success'), '', 'other-turn').label, 'public_stage_unobservable')
})

function stubRunTurn(detail, calls) {
  return vm.runInNewContext(extract('async function runTurn(', 'async function runOrdinaryTurn(') + '\nrunTurn', {
    providerID: 'synthetic-provider', providerModel: 'synthetic-model',
    requireCondition: (condition) => assert.ok(condition),
    httpJSON: async (url) => {
      calls.push('POST')
      return url.endsWith('/turns') ? {status: 202, body: {turnId: turnID}} : {status: 201, body: {id: 'synthetic-thread'}}
    },
    waitTurn: async () => { calls.push('completed-events'); return sse('turn_completed', 'success') },
    httpRequest: async (url) => {
      if (url.endsWith('/turns')) return {status: 202, body: JSON.stringify({turnId: turnID})}
      calls.push('diagnostic-detail-read')
      return detail()
    }
  })
}
const runtime = {ready: {url: 'https://synthetic.invalid'}}

test('ordinary turns retain their original success criteria without a diagnostic GET', async () => {
  const calls = []
  const result = await stubRunTurn(() => { throw new Error('detail unavailable') }, calls)(runtime, 'workspace', 'prompt')
  assert.equal(result.turnID, turnID)
  assert.deepEqual(calls, ['POST', 'POST', 'completed-events'])
})

test('optional account-flow detail failures do not replace successful turn execution', async () => {
  for (const detail of [
    () => { throw new Error('transport unavailable') },
    () => ({status: 200, body: 'malformed JSON'}),
    () => ({status: 503, body: 'unavailable'})
  ]) {
    const calls = []
    const result = await stubRunTurn(detail, calls)(runtime, 'workspace', 'prompt', {provider: {requests: []}, providerRequestStart: 0})
    assert.equal(result.turnID, turnID)
    assert.equal(result.publicThread, null)
    assert.equal(calls.filter((call) => call === 'diagnostic-detail-read').length, 1)
  }
})
