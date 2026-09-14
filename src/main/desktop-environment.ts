import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import type { DesktopEnvironment } from '../shared/desktop-environment'

const fingerprint = (value: string | Buffer): string =>
  createHash('sha256').update(value).digest('hex').slice(0, 12)

export function desktopEnvironment(input: {
  isPackaged: boolean
  isolated: boolean
  userData: string
  version: string
  mainBundlePath: string
  env: NodeJS.ProcessEnv
}): DesktopEnvironment {
  if (input.isPackaged) return { mode: 'packaged' }
  const isolated = input.isolated
  let mainBuildId: string | undefined
  try { mainBuildId = fingerprint(readFileSync(input.mainBundlePath)) } catch { /* No guessed build identity. */ }
  return {
    mode: 'development',
    profileId: fingerprint(input.userData),
    isolated,
    mock: isolated && input.env.ANALYTIX_DEV_PROVIDER_MODE === 'mock',
    version: input.version.slice(0, 80),
    ...(mainBuildId ? { mainBuildId } : {})
  }
}
