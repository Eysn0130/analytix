#!/usr/bin/env node

import http from 'node:http'
import { spawn } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { performance } from 'node:perf_hooks'
import { fileURLToPath, pathToFileURL } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const repoRoot = resolve(here, '..')
const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const includeDiagnostics = args.has('--include-diagnostics')
const READY_PREFIX = 'ANALYTIX_RUNTIME_SERVER_READY '
const DEFAULT_TIMEOUT_MS = 45_000
// Browser preview account mode intentionally patches settings to Analytix Hub.
// Keep the local fake runtime provider aligned with that UI path so the
// benchmark exercises the same authenticated renderer branch without secrets.
const LOCAL_PROVIDER_ID = 'analytix-hub'
const LOCAL_MODEL = 'qwen-plus'

function argValue(name, fallback = '') {
  const inline = rawArgs.find((item) => item.startsWith(`${name}=`))
  if (inline) return inline.slice(name.length + 1)
  const index = rawArgs.indexOf(name)
  return index >= 0 ? rawArgs[index + 1] || '' : fallback
}

function sha256(value) {
  return createHash('sha256').update(String(value)).digest('hex')
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

async function listen(server) {
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('server did not bind to TCP')
  return `http://127.0.0.1:${address.port}`
}

function getFreePort() {
  return new Promise((resolve, reject) => {
    const server = http.createServer()
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      server.close(() => {
        if (!address || typeof address === 'string') reject(new Error('failed to allocate port'))
        else resolve(address.port)
      })
    })
  })
}

async function readJSONBody(req) {
  let body = ''
  for await (const chunk of req) body += String(chunk)
  try {
    return body.trim() ? JSON.parse(body) : {}
  } catch {
    return {}
  }
}

function sseFrame(payload) {
  return `data: ${JSON.stringify(payload)}\n\n`
}

function chunkText(text, size) {
  const out = []
  for (let index = 0; index < text.length; index += size) out.push(text.slice(index, index + size))
  return out
}

function scenarioFromPrompt(prompt) {
  const lower = String(prompt || '').toLowerCase()
  if (lower.includes('[tool]')) return 'tool'
  if (lower.includes('[proxy-burst]')) return 'proxy-burst'
  if (lower.includes('[long]')) return 'long'
  if (lower.includes('[reasoning]')) return 'reasoning'
  if (lower.includes('[markdown]')) return 'markdown'
  return 'short'
}

function providerWriteSummary(requests) {
  const firstDelays = []
  const intervals = []
  let writeCount = 0
  let totalBytes = 0
  let maxBytesPerWrite = 0
  let maxFramesPerWrite = 0
  for (const request of requests) {
    const writes = request.writes || []
    if (writes.length > 0) {
      firstDelays.push(writes[0].relativeMs)
    }
    for (let index = 0; index < writes.length; index += 1) {
      const write = writes[index]
      writeCount += 1
      totalBytes += write.bytes
      maxBytesPerWrite = Math.max(maxBytesPerWrite, write.bytes)
      maxFramesPerWrite = Math.max(maxFramesPerWrite, write.frames)
      if (index > 0) intervals.push(write.relativeMs - writes[index - 1].relativeMs)
    }
  }
  const writeIntervalP95 = percentile(intervals, 95)
  const firstWriteDelay = percentile(firstDelays, 50)
  const batched = maxFramesPerWrite > 8 || maxBytesPerWrite > 4096 || Number(writeIntervalP95 || 0) > 300
  return {
    request_count: requests.length,
    write_count: writeCount,
    first_write_delay_ms: firstWriteDelay,
    write_interval_p95_ms: writeIntervalP95,
    max_bytes_per_write: maxBytesPerWrite,
    max_frames_per_write: maxFramesPerWrite,
    total_bytes: totalBytes,
    cadence: batched ? 'provider_or_proxy_batched' : 'provider_incremental'
  }
}

function recordProviderWrite(request, data, frames) {
  const now = performance.now()
  request.writes.push({
    atMs: now,
    relativeMs: Math.round(now - request.atMs),
    bytes: Buffer.byteLength(data),
    frames
  })
}

function writeProviderData(res, request, data, frames = 1) {
  recordProviderWrite(request, data, frames)
  res.write(data)
}

async function emitSseFrames(res, request, frames, options) {
  const delayMs = Math.max(0, Number(options.delayMs || 0))
  const burstFrames = Math.max(1, Number(options.burstFrames || 1))
  let pending = []
  for (const frame of frames) {
    pending.push(frame)
    if (pending.length >= burstFrames) {
      writeProviderData(res, request, pending.join(''), pending.length)
      pending = []
    }
    await sleep(delayMs)
  }
  if (pending.length > 0) {
    writeProviderData(res, request, pending.join(''), pending.length)
  }
}

function startFakeProvider({ chunkDelayMs = 24, firstDelayMs = 60 }) {
  const requests = []
  const server = http.createServer(async (req, res) => {
    const body = await readJSONBody(req)
    const prompt = Array.isArray(body.messages)
      ? body.messages.map((message) => message?.content).filter(Boolean).join('\n')
      : ''
    const scenario = scenarioFromPrompt(prompt)
    const hasToolResult = Array.isArray(body.messages) && body.messages.some((message) => (
      message?.role === 'tool' || message?.role === 'function'
    ))
    const requestRecord = {
      atMs: performance.now(),
      url: req.url || '',
      scenario,
      model: typeof body.model === 'string' ? body.model : '',
      stream: body.stream === true,
      messageCount: Array.isArray(body.messages) ? body.messages.length : 0,
      toolCount: Array.isArray(body.tools) ? body.tools.length : 0,
      hasToolResult,
      bodyHash: sha256(JSON.stringify(body)),
      writes: []
    }
    requests.push(requestRecord)

    res.writeHead(200, {
      'content-type': 'text/event-stream; charset=utf-8',
      'cache-control': 'no-cache, no-transform',
      connection: 'keep-alive'
    })
    res.flushHeaders?.()
    await sleep(firstDelayMs)

    const burstFrames = scenario === 'proxy-burst'
      ? Math.max(2, Number(argValue('--provider-burst-frames', '18')))
      : 1

    const reasoning = scenario === 'long' || scenario === 'proxy-burst'
      ? Array.from({ length: 18 }, (_, index) =>
          `第${index + 1}步确认长思考流仍然按小增量进入界面，不触发整页重排。`
        ).join('')
      : scenario === 'reasoning'
      ? '先确认任务边界，再检查可见反馈、流式增量和完成态收束。'
      : scenario === 'tool' && !hasToolResult
        ? '我会先读取一个安全的项目文件，再给出工具路径是否连续的结论。'
      : ''
    await emitSseFrames(
      res,
      requestRecord,
      chunkText(reasoning, 6).map((part) => sseFrame({ choices: [{ delta: { reasoning_content: part }, finish_reason: null }] })),
      { delayMs: chunkDelayMs, burstFrames }
    )

    if (scenario === 'tool' && !hasToolResult) {
      await emitSseFrames(
        res,
        requestRecord,
        [
          sseFrame({
            choices: [{
              delta: {
                tool_calls: [{
                  index: 0,
                  id: 'call_bench_read',
                  type: 'function',
                  function: { name: 'read_file', arguments: '{"path"' }
                }]
              },
              finish_reason: null
            }]
          }),
          sseFrame({
            choices: [{
              delta: {
                tool_calls: [{
                  index: 0,
                  function: { arguments: ':"README.md"}' }
                }]
              },
              finish_reason: 'tool_calls'
            }]
          }),
          sseFrame({
            choices: [{ delta: {}, finish_reason: 'tool_calls' }],
            usage: {
              prompt_tokens: 160,
              completion_tokens: 12,
              total_tokens: 172,
              prompt_cache_hit_tokens: 90,
              prompt_cache_miss_tokens: 70
            }
          }),
          'data: [DONE]\n\n'
        ],
        { delayMs: chunkDelayMs, burstFrames: 1 }
      )
      res.end()
      return
    }

    const answer = scenario === 'long' || scenario === 'proxy-burst'
      ? Array.from({ length: 36 }, (_, index) =>
          `长回答段落 ${index + 1}: Analytix 应持续显示 final answer，小步追加、保持底部锚点稳定，并避免把历史时间线或右侧面板整棵重渲染。`
        ).join('\n\n')
      : scenario === 'markdown'
      ? [
          '# Streaming Benchmark Result',
          '',
          '- shell appears early',
          '- reasoning updates in small frames',
          '- final answer stays deduplicated',
          '',
          '| metric | target |',
          '| --- | --- |',
          '| paint p95 | under 80ms |',
          '',
          '```ts',
          'const smooth = true',
          '```'
        ].join('\n')
      : scenario === 'reasoning'
        ? '结论：这个本地基准用于观察 reasoning 与 final answer 是否连续进入右侧时间线。'
      : scenario === 'tool'
          ? '工具基准回复：工具行应该在调用开始时快速出现，工具完成后最终回答继续流式显示。'
        : '本地基准回复：右侧应该快速出现工作状态，并连续显示这段回答。'
    await emitSseFrames(
      res,
      requestRecord,
      chunkText(answer, scenario === 'markdown' ? 10 : 5).map((part) => sseFrame({ choices: [{ delta: { content: part }, finish_reason: null }] })),
      { delayMs: chunkDelayMs, burstFrames }
    )

    await emitSseFrames(
      res,
      requestRecord,
      [
        sseFrame({
          choices: [{ delta: {}, finish_reason: 'stop' }],
          usage: {
            prompt_tokens: 120,
            completion_tokens: 40,
            total_tokens: 160,
            prompt_cache_hit_tokens: 80,
            prompt_cache_miss_tokens: 40
          }
        }),
        'data: [DONE]\n\n'
      ],
      { delayMs: chunkDelayMs, burstFrames: 1 }
    )
    res.end()
  })
  return { server, requests }
}

function runtimeLaunchCommand() {
  const explicit = argValue('--runtime-bin', process.env.ANALYTIX_STREAM_BENCH_RUNTIME_BIN || '').trim()
  if (explicit) return { command: explicit, args: [], cwd: repoRoot }
  return {
    command: 'go',
    args: ['run', '-tags', 'analytix_prod', './cmd/runtime-server'],
    cwd: join(repoRoot, 'packages/runtime-go')
  }
}

function waitForReady(child, timeoutMs) {
  return new Promise((resolve, reject) => {
    let output = ''
    const timer = setTimeout(() => reject(new Error(`timeout waiting for runtime ready: ${output.slice(-1000)}`)), timeoutMs)
    const cleanup = () => {
      clearTimeout(timer)
      child.stdout?.off('data', onStdout)
      child.stderr?.off('data', onStderr)
      child.off('exit', onExit)
    }
    const onStdout = (chunk) => {
      output += String(chunk)
      const index = output.indexOf(READY_PREFIX)
      if (index < 0) return
      const line = output.slice(index + READY_PREFIX.length).split(/\r?\n/)[0]
      cleanup()
      resolve(JSON.parse(line))
    }
    const onStderr = (chunk) => {
      output += String(chunk)
    }
    const onExit = (code, signal) => {
      cleanup()
      reject(new Error(`runtime exited before ready: ${code ?? signal}; ${output.slice(-1000)}`))
    }
    child.stdout?.on('data', onStdout)
    child.stderr?.on('data', onStderr)
    child.once('exit', onExit)
  })
}

function spawnRuntime(providerBaseUrl, tempRoot) {
  const launch = runtimeLaunchCommand()
  const dataDir = join(tempRoot, 'runtime-data')
  const modelProviders = JSON.stringify({
    defaultProviderId: LOCAL_PROVIDER_ID,
    providers: [{
      id: LOCAL_PROVIDER_ID,
      name: 'Analytix local streaming benchmark provider',
      apiKey: 'local-streaming-benchmark-credential',
      baseUrl: providerBaseUrl,
      endpointFormat: 'chat_completions',
      models: [LOCAL_MODEL],
      prices: {
        [LOCAL_MODEL]: { input: 0.14, output: 0.28, cacheHit: 0.014, currency: 'USD' }
      }
    }]
  })
  const child = spawn(
    launch.command,
    [
      ...launch.args,
      '-addr', '127.0.0.1:0',
      '-insecure',
      '-durable-root', dataDir,
      '-data-dir', dataDir,
      '-model', LOCAL_MODEL,
      '-provider-id', LOCAL_PROVIDER_ID
    ],
    {
      cwd: launch.cwd,
      env: {
        ...process.env,
        ANALYTIX_MODEL_PROVIDERS: modelProviders,
        ANALYTIX_MCP_CONFIG_JSON: JSON.stringify({ mcpServers: {} })
      },
      stdio: ['ignore', 'pipe', 'pipe'],
      detached: process.platform !== 'win32'
    }
  )
  return { child, dataDir }
}

function writeBenchViteConfig(runtimeUrl, tempRoot) {
  const configPath = join(tempRoot, 'streaming-bench-vite.config.mjs')
  const viteModuleUrl = pathToFileURL(join(repoRoot, 'node_modules/vite/dist/node/index.js')).href
  const reactPluginModuleUrl = pathToFileURL(join(repoRoot, 'node_modules/@vitejs/plugin-react/dist/index.js')).href
  writeFileSync(configPath, `
import { defineConfig } from ${JSON.stringify(viteModuleUrl)}
import react from ${JSON.stringify(reactPluginModuleUrl)}

async function readRequestBody(req) {
  const chunks = []
  for await (const chunk of req) chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(String(chunk)))
  return chunks.length ? Buffer.concat(chunks) : undefined
}

async function writeFetchResponse(res, upstream) {
  res.statusCode = upstream.status
  upstream.headers.forEach((value, key) => res.setHeader(key, value))
  if (!upstream.body) {
    res.end()
    return
  }
  const reader = upstream.body.getReader()
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      if (value) res.write(Buffer.from(value))
    }
  } finally {
    res.end()
    reader.releaseLock()
  }
}

export default defineConfig({
  root: ${JSON.stringify(join(repoRoot, 'src/renderer'))},
  resolve: {
    alias: {
      '@renderer': ${JSON.stringify(join(repoRoot, 'src/renderer/src'))},
      '@shared': ${JSON.stringify(join(repoRoot, 'src/shared'))}
    }
  },
  plugins: [
    react(),
    {
      name: 'analytix-streaming-bench-runtime-proxy',
      configureServer(server) {
        server.middlewares.use('/__analytix-runtime', (req, res) => {
          void (async () => {
            try {
              const path = req.url?.startsWith('/') ? req.url : '/' + (req.url || '')
              const method = req.method || 'GET'
              const body = method === 'GET' || method === 'HEAD' ? undefined : await readRequestBody(req)
              const upstream = await fetch(${JSON.stringify(runtimeUrl)} + path, {
                method,
                headers: req.headers,
                body
              })
              await writeFetchResponse(res, upstream)
            } catch (error) {
              res.statusCode = 502
              res.setHeader('content-type', 'application/json; charset=utf-8')
              res.end(JSON.stringify({ code: 'bench_proxy_failed', message: error instanceof Error ? error.message : String(error) }))
            }
          })()
        })
        server.middlewares.use('/__analytix-desktop/settings', (_req, res) => {
          res.statusCode = 404
          res.setHeader('content-type', 'application/json; charset=utf-8')
          res.end(JSON.stringify({ ok: false, message: 'Desktop settings not used by streaming benchmark' }))
        })
      }
    }
  ],
  server: {
    host: '127.0.0.1',
    port: 0,
    strictPort: false
  }
})
`)
  return configPath
}

function spawnRenderer(runtimeUrl, tempRoot) {
  const configPath = writeBenchViteConfig(runtimeUrl, tempRoot)
  const viteBin = join(repoRoot, 'node_modules/vite/bin/vite.js')
  const child = spawn(
    process.execPath,
    [viteBin, '--config', configPath, '--clearScreen=false'],
    {
      cwd: repoRoot,
      env: {
        ...process.env,
        ANALYTIX_BROWSER_RUNTIME_URL: runtimeUrl,
        ANALYTIX_THREAD_TRACE: '1',
        VITE_ANALYTIX_RUNTIME_PORT: '0'
      },
      stdio: ['ignore', 'pipe', 'pipe'],
      detached: process.platform !== 'win32'
    }
  )
  return child
}

function waitForRendererUrl(child, timeoutMs) {
  return new Promise((resolve, reject) => {
    let output = ''
    const timer = setTimeout(() => reject(new Error(`timeout waiting for renderer URL: ${output.slice(-1000)}`)), timeoutMs)
    const cleanup = () => {
      clearTimeout(timer)
      child.stdout?.off('data', onOutput)
      child.stderr?.off('data', onOutput)
      child.off('exit', onExit)
    }
    const onOutput = (chunk) => {
      output += String(chunk)
      const match = output.match(/Local:\s+(http:\/\/(?:localhost|127\.0\.0\.1):\d+\/)/)
      if (!match) return
      cleanup()
      resolve(match[1].replace('localhost', '127.0.0.1'))
    }
    const onExit = (code, signal) => {
      cleanup()
      reject(new Error(`renderer exited before URL: ${code ?? signal}; ${output.slice(-1000)}`))
    }
    child.stdout?.on('data', onOutput)
    child.stderr?.on('data', onOutput)
    child.once('exit', onExit)
  })
}

async function waitForHttpOk(url, timeoutMs) {
  const started = Date.now()
  while (Date.now() - started <= timeoutMs) {
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(2_000) })
      if (response.ok) return
    } catch {
      // Vite has printed the URL before the socket is fully accepting.
    }
    await sleep(120)
  }
  throw new Error(`timeout waiting for renderer HTTP readiness: ${url}`)
}

function chromeExecutable() {
  const explicit = argValue('--chrome', process.env.CHROME_BIN || '').trim()
  if (explicit) return explicit
  const candidates = [
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/Applications/Chromium.app/Contents/MacOS/Chromium',
    '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge'
  ]
  return candidates.find((candidate) => existsSync(candidate)) || ''
}

async function fetchJSON(url, timeoutMs = 10_000) {
  const response = await fetch(url, { signal: AbortSignal.timeout(timeoutMs) })
  if (!response.ok) throw new Error(`fetch ${url} failed: ${response.status}`)
  return response.json()
}

async function waitForChromePage(debugPort, timeoutMs) {
  const started = Date.now()
  while (Date.now() - started <= timeoutMs) {
    try {
      const pages = await fetchJSON(`http://127.0.0.1:${debugPort}/json/list`, 2_000)
      const page = pages.find((item) => item.type === 'page' && item.webSocketDebuggerUrl)
      if (page) return page.webSocketDebuggerUrl
    } catch {
      // Chrome may still be starting.
    }
    await sleep(120)
  }
  throw new Error('timeout waiting for Chrome DevTools page')
}

class CDPClient {
  constructor(url) {
    this.socket = new WebSocket(url)
    this.nextId = 1
    this.pending = new Map()
    this.events = new Map()
  }

  async open() {
    await new Promise((resolve, reject) => {
      this.socket.addEventListener('open', resolve, { once: true })
      this.socket.addEventListener('error', reject, { once: true })
    })
    this.socket.addEventListener('message', (message) => this.onMessage(message))
  }

  onMessage(message) {
    const payload = JSON.parse(String(message.data))
    if (payload.id) {
      const pending = this.pending.get(payload.id)
      if (!pending) return
      this.pending.delete(payload.id)
      if (payload.error) pending.reject(new Error(payload.error.message || JSON.stringify(payload.error)))
      else pending.resolve(payload.result)
      return
    }
    const handlers = this.events.get(payload.method)
    if (handlers) handlers.forEach((handler) => handler(payload.params))
  }

  send(method, params = {}) {
    const id = this.nextId++
    this.socket.send(JSON.stringify({ id, method, params }))
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject })
    })
  }

  on(method, handler) {
    const handlers = this.events.get(method) || new Set()
    handlers.add(handler)
    this.events.set(method, handlers)
  }

  async close() {
    if (this.socket.readyState === WebSocket.CLOSED) return
    await Promise.race([
      new Promise((resolve) => {
        this.socket.addEventListener('close', resolve, { once: true })
        this.socket.close()
      }),
      sleep(500)
    ])
  }
}

function browserSettingsSeed(providerBaseUrl) {
  return JSON.stringify({
    version: 1,
    provider: {
      activeProviderId: LOCAL_PROVIDER_ID,
      providers: [{
        id: LOCAL_PROVIDER_ID,
        name: 'Local Streaming Benchmark',
        apiKey: '',
        baseUrl: providerBaseUrl,
        endpointFormat: 'chat_completions',
        models: [LOCAL_MODEL],
        modelProfiles: {}
      }]
    },
    runtime: {
      providerId: LOCAL_PROVIDER_ID,
      model: LOCAL_MODEL
    }
  })
}

function initScript(providerBaseUrl) {
  return `
(() => {
  localStorage.setItem('analytix.browserPreview.account', '1');
  localStorage.setItem('ANALYTIX_THREAD_TRACE', '1');
  localStorage.setItem('analytix.browserPreview.settings', ${JSON.stringify(browserSettingsSeed(providerBaseUrl))});
  localStorage.setItem('analytix.composerModel', ${JSON.stringify(LOCAL_MODEL)});
  localStorage.setItem('analytix.composerProviderId', ${JSON.stringify(LOCAL_PROVIDER_ID)});
  const bench = window.__analytixBench = {
    sendAt: 0,
    firstShellAt: 0,
    firstAssistantAt: 0,
    firstProcessAt: 0,
    firstToolAt: 0,
    assistantPaints: [],
    processPaints: [],
    toolPaints: [],
    bottomDistances: [],
    responseSpacerSamples: [],
    responseSpacerTransitions: [],
    postCompletionBottomDistances: [],
    longTasks: [],
    fetches: [],
    settingsSnapshot: null,
    errorTextChars: 0,
    samples: []
  };
  if (${JSON.stringify(includeDiagnostics)}) {
    const originalFetch = window.fetch.bind(window);
    window.fetch = async (input, init) => {
      const startedAt = performance.now();
      const url = typeof input === 'string' ? input : input?.url || '';
      const method = init?.method || (typeof input === 'object' && input?.method) || 'GET';
      try {
        const response = await originalFetch(input, init);
        if (String(url).includes('/__analytix-runtime') || String(url).includes('/__analytix-desktop')) {
          bench.fetches.push({
            at: Math.round(startedAt),
            ms: Math.round(performance.now() - startedAt),
            method,
            channel: String(url).includes('/__analytix-runtime') ? 'runtime' : 'desktop',
            status: response.status,
            ok: response.ok
          });
        }
        return response;
      } catch (error) {
        if (String(url).includes('/__analytix-runtime') || String(url).includes('/__analytix-desktop')) {
          bench.fetches.push({
            at: Math.round(startedAt),
            ms: Math.round(performance.now() - startedAt),
            method,
            channel: String(url).includes('/__analytix-runtime') ? 'runtime' : 'desktop',
            status: 0,
            ok: false,
            errorType: error instanceof Error ? error.name : 'unknown'
          });
        }
        throw error;
      }
    };
  }
  try {
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        bench.longTasks.push({ at: entry.startTime, duration: entry.duration });
      }
    }).observe({ type: 'longtask', buffered: true });
  } catch {}
  let installed = false;
  let lastAssistantLength = 0;
  let lastProcessLength = 0;
  let lastToolLength = 0;
  let lastSpacerState = '';
  function sample() {
    const now = performance.now();
    const scroller = document.querySelector('[data-analytix-message-timeline-scroller]');
    const spacer = document.querySelector('[data-analytix-response-spacer]');
    const shell = document.querySelector('[data-analytix-live-process], [data-analytix-live-progress], [data-analytix-live-assistant]');
    const assistant = document.querySelector('[data-analytix-live-assistant]');
    const process = document.querySelector('[data-analytix-live-process]');
    const tool = document.querySelector('[data-analytix-process-block-kind="tool"]');
    const spacerState = spacer?.getAttribute('data-analytix-response-spacer') || '';
    const spacerHeight = spacer ? spacer.getBoundingClientRect().height : 0;
    const assistantLength = assistant?.textContent?.length || 0;
    const processLength = process?.textContent?.length || 0;
    const toolLength = tool?.textContent?.length || 0;
    const bottomDistance = scroller
      ? Math.max(0, scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight)
      : 0;
    if (bench.sendAt && shell && !bench.firstShellAt) bench.firstShellAt = now;
    if (bench.sendAt && assistantLength > 0 && !bench.firstAssistantAt) bench.firstAssistantAt = now;
    if (bench.sendAt && processLength > 0 && !bench.firstProcessAt) bench.firstProcessAt = now;
    if (bench.sendAt && tool && !bench.firstToolAt) bench.firstToolAt = now;
    if (bench.sendAt && assistantLength !== lastAssistantLength) {
      bench.assistantPaints.push({ at: now, chars: assistantLength, deltaChars: assistantLength - lastAssistantLength });
      lastAssistantLength = assistantLength;
    }
    if (bench.sendAt && processLength !== lastProcessLength) {
      bench.processPaints.push({ at: now, chars: processLength, deltaChars: processLength - lastProcessLength });
      lastProcessLength = processLength;
    }
    if (bench.sendAt && toolLength !== lastToolLength) {
      bench.toolPaints.push({ at: now, chars: toolLength, deltaChars: toolLength - lastToolLength });
      lastToolLength = toolLength;
    }
    if (scroller) {
      bench.bottomDistances.push({
        at: now,
        value: bottomDistance
      });
    }
    if (spacer) {
      bench.responseSpacerSamples.push({
        at: now,
        state: spacerState,
        height: Math.round(spacerHeight),
        bottomDistance: Math.round(bottomDistance)
      });
      if (lastSpacerState === 'live' && spacerState === 'idle') {
        bench.responseSpacerTransitions.push({
          at: now,
          from: 'live',
          to: 'idle',
          bottomDistance: Math.round(bottomDistance)
        });
      }
      const latestTransition = bench.responseSpacerTransitions[bench.responseSpacerTransitions.length - 1];
      if (latestTransition && now - latestTransition.at <= 1000) {
        bench.postCompletionBottomDistances.push({
          at: now,
          value: Math.round(bottomDistance),
          delta: Math.round(Math.abs(bottomDistance - latestTransition.bottomDistance))
        });
      }
      lastSpacerState = spacerState;
    }
    bench.samples.push({ at: now, assistantLength, processLength, toolLength, shell: Boolean(shell) });
  }
  function scheduleSample() {
    requestAnimationFrame(sample);
  }
  function install() {
    if (installed || !document.body) return;
    installed = true;
    new MutationObserver(scheduleSample).observe(document.body, {
      subtree: true,
      childList: true,
      characterData: true,
      attributes: true,
      attributeFilter: [
        'data-analytix-live-process',
        'data-analytix-live-assistant',
        'data-analytix-process-block-kind',
        'data-analytix-process-block-status',
        'data-analytix-response-spacer'
      ]
    });
    window.setInterval(sample, 50);
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', install, { once: true });
  } else {
    install();
  }
})();
`
}

const COMPOSER_HELPERS = `
function analytixBenchVisible(el) {
  if (!el || el.disabled) return false;
  const rect = el.getBoundingClientRect();
  const style = window.getComputedStyle(el);
  return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none';
}
function analytixBenchFindComposer() {
  const textarea = [...document.querySelectorAll('textarea')]
    .find((el) => analytixBenchVisible(el) && !el.readOnly);
  if (textarea) return { kind: 'textarea', el: textarea };
  const editable = [...document.querySelectorAll('[contenteditable="true"], [role="textbox"][contenteditable]')]
    .find((el) => analytixBenchVisible(el) && !el.closest('[aria-hidden="true"]'));
  if (editable) return { kind: 'contenteditable', el: editable };
  return null;
}
function analytixBenchFocusComposer() {
  const target = analytixBenchFindComposer();
  if (!target) return false;
  target.el.focus();
  return target.kind;
}
`

async function cdpEvaluate(client, expression, timeoutMs = 10_000) {
  const result = await client.send('Runtime.evaluate', {
    expression,
    awaitPromise: true,
    returnByValue: true,
    timeout: timeoutMs
  })
  if (result.exceptionDetails) throw new Error(result.exceptionDetails.text || 'Runtime.evaluate failed')
  return result.result?.value
}

async function waitForEvaluate(client, expression, timeoutMs, label) {
  const started = Date.now()
  while (Date.now() - started <= timeoutMs) {
    const value = await cdpEvaluate(client, expression).catch(() => false)
    if (value) return value
    await sleep(120)
  }
  throw new Error(`timeout waiting for ${label}`)
}

async function runBrowserScenario({ rendererUrl, providerBaseUrl, prompt, timeoutMs }) {
  const chrome = chromeExecutable()
  if (!chrome) throw new Error('Chrome executable not found. Set CHROME_BIN or --chrome.')
  const debugPort = await getFreePort()
  const userDataDir = mkdtempSync(join(tmpdir(), 'analytix-streaming-chrome-'))
  const child = spawn(chrome, [
    `--remote-debugging-port=${debugPort}`,
    `--user-data-dir=${userDataDir}`,
    '--no-first-run',
    '--disable-background-networking',
    '--disable-extensions',
    '--disable-sync',
    '--disable-default-apps',
    'about:blank'
  ], { stdio: ['ignore', 'ignore', 'ignore'], detached: process.platform !== 'win32' })
  let client
  const browserEvents = []
  try {
    const wsUrl = await waitForChromePage(debugPort, timeoutMs)
    client = new CDPClient(wsUrl)
    await client.open()
    await client.send('Page.enable')
    await client.send('Runtime.enable')
    client.on('Runtime.exceptionThrown', (params) => {
      browserEvents.push({ type: 'exception', textChars: String(params?.exceptionDetails?.text || '').length })
    })
    client.on('Runtime.consoleAPICalled', (params) => {
      const consoleText = (params?.args || []).map((arg) => arg.value || arg.description || '').join(' ')
      browserEvents.push({
        type: 'console',
        textChars: consoleText.length
      })
    })
    await client.send('Page.addScriptToEvaluateOnNewDocument', { source: initScript(providerBaseUrl) })
    await client.send('Page.navigate', { url: `${rendererUrl}?accountPreview=1` })
    try {
      await waitForEvaluate(client, `(() => { ${COMPOSER_HELPERS}; return Boolean(analytixBenchFindComposer()); })()`, timeoutMs, 'composer')
    } catch (error) {
      const snapshot = await cdpEvaluate(client, `
(() => ({
  readyState: document.readyState,
  textareaCount: document.querySelectorAll('textarea').length,
  contentEditableCount: document.querySelectorAll('[contenteditable="true"], [role="textbox"][contenteditable]').length,
  buttonCount: document.querySelectorAll('button').length,
  bodyTextChars: document.body?.innerText?.length || 0,
  browserEvents: ${JSON.stringify(browserEvents)}
}))()
`, 5_000).catch((snapshotError) => ({
        snapshotErrorType: snapshotError instanceof Error ? snapshotError.name : 'unknown',
        browserEvents
      }))
      throw new Error(`${error instanceof Error ? error.message : String(error)}; page snapshot: ${JSON.stringify(snapshot)}`)
    }
    const focusedComposerKind = await cdpEvaluate(client, `
(() => {
  ${COMPOSER_HELPERS}
  return analytixBenchFocusComposer();
})()
`)
    if (!focusedComposerKind) throw new Error('composer focus failed')
    if (includeDiagnostics) {
      const settingsSnapshot = await cdpEvaluate(client, `
window.analytix?.settings?.getSettings?.().then((settings) => ({
  runtime: {
    providerId: settings?.runtime?.providerId,
    model: settings?.runtime?.model
  },
  provider: {
    activeProviderId: settings?.provider?.activeProviderId,
    providers: (settings?.provider?.providers || []).map((provider) => ({
      id: provider.id,
      endpointFormat: provider.endpointFormat,
      modelCount: Array.isArray(provider.models) ? provider.models.length : 0,
      hasApiKey: Boolean(String(provider.apiKey || '').trim())
    }))
  },
  workspaceConfigured: Boolean(String(settings?.workspaceRoot || '').trim())
}))
`, 5_000).catch((error) => ({ errorType: error instanceof Error ? error.name : 'unknown' }))
      await cdpEvaluate(client, `
(() => {
  window.__analytixBench.settingsSnapshot = ${JSON.stringify(settingsSnapshot)};
})()
`)
    }
    await client.send('Input.insertText', { text: prompt })
    await waitForEvaluate(client, `
(() => {
  const button = document.querySelector('button.ds-composer-primary-action-button');
  return Boolean(button && !button.disabled);
})()
`, timeoutMs, 'enabled send button')
    await cdpEvaluate(client, `
(() => {
  window.__analytixBench.sendAt = performance.now();
  const button = document.querySelector('button.ds-composer-primary-action-button');
  button?.click();
  return Boolean(button);
})()
`)
    await waitForEvaluate(client, 'window.__analytixBench.firstShellAt > 0', timeoutMs, 'first shell paint')
    await sleep(Number(argValue('--settle-ms', '5000')))
    const bench = await cdpEvaluate(client, includeDiagnostics ? `
(() => {
  window.__analytixBench.errorTextChars = document.body?.innerText?.length || 0;
  return window.__analytixBench;
})()
` : 'window.__analytixBench')
    if (includeDiagnostics) bench.browserEvents = browserEvents
    return bench
  } finally {
    await client?.close()
    await stopChild(child)
    rmSync(userDataDir, { recursive: true, force: true })
  }
}

function signalChild(child, signal) {
  if (!child?.pid) return
  try {
    if (process.platform !== 'win32') process.kill(-child.pid, signal)
    else child.kill(signal)
  } catch {
    try {
      child.kill(signal)
    } catch {
      // Already gone.
    }
  }
}

async function stopChild(child) {
  if (!child || child.exitCode !== null || child.signalCode !== null) return
  signalChild(child, 'SIGTERM')
  await Promise.race([
    new Promise((resolve) => child.once('exit', resolve)),
    sleep(1500)
  ])
  if (child.exitCode === null && child.signalCode === null) {
    signalChild(child, 'SIGKILL')
    await Promise.race([
      new Promise((resolve) => child.once('exit', resolve)),
      sleep(1500)
    ])
  }
}

async function closeServer(server) {
  const timer = setTimeout(() => {
    server.closeIdleConnections?.()
    server.closeAllConnections?.()
  }, 500)
  await new Promise((resolve) => server.close(() => resolve()))
  clearTimeout(timer)
}

function intervals(events) {
  const out = []
  for (let index = 1; index < events.length; index += 1) out.push(events[index].at - events[index - 1].at)
  return out
}

function percentile(values, p) {
  const clean = values.filter((value) => Number.isFinite(value)).sort((a, b) => a - b)
  if (clean.length === 0) return null
  const index = Math.min(clean.length - 1, Math.max(0, Math.ceil((p / 100) * clean.length) - 1))
  return Math.round(clean[index])
}

function summarizeScenario(label, bench) {
  const sendAt = Number(bench.sendAt || 0)
  const assistantIntervals = intervals(bench.assistantPaints || [])
  const processIntervals = intervals(bench.processPaints || [])
  const toolIntervals = intervals(bench.toolPaints || [])
  const spacerSamples = bench.responseSpacerSamples || []
  const liveSpacerHeights = spacerSamples
    .filter((item) => item.state === 'live')
    .map((item) => Number(item.height || 0))
  const idleSpacerHeights = spacerSamples
    .filter((item) => item.state === 'idle')
    .map((item) => Number(item.height || 0))
  const postCompletionDeltas = (bench.postCompletionBottomDistances || [])
    .map((item) => Number(item.delta || 0))
  return {
    label,
    send_click_to_shell_visible_ms: bench.firstShellAt ? Math.round(bench.firstShellAt - sendAt) : null,
    send_click_to_first_assistant_paint_ms: bench.firstAssistantAt ? Math.round(bench.firstAssistantAt - sendAt) : null,
    send_click_to_first_process_paint_ms: bench.firstProcessAt ? Math.round(bench.firstProcessAt - sendAt) : null,
    tool_start_visible_ms: bench.firstToolAt ? Math.round(bench.firstToolAt - sendAt) : null,
    assistant_paint_interval_p50: percentile(assistantIntervals, 50),
    assistant_paint_interval_p95: percentile(assistantIntervals, 95),
    process_paint_interval_p50: percentile(processIntervals, 50),
    process_paint_interval_p95: percentile(processIntervals, 95),
    tool_paint_interval_p50: percentile(toolIntervals, 50),
    tool_paint_interval_p95: percentile(toolIntervals, 95),
    assistant_paint_count: bench.assistantPaints?.length || 0,
    process_paint_count: bench.processPaints?.length || 0,
    tool_paint_count: bench.toolPaints?.length || 0,
    assistant_max_chars_per_paint: Math.max(0, ...(bench.assistantPaints || []).map((item) => Math.abs(item.deltaChars || 0))),
    process_max_chars_per_paint: Math.max(0, ...(bench.processPaints || []).map((item) => Math.abs(item.deltaChars || 0))),
    tool_max_chars_per_paint: Math.max(0, ...(bench.toolPaints || []).map((item) => Math.abs(item.deltaChars || 0))),
    response_spacer_transition_count: bench.responseSpacerTransitions?.length || 0,
    response_spacer_live_height_px: Math.round(Math.max(0, ...liveSpacerHeights)),
    response_spacer_idle_height_px: Math.round(Math.max(0, ...idleSpacerHeights)),
    max_post_completion_bottom_delta_px: Math.round(Math.max(0, ...postCompletionDeltas)),
    long_task_count: bench.longTasks?.length || 0,
    long_task_p95_ms: percentile((bench.longTasks || []).map((item) => item.duration), 95),
    max_bottom_distance_px: Math.round(Math.max(0, ...(bench.bottomDistances || []).map((item) => Number(item.value || 0))))
  }
}

function gateMetric(failures, scenario, metric, value, max, options = {}) {
  if (value === null || value === undefined) {
    if (options.required) failures.push(`${scenario.label}.${metric} missing`)
    return
  }
  if (Number(value) > max) failures.push(`${scenario.label}.${metric}=${value} > ${max}`)
}

function validateReport(report) {
  const failures = []
  for (const scenario of report.scenarios || []) {
    gateMetric(failures, scenario, 'send_click_to_shell_visible_ms', scenario.send_click_to_shell_visible_ms, 160, { required: true })
    gateMetric(failures, scenario, 'assistant_paint_interval_p95', scenario.assistant_paint_interval_p95, 120)
    gateMetric(failures, scenario, 'process_paint_interval_p95', scenario.process_paint_interval_p95, 360)
    gateMetric(failures, scenario, 'assistant_max_chars_per_paint', scenario.assistant_max_chars_per_paint, 640)
    gateMetric(failures, scenario, 'process_max_chars_per_paint', scenario.process_max_chars_per_paint, 320)
    gateMetric(failures, scenario, 'long_task_p95_ms', scenario.long_task_p95_ms, 140)
    gateMetric(failures, scenario, 'max_bottom_distance_px', scenario.max_bottom_distance_px, 140)
    if (scenario.label === 'markdown') {
      gateMetric(failures, scenario, 'max_post_completion_bottom_delta_px', scenario.max_post_completion_bottom_delta_px, 32)
    }
    if (scenario.label !== 'proxy-burst') {
      gateMetric(failures, scenario, 'provider.write_interval_p95_ms', scenario.provider?.write_interval_p95_ms, 120)
      gateMetric(failures, scenario, 'provider.max_frames_per_write', scenario.provider?.max_frames_per_write, 2)
    }
    if (scenario.label === 'tool') {
      gateMetric(failures, scenario, 'tool_start_visible_ms', scenario.tool_start_visible_ms, 900, { required: true })
    }
  }
  return {
    passed: failures.length === 0,
    failures
  }
}

function summarizeBenchDiagnostics(bench) {
  return {
    settings: bench.settingsSnapshot ?? null,
    fetches: (bench.fetches || []).slice(-30),
    browserEvents: (bench.browserEvents || []).slice(-30),
    errorTextChars: Number(bench.errorTextChars || 0)
  }
}

async function main() {
  if (args.has('--include-raw') || args.has('--include-raw-provider')) {
    throw new Error('raw browser/provider export is forbidden; use bounded --include-diagnostics metadata')
  }
  if (args.has('--real')) {
    throw new Error('real API cross-app mode is intentionally explicit but not run by this local fake-provider harness')
  }
  const timeoutMs = Number(argValue('--timeout-ms', String(DEFAULT_TIMEOUT_MS)))
  const runId = argValue('--run-id', new Date().toISOString().replace(/[:.]/g, '-'))
  const outDir = resolve(repoRoot, argValue('--out', `.bench/streaming/${runId}`))
  const tempRoot = mkdtempSync(join(tmpdir(), 'analytix-streaming-ui-'))
  const provider = startFakeProvider({
    firstDelayMs: Number(argValue('--provider-first-delay-ms', '60')),
    chunkDelayMs: Number(argValue('--provider-chunk-delay-ms', '24'))
  })
  let runtime = null
  let renderer = null
  try {
    mkdirSync(outDir, { recursive: true })
    const providerBaseUrl = await listen(provider.server)
    const spawnedRuntime = spawnRuntime(providerBaseUrl, tempRoot)
    runtime = spawnedRuntime.child
    const ready = await waitForReady(runtime, timeoutMs)
    renderer = spawnRenderer(ready.url, tempRoot)
    const rendererUrl = await waitForRendererUrl(renderer, timeoutMs)
    await waitForHttpOk(rendererUrl, timeoutMs)
    const scenarios = [
      { label: 'short', prompt: '[short] 用一句话说明流式输出是否连续。' },
      { label: 'reasoning', prompt: '[reasoning] 请先思考再给出一句结论，用于本地流式基准。' },
      { label: 'long', prompt: '[long] 输出长 reasoning 和长 final answer，用于验证长流式体感。' },
      { label: 'tool', prompt: '[tool] 请读取 README.md，然后用一句话说明工具调用是否连续显示。' },
      { label: 'markdown', prompt: '[markdown] 输出一个短 markdown 报告，包含表格和代码块。' }
    ]
    if (args.has('--include-proxy-burst')) {
      scenarios.push({
        label: 'proxy-burst',
        prompt: '[proxy-burst] 模拟中转 API 攒包后再吐出长 reasoning 和长 final answer。'
      })
    }
    const results = []
    for (const scenario of scenarios) {
      const requestStartIndex = provider.requests.length
      const bench = await runBrowserScenario({ rendererUrl, providerBaseUrl, prompt: scenario.prompt, timeoutMs })
      const providerRequests = provider.requests.slice(requestStartIndex)
      results.push({
        ...summarizeScenario(scenario.label, bench),
        provider: providerWriteSummary(providerRequests),
        diagnostics: includeDiagnostics ? summarizeBenchDiagnostics(bench) : undefined
      })
    }
    const report = {
      schemaVersion: 1,
      id: 'analytix-streaming-ui-benchmark',
      mode: 'fake-provider-ui',
      generatedAt: new Date().toISOString(),
      provider: {
        requestCount: provider.requests.length,
        requests: provider.requests.map((request) => ({
          scenario: request.scenario,
          model: request.model,
          stream: request.stream,
          messageCount: request.messageCount,
          toolCount: request.toolCount,
          hasToolResult: request.hasToolResult,
          bodyHash: request.bodyHash,
          summary: providerWriteSummary([request])
        })),
        summary: providerWriteSummary(provider.requests)
      },
      runtime: {
        urlHash: sha256(ready.url),
        providerId: LOCAL_PROVIDER_ID,
        model: LOCAL_MODEL
      },
      renderer: {
        urlHash: sha256(rendererUrl)
      },
      scenarios: results
    }
    report.gate = validateReport(report)
    writeFileSync(join(outDir, 'analytix-fake-ui-summary.json'), JSON.stringify(report, null, 2))
    console.log(JSON.stringify(report, null, 2))
    if (args.has('--gate') && !report.gate.passed) {
      throw new Error(`streaming UI gate failed: ${report.gate.failures.join('; ')}`)
    }
  } finally {
    await stopChild(renderer)
    await stopChild(runtime)
    await closeServer(provider.server)
    rmSync(tempRoot, { recursive: true, force: true })
  }
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error))
  process.exit(1)
})
