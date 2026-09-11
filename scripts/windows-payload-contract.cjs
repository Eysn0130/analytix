const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')

const PAYLOAD_MANIFEST_RESOURCE = 'AnalytixPayloadManifest'
const PAYLOAD_ARCHIVE_RESOURCE = 'AnalytixPayloadArchive'
const PAYLOAD_EXTRACTOR_RESOURCE = 'PayloadExtractorExe'

function fileSha256(filePath) {
  const hash = crypto.createHash('sha256')
  const file = fs.openSync(filePath, 'r')
  const buffer = Buffer.allocUnsafe(1024 * 1024)
  try {
    let offset = 0
    while (true) {
      const count = fs.readSync(file, buffer, 0, buffer.length, offset)
      if (count === 0) break
      hash.update(buffer.subarray(0, count))
      offset += count
    }
  } finally {
    fs.closeSync(file)
  }
  return hash.digest('hex')
}

function exactKeys(value, expected) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const actual = Object.keys(value)
  if (actual.length !== expected.length) return false
  const expectedKeys = new Set(expected)
  return actual.every((key) => expectedKeys.has(key))
}

function isSha256(value) {
  return typeof value === 'string' && /^[0-9a-f]{64}$/.test(value)
}

function validatePortablePayloadPath(value) {
  if (typeof value !== 'string' || !value || !/^[\x20-\x7e]+$/.test(value) || value.includes('\\')) {
    throw new Error('[windows-payload] Payload path must use printable portable ASCII and forward slashes')
  }
  const segments = value.split('/')
  for (const segment of segments) {
    const stem = segment.split('.')[0].toUpperCase()
    if (
      !segment || segment === '.' || segment === '..' ||
      /[<>:"|?*]/.test(segment) || /[. ]$/.test(segment) ||
      /^(?:CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$/.test(stem)
    ) {
      throw new Error(`[windows-payload] Payload path is not portable on Windows: ${value}`)
    }
  }
  return value
}

function collectPayloadFiles(root, current = root, files = []) {
  const stat = fs.lstatSync(current)
  if (!stat.isDirectory() || stat.isSymbolicLink()) {
    throw new Error(`[windows-payload] Payload directory is not trusted: ${current}`)
  }
  for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
    const itemPath = path.join(current, entry.name)
    if (entry.isSymbolicLink()) throw new Error(`[windows-payload] Payload contains a link/reparse point: ${itemPath}`)
    if (entry.isDirectory()) collectPayloadFiles(root, itemPath, files)
    else if (entry.isFile()) files.push(itemPath)
    else throw new Error(`[windows-payload] Payload contains a special file: ${itemPath}`)
  }
  return files
}

function treeDigest(files) {
  const hash = crypto.createHash('sha256')
  for (const file of files) {
    hash.update(file.path)
    hash.update('\0')
    hash.update(String(file.size))
    hash.update('\0')
    hash.update(file.sha256)
    hash.update('\0')
  }
  return hash.digest('hex')
}

function payloadTreeIdentity(root) {
  const rootPath = path.resolve(root)
  const paths = collectPayloadFiles(rootPath).sort((left, right) => Buffer.compare(
    Buffer.from(path.relative(rootPath, left).split(path.sep).join('/'), 'utf8'),
    Buffer.from(path.relative(rootPath, right).split(path.sep).join('/'), 'utf8')
  ))
  const seen = new Set()
  const files = paths.map((filePath) => {
    const relativePath = path.relative(rootPath, filePath).split(path.sep).join('/')
    validatePortablePayloadPath(relativePath)
    const canonicalPath = relativePath.toLowerCase()
    if (!relativePath || relativePath.startsWith('../') || path.isAbsolute(relativePath)) {
      throw new Error(`[windows-payload] Payload path escapes its root: ${filePath}`)
    }
    if (seen.has(canonicalPath)) throw new Error(`[windows-payload] Payload has a case-insensitive collision: ${relativePath}`)
    seen.add(canonicalPath)
    const stat = fs.lstatSync(filePath)
    return { path: relativePath, size: stat.size, sha256: fileSha256(filePath) }
  })
  return {
    files,
    fileCount: files.length,
    totalBytes: files.reduce((total, file) => total + file.size, 0),
    treeSha256: treeDigest(files)
  }
}

function blobIdentity(filePath) {
  const stat = fs.lstatSync(filePath)
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size <= 0) {
    throw new Error(`[windows-payload] Embedded blob must be a non-empty regular file: ${filePath}`)
  }
  return { size: stat.size, sha256: fileSha256(filePath) }
}

function createPayloadManifest(options) {
  const tree = payloadTreeIdentity(options.root)
  return {
    schemaVersion: 1,
    treeAlgorithm: 'analytix-windows-payload-tree-v1',
    fileCount: tree.fileCount,
    totalBytes: tree.totalBytes,
    treeSha256: tree.treeSha256,
    files: tree.files,
    archive: blobIdentity(options.archive),
    extractor: blobIdentity(options.extractor)
  }
}

function parsePayloadManifest(filePath) {
  const text = fs.readFileSync(filePath, 'utf8')
  const manifest = JSON.parse(text)
  if (`${JSON.stringify(manifest, null, 2)}\n` !== text) {
    throw new Error('[windows-payload] Payload manifest is not canonical duplicate-free JSON')
  }
  if (!exactKeys(manifest, [
    'schemaVersion',
    'treeAlgorithm',
    'fileCount',
    'totalBytes',
    'treeSha256',
    'files',
    'archive',
    'extractor'
  ]) || manifest.schemaVersion !== 1 || manifest.treeAlgorithm !== 'analytix-windows-payload-tree-v1') {
    throw new Error('[windows-payload] Payload manifest schema is invalid')
  }
  if (
    !Number.isSafeInteger(manifest.fileCount) || manifest.fileCount <= 0 ||
    !Number.isSafeInteger(manifest.totalBytes) || manifest.totalBytes <= 0 ||
    !isSha256(manifest.treeSha256) ||
    !Array.isArray(manifest.files) || manifest.files.length !== manifest.fileCount ||
    !exactKeys(manifest.archive, ['size', 'sha256']) ||
    !exactKeys(manifest.extractor, ['size', 'sha256']) ||
    !Number.isSafeInteger(manifest.archive.size) || manifest.archive.size <= 0 || !isSha256(manifest.archive.sha256) ||
    !Number.isSafeInteger(manifest.extractor.size) || manifest.extractor.size <= 0 || !isSha256(manifest.extractor.sha256)
  ) {
    throw new Error('[windows-payload] Payload manifest inventory is invalid')
  }
  const seen = new Set()
  let previous = null
  let totalBytes = 0
  for (const file of manifest.files) {
    if (
      !exactKeys(file, ['path', 'size', 'sha256']) ||
      !Number.isSafeInteger(file.size) || file.size < 0 ||
      !isSha256(file.sha256)
    ) {
      throw new Error('[windows-payload] Payload manifest file record is invalid')
    }
    validatePortablePayloadPath(file.path)
    const canonicalPath = file.path.toLowerCase()
    if (seen.has(canonicalPath)) throw new Error('[windows-payload] Payload manifest has a case-insensitive collision')
    seen.add(canonicalPath)
    if (previous && Buffer.compare(Buffer.from(previous, 'utf8'), Buffer.from(file.path, 'utf8')) >= 0) {
      throw new Error('[windows-payload] Payload manifest file order is not canonical')
    }
    previous = file.path
    totalBytes += file.size
  }
  if (totalBytes !== manifest.totalBytes || treeDigest(manifest.files) !== manifest.treeSha256) {
    throw new Error('[windows-payload] Payload manifest tree digest is invalid')
  }
  return manifest
}

function verifyPayloadManifest(options) {
  const manifest = parsePayloadManifest(options.manifest)
  if (options.archive && JSON.stringify(blobIdentity(options.archive)) !== JSON.stringify(manifest.archive)) {
    throw new Error('[windows-payload] Embedded archive does not match its manifest')
  }
  if (options.extractor && JSON.stringify(blobIdentity(options.extractor)) !== JSON.stringify(manifest.extractor)) {
    throw new Error('[windows-payload] Embedded extractor does not match its manifest')
  }
  if (options.root) {
    const actual = payloadTreeIdentity(options.root)
    if (
      actual.fileCount !== manifest.fileCount ||
      actual.totalBytes !== manifest.totalBytes ||
      actual.treeSha256 !== manifest.treeSha256 ||
      JSON.stringify(actual.files) !== JSON.stringify(manifest.files)
    ) {
      throw new Error('[windows-payload] Extracted payload tree does not match its manifest')
    }
  }
  return manifest
}

function atomicWriteManifest(filePath, manifest) {
  const temporary = `${filePath}.tmp-${process.pid}`
  fs.writeFileSync(temporary, `${JSON.stringify(manifest, null, 2)}\n`, { encoding: 'utf8', mode: 0o600 })
  fs.renameSync(temporary, filePath)
}

function optionValue(argv, name) {
  const index = argv.indexOf(name)
  if (index < 0 || !argv[index + 1]) throw new Error(`[windows-payload] ${name} requires a value`)
  return argv[index + 1]
}

function main(argv = process.argv.slice(2)) {
  const command = argv[0]
  if (command === 'create') {
    const output = optionValue(argv, '--output')
    atomicWriteManifest(output, createPayloadManifest({
      root: optionValue(argv, '--root'),
      archive: optionValue(argv, '--archive'),
      extractor: optionValue(argv, '--extractor')
    }))
    console.log(output)
    return
  }
  if (command === 'verify') {
    verifyPayloadManifest({
      manifest: optionValue(argv, '--manifest'),
      root: argv.includes('--root') ? optionValue(argv, '--root') : undefined,
      archive: argv.includes('--archive') ? optionValue(argv, '--archive') : undefined,
      extractor: argv.includes('--extractor') ? optionValue(argv, '--extractor') : undefined
    })
    return
  }
  throw new Error('[windows-payload] Expected create or verify command')
}

if (require.main === module) {
  try {
    main()
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error))
    process.exitCode = 1
  }
}

module.exports = {
  PAYLOAD_ARCHIVE_RESOURCE,
  PAYLOAD_EXTRACTOR_RESOURCE,
  PAYLOAD_MANIFEST_RESOURCE,
  blobIdentity,
  createPayloadManifest,
  fileSha256,
  main,
  parsePayloadManifest,
  payloadTreeIdentity,
  validatePortablePayloadPath,
  verifyPayloadManifest
}
