import { createHash } from 'node:crypto'
import {
  chmodSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  configurePackagedMainOwnedRuntimeAuthorityBootstrapV1,
  MAIN_OWNED_AUTHORITY_BOOTSTRAP_DIRECTORY_V1,
  MAIN_OWNED_AUTHORITY_BOOTSTRAP_FILE_V1
} from './main-owned-authority-bootstrap-v1'
import {
  configureMainOwnedRuntimeAuthoritySourceV1,
  takeMainOwnedRuntimeAuthorityEnvelopeV1,
  type MainOwnedRuntimeAuthorityEnvelopeV1
} from './main-owned-authority-envelope-v1'

const temporaryRoots: string[] = []

afterEach(() => {
  configureMainOwnedRuntimeAuthoritySourceV1(null)
  vi.unstubAllEnvs()
  for (const root of temporaryRoots.splice(0)) {
    rmSync(root, { recursive: true, force: true })
  }
})

describe('packaged main-owned runtime authority bootstrap v1', () => {
  it('loads the exact private singleton bootstrap through the production source', async () => {
    if (!['darwin', 'linux'].includes(process.platform)) return
    const userDataDir = newUserDataDirV1()
    const expected = authorityFixtureV1()
    writeBootstrapV1(userDataDir, expected)

    configurePackagedMainOwnedRuntimeAuthorityBootstrapV1({
      appIsPackaged: true,
      userDataDir
    })

    await expect(takeMainOwnedRuntimeAuthorityEnvelopeV1()).resolves.toEqual(expected)
    await expect(takeMainOwnedRuntimeAuthorityEnvelopeV1()).resolves.toEqual(expected)
  })

  it('does not treat direct authority env fields or an unpackaged app as a bootstrap source', async () => {
    const userDataDir = newUserDataDirV1()
    vi.stubEnv('ANALYTIX_AUTHORITY_ANCHOR_V1', JSON.stringify(authorityFixtureV1()))
    configurePackagedMainOwnedRuntimeAuthorityBootstrapV1({
      appIsPackaged: false,
      userDataDir
    })
    await expect(takeMainOwnedRuntimeAuthorityEnvelopeV1()).resolves.toBeNull()

    configurePackagedMainOwnedRuntimeAuthorityBootstrapV1({
      appIsPackaged: true,
      userDataDir
    })
    await expect(takeMainOwnedRuntimeAuthorityEnvelopeV1()).resolves.toBeNull()
  })

  it('fails closed for unsafe permissions or non-singleton inventory', async () => {
    if (!['darwin', 'linux'].includes(process.platform)) return
    const userDataDir = newUserDataDirV1()
    const root = writeBootstrapV1(userDataDir, authorityFixtureV1())
    const file = join(root, MAIN_OWNED_AUTHORITY_BOOTSTRAP_FILE_V1)
    configurePackagedMainOwnedRuntimeAuthorityBootstrapV1({
      appIsPackaged: true,
      userDataDir
    })

    chmodSync(file, 0o644)
    await expect(takeMainOwnedRuntimeAuthorityEnvelopeV1()).rejects.toThrow(
      'Managed runtime authority is unavailable.'
    )

    chmodSync(file, 0o600)
    writeFileSync(join(root, 'unexpected.json'), '{}', { mode: 0o600 })
    await expect(takeMainOwnedRuntimeAuthorityEnvelopeV1()).rejects.toThrow(
      'Managed runtime authority is unavailable.'
    )
  })
})

function newUserDataDirV1(): string {
  const root = mkdtempSync(join(tmpdir(), 'analytix-main-authority-bootstrap-'))
  temporaryRoots.push(root)
  return root
}

function writeBootstrapV1(
  userDataDir: string,
  envelope: MainOwnedRuntimeAuthorityEnvelopeV1
): string {
  const root = join(userDataDir, MAIN_OWNED_AUTHORITY_BOOTSTRAP_DIRECTORY_V1)
  mkdirSync(root, { mode: 0o700 })
  chmodSync(root, 0o700)
  const file = join(root, MAIN_OWNED_AUTHORITY_BOOTSTRAP_FILE_V1)
  writeFileSync(file, JSON.stringify(envelope), { mode: 0o600 })
  chmodSync(file, 0o600)
  return root
}

function authorityFixtureV1(): MainOwnedRuntimeAuthorityEnvelopeV1 {
  const publicKey = Buffer.alloc(32, 0x61)
  const authorityKeyId = createHash('sha256').update(publicKey).digest('hex')
  return {
    schemaVersion: 1,
    purpose: 'analytix.runtime-main-owned-authority/v1',
    authorityAnchorV1: JSON.stringify({
      schemaVersion: 1,
      installationId: '1'.repeat(64),
      authorityKeyId,
      authorityPublicKey: publicKey.toString('base64url'),
      currentManifestDigest: '2'.repeat(64)
    }),
    authorityManifestRoot: '/authority/manifests',
    authorityCredentialProfileRoot: '/authority/profiles',
    authorityCredentialBundleRoot: '/authority/bundles'
  }
}
