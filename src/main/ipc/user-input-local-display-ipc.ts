import { createHash } from 'node:crypto'
import { logUserInputLocalReadFailure, type UserInputLocalReadDiagnosticStage } from '../logger'
import type { Root, RootContent } from 'mdast'
import { parseGfmWithMath } from '../../shared/markdown/parse'
import { userInputLocalEffectRequestSchemaV1, type UserInputLocalEffectRequestV1, type UserInputLocalEffectResultV1 } from '../../shared/analytix-api'
import { clipboard, shell, ipcMain, type BrowserWindow, type IpcMainInvokeEvent } from 'electron'
import {
  USER_INPUT_LOCAL_DISPLAY_PATH_V1, USER_INPUT_LOCAL_DISPLAY_MAX_BYTES_V1,
  userInputLocalViewRequestSchemaV1, userInputLocalDisplaySchemaV1,
  userInputLocalDiscardLateReplyRequestSchemaV1, userInputLocalDiscardLateReplyResponseSchemaV1,
  userInputLocalSaveDraftRequestSchemaV1, userInputLocalSaveDraftResponseSchemaV1,
  type UserInputLocalViewResultV1, type UserInputLocalDiscardLateReplyResponseV1, type UserInputLocalDisplayV1, type UserInputLocalSelectorV1
} from '../../../packages/runtime/src/contracts/user-input-local-display'
import type { UserInputDraftSaveResultV1, UserInputLocalInvalidationReasonV1 } from '../../shared/analytix-api'
import { beginUserInputWaitTransitionV1 } from './user-input-wait-ipc'
import { captureNativeRendererDocument } from './native-renderer-document'

type View = { owner: Owner; selector: UserInputLocalSelectorV1; digest?: string; readOnly?: boolean }
type NativeEffects = { copy: (text: string) => void; openLink: (url: string) => Promise<void> }
function privateQuestionDigest(display: UserInputLocalDisplayV1): string {
  return createHash('sha256').update(JSON.stringify({ questions: display.questions, callId: display.callId ?? null,
    readOnly: display.readOnly === true, status: display.status, submission: display.submission ?? null, lateReply: display.lateReply ?? null, historyReply: display.historyReply ?? null })).digest('hex')
}
function replyClipboardText(display: UserInputLocalDisplayV1, locale: 'en' | 'zh'): string {
  return display.questions.map(question => {
    const submission = display.submission
    const legacy = submission?.format === 'legacy' ? submission.answers.find(answer => answer.questionId === question.id) : undefined
    const answer = submission?.format === 'structured' ? submission.answers.find(answer => answer.id === question.id) : undefined
    const values = legacy ? [legacy.value] : answer?.kind === 'answer'
      ? [...answer.optionIds.map(id => question.options.find(option => option.id === id)!.label), ...(answer.customText?.trim() ? [answer.customText.trim()] : [])] : []
    const heading = question.header && question.header !== question.question ? `${question.header} — ${question.question}` : question.question
    return `${heading}\n${values.length ? (locale === 'zh' ? '回答：' : 'Answer: ') + values.join(', ') : locale === 'zh' ? '已跳过' : 'Skipped'}`
  }).join('\n\n')
}
function detailEffectValue(display: UserInputLocalDisplayV1, effect: Exclude<UserInputLocalEffectRequestV1['effect'], { kind: 'copyReply' }>): string | null {
  const detail = display.questions.find(question => question.id === effect.questionId)?.detail
  if (detail === undefined || effect.end > detail.length) return null
  const root = parseGfmWithMath(detail), definitions = new Map<string, string>()
  let selected: RootContent | null = null
  const visit = (node: Root | RootContent): void => {
    if (node.type === 'definition' && !definitions.has(node.identifier.toUpperCase())) definitions.set(node.identifier.toUpperCase(), node.url)
    if (node.type !== 'root' && node.position?.start.offset === effect.start && node.position.end.offset === effect.end) selected = node
    if ('children' in node) for (const child of node.children) visit(child)
  }
  visit(root)
  const node = selected as RootContent | null
  if (effect.kind === 'copyCode') return node?.type === 'code' ? node.value : null
  const url = node?.type === 'link' ? node.url : node?.type === 'linkReference' ? definitions.get(node.identifier.toUpperCase())
    : node?.type === 'inlineCode' ? node.value.replace(/\r?\n|\r/g, ' ') : undefined
  if (!url || url.trim() !== url) return null
  try {
    const destination = new URL(url)
    const protocols = node?.type === 'inlineCode' ? ['http:', 'https:'] : ['http:', 'https:', 'mailto:']
    return protocols.includes(destination.protocol) ? destination.href : null
  } catch { return null }
}

type Owner = { isCurrent: () => boolean; release: () => void; signal: AbortSignal }
type Transport = (path: string, body: string, signal?: AbortSignal) => Promise<{ ok: boolean; status: number; body: string }>
const unavailable = (): UserInputLocalViewResultV1 => ({ ok: false, code: 'unavailable' })
const invalidators = new Set<(reason?: UserInputLocalInvalidationReasonV1) => void>()
let transitionDepth = 0
export function invalidateUserInputLocalDisplaysV1(reason: UserInputLocalInvalidationReasonV1 = 'authority'): void {
  for (const invalidate of [...invalidators]) {
    try { invalidate(reason) } catch { /* Retire every independent document lease. */ }
  }
}
// Composition attaches this to the existing native authority transition.
// It neither creates a grant nor retains private question/answer bytes.
export function beginUserInputLocalDisplayTransitionV1(): () => void {
  transitionDepth += 1
  const releaseWaits = beginUserInputWaitTransitionV1()
  invalidateUserInputLocalDisplaysV1()
  let active = true
  return () => {
    if (!active) return
    active = false
    try { invalidateUserInputLocalDisplaysV1() }
    finally { transitionDepth -= 1; releaseWaits() }
  }
}

export class UserInputLocalDisplayHostV1 {
  private views = new Map<string, View>()
  private commands = new Set<Owner>()
  private draftCommands = new Set<Owner>()
  constructor(private transport: Transport, private notify: (viewId: string | null, reason?: UserInputLocalInvalidationReasonV1) => void,
    private effects: NativeEffects = { copy: text => clipboard.writeText(text), openLink: url => shell.openExternal(url) }) {}
  private historyView(selector: UserInputLocalSelectorV1): boolean {
    return [...this.views.values()].some(view => view.readOnly && view.owner.isCurrent() && !view.owner.signal.aborted &&
      (['threadId', 'turnId', 'itemId', 'requestId'] as const).every(key => view.selector[key] === selector[key]))
  }
  close(viewId: string): void {
    const owner = this.views.get(viewId)?.owner
    this.views.delete(viewId)
    try { owner?.release() } catch { /* Already detached document. */ }
  }
  invalidate(reason: UserInputLocalInvalidationReasonV1 = 'authority'): void {
    for (const id of [...this.views.keys()]) this.close(id)
    for (const command of [...this.commands]) {
      // A save admitted while visible may finish for this same document after
      // ordinary backgrounding. Effects/discard and true authority retirement
      // retain their stricter lifetime.
      if (reason === 'background' && this.draftCommands.has(command)) continue
      try { command.release() } catch { /* Retired document. */ }
      this.commands.delete(command); this.draftCommands.delete(command)
    }
    try { this.notify(null, reason) } catch { /* Closed renderer cannot receive cleanup. */ }
  }
  async effect(payload: unknown, owner: Owner): Promise<UserInputLocalEffectResultV1> {
    const parsed = userInputLocalEffectRequestSchemaV1.safeParse(payload)
    const fail = (): UserInputLocalEffectResultV1 => ({ ok: false, code: 'unavailable' })
    const view = parsed.success ? this.views.get(parsed.data.viewId) : undefined
    if (!parsed.success || !view?.digest || Object.entries(parsed.data.selector).some(([key, value]) => view.selector[key as keyof UserInputLocalSelectorV1] !== value) ||
        !owner.isCurrent() || owner.signal.aborted || !view.owner.isCurrent() || view.owner.signal.aborted || this.commands.size >= 32) {
      owner.release(); return fail()
    }
    const request = parsed.data
    this.commands.add(owner)
    const current = () => this.commands.has(owner) && this.views.get(request.viewId) === view && owner.isCurrent() && !owner.signal.aborted && view.owner.isCurrent() && !view.owner.signal.aborted
    const fresh = async (): Promise<UserInputLocalDisplayV1 | null> => {
      const response = await this.transport(USER_INPUT_LOCAL_DISPLAY_PATH_V1, JSON.stringify(view.selector), owner.signal)
      if (!current() || !response.ok || response.status !== 200 || Buffer.byteLength(response.body, 'utf8') > USER_INPUT_LOCAL_DISPLAY_MAX_BYTES_V1) return null
      const display = userInputLocalDisplaySchemaV1.safeParse(JSON.parse(response.body))
      return display.success && Object.entries(view.selector).every(([key, value]) => display.data[key as keyof UserInputLocalSelectorV1] === value) &&
        privateQuestionDigest(display.data) === view.digest && current() ? display.data : null
    }
    try {
      const first = await fresh()
      if (!first) return fail()
      const effect = request.effect
      const reply = first.historyReply ?? (first.lateReply?.state === 'accepted' ? first.lateReply : null)
      if (effect.kind === 'copyReply' && (!first.callId || !first.submission || !reply ||
          reply.clientUserMessageId !== effect.clientUserMessageId || reply.turnId !== effect.expectedTurnId)) return fail()
      const text = effect.kind === 'copyReply' ? replyClipboardText(first, effect.locale) : detailEffectValue(first, effect)
      if (text === null || !await fresh() || !current()) return fail()
      if (effect.kind === 'openDetailLink') { await this.effects.openLink(text); if (!current()) return fail() }
      else this.effects.copy(text)
      return { ok: true }
    } catch { return fail() }
    finally { if (this.commands.delete(owner)) owner.release() }
  }
  async discard(payload: unknown, owner: Owner): Promise<UserInputLocalDiscardLateReplyResponseV1> {
    const parsed = userInputLocalDiscardLateReplyRequestSchemaV1.safeParse(payload)
    const failure = () => new Error('Question discard unavailable.')
    if (!parsed.success || this.historyView(parsed.data) || !owner.isCurrent() || owner.signal.aborted || this.commands.size >= 32) {
      owner.release(); throw failure()
    }
    this.commands.add(owner)
    try {
      const response = await this.transport(USER_INPUT_LOCAL_DISPLAY_PATH_V1, JSON.stringify(parsed.data), owner.signal)
      if (!this.commands.has(owner) || !owner.isCurrent() || owner.signal.aborted || !response.ok || response.status !== 200 || Buffer.byteLength(response.body, 'utf8') > 4096) throw failure()
      const result = userInputLocalDiscardLateReplyResponseSchemaV1.safeParse(JSON.parse(response.body))
      if (!result.success || ['threadId', 'turnId', 'itemId', 'requestId'].some(key => result.data[key as 'threadId' | 'turnId' | 'itemId' | 'requestId'] !== parsed.data[key as 'threadId' | 'turnId' | 'itemId' | 'requestId']) ||
          result.data.lateReply.clientUserMessageId !== parsed.data.clientUserMessageId || result.data.lateReply.turnId !== parsed.data.expectedTurnId || !owner.isCurrent() || owner.signal.aborted) throw failure()
      return result.data
    } catch { throw failure() }
    finally { if (this.commands.delete(owner)) { try { owner.release() } catch { /* Already retired. */ } } }
  }
  async saveDraft(payload: unknown, owner: Owner): Promise<UserInputDraftSaveResultV1> {
    const parsed = userInputLocalSaveDraftRequestSchemaV1.safeParse(payload)
    const unavailable = (): UserInputDraftSaveResultV1 => ({ ok: false, code: 'unavailable' })
    if (!parsed.success || this.historyView(parsed.data) || !owner.isCurrent() || owner.signal.aborted || this.commands.size >= 32) {
      try { owner.release() } catch { /* Already retired document. */ }
      return parsed.success ? unavailable() : { ok: false, code: 'invalid' }
    }
    this.commands.add(owner); this.draftCommands.add(owner)
    try {
      const response = await this.transport(USER_INPUT_LOCAL_DISPLAY_PATH_V1, JSON.stringify(parsed.data), owner.signal)
      const current = () => this.commands.has(owner) && owner.isCurrent() && !owner.signal.aborted
      if (!current()) return unavailable()
      if (!response.ok) {
        const code = response.status === 400 ? 'invalid' : response.status === 404 ? 'retired'
          : response.status === 409 ? 'conflict' : response.status === 413 ? 'capacity' : 'unavailable'
        return { ok: false, code }
      }
      if (response.status !== 200 || Buffer.byteLength(response.body, 'utf8') > USER_INPUT_LOCAL_DISPLAY_MAX_BYTES_V1) return unavailable()
      const receipt = userInputLocalSaveDraftResponseSchemaV1.safeParse(JSON.parse(response.body))
      if (!receipt.success || ['threadId', 'turnId', 'itemId', 'requestId'].some(key =>
        receipt.data[key as 'threadId' | 'turnId' | 'itemId' | 'requestId'] !== parsed.data[key as 'threadId' | 'turnId' | 'itemId' | 'requestId']) ||
        receipt.data.draftRevision !== parsed.data.revision || JSON.stringify(receipt.data.draft) !== JSON.stringify(parsed.data.draft) || !current()) return unavailable()
      return { ok: true, receipt: receipt.data }
    } catch { return unavailable() }
    finally { this.draftCommands.delete(owner); if (this.commands.delete(owner)) { try { owner.release() } catch { /* Already retired. */ } } }
  }
  async open(payload: unknown, owner: Owner): Promise<UserInputLocalViewResultV1> {
    const parsed = userInputLocalViewRequestSchemaV1.safeParse(payload)
    if (!parsed.success) { logUserInputLocalReadFailure('request_admission',null); owner.release(); return unavailable() }
    if (!owner.isCurrent() || owner.signal.aborted) { logUserInputLocalReadFailure('owner_admission',null); owner.release(); return unavailable() }
    const { viewId, ...selector } = parsed.data
    this.close(viewId)
    if (this.views.size >= 32) { logUserInputLocalReadFailure('view_capacity',null); owner.release(); return unavailable() }
    const view: View = { owner, selector }
    this.views.set(viewId, view)
    const current = () => this.views.get(viewId) === view && owner.isCurrent() && !owner.signal.aborted
    let stage: UserInputLocalReadDiagnosticStage = 'runtime_request', statusCode: number | null = null
    const fail = () => {
      logUserInputLocalReadFailure(stage,statusCode)
      if (this.views.get(viewId) === view) {
        this.close(viewId)
        try { this.notify(viewId) } catch { /* Retired renderer. */ }
      }
      return unavailable()
    }
    try {
      const response = await this.transport(USER_INPUT_LOCAL_DISPLAY_PATH_V1, JSON.stringify(selector), owner.signal)
      statusCode = response.status
      stage = 'response_owner'; if (!current()) return fail()
      stage = 'response_http'; if (!response.ok || response.status !== 200) return fail()
      stage = 'response_limit'; if (Buffer.byteLength(response.body, 'utf8') > USER_INPUT_LOCAL_DISPLAY_MAX_BYTES_V1) return fail()
      stage = 'response_json'; const body: unknown = JSON.parse(response.body)
      stage = 'response_schema'; const display = userInputLocalDisplaySchemaV1.safeParse(body)
      if (!display.success) return fail()
      stage = 'response_selector'; if (Object.entries(selector).some(([key, value]) => display.data[key as keyof typeof selector] !== value)) return fail()
      stage = 'display_owner'; if (!current()) return fail()
      stage = 'display_digest'; view.digest = privateQuestionDigest(display.data)
      view.readOnly = display.data.readOnly === true
      return { ok: true, display: display.data }
    } catch { return fail() }
  }
}

export function registerUserInputLocalDisplayIpcV1(getMainWindow: () => BrowserWindow | null, transport: Transport): void {
  const hosts = new Map<number, UserInputLocalDisplayHostV1>()
  const observedWindows = new WeakSet<BrowserWindow>()
  const current = (event: IpcMainInvokeEvent): boolean => {
    const window = getMainWindow()
    return Boolean(transitionDepth === 0 && window && !window.isDestroyed() && window.isVisible() && !window.isMinimized() &&
      window.webContents === event.sender && !event.sender.isDestroyed() && event.senderFrame != null && event.senderFrame === event.sender.mainFrame)
  }
  const currentDocument = (event: IpcMainInvokeEvent): boolean => {
    const window = getMainWindow()
    return Boolean(transitionDepth === 0 && window && !window.isDestroyed() &&
      window.webContents === event.sender && !event.sender.isDestroyed() &&
      event.senderFrame != null && event.senderFrame === event.sender.mainFrame)
  }
  const hostFor = (event: IpcMainInvokeEvent) => {
    const window = getMainWindow()
    if (window && !observedWindows.has(window)) {
      observedWindows.add(window)
      const background = () => invalidateUserInputLocalDisplaysV1('background')
      window.on('hide', background)
      window.on('minimize', background)
      window.once('closed', () => {
        window.removeListener('hide', background)
        window.removeListener('minimize', background)
      })
    }
    let host = hosts.get(event.sender.id)
    if (!host) {
      host = new UserInputLocalDisplayHostV1(transport, (viewId, reason) => {
        if (!event.sender.isDestroyed()) event.sender.send('user-input-local:invalidated', viewId, reason)
      })
      hosts.set(event.sender.id, host)
      const invalidate = (reason?: UserInputLocalInvalidationReasonV1) => host!.invalidate(reason)
      invalidators.add(invalidate)
      event.sender.once('destroyed', () => { invalidate(); invalidators.delete(invalidate); hosts.delete(event.sender.id) })
    }
    return host
  }
  ipcMain.handle('user-input-local:open', async (event, payload: unknown) => {
    if (!current(event)) { logUserInputLocalReadFailure('ipc_authority',null); return unavailable() }
    const host = hostFor(event)
    const controller = new AbortController()
    const lease = captureNativeRendererDocument(event.sender, () => current(event), () => { controller.abort(); host.invalidate() })
    return host.open(payload, { signal: controller.signal, isCurrent: lease.isCurrent,
      release: () => { controller.abort(); lease.release() } })
  })
  ipcMain.handle('user-input-local:effect', async (event, payload: unknown) => {
    if (!current(event)) return { ok: false, code: 'unavailable' }
    const host = hostFor(event), controller = new AbortController()
    const lease = captureNativeRendererDocument(event.sender, () => current(event), () => { controller.abort(); host.invalidate() })
    return host.effect(payload, { signal: controller.signal, isCurrent: lease.isCurrent,
      release: () => { controller.abort(); lease.release() } })
  })
  ipcMain.handle('user-input-local:discard-late-reply', async (event, payload: unknown) => {
    if (!current(event)) throw new Error('Question discard unavailable.')
    const host = hostFor(event), controller = new AbortController()
    const lease = captureNativeRendererDocument(event.sender, () => current(event), () => { controller.abort(); host.invalidate() })
    return host.discard(payload, { signal: controller.signal, isCurrent: lease.isCurrent,
      release: () => { controller.abort(); lease.release() } })
  })
  ipcMain.handle('user-input-local:save-draft', async (event, payload: unknown) => {
    if (!current(event)) return { ok: false, code: 'unavailable' }
    const host = hostFor(event), controller = new AbortController()
    const lease = captureNativeRendererDocument(event.sender, () => currentDocument(event), () => { controller.abort(); host.invalidate() })
    return host.saveDraft(payload, { signal: controller.signal, isCurrent: lease.isCurrent,
      release: () => { controller.abort(); lease.release() } })
  })
  ipcMain.handle('user-input-local:close', (event, viewId: unknown) => {
    if (!current(event) || typeof viewId !== 'string') throw new Error('Question display authority unavailable.')
    hosts.get(event.sender.id)?.close(viewId)
    event.sender.send('user-input-local:invalidated', viewId)
  })
  ipcMain.handle('user-input-local:invalidate', event => {
    if (!current(event)) throw new Error('Question display authority unavailable.')
    invalidateUserInputLocalDisplaysV1()
  })
}
