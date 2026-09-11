'use strict'

const { execFileSync } = require('node:child_process')
const {
  accessSync,
  closeSync,
  constants,
  fstatSync,
  lstatSync,
  openSync,
  realpathSync
} = require('node:fs')
const { join } = require('node:path')

const DEVELOPMENT_CACHE_MOUNT = '/Volumes/AnalytixCache'
const DEVELOPMENT_CACHE_ROOT = '/Volumes/AnalytixCache/development-v3'
const DEVELOPMENT_CACHE_BACKING_MOUNT = '/Volumes/DataSSD'
const DEVELOPMENT_CACHE_IMAGE =
  '/Volumes/DataSSD/analytix/AnalytixCache-v3.sparsebundle'
const DEVELOPMENT_CACHE_DAMAGED_V2_IMAGE =
  '/Volumes/DataSSD/analytix/AnalytixCache-v2.sparsebundle'
const DEVELOPMENT_CACHE_LEGACY_IMAGE =
  '/Volumes/DataSSD/analytix/AnalytixCache.sparsebundle'
const DEVELOPMENT_CACHE_BACKING_VOLUME_UUID =
  'DE25E209-CF5A-3364-B7D8-EF992B64F732'
const DEVELOPMENT_CACHE_VOLUME_UUID =
  '090478FC-CE2B-4E25-98F1-667C3252DD42'
const DEVELOPMENT_CACHE_APFS_VOLUME_TYPE =
  '41504653-0000-11AA-AA11-00306543ECAC'
const DEVELOPMENT_CACHE_ENVIRONMENT = Object.freeze({
  GOCACHE: join(DEVELOPMENT_CACHE_ROOT, 'go-build'),
  GOMODCACHE: join(DEVELOPMENT_CACHE_ROOT, 'go-mod'),
  GOTMPDIR: join(DEVELOPMENT_CACHE_ROOT, 'tmp'),
  CARGO_TARGET_DIR: join(DEVELOPMENT_CACHE_ROOT, 'cargo-target')
})
const DEVELOPMENT_NATIVE_COMPONENT_ROOT = join(
  DEVELOPMENT_CACHE_ROOT,
  'native-components-development'
)
const DEVELOPMENT_CACHE_MARKER = 'ANALYTIX_DEV_CACHE_ROOT'
const DEVELOPMENT_CACHE_COMMAND_ENVIRONMENT = Object.freeze({
  LANG: 'C',
  LC_ALL: 'C',
  PATH: '/usr/bin:/bin:/usr/sbin:/sbin'
})

function identityFromStat(path, stat) {
  return {
    path,
    realpath: realpathSync(path),
    dev: String(stat.dev),
    ino: String(stat.ino),
    uid: stat.uid,
    gid: stat.gid,
    mode: stat.mode & 0o7777,
    directory: stat.isDirectory(),
    symlink: stat.isSymbolicLink()
  }
}

function pathIdentity(path) {
  return identityFromStat(path, lstatSync(path))
}

function optionalPathIdentity(path) {
  let stat
  try {
    stat = lstatSync(path)
  } catch (error) {
    if (error?.code === 'ENOENT') return null
    throw error
  }
  return identityFromStat(path, stat)
}

function parsePlistCommand(command, args) {
  const plist = execFileSync(command, args, {
    encoding: null,
    env: DEVELOPMENT_CACHE_COMMAND_ENVIRONMENT,
    maxBuffer: 8 * 1024 * 1024
  })
  const json = execFileSync(
    '/usr/bin/plutil',
    ['-convert', 'json', '-o', '-', '-'],
    {
      encoding: 'utf8',
      env: DEVELOPMENT_CACHE_COMMAND_ENVIRONMENT,
      input: plist,
      maxBuffer: 8 * 1024 * 1024
    }
  )
  return JSON.parse(json)
}

function collectDevelopmentCacheVolumeSnapshot() {
  if (process.platform !== 'darwin') {
    throw new Error('[development-cache] The development cache volume requires macOS')
  }
  const backing = parsePlistCommand(
    '/usr/sbin/diskutil',
    ['info', '-plist', DEVELOPMENT_CACHE_BACKING_MOUNT]
  )
  const cache = parsePlistCommand(
    '/usr/sbin/diskutil',
    ['info', '-plist', DEVELOPMENT_CACHE_MOUNT]
  )
  const hdiutil = parsePlistCommand('/usr/bin/hdiutil', ['info', '-plist'])
  const mounts = execFileSync('/sbin/mount', [], {
    encoding: 'utf8',
    env: DEVELOPMENT_CACHE_COMMAND_ENVIRONMENT,
    maxBuffer: 2 * 1024 * 1024
  })
  accessSync(DEVELOPMENT_CACHE_MOUNT, constants.R_OK | constants.W_OK)
  return {
    backing,
    cache,
    hdiutil,
    mounts,
    effectiveUid: typeof process.getuid === 'function' ? process.getuid() : null,
    paths: {
      backing: pathIdentity(DEVELOPMENT_CACHE_BACKING_MOUNT),
      cacheMount: pathIdentity(DEVELOPMENT_CACHE_MOUNT),
      image: pathIdentity(DEVELOPMENT_CACHE_IMAGE),
      damagedV2Image: optionalPathIdentity(DEVELOPMENT_CACHE_DAMAGED_V2_IMAGE),
      legacyImage: optionalPathIdentity(DEVELOPMENT_CACHE_LEGACY_IMAGE)
    }
  }
}

function assertCanonicalDirectoryIdentity(identity, expectedPath, label) {
  if (!identity || identity.path !== expectedPath ||
    identity.realpath !== expectedPath || identity.directory !== true ||
    identity.symlink !== false) {
    throw new Error(`[development-cache] ${label} path identity is not authoritative`)
  }
}

function assertRetiredSparsebundleIdentity(identity, expectedPath, label, backingDevice) {
  if (identity === null) return
  assertCanonicalDirectoryIdentity(identity, expectedPath, label)
  if (identity.dev !== backingDevice) {
    throw new Error(`[development-cache] ${label} escaped the trusted backing volume`)
  }
}

function validateDevelopmentCacheVolumeSnapshot(snapshot) {
  if (!snapshot || typeof snapshot !== 'object') {
    throw new Error('[development-cache] Development cache volume snapshot is unavailable')
  }
  const { backing, cache, hdiutil, mounts, paths } = snapshot
  if (!backing || backing.MountPoint !== DEVELOPMENT_CACHE_BACKING_MOUNT ||
    backing.VolumeUUID !== DEVELOPMENT_CACHE_BACKING_VOLUME_UUID ||
    backing.Writable !== true) {
    throw new Error('[development-cache] Backing-volume identity is not authoritative')
  }
  if (!cache || cache.MountPoint !== DEVELOPMENT_CACHE_MOUNT ||
    cache.FilesystemType !== 'apfs' || cache.Writable !== true ||
    cache.GlobalPermissionsEnabled !== true ||
    cache.VolumeUUID !== DEVELOPMENT_CACHE_VOLUME_UUID ||
    typeof cache.DeviceIdentifier !== 'string' ||
    !/^disk[0-9]+s[0-9]+$/u.test(cache.DeviceIdentifier)) {
    throw new Error('[development-cache] Cache-volume identity is not authoritative')
  }
  assertCanonicalDirectoryIdentity(
    paths?.backing,
    DEVELOPMENT_CACHE_BACKING_MOUNT,
    'backing volume'
  )
  assertCanonicalDirectoryIdentity(
    paths?.cacheMount,
    DEVELOPMENT_CACHE_MOUNT,
    'cache mount'
  )
  assertCanonicalDirectoryIdentity(
    paths?.image,
    DEVELOPMENT_CACHE_IMAGE,
    'v3 sparsebundle'
  )
  assertRetiredSparsebundleIdentity(
    paths?.damagedV2Image,
    DEVELOPMENT_CACHE_DAMAGED_V2_IMAGE,
    'retired damaged v2 sparsebundle',
    paths.backing.dev
  )
  assertRetiredSparsebundleIdentity(
    paths?.legacyImage,
    DEVELOPMENT_CACHE_LEGACY_IMAGE,
    'retired legacy sparsebundle',
    paths.backing.dev
  )
  if (paths.image.dev !== paths.backing.dev) {
    throw new Error('[development-cache] Sparsebundle escaped the trusted backing volume')
  }
  if (snapshot.effectiveUid !== null &&
    paths.cacheMount.uid !== snapshot.effectiveUid) {
    throw new Error('[development-cache] Cache mount owner is not authoritative')
  }

  const images = Array.isArray(hdiutil?.images) ? hdiutil.images : []
  if (images.some((image) => image?.['image-path'] === DEVELOPMENT_CACHE_DAMAGED_V2_IMAGE)) {
    throw new Error('[development-cache] Damaged v2 cache image is attached')
  }
  if (images.some((image) => image?.['image-path'] === DEVELOPMENT_CACHE_LEGACY_IMAGE)) {
    throw new Error('[development-cache] Legacy damaged cache image is attached')
  }
  const attached = images.filter(
    (image) => image?.['image-path'] === DEVELOPMENT_CACHE_IMAGE
  )
  if (attached.length !== 1) {
    throw new Error('[development-cache] Trusted v3 sparsebundle attachment is ambiguous')
  }
  const entities = Array.isArray(attached[0]?.['system-entities'])
    ? attached[0]['system-entities']
    : []
  const expectedDevice = `/dev/${cache.DeviceIdentifier}`
  const mountedEntities = entities.filter(
    (entity) => entity?.['content-hint'] === DEVELOPMENT_CACHE_APFS_VOLUME_TYPE &&
      entity?.['dev-entry'] === expectedDevice &&
      entity?.['mount-point'] === DEVELOPMENT_CACHE_MOUNT
  )
  if (mountedEntities.length !== 1) {
    throw new Error('[development-cache] Cache mount is not bound to the trusted v3 image')
  }

  const mountLines = String(mounts || '').split(/\r?\n/u).filter(Boolean)
  const prefix = `${expectedDevice} on ${DEVELOPMENT_CACHE_MOUNT} (`
  const matchingMounts = mountLines.filter((line) => line.startsWith(prefix))
  if (matchingMounts.length !== 1 || !matchingMounts[0].startsWith(`${prefix}apfs,`) ||
    /(?:^|[, ])(?:noowners|read-only|ro)(?:[, )]|$)/u.test(matchingMounts[0])) {
    throw new Error('[development-cache] Cache mount flags are unsafe')
  }
  return snapshot
}

function verifyDevelopmentCacheVolume(options = {}) {
  if (options.collectSnapshot && process.env.NODE_ENV !== 'test') {
    throw new Error('[development-cache] Cache-volume collector override is test-only')
  }
  const collect = options.collectSnapshot || collectDevelopmentCacheVolumeSnapshot
  return validateDevelopmentCacheVolumeSnapshot(collect())
}

function sameMountIdentity(left, right) {
  return Boolean(left && right &&
    left.dev === right.dev &&
    left.ino === right.ino &&
    left.uid === right.uid &&
    left.gid === right.gid &&
    left.mode === right.mode &&
    left.directory === true &&
    right.directory === true &&
    left.symlink === false &&
    right.symlink === false)
}

function openVerifiedDevelopmentCacheVolume(options = {}) {
  if (options.collectSnapshot) {
    throw new Error('[development-cache] Open cache-volume override is unavailable')
  }
  const before = verifyDevelopmentCacheVolume()
  const flags = constants.O_RDONLY |
    (constants.O_DIRECTORY || 0) |
    (constants.O_NOFOLLOW || 0)
  const descriptor = openSync(DEVELOPMENT_CACHE_MOUNT, flags)
  let closed = false
  function verify() {
    if (closed) {
      throw new Error('[development-cache] Cache-volume verifier is closed')
    }
    const opened = fstatSync(descriptor)
    if (!opened.isDirectory()) {
      throw new Error('[development-cache] Open cache mount is not a directory')
    }
    const current = verifyDevelopmentCacheVolume()
    const openedIdentity = {
      dev: String(opened.dev),
      ino: String(opened.ino),
      uid: opened.uid,
      gid: opened.gid,
      mode: opened.mode & 0o7777,
      directory: opened.isDirectory(),
      symlink: false
    }
    if (!sameMountIdentity(before.paths.cacheMount, current.paths.cacheMount) ||
      !sameMountIdentity(current.paths.cacheMount, openedIdentity)) {
      throw new Error('[development-cache] Cache mount changed while in use')
    }
    return current
  }
  try {
    verify()
  } catch (error) {
    closeSync(descriptor)
    throw error
  }
  return {
    verify,
    close() {
      if (closed) return
      closed = true
      closeSync(descriptor)
    }
  }
}

function canonicalEnvironmentEntries(environment) {
  const entries = new Map()
  for (const [rawName, rawValue] of Object.entries(environment || {})) {
    const name = String(rawName).toUpperCase()
    if (entries.has(name)) {
      throw new Error(`[development-cache] Duplicate environment key is forbidden: ${name}`)
    }
    entries.set(name, { rawName, value: String(rawValue || '') })
  }
  return entries
}

// Development cache variables are orchestration hints, not compiler
// authority. Only the repository's exact mounted cache layout may be removed
// at a hermetic Go/Rust build boundary. Unknown or relabelled overrides remain
// visible to the existing fail-closed compiler contracts.
function projectDevelopmentCacheEnvironment(environment, removeNames) {
  const projected = { ...(environment || {}) }
  const entries = canonicalEnvironmentEntries(projected)
  const marker = entries.get(DEVELOPMENT_CACHE_MARKER)
  const requested = new Set((removeNames || []).map((name) => String(name).toUpperCase()))
  for (const name of requested) {
    if (!Object.hasOwn(DEVELOPMENT_CACHE_ENVIRONMENT, name)) {
      throw new Error(`[development-cache] Unsupported cache projection: ${name}`)
    }
  }
  if (!marker || marker.value === '') return projected
  if (marker.value !== DEVELOPMENT_CACHE_ROOT) {
    throw new Error('[development-cache] Development cache root is not authoritative')
  }
  for (const [name, expected] of Object.entries(DEVELOPMENT_CACHE_ENVIRONMENT)) {
    const candidate = entries.get(name)
    if (candidate && candidate.value !== '' && candidate.value !== expected) {
      throw new Error(`[development-cache] Development cache path mismatch: ${name}`)
    }
  }
  for (const name of requested) {
    const candidate = entries.get(name)
    if (!candidate || candidate.value === '') {
      throw new Error(`[development-cache] Development cache path is unavailable: ${name}`)
    }
    delete projected[candidate.rawName]
  }
  delete projected[marker.rawName]
  return projected
}

function projectGoDevelopmentCacheEnvironment(environment) {
  const entries = canonicalEnvironmentEntries(environment || {})
  const marker = entries.get(DEVELOPMENT_CACHE_MARKER)
  const projected = projectDevelopmentCacheEnvironment(
    environment,
    ['GOCACHE', 'GOMODCACHE', 'GOTMPDIR']
  )
  if (!marker || marker.value === '') {
    return { environment: projected, authorizedModuleCache: '' }
  }
  const moduleCache = entries.get('GOMODCACHE')
  if (!moduleCache || moduleCache.value !== DEVELOPMENT_CACHE_ENVIRONMENT.GOMODCACHE) {
    throw new Error('[development-cache] Authoritative Go module cache is unavailable')
  }
  return {
    environment: projected,
    authorizedModuleCache: DEVELOPMENT_CACHE_ENVIRONMENT.GOMODCACHE
  }
}

function requireDevelopmentNativeComponentRoot(environment) {
  const entries = canonicalEnvironmentEntries(environment || {})
  const marker = entries.get(DEVELOPMENT_CACHE_MARKER)
  if (!marker || marker.value !== DEVELOPMENT_CACHE_ROOT) {
    throw new Error('[development-cache] Development cache root is not authoritative')
  }
  return DEVELOPMENT_NATIVE_COMPONENT_ROOT
}

module.exports = {
  DEVELOPMENT_CACHE_APFS_VOLUME_TYPE,
  DEVELOPMENT_CACHE_BACKING_MOUNT,
  DEVELOPMENT_CACHE_BACKING_VOLUME_UUID,
  DEVELOPMENT_CACHE_DAMAGED_V2_IMAGE,
  DEVELOPMENT_CACHE_ENVIRONMENT,
  DEVELOPMENT_CACHE_IMAGE,
  DEVELOPMENT_CACHE_LEGACY_IMAGE,
  DEVELOPMENT_CACHE_MARKER,
  DEVELOPMENT_CACHE_MOUNT,
  DEVELOPMENT_NATIVE_COMPONENT_ROOT,
  DEVELOPMENT_CACHE_ROOT,
  DEVELOPMENT_CACHE_VOLUME_UUID,
  collectDevelopmentCacheVolumeSnapshot,
  openVerifiedDevelopmentCacheVolume,
  projectDevelopmentCacheEnvironment,
  projectGoDevelopmentCacheEnvironment,
  requireDevelopmentNativeComponentRoot,
  validateDevelopmentCacheVolumeSnapshot,
  verifyDevelopmentCacheVolume
}
