import { redactSecretText } from '../config/secret-redaction.js'

export type LiveLocalIndexerFile = {
  path: string
  digest: string
}

export type LiveLocalIndexerFixture = {
  serverId: string
  cwd: string
  lowPriority: boolean
  backgroundStart: boolean
  failedServerIds: string[]
  attemptsPerFailedServer: number
  initialFiles: LiveLocalIndexerFile[]
  lateTombstonePath: string
  resumeFiles: LiveLocalIndexerFile[]
  secretDiagnostic: string
}

export type LiveLocalIndexerContractOutput = {
  serverId: string
  cwd: string
  lowPriority: boolean
  backgroundStart: boolean
  retryAttempts: Record<string, number>
  activePaths: string[]
  tombstoneCount: number
  restartedFromSnapshot: boolean
  secretSafeDiagnostic: string
}

export function runLiveLocalIndexerContract(
  fixture: LiveLocalIndexerFixture
): LiveLocalIndexerContractOutput {
  const indexer = new ContractLiveLocalIndexer({
    serverId: fixture.serverId,
    cwd: fixture.cwd,
    lowPriority: fixture.lowPriority,
    backgroundStart: fixture.backgroundStart
  })
  indexer.index(fixture.initialFiles)
  const resumed = ContractLiveLocalIndexer.restore(indexer.snapshot())
  resumed.tombstone(fixture.lateTombstonePath)
  resumed.index(fixture.resumeFiles)
  return {
    serverId: resumed.serverId,
    cwd: resumed.cwd,
    lowPriority: resumed.lowPriority,
    backgroundStart: resumed.backgroundStart,
    retryAttempts: Object.fromEntries(
      fixture.failedServerIds.map((serverId) => [serverId, fixture.attemptsPerFailedServer])
    ),
    activePaths: resumed.activePaths(),
    tombstoneCount: resumed.tombstoneCount(),
    restartedFromSnapshot: true,
    secretSafeDiagnostic: redactSecretText(`Authorization: ${fixture.secretDiagnostic}`)
  }
}

type ContractLiveLocalIndexerSnapshot = {
  serverId: string
  cwd: string
  lowPriority: boolean
  backgroundStart: boolean
  entries: LiveLocalIndexerFile[]
  tombstones: string[]
}

class ContractLiveLocalIndexer {
  private readonly entries = new Map<string, LiveLocalIndexerFile>()
  private readonly tombstones = new Set<string>()

  constructor(readonly metadata: {
    serverId: string
    cwd: string
    lowPriority: boolean
    backgroundStart: boolean
  }) {}

  get serverId(): string {
    return this.metadata.serverId
  }

  get cwd(): string {
    return this.metadata.cwd
  }

  get lowPriority(): boolean {
    return this.metadata.lowPriority
  }

  get backgroundStart(): boolean {
    return this.metadata.backgroundStart
  }

  index(files: readonly LiveLocalIndexerFile[]): void {
    for (const file of files) {
      this.tombstones.delete(file.path)
      this.entries.set(file.path, file)
    }
  }

  tombstone(path: string): void {
    this.entries.delete(path)
    this.tombstones.add(path)
  }

  activePaths(): string[] {
    return [...this.entries.keys()].sort()
  }

  tombstoneCount(): number {
    return this.tombstones.size
  }

  snapshot(): ContractLiveLocalIndexerSnapshot {
    return {
      ...this.metadata,
      entries: [...this.entries.values()],
      tombstones: [...this.tombstones]
    }
  }

  static restore(snapshot: ContractLiveLocalIndexerSnapshot): ContractLiveLocalIndexer {
    const indexer = new ContractLiveLocalIndexer({
      serverId: snapshot.serverId,
      cwd: snapshot.cwd,
      lowPriority: snapshot.lowPriority,
      backgroundStart: snapshot.backgroundStart
    })
    indexer.index(snapshot.entries)
    for (const tombstone of snapshot.tombstones) {
      indexer.tombstones.add(tombstone)
    }
    return indexer
  }
}
