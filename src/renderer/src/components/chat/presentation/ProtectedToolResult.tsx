import { createContext, useCallback, useContext, useId, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { ToolBlock } from '../../../agent/types'
import {
  toolResultLocalSelectorSchemaV1, toolResultLocalViewResultSchemaV1,
  toolResultLocalEffectResultSchemaV1, toolResultLocalNavigateResultSchemaV1, type ToolResultLocalPresentFileRequestV1, type ToolResultLocalDisplayV1
} from '../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { sharedActiveCaseContextTransitionPending, subscribeBeforeSharedActiveCaseContextChange } from '../../../data-analysis/services/analysis/stats-case-overview-resource'
import type { ToolRowState } from './chat/ToolRow'
import { toolRowState } from './chat/models/tool-call-model'
import { readCardModel } from './chat/models/read-card-model'
import { terminalCardModel } from './chat/models/terminal-card-model'
import { fileResultToolKind } from './chat/models/file-result-card-model'
import { ProtectedToolCardBody, type ProtectedToolCardModel } from './chat/toolviews/ToolCardBody'
import { ToolResultInspector, type ToolResultInspectorPresentation } from './chat/toolviews/ToolResultInspector'
import { ToolRawIO } from './chat/toolviews/ToolRawIO'
import { conversationCallRecord } from './chat/models/conversation-record-model'
import { DshInspectOutlineRegular } from './DshToolIcons'
import rowCss from './chat/ToolRow.module.css'
import { nativeScheduleOperation } from '../../schedule/schedule-turn'

import { previewWorkspaceFile } from '../../../lib/workspace-file-preview'

export const ProtectedToolResultScope = createContext<string | null>(null)

type InspectorSelection = {
  selectedLeafId: string | null
  select: (leafId: string) => void
  release: (leafId: string) => void
}
const ProtectedToolInspectorContext = createContext<InspectorSelection | null>(null)

function InspectorSelectionOwner({ scopeKey, children }: { scopeKey: string; children: ReactNode }) {
  // Only the current mounted leaf identity is shared. Its authorized snapshot
  // remains inside ProtectedToolResult, with the existing Host lease and clear.
  const [selected, setSelected] = useState<{ scopeKey: string; leafId: string } | null>(null)
  const selectedLeafId = selected?.scopeKey === scopeKey ? selected.leafId : null
  const select = useCallback((leafId: string) => setSelected({ scopeKey, leafId }), [scopeKey])
  const release = useCallback((leafId: string) => setSelected(current => current?.leafId === leafId ? null : current), [])
  useLayoutEffect(() => { setSelected(null) }, [scopeKey])
  const value = useMemo(() => ({ selectedLeafId, select, release }), [selectedLeafId, select, release])
  return <ProtectedToolInspectorContext.Provider value={value}>{children}</ProtectedToolInspectorContext.Provider>
}

/** One current Inspect selection per actual timeline, or standalone call tree. */
export function ProtectedToolInspectorSelection({ scopeKey, children }: { scopeKey: string; children: ReactNode }) {
  const inherited = useContext(ProtectedToolInspectorContext)
  return inherited ? <>{children}</> : <InspectorSelectionOwner scopeKey={scopeKey}>{children}</InspectorSelectionOwner>
}

/** Public admitted records share the existing selection, without opening a private lease. */
export function useRecordedToolInspectorSelection(leafId: string, enabled: boolean) {
  const selection = useContext(ProtectedToolInspectorContext)
  const release = selection?.release
  const [standalone, setStandalone] = useState(false)
  const returnFocus = useRef<HTMLElement | null>(null)
  const api = window.analytix.runtime
  const retire = useCallback(() => { setStandalone(false); release?.(leafId) }, [release, leafId])
  useLayoutEffect(() => {
    retire()
    if (!enabled) return
    const off = api.onToolResultLocalDisplayInvalidated?.(id => { if (id === null) retire() })
    const runtimeOff = api.onRuntimeStatus?.(status => { if (status.state !== 'running') retire() })
    const caseOff = subscribeBeforeSharedActiveCaseContextChange(retire)
    const hidden = () => { if (document.hidden) retire() }
    document.addEventListener('visibilitychange', hidden)
    return () => { retire(); off?.(); runtimeOff?.(); caseOff(); document.removeEventListener('visibilitychange', hidden) }
  }, [api, enabled, retire])
  return {
    selected: enabled && !sharedActiveCaseContextTransitionPending() && (selection ? selection.selectedLeafId === leafId : standalone),
    inspect: () => {
      if (!enabled || document.hidden || sharedActiveCaseContextTransitionPending()) return
      returnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
      if (selection) selection.select(leafId); else setStandalone(true)
    },
    close: () => { retire(); if (returnFocus.current?.isConnected) returnFocus.current.focus({ preventScroll: true }) }
  }
}
type CopySection = 'payload' | 'result' | 'schema' | 'request-url' | 'result-url'
const domainCapture = (kind: string) => kind === 'goal' || kind === 'todo' || kind === 'skill'
const readonlyCapture = (kind: string) => domainCapture(kind) || kind === 'job' || kind === 'inspect' || kind === 'schedule' || kind === 'present' || kind === 'file_result'
const retainedSummaryCapture = (display: ToolResultLocalDisplayV1) => Boolean(display.inspect) ||
  display.capture.kind === 'web_fetch' || display.capture.kind === 'search' || readonlyCapture(display.capture.kind)
const jobOperations = { task: 'spawn', delegate_task: 'spawn', parallel_tasks: 'spawn', list_jobs: 'list', wait: 'wait', bash_output: 'output', kill_shell: 'cancel', restart_job: 'restart' } as const
export type ProtectedToolResultContentActions = { onCopy: () => void; copied: boolean; pending: boolean; state?: ToolRowState }
type Controls = { open: boolean; pending: boolean; canOpen: boolean; bodyId: string; state?: ToolRowState; onToggle: () => void }
export type ProtectedToolResultViewControls = Controls & {
  display: ToolResultLocalDisplayV1 | null; body: ReactNode
  presentReference?: Omit<ToolResultLocalPresentFileRequestV1, 'index'>
  onOpenWebLink: (role: 'request' | 'result') => void
  inspect?: { label: string; run: () => void }
}

// Payload lives only in this mounted lazy leaf. No transcript/markdown, tool
// argument, model state, clipboard/Blob or cross-thread cache receives it.
export function ProtectedToolResult({ block, threadId, renderTrigger, renderView, renderContent, preload = false, onInspect,
  inspectRequest, inspectorOnly = false, inspectorPresentation = 'overlay', onInspectorClose, onInspectorRetire, hideUnavailable = false }: {
  block: ToolBlock; threadId?: string | null
  renderTrigger?: (controls: Controls) => ReactNode
  renderView?: (controls: ProtectedToolResultViewControls) => ReactNode
  renderContent?: (display: ToolResultLocalDisplayV1, openWebLink: (role: 'request' | 'result') => void, actions: ProtectedToolResultContentActions) => ReactNode
  preload?: boolean; onInspect?: (turnId: string, callId: string) => void
  inspectorPresentation?: ToolResultInspectorPresentation
  inspectRequest?: number; inspectorOnly?: boolean; onInspectorClose?: () => void; onInspectorRetire?: () => void
  /** Tail receipts have no placeholder; the tool Inspect path still reports unavailable reads. */
  hideUnavailable?: boolean
}) {
  const inheritedThread = useContext(ProtectedToolResultScope)
  const inspectorSelection = useContext(ProtectedToolInspectorContext)
  const releaseInspector = inspectorSelection?.release
  const selectInspector = inspectorSelection?.select
  const inspectorRetire = useRef(onInspectorRetire)
  inspectorRetire.current = onInspectorRetire
  const { t } = useTranslation('common')
  const [payload, setPayload] = useState<{ key: string; value: ToolResultLocalDisplayV1 } | null>(null)
  const rootRef = useRef<HTMLDivElement>(null)
  const bodyId = useId()
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const [message, setMessage] = useState('')
  const [copied, setCopied] = useState(false)
  const [inspectorOpen, setInspectorOpen] = useState(false)
  const inspectReturnFocus = useRef<HTMLElement | null>(null)
  const preloaded = useRef<string | null>(null), loadCurrent = useRef<(show: boolean) => void>(() => undefined)
  const inspectedRequest = useRef<number | undefined>(undefined)
  const [, setRetirementRevision] = useState(0)
  const copyCurrent = useRef<(section?: CopySection) => void>(() => undefined)
  const generation = useRef(0)
  const opening = useRef(false)
  const viewId = useRef<string | null>(null)
  const api = window.analytix.runtime
  const reference = toolResultLocalSelectorSchemaV1.omit({ threadId: true }).safeParse(block.meta?.localResult)
  const referenceMatches = reference.success &&
    (!block.meta?.turnId || reference.data.turnId === block.meta.turnId) &&
    (!block.meta?.callId || reference.data.callId === block.meta.callId)
  const selector = toolResultLocalSelectorSchemaV1.safeParse(referenceMatches ? {
    ...reference.data, threadId: threadId ?? inheritedThread
  } : null)
  const selectorKey = selector.success ? JSON.stringify(selector.data) : ''
  const scopeKey = selectorKey + ':' + block.status + ':' + Boolean(referenceMatches) + ':' +
    (typeof block.meta?.toolName === 'string' ? block.meta.toolName : '') + ':' + (block.toolKind ?? '')
  const inspectorLeafId = bodyId + ':' + scopeKey
  const inspectorSelected = inspectorSelection ? inspectorSelection.selectedLeafId === inspectorLeafId : inspectorOpen
  const closeInspector = useCallback(() => { setInspectorOpen(false); releaseInspector?.(inspectorLeafId) }, [releaseInspector, inspectorLeafId])
  const currentKey = useRef(scopeKey)
  const display = payload?.key === scopeKey && !sharedActiveCaseContextTransitionPending() ? payload.value : null
  const clear = useCallback(() => {
    const selection = document.getSelection()
    for (let index = 0; selection && index < selection.rangeCount; index++) {
      try {
        if ([...(rootRef.current?.querySelectorAll('[data-protected-tool-body],[data-protected-tool-summary]') ?? [])]
          .some(node => selection.getRangeAt(index).intersectsNode(node))) { selection.removeAllRanges(); break }
      } catch { /* Retired selection. */ }
    }
    generation.current += 1
    opening.current = false
    setRetirementRevision(value => value + 1)
    copyCurrent.current = () => undefined
    const id = viewId.current
    viewId.current = null
    setPayload(null); setPending(false); setCopied(false); setOpen(false); closeInspector(); setMessage('')
    if (id) void api.closeToolResultLocalDisplay(id).catch(() => undefined)
  }, [api, closeInspector])
  useLayoutEffect(() => {
    currentKey.current = scopeKey
    const off = api.onToolResultLocalDisplayInvalidated?.(id => {
      if (id === null || id === viewId.current) { clear(); if (inspectorOnly) inspectorRetire.current?.(); if (id !== null) setMessage(t('protectedToolUnavailable')) }
    })
    const runtimeOff = api.onRuntimeStatus?.(status => { if (status.state !== 'running') clear() })
    const caseOff = subscribeBeforeSharedActiveCaseContextChange(clear)
    const hidden = () => { if (document.hidden) clear(); else if (preload) loadCurrent.current(false) }
    document.addEventListener('visibilitychange', hidden)
    return () => { clear(); off?.(); runtimeOff?.(); caseOff(); document.removeEventListener('visibilitychange', hidden) }
  }, [clear, api, scopeKey, preload, t, inspectorOnly])

  useLayoutEffect(() => {
    if ((!open && !inspectorSelected && !(preload && display && retainedSummaryCapture(display))) || !display || !selector.success || !referenceMatches || block.status === 'running') return
    const intersects = (event: Event) => {
      const target = event.target instanceof Element ? event.target : null
      // Input/editor selections have their own copy semantics. A stale DOM
      // selection elsewhere must not take over a focused ordinary editor.
      for (const element of event.type === 'dragstart' ? [target] : [target, document.activeElement]) {
        if (element instanceof Element && element.closest('input,textarea,[contenteditable]:not([contenteditable="false"]),[role="textbox"]') &&
          !element.closest('[data-protected-tool-body],[data-protected-tool-summary]')) return []
      }
      const selection = document.getSelection()
      const matches: Element[] = []
      for (const body of document.querySelectorAll('[data-protected-tool-body],[data-protected-tool-summary]')) {
        let selected = false
        for (let index = 0; selection && !selection.isCollapsed && index < selection.rangeCount; index++) {
          try { if (selection.getRangeAt(index).intersectsNode(body)) selected = true } catch { /* Retired range. */ }
        }
        if (selected || event.type === 'dragstart' && target && body.contains(target)) {
          const leaf = body.closest('[data-protected-tool-result]')
          if (leaf && !matches.includes(leaf)) matches.push(leaf)
        }
      }
      return matches
    }
    const copy = (event: Event) => {
      const matches = intersects(event)
      if (!matches.length) return
      event.preventDefault()
      if (matches.length === 1 && matches[0] === rootRef.current) {
        const sections = new Set<CopySection>()
        const selection = document.getSelection()
        for (const node of rootRef.current.querySelectorAll('[data-protected-tool-readonly]')) {
          for (let index = 0; selection && !selection.isCollapsed && index < selection.rangeCount; index++) {
            try { if (selection.getRangeAt(index).intersectsNode(node)) return } catch { /* Retired range. */ }
          }
        }
        for (const node of rootRef.current.querySelectorAll<HTMLElement>('[data-protected-tool-copy-section]')) {
          for (let index = 0; selection && !selection.isCollapsed && index < selection.rangeCount; index++) {
            try { if (selection.getRangeAt(index).intersectsNode(node)) sections.add(node.dataset.protectedToolCopySection as CopySection) } catch { /* Retired range. */ }
          }
        }
        if (sections.size === 1 || sections.size === 0 && display.capture.kind !== 'web_fetch') copyCurrent.current(sections.values().next().value)
      }
    }
    const blockTransfer = (event: Event) => { if (intersects(event).length) event.preventDefault() }
    document.addEventListener('copy', copy, true)
    document.addEventListener('cut', blockTransfer, true)
    document.addEventListener('dragstart', blockTransfer, true)
    return () => {
      document.removeEventListener('copy', copy, true)
      document.removeEventListener('cut', blockTransfer, true)
      document.removeEventListener('dragstart', blockTransfer, true)
    }
  }, [open, inspectorSelected, preload, display, scopeKey, block.status, referenceMatches, selector.success])
  copyCurrent.current = () => undefined

  const load = async (show = true) => {
    if (!selector.success || !referenceMatches || block.status === 'running' || opening.current || sharedActiveCaseContextTransitionPending()) return
    if (display && viewId.current) { setOpen(show); return }
    clear()
    opening.current = true
    const epoch = generation.current
    const key = scopeKey
    const id = crypto.randomUUID()
    viewId.current = id
    setOpen(show); setPending(true)
    try {
      const parsed = toolResultLocalViewResultSchemaV1.safeParse(await api.openToolResultLocalDisplay({ ...selector.data, viewId: id }))
      if (epoch !== generation.current || currentKey.current !== key || viewId.current !== id || sharedActiveCaseContextTransitionPending()) return
      if (parsed.success && parsed.data.ok && parsed.data.display.resultItemId === selector.data.resultItemId) {
        const received = parsed.data.display
        const touchesWeb = block.meta?.toolName === 'web_fetch' || received.toolName === 'web_fetch' || received.capture.kind === 'web_fetch'
        const jobName = typeof block.meta?.toolName === 'string' && Object.hasOwn(jobOperations, block.meta.toolName)
          ? block.meta.toolName as keyof typeof jobOperations : null
        const jobKind = jobName ? jobOperations[jobName] === 'spawn' ? 'subagent' : 'command_execution' : null
        const scheduleOperation = nativeScheduleOperation(block.meta?.toolName)
        const requiresIdentity = jobName || received.inspect || received.capture.kind === 'job' || received.capture.kind === 'inspect' || received.capture.kind === 'file_result'
        const foreignJobCapture = jobName && received.capture.kind !== 'job' && received.capture.kind !== 'inspect'
        const foreignOrdinaryKind = received.inspect && (received.capture.kind === 'shell' || received.capture.kind === 'read') &&
          block.toolKind !== (received.capture.kind === 'shell' ? 'command_execution' : 'tool_call')
        if (requiresIdentity && received.toolName !== block.meta?.toolName || foreignJobCapture || foreignOrdinaryKind) {
          clear(); setMessage(t('protectedToolUnavailable'))
        } else if (received.capture.kind === 'job' && !(jobName &&
          block.toolKind === jobKind && received.capture.job.operation === jobOperations[jobName])) {
          clear(); setMessage(t('protectedToolUnavailable'))
        } else if ((scheduleOperation || received.capture.kind === 'schedule') && received.capture.kind !== 'inspect' &&
          !(scheduleOperation && block.toolKind === 'tool_call' && received.toolName === block.meta?.toolName &&
            received.capture.kind === 'schedule' && received.capture.schedule.operation === scheduleOperation)) {
          clear(); setMessage(t('protectedToolUnavailable'))
        } else if ((block.meta?.toolName === 'present' || received.capture.kind === 'present') && received.capture.kind !== 'inspect' &&
          !(block.meta?.toolName === 'present' && block.toolKind === 'tool_call' && received.toolName === 'present' &&
            received.capture.kind === 'present')) {
          clear(); setMessage(t('protectedToolUnavailable'))
        } else if ((fileResultToolKind(block.meta?.toolName) || received.capture.kind === 'file_result') && received.capture.kind !== 'inspect' &&
          !(received.capture.kind === 'file_result' && received.toolName === block.meta?.toolName && block.toolKind === fileResultToolKind(received.toolName))) {
          clear(); setMessage(t('protectedToolUnavailable'))
        } else if (received.capture.kind === 'inspect') {
          // Input/schema may remain available when no typed result was captured.
          // Preserve the actual admitted call kind; no public prose fills Result.
          const expectedKind = jobKind ?? fileResultToolKind(received.toolName) ?? (received.toolName === 'bash' ? 'command_execution'
            : touchesWeb || scheduleOperation || ['read', 'read_file', 'grep', 'glob', 'create_goal', 'get_goal', 'update_goal', 'todo_write', 'run_skill', 'present'].includes(received.toolName) ? 'tool_call' : block.toolKind)
          if (!expectedKind || block.toolKind !== expectedKind) { clear(); setMessage(t('protectedToolUnavailable')) }
          else setPayload({ key, value: received })
        } else if (touchesWeb && !(block.meta?.toolName === 'web_fetch' && block.toolKind === 'tool_call' &&
          received.toolName === 'web_fetch' && received.capture.kind === 'web_fetch')) {
          clear(); setMessage(t('protectedToolUnavailable'))
        } else if ((block.meta?.toolName === 'grep' || block.meta?.toolName === 'glob' || received.capture.kind === 'search') &&
          !(block.toolKind === 'tool_call' && received.toolName === block.meta?.toolName && received.capture.kind === 'search')) {
          clear(); setMessage(t('protectedToolUnavailable'))
        } else if ((domainCapture(received.capture.kind) || ['create_goal', 'get_goal', 'update_goal', 'todo_write', 'run_skill'].includes(String(block.meta?.toolName))) &&
          !(block.toolKind === 'tool_call' && received.toolName === block.meta?.toolName &&
            (['create_goal', 'get_goal', 'update_goal'].includes(received.toolName) ? received.capture.kind === 'goal' :
              received.toolName === 'todo_write' ? received.capture.kind === 'todo' : received.toolName === 'run_skill' && received.capture.kind === 'skill'))) {
          clear(); setMessage(t('protectedToolUnavailable'))
        } else setPayload({ key, value: received })
      } else { clear(); setMessage(t('protectedToolUnavailable')) }
    } catch {
      if (epoch === generation.current) { clear(); setMessage(t('protectedToolUnavailable')) }
    } finally {
      if (epoch === generation.current) { opening.current = false; setPending(false) }
    }
  }
  loadCurrent.current = show => { void load(show) }
  useLayoutEffect(() => {
    if (inspectRequest === undefined || inspectedRequest.current === inspectRequest || document.hidden) return
    inspectedRequest.current = inspectRequest
    loadCurrent.current(true)
  }, [inspectRequest])
  useLayoutEffect(() => {
    if (inspectRequest === undefined || !display || !retainedSummaryCapture(display)) return
    if (selectInspector) selectInspector(inspectorLeafId); else setInspectorOpen(true)
  }, [inspectRequest, display, selectInspector, inspectorLeafId])
  useLayoutEffect(() => {
    if (preload && selector.success && referenceMatches && block.status !== 'running' && preloaded.current !== scopeKey && !document.hidden) {
      preloaded.current = scopeKey; loadCurrent.current(false)
    }
  }, [preload, scopeKey, selector.success, referenceMatches, block.status])
  const effect = async (action: 'copy' | 'save', section?: CopySection) => {
    if (!selector.success || !display || readonlyCapture(display.capture.kind) ||
        display.inspect && (section === 'payload' || section === 'schema') || !viewId.current || pending ||
        currentKey.current !== scopeKey || sharedActiveCaseContextTransitionPending()) return
    const epoch = generation.current
    setPending(true); setMessage('')
    try {
      const request = { ...selector.data, viewId: viewId.current, snapshotDigest: display.snapshotDigest }
      const parsed = toolResultLocalEffectResultSchemaV1.safeParse(await (action === 'copy'
        ? api.copyToolResultLocalDisplay(section ? { ...request, section } : request) : api.saveToolResultLocalDisplay(request)))
      if (epoch !== generation.current) return
      if (parsed.success && parsed.data.ok) { if (action === 'copy') setCopied(true) }
      else if (parsed.success && !parsed.data.ok && parsed.data.code === 'canceled') return
      else if (parsed.success && !parsed.data.ok && parsed.data.code === 'unsupported') setMessage(t('protectedToolUnsupported'))
      else { clear(); setMessage(t('protectedToolUnavailable')) }
    } catch {
      if (epoch === generation.current) { clear(); setMessage(t('protectedToolUnavailable')) }
    } finally { if (epoch === generation.current) setPending(false) }
  }
  const navigate = async () => {
    if (!selector.success || !display?.canNavigate || !viewId.current || pending || currentKey.current !== scopeKey || sharedActiveCaseContextTransitionPending()) return
    const epoch = generation.current, key = scopeKey, id = viewId.current
    setPending(true); setMessage('')
    try {
      const request = { ...selector.data, viewId: id, snapshotDigest: display.snapshotDigest }
      const parsed = toolResultLocalNavigateResultSchemaV1.safeParse(await api.navigateToolResultLocalDisplay(request))
      if (epoch !== generation.current || currentKey.current !== key || viewId.current !== id || sharedActiveCaseContextTransitionPending()) return
      if (parsed.success && parsed.data.ok) previewWorkspaceFile({ ...parsed.data.target, preview: 'source', protectedRead: request })
      else { clear(); setMessage(t('protectedToolUnavailable')) }
    } catch {
      if (epoch === generation.current) { clear(); setMessage(t('protectedToolUnavailable')) }
    } finally { if (epoch === generation.current) setPending(false) }
  }
  const openWebLink = async (role: 'request' | 'result') => {
    if (!selector.success || !display?.canOpenWeb?.[role] || !viewId.current || pending ||
        currentKey.current !== scopeKey || sharedActiveCaseContextTransitionPending()) return
    const epoch = generation.current, key = scopeKey, id = viewId.current
    setPending(true); setMessage('')
    try {
      const parsed = toolResultLocalEffectResultSchemaV1.safeParse(await api.openToolResultLocalWebLink({
        ...selector.data, viewId: id, snapshotDigest: display.snapshotDigest, targetRole: role
      }))
      if (epoch !== generation.current || currentKey.current !== key || viewId.current !== id || sharedActiveCaseContextTransitionPending()) return
      if (!parsed.success || !parsed.data.ok) { clear(); setMessage(t('protectedToolUnavailable')) }
    } catch {
      if (epoch === generation.current) { clear(); setMessage(t('protectedToolUnavailable')) }
    } finally { if (epoch === generation.current) setPending(false) }
  }
  copyCurrent.current = display && !readonlyCapture(display.capture.kind) ? section => { void effect('copy', section) } : () => undefined
  const capture = display?.capture
  const read = display ? readCardModel(display) : null
  const terminal = display ? terminalCardModel(display) : null
  const model: ProtectedToolCardModel | null = read ? { kind: 'read', card: read } : terminal ? { kind: 'terminal', card: terminal } : null
  const captureState: ToolRowState | undefined = capture ? capture.status === 'canceled' ? 'stopped' : capture.status === 'unknown' ? 'unknown'
    : ['failed', 'timeout'].includes(capture.status) || capture.kind === 'shell' && capture.exitCode !== undefined && capture.exitCode !== 0 ||
      capture.kind === 'web_fetch' && capture.web.toolIsError || capture.kind === 'search' && capture.search.toolIsError ? 'error' : toolRowState(block) : undefined
  const onToggle = () => {
    if (open) {
      if (preload && display && retainedSummaryCapture(display)) { setOpen(false); closeInspector(); setCopied(false) }
      else clear()
    } else if (!pending) void load()
  }
  if (!referenceMatches || !selector.success || block.status === 'running') {
    const unavailable: Controls = { open: false, pending: false, canOpen: false, bodyId, onToggle: () => undefined }
    return renderView ? <>{renderView({ ...unavailable, display: null, body: null, onOpenWebLink: () => undefined })}</>
      : renderTrigger ? <>{renderTrigger(unavailable)}</> : null
  }
  const inspect = display && retainedSummaryCapture(display) ? { label: t('dshToolInspect.inspect'), run: () => {
    if (!pending && !sharedActiveCaseContextTransitionPending() && currentKey.current === scopeKey) {
      inspectReturnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
      if (onInspect) onInspect(selector.data.turnId, selector.data.callId)
      else if (inspectorSelection) inspectorSelection.select(inspectorLeafId); else setInspectorOpen(true)
    }
  } } : undefined
  const controls: Controls = { open, pending, bodyId, state: captureState,
    canOpen: open || !pending && !sharedActiveCaseContextTransitionPending(), onToggle }
  const body = open ? <div id={bodyId} data-protected-tool-body className={renderView ? undefined : renderTrigger ? 'mt-1 ml-[22px]' : undefined} inert={pending ? true : undefined}>
    {display && capture ? renderContent ? renderContent(display, role => { void openWebLink(role) }, { onCopy: () => void effect('copy'), copied, pending, state: captureState }) : <>
      {model ? <ProtectedToolCardBody key={display.snapshotDigest} model={model} copied={copied}
        onCopy={() => void effect('copy')} onSave={() => void effect('save')} onOpenFile={display.canNavigate ? () => void navigate() : undefined} />
        : capture.kind === 'inspect' ? display.inspect?.outputText ? <ToolRawIO outputText={display.inspect.outputText} /> : <p>{t('dshToolInspect.unavailable')}</p>
          : capture.kind === 'file_result' ? <pre data-protected-tool-readonly>{JSON.stringify(capture.fileResult, null, 2)}</pre> : <pre>{capture.body}</pre>}
      <div className="text-xs text-ds-faint">
        {capture.truncated && <span>{t('protectedToolTruncated')} · </span>}
        {capture.kind === 'shell' && <span>{t('protectedToolDuration', { duration: capture.durationMs })}</span>}
      </div>
    </> : null}
    {display?.inspect?.outputError ? <ToolRawIO outputError={display.inspect.outputError} /> : null}
    {renderTrigger && inspect ? <button type="button" className={rowCss.inspectButton} onClick={inspect.run}><DshInspectOutlineRegular />{inspect.label}</button> : null}
  </div> : null
  return (
    <div ref={rootRef} data-protected-tool-result>
      {inspectorOnly ? null : renderView ? renderView({ ...controls, display, body, inspect,
        ...(display?.capture.kind === 'present' && selector.success && viewId.current ? { presentReference: {
          ...selector.data, viewId: viewId.current, snapshotDigest: display.snapshotDigest
        } } : {}), onOpenWebLink: role => { void openWebLink(role) } })
        : renderTrigger ? renderTrigger(controls) : <button type="button" aria-expanded={open} disabled={!open && (pending || sharedActiveCaseContextTransitionPending())}
        className="rounded px-2 py-1 text-xs text-ds-muted hover:bg-ds-hover"
        onClick={onToggle}>
        {open ? t('processCollapseDetail') : t('protectedToolOpen')}
      </button>}
      {pending && !renderTrigger && !renderView && <span role="status" className="px-2 text-xs text-ds-muted">{t('protectedToolLoading')}</span>}
      {message && !hideUnavailable && <p role="status" className="text-xs text-ds-muted">{message}
        {!display && <button type="button" disabled={pending} onClick={() => void load()} className="ml-2 underline">{t('protectedToolRetry')}</button>}
      </p>}
      {!inspectorOnly && !renderView ? body : null}
      {inspectorOnly && (!display || !retainedSummaryCapture(display)) ? <ToolResultInspector publicRecord={block} presentation={inspectorPresentation}
        callId={selector.data.callId} state={toolRowState(block)} onClose={() => { clear(); onInspectorClose?.() }} /> : null}
      {inspectorSelected && display && retainedSummaryCapture(display) ? <ToolResultInspector key={display.snapshotDigest} display={display} presentation={inspectorPresentation}
        recordIdentity={conversationCallRecord(selector.data.threadId, block) ?? undefined}
        callId={selector.data.callId} pending={pending} copied={copied} state={captureState} onCopy={section => void effect('copy', section)}
        onClose={() => { closeInspector(); onInspectorClose?.(); if (inspectReturnFocus.current?.isConnected) inspectReturnFocus.current.focus({ preventScroll: true }) }} /> : null}
    </div>
  )
}
