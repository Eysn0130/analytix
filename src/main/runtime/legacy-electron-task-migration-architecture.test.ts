import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('desktop private history migration startup barrier', () => {
  it('completes before runtime supervision, IPC registration, and the first window', () => {
    const source = readFileSync('src/main/index.ts', 'utf8')
    const load = source.indexOf('const initial = await store.load()')
    const migration = source.indexOf(
      "await migrateDesktopPrivateHistoryBeforeStartV2(initial, app.getPath('userData'))"
    )
    const runtimeSupervision = source.indexOf('setAnalytixUnexpectedExitHandler(handleUnexpectedAnalytixExit)')
    const ipc = source.indexOf("traceStartup('ipc registration:start')")
    const barrierRelease = source.indexOf('desktopStartupBarrierComplete = true')
    const window = source.indexOf('createWindow({', ipc)
    expect(load).toBeGreaterThanOrEqual(0)
    expect(migration).toBeGreaterThan(load)
    expect(runtimeSupervision).toBeGreaterThan(migration)
    expect(ipc).toBeGreaterThan(runtimeSupervision)
    expect(barrierRelease).toBeGreaterThan(ipc)
    expect(window).toBeGreaterThan(barrierRelease)
    expect(source).toContain('!gotSingleInstanceLock || !desktopStartupBarrierComplete')
  })

  it('uses a dedicated Go command without syncing provider, MCP, or skill configuration', () => {
    const source = readFileSync('src/main/runtime/analytix-adapter.ts', 'utf8')
    const start = source.indexOf('export async function migrateDesktopPrivateHistoryBeforeStartV2(')
    const end = source.indexOf('\nexport function waitForDesktopPrivateHistoryMigrationV2', start)
    const body = source.slice(start, end)
    expect(start).toBeGreaterThanOrEqual(0)
    expect(end).toBeGreaterThan(start)
    expect(body).toContain('runtimeServer: true')
    expect(body).toContain('buildDesktopPrivateHistoryMigrationArgsV2')
    expect(body).toContain('buildDesktopPrivateHistoryMigrationEnvV2')
    expect(body).not.toContain('syncGuiManagedAnalytixConfig')
    expect(body).not.toContain('buildGoRuntimeProviderArgs')
    expect(body).not.toContain('resolveGoRuntimeMCPConfigPath')
    expect(body).not.toContain('JSON.parse')
    expect(body).not.toContain('reasoning')
    expect(body).not.toContain('autoStart')
    expect(body).not.toContain('hasApiKey')
  })
})
