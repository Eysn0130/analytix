import { clipboard, dialog, ipcMain, shell, type BrowserWindow, type IpcMainInvokeEvent } from 'electron'
import { lstat, open, realpath, rename, stat, unlink, writeFile } from 'node:fs/promises'
import { constants, type BigIntStats } from 'node:fs'
import { createHash, randomUUID } from 'node:crypto'
import { basename, dirname, extname, isAbsolute, join, normalize } from 'node:path'
import { listPresentFileApplications, openPresentFileApplication } from '../services/workspace-editors'
import type { GeneratedArtifactResponse } from '../../../packages/runtime/src/contracts/generated-artifact'
import type { WorkspaceFileReadResult, WorkspaceImageReadResult, WorkspacePdfReadResult, WorkspaceExcelReadResult } from '../../shared/workspace-file'
import { resolveWorkspaceFile } from '../services/workspace-files'
import {
  TOOL_RESULT_LOCAL_DISPLAY_PATH_V1, TOOL_RESULT_LOCAL_DISPLAY_MAX_BYTES_V1, TOOL_RESULT_LOCAL_WEB_DISPLAY_MAX_BYTES_V1,
  TOOL_RESULT_LOCAL_TODO_DISPLAY_MAX_BYTES_V1, TOOL_RESULT_LOCAL_PRESENT_SAVE_MAX_BYTES_V1,
  toolResultLocalPresentHistoryViewRequestSchemaV1, toolResultLocalPresentHistorySchemaV1,
  type ToolResultLocalPresentHistoryViewResultV1,
  toolResultLocalHostDisplaySchemaV1, toolResultLocalViewRequestSchemaV1,
  toolResultLocalSaveRequestSchemaV1, toolResultLocalEffectRequestSchemaV1, toolResultLocalCopyRequestSchemaV1, toolResultLocalPresentExcelCopyRequestSchemaV1, toolResultLocalWebLinkRequestSchemaV1,
  toolResultLocalPresentFileRequestSchemaV1, toolResultLocalPresentOpenRequestSchemaV1, toolResultLocalPresentAssociationRequestSchemaV1,
  type ToolResultLocalHostDisplayV1, type ToolResultLocalSelectorV1,
  type ToolResultLocalPresentAssociationResultV1, type ToolResultLocalNavigateResultV1, type ToolResultLocalPresentFileRequestV1, type ToolResultLocalPresentApplicationsResultV1,
  type ToolResultLocalViewResultV1, type ToolResultLocalEffectResultV1, type ToolResultLocalPresentExcelCopyRequestV1
} from '../../../packages/runtime/src/contracts/tool-result-local-display'
import { beginUserInputLocalDisplayTransitionV1, invalidateUserInputLocalDisplaysV1 } from './user-input-local-display-ipc'
import { captureNativeRendererDocument } from './native-renderer-document'
import { projectOrdinaryToolExportTextV1 as ordinaryToolExportTextV1 } from '../services/write-export-content'
import { projectExcelCopyContentV1 } from '../services/excel-preview-copy-html'
import { excelFormat, EXCEL_PREVIEW_LIMITS, type ExcelFormat } from '../../shared/excel/format'
import { validateExcelCopyFrameV1, type ExcelCopyFrameV1, type ExcelPreviewCopyReaderV1 } from '../services/excel-preview-copy-service'

type Owner = { isCurrent: () => boolean; release: () => void; signal: AbortSignal }
type View = { selector: ToolResultLocalSelectorV1; digest?: string; owner: Owner }
type HistoryView = { threadId: string; turnId: string; owner: Owner }
type SelectedPresentFile = { target: { index: number; path: string }; workspaceRoot: string; observed: BigIntStats;
  guard: () => Promise<boolean>; isCurrent: () => boolean }
type PresentPreviewBytes = { path: string; bytes: Buffer; size: number; mtimeMs: number; truncated: boolean; mimeType?: string; excelFormat?: ExcelFormat; isCurrent: () => boolean; guard: () => Promise<boolean> }
const samePresentFileVersion = (left: BigIntStats, right: BigIntStats): boolean =>
  left.isFile() && right.isFile() && left.dev === right.dev && left.ino === right.ino && left.birthtimeNs === right.birthtimeNs &&
  left.size === right.size && left.mtimeNs === right.mtimeNs && left.ctimeNs === right.ctimeNs
export type PresentHtmlSnapshot = PresentPreviewBytes & {
  workspaceRoot: string; digest: string; signal: AbortSignal
}
export type PresentOfficeSnapshot = PresentHtmlSnapshot & {
  kind: 'docx' | 'pptx'; selector: ToolResultLocalPresentFileRequestV1
}
type ProtectedPreviewReader = ((event: IpcMainInvokeEvent, request: unknown) => Promise<WorkspaceFileReadResult>) & {
  image: (event: IpcMainInvokeEvent, request: unknown) => Promise<WorkspaceImageReadResult>
  pdf: (event: IpcMainInvokeEvent, request: unknown) => Promise<WorkspacePdfReadResult>
  excel: (event: IpcMainInvokeEvent, request: unknown) => Promise<WorkspaceExcelReadResult>
  html: (event: IpcMainInvokeEvent, request: unknown) => Promise<PresentHtmlSnapshot | null>
  office: (event: IpcMainInvokeEvent, request: unknown) => Promise<PresentOfficeSnapshot | null>
}
const presentImageMimeTypes = new Map([
  ['.png', 'image/png'], ['.jpg', 'image/jpeg'], ['.jpeg', 'image/jpeg'], ['.gif', 'image/gif'],
  ['.webp', 'image/webp'], ['.bmp', 'image/bmp'], ['.ico', 'image/x-icon'], ['.svg', 'image/svg+xml']
])
type Transport = (path: string, body: string, signal?: AbortSignal) => Promise<{ ok: boolean; status: number; body: string }>
// Main-only effect sink. HTML never becomes renderer request or reply authority.
type ToolResultLocalCopySinkV1 = (text: string, checkedHtml?: string) => void
type PresentSaveGuard = { check: () => Promise<boolean>; isCurrent: () => boolean }
type ToolResultLocalSaveSinkV1 = (path: string, contents: string | Uint8Array, guard?: PresentSaveGuard) => Promise<void>
// Stage complete original bytes beside the chosen destination, then recheck
// before the atomic commit. A revoked/failed export leaves the destination intact.
export const saveToolResultLocalFileV1: ToolResultLocalSaveSinkV1 = async (path, contents, guard) => {
  if (!guard) {
    if (typeof contents !== 'string') throw new Error('Protected file save unavailable.')
    await writeFile(path, contents, { encoding: 'utf8', mode: 0o600 })
    return
  }
  if (!await guard.check() || !guard.isCurrent()) throw new Error('Protected file save unavailable.')
  const temporaryPath = join(dirname(path), `.analytix-save-${randomUUID()}.tmp`)
  const handle = await open(temporaryPath, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL, 0o600)
  try {
    try { await handle.writeFile(contents) } finally { await handle.close() }
    if (!await guard.check() || !guard.isCurrent()) throw new Error('Protected file save unavailable.')
    // Native rename is the commit boundary; no await separates the last lease
    // check from dispatch. Subsequent retirement cannot undo a completed save.
    await rename(temporaryPath, path)
  } finally {
    await unlink(temporaryPath).catch(error => { if (error?.code !== 'ENOENT') throw error })
  }
}
const invalidators = new Set<() => void>()
let hostTransitionDepth = 0
export async function withToolResultLocalDisplayTransitionV1<T>(operation: () => Promise<T>): Promise<T> {
  hostTransitionDepth += 1
  const releaseQuestions = beginUserInputLocalDisplayTransitionV1()
  try { invalidateToolResultLocalDisplaysV1(); return await operation() }
  finally {
    try { invalidateToolResultLocalDisplaysV1() }
    finally { hostTransitionDepth -= 1; releaseQuestions() }
  }
}
export function invalidateToolResultLocalDisplaysV1(reason: 'background' | 'authority' = 'authority'): void {
  invalidateUserInputLocalDisplaysV1(reason)
  for (const invalidate of [...invalidators]) {
    try { invalidate() } catch { /* Continue retiring independent document owners. */ }
  }
}
const unavailable = (): { ok: false; code: 'unavailable' } => ({ ok: false, code: 'unavailable' })
const selectorOf = ({ threadId, turnId, callId, resultItemId }: ToolResultLocalSelectorV1): ToolResultLocalSelectorV1 =>
  ({ threadId, turnId, callId, resultItemId })

// Retains only document leases, identities and immutable hashes; no body cache.
export class ToolResultLocalDisplayHostV1 {
  private views = new Map<string, View>()
  private historyViews = new Map<string, HistoryView>()
  private excelCopyPending = false
  // Ephemeral native object identities are scoped to this document's current
  // leases. Neither path labels nor content hashes determine file equality.
  private presentSlots = new Map<string, string>()
  private presentObjects = new Map<string, { id: string; views: Set<string> }>()
  constructor(private transport: Transport, private notify: (viewId?: string) => void,
    private readExcelCells?: ExcelPreviewCopyReaderV1) {}
  close(viewId: string): void {
    const view = this.views.get(viewId) ?? this.historyViews.get(viewId)
    this.views.delete(viewId); this.historyViews.delete(viewId)
    for (const [key, object] of this.presentObjects) {
      for (const slot of object.views) if (slot.startsWith(viewId + ':')) { object.views.delete(slot); this.presentSlots.delete(slot) }
      if (!object.views.size) this.presentObjects.delete(key)
    }
    try { view?.owner.release() } catch { /* The lease is already detached. */ }
  }
  invalidate(): void {
    for (const id of [...this.views.keys(), ...this.historyViews.keys()]) this.close(id)
    this.notifySafely()
  }
  private notifySafely(viewId?: string): void {
    try { this.notify(viewId) } catch { /* A closed document cannot acknowledge cleanup. */ }
  }
  private fail(viewId: string, expected: View): { ok: false; code: 'unavailable' } {
    if (this.views.get(viewId) !== expected) return unavailable()
    this.close(viewId)
    this.notifySafely(viewId)
    return unavailable()
  }
  private current(viewId: string, view: View): boolean {
    return this.views.get(viewId) === view && view.owner.isCurrent() && !view.owner.signal.aborted
  }
  private async fresh(viewId: string, view: View): Promise<ToolResultLocalHostDisplayV1 | null> {
    if (!this.current(viewId, view)) return null
    try {
      const response = await this.transport(TOOL_RESULT_LOCAL_DISPLAY_PATH_V1, JSON.stringify(view.selector), view.owner.signal)
      if (!this.current(viewId, view) || !response.ok || response.status !== 200 ||
          Buffer.byteLength(response.body, 'utf8') > TOOL_RESULT_LOCAL_WEB_DISPLAY_MAX_BYTES_V1) return null
      const parsed = toolResultLocalHostDisplaySchemaV1.safeParse(JSON.parse(response.body))
      if (!parsed.success || Buffer.byteLength(response.body, 'utf8') >
          (parsed.data.capture.kind === 'web_fetch' ? TOOL_RESULT_LOCAL_WEB_DISPLAY_MAX_BYTES_V1 :
            parsed.data.capture.kind === 'todo' ? TOOL_RESULT_LOCAL_TODO_DISPLAY_MAX_BYTES_V1 : TOOL_RESULT_LOCAL_DISPLAY_MAX_BYTES_V1) ||
          parsed.data.resultItemId !== view.selector.resultItemId ||
          view.digest !== undefined && parsed.data.snapshotDigest !== view.digest) return null
      return parsed.data
    } catch { return null }
  }
  async open(payload: unknown, owner: Owner): Promise<ToolResultLocalViewResultV1> {
    const request = toolResultLocalViewRequestSchemaV1.safeParse(payload)
    if (!request.success || !owner.isCurrent()) { owner.release(); return unavailable() }
    const { viewId } = request.data
    this.close(viewId)
    if (this.views.size >= 32) { owner.release(); return unavailable() }
    const view: View = { selector: selectorOf(request.data), owner }
    this.views.set(viewId, view)
    const display = await this.fresh(viewId, view)
    if (!display || !this.current(viewId, view)) return this.fail(viewId, view)
    view.digest = display.snapshotDigest
    // Qualify only the existing bounded preview bytes, without hashing an
    // entire document on card expansion. Clicks still verify the full digest.
    const canNavigate = display.readTarget !== undefined && Boolean(await this.readTarget(viewId, view, display.readTarget, false))
    if (!this.current(viewId, view)) return this.fail(viewId, view)
    const { schemaVersion, snapshotDigest, resultItemId, toolName, capture, inspect } = display
    return { ok: true, display: { schemaVersion, snapshotDigest, resultItemId, toolName, capture, canNavigate,
      ...(inspect ? { inspect } : {}),
      ...(capture.kind === 'web_fetch' ? { canOpenWeb: {
        request: display.webTargets?.requestUrl !== undefined, result: display.webTargets?.resultUrl !== undefined
      } } : {}) } }
  }
  async openPresentHistory(payload: unknown, owner: Owner): Promise<ToolResultLocalPresentHistoryViewResultV1> {
    const request = toolResultLocalPresentHistoryViewRequestSchemaV1.safeParse(payload)
    if (!request.success || !owner.isCurrent() || owner.signal.aborted) { owner.release(); return unavailable() }
    const { viewId, threadId, turnId } = request.data
    this.close(viewId)
    if (this.historyViews.size >= 32) { owner.release(); return unavailable() }
    const view: HistoryView = { threadId, turnId, owner }
    this.historyViews.set(viewId, view)
    const current = () => this.historyViews.get(viewId) === view && owner.isCurrent() && !owner.signal.aborted
    try {
      const response = await this.transport(TOOL_RESULT_LOCAL_DISPLAY_PATH_V1,
        JSON.stringify({ kind: 'present_history', threadId, turnId }), owner.signal)
      if (current() && response.ok && response.status === 200 &&
        Buffer.byteLength(response.body, 'utf8') <= TOOL_RESULT_LOCAL_DISPLAY_MAX_BYTES_V1) {
        const parsed = toolResultLocalPresentHistorySchemaV1.safeParse(JSON.parse(response.body))
        if (parsed.success && parsed.data.threadId === threadId && parsed.data.turnId === turnId && current()) {
          // History is an identity discovery read with no effects. Release the
          // document read lease; mounted consumers still retire on the shared
          // global authority lifecycle and acquire separate result leases.
          this.close(viewId)
          return { ok: true, history: parsed.data }
        }
      }
    } catch { /* Authority, transport and malformed history retire the same lease. */ }
    if (this.historyViews.get(viewId) === view) { this.close(viewId); this.notifySafely(viewId) }
    return unavailable()
  }
  // Safe opening verifies actual bytes, not an independently observed path/stat.
  // Temporary descriptor identity checks are not a persisted inode protocol.
  private async readTarget(viewId: string, view: View, target: NonNullable<ToolResultLocalHostDisplayV1['readTarget']>, verifyDigest = true): Promise<WorkspaceFileReadResult | null> {
    if (!this.current(viewId, view) || !constants.O_NOFOLLOW) return null
    try {
      const resolved = await resolveWorkspaceFile({ path: target.path, workspaceRoot: target.workspaceRoot })
      if (!resolved.ok || resolved.path !== target.path || !this.current(viewId, view) || await realpath(target.path) !== target.path) return null
      const handle = await open(target.path, constants.O_RDONLY | constants.O_NOFOLLOW)
      try {
        const before = await handle.stat({ bigint: true })
        if (!before.isFile() || !this.current(viewId, view)) return null
        const hash = createHash('sha256'), chunk = Buffer.alloc(64 << 10)
        const previewChunks: Buffer[] = []
        let previewBytes = 0
        // Same bounded bytes and truncation behavior as the existing workspace preview.
        const maxPreviewBytes = 1_500_000
        for (;;) {
          const length = verifyDigest ? chunk.length : Math.min(chunk.length, maxPreviewBytes - previewBytes)
          if (!length) break
          const { bytesRead } = await handle.read(chunk, 0, length, null)
          if (!this.current(viewId, view)) return null
          if (!bytesRead) break
          if (verifyDigest) hash.update(chunk.subarray(0, bytesRead))
          const shown = Math.min(bytesRead, maxPreviewBytes - previewBytes)
          if (shown > 0) { previewChunks.push(Buffer.from(chunk.subarray(0, shown))); previewBytes += shown }
        }
        const after = await handle.stat({ bigint: true }), linked = await stat(target.path, { bigint: true })
        const valid = [after, linked].every(info => info.isFile() && info.dev === before.dev && info.ino === before.ino &&
          info.size === before.size && info.mtimeNs === before.mtimeNs && info.ctimeNs === before.ctimeNs) &&
          await realpath(target.path) === target.path && (!verifyDigest || hash.digest('hex') === target.contentDigest) && this.current(viewId, view)
        const bytes = Buffer.concat(previewChunks, previewBytes)
        if (!valid || bytes.includes(0)) return null
        // The source preview supports UTF-8 only. A truncated final sequence is
        // allowed at the existing byte cap; UTF-16/invalid text stays unavailable.
        new TextDecoder('utf-8', { fatal: true }).decode(bytes, { stream: before.size > BigInt(maxPreviewBytes) })
        return { ok: true, path: target.path, content: bytes.toString('utf8'), size: Number(before.size), truncated: before.size > BigInt(maxPreviewBytes), line: target.startLine }
      } finally { await handle.close() }
    } catch { return null }
  }
  async navigate(payload: unknown, current: () => boolean): Promise<ToolResultLocalNavigateResultV1> {
    const request = toolResultLocalEffectRequestSchemaV1.safeParse(payload)
    if (!request.success || !current()) return unavailable()
    const { viewId, snapshotDigest } = request.data
    const view = this.views.get(viewId)
    if (!view || view.digest !== snapshotDigest || JSON.stringify(view.selector) !== JSON.stringify(selectorOf(request.data))) return unavailable()
    const first = await this.fresh(viewId, view)
    if (!first?.readTarget || !current() || !await this.readTarget(viewId, view, first.readTarget)) return this.fail(viewId, view)
    const final = await this.fresh(viewId, view)
    if (!final?.readTarget || JSON.stringify(final.readTarget) !== JSON.stringify(first.readTarget) ||
        !current() || !await this.readTarget(viewId, view, final.readTarget) || !current() || !this.current(viewId, view)) return this.fail(viewId, view)
    return { ok: true, target: { path: final.readTarget.path, workspaceRoot: final.readTarget.workspaceRoot, line: final.readTarget.startLine } }
  }
  async openWebLink(
    payload: unknown, current: () => boolean, openExternal: (url: string) => Promise<void>
  ): Promise<ToolResultLocalEffectResultV1> {
    const request = toolResultLocalWebLinkRequestSchemaV1.safeParse(payload)
    if (!request.success || !current()) return unavailable()
    const { viewId, snapshotDigest, targetRole } = request.data
    const view = this.views.get(viewId)
    if (!view || view.digest !== snapshotDigest || JSON.stringify(view.selector) !== JSON.stringify(selectorOf(request.data))) return unavailable()
    const first = await this.fresh(viewId, view)
    const targetOf = (display: ToolResultLocalHostDisplayV1 | null): string | undefined => display?.capture.kind === 'web_fetch'
      ? targetRole === 'request' ? display.webTargets?.requestUrl : display.webTargets?.resultUrl : undefined
    const target = targetOf(first)
    if (target === undefined || !current()) return this.fail(viewId, view)
    const final = await this.fresh(viewId, view)
    if (targetOf(final) !== target || !current() || !this.current(viewId, view)) return this.fail(viewId, view)
    // This is a sealed Host target from two fresh authorized reads. No renderer
    // URL, display label or projected prefix is accepted as effect authority.
    try {
      const url = new URL(target)
      if (url.protocol !== 'http:' && url.protocol !== 'https:') return this.fail(viewId, view)
      await openExternal(target)
      return { ok: true }
    } catch { return unavailable() }
  }
  async preview(payload: unknown, current: () => boolean): Promise<WorkspaceFileReadResult> {
    const denied: WorkspaceFileReadResult = { ok: false, message: 'Protected file preview unavailable.' }
    const request = toolResultLocalEffectRequestSchemaV1.safeParse(payload)
    if (!request.success || !current()) return denied
    const { viewId, snapshotDigest } = request.data
    const view = this.views.get(viewId)
    if (!view || view.digest !== snapshotDigest || JSON.stringify(view.selector) !== JSON.stringify(selectorOf(request.data))) return denied
    const first = await this.fresh(viewId, view)
    if (!first?.readTarget || !current()) { this.fail(viewId, view); return denied }
    const read = await this.readTarget(viewId, view, first.readTarget)
    if (!read || !current()) { this.fail(viewId, view); return denied }
    // The buffer came from the verified descriptor, not a subsequent pathname
    // read. Release it only after the same snapshot and authority are fresh.
    const final = await this.fresh(viewId, view)
    if (!final?.readTarget || JSON.stringify(final.readTarget) !== JSON.stringify(first.readTarget) ||
        !current() || !this.current(viewId, view)) { this.fail(viewId, view); return denied }
    return read
  }
  private async selectedPresentFile(request: ToolResultLocalPresentFileRequestV1, current: () => boolean): Promise<SelectedPresentFile | null> {
    const { viewId, snapshotDigest, index } = request
    const view = this.views.get(viewId)
    if (!view || view.digest !== snapshotDigest || JSON.stringify(view.selector) !== JSON.stringify(selectorOf(request)) || !current()) return null
    const first = await this.fresh(viewId, view)
    const targets = first?.presentTargets, target = targets?.files[index]
    if (first?.capture.kind !== 'present' || !targets || !target || target.index !== index || !current()) {
      this.fail(viewId, view); return null
    }
    try {
      if (!isAbsolute(target.path) || normalize(target.path) !== target.path || await realpath(target.path) !== target.path) return null
      const observed = await lstat(target.path, { bigint: true })
      if (!observed.isFile() || !current() || !this.current(viewId, view)) return null
      const guard = async () => {
        if (!current() || !this.current(viewId, view)) return false
        const fresh = await this.fresh(viewId, view)
        const freshTargets = fresh?.presentTargets
        if (fresh?.capture.kind !== 'present' || !freshTargets || freshTargets.workspaceRoot !== targets.workspaceRoot ||
          JSON.stringify(freshTargets.files[index]) !== JSON.stringify(target) || !current()) return false
        try {
          if (await realpath(target.path) !== target.path) return false
          // The last filesystem await checks the same observed object. Native
          // pathname dispatch still has the operating system's ordinary race.
          const linked = await lstat(target.path, { bigint: true })
          return linked.isFile() && linked.dev === observed.dev && linked.ino === observed.ino && linked.birthtimeNs === observed.birthtimeNs && current() && this.current(viewId, view)
        } catch { return false }
      }
      return { target, workspaceRoot: targets.workspaceRoot, observed, guard, isCurrent: () => current() && this.current(viewId, view) }
    } catch { return null }
  }
  async associatePresent(payload: unknown, current: () => boolean,
    resolveArtifact?: (payload: unknown) => Promise<GeneratedArtifactResponse>): Promise<ToolResultLocalPresentAssociationResultV1> {
    const request = toolResultLocalPresentAssociationRequestSchemaV1.safeParse(payload)
    if (!request.success) return unavailable()
    const selected = await this.selectedPresentFile(request.data, current)
    if (!selected || !await selected.guard()) return unavailable()
    const artifactIds: string[] = []
    if (resolveArtifact) for (const artifactId of request.data.artifactIds) {
      if (!await selected.guard()) return unavailable()
      try {
        const response = await resolveArtifact({ threadId: request.data.threadId, artifactId })
        if (!await selected.guard()) return unavailable()
        if (!response.ok) continue
        const artifact = response.artifact
        if (artifact.threadId !== request.data.threadId || artifact.artifactId !== artifactId ||
          artifact.workspace !== selected.workspaceRoot || artifact.path !== selected.target.path ||
          await realpath(artifact.path) !== artifact.path) continue
        const identity = await lstat(artifact.path, { bigint: true })
        if (identity.isFile() && identity.dev === selected.observed.dev && identity.ino === selected.observed.ino && identity.birthtimeNs === selected.observed.birthtimeNs) artifactIds.push(artifactId)
      } catch { /* An unknown Office receipt retains its existing fallback. */ }
    }
    if (!await selected.guard()) return unavailable()
    const slot = `${request.data.viewId}:${request.data.index}`
    // Creation time separates inode reuse on the native filesystem. Where no
    // creation time exists, expose no cross-lease equality claim.
    const key = selected.observed.birthtimeNs > 0n ? `${selected.observed.dev}:${selected.observed.ino}:${selected.observed.birthtimeNs}` : `${slot}:${selected.observed.dev}:${selected.observed.ino}`
    const previous = this.presentSlots.get(slot)
    if (previous && previous !== key) {
      const old = this.presentObjects.get(previous)
      old?.views.delete(slot)
      if (!old?.views.size) this.presentObjects.delete(previous)
    }
    this.presentSlots.set(slot, key)
    let object = this.presentObjects.get(key)
    if (!object) { object = { id: randomUUID(), views: new Set() }; this.presentObjects.set(key, object) }
    object.views.add(slot)
    return { ok: true, fileIdentity: object.id, artifactIds }
  }
  async presentApplications(payload: unknown, current: () => boolean,
    list = listPresentFileApplications): Promise<ToolResultLocalPresentApplicationsResultV1> {
    const request = toolResultLocalPresentFileRequestSchemaV1.safeParse(payload)
    if (!request.success) return unavailable()
    const selected = await this.selectedPresentFile(request.data, current)
    if (!selected || !await selected.guard()) return unavailable()
    const result = await list(selected.target.path, selected.guard)
    return await selected.guard() ? result : unavailable()
  }
  async openPresent(payload: unknown, current: () => boolean,
    launch = openPresentFileApplication, reveal: (path: string) => void = path => shell.showItemInFolder(path)): Promise<ToolResultLocalEffectResultV1> {
    const request = toolResultLocalPresentOpenRequestSchemaV1.safeParse(payload)
    if (!request.success) return unavailable()
    const selected = await this.selectedPresentFile(request.data, current)
    if (!selected || !await selected.guard()) return unavailable()
    if ('action' in request.data) {
      // The captured original lease must still be current after the last await.
      // No renderer path or application catalog participates in this effect.
      if (!selected.isCurrent()) return unavailable()
      try { reveal(selected.target.path); return selected.isCurrent() ? { ok: true } : unavailable() }
      catch { return unavailable() }
    }
    const result = await launch(selected.target.path, request.data.applicationId, selected.guard)
    return await selected.guard() ? result : unavailable()
  }
  private async readPresentPreviewBytes(payload: unknown, current: () => boolean, limit: number,
    media?: 'image' | 'pdf' | 'excel'): Promise<PresentPreviewBytes | null> {
    const request = toolResultLocalPresentFileRequestSchemaV1.safeParse(payload)
    if (!request.success || !constants.O_NOFOLLOW) return null
    const selected = await this.selectedPresentFile(request.data, current)
    return selected ? this.readSelectedPresentBytes(selected, limit, media) : null
  }
  private async readSelectedPresentBytes(selected: SelectedPresentFile, limit: number,
    media?: 'image' | 'pdf' | 'excel', complete = media !== undefined): Promise<PresentPreviewBytes | null> {
    if (!constants.O_NOFOLLOW || !await selected.guard() || !selected.isCurrent()) return null
    // Only the sealed Host path chooses MIME. Renderer labels never authorize
    // an ordinary file reader or change the declared index's current target.
    const extension = extname(selected.target.path).toLowerCase()
    const mimeType = media === 'image' ? presentImageMimeTypes.get(extension)
      : media === 'pdf' && extension === '.pdf' ? 'application/pdf' : undefined
    let format: ExcelFormat | undefined
    if (media === 'excel') {
      try { format = excelFormat(selected.target.path) } catch { return null }
    } else if (media && !mimeType) return null
    try {
      const handle = await open(selected.target.path, constants.O_RDONLY | constants.O_NOFOLLOW)
      let staged: PresentPreviewBytes | null = null
      try {
        const before = await handle.stat({ bigint: true })
        if (!before.isFile() || before.dev !== selected.observed.dev || before.ino !== selected.observed.ino ||
          complete && (before.size > BigInt(limit) || !samePresentFileVersion(before, selected.observed))) return null
        const chunk = Buffer.alloc(64 << 10), chunks: Buffer[] = []
        let length = 0
        while (length < limit) {
          const { bytesRead } = await handle.read(chunk, 0, Math.min(chunk.length, limit - length), null)
          if (!selected.isCurrent()) return null
          if (!bytesRead) break
          chunks.push(Buffer.from(chunk.subarray(0, bytesRead))); length += bytesRead
        }
        const after = await handle.stat({ bigint: true })
        if (after.dev !== before.dev || after.ino !== before.ino || after.size !== before.size ||
          after.mtimeNs !== before.mtimeNs || after.ctimeNs !== before.ctimeNs ||
          complete && before.size !== BigInt(length) || !await selected.guard() || !selected.isCurrent()) return null
        const guard = async () => {
          if (!await selected.guard() || !selected.isCurrent()) return false
          if (!complete) return true
          try {
            const linked = await lstat(selected.target.path, { bigint: true })
            return samePresentFileVersion(linked, before) && selected.isCurrent()
          } catch { return false }
        }
        staged = { path: selected.target.path, bytes: Buffer.concat(chunks, length), size: Number(before.size),
          mtimeMs: Number(before.mtimeNs) / 1_000_000, truncated: before.size > BigInt(length),
          ...(mimeType ? { mimeType } : {}), ...(format ? { excelFormat: format } : {}), isCurrent: selected.isCurrent, guard }
      } finally { await handle.close() }
      // Recheck after close and after the guard's last await. A same-view
      // successor never inherits the original reader's staged bytes.
      return staged && await staged.guard() && selected.isCurrent() ? staged : null
    } catch { return null }
  }
  // Main-only snapshot; complete descriptor bytes never cross the bridge.
  async resolvePresentOffice(payload: unknown, current: () => boolean): Promise<PresentOfficeSnapshot | null> {
    const parsed = toolResultLocalPresentFileRequestSchemaV1.safeParse(payload)
    if (!parsed.success) return null
    const view = this.views.get(parsed.data.viewId)
    const selected = await this.selectedPresentFile(parsed.data, current)
    if (!view || !selected || view.owner.signal.aborted) return null
    const kind = extname(selected.target.path).slice(1).toLowerCase()
    if (kind !== 'docx' && kind !== 'pptx') return null
    const file = await this.readSelectedPresentBytes(selected, 16 * 1024 * 1024, undefined, true)
    if (!file || file.truncated || file.bytes.byteLength === 0 || view.owner.signal.aborted || !file.isCurrent()) { file?.bytes.fill(0); return null }
    return { ...file, workspaceRoot: selected.workspaceRoot, kind, selector: parsed.data,
      digest: createHash('sha256').update(file.bytes).digest('hex'), signal: view.owner.signal,
      isCurrent: () => this.current(parsed.data.viewId, view) && !view.owner.signal.aborted && file.isCurrent(),
      guard: async () => this.current(parsed.data.viewId, view) && !view.owner.signal.aborted && await file.guard() && this.current(parsed.data.viewId, view) && !view.owner.signal.aborted }
  }

  async resolvePresentHtml(payload: unknown, current: () => boolean): Promise<PresentHtmlSnapshot | null> {
    const request = toolResultLocalPresentFileRequestSchemaV1.safeParse(payload)
    if (!request.success) return null
    const view = this.views.get(request.data.viewId)
    const selected = await this.selectedPresentFile(request.data, current)
    if (!view || !selected || view.owner.signal.aborted || !/\.html?$/i.test(selected.target.path)) return null
    const file = await this.readSelectedPresentBytes(selected, 8 << 20, undefined, true)
    if (!file) return null
    if (file.truncated || file.bytes.includes(0) || !file.isCurrent() || view.owner.signal.aborted) { file.bytes.fill(0); return null }
    const isCurrent = () => !view.owner.signal.aborted && this.current(request.data.viewId, view) && file.isCurrent()
    const guard = async () => isCurrent() && await file.guard() && isCurrent()
    if (!await guard() || !isCurrent()) { file.bytes.fill(0); return null }
    return { ...file, workspaceRoot: selected.workspaceRoot,
      digest: createHash('sha256').update(file.bytes).digest('hex'), guard, isCurrent, signal: view.owner.signal }
  }
  async previewPresent(payload: unknown, current: () => boolean): Promise<WorkspaceFileReadResult> {
    const denied: WorkspaceFileReadResult = { ok: false, message: 'Protected current file preview unavailable.' }
    const file = await this.readPresentPreviewBytes(payload, current, 1_500_000)
    if (!file || !file.isCurrent()) return denied
    if (file.bytes.includes(0)) return { ok: false, message: 'This file format has no source preview. Open it in its associated application.' }
    try {
      // A byte-limited final sequence may be incomplete; malformed text
      // earlier in the file is never accepted through a replacement decoder.
      new TextDecoder('utf-8', { fatal: true }).decode(file.bytes, { stream: file.truncated })
      return { ok: true, path: file.path, content: file.bytes.toString('utf8'), size: file.size, truncated: file.truncated }
    } catch { return denied }
  }
  async previewPresentImage(payload: unknown, current: () => boolean): Promise<WorkspaceImageReadResult> {
    const file = await this.readPresentPreviewBytes(payload, current, 12 * 1024 * 1024, 'image')
    if (!file || !file.isCurrent() || !file.mimeType) return { ok: false, message: 'Protected current image preview unavailable.' }
    return { ok: true, path: file.path, dataUrl: `data:${file.mimeType};base64,${file.bytes.toString('base64')}`,
      mimeType: file.mimeType, size: file.size }
  }
  async previewPresentPdf(payload: unknown, current: () => boolean): Promise<WorkspacePdfReadResult> {
    const file = await this.readPresentPreviewBytes(payload, current, 64 * 1024 * 1024, 'pdf')
    if (!file || !file.isCurrent()) return { ok: false, message: 'Protected current PDF preview unavailable.' }
    return { ok: true, path: file.path, dataBase64: file.bytes.toString('base64'), mimeType: 'application/pdf',
      size: file.size, mtimeMs: file.mtimeMs }
  }
  async previewPresentExcel(payload: unknown, current: () => boolean): Promise<WorkspaceExcelReadResult> {
    const file = await this.readPresentPreviewBytes(payload, current, EXCEL_PREVIEW_LIMITS.maxBytes, 'excel')
    if (!file || !file.isCurrent() || file.truncated || !file.excelFormat) {
      return { ok: false, message: 'Protected current spreadsheet preview unavailable.' }
    }
    return { ok: true, path: file.path, dataBase64: file.bytes.toString('base64'), format: file.excelFormat,
      size: file.size, contentDigest: createHash('sha256').update(file.bytes).digest('hex'), limits: EXCEL_PREVIEW_LIMITS }
  }
  private async copyPresentExcelSelection(
    request: ToolResultLocalPresentExcelCopyRequestV1, view: View,
    current: () => boolean, copy: ToolResultLocalCopySinkV1
  ): Promise<ToolResultLocalEffectResultV1> {
    if (!this.readExcelCells || this.excelCopyPending) return unavailable()
    this.excelCopyPending = true
    const live = () => current() && this.current(request.viewId, view)
    // Preserve index and the original captured View. The complete-read request
    // must not carry the Copy-only fields through its strict file schema.
    const fileRequest: ToolResultLocalPresentFileRequestV1 = { ...selectorOf(request),
      viewId: request.viewId, snapshotDigest: request.snapshotDigest, index: request.index }
    const selection: ToolResultLocalPresentExcelCopyRequestV1['selection'] = {
      sheetIndex: request.selection.sheetIndex, sheetId: request.selection.sheetId,
      ranges: request.selection.ranges.map(range => ({ row: [...range.row] as [number, number], column: [...range.column] as [number, number] }))
    }
    let first: PresentPreviewBytes | null = null, final: PresentPreviewBytes | null = null
    let bytes: Uint8Array<ArrayBuffer> | null = null
    try {
      first = await this.readPresentPreviewBytes(fileRequest, live, EXCEL_PREVIEW_LIMITS.maxBytes, 'excel')
      if (!first || !live() || first.truncated || !first.excelFormat
          || createHash('sha256').update(first.bytes).digest('hex') !== request.contentDigest) return unavailable()
      bytes = Uint8Array.from(first.bytes)
      const frame = validateExcelCopyFrameV1(await this.readExcelCells(bytes, first.excelFormat, selection, view.owner.signal), selection)
      if (!live()) return unavailable()
      let projected: { text: string; html: string }
      try { projected = projectExcelCopyContentV1(frame) }
      catch { return { ok: false, code: 'unsupported' } }
      // The parser await must not export an older edited source or an inherited
      // same-view successor. Re-read complete current bytes through the same
      // sealed declaration before the final synchronous clipboard effect.
      final = await this.readPresentPreviewBytes(fileRequest, live, EXCEL_PREVIEW_LIMITS.maxBytes, 'excel')
      if (!final || final.truncated || final.excelFormat !== first.excelFormat
          || createHash('sha256').update(final.bytes).digest('hex') !== request.contentDigest || !live()) return unavailable()
      copy(projected.text, projected.html)
      return { ok: true }
    } catch { return unavailable() }
    finally {
      bytes?.fill(0); first?.bytes.fill(0); final?.bytes.fill(0)
      this.excelCopyPending = false
    }
  }
  private async savePresentFile(request: ToolResultLocalPresentFileRequestV1, current: () => boolean,
    pickDestination: (suggestedName?: string) => Promise<string | null>,
    save: ToolResultLocalSaveSinkV1): Promise<ToolResultLocalEffectResultV1> {
    // Pin the original View and file version before the native dialog. Selecting
    // again afterwards would allow a changed file or same-view successor to win.
    const selected = await this.selectedPresentFile(request, current)
    if (!selected || !constants.O_NOFOLLOW || !await selected.guard() || !selected.isCurrent()) return unavailable()
    if (selected.observed.size > BigInt(TOOL_RESULT_LOCAL_PRESENT_SAVE_MAX_BYTES_V1)) return { ok: false, code: 'unsupported' }
    let destination: string | null
    try { destination = await pickDestination(basename(selected.target.path)) } catch { return unavailable() }
    if (destination === null) return { ok: false, code: 'canceled' }
    const file = await this.readSelectedPresentBytes(selected, TOOL_RESULT_LOCAL_PRESENT_SAVE_MAX_BYTES_V1, undefined, true)
    if (!file || file.truncated || !await file.guard() || !file.isCurrent()) return unavailable()
    // Only a complete descriptor read reaches the native write. Raw bytes stay
    // in Main; the renderer receives no path, content or persistent grant.
    try { await save(destination, file.bytes, { check: file.guard, isCurrent: file.isCurrent }); return { ok: true } } catch { return unavailable() }
  }
  async effect(
    payload: unknown, action: 'copy' | 'save', current: () => boolean,
    pickDestination: (suggestedName?: string) => Promise<string | null>,
    copy: ToolResultLocalCopySinkV1, save: ToolResultLocalSaveSinkV1
  ): Promise<ToolResultLocalEffectResultV1> {
    const request = (action === 'copy' ? toolResultLocalCopyRequestSchemaV1 : toolResultLocalSaveRequestSchemaV1).safeParse(payload)
    if (!request.success || !current()) return unavailable()
    const { viewId, snapshotDigest } = request.data
    const view = this.views.get(viewId)
    if (!view || view.digest !== snapshotDigest ||
        JSON.stringify(view.selector) !== JSON.stringify(selectorOf(request.data))) return unavailable()
    if (action === 'save' && 'index' in request.data) {
      return this.savePresentFile(request.data, () => current() && this.current(viewId, view), pickDestination, save)
    }
    if (action === 'copy' && 'selection' in request.data) {
      const selected = toolResultLocalPresentExcelCopyRequestSchemaV1.safeParse(request.data)
      return selected.success ? this.copyPresentExcelSelection(selected.data, view, current, copy) : unavailable()
    }
    let destination: string | null = null
    if (action === 'save') {
      // Check authority before opening the dialog and again after it returns.
      const beforeDialog = await this.fresh(viewId, view)
      if (!beforeDialog || !current()) return this.fail(viewId, view)
      if (['goal', 'todo', 'skill', 'job', 'schedule', 'inspect', 'present', 'file_result'].includes(beforeDialog.capture.kind)) return { ok: false, code: 'unsupported' }
      try { destination = await pickDestination() } catch { return unavailable() }
      if (destination === null) return { ok: false, code: 'canceled' }
    }
    const first = await this.fresh(viewId, view)
    if (!first || !current()) return this.fail(viewId, view)
    if (['goal', 'todo', 'skill', 'job', 'schedule', 'inspect', 'present', 'file_result'].includes(first.capture.kind)) return { ok: false, code: 'unsupported' }
    const section = 'section' in request.data ? request.data.section ?? 'result' : 'result'
    const sectionText = (display: ToolResultLocalHostDisplayV1): string | undefined => {
      if (section === 'result') return display.capture.body
      if (display.capture.kind === 'search') return section === 'payload' ? display.capture.search.inputJSON : undefined
      if (display.capture.kind !== 'web_fetch') return undefined
      const web = display.capture.web
      switch (section) {
        case 'payload': return web.inputJSON
        case 'schema': return web.schemaJSON
        case 'request-url': return web.requestUrl
        case 'result-url': return web.finalUrl
      }
      return undefined
    }
    const original = sectionText(first)
    if (original === undefined) return { ok: false, code: 'unsupported' }
    let projected: string
    try { projected = ordinaryToolExportTextV1(original) }
    catch { return { ok: false, code: 'unsupported' } }
    if (original !== '' && projected === '') return { ok: false, code: 'unsupported' }
    const final = await this.fresh(viewId, view)
    if (!final || sectionText(final) !== original || !current() || !this.current(viewId, view)) return this.fail(viewId, view)
    try {
      if (action === 'copy') copy(projected)
      else await save(destination!, projected)
      return { ok: true }
    } catch { return unavailable() }
  }
}

export function registerToolResultLocalDisplayIpcV1(
  getMainWindow: () => BrowserWindow | null, transport: Transport,
  resolveArtifact?: (payload: unknown) => Promise<GeneratedArtifactResponse>,
  readExcelCells?: ExcelPreviewCopyReaderV1
): ProtectedPreviewReader {
  const hosts = new Map<number, ToolResultLocalDisplayHostV1>()
  const observedWindows = new WeakSet<BrowserWindow>()
  const current = (event: IpcMainInvokeEvent): boolean => {
    const window = getMainWindow()
    return Boolean(hostTransitionDepth === 0 && window && !window.isDestroyed() && window.isVisible() && !window.isMinimized() && window.webContents === event.sender &&
      !event.sender.isDestroyed() && event.senderFrame != null && event.senderFrame === event.sender.mainFrame)
  }
  const hostFor = (event: IpcMainInvokeEvent) => {
    const window = getMainWindow()
    if (window && !observedWindows.has(window)) {
      observedWindows.add(window)
      const background = () => invalidateToolResultLocalDisplaysV1('background')
      window.on('hide', background)
      window.on('minimize', background)
      window.once('closed', () => {
        window.removeListener('hide', background)
        window.removeListener('minimize', background)
      })
    }
    let host = hosts.get(event.sender.id)
    if (!host) {
      host = new ToolResultLocalDisplayHostV1(transport, viewId => {
        if (!event.sender.isDestroyed()) event.sender.send('tool-result-local:invalidated', viewId ?? null)
      }, readExcelCells)
      hosts.set(event.sender.id, host)
      const invalidate = () => host!.invalidate()
      invalidators.add(invalidate)
      event.sender.once('destroyed', () => { invalidate(); invalidators.delete(invalidate); hosts.delete(event.sender.id) })
    }
    return host
  }
  ipcMain.handle('tool-result-local:open', async (event, payload: unknown) => {
    if (!current(event)) return unavailable()
    const host = hostFor(event)
    const controller = new AbortController()
    const lease = captureNativeRendererDocument(event.sender, () => current(event), () => { controller.abort(); host.invalidate() })
    return host.open(payload, {
      signal: controller.signal,
      isCurrent: lease.isCurrent,
      release: () => { controller.abort(); lease.release() }
    })
  })
  ipcMain.handle('tool-result-local:present-history', async (event, payload: unknown) => {
    if (!current(event)) return unavailable()
    const host = hostFor(event), controller = new AbortController()
    const lease = captureNativeRendererDocument(event.sender, () => current(event), () => { controller.abort(); host.invalidate() })
    return host.openPresentHistory(payload, {
      signal: controller.signal, isCurrent: lease.isCurrent,
      release: () => { controller.abort(); lease.release() }
    })
  })
  ipcMain.handle('tool-result-local:invalidate', event => {
    if (!current(event)) throw new Error('Tool display authority unavailable.')
    invalidateToolResultLocalDisplaysV1()
  })
  ipcMain.handle('tool-result-local:close', (event, viewId: unknown) => {
    if (current(event) && typeof viewId === 'string') {
      hosts.get(event.sender.id)?.close(viewId)
      event.sender.send('tool-result-local:invalidated', viewId)
    }
  })
  ipcMain.handle('tool-result-local:navigate', async (event, payload: unknown) => {
    if (!current(event)) return unavailable()
    return hostFor(event).navigate(payload, () => current(event))
  })
  ipcMain.handle('tool-result-local:web-link', async (event, payload: unknown) => {
    if (!current(event)) return unavailable()
    return hostFor(event).openWebLink(payload, () => current(event), url => shell.openExternal(url))
  })
  ipcMain.handle('tool-result-local:present-associations', async (event, payload: unknown) => {
    if (!current(event)) return unavailable()
    return hostFor(event).associatePresent(payload, () => current(event), resolveArtifact)
  })
  ipcMain.handle('tool-result-local:present-applications', async (event, payload: unknown) => {
    if (!current(event)) return unavailable()
    return hostFor(event).presentApplications(payload, () => current(event))
  })
  ipcMain.handle('tool-result-local:present-open', async (event, payload: unknown) => {
    if (!current(event)) return unavailable()
    return hostFor(event).openPresent(payload, () => current(event))
  })
  for (const action of ['copy', 'save'] as const) {
    ipcMain.handle('tool-result-local:' + action, async (event, payload: unknown) => {
      if (!current(event)) return unavailable()
      return hostFor(event).effect(payload, action, () => current(event), async suggestedName => {
        const window = getMainWindow()
        if (!window || !current(event)) return null
        const result = await dialog.showSaveDialog(window, suggestedName ? { defaultPath: suggestedName } : {
          defaultPath: 'tool-result-redacted.txt', filters: [{ name: 'Text', extensions: ['txt'] }]
        })
        return result.canceled || !result.filePath ? null : result.filePath
      }, (text, checkedHtml) => {
        // A checked dual-format result is one synchronous native effect.
        if (checkedHtml !== undefined) clipboard.write({ text, html: checkedHtml })
        else clipboard.writeText(text)
      }, saveToolResultLocalFileV1)
    })
  }
  return Object.assign(async (event: IpcMainInvokeEvent, request: unknown): Promise<WorkspaceFileReadResult> => {
    if (!current(event)) return { ok: false, message: 'Protected file preview unavailable.' }
    return toolResultLocalPresentFileRequestSchemaV1.safeParse(request).success
      ? hostFor(event).previewPresent(request, () => current(event)) : hostFor(event).preview(request, () => current(event))
  }, {
    office: async (event: IpcMainInvokeEvent, request: unknown): Promise<PresentOfficeSnapshot | null> =>
      current(event) ? hostFor(event).resolvePresentOffice(request, () => current(event)) : null,
    html: async (event: IpcMainInvokeEvent, request: unknown): Promise<PresentHtmlSnapshot | null> =>
      current(event) ? hostFor(event).resolvePresentHtml(request, () => current(event)) : null,
    image: async (event: IpcMainInvokeEvent, request: unknown): Promise<WorkspaceImageReadResult> =>
      current(event) ? hostFor(event).previewPresentImage(request, () => current(event))
        : { ok: false, message: 'Protected current image preview unavailable.' },
    excel: async (event: IpcMainInvokeEvent, request: unknown): Promise<WorkspaceExcelReadResult> =>
      current(event) ? hostFor(event).previewPresentExcel(request, () => current(event))
        : { ok: false, message: 'Protected current spreadsheet preview unavailable.' },
    pdf: async (event: IpcMainInvokeEvent, request: unknown): Promise<WorkspacePdfReadResult> =>
      current(event) ? hostFor(event).previewPresentPdf(request, () => current(event))
        : { ok: false, message: 'Protected current PDF preview unavailable.' }
  })
}
