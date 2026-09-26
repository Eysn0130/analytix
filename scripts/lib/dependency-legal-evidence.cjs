// Exact supplemental legal materials owned by the artifact legal audit.
// This catalog does not authorize execution, signing or publication.
const { createHash } = require('node:crypto')
const { readFileSync, lstatSync, mkdirSync, writeFileSync } = require('node:fs')
const { join, resolve, relative, isAbsolute, sep } = require('node:path')

const RESOURCE = 'dependency-legal'
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex')
const safePath = value => typeof value === 'string' && value.length > 0 &&
  !value.includes('\\') && !value.includes('\0') && !isAbsolute(value) &&
  value.split('/').every(part => part && part !== '.' && part !== '..')

function readRegular(root, entry, maximumBytes = 8 * 1024 * 1024) {
  if (!safePath(entry)) throw Error('dependency_legal_path_invalid')
  let path = resolve(root)
  for (const part of ['', ...entry.split('/')]) {
    if (part) path = join(path, part)
    const stat = lstatSync(path)
    if (stat.isSymbolicLink()) throw Error('dependency_legal_symlink_rejected')
  }
  const stat = lstatSync(path)
  if (!stat.isFile() || stat.nlink !== 1 || stat.size > maximumBytes) {
    throw Error('dependency_legal_file_invalid')
  }
  return readFileSync(path)
}

function loadCatalog(repoRoot) {
  const root = join(repoRoot, 'build', RESOURCE)
  const bytes = readRegular(repoRoot, `build/${RESOURCE}/manifest.json`)
  const catalog = JSON.parse(bytes)
  if (catalog.schemaVersion !== 1 || !Array.isArray(catalog.packages) ||
      !Array.isArray(catalog.locks)) throw Error('dependency_legal_catalog_invalid')
  const ids = new Set()
  for (const record of catalog.packages) {
    if (!safePath(record.id) || ids.has(record.id) || !record.name || !record.version ||
        !Array.isArray(record.bindings) || !Array.isArray(record.materials) ||
        !Array.isArray(record.packageJsonSha256) || !record.files ||
        !Object.keys(record.files).every(safePath)) throw Error('dependency_legal_record_invalid')
    ids.add(record.id)
    for (const material of record.materials) {
      const content = readRegular(repoRoot, `build/${RESOURCE}/${material.file}`)
      if (!content.toString('utf8').trim() || sha256(content) !== material.sha256) {
        throw Error('dependency_legal_material_changed')
      }
    }
  }
  return { root, bytes, catalog }
}

// Electron's official distribution supplies the full generated Chromium notice
// set. Keep its exact pins in source, not a second vendored 19 MB HTML copy.
function distributionMaterial(repoRoot, material) {
  const lock = JSON.parse(readRegular(repoRoot, 'package-lock.json')).packages[material.packagePath]
  const pkg = JSON.parse(readRegular(repoRoot, `${material.packagePath}/package.json`))
  if (pkg.name !== material.name || pkg.version !== material.version ||
      lock?.version !== material.version || lock.integrity !== material.integrity ||
      lock.resolved !== material.resolved) throw Error('dependency_legal_distribution_identity_changed')
  const bytes = readRegular(repoRoot, material.sourceFile, 32 * 1024 * 1024)
  if (!bytes.toString('utf8').trim() || bytes.length !== material.byteLength || sha256(bytes) !== material.sha256) {
    throw Error('dependency_legal_distribution_material_changed')
  }
  return bytes
}

function verifyDistributionMaterials(reader, loaded, options = {}) {
  if (!loaded.catalog.distributionMaterials?.length) return
  let platform = options.platform
  let arch = options.arch
  if (!platform) {
    const path = 'packaged-root/Contents/Frameworks/Electron Framework.framework/Versions/A/Electron Framework'
    if (!reader.has(path)) return // No macOS framework in this reader's scope.
    const framework = reader.read(path)
    if (framework.length < 32 || framework.readUInt32LE(0) !== 0xfeedfacf) {
      throw Error('dependency_legal_distribution_architecture_unknown')
    }
    platform = 'darwin'
    const cpu = framework.readUInt32LE(4)
    if (cpu !== 0x0100000c && cpu !== 0x01000007) throw Error('dependency_legal_distribution_architecture_unknown')
    arch = cpu === 0x0100000c ? 'arm64' : 'x64'
  }
  const materials = loaded.catalog.distributionMaterials.filter(material => material.platform === platform && material.arch === arch)
  if (!materials.length) return
  const prefix = resourcePrefix(reader)
  if (!reader.read(`${prefix}manifest.json`)?.equals(loaded.bytes)) throw Error('dependency_legal_catalog_changed')
  for (const material of materials) {
    const bytes = reader.read(`${prefix}${material.file}`)
    if (!bytes || bytes.length !== material.byteLength || sha256(bytes) !== material.sha256) {
      throw Error('dependency_legal_distribution_material_changed')
    }
  }
}

function resourcePrefix(reader) {
  const prefixes = ['', 'packaged-root/Contents/Resources/', 'packaged-root/resources/', 'packaged-root/Resources/']
    .filter(prefix => reader.has(`${prefix}${RESOURCE}/manifest.json`))
  if (prefixes.length !== 1) throw Error('dependency_legal_resource_owner_missing_or_ambiguous')
  return `${prefixes[0]}${RESOURCE}/`
}

function logicalEntry(reader, entry) {
  if (!safePath(entry)) throw Error('dependency_legal_instance_path_invalid')
  if (!entry.startsWith('packaged-root/')) return entry
  for (const component of reader.components || []) {
    const prefix = `packaged-root/${component.artifactEntry}.unpacked/`
    if (entry.startsWith(prefix)) return entry.slice(prefix.length)
  }
  throw Error('dependency_legal_instance_owner_invalid')
}

function verifyPackage(reader, entry, record, options = {}) {
  const logical = logicalEntry(reader, entry)
  const binding = record.bindings.find(binding =>
    `${binding.lock === 'package-lock.json' ? '' : 'packages/runtime/'}${binding.path}/package.json` === logical)
  if (!binding) throw Error('dependency_legal_instance_not_bound')
  const bytes = reader.read(entry)
  if (!bytes || !record.packageJsonSha256.includes(sha256(bytes))) {
    throw Error('dependency_legal_package_metadata_changed')
  }
  const pkg = JSON.parse(bytes)
  if (pkg.name !== record.name || pkg.version !== record.version) throw Error('dependency_legal_identity_changed')
  const prefix = entry.slice(0, -'package.json'.length)
  for (const pin of record.signingTransforms || []) {
    if (!safePath(pin.file) || !reader.has(`${prefix}${pin.file}`)) {
      throw Error('dependency_legal_native_source_missing')
    }
  }
  let count = 0
  for (const file of reader.entries()) {
    if (!file.startsWith(prefix)) continue
    const suffix = file.slice(prefix.length)
    if (suffix.startsWith('node_modules/') || suffix === 'package.json') continue
    const content = reader.read(file)
    const exact = content && sha256(content) === record.files[suffix]
    if (!safePath(suffix) || !record.files[suffix] ||
        (!exact && !verifySigningTransformation(content, suffix, record, options))) {
      throw Error('dependency_legal_package_content_changed')
    }
    count++
  }
  return { binding, fileCount: count }
}

// Source pins live in the canonical catalog, not in a mutable sidecar receipt.
// This proves source equivalence only. The existing staged closure, compiled Go
// authority and after-sign codesign verification still own candidate/signing trust.
function verifySigningTransformation(bytes, file, record, options) {
  const pin = record.signingTransforms?.find(pin => pin.file === file)
  if (options.allowSigned === false || !bytes || !pin ||
      pin.kind !== 'mach-o-thin64-signature-v1' || pin.arch !== 'arm64' ||
      pin.sourceSha256 !== record.files[file] || !/^[a-f0-9]{64}$/.test(pin.payloadSha256) ||
      !Number.isSafeInteger(pin.payloadByteLength) || pin.payloadByteLength <= 0 ||
      bytes.length < 32 || bytes.readUInt32LE(0) !== 0xfeedfacf ||
      bytes.readUInt32LE(4) !== 0x0100000c) return false
  try {
    const payload = require('../native-component-contract.cjs').nativePayloadIdentity(bytes, 'mach-o')
    return payload.sha256 === pin.payloadSha256 && payload.size === pin.payloadByteLength
  } catch { return false }
}

function verifyBoundSource(reader, entry, record, loaded, options = {}) {
  const prefix = resourcePrefix(reader)
  if (!reader.read(`${prefix}manifest.json`)?.equals(loaded.bytes)) {
    throw Error('dependency_legal_catalog_changed')
  }
  for (const lock of loaded.catalog.locks) {
    const bytes = reader.read(`${prefix}${lock.file}`)
    if (!bytes || sha256(bytes) !== lock.sha256) throw Error('dependency_legal_lock_changed')
    const value = JSON.parse(bytes)
    for (const binding of record.bindings.filter(binding => binding.lock === lock.path)) {
      const actual = value.packages?.[binding.path]
      if (actual?.version !== record.version || actual.resolved !== binding.resolved ||
          actual.integrity !== binding.integrity) throw Error('dependency_legal_lock_binding_changed')
    }
  }
  const matched = verifyPackage(reader, entry, record, options)
  for (const material of record.materials) {
    const bytes = reader.read(`${prefix}${material.file}`)
    if (!bytes || !bytes.toString('utf8').trim() || sha256(bytes) !== material.sha256) {
      throw Error('dependency_legal_material_changed')
    }
  }
  return matched
}

function verifyEvidence(reader, entry, record, loaded, options = {}) {
  const matched = verifyBoundSource(reader, entry, record, loaded, options)
  const prefix = resourcePrefix(reader)
  if (record.unresolved) throw Error(record.unresolved)
  // Only the explicitly reviewed MIT alternative can supplement an OR term.
  // Unknown compound obligations never become a single-license grant.
  const declared = JSON.parse(reader.read(entry)).license
  const selectedMIT = record.selectedLicense === 'MIT' &&
    (!declared || declared === 'MIT' || declared === '(MIT OR CC0-1.0)')
  const selectedApache = record.selectedLicense === 'Apache-2.0' && declared === 'Apache-2.0' &&
    record.sourceReview?.archiveIntegrity === record.integrity &&
    typeof record.sourceReview?.noticeReview === 'string' && record.sourceReview.noticeReview.length > 0
  if ((!selectedMIT && !selectedApache) || record.materials.length === 0) {
    throw Error('dependency_legal_license_selection_invalid')
  }
  return { ...matched, selectedLicense: record.selectedLicense,
    governingExpression: record.governingExpression || record.selectedLicense,
    licenseFile: `/${prefix}${record.materials[0].file}`, evidenceId: record.id }
}

function materializeLegalEvidence(repoRoot, resourcesDir, options = {}) {
  const loaded = loadCatalog(repoRoot)
  const resources = resolve(resourcesDir)
  if (lstatSync(resources).isSymbolicLink()) throw Error('dependency_legal_destination_invalid')
  const target = join(resources, RESOURCE)
  mkdirSync(target, { mode: 0o700 }) // No replacement of a previous/signed resource set.
  function put(entry, bytes) {
    if (!safePath(entry)) throw Error('dependency_legal_path_invalid')
    const destination = join(target, entry)
    const rel = relative(target, destination)
    if (isAbsolute(rel) || rel.startsWith(`..${sep}`)) throw Error('dependency_legal_path_invalid')
    mkdirSync(join(destination, '..'), { recursive: true, mode: 0o700 })
    writeFileSync(destination, bytes, { mode: 0o600, flag: 'wx' })
  }
  put('manifest.json', loaded.bytes)
  for (const lock of loaded.catalog.locks) {
    const bytes = readRegular(repoRoot, lock.path)
    if (sha256(bytes) !== lock.sha256) throw Error('dependency_legal_source_lock_changed')
    put(lock.file, bytes)
  }
  const copied = new Set()
  for (const record of loaded.catalog.packages) {
    for (const binding of record.bindings) {
      const lock = JSON.parse(readRegular(repoRoot, binding.lock)).packages[binding.path]
      if (lock?.version !== record.version || lock.resolved !== binding.resolved ||
          lock.integrity !== binding.integrity) throw Error('dependency_legal_source_binding_changed')
    }
    for (const material of record.materials) {
      if (!copied.has(material.file)) put(material.file, readRegular(loaded.root, material.file))
      copied.add(material.file)
    }
  }
  for (const material of loaded.catalog.distributionMaterials || []) {
    if (material.platform === (options.platform || process.platform) && material.arch === (options.arch || process.arch)) {
      put(material.file, distributionMaterial(repoRoot, material))
    }
  }
  return loaded
}

function verifyPackagedLegalMaterials(reader, loaded, options = {}) {
  verifyDistributionMaterials(reader, loaded, options)
  const matches = []
  for (const entry of reader.entries().filter(entry => entry.endsWith('/package.json'))) {
    let logical
    try { logical = logicalEntry(reader, entry) } catch { continue }
    const record = loaded.catalog.packages.find(record => record.bindings.some(binding =>
      `${binding.lock === 'package-lock.json' ? '' : 'packages/runtime/'}${binding.path}/package.json` === logical))
    if (record && !record.unresolved) matches.push(verifyEvidence(reader, entry, record, loaded, options))
    // Embedded component obligations may remain blocked, but source mutation
    // must already fail before sealing and again after signing.
    else if (record?.signingTransforms?.length) {
      matches.push(verifyBoundSource(reader, entry, record, loaded, options))
    }
  }
  return matches
}

module.exports = { loadCatalog, materializeLegalEvidence, verifyEvidence, verifyPackage, verifyPackagedLegalMaterials, verifyDistributionMaterials, logicalEntry, sha256, RESOURCE }
