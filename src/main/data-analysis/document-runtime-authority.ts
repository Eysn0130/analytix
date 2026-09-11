import { createHash } from 'node:crypto'
import {
  closeSync,
  constants,
  fstatSync,
  lstatSync,
  openSync,
  readFileSync,
  readSync,
  readdirSync,
  realpathSync
} from 'node:fs'
import { basename, isAbsolute, join, posix, relative, resolve, sep } from 'node:path'
import { parseStrictJsonObject } from '../controlled-artifact/strict-json'
import { inspectNativePayload, type NativePayloadFormat } from './native-payload-identity'
import { nativeTargetKey, normalizeNativeArch, normalizeNativePlatform } from './native-runtime-paths'
import frozenAuthorityLockBytes from '../../../scripts/document-runtime-authority-lock.json?raw'

export const DOCUMENT_RUNTIME_MANIFEST_FILE = 'analytix-document-runtime-manifest.json'
export const DOCUMENT_RUNTIME_CONTRACT = 'analytix.document-runtime/v2'
export const DOCUMENT_RUNTIME_UNAVAILABLE = 'document_runtime_asset_authority_unavailable'
export const DOCUMENT_RUNTIME_AUTHORITY_LOCK_FILE = 'analytix-document-runtime-authority-lock.json'

const TREE_DOMAIN = 'AnalytixDocumentRuntimeTreeV2\0'
const SOURCE_SET_DOMAIN = 'AnalytixDocumentRuntimeSourceSetV1\0'
const LICENSE_SET_DOMAIN = 'AnalytixDocumentRuntimeLicenseSetV1\0'
const AUTHORITY_LOCK_CONTRACT = 'analytix.document-runtime-authority-lock/v1'
const FROZEN_AUTHORITY_LOCK_SHA256 = 'd1fa4ec449edbef8d310a2fdfb8bc236609aacf75b9dd6cb295c4b374d0226f3'
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
const SHA256_RE = /^[0-9a-f]{64}$/u
const ID_RE = /^[a-z0-9][a-z0-9._-]{0,127}$/u
const SPDX_RE = /^[A-Za-z0-9][A-Za-z0-9.+() -]{0,127}$/u
const FILE_KINDS = new Set(['binary', 'library', 'data', 'license'])
const LOCK_ROOT_KEYS = ['schemaVersion', 'contract', 'targets']
const LOCK_TARGET_KEYS = [
  'targetKey', 'assetReleaseId', 'manifestSha256', 'treeSha256',
  'sourceSetSha256', 'licenseSetSha256'
]
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

type Target = {
  key: string
  platform: string
  arch: string
  triple: string
  format: NativePayloadFormat
}

type Manifest = Record<string, any> & {
  target: Record<string, string>
  sources: Array<Record<string, string>>
  licenses: Array<Record<string, string>>
  capabilities: {
    sofficeBinary: string
    tesseractBinary: string
    tessdataDirectory: string
    ocrLanguages: string[]
  }
  files: Array<Record<string, any>>
  treeSha256: string
}

export type PackagedDocumentRuntimeAuthority = {
  root: string
  manifestSha256: string
  treeSha256: string
  targetKey: string
  sofficeBinary: string
  tesseractBinary: string
  tessdataDirectory: string
}

function sha256(value: Buffer | string): string {
  return createHash('sha256').update(value).digest('hex')
}

function exactKeys(value: unknown, expected: string[]): value is Record<string, any> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const keys = Object.keys(value)
  return keys.length === expected.length && keys.every((key) => expected.includes(key))
}

function normalizedRelativePath(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && !value.includes('\\') &&
    !value.includes('\0') && !isAbsolute(value) && !posix.isAbsolute(value) &&
    posix.normalize(value) === value && value !== '.' && !value.startsWith('../') &&
    !value.includes('/../') && !value.endsWith('/')
}

function targetFor(platform: string, arch: string): Target | undefined {
  const normalizedPlatform = normalizeNativePlatform(platform)
  const normalizedArch = normalizeNativeArch(arch)
  const key = nativeTargetKey(platform, arch)
  if (!normalizedPlatform || !normalizedArch || !key) return undefined
  if (key === 'darwin-arm64') {
    return { key, platform: 'darwin', arch: 'arm64', triple: 'aarch64-apple-darwin', format: 'mach-o' }
  }
  if (key === 'win32-x64') {
    return { key, platform: 'win32', arch: 'x64', triple: 'x86_64-pc-windows-msvc', format: 'pe' }
  }
  return undefined
}

function assertPath(root: string, path: string, kind: 'file' | 'directory'): void {
  const rootPath = resolve(root)
  const resolvedPath = resolve(path)
  const rel = relative(rootPath, resolvedPath)
  if (rel === '..' || rel.startsWith(`..${sep}`) || isAbsolute(rel)) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  const rootStat = lstatSync(rootPath)
  if (!rootStat.isDirectory() || rootStat.isSymbolicLink()) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  let current = rootPath
  for (const part of rel.split(sep).filter(Boolean)) {
    current = join(current, part)
    if (lstatSync(current).isSymbolicLink()) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  const stat = lstatSync(resolvedPath)
  if (kind === 'file' ? (!stat.isFile() || stat.nlink !== 1) : !stat.isDirectory()) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
}

function hashStableFile(path: string, maximumBytes = 2 * 1024 * 1024 * 1024): {
  sha256: string
  byteLength: number
} {
  const before = lstatSync(path)
  if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1 || before.size <= 0 ||
    before.size > maximumBytes || (before.mode & 0o022) !== 0) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  const descriptor = openSync(path, constants.O_RDONLY | (constants.O_NOFOLLOW || 0))
  const digest = createHash('sha256')
  let byteLength = 0
  try {
    const opened = fstatSync(descriptor)
    if (!opened.isFile() || opened.dev !== before.dev || opened.ino !== before.ino || opened.size !== before.size) {
      throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    }
    const buffer = Buffer.allocUnsafe(1024 * 1024)
    while (true) {
      const count = readSync(descriptor, buffer, 0, buffer.length, null)
      if (count === 0) break
      byteLength += count
      if (byteLength > maximumBytes) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
      digest.update(buffer.subarray(0, count))
    }
    const afterOpened = fstatSync(descriptor)
    const after = lstatSync(path)
    if (after.dev !== before.dev || after.ino !== before.ino || after.size !== before.size ||
      after.mtimeMs !== before.mtimeMs || afterOpened.size !== before.size || byteLength !== before.size) {
      throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    }
  } finally {
    closeSync(descriptor)
  }
  return { sha256: digest.digest('hex'), byteLength }
}

function readBoundedStableFile(path: string, maximumBytes: number): Buffer {
  const identity = hashStableFile(path, maximumBytes)
  const bytes = readFileSync(path)
  const after = hashStableFile(path, maximumBytes)
  if (bytes.length !== identity.byteLength || sha256(bytes) !== identity.sha256 ||
    after.sha256 !== identity.sha256 || after.byteLength !== identity.byteLength) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  return bytes
}

function treeDigest(manifest: Manifest): string {
  return sha256(Buffer.concat([
    Buffer.from(TREE_DOMAIN, 'utf8'),
    Buffer.from(JSON.stringify({
      target: manifest.target,
      sources: manifest.sources,
      licenses: manifest.licenses,
      capabilities: manifest.capabilities,
      files: manifest.files
    }), 'utf8')
  ]))
}

function validShape(value: unknown, target: Target): value is Manifest {
  if (!exactKeys(value, ROOT_KEYS) || value.schemaVersion !== 2 || value.contract !== DOCUMENT_RUNTIME_CONTRACT ||
    !exactKeys(value.target, TARGET_KEYS) || value.target.key !== target.key ||
    value.target.platform !== target.platform || value.target.arch !== target.arch || value.target.triple !== target.triple ||
    !Array.isArray(value.sources) || value.sources.length === 0 || !Array.isArray(value.licenses) ||
    value.licenses.length === 0 || !exactKeys(value.capabilities, CAPABILITY_KEYS) ||
    !Array.isArray(value.files) || value.files.length < 5 || !SHA256_RE.test(value.treeSha256 || '')) return false
  if (value.sources.some((source: unknown) => !exactKeys(source, SOURCE_KEYS) || !ID_RE.test(source.id || '') ||
    typeof source.name !== 'string' || !source.name.trim() || typeof source.version !== 'string' ||
    !source.version.trim() || !SHA256_RE.test(source.archiveSha256 || '') || !validSourceUri(source.sourceUri))) return false
  if (value.licenses.some((license: unknown) => !exactKeys(license, LICENSE_KEYS) ||
    !ID_RE.test(license.id || '') || !ID_RE.test(license.sourceId || '') ||
    !SPDX_RE.test(license.spdxExpression || '') || !normalizedRelativePath(license.path))) return false
  const sourceIds = new Set(value.sources.map((source: Record<string, string>) => source.id))
  const licenseIds = new Set(value.licenses.map((license: Record<string, string>) => license.id))
  const paths = new Set<string>()
  for (const file of value.files) {
    if (!exactKeys(file, FILE_KEYS) || !normalizedRelativePath(file.path) || !FILE_KINDS.has(file.kind) ||
      !sourceIds.has(file.sourceId) || !licenseIds.has(file.licenseId) || !SHA256_RE.test(file.sha256 || '') ||
      !Number.isSafeInteger(file.byteLength) || file.byteLength <= 0 ||
      paths.has(file.path.toLowerCase()) || !validSignature(file.platformSignature, file, target) ||
      !Array.isArray(file.dependencies) || file.dependencies.some((path: unknown) => !normalizedRelativePath(path)) ||
      JSON.stringify([...file.dependencies].sort()) !== JSON.stringify(file.dependencies) ||
      new Set(file.dependencies).size !== file.dependencies.length ||
      (!['binary', 'library'].includes(file.kind) && file.dependencies.length !== 0)) return false
    paths.add(file.path.toLowerCase())
  }
  if (JSON.stringify([...value.sources].sort((a, b) => a.id.localeCompare(b.id))) !== JSON.stringify(value.sources) ||
    JSON.stringify([...value.licenses].sort((a, b) => a.id.localeCompare(b.id))) !== JSON.stringify(value.licenses) ||
    JSON.stringify([...value.files].sort((a, b) => a.path.localeCompare(b.path))) !== JSON.stringify(value.files)) return false
  const byPath = new Map(value.files.map((file) => [file.path, file]))
  if (value.licenses.some((license) => byPath.get(license.path)?.kind !== 'license' ||
    byPath.get(license.path)?.sourceId !== license.sourceId)) return false
  const capabilities = value.capabilities
  if (!normalizedRelativePath(capabilities.sofficeBinary) || !normalizedRelativePath(capabilities.tesseractBinary) ||
    !normalizedRelativePath(capabilities.tessdataDirectory) || !Array.isArray(capabilities.ocrLanguages) ||
    capabilities.ocrLanguages.length < 2 || new Set(capabilities.ocrLanguages).size !== capabilities.ocrLanguages.length ||
    JSON.stringify([...capabilities.ocrLanguages].sort()) !== JSON.stringify(capabilities.ocrLanguages) ||
    !capabilities.ocrLanguages.includes('chi_sim') || !capabilities.ocrLanguages.includes('eng') ||
    byPath.get(capabilities.sofficeBinary)?.kind !== 'binary' ||
    byPath.get(capabilities.tesseractBinary)?.kind !== 'binary') return false
  for (const language of capabilities.ocrLanguages) {
    if (byPath.get(`${capabilities.tessdataDirectory}/${language}.traineddata`)?.kind !== 'data') return false
  }
  for (const file of value.files) {
    if (file.dependencies.some((path: string) => byPath.get(path)?.kind !== 'library')) return false
  }
  const reachableLibraries = new Set<string>()
  const queue = [capabilities.sofficeBinary, capabilities.tesseractBinary]
  while (queue.length) {
    const current = byPath.get(queue.pop()!)
    if (!current) return false
    for (const dependency of current.dependencies) {
      if (!reachableLibraries.has(dependency)) {
        reachableLibraries.add(dependency)
        queue.push(dependency)
      }
    }
  }
  if (value.files.some((file) => file.kind === 'library' && !reachableLibraries.has(file.path))) return false
  return value.treeSha256 === treeDigest(value as Manifest)
}

function validSourceUri(value: unknown): boolean {
  if (typeof value !== 'string') return false
  try {
    const uri = new URL(value)
    return ['https:', 'git+https:'].includes(uri.protocol) && !uri.username && !uri.password
  } catch {
    return false
  }
}

function validSignature(value: unknown, file: Record<string, any>, target: Target): boolean {
  if (file.kind === 'binary' || file.kind === 'library') {
    return exactKeys(value, NATIVE_SIGNATURE_KEYS) && value.kind === 'native' && value.targetKey === target.key &&
      value.format === target.format && value.arch === target.arch && SHA256_RE.test(value.payloadSha256 || '') &&
      Number.isSafeInteger(value.payloadByteLength) && value.payloadByteLength > 0 &&
      value.payloadByteLength <= file.byteLength
  }
  return exactKeys(value, CONTENT_SIGNATURE_KEYS) && value.kind === 'content' && value.targetKey === target.key
}

function readCString(bytes: Buffer, start: number, end: number): string {
  if (!Number.isSafeInteger(start) || start < 0 || start >= end || end > bytes.length) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  const terminator = bytes.indexOf(0, start)
  if (terminator < start || terminator >= end || terminator === start || terminator - start > 4096) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  const value = bytes.toString('utf8', start, terminator)
  if (!value || Buffer.byteLength(value, 'utf8') !== terminator - start ||
    [...value].some((character) => character.codePointAt(0)! < 32 || character.codePointAt(0) === 127)) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  return value
}

function parseMachOImports(bytes: Buffer): { imports: string[]; rpaths: string[] } {
  if (bytes.length < 32) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  const magicBE = bytes.readUInt32BE(0)
  const magicLE = bytes.readUInt32LE(0)
  const read32 = magicBE === 0xfeedfacf
    ? (offset: number): number => bytes.readUInt32BE(offset)
    : magicLE === 0xfeedfacf
      ? (offset: number): number => bytes.readUInt32LE(offset)
      : undefined
  if (!read32) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  const commandCount = read32(16)
  const commandsEnd = 32 + read32(20)
  if (commandCount === 0 || commandCount > 4096 || commandsEnd > bytes.length) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  const imports: string[] = []
  const rpaths: string[] = []
  let offset = 32
  for (let index = 0; index < commandCount; index += 1) {
    if (offset + 8 > commandsEnd) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    const command = read32(offset)
    const size = read32(offset + 4)
    if (size < 8 || size % 4 !== 0 || offset + size > commandsEnd) {
      throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    }
    if (MACH_O_DYLIB_COMMANDS.has(command)) {
      if (size < 24) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
      const nameOffset = read32(offset + 8)
      if (nameOffset < 24 || nameOffset >= size) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
      imports.push(readCString(bytes, offset + nameOffset, offset + size))
    } else if (command === MACH_O_RPATH_COMMAND) {
      if (size < 12) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
      const nameOffset = read32(offset + 8)
      if (nameOffset < 12 || nameOffset >= size) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
      rpaths.push(readCString(bytes, offset + nameOffset, offset + size))
    }
    offset += size
  }
  if (offset !== commandsEnd) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  return { imports, rpaths }
}

type PESection = { virtualSize: number; virtualAddress: number; rawSize: number; rawOffset: number }

function parsePEImports(bytes: Buffer): { imports: string[]; rpaths: string[] } {
  if (bytes.length < 0x40 || bytes.toString('ascii', 0, 2) !== 'MZ') {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  const peOffset = bytes.readUInt32LE(0x3c)
  if (peOffset < 0x40 || peOffset + 24 > bytes.length || bytes.toString('ascii', peOffset, peOffset + 4) !== 'PE\0\0') {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  const sectionCount = bytes.readUInt16LE(peOffset + 6)
  const optionalSize = bytes.readUInt16LE(peOffset + 20)
  const optionalOffset = peOffset + 24
  if (sectionCount === 0 || sectionCount > 96 || optionalSize < 152 ||
    optionalOffset + optionalSize > bytes.length || bytes.readUInt16LE(optionalOffset) !== 0x20b) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  const directoryCount = bytes.readUInt32LE(optionalOffset + 108)
  if (directoryCount < 2 || directoryCount > 32) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  const sizeOfHeaders = bytes.readUInt32LE(optionalOffset + 60)
  const table = optionalOffset + optionalSize
  if (table + sectionCount * 40 > bytes.length) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  const sections: PESection[] = []
  for (let index = 0; index < sectionCount; index += 1) {
    const offset = table + index * 40
    const section: PESection = {
      virtualSize: bytes.readUInt32LE(offset + 8),
      virtualAddress: bytes.readUInt32LE(offset + 12),
      rawSize: bytes.readUInt32LE(offset + 16),
      rawOffset: bytes.readUInt32LE(offset + 20)
    }
    if (section.rawSize > 0 && (section.rawOffset < sizeOfHeaders || section.rawOffset + section.rawSize > bytes.length)) {
      throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    }
    sections.push(section)
  }
  const rvaOffset = (rva: number, minimumBytes = 1): number => {
    if (!Number.isSafeInteger(rva) || rva <= 0 || !Number.isSafeInteger(minimumBytes) || minimumBytes <= 0) {
      throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    }
    if (rva < sizeOfHeaders) {
      if (rva + minimumBytes > sizeOfHeaders || rva + minimumBytes > bytes.length) {
        throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
      }
      return rva
    }
    const matches = sections.filter((section) => {
      const mappedSize = Math.max(section.virtualSize, section.rawSize)
      return rva >= section.virtualAddress && rva < section.virtualAddress + mappedSize
    })
    if (matches.length !== 1) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    const section = matches[0]
    const delta = rva - section.virtualAddress
    if (delta + minimumBytes > section.rawSize || section.rawOffset + delta + minimumBytes > bytes.length) {
      throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    }
    return section.rawOffset + delta
  }
  const imports: string[] = []
  const readDirectory = (index: number, delay: boolean): void => {
    if (index >= directoryCount) return
    const directory = optionalOffset + 112 + index * 8
    const rva = bytes.readUInt32LE(directory)
    const size = bytes.readUInt32LE(directory + 4)
    if ((rva === 0) !== (size === 0)) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    if (rva === 0) return
    const itemSize = delay ? 32 : 20
    if (size < itemSize || size > 1024 * 1024) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    const start = rvaOffset(rva, itemSize)
    let terminated = false
    for (let cursor = 0; cursor + itemSize <= size && cursor / itemSize < 4096; cursor += itemSize) {
      const offset = start + cursor
      if (offset + itemSize > bytes.length) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
      let empty = true
      for (let byte = 0; byte < itemSize; byte += 1) empty = empty && bytes[offset + byte] === 0
      if (empty) {
        terminated = true
        break
      }
      if (delay && (bytes.readUInt32LE(offset) & 1) !== 1) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
      const nameRva = bytes.readUInt32LE(offset + (delay ? 4 : 12))
      const nameOffset = rvaOffset(nameRva)
      imports.push(readCString(bytes, nameOffset, Math.min(bytes.length, nameOffset + 4097)))
    }
    if (!terminated) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  readDirectory(1, false)
  if (directoryCount > 13) readDirectory(13, true)
  return { imports, rpaths: [] }
}

function normalizedPackageCandidate(base: string, suffix: string): string {
  const candidate = posix.normalize(posix.join(base, suffix))
  return normalizedRelativePath(candidate) ? candidate : ''
}

function macBundledDependency(
  importName: string,
  importer: Record<string, any>,
  metadata: { imports: string[]; rpaths: string[] },
  manifest: Manifest
): string {
  if (MACH_O_SYSTEM_PREFIXES.some((prefix) => importName.startsWith(prefix))) return ''
  const byPath = new Map(manifest.files.map((entry) => [entry.path, entry]))
  const importerDirectory = posix.dirname(importer.path)
  const binaryDirectories = [...new Set([
    posix.dirname(manifest.capabilities.sofficeBinary),
    posix.dirname(manifest.capabilities.tesseractBinary)
  ])]
  let candidates: string[] = []
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
        throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
      }
    }
  } else {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  const bundled = [...new Set(candidates.filter((candidate) => byPath.get(candidate)?.kind === 'library'))]
  if (bundled.length !== 1) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  return bundled[0]
}

function windowsBundledDependency(importName: string, manifest: Manifest): string {
  if (!/^[A-Za-z0-9_.+-]{1,255}\.dll$/iu.test(importName) || basename(importName) !== importName) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  const folded = importName.toLowerCase()
  if (folded.startsWith('api-ms-win-') || folded.startsWith('ext-ms-win-') || WINDOWS_SYSTEM_DLLS.has(folded)) return ''
  const matches = manifest.files.filter((entry) => entry.kind === 'library' &&
    posix.basename(entry.path).toLowerCase() === folded)
  if (matches.length !== 1) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  return matches[0].path
}

function verifyNativeDependencyClosure(root: string, manifest: Manifest, target: Target): void {
  for (const file of manifest.files.filter((entry) => entry.kind === 'binary' || entry.kind === 'library')) {
    const path = join(root, ...file.path.split('/'))
    const bytes = readBoundedStableFile(path, MAX_NATIVE_INSPECTION_BYTES)
    const metadata = target.format === 'mach-o' ? parseMachOImports(bytes) : parsePEImports(bytes)
    const actual = new Set<string>()
    for (const importName of metadata.imports) {
      const dependency = target.format === 'mach-o'
        ? macBundledDependency(importName, file, metadata, manifest)
        : windowsBundledDependency(importName, manifest)
      if (dependency) actual.add(dependency)
    }
    if (JSON.stringify([...actual].sort()) !== JSON.stringify(file.dependencies)) {
      throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    }
  }
}

function setDigest(domain: string, value: unknown): string {
  return sha256(Buffer.concat([
    Buffer.from(domain, 'utf8'),
    Buffer.from(JSON.stringify(value), 'utf8')
  ]))
}

function validAuthorityLock(value: unknown): value is Record<string, any> {
  if (!exactKeys(value, LOCK_ROOT_KEYS) || value.schemaVersion !== 1 ||
    value.contract !== AUTHORITY_LOCK_CONTRACT || !Array.isArray(value.targets) ||
    JSON.stringify([...value.targets].sort((a, b) => a.targetKey.localeCompare(b.targetKey))) !==
      JSON.stringify(value.targets)) return false
  const seen = new Set<string>()
  for (const target of value.targets) {
    if (!exactKeys(target, LOCK_TARGET_KEYS) || typeof target.targetKey !== 'string' ||
      !ID_RE.test(target.assetReleaseId || '') || !SHA256_RE.test(target.manifestSha256 || '') ||
      !SHA256_RE.test(target.treeSha256 || '') || !SHA256_RE.test(target.sourceSetSha256 || '') ||
      !SHA256_RE.test(target.licenseSetSha256 || '') || seen.has(target.targetKey)) return false
    seen.add(target.targetKey)
  }
  return true
}

function parseAuthorityLock(bytes: Buffer): Record<string, any> {
  let value: unknown
  try {
    value = parseStrictJsonObject(bytes, {
      maxBytes: 1024 * 1024,
      maxDepth: 4,
      maxTokens: 4096,
      maxStringBytes: 4096,
      maxNumberBytes: 32
    })
  } catch {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  if (!validAuthorityLock(value) || JSON.stringify(value) !== bytes.toString('utf8')) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  return value
}

function requireAuthorityLock(
  root: string,
  manifest: Manifest,
  manifestSha256: string,
  testOnlyAuthorityLock?: Buffer
): void {
  let authorizedBytes = Buffer.from(frozenAuthorityLockBytes, 'utf8')
  if (testOnlyAuthorityLock) {
    if (process.env.NODE_ENV !== 'test') throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    authorizedBytes = Buffer.from(testOnlyAuthorityLock)
  } else if (sha256(authorizedBytes) !== FROZEN_AUTHORITY_LOCK_SHA256) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  const packagedBytes = readBoundedStableFile(join(root, DOCUMENT_RUNTIME_AUTHORITY_LOCK_FILE), 1024 * 1024)
  if (!packagedBytes.equals(authorizedBytes)) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  const lock = parseAuthorityLock(authorizedBytes)
  const authorized = lock.targets.find((entry: Record<string, any>) => entry.targetKey === manifest.target.key)
  if (!authorized) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  const expected = {
    targetKey: manifest.target.key,
    assetReleaseId: authorized.assetReleaseId,
    manifestSha256,
    treeSha256: manifest.treeSha256,
    sourceSetSha256: setDigest(SOURCE_SET_DOMAIN, manifest.sources),
    licenseSetSha256: setDigest(LICENSE_SET_DOMAIN, manifest.licenses)
  }
  if (JSON.stringify(authorized) !== JSON.stringify(expected)) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
}

export function verifyPackagedDocumentRuntime(options: {
  resourcesRoot: string
  platform: string
  arch: string
  testOnlyAuthorityLock?: Buffer
}): PackagedDocumentRuntimeAuthority {
  const target = targetFor(options.platform, options.arch)
  if (!target) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  const root = join(options.resourcesRoot, 'runtime', 'document-runtime')
  assertPath(root, root, 'directory')
  if (realpathSync(root) !== resolve(root)) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  const manifestPath = join(root, DOCUMENT_RUNTIME_MANIFEST_FILE)
  assertPath(root, manifestPath, 'file')
  const manifestBytes = readBoundedStableFile(manifestPath, 1024 * 1024)
  let parsed: unknown
  try {
    parsed = parseStrictJsonObject(manifestBytes, {
      maxBytes: 1024 * 1024,
      maxDepth: 8,
      maxTokens: 65536,
      maxStringBytes: 16384,
      maxNumberBytes: 32
    })
  } catch {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  if (!validShape(parsed, target) || JSON.stringify(parsed) !== manifestBytes.toString('utf8')) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  requireAuthorityLock(root, parsed, sha256(manifestBytes), options.testOnlyAuthorityLock)
  const expected = [
    DOCUMENT_RUNTIME_MANIFEST_FILE,
    DOCUMENT_RUNTIME_AUTHORITY_LOCK_FILE,
    ...parsed.files.map((file) => file.path)
  ].sort()
  const actual: string[] = []
  const actualDirectories: string[] = []
  const stack = [root]
  while (stack.length) {
    const current = stack.pop()!
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const path = join(current, entry.name)
      if (entry.isSymbolicLink()) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
      if (entry.isDirectory()) {
        actualDirectories.push(relative(root, path).split(sep).join('/'))
        stack.push(path)
      }
      else if (entry.isFile()) actual.push(relative(root, path).split(sep).join('/'))
      else throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    }
  }
  const expectedDirectories = new Set<string>()
  for (const file of expected) {
    const parts = file.split('/').slice(0, -1)
    for (let index = 1; index <= parts.length; index += 1) {
      expectedDirectories.add(parts.slice(0, index).join('/'))
    }
  }
  if (JSON.stringify(actual.sort()) !== JSON.stringify(expected) ||
    JSON.stringify(actualDirectories.sort()) !== JSON.stringify([...expectedDirectories].sort())) {
    throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
  }
  for (const file of parsed.files) {
    const path = join(root, ...file.path.split('/'))
    assertPath(root, path, 'file')
    const identity = hashStableFile(path)
    if (identity.byteLength !== file.byteLength || identity.sha256 !== file.sha256) {
      throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    }
    if (file.kind === 'binary' || file.kind === 'library') {
      if (identity.byteLength > MAX_NATIVE_INSPECTION_BYTES) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
      const bytes = readBoundedStableFile(path, MAX_NATIVE_INSPECTION_BYTES)
      const payloadIdentity = inspectNativePayload(bytes, target.format)
      if (payloadIdentity.format !== target.format || payloadIdentity.arch !== target.arch ||
        payloadIdentity.payloadSha256 !== file.platformSignature.payloadSha256 ||
        payloadIdentity.payloadSize !== file.platformSignature.payloadByteLength) throw new Error(DOCUMENT_RUNTIME_UNAVAILABLE)
    }
  }
  verifyNativeDependencyClosure(root, parsed, target)
  return {
    root,
    manifestSha256: sha256(manifestBytes),
    treeSha256: parsed.treeSha256,
    targetKey: target.key,
    sofficeBinary: join(root, ...parsed.capabilities.sofficeBinary.split('/')),
    tesseractBinary: join(root, ...parsed.capabilities.tesseractBinary.split('/')),
    tessdataDirectory: join(root, ...parsed.capabilities.tessdataDirectory.split('/'))
  }
}

export function packagedDocumentRuntimeEnvironment(options: {
  resourcesRoot: string
  packaged: boolean
  platform: string
  arch: string
  testOnlyAuthorityLock?: Buffer
}): NodeJS.ProcessEnv {
  if (!options.packaged) return {}
  const authority = verifyPackagedDocumentRuntime(options)
  return {
    ANALYTIX_DOCUMENT_SOFFICE_BIN: authority.sofficeBinary,
    ANALYTIX_DOCUMENT_TESSERACT_BIN: authority.tesseractBinary,
    TESSDATA_PREFIX: authority.tessdataDirectory
  }
}
