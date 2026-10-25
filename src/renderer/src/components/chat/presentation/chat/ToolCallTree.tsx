/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useContext, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChatBlock } from '../../../../agent/types'
import { ProtectedToolInspectorSelection, ProtectedToolResultScope } from '../ProtectedToolResult'
import { GateResponseContext, MessageBubble } from '../../message-timeline-bubbles'
import { describeProcessBlock, groupProcessSections, presentationPolicyFor, rankedProcessActivities, processTitle, type ChatPresentationPolicy } from './chat-view-model'
import type { ProcessDisplayBlockV1, ProcessDisplayUnitV2 } from '../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import { DshAgentPresetOutlineRegular, DshApiOutlineRegular, DshBrowseOutlineRegular, DshCodeOutlineRegular, DshEditOutlineRegular, DshGlobeOutlineRegular, DshPlanOutlineRegular, DshQuestionOutlineRegular, DshSearchOutlineRegular, DshSparkleRegular, DshThinkOutlineRegular } from '../DshToolIcons'
import styles from './ChatView.module.css'
import { TurnStatusView } from './TurnStatusView'
import { ToolRow } from './ToolRow'
import { ChatGroupSeat } from './ChatGroupSeat'
import { GenericToolCard } from './GenericToolCard'
import { toolFamilyFor, toolFamilyForName, processActivityFor, processActivityForName, ReadRow, FileMutationRow, BashRow, WebRow, SearchRow, PlanRow, SubagentRow, DetailsRow, type ToolViewProps } from './ToolViews'
import { useChatStore } from '../../../../store/chat-store'
import { useDisclosure } from './use-disclosure'
import { requestUserInputLocalReopen } from './useUserInputLocalDisplay'
import { AskQuestionRow, QuestionAssociationContext, QuestionAssociationLeaf } from './toolviews/ask-question-row'
import { WebFetchRow } from './toolviews/web-fetch-row'
import { GoalRow, RecordedJobRow, goalToolNames } from './toolviews/recorded-tool-row'
import { recordedJobData, recordedJobNames } from './models/recorded-tool-card-model'
import { hasPublicFocus } from './searchable-hidden'
import { TodoRow } from './toolviews/todo-row'
import { SkillRow } from './toolviews/skill-row'
import { OfficeGenerationRow } from './toolviews/office-generation-row'
import { PresentRow } from './toolviews/present-row'
import { FileResultRow } from './toolviews/file-result-row'
import { fileResultToolKind } from './models/file-result-card-model'
import { PresentClosingTail } from './PresentedFiles'
import { toolPreparationRowModel } from './models/tool-call-model'
const detailKeys = new Set(['schedule_create', 'schedule_list', 'schedule_delete', 'schedule_update',
  'cordis_inspect_list', 'cordis_inspect_query', 'cordis_inspect_self', 'workflow', 'ralph', 'session_event_read', 'session_event_search', 'session_event_trace',
  'session_search', 'session_trace', 'list_subagent_models', 'list_agents', 'send_message', 'interrupt_agent', 'job_list', 'job_output', 'job_kill',
  'terminal_open', 'terminal_read', 'terminal_list', 'terminal_signal', 'terminal_close', 'lsp', 'spawn_teammate', 'team_task_create', 'team_task_get',
  'team_task_update', 'team_task_list', 'wait_agent'])
function ToolCallView(props: ToolViewProps) {
  // Read live exports at render; the Search view imports the timeline which imports this module.
  const toolViews = { read: ReadRow, diff: FileMutationRow, shell: BashRow, web: WebRow, search: SearchRow, plan: PlanRow, subagent: SubagentRow, tool: GenericToolCard }
  if (props.block.meta?.toolName === 'web_fetch' && props.block.toolKind === 'tool_call') return <WebFetchRow {...props} />
  const producer = props.block.meta?.toolName
  if (fileResultToolKind(producer) && props.block.toolKind === fileResultToolKind(producer)) return <FileResultRow {...props} />
  if (producer === 'generate_office_document' && props.block.toolKind === 'file_change') return <OfficeGenerationRow {...props} />
  if (props.block.toolKind === 'tool_call') {
    if (producer === 'present') return <PresentRow {...props} />
    if (goalToolNames.some(name => name === producer)) return <GoalRow {...props} />
    if (producer === 'todo_write') return <TodoRow {...props} />
    if (producer === 'run_skill') return <SkillRow {...props} />
  }
  if (typeof producer === 'string' && Object.hasOwn(recordedJobNames, producer) && props.block.toolKind === 'command_execution') return <RecordedJobRow {...props} />
  const name = typeof props.block.meta?.toolName === 'string' ? props.block.meta.toolName.toLowerCase() : ''
  if (['user_input', 'request_user_input', 'ask_user_question'].includes(name) && typeof props.block.meta?.callId === 'string' && props.block.toolKind === 'tool_call') return <AskQuestionRow {...props} callId={props.block.meta.callId} />
  const View = detailKeys.has(name) ? DetailsRow : toolViews[toolFamilyFor(props.block)]
  return <View {...props} />
}
function GateRequestSeat({ block, visible, resetKey, anchorsInTree }: { block: Extract<ChatBlock, { kind: 'approval' | 'user_input' }>; visible: boolean; resetKey: string; anchorsInTree: ChatBlock[] }) {
  const { t } = useTranslation('common'), gateContext = useContext(GateResponseContext), threadId = useContext(ProtectedToolResultScope)
  const activeThreadId = useChatStore(state => state.activeThreadId), selectInput = useChatStore(state => state.selectUserInputPanel)
  const disclosure = useDisclosure(block.id + ':' + resetKey)
  const setFallbackOpen = disclosure.setOpen
  const associations = useContext(QuestionAssociationContext)
  const paired = block.kind === 'user_input' && Boolean(associations[block.id]) && anchorsInTree.some(anchor => anchor.id === associations[block.id])
  // A fallback disclosure must not remount raw content when a later privacy
  // revoke retires the real association. Opening it again is a user action.
  useLayoutEffect(() => { if (paired) setFallbackOpen(false) }, [paired, setFallbackOpen])
  const requestId = block.kind === 'approval' ? block.approvalId : block.requestId
  const openOwner = () => {
    if (gateContext) { gateContext.onOpenOwnerThread?.(requestId); return }
    // A retired/missing private association leaves a metadata-only owner seat.
    // Explicit selection reopens that existing leaf through fresh Host authority.
    if (block.kind === 'user_input') {
      if (threadId === activeThreadId && threadId && typeof block.meta?.turnId === 'string') {
        requestUserInputLocalReopen({ threadId, turnId: block.meta.turnId, itemId: block.id, requestId: block.requestId })
        selectInput(requestId)
      }
      return
    }
    const composer = Array.from(document.querySelectorAll<HTMLElement>('[data-analytix-gate-composer] [data-gate-request-id]'))
      .find(node => node.dataset.gateRequestId === requestId && node.closest<HTMLElement>('[data-analytix-gate-composer]')?.dataset.gateThreadId === threadId)
    const control = composer?.querySelector<HTMLElement>('button:not(:disabled),textarea:not(:disabled),input:not(:disabled)')
    ;(control ?? composer)?.focus({ preventScroll: true })
  }
  const title = t(block.kind === 'approval' ? 'approvalTitle' : 'userInputTitle')
  if (paired) return null
  return block.status === 'pending' ? <div data-gate-request-id={requestId}><ToolRow title={title} summary={t('pendingRequestNotResolved')}
    actions={<button type="button" onClick={openOwner} disabled={Boolean(gateContext ? !gateContext.onOpenOwnerThread : block.kind === 'user_input' && threadId !== activeThreadId)}>{t(gateContext ? 'respondInOwnedThread' : 'dshRespond')}</button>} /></div>
    : <ToolRow title={title} summary={block.status} expanded={visible && disclosure.open} expandable onToggle={disclosure.toggle}
      actions={block.kind === 'user_input' && block.status === 'continued' ? <button type="button" onClick={openOwner}
        disabled={Boolean(gateContext ? !gateContext.onOpenOwnerThread : threadId !== activeThreadId)}>{t(gateContext ? 'respondInOwnedThread' : 'dshRespond')}</button> : undefined}>
      <GateResponseContext.Provider value={gateContext ?? (block.kind === 'user_input' ? { onOpenOwnerThread: openOwner } : {})}><MessageBubble block={block} nested /></GateResponseContext.Provider>
    </ToolRow>
}
export type ProcessCaptureSlot = {
  blockId: string; preview: ReactNode; body: ReactNode; expanded: boolean; expandable: boolean
  setOpen: (open: boolean) => void; diffStats?: { added: number; removed: number }; requestUrl?: string; openWebLink?: () => void
}
export type ProcessLocalRows = {
  threadId: string; turnId: string; units: ProcessDisplayUnitV2[]
  identities: (Pick<ProcessDisplayBlockV1, 'blockId' | 'source'> & { state?: ProcessDisplayBlockV1['state'] })[]; blocks: ProcessDisplayBlockV1[]
  render: (block: ProcessDisplayBlockV1, visible: boolean) => ReactNode
  capture: (block: ProcessDisplayBlockV1) => ProcessCaptureSlot
}
export type ProcessVisibilityReport = {
  kind: 'whole_folded' | 'all_groups_folded' | 'visible' | 'empty'
  visiblePrivateIds: readonly string[]
  explicitGroupOpen: number
  draftsOnly?: boolean
}
// A draft seat belongs to the real logical unit, independent of its physical
// retry block. No native ordinal, public tool row or caller-created call anchors
// are used to locate it. Both Provider channels remain untrusted process prose.
export function processDraftSeats(local?: Pick<ProcessLocalRows, 'units' | 'identities'>): Array<{ key: string; blockId: string }> {
  return (local?.units ?? []).flatMap(unit => {
    const drafts = local!.identities.filter(identity => identity.source.kind === 'assistant_draft' && identity.source.unitId === unit.unitId)
      .sort((a, b) => a.source.kind === 'assistant_draft' && b.source.kind === 'assistant_draft' ? b.source.attempt - a.source.attempt : 0)
    return drafts[0] ? [{ key: unit.unitId + ':assistant_draft', blockId: drafts[0].blockId }] : []
  })
}
type CallAnchor = ProcessDisplayUnitV2['calls'][number]
function matchesAnchor(block: ChatBlock, anchor: CallAnchor, turnId: string): boolean {
  if (block.kind !== 'tool' || block.meta?.turnId !== turnId || block.meta.callId !== anchor.callId) return false
  const item = block.meta.sourceItemId
  if (item !== anchor.toolCallItemId && (anchor.toolResultItemId === undefined || item !== anchor.toolResultItemId)) return false
  const local = block.meta.localResult
  return !local || local.turnId === turnId && local.callId === anchor.callId && local.resultItemId === anchor.toolResultItemId
}
type ReasoningSource = Extract<ProcessDisplayBlockV1['source'], { kind: 'native' | 'provider_reasoning' }>
const reasoningSource = (source: ProcessDisplayBlockV1['source']): source is ReasoningSource => source.kind === 'native' || source.kind === 'provider_reasoning'
type OrderedSource = Extract<ProcessDisplayBlockV1['source'], { kind: 'native' | 'provider_reasoning' | 'tool_preparation' }>
type OrderedIdentity = ProcessLocalRows['identities'][number] & { source: OrderedSource }
const sameSourceOrigin = (source: ProcessDisplayBlockV1['source'], unit: ProcessDisplayUnitV2): source is OrderedSource =>
  (reasoningSource(source) || source.kind === 'tool_preparation') && source.unitId === unit.unitId
  && source.producerId === unit.producerId && source.providerSequence === unit.providerSequence
function orderedUnitSources(identities: ProcessLocalRows['identities'], unit: ProcessDisplayUnitV2): ProcessLocalRows['identities'] {
  const sources = identities.filter(identity => identity.source.unitId === unit.unitId)
  const ordered = sources.filter((identity): identity is OrderedIdentity => sameSourceOrigin(identity.source, unit))
    .sort((left, right) => {
      const a = BigInt(left.source.sourceOrdinal), b = BigInt(right.source.sourceOrdinal)
      return a < b ? -1 : a > b ? 1 : 0
    })
  let next = 0
  // Drafts, captures and other origins keep their established slots.
  return sources.map(identity => sameSourceOrigin(identity.source, unit) ? ordered[next++]! : identity)
}
type NativePlacement = { owner: string; anchorRow?: string; relation: 'before' | 'after' | 'unit' }
type Seat = { key: string; publicBlock?: ChatBlock; privateIdentity?: ProcessLocalRows['identities'][number]; privateBlock?: ProcessDisplayBlockV1 }
type Group = { key: string; owner: string; seats: Seat[]; unit: boolean; presentationClosed?: boolean; status?: 'error' | 'retry' }
// Only real call/item anchors join a private capture to its public row. Public
// order locates a proven anchor; it never creates an association.
function processGroups(blocks: ChatBlock[], local?: ProcessLocalRows, nativeOwners = new Map<string, NativePlacement>(), replyBoundaryAfterIds: readonly string[] = []): Group[] {
  const sections = groupProcessSections(blocks, replyBoundaryAfterIds)
  const rows = sections.flatMap(section => section.blocks).map(block => ({ key: block.id, publicBlock: block } as Seat))
  const unitRows = new Set<string>()
  const details = new Map(local?.blocks.map(block => [block.blockId, block]) ?? [])
  const nativeBefore = new Map<string, Seat[]>(), nativeAfter = new Map<string, Seat[]>()
  const tail: Group[] = []
  for (const unit of local?.units ?? []) {
    const find = (anchor: CallAnchor) => {
      const found = rows.filter(row => row.publicBlock && matchesAnchor(row.publicBlock, anchor, local!.turnId))
      return found.length === 1 ? found[0] : undefined
    }
    const calls = unit.calls.map(find).filter((row): row is Seat => row !== undefined)
    for (const row of calls) {
      unitRows.add(row.key)
      const captures = local!.identities.filter(block => block.source.kind === 'tool' && block.source.unitId === unit.unitId
        && matchesAnchor(row.publicBlock!, block.source, local!.turnId))
      const publicTool = row.publicBlock?.kind === 'tool' ? row.publicBlock : undefined
      // Exact typed presenter ownership survives the pane clearing hidden
      // metadata. Ordinary Inspect has request facts; mutation Diff stays here.
      const typedResultOwner = publicTool?.status !== 'running' && publicTool?.meta?.localResult && (
        publicTool.toolKind === 'command_execution' && publicTool.meta.toolName === 'bash' ||
        publicTool.toolKind === 'tool_call' && publicTool.meta.toolName === 'web_fetch')
      if (captures.length === 1 && !typedResultOwner) {
        row.privateIdentity = captures[0]; row.privateBlock = details.get(captures[0].blockId)
      }
    }
    const sources = orderedUnitSources(local!.identities, unit)
    // A retired preparation has no visible payload. It remains a real source
    // boundary and control owner for its exact admitted call/item seat.
    const boundaries = sources.flatMap((identity, index) => {
      if (identity.source.kind !== 'tool_preparation' || !sameSourceOrigin(identity.source, unit)) return []
      const callId = identity.source.callId
      const anchor = unit.calls.find(call => call.callId === callId), row = anchor && find(anchor)
      if (!row) return []
      if (!row.privateIdentity) row.privateIdentity = identity
      return [{ index, row }]
    })
    const privateSeats = sources.flatMap((identity, index) => {
      if (reasoningSource(identity.source)) return [{ index, seat: { key: identity.blockId, privateIdentity: identity, privateBlock: details.get(identity.blockId) } as Seat }]
      if (identity.source.kind !== 'tool_preparation' || (details.get(identity.blockId)?.state ?? identity.state) !== 'preparing' || boundaries.some(boundary => boundary.index === index)) return []
      return [{ index, seat: { key: unit.unitId + ':preparation:' + identity.source.callId, privateIdentity: identity, privateBlock: details.get(identity.blockId) } as Seat }]
    })
    if (!privateSeats.length) continue
    const first = rows.find(row => calls.includes(row))
    const preceding = unit.precedingCalls.map(find).filter((row): row is Seat => row !== undefined)
    const last = [...rows].reverse().find(row => preceding.includes(row))
    const publicOwner = (row: Seat) => 'public:' + sections.find(section => section.blocks.some(block => block.id === row.key))!.blocks[0].id
    const anchorFor = (placement: NativePlacement) => placement.anchorRow ? rows.find(row => row.key === placement.anchorRow
      && publicOwner(row) === placement.owner && (calls.includes(row) || preceding.includes(row))) : undefined
    const reasoningPlacements = new Map<string, NativePlacement>(), unshareableSeats = new Set<string>()
    const placementFor = (index: number, seat: Seat): NativePlacement | undefined => {
      const retained = nativeOwners.get(seat.key), source = seat.privateIdentity?.source
      const comparable = source !== undefined && sameSourceOrigin(source, unit)
      const previous = comparable ? [...boundaries].reverse().find(boundary => boundary.index < index) : undefined
      const next = comparable ? boundaries.find(boundary => boundary.index > index) : undefined
      // Public row order and parent ownership are immutable. A settled capture
      // appended at result time cannot substitute for a missing call-start boundary.
      const incompatible = previous && next && (publicOwner(previous.row) !== publicOwner(next.row) || rows.indexOf(previous.row) >= rows.indexOf(next.row))
      if (incompatible) unshareableSeats.add(seat.key)
      const boundary = incompatible ? undefined : next ?? previous
      const reason = source !== undefined && reasoningSource(source)
      // Preserve the existing native unit association when no start boundary
      // was projected. It proves ownership only, not interleaving with that call.
      const fallback: NativePlacement = reason && (first || last)
        ? { owner: publicOwner((first ?? last)!), anchorRow: first?.key ?? last?.key, relation: first ? 'before' : 'after' }
        : { owner: 'unit:' + unit.unitId, relation: 'unit' }
      let neighbour: NativePlacement | undefined
      if (!retained && !boundary && !incompatible && source?.kind === 'tool_preparation' && sameSourceOrigin(source, unit)) {
        // Use actual reasoning seats, even when both arrive in one poll. Sharing
        // their validated bucket does not associate a public call with this prep.
        const neighbours = privateSeats.flatMap(({ index: peerIndex, seat: peer }) => {
          const peerSource = peer.privateIdentity?.source, placed = reasoningPlacements.get(peer.key)
          if (!peerSource || !reasoningSource(peerSource) || !sameSourceOrigin(peerSource, unit)
            || peerSource.attempt !== source.attempt) return []
          const shareable = Boolean(placed && !unshareableSeats.has(peer.key)
            && (placed.owner === 'unit:' + unit.unitId && placed.relation === 'unit' || anchorFor(placed)))
          return [{ index: peerIndex, placement: placed, shareable }]
        })
        const before = [...neighbours].reverse().find(peer => peer.index < index)
        const after = neighbours.find(peer => peer.index > index)
        // Do not skip an unresolved nearer reasoning seat to borrow a farther one.
        if ((!before || before.shareable) && (!after || after.shareable)) {
          const a = before?.placement, b = after?.placement
          if (!a || !b || a.owner === b.owner && a.anchorRow === b.anchorRow && a.relation === b.relation) neighbour = b ?? a
        }
      }
      if (!retained && !neighbour && unit.placement === 'anchored' && !first && !last) return undefined
      const proven: NativePlacement | undefined = boundary
        ? { owner: publicOwner(boundary.row), anchorRow: boundary.row.key, relation: boundary === next ? 'before' : 'after' }
        : undefined
      if (retained && proven && proven.owner !== retained.owner) unshareableSeats.add(seat.key)
      // Parent ownership is permanent; a real same-origin boundary may refine
      // before/after only inside that parent. Cross-parent cases keep the pin.
      return retained
        ? proven && proven.owner === retained.owner ? proven : retained
        : proven ?? neighbour ?? fallback
    }
    // Calculate only real reasoning placements first, then insert all actual
    // seats in source order. This avoids making first-poll order depend on kind.
    for (const { index, seat } of privateSeats) if (seat.privateIdentity && reasoningSource(seat.privateIdentity.source)) {
      const placement = placementFor(index, seat)
      if (placement) reasoningPlacements.set(seat.key, placement)
    }
    const retainedTail: Seat[] = []
    for (const { index, seat } of privateSeats) {
      const placement = reasoningPlacements.get(seat.key) ?? placementFor(index, seat)
      if (!placement) continue
      nativeOwners.set(seat.key, placement)
      const anchored = anchorFor(placement)
      if (anchored && placement.relation === 'before') nativeBefore.set(anchored.key, [...nativeBefore.get(anchored.key) ?? [], seat])
      else if (anchored && placement.relation === 'after') nativeAfter.set(anchored.key, [...nativeAfter.get(anchored.key) ?? [], seat])
      else if (placement.relation === 'unit' && placement.owner === 'unit:' + unit.unitId) retainedTail.push(seat)
    }
    if (retainedTail.length) tail.push({ key: unit.unitId, owner: unit.unitId, seats: retainedTail, unit: true })
  }
  // Public section ownership is permanent. Late private anchors insert seats
  // inside that existing owner; neither mode nor native arrival reparents rows.
  return [...sections.map(section => ({ key: 'public:' + section.blocks[0].id, owner: section.id, unit: section.blocks.some(block => unitRows.has(block.id)),
    presentationClosed: section.presentationClosed === true,
    status: section.kind === 'error' || section.kind === 'retry' ? section.kind : undefined,
    seats: section.blocks.flatMap(block => {
      const row = rows.find(seat => seat.key === block.id)!
      return [...nativeBefore.get(row.key) ?? [], row, ...nativeAfter.get(row.key) ?? []]
    }) })), ...tail.map(group => ({ ...group, key: 'unit:' + group.owner }))]
}
function ChatNodeSeat({ seat, treeOwnerId, processing, policy, local, visible, publicVisible, resetKey, privateGeneration, onInspect, anchorsInTree }: {
  treeOwnerId: string; anchorsInTree: ChatBlock[]; seat: Seat; processing: boolean; policy: ChatPresentationPolicy; local?: ProcessLocalRows; visible: boolean; publicVisible: boolean; resetKey: string; privateGeneration: number; onInspect?: (turnId: string, callId: string) => void
}) {
  const block = seat.publicBlock
  // The pinned Read presenter keeps its path/typed result; a unified private
  // producer does not add an arguments body to its single-file running row.
  const readInput = block?.kind === 'tool' && toolFamilyFor(block) === 'read' && seat.privateBlock?.kind === 'tool_input'
  return <div className={styles.groupBody} data-chat-seat={seat.key} data-process-tree-owner={treeOwnerId}>
    {block?.kind === 'tool' ? <ToolCallView block={block} processing={processing} policy={policy} visible={visible} publicVisible={publicVisible}
      publicResetKey={resetKey} resetKey={resetKey} privateGeneration={privateGeneration} onInspect={onInspect}
      capture={visible && seat.privateBlock && local && !readInput ? local.capture(seat.privateBlock) : undefined} />
      : block?.kind === 'assistant' ? <><MessageBubble block={block} nested showActions={false} /><PresentClosingTail closing={block} /></>
      : block?.kind === 'compaction' || block?.kind === 'system' ? <TurnStatusView block={block} />
      : block?.kind === 'approval' || block?.kind === 'user_input' ? <GateRequestSeat block={block} visible={visible} resetKey={resetKey} anchorsInTree={anchorsInTree} />
      : block ? <MessageBubble block={block} nested />
      : visible && seat.privateBlock && local ? local.render(seat.privateBlock, true) : null}
  </div>
}
export function ToolCallTree({ blocks, processing, replyBoundaryAfterIds, turnSettled = false, policy = presentationPolicyFor('detailed'), local, visible = true, publicVisible = visible, draftsVisible = false, resetKey = '', privateGeneration = 0, onVisibility, onInspect, onRevealPublic }: {
  blocks: ChatBlock[]; processing: boolean; replyBoundaryAfterIds?: readonly string[]; turnSettled?: boolean; policy?: ChatPresentationPolicy; local?: ProcessLocalRows; visible?: boolean; publicVisible?: boolean; draftsVisible?: boolean; resetKey?: string; privateGeneration?: number; onVisibility?: (report: ProcessVisibilityReport) => void; onRevealPublic?: () => void; onInspect?: (turnId: string, callId: string) => void
}) {
  const { t } = useTranslation('common')
  const threadId = useContext(ProtectedToolResultScope)
  const treeOwnerId = useId()
  const selectionOwner = JSON.stringify([threadId, local?.threadId, local?.turnId])
  const selectionKey = JSON.stringify([selectionOwner, resetKey, privateGeneration])
  const selectionScope = useRef({ owner: selectionOwner, key: selectionKey })
  const active = typeof document === 'undefined' ? null : document.activeElement
  const focusedSeat = active?.closest<HTMLElement>('[data-chat-seat]')
  const publicInspector = active?.closest<HTMLElement>('[data-public-job-inspector]')?.querySelector<HTMLElement>('[data-tool-result-inspector]')
  const focusedJob = focusedSeat?.dataset.processTreeOwner === treeOwnerId
    ? blocks.find(block => block.id === focusedSeat.dataset.chatSeat) : undefined
  const retainPublicSelection = publicInspector?.contains(active) && focusedJob?.kind === 'tool' &&
    focusedJob.meta?.callId === publicInspector.dataset.callId && recordedJobData(focusedJob) && hasPublicFocus(focusedSeat ?? null)
  // A private generation still retires every protected leaf via its original key.
  // Only this tree's focused, admitted public Inspector keeps the shared selection.
  if (selectionScope.current.owner !== selectionOwner || !retainPublicSelection) selectionScope.current = { owner: selectionOwner, key: selectionKey }
  const activeThreadId = useChatStore(state => state.activeThreadId)
  const [associations, setAssociations] = useState<Record<string, string>>({})
  const [onAssociation] = useState(() => (id: string, toolId: string | null) => setAssociations(previous => {
    if ((previous[id] ?? null) === toolId) return previous
    const next = { ...previous }; if (toolId) next[id] = toolId; else delete next[id]; return next
  }))
  const inputs = blocks.filter((block): block is Extract<ChatBlock, { kind: 'user_input' }> => block.kind === 'user_input')
  const associated = visible && threadId === activeThreadId ? associations : {}
  const groupOpenIntent = useRef(0)
  const nativeOwners = useRef({ generation: privateGeneration, owners: new Map<string, NativePlacement>() })
  if (nativeOwners.current.generation !== privateGeneration) nativeOwners.current = { generation: privateGeneration, owners: new Map() }
  const [expandedGroups, setExpandedGroups] = useState<{ version: string; ids: Set<string>; publicIds: Set<string> }>({ version: resetKey, ids: new Set(), publicIds: new Set() })
  useLayoutEffect(() => { setExpandedGroups({ version: resetKey, ids: new Set(), publicIds: new Set() }) }, [resetKey])
  const openGroups = expandedGroups.version === resetKey ? expandedGroups.ids : new Set<string>()
  const publicGroups = expandedGroups.version === resetKey ? expandedGroups.publicIds : new Set<string>()
  const toggle = (key: string) => setExpandedGroups(previous => {
    const ids = new Set(previous.version === resetKey ? previous.ids : [])
    const publicIds = new Set(previous.version === resetKey ? previous.publicIds : [])
    if (ids.has(key) || publicIds.has(key)) { ids.delete(key); publicIds.delete(key) }
    else { ids.add(key); groupOpenIntent.current++ }
    return { version: resetKey, ids, publicIds }
  })
  // Browser find and public focus reveal presentation only. The private group
  // set and explicit-open counter remain the existing Host visibility inputs.
  const reveal = (key: string) => {
    onRevealPublic?.()
    setExpandedGroups(previous => {
      if (previous.version === resetKey && previous.publicIds.has(key)) return previous
      return { version: resetKey, ids: previous.version === resetKey ? previous.ids : new Set(),
        publicIds: new Set([...(previous.version === resetKey ? previous.publicIds : []), key]) }
    })
  }
  const groups = processGroups(blocks, local, nativeOwners.current.owners, replyBoundaryAfterIds), visiblePrivateIds: string[] = []
  // This is the public display tail, not source order or a private-unit lifecycle claim.
  const publicTail = [...groups].reverse().find(group => group.seats.some(seat => seat.publicBlock))
  const processGroupCount = groups.filter(group => !group.status).length
  let privateGroupCount = 0, shownPrivateGroups = 0, shownGroups = 0
  const nodes = groups.map(group => {
    const grouped = !group.status && (group.unit || group.seats.length > 1 || group.seats[0]?.publicBlock?.kind === 'tool') && (policy.stepGrouping === 'collapsed' || policy.stepGrouping === 'history' && !processing)
    const open = openGroups.has(group.key), shown = visible && (!grouped || open)
    const publicOpen = open || publicGroups.has(group.key), publicShown = group.status === 'error' || publicVisible && (!grouped || publicOpen)
    if (shown && !group.status) shownGroups++
    if (!group.status && group.seats.some(seat => seat.privateIdentity)) {
      privateGroupCount++
      if (shown) shownPrivateGroups++
    }
    for (const seat of group.seats) if (shown && seat.privateIdentity) visiblePrivateIds.push(seat.privateIdentity.blockId)
    const activeTool = [...group.seats].reverse().find(seat => seat.publicBlock?.kind === 'tool' && seat.publicBlock.status === 'running'
      || seat.privateBlock && toolPreparationRowModel(seat.privateBlock))
    const live = activeTool?.publicBlock?.kind === 'tool' && activeTool.publicBlock.status === 'running' ? activeTool.publicBlock : undefined
    const preparation = activeTool?.privateBlock ? toolPreparationRowModel(activeTool.privateBlock) : null
    const liveReasoning = group.seats.map(seat => seat.privateBlock).reverse().find(block => block?.kind === 'native_reasoning'
      && reasoningSource(block.source) && block.state === 'running' && block.classification !== 'withheld')
    const publicBlocks = group.seats.flatMap(seat => seat.publicBlock ? [seat.publicBlock] : [])
    const counts = rankedProcessActivities(publicBlocks, processActivityFor)
    const hasNative = group.seats.some(seat => seat.privateIdentity && reasoningSource(seat.privateIdentity.source))
    const preparing = Boolean(preparation)
    const reasoningRunning = Boolean(liveReasoning)
    // Presentation closure suppresses only the header; leaf states and private visibility keep their real owners.
    const presentationClosed = turnSettled || group.presentationClosed === true
    const publicThinkingGap = processing && !presentationClosed && group === publicTail && !live && !preparing && !reasoningRunning
    const running = Boolean(!presentationClosed && processing && (live || preparing || reasoningRunning || publicThinkingGap))
    const activity = presentationClosed ? counts.at(0)?.kind ?? (hasNative ? 'thinking' : 'tools')
      : live ? processActivityFor(live) : preparing ? processActivityForName(preparation?.toolName)
      : reasoningRunning || publicThinkingGap ? 'thinking' : counts.at(0)?.kind ?? (hasNative ? 'thinking' : 'tools')
    const activityBlock = !presentationClosed && live ? live : publicBlocks.find(block => block.kind === 'tool' && processActivityFor(block) === activity)
    const family = activityBlock?.kind === 'tool' ? toolFamilyFor(activityBlock) : !presentationClosed && preparing ? toolFamilyForName(preparation?.toolName) : undefined
    const glyphActivity = activity
    const Icon = glyphActivity === 'plan' ? DshPlanOutlineRegular : glyphActivity === 'subagents' ? DshAgentPresetOutlineRegular
      : glyphActivity === 'questions' ? DshQuestionOutlineRegular : glyphActivity === 'webFetch' ? DshBrowseOutlineRegular
      : activity === 'thinking' ? DshThinkOutlineRegular : glyphActivity === 'code' ? DshCodeOutlineRegular
      : family ? ({ read: DshBrowseOutlineRegular, diff: DshEditOutlineRegular, shell: DshApiOutlineRegular, web: DshGlobeOutlineRegular, search: DshSearchOutlineRegular, plan: DshSparkleRegular, subagent: DshSparkleRegular, tool: DshSparkleRegular })[family] : DshSparkleRegular
    const detail = running && policy.liveProcessDetail && live ? describeProcessBlock(live, t) : undefined
    // Folded draft-only reads retain identity for reopening, not current state.
    // Without a fresh block, neither running nor completion may be inferred.
    const stalePrivateOnly = !counts.length && group.seats.some(seat => seat.privateIdentity && !seat.privateBlock)
    const settledTitle = stalePrivateOnly ? t('dshTurnProcess') : counts.length || hasNative ? processTitle(counts, t) : t('dshTurnProcess')
    const privateDetailBlock = preparing ? activeTool?.privateBlock : !live ? liveReasoning : undefined
    const privateDetailText = preparing ? preparation?.summary : privateDetailBlock?.preview
    // Private text is an immediate leaf, never the stable title hook's cached state.
    const protectedDetail = running && shown && policy.liveProcessDetail && privateDetailBlock
      && privateDetailBlock.classification !== 'withheld' && privateDetailText
      ? <span data-protected-process-block={privateDetailBlock.blockId}>{privateDetailText}</span> : undefined
    return <ChatGroupSeat key={group.key} groupKey={group.key} grouped={grouped} open={publicOpen} visible={publicShown} running={running}
      activity={activity} preparing={!presentationClosed && preparing} detail={detail} protectedDetail={protectedDetail}
      label={(kind, isPreparing) => t('dshStepProcess.' + (isPreparing ? 'preparing.' : 'live.') + kind)} settledTitle={settledTitle} icon={<Icon size={14} />} iconKey={glyphActivity + ':' + (family ?? '')} onToggle={() => toggle(group.key)} onReveal={() => reveal(group.key)}>
      {group.seats.map(seat => <ChatNodeSeat key={seat.key} seat={seat} treeOwnerId={treeOwnerId} anchorsInTree={blocks} processing={processing} policy={policy} local={local} visible={shown} publicVisible={publicShown}
        resetKey={resetKey} privateGeneration={privateGeneration} onInspect={onInspect} />)}
    </ChatGroupSeat>
  })
  if (visible && draftsVisible) visiblePrivateIds.push(...processDraftSeats(local).map(seat => seat.blockId))
  useLayoutEffect(() => { onVisibility?.({ kind: !visible ? 'whole_folded' : visiblePrivateIds.some(id => local?.identities.some(identity => identity.blockId === id && identity.source.kind === 'assistant_draft'))
    ? 'visible' : processGroupCount === 0 ? 'empty' : shownGroups === 0 ? 'all_groups_folded' : 'visible',
    visiblePrivateIds, explicitGroupOpen: groupOpenIntent.current, // A visible public status must not stop draft discovery while every known
    // private group is folded. Keep those groups on the draft-only Host projection.
    draftsOnly: visible && processGroupCount > 0 && (shownGroups === 0 || privateGroupCount > 0 && shownPrivateGroups === 0) }) })
  return <QuestionAssociationContext.Provider value={associated}>
    {visible && threadId && threadId === activeThreadId ? inputs.map(input => <QuestionAssociationLeaf key={threadId + ':' + input.id} input={input} threadId={threadId} anchors={blocks} onMatch={onAssociation} />) : null}
    <ProtectedToolInspectorSelection scopeKey={selectionScope.current.key}>{nodes}</ProtectedToolInspectorSelection>
  </QuestionAssociationContext.Provider>
}
