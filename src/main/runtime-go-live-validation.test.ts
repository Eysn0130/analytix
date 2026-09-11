import { spawnSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

function runProviderSettingsSelfTest(): {
  status: number
  stdout: string
  report: {
    id: string
    status: string
    provider?: { activeProviderId?: string; providerId?: string; model?: string; hasApiKey?: boolean }
    runtimeProviderOverride?: { activeProviderId?: string; providerId?: string; model?: string; hasApiKey?: boolean }
    nonDeepSeekProvider?: { providerId?: string; endpointFormat?: string; hasApiKey?: boolean; providerIsDeepSeek?: boolean }
    presetNonDeepSeekProvider?: { providerId?: string; model?: string; baseUrl?: string; endpointFormat?: string; hasApiKey?: boolean; providerIsDeepSeek?: boolean }
    multiNonDeepSeekProvider?: { providerId?: string; model?: string; baseUrl?: string; endpointFormat?: string; hasApiKey?: boolean; providerIsDeepSeek?: boolean }
    tokenPlanNonDeepSeekProvider?: { providerId?: string; model?: string; baseUrl?: string; endpointFormat?: string; hasApiKey?: boolean; providerIsDeepSeek?: boolean }
    deepSeekProviderFromProfiles?: { activeProviderId?: string; providerId?: string; model?: string; endpointFormat?: string; hasApiKey?: boolean; providerIsDeepSeek?: boolean }
    modelOverrideNonDeepSeekProvider?: { providerId?: string; model?: string; baseUrl?: string; endpointFormat?: string; hasApiKey?: boolean; providerIsDeepSeek?: boolean; requestUrl?: string }
    requestShape?: { deepSeekChatCompletionsAppendIdempotent?: boolean }
    redaction?: { credentialSecretsRecorded?: boolean }
  }
} {
  const result = spawnSync(process.execPath, [
    './scripts/runtime-go-performance-check.mjs',
    '--self-test-settings-resolution',
    '--json',
    '--gate'
  ], {
    cwd: process.cwd(),
    encoding: 'utf8'
  })
  return {
    status: result.status ?? 0,
    stdout: result.stdout,
    report: JSON.parse(result.stdout)
  }
}

function runLiveValidationDryRun(args: string[]): {
  status: number
  report: {
    checks?: Array<{ id: string; command: string }>
  }
} {
  const result = spawnSync(process.execPath, [
    './scripts/runtime-go-live-validation.mjs',
    '--json',
    '--dry-run',
    ...args
  ], {
    cwd: process.cwd(),
    encoding: 'utf8'
  })
  const report = JSON.parse(result.stdout)
  return { status: result.status ?? 0, report }
}

describe('runtime-go live validation wrapper', () => {
  it('reads live provider credentials from top-level provider profiles without printing keys', () => {
    const { status, stdout, report } = runProviderSettingsSelfTest()

    expect(status).toBe(0)
    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-provider-settings-resolution-self-test',
      status: 'passed',
      provider: expect.objectContaining({
        activeProviderId: 'deepseek',
        providerId: 'deepseek',
        model: 'deepseek-chat',
        hasApiKey: true
      }),
      runtimeProviderOverride: expect.objectContaining({
        activeProviderId: 'custom',
        providerId: 'deepseek',
        model: 'deepseek-chat',
        hasApiKey: true
      }),
      nonDeepSeekProvider: expect.objectContaining({
        providerId: 'custom',
        endpointFormat: 'messages',
        hasApiKey: true,
        providerIsDeepSeek: false
      }),
      presetNonDeepSeekProvider: expect.objectContaining({
        providerId: 'moonshot-cn',
        model: 'kimi-k2.7-code',
        baseUrl: 'https://api.moonshot.cn/v1',
        endpointFormat: 'chat_completions',
        hasApiKey: true,
        providerIsDeepSeek: false
      }),
      multiNonDeepSeekProvider: expect.objectContaining({
        providerId: 'moonshot-cn',
        model: 'kimi-k2.7-code',
        baseUrl: 'https://api.moonshot.cn/v1',
        endpointFormat: 'chat_completions',
        hasApiKey: true,
        providerIsDeepSeek: false
      }),
      tokenPlanNonDeepSeekProvider: expect.objectContaining({
        providerId: 'xiaomi-token-plan',
        model: 'mimo-v2.5-pro',
        baseUrl: 'https://token-plan-cn.xiaomimimo.com/v1',
        endpointFormat: 'chat_completions',
        hasApiKey: true,
        providerIsDeepSeek: false
      }),
      deepSeekProviderFromProfiles: expect.objectContaining({
        activeProviderId: 'xiaomi-token-plan',
        providerId: 'deepseek',
        model: 'deepseek-chat',
        endpointFormat: 'chat_completions',
        hasApiKey: true,
        providerIsDeepSeek: true
      }),
      modelOverrideNonDeepSeekProvider: expect.objectContaining({
        providerId: 'opencode-go',
        model: 'minimax-m3',
        baseUrl: 'https://opencode.ai/zen/go/v1',
        endpointFormat: 'messages',
        hasApiKey: true,
        providerIsDeepSeek: false,
        requestUrl: 'https://opencode.ai/zen/go/v1/messages'
      }),
      requestShape: expect.objectContaining({
        deepSeekChatCompletionsAppendIdempotent: true
      }),
      redaction: expect.objectContaining({
        credentialSecretsRecorded: false
      })
    }))
    expect(stdout).not.toContain('sk-self-test-settings-resolution')
    expect(stdout).not.toContain('sk-self-test-runtime-provider-override')
    expect(stdout).not.toContain('sk-custom-provider')
    expect(stdout).not.toContain('sk-moonshot-provider')
    expect(stdout).not.toContain('sk-stale-provider')
    expect(stdout).not.toContain('tp-xiaomi-provider')
    expect(stdout).not.toContain('sk-self-test-settings-resolution')
    expect(stdout).not.toContain('sk-opencode-provider')
  })

  it('keeps report-only live probes out of child gate mode', () => {
    const { status, report } = runLiveValidationDryRun([
      '--no-gate',
      '--live-non-deepseek-provider-from-settings'
    ])

    expect(status).toBe(0)
    const liveProbe = report.checks?.find((check) => check.id === 'live-provider-probes-command')
    expect(liveProbe?.command).toContain('runtime-go-performance-check.mjs')
    expect(liveProbe?.command).not.toContain(' --gate')
  })

  it('keeps live probes gated by default', () => {
    const { status, report } = runLiveValidationDryRun([
      '--live-non-deepseek-provider-from-settings'
    ])

    expect(status).toBe(1)
    const liveProbe = report.checks?.find((check) => check.id === 'live-provider-probes-command')
    expect(liveProbe?.command).toContain(' --gate')
  })

  it('keeps safe provider probe summaries visible in live validation reports', () => {
    const source = readFileSync('scripts/runtime-go-live-validation.mjs', 'utf8')

    expect(source).toContain('liveDeepSeekCacheSummary')
    expect(source).toContain('liveNonDeepSeekProviderSummary')
    expect(source).toContain('summary: summary || null')
    expect(source).toContain('hasApiKey: probe.provider?.hasApiKey === true')
    expect(source).toContain('prompt_cache_hit_tokens')
    expect(source).toContain('prompt_cache_miss_tokens')
  })
})
