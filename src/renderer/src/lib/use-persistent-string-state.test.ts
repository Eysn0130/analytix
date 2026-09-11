import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  readPersistentString,
  writePersistentString
} from './use-persistent-string-state'

function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() {
      return values.size
    },
    clear: () => values.clear(),
    getItem: (key: string) => values.get(key) ?? null,
    key: (index: number) => [...values.keys()][index] ?? null,
    removeItem: (key: string) => {
      values.delete(key)
    },
    setItem: (key: string, value: string) => {
      values.set(key, value)
    }
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('usePersistentStringState', () => {
  it('reads and writes persistent string values', () => {
    const storage = memoryStorage()
    vi.stubGlobal('localStorage', storage)
    writePersistentString('analytix.test.tab', 'skill')

    expect(readPersistentString('analytix.test.tab', 'plugin', ['plugin', 'skill'])).toBe('skill')
  })

  it('falls back when stored value is outside the allowed set', () => {
    const storage = memoryStorage()
    vi.stubGlobal('localStorage', storage)
    storage.setItem('analytix.test.filter', 'unknown')

    expect(readPersistentString('analytix.test.filter', 'all', ['all', 'enabled'])).toBe('all')
  })
})
