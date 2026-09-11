import type { AppSettingsV1 } from '../../shared/app-settings'
import {
  getRuntimeBaseUrlForSettings,
  runtimeAuthHeaders
} from './analytix-adapter'
import { classifyRuntimeActivity, observeRuntimeIdle } from './runtime-idle-observer'

export type RuntimeThreadsListResult = {
  ok: boolean
  status: number
  body: string
}

export type WaitForRuntimeIdleOptions = {
  settings: AppSettingsV1
  fetchThreads?: (settings: AppSettingsV1) => Promise<RuntimeThreadsListResult>
  sleepMs?: (ms: number) => Promise<void>
  timeoutMs?: number
  intervalMs?: number
}

const DEFAULT_IDLE_TIMEOUT_MS = 10 * 60 * 1000
const DEFAULT_IDLE_POLL_MS = 1_000

export function runtimeThreadsListHasActiveTurn(body: string): boolean {
  return classifyRuntimeActivity(body) === 'active'
}

export async function waitForRuntimeTurnsIdle(
  options: WaitForRuntimeIdleOptions
): Promise<'idle' | 'timeout' | 'unavailable'> {
  const fetchThreads = options.fetchThreads ?? fetchRuntimeThreads
  return observeRuntimeIdle({
    read: () => fetchThreads(options.settings),
    sleep: options.sleepMs ?? ((ms) => new Promise<void>((resolve) => setTimeout(resolve, ms))),
    timeoutMs: options.timeoutMs ?? DEFAULT_IDLE_TIMEOUT_MS,
    intervalMs: options.intervalMs ?? DEFAULT_IDLE_POLL_MS
  })
}

async function fetchRuntimeThreads(settings: AppSettingsV1): Promise<RuntimeThreadsListResult> {
  const url = `${getRuntimeBaseUrlForSettings(settings)}/v1/threads?limit=500&include=side`
  const res = await fetch(url, {
    headers: runtimeAuthHeaders(settings),
    signal: AbortSignal.timeout(5_000)
  })
  return { ok: res.ok, status: res.status, body: await res.text() }
}
