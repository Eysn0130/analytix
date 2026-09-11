import { createHash, randomBytes, webcrypto } from 'node:crypto'

const LOOPBACK_ADDRESS = '127.0.0.1'
const CLOCK_SKEW_MS = 60_000
const CERTIFICATE_LIFETIME_MS = 7 * 24 * 60 * 60 * 1_000
const SERIAL_BYTES = 16
const MAX_ROOT_CERTIFICATE_DER_BYTES = 2 << 10

const KEY_ALGORITHM = {
  name: 'ECDSA',
  namedCurve: 'P-256'
} as const

const SIGNING_ALGORITHM = {
  name: 'ECDSA',
  hash: 'SHA-256'
} as const

const ROOT_NAME = 'CN=analytix controlled artifact ephemeral root'
const LEAF_NAME = 'CN=analytix controlled artifact ephemeral host'

export type EphemeralControlledArtifactTLSIdentityV2 = Readonly<{
  rootCertificateDERBase64URL: string
  leafCertificatePEM: string
  leafSPKISHA256: string
  notBefore: Date
  notAfter: Date
  takeLeafPrivateKeyPEM: () => Buffer
}>

export type EphemeralControlledArtifactTLSIdentityOptionsV2 = Readonly<{
  now?: () => Date
  random?: (size: number) => Buffer
}>

export async function generateEphemeralControlledArtifactTLSIdentityV2(
  options: EphemeralControlledArtifactTLSIdentityOptionsV2 = {}
): Promise<EphemeralControlledArtifactTLSIdentityV2> {
  const now = cloneValidDate((options.now ?? (() => new Date()))())
  const random = options.random ?? randomBytes
  const roundedNow = Math.floor(now.getTime() / 1_000) * 1_000
  const notBefore = new Date(roundedNow - CLOCK_SKEW_MS)
  const notAfter = new Date(roundedNow + CERTIFICATE_LIFETIME_MS)
  const crypto = webcrypto as unknown as Crypto
  await import('reflect-metadata')
  const {
    BasicConstraintsExtension,
    ExtendedKeyUsage,
    ExtendedKeyUsageExtension,
    KeyUsageFlags,
    KeyUsagesExtension,
    SubjectAlternativeNameExtension,
    X509CertificateGenerator
  } = await import('@peculiar/x509')

  const rootKeys = await crypto.subtle.generateKey(KEY_ALGORITHM, false, ['sign', 'verify'])
  const rootCertificate = await X509CertificateGenerator.createSelfSigned({
    serialNumber: createPositiveSerialNumber(random),
    name: ROOT_NAME,
    notBefore,
    notAfter,
    signingAlgorithm: SIGNING_ALGORITHM,
    keys: rootKeys,
    extensions: [
      new BasicConstraintsExtension(true, undefined, true),
      new KeyUsagesExtension(KeyUsageFlags.keyCertSign | KeyUsageFlags.cRLSign, true)
    ]
  }, crypto)

  const leafKeys = await crypto.subtle.generateKey(KEY_ALGORITHM, true, ['sign', 'verify'])
  const leafCertificate = await X509CertificateGenerator.create({
    serialNumber: createPositiveSerialNumber(random),
    subject: LEAF_NAME,
    issuer: rootCertificate.subjectName,
    notBefore,
    notAfter,
    signingAlgorithm: SIGNING_ALGORITHM,
    publicKey: leafKeys.publicKey,
    signingKey: rootKeys.privateKey,
    extensions: [
      new BasicConstraintsExtension(false, undefined, true),
      new KeyUsagesExtension(KeyUsageFlags.digitalSignature, true),
      new ExtendedKeyUsageExtension([ExtendedKeyUsage.serverAuth], true),
      new SubjectAlternativeNameExtension([{ type: 'ip', value: LOOPBACK_ADDRESS }], true)
    ]
  }, crypto)

  if (!await rootCertificate.isSelfSigned(crypto) ||
      !await leafCertificate.verify({ publicKey: rootCertificate.publicKey, date: now }, crypto)) {
    throw new Error('controlled artifact TLS certificate verification failed')
  }

  const rootDER = Buffer.from(rootCertificate.rawData)
  const leafSPKI = Buffer.from(leafCertificate.publicKey.rawData)
  const privateKeyDER = Buffer.from(await crypto.subtle.exportKey('pkcs8', leafKeys.privateKey))
  let privateKeyPEM: Buffer | null = null
  let identityOwnsPrivateKey = false
  try {
    if (rootDER.length === 0 || rootDER.length > MAX_ROOT_CERTIFICATE_DER_BYTES) {
      throw new Error('controlled artifact TLS root certificate size is invalid')
    }
    privateKeyPEM = encodePEMBuffer(privateKeyDER, 'PRIVATE KEY')
    const identity: EphemeralControlledArtifactTLSIdentityV2 = {
      rootCertificateDERBase64URL: rootDER.toString('base64url'),
      leafCertificatePEM: leafCertificate.toString('pem'),
      leafSPKISHA256: createHash('sha256').update(leafSPKI).digest('hex'),
      notBefore: new Date(notBefore.getTime()),
      notAfter: new Date(notAfter.getTime()),
      takeLeafPrivateKeyPEM: () => {
        if (!privateKeyPEM) throw new Error('controlled artifact TLS private key was already consumed')
        const value = privateKeyPEM
        privateKeyPEM = null
        return value
      }
    }
    identityOwnsPrivateKey = true
    return Object.freeze(identity)
  } finally {
    if (!identityOwnsPrivateKey) privateKeyPEM?.fill(0)
    rootDER.fill(0)
    leafSPKI.fill(0)
    privateKeyDER.fill(0)
  }
}

function createPositiveSerialNumber(random: (size: number) => Buffer): string {
  const generated = random(SERIAL_BYTES)
  if (!Buffer.isBuffer(generated) || generated.length !== SERIAL_BYTES) {
    throw new Error('controlled artifact TLS serial generation failed')
  }
  const serial = Buffer.from(generated)
  try {
    serial[0] &= 0x7f
    if (serial.every((value) => value === 0)) serial[serial.length - 1] = 1
    return serial.toString('hex')
  } finally {
    serial.fill(0)
  }
}

function cloneValidDate(value: Date): Date {
  if (!(value instanceof Date) || !Number.isFinite(value.getTime())) {
    throw new Error('controlled artifact TLS clock is invalid')
  }
  return new Date(value.getTime())
}

function encodePEMBuffer(der: Buffer, label: string): Buffer {
  if (!Buffer.isBuffer(der) || der.length === 0 || !/^[A-Z][A-Z ]{0,31}$/.test(label)) {
    throw new Error('controlled artifact TLS private key encoding failed')
  }
  const header = Buffer.from(`-----BEGIN ${label}-----\n`, 'ascii')
  const footer = Buffer.from(`-----END ${label}-----\n`, 'ascii')
  const encodedLength = Math.ceil(der.length / 3) * 4
  const lineBreaks = Math.ceil(encodedLength / 64)
  const pem = Buffer.allocUnsafe(header.length + encodedLength + lineBreaks + footer.length)
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
  let offset = 0
  let column = 0
  header.copy(pem, offset)
  offset += header.length

  const writeCharacter = (value: number): void => {
    pem[offset] = value
    offset += 1
    column += 1
    if (column === 64) {
      pem[offset] = 0x0a
      offset += 1
      column = 0
    }
  }

  for (let index = 0; index < der.length; index += 3) {
    const first = der[index]
    const hasSecond = index + 1 < der.length
    const hasThird = index + 2 < der.length
    const second = hasSecond ? der[index + 1] : 0
    const third = hasThird ? der[index + 2] : 0
    writeCharacter(alphabet.charCodeAt(first >>> 2))
    writeCharacter(alphabet.charCodeAt(((first & 0x03) << 4) | (second >>> 4)))
    writeCharacter(hasSecond ? alphabet.charCodeAt(((second & 0x0f) << 2) | (third >>> 6)) : 0x3d)
    writeCharacter(hasThird ? alphabet.charCodeAt(third & 0x3f) : 0x3d)
  }
  if (column !== 0) {
    pem[offset] = 0x0a
    offset += 1
  }
  footer.copy(pem, offset)
  offset += footer.length
  if (offset !== pem.length) {
    pem.fill(0)
    throw new Error('controlled artifact TLS private key encoding failed')
  }
  return pem
}
