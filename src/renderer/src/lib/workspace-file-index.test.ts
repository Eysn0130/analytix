import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  loadWorkspaceMentionPathSuggestions,
  loadWorkspaceMentionDirectorySuggestions,
  mentionQueryDirectory,
  mergeMentionCandidates
} from './workspace-file-index'
import { filterWorkspaceFileMentionSuggestions, type ComposerFileReference } from './composer-file-references'

function entry(path: string, type: 'file' | 'directory'): {
  name: string
  path: string
  type: 'file' | 'directory'
  ext: string
} {
  const name = path.split('/').pop() ?? path
  const dot = name.lastIndexOf('.')
  return { name, path, type, ext: type === 'file' && dot > 0 ? name.slice(dot) : '' }
}

function installListDirectory(
  impl: (options: { workspaceRoot: string; path?: string }) => unknown
): ReturnType<typeof vi.fn> {
  const fn = vi.fn(async (options: { workspaceRoot: string; path?: string }) => impl(options))
  vi.stubGlobal('window', { analytix: { files: { listDirectory: fn } } })
  return fn
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('mentionQueryDirectory', () => {
  it('returns the directory portion of a path-like query', () => {
    expect(mentionQueryDirectory('src/a/b/c/deep.ts')).toBe('src/a/b/c')
    expect(mentionQueryDirectory('src/a/b/c/')).toBe('src/a/b/c')
    expect(mentionQueryDirectory('src\\a\\b\\file')).toBe('src/a/b')
  })

  it('returns null for a name-only query the index already covers', () => {
    expect(mentionQueryDirectory('deep.ts')).toBeNull()
    expect(mentionQueryDirectory('')).toBeNull()
  })
})

describe('loadWorkspaceMentionPathSuggestions', () => {
  it('lists the deep directory implied by the query and yields mentionable entries', async () => {
    const root = '/ws-deep-1'
    const list = installListDirectory(() => ({
      ok: true,
      root: `${root}/src/a/b/c`,
      entries: [
        entry(`${root}/src/a/b/c/deep.ts`, 'file'),
        entry(`${root}/src/a/b/c/nested`, 'directory'),
        entry(`${root}/src/a/b/c/image.png`, 'file'), // not a mentionable text file
        entry(`${root}/src/a/b/c/node_modules`, 'directory') // ignored dir
      ]
    }))

    const suggestions = await loadWorkspaceMentionPathSuggestions(root, 'src/a/b/c/de')

    expect(list).toHaveBeenCalledWith({ workspaceRoot: root, path: 'src/a/b/c' })
    expect(suggestions.map((ref) => ref.relativePath)).toEqual([
      'src/a/b/c/deep.ts',
      'src/a/b/c/nested'
    ])
  })

  it('returns [] for name-only queries without hitting the IPC', async () => {
    const list = installListDirectory(() => ({ ok: true, root: '/ws', entries: [] }))
    expect(await loadWorkspaceMentionPathSuggestions('/ws-name-only', 'deep.ts')).toEqual([])
    expect(list).not.toHaveBeenCalled()
  })

  it('returns [] when the directory cannot be listed', async () => {
    installListDirectory(() => ({ ok: false, message: 'not found' }))
    expect(await loadWorkspaceMentionPathSuggestions('/ws-missing', 'no/such/dir/x')).toEqual([])
  })

  it('makes a deeply nested file selectable through fuzzy filtering', async () => {
    const root = '/ws-deep-2'
    installListDirectory(() => ({
      ok: true,
      root: `${root}/src/a/b/c/d/e/f/g`,
      entries: [entry(`${root}/src/a/b/c/d/e/f/g/VeryDeep.ts`, 'file')]
    }))

    const onDemand = await loadWorkspaceMentionPathSuggestions(root, 'src/a/b/c/d/e/f/g/VeryDeep')
    // The bounded index (empty here, mimicking a file past the depth cap) plus
    // the on-demand directory listing must surface the deep file.
    const candidates = mergeMentionCandidates([], onDemand)
    const filtered = filterWorkspaceFileMentionSuggestions(candidates, 'src/a/b/c/d/e/f/g/VeryDeep', [])
    expect(filtered.map((ref) => ref.relativePath)).toContain('src/a/b/c/d/e/f/g/VeryDeep.ts')
  })
})

describe('drilled workspace directory candidates', () => {
  it('lists only actual direct children, retaining directory type and the existing file filter', async () => {
    const root = '/ws-drill-directory'
    const list = installListDirectory(({ path }) => ({ ok: true, entries: path === 'src' ? [
      entry(`${root}/src/nested`, 'directory'), entry(`${root}/src/note.md`, 'file'),
      entry(`${root}/src/image.png`, 'file'), entry(`${root}/src/node_modules`, 'directory')
    ] : [] }))
    const references = await loadWorkspaceMentionDirectorySuggestions(root, 'src/')
    expect(list).toHaveBeenCalledExactlyOnceWith({ workspaceRoot: root, path: 'src' })
    expect(references).toEqual([
      { path: `${root}/src/nested`, relativePath: 'src/nested', name: 'nested', type: 'directory' },
      { path: `${root}/src/note.md`, relativePath: 'src/note.md', name: 'note.md', type: 'file' }
    ])
  })

  it('returns to the workspace root through a real directory listing', async () => {
    const root = '/ws-drill-root'
    const list = installListDirectory(() => ({ ok: true, entries: [entry(`${root}/src`, 'directory')] }))
    expect((await loadWorkspaceMentionDirectorySuggestions(root, '')).map(reference => reference.relativePath)).toEqual(['src'])
    expect(list).toHaveBeenCalledExactlyOnceWith({ workspaceRoot: root, path: root })
  })
})

describe('mergeMentionCandidates', () => {
  it('appends new references and de-dupes by relative path', () => {
    const base: ComposerFileReference[] = [
      { path: '/ws/a.ts', relativePath: 'a.ts', name: 'a.ts', type: 'file' }
    ]
    const extra: ComposerFileReference[] = [
      { path: '/ws/a.ts', relativePath: 'a.ts', name: 'a.ts', type: 'file' },
      { path: '/ws/deep/b.ts', relativePath: 'deep/b.ts', name: 'b.ts', type: 'file' }
    ]
    const merged = mergeMentionCandidates(base, extra)
    expect(merged.map((ref) => ref.relativePath)).toEqual(['a.ts', 'deep/b.ts'])
  })

  it('returns the base array unchanged when there is nothing to merge', () => {
    const base: ComposerFileReference[] = [
      { path: '/ws/a.ts', relativePath: 'a.ts', name: 'a.ts', type: 'file' }
    ]
    expect(mergeMentionCandidates(base, [])).toBe(base)
  })
})
