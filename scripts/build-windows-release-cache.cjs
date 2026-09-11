const crypto = require('node:crypto')
const { spawnSync } = require('node:child_process')
const fs = require('node:fs')
const path = require('node:path')
const {
  verifyPythonRuntime,
  verifySitePackages
} = require('./windows-python-runtime-contract.cjs')

const repoRoot = path.resolve(__dirname, '..')
const cacheRoot = process.env.ANALYTIX_WINDOWS_RELEASE_CACHE ||
  path.join(repoRoot, '.cache', 'windows-release')
const cacheSchemaVersion = 3

function toPosix(value) {
  return String(value).split(path.sep).join('/')
}

function resolveRepoPath(relativePath) {
  return path.isAbsolute(relativePath) ? relativePath : path.join(repoRoot, relativePath)
}

function relativeLabel(filePath) {
  const relative = path.relative(repoRoot, filePath)
  return relative && !relative.startsWith('..') ? toPosix(relative) : filePath
}

function fileSha256(filePath) {
  return crypto.createHash('sha256').update(fs.readFileSync(filePath)).digest('hex')
}

function treeDigest(root) {
  const hash = crypto.createHash('sha256')
  let size = 0
  const files = collectFiles(root).sort((left, right) => left.localeCompare(right, 'en'))
  for (const filePath of files) {
    const stat = fs.lstatSync(filePath)
    const label = relativeLabel(filePath)
    hash.update(label)
    hash.update('\0')
    if (stat.isSymbolicLink()) {
      hash.update('symlink\0')
      hash.update(fs.readlinkSync(filePath))
      hash.update('\0')
      continue
    }
    if (!stat.isFile()) continue
    size += stat.size
    hash.update(String(stat.size))
    hash.update('\0')
    hash.update(fileSha256(filePath))
    hash.update('\0')
  }
  return { size, sha256: hash.digest('hex') }
}

function collectFiles(root, acc = [], current = root) {
  if (!fs.existsSync(current)) return acc
  const stat = fs.lstatSync(current)
  if (stat.isSymbolicLink()) {
    acc.push(current)
    return acc
  }
  if (stat.isFile()) {
    acc.push(current)
    return acc
  }
  if (!stat.isDirectory()) return acc

  for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
    if (
      (current === root && (entry.name === 'target' || entry.name === '.git')) ||
      entry.name === 'node_modules' ||
      entry.name === '__pycache__'
    ) {
      continue
    }
    collectFiles(root, acc, path.join(current, entry.name))
  }
  return acc
}

function updateHashWithPath(hash, inputPath, label) {
  const resolved = resolveRepoPath(inputPath)
  hash.update(`input:${label || relativeLabel(resolved)}\0`)
  if (!fs.existsSync(resolved)) {
    hash.update('missing\0')
    return
  }

  const stat = fs.lstatSync(resolved)
  if (stat.isSymbolicLink()) {
    hash.update('symlink\0')
    hash.update(fs.readlinkSync(resolved))
    hash.update('\0')
    return
  }

  if (stat.isFile()) {
    hash.update('file\0')
    hash.update(String(stat.size))
    hash.update('\0')
    hash.update(fileSha256(resolved))
    hash.update('\0')
    return
  }

  if (!stat.isDirectory()) {
    hash.update(`${stat.mode}\0`)
    return
  }

  hash.update('dir\0')
  const files = collectFiles(resolved).sort((left, right) => left.localeCompare(right, 'en'))
  for (const filePath of files) {
    const fileStat = fs.lstatSync(filePath)
    hash.update(relativeLabel(filePath))
    hash.update('\0')
    if (fileStat.isSymbolicLink()) {
      hash.update('symlink\0')
      hash.update(fs.readlinkSync(filePath))
      hash.update('\0')
      continue
    }
    hash.update(String(fileStat.size))
    hash.update('\0')
    hash.update(fileSha256(filePath))
    hash.update('\0')
  }
}

function envValue(name, env = process.env) {
  return String(env[name] || '').trim()
}

function backendDynamicInputs(env = process.env) {
  const sourceRoot = envValue('ANALYTIX_WINDOWS_BACKEND_PYTHON_RUNTIME_SOURCE', env)
  if (!sourceRoot) return []
  return [
    {
      path: path.resolve(repoRoot, sourceRoot),
      label: 'ANALYTIX_WINDOWS_BACKEND_PYTHON_RUNTIME_SOURCE'
    }
  ]
}

const steps = {
  'backend-win-runtime': {
    description: 'Windows backend Python runtime, wheels, and archive extractor',
    command: [process.execPath, ['./scripts/build-windows-backend-runtime-assets.cjs']],
    inputs: [
      'scripts/build-windows-backend-runtime-assets.cjs',
      'scripts/windows-python-runtime-contract.cjs',
      'scripts/windows-python-runtime-lock.json',
      'scripts/windows-backend-requirements.lock.txt',
      'package.json',
      'package-lock.json',
      'node_modules/7zip-bin/package.json',
      'node_modules/7zip-bin/win/x64/7za.exe'
    ],
    dynamicInputs: backendDynamicInputs,
    env: [
      'ANALYTIX_WINDOWS_BACKEND_PYTHON_RUNTIME_SOURCE',
      'ANALYTIX_WINDOWS_BACKEND_PYTHON_RUNTIME_SOURCE_SHA256',
      'ANALYTIX_WINDOWS_BACKEND_PYTHON_URL',
      'ANALYTIX_WINDOWS_BACKEND_ALLOW_UPSTREAM_DOWNLOAD',
      'PYTHON'
    ],
    outputs: [
      'build/windows-backend-runtime/.python-runtime/current',
      'build/windows-backend-runtime/.python-runtime/current/python/python.exe',
      'build/windows-backend-runtime/.python-runtime/current/analytix-python-runtime-manifest.json',
      'build/windows-backend-runtime/python-site-packages',
      'build/windows-backend-runtime/python-site-packages/analytix-site-packages-manifest.json',
      'runtime/7za.exe',
      'runtime/analytix-archive-extractor-manifest.json'
    ]
  }
}

function assertCacheStepAvailable(stepName) {
  if (stepName === 'data-native') {
    throw new Error('native_component_release_authority_unavailable')
  }
}

function stepCachePath(stepName) {
  return path.join(cacheRoot, `${stepName}.json`)
}

function outputStatus(outputs) {
  return outputs.map((outputPath) => {
    const resolved = resolveRepoPath(outputPath)
    if (!fs.existsSync(resolved)) {
      return { path: toPosix(outputPath), exists: false, kind: 'missing', size: 0, sha256: '' }
    }
    const stat = fs.lstatSync(resolved)
    if (stat.isDirectory()) {
      const digest = treeDigest(resolved)
      return {
        path: toPosix(outputPath),
        exists: true,
        kind: 'directory',
        size: digest.size,
        sha256: digest.sha256
      }
    }
    return {
      path: toPosix(outputPath),
      exists: stat.isFile(),
      kind: stat.isFile() ? 'file' : 'other',
      size: stat.isFile() ? stat.size : 0,
      sha256: stat.isFile() ? fileSha256(resolved) : ''
    }
  })
}

function outputsReady(outputs) {
  return outputStatus(outputs).every((item) => item.exists && item.size > 0)
}

function outputsMatchState(current, state) {
  if (!state || !Array.isArray(state.outputs)) return false
  const expected = new Map(state.outputs.map((item) => [item.path, item]))
  for (const item of current) {
    const previous = expected.get(item.path)
    if (!previous) return false
    if (!item.exists || previous.exists !== true) return false
    if (item.kind !== previous.kind) return false
    if (item.size !== previous.size) return false
    if (!item.sha256 || item.sha256 !== previous.sha256) return false
  }
  return true
}

function verifyStepOutputs(stepName) {
  try {
    assertCacheStepAvailable(stepName)
    if (stepName === 'backend-win-runtime') {
      verifyPythonRuntime(resolveRepoPath('build/windows-backend-runtime/.python-runtime/current'))
      verifySitePackages(resolveRepoPath('build/windows-backend-runtime/python-site-packages'))
    } else {
      return true
    }
    return true
  } catch (error) {
    console.warn(`[official-win-cache] ${stepName} semantic verification failed: ${error instanceof Error ? error.message : String(error)}`)
    return false
  }
}

function computeStepKey(stepName, env = process.env) {
  assertCacheStepAvailable(stepName)
  const step = steps[stepName]
  if (!step) {
    throw new Error(`Unknown Windows release cache step: ${stepName}`)
  }

  const hash = crypto.createHash('sha256')
  hash.update(`schema:${cacheSchemaVersion}\0`)
  hash.update(`step:${stepName}\0`)
  hash.update(`platform:${process.platform}\0`)
  hash.update(`arch:${process.arch}\0`)
  hash.update(`node:${process.version}\0`)
  hash.update(`command:${step.command[0]} ${step.command[1].join(' ')}\0`)

  for (const name of step.env || []) {
    hash.update(`env:${name}=${envValue(name, env)}\0`)
  }

  for (const input of step.inputs || []) {
    updateHashWithPath(hash, input)
  }

  for (const input of step.dynamicInputs ? step.dynamicInputs(env) : []) {
    updateHashWithPath(hash, input.path, input.label)
  }

  return hash.digest('hex')
}

function readCacheState(stepName) {
  const cachePath = stepCachePath(stepName)
  if (!fs.existsSync(cachePath)) return null
  try {
    return JSON.parse(fs.readFileSync(cachePath, 'utf8'))
  } catch {
    return null
  }
}

function writeCacheState(stepName, key, outputs) {
  fs.mkdirSync(cacheRoot, { recursive: true })
  fs.writeFileSync(
    stepCachePath(stepName),
    `${JSON.stringify({
      schemaVersion: cacheSchemaVersion,
      step: stepName,
      key,
      platform: process.platform,
      arch: process.arch,
      outputs: outputStatus(outputs),
      updatedAt: new Date().toISOString()
    }, null, 2)}\n`,
    'utf8'
  )
}

function runCommand(stepName, step) {
  const [command, args] = step.command
  console.log(`[official-win-cache] building ${stepName}: ${step.description}`)
  const result = spawnSync(command, args, {
    cwd: repoRoot,
    stdio: 'inherit',
    env: process.env
  })
  if (result.error) {
    throw result.error
  }
  if (result.status !== 0) {
    throw new Error(`${stepName} failed with exit code ${result.status}`)
  }
}

function cacheDisabled() {
  return /^(1|true|yes|on)$/i.test(String(process.env.ANALYTIX_RELEASE_CACHE_DISABLE || '').trim())
}

function runStep(stepName) {
  assertCacheStepAvailable(stepName)
  const step = steps[stepName]
  if (!step) {
    throw new Error(`Unknown Windows release cache step: ${stepName}. Expected one of: ${Object.keys(steps).join(', ')}`)
  }

  const key = computeStepKey(stepName)
  const state = readCacheState(stepName)
  const currentOutputs = outputStatus(step.outputs)
  const ready = currentOutputs.every((item) => item.exists && item.size > 0)
  if (
    !cacheDisabled() &&
    ready &&
    state &&
    state.key === key &&
    outputsMatchState(currentOutputs, state) &&
    verifyStepOutputs(stepName)
  ) {
    console.log(`[official-win-cache] ${stepName} cache hit; verified ${step.outputs.length} outputs.`)
    return { step: stepName, cacheHit: true, key }
  }

  if (cacheDisabled()) {
    console.log(`[official-win-cache] ${stepName} cache disabled by ANALYTIX_RELEASE_CACHE_DISABLE.`)
  } else if (!ready) {
    console.log(`[official-win-cache] ${stepName} cache miss: required outputs are missing.`)
  } else {
    console.log(`[official-win-cache] ${stepName} cache miss: inputs changed.`)
  }

  runCommand(stepName, step)
  if (!outputsReady(step.outputs)) {
    throw new Error(`${stepName} completed but required outputs are missing or empty.`)
  }
  if (!verifyStepOutputs(stepName)) {
    throw new Error(`${stepName} completed but semantic output verification failed.`)
  }
  writeCacheState(stepName, key, step.outputs)
  return { step: stepName, cacheHit: false, key }
}

function printHelp() {
  console.log([
    'Usage: node scripts/build-windows-release-cache.cjs <step>',
    '',
    'Steps:',
    ...Object.entries(steps).map(([name, step]) => `  ${name} - ${step.description}`),
    '',
    'Set ANALYTIX_RELEASE_CACHE_DISABLE=1 to force rebuild cached steps.'
  ].join('\n'))
}

function main(argv = process.argv.slice(2)) {
  const stepName = argv[0]
  if (!stepName || stepName === '--help' || stepName === '-h') {
    printHelp()
    return
  }
  runStep(stepName)
}

if (require.main === module) {
  try {
    main()
  } catch (error) {
    console.error(`[official-win-cache] ${error instanceof Error ? error.message : String(error)}`)
    process.exitCode = 1
  }
}

module.exports = {
  _internals: {
    backendDynamicInputs,
    assertCacheStepAvailable,
    cacheDisabled,
    computeStepKey,
    outputStatus,
    outputsMatchState,
    outputsReady,
    steps,
    verifyStepOutputs
  },
  runStep
}
