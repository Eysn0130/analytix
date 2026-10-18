import { afterEach, describe, expect, it } from 'vitest'
import {
  UI_MODE_DEFAULT,
  UI_MODE_MASCOT,
  UI_MODE_STORAGE_KEY,
  readUiModePreference,
  writeUiModePreference
} from './ui-mode'

class MemoryStorage {
  private values = new Map<string, string>()

  getItem(key: string): string | null {
    return this.values.get(key) ?? null
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value)
  }
}

const originalLocalStorage = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')

function installStorage(): MemoryStorage {
  const storage = new MemoryStorage()
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: storage
  })
  return storage
}

function restoreLocalStorage(): void {
  if (originalLocalStorage) {
    Object.defineProperty(globalThis, 'localStorage', originalLocalStorage)
  } else {
    Reflect.deleteProperty(globalThis, 'localStorage')
  }
}

afterEach(() => {
  restoreLocalStorage()
})

describe('ui mode preference', () => {
  it('defaults to the default theme when no preference exists', () => {
    installStorage()

    expect(readUiModePreference()).toBe(UI_MODE_DEFAULT)
  })

  it('ignores the retired mascot preference without rewriting it', () => {
    const storage = installStorage()

    storage.setItem(UI_MODE_STORAGE_KEY, UI_MODE_MASCOT)
    expect(readUiModePreference()).toBe(UI_MODE_DEFAULT)

    expect(storage.getItem(UI_MODE_STORAGE_KEY)).toBe(UI_MODE_MASCOT)
  })

  it('writes theme preferences without modifying retired preferences', () => {
    const storage = installStorage()

    storage.setItem('analytix.mascotMode', '1')
    storage.setItem('analytix.focusMode', '1')

    writeUiModePreference('starlight')

    expect(storage.getItem(UI_MODE_STORAGE_KEY)).toBe('starlight')
    expect(storage.getItem('analytix.mascotMode')).toBe('1')
    expect(storage.getItem('analytix.focusMode')).toBe('1')
  })
})
