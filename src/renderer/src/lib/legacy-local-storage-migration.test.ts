import { describe, expect, it } from 'vitest'
import { migrateLegacyLocalStorageKeys } from './legacy-local-storage-migration'

function memoryStorage(seed: Record<string, string> = {}): Storage {
  const items = new Map(Object.entries(seed))
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

describe('legacy localStorage migration', () => {
  it('leaves existing analytix keys untouched', () => {
    const storage = memoryStorage({
      'analytix.plan.registry.v1': '{"plans":{}}',
      'analytix.turnModelLabel': '{"thread|item":"deepseek-chat"}'
    })

    expect(migrateLegacyLocalStorageKeys(storage)).toBe(0)
    expect(storage.getItem('analytix.plan.registry.v1')).toBe('{"plans":{}}')
    expect(storage.getItem('analytix.turnModelLabel')).toBe('{"thread|item":"deepseek-chat"}')
  })

  it('does not auto-copy legacy desktop keys into analytix', () => {
    const storage = memoryStorage({
      'analytix.plan.registry.v1': 'current'
    })

    expect(migrateLegacyLocalStorageKeys(storage)).toBe(0)
    expect(storage.getItem('analytix.plan.registry.v1')).toBe('current')
  })
})
