const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const nativeComponentContract = require('./native-component-contract.cjs')
const pythonRuntimeContract = require('./windows-python-runtime-contract.cjs')
const windowsPayloadContract = require('./windows-payload-contract.cjs')

let asar = null
for (const candidate of [
  '@electron/asar',
  'asar',
  'app-builder-lib/node_modules/@electron/asar'
]) {
  try {
    asar = require(candidate)
    break
  } catch {
    asar = null
  }
}

function readAsarHeader(archivePath) {
  const fd = fs.openSync(archivePath, 'r')
  try {
    const sizePickle = Buffer.alloc(8)
    fs.readSync(fd, sizePickle, 0, sizePickle.length, 0)
    const headerPickleSize = sizePickle.readUInt32LE(4)
    const headerPickle = Buffer.alloc(headerPickleSize)
    fs.readSync(fd, headerPickle, 0, headerPickle.length, 8)
    const headerStringSize = headerPickle.readUInt32LE(4)
    const headerText = headerPickle.slice(8, 8 + headerStringSize).toString('utf8')
    return JSON.parse(headerText)
  } finally {
    fs.closeSync(fd)
  }
}

function listAsarPackage(archivePath) {
  if (asar) {
    return asar.listPackage(archivePath).map((item) => item.replace(/^\//, ''))
  }

  const header = readAsarHeader(archivePath)
  const list = []

  function visit(node, prefix) {
    if (!node || typeof node !== 'object') return
    if (node.files && typeof node.files === 'object') {
      for (const [name, child] of Object.entries(node.files)) {
        const childPath = prefix ? `${prefix}/${name}` : name
        visit(child, childPath)
      }
      return
    }
    if (prefix) list.push(prefix)
  }

  visit(header, '')
  return list
}

const repo = process.cwd()
const version = process.argv[2] || process.env.ANALYTIX_APP_VERSION || require('../package.json').version
const unpacked = path.join(repo, 'dist-standard-win', 'win-unpacked')
const resources = path.join(unpacked, 'resources')
const pythonRuntimeManifest = path.join(resources, '.python-runtime/current/analytix-python-runtime-manifest.json')
const archiveExtractorManifest = path.join(resources, 'runtime/analytix-archive-extractor-manifest.json')
const installer = path.join(repo, 'dist-standard-win', `analytix-standard-${version}-x64.exe`)
const blockmap = `${installer}.blockmap`
const latest = path.join(repo, 'dist-standard-win', 'latest.yml')

function envFlag(name) {
  return /^(1|true|yes|on)$/i.test(String(process.env[name] || '').trim())
}

const allowUnsignedWindowsQA = envFlag('ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA')
const requireWindowsAuthenticode = process.platform === 'win32' && !allowUnsignedWindowsQA
const expectedWindowsSignerSha1 = String(process.env.ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1 || '').trim().toUpperCase()

function exists(filePath) {
  return fs.existsSync(filePath)
}

function sizeFile(filePath) {
  return exists(filePath) ? fs.statSync(filePath).size : 0
}

function sha256(filePath) {
  if (!exists(filePath)) return ''
  return crypto.createHash('sha256').update(fs.readFileSync(filePath)).digest('hex').toUpperCase()
}

function fileContainsAscii(filePath, needle) {
  if (!exists(filePath)) return false
  return fs.readFileSync(filePath).includes(Buffer.from(needle, 'ascii'))
}

function readJsonIfPresent(filePath) {
  if (!exists(filePath)) return null
  return JSON.parse(fs.readFileSync(filePath, 'utf8'))
}

function walk(root, acc = []) {
  if (!exists(root)) return acc
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const fullPath = path.join(root, entry.name)
    if (entry.isSymbolicLink()) continue
    if (entry.isDirectory()) {
      walk(fullPath, acc)
    } else if (entry.isFile()) {
      acc.push(fullPath)
    }
  }
  return acc
}

function collectSymlinks(root, acc = []) {
  if (!exists(root)) return acc
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const fullPath = path.join(root, entry.name)
    if (entry.isSymbolicLink()) {
      let target = ''
      try {
        target = fs.readlinkSync(fullPath)
      } catch {
        target = '<unreadable>'
      }
      acc.push({ path: fullPath, target })
      continue
    }
    if (entry.isDirectory()) {
      collectSymlinks(fullPath, acc)
    }
  }
  return acc
}

function dirSize(root) {
  return walk(root).reduce((sum, filePath) => sum + sizeFile(filePath), 0)
}

function toPackagePath(filePath) {
  return path.relative(unpacked, filePath).split(path.sep).join('/')
}

function readTextIfPresent(filePath) {
  return exists(filePath) ? fs.readFileSync(filePath, 'utf8') : ''
}

function releaseRootAudit() {
  const allowedNames = new Set([
    'win-unpacked',
    'builder-debug.yml',
    'builder-effective-config.yaml',
    'latest.yml',
    `analytix-standard-${version}-x64.exe`,
    `analytix-standard-${version}-x64.exe.blockmap`
  ])
  const entries = exists(path.dirname(installer))
    ? fs.readdirSync(path.dirname(installer), { withFileTypes: true })
    : []
  const names = entries.map((entry) => entry.name)
  const forbidden = names.filter((name) => {
    if (/\.nsis-payload\.exe$/i.test(name)) return true
    const standard = name.match(/^analytix-standard-(.+)-x64\.exe(?:\.blockmap)?$/i)
    if (standard && standard[1] !== version) return true
    if (/Analytix_new/i.test(name)) return true
    return false
  })
  return {
    entries: names.sort(),
    forbidden,
    unexpectedPublishable: names.filter((name) => {
      if (allowedNames.has(name)) return false
      if (/^analytix-standard-.+-x64\.exe(?:\.blockmap)?$/i.test(name)) return false
      return /\.(exe|blockmap|yml|yaml)$/i.test(name)
    }).sort()
  }
}

function trustedWindowsPowerShell() {
  const systemRoot = String(process.env.SystemRoot || '').trim()
  if (!path.isAbsolute(systemRoot)) throw new Error('SystemRoot is unavailable')
  const canonicalRoot = path.join(path.parse(systemRoot).root, 'Windows')
  if (path.resolve(systemRoot).toLowerCase() !== path.resolve(canonicalRoot).toLowerCase()) {
    throw new Error('SystemRoot is not the canonical Windows OS directory')
  }
  const powershell = path.join(canonicalRoot, 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe')
  const stat = fs.lstatSync(powershell)
  if (!stat.isFile() || stat.isSymbolicLink()) throw new Error('system PowerShell is not a regular file')
  return powershell
}

function verifyAuthenticodeSignature(filePath) {
  if (!exists(filePath)) {
    return { filePath, status: 'missing', ok: false }
  }
  if (process.platform !== 'win32') {
    return { filePath, status: 'skipped', ok: true, reason: 'non-windows audit host' }
  }
  const script = [
    '$ErrorActionPreference = "Stop";',
    '$path = [Console]::In.ReadToEnd();',
    '$signature = Get-AuthenticodeSignature -LiteralPath $path;',
    '$signer = "";',
    '$thumbprint = "";',
    '$timestampSigner = "";',
    '$timestampThumbprint = "";',
    'if ($signature.SignerCertificate) {',
    '  $signer = [string]$signature.SignerCertificate.Subject;',
    '  $thumbprint = [string]$signature.SignerCertificate.Thumbprint;',
    '}',
    'if ($signature.TimeStamperCertificate) {',
    '  $timestampSigner = [string]$signature.TimeStamperCertificate.Subject;',
    '  $timestampThumbprint = [string]$signature.TimeStamperCertificate.Thumbprint;',
    '}',
    '$result = [pscustomobject]@{',
    '  status = [string]$signature.Status;',
    '  statusMessage = [string]$signature.StatusMessage;',
    '  signer = $signer;',
    '  thumbprint = $thumbprint;',
    '  timestampSigner = $timestampSigner;',
    '  timestampThumbprint = $timestampThumbprint;',
    '};',
    '$result | ConvertTo-Json -Compress'
  ].join('\n')
  let powershell
  try {
    powershell = trustedWindowsPowerShell()
  } catch (error) {
    return {
      filePath,
      status: 'error',
      ok: false,
      error: `trusted Windows signature verifier is unavailable: ${error instanceof Error ? error.message : String(error)}`
    }
  }
  const child = require('node:child_process').spawnSync(powershell, [
    '-NoLogo',
    '-NoProfile',
    '-NonInteractive',
    '-ExecutionPolicy',
    'Bypass',
    '-Command',
    script
  ], {
    input: filePath,
    encoding: 'utf8',
    timeout: 15_000,
    windowsHide: true
  })
  if (child.status !== 0) {
    return {
      filePath,
      status: 'error',
      ok: false,
      error: String(child.stderr || child.stdout || '').trim()
    }
  }
  try {
    const parsed = JSON.parse(String(child.stdout || '{}'))
    return {
      filePath,
      ...parsed,
      ok: parsed.status === 'Valid'
    }
  } catch (error) {
    return {
      filePath,
      status: 'error',
      ok: false,
      error: error instanceof Error ? error.message : String(error)
    }
  }
}

function trustedExecutableSignatureFailure(signature) {
  if (!requireWindowsAuthenticode) return 'official Authenticode policy is not active'
  if (!/^[0-9A-F]{40}$/.test(expectedWindowsSignerSha1)) return 'approved signer thumbprint is unavailable'
  if (!signature.ok || signature.status !== 'Valid') return `signature status is ${signature.status || 'unknown'}`
  if (!signature.thumbprint || String(signature.thumbprint).toUpperCase() !== expectedWindowsSignerSha1) {
    return 'signer thumbprint is not approved'
  }
  if (!signature.timestampThumbprint) return 'RFC3161 timestamp is missing'
  return ''
}

const installerSignaturePreflight = verifyAuthenticodeSignature(installer)

function auditEmbeddedBootstrapperPayload() {
  if (process.platform !== 'win32') return { ok: true, status: 'skipped-non-windows' }
  if (!requireWindowsAuthenticode) {
    return { ok: true, status: 'skipped-unsigned-local-qa' }
  }
  const installerSignatureFailure = trustedExecutableSignatureFailure(installerSignaturePreflight)
  if (installerSignatureFailure) {
    return { ok: false, status: 'blocked-before-resource-extraction', error: installerSignatureFailure }
  }
  const temporaryRoot = fs.mkdtempSync(path.join(require('node:os').tmpdir(), 'analytix-bootstrapper-audit-'))
  try {
    const resourceDir = path.join(temporaryRoot, 'resources')
    const extractedDir = path.join(temporaryRoot, 'payload')
    fs.mkdirSync(resourceDir, { recursive: true })
    fs.mkdirSync(extractedDir, { recursive: true })
    const extraction = require('node:child_process').spawnSync(trustedWindowsPowerShell(), [
      '-NoLogo',
      '-NoProfile',
      '-NonInteractive',
      '-ExecutionPolicy',
      'Bypass',
      '-File',
      path.join(repo, 'scripts', 'extract-windows-bootstrapper-resources.ps1'),
      '-Installer',
      installer,
      '-OutputDirectory',
      resourceDir
    ], { encoding: 'utf8', timeout: 120_000, windowsHide: true })
    if (extraction.error || extraction.status !== 0 || extraction.signal) {
      throw new Error(String(extraction.stderr || extraction.stdout || extraction.error?.message || '').trim())
    }
    const archive = path.join(resourceDir, 'AnalytixPayload.7z')
    const extractor = path.join(resourceDir, '7za.exe')
    const manifestPath = path.join(resourceDir, 'AnalytixPayloadManifest.json')
    const extractorSignature = verifyAuthenticodeSignature(extractor)
    const extractorSignatureFailure = trustedExecutableSignatureFailure(extractorSignature)
    if (extractorSignatureFailure) {
      throw new Error(`embedded extractor is not trusted: ${extractorSignatureFailure}`)
    }
    const manifest = windowsPayloadContract.verifyPayloadManifest({
      manifest: manifestPath,
      archive,
      extractor
    })
    for (const args of [
      ['t', '-bd', '-bb0', archive],
      ['x', '-y', '-bd', '-bb0', archive, `-o${extractedDir}`]
    ]) {
      const result = require('node:child_process').spawnSync(extractor, args, {
        encoding: 'utf8',
        timeout: 15 * 60_000,
        windowsHide: true,
        maxBuffer: 64 * 1024 * 1024
      })
      if (result.error || result.status !== 0 || result.signal) {
        throw new Error(String(result.stderr || result.stdout || result.error?.message || '').trim())
      }
    }
    windowsPayloadContract.verifyPayloadManifest({ manifest: manifestPath, root: extractedDir })
    return {
      ok: true,
      status: 'verified',
      treeSha256: manifest.treeSha256,
      fileCount: manifest.fileCount,
      archiveSha256: manifest.archive.sha256,
      extractorSha256: manifest.extractor.sha256,
      extractorSignature
    }
  } catch (error) {
    return { ok: false, status: 'failed', error: error instanceof Error ? error.message : String(error) }
  } finally {
    fs.rmSync(temporaryRoot, { recursive: true, force: true })
  }
}

const files = walk(unpacked)
const symlinks = collectSymlinks(unpacked).map((item) => ({
  path: toPackagePath(item.path),
  target: item.target
}))
const top50 = files
  .map((filePath) => ({ path: toPackagePath(filePath), size: sizeFile(filePath) }))
  .sort((left, right) => right.size - left.size)
  .slice(0, 50)

const forbiddenChecks = {
  git: (item) => /(^|\/)\.git(\/|$)/.test(item),
  dsStore: (item) => /(^|\/)\.DS_Store$/.test(item),
  appleDouble: (item) => /(^|\/)\._/.test(item),
  pdb: (item) => /\.pdb$/i.test(item),
  sourceMap: (item) => /\.map$/i.test(item),
  tsSource: (item) => /\.ts$/i.test(item) && !/\.d\.ts$/i.test(item),
  dts: (item) => /\.d\.ts$/i.test(item),
  tsconfig: (item) => /(^|\/)tsconfig[^/]*\.json$/i.test(item),
  readmeChangelog: (item) => /(^|\/)(README|CHANGELOG)(\.[^/]*)?$/i.test(item),
  backendVenv: (item) => /^resources\/backend\/\.venv\//.test(item),
  pycache: (item) => /(^|\/)__pycache__(\/|$)/.test(item),
  pyc: (item) => /\.pyc$/i.test(item),
  pytestCache: (item) => /(^|\/)\.pytest_cache(\/|$)/.test(item),
  testFixtures: (item) => /(^|\/)(tests?|__tests__|fixtures|__fixtures__|testsupport|conformance|upstreamaudit|readiness)(\/|$)/i.test(item),
  pluginNodeModules: (item) => /^resources\/plugins\/[^/]+\/node_modules\//.test(item),
  pluginOutputEvidence: (item) => /^resources\/plugins\/[^/]+\/(output|evidence|legacy-diagnostics|eval-fixtures)\//.test(item),
  nodeModulesExamplesDocs: (item) =>
    /^resources\/app\.asar\.unpacked\/(?:packages\/runtime\/)?node_modules\/.*\/(?:examples?|demos?|docs?|documentation|benchmarks?|coverage)\//i.test(item),
  staleDataNativeNoExt: (item) => /^resources\/runtime\/analytix-(?:import-accelerator|cleaning-ops|analysis-compute|data-engine)$/i.test(item),
  privacyProjectionNative: (item) =>
    /^resources\/runtime\/analytix-privacy-projection(?:\.exe)?$/i.test(item) ||
    /(^|\/)tools\/privacy_projection(\/|$)/i.test(item),
  nonWin32X64Native: (item) =>
    /^resources\/app\.asar\.unpacked\/node_modules\/node-pty\/prebuilds\/(?!win32-x64\/)/i.test(item) ||
    /^resources\/app\.asar\.unpacked\/node_modules\/@napi-rs\/canvas-(?!win32-x64-msvc\/)/i.test(item) ||
    /^resources\/app\.asar\.unpacked\/packages\/runtime\/node_modules\/analytix-computer-use\/dist\/(?:linux|darwin|windows\/arm64)\//i.test(item),
  oldTypeScriptRuntimeSource: (item) => /^resources\/app\.asar\.unpacked\/packages\/runtime\/src\/(?:server|loop|adapters)\//.test(item),
  oldAnalytixNew: (item) => /Analytix_new/i.test(item)
}

const packagePaths = files.map(toPackagePath)
const forbidden = {}
for (const [name, check] of Object.entries(forbiddenChecks)) {
  const matches = packagePaths.filter(check)
  forbidden[name] = { count: matches.length, sample: matches.slice(0, 10) }
}

const required = [
  'analytix.exe',
  'resources/app.asar',
  'resources/app-update.yml',
  'resources/backend/app/main.py',
  ...nativeComponentContract.manifest.components.map(
    (component) => `resources/${component.packagePath}.exe`
  ),
  `resources/runtime/${nativeComponentContract.RECEIPT_FILE_NAME}`,
  'resources/runtime/7za.exe',
  'resources/runtime/analytix-archive-extractor-manifest.json',
  'resources/.python-runtime/current/python/python.exe',
  'resources/.python-runtime/current/analytix-python-runtime-manifest.json',
  'resources/python-site-packages/analytix-site-packages-manifest.json',
  'resources/python-site-packages/fastapi/__init__.py',
  'resources/python-site-packages/uvicorn/__init__.py',
  'resources/python-site-packages/pandas/__init__.py',
  'resources/python-site-packages/duckdb/__init__.py',
  'resources/runtime-go/bin/runtime-server.exe',
  'resources/managed-chrome/codex-extension/manifest.json',
  'resources/managed-chrome/codex-extension/background.js',
  'resources/managed-chrome/chrome/scripts/browser-client.mjs',
  'resources/managed-chrome/chrome/extension-host/windows/x64/extension-host.exe',
  'resources/app.asar.unpacked/packages/runtime/dist/cli/serve-entry.js',
  'resources/app.asar.unpacked/packages/runtime/package.json',
  'resources/app.asar.unpacked/packages/runtime/package-lock.json',
  'resources/app.asar.unpacked/packages/runtime/node_modules/.bin/analytix-computer-use.cmd',
  'resources/app.asar.unpacked/packages/runtime/node_modules/.bin/analytix-computer-use-mcp.cmd',
  'resources/app.asar.unpacked/packages/runtime/node_modules/.bin/open-computer-use.cmd',
  'resources/app.asar.unpacked/packages/runtime/node_modules/.bin/open-computer-use-mcp.cmd',
  'resources/app.asar.unpacked/packages/runtime/node_modules/analytix-computer-use/package.json',
  'resources/app.asar.unpacked/packages/runtime/node_modules/analytix-computer-use/dist/windows/amd64/analytix-computer-use.exe',
  'resources/app.asar.unpacked/node_modules/node-pty/package.json',
  'resources/app.asar.unpacked/node_modules/node-pty/prebuilds/win32-x64/pty.node',
  'resources/app.asar.unpacked/node_modules/node-pty/prebuilds/win32-x64/conpty.node',
  'resources/app.asar.unpacked/node_modules/node-pty/prebuilds/win32-x64/conpty/conpty.dll',
  'resources/app.asar.unpacked/node_modules/node-pty/prebuilds/win32-x64/winpty.dll',
  'resources/app.asar.unpacked/node_modules/node-pty/prebuilds/win32-x64/winpty-agent.exe',
  'resources/app.asar.unpacked/node_modules/better-sqlite3/package.json',
  'resources/app.asar.unpacked/node_modules/better-sqlite3/build/Release/better_sqlite3.node',
  'resources/plugins/analytix-fund-analysis/.mcp.json',
  'resources/plugins/analytix-fund-analysis/.codex-plugin/plugin.json'
]
const requiredStatus = Object.fromEntries(required.map((item) => [item, exists(path.join(unpacked, item))]))
const releaseRoot = releaseRootAudit()
const bootstrapperWorkDir = path.join(repo, 'build', 'winforms-bootstrapper')
const bootstrapperPayload = {
  archive: path.join(bootstrapperWorkDir, 'AnalytixPayload.7z'),
  archiveSize: sizeFile(path.join(bootstrapperWorkDir, 'AnalytixPayload.7z')),
  archiveSha256: sha256(path.join(bootstrapperWorkDir, 'AnalytixPayload.7z')),
  legacyZip: path.join(bootstrapperWorkDir, 'AnalytixPayloadZip.zip'),
  legacyZipPresent: exists(path.join(bootstrapperWorkDir, 'AnalytixPayloadZip.zip')),
  extractor: path.join(bootstrapperWorkDir, '7za.exe'),
  extractorSize: sizeFile(path.join(bootstrapperWorkDir, '7za.exe')),
  extractorSha256: sha256(path.join(bootstrapperWorkDir, '7za.exe')),
  installerContainsPayloadArchiveMarker: fileContainsAscii(installer, 'AnalytixPayloadArchive'),
  installerContainsPayloadExtractorMarker: fileContainsAscii(installer, 'PayloadExtractorExe'),
  installerContainsPayloadManifestMarker: fileContainsAscii(installer, 'AnalytixPayloadManifest'),
  installerContainsNullsoftMarker: fileContainsAscii(installer, 'NullsoftInst')
}
bootstrapperPayload.embedded = auditEmbeddedBootstrapperPayload()
const privatePwshRoot = path.join(resources, 'pwsh')
const privatePwsh = {
  bundled: exists(privatePwshRoot),
  executable: path.join(privatePwshRoot, 'pwsh.exe'),
  executablePresent: exists(path.join(privatePwshRoot, 'pwsh.exe')),
  manifest: readJsonIfPresent(path.join(privatePwshRoot, 'analytix-private-pwsh-manifest.json')),
  size: dirSize(privatePwshRoot)
}
const backendManagerSource = readTextIfPresent(path.join(repo, 'src/main/data-analysis/backend-manager.ts'))
const nativeRuntimePathsSource = readTextIfPresent(path.join(repo, 'src/main/data-analysis/native-runtime-paths.ts'))
const dataEngineOwnerGate = {
  dataEnginePresent: exists(path.join(resources, 'runtime/analytix-data-engine.exe')),
  analysisComputePresent: exists(path.join(resources, 'runtime/analytix-analysis-compute.exe')),
  cleaningOpsPresent: exists(path.join(resources, 'runtime/analytix-cleaning-ops.exe')),
  electronBackendQuarantined:
    backendManagerSource.includes('data_analysis_native_authority_unavailable') &&
    !backendManagerSource.includes('node:child_process') &&
    !backendManagerSource.includes('ANALYTIX_DATA_ANALYSIS_PYTHON') &&
    !backendManagerSource.includes('ANALYTIX_DATA_ANALYSIS_BACKEND_DIR') &&
    !backendManagerSource.includes('spawn(') &&
    !backendManagerSource.includes('execFile('),
  packagedEnvironmentDropsExternalPython: [
    'ANALYTIX_DATA_ANALYSIS_PYTHON',
    'CONDA_PREFIX',
    'PYTHON',
    'PYTHONHOME',
    'PYTHONPATH',
    'PYTHONUSERBASE',
    'VIRTUAL_ENV'
  ].every((key) => nativeRuntimePathsSource.includes(`'${key}'`)),
  packagedPythonIsDormant: !backendManagerSource.includes('verifyPackagedPythonBundle'),
  activeDbPolicy: 'Electron Python/Uvicorn is fail-closed; product active case.duckdb access may resume only through the Go native authority'
}
const signatureTargets = [
  installer,
  path.join(unpacked, 'analytix.exe'),
  path.join(resources, 'runtime-go/bin/runtime-server.exe'),
  path.join(resources, '.python-runtime/current/python/python.exe'),
  ...nativeComponentContract.manifest.components.map(
    (component) => path.join(resources, `${component.packagePath}.exe`)
  ),
  path.join(resources, 'runtime/7za.exe'),
  path.join(resources, 'app.asar.unpacked/packages/runtime/node_modules/analytix-computer-use/dist/windows/amd64/analytix-computer-use.exe')
].concat(privatePwsh.bundled ? [privatePwsh.executable] : [])
const authenticode = signatureTargets.map((filePath) => (
  filePath === installer ? installerSignaturePreflight : verifyAuthenticodeSignature(filePath)
))
let dataNativeContract = { ok: false, error: '', receipt: null }
try {
  dataNativeContract = {
    ok: true,
    error: '',
    receipt: nativeComponentContract.verifyPackagedComponents(
      path.join(resources, 'runtime'),
      nativeComponentContract.targetContract('win32', 'x64')
    )
  }
} catch (error) {
  dataNativeContract = {
    ok: false,
    error: error instanceof Error ? error.message : String(error),
    receipt: null
  }
}

let pythonBundleContract = { ok: false, error: '', runtime: null, sitePackages: null }
try {
  pythonBundleContract = {
    ok: true,
    error: '',
    runtime: pythonRuntimeContract.verifyPythonRuntime(path.join(resources, '.python-runtime', 'current')),
    sitePackages: pythonRuntimeContract.verifySitePackages(path.join(resources, 'python-site-packages'))
  }
} catch (error) {
  pythonBundleContract = {
    ok: false,
    error: error instanceof Error ? error.message : String(error),
    runtime: null,
    sitePackages: null
  }
}

let appAsarScan = { available: Boolean(asar) }
try {
  const list = listAsarPackage(path.join(resources, 'app.asar'))
  const matches = (check) => list.filter(check)
  const count = (check) => matches(check).length
  appAsarScan = {
    available: true,
    parser: asar ? 'module' : 'built-in',
    fileCount: list.length,
    forbiddenCounts: {
      maps: count((item) => /\.map$/i.test(item)),
      ts: count((item) => /\.ts$/i.test(item) && !/\.d\.ts$/i.test(item)),
      dts: count((item) => /\.d\.ts$/i.test(item)),
      tsconfig: count((item) => /(^|\/)tsconfig[^/]*\.json$/i.test(item)),
      docs: count((item) => /(^|\/)(README|CHANGELOG)(\.[^/]*)?$/i.test(item)),
      tests: count((item) => /(^|\/)(tests?|fixtures|testsupport|conformance|upstreamaudit|readiness)(\/|$)/i.test(item)),
      examplesDocs: count((item) => /(^|\/)(examples?|demos?|docs?|documentation|benchmarks?|coverage)(\/|$)/i.test(item)),
      appleDouble: count((item) => /(^|\/)\._/.test(item)),
      pdb: count((item) => /\.pdb$/i.test(item)),
      dsStore: count((item) => /(^|\/)\.DS_Store$/.test(item))
    },
    samples: {
      maps: matches((item) => /\.map$/i.test(item)).slice(0, 20),
      ts: matches((item) => /\.ts$/i.test(item) && !/\.d\.ts$/i.test(item)).slice(0, 20),
      dts: matches((item) => /\.d\.ts$/i.test(item)).slice(0, 20),
      tsconfig: matches((item) => /(^|\/)tsconfig[^/]*\.json$/i.test(item)).slice(0, 20),
      docs: matches((item) => /(^|\/)(README|CHANGELOG)(\.[^/]*)?$/i.test(item)).slice(0, 20),
      tests: matches((item) => /(^|\/)(tests?|fixtures|testsupport|conformance|upstreamaudit|readiness)(\/|$)/i.test(item)).slice(0, 20),
      examplesDocs: matches((item) => /(^|\/)(examples?|demos?|docs?|documentation|benchmarks?|coverage)(\/|$)/i.test(item)).slice(0, 20),
      pdb: matches((item) => /\.pdb$/i.test(item)).slice(0, 20)
    }
  }
} catch (error) {
  appAsarScan = {
    available: false,
    error: error instanceof Error ? error.message : String(error)
  }
}

const output = {
  generatedAt: new Date().toISOString(),
  signaturePolicy: {
    platform: process.platform,
    authenticodeRequired: requireWindowsAuthenticode,
    qaOverrideEnv: 'ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA',
    expectedSignerSha1Configured: /^[0-9A-F]{40}$/.test(expectedWindowsSignerSha1),
    mode: process.platform !== 'win32' ? 'not-applicable' : requireWindowsAuthenticode ? 'required' : 'unsigned-local-qa',
    note: 'Official Windows artifacts require valid Authenticode by default. ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA=1 is limited to local QA and must never be uploaded or promoted.'
  },
  artifact: {
    installer,
    installerSize: sizeFile(installer),
    installerSha256: sha256(installer),
    blockmap,
    blockmapSize: sizeFile(blockmap),
    blockmapSha256: sha256(blockmap),
    latest,
    latestSize: sizeFile(latest),
    latestText: fs.readFileSync(latest, 'utf8')
  },
  sizes: {
    winUnpacked: dirSize(unpacked),
    appAsar: sizeFile(path.join(resources, 'app.asar')),
    appAsarUnpacked: dirSize(path.join(resources, 'app.asar.unpacked')),
    backend: dirSize(path.join(resources, 'backend')),
    plugins: dirSize(path.join(resources, 'plugins')),
    runtimeGoServer: sizeFile(path.join(resources, 'runtime-go/bin/runtime-server.exe')),
    runtimeNodeModules: dirSize(path.join(resources, 'app.asar.unpacked/packages/runtime/node_modules')),
    nodePty: dirSize(path.join(resources, 'app.asar.unpacked/node_modules/node-pty')),
    betterSqlite3: dirSize(path.join(resources, 'app.asar.unpacked/node_modules/better-sqlite3')),
    dataNativeRuntime: dirSize(path.join(resources, 'runtime')),
    archiveExtractor: sizeFile(path.join(resources, 'runtime/7za.exe')),
    pythonRuntime: dirSize(path.join(resources, '.python-runtime')),
    pythonSitePackages: dirSize(path.join(resources, 'python-site-packages')),
    managedChrome: dirSize(path.join(resources, 'managed-chrome')),
    privatePwsh: privatePwsh.size,
    computerUse: dirSize(path.join(resources, 'app.asar.unpacked/packages/runtime/node_modules/analytix-computer-use')),
    cleaningOps: sizeFile(path.join(resources, 'runtime/analytix-cleaning-ops.exe')),
    importAccelerator: sizeFile(path.join(resources, 'runtime/analytix-import-accelerator.exe')),
    analysisCompute: sizeFile(path.join(resources, 'runtime/analytix-analysis-compute.exe')),
    dataEngine: sizeFile(path.join(resources, 'runtime/analytix-data-engine.exe'))
  },
  pythonRuntime: readJsonIfPresent(pythonRuntimeManifest),
  archiveExtractor: readJsonIfPresent(archiveExtractorManifest),
  bootstrapperPayload,
  privatePwsh,
  dataEngineOwnerGate,
  dataNativeContract,
  pythonBundleContract,
  releaseRoot,
  authenticode,
  symlinks,
  requiredStatus,
  forbidden,
  appAsarScan,
  top50
}

const outDir = path.join(repo, 'validation-evidence', 'package-audit')
fs.mkdirSync(outDir, { recursive: true })
const outPath = path.join(outDir, 'package-audit.json')
fs.writeFileSync(outPath, `${JSON.stringify(output, null, 2)}\n`, 'utf8')
const requiredMissing = Object.entries(requiredStatus).filter(([, present]) => !present).map(([item]) => item)
const forbiddenNonZero = Object.fromEntries(Object.entries(forbidden).filter(([, value]) => value.count))
const appAsarForbiddenNonZero = appAsarScan.available
  ? Object.fromEntries(Object.entries(appAsarScan.forbiddenCounts || {}).filter(([, count]) => Number(count) > 0))
  : {}
const invalidSignatures = process.platform === 'win32'
  ? authenticode.filter((item) => !item.ok)
  : []
const validSignerThumbprints = process.platform === 'win32'
  ? new Set(authenticode.filter((item) => item.ok && item.thumbprint).map((item) => String(item.thumbprint).toUpperCase()))
  : new Set()
const signatureFailures = requireWindowsAuthenticode
  ? [
      ...(/^[0-9A-F]{40}$/.test(expectedWindowsSignerSha1) ? [] : [{ filePath: '<signature-policy>', status: 'expected-signer-missing' }]),
      ...invalidSignatures,
      ...authenticode.filter((item) => item.ok && (!item.thumbprint || !item.timestampThumbprint)),
      ...(validSignerThumbprints.size === 1 ? [] : [{ filePath: '<signature-set>', status: 'mixed-or-missing-signer' }]),
      ...(validSignerThumbprints.size === 1 && validSignerThumbprints.has(expectedWindowsSignerSha1)
        ? []
        : [{ filePath: '<signature-set>', status: 'unexpected-signer' }])
    ]
  : []
const privatePwshFailures = privatePwsh.bundled && (!privatePwsh.executablePresent || !privatePwsh.manifest)
  ? ['private pwsh bundle must include resources/pwsh/pwsh.exe and analytix-private-pwsh-manifest.json']
  : []
const bootstrapperFailures = [
  ...(!bootstrapperPayload.installerContainsPayloadArchiveMarker ? ['final installer must be the WinForms bootstrapper with embedded AnalytixPayloadArchive'] : []),
  ...(!bootstrapperPayload.installerContainsPayloadExtractorMarker ? ['final installer must embed PayloadExtractorExe for 7z payload extraction'] : []),
  ...(!bootstrapperPayload.installerContainsPayloadManifestMarker ? ['final installer must embed AnalytixPayloadManifest'] : []),
  ...(process.platform === 'win32' && !bootstrapperPayload.embedded.ok
    ? [`final installer embedded payload audit failed: ${bootstrapperPayload.embedded.error || bootstrapperPayload.embedded.status}`]
    : []),
  ...(bootstrapperPayload.installerContainsNullsoftMarker ? ['final installer is still a raw NSIS installer; expected WinForms 7z bootstrapper'] : []),
  ...(bootstrapperPayload.legacyZipPresent ? ['legacy bootstrapper zip payload must not remain after 7z migration'] : [])
]
const dataEngineOwnerGateFailures = [
  ...(!dataEngineOwnerGate.dataEnginePresent ? ['resources/runtime/analytix-data-engine.exe is required for single DuckDB owner'] : []),
  ...(!dataEngineOwnerGate.electronBackendQuarantined ? ['backend-manager.ts must be a fixed native-authority boundary with zero child-process or Python selector path'] : []),
  ...(!dataEngineOwnerGate.packagedEnvironmentDropsExternalPython ? ['packaged backend environment must remove external Python selectors and paths'] : []),
  ...(!dataEngineOwnerGate.packagedPythonIsDormant ? ['packaged Python must remain unreachable until the Go native authority owns its lifecycle'] : [])
]
const failures = [
  ...(!exists(unpacked) ? [`win-unpacked is missing: ${unpacked}`] : []),
  ...(requiredMissing.length ? [`required files missing: ${requiredMissing.join(', ')}`] : []),
  ...bootstrapperFailures,
  ...privatePwshFailures,
  ...dataEngineOwnerGateFailures,
  ...(!dataNativeContract.ok ? [`native data component contract failed: ${dataNativeContract.error}`] : []),
  ...(!pythonBundleContract.ok ? [`Python runtime contract failed: ${pythonBundleContract.error}`] : []),
  ...(releaseRoot.forbidden.length ? [`forbidden release root files found: ${releaseRoot.forbidden.join(', ')}`] : []),
  ...(symlinks.length ? [`packaged symlinks/reparse points found: ${symlinks.map((item) => `${item.path}->${item.target}`).join(', ')}`] : []),
  ...(Object.keys(forbiddenNonZero).length ? [`forbidden packaged files found: ${Object.keys(forbiddenNonZero).join(', ')}`] : []),
  ...(!appAsarScan.available ? [`app.asar scan failed: ${appAsarScan.error || 'unknown error'}`] : []),
  ...(Object.keys(appAsarForbiddenNonZero).length ? [`forbidden app.asar entries found: ${Object.keys(appAsarForbiddenNonZero).join(', ')}`] : []),
  ...(signatureFailures.length ? [`Authenticode signature check failed: ${signatureFailures.map((item) => `${item.filePath}=${item.status}`).join(', ')}`] : [])
]
console.log(outPath)
console.log(JSON.stringify({
  signaturePolicy: output.signaturePolicy,
  artifact: output.artifact,
  sizes: output.sizes,
  pythonRuntime: output.pythonRuntime,
  archiveExtractor: output.archiveExtractor,
  bootstrapperPayload: output.bootstrapperPayload,
  privatePwsh: output.privatePwsh,
  dataEngineOwnerGate: output.dataEngineOwnerGate,
  pythonBundleContract: output.pythonBundleContract,
  releaseRoot: output.releaseRoot,
  authenticode: output.authenticode,
  symlinks: output.symlinks,
  requiredMissing,
  forbiddenNonZero,
  invalidSignatures,
  appAsarScan
}, null, 2))
if (failures.length > 0) {
  console.error(`[package-audit] FAILED\n- ${failures.join('\n- ')}`)
  process.exitCode = 1
}
