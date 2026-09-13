import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const source = readFileSync('src/main/index.ts', 'utf8')
const ipcSource = readFileSync('src/main/ipc/register-app-ipc-handlers.ts', 'utf8')

function sourceSlice(startMarker: string, endMarker: string): string {
  const start = source.indexOf(startMarker)
  const end = source.indexOf(endMarker, start + startMarker.length)
  expect(start, `missing start marker: ${startMarker}`).toBeGreaterThanOrEqual(0)
  expect(end, `missing end marker: ${endMarker}`).toBeGreaterThan(start)
  return source.slice(start, end)
}

describe('packaged desktop startup contract', () => {
  it('keeps ordinary main startup Hub-cold and loads compatibility only on explicit demand', () => {
    expect(source).not.toContain("from './services/hub-account-service'")
    expect(source).not.toContain("from '../shared/hub-account'")
    expect(source).not.toContain('isHubModelProviderSelected')
    expect(source).not.toContain('const hubAccountService = createHubAccountService(')
    expect(source).not.toContain('await hubAccountService.models()')
    expect(source).toContain("import('./services/hub-account-service')")
    expect(source.match(/hubAccountServicePromise = import\('\.\/services\/hub-account-service'\)/g)).toHaveLength(1)
    expect(source).toContain('if (!hubAccountServicePromise)')
    expect(source).toContain('loadHubAccountService')
    const models = sourceSlice('const fetchModels = async () => {', 'const saveSettingsPatch = async')
    expect(models).toContain("mainProviderRegistry({ schemaVersion: 1, operation: 'list' })")
    expect(models).toContain('fetchUpstreamModelIds(registry)')
    expect(models).not.toContain('store.load()')
    expect(ipcSource).not.toMatch(/from ['"]\.\.\/services\/hub-agent-marketplace-service['"]/)
    expect(ipcSource).toContain("import('../services/hub-agent-marketplace-service')")
    expect(ipcSource.match(/hubAgentMarketplaceServicePromise = import\('\.\.\/services\/hub-agent-marketplace-service'\)/g)).toHaveLength(1)
    expect(ipcSource).toContain('loadHubAgentMarketplaceService')
  })

  it('emits the main-private zero-activity checkpoint only after ordinary window creation', () => {
    expect(source).toContain("import { emitHubActivityObservation } from './hub-activity-observation'")
    const createWindowReturned = source.indexOf("traceStartup('createWindow:returned')")
    const observation = source.indexOf("emitHubActivityObservation('ordinary_startup_ready')")

    expect(createWindowReturned).toBeGreaterThanOrEqual(0)
    expect(observation).toBeGreaterThan(createWindowReturned)
    expect(source.match(/emitHubActivityObservation\('ordinary_startup_ready'\)/g)).toHaveLength(1)
  })

  it('does not make managed Go runtime lifecycle depend on provider credentials', () => {
    const lifecycleSlices = [
      sourceSlice('async function superviseAnalytixCrash(', 'function startRuntimeWatchdog('),
      sourceSlice('async function ensureAnalytixRuntime(', 'async function restartRuntime('),
      sourceSlice('async function restartRuntimeOnce(', 'function createWindow('),
      sourceSlice(
        'async function restartManagedRuntimeForSettingsChange(',
        'async function rollbackRuntimeSettingsAfterFailedApply('
      ),
      sourceSlice(
        'async function rollbackRuntimeSettingsAfterFailedApply(',
        'async function restartManagedRuntimeForMcpConfigChange('
      ),
      sourceSlice(
        'async function restartManagedRuntimeForMcpConfigChange(',
        'async function waitForManagedRuntimeReadyBeforeStop('
      )
    ]

    for (const body of lifecycleSlices) {
      expect(body).toContain('autoStart')
      expect(body).not.toContain('resolveConfiguredApiKey')
      expect(body).not.toContain('missing_api_key')
    }

    const prewarm = sourceSlice(
      'if (getAnalytixRuntimeSettings(initial).autoStart) {',
      "app.on('second-instance'"
    )
    expect(prewarm).toContain('analytixRuntimeAdapter.resolveExecutable(initial)')
    expect(prewarm).not.toContain('resolveConfiguredApiKey')

    expect(source).not.toContain('resolveConfiguredApiKey')
  })

  it('quits the lock-losing process before Electron readiness work', () => {
    const lock = source.indexOf('const gotSingleInstanceLock =')
    const quit = source.indexOf('if (!gotSingleInstanceLock) {\n  app.quit()\n}', lock)
    const ready = source.indexOf('app.whenReady().then', lock)

    expect(lock).toBeGreaterThanOrEqual(0)
    expect(quit).toBeGreaterThan(lock)
    expect(ready).toBeGreaterThan(quit)
    const readinessBranch = source.slice(
      source.lastIndexOf('if (runningClawScheduleMcpServer)', ready),
      ready + 'app.whenReady().then'.length
    )
    expect(readinessBranch).toContain(
      '} else if (gotSingleInstanceLock) {\nregisterPackagedRendererScheme()\napp.whenReady().then'
    )
    expect(source).not.toContain("traceStartup('app.whenReady:start')\n  if (!gotSingleInstanceLock) return")

    const lockLoserBody = source.slice(quit, source.indexOf('pendingDeepLinkThreadId =', quit))
    expect(lockLoserBody).toBe('if (!gotSingleInstanceLock) {\n  app.quit()\n}\n\n')
    for (const forbidden of ['app.whenReady(', 'createWindow(', 'ensureAnalytixRuntime(', 'startGoConformanceSidecarOnce(']) {
      expect(lockLoserBody).not.toContain(forbidden)
    }
  })

  it('uses the bundled renderer whenever Electron reports a packaged app', () => {
    const createWindow = sourceSlice('function createWindow(', 'function openThreadInNewWindow(')
    expect(createWindow).toContain('devServerHintUrl(app.isPackaged)')
  })
})
