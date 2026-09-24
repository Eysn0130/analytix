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
    socket.reply({ id: message.id, result: { result: { value: probes >= 2 } } })
  })])
  await f.waitForRendererReady({ debugPort: 1, deadline: Date.now() + 300 })
  assert.equal(probes, 3)
  assert.ok(f.messages.every((message) => message.params.expression.includes("runtimeRequest('/health', 'GET')")))
  assert.equal(f.messages.length, 3)
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
