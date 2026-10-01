import { describe, expect, it } from 'vitest'
import { assertUpdateTarget, profileUpdateFeed, updateProfile } from './gui-update-target'

describe('Core update target', () => {
  const info = () => ({ version: '1.0.7', analytixProfile: 'core', analytixTarget: 'darwin-arm64', analytixChannel: 'beta', analytixAppId: 'com.analytix.desktop', analytixDataCompatibility: 'core-v1', files: [{url: 'analytix-core-1.0.7-mac-arm64.zip',sha512:'a'.repeat(86)+'==',size:128}] })
  it('binds profile/platform/channel and retains ordinary identity', () => {
    expect(profileUpdateFeed('https://example.test/releases/', 'core', 'darwin', 'arm64', 'beta')).toBe('https://example.test/releases/core/darwin-arm64/channels/beta/latest/')
    expect(() => assertUpdateTarget(info(), 'core', 'darwin', 'arm64', 'beta')).not.toThrow()
    expect(() => profileUpdateFeed('https://example.test', 'core', 'win32', 'x64', 'stable')).toThrow()
    expect(() => updateProfile('unknown')).toThrow()
  })
  it('rejects wrong profile, target, channel, identity, compatibility and corrupt metadata', () => {
    for (const key of ['analytixProfile','analytixTarget','analytixChannel','analytixAppId','analytixDataCompatibility']) {
      expect(() => assertUpdateTarget({...info(),[key]:'wrong'}, 'core','darwin','arm64','beta')).toThrow()
    }
    for (const patch of [{url:'analytix-full-1.0.7-mac-arm64.zip'},{url:'../outside.zip'},{sha512:'bad'},{size:0}]) {
      expect(() => assertUpdateTarget({...info(),files:[{...info().files[0],...patch}]}, 'core','darwin','arm64','beta')).toThrow()
    }
    expect(() => assertUpdateTarget(info(), 'full','darwin','arm64','beta')).toThrow()
    expect(() => assertUpdateTarget({version:'1.0.7'}, 'core','darwin','arm64','beta')).toThrow()
  })
})
