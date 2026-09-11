import { afterEach, describe, expect, it } from 'vitest'
import {
  UI_MODE_DEFAULT,
  UI_MODE_MASCOT,
  UI_MODE_STORAGE_KEY,
  readUiModePreference,
  writeUiModePreference
} from './ui-mode'
import { MASCOT_MODE_STORAGE_KEY } from './mascot-mode'

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
  it('defaults to the Xiezhi/default mode when no preference exists', () => {
    installStorage()

    expect(readUiModePreference()).toBe(UI_MODE_DEFAULT)
  })

  it('migrates the retired mascot preference back to default', () => {
    const storage = installStorage()

    storage.setItem(UI_MODE_STORAGE_KEY, UI_MODE_MASCOT)
    expect(readUiModePreference()).toBe(UI_MODE_DEFAULT)

    storage.setItem(UI_MODE_STORAGE_KEY, '')
    storage.setItem(MASCOT_MODE_STORAGE_KEY, '1')
    expect(readUiModePreference()).toBe(UI_MODE_DEFAULT)
  })

  it('writes non-mascot modes while keeping the legacy mascot flag off', () => {
    const storage = installStorage()

    writeUiModePreference('starlight')

    expect(storage.getItem(UI_MODE_STORAGE_KEY)).toBe('starlight')
    expect(storage.getItem(MASCOT_MODE_STORAGE_KEY)).toBe('0')
  })
})
