import { PassThrough } from 'node:stream'
import { describe, expect, it } from 'vitest'
import {
  encodeRuntimeStartupPrivateFrameV1,
  writeRuntimeStartupPrivateFrameV1
} from './runtime-startup-private-frame-v1'

const FRAME_ERROR = 'Runtime private startup frame is unavailable.'
const invalidDirectories: readonly (readonly [string, unknown])[] = [
  ['repeated separator', '/private/development//provider-credentials'],
  ['dot component', '/private/./development/provider-credentials'],
  ['leading repeated separator', '//private/development/provider-credentials'],
  ['root dot component', '/./provider-credentials'],
  ['array instead of string', ['/private/development/provider-credentials']]
]

describe('development credential directory private-frame preflight', () => {
  it.each(invalidDirectories)('rejects %s without normalizing authority', (_name, directory) => {
    expect(() => encodeRuntimeStartupPrivateFrameV1({
      developmentProviderAuthorityDir: directory
    } as never)).toThrow(FRAME_ERROR)
  })

  it.each(invalidDirectories)('does not emit stdin bytes for %s', async (_name, directory) => {
    const stdin = new PassThrough()
    let bytes = 0
    stdin.on('data', (chunk: Buffer) => { bytes += chunk.length })
    try {
      await expect(writeRuntimeStartupPrivateFrameV1(stdin, {
        developmentProviderAuthorityDir: directory
      } as never)).rejects.toThrow(FRAME_ERROR)
      expect(bytes).toBe(0)
      expect(stdin.writableEnded).toBe(false)
    } finally {
      stdin.destroy()
    }
  })

  it('preserves canonical hidden-directory bytes and the exact length boundary', () => {
    const suffix = '/provider-credentials'
    for (const root of [
      '/Users/test/.analytix-development/provider-credentials',
      `/${'a'.repeat(1024 - suffix.length - 1)}${suffix}`
    ]) {
      const frame = encodeRuntimeStartupPrivateFrameV1({ developmentProviderAuthorityDir: root })
      try {
        expect(Number(frame.readBigUInt64BE(0))).toBe(frame.length - 8)
        expect(frame.subarray(8).toString('utf8')).toBe(JSON.stringify({
          schemaVersion: 1,
          purpose: 'analytix.runtime-startup-private-frame/v1',
          developmentProviderAuthorityDir: root
        }))
      } finally {
        frame.fill(0)
      }
    }
    expect(() => encodeRuntimeStartupPrivateFrameV1({
      developmentProviderAuthorityDir: `/${'a'.repeat(1025 - suffix.length - 1)}${suffix}`
    })).toThrow(FRAME_ERROR)
  })

  it('rejects an object without invoking its string coercion', () => {
    let coerced = false
    const directory = {
      toString() { coerced = true; return '/private/development/provider-credentials' }
    }
    expect(() => encodeRuntimeStartupPrivateFrameV1({
      developmentProviderAuthorityDir: directory
    } as never)).toThrow(FRAME_ERROR)
    expect(coerced).toBe(false)
  })
})
