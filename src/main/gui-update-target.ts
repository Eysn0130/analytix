export type UpdateProfile = 'core' | 'full'

export function updateProfile(value: unknown): UpdateProfile {
  if (value === undefined || value === 'full') return 'full'
  if (value === 'core') return 'core'
  throw new Error('update_profile_invalid')
}

export function profileUpdateFeed(base: string, profile: UpdateProfile, platform: string, arch: string, channel: string): string {
  if (!['stable', 'beta'].includes(channel)) throw new Error('update_channel_invalid')
  if (profile === 'core' && (platform !== 'darwin' || arch !== 'arm64')) throw new Error('core_update_target_unsupported')
  const prefix = profile === 'core' ? '/core/darwin-arm64' : ''
  return `${base.replace(/\/+$/, '')}${prefix}/channels/${channel}/latest/`
}

// This checks a delivery target, not OS signature or publication authority.
// electron-updater still owns signed-app verification and download integrity.
export function assertUpdateTarget(info: Record<string, unknown>, profile: UpdateProfile, platform: string, arch: string, channel: string): void {
  if (profile === 'full') {
    if (info.analytixProfile !== undefined && info.analytixProfile !== 'full') throw new Error('update_profile_migration_unsupported')
    return
  }
  if (platform !== 'darwin' || arch !== 'arm64' || info.analytixProfile !== 'core' ||
      info.analytixTarget !== 'darwin-arm64' || info.analytixChannel !== channel ||
      info.analytixAppId !== 'com.analytix.desktop' || info.analytixDataCompatibility !== 'core-v1') {
    throw new Error('core_update_target_mismatch')
  }
  if (typeof info.version !== 'string' || !/^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/.test(info.version)) throw new Error('core_update_version_invalid')
  const files = info.files
  if (!Array.isArray(files) || files.length === 0 || !files.every(file => {
    if (!file || typeof file !== 'object') return false
    const entry = file as Record<string, unknown>
    return typeof info.version === 'string' && entry.url === `analytix-core-${info.version}-mac-arm64.${String(entry.url).endsWith('.zip') ? 'zip' : 'dmg'}` &&
      typeof entry.sha512 === 'string' && /^[A-Za-z0-9+/]{86}==$/.test(entry.sha512) &&
      typeof entry.size === 'number' && Number.isSafeInteger(entry.size) && entry.size > 0
  }) || new Set(files.map(file => file.url)).size !== files.length || !files.some(file => String(file.url).endsWith('.zip'))) throw new Error('core_update_artifacts_invalid')
}
