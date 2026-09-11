const { isAbsolute, relative, resolve, sep } = require('node:path')

const MAX_COMPLETED_LIFECYCLES = 16
const completedLifecycles = new Map()
const startedArtifacts = new Map()

const TARGET_PRESENTATION = Object.freeze({
  dmg: Object.freeze({ presentable: 'DMG', extension: '.dmg' }),
  zip: Object.freeze({ presentable: 'macOS zip', extension: '.zip' }),
  nsis: Object.freeze({ presentable: 'nsis', extension: '.exe' }),
  appImage: Object.freeze({ presentable: 'AppImage', extension: '.AppImage' })
})

function prepackagedInput(packager) {
  return packager?.packagerOptions?.prepackaged ?? packager?.info?.options?.prepackaged
}

function isPrepackagedArgument(argument) {
  return argument === '--prepackaged' || argument.startsWith('--prepackaged=') ||
    argument === '--pd' || argument.startsWith('--pd=') ||
    argument === '-pd' || argument.startsWith('-pd=')
}

function assertNoPrepackagedCommandLine(argv = process.argv) {
  if (!Array.isArray(argv) || argv.some((argument) =>
    typeof argument === 'string' && isPrepackagedArgument(argument))) {
    throw new Error('[packaged-lifecycle] Prepackaged electron-builder input is not eligible')
  }
}

function exactTargetRecords(context) {
  if (!context || typeof context !== 'object' || !Array.isArray(context.targets)) {
    throw new Error('[packaged-lifecycle] Electron builder target inventory is unavailable')
  }
  const records = context.targets.map((target) => {
    const name = target?.name
    const contract = TARGET_PRESENTATION[name]
    if (!contract) {
      throw new Error('[packaged-lifecycle] Electron builder target is unsupported')
    }
    return Object.freeze({ name, ...contract })
  }).sort((left, right) => left.name.localeCompare(right.name))
  for (let index = 1; index < records.length; index += 1) {
    if (records[index - 1].name === records[index].name) {
      throw new Error('[packaged-lifecycle] Electron builder target inventory is ambiguous')
    }
  }
  return Object.freeze(records)
}

function lifecycleKey(outDir, arch) {
  return `${process.pid}:${arch}:${outDir}`
}

function artifactKey(file, arch) {
  return `${arch}:${file}`
}

// afterPack calls this only after the packaged authority and its complete
// staged payload have been written and read back. ArtifactBuildStarted has no
// Packager reference in electron-builder 26.15.3, so this same-process,
// target-bound completion record is the public hook's only trustworthy proof
// that a programmatic `prepackaged` input did not skip afterExtract/afterPack.
function recordCompletedAfterPack(context, authorityDigest) {
  if (prepackagedInput(context?.packager) != null) {
    throw new Error('[packaged-lifecycle] Prepackaged electron-builder input is not eligible')
  }
  const requestedOutDir = context?.outDir
  const arch = context?.arch
  if (typeof requestedOutDir !== 'string' || !requestedOutDir.trim() ||
    !isAbsolute(requestedOutDir.trim()) || !Number.isInteger(arch) || arch < 0 ||
    !context?.packager || typeof context.packager !== 'object' ||
    typeof authorityDigest !== 'string' || !/^[a-f0-9]{64}$/.test(authorityDigest)) {
    throw new Error('[packaged-lifecycle] Completed afterPack identity is invalid')
  }
  const outDir = resolve(requestedOutDir.trim())
  const targets = exactTargetRecords(context)
  // electron-builder 26.15.3 represents `--dir` as the exact empty Target[] in
  // PackContext. It still traverses afterExtract/afterPack, but emits no
  // ArtifactBuildStarted event and must not leave a replayable credential.
  if (targets.length === 0) return
  const key = lifecycleKey(outDir, arch)
  if (completedLifecycles.has(key) || completedLifecycles.size >= MAX_COMPLETED_LIFECYCLES) {
    throw new Error('[packaged-lifecycle] Completed afterPack lifecycle is duplicate or unbounded')
  }
  completedLifecycles.set(key, {
    outDir,
    arch,
    packager: context.packager,
    authorityDigest,
    targets,
    consumed: new Set()
  })
}

function artifactBuildStarted(event) {
  if (!event || typeof event !== 'object' || Array.isArray(event) ||
    Object.keys(event).sort().join(',') !== 'arch,file,targetPresentableName' ||
    typeof event.targetPresentableName !== 'string' || !event.targetPresentableName ||
    typeof event.file !== 'string' || !event.file.trim() || !isAbsolute(event.file.trim()) ||
    !Number.isInteger(event.arch) || event.arch < 0) {
    throw new Error('[packaged-lifecycle] ArtifactBuildStarted event is invalid')
  }
  const artifactPath = resolve(event.file.trim())
  const candidates = []
  for (const lifecycle of completedLifecycles.values()) {
    if (lifecycle.arch !== event.arch) continue
    const relativeArtifact = relative(lifecycle.outDir, artifactPath)
    if (!relativeArtifact || relativeArtifact === '..' || relativeArtifact.startsWith(`..${sep}`) ||
      isAbsolute(relativeArtifact)) continue
    const target = lifecycle.targets.find((candidate) =>
      candidate.presentable === event.targetPresentableName &&
      artifactPath.endsWith(candidate.extension) &&
      !lifecycle.consumed.has(candidate.name))
    if (target) candidates.push({ lifecycle, target })
  }
  if (candidates.length !== 1) {
    throw new Error('[packaged-lifecycle] Artifact lacks one exact completed afterPack lifecycle')
  }
  const { lifecycle, target } = candidates[0]
  const startedKey = artifactKey(artifactPath, event.arch)
  if (startedArtifacts.has(startedKey) || startedArtifacts.size >= MAX_COMPLETED_LIFECYCLES * 4) {
    throw new Error('[packaged-lifecycle] Artifact start is duplicate or unbounded')
  }
  startedArtifacts.set(startedKey, Object.freeze({
    targetName: target.name,
    artifactPath,
    arch: event.arch,
    packager: lifecycle.packager,
    authorityDigest: lifecycle.authorityDigest
  }))
  lifecycle.consumed.add(target.name)
  if (lifecycle.consumed.size === lifecycle.targets.length) {
    completedLifecycles.delete(lifecycleKey(lifecycle.outDir, lifecycle.arch))
  }
}

function artifactBuildCompleted(event) {
  // ArtifactCreated is the first public hook event that carries the real
  // PlatformPackager. This is the definitive programmatic-API guard even if a
  // prior failed multi-target build left an unused start credential in this
  // Node process.
  if (prepackagedInput(event?.packager) != null) {
    throw new Error('[packaged-lifecycle] Prepackaged electron-builder input is not eligible')
  }
  if (!event || typeof event !== 'object' || Array.isArray(event) ||
    !event.packager || typeof event.packager !== 'object' ||
    typeof event.file !== 'string' || !event.file.trim() || !isAbsolute(event.file.trim()) ||
    typeof event.target?.name !== 'string' || !event.target.name) {
    throw new Error('[packaged-lifecycle] ArtifactBuildCompleted event is invalid')
  }
  const artifactPath = resolve(event.file.trim())
  // electron-builder 26.15.3 emits exactly one completion-only sub-artifact
  // for the supported targets: `<main artifact>.blockmap`, with arch=null.
  // Bind it to the already-started main artifact and the exact PlatformPackager
  // but leave the main credential live for its later completion.
  if (event.arch === null) {
    const suffix = '.blockmap'
    if (!artifactPath.endsWith(suffix)) {
      throw new Error('[packaged-lifecycle] ArtifactBuildCompleted sub-artifact is unsupported')
    }
    const mainArtifactPath = artifactPath.slice(0, -suffix.length)
    const candidates = [...startedArtifacts.values()].filter((started) =>
      started.artifactPath === mainArtifactPath && started.targetName === event.target.name &&
      started.packager === event.packager)
    if (candidates.length !== 1) {
      throw new Error('[packaged-lifecycle] Artifact sub-completion lacks one exact start credential')
    }
    return
  }
  if (!Number.isInteger(event.arch) || event.arch < 0) {
    throw new Error('[packaged-lifecycle] ArtifactBuildCompleted event is invalid')
  }
  const key = artifactKey(artifactPath, event.arch)
  const started = startedArtifacts.get(key)
  if (!started || started.targetName !== event.target.name ||
    started.artifactPath !== artifactPath || started.arch !== event.arch ||
    started.packager !== event.packager ||
    typeof started.authorityDigest !== 'string' || !/^[a-f0-9]{64}$/.test(started.authorityDigest)) {
    throw new Error('[packaged-lifecycle] Artifact completion lacks one exact start credential')
  }
  startedArtifacts.delete(key)
}

function resetForTests() {
  if (process.env.NODE_ENV !== 'test') {
    throw new Error('[packaged-lifecycle] Lifecycle state reset is test-only')
  }
  completedLifecycles.clear()
  startedArtifacts.clear()
}

exports._internals = {
  artifactBuildStarted,
  artifactBuildCompleted,
  assertNoPrepackagedCommandLine,
  isPrepackagedArgument,
  prepackagedInput,
  recordCompletedAfterPack,
  resetForTests
}
exports.artifactBuildStarted = artifactBuildStarted
exports.artifactBuildCompleted = artifactBuildCompleted
exports.default = artifactBuildStarted
