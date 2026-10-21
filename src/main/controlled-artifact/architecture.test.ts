import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

const FORBIDDEN_PUBLIC_CAPABILITIES = [
  'ControlledArtifactRuntimeReleaseInvocationV1',
  'ANALYTIX_CONTROLLED_ARTIFACT_HOST_URL',
  'ANALYTIX_CONTROLLED_ARTIFACT_HOST_TOKEN',
  'controlledHandle',
  'useSlot',
  'rendererPrincipal',
  '/v1/controlled-artifacts/release'
] as const

describe('controlled artifact desktop architecture', () => {
  it('keeps host secrets, raw invocation capabilities, and release transport out of public desktop layers', () => {
    const roots = ['src/preload', 'src/renderer', 'src/shared', 'packages/runtime/src']
    const violations: string[] = []
    for (const root of roots) {
      for (const file of sourceFiles(root)) {
        const body = readFileSync(file, 'utf8')
        for (const forbidden of FORBIDDEN_PUBLIC_CAPABILITIES) {
          if (body.includes(forbidden)) violations.push(`${file}:${forbidden}`)
        }
      }
    }
    expect(violations).toEqual([])
  })

  it('does not assemble or inject the retired V1 authority in production', () => {
    const main = readFileSync('src/main/index.ts', 'utf8')
    const adapter = readFileSync('src/main/runtime/analytix-adapter.ts', 'utf8')

    for (const forbidden of [
      'ControlledArtifactHostV1',
      'ElectronControlledArtifactReleaseEffectsV1',
      'configureControlledArtifactHostRuntimeEnvironment',
      'controlled-artifact-display-v1'
    ]) {
      expect(main).not.toContain(forbidden)
      expect(adapter).not.toContain(forbidden)
    }
    expect(adapter).not.toContain("from '../controlled-artifact/host'")
    expect(adapter.match(/RETIRED_CONTROLLED_ARTIFACT_HOST_URL_ENV/g)).toHaveLength(2)
    expect(adapter.match(/RETIRED_CONTROLLED_ARTIFACT_HOST_TOKEN_ENV/g)).toHaveLength(2)
  })
})

function sourceFiles(root: string): string[] {
  const files: string[] = []
  for (const name of readdirSync(root)) {
    const path = join(root, name)
    const stats = statSync(path)
    if (stats.isDirectory()) files.push(...sourceFiles(path))
    else if (/\.(?:ts|tsx)$/u.test(name)) files.push(path)
  }
  return files
}
