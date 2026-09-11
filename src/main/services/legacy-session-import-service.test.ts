import { mkdir, mkdtemp, rm, stat, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  defaultLegacySourceCandidates,
  detectLegacySessions,
  importLegacySessions,
  LEGACY_SESSION_IMPORT_BLOCKED_MESSAGE
} from './legacy-session-import-service'

const tempRoots: string[] = []

async function makeTempRoot(): Promise<string> {
  const root = await mkdtemp(join(tmpdir(), 'analytix-session-import-'))
  tempRoots.push(root)
  return root
}

/** Create a thread folder with a minimal metadata.jsonl so it hydrates like the real store. */
async function writeThread(threadsDir: string, threadId: string, title = threadId): Promise<void> {
  const dir = join(threadsDir, threadId)
  await mkdir(dir, { recursive: true })
  const metadata = {
    kind: 'thread_metadata',
    version: 1,
    timestamp: '2026-06-15T00:00:00.000Z',
    thread: { id: threadId, title, turns: [] }
  }
  await writeFile(join(dir, 'metadata.jsonl'), `${JSON.stringify(metadata)}\n`, 'utf8')
  await writeFile(join(dir, 'messages.jsonl'), '', 'utf8')
  await writeFile(join(dir, 'events.jsonl'), '', 'utf8')
}

afterEach(async () => {
  while (tempRoots.length > 0) {
    const root = tempRoots.pop()
    if (root) await rm(root, { recursive: true, force: true })
  }
})

describe('defaultLegacySourceCandidates', () => {
  it('points at the legacy product analytix and coreagent threads dirs', () => {
    const candidates = defaultLegacySourceCandidates('/home/zoe')
    expect(candidates.map((c) => c.path)).toEqual([
      join('/home/zoe', '.analytix', 'analytix', 'threads'),
      join('/home/zoe', '.kun', 'data', 'threads'),
      join('/home/zoe', '.analytix', 'coreagent', 'threads')
    ])
    expect(candidates.map((c) => c.kind)).toEqual(['analytix', 'kun', 'coreagent'])
  })
})

describe('detectLegacySessions', () => {
  it('reports thread counts and how many are new vs the destination', async () => {
    const root = await makeTempRoot()
    const analytixThreads = join(root, '.analytix', 'analytix', 'threads')
    await writeThread(analytixThreads, 'thr_a')
    await writeThread(analytixThreads, 'thr_b')
    const coreagentThreads = join(root, '.analytix', 'coreagent', 'threads')
    await writeThread(coreagentThreads, 'thr_c')
    const kunThreads = join(root, '.kun', 'data', 'threads')
    await writeThread(kunThreads, 'thr_kun')
    // One of the analytix threads already exists in the destination.
    const dataDir = join(root, '.analytix', 'data')
    await writeThread(join(dataDir, 'threads'), 'thr_a')

    const detection = await detectLegacySessions({ homeDir: root, destDataDir: dataDir })

    expect(detection.destDir).toBe(join(dataDir, 'threads'))
    const analytix = detection.sources.find((s) => s.kind === 'analytix')
    expect(analytix).toMatchObject({ threadCount: 2, newCount: 1 })
    const kun = detection.sources.find((s) => s.kind === 'kun')
    expect(kun).toMatchObject({ threadCount: 1, newCount: 1 })
    const coreagent = detection.sources.find((s) => s.kind === 'coreagent')
    expect(coreagent).toMatchObject({ threadCount: 1, newCount: 1 })
  })

  it('omits sources that do not exist', async () => {
    const root = await makeTempRoot()
    await writeThread(join(root, '.analytix', 'analytix', 'threads'), 'thr_a')
    const detection = await detectLegacySessions({
      homeDir: root,
      destDataDir: join(root, '.analytix', 'data')
    })
    expect(detection.sources.map((s) => s.kind)).toEqual(['analytix'])
  })
})

describe('importLegacySessions', () => {
  it('fails closed before writing raw legacy records into the production data directory', async () => {
    const root = await makeTempRoot()
    await writeThread(join(root, '.analytix', 'analytix', 'threads'), 'thr_a')
    const dataDir = join(root, '.analytix', 'data')
    const log = vi.fn()

    await expect(importLegacySessions({
      homeDir: root,
      destDataDir: dataDir,
      log
    })).rejects.toThrow(LEGACY_SESSION_IMPORT_BLOCKED_MESSAGE)
    await expect(stat(join(dataDir, 'threads'))).rejects.toThrow()
    expect(log).toHaveBeenCalledWith(
      'legacy-session-import: blocked before filesystem mutation',
      { code: 'legacy_import_public_validation_required' }
    )
  })
})
