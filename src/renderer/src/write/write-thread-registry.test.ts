import { describe, expect, it } from 'vitest'
import type { NormalizedThread } from '../agent/types'
import {
  MAX_WRITE_THREAD_IDS_PER_WORKSPACE,
  MAX_WRITE_THREAD_REGISTRY_WORKSPACES,
  WRITE_ASSISTANT_THREAD_TITLE,
  emptyWriteThreadRegistry,
  forgetWriteThread,
  hydrateWriteThreadRegistry,
  isWriteThreadId,
  normalizeWriteThreadRegistry,
  pruneWriteThreadRegistry,
  readWriteThreadRegistry,
  saveWriteThreadRegistry,
  writeWorkspaceForThreadId
} from './write-thread-registry'

class MemoryStorage {
  private values = new Map<string, string>()

  getItem(key: string): string | null {
    return this.values.get(key) ?? null
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value)
  }
}

function thread(id: string, workspace: string): NormalizedThread {
  return {
    id,
    title: id,
    updatedAt: '2026-05-24T00:00:00.000Z',
    model: 'auto',
    mode: 'agent',
    workspace
  }
}

describe('write-thread-registry', () => {
  it('saves and restores write thread records by workspace', () => {
    const storage = new MemoryStorage()
    const registry = normalizeWriteThreadRegistry({ workspaces: {
      '/Users/zxy/workspace': { activeThreadId: 'thread-1', threadIds: ['thread-1'] }
    } })
    saveWriteThreadRegistry(registry, storage)

    const restored = readWriteThreadRegistry(storage)
    expect(isWriteThreadId('thread-1', restored)).toBe(true)
    expect(writeWorkspaceForThreadId('thread-1', restored)).toBe('/Users/zxy/workspace')
  })

  it('preserves the stored active legacy thread and normalizes duplicate ids', () => {
    const registry = normalizeWriteThreadRegistry({ workspaces: {
      '/Users/zxy/workspace': { activeThreadId: 'thread-2', threadIds: ['thread-1', 'thread-2', 'thread-1'] }
    } })

    expect(registry.workspaces['/Users/zxy/workspace']).toEqual({
      activeThreadId: 'thread-2', threadIds: ['thread-2', 'thread-1']
    })
  })

  it('caps remembered write thread ids per workspace', () => {
    const registry = normalizeWriteThreadRegistry({ workspaces: {
      '/Users/zxy/write': {
        activeThreadId: `thread-${MAX_WRITE_THREAD_IDS_PER_WORKSPACE + 4}`,
        threadIds: Array.from({ length: MAX_WRITE_THREAD_IDS_PER_WORKSPACE + 5 }, (_, index) =>
          `thread-${MAX_WRITE_THREAD_IDS_PER_WORKSPACE + 4 - index}`)
      }
    } })

    const record = registry.workspaces['/Users/zxy/write']
    expect(record.activeThreadId).toBe(`thread-${MAX_WRITE_THREAD_IDS_PER_WORKSPACE + 4}`)
    expect(record.threadIds).toHaveLength(MAX_WRITE_THREAD_IDS_PER_WORKSPACE)
    expect(record.threadIds).not.toContain('thread-0')
    expect(record.threadIds).not.toContain('thread-4')
    expect(record.threadIds).toContain('thread-5')
  })

  it('caps restored legacy workspaces while preserving the latest stored records', () => {
    const workspaces = Object.fromEntries(
      Array.from({ length: MAX_WRITE_THREAD_REGISTRY_WORKSPACES + 1 }, (_, index) => [
        `/Users/zxy/write-${index}`,
        { activeThreadId: `thread-${index}`, threadIds: [`thread-${index}`] }
      ])
    )
    const registry = normalizeWriteThreadRegistry({ workspaces })

    expect(Object.keys(registry.workspaces)).toHaveLength(MAX_WRITE_THREAD_REGISTRY_WORKSPACES)
    expect(registry.workspaces['/Users/zxy/write-0']).toBeUndefined()
    expect(registry.workspaces['/Users/zxy/write-1']?.activeThreadId).toBe('thread-1')
    expect(registry.workspaces[`/Users/zxy/write-${MAX_WRITE_THREAD_REGISTRY_WORKSPACES}`]?.activeThreadId).toBe(
      `thread-${MAX_WRITE_THREAD_REGISTRY_WORKSPACES}`
    )
  })

  it('prunes missing runtime threads and forgets deleted threads', () => {
    const registry = normalizeWriteThreadRegistry({ workspaces: {
      '/Users/zxy/workspace': { activeThreadId: 'thread-2', threadIds: ['thread-2', 'thread-1'] }
    } })
    const pruned = pruneWriteThreadRegistry([thread('thread-1', '/Users/zxy/workspace')], registry)

    expect(isWriteThreadId('thread-2', pruned)).toBe(false)
    expect(pruned.workspaces['/Users/zxy/workspace'].activeThreadId).toBe('thread-1')
    expect(forgetWriteThread('thread-1', pruned).workspaces['/Users/zxy/workspace']).toBeUndefined()
  })

  it('hydrates leaked write assistant threads from configured write workspaces', () => {
    const leaked = {
      ...thread('write-thread', '/Users/zxy/.analytix/write_workspace'),
      title: WRITE_ASSISTANT_THREAD_TITLE
    }
    const normalCodeThread = {
      ...thread('code-thread', '/Users/zxy/.analytix/write_workspace'),
      title: 'Explain this project'
    }
    const sameTitleElsewhere = {
      ...thread('elsewhere', '/Users/zxy/code/project'),
      title: WRITE_ASSISTANT_THREAD_TITLE
    }

    const registry = hydrateWriteThreadRegistry(
      [leaked, normalCodeThread, sameTitleElsewhere],
      ['/Users/zxy/.analytix/write_workspace'],
      emptyWriteThreadRegistry()
    )

    expect(isWriteThreadId('write-thread', registry)).toBe(true)
    expect(isWriteThreadId('code-thread', registry)).toBe(false)
    expect(isWriteThreadId('elsewhere', registry)).toBe(false)
  })

  it('hydrates legacy tilde write assistant threads under the configured absolute workspace', () => {
    const legacyThread = {
      ...thread('legacy-write-thread', '~/.analytix/write_workspace'),
      title: WRITE_ASSISTANT_THREAD_TITLE
    }

    const registry = hydrateWriteThreadRegistry(
      [legacyThread],
      ['/Users/zxy/.analytix/write_workspace'],
      emptyWriteThreadRegistry()
    )

    expect(isWriteThreadId('legacy-write-thread', registry)).toBe(true)
    expect(registry.workspaces['/Users/zxy/.analytix/write_workspace'].threadIds).toEqual([
      'legacy-write-thread'
    ])
    expect(writeWorkspaceForThreadId('legacy-write-thread', registry)).toBe('/Users/zxy/.analytix/write_workspace')
  })

  it('hydrates Reasonix write-context threads even when the session list reports the default workspace', () => {
    const leaked = {
      ...thread('reasonix-write-thread', '/Users/zxy/.analytix/default_workspace'),
      title: '[写作上下文] 交互限制：当前 GUI 无法提交 request_user_input'
    }

    const registry = hydrateWriteThreadRegistry(
      [leaked],
      ['/Users/zxy/.analytix/write_workspace'],
      emptyWriteThreadRegistry()
    )

    expect(isWriteThreadId('reasonix-write-thread', registry)).toBe(true)
    expect(writeWorkspaceForThreadId('reasonix-write-thread', registry)).toBe('/Users/zxy/.analytix/write_workspace')
  })

  it('preserves the active write thread while adding newly inferred thread ids', () => {
    const existing = normalizeWriteThreadRegistry({ workspaces: {
      '/Users/zxy/write': { activeThreadId: 'existing-thread', threadIds: ['existing-thread'] }
    } })
    const registry = hydrateWriteThreadRegistry(
      [
        {
          ...thread('newer-thread', '/Users/zxy/write'),
          title: WRITE_ASSISTANT_THREAD_TITLE,
          updatedAt: '2026-05-25T00:00:00.000Z'
        }
      ],
      ['/Users/zxy/write'],
      existing
    )

    expect(registry.workspaces['/Users/zxy/write'].activeThreadId).toBe('existing-thread')
    expect(registry.workspaces['/Users/zxy/write'].threadIds).toEqual([
      'existing-thread',
      'newer-thread'
    ])
  })

  it('preserves archived legacy history through hydration, storage and pruning', () => {
    const storage = new MemoryStorage()
    const archivedThread = {
      ...thread('archived-thread', '/Users/zxy/write'),
      title: WRITE_ASSISTANT_THREAD_TITLE,
      archived: true
    }
    const registry = hydrateWriteThreadRegistry([archivedThread], ['/Users/zxy/write'], emptyWriteThreadRegistry())
    saveWriteThreadRegistry(registry, storage)
    const restored = pruneWriteThreadRegistry([archivedThread], readWriteThreadRegistry(storage))

    expect(isWriteThreadId('archived-thread', restored)).toBe(true)
    expect(writeWorkspaceForThreadId('archived-thread', restored)).toBe('/Users/zxy/write')
    expect(forgetWriteThread('archived-thread', restored).workspaces).toEqual({})
  })
})
