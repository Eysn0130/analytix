import { mkdtemp, readFile, rm, stat } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { AutoResearchProjectStore } from '../src/research/autoresearch-store.js'

describe('AutoResearchProjectStore', () => {
  let workspace: string

  beforeEach(async () => {
    workspace = await mkdtemp(join(tmpdir(), 'analytix-autoresearch-'))
  })

  afterEach(async () => {
    await rm(workspace, { recursive: true, force: true })
  })

  it('creates analytix-owned project-local research state files', async () => {
    const store = new AutoResearchProjectStore({ nowIso: () => '2026-06-21T00:00:00.000Z' })

    const state = await store.createOrResume({
      workspace,
      threadId: 'thr_research',
      objective: 'Map provider cache behavior',
      requirements: ['Compare providers', 'Record unsupported fallback']
    })

    expect(state.descriptor.stateRelativePath).toBe('.analytix/autoresearch/thr_research')
    await expect(stat(join(workspace, state.descriptor.taskSpecPath))).resolves.toBeTruthy()
    await expect(stat(join(workspace, state.descriptor.progressPath))).resolves.toBeTruthy()
    await expect(stat(join(workspace, state.descriptor.findingsPath))).resolves.toBeTruthy()
    await expect(stat(join(workspace, state.descriptor.directionsTriedPath))).resolves.toBeTruthy()
    await expect(stat(join(workspace, state.descriptor.iterationLogPath))).resolves.toBeTruthy()
    await expect(stat(join(workspace, 'REASONIX.md'))).rejects.toThrow()
    await expect(stat(join(workspace, 'AGENTS.md'))).rejects.toThrow()

    const taskSpec = await readFile(join(workspace, state.descriptor.taskSpecPath), 'utf8')
    expect(taskSpec).toContain('Map provider cache behavior')
    expect(taskSpec).toContain('req_1')
    const progress = JSON.parse(await readFile(join(workspace, state.descriptor.progressPath), 'utf8'))
    expect(progress).toMatchObject({
      schemaVersion: 1,
      threadId: 'thr_research',
      status: 'active'
    })
    expect(progress.requirements).toHaveLength(2)
  })

  it('resumes state across store instances and audits requirements by evidence', async () => {
    const now = () => '2026-06-21T00:00:00.000Z'
    const first = new AutoResearchProjectStore({ nowIso: now })
    await first.createOrResume({
      workspace,
      threadId: 'thr_research',
      objective: 'Research long task recovery',
      requirements: ['Find restart path', 'Prove evidence audit']
    })
    await first.recordDirection({
      workspace,
      threadId: 'thr_research',
      direction: 'Inspect runtime stores',
      outcome: 'promising',
      summary: 'Found project-local state boundary.'
    })
    await first.recordEvidence({
      workspace,
      threadId: 'thr_research',
      requirementId: 'req_1',
      step: 'Restart path documented',
      evidence: ['progress.json reloaded after store recreation']
    })

    const second = new AutoResearchProjectStore({ nowIso: now })
    const snapshot = await second.createOrResume({
      workspace,
      threadId: 'thr_research',
      objective: 'Research long task recovery',
      requirements: ['Find restart path', 'Prove evidence audit']
    })
    const audit = await second.auditRequirements({ workspace, threadId: 'thr_research' })

    expect(snapshot.progress.requirements).toMatchObject([
      { id: 'req_1', status: 'completed', evidenceCount: 1 },
      { id: 'req_2', status: 'pending', evidenceCount: 0 }
    ])
    expect(audit.complete).toBe(false)
    expect(audit.requirements).toMatchObject([
      { id: 'req_1', hasEvidence: true },
      { id: 'req_2', hasEvidence: false }
    ])
    const directions = JSON.parse(await readFile(join(workspace, snapshot.descriptor.directionsTriedPath), 'utf8'))
    expect(directions.directions).toMatchObject([
      { direction: 'Inspect runtime stores', outcome: 'promising' }
    ])
    const findings = await readFile(join(workspace, snapshot.descriptor.findingsPath), 'utf8')
    expect(findings).toContain('Restart path documented')
    const iterationLog = await readFile(join(workspace, snapshot.descriptor.iterationLogPath), 'utf8')
    expect(iterationLog).toContain('direction_recorded')
    expect(iterationLog).toContain('evidence_recorded')
  })

  it('rejects evidence for unknown requirements without writing findings', async () => {
    const store = new AutoResearchProjectStore({ nowIso: () => '2026-06-21T00:00:00.000Z' })
    const state = await store.createOrResume({
      workspace,
      threadId: 'thr_research',
      objective: 'Research long task recovery',
      requirements: ['Find restart path']
    })

    await expect(store.recordEvidence({
      workspace,
      threadId: 'thr_research',
      requirementId: 'req_missing',
      step: 'Wrong requirement',
      evidence: ['this should not be accepted']
    })).rejects.toThrow(/unknown research requirement/)

    const audit = await store.auditRequirements({ workspace, threadId: 'thr_research' })
    expect(audit).toMatchObject({
      complete: false,
      requirements: [{ id: 'req_1', hasEvidence: false, evidenceCount: 0 }]
    })
    await expect(readFile(join(workspace, state.descriptor.findingsPath), 'utf8'))
      .resolves.toBe('')
  })
})
