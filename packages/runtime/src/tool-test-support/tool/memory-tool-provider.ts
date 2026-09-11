import type { CapabilityToolProvider } from './capability-registry.js'
import type { MemoryStore } from '../../memory/memory-store.js'

export function buildMemoryToolProviders(store: MemoryStore | undefined): CapabilityToolProvider[] {
  void store
  return []
}
