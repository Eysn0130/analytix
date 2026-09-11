import { createHash } from 'node:crypto'
import { mkdir, realpath, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { net } from 'electron'
import type { AppSettingsV1, ClawImChannelV1 } from '../shared/app-settings'
import type { ClawImTelegramConnectErrorCode } from '../shared/analytix-api'

const TELEGRAM_API_BASE = 'https://api.telegram.org'
const POLL_TIMEOUT_SECONDS = 25
const POLL_HTTP_TIMEOUT_MS = (POLL_TIMEOUT_SECONDS + 10) * 1000
const MAX_MESSAGE_LENGTH = 4096
const MAX_DOWNLOAD_BYTES = 20 * 1024 * 1024
const MIN_BACKOFF_MS = 1_500
const MAX_BACKOFF_MS = 30_000
const BACKOFF_JITTER_MS = 250
const GROUP_CHAT_TYPES = new Set(['group', 'supergroup', 'channel'])

function telegramFetch(input: string, init?: RequestInit): Promise<Response> {
  return typeof net.fetch === 'function'
    ? net.fetch(input, init)
    : fetch(input, init)
}

export type TelegramLogFn = (category: string, message: string, detail?: unknown) => void

export type TelegramInboundPayload = {
  channelId: string
  chatId: string
  messageId: string
  senderId: string
  senderName: string
  text: string
  localFilePath?: string
  updateId: number
}

export type TelegramRuntimeDeps = {
  logError: TelegramLogFn
  onInbound: (payload: TelegramInboundPayload) => void | Promise<void>
  resolveAccountCredential: (
    request: TelegramAccountCredentialRequest
  ) => Promise<TelegramAccountCredentialResolution>
}

export type TelegramAccountCredentialRequest = {
  owner: 'transport'
  provider: 'telegram'
  accountId: string
  channelId: string
  purpose: 'transport-telegram-bot-token'
}

export type TelegramAccountCredentialResolution =
  | {
      status: 'ready'
      generation: string
      incarnation: string
      botToken: string
      allowedChatIds: string
    }
  | {
      status: 'unavailable' | 'revoked'
      generation: string
      incarnation: string
    }

type TelegramChat = {
  id: number
  type?: string
  first_name?: string
  last_name?: string
  username?: string
  title?: string
}

type TelegramUser = {
  id: number
  is_bot?: boolean
  first_name?: string
  last_name?: string
  username?: string
}

type TelegramMessage = {
  message_id: number
  date?: number
  chat: TelegramChat
  from?: TelegramUser
  text?: string
  caption?: string
  photo?: Array<{ file_id: string; file_size?: number; width?: number; height?: number }>
}

type TelegramUpdate = {
  update_id: number
  message?: TelegramMessage
  edited_message?: TelegramMessage
  channel_post?: TelegramMessage
}

type TelegramApiResponse<T> = {
  ok: boolean
  result?: T
  description?: string
  error_code?: number
  parameters?: { retry_after?: number; migrate_to_chat_id?: number }
}

type TelegramFile = {
  file_id: string
  file_unique_id?: string
  file_size?: number
  file_path?: string
}

type TelegramBotInfo = {
  id: number
  username: string
  first_name?: string
  can_join_groups?: boolean
}

export type TelegramVerifyResult =
  | { ok: true; botId: number; botUsername: string; botFirstName: string }
  | { ok: false; code: ClawImTelegramConnectErrorCode; message: string }

class TelegramChannel {
  private abort: AbortController | null = null
  private running = false
  private offset = 0
  private consecutiveErrors = 0

  constructor(
    private readonly channelId: string,
    private readonly accountId: string,
    private readonly authority: Pick<
      Extract<TelegramAccountCredentialResolution, { status: 'ready' }>,
      'generation' | 'incarnation'
    >,
    private readonly allowedChatIds: ReadonlySet<number>,
    private readonly deps: TelegramRuntimeDeps
  ) {}

  get id(): string {
    return this.channelId
  }

  async start(): Promise<void> {
    if (this.running) return
    this.running = true
    this.abort = new AbortController()
    void this.pollLoop()
  }

  async stop(): Promise<void> {
    this.running = false
    this.abort?.abort()
    this.abort = null
  }

  async sendMessage(chatId: string, text: string): Promise<{ ok: true; messageId?: number } | { ok: false; message: string }> {
    const trimmed = text.trim()
    if (!trimmed) return { ok: true }
    let lastMessageId: number | undefined
    for (const chunk of splitForTelegram(trimmed)) {
      const result = await this.callApi<{ message_id: number }>('sendMessage', {
        chat_id: chatId,
        text: chunk,
        parse_mode: 'HTML',
        disable_web_page_preview: true
      })
      if (!result.ok) {
        const fallback = await this.callApi<{ message_id: number }>('sendMessage', {
          chat_id: chatId,
          text: chunk,
          disable_web_page_preview: true
        })
        if (!fallback.ok) return { ok: false, message: 'Telegram message delivery failed.' }
        lastMessageId = fallback.result?.message_id
        continue
      }
      lastMessageId = result.result?.message_id
    }
    return { ok: true, messageId: lastMessageId }
  }

  private async pollLoop(): Promise<void> {
    while (this.running) {
      const controller = new AbortController()
      const timeout = setTimeout(() => controller.abort(), POLL_HTTP_TIMEOUT_MS)
      const stopWatcher = (): void => controller.abort()
      this.abort?.signal.addEventListener('abort', stopWatcher, { once: true })
      try {
        const response = await this.callApi<TelegramUpdate[]>(
          'getUpdates',
          {
            offset: this.offset || undefined,
            timeout: POLL_TIMEOUT_SECONDS,
            allowed_updates: ['message']
          },
          controller.signal
        )
        if (!response.ok) {
          await this.handlePollError(response)
          continue
        }
        this.consecutiveErrors = 0
        const updates = Array.isArray(response.result) ? response.result : []
        for (const update of updates) {
          this.offset = update.update_id + 1
          this.dispatchUpdate(update)
        }
      } catch (error) {
        if (!this.running) break
        await this.handlePollException(error)
      } finally {
        clearTimeout(timeout)
        this.abort?.signal.removeEventListener('abort', stopWatcher)
      }
    }
  }

  private dispatchUpdate(update: TelegramUpdate): void {
    const message = update.message ?? update.edited_message
    if (!message?.chat) return
    const chat = message.chat
    if (isGroupChat(chat) || !this.isChatAllowed(chat.id)) return
    const sender = message.from
    const payload: TelegramInboundPayload = {
      channelId: this.channelId,
      chatId: String(chat.id),
      messageId: String(message.message_id),
      senderId: sender ? String(sender.id) : String(chat.id),
      senderName: senderDisplayName(sender) || String(chat.id),
      text: (message.caption ?? message.text ?? '').trim(),
      updateId: update.update_id
    }
    const photo = Array.isArray(message.photo) && message.photo.length > 0 ? message.photo : undefined
    if (!payload.text && !photo) return

    void (async () => {
      if (photo) {
        const downloaded = await this.downloadLargestPhoto(photo, chat.id, message.message_id)
        if (downloaded) payload.localFilePath = downloaded
        if (!payload.text) payload.text = '[image]'
      }
      try {
        await this.deps.onInbound(payload)
      } catch (error) {
        this.deps.logError('claw-telegram', 'Inbound handler threw for a Telegram update.', {
          channelId: this.channelId,
          chatId: payload.chatId,
          updateId: payload.updateId,
          message: error instanceof Error ? error.message : String(error)
        })
      }
    })()
  }

  private isChatAllowed(chatId: number): boolean {
    if (this.allowedChatIds.size === 0) return true
    return this.allowedChatIds.has(chatId)
  }

  private async downloadLargestPhoto(
    photo: NonNullable<TelegramMessage['photo']>,
    chatId: number,
    messageId: number
  ): Promise<string | undefined> {
    const largest = [...photo].sort((a, b) => (b.file_size ?? 0) - (a.file_size ?? 0))[0]
    if (!largest?.file_id) return undefined
    if (largest.file_size && largest.file_size > MAX_DOWNLOAD_BYTES) {
      this.deps.logError('claw-telegram', 'Skipping inbound Telegram photo: exceeds the 20 MB cap.', {
        channelId: this.channelId,
        chatId,
        messageId,
        bytes: largest.file_size
      })
      return undefined
    }
    try {
      const fileMeta = await this.callApi<TelegramFile>('getFile', { file_id: largest.file_id })
      if (!fileMeta.ok || !fileMeta.result?.file_path) {
        this.deps.logError('claw-telegram', 'Telegram getFile failed while downloading an inbound photo.', {
          channelId: this.channelId,
          chatId,
          message: fileMeta.ok ? 'missing file_path' : fileMeta.message
        })
        return undefined
      }
      const credential = await this.resolveCurrentCredential()
      if (!credential) return undefined
      const downloadUrl = `${TELEGRAM_API_BASE}/file/bot${credential.botToken}/${fileMeta.result.file_path}`
      const res = await telegramFetch(downloadUrl, { signal: AbortSignal.timeout(30_000) })
      if (!await this.isResolutionCurrent(credential)) return undefined
      if (!res.ok) {
        this.deps.logError('claw-telegram', 'Telegram file download returned a non-OK status.', {
          channelId: this.channelId,
          chatId,
          status: res.status
        })
        return undefined
      }
      const buffer = Buffer.from(await res.arrayBuffer())
      if (buffer.byteLength > MAX_DOWNLOAD_BYTES) {
        this.deps.logError('claw-telegram', 'Downloaded Telegram photo exceeds the 20 MB cap.', {
          channelId: this.channelId,
          chatId,
          bytes: buffer.byteLength
        })
        return undefined
      }
      const ext = inferImageExtension(fileMeta.result.file_path)
      const dir = join(tmpdir(), 'analytix-telegram-attachments')
      await mkdir(dir, { recursive: true })
      const digest = createHash('sha1').update(largest.file_id).digest('hex').slice(0, 10)
      const filePath = join(dir, `tg-${chatId}-${messageId}-${digest}.${ext}`)
      await writeFile(filePath, buffer)
      return realpath(filePath).catch(() => filePath)
    } catch (error) {
      this.deps.logError('claw-telegram', 'Failed to download an inbound Telegram photo.', {
        channelId: this.channelId,
        chatId
      })
      return undefined
    }
  }

  private async handlePollError(response: { ok: false; message: string; retryAfterMs?: number }): Promise<void> {
    this.consecutiveErrors += 1
    this.deps.logError('claw-telegram', 'Telegram getUpdates failed.', {
      channelId: this.channelId,
      message: response.message,
      retryAfterMs: response.retryAfterMs
    })
    await sleep(response.retryAfterMs ? Math.min(MAX_BACKOFF_MS, response.retryAfterMs) : this.nextBackoff())
  }

  private async handlePollException(_error: unknown): Promise<void> {
    if (!this.running) return
    this.consecutiveErrors += 1
    this.deps.logError('claw-telegram', 'Telegram poll loop caught an exception.', {
      channelId: this.channelId
    })
    await sleep(this.nextBackoff())
  }

  private nextBackoff(): number {
    const exponent = Math.min(this.consecutiveErrors, 6)
    const base = Math.min(MAX_BACKOFF_MS, MIN_BACKOFF_MS * 2 ** exponent)
    return Math.max(MIN_BACKOFF_MS, base - Math.floor(Math.random() * BACKOFF_JITTER_MS))
  }

  private async callApi<T>(
    method: string,
    body: Record<string, unknown>,
    signal?: AbortSignal
  ): Promise<{ ok: true; result?: T } | { ok: false; message: string; retryAfterMs?: number }> {
    const credential = await this.resolveCurrentCredential()
    if (!credential) {
      return { ok: false, message: 'Telegram account credential is unavailable.' }
    }
    const url = `${TELEGRAM_API_BASE}/bot${credential.botToken}/${method}`
    try {
      const res = await telegramFetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
        signal: signal ?? this.abort?.signal
      })
      const data = (await res.json().catch(() => null)) as TelegramApiResponse<T> | null
      if (!await this.isResolutionCurrent(credential)) {
        return { ok: false, message: 'Telegram account credential changed while the request was in flight.' }
      }
      if (!data) return { ok: false, message: `Telegram request failed (HTTP ${res.status}).` }
      if (!data.ok) {
        const retryAfter = data.parameters?.retry_after
        return {
          ok: false,
          message: `Telegram request failed (HTTP ${res.status}).`,
          retryAfterMs: typeof retryAfter === 'number' ? retryAfter * 1000 : undefined
        }
      }
      return { ok: true, result: data.result }
    } catch {
      if (!this.running && method === 'getUpdates') return { ok: false, message: 'aborted' }
      return { ok: false, message: 'Telegram request failed.' }
    }
  }

  private async resolveCurrentCredential(): Promise<
    Extract<TelegramAccountCredentialResolution, { status: 'ready' }> | null
  > {
    const resolution = await this.deps.resolveAccountCredential({
      owner: 'transport',
      provider: 'telegram',
      accountId: this.accountId,
      channelId: this.channelId,
      purpose: 'transport-telegram-bot-token'
    })
    if (resolution.status !== 'ready' ||
        resolution.generation !== this.authority.generation ||
        resolution.incarnation !== this.authority.incarnation ||
        !resolution.botToken.trim()) {
      return null
    }
    return resolution
  }

  private async isResolutionCurrent(
    prior: Extract<TelegramAccountCredentialResolution, { status: 'ready' }>
  ): Promise<boolean> {
    const current = await this.resolveCurrentCredential()
    return current != null &&
      current.generation === prior.generation &&
      current.incarnation === prior.incarnation
  }
}

export type TelegramRuntime = {
  sync(settings: AppSettingsV1): void
  stop(): void
  has(channelId: string): boolean
  sendMessage(channelId: string, chatId: string, text: string): Promise<{ ok: true } | { ok: false; message: string }>
}

export async function verifyTelegramBotToken(botToken: string): Promise<TelegramVerifyResult> {
  const token = botToken.trim()
  if (!token || !/^\d+:[A-Za-z0-9_-]{30,}$/.test(token)) {
    return {
      ok: false,
      code: 'invalid_format',
      message: 'Invalid token format. Expected "<numeric-id>:<35+ chars>".'
    }
  }
  const url = `${TELEGRAM_API_BASE}/bot${token}/getMe`
  try {
    const res = await telegramFetch(url, { signal: AbortSignal.timeout(15_000) })
    const data = (await res.json().catch(() => null)) as TelegramApiResponse<TelegramBotInfo> | null
    if (!data?.ok || !data.result) {
      return {
        ok: false,
        code: 'rejected',
        message: `Telegram rejected the token (HTTP ${res.status}).`
      }
    }
    return {
      ok: true,
      botId: data.result.id,
      botUsername: data.result.username,
      botFirstName: data.result.first_name ?? data.result.username
    }
  } catch {
    return {
      ok: false,
      code: 'network',
      message: 'Telegram verification request failed.'
    }
  }
}

export function createTelegramRuntime(deps: TelegramRuntimeDeps): TelegramRuntime {
  const channels = new Map<string, TelegramChannel>()
  const channelKeys = new Map<string, string>()
  let syncVersion = 0

  async function resolveTargets(
    settings: AppSettingsV1
  ): Promise<Array<{
    channel: ClawImChannelV1
    authority: Extract<TelegramAccountCredentialResolution, { status: 'ready' }>
    allowed: Set<number>
  }>> {
    if (!settings.claw.enabled || !settings.claw.im.enabled) return []
    const targets: Array<{
      channel: ClawImChannelV1
      authority: Extract<TelegramAccountCredentialResolution, { status: 'ready' }>
      allowed: Set<number>
    }> = []
    for (const channel of settings.claw.channels) {
      if (!channel.enabled || channel.provider !== 'telegram') continue
      const credential = await deps.resolveAccountCredential({
        owner: 'transport',
        provider: 'telegram',
        accountId: channel.platformAccount?.kind === 'telegram'
          ? channel.platformAccount.accountId
          : channel.id,
        channelId: channel.id,
        purpose: 'transport-telegram-bot-token'
      })
      if (credential.status !== 'ready' || !credential.botToken.trim()) continue
      targets.push({
        channel,
        authority: credential,
        allowed: parseAllowedChatIds(credential.allowedChatIds)
      })
    }
    return targets
  }

  function buildKey(
    channel: ClawImChannelV1,
    authority: Extract<TelegramAccountCredentialResolution, { status: 'ready' }>,
    allowed: Set<number>
  ): string {
    return `${channel.id}|${authority.generation}|${authority.incarnation}|${[...allowed].sort((a, b) => a - b).join(',')}`
  }

  async function closeChannel(channelId: string): Promise<void> {
    const channel = channels.get(channelId)
    if (!channel) return
    channels.delete(channelId)
    channelKeys.delete(channelId)
    await channel.stop().catch(() => undefined)
  }

  return {
    sync(settings: AppSettingsV1): void {
      const version = ++syncVersion
      void (async () => {
        const targets = await resolveTargets(settings)
        if (version !== syncVersion) return
        const targetMap = new Map(targets.map((entry) => [entry.channel.id, entry]))
        await Promise.all(
          [...channels.keys()]
            .filter((channelId) => !targetMap.has(channelId))
            .map((channelId) => closeChannel(channelId))
        )
        if (version !== syncVersion) return
        for (const target of targets) {
          if (version !== syncVersion) return
          const nextKey = buildKey(target.channel, target.authority, target.allowed)
          const currentKey = channelKeys.get(target.channel.id)
          if (channels.has(target.channel.id) && currentKey === nextKey) continue
          if (channels.has(target.channel.id)) {
            await closeChannel(target.channel.id)
            if (version !== syncVersion) return
          }
          const accountId = target.channel.platformAccount?.kind === 'telegram'
            ? target.channel.platformAccount.accountId
            : target.channel.id
          const channel = new TelegramChannel(target.channel.id, accountId, target.authority, target.allowed, deps)
          try {
            await channel.start()
            if (version !== syncVersion) {
              await channel.stop().catch(() => undefined)
              return
            }
            channels.set(target.channel.id, channel)
            channelKeys.set(target.channel.id, nextKey)
          } catch {
            deps.logError('claw-telegram', 'Failed to start a Telegram channel.', {
              channelId: target.channel.id
            })
          }
        }
      })()
    },

    stop(): void {
      syncVersion += 1
      for (const channel of channels.values()) {
        void channel.stop().catch(() => undefined)
      }
      channels.clear()
      channelKeys.clear()
    },

    has(channelId: string): boolean {
      return channels.has(channelId)
    },

    async sendMessage(channelId: string, chatId: string, text: string): Promise<{ ok: true } | { ok: false; message: string }> {
      const channel = channels.get(channelId)
      if (!channel) return { ok: false, message: 'Telegram channel is not connected.' }
      const result = await channel.sendMessage(chatId, text)
      if (!result.ok) {
        deps.logError('claw-telegram', 'Failed to send a Telegram reply.', {
          channelId,
          chatId,
          message: result.message
        })
      }
      return result.ok ? { ok: true } : { ok: false, message: result.message }
    }
  }
}

function isGroupChat(chat: TelegramChat): boolean {
  if (chat.id < 0) return true
  return typeof chat.type === 'string' && GROUP_CHAT_TYPES.has(chat.type)
}

function senderDisplayName(user: TelegramUser | undefined): string {
  if (!user) return ''
  const first = (user.first_name ?? '').trim()
  const last = (user.last_name ?? '').trim()
  const full = `${first} ${last}`.trim()
  return full || (user.username ?? '').trim()
}

export function parseAllowedChatIds(raw: string): Set<number> {
  const set = new Set<number>()
  if (typeof raw !== 'string') return set
  for (const part of raw.split(/[\s,]+/)) {
    const trimmed = part.trim()
    if (!trimmed) continue
    const id = Number(trimmed)
    if (!Number.isFinite(id) || id <= 0) continue
    set.add(id)
  }
  return set
}

function inferImageExtension(filePath: string): string {
  const ext = filePath.split('.').pop()?.toLowerCase() ?? ''
  if (ext === 'jpg' || ext === 'jpeg' || ext === 'png' || ext === 'gif' || ext === 'webp') return ext
  return 'jpg'
}

function splitForTelegram(text: string): string[] {
  if (text.length <= MAX_MESSAGE_LENGTH) return [text]
  const chunks: string[] = []
  let remaining = text
  while (remaining.length > MAX_MESSAGE_LENGTH) {
    let cut = remaining.lastIndexOf('\n\n', MAX_MESSAGE_LENGTH)
    if (cut <= 0) cut = remaining.lastIndexOf('\n', MAX_MESSAGE_LENGTH)
    if (cut <= 0) cut = remaining.lastIndexOf('. ', MAX_MESSAGE_LENGTH)
    if (cut <= 0) cut = MAX_MESSAGE_LENGTH
    chunks.push(remaining.slice(0, cut).trimEnd())
    remaining = remaining.slice(cut).trimStart()
  }
  if (remaining) chunks.push(remaining)
  return chunks
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
