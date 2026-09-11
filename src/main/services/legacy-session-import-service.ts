import { readdir, realpath, stat } from 'node:fs/promises'
import { homedir } from 'node:os'
import { join } from 'node:path'
import type {
  LegacySessionDetectResult,
  LegacySessionDetectedSource,
  LegacySessionImportSummary,
  LegacySessionSourceKind
} from '../../shared/analytix-api'

/**
 * 旧会话检测保持只读。导入在宿主具备 staging、私密 reasoning 清理、
 * 全量公共记录验证和原子发布前必须 fail closed，不能把原始 JSONL 先写进
 * 当前生产数据目录再依赖一次可取消的重启迁移。
 */

const THREAD_DIR_MARKERS = ['metadata.jsonl', 'thread.json', 'messages.jsonl'] as const

export type LegacySessionSourceCandidate = {
  id: string
  kind: LegacySessionSourceKind
  /** 旧版线程目录的绝对路径,如 ~/.analytix/analytix/threads。 */
  path: string
}

export type LegacySessionImportLogger = (message: string, detail?: unknown) => void

export const LEGACY_SESSION_IMPORT_BLOCKED_MESSAGE =
  'Legacy session import is disabled until the host can sanitize and validate every record before atomic publication.'

/**
 * 自动检测的旧数据来源。顺序即展示优先级:先列近期版本(analytix),
 * 再列 Kun 默认数据根和更早的 coreagent 时代数据。三者磁盘格式相同。
 */
export function defaultLegacySourceCandidates(homeDir: string): LegacySessionSourceCandidate[] {
  return [
    { id: 'analytix-analytix', kind: 'analytix', path: join(homeDir, '.analytix', 'analytix', 'threads') },
    { id: 'kun-data', kind: 'kun', path: join(homeDir, '.kun', 'data', 'threads') },
    {
      id: 'analytix-coreagent',
      kind: 'coreagent',
      path: join(homeDir, '.analytix', 'coreagent', 'threads')
    }
  ]
}

async function pathExists(target: string): Promise<boolean> {
  try {
    await stat(target)
    return true
  } catch {
    return false
  }
}

/** realpath 解析失败(路径不存在等)时回退到原路径,只用于同目录判定。 */
async function safeRealpath(target: string): Promise<string> {
  try {
    return await realpath(target)
  } catch {
    return target
  }
}

/**
 * 列出 parent 下“看起来像线程目录”的子目录名。判定:目录名以 thr_ 开头,
 * 或目录内含已知线程标志文件(兼容自定义命名 / 更早格式)。
 */
async function listThreadDirNames(parent: string): Promise<string[]> {
  const entries = await readdir(parent, { withFileTypes: true }).catch(() => null)
  if (!entries) return []
  const names: string[] = []
  for (const entry of entries) {
    if (!entry.isDirectory()) continue
    if (entry.name.startsWith('thr_')) {
      names.push(entry.name)
      continue
    }
    const dir = join(parent, entry.name)
    for (const marker of THREAD_DIR_MARKERS) {
      if (await pathExists(join(dir, marker))) {
        names.push(entry.name)
        break
      }
    }
  }
  return names
}

/** 检测可导入的旧会话来源,以及其中有多少是目标里尚不存在的。 */
export async function detectLegacySessions(input: {
  destDataDir: string
  homeDir?: string
}): Promise<LegacySessionDetectResult> {
  const homeDir = input.homeDir ?? homedir()
  const destDir = join(input.destDataDir, 'threads')
  const destReal = await safeRealpath(destDir)
  const existing = new Set(await listThreadDirNames(destDir))

  const sources: LegacySessionDetectedSource[] = []
  for (const candidate of defaultLegacySourceCandidates(homeDir)) {
    if (!(await pathExists(candidate.path))) continue
    // 已经是当前数据目录本身(老版本启动迁移留下的符号链接)——无需再导入。
    if ((await safeRealpath(candidate.path)) === destReal) continue
    const names = await listThreadDirNames(candidate.path)
    if (names.length === 0) continue
    const newCount = names.reduce((count, name) => (existing.has(name) ? count : count + 1), 0)
    sources.push({
      id: candidate.id,
      kind: candidate.kind,
      path: candidate.path,
      threadCount: names.length,
      newCount
    })
  }
  return { destDir, sources }
}

/**
 * 执行导入。sourceDir 为空 = 导入所有自动检测到的默认来源;否则只导入用户
 * 手选的目录。已存在的线程目录一律跳过(skipped),不覆盖。
 */
export async function importLegacySessions(input: {
  destDataDir: string
  homeDir?: string
  sourceDir?: string
  log?: LegacySessionImportLogger
}): Promise<LegacySessionImportSummary> {
  input.log?.('legacy-session-import: blocked before filesystem mutation', {
    code: 'legacy_import_public_validation_required'
  })
  throw new Error(LEGACY_SESSION_IMPORT_BLOCKED_MESSAGE)
}
