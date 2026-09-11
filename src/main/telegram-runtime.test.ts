import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  defaultAnalytixRuntimeSettings,
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  type AppSettingsV1
} from '../shared/app-settings'

const { fetchMock } = vi.hoisted(() => ({
  fetchMock: vi.fn()
}))

vi.mock('electron', () => ({
  net: {
    fetch: fetchMock
  }
}))

import { createTelegramRuntime, parseAllowedChatIds, verifyTelegramBotToken } from './telegram-runtime'

type Deferred<T> = {
  promise: Promise<T>
  resolve: (value: T) => void
}

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' }
  })
}

function telegramSettings(): AppSettingsV1 {
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    provider: defaultModelProviderSettings(),
    runtime: defaultAnalytixRuntimeSettings(),
    workspaceRoot: '/tmp/workspace',
    log: { enabled: true, retentionDays: 7 },
    notifications: { turnComplete: true },
    appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
    keyboardShortcuts: defaultKeyboardShortcuts(),
    write: defaultWriteSettings(),
    schedule: defaultScheduleSettings(),
    claw: {
      ...defaultClawSettings(),
      enabled: true,
      im: { ...defaultClawSettings().im, enabled: true },
      channels: [{
        id: 'channel-telegram',
        provider: 'telegram',
        label: 'Telegram Bot',
        enabled: true,
        model: 'auto',
        threadId: '',
        workspaceRoot: '/tmp/workspace',
        agentProfile: {
          name: 'analytix',
          description: '',
          identity: '',
          personality: '',
          userContext: '',
          replyRules: ''
        },
        platformAccount: {
          kind: 'telegram',
          accountId: 'channel-telegram',
          allowedChatIds: '1001',
          createdAt: '2026-08-26T00:00:00.000Z'
        },
        conversations: [],
        createdAt: '2026-08-26T00:00:00.000Z',
        updatedAt: '2026-08-26T00:00:00.000Z'
      }]
    },
    guiUpdate: { channel: 'stable' },
    codePromptPrefix: '',
    disabledSkillIds: []
  }
}

function telegramAccountResolver() {
  return vi.fn(async () => ({
    status: 'ready' as const,
    generation: '1',
    incarnation: 'incarnation-test',
    botToken: '123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcd',
    allowedChatIds: '1001'
  }))
}

describe('telegram-runtime', () => {
  beforeEach(() => {
    fetchMock.mockReset()
  })

  it('parses Telegram private chat allowlists from comma and whitespace separated input', () => {
    expect([...parseAllowedChatIds('1001, 1002\nabc 1001 -2000')]).toEqual([1001, 1002])
  })

  it('rejects malformed Telegram bot tokens before making a network request', async () => {
    const result = await verifyTelegramBotToken('not-a-token')

    expect(result).toMatchObject({ ok: false, code: 'invalid_format' })
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('verifies Telegram bot tokens with the official getMe API', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({
      ok: true,
      result: {
        id: 123456789,
        username: 'analytix_bot',
        first_name: 'analytix'
      }
    }))

    const result = await verifyTelegramBotToken('123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcd')

    expect(fetchMock).toHaveBeenCalledWith(
      'https://api.telegram.org/bot123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcd/getMe',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    expect(result).toEqual({
      ok: true,
      botId: 123456789,
      botUsername: 'analytix_bot',
      botFirstName: 'analytix'
    })
  })

  it('maps Telegram rejections into connect-phone error codes', async () => {
    const providerBodySentinel = '/private/customer-pii-13900000029 raw-telegram-provider-body'
    fetchMock.mockResolvedValueOnce(jsonResponse({
      ok: false,
      description: providerBodySentinel
    }, 401))

    const result = await verifyTelegramBotToken('123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcd')

    expect(result).toEqual({
      ok: false,
      code: 'rejected',
      message: 'Telegram rejected the token (HTTP 401).'
    })
    expect(JSON.stringify(result)).not.toContain(providerBodySentinel)
    expect(fetchMock).toHaveBeenCalledOnce()
  })

  it('maps Telegram network failures into recoverable connect-phone errors', async () => {
    const networkErrorSentinel = 'https://api.telegram.example/private/customer-pii-13900000030?token=secret'
    fetchMock.mockRejectedValueOnce(new Error(networkErrorSentinel))

    const result = await verifyTelegramBotToken('123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcd')

    expect(result).toEqual({
      ok: false,
      code: 'network',
      message: 'Telegram verification request failed.'
    })
    expect(JSON.stringify(result)).not.toContain(networkErrorSentinel)
    expect(fetchMock).toHaveBeenCalledOnce()
  })

  it('resolves the current purpose-bound account credential JIT and rejects stale or revoked generations', async () => {
    fetchMock.mockImplementation(() => new Promise<Response>(() => undefined))
    const stale = deferred<{
      status: 'ready'
      generation: string
      incarnation: string
      botToken: string
      allowedChatIds: string
    }>()
    let current: {
      status: 'ready' | 'revoked'
      generation: string
      incarnation: string
      botToken?: string
      allowedChatIds?: string
    } = {
      status: 'ready',
      generation: '2',
      incarnation: 'incarnation-current',
      botToken: '123456789:synthetic-current-telegram-token',
      allowedChatIds: '1001'
    }
    const resolveAccountCredential = vi.fn()
      .mockImplementationOnce(() => stale.promise)
      .mockImplementation(() => Promise.resolve(current))
    const settings = telegramSettings()
    const logError = vi.fn()
    const runtime = createTelegramRuntime({
      logError,
      onInbound: vi.fn(),
      resolveAccountCredential
    } as unknown as Parameters<typeof createTelegramRuntime>[0])

    runtime.sync(settings)
    runtime.sync(settings)
    stale.resolve({
      status: 'ready',
      generation: '1',
      incarnation: 'incarnation-stale',
      botToken: '123456789:synthetic-stale-telegram-token',
      allowedChatIds: '9999'
    })

    await vi.waitFor(() => expect(runtime.has('channel-telegram')).toBe(true))
    expect(resolveAccountCredential).toHaveBeenCalledWith({
      owner: 'transport',
      provider: 'telegram',
      accountId: 'channel-telegram',
      channelId: 'channel-telegram',
      purpose: 'transport-telegram-bot-token'
    })
    const requestedURLs = fetchMock.mock.calls.map(([input]) => String(input))
    expect(requestedURLs.some((url) => url.includes('synthetic-current-telegram-token'))).toBe(true)
    expect(requestedURLs.some((url) => url.includes('synthetic-stale-telegram-token'))).toBe(false)

    current = {
      status: 'revoked',
      generation: '3',
      incarnation: 'incarnation-current'
    }
    runtime.sync(settings)
    await vi.waitFor(() => expect(runtime.has('channel-telegram')).toBe(false))
    runtime.stop()
  })

  it.each([
    {
      name: 'remote rejection bodies',
      sentinel: '/private/customer-pii-13900000031 raw-telegram-send-provider-body',
      sendResponse: () => Promise.resolve(jsonResponse({
        ok: false,
        description: '/private/customer-pii-13900000031 raw-telegram-send-provider-body'
      }, 400))
    },
    {
      name: 'network exceptions',
      sentinel: 'https://api.telegram.example/private/customer-pii-13900000032?token=secret',
      sendResponse: () => Promise.reject(new Error(
        'https://api.telegram.example/private/customer-pii-13900000032?token=secret'
      ))
    }
  ])('projects Telegram send $name after the plain-text fallback', async ({ sentinel, sendResponse }) => {
    fetchMock.mockImplementation((input: string) => (
      input.endsWith('/getUpdates')
        ? new Promise<Response>(() => undefined)
        : sendResponse()
    ))
    const logError = vi.fn()
    const runtime = createTelegramRuntime({
      logError,
      onInbound: vi.fn(),
      resolveAccountCredential: telegramAccountResolver()
    })
    runtime.sync(telegramSettings())
    await vi.waitFor(() => expect(runtime.has('channel-telegram')).toBe(true))

    const result = await runtime.sendMessage('channel-telegram', '1001', 'hello')
    runtime.stop()

    expect(result).toEqual({ ok: false, message: 'Telegram message delivery failed.' })
    expect(JSON.stringify(result)).not.toContain(sentinel)
    expect(JSON.stringify(logError.mock.calls)).not.toContain(sentinel)
    expect(runtime.has('channel-telegram')).toBe(false)
    const sendCalls = fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/sendMessage'))
    expect(sendCalls).toHaveLength(2)
    expect(JSON.parse(String(sendCalls[0]?.[1]?.body))).toMatchObject({ parse_mode: 'HTML' })
    expect(JSON.parse(String(sendCalls[1]?.[1]?.body))).not.toHaveProperty('parse_mode')
  })
})
