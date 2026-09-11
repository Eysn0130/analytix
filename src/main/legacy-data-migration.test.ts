import {
  chmod,
  cp,
  lstat,
  mkdir,
  mkdtemp,
  readFile,
  realpath,
  readdir,
  rename,
  rm,
  stat,
  symlink,
  writeFile
} from 'node:fs/promises'
import {
  closeSync,
  constants,
  fsyncSync,
  lstatSync,
  mkdirSync,
  openSync,
  readFileSync,
  renameSync,
  writeFileSync
} from 'node:fs'
import { createServer, type Server } from 'node:net'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import {
  HOME_DATA_MIGRATION_MAPPINGS,
  inspectElectronSingletonArtifacts,
  issueElectronLegacySingletonAuthority,
  LegacyMigrationBlockedError,
  runLegacyAnalytixDataMigration,
  type LegacyMigrationCheckpoint
} from './legacy-data-migration'
import {
  acquireLegacyMigrationLease,
  LEGACY_MIGRATION_JOURNAL_DIR_NAME,
  LEGACY_MIGRATION_LEASE_FILE_NAME
} from './legacy-data-migration-journal'
import {
  detectPathCaseSensitivity,
  sha256Bytes,
  snapshotDirectoryStrict,
  stableJson
} from './legacy-data-migration-platform'

const tempRoots: string[] = []
const servers: Server[] = []

async function makeTempRoot(): Promise<string> {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'analytix-migration-v2-')))
  tempRoots.push(root)
  return root
}

afterEach(async () => {
  await Promise.all(servers.splice(0).map((server) => new Promise<void>((resolve) => server.close(() => resolve()))))
  while (tempRoots.length > 0) {
    const root = tempRoots.pop()
    if (root) await rm(root, { recursive: true, force: true })
  }
})

async function exists(path: string): Promise<boolean> {
  try {
    await lstat(path)
    return true
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return false
    throw error
  }
}

async function expectDirectoryNotLink(path: string): Promise<void> {
  const stats = await lstat(path)
  expect(stats.isDirectory()).toBe(true)
  expect(stats.isSymbolicLink()).toBe(false)
}

async function makeTwoOwnerFixture(): Promise<{
  root: string
  home: string
  appData: string
  legacyUserData: string
  userData: string
  legacyRuntimeData: string
  runtimeData: string
}> {
  const root = await makeTempRoot()
  const home = join(root, 'home')
  const appData = join(root, 'appData')
  const legacyUserData = join(appData, 'DeepSeek GUI')
  const userData = join(appData, 'analytix')
  const legacyRuntimeData = join(home, '.analytix', 'analytix')
  const runtimeData = join(home, '.analytix', 'data')
  await mkdir(join(legacyUserData, 'Local Storage'), { recursive: true })
  await mkdir(legacyRuntimeData, { recursive: true })
  await writeFile(join(legacyUserData, 'Local Storage', 'state.json'), '{"ok":true}', 'utf8')
  await writeFile(join(legacyRuntimeData, 'runtime.db'), 'database', 'utf8')
  await writeFile(
    join(legacyUserData, 'analytix-settings.json'),
    JSON.stringify({
      version: 1,
      runtime: {
        dataDir: '~/.analytix/analytix',
        storage: {
          sqlitePath: join(home, '.analytix', 'analytix', 'runtime.sqlite')
        }
      },
      workspaceRoot: join(home, '.analytix', 'analytix', 'workspace'),
      write: {
        workspaces: [join(home, '.analytix', 'analytix', 'write')]
      },
      claw: {
        channels: [{
          workspaceRoot: join(home, '.analytix', 'analytix', 'channel'),
          conversations: [{ workspaceRoot: join(home, '.analytix', 'analytix', 'conversation') }]
        }]
      },
      codePromptPrefix: `Do not rewrite prose: ${join(home, '.analytix', 'analytix')}`,
      extension: {
        arbitraryPath: join(home, '.analytix', 'analytix', 'must-not-change')
      }
    }),
    'utf8'
  )
  return {
    root,
    home,
    appData,
    legacyUserData,
    userData,
    legacyRuntimeData,
    runtimeData
  }
}

function runFixture(
  fixture: Awaited<ReturnType<typeof makeTwoOwnerFixture>>,
  checkpoint?: (point: LegacyMigrationCheckpoint) => void
) {
  return runLegacyAnalytixDataMigration({
    userDataPath: fixture.userData,
    homeDir: fixture.home,
    quiescenceAuthority: fixtureQuiescenceAuthority(fixture),
    testHooks: checkpoint ? { checkpoint } : undefined
  })
}

function fixtureQuiescenceAuthority(
  fixture: Awaited<ReturnType<typeof makeTwoOwnerFixture>>
) {
  const binding = (
    path: string,
    kind: 'parent-directory' | 'source-directory'
  ) => {
    const state = lstatSync(path)
    return {
      path,
      kind,
      dev: String(state.dev),
      ino: String(state.ino)
    }
  }
  const bindings = [
    binding(fixture.appData, 'parent-directory'),
    binding(dirname(fixture.legacyRuntimeData), 'parent-directory'),
    binding(fixture.legacyRuntimeData, 'source-directory'),
    binding(fixture.legacyUserData, 'source-directory')
  ].sort((left, right) =>
    `${left.kind}\0${left.path}`.localeCompare(`${right.kind}\0${right.path}`, 'en')
  )
  const primitive = process.platform === 'darwin'
    ? 'darwin-renameatx-np-rename-excl' as const
    : process.platform === 'win32'
      ? 'windows-handle-move-fail-if-exists' as const
      : 'linux-renameat2-rename-noreplace' as const
  return {
    schemaVersion: 2 as const,
    kind: 'electron-legacy-user-data-singleton' as const,
    legacyUserDataPath: fixture.legacyUserData,
    singletonArtifacts: inspectElectronSingletonArtifacts(fixture.legacyUserData),
    assertHeld: () => true,
    filesystem: {
      schemaVersion: 2 as const,
      kind: 'host-handle-relative-legacy-migration-transaction' as const,
      scopeSha256: sha256Bytes(stableJson(bindings)),
      semanticReceiptSha256: sha256Bytes('test-only-host-filesystem-receipt'),
      activationPrimitive: primitive,
      bindings,
      assertHeld: () => true,
      // This is intentionally only a unit seam. It is not product evidence for
      // the native no-replace primitive required by the authority contract.
      activateOrRecoverDirectoryNoReplace: (request: {
        stagePath: string
        targetPath: string
      }) => {
        if (existsSyncForTest(request.targetPath)) {
          if (existsSyncForTest(request.stagePath)) {
            throw new LegacyMigrationBlockedError('target_conflict')
          }
        } else {
          renameSync(request.stagePath, request.targetPath)
        }
        const parentFd = openSync(dirname(request.targetPath), constants.O_RDONLY)
        try {
          fsyncSync(parentFd)
        } finally {
          closeSync(parentFd)
        }
        const target = lstatSync(request.targetPath)
        return {
          schemaVersion: 2 as const,
          requestSha256: sha256Bytes(stableJson(request)),
          primitive,
          stageDev: String(target.dev),
          stageIno: String(target.ino),
          targetDev: String(target.dev),
          targetIno: String(target.ino),
          targetParentDev: requestTargetParentIdentity(request.targetPath).dev,
          targetParentIno: requestTargetParentIdentity(request.targetPath).ino,
          targetParentDurablySynced: true as const
        }
      }
    },
    homeData: {
      schemaVersion: 2 as const,
      kind: 'host-go-persistence-quiescence' as const,
      sourcePaths: [fixture.legacyRuntimeData],
      semanticReceiptSha256: sha256Bytes('test-only-host-semantic-receipt'),
      assertHeld: () => true
    }
  }
}

function existsSyncForTest(path: string): boolean {
  try {
    lstatSync(path)
    return true
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return false
    throw error
  }
}

function requestTargetParentIdentity(path: string): { dev: string; ino: string } {
  const state = lstatSync(dirname(path))
  return { dev: String(state.dev), ino: String(state.ino) }
}

function expectBlocked(code: LegacyMigrationBlockedError['code'], fn: () => unknown): void {
  try {
    fn()
    throw new Error('expected migration to be blocked')
  } catch (error) {
    expect(error).toBeInstanceOf(LegacyMigrationBlockedError)
    expect((error as LegacyMigrationBlockedError).code).toBe(code)
  }
}

async function seedPlannedJournal(
  fixture: Awaited<ReturnType<typeof makeTwoOwnerFixture>>
): Promise<string> {
  expectBlocked('io_error', () => runFixture(fixture, (point) => {
    if (point === 'after-plan') throw new Error('simulated crash after plan')
  }))
  return join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME, '000-planned.json')
}

describe('legacy data migration v2 activation barrier', () => {
  it('copies all owners from one fixed plan, rewrites only typed settings paths, and preserves sources', async () => {
    const fixture = await makeTwoOwnerFixture()

    const result = runFixture(fixture)

    expect(result.barrier).toMatchObject({
      schemaVersion: 2,
      ready: true,
      status: 'migrated'
    })
    expect(result.migratedOwnerIds).toEqual(['user-data', 'home-data-0'])
    expect(result.settingsRewritten).toBe(true)
    await expectDirectoryNotLink(fixture.legacyUserData)
    await expectDirectoryNotLink(fixture.legacyRuntimeData)
    await expectDirectoryNotLink(fixture.userData)
    await expectDirectoryNotLink(fixture.runtimeData)
    expect(await readFile(join(fixture.legacyRuntimeData, 'runtime.db'), 'utf8')).toBe('database')
    expect(await readFile(join(fixture.runtimeData, 'runtime.db'), 'utf8')).toBe('database')

    const sourceSettings = JSON.parse(
      await readFile(join(fixture.legacyUserData, 'analytix-settings.json'), 'utf8')
    )
    const targetSettings = JSON.parse(
      await readFile(join(fixture.userData, 'analytix-settings.json'), 'utf8')
    )
    expect(sourceSettings.runtime.dataDir).toBe('~/.analytix/analytix')
    expect(targetSettings.runtime.dataDir).toBe('~/.analytix/data')
    expect(targetSettings.runtime.storage.sqlitePath).toBe(
      join(fixture.home, '.analytix', 'data', 'runtime.sqlite')
    )
    expect(targetSettings.workspaceRoot).toBe(join(fixture.home, '.analytix', 'data', 'workspace'))
    expect(targetSettings.write.workspaces[0]).toBe(join(fixture.home, '.analytix', 'data', 'write'))
    expect(targetSettings.claw.channels[0].conversations[0].workspaceRoot).toBe(
      join(fixture.home, '.analytix', 'data', 'conversation')
    )
    expect(targetSettings.codePromptPrefix).toBe(sourceSettings.codePromptPrefix)
    expect(targetSettings.extension.arbitraryPath).toBe(sourceSettings.extension.arbitraryPath)

    const journalNames = await readdir(join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME))
    expect(journalNames).toEqual([
      '000-planned.json',
      '001-staged.json',
      '002-activation-intent.json',
      '003-owner-visible.json',
      '004-owner-visible.json',
      '005-active.json'
    ])
    const firstVisible = JSON.parse(
      await readFile(
        join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME, '003-owner-visible.json'),
        'utf8'
      )
    )
    expect(firstVisible.payload.activationReceipt).toMatchObject({
      schemaVersion: 2,
      primitive: fixtureQuiescenceAuthority(fixture).filesystem.activationPrimitive,
      targetParentDurablySynced: true
    })
    expect(firstVisible.payload.activationReceipt.requestSha256).toMatch(/^[a-f0-9]{64}$/)

    const second = runFixture(fixture)
    expect(second.barrier.status).toBe('ready-existing')
    expect(second.barrier.receiptSha256).toBe(result.barrier.receiptSha256)
    expect(await readFile(join(fixture.runtimeData, 'runtime.db'), 'utf8')).toBe('database')
  })

  it('is a mutation-free ready barrier on a fresh install', async () => {
    const root = await makeTempRoot()
    const appData = join(root, 'appData')
    const home = join(root, 'home')
    await mkdir(appData, { recursive: true })
    await mkdir(home, { recursive: true })
    const userData = join(appData, 'analytix')

    const result = runLegacyAnalytixDataMigration({ userDataPath: userData, homeDir: home })

    expect(result.barrier.status).toBe('ready-existing')
    expect(result.migratedOwnerIds).toEqual([])
    expect(await exists(userData)).toBe(false)
    expect(await exists(join(appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME))).toBe(false)
    expect(await exists(join(appData, LEGACY_MIGRATION_LEASE_FILE_NAME))).toBe(false)
  })

  it('recovers the empty private journal-root crash cut before planning', async () => {
    const fixture = await makeTwoOwnerFixture()
    await mkdir(join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME), { mode: 0o700 })

    const result = runFixture(fixture)

    expect(result.barrier.status).toBe('migrated')
    expect(await readFile(join(fixture.runtimeData, 'runtime.db'), 'utf8')).toBe('database')
  })

  it('discards an incomplete unpublished journal record and resumes the prior durable state', async () => {
    const fixture = await makeTwoOwnerFixture()
    const plannedPath = await seedPlannedJournal(fixture)
    const planned = JSON.parse(await readFile(plannedPath, 'utf8'))
    const pendingPath = join(
      fixture.appData,
      LEGACY_MIGRATION_JOURNAL_DIR_NAME,
      `.pending-001-staged-${planned.transactionId}.json`
    )
    await writeFile(pendingPath, '{"partial":', { encoding: 'utf8', mode: 0o600 })

    const result = runFixture(fixture)

    expect(result.barrier.status).toBe('recovered')
    expect(await exists(pendingPath)).toBe(false)
    expect(await readFile(join(fixture.runtimeData, 'runtime.db'), 'utf8')).toBe('database')
  })

  it('discards a syntactically valid but semantically hostile unpublished record', async () => {
    const fixture = await makeTwoOwnerFixture()
    const plannedPath = await seedPlannedJournal(fixture)
    const plannedRaw = await readFile(plannedPath, 'utf8')
    const planned = JSON.parse(plannedRaw)
    const pendingPath = join(
      fixture.appData,
      LEGACY_MIGRATION_JOURNAL_DIR_NAME,
      `.pending-001-staged-${planned.transactionId}.json`
    )
    const hostile = {
      schemaVersion: 2,
      transactionId: planned.transactionId,
      sequence: 1,
      state: 'staged',
      previousRecordSha256: sha256Bytes(plannedRaw),
      payload: {
        owners: planned.payload.owners.map((owner: { id: string; sourceSnapshot: unknown }) => ({
          id: owner.id === 'user-data' ? 'home-data-999' : owner.id,
          stageSnapshot: owner.sourceSnapshot,
          stageDev: '1',
          stageIno: '1'
        })),
        settingsRewritten: false
      }
    }
    await writeFile(pendingPath, `${stableJson(hostile)}\n`, { encoding: 'utf8', mode: 0o600 })

    const result = runFixture(fixture)

    expect(result.barrier.status).toBe('recovered')
    expect(await exists(pendingPath)).toBe(false)
    expect(await readFile(join(fixture.runtimeData, 'runtime.db'), 'utf8')).toBe('database')
  })

  it('recovers a partial unpublished planned record without trusting its bytes', async () => {
    const fixture = await makeTwoOwnerFixture()
    const journalRoot = join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME)
    await mkdir(journalRoot, { mode: 0o700 })
    await writeFile(
      join(journalRoot, '.pending-000-planned-11111111-1111-4111-8111-111111111111.json'),
      '{"schemaVersion":2',
      { encoding: 'utf8', mode: 0o600 }
    )

    const result = runFixture(fixture)

    expect(result.barrier.status).toBe('migrated')
    expect(await readFile(join(fixture.runtimeData, 'runtime.db'), 'utf8')).toBe('database')
  })

  it('blocks a concurrent product process before creating a transaction', async () => {
    const fixture = await makeTwoOwnerFixture()
    const lease = acquireLegacyMigrationLease({ userDataPath: fixture.userData })
    try {
      expectBlocked('concurrent_migration', () => runFixture(fixture))
    } finally {
      lease.release()
    }

    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(fixture.runtimeData)).toBe(false)
    expect(await exists(join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME))).toBe(false)
    await expectDirectoryNotLink(fixture.legacyUserData)
    await expectDirectoryNotLink(fixture.legacyRuntimeData)
  })

  it('requires the exact legacy Electron singleton authority before planning', async () => {
    const fixture = await makeTwoOwnerFixture()

    expectBlocked('quiescence_unavailable', () => runLegacyAnalytixDataMigration({
      userDataPath: fixture.userData,
      homeDir: fixture.home
    }))

    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(fixture.runtimeData)).toBe(false)
    expect(await exists(join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME))).toBe(false)
  })

  it('requires an independent host/Go semantic receipt before copying runtime persistence', async () => {
    const fixture = await makeTwoOwnerFixture()

    expectBlocked('quiescence_unavailable', () => runLegacyAnalytixDataMigration({
      userDataPath: fixture.userData,
      homeDir: fixture.home,
      quiescenceAuthority: {
        schemaVersion: 2,
        kind: 'electron-legacy-user-data-singleton',
        legacyUserDataPath: fixture.legacyUserData,
        singletonArtifacts: [],
        assertHeld: () => true
      }
    }))

    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(fixture.runtimeData)).toBe(false)
    expect(await exists(join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME))).toBe(false)
  })

  it('requires native handle-bound authority before lease, journal, stage, or target writes', async () => {
    const fixture = await makeTwoOwnerFixture()
    const authority = fixtureQuiescenceAuthority(fixture)
    const withoutFilesystem = { ...authority, filesystem: undefined }

    expectBlocked('host_filesystem_authority_unavailable', () =>
      runLegacyAnalytixDataMigration({
        userDataPath: fixture.userData,
        homeDir: fixture.home,
        quiescenceAuthority: withoutFilesystem
      })
    )

    expect(await exists(join(fixture.appData, LEGACY_MIGRATION_LEASE_FILE_NAME))).toBe(false)
    expect(await exists(join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME))).toBe(false)
    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(fixture.runtimeData)).toBe(false)
  })

  it('rejects an activation receipt that is not bound to the exact request', async () => {
    const fixture = await makeTwoOwnerFixture()
    const authority = fixtureQuiescenceAuthority(fixture)
    const activate = authority.filesystem.activateOrRecoverDirectoryNoReplace
    authority.filesystem.activateOrRecoverDirectoryNoReplace = (request) => ({
      ...activate(request),
      requestSha256: '0'.repeat(64)
    })

    expectBlocked('invalid_activation_receipt', () =>
      runLegacyAnalytixDataMigration({
        userDataPath: fixture.userData,
        homeDir: fixture.home,
        quiescenceAuthority: authority
      })
    )

    const recovered = runFixture(fixture)
    expect(recovered.barrier.status).toBe('recovered')
  })

  for (const crashPoint of [
    'after-plan',
    'after-staged',
    'after-activation-intent',
    'after-owner-visible:user-data',
    'after-owner-visible:home-data-0',
    'after-active'
  ] satisfies LegacyMigrationCheckpoint[]) {
    it(`recovers idempotently after the durable crash cut ${crashPoint}`, async () => {
      const fixture = await makeTwoOwnerFixture()
      let crashed = false
      expectBlocked('io_error', () => runFixture(fixture, (point) => {
        if (!crashed && point === crashPoint) {
          crashed = true
          throw new Error(`simulated crash at ${point}`)
        }
      }))

      const recovered = runFixture(fixture)
      expect(recovered.barrier.ready).toBe(true)
      expect(['recovered', 'ready-existing']).toContain(recovered.barrier.status)
      expect(await readFile(join(fixture.userData, 'Local Storage', 'state.json'), 'utf8')).toBe('{"ok":true}')
      expect(await readFile(join(fixture.runtimeData, 'runtime.db'), 'utf8')).toBe('database')
      await expectDirectoryNotLink(fixture.legacyUserData)
      await expectDirectoryNotLink(fixture.legacyRuntimeData)

      const again = runFixture(fixture)
      expect(again.barrier.status).toBe('ready-existing')
      expect(again.barrier.receiptSha256).toBe(recovered.barrier.receiptSha256)
    })
  }

  it('reclaims an authenticated stale lease but never an active one', async () => {
    const fixture = await makeTwoOwnerFixture()
    const lease = acquireLegacyMigrationLease({ userDataPath: fixture.userData })
    // Simulate restart after the recorded owner process died. The exact token
    // is validated before removal; an active owner remains fail-closed above.
    const recoveredLease = acquireLegacyMigrationLease({
      userDataPath: fixture.userData,
      isProcessAlive: () => false
    })
    recoveredLease.release()
    // The old release handle cannot delete a newer owner's lease.
    expect(() => lease.release()).toThrow()
  })

  it('recovers a dead process partial lease publication without accepting its bytes', async () => {
    const fixture = await makeTwoOwnerFixture()
    const pendingLease = join(
      fixture.appData,
      `${LEGACY_MIGRATION_LEASE_FILE_NAME}.pending-99999999-11111111-1111-4111-8111-111111111111`
    )
    await writeFile(pendingLease, '{"partial":', { encoding: 'utf8', mode: 0o600 })

    const result = runFixture(fixture)

    expect(result.barrier.ready).toBe(true)
    expect(await exists(pendingLease)).toBe(false)
  })

  it('blocks source drift after staging and does not expose either target', async () => {
    const fixture = await makeTwoOwnerFixture()
    expectBlocked('snapshot_mismatch', () => runFixture(fixture, (point) => {
      if (point === 'after-staged') {
        // A non-cooperating old release changed the preserved source after
        // snapshot/copy. Activation must stop before any target is visible.
        void 0
        writeFileSync(
          join(fixture.legacyRuntimeData, 'runtime.db'),
          'changed-after-snapshot',
          'utf8'
        )
      }
    }))
    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(fixture.runtimeData)).toBe(false)
    expect(await readFile(join(fixture.legacyRuntimeData, 'runtime.db'), 'utf8')).toBe(
      'changed-after-snapshot'
    )
  })

  it('does not overwrite a target introduced immediately before activation', async () => {
    const fixture = await makeTwoOwnerFixture()
    let injected = false
    expectBlocked('target_conflict', () => runLegacyAnalytixDataMigration({
      userDataPath: fixture.userData,
      homeDir: fixture.home,
      quiescenceAuthority: fixtureQuiescenceAuthority(fixture),
      testHooks: {
        beforeActivation: (owner) => {
          if (injected || owner.id !== 'user-data') return
          injected = true
          mkdirSync(owner.targetPath)
          writeFileSync(join(owner.targetPath, 'external.txt'), 'external')
        }
      }
    }))

    expect(await readFile(join(fixture.userData, 'external.txt'), 'utf8')).toBe('external')
    expect(await exists(fixture.runtimeData)).toBe(false)
    await expectDirectoryNotLink(fixture.legacyUserData)
  })

  it('does not replace a concurrently introduced empty target directory', async () => {
    const fixture = await makeTwoOwnerFixture()
    let targetIno = ''
    expectBlocked('target_conflict', () => runLegacyAnalytixDataMigration({
      userDataPath: fixture.userData,
      homeDir: fixture.home,
      quiescenceAuthority: fixtureQuiescenceAuthority(fixture),
      testHooks: {
        beforeActivation: (owner) => {
          if (owner.id !== 'user-data' || targetIno) return
          mkdirSync(owner.targetPath)
          targetIno = String(lstatSync(owner.targetPath).ino)
        }
      }
    }))

    expect(String((await lstat(fixture.userData)).ino)).toBe(targetIno)
    expect((await readdir(fixture.userData))).toEqual([])
    expect(await exists(fixture.runtimeData)).toBe(false)
  })

  it('rejects a same-content target with a different inode after activation crashes before receipt', async () => {
    const fixture = await makeTwoOwnerFixture()
    let crashed = false
    expectBlocked('io_error', () => runLegacyAnalytixDataMigration({
      userDataPath: fixture.userData,
      homeDir: fixture.home,
      quiescenceAuthority: fixtureQuiescenceAuthority(fixture),
      testHooks: {
        afterActivationBeforeReceipt: (owner) => {
          if (crashed || owner.id !== 'user-data') return
          crashed = true
          throw new Error('simulated crash after native activation')
        }
      }
    }))
    const displaced = join(fixture.appData, 'displaced-after-activation')
    const originalIno = String((await lstat(fixture.userData)).ino)
    await rename(fixture.userData, displaced)
    await cp(displaced, fixture.userData, { recursive: true, preserveTimestamps: true })
    expect(String((await lstat(fixture.userData)).ino)).not.toBe(originalIno)

    expectBlocked('target_conflict', () => runFixture(fixture))
    expect(await readFile(join(fixture.userData, 'Local Storage', 'state.json'), 'utf8')).toBe(
      '{"ok":true}'
    )
  })

  it('blocks a legacy source/target conflict without creating a journal', async () => {
    const fixture = await makeTwoOwnerFixture()
    await mkdir(fixture.userData, { recursive: true })
    await writeFile(join(fixture.userData, 'new.txt'), 'new', 'utf8')

    expectBlocked('target_conflict', () => runFixture(fixture))

    expect(await readFile(join(fixture.userData, 'new.txt'), 'utf8')).toBe('new')
    expect(await exists(fixture.runtimeData)).toBe(false)
    expect(await exists(join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME))).toBe(false)
  })

  it('blocks multiple legacy userData owners instead of selecting one silently', async () => {
    const fixture = await makeTwoOwnerFixture()
    await mkdir(join(fixture.appData, 'Kun'), { recursive: true })
    await writeFile(join(fixture.appData, 'Kun', 'other.txt'), 'other', 'utf8')

    expectBlocked('legacy_source_conflict', () => runFixture(fixture))

    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME))).toBe(false)
  })

  it('rejects source symlinks and leaves all source bytes untouched', async () => {
    const fixture = await makeTwoOwnerFixture()
    await symlink(
      join(fixture.legacyRuntimeData, 'runtime.db'),
      join(fixture.legacyUserData, 'linked-runtime.db')
    )

    expectBlocked('symlink_forbidden', () => runFixture(fixture))

    expect(await readFile(join(fixture.legacyRuntimeData, 'runtime.db'), 'utf8')).toBe('database')
    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(fixture.runtimeData)).toBe(false)
    expect(await exists(join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME))).toBe(false)
  })

  it('rejects an aliased ancestor before journal, stage, or target access', async () => {
    const fixture = await makeTwoOwnerFixture()
    const appDataAlias = join(fixture.root, 'appData-alias')
    await symlink(fixture.appData, appDataAlias, 'dir')

    expectBlocked('symlink_forbidden', () => runLegacyAnalytixDataMigration({
      userDataPath: join(appDataAlias, 'analytix'),
      homeDir: fixture.home,
      quiescenceAuthority: {
        ...fixtureQuiescenceAuthority(fixture),
        legacyUserDataPath: join(appDataAlias, 'DeepSeek GUI')
      }
    }))

    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(fixture.runtimeData)).toBe(false)
    expect(await exists(join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME))).toBe(false)
  })

  it('excludes only Electron singleton root artifacts and never copies their links', async () => {
    const fixture = await makeTwoOwnerFixture()
    await symlink('/tmp/analytix-singleton-cookie', join(fixture.legacyUserData, 'SingletonCookie'))
    await symlink('/tmp/analytix-singleton-lock', join(fixture.legacyUserData, 'SingletonLock'))
    await symlink('/tmp/analytix-singleton-socket', join(fixture.legacyUserData, 'SingletonSocket'))

    const result = runFixture(fixture)

    expect(result.barrier.ready).toBe(true)
    expect((await lstat(join(fixture.legacyUserData, 'SingletonLock'))).isSymbolicLink()).toBe(true)
    expect(await exists(join(fixture.userData, 'SingletonCookie'))).toBe(false)
    expect(await exists(join(fixture.userData, 'SingletonLock'))).toBe(false)
    expect(await exists(join(fixture.userData, 'SingletonSocket'))).toBe(false)
  })

  it('does not issue singleton authority from unchanged pre-existing basenames', async () => {
    const fixture = await makeTwoOwnerFixture()
    await symlink('/tmp/analytix-stale-lock', join(fixture.legacyUserData, 'SingletonLock'))
    const baseline = inspectElectronSingletonArtifacts(fixture.legacyUserData)

    expectBlocked('quiescence_unavailable', () => issueElectronLegacySingletonAuthority({
      legacyUserDataPath: fixture.legacyUserData,
      baseline,
      assertHeld: () => true
    }))
  })

  it('binds issued singleton authority to artifacts created after the baseline', async () => {
    const fixture = await makeTwoOwnerFixture()
    const baseline = inspectElectronSingletonArtifacts(fixture.legacyUserData)
    await symlink('/tmp/analytix-current-lock', join(fixture.legacyUserData, 'SingletonLock'))

    const authority = issueElectronLegacySingletonAuthority({
      legacyUserDataPath: fixture.legacyUserData,
      baseline,
      assertHeld: () => true
    })

    expect(authority.singletonArtifacts).toHaveLength(1)
    expect(authority.singletonArtifacts[0].name).toBe('SingletonLock')
  })

  it('rejects special files rather than copying an unbounded runtime endpoint', async () => {
    const fixture = await makeTwoOwnerFixture()
    const socketPath = join(fixture.legacyRuntimeData, 'runtime.sock')
    const server = createServer()
    servers.push(server)
    await new Promise<void>((resolve, reject) => {
      server.once('error', reject)
      server.listen(socketPath, resolve)
    })

    expectBlocked('special_file', () => runFixture(fixture))
    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(fixture.runtimeData)).toBe(false)
  })

  it('propagates permission failures instead of classifying the source as missing', async () => {
    const fixture = await makeTwoOwnerFixture()
    await chmod(fixture.legacyRuntimeData, 0o000)
    try {
      expectBlocked('permission_denied', () => runFixture(fixture))
    } finally {
      await chmod(fixture.legacyRuntimeData, 0o700)
    }
    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(fixture.runtimeData)).toBe(false)
  })

  it('rejects case-fold aliases in an inventory deterministically', async () => {
    const root = await makeTempRoot()
    await mkdir(join(root, 'Case'), { recursive: true })
    await mkdir(join(root, 'case'), { recursive: true }).catch(() => undefined)
    const entries = await readdir(root)
    if (entries.length === 2) {
      expectBlocked('casefold_alias', () => snapshotDirectoryStrict(root))
      return
    }

    // A case-insensitive host cannot materialize both spellings. The same
    // collision is exercised through duplicate legacy candidates resolving to
    // the one directory/inode.
    const appData = await makeTempRoot()
    await mkdir(join(appData, 'Kun'), { recursive: true })
    await mkdir(join(appData, 'home'), { recursive: true })
    expectBlocked('legacy_source_conflict', () => runLegacyAnalytixDataMigration({
      userDataPath: join(appData, 'Analytix'),
      homeDir: join(appData, 'home'),
      legacyDirNames: ['Kun', 'kun'],
      mappings: []
    }))
  })

  it('blocks unknown journal content and performs zero target mutation', async () => {
    const fixture = await makeTwoOwnerFixture()
    const journalRoot = join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME)
    await mkdir(journalRoot, { recursive: true })
    await writeFile(join(journalRoot, 'unknown.json'), '{}', 'utf8')

    expectBlocked('invalid_journal', () => runFixture(fixture))

    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(fixture.runtimeData)).toBe(false)
    expect(await readFile(join(journalRoot, 'unknown.json'), 'utf8')).toBe('{}')
  })

  it('rejects a journal snapshot path that would escape the owned stage', async () => {
    const fixture = await makeTwoOwnerFixture()
    const plannedPath = await seedPlannedJournal(fixture)
    const record = JSON.parse(await readFile(plannedPath, 'utf8'))
    const fileEntry = record.payload.owners[0].sourceSnapshot.entries.find(
      (entry: { kind: string }) => entry.kind === 'file'
    )
    fileEntry.relativePath = '../outside.txt'
    record.payload.owners[0].sourceSnapshot.manifestSha256 = sha256Bytes(
      stableJson(record.payload.owners[0].sourceSnapshot.entries)
    )
    await writeFile(plannedPath, `${stableJson(record)}\n`, 'utf8')

    expectBlocked('invalid_journal', () => runFixture(fixture))

    expect(await exists(join(fixture.appData, 'outside.txt'))).toBe(false)
    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(fixture.runtimeData)).toBe(false)
  })

  it('rebinds a recovered plan to the invocation scope before any path mutation', async () => {
    const fixture = await makeTwoOwnerFixture()
    const plannedPath = await seedPlannedJournal(fixture)
    const record = JSON.parse(await readFile(plannedPath, 'utf8'))
    const outsideTarget = join(fixture.root, 'outside-target')
    record.payload.owners[0].targetPath = outsideTarget
    record.payload.owners[0].stagePath = join(
      fixture.root,
      `.analytix-migration-v2-${record.transactionId}-user-data.stage`
    )
    await writeFile(plannedPath, `${stableJson(record)}\n`, 'utf8')

    expectBlocked('invalid_journal', () => runFixture(fixture))

    expect(await exists(outsideTarget)).toBe(false)
    expect(await exists(fixture.userData)).toBe(false)
    expect(await exists(fixture.runtimeData)).toBe(false)
  })

  it('preserves invalid settings JSON for the existing typed settings recovery path', async () => {
    const fixture = await makeTwoOwnerFixture()
    const invalid = '{not-json'
    await writeFile(join(fixture.legacyUserData, 'analytix-settings.json'), invalid, 'utf8')

    const result = runFixture(fixture)

    expect(result.settingsRewritten).toBe(false)
    expect(await readFile(join(fixture.legacyUserData, 'analytix-settings.json'), 'utf8')).toBe(invalid)
    expect(await readFile(join(fixture.userData, 'analytix-settings.json'), 'utf8')).toBe(invalid)
  })

  it('uses platform case-fold semantics only for declared typed path fields', async () => {
    const fixture = await makeTwoOwnerFixture()
    const settingsPath = join(fixture.legacyUserData, 'analytix-settings.json')
    const settings = JSON.parse(await readFile(settingsPath, 'utf8'))
    settings.runtime.dataDir = '~/.ANALYTIX/ANALYTIX'
    settings.runtime.storage.sqlitePath =
      join(fixture.home, '.analytix', 'analytix', 'runtime.sqlite').toUpperCase()
    await writeFile(settingsPath, JSON.stringify(settings), 'utf8')

    runFixture(fixture)

    const migrated = JSON.parse(
      await readFile(join(fixture.userData, 'analytix-settings.json'), 'utf8')
    )
    if (detectPathCaseSensitivity(dirname(fixture.legacyRuntimeData)) === 'insensitive') {
      expect(migrated.runtime.dataDir).toBe('~/.analytix/data')
      expect(migrated.runtime.storage.sqlitePath).toBe(
        `${join(fixture.home, '.analytix', 'data')}/RUNTIME.SQLITE`
      )
    } else {
      expect(migrated.runtime.dataDir).toBe('~/.ANALYTIX/ANALYTIX')
    }
  })

  it('binds an active receipt to the exact target root identity', async () => {
    const fixture = await makeTwoOwnerFixture()
    runFixture(fixture)
    const displaced = join(fixture.appData, 'displaced-analytix')
    await rename(fixture.userData, displaced)
    await mkdir(fixture.userData)

    expectBlocked('target_conflict', () => runFixture(fixture))

    expect(await readFile(join(displaced, 'Local Storage', 'state.json'), 'utf8')).toBe('{"ok":true}')
  })

  it('preserves regular-file and nested-directory permissions on the activated generation', async () => {
    const fixture = await makeTwoOwnerFixture()
    const sourceDir = join(fixture.legacyUserData, 'Local Storage')
    const sourceFile = join(sourceDir, 'state.json')
    await chmod(fixture.legacyUserData, 0o750)
    await chmod(sourceDir, 0o750)
    await chmod(sourceFile, 0o640)

    runFixture(fixture)

    expect((await stat(fixture.userData)).mode & 0o777).toBe(0o750)
    expect((await stat(join(fixture.userData, 'Local Storage'))).mode & 0o777).toBe(0o750)
    expect((await stat(join(fixture.userData, 'Local Storage', 'state.json'))).mode & 0o777).toBe(0o640)
  })

  it('blocks a home-only transaction until a Go/installer quiescence authority exists', async () => {
    const fixture = await makeTwoOwnerFixture()
    await rm(fixture.legacyUserData, { recursive: true })
    await mkdir(fixture.userData, { recursive: true })
    await writeFile(join(fixture.userData, 'current.txt'), 'current', 'utf8')

    expectBlocked('quiescence_unavailable', () => runLegacyAnalytixDataMigration({
      userDataPath: fixture.userData,
      homeDir: fixture.home,
      mappings: HOME_DATA_MIGRATION_MAPPINGS
    }))

    expect(await readFile(join(fixture.userData, 'current.txt'), 'utf8')).toBe('current')
    expect(await exists(fixture.runtimeData)).toBe(false)
    expect(await readFile(join(fixture.legacyRuntimeData, 'runtime.db'), 'utf8')).toBe('database')
    expect(await exists(join(fixture.appData, LEGACY_MIGRATION_JOURNAL_DIR_NAME))).toBe(false)
  })
})

describe('desktop migration composition barrier', () => {
  const mainSource = readFileSync('src/main/index.ts', 'utf8')

  it('keeps helper mode outside every migration planning/mutation call', () => {
    const helperGuard = mainSource.indexOf('if (!runningClawScheduleMcpServer) {')
    const migrationCall = mainSource.indexOf('const legacyMigration = runLegacyAnalytixDataMigration({')
    const helperBranch = mainSource.indexOf('if (runningClawScheduleMcpServer) {', migrationCall)
    expect(helperGuard).toBeGreaterThan(0)
    expect(migrationCall).toBeGreaterThan(helperGuard)
    expect(helperBranch).toBeGreaterThan(migrationCall)
  })

  it('holds the legacy singleton through activation and gates consumers on the final barrier', () => {
    const legacyLock = mainSource.indexOf('legacySingletonHeld = app.requestSingleInstanceLock()')
    const migrationCall = mainSource.indexOf('const legacyMigration = runLegacyAnalytixDataMigration({')
    const legacyRelease = mainSource.indexOf('app.releaseSingleInstanceLock()', migrationCall)
    const currentSingleton = mainSource.indexOf('const gotSingleInstanceLock =')
    const storeConsumer = mainSource.indexOf('store = new JsonSettingsStore(')
    expect(legacyLock).toBeGreaterThan(0)
    expect(migrationCall).toBeGreaterThan(legacyLock)
    expect(legacyRelease).toBeGreaterThan(migrationCall)
    expect(currentSingleton).toBeGreaterThan(legacyRelease)
    expect(storeConsumer).toBeGreaterThan(currentSingleton)
    expect(mainSource.slice(currentSingleton, currentSingleton + 200)).toContain(
      'legacyMigrationReady &&'
    )
  })
})
