import type { ToolResultLocalPresentFileRequestV1 } from '../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ResolvedWriteQuickAction } from '../write/quick-actions'
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { FileText, RotateCcw, Undo2, Quote, Pencil, ChevronDown, ScanEye, Play, Pause, Square } from '../design/AnalytixUiIcons'
import { nativeOfficeActionChoiceSchema, nativeOfficeFormatSchema, nativeOfficeFormatStateSchema, nativeOfficeRangeLeaseSchema, type NativeOfficeActionMenu, type NativeOfficeFormat, type NativeOfficeFormatState, type NativeOfficeRangeLease, type NativeOfficeMenuTarget, type NativeOfficeRequest, type NativeOfficeResponse, type NativeOfficeSelection, type NativeOfficeView } from '@shared/native-office'
import { useNativeOfficeStore, nativeOfficeTargetKey, nativeOfficeTargetMatches, nativeOfficeManualPhase, nativeOfficeHasUnresolvedChange, type NativeOfficeManualDiscardAttempt, type NativeOfficeTarget } from './native-office-store'
import { nativeSelectionActions } from './native-selection-actions'
import { WorkbookCellEditor } from './WorkbookCellEditor'
import { Tooltip } from '../components/chat/presentation/Tooltip'
import styles from './native-office-panel.module.css'

import { useNativeReferenceStore, nativeSelectionLabel, nativeSelectionText, isNativeSelectionEditable, nativeOfficeDraftReferencesCurrent, nativeOfficeSavedSelection, type NativeReference } from './native-reference-store'

const appearance = () => ({ theme: document.documentElement.dataset.theme === 'dark' ? 'dark' as const : 'light' as const, reducedMotion: document.documentElement.dataset.motionReduced === 'true' })
const presentationErrors: Record<string,string> = {
  presentation_unsupported:'当前文档或原生引擎不支持放映。',
  presentation_failed:'放映失败；当前文档保留。',
  presentation_start_timeout:'放映启动尚未确认，请重新打开预览。',
  unavailable:'放映状态暂不可确认，请刷新放映状态。'
}

type NativeFormatContext = {
  target: NativeOfficeTarget; threadId: string; objectId: string; revision: string
  expectedChangeSequence: number; changeId: string; generation: number
  rangeLease: NativeOfficeRangeLease; formatState: NativeOfficeFormatState
}
type NativeFormatMenu = NonNullable<NativeOfficeActionMenu['formatting']>
function currentFormatContext(state: ReturnType<typeof useNativeOfficeStore.getState>, threadId: string | null): NativeFormatContext | null {
  const {target,view,error,manualOwners} = state, manual = view?.manual, selection = view?.selection
  if (!threadId || !target || target.protectedPresent || !view || !nativeOfficeTargetMatches(target,view) ||
    nativeOfficeManualPhase(view) !== 'active' || !manual || nativeOfficeHasUnresolvedChange(view,error) ||
    [view.recovery?.pending,view.recovery?.current].some(change => change?.changeId === manual.changeId && ['committed','undone'].includes(change.status)) ||
    selection?.kind !== 'text' || !selection.text.trim() || selection.documentId !== view.objectId || selection.version !== view.revision ||
    selection.changeSequence !== (view.changeSequence ?? 0)) return null
  const owner = manualOwners[view.objectId]
  if (!owner || owner.owner !== nativeOfficeTargetKey(target) || owner.threadId !== threadId || owner.objectId !== view.objectId ||
    owner.changeId !== manual.changeId || owner.generation !== manual.generation || owner.baseRevision !== view.revision) return null
  const lease = nativeOfficeRangeLeaseSchema.safeParse(selection.rangeLease), format = nativeOfficeFormatStateSchema.safeParse(selection.formatState)
  if (!lease.success || !format.success || lease.data.token !== selection.token || lease.data.sequence !== selection.changeSequence ||
    lease.data.generation !== manual.generation) return null
  return {target,threadId,objectId:view.objectId,revision:view.revision,expectedChangeSequence:view.changeSequence ?? 0,
    changeId:manual.changeId,generation:manual.generation,rangeLease:lease.data,formatState:format.data}
}
const rangeLeaseMatches = (left: NativeOfficeRangeLease, right: NativeOfficeRangeLease) =>
  left.generation === right.generation && left.sequence === right.sequence && left.token === right.token
const formatContextKey = (context: NativeFormatContext) => JSON.stringify([nativeOfficeTargetKey(context.target),context.threadId,context.objectId,
  context.revision,context.changeId,context.generation,context.expectedChangeSequence,context.rangeLease.token,
  context.formatState.fontFamily,context.formatState.fontSize,context.formatState.bold,context.formatState.italic,context.formatState.underline])
const formatContextMatches = (current: NativeFormatContext | null, expected: NativeFormatContext) =>
  !!current && current.target === expected.target && formatContextKey(current) === formatContextKey(expected)

// Input readiness and explicit recovery have different gates: Host may retain
// a valid frozen transaction in status=error while its outcome is recovered.
function manualSessionCurrent(view: NativeOfficeView | null): boolean {
  return !!(view && !view.protectedPresent && view.kind === 'docx' && view.status !== 'loading' && view.editing &&
    view.manual?.version === 2 && view.manual.origin === 'manual' && view.manual.baseRevision === view.revision &&
    !['running','paused','error'].includes(view.playback?.phase ?? ''))
}
function manualRecoveryCanCancel(view: NativeOfficeView | null, threadId: string, changeId: string): boolean {
  const pending = view?.recovery?.pending
  return !!(pending && 'origin' in pending && pending.version === 2 && pending.origin === 'manual' &&
    pending.threadId === threadId && pending.changeId === changeId && pending.canCancel)
}
function manualConflict(view: NativeOfficeView | null, error: string | null): boolean {
  return [error,view?.error].includes('conflict') ||
    [view?.recovery?.pending,view?.recovery?.current].some(change => change?.status === 'conflict')
}
function canRetryManualDiscard(current: NativeOfficeView | null, owner: string, thread: string, changeId: string, attempt: NativeOfficeManualDiscardAttempt | undefined): boolean {
  if ([current?.recovery?.pending,current?.recovery?.current].some(change => change?.changeId === changeId && ['committed','undone'].includes(change.status))) return false
  return !!(attempt && current && manualSessionCurrent(current) && current.manual?.changeId === changeId &&
    attempt.owner === owner && attempt.objectId === current.objectId && attempt.threadId === thread &&
    attempt.changeId === changeId && attempt.baseRevision === current.revision)
}
function finishManualDiscard(attempt: NativeOfficeManualDiscardAttempt): void {
  useNativeOfficeStore.setState(state => {
    if (state.manualDiscardAttempts[attempt.objectId] !== attempt) return {}
    const remaining = {...state.manualDiscardAttempts}; delete remaining[attempt.objectId]
    return {manualDiscardAttempts:remaining}
  })
}

export function NativeOfficePanel({ visible: requestedVisible, presentSelector, onFocusConversation, onSubmitPrompt, onPrepareDraft, threadId = null, quickActions = [], fileActions }: { visible: boolean; presentSelector?: ToolResultLocalPresentFileRequestV1; fileActions?: ReactNode; quickActions?: ResolvedWriteQuickAction[]; threadId?: string | null; onFocusConversation?: () => void; onSubmitPrompt?: (prompt: string, references: NativeReference[]) => void; onPrepareDraft?: (prompt: string, references: NativeReference[]) => boolean; setInput?: (value: string) => void }) {
  const { t } = useTranslation('common')
  const target = useNativeOfficeStore((s) => s.target)
  const view = useNativeOfficeStore((s) => s.view)
  const error = useNativeOfficeStore((s) => s.error)
  const annotationInputFrozen = useNativeOfficeStore(s => s.annotationInputFrozen)
  const ownsSurface = presentSelector ? !!target?.protectedPresent && nativeOfficeTargetKey({workspace:'',path:'',protectedPresent:presentSelector}) === nativeOfficeTargetKey(target) : !target?.protectedPresent
  const instanceOwner = useRef(ownsSurface); instanceOwner.current = ownsSurface
  const renderedOwner = useRef({ownsSurface,visible:requestedVisible && ownsSurface}); renderedOwner.current = {ownsSurface,visible:requestedVisible && ownsSurface}
  const visible = requestedVisible && ownsSurface
  const readonlyPresent = !!presentSelector || !!target?.protectedPresent
  const targetIdentity = target ? nativeOfficeTargetKey(target) : ''
  const targetMatches = !target || nativeOfficeTargetMatches(target,view)
  const loading = !error && (!view || view.status === 'loading' || !targetMatches)
  const draftKey = JSON.stringify([threadId, target?.workspace, target?.path])
  const docxAnnotation = !readonlyPresent && view?.kind === 'docx'
  const annotationAvailable = view?.status === 'ready' || !!view?.editing
  const annotationControlOwner = JSON.stringify([draftKey, targetIdentity, view?.objectId, view?.revision])
  const [notesOwner, setNotesOwner] = useState<string | null>(null)
  const notesOpen = docxAnnotation && visible && notesOwner === annotationControlOwner
  useEffect(() => { setNotesOwner(null) }, [annotationControlOwner, visible])
  const draft = useNativeReferenceStore(state => readonlyPresent ? undefined : state.drafts[draftKey])
  const note = draft?.note ?? ''
  const setNote = (value: string) => {
    if (readonlyPresent || playbackActive || useNativeOfficeStore.getState().annotationInputFrozen || docxAnnotation && !annotationAvailable) return
    const state = useNativeReferenceStore.getState(), previous = state.drafts[draftKey]
    const sourceRevision = selectionMatches ? view?.revision : previous?.sourceRevision ?? view?.revision
    state.setDraft(draftKey, {...previous,note:value,dirty:true,sourceRevision})
    if (!view || !threadId || !sourceRevision) return
    const ownerId = view.objectId
    // Send every keystroke to Main immediately. Main coalesces pending input
    // and owns the close barrier; an unmount cleanup is not persistence.
    void window.analytix.office.request({action:'annotationSave',objectId:ownerId,threadId,note:value,sourceRevision}).then(result => {
      const saved = result.view?.annotation, latest = useNativeReferenceStore.getState().drafts[draftKey]
      if (result.ok && saved?.objectId === ownerId && saved.threadId === threadId && saved.note === value && latest?.note === value && latest.sourceRevision === sourceRevision) {
        useNativeReferenceStore.getState().setDraft(draftKey,{...latest,dirty:false})
      }
      // An older response can arrive after a newer edit was acknowledged. It
      // must not republish its old annotation into the hydration effect.
      if (saved && latest && (saved.note !== latest.note || saved.sourceRevision !== latest.sourceRevision)) return
      if (currentThread.current === threadId && currentTarget.current === target) useNativeOfficeStore.getState().receive(result.view,result.ok ? null : result.error ?? 'unavailable')
    }).catch(() => { if (currentThread.current === threadId && currentTarget.current === target) useNativeOfficeStore.setState({error:'unavailable'}) })
  }
  const [added, setAdded] = useState(false)
  const frozenSelection = useRef<{ owner: string; selection: NativeOfficeSelection } | null>(null)
  const annotationThread = useRef(threadId)
  const ignoredSelection = useRef<string | null>(null)
  const [busy, setBusy] = useState(false)
  const viewButton = useRef<HTMLButtonElement>(null)
  const [viewMenuOwner, setViewMenuOwner] = useState<string | null>(null)
  const [viewFocusReturn, setViewFocusReturn] = useState<{owner:string;epoch:number} | null>(null)
  const viewOperation = useRef(0)
  const viewPending = useRef<{owner:string; operation:number} | null>(null)
  const [viewBusyOwner, setViewBusyOwner] = useState<string | null>(null)
  const [playbackFailure, setPlaybackFailure] = useState<{owner:string;error:string} | null>(null)
  const controlledObjectId = view?.objectId, controlledRevision = view?.revision
  const controlDocumentOwner = JSON.stringify([visible, threadId, target?.workspace, target?.path, view?.objectId, view?.revision, view?.changeSequence ?? 0])
  const controlOwner = JSON.stringify([controlDocumentOwner, view?.playback?.generation])
  const currentControlOwner = useRef(controlOwner); currentControlOwner.current = controlOwner
  const currentControlDocumentOwner = useRef(controlDocumentOwner); currentControlDocumentOwner.current = controlDocumentOwner
  const previewView = targetMatches && view?.status === 'ready' ? view.previewView : undefined
  const playback = targetMatches && view?.kind === 'pptx' ? view.playback : undefined
  const playbackActive = view?.status === 'ready' && (playback?.phase === 'running' || playback?.phase === 'paused')
  const presentationError = view?.error?.startsWith('presentation_') ? view.error : error?.startsWith('presentation_') ? error : undefined
  const playbackNeedsReopen = targetMatches && view?.kind === 'pptx' && view.status === 'error'
  const nativeActionError = error === 'native_input_unavailable' || error === 'composition_pending'
  const previewError = (nativeActionError ? null : error) ?? (playbackNeedsReopen ? view?.error ?? 'unavailable'
    : presentationError === 'presentation_start_timeout' ? presentationError : null)
  const playbackNotice = presentationError ? `${presentationErrors[presentationError]}${playbackNeedsReopen && presentationError !== 'presentation_start_timeout' ? ' 请重新打开预览。' : ''}`
    : view?.status === 'error' ? '放映状态未确认，请重新打开预览。' : undefined
  const playbackNeedsRefresh = view?.status === 'ready' && presentationError !== 'presentation_start_timeout' &&
    !!(presentationError || playback?.phase === 'unsupported' || playback?.phase === 'error' || playbackFailure?.owner === controlDocumentOwner)
  const controlReady = visible && !loading && targetMatches && view?.status === 'ready' && !view.saving
  const controlsBusy = viewBusyOwner === controlOwner || viewMenuOwner === controlOwner || busy
  useEffect(() => {
    if (!viewFocusReturn) return
    if (currentControlDocumentOwner.current !== viewFocusReturn.owner || generation.current !== viewFocusReturn.epoch) { setViewFocusReturn(null); return }
    if (controlsBusy || !controlReady || !viewButton.current?.isConnected || viewButton.current.disabled) return
    viewButton.current.focus(); setViewFocusReturn(null)
  }, [viewFocusReturn, controlsBusy, controlReady])
  const currentTarget = useRef(target); currentTarget.current = target
  const currentThread = useRef(threadId); currentThread.current = threadId
  const shown = useRef(visible); shown.current = visible
  const generation = useRef(0)
  const quoteInFlight = useRef(false)
  const actionInFlight = useRef(false)
  const manualDiscardAttempts = useNativeOfficeStore(state => state.manualDiscardAttempts)
  const surface = useRef<HTMLDivElement>(null)
  const menuHandler = useRef<(target?: NativeOfficeMenuTarget) => Promise<void>>(async () => {})
  const menuOpen = useRef(false)
  const pointerMenuOpen = useRef(false), menuSequence = useRef(0)
  const menuContext = useRef('')
  const formatInFlight = useRef<NativeFormatContext | null>(null)
  const queuedFormatMenu = useRef<{request:NativeOfficeMenuTarget;context:NativeFormatContext} | null>(null)
  const [formatQueueVersion,setFormatQueueVersion] = useState(0)
  useEffect(() => {
    const instance = instanceOwner, epoch = generation, showing = shown
    instance.current = renderedOwner.current.ownsSurface; showing.current = renderedOwner.current.visible
    return () => { epoch.current++; showing.current = false; instance.current = false }
  }, [])
  useEffect(() => window.analytix.office.onMenuRequested?.(target => { void menuHandler.current(target) }), [])
  useEffect(() => window.analytix.office.onAnnotationInputFreeze?.(frozen => useNativeOfficeStore.setState({annotationInputFrozen:frozen})), [])
  const bounds = () => {
    const rect = surface.current?.getBoundingClientRect()
    return { x: Math.max(0, Math.round(rect?.x ?? 0)), y: Math.max(0, Math.round(rect?.y ?? 0)),
      width: Math.max(1, Math.round(rect?.width ?? 1)), height: Math.max(1, Math.round(rect?.height ?? 1)) }
  }
  const invoke = useCallback(async (request: NativeOfficeRequest, acceptResult?: (result: NativeOfficeResponse) => boolean) => {
    if (!instanceOwner.current) return undefined
    const epoch = generation.current, context = menuContext.current
    const live = useNativeOfficeStore.getState(), selected = live.target
    const unresolved = nativeOfficeHasUnresolvedChange(live.view,live.error)
    if (request.action === 'undoChange' && live.view?.kind === 'xlsx' && (live.view.status !== 'ready' || live.view.saving || unresolved)) return undefined
    const formatting = request.action === 'format' ? currentFormatContext(live,currentThread.current) : null
    if (request.action === 'format' && (!shown.current || !formatting || !acceptResult ||
      request.objectId !== formatting.objectId || request.revision !== formatting.revision || request.changeId !== formatting.changeId ||
      request.expectedChangeSequence !== formatting.expectedChangeSequence || !rangeLeaseMatches(request.rangeLease,formatting.rangeLease) ||
      !nativeOfficeFormatSchema.safeParse(request.format).success)) return undefined
    const discard = request.action === 'discardManualEdit' ? request : null
    if (request.action === 'editWorkbookCell' && (!shown.current || !selected || selected.protectedPresent ||
      !nativeOfficeTargetMatches(selected,live.view) || live.view?.kind !== 'xlsx' || live.view.status !== 'ready' ||
      !live.view.workbookEditing?.editable || live.view.dirty || live.view.saving || unresolved ||
      request.threadId !== currentThread.current || request.objectId !== live.view.objectId || request.revision !== live.view.revision ||
      request.expectedChangeSequence !== (live.view.changeSequence ?? 0) || live.view.selection?.kind !== 'cells' ||
      request.selectionToken !== live.view.selection.token || live.view.selection.documentId !== live.view.objectId ||
      live.view.selection.version !== live.view.revision || live.view.selection.changeSequence !== (live.view.changeSequence ?? 0))) return undefined
    const discardAllowed = !!(discard && shown.current && selected && nativeOfficeTargetMatches(selected,live.view) &&
      !selected.protectedPresent && live.view?.kind === 'docx' && live.view.status !== 'loading' && !live.view.saving &&
      discard.threadId === currentThread.current && discard.objectId === live.view.objectId &&
      discard.revision === live.view.revision && discard.expectedChangeSequence === (live.view.changeSequence ?? 0) &&
      !manualConflict(live.view,live.error) && (manualRecoveryCanCancel(live.view,discard.threadId,discard.changeId) ||
        canRetryManualDiscard(live.view,nativeOfficeTargetKey(selected),discard.threadId,discard.changeId,live.manualDiscardAttempts[discard.objectId])))
    if (discard && !discardAllowed) return undefined
    if (unresolved && !discardAllowed &&
      ['beginManualEdit','editWorkbookCell','format','discardManualEdit','save','reference','acceptProposal','rejectProposal'].includes(request.action)) return undefined
    if (live.view?.manual?.frozen && ['beginManualEdit','format','reference'].includes(request.action)) return undefined
    if (live.view?.saving && ['discardManualEdit','cancelChange','resumeChange'].includes(request.action)) return undefined
    if (request.action === 'resumeChange') {
      const pending = live.view?.recovery?.pending
      if (!shown.current || !selected || !live.view || !pending || !nativeOfficeTargetMatches(selected,live.view) ||
        request.threadId !== currentThread.current || request.objectId !== live.view.objectId || request.revision !== live.view.revision ||
        request.expectedChangeSequence !== (live.view.changeSequence ?? 0) || pending.threadId !== request.threadId ||
        pending.changeId !== request.changeId || !pending.canResume) return undefined
    }
    if (selected?.protectedPresent && !['openPresent','bounds','hide','status','viewControl','close'].includes(request.action)) return undefined
    const phase = useNativeOfficeStore.getState().view?.playback?.phase
    if ((phase === 'running' || phase === 'paused') &&
      ['annotate','beginManualEdit','editWorkbookCell','format','discardManualEdit','undoChange','cancelChange','resumeChange','save','annotationSave','annotationRetry','reference','acceptProposal','rejectProposal'].includes(request.action)) return undefined
    const guardedAction = ['annotate','beginManualEdit','editWorkbookCell','format','discardManualEdit','undoChange','cancelChange','resumeChange','save','saveStatus','reference','acceptProposal','rejectProposal'].includes(request.action)
    if (guardedAction && actionInFlight.current) return undefined
    if (guardedAction) actionInFlight.current = true
    if (formatting) formatInFlight.current = formatting
    let formatCompleted = false
    setBusy(true)
    if (request.action === 'open' || request.action === 'openPresent') {
      request = { ...request, appearance: appearance() }
      useNativeOfficeStore.setState({ view: null, error: null })
    }
    const cancellation: NativeOfficeManualDiscardAttempt | null = discard && selected ? {owner:nativeOfficeTargetKey(selected),
      objectId:discard.objectId,threadId:discard.threadId,changeId:discard.changeId,baseRevision:discard.revision} : null
    if (cancellation) useNativeOfficeStore.setState(state => ({manualDiscardAttempts:{...state.manualDiscardAttempts,[cancellation.objectId]:cancellation}}))
    try {
      const result = await window.analytix.office.request(request)
      if (cancellation && (result.ok || ['invalid_request','forbidden','conflict','stale_selection'].includes(result.error ?? ''))) finishManualDiscard(cancellation)
      if (request.action === 'format') {
        if (!acceptResult?.(result)) return undefined
        if (!result.ok) {
          if (formatting && formatContextMatches(currentFormatContext(useNativeOfficeStore.getState(),currentThread.current),formatting))
            useNativeOfficeStore.setState({error:result.error ?? 'unavailable'})
          return result
        }
        formatCompleted = true
        const current = useNativeOfficeStore.getState()
        const next = result.view ? currentFormatContext({...current,view:result.view,error:result.view.error ?? null},currentThread.current) : null
        if (next && formatContextMatches(currentFormatContext(current,currentThread.current),next)) return result
      }
      if (request.action === 'reference' || request.action === 'annotate' || request.action === 'beginManualEdit') {
        // Core revokes all earlier scopes for this object, even if focus moved
        // while capture was in flight. Reflect that fact without accepting old UI.
        if (request.action === 'reference' && result.ok && result.view?.scope) useNativeReferenceStore.getState().revokeScopes(request.objectId)
        if (acceptResult && !acceptResult(result)) return undefined
        const current = useNativeOfficeStore.getState().view
        if (request.action === 'reference' && !acceptResult && context !== menuContext.current) return undefined
        // Only the explicit save-first consumer can accept Host's newly saved
        // revision. Its callback also fences every observed selection/version.
        const savedReference = request.action === 'reference' && request.saveFirst === true && !!acceptResult
        if (!current || current.objectId !== request.objectId || !savedReference && (current.revision !== request.revision ||
          (current.changeSequence ?? 0) !== request.expectedChangeSequence)) return undefined
        if (acceptResult && !result.ok) {
          // A current refusal remains visible without restoring its possibly
          // older view or treating it as a successful scope acquisition.
          if (instanceOwner.current && epoch === generation.current && selected === useNativeOfficeStore.getState().target) {
            useNativeOfficeStore.setState({error:result.error ?? 'unavailable'})
          }
          return result
        }
      }
      if (instanceOwner.current && epoch === generation.current) useNativeOfficeStore.getState().receive(result.view, result.ok ? null : result.error ?? 'unavailable')
      if (instanceOwner.current && epoch === generation.current && selected === useNativeOfficeStore.getState().target && !shown.current) void window.analytix.office.request({action:'hide'})
      return result
    } catch {
      if (instanceOwner.current && epoch === generation.current && (!formatting || shown.current &&
        formatContextMatches(currentFormatContext(useNativeOfficeStore.getState(),currentThread.current),formatting)))
        useNativeOfficeStore.setState({ error: cancellation || request.action === 'editWorkbookCell' ? 'unknown' : 'unavailable' })
    }
    finally {
      if (guardedAction) actionInFlight.current = false
      if (formatting && formatInFlight.current === formatting) {
        formatInFlight.current = null
        if (formatCompleted && queuedFormatMenu.current) setFormatQueueVersion(value => value + 1)
        else queuedFormatMenu.current = null
      }
      if (epoch === generation.current) setBusy(false)
    }
  }, [])
  useEffect(() => window.analytix.office.onChange((next) => { if (instanceOwner.current) useNativeOfficeStore.getState().receive(next, next?.error ?? null) }), [])
  const controlView = useCallback(async (control: Extract<NativeOfficeRequest, {action:'viewControl'}>['control'], zoom?: number) => {
    const current = useNativeOfficeStore.getState(), owner = controlOwner
    if (!visible || !target || current.target !== target || !current.view || current.view.status !== 'ready' || current.view.saving ||
      current.view.objectId !== controlledObjectId || current.view.revision !== controlledRevision || viewPending.current?.owner === owner) return
    const playControl = control.startsWith('play-'), state = current.view.playback
    if (playControl && current.view.kind !== 'pptx') return
    if (!playControl && (state?.phase === 'running' || state?.phase === 'paused')) return
    if ((control === 'play-start' || control === 'play-current') && current.view.dirty) return
    if (playControl && control !== 'play-read' && !state) return
    const operation = ++viewOperation.current, epoch = generation.current, received = current.receiveSequence
    viewPending.current = {owner, operation}; setViewBusyOwner(owner)
    const owns = () => currentControlOwner.current === owner && generation.current === epoch &&
      viewPending.current?.operation === operation && useNativeOfficeStore.getState().target === target
    try {
      const result = await window.analytix.office.request({action:'viewControl', objectId:current.view.objectId,
        revision:current.view.revision, expectedChangeSequence:current.view.changeSequence ?? 0, control,
        ...(control === 'set-zoom' ? {zoom} : {}),
        ...(playControl && control !== 'play-read' ? {playbackGeneration:state!.generation} : {})})
      if (playControl && owns() && received === useNativeOfficeStore.getState().receiveSequence) {
        setPlaybackFailure(result.ok ? null : {owner:controlDocumentOwner, error:result.error ?? 'unavailable'})
      }
      if (owns() && received === useNativeOfficeStore.getState().receiveSequence && result.view?.objectId === current.view.objectId && result.view.revision === current.view.revision) {
        useNativeOfficeStore.getState().receive(result.view, playControl && result.error === 'presentation_start_timeout' ? result.error : null)
      }
    } catch {
      if (owns() && received === useNativeOfficeStore.getState().receiveSequence) {
        if (playControl) { setPlaybackFailure({owner:controlDocumentOwner, error:'unavailable'}); return }
        useNativeOfficeStore.setState(state => state.view?.objectId === current.view!.objectId && state.view.revision === current.view!.revision
          ? {view:{...state.view, previewView:undefined}} : {})
      }
    } finally {
      if (viewPending.current?.operation === operation) { viewPending.current = null; setViewBusyOwner(null) }
    }
  }, [controlOwner, controlDocumentOwner, target, controlledObjectId, controlledRevision, visible])
  const needsViewRead = controlReady && !!target && !previewView && !playbackActive
  const needsPlaybackRead = controlReady && !!target && view?.kind === 'pptx' && !playback
  const openViewMenu = async () => {
    if (!controlReady || controlsBusy || menuOpen.current && !pointerMenuOpen.current || typeof window.analytix.office.showActionMenu !== 'function') return
    const owner = controlOwner, epoch = generation.current, current = useNativeOfficeStore.getState()
    if (!current.view || !view || current.target !== target || current.view.objectId !== view.objectId || current.view.revision !== view.revision) return
    type Control = Extract<NativeOfficeRequest, {action:'viewControl'}>['control']
    const items: Array<{id:string;label:string;enabled:boolean;control:Control;zoom?:number}> = []
    if (current.view.kind === 'pptx') {
      const state = current.view.playback, active = state?.phase === 'running' || state?.phase === 'paused'
      const startable = !!state && !active && state.phase !== 'unsupported' && !current.view.dirty
      items.push(
        {id:'view-play-start',label:'放映（按文档设置）',enabled:startable,control:'play-start'},
        {id:'view-play-current',label:'从当前页放映（不使用自定义顺序）',enabled:startable,control:'play-current'},
        {id:'view-play-previous-effect',label:'上一步动画',enabled:state?.phase === 'running' && state.slideIndex !== null && state.slideIndex >= 0,control:'play-previous-effect'},
        {id:'view-play-next-effect',label:'下一步动画',enabled:state?.phase === 'running',control:'play-next-effect'},
        {id:'view-play-previous-slide',label:'上一张幻灯片',enabled:active && state.slideIndex !== null && state.slideIndex > 0,control:'play-previous-slide'},
        {id:'view-play-next-slide',label:'下一张幻灯片',enabled:active && state.slideIndex !== null && state.slideCount !== null && state.slideIndex < state.slideCount - 1,control:'play-next-slide'},
        {id:'view-play-pause',label:'暂停放映',enabled:state?.phase === 'running',control:'play-pause'},
        {id:'view-play-resume',label:'继续放映',enabled:state?.phase === 'paused',control:'play-resume'},
        {id:'view-play-end',label:'结束放映',enabled:active,control:'play-end'},
        {id:'view-play-read',label:'刷新放映状态',enabled:true,control:'play-read'},
        {id:'view-previous-page',label:'上一页',enabled:!active && !!previewView?.page && previewView.page > 1,control:'previous-page'},
        {id:'view-next-page',label:'下一页',enabled:!active && !!previewView?.page && !!previewView.pages && previewView.page < previewView.pages,control:'next-page'})
    }
    items.push({id:'view-fit',label:t('writeImageFitShort'),enabled:!playbackActive && !!previewView,control:'fit'},
      {id:'view-zoom-out',label:'缩小',enabled:!playbackActive && !!previewView && previewView.zoom > 20,control:'zoom-out'},
      {id:'view-zoom-in',label:'放大',enabled:!playbackActive && !!previewView && previewView.zoom < 400,control:'zoom-in'})
    for (const zoom of [25,50,100,200]) items.push({id:`view-zoom-${zoom}`,label:`${zoom}%`,enabled:!playbackActive && !!previewView,control:'set-zoom',zoom})
    if (!previewView) items.push({id:'view-read',label:'重试视图',enabled:!playbackActive,control:'read'})
    const menuId = ++menuSequence.current
    menuOpen.current = true; pointerMenuOpen.current = false; setViewMenuOwner(owner)
    let menuHeld = true
    try {
      const result = await window.analytix.office.showActionMenu({objectId:current.view.objectId,revision:current.view.revision,
        expectedChangeSequence:current.view.changeSequence ?? 0,actions:items.map(({id,label,enabled}) => ({id,label,enabled}))})
      if (menuId !== menuSequence.current || currentControlOwner.current !== owner || generation.current !== epoch || useNativeOfficeStore.getState().target !== target) return
      const action = items.find(item => item.id === result.actionId && item.enabled)
      menuOpen.current = false; setViewMenuOwner(null); menuHeld = false
      if (action) await controlView(action.control, action.zoom)
    } catch { /* A cancelled or retired native menu preserves the current view. */ }
    finally {
      if (currentControlDocumentOwner.current === controlDocumentOwner && generation.current === epoch && viewButton.current?.isConnected) setViewFocusReturn({owner:controlDocumentOwner,epoch})
      if (menuHeld && menuId === menuSequence.current) { menuOpen.current = false; setViewMenuOwner(null) }
    }
  }
  const savedAnnotation = !readonlyPresent && targetMatches && view?.annotation?.threadId === threadId ? view.annotation : undefined
  useEffect(() => {
    if (!savedAnnotation) return
    const state = useNativeReferenceStore.getState(), current = state.drafts[draftKey]
    if (current?.dirty) return
    if (current?.note === savedAnnotation.note && current.sourceRevision === savedAnnotation.sourceRevision) return
    // Disk drafts restore text only, never a native selection token or scope.
    state.setDraft(draftKey,{...current,note:savedAnnotation.note,sourceRevision:savedAnnotation.sourceRevision})
  }, [draftKey,savedAnnotation])
  useEffect(() => {
    generation.current++
    setAdded(false)
    setBusy(false)
    if (!ownsSurface || !target || !visible) return
    void invoke(target.protectedPresent ? {action:'openPresent',protectedPresent:target.protectedPresent,bounds:bounds()} : { action: 'open', workspace:target.workspace,path:target.path, threadId, bounds: bounds() })
  }, [ownsSurface, target, targetIdentity, visible, threadId, invoke]) // Main restores the retained object session; hidden tabs never open a surface.
  useEffect(() => {
    let current = true
    void (async () => {
      if (needsViewRead) await controlView('read')
      if (current && needsPlaybackRead) await controlView('play-read')
    })()
    return () => { current = false }
  }, [needsViewRead, needsPlaybackRead, controlView])
  useEffect(() => {
    const element = surface.current
    if (!ownsSurface || !element) return
    const hide = () => { if (instanceOwner.current) void window.analytix.office.request({ action: 'hide' }).catch(() => undefined) }
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
  }, [ownsSurface, visible, targetIdentity])
  const scopeId = view?.scope?.scopeId, scopeThreadId = view?.scope?.threadId, objectId = view?.objectId
  useEffect(() => {
    if (readonlyPresent || !visible || !view?.scope?.editable || !scopeId || !objectId || !threadId || scopeThreadId !== threadId) return
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
  }, [readonlyPresent, visible, objectId, scopeId, scopeThreadId, threadId, view?.scope?.editable])
  const liveSelection = readonlyPresent ? undefined : view?.selection
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
  const hasSelection = selectionMatches && !!selection && (!!nativeSelectionText(selection).trim() || (selection.kind === 'shapes' && !!selection.presentation))
  const annotationIdentity = JSON.stringify(selection)
  useEffect(() => {
    setAdded(false)
    if (!readonlyPresent && selectionMatches && selection) {
      const state = useNativeReferenceStore.getState()
      state.setDraft(draftKey, {...state.drafts[draftKey],note:state.drafts[draftKey]?.note ?? '',selection:structuredClone(selection)})
    }
  }, [readonlyPresent, draftKey, annotationIdentity, selectionMatches])
  const quote = async (allowEdit: boolean, actionPrompt?: string, prepareDraft = false, selected = selection) => {
    // Pointer leases name the fresh native capture; frozen note/proposal actions keep their original selection.
    const selection = selected
    if (readonlyPresent || playbackActive || quoteInFlight.current || !visible || !view || !selection || !target || !targetMatches || selection.documentId !== view.objectId || !(nativeSelectionText(selection).trim() || selection.kind === 'shapes' && selection.presentation) || !threadId || !targetRequest || !selection.token) return false
    const current = useNativeOfficeStore.getState().view
    if (!shown.current || currentThread.current !== threadId || currentTarget.current !== target ||
      current?.objectId !== view.objectId || current.revision !== selection.version ||
      (current.changeSequence ?? 0) !== selection.changeSequence) return false
    if (current.dirty || current.saving || current.manual?.frozen || current.status !== 'ready') return false
    if (allowEdit && !isNativeSelectionEditable({...current,editing:true}, selection)) return false
    const chosen = structuredClone(selection), epoch = generation.current, owner = currentTarget.current
    const permittedSelections = new Set([JSON.stringify(current.selection), JSON.stringify(chosen)])
    const observedSelections = new Set(permittedSelections)
    let selectionOverflowed = false
    // Main can publish its normal annotation recapture before the IPC reply.
    // Remember intermediate selections too: accepting the latest reply must
    // not conceal a newer user selection that it would overwrite.
    const unsubscribe = useNativeOfficeStore.subscribe(state => {
      const identity = JSON.stringify(state.view?.selection)
      // At most the initial live capture, clicked capture and own recapture.
      if (!observedSelections.has(identity) && observedSelections.size >= 3) selectionOverflowed = true
      else observedSelections.add(identity)
    })
    const owns = () => {
      const state = useNativeOfficeStore.getState(), next = state.view
      return epoch === generation.current && instanceOwner.current && shown.current && currentThread.current === threadId &&
        owner === currentTarget.current && owner === state.target && !state.target?.protectedPresent && !!next && !next.protectedPresent &&
        next.objectId === view.objectId && next.revision === chosen.version && (next.changeSequence ?? 0) === chosen.changeSequence && !next.dirty && !next.saving && !next.manual?.frozen &&
        (!allowEdit || next.status === 'ready' && !next.saving) &&
        !['running','paused'].includes(next.playback?.phase ?? '')
    }
    const selectionCurrent = () => !selectionOverflowed && [...observedSelections].every(identity => permittedSelections.has(identity)) &&
      permittedSelections.has(JSON.stringify(useNativeOfficeStore.getState().view?.selection))
    const editorCurrent = () => {
      const latest = useNativeOfficeStore.getState().view
      return !!latest && isNativeSelectionEditable(latest, chosen)
    }
    const scopeMatches = (result: NativeOfficeResponse) => {
      const scope = result.view?.scope
      return !!(result.ok && result.view && !result.view.protectedPresent && result.view.objectId === view.objectId &&
        result.view.revision === chosen.version && (result.view.changeSequence ?? 0) === chosen.changeSequence && !result.view.dirty && !result.view.saving && !result.view.manual?.frozen &&
        (!allowEdit || result.view.status === 'ready' && !result.view.saving && isNativeSelectionEditable(result.view, chosen)) &&
        scope && /^[a-f0-9]{48}$/.test(scope.scopeId) && scope.editable === allowEdit &&
        scope.threadId === threadId && scope.selectionToken === chosen.token && scope.baseRevision === chosen.version && scope.changeSequence === chosen.changeSequence)
    }
    quoteInFlight.current = true
    try {
      if (allowEdit && !current.editing) {
        const annotated = await invoke({action:'annotate',...targetRequest}, result => {
          if (!owns()) return false
          if (!result.ok) return selectionCurrent()
          if (!result.view || !isNativeSelectionEditable(result.view, chosen)) return false
          permittedSelections.add(JSON.stringify(result.view.selection))
          return selectionCurrent()
        })
        if (!annotated?.ok || !owns() || !selectionCurrent()) return false
      }
      const editingView = useNativeOfficeStore.getState().view
      if (allowEdit && (!editingView || !isNativeSelectionEditable(editingView, chosen))) return false
      // Keep the clicked token: it still owns its native range/rectangle/shape.
      // A recaptured DOCX range with equal text is not proof of the same target.
      const result = await invoke({action:'reference', ...targetRequest, selectionToken:chosen.token!, threadId, editable:allowEdit},
        result => owns() && selectionCurrent() && (!allowEdit || editorCurrent()) &&
          (!result.ok || scopeMatches(result) && permittedSelections.has(JSON.stringify(result.view?.selection))))
      if (!result?.view || !owns() || !selectionCurrent() || !scopeMatches(result)) return false
      const scope = result?.view?.scope
      if (!scope) return false
      const annotationNote = [note.trim(), actionPrompt?.trim()].filter(Boolean).join('\n')
      const reference: NativeReference = {id:crypto.randomUUID(),threadId,scopeId:scope.scopeId,editable:allowEdit,workspace:target.workspace,path:view.path,objectId:view.objectId,revision:chosen.version,selection:chosen,label:nativeSelectionLabel(view,chosen),text:nativeSelectionText(chosen),...(annotationNote ? {note:annotationNote} : {})}
      if (prepareDraft) {
        if (!nativeOfficeDraftReferencesCurrent([reference],[result.view])) return false
        if (!onPrepareDraft?.(actionPrompt ?? '请根据所选内容提出修改建议。', [reference])) return false
      } else if (actionPrompt) onSubmitPrompt?.(actionPrompt, [reference])
      else useNativeReferenceStore.getState().add(reference)
      setAdded(!actionPrompt)
      onFocusConversation?.()
      return true
    } finally { unsubscribe(); quoteInFlight.current = false }
  }
  const manualPhase = nativeOfficeManualPhase(view)
  const manualUncertain = nativeOfficeHasUnresolvedChange(view,error)
  const workbookTransitioning = view?.kind === 'xlsx' && (view.status !== 'ready' || !!view.saving || manualUncertain)
  const manualCurrent = manualSessionCurrent(view)
  const manualSettled = [view?.recovery?.pending,view?.recovery?.current].some(change => change?.changeId === view?.manual?.changeId && !!change && ['committed','undone'].includes(change.status))
  const manualCanSave = !!threadId && manualCurrent && !view?.saving && !manualUncertain && !manualSettled
  const manualRecovery = [view?.recovery?.pending,view?.recovery?.current].find(change => change?.changeId === view?.manual?.changeId)
  const manualCanDiscard = !!(manualCurrent && view?.manual && threadId && !view.saving && !manualConflict(view,error) &&
    (manualRecoveryCanCancel(view,threadId,view.manual.changeId) || canRetryManualDiscard(view,target ? nativeOfficeTargetKey(target) : '',threadId,view.manual.changeId,manualDiscardAttempts[view.objectId]) ||
      !manualUncertain && !manualRecovery))
  const beginManualEdit = async () => {
    const state = useNativeOfficeStore.getState(), current = state.view
    if (!threadId || readonlyPresent || !visible || !target || state.target !== target || !current ||
      !targetMatches || nativeOfficeManualPhase(current) !== 'preview' || current.dirty || current.saving ||
      current.recovery?.pending || manualUncertain || actionInFlight.current) return
    const epoch = generation.current
    await invoke({action:'beginManualEdit',objectId:current.objectId,revision:current.revision,expectedChangeSequence:current.changeSequence ?? 0,threadId}, result => {
      if (epoch !== generation.current || !shown.current || currentThread.current !== threadId || currentTarget.current !== target) return false
      if (!result.ok) return true
      if (!result.view || result.view.objectId !== current.objectId || result.view.revision !== current.revision ||
        (result.view.changeSequence ?? 0) !== (current.changeSequence ?? 0) ||
        !['active','frozen'].includes(nativeOfficeManualPhase(result.view))) {
        useNativeOfficeStore.setState({error:'invalid_response'}); return false
      }
      const manual = result.view.manual
      if (!manual) return false
      useNativeOfficeStore.setState(state => ({manualOwners:{...state.manualOwners,[current.objectId]:{
        owner:nativeOfficeTargetKey(target),objectId:current.objectId,threadId,changeId:manual.changeId,
        baseRevision:manual.baseRevision,generation:manual.generation}}}))
      return true
    })
  }
  const manualAction = async (action: 'save' | 'discardManualEdit') => {
    const state = useNativeOfficeStore.getState(), current = state.view
    if (!visible || !shown.current || readonlyPresent || !threadId || currentThread.current !== threadId || !target ||
      currentTarget.current !== target || state.target !== target || !current || !nativeOfficeTargetMatches(target,current) ||
      !manualSessionCurrent(current) || !current.manual || current.saving || actionInFlight.current) return
    const requestTarget = {objectId:current.objectId,revision:current.revision,expectedChangeSequence:current.changeSequence ?? 0}
    if (action === 'save') {
      if (!nativeOfficeHasUnresolvedChange(current,state.error) &&
        ![current.recovery?.pending,current.recovery?.current].some(change => change?.changeId === current.manual?.changeId && !!change && ['committed','undone'].includes(change.status))) await invoke({action,...requestTarget})
      return
    }
    if (!threadId || manualConflict(current,state.error)) return
    const changeId = current.manual.changeId
    if (!manualRecoveryCanCancel(current,threadId,changeId) && !canRetryManualDiscard(current,nativeOfficeTargetKey(target),threadId,changeId,state.manualDiscardAttempts[current.objectId])) {
      if (nativeOfficeHasUnresolvedChange(current,state.error)) return
      // Begin authority need not include recovery flags. Query Core before the
      // first cancel; never infer cancellability from dirty=false or frozen.
      await invoke({action:'saveStatus',objectId:current.objectId})
    }
    const latest = useNativeOfficeStore.getState(), next = latest.view
    if (!shown.current || currentThread.current !== threadId || currentTarget.current !== target || latest.target !== target ||
      !next || next.objectId !== current.objectId || !manualSessionCurrent(next) || next.manual?.changeId !== changeId ||
      next.revision !== current.revision || next.saving || manualConflict(next,latest.error)) return
    if (manualRecoveryCanCancel(next,threadId,changeId) || canRetryManualDiscard(next,nativeOfficeTargetKey(target),threadId,changeId,latest.manualDiscardAttempts[next.objectId])) {
      await invoke({action,objectId:next.objectId,revision:next.revision,expectedChangeSequence:next.changeSequence ?? 0,threadId,changeId})
    }
  }
  const saveAndReference = async () => {
    const state = useNativeOfficeStore.getState(), before = state.view, selected = selection
    if (readonlyPresent || !visible || !threadId || !target || state.target !== target || !before || !selected ||
      before.kind !== 'docx' || !before.dirty || before.saving || before.manual?.frozen || before.status !== 'ready' || !targetMatches ||
      !hasSelection || !selected.token || manualUncertain || quoteInFlight.current || actionInFlight.current) return false
    const chosen = structuredClone(selected), epoch = generation.current
    const identity = (revision: string, sequence: number, selection: NativeOfficeSelection | undefined) => JSON.stringify([revision,sequence,selection])
    const observed = new Set([identity(before.revision,before.changeSequence ?? 0,before.selection)])
    let overflowed = false
    const unsubscribe = useNativeOfficeStore.subscribe(next => {
      if (!next.view || next.view.objectId !== before.objectId) return
      const value = identity(next.view.revision,next.view.changeSequence ?? 0,next.view.selection)
      if (!observed.has(value) && observed.size >= 8) overflowed = true
      else observed.add(value)
    })
    const owns = () => {
      const latest = useNativeOfficeStore.getState()
      return !overflowed && instanceOwner.current && shown.current && epoch === generation.current &&
        currentThread.current === threadId && currentTarget.current === target && latest.target === target &&
        !latest.target?.protectedPresent && latest.view?.objectId === before.objectId && !latest.view.protectedPresent &&
        !['running','paused'].includes(latest.view.playback?.phase ?? '')
    }
    const recaptured = (result: NativeOfficeResponse) => {
      if (!owns()) return null
      const after = result.view
      if (!result.ok || !after) return null
      const fresh = nativeOfficeSavedSelection(before,chosen,after,threadId)
      if (!fresh) return null
      // Host may invalidate the range while frozen. Any other capture or edit
      // sequence observed during the request prevents overwriting newer work.
      const allowed = new Set([
        identity(before.revision,before.changeSequence ?? 0,before.selection),
        identity(before.revision,before.changeSequence ?? 0,chosen),
        identity(before.revision,before.changeSequence ?? 0,undefined),
        identity(after.revision,after.changeSequence ?? 0,undefined),
        identity(after.revision,after.changeSequence ?? 0,fresh)
      ])
      return [...observed].every(value => allowed.has(value)) ? fresh : null
    }
    quoteInFlight.current = true
    try {
      const result = await invoke({action:'reference',objectId:before.objectId,revision:before.revision,
        expectedChangeSequence:before.changeSequence ?? 0,selectionToken:chosen.token!,threadId,editable:false,saveFirst:true},
      result => {
        if (!owns()) return false
        if (!result.ok || recaptured(result)) return true
        useNativeOfficeStore.setState({error:'stale_selection'}); return false
      })
      const fresh = result && recaptured(result), after = result?.view
      if (!fresh || !after?.scope) return false
      const reference: NativeReference = {id:crypto.randomUUID(),threadId,scopeId:after.scope.scopeId,editable:false,
        workspace:target.workspace,path:after.path,objectId:after.objectId,revision:after.revision,selection:fresh,
        label:nativeSelectionLabel(after,fresh),text:nativeSelectionText(fresh),...(note.trim() ? {note:note.trim()} : {})}
      if (!nativeOfficeDraftReferencesCurrent([reference],[after])) return false
      useNativeReferenceStore.getState().add(reference)
      setAdded(true); onFocusConversation?.(); return true
    } finally { unsubscribe(); quoteInFlight.current = false }
  }
  const decideProposal = async (proposalId: string, accept: boolean) => {
    const state = useNativeOfficeStore.getState(), current = state.view, scope = current?.scope
    const proposal = current?.proposals?.find(item => item.proposalId === proposalId)
    if (!visible || readonlyPresent || playbackActive || state.target !== target || !targetMatches || !current ||
      current.status !== 'ready' || current.saving || current.dirty || !scope || scope.threadId !== threadId ||
      proposal?.status !== 'proposed' || current.appliedProposals?.includes(proposalId) || actionInFlight.current) return
    if (accept && (!scope.editable || scope.baseRevision !== current.revision || scope.changeSequence !== (current.changeSequence ?? 0) ||
      !current.localReviews?.some(review => review.proposalId === proposalId))) return
    await invoke(accept ? {action:'acceptProposal',objectId:current.objectId,revision:current.revision,
      expectedChangeSequence:current.changeSequence ?? 0,scopeId:scope.scopeId,proposalId}
      : {action:'rejectProposal',objectId:current.objectId,scopeId:scope.scopeId,proposalId})
  }
  const proposalSelectionCurrent = (current: NativeOfficeView | null) => {
    const scope = current?.scope
    return !!(current && scope?.editable && scope.threadId === threadId && selection &&
      selection.token === scope.selectionToken && selection.documentId === current.objectId &&
      selection.version === current.revision && scope.baseRevision === current.revision &&
      selection.changeSequence === (current.changeSequence ?? 0) && scope.changeSequence === selection.changeSequence &&
      (!current.selection || current.selection.token === scope.selectionToken) && isNativeSelectionEditable(current,selection))
  }
  const regenerateProposal = async (proposalId: string) => {
    const state = useNativeOfficeStore.getState(), current = state.view
    const proposal = current?.proposals?.find(item => item.proposalId === proposalId)
    if (!onSubmitPrompt || !visible || readonlyPresent || state.target !== target || !targetMatches ||
      !current || current.appliedProposals?.includes(proposalId) || !proposal ||
      !['proposed','rejected'].includes(proposal.status) || !proposalSelectionCurrent(current)) return false
    return quote(true,t('nativeOfficeRegeneratePrompt'),false,selection)
  }
  const errors: Record<string, string> = {forbidden:t('nativeOfficeForbidden'), conflict:'文件已在外部修改；草稿保留，请核对版本。', unknown:'修改结果尚未确认，请查询本次更新状态。', unsaved_changes:'文档更新尚未完成，请稍后重试。', stale_selection:'选区已过期，请重新选择。', unsupported_selection:'此选区暂不支持该操作。', package_changed:'插件版本已变化；草稿保留。', capacity:'已有两个原生文档，请先完成更新并关闭一个。', save_failed:'更新未完成，可重试；工作副本保留。', native_input_unavailable:t('nativeOfficeManualUnavailable'), composition_pending:t('nativeOfficeCompositionPending')}
  const canRequestEdit = !!(!readonlyPresent && !playbackActive && !workbookTransitioning && threadId && view?.status === 'ready' && !view.saving && !view.dirty && !view.manual?.frozen && selection && selectionMatches && isNativeSelectionEditable({...view,editing:true},selection))
  const formatSelection = async (context: NativeFormatContext, format: NativeOfficeFormat) => {
    const state = useNativeOfficeStore.getState()
    if (!visible || !shown.current || !instanceOwner.current || currentThread.current !== context.threadId || currentTarget.current !== context.target ||
      actionInFlight.current || !formatContextMatches(currentFormatContext(state,context.threadId),context)) return
    const epoch = generation.current, received = state.receiveSequence
    await invoke({action:'format',objectId:context.objectId,revision:context.revision,expectedChangeSequence:context.expectedChangeSequence,
      changeId:context.changeId,rangeLease:context.rangeLease,format}, result => {
      const current = useNativeOfficeStore.getState()
      if (!instanceOwner.current || epoch !== generation.current || !shown.current || currentThread.current !== context.threadId ||
        currentTarget.current !== context.target || current.target !== context.target) return false
      if (!result.ok) return current.receiveSequence === received && formatContextMatches(currentFormatContext(current,context.threadId),context)
      const next = result.view ? currentFormatContext({...current,view:result.view,error:result.view.error ?? null},context.threadId) : null
      if (!next || next.objectId !== context.objectId || next.revision !== context.revision || next.changeId !== context.changeId ||
        next.generation !== context.generation || next.expectedChangeSequence < context.expectedChangeSequence || rangeLeaseMatches(next.rangeLease,context.rangeLease)) {
        if (current.receiveSequence === received && formatContextMatches(currentFormatContext(current,context.threadId),context))
          useNativeOfficeStore.setState({error:'invalid_response'})
        return false
      }
      return current.receiveSequence === received && formatContextMatches(currentFormatContext(current,context.threadId),context) ||
        formatContextMatches(currentFormatContext(current,context.threadId),next)
    })
  }
  const selectionActions = (selected = selection, actionBusy = busy || playbackActive) => {
    const state = useNativeOfficeStore.getState(), current = state.view
    const workbookUncertain = current?.kind === 'xlsx' && nativeOfficeHasUnresolvedChange(current,state.error)
    const selectedMatches = !!(selected && current && selected.documentId === current.objectId && selected.version === current.revision && selected.changeSequence === (current.changeSequence ?? 0))
    const selectedHasContent = selectedMatches && !!selected && !!(nativeSelectionText(selected).trim() || selected.kind === 'shapes' && selected.presentation)
    const editable = !!(!readonlyPresent && !playbackActive && !workbookUncertain && threadId && current?.status === 'ready' && !current.saving && !current.dirty && !current.manual?.frozen && selected && selectedMatches && isNativeSelectionEditable({...current,editing:true},selected))
    const actions = nativeSelectionActions({quickActions,hasSelection:selectedHasContent,editable,busy:actionBusy,canSubmit:!!onSubmitPrompt,
      labels:{copy:t('nativeActionCopy'),quote:t('nativeActionQuote')},
      copy:async () => {
        try { if (!readonlyPresent && selected) await navigator.clipboard.writeText(nativeSelectionText(selected)) }
        catch { if (shown.current) useNativeOfficeStore.setState({error:'unavailable'}) }
      },
      quote:() => quote(false,undefined,false,selected),task:action => quote(action.mode === 'edit',action.prompt,false,selected)})
    if (current?.dirty || current?.saving || current?.manual?.frozen || workbookUncertain) for (const action of actions) if (action.kind !== 'local') action.enabled = false
    actions.splice(2,0,{id:'request-modify',label:'请求修改',kind:'proposal',enabled:!!(editable && onPrepareDraft && !actionBusy),run:() => quote(true,undefined,true,selected)})
    return actions
  }
  const actions = selectionActions()
  const contextIdentity = JSON.stringify([threadId,target?.workspace,target?.path,view?.objectId,view?.revision,view?.changeSequence,annotationIdentity])
  menuContext.current = contextIdentity
  menuHandler.current = async requested => {
    const inFlight = formatInFlight.current
    if (inFlight && requested?.pointer) {
      if (visible && shown.current && currentThread.current === inFlight.threadId && currentTarget.current === inFlight.target &&
        requested.objectId === inFlight.objectId && requested.revision === inFlight.revision && requested.expectedChangeSequence >= inFlight.expectedChangeSequence)
        queuedFormatMenu.current = {request:structuredClone(requested),context:inFlight}
      return
    }
    if (actionInFlight.current) return
    const pointer = requested?.pointer, state = useNativeOfficeStore.getState(), fresh = state.view
    const livePlayback = fresh?.playback?.phase === 'running' || fresh?.playback?.phase === 'paused'
    if (readonlyPresent || (pointer ? livePlayback : playbackActive) || !visible || !shown.current || busy && !pointer ||
      menuOpen.current && !pointerMenuOpen.current || !window.analytix.office.showActionMenu) return
    if (pointer && (!state.target || state.target !== target || currentTarget.current !== target || currentThread.current !== threadId ||
      state.target.protectedPresent || !nativeOfficeTargetMatches(state.target,fresh))) return
    const requestTarget = pointer && fresh ? {objectId:fresh.objectId,revision:fresh.revision,expectedChangeSequence:fresh.changeSequence ?? 0} : targetRequest
    if (!requestTarget || requested && (requested.objectId !== requestTarget.objectId || requested.revision !== requestTarget.revision || requested.expectedChangeSequence !== requestTarget.expectedChangeSequence)) return
    const selected = pointer ? fresh?.selection : selection
    if (pointer && (fresh?.kind !== 'docx' || fresh.protectedPresent || fresh.objectId !== requested.objectId || fresh.revision !== requested.revision ||
      (fresh.changeSequence ?? 0) !== requested.expectedChangeSequence || selected?.token !== pointer.selectionToken || selected.kind !== 'text' || !selected.text.trim())) return
    if (!pointer && !hasSelection) return
    const offered = pointer ? selectionActions(selected,actionInFlight.current || quoteInFlight.current || livePlayback) : actions
    const formatContext = pointer ? currentFormatContext(state,threadId) : null
    const formatting: NativeFormatMenu | undefined = formatContext ? {changeId:formatContext.changeId,rangeLease:formatContext.rangeLease,
      formatState:formatContext.formatState,labels:{fontFamily:t('nativeOfficeFormatFontFamily'),fontSize:t('nativeOfficeFormatFontSize'),
        bold:t('nativeOfficeFormatBold'),italic:t('nativeOfficeFormatItalic'),underline:t('nativeOfficeFormatUnderline'),mixed:t('nativeOfficeFormatMixed')}} : undefined
    const pointerContext = JSON.stringify([threadId,target?.workspace,target?.path,fresh?.objectId,fresh?.revision,fresh?.changeSequence,fresh?.manual?.changeId,fresh?.manual?.generation,fresh?.manual?.frozen,!!fresh?.saving])
    const pointerCurrent = () => {
      const latest = useNativeOfficeStore.getState()
      return !!pointer && currentThread.current === threadId && currentTarget.current === target && latest.target === target && !latest.target?.protectedPresent &&
        JSON.stringify([threadId,target?.workspace,target?.path,latest.view?.objectId,latest.view?.revision,latest.view?.changeSequence,latest.view?.manual?.changeId,latest.view?.manual?.generation,latest.view?.manual?.frozen,!!latest.view?.saving]) === pointerContext &&
        latest.view?.selection?.token === pointer.selectionToken
    }
    const epoch = generation.current
    const menuId = ++menuSequence.current
    menuOpen.current = true; pointerMenuOpen.current = !!pointer
    try {
      const choice = await window.analytix.office.showActionMenu({...requestTarget,...(pointer ? {pointer} : {}),...(formatting ? {formatting} : {}),actions:offered.map(({id,label,enabled}) => ({id,label:label.replace(/\p{Cc}/gu,' ').trim(),enabled}))})
      if (menuId !== menuSequence.current || epoch !== generation.current || !shown.current || (pointer ? !pointerCurrent() : menuContext.current !== contextIdentity)) return
      const parsed = nativeOfficeActionChoiceSchema.safeParse(choice)
      if (!parsed.success) return
      if ('format' in parsed.data) {
        const format = nativeOfficeFormatSchema.safeParse(parsed.data.format)
        const lease = nativeOfficeRangeLeaseSchema.safeParse('rangeLease' in parsed.data ? parsed.data.rangeLease : undefined)
        const changeId = 'changeId' in parsed.data ? parsed.data.changeId : null
        if (formatContext && format.success && lease.success && changeId === formatContext.changeId &&
          rangeLeaseMatches(lease.data,formatContext.rangeLease)) await formatSelection(formatContext,format.data)
        return
      }
      const action = offered.find(action => action.id === parsed.data.actionId)
      if (action?.enabled) await action.run()
    } catch { if (epoch === generation.current && shown.current && menuContext.current === contextIdentity) useNativeOfficeStore.setState({error:'unavailable'}) }
    finally { if (menuId === menuSequence.current) { menuOpen.current = false; pointerMenuOpen.current = false } }
  }
  useEffect(() => {
    const pending = queuedFormatMenu.current
    if (!pending || formatInFlight.current || actionInFlight.current) return
    queuedFormatMenu.current = null
    const current = currentFormatContext(useNativeOfficeStore.getState(),threadId)
    if (!visible || !shown.current || !current || current.target !== pending.context.target || current.threadId !== pending.context.threadId ||
      current.objectId !== pending.context.objectId || current.revision !== pending.context.revision ||
      current.changeId !== pending.context.changeId || current.generation !== pending.context.generation) return
    void menuHandler.current(pending.request)
  }, [formatQueueVersion,visible,target,threadId,view])

  // DOCX selection belongs to the native nearby menu. Keep notes and failure
  // feedback inside the existing fixed toolbar so a capture cannot resize it.
  const selectionWarning = selection?.capture?.truncated
    ? `已捕获 ${selection.capture.capturedCharacters} / ${selection.capture.totalCharacters} UTF-16 单元；引用不包含完整选区。` : ''
  const operationNotice = (error || view?.error || view?.saving || manualUncertain) && view && (docxAnnotation || view.editing || view.saving || nativeActionError)
    ? errors[error ?? view.error ?? 'unknown'] ?? t('nativeOfficeOperationFailed') : ''
  const noteNotice = view?.annotationSaving ? '正在保存备注…' : view?.annotationError || draft?.dirty ? '备注尚未保存'
    : savedAnnotation?.note ? `备注已保存${savedAnnotation.sourceRevision !== view?.revision ? '；文件版本已变化，请重新选择。' : ''}` : ''
  const selectionNotice = hasSelection ? canRequestEdit ? '可让 AI 提出局部修改，接受后应用。' : '仅供讨论，此选区不支持直接应用修改。'
    : note ? '选区已过期，请重新选择；备注已保留。' : view?.editing ? '请在文档中选择文本、单元格或文本框，再添加标注。' : '点击“标注”，再选择需要讨论或修改的内容。'
  const docxNotice = [operationNotice, selectionWarning, noteNotice, added ? '标注已加入对话，尚未发送。' : selectionNotice].filter(Boolean).join(' ')

  return <div className={styles.host} aria-busy={!!loading} onKeyDown={event => {
    if (event.nativeEvent.isComposing || event.keyCode === 229 || event.altKey || event.ctrlKey || event.metaKey) return
    if (event.target instanceof Element && event.target.closest('input, textarea, [contenteditable="true"]')) return
    if (event.key === 'F2' && !event.shiftKey && view?.kind === 'xlsx') {
      const input = event.currentTarget.querySelector<HTMLInputElement>('[data-workbook-cell-input]:not(:disabled)')
      if (input) { event.preventDefault(); input.focus(); input.select() }
      return
    }
    if (event.key === 'ContextMenu' || event.key === 'F10' && event.shiftKey) { event.preventDefault(); void menuHandler.current() }
  }}>
    <div className={`${styles.toolbar} ${docxAnnotation ? styles.docxToolbar : ''}`} role="toolbar" aria-label="文档操作">
      <div className={docxAnnotation ? styles.docxControls : styles.toolbarContents}>
      {docxAnnotation && operationNotice && (view.editing || view.saving || nativeActionError) && (view.saving || manualUncertain || (error ?? view.error) === 'save_failed') ? <button
        className={styles.noteAction} disabled={busy || playbackActive || !visible || !targetMatches}
        onClick={() => targetRequest && void invoke(manualUncertain || view.saving || view.manual ? {action:'saveStatus',objectId:view.objectId} : {action:'save',...targetRequest})}>{manualUncertain || view.saving || view.manual ? '查询结果' : '重试'}</button> : null}
      {!readonlyPresent && <Tooltip bubbleClassName={styles.tooltipBubble} label="标注" side="bottom" portal><button type="button" className={styles.iconButton} disabled={busy || playbackActive || loading || !targetRequest || !targetMatches || view?.status !== 'ready' || !!view?.editing} aria-label="标注文档" aria-pressed={!!view?.editing} onClick={() => targetRequest && void invoke({action:'annotate', ...targetRequest})}><Pencil /></button></Tooltip>}
      {docxAnnotation ? <>
        <button type="button" className={styles.viewButton} aria-label={t('nativeActionMore')}
          disabled={busy || playbackActive || !annotationAvailable || !hasSelection} onClick={() => void menuHandler.current()}><ChevronDown className={styles.viewIcon} aria-hidden="true" /><span>{t('nativeActionMore')}</span></button>
        {notesOpen ? <div className={styles.noteEntry} data-native-docx-notes>
          {hasSelection && selection || note ? <>
            <input aria-label="标注备注" placeholder="备注（可选）" disabled={!annotationAvailable || annotationInputFrozen || playbackActive} maxLength={4096}
              className={styles.noteInput} value={note} onChange={event => { setNote(event.target.value); setAdded(false) }} />
            {view.dirty ? <button type="button" className={styles.noteAction} disabled={!annotationAvailable || busy || !hasSelection || !!view.saving || !!view.manual?.frozen || manualUncertain}
              onClick={() => void saveAndReference()}><Quote aria-hidden="true" />{t('nativeOfficeSaveAndReference')}</button>
              : <button type="button" className={styles.noteAction} disabled={!annotationAvailable || busy || playbackActive || !hasSelection || !!view.saving || !!view.manual?.frozen || view.dirty}
                onClick={() => void actions.find(action => action.id === 'quote')?.run()}><Quote aria-hidden="true" />加入对话</button>}
            {actions.filter(action => action.id !== 'quote').slice(0,4).map(action => <button type="button" key={action.id} className={styles.noteAction}
              disabled={!annotationAvailable || !action.enabled} onClick={() => void action.run()}>{action.label}</button>)}
          </> : null}
          {view.annotationError || draft?.dirty ? <button type="button" className={styles.noteAction} disabled={!annotationAvailable || annotationInputFrozen || playbackActive} onClick={() => setNote(note)}>重试</button> : null}
          <button type="button" className={styles.noteAction} aria-label="关闭标注备注" onClick={() => setNotesOwner(null)}>关闭备注</button>
        </div> : <button type="button" className={styles.viewButton} aria-label="编辑标注备注" aria-expanded="false"
          disabled={!visible || loading || !targetMatches} onClick={() => setNotesOwner(annotationControlOwner)}>备注</button>}
      </> : null}
      {!readonlyPresent && view?.kind === 'docx' ? <>
        <button type="button" className={`${styles.viewButton} ${styles.docxTextButton}`} aria-label={t('nativeOfficeManualEdit')} aria-pressed={manualPhase === 'active'}
          disabled={busy || loading || !visible || !target || !threadId || !targetMatches || manualPhase !== 'preview' || !!view.dirty || !!view.saving || !!view.recovery?.pending || manualUncertain}
          onClick={() => void beginManualEdit()}>{t('nativeOfficeManualEdit')}</button>
        {view.manual ? <button type="button" className={`${styles.viewButton} ${styles.docxTextButton}`} disabled={busy || loading || !visible || !target || !targetMatches || !manualCanSave}
          onClick={() => void manualAction('save')}>{t('nativeOfficeManualSave')}</button> : null}
        {view.manual ? <button type="button" className={`${styles.viewButton} ${styles.docxTextButton}`} disabled={busy || loading || !visible || !target || !targetMatches || !manualCanDiscard}
          onClick={() => void manualAction('discardManualEdit')}>{t('nativeOfficeManualDiscard')}</button> : null}
      </> : null}
      {!readonlyPresent && view?.canUndo ? <Tooltip bubbleClassName={styles.tooltipBubble} label={view.recovery?.current?.canRetryUndo ? '完成撤销' : '撤销'} side="bottom" portal><button type="button" className={styles.iconButton} disabled={busy || playbackActive || view.saving || view.dirty || workbookTransitioning} aria-label={view.recovery?.current?.canRetryUndo ? '完成撤销' : '撤销修改'} onClick={() => targetRequest && threadId && void invoke({action:'undoChange', ...targetRequest, threadId})}><Undo2 /></button></Tooltip> : null}
      <span className={styles.status} role="status">{loading ? t('nativeOfficeStatus_loading') : view?.saving || view?.kind === 'xlsx' && manualUncertain ? '更新结果待确认' : view?.dirty ? '更新待完成' : manualPhase === 'active' ? t('nativeOfficeManualActive') : manualPhase === 'frozen' ? t('nativeOfficeManualFrozen') : view?.canUndo ? '已更新' : view?.editing ? '标注中' : error ? t('nativeOfficeOperationFailed') : '预览'}</span>
      <button ref={viewButton} type="button" className={styles.viewButton} data-native-view-controls
        aria-label="预览视图" aria-haspopup="menu" disabled={!controlReady || controlsBusy} onClick={() => void openViewMenu()}>
        {playbackActive ? <Play className={styles.viewIcon} aria-hidden="true" /> : <ScanEye className={styles.viewIcon} aria-hidden="true" />}
        {view?.kind === 'pptx' ? playbackActive ? <output data-native-playback-slide aria-label="放映幻灯片与总张数">{playback.slideIndex === null || playback.slideIndex < 0 ? '—' : playback.slideIndex + 1} / {playback.slideCount}</output> : <output data-native-preview-page aria-label="当前页与总页数">{previewView?.page && previewView.pages ? `${previewView.page} / ${previewView.pages}` : '—'}</output> : null}
        <span data-native-preview-zoom>{playbackActive ? playback.phase === 'paused' ? '已暂停' : '放映中' : previewView ? previewView.fit ? t('writeImageFitShort') : `${previewView.zoom}%` : '视图不可用'}</span><ChevronDown aria-hidden="true" />
      </button>
      {playbackActive ? <>
        <Tooltip bubbleClassName={styles.tooltipBubble} label={playback.phase === 'paused' ? '继续放映' : '暂停放映'} side="bottom" portal><button type="button" className={styles.iconButton} disabled={!controlReady || controlsBusy} aria-label={playback.phase === 'paused' ? '继续放映' : '暂停放映'} onClick={() => void controlView(playback.phase === 'paused' ? 'play-resume' : 'play-pause')}>{playback.phase === 'paused' ? <Play /> : <Pause />}</button></Tooltip>
        <Tooltip bubbleClassName={styles.tooltipBubble} label="结束放映" side="bottom" portal><button type="button" className={styles.iconButton} disabled={!controlReady || controlsBusy} aria-label="结束放映" onClick={() => { void controlView('play-end'); setViewFocusReturn({owner:controlDocumentOwner,epoch:generation.current}) }}><Square /></button></Tooltip>
      </> : null}
      <div className={styles.fileActions}>{fileActions}</div>
      </div>
      {docxAnnotation ? <div className={styles.docxFeedback} data-native-docx-feedback role={operationNotice ? 'alert' : 'status'} title={docxNotice}>{docxNotice}</div> : null}
    </div>
    {!readonlyPresent && targetMatches && view?.kind === 'xlsx' ? <WorkbookCellEditor view={view} threadId={threadId} workspace={target?.workspace} path={target?.path}
      active={visible && !loading && !!target} busy={busy || manualUncertain} onRequest={invoke} /> : null}
    {visible && targetMatches && view?.kind === 'pptx' ? <div data-native-playback-state role={playbackNotice || playback?.phase === 'unsupported' || playback?.phase === 'error' || playbackFailure?.owner === controlDocumentOwner ? 'alert' : 'status'} className="shrink-0 px-2 text-xs text-ds-muted">
      {playbackNotice ?? (playback?.phase === 'unsupported' ? presentationErrors.presentation_unsupported : playback?.phase === 'error' ? presentationErrors.presentation_failed : playbackActive ? `${playback.phase === 'paused' ? '已暂停' : '放映中'} · ${playback.slideIndex === null || playback.slideIndex < 0 ? '—' : playback.slideIndex + 1} / ${playback.slideCount}` : playback ? '放映未开始' : '放映状态未确认')}
      {!playbackNotice && playbackFailure?.owner === controlDocumentOwner ? <span> {presentationErrors[playbackFailure.error] ?? presentationErrors.unavailable}</span> : null}
      {playbackNeedsRefresh ? <button type="button" disabled={!controlReady || controlsBusy} aria-label="刷新放映状态" onClick={() => void controlView('play-read')}>刷新放映状态</button> : null}
    </div> : null}
    {!readonlyPresent && !docxAnnotation && (view?.status === 'ready' || view?.editing) ? <div className={styles.annotation} aria-label="文档标注">
      {hasSelection && selection || note ? <>
        <p className="truncate text-xs">{hasSelection && selection ? nativeSelectionLabel(view, selection) : '选区已过期，请重新选择；备注已保留。'}</p>
        <p className="text-[11px] text-ds-muted">{canRequestEdit ? '可让 AI 提出局部修改，接受后应用。' : '仅供讨论，此选区不支持直接应用修改。'}</p>
        <div className="flex items-center gap-2">
          <input aria-label="标注备注" placeholder="备注（可选）" disabled={annotationInputFrozen || playbackActive} maxLength={4096} className="min-w-0 flex-1 bg-transparent text-xs" value={note} onChange={event => { setNote(event.target.value); setAdded(false) }} />
          {view.dirty && view.kind === 'docx' ? <button className="inline-flex shrink-0 items-center gap-1 text-xs" disabled={busy || !hasSelection || !!view.saving || !!view.manual?.frozen || manualUncertain}
            onClick={() => void saveAndReference()}><Quote className="h-3.5 w-3.5" />{t('nativeOfficeSaveAndReference')}</button>
            : <button className="inline-flex shrink-0 items-center gap-1 text-xs" disabled={busy || playbackActive || !hasSelection || !!view.saving || !!view.manual?.frozen || view.dirty} onClick={() => void actions.find(action => action.id === 'quote')?.run()}><Quote className="h-3.5 w-3.5" />加入对话</button>}
        </div>
        {view.annotationSaving ? <p role="status" className="text-[11px] text-ds-muted">正在保存备注…</p> : view.annotationError || draft?.dirty ? <p role="status" className="text-[11px] text-ds-muted">备注尚未保存 <button onClick={() => setNote(note)}>重试</button></p> : savedAnnotation?.note ? <p role="status" className="text-[11px] text-ds-muted">备注已保存{savedAnnotation.sourceRevision !== view.revision ? '；文件版本已变化，请重新选择。' : ''}</p> : null}
        <div className="flex flex-wrap gap-2">{actions.filter(action => action.id !== 'quote').slice(0,4).map(action => <button key={action.id} className="text-xs text-ds-muted" disabled={!action.enabled} onClick={() => void action.run()}>{action.label}</button>)}<button className="text-xs text-ds-muted" disabled={busy || playbackActive || !hasSelection} onClick={() => void menuHandler.current()}>{t('nativeActionMore')}</button></div>
        {added ? <p role="status" className="text-[11px] text-ds-muted">标注已加入对话，尚未发送。</p> : null}
      </> : <p className="text-xs text-ds-muted">{view.editing ? '请在文档中选择文本、单元格或文本框，再添加标注。' : '点击“标注”，再选择需要讨论或修改的内容。'}</p>}
    </div> : null}
    {!docxAnnotation && selection?.capture?.truncated ? <p className="shrink-0 px-2 text-xs text-ds-muted">已捕获 {selection.capture.capturedCharacters} / {selection.capture.totalCharacters} UTF-16 单元；引用不包含完整选区。</p> : null}
    {!docxAnnotation && (error || view?.error || view?.saving || manualUncertain) && view && (view.editing || view.saving || nativeActionError || view.kind === 'xlsx') ? <div role="alert" className="flex shrink-0 items-center gap-2 px-2 text-xs">
      <p>{errors[error ?? view.error ?? 'unknown'] ?? t('nativeOfficeOperationFailed')}</p>
      {view.saving || manualUncertain || (error ?? view.error) === 'save_failed' ? <button disabled={busy || playbackActive || !visible || !targetMatches} className="shrink-0 text-xs" onClick={() => targetRequest && void invoke(manualUncertain || view.saving || view.manual || view.kind === 'xlsx' ? {action:'saveStatus',objectId:view.objectId} : {action:'save',...targetRequest})}>{manualUncertain || view.saving || view.manual || view.kind === 'xlsx' ? '查询结果' : '重试'}</button> : null}
    </div> : null}
    {!readonlyPresent && view?.scope?.threadId === threadId && view?.proposals?.length ? <div className="max-h-36 shrink-0 overflow-auto border-b border-ds-border px-2 py-1" aria-label="原生修改提案">
      {view.proposals.map(proposal => { const review = view.localReviews?.find(item => item.proposalId === proposal.proposalId); return <div key={proposal.proposalId} className="py-1 text-xs">
        <div className="my-1 overflow-hidden rounded border border-ds-border-muted font-mono text-xs">
          {review?.presentation ? <p>形状位置、尺寸或填充颜色修改；接受后验证原文件中其他对象保持不变。</p> : null}
          {review?.presentation ? <table className="w-full text-left"><caption>形状属性（位置与尺寸单位：1/100 毫米）</caption><thead><tr><th>属性</th><th>修改前</th><th>修改后</th></tr></thead><tbody>{(['x100thMm','y100thMm','width100thMm','height100thMm','fillRGB'] as const).map(key => <tr key={key}><th>{{x100thMm:'横坐标',y100thMm:'纵坐标',width100thMm:'宽度',height100thMm:'高度',fillRGB:'填充色'}[key]}</th><td>{review.presentation!.before.shapes[review.presentation!.before.targetShapeIndex][key]}</td><td>{review.presentation!.after.shapes[review.presentation!.after.targetShapeIndex][key]}</td></tr>)}</tbody></table> : null}
          {review?.workbook ? <p>单元格类型与公式修改；Core 验算结果不代表原生缓存值。</p> : null}
          <del aria-label="修改前" className="block whitespace-pre-wrap bg-red-50 px-2 py-1 text-red-800 no-underline dark:bg-red-950/30 dark:text-red-200">{review?.beforeText ?? '原文暂不可核实，请重新选择。'}</del>
          <ins aria-label="修改后" className="block whitespace-pre-wrap bg-emerald-50 px-2 py-1 text-emerald-800 no-underline dark:bg-emerald-950/30 dark:text-emerald-200">{review?.afterText ?? '修改内容暂不可核实。'}</ins>
        </div>
        {view.appliedProposals?.includes(proposal.proposalId) ? <span>{view.saving || view.dirty || workbookTransitioning ? '正在更新，结果待确认' : view.canUndo ? '已更新' : '修改已处理'}</span> : proposal.status === 'approved' ? <span>{t('nativeOfficeProposalApproved')}</span> : <div className="flex gap-3">
          {proposal.status === 'rejected' ? <span>已拒绝</span> : <>
          <button disabled={busy || playbackActive || view.dirty || view.saving || workbookTransitioning || !review || !targetRequest || !view.scope?.editable || view.scope.baseRevision !== view.revision || view.scope.changeSequence !== (view.changeSequence ?? 0)} onClick={() => void decideProposal(proposal.proposalId,true)}>应用修改</button>
          <button disabled={busy || playbackActive || view.dirty || view.saving || workbookTransitioning} onClick={() => void decideProposal(proposal.proposalId,false)}>拒绝</button></>}
          {onSubmitPrompt ? <button disabled={busy || !canRequestEdit || !proposalSelectionCurrent(view)} onClick={() => void regenerateProposal(proposal.proposalId)}>{t('nativeOfficeRegenerate')}</button> : null}
        </div>}
      </div>})}
    </div> : null}
    {!readonlyPresent && view?.recovery && [view.recovery.pending, view.recovery.current].filter(change => change?.threadId === threadId).map(change => change && <details key={change.changeId} className="max-h-40 shrink-0 overflow-auto border-b border-ds-border-muted px-2 py-1 text-xs">
      <summary>{change.status === 'committed' ? '已保存的修改' : change.status === 'undone' ? '已撤销的修改' : change.status === 'prepared' ? 'origin' in change && change.origin === 'manual' ? t('nativeOfficeManualDraft') : '已批准，尚未确认保存' : change.status === 'conflict' ? '文件已变化，保留修改记录' : '修改结果待确认'}</summary>
      {change.canResume ? <button disabled={busy || playbackActive || view.saving} className="my-1" onClick={() => targetRequest && threadId && void invoke({action:'resumeChange',...targetRequest,threadId,changeId:change.changeId})}>继续保存此修改</button> : null}
      {change.canCancel ? <button disabled={busy || playbackActive || view.saving} className="my-1" onClick={() => targetRequest && threadId && void invoke({action:'origin' in change && change.origin === 'manual' && change.version === 2 ? 'discardManualEdit' : 'cancelChange',...targetRequest,threadId,changeId:change.changeId})}>取消未保存的修改</button> : null}
      {change.status === 'unknown' ? <button disabled={busy} className="my-1" onClick={() => void invoke({action:'saveStatus',objectId:view.objectId})}>核实结果</button> : null}
      {'origin' in change ? <p>{t('nativeOfficeManualRecord')}</p> : <>
        <del aria-label="已记录的修改前" className="block whitespace-pre-wrap no-underline">{change.beforeText}</del>
        <ins aria-label="已记录的修改后" className="block whitespace-pre-wrap no-underline">{change.afterText}</ins>
      </>}
    </details>)}
    <div className={styles.viewport}>
    <div ref={surface} className={styles.surface} aria-label={t('nativeOfficeEditor')} />
    {loading || playbackNeedsReopen || presentationError === 'presentation_start_timeout' || (error && !nativeActionError && !error.startsWith('presentation_') && !view?.editing && !view?.saving) ? <div className={styles.state} data-native-office-state role={previewError || playbackNeedsReopen ? 'alert' : 'status'}>
      <FileText className={styles.stateIcon} aria-hidden="true" />
      <p>{previewError ? errors[previewError] ?? (previewError.startsWith('presentation_') ? presentationErrors[previewError] : undefined) ?? t('nativeOfficeOperationFailed') : t('nativeOfficeStatus_loading')}</p>
      {previewError && target ? <Tooltip bubbleClassName={styles.tooltipBubble} label={t('nativeOfficeRetry')} side="bottom" portal><button type="button" className={styles.iconButton} aria-label={t('nativeOfficeRetry')} onClick={() => void invoke(target.protectedPresent ? {action:'openPresent',protectedPresent:target.protectedPresent,bounds:bounds()} : {action:'open',workspace:target.workspace,path:target.path,threadId,bounds:bounds()})}><RotateCcw /></button></Tooltip> : null}
    </div> : null}
    </div>
  </div>
}
