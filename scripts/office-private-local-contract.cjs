'use strict'

// This content qualification is not package identity or release permission.
// Callers must additionally verify packaged authority and the resource seal.
const { createHash } = require('node:crypto')
const fs = require('node:fs')
const { dirname, isAbsolute, join, relative, resolve, sep } = require('node:path')
const { parseStrictJsonObject } = require('./lib/strict-json.cjs')

const CONTRACT = 'analytix.office-private-local/v1'
const DOMAIN = 'AnalytixOfficePrivateLocalV1\0'
const layout = require('../packages/runtime-go/internal/adapters/outbound/officeengineassets/private-local-layout.json')
const BUILD_ID = layout.buildId
const MANIFEST_SHA256 = layout.manifestSha256
const SHA256 = /^[a-f0-9]{64}$/u
const COMMIT = /^[a-f0-9]{40}$/u
const MAX_QUALIFICATION = 64 * 1024
const MAX_TOTAL = 320 * 1024 * 1024
const ROOT_KEYS = ['schemaVersion', 'contract', 'usage', 'publishable', 'releaseEligible',
  'targetKey', 'sourceCommit', 'worktreeSnapshotDigest', 'engineBuildId',
  'engineManifestSha256', 'files', 'qualificationDigest']
const PLUGINS = ['documents', 'spreadsheets', 'presentations']
function pluginFiles(kind) {
  return ['.analytix-plugin/package.json', '.codex-plugin/plugin.json', 'assets/adapter.json',
    'ui/editor.json', `skills/${kind}/SKILL.md`]
}
const fail = code => { throw new Error(`office-private-local-${code}`) }
const digest = value => createHash('sha256').update(DOMAIN).update(JSON.stringify(value)).digest('hex')
const exact = (value, keys) => value && typeof value === 'object' && !Array.isArray(value) &&
  Object.keys(value).length === keys.length && keys.every(key => Object.hasOwn(value, key))

// Source-owned layout is shared with Go and Main. No caller-supplied pins.
const safeRelative = value => typeof value === 'string' && /^[A-Za-z0-9._/-]+$/u.test(value) &&
  value.split('/').every(part => part !== '' && part !== '.' && part !== '..')
if (!exact(layout, ['schemaVersion', 'contract', 'buildId', 'manifestSha256', 'files']) ||
    layout.schemaVersion !== 1 || layout.contract !== 'analytix.office-private-local-layout/v1' ||
    typeof BUILD_ID !== 'string' || !COMMIT.test(BUILD_ID) || typeof MANIFEST_SHA256 !== 'string' || !SHA256.test(MANIFEST_SHA256) ||
    !Array.isArray(layout.files) || layout.files.length !== 35) fail('invalid-source-layout')
const sources = new Map()
let previous = ''
for (const file of layout.files) {
  if (!exact(file, ['path', 'source', 'owner', 'maximum', 'byteLength', 'sha256']) ||
      !safeRelative(file.path) || !safeRelative(file.source) || file.path <= previous || file.path === 'qualification.json' ||
      !['repo', 'asset'].includes(file.owner) || !Number.isSafeInteger(file.maximum) || file.maximum <= 0 || file.maximum > MAX_TOTAL ||
      (file.byteLength !== null && (!Number.isSafeInteger(file.byteLength) || file.byteLength <= 0 || file.byteLength > file.maximum)) ||
      (file.sha256 !== null && (typeof file.sha256 !== 'string' || !SHA256.test(file.sha256))) ||
      (file.byteLength === null) !== (file.sha256 === null) || (file.owner === 'asset' && file.sha256 === null)) fail('invalid-source-layout')
  sources.set(file.path, Object.freeze({ ...file }))
  previous = file.path
}
if (sources.get('manifest.json')?.sha256 !== MANIFEST_SHA256 || sources.get('manifest.json')?.owner !== 'repo') fail('invalid-source-layout')
const FILES = Object.freeze([...sources.keys()])

function canonicalBody(value) {
  return {
    schemaVersion: 1, contract: CONTRACT, usage: 'private-local', publishable: false,
    releaseEligible: false, targetKey: value.targetKey, sourceCommit: value.sourceCommit,
    worktreeSnapshotDigest: value.worktreeSnapshotDigest, engineBuildId: BUILD_ID,
    engineManifestSha256: MANIFEST_SHA256,
    files: value.files.map(file => ({ path: file.path, byteLength: file.byteLength, sha256: file.sha256 }))
  }
}

/** Pure parser: no filesystem, environment, package identity or permission grant. */
function validateQualification(bytes) {
  let value
  try { value = parseStrictJsonObject(bytes, { maxBytes: MAX_QUALIFICATION, integerOnly: true, maxTokens: 4096 }) }
  catch { fail('invalid-qualification') }
  if (!exact(value, ROOT_KEYS) || value.schemaVersion !== 1 || value.contract !== CONTRACT ||
      value.usage !== 'private-local' || value.publishable !== false || value.releaseEligible !== false ||
      value.targetKey !== 'darwin-arm64' || typeof value.sourceCommit !== 'string' || !COMMIT.test(value.sourceCommit) ||
      typeof value.worktreeSnapshotDigest !== 'string' || !SHA256.test(value.worktreeSnapshotDigest) || value.engineBuildId !== BUILD_ID ||
      value.engineManifestSha256 !== MANIFEST_SHA256 || typeof value.qualificationDigest !== 'string' || !SHA256.test(value.qualificationDigest) ||
      !Array.isArray(value.files) || value.files.length !== FILES.length) fail('invalid-qualification')
  let total = 0
  for (let index = 0; index < FILES.length; index++) {
    const file = value.files[index], expected = sources.get(FILES[index])
    if (!exact(file, ['path', 'byteLength', 'sha256']) || file.path !== FILES[index] ||
        !Number.isSafeInteger(file.byteLength) || file.byteLength <= 0 || file.byteLength > expected.maximum ||
        typeof file.sha256 !== 'string' || !SHA256.test(file.sha256) || (expected.sha256 && file.sha256 !== expected.sha256) ||
        (expected.byteLength != null && file.byteLength !== expected.byteLength)) fail('invalid-file-binding')
    total += file.byteLength
  }
  if (total > MAX_TOTAL) fail('invalid-file-binding')
  const body = canonicalBody(value)
  if (digest(body) !== value.qualificationDigest) fail('invalid-digest')
  const result = { ...body, qualificationDigest: value.qualificationDigest }
  if (!bytes.equals(Buffer.from(JSON.stringify(result) + '\n'))) fail('noncanonical-qualification')
  result.files.forEach(Object.freeze)
  Object.freeze(result.files)
  return Object.freeze(result)
}

function directoryChain(root) {
  if (typeof root !== 'string' || !isAbsolute(root) || resolve(root) !== root || fs.realpathSync(root) !== root) fail('unsafe-directory')
  const chain = []
  let current = root
  for (;;) {
    const info = fs.lstatSync(current, { bigint: true })
    if (!info.isDirectory() || info.isSymbolicLink()) fail('unsafe-directory')
    chain.push({ path: current, info })
    const parent = dirname(current)
    if (parent === current) break
    current = parent
  }
  return chain
}
function checkChain(chain) {
  for (const entry of chain) {
    const info = fs.lstatSync(entry.path, { bigint: true })
    if (!info.isDirectory() || info.isSymbolicLink() || info.dev !== entry.info.dev ||
        info.ino !== entry.info.ino || info.mode !== entry.info.mode || info.uid !== entry.info.uid ||
        info.gid !== entry.info.gid) fail('directory-changed')
  }
}
function safeFile(info, maximum) {
  return info.isFile() && !info.isSymbolicLink() && info.nlink === 1n && info.size > 0n &&
    info.size <= BigInt(maximum) && (info.mode & 0o022n) === 0n
}
function sameFile(a, b) {
  return ['dev', 'ino', 'mode', 'uid', 'gid', 'size', 'mtimeNs', 'ctimeNs', 'nlink'].every(key => a[key] === b[key])
}

// Each stream is sampled through its held descriptor and checked against the
// pathname and every ancestor before and after reading. No source is modified.
function readStable(path, maximum, consume) {
  const chain = directoryChain(dirname(path))
  const before = fs.lstatSync(path, { bigint: true })
  if (!safeFile(before, maximum)) fail('unsafe-file')
  const descriptor = fs.openSync(path, fs.constants.O_RDONLY | (fs.constants.O_NOFOLLOW || 0))
  try {
    const opened = fs.fstatSync(descriptor, { bigint: true })
    if (!sameFile(before, opened)) fail('source-changed')
    const digest = createHash('sha256'), buffer = Buffer.alloc(1024 * 1024)
    let byteLength = 0
    for (;;) {
      const count = fs.readSync(descriptor, buffer, 0, buffer.length, null)
      if (!count) break
      byteLength += count
      if (byteLength > maximum) fail('file-too-large')
      const chunk = buffer.subarray(0, count)
      digest.update(chunk)
      if (consume) consume(chunk)
    }
    const after = fs.fstatSync(descriptor, { bigint: true }), pathAfter = fs.lstatSync(path, { bigint: true })
    if (!sameFile(before, after) || !sameFile(after, pathAfter) || byteLength !== Number(before.size)) fail('source-changed')
    checkChain(chain)
    return { byteLength, sha256: digest.digest('hex'), info: after }
  } finally { fs.closeSync(descriptor) }
}
function readBytes(path, maximum) {
  const chunks = []
  const identity = readStable(path, maximum, chunk => chunks.push(Buffer.from(chunk)))
  return { ...identity, bytes: Buffer.concat(chunks) }
}
function directoriesFor(files) {
  const directories = new Set()
  for (const file of files) for (let parent = dirname(file); parent !== '.'; parent = dirname(parent)) directories.add(parent)
  return directories
}
function inspectClosedTree(root, files) {
  const chain = directoryChain(root), expected = new Set(files), directories = directoriesFor(files), seen = new Set()
  const visit = (path, prefix) => {
    for (const name of fs.readdirSync(path)) {
      const rel = prefix ? `${prefix}/${name}` : name, absolute = join(path, name)
      const info = fs.lstatSync(absolute, { bigint: true })
      if (info.isSymbolicLink()) fail('unsafe-tree')
      if (directories.has(rel) && info.isDirectory()) visit(absolute, rel)
      else if (expected.has(rel) && safeFile(info, MAX_TOTAL)) seen.add(rel)
      else fail('unexpected-payload')
    }
  }
  visit(root, '')
  if (seen.size !== expected.size) fail('incomplete-tree')
  checkChain(chain)
}

function verifyPlugin(root, kind) {
  inspectClosedTree(root, pluginFiles(kind))
  const parse = file => parseStrictJsonObject(readBytes(join(root, file), 1024 * 1024).bytes,
    { maxBytes: 1024 * 1024, maxTokens: 8192 })
  const declaration = parse('.analytix-plugin/package.json'), codex = parse('.codex-plugin/plugin.json')
  const contributions = declaration.contributions, id = `analytix-${kind}`
  if (declaration.schemaVersion !== 1 || declaration.packageId !== id || codex.name !== id ||
      declaration.packageVersion !== '0.2.0' || codex.version !== declaration.packageVersion ||
      !exact(contributions, ['skills', 'mcpServers', 'hooks', 'assets', 'publicUi']) ||
      JSON.stringify(contributions.skills) !== JSON.stringify([{ id: kind, path: `skills/${kind}/SKILL.md` }]) ||
      JSON.stringify(contributions.assets) !== JSON.stringify([{ id: 'editor-adapter', path: 'assets/adapter.json' }]) ||
      JSON.stringify(contributions.publicUi) !== JSON.stringify([{ id: 'workspace-editor', path: 'ui/editor.json' }]) ||
      !Array.isArray(contributions.mcpServers) || contributions.mcpServers.length !== 0 ||
      !Array.isArray(contributions.hooks) || contributions.hooks.length !== 0) fail('invalid-plugin-closure')
  // Full declaration/capability admission remains owned by the Core host.
}

/** Content-only verification. The caller must verify the existing package seal. */
function verifyPrivateLocal(root, expected) {
  try {
    if (!exact(expected, ['sourceCommit', 'worktreeSnapshotDigest', 'targetKey']) ||
        typeof expected.sourceCommit !== 'string' || !COMMIT.test(expected.sourceCommit) ||
        typeof expected.worktreeSnapshotDigest !== 'string' || !SHA256.test(expected.worktreeSnapshotDigest) ||
        expected.targetKey !== 'darwin-arm64') fail('invalid-expected-identity')
    const chain = directoryChain(root)
    const initial = readBytes(join(root, 'qualification.json'), MAX_QUALIFICATION)
    const qualification = validateQualification(initial.bytes)
    if (qualification.sourceCommit !== expected.sourceCommit || qualification.worktreeSnapshotDigest !== expected.worktreeSnapshotDigest ||
        qualification.targetKey !== expected.targetKey) fail('identity-mismatch')
    inspectClosedTree(root, [...FILES, 'qualification.json'])
    const observations = []
    for (const file of qualification.files) {
      const actual = readStable(join(root, file.path), sources.get(file.path).maximum)
      if (actual.byteLength !== file.byteLength || actual.sha256 !== file.sha256) fail('content-mismatch')
      observations.push({ path: join(root, file.path), info: actual.info })
    }
    for (const kind of PLUGINS) verifyPlugin(join(root, `plugins/analytix-${kind}`), kind)
    const final = readBytes(join(root, 'qualification.json'), MAX_QUALIFICATION)
    if (!sameFile(initial.info, final.info) || !initial.bytes.equals(final.bytes)) fail('qualification-changed')
    inspectClosedTree(root, [...FILES, 'qualification.json'])
    for (const observation of observations) {
      if (!sameFile(observation.info, fs.lstatSync(observation.path, { bigint: true }))) fail('content-changed')
    }
    checkChain(chain)
    return qualification
  } catch (error) {
    if (error.message.startsWith('office-private-local-')) throw error
    fail('verification-failed')
  }
}

function writeNew(path, producer) {
  const chain = directoryChain(dirname(path))
  const descriptor = fs.openSync(path, fs.constants.O_WRONLY | fs.constants.O_CREAT |
    fs.constants.O_EXCL | (fs.constants.O_NOFOLLOW || 0), 0o644)
  try {
    const opened = fs.fstatSync(descriptor, { bigint: true })
    const consume = chunk => {
      let offset = 0
      while (offset < chunk.length) {
        const written = fs.writeSync(descriptor, chunk, offset, chunk.length - offset)
        if (written <= 0) fail('write-failed')
        offset += written
      }
    }
    producer(consume)
    fs.fsyncSync(descriptor)
    const held = fs.fstatSync(descriptor, { bigint: true }), after = fs.lstatSync(path, { bigint: true })
    if (!safeFile(held, MAX_TOTAL) || opened.dev !== held.dev || opened.ino !== held.ino || !sameFile(held, after)) fail('destination-changed')
    checkChain(chain)
  } finally { fs.closeSync(descriptor) }
}
function overlaps(a, b) {
  const rel = relative(a, b)
  return rel === '' || (!rel.startsWith(`..${sep}`) && rel !== '..' && !isAbsolute(rel))
}

/** Stages an absent directory; failed partial candidates are retained, never deleted. */
function stagePrivateLocal({ repoRoot, assetRoot, destination, worktreeSnapshot, targetKey = 'darwin-arm64' }) {
  try {
    if (!worktreeSnapshot || typeof worktreeSnapshot.sourceCommit !== 'string' || !COMMIT.test(worktreeSnapshot.sourceCommit) ||
        typeof worktreeSnapshot.snapshotDigest !== 'string' || !SHA256.test(worktreeSnapshot.snapshotDigest) ||
        targetKey !== 'darwin-arm64' || typeof destination !== 'string' || !isAbsolute(destination) || resolve(destination) !== destination) fail('invalid-stage-input')
    const repoChain = directoryChain(repoRoot), assetChain = directoryChain(assetRoot), parentChain = directoryChain(dirname(destination))
    if (overlaps(repoRoot, destination) || overlaps(destination, repoRoot) ||
        overlaps(assetRoot, destination) || overlaps(destination, assetRoot)) fail('overlapping-roots')
    try { fs.lstatSync(destination); fail('destination-exists') }
    catch (error) { if (error.code !== 'ENOENT') throw error }
    for (const kind of PLUGINS) verifyPlugin(join(repoRoot, `plugins/analytix-${kind}`), kind)
    const observed = new Map()
    for (const path of FILES) {
      const source = sources.get(path), root = source.owner === 'repo' ? repoRoot : assetRoot
      const actual = readStable(join(root, source.source), source.maximum)
      if ((source.sha256 && source.sha256 !== actual.sha256) ||
          (source.byteLength != null && source.byteLength !== actual.byteLength)) fail('fixed-source-mismatch')
      observed.set(path, { root, ...actual })
    }
    const body = canonicalBody({ targetKey, sourceCommit: worktreeSnapshot.sourceCommit,
      worktreeSnapshotDigest: worktreeSnapshot.snapshotDigest,
      files: FILES.map(path => ({ path, byteLength: observed.get(path).byteLength, sha256: observed.get(path).sha256 })) })
    const bytes = Buffer.from(JSON.stringify({ ...body, qualificationDigest: digest(body) }) + '\n')
    validateQualification(bytes)
    checkChain(repoChain); checkChain(assetChain); checkChain(parentChain)
    fs.mkdirSync(destination, { mode: 0o755 })
    const destinationChain = directoryChain(destination)
    for (const dir of [...directoriesFor(FILES)].sort()) {
      checkChain(destinationChain)
      fs.mkdirSync(join(destination, dir), { mode: 0o755 })
    }
    for (const path of FILES) {
      const source = sources.get(path), before = observed.get(path)
      checkChain(destinationChain)
      writeNew(join(destination, path), consume => {
        const actual = readStable(join(before.root, source.source), source.maximum, consume)
        if (!sameFile(before.info, actual.info) || before.sha256 !== actual.sha256 || before.byteLength !== actual.byteLength) fail('source-changed')
      })
    }
    checkChain(repoChain); checkChain(assetChain); checkChain(parentChain); checkChain(destinationChain)
    writeNew(join(destination, 'qualification.json'), consume => consume(bytes))
    const qualification = verifyPrivateLocal(destination, { sourceCommit: body.sourceCommit,
      worktreeSnapshotDigest: body.worktreeSnapshotDigest, targetKey })
    checkChain(destinationChain)
    return { root: destination, qualification }
  } catch (error) {
    if (error.message.startsWith('office-private-local-')) throw error
    fail('staging-failed')
  }
}

module.exports = { validateQualification, verifyPrivateLocal, stagePrivateLocal }
