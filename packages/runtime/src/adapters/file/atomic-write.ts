import { randomUUID } from 'node:crypto'
import { chmod, mkdir, rename, rm, writeFile } from 'node:fs/promises'
import { dirname } from 'node:path'

export type AtomicWriteFileOptions = {
  /** POSIX mode applied to the replacement and final file. Ignored on Windows. */
  mode?: number
  renameRetry?: {
    attempts?: number
    baseDelayMs?: number
  }
}

const DEFAULT_RENAME_RETRY_ATTEMPTS = 6
const DEFAULT_RENAME_RETRY_BASE_DELAY_MS = 25
const RETRYABLE_RENAME_ERROR_CODES = new Set(['EPERM', 'EACCES', 'EBUSY'])

export async function atomicWriteFile(
  path: string,
  contents: string,
  options: AtomicWriteFileOptions = {}
): Promise<void> {
  await mkdir(dirname(path), { recursive: true })
  const tmp = `${path}.${process.pid}.${Date.now()}.${randomUUID()}.tmp`
  const mode = options.mode
  const writeOptions = mode === undefined
    ? { encoding: 'utf-8' as const }
    : { encoding: 'utf-8' as const, mode }
  try {
    await writeFile(tmp, contents, writeOptions)
    try {
      await renameWithRetry(tmp, path, options.renameRetry)
    } catch (error) {
      if (!shouldFallbackToDirectWrite(error)) {
        throw error
      }
      await writeFile(path, contents, writeOptions)
    }
  } catch (error) {
    await rm(tmp, { force: true }).catch(() => undefined)
    throw error
  }
  if (mode !== undefined && process.platform !== 'win32') {
    await chmod(path, mode)
  }
  await rm(tmp, { force: true }).catch(() => undefined)
}

export async function ensureFileMode(path: string, mode: number): Promise<void> {
  if (process.platform === 'win32') return
  try {
    await chmod(path, mode)
  } catch (error) {
    if (String((error as { code?: unknown })?.code ?? '') === 'ENOENT') return
    throw error
  }
}

async function renameWithRetry(
  from: string,
  to: string,
  options: NonNullable<AtomicWriteFileOptions['renameRetry']> | undefined
): Promise<void> {
  const attempts = Math.max(1, Math.floor(options?.attempts ?? DEFAULT_RENAME_RETRY_ATTEMPTS))
  const baseDelayMs = Math.max(0, Math.floor(options?.baseDelayMs ?? DEFAULT_RENAME_RETRY_BASE_DELAY_MS))

  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      await rename(from, to)
      return
    } catch (error) {
      if (attempt >= attempts || !isRetryableRenameError(error)) {
        throw error
      }
      await delay(baseDelayMs * attempt)
    }
  }
}

function isRetryableRenameError(error: unknown): boolean {
  return RETRYABLE_RENAME_ERROR_CODES.has(String((error as { code?: unknown })?.code ?? ''))
}

function shouldFallbackToDirectWrite(error: unknown): boolean {
  return process.platform === 'win32' && isRetryableRenameError(error)
}

function delay(ms: number): Promise<void> {
  if (ms <= 0) return Promise.resolve()
  return new Promise((resolve) => setTimeout(resolve, ms))
}
