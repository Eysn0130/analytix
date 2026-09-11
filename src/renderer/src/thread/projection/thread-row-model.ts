import type { ReactElement } from 'react'
import type { ChatBlock, RuntimeConnectionStatus, ToolBlock } from '../../agent/types'
import type { TurnSections } from './derive-turn-sections'
import type { ThreadTurn } from './thread-turns'

export type ThreadRowKind =
  | 'emptyHero'
  | 'forkBanner'
  | 'forkPoint'
  | 'loadEarlier'
  | 'turn'
  | 'collapseEarlier'
  | 'liveOnlyTurn'
  | 'bottomSpacer'

export type ThreadTurnTiming = {
  durationMs?: number
}

export type ThreadTurnProjection = {
  key: string
  userNavigationItemId?: string
  turn: ThreadTurn
  absoluteIndex: number
  isLatest: boolean
  isLiveTurn: boolean
  isProcessing: boolean
  hasLiveStream: boolean
  live: string
  liveProcessText: string
  liveContent: string
  timing: ThreadTurnTiming
  pendingRuntimeWork: boolean
  sections: TurnSections
  reviewBlocks: Extract<ChatBlock, { kind: 'review' }>[]
  generatedFileBlocks: ToolBlock[]
  devPreviewCard: ReactElement | null
  showForkPoint: boolean
}

export type ThreadRow =
  | { kind: 'emptyHero'; id: string }
  | { kind: 'forkBanner'; id: string; parentTitle: string }
  | { kind: 'forkPoint'; id: string; parentTitle: string }
  | { kind: 'loadEarlier'; id: string; hiddenCount: number; pageSize: number }
  | { kind: 'turn'; id: string; turn: ThreadTurnProjection }
  | { kind: 'collapseEarlier'; id: string }
  | {
      kind: 'liveOnlyTurn'
      id: string
      turnId?: string | null
      live: string
      isProcessing: boolean
      liveContent: string
      timing: ThreadTurnTiming
      sections: TurnSections
      reviewBlocks: Extract<ChatBlock, { kind: 'review' }>[]
      devPreviewCard: ReactElement | null
    }
  | { kind: 'bottomSpacer'; id: string }

export type ThreadJumpAnchor = {
  key: string
  label: string
  title: string
}

export type ThreadUserMessageNavigationItem = {
  id: string
  turnKey: string
  label: string
  preview: string
  responsePreview?: string
  contextChips?: ThreadUserMessageNavigationContextChip[]
  contextChipOverflowCount?: number
  title: string
  position: number
  durationMs?: number
  isHeartbeat?: boolean
}

export type ThreadUserMessageNavigationContextChip = {
  label: string
  title?: string
  icon: ThreadUserMessageNavigationContextIcon
}

export type ThreadUserMessageNavigationContextIcon =
  | 'code'
  | 'css'
  | 'document'
  | 'file'
  | 'globe'
  | 'image'
  | 'javascript'
  | 'json'
  | 'react'
  | 'typescript'

export type ThreadProjectionSnapshot = {
  rows: ThreadRow[]
  turns: ThreadTurn[]
  visibleTurns: ThreadTurn[]
  hiddenTurnCount: number
  anchors: ThreadJumpAnchor[]
  userMessageNavigationItems: ThreadUserMessageNavigationItem[]
  hasContent: boolean
  scrollContentKey: string
}

export type ThreadProjectionRuntimeState = {
  activeThreadId: string | null
  runtimeConnection: RuntimeConnectionStatus
  blocks: ChatBlock[]
  live: string
}
