const { createHash } = require('node:crypto')
const {
  chmodSync,
  closeSync,
  constants,
  copyFileSync,
  existsSync,
  fstatSync,
  fsyncSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readSync,
  readFileSync,
  readdirSync,
  realpathSync,
  renameSync,
  rmSync
} = require('node:fs')
const { basename, dirname, isAbsolute, join, posix, relative, resolve, sep } = require('node:path')
const { parseStrictJsonObject } = require('./lib/strict-json.cjs')
const {
  assertNativeBinaryTarget,
  targetContract
} = require('./native-component-contract.cjs')

const MANIFEST_FILE_NAME = 'analytix-document-runtime-manifest.json'
const MANIFEST_CONTRACT = 'analytix.document-runtime/v2'
const AUTHORITY_LOCK_FILE_NAME = 'analytix-document-runtime-authority-lock.json'
const AUTHORITY_LOCK_CONTRACT = 'analytix.document-runtime-authority-lock/v1'
const AUTHORITY_LOCK_PATH = join(__dirname, 'document-runtime-authority-lock.json')
const FROZEN_AUTHORITY_LOCK_SHA256 = 'd1fa4ec449edbef8d310a2fdfb8bc236609aacf75b9dd6cb295c4b374d0226f3'
const ASSET_ROOT_ENV = 'ANALYTIX_DOCUMENT_RUNTIME_ASSET_ROOT'
const UNAVAILABLE = 'document_runtime_asset_authority_unavailable'
const TREE_DOMAIN = 'AnalytixDocumentRuntimeTreeV2\0'
const SOURCE_SET_DOMAIN = 'AnalytixDocumentRuntimeSourceSetV1\0'
const LICENSE_SET_DOMAIN = 'AnalytixDocumentRuntimeLicenseSetV1\0'
const MAX_MANIFEST_BYTES = 1024 * 1024
const MAX_FILE_BYTES = 2 * 1024 * 1024 * 1024
const MAX_NATIVE_INSPECTION_BYTES = 256 * 1024 * 1024
const ROOT_KEYS = [
  'schemaVersion', 'contract', 'target', 'sources', 'licenses', 'capabilities',
  'files', 'treeSha256'
]
const TARGET_KEYS = ['key', 'platform', 'arch', 'triple']
const SOURCE_KEYS = ['id', 'name', 'version', 'sourceUri', 'archiveSha256']
const LICENSE_KEYS = ['id', 'spdxExpression', 'sourceId', 'path']
const CAPABILITY_KEYS = ['sofficeBinary', 'tesseractBinary', 'tessdataDirectory', 'ocrLanguages']
const FILE_KEYS = [
  'path', 'kind', 'sourceId', 'licenseId', 'sha256', 'byteLength',
  'platformSignature', 'dependencies'
]
const CONTENT_SIGNATURE_KEYS = ['kind', 'targetKey']
const NATIVE_SIGNATURE_KEYS = [
  'kind', 'targetKey', 'format', 'arch', 'payloadSha256', 'payloadByteLength'
]
const FILE_KINDS = new Set(['binary', 'library', 'data', 'license'])
const LOCK_ROOT_KEYS = ['schemaVersion', 'contract', 'targets']
const LOCK_TARGET_KEYS = [
  'targetKey', 'assetReleaseId', 'manifestSha256', 'treeSha256',
  'sourceSetSha256', 'licenseSetSha256'
]
const SHA256_RE = /^[0-9a-f]{64}$/u
const ID_RE = /^[a-z0-9][a-z0-9._-]{0,127}$/u
const SPDX_RE = /^[A-Za-z0-9][A-Za-z0-9.+() -]{0,127}$/u

function exactKeys(value, expected) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const keys = Object.keys(value)
  return keys.length === expected.length && keys.every((key) => expected.includes(key))
}

function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}

function canonicalJSON(value) {
  return JSON.stringify(value)
}

function sameIdentity(left, right) {
  return left.sha256 === right.sha256 && left.byteLength === right.byteLength
}

function hashStableRegularFile(path, maximumBytes = MAX_FILE_BYTES) {
  const before = lstatSync(path)
  if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1 || before.size <= 0 ||
    before.size > maximumBytes || (before.mode & 0o022) !== 0) {
    throw new Error(`[document-runtime] Untrusted regular file: ${path}`)
  }
  const descriptor = openSync(path, constants.O_RDONLY | (constants.O_NOFOLLOW || 0))
  const digest = createHash('sha256')
  let byteLength = 0
  try {
    const opened = fstatSync(descriptor)
    if (!opened.isFile() || opened.dev !== before.dev || opened.ino !== before.ino ||
      opened.size !== before.size) throw new Error(`[document-runtime] File identity changed before hashing: ${path}`)
    const buffer = Buffer.allocUnsafe(1024 * 1024)
    while (true) {
      const count = readSync(descriptor, buffer, 0, buffer.length, null)
      if (count === 0) break
      byteLength += count
      if (byteLength > maximumBytes) throw new Error(`[document-runtime] File exceeds its bound: ${path}`)
      digest.update(buffer.subarray(0, count))
    }
    const afterOpened = fstatSync(descriptor)
    const after = lstatSync(path)
    if (after.dev !== before.dev || after.ino !== before.ino || after.size !== before.size ||
      after.mtimeMs !== before.mtimeMs || afterOpened.size !== before.size || byteLength !== before.size) {
      throw new Error(`[document-runtime] File changed during identity sampling: ${path}`)
    }
  } finally {
    closeSync(descriptor)
  }
  return { sha256: digest.digest('hex'), byteLength }
}

function readBoundedStableFile(path, maximumBytes) {
  const before = hashStableRegularFile(path, maximumBytes)
  const bytes = readFileSync(path)
  const after = hashStableRegularFile(path, maximumBytes)
  if (!sameIdentity(before, after) || bytes.length !== before.byteLength || sha256(bytes) !== before.sha256) {
    throw new Error(`[document-runtime] File changed around bounded read: ${path}`)
  }
  return bytes
}

function normalizedRelativePath(value) {
  if (typeof value !== 'string' || !value || value.includes('\\') || value.includes('\0') ||
    isAbsolute(value) || posix.isAbsolute(value) || posix.normalize(value) !== value ||
    value === '.' || value.startsWith('../') || value.includes('/../') || value.endsWith('/')) {
    return ''
  }
  return value
}

function pathWithinRoot(root, path) {
  const rel = relative(resolve(root), resolve(path))
  return rel !== '..' && !rel.startsWith(`..${sep}`) && !isAbsolute(rel)
}

function assertNoSymlinkPath(root, path, finalKind = 'file') {
  if (!pathWithinRoot(root, path)) throw new Error('[document-runtime] Asset path escaped its root')
  const rootStat = lstatSync(root)
  if (!rootStat.isDirectory() || rootStat.isSymbolicLink()) {
    throw new Error('[document-runtime] Asset root is untrusted')
  }
  let current = resolve(root)
  const rel = relative(current, resolve(path))
  for (const part of rel.split(sep).filter(Boolean)) {
    current = join(current, part)
    const stat = lstatSync(current)
    if (stat.isSymbolicLink()) throw new Error('[document-runtime] Asset path traverses a symbolic link')
  }
  const final = lstatSync(path)
  if (finalKind === 'file' && (!final.isFile() || final.nlink !== 1)) {
    throw new Error('[document-runtime] Asset file is invalid')
  }
  if (finalKind === 'directory' && !final.isDirectory()) {
    throw new Error('[document-runtime] Asset directory is invalid')
  }
}

function validateTarget(value, expected) {
  return exactKeys(value, TARGET_KEYS) && value.key === expected.key &&
    value.platform === expected.platform && value.arch === expected.arch &&
    value.triple === expected.triple
}

function validateSource(value) {
  if (!exactKeys(value, SOURCE_KEYS) || !ID_RE.test(value.id || '') ||
    typeof value.name !== 'string' || !value.name.trim() ||
    typeof value.version !== 'string' || !value.version.trim() ||
    !SHA256_RE.test(value.archiveSha256 || '')) return false
  try {
    const uri = new URL(value.sourceUri)
    return ['https:', 'git+https:'].includes(uri.protocol) && !uri.username && !uri.password
  } catch {
    return false
  }
}

function validateLicense(value) {
  return exactKeys(value, LICENSE_KEYS) && ID_RE.test(value.id || '') &&
    SPDX_RE.test(value.spdxExpression || '') && ID_RE.test(value.sourceId || '') &&
    Boolean(normalizedRelativePath(value.path))
}

function validatePlatformSignature(value, entry, target) {
  if (entry.kind === 'binary' || entry.kind === 'library') {
    return exactKeys(value, NATIVE_SIGNATURE_KEYS) && value.kind === 'native' &&
      value.targetKey === target.key && value.format === target.format &&
      value.arch === target.arch && SHA256_RE.test(value.payloadSha256 || '') &&
      Number.isSafeInteger(value.payloadByteLength) && value.payloadByteLength > 0 &&
      value.payloadByteLength <= entry.byteLength
  }
  return exactKeys(value, CONTENT_SIGNATURE_KEYS) && value.kind === 'content' &&
    value.targetKey === target.key
}

function validateFileEntry(value, target) {
  return exactKeys(value, FILE_KEYS) && Boolean(normalizedRelativePath(value.path)) &&
    FILE_KINDS.has(value.kind) && ID_RE.test(value.sourceId || '') &&
    ID_RE.test(value.licenseId || '') && SHA256_RE.test(value.sha256 || '') &&
    Number.isSafeInteger(value.byteLength) && value.byteLength > 0 &&
    validatePlatformSignature(value.platformSignature, value, target) &&
    Array.isArray(value.dependencies) &&
    value.dependencies.every((path) => Boolean(normalizedRelativePath(path))) &&
    canonicalJSON([...value.dependencies].sort()) === canonicalJSON(value.dependencies) &&
    new Set(value.dependencies).size === value.dependencies.length &&
    ((value.kind === 'binary' || value.kind === 'library') || value.dependencies.length === 0)
}

function treeDigest(value) {
  return sha256(Buffer.concat([
    Buffer.from(TREE_DOMAIN, 'utf8'),
    Buffer.from(canonicalJSON({
      target: value.target,
      sources: value.sources,
      licenses: value.licenses,
      capabilities: value.capabilities,
      files: value.files
    }), 'utf8')
  ]))
}

function setDigest(domain, value) {
  return sha256(Buffer.concat([
    Buffer.from(domain, 'utf8'),
    Buffer.from(canonicalJSON(value), 'utf8')
  ]))
}

function createAuthorityLockEntry(manifest, manifestSha256, assetReleaseId = 'test-authorized-assets') {
  return {
    targetKey: manifest.target.key,
    assetReleaseId,
    manifestSha256,
    treeSha256: manifest.treeSha256,
    sourceSetSha256: setDigest(SOURCE_SET_DOMAIN, manifest.sources),
    licenseSetSha256: setDigest(LICENSE_SET_DOMAIN, manifest.licenses)
  }
}

function validAuthorityLock(value) {
  if (!exactKeys(value, LOCK_ROOT_KEYS) || value.schemaVersion !== 1 ||
    value.contract !== AUTHORITY_LOCK_CONTRACT || !Array.isArray(value.targets) ||
    canonicalJSON([...value.targets].sort((a, b) => a.targetKey.localeCompare(b.targetKey))) !==
      canonicalJSON(value.targets)) return false
  const seen = new Set()
  for (const target of value.targets) {
    if (!exactKeys(target, LOCK_TARGET_KEYS) || typeof target.targetKey !== 'string' ||
      !ID_RE.test(target.assetReleaseId || '') || !SHA256_RE.test(target.manifestSha256 || '') ||
      !SHA256_RE.test(target.treeSha256 || '') || !SHA256_RE.test(target.sourceSetSha256 || '') ||
      !SHA256_RE.test(target.licenseSetSha256 || '') || seen.has(target.targetKey)) return false
    seen.add(target.targetKey)
  }
  return true
}

function readAuthorityLock(options = {}) {
  const lockPath = options.lockPath || AUTHORITY_LOCK_PATH
  const identity = hashStableRegularFile(lockPath, MAX_MANIFEST_BYTES)
  const bytes = readFileSync(lockPath)
  if (sha256(bytes) !== identity.sha256 || bytes.length !== identity.byteLength) {
    throw new Error('[document-runtime] Authority lock changed around bounded read')
  }
  if (options.authorityLock !== undefined) {
    if (process.env.NODE_ENV !== 'test' || !validAuthorityLock(options.authorityLock)) {
      throw new Error('[document-runtime] Authority lock override is test-only or invalid')
    }
    if (canonicalJSON(options.authorityLock) !== bytes.toString('utf8')) {
      throw new Error('[document-runtime] Test authority lock file mismatch')
    }
    return options.authorityLock
  }
  if (identity.sha256 !== FROZEN_AUTHORITY_LOCK_SHA256) {
    throw new Error(`[document-runtime] ${UNAVAILABLE}: frozen authority lock identity is invalid`)
  }
  let value
  try {
    value = parseStrictJsonObject(bytes, {
      maxBytes: MAX_MANIFEST_BYTES,
      maxDepth: 4,
      maxTokens: 4096,
      maxStringBytes: 4096,
      maxNumberBytes: 32,
      integerOnly: true
    })
  } catch {
    throw new Error(`[document-runtime] ${UNAVAILABLE}: frozen authority lock JSON is invalid`)
  }
  if (!validAuthorityLock(value) || canonicalJSON(value) !== bytes.toString('utf8')) {
    throw new Error(`[document-runtime] ${UNAVAILABLE}: frozen authority lock schema is invalid`)
  }
  return value
}

function requireAuthorityLock(record, target, root, options = {}) {
  const lock = readAuthorityLock({
    ...options,
    lockPath: join(root, AUTHORITY_LOCK_FILE_NAME)
  })
  const expected = createAuthorityLockEntry(
    record.manifest,
    record.manifestSha256,
    lock.targets.find((entry) => entry.targetKey === target.key)?.assetReleaseId ||
      'missing-authority'
  )
  const authorized = lock.targets.find((entry) => entry.targetKey === target.key)
  if (!authorized || canonicalJSON(authorized) !== canonicalJSON(expected)) {
    throw new Error(`[document-runtime] ${UNAVAILABLE}`)
  }
  return { lock, authority: authorized }
}

function validateManifestShape(value, target) {
  if (!exactKeys(value, ROOT_KEYS) || value.schemaVersion !== 2 ||
    value.contract !== MANIFEST_CONTRACT || !validateTarget(value.target, target) ||
    !Array.isArray(value.sources) || value.sources.length === 0 ||
    !Array.isArray(value.licenses) || value.licenses.length === 0 ||
    !exactKeys(value.capabilities, CAPABILITY_KEYS) ||
    !Array.isArray(value.files) || value.files.length < 5 ||
    !SHA256_RE.test(value.treeSha256 || '')) return false
  if (!value.sources.every(validateSource) || !value.licenses.every(validateLicense) ||
    !value.files.every((entry) => validateFileEntry(entry, target))) return false
  const sortedSources = [...value.sources].sort((a, b) => a.id.localeCompare(b.id))
  const sortedLicenses = [...value.licenses].sort((a, b) => a.id.localeCompare(b.id))
  const sortedFiles = [...value.files].sort((a, b) => a.path.localeCompare(b.path))
  if (canonicalJSON(sortedSources) !== canonicalJSON(value.sources) ||
    canonicalJSON(sortedLicenses) !== canonicalJSON(value.licenses) ||
    canonicalJSON(sortedFiles) !== canonicalJSON(value.files)) return false
  const sourceIds = new Set(value.sources.map((item) => item.id))
  const licenseIds = new Set(value.licenses.map((item) => item.id))
  const filePaths = new Set()
  for (const entry of value.files) {
    const folded = entry.path.toLowerCase()
    if (filePaths.has(folded) || !sourceIds.has(entry.sourceId) || !licenseIds.has(entry.licenseId)) return false
    filePaths.add(folded)
  }
  const licenseFiles = new Map(value.files.filter((entry) => entry.kind === 'license').map((entry) => [entry.path, entry]))
  if (value.licenses.some((license) => !sourceIds.has(license.sourceId) ||
    !licenseFiles.has(license.path) || licenseFiles.get(license.path).sourceId !== license.sourceId)) return false
  const capabilities = value.capabilities
  if (!normalizedRelativePath(capabilities.sofficeBinary) ||
    !normalizedRelativePath(capabilities.tesseractBinary) ||
    !normalizedRelativePath(capabilities.tessdataDirectory) ||
    !Array.isArray(capabilities.ocrLanguages) || capabilities.ocrLanguages.length < 2 ||
    capabilities.ocrLanguages.some((language) => !/^[A-Za-z0-9_.+-]{1,64}$/u.test(language)) ||
    canonicalJSON([...capabilities.ocrLanguages].sort()) !== canonicalJSON(capabilities.ocrLanguages) ||
    new Set(capabilities.ocrLanguages).size !== capabilities.ocrLanguages.length ||
    !capabilities.ocrLanguages.includes('chi_sim') || !capabilities.ocrLanguages.includes('eng')) return false
  const byPath = new Map(value.files.map((entry) => [entry.path, entry]))
  if (byPath.get(capabilities.sofficeBinary)?.kind !== 'binary' ||
    byPath.get(capabilities.tesseractBinary)?.kind !== 'binary') return false
  for (const language of capabilities.ocrLanguages) {
    if (byPath.get(`${capabilities.tessdataDirectory}/${language}.traineddata`)?.kind !== 'data') return false
  }
  for (const entry of value.files) {
    if (entry.dependencies.some((path) => byPath.get(path)?.kind !== 'library')) return false
  }
  const reachableLibraries = new Set()
  const queue = [capabilities.sofficeBinary, capabilities.tesseractBinary]
  while (queue.length) {
    const current = byPath.get(queue.pop())
    if (!current) return false
    for (const dependency of current.dependencies) {
      if (!reachableLibraries.has(dependency)) {
        reachableLibraries.add(dependency)
        queue.push(dependency)
      }
    }
  }
  if (value.files.some((entry) => entry.kind === 'library' && !reachableLibraries.has(entry.path))) return false
  return value.treeSha256 === treeDigest(value)
}

function parseManifest(path, target) {
  const stable = hashStableRegularFile(path, MAX_MANIFEST_BYTES)
  const bytes = readFileSync(path)
  const stableAfterRead = hashStableRegularFile(path, MAX_MANIFEST_BYTES)
  if (!sameIdentity(stable, stableAfterRead) || bytes.length !== stable.byteLength ||
    sha256(bytes) !== stable.sha256) {
    throw new Error('[document-runtime] Manifest changed around bounded read')
  }
  let value
  try {
    value = parseStrictJsonObject(bytes, {
      maxBytes: MAX_MANIFEST_BYTES,
      maxDepth: 8,
      maxTokens: 65536,
      maxStringBytes: 16384,
      maxNumberBytes: 32,
      integerOnly: true
    })
  } catch {
    throw new Error('[document-runtime] Manifest JSON is invalid or contains duplicate keys')
  }
  if (!validateManifestShape(value, target) || canonicalJSON(value) !== bytes.toString('utf8')) {
    throw new Error('[document-runtime] Manifest schema or canonical encoding is invalid')
  }
  return { manifest: value, manifestSha256: stable.sha256, manifestByteLength: stable.byteLength }
}

function nativeSignature(path, target) {
  const stat = lstatSync(path)
  if (stat.size > MAX_NATIVE_INSPECTION_BYTES) {
    throw new Error('[document-runtime] Native file exceeds the bounded target-inspection limit')
  }
  const first = assertNativeBinaryTarget(path, target)
  const second = assertNativeBinaryTarget(path, target)
  if (canonicalJSON(first) !== canonicalJSON(second)) {
    throw new Error('[document-runtime] Native file changed between target samples')
  }
  return {
    kind: 'native',
    targetKey: target.key,
    format: first.format,
    arch: first.arch,
    payloadSha256: first.payloadSha256,
    payloadByteLength: first.payloadSize
  }
}

const MACH_O_DYLIB_COMMANDS = new Set([0x0c, 0x80000018, 0x8000001f, 0x20, 0x80000023])
const MACH_O_RPATH_COMMAND = 0x8000001c
const MACH_O_SYSTEM_PREFIXES = ['/usr/lib/', '/System/Library/Frameworks/', '/System/Library/PrivateFrameworks/']
const WINDOWS_SYSTEM_DLLS = new Set([
  'advapi32.dll', 'bcrypt.dll', 'cabinet.dll', 'comctl32.dll', 'comdlg32.dll', 'crypt32.dll',
  'dwmapi.dll', 'gdi32.dll', 'imm32.dll', 'kernel32.dll', 'msimg32.dll', 'netapi32.dll',
  'ntdll.dll', 'ole32.dll', 'oleaut32.dll', 'rpcrt4.dll', 'secur32.dll', 'setupapi.dll',
  'shell32.dll', 'shlwapi.dll', 'user32.dll', 'userenv.dll', 'version.dll', 'winhttp.dll',
  'wininet.dll', 'winmm.dll', 'ws2_32.dll'
])

function readCString(bytes, start, end, label) {
  if (!Number.isSafeInteger(start) || start < 0 || start >= end || end > bytes.length) {
    throw new Error(`[document-runtime] ${label} string range is invalid`)
  }
  const terminator = bytes.indexOf(0, start)
  if (terminator < start || terminator >= end || terminator === start || terminator - start > 4096) {
    throw new Error(`[document-runtime] ${label} string is not bounded`)
  }
  const value = bytes.toString('utf8', start, terminator)
  if (!value || Buffer.byteLength(value, 'utf8') !== terminator - start ||
    [...value].some((character) => character.codePointAt(0) < 32 || character.codePointAt(0) === 127)) {
    throw new Error(`[document-runtime] ${label} string is invalid`)
  }
  return value
}

function parseMachOImports(bytes) {
  if (bytes.length < 32) throw new Error('[document-runtime] Mach-O import header is truncated')
  const magicBE = bytes.readUInt32BE(0)
  const magicLE = bytes.readUInt32LE(0)
  const read32 = magicBE === 0xfeedfacf
    ? (offset) => bytes.readUInt32BE(offset)
    : magicLE === 0xfeedfacf
      ? (offset) => bytes.readUInt32LE(offset)
      : null
  if (!read32) throw new Error('[document-runtime] Mach-O import magic is invalid')
  const commandCount = read32(16)
  const commandBytes = read32(20)
  const commandsEnd = 32 + commandBytes
  if (commandCount === 0 || commandCount > 4096 || commandsEnd > bytes.length) {
    throw new Error('[document-runtime] Mach-O import commands are invalid')
  }
  const imports = []
  const rpaths = []
  let offset = 32
  for (let index = 0; index < commandCount; index += 1) {
    if (offset + 8 > commandsEnd) throw new Error('[document-runtime] Mach-O import command is truncated')
    const command = read32(offset)
    const size = read32(offset + 4)
    if (size < 8 || size % 4 !== 0 || offset + size > commandsEnd) {
      throw new Error('[document-runtime] Mach-O import command size is invalid')
    }
    if (MACH_O_DYLIB_COMMANDS.has(command)) {
      if (size < 24) throw new Error('[document-runtime] Mach-O dylib command is truncated')
      const nameOffset = read32(offset + 8)
      if (nameOffset < 24 || nameOffset >= size) throw new Error('[document-runtime] Mach-O dylib name offset is invalid')
      imports.push(readCString(bytes, offset + nameOffset, offset + size, 'Mach-O dylib'))
    } else if (command === MACH_O_RPATH_COMMAND) {
      if (size < 12) throw new Error('[document-runtime] Mach-O rpath command is truncated')
      const nameOffset = read32(offset + 8)
      if (nameOffset < 12 || nameOffset >= size) throw new Error('[document-runtime] Mach-O rpath offset is invalid')
      rpaths.push(readCString(bytes, offset + nameOffset, offset + size, 'Mach-O rpath'))
    }
    offset += size
  }
  if (offset !== commandsEnd) throw new Error('[document-runtime] Mach-O import command walk is incomplete')
  return { imports, rpaths }
}

function peSections(bytes, optionalOffset, optionalSize, sectionCount) {
  const sections = []
  const table = optionalOffset + optionalSize
  if (table + sectionCount * 40 > bytes.length) throw new Error('[document-runtime] PE import section table is truncated')
  for (let index = 0; index < sectionCount; index += 1) {
    const offset = table + index * 40
    const virtualSize = bytes.readUInt32LE(offset + 8)
    const virtualAddress = bytes.readUInt32LE(offset + 12)
    const rawSize = bytes.readUInt32LE(offset + 16)
    const rawOffset = bytes.readUInt32LE(offset + 20)
    if (rawSize > 0 && (rawOffset + rawSize > bytes.length || rawOffset < bytes.readUInt32LE(optionalOffset + 60))) {
      throw new Error('[document-runtime] PE import section range is invalid')
    }
    sections.push({ virtualSize, virtualAddress, rawSize, rawOffset })
  }
  return sections
}

function peRvaOffset(bytes, rva, sections, sizeOfHeaders, minimumBytes = 1) {
  if (!Number.isSafeInteger(rva) || rva <= 0 || !Number.isSafeInteger(minimumBytes) || minimumBytes <= 0) {
    throw new Error('[document-runtime] PE import RVA is invalid')
  }
  if (rva < sizeOfHeaders) {
    if (rva + minimumBytes > sizeOfHeaders || rva + minimumBytes > bytes.length) {
      throw new Error('[document-runtime] PE import header RVA is out of range')
    }
    return rva
  }
  const matches = sections.filter((section) => {
    const mappedSize = Math.max(section.virtualSize, section.rawSize)
    return rva >= section.virtualAddress && rva < section.virtualAddress + mappedSize
  })
  if (matches.length !== 1) throw new Error('[document-runtime] PE import RVA is ambiguous')
  const section = matches[0]
  const delta = rva - section.virtualAddress
  if (delta + minimumBytes > section.rawSize || section.rawOffset + delta + minimumBytes > bytes.length) {
    throw new Error('[document-runtime] PE import RVA is not file-backed')
  }
  return section.rawOffset + delta
}

function parsePEImports(bytes) {
  if (bytes.length < 0x40 || bytes.toString('ascii', 0, 2) !== 'MZ') {
    throw new Error('[document-runtime] PE import DOS header is invalid')
  }
  const peOffset = bytes.readUInt32LE(0x3c)
  if (peOffset < 0x40 || peOffset + 24 > bytes.length || bytes.toString('ascii', peOffset, peOffset + 4) !== 'PE\0\0') {
    throw new Error('[document-runtime] PE import signature is invalid')
  }
  const sectionCount = bytes.readUInt16LE(peOffset + 6)
  const optionalSize = bytes.readUInt16LE(peOffset + 20)
  const optionalOffset = peOffset + 24
  if (sectionCount === 0 || sectionCount > 96 || optionalSize < 152 ||
    optionalOffset + optionalSize > bytes.length || bytes.readUInt16LE(optionalOffset) !== 0x20b) {
    throw new Error('[document-runtime] PE import optional header is invalid')
  }
  const directoryCount = bytes.readUInt32LE(optionalOffset + 108)
  if (directoryCount < 2 || directoryCount > 32) throw new Error('[document-runtime] PE import directory count is invalid')
  const sizeOfHeaders = bytes.readUInt32LE(optionalOffset + 60)
  const sections = peSections(bytes, optionalOffset, optionalSize, sectionCount)
  const imports = []
  const readDirectory = (index, delay) => {
    if (index >= directoryCount) return
    const directory = optionalOffset + 112 + index * 8
    const rva = bytes.readUInt32LE(directory)
    const size = bytes.readUInt32LE(directory + 4)
    if ((rva === 0) !== (size === 0)) throw new Error('[document-runtime] PE import directory is partially declared')
    if (rva === 0) return
    const itemSize = delay ? 32 : 20
    if (size < itemSize || size > 1024 * 1024) throw new Error('[document-runtime] PE import directory size is invalid')
    const start = peRvaOffset(bytes, rva, sections, sizeOfHeaders, itemSize)
    let terminated = false
    for (let cursor = 0; cursor + itemSize <= size && cursor / itemSize < 4096; cursor += itemSize) {
      const offset = start + cursor
      if (offset + itemSize > bytes.length) throw new Error('[document-runtime] PE import descriptor is truncated')
      let empty = true
      for (let byte = 0; byte < itemSize; byte += 1) empty = empty && bytes[offset + byte] === 0
      if (empty) {
        terminated = true
        break
      }
      if (delay && (bytes.readUInt32LE(offset) & 1) !== 1) {
        throw new Error('[document-runtime] PE delay import uses an unsupported VA form')
      }
      const nameRva = bytes.readUInt32LE(offset + (delay ? 4 : 12))
      const nameOffset = peRvaOffset(bytes, nameRva, sections, sizeOfHeaders)
      imports.push(readCString(bytes, nameOffset, Math.min(bytes.length, nameOffset + 4097), 'PE import'))
    }
    if (!terminated) throw new Error('[document-runtime] PE import descriptor table is unterminated')
  }
  readDirectory(1, false)
  if (directoryCount > 13) readDirectory(13, true)
  return { imports, rpaths: [] }
}

function normalizedPackageCandidate(base, suffix) {
  const candidate = posix.normalize(posix.join(base, suffix))
  return normalizedRelativePath(candidate) ? candidate : ''
}

function macBundledDependency(importName, importer, metadata, manifest) {
  if (MACH_O_SYSTEM_PREFIXES.some((prefix) => importName.startsWith(prefix))) return ''
  const byPath = new Map(manifest.files.map((entry) => [entry.path, entry]))
  const importerDirectory = posix.dirname(importer.path)
  const binaryDirectories = [...new Set([
    posix.dirname(manifest.capabilities.sofficeBinary),
    posix.dirname(manifest.capabilities.tesseractBinary)
  ])]
  let candidates = []
  if (importName.startsWith('@loader_path/')) {
    candidates = [normalizedPackageCandidate(importerDirectory, importName.slice('@loader_path/'.length))]
  } else if (importName.startsWith('@executable_path/')) {
    const suffix = importName.slice('@executable_path/'.length)
    candidates = (importer.kind === 'binary' ? [importerDirectory] : binaryDirectories)
      .map((base) => normalizedPackageCandidate(base, suffix))
  } else if (importName.startsWith('@rpath/')) {
    const suffix = importName.slice('@rpath/'.length)
    for (const rpath of metadata.rpaths) {
      if (rpath.startsWith('@loader_path/')) {
        candidates.push(normalizedPackageCandidate(importerDirectory, posix.join(rpath.slice('@loader_path/'.length), suffix)))
      } else if (rpath.startsWith('@executable_path/')) {
        for (const base of binaryDirectories) {
          candidates.push(normalizedPackageCandidate(base, posix.join(rpath.slice('@executable_path/'.length), suffix)))
        }
      } else if (MACH_O_SYSTEM_PREFIXES.some((prefix) => rpath.startsWith(prefix))) {
        candidates.push('')
      } else {
        throw new Error(`[document-runtime] Unsupported Mach-O rpath in ${importer.path}`)
      }
    }
  } else {
    throw new Error(`[document-runtime] Uncontrolled Mach-O import in ${importer.path}: ${importName}`)
  }
  const bundled = [...new Set(candidates.filter((candidate) => byPath.get(candidate)?.kind === 'library'))]
  if (bundled.length !== 1) {
    throw new Error(`[document-runtime] Mach-O import does not resolve to one bundled library in ${importer.path}: ${importName}`)
  }
  return bundled[0]
}

function windowsBundledDependency(importName, importer, manifest) {
  if (!/^[A-Za-z0-9_.+-]{1,255}\.dll$/iu.test(importName) || basename(importName) !== importName) {
    throw new Error(`[document-runtime] Windows import name is invalid in ${importer.path}`)
  }
  const folded = importName.toLowerCase()
  if (folded.startsWith('api-ms-win-') || folded.startsWith('ext-ms-win-') || WINDOWS_SYSTEM_DLLS.has(folded)) return ''
  const matches = manifest.files.filter((entry) => entry.kind === 'library' &&
    posix.basename(entry.path).toLowerCase() === folded)
  if (matches.length !== 1) {
    throw new Error(`[document-runtime] PE import does not resolve to one bundled library in ${importer.path}: ${importName}`)
  }
  return matches[0].path
}

function verifyNativeDependencyClosure(root, manifest, target) {
  for (const entry of manifest.files.filter((item) => item.kind === 'binary' || item.kind === 'library')) {
    const path = join(root, ...entry.path.split('/'))
    const bytes = readBoundedStableFile(path, MAX_NATIVE_INSPECTION_BYTES)
    const metadata = target.format === 'mach-o' ? parseMachOImports(bytes) : parsePEImports(bytes)
    const actual = new Set()
    for (const importName of metadata.imports) {
      const dependency = target.format === 'mach-o'
        ? macBundledDependency(importName, entry, metadata, manifest)
        : windowsBundledDependency(importName, entry, manifest)
      if (dependency) actual.add(dependency)
    }
    const actualSorted = [...actual].sort()
    if (canonicalJSON(actualSorted) !== canonicalJSON(entry.dependencies)) {
      throw new Error(`[document-runtime] Native import closure mismatch: ${entry.path}`)
    }
  }
}

function verifyDocumentRuntime(root, target, options = {}) {
  const canonicalRoot = realpathSync(root)
  if (canonicalRoot !== resolve(root)) throw new Error('[document-runtime] Runtime root is not canonical')
  assertNoSymlinkPath(canonicalRoot, canonicalRoot, 'directory')
  const record = parseManifest(join(canonicalRoot, MANIFEST_FILE_NAME), target)
  const locked = requireAuthorityLock(record, target, canonicalRoot, options)
  const expectedFiles = new Set([
    MANIFEST_FILE_NAME,
    AUTHORITY_LOCK_FILE_NAME,
    ...record.manifest.files.map((entry) => entry.path)
  ])
  const actualFiles = []
  const actualDirectories = []
  const stack = [canonicalRoot]
  while (stack.length) {
    const current = stack.pop()
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const path = join(current, entry.name)
      if (entry.isSymbolicLink()) throw new Error('[document-runtime] Runtime tree contains a symbolic link')
      if (entry.isDirectory()) {
        actualDirectories.push(relative(canonicalRoot, path).split(sep).join('/'))
        stack.push(path)
      } else if (entry.isFile()) {
        actualFiles.push(relative(canonicalRoot, path).split(sep).join('/'))
      } else {
        throw new Error('[document-runtime] Runtime tree contains an unsupported entry')
      }
    }
  }
  actualFiles.sort()
  const expectedSorted = [...expectedFiles].sort()
  const expectedDirectories = new Set()
  for (const file of expectedFiles) {
    const parts = file.split('/').slice(0, -1)
    for (let index = 1; index <= parts.length; index += 1) {
      expectedDirectories.add(parts.slice(0, index).join('/'))
    }
  }
  if (canonicalJSON(actualFiles) !== canonicalJSON(expectedSorted) ||
    canonicalJSON(actualDirectories.sort()) !== canonicalJSON([...expectedDirectories].sort())) {
    throw new Error('[document-runtime] Runtime tree inventory does not match the manifest')
  }
  for (const entry of record.manifest.files) {
    const path = join(canonicalRoot, ...entry.path.split('/'))
    assertNoSymlinkPath(canonicalRoot, path)
    const identity = hashStableRegularFile(path)
    if (!sameIdentity(identity, { sha256: entry.sha256, byteLength: entry.byteLength })) {
      throw new Error(`[document-runtime] Full file identity mismatch: ${entry.path}`)
    }
    if (entry.kind === 'binary' || entry.kind === 'library') {
      const signature = nativeSignature(path, target)
      if (canonicalJSON(signature) !== canonicalJSON(entry.platformSignature)) {
        throw new Error(`[document-runtime] Native target signature mismatch: ${entry.path}`)
      }
    }
  }
  verifyNativeDependencyClosure(canonicalRoot, record.manifest, target)
  return {
    ...record,
    root: canonicalRoot,
    sofficeBinary: join(canonicalRoot, ...record.manifest.capabilities.sofficeBinary.split('/')),
    tesseractBinary: join(canonicalRoot, ...record.manifest.capabilities.tesseractBinary.split('/')),
    tessdataDirectory: join(canonicalRoot, ...record.manifest.capabilities.tessdataDirectory.split('/')),
    treeSha256: record.manifest.treeSha256,
    targetKey: target.key,
    authority: locked.authority
  }
}

function createDocumentRuntimeManifest(root, target, definition) {
  const files = [...definition.files]
    .sort((a, b) => a.path.localeCompare(b.path))
    .map((metadata) => {
      const path = join(root, ...metadata.path.split('/'))
      const identity = hashStableRegularFile(path)
      return {
        path: metadata.path,
        kind: metadata.kind,
        sourceId: metadata.sourceId,
        licenseId: metadata.licenseId,
        sha256: identity.sha256,
        byteLength: identity.byteLength,
        platformSignature: metadata.kind === 'binary' || metadata.kind === 'library'
          ? nativeSignature(path, target)
          : { kind: 'content', targetKey: target.key },
        dependencies: [...(metadata.dependencies || [])].sort()
      }
    })
  const withoutTree = {
    schemaVersion: 2,
    contract: MANIFEST_CONTRACT,
    target: { key: target.key, platform: target.platform, arch: target.arch, triple: target.triple },
    sources: [...definition.sources].sort((a, b) => a.id.localeCompare(b.id)),
    licenses: [...definition.licenses].sort((a, b) => a.id.localeCompare(b.id)),
    capabilities: definition.capabilities,
    files
  }
  const manifest = { ...withoutTree, treeSha256: treeDigest(withoutTree) }
  if (!validateManifestShape(manifest, target)) throw new Error('[document-runtime] Manifest definition is invalid')
  verifyNativeDependencyClosure(root, manifest, target)
  return manifest
}

function syncFile(path) {
  const descriptor = openSync(path, constants.O_RDONLY)
  try { fsyncSync(descriptor) } finally { closeSync(descriptor) }
}

function syncDirectory(path) {
  if (process.platform === 'win32') return
  const descriptor = openSync(path, constants.O_RDONLY)
  try { fsyncSync(descriptor) } finally { closeSync(descriptor) }
}

function syncDirectoryTree(root) {
  const directories = []
  const stack = [root]
  while (stack.length) {
    const current = stack.pop()
    directories.push(current)
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      if (entry.isDirectory() && !entry.isSymbolicLink()) stack.push(join(current, entry.name))
    }
  }
  directories.sort((left, right) => right.split(sep).length - left.split(sep).length)
  for (const directory of directories) syncDirectory(directory)
}

function resolveAssetRoot(repoRoot, target, options = {}) {
  const explicit = options.assetRoot || String((options.env || process.env)[ASSET_ROOT_ENV] || '').trim()
  if (explicit) {
    if (!isAbsolute(explicit) || explicit.includes('\0') || resolve(explicit) !== explicit) {
      throw new Error(`[document-runtime] ${UNAVAILABLE}`)
    }
    if (!existsSync(explicit)) throw new Error(`[document-runtime] ${UNAVAILABLE}`)
    return realpathSync(explicit)
  }
  const staged = join(repoRoot, 'runtime', 'document-runtime-staging', target.key)
  return existsSync(staged) ? realpathSync(staged) : ''
}

function materializeDocumentRuntime(options) {
  const target = targetContract(options.platform, options.arch)
  const sourceRoot = resolveAssetRoot(realpathSync(options.repoRoot), target, options)
  const destination = join(options.resourcesRoot, 'runtime', 'document-runtime')
  if (!sourceRoot) {
    if (existsSync(destination)) throw new Error('[document-runtime] Uncontrolled packaged document runtime is forbidden')
    if (options.required === true) throw new Error(`[document-runtime] ${UNAVAILABLE}`)
    return { available: false, blocker: UNAVAILABLE, targetKey: target.key }
  }
  if (existsSync(destination)) throw new Error('[document-runtime] Packaged document runtime destination is not empty')
  const source = verifyDocumentRuntime(sourceRoot, target, {
    authorityLock: options.authorityLock
  })
  mkdirSync(dirname(destination), { recursive: true, mode: 0o755 })
  const stage = mkdtempSync(join(dirname(destination), '.document-runtime-stage-'))
  let published = false
  try {
    for (const entry of [
      ...source.manifest.files,
      { path: MANIFEST_FILE_NAME, kind: 'data' },
      { path: AUTHORITY_LOCK_FILE_NAME, kind: 'data' }
    ]) {
      const from = join(sourceRoot, ...entry.path.split('/'))
      const to = join(stage, ...entry.path.split('/'))
      mkdirSync(dirname(to), { recursive: true, mode: 0o755 })
      copyFileSync(from, to, constants.COPYFILE_EXCL)
      chmodSync(to, entry.kind === 'binary' || entry.kind === 'library' ? 0o755 : 0o600)
      syncFile(to)
    }
    const staged = verifyDocumentRuntime(stage, target, { authorityLock: options.authorityLock })
    const sourceAfter = verifyDocumentRuntime(sourceRoot, target, { authorityLock: options.authorityLock })
    if (staged.manifestSha256 !== source.manifestSha256 || staged.treeSha256 !== source.treeSha256 ||
      sourceAfter.manifestSha256 !== source.manifestSha256 || sourceAfter.treeSha256 !== source.treeSha256) {
      throw new Error('[document-runtime] Runtime changed around atomic materialization')
    }
    syncDirectoryTree(stage)
    renameSync(stage, destination)
    published = true
    syncDirectory(dirname(destination))
    const packaged = verifyDocumentRuntime(destination, target, { authorityLock: options.authorityLock })
    return {
      available: true,
      blocker: '',
      targetKey: target.key,
      manifestSha256: packaged.manifestSha256,
      treeSha256: packaged.treeSha256,
      root: destination
    }
  } finally {
    if (!published) rmSync(stage, { recursive: true, force: true })
  }
}

module.exports = {
  ASSET_ROOT_ENV,
  AUTHORITY_LOCK_CONTRACT,
  AUTHORITY_LOCK_FILE_NAME,
  AUTHORITY_LOCK_PATH,
  FROZEN_AUTHORITY_LOCK_SHA256,
  MANIFEST_CONTRACT,
  MANIFEST_FILE_NAME,
  UNAVAILABLE,
  createAuthorityLockEntry,
  createDocumentRuntimeManifest,
  materializeDocumentRuntime,
  parseManifest,
  treeDigest,
  validAuthorityLock,
  validateManifestShape,
  verifyDocumentRuntime
}
