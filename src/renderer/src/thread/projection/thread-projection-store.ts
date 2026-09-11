import type { ReactElement } from 'react'
import type { ChatBlock } from '../../agent/types'
import { threadHasPendingRuntimeWork } from '../../store/chat-store-runtime-helpers'
import { deriveTurnSections } from './derive-turn-sections'
import {
  groupThreadTurns,
  splitThink,
  stableThreadTurnKey,
  type ThreadTurn
} from './thread-turns'
import type {
  ThreadJumpAnchor,
  ThreadUserMessageNavigationContextIcon,
  ThreadUserMessageNavigationContextChip,
  ThreadProjectionSnapshot,
  ThreadRow,
  ThreadTurnProjection,
  ThreadTurnTiming,
  ThreadUserMessageNavigationItem
} from './thread-row-model'

const MAX_NAVIGATION_CONTEXT_CHIPS = 2

export type BuildThreadProjectionInput = {
  blocks: ChatBlock[]
  turns?: ThreadTurn[]
  live: string
  activeThreadId: string | null
  workspaceRoot: string
  currentTurnId?: string | null
  currentTurnUserId?: string | null
  turnStartedAtByUserId: Record<string, number>
  turnDurationByUserId: Record<string, number>
  busy: boolean
  tickNow: number
  hiddenTurnCount: number
  pageSize: number
  autoCollapseThreshold: number
  activeThreadGoal: unknown
  devPreviewCard?: ReactElement | null
  forkedFromThreadId?: string | null
  forkedFromTitle?: string
  forkedFromTurnCount?: number
  turnPreview: (turn: ThreadTurn, fallback: string) => string
  turnTitle: (index: number) => string
}

function blockRuntimeTurnId(block: ChatBlock | undefined): string | null {
  const meta = (block as { meta?: Record<string, unknown> } | undefined)?.meta
  const turnId = typeof meta?.turnId === 'string' ? meta.turnId.trim() : ''
  return turnId || null
}

function runtimeTurnId(turn: ThreadTurn): string | null {
  const userTurnId = blockRuntimeTurnId(turn.user)
  if (userTurnId) return userTurnId
  for (const block of turn.blocks) {
    const turnId = blockRuntimeTurnId(block)
    if (turnId) return turnId
  }
  return null
}

function blockScrollStamp(block: ChatBlock | undefined): string {
  if (!block) return ''
  switch (block.kind) {
    case 'user':
    case 'assistant':
    case 'system':
      return `${block.id}:${block.kind}:${block.text.length}`
    case 'tool':
      return `${block.id}:${block.kind}:${block.status}:${block.summary.length}:${block.detail?.length ?? 0}`
    case 'review':
      return `${block.id}:${block.kind}:${block.status}:${block.reviewText?.length ?? 0}`
    case 'approval':
    case 'user_input':
    case 'compaction':
      return `${block.id}:${block.kind}:${block.status}`
    default:
      return ''
  }
}

function deriveTurnTiming({
  turn,
  currentTurnUserId,
  turnStartedAtByUserId,
  turnDurationByUserId,
  tickNow
}: Pick<
  BuildThreadProjectionInput,
  | 'currentTurnUserId'
  | 'turnStartedAtByUserId'
  | 'turnDurationByUserId'
  | 'tickNow'
> & {
  turn: ThreadTurn
}): ThreadTurnTiming {
  const userId = turn.user?.id
  const isLive = !!(userId && currentTurnUserId === userId)
  const startedAt = userId ? turnStartedAtByUserId[userId] : undefined
  const recordedDuration = userId ? turnDurationByUserId[userId] : undefined
  const liveDuration =
    isLive && typeof startedAt === 'number'
      ? Math.max(0, tickNow - startedAt)
      : undefined
  const durationMs =
    typeof liveDuration === 'number'
      ? Math.max(recordedDuration ?? 0, liveDuration)
      : recordedDuration
  return { durationMs }
}

function compactOneLine(text: string): string {
  return text.trim().replace(/\s+/g, ' ')
}

function navigationResponsePreview(turn: ThreadTurn): string {
  for (const block of turn.blocks) {
    if (block.kind !== 'assistant') continue
    const text = compactOneLine(splitThink(block.text).content)
    if (!text) continue
    return text.length > 132 ? `${text.slice(0, 131).trimEnd()}...` : text
  }
  return ''
}

function normalizedNavigationPath(path: string): string {
  return path.trim().replace(/\\/g, '/').replace(/\/+$/, '')
}

function basename(path: string): string {
  const normalized = normalizedNavigationPath(path)
  return normalized.split('/').filter(Boolean).pop() ?? normalized
}

function urlNavigationLabel(value: string): string | null {
  try {
    const url = new URL(value)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return null
    const path = url.pathname === '/' ? '' : url.pathname
    return `${url.host}${path}${url.search}` || value
  } catch {
    return null
  }
}

function navigationContextIcon(label: string, raw: string): ThreadUserMessageNavigationContextIcon | null {
  if (urlNavigationLabel(raw)) return 'globe'

  const extension = label.includes('.')
    ? label.split('.').pop()?.toLowerCase() ?? ''
    : ''

  switch (extension) {
    case 'ts':
      return 'typescript'
    case 'tsx':
    case 'jsx':
      return 'react'
    case 'js':
    case 'mjs':
    case 'cjs':
      return 'javascript'
    case 'css':
    case 'scss':
    case 'sass':
    case 'less':
      return 'css'
    case 'json':
    case 'jsonc':
      return 'json'
    case 'md':
    case 'mdx':
    case 'markdown':
      return 'document'
    case 'png':
    case 'jpg':
    case 'jpeg':
    case 'gif':
    case 'webp':
    case 'svg':
      return 'image'
    case 'py':
    case 'go':
    case 'rs':
    case 'java':
    case 'rb':
    case 'sql':
      return 'code'
    case 'pdf':
    case 'doc':
    case 'docx':
    case 'xls':
    case 'xlsx':
    case 'ppt':
    case 'pptx':
      return 'file'
    default:
      return null
  }
}

function readRecordString(value: unknown, key: string): string | undefined {
  if (!value || typeof value !== 'object') return undefined
  const raw = (value as Record<string, unknown>)[key]
  return typeof raw === 'string' && raw.trim() ? raw.trim() : undefined
}

function addNavigationContextChip(
  chips: ThreadUserMessageNavigationContextChip[],
  seen: Set<string>,
  path: string | undefined,
  fallbackName?: string
): void {
  const raw = path?.trim() || fallbackName?.trim()
  if (!raw) return
  const key = normalizedNavigationPath(raw).toLowerCase()
  if (!key || seen.has(key)) return

  const label = urlNavigationLabel(raw) ?? fallbackName?.trim() ?? basename(raw)
  const icon = navigationContextIcon(label, raw)
  if (!icon) return

  seen.add(key)
  chips.push({
    label,
    title: raw,
    icon
  })
}

function navigationContextChips(turn: ThreadTurn): {
  contextChips?: ThreadUserMessageNavigationContextChip[]
  contextChipOverflowCount?: number
} {
  const chips: ThreadUserMessageNavigationContextChip[] = []
  const seen = new Set<string>()

  for (const reference of turn.user?.meta?.fileReferences ?? []) {
    addNavigationContextChip(chips, seen, reference.relativePath || reference.path, reference.name)
  }

  for (const block of turn.blocks) {
    if (block.kind !== 'tool') continue

    addNavigationContextChip(chips, seen, block.filePath)

    const generatedFiles = block.meta?.generatedFiles
    if (Array.isArray(generatedFiles)) {
      for (const generatedFile of generatedFiles) {
        addNavigationContextChip(
          chips,
          seen,
          readRecordString(generatedFile, 'relativePath')
            ?? readRecordString(generatedFile, 'path')
            ?? readRecordString(generatedFile, 'absolutePath')
            ?? readRecordString(generatedFile, 'localFilePath')
            ?? readRecordString(generatedFile, 'FilePath'),
          readRecordString(generatedFile, 'name')
        )
      }
    }

    const attachments = block.meta?.attachments
    if (Array.isArray(attachments)) {
      for (const attachment of attachments) {
        addNavigationContextChip(
          chips,
          seen,
          readRecordString(attachment, 'localFilePath')
            ?? readRecordString(attachment, 'FilePath')
            ?? readRecordString(attachment, 'path'),
          readRecordString(attachment, 'name')
        )
      }
    }
  }

  if (chips.length === 0) return {}

  const visibleChips = chips.slice(0, MAX_NAVIGATION_CONTEXT_CHIPS)
  const overflowCount = chips.length - visibleChips.length

  return {
    contextChips: visibleChips,
    ...(overflowCount > 0 ? { contextChipOverflowCount: overflowCount } : {})
  }
}

export function userMessageNavigationItemId(turnKey: string): string {
  return `${turnKey}:user`
}

function anchorsFromNavigationItems(items: ThreadUserMessageNavigationItem[]): ThreadJumpAnchor[] {
  return items.map((item) => ({
    key: item.turnKey,
    label: item.label,
    title: item.title
  }))
}

export function buildThreadProjection(input: BuildThreadProjectionInput): ThreadProjectionSnapshot {
  const turns = input.turns ?? groupThreadTurns(input.blocks)
  const visibleTurns = input.hiddenTurnCount > 0 ? turns.slice(input.hiddenTurnCount) : turns
  const hasLiveShell = input.busy && !!input.activeThreadId && !!input.currentTurnId
  const hasContent = input.blocks.length > 0 || !!input.live || hasLiveShell
  const latestBlock = input.blocks[input.blocks.length - 1]
  const rows: ThreadRow[] = []
  const parentTitle = input.forkedFromTitle?.trim() ?? ''
  const forkBoundaryTurnCount =
    typeof input.forkedFromTurnCount === 'number'
      ? Math.max(0, input.forkedFromTurnCount)
      : undefined
  const userMessageNavigationItems: ThreadUserMessageNavigationItem[] = []
  let questionIndex = turns.slice(0, input.hiddenTurnCount).filter((turn) => turn.user).length
  let hasVisibleLiveTurn = false
  const currentTurnId = input.currentTurnId?.trim() ?? ''
  const currentTurnUserId = input.currentTurnUserId?.trim() ?? ''

  if (!hasContent || !input.activeThreadId) {
    rows.push({ kind: 'emptyHero', id: `empty:${input.activeThreadId ?? 'none'}` })
  }

  if (input.forkedFromThreadId) {
    rows.push({ kind: 'forkBanner', id: `fork-banner:${input.forkedFromThreadId}`, parentTitle })
  }

  if (input.hiddenTurnCount > 0) {
    rows.push({
      kind: 'loadEarlier',
      id: `load-earlier:${input.hiddenTurnCount}`,
      hiddenCount: input.hiddenTurnCount,
      pageSize: input.pageSize
    })
  }

  visibleTurns.forEach((turn, index) => {
    const absoluteIndex = input.hiddenTurnCount + index
    const key = stableThreadTurnKey(turn, absoluteIndex)
    const timing = deriveTurnTiming({ ...input, turn })
    const userNavigationItemId = turn.user ? userMessageNavigationItemId(key) : undefined

    if (turn.user && userNavigationItemId) {
      questionIndex += 1
      const responsePreview = navigationResponsePreview(turn)
      const context = navigationContextChips(turn)
      userMessageNavigationItems.push({
        id: userNavigationItemId,
        turnKey: key,
        label: String(questionIndex),
        preview: compactOneLine(turn.user.text),
        ...(responsePreview ? { responsePreview } : {}),
        ...context,
        title: input.turnPreview(turn, input.turnTitle(questionIndex)),
        position: questionIndex,
        durationMs: timing.durationMs
      })
    }

    if (forkBoundaryTurnCount !== undefined && absoluteIndex === forkBoundaryTurnCount) {
      rows.push({ kind: 'forkPoint', id: `fork-point:${absoluteIndex}`, parentTitle })
    }

    const isLatest = index === visibleTurns.length - 1
    const turnId = runtimeTurnId(turn)
    const isLiveTurn = currentTurnId
      ? Boolean(
          turnId === currentTurnId ||
          (currentTurnUserId && turn.user?.id === currentTurnUserId)
        )
      : isLatest && Boolean(input.busy || input.live.trim())
    if (isLiveTurn) hasVisibleLiveTurn = true
    const live = isLiveTurn ? input.live : ''
    const { content: liveContent } = splitThink(live)
    const liveProcessText = ''
    const hasLiveStream = isLiveTurn && !!liveContent.trim()
    const pendingRuntimeWork = threadHasPendingRuntimeWork(turn.blocks)
    const isProcessing = (input.busy && isLiveTurn) || pendingRuntimeWork || hasLiveStream
    const sections = deriveTurnSections({
      turn,
      isProcessing,
      liveProcessText,
      liveContent,
      workspaceRoot: input.workspaceRoot
    })
    const projected: ThreadTurnProjection = {
      key,
      userNavigationItemId,
      turn,
      absoluteIndex,
      isLatest,
      isLiveTurn,
      isProcessing,
      hasLiveStream,
      live,
      liveProcessText,
      liveContent,
      timing,
      pendingRuntimeWork,
      sections,
      reviewBlocks: turn.blocks.filter((block): block is Extract<ChatBlock, { kind: 'review' }> => block.kind === 'review'),
      generatedFileBlocks: sections.generatedFileBlocks,
      devPreviewCard: isLatest ? input.devPreviewCard ?? null : null,
      showForkPoint: false
    }
    rows.push({ kind: 'turn', id: `turn:${key}`, turn: projected })
  })

  if (forkBoundaryTurnCount !== undefined && forkBoundaryTurnCount === turns.length && hasContent) {
    rows.push({ kind: 'forkPoint', id: `fork-point:${turns.length}`, parentTitle })
  }

  if (
    input.hiddenTurnCount === 0 &&
    turns.length > input.pageSize &&
    turns.length > input.autoCollapseThreshold &&
    !input.busy
  ) {
    rows.push({ kind: 'collapseEarlier', id: 'collapse-earlier' })
  }

  if ((input.blocks.length === 0 && (input.live || hasLiveShell)) || (hasLiveShell && !hasVisibleLiveTurn)) {
    const { content: liveContent } = splitThink(input.live)
    const liveOnlyTurnId = input.currentTurnId?.trim() || 'pending'
    const liveOnlyThreadId = input.activeThreadId?.trim() || 'thread'
    const sections = deriveTurnSections({
      turn: { blocks: [] },
      isProcessing: input.busy,
      liveProcessText: '',
      liveContent,
      workspaceRoot: input.workspaceRoot
    })
    rows.push({
      kind: 'liveOnlyTurn',
      id: `live-only-turn:${liveOnlyThreadId}:${liveOnlyTurnId}`,
      turnId: input.currentTurnId ?? null,
      live: input.live,
      isProcessing: input.busy,
      liveContent,
      timing: deriveTurnTiming({ ...input, turn: { blocks: [] } }),
      sections,
      reviewBlocks: [],
      devPreviewCard: input.devPreviewCard ?? null
    })
  }

  rows.push({ kind: 'bottomSpacer', id: 'bottom-spacer' })

  return {
    rows,
    turns,
    visibleTurns,
    hiddenTurnCount: input.hiddenTurnCount,
    anchors: anchorsFromNavigationItems(userMessageNavigationItems),
    userMessageNavigationItems,
    hasContent,
    scrollContentKey: [
      input.activeThreadId ?? '',
      turns.length,
      input.blocks.length,
      blockScrollStamp(latestBlock),
      input.live.length
    ].join(':')
  }
}
