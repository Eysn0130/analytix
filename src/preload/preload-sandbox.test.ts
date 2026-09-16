import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

describe('preload sandbox guard', () => {
  it('keeps the sandbox preload free of node builtins and bridge aliases', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/preload/index.ts'), 'utf8')
    const forbiddenWindowAliases = [
      'analytixGui',
      'deepseek',
      'deepseekGui',
      'kun',
      'kunGui',
      'reasonix',
      'reasonixGui'
    ]
    const exposeCallCount = source.match(/contextBridge\.exposeInMainWorld\(/g)?.length ?? 0
    const exposedBridgeNames = Array.from(
      source.matchAll(/contextBridge\.exposeInMainWorld\(\s*(['"`])([^'"`]+)\1/g)
    ).map((match) => match[2])

    expect(source).not.toMatch(/\bfrom\s+['"]node:/)
    for (const alias of forbiddenWindowAliases) {
      expect(source).not.toContain(['window', alias].join('.'))
    }
    expect(exposedBridgeNames).toHaveLength(exposeCallCount)
    expect(exposedBridgeNames).toEqual(['analytix'])
  })

  it('bundles preload runtime validators for the Electron sandbox', () => {
    const config = readFileSync(resolve(process.cwd(), 'electron.vite.config.ts'), 'utf8')

    expect(config).toContain("externalizeDepsPlugin({ exclude: ['zod'] })")
  })

  it('keeps the renderer window type on the analytix bridge only', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/preload/index.d.ts'), 'utf8')
    const windowInterface = source.match(/interface Window\s*{([\s\S]*?)^\s*}/m)
    const windowProperties = Array.from(
      windowInterface?.[1].matchAll(/^\s*([A-Za-z_$][\w$]*)\??\s*:/gm) ?? []
    ).map((match) => match[1])

    expect(windowProperties).toEqual(['analytix'])
  })

  it('keeps runtime requests on the analytix runtime IPC bridge', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/preload/index.ts'), 'utf8')

    expect(source).toMatch(
      /runtimeRequest:\s*\(path,\s*method,\s*body\)\s*=>\s*[\r\n\s]*ipcRenderer\.invoke\('runtime:request',\s*\{\s*path,\s*method,\s*body\s*\}\)/m
    )
    expect(source).not.toMatch(/reasonix:request|kun:request|runtime:go:request|workflow:request/i)
    expect(source).not.toContain("ipcRenderer.invoke('provider:probe'")
  })

  it('exposes direct source preview and accepted slot display through narrow typed channels', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/preload/index.ts'), 'utf8')

    expect(source).toMatch(
      /importMappingPreview:\s*\(request\)\s*=>\s*ipcRenderer\.invoke\('runtime:import-mapping-preview',\s*request\)/m
    )
    expect(source).toMatch(
      /cleaningDiffPreview:\s*\(request\)\s*=>\s*ipcRenderer\.invoke\('runtime:cleaning-diff-preview',\s*request\)/m
    )
    expect(source).toMatch(
      /directSourcePreview:\s*\(request\)\s*=>\s*ipcRenderer\.invoke\('runtime:direct-source-preview',\s*request\)/m
    )
    expect(source).toMatch(
      /acceptedSlotDisplay:\s*\(request\)\s*=>\s*ipcRenderer\.invoke\('runtime:accepted-slot-display',\s*request\)/m
    )
    expect(source).toMatch(
      /stageFundsCSVSnapshot:\s*\(\)\s*=>\s*[\r\n\s]*ipcRenderer\.invoke\('runtime:stage-funds-csv-snapshot'\)/m
    )
    expect(source).toMatch(
      /confirmFundsCSVSnapshot:\s*\(selector\)\s*=>\s*[\r\n\s]*ipcRenderer\.invoke\('runtime:confirm-funds-csv-snapshot',\s*selector\)/m
    )
    expect(source).toMatch(
      /cancelFundsCSVImport:\s*\(selector\)\s*=>\s*[\r\n\s]*ipcRenderer\.invoke\('runtime:cancel-funds-csv-import',\s*selector\)/m
    )
    expect(source).toMatch(
      /statusFundsCSVImport:\s*\(selector\)\s*=>\s*[\r\n\s]*ipcRenderer\.invoke\('runtime:status-funds-csv-import',\s*selector\)/m
    )
    expect(source).toMatch(
      /runDeterministicFundsCleaning:\s*\(\)\s*=>\s*[\r\n\s]*ipcRenderer\.invoke\('runtime:run-deterministic-funds-cleaning'\)/m
    )
    expect(source).toMatch(
      /revokeCleaningDiffPreview:\s*\(selector\)\s*=>\s*[\r\n\s]*ipcRenderer\.invoke\('runtime:revoke-cleaning-diff-preview',\s*selector\)/m
    )
    expect(source).not.toMatch(/(?:importMappingPreview|cleaningDiffPreview|directSourcePreview|acceptedSlotDisplay)\s*:\s*\(path/i)
    expect(source).not.toMatch(/stageFundsCSVSnapshot:\s*\([^)]/)
    expect(source).not.toMatch(/runDeterministicFundsCleaning:\s*\([^)]/)
    expect(source).not.toMatch(/(?:confirmFundsCSVSnapshot|cancelFundsCSVImport|statusFundsCSVImport):\s*\((?:path|workspace|case|snapshot|principal|grant)/i)
    expect(source).not.toMatch(/revokeCleaningDiffPreview:\s*\((?:path|workspace|case|snapshot|principal|grant)/i)
  })

  it('keeps SSE streaming on the analytix runtime IPC bridge', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/preload/index.ts'), 'utf8')

    expect(source).toMatch(
      /startSse:\s*\(threadId,\s*sinceSeq,\s*streamId\)\s*=>\s*[\r\n\s]*ipcRenderer\.invoke\('runtime:sse:start',\s*\{\s*threadId,\s*sinceSeq,\s*streamId\s*\}\)/m
    )
    expect(source).toMatch(
      /stopSse:\s*\(streamId\)\s*=>\s*ipcRenderer\.invoke\('runtime:sse:stop',\s*streamId\)/m
    )
    expect(source).not.toMatch(/reasonix:sse|kun:sse|runtime:go:sse|workflow:sse/i)
  })

  it('keeps shared public API types under analytix ownership', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/shared/analytix-api.ts'), 'utf8')
    const forbiddenExportedApiNames = [
      'AnalytixGuiApi',
      'DeepSeekApi',
      'KunApi',
      'KunGuiApi',
      'ReasonixApi',
      'ReasonixSessionAPI'
    ]

    expect(source).toMatch(/\bexport type AnalytixApi\b/)
    expect(source).not.toContain('probeModelProvider')
    for (const name of forbiddenExportedApiNames) {
      expect(source).not.toMatch(new RegExp(`\\bexport\\s+(?:type|interface)\\s+${name}\\b`))
    }
  })

  it('keeps the public facade on analytix-owned top-level domains', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/shared/analytix-api.ts'), 'utf8')
    const facade = source.match(/export type AnalytixDomainFacade = \{([\s\S]*?)^\}/m)
    const domains = Array.from(
      facade?.[1].matchAll(/^  ([A-Za-z_$][\w$]*)\s*:/gm) ?? []
    ).map((match) => match[1])

    expect(domains).toEqual([
      'canvas',
      'office',
      'packageHost',
      'objects',
      'settings',
      'account',
      'providerRegistry',
      'providerCredentialRecovery',
      'providerOAuth',
      'mcpOAuth',
      'extensionOAuth',
      'runtime',
      'connectPhone',
      'schedule',
      'workspace',
      'files',
      'write',
      'speech',
      'terminal',
      'backgroundTasks',
      'updates',
      'logs',
      'app',
      'diagnostics',
      'dataAnalysis'
    ])
    expect(domains).not.toEqual(expect.arrayContaining(['claw', 'kun', 'reasonix']))
  })
})
