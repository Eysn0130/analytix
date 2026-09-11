import { chmod, link, mkdir, mkdtemp, symlink, unlink, writeFile } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import { lstatSync, realpathSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { describe, expect, it, vi } from 'vitest'
import {
  ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1,
  resolveDesktopExternalStateBoundary
} from '../desktop-external-state-isolation'
import {
  type DarwinSecretStoreKeychainBindingV1,
  DARWIN_SECRET_STORE_KEYCHAIN_BINDING_PURPOSE_V1,
  darwinTaskKeychainPathsV1,
  resolveDarwinSecretStoreKeychainBindingV1,
  validateDarwinSecretStoreKeychainBindingDocumentV1
} from './darwin-secret-store-keychain-binding-v1'
import { encodeRuntimeStartupPrivateFrameV1 } from './runtime-startup-private-frame-v1'

vi.mock('node:fs', async (importOriginal) => {
  const actual = await importOriginal<typeof import('node:fs')>()
  return { ...actual, lstatSync: vi.fn(actual.lstatSync), realpathSync: vi.fn(actual.realpathSync) }
})

async function fixture(): Promise<{
  cacheRoot: string
  isolationRoot: string
  userDataDir: string
  dataDir: string
}> {
  const parent = await mkdtemp(join(tmpdir(), 'analytix-darwin-keychain-binding-'))
  const cacheRoot = join(parent, 'cache')
  const isolationRoot = join(cacheRoot, 'tmp', 'task-one')
  const userDataDir = join(isolationRoot, 'user-data')
  const dataDir = join(isolationRoot, 'runtime-data')
  for (const path of [cacheRoot, join(cacheRoot, 'tmp'), isolationRoot, userDataDir, dataDir]) {
    await mkdir(path, { recursive: true, mode: 0o700 })
    await chmod(path, 0o700)
  }
  return { cacheRoot, isolationRoot, userDataDir, dataDir }
}

async function preparedBindingFixture() {
  const paths = await fixture()
  const boundary = resolveDesktopExternalStateBoundary({
    ANALYTIX_DEV_CACHE_ROOT: paths.cacheRoot,
    ANALYTIX_USER_DATA_DIR: paths.userDataDir,
    ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE:
      ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1
  })
  const keychain = darwinTaskKeychainPathsV1(boundary)
  await mkdir(keychain.parentDir, { recursive: true, mode: 0o700 })
  await chmod(keychain.parentDir, 0o700)
  await writeFile(keychain.databasePath, 'synthetic-keychain-db', { mode: 0o600 })
  await chmod(keychain.databasePath, 0o600)
  return { ...paths, boundary, keychain }
}

describe('Darwin Secret Store explicit task Keychain binding v1', () => {
  it('keeps default and non-Darwin launches unbound', async () => {
    const prepared = await preparedBindingFixture()
    expect(resolveDarwinSecretStoreKeychainBindingV1({
      boundary: { isolated: false, isolationRoot: '', userDataRoot: '', stateHomeRoot: '' },
      dataDir: prepared.dataDir,
      platform: 'darwin'
    })).toBeNull()
    expect(resolveDarwinSecretStoreKeychainBindingV1({
      boundary: prepared.boundary,
      dataDir: prepared.dataDir,
      platform: 'linux'
    })).toBeNull()
  })

  it('binds one deterministic current owner-only DB under the shared isolated root', async () => {
    const prepared = await preparedBindingFixture()
    const binding = resolveDarwinSecretStoreKeychainBindingV1({
      boundary: prepared.boundary,
      dataDir: prepared.dataDir,
      platform: 'darwin'
    })
    expect(binding).toEqual({
      schemaVersion: 1,
      purpose: DARWIN_SECRET_STORE_KEYCHAIN_BINDING_PURPOSE_V1,
      externalStateMode: ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1,
      isolationRoot: prepared.isolationRoot,
      userDataDir: prepared.userDataDir,
      dataDir: prepared.dataDir,
      keychainDBPath: prepared.keychain.databasePath,
      keychainSecurityDigest: expect.stringMatching(/^[a-f0-9]{64}$/u),
      bindingDigest: expect.stringMatching(/^[a-f0-9]{64}$/u)
    })
    expect(prepared.keychain.requestedPath).toBe(prepared.keychain.databasePath)
    expect(prepared.keychain.databasePath).toBe(join(prepared.isolationRoot,
      'darwin-secret-store-keychain', 'analytix-task.keychain-db'))
    const frame = encodeRuntimeStartupPrivateFrameV1({
      darwinSecretStoreKeychainBindingV1: binding!
    })
    const body = JSON.parse(frame.subarray(8).toString('utf8')) as Record<string, unknown>
    expect(body).toEqual({
      schemaVersion: 1,
      purpose: 'analytix.runtime-startup-private-frame/v1',
      darwinSecretStoreKeychainBindingV1: binding
    })
    frame.fill(0)
  })

  it('fails closed for missing, outside, symlink, hardlink, permission, and owner mismatch', async () => {
    const cases: Array<(prepared: Awaited<ReturnType<typeof preparedBindingFixture>>) => Promise<void>> = [
      async (prepared) => { await unlink(prepared.keychain.databasePath) },
      async (prepared) => { await chmod(prepared.keychain.databasePath, 0o640) },
      async (prepared) => {
        const target = join(prepared.isolationRoot, 'target-keychain-db')
        await writeFile(target, 'target', { mode: 0o600 })
        await unlink(prepared.keychain.databasePath)
        await symlink(target, prepared.keychain.databasePath)
      },
      async (prepared) => {
        await link(prepared.keychain.databasePath, `${prepared.keychain.databasePath}.hardlink`)
      }
    ]

    for (const mutate of cases) {
      const prepared = await preparedBindingFixture()
      await mutate(prepared)
      expect(() => resolveDarwinSecretStoreKeychainBindingV1({
        boundary: prepared.boundary,
        dataDir: prepared.dataDir,
        platform: 'darwin'
      })).toThrow('Darwin Secret Store task Keychain binding is unavailable.')
    }

    const prepared = await preparedBindingFixture()
    expect(() => resolveDarwinSecretStoreKeychainBindingV1({
      boundary: prepared.boundary,
      dataDir: join(dirname(prepared.isolationRoot), 'outside-runtime-data'),
      platform: 'darwin'
    })).toThrow('Darwin Secret Store task Keychain binding is unavailable.')

    expect(() => resolveDarwinSecretStoreKeychainBindingV1({
      boundary: prepared.boundary,
      dataDir: prepared.dataDir,
      platform: 'darwin',
      ownerUID: (process.getuid?.() ?? 0) + 1
    })).toThrow('Darwin Secret Store task Keychain binding is unavailable.')
  })

  it('rejects unsafe interactive-token path characters before resolving a binding', async () => {
    const prepared = await preparedBindingFixture()
    for (const unsafe of [
      join(prepared.isolationRoot, 'runtime data'),
      join(prepared.isolationRoot, "runtime'data"),
      `${prepared.dataDir}\nquit`,
      `${prepared.dataDir}\\child`
    ]) {
      expect(() => resolveDarwinSecretStoreKeychainBindingV1({
        boundary: prepared.boundary,
        dataDir: unsafe,
        platform: 'darwin'
      })).toThrow('Darwin Secret Store task Keychain binding is unavailable.')
    }
  })

  it('rejects the full login substring and aliases before filesystem resolution, even with a valid document digest', () => {
    const root = '/private/synthetic-task'
    const document: Omit<DarwinSecretStoreKeychainBindingV1, 'bindingDigest'> = {
      schemaVersion: 1 as const, purpose: DARWIN_SECRET_STORE_KEYCHAIN_BINDING_PURPOSE_V1,
      externalStateMode: ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1,
      isolationRoot: root, userDataDir: root + '/user-data', dataDir: root + '/runtime-data',
      keychainDBPath: root + '/darwin-secret-store-keychain/analytix-task.keychain-db',
      keychainSecurityDigest: 'a'.repeat(64)
    }
    const sign = (value: typeof document) => ({ ...value,
      bindingDigest: createHash('sha256').update(JSON.stringify(value)).digest('hex') })
    expect(validateDarwinSecretStoreKeychainBindingDocumentV1(sign(document))).toEqual(sign(document))
    vi.mocked(lstatSync).mockClear(); vi.mocked(realpathSync).mockClear()
    for (const field of ['isolationRoot', 'userDataDir', 'dataDir', 'keychainDBPath'] as const) {
      for (const unsafe of ['/private/prefix-login.keychain-suffix/task', '/private/LoGiN.KeYcHaIn/task',
        root + '/child/../runtime-data', root + '//runtime-data', root + '/runtime-data\nquit']) {
        expect(() => validateDarwinSecretStoreKeychainBindingDocumentV1(sign({ ...document, [field]: unsafe })))
          .toThrow('Darwin Secret Store task Keychain binding is unavailable.')
      }
    }
    for (const unsafeRoot of ['/private/prefix-login.keychain-suffix', '/private/LoGiN.KeYcHaIn', root + '/child/..']) {
      expect(() => resolveDarwinSecretStoreKeychainBindingV1({
        boundary: { isolated: true, isolationRoot: unsafeRoot, userDataRoot: unsafeRoot + '/user-data', stateHomeRoot: '' },
        dataDir: unsafeRoot + '/runtime-data', platform: 'darwin'
      })).toThrow('Darwin Secret Store task Keychain binding is unavailable.')
    }
    expect(lstatSync).not.toHaveBeenCalled(); expect(realpathSync).not.toHaveBeenCalled()
  })

  it('rejects canonical parent aliases without creating or opening a Keychain', async () => {
    const prepared = await preparedBindingFixture()
    vi.mocked(realpathSync).mockReturnValueOnce('/private/LoGiN.KeYcHaIn-alias')
    expect(() => resolveDarwinSecretStoreKeychainBindingV1({ boundary: prepared.boundary,
      dataDir: prepared.dataDir, platform: 'darwin' }))
      .toThrow('Darwin Secret Store task Keychain binding is unavailable.')
  })
})
