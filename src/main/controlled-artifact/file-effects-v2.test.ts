import { createHash } from 'node:crypto'
import {
  lstat,
  link,
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
import type { ControlledArtifactReleaseEffectInputV2 } from './host-v2'
import { freezeControlledArtifactExportTargetV2 } from './file-effects-v2'
import { ControlledArtifactExportJournalV2 } from './export-journal-v2'

const FULL_ACCOUNT = '0006222020202020202020'
const DEADLINE = '2026-07-18T12:10:00.123456789Z'

describe('controlled artifact V2 export effects', () => {
  it('stages privately and performs one exact no-replace commit with a bound receipt', async () => {
    const root = await canonicalTempRoot('analytix-controlled-export-v2-')
    let now = '2026-07-18T12:00:00.123456789Z'
    try {
      const journal = await ControlledArtifactExportJournalV2.open(join(root, 'journal'))
      const target = join(root, 'verified-case-evidence.json')
      const selected = await freezeControlledArtifactExportTargetV2(target, journal, () => now)
      const input = effectInput(selected.targetIdentityDigest())
      expect(() => JSON.stringify(selected)).toThrow('controlled_artifact_file_effect_v2_failed')
      const prepared = await selected.createPreparation()(input, new AbortController().signal)

      await expect(readFile(target)).rejects.toMatchObject({ code: 'ENOENT' })
      const stages = (await readdir(root)).filter((name) => name.startsWith('.stage-v2-'))
      expect(stages).toHaveLength(1)
      expect((await stat(join(root, stages[0]))).mode & 0o077).toBe(0)

      now = '2026-07-18T12:00:01.123456789Z'
      const receipt = await prepared.commitBefore(DEADLINE, new AbortController().signal)
      expect(receipt).toEqual({
        releaseTargetIdentityDigest: input.releaseTargetIdentityDigest,
        artifactSha256: input.receipt.artifactSha256,
        artifactByteLength: input.body.length,
        mediaType: input.receipt.mediaType,
        committedAt: now
      })
      expect(input.receipt.targetIdentityDigest).not.toBe(input.releaseTargetIdentityDigest)
      const body = await readFile(target)
      expect(body.equals(input.body)).toBe(true)
      expect(body.toString('utf8')).toContain(FULL_ACCOUNT)
      body.fill(0)
      expect((await stat(target)).mode & 0o077).toBe(0)
      expect((await readdir(root)).filter((name) => name.startsWith('.stage-v2-'))).toEqual([])
      await expect(prepared.commitBefore(DEADLINE, new AbortController().signal)).rejects.toThrow(
        'controlled_artifact_file_effect_v2_failed'
      )
      await prepared.abort(new AbortController().signal)
      await expect(readFile(target)).resolves.toBeInstanceOf(Buffer)
      await journal.close()
      input.body.fill(0)
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('removes exact staging on abort and never creates the selected target', async () => {
    const root = await canonicalTempRoot('analytix-controlled-export-v2-abort-')
    try {
      const journal = await ControlledArtifactExportJournalV2.open(join(root, 'journal'))
      const target = join(root, 'never-published.json')
      const selected = await freezeControlledArtifactExportTargetV2(
        target,
        journal,
        () => '2026-07-18T12:00:00.123456789Z'
      )
      const input = effectInput(selected.targetIdentityDigest())
      const prepared = await selected.createPreparation()(input, new AbortController().signal)
      expect((await readdir(root)).some((name) => name.startsWith('.stage-v2-'))).toBe(true)

      await prepared.abort(new AbortController().signal)
      expect((await readdir(root)).filter((name) => name.startsWith('.stage-v2-'))).toEqual([])
      await expect(readFile(target)).rejects.toMatchObject({ code: 'ENOENT' })
      await journal.close()
      input.body.fill(0)
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('rejects parent substitution before staging any protected byte', async () => {
    const root = await canonicalTempRoot('analytix-controlled-export-v2-parent-race-')
    try {
      const journal = await ControlledArtifactExportJournalV2.open(join(root, 'journal'))
      const selectedParent = join(root, 'selected')
      const movedParent = join(root, 'selected-old')
      const hostileParent = join(root, 'hostile')
      await import('node:fs/promises').then(({ mkdir }) => Promise.all([
        mkdir(selectedParent, { mode: 0o700 }),
        mkdir(hostileParent, { mode: 0o700 })
      ]))
      const target = join(selectedParent, 'case-evidence.json')
      const selected = await freezeControlledArtifactExportTargetV2(target, journal)
      await rename(selectedParent, movedParent)
      await symlink(hostileParent, selectedParent)
      const input = effectInput(selected.targetIdentityDigest())

      let message = ''
      try {
        await selected.createPreparation()(input, new AbortController().signal)
      } catch (error) {
        message = error instanceof Error ? error.message : String(error)
      }
      expect(message).toBe('controlled_artifact_file_effect_v2_failed')
      expect(message).not.toContain(root)
      expect(message).not.toContain(FULL_ACCOUNT)
      expect(await readdir(hostileParent)).toEqual([])
      await journal.close()
      input.body.fill(0)
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('rejects target substitution after staging without changing the victim', async () => {
    const root = await canonicalTempRoot('analytix-controlled-export-v2-target-race-')
    try {
      const journal = await ControlledArtifactExportJournalV2.open(join(root, 'journal'))
      const target = join(root, 'case-evidence.json')
      const victim = join(root, 'victim.json')
      const selected = await freezeControlledArtifactExportTargetV2(target, journal)
      const input = effectInput(selected.targetIdentityDigest())
      const prepared = await selected.createPreparation()(input, new AbortController().signal)
      await writeFile(victim, 'untouched', { mode: 0o600 })
      await symlink(victim, target)

      await expect(prepared.commitBefore(DEADLINE, new AbortController().signal)).rejects.toThrow(
        'controlled_artifact_file_effect_v2_failed'
      )
      await prepared.abort(new AbortController().signal)
      await expect(readFile(victim, 'utf8')).resolves.toBe('untouched')
      expect((await readdir(root)).filter((name) => name.startsWith('.stage-v2-'))).toEqual([])
      await journal.close()
      input.body.fill(0)
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('cleans staging and publishes zero bytes when authorization expires before commit', async () => {
    const root = await canonicalTempRoot('analytix-controlled-export-v2-expired-')
    let now = '2026-07-18T12:00:00.123456789Z'
    try {
      const journal = await ControlledArtifactExportJournalV2.open(join(root, 'journal'))
      const target = join(root, 'expired.json')
      const selected = await freezeControlledArtifactExportTargetV2(target, journal, () => now)
      const input = effectInput(selected.targetIdentityDigest())
      const prepared = await selected.createPreparation()(input, new AbortController().signal)
      now = DEADLINE

      await expect(prepared.commitBefore(DEADLINE, new AbortController().signal)).rejects.toThrow(
        'controlled_artifact_file_effect_v2_failed'
      )
      await prepared.abort(new AbortController().signal)
      await expect(lstat(target)).rejects.toMatchObject({ code: 'ENOENT' })
      expect((await readdir(root)).filter((name) => name.startsWith('.stage-v2-'))).toEqual([])
      await journal.close()
      input.body.fill(0)
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('recovers a crash-staged artifact without retaining PII or journal residue', async () => {
    const root = await canonicalTempRoot('analytix-controlled-export-v2-recovery-stage-')
    const journalRoot = join(root, 'journal')
    try {
      const journal = await ControlledArtifactExportJournalV2.open(journalRoot)
      const target = join(root, 'recovered-never-published.json')
      const selected = await freezeControlledArtifactExportTargetV2(target, journal)
      const input = effectInput(selected.targetIdentityDigest())
      await selected.createPreparation()(input, new AbortController().signal)
      const stage = (await readdir(root)).find((name) => name.startsWith('.stage-v2-'))
      expect(stage).toBeDefined()
      const journalNames = await readdir(journalRoot)
      expect(journalNames.length).toBeGreaterThan(0)
      for (const name of journalNames) {
        expect(await readFile(join(journalRoot, name), 'utf8')).not.toContain(FULL_ACCOUNT)
      }
      await expect(journal.close()).rejects.toThrow('controlled_artifact_export_journal_v2_unavailable')

      const recovered = await ControlledArtifactExportJournalV2.open(journalRoot)
      expect((await readdir(root)).filter((name) => name.startsWith('.stage-v2-'))).toEqual([])
      expect(await readdir(journalRoot)).toEqual([])
      await expect(readFile(target)).rejects.toMatchObject({ code: 'ENOENT' })
      await recovered.close()
      input.body.fill(0)
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('recovery preserves a target linked before a crash and removes only the stage alias', async () => {
    const root = await canonicalTempRoot('analytix-controlled-export-v2-recovery-linked-')
    const journalRoot = join(root, 'journal')
    try {
      const journal = await ControlledArtifactExportJournalV2.open(journalRoot)
      const target = join(root, 'linked-before-crash.json')
      const selected = await freezeControlledArtifactExportTargetV2(target, journal)
      const input = effectInput(selected.targetIdentityDigest())
      await selected.createPreparation()(input, new AbortController().signal)
      const stageName = (await readdir(root)).find((name) => name.startsWith('.stage-v2-'))
      if (!stageName) throw new Error('expected a staged artifact')
      await link(join(root, stageName), target)
      await expect(journal.close()).rejects.toThrow('controlled_artifact_export_journal_v2_unavailable')

      const recovered = await ControlledArtifactExportJournalV2.open(journalRoot)
      expect((await readdir(root)).filter((name) => name.startsWith('.stage-v2-'))).toEqual([])
      const targetBody = await readFile(target)
      expect(targetBody.equals(input.body)).toBe(true)
      targetBody.fill(0)
      expect(await readdir(journalRoot)).toEqual([])
      await recovered.close()
      input.body.fill(0)
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })

  it('plans all recovery entries before mutation and leaves staging untouched on unknown journal content', async () => {
    const root = await canonicalTempRoot('analytix-controlled-export-v2-recovery-corrupt-')
    const journalRoot = join(root, 'journal')
    try {
      const journal = await ControlledArtifactExportJournalV2.open(journalRoot)
      const target = join(root, 'quarantined.json')
      const selected = await freezeControlledArtifactExportTargetV2(target, journal)
      const input = effectInput(selected.targetIdentityDigest())
      await selected.createPreparation()(input, new AbortController().signal)
      const stageName = (await readdir(root)).find((name) => name.startsWith('.stage-v2-'))
      if (!stageName) throw new Error('expected a staged artifact')
      await expect(journal.close()).rejects.toThrow('controlled_artifact_export_journal_v2_unavailable')
      await writeFile(join(journalRoot, 'unknown-entry'), 'not authority', { mode: 0o600 })

      await expect(ControlledArtifactExportJournalV2.open(journalRoot)).rejects.toThrow(
        'controlled_artifact_export_journal_v2_unavailable'
      )
      const stagedBody = await readFile(join(root, stageName))
      expect(stagedBody.equals(input.body)).toBe(true)
      stagedBody.fill(0)
      input.body.fill(0)
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })
})

function effectInput(releaseTargetIdentityDigest: string): ControlledArtifactReleaseEffectInputV2 {
  const body = Buffer.from(JSON.stringify({ bankAccount: FULL_ACCOUNT }), 'utf8')
  return {
    action: 'export',
    body,
    releaseTargetIdentityDigest,
    receipt: {
      accessAction: 'export',
      targetIdentityDigest: '6'.repeat(64),
      artifactByteLength: body.length,
      artifactSha256: createHash('sha256').update(body).digest('hex'),
      mediaType: 'application/vnd.analytix.controlled-case-evidence+json',
      authorizedUntil: DEADLINE
    }
  } as ControlledArtifactReleaseEffectInputV2
}

async function canonicalTempRoot(prefix: string): Promise<string> {
  return realpath(await mkdtemp(join(tmpdir(), prefix)))
}
