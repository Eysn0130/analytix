import { afterEach, describe, expect, it, vi } from 'vitest'
import { mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { request as httpRequest } from 'node:http'
import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'
import {
  ensureWeixinBridgeRpcUrl,
  configureWeixinBridgeAccountCredentialResolver,
  configureWeixinBridgeContextTokenAuthority,
  sendWeixinBridgeMessage,
  stopWeixinBridgeRuntime,
  weixinBridgeRuntimeInternals
} from './weixin-bridge-runtime'

vi.mock('electron', () => ({
  app: {
    isPackaged: false,
    getPath: () => '/tmp/analytix-test-user-data',
    getVersion: () => '0.2.0-test'
  }
}))

const requireFromTest = createRequire(import.meta.url)

function postJson(url: string, payload: unknown): Promise<{ status: number; body: string }> {
  const target = new URL(url)
  const body = JSON.stringify(payload)
  return new Promise((resolve, reject) => {
    const request = httpRequest({
      hostname: target.hostname,
      port: target.port,
      path: target.pathname,
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        'content-length': Buffer.byteLength(body)
      }
    }, (response) => {
      const chunks: Buffer[] = []
      response.on('data', (chunk: Buffer) => chunks.push(chunk))
      response.on('end', () => resolve({
        status: response.statusCode ?? 0,
        body: Buffer.concat(chunks).toString('utf8')
      }))
    })
    request.on('error', reject)
    request.end(body)
  })
}

describe('weixin bridge runtime', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    configureWeixinBridgeAccountCredentialResolver(null)
    configureWeixinBridgeContextTokenAuthority(null)
    stopWeixinBridgeRuntime()
  })

  it('builds WeChat base_info from the bundled WeChat plugin package', () => {
    const pkg = requireFromTest('@tencent-weixin/openclaw-weixin/package.json') as {
      version: string
    }
    const baseInfo = weixinBridgeRuntimeInternals.buildBaseInfo()

    expect(baseInfo).toMatchObject({
      channel_version: pkg.version,
      bot_agent: 'Analytix/0.2.0-test'
    })
  })

  it('keeps OpenClaw-compatible account id normalization for existing WeChat state files', () => {
    const { normalizeAccountId } = weixinBridgeRuntimeInternals

    expect(normalizeAccountId('b0f5860fdecb@im.bot')).toBe('b0f5860fdecb-im-bot')
    expect(normalizeAccountId('ABC@IM.WECHAT')).toBe('abc-im-wechat')
    expect(normalizeAccountId('')).toBe('default')
    expect(normalizeAccountId('__proto__')).toBe('default')
  })

  it('does not expose the removed OpenClaw adapter builders', () => {
    expect(Object.keys(weixinBridgeRuntimeInternals)).not.toContain('buildGuiManagedOpenClawConfig')
    expect(Object.keys(weixinBridgeRuntimeInternals)).not.toContain('buildWeixinBridgeAdapterSource')
    expect(Object.keys(weixinBridgeRuntimeInternals)).not.toContain('parseNodeVersion')
  })

  it('extracts webhook generated files for WeChat media delivery, capped at three', () => {
    const { webhookGeneratedFiles } = weixinBridgeRuntimeInternals

    expect(webhookGeneratedFiles({
      ok: true,
      reply: 'done',
      files: [
        { path: '/ws/.analytix-images/cat.png', fileName: 'cat.png' },
        { path: '/ws/out/report.pdf' },
        { unrelated: true },
        { path: '/ws/a.png' },
        { path: '/ws/b.png' }
      ]
    })).toEqual([
      { path: '/ws/.analytix-images/cat.png', fileName: 'cat.png' },
      { path: '/ws/out/report.pdf', fileName: 'report.pdf' },
      { path: '/ws/a.png', fileName: 'a.png' }
    ])

    expect(webhookGeneratedFiles({ ok: true, reply: 'no files' })).toEqual([])
    expect(webhookGeneratedFiles({ files: 'not-an-array' })).toEqual([])
  })

  it('projects WeChat QR-start RPC failures without upstream response text', async () => {
    const errorSentinel = '/private/customer-pii-13900000053/wechat-qr-upstream-error'
    const rpcUrl = await ensureWeixinBridgeRpcUrl()
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response(JSON.stringify({ message: errorSentinel }), {
        status: 500,
        headers: { 'content-type': 'application/json' }
      }))
    vi.stubGlobal('fetch', fetchMock)

    const response = await postJson(rpcUrl, {
      jsonrpc: '2.0',
      id: 'qr-start-error',
      method: 'web.login.start',
      params: { force: true }
    })

    expect(response.status).toBe(200)
    expect(JSON.parse(response.body)).toEqual({
      jsonrpc: '2.0',
      id: null,
      ok: false,
      error: { message: 'WeChat QR setup failed.' }
    })
    expect(response.body).not.toContain(errorSentinel)
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain('/ilink/bot/get_bot_qrcode')
  })

  it('does not fall back to a legacy account-file token when canonical authority is unavailable', async () => {
    const accountId = 'outbound-error'
    const accountFile = join(
      '/tmp/analytix-test-user-data',
      'weixin-bridge',
      'openclaw-weixin',
      'accounts',
      `${accountId}.json`
    )
    await ensureWeixinBridgeRpcUrl()
    await mkdir(dirname(accountFile), { recursive: true })
    await writeFile(accountFile, JSON.stringify({ token: 'test-token' }), 'utf8')
    configureWeixinBridgeAccountCredentialResolver(vi.fn(async () => ({
      status: 'unavailable' as const, channelId: 'channel-revoked', generation: '0', incarnation: ''
    })))
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    try {
      const result = await sendWeixinBridgeMessage({
        accountId,
        to: 'wechat-recipient',
        text: 'public message'
      })

      expect(result).toEqual({ ok: false, message: 'WeChat account is not configured.' })
      expect(fetchMock).not.toHaveBeenCalled()
    } finally {
      await rm(accountFile, { force: true })
    }
  })

  it('resolves WeChat tokens JIT and discards a response after the credential generation changes', async () => {
    const accountId = 'purpose-bound-account'
    const accountFile = join(
      '/tmp/analytix-test-user-data',
      'weixin-bridge',
      'openclaw-weixin',
      'accounts',
      `${accountId}.json`
    )
    await ensureWeixinBridgeRpcUrl()
    await mkdir(dirname(accountFile), { recursive: true })
    await writeFile(accountFile, JSON.stringify({
      baseUrl: 'https://weixin.invalid',
      userId: 'owner-a'
    }), 'utf8')
    let generation = '1'
    const resolver = vi.fn(async () => generation === '0'
      ? { status: 'unavailable' as const, channelId: 'channel-purpose-bound', generation: '0', incarnation: '' }
      : {
          status: 'ready' as const,
          token: `synthetic-weixin-token-generation-${generation}`,
          channelId: 'channel-purpose-bound',
          generation,
          incarnation: 'incarnation-a'
        })
    configureWeixinBridgeAccountCredentialResolver(resolver)
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(init?.headers).toMatchObject({
        Authorization: 'Bearer synthetic-weixin-token-generation-1'
      })
      generation = '2'
      return new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } })
    })
    vi.stubGlobal('fetch', fetchMock)

    try {
      await expect(sendWeixinBridgeMessage({
        accountId,
        to: 'wechat-recipient',
        text: 'public message'
      })).resolves.toEqual({
        ok: false,
        message: 'WeChat account credential changed during delivery.'
      })
      expect(resolver).toHaveBeenCalledTimes(2)
      expect(await readFile(accountFile, 'utf8')).not.toContain('synthetic-weixin-token')

      generation = '0'
      fetchMock.mockClear()
      await expect(sendWeixinBridgeMessage({
        accountId,
        to: 'wechat-recipient',
        text: 'must not send'
      })).resolves.toEqual({ ok: false, message: 'WeChat account is not configured.' })
      expect(fetchMock).not.toHaveBeenCalled()
    } finally {
      await rm(accountFile, { force: true })
    }
  })
})
