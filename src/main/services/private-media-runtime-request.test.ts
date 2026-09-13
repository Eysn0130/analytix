import { createServer } from 'node:http'
import type { AddressInfo } from 'node:net'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AppSettingsV1 } from '../../shared/app-settings'
import { createPrivateMediaRuntimeRequest } from './private-media-runtime-request'

const intent = JSON.stringify({ schemaVersion: 1, operation: 'image.generate', prompt: 'synthetic diagram', timeoutMs: 1_000 })
const syntheticSettings = { workspaceRoot: '/synthetic/profile' } as AppSettingsV1
const canary = '/Users/private-owner/case.csv 13800138000 synthetic-secret'

function fixture(base = 'http://127.0.0.1:48999') {
  const pin = { runtimeUrl: base, runtimePid: 123, generation: 1, keyId: 'synthetic-key', publicKey: 'synthetic-public-key' }
  const state = { current: true, base, profile: 'profile-one', authorization: 'Bearer synthetic-token' }
  const options = {
    loadSettings: vi.fn(async () => syntheticSettings),
    ensureRuntime: vi.fn(async () => syntheticSettings),
    captureAuthorityPin: vi.fn(() => pin),
    isCurrentAuthorityPin: vi.fn(() => state.current),
    getBaseUrl: vi.fn(() => state.base),
    getProfileBinding: vi.fn(() => state.profile),
    authHeaders: vi.fn(() => new Headers({ Authorization: state.authorization }))
  }
  return { state, options, request: createPrivateMediaRuntimeRequest(options) }
}

afterEach(() => vi.unstubAllGlobals())

describe('Main-only private media runtime transport', () => {
  it('uses the fixed authenticated local HTTP route for successful images and Core privacy refusal', async () => {
    const received: Array<{ path?: string; authorization?: string; body: string }> = []
    const image = { schemaVersion: 1, status: 'ok', mimeType: 'image/png', imageBase64: 'c3ludGhldGlj' }
    const server = createServer(async (request, response) => {
      const chunks: Buffer[] = []
      for await (const chunk of request) chunks.push(Buffer.from(chunk))
      const body = Buffer.concat(chunks).toString('utf8')
      received.push({ path: request.url, authorization: request.headers.authorization, body })
      response.setHeader('Content-Type', 'application/json')
      if (JSON.parse(body).operation === 'image.generate') {
        response.end(JSON.stringify(image))
      } else {
        response.statusCode = 503
        response.end(JSON.stringify({ schemaVersion: 1, status: 'error', code: 'privacy_unavailable', message: canary }))
      }
    })
    await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
    try {
      const base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`
      const { request } = fixture(base)
      expect(await request(intent)).toEqual({ ok: true, status: 200, body: JSON.stringify(image) })
      for (const operation of ['image.edit', 'speech.transcribe']) {
        const result = await request(JSON.stringify({ schemaVersion: 1, operation, timeoutMs: 1_000 }))
        expect(result).toEqual({ ok: false, status: 503, body: JSON.stringify({ code: 'privacy_unavailable' }) })
        expect(JSON.stringify(result)).not.toContain(canary)
      }
      expect(received).toHaveLength(3)
      expect(received.every((entry) => entry.path === '/v1/runtime/_private/media-execution' && entry.authorization === 'Bearer synthetic-token')).toBe(true)
      expect(received[0].body).toBe(intent)
    } finally {
      server.closeAllConnections()
      await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()))
    }
  })

  it('rejects missing or mismatched current runtime authority before sending private bytes', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const missing = fixture()
    missing.options.captureAuthorityPin.mockReturnValue(null as never)
    expect(await missing.request(intent)).toMatchObject({ ok: false, status: 503 })
    const mismatch = fixture()
    mismatch.state.base = 'http://127.0.0.1:48998'
    expect(await mismatch.request(intent)).toMatchObject({ ok: false, status: 409 })
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it.each(['pin', 'base', 'profile', 'authorization'])('rejects %s changes while a request is in flight', async (change) => {
    const { request, state } = fixture()
    vi.stubGlobal('fetch', vi.fn(async () => {
      if (change === 'pin') state.current = false
      if (change === 'base') state.base = 'http://127.0.0.1:48998'
      if (change === 'profile') state.profile = 'profile-two'
      if (change === 'authorization') state.authorization = 'Bearer changed-token'
      return new Response(JSON.stringify({ schemaVersion: 1, status: 'ok', transcript: canary }), { headers: { 'Content-Type': 'application/json' } })
    }))
    expect(await request(intent)).toEqual({ ok: false, status: 409, body: JSON.stringify({ code: 'authority_changed' }) })
  })

  it('bounds request bodies and rejects renderer-supplied provider authority before dispatch', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const { request } = fixture()
    for (const body of ['', 'x'.repeat((24 << 20) + 1), '{broken', JSON.stringify({ ...JSON.parse(intent), baseUrl: canary })]) {
      expect(await request(body)).toEqual({ ok: false, status: 400, body: JSON.stringify({ code: 'invalid_request' }) })
    }
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('bounds both declared and streamed response bodies before typed projection', async () => {
    const { request } = fixture()
    vi.stubGlobal('fetch', vi.fn(async () => new Response(canary, {
      headers: { 'Content-Type': 'application/json', 'Content-Length': String((24 << 20) + 1) }
    })))
    expect(await request(intent)).toMatchObject({ ok: false, status: 502 })
    let chunks = 0
    const cancel = vi.fn()
    vi.stubGlobal('fetch', vi.fn(async () => new Response(new ReadableStream<Uint8Array>({
      pull(controller) {
        if (chunks++ < 2) controller.enqueue(new Uint8Array(13 << 20))
      }, cancel
    }), { headers: { 'Content-Type': 'application/json' } })))
    expect(await request(intent)).toEqual({ ok: false, status: 502, body: JSON.stringify({ code: 'provider_failed' }) })
    expect(cancel).toHaveBeenCalled()
  })

  it('projects only fixed failure enums and never follows a redirect or logs raw errors', async () => {
    const { request } = fixture()
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ code: canary, message: canary }), {
      status: 502, headers: { 'Content-Type': 'application/json' }
    }))
    vi.stubGlobal('fetch', fetchMock)
    expect(await request(intent)).toEqual({ ok: false, status: 502, body: JSON.stringify({ code: 'provider_failed' }) })
    expect(fetchMock).toHaveBeenCalledWith(expect.any(URL), expect.objectContaining({ redirect: 'error', cache: 'no-store' }))
    fetchMock.mockRejectedValueOnce(new Error(canary))
    expect(await request(intent)).toEqual({ ok: false, status: 503, body: JSON.stringify({ code: 'unavailable' }) })
  })

  it.each([
    [400, 'validation_error', 'invalid_request'],
    [409, 'conflict', 'authority_changed'],
    [503, 'internal_error', 'unavailable'],
    [400, 'privacy_unavailable', 'invalid_request']
  ])('preserves the Core HTTP failure classification for %s/%s', async (status, code, expected) => {
    const { request } = fixture()
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ code, message: canary }), {
      status: Number(status), headers: { 'Content-Type': 'application/json' }
    })))
    expect(await request(intent)).toEqual({ ok: false, status, body: JSON.stringify({ code: expected }) })
  })
})
