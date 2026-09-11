import { mkdir, readFile, writeFile, appendFile } from 'node:fs/promises'
import { isAbsolute, join, relative, resolve } from 'node:path'

export type AutoResearchRequirementStatus = 'pending' | 'completed'

export type AutoResearchRequirement = {
  id: string
  text: string
  status: AutoResearchRequirementStatus
  evidenceCount: number
  updatedAt: string
}

export type AutoResearchProgress = {
  schemaVersion: 1
  threadId: string
  objective: string
  status: 'active' | 'complete'
  requirements: AutoResearchRequirement[]
  createdAt: string
  updatedAt: string
}

export type AutoResearchDescriptor = {
  enabled: true
  stateRelativePath: string
  taskSpecPath: string
  progressPath: string
  findingsPath: string
  directionsTriedPath: string
  iterationLogPath: string
  requirementCount: number
}

export type AutoResearchSnapshot = {
  descriptor: AutoResearchDescriptor
  progress: AutoResearchProgress
}

export type AutoResearchRequirementAudit = {
  complete: boolean
  requirements: Array<{
    id: string
    text: string
    status: AutoResearchRequirementStatus
    hasEvidence: boolean
    evidenceCount: number
  }>
}

export type AutoResearchProjectStoreOptions = {
  nowIso: () => string
}

export type AutoResearchDirectionRecord = {
  id: string
  direction: string
  outcome: 'tried' | 'promising' | 'dead_end'
  summary?: string
  createdAt: string
}

type AutoResearchPaths = {
  stateRelativePath: string
  absoluteStateDir: string
  taskSpecRelativePath: string
  progressRelativePath: string
  findingsRelativePath: string
  directionsRelativePath: string
  iterationLogRelativePath: string
  taskSpecAbsolutePath: string
  progressAbsolutePath: string
  findingsAbsolutePath: string
  directionsAbsolutePath: string
  iterationLogAbsolutePath: string
}

export class AutoResearchProjectStore {
  private readonly nowIso: () => string

  constructor(options: AutoResearchProjectStoreOptions) {
    this.nowIso = options.nowIso
  }

  async createOrResume(input: {
    workspace: string
    threadId: string
    objective: string
    requirements?: readonly string[]
  }): Promise<AutoResearchSnapshot> {
    const paths = this.paths(input.workspace, input.threadId)
    await mkdir(paths.absoluteStateDir, { recursive: true })
    let progress = await this.readProgress(paths.progressAbsolutePath)
    if (!progress) {
      const now = this.nowIso()
      const requirements = normalizeRequirements(input.requirements, input.objective)
        .map((text, index) => ({
          id: `req_${index + 1}`,
          text,
          status: 'pending' as const,
          evidenceCount: 0,
          updatedAt: now
        }))
      progress = {
        schemaVersion: 1,
        threadId: input.threadId,
        objective: input.objective,
        status: 'active',
        requirements,
        createdAt: now,
        updatedAt: now
      }
      await writeFile(paths.progressAbsolutePath, `${JSON.stringify(progress, null, 2)}\n`, 'utf8')
      await writeFile(paths.taskSpecAbsolutePath, renderTaskSpec(progress), 'utf8')
      await writeFile(paths.findingsAbsolutePath, '', { flag: 'wx' }).catch(() => undefined)
      await writeFile(paths.iterationLogAbsolutePath, '', { flag: 'wx' }).catch(() => undefined)
      await writeFile(
        paths.directionsAbsolutePath,
        `${JSON.stringify({ schemaVersion: 1, threadId: input.threadId, directions: [] }, null, 2)}\n`,
        { flag: 'wx' }
      ).catch(() => undefined)
    } else {
      await ensureFile(paths.taskSpecAbsolutePath, renderTaskSpec(progress))
      await ensureFile(paths.findingsAbsolutePath, '')
      await ensureFile(paths.iterationLogAbsolutePath, '')
      await ensureFile(paths.directionsAbsolutePath, `${JSON.stringify({
        schemaVersion: 1,
        threadId: input.threadId,
        directions: []
      }, null, 2)}\n`)
    }
    return {
      descriptor: descriptorFromPaths(paths, progress.requirements.length),
      progress
    }
  }

  async recordDirection(input: {
    workspace: string
    threadId: string
    direction: string
    outcome: AutoResearchDirectionRecord['outcome']
    summary?: string
  }): Promise<AutoResearchDirectionRecord> {
    const paths = this.paths(input.workspace, input.threadId)
    const direction = input.direction.trim()
    if (!direction) throw new Error('research direction is required')
    const record: AutoResearchDirectionRecord = {
      id: `dir_${Date.now().toString(36)}`,
      direction,
      outcome: input.outcome,
      ...(input.summary?.trim() ? { summary: input.summary.trim() } : {}),
      createdAt: this.nowIso()
    }
    const raw = await readJsonFile<{ schemaVersion: 1; threadId: string; directions: AutoResearchDirectionRecord[] }>(
      paths.directionsAbsolutePath
    ) ?? { schemaVersion: 1, threadId: input.threadId, directions: [] }
    raw.directions.push(record)
    await writeFile(paths.directionsAbsolutePath, `${JSON.stringify(raw, null, 2)}\n`, 'utf8')
    await appendJsonl(paths.iterationLogAbsolutePath, {
      kind: 'direction_recorded',
      directionId: record.id,
      direction: record.direction,
      outcome: record.outcome,
      createdAt: record.createdAt
    })
    return record
  }

  async recordEvidence(input: {
    workspace: string
    threadId: string
    requirementId?: string
    step: string
    evidence: readonly string[]
    summary?: string
  }): Promise<AutoResearchSnapshot> {
    const paths = this.paths(input.workspace, input.threadId)
    const progress = await this.requireProgress(paths.progressAbsolutePath)
    const now = this.nowIso()
    const requirementId = input.requirementId?.trim()
    const evidence = input.evidence.map((item) => item.trim()).filter(Boolean)
    if (evidence.length === 0) throw new Error('research evidence is required')
    if (requirementId) {
      if (!progress.requirements.some((requirement) => requirement.id === requirementId)) {
        throw new Error(`unknown research requirement: ${requirementId}`)
      }
      progress.requirements = progress.requirements.map((requirement) =>
        requirement.id === requirementId
          ? {
              ...requirement,
              status: 'completed',
              evidenceCount: requirement.evidenceCount + evidence.length,
              updatedAt: now
            }
          : requirement
      )
    }
    progress.status = progress.requirements.every((requirement) => requirement.status === 'completed')
      ? 'complete'
      : 'active'
    progress.updatedAt = now
    await writeFile(paths.progressAbsolutePath, `${JSON.stringify(progress, null, 2)}\n`, 'utf8')
    await appendJsonl(paths.findingsAbsolutePath, {
      kind: 'finding',
      requirementId,
      step: input.step.trim(),
      evidence,
      ...(input.summary?.trim() ? { summary: input.summary.trim() } : {}),
      createdAt: now
    })
    await appendJsonl(paths.iterationLogAbsolutePath, {
      kind: 'evidence_recorded',
      requirementId,
      evidenceCount: evidence.length,
      createdAt: now
    })
    return {
      descriptor: descriptorFromPaths(paths, progress.requirements.length),
      progress
    }
  }

  async auditRequirements(input: {
    workspace: string
    threadId: string
  }): Promise<AutoResearchRequirementAudit> {
    const paths = this.paths(input.workspace, input.threadId)
    const progress = await this.requireProgress(paths.progressAbsolutePath)
    const requirements = progress.requirements.map((requirement) => ({
      id: requirement.id,
      text: requirement.text,
      status: requirement.status,
      hasEvidence: requirement.evidenceCount > 0,
      evidenceCount: requirement.evidenceCount
    }))
    return {
      complete: requirements.length > 0 && requirements.every((requirement) =>
        requirement.status === 'completed' && requirement.hasEvidence
      ),
      requirements
    }
  }

  private paths(workspace: string, threadId: string): AutoResearchPaths {
    if (!isAbsolute(workspace)) throw new Error(`workspace must be absolute: ${workspace}`)
    const safeThreadId = sanitizePathSegment(threadId)
    const stateRelativePath = `.analytix/autoresearch/${safeThreadId}`
    const absoluteStateDir = resolve(workspace, stateRelativePath)
    const workspaceRoot = resolve(workspace)
    const relativeStateDir = relative(workspaceRoot, absoluteStateDir)
    if (relativeStateDir.startsWith('..') || isAbsolute(relativeStateDir)) {
      throw new Error('autoresearch state path escaped workspace')
    }
    return {
      stateRelativePath,
      absoluteStateDir,
      taskSpecRelativePath: `${stateRelativePath}/task_spec.md`,
      progressRelativePath: `${stateRelativePath}/progress.json`,
      findingsRelativePath: `${stateRelativePath}/findings.jsonl`,
      directionsRelativePath: `${stateRelativePath}/directions_tried.json`,
      iterationLogRelativePath: `${stateRelativePath}/iteration_log.jsonl`,
      taskSpecAbsolutePath: join(absoluteStateDir, 'task_spec.md'),
      progressAbsolutePath: join(absoluteStateDir, 'progress.json'),
      findingsAbsolutePath: join(absoluteStateDir, 'findings.jsonl'),
      directionsAbsolutePath: join(absoluteStateDir, 'directions_tried.json'),
      iterationLogAbsolutePath: join(absoluteStateDir, 'iteration_log.jsonl')
    }
  }

  private async readProgress(path: string): Promise<AutoResearchProgress | null> {
    return readJsonFile<AutoResearchProgress>(path)
  }

  private async requireProgress(path: string): Promise<AutoResearchProgress> {
    const progress = await this.readProgress(path)
    if (!progress) throw new Error('autoresearch progress does not exist')
    return progress
  }
}

function descriptorFromPaths(
  paths: AutoResearchPaths,
  requirementCount: number
): AutoResearchDescriptor {
  return {
    enabled: true,
    stateRelativePath: paths.stateRelativePath,
    taskSpecPath: paths.taskSpecRelativePath,
    progressPath: paths.progressRelativePath,
    findingsPath: paths.findingsRelativePath,
    directionsTriedPath: paths.directionsRelativePath,
    iterationLogPath: paths.iterationLogRelativePath,
    requirementCount
  }
}

function normalizeRequirements(requirements: readonly string[] | undefined, objective: string): string[] {
  const normalized = requirements?.map((item) => item.trim()).filter(Boolean) ?? []
  return normalized.length > 0 ? normalized : [objective.trim()]
}

function renderTaskSpec(progress: AutoResearchProgress): string {
  return [
    '# AutoResearch Task Spec',
    '',
    `Thread: ${progress.threadId}`,
    `Status: ${progress.status}`,
    '',
    '## Objective',
    '',
    progress.objective,
    '',
    '## Requirements',
    '',
    ...progress.requirements.map((requirement) =>
      `- [ ] ${requirement.id}: ${requirement.text}`
    ),
    ''
  ].join('\n')
}

function sanitizePathSegment(value: string): string {
  const sanitized = value.replace(/[^A-Za-z0-9_.-]/g, '_').replace(/^\\.+/, '_')
  return sanitized || 'thread'
}

async function readJsonFile<T>(path: string): Promise<T | null> {
  try {
    return JSON.parse(await readFile(path, 'utf8')) as T
  } catch (error) {
    if (error && typeof error === 'object' && 'code' in error && error.code === 'ENOENT') {
      return null
    }
    throw error
  }
}

async function ensureFile(path: string, content: string): Promise<void> {
  try {
    await writeFile(path, content, { flag: 'wx' })
  } catch (error) {
    if (error && typeof error === 'object' && 'code' in error && error.code === 'EEXIST') return
    throw error
  }
}

async function appendJsonl(path: string, value: Record<string, unknown>): Promise<void> {
  await appendFile(path, `${JSON.stringify(value)}\n`, 'utf8')
}
