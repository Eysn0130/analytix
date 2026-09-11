import type { LarkChannel, MarkdownStreamController, SendOptions } from '@larksuiteoapi/node-sdk'
import {
  AcceptedFinalItemCompletedEventV3Schema,
  TurnLifecycleEvent
} from '../../packages/runtime/src/contracts/events.js'
import type { SseSubscriber } from './claw-runtime-helpers'
import {
  acceptedFinalPublicationEventId,
  acceptedFinalPublicationPayloadDigest,
  verifiedAcceptedFinalDeliveryBatch
} from './accepted-final-publication'
import type { ManagedFinalPublicationAuthorityPinV1 } from './runtime/analytix-adapter'

export type { SseSubscriber } from './claw-runtime-helpers'

export type FeishuStreamLogger = (category: string, message: string, detail?: unknown) => void

export type FeishuStreamerOptions = {
  bridge: LarkChannel
  chatId: string
  turnId: string
  threadId: string
  replyOptions: SendOptions
  logger: FeishuStreamLogger
  finalPublicationAuthorityPin?: ManagedFinalPublicationAuthorityPinV1 | null
  isFinalPublicationAuthorityPinCurrent?: (pin: ManagedFinalPublicationAuthorityPinV1 | null) => boolean
  isBridgeCredentialAuthorityCurrent?: () => Promise<boolean>
}

export type FeishuStreamerResult = {
  ok: boolean
  messageId: string
  finalText: string
  fellBack: boolean
}

export const FEISHU_PUBLIC_PROJECTION_BOUNDARY =
  '案件公开投影当前不可用，未发布任何案件事实。请返回 Analytix 完成证据与发布权限复验后重试。'

type BufferedAssistantFinal = {
  itemId: string
  text: string
  acceptedFinalDigest: string
  terminalReason: string
  acceptedAt: string
  itemSeq: number
  publicationEventId: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

export class FeishuStreamer {
  private readonly opts: FeishuStreamerOptions
  private readonly finalPublicationAuthorityPin: ManagedFinalPublicationAuthorityPinV1 | null
  private readonly outbox: Array<string | null> = []
  private readonly waiters: Array<(chunk: string | null) => void> = []
  private state: 'pending' | 'streaming' | 'boundary' | 'closed' = 'pending'
  private accumulatedText = ''
  private subscription: { close: () => void } | null = null
  private startAbortController: AbortController | null = null
  private failed = false
  private terminalHandled = false
  private bufferedFinal: BufferedAssistantFinal | null = null

  constructor(opts: FeishuStreamerOptions) {
    this.opts = opts
    this.finalPublicationAuthorityPin = opts.finalPublicationAuthorityPin
      ? { ...opts.finalPublicationAuthorityPin }
      : null
  }

  private isFinalPublicationAuthorityCurrent(): boolean {
    return Boolean(
      this.finalPublicationAuthorityPin &&
      this.opts.isFinalPublicationAuthorityPinCurrent?.(this.finalPublicationAuthorityPin)
    )
  }

  private async isBridgeCredentialAuthorityCurrent(): Promise<boolean> {
    try {
      return this.opts.isBridgeCredentialAuthorityCurrent
        ? await this.opts.isBridgeCredentialAuthorityCurrent()
        : true
    } catch {
      return false
    }
  }

  start(input: { subscribe: SseSubscriber }): Promise<FeishuStreamerResult> {
    return new Promise<FeishuStreamerResult>((resolve, reject) => {
      const controller = new AbortController()
      this.startAbortController = controller
      this.failed = false
      this.terminalHandled = false
      if (this.state === 'pending') this.state = 'streaming'
      if (!this.isFinalPublicationAuthorityCurrent()) this.enterBoundaryState()
      let resolved = false
      const onComplete = (result: FeishuStreamerResult): void => {
        if (resolved) return
        resolved = true
        resolve(result)
      }
      const onError = (error: Error): void => {
        if (resolved) return
        resolved = true
        reject(error)
      }

      const producer = async (streamController: MarkdownStreamController): Promise<void> => {
        try {
          if (!await this.isBridgeCredentialAuthorityCurrent()) {
            this.state = 'closed'
            onComplete({
              ok: false, messageId: streamController.messageId,
              finalText: '', fellBack: false
            })
            return
          }
          while (this.state === 'streaming') {
            const chunk = await this.nextDelta()
            if (chunk === null) break
            if (this.state !== 'streaming') break
            this.accumulatedText += chunk
          }
          if (!this.isFinalPublicationAuthorityCurrent()) this.enterBoundaryState()
          if (this.isBoundaryState()) {
            const replaced = await this.replaceWithBoundary(streamController)
            this.state = 'closed'
            onComplete({
              ok: false,
              messageId: streamController.messageId,
              finalText: FEISHU_PUBLIC_PROJECTION_BOUNDARY,
              fellBack: false
            })
            if (!replaced) {
              this.opts.logger('claw-feishu-stream', 'failed to replace or recall revoked stream message')
            }
            return
          }
          try {
            if (!await this.isBridgeCredentialAuthorityCurrent()) {
              this.state = 'closed'
              onComplete({ ok: false, messageId: streamController.messageId, finalText: '', fellBack: false })
              return
            }
            await streamController.setContent(this.accumulatedText)
            if (!await this.isBridgeCredentialAuthorityCurrent()) {
              this.state = 'closed'
              onComplete({ ok: false, messageId: streamController.messageId, finalText: '', fellBack: false })
              return
            }
          } catch (error) {
            this.opts.logger('claw-feishu-stream', 'final setContent failed; revoking provisional content', {
              message: error instanceof Error ? error.message : String(error)
            })
            this.enterBoundaryState()
            await this.replaceWithBoundary(streamController)
            this.state = 'closed'
            onComplete({
              ok: false,
              messageId: streamController.messageId,
              finalText: FEISHU_PUBLIC_PROJECTION_BOUNDARY,
              fellBack: false
            })
            return
          }
          if (!this.isFinalPublicationAuthorityCurrent()) this.enterBoundaryState()
          if (this.state === 'boundary') {
            await this.replaceWithBoundary(streamController)
            this.state = 'closed'
            onComplete({
              ok: false,
              messageId: streamController.messageId,
              finalText: FEISHU_PUBLIC_PROJECTION_BOUNDARY,
              fellBack: false
            })
            return
          }
          this.state = 'closed'
          onComplete({
            ok: !this.failed,
            messageId: streamController.messageId,
            finalText: this.accumulatedText,
            fellBack: false
          })
        } catch (error) {
          onError(error instanceof Error ? error : new Error(String(error)))
        }
      }

      try {
        const subscription = input.subscribe(controller.signal)
        if (this.state === 'boundary' || this.state === 'closed' || this.terminalHandled) {
          subscription.close()
        } else {
          this.subscription = subscription
        }
      } catch (error) {
        onError(error instanceof Error ? error : new Error(String(error)))
        return
      }
      const onAbort = (): void => {
        this.enterBoundaryState()
      }
      controller.signal.addEventListener('abort', onAbort, { once: true })

      const bridgeAny = this.opts.bridge as unknown as {
        stream: (
          to: string,
          input: { markdown: (c: MarkdownStreamController) => Promise<void> },
          opts?: SendOptions
        ) => Promise<{ messageId: string }>
      }
      const sendPromise = (async () => {
        if (!await this.isBridgeCredentialAuthorityCurrent()) {
          throw new Error('Feishu account credential is unavailable.')
        }
        const result = await bridgeAny.stream(
          this.opts.chatId,
          { markdown: producer },
          this.opts.replyOptions
        )
        if (!await this.isBridgeCredentialAuthorityCurrent()) {
          throw new Error('Feishu account credential changed.')
        }
        return result
      })()
      void sendPromise.catch((error: unknown) => {
        this.enterBoundaryState()
        controller.abort()
        onError(error instanceof Error ? error : new Error(String(error)))
      })
    })
  }

  onSseEvent(event: Record<string, unknown>): void {
    const kind = event.kind
    if (kind === 'public_projection_revoked' && event.threadId === this.opts.threadId) {
      this.enterBoundaryState()
      return
    }
    if (this.state !== 'streaming') return
    if (!this.isFinalPublicationAuthorityCurrent()) {
      this.enterBoundaryState()
      return
    }
    if (kind === 'assistant_text_delta') {
      this.opts.logger('claw-feishu-stream-debug', 'drop non-authoritative assistant draft', {
        turnId: this.opts.turnId
      })
      return
    }
    if (kind === 'assistant_reasoning_delta') {
      this.opts.logger('claw-feishu-stream-debug', 'drop reasoning delta', { turnId: this.opts.turnId })
      return
    }
    if (this.terminalHandled) return
    if (kind === 'accepted_final_batch') {
      const accepted = verifiedAcceptedFinalDeliveryBatch(
        event,
        this.finalPublicationAuthorityPin
      )
      if (!accepted || accepted.batch.threadId !== this.opts.threadId ||
          accepted.batch.turnId !== this.opts.turnId) {
        this.enterBoundaryState()
        return
      }
      for (const nested of accepted.events) {
        if (nested.publicationSlot === 'assistant-final') this.bufferAssistantFinal(nested)
        if (nested.publicationSlot === 'terminal') this.finishTerminal(nested)
      }
      return
    }
    if (kind === 'item_completed') {
      if (isRecord(event.item) && ('acceptedFinal' in event.item || 'acceptedFinalView' in event.item)) {
        this.enterBoundaryState()
      }
      return
    }
    if (
      (kind === 'turn_completed' || kind === 'turn_failed' || kind === 'turn_aborted') &&
      event.threadId === this.opts.threadId &&
      event.turnId === this.opts.turnId
    ) {
      this.finishTerminal(event)
    }
  }

  private bufferAssistantFinal(event: Record<string, unknown>): void {
    if (event.threadId !== this.opts.threadId || event.turnId !== this.opts.turnId || !isRecord(event.item)) {
      return
    }
    const item = event.item
    const expectedItemId = `item_${this.opts.turnId}_assistant`
    if (
      item.kind !== 'assistant_text' ||
      item.role !== 'assistant' ||
      item.status !== 'completed' ||
      item.threadId !== this.opts.threadId ||
      item.turnId !== this.opts.turnId ||
      item.id !== expectedItemId ||
      event.itemId !== expectedItemId ||
      typeof item.text !== 'string'
    ) {
      return
    }

    const accepted = AcceptedFinalItemCompletedEventV3Schema.safeParse(event)
    if (!accepted.success) {
      this.enterBoundaryState()
      return
    }
    const view = accepted.data.item.acceptedFinalView
    if (!view || view.schemaVersion !== 3) {
      this.enterBoundaryState()
      return
    }
    const candidate: BufferedAssistantFinal = {
      itemId: expectedItemId,
      text: item.text,
      acceptedFinalDigest: view.acceptedFinalDigest,
      terminalReason: view.terminalReason,
      acceptedAt: view.acceptedAt,
      itemSeq: accepted.data.seq,
      publicationEventId: accepted.data.publicationEventId
    }

    if (this.bufferedFinal) {
      const duplicate = this.bufferedFinal.itemId === candidate.itemId &&
        this.bufferedFinal.text === candidate.text &&
        this.bufferedFinal.acceptedFinalDigest === candidate.acceptedFinalDigest &&
        this.bufferedFinal.terminalReason === candidate.terminalReason &&
        this.bufferedFinal.itemSeq === candidate.itemSeq &&
        this.bufferedFinal.publicationEventId === candidate.publicationEventId
      if (!duplicate) this.enterBoundaryState()
      return
    }
    this.bufferedFinal = candidate
  }

  private finishTerminal(event: Record<string, unknown>): void {
    if (!this.isFinalPublicationAuthorityCurrent()) {
      this.enterBoundaryState()
      return
    }
    const final = this.bufferedFinal
    if (!final?.text) {
      this.enterBoundaryState()
      return
    }

    const terminal = TurnLifecycleEvent.safeParse(event)
    if (
      !terminal.success ||
      terminal.data.threadId !== this.opts.threadId ||
      terminal.data.turnId !== this.opts.turnId ||
      terminal.data.acceptedFinalDigest !== final.acceptedFinalDigest ||
      terminal.data.terminalReason !== final.terminalReason ||
      terminal.data.timestamp !== final.acceptedAt ||
      terminal.data.seq <= final.itemSeq ||
      event.publicationCommitId !== final.acceptedFinalDigest ||
      event.publicationSlot !== 'terminal' ||
      event.publicationEventId !== acceptedFinalPublicationEventId(final.acceptedFinalDigest, 'terminal') ||
      event.publicationPayloadDigest !== acceptedFinalPublicationPayloadDigest(event)
    ) {
      this.enterBoundaryState()
      return
    }
    this.failed = terminal.data.kind !== 'turn_completed'

    this.terminalHandled = true
    this.bufferedFinal = null
    this.subscription?.close()
    this.subscription = null
    this.push(final.text)
    this.push(null)
  }

  private push(chunk: string | null): void {
    if (this.waiters.length > 0) {
      const waiter = this.waiters.shift()!
      waiter(chunk)
      return
    }
    this.outbox.push(chunk)
  }

  private nextDelta(): Promise<string | null> {
    if (this.outbox.length > 0) {
      return Promise.resolve(this.outbox.shift() ?? null)
    }
    return new Promise<string | null>((resolve) => {
      this.waiters.push(resolve)
    })
  }

  getAccumulatedText(): string {
    return this.accumulatedText
  }

  private isBoundaryState(): boolean {
    return this.state === 'boundary'
  }

  private enterBoundaryState(): void {
    if (this.state === 'closed') return
    this.failed = true
    this.terminalHandled = true
    this.state = 'boundary'
    this.accumulatedText = ''
    this.bufferedFinal = null
    this.outbox.length = 0
    this.subscription?.close()
    this.subscription = null
    while (this.waiters.length > 0) {
      const waiter = this.waiters.shift()!
      waiter(null)
    }
  }

  private async replaceWithBoundary(streamController: MarkdownStreamController): Promise<boolean> {
    if (!await this.isBridgeCredentialAuthorityCurrent()) return false
    try {
      await streamController.setContent(FEISHU_PUBLIC_PROJECTION_BOUNDARY)
      return this.isBridgeCredentialAuthorityCurrent()
    } catch (error) {
      this.opts.logger('claw-feishu-stream', 'boundary replacement failed; recalling message', {
        message: error instanceof Error ? error.message : String(error)
      })
      const recallMessage = (this.opts.bridge as unknown as {
        recallMessage?: (messageId: string) => Promise<void>
      }).recallMessage
      if (typeof recallMessage !== 'function') return false
      try {
        if (!await this.isBridgeCredentialAuthorityCurrent()) return false
        await recallMessage.call(this.opts.bridge, streamController.messageId)
        return this.isBridgeCredentialAuthorityCurrent()
      } catch (recallError) {
        this.opts.logger('claw-feishu-stream', 'revoked message recall failed', {
          message: recallError instanceof Error ? recallError.message : String(recallError)
        })
        return false
      }
    }
  }

  abort(): void {
    this.enterBoundaryState()
    this.startAbortController?.abort()
  }

  dispose(): void {
    this.abort()
    this.state = 'closed'
    while (this.waiters.length > 0) {
      const waiter = this.waiters.shift()!
      waiter(null)
    }
  }
}
