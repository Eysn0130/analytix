/* Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
import {
  MessageFeedbackConflictV1Schema, MessageFeedbackDeleteResponseV1Schema,
  MessageFeedbackItemV1Schema, MessageFeedbackListResponseV1Schema,
  MessageFeedbackPutRequestV1Schema,
  type MessageFeedbackCategoryV1, type MessageFeedbackItemV1, type MessageFeedbackRatingV1
} from '../../../../../../../packages/runtime/src/contracts/message-feedback'
import { analytixMessageFeedbackItemPath, analytixMessageFeedbackPath } from '@shared/analytix-endpoints'
import type { AnalytixRuntimeApi } from '@shared/analytix-api'
import { rendererRuntimeClient } from '../../../../agent/runtime-client'

type Transport = Pick<AnalytixRuntimeApi, 'runtimeRequest' | 'onRuntimeStatus'>
export type MessageFeedbackError = 'load' | 'save' | 'conflict' | 'note-too-large'
export type MessageFeedbackDialogState = {
  messageId: string; rating: MessageFeedbackRatingV1; note: string
  category?: MessageFeedbackCategoryV1; opener: HTMLElement | null; owner: symbol
}
export type MessageFeedbackState = {
  ensured: boolean; loading: boolean; pending: boolean
  items: Readonly<Record<string, MessageFeedbackItemV1>>
  dialog: MessageFeedbackDialogState | null
  error: MessageFeedbackError | null; errorMessageId: string | null; errorOwner: symbol | null
  acknowledgement: { messageId: string; serial: number; owner: symbol } | null
}

// One queue owns reads, mutations and reconnect reconciliation for a thread.
// No reply text, transcript or telemetry enters this client.
export class MessageFeedbackController {
  private state: MessageFeedbackState = { ensured: false, loading: false, pending: false,
    items: {}, dialog: null, error: null, errorMessageId: null, errorOwner: null, acknowledgement: null }
  private listeners = new Set<() => void>()
  private owners = new Map<symbol, number>()
  private defaultOwner = Symbol('feedback-controller')
  private queue: Promise<unknown> = Promise.resolve()
  private loading: Promise<void> | null = null
  private refreshQueued = false
  private unsubscribeStatus: (() => void) | null = null
  private attempted = false
  private disposed = false
  private serial = 0

  constructor(readonly threadId: string, private transport: Transport, private onEmpty?: () => void) {}
  getSnapshot = (): MessageFeedbackState => this.state
  subscribe = (listener: () => void, owner = this.defaultOwner): (() => void) => {
    this.listeners.add(listener)
    this.owners.set(owner, (this.owners.get(owner) ?? 0) + 1)
    if (!this.unsubscribeStatus) this.unsubscribeStatus = this.transport.onRuntimeStatus(payload => {
      if (payload.state === 'running' && this.attempted && !this.disposed) void this.ensure(true)
    })
    return () => {
      this.listeners.delete(listener)
      this.owners.set(owner, (this.owners.get(owner) ?? 1) - 1)
      queueMicrotask(() => {
        if (!this.owners.get(owner)) {
          this.owners.delete(owner)
          if (this.state.dialog?.owner === owner) this.publish({ dialog: null })
          if (this.state.errorOwner === owner) this.publish({ error: null, errorMessageId: null, errorOwner: null })
          if (this.state.acknowledgement?.owner === owner) this.publish({ acknowledgement: null })
        }
        if (this.listeners.size) return
        this.disposed = true
        this.unsubscribeStatus?.(); this.unsubscribeStatus = null
        this.onEmpty?.()
      })
    }
  }
  private publish(patch: Partial<MessageFeedbackState>): void {
    if (this.disposed) return
    this.state = { ...this.state, ...patch }
    for (const listener of this.listeners) listener()
  }
  private serialized<T>(task: () => Promise<T>): Promise<T> {
    const operation = this.queue.then(task, task)
    this.queue = operation.catch(() => undefined)
    return operation
  }
  ensure = (refresh = false): Promise<void> => {
    if (this.disposed) return Promise.resolve()
    if (this.loading) { if (refresh) this.refreshQueued = true; return this.loading }
    if (this.state.ensured && !refresh) return Promise.resolve()
    this.attempted = true
    this.publish({ loading: true })
    const operation = this.serialized(async () => {
      try {
        const response = await this.transport.runtimeRequest(analytixMessageFeedbackPath(this.threadId), 'GET')
        if (!response.ok) throw new Error('feedback-read-unconfirmed')
        const result = MessageFeedbackListResponseV1Schema.parse(JSON.parse(response.body))
        const items = Object.fromEntries(result.items.map(item => [item.messageId, item]))
        this.publish({ items, ensured: true, loading: false,
          ...(this.state.error === 'load' ? { error: null, errorMessageId: null, errorOwner: null } : {}) })
      } catch {
        this.publish({ loading: false, error: 'load', errorMessageId: null, errorOwner: null })
      }
    })
    this.loading = operation
    void operation.finally(() => {
      if (this.loading === operation) this.loading = null
      if (this.refreshQueued && !this.disposed) { this.refreshQueued = false; void this.ensure(true) }
    })
    return operation
  }
  rate = async (messageId: string, rating: MessageFeedbackRatingV1, opener: HTMLElement | null, owner = this.defaultOwner): Promise<void> => {
    if (this.disposed || this.state.pending || this.state.dialog) return
    await this.ensure()
    if (this.disposed || !this.owners.get(owner) || this.state.loading || this.state.pending || this.state.dialog) return
    const current = this.state.items[messageId]
    if (current?.rating === rating) {
      this.publish({ pending: true, error: null, errorMessageId: null, errorOwner: null })
      await this.mutate(messageId, 'DELETE', { ifVersion: current.version }, owner)
    } else {
      this.publish({ dialog: { messageId, rating, note: '', opener, owner }, error: null, errorMessageId: null, errorOwner: null })
    }
  }
  edit = (patch: Partial<Pick<MessageFeedbackDialogState, 'note' | 'category'>>): void => {
    if (!this.state.dialog || this.state.pending || this.disposed) return
    this.publish({ dialog: { ...this.state.dialog, ...patch }, error: null, errorMessageId: null, errorOwner: null })
  }
  close = (): void => { if (!this.state.pending) this.publish({ dialog: null, error: null, errorMessageId: null, errorOwner: null }) }
  clearAcknowledgement = (owner: symbol, serial: number): void => {
    if (this.state.acknowledgement?.owner === owner && this.state.acknowledgement.serial === serial) this.publish({ acknowledgement: null })
  }
  submit = async (): Promise<void> => {
    const dialog = this.state.dialog
    if (!dialog || this.state.pending || this.disposed) return
    const note = dialog.note.trim()
    const request = { rating: dialog.rating, ...(note ? { note } : {}),
      ...(dialog.category ? { category: dialog.category } : {}),
      ifVersion: this.state.items[dialog.messageId]?.version ?? null }
    if (!MessageFeedbackPutRequestV1Schema.safeParse(request).success) {
      this.publish({ error: 'note-too-large', errorMessageId: dialog.messageId, errorOwner: dialog.owner }); return
    }
    this.publish({ pending: true, error: null, errorMessageId: null, errorOwner: null })
    await this.mutate(dialog.messageId, 'PUT', request, dialog.owner)
  }
  private mutate(messageId: string, method: 'PUT' | 'DELETE', request: unknown, owner: symbol): Promise<void> {
    return this.serialized(async () => {
      try {
        const response = await this.transport.runtimeRequest(analytixMessageFeedbackItemPath(this.threadId, messageId), method, JSON.stringify(request))
        const body: unknown = JSON.parse(response.body)
        if (!response.ok) {
          const conflict = response.status === 409 ? MessageFeedbackConflictV1Schema.safeParse(body) : null
          if (conflict?.success && (!conflict.data.current || conflict.data.current.messageId === messageId)) {
            const items = { ...this.state.items }
            if (conflict.data.current) items[messageId] = conflict.data.current
            else delete items[messageId]
            this.publish({ items, pending: false, ...(this.owners.get(owner)
              ? { error: 'conflict', errorMessageId: messageId, errorOwner: owner } : {}) })
            return
          }
          throw new Error('feedback-write-unconfirmed')
        }
        const items = { ...this.state.items }
        if (method === 'PUT') {
          const item = MessageFeedbackItemV1Schema.parse(body)
          if (item.messageId !== messageId) throw new Error('feedback-owner-mismatch')
          items[messageId] = item
        } else {
          MessageFeedbackDeleteResponseV1Schema.parse(body)
          delete items[messageId]
        }
        this.publish({ items, pending: false, dialog: null, error: null, errorMessageId: null, errorOwner: null,
          acknowledgement: method === 'PUT' && this.owners.get(owner) ? { messageId, serial: ++this.serial, owner } : null })
      } catch {
        this.publish({ pending: false, ...(this.owners.get(owner)
          ? { error: 'save', errorMessageId: messageId, errorOwner: owner } : {}) })
      }
    })
  }
}

const controllers = new Map<string, MessageFeedbackController>()
export function messageFeedbackController(threadId: string): MessageFeedbackController {
  let controller = controllers.get(threadId)
  if (!controller) {
    controller = new MessageFeedbackController(threadId, rendererRuntimeClient, () => {
      if (controllers.get(threadId) === controller) controllers.delete(threadId)
    })
    controllers.set(threadId, controller)
  }
  return controller
}
