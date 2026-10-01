import type { ChatBlock, ToolBlock } from '../../agent/types'
import { generatedArtifactMetadataSchema } from '../../../../../packages/runtime/src/contracts/generated-artifact'
import {
  extractDiffFilePath,
  extractUnifiedDiffText,
  formatFilePathForDisplay,
} from '../../lib/diff-stats'
import {
  isProcessBlock,
  splitThink,
  type ThreadTurn
} from './thread-turns'

export type TurnAssistantBlock = Extract<ChatBlock, { kind: 'assistant' }>
export type TurnCompactionBlock = Extract<ChatBlock, { kind: 'compaction' }>

export type TurnSections = {
  processBlocks: ChatBlock[]
  assistantContentBlocks: TurnAssistantBlock[]
  compactionBlocks: TurnCompactionBlock[]
  generatedFileBlocks: ToolBlock[]
  turnFileChanges: ToolBlock[]
}

type ResolvedFileChangeBlock = ToolBlock & {
  detail: string
  filePath: string
}

type DeriveTurnSectionsInput = {
  turn: ThreadTurn
  isProcessing: boolean
  liveProcessText: string
  liveContent: string
  workspaceRoot: string
}

function fileChangeGroupKey(filePath: string): string {
  return filePath.trim().replace(/\\/g, '/').replace(/\/+$/, '')
}

function mergeFileChangeBlocks(changes: ResolvedFileChangeBlock[]): ToolBlock[] {
  const merged: ResolvedFileChangeBlock[] = []
  const indexByPath = new Map<string, number>()

  for (const change of changes) {
    const key = fileChangeGroupKey(change.filePath)
    const existingIndex = indexByPath.get(key)
    if (existingIndex === undefined) {
      indexByPath.set(key, merged.length)
      merged.push(change)
      continue
    }

    const existing = merged[existingIndex]
    merged[existingIndex] = {
      ...existing,
      detail: [existing.detail, change.detail].filter(Boolean).join('\n\n')
    }
  }

  return merged
}

function metaArrayLength(meta: Record<string, unknown> | undefined, key: string): number {
  const value = meta?.[key]
  return Array.isArray(value) ? value.length : 0
}

function hasGeneratedFiles(block: ToolBlock): boolean {
  return (
    block.status === 'success' &&
    (metaArrayLength(block.meta, 'attachments') > 0 || metaArrayLength(block.meta, 'generatedFiles') > 0 ||
      generatedArtifactMetadataSchema.safeParse(block.meta?.generatedArtifact).success)
  )
}

function hasRewindPlan(block: ToolBlock): boolean {
  return Boolean(block.meta?.rewindPlan && block.status === 'success')
}

function findLastAssistantContentIndex(blocks: ChatBlock[]): number {
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    const block = blocks[index]
    if (block.kind === 'assistant' && splitThink(block.text).content.trim()) {
      return index
    }
  }
  return -1
}

export function deriveTurnSections({
  turn,
  isProcessing,
  liveProcessText,
  liveContent,
  workspaceRoot
}: DeriveTurnSectionsInput): TurnSections {
  const processBlocks: ChatBlock[] = []
  const assistantContentBlocks: TurnAssistantBlock[] = []
  const compactionBlocks: TurnCompactionBlock[] = []
  const finalAssistantContentIndex = isProcessing ? -1 : findLastAssistantContentIndex(turn.blocks)

  for (const [index, block] of turn.blocks.entries()) {
    if (block.kind === 'assistant') {
      const split = splitThink(block.text)
      if (split.content.trim()) {
        const contentBlock: TurnAssistantBlock = { ...block, text: split.content }
        if (index === finalAssistantContentIndex) {
          assistantContentBlocks.push(contentBlock)
        } else {
          processBlocks.push(contentBlock)
        }
      }
      continue
    }
    if (block.kind === 'compaction') {
      compactionBlocks.push(block)
      continue
    }
    if (isProcessBlock(block)) {
      processBlocks.push(block)
    }
  }

  void liveProcessText
  // Streaming assistant text is rendered by MessageTimeline as the normal
  // live assistant MessageBubble. Keep only reasoning in the work/process
  // area so completion does not move the answer between surfaces.

  const turnFileChanges: ToolBlock[] = isProcessing
    ? []
    : mergeFileChangeBlocks(turn.blocks.flatMap((block): ResolvedFileChangeBlock[] => {
        if (
          !(block.kind === 'tool' && block.toolKind === 'file_change' && block.status === 'success')
        ) {
          return []
        }

        if (hasRewindPlan(block)) {
          const rewindPlan = block.meta!.rewindPlan!
          return [{
            ...block,
            detail: '',
            filePath: `Checkpoint rewind plan (${rewindPlan.summary.fileCount} files)`
          }]
        }

        const detailText = extractUnifiedDiffText(block.detail)
        if (!detailText) return []

        const resolvedFilePath = formatFilePathForDisplay(
          extractDiffFilePath(detailText, block.filePath),
          workspaceRoot
        )
        if (!resolvedFilePath) return []

        return [{ ...block, detail: detailText, filePath: resolvedFilePath }]
      }))

  const generatedFileBlocks: ToolBlock[] = turn.blocks.filter(
    (block): block is ToolBlock => block.kind === 'tool' && hasGeneratedFiles(block)
  )

  return { processBlocks, assistantContentBlocks, compactionBlocks, generatedFileBlocks, turnFileChanges }
}
