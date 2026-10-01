import { readFileSync } from 'node:fs'
import { join } from 'node:path'

// Package metadata only narrows startup. Go independently checks the sealed
// package disposition against its build-time profile before granting authority.
export function packagedReleaseProfile(appPath: string): 'full' | 'core' {
  const value = JSON.parse(readFileSync(join(appPath, 'package.json'), 'utf8')).releaseProfile
  if (value === undefined || value === 'full') return 'full'
  if (value === 'core') return 'core'
  throw new Error('packaged_release_profile_invalid')
}
