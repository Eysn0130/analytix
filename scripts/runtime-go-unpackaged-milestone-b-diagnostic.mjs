#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import { createHash, generateKeyPairSync } from 'node:crypto'
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync
} from 'node:fs'
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const rawArgs = process.argv.slice(2)

function optionValue(name, fallback = '') {
  const inline = rawArgs.find((value) => value.startsWith(`${name}=`))
  if (inline) return inline.slice(name.length + 1)
  const index = rawArgs.indexOf(name)
  return index >= 0 ? rawArgs[index + 1] || '' : fallback
}

function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}

function canonicalJSON(value) {
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`
  if (value && typeof value === 'object') {
    return `{${Object.entries(value)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSON(item)}`)
      .join(',')}}`
  }
  return JSON.stringify(value)
}

function diagnosticEnvironment() {
  const environment = {}
  for (const name of [
    'PATH', 'SHELL', 'LANG', 'LC_ALL', 'LC_CTYPE', 'SystemRoot',
    'TMPDIR', 'TEMP', 'TMP', 'ANALYTIX_DEV_CACHE_ROOT', 'npm_config_cache',
    'XDG_CACHE_HOME', 'PIP_CACHE_DIR', 'UV_CACHE_DIR', 'PYTHONPYCACHEPREFIX',
    'MYPY_CACHE_DIR', 'RUFF_CACHE_DIR', 'GOCACHE', 'GOMODCACHE', 'GOTMPDIR',
    'CARGO_HOME', 'CARGO_TARGET_DIR', 'CCACHE_DIR', 'SCCACHE_DIR', 'XWIN_CACHE_DIR',
    'COREPACK_HOME', 'ELECTRON_CACHE', 'ELECTRON_BUILDER_CACHE',
    'PLAYWRIGHT_BROWSERS_PATH', 'NODE_COMPILE_CACHE'
  ]) {
    if (process.env[name] !== undefined) environment[name] = process.env[name]
  }
  return environment
}

function privateTempRoot(prefix, parent = '/private/tmp') {
  const root = mkdtempSync(join(parent, prefix))
  chmodSync(root, 0o700)
  return root
}

function trustedCacheTempRoot() {
  const configured = String(process.env.TMPDIR || '').trim()
  if (!configured) throw new Error('diagnostic trusted cache TMPDIR is unavailable')
  const root = realpathSync(configured)
  const escaped = relative('/Volumes/AnalytixCache', root)
  if (escaped === '..' || escaped.startsWith(`..${sep}`) || isAbsolute(escaped)) {
    throw new Error('diagnostic TMPDIR must be contained by /Volumes/AnalytixCache')
  }
  return root
}

function writePrivate(path, body) {
  writeFileSync(path, body, { encoding: 'utf8', mode: 0o600, flag: 'wx' })
  chmodSync(path, 0o600)
}

const headers = [
  '交易卡号', '交易账号', '账户开户名称', '开户人证件号码', '交易时间', '交易金额',
  '交易余额', '收付标志', '交易对手账卡号', '现金标志', '对手户名', '对手身份证号',
  '对手开户银行', '摘要说明', '交易币种', '交易网点名称', '交易网点代码', '交易发生地',
  '交易是否成功', '传票号', '终端号', 'IP地址', 'MAC地址', '对手交易余额', '交易流水号',
  '日志号', '凭证种类', '凭证号', '交易柜员号', '商户名称', '商户号', '备注', '交易类型',
  '查询反馈结果原因'
]

function csvRow({ account, counterparty, timestamp, amount, direction, sequence }) {
  const values = Array.from({ length: headers.length }, () => '')
  values[0] = account
  values[1] = account
  values[2] = '合成主体甲'
  values[3] = '110101199001010011'
  values[4] = timestamp
  values[5] = amount
  values[6] = '1000.00'
  values[7] = direction
  values[8] = counterparty
  values[10] = '合成对手乙'
  values[11] = '110101199202020022'
  values[12] = '合成测试银行'
  values[14] = 'CNY'
  values[18] = '成功'
  values[20] = '13800138000'
  values[21] = '192.0.2.10'
  values[22] = '02:00:00:00:00:01'
  values[24] = sequence
  return values.join(',')
}

function createSyntheticCase(ownerRoot) {
  const workspace = join(ownerRoot, 'case-workspace')
  mkdirSync(workspace, { recursive: true, mode: 0o700 })
  chmodSync(workspace, 0o700)
  const account = '6222021234567890123'
  const counterparty = '6217009876543210987'
  const paginationRows = Array.from({ length: 36 }, (_, index) => csvRow({
    account,
    counterparty,
    timestamp: `2025-12-10 04:${String(index).padStart(2, '0')}:00`,
    amount: '1.00',
    direction: index % 2 === 0 ? '进' : '出',
    sequence: `synthetic-pagination-${String(index + 1).padStart(2, '0')}`
  }))
  const baseline = [
    headers.join(','),
    csvRow({ account, counterparty, timestamp: '2026-01-02 03:04:05', amount: '100.25', direction: '进', sequence: 'synthetic-baseline-1' }),
    csvRow({ account, counterparty, timestamp: '2026-01-03 03:04:05', amount: '40.00', direction: '出', sequence: 'synthetic-baseline-2' }),
    ...paginationRows
  ].join('\n')
  const evolved = [
    baseline,
    csvRow({ account, counterparty, timestamp: '2026-01-04 03:04:05', amount: '20.00', direction: '进', sequence: 'synthetic-evolved-3' })
  ].join('\n')
  const baselinePath = join(workspace, 'baseline.csv')
  const evolvedPath = join(workspace, 'evolved.csv')
  writePrivate(baselinePath, baseline)
  writePrivate(evolvedPath, evolved)
  const query = {
    completeAccount: account,
    startInclusive: '2026-01-01T00:00:00.000000Z',
    endInclusive: '2026-01-31T23:59:59.000000Z',
    evidenceRowLimit: 3
  }
  const contract = {
    contract: 'analytix.milestone-b.synthetic-case-contract.v1',
    case: {
      classification: 'isolated-synthetic-diagnostic-case',
      canonicalProfile: 'canonical_direct_csv_v1'
    },
    snapshots: [
      {
        label: 'baseline', sourceRelativePath: 'baseline.csv', sourceRevision: 1, query,
        expected: {
          currency: 'CNY', minorUnitScale: 2, inflowMinor: '10025', outflowMinor: '4000',
          netMinor: '6025', transactionCount: 2, evidenceTransactionCount: 2
        }
      },
      {
        label: 'evolved', sourceRelativePath: 'evolved.csv', sourceRevision: 2, query,
        expected: {
          currency: 'CNY', minorUnitScale: 2, inflowMinor: '12025', outflowMinor: '4000',
          netMinor: '8025', transactionCount: 3, evidenceTransactionCount: 3
        }
      }
    ]
  }
  const contractBody = JSON.stringify(contract)
  const contractPath = join(ownerRoot, 'diagnostic-acceptance.json')
  writePrivate(contractPath, contractBody)
  const contractSha256 = sha256(contractBody)
  const provenanceEntry = (label, body, acquiredAt) => {
    const sourceSha256 = sha256(body)
    const sourceByteLength = Buffer.byteLength(body)
    return {
      label,
      sourceSha256,
      sourceByteLength,
      provenanceRecordDigest: sha256(canonicalJSON({
        contract: 'analytix.milestone-b.snapshot-provenance-record.v1',
        caseContractSha256: contractSha256,
        label,
        sourceSha256,
        sourceByteLength,
        acquiredAt
      })),
      acquiredAt
    }
  }
  const provenance = {
    contract: 'analytix.milestone-b.synthetic-case-provenance.v1',
    caseContractSha256: contractSha256,
    snapshots: [
      provenanceEntry('baseline', baseline, '2026-01-01T00:00:01.000Z'),
      provenanceEntry('evolved', evolved, '2026-01-01T00:00:02.000Z')
    ]
  }
  const provenancePath = join(ownerRoot, 'diagnostic-provenance.json')
  writePrivate(provenancePath, JSON.stringify(provenance))
  return { workspace, contractPath, provenancePath }
}

function createManagedProductAuthority(ownerRoot) {
  const manifestRoot = join(ownerRoot, 'authority-manifests')
  const profileRoot = join(ownerRoot, 'authority-credential-profiles')
  const bundleRoot = join(ownerRoot, 'authority-credential-bundles')
  for (const path of [manifestRoot, profileRoot, bundleRoot]) {
    mkdirSync(path, { recursive: true, mode: 0o700 })
    chmodSync(path, 0o700)
  }
  const keys = generateKeyPairSync('ed25519')
  const publicDER = keys.publicKey.export({ format: 'der', type: 'spki' })
  const publicRaw = publicDER.subarray(publicDER.length - 32)
  const privateJWK = keys.privateKey.export({ format: 'jwk' })
  const authorityKeyId = sha256(publicRaw)
  const authorityAnchorV1 = JSON.stringify({
    schemaVersion: 1,
    installationId: sha256('analytix-b1-unpackaged-diagnostic-installation'),
    authorityKeyId,
    authorityPublicKey: publicRaw.toString('base64url'),
    currentManifestDigest: sha256('analytix-b1-unpackaged-diagnostic-manifest')
  })
  const installationAuthorityKeyPath = join(ownerRoot, 'installation-authority-key.json')
  writePrivate(installationAuthorityKeyPath, JSON.stringify({
    schemaVersion: 1,
    algorithm: 'Ed25519',
    keyId: authorityKeyId,
    publicKey: publicRaw.toString('base64url'),
    privateSeed: privateJWK.d
  }))
  const bootstrapPath = join(ownerRoot, 'authority-bootstrap-v1.json')
  writePrivate(bootstrapPath, JSON.stringify({
    schemaVersion: 1,
    purpose: 'analytix.runtime-main-owned-authority/v1',
    authorityAnchorV1,
    authorityManifestRoot: manifestRoot,
    authorityCredentialProfileRoot: profileRoot,
    authorityCredentialBundleRoot: bundleRoot
  }))
  return { bootstrapPath, installationAuthorityKeyPath }
}

function createProviderAuditAuthority(ownerRoot) {
  const keys = generateKeyPairSync('ed25519')
  const publicKeyPath = join(ownerRoot, 'provider-audit-public.pem')
  const privateKeyPath = join(ownerRoot, 'provider-audit-private.pem')
  writePrivate(publicKeyPath, keys.publicKey.export({ format: 'pem', type: 'spki' }))
  writePrivate(privateKeyPath, keys.privateKey.export({ format: 'pem', type: 'pkcs8' }))
  return {
    publicKeyPath,
    privateKeyPath,
    publicKeySha256: sha256(readFileSync(publicKeyPath)),
    challengePath: join(ownerRoot, 'provider-audit-challenge.json'),
    receiptPath: join(ownerRoot, 'provider-audit-receipt.json'),
    socketPath: join(ownerRoot, 'provider-audit.sock')
  }
}

const runtimeServer = resolve(optionValue('--runtime-server'))
if (!optionValue('--runtime-server') || !existsSync(runtimeServer)) {
  throw new Error('pass --runtime-server with the current-source Go runtime binary')
}
const sourceCommit = optionValue('--source-commit')
if (!/^[0-9a-f]{40}$/u.test(sourceCommit)) {
  throw new Error('pass --source-commit with the current 40-hex HEAD')
}
const productOwnerRoot = privateTempRoot('analytix-b1-diagnostic-product-')
// Renderer workspace normalization deliberately rejects OS temporary roots.
// Keep the synthetic case isolated and disposable, but place its workspace
// under the configured trusted cache so it exercises the accepted workspace seam.
const caseOwnerRoot = privateTempRoot(
  'analytix-b1-diagnostic-case-',
  trustedCacheTempRoot()
)
const providerOwnerRoot = privateTempRoot('analytix-b1-diagnostic-provider-')
try {
  const product = createManagedProductAuthority(productOwnerRoot)
  const caseAuthority = createSyntheticCase(caseOwnerRoot)
  const providerAudit = createProviderAuditAuthority(providerOwnerRoot)
  const scannerPath = resolve(repositoryRoot, 'scripts/provider-request-audit-scanner.mjs')
  const result = spawnSync(process.execPath, [
    resolve(repositoryRoot, 'scripts/runtime-go-packaged-milestone-b.mjs'),
    '--diagnostic-unpackaged',
    '--json',
    '--no-write',
    '--no-gate',
    '--source-commit', sourceCommit,
    '--diagnostic-runtime-server', runtimeServer,
    '--product-data-owner-root', productOwnerRoot,
    '--authority-bootstrap', product.bootstrapPath,
    '--installation-authority-key', product.installationAuthorityKeyPath,
    '--case-workspace', caseAuthority.workspace,
    '--case-owner-root', caseOwnerRoot,
    '--case-contract', caseAuthority.contractPath,
    '--case-provenance', caseAuthority.provenancePath,
    '--provider-audit-owner-root', providerOwnerRoot,
    '--provider-audit-receipt', providerAudit.receiptPath,
    '--provider-audit-challenge', providerAudit.challengePath,
    '--provider-audit-public-key', providerAudit.publicKeyPath,
    '--provider-audit-private-key', providerAudit.privateKeyPath,
    '--provider-audit-socket', providerAudit.socketPath,
    '--timeout-ms', optionValue('--timeout-ms', '1200000')
  ], {
    cwd: repositoryRoot,
    env: {
      ...diagnosticEnvironment(),
      ANALYTIX_RUNTIME_GO_PACKAGED_SOURCE_COMMIT: sourceCommit,
      ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_TRUSTED_PUBLIC_KEY_SHA256: providerAudit.publicKeySha256,
      ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_SCANNER_BUILD_SHA256: sha256(readFileSync(scannerPath))
    },
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
    maxBuffer: 16 * 1024 * 1024
  })
  if (result.error) throw result.error
  let report
  try {
    report = JSON.parse(String(result.stdout || '').trim())
  } catch {
    throw new Error('unpackaged diagnostic did not return a JSON report')
  }
  process.stdout.write(`${JSON.stringify(report)}\n`)
  if (result.status !== 0 || report?.diagnostic?.passed !== true || report?.passed === true ||
      report?.acceptanceClass !== 'development-only-unpackaged-synthetic-case-diagnostic' ||
      report?.fixtureUsed !== true || report?.syntheticProviderUsed !== false) {
    process.exitCode = 1
  }
} finally {
  for (const root of [providerOwnerRoot, caseOwnerRoot, productOwnerRoot]) {
    rmSync(root, { recursive: true, force: true })
  }
}
