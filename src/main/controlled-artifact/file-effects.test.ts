import { createHash } from 'node:crypto'
import {
  chmod,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  realpath,
  rename,
  rm,
  stat,
  symlink,
  writeFile
} from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import type { ControlledArtifactReleaseEffectInputV1 } from './host'
import {
  ControlledArtifactDisplayStoreV1,
  prepareControlledArtifactExportV1
} from './file-effects'

const FULL_ACCOUNT = '6222020202020202020'

describe('controlled artifact main-process file effects', () => {
  it('atomically publishes exact full PII to a new host-selected export target', async () => {
    const root = await canonicalTempRoot('analytix-controlled-export-')
    try {
      const target = join(root, 'case-evidence.json')
      const input = effectInput('export')
      const prepared = await prepareControlledArtifactExportV1(target)

      expect(Object.keys(prepared)).toEqual(['release'])
      await prepared.release(input)

      const body = await readFile(target)
      expect(body.equals(input.body)).toBe(true)
      expect(body.toString('utf8')).toContain(FULL_ACCOUNT)
      expect((await stat(target)).mode & 0o077).toBe(0)
      expect((await readdir(root)).filter((name) => name.startsWith('.stage-'))).toEqual([])
      await expect(prepared.release(input)).rejects.toThrow('controlled_artifact_file_effect_failed')
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('rejects target and parent substitution without modifying or leaking to a victim', async () => {
    const root = await canonicalTempRoot('analytix-controlled-export-hostile-')
    try {
      const selectedParent = join(root, 'selected')
      const movedParent = join(root, 'selected-old')
      const hostileParent = join(root, 'hostile')
      await mkdir(selectedParent, { mode: 0o700 })
      await mkdir(hostileParent, { mode: 0o700 })
      const target = join(selectedParent, 'case-evidence.json')
      const prepared = await prepareControlledArtifactExportV1(target)

      await rename(selectedParent, movedParent)
      await symlink(hostileParent, selectedParent)
      await expect(prepared.release(effectInput('export'))).rejects.toThrow('controlled_artifact_file_effect_failed')
      await expect(readFile(join(hostileParent, 'case-evidence.json'))).rejects.toMatchObject({ code: 'ENOENT' })

      await rm(selectedParent)
      await rename(movedParent, selectedParent)
      const victim = join(root, 'victim.json')
      await writeFile(victim, 'untouched', { mode: 0o600 })
      const targetPrepared = await prepareControlledArtifactExportV1(target)
      await symlink(victim, target)
      await expect(targetPrepared.release(effectInput('export'))).rejects.toThrow(
        'controlled_artifact_file_effect_failed'
      )
      await expect(readFile(victim, 'utf8')).resolves.toBe('untouched')
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('stages a private display copy, requires viewer success, and removes it at shutdown', async () => {
    const root = await canonicalTempRoot('analytix-controlled-display-')
    const base = join(root, 'protected')
    let viewerPath = ''
    try {
      const store = await ControlledArtifactDisplayStoreV1.open(base, async (path) => {
        viewerPath = path
        const body = await readFile(path, 'utf8')
        expect(body).toContain(FULL_ACCOUNT)
        return ''
      })
      await store.createReleaseEffect()(effectInput('display'))

      expect(viewerPath).toMatch(/\/\.launch-[^/]+\/\.artifact-[a-f0-9]{32}-[A-Za-z0-9_-]{22}\.json$/u)
      expect((await stat(viewerPath)).mode & 0o077).toBe(0)
      await store.close()
      await expect(readFile(viewerPath)).rejects.toMatchObject({ code: 'ENOENT' })
      expect(await readdir(base)).toEqual([])
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('removes a protected display copy after viewer failure and emits only a fixed error', async () => {
    const root = await canonicalTempRoot('analytix-controlled-display-failure-')
    const base = join(root, 'protected')
    let viewerPath = ''
    try {
      const store = await ControlledArtifactDisplayStoreV1.open(base, async (path) => {
        viewerPath = path
        throw new Error(`${path}:${FULL_ACCOUNT}`)
      })
      const release = store.createReleaseEffect()
      let message = ''
      try {
        await release(effectInput('display'))
      } catch (error) {
        message = error instanceof Error ? error.message : String(error)
      }
      expect(message).toBe('controlled_artifact_file_effect_failed')
      expect(message).not.toContain(FULL_ACCOUNT)
      expect(message).not.toContain(root)
      await expect(readFile(viewerPath)).rejects.toMatchObject({ code: 'ENOENT' })
      await store.close()
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('cleans exact stale display residues but quarantines an unknown entry', async () => {
    const root = await canonicalTempRoot('analytix-controlled-display-recovery-')
    const base = join(root, 'protected')
    try {
      await mkdir(base, { recursive: true, mode: 0o700 })
      await chmod(base, 0o700)
      const stale = join(base, '.launch-stale')
      await mkdir(stale, { mode: 0o700 })
      await writeFile(
        join(stale, `.artifact-${'a'.repeat(32)}-${'b'.repeat(22)}.json`),
        FULL_ACCOUNT,
        { mode: 0o600 }
      )
      const store = await ControlledArtifactDisplayStoreV1.open(base, async () => '')
      expect((await readdir(base)).filter((name) => name === '.launch-stale')).toEqual([])
      await store.close()

      const hostile = join(base, '.launch-hostile')
      await mkdir(hostile, { mode: 0o700 })
      await writeFile(join(hostile, 'unknown.txt'), FULL_ACCOUNT, { mode: 0o600 })
      await expect(ControlledArtifactDisplayStoreV1.open(base, async () => '')).rejects.toThrow(
        'controlled_artifact_file_effect_failed'
      )
      await expect(readFile(join(hostile, 'unknown.txt'), 'utf8')).resolves.toBe(FULL_ACCOUNT)
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })
})

function effectInput(action: 'display' | 'export'): ControlledArtifactReleaseEffectInputV1 {
  const body = Buffer.from(JSON.stringify({ bankAccount: FULL_ACCOUNT }), 'utf8')
  return {
    action,
    body,
    receipt: {
      accessAction: action,
      accessId: '1'.repeat(64),
      artifactByteLength: body.length,
      artifactSha256: createHash('sha256').update(body).digest('hex')
    }
  } as ControlledArtifactReleaseEffectInputV1
}

async function canonicalTempRoot(prefix: string): Promise<string> {
  return realpath(await mkdtemp(join(tmpdir(), prefix)))
}
