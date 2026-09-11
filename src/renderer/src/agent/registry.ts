import type { AgentProvider } from './types'
import { AnalytixRuntimeProvider } from './analytix-runtime'

let cachedProvider: AgentProvider | null = null

export function getProvider(): AgentProvider {
  if (cachedProvider) return cachedProvider
  cachedProvider = new AnalytixRuntimeProvider()
  return cachedProvider
}

export function resetProviderCacheForTests(): void {
  cachedProvider = null
}
