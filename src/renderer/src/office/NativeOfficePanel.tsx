import type { ResolvedWriteQuickAction } from '../write/quick-actions'
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { FileText, RotateCcw, Undo2, Quote, Pencil } from 'lucide-react'
import type { NativeOfficeMenuTarget, NativeOfficeRequest, NativeOfficeSelection } from '@shared/native-office'
import { useNativeOfficeStore } from './native-office-store'
import { nativeSelectionActions } from './native-selection-actions'

import { useNativeReferenceStore, nativeSelectionLabel, nativeSelectionText, isNativeSelectionEditable, type NativeReference } from './native-reference-store'

const appearance = () => ({ theme: document.documentElement.dataset.theme === 'dark' ? 'dark' as const : 'light' as const, reducedMotion: document.documentElement.dataset.motionReduced === 'true' })

export function NativeOfficePanel({ visible, onFocusConversation, onSubmitPrompt, threadId = null, quickActions = [], fileActions }: { visible: boolean; fileActions?: ReactNode; quickActions?: ResolvedWriteQuickAction[]; threadId?: string | null; onFocusConversation?: () => void; onSubmitPrompt?: (prompt: string, references: NativeReference[]) => void; setInput?: (value: string) => void }) {
  const { t } = useTranslation('common')
  const target = useNativeOfficeStore((s) => s.target)
  const view = useNativeOfficeStore((s) => s.view)
  const error = useNativeOfficeStore((s) => s.error)
  const targetMatches = !target || view?.path === target.path || view?.path === `${target.workspace.replace(/\/$/, '')}/${target.path}`
  const loading = !error && (!view || view.status === 'loading' || !targetMatches)
  const draftKey = JSON.stringify([threadId, target?.workspace, target?.path])
  const draft = useNativeReferenceStore(state => state.drafts[draftKey])
  const note = draft?.note ?? ''
  const setNote = (value: string) => useNativeReferenceStore.getState().setDraft(draftKey, {note:value,selection:draft?.selection})
  const [added, setAdded] = useState(false)
  const frozenSelection = useRef<{ owner: string; selection: NativeOfficeSelection } | null>(null)
  const annotationThread = useRef(threadId)
  const ignoredSelection = useRef<string | null>(null)
  const [busy, setBusy] = useState(false)
  const currentTarget = useRef(target); currentTarget.current = target
  const currentThread = useRef(threadId); currentThread.current = threadId
  const shown = useRef(visible); shown.current = visible
  const generation = useRef(0)
  const surface = useRef<HTMLDivElement>(null)
  const menuHandler = useRef<(target?: NativeOfficeMenuTarget) => Promise<void>>(async () => {})
  const menuOpen = useRef(false)
  const menuContext = useRef('')
  useEffect(() => { shown.current = visible; return () => { generation.current++; shown.current = false } }, [])
  useEffect(() => window.analytix.office.onMenuRequested?.(target => { void menuHandler.current(target) }), [])
  const bounds = () => {
    const rect = surface.current?.getBoundingClientRect()
    return { x: Math.max(0, Math.round(rect?.x ?? 0)), y: Math.max(0, Math.round(rect?.y ?? 0)),
      width: Math.max(1, Math.round(rect?.width ?? 1)), height: Math.max(1, Math.round(rect?.height ?? 1)) }
  }
  const invoke = useCallback(async (request: NativeOfficeRequest) => {
    const epoch = generation.current
    setBusy(true)
    if (request.action === 'open') {
      request = { ...request, appearance: appearance() }
      useNativeOfficeStore.setState({ view: null, error: null })
    }
    try {
      const result = await window.analytix.office.request(request)
      if (request.action === 'reference' || request.action === 'annotate') {
        // Core revokes all earlier scopes for this object, even if focus moved
        // while capture was in flight. Reflect that fact without accepting old UI.
        if (request.action === 'reference' && result.ok && result.view?.scope?.editable) useNativeReferenceStore.getState().revokeScopes(request.objectId)
        const current = useNativeOfficeStore.getState().view
        if (!current || current.objectId !== request.objectId || current.revision !== request.revision ||
          (current.changeSequence ?? 0) !== request.expectedChangeSequence) return undefined
      }
      if (epoch === generation.current) useNativeOfficeStore.getState().receive(result.view, result.ok ? null : result.error ?? 'unavailable')
      if (!shown.current) void window.analytix.office.request({action:'hide'})
      return result
    } catch { if (epoch === generation.current) useNativeOfficeStore.setState({ error: 'unavailable' }) }
    finally { if (epoch === generation.current) setBusy(false) }
  }, [])
  useEffect(() => window.analytix.office.onChange((next) => useNativeOfficeStore.getState().receive(next, next?.error ?? null)), [])
  useEffect(() => {
    generation.current++
    setAdded(false)
    setBusy(false)
    if (!target || !visible) return
    void invoke({ action: 'open', ...target, bounds: bounds() })
  }, [target, visible, invoke]) // Main restores the retained object session; hidden tabs never open a surface.
  useEffect(() => {
    const element = surface.current
    if (!element) return
    const hide = () => { void window.analytix.office.request({ action: 'hide' }).catch(() => undefined) }
    if (!visible) { hide(); return }
    let stopped = false
    let lastBounds: string | undefined
    const update = () => {
      if (stopped) return
      const next = bounds()
      const style = appearance()
      const key = `${next.x},${next.y},${next.width},${next.height},${style.theme},${style.reducedMotion}`
      if (key === lastBounds) return
      lastBounds = key
      void window.analytix.office.request({ action: 'bounds', bounds: next, appearance: style }).then((result) => {
        if (!result.ok && lastBounds === key) lastBounds = undefined
      }).catch(() => { if (lastBounds === key) lastBounds = undefined })
    }
    update()
    const observer = new ResizeObserver(update)
    observer.observe(element)
    // Dock animation changes the ancestor's width while this fixed-width
    // surface only moves. Observe that boundary to refresh viewport coordinates.
    const dock = element.closest('.ds-right-sidebar-pane')
    if (dock && dock !== element) observer.observe(dock)
    const appearanceObserver = new MutationObserver(update)
    appearanceObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'data-motion-reduced'] })
    window.addEventListener('resize', update)
    return () => {
      stopped = true
      observer.disconnect()
      appearanceObserver.disconnect()
      window.removeEventListener('resize', update)
      hide()
    }
  }, [visible, view?.objectId])
  const scopeId = view?.scope?.scopeId, scopeThreadId = view?.scope?.threadId, objectId = view?.objectId
  useEffect(() => {
    if (!visible || !scopeId || !objectId || !threadId || scopeThreadId !== threadId) return
    let stopped = false
    let timer: ReturnType<typeof setTimeout> | undefined
    let delay = 2000
    const poll = async () => {
      if (!useNativeOfficeStore.getState().beginProposalPoll()) {
        timer = setTimeout(() => { void poll() }, delay)
        return
      }
      const received = useNativeOfficeStore.getState().receiveSequence
      try {
        const result = await window.analytix.office.request({action:'proposals',objectId,scopeId})
        if (!stopped && result.ok && received === useNativeOfficeStore.getState().receiveSequence) useNativeOfficeStore.getState().receive(result.view)
        delay = result.ok ? 2000 : Math.min(delay * 2, 16000)
      } catch { delay = Math.min(delay * 2, 16000) }
      finally {
        useNativeOfficeStore.getState().endProposalPoll()
        if (!stopped) timer = setTimeout(() => { void poll() }, delay)
      }
    }
    void poll()
    return () => { stopped = true; clearTimeout(timer) }
  }, [visible, objectId, scopeId, scopeThreadId, threadId])
  const liveSelection = view?.selection
  const liveIdentity = JSON.stringify(liveSelection) ?? ''
  const selectionOwner = target && view ? JSON.stringify([threadId, target.workspace, target.path, view.objectId, view.revision, view.changeSequence ?? 0]) : ''
  if (annotationThread.current !== threadId) {
    annotationThread.current = threadId
    ignoredSelection.current = liveIdentity
    frozenSelection.current = null
  }
  if (frozenSelection.current?.owner !== selectionOwner) frozenSelection.current = null
  // Native focus leaves the selection surface when the user types a note.
  // Keep only the last nonempty capture, bounded to this exact document version.
  if (selectionOwner && view && targetMatches && liveSelection && liveIdentity !== ignoredSelection.current &&
    liveSelection.documentId === view.objectId && liveSelection.version === view.revision &&
    liveSelection.changeSequence === (view.changeSequence ?? 0) && nativeSelectionText(liveSelection).trim()) {
    frozenSelection.current = {owner:selectionOwner, selection:liveSelection}
  }
  const selection = frozenSelection.current?.selection ?? draft?.selection
  const targetRequest = view ? {objectId:view.objectId, revision:view.revision, expectedChangeSequence:view.changeSequence ?? 0} : null
  const selectionMatches = !!(selection && view && targetMatches && selection.documentId === view.objectId && selection.version === view.revision && selection.changeSequence === (view.changeSequence ?? 0))
  const hasSelection = selectionMatches && !!selection && !!nativeSelectionText(selection).trim()
  const editable = !!(threadId && view && selection && selectionMatches && isNativeSelectionEditable(view, selection))
  const annotationIdentity = JSON.stringify(selection)
  useEffect(() => {
    setAdded(false)
    if (selectionMatches && selection) {
      const state = useNativeReferenceStore.getState()
      state.setDraft(draftKey, {note:state.drafts[draftKey]?.note ?? '',selection:structuredClone(selection)})
    }
  }, [draftKey, annotationIdentity, selectionMatches])
  const quote = async (allowEdit = editable, actionPrompt?: string) => {
    if (!visible || !view || !selection || !target || !hasSelection) return false
    const current = useNativeOfficeStore.getState().view
    if (!shown.current || currentThread.current !== threadId || currentTarget.current !== target ||
      current?.objectId !== view.objectId || current.revision !== selection.version ||
      (current.changeSequence ?? 0) !== selection.changeSequence) return false
    const epoch = generation.current
    const owner = currentTarget.current
    let scopeId: string | undefined
    if (allowEdit) {
      if (!isNativeSelectionEditable(useNativeOfficeStore.getState().view ?? view, selection) || !threadId || !targetRequest || !selection.token) return false
      const result = await invoke({action:'reference', ...targetRequest, selectionToken:selection.token, threadId, editable:true})
      if (epoch !== generation.current || owner !== currentTarget.current || currentThread.current !== threadId || !shown.current) return false
      if (!result?.ok || !result.view?.scope?.editable) return false
      scopeId = result.view.scope.scopeId
    }
    const annotationNote = [note.trim(), actionPrompt?.trim()].filter(Boolean).join('\n')
    const reference: NativeReference = {id:crypto.randomUUID(),threadId, ...(scopeId ? {scopeId,editable:true} : {editable:false}), workspace:target.workspace, path:view.path, objectId:view.objectId, revision:selection.version, selection:structuredClone(selection), label:nativeSelectionLabel(view, selection), text:nativeSelectionText(selection), ...(annotationNote ? {note:annotationNote} : {})}
    if (!actionPrompt) useNativeReferenceStore.getState().add(reference)
    setAdded(!actionPrompt)
    if (actionPrompt) onSubmitPrompt?.(actionPrompt, [reference])
    onFocusConversation?.()
    return true
  }
  const errors: Record<string, string> = {conflict:'文件已在外部修改；草稿保留，请核对版本。', unknown:'修改结果尚未确认，请查询本次更新状态。', unsaved_changes:'文档更新尚未完成，请稍后重试。', stale_selection:'选区已过期，请重新选择。', unsupported_selection:'此选区暂不支持该操作。', package_changed:'插件版本已变化；草稿保留。', capacity:'已有两个原生文档，请先完成更新并关闭一个。', save_failed:'更新未完成，可重试；工作副本保留。'}
  const canRequestEdit = !!(threadId && view && selection && selectionMatches && isNativeSelectionEditable({...view,editing:true},selection))
  const actions = nativeSelectionActions({quickActions,hasSelection,editable:canRequestEdit,busy,canSubmit:!!onSubmitPrompt,
    labels:{copy:t('nativeActionCopy'),quote:t('nativeActionQuote')},
    copy:async () => {
      try { if (selection) await navigator.clipboard.writeText(nativeSelectionText(selection)) }
      catch { if (shown.current) useNativeOfficeStore.setState({error:'unavailable'}) }
    },
    quote:() => quote(),task:async action => {
      if (action.mode === 'edit' && !view?.editing) {
        if (!targetRequest) return
        const epoch = generation.current
        const result = await invoke({action:'annotate',...targetRequest})
        if (!result?.ok || epoch !== generation.current || !shown.current) return
      }
      await quote(action.mode === 'edit',action.prompt)
    }})
  const contextIdentity = JSON.stringify([threadId,target?.workspace,target?.path,view?.objectId,view?.revision,view?.changeSequence,annotationIdentity])
  menuContext.current = contextIdentity
  menuHandler.current = async requested => {
    if (!visible || !targetRequest || !hasSelection || busy || menuOpen.current || !window.analytix.office.showActionMenu) return
    if (requested && (requested.objectId !== targetRequest.objectId || requested.revision !== targetRequest.revision || requested.expectedChangeSequence !== targetRequest.expectedChangeSequence)) return
    const epoch = generation.current
    menuOpen.current = true
    try {
      const choice = await window.analytix.office.showActionMenu({...targetRequest,actions:actions.map(({id,label,enabled}) => ({id,label:label.replace(/[\x00-\x1f]/g,' ').trim(),enabled}))})
      if (epoch !== generation.current || !shown.current || menuContext.current !== contextIdentity) return
      const action = actions.find(action => action.id === choice.actionId)
      if (action?.enabled) await action.run()
    } catch { if (epoch === generation.current && shown.current && menuContext.current === contextIdentity) useNativeOfficeStore.setState({error:'unavailable'}) }
    finally { menuOpen.current = false }
  }
  return <div className="office-preview-host relative flex min-h-0 flex-1 flex-col" aria-busy={!!loading} onKeyDown={event => {
    if (event.nativeEvent.isComposing || event.keyCode === 229 || event.altKey || event.ctrlKey || event.metaKey) return
    if (event.target instanceof Element && event.target.closest('input, textarea, [contenteditable="true"]')) return
    if (event.key === 'ContextMenu' || event.key === 'F10' && event.shiftKey) { event.preventDefault(); void menuHandler.current() }
  }}>
    {view?.status === 'ready' || view?.editing || view?.saving ? <div className="flex shrink-0 flex-wrap items-center gap-1 border-b border-ds-border-muted px-2 py-1" role="toolbar" aria-label="文档操作">
      <button className="inline-flex items-center gap-1 rounded px-2 py-1 text-xs" disabled={busy || view.editing} aria-label="标注文档" aria-pressed={!!view.editing} title="标注" onClick={() => targetRequest && void invoke({action:'annotate', ...targetRequest})}><Pencil className="h-3.5 w-3.5" />标注</button>
      {view.canUndo ? <button className="inline-flex items-center gap-1 rounded px-2 py-1 text-xs" disabled={busy || view.saving || view.dirty} aria-label="撤销修改" onClick={() => targetRequest && void invoke({action:'undoChange', ...targetRequest})}><Undo2 className="h-3.5 w-3.5" />撤销</button> : null}
      <span className="ml-auto text-xs text-ds-muted" role="status">{view.saving ? '更新结果待确认' : view.dirty ? '更新待完成' : view.canUndo ? '已更新' : view.editing ? '标注中' : '预览'}</span>
      {fileActions}
    </div> : null}
    {view?.status === 'ready' || view?.editing ? <div className="shrink-0 space-y-1 border-b border-ds-border-muted px-2 py-2" aria-label="文档标注">
      {hasSelection && selection || note ? <>
        <p className="truncate text-xs">{hasSelection && selection ? nativeSelectionLabel(view, selection) : '选区已过期，请重新选择；备注已保留。'}</p>
        <p className="text-[11px] text-ds-muted">{editable ? '可让 AI 提出局部修改，接受后应用。' : '仅供讨论，此选区不支持直接应用修改。'}</p>
        <div className="flex items-center gap-2">
          <input aria-label="标注备注" placeholder="备注（可选）" maxLength={4096} className="min-w-0 flex-1 bg-transparent text-xs" value={note} onChange={event => { setNote(event.target.value); setAdded(false) }} />
          <button className="inline-flex shrink-0 items-center gap-1 text-xs" disabled={busy || !hasSelection} onClick={() => void actions.find(action => action.id === 'quote')?.run()}><Quote className="h-3.5 w-3.5" />加入对话</button>
        </div>
        <div className="flex flex-wrap gap-2">{actions.filter(action => action.id !== 'quote').slice(0,4).map(action => <button key={action.id} className="text-xs text-ds-muted" disabled={!action.enabled} onClick={() => void action.run()}>{action.label}</button>)}<button className="text-xs text-ds-muted" disabled={busy || !hasSelection} onClick={() => void menuHandler.current()}>{t('nativeActionMore')}</button></div>
        {added ? <p role="status" className="text-[11px] text-ds-muted">标注已加入对话，尚未发送。</p> : null}
      </> : <p className="text-xs text-ds-muted">{view.editing ? '请在文档中选择文本、单元格或文本框，再添加标注。' : '点击“标注”，再选择需要讨论或修改的内容。'}</p>}
    </div> : null}
    {selection?.capture?.truncated ? <p className="shrink-0 px-2 text-xs text-ds-muted">已捕获 {selection.capture.capturedCharacters} / {selection.capture.totalCharacters} UTF-16 单元；引用不包含完整选区。</p> : null}
    {error && (view?.editing || view?.saving) ? <div role="alert" className="flex shrink-0 items-center gap-2 px-2 text-xs">
      <p>{errors[error] ?? t('nativeOfficeOperationFailed')}</p>
      {error === 'save_failed' || error === 'unknown' ? <button disabled={busy} className="shrink-0 text-xs" onClick={() => targetRequest && void invoke(error === 'unknown' || view.saving ? {action:'saveStatus',objectId:view.objectId} : {action:view.canUndo ? 'undoChange' : 'save',...targetRequest})}>{error === 'unknown' || view.saving ? '查询结果' : '重试'}</button> : null}
    </div> : null}
    {view?.proposals?.length ? <div className="max-h-36 shrink-0 overflow-auto border-b border-ds-border px-2 py-1" aria-label="原生修改提案">
      {view.proposals.map(proposal => <div key={proposal.proposalId} className="py-1 text-xs">
        <div className="my-1 overflow-hidden rounded border border-ds-border-muted font-mono text-xs">
          <del aria-label="修改前" className="block whitespace-pre-wrap bg-red-50 px-2 py-1 text-red-800 no-underline dark:bg-red-950/30 dark:text-red-200">{view.scope?.parts.map(part => part.kind === 'literal' ? part.text : '[保留原始字段]').join('')}</del>
          <ins aria-label="修改后" className="block whitespace-pre-wrap bg-emerald-50 px-2 py-1 text-emerald-800 no-underline dark:bg-emerald-950/30 dark:text-emerald-200">{proposal.parts.map(part => part.kind === 'literal' ? part.text : '[保留原始字段]').join('')}</ins>
        </div>
        {view.appliedProposals?.includes(proposal.proposalId) ? <span>{view.saving || view.dirty ? '正在更新，结果待确认' : view.canUndo ? '已更新' : '修改已处理'}</span> : proposal.status === 'rejected' ? <span>已拒绝</span> : <div className="flex gap-3">
          <button disabled={busy || !targetRequest || view.scope?.changeSequence !== view.changeSequence} onClick={() => targetRequest && view.scope && void invoke({action:'acceptProposal',...targetRequest,scopeId:view.scope.scopeId,proposalId:proposal.proposalId})}>应用修改</button>
          <button disabled={busy} onClick={() => view.scope && void invoke({action:'rejectProposal',objectId:view.objectId,scopeId:view.scope.scopeId,proposalId:proposal.proposalId})}>拒绝</button>
        </div>}
      </div>)}
    </div> : null}
    <div ref={surface} className="min-h-0 flex-1" aria-label={t('nativeOfficeEditor')} />
    {loading || (error && !view?.editing && !view?.saving) ? <div className="office-preview-state" role={error ? 'alert' : 'status'}>
      <FileText className="office-preview-state-icon" aria-hidden="true" />
      <p>{error ? errors[error] ?? t('nativeOfficeOperationFailed') : t('nativeOfficeStatus_loading')}</p>
      {error && target ? <button type="button" className="ds-toolbar-icon-button" aria-label={t('nativeOfficeRetry')} title={t('nativeOfficeRetry')} onClick={() => void invoke({ action: 'open', ...target, bounds: bounds() })}><RotateCcw className="h-4 w-4" /></button> : null}
    </div> : null}
  </div>
}
