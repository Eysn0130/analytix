import 'reflect-metadata'

import { createHash, createPrivateKey, createPublicKey, X509Certificate } from 'node:crypto'
import {
  BasicConstraintsExtension,
  ExtendedKeyUsage,
  ExtendedKeyUsageExtension,
  KeyUsageFlags,
  KeyUsagesExtension,
  SubjectAlternativeNameExtension,
  X509Certificate as PeculiarX509Certificate
} from '@peculiar/x509'
import { describe, expect, it } from 'vitest'
import { generateEphemeralControlledArtifactTLSIdentityV2 } from './ephemeral-tls-identity'

describe('ephemeral controlled artifact TLS identity V2', () => {
  it('issues the exact private root and loopback-only leaf policy accepted by the Go client', async () => {
    const now = new Date('2026-07-18T04:00:00.987Z')
    let serialSeed = 0
    const identity = await generateEphemeralControlledArtifactTLSIdentityV2({
      now: () => now,
      random: (size) => Buffer.alloc(size, ++serialSeed)
    })

    const rootDER = Buffer.from(identity.rootCertificateDERBase64URL, 'base64url')
    const root = new X509Certificate(rootDER)
    const leaf = new X509Certificate(identity.leafCertificatePEM)
    const peculiarRoot = new PeculiarX509Certificate(rootDER)
    const peculiarLeaf = new PeculiarX509Certificate(identity.leafCertificatePEM)

    expect(root.ca).toBe(true)
    expect(root.subject).toBe(root.issuer)
    expect(root.verify(root.publicKey)).toBe(true)
    expect(root.subjectAltName).toBeUndefined()
    expect(root.keyUsage).toBeUndefined()
    expect(rootDER.length).toBeLessThanOrEqual(2 << 10)
    expect(root.publicKey.asymmetricKeyType).toBe('ec')
    expect(root.publicKey.asymmetricKeyDetails?.namedCurve).toBe('prime256v1')

    expect(leaf.ca).toBe(false)
    expect(leaf.issuer).toBe(root.subject)
    expect(leaf.verify(root.publicKey)).toBe(true)
    expect(leaf.checkIP('127.0.0.1')).toBe('127.0.0.1')
    expect(leaf.checkIP('::1')).toBeUndefined()
    expect(leaf.checkHost('localhost')).toBeUndefined()
    expect(leaf.subjectAltName).toBe('IP Address:127.0.0.1')
    expect(leaf.keyUsage).toEqual([ExtendedKeyUsage.serverAuth])
    expect(leaf.publicKey.asymmetricKeyType).toBe('ec')
    expect(leaf.publicKey.asymmetricKeyDetails?.namedCurve).toBe('prime256v1')

    const rootBasicConstraints = peculiarRoot.getExtension(BasicConstraintsExtension)
    const rootKeyUsage = peculiarRoot.getExtension(KeyUsagesExtension)
    expect(peculiarRoot.extensions).toHaveLength(2)
    expect(rootBasicConstraints?.critical).toBe(true)
    expect(rootBasicConstraints?.ca).toBe(true)
    expect(rootKeyUsage?.critical).toBe(true)
    expect(rootKeyUsage?.usages).toBe(KeyUsageFlags.keyCertSign | KeyUsageFlags.cRLSign)

    const leafBasicConstraints = peculiarLeaf.getExtension(BasicConstraintsExtension)
    const leafKeyUsage = peculiarLeaf.getExtension(KeyUsagesExtension)
    const leafExtendedKeyUsage = peculiarLeaf.getExtension(ExtendedKeyUsageExtension)
    const leafSAN = peculiarLeaf.getExtension(SubjectAlternativeNameExtension)
    expect(peculiarLeaf.extensions).toHaveLength(4)
    expect(leafBasicConstraints?.critical).toBe(true)
    expect(leafBasicConstraints?.ca).toBe(false)
    expect(leafKeyUsage?.critical).toBe(true)
    expect(leafKeyUsage?.usages).toBe(KeyUsageFlags.digitalSignature)
    expect(leafExtendedKeyUsage?.critical).toBe(true)
    expect(leafExtendedKeyUsage?.usages).toEqual([ExtendedKeyUsage.serverAuth])
    expect(leafSAN?.critical).toBe(true)
    expect(leafSAN?.names.toJSON()).toEqual([{ type: 'ip', value: '127.0.0.1' }])

    const leafSPKI = leaf.publicKey.export({ format: 'der', type: 'spki' })
    expect(createHash('sha256').update(leafSPKI).digest('hex')).toBe(identity.leafSPKISHA256)
    const privateKeyPEM = identity.takeLeafPrivateKeyPEM()
    try {
      const privateKey = createPrivateKey(privateKeyPEM)
      expect(privateKey.asymmetricKeyType).toBe('ec')
      expect(privateKey.asymmetricKeyDetails?.namedCurve).toBe('prime256v1')
      expect(createPublicKey(privateKey).export({ format: 'der', type: 'spki' })).toEqual(leafSPKI)
    } finally {
      privateKeyPEM.fill(0)
    }
    expect(() => identity.takeLeafPrivateKeyPEM()).toThrow('private key was already consumed')
    expect(identity.notBefore.toISOString()).toBe('2026-07-18T03:59:00.000Z')
    expect(identity.notAfter.toISOString()).toBe('2026-07-25T04:00:00.000Z')
  })

  it('rotates the root, leaf pin, and private key for every identity', async () => {
    const first = await generateEphemeralControlledArtifactTLSIdentityV2()
    const second = await generateEphemeralControlledArtifactTLSIdentityV2()
    const firstPEM = first.takeLeafPrivateKeyPEM()
    const secondPEM = second.takeLeafPrivateKeyPEM()

    try {
      expect(second.rootCertificateDERBase64URL).not.toBe(first.rootCertificateDERBase64URL)
      expect(second.leafSPKISHA256).not.toBe(first.leafSPKISHA256)
      expect(createPrivateKey(secondPEM).equals(createPrivateKey(firstPEM))).toBe(false)
    } finally {
      firstPEM.fill(0)
      secondPEM.fill(0)
    }
  })

  it('rejects invalid clocks and non-canonical serial entropy', async () => {
    await expect(generateEphemeralControlledArtifactTLSIdentityV2({
      now: () => new Date(Number.NaN)
    })).rejects.toThrow('controlled artifact TLS clock is invalid')
    await expect(generateEphemeralControlledArtifactTLSIdentityV2({
      random: () => Buffer.alloc(15)
    })).rejects.toThrow('controlled artifact TLS serial generation failed')
  })
})
