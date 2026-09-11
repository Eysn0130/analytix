import { Buffer } from 'node:buffer'
import { createHash, randomUUID } from 'node:crypto'
import { constants as fsConstants } from 'node:fs'
import {
  lstat,
  mkdir,
  open,
  readFile,
  readdir,
  realpath,
  rename,
  rmdir,
  unlink,
  writeFile
} from 'node:fs/promises'
import { homedir } from 'node:os'
import { basename, dirname, isAbsolute, join, resolve, sep } from 'node:path'
import { atomicWriteFile } from '../../packages/runtime/src/adapters/file/atomic-write.js'
import {
  type AccountCredentialDraftV1,
  type AccountCredentialScopeV1,
  providerRegistryProviderInputSchemaV1,
  type ProviderRegistryProviderInputV1
} from '../../packages/runtime/src/contracts/provider-registry.js'
import {
  applyAnalytixRuntimePatch,
  DEFAULT_APP_MOTION_PREFERENCE,
  DEFAULT_ANALYTIX_DATA_DIR,
  DEFAULT_GUI_UPDATE_CHANNEL,
  DEFAULT_MODEL_ENDPOINT_FORMAT,
  DEFAULT_MODEL_PROVIDER_ID,
  DEFAULT_WRITE_WORKSPACE_ROOT,
  defaultClawSettings,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  getAnalytixRuntimeSettings,
  mergeAnalytixRuntimeSettings,
  mergeModelProviderSettings,
  defaultWriteSettings,
  mergeClawSettings,
  mergeAppBehaviorSettings,
  mergeScheduleSettings,
  mergeWriteSettings,
  normalizeAppBehaviorSettings,
  normalizeKeyboardShortcuts,
  scheduleSettingsNeedsMessageKeyMigration,
  migrateLegacyAppSettings,
  normalizeAppSettings,
  normalizeModelProviderId,
  normalizeModelProviderSettings,
  type AppSettingsPatch,
  type AppSettingsV1,
  type ClawImChannelV1,
  type ClawImConversationV1
} from '../shared/app-settings'
import { publicConsoleWarn } from './logger'

export type { AppSettingsV1 }

// 数据默认根目录从 ~/.analytix 升级为 ~/.analytix。老安装的既有目录由
// legacy-data-migration.ts 在启动期搬迁并留兼容链接;settings 里存的旧
// 绝对路径也在那里按迁移结果重写,这里只负责“新值”。
const SETTINGS_FILE_NAME = 'analytix-settings.json'
// 旧版设置文件名。userData 整目录迁移后旧文件会原样留在新目录里,
// 首次加载从它兜底读取,load() 随后把规范化结果另存为新文件名;旧
// 文件保留不动,用户回滚老版本时还能读到可用配置。
const LEGACY_SETTINGS_FILE_NAME = 'analytix-settings.json'
const LEGACY_KUN_SETTINGS_FILE_NAME = 'kun-settings.json'
// 旧版 userData 目录名(更早版本还没有 app.setName 时用过小写包名)。
// 正常情况下迁移模块已把它们 rename 走,这里是迁移失败/被跳过时的
// 跨目录兜底。
const LEGACY_COMPATIBLE_USER_DATA_DIR_NAMES = ['Kun', 'analytix', 'DeepSeek GUI'] as const
const LEGACY_PROVIDER_INSPECTION_MAX_SOURCE_BYTES = 2 << 20
const LEGACY_PROVIDER_ROLLBACK_MAX_SOURCE_BYTES = 256 << 10
const LEGACY_PROVIDER_INSPECTION_MAX_CREDENTIAL_BYTES = 1 << 20
const LEGACY_PROVIDER_INSPECTION_MAX_DECODED_CREDENTIAL_BYTES = 1 << 20
const LEGACY_PROVIDER_INSPECTION_MAX_PROVIDER_ID_BYTES = 64
const LEGACY_PROVIDER_INSPECTION_MAX_PROFILES = 64
const LEGACY_PROVIDER_INSPECTION_MAX_CANDIDATE_SOURCES = 64
const LEGACY_PROVIDER_INSPECTION_FAILURE = 'Legacy Provider credential source inspection failed.'
const LEGACY_PROVIDER_CLEANUP_FAILURE = 'Legacy Provider credential source cleanup failed.'
const LEGACY_PROVIDER_SOURCE_AUTHORITY_FAILURE = 'Legacy Provider credential source authority failed.'
const LEGACY_PROVIDER_PLAINTEXT_PERSISTENCE_FAILURE =
  'Legacy Provider plaintext persistence is not permitted.'
const LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE =
  'Legacy IM plaintext persistence is not permitted.'
const LEGACY_PROVIDER_CLEANUP_PURPOSE = 'remove-verified-legacy-provider-credentials'
const LEGACY_PROVIDER_CLEANUP_CONFIRMATION = 'REMOVE_VERIFIED_LEGACY_PROVIDER_PLAINTEXT'
const LEGACY_PROVIDER_SOURCE_LOCATOR_PATTERN = /^(?:current|compatibility:[0-9]{2}):(?:analytix-settings|kun-settings)\.json$/
const LEGACY_PROVIDER_MIGRATION_ID_PATTERN = /^[a-z0-9][a-z0-9._-]{0,95}$/
const LOWER_SHA256_PATTERN = /^[0-9a-f]{64}$/
const LEGACY_PROVIDER_SOURCE_AUTHORITY_CHALLENGE_PATTERN = /^lmsa_[A-Za-z0-9_-]{43}$/
const LEGACY_PROVIDER_SOURCE_LOCK_SUFFIX = '.analytix-provider-credential-migration.lock'
const LEGACY_PROVIDER_SOURCE_LOCK_WAIT_MS = 10_000
const LEGACY_PROVIDER_SOURCE_LOCK_POLL_MS = 25
const LEGACY_PROVIDER_SOURCE_OWNERLESS_STALE_MS = 30_000
const legacyProviderSourceWriteQueues = new Map<string, Promise<void>>()
let legacyProviderSourceQueueRegistration = Promise.resolve()
const WELCOME_MARKDOWN = `# Welcome to Write

This is your default writing workspace.

- Create Markdown drafts from the sidebar.
- Select text in the editor and ask the writing assistant about it.
- Switch between source, live, split, and preview modes from the top bar.
`

type LegacyProviderSourceLockOwner = {
  schemaVersion: 1
  purpose: 'analytix-provider-credential-migration-source-lock'
  pid: number
  token: string
  createdAtMs: number
  sourceAuthority?: LegacyProviderSourceAuthorityBinding
}

type LegacyProviderSourceAuthorityBinding = {
  schemaVersion: 1
  challenge: string
  operation: 'rollback-commit' | 'finalize'
  migrationId: string
  sourceLocator: string
  sourceSHA256: string
  currentSourceSHA256: string
  sourcePhysicalIdentitySHA256: string
  sourceDevice: string
  sourceInode: string
}

type LegacyProviderSourceLockTestHooks = {
  now?: () => number
  isProcessAlive?: (pid: number) => boolean
  afterStaleObservation?: () => Promise<void>
  afterOwnerInstalled?: () => Promise<void>
}

type LegacyProviderSourceLockObservation =
  | { kind: 'missing' }
  | { kind: 'empty'; mtimeMs: number }
  | { kind: 'owned'; owner: LegacyProviderSourceLockOwner; ownerName: string }

function isFileError(error: unknown, code: string): boolean {
  return typeof error === 'object' && error !== null && 'code' in error && error.code === code
}

function processIsAlive(pid: number): boolean {
  try {
    process.kill(pid, 0)
    return true
  } catch (error) {
    if (isFileError(error, 'ESRCH')) return false
    if (isFileError(error, 'EPERM')) return true
    throw error
  }
}

function delayLegacyProviderSourceLock(): Promise<void> {
  return new Promise((resolveDelay) => setTimeout(resolveDelay, LEGACY_PROVIDER_SOURCE_LOCK_POLL_MS))
}

async function readLegacyProviderSourceLockObservation(
  lockPath: string
): Promise<LegacyProviderSourceLockObservation> {
  try {
    const info = await lstat(lockPath)
    if (!info.isDirectory() || info.isSymbolicLink()) throw new Error('invalid lock')
    const entries = await readdir(lockPath, { withFileTypes: true })
    if (entries.length === 0) return { kind: 'empty', mtimeMs: info.mtimeMs }
    if (entries.length !== 1 || !entries[0].isFile() || entries[0].isSymbolicLink() ||
      !/^owner-[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\.json$/i
        .test(entries[0].name)) throw new Error('invalid lock')
    const ownerPath = join(lockPath, entries[0].name)
    const ownerInfo = await lstat(ownerPath)
    if (!ownerInfo.isFile() || ownerInfo.isSymbolicLink() || ownerInfo.nlink !== 1) {
      throw new Error('invalid lock')
    }
    const parsed = JSON.parse(await readFile(ownerPath, 'utf8')) as Partial<LegacyProviderSourceLockOwner>
    if (parsed.schemaVersion === 1 &&
      parsed.purpose === 'analytix-provider-credential-migration-source-lock' &&
      Number.isInteger(parsed.pid) && (parsed.pid ?? 0) > 0 &&
      typeof parsed.token === 'string' && entries[0].name === `owner-${parsed.token}.json` &&
      typeof parsed.createdAtMs === 'number' && Number.isFinite(parsed.createdAtMs)) {
      return {
        kind: 'owned',
        owner: parsed as LegacyProviderSourceLockOwner,
        ownerName: entries[0].name
      }
    }
    throw new Error('invalid lock')
  } catch (error) {
    if (isFileError(error, 'ENOENT')) return { kind: 'missing' }
    throw error
  }
}

async function removeObservedLegacyProviderSourceLock(
  lockPath: string,
  observation: Exclude<LegacyProviderSourceLockObservation, { kind: 'missing' }>
): Promise<boolean> {
  if (observation.kind === 'owned') {
    const ownerPath = join(lockPath, observation.ownerName)
    let current: LegacyProviderSourceLockObservation
    try {
      current = await readLegacyProviderSourceLockObservation(lockPath)
    } catch {
      return false
    }
    if (current.kind !== 'owned' || current.ownerName !== observation.ownerName ||
      current.owner.token !== observation.owner.token || current.owner.pid !== observation.owner.pid ||
      current.owner.createdAtMs !== observation.owner.createdAtMs) return false
    try {
      await unlink(ownerPath)
    } catch (error) {
      if (isFileError(error, 'ENOENT')) return false
      throw error
    }
  }
  try {
    await rmdir(lockPath)
    return true
  } catch (error) {
    if (isFileError(error, 'ENOENT')) return observation.kind === 'empty'
    if (isFileError(error, 'ENOTEMPTY') || isFileError(error, 'EEXIST')) return false
    throw error
  }
}

async function cleanupLegacyProviderSourceLockCandidate(
  candidatePath: string,
  ownerName: string
): Promise<void> {
  await unlink(join(candidatePath, ownerName)).catch((error: unknown) => {
    if (!isFileError(error, 'ENOENT')) throw error
  })
  await rmdir(candidatePath).catch((error: unknown) => {
    if (!isFileError(error, 'ENOENT')) throw error
  })
}

async function acquireLegacyProviderSourceLock(
  physicalSourcePath: string,
  hooks: LegacyProviderSourceLockTestHooks = {},
  sourceAuthority?: LegacyProviderSourceAuthorityBinding
): Promise<{ release: () => Promise<void>, token: string }> {
  const lockPath = `${physicalSourcePath}${LEGACY_PROVIDER_SOURCE_LOCK_SUFFIX}`
  const now = hooks.now ?? Date.now
  const isProcessAlive = hooks.isProcessAlive ?? processIsAlive
  const startedAt = now()
  const token = randomUUID()
  const ownerName = `owner-${token}.json`
  const owner: LegacyProviderSourceLockOwner = {
    schemaVersion: 1,
    purpose: 'analytix-provider-credential-migration-source-lock',
    pid: process.pid,
    token,
    createdAtMs: now(),
    ...(sourceAuthority === undefined ? {} : { sourceAuthority })
  }
  for (;;) {
    const observation = await readLegacyProviderSourceLockObservation(lockPath)
    if (observation.kind !== 'missing') {
      const stale = observation.kind === 'owned'
        ? !isProcessAlive(observation.owner.pid)
        : now() - observation.mtimeMs >= LEGACY_PROVIDER_SOURCE_OWNERLESS_STALE_MS
      if (stale) {
        await hooks.afterStaleObservation?.()
        if (!await removeObservedLegacyProviderSourceLock(lockPath, observation)) {
          throw new Error('lock changed')
        }
        continue
      }
      if (now() - startedAt >= LEGACY_PROVIDER_SOURCE_LOCK_WAIT_MS) throw new Error('lock timeout')
      await delayLegacyProviderSourceLock()
      continue
    }
    const candidatePath = `${lockPath}.candidate-${token}`
    try {
      await mkdir(candidatePath, { mode: 0o700 })
      const handle = await open(join(candidatePath, ownerName), 'wx', 0o600)
      try {
        await handle.writeFile(JSON.stringify(owner))
        await handle.sync()
      } finally {
        await handle.close()
      }
      const directory = await open(candidatePath, fsConstants.O_RDONLY)
      try {
        await directory.sync()
      } finally {
        await directory.close()
      }
      await rename(candidatePath, lockPath)
      await hooks.afterOwnerInstalled?.()
      const acquired = await readLegacyProviderSourceLockObservation(lockPath)
      if (acquired.kind !== 'owned' || acquired.ownerName !== ownerName ||
        acquired.owner.pid !== process.pid || acquired.owner.token !== token ||
        acquired.owner.createdAtMs !== owner.createdAtMs) {
        await removeObservedLegacyProviderSourceLock(lockPath, {
          kind: 'owned', owner, ownerName
        }).catch(() => false)
        throw new Error('invalid lock')
      }
      return {
        token,
        release: async () => {
          await removeObservedLegacyProviderSourceLock(lockPath, {
            kind: 'owned', owner, ownerName
          })
        }
      }
    } catch (error) {
      await cleanupLegacyProviderSourceLockCandidate(candidatePath, ownerName).catch(() => undefined)
      if (!isFileError(error, 'EEXIST') && !isFileError(error, 'ENOTEMPTY')) {
        throw error
      }
    }
  }
}

async function withLegacyProviderSourceWriteLock<T>(
  physicalSourcePath: string,
  hooks: LegacyProviderSourceLockTestHooks,
  action: (lease: { token: string }) => Promise<T>,
  sourceAuthority?: LegacyProviderSourceAuthorityBinding
): Promise<T> {
  const key = physicalSourcePath
  const registration = legacyProviderSourceQueueRegistration.then(() => {
    const prior = legacyProviderSourceWriteQueues.get(key) ?? Promise.resolve()
    let releaseQueue!: () => void
    const current = new Promise<void>((resolveQueue) => { releaseQueue = resolveQueue })
    const chained = prior.then(() => current)
    legacyProviderSourceWriteQueues.set(key, chained)
    return { prior, current: chained, releaseQueue }
  })
  legacyProviderSourceQueueRegistration = registration.then(() => undefined, () => undefined)
  const queued = await registration
  await queued.prior
  let releaseLock: (() => Promise<void>) | undefined
  try {
    const lease = await acquireLegacyProviderSourceLock(key, hooks, sourceAuthority)
    releaseLock = lease.release
    return await action({ token: lease.token })
  } finally {
    try {
      await releaseLock?.()
    } finally {
      queued.releaseQueue()
      if (legacyProviderSourceWriteQueues.get(key) === queued.current) {
        legacyProviderSourceWriteQueues.delete(key)
      }
    }
  }
}

export function expandHomePath(
  raw: string | null | undefined,
  homeRoot: string = homedir()
): string {
  const value = typeof raw === 'string' ? raw.trim() : ''
  if (!value) return ''
  if (value === '~') return homeRoot
  if (value.startsWith('~/') || value.startsWith('~\\')) {
    return join(homeRoot, value.slice(2))
  }
  return value
}

function defaultWorkspaceRoot(homeRoot: string): string {
  return join(homeRoot, '.analytix', 'default_workspace')
}

function defaultClawChannelsRoot(homeRoot: string): string {
  return join(homeRoot, '.analytix', 'claw')
}

function defaultWriteWorkspaceRoot(homeRoot: string): string {
  return expandHomePath(DEFAULT_WRITE_WORKSPACE_ROOT, homeRoot)
}

function normalizeWorkspaceRoot(
  raw: string | null | undefined,
  homeRoot: string
): string {
  return expandHomePath(raw, homeRoot) || defaultWorkspaceRoot(homeRoot)
}

function normalizeWriteWorkspaceRoot(
  raw: string | null | undefined,
  homeRoot: string
): string {
  return expandHomePath(raw, homeRoot) || defaultWriteWorkspaceRoot(homeRoot)
}

function sanitizePathSegment(raw: string | null | undefined, fallback: string): string {
  const value = typeof raw === 'string' ? raw.trim() : ''
  const sanitized = value
    .replace(/[\\/]/g, '-')
    .replace(/[^A-Za-z0-9._-]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return sanitized || fallback
}

function defaultClawChannelWorkspaceRoot(
  channel: ClawImChannelV1,
  homeRoot: string
): string {
  const account = channel.platformAccount
  const domain = account?.kind === 'feishu'
    ? account.domain
    : account?.kind === 'weixin'
      ? 'weixin'
      : channel.provider
  const accountId = account?.kind === 'feishu'
    ? account.appId
    : account?.kind === 'weixin' || account?.kind === 'telegram'
      ? account.accountId
      : ''
  const workspaceId = sanitizePathSegment(accountId || channel.id, 'channel')
  return join(defaultClawChannelsRoot(homeRoot), channel.provider, domain, workspaceId)
}

function normalizeClawChannelWorkspaceRoot(
  channel: ClawImChannelV1,
  homeRoot: string
): string {
  return expandHomePath(channel.workspaceRoot, homeRoot) ||
    defaultClawChannelWorkspaceRoot(channel, homeRoot)
}

function sanitizeConversationWorkspaceSegment(conversation: ClawImConversationV1): string {
  return sanitizePathSegment(
    conversation.remoteThreadId || conversation.chatId,
    conversation.id || 'conversation'
  )
}

function defaultClawConversationWorkspaceRoot(
  channel: ClawImChannelV1,
  conversation: ClawImConversationV1,
  homeRoot: string
): string {
  return join(
    normalizeClawChannelWorkspaceRoot(channel, homeRoot),
    'conversations',
    sanitizeConversationWorkspaceSegment(conversation)
  )
}

function normalizeClawConversationWorkspaceRoot(
  channel: ClawImChannelV1,
  conversation: ClawImConversationV1,
  homeRoot: string
): string {
  return expandHomePath(conversation.workspaceRoot, homeRoot) ||
    defaultClawConversationWorkspaceRoot(channel, conversation, homeRoot)
}

function normalizeStoredSettings(
  settings: AppSettingsV1,
  homeRoot: string = homedir()
): AppSettingsV1 {
  const normalized = normalizeAppSettings(settings)
  const writeDefaultRoot = normalizeWriteWorkspaceRoot(
    normalized.write.defaultWorkspaceRoot,
    homeRoot
  )
  const writeActiveRoot = normalizeWriteWorkspaceRoot(
    normalized.write.activeWorkspaceRoot || writeDefaultRoot,
    homeRoot
  )
  const writeWorkspaces = [...new Set(
    [
      writeDefaultRoot,
      writeActiveRoot,
      ...normalized.write.workspaces.map((path) => normalizeWriteWorkspaceRoot(path, homeRoot))
    ]
      .filter(Boolean)
  )]
  return {
    ...normalized,
    runtime: {
      ...normalized.runtime,
      dataDir: homeRoot !== homedir() && normalized.runtime.dataDir === DEFAULT_ANALYTIX_DATA_DIR
        ? join(homeRoot, '.analytix', 'data')
        : normalized.runtime.dataDir
    },
    workspaceRoot: normalizeWorkspaceRoot(normalized.workspaceRoot, homeRoot),
    write: {
      ...normalized.write,
      defaultWorkspaceRoot: writeDefaultRoot,
      activeWorkspaceRoot: writeWorkspaces.includes(writeActiveRoot) ? writeActiveRoot : writeDefaultRoot,
      workspaces: writeWorkspaces.length > 0 ? writeWorkspaces : [writeDefaultRoot]
    },
    claw: {
      ...normalized.claw,
      channels: normalized.claw.channels.map((channel) => ({
        ...channel,
        workspaceRoot: normalizeClawChannelWorkspaceRoot(channel, homeRoot),
        conversations: channel.conversations.map((conversation) => ({
          ...conversation,
          workspaceRoot: normalizeClawConversationWorkspaceRoot(channel, conversation, homeRoot)
        }))
      }))
    }
  }
}

function serializeSettingsForDisk(
  settings: AppSettingsV1,
  homeRoot: string = homedir()
): string {
  return JSON.stringify(normalizeStoredSettings(settings, homeRoot), null, 2)
}

export async function ensureWorkspaceRootExists(
  workspaceRoot: string,
  homeRoot: string = homedir()
): Promise<string> {
  const normalized = normalizeWorkspaceRoot(workspaceRoot, homeRoot)
  await mkdir(normalized, { recursive: true })
  return normalized
}

async function ensureIsolatedRuntimeDataDirExists(
  settings: AppSettingsV1,
  homeRoot: string
): Promise<void> {
  if (homeRoot === homedir()) return
  const dataDir = settings.runtime.dataDir
  if (!isAbsolute(dataDir) || resolve(dataDir) !== dataDir ||
    !dataDir.startsWith(`${homeRoot}${sep}`)) {
    return
  }
  await mkdir(dataDir, { recursive: true, mode: 0o700 })
}

async function ensureWriteWorkspaceRootsExist(settings: AppSettingsV1): Promise<void> {
  for (const workspaceRoot of settings.write.workspaces) {
    if (!workspaceRoot) continue
    await mkdir(workspaceRoot, { recursive: true })
  }

  const welcomePath = join(settings.write.defaultWorkspaceRoot, 'welcome.md')
  try {
    await writeFile(welcomePath, WELCOME_MARKDOWN, { encoding: 'utf8', flag: 'wx' })
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error
  }
}

async function ensureClawChannelWorkspaceRootsExist(
  settings: AppSettingsV1,
  homeRoot: string = homedir()
): Promise<void> {
  for (const channel of settings.claw.channels) {
    const workspaceRoot = normalizeClawChannelWorkspaceRoot(channel, homeRoot)
    if (!workspaceRoot) continue
    await mkdir(workspaceRoot, { recursive: true })
    for (const conversation of channel.conversations) {
      const conversationWorkspaceRoot = normalizeClawConversationWorkspaceRoot(
        channel,
        conversation,
        homeRoot
      )
      if (!conversationWorkspaceRoot) continue
      await mkdir(conversationWorkspaceRoot, { recursive: true })
    }
  }
}

function defaultRuntimeSettingsForHomeRoot(homeRoot: string) {
  const defaults = defaultAnalytixRuntimeSettings()
  return homeRoot === homedir()
    ? defaults
    : { ...defaults, dataDir: join(homeRoot, '.analytix', 'data') }
}

const defaultSettings = (homeRoot: string = homedir()): AppSettingsV1 => ({
  version: 1,
  locale: 'zh',
  theme: 'light',
  uiFontScale: 'small',
  motionPreference: DEFAULT_APP_MOTION_PREFERENCE,
  provider: defaultModelProviderSettings(),
  runtime: defaultRuntimeSettingsForHomeRoot(homeRoot),
  workspaceRoot: defaultWorkspaceRoot(homeRoot),
  log: {
    enabled: true,
    retentionDays: 2
  },
  notifications: {
    turnComplete: true
  },
  appBehavior: normalizeAppBehaviorSettings(),
  keyboardShortcuts: normalizeKeyboardShortcuts(),
  guiUpdate: {
    channel: DEFAULT_GUI_UPDATE_CHANNEL
  },
  codePromptPrefix: '',
  disabledSkillIds: [],
  write: defaultWriteSettings(),
  claw: defaultClawSettings(),
  schedule: defaultScheduleSettings()
})

function buildMergedSettings(
  parsed: Partial<AppSettingsV1>,
  homeRoot: string = homedir()
): AppSettingsV1 {
  const migrated = migrateLegacyAppSettings(parsed)
  const defaults = defaultSettings(homeRoot)
  return {
    ...defaults,
    ...migrated,
    provider: mergeModelProviderSettings(defaults.provider, migrated.provider),
    runtime: mergeAnalytixRuntimeSettings(getAnalytixRuntimeSettings(defaults), migrated.runtime),
    log: { ...defaults.log, ...migrated.log },
    notifications: { ...defaults.notifications, ...migrated.notifications },
    appBehavior: mergeAppBehaviorSettings(defaults.appBehavior, migrated.appBehavior),
    keyboardShortcuts: normalizeKeyboardShortcuts(migrated.keyboardShortcuts),
    write: mergeWriteSettings(defaults.write, migrated.write),
    claw: mergeClawSettings(defaults.claw, migrated.claw),
    schedule: mergeScheduleSettings(defaults.schedule, migrated.schedule),
    guiUpdate: { ...defaults.guiUpdate, ...migrated.guiUpdate },
    motionPreference: migrated.motionPreference ?? defaults.motionPreference,
    codePromptPrefix: typeof migrated.codePromptPrefix === 'string' ? migrated.codePromptPrefix : '',
    disabledSkillIds: normalizeDisabledSkillIds(migrated.disabledSkillIds)
  }
}

function normalizeDisabledSkillIds(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return [...new Set(value
    .filter((id): id is string => typeof id === 'string')
    .map((id) => id.trim().replace(/^\/?skill:/i, '').trim())
    .filter(Boolean))]
}

function isErrnoException(error: unknown): error is NodeJS.ErrnoException {
  return typeof error === 'object' && error !== null
}

async function loadDefaultSettings(homeRoot: string = homedir()): Promise<AppSettingsV1> {
  const defaults = normalizeStoredSettings(defaultSettings(homeRoot), homeRoot)
  await ensureIsolatedRuntimeDataDirExists(defaults, homeRoot)
  await ensureWorkspaceRootExists(defaults.workspaceRoot, homeRoot)
  await ensureWriteWorkspaceRootsExist(defaults)
  await ensureClawChannelWorkspaceRootsExist(defaults, homeRoot)
  return defaults
}

function executionPolicyNeedsPersistence(
  parsed: Partial<AppSettingsV1>,
  normalized: AppSettingsV1
): boolean {
  const runtime = parsed.runtime as Partial<AppSettingsV1['runtime']> | undefined
  return runtime?.executionPolicyVersion !== normalized.runtime.executionPolicyVersion ||
    runtime?.approvalPolicy !== normalized.runtime.approvalPolicy ||
    runtime?.sandboxMode !== normalized.runtime.sandboxMode
}

async function writeInvalidSettingsBackup(path: string, raw: string): Promise<string | null> {
  const stamp = new Date().toISOString().replace(/[:.]/g, '-')
  const backupPath = join(
    dirname(path),
    `${basename(path, '.json')}.invalid-${stamp}.json`
  )
  try {
    await atomicWriteFile(backupPath, raw, { mode: 0o600 })
    return backupPath
  } catch {
    return null
  }
}

function compatibleSettingsPaths(currentPath: string): string[] {
  const currentUserDataDir = dirname(currentPath)
  const currentDirName = basename(currentUserDataDir)
  const parentDir = dirname(currentUserDataDir)
  // 顺序:当前目录里的旧文件名(userData 迁移后的常见形态)优先,
  // 然后才是旧目录里的新旧文件名。
  const candidates = [
    join(currentUserDataDir, LEGACY_SETTINGS_FILE_NAME),
    join(currentUserDataDir, LEGACY_KUN_SETTINGS_FILE_NAME)
  ]
  for (const dirName of LEGACY_COMPATIBLE_USER_DATA_DIR_NAMES) {
    if (dirName === currentDirName) continue
    candidates.push(join(parentDir, dirName, SETTINGS_FILE_NAME))
    candidates.push(join(parentDir, dirName, LEGACY_SETTINGS_FILE_NAME))
    if (dirName === 'Kun') candidates.push(join(parentDir, dirName, LEGACY_KUN_SETTINGS_FILE_NAME))
  }
  return [...new Set(candidates)]
}

async function readSettingsFileWithCompatibility(
  currentPath: string
): Promise<{ raw: string, sourcePath: string } | null> {
  const loaded = await readProtectedSettingsBytesWithCompatibility(currentPath)
  if (!loaded) return null
  try {
    return { raw: loaded.raw.toString('utf8'), sourcePath: loaded.sourcePath }
  } finally {
    loaded.raw.fill(0)
  }
}

type SettingsBytesSource = {
  raw: Buffer
  sourcePath: string
  sourceLocator: string
}

type ProtectedSettingsBytesSource = SettingsBytesSource & {
  physicalPath: string
  device: number
  inode: number
}

async function readProtectedSettingsSource(
  sourcePath: string,
  sourceLocator: string
): Promise<ProtectedSettingsBytesSource> {
  const before = await lstat(sourcePath)
  if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1) throw new Error('invalid source')
  const physicalPath = await realpath(sourcePath)
  const handle = await open(sourcePath, fsConstants.O_RDONLY | fsConstants.O_NOFOLLOW)
  try {
    const opened = await handle.stat()
    if (!opened.isFile() || opened.nlink !== 1 || opened.dev !== before.dev || opened.ino !== before.ino) {
      throw new Error('invalid source')
    }
    const raw = await handle.readFile()
    const after = await lstat(sourcePath)
    const physicalAfter = await realpath(sourcePath)
    if (!after.isFile() || after.isSymbolicLink() || after.nlink !== 1 ||
      after.dev !== opened.dev || after.ino !== opened.ino || physicalAfter !== physicalPath) {
      raw.fill(0)
      throw new Error('invalid source')
    }
    return {
      raw,
      sourcePath,
      sourceLocator,
      physicalPath,
      device: opened.dev,
      inode: opened.ino
    }
  } finally {
    await handle.close()
  }
}

function sameProtectedSettingsSource(
  left: ProtectedSettingsBytesSource,
  right: ProtectedSettingsBytesSource
): boolean {
  return left.sourceLocator === right.sourceLocator && left.physicalPath === right.physicalPath &&
    left.device === right.device && left.inode === right.inode
}

function protectedSettingsSourceIdentitySHA256(source: {
  sourceLocator: string
  physicalPath: string
}): string {
  return createHash('sha256')
    .update('analytix-provider-settings-physical-source-v1', 'utf8')
    .update('\0', 'utf8')
    .update(source.sourceLocator, 'utf8')
    .update('\0', 'utf8')
    .update(source.physicalPath, 'utf8')
    .digest('hex')
}

function protectedSettingsSources(currentPath: string): Array<{
  sourcePath: string
  sourceLocator: string
}> {
  return [
    {
      sourcePath: currentPath,
      sourceLocator: `current:${SETTINGS_FILE_NAME}`
    },
    ...compatibleSettingsPaths(currentPath).map((sourcePath, index) => ({
      sourcePath,
      sourceLocator: `compatibility:${String(index).padStart(2, '0')}:${basename(sourcePath)}`
    }))
  ]
}

async function readProtectedSettingsBytesForLocator(
  currentPath: string,
  sourceLocator: string
): Promise<ProtectedSettingsBytesSource | null> {
  const source = protectedSettingsSources(currentPath)
    .find((candidate) => candidate.sourceLocator === sourceLocator)
  if (!source) throw new Error('invalid source')
  try {
    return await readProtectedSettingsSource(source.sourcePath, source.sourceLocator)
  } catch (error) {
    if (isFileError(error, 'ENOENT')) return null
    throw error
  }
}

async function readProtectedSettingsBytesWithCompatibility(
  currentPath: string
): Promise<ProtectedSettingsBytesSource | null> {
  const sources = protectedSettingsSources(currentPath)
  for (const source of sources) {
    try {
      return await readProtectedSettingsSource(source.sourcePath, source.sourceLocator)
    } catch (error) {
      if (isFileError(error, 'ENOENT')) continue
      throw error
    }
  }
  return null
}

function normalizeKunSettingString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function normalizeKunSettingNumber(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? Math.floor(value) : undefined
}

function migrateLegacyKunSettingsFile(parsed: Partial<AppSettingsV1>): Partial<AppSettingsV1> {
  const raw = parsed as Partial<AppSettingsV1> & {
    agents?: {
      kun?: {
        apiKey?: unknown
        baseUrl?: unknown
        model?: unknown
        port?: unknown
        runtimeToken?: unknown
        approvalPolicy?: unknown
        sandboxMode?: unknown
      }
    }
  }
  const kun = raw.agents?.kun
  if (!kun) return parsed

  const apiKey = normalizeKunSettingString(kun.apiKey)
  const baseUrl = normalizeKunSettingString(kun.baseUrl)
  const model = normalizeKunSettingString(kun.model)
  const port = normalizeKunSettingNumber(kun.port)
  const runtimeToken = normalizeKunSettingString(kun.runtimeToken)

  const providerPatch = {
    ...(raw.provider ?? {}),
    ...(apiKey ? { apiKey } : {}),
    ...(baseUrl ? { baseUrl } : {}),
    providers: [
      ...((raw.provider?.providers ?? []).filter((provider) => provider?.id !== DEFAULT_MODEL_PROVIDER_ID)),
      {
        id: DEFAULT_MODEL_PROVIDER_ID,
        name: 'DeepSeek',
        ...(apiKey ? { apiKey } : {}),
        ...(baseUrl ? { baseUrl } : {}),
        endpointFormat: DEFAULT_MODEL_ENDPOINT_FORMAT,
        ...(model ? { models: [model] } : {})
      }
    ]
  }

  return {
    ...raw,
    provider: providerPatch,
    runtime: {
      ...(raw.runtime ?? {}),
      ...(port ? { port } : {}),
      ...(runtimeToken ? { runtimeToken } : {}),
      ...(model ? { model } : {}),
      ...(typeof kun.approvalPolicy === 'string' ? { approvalPolicy: kun.approvalPolicy } : {}),
      ...(typeof kun.sandboxMode === 'string' ? { sandboxMode: kun.sandboxMode } : {}),
      apiKey: '',
      baseUrl: '',
      providerId: ''
    }
  } as Partial<AppSettingsV1>
}

export type LegacyProviderKeyFreeProfileSnapshot = Omit<
  AppSettingsV1['provider']['providers'][number],
  'apiKey'
>

export type LegacyProviderRollbackCredentialArtifactSnapshot = {
  schemaVersion: 1
  sourceLocator: string
  credentialLocators: string[]
  credential: Uint8Array
}

export type LegacyProviderCredentialCandidateSnapshot = {
  schemaVersion: 1
  migrationId: string
  sourceLocator: string
  credentialLocators: string[]
  providerId: string
  providerMetadata: {
    activeProviderId: string
    runtimeModel: string
    proxy: AppSettingsV1['provider']['proxy']
    profile: LegacyProviderKeyFreeProfileSnapshot
  }
  credential: Uint8Array
  rollbackCredentialArtifacts: LegacyProviderRollbackCredentialArtifactSnapshot[]
}

export type LegacyProviderCredentialInspection = {
  schemaVersion: 1
  sourceLocator: string | null
  sourceSHA256: string | null
  expectedCleanedSourceSHA256: string | null
  sourcePhysicalIdentitySHA256: string | null
  sourceSnapshot: Uint8Array | null
  candidates: LegacyProviderCredentialCandidateSnapshot[]
}

export type LegacyProviderCredentialCleanupRequestV1 = {
  schemaVersion: 1
  purpose: typeof LEGACY_PROVIDER_CLEANUP_PURPOSE
  confirmation: typeof LEGACY_PROVIDER_CLEANUP_CONFIRMATION
  sourceLocator: string
  sourceSHA256: string
  expectedCleanedSourceSHA256: string
  protectedCredentialLocators: string[]
}

export type LegacyProviderCredentialCleanupReceiptV1 = {
  schemaVersion: 1
  sourceLocator: string
  sourceSHA256: string
  cleanedSourceSHA256: string
  verifiedSourceSHA256: string
  sourcePhysicalIdentitySHA256: string
  verifiedSource: Uint8Array
}

export type LegacyProviderCredentialSourceReceiptV1 = {
  schemaVersion: 1
  sourceLocator: string
  sourceSHA256: string
  verifiedSourceSHA256: string
  sourcePhysicalIdentitySHA256: string
  verifiedSource: Uint8Array
}

export type LegacyProviderCredentialSourceAuthorityRequestV1 = {
  schemaVersion: 1
  challenge: string
  operation: 'rollback-commit' | 'finalize'
  migrationId: string
  sourceLocator: string
  sourceSHA256: string
  currentSourceSHA256: string
  sourcePhysicalIdentitySHA256: string
}

export type LegacyProviderCredentialSourceAuthorityProofV1 = {
  challenge: string
  sourcePath: string
  lockOwnerToken: string
  sourceDevice: string
  sourceInode: string
}

export type LegacyProviderCredentialLiveSourceAuthorityV1 = LegacyProviderCredentialSourceReceiptV1 & {
  sourceAuthority: LegacyProviderCredentialSourceAuthorityProofV1
}

type ParsedSettingsRecord = Record<string, unknown>
type CredentialValueGroup = {
  credential: Uint8Array
  credentialLocators: string[]
}
type CredentialGroup = CredentialValueGroup & {
  rollbackCredentialArtifacts: CredentialValueGroup[]
}
type CredentialInspectionBudget = {
  sourceCount: number
  decodedCredentialBytes: number
}

function isRecord(value: unknown): value is ParsedSettingsRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function hasOwn(record: ParsedSettingsRecord, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(record, key)
}

function inspectionFailure(): Error {
  return new Error(LEGACY_PROVIDER_INSPECTION_FAILURE)
}

function cleanupFailure(): Error {
  return new Error(LEGACY_PROVIDER_CLEANUP_FAILURE)
}

function isCredentialSentinel(value: string): boolean {
  const normalized = value.trim().toLowerCase()
  if (!normalized) return true
  if ([
    'redacted', '[redacted]', '<redacted>', '__redacted__',
    'masked', '[masked]', '<masked>', '__masked__',
    'unset', 'not-set', 'not_set', 'null', 'undefined'
  ].includes(normalized)) return true
  return /^(?:[a-z0-9]+[-_:])?[*•●x]{4,}$/i.test(normalized)
}

function credentialBytes(value: unknown): Uint8Array | null {
  if (typeof value !== 'string') throw inspectionFailure()
  const normalized = value.trim()
  if (isCredentialSentinel(normalized)) return null
  const bytes = new TextEncoder().encode(normalized)
  if (bytes.byteLength === 0 || bytes.byteLength > LEGACY_PROVIDER_INSPECTION_MAX_CREDENTIAL_BYTES) {
    bytes.fill(0)
    throw inspectionFailure()
  }
  if (new TextDecoder().decode(bytes) !== normalized) {
    bytes.fill(0)
    throw inspectionFailure()
  }
  return bytes
}

function bytesEqual(left: Uint8Array, right: Uint8Array): boolean {
  if (left.byteLength !== right.byteLength) return false
  let difference = 0
  for (let index = 0; index < left.byteLength; index += 1) {
    difference |= left[index] ^ right[index]
  }
  return difference === 0
}

function addCredentialCandidate(
  groups: Map<string, CredentialGroup>,
  providerId: string,
  locator: string,
  value: unknown,
  inspectionBudget: CredentialInspectionBudget
): void {
  const bytes = credentialBytes(value)
  if (!bytes) return
  inspectionBudget.sourceCount += 1
  inspectionBudget.decodedCredentialBytes += bytes.byteLength
  if (
    inspectionBudget.sourceCount > LEGACY_PROVIDER_INSPECTION_MAX_CANDIDATE_SOURCES ||
    inspectionBudget.decodedCredentialBytes > LEGACY_PROVIDER_INSPECTION_MAX_DECODED_CREDENTIAL_BYTES
  ) {
    bytes.fill(0)
    throw inspectionFailure()
  }
  const existing = groups.get(providerId)
  if (!existing) {
    groups.set(providerId, {
      credential: bytes,
      credentialLocators: [locator],
      rollbackCredentialArtifacts: []
    })
    return
  }
  if (bytesEqual(existing.credential, bytes)) {
    bytes.fill(0)
    if (!existing.credentialLocators.includes(locator)) existing.credentialLocators.push(locator)
    return
  }
  const rollbackArtifact = existing.rollbackCredentialArtifacts.find((artifact) => (
    bytesEqual(artifact.credential, bytes)
  ))
  if (!rollbackArtifact) {
    existing.rollbackCredentialArtifacts.push({
      credential: bytes,
      credentialLocators: [locator]
    })
    return
  }
  bytes.fill(0)
  if (!rollbackArtifact.credentialLocators.includes(locator)) {
    rollbackArtifact.credentialLocators.push(locator)
  }
}

function clearCredentialGroup(group: CredentialGroup): void {
  group.credential.fill(0)
  for (const artifact of group.rollbackCredentialArtifacts) artifact.credential.fill(0)
}

function validateOptionalString(record: ParsedSettingsRecord, key: string): void {
  if (hasOwn(record, key) && typeof record[key] !== 'string') throw inspectionFailure()
}

function validateOptionalStringArray(record: ParsedSettingsRecord, key: string): void {
  if (!hasOwn(record, key)) return
  const value = record[key]
  if (!Array.isArray(value) || value.some((item) => typeof item !== 'string')) throw inspectionFailure()
}

function validateProviderMetadataUrl(value: unknown): void {
  if (typeof value !== 'string') throw inspectionFailure()
  const normalized = value.trim()
  if (!normalized) return
  if (normalized.includes('?') || normalized.includes('#')) throw inspectionFailure()
  let parsed: URL
  try {
    parsed = new URL(normalized)
  } catch {
    throw inspectionFailure()
  }
  if (
    (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') ||
    parsed.username ||
    parsed.password
  ) {
    throw inspectionFailure()
  }
}

function validateOptionalProviderMetadataUrl(record: ParsedSettingsRecord, key: string): void {
  if (hasOwn(record, key)) validateProviderMetadataUrl(record[key])
}

function validateProviderProfileMetadataUrls(profile: ParsedSettingsRecord): void {
  validateOptionalProviderMetadataUrl(profile, 'baseUrl')
  for (const capabilityKey of ['image', 'speech', 'textToSpeech', 'music', 'video']) {
    if (!hasOwn(profile, capabilityKey)) continue
    const capability = profile[capabilityKey]
    if (!isRecord(capability)) throw inspectionFailure()
    validateOptionalProviderMetadataUrl(capability, 'baseUrl')
  }
}

function keyFreeProfileMetadata(record: ParsedSettingsRecord): LegacyProviderKeyFreeProfileSnapshot {
  const normalizedId = normalizeModelProviderId(record.id)
  const normalized = normalizeModelProviderSettings({
    providers: [{
      ...(record as Partial<AppSettingsV1['provider']['providers'][number]>)
    }]
  })
  const profile = normalized.providers.find((item) => item.id === normalizedId)
  if (!profile) throw inspectionFailure()
  return cloneKeyFreeValue(profile)
}

function cloneKeyFreeValue<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function assertMetadataUrlsAreKeyFree(value: unknown, key = ''): void {
  if (Array.isArray(value)) {
    for (const item of value) assertMetadataUrlsAreKeyFree(item, key)
    return
  }
  if (isRecord(value)) {
    for (const [childKey, childValue] of Object.entries(value)) {
      assertMetadataUrlsAreKeyFree(childValue, childKey)
    }
    return
  }
  if (typeof value !== 'string' || !/url$/i.test(key) || !value.trim()) return
  validateProviderMetadataUrl(value)
}

function canonicalValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalValue)
  if (!isRecord(value)) return value
  const result: ParsedSettingsRecord = {}
  for (const key of Object.keys(value).sort()) result[key] = canonicalValue(value[key])
  return result
}

function profileMetadataFingerprint(record: ParsedSettingsRecord): string {
  return JSON.stringify(canonicalValue(keyFreeProfileMetadata(record)))
}

function validateAndCollectProviderProfiles(
  provider: ParsedSettingsRecord,
  sourceLocator: string,
  groups: Map<string, CredentialGroup>,
  inspectionBudget: CredentialInspectionBudget
): void {
  if (!hasOwn(provider, 'providers')) return
  const profiles = provider.providers
  if (!Array.isArray(profiles) || profiles.length > LEGACY_PROVIDER_INSPECTION_MAX_PROFILES) {
    throw inspectionFailure()
  }
  const metadataByProviderId = new Map<string, string>()
  const identityByProviderId = new Map<string, string>()
  for (let index = 0; index < profiles.length; index += 1) {
    const profile = profiles[index]
    if (!isRecord(profile) || typeof profile.id !== 'string') throw inspectionFailure()
    const rawProviderId = profile.id.trim()
    if (new TextEncoder().encode(rawProviderId).byteLength > LEGACY_PROVIDER_INSPECTION_MAX_PROVIDER_ID_BYTES) {
      throw inspectionFailure()
    }
    const providerId = normalizeModelProviderId(profile.id)
    if (!providerId) throw inspectionFailure()
    const normalizedIdentity = rawProviderId.toLowerCase()
    const existingIdentity = identityByProviderId.get(providerId)
    if (existingIdentity !== undefined && existingIdentity !== normalizedIdentity) throw inspectionFailure()
    identityByProviderId.set(providerId, normalizedIdentity)
    validateOptionalString(profile, 'name')
    validateOptionalString(profile, 'baseUrl')
    validateProviderProfileMetadataUrls(profile)
    validateOptionalString(profile, 'endpointFormat')
    validateOptionalStringArray(profile, 'models')
    const fingerprint = profileMetadataFingerprint(profile)
    const existingFingerprint = metadataByProviderId.get(providerId)
    if (existingFingerprint !== undefined && existingFingerprint !== fingerprint) throw inspectionFailure()
    metadataByProviderId.set(providerId, fingerprint)
    if (hasOwn(profile, 'apiKey')) {
      addCredentialCandidate(
        groups,
        providerId,
        `${sourceLocator}:provider.providers[${index}].apiKey`,
        profile.apiKey,
        inspectionBudget
      )
    }
  }
}

function collectCanonicalProviderCredentials(
  parsed: ParsedSettingsRecord,
  sourceLocator: string,
  groups: Map<string, CredentialGroup>,
  inspectionBudget: CredentialInspectionBudget
): void {
  const provider = parsed.provider
  if (!isRecord(provider)) throw inspectionFailure()
  validateOptionalString(provider, 'activeProviderId')
  validateOptionalString(provider, 'baseUrl')
  validateOptionalProviderMetadataUrl(provider, 'baseUrl')
  if (hasOwn(provider, 'apiKey')) {
    addCredentialCandidate(
      groups,
      DEFAULT_MODEL_PROVIDER_ID,
      `${sourceLocator}:provider.apiKey`,
      provider.apiKey,
      inspectionBudget
    )
  }
  validateAndCollectProviderProfiles(provider, sourceLocator, groups, inspectionBudget)
}

function selectedLegacyCredential(
  parsed: ParsedSettingsRecord
): { locator: string; value: unknown } | null {
  if (hasOwn(parsed, 'runtime')) {
    if (!isRecord(parsed.runtime)) throw inspectionFailure()
    validateOptionalProviderMetadataUrl(parsed.runtime, 'baseUrl')
    return hasOwn(parsed.runtime, 'apiKey')
      ? { locator: 'runtime.apiKey', value: parsed.runtime.apiKey }
      : null
  }

  if (hasOwn(parsed, 'agentProvider') && typeof parsed.agentProvider !== 'string') throw inspectionFailure()
  const agentProvider = typeof parsed.agentProvider === 'string' ? parsed.agentProvider : ''
  const deepseek = hasOwn(parsed, 'deepseek')
    ? parsed.deepseek
    : undefined
  if (deepseek !== undefined && !isRecord(deepseek)) throw inspectionFailure()
  const agents = hasOwn(parsed, 'agents') ? parsed.agents : undefined
  if (agents !== undefined && !isRecord(agents)) throw inspectionFailure()

  if (!agentProvider && isRecord(deepseek)) {
    validateOptionalProviderMetadataUrl(deepseek, 'baseUrl')
    return hasOwn(deepseek, 'apiKey')
      ? { locator: 'deepseek.apiKey', value: deepseek.apiKey }
      : null
  }
  if (agentProvider === 'reasonix') {
    const reasonix = isRecord(agents) ? agents.reasonix : undefined
    if (reasonix !== undefined && !isRecord(reasonix)) throw inspectionFailure()
    if (isRecord(reasonix)) validateOptionalProviderMetadataUrl(reasonix, 'baseUrl')
    return isRecord(reasonix) && hasOwn(reasonix, 'apiKey')
      ? { locator: 'agents.reasonix.apiKey', value: reasonix.apiKey }
      : null
  }
  if (agentProvider === 'codewhale') {
    const codewhale = isRecord(agents) ? agents.codewhale : undefined
    if (codewhale !== undefined && !isRecord(codewhale)) throw inspectionFailure()
    if (isRecord(codewhale)) validateOptionalProviderMetadataUrl(codewhale, 'baseUrl')
    if (isRecord(deepseek)) validateOptionalProviderMetadataUrl(deepseek, 'baseUrl')
    if (isRecord(deepseek) && hasOwn(deepseek, 'apiKey')) {
      return { locator: 'deepseek.apiKey', value: deepseek.apiKey }
    }
    return isRecord(codewhale) && hasOwn(codewhale, 'apiKey')
      ? { locator: 'agents.codewhale.apiKey', value: codewhale.apiKey }
      : null
  }
  if (agentProvider === 'deepseek-runtime') {
    if (isRecord(deepseek)) validateOptionalProviderMetadataUrl(deepseek, 'baseUrl')
    return isRecord(deepseek) && hasOwn(deepseek, 'apiKey')
      ? { locator: 'deepseek.apiKey', value: deepseek.apiKey }
      : null
  }
  return null
}

function collectLegacySeedCredential(
  parsed: ParsedSettingsRecord,
  sourceLocator: string,
  groups: Map<string, CredentialGroup>,
  inspectionBudget: CredentialInspectionBudget
): void {
  const selected = selectedLegacyCredential(parsed)
  if (selected) {
    addCredentialCandidate(
      groups,
      DEFAULT_MODEL_PROVIDER_ID,
      `${sourceLocator}:${selected.locator}`,
      selected.value,
      inspectionBudget
    )
  }
  const agentProvider = typeof parsed.agentProvider === 'string' ? parsed.agentProvider : ''
  if (agentProvider !== 'codewhale') return
  if (selected?.locator === 'deepseek.apiKey' && !groups.has(DEFAULT_MODEL_PROVIDER_ID)) return
  const agents = isRecord(parsed.agents) ? parsed.agents : undefined
  const codewhale = isRecord(agents?.codewhale) ? agents.codewhale : undefined
  if (!codewhale || !hasOwn(codewhale, 'apiKey') || selected?.locator === 'agents.codewhale.apiKey') return
  addCredentialCandidate(
    groups,
    DEFAULT_MODEL_PROVIDER_ID,
    `${sourceLocator}:agents.codewhale.apiKey`,
    codewhale.apiKey,
    inspectionBudget
  )
}

function collectKunCredentials(
  parsed: ParsedSettingsRecord,
  sourceLocator: string,
  groups: Map<string, CredentialGroup>,
  inspectionBudget: CredentialInspectionBudget
): void {
  const agents = hasOwn(parsed, 'agents') ? parsed.agents : undefined
  if (agents !== undefined && !isRecord(agents)) throw inspectionFailure()
  const kun = isRecord(agents) ? agents.kun : undefined
  if (kun !== undefined && !isRecord(kun)) throw inspectionFailure()
  if (isRecord(kun)) validateOptionalProviderMetadataUrl(kun, 'baseUrl')
  const provider = hasOwn(parsed, 'provider') ? parsed.provider : undefined
  if (provider !== undefined && !isRecord(provider)) throw inspectionFailure()
  if (isRecord(provider)) validateOptionalProviderMetadataUrl(provider, 'baseUrl')
  if (isRecord(kun) && hasOwn(kun, 'apiKey')) {
    addCredentialCandidate(
      groups,
      DEFAULT_MODEL_PROVIDER_ID,
      `${sourceLocator}:agents.kun.apiKey`,
      kun.apiKey,
      inspectionBudget
    )
  }
  if (isRecord(provider) && hasOwn(provider, 'apiKey')) {
    addCredentialCandidate(
      groups,
      DEFAULT_MODEL_PROVIDER_ID,
      `${sourceLocator}:provider.apiKey`,
      provider.apiKey,
      inspectionBudget
    )
  }
  if (isRecord(provider)) {
    validateAndCollectProviderProfiles(provider, sourceLocator, groups, inspectionBudget)
  }
}

function migrationIdFor(sourceLocator: string, providerId: string): string {
  const digest = createHash('sha256')
    .update(sourceLocator, 'utf8')
    .update('\0', 'utf8')
    .update(providerId, 'utf8')
    .digest('hex')
  return `legacy-provider-settings-v1-${digest}`
}

function appendSentinelRollbackArtifacts(
  candidates: LegacyProviderCredentialCandidateSnapshot[],
  sourceLocator: string,
  parsed: ParsedSettingsRecord
): void {
  if (candidates.length === 0) return
  const sentinelGroups = new Map<string, { credential: Uint8Array; credentialLocators: string[] }>()
  let totalBytes = 0
  for (const field of collectCleanupCredentialFields(parsed, sourceLocator)) {
    if (!field.sentinel) continue
    const existing = sentinelGroups.get(field.value)
    if (existing) {
      existing.credentialLocators.push(field.locator)
      continue
    }
    const credential = new TextEncoder().encode(field.value)
    totalBytes += credential.byteLength
    if (credential.byteLength === 0 ||
      totalBytes > LEGACY_PROVIDER_INSPECTION_MAX_DECODED_CREDENTIAL_BYTES) {
      credential.fill(0)
      for (const group of sentinelGroups.values()) group.credential.fill(0)
      throw inspectionFailure()
    }
    sentinelGroups.set(field.value, { credential, credentialLocators: [field.locator] })
  }
  for (const group of sentinelGroups.values()) {
    if (candidates[0].rollbackCredentialArtifacts.length >= 64) {
      for (const remaining of sentinelGroups.values()) remaining.credential.fill(0)
      throw inspectionFailure()
    }
    candidates[0].rollbackCredentialArtifacts.push({
      schemaVersion: 1,
      sourceLocator: group.credentialLocators[0],
      credentialLocators: group.credentialLocators,
      credential: group.credential
    })
  }
}

function buildCredentialCandidates(
  sourceLocator: string,
  parsed: ParsedSettingsRecord,
  groups: Map<string, CredentialGroup>,
  isKunSource: boolean
): LegacyProviderCredentialCandidateSnapshot[] {
  const migratedInput = isKunSource
    ? migrateLegacyKunSettingsFile(parsed as Partial<AppSettingsV1>)
    : parsed as Partial<AppSettingsV1>
  const normalizedSettings = buildMergedSettings(migratedInput)
  const provider = normalizedSettings.provider
  const normalizedSourceProvider = normalizeModelProviderSettings(migratedInput.provider)
  const requestedActiveProviderId = typeof migratedInput.provider?.activeProviderId === 'string'
    ? migratedInput.provider.activeProviderId.trim()
    : ''
  if (requestedActiveProviderId && !provider.activeProviderId) throw inspectionFailure()
  const runtimeModel = typeof migratedInput.runtime?.model === 'string'
    ? migratedInput.runtime.model.trim()
    : ''
  const candidates: LegacyProviderCredentialCandidateSnapshot[] = [...groups.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([providerId, group]) => {
      const profile = provider.providers.find((item) => item.id === providerId)
      if (!profile) throw inspectionFailure()
      const keyFreeProfile = cloneKeyFreeValue(profile)
      assertMetadataUrlsAreKeyFree(keyFreeProfile)
      return {
        schemaVersion: 1,
        migrationId: migrationIdFor(sourceLocator, providerId),
        sourceLocator: group.credentialLocators[0],
        credentialLocators: [...group.credentialLocators],
        providerId,
        providerMetadata: {
          activeProviderId: provider.activeProviderId ?? '',
          runtimeModel,
          proxy: cloneKeyFreeValue(normalizedSourceProvider.proxy),
          profile: keyFreeProfile
        },
        credential: group.credential,
        rollbackCredentialArtifacts: group.rollbackCredentialArtifacts.map((artifact) => ({
          schemaVersion: 1,
          sourceLocator: artifact.credentialLocators[0],
          credentialLocators: [...artifact.credentialLocators],
          credential: artifact.credential
        }))
      }
    })
  appendSentinelRollbackArtifacts(candidates, sourceLocator, parsed)
  for (const candidate of candidates) {
    mapLegacyProviderCredentialCandidateToProviderRegistryProviderInputV1(candidate)
  }
  return candidates
}

type LegacyProviderCredentialField = {
  record: ParsedSettingsRecord
  key: 'apiKey'
  locator: string
  value: string
  sentinel: boolean
}

function collectCleanupCredentialField(
  fields: LegacyProviderCredentialField[],
  record: ParsedSettingsRecord | undefined,
  locator: string
): void {
  if (!record || !hasOwn(record, 'apiKey')) return
  if (typeof record.apiKey !== 'string') throw cleanupFailure()
  fields.push({
    record,
    key: 'apiKey',
    locator,
    value: record.apiKey,
    sentinel: isCredentialSentinel(record.apiKey)
  })
}

function collectCleanupCredentialFields(
  parsed: ParsedSettingsRecord,
  sourceLocator: string
): LegacyProviderCredentialField[] {
  const fields: LegacyProviderCredentialField[] = []
  const provider = hasOwn(parsed, 'provider') ? parsed.provider : undefined
  if (provider !== undefined && !isRecord(provider)) throw cleanupFailure()
  if (isRecord(provider)) {
    collectCleanupCredentialField(fields, provider, `${sourceLocator}:provider.apiKey`)
    if (hasOwn(provider, 'providers')) {
      if (!Array.isArray(provider.providers) || provider.providers.length > LEGACY_PROVIDER_INSPECTION_MAX_PROFILES) {
        throw cleanupFailure()
      }
      for (let index = 0; index < provider.providers.length; index += 1) {
        const profile = provider.providers[index]
        if (!isRecord(profile)) throw cleanupFailure()
        collectCleanupCredentialField(
          fields,
          profile,
          `${sourceLocator}:provider.providers[${index}].apiKey`
        )
      }
    }
  }

  const runtime = hasOwn(parsed, 'runtime') ? parsed.runtime : undefined
  if (runtime !== undefined && !isRecord(runtime)) throw cleanupFailure()
  collectCleanupCredentialField(fields, isRecord(runtime) ? runtime : undefined, `${sourceLocator}:runtime.apiKey`)

  const deepseek = hasOwn(parsed, 'deepseek') ? parsed.deepseek : undefined
  if (deepseek !== undefined && !isRecord(deepseek)) throw cleanupFailure()
  collectCleanupCredentialField(fields, isRecord(deepseek) ? deepseek : undefined, `${sourceLocator}:deepseek.apiKey`)

  const agents = hasOwn(parsed, 'agents') ? parsed.agents : undefined
  if (agents !== undefined && !isRecord(agents)) throw cleanupFailure()
  for (const agentName of ['kun', 'reasonix', 'codewhale'] as const) {
    const agent = isRecord(agents) && hasOwn(agents, agentName) ? agents[agentName] : undefined
    if (agent !== undefined && !isRecord(agent)) throw cleanupFailure()
    collectCleanupCredentialField(
      fields,
      isRecord(agent) ? agent : undefined,
      `${sourceLocator}:agents.${agentName}.apiKey`
    )
  }
  return fields
}

function protectedLocatorsFromCandidates(
  candidates: LegacyProviderCredentialCandidateSnapshot[]
): string[] {
  return candidates.flatMap((candidate) => [
    ...candidate.credentialLocators,
    ...candidate.rollbackCredentialArtifacts.flatMap((artifact) => artifact.credentialLocators)
  ]).sort()
}

function sameStringList(left: string[], right: string[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index])
}

function validCleanupRequest(input: unknown): input is LegacyProviderCredentialCleanupRequestV1 {
  if (!isRecord(input)) return false
  const keys = Object.keys(input).sort()
  if (!sameStringList(keys, [
    'confirmation',
    'expectedCleanedSourceSHA256',
    'protectedCredentialLocators',
    'purpose',
    'schemaVersion',
    'sourceLocator',
    'sourceSHA256'
  ])) return false
  if (input.schemaVersion !== 1 || input.purpose !== LEGACY_PROVIDER_CLEANUP_PURPOSE ||
    input.confirmation !== LEGACY_PROVIDER_CLEANUP_CONFIRMATION ||
    typeof input.sourceLocator !== 'string' ||
    !LEGACY_PROVIDER_SOURCE_LOCATOR_PATTERN.test(input.sourceLocator) ||
    typeof input.sourceSHA256 !== 'string' || !LOWER_SHA256_PATTERN.test(input.sourceSHA256) ||
    typeof input.expectedCleanedSourceSHA256 !== 'string' ||
    !LOWER_SHA256_PATTERN.test(input.expectedCleanedSourceSHA256) ||
    !Array.isArray(input.protectedCredentialLocators) ||
    input.protectedCredentialLocators.length > LEGACY_PROVIDER_INSPECTION_MAX_CANDIDATE_SOURCES * 4) {
    return false
  }
  const locators = input.protectedCredentialLocators
  return locators.every((locator) => (
    typeof locator === 'string' && locator.startsWith(`${input.sourceLocator}:`) && locator.length <= 1024
  )) && new Set(locators).size === locators.length &&
    sameStringList(locators, [...locators].sort())
}

function containsCredentialCanary(value: unknown, canaries: Set<string>): boolean {
  if (typeof value === 'string') return canaries.has(value) || canaries.has(value.trim())
  if (Array.isArray(value)) return value.some((item) => containsCredentialCanary(item, canaries))
  if (!isRecord(value)) return false
  return Object.values(value).some((item) => containsCredentialCanary(item, canaries))
}

function containsLegacyProviderPlaintext(parsed: ParsedSettingsRecord, sourceLocator: string): boolean {
  return collectCleanupCredentialFields(parsed, sourceLocator).some((field) => !field.sentinel)
}

export type LegacyImAccountCredentialCandidateV1 = {
  channelId: string
  scope: AccountCredentialScopeV1
  credential: AccountCredentialDraftV1
  platformAccount: NonNullable<ClawImChannelV1['platformAccount']>
}

export type LegacyImAccountCredentialInspectionV1 = {
  schemaVersion: 1
  sourceLocator: string
  sourceSHA256: string
  sourceDevice: string
  sourceInode: string
  candidates: LegacyImAccountCredentialCandidateV1[]
}

function legacyImString(record: Record<string, unknown>, key: string, maximum = 16 << 10): string {
  const value = typeof record[key] === 'string' ? record[key].trim() : ''
  if (!value || Buffer.byteLength(value, 'utf8') > maximum || /[\u0000\r\n]/.test(value)) {
    throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
  }
  return value
}

function collectLegacyImAccountCredentials(parsed: ParsedSettingsRecord): LegacyImAccountCredentialCandidateV1[] {
  const claw = isRecord(parsed.claw) ? parsed.claw : null
  if (!claw || claw.channels === undefined) return []
  if (!Array.isArray(claw.channels) || claw.channels.length > 64) {
    throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
  }
  const candidates: LegacyImAccountCredentialCandidateV1[] = []
  for (const entry of claw.channels) {
    if (!isRecord(entry) || entry.platformCredential === undefined) continue
    if (!isRecord(entry.platformCredential)) throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
    const legacy = entry.platformCredential
    const channelId = legacyImString(entry, 'id', 512)
    const kind = typeof legacy.kind === 'string' ? legacy.kind : entry.provider
    const createdAt = typeof legacy.createdAt === 'string' && legacy.createdAt
      ? legacy.createdAt
      : new Date(0).toISOString()
    if (kind === 'telegram') {
      const accountId = typeof legacy.accountId === 'string' && legacy.accountId.trim()
        ? legacy.accountId.trim()
        : channelId
      const allowedChatIds = typeof legacy.allowedChatIds === 'string' ? legacy.allowedChatIds.trim() : ''
      const botUsername = typeof legacy.botUsername === 'string' ? legacy.botUsername.trim().replace(/^@+/, '') : ''
      candidates.push({
        channelId,
        scope: { owner: 'transport', provider: 'telegram', accountId, channelId, purpose: 'transport-telegram-bot-token' },
        credential: { kind: 'telegram', botToken: legacyImString(legacy, 'botToken'), allowedChatIds },
        platformAccount: {
          kind: 'telegram', accountId, allowedChatIds, ...(botUsername ? { botUsername } : {}), createdAt
        }
      })
      continue
    }
    if (kind === 'weixin') {
      const accountId = legacyImString(legacy, 'accountId', 512)
      candidates.push({
        channelId,
        scope: { owner: 'transport', provider: 'weixin', accountId, channelId, purpose: 'transport-weixin-session-key' },
        credential: { kind: 'weixin', sessionKey: legacyImString(legacy, 'sessionKey') },
        platformAccount: { kind: 'weixin', accountId, createdAt }
      })
      continue
    }
    if (kind === 'feishu') {
      const appId = legacyImString(legacy, 'appId', 512)
      const accountId = typeof legacy.accountId === 'string' && legacy.accountId.trim()
        ? legacy.accountId.trim()
        : appId
      const domain = typeof legacy.domain === 'string' && legacy.domain.trim() ? legacy.domain.trim() : 'feishu'
      candidates.push({
        channelId,
        scope: { owner: 'transport', provider: 'feishu', accountId, channelId, purpose: 'transport-feishu-app-secret' },
        credential: { kind: 'feishu', appSecret: legacyImString(legacy, 'appSecret') },
        platformAccount: { kind: 'feishu', accountId, appId, domain, createdAt }
      })
      continue
    }
    throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
  }
  return candidates
}

function containsLegacyImPlaintext(parsed: ParsedSettingsRecord): boolean {
  return collectLegacyImAccountCredentials(parsed).length > 0
}

function serializeCleanedSettings(raw: Buffer, parsed: ParsedSettingsRecord): string {
  const rawText = new TextDecoder('utf-8', { fatal: true }).decode(raw)
  const indentation = rawText.match(/\r?\n([\t ]+)"/)?.[1]
  const newline = rawText.includes('\r\n') ? '\r\n' : '\n'
  const trailingNewline = /(?:\r\n|\n)$/.test(rawText)
  let serialized = JSON.stringify(parsed, null, indentation)
  if (newline === '\r\n') serialized = serialized.replace(/\n/g, '\r\n')
  if (trailingNewline) serialized += newline
  return serialized
}

function parseCleanupSource(raw: Buffer): ParsedSettingsRecord {
  if (raw.byteLength > LEGACY_PROVIDER_INSPECTION_MAX_SOURCE_BYTES) throw cleanupFailure()
  const parsed = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(raw)) as unknown
  if (!isRecord(parsed)) throw cleanupFailure()
  return parsed
}

function validateCleanupInventory(
  parsed: ParsedSettingsRecord,
  sourceLocator: string,
  isKunSource: boolean,
  protectedCredentialLocators: string[]
): LegacyProviderCredentialField[] {
  const groups = new Map<string, CredentialGroup>()
  let candidates: LegacyProviderCredentialCandidateSnapshot[] = []
  try {
    const inspectionBudget: CredentialInspectionBudget = {
      sourceCount: 0,
      decodedCredentialBytes: 0
    }
    if (isKunSource) {
      collectKunCredentials(parsed, sourceLocator, groups, inspectionBudget)
    } else if (hasOwn(parsed, 'provider')) {
      collectCanonicalProviderCredentials(parsed, sourceLocator, groups, inspectionBudget)
    } else {
      collectLegacySeedCredential(parsed, sourceLocator, groups, inspectionBudget)
    }
    candidates = buildCredentialCandidates(sourceLocator, parsed, groups, isKunSource)
    if (!sameStringList(protectedLocatorsFromCandidates(candidates), protectedCredentialLocators)) {
      throw cleanupFailure()
    }
    const fields = collectCleanupCredentialFields(parsed, sourceLocator)
    const protectedFields = fields
      .filter((field) => candidates.length > 0 || !field.sentinel)
      .map((field) => field.locator)
      .sort()
    if (!sameStringList(protectedFields, protectedCredentialLocators) ||
      fields.some((field) => !field.sentinel && field.value !== field.value.trim())) {
      throw cleanupFailure()
    }
    return fields
  } catch {
    throw cleanupFailure()
  } finally {
    for (const candidate of candidates) {
      candidate.credential.fill(0)
      for (const artifact of candidate.rollbackCredentialArtifacts) artifact.credential.fill(0)
    }
    for (const group of groups.values()) clearCredentialGroup(group)
  }
}

function stableUniqueMediaModels(profile: LegacyProviderKeyFreeProfileSnapshot): string[] {
  const models: string[] = []
  const seen = new Set<string>()
  for (const capability of [
    profile.image,
    profile.speech,
    profile.textToSpeech,
    profile.music,
    profile.video
  ]) {
    for (const model of capability?.models ?? []) {
      if (seen.has(model)) continue
      seen.add(model)
      models.push(model)
    }
  }
  return models
}

export function mapLegacyProviderCredentialCandidateToProviderRegistryProviderInputV1(
  candidate: LegacyProviderCredentialCandidateSnapshot
): ProviderRegistryProviderInputV1 {
  const profile = candidate.providerMetadata.profile
  const activeProviderId = candidate.providerMetadata.activeProviderId
  const isSelectedCandidate = activeProviderId
    ? activeProviderId === candidate.providerId
    : candidate.providerId === DEFAULT_MODEL_PROVIDER_ID
  const runtimeModel = candidate.providerMetadata.runtimeModel
  const selectedModel = isSelectedCandidate && profile.models.includes(runtimeModel)
    ? runtimeModel
    : ''
  const proxy = candidate.providerMetadata.proxy.enabled
    ? candidate.providerMetadata.proxy.url
    : ''
  const kind = candidate.providerId === DEFAULT_MODEL_PROVIDER_ID
    ? DEFAULT_MODEL_PROVIDER_ID
    : profile.endpointFormat === 'messages'
      ? 'anthropic-compatible'
      : profile.endpointFormat === 'custom_endpoint'
        ? 'custom-endpoint'
        : 'openai-compatible'
  const mapped = providerRegistryProviderInputSchemaV1.safeParse({
    id: candidate.providerId,
    kind,
    endpoint: profile.baseUrl,
    proxy,
    models: profile.models,
    mediaModels: stableUniqueMediaModels(profile),
    selectedModel,
    selectedMediaModel: '',
    selectedRoutes: []
  })
  if (!mapped.success) throw inspectionFailure()
  return mapped.data
}

export class JsonSettingsStore {
  private path: string
  private homeRoot: string
  private cache: AppSettingsV1 | null = null
  private legacyProviderCleanupAtomicWriteFile: typeof atomicWriteFile
  private legacyProviderBeforeExclusiveWrite: () => Promise<void>
  private legacyProviderSourceLockTestHooks: LegacyProviderSourceLockTestHooks

  constructor(userDataPath: string, options: {
    homeRoot?: string
    legacyProviderCleanupAtomicWriteFile?: typeof atomicWriteFile
    legacyProviderBeforeExclusiveWrite?: () => Promise<void>
    legacyProviderSourceLockTestHooks?: LegacyProviderSourceLockTestHooks
  } = {}) {
    this.path = join(userDataPath, SETTINGS_FILE_NAME)
    const homeRoot = options.homeRoot ?? homedir()
    if (!isAbsolute(homeRoot) || resolve(homeRoot) !== homeRoot) {
      throw new Error('Settings home root must be an absolute normalized path.')
    }
    this.homeRoot = homeRoot
    this.legacyProviderCleanupAtomicWriteFile = options.legacyProviderCleanupAtomicWriteFile ?? atomicWriteFile
    this.legacyProviderBeforeExclusiveWrite = options.legacyProviderBeforeExclusiveWrite ?? (async () => {})
    this.legacyProviderSourceLockTestHooks = options.legacyProviderSourceLockTestHooks ?? {}
  }

  async inspectLegacyProviderCredentialSources(
    exactSourceLocator?: string
  ): Promise<LegacyProviderCredentialInspection> {
    let loaded: ProtectedSettingsBytesSource | null
    try {
      loaded = exactSourceLocator === undefined
        ? await readProtectedSettingsBytesWithCompatibility(this.path)
        : await readProtectedSettingsBytesForLocator(this.path, exactSourceLocator)
    } catch {
      throw inspectionFailure()
    }
    if (!loaded) {
      return {
        schemaVersion: 1,
        sourceLocator: null,
        sourceSHA256: null,
        expectedCleanedSourceSHA256: null,
        sourcePhysicalIdentitySHA256: null,
        sourceSnapshot: null,
        candidates: []
      }
    }

    const sourceSHA256 = createHash('sha256').update(loaded.raw).digest('hex')
    const groups = new Map<string, CredentialGroup>()
    try {
      if (loaded.raw.byteLength > LEGACY_PROVIDER_INSPECTION_MAX_SOURCE_BYTES) throw inspectionFailure()
      if (loaded.raw.byteLength > LEGACY_PROVIDER_ROLLBACK_MAX_SOURCE_BYTES) throw inspectionFailure()
      const parsed = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(loaded.raw)) as unknown
      if (!isRecord(parsed)) throw inspectionFailure()
      const inspectionBudget: CredentialInspectionBudget = {
        sourceCount: 0,
        decodedCredentialBytes: 0
      }
      const isKunSource = basename(loaded.sourcePath) === LEGACY_KUN_SETTINGS_FILE_NAME
      if (isKunSource) {
        collectKunCredentials(parsed, loaded.sourceLocator, groups, inspectionBudget)
      } else if (hasOwn(parsed, 'provider')) {
        collectCanonicalProviderCredentials(parsed, loaded.sourceLocator, groups, inspectionBudget)
      } else {
        collectLegacySeedCredential(parsed, loaded.sourceLocator, groups, inspectionBudget)
      }
      const candidates = buildCredentialCandidates(loaded.sourceLocator, parsed, groups, isKunSource)
      let expectedCleanedSourceSHA256 = sourceSHA256
      const fields = collectCleanupCredentialFields(parsed, loaded.sourceLocator)
      if (candidates.length > 0 || (fields.length > 0 && fields.every((field) => field.sentinel))) {
        const fieldLocators = new Set(fields.map((field) => field.locator))
        if (protectedLocatorsFromCandidates(candidates).some((locator) => !fieldLocators.has(locator))) {
          throw inspectionFailure()
        }
        const canaries = new Set(fields
          .filter((field) => !field.sentinel)
          .flatMap((field) => [field.value, field.value.trim()]))
        for (const field of fields) delete field.record[field.key]
        if (containsCredentialCanary(parsed, canaries)) throw inspectionFailure()
        expectedCleanedSourceSHA256 = createHash('sha256')
          .update(serializeCleanedSettings(loaded.raw, parsed), 'utf8')
          .digest('hex')
      }
      return {
        schemaVersion: 1,
        sourceLocator: loaded.sourceLocator,
        sourceSHA256,
        expectedCleanedSourceSHA256,
        sourcePhysicalIdentitySHA256: protectedSettingsSourceIdentitySHA256(loaded),
        sourceSnapshot: Uint8Array.from(loaded.raw),
        candidates
      }
    } catch {
      for (const group of groups.values()) clearCredentialGroup(group)
      throw inspectionFailure()
    } finally {
      loaded.raw.fill(0)
    }
  }

  async cleanupLegacyProviderCredentialSource(
    input: LegacyProviderCredentialCleanupRequestV1
  ): Promise<LegacyProviderCredentialCleanupReceiptV1> {
    if (!validCleanupRequest(input)) throw cleanupFailure()
    let loaded: ProtectedSettingsBytesSource | null = null
    let expectedSourceRaw: Buffer | null = null
    let verifiedSource: Buffer | null = null
    try {
      loaded = await readProtectedSettingsBytesWithCompatibility(this.path)
      if (!loaded || loaded.sourceLocator !== input.sourceLocator ||
        createHash('sha256').update(loaded.raw).digest('hex') !== input.sourceSHA256) {
        throw cleanupFailure()
      }
      const parsed = parseCleanupSource(loaded.raw)
      const fields = validateCleanupInventory(
        parsed,
        loaded.sourceLocator,
        basename(loaded.sourcePath) === LEGACY_KUN_SETTINGS_FILE_NAME,
        input.protectedCredentialLocators
      )
      const roundTrip = Buffer.from(serializeCleanedSettings(loaded.raw, parsed), 'utf8')
      const roundTripMatches = roundTrip.equals(loaded.raw)
      roundTrip.fill(0)
      if (!roundTripMatches) {
        throw cleanupFailure()
      }
      const canaries = new Set(fields
        .filter((field) => !field.sentinel)
        .flatMap((field) => [field.value, field.value.trim()]))
      for (const field of fields) delete field.record[field.key]
      if (containsCredentialCanary(parsed, canaries)) throw cleanupFailure()
      const serialized = serializeCleanedSettings(loaded.raw, parsed)
      if (createHash('sha256').update(serialized, 'utf8').digest('hex') !==
        input.expectedCleanedSourceSHA256) throw cleanupFailure()
      const cleanupSourcePath = loaded.sourcePath
      const cleanupPhysicalPath = loaded.physicalPath
      const sourcePhysicalIdentitySHA256 = protectedSettingsSourceIdentitySHA256(loaded)
      const cleanupSourceIdentity = {
        sourceLocator: loaded.sourceLocator,
        physicalPath: loaded.physicalPath,
        device: loaded.device,
        inode: loaded.inode
      }
      expectedSourceRaw = Buffer.from(loaded.raw)
      loaded.raw.fill(0)
      loaded = null

      this.cache = null
      try {
        await this.legacyProviderBeforeExclusiveWrite()
        await withLegacyProviderSourceWriteLock(
          cleanupPhysicalPath,
          this.legacyProviderSourceLockTestHooks,
          async () => {
            const current = await readProtectedSettingsBytesWithCompatibility(this.path)
            if (!current) throw cleanupFailure()
            try {
              if (!sameProtectedSettingsSource(current, {
                ...cleanupSourceIdentity,
                raw: expectedSourceRaw!,
                sourcePath: cleanupSourcePath
              }) || current.sourcePath !== cleanupSourcePath ||
                !current.raw.equals(expectedSourceRaw!) ||
                createHash('sha256').update(current.raw).digest('hex') !== input.sourceSHA256) {
                throw cleanupFailure()
              }
            } finally {
              current.raw.fill(0)
            }
            await this.legacyProviderCleanupAtomicWriteFile(cleanupSourcePath, serialized, { mode: 0o600 })
            const verified = await readProtectedSettingsBytesWithCompatibility(this.path)
            if (!verified) throw cleanupFailure()
            try {
              if (verified.sourcePath !== cleanupSourcePath || verified.sourceLocator !== input.sourceLocator ||
                verified.physicalPath !== cleanupPhysicalPath || verified.raw.toString('utf8') !== serialized ||
                createHash('sha256').update(verified.raw).digest('hex') !==
                  input.expectedCleanedSourceSHA256) {
                throw cleanupFailure()
              }
              const verifiedParsed = parseCleanupSource(verified.raw)
              if (collectCleanupCredentialFields(verifiedParsed, verified.sourceLocator).length > 0 ||
                containsCredentialCanary(verifiedParsed, canaries)) {
                throw cleanupFailure()
              }
              verifiedSource = Buffer.from(verified.raw)
            } finally {
              verified.raw.fill(0)
            }
          })
      } catch {
        throw cleanupFailure()
      }
      return {
        schemaVersion: 1,
        sourceLocator: input.sourceLocator,
        sourceSHA256: input.sourceSHA256,
        cleanedSourceSHA256: input.expectedCleanedSourceSHA256,
        verifiedSourceSHA256: input.expectedCleanedSourceSHA256,
        sourcePhysicalIdentitySHA256,
        verifiedSource: Uint8Array.from(verifiedSource!)
      }
    } catch {
      throw cleanupFailure()
    } finally {
      loaded?.raw.fill(0)
      expectedSourceRaw?.fill(0)
      ;(verifiedSource as Buffer | null)?.fill(0)
    }
  }

  async withLegacyProviderCredentialSourceAuthority<T>(
    input: LegacyProviderCredentialSourceAuthorityRequestV1,
    action: (authority: LegacyProviderCredentialLiveSourceAuthorityV1) => Promise<T>
  ): Promise<T> {
    if (input.schemaVersion !== 1 ||
      !LEGACY_PROVIDER_SOURCE_AUTHORITY_CHALLENGE_PATTERN.test(input.challenge) ||
      !['rollback-commit', 'finalize'].includes(input.operation) ||
      !LEGACY_PROVIDER_MIGRATION_ID_PATTERN.test(input.migrationId) ||
      !LEGACY_PROVIDER_SOURCE_LOCATOR_PATTERN.test(input.sourceLocator) ||
      !LOWER_SHA256_PATTERN.test(input.sourceSHA256) ||
      !LOWER_SHA256_PATTERN.test(input.currentSourceSHA256) ||
      !LOWER_SHA256_PATTERN.test(input.sourcePhysicalIdentitySHA256) ||
      typeof action !== 'function') {
      throw new Error(LEGACY_PROVIDER_SOURCE_AUTHORITY_FAILURE)
    }
    let loaded: ProtectedSettingsBytesSource | null = null
    let expectedRaw: Buffer | null = null
    try {
      loaded = await readProtectedSettingsBytesForLocator(this.path, input.sourceLocator)
      if (!loaded || createHash('sha256').update(loaded.raw).digest('hex') !== input.currentSourceSHA256 ||
        protectedSettingsSourceIdentitySHA256(loaded) !== input.sourcePhysicalIdentitySHA256) {
        throw new Error(LEGACY_PROVIDER_SOURCE_AUTHORITY_FAILURE)
      }
      const sourcePath = loaded.sourcePath
      const physicalPath = loaded.physicalPath
      const device = loaded.device
      const inode = loaded.inode
      const sourceIdentity = {
        sourceLocator: loaded.sourceLocator,
        physicalPath,
        device,
        inode
      }
      expectedRaw = Buffer.from(loaded.raw)
      const binding: LegacyProviderSourceAuthorityBinding = {
        schemaVersion: 1,
        challenge: input.challenge,
        operation: input.operation,
        migrationId: input.migrationId,
        sourceLocator: input.sourceLocator,
        sourceSHA256: input.sourceSHA256,
        currentSourceSHA256: input.currentSourceSHA256,
        sourcePhysicalIdentitySHA256: input.sourcePhysicalIdentitySHA256,
        sourceDevice: String(device),
        sourceInode: String(inode)
      }
      loaded.raw.fill(0)
      loaded = null
      return await withLegacyProviderSourceWriteLock(
        physicalPath,
        this.legacyProviderSourceLockTestHooks,
        async ({ token }) => {
          const current = await readProtectedSettingsBytesForLocator(this.path, input.sourceLocator)
          if (!current) throw new Error(LEGACY_PROVIDER_SOURCE_AUTHORITY_FAILURE)
          let verifiedSource: Uint8Array | undefined
          try {
            if (!sameProtectedSettingsSource(current, {
              ...sourceIdentity,
              raw: expectedRaw!,
              sourcePath
            }) || current.sourcePath !== sourcePath || !current.raw.equals(expectedRaw!) ||
              createHash('sha256').update(current.raw).digest('hex') !== input.currentSourceSHA256 ||
              protectedSettingsSourceIdentitySHA256(current) !== input.sourcePhysicalIdentitySHA256) {
              throw new Error(LEGACY_PROVIDER_SOURCE_AUTHORITY_FAILURE)
            }
            verifiedSource = Uint8Array.from(current.raw)
            const result = await action({
              schemaVersion: 1,
              sourceLocator: input.sourceLocator,
              sourceSHA256: input.sourceSHA256,
              verifiedSourceSHA256: input.currentSourceSHA256,
              sourcePhysicalIdentitySHA256: input.sourcePhysicalIdentitySHA256,
              verifiedSource,
              sourceAuthority: {
                challenge: input.challenge,
                sourcePath: physicalPath,
                lockOwnerToken: token,
                sourceDevice: String(device),
                sourceInode: String(inode)
              }
            })
            const after = await readProtectedSettingsBytesForLocator(this.path, input.sourceLocator)
            if (!after) throw new Error(LEGACY_PROVIDER_SOURCE_AUTHORITY_FAILURE)
            try {
              if (!sameProtectedSettingsSource(after, current) || !after.raw.equals(current.raw) ||
                createHash('sha256').update(after.raw).digest('hex') !== input.currentSourceSHA256) {
                throw new Error(LEGACY_PROVIDER_SOURCE_AUTHORITY_FAILURE)
              }
            } finally {
              after.raw.fill(0)
            }
            return result
          } finally {
            verifiedSource?.fill(0)
            current.raw.fill(0)
          }
        },
        binding
      )
    } catch {
      throw new Error(LEGACY_PROVIDER_SOURCE_AUTHORITY_FAILURE)
    } finally {
      loaded?.raw.fill(0)
      expectedRaw?.fill(0)
    }
  }

  async inspectLegacyImAccountCredentials(): Promise<LegacyImAccountCredentialInspectionV1 | null> {
    let loaded: ProtectedSettingsBytesSource | null = null
    try {
      loaded = await readProtectedSettingsBytesWithCompatibility(this.path)
      if (!loaded) return null
      if (loaded.sourceLocator !== `current:${SETTINGS_FILE_NAME}` ||
        loaded.raw.byteLength > LEGACY_PROVIDER_INSPECTION_MAX_SOURCE_BYTES) {
        throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
      }
      const parsed = parseCleanupSource(loaded.raw)
      const candidates = collectLegacyImAccountCredentials(parsed)
      if (candidates.length === 0) return null
      return {
        schemaVersion: 1,
        sourceLocator: loaded.sourceLocator,
        sourceSHA256: createHash('sha256').update(loaded.raw).digest('hex'),
        sourceDevice: String(loaded.device),
        sourceInode: String(loaded.inode),
        candidates
      }
    } catch (error) {
      if (error instanceof Error && error.message === LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE) throw error
      throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
    } finally {
      loaded?.raw.fill(0)
    }
  }

  async finalizeLegacyImAccountCredentialMigration(
    inspection: LegacyImAccountCredentialInspectionV1
  ): Promise<void> {
    if (inspection.schemaVersion !== 1 || inspection.sourceLocator !== `current:${SETTINGS_FILE_NAME}` ||
      !LOWER_SHA256_PATTERN.test(inspection.sourceSHA256) || inspection.candidates.length === 0 ||
      !/^(?:0|[1-9][0-9]*)$/.test(inspection.sourceDevice) || inspection.sourceDevice.length > 32 ||
      !/^(?:0|[1-9][0-9]*)$/.test(inspection.sourceInode) || inspection.sourceInode.length > 32 ||
      inspection.candidates.length > 64) {
      throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
    }
    let selected: ProtectedSettingsBytesSource | null = null
    try {
      selected = await readProtectedSettingsBytesWithCompatibility(this.path)
      if (!selected || selected.sourceLocator !== inspection.sourceLocator ||
        createHash('sha256').update(selected.raw).digest('hex') !== inspection.sourceSHA256 ||
        String(selected.device) !== inspection.sourceDevice || String(selected.inode) !== inspection.sourceInode) {
        throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
      }
      await withLegacyProviderSourceWriteLock(
        selected.physicalPath,
        this.legacyProviderSourceLockTestHooks,
        async () => {
          const current = await readProtectedSettingsBytesWithCompatibility(this.path)
          if (!current) throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
          try {
            if (!sameProtectedSettingsSource(selected!, current) || !current.raw.equals(selected!.raw) ||
              createHash('sha256').update(current.raw).digest('hex') !== inspection.sourceSHA256 ||
              String(current.device) !== inspection.sourceDevice || String(current.inode) !== inspection.sourceInode) {
              throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
            }
            const parsed = parseCleanupSource(current.raw)
            const currentCandidates = collectLegacyImAccountCredentials(parsed)
            const candidateKey = (candidate: LegacyImAccountCredentialCandidateV1): string =>
              `${candidate.channelId}|${candidate.scope.owner}|${candidate.scope.provider}|${candidate.scope.accountId}|${candidate.scope.channelId ?? ''}|${candidate.scope.purpose}|${JSON.stringify(candidate.credential)}`
            const expectedKeys = inspection.candidates.map(candidateKey).sort()
            const currentKeys = currentCandidates.map(candidateKey).sort()
            if (expectedKeys.length !== currentKeys.length ||
              expectedKeys.some((value, index) => value !== currentKeys[index])) {
              throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
            }
            const claw = parsed.claw as Record<string, unknown>
            const channels = claw.channels as Array<Record<string, unknown>>
            const byChannel = new Map(currentCandidates.map((candidate) => [candidate.channelId, candidate]))
            for (const channel of channels) {
              const channelId = typeof channel.id === 'string' ? channel.id.trim() : ''
              const candidate = byChannel.get(channelId)
              if (!candidate) continue
              channel.platformAccount = candidate.platformAccount
              delete channel.platformCredential
            }
            if (containsLegacyImPlaintext(parsed)) throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
            const serialized = serializeCleanedSettings(current.raw, parsed)
            await atomicWriteFile(current.sourcePath, serialized, { mode: 0o600 })
            const verified = await readProtectedSettingsBytesWithCompatibility(this.path)
            if (!verified) throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
            try {
              const verifiedParsed = parseCleanupSource(verified.raw)
              if (verified.sourceLocator !== inspection.sourceLocator ||
                containsLegacyImPlaintext(verifiedParsed) ||
                JSON.stringify(normalizeStoredSettings(
                  buildMergedSettings(verifiedParsed, this.homeRoot),
                  this.homeRoot
                )).includes('platformCredential')) {
                throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
              }
            } finally {
              verified.raw.fill(0)
            }
          } finally {
            current.raw.fill(0)
          }
        }
      )
      this.cache = null
      await this.load()
    } catch (error) {
      this.cache = null
      if (error instanceof Error && error.message === LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE) throw error
      throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
    } finally {
      selected?.raw.fill(0)
    }
  }

  async load(): Promise<AppSettingsV1> {
    if (this.cache) return this.cache

    let raw = ''
    let sourcePath = this.path
    try {
      const loaded = await readSettingsFileWithCompatibility(this.path)
      if (!loaded) {
        const defaults = await loadDefaultSettings(this.homeRoot)
        await this.save(defaults)
        return defaults
      }
      raw = loaded.raw
      sourcePath = loaded.sourcePath
    } catch (error) {
      throw new Error('Failed to read settings file.', { cause: error })
    }

    let parsed: Partial<AppSettingsV1>
    try {
      parsed = JSON.parse(raw) as Partial<AppSettingsV1>
      if (basename(sourcePath) === LEGACY_KUN_SETTINGS_FILE_NAME) {
        parsed = migrateLegacyKunSettingsFile(parsed)
      }
    } catch (error) {
      if (error instanceof SyntaxError) {
        const backupPath = await writeInvalidSettingsBackup(sourcePath, raw)
        const defaults = await loadDefaultSettings(this.homeRoot)
        await this.saveWithCurrentPolicy(defaults, true)
        if (backupPath) {
          publicConsoleWarn('settings', 'Invalid settings JSON was replaced with defaults.', {
            backupPath
          })
        } else {
          publicConsoleWarn(
            'settings',
            'Invalid settings JSON was replaced with defaults; backup could not be written.',
            { sourcePath }
          )
        }
        return defaults
      }
      const message = error instanceof Error ? error.message : String(error)
      throw new Error(`Failed to parse settings file ${sourcePath}: ${message}`, { cause: error })
    }

    const normalized = normalizeStoredSettings(
      buildMergedSettings(parsed, this.homeRoot),
      this.homeRoot
    )
    await ensureIsolatedRuntimeDataDirExists(normalized, this.homeRoot)
    await ensureWorkspaceRootExists(normalized.workspaceRoot, this.homeRoot)
    await ensureWriteWorkspaceRootsExist(normalized)
    await ensureClawChannelWorkspaceRootsExist(normalized, this.homeRoot)
    this.cache = normalized
    const compatibilityContainsLegacyProviderPlaintext = sourcePath !== this.path &&
      containsLegacyProviderPlaintext(parsed as ParsedSettingsRecord, `current:${SETTINGS_FILE_NAME}`)
    const containsLegacyImAccountPlaintext = containsLegacyImPlaintext(parsed as ParsedSettingsRecord)
    if (
      sourcePath !== this.path ||
      executionPolicyNeedsPersistence(parsed, normalized) ||
      scheduleSettingsNeedsMessageKeyMigration(parsed.schedule)
    ) {
      if (!compatibilityContainsLegacyProviderPlaintext && !containsLegacyImAccountPlaintext) {
        await this.save(normalized)
      }
    }
    return this.cache
  }

  async save(data: AppSettingsV1): Promise<void> {
    await this.saveWithCurrentPolicy(data, false)
  }

  private async saveWithCurrentPolicy(
    data: AppSettingsV1,
    allowInvalidCurrentReplacement: boolean
  ): Promise<void> {
    if (containsLegacyImPlaintext(data as unknown as ParsedSettingsRecord)) {
      throw new Error(LEGACY_IM_PLAINTEXT_PERSISTENCE_FAILURE)
    }
    const normalized = normalizeStoredSettings(data, this.homeRoot)
    await ensureIsolatedRuntimeDataDirExists(normalized, this.homeRoot)
    await ensureWorkspaceRootExists(normalized.workspaceRoot, this.homeRoot)
    await ensureWriteWorkspaceRootsExist(normalized)
    await ensureClawChannelWorkspaceRootsExist(normalized, this.homeRoot)
    await mkdir(dirname(this.path), { recursive: true })
    const serialized = serializeSettingsForDisk(normalized, this.homeRoot)
    const serializedRecord = JSON.parse(serialized) as unknown
    if (!isRecord(serializedRecord)) throw new Error(LEGACY_PROVIDER_PLAINTEXT_PERSISTENCE_FAILURE)
    const desiredContainsPlaintext = containsLegacyProviderPlaintext(
      serializedRecord,
      'current:analytix-settings.json'
    )
    let selected: ProtectedSettingsBytesSource | null = null
    try {
      try {
        selected = await readProtectedSettingsBytesWithCompatibility(this.path)
      } catch {
        throw new Error(LEGACY_PROVIDER_PLAINTEXT_PERSISTENCE_FAILURE)
      }
      const physicalWritePath = selected?.physicalPath ??
        join(await realpath(dirname(this.path)), basename(this.path))
      await withLegacyProviderSourceWriteLock(
        physicalWritePath,
        this.legacyProviderSourceLockTestHooks,
        async () => {
          const current = await readProtectedSettingsBytesWithCompatibility(this.path)
          try {
            if ((selected === null && current !== null) ||
              (selected !== null && (current === null || !sameProtectedSettingsSource(selected, current) ||
                current.sourcePath !== selected.sourcePath || !current.raw.equals(selected.raw)))) {
              throw new Error(LEGACY_PROVIDER_PLAINTEXT_PERSISTENCE_FAILURE)
            }
            let currentRecord: ParsedSettingsRecord | null = null
            if (current) {
              try {
                currentRecord = parseCleanupSource(current.raw)
              } catch (error) {
                if (!allowInvalidCurrentReplacement) throw error
              }
            }
            const currentContainsPlaintext = currentRecord !== null &&
              containsLegacyProviderPlaintext(currentRecord, current!.sourceLocator)
            const currentContainsLegacyImPlaintext = currentRecord !== null && containsLegacyImPlaintext(currentRecord)
            if ((currentContainsPlaintext && current!.sourceLocator !== `current:${SETTINGS_FILE_NAME}`) ||
              (currentRecord !== null && !currentContainsPlaintext && desiredContainsPlaintext) ||
              currentContainsLegacyImPlaintext) {
              throw new Error(LEGACY_PROVIDER_PLAINTEXT_PERSISTENCE_FAILURE)
            }
            await atomicWriteFile(this.path, serialized, { mode: 0o600 })
          } finally {
            current?.raw.fill(0)
          }
        })
      this.cache = normalized
    } catch (error) {
      this.cache = null
      if (error instanceof Error && error.message === LEGACY_PROVIDER_PLAINTEXT_PERSISTENCE_FAILURE) {
        throw error
      }
      throw new Error(LEGACY_PROVIDER_PLAINTEXT_PERSISTENCE_FAILURE, { cause: error })
    } finally {
      selected?.raw.fill(0)
    }
  }

  async patch(partial: AppSettingsPatch): Promise<AppSettingsV1> {
    const cur = await this.load()
    const { runtime: runtimePatch, provider: providerPatch, ...restPatch } = partial
    const next = normalizeStoredSettings({
      ...applyAnalytixRuntimePatch(cur, runtimePatch),
      ...restPatch,
      provider: mergeModelProviderSettings(cur.provider, providerPatch),
      log: { ...cur.log, ...(partial.log ?? {}) },
      notifications: { ...cur.notifications, ...(partial.notifications ?? {}) },
      appBehavior: mergeAppBehaviorSettings(cur.appBehavior, partial.appBehavior),
      keyboardShortcuts: normalizeKeyboardShortcuts({
        bindings: {
          ...cur.keyboardShortcuts.bindings,
          ...(partial.keyboardShortcuts?.bindings ?? {})
        }
      }),
      write: mergeWriteSettings(cur.write, partial.write),
      claw: mergeClawSettings(cur.claw, partial.claw),
      schedule: mergeScheduleSettings(cur.schedule, partial.schedule),
      guiUpdate: { ...cur.guiUpdate, ...(partial.guiUpdate ?? {}) }
    }, this.homeRoot)
    await this.save(next)
    return next
  }
}

export function getRuntimeBaseUrl(port: number): string {
  return `http://127.0.0.1:${port}`
}

export function devServerHintUrl(
  isPackaged: boolean,
  rendererUrl = process.env.ELECTRON_RENDERER_URL
): string | undefined {
  return isPackaged ? undefined : rendererUrl
}
