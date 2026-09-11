import { describe, expect, it } from 'vitest'
import {
  controlledArtifactSidecarLaunchBindingProofV2,
  verifyControlledArtifactSidecarLaunchBindingProofV2,
  type ControlledArtifactSidecarLaunchBindingV2
} from './sidecar-launch-binding-v2'

describe('controlled artifact sidecar launch binding V2', () => {
  it('matches the Go vector and binds every launch authority field', () => {
    const secret = Buffer.from('0123456789abcdef0123456789abcdef').toString('base64url')
    const binding: ControlledArtifactSidecarLaunchBindingV2 = {
      runtimeURL: 'http://127.0.0.1:45123/',
      runtimePID: 4242,
      controlledArtifactHostURL: 'https://127.0.0.1:45124',
      backendGeneration: 17,
      allocationRecordDigest: 'd'.repeat(64),
      tlsRootCertificateSHA256: 'b'.repeat(64),
      tlsLeafSPKISHA256: 'a'.repeat(64),
      controlledArtifactHostReady: true,
      runtimeTokenConfigured: true,
      persistenceRootsConfigured: true,
      productionRuntime: true
    }
    const proof = controlledArtifactSidecarLaunchBindingProofV2(secret, binding)
    expect(proof).toBe('5e055acc7f72b3c10dba94edd2375e1d22d3e62d05ac6c04b8cd1a1863ff5431')
    expect(verifyControlledArtifactSidecarLaunchBindingProofV2(secret, binding, proof)).toBe(true)
    expect(verifyControlledArtifactSidecarLaunchBindingProofV2(secret, {
      ...binding,
      backendGeneration: 18
    }, proof)).toBe(false)
    expect(verifyControlledArtifactSidecarLaunchBindingProofV2(secret, {
      ...binding,
      tlsRootCertificateSHA256: 'c'.repeat(64)
    }, proof)).toBe(false)
  })

  it('rejects weak secrets, unsafe origins and non-production bindings', () => {
    const binding: ControlledArtifactSidecarLaunchBindingV2 = {
      runtimeURL: 'http://127.0.0.1:45123/',
      runtimePID: 4242,
      controlledArtifactHostURL: 'https://127.0.0.1:45124',
      backendGeneration: 17,
      allocationRecordDigest: 'd'.repeat(64),
      tlsRootCertificateSHA256: 'b'.repeat(64),
      tlsLeafSPKISHA256: 'a'.repeat(64),
      controlledArtifactHostReady: true,
      runtimeTokenConfigured: true,
      persistenceRootsConfigured: true,
      productionRuntime: true
    }
    expect(() => controlledArtifactSidecarLaunchBindingProofV2('weak', binding)).toThrow()
    expect(() => controlledArtifactSidecarLaunchBindingProofV2(
      Buffer.alloc(32, 1).toString('base64url'),
      { ...binding, runtimeURL: 'http://localhost:45123/' }
    )).toThrow()
  })
})
