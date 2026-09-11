import { useCallback, useState, type Dispatch, type SetStateAction } from 'react'
import { readBrowserStorageItem, writeBrowserStorageItem } from './browser-storage'

function resolveNextValue<T extends string>(next: SetStateAction<T>, current: T): T {
  return typeof next === 'function'
    ? (next as (value: T) => T)(current)
    : next
}

export function readPersistentString<T extends string>(
  key: string,
  fallback: T,
  allowed?: readonly T[]
): T {
  const raw = readBrowserStorageItem(key)
  if (!raw) return fallback
  if (allowed && !allowed.includes(raw as T)) return fallback
  return raw as T
}

export function writePersistentString(key: string, value: string): void {
  writeBrowserStorageItem(key, value)
}

export function usePersistentStringState<T extends string>(
  key: string,
  fallback: T,
  allowed?: readonly T[]
): readonly [T, Dispatch<SetStateAction<T>>] {
  const [value, setValue] = useState<T>(() => readPersistentString(key, fallback, allowed))
  const setPersistentValue = useCallback<Dispatch<SetStateAction<T>>>(
    (next) => {
      setValue((current) => {
        const value = resolveNextValue(next, current)
        if (!allowed || allowed.includes(value)) {
          writePersistentString(key, value)
          return value
        }
        writePersistentString(key, fallback)
        return fallback
      })
    },
    [allowed, fallback, key]
  )
  return [value, setPersistentValue]
}
