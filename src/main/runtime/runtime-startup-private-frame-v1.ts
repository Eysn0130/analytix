import type { Writable } from 'node:stream'
import {
  goCompatibleJSONStringifyV1,
  MAX_RUNTIME_STARTUP_PRIVATE_FRAME_BYTES_V1,
  RUNTIME_STARTUP_PRIVATE_FRAME_PURPOSE_V1,
  validateRuntimeHostScheduleMcpBindingV1,
  type RuntimeHostScheduleMcpBindingV1
} from '../claw-schedule-mcp-config'
import {
  authorityAnchorProjectionV1,
  validateMainOwnedRuntimeAuthorityEnvelopeV1,
  type MainOwnedRuntimeAuthorityEnvelopeV1
} from './main-owned-authority-envelope-v1'
import {
  validateDarwinSecretStoreKeychainBindingDocumentV1,
  type DarwinSecretStoreKeychainBindingV1
} from './darwin-secret-store-keychain-binding-v1'

const STARTUP_FRAME_ERROR = 'Runtime private startup frame is unavailable.'

export type RuntimeStartupPrivateFrameInputV1 = Readonly<{
  developmentProviderAuthorityDir?: string
  protectedAuthorityV1?: MainOwnedRuntimeAuthorityEnvelopeV1
  hostScheduleMcpBindingV1?: RuntimeHostScheduleMcpBindingV1
  darwinSecretStoreKeychainBindingV1?: DarwinSecretStoreKeychainBindingV1
}>

export function selectRuntimeHostScheduleMcpBindingV1(
  authority: MainOwnedRuntimeAuthorityEnvelopeV1 | null,
  binding: RuntimeHostScheduleMcpBindingV1 | null,
  darwinSecretStoreKeychainBindingV1: DarwinSecretStoreKeychainBindingV1 | null = null
): RuntimeHostScheduleMcpBindingV1 | null {
  if (!binding) return null
  let frame: Buffer | null = null
  try {
    const normalized = validateRuntimeHostScheduleMcpBindingV1(binding)
    frame = encodeRuntimeStartupPrivateFrameV1({
      ...(authority ? { protectedAuthorityV1: authority } : {}),
      ...(darwinSecretStoreKeychainBindingV1 ? { darwinSecretStoreKeychainBindingV1 } : {}),
      hostScheduleMcpBindingV1: normalized
    })
    return normalized
  } catch {
    return null
  } finally {
    frame?.fill(0)
  }
}

/**
 * Encodes the sole closed private startup document accepted by the production
 * Go runtime. Authority, the exact host schedule projection, and the validated
 * Darwin task Keychain binding or explicit source-development credential
 * directory are the accepted capabilities; unknown fields remain closed.
 */
export function encodeRuntimeStartupPrivateFrameV1(
  input: RuntimeStartupPrivateFrameInputV1
): Buffer {
  if (!input || typeof input !== 'object' || Array.isArray(input)) {
    throw fixedStartupFrameErrorV1()
  }
  const inputRecord = input as Record<string, unknown>
  const inputKeys = Object.keys(inputRecord).sort()
  const allowedInputKeys = new Set([
    'hostScheduleMcpBindingV1',
    'protectedAuthorityV1',
    'darwinSecretStoreKeychainBindingV1',
    'developmentProviderAuthorityDir'
  ])
  if (
    inputKeys.length === 0 || inputKeys.length > allowedInputKeys.size ||
    inputKeys.some((key) => !allowedInputKeys.has(key) || inputRecord[key] == null) ||
    Object.getOwnPropertySymbols(input).length !== 0
  ) {
    throw fixedStartupFrameErrorV1()
  }
  let authority: MainOwnedRuntimeAuthorityEnvelopeV1 | null = null
  try {
    if (Object.prototype.hasOwnProperty.call(input, 'protectedAuthorityV1')) {
      authority = validateMainOwnedRuntimeAuthorityEnvelopeV1(
        input.protectedAuthorityV1 as MainOwnedRuntimeAuthorityEnvelopeV1
      )
    }
  } catch {
    throw fixedStartupFrameErrorV1()
  }
  let hostScheduleMcpBinding: RuntimeHostScheduleMcpBindingV1 | null = null
  let darwinSecretStoreKeychainBinding: DarwinSecretStoreKeychainBindingV1 | null = null
  try {
    if (Object.prototype.hasOwnProperty.call(input, 'hostScheduleMcpBindingV1')) {
      hostScheduleMcpBinding = validateRuntimeHostScheduleMcpBindingV1(
        input.hostScheduleMcpBindingV1
      )
    }
  } catch {
    throw fixedStartupFrameErrorV1()
  }
  try {
    if (Object.prototype.hasOwnProperty.call(input, 'darwinSecretStoreKeychainBindingV1')) {
      darwinSecretStoreKeychainBinding = validateDarwinSecretStoreKeychainBindingDocumentV1(
        input.darwinSecretStoreKeychainBindingV1 as DarwinSecretStoreKeychainBindingV1
      )
    }
  } catch {
    throw fixedStartupFrameErrorV1()
  }
  const developmentProviderAuthorityDir = input.developmentProviderAuthorityDir
  if (developmentProviderAuthorityDir !== undefined && (darwinSecretStoreKeychainBinding ||
    !/^\/[A-Za-z0-9._/-]+\/provider-credentials$/.test(developmentProviderAuthorityDir) ||
    developmentProviderAuthorityDir.includes('/../') || developmentProviderAuthorityDir.length > 1024)) throw fixedStartupFrameErrorV1()
  if (!authority && !hostScheduleMcpBinding && !darwinSecretStoreKeychainBinding && !developmentProviderAuthorityDir) {
    throw fixedStartupFrameErrorV1()
  }

  let body: Buffer | null = null
  try {
    const protectedAuthorityV1 = authority
      ? encodeProtectedAuthorityDocumentV1(authority)
      : null
    const document = {
      schemaVersion: 1,
      purpose: RUNTIME_STARTUP_PRIVATE_FRAME_PURPOSE_V1,
      ...(protectedAuthorityV1 ? { protectedAuthorityV1 } : {}),
      ...(darwinSecretStoreKeychainBinding
        ? { darwinSecretStoreKeychainBindingV1: darwinSecretStoreKeychainBinding }
        : {}),
      ...(developmentProviderAuthorityDir ? { developmentProviderAuthorityDir } : {}),
      ...(hostScheduleMcpBinding
        ? { hostScheduleMcpBindingV1: hostScheduleMcpBinding }
        : {})
    }
    const encoded = hostScheduleMcpBinding || darwinSecretStoreKeychainBinding
      ? goCompatibleJSONStringifyV1(document)
      : JSON.stringify(document)
    body = Buffer.from(encoded, 'utf8')
    if (body.length === 0 || body.length > MAX_RUNTIME_STARTUP_PRIVATE_FRAME_BYTES_V1) {
      throw fixedStartupFrameErrorV1()
    }
    const frame = Buffer.alloc(8 + body.length)
    frame.writeBigUInt64BE(BigInt(body.length), 0)
    body.copy(frame, 8)
    return frame
  } catch {
    throw fixedStartupFrameErrorV1()
  } finally {
    body?.fill(0)
  }
}

export async function writeRuntimeStartupPrivateFrameV1(
  stdin: Writable | null,
  input: RuntimeStartupPrivateFrameInputV1
): Promise<void> {
  if (!stdin) throw fixedStartupFrameErrorV1()
  const frame = encodeRuntimeStartupPrivateFrameV1(input)
  try {
    await new Promise<void>((resolveWrite, rejectWrite) => {
      let settled = false
      const finish = (error?: Error): void => {
        if (settled) return
        settled = true
        stdin.off('error', onError)
        if (error) rejectWrite(fixedStartupFrameErrorV1())
        else resolveWrite()
      }
      const onError = (): void => finish(fixedStartupFrameErrorV1())
      stdin.once('error', onError)
      stdin.end(frame, () => finish())
    })
  } finally {
    frame.fill(0)
  }
}

function encodeProtectedAuthorityDocumentV1(
  authority: MainOwnedRuntimeAuthorityEnvelopeV1
): Record<string, unknown> {
  const anchor = authorityAnchorProjectionV1(authority.authorityAnchorV1)
  return {
    schemaVersion: 1,
    purpose: 'analytix.runtime-main-owned-authority/v1',
    authorityAnchorV1: {
      schemaVersion: 1,
      installationId: anchor.installationId,
      authorityKeyId: anchor.authorityKeyId,
      authorityPublicKey: anchor.authorityPublicKey,
      currentManifestDigest: anchor.currentManifestDigest
    },
    authorityManifestRoot: authority.authorityManifestRoot,
    authorityCredentialProfileRoot: authority.authorityCredentialProfileRoot,
    authorityCredentialBundleRoot: authority.authorityCredentialBundleRoot
  }
}

function fixedStartupFrameErrorV1(): Error {
  return new Error(STARTUP_FRAME_ERROR)
}
