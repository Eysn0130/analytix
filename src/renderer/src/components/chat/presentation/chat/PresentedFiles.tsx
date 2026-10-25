/* Adapted from pinned DSH PresentedFileCard/Deliverables/OpenTargetButton, 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
import { createContext, useCallback, useContext, useId, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import type { GeneratedArtifactMetadata } from '../../../../../../../packages/runtime/src/contracts/generated-artifact'
import { toolResultLocalPresentApplicationsResultSchemaV1, toolResultLocalPresentAssociationResultSchemaV1,
  toolResultLocalEffectResultSchemaV1, type ToolResultLocalDisplayV1, type ToolResultLocalPresentApplicationV1,
  toolResultLocalPresentHistoryViewResultSchemaV1, type ToolResultLocalPresentHistoryV1,
  type ToolResultLocalPresentFileRequestV1 } from '../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ChatBlock } from '../../../../agent/types'
import { openGeneratedArtifact } from '../../../../office/open-generated-artifact'
import { useChatStore } from '../../../../store/chat-store'
import { sharedActiveCaseContextTransitionPending, subscribeBeforeSharedActiveCaseContextChange } from '../../../../data-analysis/services/analysis/stats-case-overview-resource'
import { previewWorkspaceFile } from '../../../../lib/workspace-file-preview'
import { ActionMenuItem, ActionMenuSeparator } from '../../../common/ActionMenu'
import { GeneratedOfficeArtifact, GeneratedOfficeDeliverables } from '../../GeneratedOfficeArtifact'
import { DshPresentedFileIcon, DshRevealFolderIcon } from '../DeliverableIcons'
import { DshChevronDownOutlineRegular, DshChevronUpOutlineRegular, DshInspectOutlineRegular } from '../DshToolIcons'
import { ProtectedToolResult } from '../ProtectedToolResult'
import { useDismissOnOutsidePointer } from '../useDismissOnOutsidePointer'
import { latestPresentedFiles, presentedBasename, presentedDescription, type PresentCall } from './presented-files'
import css from '../../GeneratedOfficeArtifact.module.css'

/** Bounded mounted history lease, retired by the same authority lifecycle. */
export function usePresentHistory(threadId: string | null, turnId: string | null, blocks: readonly ChatBlock[],
  processing: boolean, status: string | undefined, runtimeAvailable?: boolean): ToolResultLocalPresentHistoryV1 | null {
  const api = typeof window === 'undefined' ? undefined : window.analytix?.runtime
  const revision = JSON.stringify([threadId, turnId, processing, status, runtimeAvailable,
    blocks.map(block => [block.id, block.kind, block.kind === 'tool' ? block.status : block.kind === 'assistant' ? Boolean(block.text.trim()) : null])])
  const [value, setValue] = useState<{ key: string; history: ToolResultLocalPresentHistoryV1 } | null>(null)
  const [wake, setWake] = useState(0)
  const viewId = useRef<string | null>(null), generation = useRef(0)
  useLayoutEffect(() => {
    if (!api || typeof api.openToolResultLocalPresentHistory !== 'function') { setValue(null); return }
    let active = true
    const clear = () => {
      generation.current++; setValue(null)
      const id = viewId.current; viewId.current = null
      if (id) void api.closeToolResultLocalDisplay(id).catch(() => undefined)
    }
    clear()
    const off = api.onToolResultLocalDisplayInvalidated?.(id => { if (id === null || id === viewId.current) clear() })
    const runtimeOff = api.onRuntimeStatus?.(event => { if (event.state !== 'running') clear(); else setWake(value => value + 1) })
    const caseOff = subscribeBeforeSharedActiveCaseContextChange(() => {
      clear()
      queueMicrotask(() => { if (active && !document.hidden && !sharedActiveCaseContextTransitionPending()) setWake(value => value + 1) })
    })
    const visible = () => { if (document.hidden) clear(); else setWake(value => value + 1) }
    document.addEventListener('visibilitychange', visible)
    if (typeof api.openToolResultLocalPresentHistory === 'function' && threadId && turnId && runtimeAvailable !== false && !document.hidden && !sharedActiveCaseContextTransitionPending()) {
      const id = crypto.randomUUID(), epoch = generation.current
      viewId.current = id
      void api.openToolResultLocalPresentHistory({ kind: 'present_history', threadId, turnId, viewId: id }).then(result => {
        const parsed = toolResultLocalPresentHistoryViewResultSchemaV1.safeParse(result)
        if (active && epoch === generation.current && viewId.current === id && !document.hidden &&
          !sharedActiveCaseContextTransitionPending() && parsed.success && parsed.data.ok &&
          parsed.data.history.threadId === threadId && parsed.data.history.turnId === turnId) setValue({ key: revision, history: parsed.data.history })
      }).catch(() => undefined)
    }
    return () => { active = false; off?.(); runtimeOff?.(); caseOff(); document.removeEventListener('visibilitychange', visible); clear() }
  }, [api, revision, wake, threadId, turnId, runtimeAvailable])
  return typeof api?.openToolResultLocalPresentHistory === 'function' && value?.key === revision && !sharedActiveCaseContextTransitionPending() ? value.history : null
}

/** Same actual closing renderer in the earlier process and final answer seats. */
export const PresentClosingContext = createContext<((closing: ChatBlock) => ReactNode) | null>(null)
export function PresentClosingTail({ closing }: { closing: ChatBlock }) {
  const render = useContext(PresentClosingContext)
  return <>{render?.(closing)}</>
}
type Contribution = { key: string; block: PresentCall['block']; display: ToolResultLocalDisplayV1;
  reference: Omit<ToolResultLocalPresentFileRequestV1, 'index'>; inspect?: { label: string; run: () => void } }
type FileEntry = { key: string; path: string; description?: string; reference: ToolResultLocalPresentFileRequestV1;
  fileIdentity?: string; artifactIds?: string[]; inspect?: Contribution['inspect'] }

function CurrentContribution({ call, display, reference, inspect, register }: {
  call: PresentCall; display: ToolResultLocalDisplayV1 | null; reference?: Contribution['reference']; inspect?: Contribution['inspect']; register: (key: string, value: Contribution | null) => void
}) {
  const inspectRef = useRef(inspect); inspectRef.current = inspect
  const inspectCurrent = useCallback(() => inspectRef.current?.run(), [])
  const referenceKey = JSON.stringify(reference), inspectLabel = inspect?.label
  const referenceRef = useRef(reference); referenceRef.current = reference
  useLayoutEffect(() => {
    const currentReference = referenceRef.current
    if (display?.capture.kind === 'present' && display.toolName === 'present' && currentReference) register(call.key, { key: call.key, block: call.block, display, reference: currentReference, ...(inspectLabel !== undefined ? { inspect: { label: inspectLabel, run: inspectCurrent } } : {}) })
    else register(call.key, null)
    return () => register(call.key, null)
  }, [call.key, call.block, display, referenceKey, inspectLabel, inspectCurrent, register])
  return null
}

/** One mounted closing owns both old Office receipts and current private Present leases. */
export function PresentedFiles({ threadId, calls, artifacts }: { threadId: string | null; calls: PresentCall[]; artifacts: GeneratedArtifactMetadata[] }) {
  const { t } = useTranslation('common'), owner = useId(), api = window.analytix?.runtime
  const [contributions, setContributions] = useState<Record<string, Contribution>>({})
  const [associations, setAssociations] = useState<{ key: string; entries: Record<string, { fileIdentity: string; artifactIds: string[] }> } | null>(null)
  const [expanded, setExpanded] = useState(false), [retired, setRetired] = useState(false)
  const retiredRef = useRef(false)
  const callKey = JSON.stringify([threadId, calls.map(call => call.key)])
  const historyOwner = calls[0]?.history
  useLayoutEffect(() => { retiredRef.current = false; setRetired(false) }, [callKey, historyOwner])
  useLayoutEffect(() => {
    const visible = () => { if (!document.hidden) { retiredRef.current = false; setRetired(false) } }
    document.addEventListener('visibilitychange', visible)
    return () => document.removeEventListener('visibilitychange', visible)
  }, [])
  const root = useRef<HTMLDivElement>(null)
  const contributionsRef = useRef(contributions), retiredViews = useRef(new Set<string>())
  contributionsRef.current = contributions
  useLayoutEffect(() => api?.onToolResultLocalDisplayInvalidated?.(viewId => {
    const values = Object.values(contributionsRef.current)
    if (viewId !== null && !values.some(value => value.reference.viewId === viewId)) return
    for (const value of values) retiredViews.current.add(value.reference.viewId)
    retiredRef.current = true; setRetired(true)
    contributionsRef.current = {}; setContributions({}); setAssociations(null)
  }), [api])
  const register = useCallback((key: string, value: Contribution | null) => setContributions(current => {
    if (value && (retiredRef.current || retiredViews.current.has(value.reference.viewId))) return current
    if (!value && !current[key]) return current
    const next = { ...current }
    if (value) next[key] = value; else delete next[key]
    return next
  }), [])
  const declarations = useMemo(() => retired ? [] : calls.flatMap(call => {
    const contribution = contributions[call.key]
    if (contribution?.display.capture.kind !== 'present') return []
    return contribution.display.capture.present.files.map(file => ({ ...file, key: call.key + ':' + file.index,
      reference: { ...contribution.reference, index: file.index }, inspect: contribution.inspect }))
  }), [calls, contributions, retired])
  const associationKey = JSON.stringify([declarations.map(file => file.reference), artifacts.map(artifact => artifact.artifactId)])
  const associationInputs = useRef({ declarations, artifacts }); associationInputs.current = { declarations, artifacts }
  useLayoutEffect(() => {
    let current = true
    setAssociations(null)
    const { declarations, artifacts } = associationInputs.current
    const entries: Record<string, { fileIdentity: string; artifactIds: string[] }> = {}
    void Promise.all(declarations.map(async file => {
      try {
        const parsed = toolResultLocalPresentAssociationResultSchemaV1.safeParse(await api?.associateToolResultLocalPresentFile({
          ...file.reference, artifactIds: artifacts.slice(0, 32).map(artifact => artifact.artifactId)
        }))
        if (current && parsed.success && parsed.data.ok) entries[file.key] = { fileIdentity: parsed.data.fileIdentity, artifactIds: parsed.data.artifactIds }
      } catch { /* Unknown matches keep the Office-only history fallback. */ }
    })).then(() => { if (current) setAssociations({ key: associationKey, entries }) })
    return () => { current = false }
  }, [associationKey, api])
  useLayoutEffect(() => {
    const protectedNodes = () => [...document.querySelectorAll<HTMLElement>('[data-present-owner]')].filter(node => node.dataset.presentOwner === owner)
    const intersects = (event: Event) => {
      const selection = document.getSelection()
      for (const element of event.type === 'dragstart' ? [event.target] : [event.target, document.activeElement]) {
        if (element instanceof Element && element.closest('input,textarea,[contenteditable]:not([contenteditable="false"]),[role="textbox"]') && !element.closest('[data-present-owner]')) return false
      }
      return protectedNodes().some(node => node.contains(event.target as Node) || Array.from({ length: selection && !selection.isCollapsed ? selection.rangeCount : 0 }, (_, index) => {
        try { return selection!.getRangeAt(index).intersectsNode(node) } catch { return false }
      }).some(Boolean))
    }
    const blockTransfer = (event: Event) => { if (intersects(event)) event.preventDefault() }
    for (const name of ['copy', 'cut', 'dragstart']) document.addEventListener(name, blockTransfer, true)
    return () => {
      const selection = document.getSelection()
      if (selection && protectedNodes().some(node => Array.from({ length: selection.rangeCount }, (_, index) => {
        try { return selection.getRangeAt(index).intersectsNode(node) } catch { return false }
      }).some(Boolean))) selection.removeAllRanges()
      for (const name of ['copy', 'cut', 'dragstart']) document.removeEventListener(name, blockTransfer, true)
    }
  }, [owner])
  const currentAssociations = associations?.key === associationKey ? associations.entries : {}
  const files: FileEntry[] = latestPresentedFiles(declarations.filter(file => currentAssociations[file.key]).map(file => ({ ...file, ...currentAssociations[file.key] })))
  const matched = new Set(Object.values(currentAssociations).flatMap(value => value.artifactIds))
  const office = artifacts.filter(artifact => !matched.has(artifact.artifactId))
  const cards = [...office.map(artifact => ({ key: artifact.artifactId, node: <GeneratedOfficeArtifact artifact={artifact} /> })),
    ...files.map(file => {
      // Every association was freshly verified by Host. A later declaration of
      // the same native object must retain an earlier matching Office receipt.
      const related = new Set(Object.values(currentAssociations).filter(value => value.fileIdentity === file.fileIdentity).flatMap(value => value.artifactIds))
      const artifact = artifacts.find(candidate => related.has(candidate.artifactId))
      return { key: file.key, node: <PresentedFileCard file={file} owner={owner} artifactId={artifact?.artifactId} /> }
    })]
  const shown = expanded ? cards : cards.slice(0, 4)
  if (!calls.length) return <GeneratedOfficeDeliverables artifacts={artifacts} />
  return <>
    {!retired && calls.map(call => <ProtectedToolResult key={call.key} block={call.block} threadId={threadId} preload hideUnavailable
      renderView={({ display, presentReference, inspect }) => <CurrentContribution call={call} display={display} reference={presentReference} inspect={inspect} register={register} />} />)}
    {cards.length ? <div ref={root} className={css.root} data-presented-deliverables>
      <div className={css.presented} data-single={cards.length === 1 || undefined}>{shown.map(card => <div className={css.cardSlot} key={card.key}>{card.node}</div>)}</div>
      {cards.length > 4 ? <button type="button" className={css.toggle} aria-expanded={expanded}
        aria-label={t(expanded ? 'dshPresent.collapseAria' : 'dshPresent.expandAria', { count: cards.length })} onClick={() => setExpanded(value => !value)}>
        {t(expanded ? 'dshPresent.collapse' : 'dshPresent.all', { count: cards.length })}
        {expanded ? <DshChevronUpOutlineRegular /> : <DshChevronDownOutlineRegular />}
      </button> : null}
    </div> : null}
  </>
}

function PresentApplicationIcon({ application, path, size = 16 }: { application: ToolResultLocalPresentApplicationV1 | undefined; path: string; size?: number }) {
  const [failed, setFailed] = useState(false)
  return application?.iconDataUrl && !failed
    ? <img src={application.iconDataUrl} width={size} height={size} alt="" draggable={false} onError={() => setFailed(true)} />
    : <DshPresentedFileIcon path={path} size={size} />
}

function presentMenuPosition(trigger: HTMLButtonElement, rows: number) {
  const rect = trigger.getBoundingClientRect(), scale = parseFloat(getComputedStyle(document.body).zoom) || 1
  const height = Math.min(320, rows * 36 + 16)
  return { left: Math.max(8, Math.min(rect.right - 240 * scale, window.innerWidth - 248 * scale)) / scale,
    top: Math.max(8, Math.min(rect.bottom + 4 * scale, window.innerHeight - (height + 8) * scale)) / scale }
}

export function PresentedFileCard({ file, owner, artifactId }: { file: FileEntry; owner: string; artifactId?: string }) {
  const workspace = useChatStore(state => state.workspaceRoot)
  const { t } = useTranslation('common'), api = window.analytix.runtime
  const [catalog, setCatalog] = useState<{ applications: ToolResultLocalPresentApplicationV1[]; loading: boolean; failed: boolean }>({ applications: [], loading: true, failed: false })
  const [phase, setPhase] = useState<'opening' | 'opened' | 'error' | 'revealing' | 'revealed' | 'revealError'>()
  const [menu, setMenu] = useState<{ left: number; top: number } | null>(null)
  const inFlight = useRef(false), generation = useRef(0), query = useRef(0), timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const root = useRef<HTMLDivElement>(null), portal = useRef<HTMLDivElement>(null), trigger = useRef<HTMLButtonElement>(null)
  const requestKey = JSON.stringify(file.reference), name = presentedBasename(file.path)
  const fileReference = useRef(file.reference); fileReference.current = file.reference
  const closeMenu = useCallback(() => { setMenu(null) }, [])
  useDismissOnOutsidePointer(root, Boolean(menu), closeMenu, portal)
  const refresh = useCallback(() => {
    const epoch = generation.current, issued = ++query.current, reference = fileReference.current
    setCatalog(current => ({ ...current, loading: true, failed: false }))
    const settle = (value: unknown) => {
      if (generation.current !== epoch || query.current !== issued) return
      const result = toolResultLocalPresentApplicationsResultSchemaV1.safeParse(value)
      setCatalog(result.success && result.data.ok ? { applications: result.data.applications, loading: false, failed: false }
        : { applications: [], loading: false, failed: true })
    }
    void Promise.resolve().then(() => api.listToolResultLocalPresentApplications(reference)).then(settle).catch(() => settle(null))
  }, [api])
  useLayoutEffect(() => {
    const epoch = ++generation.current
    inFlight.current = false; setCatalog({ applications: [], loading: true, failed: false }); setPhase(undefined); setMenu(null)
    refresh()
    return () => { generation.current = epoch + 1; if (timer.current !== null) clearTimeout(timer.current) }
  }, [requestKey, refresh])
  const { applications, loading, failed } = catalog
  const preferred = applications.find(application => application.isDefault) ?? applications[0]
  const revealDefault = preferred === undefined && !loading
  const busy = phase === 'opening' || phase === 'revealing', disabled = busy || loading
  const hasMenu = loading || failed || applications.length > 0
  const menuRequested = Boolean(menu), menuVisible = menuRequested && !disabled && hasMenu
  useLayoutEffect(() => {
    if (!menuRequested) return
    const close = () => closeMenu()
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.preventDefault(); closeMenu(); trigger.current?.focus() }
      else if (event.key === 'Tab') closeMenu()
    }
    // Refresh temporarily hides the portal; its requested menu still has to
    // honor cancellation and window movement before the query settles.
    document.addEventListener('keydown', escape)
    window.addEventListener('resize', close); window.addEventListener('scroll', close, true)
    return () => { document.removeEventListener('keydown', escape); window.removeEventListener('resize', close); window.removeEventListener('scroll', close, true) }
  }, [menuRequested, closeMenu])
  useLayoutEffect(() => {
    if (!menuVisible || !trigger.current) return
    const position = presentMenuPosition(trigger.current, applications.length + (failed ? 1 : 0) + 1)
    setMenu(current => current && (current.left !== position.left || current.top !== position.top) ? position : current)
    portal.current?.querySelector<HTMLButtonElement>('button[role^="menuitem"]:not(:disabled)')?.focus()
  }, [menuVisible, applications.length, failed])
  const open = async (applicationId?: string, action?: 'reveal') => {
    if (inFlight.current || loading) return
    inFlight.current = true; setMenu(null); setPhase(action === 'reveal' ? 'revealing' : 'opening')
    if (timer.current !== null) clearTimeout(timer.current)
    const epoch = generation.current
    try {
      const result = toolResultLocalEffectResultSchemaV1.safeParse(await api.openToolResultLocalPresentFile({ ...file.reference,
        ...(action === 'reveal' ? { action: 'reveal' as const } : applicationId !== undefined ? { applicationId } : {}) }))
      if (generation.current !== epoch) return
      const succeeded = result.success && result.data.ok
      setPhase(succeeded ? action === 'reveal' ? 'revealed' : 'opened' : action === 'reveal' ? 'revealError' : 'error')
      if (succeeded) timer.current = setTimeout(() => { if (generation.current === epoch) setPhase(undefined) }, 5200)
    } catch { if (generation.current === epoch) setPhase(action === 'reveal' ? 'revealError' : 'error') }
    finally { if (generation.current === epoch) inFlight.current = false }
  }
  const preview = async () => {
    if (!artifactId) { previewWorkspaceFile({ path: file.path, preview: 'source', protectedPresent: file.reference }); return }
    if (inFlight.current) return
    inFlight.current = true; setPhase('opening')
    if (timer.current !== null) clearTimeout(timer.current)
    const epoch = generation.current
    try {
      const opened = await openGeneratedArtifact(artifactId, { threadId: file.reference.threadId, workspace })
      if (generation.current !== epoch) return
      setPhase(opened ? 'opened' : 'error')
      if (opened) timer.current = setTimeout(() => { if (generation.current === epoch) setPhase(undefined) }, 5200)
    } catch { if (generation.current === epoch) setPhase('error') }
    finally { if (generation.current === epoch) inFlight.current = false }
  }
  const succeeded = phase === 'opened' || phase === 'revealed'
  const description = phase ? t(phase === 'error' ? 'dshPresent.unavailable' : 'dshPresent.' + phase) : presentedDescription(file.description, file.path)
  const primaryLabel = preferred ? t('dshPresent.openInApplication', { app: preferred.label }) : t('dshPresent.reveal')
  return <div ref={root} className={css.file} data-presented-file data-present-owner={owner} data-protected-tool-summary data-protected-tool-readonly>
    <button type="button" className={css.cardPreview} title={file.path} aria-label={t('dshPresent.previewCard', { name: file.path })}
      disabled={phase === 'opening' || phase === 'revealing'} aria-busy={phase === 'opening'} onClick={() => void preview()} />
    <span className={css.fileIcon}><DshPresentedFileIcon path={file.path} size={20} /></span>
    <div className={css.fileBody}><div className={css.details}>
      <span className={css.fileName}>{name}</span>
      <span className={css.description} role={phase ? 'status' : undefined} data-error={phase === 'error' || phase === 'revealError' || undefined}>
        <span className={css.secondaryText} data-success={succeeded || undefined}
          style={succeeded ? { animationDelay: '5000ms', animationDuration: '200ms' } : undefined}>{description}</span>
        <span className={css.previewHint}>{t('dshPresent.preview')}</span>
      </span>
    </div><div className={css.actions}>
      <button type="button" className={css.openAction} disabled={disabled} aria-busy={busy} aria-label={primaryLabel} title={primaryLabel}
        onClick={() => void open(preferred && !preferred.isDefault ? preferred.applicationId : undefined, revealDefault ? 'reveal' : undefined)}>
        {revealDefault ? <DshRevealFolderIcon size={13} /> : <PresentApplicationIcon key={preferred?.iconDataUrl} application={preferred} path={file.path} size={13} />}
        {t(revealDefault ? 'dshPresent.reveal' : 'dshPresent.open')}
      </button>
      {hasMenu ? <button ref={trigger} type="button" className={css.openAction} disabled={disabled}
        aria-label={t('dshPresent.openWith')} aria-haspopup="menu" aria-expanded={menuVisible} onClick={() => {
          if (menu) closeMenu()
          else {
            setMenu(presentMenuPosition(trigger.current!, applications.length + (failed ? 1 : 0) + 1))
            refresh()
          }
        }}><DshChevronDownOutlineRegular /></button> : null}
      {file.inspect ? <button type="button" className={css.openAction} aria-label={file.inspect.label} title={file.inspect.label}
        onClick={file.inspect.run}><DshInspectOutlineRegular /></button> : null}
    </div></div>
    {menuVisible && menu ? createPortal(<div ref={portal} role="menu" aria-label={t('dshPresent.openWith')}
      data-present-owner={owner} data-protected-tool-summary data-protected-tool-readonly className={'ds-session-actions-menu ' + css.applicationMenu}
      style={{ left: menu.left, top: menu.top }} onKeyDown={event => {
        const items = [...portal.current!.querySelectorAll<HTMLButtonElement>('button[role^="menuitem"]:not(:disabled)')]
        const index = items.indexOf(document.activeElement as HTMLButtonElement)
        if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
          event.preventDefault()
          items[event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length]?.focus()
        } else if (event.key === 'Escape' || event.key === 'Tab') { if (event.key === 'Escape') event.preventDefault(); closeMenu(); trigger.current?.focus() }
      }}>{applications.map(application => <ActionMenuItem key={application.applicationId} role="menuitemradio" checked={application === preferred}
        icon={<PresentApplicationIcon key={application.iconDataUrl} application={application} path={file.path} />}
        label={application === preferred ? t('dshPresent.applicationDefault', { app: application.label }) : application.label}
        onClick={() => void open(application.applicationId)} />)}
      {failed ? <ActionMenuItem icon={<span aria-hidden="true" />} label={t('dshPresent.appsError')} disabled /> : null}
      <ActionMenuSeparator />
      <ActionMenuItem icon={<DshRevealFolderIcon />} label={revealDefault ? t('dshPresent.applicationDefault', { app: t('dshPresent.reveal') }) : t('dshPresent.reveal')}
        onClick={() => void open(undefined, 'reveal')} />
    </div>, document.body) : null}
  </div>
}
