import { describe, expect, it, vi } from 'vitest'
import { PENDING_AUTO_THREAD_TITLE, type ThreadRecord } from '../contracts/threads.js'
import type { Turn } from '../contracts/turns.js'
import type { ModelClient, ModelRequest, ModelStreamChunk } from '../ports/model-client.js'
import {
  cleanGeneratedTitle,
  deriveFallbackTitle,
  shouldAutoGenerateTitle,
  ThreadTitleService
} from '../services-test-support/thread-title-service.js'

function thread(overrides: Partial<ThreadRecord> = {}): ThreadRecord {
  return {
    id: 'thr_title',
    title: PENDING_AUTO_THREAD_TITLE,
    workspace: '/workspace/analytix',
    model: 'deepseek-v4-pro',
    mode: 'agent',
    status: 'idle',
    approvalPolicy: 'on-request',
    sandboxMode: 'workspace-write',
    relation: 'primary',
    createdAt: '2026-06-24T00:00:00.000Z',
    updatedAt: '2026-06-24T00:00:00.000Z',
    turns: [turnRecord()],
    ...overrides
  }
}

function turnRecord(overrides: Partial<Turn> = {}): Turn {
  return {
    id: 'turn_1',
    threadId: 'thr_title',
    status: 'running',
    prompt: 'prompt',
    steering: [],
    createdAt: '2026-06-24T00:00:00.000Z',
    items: [],
    attachmentIds: [],
    activeSkillIds: [],
    injectedMemoryIds: [],
    ...overrides
  }
}

function model(chunks: ModelStreamChunk[] | Error): ModelClient {
  return {
    provider: 'test',
    model: 'deepseek-v4-pro',
    async *stream(_request: ModelRequest): AsyncIterable<ModelStreamChunk> {
      if (chunks instanceof Error) throw chunks
      for (const chunk of chunks) yield chunk
    }
  }
}

describe('ThreadTitleService', () => {
  it('writes a cleaned model-generated title for the first user prompt', async () => {
    let current = thread()
    const update = vi.fn(async (_threadId: string, patch: { title?: string }) => {
      current = { ...current, ...patch }
      return current
    })
    const service = new ThreadTitleService({
      threadService: {
        get: vi.fn(async () => current),
        update
      },
      model: model([
        { kind: 'assistant_text_delta', text: '"修复顶栏标题。"' },
        { kind: 'completed', stopReason: 'stop' }
      ])
    })

    await service.generateForFirstUserPrompt({
      threadId: current.id,
      prompt: '请修复顶栏标题和副标题的显示逻辑'
    })

    expect(update).toHaveBeenCalledWith(current.id, { title: '修复顶栏标题' })
  })

  it('falls back to a prompt-derived title when the model fails', async () => {
    let current = thread()
    const update = vi.fn(async (_threadId: string, patch: { title?: string }) => {
      current = { ...current, ...patch }
      return current
    })
    const service = new ThreadTitleService({
      threadService: {
        get: vi.fn(async () => current),
        update
      },
      model: model(new Error('model unavailable'))
    })

    await service.generateForFirstUserPrompt({
      threadId: current.id,
      prompt: '分析 Code 页标题为什么提前出现。请给出方案。'
    })

    expect(update).toHaveBeenCalledWith(current.id, { title: '分析 Code 页标题为什么提前出现' })
  })

  it('does not overwrite a title changed while generation is in flight', async () => {
    const pending = thread()
    const renamed = thread({ title: 'Manual title' })
    let reads = 0
    const update = vi.fn()
    const service = new ThreadTitleService({
      threadService: {
        get: vi.fn(async () => {
          reads += 1
          return reads === 1 ? pending : renamed
        }),
        update
      },
      model: model([
        { kind: 'assistant_text_delta', text: 'Generated title' },
        { kind: 'completed', stopReason: 'stop' }
      ])
    })

    await service.generateForFirstUserPrompt({
      threadId: pending.id,
      prompt: 'Create a title'
    })

    expect(update).not.toHaveBeenCalled()
  })

  it('only treats empty first-turn placeholder titles as auto-title candidates', () => {
    expect(shouldAutoGenerateTitle(thread({ title: PENDING_AUTO_THREAD_TITLE }))).toBe(true)
    expect(shouldAutoGenerateTitle(thread({ title: 'New chat' }))).toBe(true)
    expect(shouldAutoGenerateTitle(thread({ title: 'Manual title' }))).toBe(false)
    expect(shouldAutoGenerateTitle(thread({
      turns: [
        turnRecord({ status: 'completed', prompt: 'one' }),
        turnRecord({ id: 'turn_2', status: 'running', prompt: 'two', createdAt: '2026-06-24T00:00:01.000Z' })
      ]
    }))).toBe(false)
  })

  it('cleans generated titles and derives compact fallback titles', () => {
    expect(cleanGeneratedTitle('「排查标题跳动。」')).toBe('排查标题跳动')
    expect(deriveFallbackTitle('# 请分析标题逻辑。后面是细节')).toBe('请分析标题逻辑')
  })
})
