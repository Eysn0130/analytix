import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentProvider, NormalizedThread } from '../agent/types'
import {
  invalidateThreadDetailCache,
  loadThreadDetailWithCache,
  prewarmThreadDetails
} from './thread-detail-cache'

function thread(overrides: Partial<NormalizedThread> & Pick<NormalizedThread, 'id'>): NormalizedThread {
  return {
    id: overrides.id,
    title: overrides.title ?? overrides.id,
    updatedAt: overrides.updatedAt ?? '2026-06-01T00:00:00.000Z',
    model: overrides.model ?? 'reasonix',
    mode: overrides.mode ?? 'agent',
    workspace: overrides.workspace ?? '/tmp/project',
    status: overrides.status ?? 'idle',
    ...(overrides.historyAuthority ? { historyAuthority: overrides.historyAuthority } : {})
  }
}

function provider(): AgentProvider {
  return {
    getThreadDetail: vi.fn(async (threadId: string) => ({
      blocks: [{ kind: 'assistant' as const, id: `${threadId}-block`, text: 'ready' }],
      latestSeq: 1
    }))
  } as unknown as AgentProvider
}

describe('thread detail cache', () => {
  beforeEach(() => {
    invalidateThreadDetailCache()
  })

  it('prewarms stable thread details and reuses the cached snapshot', async () => {
    const p = provider()
    const summary = thread({ id: 'thr_cached' })

    await prewarmThreadDetails(p, [summary])
    const detail = await loadThreadDetailWithCache(p, summary.id, summary)

    expect(detail.blocks).toHaveLength(1)
    expect(p.getThreadDetail).toHaveBeenCalledTimes(1)
  })

  it('reloads when the sidebar summary version changes', async () => {
    const p = provider()
    const summary = thread({ id: 'thr_changed', updatedAt: '2026-06-01T00:00:00.000Z' })

    await prewarmThreadDetails(p, [summary])
    await loadThreadDetailWithCache(p, summary.id, {
      ...summary,
      updatedAt: '2026-06-02T00:00:00.000Z'
    })

    expect(p.getThreadDetail).toHaveBeenCalledTimes(2)
  })

  it('does not cache running threads', async () => {
    const p = provider()
    const summary = thread({ id: 'thr_running', status: 'running' })

    await loadThreadDetailWithCache(p, summary.id, summary)
    await loadThreadDetailWithCache(p, summary.id, summary)

    expect(p.getThreadDetail).toHaveBeenCalledTimes(2)
  })

  it('never prewarms or caches case-boundary thread details', async () => {
    const p = provider()
    const summary = thread({
      id: 'thr_case',
      historyAuthority: 'case_boundary_only_v1'
    })

    await prewarmThreadDetails(p, [summary])
    await loadThreadDetailWithCache(p, summary.id, summary)
    await loadThreadDetailWithCache(p, summary.id, summary)

    expect(p.getThreadDetail).toHaveBeenCalledTimes(2)
  })
})
