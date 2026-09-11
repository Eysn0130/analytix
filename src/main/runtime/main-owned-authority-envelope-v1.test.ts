import { createHash } from 'node:crypto'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  configureMainOwnedRuntimeAuthoritySourceV1,
  takeMainOwnedRuntimeAuthorityEnvelopeV1,
  validateMainOwnedRuntimeAuthorityEnvelopeV1,
  type MainOwnedRuntimeAuthorityEnvelopeV1
} from './main-owned-authority-envelope-v1'

afterEach(() => {
  configureMainOwnedRuntimeAuthoritySourceV1(null)
  vi.unstubAllEnvs()
})

describe('main-owned runtime authority envelope v1', () => {
  it('takes a fresh exact envelope only from the installed main-process source', async () => {
    let takes = 0
    configureMainOwnedRuntimeAuthoritySourceV1({
      take: async () => {
        takes++
        return authorityFixtureV1()
      }
    })

    const first = await takeMainOwnedRuntimeAuthorityEnvelopeV1()
    const second = await takeMainOwnedRuntimeAuthorityEnvelopeV1()
    expect(takes).toBe(2)
    expect(first).toEqual(authorityFixtureV1())
    expect(second).toEqual(authorityFixtureV1())
    expect(Object.isFrozen(first)).toBe(true)
  })

  it('has no ambient environment, argv or settings fallback', async () => {
    vi.stubEnv('ANALYTIX_AUTHORITY_ANCHOR_V1', authorityFixtureV1().authorityAnchorV1)
    vi.stubEnv('ANALYTIX_AUTHORITY_MANIFEST_ROOT', '/private/authority/manifest')
    vi.stubEnv('ANALYTIX_AUTHORITY_CREDENTIAL_PROFILE_ROOT', '/private/authority/profile')
    vi.stubEnv('ANALYTIX_AUTHORITY_CREDENTIAL_BUNDLE_ROOT', '/private/authority/bundle')
    expect(await takeMainOwnedRuntimeAuthorityEnvelopeV1()).toBeNull()
  })

  it('rejects noncanonical anchors, unknown fields, aliased roots and malformed key bindings', () => {
    const valid = authorityFixtureV1()
    expect(() => validateMainOwnedRuntimeAuthorityEnvelopeV1({
      ...valid,
      authorityAnchorV1: `${valid.authorityAnchorV1}\n`
    })).toThrow('Managed runtime authority is unavailable.')
    expect(() => validateMainOwnedRuntimeAuthorityEnvelopeV1({
      ...valid,
      authorityCredentialBundleRoot: valid.authorityManifestRoot
    })).toThrow('Managed runtime authority is unavailable.')
    expect(() => validateMainOwnedRuntimeAuthorityEnvelopeV1({
      ...valid,
      authorityAnchorV1: JSON.stringify({
        ...JSON.parse(valid.authorityAnchorV1),
        authorityKeyId: '0'.repeat(64)
      })
    })).toThrow('Managed runtime authority is unavailable.')
    expect(() => validateMainOwnedRuntimeAuthorityEnvelopeV1({
      ...valid,
      extra: true
    } as unknown as MainOwnedRuntimeAuthorityEnvelopeV1)).toThrow(
      'Managed runtime authority is unavailable.'
    )
  })
})

function authorityFixtureV1(): MainOwnedRuntimeAuthorityEnvelopeV1 {
  const publicKey = Buffer.from(Array.from({ length: 32 }, (_, index) => index + 1))
  const authorityKeyId = createHash('sha256').update(publicKey).digest('hex')
  const authorityAnchorV1 = JSON.stringify({
    schemaVersion: 1,
    installationId: '1'.repeat(64),
    authorityKeyId,
    authorityPublicKey: publicKey.toString('base64url'),
    currentManifestDigest: '2'.repeat(64)
  })
  publicKey.fill(0)
  return {
    schemaVersion: 1,
    purpose: 'analytix.runtime-main-owned-authority/v1',
    authorityAnchorV1,
    authorityManifestRoot: '/private/authority/manifest',
    authorityCredentialProfileRoot: '/private/authority/profile',
    authorityCredentialBundleRoot: '/private/authority/bundle'
  }
}
