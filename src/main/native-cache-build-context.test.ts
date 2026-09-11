import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createRequire } from 'node:module'
import { describe, expect, it } from 'vitest'

const require = createRequire(import.meta.url)
const nativeComponentContract = require('../../scripts/native-component-contract.cjs') as {
  NATIVE_BUILD_CONTEXT_PATHS: readonly string[]
  nativeBuildContextDigest: (repositoryRoot: string) => string
}

describe('native cache preflight build context', () => {
  it('binds the cache helper and its exact storage parser blob', () => {
    expect(nativeComponentContract.NATIVE_BUILD_CONTEXT_PATHS).toEqual(expect.arrayContaining([
      'scripts/use-analytix-cache.sh',
      'scripts/analytix-cache-storage.zsh'
    ]))

    const repositoryRoot = mkdtempSync(join(tmpdir(), 'analytix-native-cache-context-'))
    try {
      const parserPath = join(repositoryRoot, 'scripts', 'analytix-cache-storage.zsh')
      mkdirSync(join(repositoryRoot, 'scripts'), { recursive: true })
      writeFileSync(parserPath, 'parser-v1\n', { mode: 0o600 })
      const firstDigest = nativeComponentContract.nativeBuildContextDigest(repositoryRoot)
      writeFileSync(parserPath, 'parser-v2\n', { mode: 0o600 })
      expect(nativeComponentContract.nativeBuildContextDigest(repositoryRoot)).not.toBe(firstDigest)
    } finally {
      rmSync(repositoryRoot, { recursive: true, force: true })
    }
  })
})
