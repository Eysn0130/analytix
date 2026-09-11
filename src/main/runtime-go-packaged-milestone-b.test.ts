import { spawnSync } from 'node:child_process'
import { createHash, generateKeyPairSync } from 'node:crypto'
import {
  chmodSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync
} from 'node:fs'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { afterEach, describe, expect, it } from 'vitest'

const repositoryRoot = process.cwd()
const milestoneScriptPath = join(repositoryRoot, 'scripts/runtime-go-packaged-milestone-b.mjs')
const sandboxes: string[] = []

function source(path: string): string {
  return readFileSync(join(repositoryRoot, path), 'utf8')
}

function sha256(value: string | Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function canonicalJSON(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`
  if (value && typeof value === 'object') {
    return `{${Object.entries(value as Record<string, unknown>)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSON(item)}`)
      .join(',')}}`
  }
  return JSON.stringify(value)
}

function goJSON(value: unknown): string {
  return JSON.stringify(value)
    .replaceAll('&', '\\u0026')
    .replaceAll('<', '\\u003c')
    .replaceAll('>', '\\u003e')
    .replaceAll('\u2028', '\\u2028')
    .replaceAll('\u2029', '\\u2029')
}

function goCanonicalJSON(value: unknown): string {
  return canonicalJSON(value)
    .replaceAll('&', '\\u0026')
    .replaceAll('<', '\\u003c')
    .replaceAll('>', '\\u003e')
    .replaceAll('\u2028', '\\u2028')
    .replaceAll('\u2029', '\\u2029')
}

function runtimePublicSeamObservation(mcpServers: Record<string, unknown>[]): Record<string, any> {
  return {
    health: { service: 'analytix' },
    runtimeInfo: {
      schemaVersion: 2,
      status: 'ready',
      listenerScope: 'loopback',
      port: 43210,
      insecure: false,
      storage: { configured: true, available: true },
      executionPolicy: { approvalPolicy: 'auto', sandboxMode: 'danger-full-access' }
    },
    runtimeTools: {
      schemaVersion: 2,
      providerCount: 1,
      toolContracts: { count: 4, catalogHash: 'a'.repeat(64) },
      mcpServers
    }
  }
}

function localProviderObservation(): Record<string, any> {
  const id = 'deepseek-local'
  const model = 'deepseek-v4-flash'
  return {
    providerRegistry: {
      schemaVersion: 1,
      registryRevision: '1',
      registryIncarnation: `inc_${'R'.repeat(43)}`,
      selectedProviderId: id,
      providers: [{
        id, kind: 'deepseek', endpoint: 'https://provider.example.test/v1',
        models: [model], mediaModels: [], selectedModel: model, selectedRoutes: ['primary'],
        credentialConfigured: true, credentialPurpose: 'api-key',
        revision: '1', generation: '1', incarnation: `inc_${'P'.repeat(43)}`, tombstone: false
      }]
    },
    settings: {
      workspaceRoot: '/synthetic/workspace',
      runtime: {
        providerId: id, model, endpointFormat: 'chat_completions',
        apiKeyEmpty: true, runtimeTokenEmpty: true,
        dataDir: '/synthetic/runtime', executionPolicyVersion: 2,
        approvalPolicy: 'auto', sandboxMode: 'danger-full-access'
      },
      provider: {
        activeProviderId: id, topLevelApiKeyEmpty: true, allProfilesApiKeyEmpty: true,
        profile: {
          id, baseUrl: 'https://provider.example.test/v1',
          endpointFormat: 'chat_completions', apiKeyEmpty: true, models: [model]
        }
      }
    },
    runtimeInfo: {
      provider: {
        id, model, endpointFormat: 'chat_completions', available: true,
        apiKeyConfigured: true, baseUrlConfigured: true
      }
    }
  }
}

const generalCompactionSummary =
  'Prior conversation compacted. Assistant prose, tool payloads, case facts, and private reasoning were excluded.'
const caseCompactionSummary =
  'Case-bound history compacted. No case facts, assistant prose, tool output, evidence authority, or prior compaction prose were carried into the new context epoch.'

function sealedTaskContinuation(): Record<string, any> {
  const snapshot: Record<string, any> = {
    schemaVersion: 1,
    todos: [],
    latestUserConstraints: ['preserve <T> & exact public source binding'],
    evidenceReferences: [],
    evidenceAuthority: 'unverified_for_case_facts'
  }
  snapshot.stateDigest = sha256(goJSON(snapshot))
  return snapshot
}

function publicCompactionSourceItem(
  id: string,
  turnId: string,
  text: string
): Record<string, any> {
  return {
    id,
    turnId,
    threadId: 'thread_1',
    role: 'user',
    status: 'completed',
    createdAt: '2026-07-29T01:00:00.000Z',
    finishedAt: '2026-07-29T01:00:00.000Z',
    kind: 'user_message',
    text
  }
}

function validGeneralCompactionTransition(schemaVersion: 3 | 4 = 3): {
  before: Record<string, any>
  after: Record<string, any>
  item: Record<string, any>
} {
  const sourceItems = [
    publicCompactionSourceItem('item_user_1', 'turn_1', 'first exact public request'),
    publicCompactionSourceItem('item_user_2', 'turn_2', 'second exact public request')
  ]
  const tail = [{
    id: 'turn_3',
    threadId: 'thread_1',
    status: 'completed',
    items: [publicCompactionSourceItem('item_tail_1', 'turn_3', 'retained tail one')]
  }, {
    id: 'turn_4',
    threadId: 'thread_1',
    status: 'completed',
    items: [publicCompactionSourceItem('item_tail_2', 'turn_4', 'retained tail two')]
  }]
  const before = {
    id: 'thread_1',
    turns: [{
      id: 'turn_1', threadId: 'thread_1', status: 'completed', items: [sourceItems[0]]
    }, {
      id: 'turn_2', threadId: 'thread_1', status: 'completed', items: [sourceItems[1]]
    }, ...structuredClone(tail)]
  }
  const sourceDigest = sha256(goCanonicalJSON(sourceItems))
  const item: Record<string, any> = {
    id: 'compaction_thread_1_1',
    turnId: 'turn_thread_1_compaction_1',
    threadId: 'thread_1',
    role: 'system',
    status: 'completed',
    createdAt: '2026-07-29T01:02:03.000Z',
    finishedAt: '2026-07-29T01:02:03.000Z',
    kind: 'compaction',
    summary: generalCompactionSummary,
    replacedTokens: Math.max(1, Math.floor(sourceItems.reduce(
      (total, source) => total + Buffer.byteLength(goCanonicalJSON(source)), 0
    ) / 4)),
    auto: schemaVersion === 4,
    pinnedConstraints: ['user: preserve recent turns'],
    sourceDigest,
    digestMarker: `sha256:${sourceDigest.slice(0, 12)}`,
    sourceItemIds: sourceItems.map((source) => source.id),
    schemaVersion,
    reasoningExcluded: true,
    assistantProseExcluded: true,
    toolPayloadsExcluded: true,
    caseFactsExcluded: true,
    providerHistoryProjectionVersion: schemaVersion === 4 ? 2 : 1
  }
  if (schemaVersion === 4) item.taskContinuation = sealedTaskContinuation()
  item.reasoningExclusionProof = `sha256:${sha256(goCanonicalJSON(item))}`
  const compactionTurn = {
    id: item.turnId,
    threadId: item.threadId,
    status: 'completed',
    items: [item]
  }
  return {
    before,
    after: { id: before.id, turns: [compactionTurn, ...structuredClone(tail)] },
    item
  }
}

function validCaseCompactionItem(): {
  thread: Record<string, any>
  turn: Record<string, any>
  item: Record<string, any>
} {
  const thread = { id: 'thread_1' }
  const taskContinuation = sealedTaskContinuation()
  const sourceContextDigest = 'a'.repeat(64)
  const caseCompactionBinding: Record<string, any> = {
    schemaVersion: 1,
    purpose: 'analytix.case-compaction-operation-binding/v1',
    threadIdHash: sha256(thread.id),
    sourceContextDigest,
    compactedTurnsDigest: 'b'.repeat(64),
    continuationDigest: taskContinuation.stateDigest,
    authorityTurnIds: ['turn_1', 'turn_2'],
    operationStamp: '1'
  }
  const sourceDigest = sha256(goJSON(caseCompactionBinding))
  const item: Record<string, any> = {
    id: 'compaction_thread_1_1',
    turnId: 'turn_thread_1_compaction_1',
    threadId: thread.id,
    role: 'system',
    status: 'completed',
    createdAt: '2026-07-29T01:02:03.000Z',
    finishedAt: '2026-07-29T01:02:03.000Z',
    kind: 'compaction',
    summary: caseCompactionSummary,
    replacedTokens: 2048,
    auto: false,
    pinnedConstraints: ['user: preserve recent turns'],
    sourceDigest,
    digestMarker: `sha256:${sourceDigest.slice(0, 12)}`,
    sourceItemIds: [],
    schemaVersion: 3,
    reasoningExcluded: true,
    caseFactsExcluded: true,
    caseHistoryProjectionVersion: 2,
    taskContinuation,
    sourceContextDigest,
    caseCompactionBinding
  }
  item.reasoningExclusionProof = `sha256:${sha256(goCanonicalJSON(item))}`
  const turn = {
    id: item.turnId,
    threadId: thread.id,
    status: 'completed',
    items: [item]
  }
  return { thread, turn, item }
}

function taskOwnedSandbox(): string {
  const root = process.env.TMPDIR
  if (!root || !root.startsWith('/Volumes/AnalytixCache/')) {
    throw new Error('test TMPDIR must use the trusted Analytix cache')
  }
  const sandbox = mkdtempSync(join(root, 'milestone-b-contract-test-'))
  chmodSync(sandbox, 0o700)
  sandboxes.push(sandbox)
  return sandbox
}

async function milestoneModule(): Promise<Record<string, any>> {
  return import(`${pathToFileURL(milestoneScriptPath).href}?test=${Date.now()}`)
}

const headers = [
  '交易卡号', '交易账号', '账户开户名称', '开户人证件号码', '交易时间', '交易金额',
  '交易余额', '收付标志', '交易对手账卡号', '现金标志', '对手户名', '对手身份证号',
  '对手开户银行', '摘要说明', '交易币种', '交易网点名称', '交易网点代码', '交易发生地',
  '交易是否成功', '传票号', '终端号', 'IP地址', 'MAC地址', '对手交易余额', '交易流水号',
  '日志号', '凭证种类', '凭证号', '交易柜员号', '商户名称', '商户号', '备注', '交易类型',
  '查询反馈结果原因'
]

function csvRow(input: {
  account: string
  counterparty: string
  timestamp: string
  amount: string
  direction: '进' | '出'
  sequence: string
}): string {
  const values = Array.from({ length: headers.length }, () => '')
  values[0] = input.account
  values[1] = input.account
  values[2] = '授权主体'
  values[4] = input.timestamp
  values[5] = input.amount
  values[6] = '1000.00'
  values[7] = input.direction
  values[8] = input.counterparty
  values[10] = '授权对手'
  values[12] = '测试银行'
  values[14] = 'CNY'
  values[18] = '成功'
  values[24] = input.sequence
  return values.join(',')
}

function externalCaseAcceptance(
  root: string,
  diagnosticSynthetic = false,
  paginationRowCount = 0
): {
  ownerRoot: string
  workspace: string
  contractPath: string
  provenancePath: string
  baselinePath: string
} {
  const ownerRoot = join(root, 'owner')
  const workspace = join(ownerRoot, 'case-workspace')
  mkdirSync(workspace, { recursive: true, mode: 0o700 })
  chmodSync(ownerRoot, 0o700)
  chmodSync(workspace, 0o700)
  const account = '6222021234567890123'
  const counterparty = '6217009876543210987'
  const paginationRows = Array.from({ length: paginationRowCount }, (_, index) => csvRow({
    account,
    counterparty,
    timestamp: `2025-12-10 04:${String(index).padStart(2, '0')}:00`,
    amount: '1.00',
    direction: index % 2 === 0 ? '进' : '出',
    sequence: `pagination-${String(index + 1).padStart(2, '0')}`
  }))
  const baseline = [
    headers.join(','),
    csvRow({
      account,
      counterparty,
      timestamp: '2026-01-02 03:04:05',
      amount: '100.25',
      direction: '进',
      sequence: 'baseline-1'
    }),
    csvRow({
      account,
      counterparty,
      timestamp: '2026-01-03 03:04:05',
      amount: '40.00',
      direction: '出',
      sequence: 'baseline-2'
    }),
    ...paginationRows
  ].join('\n')
  const evolved = [
    baseline,
    csvRow({
      account,
      counterparty,
      timestamp: '2026-01-04 03:04:05',
      amount: '20.00',
      direction: '进',
      sequence: 'evolved-3'
    })
  ].join('\n')
  const baselinePath = join(workspace, 'baseline.csv')
  const evolvedPath = join(workspace, 'evolved.csv')
  writeFileSync(baselinePath, baseline, { encoding: 'utf8', mode: 0o600 })
  writeFileSync(evolvedPath, evolved, { encoding: 'utf8', mode: 0o600 })
  chmodSync(baselinePath, 0o600)
  chmodSync(evolvedPath, 0o600)
  const query = {
    completeAccount: account,
    startInclusive: '2026-01-01T00:00:00.000000Z',
    endInclusive: '2026-01-31T23:59:59.000000Z',
    evidenceRowLimit: 3
  }
  const contract = {
    contract: diagnosticSynthetic
      ? 'analytix.milestone-b.synthetic-case-contract.v1'
      : 'analytix.milestone-b.real-case-contract.v1',
    case: {
      classification: diagnosticSynthetic
        ? 'isolated-synthetic-diagnostic-case'
        : 'external-preexisting-owner-isolated-real-case',
      canonicalProfile: 'canonical_direct_csv_v1'
    },
    snapshots: [
      {
        label: 'baseline',
        sourceRelativePath: 'baseline.csv',
        sourceRevision: 1,
        query,
        expected: {
          currency: 'CNY',
          minorUnitScale: 2,
          inflowMinor: '10025',
          outflowMinor: '4000',
          netMinor: '6025',
          transactionCount: 2,
          evidenceTransactionCount: 2
        }
      },
      {
        label: 'evolved',
        sourceRelativePath: 'evolved.csv',
        sourceRevision: 2,
        query,
        expected: {
          currency: 'CNY',
          minorUnitScale: 2,
          inflowMinor: '12025',
          outflowMinor: '4000',
          netMinor: '8025',
          transactionCount: 3,
          evidenceTransactionCount: 3
        }
      }
    ]
  }
  const contractPath = join(ownerRoot, 'acceptance.json')
  writeFileSync(contractPath, JSON.stringify(contract), { encoding: 'utf8', mode: 0o600 })
  chmodSync(contractPath, 0o600)
  const provenance = {
    contract: diagnosticSynthetic
      ? 'analytix.milestone-b.synthetic-case-provenance.v1'
      : 'analytix.milestone-b.real-case-provenance.v1',
    caseContractSha256: sha256(JSON.stringify(contract)),
    snapshots: [
      {
        label: 'baseline',
        sourceSha256: sha256(baseline),
        sourceByteLength: Buffer.byteLength(baseline),
        provenanceRecordDigest: sha256(canonicalJSON({
          contract: 'analytix.milestone-b.snapshot-provenance-record.v1',
          caseContractSha256: sha256(JSON.stringify(contract)),
          label: 'baseline',
          sourceSha256: sha256(baseline),
          sourceByteLength: Buffer.byteLength(baseline),
          acquiredAt: '2026-01-01T00:00:01.000Z'
        })),
        acquiredAt: '2026-01-01T00:00:01.000Z'
      },
      {
        label: 'evolved',
        sourceSha256: sha256(evolved),
        sourceByteLength: Buffer.byteLength(evolved),
        provenanceRecordDigest: sha256(canonicalJSON({
          contract: 'analytix.milestone-b.snapshot-provenance-record.v1',
          caseContractSha256: sha256(JSON.stringify(contract)),
          label: 'evolved',
          sourceSha256: sha256(evolved),
          sourceByteLength: Buffer.byteLength(evolved),
          acquiredAt: '2026-01-01T00:00:02.000Z'
        })),
        acquiredAt: '2026-01-01T00:00:02.000Z'
      }
    ]
  }
  const provenancePath = join(ownerRoot, 'provenance.json')
  writeFileSync(provenancePath, JSON.stringify(provenance), { encoding: 'utf8', mode: 0o600 })
  chmodSync(provenancePath, 0o600)
  return {
    ownerRoot,
    workspace,
    contractPath,
    provenancePath,
    baselinePath
  }
}

function managedProductAuthorityFixture(root: string): {
  ownerRoot: string
  bootstrapPath: string
  installationAuthorityKeyPath: string
  cacheRoot: string
  volumeInfo: () => Record<string, unknown>
} {
  const cacheRoot = join(root, 'cache-volume')
  const ownerRoot = join(root, 'managed-product-data')
  const manifestRoot = join(ownerRoot, 'authority-manifests')
  const profileRoot = join(ownerRoot, 'authority-credential-profiles')
  const bundleRoot = join(ownerRoot, 'authority-credential-bundles')
  for (const path of [cacheRoot, ownerRoot, manifestRoot, profileRoot, bundleRoot]) {
    mkdirSync(path, { recursive: true, mode: 0o700 })
    chmodSync(path, 0o700)
  }
  const keys = generateKeyPairSync('ed25519')
  const publicKeyDer = keys.publicKey.export({ format: 'der', type: 'spki' })
  const rawPublicKey = publicKeyDer.subarray(publicKeyDer.length - 32)
  const authorityAnchorV1 = JSON.stringify({
    schemaVersion: 1,
    installationId: '1'.repeat(64),
    authorityKeyId: sha256(rawPublicKey),
    authorityPublicKey: rawPublicKey.toString('base64url'),
    currentManifestDigest: '2'.repeat(64)
  })
  const privateJWK = keys.privateKey.export({ format: 'jwk' })
  const installationAuthorityKeyPath = join(ownerRoot, 'installation-authority-key.json')
  writeFileSync(installationAuthorityKeyPath, JSON.stringify({
    schemaVersion: 1,
    algorithm: 'Ed25519',
    keyId: sha256(rawPublicKey),
    publicKey: rawPublicKey.toString('base64url'),
    privateSeed: privateJWK.d
  }), { encoding: 'utf8', mode: 0o600 })
  chmodSync(installationAuthorityKeyPath, 0o600)
  const bootstrap = {
    schemaVersion: 1,
    purpose: 'analytix.runtime-main-owned-authority/v1',
    authorityAnchorV1,
    authorityManifestRoot: manifestRoot,
    authorityCredentialProfileRoot: profileRoot,
    authorityCredentialBundleRoot: bundleRoot
  }
  const bootstrapPath = join(ownerRoot, 'authority-bootstrap-v1.json')
  writeFileSync(bootstrapPath, JSON.stringify(bootstrap), { encoding: 'utf8', mode: 0o600 })
  chmodSync(bootstrapPath, 0o600)
  return {
    ownerRoot,
    bootstrapPath,
    installationAuthorityKeyPath,
    cacheRoot,
    volumeInfo: () => ({
      ok: true,
      blocker: '',
      mountPoint: root,
      volumeUUID: 'TEST-INTERNAL-APFS',
      deviceIdentifier: 'disk-test'
    })
  }
}

afterEach(() => {
  while (sandboxes.length > 0) {
    rmSync(sandboxes.pop()!, { recursive: true, force: true })
  }
})

describe('packaged Milestone B formal public-seam harness', () => {
  it('resolves the product-data volume through the path device before diskutil', async () => {
    const { managedProductVolumeInfo } = await milestoneModule()
    const calls: Array<{ command: string; arguments_: string[] }> = []
    const info = {
      FilesystemType: 'apfs',
      Writable: true,
      WritableVolume: true,
      GlobalPermissionsEnabled: true,
      Internal: true,
      Removable: false,
      RemovableMedia: false,
      RemovableMediaOrExternalDevice: false,
      Ejectable: false,
      APFSSnapshot: false,
      VolumeUUID: 'PRODUCT-VOLUME',
      MountPoint: '/System/Volumes/Data',
      DeviceIdentifier: 'disk3s5'
    }
    const evidence = managedProductVolumeInfo('/Users/product-owner', {
      realpathSync: (path: string) => path,
      statSync: () => ({ dev: 42 }),
      spawnSync: (command: string, arguments_: string[]) => {
        calls.push({ command, arguments_ })
        if (command === '/bin/df') {
          return {
            status: 0,
            stdout: Buffer.from(
              'Filesystem 512-blocks Used Available Capacity Mounted on\n' +
              '/dev/disk3s5 100 50 50 50% /System/Volumes/Data\n'
            )
          }
        }
        if (command === '/usr/sbin/diskutil') {
          return { status: 0, stdout: Buffer.from('plist') }
        }
        if (command === '/usr/bin/plutil') {
          return { status: 0, stdout: JSON.stringify(info) }
        }
        throw new Error(`unexpected command: ${command}`)
      }
    })
    expect(evidence).toEqual(expect.objectContaining({
      ok: true,
      mountPoint: '/System/Volumes/Data',
      volumeUUID: 'PRODUCT-VOLUME',
      deviceIdentifier: 'disk3s5'
    }))
    expect(calls[1]).toEqual({
      command: '/usr/sbin/diskutil',
      arguments_: ['info', '-plist', '/dev/disk3s5']
    })
  })

  it('routes the one canonical package command through the validation dispatcher', () => {
    expect(source('scripts/runtime-go-validation-command.mjs')).toContain(
      "'packaged-milestone-b': { script: 'runtime-go-packaged-milestone-b.mjs' }"
    )
    expect(JSON.parse(source('package.json')).scripts).toEqual(expect.objectContaining({
      'runtime:go:packaged-milestone-b':
        'node ./scripts/runtime-go-validation-command.mjs packaged-milestone-b'
    }))
  })

  it('restores the exact case binding and removes only harness-owned ordinary and protected aliases', async () => {
    const module = await milestoneModule()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'case-workspace')
    const metadata = join(workspace, '.analytix')
    const runtimeData = join(root, 'runtime-data')
    mkdirSync(metadata, { recursive: true, mode: 0o700 })
    mkdirSync(join(runtimeData, 'one'), { recursive: true, mode: 0o700 })
    mkdirSync(join(runtimeData, 'two'), { recursive: true, mode: 0o700 })
    const bindingPath = join(metadata, 'case-project.json')
    const binding = JSON.stringify({
      version: 1,
      workspaceRoot: workspace,
      caseId: 'case_formal_real',
      source: 'formal-test',
      updatedAt: '2026-08-17T00:00:00.000Z'
    })
    writeFileSync(bindingPath, binding, { encoding: 'utf8', mode: 0o600 })
    const bindingIdentity = lstatSync(bindingPath)

    const mixed = module.prepareMixedCodeWorkspace(workspace)
    const source = readFileSync(mixed.sourcePath, 'utf8')
    writeFileSync(mixed.sourcePath, source.replace('left - right', 'left + right'), {
      encoding: 'utf8', mode: 0o600
    })
    expect(module.mixedCodeWorkspaceEvidence(mixed).ok).toBe(true)
    expect(module.cleanupMixedCodeWorkspace(mixed)).toBe(true)

    const switched = module.prepareCaseSwitchAuthority(workspace)
    expect(module.switchToNegativeCase(switched)).toBe(true)
    expect(JSON.parse(readFileSync(bindingPath, 'utf8')).caseId).not.toBe('case_formal_real')
    expect(module.restoreOriginalCase(switched)).toBe(true)
    expect(readFileSync(bindingPath, 'utf8')).toBe(binding)
    expect(lstatSync(bindingPath).ino).toBe(bindingIdentity.ino)

    writeFileSync(join(runtimeData, 'one', 'a.duckdb'), 'first', { mode: 0o600 })
    writeFileSync(join(runtimeData, 'two', 'b.duckdb'), 'second', { mode: 0o600 })
    const alias = module.prepareProtectedSourceAlias(workspace, runtimeData)
    expect(lstatSync(alias.aliasPath).isSymbolicLink()).toBe(true)
    expect(module.cleanupProtectedSourceAlias(alias)).toBe(true)
    expect(() => lstatSync(alias.aliasPath)).toThrow()
  })

  it('binds a formal package, real native UI, additive Agent flow, evidence gate, and recovery', () => {
    const script = source('scripts/runtime-go-packaged-milestone-b.mjs')
    expect(source('src/preload/index.ts')).toContain(
      'runtimeRequest: flatApi.runtimeRequest'
    )
    expect(source('src/shared/analytix-api.ts')).toMatch(
      /export type AnalytixRuntimeApi = Pick<[\s\S]*\| 'runtimeRequest'/
    )
    for (const required of [
      'PACKAGED_BUILD_AUTHORITY_CONTRACT',
      'verifyPackagedBuildAuthorityArtifacts',
      "'/usr/bin/codesign'",
      'collectPackagedWorktreeSnapshotV1',
      'analytix.milestone-b.real-case-contract.v1',
      'external-preexisting-owner-isolated-real-case',
      'analytix.typed-local-display.v1',
      'typed-local-display',
      'direct-source-preview',
      'typed-local-display-rehydration',
      'analytix.runtime-main-owned-authority/v1',
      'managed-product-data-authority',
      '--product-data-owner-root',
      '--authority-bootstrap',
      'canonical_direct_csv_v1',
      "method: 'Input.dispatchMouseEvent'",
      "method: 'Input.insertText'",
      'modelPickerDisabled',
      'composer_workspace_not_ready',
      'composer_input_state_not_updated',
      "event?.kind === 'turn_failed'",
      'window.analytix',
      "api.runtime.runtimeRequest(path, 'GET')",
      'mcp__analytix_funds__analyze_account_flows',
      'EvidenceBackedAnswer',
      'analytix.final-evidence-gate/v4',
      'evidenceReceiptCount',
      'masked_metadata_only',
      'SourceUnavailableAnswer',
      "cdpComposerSubmit(firstDebugPort, '/compact'",
      'fresh-packaged-relaunch',
      'exact-thread-recovery',
      'exactCompactionRecoveryMatches',
      "expectedFunds: 'independent'",
      'fundsExplicitlyUnavailable',
      'recovered-case-query',
      'same-thread-additive-sequence',
      'preserveExplicitBlockedLane',
      'deterministic_funds_provider_independent_seam_not_started',
      'deterministic_funds_provider_independent_seam_not_completed'
    ]) {
      expect(script).toContain(required)
    }
    expect(script).toContain('incompletePublicSurfaceCount')
    expect(script).toContain('bodyTextComplete')
    expect(script).toContain('bodyHTMLComplete')
    expect(script).toContain('sseBodyComplete')
    expect(script).toContain('native_snapshot_staging_prior_success_not_cleared')
    expect(script).not.toContain('deterministic_funds_lane_interrupted')
    expect(script).not.toContain('agent_accepted_slot_lane_not_reached\', executed: true')
    expect(script).not.toContain('blockUnexecutedMilestoneBChecks')
    expect(script).not.toMatch(/\bapi\.runtime\.request\b/)
    expect(script).not.toContain('fundsServerUnavailable')
    expect(script).not.toContain("String(replay.body || '').slice")
  })

  it('keeps ordinary readiness independent from absent funds diagnostics', async () => {
    const { ordinaryPublicSeam } = await milestoneModule()

    const noDiagnostics = ordinaryPublicSeam(runtimePublicSeamObservation([]), 43210)
    expect(noDiagnostics).toEqual(expect.objectContaining({
      ok: true,
      ordinaryCatalogNonempty: true,
      fundsAvailable: false,
      fundsExplicitlyUnavailable: false,
      fundsServerDiagnosticCount: 0
    }))

    const connected = ordinaryPublicSeam(runtimePublicSeamObservation([{
      id: 'analytix_funds',
      status: 'connected',
      enabled: true,
      available: true,
      connected: true,
      toolCount: 2
    }]), 43210)
    expect(connected).toEqual(expect.objectContaining({
      ok: true,
      fundsAvailable: true,
      fundsExplicitlyUnavailable: false,
      fundsServerDiagnosticCount: 1
    }))

    const explicitHostRejection = ordinaryPublicSeam(runtimePublicSeamObservation([{
      id: 'analytix_funds',
      status: 'unavailable',
      failureCode: 'funds_installed_state_invalid',
      enabled: false,
      available: false,
      toolCount: 0
    }]), 43210)
    expect(explicitHostRejection).toEqual(expect.objectContaining({
      ok: true,
      fundsAvailable: false,
      fundsExplicitlyUnavailable: true,
      fundsServerDiagnosticCount: 1
    }))

    const unrecognizedRejection = ordinaryPublicSeam(runtimePublicSeamObservation([{
      id: 'analytix_funds',
      status: 'unavailable',
      failureCode: 'unrecognized_or_unbounded_failure',
      enabled: false,
      available: false,
      toolCount: 0
    }]), 43210)
    expect(unrecognizedRejection.fundsExplicitlyUnavailable).toBe(false)
  })

  it('settles ordinary Provider and B1 lanes independently', async () => {
    const {
      createMilestoneBPhaseLanes,
      evaluateMilestoneBPhaseDAG,
      finalizeMilestoneBPhaseLanes,
      setMilestoneBPhaseLane
    } = await milestoneModule()
    const initial = createMilestoneBPhaseLanes()
    expect(initial).toEqual(expect.objectContaining({
      ordinaryProvider: expect.objectContaining({
        phaseOwner: 'ordinary_provider',
        status: 'UNVERIFIED',
        executed: false,
        not_executed: true
      }),
      b1DirectSourcePreview: expect.objectContaining({
        phaseOwner: 'b1_direct_source_preview'
      }),
      b1DeterministicFunds: expect.objectContaining({
        phaseOwner: 'b1_deterministic_funds'
      }),
      b1AgentAcceptedSlot: expect.objectContaining({
        phaseOwner: 'b1_agent_accepted_slot'
      })
    }))

    const lanes = evaluateMilestoneBPhaseDAG({
      ordinaryProvider: {
        status: 'FAIL',
        blocker: 'ordinary_before_case_provider_error',
        executed: true
      },
      b1DirectSourcePreview: {
        status: 'PASS',
        blocker: '',
        executed: true
      },
      b1DeterministicFunds: {
        status: 'PASS',
        blocker: '',
        executed: true
      },
      b1AgentAcceptedSlot: {
        status: 'BLOCKED',
        blocker: 'provider_not_configured',
        executed: false
      }
    })
    expect(lanes.ordinaryProvider).toEqual(expect.objectContaining({
      status: 'FAIL',
      blocker: 'ordinary_before_case_provider_error',
      executed: true,
      not_executed: false
    }))
    expect(lanes.b1DirectSourcePreview).toEqual(expect.objectContaining({
      status: 'PASS',
      blocker: '',
      executed: true,
      not_executed: false
    }))
    expect(lanes.b1DeterministicFunds).toEqual(expect.objectContaining({
      status: 'PASS',
      executed: true,
      not_executed: false
    }))
    expect(lanes.b1AgentAcceptedSlot).toEqual(expect.objectContaining({
      status: 'BLOCKED',
      blocker: 'provider_not_configured',
      executed: false,
      not_executed: true
    }))

    const directOnly = createMilestoneBPhaseLanes()
    setMilestoneBPhaseLane(directOnly, 'ordinaryProvider', {
      status: 'FAIL', blocker: 'ordinary_before_case_provider_error', executed: true
    })
    setMilestoneBPhaseLane(directOnly, 'b1DirectSourcePreview', {
      status: 'PASS', blocker: '', executed: true
    })
    expect(directOnly.b1DirectSourcePreview.status).toBe('PASS')
    expect(directOnly.b1DeterministicFunds).toEqual(expect.objectContaining({
      status: 'UNVERIFIED',
      blocker: 'lane_not_started',
      not_executed: true
    }))

    const noProvider = evaluateMilestoneBPhaseDAG({
      ordinaryProvider: {
        status: 'BLOCKED',
        blocker: 'provider_not_configured',
        executed: false
      },
      b1DirectSourcePreview: {
        status: 'PASS',
        blocker: '',
        executed: true
      },
      b1DeterministicFunds: {
        status: 'BLOCKED',
        blocker: 'deterministic_funds_provider_independent_seam_not_started',
        executed: false
      },
      b1AgentAcceptedSlot: {
        status: 'BLOCKED',
        blocker: 'provider_not_configured',
        executed: false
      }
    })
    expect(noProvider).toEqual(expect.objectContaining({
      ordinaryProvider: expect.objectContaining({
        status: 'BLOCKED',
        blocker: 'provider_not_configured',
        executed: false,
        not_executed: true
      }),
      b1DirectSourcePreview: expect.objectContaining({
        status: 'PASS',
        executed: true,
        not_executed: false
      }),
      b1DeterministicFunds: expect.objectContaining({
        status: 'BLOCKED',
        blocker: 'deterministic_funds_provider_independent_seam_not_started',
        executed: false,
        not_executed: true
      }),
      b1AgentAcceptedSlot: expect.objectContaining({
        status: 'BLOCKED',
        blocker: 'provider_not_configured',
        executed: false,
        not_executed: true
      })
    }))

    const report = {
      phaseLanes: noProvider,
      checks: [
        { id: 'ordinary-before-case', status: 'BLOCKED', message: 'provider_not_configured' },
        { id: 'direct-source-preview', status: 'PASS', message: 'direct_source_preview_passed' },
        { id: 'native-snapshot-one-staging', status: 'UNVERIFIED', message: 'check was not executed' },
        { id: 'case-authority-unavailable-fails-closed', status: 'BLOCKED', message: 'provider_not_configured' }
      ]
    }
    finalizeMilestoneBPhaseLanes(report)
    expect(report.phaseLanes).toEqual(expect.objectContaining({
      ordinaryProvider: expect.objectContaining({
        status: 'BLOCKED', blocker: 'provider_not_configured', executed: false
      }),
      b1DirectSourcePreview: expect.objectContaining({
        status: 'PASS', executed: true
      }),
      b1DeterministicFunds: expect.objectContaining({
        status: 'BLOCKED',
        blocker: 'deterministic_funds_provider_independent_seam_not_started',
        executed: false
      }),
      b1AgentAcceptedSlot: expect.objectContaining({
        status: 'BLOCKED', blocker: 'provider_not_configured', executed: false
      })
    }))

    const startedDeterministic = evaluateMilestoneBPhaseDAG({
      b1DeterministicFunds: {
        status: 'UNVERIFIED',
        blocker: '',
        executed: true
      }
    })
    const startedReport = {
      phaseLanes: startedDeterministic,
      checks: [
        { id: 'native-snapshot-one-staging', status: 'PASS', message: 'snapshot_staged' },
        { id: 'funds-catalog-after-snapshot-one', status: 'UNVERIFIED', message: 'check was not executed' }
      ]
    }
    finalizeMilestoneBPhaseLanes(startedReport)
    expect(startedReport.phaseLanes.b1DeterministicFunds).toEqual(expect.objectContaining({
      status: 'UNVERIFIED',
      blocker: 'deterministic_funds_provider_independent_seam_not_completed',
      executed: true,
      not_executed: false
    }))

    const separatedReport = {
      phaseLanes: evaluateMilestoneBPhaseDAG({
        b1DeterministicFunds: { status: 'UNVERIFIED', blocker: '', executed: true },
        b1AgentAcceptedSlot: { status: 'UNVERIFIED', blocker: '', executed: true }
      }),
      checks: [
        { id: 'native-snapshot-one-staging', status: 'PASS', message: 'snapshot_one_staged' },
        { id: 'funds-catalog-after-snapshot-one', status: 'PASS', message: 'funds_catalog_ready' },
        { id: 'native-snapshot-two-staging', status: 'PASS', message: 'snapshot_two_staged' },
        { id: 'real-account-flow-one', status: 'FAIL', message: 'provider_error' }
      ]
    }
    finalizeMilestoneBPhaseLanes(separatedReport)
    expect(separatedReport.phaseLanes.b1DeterministicFunds).toEqual(expect.objectContaining({
      status: 'PASS', blocker: '', executed: true
    }))
    expect(separatedReport.phaseLanes.b1AgentAcceptedSlot).toEqual(expect.objectContaining({
      status: 'FAIL', blocker: 'provider_error', executed: true
    }))

    const guardedDeterministic = evaluateMilestoneBPhaseDAG({
      b1DeterministicFunds: {
        status: 'BLOCKED',
        blocker: 'provider_not_configured',
        executed: false
      }
    })
    expect(guardedDeterministic.b1DeterministicFunds.blocker).toBe(
      'deterministic_funds_provider_independent_seam_not_completed'
    )
  })

  it('does not require a Provider audit receipt for zero-request Direct Source Preview', async () => {
    const { directSourcePreviewAuditBindingEvidence } = await milestoneModule()
    expect(directSourcePreviewAuditBindingEvidence({
      minimumProviderRequestCount: 0,
      auditReceipt: { ok: false, blocker: 'provider_request_audit_authority_not_configured' },
      providerRequestAbsent: true
    })).toBe(true)
    expect(directSourcePreviewAuditBindingEvidence({
      minimumProviderRequestCount: 0,
      auditReceipt: { ok: false, blocker: 'provider_request_audit_authority_not_configured' },
      providerRequestAbsent: false
    })).toBe(false)
    expect(directSourcePreviewAuditBindingEvidence({
      minimumProviderRequestCount: 1,
      auditReceipt: { ok: false, providerRequestCount: 1 },
      providerRequestAbsent: true
    })).toBe(false)
    expect(directSourcePreviewAuditBindingEvidence({
      minimumProviderRequestCount: 1,
      auditReceipt: { ok: true, providerRequestCount: 1 },
      providerRequestAbsent: true
    })).toBe(true)
  })

  it('binds each ordinary milestone bash to one exact host-owned execution', async () => {
    const workspace = taskOwnedSandbox()
    const module = await milestoneModule()
    const threadId = 'thread-ordinary-binding'
    const turnId = 'turn-ordinary-binding'
    const marker = 'MILESTONE_B_ORDINARY_BEFORE_OK'
    const command = `/usr/bin/printf ${marker}`
    const observation = {
      schemaVersion: 1,
      disclosure: 'metadata_only',
      privatePayloadWithheld: true,
      threadId,
      turnId,
      toolName: 'bash',
      status: 'completed',
      workId: '1'.repeat(64),
      receiptId: '2'.repeat(64),
      dispositionId: '3'.repeat(64),
      executionGrantId: '4'.repeat(64),
      resultItemId: `item_result_${'5'.repeat(64)}`,
      resultItemDigest: '6'.repeat(64)
    }
    const raw = { status: 200, observation }
    const evidence = await module.observeHostToolExecution({
      debugPort: 9222,
      threadId,
      turnId,
      toolName: 'bash',
      workspace,
      arguments: { command },
      timeoutMs: 20_000
    }, {
      waitForDebugTarget: async () => ({
        pageTargetCount: 1,
        webSocketDebuggerUrl: 'ws://127.0.0.1/devtools/page/ordinary'
      }),
      evaluateReadonlyCdp: async (_target: string, expression: string) => {
        expect(expression).toContain('/v1/runtime/tool-executions/observe')
        expect(expression).toContain(command)
        return raw
      },
      resolveWorkspaceRealPath: (value: string) => value
    })
    const result = {
      threadId,
      evidence: {
        id: turnId,
        status: 'completed',
        toolNames: ['bash'],
        assistantText: marker,
        toolExecutionDigest: '7'.repeat(64),
        provider: { ok: true, digest: '8'.repeat(64) }
      }
    }
    expect(module.ordinaryTurnEvidence(result, marker, evidence, workspace)).toEqual(
      expect.objectContaining({
        ok: false,
        hostOwnedToolInvocationBound: true,
        hostPublicContractValidated: false,
        hostObservationDigest: expect.stringMatching(/^[a-f0-9]{64}$/u),
        hostAuthorityBindingDigest: expect.stringMatching(/^[a-f0-9]{64}$/u)
      })
    )

    const shapeOnlyForgery = module.hostToolExecutionObservationEvidence(raw, {
      threadId,
      turnId,
      toolName: 'bash'
    })
    expect(shapeOnlyForgery.ok).toBe(true)
    expect(module.ordinaryTurnEvidence(result, marker, shapeOnlyForgery, workspace).ok).toBe(false)
    expect((await module.observeHostToolExecution({
      debugPort: 9222,
      threadId,
      turnId,
      toolName: 'bash',
      workspace,
      arguments: { command: '/usr/bin/printf UNAUTHORIZED_MARKER' },
      timeoutMs: 20_000
    }, {
      waitForDebugTarget: async () => ({
        pageTargetCount: 1,
        webSocketDebuggerUrl: 'ws://127.0.0.1/devtools/page/ordinary'
      }),
      evaluateReadonlyCdp: async () => raw,
      resolveWorkspaceRealPath: (value: string) => value
    })).ok).toBe(false)
  })

  it('classifies submitted-turn timeouts without exposing turn content', async () => {
    const {
      publicTurnFailureProjection,
      publicTurnFailureReasonCode,
      submittedTurnObservationReady,
      submittedTurnTimeoutBlocker
    } = await milestoneModule()
    expect(submittedTurnTimeoutBlocker(null, 0, 'PRIVATE_MARKER'))
      .toBe('packaged_turn_not_created')
    expect(submittedTurnTimeoutBlocker({ thread: { turns: [{ status: 'running' }] } }, 0))
      .toBe('packaged_turn_completion_timeout')
    expect(submittedTurnTimeoutBlocker({ thread: { turns: [{ status: 'failed' }] } }, 0))
      .toBe('packaged_turn_failed')
    const providerFailureTurn = {
      status: 'failed',
      items: [{
        kind: 'error',
        status: 'failed',
        message: 'PRIVATE_PROVIDER_ERROR',
        details: { reasonCode: 'provider_authentication_failed', raw: 'PRIVATE_BODY' }
      }]
    }
    expect(publicTurnFailureReasonCode(providerFailureTurn))
      .toBe('provider_authentication_failed')
    expect(publicTurnFailureProjection({
      ...providerFailureTurn,
      id: 'turn-safe-provider-failure',
      threadId: 'thread-safe-provider-failure'
    }, {
      turnFailures: [{
        kind: 'pipeline_stage',
        stage: 'provider_error',
        seq: 7,
        threadId: 'thread-safe-provider-failure',
        turnId: 'turn-safe-provider-failure',
        reasonCode: 'provider_authentication_failed',
        rawBody: 'PRIVATE_PROVIDER_BODY'
      }]
    })).toEqual({
      turnStatus: 'failed',
      reasonCode: 'provider_authentication_failed',
      itemCodes: ['provider_authentication_failed'],
      events: [{
        kind: 'pipeline_stage',
        stage: 'provider_error',
        seq: 7,
        reasonCode: 'provider_authentication_failed'
      }]
    })
    expect(publicTurnFailureProjection({
      id: 'turn-untrusted-event-kind',
      threadId: 'thread-untrusted-event-kind',
      status: 'failed',
      items: []
    }, {
      turnFailures: [{
        kind: 'usage',
        threadId: 'thread-untrusted-event-kind',
        turnId: 'turn-untrusted-event-kind',
        reasonCode: 'provider_authentication_failed'
      }]
    })).toEqual({
      turnStatus: 'failed',
      reasonCode: '',
      itemCodes: [],
      events: []
    })
    expect(submittedTurnTimeoutBlocker({ thread: { turns: [providerFailureTurn] } }, 0))
      .toBe('packaged_turn_failed_provider_authentication_failed')
    expect(publicTurnFailureReasonCode({
      ...providerFailureTurn,
      items: [{
        kind: 'error', status: 'failed', code: 'provider_reasoning_markup_invalid'
      }]
    })).toBe('provider_reasoning_markup_invalid')
    expect(publicTurnFailureReasonCode({
      ...providerFailureTurn,
      items: [{
        kind: 'error', status: 'failed', details: { reasonCode: 'provider_invented' }
      }]
    })).toBe('')
    expect(publicTurnFailureReasonCode({
      id: 'turn-safe-provider-failure',
      threadId: 'thread-safe-provider-failure',
      status: 'failed',
      items: []
    }, {
      turnFailures: [{
        kind: 'pipeline_stage',
        stage: 'provider_error',
        threadId: 'thread-safe-provider-failure',
        turnId: 'turn-safe-provider-failure',
        reasonCode: 'provider_insufficient_balance'
      }]
    })).toBe('provider_insufficient_balance')
    expect(publicTurnFailureReasonCode({
      id: 'turn-safe-tool-failure',
      threadId: 'thread-safe-tool-failure',
      status: 'failed',
      items: []
    }, {
      turnFailures: [{
        kind: 'turn_failed',
        threadId: 'thread-safe-tool-failure',
        turnId: 'turn-safe-tool-failure',
        reasonCode: 'tool_not_advertised'
      }]
    })).toBe('tool_not_advertised')
    expect(submittedTurnTimeoutBlocker({ thread: { turns: [{ status: 'aborted' }] } }, 0))
      .toBe('packaged_turn_aborted')
    expect(submittedTurnTimeoutBlocker({
      thread: {
        turns: [{
          status: 'completed',
          items: [{ kind: 'assistant_text', status: 'completed', text: 'safe answer' }]
        }]
      }
    }, 0, 'PRIVATE_MARKER')).toBe('packaged_turn_required_marker_not_observed')
    expect(submittedTurnObservationReady(
      { status: 'failed' },
      { assistantText: '' },
      'PRIVATE_MARKER'
    )).toBe(true)
    expect(submittedTurnObservationReady(
      { status: 'completed' },
      { assistantText: 'safe answer' },
      'PRIVATE_MARKER'
    )).toBe(false)
    expect(submittedTurnObservationReady(
      { status: 'completed' },
      { assistantText: 'safe PRIVATE_MARKER' },
      'PRIVATE_MARKER'
    )).toBe(true)
  })

  it('treats absent optional diagnostic resources as already clean', async () => {
    const { cleanupOptionalHarnessResource } = await milestoneModule()
    let cleanupCalls = 0
    expect(cleanupOptionalHarnessResource(null, () => {
      cleanupCalls += 1
      return false
    })).toBe(true)
    expect(cleanupCalls).toBe(0)
    expect(cleanupOptionalHarnessResource({ id: 'created' }, () => {
      cleanupCalls += 1
      return true
    })).toBe(true)
    expect(cleanupOptionalHarnessResource({ id: 'created' }, () => {
      cleanupCalls += 1
      return false
    })).toBe(false)
    expect(cleanupCalls).toBe(2)
  })

  it('observes the typed local Registry without activating an account API', async () => {
    const module = await milestoneModule()
    const fixture = localProviderObservation()
    const requests: unknown[] = []
    const observation = await new Function('window', 'document',
      `return ${module.readonlyObservationExpression('/synthetic/workspace')}`
    )({ analytix: {
      account: { getSnapshot: () => { throw new Error('account API must stay lazy') } },
      providerRegistry: { request: async (request: unknown) => {
        requests.push(request)
        return fixture.providerRegistry
      } },
      settings: { getSettings: async () => ({
        ...fixture.settings,
        provider: { activeProviderId: 'deepseek-local', providers: [{
          ...fixture.settings.provider.profile, apiKey: ''
        }] }
      }) }
    } }, { title: 'synthetic', body: null, querySelector: () => null })
    expect(requests).toEqual([{ schemaVersion: 1, operation: 'list' }])
    expect(observation.providerRegistry).toEqual(fixture.providerRegistry)
    expect(observation.settings.provider.profile.id).toBe('deepseek-local')
    expect(observation).not.toHaveProperty('account')
  })

  it('keeps local lanes ready without Provider credentials and requires a real empty-to-configured setup transition', async () => {
    const module = await milestoneModule()
    const fixture = localProviderObservation()
    const seam = runtimePublicSeamObservation([])
    const ready = {
      ...fixture, ...seam,
      runtimeInfo: { ...seam.runtimeInfo, ...fixture.runtimeInfo },
      apiPresent: true, rendererTargetCount: 1, composerPresent: true, primaryButtonPresent: true
    }
    const empty = { ...ready, providerRegistry: {
      schemaVersion: 1, registryRevision: '0',
      registryIncarnation: fixture.providerRegistry.registryIncarnation, providers: []
    } }
    const options = {
      debugPort: 1, workspace: '/synthetic/workspace', runtimePort: 43210,
      runtimeDataDir: '/synthetic/runtime', timeoutMs: 1000, providerRequired: false
    }
    let reads = 0
    const independent = await module.waitForLocalProviderWorkbench(options, {
      observeRenderer: async () => { reads++; return empty }, sleep: async () => {}
    })
    expect(reads).toBe(1)
    expect(independent).toMatchObject({ ok: true, normalLocalProviderSetupObserved: false,
      provider: { ok: false } })
    reads = 0
    let setupCalls = 0
    const configured = await module.waitForLocalProviderWorkbench({
      ...options, onFreshRegistry: async () => { setupCalls++ }
    }, {
      observeRenderer: async () => ++reads === 1 ? empty : ready, sleep: async () => {}
    })
    expect(configured).toMatchObject({ ok: true, normalLocalProviderSetupObserved: true,
      provider: { ok: true, credentialAuthorityBound: true } })
    expect(setupCalls).toBe(1)
    const preconfigured = await module.waitForLocalProviderWorkbench(options, {
      observeRenderer: async () => ready, sleep: async () => {}
    })
    expect(preconfigured.normalLocalProviderSetupObserved).toBe(false)
    expect(module.milestoneBLocalProvider({ ...fixture, providerRegistry: null }).ok).toBe(false)
    expect(module.milestoneBLocalProvider({
      ...fixture, runtimeInfo: { provider: { ...fixture.runtimeInfo.provider, id: 'analytix-hub' } }
    }).ok).toBe(false)
    expect(module.milestoneBLocalProvider({
      ...fixture, providerRegistry: { ...fixture.providerRegistry, providers: [{
        ...fixture.providerRegistry.providers[0], credentialConfigured: false
      }] }
    }).ok).toBe(false)
  })

  it('R130-F1 binds B1 credential scans to entry authority across both isolated roots', async () => {
    const module = await milestoneModule()
    const scanner = await import(pathToFileURL(
      join(repositoryRoot, 'scripts/lib/local-provider-credential-scan.mjs')
    ).href)
    const root = taskOwnedSandbox()
    const sandboxRoot = join(root, 'sandbox')
    const productRunRoot = join(root, 'product')
    const runtimeDataDir = join(productRunRoot, 'runtime')
    const store = join(runtimeDataDir, 'private', 'provider-secrets')
    for (const directory of [sandboxRoot, store]) mkdirSync(directory, { recursive: true, mode: 0o700 })
    writeFileSync(join(store, 'credentials.v1.json'), '{"ciphertext":"synthetic-cipher"}', { mode: 0o600 })
    const settingsPath = join(productRunRoot, 'settings.json')
    writeFileSync(settingsPath, '{}', { mode: 0o600 })
    writeFileSync(join(sandboxRoot, 'log.txt'), 'synthetic clean log', { mode: 0o600 })
    const entryObservation = localProviderObservation()
    const provider = module.milestoneBLocalProvider(entryObservation)
    const secret = Buffer.from('synthetic-b1-entry-only-Secret!')
    const source = await scanner.captureLocalCredentialEntry({
      runId: sandboxRoot, entryAttemptId: 'entry-b1', providerId: provider.id,
      entryMethod: 'visible-computer-use', credential: secret
    }, async () => ({ completed: true, providerRegistry: entryObservation.providerRegistry }))
    const input = {
      source, runId: sandboxRoot, entryAttemptId: 'entry-b1', provider, runtimeDataDir,
      sandboxRoot, productRunRoot, settingsPath, reportSnapshot: { synthetic: true }
    }
    try {
      const accepted = await module.scanMilestoneBLocalCredentials(input)
      expect(accepted).toMatchObject({
        ok: true, status: 'passed', sourceBound: true, protectedStoreOwnerPrivate: true,
        expectedSecretCount: 1, sourceSecretCount: 1, uniqueSecretCount: 1,
        automatedCredentialEntryUsed: true, findingCount: 0
      })
      expect(accepted.scannedFileCount).toBeGreaterThan(0)
      const stale = await module.scanMilestoneBLocalCredentials({ ...input, entryAttemptId: 'different-entry' })
      expect(stale).toMatchObject({ ok: false, status: 'blocked', sourceBound: false })
      const replaced = structuredClone(entryObservation)
      replaced.providerRegistry.registryRevision = '2'
      replaced.providerRegistry.providers[0].revision = '2'
      replaced.providerRegistry.providers[0].generation = '2'
      const replacementProvider = module.milestoneBLocalProvider(replaced)
      expect(replacementProvider.ok).toBe(true)
      expect(scanner.readLocalCredentialScanSource(source, { ...input, provider: replacementProvider }).ok).toBe(false)
      writeFileSync(join(sandboxRoot, 'log.txt'), 'synthetic-B1-only-K2-leaks', { mode: 0o600 })
      expect(await module.scanMilestoneBLocalCredentials({ ...input, provider: replacementProvider }))
        .toMatchObject({ ok: false, status: 'blocked', sourceBound: false, scannedFileCount: 0 })
      writeFileSync(join(sandboxRoot, 'log.txt'), secret.toString('base64'), { mode: 0o600 })
      const leaked = await module.scanMilestoneBLocalCredentials(input)
      expect(leaked).toMatchObject({ ok: false, status: 'failed', findingCount: 1 })
      expect(JSON.stringify(leaked)).not.toContain(secret.toString('base64'))
    } finally {
      scanner.disposeLocalCredentialScanSource(source)
      secret.fill(0)
    }
  })

  it('binds provider completion, accepts renderer v2 funds evidence, and rejects drift', async () => {
    const module = await milestoneModule()
    const threadId = 'thread-funds-final'
    const turn: Record<string, any> = {
      id: 'turn-funds-final',
      threadId,
      status: 'completed',
      items: [{
        id: 'item-user',
        threadId,
        turnId: 'turn-funds-final',
        kind: 'user_message',
        status: 'completed'
      }, {
        id: 'item-call',
        threadId,
        turnId: 'turn-funds-final',
        kind: 'tool_call',
        status: 'completed',
        callId: 'flow-call',
        toolName: 'mcp__analytix_funds__analyze_account_flows',
        toolKind: 'tool_call'
      }, {
        id: 'item-result',
        threadId,
        turnId: 'turn-funds-final',
        kind: 'tool_result',
        status: 'completed',
        callId: 'flow-call',
        toolName: 'mcp__analytix_funds__analyze_account_flows',
        toolKind: 'tool_call',
        isError: false,
        output: {
          status: 'completed',
          projectionKind: 'case_source_status',
          messageKey: 'case_source_private'
        }
      }]
    }
    const model = 'deepseek-v4-flash'
    const attempt = {
      kind: 'usage',
      seq: 40,
      threadId,
      turnId: turn.id,
      model,
      usageFinalStatus: 'completed',
      promptTokens: 100,
      completionTokens: 50,
      totalTokens: 150,
      turns: 1,
      providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
      providerAttemptTelemetryValid: true,
      providerLogicalCallCount: 2,
      providerAttemptCount: 2,
      providerAttemptStatuses: {
        succeeded: 2,
        failed: 0,
        cancelled: 0,
        timedOut: 0,
        streamAborted: 0
      }
    }
    const observation: Record<string, any> = {
      ...localProviderObservation(),
      thread: { id: threadId, providerId: 'deepseek-local', model },
      providerAttempts: [attempt],
      providerTerminals: [{
        kind: 'turn_completed',
        seq: 41,
        threadId,
        turnId: turn.id,
        status: 'completed'
      }]
    }
    expect(module.providerReceiptForTurn(observation, turn)).toEqual(
      expect.objectContaining({ ok: true, digest: expect.stringMatching(/^[a-f0-9]{64}$/u) })
    )
    expect(module.providerReceiptForTurn({
      ...observation,
      providerAttempts: [{ ...attempt, totalTokens: 149 }]
    }, turn).ok).toBe(false)
    expect(module.providerReceiptForTurn({
      ...observation,
      providerAttempts: [attempt, { ...attempt }]
    }, turn).ok).toBe(false)
    expect(module.providerReceiptForTurn({
      ...observation,
      providerTerminals: [
        observation.providerTerminals[0],
        { ...observation.providerTerminals[0] }
      ]
    }, turn).ok).toBe(false)

    const text = '主体 〔账户槽位 1〕 的账户 〔账户槽位 1〕 资金汇总：流入 10025 CNY，流出 4000 CNY，有符号净额 6025 CNY，交易笔数 2。（限定范围：实体 〔账户槽位 1〕；账户 〔账户槽位 1〕；方向 in、out；时间 2026-01-01T00:00:00Z 至 2026-01-31T23:59:59Z；粒度 aggregate；已核验来源 1 项）'
    const acceptedFinal = {
      schemaVersion: 5,
      authorityPurpose: 'analytix.case-final/v1',
      authorityAlgorithm: 'Ed25519',
      authorityKeyId: 'b'.repeat(64),
      authorityPublicKey: 'fixture-public-key',
      threadId,
      turnId: turn.id,
      envelopeDigest: 'c'.repeat(64),
      contextDigest: 'd'.repeat(64),
      variant: 'EvidenceBackedAnswer',
      terminalReason: 'success',
      renderedTextSha256: sha256(text),
      registrySequence: 4,
      registryStateDigest: 'e'.repeat(64),
      rendererVersion: 'analytix.host-final-renderer/v2',
      finalGateVersion: 'analytix.final-evidence-gate/v4',
      verifierVersion: 'analytix.claim-verifier-policy/v1',
      publicationSnapshotProofDigest: 'f'.repeat(64),
      privateRecordDigest: '1'.repeat(64),
      publicViewDigest: '2'.repeat(64),
      acceptedAt: '2026-01-04T00:00:00.000Z',
      authoritySignature: 'fixture-signature',
      recordDigest: 'a'.repeat(64),
      contextEpoch: 3,
      datasetSnapshotId: `dsv2_${'b'.repeat(64)}`,
      factFinalWitnessAdmission: {
        schemaVersion: 1,
        purpose: 'analytix.fact-final-witness-admission/v1',
        contextDigest: 'd'.repeat(64),
        datasetSnapshotId: `dsv2_${'b'.repeat(64)}`,
        renderedTextSha256: sha256(text),
        publicationSnapshotProofDigest: 'f'.repeat(64),
        registrySequence: 4,
        registryStateDigest: 'e'.repeat(64),
        sourceManifestHash: '3'.repeat(64),
        envelopeDigest: 'c'.repeat(64),
        evidenceReceiptIdsDigest: '4'.repeat(64),
        admissionDigest: '5'.repeat(64),
        evidenceReceiptCount: 1
      },
      publicView: {
        schemaVersion: 2,
        publicationState: 'accepted',
        envelopeDigest: 'c'.repeat(64),
        contextDigest: 'd'.repeat(64),
        contextEpoch: 3,
        datasetSnapshotId: `dsv2_${'b'.repeat(64)}`,
        variant: 'EvidenceBackedAnswer',
        terminalReason: 'success',
        blockerCode: '',
        coverageStatus: 'complete',
        checkedScopeDigest: '6'.repeat(64),
        missingScopeCount: 0,
        claimCount: 3,
        claimTypes: ['amount', 'count'],
        receiptMetadata: {
          projection: 'masked_metadata_only',
          count: 1,
          setDigest: '7'.repeat(64),
          citations: [{ handle: `cite_${'8'.repeat(64)}`, label: 'evidence-1' }]
        },
        noHitWording: '',
        envelopeIssuedAt: '2026-01-04T00:00:00.000Z',
        acceptedAt: '2026-01-04T00:00:00.000Z'
      }
    }
    turn.acceptedFinal = acceptedFinal
    turn.items.push({
      id: 'item-assistant',
      threadId,
      turnId: turn.id,
      kind: 'assistant_text',
      status: 'completed',
      text
    })
    observation.thread.turns = [turn]
    const result = module.bindPackagedRendererTestTurn(observation, turn)
    const snapshot = {
      expected: {
        currency: 'CNY',
        inflowMinor: '10025',
        outflowMinor: '4000',
        netMinor: '6025',
        transactionCount: 2
      }
    }
    expect(module.exactFundsFinalEvidence(result, snapshot)).toEqual(expect.objectContaining({
      ok: true,
      hostPublicContractValidated: true,
      claimTypes: ['amount', 'count']
    }))
    expect(module.exactFundsFinalEvidence({
      ...result,
      evidence: { ...result.evidence, assistantText: `${text}\nmodel-added material` }
    }, snapshot).ok).toBe(false)
    const legacyText = [
      '主体 〔账户槽位 1〕 的资金分析如下：',
      '流入金额（最小货币单位）为 10025 CNY',
      '流出金额（最小货币单位）为 4000 CNY',
      '资金净额（最小货币单位）为 6025 CNY',
      '记录数量为 2'
    ].join('\n')
    expect(module.exactFundsFinalEvidence({
      ...result,
      evidence: {
        ...result.evidence,
        assistantText: legacyText,
        assistantTextDigest: sha256(legacyText),
        acceptedFinal: {
          ...result.evidence.acceptedFinal,
          renderedTextSha256: sha256(legacyText),
          factFinalWitnessAdmission: {
            ...result.evidence.acceptedFinal.factFinalWitnessAdmission,
            renderedTextSha256: sha256(legacyText)
          }
        }
      }
    }, snapshot).ok).toBe(false)
    expect(module.exactFundsFinalEvidence({
      ...result,
      evidence: {
        ...result.evidence,
        acceptedFinal: {
          ...result.evidence.acceptedFinal,
          rendererVersion: 'analytix.host-final-renderer/v1'
        }
      }
    }, snapshot).ok).toBe(false)
    expect(module.exactFundsFinalEvidence({
      ...result,
      evidence: {
        ...result.evidence,
        acceptedFinal: {
          ...result.evidence.acceptedFinal,
          publicView: {
            ...result.evidence.acceptedFinal.publicView,
            claimTypes: ['amount', 'count', 'transaction']
          }
        }
      }
    }, snapshot).ok).toBe(false)
    expect(module.exactFundsFinalEvidence({
      ...result,
      evidence: {
        ...result.evidence,
        acceptedFinal: {
          ...result.evidence.acceptedFinal,
          factFinalWitnessAdmission: {
            ...result.evidence.acceptedFinal.factFinalWitnessAdmission,
            renderedTextSha256: sha256('evidence-drift')
          }
        }
      }
    }, snapshot).ok).toBe(false)

    const fundsOnlyTurn = {
      ...turn,
      items: turn.items.slice(0, 3)
    }
    expect(module.fundsOnlyTurnLifecycleEvidence(fundsOnlyTurn)).toEqual(
      expect.objectContaining({ ok: true, toolExecutionCount: 1 })
    )
    expect(module.fundsOnlyTurnLifecycleEvidence({
      ...fundsOnlyTurn,
      items: [...fundsOnlyTurn.items, {
        id: 'item-extra-call',
        threadId,
        turnId: turn.id,
        kind: 'tool_call',
        status: 'failed',
        callId: 'extra-call',
        toolName: 'bash',
        toolKind: 'tool_call'
      }]
    }).ok).toBe(false)
  })

  it('proves typed local display is the only allowed source-exact sink', async () => {
    const module = await milestoneModule()
    const completeAccount = '6222021234567890123'
    const raw = {
      full: {
        kind: 'accepted_slot_display',
        mode: 'full',
        values: [completeAccount],
        slotCount: 1
      },
      masked: {
        kind: 'accepted_slot_display',
        mode: 'masked',
        values: ['*************0123'],
        slotCount: 1
      },
      forbiddenGenericProviderChannels: {
        thread: { turns: [] },
        summary: { status: 'ready' },
        sseBody: '',
        bodyText: 'safe ordinary surface',
        bodyHTML: '<main>safe ordinary surface</main>'
      },
      acceptedFinalDigestSha256: true,
      claimReceiptRefsOnly: true,
      invalidatedAfterAuthorityChange: true
    }
    const evidence = module.typedLocalDisplayEvidence(
      raw,
      [completeAccount],
      completeAccount
    )
    expect(evidence).toEqual(expect.objectContaining({
      ok: true,
      fullModeExactValue: true,
      maskedModeLocalOnly: true,
      exactValueOnlyInTypedSink: true,
      acceptedSlotBindingVerified: true,
      forbiddenGenericProviderChannelsZero: true,
      invalidatedAfterAuthorityChange: true
    }))
    expect(JSON.stringify(evidence)).not.toContain(completeAccount)
    expect(module.typedLocalDisplayEvidence({
      ...raw,
      forbiddenGenericProviderChannels: {
        ...raw.forbiddenGenericProviderChannels,
        bodyText: completeAccount
      }
    }, [completeAccount], completeAccount).ok).toBe(false)
    expect(module.typedLocalDisplayEvidence({
      ...raw,
      invalidatedAfterAuthorityChange: false
    }, [completeAccount], completeAccount).ok).toBe(true)
    const scan = module.publicPIIScanEvidence([{
      thread: { turns: [] },
      summary: { status: 'ready' },
      sseBody: '',
      bodyText: 'safe ordinary surface',
      bodyHTML: '<main>safe ordinary surface</main>',
      bodyTextComplete: true,
      bodyHTMLComplete: true,
      sseBodyComplete: true,
      typedLocalDisplay: { fullValue: completeAccount }
    }], [completeAccount], [taskOwnedSandbox()])
    expect(scan).toEqual(expect.objectContaining({
      ok: true,
      allowedTypedLocalSink: expect.objectContaining({
        excludedFromForbiddenScan: true,
        sourceExactFieldsAllowed: true
      }),
      forbiddenGenericProviderChannels: expect.objectContaining({
        completePIIFindingCount: 0
      })
    }))
  })

  it('proves Direct Source Preview pagination stays outside Agent, Provider, MCP, and final state', async () => {
    const module = await milestoneModule()
    const completeAccount = '6222021234567890123'
    const firstRows = Array.from({ length: 25 }, (_, index) => index)
    const secondRows = Array.from({ length: 13 }, (_, index) => 25 + index)
    const evidence = module.directSourcePreviewEvidence({
      first: {
        kind: 'direct_source_preview', mode: 'full', values: [completeAccount],
        rowIndices: firstRows, rowCount: firstRows.length,
        previous: { disabled: true }, next: { disabled: false }
      },
      next: {
        kind: 'direct_source_preview', mode: 'full', values: ['safe'],
        rowIndices: secondRows, rowCount: secondRows.length,
        previous: { disabled: false }, next: { disabled: true }
      },
      masked: {
        kind: 'direct_source_preview', mode: 'masked', values: ['*************0123'],
        rowIndices: firstRows, rowCount: firstRows.length
      },
      providerRequestAbsent: true,
      agentTurnAbsent: true,
      mcpCallAbsent: true,
      claimReceiptFinalGateAbsent: true
    }, [completeAccount], completeAccount, 38)
    expect(evidence).toEqual(expect.objectContaining({
      ok: true,
      fullModeExactValue: true,
      maskedModeLocalOnly: true,
      paginationObserved: true,
      providerRequestAbsent: true,
      agentTurnAbsent: true,
      mcpCallAbsent: true,
      claimReceiptFinalGateAbsent: true
    }))
    expect(JSON.stringify(evidence)).not.toContain(completeAccount)
    expect(module.directSourcePreviewEvidence({
      first: {
        kind: 'direct_source_preview', mode: 'full', values: [completeAccount],
        rowIndices: firstRows, previous: { disabled: true }, next: { disabled: false }
      },
      next: {
        kind: 'direct_source_preview', mode: 'full', values: ['safe'],
        rowIndices: secondRows.slice(0, -1), previous: { disabled: false },
        next: { disabled: true }
      },
      masked: {
        kind: 'direct_source_preview', mode: 'masked', values: ['masked'],
        rowIndices: firstRows
      },
      providerRequestAbsent: true,
      agentTurnAbsent: true,
      mcpCallAbsent: true,
      claimReceiptFinalGateAbsent: true
    }, [completeAccount], completeAccount, 38).ok).toBe(false)

    const observation = {
      thread: {
        turns: [{
          acceptedFinal: { recordDigest: 'a'.repeat(64) },
          items: []
        }]
      },
      providerAttempts: []
    }
    const before = module.directSourcePreviewBoundaryProjection(observation)
    const after = module.directSourcePreviewBoundaryProjection(structuredClone(observation))
    expect(module.directSourcePreviewBoundaryUnchanged(before, after)).toBe(true)
    expect(module.directSourcePreviewBoundaryUnchanged(before, {
      ...after,
      turnCount: after.turnCount + 1
    })).toBe(false)
  })

  it('requires one exact visible renderer row with all safe expected fragments', async () => {
    const module = await milestoneModule()
    expect(module.turnRowPresentationEvidence({
      turnRowCount: 1,
      rowTextComplete: true,
      expectedFragmentsPresent: true,
      internalReferenceAbsent: true
    })).toEqual(expect.objectContaining({ ok: true, turnRowCount: 1 }))
    expect(module.turnRowPresentationEvidence({
      turnRowCount: 1,
      rowTextComplete: true,
      expectedFragmentsPresent: false,
      internalReferenceAbsent: true
    }).ok).toBe(false)
    expect(module.turnRowPresentationEvidence({
      turnRowCount: 2,
      rowTextComplete: true,
      expectedFragmentsPresent: true,
      internalReferenceAbsent: true
    }).ok).toBe(false)
  })

  it('rejects internal entity references across every ordinary public surface and log', async () => {
    const module = await milestoneModule()
    const logRoot = taskOwnedSandbox()
    const clean = {
      thread: { turns: [] },
      summary: { status: 'ready' },
      sseBody: '',
      bodyText: 'safe public result',
      bodyHTML: '<main>safe public result</main>',
      bodyTextComplete: true,
      bodyHTMLComplete: true,
      sseBodyComplete: true
    }
    expect(module.publicPIIScanEvidence([clean], ['6222021234567890123'], [logRoot]))
      .toEqual(expect.objectContaining({
        ok: true,
        publicInternalReferenceFindingCount: 0,
        logInternalReferenceFindingCount: 0
      }))
    expect(module.publicPIIScanEvidence([{
      ...clean,
      summary: { text: 'entity cer1_case_scoped_reference' }
    }], ['6222021234567890123'], [logRoot])).toEqual(expect.objectContaining({
      ok: false,
      publicInternalReferenceFindingCount: 1
    }))
    expect(module.publicPIIScanEvidence([{
      ...clean,
      bodyHTML: '<span>6222-0212 3456 7890 123</span>'
    }], ['6222021234567890123'], [logRoot])).toEqual(expect.objectContaining({
      ok: false,
      publicFindingCount: 1
    }))
    expect(module.publicPIIScanEvidence([{
      ...clean,
      bodyHTML: '<span>&#54;&#50;&#50;&#50;&#48;&#50;&#49;&#50;&#51;&#52;&#53;&#54;&#55;&#56;&#57;&#48;&#49;&#50;&#51;</span>'
    }], ['6222021234567890123'], [logRoot])).toEqual(expect.objectContaining({
      ok: false,
      publicFindingCount: 1
    }))
    writeFileSync(join(logRoot, 'runtime.log'), 'leaked cer1_private_entity\n', { mode: 0o600 })
    expect(module.publicPIIScanEvidence([clean], ['6222021234567890123'], [logRoot]))
      .toEqual(expect.objectContaining({
        ok: false,
        logInternalReferenceFindingCount: 1
      }))
  })

  it('binds production general V3/V4 compaction to exact pre-history and replacement', async () => {
    const {
      compactionItems,
      compactionTransitionEvidence,
      exactCompactionRecoveryMatches,
      inspectCompactionItem
    } = await milestoneModule()
    for (const schemaVersion of [3, 4] as const) {
      const fixture = validGeneralCompactionTransition(schemaVersion)
      const turn = fixture.after.turns[0]
      expect(inspectCompactionItem(fixture.after, turn, fixture.item)).toEqual({
        ok: true,
        schemaKind: `general_v${schemaVersion}`,
        blocker: ''
      })
      expect(compactionItems(fixture.after)).toEqual([])

      const compacted = compactionTransitionEvidence(fixture.before, fixture.after)
      expect(compacted).toEqual(expect.objectContaining({
        ok: true,
        schemaKind: `general_v${schemaVersion}`,
        compactions: [fixture.item],
        sourceProjectionDigest: fixture.item.sourceDigest
      }))

      const compactedProjection = {
        compactionEvidence: compacted,
        compactionCount: compacted.compactions.length,
        compactionsDigest: sha256(canonicalJSON(compacted.compactions))
      }
      const recovered = compactionTransitionEvidence(
        fixture.before,
        structuredClone(fixture.after)
      )
      const recoveredProjection = {
        compactionEvidence: recovered,
        compactionCount: recovered.compactions.length,
        compactionsDigest: sha256(canonicalJSON(recovered.compactions))
      }
      expect(exactCompactionRecoveryMatches(
        compactedProjection,
        recoveredProjection
      )).toBe(true)

      const changedAfter = structuredClone(fixture.after)
      changedAfter.turns[2].items[0].text = 'tampered retained tail'
      const changed = compactionTransitionEvidence(fixture.before, changedAfter)
      expect(changed).toEqual(expect.objectContaining({
        ok: false,
        blocker: 'compaction_replacement_projection_mismatch'
      }))
      expect(exactCompactionRecoveryMatches(compactedProjection, {
        compactionEvidence: changed,
        compactionCount: 0,
        compactionsDigest: sha256(canonicalJSON([]))
      })).toBe(false)

      if (schemaVersion === 4) {
        const invalidContinuation = structuredClone(fixture.after)
        const invalidItem = invalidContinuation.turns[0].items[0]
        invalidItem.taskContinuation.stateDigest = 'f'.repeat(64)
        const proof = structuredClone(invalidItem)
        delete proof.reasoningExclusionProof
        invalidItem.reasoningExclusionProof = `sha256:${sha256(goCanonicalJSON(proof))}`
        expect(compactionTransitionEvidence(
          fixture.before,
          invalidContinuation
        )).toEqual(expect.objectContaining({
          ok: false,
          blocker: 'compaction_schema_contract_invalid'
        }))
      }
    }
  })

  it('rejects self-consistent markers detached from the actual pre-compaction public items', async () => {
    const { compactionItems, compactionTransitionEvidence } = await milestoneModule()
    const fixture = validGeneralCompactionTransition(3)
    expect(compactionItems(fixture.after)).toEqual([])

    const mutations: Array<{
      blocker: string
      mutate: (before: Record<string, any>, after: Record<string, any>) => void
    }> = [{
      blocker: 'compaction_source_item_membership_mismatch',
      mutate: (_before, after) => {
        const item = after.turns[0].items[0]
        item.sourceItemIds = ['item_user_1', 'forged_source']
        item.reasoningExclusionProof = ''
        const proof = structuredClone(item)
        delete proof.reasoningExclusionProof
        item.reasoningExclusionProof = `sha256:${sha256(goCanonicalJSON(proof))}`
      }
    }, {
      blocker: 'compaction_source_digest_not_publicly_recomputable',
      mutate: (_before, after) => {
        const item = after.turns[0].items[0]
        item.sourceDigest = 'c'.repeat(64)
        item.digestMarker = `sha256:${'c'.repeat(12)}`
        item.reasoningExclusionProof = ''
        const proof = structuredClone(item)
        delete proof.reasoningExclusionProof
        item.reasoningExclusionProof = `sha256:${sha256(goCanonicalJSON(proof))}`
      }
    }, {
      blocker: 'compaction_source_digest_not_publicly_recomputable',
      mutate: (before) => {
        before.turns[0].items[0].text = 'different pre-compaction bytes'
      }
    }, {
      blocker: 'compaction_replaced_token_count_mismatch',
      mutate: (_before, after) => {
        const item = after.turns[0].items[0]
        item.replacedTokens += 1
        item.reasoningExclusionProof = ''
        const proof = structuredClone(item)
        delete proof.reasoningExclusionProof
        item.reasoningExclusionProof = `sha256:${sha256(goCanonicalJSON(proof))}`
      }
    }]
    for (const { blocker, mutate } of mutations) {
      const before = structuredClone(fixture.before)
      const after = structuredClone(fixture.after)
      mutate(before, after)
      expect(compactionTransitionEvidence(before, after)).toEqual(expect.objectContaining({
        ok: false,
        blocker
      }))
    }
  })

  it('recognizes production case V3 but fails closed without a public source attestation', async () => {
    const { compactionTransitionEvidence, inspectCompactionItem } = await milestoneModule()
    const fixture = validCaseCompactionItem()
    expect(inspectCompactionItem(fixture.thread, fixture.turn, fixture.item)).toEqual({
      ok: true,
      schemaKind: 'case_v3',
      blocker: ''
    })

    const before = validGeneralCompactionTransition(3).before
    const rawAfter = {
      id: fixture.thread.id,
      historyAuthority: 'case_boundary_only_v1',
      turns: [fixture.turn, ...structuredClone(before.turns.slice(-2))]
    }
    expect(compactionTransitionEvidence(before, rawAfter)).toEqual(expect.objectContaining({
      ok: false,
      schemaKind: 'case_v3',
      blocker: 'case_compaction_source_not_publicly_recomputable'
    }))

    const publicAfter = {
      id: fixture.thread.id,
      historyAuthority: 'case_boundary_only_v1',
      turns: [{
        id: fixture.turn.id,
        threadId: fixture.thread.id,
        status: 'completed',
        items: []
      }, ...structuredClone(before.turns.slice(-2))]
    }
    expect(compactionTransitionEvidence(before, publicAfter)).toEqual(expect.objectContaining({
      ok: false,
      schemaKind: 'case_public_projection_withheld',
      blocker: 'case_compaction_public_attestation_unavailable'
    }))

    const tampered = structuredClone(fixture)
    tampered.item.caseCompactionBinding.compactedTurnsDigest = 'd'.repeat(64)
    const tamperedProof = structuredClone(tampered.item)
    delete tamperedProof.reasoningExclusionProof
    tampered.item.reasoningExclusionProof = `sha256:${sha256(goCanonicalJSON(tamperedProof))}`
    expect(inspectCompactionItem(tampered.thread, tampered.turn, tampered.item)).toEqual({
      ok: false,
      schemaKind: 'invalid',
      blocker: 'compaction_schema_contract_invalid'
    })
  })

  it('has no built-in substitute, private data backdoor, direct turn driver, or credential injection', () => {
    const script = source('scripts/runtime-go-packaged-milestone-b.mjs')
    for (const forbidden of [
      'writeFixture',
      'createTurn(',
      'stageImmutableDatasetSnapshotV2',
      'dataset-snapshot:v2:stage',
      'ANALYTIX_TEST_AUTH',
      'ANALYTIX_PROVIDER_API_KEY',
      'ANALYTIX_RUNTIME_TOKEN',
      'arbitrarySql: true',
      'databasePathInput: true',
      'count_case_rows'
    ]) {
      expect(script).not.toContain(forbidden)
    }
    expect(script).toContain('directRuntimeTurnDriverUsed: false')
    expect(script).toContain('directSnapshotIPCUsed: false')
    expect(script).toContain('fixtureUsed: false')
    expect(script).toContain('syntheticProviderUsed: false')
  })

  it('revalidates process start, command, executable, and live ancestry before every exact signal', () => {
    const script = source('scripts/runtime-go-packaged-milestone-b.mjs')
    expect(script).toContain("'/bin/ps', ['-p', String(pid), '-o', 'lstart=']")
    expect(script).toContain('commandDigest: sha256(commandText)')
    expect(script).toContain('executableSetDigest: sha256(canonicalJSON(executablePaths))')
    expect(script).toContain('processIdentityMatches(launchIdentity)')
    expect(script).toContain('const descendants = descendantPids(child.pid)')
    expect(script).toContain('const confirmedDescendants = new Set(descendantPids(child.pid))')
    expect(script).toContain('confirmedDescendants.has(identity.pid) && processIdentityMatches(identity)')
    expect(script).toContain('if (!processIdentityMatches(identity)) continue')
    expect(script).not.toContain('ownedPids = new Set')
    expect(script).not.toContain('exact.reverse()')
    expect(script).not.toContain('process.kill(pid, signal)')
  })

  it('requires a fresh signed provider-body audit and the typed local display seam', () => {
    const script = source('scripts/runtime-go-packaged-milestone-b.mjs')
    expect(script).toContain('analytix.milestone-b.provider-request-audit.v1')
    expect(script).toContain('outbound-provider-request-bodies-before-network-send')
    expect(script).toContain('completeIdentifierFindingCount !== 0')
    expect(script).toContain('verifySignature(')
    expect(script).toContain('ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_TRUSTED_PUBLIC_KEY_SHA256')
    expect(script).toContain('ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_SCANNER_BUILD_SHA256')
    expect(script).toContain('ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_PRIVATE_KEY')
    expect(script).toContain('ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_SOCKET')
    expect(script).toContain('launchProviderAuditScanner(')
    expect(script).toContain('ANALYTIX_PROVIDER_AUDIT_SOCKET_PATH')
    expect(script).toContain('protectedValueSetDigest')
    expect(script).toContain('protectedValueCount')
    expect(script).toContain('scannerPolicySha256')
    expect(script).toContain('canonical-protected-value-match/v1')
    expect(script).toContain("subjectReferencePattern: '^cer1_[a-p]{64}$'")
    expect(script).toContain('html-numeric-entities')
    expect(script).toContain('numericSeparatorElision')
    expect(script).toContain('every-request-body-byte-without-truncation')
    expect(script).toContain('analytix.typed-local-display.v1')
    expect(script).toContain("status: 'UNVERIFIED'")
    expect(script).toContain('blocksMilestoneB: true')
    expect(script).toContain('forbiddenGenericProviderChannels')
    expect(script).toContain('allowedTypedLocalSink')
    expect(script).toContain('fullModeExactValue')
    expect(script).toContain('maskedModeLocalOnly')
    expect(script).toContain('acceptedSlotBindingVerified')
    expect(script).toContain('ordinary-after-typed-local-display')
    expect(script).toContain('typed-local-display-revocation')
    expect(script).toContain('directSourcePreviewEvidence')
    expect(script).toContain('paginationObserved')
    expect(script).toContain("'Chat actions', '会话操作'")
    expect(script).not.toContain("'Data analysis', '数据分析', 'Expand data analysis', '展开数据分析'")
    expect(script).toContain('directPreviewProviderAuditExact')
    expect(script).not.toContain('typed_local_direct_preview_not_safely_triggered_in_packaged_harness')
    expect(script).not.toContain('controlled')
    expect(script).not.toContain('ANALYTIX_MILESTONE_B_CONTROLLED_PII_AUTHORIZATION')
    expect(script).not.toContain('ANALYTIX_MILESTONE_B_CONTROLLED_PII_PUBLIC_KEY')
    expect(script).toContain("arbitrarySQLUsed: null")
    expect(script).toContain("rendererDatabasePathReceived: null")
    expect(script).toContain("completePIIRecorded: null")
    expect(script).toContain("'case-switch-epoch-and-default-entity-isolation'")
    expect(script).toContain("'fork-subagent-case-isolation'")
    expect(script).toContain("'protected-source-general-tool-bypass-denied'")
  })

  it('uses typed local display as the only allowed source-exact sink', async () => {
    const script = source('scripts/runtime-go-packaged-milestone-b.mjs')
    expect(script).toContain("'typed-local-display'")
    expect(script).toContain('typedLocalDisplay')
    expect(script).toContain('forbiddenGenericProviderChannels')
    expect(script).toContain('allowedTypedLocalSink')
    expect(script).toContain('fullModeExactValue')
    expect(script).toContain('maskedModeLocalOnly')
    expect(script).not.toContain('EXTERNAL_CASE_AUTHORIZATION_ATTESTATION')
    expect(script).not.toContain('EXTERNAL_CASE_AUTHORIZATION_PURPOSE')
    expect(script).not.toContain('CONTROLLED_PII_AUTHORIZATION_CONTRACT')
    expect(script).not.toContain('openAndObserveControlledArtifactDisplay')
    expect(script).not.toContain('waitForControlledArtifactRevocation')
    expect(script).not.toContain('controlledPii')
    expect(script).not.toContain('controlled-full-value-public-seam')
    expect(script).not.toContain('controlled-release-audit-binding')
    expect(script).not.toContain('ordinary-after-controlled-display')
  })

  it('keeps product state and protected authority on a separate managed volume topology', async () => {
    const root = taskOwnedSandbox()
    const input = managedProductAuthorityFixture(root)
    const module = await milestoneModule()
    const authority = module.loadMilestoneBManagedProductAuthority({
      ...input,
      repositoryRoot,
      startedAt: new Date(Date.now() + 10_000)
    })
    expect(authority).toEqual(expect.objectContaining({
      contract: 'analytix.runtime-main-owned-authority/v1',
      ownerRoot: input.ownerRoot,
      bootstrapPath: input.bootstrapPath,
      authorityRootCount: 3,
      volumeUUID: 'TEST-INTERNAL-APFS'
    }))
    expect(authority.bootstrapSha256).toMatch(/^[a-f0-9]{64}$/u)

    expect(() => module.loadMilestoneBManagedProductAuthority({
      ...input,
      volumeInfo: () => ({
        ok: false,
        blocker: 'managed_product_data_volume_is_not_internal_nonremovable_apfs'
      }),
      repositoryRoot,
      startedAt: new Date(Date.now() + 10_000)
    })).toThrow('managed_product_data_volume_is_not_internal_nonremovable_apfs')

    const script = source('scripts/runtime-go-packaged-milestone-b.mjs')
    expect(script).toContain("mkdtempSync(join(cache.path, 'analytix-milestone-b-'))")
    expect(script).toContain("'analytix-milestone-b-run-'")
    expect(script).toContain('const userDataDir = join(productRunRoot, \'user-data\')')
    expect(script).toContain('const runtimeDataDir = join(productRunRoot, \'runtime-data\')')
    expect(script).toContain('installManagedAuthorityBootstrap(')
    expect(script).toContain('seedManagedInstallationAuthority(')
    expect(script).not.toContain("const userDataDir = join(sandboxRoot, 'user-data')")
    expect(script).not.toContain("const runtimeDataDir = join(sandboxRoot, 'runtime-data')")
  })

  it('independently verifies external canonical CSV truth and detects later source mutation', async () => {
    const root = taskOwnedSandbox()
    const input = externalCaseAcceptance(root)
    for (const path of [
      input.contractPath,
      input.provenancePath
    ]) {
      const stat = lstatSync(path)
      expect(stat.isFile()).toBe(true)
      expect(stat.nlink).toBe(1)
      expect(stat.mode & 0o077).toBe(0)
      expect(stat.uid).toBe(process.getuid?.())
    }
    const module = await milestoneModule()
    const authority = module.loadMilestoneBExternalCaseAcceptance({
      ...input,
      repositoryRoot,
      startedAt: new Date(Date.now() + 10_000)
    })
    expect(authority.snapshots).toHaveLength(2)
    expect(authority.snapshots[0].expected).toMatchObject({
      inflowMinor: '10025',
      outflowMinor: '4000',
      netMinor: '6025',
      transactionCount: 2
    })
    expect(authority.snapshots[1].expected).toMatchObject({
      inflowMinor: '12025',
      outflowMinor: '4000',
      netMinor: '8025',
      transactionCount: 3
    })
    expect(authority.snapshots[0].evidenceRows).toEqual([
      { occurredAt: '2026-01-02T03:04:05.000000Z', direction: '流入', amount: 'CNY 100.25' },
      { occurredAt: '2026-01-03T03:04:05.000000Z', direction: '流出', amount: 'CNY 40.00' }
    ])
    expect(authority.protectedValues).toEqual(expect.arrayContaining([
      '6222021234567890123',
      '6217009876543210987',
      '授权主体',
      '授权对手',
      'baseline-1'
    ]))
    expect(module.verifyMilestoneBExternalCasePreserved(authority)).toBe(true)
    writeFileSync(input.baselinePath, `${readFileSync(input.baselinePath, 'utf8')}\n`, 'utf8')
    expect(module.verifyMilestoneBExternalCasePreserved(authority)).toBe(false)
  })

  it('keeps synthetic unpackaged diagnostics outside formal real-case admission', async () => {
    const input = externalCaseAcceptance(taskOwnedSandbox(), true, 36)
    const module = await milestoneModule()
    const startedAt = new Date(Date.now() + 10_000)
    expect(() => module.loadMilestoneBExternalCaseAcceptance({
      ...input, repositoryRoot, startedAt
    })).toThrow('external_case_contract_invalid')
    const diagnostic = module.loadMilestoneBExternalCaseAcceptance({
      ...input, repositoryRoot, startedAt, diagnosticSynthetic: true
    })
    expect(diagnostic.diagnosticSynthetic).toBe(true)
    expect(diagnostic.snapshots).toHaveLength(2)
    expect(diagnostic.snapshots[0]).toMatchObject({
      sourceRowCount: 38,
      expected: { transactionCount: 2, evidenceTransactionCount: 2 }
    })
    expect(diagnostic.snapshots[1]).toMatchObject({
      sourceRowCount: 39,
      expected: { transactionCount: 3, evidenceTransactionCount: 3 }
    })

    const script = source('scripts/runtime-go-packaged-milestone-b.mjs')
    expect(script).toContain("args.has('--diagnostic-unpackaged')")
    expect(script).toContain('development-only-unpackaged-synthetic-case-diagnostic')
    expect(script).toContain('development diagnostic does not admit or substitute for a formal packaged artifact')
    expect(script).not.toContain("api.account")
    expect(script).toContain("environment.NODE_ENV = 'development'")
    expect(script).not.toContain("ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP")
    expect(script).toContain('environment.ELECTRON_RENDERER_URL = expectedPackagedRendererURL()')
    expect(script).toContain("resolve(process.cwd(), 'out/preload/index.cjs')")
    expect(script).toContain('diagnosticSynthetic: diagnosticUnpackaged')
    expect(script).toContain('child.analytixLaunchIdentity = null')
    expect(script).toContain('identity.executableSetDigest === previous.executableSetDigest')
    expect(script).toContain('if (!await establishLaunchIdentity(firstChild))')
    expect(script).toContain('if (!await establishLaunchIdentity(secondChild))')
    expect(script).toContain('function refreshTaskOwnedChildIdentity(child)')
    expect(script).toContain('current.startTime !== pinned.startTime')
    expect(script).toContain('const launchIdentity = refreshTaskOwnedChildIdentity(child)')
    expect(script).toContain("lastObservationBlocker || 'packaged_renderer_not_ready'")
    expect(script).toContain('const observationSliceMs = diagnosticUnpackaged ? 30_000 : 10_000')
    expect(script).toContain("const composer = editor?.closest('.ds-composer-shell') || null")
    expect(script).toContain("blocker: latest?.editorTextLength === 0")
    expect(script).toContain("? 'composer_workspace_not_ready'")
    expect(script).toContain(": 'composer_input_state_not_updated'")
    expect(script).toContain("id !== 'formal-packaged-artifact'")
    expect(script).not.toContain("report.passed = report.diagnostic.passed")

    const runner = source('scripts/runtime-go-unpackaged-milestone-b-diagnostic.mjs')
    expect(runner).toContain('isolated-synthetic-diagnostic-case')
    expect(runner).not.toContain('ANALYTIX_HUB_TEST_')
    expect(runner).not.toContain('parseEnvFile')
    expect(runner).not.toContain('--env-file')
    expect(runner).not.toContain('...process.env')
    expect(runner).toContain('function trustedCacheTempRoot()')
    expect(runner).toContain("relative('/Volumes/AnalytixCache', root)")
    expect(runner).toContain("'analytix-b1-diagnostic-case-',")
    expect(runner).toContain('2025-12-10 04:')
    expect(runner).toContain('transactionCount: 2, evidenceTransactionCount: 2')
    expect(runner).toContain('transactionCount: 3, evidenceTransactionCount: 3')
    expect(runner).toContain("report?.passed === true")
    expect(runner).toContain("report?.diagnostic?.passed !== true")

    const scanner = source('scripts/provider-request-audit-scanner.mjs')
    expect(scanner).toContain('diagnosticSynthetic: input.diagnosticSynthetic')
    expect(scanner).toContain("typeof value.diagnosticSynthetic !== 'boolean'")
  })

  it('attributes the active diagnostic failure despite an intentional formal blocker', async () => {
    const module = await milestoneModule()
    const report = {
      runtimeBlocker: '',
      checks: [
        {
          id: 'formal-packaged-artifact',
          status: 'BLOCKED',
          message: 'development diagnostic does not admit a formal artifact'
        },
        {
          id: 'ordinary-before-case',
          status: 'UNVERIFIED',
          message: 'not run'
        }
      ]
    }

    expect(module.attributeMilestoneBActiveFailure(
      report,
      { phase: 'ordinary_before_case', checkId: 'ordinary-before-case' },
      'ordinary_before_case_failed'
    )).toEqual({
      phase: 'ordinary_before_case',
      checkId: 'ordinary-before-case',
      reasonCode: 'ordinary_before_case_failed'
    })
    expect(report.runtimeBlocker).toBe('ordinary_before_case_failed')
    expect(report.checks).toEqual([
      {
        id: 'formal-packaged-artifact',
        status: 'BLOCKED',
        message: 'development diagnostic does not admit a formal artifact'
      },
      {
        id: 'ordinary-before-case',
        status: 'FAIL',
        message: 'ordinary_before_case_failed'
      }
    ])

    const zeroRequest = module.providerAuditDependentEvidence({
      auditReceipt: { ok: false, blocker: 'provider_audit_receipt_not_observed' },
      observedProviderRequestCount: 0,
      minimumProviderSafeSemanticBlockCount: 0,
      upstreamReasonCode: report.runtimeBlocker
    })
    expect(zeroRequest.payload).toEqual({
      ok: false,
      blocked: true,
      blocker: 'provider_audit_blocked_by_ordinary_before_case_failed'
    })
    expect(zeroRequest.semantic).toEqual(zeroRequest.payload)

    const invalidObservedReceipt = module.providerAuditDependentEvidence({
      auditReceipt: { ok: false, blocker: 'provider_audit_receipt_invalid' },
      observedProviderRequestCount: 1,
      minimumProviderSafeSemanticBlockCount: 1,
      upstreamReasonCode: ''
    })
    expect(invalidObservedReceipt.payload).toEqual({
      ok: false,
      blocker: 'provider_audit_receipt_invalid'
    })
    expect(invalidObservedReceipt.payload).not.toHaveProperty('blocked')
    expect(invalidObservedReceipt.semantic).toEqual({
      ok: false,
      blocker: 'provider_audit_receipt_invalid'
    })

    const unsafeReport = {
      runtimeBlocker: '',
      checks: [{ id: 'ordinary-before-case', status: 'UNVERIFIED', message: 'not run' }]
    }
    expect(module.attributeMilestoneBActiveFailure(
      unsafeReport,
      {
        phase: 'ordinary turn: complete provider body',
        checkId: 'ordinary-before-case\nunsafe'
      },
      'raw Provider error with token=secret'
    )).toEqual({
      phase: 'runtime_verification',
      checkId: '',
      reasonCode: 'packaged_milestone_b_runtime_verification_failed'
    })
    expect(unsafeReport.runtimeBlocker).toBe(
      'packaged_milestone_b_runtime_verification_failed'
    )
    expect(unsafeReport.checks[0].status).toBe('UNVERIFIED')
  })

  it('rejects tampered snapshot provenance without an external legal-authorization gate', async () => {
    const input = externalCaseAcceptance(taskOwnedSandbox())
    const module = await milestoneModule()
    const startedAt = new Date(Date.now() + 10_000)
    const originalProvenance = readFileSync(input.provenancePath, 'utf8')
    const provenance = JSON.parse(originalProvenance)
    provenance.snapshots[0].provenanceRecordDigest = sha256('self-declared-provenance')
    writeFileSync(input.provenancePath, JSON.stringify(provenance), { encoding: 'utf8', mode: 0o600 })
    expect(() => module.loadMilestoneBExternalCaseAcceptance({
      ...input, repositoryRoot, startedAt
    })).toThrow('external_case_snapshot_provenance_invalid')
  })

  it('reports missing external seams as BLOCKED/UNVERIFIED and never passes from exit zero', () => {
    const baseArgs = [milestoneScriptPath, '--dry-run', '--json', '--no-write']
    const reportOnly = spawnSync(process.execPath, [...baseArgs, '--no-gate'], {
      cwd: repositoryRoot,
      env: {
        ...process.env,
        ANALYTIX_RUNTIME_GO_PACKAGED_SOURCE_COMMIT: '',
        ANALYTIX_MILESTONE_B_PRODUCT_DATA_OWNER_ROOT: '',
        ANALYTIX_MILESTONE_B_AUTHORITY_BOOTSTRAP: '',
        ANALYTIX_MILESTONE_B_INSTALLATION_AUTHORITY_KEY: '',
        ANALYTIX_MILESTONE_B_CASE_WORKSPACE: '',
        ANALYTIX_MILESTONE_B_CASE_OWNER_ROOT: '',
        ANALYTIX_MILESTONE_B_CASE_CONTRACT: '',
        ANALYTIX_MILESTONE_B_CASE_PROVENANCE: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_OWNER_ROOT: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_RECEIPT: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_CHALLENGE: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_PUBLIC_KEY: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_PRIVATE_KEY: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_SOCKET: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_TRUSTED_PUBLIC_KEY_SHA256: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_SCANNER_BUILD_SHA256: ''
      },
      encoding: 'utf8',
      stdio: 'pipe'
    })
    expect(reportOnly.status).toBe(0)
    const report = JSON.parse(reportOnly.stdout)
    expect(report).toMatchObject({
      id: 'runtime-go-packaged-milestone-b',
      status: 'BLOCKED',
      passed: false,
      mockUsed: false,
      fixtureUsed: false,
      syntheticProviderUsed: false,
      typedLocalDisplay: {
        status: 'UNVERIFIED',
        blocksMilestoneB: true
      },
      directSourcePreview: {
        status: 'UNVERIFIED',
        blocksMilestoneB: true
      },
      phaseLanes: expect.objectContaining({
        ordinaryProvider: expect.objectContaining({
          status: 'UNVERIFIED',
          executed: false,
          not_executed: true
        }),
        b1DirectSourcePreview: expect.objectContaining({
          status: 'UNVERIFIED',
          executed: false,
          not_executed: true
        }),
        b1DeterministicFunds: expect.objectContaining({
          status: 'UNVERIFIED',
          executed: false,
          not_executed: true
        }),
        b1AgentAcceptedSlot: expect.objectContaining({
          status: 'UNVERIFIED',
          executed: false,
          not_executed: true
        })
      })
    })
    expect(report.blockedCheckIds).toEqual(expect.arrayContaining([
      'formal-packaged-artifact',
      'managed-product-data-authority',
      'external-owner-isolated-case',
      'provider-request-audit-authority'
    ]))
    expect(report.unverifiedCheckIds).toContain('typed-local-display')
    expect(report.unverifiedCheckIds).toContain('direct-source-preview')
    expect(report.unverifiedCheckIds).toContain('typed-local-display-rehydration')
    expect(report.unverifiedCheckIds).toContain('real-account-flow-one')
    expect(report.postRCChecks).toEqual([
      expect.objectContaining({
        id: 'authorized-joint-case-linkage',
        status: 'UNVERIFIED'
      })
    ])
    expect(report.unverifiedCheckIds).not.toContain('authorized-joint-case-linkage')

    const gated = spawnSync(process.execPath, baseArgs, {
      cwd: repositoryRoot,
      env: {
        ...process.env,
        ANALYTIX_RUNTIME_GO_PACKAGED_SOURCE_COMMIT: '',
        ANALYTIX_MILESTONE_B_PRODUCT_DATA_OWNER_ROOT: '',
        ANALYTIX_MILESTONE_B_AUTHORITY_BOOTSTRAP: '',
        ANALYTIX_MILESTONE_B_INSTALLATION_AUTHORITY_KEY: '',
        ANALYTIX_MILESTONE_B_CASE_WORKSPACE: '',
        ANALYTIX_MILESTONE_B_CASE_OWNER_ROOT: '',
        ANALYTIX_MILESTONE_B_CASE_CONTRACT: '',
        ANALYTIX_MILESTONE_B_CASE_PROVENANCE: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_OWNER_ROOT: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_RECEIPT: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_CHALLENGE: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_PUBLIC_KEY: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_PRIVATE_KEY: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_SOCKET: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_TRUSTED_PUBLIC_KEY_SHA256: '',
        ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_SCANNER_BUILD_SHA256: ''
      },
      encoding: 'utf8',
      stdio: 'pipe'
    })
    expect(gated.status).not.toBe(0)
    expect(JSON.parse(gated.stdout).passed).toBe(false)
  })
})

it('R130 binds B1 producer harness entries to the formal consumer source closure', async () => {
  const { milestoneBHarnessManifestEvidence } = await milestoneModule()
  // @ts-expect-error The formal evidence harness is JavaScript.
  const { runtimeGoFormalEvidenceContract } = await import('../../scripts/runtime-go-formal-evidence.mjs')
  const expected = runtimeGoFormalEvidenceContract.b1HarnessFiles.map((file: string) => {
    const bytes = readFileSync(join(repositoryRoot, file))
    return { name: file.split('/').at(-1), regular: true, byteLength: bytes.length, sha256: sha256(bytes) }
  })
  expect(milestoneBHarnessManifestEvidence().entries).toEqual(expected)
  expect(source('scripts/runtime-go-packaged-milestone-b.mjs')).toContain('expectedProvider: credentialProvider')
})
