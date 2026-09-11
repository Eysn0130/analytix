import { createHash, randomBytes } from 'node:crypto'
import { constants, type BigIntStats } from 'node:fs'
import {
  chmod,
  link,
  lstat,
  mkdir,
  mkdtemp,
  open,
  readdir,
  realpath,
  rmdir,
  unlink,
  type FileHandle
} from 'node:fs/promises'
import { basename, dirname, isAbsolute, join, resolve } from 'node:path'
import type {
  ControlledArtifactReleaseEffectInputV1,
  ControlledArtifactReleaseEffectV1
} from './host'

const FILE_EFFECT_ERROR = 'controlled_artifact_file_effect_failed'
const DISPLAY_RUN_PREFIX = '.launch-'
const DISPLAY_FILE_PATTERN = /^\.artifact-[a-f0-9]{32}-[A-Za-z0-9_-]{22}\.json$/u
const STAGE_FILE_PATTERN = /^\.stage-[A-Za-z0-9_-]{22}\.tmp$/u
const SHA256_PATTERN = /^[a-f0-9]{64}$/u
const NO_FOLLOW = process.platform === 'win32' ? 0 : constants.O_NOFOLLOW
const DIRECTORY = process.platform === 'win32' ? 0 : constants.O_DIRECTORY

type FileIdentityV1 = {
  dev: bigint
  ino: bigint
}

type DirectoryAuthorityV1 = FileIdentityV1 & {
  path: string
  handle: FileHandle
}

export type ControlledArtifactViewerV1 = (path: string) => Promise<string | void>

export type PreparedControlledArtifactExportV1 = {
  release: ControlledArtifactReleaseEffectV1
}

/**
 * Freezes a host-selected, currently absent export target. The returned
 * effect exposes neither the target path nor artifact bytes to its caller.
 * Publication is a same-directory, no-replace hard-link of a fully synced
 * 0600 staging inode, so a target substitution cannot overwrite a victim.
 */
export async function prepareControlledArtifactExportV1(
  targetPath: string
): Promise<PreparedControlledArtifactExportV1> {
  try {
    const target = exactAbsolutePath(targetPath)
    const targetName = basename(target)
    if (!isSafeLeafName(targetName)) throw fixedFileEffectError()
    const parent = await openDirectoryAuthorityV1(dirname(target), false)
    try {
      await requireAbsentV1(target)
      const frozen = identityOf(parent)
      let state: 'ready' | 'releasing' | 'committed' | 'indeterminate' = 'ready'
      const release: ControlledArtifactReleaseEffectV1 = async (input) => {
        if (state !== 'ready') throw fixedFileEffectError()
        state = 'releasing'
        try {
          validateEffectInputV1(input, 'export')
          await publishNewFileV1(parent.path, frozen, targetName, input.body)
          state = 'committed'
        } catch {
          state = 'indeterminate'
          throw fixedFileEffectError()
        }
      }
      return { release }
    } finally {
      await parent.handle.close()
    }
  } catch {
    throw fixedFileEffectError()
  }
}

/**
 * Owns protected display copies for one Electron-main launch. Stale launch
 * directories are removed only when every entry has the exact host-owned
 * shape; unknown or linked content fails closed and is never traversed.
 */
export class ControlledArtifactDisplayStoreV1 {
  private closed = false
  private readonly files = new Map<string, FileIdentityV1>()

  private constructor(
    private readonly run: DirectoryAuthorityV1,
    private readonly viewer: ControlledArtifactViewerV1
  ) {}

  static async open(baseRoot: string, viewer: ControlledArtifactViewerV1): Promise<ControlledArtifactDisplayStoreV1> {
    if (typeof viewer !== 'function') throw fixedFileEffectError()
    try {
      const base = exactAbsolutePath(baseRoot)
      await mkdir(base, { recursive: true, mode: 0o700 })
      await chmod(base, 0o700)
      const authority = await openDirectoryAuthorityV1(base, true)
      try {
        await cleanupStaleDisplayRunsV1(authority)
        const runPath = await mkdtemp(join(base, DISPLAY_RUN_PREFIX))
        await chmod(runPath, 0o700)
        const run = await openDirectoryAuthorityV1(runPath, true)
        return new ControlledArtifactDisplayStoreV1(run, viewer)
      } finally {
        await authority.handle.close()
      }
    } catch {
      throw fixedFileEffectError()
    }
  }

  createReleaseEffect(): ControlledArtifactReleaseEffectV1 {
    if (this.closed) throw fixedFileEffectError()
    let state: 'ready' | 'releasing' | 'committed' | 'indeterminate' = 'ready'
    return async (input) => {
      if (this.closed || state !== 'ready') throw fixedFileEffectError()
      state = 'releasing'
      let name = ''
      try {
        validateEffectInputV1(input, 'display')
        name = displayFileNameV1(input.receipt.accessId)
        const target = join(this.run.path, name)
        await publishNewFileV1(this.run.path, identityOf(this.run), name, input.body)
        const file = await inspectRegularFileV1(target, true)
        this.files.set(name, identityOf(file))
        const viewerResult = await this.viewer(target)
        if (typeof viewerResult === 'string' && viewerResult !== '') throw fixedFileEffectError()
        state = 'committed'
      } catch {
        state = 'indeterminate'
        if (name) await this.removeOwnedFileV1(name).catch(() => undefined)
        throw fixedFileEffectError()
      }
    }
  }

  async close(): Promise<void> {
    if (this.closed) return
    this.closed = true
    let failed = false
    for (const name of [...this.files.keys()]) {
      try {
        await this.removeOwnedFileV1(name)
      } catch {
        failed = true
      }
    }
    try {
      await revalidateDirectoryV1(this.run.path, identityOf(this.run), true)
      const remaining = await readdir(this.run.path)
      if (remaining.length !== 0) throw fixedFileEffectError()
      await this.run.handle.sync()
      await this.run.handle.close()
      await rmdir(this.run.path)
    } catch {
      await this.run.handle.close().catch(() => undefined)
      failed = true
    }
    if (failed) throw fixedFileEffectError()
  }

  private async removeOwnedFileV1(name: string): Promise<void> {
    const expected = this.files.get(name)
    if (!expected || !DISPLAY_FILE_PATTERN.test(name)) throw fixedFileEffectError()
    const target = join(this.run.path, name)
    const current = await inspectRegularFileV1(target, true)
    if (!sameIdentity(current, expected)) throw fixedFileEffectError()
    await revalidateDirectoryV1(this.run.path, identityOf(this.run), true)
    await unlink(target)
    this.files.delete(name)
    await this.run.handle.sync()
  }
}

async function publishNewFileV1(
  parentPath: string,
  parentIdentity: FileIdentityV1,
  targetName: string,
  body: Buffer
): Promise<void> {
  if (!Buffer.isBuffer(body) || body.length === 0 || !isSafeLeafName(targetName)) {
    throw fixedFileEffectError()
  }
  const parent = await openDirectoryAuthorityV1(parentPath, false)
  let stageName = ''
  let stageCommitted = false
  try {
    if (!sameIdentity(parent, parentIdentity)) throw fixedFileEffectError()
    const targetPath = join(parent.path, targetName)
    await requireAbsentV1(targetPath)
    stageName = `.stage-${randomBytes(16).toString('base64url')}.tmp`
    if (!STAGE_FILE_PATTERN.test(stageName)) throw fixedFileEffectError()
    const stagePath = join(parent.path, stageName)
    const stage = await open(
      stagePath,
      constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | NO_FOLLOW,
      0o600
    )
    let stageIdentity: FileIdentityV1
    try {
      const before = await stage.stat({ bigint: true })
      requirePrivateRegularFileV1(before, 1n)
      await writeAllV1(stage, body)
      await stage.sync()
      const after = await stage.stat({ bigint: true })
      requirePrivateRegularFileV1(after, 1n)
      if (after.size !== BigInt(body.length) || !sameIdentity(before, after)) throw fixedFileEffectError()
      stageIdentity = identityOf(after)
    } finally {
      await stage.close()
    }
    const staged = await inspectRegularFileV1(stagePath, true)
    if (!sameIdentity(staged, stageIdentity)) throw fixedFileEffectError()
    await revalidateDirectoryV1(parent.path, parentIdentity, false)
    await requireAbsentV1(targetPath)
    await link(stagePath, targetPath)
    stageCommitted = true
    await parent.handle.sync()
    const linked = await inspectRegularFileV1(targetPath, true, 2n)
    if (!sameIdentity(linked, stageIdentity)) throw fixedFileEffectError()
    await verifyFileBytesV1(targetPath, body)
    await unlink(stagePath)
    stageName = ''
    const published = await inspectRegularFileV1(targetPath, true)
    if (!sameIdentity(published, stageIdentity)) throw fixedFileEffectError()
    await parent.handle.sync()
    await revalidateDirectoryV1(parent.path, parentIdentity, false)
  } catch {
    if (stageCommitted) {
      // The complete target may already be durable. Never remove it after an
      // ambiguous host-side commit; the caller must settle as indeterminate.
    }
    throw fixedFileEffectError()
  } finally {
    if (stageName) await unlink(join(parent.path, stageName)).catch(() => undefined)
    await parent.handle.close().catch(() => undefined)
  }
}

async function cleanupStaleDisplayRunsV1(base: DirectoryAuthorityV1): Promise<void> {
  for (const name of await readdir(base.path)) {
    if (!name.startsWith(DISPLAY_RUN_PREFIX)) continue
    if (!/^\.launch-[A-Za-z0-9_-]+$/u.test(name)) throw fixedFileEffectError()
    const path = join(base.path, name)
    const run = await openDirectoryAuthorityV1(path, true)
    try {
      for (const child of await readdir(path)) {
        if (!DISPLAY_FILE_PATTERN.test(child) && !STAGE_FILE_PATTERN.test(child)) {
          throw fixedFileEffectError()
        }
        await inspectRegularFileV1(join(path, child), true)
        await unlink(join(path, child))
      }
      await run.handle.sync()
    } finally {
      await run.handle.close()
    }
    await revalidateDirectoryV1(base.path, identityOf(base), true)
    await rmdir(path)
    await base.handle.sync()
  }
}

async function openDirectoryAuthorityV1(path: string, requirePrivate: boolean): Promise<DirectoryAuthorityV1> {
  const exact = exactAbsolutePath(path)
  const resolved = await realpath(exact)
  if (resolved !== exact) throw fixedFileEffectError()
  const before = await lstat(exact, { bigint: true })
  requireDirectoryV1(before, requirePrivate)
  const handle = await open(exact, constants.O_RDONLY | DIRECTORY | NO_FOLLOW)
  try {
    const opened = await handle.stat({ bigint: true })
    requireDirectoryV1(opened, requirePrivate)
    if (!sameIdentity(before, opened)) throw fixedFileEffectError()
    const after = await lstat(exact, { bigint: true })
    requireDirectoryV1(after, requirePrivate)
    if (!sameIdentity(opened, after)) throw fixedFileEffectError()
    return { path: exact, handle, ...identityOf(opened) }
  } catch {
    await handle.close().catch(() => undefined)
    throw fixedFileEffectError()
  }
}

async function revalidateDirectoryV1(
  path: string,
  expected: FileIdentityV1,
  requirePrivate: boolean
): Promise<void> {
  const current = await openDirectoryAuthorityV1(path, requirePrivate)
  try {
    if (!sameIdentity(current, expected)) throw fixedFileEffectError()
  } finally {
    await current.handle.close()
  }
}

async function inspectRegularFileV1(
  path: string,
  requirePrivate: boolean,
  expectedLinks = 1n
): Promise<BigIntStats> {
  const before = await lstat(path, { bigint: true })
  if (requirePrivate) requirePrivateRegularFileV1(before, expectedLinks)
  else requireRegularFileV1(before, expectedLinks)
  const file = await open(path, constants.O_RDONLY | NO_FOLLOW)
  try {
    const opened = await file.stat({ bigint: true })
    if (requirePrivate) requirePrivateRegularFileV1(opened, expectedLinks)
    else requireRegularFileV1(opened, expectedLinks)
    if (!sameIdentity(before, opened)) throw fixedFileEffectError()
    return opened
  } finally {
    await file.close()
  }
}

async function verifyFileBytesV1(path: string, expected: Buffer): Promise<void> {
  const file = await open(path, constants.O_RDONLY | NO_FOLLOW)
  let body = Buffer.alloc(0)
  try {
    body = await file.readFile()
    if (body.length !== expected.length || sha256(body) !== sha256(expected)) throw fixedFileEffectError()
  } finally {
    body.fill(0)
    await file.close()
  }
}

async function writeAllV1(file: FileHandle, body: Buffer): Promise<void> {
  let offset = 0
  while (offset < body.length) {
    const result = await file.write(body, offset, body.length - offset, null)
    if (!Number.isSafeInteger(result.bytesWritten) || result.bytesWritten <= 0) throw fixedFileEffectError()
    offset += result.bytesWritten
  }
}

async function requireAbsentV1(path: string): Promise<void> {
  try {
    await lstat(path)
  } catch (error) {
    if (isNodeErrorCode(error, 'ENOENT')) return
    throw fixedFileEffectError()
  }
  throw fixedFileEffectError()
}

function validateEffectInputV1(
  input: ControlledArtifactReleaseEffectInputV1,
  expectedAction: 'display' | 'export'
): void {
  if (!input || input.action !== expectedAction || input.receipt.accessAction !== expectedAction ||
    !Buffer.isBuffer(input.body) || input.body.length === 0 ||
    input.receipt.artifactByteLength !== input.body.length ||
    !SHA256_PATTERN.test(input.receipt.artifactSha256) || input.receipt.artifactSha256 !== sha256(input.body)) {
    throw fixedFileEffectError()
  }
}

function requireDirectoryV1(stats: BigIntStats, requirePrivate: boolean): void {
  if (!stats.isDirectory() || stats.isSymbolicLink() || stats.nlink < 1n || !ownedByCurrentUserV1(stats)) {
    throw fixedFileEffectError()
  }
  if (requirePrivate && (stats.mode & 0o077n) !== 0n) throw fixedFileEffectError()
}

function requireRegularFileV1(stats: BigIntStats, expectedLinks: bigint): void {
  if (!stats.isFile() || stats.isSymbolicLink() || stats.nlink !== expectedLinks || !ownedByCurrentUserV1(stats)) {
    throw fixedFileEffectError()
  }
}

function requirePrivateRegularFileV1(stats: BigIntStats, expectedLinks: bigint): void {
  requireRegularFileV1(stats, expectedLinks)
  if ((stats.mode & 0o077n) !== 0n) throw fixedFileEffectError()
}

function ownedByCurrentUserV1(stats: BigIntStats): boolean {
  return typeof process.getuid !== 'function' || stats.uid === BigInt(process.getuid())
}

function exactAbsolutePath(value: string): string {
  if (typeof value !== 'string' || value === '' || value !== value.trim() || value.includes('\0') || !isAbsolute(value)) {
    throw fixedFileEffectError()
  }
  const normalized = resolve(value)
  if (normalized !== value) throw fixedFileEffectError()
  return normalized
}

function isSafeLeafName(value: string): boolean {
  return value !== '' && value !== '.' && value !== '..' && !value.includes('/') && !value.includes('\\') &&
    Buffer.byteLength(value, 'utf8') <= 240
}

function displayFileNameV1(accessId: string): string {
  if (!SHA256_PATTERN.test(accessId)) throw fixedFileEffectError()
  const stable = sha256(Buffer.from(accessId, 'ascii')).slice(0, 32)
  const random = randomBytes(16).toString('base64url')
  const name = `.artifact-${stable}-${random}.json`
  if (!DISPLAY_FILE_PATTERN.test(name)) throw fixedFileEffectError()
  return name
}

function identityOf(value: FileIdentityV1): FileIdentityV1 {
  return { dev: value.dev, ino: value.ino }
}

function sameIdentity(left: FileIdentityV1, right: FileIdentityV1): boolean {
  return left.dev === right.dev && left.ino === right.ino
}

function sha256(value: Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function isNodeErrorCode(error: unknown, code: string): boolean {
  return error instanceof Error && 'code' in error && (error as NodeJS.ErrnoException).code === code
}

function fixedFileEffectError(): Error {
  return new Error(FILE_EFFECT_ERROR)
}
