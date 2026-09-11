import { readFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  AnalytixConfigSchema,
  DEFAULT_ANALYTIX_MODEL,
  expandHomePath,
  RuntimeTuningConfigSchema
} from './analytix-config.js'

describe('AnalytixConfigSchema defaults', () => {
  it('defaults fresh runtime launches to DeepSeek v4 flash', () => {
    expect(DEFAULT_ANALYTIX_MODEL).toBe('deepseek-v4-flash')
  })
})

describe('RuntimeTuningConfigSchema streamIdleTimeoutMs', () => {
  it('accepts a custom timeout, including 0 to disable the guard', () => {
    expect(RuntimeTuningConfigSchema.safeParse({ streamIdleTimeoutMs: 300_000 }).success).toBe(true)
    expect(RuntimeTuningConfigSchema.safeParse({ streamIdleTimeoutMs: 0 }).success).toBe(true)
  })

  it('rejects negative or fractional timeouts', () => {
    expect(RuntimeTuningConfigSchema.safeParse({ streamIdleTimeoutMs: -1 }).success).toBe(false)
    expect(RuntimeTuningConfigSchema.safeParse({ streamIdleTimeoutMs: 1.5 }).success).toBe(false)
  })
})

describe('RuntimeTuningConfigSchema stepLimits', () => {
  it('accepts non-negative step limits', () => {
    expect(RuntimeTuningConfigSchema.safeParse({
      stepLimits: {
        defaultMaxModelSteps: 0,
        userGlobalMaxModelSteps: 64,
        plannerMaxModelSteps: 12,
        headlessMaxModelSteps: 25
      }
    }).success).toBe(true)
  })

  it('rejects negative and fractional step limits', () => {
    expect(RuntimeTuningConfigSchema.safeParse({
      stepLimits: { defaultMaxModelSteps: -1 }
    }).success).toBe(false)
    expect(RuntimeTuningConfigSchema.safeParse({
      stepLimits: { userGlobalMaxModelSteps: 1.5 }
    }).success).toBe(false)
  })
})

describe('AnalytixConfigSchema auto-plan boundary', () => {
  it('rejects Reasonix auto-plan config roots', () => {
    expect(AnalytixConfigSchema.safeParse({
      agent: { auto_plan: true }
    }).success).toBe(false)
    expect(AnalytixConfigSchema.safeParse({
      runtime: { auto_plan: true }
    }).success).toBe(false)
    expect(AnalytixConfigSchema.safeParse({
      serve: { autoPlan: true }
    }).success).toBe(false)
  })
})

describe('runtime config example', () => {
  it('contains only currently advertised production config families', () => {
    const example = JSON.parse(
      readFileSync(new URL('../../config.example.json', import.meta.url), 'utf8')
    ) as Record<string, unknown>
    const serve = example.serve as Record<string, unknown>
    const capabilities = example.capabilities as Record<string, unknown>

    expect(AnalytixConfigSchema.safeParse(example).success).toBe(true)
    expect(Object.keys(example).sort()).toEqual([
      'capabilities',
      'modelProviders',
      'runtime',
      'serve'
    ])
    expect(Object.keys(serve).sort()).toEqual([
      'approvalPolicy',
      'baseUrl',
      'dataDir',
      'endpointFormat',
      'host',
      'insecure',
      'model',
      'port',
      'sandboxMode'
    ])
    expect(Object.keys(capabilities).sort()).toEqual([
      'mcp',
      'skills',
      'subagents',
      'visionBridge',
      'web'
    ])
    expect(example).not.toHaveProperty('contextCompaction')
    expect(example).not.toHaveProperty('models')
    expect(example).not.toHaveProperty('quality')
    expect(example).not.toHaveProperty('hooks')
    expect(serve).not.toHaveProperty('apiKey')
    expect(serve).not.toHaveProperty('runtimeToken')
    expect(serve).not.toHaveProperty('storage')
    expect(serve).not.toHaveProperty('tokenEconomy')
    expect(serve).not.toHaveProperty('tokenEconomyMode')
    expect(capabilities).not.toHaveProperty('memory')
  })
})

describe('expandHomePath', () => {
  it('expands Windows-style home-relative paths', () => {
    expect(expandHomePath('~\\analytix\\config.json')).toBe(join(homedir(), 'analytix', 'config.json'))
  })

  it('leaves non-home tilde prefixes untouched', () => {
    expect(expandHomePath('~other/config.json')).toBe('~other/config.json')
  })
})
