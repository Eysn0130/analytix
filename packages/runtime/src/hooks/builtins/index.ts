import type { ResolvedHook } from '../hook-engine.js'
import type { QualityConfig } from '../../config/analytix-config.js'
import { buildDesignQualityHook } from './design-quality-hook.js'

export type BuiltinHookInput = {
  quality?: QualityConfig
}

export function buildBuiltinHooks(input: BuiltinHookInput): ResolvedHook[] {
  const hooks: ResolvedHook[] = []
  if (input.quality) {
    const designQuality = buildDesignQualityHook(input.quality)
    if (designQuality) hooks.push(designQuality)
  }
  return hooks
}

export { buildDesignQualityHook, matchesGlob } from './design-quality-hook.js'
