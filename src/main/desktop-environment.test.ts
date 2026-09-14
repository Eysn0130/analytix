import { mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { afterEach, describe, expect, it } from 'vitest'
import { desktopEnvironment } from './desktop-environment'
import { desktopEnvironmentSchema } from '../shared/desktop-environment'

const roots: string[] = []
afterEach(() => roots.splice(0).forEach((root) => rmSync(root, { recursive: true, force: true })))

describe('desktop environment identity', () => {
  const base = { isPackaged: false, isolated: false, userData: '/private/task/profile', version: '1.0.6', mainBundlePath: '/missing/main.js', env: {} }
  it('identifies development profiles without exposing paths or environment contents', () => {
    const result = desktopEnvironment({ ...base, env: { PRIVATE_CANARY: 'PRIVATE_CANARY' } })
    expect(desktopEnvironmentSchema.safeParse(result).success).toBe(true)
    expect(result).toMatchObject({ mode: 'development', mock: false, isolated: false })
    expect(JSON.stringify(result)).not.toMatch(/private|CANARY|main\.js/)
    expect(desktopEnvironment({ ...base, userData: '/another/profile' })).not.toEqual(result)
    expect(desktopEnvironment(base)).toEqual(result)
  })
  it('requires explicit Mock intent AND isolation, and never marks a package as a test run', () => {
    // Startup consumes the environment marker; use its validated boundary receipt.
    const env = { ANALYTIX_DEV_PROVIDER_MODE: 'mock' }
    expect(desktopEnvironment({ ...base, env, isolated: true })).toMatchObject({ mock: true, isolated: true })
    expect(desktopEnvironment({ ...base, env: { ANALYTIX_DEV_PROVIDER_MODE: 'mock' } })).toMatchObject({ mock: false })
    expect(desktopEnvironment({ ...base, env, isPackaged: true })).toEqual({ mode: 'packaged' })
  })
  it('fingerprints the supplied startup Main bundle and leaves a missing build unidentified', () => {
    const root = mkdtempSync(join(tmpdir(), 'desktop-identity-')); roots.push(root)
    const mainBundlePath = join(root, 'index.js')
    writeFileSync(mainBundlePath, 'first build')
    const before = desktopEnvironment({ ...base, mainBundlePath })
    writeFileSync(mainBundlePath, 'second build')
    const after = desktopEnvironment({ ...base, mainBundlePath })
    expect(before).not.toEqual(after)
    expect(desktopEnvironmentSchema.safeParse(after).success).toBe(true)
    expect(desktopEnvironment(base)).not.toHaveProperty('mainBuildId')
  })
})
