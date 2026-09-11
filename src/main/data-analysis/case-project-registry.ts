import { createHash, randomUUID } from 'node:crypto'
import { constants } from 'node:fs'
import {
  link,
  lstat,
  mkdir,
  open,
  realpath,
  rm,
  stat
} from 'node:fs/promises'
import { basename, join, resolve } from 'node:path'
import { atomicWriteFile } from '../../../packages/runtime/src/adapters/file/atomic-write.js'
import type { DataAnalysisWorkspaceCaseResult } from '../../shared/data-analysis'
import {
  DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER,
  type DataAnalysisBackendManager
} from './backend-manager'

type CaseProjectBinding = {
  version: 1
  workspaceRoot: string
  caseId: string
  source: 'analytix-data-analysis'
  updatedAt: string
}

type BindingReadResult =
  | { status: 'missing' }
  | { status: 'valid'; binding: CaseProjectBinding; sha256: string }
  | { status: 'invalid' }
  | { status: 'unreadable' }

type Envelope<T> = {
  data?: T
  error?: {
    code?: string
    message?: string
  }
}

const BINDING_FILE_NAME = 'case-project.json'
const BINDING_DIRECTORY_NAME = '.analytix'
const MAX_BINDING_BYTES = 1024 * 1024
const CASE_ID_PATTERN = /^[A-Za-z0-9_-]{4,80}$/
const BINDING_KEYS = ['caseId', 'source', 'updatedAt', 'version', 'workspaceRoot']

const INVALID_BINDING_MESSAGE = '案件项目绑定文件无效，已保留原文件；请修复或通过受信任迁移流程处理。'
const UNREADABLE_BINDING_MESSAGE = '案件项目绑定文件不可读取，已保留原文件；请检查文件权限后重试。'
const UNRESOLVED_BINDING_MESSAGE = '案件项目绑定指向的案件当前不可用，已保留原绑定且未创建替代案件。'
const WRITE_BINDING_MESSAGE = '案件项目绑定写入或校验失败，案件未激活；请检查工作区后重试。'

class BindingWriteError extends Error {
  constructor() {
    super(WRITE_BINDING_MESSAGE)
  }
}

function nowIso(): string {
  return new Date().toISOString()
}

function bindingDirectory(workspaceRoot: string): string {
  return join(workspaceRoot, BINDING_DIRECTORY_NAME)
}

function bindingPath(workspaceRoot: string): string {
  return join(bindingDirectory(workspaceRoot), BINDING_FILE_NAME)
}

function extractCaseId(payload: unknown): string {
  if (!payload || typeof payload !== 'object') return ''
  const data = (payload as Envelope<Record<string, unknown>>).data
  if (!data || typeof data !== 'object') return ''
  return String(data.case_id ?? data.caseId ?? '').trim()
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function errorCode(error: unknown): string {
  return String((error as { code?: unknown } | null)?.code ?? '')
}

function sha256(value: string): string {
  return createHash('sha256').update(value, 'utf8').digest('hex')
}

function sameFileIdentity(left: Awaited<ReturnType<typeof lstat>>, right: Awaited<ReturnType<typeof lstat>>): boolean {
  return left.dev === right.dev && left.ino === right.ino && left.mode === right.mode &&
    left.nlink === right.nlink && left.size === right.size
}

function isStrictTimestamp(value: string): boolean {
  return /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value) &&
    Number.isFinite(Date.parse(value))
}

function parseStrictBinding(raw: string, workspaceRoot: string): CaseProjectBinding | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null
  const record = parsed as Record<string, unknown>
  const keys = Object.keys(record).sort()
  if (keys.length !== BINDING_KEYS.length || keys.some((key, index) => key !== BINDING_KEYS[index])) {
    return null
  }
  if (record.version !== 1 || record.source !== 'analytix-data-analysis' || record.workspaceRoot !== workspaceRoot) {
    return null
  }
  if (typeof record.caseId !== 'string' || !CASE_ID_PATTERN.test(record.caseId)) return null
  if (typeof record.updatedAt !== 'string' || !isStrictTimestamp(record.updatedAt)) return null
  return {
    version: 1,
    workspaceRoot,
    caseId: record.caseId,
    source: 'analytix-data-analysis',
    updatedAt: record.updatedAt
  }
}

async function inspectBindingDirectory(workspaceRoot: string): Promise<'missing' | 'valid' | 'invalid' | 'unreadable'> {
  const directory = bindingDirectory(workspaceRoot)
  let info: Awaited<ReturnType<typeof lstat>>
  try {
    info = await lstat(directory)
  } catch (error) {
    return errorCode(error) === 'ENOENT' ? 'missing' : 'unreadable'
  }
  if (info.isSymbolicLink() || !info.isDirectory()) return 'invalid'
  try {
    return await realpath(directory) === directory ? 'valid' : 'invalid'
  } catch {
    return 'unreadable'
  }
}

async function readStableRegularFile(path: string): Promise<{ raw: string; sha256: string } | null> {
  const initial = await lstat(path)
  if (initial.isSymbolicLink() || !initial.isFile() || initial.nlink !== 1 || initial.size <= 0 || initial.size > MAX_BINDING_BYTES) {
    return null
  }
  const noFollow = process.platform === 'win32' ? 0 : constants.O_NOFOLLOW
  const handle = await open(path, constants.O_RDONLY | noFollow)
  try {
    const opened = await handle.stat()
    if (!opened.isFile() || opened.nlink !== 1 || !sameFileIdentity(initial, opened)) return null
    const raw = await handle.readFile({ encoding: 'utf8' })
    const afterRead = await handle.stat()
    const current = await lstat(path)
    if (!sameFileIdentity(opened, afterRead) || !sameFileIdentity(afterRead, current) ||
      Buffer.byteLength(raw, 'utf8') !== current.size) {
      return null
    }
    return { raw, sha256: sha256(raw) }
  } finally {
    await handle.close()
  }
}

async function readBinding(workspaceRoot: string): Promise<BindingReadResult> {
  const directoryState = await inspectBindingDirectory(workspaceRoot)
  if (directoryState === 'missing') return { status: 'missing' }
  if (directoryState !== 'valid') return { status: directoryState }
  const target = bindingPath(workspaceRoot)
  let stable: Awaited<ReturnType<typeof readStableRegularFile>>
  try {
    stable = await readStableRegularFile(target)
  } catch (error) {
    if (errorCode(error) === 'ENOENT') return { status: 'missing' }
    if (errorCode(error) === 'ELOOP') return { status: 'invalid' }
    return { status: 'unreadable' }
  }
  if (!stable) return { status: 'invalid' }
  const binding = parseStrictBinding(stable.raw, workspaceRoot)
  return binding
    ? { status: 'valid', binding, sha256: stable.sha256 }
    : { status: 'invalid' }
}

async function ensureSafeBindingDirectory(workspaceRoot: string): Promise<void> {
  const directory = bindingDirectory(workspaceRoot)
  const state = await inspectBindingDirectory(workspaceRoot)
  if (state === 'missing') {
    try {
      await mkdir(directory, { mode: 0o700 })
    } catch (error) {
      if (errorCode(error) !== 'EEXIST') throw error
    }
  }
  if (await inspectBindingDirectory(workspaceRoot) !== 'valid') {
    throw new BindingWriteError()
  }
}

async function writeBinding(binding: CaseProjectBinding): Promise<void> {
  await ensureSafeBindingDirectory(binding.workspaceRoot)
  if ((await readBinding(binding.workspaceRoot)).status !== 'missing') {
    throw new BindingWriteError()
  }
  const serialized = `${JSON.stringify(binding, null, 2)}\n`
  const expectedHash = sha256(serialized)
  const directory = bindingDirectory(binding.workspaceRoot)
  const target = bindingPath(binding.workspaceRoot)
  const stage = join(directory, `.case-project.${process.pid}.${randomUUID()}.stage`)
  try {
    await atomicWriteFile(stage, serialized, { mode: 0o600 })
    const staged = await readStableRegularFile(stage)
    if (!staged || staged.sha256 !== expectedHash) throw new BindingWriteError()
    try {
      await link(stage, target)
    } catch {
      throw new BindingWriteError()
    }
    try {
      await rm(stage)
    } catch {
      throw new BindingWriteError()
    }
    const readback = await readBinding(binding.workspaceRoot)
    if (readback.status !== 'valid' || readback.sha256 !== expectedHash ||
      JSON.stringify(readback.binding) !== JSON.stringify(binding)) {
      throw new BindingWriteError()
    }
  } catch {
    throw new BindingWriteError()
  } finally {
    await rm(stage, { force: true }).catch(() => undefined)
  }
}

async function ensureWorkspaceDirectory(workspaceRoot: string): Promise<string> {
  const normalized = resolve(workspaceRoot)
  const info = await stat(normalized)
  if (!info.isDirectory()) {
    throw new Error(`workspaceRoot is not a directory: ${normalized}`)
  }
  return realpath(normalized)
}

async function getCase(manager: DataAnalysisBackendManager, caseId: string): Promise<boolean> {
  const response = await manager.request(`/api/v1/cases/${encodeURIComponent(caseId)}`, {
    method: 'GET'
  })
  return response.ok
}

async function activateCase(manager: DataAnalysisBackendManager, caseId: string): Promise<void> {
  const response = await manager.request(`/api/v1/cases/${encodeURIComponent(caseId)}/activate`, {
    method: 'POST'
  })
  if (!response.ok) {
    throw new Error(`activate case failed: HTTP ${response.status}`)
  }
}

async function createCase(manager: DataAnalysisBackendManager, workspaceRoot: string): Promise<string> {
  const workspaceName = basename(workspaceRoot) || '案件项目'
  const response = await manager.request('/api/v1/cases', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      case_name: workspaceName,
      case_number: workspaceName.slice(0, 64) || String(Date.now()),
      owner: '',
      note: `analytix project: ${workspaceRoot}`,
      case_type: '',
      tags: []
    })
  })
  const payload = await response.json().catch(() => null)
  if (!response.ok) {
    const message = payload && typeof payload === 'object'
      ? String((payload as Envelope<unknown>).error?.message || '')
      : ''
    throw new Error(message || `create case failed: HTTP ${response.status}`)
  }
  const caseId = extractCaseId(payload)
  if (!CASE_ID_PATTERN.test(caseId)) {
    throw new Error('create case response did not include a valid case_id')
  }
  return caseId
}

export async function ensureDataAnalysisWorkspaceCase(
  manager: DataAnalysisBackendManager,
  workspaceRoot: string
): Promise<DataAnalysisWorkspaceCaseResult> {
  try {
    const authority = await manager.ensureBackend()
    if (
      authority.terminal ||
      authority.authority !== 'go-native' ||
      authority.phase !== 'running'
    ) {
      return { ok: false, message: DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER }
    }
    const normalizedWorkspaceRoot = await ensureWorkspaceDirectory(workspaceRoot)
    const current = await readBinding(normalizedWorkspaceRoot)
    if (current.status === 'invalid') return { ok: false, message: INVALID_BINDING_MESSAGE }
    if (current.status === 'unreadable') return { ok: false, message: UNREADABLE_BINDING_MESSAGE }
    if (current.status === 'valid') {
      if (!await getCase(manager, current.binding.caseId)) {
        return { ok: false, message: UNRESOLVED_BINDING_MESSAGE }
      }
      await activateCase(manager, current.binding.caseId)
      return { ok: true, caseId: current.binding.caseId, workspaceRoot: normalizedWorkspaceRoot }
    }

    const caseId = await createCase(manager, normalizedWorkspaceRoot)
    await writeBinding({
      version: 1,
      workspaceRoot: normalizedWorkspaceRoot,
      caseId,
      source: 'analytix-data-analysis',
      updatedAt: nowIso()
    })
    await activateCase(manager, caseId)
    return { ok: true, caseId, workspaceRoot: normalizedWorkspaceRoot }
  } catch (error) {
    return { ok: false, message: errorMessage(error) }
  }
}
