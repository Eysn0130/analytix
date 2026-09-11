import type { ChatBlock } from '../../agent/types'
import { sanitizePublicAssistantText } from '@shared/public-runtime-content'

export type ThreadTurn = {
  user?: Extract<ChatBlock, { kind: 'user' }>
  blocks: ChatBlock[]
}

export function groupThreadTurns(blocks: ChatBlock[]): ThreadTurn[] {
  const turns: ThreadTurn[] = []
  let current: ThreadTurn | null = null

  for (const block of blocks) {
    if (block.kind === 'user') {
      if (current) turns.push(current)
      current = { user: block, blocks: [] }
      continue
    }
    if (!current) current = { blocks: [] }
    current.blocks.push(block)
  }

  if (current) turns.push(current)
  return turns
}

export function stableThreadTurnKey(turn: ThreadTurn, fallbackIndex: number): string {
  return turn.user?.id ?? turn.blocks[0]?.id ?? `turn-${fallbackIndex}`
}

export function sameThreadTurnContent(left: ThreadTurn, right: ThreadTurn): boolean {
  if (left.user !== right.user) return false
  if (left.blocks.length !== right.blocks.length) return false
  for (let index = 0; index < left.blocks.length; index += 1) {
    if (left.blocks[index] !== right.blocks[index]) return false
  }
  return true
}

export function splitThink(text: string): { think: string; content: string } {
  return { think: '', content: sanitizePublicAssistantText(text) }
}

export function blockHasPendingRuntimeWork(block: ChatBlock): boolean {
  if (block.kind === 'tool') return block.status === 'running'
  if (block.kind === 'compaction') return block.status === 'running'
  if (block.kind === 'review') return block.status === 'running'
  if (block.kind === 'approval') return block.status === 'pending'
  if (block.kind === 'user_input') return block.status === 'pending'
  return false
}

export function isProcessBlock(block: ChatBlock): boolean {
  return (
    block.kind === 'tool' ||
    block.kind === 'approval' ||
    block.kind === 'user_input' ||
    block.kind === 'system'
  )
}

export function findTrailingAssistantContentStart(blocks: ChatBlock[]): number {
  let start = blocks.length

  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    const block = blocks[index]
    if (block.kind !== 'assistant') break

    const split = splitThink(block.text)
    if (!split.content.trim()) break
    start = index
  }

  return start
}
