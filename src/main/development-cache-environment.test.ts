import { describe, expect, it } from 'vitest'

const {
  DEVELOPMENT_CACHE_DAMAGED_V2_IMAGE,
  DEVELOPMENT_CACHE_ENVIRONMENT,
  DEVELOPMENT_CACHE_IMAGE,
  DEVELOPMENT_CACHE_LEGACY_IMAGE,
  DEVELOPMENT_CACHE_ROOT,
  DEVELOPMENT_CACHE_VOLUME_UUID,
  projectDevelopmentCacheEnvironment,
  projectGoDevelopmentCacheEnvironment
} = require('../../scripts/lib/development-cache-environment.cjs')

describe('development cache environment projection', () => {
  it('binds the recovered v3 cache authority and tracks retired predecessor denylist paths', () => {
    expect(DEVELOPMENT_CACHE_ROOT).toBe('/Volumes/AnalytixCache/development-v3')
    expect(DEVELOPMENT_CACHE_IMAGE).toBe(
      '/Volumes/DataSSD/analytix/AnalytixCache-v3.sparsebundle'
    )
    expect(DEVELOPMENT_CACHE_DAMAGED_V2_IMAGE).toBe(
      '/Volumes/DataSSD/analytix/AnalytixCache-v2.sparsebundle'
    )
    expect(DEVELOPMENT_CACHE_LEGACY_IMAGE).toBe(
      '/Volumes/DataSSD/analytix/AnalytixCache.sparsebundle'
    )
    expect(DEVELOPMENT_CACHE_VOLUME_UUID).toBe(
      '090478FC-CE2B-4E25-98F1-667C3252DD42'
    )
  })

  const exactEnvironment = () => ({
    PATH: '/usr/bin:/bin',
    ANALYTIX_DEV_CACHE_ROOT: DEVELOPMENT_CACHE_ROOT,
    ...DEVELOPMENT_CACHE_ENVIRONMENT
  })

  it('removes only exact Go cache hints at the hermetic boundary', () => {
    const projected = projectDevelopmentCacheEnvironment(
      exactEnvironment(),
      ['GOCACHE', 'GOMODCACHE', 'GOTMPDIR']
    )
    expect(projected).toEqual({
      PATH: '/usr/bin:/bin',
      CARGO_TARGET_DIR: DEVELOPMENT_CACHE_ENVIRONMENT.CARGO_TARGET_DIR
    })
    expect(JSON.stringify(projected)).not.toContain('ANALYTIX_DEV_CACHE_ROOT')
  })

  it('removes only the exact Rust target hint at the hermetic boundary', () => {
    const projected = projectDevelopmentCacheEnvironment(
      exactEnvironment(),
      ['CARGO_TARGET_DIR']
    )
    expect(projected.CARGO_TARGET_DIR).toBeUndefined()
    expect(projected.GOCACHE).toBe(DEVELOPMENT_CACHE_ENVIRONMENT.GOCACHE)
    expect(JSON.stringify(projected)).not.toContain('ANALYTIX_DEV_CACHE_ROOT')
  })

  it('carries only the exact repository-authorized Go module cache across the hermetic boundary', () => {
    expect(projectGoDevelopmentCacheEnvironment(exactEnvironment())).toEqual({
      environment: {
        PATH: '/usr/bin:/bin',
        CARGO_TARGET_DIR: DEVELOPMENT_CACHE_ENVIRONMENT.CARGO_TARGET_DIR
      },
      authorizedModuleCache: DEVELOPMENT_CACHE_ENVIRONMENT.GOMODCACHE
    })
    expect(projectGoDevelopmentCacheEnvironment({ PATH: '/usr/bin:/bin' })).toEqual({
      environment: { PATH: '/usr/bin:/bin' },
      authorizedModuleCache: ''
    })
    expect(() => projectGoDevelopmentCacheEnvironment({
      ...exactEnvironment(),
      GOMODCACHE: ''
    })).toThrow(/path is unavailable: GOMODCACHE/)
    expect(() => projectDevelopmentCacheEnvironment({
      ...exactEnvironment(),
      GOCACHE: ''
    }, ['GOCACHE'])).toThrow(/path is unavailable: GOCACHE/)
  })

  it('keeps unmarked overrides visible to fail-closed compiler contracts', () => {
    expect(projectDevelopmentCacheEnvironment({
      GOCACHE: '/tmp/attacker-cache'
    }, ['GOCACHE'])).toEqual({ GOCACHE: '/tmp/attacker-cache' })
  })

  it('rejects relabelled roots, cache paths, duplicate keys, and unknown projections', () => {
    expect(() => projectDevelopmentCacheEnvironment({
      ...exactEnvironment(),
      ANALYTIX_DEV_CACHE_ROOT: '/tmp/attacker-root'
    }, ['GOCACHE'])).toThrow(/root is not authoritative/)
    expect(() => projectDevelopmentCacheEnvironment({
      ...exactEnvironment(),
      GOCACHE: '/tmp/attacker-cache'
    }, ['GOCACHE'])).toThrow(/GOCACHE/)
    expect(() => projectDevelopmentCacheEnvironment({
      npm_config_cache: '/tmp/a',
      NPM_CONFIG_CACHE: '/tmp/b'
    }, ['GOCACHE'])).toThrow(/Duplicate environment key/)
    expect(() => projectDevelopmentCacheEnvironment(exactEnvironment(), ['GOFLAGS'])).toThrow(
      /Unsupported cache projection/
    )
  })
})
