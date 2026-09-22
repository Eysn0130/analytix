const { CORE_CONTROLLED_DISPOSITION, isCoreDisposition, CORE_DISPOSITION, ABSENT_FUNDS, isCoreContext, assertCoreResourcesAbsent } = require('./core-package-profile.cjs')
const { execFileSync } = require('node:child_process')
const { createHash, randomUUID } = require('node:crypto')
const { chmodSync, closeSync, constants, copyFileSync, cpSync, existsSync, fstatSync, fsyncSync, linkSync, lstatSync, mkdirSync, mkdtempSync, openSync, readFileSync, readlinkSync, readSync, readdirSync, realpathSync, renameSync, rmdirSync, rmSync, unlinkSync, writeFileSync } = require('node:fs')
const { dirname, isAbsolute, join, relative, resolve, sep } = require('node:path')
const { parse: parseJavaScriptModule } = require('acorn')
const { stagePrivateLocal, verifyPrivateLocal } = require('./office-private-local-contract.cjs')
const {
  RECEIPT_FILE_NAME,
  assertNativeBinaryTarget,
  binaryName: nativeBinaryName,
  nativePayloadIdentity,
  manifestDigest: nativeManifestDigest,
  manifest: nativeComponentManifest,
  nativeBuildContextDigest,
  sha256File,
  sourceSetDigest,
  targetContract: dataNativeTargetContract,
  verifyPackagedComponents
} = require('./native-component-contract.cjs')
const {
  ELECTRON_FUSE_POLICY_V1,
  ELECTRON_FUSE_POLICY_V1_CONTRACT,
  electronFusePolicyV1,
  electronFusePolicyV1Digest
} = require('./electron-fuse-policy.cjs')
const {
  hermeticGoBuildEnvironment,
  resolvePinnedGoToolchain
} = require('./go-runtime-build-contract.cjs')
const {
  policyDigest: macSigningPolicyDigest,
  requireOfficialTeamIdentifier
} = require('./macos-signing-policy.cjs')
const {
  verifyPythonRuntime,
  verifySitePackages
} = require('./windows-python-runtime-contract.cjs')
const {
  loadProductionMcpEntryClosureContract
} = require('../plugins/analytix-fund-analysis/scripts/production-mcp-entry-closure-manifest.cjs')
const { parseStrictJsonObject } = require('./lib/strict-json.cjs')
const {
  DEVELOPMENT_CACHE_ROOT,
  openVerifiedDevelopmentCacheVolume,
  projectGoDevelopmentCacheEnvironment,
  requireDevelopmentNativeComponentRoot
} = require('./lib/development-cache-environment.cjs')
const {
  recordCompletedAfterPack
} = require('./packaged-lifecycle-guard.cjs')._internals
const {
  MANIFEST_FILE_NAME: DOCUMENT_RUNTIME_MANIFEST_FILE,
  UNAVAILABLE: DOCUMENT_RUNTIME_UNAVAILABLE,
  materializeDocumentRuntime,
  verifyDocumentRuntime
} = require('./document-runtime-contract.cjs')

const ANALYTIX_FUNDS_PRODUCTION_MCP_CONTRACT = loadProductionMcpEntryClosureContract(
  join(__dirname, '..', 'plugins', 'analytix-fund-analysis', 'scripts', 'production-mcp-entry-closure.json')
)
const ANALYTIX_FUNDS_PRODUCTION_MCP_FILES = ANALYTIX_FUNDS_PRODUCTION_MCP_CONTRACT.files

const ANALYTIX_FUNDS_PLUGIN_NAME = 'analytix-fund-analysis'
const ANALYTIX_FUNDS_PLUGIN_DECLARATION_PATH = '.analytix-plugin/package.json'
const ANALYTIX_PROJECT_OWNER_NAME = 'Guoqin He'
const ANALYTIX_PROJECT_OWNER_GITHUB_URL = 'https://github.com/Eysn0130'
const ANALYTIX_FUNDS_FORBIDDEN_INTERPRETER_ENV = 'ANALYTIX_FUNDS_DUCKDB_PYTHON'
const PACKAGED_BUILD_AUTHORITY_CONTRACT = 'analytix.packaged-build-authority/v2'
const PACKAGED_BUILD_AUTHORITY_FILE = 'analytix-packaged-build-authority.json'
const PACKAGED_BUILD_AUTHORITY_DOMAIN = 'AnalytixPackagedBuildAuthorityV2\0'
const PACKAGED_WORKTREE_SNAPSHOT_CONTRACT = 'analytix.packaged-worktree-snapshot/v1'
const PACKAGED_WORKTREE_SNAPSHOT_DOMAIN = 'AnalytixPackagedWorktreeSnapshotV1\0'
const PACKAGED_WORKTREE_PATCH_MANIFEST_CONTRACT = 'analytix.packaged-worktree-patch-manifest/v1'
const PACKAGED_WORKTREE_PATCH_MANIFEST_DOMAIN = 'AnalytixPackagedWorktreePatchManifestV1\0'
const PACKAGED_WORKTREE_UNTRACKED_MANIFEST_CONTRACT = 'analytix.packaged-worktree-untracked-manifest/v1'
const PACKAGED_WORKTREE_UNTRACKED_MANIFEST_DOMAIN = 'AnalytixPackagedWorktreeUntrackedManifestV1\0'
const EFFECTIVE_BUILDER_CONTEXT_CONTRACT = 'analytix.electron-builder-effective-context/v1'
const EFFECTIVE_BUILDER_CONFIG_DOMAIN = 'AnalytixElectronBuilderEffectiveConfigV1\0'
const EFFECTIVE_BUILDER_TARGET_DOMAIN = 'AnalytixElectronBuilderEffectiveTargetV1\0'
const EFFECTIVE_BUILDER_CONTEXT_DOMAIN = 'AnalytixElectronBuilderEffectiveContextV1\0'
const STAGED_PAYLOAD_CONTRACT = 'analytix.packaged-staged-payload/v1'
const STAGED_PAYLOAD_MANIFEST_DOMAIN = 'AnalytixPackagedStagedPayloadManifestV1\0'
const STAGED_PAYLOAD_EXCLUSION_DOMAIN = 'AnalytixPackagedStagedPayloadExclusionPolicyV1\0'
const ELECTRON_FUSE_SENTINEL = Buffer.from('dL7pKGdnNz796PbbjQWNKmHXBZaB9tsX', 'ascii')
const ELECTRON_FUSE_V1_WIRE_LENGTH = 9
const ELECTRON_FUSE_STATE_DISABLED = 0x30
const ELECTRON_FUSE_STATE_ENABLED = 0x31
const NATIVE_DEVELOPMENT_BUILD_MARKER = 'analytix-native-development-build.json'
const NATIVE_DISPOSITION_CONTROLLED_RELEASE = 'controlled_release_receipt'
const NATIVE_DISPOSITION_DEVELOPMENT = 'development_non_publishable'
const PACKAGED_DARWIN_NODE_PTY_DIRECTORY_NAME = 'analytix-node-pty'
const PACKAGED_DARWIN_NODE_PTY_RUNTIME_LIBRARIES = Object.freeze([
  'eventEmitter2.js',
  'index.js',
  'terminal.js',
  'unixTerminal.js',
  'utils.js'
])
const MAX_NATIVE_DEVELOPMENT_MARKER_BYTES = 256 * 1024
const PACKAGED_WORKTREE_EXCLUSION_POLICY = Object.freeze({
  schemaVersion: 1,
  contract: 'analytix.packaged-worktree-exclusion-policy/v1',
  untrackedPolicy: 'git-ls-files-others-exclude-standard-no-additional-filter'
})
const PACKAGED_WORKTREE_MAX_GIT_OUTPUT_BYTES = 64 * 1024 * 1024
const PACKAGED_WORKTREE_MAX_FILE_BYTES = 16 * 1024 * 1024
const PACKAGED_WORKTREE_MAX_TOTAL_UNTRACKED_BYTES = 64 * 1024 * 1024
const PACKAGED_WORKTREE_MAX_TRACKED_FILE_BYTES = 2 * 1024 * 1024 * 1024
const PACKAGED_WORKTREE_MAX_TOTAL_TRACKED_BYTES = 16 * 1024 * 1024 * 1024
const PACKAGED_WORKTREE_MAX_PATHS = 4096
const EFFECTIVE_BUILDER_CONFIG_MAX_BYTES = 1024 * 1024
const EFFECTIVE_BUILDER_CONFIG_MAX_DEPTH = 32
const EFFECTIVE_BUILDER_CONFIG_MAX_NODES = 100000
const STAGED_PAYLOAD_MAX_FILE_BYTES = 2 * 1024 * 1024 * 1024
const STAGED_PAYLOAD_MAX_TOTAL_BYTES = 16 * 1024 * 1024 * 1024
const FUNDS_PLUGIN_MAX_FILE_BYTES = 128 * 1024 * 1024
const FUNDS_PLUGIN_MAX_TREE_BYTES = 512 * 1024 * 1024
const FUNDS_PLUGIN_MAX_FILES = 100_000
const FUNDS_PLUGIN_MANIFEST_PATH = '.codex-plugin/plugin.json'
const FUNDS_PLUGIN_ENTRYPOINT_PATH = 'mcp/server.mjs'
const FUNDS_PLUGIN_INSTALL_MARKER = '.analytix-hub-installed-plugin.json'
const FUNDS_PLUGIN_ADMISSION_CONTRACT_V1 = 'analytix.funds-packaged-admission/v1'
const FUNDS_PLUGIN_ADMISSION_DOMAIN_V1 = 'AnalytixFundsPackagedAdmissionV1\0'
const STAGED_PAYLOAD_MAX_PATHS = 200000
const STAGED_PAYLOAD_EXCLUSION_POLICY = Object.freeze({
  schemaVersion: 1,
  contract: 'analytix.packaged-staged-payload-exclusion-policy/v1',
  authoritySelf: `resources/**/${PACKAGED_BUILD_AUTHORITY_FILE}`,
  signingEnvelope: [
    '**/_CodeSignature',
    '**/_CodeSignature/**',
    '**/Contents/CodeResources'
  ]
})
const ANALYTIX_FUNDS_EXPECTED_ENV_VARS = Object.freeze([
  'ANALYTIX_API_BASE_URL',
  'ANALYTIX_API_TOKEN',
  'ANALYTIX_CASE_PROJECT_CONFIG',
  'ANALYTIX_CASE_PROJECT_ROOT',
  'ANALYTIX_WORKSPACE_ROOT',
  'ANALYTIX_DATA_ANALYSIS_DIR',
  'ANALYTIX_DATA_ANALYSIS_HOME',
  'ANALYTIX_DATA_ANALYSIS_CASES_ROOT',
  'ANALYTIX_FUNDS_DISABLE_LOCAL_DUCKDB_WORKBENCH',
  'ANALYTIX_FUNDS_ARTIFACT_DIR',
  'ANALYTIX_FUNDS_DISCOVERY_MODE',
  'ANALYTIX_FUNDS_EXPOSE_ALL_TOOLS',
  'ANALYTIX_FUNDS_ALLOW_DEBUG_PAYLOAD'
])

const ANALYTIX_RUNTIME_REQUIRED_PATHS = [
  'packages/runtime/dist/cli/serve-entry.js',
  'packages/runtime/package.json',
  'packages/runtime/package-lock.json',
  'packages/runtime/node_modules/zod/package.json',
  'packages/runtime/node_modules/diff/package.json',
  'packages/runtime/node_modules/@modelcontextprotocol/sdk/package.json',
  'packages/runtime/node_modules/analytix-computer-use/package.json',
  'packages/runtime/node_modules/analytix-computer-use/bin/analytix-computer-use'
]

const ANALYTIX_RUNTIME_GO_REQUIRED_PATHS = [
  'runtime-go/bin/runtime-server'
]

const ANALYTIX_DATA_NATIVE_REQUIRED_TOOLS = nativeComponentManifest.components.map(
  (component) => component.binaryName
)

const ANALYTIX_NATIVE_GENERATION_METADATA = [
  RECEIPT_FILE_NAME,
  'native-component-generation.v1.json',
  'native-publication-intent.v1.json',
  'inventory.v1.json',
  'receipt.v1.json'
]

const ANALYTIX_MANAGED_CHROME_REQUIRED_PATHS = [
  'managed-chrome/codex-extension/manifest.json',
  'managed-chrome/codex-extension/background.js',
  'managed-chrome/chrome/scripts/browser-client.mjs',
  'managed-chrome/chrome/extension-host/windows/x64/extension-host.exe'
]

const ANALYTIX_RUNTIME_FORBIDDEN_PATHS = [
  'packages/runtime/dist/server',
  'packages/runtime/dist/loop',
  'packages/runtime/dist/adapters/model',
  'packages/runtime/dist/adapters/tool',
  'packages/runtime/dist/delegation',
  'packages/runtime/dist/services',
  'packages/runtime/dist/review'
]

const ANALYTIX_RUNTIME_GO_FORBIDDEN_PATHS = [
  'packages/runtime-go/cmd/contract-sidecar/main.go',
  'packages/runtime-go/cmd/runtime-engine-absorption-report/main.go',
  'packages/runtime-go/internal/upstreamaudit/baseline_absorption.go',
  'packages/runtime-go/internal/mcp/mcp_contract_replay_handler.go',
  'packages/runtime-go/internal/mcp/manager_test_double.go',
  'packages/runtime-go/internal/provider/provider_contract_replay_handler.go',
  'packages/runtime-go/internal/provider/provider_contract_matrix.go',
  'packages/runtime-go/internal/testsupport/providerscript/provider_script.go',
  'packages/runtime-go/internal/readiness/mcp_matrix_contract.go',
  'packages/runtime-go/internal/readiness/provider_matrix_contract.go',
  'packages/runtime-go/internal/conformance/livelocal/harness.go',
  'packages/runtime-go/internal/conformance/livelocal/production_candidate_handler.go',
  'packages/runtime-go/internal/conformance/livelocal/production_candidate.go'
]

const ANALYTIX_RUNTIME_GO_FORBIDDEN_PATTERNS = [
  /^packages\/runtime-go\/internal\/upstreamaudit\//,
  /^packages\/runtime-go\/internal\/conformance\//,
  /^packages\/runtime-go\/internal\/testsupport\/providerscript\//,
  /^packages\/runtime-go\/internal\/readiness\/.*_conformance\.go$/,
  /^packages\/runtime-go\/internal\/server\/.*_conformance\.go$/
]

const BUILD_ONLY_DIRECTORY_NAMES = new Set([
  '.git',
  '.venv',
  '.pytest_cache',
  '__pycache__',
  '__tests__',
  '__fixtures__',
  'benchmark',
  'benchmarks',
  'coverage',
  'demo',
  'demos',
  'doc',
  'docs',
  'documentation',
  'example',
  'examples',
  'fixtures',
  'test',
  'tests',
  'testsupport',
  'upstreamaudit'
])

const BUILD_ONLY_FILE_NAMES = new Set([
  '.DS_Store',
  'README',
  'readme',
  'README.md',
  'readme.md',
  'CHANGELOG',
  'CHANGELOG.md',
  'changelog.md'
])

function normalizePlatform(platform) {
  return platform === 'win' ? 'win32' : platform
}

function goOSForPlatform(platform) {
  const normalized = normalizePlatform(platform)
  if (normalized === 'win32') return 'windows'
  if (normalized === 'darwin') return 'darwin'
  if (normalized === 'linux') return 'linux'
  throw new Error(`[after-pack] Unsupported Go runtime target platform: ${platform}`)
}

function normalizeArch(arch) {
  if (typeof arch === 'number') {
    const electronBuilderArch = new Map([
      [0, 'ia32'],
      [1, 'x64'],
      [2, 'armv7l'],
      [3, 'arm64'],
      [4, 'universal']
    ])
    return electronBuilderArch.get(arch) || process.arch
  }
  return arch || process.arch
}

function goArchForTarget(arch) {
  const normalized = String(normalizeArch(arch)).toLowerCase()
  if (normalized === 'x64' || normalized === 'amd64') return 'amd64'
  if (normalized === 'arm64' || normalized === 'aarch64') return 'arm64'
  if (normalized === 'ia32' || normalized === 'x86') return '386'
  if (normalized === 'armv7l' || normalized === 'arm') return 'arm'
  if (normalized === 'universal') {
    return process.arch === 'arm64' ? 'arm64' : 'amd64'
  }
  throw new Error(`[after-pack] Unsupported Go runtime target arch: ${arch}`)
}

function canonicalJSON(value) {
  return JSON.stringify(value)
}

function sha256Bytes(value) {
  return createHash('sha256').update(value).digest('hex')
}

function domainSeparatedSha256(domain, value) {
  return createHash('sha256').update(domain, 'utf8').update(value).digest('hex')
}

function exactKeys(value, expectedKeys) {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).sort().join(',') === [...expectedKeys].sort().join(',')
}

function isSha256(value) {
  return typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value)
}

function isGitCommit(value) {
  return typeof value === 'string' && /^[0-9a-f]{40}$/u.test(value)
}

function isNonNegativeSafeInteger(value) {
  return Number.isSafeInteger(value) && value >= 0
}

function sameFileIdentity(left, right) {
  return left.dev === right.dev && left.ino === right.ino && left.size === right.size &&
    left.mtimeMs === right.mtimeMs && left.ctimeMs === right.ctimeMs &&
    left.nlink === right.nlink
}

function sameRelocatedDirectoryIdentity(left, right) {
  return left.dev === right.dev && left.ino === right.ino && left.mode === right.mode &&
    left.uid === right.uid && left.gid === right.gid && left.size === right.size &&
    left.mtimeMs === right.mtimeMs && left.birthtimeMs === right.birthtimeMs &&
    left.nlink === right.nlink
}

function hashStableRegularFile(path, options = {}) {
  const maximumBytes = options.maximumBytes || Number.MAX_SAFE_INTEGER
  let before
  try {
    before = lstatSync(path)
  } catch {
    throw new Error(`[after-pack] Missing regular file: ${path}`)
  }
  if (!before.isFile() || before.isSymbolicLink() ||
    (options.requireSingleLink === true && before.nlink !== 1) ||
    (options.requireTrustedMode === true && (before.mode & 0o022) !== 0) ||
    (options.allowEmpty !== true && before.size <= 0) || before.size > maximumBytes) {
    throw new Error(`[after-pack] Untrusted regular file identity: ${path}`)
  }
  const noFollow = typeof constants.O_NOFOLLOW === 'number' ? constants.O_NOFOLLOW : 0
  const fd = openSync(path, constants.O_RDONLY | noFollow)
  try {
    const opened = fstatSync(fd)
    if (!opened.isFile() || !sameFileIdentity(before, opened) ||
      (options.requireSingleLink === true && opened.nlink !== 1) ||
      (options.requireTrustedMode === true && (opened.mode & 0o022) !== 0)) {
      throw new Error(`[after-pack] Regular file changed before read: ${path}`)
    }
    const hash = createHash('sha256')
    const contentChunks = options.includeContent === true ? [] : null
    const buffer = Buffer.allocUnsafe(1024 * 1024)
    let byteLength = 0
    while (true) {
      const read = readSync(fd, buffer, 0, buffer.length, null)
      if (read === 0) break
      byteLength += read
      if (byteLength > maximumBytes) {
        throw new Error(`[after-pack] Regular file exceeds bounded read: ${path}`)
      }
      const chunk = buffer.subarray(0, read)
      hash.update(chunk)
      if (contentChunks) contentChunks.push(Buffer.from(chunk))
    }
    const after = fstatSync(fd)
    let pathAfter
    try {
      pathAfter = lstatSync(path)
    } catch {
      throw new Error(`[after-pack] Regular file disappeared during read: ${path}`)
    }
    if (!sameFileIdentity(opened, after) || !sameFileIdentity(after, pathAfter) ||
      pathAfter.isSymbolicLink() ||
      (options.requireSingleLink === true && (after.nlink !== 1 || pathAfter.nlink !== 1)) ||
      (options.requireTrustedMode === true && ((after.mode | pathAfter.mode) & 0o022) !== 0) ||
      byteLength !== after.size) {
      throw new Error(`[after-pack] Regular file changed during read: ${path}`)
    }
    const identity = { sha256: hash.digest('hex'), byteLength }
    return contentChunks
      ? { ...identity, content: Buffer.concat(contentChunks, byteLength) }
      : identity
  } finally {
    closeSync(fd)
  }
}

function gitOutput(repoRoot, args) {
  return execFileSync('git', [
    '-c', 'core.quotepath=false',
    '-c', 'color.ui=false',
    '-c', 'diff.noprefix=false',
    '-c', 'diff.mnemonicPrefix=false',
    '-c', 'diff.algorithm=myers',
    ...args
  ], {
    cwd: repoRoot,
    encoding: 'buffer',
    env: { ...process.env, GIT_EXTERNAL_DIFF: '' },
    maxBuffer: PACKAGED_WORKTREE_MAX_GIT_OUTPUT_BYTES,
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function nulSeparatedGitPaths(bytes) {
  if (bytes.length === 0) return []
  const parts = bytes.toString('utf8').split('\0')
  if (parts.at(-1) === '') parts.pop()
  const unique = new Set(parts)
  if (unique.size !== parts.length || parts.length > PACKAGED_WORKTREE_MAX_PATHS) {
    throw new Error('[after-pack] Worktree path inventory is duplicate or exceeds its bound')
  }
  return parts.sort((left, right) => Buffer.compare(Buffer.from(left), Buffer.from(right)))
}

function normalizedRepoPath(path) {
  const value = String(path || '').replaceAll('\\', '/')
  if (!value || value.startsWith('/') || value.includes('\0')) return ''
  const segments = value.split('/')
  if (segments.some((segment) => !segment || segment === '.' || segment === '..')) return ''
  return value
}

function isExcludedPackagedWorktreePath(path) {
  const normalized = normalizedRepoPath(path)
  return !normalized
}

function isPackagedWorktreeSourcePath(path) {
  const normalized = normalizedRepoPath(path)
  return Boolean(normalized) && !isExcludedPackagedWorktreePath(normalized)
}

function exclusionPolicySha256() {
  return domainSeparatedSha256(
    'AnalytixPackagedWorktreeExclusionPolicyV1\0',
    canonicalJSON(PACKAGED_WORKTREE_EXCLUSION_POLICY)
  )
}

function gitChangedPaths(repoRoot, staged) {
  const args = ['diff']
  if (staged) args.push('--cached')
  args.push('--name-only', '-z', '--no-renames', '--diff-filter=ACDMRTUXB')
  const paths = nulSeparatedGitPaths(gitOutput(repoRoot, args))
  for (const path of paths) {
    const normalized = normalizedRepoPath(path)
    if (!normalized || normalized !== path) {
      throw new Error('[after-pack] Tracked worktree path escaped the repository')
    }
    const absolutePath = resolve(repoRoot, path)
    let identity
    try {
      identity = lstatSync(absolutePath)
    } catch (error) {
      if (error?.code === 'ENOENT') continue
      throw error
    }
    if (!identity.isFile() || identity.isSymbolicLink()) {
      throw new Error(`[after-pack] Changed tracked path is not a regular file: ${path}`)
    }
  }
  return paths
}

function gitRawChangeInventory(repoRoot, staged) {
  const args = ['diff']
  if (staged) args.push('--cached')
  args.push(
    '--raw', '-z', '--full-index', '--no-color', '--no-ext-diff', '--no-textconv', '--no-renames'
  )
  return gitOutput(repoRoot, args)
}

function collectGitPatchManifest(repoRoot, staged) {
  const paths = gitChangedPaths(repoRoot, staged)
  const raw = gitRawChangeInventory(repoRoot, staged)
  const entries = []
  let byteLength = raw.length
  if (!staged) {
    for (const path of paths) {
      const absolutePath = resolve(repoRoot, path)
      let identity
      try {
        identity = lstatSync(absolutePath)
      } catch (error) {
        if (error?.code === 'ENOENT') {
          entries.push({ path, state: 'missing' })
          continue
        }
        throw error
      }
      if (!identity.isFile() || identity.isSymbolicLink()) {
        throw new Error(`[after-pack] Changed tracked path is not a regular file: ${path}`)
      }
      const content = hashStableRegularFile(absolutePath, {
        maximumBytes: PACKAGED_WORKTREE_MAX_TRACKED_FILE_BYTES,
        allowEmpty: true
      })
      byteLength += content.byteLength
      entries.push({
        path,
        state: 'regular',
        mode: identity.mode & 0o7777,
        byteLength: content.byteLength,
        sha256: content.sha256
      })
    }
    if (byteLength > PACKAGED_WORKTREE_MAX_TOTAL_TRACKED_BYTES) {
      throw new Error('[after-pack] Worktree change manifest exceeds its byte bound')
    }
  }
  const manifest = {
    schemaVersion: 1,
    contract: PACKAGED_WORKTREE_PATCH_MANIFEST_CONTRACT,
    staged,
    paths,
    raw: {
      sha256: sha256Bytes(raw),
      byteLength: raw.length
    },
    entries
  }
  return {
    count: paths.length,
    byteLength,
    sha256: domainSeparatedSha256(
      PACKAGED_WORKTREE_PATCH_MANIFEST_DOMAIN,
      canonicalJSON(manifest)
    )
  }
}

function collectUntrackedSourceManifest(repoRoot) {
  const paths = nulSeparatedGitPaths(gitOutput(
    repoRoot,
    ['ls-files', '--others', '--exclude-standard', '-z']
  ))
  const entries = []
  let byteLength = 0
  for (const path of paths) {
    if (!isPackagedWorktreeSourcePath(path)) {
      throw new Error('[after-pack] Untracked worktree path is invalid')
    }
    const absolutePath = resolve(repoRoot, path)
    const relativePath = toPosixPath(relative(repoRoot, absolutePath))
    if (relativePath !== path || isAbsolute(relativePath)) {
      throw new Error('[after-pack] Untracked source path escaped the repository')
    }
    const identity = hashStableRegularFile(absolutePath, {
      maximumBytes: PACKAGED_WORKTREE_MAX_FILE_BYTES,
      requireSingleLink: true,
      allowEmpty: true
    })
    byteLength += identity.byteLength
    if (byteLength > PACKAGED_WORKTREE_MAX_TOTAL_UNTRACKED_BYTES) {
      throw new Error('[after-pack] Untracked source manifest exceeds its byte bound')
    }
    const mode = lstatSync(absolutePath).mode & 0o7777
    entries.push({ path, mode, sha256: identity.sha256, byteLength: identity.byteLength })
  }
  const manifest = {
    schemaVersion: 1,
    contract: PACKAGED_WORKTREE_UNTRACKED_MANIFEST_CONTRACT,
    entries
  }
  return {
    count: entries.length,
    byteLength,
    sha256: domainSeparatedSha256(
      PACKAGED_WORKTREE_UNTRACKED_MANIFEST_DOMAIN,
      canonicalJSON(manifest)
    )
  }
}

function packagedWorktreeSnapshotDigest(snapshotWithoutDigest) {
  return domainSeparatedSha256(
    PACKAGED_WORKTREE_SNAPSHOT_DOMAIN,
    canonicalJSON(snapshotWithoutDigest)
  )
}

function isWorktreeManifestSummary(value) {
  return exactKeys(value, ['count', 'byteLength', 'sha256']) &&
    isNonNegativeSafeInteger(value.count) && isNonNegativeSafeInteger(value.byteLength) &&
    isSha256(value.sha256)
}

function isPackagedWorktreeSnapshotV1(value) {
  if (!exactKeys(value, [
    'schemaVersion', 'contract', 'sourceCommit', 'state', 'dirty',
    'exclusionPolicySha256', 'stagedPatch', 'unstagedPatch', 'untrackedSource',
    'snapshotDigest'
  ]) || value.schemaVersion !== 1 || value.contract !== PACKAGED_WORKTREE_SNAPSHOT_CONTRACT ||
    !isGitCommit(value.sourceCommit) || !['clean', 'dirty'].includes(value.state) ||
    typeof value.dirty !== 'boolean' || value.dirty !== (value.state === 'dirty') ||
    !isSha256(value.exclusionPolicySha256) ||
    value.exclusionPolicySha256 !== exclusionPolicySha256() ||
    !isWorktreeManifestSummary(value.stagedPatch) ||
    !isWorktreeManifestSummary(value.unstagedPatch) ||
    !isWorktreeManifestSummary(value.untrackedSource) || !isSha256(value.snapshotDigest)) {
    return false
  }
  const { snapshotDigest, ...withoutDigest } = value
  const dirty = value.stagedPatch.count > 0 || value.unstagedPatch.count > 0 ||
    value.untrackedSource.count > 0
  return value.dirty === dirty &&
    snapshotDigest === packagedWorktreeSnapshotDigest(withoutDigest)
}

function capturePackagedWorktreeSnapshotV1(repoRoot) {
  const sourceCommit = gitOutput(repoRoot, ['rev-parse', '--verify', 'HEAD'])
    .toString('utf8').trim().toLowerCase()
  if (!isGitCommit(sourceCommit)) {
    throw new Error('[after-pack] Repository HEAD is not a supported source commit')
  }
  const stagedPatch = collectGitPatchManifest(repoRoot, true)
  const unstagedPatch = collectGitPatchManifest(repoRoot, false)
  const untrackedSource = collectUntrackedSourceManifest(repoRoot)
  const dirty = stagedPatch.count > 0 || unstagedPatch.count > 0 || untrackedSource.count > 0
  const withoutDigest = {
    schemaVersion: 1,
    contract: PACKAGED_WORKTREE_SNAPSHOT_CONTRACT,
    sourceCommit,
    state: dirty ? 'dirty' : 'clean',
    dirty,
    exclusionPolicySha256: exclusionPolicySha256(),
    stagedPatch,
    unstagedPatch,
    untrackedSource
  }
  return { ...withoutDigest, snapshotDigest: packagedWorktreeSnapshotDigest(withoutDigest) }
}

function collectPackagedWorktreeSnapshotV1(repoRoot = join(__dirname, '..')) {
  const requestedRoot = realpathSync(repoRoot)
  const gitRoot = realpathSync(gitOutput(requestedRoot, ['rev-parse', '--show-toplevel'])
    .toString('utf8').trim())
  if (gitRoot !== requestedRoot) {
    throw new Error('[after-pack] Packaged worktree snapshot requires the exact repository root')
  }
  for (let attempt = 0; attempt < 3; attempt += 1) {
    const first = capturePackagedWorktreeSnapshotV1(requestedRoot)
    const second = capturePackagedWorktreeSnapshotV1(requestedRoot)
    if (canonicalJSON(first) === canonicalJSON(second) && isPackagedWorktreeSnapshotV1(first)) {
      return first
    }
  }
  throw new Error('[after-pack] Worktree changed while the package snapshot was captured')
}

function projectEffectiveBuilderConfig(value, state, depth = 0, key = '') {
  if (depth > EFFECTIVE_BUILDER_CONFIG_MAX_DEPTH || state.nodes >= EFFECTIVE_BUILDER_CONFIG_MAX_NODES) {
    throw new Error('[after-pack] Effective electron-builder config exceeds its structural bound')
  }
  state.nodes += 1
  if (/(?:password|token|secret|api[_-]?key|csc(?:link|keypassword))/iu.test(key)) {
    return value === '' || value == null ? '[redacted-empty]' : '[redacted-present]'
  }
  if (value === null || typeof value === 'boolean' || typeof value === 'string') {
    return value
  }
  if (typeof value === 'number' && Number.isFinite(value) && Number.isSafeInteger(value)) {
    return value
  }
  if (value === undefined) return undefined
  if (Array.isArray(value)) {
    return value.map((entry) => {
      const projected = projectEffectiveBuilderConfig(entry, state, depth + 1)
      if (projected === undefined) {
        throw new Error('[after-pack] Effective electron-builder config array contains undefined')
      }
      return projected
    })
  }
  if (!value || typeof value !== 'object') {
    throw new Error('[after-pack] Effective electron-builder config contains a non-JSON value')
  }
  const prototype = Object.getPrototypeOf(value)
  if (prototype !== Object.prototype && prototype !== null) {
    throw new Error('[after-pack] Effective electron-builder config contains a non-plain value')
  }
  const projected = {}
  for (const name of Object.keys(value)
    .sort((left, right) => Buffer.compare(Buffer.from(left), Buffer.from(right)))) {
    const child = projectEffectiveBuilderConfig(value[name], state, depth + 1, name)
    if (child !== undefined) projected[name] = child
  }
  return projected
}

function effectiveBuilderTargetV1(context) {
  const platform = normalizePlatform(context?.electronPlatformName)
  const goArch = goArchForTarget(context?.arch)
  const arch = goArch === 'amd64' ? 'x64' : goArch
  const rawTargets = Array.isArray(context?.targets)
    ? context.targets
    : context?.targets instanceof Set
      ? [...context.targets]
      : []
  const targetNames = rawTargets.map((target) => String(target?.name || '').trim())
  if (targetNames.some((name) => !name || name.includes('\0')) ||
    new Set(targetNames).size !== targetNames.length) {
    throw new Error('[after-pack] Effective electron-builder target inventory is invalid')
  }
  targetNames.sort((left, right) => Buffer.compare(Buffer.from(left), Buffer.from(right)))
  const target = {
    key: packagedTargetKey(context),
    platform,
    arch,
    targets: targetNames
  }
  return {
    ...target,
    digest: domainSeparatedSha256(
      EFFECTIVE_BUILDER_TARGET_DOMAIN,
      canonicalJSON(target)
    )
  }
}

function collectEffectiveBuilderContextV1(context) {
  if (!context?.packager || !context.packager.config ||
    typeof context.packager.config !== 'object' || Array.isArray(context.packager.config)) {
    throw new Error('[after-pack] Effective electron-builder config is unavailable')
  }
  if (context.packager.config.electronFuses != null) {
    throw new Error('[after-pack] Automatic electron-builder fuse mutation must be disabled')
  }
  const projectedConfig = projectEffectiveBuilderConfig(
    context.packager.config,
    { nodes: 0 }
  )
  const configCanonical = canonicalJSON(projectedConfig)
  if (Buffer.byteLength(configCanonical) > EFFECTIVE_BUILDER_CONFIG_MAX_BYTES) {
    throw new Error('[after-pack] Effective electron-builder config exceeds its byte bound')
  }
  const target = effectiveBuilderTargetV1(context)
  const withoutDigest = {
    schemaVersion: 1,
    contract: EFFECTIVE_BUILDER_CONTEXT_CONTRACT,
    effectiveConfigSha256: domainSeparatedSha256(
      EFFECTIVE_BUILDER_CONFIG_DOMAIN,
      configCanonical
    ),
    target,
    fusePolicyContract: ELECTRON_FUSE_POLICY_V1_CONTRACT,
    fusePolicySha256: electronFusePolicyV1Digest()
  }
  return {
    ...withoutDigest,
    contextDigest: domainSeparatedSha256(
      EFFECTIVE_BUILDER_CONTEXT_DOMAIN,
      canonicalJSON(withoutDigest)
    )
  }
}

function isEffectiveBuilderContextV1(value) {
  if (!exactKeys(value, [
    'schemaVersion', 'contract', 'effectiveConfigSha256', 'target',
    'fusePolicyContract', 'fusePolicySha256', 'contextDigest'
  ]) || value.schemaVersion !== 1 || value.contract !== EFFECTIVE_BUILDER_CONTEXT_CONTRACT ||
    !isSha256(value.effectiveConfigSha256) ||
    value.fusePolicyContract !== ELECTRON_FUSE_POLICY_V1_CONTRACT ||
    value.fusePolicySha256 !== electronFusePolicyV1Digest() || !isSha256(value.contextDigest) ||
    !exactKeys(value.target, ['key', 'platform', 'arch', 'targets', 'digest']) ||
    typeof value.target.key !== 'string' || !value.target.key ||
    !['darwin', 'win32', 'linux'].includes(value.target.platform) ||
    !['arm64', 'x64'].includes(value.target.arch) ||
    value.target.key !== `${value.target.platform}-${value.target.arch}` ||
    !Array.isArray(value.target.targets) || value.target.targets.some((target) =>
      typeof target !== 'string' || !target || target.includes('\0')) ||
    new Set(value.target.targets).size !== value.target.targets.length ||
    canonicalJSON([...value.target.targets].sort((left, right) =>
      Buffer.compare(Buffer.from(left), Buffer.from(right)))) !== canonicalJSON(value.target.targets) ||
    !isSha256(value.target.digest)) {
    return false
  }
  const { digest, ...targetWithoutDigest } = value.target
  if (digest !== domainSeparatedSha256(
    EFFECTIVE_BUILDER_TARGET_DOMAIN,
    canonicalJSON(targetWithoutDigest)
  )) return false
  const { contextDigest, ...withoutDigest } = value
  return contextDigest === domainSeparatedSha256(
    EFFECTIVE_BUILDER_CONTEXT_DOMAIN,
    canonicalJSON(withoutDigest)
  )
}

function consumePackagedAfterExtractSnapshot(context, repoRoot, options = {}) {
  const afterExtract = require('./after-extract.cjs')
  const lifecycle = afterExtract._internals.consumeAfterExtractSnapshot(context, { repoRoot })
  const collectSnapshot = options.collectSnapshot || collectPackagedWorktreeSnapshotV1
  const entrySnapshot = collectSnapshot(repoRoot)
  if (!isPackagedWorktreeSnapshotV1(entrySnapshot)) {
    throw new Error('[after-pack] AfterPack-entry worktree snapshot is invalid')
  }
  const buildContext = (options.collectBuildContext || collectEffectiveBuilderContextV1)(context)
  if (!isEffectiveBuilderContextV1(buildContext) ||
    canonicalJSON(buildContext) !== lifecycle.buildContextCanonical ||
    buildContext.contextDigest !== lifecycle.buildContextDigest) {
    throw new Error('[after-pack] Effective builder context changed after afterExtract')
  }
  if (canonicalJSON(entrySnapshot) !== lifecycle.snapshotCanonical) {
    throw new Error('[after-pack] Source inputs changed between afterExtract and afterPack entry')
  }
  return { lifecycle, entrySnapshot, buildContext }
}

function appBundlePath(context) {
  return join(context.appOutDir, `${context.packager.appInfo.productFilename}.app`)
}

function packedResourcesDir(context) {
  if (normalizePlatform(context.electronPlatformName) === 'darwin') {
    return join(appBundlePath(context), 'Contents', 'Resources')
  }
  return join(context.appOutDir, 'resources')
}

function runtimeServerBinaryName(context) {
  return normalizePlatform(context.electronPlatformName) === 'win32'
    ? 'runtime-server.exe'
    : 'runtime-server'
}

function dataAnalysisNativeBinaryName(context, name) {
  return normalizePlatform(context.electronPlatformName) === 'win32'
    ? `${name}.exe`
    : name
}

function openComputerUseNativeRelativePath(context) {
  const platform = normalizePlatform(context.electronPlatformName)
  const arch = goArchForTarget(context.arch)
  if (platform === 'darwin') {
    return join('dist', 'Analytix Computer Use.app', 'Contents', 'MacOS', 'OpenComputerUse')
  }
  if (platform === 'win32') {
    return join('dist', 'windows', arch === 'amd64' ? 'amd64' : 'arm64', 'analytix-computer-use.exe')
  }
  if (platform === 'linux') {
    return join('dist', 'linux', arch === 'amd64' ? 'amd64' : 'arm64', 'analytix-computer-use')
  }
  throw new Error(`[after-pack] Unsupported Analytix Computer Use target platform: ${context.electronPlatformName}`)
}

function bundledGoRuntimeServerPath(context) {
  return join(packedResourcesDir(context), 'runtime-go', 'bin', runtimeServerBinaryName(context))
}

function bundledDataAnalysisNativeToolPath(context, name) {
  return join(packedResourcesDir(context), 'runtime', dataAnalysisNativeBinaryName(context, name))
}

function bundledDocumentRuntimeRoot(context) {
  return join(packedResourcesDir(context), 'runtime', 'document-runtime')
}

function materializePackagedDocumentRuntime(context, options = {}) {
  return materializeDocumentRuntime({
    repoRoot: options.repoRoot || join(__dirname, '..'),
    resourcesRoot: packedResourcesDir(context),
    platform: normalizePlatform(context.electronPlatformName),
    arch: goArchForTarget(context.arch),
    assetRoot: options.assetRoot,
    authorityLock: options.authorityLock,
    env: options.env || process.env,
    required: options.required === true
  })
}

function verifyPackagedDocumentRuntime(context, options = {}) {
  const root = bundledDocumentRuntimeRoot(context)
  if (!pathEntryExists(root)) {
    if (options.required === true) throw new Error(`[after-pack] ${DOCUMENT_RUNTIME_UNAVAILABLE}`)
    return { available: false, blocker: DOCUMENT_RUNTIME_UNAVAILABLE }
  }
  const target = dataNativeTargetContract(
    normalizePlatform(context.electronPlatformName),
    goArchForTarget(context.arch)
  )
  return {
    available: true,
    ...verifyDocumentRuntime(root, target, { authorityLock: options.authorityLock })
  }
}

function packagedTargetKey(context) {
  const arch = goArchForTarget(context.arch)
  return `${normalizePlatform(context.electronPlatformName)}-${arch === 'amd64' ? 'x64' : arch}`
}

function packagedExecutablePath(context) {
  const platform = normalizePlatform(context.electronPlatformName)
  const productFilename = context.packager?.appInfo?.productFilename
  if (!productFilename) throw new Error('[after-pack] Packaged product filename is unavailable')
  if (platform === 'darwin') {
    return join(appBundlePath(context), 'Contents', 'MacOS', productFilename)
  }
  const executableName = context.packager?.platformSpecificBuildOptions?.executableName ||
    context.packager?.config?.executableName || productFilename
  return join(context.appOutDir, platform === 'win32' ? `${executableName}.exe` : executableName)
}

function packagedAppAsarPath(context) {
  return join(packedResourcesDir(context), 'app.asar')
}

function stagedPayloadExclusionPolicySha256() {
  return domainSeparatedSha256(
    STAGED_PAYLOAD_EXCLUSION_DOMAIN,
    canonicalJSON(STAGED_PAYLOAD_EXCLUSION_POLICY)
  )
}

function stagedPayloadAuthorityRelativePath(context) {
  const path = toPosixPath(relative(
    resolve(context.appOutDir),
    join(packedResourcesDir(context), 'runtime', PACKAGED_BUILD_AUTHORITY_FILE)
  ))
  if (!normalizedRepoPath(path) || isAbsolute(path) || path.startsWith('../')) {
    throw new Error('[after-pack] Packaged authority path escaped the staged payload root')
  }
  return path
}

function hasBundleAncestor(segments, endExclusive) {
  return segments.slice(0, endExclusive).some((segment) =>
    /\.(?:app|appex|framework|xpc)$/iu.test(segment)
  )
}

function isSigningEnvelopePath(context, relativePath) {
  if (normalizePlatform(context.electronPlatformName) !== 'darwin') return false
  const segments = relativePath.split('/')
  const signatureIndex = segments.indexOf('_CodeSignature')
  if (signatureIndex >= 0 && hasBundleAncestor(segments, signatureIndex)) return true
  return segments.at(-1) === 'CodeResources' &&
    segments.at(-2) === 'Contents' && hasBundleAncestor(segments, segments.length - 2)
}

function isExcludedStagedPayloadPath(context, relativePath) {
  return relativePath === stagedPayloadAuthorityRelativePath(context) ||
    isSigningEnvelopePath(context, relativePath)
}

function stableRegularFilePrefix(path, length) {
  const before = lstatSync(path)
  if (!before.isFile() || before.isSymbolicLink()) {
    throw new Error(`[after-pack] Staged payload file is not regular: ${path}`)
  }
  const noFollow = typeof constants.O_NOFOLLOW === 'number' ? constants.O_NOFOLLOW : 0
  const fd = openSync(path, constants.O_RDONLY | noFollow)
  try {
    const opened = fstatSync(fd)
    if (!opened.isFile() || !sameFileIdentity(before, opened)) {
      throw new Error(`[after-pack] Staged payload file changed before inspection: ${path}`)
    }
    const prefix = Buffer.alloc(Math.min(length, opened.size))
    const bytesRead = prefix.length === 0 ? 0 : readSync(fd, prefix, 0, prefix.length, 0)
    const after = fstatSync(fd)
    const pathAfter = lstatSync(path)
    if (bytesRead !== prefix.length || !sameFileIdentity(opened, after) ||
      !sameFileIdentity(after, pathAfter) || pathAfter.isSymbolicLink()) {
      throw new Error(`[after-pack] Staged payload file changed during inspection: ${path}`)
    }
    return prefix
  } finally {
    closeSync(fd)
  }
}

function hasNativeExecutableMagic(prefix) {
  if (prefix.length >= 2 && prefix[0] === 0x4d && prefix[1] === 0x5a) return true
  if (prefix.length >= 4 && prefix[0] === 0x7f && prefix.toString('ascii', 1, 4) === 'ELF') {
    return true
  }
  if (prefix.length < 4) return false
  const magicBE = prefix.readUInt32BE(0)
  const magicLE = prefix.readUInt32LE(0)
  return magicBE === 0xfeedfacf || magicLE === 0xfeedfacf ||
    magicBE === 0xcafebabe || magicBE === 0xcafebabf
}

function stagedNativePayloadBinding(path, before) {
  const stable = readStableRegularFileContent(path, {
    maximumBytes: STAGED_PAYLOAD_MAX_FILE_BYTES
  })
  const bytes = stable.content
  let format
  let arch
  if (bytes.length >= 8 &&
    (bytes.readUInt32BE(0) === 0xcafebabe || bytes.readUInt32BE(0) === 0xcafebabf)) {
    format = 'mach-o'
    arch = 'universal'
  } else if (bytes.length >= 8 &&
    (bytes.readUInt32BE(0) === 0xfeedfacf || bytes.readUInt32LE(0) === 0xfeedfacf)) {
    format = 'mach-o'
    const cpu = bytes.readUInt32BE(0) === 0xfeedfacf
      ? bytes.readUInt32BE(4)
      : bytes.readUInt32LE(4)
    arch = cpu === 0x01000007 ? 'x64' : cpu === 0x0100000c ? 'arm64' : ''
  } else if (bytes.length >= 0x40 && bytes[0] === 0x4d && bytes[1] === 0x5a) {
    format = 'pe'
    const peOffset = bytes.readUInt32LE(0x3c)
    if (peOffset < 0x40 || peOffset + 6 > bytes.length ||
      bytes.toString('ascii', peOffset, peOffset + 4) !== 'PE\0\0') {
      throw new Error(`[after-pack] Staged PE payload header is invalid: ${path}`)
    }
    const machine = bytes.readUInt16LE(peOffset + 4)
    arch = machine === 0x8664 ? 'x64' : machine === 0xaa64 ? 'arm64' : ''
  } else if (bytes.length >= 20 && bytes[0] === 0x7f &&
    bytes.toString('ascii', 1, 4) === 'ELF') {
    format = 'elf'
    const machine = bytes[5] === 2 ? bytes.readUInt16BE(18) : bytes.readUInt16LE(18)
    arch = machine === 62 ? 'x64' : machine === 183 ? 'arm64' : ''
  }
  if (!format || !arch) {
    throw new Error(`[after-pack] Staged native payload format or architecture is unsupported: ${path}`)
  }
  const payload = nativePayloadIdentity(bytes, format)
  const after = lstatSync(path)
  if (!sameFileIdentity(before, after) || after.isSymbolicLink() || !after.isFile() ||
    !isSha256(payload.sha256) || !Number.isSafeInteger(payload.size) || payload.size <= 0) {
    throw new Error(`[after-pack] Staged native payload identity is invalid: ${path}`)
  }
  return {
    payloadSha256: payload.sha256,
    payloadByteLength: payload.size,
    format,
    arch
  }
}

function collectStagedPayloadClosureV1Once(context) {
  const requestedRoot = context?.appOutDir
  if (typeof requestedRoot !== 'string' || !requestedRoot.trim() ||
    !isAbsolute(requestedRoot.trim())) {
    throw new Error('[after-pack] Staged payload root is invalid')
  }
  const root = resolve(requestedRoot.trim())
  const rootStat = lstatSync(root)
  if (!rootStat.isDirectory() || rootStat.isSymbolicLink()) {
    throw new Error('[after-pack] Staged payload root is untrusted')
  }
  const realRoot = realpathSync(root)
  const entries = []
  let contentByteLength = 0
  let directoryCount = 0
  let regularFileCount = 0
  let nativeFileCount = 0
  let symlinkCount = 0

  const register = (entry) => {
    entries.push(entry)
    if (entries.length > STAGED_PAYLOAD_MAX_PATHS) {
      throw new Error('[after-pack] Staged payload closure exceeds its path bound')
    }
  }
  const visit = (absolutePath) => {
    const relativePath = toPosixPath(relative(root, absolutePath))
    if (!normalizedRepoPath(relativePath) || isAbsolute(relativePath) ||
      relativePath.startsWith('../')) {
      throw new Error('[after-pack] Staged payload path escaped its root')
    }
    if (isExcludedStagedPayloadPath(context, relativePath)) return
    const before = lstatSync(absolutePath)
    const mode = before.mode & 0o7777
    if (before.isSymbolicLink()) {
      const target = readlinkSync(absolutePath)
      if (!target || target.includes('\0') || target.length > 4096 || isAbsolute(target)) {
        throw new Error(`[after-pack] Staged payload symlink target is invalid: ${relativePath}`)
      }
      const resolvedTarget = resolve(dirname(absolutePath), target)
      const targetRelative = relative(realRoot, realpathSync(resolvedTarget))
      const after = lstatSync(absolutePath)
      if (targetRelative === '..' || targetRelative.startsWith(`..${sep}`) ||
        isAbsolute(targetRelative) || !sameFileIdentity(before, after) ||
        !after.isSymbolicLink() || readlinkSync(absolutePath) !== target) {
        throw new Error(`[after-pack] Staged payload symlink escaped its root: ${relativePath}`)
      }
      symlinkCount += 1
      register({ path: relativePath, kind: 'symlink', target })
      return
    }
    if (before.isDirectory()) {
      const namesBefore = readdirSync(absolutePath)
        .sort((left, right) => Buffer.compare(Buffer.from(left), Buffer.from(right)))
      directoryCount += 1
      register({ path: relativePath, kind: 'directory', mode })
      for (const name of namesBefore) visit(join(absolutePath, name))
      const after = lstatSync(absolutePath)
      const namesAfter = readdirSync(absolutePath)
        .sort((left, right) => Buffer.compare(Buffer.from(left), Buffer.from(right)))
      if (!sameFileIdentity(before, after) || canonicalJSON(namesBefore) !== canonicalJSON(namesAfter)) {
        throw new Error(`[after-pack] Staged payload directory changed during traversal: ${relativePath}`)
      }
      return
    }
    if (!before.isFile()) {
      throw new Error(`[after-pack] Staged payload contains a special file: ${relativePath}`)
    }
    const prefix = stableRegularFilePrefix(absolutePath, 4)
    if (hasNativeExecutableMagic(prefix)) {
      const binding = stagedNativePayloadBinding(absolutePath, before)
      nativeFileCount += 1
      contentByteLength += binding.payloadByteLength
      register({ path: relativePath, kind: 'native', mode, ...binding })
    } else {
      const binding = hashStableRegularFile(absolutePath, {
        maximumBytes: STAGED_PAYLOAD_MAX_FILE_BYTES,
        allowEmpty: true
      })
      regularFileCount += 1
      contentByteLength += binding.byteLength
      register({ path: relativePath, kind: 'regular', mode, ...binding })
    }
    if (contentByteLength > STAGED_PAYLOAD_MAX_TOTAL_BYTES) {
      throw new Error('[after-pack] Staged payload closure exceeds its byte bound')
    }
  }

  for (const name of readdirSync(root)
    .sort((left, right) => Buffer.compare(Buffer.from(left), Buffer.from(right)))) {
    visit(join(root, name))
  }
  const rootAfter = lstatSync(root)
  if (!rootAfter.isDirectory() || rootAfter.isSymbolicLink() ||
    !sameFileIdentity(rootStat, rootAfter) || realpathSync(root) !== realRoot) {
    throw new Error('[after-pack] Staged payload root changed during traversal')
  }
  entries.sort((left, right) => Buffer.compare(Buffer.from(left.path), Buffer.from(right.path)))
  const exclusionPolicySha256 = stagedPayloadExclusionPolicySha256()
  const manifest = {
    schemaVersion: 1,
    contract: STAGED_PAYLOAD_CONTRACT,
    exclusionPolicySha256,
    entries
  }
  return {
    schemaVersion: 1,
    contract: STAGED_PAYLOAD_CONTRACT,
    exclusionPolicySha256,
    entryCount: entries.length,
    directoryCount,
    regularFileCount,
    nativeFileCount,
    symlinkCount,
    contentByteLength,
    manifestSha256: domainSeparatedSha256(
      STAGED_PAYLOAD_MANIFEST_DOMAIN,
      canonicalJSON(manifest)
    )
  }
}

function collectStagedPayloadClosureV1(context) {
  const first = collectStagedPayloadClosureV1Once(context)
  const second = collectStagedPayloadClosureV1Once(context)
  if (canonicalJSON(first) !== canonicalJSON(second) || !isStagedPayloadClosureV1(first)) {
    throw new Error('[after-pack] Staged payload changed while its closure was captured')
  }
  return first
}

function isStagedPayloadClosureV1(value) {
  return exactKeys(value, [
    'schemaVersion', 'contract', 'exclusionPolicySha256', 'entryCount',
    'directoryCount', 'regularFileCount', 'nativeFileCount', 'symlinkCount',
    'contentByteLength', 'manifestSha256'
  ]) && value.schemaVersion === 1 && value.contract === STAGED_PAYLOAD_CONTRACT &&
    value.exclusionPolicySha256 === stagedPayloadExclusionPolicySha256() &&
    isNonNegativeSafeInteger(value.entryCount) &&
    isNonNegativeSafeInteger(value.directoryCount) &&
    isNonNegativeSafeInteger(value.regularFileCount) &&
    isNonNegativeSafeInteger(value.nativeFileCount) &&
    isNonNegativeSafeInteger(value.symlinkCount) &&
    value.entryCount === value.directoryCount + value.regularFileCount +
      value.nativeFileCount + value.symlinkCount &&
    isNonNegativeSafeInteger(value.contentByteLength) && isSha256(value.manifestSha256)
}

function electronFuseFilePath(context) {
  if (normalizePlatform(context.electronPlatformName) === 'darwin') {
    return join(
      appBundlePath(context),
      'Contents',
      'Frameworks',
      'Electron Framework.framework',
      'Electron Framework'
    )
  }
  return packagedExecutablePath(context)
}

function canonicalElectronFrameworkFuseTargetV1(context) {
  const frameworkRoot = join(
    appBundlePath(context),
    'Contents',
    'Frameworks',
    'Electron Framework.framework'
  )
  const versionsRoot = join(frameworkRoot, 'Versions')
  const currentAlias = join(versionsRoot, 'Current')
  const binaryAlias = electronFuseFilePath(context)
  let frameworkBefore
  let versionsBefore
  let currentBefore
  let binaryAliasBefore
  try {
    frameworkBefore = lstatSync(frameworkRoot)
    versionsBefore = lstatSync(versionsRoot)
    currentBefore = lstatSync(currentAlias)
    binaryAliasBefore = lstatSync(binaryAlias)
  } catch {
    throw new Error('[after-pack] Electron framework layout is incomplete')
  }
  const canonicalFrameworkRoot = realpathSync(frameworkRoot)
  if (!frameworkBefore.isDirectory() || frameworkBefore.isSymbolicLink() ||
    !versionsBefore.isDirectory() || versionsBefore.isSymbolicLink() ||
    !currentBefore.isSymbolicLink() || !binaryAliasBefore.isSymbolicLink() ||
    canonicalFrameworkRoot !== resolve(frameworkRoot) ||
    readlinkSync(binaryAlias) !== 'Versions/Current/Electron Framework') {
    throw new Error('[after-pack] Electron framework layout is not canonical')
  }

  const version = readlinkSync(currentAlias)
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/u.test(version) || version === 'Current') {
    throw new Error('[after-pack] Electron framework version link is invalid')
  }
  const versionRoot = join(versionsRoot, version)
  const expectedTarget = join(versionRoot, 'Electron Framework')
  let versionBefore
  let targetBefore
  try {
    versionBefore = lstatSync(versionRoot)
    targetBefore = lstatSync(expectedTarget)
  } catch {
    throw new Error('[after-pack] Electron framework version payload is incomplete')
  }
  const canonicalTarget = realpathSync(binaryAlias)
  const relativeTarget = relative(canonicalFrameworkRoot, canonicalTarget)
  const relativeSegments = relativeTarget.split(sep)
  if (!versionBefore.isDirectory() || versionBefore.isSymbolicLink() ||
    realpathSync(versionRoot) !== resolve(versionRoot) ||
    canonicalTarget !== resolve(expectedTarget) || realpathSync(expectedTarget) !== canonicalTarget ||
    relativeSegments.length !== 3 || relativeSegments[0] !== 'Versions' ||
    relativeSegments[1] !== version || relativeSegments[2] !== 'Electron Framework' ||
    isAbsolute(relativeTarget) || relativeTarget.startsWith(`..${sep}`)) {
    throw new Error('[after-pack] Electron framework binary escaped its canonical version')
  }
  if (!targetBefore.isFile() || targetBefore.isSymbolicLink() ||
    targetBefore.nlink !== 1 || targetBefore.size <= 0) {
    throw new Error('[after-pack] Electron framework target is not a single-link regular file')
  }

  const frameworkAfter = lstatSync(frameworkRoot)
  const versionsAfter = lstatSync(versionsRoot)
  const currentAfter = lstatSync(currentAlias)
  const binaryAliasAfter = lstatSync(binaryAlias)
  const versionAfter = lstatSync(versionRoot)
  const targetAfter = lstatSync(canonicalTarget)
  if (!sameFileIdentity(frameworkBefore, frameworkAfter) ||
    !sameFileIdentity(versionsBefore, versionsAfter) ||
    !sameFileIdentity(currentBefore, currentAfter) ||
    !sameFileIdentity(binaryAliasBefore, binaryAliasAfter) ||
    !sameFileIdentity(versionBefore, versionAfter) ||
    !sameFileIdentity(targetBefore, targetAfter) ||
    readlinkSync(currentAlias) !== version ||
    readlinkSync(binaryAlias) !== 'Versions/Current/Electron Framework' ||
    realpathSync(frameworkRoot) !== canonicalFrameworkRoot ||
    realpathSync(binaryAlias) !== canonicalTarget) {
    throw new Error('[after-pack] Electron framework layout changed during resolution')
  }
  return {
    path: canonicalTarget,
    version,
    frameworkIdentity: frameworkAfter,
    versionsIdentity: versionsAfter,
    currentIdentity: currentAfter,
    binaryAliasIdentity: binaryAliasAfter,
    versionIdentity: versionAfter,
    targetIdentity: targetAfter
  }
}

function sameElectronFrameworkFuseTargetV1(left, right) {
  return left.path === right.path && left.version === right.version &&
    sameFileIdentity(left.frameworkIdentity, right.frameworkIdentity) &&
    sameFileIdentity(left.versionsIdentity, right.versionsIdentity) &&
    sameFileIdentity(left.currentIdentity, right.currentIdentity) &&
    sameFileIdentity(left.binaryAliasIdentity, right.binaryAliasIdentity) &&
    sameFileIdentity(left.versionIdentity, right.versionIdentity) &&
    sameFileIdentity(left.targetIdentity, right.targetIdentity)
}

function verifyElectronV8SnapshotInventoryV1(context) {
  const platform = normalizePlatform(context.electronPlatformName)
  const frameworkTarget = platform === 'darwin'
    ? canonicalElectronFrameworkFuseTargetV1(context)
    : null
  const snapshotRoot = frameworkTarget
    ? join(dirname(frameworkTarget.path), 'Resources')
    : context.appOutDir
  let rootBefore
  try {
    rootBefore = lstatSync(snapshotRoot)
  } catch {
    throw new Error('[after-pack] Electron V8 snapshot root is unavailable')
  }
  if (!rootBefore.isDirectory() || rootBefore.isSymbolicLink() ||
    realpathSync(snapshotRoot) !== resolve(snapshotRoot)) {
    throw new Error('[after-pack] Electron V8 snapshot root is not canonical')
  }

  const entries = readdirSync(snapshotRoot, { withFileTypes: true })
  const standardPattern = platform === 'darwin'
    ? /^v8_context_snapshot(?:\.(?:arm64|x86_64))?\.bin$/u
    : /^v8_context_snapshot\.bin$/u
  const standardEntries = entries.filter((entry) => standardPattern.test(entry.name))
  const browserEntries = entries.filter((entry) => entry.name === 'browser_v8_context_snapshot.bin')
  if (standardEntries.length !== 1 || browserEntries.length > 1) {
    throw new Error('[after-pack] Electron V8 snapshot inventory is ambiguous')
  }
  if (!standardEntries[0].isFile() ||
    (browserEntries[0] && !browserEntries[0].isFile())) {
    throw new Error('[after-pack] Electron V8 snapshot inventory is not regular')
  }

  const browserProcessSpecific = ELECTRON_FUSE_POLICY_V1[6] === true
  if (browserProcessSpecific !== (browserEntries.length === 1)) {
    throw new Error('[after-pack] Electron V8 snapshot inventory conflicts with fuse policy')
  }
  const standard = hashStableRegularFile(join(snapshotRoot, standardEntries[0].name), {
    maximumBytes: STAGED_PAYLOAD_MAX_FILE_BYTES,
    requireSingleLink: true
  })
  const browser = browserEntries.length === 1
    ? hashStableRegularFile(join(snapshotRoot, browserEntries[0].name), {
        maximumBytes: STAGED_PAYLOAD_MAX_FILE_BYTES,
        requireSingleLink: true
      })
    : null
  const rootAfter = lstatSync(snapshotRoot)
  if (!sameFileIdentity(rootBefore, rootAfter) ||
    realpathSync(snapshotRoot) !== resolve(snapshotRoot) ||
    (frameworkTarget && !sameElectronFrameworkFuseTargetV1(
      frameworkTarget,
      canonicalElectronFrameworkFuseTargetV1(context)
    ))) {
    throw new Error('[after-pack] Electron V8 snapshot inventory changed during verification')
  }
  return {
    root: snapshotRoot,
    standardFileName: standardEntries[0].name,
    standardSha256: standard.sha256,
    standardByteLength: standard.byteLength,
    browserProcessSpecific,
    browserFileName: browserEntries[0]?.name || '',
    browserSha256: browser?.sha256 || '',
    browserByteLength: browser?.byteLength || 0
  }
}

function verifyElectronFusePolicyV1(context) {
  verifyElectronV8SnapshotInventoryV1(context)
  const frameworkTarget = normalizePlatform(context.electronPlatformName) === 'darwin'
    ? canonicalElectronFrameworkFuseTargetV1(context)
    : null
  const fuseFilePath = frameworkTarget?.path || electronFuseFilePath(context)
  const stable = readStableRegularFileContent(fuseFilePath, {
    maximumBytes: STAGED_PAYLOAD_MAX_FILE_BYTES
  })
  if (frameworkTarget && !sameElectronFrameworkFuseTargetV1(
    frameworkTarget,
    canonicalElectronFrameworkFuseTargetV1(context)
  )) {
    throw new Error('[after-pack] Electron framework changed during fuse verification')
  }
  const sentinelOffsets = []
  for (let offset = stable.content.indexOf(ELECTRON_FUSE_SENTINEL);
    offset >= 0;
    offset = stable.content.indexOf(ELECTRON_FUSE_SENTINEL, offset + ELECTRON_FUSE_SENTINEL.length)) {
    sentinelOffsets.push(offset)
    if (sentinelOffsets.length > 2) {
      throw new Error('[after-pack] Electron fuse sentinel inventory is invalid')
    }
  }
  if (sentinelOffsets.length === 0) {
    throw new Error('[after-pack] Electron FuseV1 sentinel is missing')
  }
  for (const sentinelOffset of sentinelOffsets) {
    const wireOffset = sentinelOffset + ELECTRON_FUSE_SENTINEL.length
    if (wireOffset + 2 > stable.content.length || stable.content[wireOffset] !== 1) {
      throw new Error('[after-pack] Electron FuseV1 wire version is invalid')
    }
    const wireLength = stable.content[wireOffset + 1]
    if (wireLength !== ELECTRON_FUSE_V1_WIRE_LENGTH) {
      throw new Error('[after-pack] Electron FuseV1 wire length is invalid')
    }
    for (let index = 0; index < ELECTRON_FUSE_V1_WIRE_LENGTH; index += 1) {
      const enabled = ELECTRON_FUSE_POLICY_V1[index]
      const expected = enabled ? ELECTRON_FUSE_STATE_ENABLED : ELECTRON_FUSE_STATE_DISABLED
      if (typeof enabled !== 'boolean' || stable.content[wireOffset + 2 + index] !== expected) {
        throw new Error(`[after-pack] Electron FuseV1 policy mismatch at wire index ${index}`)
      }
    }
  }
  return {
    contract: ELECTRON_FUSE_POLICY_V1_CONTRACT,
    policySha256: electronFusePolicyV1Digest(),
    wireCount: sentinelOffsets.length
  }
}

async function applyElectronFusePolicyV1(context) {
  if (context?.packager?.config?.electronFuses != null) {
    throw new Error('[after-pack] Automatic electron-builder fuse mutation must be disabled')
  }
  const { flipFuses } = await import('@electron/fuses')
  if (typeof flipFuses !== 'function') {
    throw new Error('[after-pack] Official Electron fuse mutator is unavailable')
  }
  verifyElectronV8SnapshotInventoryV1(context)
  const resetAdHocDarwinSignature = normalizePlatform(context.electronPlatformName) === 'darwin' &&
    goArchForTarget(context.arch) === 'arm64'
  const mutatedWireCount = await flipFuses(appBundlePath(context), {
    ...electronFusePolicyV1(),
    resetAdHocDarwinSignature
  })
  const verified = verifyElectronFusePolicyV1(context)
  if (mutatedWireCount !== verified.wireCount) {
    throw new Error('[after-pack] Electron fuse mutation count is invalid')
  }
  return verified
}

function packagedBuildAuthorityDigest(authorityWithoutDigest) {
  return domainSeparatedSha256(
    PACKAGED_BUILD_AUTHORITY_DOMAIN,
    canonicalJSON(authorityWithoutDigest)
  )
}

function isContentArtifactBinding(value) {
  return exactKeys(value, ['sha256', 'byteLength']) && isSha256(value.sha256) &&
    Number.isSafeInteger(value.byteLength) && value.byteLength > 0
}

function isNativeArtifactBinding(value) {
  return exactKeys(value, [
    'preSignSha256', 'preSignByteLength', 'payloadSha256', 'payloadByteLength',
    'format', 'arch'
  ]) && isSha256(value.preSignSha256) && isSha256(value.payloadSha256) &&
    Number.isSafeInteger(value.preSignByteLength) && value.preSignByteLength > 0 &&
    Number.isSafeInteger(value.payloadByteLength) && value.payloadByteLength > 0 &&
    value.payloadByteLength <= value.preSignByteLength &&
    ['mach-o', 'pe', 'elf'].includes(value.format) &&
    ['arm64', 'x64'].includes(value.arch)
}

function sameNativeInspection(left, right) {
  return left.sha256 === right.sha256 && left.size === right.size &&
    left.payloadSha256 === right.payloadSha256 && left.payloadSize === right.payloadSize &&
    left.format === right.format && left.arch === right.arch
}

function signingInvariantNativeArtifactBinding(path, target) {
  const first = assertNativeBinaryTarget(path, target)
  const second = assertNativeBinaryTarget(path, target)
  if (!sameNativeInspection(first, second)) {
    throw new Error(`[after-pack] Native artifact changed between identity samples: ${path}`)
  }
  return {
    preSignSha256: first.sha256,
    preSignByteLength: first.size,
    payloadSha256: first.payloadSha256,
    payloadByteLength: first.payloadSize,
    format: first.format,
    arch: first.arch
  }
}

function isControlledReleaseNativeDisposition(value) {
  if (!exactKeys(value, [
    'kind', 'targetKey', 'receiptSha256', 'manifestSha256',
    'signingPolicySha256', 'signingMode', 'appleTeamIdentifier'
  ]) || value.kind !== NATIVE_DISPOSITION_CONTROLLED_RELEASE ||
    typeof value.targetKey !== 'string' || !value.targetKey ||
    !isSha256(value.receiptSha256) || !isSha256(value.manifestSha256)) {
    return false
  }
  if (value.targetKey.startsWith('darwin-')) {
    return isSha256(value.signingPolicySha256) &&
      ['ad-hoc', 'developer-id'].includes(value.signingMode) &&
      (value.signingMode === 'ad-hoc'
        ? value.appleTeamIdentifier === ''
        : /^[A-Z0-9]{10}$/u.test(value.appleTeamIdentifier))
  }
  return value.signingPolicySha256 === '' && value.signingMode === '' &&
    value.appleTeamIdentifier === ''
}

function isDevelopmentNativeComponentBinding(value) {
  return exactKeys(value, [
    'id', 'binaryName', 'markerBinarySha256', 'markerBinaryByteLength',
    'payloadSha256', 'payloadByteLength', 'format', 'arch'
  ]) && typeof value.id === 'string' && value.id.length > 0 &&
    typeof value.binaryName === 'string' && value.binaryName.length > 0 &&
    isSha256(value.markerBinarySha256) && isSha256(value.payloadSha256) &&
    Number.isSafeInteger(value.markerBinaryByteLength) && value.markerBinaryByteLength > 0 &&
    Number.isSafeInteger(value.payloadByteLength) && value.payloadByteLength > 0 &&
    value.payloadByteLength <= value.markerBinaryByteLength &&
    ['mach-o', 'pe', 'elf'].includes(value.format) && ['arm64', 'x64'].includes(value.arch)
}

function isDevelopmentNativeDisposition(value) {
  if (!exactKeys(value, ['kind', 'targetKey', 'marker', 'components']) ||
    value.kind !== NATIVE_DISPOSITION_DEVELOPMENT ||
    typeof value.targetKey !== 'string' || value.targetKey.length === 0 ||
    !isContentArtifactBinding(value.marker) || !Array.isArray(value.components) ||
    value.components.length !== nativeComponentManifest.components.length ||
    !value.components.every(isDevelopmentNativeComponentBinding)) {
    return false
  }
  const targetKeyMatch = /^(darwin|linux|win32)-(arm64|x64)$/u.exec(value.targetKey)
  if (!targetKeyMatch) return false
  let target
  try {
    target = dataNativeTargetContract(targetKeyMatch[1], targetKeyMatch[2])
  } catch {
    return false
  }
  return target.key === value.targetKey && value.components.every((component, index) => {
    const expected = nativeComponentManifest.components[index]
    return component.id === expected.id &&
      component.binaryName === nativeBinaryName(expected, target.platform) &&
      component.format === target.format && component.arch === target.arch
  })
}

function isControlledCoreDisposition(value) {
  return exactKeys(value, ['kind', 'targetKey', 'signingPolicySha256', 'signingMode', 'appleTeamIdentifier']) &&
    value.kind === CORE_CONTROLLED_DISPOSITION && value.targetKey === 'darwin-arm64' &&
    isSha256(value.signingPolicySha256) && value.signingMode === 'developer-id' &&
    /^[A-Z0-9]{10}$/u.test(value.appleTeamIdentifier)
}

function isNativeDisposition(value) {
  return isControlledCoreDisposition(value) || isControlledReleaseNativeDisposition(value) || isDevelopmentNativeDisposition(value) ||
    (exactKeys(value, ['kind', 'targetKey']) && value.kind === CORE_DISPOSITION && value.targetKey === 'darwin-arm64')
}

function normalizeNativeDisposition(value) {
  if (isNativeDisposition(value)) return value
  const controlled = {
    kind: NATIVE_DISPOSITION_CONTROLLED_RELEASE,
    targetKey: value?.targetKey,
    receiptSha256: value?.receiptSHA256,
    manifestSha256: value?.manifestSHA256,
    signingPolicySha256: value?.signingPolicySHA256 || '',
    signingMode: value?.signingMode || '',
    appleTeamIdentifier: value?.appleTeamIdentifier || ''
  }
  if (!isControlledReleaseNativeDisposition(controlled)) {
    throw new Error('[after-pack] Native disposition is invalid')
  }
  return controlled
}

function authorityClassification(worktreeSnapshot, nativeDisposition) {
  if (nativeDisposition.kind === NATIVE_DISPOSITION_DEVELOPMENT || nativeDisposition.kind === CORE_DISPOSITION) {
    return worktreeSnapshot.dirty
      ? 'development_dirty_non_publishable'
      : 'development_clean_non_publishable'
  }
  return worktreeSnapshot.dirty
    ? 'controlled_release_dirty_non_publishable'
    : 'controlled_release_clean_candidate_non_publishable'
}

function validProfileFundsBinding(disposition, binding) {
  return isCoreDisposition(disposition)
    ? canonicalJSON(binding) === canonicalJSON(ABSENT_FUNDS)
    : isFundsPluginArtifactBindingV2(binding)
}

function isPackagedBuildAuthorityV2(value) {
  if (!exactKeys(value, [
    'schemaVersion', 'contract', 'sourceCommit', 'worktreeSnapshot', 'classification',
    'publishable', 'releaseEligible', 'publicationReceiptIssued', 'targetKey',
    'buildContext', 'nativeDisposition', 'artifacts', 'stagedPayload', 'authorityDigest'
  ]) || value.schemaVersion !== 2 || value.contract !== PACKAGED_BUILD_AUTHORITY_CONTRACT ||
    !isGitCommit(value.sourceCommit) || !isPackagedWorktreeSnapshotV1(value.worktreeSnapshot) ||
    value.sourceCommit !== value.worktreeSnapshot.sourceCommit ||
    !isEffectiveBuilderContextV1(value.buildContext) ||
    !isNativeDisposition(value.nativeDisposition) ||
    value.classification !== authorityClassification(value.worktreeSnapshot, value.nativeDisposition) ||
    value.publishable !== false || value.releaseEligible !== false ||
    value.publicationReceiptIssued !== false ||
    typeof value.targetKey !== 'string' || !value.targetKey ||
    value.buildContext.target.key !== value.targetKey ||
    value.nativeDisposition.targetKey !== value.targetKey ||
    !exactKeys(value.artifacts, ['executable', 'appAsar', 'runtimeServer', 'fundsPlugin']) ||
    !isNativeArtifactBinding(value.artifacts.executable) ||
    !isContentArtifactBinding(value.artifacts.appAsar) ||
    !isNativeArtifactBinding(value.artifacts.runtimeServer) ||
    !validProfileFundsBinding(value.nativeDisposition, value.artifacts.fundsPlugin) ||
    !isStagedPayloadClosureV1(value.stagedPayload) || !isSha256(value.authorityDigest)) {
    return false
  }
  const { authorityDigest, ...withoutDigest } = value
  return authorityDigest === packagedBuildAuthorityDigest(withoutDigest)
}

function createPackagedBuildAuthorityV2(options) {
  const worktreeSnapshot = options.worktreeSnapshot
  const buildContext = options.buildContext
  const nativeDisposition = normalizeNativeDisposition(options.nativeDisposition)
  if (!isPackagedWorktreeSnapshotV1(worktreeSnapshot)) {
    throw new Error('[after-pack] Packaged worktree snapshot is invalid')
  }
  if (!isEffectiveBuilderContextV1(buildContext) ||
    typeof options.targetKey !== 'string' || !options.targetKey ||
    buildContext.target.key !== options.targetKey ||
    nativeDisposition.targetKey !== options.targetKey ||
    !exactKeys(options.artifacts, ['executable', 'appAsar', 'runtimeServer', 'fundsPlugin']) ||
    !isNativeArtifactBinding(options.artifacts.executable) ||
    !isContentArtifactBinding(options.artifacts.appAsar) ||
    !isNativeArtifactBinding(options.artifacts.runtimeServer) ||
    !validProfileFundsBinding(nativeDisposition, options.artifacts.fundsPlugin) ||
    !isStagedPayloadClosureV1(options.stagedPayload)) {
    throw new Error('[after-pack] Packaged build authority inputs are invalid')
  }
  const withoutDigest = {
    schemaVersion: 2,
    contract: PACKAGED_BUILD_AUTHORITY_CONTRACT,
    sourceCommit: worktreeSnapshot.sourceCommit,
    worktreeSnapshot,
    classification: authorityClassification(worktreeSnapshot, nativeDisposition),
    publishable: false,
    releaseEligible: false,
    publicationReceiptIssued: false,
    targetKey: options.targetKey,
    buildContext,
    nativeDisposition,
    artifacts: options.artifacts,
    stagedPayload: options.stagedPayload
  }
  return { ...withoutDigest, authorityDigest: packagedBuildAuthorityDigest(withoutDigest) }
}

function formalPackagedReleaseIntent(env = process.env) {
  return env.ANALYTIX_PACKAGED_REQUIRE_PUBLISHABLE === '1' ||
    env.ANALYTIX_RELEASE_BUILD === '1' || env.MAC_SIGN === '1' ||
    Boolean(env.CSC_LINK || env.CSC_NAME || env.CSC_KEY_PASSWORD || env.WIN_CSC_LINK ||
      env.ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1)
}

// Private Office admission is a build-only disposition. The runtime never reads
// these variables; it verifies the installed resource seal and qualified bytes.
function privateOfficeBuildRequested(context, nativeDisposition, env = process.env) {
  const requested = env.ANALYTIX_OFFICE_PRIVATE_LOCAL_BUILD
  const assetRoot = env.ANALYTIX_OFFICE_PRIVATE_LOCAL_ASSET_ROOT
  if (requested == null && assetRoot == null) return false
  if (requested !== '1' || typeof assetRoot !== 'string' || !isAbsolute(assetRoot) ||
    resolve(assetRoot) !== assetRoot || packagedTargetKey(context) !== 'darwin-arm64' ||
    nativeDisposition.kind !== NATIVE_DISPOSITION_DEVELOPMENT || formalPackagedReleaseIntent(env) ||
    env.ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE !== 'isolated-local-v1') {
    throw new Error('[after-pack] office_private_local_build_scope_invalid')
  }
  return true
}
function stagePrivateOfficeForPack(context, nativeDisposition, repoRoot, snapshot, env = process.env) {
  const root = join(packedResourcesDir(context), 'office-private')
  const requested = privateOfficeBuildRequested(context, nativeDisposition, env)
  if (pathEntryExists(root)) throw new Error('[after-pack] office_private_local_payload_already_present')
  if (!requested) return
  return stagePrivateLocal({repoRoot,assetRoot:env.ANALYTIX_OFFICE_PRIVATE_LOCAL_ASSET_ROOT,
    destination:root,worktreeSnapshot:snapshot,targetKey:packagedTargetKey(context)})
}
function verifyPrivateOfficeForPack(context, nativeDisposition, snapshot, env = process.env) {
  const root = join(packedResourcesDir(context), 'office-private')
  const requested = privateOfficeBuildRequested(context, nativeDisposition, env)
  if (!requested) {
    if (pathEntryExists(root)) throw new Error('[after-pack] office_private_local_payload_not_authorized')
    return
  }
  return verifyPrivateLocal(root,{sourceCommit:snapshot.sourceCommit,
    worktreeSnapshotDigest:snapshot.snapshotDigest,targetKey:packagedTargetKey(context)})
}

function syncRegularFile(path) {
  const fd = openSync(path, constants.O_RDONLY)
  try {
    fsyncSync(fd)
  } finally {
    closeSync(fd)
  }
}

function sameFilesystemObject(left, right) {
  return left.dev === right.dev && left.ino === right.ino
}

function publishRegularFileNoReplace(sourcePath, destinationPath) {
  const source = lstatSync(sourcePath)
  if (!source.isFile() || source.isSymbolicLink()) {
    throw new Error('[after-pack] Packaged build authority temporary file is untrusted')
  }
  try {
    linkSync(sourcePath, destinationPath)
  } catch (error) {
    if (error?.code === 'EEXIST') {
      throw new Error('[after-pack] Packaged build authority already exists')
    }
    throw new Error('[after-pack] Packaged build authority atomic publication failed')
  }
  return Object.freeze({ dev: source.dev, ino: source.ino })
}

function removePublishedAuthorityIfOwned(path, ownership, expectedIdentity) {
  if (!ownership) return false
  let current
  try {
    current = lstatSync(path)
  } catch (error) {
    if (error?.code === 'ENOENT') return false
    throw error
  }
  if (!current.isFile() || current.isSymbolicLink() ||
    current.dev !== ownership.dev || current.ino !== ownership.ino) {
    return false
  }
  let stable
  try {
    stable = hashStableRegularFile(path, { maximumBytes: 256 * 1024 })
  } catch {
    return false
  }
  if (stable.sha256 !== expectedIdentity.sha256 ||
    stable.byteLength !== expectedIdentity.byteLength) {
    return false
  }
  unlinkSync(path)
  syncDirectory(dirname(path))
  return true
}

function removeOwnedTemporaryFile(path, ownership) {
  if (!ownership) return
  let current
  try {
    current = lstatSync(path)
  } catch (error) {
    if (error?.code === 'ENOENT') return
    throw error
  }
  if (!current.isFile() || current.isSymbolicLink() ||
    current.dev !== ownership.dev || current.ino !== ownership.ino) {
    throw new Error('[after-pack] Packaged build authority temporary ownership changed')
  }
  unlinkSync(path)
  syncDirectory(dirname(path))
}

function writePackagedBuildAuthorityV2(context, nativeTrust, options = {}) {
  const targetKey = packagedTargetKey(context)
  const nativeDisposition = normalizeNativeDisposition(nativeTrust)
  if (isCoreDisposition(nativeDisposition) !== isCoreContext(context)) throw Error('core_profile_authority_mismatch')
  if (nativeDisposition.targetKey !== targetKey) {
    throw new Error('[after-pack] Native disposition does not match the packaged authority target')
  }
  if (formalPackagedReleaseIntent(options.env || process.env) &&
    nativeDisposition.kind !== NATIVE_DISPOSITION_CONTROLLED_RELEASE && !isControlledCoreDisposition(nativeDisposition)) {
    throw new Error('[after-pack] packaged_development_native_disposition_forbidden_for_release')
  }
  const runtimeDir = join(packedResourcesDir(context), 'runtime')
  const runtimeDirStat = lstatSync(runtimeDir)
  if (!runtimeDirStat.isDirectory() || runtimeDirStat.isSymbolicLink()) {
    throw new Error('[after-pack] Packaged runtime authority directory is untrusted')
  }
  if (nativeDisposition.kind === NATIVE_DISPOSITION_CONTROLLED_RELEASE) {
    const nativeReceiptPath = join(runtimeDir, RECEIPT_FILE_NAME)
    const nativeReceipt = hashStableRegularFile(nativeReceiptPath, { maximumBytes: 1024 * 1024 })
    if (nativeReceipt.sha256 !== nativeDisposition.receiptSha256) {
      throw new Error('[after-pack] Native receipt changed before packaged authority issuance')
    }
  } else if (isCoreDisposition(nativeDisposition)) {
    if (!isCoreContext(context)) throw Error('core_profile_authority_mismatch')
    assertCoreResourcesAbsent(packedResourcesDir(context))
  } else {
    verifyPackagedDevelopmentNativeDisposition(context, nativeDisposition, {
      repoRoot: options.repoRoot,
      verifyCurrentSource: options.verifyCurrentSource,
      requireMarkerBinaryIdentity: true
    })
  }
  const worktreeSnapshot = options.worktreeSnapshot ||
    collectPackagedWorktreeSnapshotV1(options.repoRoot || join(__dirname, '..'))
  if (formalPackagedReleaseIntent(options.env || process.env) && worktreeSnapshot.dirty) {
    throw new Error('[after-pack] Formal packaged release requires a clean Git worktree')
  }
  verifyPrivateOfficeForPack(context, nativeDisposition, worktreeSnapshot, options.env || process.env)
  const buildContext = options.buildContext || collectEffectiveBuilderContextV1(context)
  const stagedPayload = options.stagedPayload || collectStagedPayloadClosureV1(context)
  const expectedFundsPluginAdmission = options.fundsPluginAdmission
  let currentFundsPluginAdmission = requireCurrentFundsPluginAdmissionV1(
    context,
    expectedFundsPluginAdmission
  )
  if (options.afterFundsPluginAdmission != null) {
    if (typeof options.afterFundsPluginAdmission !== 'function') {
      throw new Error('[after-pack] funds_plugin_admission_changed')
    }
    options.afterFundsPluginAdmission()
    currentFundsPluginAdmission = requireCurrentFundsPluginAdmissionV1(
      context,
      expectedFundsPluginAdmission
    )
  }
  const target = dataNativeTargetContract(
    normalizePlatform(context.electronPlatformName),
    goArchForTarget(context.arch)
  )
  const authority = createPackagedBuildAuthorityV2({
    worktreeSnapshot,
    targetKey,
    buildContext,
    nativeDisposition,
    artifacts: {
      executable: signingInvariantNativeArtifactBinding(packagedExecutablePath(context), target),
      appAsar: hashStableRegularFile(packagedAppAsarPath(context), {
        requireSingleLink: true
      }),
      runtimeServer: signingInvariantNativeArtifactBinding(bundledGoRuntimeServerPath(context), target),
      fundsPlugin: currentFundsPluginAdmission.artifact
    },
    stagedPayload
  })
  currentFundsPluginAdmission = requireCurrentFundsPluginAdmissionV1(
    context,
    expectedFundsPluginAdmission
  )
  if (canonicalJSON(authority.artifacts.fundsPlugin) !==
    canonicalJSON(currentFundsPluginAdmission.artifact)) {
    throw new Error('[after-pack] funds_plugin_admission_changed')
  }
  const authorityPath = join(runtimeDir, PACKAGED_BUILD_AUTHORITY_FILE)
  if (existsSync(authorityPath)) {
    const existing = lstatSync(authorityPath)
    if (!existing.isFile() || existing.isSymbolicLink()) {
      throw new Error('[after-pack] Existing packaged build authority is untrusted')
    }
  }
  if (options.afterPackagedBuildAuthorityPrecheck != null) {
    if (typeof options.afterPackagedBuildAuthorityPrecheck !== 'function') {
      throw new Error('[after-pack] Packaged build authority publication hook is invalid')
    }
    options.afterPackagedBuildAuthorityPrecheck()
  }
  const authorityText = canonicalJSON(authority)
  const temporaryPath = join(
    runtimeDir,
    `.${PACKAGED_BUILD_AUTHORITY_FILE}.${process.pid}.${randomUUID()}.tmp`
  )
  let temporaryOwnership
  let publishedOwnership
  let temporaryIdentity
  try {
    writeFileSync(temporaryPath, authorityText, { encoding: 'utf8', flag: 'wx', mode: 0o600 })
    const temporaryStat = lstatSync(temporaryPath)
    if (!temporaryStat.isFile() || temporaryStat.isSymbolicLink()) {
      throw new Error('[after-pack] Packaged build authority temporary file is untrusted')
    }
    temporaryOwnership = Object.freeze({ dev: temporaryStat.dev, ino: temporaryStat.ino })
    syncRegularFile(temporaryPath)
    temporaryIdentity = hashStableRegularFile(temporaryPath, { maximumBytes: 256 * 1024 })
    if (temporaryIdentity.sha256 !== sha256Bytes(authorityText) ||
      temporaryIdentity.byteLength !== Buffer.byteLength(authorityText)) {
      throw new Error('[after-pack] Packaged build authority temporary readback failed')
    }
    requireCurrentFundsPluginAdmissionV1(context, expectedFundsPluginAdmission)
    publishedOwnership = publishRegularFileNoReplace(temporaryPath, authorityPath)
    const publishedStat = lstatSync(authorityPath)
    if (!publishedStat.isFile() || publishedStat.isSymbolicLink() ||
      !sameFilesystemObject(temporaryStat, publishedStat)) {
      throw new Error('[after-pack] Packaged build authority publication identity is invalid')
    }
    syncDirectory(runtimeDir)
    if (options.afterFundsPluginAuthorityRename != null) {
      if (typeof options.afterFundsPluginAuthorityRename !== 'function') {
        throw new Error('[after-pack] funds_plugin_admission_changed')
      }
      options.afterFundsPluginAuthorityRename()
    }
    requireCurrentFundsPluginAdmissionV1(context, expectedFundsPluginAdmission)
    const publishedIdentity = hashStableRegularFile(authorityPath, { maximumBytes: 256 * 1024 })
    const publishedText = readFileSync(authorityPath, 'utf8')
    if (publishedIdentity.sha256 !== temporaryIdentity.sha256 || publishedText !== authorityText ||
      !isPackagedBuildAuthorityV2(JSON.parse(publishedText))) {
      throw new Error('[after-pack] Packaged build authority stable readback failed')
    }
    currentFundsPluginAdmission = requireCurrentFundsPluginAdmissionV1(
      context,
      expectedFundsPluginAdmission
    )
    if (canonicalJSON(authority.artifacts.fundsPlugin) !==
      canonicalJSON(currentFundsPluginAdmission.artifact)) {
      throw new Error('[after-pack] funds_plugin_admission_changed')
    }
    removeOwnedTemporaryFile(temporaryPath, temporaryOwnership)
    temporaryOwnership = undefined
    const durableIdentity = hashStableRegularFile(authorityPath, { maximumBytes: 256 * 1024 })
    if (durableIdentity.sha256 !== temporaryIdentity.sha256 ||
      durableIdentity.byteLength !== temporaryIdentity.byteLength) {
      throw new Error('[after-pack] Packaged build authority durable readback failed')
    }
  } catch (error) {
    removePublishedAuthorityIfOwned(authorityPath, publishedOwnership, temporaryIdentity || {
      sha256: sha256Bytes(authorityText),
      byteLength: Buffer.byteLength(authorityText)
    })
    throw error
  } finally {
    removeOwnedTemporaryFile(temporaryPath, temporaryOwnership)
  }
  return authority
}

function readPackagedBuildAuthorityV2(context) {
  const authorityPath = join(
    packedResourcesDir(context),
    'runtime',
    PACKAGED_BUILD_AUTHORITY_FILE
  )
  const stable = readStableRegularFileContent(authorityPath, {
    maximumBytes: 256 * 1024
  })
  let authority
  try {
    authority = parseStrictJsonObject(stable.content, {
      maxBytes: 256 * 1024,
      maxDepth: 12,
      maxTokens: 16384,
      maxStringBytes: 16384,
      maxNumberBytes: 32,
      integerOnly: true
    })
  } catch {
    throw new Error('[after-pack] Packaged build authority JSON is invalid')
  }
  if (!isPackagedBuildAuthorityV2(authority) ||
    authority.targetKey !== packagedTargetKey(context)) {
    throw new Error('[after-pack] Packaged build authority is invalid for this target')
  }
  if (stable.sha256 !== sha256Bytes(canonicalJSON(authority))) {
    throw new Error('[after-pack] Packaged build authority is not canonical')
  }
  return authority
}

function samePublishedNativeArtifact(binding, observed) {
  return binding.payloadSha256 === observed.payloadSha256 &&
    binding.payloadByteLength === observed.payloadByteLength &&
    binding.format === observed.format && binding.arch === observed.arch
}

function verifyPackagedBuildAuthorityArtifacts(context, authority, options = {}) {
  if (!isPackagedBuildAuthorityV2(authority) ||
    authority.targetKey !== packagedTargetKey(context)) {
    throw new Error('[after-pack] Packaged build authority is invalid for artifact verification')
  }
  const target = dataNativeTargetContract(
    normalizePlatform(context.electronPlatformName),
    goArchForTarget(context.arch)
  )
  const executable = signingInvariantNativeArtifactBinding(packagedExecutablePath(context), target)
  const runtimeServer = signingInvariantNativeArtifactBinding(bundledGoRuntimeServerPath(context), target)
  const appAsar = hashStableRegularFile(packagedAppAsarPath(context), {
    requireSingleLink: true
  })
  const fundsPlugin = isCoreDisposition(authority.nativeDisposition)
    ? (assertCoreResourcesAbsent(packedResourcesDir(context)), ABSENT_FUNDS)
    : collectPackagedFundsPluginIdentityV2(context)
  const stagedPayload = collectStagedPayloadClosureV1(context)
  const fusePolicy = options.verifyFuses === false ? null : verifyElectronFusePolicyV1(context)
  if (!samePublishedNativeArtifact(authority.artifacts.executable, executable) ||
    !samePublishedNativeArtifact(authority.artifacts.runtimeServer, runtimeServer) ||
    authority.artifacts.appAsar.sha256 !== appAsar.sha256 ||
    authority.artifacts.appAsar.byteLength !== appAsar.byteLength ||
    canonicalJSON(authority.artifacts.fundsPlugin) !== canonicalJSON(fundsPlugin) ||
    canonicalJSON(authority.stagedPayload) !== canonicalJSON(stagedPayload)) {
    throw new Error('[after-pack] Packaged build authority artifact binding mismatch')
  }
  return { executable, appAsar, runtimeServer, fundsPlugin, stagedPayload, fusePolicy }
}

function hasExactKeys(value, expectedKeys) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const actual = Object.keys(value).sort()
  const expected = [...expectedKeys].sort()
  return actual.length === expected.length && actual.every((key, index) => key === expected[index])
}

function readStaticStringConstant(modulePath, constantName) {
  let program
  try {
    const stable = readStableRegularFileContent(modulePath, { maximumBytes: 1024 * 1024 })
    program = parseJavaScriptModule(stable.content.toString('utf8'), {
      ecmaVersion: 'latest',
      sourceType: 'module'
    })
  } catch {
    throw new Error(`[after-pack] funds_runtime_identity_parse_failed:${constantName}`)
  }
  const matches = []
  function visit(node) {
    if (!node || typeof node !== 'object') return
    if (
      node.type === 'VariableDeclarator' &&
      node.id?.type === 'Identifier' &&
      node.id.name === constantName &&
      node.init?.type === 'Literal' &&
      typeof node.init.value === 'string'
    ) {
      matches.push(node.init.value)
    }
    for (const [key, value] of Object.entries(node)) {
      if (key === 'start' || key === 'end' || key === 'loc') continue
      if (Array.isArray(value)) value.forEach(visit)
      else if (value && typeof value === 'object' && typeof value.type === 'string') visit(value)
    }
  }
  visit(program)
  if (matches.length !== 1 || !matches[0]) {
    throw new Error(`[after-pack] funds_runtime_identity_invalid:${constantName}`)
  }
  return matches[0]
}

function validateFundsPluginMetadata(manifest, mcpConfig, runtimeVersions) {
  const manifestKeys = [
    'name', 'version', 'description', 'author', 'homepage', 'repository', 'license',
    'keywords', 'skills', 'mcpServers', 'interface'
  ]
  const interfaceKeys = [
    'displayName', 'shortDescription', 'longDescription', 'developerName', 'category',
    'capabilities', 'websiteURL', 'privacyPolicyURL', 'termsOfServiceURL', 'defaultPrompt',
    'brandColor', 'composerIcon', 'logo'
  ]
  if (
    !hasExactKeys(manifest, manifestKeys) ||
    manifest.name !== ANALYTIX_FUNDS_PLUGIN_NAME ||
    typeof manifest.version !== 'string' ||
    !/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/u.test(manifest.version) ||
    manifest.version !== runtimeVersions.serverVersion ||
    manifest.version !== runtimeVersions.handlerVersion ||
    manifest.skills !== './skills/' ||
    manifest.mcpServers !== './.mcp.json' ||
    !hasExactKeys(manifest.author, ['name', 'url']) ||
    manifest.author.name !== ANALYTIX_PROJECT_OWNER_NAME ||
    manifest.author.url !== ANALYTIX_PROJECT_OWNER_GITHUB_URL ||
    !Array.isArray(manifest.keywords) ||
    manifest.keywords.some((keyword) => typeof keyword !== 'string' || !keyword) ||
    !hasExactKeys(manifest.interface, interfaceKeys) ||
    JSON.stringify(manifest.interface.capabilities) !== JSON.stringify(['Interactive', 'Read']) ||
    manifest.interface.developerName !== 'Analytix' ||
    manifest.interface.category !== 'Productivity' ||
    manifest.interface.websiteURL !== 'https://analytix.top' ||
    manifest.interface.privacyPolicyURL !== 'https://analytix.top/privacy' ||
    manifest.interface.termsOfServiceURL !== 'https://analytix.top/terms' ||
    !Array.isArray(manifest.interface.defaultPrompt) ||
    manifest.interface.defaultPrompt.length !== 1 ||
    typeof manifest.interface.defaultPrompt[0] !== 'string' ||
    !manifest.interface.defaultPrompt[0]
  ) {
    throw new Error('[after-pack] funds_plugin_manifest_contract_invalid')
  }

  if (!hasExactKeys(mcpConfig, ['mcpServers']) || !hasExactKeys(mcpConfig.mcpServers, ['analytix_funds'])) {
    throw new Error('[after-pack] funds_plugin_quarantine_config_invalid')
  }
  const server = mcpConfig.mcpServers.analytix_funds
  if (
    !hasExactKeys(server, [
      'disabled', 'command', 'cwd', 'args', 'env_vars', 'startup_timeout_sec',
      'tool_timeout_sec', 'default_tools_approval_mode', 'tools'
    ]) ||
    server.disabled !== true ||
    JSON.stringify(server.env_vars) !== JSON.stringify(ANALYTIX_FUNDS_EXPECTED_ENV_VARS) ||
    server.startup_timeout_sec !== 10 ||
    server.tool_timeout_sec !== 120 ||
    server.default_tools_approval_mode !== 'prompt' ||
    !hasExactKeys(server.tools, ['export_cleaned_case_data', 'run_full_case_analysis']) ||
    !hasExactKeys(server.tools.export_cleaned_case_data, ['approval_mode']) ||
    server.tools.export_cleaned_case_data.approval_mode !== 'prompt' ||
    !hasExactKeys(server.tools.run_full_case_analysis, ['approval_mode']) ||
    server.tools.run_full_case_analysis.approval_mode !== 'prompt' ||
    server.env_vars.includes(ANALYTIX_FUNDS_FORBIDDEN_INTERPRETER_ENV)
  ) {
    throw new Error('[after-pack] funds_plugin_quarantine_config_invalid')
  }
}

function normalizedFundsMCPEntrypoint(server) {
  if (!server || typeof server !== 'object' || Array.isArray(server) ||
    server.command !== 'node' || server.cwd !== '.' || !Array.isArray(server.args) ||
    server.args.length !== 1 || typeof server.args[0] !== 'string' ||
    !server.args[0].startsWith('./')) {
    return ''
  }
  const relativePath = server.args[0].slice(2)
  return portableFundsPluginPathV2(relativePath) ? relativePath : ''
}

// pluginpackage.ParseDeclarationV1 remains the only Core schema owner. This
// packaging-side mirror is a defensive pre-admission gate for staged bytes;
// its hostile corpus is executed against the Go owner by the packaging suite.
function validateFundsPluginDeclarationV1Bytes(body) {
  let declaration
  try {
    assertFundsPluginDeclarationIntegerLexemesV1(body)
    declaration = parseStrictJsonObject(body, {
      maxBytes: 256 * 1024,
      maxDepth: 8,
      maxTokens: 8192,
      maxStringBytes: 64 * 1024,
      maxNumberBytes: 32,
      integerOnly: false
    })
    declaration = validateFundsPluginDeclarationV1(declaration)
  } catch {
    throw new Error('[after-pack] funds_plugin_declaration_v1_invalid')
  }
  const canonical = canonicalJSON(declaration)
  return Object.freeze({
    declaration,
    canonicalSha256: sha256Bytes(canonical),
    canonicalByteLength: Buffer.byteLength(canonical)
  })
}

function assertFundsPluginDeclarationIntegerLexemesV1(body) {
  if (!Buffer.isBuffer(body)) throw new Error('declaration bytes are invalid')
  const source = body.toString('utf8')
  let inString = false
  let escaped = false
  for (let index = 0; index < source.length; index += 1) {
    const character = source[index]
    if (inString) {
      if (escaped) {
        escaped = false
      } else if (character === '\\') {
        escaped = true
      } else if (character === '"') {
        inString = false
      }
      continue
    }
    if (character === '"') {
      inString = true
      continue
    }
    if (character !== '-' && (character < '0' || character > '9')) continue
    const number = /^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?/u.exec(
      source.slice(index)
    )?.[0]
    if (!number || number.length > 32 || /[.eE]/u.test(number)) {
      throw new Error('declaration integer is invalid')
    }
    index += number.length - 1
  }
}

function validateFundsPluginDeclarationV1(value) {
  if (!hasExactKeys(value, [
    'schemaVersion', 'packageId', 'packageVersion', 'contributions',
    'requestedCapabilities', 'lifecycle'
  ]) || value.schemaVersion !== 1 ||
    !validFundsPackageIDV1(value.packageId) ||
    !validFundsSemanticVersionV1(value.packageVersion) ||
    !hasExactKeys(value.contributions, [
      'skills', 'mcpServers', 'hooks', 'assets', 'publicUi'
    ]) || !Array.isArray(value.requestedCapabilities) ||
    !hasExactKeys(value.lifecycle, ['protocolVersion', 'entryPolicy']) ||
    !positiveSafeIntegerV1(value.lifecycle.protocolVersion) ||
    !validFundsContractIdentifierV1(value.lifecycle.entryPolicy)) {
    throw new Error('declaration shape is invalid')
  }

  const contributionIDs = new Set()
  const contributionPaths = new Set()
  const normalizePathContribution = (contribution) => {
    if (!hasExactKeys(contribution, ['id', 'path']) ||
      !validFundsContractIdentifierV1(contribution.id) ||
      !portableFundsPluginDeclarationPathV1(contribution.path) ||
      contributionIDs.has(contribution.id) || contributionPaths.has(contribution.path)) {
      throw new Error('path contribution is invalid')
    }
    contributionIDs.add(contribution.id)
    contributionPaths.add(contribution.path)
    return { id: contribution.id, path: contribution.path }
  }
  const normalizeMCPContribution = (contribution) => {
    if (!hasExactKeys(contribution, ['id', 'entrypoint']) ||
      !validFundsContractIdentifierV1(contribution.id) ||
      !portableFundsPluginDeclarationPathV1(contribution.entrypoint) ||
      contributionIDs.has(contribution.id) || contributionPaths.has(contribution.entrypoint)) {
      throw new Error('MCP contribution is invalid')
    }
    contributionIDs.add(contribution.id)
    contributionPaths.add(contribution.entrypoint)
    return { id: contribution.id, entrypoint: contribution.entrypoint }
  }
  const contributionCollections = value.contributions
  for (const name of ['skills', 'mcpServers', 'hooks', 'assets', 'publicUi']) {
    if (!Array.isArray(contributionCollections[name])) {
      throw new Error('contribution collection is invalid')
    }
  }
  const contributions = {
    skills: contributionCollections.skills.map(normalizePathContribution),
    mcpServers: contributionCollections.mcpServers.map(normalizeMCPContribution),
    hooks: contributionCollections.hooks.map(normalizePathContribution),
    assets: contributionCollections.assets.map(normalizePathContribution),
    publicUi: contributionCollections.publicUi.map(normalizePathContribution)
  }
  if (Object.values(contributions).reduce((count, items) => count + items.length, 0) === 0) {
    throw new Error('empty contributions')
  }

  const capabilityIDs = new Set()
  const requestedCapabilities = value.requestedCapabilities.map((request) => {
    if (!hasExactKeys(request, ['id', 'protocolVersion', 'scopeConstraints']) ||
      !validFundsContractIdentifierV1(request.id) ||
      !positiveSafeIntegerV1(request.protocolVersion) ||
      !Array.isArray(request.scopeConstraints) || request.scopeConstraints.length === 0 ||
      capabilityIDs.has(request.id)) {
      throw new Error('capability request is invalid')
    }
    capabilityIDs.add(request.id)
    const scopes = new Set()
    for (const scope of request.scopeConstraints) {
      if (!validFundsContractIdentifierV1(scope) || scopes.has(scope)) {
        throw new Error('capability scope is invalid')
      }
      scopes.add(scope)
    }
    return {
      id: request.id,
      protocolVersion: request.protocolVersion,
      scopeConstraints: [...scopes].sort()
    }
  })

  const sortContributions = (items, pathKey) => items.sort((left, right) =>
    left.id === right.id
      ? Buffer.compare(Buffer.from(left[pathKey]), Buffer.from(right[pathKey]))
      : Buffer.compare(Buffer.from(left.id), Buffer.from(right.id)))
  sortContributions(contributions.skills, 'path')
  sortContributions(contributions.mcpServers, 'entrypoint')
  sortContributions(contributions.hooks, 'path')
  sortContributions(contributions.assets, 'path')
  sortContributions(contributions.publicUi, 'path')
  requestedCapabilities.sort((left, right) =>
    Buffer.compare(Buffer.from(left.id), Buffer.from(right.id)))
  return {
    schemaVersion: value.schemaVersion,
    packageId: value.packageId,
    packageVersion: value.packageVersion,
    contributions,
    requestedCapabilities,
    lifecycle: {
      protocolVersion: value.lifecycle.protocolVersion,
      entryPolicy: value.lifecycle.entryPolicy
    }
  }
}

function positiveSafeIntegerV1(value) {
  return Number.isSafeInteger(value) && value > 0
}

function validFundsPackageIDV1(value) {
  if (typeof value !== 'string' || value.length === 0 || value.length > 128 ||
    value !== value.trim() || !/^[a-z]/u.test(value)) {
    return false
  }
  let previousSeparator = false
  for (let index = 0; index < value.length; index += 1) {
    const character = value[index]
    const separator = character === '-'
    if (!/[a-z0-9-]/u.test(character) ||
      (separator && (index === 0 || index === value.length - 1 || previousSeparator))) {
      return false
    }
    previousSeparator = separator
  }
  return true
}

function validFundsContractIdentifierV1(value) {
  if (typeof value !== 'string' || value.length === 0 || value.length > 128 ||
    value !== value.trim() || !/^[a-z][a-z0-9._:-]*$/u.test(value)) {
    return false
  }
  const separators = new Set(['-', '_', '.', ':'])
  if (separators.has(value.at(-1))) return false
  for (let index = 1; index < value.length; index += 1) {
    if (separators.has(value[index]) && separators.has(value[index - 1])) return false
  }
  return true
}

function validFundsSemanticVersionV1(value) {
  if (typeof value !== 'string' || value.length === 0 || value.length > 128 ||
    value !== value.trim() || (value.match(/\+/gu) || []).length > 1) {
    return false
  }
  const plus = value.indexOf('+')
  const versionWithoutBuild = plus < 0 ? value : value.slice(0, plus)
  const build = plus < 0 ? '' : value.slice(plus + 1)
  if (plus >= 0 && !validFundsSemanticIdentifiersV1(build, false)) return false
  const hyphen = versionWithoutBuild.indexOf('-')
  const coreText = hyphen < 0 ? versionWithoutBuild : versionWithoutBuild.slice(0, hyphen)
  const preRelease = hyphen < 0 ? '' : versionWithoutBuild.slice(hyphen + 1)
  if (hyphen >= 0 && !validFundsSemanticIdentifiersV1(preRelease, true)) return false
  const core = coreText.split('.')
  return core.length === 3 && core.every(canonicalFundsUnsignedDecimalV1)
}

function validFundsSemanticIdentifiersV1(value, rejectNumericLeadingZero) {
  const identifiers = value.split('.')
  return identifiers.length > 0 && identifiers.every((identifier) => {
    if (!identifier || !/^[A-Za-z0-9-]+$/u.test(identifier)) return false
    return !(rejectNumericLeadingZero && /^[0-9]+$/u.test(identifier) &&
      identifier.length > 1 && identifier.startsWith('0'))
  })
}

function canonicalFundsUnsignedDecimalV1(value) {
  return /^(?:0|[1-9][0-9]*)$/u.test(value)
}

function portableFundsPluginDeclarationPathV1(value) {
  return portableFundsPluginPathV2(value) && value === value.trim() &&
    !(value.length >= 2 && value[1] === ':')
}

function validateFundsPluginProjectionParity(declaration, manifest, mcpConfig, runtimeIdentity) {
  try {
    const mcpContributions = declaration?.contributions?.mcpServers
    if (typeof declaration?.packageId !== 'string' || !declaration.packageId ||
      typeof declaration?.packageVersion !== 'string' || !declaration.packageVersion ||
      !Array.isArray(mcpContributions) || mcpContributions.length !== 1 ||
      declaration.packageId !== manifest?.name || declaration.packageVersion !== manifest?.version ||
      !hasExactKeys(runtimeIdentity, ['serverName', 'serverVersion', 'handlerVersion']) ||
      runtimeIdentity.serverVersion !== declaration.packageVersion ||
      runtimeIdentity.handlerVersion !== declaration.packageVersion ||
      !hasExactKeys(mcpConfig, ['mcpServers']) ||
      !mcpConfig.mcpServers || typeof mcpConfig.mcpServers !== 'object' ||
      Array.isArray(mcpConfig.mcpServers)) {
      throw new Error('identity mismatch')
    }
    const contribution = mcpContributions[0]
    if (!hasExactKeys(contribution, ['id', 'entrypoint']) ||
      typeof contribution.id !== 'string' || !contribution.id ||
      !portableFundsPluginPathV2(contribution.entrypoint) ||
      contribution.entrypoint !== FUNDS_PLUGIN_ENTRYPOINT_PATH ||
      runtimeIdentity.serverName !== contribution.id ||
      !hasExactKeys(mcpConfig.mcpServers, [contribution.id])) {
      throw new Error('MCP identity mismatch')
    }
    const server = mcpConfig.mcpServers[contribution.id]
    if (normalizedFundsMCPEntrypoint(server) !== FUNDS_PLUGIN_ENTRYPOINT_PATH ||
      contribution.entrypoint !== FUNDS_PLUGIN_ENTRYPOINT_PATH) {
      throw new Error('MCP entrypoint mismatch')
    }
  } catch {
    throw new Error('[after-pack] funds_plugin_projection_parity_invalid')
  }
}

function fundsPluginProjectionPath(root, relativePath) {
  if (!portableFundsPluginDeclarationPathV1(relativePath)) {
    throw new Error('[after-pack] funds_plugin_projection_path_invalid')
  }
  const target = resolve(root, ...relativePath.split('/'))
  const fromRoot = relative(root, target)
  if (!fromRoot || fromRoot === '..' || fromRoot.startsWith(`..${sep}`) || isAbsolute(fromRoot)) {
    throw new Error('[after-pack] funds_plugin_projection_path_invalid')
  }
  let canonicalTarget
  try {
    canonicalTarget = realpathSync(target)
  } catch {
    throw new Error('[after-pack] funds_plugin_projection_path_invalid')
  }
  if (!isPathInsideOrEqual(root, canonicalTarget)) {
    throw new Error('[after-pack] funds_plugin_projection_path_invalid')
  }
  return target
}

function inspectFundsPluginSourceProjectionsV1(pluginRoot) {
  let root
  let rootBefore
  try {
    root = realpathSync(resolve(pluginRoot))
    rootBefore = lstatSync(root)
    if (!rootBefore.isDirectory() || rootBefore.isSymbolicLink()) {
      throw new Error('invalid root')
    }
  } catch {
    throw new Error('[after-pack] funds_plugin_projection_root_invalid')
  }

  const readProjection = (label, relativePath) => {
    try {
      return readStableRegularFileContent(
        fundsPluginProjectionPath(root, relativePath),
        { maximumBytes: 256 * 1024 }
      )
    } catch {
      throw new Error(`[after-pack] funds_plugin_${label}_source_invalid`)
    }
  }
  const projectionFiles = Object.freeze({
    declaration: readProjection('declaration', ANALYTIX_FUNDS_PLUGIN_DECLARATION_PATH),
    manifest: readProjection('manifest', FUNDS_PLUGIN_MANIFEST_PATH),
    config: readProjection('config', '.mcp.json')
  })

  let declarationAdmission
  let manifest
  let mcpConfig
  try {
    declarationAdmission = validateFundsPluginDeclarationV1Bytes(projectionFiles.declaration.content)
    const projectionStrictOptions = {
      maxBytes: 256 * 1024,
      maxDepth: 16,
      maxTokens: 16384,
      maxStringBytes: 64 * 1024,
      maxNumberBytes: 128,
      integerOnly: false
    }
    manifest = parseStrictJsonObject(projectionFiles.manifest.content, projectionStrictOptions)
    mcpConfig = parseStrictJsonObject(projectionFiles.config.content, projectionStrictOptions)
  } catch (error) {
    if (error?.message === '[after-pack] funds_plugin_declaration_v1_invalid') throw error
    throw new Error('[after-pack] funds_plugin_json_invalid')
  }

  const declaration = declarationAdmission.declaration
  const runtimeIdentity = Object.freeze({
    serverName: readStaticStringConstant(
      fundsPluginProjectionPath(root, FUNDS_PLUGIN_ENTRYPOINT_PATH),
      'SERVER_NAME'
    ),
    serverVersion: readStaticStringConstant(
      fundsPluginProjectionPath(root, FUNDS_PLUGIN_ENTRYPOINT_PATH),
      'SERVER_VERSION'
    ),
    handlerVersion: readStaticStringConstant(
      fundsPluginProjectionPath(root, 'mcp/mcp-request-handler-runtime.mjs'),
      'MCP_REQUEST_HANDLER_RUNTIME_VERSION'
    )
  })
  validateFundsPluginProjectionParity(declaration, manifest, mcpConfig, runtimeIdentity)
  validateFundsPluginMetadata(manifest, mcpConfig, runtimeIdentity)

  if (!Array.isArray(declaration.contributions.publicUi) ||
    declaration.contributions.publicUi.length === 0) {
    throw new Error('[after-pack] funds_plugin_public_ui_invalid')
  }
  const publicUi = Object.freeze(declaration.contributions.publicUi.map((contribution) => {
    let stable
    try {
      stable = hashStableRegularFile(
        fundsPluginProjectionPath(root, contribution.path),
        { maximumBytes: 256 * 1024 }
      )
    } catch {
      throw new Error('[after-pack] funds_plugin_public_ui_invalid')
    }
    return Object.freeze({
      id: contribution.id,
      path: contribution.path,
      sha256: stable.sha256,
      byteLength: stable.byteLength
    })
  }))

  let rootAfter
  try {
    rootAfter = lstatSync(root)
  } catch {
    throw new Error('[after-pack] funds_plugin_projection_root_changed')
  }
  if (!rootAfter.isDirectory() || rootAfter.isSymbolicLink() ||
    !sameFilesystemObject(rootBefore, rootAfter)) {
    throw new Error('[after-pack] funds_plugin_projection_root_changed')
  }

  return Object.freeze({
    root,
    packageId: declaration.packageId,
    packageVersion: declaration.packageVersion,
    declarationAdmission,
    declaration,
    manifest,
    mcpConfig,
    runtimeIdentity,
    projectionFiles,
    publicUi
  })
}

function portableFundsPluginPathV2(value) {
  return typeof value === 'string' && value.length > 0 && !value.includes('\\') &&
    !value.startsWith('/') && /^[\x21-\x7e]+$/u.test(value) &&
    value.split('/').every((segment) => segment && segment !== '.' && segment !== '..')
}

function isFundsPluginArtifactBindingV2(value) {
  return exactKeys(value, [
    'treeSha256', 'fileCount', 'manifestSha256', 'entrypointSha256', 'totalBytes'
  ]) && isSha256(value.treeSha256) && isSha256(value.manifestSha256) &&
    isSha256(value.entrypointSha256) && Number.isSafeInteger(value.fileCount) &&
    value.fileCount > 0 && value.fileCount <= FUNDS_PLUGIN_MAX_FILES &&
    Number.isSafeInteger(value.totalBytes) && value.totalBytes > 0 &&
    value.totalBytes <= FUNDS_PLUGIN_MAX_TREE_BYTES
}

function createFundsPluginAdmissionV1(options) {
  const withoutFingerprint = {
    schemaVersion: 1,
    contract: FUNDS_PLUGIN_ADMISSION_CONTRACT_V1,
    declarationSha256: options.declarationSha256,
    declarationCanonicalSha256: options.declarationCanonicalSha256,
    manifestSha256: options.manifestSha256,
    mcpConfigSha256: options.mcpConfigSha256,
    runtimeIdentitySha256: options.runtimeIdentitySha256,
    artifact: options.artifact
  }
  if (!isSha256(withoutFingerprint.declarationSha256) ||
    !isSha256(withoutFingerprint.declarationCanonicalSha256) ||
    !isSha256(withoutFingerprint.manifestSha256) ||
    !isSha256(withoutFingerprint.mcpConfigSha256) ||
    !isSha256(withoutFingerprint.runtimeIdentitySha256) ||
    !isFundsPluginArtifactBindingV2(withoutFingerprint.artifact)) {
    throw new Error('[after-pack] funds_plugin_admission_invalid')
  }
  return Object.freeze({
    ...withoutFingerprint,
    fingerprint: domainSeparatedSha256(
      FUNDS_PLUGIN_ADMISSION_DOMAIN_V1,
      canonicalJSON(withoutFingerprint)
    )
  })
}

function isFundsPluginAdmissionV1(value) {
  if (!hasExactKeys(value, [
    'schemaVersion', 'contract', 'declarationSha256', 'declarationCanonicalSha256',
    'manifestSha256', 'mcpConfigSha256', 'runtimeIdentitySha256', 'artifact', 'fingerprint'
  ]) || value.schemaVersion !== 1 || value.contract !== FUNDS_PLUGIN_ADMISSION_CONTRACT_V1 ||
    !isSha256(value.declarationSha256) || !isSha256(value.declarationCanonicalSha256) ||
    !isSha256(value.manifestSha256) || !isSha256(value.mcpConfigSha256) ||
    !isSha256(value.runtimeIdentitySha256) || !isFundsPluginArtifactBindingV2(value.artifact) ||
    !isSha256(value.fingerprint)) {
    return false
  }
  const { fingerprint, ...withoutFingerprint } = value
  return fingerprint === domainSeparatedSha256(
    FUNDS_PLUGIN_ADMISSION_DOMAIN_V1,
    canonicalJSON(withoutFingerprint)
  )
}

function sameFundsPluginAdmissionV1(left, right) {
  return isFundsPluginAdmissionV1(left) && isFundsPluginAdmissionV1(right) &&
    left.fingerprint === right.fingerprint && canonicalJSON(left) === canonicalJSON(right)
}

function requireCurrentFundsPluginAdmissionV1(context, expected) {
  if (isCoreContext(context)) {
    assertCoreResourcesAbsent(packedResourcesDir(context))
    if (canonicalJSON(expected) !== canonicalJSON({ artifact: ABSENT_FUNDS })) throw Error('core_profile_funds_admission_mismatch')
    return { artifact: ABSENT_FUNDS }
  }
  if (!isFundsPluginAdmissionV1(expected)) {
    throw new Error('[after-pack] funds_plugin_admission_changed')
  }
  let current
  try {
    current = validateBundledFundsPlugin(context, { quiet: true })
  } catch {
    throw new Error('[after-pack] funds_plugin_admission_changed')
  }
  if (!sameFundsPluginAdmissionV1(expected, current)) {
    throw new Error('[after-pack] funds_plugin_admission_changed')
  }
  return current
}

// This is byte-compatible with pluginmaterializationfs.InspectSourceTreeV1.
// It intentionally ignores directory modes and hashes only portable relative
// file paths, exact byte lengths, and file SHA-256 values. The result is
// embedded in the signed packaged authority so the later Go materializer
// cannot bless whatever plugin tree happens to occupy the path at startup.
function collectPackagedFundsPluginIdentityV2(context) {
  if (isCoreContext(context)) {
    assertCoreResourcesAbsent(packedResourcesDir(context))
    return ABSENT_FUNDS
  }
  const requestedRoot = join(
    packedResourcesDir(context),
    'plugins',
    ANALYTIX_FUNDS_PLUGIN_NAME
  )
  const root = resolve(requestedRoot)
  const realRoot = realpathSync(root)
  const rootBefore = lstatSync(root)
  if (realRoot !== root || !rootBefore.isDirectory() || rootBefore.isSymbolicLink()) {
    throw new Error('[after-pack] Packaged funds plugin root is untrusted')
  }
  const records = []
  let totalBytes = 0
  const visit = (absoluteDirectory, relativeDirectory) => {
    const directoryBefore = lstatSync(absoluteDirectory)
    const namesBefore = readdirSync(absoluteDirectory)
      .sort((left, right) => Buffer.compare(Buffer.from(left), Buffer.from(right)))
    for (const name of namesBefore) {
      const relativePath = relativeDirectory ? `${relativeDirectory}/${name}` : name
      if (!portableFundsPluginPathV2(relativePath)) {
        throw new Error('[after-pack] Packaged funds plugin path is not portable')
      }
      const absolutePath = join(absoluteDirectory, name)
      const info = lstatSync(absolutePath)
      if (info.isSymbolicLink()) {
        throw new Error('[after-pack] Packaged funds plugin contains a symbolic link')
      }
      if (info.isDirectory()) {
        visit(absolutePath, relativePath)
        continue
      }
      if (!info.isFile() || relativePath === FUNDS_PLUGIN_INSTALL_MARKER) {
        throw new Error('[after-pack] Packaged funds plugin contains an invalid source entry')
      }
      const identity = hashStableRegularFile(absolutePath, {
        maximumBytes: FUNDS_PLUGIN_MAX_FILE_BYTES,
        allowEmpty: true
      })
      totalBytes += identity.byteLength
      if (totalBytes > FUNDS_PLUGIN_MAX_TREE_BYTES || records.length >= FUNDS_PLUGIN_MAX_FILES) {
        throw new Error('[after-pack] Packaged funds plugin tree exceeds its bound')
      }
      records.push({
        path: relativePath,
        size: identity.byteLength,
        sha256: identity.sha256
      })
    }
    const directoryAfter = lstatSync(absoluteDirectory)
    const namesAfter = readdirSync(absoluteDirectory)
      .sort((left, right) => Buffer.compare(Buffer.from(left), Buffer.from(right)))
    if (!sameFileIdentity(directoryBefore, directoryAfter) || directoryAfter.isSymbolicLink() ||
      canonicalJSON(namesBefore) !== canonicalJSON(namesAfter)) {
      throw new Error('[after-pack] Packaged funds plugin directory changed during inspection')
    }
  }
  visit(root, '')
  const rootAfter = lstatSync(root)
  if (!sameFileIdentity(rootBefore, rootAfter) || !rootAfter.isDirectory() ||
    rootAfter.isSymbolicLink() || realpathSync(root) !== realRoot || records.length === 0) {
    throw new Error('[after-pack] Packaged funds plugin root changed during inspection')
  }
  records.sort((left, right) => Buffer.compare(Buffer.from(left.path), Buffer.from(right.path)))
  const manifest = records.find((record) => record.path === FUNDS_PLUGIN_MANIFEST_PATH)
  const entrypoint = records.find((record) => record.path === FUNDS_PLUGIN_ENTRYPOINT_PATH)
  const binding = {
    treeSha256: sha256Bytes(canonicalJSON(records)),
    fileCount: records.length,
    manifestSha256: manifest?.sha256 || '',
    entrypointSha256: entrypoint?.sha256 || '',
    totalBytes
  }
  if (!isFundsPluginArtifactBindingV2(binding)) {
    throw new Error('[after-pack] Packaged funds plugin identity is invalid')
  }
  return binding
}

function validateBundledFundsPlugin(context, options = {}) {
  if (isCoreContext(context)) {
    assertCoreResourcesAbsent(packedResourcesDir(context))
    return { artifact: ABSENT_FUNDS }
  }
  const canonicalSourceRoot = join(__dirname, '..', 'plugins', ANALYTIX_FUNDS_PLUGIN_NAME)
  const sourceRoot = options.sourceRoot == null
    ? canonicalSourceRoot
    : realpathSync(resolve(options.sourceRoot))
  const packagedRoot = join(packedResourcesDir(context), 'plugins', ANALYTIX_FUNDS_PLUGIN_NAME)
  const packagedRootStat = lstatSync(packagedRoot)
  if (!packagedRootStat.isDirectory() || packagedRootStat.isSymbolicLink()) {
    throw new Error('[after-pack] funds_plugin_root_invalid')
  }
  const packagedMcpRoot = join(packagedRoot, 'mcp')
  const packagedMcpRootStat = lstatSync(packagedMcpRoot)
  if (!packagedMcpRootStat.isDirectory() || packagedMcpRootStat.isSymbolicLink()) {
    throw new Error('[after-pack] funds_mcp_root_invalid')
  }
  const actualMcpFiles = readdirSync(packagedMcpRoot, { withFileTypes: true })
    .map((entry) => entry.name)
    .sort()
  const expectedMcpFiles = ANALYTIX_FUNDS_PRODUCTION_MCP_FILES
    .map((relativePath) => relativePath.slice('mcp/'.length))
    .sort()
  if (
    actualMcpFiles.length !== expectedMcpFiles.length ||
    actualMcpFiles.some((name, index) => name !== expectedMcpFiles[index])
  ) {
    throw new Error('[after-pack] funds_mcp_inventory_mismatch')
  }
  for (const relativePath of ANALYTIX_FUNDS_PRODUCTION_MCP_FILES) {
    const sourcePath = join(sourceRoot, relativePath)
    const packagedPath = join(packagedRoot, relativePath)
    for (const [label, filePath] of [['source', sourcePath], ['packaged', packagedPath]]) {
      const stat = lstatSync(filePath)
      if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1 || stat.size <= 0) {
        throw new Error(`[after-pack] funds_mcp_${label}_file_invalid:${relativePath}`)
      }
    }
    if (sha256File(packagedPath) !== sha256File(sourcePath)) {
      throw new Error(`[after-pack] funds_mcp_hash_mismatch:${relativePath}`)
    }
  }

  const declarationPath = join(packagedRoot, ANALYTIX_FUNDS_PLUGIN_DECLARATION_PATH)
  const manifestPath = join(packagedRoot, '.codex-plugin', 'plugin.json')
  const mcpConfigPath = join(packagedRoot, '.mcp.json')
  const stableProjectionFiles = new Map()
  for (const [label, sourcePath, packagedPath] of [
    ['declaration', join(sourceRoot, ANALYTIX_FUNDS_PLUGIN_DECLARATION_PATH), declarationPath],
    ['manifest', join(sourceRoot, '.codex-plugin', 'plugin.json'), manifestPath],
    ['config', join(sourceRoot, '.mcp.json'), mcpConfigPath]
  ]) {
    const stableByOrigin = new Map()
    for (const [origin, filePath] of [['source', sourcePath], ['packaged', packagedPath]]) {
      try {
        stableByOrigin.set(origin, readStableRegularFileContent(filePath, {
          maximumBytes: 256 * 1024
        }))
      } catch {
        throw new Error(`[after-pack] funds_plugin_${label}_${origin}_invalid`)
      }
    }
    const sourceStable = stableByOrigin.get('source')
    const packagedStable = stableByOrigin.get('packaged')
    if (sourceStable.sha256 !== packagedStable.sha256 ||
      sourceStable.byteLength !== packagedStable.byteLength) {
      throw new Error(`[after-pack] funds_plugin_${label}_hash_mismatch`)
    }
    stableProjectionFiles.set(label, packagedStable)
  }
  const inspection = inspectFundsPluginSourceProjectionsV1(packagedRoot)
  for (const label of ['declaration', 'manifest', 'config']) {
    const firstRead = stableProjectionFiles.get(label)
    const inspected = inspection.projectionFiles[label]
    if (firstRead.sha256 !== inspected.sha256 || firstRead.byteLength !== inspected.byteLength) {
      throw new Error(`[after-pack] funds_plugin_${label}_changed_during_inspection`)
    }
  }
  const { declarationAdmission, runtimeIdentity } = inspection
  try {
    execFileSync(process.execPath, [
      join(canonicalSourceRoot, 'scripts', 'production-mcp-entry-closure-contract.mjs'),
      '--plugin-root',
      packagedRoot
    ], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe']
    })
  } catch {
    throw new Error('[after-pack] funds_mcp_ast_closure_invalid')
  }
  const admission = createFundsPluginAdmissionV1({
    declarationSha256: stableProjectionFiles.get('declaration').sha256,
    declarationCanonicalSha256: declarationAdmission.canonicalSha256,
    manifestSha256: stableProjectionFiles.get('manifest').sha256,
    mcpConfigSha256: stableProjectionFiles.get('config').sha256,
    runtimeIdentitySha256: sha256Bytes(canonicalJSON(runtimeIdentity)),
    artifact: collectPackagedFundsPluginIdentityV2(context)
  })
  if (options.quiet !== true) {
    console.log(`[after-pack] Validated funds MCP production closure (${expectedMcpFiles.length} modules).`)
  }
  return admission
}

function pathEntryExists(path) {
  try {
    lstatSync(path)
    return true
  } catch (error) {
    if (error && error.code === 'ENOENT') return false
    throw error
  }
}

function assertNativePackageQuarantined(context, options = {}) {
  const runtimeDir = join(packedResourcesDir(context), 'runtime')
  for (const name of ANALYTIX_DATA_NATIVE_REQUIRED_TOOLS) {
    for (const fileName of [name, `${name}.exe`]) {
      if (pathEntryExists(join(runtimeDir, fileName))) {
        throw new Error('[after-pack] legacy_native_layout_forbidden')
      }
    }
  }
  for (const fileName of ANALYTIX_NATIVE_GENERATION_METADATA) {
    if (pathEntryExists(join(runtimeDir, fileName))) {
      throw new Error('[after-pack] legacy_native_layout_forbidden')
    }
  }
  if (pathEntryExists(join(runtimeDir, NATIVE_DEVELOPMENT_BUILD_MARKER))) {
    throw new Error('[after-pack] legacy_native_layout_forbidden')
  }
  if (pathEntryExists(join(runtimeDir, 'data-native'))) {
    throw new Error('[after-pack] legacy_native_layout_forbidden')
  }
  const generationRoot = join(runtimeDir, 'native-components')
  if (pathEntryExists(generationRoot)) {
    try {
      const target = dataNativeTargetContract(
        normalizePlatform(context.electronPlatformName),
        goArchForTarget(context.arch)
      )
      requireExactDirectoryEntries(generationRoot, [target.key], 'directories')
      const targetRoot = join(generationRoot, target.key)
      requireExactDirectoryEntries(targetRoot, ['current'], 'directories')
      const current = join(targetRoot, 'current')
      requireExactDirectoryEntries(current, [
        ...ANALYTIX_DATA_NATIVE_REQUIRED_TOOLS.map((name) => dataAnalysisNativeBinaryName(context, name)),
        ...ANALYTIX_NATIVE_GENERATION_METADATA
      ], 'files')
    } catch {
      throw new Error('[after-pack] generation_transport_invalid')
    }
    throw new Error('[after-pack] native_component_release_authority_unavailable')
  }
  if (options.allowUnavailable === true) return
  throw new Error('[after-pack] native_component_release_authority_unavailable')
}

function requireExactDirectoryEntries(path, expectedNames, kind) {
  const stat = lstatSync(path)
  if (!stat.isDirectory() || stat.isSymbolicLink()) {
    throw new Error('invalid generation directory')
  }
  const entries = readdirSync(path, { withFileTypes: true })
  const actual = entries.map((entry) => entry.name).sort()
  const expected = [...expectedNames].sort()
  if (actual.length !== expected.length || actual.some((name, index) => name !== expected[index])) {
    throw new Error('invalid generation inventory')
  }
  for (const entry of entries) {
    const entryStat = lstatSync(join(path, entry.name))
    if (entry.isSymbolicLink() || entryStat.isSymbolicLink()) {
      throw new Error('invalid generation link')
    }
    if (kind === 'directories' && (!entry.isDirectory() || !entryStat.isDirectory())) {
      throw new Error('invalid generation directory entry')
    }
    if (kind === 'files' && (!entry.isFile() || !entryStat.isFile() || entryStat.nlink !== 1 || entryStat.size <= 0)) {
      throw new Error('invalid generation file entry')
    }
  }
}

function readStableRegularFileContent(path, options = {}) {
  const stable = hashStableRegularFile(path, {
    maximumBytes: options.maximumBytes,
    requireSingleLink: true,
    requireTrustedMode: true,
    includeContent: true
  })
  return stable
}

function developmentMarkerComponentValid(recorded, component, target) {
  return exactKeys(recorded, [
    'id', 'sourceDigest', 'cargoLockSha256', 'buildEnvironmentSha256',
    'binaryName', 'binarySha256', 'binarySize', 'payloadSha256', 'payloadSize',
    'format', 'arch'
  ]) && recorded.id === component.id &&
    recorded.binaryName === nativeBinaryName(component, target.platform) &&
    isSha256(recorded.sourceDigest) && isSha256(recorded.cargoLockSha256) &&
    isSha256(recorded.buildEnvironmentSha256) && isSha256(recorded.binarySha256) &&
    isSha256(recorded.payloadSha256) &&
    Number.isSafeInteger(recorded.binarySize) && recorded.binarySize > 0 &&
    Number.isSafeInteger(recorded.payloadSize) && recorded.payloadSize > 0 &&
    recorded.payloadSize <= recorded.binarySize && recorded.format === target.format &&
    recorded.arch === target.arch
}

function validateDevelopmentBuildMarker(marker, target, options = {}) {
  if (!exactKeys(marker, [
    'schemaVersion', 'kind', 'classification', 'publishable', 'releaseEligible',
    'authorityUse', 'targetKey', 'targetTriple', 'platform', 'arch',
    'sourceSetSha256', 'buildContextSha256', 'buildEnvironmentSha256',
    'toolchain', 'components'
  ]) || marker.schemaVersion !== 1 || marker.kind !== 'analytix_native_development_build' ||
    marker.classification !== NATIVE_DISPOSITION_DEVELOPMENT || marker.publishable !== false ||
    marker.releaseEligible !== false || marker.authorityUse !== 'development_only' ||
    marker.targetKey !== target.key || marker.targetTriple !== target.triple ||
    marker.platform !== target.platform || marker.arch !== target.arch ||
    !isSha256(marker.sourceSetSha256) || !isSha256(marker.buildContextSha256) ||
    !isSha256(marker.buildEnvironmentSha256) ||
    !exactKeys(marker.toolchain, [
      'cargoExecutableSha256', 'cargoVersion', 'rustcExecutableSha256', 'rustcVersion'
    ]) || !isSha256(marker.toolchain.cargoExecutableSha256) ||
    !isSha256(marker.toolchain.rustcExecutableSha256) ||
    typeof marker.toolchain.cargoVersion !== 'string' || !marker.toolchain.cargoVersion ||
    typeof marker.toolchain.rustcVersion !== 'string' || !marker.toolchain.rustcVersion ||
    !Array.isArray(marker.components) ||
    marker.components.length !== nativeComponentManifest.components.length ||
    !marker.components.every((recorded, index) =>
      developmentMarkerComponentValid(recorded, nativeComponentManifest.components[index], target))) {
    throw new Error('[after-pack] Native development marker contract is invalid')
  }
  const verifyCurrentSource = options.verifyCurrentSource !== false
  if (!verifyCurrentSource && process.env.NODE_ENV !== 'test') {
    throw new Error('[after-pack] Current native development source verification cannot be disabled')
  }
  if (verifyCurrentSource) {
    const repoRoot = options.repoRoot || join(__dirname, '..')
    if (marker.sourceSetSha256 !== sourceSetDigest(repoRoot) ||
      marker.buildContextSha256 !== nativeBuildContextDigest(repoRoot)) {
      throw new Error('[after-pack] Native development marker is stale for the current source')
    }
  }
  return marker
}

function readDevelopmentBuildMarker(path, target, options = {}) {
  const stable = readStableRegularFileContent(path, {
    maximumBytes: MAX_NATIVE_DEVELOPMENT_MARKER_BYTES
  })
  let marker
  try {
    marker = parseStrictJsonObject(stable.content, {
      maxBytes: MAX_NATIVE_DEVELOPMENT_MARKER_BYTES,
      maxDepth: 5,
      maxTokens: 512,
      maxStringBytes: 4096,
      maxNumberBytes: 32,
      integerOnly: true
    })
  } catch {
    throw new Error('[after-pack] Native development marker JSON is invalid')
  }
  validateDevelopmentBuildMarker(marker, target, options)
  return { marker, binding: { sha256: stable.sha256, byteLength: stable.byteLength } }
}

function developmentNativeDisposition(markerRecord, target) {
  const disposition = {
    kind: NATIVE_DISPOSITION_DEVELOPMENT,
    targetKey: target.key,
    marker: markerRecord.binding,
    components: markerRecord.marker.components.map((component) => ({
      id: component.id,
      binaryName: component.binaryName,
      markerBinarySha256: component.binarySha256,
      markerBinaryByteLength: component.binarySize,
      payloadSha256: component.payloadSha256,
      payloadByteLength: component.payloadSize,
      format: component.format,
      arch: component.arch
    }))
  }
  if (!isDevelopmentNativeDisposition(disposition)) {
    throw new Error('[after-pack] Native development disposition is invalid')
  }
  return disposition
}

function assertDevelopmentBinarySample(path, target, recorded) {
  const inspected = assertNativeBinaryTarget(path, target)
  if (inspected.sha256 !== recorded.binarySha256 || inspected.size !== recorded.binarySize ||
    inspected.payloadSha256 !== recorded.payloadSha256 || inspected.payloadSize !== recorded.payloadSize ||
    inspected.format !== recorded.format || inspected.arch !== recorded.arch) {
    throw new Error(`[after-pack] Native development binary does not match its marker: ${recorded.id}`)
  }
  return inspected
}

function sameDevelopmentMarkerRecord(left, right) {
  return left.binding.sha256 === right.binding.sha256 &&
    left.binding.byteLength === right.binding.byteLength &&
    canonicalJSON(left.marker) === canonicalJSON(right.marker)
}

function syncDirectory(path) {
  if (process.platform === 'win32') return
  const descriptor = openSync(path, constants.O_RDONLY)
  try {
    fsyncSync(descriptor)
  } finally {
    closeSync(descriptor)
  }
}

function assertCanonicalDirectory(path, boundaryRoot, label) {
  const resolvedBoundary = resolve(boundaryRoot)
  const resolvedPath = resolve(path)
  const escaped = relative(resolvedBoundary, resolvedPath)
  if (escaped === '..' || escaped.startsWith(`..${sep}`) || isAbsolute(escaped)) {
    throw new Error(`[after-pack] ${label} escaped its trusted boundary`)
  }
  const stat = lstatSync(resolvedPath)
  if (!stat.isDirectory() || stat.isSymbolicLink() ||
    realpathSync(resolvedPath) !== resolvedPath) {
    throw new Error(`[after-pack] ${label} is not a canonical directory`)
  }
  return stat
}

function directoryPathIdentity(path, stat, strict = false) {
  return {
    path: resolve(path),
    dev: String(stat.dev),
    ino: String(stat.ino),
    uid: stat.uid,
    gid: stat.gid,
    mode: stat.mode & 0o7777,
    nlink: strict ? stat.nlink : null,
    mtimeMs: strict ? stat.mtimeMs : null,
    ctimeMs: strict ? stat.ctimeMs : null
  }
}

function sameDirectoryPathIdentity(left, right) {
  return Boolean(left && right &&
    left.path === right.path &&
    left.dev === right.dev &&
    left.ino === right.ino &&
    left.uid === right.uid &&
    left.gid === right.gid &&
    left.mode === right.mode &&
    left.nlink === right.nlink &&
    left.mtimeMs === right.mtimeMs &&
    left.ctimeMs === right.ctimeMs)
}

function captureCanonicalDirectory(path, boundaryRoot, label, strict = false) {
  const stat = assertCanonicalDirectory(path, boundaryRoot, label)
  return {
    label,
    boundaryRoot: resolve(boundaryRoot),
    identity: directoryPathIdentity(path, stat, strict)
  }
}

function assertDirectoryChainUnchanged(expected) {
  const current = expected.map((entry) =>
    captureCanonicalDirectory(
      entry.identity.path,
      entry.boundaryRoot,
      entry.label,
      entry.identity.mtimeMs !== null
    )
  )
  if (current.length !== expected.length ||
    current.some((entry, index) =>
      !sameDirectoryPathIdentity(entry.identity, expected[index].identity))) {
    throw new Error('[after-pack] Trusted directory chain changed during packaging')
  }
  return current
}

function inspectPrivateNativeCacheSource(sourceRoot, cacheRoot, options = {}) {
  if ((options.mountRoot || options.expectedCacheRoot) && process.env.NODE_ENV !== 'test') {
    throw new Error('[after-pack] Native cache authority override is test-only')
  }
  const mountRoot = resolve(options.mountRoot || '/Volumes/AnalytixCache')
  const expectedCacheRoot = resolve(
    options.expectedCacheRoot ||
    join(DEVELOPMENT_CACHE_ROOT, 'native-components-development')
  )
  const resolvedCacheRoot = resolve(cacheRoot)
  const resolvedSourceRoot = resolve(sourceRoot)
  if ((!options.expectedCacheRoot &&
      resolvedCacheRoot !== requireDevelopmentNativeComponentRoot(process.env)) ||
    resolvedCacheRoot !== expectedCacheRoot) {
    throw new Error('[after-pack] Native development cache root is not authoritative')
  }
  const developmentRoot = options.expectedCacheRoot
    ? dirname(resolvedCacheRoot)
    : DEVELOPMENT_CACHE_ROOT
  const chain = [
    captureCanonicalDirectory(mountRoot, mountRoot, 'native cache mount'),
    captureCanonicalDirectory(
      developmentRoot,
      mountRoot,
      'development cache root'
    ),
    captureCanonicalDirectory(
      resolvedCacheRoot,
      developmentRoot,
      'native development cache root',
      true
    ),
    captureCanonicalDirectory(
      resolvedSourceRoot,
      resolvedCacheRoot,
      'native development target',
      true
    )
  ]
  const [mount, cache, nativeCache, source] =
    chain.map((entry) => lstatSync(entry.identity.path))
  const uid = typeof process.getuid === 'function' ? process.getuid() : null
  for (const [label, stat] of [
    ['native cache mount', mount],
    ['development cache root', cache],
    ['native development cache root', nativeCache],
    ['native development target', source]
  ]) {
    if (stat.dev !== mount.dev || (uid !== null && stat.uid !== uid)) {
      throw new Error(`[after-pack] ${label} owner or device is not authoritative`)
    }
  }
  for (const [label, stat] of [
    ['development cache root', cache],
    ['native development cache root', nativeCache],
    ['native development target', source]
  ]) {
    if ((stat.mode & 0o777) !== 0o700) {
      throw new Error(`[after-pack] ${label} is not owner-private`)
    }
  }
  return { source, chain }
}

function assertPrivateNativeCacheSource(sourceRoot, cacheRoot, options = {}) {
  return inspectPrivateNativeCacheSource(sourceRoot, cacheRoot, options).source
}

function privateNativeSourceFileIdentities(sourceRoot, target, sourceStat) {
  const uid = typeof process.getuid === 'function' ? process.getuid() : null
  const names = [
    ...nativeComponentManifest.components.map((component) =>
      nativeBinaryName(component, target.platform)),
    NATIVE_DEVELOPMENT_BUILD_MARKER
  ]
  return names.map((name) => {
    const path = join(sourceRoot, name)
    const stat = lstatSync(path)
    const expectedMode = name === NATIVE_DEVELOPMENT_BUILD_MARKER ? 0o600 : 0o700
    if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1 ||
      stat.size <= 0 || (stat.mode & 0o777) !== expectedMode ||
      stat.dev !== sourceStat.dev || (uid !== null && stat.uid !== uid)) {
      throw new Error(`[after-pack] Native development source file is unsafe: ${name}`)
    }
    return {
      path,
      dev: String(stat.dev),
      ino: String(stat.ino),
      uid: stat.uid,
      gid: stat.gid,
      mode: stat.mode & 0o7777,
      nlink: stat.nlink,
      size: stat.size,
      mtimeMs: stat.mtimeMs,
      ctimeMs: stat.ctimeMs
    }
  })
}

function samePrivateNativeSourceFiles(left, right) {
  return canonicalJSON(left) === canonicalJSON(right)
}

function assertPackagedNativeDestination(context, sourceRoot) {
  const appOutRoot = resolve(context.appOutDir)
  const platform = normalizePlatform(context.electronPlatformName)
  const resources = packedResourcesDir(context)
  const chain = [
    captureCanonicalDirectory(appOutRoot, appOutRoot, 'packaged app output')
  ]
  if (platform === 'darwin') {
    const app = appBundlePath(context)
    chain.push(
      captureCanonicalDirectory(app, appOutRoot, 'packaged app bundle'),
      captureCanonicalDirectory(
        join(app, 'Contents'),
        appOutRoot,
        'packaged app Contents'
      )
    )
  }
  chain.push(
    captureCanonicalDirectory(resources, appOutRoot, 'packaged app Resources')
  )
  const canonicalSource = realpathSync(sourceRoot)
  const canonicalOutput = realpathSync(appOutRoot)
  if (isPathInsideOrEqual(canonicalOutput, canonicalSource) ||
    isPathInsideOrEqual(canonicalSource, canonicalOutput)) {
    throw new Error('[after-pack] Native development source overlaps the packaged output')
  }
  const runtimeDir = join(resources, 'runtime')
  if (!pathEntryExists(runtimeDir)) mkdirSync(runtimeDir, { mode: 0o755 })
  chain.push(
    captureCanonicalDirectory(runtimeDir, appOutRoot, 'packaged runtime')
  )
  const uid = typeof process.getuid === 'function' ? process.getuid() : null
  const rootDevice = chain[0].identity.dev
  for (const entry of chain) {
    if (entry.identity.dev !== rootDevice ||
      (uid !== null && entry.identity.uid !== uid) ||
      (entry.identity.mode & 0o022) !== 0) {
      throw new Error(`[after-pack] ${entry.label} owner, mode, or device is unsafe`)
    }
  }
  return { appOutRoot, resources, runtimeDir, chain }
}

function publishStagedNativeFile(staged, destination, expectedMode, publish) {
  const before = lstatSync(staged)
  if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1 ||
    before.size <= 0 || (before.mode & 0o777) !== expectedMode ||
    pathEntryExists(destination)) {
    throw new Error('[after-pack] Staged native publication identity is invalid')
  }
  publish(staged, destination)
  const stagedLinked = lstatSync(staged)
  const destinationLinked = lstatSync(destination)
  if (!stagedLinked.isFile() || stagedLinked.isSymbolicLink() ||
    !destinationLinked.isFile() || destinationLinked.isSymbolicLink() ||
    stagedLinked.dev !== before.dev || stagedLinked.ino !== before.ino ||
    destinationLinked.dev !== before.dev || destinationLinked.ino !== before.ino ||
    stagedLinked.nlink !== 2 || destinationLinked.nlink !== 2 ||
    destinationLinked.uid !== before.uid || destinationLinked.gid !== before.gid ||
    destinationLinked.size !== before.size ||
    (destinationLinked.mode & 0o777) !== expectedMode) {
    throw new Error('[after-pack] Atomic native publication identity changed')
  }
  unlinkSync(staged)
  const published = lstatSync(destination)
  if (!published.isFile() || published.isSymbolicLink() ||
    published.dev !== before.dev || published.ino !== before.ino ||
    published.nlink !== 1 || published.size !== before.size ||
    published.uid !== before.uid || published.gid !== before.gid ||
    (published.mode & 0o777) !== expectedMode) {
    throw new Error('[after-pack] Published native file identity is invalid')
  }
  syncRegularFile(destination)
  return published
}

function materializePackagedDevelopmentNativeComponents(context, options = {}) {
  const repoRoot = realpathSync(options.repoRoot || join(__dirname, '..'))
  const target = dataNativeTargetContract(
    normalizePlatform(context.electronPlatformName),
    goArchForTarget(context.arch)
  )
  const strictTestCache = options.nativeComponentRoot ||
    options.cacheMountRoot ||
    options.cacheVolume
  if (strictTestCache && process.env.NODE_ENV !== 'test') {
    throw new Error('[after-pack] Native cache verifier override is test-only')
  }
  if (strictTestCache && (!options.nativeComponentRoot ||
    !options.cacheMountRoot || !options.cacheVolume)) {
    throw new Error('[after-pack] Native cache test verifier is incomplete')
  }
  const testSourceRoot = !strictTestCache &&
    process.env.NODE_ENV === 'test' && options.verifyCurrentSource === false
    ? join(repoRoot, 'runtime', 'native-components-development', target.key)
    : ''
  const cacheRoot = testSourceRoot
    ? dirname(testSourceRoot)
    : options.nativeComponentRoot ||
      requireDevelopmentNativeComponentRoot(options.env || process.env)
  const sourceRoot = testSourceRoot || join(cacheRoot, target.key)
  if (!pathEntryExists(sourceRoot)) {
    throw new Error('[after-pack] native_component_release_authority_unavailable')
  }
  if (options.copyFileSync && process.env.NODE_ENV !== 'test') {
    throw new Error('[after-pack] Native development copy override is test-only')
  }
  if (options.linkSync && process.env.NODE_ENV !== 'test') {
    throw new Error('[after-pack] Native development publish override is test-only')
  }
  let cacheVolume = null
  if (!testSourceRoot) {
    cacheVolume = options.cacheVolume || openVerifiedDevelopmentCacheVolume()
    if (!cacheVolume || typeof cacheVolume.verify !== 'function' ||
      typeof cacheVolume.close !== 'function') {
      throw new Error('[after-pack] Native cache verifier is invalid')
    }
  }
  const copy = options.copyFileSync || copyFileSync
  const publish = options.linkSync || linkSync
  const privateOptions = strictTestCache
    ? {
        mountRoot: options.cacheMountRoot,
        expectedCacheRoot: cacheRoot
      }
    : {}
  let stage = ''
  let stageIdentity = null
  try {
    if (cacheVolume) cacheVolume.verify()
    const sourceBefore = testSourceRoot
      ? {
          source: assertCanonicalDirectory(
            sourceRoot,
            cacheRoot,
            'test native development target'
          ),
          chain: []
        }
      : inspectPrivateNativeCacheSource(sourceRoot, cacheRoot, privateOptions)
    requireExactDirectoryEntries(sourceRoot, [
      ...nativeComponentManifest.components.map((component) =>
        nativeBinaryName(component, target.platform)),
      NATIVE_DEVELOPMENT_BUILD_MARKER
    ], 'files')
    const sourceFilesBefore = privateNativeSourceFileIdentities(
      sourceRoot,
      target,
      sourceBefore.source
    )
    const markerPath = join(sourceRoot, NATIVE_DEVELOPMENT_BUILD_MARKER)
    const markerBefore = readDevelopmentBuildMarker(markerPath, target, {
      repoRoot,
      verifyCurrentSource: options.verifyCurrentSource
    })
    const destinationState = assertPackagedNativeDestination(context, sourceRoot)
    const { appOutRoot, resources, runtimeDir } = destinationState
    for (const name of [
      ...markerBefore.marker.components.map((component) => component.binaryName),
      NATIVE_DEVELOPMENT_BUILD_MARKER,
      RECEIPT_FILE_NAME
    ]) {
      if (pathEntryExists(join(runtimeDir, name))) {
        throw new Error('[after-pack] Packaged native destination is already occupied')
      }
    }
    stage = mkdtempSync(join(resources, '.runtime-native-development-'))
    stageIdentity = captureCanonicalDirectory(
      stage,
      appOutRoot,
      'packaged native stage'
    )
    for (let index = 0; index < markerBefore.marker.components.length; index += 1) {
      const recorded = markerBefore.marker.components[index]
      const source = join(sourceRoot, recorded.binaryName)
      if (cacheVolume) cacheVolume.verify()
      if (sourceBefore.chain.length > 0) {
        assertDirectoryChainUnchanged(sourceBefore.chain)
      }
      assertDirectoryChainUnchanged(destinationState.chain)
      assertDirectoryChainUnchanged([stageIdentity])
      const first = assertDevelopmentBinarySample(source, target, recorded)
      const second = assertDevelopmentBinarySample(source, target, recorded)
      if (!sameNativeInspection(first, second)) {
        throw new Error(`[after-pack] Native development binary changed between samples: ${recorded.id}`)
      }
      const staged = join(stage, recorded.binaryName)
      copy(source, staged, constants.COPYFILE_EXCL)
      chmodSync(staged, 0o755)
      syncRegularFile(staged)
      assertDirectoryChainUnchanged([stageIdentity])
      const stagedIdentity = assertDevelopmentBinarySample(staged, target, recorded)
      const sourceAfterCopy = assertDevelopmentBinarySample(source, target, recorded)
      if (!sameNativeInspection(second, stagedIdentity) ||
        !sameNativeInspection(second, sourceAfterCopy)) {
        throw new Error(`[after-pack] Native development binary changed around atomic copy: ${recorded.id}`)
      }
    }
    const stagedMarker = join(stage, NATIVE_DEVELOPMENT_BUILD_MARKER)
    if (cacheVolume) cacheVolume.verify()
    if (sourceBefore.chain.length > 0) {
      assertDirectoryChainUnchanged(sourceBefore.chain)
    }
    assertDirectoryChainUnchanged(destinationState.chain)
    assertDirectoryChainUnchanged([stageIdentity])
    copy(markerPath, stagedMarker, constants.COPYFILE_EXCL)
    chmodSync(stagedMarker, 0o600)
    syncRegularFile(stagedMarker)
    if (cacheVolume) cacheVolume.verify()
    if (sourceBefore.chain.length > 0) {
      assertDirectoryChainUnchanged(sourceBefore.chain)
    }
    assertDirectoryChainUnchanged(destinationState.chain)
    assertDirectoryChainUnchanged([stageIdentity])
    const stagedMarkerIdentity = hashStableRegularFile(stagedMarker, {
      maximumBytes: MAX_NATIVE_DEVELOPMENT_MARKER_BYTES,
      requireSingleLink: true
    })
    const markerAfter = readDevelopmentBuildMarker(markerPath, target, {
      repoRoot,
      verifyCurrentSource: options.verifyCurrentSource
    })
    if (!sameDevelopmentMarkerRecord(markerBefore, markerAfter) ||
      canonicalJSON(stagedMarkerIdentity) !== canonicalJSON(markerBefore.binding)) {
      throw new Error('[after-pack] Native development marker changed around atomic copy')
    }
    for (const recorded of markerBefore.marker.components) {
      const destination = join(runtimeDir, recorded.binaryName)
      const staged = join(stage, recorded.binaryName)
      if (cacheVolume) cacheVolume.verify()
      if (sourceBefore.chain.length > 0) {
        assertDirectoryChainUnchanged(sourceBefore.chain)
      }
      assertDirectoryChainUnchanged(destinationState.chain)
      assertDirectoryChainUnchanged([stageIdentity])
      publishStagedNativeFile(staged, destination, 0o755, publish)
      assertDirectoryChainUnchanged(destinationState.chain)
      assertDirectoryChainUnchanged([stageIdentity])
    }
    const markerDestination = join(runtimeDir, NATIVE_DEVELOPMENT_BUILD_MARKER)
    if (cacheVolume) cacheVolume.verify()
    if (sourceBefore.chain.length > 0) {
      assertDirectoryChainUnchanged(sourceBefore.chain)
    }
    assertDirectoryChainUnchanged(destinationState.chain)
    assertDirectoryChainUnchanged([stageIdentity])
    publishStagedNativeFile(stagedMarker, markerDestination, 0o600, publish)
    assertDirectoryChainUnchanged(destinationState.chain)
    assertDirectoryChainUnchanged([stageIdentity])
    syncDirectory(runtimeDir)
    requireExactDirectoryEntries(sourceRoot, [
      ...nativeComponentManifest.components.map((component) =>
        nativeBinaryName(component, target.platform)),
      NATIVE_DEVELOPMENT_BUILD_MARKER
    ], 'files')
    if (cacheVolume) cacheVolume.verify()
    if (sourceBefore.chain.length > 0) {
      assertDirectoryChainUnchanged(sourceBefore.chain)
    }
    const sourceFilesAfter = privateNativeSourceFileIdentities(
      sourceRoot,
      target,
      sourceBefore.source
    )
    if (!samePrivateNativeSourceFiles(sourceFilesBefore, sourceFilesAfter)) {
      throw new Error('[after-pack] Native development source changed during packaging')
    }
    assertDirectoryChainUnchanged(destinationState.chain)
    const disposition = developmentNativeDisposition(markerBefore, target)
    verifyPackagedDevelopmentNativeDisposition(context, disposition, {
      repoRoot,
      verifyCurrentSource: options.verifyCurrentSource,
      requireMarkerBinaryIdentity: true
    })
    assertDirectoryChainUnchanged(destinationState.chain)
    assertDirectoryChainUnchanged([stageIdentity])
    rmdirSync(stage)
    stage = ''
    stageIdentity = null
    return disposition
  } finally {
    if (stage && pathEntryExists(stage)) {
      try {
        if (stageIdentity) assertDirectoryChainUnchanged([stageIdentity])
        if (readdirSync(stage).length === 0) rmdirSync(stage)
      } catch {
        // Preserve an invalid package tree; never recursively remove raced content.
      }
    }
    if (cacheVolume) cacheVolume.close()
  }
}

function verifyPackagedDevelopmentNativeDisposition(context, disposition, options = {}) {
  if (!isDevelopmentNativeDisposition(disposition) || disposition.targetKey !== packagedTargetKey(context)) {
    throw new Error('[after-pack] Packaged native development disposition is invalid')
  }
  const target = dataNativeTargetContract(
    normalizePlatform(context.electronPlatformName),
    goArchForTarget(context.arch)
  )
  const runtimeDir = join(packedResourcesDir(context), 'runtime')
  if (pathEntryExists(join(runtimeDir, RECEIPT_FILE_NAME))) {
    throw new Error('[after-pack] Native development package cannot contain a controlled release receipt')
  }
  const markerRecord = readDevelopmentBuildMarker(
    join(runtimeDir, NATIVE_DEVELOPMENT_BUILD_MARKER),
    target,
    {
      repoRoot: options.repoRoot || join(__dirname, '..'),
      verifyCurrentSource: options.verifyCurrentSource
    }
  )
  const expectedDisposition = developmentNativeDisposition(markerRecord, target)
  if (canonicalJSON(expectedDisposition) !== canonicalJSON(disposition)) {
    throw new Error('[after-pack] Packaged native development marker disposition mismatch')
  }
  for (const recorded of markerRecord.marker.components) {
    const path = join(runtimeDir, recorded.binaryName)
    const first = assertNativeBinaryTarget(path, target)
    const second = assertNativeBinaryTarget(path, target)
    if (!sameNativeInspection(first, second) ||
      first.payloadSha256 !== recorded.payloadSha256 || first.payloadSize !== recorded.payloadSize ||
      first.format !== recorded.format || first.arch !== recorded.arch ||
      (options.requireMarkerBinaryIdentity === true &&
        (first.sha256 !== recorded.binarySha256 || first.size !== recorded.binarySize))) {
      throw new Error(`[after-pack] Packaged native development component mismatch: ${recorded.id}`)
    }
  }
  return { marker: markerRecord.marker, disposition }
}

function bundledWindowsBackendPythonExecutablePath(context) {
  return join(packedResourcesDir(context), '.python-runtime', 'current', 'python', 'python.exe')
}

function bundledWindowsBackendSitePackagesDir(context) {
  return join(packedResourcesDir(context), 'python-site-packages')
}

function bundledManagedChromePath(context, relativePath = '') {
  return join(packedResourcesDir(context), relativePath)
}

function bundledOpenComputerUsePackageRoot(context) {
  return join(
    unpackedAppRoot(context),
    'packages',
    'runtime',
    'node_modules',
    'analytix-computer-use'
  )
}

function bundledOpenComputerUseNativePath(context) {
  return join(bundledOpenComputerUsePackageRoot(context), openComputerUseNativeRelativePath(context))
}

function unpackedAppRoot(context) {
  return join(packedResourcesDir(context), 'app.asar.unpacked')
}

function nodePtyRequiredPaths(context) {
  const platform = normalizePlatform(context.electronPlatformName)
  const arch = goArchForTarget(context.arch)
  const prebuildFolder = platform === 'win32'
    ? `win32-${arch === 'amd64' ? 'x64' : arch}`
    : platform === 'darwin'
      ? `darwin-${arch === 'amd64' ? 'x64' : arch}`
      : null
  const required = [
    'node_modules/node-pty/package.json'
  ]
  if (!prebuildFolder) return required
  required.push(`node_modules/node-pty/prebuilds/${prebuildFolder}/pty.node`)
  if (platform === 'win32') {
    required.push(
      `node_modules/node-pty/prebuilds/${prebuildFolder}/conpty.node`,
      `node_modules/node-pty/prebuilds/${prebuildFolder}/conpty/conpty.dll`,
      `node_modules/node-pty/prebuilds/${prebuildFolder}/winpty.dll`,
      `node_modules/node-pty/prebuilds/${prebuildFolder}/winpty-agent.exe`
    )
  } else if (platform === 'darwin') {
    required.push(`node_modules/node-pty/prebuilds/${prebuildFolder}/spawn-helper`)
  }
  return required
}

function assertExists(path, label) {
  if (!existsSync(path)) {
    throw new Error(`[after-pack] Missing ${label}: ${path}`)
  }
}

function toPosixPath(path) {
  return path.split(sep).join('/')
}

function collectFiles(root, dir = root, acc = []) {
  if (!existsSync(dir)) return acc
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name)
    if (entry.isDirectory()) {
      collectFiles(root, fullPath, acc)
    } else if (entry.isFile()) {
      acc.push(fullPath)
    }
  }
  return acc
}

function removePath(path) {
  rmSync(path, { recursive: true, force: true })
}

function isPathInsideOrEqual(parent, child) {
  const pathFromParent = relative(parent, child)
  return pathFromParent === '' || (!pathFromParent.startsWith('..') && !isAbsolute(pathFromParent))
}

function pruneRuntimeNodeModuleSelfLinks(context) {
  const root = unpackedAppRoot(context)
  const runtimeNodeModules = join(root, 'packages', 'runtime', 'node_modules')
  if (!existsSync(runtimeNodeModules)) return

  const realRoot = realpathSync(root)
  for (const entry of readdirSync(runtimeNodeModules, { withFileTypes: true })) {
    if (!entry.isSymbolicLink()) continue
    const fullPath = join(runtimeNodeModules, entry.name)
    let target = ''
    try {
      target = realpathSync(fullPath)
    } catch {
      removePath(fullPath)
      continue
    }
    if (isPathInsideOrEqual(realRoot, target)) {
      removePath(fullPath)
    }
  }
}

function pruneDirectoryChildren(root, keepNames) {
  if (!existsSync(root)) return
  const keep = new Set(keepNames)
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue
    if (!keep.has(entry.name)) {
      removePath(join(root, entry.name))
    }
  }
}

function pruneDirectoryEntries(root, keepNames) {
  if (!existsSync(root)) return
  const keep = new Set(keepNames)
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    if (!keep.has(entry.name)) removePath(join(root, entry.name))
  }
}

function pruneBuildOnlyDirectories(root) {
  if (!existsSync(root)) return
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const fullPath = join(root, entry.name)
    if (!entry.isDirectory()) continue
    if (BUILD_ONLY_DIRECTORY_NAMES.has(entry.name)) {
      removePath(fullPath)
      continue
    }
    pruneBuildOnlyDirectories(fullPath)
  }
}

function isBuildOnlyFileName(name) {
  if (BUILD_ONLY_FILE_NAMES.has(name)) return true
  if (/^(?:readme|changelog)(?:\.[^.]+)?$/i.test(name)) return true
  if (name.startsWith('._')) return true
  if (name === '.DS_Store') return true
  if (/^tsconfig.*\.json$/i.test(name)) return true
  if (/\.(?:map|pyc|ts)$/i.test(name)) return true
  if (/\.d\.ts$/i.test(name)) return true
  return false
}

function pruneBuildOnlyFiles(root) {
  for (const file of collectFiles(root)) {
    const name = file.split(sep).at(-1) || ''
    if (isBuildOnlyFileName(name)) {
      removePath(file)
    }
  }
}

function npmCommand(args, platform = process.platform) {
  if (platform === 'win32') {
    return {
      command: 'cmd.exe',
      args: ['/d', '/s', '/c', 'npm', ...args]
    }
  }
  return { command: 'npm', args }
}

function requireCanonicalNodePtyDirectory(appOutRoot, directory, label) {
  const canonicalAppOutRoot = resolve(appOutRoot)
  const canonicalDirectory = resolve(directory)
  let stat
  try {
    stat = lstatSync(canonicalDirectory)
  } catch {
    throw new Error(`[after-pack] Packaged node-pty ${label} directory is unavailable`)
  }
  if (
    !isPathInsideOrEqual(canonicalAppOutRoot, canonicalDirectory) ||
    !stat.isDirectory() || stat.isSymbolicLink() ||
    realpathSync(canonicalDirectory) !== canonicalDirectory
  ) {
    throw new Error(`[after-pack] Packaged node-pty ${label} directory is not canonical`)
  }
  return stat
}

function preflightNodePtyForTarget(context, options = {}) {
  const appOutRoot = resolve(context.appOutDir)
  requireCanonicalNodePtyDirectory(appOutRoot, appOutRoot, 'app output')
  const unpackedRoot = unpackedAppRoot(context)
  requireCanonicalNodePtyDirectory(appOutRoot, unpackedRoot, 'unpacked app')
  const nodePtyRoot = join(unpackedRoot, 'node_modules', 'node-pty')
  requireCanonicalNodePtyDirectory(appOutRoot, nodePtyRoot, 'root')

  const platform = normalizePlatform(context.electronPlatformName)
  const arch = goArchForTarget(context.arch)
  const prebuildFolder = platform === 'win32'
    ? `win32-${arch === 'amd64' ? 'x64' : arch}`
    : platform === 'darwin'
      ? `darwin-${arch === 'amd64' ? 'x64' : arch}`
      : ''
  if (prebuildFolder) {
    const prebuildsRoot = join(nodePtyRoot, 'prebuilds')
    requireCanonicalNodePtyDirectory(appOutRoot, prebuildsRoot, 'prebuilds')
    requireCanonicalNodePtyDirectory(
      appOutRoot,
      join(prebuildsRoot, prebuildFolder),
      'target prebuild'
    )
  }
  if (options.requireDarwinRuntime === true) {
    if (platform !== 'darwin') {
      throw new Error('[after-pack] Packaged Darwin node-pty preflight requires Darwin')
    }
    requireCanonicalNodePtyDirectory(
      appOutRoot,
      join(nodePtyRoot, 'lib'),
      'runtime library'
    )
  }
  return { nodePtyRoot, prebuildFolder }
}

function pruneNodePtyForTarget(context) {
  const root = unpackedAppRoot(context)
  const nodePtyRoot = join(root, 'node_modules', 'node-pty')
  if (!existsSync(nodePtyRoot)) return

  const { prebuildFolder } = preflightNodePtyForTarget(context)

  removePath(join(nodePtyRoot, 'build'))
  removePath(join(nodePtyRoot, 'third_party'))
  if (prebuildFolder) {
    pruneDirectoryChildren(join(nodePtyRoot, 'prebuilds'), [prebuildFolder])
  }
}

function pruneBetterSqliteForRuntime(context) {
  const root = unpackedAppRoot(context)
  const sqliteRoot = join(root, 'node_modules', 'better-sqlite3')
  if (!existsSync(sqliteRoot)) return

  removePath(join(sqliteRoot, 'deps'))
  removePath(join(sqliteRoot, 'src'))
}

function napiRsCanvasKeepName(context) {
  const platform = normalizePlatform(context.electronPlatformName)
  const arch = goArchForTarget(context.arch)
  if (platform === 'win32') return arch === 'amd64' ? 'canvas-win32-x64-msvc' : 'canvas-win32-arm64-msvc'
  if (platform === 'darwin') return arch === 'amd64' ? 'canvas-darwin-x64' : 'canvas-darwin-arm64'
  if (platform === 'linux') return arch === 'amd64' ? 'canvas-linux-x64-gnu' : 'canvas-linux-arm64-gnu'
  return ''
}

function pruneNapiRsCanvasForTarget(context) {
  const root = unpackedAppRoot(context)
  const scopeRoot = join(root, 'node_modules', '@napi-rs')
  if (!existsSync(scopeRoot)) return

  const keepName = napiRsCanvasKeepName(context)
  for (const entry of readdirSync(scopeRoot, { withFileTypes: true })) {
    if (!entry.isDirectory() || !entry.name.startsWith('canvas-')) continue
    if (entry.name !== keepName) {
      removePath(join(scopeRoot, entry.name))
    }
  }
}

function pruneConditionalNativeDependenciesForTarget(context) {
  const root = unpackedAppRoot(context)
  const platform = normalizePlatform(context.electronPlatformName)
  const arch = goArchForTarget(context.arch)
  const targetLibnutPackage = platform === 'win32'
    ? 'libnut-win32'
    : platform === 'darwin'
      ? 'libnut-darwin'
      : platform === 'linux'
        ? 'libnut-linux'
        : ''
  if (!targetLibnutPackage) {
    throw new Error(`[after-pack] Unsupported conditional native target: ${platform}`)
  }

  for (const nodeModulesRoot of [
    join(root, 'node_modules'),
    join(root, 'packages', 'runtime', 'node_modules')
  ]) {
    const computerUseRoot = join(nodeModulesRoot, '@computer-use')
    if (existsSync(join(computerUseRoot, 'libnut', 'package.json'))) {
      for (const packageName of ['libnut-darwin', 'libnut-linux', 'libnut-win32']) {
        if (packageName !== targetLibnutPackage) removePath(join(computerUseRoot, packageName))
      }
      assertExists(
        join(computerUseRoot, targetLibnutPackage, 'package.json'),
        `target ${targetLibnutPackage} package`
      )
      assertExists(
        join(computerUseRoot, targetLibnutPackage, 'build', 'Release', 'libnut.node'),
        `target ${targetLibnutPackage} native module`
      )
      if (platform !== 'darwin') {
        removePath(join(computerUseRoot, 'node-mac-permissions'))
        removePath(join(
          computerUseRoot,
          targetLibnutPackage,
          'node_modules',
          '@computer-use',
          'node-mac-permissions'
        ))
      }
    }

    const clipboardyRoot = join(nodeModulesRoot, 'clipboardy')
    if (!existsSync(join(clipboardyRoot, 'package.json'))) continue
    const fallbacksRoot = join(clipboardyRoot, 'fallbacks')
    if (platform === 'darwin') {
      removePath(fallbacksRoot)
    } else if (platform === 'win32') {
      removePath(join(fallbacksRoot, 'linux'))
      if (arch !== 'amd64') {
        throw new Error(`[after-pack] clipboardy target fallback unavailable for win32-${arch}`)
      }
      const windowsRoot = join(fallbacksRoot, 'windows')
      pruneDirectoryEntries(windowsRoot, ['clipboard_x86_64.exe'])
      assertExists(join(windowsRoot, 'clipboard_x86_64.exe'), 'target clipboardy fallback')
    } else {
      removePath(join(fallbacksRoot, 'windows'))
      if (arch === 'amd64') {
        assertExists(join(fallbacksRoot, 'linux', 'xsel'), 'target clipboardy fallback')
      } else {
        removePath(join(fallbacksRoot, 'linux'))
      }
    }
  }
}

function pruneOpenComputerUsePackageForTarget(context, packageRoot) {
  if (!existsSync(packageRoot)) return

  const platform = normalizePlatform(context.electronPlatformName)
  const arch = goArchForTarget(context.arch)
  const distRoot = join(packageRoot, 'dist')
  if (!existsSync(distRoot)) return

  if (platform === 'win32') {
    removePath(join(distRoot, 'linux'))
    removePath(join(distRoot, 'Analytix Computer Use.app'))
    pruneDirectoryChildren(join(distRoot, 'windows'), [arch === 'amd64' ? 'amd64' : 'arm64'])
  } else if (platform === 'darwin') {
    removePath(join(distRoot, 'linux'))
    removePath(join(distRoot, 'windows'))
  } else if (platform === 'linux') {
    removePath(join(distRoot, 'windows'))
    removePath(join(distRoot, 'Analytix Computer Use.app'))
    pruneDirectoryChildren(join(distRoot, 'linux'), [arch === 'amd64' ? 'amd64' : 'arm64'])
  }
}

function prunePackedSymlinks(root) {
  if (!existsSync(root)) return
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const fullPath = join(root, entry.name)
    if (entry.isSymbolicLink()) {
      removePath(fullPath)
      continue
    }
    if (entry.isDirectory()) {
      prunePackedSymlinks(fullPath)
    }
  }
}

function pruneWindowsOnlyArtifacts(context) {
  if (normalizePlatform(context.electronPlatformName) !== 'win32') return

  const resources = packedResourcesDir(context)
  for (const name of ANALYTIX_DATA_NATIVE_REQUIRED_TOOLS) {
    removePath(join(resources, 'runtime', name))
  }
  prunePackedSymlinks(unpackedAppRoot(context))
}

function prunePackedProductionArtifacts(context) {
  const root = unpackedAppRoot(context)
  pruneRuntimeNodeModuleSelfLinks(context)
  pruneNodePtyForTarget(context)
  pruneBetterSqliteForRuntime(context)
  pruneNapiRsCanvasForTarget(context)
  pruneConditionalNativeDependenciesForTarget(context)

  const runtimeComputerUseRoot = bundledOpenComputerUsePackageRoot(context)
  pruneOpenComputerUsePackageForTarget(context, runtimeComputerUseRoot)

  const rootComputerUseRoot = join(root, 'node_modules', 'analytix-computer-use')
  if (rootComputerUseRoot !== runtimeComputerUseRoot && existsSync(runtimeComputerUseRoot)) {
    removePath(rootComputerUseRoot)
  }

  pruneBuildOnlyDirectories(root)
  pruneBuildOnlyFiles(root)
  for (const resourceRoot of [
    join(packedResourcesDir(context), 'backend'),
    join(packedResourcesDir(context), 'plugins'),
    join(packedResourcesDir(context), '.python-runtime'),
    join(packedResourcesDir(context), 'python-site-packages')
  ]) {
    pruneBuildOnlyDirectories(resourceRoot)
    pruneBuildOnlyFiles(resourceRoot)
  }
  pruneWindowsOnlyArtifacts(context)
}

function prunePackedAnalytixDependencies(context) {
  const root = unpackedAppRoot(context)
  const runtimeDir = join(root, 'packages', 'runtime')
  if (!existsSync(runtimeDir)) return

  assertExists(join(runtimeDir, 'package.json'), 'analytix runtime package manifest')
  assertExists(join(runtimeDir, 'node_modules'), 'analytix runtime node_modules')

  const prune = npmCommand(['prune', '--omit=dev', '--ignore-scripts'])
  execFileSync(prune.command, prune.args, {
    cwd: runtimeDir,
    env: {
      ...process.env,
      npm_config_audit: 'false',
      npm_config_fund: 'false'
    },
    stdio: 'inherit'
  })

  // Keep native SQLite on the app root dependency so electron-builder's
  // native-module rebuild owns the target arch and Electron ABI.
  assertExists(
    join(root, 'node_modules', 'better-sqlite3', 'package.json'),
    'root better-sqlite3 dependency'
  )
  rmSync(join(runtimeDir, 'node_modules', 'better-sqlite3'), { recursive: true, force: true })
}

function findLocalOpenComputerUsePackageRoot(options = {}) {
  const exists = options.existsSync || existsSync
  const realpath = options.realpathSync || realpathSync
  const candidates = [
    process.env.ANALYTIX_COMPUTER_USE_PACKAGE_ROOT,
    join(__dirname, '..', 'packages', 'runtime', 'node_modules', 'analytix-computer-use'),
    join(__dirname, '..', 'node_modules', 'analytix-computer-use')
  ].filter(Boolean)

  for (const candidate of candidates) {
    if (
      exists(join(candidate, 'package.json')) &&
      exists(join(candidate, 'bin', 'analytix-computer-use'))
    ) {
      return realpath(candidate)
    }
  }

  throw new Error(
    `[after-pack] Could not find local analytix-computer-use package. ` +
      `Run npm ci or set ANALYTIX_COMPUTER_USE_PACKAGE_ROOT.`
  )
}

function ensureBundledOpenComputerUsePackage(context, options = {}) {
  const exists = options.existsSync || existsSync
  const copy = options.cpSync || cpSync
  const mkdir = options.mkdirSync || mkdirSync
  const remove = options.rmSync || rmSync
  const destination = bundledOpenComputerUsePackageRoot(context)

  if (
    exists(join(destination, 'package.json')) &&
    exists(bundledOpenComputerUseNativePath(context))
  ) {
    return { copied: false, destination }
  }

  const source = options.sourceRoot || findLocalOpenComputerUsePackageRoot(options)
  remove(destination, { recursive: true, force: true })
  mkdir(dirname(destination), { recursive: true })
  copy(source, destination, { recursive: true, dereference: true })

  assertExists(join(destination, 'package.json'), 'Analytix Computer Use package manifest')
  assertExists(
    join(destination, 'bin', 'analytix-computer-use'),
    'Analytix Computer Use package CLI'
  )
  return { copied: true, source, destination }
}

function ensureBundledOpenComputerUseWindowsShims(context) {
  if (normalizePlatform(context.electronPlatformName) !== 'win32') return
  const root = unpackedAppRoot(context)
  const binDir = join(root, 'packages', 'runtime', 'node_modules', '.bin')
  mkdirSync(binDir, { recursive: true })

  const shims = new Map([
    ['analytix-computer-use.cmd', 'bin\\analytix-computer-use'],
    ['analytix-computer-use-mcp.cmd', 'bin\\analytix-computer-use-mcp'],
    ['open-computer-use.cmd', 'bin\\open-computer-use'],
    ['open-computer-use-mcp.cmd', 'bin\\open-computer-use-mcp']
  ])
  for (const [name, target] of shims) {
    writeFileSync(
      join(binDir, name),
      [
        '@ECHO off',
        'SETLOCAL',
        'SET "dp0=%~dp0"',
        `node "%dp0%\\..\\analytix-computer-use\\${target}" %*`
      ].join('\r\n') + '\r\n',
      'utf8'
    )
  }
}

function validateBundledAnalytixRuntime(context, options = {}) {
  const root = unpackedAppRoot(context)
  const resources = packedResourcesDir(context)
  const requiredPaths = [
    ...ANALYTIX_RUNTIME_REQUIRED_PATHS,
    ...(normalizePlatform(context.electronPlatformName) === 'win32'
      ? [
          'packages/runtime/node_modules/.bin/analytix-computer-use.cmd',
          'packages/runtime/node_modules/.bin/open-computer-use.cmd'
        ]
      : [])
  ]
  for (const relativePath of requiredPaths) {
    assertExists(join(root, relativePath), relativePath)
  }
  for (const relativePath of ANALYTIX_RUNTIME_FORBIDDEN_PATHS) {
    const fullPath = join(root, relativePath)
    if (existsSync(fullPath)) {
      throw new Error(`[after-pack] Retired TypeScript agent runtime path must not ship in production app: ${fullPath}`)
    }
  }
  assertExists(bundledGoRuntimeServerPath(context), 'Go runtime server binary')
  for (const name of (isCoreContext(context) ? [] : ANALYTIX_DATA_NATIVE_REQUIRED_TOOLS)) {
    assertExists(
      bundledDataAnalysisNativeToolPath(context, name),
      `data analysis native tool (${name})`
    )
  }
  const nativeDisposition = options.nativeDisposition
    ? normalizeNativeDisposition(options.nativeDisposition)
    : null
  if (isCoreContext(context)) {
    if (!isCoreDisposition(nativeDisposition)) throw Error('core_profile_native_disposition_mismatch')
    assertCoreResourcesAbsent(resources)
  } else if (nativeDisposition?.kind === NATIVE_DISPOSITION_DEVELOPMENT) {
    verifyPackagedDevelopmentNativeDisposition(context, nativeDisposition, {
      repoRoot: options.repoRoot,
      verifyCurrentSource: options.verifyCurrentSource,
      requireMarkerBinaryIdentity: options.requireMarkerBinaryIdentity === true
    })
  } else {
    let nativeComponentVerifier = verifyPackagedComponents
    if (options.testOnlyNativeComponentVerifier !== undefined) {
      if (process.env.NODE_ENV !== 'test' || typeof options.testOnlyNativeComponentVerifier !== 'function') {
        throw new Error('[after-pack] Native component verifier override is test-only')
      }
      nativeComponentVerifier = options.testOnlyNativeComponentVerifier
    }
    nativeComponentVerifier(
      join(resources, 'runtime'),
      dataNativeTargetContract(
        normalizePlatform(context.electronPlatformName),
        goArchForTarget(context.arch)
      ),
      {
        verifyExecution: options.verifyNativeExecution !== false,
        requireBuildIdentity: normalizePlatform(context.electronPlatformName) !== 'win32',
        authorityUse: options.authorityUse
      }
    )
  }
  if (normalizePlatform(context.electronPlatformName) === 'win32') {
    assertExists(join(resources, 'backend', 'app', 'main.py'), 'data analysis backend entrypoint')
    assertExists(
      bundledWindowsBackendPythonExecutablePath(context),
      'Windows data analysis Python runtime'
    )
    const sitePackages = bundledWindowsBackendSitePackagesDir(context)
    assertExists(sitePackages, 'Windows data analysis Python site-packages')
    assertExists(join(sitePackages, 'analytix-site-packages-manifest.json'), 'Windows data analysis wheel manifest')
    assertExists(join(sitePackages, 'fastapi'), 'Windows backend FastAPI dependency')
    assertExists(join(sitePackages, 'uvicorn'), 'Windows backend Uvicorn dependency')
    assertExists(join(sitePackages, 'pandas'), 'Windows backend pandas dependency')
    assertExists(join(sitePackages, 'duckdb'), 'Windows backend duckdb dependency')
    if (options.verifyPythonAuthority !== false) {
      verifyPythonRuntime(join(resources, '.python-runtime', 'current'))
      verifySitePackages(sitePackages)
    }
    for (const relativePath of ANALYTIX_MANAGED_CHROME_REQUIRED_PATHS) {
      assertExists(
        bundledManagedChromePath(context, relativePath),
        `managed Chrome resource (${relativePath})`
      )
    }
  }
  for (const relativePath of ANALYTIX_RUNTIME_GO_FORBIDDEN_PATHS) {
    const fullPath = join(root, relativePath)
    if (existsSync(fullPath)) {
      throw new Error(`[after-pack] Forbidden Go runtime internal validation file in production app: ${fullPath}`)
    }
  }
  verifyPackagedDocumentRuntime(context, { required: options.requireDocumentRuntime === true })
  for (const fullPath of collectFiles(join(root, 'packages', 'runtime-go'))) {
    const relativePath = toPosixPath(relative(root, fullPath))
    for (const pattern of ANALYTIX_RUNTIME_GO_FORBIDDEN_PATTERNS) {
      if (pattern.test(relativePath)) {
        throw new Error(`[after-pack] Forbidden Go runtime internal validation file in production app: ${fullPath}`)
      }
    }
  }
  assertExists(
    join(root, 'node_modules', 'better-sqlite3', 'package.json'),
    'root better-sqlite3 dependency'
  )
  assertExists(
    bundledOpenComputerUseNativePath(context),
    'Analytix Computer Use native runtime'
  )
  for (const relativePath of nodePtyRequiredPaths(context)) {
    assertExists(join(root, relativePath), relativePath)
  }
  if (normalizePlatform(context.electronPlatformName) !== 'win32') {
    chmodSync(join(root, 'packages/runtime/node_modules/analytix-computer-use/bin/analytix-computer-use'), 0o755)
    chmodSync(bundledOpenComputerUseNativePath(context), 0o755)
    for (const name of (isCoreContext(context) ? [] : ANALYTIX_DATA_NATIVE_REQUIRED_TOOLS)) {
      chmodSync(bundledDataAnalysisNativeToolPath(context, name), 0o755)
    }
  }
}

function verifyPackagedNativeBeforeRuntimeBuild(context, options = {}) {
  const platform = normalizePlatform(context.electronPlatformName)
  const arch = goArchForTarget(context.arch)
  const target = dataNativeTargetContract(platform, arch)
  const runtimeDir = join(packedResourcesDir(context), 'runtime')
  const receipt = verifyPackagedComponents(runtimeDir, target, {
    verifyExecution: options.verifyNativeExecution !== false,
    requireBuildIdentity: platform !== 'win32',
    authorityUse: options.authorityUse
  })
  const receiptSHA256 = sha256File(join(runtimeDir, RECEIPT_FILE_NAME))
  if (!/^[0-9a-f]{64}$/.test(receiptSHA256) || receipt.manifestSha256 !== nativeManifestDigest || receipt.targetKey !== target.key) {
    throw new Error('[after-pack] Packaged native trust is invalid before Go runtime build')
  }
  const signingTrust = platform === 'darwin'
    ? darwinSigningTrust(options.env || process.env)
    : { signingPolicySHA256: '', signingMode: '', appleTeamIdentifier: '' }
  return { receiptSHA256, manifestSHA256: nativeManifestDigest, targetKey: target.key, ...signingTrust }
}

function darwinSigningTrust(env) {
  const developerID = Boolean(
    env.CSC_LINK || env.CSC_NAME || env.CSC_KEY_PASSWORD || env.MAC_SIGN === '1'
  )
  return {
    signingPolicySHA256: macSigningPolicyDigest,
    signingMode: developerID ? 'developer-id' : 'ad-hoc',
    appleTeamIdentifier: developerID ? requireOfficialTeamIdentifier() : ''
  }
}

function buildBundledGoRuntimeServer(context, nativeTrust, options = {}) {
  const exec = options.execFileSync || execFileSync
  const chmod = options.chmodSync || chmodSync
  const output = bundledGoRuntimeServerPath(context)
  const goos = goOSForPlatform(context.electronPlatformName)
  const goarch = goArchForTarget(context.arch)
  const nativeDisposition = normalizeNativeDisposition(nativeTrust)
  if (isCoreDisposition(nativeDisposition) !== isCoreContext(context)) throw Error('core_profile_authority_mismatch')
  if (nativeDisposition.targetKey !== packagedTargetKey(context)) {
    throw new Error('[after-pack] Exact packaged native disposition is required before building runtime-server')
  }
  const runtimeNativeTrust = nativeDisposition.kind === NATIVE_DISPOSITION_CONTROLLED_RELEASE
    ? {
        receiptSHA256: nativeDisposition.receiptSha256,
        manifestSHA256: nativeDisposition.manifestSha256,
        targetKey: nativeDisposition.targetKey,
        signingPolicySHA256: nativeDisposition.signingPolicySha256,
        signingMode: nativeDisposition.signingMode,
        appleTeamIdentifier: nativeDisposition.appleTeamIdentifier
      }
    : {
        receiptSHA256: '',
        manifestSHA256: '',
        targetKey: '',
        // Local-build admission still consumes the compiled Host-owned
        // platform policy.  Receipt/package/component authority is resolved
        // and revalidated from the exact non-publishable package at runtime;
        // these two values cannot be supplied by CLI, env, or package
        // metadata and therefore do not upgrade the development artifact.
        signingPolicySHA256: goos === 'darwin' ? macSigningPolicyDigest : '',
        signingMode: goos === 'darwin' ? 'ad-hoc' : '',
        appleTeamIdentifier: ''
      }
  if (
    goos === 'darwin' &&
    nativeDisposition.kind === NATIVE_DISPOSITION_CONTROLLED_RELEASE &&
    (!/^[0-9a-f]{64}$/.test(runtimeNativeTrust.signingPolicySHA256 || '') ||
      !['ad-hoc', 'developer-id'].includes(runtimeNativeTrust.signingMode) ||
      (runtimeNativeTrust.signingMode === 'ad-hoc' && runtimeNativeTrust.appleTeamIdentifier !== '') ||
      (runtimeNativeTrust.signingMode === 'developer-id' && !/^[A-Z0-9]{10}$/.test(runtimeNativeTrust.appleTeamIdentifier || '')))
  ) {
    throw new Error('[after-pack] Exact macOS signing trust is required before building runtime-server')
  }
  const projectedGoCache = projectGoDevelopmentCacheEnvironment(options.env || process.env)
  const toolchain = options.toolchain || resolvePinnedGoToolchain({
    env: projectedGoCache.environment,
    authorizedModuleCache: projectedGoCache.authorizedModuleCache
  })
  mkdirSync(dirname(output), { recursive: true })
  const privateBuildRoot = (options.mkdtempSync || mkdtempSync)(join(dirname(output), '.runtime-server-build-'))
  const temporaryOutput = join(privateBuildRoot, runtimeServerBinaryName(context))
  const environment = hermeticGoBuildEnvironment(toolchain, { goos, goarch }, {
    cache: join(privateBuildRoot, 'cache'),
    path: join(privateBuildRoot, 'path'),
    temp: join(privateBuildRoot, 'temp')
  })
  for (const directory of [environment.GOCACHE, environment.GOPATH, environment.GOTMPDIR]) {
    mkdirSync(directory, { recursive: true, mode: 0o700 })
  }
  const ldflags = [
    `-X analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs.embeddedReleaseProfile=${isCoreContext(context) ? 'core' : 'full'}`,
    ...(isControlledCoreDisposition(nativeDisposition) ? [`-X analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs.embeddedCoreQualification=${sha256Bytes(JSON.stringify(nativeDisposition))}`] : []),
    `-X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedReceiptSHA256=${runtimeNativeTrust.receiptSHA256}`,
    `-X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedManifestSHA256=${runtimeNativeTrust.manifestSHA256}`,
    `-X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedTargetKey=${runtimeNativeTrust.targetKey}`,
    `-X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedSigningPolicySHA256=${runtimeNativeTrust.signingPolicySHA256}`,
    `-X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedSigningMode=${runtimeNativeTrust.signingMode}`,
    `-X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedAppleTeamIdentifier=${runtimeNativeTrust.appleTeamIdentifier}`
  ].join(' ')
  const cwd = join(__dirname, '..', 'packages', 'runtime-go')
  try {
    exec(toolchain.executable, ['mod', 'verify'], { cwd, env: environment, stdio: 'inherit' })
    exec(toolchain.executable, [
      'build', '-mod=readonly', '-buildvcs=false', '-trimpath', '-tags', 'analytix_prod',
      '-ldflags', ldflags, '-o', temporaryOutput, './cmd/runtime-server'
    ], { cwd, env: environment, stdio: 'inherit' })
    const outputStat = lstatSync(temporaryOutput)
    if (!outputStat.isFile() || outputStat.isSymbolicLink() || outputStat.size <= 0 || outputStat.mode & 0o022) {
      throw new Error('[after-pack] Hermetic Go build did not produce a trusted regular runtime-server')
    }
    renameSync(temporaryOutput, output)
    if (normalizePlatform(context.electronPlatformName) !== 'win32') {
      chmod(output, 0o755)
    }
  } finally {
    rmSync(privateBuildRoot, { recursive: true, force: true })
  }
  return { output, goos, goarch, toolchainKey: toolchain.key, nativeTrust: { ...nativeTrust } }
}

async function signBundledWindowsRuntime(context, output, options = {}) {
  if (normalizePlatform(context.electronPlatformName) !== 'win32') return false
  const signIf = options.signIf || context.packager?.signIf
  if (typeof signIf !== 'function') {
    throw new Error('[after-pack] electron-builder Windows signer is unavailable for runtime-server.exe')
  }
  const signed = await signIf.call(context.packager, output)
  if (signed !== true && process.env.ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA !== '1') {
    throw new Error('[after-pack] Windows runtime-server.exe was not signed')
  }
  return signed === true
}

// node-pty execs a bundled `spawn-helper` binary to fork the child shell.
// asar unpacking can drop the executable bit, which makes every PTY spawn
// fail with `posix_spawnp`. Re-chmod every bundled helper after packing so
// the built-in terminal works in the shipped app. Non-fatal: best effort.
function ensureNodePtyHelpersExecutable(context) {
  const root = unpackedAppRoot(context)
  const prebuildsDir = join(root, 'node_modules', 'node-pty', 'prebuilds')
  if (!existsSync(prebuildsDir)) return
  for (const folder of readdirSync(prebuildsDir)) {
    const helper = join(prebuildsDir, folder, 'spawn-helper')
    if (!existsSync(helper)) continue
    try {
      chmodSync(helper, 0o755)
    } catch (error) {
      console.warn(`[after-pack] could not chmod node-pty spawn-helper (${folder}):`, error.message)
    }
  }
}

function packagedDarwinNodePtyRoot(context) {
  return join(
    appBundlePath(context),
    'Contents',
    'MacOS',
    PACKAGED_DARWIN_NODE_PTY_DIRECTORY_NAME
  )
}

function retainPackagedDarwinNodePtyEntries(root, keepNames) {
  const keep = new Set(keepNames)
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    if (!keep.has(entry.name)) removePath(join(root, entry.name))
  }
}

function requireExactPackagedDarwinNodePtyEntries(root, expectedNames) {
  const rootStat = lstatSync(root)
  if (
    !rootStat.isDirectory() || rootStat.isSymbolicLink() ||
    realpathSync(root) !== resolve(root)
  ) {
    throw new Error('[after-pack] Packaged Darwin node-pty directory is not canonical')
  }
  const entries = readdirSync(root, { withFileTypes: true })
  const actualNames = entries.map((entry) => entry.name).sort()
  const expected = [...expectedNames].sort()
  if (
    actualNames.length !== expected.length ||
    actualNames.some((name, index) => name !== expected[index]) ||
    entries.some((entry) => entry.isSymbolicLink())
  ) {
    throw new Error('[after-pack] Packaged Darwin node-pty inventory is invalid')
  }
}

function prunePackagedDarwinNodePtyRuntime(root) {
  // node-addon-api supplies rebuild headers only; the prebuilt runtime never loads it.
  retainPackagedDarwinNodePtyEntries(root, [
    'LICENSE',
    'lib',
    'package.json',
    'prebuilds'
  ])
  retainPackagedDarwinNodePtyEntries(
    join(root, 'lib'),
    PACKAGED_DARWIN_NODE_PTY_RUNTIME_LIBRARIES
  )
}

function validatePackagedDarwinNodePty(context, root) {
  const arch = goArchForTarget(context.arch)
  const prebuildFolder = `darwin-${arch === 'amd64' ? 'x64' : arch}`
  const rootStat = lstatSync(root)
  if (
    !rootStat.isDirectory() || rootStat.isSymbolicLink() ||
    realpathSync(root) !== resolve(root) || (rootStat.mode & 0o022) !== 0
  ) {
    throw new Error('[after-pack] Packaged Darwin node-pty root is not canonical')
  }
  requireExactPackagedDarwinNodePtyEntries(root, [
    'LICENSE',
    'lib',
    'package.json',
    'prebuilds'
  ])
  requireExactDirectoryEntries(
    join(root, 'lib'),
    PACKAGED_DARWIN_NODE_PTY_RUNTIME_LIBRARIES,
    'files'
  )
  requireExactDirectoryEntries(join(root, 'prebuilds'), [prebuildFolder], 'directories')
  requireExactDirectoryEntries(
    join(root, 'prebuilds', prebuildFolder),
    ['pty.node', 'spawn-helper'],
    'files'
  )
  for (const relativePath of [
    'LICENSE',
    'package.json',
    'lib/index.js',
    `prebuilds/${prebuildFolder}/pty.node`,
    `prebuilds/${prebuildFolder}/spawn-helper`
  ]) {
    const filePath = join(root, ...relativePath.split('/'))
    hashStableRegularFile(filePath, {
      maximumBytes: STAGED_PAYLOAD_MAX_FILE_BYTES,
      requireSingleLink: true
    })
    if ((lstatSync(filePath).mode & 0o022) !== 0) {
      throw new Error('[after-pack] Packaged Darwin node-pty file is writable by an untrusted group or user')
    }
  }
  const helper = join(root, 'prebuilds', prebuildFolder, 'spawn-helper')
  if ((lstatSync(helper).mode & 0o111) === 0) {
    throw new Error('[after-pack] Packaged Darwin node-pty spawn-helper is not executable')
  }
  return { root, helper, prebuildFolder, rootStat }
}

function relocatePackagedDarwinNodePty(context) {
  if (normalizePlatform(context.electronPlatformName) !== 'darwin') return null
  const source = join(unpackedAppRoot(context), 'node_modules', 'node-pty')
  const destination = packagedDarwinNodePtyRoot(context)
  if (pathEntryExists(destination)) {
    throw new Error('[after-pack] Packaged Darwin node-pty destination already exists')
  }
  const destinationParent = dirname(destination)
  const destinationParentStat = lstatSync(destinationParent)
  if (
    !destinationParentStat.isDirectory() || destinationParentStat.isSymbolicLink() ||
    realpathSync(destinationParent) !== resolve(destinationParent)
  ) {
    throw new Error('[after-pack] Packaged Darwin node-pty destination parent is not canonical')
  }
  const preflight = preflightNodePtyForTarget(context, {
    requireDarwinRuntime: true
  })
  if (preflight.nodePtyRoot !== source) {
    throw new Error('[after-pack] Packaged Darwin node-pty source binding changed')
  }
  const { prebuildFolder } = preflight
  prunePackagedDarwinNodePtyRuntime(source)
  chmodSync(join(source, 'prebuilds', prebuildFolder, 'spawn-helper'), 0o755)
  const before = validatePackagedDarwinNodePty(context, source)
  renameSync(source, destination)
  if (pathEntryExists(source)) {
    throw new Error('[after-pack] Packaged Darwin node-pty source remained after relocation')
  }
  const after = validatePackagedDarwinNodePty(context, destination)
  if (!sameRelocatedDirectoryIdentity(before.rootStat, after.rootStat)) {
    throw new Error('[after-pack] Packaged Darwin node-pty changed during relocation')
  }
  return after
}

function bundledOpenComputerUseMacAppPath(context) {
  return join(bundledOpenComputerUsePackageRoot(context), 'dist', 'Analytix Computer Use.app')
}

function validateBundledOpenComputerUseMacApp(context, options = {}) {
  if (normalizePlatform(context.electronPlatformName) !== 'darwin') return
  const exec = options.execFileSync || execFileSync
  const appPath = bundledOpenComputerUseMacAppPath(context)
  const contents = join(appPath, 'Contents')
  const infoPlist = join(contents, 'Info.plist')
  const resources = join(contents, 'Resources')
  const executable = join(contents, 'MacOS', 'OpenComputerUse')
  const iconPath = join(resources, 'AnalytixComputerUse.icns')

  assertExists(infoPlist, 'Analytix Computer Use macOS Info.plist')
  assertExists(executable, 'Analytix Computer Use macOS executable')
  assertExists(iconPath, 'Analytix Computer Use icon')
  for (const [key, expected] of [
    ['CFBundleName', 'Analytix Computer Use'],
    ['CFBundleDisplayName', 'Analytix Computer Use'],
    ['CFBundleIdentifier', 'com.analytix.computer-use'],
    ['CFBundleIconFile', 'AnalytixComputerUse.icns']
  ]) {
    const actual = String(exec('plutil', ['-extract', key, 'raw', infoPlist], {
      encoding: 'utf8'
    })).trim()
    if (actual !== expected) {
      throw new Error(`[after-pack] Bundled Analytix Computer Use ${key} expected ${expected}, got ${actual}`)
    }
  }
  chmodSync(executable, 0o755)
  console.log('[after-pack] Validated bundled Analytix Computer Use helper identity.')
}

async function afterPack(context) {
  const repoRoot = realpathSync(join(__dirname, '..'))
  const { lifecycle, entrySnapshot, buildContext } = consumePackagedAfterExtractSnapshot(
    context,
    repoRoot
  )
  const runtimeDir = join(packedResourcesDir(context), 'runtime')
  const controlledNativeReceiptPresent = pathEntryExists(join(runtimeDir, RECEIPT_FILE_NAME))
  let nativeTrust
  if (isCoreContext(context)) {
    if (packagedTargetKey(context) !== 'darwin-arm64' || process.env.ANALYTIX_OFFICE_PRIVATE_LOCAL_BUILD) throw Error('core_profile_candidate_scope_invalid')
    assertCoreResourcesAbsent(packedResourcesDir(context))
    mkdirSync(runtimeDir, { recursive: true, mode: 0o700 })
    nativeTrust = formalPackagedReleaseIntent(process.env)
      ? { kind: CORE_CONTROLLED_DISPOSITION, targetKey: packagedTargetKey(context),
          signingPolicySha256: macSigningPolicyDigest, signingMode: 'developer-id',
          appleTeamIdentifier: require('./macos-signing-policy.cjs').requireOfficialTeamIdentifier() }
      : { kind: CORE_DISPOSITION, targetKey: packagedTargetKey(context) }
  } else if (controlledNativeReceiptPresent) {
    nativeTrust = verifyPackagedNativeBeforeRuntimeBuild(context)
  } else {
    assertNativePackageQuarantined(context, { allowUnavailable: true })
    if (formalPackagedReleaseIntent(process.env)) {
      throw new Error('[after-pack] native_component_release_authority_unavailable')
    }
  }
  if (!controlledNativeReceiptPresent && !isCoreContext(context)) {
    nativeTrust = materializePackagedDevelopmentNativeComponents(context)
  }
  const nativeDisposition = normalizeNativeDisposition(nativeTrust)
  if (!isCoreContext(context)) materializePackagedDocumentRuntime(context, {
    required: formalPackagedReleaseIntent(process.env)
  })
  const runtimeServer = buildBundledGoRuntimeServer(context, nativeDisposition)
  await signBundledWindowsRuntime(context, runtimeServer.output)
  prunePackedAnalytixDependencies(context)
  ensureBundledOpenComputerUsePackage(context)
  ensureBundledOpenComputerUseWindowsShims(context)
  prunePackedProductionArtifacts(context)
  const fundsPluginAdmission = validateBundledFundsPlugin(context)
  validateBundledAnalytixRuntime(context, { nativeDisposition })
  validateBundledOpenComputerUseMacApp(context)
  ensureNodePtyHelpersExecutable(context)
  relocatePackagedDarwinNodePty(context)
  await applyElectronFusePolicyV1(context)
  const worktreeSnapshotAfter = collectPackagedWorktreeSnapshotV1(repoRoot)
  if (canonicalJSON(worktreeSnapshotAfter) !== lifecycle.snapshotCanonical ||
    canonicalJSON(worktreeSnapshotAfter) !== canonicalJSON(entrySnapshot)) {
    throw new Error('[after-pack] Source worktree changed during afterPack')
  }
  stagePrivateOfficeForPack(context, nativeDisposition, repoRoot, worktreeSnapshotAfter)
  const { validateOfficeCodecDirectory } = await import('./build-office-codec.mjs')
  await validateOfficeCodecDirectory(join(unpackedAppRoot(context), 'out', 'office-codec'))
  // Supply legal inputs before the resource closure and signatures are created.
  const legal = require('./lib/dependency-legal-evidence.cjs')
  const legalTarget = { platform: context.electronPlatformName, arch: normalizeArch(context.arch) }
  const legalInputs = legal.materializeLegalEvidence(repoRoot, packedResourcesDir(context), legalTarget)
  const { createArtifactReaderFromPath } = await import('./artifact-legal-obligations-audit.mjs')
  const legalReader = createArtifactReaderFromPath(
    context.electronPlatformName === 'darwin' ? appBundlePath(context) : context.appOutDir
  )
  legal.verifyPackagedLegalMaterials(legalReader, legalInputs, { ...legalTarget, allowSigned: false })
  legalReader.assertStable?.()
  if (canonicalJSON(collectPackagedWorktreeSnapshotV1(repoRoot)) !== canonicalJSON(worktreeSnapshotAfter)) {
    throw new Error('[after-pack] Source inputs changed while materializing legal evidence')
  }
  const authority = writePackagedBuildAuthorityV2(context, nativeDisposition, {
    repoRoot,
    worktreeSnapshot: worktreeSnapshotAfter,
    buildContext,
    fundsPluginAdmission
  })
  verifyPackagedBuildAuthorityArtifacts(context, authority)
  recordCompletedAfterPack(context, authority.authorityDigest)
}

exports.ANALYTIX_RUNTIME_REQUIRED_PATHS = ANALYTIX_RUNTIME_REQUIRED_PATHS
exports.ANALYTIX_RUNTIME_FORBIDDEN_PATHS = ANALYTIX_RUNTIME_FORBIDDEN_PATHS
exports.ANALYTIX_RUNTIME_GO_REQUIRED_PATHS = ANALYTIX_RUNTIME_GO_REQUIRED_PATHS
exports.ANALYTIX_DATA_NATIVE_REQUIRED_TOOLS = ANALYTIX_DATA_NATIVE_REQUIRED_TOOLS
exports.ANALYTIX_NATIVE_GENERATION_METADATA = ANALYTIX_NATIVE_GENERATION_METADATA
exports.ANALYTIX_MANAGED_CHROME_REQUIRED_PATHS = ANALYTIX_MANAGED_CHROME_REQUIRED_PATHS
exports.ANALYTIX_RUNTIME_GO_FORBIDDEN_PATHS = ANALYTIX_RUNTIME_GO_FORBIDDEN_PATHS
exports.ANALYTIX_RUNTIME_GO_FORBIDDEN_PATTERNS = ANALYTIX_RUNTIME_GO_FORBIDDEN_PATTERNS
exports.ANALYTIX_FUNDS_PRODUCTION_MCP_FILES = ANALYTIX_FUNDS_PRODUCTION_MCP_FILES
exports.PACKAGED_BUILD_AUTHORITY_CONTRACT = PACKAGED_BUILD_AUTHORITY_CONTRACT
exports.PACKAGED_BUILD_AUTHORITY_FILE = PACKAGED_BUILD_AUTHORITY_FILE
exports.PACKAGED_WORKTREE_SNAPSHOT_CONTRACT = PACKAGED_WORKTREE_SNAPSHOT_CONTRACT
exports.EFFECTIVE_BUILDER_CONTEXT_CONTRACT = EFFECTIVE_BUILDER_CONTEXT_CONTRACT
exports.STAGED_PAYLOAD_CONTRACT = STAGED_PAYLOAD_CONTRACT
exports.NATIVE_DEVELOPMENT_BUILD_MARKER = NATIVE_DEVELOPMENT_BUILD_MARKER
exports.NATIVE_DISPOSITION_CONTROLLED_RELEASE = NATIVE_DISPOSITION_CONTROLLED_RELEASE
exports.NATIVE_DISPOSITION_DEVELOPMENT = NATIVE_DISPOSITION_DEVELOPMENT
exports.DOCUMENT_RUNTIME_MANIFEST_FILE = DOCUMENT_RUNTIME_MANIFEST_FILE
exports.DOCUMENT_RUNTIME_UNAVAILABLE = DOCUMENT_RUNTIME_UNAVAILABLE
exports._internals = {
  privateOfficeBuildRequested,
  stagePrivateOfficeForPack,
  verifyPrivateOfficeForPack,
  appBundlePath,
  packedResourcesDir,
  unpackedAppRoot,
  bundledGoRuntimeServerPath,
  bundledDataAnalysisNativeToolPath,
  bundledDocumentRuntimeRoot,
  materializePackagedDocumentRuntime,
  verifyPackagedDocumentRuntime,
  packagedExecutablePath,
  packagedAppAsarPath,
  packagedTargetKey,
  collectPackagedWorktreeSnapshotV1,
  collectEffectiveBuilderContextV1,
  isEffectiveBuilderContextV1,
  collectStagedPayloadClosureV1,
  isStagedPayloadClosureV1,
  consumePackagedAfterExtractSnapshot,
  applyElectronFusePolicyV1,
  canonicalElectronFrameworkFuseTargetV1,
  verifyElectronV8SnapshotInventoryV1,
  verifyElectronFusePolicyV1,
  isPackagedWorktreeSnapshotV1,
  packagedWorktreeSnapshotDigest,
  isExcludedPackagedWorktreePath,
  isPackagedWorktreeSourcePath,
  createPackagedBuildAuthorityV2,
  isPackagedBuildAuthorityV2,
  packagedBuildAuthorityDigest,
  writePackagedBuildAuthorityV2,
  readPackagedBuildAuthorityV2,
  verifyPackagedBuildAuthorityArtifacts,
  isNativeDisposition,
  normalizeNativeDisposition,
  signingInvariantNativeArtifactBinding,
  validateDevelopmentBuildMarker,
  readDevelopmentBuildMarker,
  developmentNativeDisposition,
  materializePackagedDevelopmentNativeComponents,
  verifyPackagedDevelopmentNativeDisposition,
  assertPrivateNativeCacheSource,
  formalPackagedReleaseIntent,
  assertNativePackageQuarantined,
  bundledWindowsBackendPythonExecutablePath,
  bundledWindowsBackendSitePackagesDir,
  bundledManagedChromePath,
  dataAnalysisNativeBinaryName,
  bundledOpenComputerUsePackageRoot,
  bundledOpenComputerUseNativePath,
  bundledOpenComputerUseMacAppPath,
  nodePtyRequiredPaths,
  buildBundledGoRuntimeServer,
  verifyPackagedNativeBeforeRuntimeBuild,
  darwinSigningTrust,
  signBundledWindowsRuntime,
  validateBundledOpenComputerUseMacApp,
  validateBundledFundsPlugin,
  inspectFundsPluginSourceProjectionsV1,
  validateFundsPluginDeclarationV1Bytes,
  isFundsPluginAdmissionV1,
  collectPackagedFundsPluginIdentityV2,
  isFundsPluginArtifactBindingV2,
  validateFundsPluginMetadata,
  validateFundsPluginProjectionParity,
  goOSForPlatform,
  goArchForTarget,
  npmCommand,
  collectFiles,
  prunePackedAnalytixDependencies,
  pruneRuntimeNodeModuleSelfLinks,
  prunePackedProductionArtifacts,
  pruneOpenComputerUsePackageForTarget,
  pruneNapiRsCanvasForTarget,
  pruneConditionalNativeDependenciesForTarget,
  findLocalOpenComputerUsePackageRoot,
  ensureBundledOpenComputerUsePackage,
  ensureBundledOpenComputerUseWindowsShims,
  validateBundledAnalytixRuntime,
  ensureNodePtyHelpersExecutable,
  packagedDarwinNodePtyRoot,
  prunePackagedDarwinNodePtyRuntime,
  validatePackagedDarwinNodePty,
  relocatePackagedDarwinNodePty,
  sameFileIdentity,
  sameRelocatedDirectoryIdentity
}
exports.default = afterPack
