import { describe, it, expect } from 'vitest'
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { packagedReleaseProfile } from './release-profile'

describe('sealed package startup profile', () => {
  it('reads the package choice and rejects unknown or malformed metadata', () => {
    const root = mkdtempSync(join(tmpdir(), 'analytix-profile-'))
    try {
      for (const value of [undefined, 'full', 'core', 'unknown', false]) {
        writeFileSync(join(root, 'package.json'), JSON.stringify({ releaseProfile: value }))
        if (value === 'unknown' || value === false) expect(() => packagedReleaseProfile(root)).toThrow()
        else expect(packagedReleaseProfile(root)).toBe(value ?? 'full')
      }
      writeFileSync(join(root, 'package.json'), '{')
      expect(() => packagedReleaseProfile(root)).toThrow()
    } finally { rmSync(root, { recursive: true }) }
  })
})
