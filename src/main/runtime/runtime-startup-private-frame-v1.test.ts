import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { PassThrough } from 'node:stream'
import { describe, expect, it } from 'vitest'
import { MAX_RUNTIME_STARTUP_PRIVATE_FRAME_BYTES_V1 } from '../claw-schedule-mcp-config'
import type { MainOwnedRuntimeAuthorityEnvelopeV1 } from './main-owned-authority-envelope-v1'
import {
  encodeRuntimeStartupPrivateFrameV1,
  selectRuntimeHostScheduleMcpBindingV1,
  writeRuntimeStartupPrivateFrameV1
} from './runtime-startup-private-frame-v1'

describe('runtime startup private frame v1', () => {
  it('encodes one closed canonical authority document without argv or environment names', () => {
    const frame = encodeRuntimeStartupPrivateFrameV1({
      protectedAuthorityV1: authorityFixtureV1()
    })
    try {
      expect(Number(frame.readBigUInt64BE(0))).toBe(frame.length - 8)
      const bodyText = frame.subarray(8).toString('utf8')
      expect(bodyText).not.toContain('ANALYTIX_AUTHORITY_')
      const expectedBody = {
        schemaVersion: 1,
        purpose: 'analytix.runtime-startup-private-frame/v1',
        protectedAuthorityV1: {
          schemaVersion: 1,
          purpose: 'analytix.runtime-main-owned-authority/v1',
          authorityAnchorV1: JSON.parse(authorityFixtureV1().authorityAnchorV1),
          authorityManifestRoot: '/private/authority/manifest',
          authorityCredentialProfileRoot: '/private/authority/profile',
          authorityCredentialBundleRoot: '/private/authority/bundle'
        }
      }
      expect(bodyText).toBe(JSON.stringify(expectedBody))
      expect(JSON.parse(bodyText)).toEqual(expectedBody)
    } finally {
      frame.fill(0)
    }
  })

  it('preserves the legacy authority-only JSON bytes for special path characters', () => {
    const authority = {
      ...authorityFixtureV1(),
      authorityManifestRoot: '/private/R&D/<manifest>\u2028middle\u2029root'
    }
    const expectedBody = JSON.stringify({
      schemaVersion: 1,
      purpose: 'analytix.runtime-startup-private-frame/v1',
      protectedAuthorityV1: {
        schemaVersion: 1,
        purpose: 'analytix.runtime-main-owned-authority/v1',
        authorityAnchorV1: JSON.parse(authority.authorityAnchorV1),
        authorityManifestRoot: authority.authorityManifestRoot,
        authorityCredentialProfileRoot: authority.authorityCredentialProfileRoot,
        authorityCredentialBundleRoot: authority.authorityCredentialBundleRoot
      }
    })
    const frame = encodeRuntimeStartupPrivateFrameV1({ protectedAuthorityV1: authority })
    try {
      expect(frame.subarray(8).toString('utf8')).toBe(expectedBody)
      expect(expectedBody).toContain('R&D/<manifest>\u2028middle\u2029root')
    } finally {
      frame.fill(0)
    }
  })

  it('carries the explicit development directory and rejects malformed authority paths', () => {
    const root = '/private/development/provider-credentials'
    const frame = encodeRuntimeStartupPrivateFrameV1({ developmentProviderAuthorityDir: root })
    expect(frame.subarray(8).toString('utf8')).toBe(JSON.stringify({
      schemaVersion: 1,
      purpose: 'analytix.runtime-startup-private-frame/v1',
      developmentProviderAuthorityDir: root
    }))
    for (const invalid of ['relative/provider-credentials', '/private/../provider-credentials', '/private/wrong']) {
      expect(() => encodeRuntimeStartupPrivateFrameV1({ developmentProviderAuthorityDir: invalid })).toThrow()
    }
  })

  it('writes exactly one frame and closes private stdin', async () => {
    const stdin = new PassThrough()
    const chunks: Buffer[] = []
    stdin.on('data', (chunk: Buffer) => chunks.push(Buffer.from(chunk)))
    await writeRuntimeStartupPrivateFrameV1(stdin, {
      protectedAuthorityV1: authorityFixtureV1()
    })
    const written = Buffer.concat(chunks)
    try {
      expect(stdin.writableEnded).toBe(true)
      expect(Number(written.readBigUInt64BE(0))).toBe(written.length - 8)
      expect(written.subarray(8).toString('utf8').match(/"protectedAuthorityV1"/gu)).toHaveLength(1)
    } finally {
      written.fill(0)
      chunks.forEach((chunk) => chunk.fill(0))
    }
  })

  it('binds one exact host schedule helper without using argv or ambient environment', () => {
    const hostScheduleMcpBindingV1 = scheduleBindingFixtureV1()
    const scheduleOnly = encodeRuntimeStartupPrivateFrameV1({ hostScheduleMcpBindingV1 })
    const combined = encodeRuntimeStartupPrivateFrameV1({
      protectedAuthorityV1: authorityFixtureV1(),
      hostScheduleMcpBindingV1
    })
    try {
      expect(JSON.parse(scheduleOnly.subarray(8).toString('utf8'))).toEqual({
        schemaVersion: 1,
        purpose: 'analytix.runtime-startup-private-frame/v1',
        hostScheduleMcpBindingV1
      })
      const combinedBody = JSON.parse(combined.subarray(8).toString('utf8'))
      expect(combinedBody.protectedAuthorityV1).toBeDefined()
      expect(combinedBody.hostScheduleMcpBindingV1).toEqual(hostScheduleMcpBindingV1)
    } finally {
      scheduleOnly.fill(0)
      combined.fill(0)
    }
  })

  it('uses Go-compatible string encoding for the exact schedule binding', () => {
    const hostScheduleMcpBindingV1 = {
      ...scheduleBindingFixtureV1(),
      command: '/Applications/R&D/Analytix Helper',
      args: [
        '/Applications/R&D/claw-schedule-mcp-node-entry.js',
        '--gui-schedule-mcp-server',
        '--base-url',
        'http://127.0.0.1:9787',
        '--secret',
        'secret<>&\u2028middle\u2029value'
      ]
    }
    const frame = encodeRuntimeStartupPrivateFrameV1({ hostScheduleMcpBindingV1 })
    try {
      const bodyText = frame.subarray(8).toString('utf8')
      expect(bodyText).toContain('R\\u0026D')
      expect(bodyText).toContain(
        'secret\\u003c\\u003e\\u0026\\u2028middle\\u2029value'
      )
      expect(JSON.parse(bodyText).hostScheduleMcpBindingV1).toEqual(
        hostScheduleMcpBindingV1
      )
    } finally {
      frame.fill(0)
    }
  })

  it('drops only a schedule binding when the actual combined authority frame exceeds budget', () => {
    let largestAccepted: ReturnType<typeof scheduleBindingFixtureV1> | null = null
    let lower = 1
    let upper = MAX_RUNTIME_STARTUP_PRIVATE_FRAME_BYTES_V1
    while (lower <= upper) {
      const length = Math.floor((lower + upper) / 2)
      const candidate = {
        ...scheduleBindingFixtureV1(),
        args: [
          ...scheduleBindingFixtureV1().args,
          '--secret',
          'a'.repeat(length)
        ]
      }
      if (selectRuntimeHostScheduleMcpBindingV1(null, candidate)) {
        largestAccepted = candidate
        lower = length + 1
      } else {
        upper = length - 1
      }
    }

    expect(largestAccepted).not.toBeNull()
    expect(selectRuntimeHostScheduleMcpBindingV1(
      authorityFixtureV1(),
      largestAccepted
    )).toBeNull()
    expect(selectRuntimeHostScheduleMcpBindingV1(
      authorityFixtureV1(),
      scheduleBindingFixtureV1()
    )).not.toBeNull()
    expect(() => encodeRuntimeStartupPrivateFrameV1({
      protectedAuthorityV1: authorityFixtureV1()
    })).not.toThrow()
  })

  it('preflights the combined frame only after the final production config sync', () => {
    const source = readFileSync(new URL('./analytix-adapter.ts', import.meta.url), 'utf8')
    const start = source.indexOf('async function startGoConformanceSidecarOnce')
    const body = source.slice(start)
    const deferredSync = body.indexOf(
      'if (!bundledFundsConfigSynced) await syncRuntimeConfig()'
    )
    const directBinding = Math.max(
      body.indexOf('hostScheduleMcpBindingV1 = synced.hostScheduleMcpBindingV1'),
      body.indexOf('hostScheduleMcpBindingV1 = syncedConfig.hostScheduleMcpBindingV1')
    )
    const bindingReady = Math.max(deferredSync, directBinding)
    const preflight = body.indexOf(
      'hostScheduleMcpBindingV1 = selectRuntimeHostScheduleMcpBindingV1('
    )
    const frameDecision = body.indexOf('hasPrivateStartupFrame = Boolean(', preflight)
    const spawn = body.indexOf('spawn(', frameDecision)

    expect(start).toBeGreaterThanOrEqual(0)
    expect(source).not.toContain('resolveDarwinSecretStoreKeychainBindingV1(')
    expect(bindingReady).toBeGreaterThanOrEqual(0)
    expect(preflight).toBeGreaterThan(bindingReady)
    expect(frameDecision).toBeGreaterThan(preflight)
    expect(spawn).toBeGreaterThan(frameDecision)
  })

  it('rejects an empty or malformed startup authority frame', () => {
    expect(() => encodeRuntimeStartupPrivateFrameV1({})).toThrow(
      'Runtime private startup frame is unavailable.'
    )
    expect(() => encodeRuntimeStartupPrivateFrameV1({
      protectedAuthorityV1: {
        ...authorityFixtureV1(),
        authorityManifestRoot: 'relative'
      }
    })).toThrow('Runtime private startup frame is unavailable.')
    expect(() => encodeRuntimeStartupPrivateFrameV1({
      hostScheduleMcpBindingV1: {
        ...scheduleBindingFixtureV1(),
        args: [
          '/Applications/Analytix.app/Contents/Resources/app.asar/out/main/claw-schedule-mcp-node-entry.js',
          '--gui-schedule-mcp-server',
          '--base-url',
          'http://127.0.0.1:9787/other'
        ]
      }
    })).toThrow('Runtime private startup frame is unavailable.')
    expect(() => encodeRuntimeStartupPrivateFrameV1({
      protectedAuthorityV1: authorityFixtureV1(),
      unknownExpansion: true
    } as never)).toThrow('Runtime private startup frame is unavailable.')
    expect(() => encodeRuntimeStartupPrivateFrameV1({
      protectedAuthorityV1: null,
      hostScheduleMcpBindingV1: scheduleBindingFixtureV1()
    } as never)).toThrow('Runtime private startup frame is unavailable.')
    expect(() => encodeRuntimeStartupPrivateFrameV1({
      hostScheduleMcpBindingV1: {
        ...scheduleBindingFixtureV1(),
        args: [
          '/Applications/Analytix.app/Contents/Resources/app.asar/out/main/claw-schedule-mcp-node-entry.js',
          '--gui-schedule-mcp-server',
          '--base-url',
          'http://127.0.0.1:9787',
          '--secret',
          'invalid-\ud800'
        ]
      }
    })).toThrow('Runtime private startup frame is unavailable.')
  })
})

function scheduleBindingFixtureV1() {
  return {
    schemaVersion: 1 as const,
    purpose: 'analytix.runtime-host-schedule-mcp-binding/v1' as const,
    serverId: 'gui_schedule' as const,
    command: '/Applications/Analytix.app/Contents/Frameworks/Analytix Helper.app/Contents/MacOS/Analytix Helper',
    args: [
      '/Applications/Analytix.app/Contents/Resources/app.asar/out/main/claw-schedule-mcp-node-entry.js',
      '--gui-schedule-mcp-server',
      '--base-url',
      'http://127.0.0.1:9787'
    ],
    env: { ELECTRON_RUN_AS_NODE: '1' as const },
    trustScope: 'user' as const,
    timeoutMs: 5_000 as const
  }
}

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
