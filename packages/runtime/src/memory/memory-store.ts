import { mkdir, readFile, readdir } from 'node:fs/promises'
import { join } from 'node:path'
import type { MemoryCapabilityConfig } from '../contracts/capabilities.js'
import { atomicWriteFile } from '../adapters/file/atomic-write.js'
import {
  ActiveMemoryRecord,
  MemoryCreateRequest,
  MemoryDiagnostics,
  MemoryRecord,
  MemoryTombstone,
  MemoryUpdateRequest
} from '../contracts/memory.js'

export interface MemoryStore {
  create(input: MemoryCreateRequest): Promise<ActiveMemoryRecord>
  update(id: string, patch: MemoryUpdateRequest): Promise<ActiveMemoryRecord>
  delete(id: string): Promise<MemoryTombstone>
  list(filter?: { workspace?: string; includeDeleted?: boolean }): Promise<MemoryRecord[]>
  retrieve(input: { query: string; workspace?: string; limit: number }): Promise<ActiveMemoryRecord[]>
  diagnostics(): Promise<MemoryDiagnostics>
  setLastInjected(ids: string[]): void
}

export class FileMemoryStore implements MemoryStore {
  constructor(
    private readonly options: {
      rootDir: string
      config: MemoryCapabilityConfig
      nowIso?: () => string
      idGenerator?: () => string
    }
  ) {}

  async create(input: MemoryCreateRequest): Promise<ActiveMemoryRecord> {
    const admitted = MemoryCreateRequest.parse(input)
    await mkdir(this.options.rootDir, { recursive: true })
    const now = this.now()
    const parsed = ActiveMemoryRecord.parse({
      id: this.options.idGenerator?.() ?? `mem_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
      content: admitted.content,
      scope: admitted.scope,
      workspace: admitted.workspace,
      project: admitted.project,
      provenance: 'manual-general',
      captureMode: 'manual',
      modelInjection: false,
      tags: admitted.tags,
      confidence: admitted.confidence,
      createdAt: now,
      updatedAt: now
    })
    await this.write(parsed)
    return parsed
  }

  async update(id: string, patch: MemoryUpdateRequest): Promise<ActiveMemoryRecord> {
    const admitted = MemoryUpdateRequest.parse(patch)
    const current = await this.mustGet(id)
    const now = this.now()
    const next = ActiveMemoryRecord.parse({
      ...current,
      ...(admitted.content !== undefined ? { content: admitted.content } : {}),
      ...(admitted.tags !== undefined ? { tags: admitted.tags } : {}),
      ...(admitted.confidence !== undefined ? { confidence: admitted.confidence } : {}),
      ...(admitted.disabled === true ? { disabledAt: current.disabledAt ?? now } : {}),
      ...(admitted.disabled === false ? { disabledAt: undefined } : {}),
      updatedAt: now
    })
    await this.write(next)
    return next
  }

  async delete(id: string): Promise<MemoryTombstone> {
    const current = await this.mustGet(id)
    const now = this.now()
    const next = MemoryTombstone.parse({
      id: current.id,
      createdAt: current.createdAt,
      deletedAt: now,
      updatedAt: now
    })
    await this.write(next)
    return next
  }

  async list(filter: { workspace?: string; includeDeleted?: boolean } = {}): Promise<MemoryRecord[]> {
    const records = await this.readAll()
    return records
      .filter((record) => filter.includeDeleted || !isMemoryTombstone(record))
      .filter((record) => inScope(record, filter.workspace))
      .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
  }

  async retrieve(input: { query: string; workspace?: string; limit: number }): Promise<ActiveMemoryRecord[]> {
    void input
    return []
  }

  async diagnostics(): Promise<MemoryDiagnostics> {
    const records = await this.readAll()
    return {
      enabled: this.options.config.enabled,
      rootDir: this.options.rootDir,
      activeCount: records.filter((record) => !isMemoryTombstone(record) && !record.disabledAt).length,
      tombstoneCount: records.filter(isMemoryTombstone).length,
      lastInjectedIds: [],
      admittedProvenance: 'manual-general',
      captureMode: 'manual',
      modelInjection: false
    }
  }

  setLastInjected(ids: string[]): void {
    void ids
  }

  private async mustGet(id: string): Promise<ActiveMemoryRecord> {
    const record = (await this.readAll()).find((candidate) => candidate.id === id)
    if (!record || isMemoryTombstone(record)) throw new Error(`memory not found: ${id}`)
    return record
  }

  private async readAll(): Promise<MemoryRecord[]> {
    await mkdir(this.options.rootDir, { recursive: true })
    const entries = await readdir(this.options.rootDir).catch(() => [])
    const records = await Promise.all(entries
      .filter((entry) => entry.endsWith('.json'))
      .map((entry) => readFile(join(this.options.rootDir, entry), 'utf8')
        .then((text) => MemoryRecord.parse(JSON.parse(text)))
        .catch(() => null)))
    return records.filter((record): record is MemoryRecord => Boolean(record))
  }

  private write(record: MemoryRecord): Promise<void> {
    return atomicWriteFile(
      join(this.options.rootDir, `${record.id}.json`),
      JSON.stringify(record, null, 2)
    )
  }

  private now(): string {
    return this.options.nowIso?.() ?? new Date().toISOString()
  }
}

function inScope(record: MemoryRecord, workspace: string | undefined): boolean {
  if (isMemoryTombstone(record)) return workspace === undefined
  if (record.scope === 'user') return true
  if (record.scope === 'workspace') {
    // Records created via the GUI may not carry a workspace (e.g. manually
    // added before any thread ran). Treat a missing workspace as in-scope so
    // they are still retrievable; otherwise require an exact match.
    if (!record.workspace) return true
    return Boolean(workspace && record.workspace === workspace)
  }
  return true
}

function isMemoryTombstone(record: MemoryRecord): record is MemoryTombstone {
  return 'deletedAt' in record
}
