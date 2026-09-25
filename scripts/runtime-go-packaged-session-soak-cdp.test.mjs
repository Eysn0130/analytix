import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'

// Exercise the existing owner's actual functions without importing the CLI
// entry point (which would run Go/native/Mac acceptance as an import effect).
const source = readFileSync(new URL('./runtime-go-packaged-session-soak.mjs', import.meta.url), 'utf8')
const start = source.indexOf('async function evaluateCdp(')
const end = source.indexOf('\nfunction startContractProvider()', start)
assert.ok(start >= 0 && source.includes('async function evaluateRendererWithRetries('))
const functions = source.slice(start, end < 0 ? source.length : end)
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

function fixture(t, plans, options = {}) {
  const sockets = []
  const timers = new Set()
  const callbackErrors = []
  const messages = []
  let discoveries = 0
  class Socket {
    constructor() {
      this.listeners = new Map()
      this.closed = 0
      this.plan = plans[Math.min(sockets.length, plans.length - 1)]
      sockets.push(this)
      queueMicrotask(() => this.plan.connect?.(this))
    }
    addEventListener(name, fn, opts = {}) {
      const listeners = this.listeners.get(name) || []
      listeners.push({ fn, once: opts.once })
      this.listeners.set(name, listeners)
    }
    removeEventListener(name, fn) {
      this.listeners.set(name, (this.listeners.get(name) || []).filter((x) => x.fn !== fn))
    }
    emit(name, value = {}) {
      for (const listener of [...(this.listeners.get(name) || [])]) {
        if (listener.once) this.removeEventListener(name, listener.fn)
        try { listener.fn(value) } catch (error) { callbackErrors.push(error) }
      }
    }
    reply(body) { this.emit('message', { data: JSON.stringify(body) }) }
    send(text) {
      const message = JSON.parse(text)
      messages.push(message)
      this.plan.send?.(this, message)
    }
    close() { this.closed += 1 }
  }
  const api = vm.runInNewContext(`${functions}\n({ evaluateCdp, evaluateRendererWithRetries, waitForRendererReady })`, {
    WebSocket: options.noWebSocket ? undefined : Socket,
    Error, Date, Number, JSON, Object, String,
    setTimeout(fn, ms) {
      const timer = setTimeout(() => { timers.delete(timer); fn() }, ms)
      timers.add(timer)
      return timer
    },
    clearTimeout(timer) { timers.delete(timer); clearTimeout(timer) },
    sleep: async () => {},
    async waitForDebugTarget(_port, budget) {
      discoveries += 1
      if (options.discover) return options.discover(budget)
      return { webSocketDebuggerUrl: 'ws://127.0.0.1:1/devtools/page/synthetic' }
    }
  })
  t.after(() => { for (const timer of timers) clearTimeout(timer) })
  return { ...api, sockets, messages, callbackErrors, timers,
    get discoveries() { return discoveries } }
}
const opened = (send) => ({ connect: (socket) => socket.emit('open'), send })
const success = opened((socket, message) => socket.reply({ id: message.id, result: { result: { value: 42 } } }))
const url = 'ws://127.0.0.1:1/devtools/page/synthetic'
async function outcome(promise, watchdog = 70) {
  let timer
  try {
    return await Promise.race([
      promise.then((value) => ({ value }), (error) => ({ error })),
      new Promise((resolve) => { timer = setTimeout(() => resolve({ watchdog: true }), watchdog) })
    ])
  } finally { clearTimeout(timer) }
}

// These tests are transport/QA-owner evidence, not Electron, Core, or installed acceptance.
test('valid response keeps the expression and result, sends once', async (t) => {
  const f = fixture(t, [success])
  assert.equal(await f.evaluateCdp(url, '42', 100), 42)
  assert.equal(f.messages.length, 1)
  assert.equal(f.messages[0].params.expression, '42')
  assert.equal(f.messages[0].params.awaitPromise, true)
})
test('success releases every event listener and timer', async (t) => {
  const f = fixture(t, [success])
  await f.evaluateCdp(url, '42', 100)
  assert.equal(f.sockets[0].closed, 1)
  assert.equal([...f.sockets[0].listeners.values()].flat().length, 0)
  assert.equal(f.timers.size, 0)
})
test('stalled websocket handshake is bounded before any mutation', async (t) => {
  const f = fixture(t, [{}])
  const result = await outcome(f.evaluateCdp(url, 'mutation()', 10))
  assert.ok(result.error, 'handshake exceeded its own deadline')
  assert.equal(f.messages.length, 0)
  assert.equal(f.sockets[0].closed, 1)
})
test('pre-open failure closes the socket', async (t) => {
  const f = fixture(t, [{ connect: (socket) => socket.emit('error', new Error('WebSocket unavailable')) }])
  const result = await outcome(f.evaluateCdp(url, 'mutation()', 100))
  assert.ok(result.error)
  assert.equal(f.messages.length, 0)
  assert.equal(f.sockets[0].closed, 1)
})
test('post-send close rejects promptly rather than waiting for evaluation timeout', async (t) => {
  const f = fixture(t, [opened((socket) => socket.emit('close'))])
  const result = await outcome(f.evaluateCdp(url, 'mutation()', 500), 50)
  assert.ok(result.error, 'closed transport was left waiting')
  assert.equal(f.messages.length, 1)
})
test('post-send transport error rejects promptly', async (t) => {
  const f = fixture(t, [opened((socket) => socket.emit('error', new Error('WebSocket disconnected')))])
  const result = await outcome(f.evaluateCdp(url, 'mutation()', 500), 50)
  assert.ok(result.error, 'failed transport was left waiting')
})
test('malformed remote JSON settles without an uncaught callback', async (t) => {
  const f = fixture(t, [opened((socket) => socket.emit('message', { data: '{' }))])
  const result = await outcome(f.evaluateCdp(url, 'mutation()', 500), 50)
  assert.equal(f.callbackErrors.length, 0)
  assert.ok(result.error)
})
test('remote exception data is not copied into the error receipt', async (t) => {
  const f = fixture(t, [opened((socket, message) => socket.reply({
    id: message.id, result: { exceptionDetails: {
      text: 'PRIVATE_CANARY_/synthetic/private', exception: { className: 'TypeError' }
    } }
  }))])
  const result = await outcome(f.evaluateCdp(url, 'mutation()', 100))
  assert.ok(result.error)
  assert.doesNotMatch(String(result.error), /PRIVATE_CANARY/)
  assert.equal(result.error.cdpRemoteKind, 'exception')
  assert.equal(result.error.cdpExceptionClass, 'TypeError')
})
test('protocol error keeps only its numeric code', async (t) => {
  const f = fixture(t, [opened((socket, message) => socket.reply({
    id: message.id, error: { code: -32000, message: 'PRIVATE_CANARY_/synthetic/private' }
  }))])
  const result = await outcome(f.evaluateCdp(url, 'mutation()', 100))
  assert.equal(result.error.cdpRemoteKind, 'protocol_error')
  assert.equal(result.error.cdpProtocolCode, -32000)
  assert.doesNotMatch(String(result.error), /PRIVATE_CANARY/)
})
test('read-only startup probe waits for two healthy observations before mutation', async (t) => {
  let probes = 0
  const f = fixture(t, [opened((socket, message) => {
    probes += 1
    socket.reply({ id: message.id, result: { result: { value: probes >= 2 ? 'ready' : 'runtime_bridge_missing' } } })
  })])
  await f.waitForRendererReady({ debugPort: 1, deadline: Date.now() + 300 })
  assert.equal(probes, 3)
  assert.ok(f.messages.every((message) => message.params.expression.includes("runtimeRequest('/health', 'GET')")))
  assert.equal(f.messages.length, 3)
})
test('startup probe reports a fixed bridge phase without remote details', async (t) => {
  const f = fixture(t, [
    opened((socket, message) => {
      socket.reply({ id: message.id, result: { result: { value: 'runtime_bridge_missing' } } })
    }),
    {} // The final read-only connection may consume the remaining deadline.
  ])
  // Leave fixture scheduling slack; the product deadline is supplied by the caller.
  const result = await outcome(f.waitForRendererReady({ debugPort: 1, deadline: Date.now() + 300 }), 700)
  assert.equal(result.error?.message, 'cdp_renderer_readiness_timeout')
  assert.equal(result.error?.cdpPreflightLastFailure, 'runtime_bridge_missing')
  assert.ok(f.messages.every((message) => message.params.expression.includes("runtimeRequest('/health', 'GET')")))
})
test('startup probe classifies unavailable debug targets without a raw fetch error', async (t) => {
  const f = fixture(t, [], { discover: async () => {
    throw new Error('renderer debug target not ready: PRIVATE_CANARY_/synthetic/private')
  } })
  const result = await outcome(f.waitForRendererReady({ debugPort: 1, deadline: Date.now() + 100 }), 500)
  assert.equal(result.error?.cdpPreflightLastFailure, 'cdp_debug_target_unavailable')
  assert.doesNotMatch(String(result.error?.cdpPreflightLastFailure), /PRIVATE_CANARY/)
})
test('an unrelated response id is ignored without resending', async (t) => {
  const f = fixture(t, [opened((socket, message) => {
    socket.reply({ id: 99, result: { result: { value: false } } })
    socket.reply({ id: message.id, result: { result: { value: 42 } } })
  })])
  assert.equal(await f.evaluateCdp(url, '42', 100), 42)
  assert.equal(f.messages.length, 1)
})
test('lost response after mutation never replays the entire journey', async (t) => {
  const f = fixture(t, [opened((socket, message) => socket.reply({
    id: message.id, error: { message: 'Execution context was destroyed' }
  })), success])
  const result = await outcome(f.evaluateRendererWithRetries({ debugPort: 1, expression: 'mutation()', timeoutMs: 500 }))
  assert.ok(result.error, 'ambiguous mutation was converted to a successful replay')
  assert.equal(f.messages.length, 1)
})
test('a genuine pre-send connection failure may retry within the same budget', async (t) => {
  const f = fixture(t, [
    { connect: (socket) => socket.emit('error', new Error('WebSocket connect failed')) }, success
  ])
  assert.equal(await f.evaluateRendererWithRetries({ debugPort: 1, expression: '42', timeoutMs: 500 }), 42)
  assert.equal(f.messages.length, 1)
  assert.equal(f.sockets.length, 2)
})
test('a thrown send is ambiguous and is not retried', async (t) => {
  const f = fixture(t, [opened(() => { throw new Error('WebSocket send failed') }), success])
  const result = await outcome(f.evaluateRendererWithRetries({ debugPort: 1, expression: 'mutation()', timeoutMs: 500 }))
  assert.ok(result.error)
  assert.equal(f.messages.length, 1)
})
test('handshake time is included in the timeout sent to CDP', async (t) => {
  const f = fixture(t, [{ connect: (socket) => setTimeout(() => socket.emit('open'), 20), send: success.send }])
  await f.evaluateCdp(url, '42', 200)
  assert.ok(f.messages[0].params.timeout < 200)
  assert.ok(f.messages[0].params.timeout > 0)
})
test('discovery consuming the full deadline cannot start a later mutation', async (t) => {
  const f = fixture(t, [success], { discover: async () => { await pause(20); return { webSocketDebuggerUrl: url } } })
  const result = await outcome(f.evaluateRendererWithRetries({ debugPort: 1, expression: 'mutation()', timeoutMs: 10 }))
  assert.ok(result.error)
  assert.equal(f.messages.length, 0)
})
test('missing websocket support does not start a request', async (t) => {
  const f = fixture(t, [success], { noWebSocket: true })
  const result = await outcome(f.evaluateCdp(url, '42', 100))
  assert.ok(result.error)
  assert.equal(f.sockets.length, 0)
})
test('invalid deadline is rejected before connecting', async (t) => {
  const f = fixture(t, [success])
  const result = await outcome(f.evaluateCdp(url, 'mutation()', NaN))
  assert.ok(result.error)
  assert.equal(f.sockets.length, 0)
})

// Exercise the same renderer stage code and existing bounded() helper.
const boundedMatch = source.match(/ {4}async function bounded\([\s\S]*?\n {4}\}/u)
const writeStage = source.indexOf("out.stage = 'settings-write';")
const stageStart = source.lastIndexOf('      try {', writeStage)
const healthStart = source.indexOf("      const health = ", writeStage)
assert.ok(boundedMatch && writeStage >= 0 && stageStart >= 0)
const settingsFragment = boundedMatch[0] + '\n' + source.slice(stageStart, healthStart < 0 ? source.length : healthStart)
const run = vm.runInNewContext(`(async function(api, profilePatch, out){${settingsFragment}\nreturn out;})`, {
  Promise, Error, String,
  // Exercise the actual 20,000ms timer contract without a 20-second test wait.
  setTimeout(fn, ms) { assert.equal(ms, 20000); return setTimeout(fn, 5) }, clearTimeout
})
const pending = () => new Promise(() => {})
async function watch(promise) {
  let timer
  try { return await Promise.race([promise, new Promise((resolve) => {timer = setTimeout(() => resolve({watchdog:true}), 50)})]) }
  finally { clearTimeout(timer) }
}
test('a stalled settings write returns its stage and never requests a restart', async () => {
  let restartCount = 0
  const result = await watch(run({ settings:{setSettings:pending}, runtime:{restartRuntime:()=>{restartCount++}} },{},{}))
  assert.equal(result.stage,'settings-write')
  assert.match(result.settingsPatchError,/settings_write_timeout/)
  assert.equal(restartCount,0)
})
test('a stalled restart retains successful settings receipt and restart stage', async () => {
  const result = await watch(run({settings:{setSettings:async()=>{}},runtime:{restartRuntime:pending}},{},{}))
  assert.equal(result.stage,'runtime-restart')
  assert.equal(result.settingsProfilePatchAccepted,true)
  assert.match(result.restartError,/runtime_restart_timeout/)
})
test('normal writes and restart occur once and keep original success flags', async () => {
  const calls=[]
  const result = await run({settings:{setSettings:async()=>calls.push('settings')},runtime:{restartRuntime:async()=>calls.push('restart')}},{},{})
  assert.deepEqual(calls,['settings','restart'])
  assert.equal(result.settingsPatchMode,'provider-profile')
  assert.equal(result.restartOk,true)
})
test('settings rejection stops the sequence and preserves its failure', async () => {
  let count=0
  const result=await run({settings:{setSettings:async()=>{throw new Error('synthetic_write_failure')}},runtime:{restartRuntime:async()=>count++}},{},{})
  assert.match(result.settingsPatchError,/synthetic_write_failure/)
  assert.equal(count,0)
})

const replayStart = source.indexOf('    async function collectSseReplay(')
const replayEnd = source.indexOf('    async function collectGateReplay(', replayStart)
assert.ok(replayStart >= 0)
const replayFunction = source.slice(replayStart, replayEnd < 0 ? source.length : replayEnd)
function replayFixture(t, events, { failStart = false, failAck = false, ack = true } = {}) {
  const listeners = new Map()
  const timers = new Set()
  let stops = 0
  const subscribe = (name, fn) => { listeners.set(name, fn); return () => listeners.delete(name) }
  const runtime = {
    onSseEvent: fn => subscribe('events', fn),
    onSseError: fn => subscribe('error', fn),
    onSseEnd: fn => subscribe('end', fn),
    async startSse(_thread, _cursor, streamId) {
      if (failStart) throw new Error('synthetic_start_failure')
      listeners.get('events')({ streamId, events })
      listeners.get('end')({ streamId })
    },
    async stopSse() { stops++ }
  }
  const collect = vm.runInNewContext(`${boundedMatch[0]}\n${replayFunction}\ncollectSseReplay`, {
    api: { runtime }, Promise, Error, Math, Set, Map,
    replayCursorByThread: new Map(), publicReplayEvents: batch => batch,
    gateEventsByTurn: new Map(),
    acknowledgeReplayBatch: async () => { if (failAck) throw new Error('synthetic_ack_failure'); return ack },
    setTimeout(fn) { const timer=setTimeout(()=>{timers.delete(timer);fn()},20);timers.add(timer);return timer },
    clearTimeout(timer) { timers.delete(timer);clearTimeout(timer) }
  })
  t.after(()=>{for(const timer of timers)clearTimeout(timer)})
  return { collect, listeners, timers, get stops(){return stops} }
}
const event = (kind, extra = {}) => ({ kind, turnId:'turn-a', ...extra })
const assistant = event('item_completed', {item:{kind:'assistant_text',text:'expected'}})
const complete = [event('turn_started'), event('usage'), assistant, event('turn_completed')]
test('SSE end cannot manufacture success when usage is missing',async(t)=>{
  const f=replayFixture(t,complete.filter(x=>x.kind!=='usage'))
  assert.equal((await f.collect('thread-a','turn-a','expected',false)).ok,false)
})
test('SSE end cannot manufacture success when turn_started is missing',async(t)=>{
  const f=replayFixture(t,complete.filter(x=>x.kind!=='turn_started'))
  assert.equal((await f.collect('thread-a','turn-a','expected',false)).ok,false)
})
test('a complete acknowledged replay still passes',async(t)=>{
  const f=replayFixture(t,complete)
  assert.equal((await f.collect('thread-a','turn-a','expected',false)).ok,true)
  assert.equal(f.listeners.size,0)
  assert.equal(f.stops,1)
})
test('tool events do not excuse a missing usage event',async(t)=>{
  const f=replayFixture(t,[...complete.filter(x=>x.kind!=='usage'),event('tool_call_ready'),event('tool_call_started'),event('tool_call_finished')])
  assert.equal((await f.collect('thread-a','turn-a','expected',true)).ok,false)
})
test('denied terminal does not excuse a missing turn_started',async(t)=>{
  const f=replayFixture(t,[event('usage'),event('turn_completed',{terminalReason:'approval_denied'})])
  assert.equal((await f.collect('thread-a','turn-a','expected',false,'approval_denied')).ok,false)
})
test('cancel replay requires the aborted terminal and usage without a started tool',async(t)=>{
  const f=replayFixture(t,[event('turn_started'),event('approval_requested'),event('usage'),event('turn_aborted',{terminalReason:'cancel'})])
  assert.equal((await f.collect('thread-a','turn-a','',false,'cancel')).ok,true)
})
test('cancel replay rejects a late tool start or missing usage',async(t)=>{
  for(const events of [
    [event('turn_started'),event('usage'),event('tool_call_started'),event('turn_aborted',{terminalReason:'cancel'})],
    [event('turn_started'),event('turn_aborted',{terminalReason:'cancel'})],
    [event('turn_started'),event('usage'),event('turn_completed',{terminalReason:'cancel'})]
  ]){
    const f=replayFixture(t,events)
    assert.equal((await f.collect('thread-a','turn-a','',false,'cancel')).ok,false)
  }
})
test('failed SSE startup still disposes owned listeners and stream',async(t)=>{
  const f=replayFixture(t,[],{failStart:true})
  await assert.rejects(f.collect('thread-a','turn-a','expected',false),/synthetic_start_failure/)
  assert.equal(f.listeners.size,0)
  assert.equal(f.stops,1)
  assert.equal(f.timers.size,0)
})
test('failed acknowledgement still disposes owned listeners and stream',async(t)=>{
  const f=replayFixture(t,complete,{failAck:true})
  await assert.rejects(f.collect('thread-a','turn-a','expected',false),/synthetic_ack_failure/)
  assert.equal(f.listeners.size,0)
  assert.equal(f.stops,1)
})
test('unacknowledged complete replay remains a failure',async(t)=>{
  const f=replayFixture(t,complete,{ack:false})
  assert.equal((await f.collect('thread-a','turn-a','expected',false)).ok,false)
})

test('a gate stream and its continuation jointly prove the denied terminal', async (t) => {
  const gateStart = source.indexOf('    async function collectGateReplay(')
  const gateEnd = source.indexOf('\n    try {\n      if (!api)', gateStart)
  assert.ok(gateStart >= 0 && gateEnd > gateStart)
  const listeners = new Map()
  const timers = new Set()
  let stops = 0
  const subscribe = (name, fn) => { listeners.set(name, fn); return () => listeners.delete(name) }
  const api = { runtime: {
    onSseEvent: fn => subscribe('event', fn),
    onSseError: fn => subscribe('error', fn),
    onSseEnd: fn => subscribe('end', fn),
    async ackSseEvent() { return true },
    async stopSse() { stops++ },
    async startSse(_thread, _cursor, streamId) {
      const gate = streamId.startsWith('packaged-session-gate-')
      listeners.get('event')({ streamId, events: gate
        ? [event('turn_started', { seq: 1 }), event('approval_requested', { seq: 2, approvalId: 'approval-a' })]
        : [event('usage', { seq: 3 }), event('turn_completed', { seq: 4, terminalReason: 'approval_denied' })] })
      listeners.get('end')({ streamId })
    }
  } }
  const context = {
    api, Promise, Error, Math, Set, Map, Number, String,
    replayCursorByThread: new Map(), publicReplayEvents: batch => batch,
    gateEventsByTurn: new Map(),
    async acknowledgeReplayBatch(_thread, _stream, batch) {
      const maxSeq = Math.max(...batch.map(item => item.seq))
      context.replayCursorByThread.set('thread-a', maxSeq)
      return true
    },
    setTimeout(fn) { const timer=setTimeout(()=>{timers.delete(timer);fn()},20);timers.add(timer);return timer },
    clearTimeout(timer) { timers.delete(timer);clearTimeout(timer) }
  }
  const functions = boundedMatch[0] + '\n' + replayFunction + '\n' + source.slice(gateStart, gateEnd)
  const replay = vm.runInNewContext(`${functions}\n({ collectGateReplay, collectSseReplay })`, context)
  t.after(() => { for (const timer of timers) clearTimeout(timer) })
  const gate = await replay.collectGateReplay('thread-a', 'turn-a', 'approval_requested', 'approvalId')
  assert.equal(gate.ok, true)
  assert.equal(gate.id, 'approval-a')
  const final = await replay.collectSseReplay('thread-a', 'turn-a', '', false, 'approval_denied')
  assert.equal(final.ok, true)
  assert.equal(stops, 2)
  assert.equal(listeners.size, 0)
})

test('failed gate SSE startup releases the stream, listeners, and timer', async (t) => {
  const gateStart = source.indexOf('    async function collectGateReplay(')
  const gateEnd = source.indexOf('\n    try {\n      if (!api)', gateStart)
  const listeners = new Map()
  const timers = new Set()
  let stops = 0
  const runtime = {
    onSseEvent(fn) { listeners.set('event', fn); return () => listeners.delete('event') },
    onSseError(fn) { listeners.set('error', fn); return () => listeners.delete('error') },
    onSseEnd(fn) { listeners.set('end', fn); return () => listeners.delete('end') },
    async startSse() { throw new Error('synthetic_gate_start_failure') },
    async stopSse() { stops++ }
  }
  const gate = vm.runInNewContext(`${boundedMatch[0]}\n${source.slice(gateStart, gateEnd)}\ncollectGateReplay`, {
    api: { runtime }, Promise, Error, Math, Set, Map, Number, String,
    replayCursorByThread: new Map(), gateEventsByTurn: new Map(),
    publicReplayEvents: batch => batch, acknowledgeReplayBatch: async () => true,
    setTimeout(fn) { const timer=setTimeout(()=>{timers.delete(timer);fn()},20);timers.add(timer);return timer },
    clearTimeout(timer) { timers.delete(timer);clearTimeout(timer) }
  })
  t.after(() => { for (const timer of timers) clearTimeout(timer) })
  await assert.rejects(gate('thread-a', 'turn-a', 'approval_requested', 'approvalId'), /synthetic_gate_start_failure/)
  assert.equal(stops, 1)
  assert.equal(listeners.size, 0)
  assert.equal(timers.size, 0)
})

test('HTTP failure reports a fixed code without response text or a dynamic path', async () => {
  const requestStart = source.indexOf('    async function request(')
  const requestEnd = source.indexOf('\n    const gateEventsByTurn', requestStart)
  assert.ok(requestStart >= 0 && requestEnd > requestStart)
  const request = vm.runInNewContext(`${source.slice(requestStart, requestEnd)}\nrequest`, {
    api: { runtime: { runtimeRequest: async () => ({
      status: 400,
      body: '{"code":"validation_error","message":"PRIVATE_CANARY dynamic-thread-42"}'
    }) } },
    parseJSON: response => JSON.parse(response.body), JSON, Error, Number, String
  })
  await assert.rejects(request('/v1/threads/dynamic-thread-42/fork', 'POST', {}), error => {
    assert.match(error.message, /status=400 code=validation_error/)
    assert.doesNotMatch(error.message, /PRIVATE_CANARY|dynamic-thread-42/)
    return true
  })
})

function relaunchExpression() {
  const start = source.indexOf('function buildRelaunchExpression(')
  const end = source.indexOf('\nfunction summarizeError(', start)
  assert.ok(start >= 0 && end > start)
  const build = vm.runInNewContext(`${source.slice(start, end)}\nbuildRelaunchExpression`, { JSON })
  return build({ threadId: 'thread-a', turnId: 'turn-a', providerId: 'xiaomi', model: 'mimo-v2.5-pro' })
}

test('relaunch refuses a continuation when the exact prior turn is absent', async () => {
  const calls = []
  const result = await vm.runInNewContext(relaunchExpression(), {
    window: { analytix: { runtime: { async runtimeRequest(path, method) {
      calls.push({ path, method })
      return { status: 200, body: JSON.stringify({ id: 'thread-a', turns: [] }) }
    } } } },
    JSON, Error, Date, Promise, setTimeout, encodeURIComponent
  })
  assert.equal(result.historyRecovered, false)
  assert.equal(result.continuationCreated, false)
  assert.deepEqual(calls, [{ path: '/v1/threads/thread-a', method: 'GET' }])
})

test('relaunch reads old history before one continuation and observes its terminal state', async () => {
  const calls = []
  let reads = 0
  const result = await vm.runInNewContext(relaunchExpression(), {
    window: { analytix: { runtime: { async runtimeRequest(path, method, payload) {
      calls.push({ path, method, payload })
      if (method === 'POST') return { status: 200, body: JSON.stringify({
        threadId: 'thread-a', turnId: 'turn-b'
      }) }
      reads += 1
      return { status: 200, body: JSON.stringify({ id: 'thread-a', turns: [
        { id: 'turn-a', status: 'completed' },
        ...(reads > 1 ? [{ id: 'turn-b', status: 'completed' }] : [])
      ] }) }
    } } } },
    JSON, Error, Date, Promise, setTimeout, encodeURIComponent
  })
  assert.equal(result.historyRecovered, true)
  assert.equal(result.continuationCreated, true)
  assert.equal(result.continuationCompleted, true)
  assert.equal(result.beforeTurnCount, 1)
  assert.equal(result.afterTurnCount, 2)
  assert.deepEqual(calls.map(({ method }) => method), ['GET', 'POST', 'GET'])
  assert.equal(JSON.parse(calls[1].payload).prompt, 'Run packaged session soak after new process.')
})

test('relaunch waits for a pending public projection using GET only after the committed POST', async () => {
  const calls = []
  let reads = 0
  const result = await vm.runInNewContext(relaunchExpression(), {
    window: { analytix: { runtime: { async runtimeRequest(path, method) {
      calls.push({ path, method })
      if (method === 'POST') return { status: 200, body: JSON.stringify({
        threadId: 'thread-a', turnId: 'turn-b'
      }) }
      reads += 1
      if (reads === 2) return { status: 503, body: JSON.stringify({ code: 'public_projection_pending' }) }
      return { status: 200, body: JSON.stringify({ id: 'thread-a', turns: [
        { id: 'turn-a', status: 'completed' },
        ...(reads > 2 ? [{ id: 'turn-b', status: 'completed' }] : [])
      ] }) }
    } } } },
    JSON, Error, Date, Promise, setTimeout: (fn) => { fn(); return 0 }, encodeURIComponent
  })
  assert.equal(result.historyRecovered, true)
  assert.equal(result.continuationCreated, true)
  assert.equal(result.continuationCompleted, true)
  assert.deepEqual(calls.map(({ method }) => method), ['GET', 'POST', 'GET', 'GET'])
})

test('relaunch reports a post-write read failure without copying response text or repeating the write', async () => {
  const calls = []
  const result = await vm.runInNewContext(relaunchExpression(), {
    window: { analytix: { runtime: { async runtimeRequest(path, method) {
      calls.push({ path, method })
      if (calls.length === 1) return { status: 200, body: JSON.stringify({ id: 'thread-a', turns: [
        { id: 'turn-a', status: 'completed' }
      ] }) }
      if (method === 'POST') return { status: 200, body: JSON.stringify({
        threadId: 'thread-a', turnId: 'turn-b'
      }) }
      return { status: 503, body: '{"code":"projection_unavailable","message":"PRIVATE_CANARY"}' }
    } } } },
    JSON, Error, Date, Promise, setTimeout, encodeURIComponent
  })
  assert.equal(result.error, 'relaunch_step_failed')
  assert.equal(result.stage, 'continuation-read')
  assert.equal(result.historyRecovered, true)
  assert.equal(result.continuationCreated, true)
  assert.equal(result.httpStatus, 503)
  assert.equal(result.httpCode, 'projection_unavailable')
  assert.doesNotMatch(JSON.stringify(result), /PRIVATE_CANARY/)
  assert.deepEqual(calls.map(({ method }) => method), ['GET', 'POST', 'GET'])
})

test('packaged thread-read diagnostics retain only bounded fixed categories', () => {
  const start = source.indexOf('function parsePackagedThreadReadDiagnostics(')
  const end = source.indexOf('\nfunction smokeChildEnv(', start)
  assert.ok(start >= 0 && end > start)
  const parse = vm.runInNewContext(
    `${source.slice(start, end)}\nparsePackagedThreadReadDiagnostics`, { JSON, Number, String }
  )
  const result = parse([
    '[packaged-thread-read] {"status":503,"code":"public_projection_pending","message":"PRIVATE_CANARY"}',
    '[packaged-thread-read] {"status":503,"code":"accepted_final_hydration_unavailable","hydrationClass":"frontier_torn","message":"PRIVATE_CANARY"}',
    '[packaged-thread-read] {"status":503,"code":"accepted_final_hydration_unavailable","hydrationClass":"PRIVATE_CANARY"}',
    '[packaged-thread-read] {"status":503,"code":"PRIVATE_CANARY"}',
    '[packaged-thread-read] invalid PRIVATE_CANARY'
  ].join('\n'))
  assert.deepEqual(JSON.parse(JSON.stringify(result)), [
    { status: 503, code: 'public_projection_pending' },
    { status: 503, code: 'accepted_final_hydration_unavailable', hydrationClass: 'frontier_torn' },
    { status: 503, code: 'accepted_final_hydration_unavailable' },
    { status: 503, code: 'unclassified' },
    { status: 0, code: 'unclassified' }
  ])
  assert.doesNotMatch(JSON.stringify(result), /PRIVATE_CANARY/)
  const bounded = parse([
    ...Array.from({ length: 12 }, () => '[packaged-thread-read] {"status":503,"code":"public_projection_pending"}'),
    '[packaged-thread-read] {"status":503,"code":"accepted_final_hydration_unavailable","hydrationClass":"manifest_mismatch"}'
  ].join('\n'))
  assert.equal(bounded.length, 12)
  assert.deepEqual(JSON.parse(JSON.stringify(bounded.at(-1))), {
    status: 503, code: 'accepted_final_hydration_unavailable', hydrationClass: 'manifest_mismatch'
  })
})

function rendererJourneyFixture(t, options = {}) {
  const builderStart = source.indexOf('function buildRendererExpression(')
  const builderEnd = source.indexOf('\nfunction buildRelaunchExpression(', builderStart)
  assert.ok(builderStart >= 0 && builderEnd > builderStart)
  const builderContext = {
    JSON, String, forkOnly: options.forkOnly === true,
    focusedForkStage: options.forkOnly ? 'initial' : '',
    relaunchAfterInitial: false, cancelOnly: false
  }
  const build = vm.runInNewContext(`${source.slice(builderStart, builderEnd)}\nbuildRendererExpression`, builderContext)
  const journeyOptions = { providerBaseUrl: 'http://127.0.0.1:1234',
    providerId: 'synthetic-provider', model: 'synthetic-model', runtimePort: 4321,
    runtimeDataDir: '/synthetic/runtime', workspace: '/synthetic/workspace',
    syntheticApiKey: 'synthetic-test-key', progressToken: 'synthetic-run' }
  const expression = options.segmented
    ? vm.runInNewContext(`${source.slice(builderStart, builderEnd)}\nbuildRendererStartExpression`,
      builderContext)(journeyOptions)
    : build(journeyOptions)
  const calls = []
  const turns = new Map()
  const latestTurn = new Map()
  const listeners = { events: new Set(), errors: new Set(), ends: new Set() }
  const timers = new Set()
  let registryLists = 0
  let childCount = 0
  let stoppedStreams = 0
  const response = (value, status = 200) => ({ status, body: JSON.stringify(value) })
  const runtimeRequest = async (path, method = 'GET', raw) => {
    const payload = raw ? JSON.parse(raw) : null
    calls.push({ path, method, payload })
    if (path === '/health') return response({ service: 'analytix' })
    if (path === '/v1/threads' && method === 'POST') {
      if (options.failInitialThread && payload.mode === 'agent') return response({})
      return response({ id: payload.mode === 'plan' ? 'plan-thread' : 'source-thread' })
    }
    if (path === '/v1/attachments' && method === 'POST') return response({
      attachment: { id: 'attachment-1', scope: 'thread', name: 'packaged-attachment.txt' }
    })
    if (path === '/v1/threads?include=side') return response({
      threads: Array.from({ length: childCount }, (_, i) => ({
        id: `fork-${i + 1}`, parentThreadId: 'source-thread', relation: 'fork'
      }))
    })
    if (path.startsWith('/v1/threads?include=side&search=')) return response({
      threads: [{ id: 'source-thread' }, { id: 'fork-1' }]
    })
    if (path.endsWith('/fork') && method === 'POST') {
      childCount += options.forkChildrenAdded ?? 1
      return response({ id: 'fork-1', parentThreadId: 'source-thread' })
    }
    if (path.endsWith('/turns') && method === 'POST') {
      const threadId = path.split('/')[3]
      const turnId = `turn-${turns.size + 1}`
      turns.set(turnId, { threadId, prompt: payload.prompt })
      latestTurn.set(threadId, turnId)
      return response({ threadId, turnId })
    }
    if (path.startsWith('/v1/approvals/') && method === 'POST') return response({
      status: payload.decision === 'deny' ? 'denied' : 'allowed'
    })
    if (path.startsWith('/v1/user-inputs/') && method === 'POST') return response({})
    if (path.endsWith('/resume-thread') && method === 'POST') return response({
      session_id: 'source-thread', thread_id: 'source-thread'
    })
    if (path === '/v1/threads/source-thread' && method === 'GET') return response({
      id: 'source-thread', turns: [...turns.keys()].map((id) => ({ id }))
    })
    if (path.startsWith('/v1/usage?')) return response({
      buckets: [{ thread_id: 'source-thread', total_tokens: 20 }]
    })
    return response({ code: 'synthetic_route_missing' }, 404)
  }
  const subscribe = (kind, fn) => { listeners[kind].add(fn); return () => listeners[kind].delete(fn) }
  const startSse = async (threadId, _cursor, streamId) => {
    const turnId = latestTurn.get(threadId)
    const prompt = turns.get(turnId)?.prompt || ''
    const event = (kind, extra = {}) => ({ kind, turnId, ...extra })
    let events
    if (streamId.includes('-gate-')) {
      events = [event('turn_started'), prompt.includes('user input')
        ? event('user_input_requested', { inputId: 'input-1', questions: [{ id: 'question-1' }] })
        : event('approval_requested', { approvalId: 'approval-1' })]
    } else {
      const denied = prompt.includes('approval deny')
      const text = prompt.includes('tool timeline') ? 'packaged tool timeline ok'
        : prompt.includes('MiMo plan') ? 'packaged mimo plan saved'
          : prompt.includes('attachment fallback') ? 'packaged attachment fallback ok'
            : prompt.includes('approval allow') ? 'packaged approval allow ok'
              : prompt.includes('user input') ? 'packaged user input ok'
                : 'packaged session soak ok'
      events = [event('turn_started'), event('usage'),
        ...(prompt.includes('tool timeline') || prompt.includes('MiMo plan')
          ? [event('tool_call_ready'), event('tool_call_started'), event('tool_call_finished')] : []),
        ...(!denied ? [event('item_completed', { item: { kind: 'assistant_text',
          text: options.failToolReplay && prompt.includes('tool timeline') ? 'wrong text' : text } })] : []),
        event('turn_completed', denied ? { terminalReason: 'approval_denied' } : {})]
    }
    for (const fn of listeners.events) fn({ streamId, events })
    for (const fn of listeners.ends) fn({ streamId })
  }
  const api = {
    settings: {
      getSettings: async () => ({ runtime: {}, provider: {} }),
      setSettings: async () => {}
    },
    runtime: {
      runtimeRequest, startSse,
      onSseEvent: fn => subscribe('events', fn),
      onSseError: fn => subscribe('errors', fn),
      onSseEnd: fn => subscribe('ends', fn),
      ackSseEvent: async () => true,
      stopSse: async () => { stoppedStreams++ },
      restartRuntime: async () => {}
    },
    providerRegistry: { request: async (request) => {
      if (request.operation === 'connect') return {}
      registryLists++
      return { providers: registryLists > 1
        ? [{ id: 'synthetic-provider', credentialConfigured: true }] : [] }
    } }
  }
  const window = { analytix: api }
  const result = vm.runInNewContext(expression, {
    window, document: { title: 'Analytix' },
    location: { href: 'app://synthetic' }, JSON, Promise, Date, Math, Map, Set,
    Number, String, Object, Error, encodeURIComponent, btoa,
    setTimeout(fn, ms) {
      const timer = setTimeout(() => { timers.delete(timer); fn() }, ms)
      timers.add(timer)
      return timer
    },
    clearTimeout(timer) { timers.delete(timer); clearTimeout(timer) }
  })
  t.after(() => { for (const timer of timers) clearTimeout(timer) })
  return { result, calls, listeners, timers, window,
    get stoppedStreams() { return stoppedStreams } }
}

test('actual full renderer expression completes the synthetic dependency journey', async (t) => {
  const fixture = rendererJourneyFixture(t)
  const result = await fixture.result
  assert.equal(result.error, '')
  assert.equal(result.stage, 'complete')
  assert.equal(fixture.window.__analytixPackagedSoakProgress.stage, 'complete')
  assert.equal(fixture.window.__analytixPackagedSoakProgress.token, 'synthetic-run')
  for (const field of ['toolTimelineOk', 'mimoPlanReplayOk', 'attachmentSseReplayOk',
    'approvalDenyNoExecuteOk', 'approvalAllowExecutedOk', 'userInputReplayOk',
    'forkOk', 'forkReplayOk', 'resumeOk', 'resumeReplayOk', 'threadListOk', 'usageOk']) {
    assert.equal(result[field], true, field)
  }
  assert.equal(result.forkChildrenAfter - result.forkChildrenBefore, 1)
  assert.equal(fixture.calls.filter(({ path, method }) => path.endsWith('/fork') && method === 'POST').length, 1)
  assert.equal(fixture.stoppedStreams, 12)
  assert.ok(Object.values(fixture.listeners).every((set) => set.size === 0))
  assert.equal(fixture.timers.size, 0)
})

test('failed tool replay in full mode stops before plan or attachment writes', async (t) => {
  const fixture = rendererJourneyFixture(t, { failToolReplay: true })
  const result = await fixture.result
  assert.equal(result.stage, 'tool-sse-replay')
  assert.equal(fixture.window.__analytixPackagedSoakProgress.stage, 'tool-sse-replay')
  assert.equal(result.toolTimelineOk, false)
  assert.equal(fixture.calls.filter(({ method }) => method === 'POST').length, 3)
  assert.ok(!fixture.calls.some(({ path }) => path === '/v1/attachments' || path.endsWith('/fork')))
  assert.ok(Object.values(fixture.listeners).every((set) => set.size === 0))
  assert.equal(fixture.timers.size, 0)
})

test('full and focused fork reject two children and do not continue a child turn', async (t) => {
  for (const forkOnly of [false, true]) {
    const fixture = rendererJourneyFixture(t, { forkOnly, forkChildrenAdded: 2 })
    const result = await fixture.result
    assert.equal(result.forkChildrenBefore, 0)
    assert.equal(result.forkChildrenAfter, 2)
    assert.equal(result.forkOk, false)
    assert.ok(!fixture.calls.some(({ path, method }) =>
      path === '/v1/threads/fork-1/turns' && method === 'POST'))
  }
})

test('focused fork succeeds with one observed child', async (t) => {
  const fixture = rendererJourneyFixture(t, { forkOnly: true })
  const result = await fixture.result
  assert.equal(result.forkOk, true)
  assert.equal(result.forkChildrenBefore, 0)
  assert.equal(result.forkChildrenAfter, 1)
  assert.equal(fixture.calls.filter(({ path }) => path.endsWith('/fork')).length, 1)
})

test('missing initial thread id stops before a turn write', async (t) => {
  const fixture = rendererJourneyFixture(t, { failInitialThread: true })
  const result = await fixture.result
  assert.equal(result.threadCreateOk, false)
  assert.ok(!fixture.calls.some(({ path }) => path.endsWith('/turns')))
})

test('explicit diagnostic retention preserves a quiesced full-mode profile', () => {
  const start = source.indexOf('    if (tempHome) {', source.indexOf('async function runActualPackagedSoak('))
  const end = source.indexOf('\n  }\n\n  const providerRequests', start)
  assert.ok(start >= 0 && end > start)
  const cleanup = source.slice(start, end)
  for (const retainDiagnosticProfile of [false, true]) {
    const removed = []
    const result = vm.runInNewContext(`(() => { let diagnosticProfilePath = ''; ${cleanup}
      return diagnosticProfilePath; })()`, {
      tempHome: '/synthetic/profile', cleanupQuiesced: true, retainDiagnosticProfile,
      join: (...parts) => parts.join('/'), existsSync: () => false,
      readFileSync: () => { throw new Error('absent') },
      rmSync: path => removed.push(path)
    })
    assert.equal(result, retainDiagnosticProfile ? '/synthetic/profile' : '')
    assert.deepEqual(removed, retainDiagnosticProfile ? [] : ['/synthetic/profile'])
  }
})

test('progress probe reads only the matching run and fixed stage', async () => {
  const start = source.indexOf('async function readRendererProgress(')
  const end = source.indexOf('\nfunction buildRelaunchExpression(', start)
  assert.ok(start >= 0 && end > start)
  const window = { __analytixPackagedSoakProgress: {
    token: 'expected-run', stage: 'approval-allow-gate', revision: 7,
    changedAt: Date.now() - 100
  } }
  const expressions = []
  const probe = vm.runInNewContext(`${source.slice(start, end)}\nreadRendererProgress`, {
    JSON, Date, Number,
    waitForDebugTarget: async () => ({ webSocketDebuggerUrl: 'ws://synthetic' }),
    evaluateCdp: async (_url, expression) => {
      expressions.push(expression)
      return vm.runInNewContext(expression, { window })
    }
  })
  const matching = await probe(1234, 'expected-run')
  assert.equal(matching.status, 'observed')
  assert.equal(matching.stage, 'approval-allow-gate')
  assert.equal(matching.revision, 7)
  assert.equal(matching.outcomeStatus, 'running')
  window.__analytixPackagedSoakOutcome = {
    token: 'expected-run', status: 'complete', result: { stage: 'complete' }
  }
  const finished = await probe(1234, 'expected-run')
  assert.equal(finished.outcomeStatus, 'complete')
  assert.equal(finished.result.stage, 'complete')
  assert.equal((await probe(1234, 'different-run')).status, 'unavailable')
  assert.ok(expressions.every((expression) =>
    !/runtimeRequest|\bPOST\b|startSse|restartRuntime/.test(expression)))
})

test('segmented start runs one full renderer journey and records its final result', async (t) => {
  const fixture = rendererJourneyFixture(t, { segmented: true })
  const acknowledgement = await fixture.result
  assert.equal(acknowledgement.started, true)
  assert.equal(acknowledgement.token, 'synthetic-run')
  for (let i = 0; i < 20 && fixture.window.__analytixPackagedSoakOutcome?.status !== 'complete'; i++) {
    await pause(5)
  }
  assert.equal(fixture.window.__analytixPackagedSoakOutcome.status, 'complete')
  assert.equal(fixture.window.__analytixPackagedSoakOutcome.result.stage, 'complete')
  assert.equal(fixture.calls.filter(({ path }) => path.endsWith('/fork')).length, 1)
  assert.ok(Object.values(fixture.listeners).every((set) => set.size === 0))
})

test('segmented observer accepts only read-only progress after an ambiguous start', async () => {
  const start = source.indexOf('async function observeRendererJourneyByStage(')
  const end = source.indexOf('\nfunction buildRelaunchExpression(', start)
  assert.ok(start >= 0 && end > start)
  let sends = 0
  let reads = 0
  const observe = vm.runInNewContext(`${source.slice(start, end)}\nobserveRendererJourneyByStage`, {
    Date, Error,
    async evaluateRendererWithRetries() {
      sends++
      throw Object.assign(new Error('cdp_connection_closed'), { cdpOutcome: 'unknown' })
    },
    async readRendererProgress() {
      reads++
      return reads === 1
        ? { status: 'observed', stage: 'tool-sse-replay', ageMs: 50, outcomeStatus: 'running' }
        : { status: 'observed', stage: 'complete', ageMs: 0,
          outcomeStatus: 'complete', result: { stage: 'complete' } }
    },
    sleep: async () => {}
  })
  const result = await observe({ debugPort: 1234, expression: 'synthetic-mutating-start',
    progressToken: 'run-1', startTimeoutMs: 1000 })
  assert.equal(result.stage, 'complete')
  assert.equal(sends, 1)
  assert.equal(reads, 2)
})
