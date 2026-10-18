/* Presentation adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useCallback, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { flushSync } from 'react-dom'
import { useTranslation } from 'react-i18next'
import {
  processPaneAuthorizeRequestSchemaV1, processPaneAuthorizeResultSchemaV1,
  processPaneReadResultSchemaV1, processPaneActionResultSchemaV1,
  hasProcessDisplayPayloadV2,
  type ProcessDisplayBlockV1, type ProcessDisplayScopeV1, type ProcessDisplayUnitV2
} from '../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import type { ChatBlock } from '../../../../agent/types'
import { sharedActiveCaseContextTransitionPending, subscribeBeforeSharedActiveCaseContextChange } from '../../../../data-analysis/services/analysis/stats-case-overview-resource'
import { DshApiOutlineRegular, DshBrowseOutlineRegular, DshEditOutlineRegular, DshSearchOutlineRegular, DshSparkleRegular } from '../DshToolIcons'
import { MarkdownText, type MarkdownLabels } from '../markdown/MarkdownText'
import { ProcessToolCardBody } from './toolviews/ToolCardBody'
import { ToolRawIO } from './toolviews/ToolRawIO'
import { toolPreparationRowModel } from './models/tool-call-model'
import { DshDeliverDocRegular } from '../DeliverableIcons'
import { ReasoningRow } from './ReasoningRow'
import { ToolRow } from './ToolRow'
import { ToolCallTree, processDraftSeats, type ProcessLocalRows, type ProcessVisibilityReport } from './ToolCallTree'
import { TurnProcessView } from './TurnProcessView'
import { presentationPolicyFor, type ChatPresentationPolicy } from './chat-view-model'
import { useDisclosure } from './use-disclosure'
import { useSearchableHidden } from './searchable-hidden'
import styles from './ChatView.module.css'
import toolRowStyles from './ToolRow.module.css'

export type ProcessPaneRoute = {
  surface: 'main' | 'side' | 'inspector'; ownerKey: string; selectionKey: string; visible: boolean
}
type Lease = {
  viewerId: string; generation: number; key: string; revision: string; hasSnapshot: boolean
  scope: ProcessDisplayScopeV1; root: HTMLDivElement
}
type ProviderSource = Extract<ProcessDisplayBlockV1['source'], { kind: 'native' | 'provider_reasoning' | 'tool_preparation' | 'assistant_draft' }>
const providerSource = (source: ProcessDisplayBlockV1['source']): source is ProviderSource => ['native', 'provider_reasoning', 'tool_preparation', 'assistant_draft'].includes(source.kind)
const privateContent = (block: ProcessDisplayBlockV1) => block.input !== undefined || block.title !== undefined || block.preview !== undefined || block.path !== undefined || block.body !== undefined || block.inputUtf16CodeUnits !== undefined || block.draftSegments !== undefined || block.draftBytes !== undefined
type Snapshot = { key: string; generation: number; units: ProcessDisplayUnitV2[]; blocks: ProcessDisplayBlockV1[] }
type Structure = Omit<Snapshot, 'blocks'> & { identities: ProcessLocalRows['identities'] }
const EMPTY_DISCOVERY_TIMEOUT_MS = 10_000
const MAX_EMPTY_DISCOVERY_READS = 8
// Stable, bounded control identities contain no title, preview, path or body.
// A transport replay cannot reorder slots. Only units from the same producer
// compare their actual logical provider sequence.
function stableUnits(units: ProcessDisplayUnitV2[]): ProcessDisplayUnitV2[] {
  const producers = new Map<string, ProcessDisplayUnitV2[]>()
  for (const unit of units) producers.set(unit.producerId, [...producers.get(unit.producerId) ?? [], unit])
  for (const values of producers.values()) values.sort((a, b) => BigInt(a.providerSequence) < BigInt(b.providerSequence) ? -1 : BigInt(a.providerSequence) > BigInt(b.providerSequence) ? 1 : 0)
  const indexes = new Map<string, number>()
  return units.map(unit => { const index = indexes.get(unit.producerId) ?? 0; indexes.set(unit.producerId, index + 1); return producers.get(unit.producerId)![index] })
}
const withoutBody = (block: ProcessDisplayBlockV1): ProcessDisplayBlockV1 => {
  const { input: _input, body: _body, draftSegments: _segments, draftBytes: _bytes, ...metadata } = block
  return metadata
}
function visibleRoot(root: HTMLDivElement | null, paneId: string): root is HTMLDivElement {
  if (!root?.isConnected || root.dataset.processPaneId !== paneId || document.hidden) return false
  const matches = [...document.querySelectorAll<HTMLElement>('[data-process-pane-id]')].filter(node => node.dataset.processPaneId === paneId)
  if (matches.length !== 1 || matches[0] !== root) return false
  for (let node: HTMLElement | null = root; node; node = node.parentElement) {
    const style = getComputedStyle(node)
    if (node.hidden || node.inert || node.hasAttribute('inert') || node.getAttribute('aria-hidden') === 'true' || style.display === 'none' || style.visibility === 'hidden' || style.visibility === 'collapse') return false
  }
  return true
}
function sameScope(a: ProcessDisplayScopeV1, b: ProcessDisplayScopeV1): boolean {
  return a.threadId === b.threadId && a.turnId === b.turnId && a.contextDigest === b.contextDigest
    && a.contextEpoch === b.contextEpoch && a.processIncarnation === b.processIncarnation
}
// This mounted leaf is the only renderer owner of private snapshots/bodies.
// No document-global snapshot, public ChatBlock, finalization or history store.
export function ProtectedProcessPane({ route, threadId, turnId, processOpen, processBlocks = [], processReplyBoundaryAfterIds, processing = false,
  policy = presentationPolicyFor('detailed'), runtimeAvailable = false, offerProcessEntry = false, requestProcessDisplay = false, onRequestProcessDisplay = () => undefined, status, collapsible = true, onToggle = () => undefined, completedDurationMs, answer, terminalAuthorized = false, onInspect }: {
  route?: ProcessPaneRoute; threadId: string | null; turnId: string | null; processOpen: boolean
  processBlocks?: ChatBlock[]; processReplyBoundaryAfterIds?: readonly string[]; processing?: boolean; policy?: ChatPresentationPolicy; runtimeAvailable?: boolean
  offerProcessEntry?: boolean; requestProcessDisplay?: boolean; onRequestProcessDisplay?: () => void
  status?: 'completed' | 'failed' | 'aborted'; collapsible?: boolean; onToggle?: () => void; completedDurationMs?: number
  answer?: ReactNode; terminalAuthorized?: boolean; onInspect?: (turnId: string, callId: string) => void
}) {
  const { t } = useTranslation('common')
  const api = typeof window === 'undefined' ? undefined : window.analytix?.runtime
  const [paneId] = useState(() => crypto.randomUUID())
  const [acquisition, setAcquisition] = useState(0)
  const runtimeReady = useRef(runtimeAvailable), previousAvailability = useRef(runtimeAvailable)
  const processingRef = useRef(processing)
  const emptyDiscovery = useRef({ deadline: 0, reads: 0 })
  const explicitOpen = useRef(false), settlementHydration = useRef(0)
  const visibilityKnownIds = useRef(new Set<string>())
  const key = JSON.stringify([route?.ownerKey, route?.selectionKey, route?.surface, threadId, turnId])
  // This presentation branch never grants eligibility or opens a private lease.
  const publicDisclosure = useDisclosure(key + ':' + (processOpen ? 'open' : 'folded'))
  const publicOpen = processOpen || publicDisclosure.open
  const revealPublic = useCallback(() => {
    if (!processOpen) publicDisclosure.setOpen(true)
  }, [processOpen, publicDisclosure.setOpen])
  const hasPersistentError = processBlocks.some(block => block.kind === 'system' && block.severity === 'error')
  const publicBody = useSearchableHidden(!publicOpen && !hasPersistentError, revealPublic)
  const browserPreview = typeof document === 'undefined' || document.documentElement.dataset.bridge === 'browser-preview'
  const eligible = Boolean(api && route?.visible && threadId && turnId && !browserPreview)
  const root = useRef<HTMLDivElement>(null), currentKey = useRef(key), eligibleRef = useRef(eligible), openRef = useRef(processOpen)
  const generation = useRef(0), ticket = useRef(0), lease = useRef<Lease | null>(null)
  const visibility = useRef<ProcessVisibilityReport | null>(null)
  const visibleIds = useRef<Set<string> | null>(null)
  const recoveryTimer = useRef<ReturnType<typeof setTimeout> | null>(null), recoveryAttempts = useRef(0)
  const expanded = useRef(new Set<string>()), lane = useRef<Promise<void>>(Promise.resolve())
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const firstProjection = useRef<Set<string> | null>(null)
  const sourcePins = useRef(new Map<string, ProcessDisplayBlockV1['source']>())
  const providerAttempts = useRef(new Map<string, number>())
  const retiredProviderBlocks = useRef(new Set<string>()), settledReasoningBlocks = useRef(new Set<string>())
  const draftAttempts = useRef(new Map<string, { attempt: number; blockId: string }>())
  const retiredDrafts = useRef(new Set<string>()), sawDraft = useRef(false), draftTerminal = useRef(false)
  const draftKey = useRef(key)
  if (draftKey.current !== key) { draftKey.current = key; sawDraft.current = false; draftTerminal.current = false }
  if (terminalAuthorized) draftTerminal.current = true
  const unitPins = useRef(new Map<string, ProcessDisplayUnitV2>())
  const callOwners = useRef(new Map<string, string>())
  const [structure, setStructure] = useState<Structure | null>(null)
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null)
  const [presence, setPresence] = useState<boolean | null>(null), [pending, setPending] = useState(false)
  const [message, setMessage] = useState(''), [expandedIds, setExpandedIds] = useState(new Set<string>())
  const labels = useMemo<MarkdownLabels>(() => ({
    code: { copyLabel: t('codeBlockCopy'), copiedLabel: t('copySuccess') }, footnotes: t('markdownFootnotes'),
    table: { title: t('markdownTableTitle'), copy: t('markdownTableCopy'), copied: t('copySuccess'), copyFailed: t('copyFailed'),
      download: t('markdownTableDownload'), downloadFailed: t('markdownTableDownloadFailed'), fullscreen: t('markdownTableFullscreen'),
      close: t('markdownTableClose'), csv: t('markdownTableCsv'), tsv: t('markdownTableTsv'), markdown: t('markdownTableMarkdown'), exportHint: t('markdownTableExportHint') }
  }), [t])
  const stopTimer = useCallback(() => { if (timer.current !== null) clearTimeout(timer.current); timer.current = null }, [])
  const stopRecovery = useCallback(() => { if (recoveryTimer.current !== null) clearTimeout(recoveryTimer.current); recoveryTimer.current = null }, [])
  // The real public groups report visibility before discovery. Empty presence
  // is not permission to read through a known folded group.
  const draftsOnly = () => visibility.current?.kind === 'all_groups_folded' || visibility.current?.draftsOnly === true ||
    !draftTerminal.current && processingRef.current && sourcePins.current.size === 0 &&
    (Date.now() >= emptyDiscovery.current.deadline || emptyDiscovery.current.reads >= MAX_EMPTY_DISCOVERY_READS)
  const readable = () => openRef.current && visibility.current !== null
    && (visibility.current.kind === 'visible' || visibility.current.kind === 'empty' || draftsOnly())
    && (visibleIds.current !== null && visibleIds.current.size > 0 || sourcePins.current.size === 0 || draftsOnly())
  const pollable = () => processingRef.current && readable() && (draftsOnly() || sourcePins.current.size > 0
    || Date.now() < emptyDiscovery.current.deadline && emptyDiscovery.current.reads < MAX_EMPTY_DISCOVERY_READS)
  const privateRoots = () => [...(root.current?.querySelectorAll<HTMLElement>('[data-protected-process-block]') ?? [])]
  const clearSelection = useCallback((blockIds?: ReadonlySet<string>) => {
    const selection = document.getSelection()
    const nodes = privateRoots().filter(node => !blockIds || blockIds.has(node.dataset.protectedProcessBlock ?? ''))
    for (let i = 0; selection && i < selection.rangeCount; i++) {
      try { if (nodes.some(node => selection.getRangeAt(i).intersectsNode(node))) { selection.removeAllRanges(); break } } catch { /* Retired DOM. */ }
    }
  }, [])
  const retire = useCallback((synchronous = false) => {
    generation.current++; ticket.current++; lane.current = Promise.resolve(); stopTimer(); stopRecovery(); clearSelection()
    emptyDiscovery.current = { deadline: 0, reads: 0 }; explicitOpen.current = false; settlementHydration.current = 0; visibilityKnownIds.current.clear()
    const old = lease.current; lease.current = null; visibility.current = null; visibleIds.current = null; expanded.current.clear(); sourcePins.current.clear(); firstProjection.current = null; unitPins.current.clear(); callOwners.current.clear(); draftAttempts.current.clear(); retiredDrafts.current.clear(); providerAttempts.current.clear(); retiredProviderBlocks.current.clear(); settledReasoningBlocks.current.clear()
    const clear = () => { setSnapshot(null); setStructure(null); setPresence(null); setPending(false); setExpandedIds(new Set()); setMessage('') }
    if (synchronous) flushSync(clear); else clear()
    if (old) void api?.closeProcessPane({ viewerId: old.viewerId }).catch(() => undefined)
  }, [api, stopTimer, stopRecovery, clearSelection])
  const isCurrent = (owner: Lease, requestTicket: number) => lease.current === owner && owner.generation === generation.current
    && owner.key === currentKey.current && ticket.current === requestTicket && eligibleRef.current && root.current === owner.root
    && visibleRoot(owner.root, paneId) && !sharedActiveCaseContextTransitionPending()
  const enqueue = (work: () => Promise<void>) => { lane.current = lane.current.then(work, work).catch(() => undefined) }
  const fail = () => { retire(true); setMessage(t('dshProcessUnavailable')) }
  const pin = (reply: Snapshot): boolean => {
    const sameAnchor = (old: ProcessDisplayUnitV2['calls'][number], next: ProcessDisplayUnitV2['calls'][number]) => old.callId === next.callId
      && old.toolCallItemId === next.toolCallItemId && (old.toolResultItemId === undefined || old.toolResultItemId === next.toolResultItemId)
      && (old.startedEventSeq === undefined || old.startedEventSeq === next.startedEventSeq)
    const anchors = new Map<string, ProcessDisplayUnitV2['calls'][number]>()
    const mergeAnchor = (next: ProcessDisplayUnitV2['calls'][number]) => {
      const old = anchors.get(next.callId)
      if (old && (old.toolCallItemId !== next.toolCallItemId
        || old.toolResultItemId !== undefined && next.toolResultItemId !== undefined && old.toolResultItemId !== next.toolResultItemId
        || old.startedEventSeq !== undefined && next.startedEventSeq !== undefined && old.startedEventSeq !== next.startedEventSeq)) return false
      anchors.set(next.callId, { ...next,
        ...(old?.toolResultItemId !== undefined ? { toolResultItemId: old.toolResultItemId } : {}),
        ...(old?.startedEventSeq !== undefined ? { startedEventSeq: old.startedEventSeq } : {}) })
      return true
    }
    // Partial preceding references never erase a result/event identity learned
    // from its owning call, including when that unit is absent from this reply.
    for (const previous of unitPins.current.values()) for (const anchor of [...previous.calls, ...previous.precedingCalls]) if (!mergeAnchor(anchor)) return false
    for (const unit of reply.units) {
      const old = unitPins.current.get(unit.unitId)
      if (old && (old.producerId !== unit.producerId || old.providerSequence !== unit.providerSequence
        || old.placement === 'anchored' && unit.placement !== 'anchored'
        || old.calls.some((anchor, index) => !unit.calls[index] || !sameAnchor(anchor, unit.calls[index]))
        || old.precedingCalls.some((anchor, index) => !unit.precedingCalls[index] || !sameAnchor(anchor, unit.precedingCalls[index])))) return false
      for (const anchor of [...unit.calls, ...unit.precedingCalls]) {
        if (!mergeAnchor(anchor)) return false
      }
      for (const call of unit.calls) {
        const owner = callOwners.current.get(call.callId)
        if (owner !== undefined && owner !== unit.unitId) return false
      }
    }
    // Stage attempt maxima from the entire reply before checking any row.
    // Source pins and tombstones contain control coordinates only.
    const nextAttempts = new Map(providerAttempts.current)
    for (const block of reply.blocks) if (providerSource(block.source)) nextAttempts.set(block.source.unitId, Math.max(nextAttempts.get(block.source.unitId) ?? 0, block.source.attempt))
    const logicalOwners = new Map<string, string>(), ordinalOwners = new Map<string, string>()
    // The schema checks each snapshot; pins also reject cross-reply coordinate reuse.
    const pinOrdinal = (id: string, source: ProviderSource) => {
      if (source.kind === 'assistant_draft') return true
      const coordinate = JSON.stringify([source.unitId, source.producerId, source.providerSequence, source.sourceOrdinal])
      const owner = ordinalOwners.get(coordinate)
      if (owner !== undefined && owner !== id) return false
      ordinalOwners.set(coordinate, id); return true
    }
    const logicalSource = (source: ProviderSource) => source.kind === 'native' || source.kind === 'provider_reasoning'
      ? JSON.stringify([source.unitId, 'reasoning', source.sourceOrdinal])
      : source.kind === 'tool_preparation' ? JSON.stringify([source.unitId, source.kind, source.callId, source.attempt])
        : JSON.stringify([source.unitId, source.kind, source.attempt])
    for (const [id, source] of sourcePins.current) if (providerSource(source)) {
      if (!pinOrdinal(id, source)) return false
      logicalOwners.set(logicalSource(source), id)
    }
    for (const block of reply.blocks) {
      const old = sourcePins.current.get(block.blockId), next = block.source
      if (old && (old.kind !== next.kind || old.unitId !== next.unitId)) return false
      if (old && providerSource(old) && providerSource(next) && JSON.stringify(old) !== JSON.stringify(next)) return false
      if (providerSource(next)) {
        if (!pinOrdinal(block.blockId, next)) return false
        const coordinate = logicalSource(next), owner = logicalOwners.get(coordinate)
        if (owner !== undefined && owner !== block.blockId) return false
        logicalOwners.set(coordinate, block.blockId)
        const active = block.state === 'running' || block.state === 'preparing'
        if (next.attempt < (nextAttempts.get(next.unitId) ?? next.attempt) && (active || privateContent(block)) ||
          retiredProviderBlocks.current.has(block.blockId) && (active || privateContent(block)) ||
          settledReasoningBlocks.current.has(block.blockId) && block.state === 'running') return false
      }
      if (old?.kind === 'tool' && next.kind === 'tool' && !sameAnchor(old, next)) return false
      if (next.kind === 'assistant_draft') {
        const previous = draftAttempts.current.get(next.unitId)
        if (previous && (next.attempt === previous.attempt && block.blockId !== previous.blockId ||
          next.attempt < previous.attempt && block.state === 'running') ||
          retiredDrafts.current.has(block.blockId) && block.state === 'running') return false
      }
    }
    if (new Set([...unitPins.current.keys(), ...reply.units.map(unit => unit.unitId)]).size > 128
      || new Set([...sourcePins.current.keys(), ...reply.blocks.map(block => block.blockId)]).size > 128
      || new Set([...callOwners.current.keys(), ...reply.units.flatMap(unit => unit.calls.map(call => call.callId))]).size > 128) return false
    const calls = new Map<string, string>()
    for (const unit of reply.units) for (const call of unit.calls) {
      if (calls.has(call.callId) && calls.get(call.callId) !== unit.unitId) return false
      calls.set(call.callId, unit.unitId)
    }
    // Retire selections only at a real settlement/attempt boundary. Ordinary
    // running revisions preserve the reader's selection and scroll position.
    const selectionRetirements = new Set<string>()
    for (const [id, source] of sourcePins.current) if (providerSource(source) && source.attempt < (nextAttempts.get(source.unitId) ?? source.attempt)) selectionRetirements.add(id)
    for (const block of reply.blocks) if (sourcePins.current.has(block.blockId) && providerSource(block.source) &&
      (['unknown', 'superseded', 'withheld'].includes(block.state) ||
        (block.source.kind === 'native' || block.source.kind === 'provider_reasoning') && block.state === 'completed' && !settledReasoningBlocks.current.has(block.blockId))) selectionRetirements.add(block.blockId)
    if (selectionRetirements.size) clearSelection(selectionRetirements)
    for (const unit of reply.units) {
      unitPins.current.set(unit.unitId, unit)
      for (const call of unit.calls) callOwners.current.set(call.callId, unit.unitId)
    }
    providerAttempts.current = nextAttempts
    for (const block of reply.blocks) {
      sourcePins.current.set(block.blockId, block.source)
      if (providerSource(block.source)) {
        if (['unknown', 'superseded', 'withheld'].includes(block.state)) retiredProviderBlocks.current.add(block.blockId)
        if ((block.source.kind === 'native' || block.source.kind === 'provider_reasoning') && block.state === 'completed') settledReasoningBlocks.current.add(block.blockId)
      }
      if (block.source.kind === 'assistant_draft') {
        const previous = draftAttempts.current.get(block.source.unitId)
        if (!previous || previous.attempt <= block.source.attempt) draftAttempts.current.set(block.source.unitId, { attempt: block.source.attempt, blockId: block.blockId })
        if (block.state !== 'running') retiredDrafts.current.add(block.blockId)
        sawDraft.current = true
      }
    }
    return true
  }
  const accept = (owner: Lease, requestTicket: number, result: unknown, onlyDrafts = false) => {
    if (!isCurrent(owner, requestTicket) || !readable()) return false
    const parsed = processPaneReadResultSchemaV1.safeParse(result)
    if (!parsed.success) { fail(); return false }
    const reply = parsed.data
    if (reply.status !== 'snapshot' && reply.status !== 'unchanged') {
      if (reply.status === 'busy') { setPending(false); setMessage(t('dshProcessBusy')); return true }
      fail(); return false
    }
    if (!sameScope(reply.scope, owner.scope)) { fail(); return false }
    if (BigInt(reply.revision) < BigInt(owner.revision)) return true
    if (reply.status === 'snapshot') {
      if (onlyDrafts && reply.blocks.some(block => block.source.kind !== 'assistant_draft')) { fail(); return false }
      const next = { key: owner.key, generation: owner.generation, units: reply.units, blocks: reply.blocks }
      if (reply.blocks.some(block => hasProcessDisplayPayloadV2(block) && !expanded.current.has(block.blockId)) || !pin(next)) { fail(); return false }
      if (firstProjection.current === null) firstProjection.current = new Set(next.blocks.map(block => block.blockId))
      owner.hasSnapshot = true; setPresence(next.blocks.length > 0)
      const currentUnits = new Set(next.units.map(unit => unit.unitId))
      setStructure(previous => {
        // A draft-only read has no authority to replace ordinary control seats.
        // Retain their identities/anchors only; hidden prose and bodies stay cleared.
        const retained = onlyDrafts && previous?.key === owner.key && previous.generation === owner.generation
          ? previous.identities.filter(identity => identity.source.kind !== 'assistant_draft') : []
        for (const identity of retained) currentUnits.add(identity.source.unitId)
        return { key: owner.key, generation: owner.generation,
          units: stableUnits([...unitPins.current.values()].filter(unit => currentUnits.has(unit.unitId))),
          // Pins retain first-observed slots for unrelated source origins.
          // Tree compares accepted real ordinals only within their owning unit;
          // draft-only replies retain ordinary control identities.
          identities: [...sourcePins.current].filter(([blockId]) => retained.some(identity => identity.blockId === blockId) || next.blocks.some(block => block.blockId === blockId))
            // A closed state is control metadata, not private display text.
            // It keeps a preparing-only seat discoverable after fold clears payload.
            .map(([blockId, source]) => ({ blockId, source, state: next.blocks.find(block => block.blockId === blockId)?.state ?? retained.find(identity => identity.blockId === blockId)?.state })) }
      })
      if (next.blocks.some(block => block.source.kind === 'assistant_draft' && block.state !== 'running' ||
        block.source.kind === 'assistant_draft' && draftAttempts.current.get(block.source.unitId)?.blockId !== block.blockId)) clearSelection()
      setSnapshot({ ...next, blocks: next.blocks.filter(block => !(draftTerminal.current && block.source.kind === 'assistant_draft') &&
        (visibleIds.current === null || visibleIds.current.has(block.blockId) || !visibilityKnownIds.current.has(block.blockId)))
        .map(block => expanded.current.has(block.blockId) ? block : withoutBody(block)) })
    }
    owner.revision = reply.revision; setPending(false); setMessage(''); return true
  }
  const schedule = (owner: Lease) => {
    stopTimer()
    if (!pollable()) return
    timer.current = setTimeout(() => {
      timer.current = null
      const requestTicket = ticket.current
      enqueue(async () => {
        if (!isCurrent(owner, requestTicket) || !pollable()) return
        if (sourcePins.current.size === 0 && !draftsOnly()) emptyDiscovery.current.reads++
        try {
          const onlyDrafts = draftsOnly()
          const reply = await api!.readProcessPane({ viewerId: owner.viewerId, ...(owner.hasSnapshot ? { afterRevision: owner.revision } : {}), waitMs: 1000, ...(onlyDrafts ? { draftsOnly: true } : {}) })
          if (accept(owner, requestTicket, reply, onlyDrafts) && isCurrent(owner, requestTicket)) schedule(owner)
        } catch { if (isCurrent(owner, requestTicket)) fail() }
      })
    }, 500)
  }
  const read = (owner: Lease) => {
    const requestTicket = ticket.current
    enqueue(async () => {
      if (!isCurrent(owner, requestTicket) || !readable()) return
      const onlyDrafts = draftsOnly()
      try { if (accept(owner, requestTicket, await api!.readProcessPane({ viewerId: owner.viewerId, waitMs: 0, ...(onlyDrafts ? { draftsOnly: true } : {}) }), onlyDrafts)) schedule(owner) }
      catch { if (isCurrent(owner, requestTicket)) fail() }
    })
  }
  useLayoutEffect(() => {
    if (!terminalAuthorized) return
    ticket.current++; clearSelection()
    setSnapshot(value => value ? { ...value, blocks: value.blocks.filter(block => block.source.kind !== 'assistant_draft') } : null)
    // Settlement can still hydrate separately authorized complete native/tool
    // captures. The draft response seat is permanently retired for this turn.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [terminalAuthorized, key])
  useLayoutEffect(() => {
    const wasProcessing = processingRef.current
    processingRef.current = processing
    const activeOwner = lease.current
    if (processing && !wasProcessing && activeOwner) schedule(activeOwner)
    if (!processing) {
      stopTimer()
      const owner = lease.current
      if (wasProcessing && owner && readable()) {
        // One visible settlement read is distinct from ongoing discovery.
        settlementHydration.current = 1
        ticket.current++; owner.hasSnapshot = false; read(owner)
      }
    }
    // Processing closes polling, never acquires or renews authority.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [processing])
  useLayoutEffect(() => { recoveryAttempts.current = 0 }, [key, eligible])
  useLayoutEffect(() => {
    if (previousAvailability.current !== runtimeAvailable) {
      const changed = runtimeReady.current !== runtimeAvailable
      previousAvailability.current = runtimeAvailable; runtimeReady.current = runtimeAvailable
      if (!runtimeAvailable) { recoveryAttempts.current = 0; retire(true) }
      else if (changed && eligible && currentKey.current === key && eligibleRef.current === eligible) setAcquisition(value => value + 1)
    }
  }, [runtimeAvailable, retire, key, eligible])
  useLayoutEffect(() => {
    currentKey.current = key; eligibleRef.current = eligible; openRef.current = processOpen
    const epoch = generation.current, leaf = root.current
    if (!eligible || !leaf) return () => retire()
    const request = processPaneAuthorizeRequestSchemaV1.safeParse({ paneId, threadId, turnId })
    if (!request.success) return () => retire()
    // A real, user-opened native pane requests fresh Go authority. Mode and
    // process disclosures are absent from this effect's permission identity.
    if (runtimeReady.current && visibleRoot(leaf, paneId) && !sharedActiveCaseContextTransitionPending()) void (async () => {
      setPending(true)
      try {
        const parsed = processPaneAuthorizeResultSchemaV1.safeParse(await api!.authorizeProcessPane(request.data))
        if (epoch !== generation.current || currentKey.current !== key || root.current !== leaf || !eligibleRef.current || !visibleRoot(leaf, paneId)) {
          if (parsed.success && parsed.data.status === 'authorized') void api!.closeProcessPane({ viewerId: parsed.data.viewerId }).catch(() => undefined)
          return
        }
        if (!parsed.success || parsed.data.status !== 'authorized') {
          const retryable = parsed.success && (parsed.data.status === 'unavailable' || parsed.data.status === 'busy')
          fail()
          // Unavailability grants no authority and remains visible. At most
          // three fresh Go requests cover producer startup in this same real
          // pane; no retry has authority until Main/Go grants it. Invalid or
          // revoked replies are not retried, and refusal never means empty success.
          if (retryable && recoveryAttempts.current < 3) {
            const delay = [1000, 2000, 4000][recoveryAttempts.current++]
            recoveryTimer.current = setTimeout(() => {
              recoveryTimer.current = null
              if (currentKey.current === key && eligibleRef.current && root.current === leaf
                && runtimeReady.current && visibleRoot(leaf, paneId) && !sharedActiveCaseContextTransitionPending()) setAcquisition(value => value + 1)
            }, delay)
          }
          return
        }
        if (parsed.data.scope.threadId !== threadId || parsed.data.scope.turnId !== turnId) {
          void api!.closeProcessPane({ viewerId: parsed.data.viewerId }).catch(() => undefined); fail(); return
        }
        const owner: Lease = { ...parsed.data, generation: epoch, key, root: leaf, hasSnapshot: false }
        lease.current = owner
        settlementHydration.current = 1
        emptyDiscovery.current = { deadline: Date.now() + EMPTY_DISCOVERY_TIMEOUT_MS, reads: 0 }
        setPresence(parsed.data.presence.nativeBlockCount + parsed.data.presence.toolCaptureCount + (parsed.data.presence.assistantDraftCount ?? 0) > 0); setPending(false)
        if (openRef.current) read(owner)
      } catch { if (epoch === generation.current) fail() }
    })()
    const off = api!.onProcessPaneInvalidated({ paneId }, () => retire(true))
    const runtimeOff = api!.onRuntimeStatus(value => {
      const ready = value.state === 'running', restored = ready && !runtimeReady.current
      runtimeReady.current = ready
      if (!ready) { recoveryAttempts.current = 0; retire(true) }
      else if (restored && root.current === leaf && visibleRoot(leaf, paneId)) setAcquisition(value => value + 1)
    })
    const caseOff = subscribeBeforeSharedActiveCaseContextChange(() => retire(true))
    let wasVisible = visibleRoot(leaf, paneId)
    const visibilityChanged = () => {
      const visible = root.current === leaf && visibleRoot(leaf, paneId), restored = visible && !wasVisible
      wasVisible = visible
      if (!visible) { recoveryAttempts.current = 0; retire(true) }
      else if (restored && runtimeReady.current) setAcquisition(value => value + 1)
    }
    document.addEventListener('visibilitychange', visibilityChanged)
    const observer = new MutationObserver(visibilityChanged)
    for (let node: HTMLElement | null = leaf; node; node = node.parentElement) observer.observe(node, { attributes: true,
      attributeFilter: ['hidden', 'inert', 'aria-hidden', 'style', 'class', 'data-process-pane-id'], childList: true })
    return () => { observer.disconnect(); retire(); off(); runtimeOff(); caseOff(); document.removeEventListener('visibilitychange', visibilityChanged) }
    // processOpen and policy cannot acquire or renew permissions.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, key, eligible, paneId, acquisition, retire])
  useLayoutEffect(() => {
    openRef.current = processOpen
    const owner = lease.current
    if (!owner) return
    ticket.current++; stopTimer(); owner.hasSnapshot = false
    if (!processOpen) {
      clearSelection(); expanded.current.clear(); setExpandedIds(new Set()); setSnapshot(null); setPending(false)
      const requestTicket = ticket.current
      enqueue(async () => {
        if (!isCurrent(owner, requestTicket)) return
        try { await api!.setProcessPaneExpanded({ viewerId: owner.viewerId, expandedBlockIds: [] }) } catch { /* Body already retired locally. */ }
      })
    } else {
      if (explicitOpen.current) {
        emptyDiscovery.current = { deadline: Date.now() + EMPTY_DISCOVERY_TIMEOUT_MS, reads: 0 }
        settlementHydration.current = 1
      }
      explicitOpen.current = false
      if (sourcePins.current.size > 0 || Date.now() < emptyDiscovery.current.deadline && emptyDiscovery.current.reads < MAX_EMPTY_DISCOVERY_READS) read(owner)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [processOpen])
  useLayoutEffect(() => {
    const blockTransfer = (event: Event) => {
      const target = event.target instanceof Element ? event.target : null
      for (const element of event.type === 'dragstart' ? [target] : [target, document.activeElement]) {
        if (element?.closest('input,textarea,[contenteditable]:not([contenteditable="false"]),[role="textbox"]') && !root.current?.contains(element)) return
      }
      const nodes = privateRoots(), selection = document.getSelection()
      let intersects = event.type === 'dragstart' && nodes.some(node => target && node.contains(target))
      for (let i = 0; selection && !selection.isCollapsed && i < selection.rangeCount; i++) {
        try { if (nodes.some(node => selection.getRangeAt(i).intersectsNode(node))) intersects = true } catch { /* Retired DOM. */ }
      }
      if (intersects) { event.preventDefault(); event.stopImmediatePropagation() }
    }
    for (const name of ['copy', 'cut', 'dragstart']) window.addEventListener(name, blockTransfer, true)
    return () => { for (const name of ['copy', 'cut', 'dragstart']) window.removeEventListener(name, blockTransfer, true) }
  }, [])
  const changeExpansion = (ids: string[]) => {
    const owner = lease.current
    if (!owner || !isCurrent(owner, ticket.current) || !openRef.current) return
    stopTimer(); ticket.current++; owner.hasSnapshot = false
    expanded.current = new Set(ids); clearSelection(); setExpandedIds(new Set(ids)); setPending(true)
    setSnapshot(previous => previous ? { ...previous, blocks: previous.blocks.map(block => ids.includes(block.blockId) ? block : withoutBody(block)) } : null)
    const requestTicket = ticket.current
    enqueue(async () => {
      if (!isCurrent(owner, requestTicket)) return
      try {
        const parsed = processPaneActionResultSchemaV1.safeParse(await api!.setProcessPaneExpanded({ viewerId: owner.viewerId, expandedBlockIds: ids }))
        if (!isCurrent(owner, requestTicket)) return
        if (!parsed.success || parsed.data.status === 'unavailable' || parsed.data.status === 'revoked') { fail(); return }
        if (parsed.data.status === 'busy') { setPending(false); setMessage(t('dshProcessBusy')); schedule(owner); return }
        const onlyDrafts = draftsOnly()
        if (accept(owner, requestTicket, await api!.readProcessPane({ viewerId: owner.viewerId, waitMs: 0, ...(onlyDrafts ? { draftsOnly: true } : {}) }), onlyDrafts)) schedule(owner)
      } catch { if (isCurrent(owner, requestTicket)) fail() }
    })
  }
  const reportGeneration = generation.current
  const setVisible = (report: ProcessVisibilityReport) => {
    // A callback from a retired pane render cannot reveal the next owner.
    if (currentKey.current !== key || reportGeneration !== generation.current) return
    const ids = [...report.visiblePrivateIds]
    const previousReport = visibility.current
    const previous = visibleIds.current, next = new Set(ids), previouslyKnown = visibilityKnownIds.current
    visibilityKnownIds.current = new Set(sourcePins.current.keys())
    if (previouslyKnown.size === sourcePins.current.size && [...sourcePins.current.keys()].every(id => previouslyKnown.has(id))
      && previousReport?.kind === report.kind && previousReport.explicitGroupOpen === report.explicitGroupOpen && previousReport.draftsOnly === report.draftsOnly
      && previous && previous.size === next.size && [...previous].every(id => next.has(id))) return
    const actualOpen = previousReport !== null && report.explicitGroupOpen > previousReport.explicitGroupOpen
    visibility.current = report; visibleIds.current = next
    if (actualOpen && (report.kind === 'visible' || report.kind === 'empty')) {
      emptyDiscovery.current = { deadline: Date.now() + EMPTY_DISCOVERY_TIMEOUT_MS, reads: 0 }; settlementHydration.current = 1
    }
    const owner = lease.current
    if (!owner) return
    const automaticDrafts = !draftTerminal.current ? ids.filter(id => sourcePins.current.get(id)?.kind === 'assistant_draft' &&
      snapshot?.blocks.some(block => block.blockId === id && block.state === 'running')) : []
    const desiredExpansion = new Set([...expanded.current].filter(id => next.has(id)))
    for (const id of automaticDrafts) desiredExpansion.add(id)
    if (automaticDrafts.length && (desiredExpansion.size !== expanded.current.size || [...desiredExpansion].some(id => !expanded.current.has(id)))) {
      changeExpansion([...desiredExpansion]); return
    }
    if (!actualOpen && readable() && structure?.identities.every(block => next.has(block.blockId))
      && (previous === null || previouslyKnown.size === 0 && owner.hasSnapshot && [...sourcePins.current.keys()].every(id => firstProjection.current?.has(id)))) {
      setSnapshot(value => value ? { ...value } : null); schedule(owner); return
    }
    stopTimer(); ticket.current++; owner.hasSnapshot = false
    // New visible seats do not retire the reader's selection in retained seats.
    const hiddenIds = new Set([...(previous ?? [])].filter(id => !next.has(id)))
    if (hiddenIds.size) clearSelection(hiddenIds)
    const kept = [...expanded.current].filter(id => next.has(id)), changed = kept.length !== expanded.current.size
    expanded.current = new Set(kept)
    if (changed) setExpandedIds(new Set(kept))
    // Hidden groups retain only body-free structure/source identities. In-flight
    // responses are ticket-fenced; no hidden title/preview/path survives in state.
    setSnapshot(value => value ? { ...value, blocks: value.blocks.filter(block => next.has(block.blockId)) } : null)
    setPending(false)
    const requestTicket = ticket.current
    enqueue(async () => {
      if (!isCurrent(owner, requestTicket)) return
      if (changed || kept.length > 0) {
        try {
          const parsed = processPaneActionResultSchemaV1.safeParse(await api!.setProcessPaneExpanded({ viewerId: owner.viewerId, expandedBlockIds: kept }))
          if (!isCurrent(owner, requestTicket)) return
          if (!parsed.success || parsed.data.status !== 'ok') { fail(); return }
        } catch { if (isCurrent(owner, requestTicket)) fail(); return }
      }
      if (!readable()) return
      if ((actualOpen || report.draftsOnly && previousReport?.draftsOnly !== true) && sourcePins.current.size === 0) { read(owner); return }
      if (previous !== null && ids.some(id => !previous.has(id))) {
        const revealedKnownSeat = ids.some(id => !previous.has(id) && previouslyKnown.has(id))
        if (processingRef.current || revealedKnownSeat || settlementHydration.current > 0) {
          if (!processingRef.current && !revealedKnownSeat) settlementHydration.current--
          read(owner)
        }
      } else schedule(owner)
    })
  }
  const display = eligible && processOpen && snapshot?.key === key && snapshot.generation === generation.current ? snapshot : null
  const captureSlot = (block: ProcessDisplayBlockV1) => {
    const visible = visibleIds.current === null || visibleIds.current.has(block.blockId)
    const open = visible && expandedIds.has(block.blockId), body = open ? block.body : undefined
    const preparation = toolPreparationRowModel(block)
    const runningReasoning = block.kind === 'native_reasoning' && (block.source.kind === 'native' || block.source.kind === 'provider_reasoning') && block.state === 'running'
    const runningCommand = block.kind === 'shell' && block.source.kind === 'tool' && block.state === 'running' && Boolean(block.title)
    const runningInput = block.source.kind === 'tool' && block.state === 'running' && ['tool_input', 'shell', 'web_fetch'].includes(block.kind)
    const requestUrl = visible && block.kind === 'web_fetch' && block.state === 'running' ? block.requestUrl : undefined
    const inputDigest = requestUrl ? block.inputDigest : undefined
    const openWebLink = requestUrl && inputDigest ? () => {
      const owner = lease.current, requestTicket = ticket.current
      if (!owner || !isCurrent(owner, requestTicket) || !readable() || !api?.openProcessWebLink) return
      void api.openProcessWebLink({ viewerId: owner.viewerId, blockId: block.blockId, inputDigest, role: 'request' }).then(reply => {
        if (!isCurrent(owner, requestTicket)) return
        const parsed = processPaneActionResultSchemaV1.safeParse(reply)
        if (!parsed.success || parsed.data.status !== 'ok') { setMessage(t('dshProcessUnavailable')) }
      }).catch(() => { if (isCurrent(owner, requestTicket)) setMessage(t('dshProcessUnavailable')) })
    } : undefined
    const preview = preparation?.summary ?? [block.preview, block.path].filter(Boolean).join(' · ')
    // The Host count uses decoded UTF-16 units, matching DSH's KB rounding.
    const inputKilobytes = preparation?.inputKilobytes ?? (preview && block.state === 'completed' && block.inputUtf16CodeUnits !== undefined ? Math.ceil(block.inputUtf16CodeUnits / 1024) : undefined)
    const inputSize = inputKilobytes !== undefined ? <span data-protected-input-size data-protected-process-block={block.blockId}>{inputKilobytes}KB</span> : null
    const summary = preview ? <span data-protected-process-block={block.blockId} className={inputSize ? toolRowStyles.summary : undefined}>{preview}</span> : null
    const detail = <div data-protected-process-body className={styles.protectedBody}>
      {open && block.input ? <ToolRawIO input={block.input} /> : null}
      {body ? <>
        {block.kind === 'native_reasoning' ? <MarkdownText text={body.text} labels={labels} variant="compact" viewOnly streaming={runningReasoning} />
          : <ProcessToolCardBody block={block} />}
        {block.kind === 'native_reasoning' && body.error ? <div className={styles.error}><MarkdownText text={body.error} labels={labels} variant="compact" viewOnly /></div> : null}

      </> : open && runningCommand && block.classification !== 'withheld' ? <ProcessToolCardBody block={block} />
        : <span>{t(pending ? 'protectedToolLoading' : 'dshProcessWithheld')}</span>}
      <div className={styles.actions}>
        <span>{t('protectedToolDuration', { duration: block.durationMs })}</span>
        {block.exitCode !== undefined ? <span>{t('protectedToolExit', { code: block.exitCode })}</span> : null}
        {block.effectApplied !== undefined ? <span>{t('dshProcessEffect.' + (block.effectApplied === true ? 'applied' : block.effectApplied === false ? 'notApplied' : 'unknown'))}</span> : null}
        {block.settlementStatus ? <span>{t('dshProcessSettlement', { status: block.settlementStatus })}</span> : null}
        {body?.approximate ? <span>{t('dshProcessApproximate')}</span> : null}
        {block.truncated ? <span>{t('protectedToolTruncated')}</span> : null}
        {block.classification === 'credential_redacted' ? <span>{t('dshProcessRedacted')}</span> : null}
      </div>
    </div>
    const settled = !['preparing', 'running', 'unknown', 'superseded', 'withheld'].includes(block.state)
    const expandable = visible && block.kind !== 'tool_preparation' && (runningInput || block.classification !== 'withheld' && (settled || runningReasoning || runningCommand))
    const setOpen = (value: boolean) => { if (visible && expandable && value !== expanded.current.has(block.blockId) && (!pending || !value)) changeExpansion(value ? [...expanded.current, block.blockId] : [...expanded.current].filter(id => id !== block.blockId)) }
    return { blockId: block.blockId, summary, inputSize, requestUrl, openWebLink, preview: summary ? <>{summary}{inputSize}</> : null,
      body: <div data-protected-process-block={block.blockId}>{detail}</div>, expanded: open, expandable, setOpen, diffStats: body && block.effectApplied === true && block.state === 'completed'
        ? { added: body.addedLines, removed: body.removedLines } : undefined }
  }
  const renderBlock = (block: ProcessDisplayBlockV1, visible: boolean) => {
    if (!visible) return null
    const capture = captureSlot(block), open = capture.expanded, body = open ? block.body : undefined
    if (block.kind === 'tool_preparation') {
      const preparation = toolPreparationRowModel(block)
      if (!preparation) return null
      const titles: Record<string, string> = { read: t('toolBuiltinRead'), read_file: t('toolBuiltinRead'), write: t('toolBuiltinWrite'), write_file: t('toolBuiltinWrite'), edit: t('toolBuiltinEdit'), edit_file: t('toolBuiltinEdit'), multi_edit: t('toolBuiltinEdit'), bash: t('dshToolFamily.shell'), grep: t('dshSearch.grep'), glob: t('dshSearch.glob'), web_fetch: t('dshWebFetch.title'), present: t('dshPresent.title') }
      const icons = { read: DshBrowseOutlineRegular, read_file: DshBrowseOutlineRegular, write: DshEditOutlineRegular, write_file: DshEditOutlineRegular, edit: DshEditOutlineRegular, edit_file: DshEditOutlineRegular, multi_edit: DshEditOutlineRegular, bash: DshApiOutlineRegular, grep: DshSearchOutlineRegular, glob: DshSearchOutlineRegular, web_fetch: DshBrowseOutlineRegular, present: DshDeliverDocRegular }
      const Icon = Object.hasOwn(icons, preparation.toolName) ? icons[preparation.toolName as keyof typeof icons] : DshSparkleRegular
      return <div data-protected-process-block={block.blockId}><ToolRow icon={<Icon size={14} />} toolName={preparation.toolName}
        title={Object.hasOwn(titles, preparation.toolName) ? titles[preparation.toolName] : preparation.toolName} summary={preparation.summary} summarySuffix={capture.inputSize}
        state="preparing" expandable={false} expanded={false} /></div>
    }
    const summaryReasoning = (block.source.kind === 'native' || block.source.kind === 'provider_reasoning') && block.source.reasoningForm === 'summary'
    const title = summaryReasoning ? t('dshProcessReasoningSummary') : block.title ?? t(block.kind === 'web_fetch' ? 'dshWebFetch.title' : block.kind === 'native_reasoning' ? 'dshProcessReasoning' : 'dshProcessKind.' + block.kind)
    const summary = block.kind === 'web_fetch' && capture.requestUrl ? capture.requestUrl : capture.summary
    const toggle = () => capture.setOpen(!open)
    return <div data-protected-process-block={block.blockId}>
      {block.kind === 'native_reasoning' ? <ReasoningRow id={block.blockId} policy={policy} protectedBody={capture.body} title={title}
        preview={[block.preview, block.path].filter(Boolean).join(' · ')} expanded={open} expandable={capture.expandable} running={block.state === 'running'} onToggle={toggle} />
        : <ToolRow icon={block.kind === 'web_fetch' ? <DshBrowseOutlineRegular size={14} /> : block.kind === 'shell' ? <DshApiOutlineRegular size={14} /> : <DshEditOutlineRegular size={14} />} title={title} summary={summary} summarySuffix={capture.inputSize}
          onOpenLink={block.kind === 'web_fetch' ? capture.openWebLink : undefined}
          state={block.state === 'canceled' ? 'stopped' : ['failed', 'timeout'].includes(block.state) ? 'error' : block.state === 'running' ? 'running' : 'ok'} expanded={open} expandable={capture.expandable}
          onToggle={toggle} diffStats={body && block.effectApplied === true && block.state === 'completed'
            ? { added: body.addedLines, removed: body.removedLines } : undefined}>{capture.body}</ToolRow>}
    </div>
  }
  const controls = eligible && structure?.key === key && structure.generation === generation.current ? structure : null
  const local: ProcessLocalRows | undefined = controls && threadId && turnId ? { threadId, turnId, units: controls.units, identities: controls.identities,
    blocks: display?.blocks ?? [], render: renderBlock, capture: captureSlot } : undefined
  const draftsVisible = eligible && processOpen && !draftTerminal.current
  const draftSeats = processDraftSeats(controls ?? undefined)
  const showAnswer = !sawDraft.current || terminalAuthorized
  const hasContent = processBlocks.some(block => block.kind !== 'system' || block.severity !== 'error') || presence === true || offerProcessEntry
  return <div ref={root} data-process-pane-id={paneId} data-process-surface={route?.surface} className={styles.process}>
    {!processing && (status || hasContent) ? <TurnProcessView open={publicDisclosure.open || (!requestProcessDisplay && processOpen)}
      onToggle={() => {
        if (publicDisclosure.open && !processOpen) { publicDisclosure.setOpen(false); return }
        explicitOpen.current = requestProcessDisplay || !openRef.current; (requestProcessDisplay ? onRequestProcessDisplay : onToggle)?.()
      }} collapsible={requestProcessDisplay || collapsible}
      hasContent={hasContent} durationMs={completedDurationMs} status={status} /> : null}
    <div ref={publicBody} className={styles.processBody} data-public-process-body>
      <ToolCallTree key={key} onInspect={onInspect} blocks={processBlocks} replyBoundaryAfterIds={processReplyBoundaryAfterIds} turnSettled={status !== undefined} processing={processing} policy={policy} local={local} onVisibility={setVisible} onRevealPublic={revealPublic} visible={processOpen} publicVisible={publicOpen} draftsVisible={draftsVisible} privateGeneration={generation.current} resetKey={processOpen ? 'open' : 'folded'} />
      {browserPreview && route?.visible ? <span role="status">{t('dshProcessUnavailable')}</span>
        : eligible && message ? <span role="status">{message}</span> : null}
    </div>
    <div data-analytix-assistant-response-seat className={styles.responseSeat}>
      {draftsVisible ? draftSeats.map(seat => {
        const block = display?.blocks.find(value => value.blockId === seat.blockId)
        const segments = block?.state === 'running' && expandedIds.has(seat.blockId) ? block.draftSegments : undefined
        return <div key={seat.key} data-chat-seat={seat.key}>
          {block?.state === 'running' ? <div data-protected-process-block={seat.blockId} data-protected-assistant-draft={block.contentClassification}>
            <div className={styles.draftLabel}>{t('dshProcessAssistantDraft')}</div>
            {/* Unclassified source prose stays component-local and view-only.
                Raw HTML remains literal; only the authorized terminal can
                replace this draft with an accepted answer. */}
            {segments ? <div data-protected-process-body><MarkdownText key={block.blockId}
              text={segments.map(segment => segment.text).join('')} labels={labels} viewOnly streaming /></div>
              : <span>{t(pending ? 'protectedToolLoading' : 'dshProcessWithheld')}</span>}
          </div> : null}
        </div>
      }) : null}
      {showAnswer ? answer : null}
    </div>
  </div>
}
