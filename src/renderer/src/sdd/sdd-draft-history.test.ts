import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { WorkspaceDirectoryListResult, WorkspaceDirectoryTarget, WorkspaceFileTarget } from '@shared/workspace-file'
import { createSddDraft, useSddDraftStore } from './sdd-draft-store'
import { listSddDraftHistory, titleFromSddDraftContent } from './sdd-draft-history'

function createMemoryStorage(): Storage {
  const items = new Map<string, string>()
  return {
    get length() {
      return items.size
    },
    clear: () => items.clear(),
    getItem: (key) => items.get(key) ?? null,
    key: (index) => [...items.keys()][index] ?? null,
    removeItem: (key) => {
      items.delete(key)
    },
    setItem: (key, value) => {
      items.set(key, value)
    }
  }
}

function directory(entries: Array<{ name: string, type: 'file' | 'directory', path?: string }>): WorkspaceDirectoryListResult {
  return {
    ok: true,
    root: '/tmp/app/.analytixsdd/requirements',
    entries: entries.map((entry) => ({
      name: entry.name,
      path: entry.path ?? `/tmp/app/.analytixsdd/requirements/${entry.name}`,
      type: entry.type,
      ext: entry.type === 'file' ? '.md' : ''
    }))
  }
}

describe('sdd-draft-history', () => {
  beforeEach(() => {
    vi.useRealTimers()
    vi.stubGlobal('localStorage', createMemoryStorage())
    vi.stubGlobal('window', { localStorage })
    useSddDraftStore.getState().clearActiveDraft()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    useSddDraftStore.getState().clearActiveDraft()
  })

  it('extracts a friendly title from the first meaningful heading', () => {
    expect(titleFromSddDraftContent('\n# Export flow {draft}\n\nbody', 'fallback')).toBe('Export flow')
    expect(titleFromSddDraftContent('\n\nplain first line\nbody', 'fallback')).toBe('plain first line')
    expect(titleFromSddDraftContent('', 'fallback')).toBe('fallback')
  })

  it('discovers disk-only requirement drafts under .analytixsdd/requirements', async () => {
    const draftId = '123e4567-e89b-12d3-a456-426614174000'
    const listWorkspaceDirectory = vi.fn(async (options: WorkspaceDirectoryTarget) => {
      if (options.path === '.analytixsdd/requirements') {
        return directory([
          { name: draftId, type: 'directory' },
          { name: 'not-a-draft', type: 'directory' },
          { name: 'requirement.md', type: 'file' }
        ])
      }
      if (options.path === `.analytixsdd/requirements/${draftId}`) {
        return directory([
          {
            name: 'requirement.md',
            type: 'file',
            path: `/tmp/app/.analytixsdd/requirements/${draftId}/requirement.md`
          }
        ])
      }
      return { ok: false as const, message: 'missing' }
    })
    const readWorkspaceFile = vi.fn(async (_options: WorkspaceFileTarget) => ({
      ok: true as const,
      path: `/tmp/app/.analytixsdd/requirements/${draftId}/requirement.md`,
      content: '# Payment requirement\n\n## Goal',
      size: 28,
      truncated: false
    }))

    const history = await listSddDraftHistory({
      workspaceRoot: '/tmp/app',
      listWorkspaceDirectory,
      readWorkspaceFile
    })

    expect(history).toHaveLength(1)
    expect(history[0]).toMatchObject({
      title: 'Payment requirement',
      relativePath: `.analytixsdd/requirements/${draftId}/requirement.md`,
      searchText: '# Payment requirement\n\n## Goal',
      source: 'disk'
    })
  })

  it('includes Requirement AI thread ids from chat meta', async () => {
    const draftId = '123e4567-e89b-12d3-a456-426614174000'
    const listWorkspaceDirectory = vi.fn(async (options: WorkspaceDirectoryTarget) => {
      if (options.path === '.analytixsdd/requirements') {
        return directory([{ name: draftId, type: 'directory' }])
      }
      if (options.path === `.analytixsdd/requirements/${draftId}`) {
        return directory([{
          name: 'requirement.md',
          type: 'file',
          path: `/tmp/app/.analytixsdd/requirements/${draftId}/requirement.md`
        }])
      }
      return { ok: false as const, message: 'missing' }
    })
    const readWorkspaceFile = vi.fn(async (options: WorkspaceFileTarget) => {
      if (options.path?.endsWith('/chat/meta.json')) {
        return {
          ok: true as const,
          path: `/tmp/app/${options.path}`,
          content: JSON.stringify({
            version: 1,
            primaryThreadId: 'thread-sdd-primary',
            threads: [
              { id: 'thread-sdd-primary' },
              { id: 'thread-sdd-previous' }
            ]
          }),
          size: 120,
          truncated: false
        }
      }
      return {
        ok: true as const,
        path: `/tmp/app/.analytixsdd/requirements/${draftId}/requirement.md`,
        content: '# Payment requirement',
        size: 21,
        truncated: false
      }
    })

    const history = await listSddDraftHistory({
      workspaceRoot: '/tmp/app',
      listWorkspaceDirectory,
      readWorkspaceFile
    })

    expect(history[0]?.chatThreadIds).toEqual(['thread-sdd-primary', 'thread-sdd-previous'])
  })

  it('merges remembered and disk drafts and prefers unsaved remembered titles', async () => {
    const draft = createSddDraft({
      id: '123e4567-e89b-12d3-a456-426614174000',
      workspaceRoot: '/tmp/app',
      now: 1
    })
    useSddDraftStore.getState().setActiveDraft(draft, '# Disk title')
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-02T03:04:05.000Z'))
    useSddDraftStore.getState().setContent('# Local unsaved title')
    useSddDraftStore.getState().clearActiveDraft()

    const listWorkspaceDirectory = vi.fn(async (options: WorkspaceDirectoryTarget) => {
      if (options.path === '.analytixsdd/requirements') {
        return directory([{ name: '123e4567-e89b-12d3-a456-426614174000', type: 'directory' }])
      }
      return directory([{
        name: 'requirement.md',
        type: 'file',
        path: '/tmp/app/.analytixsdd/requirements/123e4567-e89b-12d3-a456-426614174000/requirement.md'
      }])
    })
    const readWorkspaceFile = vi.fn(async () => ({
      ok: true as const,
      path: '/tmp/app/.analytixsdd/requirements/123e4567-e89b-12d3-a456-426614174000/requirement.md',
      content: '# Disk title',
      size: 12,
      truncated: false
    }))

    const history = await listSddDraftHistory({
      workspaceRoot: '/tmp/app',
      listWorkspaceDirectory,
      readWorkspaceFile
    })

    expect(history).toHaveLength(1)
    expect(history[0]).toMatchObject({
      id: draft.id,
      title: 'Local unsaved title',
      searchText: '# Local unsaved title',
      source: 'remembered'
    })
  })

  it('returns an empty history when the SDD draft directory is missing', async () => {
    await expect(listSddDraftHistory({
      workspaceRoot: '/tmp/app',
      listWorkspaceDirectory: vi.fn(async () => ({ ok: false as const, message: 'missing' })),
      readWorkspaceFile: vi.fn()
    })).resolves.toEqual([])
  })
})
