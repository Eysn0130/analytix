import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  readSidebarCollapsedWorkspaces,
  writeSidebarCollapsedWorkspaces
} from './sidebar-collapsed-workspaces'

function memoryStorage(): Storage {
  const store = new Map<string, string>()
  return {
    get length() {
      return store.size
    },
    clear: vi.fn(() => store.clear()),
    getItem: vi.fn((key: string) => store.get(key) ?? null),
    key: vi.fn((index: number) => [...store.keys()][index] ?? null),
    removeItem: vi.fn((key: string) => {
      store.delete(key)
    }),
    setItem: vi.fn((key: string, value: string) => {
      store.set(key, value)
    })
  }
}

describe('sidebar collapsed workspaces', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('persists explicit collapsed and expanded workspace keys', () => {
    const localStorage = memoryStorage()
    vi.stubGlobal('window', {
      localStorage,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn()
    })

    writeSidebarCollapsedWorkspaces({
      '/Users/zxy/project-a': true,
      '/Users/zxy/project-b': false,
      '': true
    })

    expect(readSidebarCollapsedWorkspaces()).toEqual({
      '/Users/zxy/project-a': true,
      '/Users/zxy/project-b': false
    })
  })
})
