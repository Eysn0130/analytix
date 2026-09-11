import { createHash } from 'node:crypto'
import { describe, expect, it, vi } from 'vitest'
import {
  AssistantTextTurnItem,
  type OrdinaryResultSlotV1
} from '../../../packages/runtime/src/contracts/items.js'
import { ThreadDetailResponseV1Schema } from '../../../packages/runtime/src/contracts/thread-detail.js'
import { validOrdinaryResultSlotV1 } from '../general-terminal-publication'
import { sanitizeRuntimeResponse } from './analytix-adapter'

vi.mock('electron', () => ({
  app: {
    isPackaged: false,
    getAppPath: () => process.cwd(),
    getPath: () => process.cwd(),
    getVersion: () => '0.0.0-test'
  }
}))

const ORDINARY_RESULT_DIGEST_DOMAIN = Buffer.from('analytix/ordinary-result/v1\0')
const FIXED_TIME = '2026-08-02T00:00:00Z'

function ordinaryResultSlot(text: string): OrdinaryResultSlotV1 {
  const slot = {
    schemaVersion: 1 as const,
    purpose: 'analytix.ordinary-result/v1' as const,
    projectionVersion: 'analytix.ordinary-output-projection/v1' as const,
    logicalEffect: 'ordinary' as const,
    ordinaryWork: true as const,
    candidateOrigin: 'provider_ordinary_only' as const,
    evidenceAuthority: false as const,
    citationAuthority: false as const,
    factAnswerAllowed: false as const,
    text,
    textSha256: createHash('sha256').update(text, 'utf8').digest('hex'),
    resultDigest: ''
  }
  const digestBody = JSON.stringify({ ...slot, resultDigest: '' })
  slot.resultDigest = createHash('sha256')
    .update(ORDINARY_RESULT_DIGEST_DOMAIN)
    .update(digestBody, 'utf8')
    .digest('hex')
  return slot
}

function assistantItem(text: string, slot: OrdinaryResultSlotV1): Record<string, unknown> {
  return {
    id: 'item-general-terminal',
    turnId: 'turn-general-before-case',
    threadId: 'thread-case-history',
    role: 'assistant',
    status: 'completed',
    createdAt: FIXED_TIME,
    finishedAt: FIXED_TIME,
    kind: 'assistant_text',
    text,
    ordinaryResult: slot
  }
}

function caseThreadDetail(item: Record<string, unknown>): Record<string, unknown> {
  return {
    id: 'thread-case-history',
    title: '案件分析',
    model: 'gpt-test',
    mode: 'agent',
    status: 'idle',
    approvalPolicy: 'on-request',
    sandboxMode: 'workspace-write',
    relation: 'primary',
    createdAt: FIXED_TIME,
    updatedAt: FIXED_TIME,
    historyAuthority: 'case_boundary_only_v1',
    turns: [{
      id: 'turn-general-before-case',
      threadId: 'thread-case-history',
      status: 'completed',
      createdAt: FIXED_TIME,
      finishedAt: FIXED_TIME,
      items: [item]
    }],
    latestSeq: 3,
    pendingApprovalIds: [],
    pendingUserInputIds: [],
    messageCount: 1,
    turnCount: 1,
    latestTurnId: 'turn-general-before-case'
  }
}

function sanitizeDetail(detail: Record<string, unknown>) {
  return sanitizeRuntimeResponse(
    { ok: true, status: 200, body: JSON.stringify(detail) },
    '/v1/threads/thread-case-history',
    null,
    'GET'
  )
}

describe('case-bound typed ordinary history', () => {
  it('preserves only a digest-verified ordinary result in case-bound detail', () => {
    const text = 'Updated the parser and the focused tests pass.'
    const slot = ordinaryResultSlot(text)
    const item = assistantItem(text, slot)
    const detail = caseThreadDetail(item)

    expect(validOrdinaryResultSlotV1(slot)).toBe(true)
    expect(AssistantTextTurnItem.safeParse(item).success).toBe(true)
    expect(ThreadDetailResponseV1Schema.safeParse(detail).success).toBe(true)
    const response = sanitizeDetail(detail)
    expect(response.status).toBe(200)
    expect(response.ok).toBe(true)
    expect(JSON.parse(response.body)).toEqual(detail)
  })

  it.each([
    {
      name: 'missing slot',
      build: () => {
        const text = 'Updated the parser and the focused tests pass.'
        const item = assistantItem(text, ordinaryResultSlot(text))
        delete item.ordinaryResult
        return { item, sentinel: text }
      }
    },
    {
      name: 'item and slot text mismatch',
      build: () => {
        const slot = ordinaryResultSlot('Updated the parser and the focused tests pass.')
        const text = 'Updated the parser but the result text changed.'
        return { item: assistantItem(text, slot), sentinel: text }
      }
    },
    {
      name: 'text hash tamper',
      build: () => {
        const text = 'Updated the parser and the focused tests pass.'
        const slot = ordinaryResultSlot(text)
        slot.textSha256 = '0'.repeat(64)
        return { item: assistantItem(text, slot), sentinel: text }
      }
    },
    {
      name: 'result digest tamper',
      build: () => {
        const text = 'Updated the parser and the focused tests pass.'
        const slot = ordinaryResultSlot(text)
        slot.resultDigest = '0'.repeat(64)
        return { item: assistantItem(text, slot), sentinel: text }
      }
    },
    {
      name: 'complete financial identifier',
      build: () => {
        const text = 'Account 6222021234567890123 was observed.'
        return { item: assistantItem(text, ordinaryResultSlot(text)), sentinel: text }
      }
    },
    {
      name: 'protected case fact',
      build: () => {
        const text = '交易金额 100 元。'
        return { item: assistantItem(text, ordinaryResultSlot(text)), sentinel: text }
      }
    },
    {
      name: 'internal case entity reference',
      build: () => {
        const text = `Entity cer1_${'a'.repeat(64)} was updated.`
        return { item: assistantItem(text, ordinaryResultSlot(text)), sentinel: text }
      }
    },
    {
      name: 'ordinary and accepted-final authority mixed',
      build: () => {
        const text = 'Updated the parser and the focused tests pass.'
        return {
          item: { ...assistantItem(text, ordinaryResultSlot(text)), acceptedFinal: {}, acceptedFinalView: {} },
          sentinel: text
        }
      }
    }
  ])('fails closed for $name without reflecting text', ({ build }) => {
    const { item, sentinel } = build()
    const response = sanitizeDetail(caseThreadDetail(item))
    expect(response.ok).toBe(false)
    expect(response.status).toBe(502)
    expect(JSON.parse(response.body)).toMatchObject({ code: 'runtime_response_not_public' })
    expect(response.body).not.toContain(sentinel)
  })
})
