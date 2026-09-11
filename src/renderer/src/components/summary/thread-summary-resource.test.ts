import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { CoreThreadSummaryResponseJson } from '../../agent/analytix-contract'

const getProviderMock = vi.hoisted(() => vi.fn())

vi.mock('../../agent/registry', () => ({
  getProvider: getProviderMock
}))

import {
  invalidateSharedThreadSummary,
  querySharedThreadSummary,
  readCachedSharedThreadSummary
} from './thread-summary-resource'

function summary(threadId: string): CoreThreadSummaryResponseJson {
  return {
    threadId,
    generatedAt: '2026-06-30T00:00:00.000Z',
    latestSeq: 1,
    subagents: [],
    tasks: [],
    outputs: [],
    sources: [],
    sideChats: [],
    backgroundProcesses: []
  }
}

describe('shared thread summary cache', () => {
  beforeEach(() => {
    invalidateSharedThreadSummary()
    getProviderMock.mockReset()
  })

  afterEach(() => {
    invalidateSharedThreadSummary()
  })

  it('never stores a summary returned for another thread', async () => {
    const getThreadSummary = vi.fn(async () => summary('thr_foreign'))
    getProviderMock.mockReturnValue({ getThreadSummary })

    await expect(querySharedThreadSummary('thr_expected', { force: true })).rejects.toThrow(
      'Thread summary identity mismatch.'
    )

    expect(getThreadSummary).toHaveBeenCalledWith('thr_expected')
    expect(readCachedSharedThreadSummary('thr_expected')).toBeNull()
    expect(readCachedSharedThreadSummary('thr_foreign')).toBeNull()
  })

  it('reuses only the matching thread projection', async () => {
    const expected = summary('thr_expected')
    const getThreadSummary = vi.fn(async () => expected)
    getProviderMock.mockReturnValue({ getThreadSummary })

    await expect(querySharedThreadSummary('thr_expected')).resolves.toBe(expected)
    await expect(querySharedThreadSummary('thr_expected')).resolves.toBe(expected)

    expect(getThreadSummary).toHaveBeenCalledTimes(1)
    expect(readCachedSharedThreadSummary('thr_expected')).toBe(expected)
  })
})
