import type { KeyboardEvent as ReactKeyboardEvent, MouseEvent as ReactMouseEvent, ReactElement, RefObject } from 'react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { LucideIcon } from 'lucide-react'
import {
  Ban,
  Bot,
  BookOpen,
  ChevronDown,
  ChevronRight,
  CircleDot,
  FolderOpen,
  ListTodo,
  MessageSquareQuote,
  Minimize2,
  PencilLine,
  Search,
  Terminal,
  Wrench
} from 'lucide-react'
import type { ChatBlock, ToolBlock } from '../../agent/types'
import { redactSecretText } from '@shared/secret-redaction'
import { extractUnifiedDiffText } from '../../lib/diff-stats'
import { useDeferredRender } from '../../hooks/use-deferred-render'
import { openWorkspacePathInEditor } from '../../lib/open-workspace-path'
import {
  dispatchRuntimeDiagnosticsFocus,
  type RuntimeDiagnosticsStatusFilter
} from '../../lib/runtime-diagnostics-focus'
import { previewWorkspaceFile } from '../../lib/workspace-file-preview'
import { useChatStore } from '../../store/chat-store'
import { DiffView } from '../DiffView'
import { AssistantMarkdown } from './AssistantMarkdown'
import { MessageBubble } from './message-timeline-bubbles'
import { blockHasPendingRuntimeWork, splitThink } from './message-timeline-turns'
import {
  BackgroundShellGroup,
  SubagentGroup,
  childProcessGroupKey,
  childProcessIdentityKey,
  isBackgroundShellProcessBlock,
  isSubagentProcessBlock,
  shouldMergeProcessChildBlocks
} from './SubagentCallCard'
import { InjectedMemoryMetaChip } from './injected-memory-meta-chip'
import {
  formatChildAgentChip,
  formatBytes,
  formatDuration,
  formatJobDiagnosticsChip,
  formatJobHeartbeatStatus,
  formatProviderDiagnosticsChip,
  formatToolTitle
} from './message-timeline-tools'

export type ProcessSection = {
  id: string
  kind: 'execution' | 'output' | 'subagent' | 'background_shell'
  blocks: ChatBlock[]
}

export function groupProcessSections(blocks: ChatBlock[]): ProcessSection[] {
  const sections: ProcessSection[] = []
  const subagentSections = new Map<string, ProcessSection>()

  for (const block of blocks) {
    if (isBackgroundShellProcessBlock(block)) {
      const last = sections[sections.length - 1]
      const first = last?.blocks[0]
      if (
        last &&
        last.kind === 'background_shell' &&
        first &&
        shouldMergeProcessChildBlocks(first, block)
      ) {
        last.blocks.push(block)
        continue
      }
      sections.push({ id: `background-shell-${block.id}`, kind: 'background_shell', blocks: [block] })
      continue
    }
    if (isSubagentProcessBlock(block)) {
      const groupKey = childProcessGroupKey(block)
      const section = groupKey ? subagentSections.get(groupKey) : undefined
      if (section) {
        upsertProcessChildBlock(section, block)
        continue
      }
      const nextSection: ProcessSection = { id: `subagent-${block.id}`, kind: 'subagent', blocks: [block] }
      sections.push(nextSection)
      if (groupKey) subagentSections.set(groupKey, nextSection)
      continue
    }
    if (isAutoContinueProcessBlock(block)) {
      sections.push({ id: `execution-${block.id}`, kind: 'execution', blocks: [block] })
      continue
    }
    const kind = block.kind === 'assistant' ? 'output' : 'execution'
    const last = sections[sections.length - 1]
    if (last && last.kind === kind) {
      last.blocks.push(block)
      continue
    }
    sections.push({
      id: `${kind}-${block.id}`,
      kind,
      blocks: [block]
    })
  }

  return sections
}

function upsertProcessChildBlock(section: ProcessSection, block: ChatBlock): void {
  const key = childProcessIdentityKey(block)
  const index = section.blocks.findIndex((candidate) => childProcessIdentityKey(candidate) === key)
  if (index < 0) {
    section.blocks.push(block)
    return
  }
  section.blocks[index] = mergeProcessChildBlock(section.blocks[index], block)
}

function mergeProcessChildBlock(previous: ChatBlock, next: ChatBlock): ChatBlock {
  if (previous.kind !== 'tool' || next.kind !== 'tool') return next
  return {
    ...previous,
    ...next,
    summary: next.summary || previous.summary,
    detail: next.detail || previous.detail,
    meta: {
      ...previous.meta,
      ...next.meta,
      child: {
        ...(
          previous.meta?.child && typeof previous.meta.child === 'object' && !Array.isArray(previous.meta.child)
            ? previous.meta.child
            : {}
        ),
        ...(
          next.meta?.child && typeof next.meta.child === 'object' && !Array.isArray(next.meta.child)
            ? next.meta.child
            : {}
        )
      }
    }
  }
}

function sectionHasDetails(
  section: ProcessSection,
  t: (key: string, opts?: Record<string, unknown>) => string
): boolean {
  if (section.kind === 'output') {
    return section.blocks.some(
      (block) => getProcessDetail(block, describeProcessBlock(block, t)).kind === 'assistant'
    )
  }
  if (section.blocks.length > 1) return true
  const [block] = section.blocks
  return block ? getProcessDetail(block, describeProcessBlock(block, t)).kind !== 'none' : false
}

function isProcessSectionActive(section: ProcessSection, processing: boolean): boolean {
  if (!processing) return false
  if (section.kind === 'output') {
    return section.blocks.some((block) => block.id === 'live-assistant')
  }
  return section.blocks.some(
    (block) => block.id === 'live-assistant' || blockHasPendingRuntimeWork(block)
  )
}

function isRequestUserInputTool(block: ChatBlock): boolean {
  if (block.kind === 'user_input' && block.status === 'pending') return true
  if (block.kind !== 'tool' || block.status !== 'running') return false
  const toolName = typeof block.meta?.toolName === 'string' ? block.meta.toolName.trim() : ''
  if (toolName === 'request_user_input' || toolName === 'user_input') return true
  return /^request_user_input\s*:/i.test(block.summary.trim())
}

type ProcessErrorTone = 'tool' | 'error' | null

function processBlockErrorTone(block: ChatBlock): ProcessErrorTone {
  if (block.kind === 'tool' && block.status === 'error') return 'tool'
  if (block.kind === 'compaction' && block.status === 'error') return 'error'
  if (block.kind === 'approval' && block.status === 'error') return 'error'
  if (block.kind === 'user_input' && block.status === 'error') return 'error'
  if (block.kind === 'system' && block.severity === 'error') return 'error'
  return null
}

function processSectionErrorTone(blocks: ChatBlock[]): ProcessErrorTone {
  let fallback: ProcessErrorTone = null
  for (const block of blocks) {
    const tone = processBlockErrorTone(block)
    if (tone === 'error') return tone
    if (tone === 'tool') fallback = tone
  }
  return fallback
}

function processErrorTextClass(tone: ProcessErrorTone): string {
  if (tone === 'tool') return 'text-orange-700 dark:text-orange-300'
  if (tone === 'error') return 'text-red-600 dark:text-red-300'
  return 'text-ds-muted'
}

function processErrorDotClass(tone: ProcessErrorTone): string {
  if (tone === 'tool') return 'bg-orange-500 dark:bg-orange-300'
  if (tone === 'error') return 'bg-red-500 dark:bg-red-300'
  return ''
}

function sectionHasRequestUserInput(section: ProcessSection): boolean {
  return section.blocks.some(isRequestUserInputTool)
}

function isPendingApproval(block: ChatBlock): boolean {
  return block.kind === 'approval' && block.status === 'pending'
}

function sectionHasPendingApproval(section: ProcessSection): boolean {
  return section.blocks.some(isPendingApproval)
}

export function ProcessSectionRow({
  section,
  processing,
  viewportRef
}: {
  section: ProcessSection
  processing: boolean
  viewportRef: RefObject<HTMLDivElement | null>
}): ReactElement {
  const { t } = useTranslation('common')
  const [userExpanded, setUserExpanded] = useState<boolean | null>(null)
  const assistantBlocks =
    section.kind === 'output'
      ? section.blocks.filter(
          (block): block is Extract<ChatBlock, { kind: 'assistant' }> => block.kind === 'assistant'
        )
      : []
  const hasDetails = sectionHasDetails(section, t)
  const active = isProcessSectionActive(section, processing)
  const errorTone = processSectionErrorTone(section.blocks)
  const hasError = errorTone !== null
  const defaultExpanded =
    (processing && hasError) ||
    sectionHasPendingApproval(section) ||
    (processing && section.kind === 'execution' && sectionHasRequestUserInput(section))
  const forceExpanded = sectionHasPendingApproval(section)
  const expanded = hasDetails && (forceExpanded || (userExpanded ?? defaultExpanded))
  const title = describeProcessSection(section, t)
  const SectionIcon = processSectionIcon(section)
  const canToggleSection = hasDetails && !forceExpanded
  const showActiveError = active && hasError
  const shouldDeferDetails = section.kind !== 'subagent'
  const { ref: deferredDetailRef, shouldRender: shouldRenderDetail } = useDeferredRender<HTMLDivElement>({
    enabled: shouldDeferDetails && expanded,
    immediate: shouldDeferDetails && (active || section.kind === 'execution'),
    root: viewportRef
  })

  if (section.kind === 'subagent') {
    return <SubagentGroup blocks={section.blocks} />
  }

  if (section.kind === 'background_shell') {
    return <BackgroundShellGroup blocks={section.blocks} />
  }

  if (section.kind === 'execution' && section.blocks.length === 1) {
    const [block] = section.blocks
    if (block) {
      return <ProcessEntryRow block={block} processing={processing} />
    }
  }

  if (section.kind === 'output') {
    return hasDetails ? (
      <div className="min-w-0">
        <div className="flex flex-col gap-2">
          {assistantBlocks.map((block) => (
            <ProcessEntryDetail
              key={block.id}
              block={block}
              detail={getProcessDetail(block)}
              processing={processing}
            />
          ))}
        </div>
      </div>
    ) : (
      <></>
    )
  }

  return (
    <div className="flex flex-col">
      {canToggleSection ? (
        <button
          type="button"
          onClick={() => setUserExpanded(!(userExpanded ?? defaultExpanded))}
          className={`group flex w-fit max-w-full items-center gap-1.5 rounded-md py-0.5 text-left text-[14px] font-medium transition hover:opacity-85 ${
            hasError ? processErrorTextClass(errorTone) : 'text-ds-muted'
          }`}
        >
          {showActiveError ? (
            <span className="ds-work-logo-slot ds-work-logo-slot-sm mr-0.5">
              <span className={`h-2 w-2 rounded-full ${processErrorDotClass(errorTone)}`} />
            </span>
          ) : null}
          {SectionIcon ? <ProcessGlyph Icon={SectionIcon} /> : null}
          <span className={active && !hasError ? 'ds-shiny-text' : ''}>{title}</span>
          {expanded ? (
            <ChevronDown className="h-3.5 w-3.5 shrink-0 opacity-45" strokeWidth={1.8} />
          ) : (
            <ChevronRight className="h-3.5 w-3.5 shrink-0 opacity-0 transition group-hover:opacity-55" strokeWidth={1.8} />
          )}
        </button>
      ) : (
        <div
          className={`flex w-fit max-w-full items-center gap-1.5 py-0.5 text-[14px] font-medium ${
            hasError ? processErrorTextClass(errorTone) : 'text-ds-muted'
          }`}
        >
          {showActiveError ? (
            <span className="ds-work-logo-slot ds-work-logo-slot-sm mr-0.5">
              <span className={`h-2 w-2 rounded-full ${processErrorDotClass(errorTone)}`} />
            </span>
          ) : null}
          {SectionIcon ? <ProcessGlyph Icon={SectionIcon} /> : null}
          <span className={active && !hasError ? 'ds-shiny-text' : ''}>{title}</span>
        </div>
      )}

      {expanded ? (
        <div
          ref={deferredDetailRef}
          className="mt-1"
          style={{ contentVisibility: 'auto', containIntrinsicSize: 'auto 220px' }}
        >
          {shouldRenderDetail ? (
            <ProcessStackRows blocks={section.blocks} processing={processing} />
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

export function CompactionStatusRow({
  block
}: {
  block: Extract<ChatBlock, { kind: 'compaction' }>
}): ReactElement {
  const { t } = useTranslation('common')
  const isError = block.status === 'error'
  const isRunning = block.status === 'running'
  const summary = describeProcessBlock(block, t)

  return (
    <div
      className={`flex w-fit max-w-full items-center gap-1.5 py-0.5 text-[13px] ${
        isError ? 'text-red-600 dark:text-red-300' : 'text-ds-faint'
      }`}
    >
      <Minimize2 className="h-3 w-3 shrink-0 opacity-70" strokeWidth={2} />
      <span className={`min-w-0 truncate ${isRunning ? 'ds-shiny-text' : ''}`}>{summary}</span>
    </div>
  )
}

function processBlockIsRunningTool(block: ChatBlock, processing: boolean): boolean {
  return processing && block.kind === 'tool' && block.status === 'running'
}

function processBlockIsAutoOpenPending(block: ChatBlock, processing: boolean): boolean {
  return (
    processing &&
    ((block.kind === 'compaction' && block.status === 'running') ||
      (block.kind === 'approval' && block.status === 'pending') ||
      (block.kind === 'user_input' && block.status === 'pending'))
  )
}

function processBlockIsActive(block: ChatBlock, processing: boolean): boolean {
  return (
    processBlockIsRunningTool(block, processing) ||
    processBlockIsAutoOpenPending(block, processing) ||
    (processing && block.kind === 'assistant' && block.id === 'live-assistant')
  )
}

function processBlockHasError(block: ChatBlock): boolean {
  return processBlockErrorTone(block) !== null
}

function processBlockHasRuntimeMetaDetail(block: ChatBlock): boolean {
  if (block.kind !== 'tool' && block.kind !== 'system') return false
  return runtimeMetaDetailSections(block, undefined).length > 0
}

function processBlockHasAttentionRuntimeMetaDetail(block: ChatBlock): boolean {
  if (block.kind !== 'tool' && block.kind !== 'system') return false
  const diagnostics = jobDiagnosticsFromMeta(block.meta)
  if (!diagnostics) return false
  const statusText = [
    readRuntimeString(diagnostics, 'status'),
    readRuntimeString(diagnostics, 'heartbeatStatus'),
    readRuntimeString(diagnostics, 'recoveryStatus'),
    readRuntimeString(diagnostics, 'deliveryStatus'),
    readRuntimeString(diagnostics, 'autoContinueStatus'),
    readRuntimeString(diagnostics, 'warningCode')
  ].join(' ')
  return (
    /error|failed|stale|lease_expired|orphaned|recovering|dead_letter|dead_lettered|retry|superseded/i.test(statusText) ||
    readRuntimeBoolean(diagnostics, 'stalled') ||
    readRuntimeBoolean(diagnostics, 'leaseExpired') ||
    readRuntimeBoolean(diagnostics, 'orphaned') ||
    readRuntimeBoolean(diagnostics, 'lateCompletionSuppressed')
  )
}

function ProcessStackRows({
  blocks,
  processing
}: {
  blocks: ChatBlock[]
  processing: boolean
}): ReactElement {
  const { t } = useTranslation('common')
  const [openBlockId, setOpenBlockId] = useState<string | null>(null)
  const [closedBlockIds, setClosedBlockIds] = useState<ReadonlySet<string>>(() => new Set())

  return (
    <div className="ds-work-stack">
      {blocks.map((block) => {
        const summary = describeProcessBlock(block, t)
        const detail = getProcessDetail(block, summary)
        const isRunningTool = processBlockIsRunningTool(block, processing)
        const canExpand = detail.kind !== 'none'
        const autoOpenRequestInput = processing && isRequestUserInputTool(block)
        const autoOpenPending = processBlockIsAutoOpenPending(block, processing) || isPendingApproval(block)
        const errorTone = processBlockErrorTone(block)
        const isError = errorTone !== null
        const defaultOpen =
          (isError && (processing || errorTone !== 'tool')) ||
          (!isError && processBlockHasAttentionRuntimeMetaDetail(block) && !isQuietRuntimeNotificationProcessBlock(block))
        const forceOpen = autoOpenPending || autoOpenRequestInput
        const userClosed = closedBlockIds.has(block.id)
        const userOpened = openBlockId === block.id
        const open = canExpand && (forceOpen || userOpened || (defaultOpen && !userClosed))
        const rowActive = processBlockIsActive(block, processing)
        const canToggle = canExpand && !forceOpen
        const RowIcon = processBlockIcon(block)
        const handleToggle = (): void => {
          if (!canToggle) return
          if (open) {
            setOpenBlockId((id) => (id === block.id ? null : id))
            if (defaultOpen) {
              setClosedBlockIds((ids) => {
                const next = new Set(ids)
                next.add(block.id)
                return next
              })
            }
            return
          }
          setClosedBlockIds((ids) => {
            if (!ids.has(block.id)) return ids
            const next = new Set(ids)
            next.delete(block.id)
            return next
          })
          setOpenBlockId(block.id)
        }
        const handleToggleButton = (event: ReactMouseEvent<HTMLButtonElement>): void => {
          event.stopPropagation()
          handleToggle()
        }
        const handleKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>): void => {
          if (!canToggle) return
          if (event.key !== 'Enter' && event.key !== ' ') return
          event.preventDefault()
          handleToggle()
        }

        return (
          <div
            key={block.id}
            className="min-w-0"
            data-analytix-process-block-id={block.id}
            data-analytix-process-block-kind={block.kind}
            data-analytix-process-block-status={'status' in block ? block.status : undefined}
          >
            <div
              role={canToggle ? 'button' : undefined}
              tabIndex={canToggle ? 0 : undefined}
              aria-expanded={canToggle ? open : undefined}
              onClick={handleToggle}
              onKeyDown={handleKeyDown}
              className={`group flex w-full min-w-0 items-center gap-1.5 rounded-md px-1 py-0.5 text-left text-[13.5px] leading-6 transition ${
                isError
                  ? processErrorTextClass(errorTone)
                  : 'text-ds-faint hover:text-ds-muted'
              } ${canToggle ? 'cursor-pointer hover:bg-ds-hover/45' : 'cursor-default'}`}
            >
              {RowIcon ? <ProcessGlyph Icon={RowIcon} /> : null}
              <span className={`min-w-0 flex-1 truncate ${rowActive && !isError ? 'ds-shiny-text' : ''}`}>
                <ProcessSummaryText block={block} summary={summary} />
              </span>
              <RuntimeDiagnosticsLink block={block} t={t} />
              {canExpand ? (
                <button
                  type="button"
                  aria-label={open ? t('processCollapseDetail') : t('processExpandDetail')}
                  aria-expanded={open}
                  disabled={!canToggle}
                  onClick={handleToggleButton}
                  className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-md transition ${
                    canToggle ? 'cursor-pointer hover:bg-ds-hover/70' : 'cursor-default'
                  }`}
                >
                  {open ? (
                    <ChevronDown className="h-3 w-3 opacity-45" strokeWidth={2} />
                  ) : (
                    <ChevronRight className="h-3 w-3 opacity-45" strokeWidth={2} />
                  )}
                </button>
              ) : null}
            </div>
            {open ? (
              detail.kind === 'assistant' ? (
                <div className="ml-1 mt-1">
                  <ProcessEntryDetail block={block} detail={detail} processing={processing} />
                </div>
              ) : (
                <div className="ds-work-timeline-detail ml-1">
                  <ProcessEntryDetail block={block} detail={detail} processing={processing} />
                </div>
              )
            ) : null}
          </div>
        )
      })}
    </div>
  )
}

/** One line inside an execution section. */
function ProcessEntryRow({
  block,
  processing
}: {
  block: ChatBlock
  processing: boolean
}): ReactElement {
  const { t } = useTranslation('common')
  const [userOpen, setUserOpen] = useState<boolean | null>(null)
  const summary = describeProcessBlock(block, t)
  const detail = getProcessDetail(block, summary)
  const canExpand = detail.kind !== 'none'
  const isAssistantProcessText = block.kind === 'assistant'
  const isRunningTool = processBlockIsRunningTool(block, processing)
  const isAutoOpenPending = processBlockIsAutoOpenPending(block, processing) || isPendingApproval(block)
  const isStreamingAssistant = processing && block.kind === 'assistant' && block.id === 'live-assistant'
  const errorTone = processBlockErrorTone(block)
  const isError = errorTone !== null
  const forceOpen = isAutoOpenPending || isAssistantProcessText || isStreamingAssistant
  const defaultOpen =
    (isError && (processing || errorTone !== 'tool')) ||
    (!isError && processBlockHasAttentionRuntimeMetaDetail(block) && !isQuietRuntimeNotificationProcessBlock(block))
  const open =
    canExpand &&
    (forceOpen || (userOpen ?? defaultOpen))

  const { verb, rest } = splitVerb(summary)
  const rowActive = isRunningTool || isAutoOpenPending || isStreamingAssistant
  const wrapSummary = (block.kind === 'system' && !canExpand) || isAssistantProcessText
  const canToggle = canExpand && !forceOpen
  const RowIcon = processBlockIcon(block)
  const handleToggle = (): void => {
    if (!canToggle) return
    setUserOpen(!open)
  }
  const handleToggleButton = (event: ReactMouseEvent<HTMLButtonElement>): void => {
    event.stopPropagation()
    handleToggle()
  }
  const handleKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>): void => {
    if (!canToggle) return
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    handleToggle()
  }

  return (
    <div
      className="flex flex-col"
      data-analytix-process-block-id={block.id}
      data-analytix-process-block-kind={block.kind}
      data-analytix-process-block-status={'status' in block ? block.status : undefined}
    >
      <div
        role={canToggle ? 'button' : undefined}
        tabIndex={canToggle ? 0 : undefined}
        aria-expanded={canToggle ? open : undefined}
        onClick={handleToggle}
        onKeyDown={handleKeyDown}
        className={`group flex w-full items-start gap-2 rounded-md px-2 py-1 text-left text-[13.5px] leading-[1.55] transition ${
          isError
            ? processErrorTextClass(errorTone)
            : 'text-ds-faint hover:text-ds-ink'
        } ${
          canToggle
            ? 'cursor-pointer hover:bg-ds-hover/70'
            : 'cursor-default'
        }`}
      >
        {RowIcon ? <ProcessGlyph Icon={RowIcon} className="mt-1" /> : null}
        <span
          className={`min-w-0 flex-1 ${wrapSummary ? 'whitespace-pre-wrap break-words' : 'truncate'} ${
            rowActive && !isError ? 'ds-shiny-text' : ''
          }`}
        >
          <span
            className={`font-medium ${isError ? '' : rowActive ? '' : 'text-ds-muted'}`}
          >
            {verb}
          </span>
          {rest ? (
            <span className="ml-1.5 font-mono text-[13px]">
              <ProcessSummaryText block={block} summary={rest} />
            </span>
          ) : null}
        </span>
        <RuntimeDiagnosticsLink block={block} t={t} />
        {canExpand ? (
          <button
            type="button"
            aria-label={open ? t('processCollapseDetail') : t('processExpandDetail')}
            aria-expanded={open}
            disabled={!canToggle}
            onClick={handleToggleButton}
            className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md transition ${
              canToggle ? 'cursor-pointer hover:bg-ds-hover/70' : 'cursor-default'
            }`}
          >
            {open ? (
              <ChevronDown className="h-3 w-3 opacity-45" strokeWidth={2} />
            ) : (
              <ChevronRight className="h-3 w-3 opacity-45" strokeWidth={2} />
            )}
          </button>
        ) : null}
      </div>
      <RuntimeMetaBadges block={block} t={t} />
      {canExpand && open ? (
        detail.kind === 'assistant' ? (
          <div className="mt-1">
            <ProcessEntryDetail block={block} detail={detail} processing={processing} />
          </div>
        ) : (
          <div className="ds-work-timeline-detail">
            <ProcessEntryDetail block={block} detail={detail} processing={processing} />
          </div>
        )
      ) : null}
    </div>
  )
}

function ProcessGlyph({
  Icon,
  className = 'mt-0.5'
}: {
  Icon: LucideIcon
  className?: string
}): ReactElement {
  return <Icon className={`${className} h-3.5 w-3.5 shrink-0 opacity-75`} strokeWidth={1.9} />
}

function RuntimeDiagnosticsLink({
  block,
  t
}: {
  block: ChatBlock
  t: (key: string, opts?: Record<string, unknown>) => string
}): ReactElement | null {
  const diagnostics = block.kind === 'tool' || block.kind === 'system'
    ? jobDiagnosticsFromMeta(block.meta)
    : null
  if (!diagnostics || !hasRuntimeDiagnosticsLinkSignal(diagnostics)) return null
  const label = t('openRuntimeDiagnostics')
  return (
    <button
      type="button"
      className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint opacity-70 transition hover:bg-ds-hover/70 hover:text-ds-ink hover:opacity-100"
      aria-label={label}
      title={label}
      onClick={(event) => {
        event.preventDefault()
        event.stopPropagation()
        dispatchRuntimeDiagnosticsFocus({
          jobId: readRuntimeString(diagnostics, 'jobId') || readRuntimeString(diagnostics, 'childRunId'),
          deliveryId: readRuntimeString(diagnostics, 'deliveryId'),
          childThreadId: readRuntimeString(diagnostics, 'childThreadId'),
          childTurnId: readRuntimeString(diagnostics, 'childTurnId'),
          parentThreadId: readRuntimeString(diagnostics, 'parentThreadId'),
          parentTurnId: readRuntimeString(diagnostics, 'parentTurnId'),
          sourceBlockId: block.id,
          status: runtimeDiagnosticsStatusForFocus(diagnostics)
        })
      }}
    >
      <CircleDot className="h-3.5 w-3.5" strokeWidth={1.9} />
    </button>
  )
}

function hasRuntimeDiagnosticsLinkSignal(diagnostics: Record<string, unknown>): boolean {
  return Boolean(
    readRuntimeString(diagnostics, 'jobId') ||
    readRuntimeString(diagnostics, 'childRunId') ||
    readRuntimeString(diagnostics, 'deliveryId') ||
    readRuntimeString(diagnostics, 'childThreadId') ||
    readRuntimeString(diagnostics, 'parentThreadId') ||
    readRuntimeString(diagnostics, 'heartbeatStatus') ||
    readRuntimeString(diagnostics, 'recoveryStatus') ||
    readRuntimeString(diagnostics, 'autoContinueStatus') ||
    readRuntimeString(diagnostics, 'deliveryStatus')
  )
}

function runtimeDiagnosticsStatusForFocus(
  diagnostics: Record<string, unknown>
): RuntimeDiagnosticsStatusFilter | undefined {
  const notificationKind = readRuntimeString(diagnostics, 'notificationKind')
  const autoStatus = readRuntimeString(diagnostics, 'autoContinueStatus')
  if (notificationKind === 'background_job_auto_continue' && autoStatus) {
    if (autoStatus === 'started') return 'running'
    if (autoStatus === 'skipped') return 'skipped'
    if (autoStatus === 'failed') return 'dead_lettered'
  }
  const deliveryStatus = readRuntimeString(diagnostics, 'deliveryStatus')
  if (deliveryStatus === 'pending') return 'delivery_pending'
  if (deliveryStatus === 'retry') return 'retrying'
  if (deliveryStatus === 'delivered') return 'delivered'
  if (deliveryStatus === 'skipped') return 'skipped'
  if (deliveryStatus === 'dead_letter' || deliveryStatus === 'dead_lettered') return 'dead_letter'
  const heartbeatStatus = readRuntimeString(diagnostics, 'heartbeatStatus')
  if (
    heartbeatStatus === 'running' ||
    heartbeatStatus === 'stale' ||
    heartbeatStatus === 'lease_expired' ||
    heartbeatStatus === 'orphaned' ||
    heartbeatStatus === 'recovering' ||
    heartbeatStatus === 'recovered' ||
    heartbeatStatus === 'dead_lettered' ||
    heartbeatStatus === 'paused'
  ) {
    return heartbeatStatus
  }
  const recoveryStatus = readRuntimeString(diagnostics, 'recoveryStatus')
  if (recoveryStatus === 'recovering' || recoveryStatus === 'recovered' || recoveryStatus === 'dead_lettered') {
    return recoveryStatus
  }
  if (readRuntimeBoolean(diagnostics, 'leaseExpired')) return 'lease_expired'
  if (readRuntimeBoolean(diagnostics, 'orphaned')) return 'orphaned'
  if (readRuntimeBoolean(diagnostics, 'stalled')) return 'stale'
  return undefined
}

function describeProcessSection(
  section: ProcessSection,
  t: (key: string, opts?: Record<string, unknown>) => string
): string {
  if (section.kind === 'output') {
    return t('processTextLabel')
  }

  if (section.blocks.length === 1) {
    return describeProcessBlock(section.blocks[0], t)
  }

  return summarizeExecutionSection(section.blocks, t)
}

const WORKSPACE_EXPLORATION_TOOLS = new Set([
  'read',
  'read_file',
  'grep',
  'grep_files',
  'search_files',
  'find',
  'glob',
  'ls',
  'list_files'
])

function executionToolName(block: ToolBlock): string {
  const rawSummary = block.summary?.trim() ?? ''
  return extractToolName(rawSummary) || readMetaString(block.meta, 'toolName') || ''
}

function isWorkspaceExplorationTool(block: ToolBlock): boolean {
  if (block.toolKind) return false
  const toolName = executionToolName(block).toLowerCase()
  return WORKSPACE_EXPLORATION_TOOLS.has(toolName)
}

function summarizeExecutionSection(
  blocks: ChatBlock[],
  t: (key: string, opts?: Record<string, unknown>) => string
): string {
  let fileCount = 0
  let commandCount = 0
  let childAgentCount = 0
  let explorationToolCount = 0
  let toolCount = 0
  let approvalCount = 0

  for (const block of blocks) {
    if (block.kind === 'approval') {
      approvalCount += 1
      continue
    }
    if (block.kind !== 'tool') continue
    if (block.toolKind === 'file_change') {
      fileCount += 1
    } else if (block.toolKind === 'command_execution') {
      commandCount += 1
    } else if (block.toolKind === 'subagent') {
      childAgentCount += 1
    } else if (isWorkspaceExplorationTool(block)) {
      explorationToolCount += 1
    } else {
      toolCount += 1
    }
  }

  const parts: string[] = []
  if (fileCount > 0) {
    parts.push(
      fileCount === 1 ? t('groupEditedFile') : t('groupEditedFiles', { count: fileCount })
    )
  }
  if (commandCount > 0) {
    parts.push(
      commandCount === 1
        ? t('groupRanCommand')
        : t('groupRanCommands', { count: commandCount })
    )
  }
  if (childAgentCount > 0) {
    parts.push(
      childAgentCount === 1
        ? t('groupChildAgent')
        : t('groupChildAgents', { count: childAgentCount })
    )
  }
  if (explorationToolCount > 0) {
    parts.push(
      explorationToolCount === 1
        ? t('groupExploredWorkspace')
        : t('groupExploredWorkspaceWithTools', { count: explorationToolCount })
    )
  }
  if (toolCount > 0) {
    parts.push(toolCount === 1 ? t('groupUsedTool') : t('groupUsedTools', { count: toolCount }))
  }
  if (approvalCount > 0) {
    parts.push(
      approvalCount === 1 ? t('groupApproval') : t('groupApprovals', { count: approvalCount })
    )
  }

  if (parts.length > 0) return parts.join(' · ')
  return t('processSteps', { count: blocks.length })
}

function processSectionIcon(section: ProcessSection): LucideIcon | null {
  if (section.kind === 'output') return MessageSquareQuote

  const toolIcons = section.blocks
    .map(processBlockIcon)
    .filter((icon): icon is LucideIcon => icon !== null)
  if (toolIcons.length === 0) return null
  const [first] = toolIcons
  return toolIcons.every((icon) => icon === first) ? first : Wrench
}

function processBlockIcon(block: ChatBlock): LucideIcon | null {
  if (block.kind === 'assistant') return MessageSquareQuote
  if (block.kind === 'compaction') return Minimize2
  if (block.kind === 'approval') return Wrench
  if (block.kind === 'user_input') return MessageSquareQuote
  if (isBackgroundShellProcessBlock(block)) return Terminal
  if (block.kind !== 'tool') return null
  if (isAutoContinueProcessBlock(block)) {
    const diagnostics = jobDiagnosticsFromMeta(block.meta)
    const status = readRuntimeString(diagnostics, 'autoContinueStatus') || readRuntimeString(diagnostics, 'status')
    return status === 'skipped' || status === 'failed' ? Ban : Bot
  }
  if (isDeliveryLedgerProcessBlock(block)) {
    const diagnostics = jobDiagnosticsFromMeta(block.meta)
    const status = readRuntimeString(diagnostics, 'deliveryStatus') || readRuntimeString(diagnostics, 'status')
    return status === 'skipped' || status === 'dead_letter' || status === 'dead_lettered' ? Ban : Bot
  }
  return toolBlockIcon(block)
}

function toolBlockIcon(block: ToolBlock): LucideIcon {
  const toolName = executionToolName(block).toLowerCase()
  switch (toolName) {
    case 'bash':
    case 'shell':
    case 'terminal':
    case 'run_command':
    case 'exec':
      return Terminal
    case 'read':
    case 'read_file':
      return BookOpen
    case 'write':
    case 'write_file':
    case 'edit':
    case 'edit_file':
    case 'apply_patch':
    case 'create_file':
      return PencilLine
    case 'grep':
    case 'grep_files':
    case 'search':
    case 'search_files':
    case 'find':
      return Search
    case 'ls':
    case 'list':
    case 'list_dir':
      return FolderOpen
    case 'create_plan':
    case 'update_plan':
      return ListTodo
    default:
      break
  }

  if (block.toolKind === 'command_execution') return Terminal
  if (block.toolKind === 'file_change') return PencilLine
  return Wrench
}

function splitVerb(summary: string): { verb: string; rest: string } {
  const trimmed = summary.trim()
  if (!trimmed) return { verb: '', rest: '' }
  const space = trimmed.search(/\s/)
  if (space < 0) return { verb: trimmed, rest: '' }
  return { verb: trimmed.slice(0, space), rest: trimmed.slice(space + 1).trim() }
}

function toolFilePath(block: ToolBlock): string | undefined {
  const sourceText = [block.summary, block.detail ?? ''].filter(Boolean).join('\n')
  return (
    block.filePath ||
    extractQuotedField(sourceText, 'path') ||
    extractQuotedField(sourceText, 'file_path') ||
    extractQuotedField(sourceText, 'file')
  )
}

function ProcessFileReference({
  path,
  children
}: {
  path: string
  children: string
}): ReactElement {
  const { t } = useTranslation('common')
  const workspaceRoot = useChatStore((s) => s.workspaceRoot)

  const stopRowToggle = (event: ReactMouseEvent<HTMLElement>): void => {
    event.stopPropagation()
  }

  const preview = (event: ReactMouseEvent<HTMLButtonElement>): void => {
    event.preventDefault()
    event.stopPropagation()
    previewWorkspaceFile({ path, workspaceRoot })
  }

  const openInEditor = (event: ReactMouseEvent<HTMLButtonElement>): void => {
    event.preventDefault()
    event.stopPropagation()
    void openWorkspacePathInEditor({ path }, workspaceRoot).then((result) => {
      if (!result.ok) {
        void window.analytix?.logs?.error?.('editor-open', 'Failed to open process file reference', {
          message: result.message,
          target: { path, workspaceRoot }
        })?.catch(() => undefined)
      }
    })
  }

  return (
    <button
      type="button"
      className="ds-process-file-reference"
      title={t('processFileReferenceHint')}
      onClick={preview}
      onDoubleClick={openInEditor}
      onMouseDown={stopRowToggle}
    >
      {children}
    </button>
  )
}

function ProcessSummaryText({
  block,
  summary
}: {
  block: ChatBlock
  summary: string
}): ReactElement {
  if (block.kind !== 'tool') return <>{summary}</>
  const path = toolFilePath(block)
  if (!path) return <>{summary}</>
  const index = summary.indexOf(path)
  if (index < 0) return <>{summary}</>
  const before = summary.slice(0, index)
  const after = summary.slice(index + path.length)
  return (
    <>
      {before}
      <ProcessFileReference path={path}>{path}</ProcessFileReference>
      {after}
    </>
  )
}

type ProcessDetail =
  | { kind: 'none' }
  | { kind: 'assistant'; text: string }
  | { kind: 'tool'; text: string; isPatch: boolean; isError: boolean; filePath?: string }
  | { kind: 'runtime_meta' }
  | { kind: 'approval' }
  | { kind: 'user_input' }
  | { kind: 'text'; text: string }

type RuntimeMetaDetailSection = {
  id: string
  title: string
  rows: Array<{ label: string; value: string }>
}

function summarizeProcessText(text: string, max = 96): string {
  const oneLine = text.replace(/\s+/g, ' ').trim()
  if (!oneLine) return ''
  if (oneLine.length <= max) return oneLine
  return `${oneLine.slice(0, max - 1).trimEnd()}…`
}

function humanizeToolName(name: string): string {
  const trimmed = name.trim().replace(/[_-]+/g, ' ')
  if (!trimmed) return ''
  return trimmed.charAt(0).toUpperCase() + trimmed.slice(1)
}

function builtInToolLabel(
  toolName: string,
  t: (key: string, opts?: Record<string, unknown>) => string
): string | undefined {
  switch (toolName) {
    case 'read':
    case 'read_file':
      return t('toolBuiltinRead')
    case 'write':
    case 'write_file':
      return t('toolBuiltinWrite')
    case 'edit':
    case 'edit_file':
    case 'multi_edit':
      return t('toolBuiltinEdit')
    case 'grep':
    case 'grep_files':
    case 'search_files':
      return t('toolBuiltinGrep')
    case 'find':
      return t('toolBuiltinFind')
    case 'ls':
      return t('toolBuiltinLs')
    case 'bash':
    case 'shell':
      return t('toolBuiltinBash')
    default:
      return undefined
  }
}

function extractToolName(summary: string): string {
  const match = summary.trim().match(/^([a-z0-9_-]+)\s*:/i)
  return match?.[1] ?? ''
}

function extractQuotedField(text: string, field: string): string | undefined {
  const escaped = field.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const attr = new RegExp(`${escaped}="([^"]+)"`, 'i').exec(text)
  if (attr?.[1]) return attr[1]
  const json = new RegExp(`"${escaped}"\\s*:\\s*"([^"]+)"`, 'i').exec(text)
  if (json?.[1]) return json[1]
  return undefined
}

function readMetaString(meta: Record<string, unknown> | undefined, key: string): string | undefined {
  if (!meta) return undefined
  const value = meta[key]
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function readMetaStringArray(meta: Record<string, unknown> | undefined, key: string): string[] {
  const value = meta?.[key]
  if (!Array.isArray(value)) return []
  return value.filter((entry): entry is string => typeof entry === 'string' && entry.trim().length > 0)
}

function readMetaSources(meta: Record<string, unknown> | undefined): Array<{ title?: string; url?: string }> {
  const value = meta?.sources
  if (!Array.isArray(value)) return []
  return value
    .map((entry) => {
      if (!entry || typeof entry !== 'object') return null
      const raw = entry as Record<string, unknown>
      const title = typeof raw.title === 'string' && raw.title.trim() ? raw.title.trim() : undefined
      const url = typeof raw.url === 'string' && raw.url.trim() ? raw.url.trim() : undefined
      return title || url ? { ...(title ? { title } : {}), ...(url ? { url } : {}) } : null
    })
    .filter((entry): entry is { title?: string; url?: string } => entry !== null)
}

function readRuntimeRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}

function readRuntimeString(record: Record<string, unknown> | null, key: string): string {
  const value = record?.[key]
  return typeof value === 'string' && value.trim()
    ? redactSecretText(value.trim())
    : ''
}

function readRuntimeNumber(record: Record<string, unknown> | null, key: string): number | undefined {
  const value = record?.[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function readRuntimeBoolean(record: Record<string, unknown> | null, key: string): boolean {
  return record?.[key] === true
}

function jobDiagnosticsFromMeta(meta: Record<string, unknown> | undefined): Record<string, unknown> | null {
  return readRuntimeRecord(meta?.diagnostics)
}

function jobNotificationKind(block: ChatBlock): string {
  if (block.kind !== 'tool' && block.kind !== 'system') return ''
  return readRuntimeString(jobDiagnosticsFromMeta(block.meta), 'notificationKind')
}

function isAutoContinueProcessBlock(block: ChatBlock): boolean {
  return jobNotificationKind(block) === 'background_job_auto_continue'
}

function isDeliveryLedgerProcessBlock(block: ChatBlock): boolean {
  return jobNotificationKind(block) === 'background_job_delivery'
}

function isQuietRuntimeNotificationProcessBlock(block: ChatBlock): boolean {
  return isAutoContinueProcessBlock(block) || isDeliveryLedgerProcessBlock(block)
}

function formatRuntimePercent(value: number | undefined): string {
  if (value === undefined) return ''
  const normalized = value <= 1 ? value * 100 : value
  return `${Math.max(0, Math.min(100, Math.round(normalized)))}%`
}

function runtimeMetaDetailSections(
  block: ChatBlock,
  t: ((key: string, opts?: Record<string, unknown>) => string) | undefined
): RuntimeMetaDetailSection[] {
  if (block.kind !== 'tool' && block.kind !== 'system') return []
  const translate = (key: string): string => t?.(key) ?? key
  const meta = block.meta
  const child = readRuntimeRecord(meta?.child)
  const diagnostics = readRuntimeRecord(meta?.diagnostics)
  const sections: RuntimeMetaDetailSection[] = []

  if (child) {
    const durationMs = readRuntimeNumber(child, 'durationMs')
    const queuedMs = readRuntimeNumber(child, 'queuedMs')
    const tokenBudget = readRuntimeNumber(child, 'tokenBudget')
    const timeBudgetMs = readRuntimeNumber(child, 'timeBudgetMs')
    const evidenceStatus = readRuntimeString(child, 'evidenceBundleStatus')
    const evidenceCount = readRuntimeNumber(child, 'evidenceCount')
    const changedFileCount = readRuntimeNumber(child, 'changedFileCount')
    const rows = [
      { label: translate('toolChildLabel'), value: readRuntimeString(child, 'childLabel') },
      { label: translate('toolChildStatus'), value: readRuntimeString(child, 'childStatus') },
      { label: translate('toolChildId'), value: readRuntimeString(child, 'childId') },
      { label: translate('toolChildJob'), value: readRuntimeString(child, 'jobId') },
      { label: translate('toolChildRun'), value: readRuntimeString(child, 'childRunId') },
      { label: translate('toolChildThread'), value: readRuntimeString(child, 'childThreadId') },
      { label: translate('toolChildTurn'), value: readRuntimeString(child, 'childTurnId') },
      {
        label: translate('toolChildParent'),
        value: [
          readRuntimeString(child, 'parentThreadId'),
          readRuntimeString(child, 'parentTurnId'),
          readRuntimeString(child, 'parentToolCallId')
        ].filter(Boolean).join(' · ')
      },
      { label: translate('toolChildProfile'), value: readRuntimeString(child, 'childProfile') },
      { label: translate('toolChildProfileMode'), value: readRuntimeString(child, 'childProfileMode') },
      { label: translate('toolChildProfileDescription'), value: readRuntimeString(child, 'childProfileDescription') },
      {
        label: translate('toolChildEffort'),
        value: readRuntimeString(child, 'childEffort') || readRuntimeString(child, 'effort')
      },
      { label: translate('toolChildToolPolicy'), value: readRuntimeString(child, 'childToolPolicy') },
      { label: translate('toolChildReturnFormat'), value: readRuntimeString(child, 'returnFormat') },
      {
        label: translate('toolChildTokenBudget'),
        value: tokenBudget !== undefined ? String(tokenBudget) : ''
      },
      {
        label: translate('toolChildTimeBudget'),
        value: timeBudgetMs !== undefined ? formatDuration(timeBudgetMs) : ''
      },
      {
        label: translate('toolChildBudgetExceeded'),
        value: readRuntimeBoolean(child, 'budgetExceeded') ? translate('toolMetaYes') : ''
      },
      {
        label: translate('toolChildEvidence'),
        value: evidenceStatus
          ? `${evidenceStatus}${evidenceCount !== undefined ? ` (${evidenceCount})` : ''}`
          : ''
      },
      {
        label: translate('toolChildParallel'),
        value: [
          readRuntimeNumber(child, 'parallelIndex') !== undefined
            ? String(readRuntimeNumber(child, 'parallelIndex'))
            : '',
          readRuntimeString(child, 'parallelGroupId')
        ].filter(Boolean).join(' · ')
      },
      {
        label: translate('toolChildSeq'),
        value: readRuntimeNumber(child, 'childSeq') !== undefined
          ? `#${readRuntimeNumber(child, 'childSeq')}`
          : ''
      },
      {
        label: translate('toolChildTools'),
        value: readRuntimeNumber(child, 'toolInvocations') !== undefined
          ? String(readRuntimeNumber(child, 'toolInvocations'))
          : ''
      },
      {
        label: translate('toolChildDuration'),
        value: durationMs !== undefined && durationMs > 0 ? formatDuration(durationMs) : ''
      },
      {
        label: translate('toolChildQueued'),
        value: queuedMs !== undefined && queuedMs > 0 ? formatDuration(queuedMs) : ''
      },
      {
        label: translate('toolChildTokens'),
        value: readRuntimeNumber(child, 'totalTokens') !== undefined
          ? String(readRuntimeNumber(child, 'totalTokens'))
          : ''
      },
      { label: translate('toolChildCache'), value: formatRuntimePercent(readRuntimeNumber(child, 'cacheHitRate')) },
      {
        label: translate('toolChildBackground'),
        value: readRuntimeBoolean(child, 'background') ? translate('toolMetaYes') : ''
      },
      { label: translate('toolChildWorktree'), value: readRuntimeString(child, 'worktreeBranch') },
      { label: translate('toolChildWorktreePath'), value: readRuntimeString(child, 'worktreePath') },
      { label: translate('toolChildBaseCommit'), value: readRuntimeString(child, 'baseCommit') },
      { label: translate('toolChildCurrentCommit'), value: readRuntimeString(child, 'currentCommit') },
      {
        label: translate('toolChildChangedFiles'),
        value: changedFileCount !== undefined ? String(changedFileCount) : ''
      },
      { label: translate('toolChildMergeStatus'), value: readRuntimeString(child, 'mergeStatus') }
    ].filter((row) => row.value)

    if (rows.length > 0) {
      sections.push({ id: 'child', title: translate('toolChildAgent'), rows })
    }
  }

  if (diagnostics) {
    const idleMs = readRuntimeNumber(diagnostics, 'idleMs')
    const heartbeatAgeMs = readRuntimeNumber(diagnostics, 'heartbeatAgeMs')
    const staleAfterMs = readRuntimeNumber(diagnostics, 'staleAfterMs') ?? readRuntimeNumber(diagnostics, 'stalledAfterMs')
    const outputBytes = readRuntimeNumber(diagnostics, 'outputBytes')
    const nextOffset = readRuntimeNumber(diagnostics, 'nextOffset')
    const rows = [
      { label: translate('toolJobNotification'), value: readRuntimeString(diagnostics, 'notificationKind') },
      { label: translate('toolChildJob'), value: readRuntimeString(diagnostics, 'jobId') },
      { label: translate('toolChildRun'), value: readRuntimeString(diagnostics, 'childRunId') },
      { label: translate('toolChildThread'), value: readRuntimeString(diagnostics, 'childThreadId') },
      { label: translate('toolChildTurn'), value: readRuntimeString(diagnostics, 'childTurnId') },
      { label: translate('toolParentThread'), value: readRuntimeString(diagnostics, 'parentThreadId') },
      { label: translate('toolParentTurn'), value: readRuntimeString(diagnostics, 'parentTurnId') },
      { label: translate('toolJobStatus'), value: readRuntimeString(diagnostics, 'status') },
      { label: translate('toolJobHeartbeatStatus'), value: formatJobHeartbeatStatus(readRuntimeString(diagnostics, 'heartbeatStatus'), translate) },
      { label: translate('toolAutoContinueStatus'), value: readRuntimeString(diagnostics, 'autoContinueStatus') },
      { label: translate('toolAutoContinueTurn'), value: readRuntimeString(diagnostics, 'autoContinueTurnId') },
      { label: translate('toolDeliveryId'), value: readRuntimeString(diagnostics, 'deliveryId') },
      { label: translate('toolDeliveryStatus'), value: readRuntimeString(diagnostics, 'deliveryStatus') },
      { label: translate('toolDeliveryItem'), value: readRuntimeString(diagnostics, 'deliveryItemId') },
      {
        label: translate('toolDeliveryAttempt'),
        value: readRuntimeNumber(diagnostics, 'completionDeliveryAttempt') !== undefined
          ? String(readRuntimeNumber(diagnostics, 'completionDeliveryAttempt'))
          : ''
      },
      { label: translate('toolDeliveryAt'), value: readRuntimeString(diagnostics, 'completionDeliveryAt') },
      { label: translate('toolDeliveryDeadLetterAt'), value: readRuntimeString(diagnostics, 'completionDeadLetterAt') },
      { label: translate('toolJobWarningCode'), value: readRuntimeString(diagnostics, 'warningCode') },
      { label: translate('toolJobLastHeartbeat'), value: readRuntimeString(diagnostics, 'lastHeartbeatAt') },
      { label: translate('toolJobLeaseOwner'), value: readRuntimeString(diagnostics, 'leaseOwner') },
      { label: translate('toolJobLeaseExpires'), value: readRuntimeString(diagnostics, 'leaseExpiresAt') },
      {
        label: translate('toolJobStaleAfter'),
        value: staleAfterMs !== undefined ? formatDuration(staleAfterMs) : ''
      },
      { label: translate('toolJobRecoveryStatus'), value: readRuntimeString(diagnostics, 'recoveryStatus') },
      {
        label: translate('toolJobRecoveryAttempt'),
        value: readRuntimeNumber(diagnostics, 'recoveryAttempt') !== undefined
          ? String(readRuntimeNumber(diagnostics, 'recoveryAttempt'))
          : ''
      },
      { label: translate('toolJobRecoveryUpdated'), value: readRuntimeString(diagnostics, 'recoveryUpdatedAt') },
      {
        label: translate('toolJobIdle'),
        value: idleMs !== undefined ? formatDuration(idleMs) : ''
      },
      {
        label: translate('toolJobHeartbeat'),
        value: [
          readRuntimeString(diagnostics, 'heartbeatAt'),
          heartbeatAgeMs !== undefined ? formatDuration(heartbeatAgeMs) : ''
        ].filter(Boolean).join(' · ')
      },
      {
        label: translate('toolJobOutput'),
        value: outputBytes !== undefined ? formatBytes(outputBytes) : ''
      },
      {
        label: translate('toolJobNextOffset'),
        value: nextOffset !== undefined ? formatBytes(nextOffset) : ''
      },
      {
        label: translate('toolJobStalled'),
        value: readRuntimeBoolean(diagnostics, 'stalled') ? translate('toolMetaYes') : ''
      },
      {
        label: translate('toolJobLeaseExpired'),
        value: readRuntimeBoolean(diagnostics, 'leaseExpired') ? translate('toolMetaYes') : ''
      },
      {
        label: translate('toolJobOrphaned'),
        value: readRuntimeBoolean(diagnostics, 'orphaned') ? translate('toolMetaYes') : ''
      },
      {
        label: translate('toolChildBackground'),
        value: readRuntimeBoolean(diagnostics, 'background') ? translate('toolMetaYes') : ''
      },
    ].filter((row) => row.value)

    if (rows.length > 0) {
      sections.push({ id: 'job', title: translate('toolJobDiagnostics'), rows })
    }
  }

  return sections
}

function RuntimeMetaDetailPanel({ block }: { block: ChatBlock }): ReactElement | null {
  const { t } = useTranslation('common')
  const sections = runtimeMetaDetailSections(block, t)
  if (sections.length === 0) return null
  return (
    <div className="mb-2 space-y-2 text-[12px] leading-5 text-ds-muted">
      {sections.map((section) => (
        <div key={section.id} className="space-y-1">
          <div className="font-medium text-ds-muted">{section.title}</div>
          <dl className="grid grid-cols-[max-content_minmax(0,1fr)] gap-x-3 gap-y-1">
            {section.rows.map((row) => (
              <div key={`${section.id}-${row.label}`} className="contents">
                <dt className="text-ds-faint">{row.label}</dt>
                <dd className="min-w-0 break-all font-mono text-ds-ink">{row.value}</dd>
              </div>
            ))}
          </dl>
        </div>
      ))}
    </div>
  )
}

function RuntimeMetaBadges({
  block,
  t
}: {
  block: ChatBlock
  t: (key: string, opts?: Record<string, unknown>) => string
}): ReactElement | null {
  const meta =
    block.kind === 'tool' || block.kind === 'approval' || block.kind === 'user' || block.kind === 'system'
      ? block.meta
      : undefined
  if (!meta) return null
  const sources = readMetaSources(meta)
  const attachmentIds = readMetaStringArray(meta, 'attachmentIds')
  const activeSkillIds = readMetaStringArray(meta, 'activeSkillIds')
  const injectedMemoryIds = readMetaStringArray(meta, 'injectedMemoryIds')
  const child = meta.child && typeof meta.child === 'object' ? meta.child as Record<string, unknown> : null
  const childChip = formatChildAgentChip(child, t)
  const jobDiagnosticsChip = formatJobDiagnosticsChip(meta, t)
  const providerDiagnosticsChip = formatProviderDiagnosticsChip(meta, t)
  if (
    sources.length === 0 &&
    attachmentIds.length === 0 &&
    activeSkillIds.length === 0 &&
    injectedMemoryIds.length === 0 &&
    !childChip &&
    !jobDiagnosticsChip &&
    !providerDiagnosticsChip
  ) {
    return null
  }
  const chipClass = 'inline-flex max-w-full items-center gap-1 rounded-md border border-ds-border-muted bg-ds-card/75 px-1.5 py-0.5 text-[11px] font-medium text-ds-faint'
  return (
    <div className="ml-7 mt-1 flex min-w-0 flex-wrap gap-1.5">
      {childChip ? (
        <span className={chipClass} title={childChip.title}>
          <span>{t('toolChildAgent')}</span>
          <span className="max-w-28 truncate font-mono text-ds-muted">{childChip.label}</span>
          {childChip.detail ? (
            <span className="max-w-48 truncate text-ds-muted">{childChip.detail}</span>
          ) : null}
        </span>
      ) : null}
      {jobDiagnosticsChip ? (
        <span className={chipClass} title={jobDiagnosticsChip.title}>
          <span>{jobDiagnosticsChip.label}</span>
          {jobDiagnosticsChip.detail ? (
            <span className="max-w-48 truncate text-ds-muted">{jobDiagnosticsChip.detail}</span>
          ) : null}
        </span>
      ) : null}
      {providerDiagnosticsChip ? (
        <span className={chipClass} title={providerDiagnosticsChip.title}>
          <span>{providerDiagnosticsChip.label}</span>
          <span className="max-w-72 truncate text-ds-muted">{providerDiagnosticsChip.detail}</span>
        </span>
      ) : null}
      {activeSkillIds.length > 0 ? (
        <span className={chipClass} title={activeSkillIds.join(', ')}>
          {t('toolActiveSkills')} {activeSkillIds.length}
        </span>
      ) : null}
      {injectedMemoryIds.length > 0 ? (
        <InjectedMemoryMetaChip meta={meta} memoryIds={injectedMemoryIds} chipClass={chipClass} />
      ) : null}
      {attachmentIds.length > 0 ? (
        <span className={chipClass} title={attachmentIds.join(', ')}>
          {t('toolAttachments')} {attachmentIds.length}
        </span>
      ) : null}
      {sources.slice(0, 4).map((source, index) =>
        source.url ? (
          <a
            key={`${source.url}-${index}`}
            href={source.url}
            target="_blank"
            rel="noreferrer"
            className={chipClass}
            title={source.url}
          >
            {t('toolSources')} {index + 1}
            <span className="max-w-32 truncate text-ds-muted">{source.title || source.url}</span>
          </a>
        ) : (
          <span key={`${source.title}-${index}`} className={chipClass} title={source.title}>
            {t('toolSources')} {index + 1}
          </span>
        )
      )}
    </div>
  )
}

export function summarizeToolBlock(
  block: ToolBlock,
  t: (key: string, opts?: Record<string, unknown>) => string
): string {
  const diagnostics = jobDiagnosticsFromMeta(block.meta)
  if (readRuntimeString(diagnostics, 'notificationKind') === 'background_job_auto_continue') {
    const status = readRuntimeString(diagnostics, 'autoContinueStatus') || readRuntimeString(diagnostics, 'status')
    if (status === 'started') {
      const turnId = readRuntimeString(diagnostics, 'autoContinueTurnId')
      return turnId
        ? t('backgroundAutoContinueStartedWithTurn', { turnId })
        : t('backgroundAutoContinueStarted')
    }
    if (status === 'skipped') {
      return t('backgroundAutoContinueSkipped')
    }
    if (status === 'failed') {
      return t('backgroundAutoContinueFailed')
    }
    return t('backgroundAutoContinueStatus', { status: status || 'updated' })
  }
  if (readRuntimeString(diagnostics, 'notificationKind') === 'background_job_delivery') {
    const status = readRuntimeString(diagnostics, 'deliveryStatus') || readRuntimeString(diagnostics, 'status')
    switch (status) {
      case 'pending':
        return t('backgroundDeliveryPending')
      case 'retry':
        return t('backgroundDeliveryRetry')
      case 'delivered':
        return t('backgroundDeliveryDelivered')
      case 'skipped':
        return t('backgroundDeliverySkipped')
      case 'dead_letter':
      case 'dead_lettered':
        return t('backgroundDeliveryDeadLetter')
      default:
        return t('backgroundDeliveryStatus', { status: status || 'updated' })
    }
  }
  const rawSummary = block.summary?.trim() ?? ''
  const metaToolName = readMetaString(block.meta, 'toolName')
  const toolName = extractToolName(rawSummary) || metaToolName || ''
  const label = block.toolKind === 'subagent'
    ? formatToolTitle(block, t)
    : builtInToolLabel(toolName, t) || humanizeToolName(toolName) || formatToolTitle(block, t)
  const sourceText = [rawSummary, block.detail ?? ''].filter(Boolean).join('\n')
  const filePath = toolFilePath(block)
  const pattern =
    extractQuotedField(sourceText, 'pattern') ||
    extractQuotedField(sourceText, 'query') ||
    readMetaString(block.meta, 'pattern')
  const command = readMetaString(block.meta, 'command')

  if ((toolName === 'read_file' || toolName === 'read') && filePath) {
    return `${label} ${filePath}`
  }
  if ((toolName === 'write' || toolName === 'edit' || toolName === 'write_file' || toolName === 'edit_file' || toolName === 'multi_edit') && filePath) {
    return `${label} ${filePath}`
  }
  if ((toolName === 'grep_files' || toolName === 'search_files' || toolName === 'grep' || toolName === 'find') && pattern) {
    return filePath ? `${label} ${pattern} · ${filePath}` : `${label} ${pattern}`
  }
  if (toolName === 'ls' && filePath) {
    return `${label} ${filePath}`
  }
  if (command && block.toolKind === 'command_execution') {
    return `${formatToolTitle(block, t)} ${summarizeProcessText(command, 72)}`
  }
  if (filePath) {
    return `${label} ${filePath}`
  }
  if (pattern) {
    return `${label} ${pattern}`
  }
  if (rawSummary) {
    const compact = toolName ? rawSummary.replace(/^([a-z0-9_-]+)\s*:\s*/i, '') : rawSummary
    const summary = summarizeProcessText(compact, 72)
    return summary ? `${label} ${summary}` : label
  }
  return label
}

function normalizeProcessText(text: string): string {
  return text.replace(/\s+/g, ' ').trim().toLowerCase()
}

function getProcessDetail(block: ChatBlock, summaryText?: string): ProcessDetail {
  if (block.kind === 'assistant') {
    const split = splitThink(block.text)
    const text = split.content || split.think
    return text.trim() ? { kind: 'assistant', text } : { kind: 'none' }
  }
  if (block.kind === 'tool') {
    const detailText = block.detail?.trim() ?? ''
    if (!detailText) {
      return processBlockHasRuntimeMetaDetail(block) ? { kind: 'runtime_meta' } : { kind: 'none' }
    }
    if (summaryText && normalizeProcessText(detailText) === normalizeProcessText(summaryText)) {
      return processBlockHasRuntimeMetaDetail(block) ? { kind: 'runtime_meta' } : { kind: 'none' }
    }
    const isError = block.status === 'error'
    const patchText =
      block.toolKind === 'file_change' && !isError
        ? extractUnifiedDiffText(detailText)
        : undefined
    return {
      kind: 'tool',
      text: patchText ?? block.detail!,
      isPatch: patchText !== undefined,
      isError,
      filePath: block.filePath
    }
  }
  if (block.kind === 'compaction') {
    const detailText = block.detail?.trim() ?? ''
    if (!detailText) return { kind: 'none' }
    if (summaryText && normalizeProcessText(detailText) === normalizeProcessText(summaryText)) {
      return { kind: 'none' }
    }
    return { kind: 'text', text: detailText }
  }
  if (block.kind === 'approval') return { kind: 'approval' }
  if (block.kind === 'user_input') return { kind: 'user_input' }
  if (block.kind === 'system' && block.text.trim()) {
    if (processBlockHasRuntimeMetaDetail(block)) return { kind: 'runtime_meta' }
    if (block.detail?.trim()) return { kind: 'text', text: block.detail }
    // Short system messages already fit in the summary line — skip the
    // expand affordance so we don't duplicate the same string.
    if (block.text.length <= 140) return { kind: 'none' }
    return { kind: 'text', text: block.text }
  }
  return { kind: 'none' }
}

function ProcessEntryDetail({
  block,
  detail,
  processing
}: {
  block: ChatBlock
  detail: ProcessDetail
  processing: boolean
}): ReactElement | null {
  const runtimeMetaDetail = <RuntimeMetaDetailPanel block={block} />
  if (detail.kind === 'assistant') {
    return (
      <div className="ds-markdown text-[13.5px] leading-6 text-ds-ink">
        <AssistantMarkdown
          text={detail.text}
          streaming={processing && block.kind === 'assistant' && block.id === 'live-assistant'}
        />
      </div>
    )
  }
  if (detail.kind === 'tool') {
    if (detail.isPatch) {
      return (
        <>
          {runtimeMetaDetail}
          <DiffView patch={detail.text} filePath={detail.filePath} />
        </>
      )
    }
    if (detail.isError) {
      return (
        <>
          {runtimeMetaDetail}
          <div className="overflow-hidden rounded-[10px] border border-orange-200/80 bg-orange-50/80 dark:border-orange-800/40 dark:bg-orange-500/10">
            {detail.filePath ? (
              <div className="border-b border-orange-200/70 bg-orange-100/50 px-3 py-1.5 font-mono text-[12px] text-orange-700 dark:border-orange-800/40 dark:bg-orange-500/15 dark:text-orange-300">
                {detail.filePath}
              </div>
            ) : null}
            <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words px-3 py-2.5 font-mono text-[12px] leading-6 text-orange-900 dark:text-orange-100">
              {detail.text}
            </pre>
          </div>
        </>
      )
    }
    return (
      <>
        {runtimeMetaDetail}
        <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words font-mono text-[12px] leading-6 text-ds-ink">
          {detail.text}
        </pre>
      </>
    )
  }
  if (detail.kind === 'runtime_meta') {
    return runtimeMetaDetail
  }
  if (detail.kind === 'text') {
    return <p className="whitespace-pre-wrap text-[13.5px] leading-6 text-ds-muted">{detail.text}</p>
  }
  if (detail.kind === 'approval' && block.kind === 'approval') {
    return <MessageBubble block={block} nested />
  }
  if (detail.kind === 'user_input' && block.kind === 'user_input') {
    return <MessageBubble block={block} nested />
  }
  return null
}

function describeProcessBlock(
  block: ChatBlock,
  t: (key: string, opts?: Record<string, unknown>) => string
): string {
  if (block.kind === 'assistant') {
    return t('processTextLabel')
  }
  if (block.kind === 'tool') {
    return summarizeToolBlock(block, t)
  }
  if (block.kind === 'compaction') {
    if (block.status === 'running') return t('compactionRunning')
    if (block.status === 'error') return block.summary || t('compactionFailed')
    if (typeof block.messagesBefore === 'number' && typeof block.messagesAfter === 'number') {
      return t('compactionCompletedWithCounts', {
        before: block.messagesBefore,
        after: block.messagesAfter
      })
    }
    return block.auto === true ? t('compactionAutoCompleted') : t('compactionManualCompleted')
  }
  if (block.kind === 'approval') {
    return block.summary || t('approvalTitle')
  }
  if (block.kind === 'user_input') {
    return t('userInputTitle')
  }
  if (block.kind === 'system') {
    return block.text
  }
  return 'text' in block ? block.text : t('processed')
}
