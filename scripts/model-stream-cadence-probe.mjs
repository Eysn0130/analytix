#!/usr/bin/env node

import { createHash } from 'node:crypto'
import { writeFileSync } from 'node:fs'
import { performance } from 'node:perf_hooks'

const rawArgs = process.argv.slice(2)

function argValue(name, fallback = '') {
  const inline = rawArgs.find((item) => item.startsWith(`${name}=`))
  if (inline) return inline.slice(name.length + 1)
  const index = rawArgs.indexOf(name)
  return index >= 0 ? rawArgs[index + 1] || '' : fallback
}

function sha256(value) {
  return createHash('sha256').update(String(value)).digest('hex')
}

function trimTrailingSlash(value) {
  return String(value || '').replace(/\/+$/, '')
}

function endpointPath(format) {
  switch (format) {
    case 'responses':
      return '/v1/responses'
    case 'messages':
      return '/v1/messages'
    case 'chat_completions':
    case 'chat-completions':
    default:
      return '/v1/chat/completions'
  }
}

function resolveEndpointUrl() {
  const full = argValue('--url', process.env.ANALYTIX_STREAM_PROBE_URL || '').trim()
  if (full) return full
  const baseUrl = argValue('--base-url', process.env.ANALYTIX_STREAM_PROBE_BASE_URL || '').trim()
  if (!baseUrl) throw new Error('missing --base-url or ANALYTIX_STREAM_PROBE_BASE_URL')
  return `${trimTrailingSlash(baseUrl)}${endpointPath(endpointFormat())}`
}

function endpointFormat() {
  return argValue('--endpoint-format', process.env.ANALYTIX_STREAM_PROBE_ENDPOINT_FORMAT || 'chat_completions').trim()
}

function sanitizedUrl(value) {
  try {
    const url = new URL(value)
    url.username = ''
    url.password = ''
    url.search = ''
    return url.toString()
  } catch {
    return '<invalid-url>'
  }
}

function requestBody(format, model, prompt) {
  if (format === 'responses') {
    return {
      model,
      stream: true,
      input: prompt
    }
  }
  if (format === 'messages') {
    return {
      model,
      stream: true,
      max_tokens: Number(argValue('--max-tokens', '512')),
      messages: [{ role: 'user', content: prompt }]
    }
  }
  return {
    model,
    stream: true,
    stream_options: { include_usage: true },
    messages: [{ role: 'user', content: prompt }]
  }
}

function requestHeaders(format, apiKey) {
  const headers = {
    'content-type': 'application/json',
    'cache-control': 'no-cache'
  }
  if (!apiKey) return headers
  if (format === 'messages') {
    headers['x-api-key'] = apiKey
    headers['anthropic-version'] = argValue('--anthropic-version', '2023-06-01')
  } else {
    headers.authorization = `Bearer ${apiKey}`
  }
  return headers
}

function percentile(values, p) {
  const clean = values.filter((value) => Number.isFinite(value)).sort((a, b) => a - b)
  if (clean.length === 0) return null
  const index = Math.min(clean.length - 1, Math.max(0, Math.ceil((p / 100) * clean.length) - 1))
  return Math.round(clean[index])
}

function intervals(events) {
  const out = []
  for (let index = 1; index < events.length; index += 1) {
    out.push(events[index].relativeMs - events[index - 1].relativeMs)
  }
  return out
}

function extractDeltaLengths(payload) {
  let contentChars = 0
  let reasoningChars = 0
  const choices = Array.isArray(payload?.choices) ? payload.choices : []
  for (const choice of choices) {
    const delta = choice?.delta
    if (typeof delta?.content === 'string') contentChars += delta.content.length
    if (typeof delta?.reasoning_content === 'string') reasoningChars += delta.reasoning_content.length
    if (typeof delta?.reasoning === 'string') reasoningChars += delta.reasoning.length
  }
  if (typeof payload?.delta === 'string' && typeof payload?.type === 'string') {
    if (payload.type.includes('reasoning')) reasoningChars += payload.delta.length
    else contentChars += payload.delta.length
  }
  if (typeof payload?.content_block?.text === 'string') contentChars += payload.content_block.text.length
  const anthropicDelta = payload?.delta
  if (typeof anthropicDelta?.text === 'string') contentChars += anthropicDelta.text.length
  if (typeof anthropicDelta?.thinking === 'string') reasoningChars += anthropicDelta.thinking.length
  return { contentChars, reasoningChars }
}

function takeSseFrame(buffer) {
  const lf = buffer.indexOf('\n\n')
  const crlf = buffer.indexOf('\r\n\r\n')
  if (lf === -1 && crlf === -1) return null
  if (crlf !== -1 && (lf === -1 || crlf < lf)) {
    return {
      frame: buffer.slice(0, crlf),
      rest: buffer.slice(crlf + 4)
    }
  }
  return {
    frame: buffer.slice(0, lf),
    rest: buffer.slice(lf + 2)
  }
}

function parseSseFrames(buffer, onFrame) {
  let remaining = buffer
  let next = takeSseFrame(remaining)
  while (next) {
    const frame = next.frame
    remaining = next.rest
    const data = frame
      .split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice(5).trim())
      .join('')
    if (data) onFrame(data)
    next = takeSseFrame(remaining)
  }
  return remaining
}

function classify(summary) {
  if (summary.max_sse_frames_per_raw_read > 8 || summary.max_bytes_per_raw_read > 4096) {
    return 'provider_or_proxy_batched'
  }
  if (Number(summary.raw_read_interval_p95_ms || 0) > 300 || Number(summary.sse_frame_interval_p95_ms || 0) > 300) {
    return 'provider_or_proxy_slow_cadence'
  }
  return 'provider_incremental'
}

async function main() {
  if (rawArgs.includes('--include-raw')) {
    throw new Error('--include-raw is forbidden; cadence diagnostics contain only bounded counters and timings')
  }
  const format = endpointFormat()
  const url = resolveEndpointUrl()
  const apiKey = argValue('--api-key', process.env.ANALYTIX_STREAM_PROBE_API_KEY || '').trim()
  const model = argValue('--model', process.env.ANALYTIX_STREAM_PROBE_MODEL || '').trim()
  if (!model) throw new Error('missing --model or ANALYTIX_STREAM_PROBE_MODEL')
  const prompt = argValue(
    '--prompt',
    'Stream a short reasoning trace if supported, then answer in 20 short numbered sentences.'
  )
  const timeoutMs = Number(argValue('--timeout-ms', process.env.ANALYTIX_STREAM_PROBE_TIMEOUT_MS || '60000'))
  const started = performance.now()
  const body = requestBody(format, model, prompt)
  const response = await fetch(url, {
    method: 'POST',
    headers: requestHeaders(format, apiKey),
    body: JSON.stringify(body),
    signal: AbortSignal.timeout(timeoutMs)
  })
  const headersAt = performance.now()
  const rawReads = []
  const sseFrames = []
  let contentChars = 0
  let reasoningChars = 0
  let done = false
  let firstTextDeltaMs = null
  let firstContentDeltaMs = null
  let firstReasoningDeltaMs = null
  let buffer = ''
  const decoder = new TextDecoder('utf-8')
  if (!response.body) throw new Error(`response has no body: ${response.status}`)
  const reader = response.body.getReader()
  try {
    while (true) {
      const read = await reader.read()
      if (read.done) break
      const relativeMs = Math.round(performance.now() - started)
      const beforeFrames = sseFrames.length
      const text = decoder.decode(read.value, { stream: true })
      buffer += text
      buffer = parseSseFrames(buffer, (data) => {
        const frame = { relativeMs: Math.round(performance.now() - started), bytes: Buffer.byteLength(data) }
        if (data === '[DONE]') {
          done = true
          sseFrames.push({ ...frame, done: true })
          return
        }
        try {
          const payload = JSON.parse(data)
          const lengths = extractDeltaLengths(payload)
          contentChars += lengths.contentChars
          reasoningChars += lengths.reasoningChars
          if (firstTextDeltaMs === null && (lengths.contentChars > 0 || lengths.reasoningChars > 0)) {
            firstTextDeltaMs = frame.relativeMs
          }
          if (firstContentDeltaMs === null && lengths.contentChars > 0) {
            firstContentDeltaMs = frame.relativeMs
          }
          if (firstReasoningDeltaMs === null && lengths.reasoningChars > 0) {
            firstReasoningDeltaMs = frame.relativeMs
          }
          sseFrames.push({ ...frame, ...lengths })
        } catch {
          sseFrames.push({ ...frame, parseError: true })
        }
      })
      rawReads.push({
        relativeMs,
        bytes: read.value.byteLength,
        sseFrames: sseFrames.length - beforeFrames
      })
    }
  } finally {
    reader.releaseLock()
  }
  const rawReadIntervals = intervals(rawReads)
  const sseFrameIntervals = intervals(sseFrames)
  const summary = {
    schemaVersion: 1,
    id: 'analytix-model-stream-cadence-probe',
    generatedAt: new Date().toISOString(),
    endpoint: {
      url: sanitizedUrl(url),
      urlHash: sha256(sanitizedUrl(url)),
      endpointFormat: format,
      model
    },
    response: {
      status: response.status,
      ok: response.ok,
      contentType: response.headers.get('content-type') || ''
    },
    timing: {
      time_to_headers_ms: Math.round(headersAt - started),
      time_to_first_raw_read_ms: rawReads[0]?.relativeMs ?? null,
      time_to_first_sse_frame_ms: sseFrames[0]?.relativeMs ?? null,
      time_to_first_text_delta_ms: firstTextDeltaMs,
      time_to_first_content_delta_ms: firstContentDeltaMs,
      time_to_first_reasoning_delta_ms: firstReasoningDeltaMs
    },
    cadence: {
      raw_read_count: rawReads.length,
      sse_frame_count: sseFrames.length,
      raw_read_interval_p50_ms: percentile(rawReadIntervals, 50),
      raw_read_interval_p95_ms: percentile(rawReadIntervals, 95),
      sse_frame_interval_p50_ms: percentile(sseFrameIntervals, 50),
      sse_frame_interval_p95_ms: percentile(sseFrameIntervals, 95),
      max_bytes_per_raw_read: Math.max(0, ...rawReads.map((item) => item.bytes)),
      max_sse_frames_per_raw_read: Math.max(0, ...rawReads.map((item) => item.sseFrames)),
      content_chars: contentChars,
      reasoning_chars: reasoningChars,
      done
    }
  }
  summary.cadence.classification = classify({
    max_sse_frames_per_raw_read: summary.cadence.max_sse_frames_per_raw_read,
    max_bytes_per_raw_read: summary.cadence.max_bytes_per_raw_read,
    raw_read_interval_p95_ms: summary.cadence.raw_read_interval_p95_ms,
    sse_frame_interval_p95_ms: summary.cadence.sse_frame_interval_p95_ms
  })
  const output = JSON.stringify(summary, null, 2)
  const outPath = argValue('--out', '').trim()
  if (outPath) writeFileSync(outPath, output)
  console.log(output)
  if (!response.ok) process.exitCode = 2
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error))
  process.exit(1)
})
