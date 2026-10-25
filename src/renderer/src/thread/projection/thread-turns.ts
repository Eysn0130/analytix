import type { ChatBlock } from '../../agent/types'
import { sanitizePublicAssistantText } from '@shared/public-runtime-content'
import { isNeutralProviderRequestStage } from '../../store/chat-store-runtime-helpers'

export type ThreadTurn = {
  user?: Extract<ChatBlock, { kind: 'user' }>
  blocks: ChatBlock[]
}

export function blockRuntimeTurnId(block: ChatBlock | undefined): string | null {
  const meta = (block as { meta?: Record<string, unknown> } | undefined)?.meta
  const turnId = typeof meta?.turnId === 'string' ? meta.turnId.trim() : ''
  return turnId || null
}

export function runtimeTurnId(turn: ThreadTurn): string | null {
  const userTurnId = blockRuntimeTurnId(turn.user)
  if (userTurnId) return userTurnId
  for (const block of turn.blocks) {
    const turnId = blockRuntimeTurnId(block)
    if (turnId) return turnId
  }
  return null
}

export function groupThreadTurns(blocks: ChatBlock[]): ThreadTurn[] {
  const turns: ThreadTurn[] = []
  const runtimeTurns = new Map<string, ThreadTurn>()
  let current: ThreadTurn | null = null

  for (const block of blocks) {
    if (block.kind === 'system' && isNeutralProviderRequestStage(block.meta?.stage) &&
      (block.severity === undefined || block.severity === 'info') && !block.code && !block.meta?.providerError && !block.meta?.providerRetry) continue
    const turnId = blockRuntimeTurnId(block)
    if (turnId) {
      let turn = runtimeTurns.get(turnId)
      if (!turn) {
        // A legacy opening input may precede the first runtime-owned item.
        turn = block.kind !== 'user' && current && !runtimeTurnId(current) ? current : { blocks: [] }
        if (turn !== current) turns.push(turn)
        runtimeTurns.set(turnId, turn)
      }
      if (block.kind === 'user' && !turn.user && turn.blocks.length === 0) turn.user = block
      else turn.blocks.push(block)
      current = turn
      continue
    }
    if (block.kind === 'user') {
      current = { user: block, blocks: [] }
      turns.push(current)
      continue
    }
    if (!current) {
      current = { blocks: [] }
      turns.push(current)
    }
    current.blocks.push(block)
  }

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
