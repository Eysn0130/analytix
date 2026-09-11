const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { spawnSync } = require('node:child_process')
const crypto = require('node:crypto')
const {
  REQUIREMENTS_LOCK_PATH,
  RUNTIME_MANIFEST_NAME,
  SITE_PACKAGES_MANIFEST_NAME,
  lock: pythonRuntimeLock,
  lockSha256: pythonRuntimeLockSha256,
  pruneProductionPythonTree,
  rawTreeIdentity,
  signingInvariantTreeIdentity,
  verifyPythonRuntime,
  verifySitePackages,
  verifyWheelhouse
} = require('./windows-python-runtime-contract.cjs')

const REPO_ROOT = path.resolve(__dirname, '..')
const PYTHON_RUNTIME_VERSION = pythonRuntimeLock.pythonRuntime.version
const PYTHON_SERIES = pythonRuntimeLock.pythonRuntime.series
const PYTHON_TARGET = pythonRuntimeLock.pythonRuntime.target
const PYTHON_ASSET_NAME = pythonRuntimeLock.pythonRuntime.assetName
const PYTHON_ASSET_SHA256 = pythonRuntimeLock.pythonRuntime.archiveSha256
const PYTHON_FIXED_UPSTREAM_URL = `https://github.com/astral-sh/python-build-standalone/releases/download/20260623/${PYTHON_ASSET_NAME}`
const STAGE_ROOT = path.join(REPO_ROOT, 'build', 'windows-backend-runtime')
const PYTHON_RUNTIME_ROOT = path.join(STAGE_ROOT, '.python-runtime')
const WHEELHOUSE_DIR = path.join(STAGE_ROOT, 'backend-wheelhouse')
const SITE_PACKAGES_DIR = path.join(STAGE_ROOT, 'python-site-packages')
const RUNTIME_BIN_DIR = path.join(REPO_ROOT, 'runtime')
const ARCHIVE_EXTRACTOR_SOURCE = path.join(REPO_ROOT, 'node_modules', '7zip-bin', 'win', 'x64', '7za.exe')
const ARCHIVE_EXTRACTOR_TARGET = path.join(RUNTIME_BIN_DIR, '7za.exe')
const ARCHIVE_EXTRACTOR_MANIFEST = 'analytix-archive-extractor-manifest.json'
const SUBPROCESS_MAX_BUFFER = 32 * 1024 * 1024

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd || REPO_ROOT,
    env: options.env || process.env,
    encoding: options.encoding === undefined ? 'utf8' : options.encoding,
    maxBuffer: options.maxBuffer || SUBPROCESS_MAX_BUFFER,
    stdio: options.stdio || 'pipe',
    shell: process.platform === 'win32' && /\.(?:cmd|bat)$/iu.test(path.basename(command))
  })
  if (!result.error && result.status === 0) {
    return result
  }
  const detail = String(result.stderr || result.stdout || result.error?.message || '').trim()
  throw new Error(`${command} ${args.join(' ')} failed${detail ? `\n${detail}` : ''}`)
}

function resolvePythonCommand() {
  const explicitPython = String(process.env.PYTHON || '').trim()
  if (explicitPython) {
    return { command: explicitPython, argsPrefix: [] }
  }
  const candidates = process.platform === 'win32'
    ? [
        path.join(REPO_ROOT, '.venv', 'Scripts', 'python.exe'),
        path.join(REPO_ROOT, 'backend', '.venv', 'Scripts', 'python.exe')
      ]
    : [
        path.join(REPO_ROOT, '.venv', 'bin', 'python'),
        path.join(REPO_ROOT, 'backend', '.venv', 'bin', 'python')
      ]
  const managedPython = candidates.find((candidate) => fs.existsSync(candidate))
  if (managedPython) {
    return { command: managedPython, argsPrefix: [] }
  }
  return process.platform === 'win32'
    ? { command: 'py', argsPrefix: ['-3'] }
    : { command: 'python3', argsPrefix: [] }
}

function resolvePythonAsset() {
  const overrideUrl = String(process.env.ANALYTIX_WINDOWS_BACKEND_PYTHON_URL || '').trim()
  if (overrideUrl) {
    return {
      name: path.basename(new URL(overrideUrl).pathname) || PYTHON_ASSET_NAME,
      url: overrideUrl,
      sourceKind: 'mirror-url'
    }
  }
  if (process.env.ANALYTIX_WINDOWS_BACKEND_ALLOW_UPSTREAM_DOWNLOAD === '1') {
    return {
      name: PYTHON_ASSET_NAME,
      url: PYTHON_FIXED_UPSTREAM_URL,
      sourceKind: 'fixed-upstream-url'
    }
  }
  throw new Error(
    'Python runtime source is not configured. Set ANALYTIX_WINDOWS_BACKEND_PYTHON_RUNTIME_SOURCE to a local cached runtime directory, ' +
    'or set ANALYTIX_WINDOWS_BACKEND_PYTHON_URL to the mirrored fixed asset. ' +
    `Expected ${PYTHON_ASSET_NAME} with sha256 ${PYTHON_ASSET_SHA256}. ` +
    'The build intentionally does not resolve GitHub latest by default.'
  )
}

function validatePythonRuntimeRoot(root) {
  const pythonExe = path.join(root, 'python', 'python.exe')
  if (!fs.existsSync(pythonExe)) {
    throw new Error(`Standalone Python runtime is missing python.exe: ${pythonExe}`)
  }
  return pythonExe
}

function fileSha256(filePath) {
  return crypto.createHash('sha256').update(fs.readFileSync(filePath)).digest('hex')
}

function verifyExpectedSha256(filePath, expectedSha256, label) {
  const actual = fileSha256(filePath)
  if (actual.toLowerCase() !== expectedSha256.toLowerCase()) {
    throw new Error(`${label} sha256 mismatch. expected ${expectedSha256}, got ${actual}`)
  }
  return actual
}

function pruneFiles(root, shouldPrune) {
  let count = 0
  let bytes = 0
  if (!fs.existsSync(root)) {
    return { count, bytes }
  }
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const fullPath = path.join(root, entry.name)
    if (entry.isDirectory()) {
      const nested = pruneFiles(fullPath, shouldPrune)
      count += nested.count
      bytes += nested.bytes
      continue
    }
    if (!entry.isFile() || !shouldPrune(fullPath)) {
      continue
    }
    const size = fs.statSync(fullPath).size
    fs.rmSync(fullPath, { force: true })
    count += 1
    bytes += size
  }
  return { count, bytes }
}

function prunePythonRuntimeBuildArtifacts(root) {
  const result = pruneFiles(root, (filePath) => /\.pdb$/iu.test(filePath))
  if (result.count > 0) {
    console.log(`[backend-runtime] Removed ${result.count} Python debug symbol files (${result.bytes} bytes)`)
  }
}

function writeCanonicalJsonAtomic(filePath, value) {
  const temporary = `${filePath}.tmp-${process.pid}`
  fs.writeFileSync(temporary, `${JSON.stringify(value, null, 2)}\n`, { encoding: 'utf8', mode: 0o600 })
  fs.renameSync(temporary, filePath)
}

function writePythonRuntimeManifest(root) {
  const pythonExe = validatePythonRuntimeRoot(root)
  const pythonVersionText = pythonRuntimeVersionText(pythonExe)
  const identity = signingInvariantTreeIdentity(root, [RUNTIME_MANIFEST_NAME])
  const manifest = {
    schemaVersion: 2,
    lockSha256: pythonRuntimeLockSha256,
    pythonRuntimeVersion: PYTHON_RUNTIME_VERSION,
    pythonSeries: PYTHON_SERIES,
    pythonTarget: PYTHON_TARGET,
    pythonVersionText,
    assetName: PYTHON_ASSET_NAME,
    archiveSha256: PYTHON_ASSET_SHA256,
    sourceTreeSha256: pythonRuntimeLock.pythonRuntime.sourceTreeSha256,
    runtimeContentSha256: identity.sha256,
    runtimeFileCount: identity.fileCount
  }
  writeCanonicalJsonAtomic(path.join(root, RUNTIME_MANIFEST_NAME), manifest)
  verifyPythonRuntime(root)
  console.log(
    `[backend-runtime] Python runtime version=${manifest.pythonRuntimeVersion} ` +
    `archiveSha256=${manifest.archiveSha256} contentSha256=${manifest.runtimeContentSha256}`
  )
  return manifest
}

function pythonRuntimeVersionText(pythonExe) {
  if (process.platform === 'win32') {
    const versionResult = run(pythonExe, ['--version'])
    return String(versionResult.stdout || versionResult.stderr || '').trim()
  }
  return `Python ${PYTHON_RUNTIME_VERSION.split('+')[0]} (${PYTHON_TARGET}; verified by fixed asset sha256)`
}

function preparePythonRuntime(root) {
  validatePythonRuntimeRoot(root)
  rawTreeIdentity(root, [RUNTIME_MANIFEST_NAME])
  prunePythonRuntimeBuildArtifacts(root)
  const sourceIdentity = rawTreeIdentity(root, [RUNTIME_MANIFEST_NAME])
  if (sourceIdentity.sha256 !== pythonRuntimeLock.pythonRuntime.sourceTreeSha256) {
    throw new Error(
      `Python runtime source tree mismatch. expected ${pythonRuntimeLock.pythonRuntime.sourceTreeSha256}, got ${sourceIdentity.sha256}`
    )
  }
  pruneProductionPythonTree(root, { runtime: true })
  const identity = signingInvariantTreeIdentity(root, [RUNTIME_MANIFEST_NAME])
  if (
    identity.sha256 !== pythonRuntimeLock.pythonRuntime.runtimeContentSha256 ||
    identity.fileCount !== pythonRuntimeLock.pythonRuntime.runtimeFileCount
  ) {
    throw new Error('Python runtime production payload does not match the frozen authority')
  }
  writePythonRuntimeManifest(root)
  return validatePythonRuntimeRoot(root)
}

function stagePythonRuntimeFromSource({ sourceRoot, currentRoot, approvedSourceSha256 }) {
  validatePythonRuntimeRoot(sourceRoot)
  if (
    approvedSourceSha256 !== pythonRuntimeLock.pythonRuntime.sourceTreeSha256 ||
    !/^[0-9a-f]{64}$/.test(approvedSourceSha256)
  ) {
    throw new Error(
      'ANALYTIX_WINDOWS_BACKEND_PYTHON_RUNTIME_SOURCE_SHA256 must equal the frozen approved Python source tree SHA-256'
    )
  }
  rawTreeIdentity(sourceRoot, [RUNTIME_MANIFEST_NAME])
  fs.rmSync(currentRoot, { recursive: true, force: true })
  fs.mkdirSync(path.dirname(currentRoot), { recursive: true })
  fs.cpSync(sourceRoot, currentRoot, { recursive: true, dereference: false })
  return preparePythonRuntime(currentRoot)
}

function installPythonRuntime() {
  const sourceRoot = String(process.env.ANALYTIX_WINDOWS_BACKEND_PYTHON_RUNTIME_SOURCE || '').trim()
  const approvedSourceSha256 = String(
    process.env.ANALYTIX_WINDOWS_BACKEND_PYTHON_RUNTIME_SOURCE_SHA256 || ''
  ).trim().toLowerCase()
  const currentRoot = path.join(PYTHON_RUNTIME_ROOT, 'current')
  if (sourceRoot) {
    const pythonExe = stagePythonRuntimeFromSource({
      sourceRoot: path.resolve(REPO_ROOT, sourceRoot),
      currentRoot,
      approvedSourceSha256
    })
    console.log(`[backend-runtime] Python ready from source at ${pythonExe}`)
    return
  }

  const asset = resolvePythonAsset()
  const archivePath = path.join(os.tmpdir(), asset.name || 'analytix-python-windows.tar.gz')
  fs.rmSync(PYTHON_RUNTIME_ROOT, { recursive: true, force: true })
  fs.mkdirSync(currentRoot, { recursive: true })
  console.log(`[backend-runtime] Downloading fixed Python runtime ${PYTHON_RUNTIME_VERSION} from ${asset.sourceKind}: ${asset.url}`)
  run('curl', [
    '-L',
    '--fail',
    '--retry',
    '3',
    '--retry-all-errors',
    '--retry-delay',
    '3',
    '-C',
    '-',
    '-H',
    'User-Agent: Analytix-Desktop-Build',
    '-o',
    archivePath,
    asset.url
  ], { stdio: 'inherit' })
  const archiveSha256 = verifyExpectedSha256(archivePath, PYTHON_ASSET_SHA256, asset.name)
  run('tar', ['-xzf', archivePath, '-C', currentRoot], { stdio: 'inherit' })
  fs.rmSync(archivePath, { force: true })
  if (archiveSha256 !== PYTHON_ASSET_SHA256) throw new Error('Python runtime archive authority mismatch')
  console.log(`[backend-runtime] Python ready at ${preparePythonRuntime(currentRoot)}`)
}

function installBackendWheelhouse() {
  const python = resolvePythonCommand()
  fs.rmSync(WHEELHOUSE_DIR, { recursive: true, force: true })
  fs.mkdirSync(WHEELHOUSE_DIR, { recursive: true })
  run(
    python.command,
    [
      ...python.argsPrefix,
      '-m',
      'pip',
      'download',
      '--dest',
      WHEELHOUSE_DIR,
      '--only-binary=:all:',
      '--platform',
      'win_amd64',
      '--python-version',
      '311',
      '--implementation',
      'cp',
      '--abi',
      'cp311',
      '--require-hashes',
      '--requirement',
      REQUIREMENTS_LOCK_PATH
    ],
    { stdio: 'inherit' }
  )
  const verified = verifyWheelhouse(WHEELHOUSE_DIR)
  console.log(
    `[backend-runtime] Wheelhouse ready with ${verified.wheels.length} locked wheels ` +
    `wheelSetSha256=${verified.wheelSetSha256}`
  )
}

function stageBackendPythonSitePackages() {
  const python = resolvePythonCommand()
  const verifiedWheelhouse = verifyWheelhouse(WHEELHOUSE_DIR)
  const wheels = verifiedWheelhouse.wheels.map((item) => path.join(WHEELHOUSE_DIR, item.fileName))

  fs.rmSync(SITE_PACKAGES_DIR, { recursive: true, force: true })
  fs.mkdirSync(SITE_PACKAGES_DIR, { recursive: true })
  const extractScript = [
    'import pathlib, shutil, stat, sys, zipfile',
    'target = pathlib.Path(sys.argv[1]).resolve()',
    'target.mkdir(parents=True, exist_ok=True)',
    'seen = set()',
    'for wheel in sys.argv[2:]:',
    '    with zipfile.ZipFile(wheel) as archive:',
    '        for item in archive.infolist():',
    '            name = item.filename',
    '            parts = pathlib.PurePosixPath(name).parts',
    "            if not name or '\\\\' in name or name.startswith('/') or '..' in parts:",
    "                raise RuntimeError(f'unsafe wheel member: {name!r}')",
    '            mode = (item.external_attr >> 16) & 0o170000',
    '            if mode not in (0, stat.S_IFREG, stat.S_IFDIR):',
    "                raise RuntimeError(f'non-regular wheel member: {name!r}')",
    '            destination = target.joinpath(*parts)',
    '            if item.is_dir():',
    '                destination.mkdir(parents=True, exist_ok=True)',
    '                continue',
    '            key = name.casefold()',
    '            if key in seen:',
    "                raise RuntimeError(f'duplicate wheel member: {name!r}')",
    '            seen.add(key)',
    '            destination.parent.mkdir(parents=True, exist_ok=True)',
    "            with archive.open(item, 'r') as source, destination.open('xb') as output:",
    '                shutil.copyfileobj(source, output)'
  ].join('\n')
  run(
    python.command,
    [
      ...python.argsPrefix,
      '-c',
      extractScript,
      SITE_PACKAGES_DIR,
      ...wheels
    ],
    { stdio: 'inherit' }
  )
  pruneProductionPythonTree(SITE_PACKAGES_DIR)
  const identity = signingInvariantTreeIdentity(SITE_PACKAGES_DIR, [SITE_PACKAGES_MANIFEST_NAME])
  if (
    identity.sha256 !== pythonRuntimeLock.sitePackages.contentSha256 ||
    identity.fileCount !== pythonRuntimeLock.sitePackages.fileCount
  ) {
    throw new Error('Staged Python site-packages do not match the frozen content authority')
  }
  writeCanonicalJsonAtomic(path.join(SITE_PACKAGES_DIR, SITE_PACKAGES_MANIFEST_NAME), {
    schemaVersion: 2,
    lockSha256: pythonRuntimeLockSha256,
    requirementsLockSha256: pythonRuntimeLock.sitePackages.requirementsLockSha256,
    pythonSeries: PYTHON_SERIES,
    platform: pythonRuntimeLock.sitePackages.platform,
    wheelCount: verifiedWheelhouse.wheels.length,
    wheelSetSha256: verifiedWheelhouse.wheelSetSha256,
    wheels: verifiedWheelhouse.wheels,
    contentSha256: identity.sha256,
    fileCount: identity.fileCount
  })
  verifySitePackages(SITE_PACKAGES_DIR)
  console.log(`[backend-runtime] Staged python site-packages from ${wheels.length} wheels`)
}

function stageArchiveExtractor() {
  if (!fs.existsSync(ARCHIVE_EXTRACTOR_SOURCE)) {
    throw new Error(`Windows archive extractor is missing: ${ARCHIVE_EXTRACTOR_SOURCE}`)
  }
  fs.mkdirSync(RUNTIME_BIN_DIR, { recursive: true })
  fs.copyFileSync(ARCHIVE_EXTRACTOR_SOURCE, ARCHIVE_EXTRACTOR_TARGET)
  const sha256 = fileSha256(ARCHIVE_EXTRACTOR_TARGET)
  fs.writeFileSync(
    path.join(RUNTIME_BIN_DIR, ARCHIVE_EXTRACTOR_MANIFEST),
    `${JSON.stringify({
      schemaVersion: 1,
      tool: '7za',
      packageName: '7zip-bin',
      packageVersion: require(path.join(REPO_ROOT, 'node_modules', '7zip-bin', 'package.json')).version,
      source: path.relative(REPO_ROOT, ARCHIVE_EXTRACTOR_SOURCE).split(path.sep).join('/'),
      target: 'resources/runtime/7za.exe',
      sha256,
      generatedAt: new Date().toISOString()
    }, null, 2)}\n`,
    'utf8'
  )
  console.log(`[backend-runtime] Staged archive extractor 7za.exe sha256=${sha256}`)
}

function main() {
  installPythonRuntime()
  installBackendWheelhouse()
  stageBackendPythonSitePackages()
  stageArchiveExtractor()
}

if (require.main === module) {
  try {
    main()
  } catch (error) {
    console.error(`[backend-runtime] ${error instanceof Error ? error.message : String(error)}`)
    process.exitCode = 1
  }
}

module.exports = {
  PYTHON_ASSET_NAME,
  PYTHON_ASSET_SHA256,
  PYTHON_RUNTIME_VERSION,
  REQUIREMENTS_LOCK_PATH,
  installBackendWheelhouse,
  installPythonRuntime,
  preparePythonRuntime,
  prunePythonRuntimeBuildArtifacts,
  resolvePythonAsset,
  stageArchiveExtractor,
  stageBackendPythonSitePackages,
  stagePythonRuntimeFromSource
}
