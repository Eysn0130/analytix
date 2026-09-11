import { afterEach, describe, expect, it } from 'vitest'
import { DEFAULT_ANALYTIX_MODEL } from '@shared/app-settings'
import {
  WRITE_ASSISTANT_MODEL_KEY,
  normalizeWriteAssistantModel,
  readStoredAssistantModel
} from './write-workspace-store-helpers'

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

describe('write workspace assistant model helpers', () => {
  it('normalizes empty and legacy auto assistant models to the default Analytix model', () => {
    expect(normalizeWriteAssistantModel('')).toBe(DEFAULT_ANALYTIX_MODEL)
    expect(normalizeWriteAssistantModel('auto')).toBe(DEFAULT_ANALYTIX_MODEL)
    expect(normalizeWriteAssistantModel(' AUTO ')).toBe(DEFAULT_ANALYTIX_MODEL)
    expect(normalizeWriteAssistantModel('custom-model')).toBe('custom-model')
  })

  it('migrates the stored legacy auto assistant model', () => {
    const storage = installStorage()
    storage.setItem(WRITE_ASSISTANT_MODEL_KEY, 'auto')

    expect(readStoredAssistantModel()).toBe(DEFAULT_ANALYTIX_MODEL)
    expect(storage.getItem(WRITE_ASSISTANT_MODEL_KEY)).toBe(DEFAULT_ANALYTIX_MODEL)
  })
})
