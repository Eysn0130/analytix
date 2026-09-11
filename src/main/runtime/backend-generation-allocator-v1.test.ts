import { chmodSync, mkdtempSync, mkdirSync, realpathSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  BackendGenerationAllocatorV1,
  parseBackendGenerationConsumedMarkerV1,
  type BackendGenerationAuthorityExecutorV1
} from './backend-generation-allocator-v1'

describe('backend generation allocator V1', () => {
  it('parses only one canonical closed marker', () => {
    const marker = markerV1(17, 'a'.repeat(64))
    expect(parseBackendGenerationConsumedMarkerV1(marker)).toEqual({
      generation: 17,
      allocationRecordDigest: 'a'.repeat(64)
    })
    for (const invalid of [
      marker.trim(),
      `noise\n${marker}`,
      marker.replace('"generation":17', '"generation":17.0'),
      marker.replace('"generation":17', '"generation":1e1'),
      marker.replace('"generation":17', '"generation":17,"generation":18'),
      marker.replace('}', ',"extra":true}'),
      marker.replace('"schemaVersion":1,"generation":17', '"generation":17,"schemaVersion":1')
    ]) {
      expect(() => parseBackendGenerationConsumedMarkerV1(invalid)).toThrow(
        'Backend generation authority is unavailable.'
      )
    }
  })

  it('serializes consumption and invokes the exact binary without ambient environment authority', async () => {
    const fixture = allocatorFixtureV1()
    let generation = 0
    let active = 0
    let maximumActive = 0
    const calls: Array<{ command: string; args: readonly string[] }> = []
    const execute: BackendGenerationAuthorityExecutorV1 = async (command, args) => {
      calls.push({ command, args })
      active += 1
      maximumActive = Math.max(maximumActive, active)
      await Promise.resolve()
      generation += 1
      active -= 1
      return { stdout: markerV1(generation, generation.toString(16).padStart(64, '0')), stderr: '' }
    }
    const allocator = new BackendGenerationAllocatorV1({ ...fixture, execute })
    const [first, second] = await Promise.all([allocator.consume(), allocator.consume()])
    expect(first.generation).toBe(1)
    expect(second.generation).toBe(2)
    expect(maximumActive).toBe(1)
    expect(calls).toEqual([
      { command: fixture.runtimeServerPath, args: ['authority', 'consume-backend-generation-v1', '--user-data-dir', fixture.userDataRealPath] },
      { command: fixture.runtimeServerPath, args: ['authority', 'consume-backend-generation-v1', '--user-data-dir', fixture.userDataRealPath] }
    ])
    expect(allocator.executableSHA256()).toMatch(/^[a-f0-9]{64}$/)
    expect(allocator.runtimeServerRealPath()).toBe(fixture.runtimeServerPath)
    expect(() => allocator.assertExecutableIdentity()).not.toThrow()
  })

  it('redacts executor output and rejects executable replacement', async () => {
    const fixture = allocatorFixtureV1()
    const sentinel = 'ACCOUNT-001234567890'
    const allocator = new BackendGenerationAllocatorV1({
      ...fixture,
      execute: async () => { throw new Error(sentinel) }
    })
    let failure: unknown
    try {
      await allocator.consume()
    } catch (error) {
      failure = error
    }
    expect(failure).toBeInstanceOf(Error)
    expect((failure as Error).message).toBe('Backend generation authority is unavailable.')
    expect((failure as Error).message).not.toContain(sentinel)

    const replaced = new BackendGenerationAllocatorV1({
      ...fixture,
      execute: async () => ({ stdout: markerV1(1, 'a'.repeat(64)), stderr: '' })
    })
    writeFileSync(fixture.runtimeServerPath, 'replacement-binary', { mode: 0o700 })
    expect(() => replaced.assertExecutableIdentity()).toThrow('Backend generation authority is unavailable.')
    await expect(replaced.consume()).rejects.toThrow('Backend generation authority is unavailable.')
  })
})

function markerV1(generation: number, digest: string): string {
  return `ANALYTIX_BACKEND_GENERATION_CONSUMED ${JSON.stringify({
    schemaVersion: 1,
    generation,
    allocationRecordDigest: digest
  })}\n`
}

function allocatorFixtureV1(): { runtimeServerPath: string; userDataRealPath: string } {
  const root = mkdtempSync(join(tmpdir(), 'analytix-backend-generation-'))
  const runtimeServerPath = join(root, 'runtime-server')
  const userDataRealPath = join(root, 'user-data')
  writeFileSync(runtimeServerPath, 'test-runtime-server', { mode: 0o700 })
  chmodSync(runtimeServerPath, 0o700)
  mkdirSync(userDataRealPath, { mode: 0o700 })
  return {
    runtimeServerPath: realpathSync(runtimeServerPath),
    userDataRealPath: realpathSync(userDataRealPath)
  }
}
