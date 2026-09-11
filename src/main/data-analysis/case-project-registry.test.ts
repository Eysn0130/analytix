import {
  chmod,
  lstat,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  realpath,
  rm,
  symlink,
  writeFile
} from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import {
  DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER,
  type DataAnalysisBackendManager
} from './backend-manager'
import { ensureDataAnalysisWorkspaceCase } from './case-project-registry'

const INVALID_MESSAGE = '案件项目绑定文件无效，已保留原文件；请修复或通过受信任迁移流程处理。'
const UNREADABLE_MESSAGE = '案件项目绑定文件不可读取，已保留原文件；请检查文件权限后重试。'
const UNRESOLVED_MESSAGE = '案件项目绑定指向的案件当前不可用，已保留原绑定且未创建替代案件。'
const WRITE_MESSAGE = '案件项目绑定写入或校验失败，案件未激活；请检查工作区后重试。'

const roots: string[] = []

type RequestRecord = { path: string; method: string }

class BackendStub {
  ensureCalls = 0
  readonly requests: RequestRecord[] = []
  caseExists = true
  createdCaseId = 'case_created_1234'
  authorityAvailable = true
  onCreate: (() => Promise<void>) | undefined

  async ensureBackend(): Promise<Record<string, unknown>> {
    this.ensureCalls += 1
    return this.authorityAvailable
      ? { phase: 'running', authority: 'go-native', terminal: false, apiBase: 'http://127.0.0.1:1' }
      : {
          phase: 'failed',
          authority: 'unavailable',
          terminal: true,
          blocker: DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER
        }
  }

  async request(path: string, init: RequestInit = {}): Promise<Response> {
    const method = String(init.method ?? 'GET').toUpperCase()
    this.requests.push({ path, method })
    if (path === '/api/v1/cases' && method === 'POST') {
      await this.onCreate?.()
      return jsonResponse({ data: { case_id: this.createdCaseId } })
    }
    if (path.endsWith('/activate') && method === 'POST') {
      return jsonResponse({ data: { active: true } })
    }
    if (method === 'GET') {
      return this.caseExists
        ? jsonResponse({ data: { case_id: decodeURIComponent(path.split('/').at(-1) ?? '') } })
        : jsonResponse({ error: { message: 'not found' } }, 404)
    }
    return jsonResponse({ error: { message: 'unexpected request' } }, 500)
  }

  manager(): DataAnalysisBackendManager {
    return this as unknown as DataAnalysisBackendManager
  }
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' }
  })
}

async function temporaryWorkspace(): Promise<string> {
  const root = await mkdtemp(join(tmpdir(), 'analytix-case-registry-'))
  roots.push(root)
  return realpath(root)
}

function bindingPath(workspace: string): string {
  return join(workspace, '.analytix', 'case-project.json')
}

function validBinding(workspace: string, caseId = 'case_existing_1234'): Record<string, unknown> {
  return {
    version: 1,
    workspaceRoot: workspace,
    caseId,
    source: 'analytix-data-analysis',
    updatedAt: '2026-07-12T00:00:00Z'
  }
}

async function seedBinding(workspace: string, body: string): Promise<void> {
  await mkdir(join(workspace, '.analytix'), { mode: 0o700 })
  await writeFile(bindingPath(workspace), body, { encoding: 'utf8', mode: 0o600 })
}

afterEach(async () => {
  await Promise.all(roots.splice(0).map((root) => rm(root, { recursive: true, force: true })))
})

describe('case project registry binding authority', () => {
  it('creates a missing binding atomically before activating the new case', async () => {
    const workspace = await temporaryWorkspace()
    const backend = new BackendStub()

    const result = await ensureDataAnalysisWorkspaceCase(backend.manager(), workspace)

    expect(result).toEqual({ ok: true, caseId: backend.createdCaseId, workspaceRoot: workspace })
    expect(backend.ensureCalls).toBe(1)
    expect(backend.requests).toEqual([
      { path: '/api/v1/cases', method: 'POST' },
      { path: `/api/v1/cases/${backend.createdCaseId}/activate`, method: 'POST' }
    ])
    const raw = await readFile(bindingPath(workspace), 'utf8')
    const parsed = JSON.parse(raw) as Record<string, unknown>
    expect(parsed).toMatchObject({
      version: 1,
      workspaceRoot: workspace,
      caseId: backend.createdCaseId,
      source: 'analytix-data-analysis'
    })
    expect(parsed.updatedAt).toEqual(expect.any(String))
    expect((await lstat(bindingPath(workspace))).nlink).toBe(1)
    expect(await readdir(join(workspace, '.analytix'))).toEqual(['case-project.json'])
  })

  it('reuses a strictly valid binding without rewriting or creating a case', async () => {
    const workspace = await temporaryWorkspace()
    const raw = `${JSON.stringify(validBinding(workspace), null, 2)}\n`
    await seedBinding(workspace, raw)
    const backend = new BackendStub()

    const result = await ensureDataAnalysisWorkspaceCase(backend.manager(), workspace)

    expect(result).toEqual({ ok: true, caseId: 'case_existing_1234', workspaceRoot: workspace })
    expect(backend.requests).toEqual([
      { path: '/api/v1/cases/case_existing_1234', method: 'GET' },
      { path: '/api/v1/cases/case_existing_1234/activate', method: 'POST' }
    ])
    expect(await readFile(bindingPath(workspace), 'utf8')).toBe(raw)
  })

  it('preserves a valid binding when its backend case is unavailable', async () => {
    const workspace = await temporaryWorkspace()
    const raw = `${JSON.stringify(validBinding(workspace))}\n`
    await seedBinding(workspace, raw)
    const backend = new BackendStub()
    backend.caseExists = false

    const result = await ensureDataAnalysisWorkspaceCase(backend.manager(), workspace)

    expect(result).toEqual({ ok: false, message: UNRESOLVED_MESSAGE })
    expect(backend.requests).toEqual([{ path: '/api/v1/cases/case_existing_1234', method: 'GET' }])
    expect(await readFile(bindingPath(workspace), 'utf8')).toBe(raw)
  })

  it.each([
    ['corrupt JSON', '{'],
    ['unknown field', (workspace: string) => JSON.stringify({ ...validBinding(workspace), safeToAnswer: true })],
    ['trailing JSON', (workspace: string) => `${JSON.stringify(validBinding(workspace))}\n{}`],
    ['wrong version', (workspace: string) => JSON.stringify({ ...validBinding(workspace), version: 2 })],
    ['wrong source', (workspace: string) => JSON.stringify({ ...validBinding(workspace), source: 'caller-reported' })],
    ['wrong workspace', (workspace: string) => JSON.stringify({ ...validBinding(workspace), workspaceRoot: join(workspace, 'other') })]
  ])('keeps %s binding bytes and performs zero backend actions', async (_name, source) => {
    const workspace = await temporaryWorkspace()
    const raw = typeof source === 'function' ? source(workspace) : source
    await seedBinding(workspace, raw)
    const backend = new BackendStub()

    const result = await ensureDataAnalysisWorkspaceCase(backend.manager(), workspace)

    expect(result).toEqual({ ok: false, message: INVALID_MESSAGE })
    expect(backend.ensureCalls).toBe(1)
    expect(backend.requests).toEqual([])
    expect(await readFile(bindingPath(workspace), 'utf8')).toBe(raw)
  })

  it('rejects a symlink binding target without touching it or the backend', async () => {
    const workspace = await temporaryWorkspace()
    const external = join(workspace, 'external-binding.json')
    const raw = JSON.stringify(validBinding(workspace))
    await writeFile(external, raw, 'utf8')
    await mkdir(join(workspace, '.analytix'), { mode: 0o700 })
    await symlink(external, bindingPath(workspace))
    const backend = new BackendStub()

    const result = await ensureDataAnalysisWorkspaceCase(backend.manager(), workspace)

    expect(result).toEqual({ ok: false, message: INVALID_MESSAGE })
    expect(backend.ensureCalls).toBe(1)
    expect(backend.requests).toEqual([])
    expect((await lstat(bindingPath(workspace))).isSymbolicLink()).toBe(true)
    expect(await readFile(external, 'utf8')).toBe(raw)
  })

  it('rejects a symlink metadata parent without creating or activating a case', async () => {
    const workspace = await temporaryWorkspace()
    const external = join(workspace, 'external-metadata')
    await mkdir(external)
    await writeFile(join(external, 'case-project.json'), JSON.stringify(validBinding(workspace)), 'utf8')
    await symlink(external, join(workspace, '.analytix'))
    const backend = new BackendStub()

    const result = await ensureDataAnalysisWorkspaceCase(backend.manager(), workspace)

    expect(result).toEqual({ ok: false, message: INVALID_MESSAGE })
    expect(backend.ensureCalls).toBe(1)
    expect(backend.requests).toEqual([])
    expect((await lstat(join(workspace, '.analytix'))).isSymbolicLink()).toBe(true)
  })

  it('does not overwrite a binding that appears after backend case creation', async () => {
    const workspace = await temporaryWorkspace()
    const backend = new BackendStub()
    const sentinel = '{"preserve":"concurrent-owner"}\n'
    backend.onCreate = async () => seedBinding(workspace, sentinel)

    const result = await ensureDataAnalysisWorkspaceCase(backend.manager(), workspace)

    expect(result).toEqual({ ok: false, message: WRITE_MESSAGE })
    expect(backend.requests).toEqual([{ path: '/api/v1/cases', method: 'POST' }])
    expect(await readFile(bindingPath(workspace), 'utf8')).toBe(sentinel)
  })

  it.skipIf(process.platform === 'win32' || typeof process.getuid !== 'function' || process.getuid() === 0)(
    'classifies an unreadable binding separately and leaves it untouched',
    async () => {
      const workspace = await temporaryWorkspace()
      const raw = JSON.stringify(validBinding(workspace))
      await seedBinding(workspace, raw)
      await chmod(bindingPath(workspace), 0o000)
      const backend = new BackendStub()
      try {
        const result = await ensureDataAnalysisWorkspaceCase(backend.manager(), workspace)
        expect(result).toEqual({ ok: false, message: UNREADABLE_MESSAGE })
        expect(backend.ensureCalls).toBe(1)
        expect(backend.requests).toEqual([])
      } finally {
        await chmod(bindingPath(workspace), 0o600)
      }
      expect(await readFile(bindingPath(workspace), 'utf8')).toBe(raw)
    }
  )

  it('returns the native authority boundary before inspecting an untrusted workspace path', async () => {
    const backend = new BackendStub()
    backend.authorityAvailable = false

    const result = await ensureDataAnalysisWorkspaceCase(
      backend.manager(),
      '/private/case/path-that-must-not-be-read'
    )

    expect(result).toEqual({ ok: false, message: DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER })
    expect(backend.ensureCalls).toBe(1)
    expect(backend.requests).toEqual([])
  })
})
