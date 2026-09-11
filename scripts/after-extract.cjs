const { createHash } = require('node:crypto')
const { realpathSync } = require('node:fs')
const { isAbsolute, join, resolve } = require('node:path')

const afterPackContract = require('./after-pack.cjs')._internals

const AFTER_EXTRACT_SNAPSHOT_CONTRACT = 'analytix.electron-builder-after-extract-snapshot/v1'
const MAX_LIFECYCLE_KEYS = 32
const pendingSnapshots = new Map()
const consumedKeys = new Set()

function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}

function assertNotPrepackaged(context, prefix) {
  const prepackaged = context?.packager?.packagerOptions?.prepackaged ??
    context?.packager?.info?.options?.prepackaged
  if (prepackaged != null) {
    throw new Error(`[${prefix}] Prepackaged electron-builder input is not eligible for packaged authority`)
  }
}

function lifecycleIdentity(context, repoRoot) {
  if (!context || typeof context !== 'object' ||
    typeof context.appOutDir !== 'string' || !context.appOutDir.trim() ||
    typeof context.outDir !== 'string' || !context.outDir.trim()) {
    throw new Error('[after-extract] Electron builder task identity is unavailable')
  }
  assertNotPrepackaged(context, 'after-extract')
  const targetKey = afterPackContract.packagedTargetKey(context)
  const requestedAppOutDir = context.appOutDir.trim()
  const requestedOutDir = context.outDir.trim()
  if (!isAbsolute(requestedAppOutDir) || !isAbsolute(requestedOutDir)) {
    throw new Error('[after-extract] Electron builder output identity is invalid')
  }
  const appOutDir = resolve(requestedAppOutDir)
  const outDir = resolve(requestedOutDir)
  const taskDigest = sha256(JSON.stringify({
    processId: process.pid,
    repoRoot,
    appOutDir,
    outDir
  }))
  return {
    key: `${taskDigest}:${targetKey}`,
    taskDigest,
    targetKey,
    appOutDirSha256: sha256(appOutDir),
    outDirSha256: sha256(outDir)
  }
}

function captureAfterExtractSnapshot(context, options = {}) {
  const repoRoot = realpathSync(options.repoRoot || join(__dirname, '..'))
  const identity = lifecycleIdentity(context, repoRoot)
  if (pendingSnapshots.has(identity.key)) {
    throw new Error('[after-extract] Duplicate pending source snapshot for task and target')
  }
  if (consumedKeys.has(identity.key)) {
    throw new Error('[after-extract] Source snapshot task and target was already consumed')
  }
  if (pendingSnapshots.size + consumedKeys.size >= MAX_LIFECYCLE_KEYS) {
    throw new Error('[after-extract] Source snapshot lifecycle key bound exceeded')
  }
  const collectSnapshot = options.collectSnapshot ||
    afterPackContract.collectPackagedWorktreeSnapshotV1
  const snapshot = collectSnapshot(repoRoot)
  if (!afterPackContract.isPackagedWorktreeSnapshotV1(snapshot)) {
    throw new Error('[after-extract] Source worktree snapshot is invalid')
  }
  if (afterPackContract.formalPackagedReleaseIntent(options.env || process.env) && snapshot.dirty) {
    throw new Error('[after-extract] Formal packaged release requires a clean Git worktree')
  }
  const buildContext = (options.collectBuildContext ||
    afterPackContract.collectEffectiveBuilderContextV1)(context)
  if (!afterPackContract.isEffectiveBuilderContextV1(buildContext)) {
    throw new Error('[after-extract] Effective builder context is invalid')
  }
  const record = Object.freeze({
    schemaVersion: 1,
    contract: AFTER_EXTRACT_SNAPSHOT_CONTRACT,
    taskDigest: identity.taskDigest,
    targetKey: identity.targetKey,
    appOutDirSha256: identity.appOutDirSha256,
    outDirSha256: identity.outDirSha256,
    repoRootSha256: sha256(repoRoot),
    snapshotDigest: snapshot.snapshotDigest,
    snapshotCanonical: JSON.stringify(snapshot),
    buildContextDigest: buildContext.contextDigest,
    buildContextCanonical: JSON.stringify(buildContext)
  })
  pendingSnapshots.set(identity.key, record)
  return record
}

function consumeAfterExtractSnapshot(context, options = {}) {
  const repoRoot = realpathSync(options.repoRoot || join(__dirname, '..'))
  const identity = lifecycleIdentity(context, repoRoot)
  if (consumedKeys.has(identity.key)) {
    throw new Error('[after-pack] AfterExtract snapshot replay detected for task and target')
  }
  const record = pendingSnapshots.get(identity.key)
  if (!record) {
    throw new Error('[after-pack] Required same-process afterExtract snapshot is missing')
  }
  pendingSnapshots.delete(identity.key)
  consumedKeys.add(identity.key)
  if (record.contract !== AFTER_EXTRACT_SNAPSHOT_CONTRACT ||
    record.taskDigest !== identity.taskDigest ||
    record.targetKey !== identity.targetKey ||
    record.appOutDirSha256 !== identity.appOutDirSha256 ||
    record.outDirSha256 !== identity.outDirSha256 ||
    record.repoRootSha256 !== sha256(repoRoot) ||
    !/^[0-9a-f]{64}$/.test(record.snapshotDigest) ||
    !/^[0-9a-f]{64}$/.test(record.buildContextDigest) ||
    typeof record.snapshotCanonical !== 'string' || !record.snapshotCanonical ||
    typeof record.buildContextCanonical !== 'string' || !record.buildContextCanonical) {
    throw new Error('[after-pack] AfterExtract snapshot task or target binding is invalid')
  }
  return record
}

function afterExtract(context) {
  const record = captureAfterExtractSnapshot(context)
  console.log(
    `[after-extract] Captured ${record.targetKey} source snapshot ${record.snapshotDigest}.`
  )
}

exports.AFTER_EXTRACT_SNAPSHOT_CONTRACT = AFTER_EXTRACT_SNAPSHOT_CONTRACT
exports._internals = {
  assertNotPrepackaged,
  captureAfterExtractSnapshot,
  consumeAfterExtractSnapshot,
  lifecycleIdentity
}
exports.default = afterExtract
