#!/usr/bin/env node

import { createHash } from 'node:crypto'
import { createRequire } from 'node:module'
import {
  closeSync,
  constants,
  existsSync,
  fstatSync,
  lstatSync,
  openSync,
  readFileSync,
  readlinkSync,
  realpathSync,
  readdirSync
} from 'node:fs'
import { dirname, extname, isAbsolute, join, relative, resolve, sep } from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

export const ARTIFACT_LEGAL_OBLIGATIONS_CONTRACT = 'analytix.artifact-legal-obligations/v1'
export const ARTIFACT_OBLIGATION_CLASSES = Object.freeze({
  mandatoryExternal: 'mandatory_external_obligation',
  internalMetadata: 'internal_metadata',
  commercialLicenseDecision: 'commercial_license_decision',
  releaseAuthorization: 'release_authorization'
})

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const { loadCatalog, verifyEvidence, logicalEntry } = createRequire(import.meta.url)('./lib/dependency-legal-evidence.cjs')
const ROOT_PACKAGE_ENTRY = 'package.json'
const ROOT_PACKAGE_LOCK_ENTRY = 'package-lock.json'
const RUNTIME_PACKAGE_ENTRY = 'packages/runtime/package.json'
const RUNTIME_PACKAGE_LOCK_ENTRY = 'packages/runtime/package-lock.json'
const FUNDS_PLUGIN_ENTRY = 'plugins/analytix-fund-analysis/.codex-plugin/plugin.json'
const COMPUTER_USE_PLUGIN_ENTRY = 'plugins/analytix-computer-use/.codex-plugin/plugin.json'
const COMPUTER_USE_VENDOR_PACKAGE_ENTRY = 'vendor/analytix-computer-use/package.json'
const COMPUTER_USE_VENDOR_PLUGIN_ENTRY =
  'vendor/analytix-computer-use/plugins/analytix-computer-use/.codex-plugin/plugin.json'
const OPENCLAW_SHIM_PACKAGE_ENTRY = 'vendor/openclaw-shim/package.json'
const OPENCLAW_SHIM_LICENSE_ENTRY = 'vendor/openclaw-shim/LICENSE'
const CODE_REUSE_PROVENANCE_ENTRY = 'docs/analytix/upstreams/code-reuse-provenance.md'
const UPSTREAM_SOURCES_ENTRY = 'docs/analytix/upstreams/upstream-sources.json'
const PACKAGED_COMPUTER_USE_PACKAGE_ENTRY = 'node_modules/analytix-computer-use/package.json'
const PACKAGED_COMPUTER_USE_PLUGIN_ENTRY =
  'node_modules/analytix-computer-use/plugins/analytix-computer-use/.codex-plugin/plugin.json'
const PACKAGED_OPENCLAW_PACKAGE_ENTRY = 'node_modules/openclaw/package.json'
const PACKAGED_OPENCLAW_LICENSE_ENTRY = 'node_modules/openclaw/LICENSE'
const BUILDER_CONFIG_ENTRIES = [
  'electron-builder.config.cjs',
  'electron-builder.standard-win.cjs'
]
const PRODUCT_LICENSE_ENTRY = 'LICENSE'
const NOTICE_ENTRY = 'THIRD_PARTY_NOTICES.md'
const PRODUCT_LICENSE_ID = 'Apache-2.0'
const PRODUCT_PACKAGE_AUTHOR = 'Guoqin He (GitHub: Eysn0130)'
const PRODUCT_PLUGIN_AUTHOR = Object.freeze({
  name: 'Guoqin He',
  url: 'https://github.com/Eysn0130'
})
const PRODUCT_COPYRIGHT_NOTICE = 'Copyright 2026 Guoqin He (GitHub: Eysn0130)'
const OPENCLAW_SHIM_VERSION = '2026.6.6+analytix.shim.0'
const OPENCLAW_UPSTREAM_CONTRIBUTOR =
  'OpenClaw contributors (upstream v2026.5.18; https://github.com/openclaw/openclaw)'
const OPENCLAW_UPSTREAM_REPOSITORY = 'https://github.com/openclaw/openclaw'
const OPENCLAW_UPSTREAM_TAG = 'v2026.5.18'
const OPENCLAW_UPSTREAM_TAG_OBJECT = '0c5e335df4311f135a36f7f72fcd87784dd01c96'
const OPENCLAW_UPSTREAM_COMMIT = '50a2481652b6a62d573ece3cead60400dc77020d'
const OPENCLAW_UPSTREAM_LICENSE_BLOB = 'f7b526698bb7ed2d26d96c49f2f32234c88f69bc'
const OPENCLAW_UPSTREAM_LICENSE_SHA256 =
  '62316704df7426e5a79d2827ff8aca36e9abb3a73b8e68557030749ebefec667'
const OPENCLAW_PROVENANCE_MARKER = Object.freeze({
  schemaVersion: 1,
  sourceId: 'openclaw',
  upstreamTag: OPENCLAW_UPSTREAM_TAG,
  upstreamTagObject: OPENCLAW_UPSTREAM_TAG_OBJECT,
  upstreamCommit: OPENCLAW_UPSTREAM_COMMIT,
  licenseBlob: OPENCLAW_UPSTREAM_LICENSE_BLOB,
  licenseSha256: OPENCLAW_UPSTREAM_LICENSE_SHA256,
  destination: 'vendor/openclaw-shim'
})
const EXACT_ARTIFACT_ENV = 'ANALYTIX_EXACT_ARTIFACT_PATH'
const LICENSE_FILE_NAMES = [
  'LICENSE',
  'LICENSE.md',
  'LICENSE.txt',
  'LICENSE.markdown',
  'LICENSE.BSD',
  'LICENSE.MIT',
  'LICENSE.APACHE2',
  'LICENCE',
  'license-mit',
  'license',
  'license.md',
  'license.txt',
  'COPYING',
  'COPYING.md'
]
const NOTICE_FILE_NAMES = [
  'NOTICE',
  'NOTICE.md',
  'NOTICE.txt',
  'notice',
  'notice.md',
  'notice.txt'
]
const INTERNAL_PACKAGE_NAME_RE = /^(?:@analytix\/|analytix-|@funds\/)/i
const COMMERCIAL_LICENSE_RE = /(?:UNLICENSED|PROPRIETARY|POLYFORM|NONCOMMERCIAL|COMMERCIAL|CUSTOM)/i

export const REQUIRED_PRODUCT_LICENSE_TEXT = Object.freeze([
  PRODUCT_COPYRIGHT_NOTICE,
  'Apache License',
  'Version 2.0, January 2004',
  'http://www.apache.org/licenses/'
])
export const REQUIRED_PRODUCT_LICENSE_SHA256 =
  '339d7dd55119d76a0286d2be28868671e8905ad65df2967324a1b75db8d1f7a8'

// Bind the complete reviewed notice, including the already recorded Office
// generation dependency inventory. Keep this expected digest independent of
// the artifact being inspected; missing or changed notice bytes still fail.
export const REQUIRED_THIRD_PARTY_NOTICE_SHA256 =
  '1b6896b8f7e985fcb10a46ebe32dd3531e42b402e0e68f12fb2ff0372962f64b'

const CLAIM_CEILING = 'Exact mandatory artifact legal admission only; Analytix licensing is Apache-2.0, while signing, notarization, publication, and release authorization remain separate'

function hasNonEmptyString(value) {
  return typeof value === 'string' && value.trim().length > 0
}

function hasExactPackageAuthor(value) {
  return value === PRODUCT_PACKAGE_AUTHOR
}

function hasExactPluginAuthor(value) {
  return value && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).length === 2 &&
    value.name === PRODUCT_PLUGIN_AUTHOR.name &&
    value.url === PRODUCT_PLUGIN_AUTHOR.url
}

function hasExactStringArray(value, expected) {
  return Array.isArray(value) && value.length === expected.length &&
    value.every((entry, index) => entry === expected[index])
}

function hasExactOpenClawProvenance(value) {
  return value && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).length === 4 &&
    value.upstreamRepository === OPENCLAW_UPSTREAM_REPOSITORY &&
    value.upstreamTag === OPENCLAW_UPSTREAM_TAG &&
    value.upstreamTagObject === OPENCLAW_UPSTREAM_TAG_OBJECT &&
    value.upstreamCommit === OPENCLAW_UPSTREAM_COMMIT
}

function hasExactOpenClawMaintainerMetadata(value) {
  return hasExactPackageAuthor(value?.author) &&
    hasExactStringArray(value?.maintainers, [PRODUCT_PACKAGE_AUTHOR])
}

function hasExactOpenClawUpstreamMetadata(value) {
  return hasExactStringArray(value?.contributors, [OPENCLAW_UPSTREAM_CONTRIBUTOR]) &&
    hasExactOpenClawProvenance(value?.analytixProvenance)
}

function openClawProvenanceMarker(text) {
  const matches = [...String(text || '').matchAll(
    /<!--\s*analytix-openclaw-shim-provenance-v1\s+(\{[^\r\n]+\})\s*-->/gu
  )]
  if (matches.length !== 1) return null
  try {
    return JSON.parse(matches[0][1])
  } catch {
    return null
  }
}

function hasExactOpenClawProvenanceMarker(text) {
  const marker = openClawProvenanceMarker(text)
  return marker && JSON.stringify(marker) === JSON.stringify(OPENCLAW_PROVENANCE_MARKER)
}

function hasExactOpenClawRegistryEntry(manifest) {
  const matches = Array.isArray(manifest?.sources)
    ? manifest.sources.filter((entry) => entry?.id === 'openclaw')
    : []
  if (matches.length !== 1) return false
  const source = matches[0]
  return source.directory === 'openclaw' &&
    source.commit === OPENCLAW_UPSTREAM_COMMIT &&
    source.remote === `${OPENCLAW_UPSTREAM_REPOSITORY}.git` &&
    source.ledger === 'docs/analytix/upstreams/openclaw-sync.md' &&
    source.license?.path === 'LICENSE' &&
    source.license?.blob === OPENCLAW_UPSTREAM_LICENSE_BLOB &&
    source.license?.class === 'permissive-with-notice'
}

function productCopyrightLines(text) {
  return String(text || '').split(/\r?\n/u).filter((line) => /^Copyright 2026\b/u.test(line))
}

function sha256Text(text) {
  return createHash('sha256').update(String(text || ''), 'utf8').digest('hex')
}

function sha256Bytes(value) {
  return createHash('sha256').update(value).digest('hex')
}

function containsNoticeText(noticeText, requiredText) {
  return String(noticeText || '').replace(/\s+/g, ' ').includes(
    String(requiredText || '').replace(/\s+/g, ' ').trim()
  )
}

function readText(repoRoot, artifactEntry) {
  const path = join(repoRoot, artifactEntry)
  if (!existsSync(path)) return null
  try {
    return readFileSync(path, 'utf8')
  } catch {
    return null
  }
}

function readJson(repoRoot, artifactEntry) {
  const text = readText(repoRoot, artifactEntry)
  if (text === null) return null
  try {
    return JSON.parse(text)
  } catch {
    return null
  }
}

function blocker({ code, artifactEntry, governingTerm, missingAction, detail }) {
  return {
    code,
    artifactEntry,
    governingTerm,
    missingAction,
    ...(detail ? { detail } : {})
  }
}

function configHasNoticeMapping(text) {
  const source = String(text || '')
  return /from\s*:\s*['"]THIRD_PARTY_NOTICES\.md['"]/i.test(source) &&
    /to\s*:\s*['"]THIRD_PARTY_NOTICES\.md['"]/i.test(source)
}

function configHasProductLicenseMapping(text) {
  const source = String(text || '')
  return /from\s*:\s*['"]LICENSE['"]/i.test(source) &&
    /to\s*:\s*['"]LICENSE['"]/i.test(source)
}

function configHasOpenClawLicenseEntry(text) {
  return /['"]node_modules\/openclaw\/LICENSE['"]/u.test(String(text || ''))
}

function explicitLicenseOrNoticeExclusions(text) {
  const source = String(text || '')
  return [...source.matchAll(/['"](![^'"]*(?:LICENSE|NOTICE)[^'"]*)['"]/gi)]
    .map((match) => match[1])
}

function planInputFromRepository(repoRoot) {
  const builderConfigs = BUILDER_CONFIG_ENTRIES.map((artifactEntry) => ({
    artifactEntry,
    text: readText(repoRoot, artifactEntry)
  }))
  return {
    packageJson: readJson(repoRoot, ROOT_PACKAGE_ENTRY),
    packageLockJson: readJson(repoRoot, ROOT_PACKAGE_LOCK_ENTRY),
    runtimePackageJson: readJson(repoRoot, RUNTIME_PACKAGE_ENTRY),
    runtimePackageLockJson: readJson(repoRoot, RUNTIME_PACKAGE_LOCK_ENTRY),
    fundsPluginManifest: readJson(repoRoot, FUNDS_PLUGIN_ENTRY),
    computerUsePluginManifest: readJson(repoRoot, COMPUTER_USE_PLUGIN_ENTRY),
    computerUseVendorPackage: readJson(repoRoot, COMPUTER_USE_VENDOR_PACKAGE_ENTRY),
    computerUseVendorPluginManifest: readJson(repoRoot, COMPUTER_USE_VENDOR_PLUGIN_ENTRY),
    openClawShimPackage: readJson(repoRoot, OPENCLAW_SHIM_PACKAGE_ENTRY),
    openClawShimLicenseText: readText(repoRoot, OPENCLAW_SHIM_LICENSE_ENTRY),
    codeReuseProvenanceText: readText(repoRoot, CODE_REUSE_PROVENANCE_ENTRY),
    upstreamSourcesManifest: readJson(repoRoot, UPSTREAM_SOURCES_ENTRY),
    builderConfigs,
    productLicenseText: readText(repoRoot, PRODUCT_LICENSE_ENTRY),
    noticeText: readText(repoRoot, NOTICE_ENTRY)
  }
}

function normalizeBuilderConfigs(input) {
  if (Array.isArray(input.builderConfigs)) {
    return input.builderConfigs.map((entry, index) => ({
      artifactEntry: String(entry?.artifactEntry || `electron-builder.config.cjs#${index + 1}`),
      text: entry?.text === null || entry?.text === undefined ? null : String(entry.text)
    }))
  }
  if (input.builderConfigText !== undefined) {
    return [{
      artifactEntry: 'electron-builder.config.cjs',
      text: input.builderConfigText === null ? null : String(input.builderConfigText)
    }]
  }
  return []
}

function evaluatePlan(input = {}) {
  const blockers = []
  const obligations = []
  const packageJson = input.packageJson
  const packageLockJson = input.packageLockJson
  const runtimePackageJson = input.runtimePackageJson
  const runtimePackageLockJson = input.runtimePackageLockJson
  const fundsPluginManifest = input.fundsPluginManifest
  const computerUsePluginManifest = input.computerUsePluginManifest
  const computerUseVendorPackage = input.computerUseVendorPackage
  const computerUseVendorPluginManifest = input.computerUseVendorPluginManifest
  const openClawShimPackage = input.openClawShimPackage
  const openClawShimLicenseText = input.openClawShimLicenseText === null ||
    input.openClawShimLicenseText === undefined
    ? null
    : String(input.openClawShimLicenseText)
  const codeReuseProvenanceText = input.codeReuseProvenanceText === null ||
    input.codeReuseProvenanceText === undefined
    ? null
    : String(input.codeReuseProvenanceText)
  const upstreamSourcesManifest = input.upstreamSourcesManifest
  const builderConfigs = normalizeBuilderConfigs(input)
  const productLicenseText = input.productLicenseText === null || input.productLicenseText === undefined
    ? null
    : String(input.productLicenseText)
  const noticeText = input.noticeText === null || input.noticeText === undefined
    ? null
    : String(input.noticeText)

  function check({ id, artifactEntry, governingTerm, missingAction, passed, detail }) {
    const obligation = {
      id,
      artifactEntry,
      governingTerm,
      status: passed ? 'passed' : 'blocked'
    }
    if (detail) obligation.detail = detail
    obligations.push(obligation)
    if (!passed) {
      blockers.push(blocker({
        code: id,
        artifactEntry,
        governingTerm,
        missingAction,
        detail
      }))
    }
  }

  check({
    id: 'package-license-field',
    artifactEntry: `${ROOT_PACKAGE_ENTRY}:license`,
    governingTerm: 'The package metadata must declare the accepted Apache-2.0 product license governing the distributed package.',
    missingAction: `Set package.json license to ${PRODUCT_LICENSE_ID} and retain it in the package plan.`,
    passed: packageJson?.license === PRODUCT_LICENSE_ID,
    detail: packageJson === null || packageJson === undefined
      ? 'package.json is missing or invalid JSON'
      : packageJson.license === PRODUCT_LICENSE_ID
        ? undefined
        : `Expected ${PRODUCT_LICENSE_ID}; received ${String(packageJson.license || '<missing>')}`
  })
  check({
    id: 'product-identity-field',
    artifactEntry: `${ROOT_PACKAGE_ENTRY}:productName`,
    governingTerm: 'The package plan must identify the product to which its license metadata applies.',
    missingAction: 'Set a non-empty package.json productName field.',
    passed: hasNonEmptyString(packageJson?.productName),
    detail: packageJson === null || packageJson === undefined
      ? 'package.json is missing or invalid JSON'
      : undefined
  })
  check({
    id: 'package-author-identity',
    artifactEntry: `${ROOT_PACKAGE_ENTRY}:author`,
    governingTerm: 'The first-party package author must identify the current Analytix Project Owner without treating the contributor group as the owner.',
    missingAction: `Set package.json author to ${PRODUCT_PACKAGE_AUTHOR}.`,
    passed: hasExactPackageAuthor(packageJson?.author),
    detail: hasExactPackageAuthor(packageJson?.author)
      ? undefined
      : `Expected ${PRODUCT_PACKAGE_AUTHOR}; received ${String(packageJson?.author || '<missing>')}`
  })
  const rootLockPackage = packageLockJson?.packages?.['']
  check({
    id: 'package-lock-license-field',
    artifactEntry: `${ROOT_PACKAGE_LOCK_ENTRY}:packages[""]:license`,
    governingTerm: 'The root lockfile metadata must agree with the accepted Apache-2.0 product license.',
    missingAction: `Set the root package-lock metadata license to ${PRODUCT_LICENSE_ID}.`,
    passed: rootLockPackage?.license === PRODUCT_LICENSE_ID,
    detail: rootLockPackage?.license === PRODUCT_LICENSE_ID
      ? undefined
      : `Expected ${PRODUCT_LICENSE_ID}; received ${String(rootLockPackage?.license || '<missing>')}`
  })
  check({
    id: 'runtime-package-license-field',
    artifactEntry: `${RUNTIME_PACKAGE_ENTRY}:license`,
    governingTerm: 'The shipped public runtime package must use the accepted Apache-2.0 product license.',
    missingAction: `Set packages/runtime/package.json license to ${PRODUCT_LICENSE_ID}.`,
    passed: runtimePackageJson?.license === PRODUCT_LICENSE_ID,
    detail: runtimePackageJson === null || runtimePackageJson === undefined
      ? `${RUNTIME_PACKAGE_ENTRY} is missing or invalid JSON`
      : runtimePackageJson.license === PRODUCT_LICENSE_ID
        ? undefined
        : `Expected ${PRODUCT_LICENSE_ID}; received ${String(runtimePackageJson.license || '<missing>')}`
  })
  check({
    id: 'runtime-package-author-identity',
    artifactEntry: `${RUNTIME_PACKAGE_ENTRY}:author`,
    governingTerm: 'The shipped public runtime package must identify the current Analytix Project Owner.',
    missingAction: `Set ${RUNTIME_PACKAGE_ENTRY} author to ${PRODUCT_PACKAGE_AUTHOR}.`,
    passed: hasExactPackageAuthor(runtimePackageJson?.author),
    detail: hasExactPackageAuthor(runtimePackageJson?.author)
      ? undefined
      : `Expected ${PRODUCT_PACKAGE_AUTHOR}; received ${String(runtimePackageJson?.author || '<missing>')}`
  })
  const runtimeLockPackage = runtimePackageLockJson?.packages?.['']
  check({
    id: 'runtime-package-lock-license-field',
    artifactEntry: `${RUNTIME_PACKAGE_LOCK_ENTRY}:packages[""]:license`,
    governingTerm: 'The shipped runtime lockfile metadata must agree with the accepted Apache-2.0 runtime license.',
    missingAction: `Set the runtime package-lock metadata license to ${PRODUCT_LICENSE_ID}.`,
    passed: runtimeLockPackage?.license === PRODUCT_LICENSE_ID,
    detail: runtimeLockPackage?.license === PRODUCT_LICENSE_ID
      ? undefined
      : `Expected ${PRODUCT_LICENSE_ID}; received ${String(runtimeLockPackage?.license || '<missing>')}`
  })
  check({
    id: 'funds-plugin-license-field',
    artifactEntry: `${FUNDS_PLUGIN_ENTRY}:license`,
    governingTerm: 'The bundled first-party Funds plugin must use the accepted Apache-2.0 product license.',
    missingAction: `Set ${FUNDS_PLUGIN_ENTRY} license to ${PRODUCT_LICENSE_ID}.`,
    passed: fundsPluginManifest?.license === PRODUCT_LICENSE_ID,
    detail: fundsPluginManifest === null || fundsPluginManifest === undefined
      ? `${FUNDS_PLUGIN_ENTRY} is missing or invalid JSON`
      : fundsPluginManifest.license === PRODUCT_LICENSE_ID
        ? undefined
        : `Expected ${PRODUCT_LICENSE_ID}; received ${String(fundsPluginManifest.license || '<missing>')}`
  })
  check({
    id: 'funds-plugin-author-identity',
    artifactEntry: `${FUNDS_PLUGIN_ENTRY}:author`,
    governingTerm: 'The bundled first-party Funds plugin must identify the current Analytix Project Owner.',
    missingAction: `Set ${FUNDS_PLUGIN_ENTRY} author to the exact Guoqin He / Eysn0130 identity.`,
    passed: hasExactPluginAuthor(fundsPluginManifest?.author),
    detail: hasExactPluginAuthor(fundsPluginManifest?.author)
      ? undefined
      : `Expected ${JSON.stringify(PRODUCT_PLUGIN_AUTHOR)}; received ${JSON.stringify(fundsPluginManifest?.author ?? '<missing>')}`
  })
  check({
    id: 'computer-use-plugin-author-identity',
    artifactEntry: `${COMPUTER_USE_PLUGIN_ENTRY}:author`,
    governingTerm: 'The first-party Computer Use plugin manifest must identify the current Analytix Project Owner.',
    missingAction: `Set ${COMPUTER_USE_PLUGIN_ENTRY} author to the exact Guoqin He / Eysn0130 identity.`,
    passed: hasExactPluginAuthor(computerUsePluginManifest?.author),
    detail: hasExactPluginAuthor(computerUsePluginManifest?.author)
      ? undefined
      : `Expected ${JSON.stringify(PRODUCT_PLUGIN_AUTHOR)}; received ${JSON.stringify(computerUsePluginManifest?.author ?? '<missing>')}`
  })
  check({
    id: 'computer-use-vendor-package-author-identity',
    artifactEntry: `${COMPUTER_USE_VENDOR_PACKAGE_ENTRY}:author`,
    governingTerm: 'The first-party Computer Use distribution package must identify the current Analytix Project Owner while retaining its third-party MIT copyright.',
    missingAction: `Set ${COMPUTER_USE_VENDOR_PACKAGE_ENTRY} author to ${PRODUCT_PACKAGE_AUTHOR}.`,
    passed: hasExactPackageAuthor(computerUseVendorPackage?.author),
    detail: hasExactPackageAuthor(computerUseVendorPackage?.author)
      ? undefined
      : `Expected ${PRODUCT_PACKAGE_AUTHOR}; received ${String(computerUseVendorPackage?.author || '<missing>')}`
  })
  check({
    id: 'computer-use-vendor-plugin-author-identity',
    artifactEntry: `${COMPUTER_USE_VENDOR_PLUGIN_ENTRY}:author`,
    governingTerm: 'The packaged Computer Use plugin manifest must identify the current Analytix Project Owner.',
    missingAction: `Set ${COMPUTER_USE_VENDOR_PLUGIN_ENTRY} author to the exact Guoqin He / Eysn0130 identity.`,
    passed: hasExactPluginAuthor(computerUseVendorPluginManifest?.author),
    detail: hasExactPluginAuthor(computerUseVendorPluginManifest?.author)
      ? undefined
      : `Expected ${JSON.stringify(PRODUCT_PLUGIN_AUTHOR)}; received ${JSON.stringify(computerUseVendorPluginManifest?.author ?? '<missing>')}`
  })
  check({
    id: 'openclaw-shim-file-dependency',
    artifactEntry: `${ROOT_PACKAGE_ENTRY}:dependencies.openclaw`,
    governingTerm: 'The shipped OpenClaw compatibility package must remain bound to the reviewed vendored source and legal material.',
    missingAction: `Set package.json dependencies.openclaw to file:${OPENCLAW_SHIM_PACKAGE_ENTRY.replace('/package.json', '')}.`,
    passed: packageJson?.dependencies?.openclaw === 'file:vendor/openclaw-shim',
    detail: packageJson?.dependencies?.openclaw === 'file:vendor/openclaw-shim'
      ? undefined
      : `Expected file:vendor/openclaw-shim; received ${String(packageJson?.dependencies?.openclaw || '<missing>')}`
  })
  check({
    id: 'openclaw-shim-maintainer-identity',
    artifactEntry: `${OPENCLAW_SHIM_PACKAGE_ENTRY}:author/maintainers`,
    governingTerm: 'The local compatibility package must identify its current Analytix maintainer without replacing the upstream copyright holder.',
    missingAction: `Retain exact author and maintainer metadata for ${PRODUCT_PACKAGE_AUTHOR}.`,
    passed: hasExactOpenClawMaintainerMetadata(openClawShimPackage),
    detail: hasExactOpenClawMaintainerMetadata(openClawShimPackage)
      ? undefined
      : 'OpenClaw shim author or maintainers metadata is missing or variant'
  })
  check({
    id: 'openclaw-shim-upstream-provenance',
    artifactEntry: `${OPENCLAW_SHIM_PACKAGE_ENTRY}:contributors/analytixProvenance`,
    governingTerm: 'The local compatibility package must retain the exact OpenClaw contributor, annotated-tag, and peeled-commit provenance.',
    missingAction: 'Restore the exact OpenClaw v2026.5.18 contributor and provenance metadata.',
    passed: hasExactOpenClawUpstreamMetadata(openClawShimPackage),
    detail: hasExactOpenClawUpstreamMetadata(openClawShimPackage)
      ? undefined
      : 'OpenClaw shim contributor or immutable upstream provenance metadata is missing or variant'
  })
  check({
    id: 'openclaw-shim-license-field',
    artifactEntry: `${OPENCLAW_SHIM_PACKAGE_ENTRY}:license`,
    governingTerm: 'The OpenClaw-derived compatibility package is governed by the retained upstream MIT terms.',
    missingAction: `Set ${OPENCLAW_SHIM_PACKAGE_ENTRY} license to MIT.`,
    passed: openClawShimPackage?.license === 'MIT',
    detail: openClawShimPackage?.license === 'MIT'
      ? undefined
      : `Expected MIT; received ${String(openClawShimPackage?.license || '<missing>')}`
  })
  const openClawLicenseSha256 = openClawShimLicenseText === null
    ? ''
    : sha256Text(openClawShimLicenseText)
  check({
    id: 'openclaw-shim-license-content-integrity',
    artifactEntry: OPENCLAW_SHIM_LICENSE_ENTRY,
    governingTerm: 'The shipped shim must retain the byte-exact OpenClaw v2026.5.18 MIT license and Peter Steinberger notice.',
    missingAction: `Restore the exact upstream license at ${OPENCLAW_SHIM_LICENSE_ENTRY}.`,
    passed: openClawLicenseSha256 === OPENCLAW_UPSTREAM_LICENSE_SHA256,
    detail: openClawShimLicenseText === null
      ? `${OPENCLAW_SHIM_LICENSE_ENTRY} is missing or unreadable`
      : openClawLicenseSha256 === OPENCLAW_UPSTREAM_LICENSE_SHA256
        ? undefined
        : `Expected SHA-256 ${OPENCLAW_UPSTREAM_LICENSE_SHA256}; received ${openClawLicenseSha256}`
  })
  const lockOpenClawLink = packageLockJson?.packages?.['node_modules/openclaw']
  const lockOpenClawSource = packageLockJson?.packages?.['vendor/openclaw-shim']
  check({
    id: 'openclaw-shim-lock-metadata',
    artifactEntry: `${ROOT_PACKAGE_LOCK_ENTRY}:packages["vendor/openclaw-shim"]`,
    governingTerm: 'The root lockfile must preserve the reviewed file dependency and its MIT governing term.',
    missingAction: 'Restore the openclaw file link and vendor/openclaw-shim MIT metadata in package-lock.json.',
    passed: lockOpenClawLink?.resolved === 'vendor/openclaw-shim' &&
      lockOpenClawLink?.link === true &&
      lockOpenClawSource?.name === 'openclaw' &&
      lockOpenClawSource?.version === OPENCLAW_SHIM_VERSION &&
      lockOpenClawSource?.license === 'MIT',
    detail: lockOpenClawLink?.resolved === 'vendor/openclaw-shim' &&
      lockOpenClawLink?.link === true &&
      lockOpenClawSource?.name === 'openclaw' &&
      lockOpenClawSource?.version === OPENCLAW_SHIM_VERSION &&
      lockOpenClawSource?.license === 'MIT'
      ? undefined
      : 'OpenClaw file-link or source lock metadata is missing or drifted'
  })
  check({
    id: 'openclaw-shim-provenance-marker',
    artifactEntry: CODE_REUSE_PROVENANCE_ENTRY,
    governingTerm: 'The substantially adapted shim must remain bound to its exact source tag object, commit, license object, and destination.',
    missingAction: `Restore the exact OpenClaw provenance marker in ${CODE_REUSE_PROVENANCE_ENTRY}.`,
    passed: hasExactOpenClawProvenanceMarker(codeReuseProvenanceText),
    detail: hasExactOpenClawProvenanceMarker(codeReuseProvenanceText)
      ? undefined
      : 'OpenClaw provenance marker is missing, duplicated, malformed, or variant'
  })
  check({
    id: 'openclaw-upstream-registry-pin',
    artifactEntry: UPSTREAM_SOURCES_ENTRY,
    governingTerm: 'The artifact-entering OpenClaw source must remain registered at the exact peeled commit and license blob.',
    missingAction: `Restore the exact OpenClaw source entry in ${UPSTREAM_SOURCES_ENTRY}.`,
    passed: hasExactOpenClawRegistryEntry(upstreamSourcesManifest),
    detail: hasExactOpenClawRegistryEntry(upstreamSourcesManifest)
      ? undefined
      : 'OpenClaw upstream registry entry is missing, duplicated, or drifted'
  })
  const openClawLicenseConfig = builderConfigs.find((entry) =>
    configHasOpenClawLicenseEntry(entry.text)
  )
  check({
    id: 'openclaw-shim-license-package-mapping',
    artifactEntry: `${openClawLicenseConfig?.artifactEntry || 'electron-builder.config.cjs'}:files`,
    governingTerm: 'The production package plan must explicitly carry the nested OpenClaw MIT license with the shim.',
    missingAction: `Add ${PACKAGED_OPENCLAW_LICENSE_ENTRY} to the production files plan.`,
    passed: openClawLicenseConfig !== undefined,
    detail: openClawLicenseConfig
      ? undefined
      : `No builder configuration explicitly includes ${PACKAGED_OPENCLAW_LICENSE_ENTRY}`
  })

  const productLicenseConfig = builderConfigs.find((entry) => configHasProductLicenseMapping(entry.text))
  check({
    id: 'product-license-resource-mapping',
    artifactEntry: `${productLicenseConfig?.artifactEntry || 'electron-builder.config.cjs'}:extraResources`,
    governingTerm: 'The production package must carry the product license text declared by package metadata.',
    missingAction: 'Add a from/to LICENSE mapping to the production extraResources package plan.',
    passed: productLicenseConfig !== undefined,
    detail: productLicenseConfig ? undefined : 'No builder configuration maps LICENSE into extraResources'
  })

  for (const requiredText of REQUIRED_PRODUCT_LICENSE_TEXT) {
    check({
      id: 'product-license-text',
      artifactEntry: PRODUCT_LICENSE_ENTRY,
      governingTerm: 'The distributed Analytix artifact must retain its declared Apache-2.0 license and product copyright.',
      missingAction: `Restore the required product license text fragment: ${requiredText}`,
      passed: productLicenseText !== null && containsNoticeText(productLicenseText, requiredText),
      detail: productLicenseText === null ? `${PRODUCT_LICENSE_ENTRY} is missing or unreadable` : undefined
    })
  }
  const copyrightLines = productCopyrightLines(productLicenseText)
  check({
    id: 'product-copyright-identity-exact',
    artifactEntry: PRODUCT_LICENSE_ENTRY,
    governingTerm: 'The product LICENSE must contain exactly one current first-party copyright identity and no 2026 owner variant.',
    missingAction: `Retain exactly one line: ${PRODUCT_COPYRIGHT_NOTICE}`,
    passed: copyrightLines.length === 1 && copyrightLines[0] === PRODUCT_COPYRIGHT_NOTICE,
    detail: productLicenseText === null
      ? `${PRODUCT_LICENSE_ENTRY} is missing or unreadable`
      : copyrightLines.length === 1 && copyrightLines[0] === PRODUCT_COPYRIGHT_NOTICE
        ? undefined
        : `Expected one canonical line; received ${JSON.stringify(copyrightLines)}`
  })
  const productLicenseSha256 = productLicenseText === null ? '' : sha256Text(productLicenseText)
  check({
    id: 'product-license-content-integrity',
    artifactEntry: PRODUCT_LICENSE_ENTRY,
    governingTerm: 'The complete canonical Apache-2.0 product license and Analytix copyright must remain byte-exact.',
    missingAction: 'Restore the complete reviewed Analytix LICENSE file.',
    passed: productLicenseSha256 === REQUIRED_PRODUCT_LICENSE_SHA256,
    detail: productLicenseText === null
      ? `${PRODUCT_LICENSE_ENTRY} is missing or unreadable`
      : productLicenseSha256 === REQUIRED_PRODUCT_LICENSE_SHA256
        ? undefined
        : `Expected SHA-256 ${REQUIRED_PRODUCT_LICENSE_SHA256}; received ${productLicenseSha256}`
  })

  const mappingConfig = builderConfigs.find((entry) => configHasNoticeMapping(entry.text))
  check({
    id: 'third-party-notice-resource-mapping',
    artifactEntry: `${mappingConfig?.artifactEntry || 'electron-builder.config.cjs'}:extraResources`,
    governingTerm: 'The production electron-builder package plan must include THIRD_PARTY_NOTICES.md as a packaged resource.',
    missingAction: 'Add a from/to THIRD_PARTY_NOTICES.md mapping to the production extraResources package plan.',
    passed: mappingConfig !== undefined,
    detail: mappingConfig ? undefined : 'No builder configuration maps THIRD_PARTY_NOTICES.md into extraResources'
  })

  const excludedEntries = builderConfigs.flatMap((entry) =>
    explicitLicenseOrNoticeExclusions(entry.text).map((pattern) => ({
      artifactEntry: `${entry.artifactEntry}:files`,
      pattern
    }))
  )
  check({
    id: 'license-notice-not-explicitly-excluded',
    artifactEntry: excludedEntries[0]?.artifactEntry || 'electron-builder.config.cjs:files',
    governingTerm: 'Production packaging rules must not explicitly exclude LICENSE or NOTICE materials required by a packaged dependency.',
    missingAction: excludedEntries.length > 0
      ? `Remove explicit LICENSE/NOTICE exclusion pattern(s): ${excludedEntries.map(({ pattern }) => pattern).join(', ')}`
      : 'Keep LICENSE and NOTICE materials eligible for production packaging.',
    passed: excludedEntries.length === 0,
    detail: excludedEntries.length > 0 ? excludedEntries : undefined
  })

  const noticeSha256 = noticeText === null ? '' : sha256Text(noticeText)
  check({
    id: 'third-party-notice-content',
    artifactEntry: NOTICE_ENTRY,
    governingTerm: 'The applicable MIT license requires the complete copyright, permission, and warranty/disclaimer notice to accompany the covered material.',
    missingAction: 'Restore the complete legally reviewed third-party notice text required by the packaged material.',
    passed: noticeSha256 === REQUIRED_THIRD_PARTY_NOTICE_SHA256,
    detail: noticeText === null
      ? `${NOTICE_ENTRY} is missing or unreadable`
      : noticeSha256 === REQUIRED_THIRD_PARTY_NOTICE_SHA256
        ? undefined
        : `Expected SHA-256 ${REQUIRED_THIRD_PARTY_NOTICE_SHA256}; received ${noticeSha256}`
  })

  const sourcePackagePlanPassed = blockers.length === 0
  return {
    schemaVersion: 1,
    id: ARTIFACT_LEGAL_OBLIGATIONS_CONTRACT,
    status: sourcePackagePlanPassed ? 'passed' : 'blocked',
    passed: sourcePackagePlanPassed,
    claimCeiling: CLAIM_CEILING,
    artifactLegalPlanPassed: sourcePackagePlanPassed,
    sourcePackagePlan: {
      status: sourcePackagePlanPassed ? 'passed' : 'blocked',
      passed: sourcePackagePlanPassed,
      obligations,
      blockers
    }
  }
}

function normalizeEntry(entry) {
  const value = String(entry || '').replaceAll('\\', '/')
  const normalized = value.replace(/^\/+/, '').replace(/\/+/g, '/').replace(/\/$/, '')
  if (normalized.split('/').some((part) => part === '..' || part === '.')) {
    throw new Error(`exact_artifact_entry_path_invalid:${normalized}`)
  }
  return normalized
}

function asarHeader(buffer) {
  if (!Buffer.isBuffer(buffer) || buffer.length < 16) {
    throw new Error('exact_artifact_asar_header_unreadable')
  }
  const sizePicklePayloadLength = buffer.readUInt32LE(0)
  const headerPickleLength = buffer.readUInt32LE(4)
  if (sizePicklePayloadLength !== 4 || headerPickleLength < 8 ||
      8 + headerPickleLength > buffer.length) {
    throw new Error('exact_artifact_asar_header_size_invalid')
  }
  const headerPickle = buffer.subarray(8, 8 + headerPickleLength)
  const payloadLength = headerPickle.readUInt32LE(0)
  const jsonLength = headerPickle.readUInt32LE(4)
  if (payloadLength < jsonLength + 4 || jsonLength <= 0 || 8 + jsonLength > headerPickle.length) {
    throw new Error('exact_artifact_asar_header_pickle_invalid')
  }
  try {
    const header = JSON.parse(headerPickle.subarray(8, 8 + jsonLength).toString('utf8'))
    if (header && typeof header === 'object' && header.files && typeof header.files === 'object') {
      return { header, dataOffset: 8 + headerPickleLength }
    }
  } catch {
    // The caller receives one closed parse failure below.
  }
  throw new Error('exact_artifact_asar_header_invalid')
}

function flattenAsarFiles(files, prefix = '', output = new Map()) {
  for (const [name, node] of Object.entries(files || {}).sort(([left], [right]) => left.localeCompare(right))) {
    const entry = normalizeEntry(`${prefix}/${name}`)
    if (node && typeof node === 'object' && node.files && typeof node.files === 'object') {
      flattenAsarFiles(node.files, entry, output)
      continue
    }
    if (node && typeof node === 'object' && typeof node.link === 'string') {
      if (node.link === '' || node.size !== undefined || node.offset !== undefined ||
          node.files !== undefined || Object.keys(node).some((key) => !['link', 'unpacked'].includes(key))) {
        throw new Error(`exact_artifact_asar_link_invalid:${entry}`)
      }
      output.set(entry, {
        link: normalizeEntry(node.link),
        offset: 0,
        size: 0,
        unpacked: node.unpacked === true
      })
      continue
    }
    if (!node || typeof node !== 'object' || node.link || node.size === undefined ||
        (node.unpacked !== true && node.offset === undefined)) {
      throw new Error(`exact_artifact_asar_entry_invalid:${entry}`)
    }
    output.set(entry, {
      offset: node.unpacked === true ? 0 : Number(node.offset),
      size: Number(node.size),
      unpacked: node.unpacked === true
    })
  }
  return output
}

function createMemoryReader(entries, label = 'memory-exact-artifact') {
  const map = new Map()
  for (const [entry, value] of Object.entries(entries || {})) {
    map.set(normalizeEntry(entry), Buffer.isBuffer(value) ? value : Buffer.from(String(value), 'utf8'))
  }
  return {
    label,
    exact: true,
    sha256: sha256Bytes(Buffer.concat([...map.entries()].sort(([left], [right]) => left.localeCompare(right)).flatMap(([entry, value]) => [Buffer.from(entry), value]))),
    byteLength: [...map.values()].reduce((total, value) => total + value.length, 0),
    entries: () => [...map.keys()].sort(),
    has: (entry) => map.has(normalizeEntry(entry)),
    read: (entry) => map.get(normalizeEntry(entry)) || null
  }
}

function filterPhysicalAsarFiles(files, readableUnpackedEntry) {
  const output = new Map()
  for (const [entry, descriptor] of files) {
    if (descriptor.link || descriptor.unpacked !== true || readableUnpackedEntry(entry)) {
      output.set(entry, descriptor)
    }
  }
  return output
}

function stableStatIdentity(stat) {
  return {
    dev: String(stat.dev),
    ino: String(stat.ino),
    mode: String(stat.mode),
    size: String(stat.size),
    mtimeNs: String(stat.mtimeNs),
    ctimeNs: String(stat.ctimeNs),
    nlink: String(stat.nlink)
  }
}

function sameStableStatIdentity(left, right) {
  return left.dev === right.dev && left.ino === right.ino &&
    left.mode === right.mode && left.size === right.size &&
    left.mtimeNs === right.mtimeNs && left.ctimeNs === right.ctimeNs &&
    left.nlink === right.nlink
}

function assertStableSingleLinkRegularFilePath(path, expected) {
  try {
    const before = lstatSync(path, { bigint: true })
    if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1n) {
      throw new Error('identity_invalid')
    }
    const currentRealPath = realpathSync(path)
    const after = lstatSync(path, { bigint: true })
    if (!after.isFile() || after.isSymbolicLink() || after.nlink !== 1n ||
        currentRealPath !== expected.realPath ||
        !sameStableStatIdentity(stableStatIdentity(before), stableStatIdentity(after)) ||
        !sameStableStatIdentity(expected.identity, stableStatIdentity(after))) {
      throw new Error('identity_changed')
    }
  } catch {
    throw new Error(`exact_artifact_regular_file_path_changed_after_read:${path}`)
  }
}

function readStableSingleLinkRegularFile(path, options = {}) {
  const maximumBytes = Number.isSafeInteger(options.maximumBytes) && options.maximumBytes >= 0
    ? options.maximumBytes
    : Number.MAX_SAFE_INTEGER
  const before = lstatSync(path, { bigint: true })
  if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1n ||
      before.size < 0n || before.size > BigInt(maximumBytes)) {
    throw new Error(`exact_artifact_regular_file_identity_invalid:${path}`)
  }
  const noFollow = typeof constants.O_NOFOLLOW === 'number' ? constants.O_NOFOLLOW : 0
  const fd = openSync(path, constants.O_RDONLY | noFollow)
  try {
    const opened = fstatSync(fd, { bigint: true })
    if (!opened.isFile() || opened.nlink !== 1n ||
        !sameStableStatIdentity(stableStatIdentity(before), stableStatIdentity(opened))) {
      throw new Error(`exact_artifact_regular_file_changed_before_read:${path}`)
    }
    const value = readFileSync(fd)
    options.afterRead?.({ path, byteLength: value.length })
    const afterFd = fstatSync(fd, { bigint: true })
    const afterPath = lstatSync(path, { bigint: true })
    const afterRealPath = realpathSync(path)
    const finalPath = lstatSync(path, { bigint: true })
    if (afterFd.nlink !== 1n || afterPath.nlink !== 1n ||
        !sameStableStatIdentity(stableStatIdentity(opened), stableStatIdentity(afterFd)) ||
        !sameStableStatIdentity(stableStatIdentity(afterFd), stableStatIdentity(afterPath)) ||
        !sameStableStatIdentity(stableStatIdentity(afterPath), stableStatIdentity(finalPath)) ||
        BigInt(value.length) !== afterFd.size) {
      throw new Error(`exact_artifact_regular_file_changed_during_read:${path}`)
    }
    return options.returnStableIdentity === true
      ? {
          value,
          identity: stableStatIdentity(afterFd),
          realPath: afterRealPath
        }
      : value
  } finally {
    closeSync(fd)
  }
}

function createAsarReaderFromBuffer(buffer, label, readUnpackedEntry = () => null) {
  const { header, dataOffset } = asarHeader(buffer)
  const allFiles = flattenAsarFiles(header.files)
  const unpacked = new Map()
  const readableUnpackedEntry = (entry) => {
    const value = readUnpackedEntry(entry)
    if (!Buffer.isBuffer(value)) return false
    unpacked.set(entry, value)
    return true
  }
  const files = filterPhysicalAsarFiles(allFiles, readableUnpackedEntry)
  return {
    label,
    exact: true,
    sha256: sha256Bytes(buffer),
    byteLength: buffer.length,
    entries: () => [...files.keys()].sort(),
    has: (entry) => files.has(normalizeEntry(entry)),
    read: (entry) => {
      const key = normalizeEntry(entry)
      const descriptor = files.get(key)
      if (!descriptor) return null
      if (descriptor.link) return null
      if (descriptor.unpacked) return unpacked.get(key) || null
      const start = dataOffset + descriptor.offset
      const end = start + descriptor.size
      if (!Number.isSafeInteger(start) || !Number.isSafeInteger(end) || start < dataOffset || end > buffer.length) return null
      return buffer.subarray(start, end)
    }
  }
}

function createAsarReader(asarPath, unpackedRoot = null, options = {}) {
  try {
    const stableFile = readStableSingleLinkRegularFile(asarPath, {
      returnStableIdentity: true
    })
    const buffer = stableFile.value
    options.afterStableRead?.({ path: asarPath, byteLength: buffer.length })
    let unpackedReader = null
    if (unpackedRoot) {
      let unpackedStat = null
      try {
        unpackedStat = lstatSync(unpackedRoot)
      } catch (error) {
        if (error?.code !== 'ENOENT') {
          throw new Error(`exact_artifact_unpack_root_unreadable:${unpackedRoot}`)
        }
      }
      if (unpackedStat) {
        if (!unpackedStat.isDirectory() || unpackedStat.isSymbolicLink()) {
          throw new Error(`exact_artifact_unpack_root_unsafe:${unpackedRoot}`)
        }
        unpackedReader = createDirectoryReader(unpackedRoot)
      }
    }
    const reader = createAsarReaderFromBuffer(
      buffer,
      asarPath,
      (entry) => unpackedReader?.read(entry) || null
    )
    return {
      ...reader,
      assertStable: () => {
        assertStableSingleLinkRegularFilePath(asarPath, stableFile)
        unpackedReader?.assertStable()
      }
    }
  } catch (error) {
    throw new Error(`exact_artifact_asar_unreadable:${error?.message || 'read_failed'}`)
  }
}

function pathContainedBy(root, candidate) {
  const path = relative(root, candidate)
  return path !== '' && path !== '..' && !path.startsWith(`..${sep}`) && !isAbsolute(path)
}

function resolveContainedSymlinkTarget(root, path, target) {
  if (typeof target !== 'string' || target === '' || target.includes('\0') || isAbsolute(target)) {
    throw new Error(`exact_artifact_directory_symlink_target_invalid:${relative(root, path)}`)
  }
  const resolved = resolve(dirname(path), target)
  if (!pathContainedBy(root, resolved)) {
    throw new Error(`exact_artifact_directory_symlink_escape:${relative(root, path)}`)
  }
  return resolved
}

function captureDirectoryInventory(root, current = root, entries = new Map()) {
  const currentBefore = lstatSync(current, { bigint: true })
  if (!currentBefore.isDirectory() || currentBefore.isSymbolicLink()) {
    throw new Error(`exact_artifact_directory_identity_invalid:${relative(root, current) || '.'}`)
  }
  const items = readdirSync(current, { withFileTypes: true })
    .sort((left, right) => left.name.localeCompare(right.name))
  for (const item of items) {
    const path = join(current, item.name)
    const entry = normalizeEntry(relative(root, path))
    const before = lstatSync(path, { bigint: true })
    if (item.isSymbolicLink() && before.isSymbolicLink()) {
      const target = readlinkSync(path)
      resolveContainedSymlinkTarget(root, path, target)
      let realTarget
      try {
        realTarget = realpathSync(path)
      } catch {
        throw new Error(`exact_artifact_directory_symlink_broken:${entry}`)
      }
      if (!pathContainedBy(root, realTarget)) {
        throw new Error(`exact_artifact_directory_symlink_escape:${entry}`)
      }
      const after = lstatSync(path, { bigint: true })
      if (!after.isSymbolicLink() || target !== readlinkSync(path) ||
          !sameStableStatIdentity(stableStatIdentity(before), stableStatIdentity(after))) {
        throw new Error(`exact_artifact_directory_symlink_changed:${entry}`)
      }
      entries.set(entry, {
        kind: 'symlink',
        target,
        identity: stableStatIdentity(after)
      })
      continue
    }
    if (item.isDirectory() && before.isDirectory()) {
      captureDirectoryInventory(root, path, entries)
      continue
    }
    if (item.isFile() && before.isFile()) {
      if (before.nlink !== 1n) {
        throw new Error(`exact_artifact_directory_file_hardlink_rejected:${entry}`)
      }
      entries.set(entry, { kind: 'file', identity: stableStatIdentity(before) })
      continue
    }
    throw new Error(`exact_artifact_directory_entry_changed_or_unsupported:${entry}`)
  }
  const currentAfter = lstatSync(current, { bigint: true })
  if (!currentAfter.isDirectory() || currentAfter.isSymbolicLink() ||
      !sameStableStatIdentity(
        stableStatIdentity(currentBefore),
        stableStatIdentity(currentAfter)
      )) {
    throw new Error(`exact_artifact_directory_changed_during_inventory:${relative(root, current) || '.'}`)
  }
  entries.set(normalizeEntry(relative(root, current)) || '.', {
    kind: 'directory',
    identity: stableStatIdentity(currentAfter)
  })
  return entries
}

function serializedDirectoryInventory(entries) {
  return JSON.stringify([...entries.entries()].sort(([left], [right]) =>
    left.localeCompare(right)))
}

function createDirectoryReader(root, options = {}) {
  const inventory = captureDirectoryInventory(root)
  options.afterInitialInventory?.({ root })
  const entries = new Map([...inventory].filter(([, descriptor]) =>
    descriptor.kind !== 'directory'))
  const digest = createHash('sha256')
  let byteLength = 0
  for (const entry of [...entries.keys()].sort()) {
    const descriptor = entries.get(entry)
    digest.update(entry, 'utf8')
    digest.update('\0')
    digest.update(descriptor.kind, 'utf8')
    digest.update('\0')
    if (descriptor.kind === 'symlink') {
      digest.update(descriptor.target, 'utf8')
      digest.update('\0')
      byteLength += Buffer.byteLength(descriptor.target, 'utf8')
      continue
    }
    const value = readStableSingleLinkRegularFile(join(root, entry), {
      afterRead: options.afterFileRead
    })
    digest.update(value)
    digest.update('\0')
    byteLength += value.length
  }
  const assertStable = () => {
    options.beforeStableAssertion?.({ root })
    const observed = captureDirectoryInventory(root)
    if (serializedDirectoryInventory(observed) !== serializedDirectoryInventory(inventory)) {
      throw new Error('exact_artifact_directory_inventory_changed_during_inspection')
    }
  }
  assertStable()
  return {
    label: root,
    exact: true,
    sha256: digest.digest('hex'),
    byteLength,
    entries: () => [...entries.keys()].sort(),
    has: (entry) => entries.has(normalizeEntry(entry)),
    read: (entry) => {
      const key = normalizeEntry(entry)
      if (entries.get(key)?.kind !== 'file') return null
      try {
        return readStableSingleLinkRegularFile(join(root, key))
      } catch {
        return null
      }
    },
    assertStable
  }
}

function createPackagedAppReader(root, asarPath) {
  const packagedRoot = createDirectoryReader(root)
  const asarEntry = normalizeEntry(relative(root, asarPath))
  const unpackedPrefix = `${asarEntry}.unpacked/`
  const asarBytes = packagedRoot.read(asarEntry)
  if (!asarBytes) throw new Error('exact_artifact_asar_unreadable_from_packaged_inventory')
  const asar = createAsarReaderFromBuffer(
    asarBytes,
    asarPath,
    (entry) => packagedRoot.read(`${unpackedPrefix}${entry}`)
  )
  const rootPrefix = 'packaged-root/'
  // after-pack stages additional runtime files in app.asar.unpacked. They are
  // shipped bytes even when no ASAR entry names them. Keep a physical namespace
  // so they cannot shadow an indexed ASAR entry or borrow its license identity.
  const packagedEntries = packagedRoot.entries().filter((entry) =>
    entry !== asarEntry && (!entry.startsWith(unpackedPrefix) ||
      !asar.has(entry.slice(unpackedPrefix.length)))
  )
  const entries = [
    ...asar.entries(),
    ...packagedEntries.map((entry) => `${rootPrefix}${entry}`)
  ].sort()
  return {
    label: root,
    exact: true,
    sha256: packagedRoot.sha256,
    byteLength: packagedRoot.byteLength,
    components: [{
      artifactEntry: asarEntry,
      sha256: asar.sha256,
      byteLength: asar.byteLength
    }],
    entries: () => entries,
    has: (entry) => {
      const key = normalizeEntry(entry)
      return key.startsWith(rootPrefix)
        ? packagedRoot.has(key.slice(rootPrefix.length))
        : asar.has(key)
    },
    read: (entry) => {
      const key = normalizeEntry(entry)
      return key.startsWith(rootPrefix)
        ? packagedRoot.read(key.slice(rootPrefix.length))
        : asar.read(key)
    },
    assertStable: () => packagedRoot.assertStable()
  }
}

export function createArtifactReaderFromPath(inputPath) {
  const path = resolve(String(inputPath || ''))
  let stat
  try {
    stat = lstatSync(path)
  } catch (error) {
    throw new Error(`exact_artifact_input_missing:${error?.message || 'not_found'}`)
  }
  if (stat.isSymbolicLink()) throw new Error('exact_artifact_input_symlink')
  if (stat.isFile()) {
    if (extname(path).toLowerCase() !== '.asar' && !path.endsWith('app.asar')) {
      throw new Error('exact_artifact_archive_format_unsupported')
    }
    return createAsarReader(path, `${path}.unpacked`)
  }
  if (!stat.isDirectory()) throw new Error('exact_artifact_input_not_directory_or_asar')

  const candidates = [
    join(path, 'Contents', 'Resources', 'app.asar'),
    join(path, 'resources', 'app.asar'),
    join(path, 'app.asar')
  ]
  const asarPath = candidates.find((candidate) => {
    try {
      const candidateStat = lstatSync(candidate)
      return candidateStat.isFile() && !candidateStat.isSymbolicLink()
    } catch {
      return false
    }
  })
  if (asarPath) return createPackagedAppReader(path, asarPath)
  return createDirectoryReader(path)
}

function createExactArtifactReader(input) {
  if (input?.read && input?.entries && input?.has) return input
  if (input?.entries && typeof input.entries === 'object') return createMemoryReader(input.entries, input.label)
  if (typeof input === 'string') return createArtifactReaderFromPath(input)
  throw new Error('exact_artifact_input_missing')
}

function licenseValue(packageJson) {
  if (hasNonEmptyString(packageJson?.license)) return packageJson.license.trim()
  if (packageJson?.license && typeof packageJson.license === 'object') {
    if (hasNonEmptyString(packageJson.license.type)) return packageJson.license.type.trim()
    if (hasNonEmptyString(packageJson.license.name)) return packageJson.license.name.trim()
  }
  if (Array.isArray(packageJson?.licenses)) {
    const values = packageJson.licenses
      .map((item) => typeof item === 'string' ? item : item?.type)
      .filter(hasNonEmptyString)
    if (values.length > 0) return values.join(' OR ')
  }
  return ''
}

function isInternalPackage(name, options = {}) {
  const explicit = new Set(options.internalPackageNames || [])
  const prefixes = options.internalPackagePrefixes || []
  return explicit.has(name) || prefixes.some((prefix) => name.startsWith(prefix)) ||
    INTERNAL_PACKAGE_NAME_RE.test(name)
}

function dependencySource(name, version, internal) {
  const locator = `${name || 'unknown'}@${version || 'unknown'}`
  return internal ? `artifact-internal:${locator}` : `npm:${locator}`
}

function findLegalEntry(reader, packageDirectory, names) {
  const entries = reader.entries()
  for (const name of names) {
    const candidate = normalizeEntry(`${packageDirectory}/${name}`)
    if (entries.includes(candidate) && reader.read(candidate)?.toString('utf8').trim()) return `/${candidate}`
  }
  const lower = new Map(entries.map((entry) => [entry.toLowerCase(), entry]))
  for (const name of names) {
    const candidate = normalizeEntry(`${packageDirectory}/${name}`).toLowerCase()
    if (lower.has(candidate) && reader.read(lower.get(candidate))?.toString('utf8').trim()) return `/${lower.get(candidate)}`
  }
  return null
}

function dependencyRecord({ reader, artifactEntry, packageJson, options = {} }) {
  const entry = normalizeEntry(artifactEntry)
  const packageDirectory = entry.slice(0, -'/package.json'.length)
  const name = hasNonEmptyString(packageJson?.name) ? packageJson.name.trim() : ''
  const version = hasNonEmptyString(packageJson?.version) ? packageJson.version.trim() : ''
  const declaredLicense = licenseValue(packageJson)
  const internal = isInternalPackage(name, options)
  const source = dependencySource(name, version, internal)
  let licenseFile = findLegalEntry(reader, packageDirectory, LICENSE_FILE_NAMES)
  const noticeFile = findLegalEntry(reader, packageDirectory, NOTICE_FILE_NAMES)
  let obligationClass = internal
    ? ARTIFACT_OBLIGATION_CLASSES.internalMetadata
    : ARTIFACT_OBLIGATION_CLASSES.mandatoryExternal
  let status = 'passed'
  let missingAction = /\sOR\s/i.test(declaredLicense)
    ? 'Preserve the complete declared license choice and its texts; this gate does not silently select a governing branch.'
    : 'none; exact artifact license and applicable notice material are present'
  let governingTerm = declaredLicense || 'Declared license metadata is absent; the applicable redistribution term cannot be established.'
  let supplementalEvidence = null

  if (!declaredLicense) {
    status = internal ? 'unverified' : 'blocked'
    missingAction = internal
      ? 'Classify the internal package ownership and governing distribution term in exact package metadata.'
      : 'Provide exact artifact package license metadata and retain the applicable license text.'
  } else if (COMMERCIAL_LICENSE_RE.test(declaredLicense)) {
    obligationClass = ARTIFACT_OBLIGATION_CLASSES.commercialLicenseDecision
    status = 'unverified'
    missingAction = 'Make the explicit commercial license decision; do not infer a commercial grant from package metadata.'
    governingTerm = declaredLicense
  } else if (!licenseFile) {
    status = 'blocked'
    missingAction = 'Retain the exact package license file in the final artifact and bind it to this dependency instance.'
  }

  const catalog = options.legalCatalog
  const record = catalog?.catalog.packages.find(record => {
    if (record.name === name && record.version === version) return true
    try {
      const logical = logicalEntry(reader, entry)
      return record.bindings.some(binding =>
        `${binding.lock === 'package-lock.json' ? '' : 'packages/runtime/'}${binding.path}/package.json` === logical)
    } catch { return false }
  })
  if (record) {
    try {
      supplementalEvidence = verifyEvidence(reader, entry, record, catalog)
      governingTerm = supplementalEvidence.governingExpression
      licenseFile = supplementalEvidence.licenseFile
      status = 'passed'
      missingAction = 'none; exact locked instance and supplemental legal materials verified'
    } catch (error) {
      status = 'blocked'
      missingAction = /^dependency_legal_[a-z_]+$/.test(error.message)
        ? error.message : 'dependency_legal_evidence_unreadable'
    }
  }

  return {
    artifactEntry: `/${entry}`,
    name,
    version,
    source,
    packageClass: internal ? 'internal' : 'external',
    governingTerm,
    licenseFile,
    noticeFile,
    missingAction,
    obligationClass,
    classification: obligationClass,
    mandatory: obligationClass === ARTIFACT_OBLIGATION_CLASSES.mandatoryExternal,
    engineeringBlocking: status === 'blocked' && obligationClass === ARTIFACT_OBLIGATION_CLASSES.mandatoryExternal,
    status,
    declaredLicense: declaredLicense || null,
    supplementalEvidence
  }
}

function decisionRecord({ artifactEntry, name, version = '', source, governingTerm, missingAction, obligationClass }) {
  return {
    artifactEntry: artifactEntry.startsWith('/') ? artifactEntry : `/${artifactEntry}`,
    name,
    version,
    source,
    governingTerm,
    licenseFile: null,
    noticeFile: null,
    missingAction,
    obligationClass,
    classification: obligationClass,
    mandatory: false,
    engineeringBlocking: false,
    status: 'unverified'
  }
}

function exactPackageEntries(reader) {
  return reader.entries()
    .filter((entry) => {
      const parts = entry.split('/')
      const nodeModulesIndex = parts.lastIndexOf('node_modules')
      if (nodeModulesIndex < 0) return false
      const packagePath = parts.slice(nodeModulesIndex + 1)
      return (packagePath.length === 2 && packagePath[1] === 'package.json') ||
        (packagePath.length === 3 && packagePath[0].startsWith('@') &&
          packagePath[2] === 'package.json')
    })
    .sort()
}

function readPackage(reader, entry) {
  const bytes = reader.read(entry)
  if (!bytes) return null
  try {
    const value = JSON.parse(bytes.toString('utf8'))
    return value && typeof value === 'object' && !Array.isArray(value) ? value : null
  } catch {
    return null
  }
}

function exactEntryForResource(reader, name) {
  const suffixes = [
    name,
    `packaged-root/Contents/Resources/${name}`,
    `packaged-root/resources/${name}`,
    `packaged-root/Resources/${name}`
  ]
  return suffixes.find((entry) => reader.has(entry)) || null
}

function readTextEntry(reader, entry) {
  const bytes = entry ? reader.read(entry) : null
  return bytes ? bytes.toString('utf8') : null
}

function packageEntryEndingWith(reader, suffix) {
  const normalizedSuffix = normalizeEntry(suffix)
  return reader.entries().find((entry) =>
    entry === normalizedSuffix || entry.endsWith(`/${normalizedSuffix}`)
  ) || null
}

function runtimeMetadataEntry(reader, logicalEntry) {
  if (reader.has(logicalEntry)) return logicalEntry
  // Only the actual ASAR's after-pack directory can supply unindexed runtime
  // metadata. An unrelated suffix match must not repair a missing owner file.
  for (const component of reader.components || []) {
    const candidate = normalizeEntry(`packaged-root/${component.artifactEntry}.unpacked/${logicalEntry}`)
    if (reader.has(candidate)) return candidate
  }
  return null
}

function exactInventory(reader, options = {}) {
  options = { ...options, legalCatalog: loadCatalog(REPO_ROOT) }
  const entries = exactPackageEntries(reader)
  const dependencyInstances = []
  const parseErrors = []
  for (const entry of entries) {
    const packageJson = readPackage(reader, entry)
    if (!packageJson) {
      parseErrors.push({ artifactEntry: `/${entry}`, reason: 'package_json_unreadable_or_invalid' })
      continue
    }
    dependencyInstances.push(dependencyRecord({ reader, artifactEntry: entry, packageJson, options }))
  }

  const productPackage = readPackage(reader, 'package.json')
  if (!productPackage || productPackage.name !== 'analytix') {
    parseErrors.push({
      artifactEntry: '/package.json',
      reason: productPackage ? 'product_identity_is_not_analytix' : 'product_package_json_unreadable_or_missing'
    })
  }
  if (entries.length === 0) {
    parseErrors.push({
      artifactEntry: '/node_modules',
      reason: 'dependency_instance_inventory_empty'
    })
  }

  const productLicenseEntry = exactEntryForResource(reader, PRODUCT_LICENSE_ENTRY)
  const exactProductLicenseText = readTextEntry(reader, productLicenseEntry)
  const noticeEntry = exactEntryForResource(reader, NOTICE_ENTRY)
  const exactNoticeText = readTextEntry(reader, noticeEntry)
  const artifactResourceBlockers = []
  if (productPackage && licenseValue(productPackage) !== PRODUCT_LICENSE_ID) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_PRODUCT_LICENSE_METADATA_MISMATCH',
      artifactEntry: '/package.json:license',
      governingTerm: 'The exact packaged product metadata must declare the accepted Apache-2.0 license.',
      missingAction: `Set the exact packaged product license metadata to ${PRODUCT_LICENSE_ID}.`
    })
  }
  if (!productPackage || !hasExactPackageAuthor(productPackage.author)) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_PRODUCT_AUTHOR_IDENTITY_MISSING_OR_MISMATCHED',
      artifactEntry: '/package.json:author',
      governingTerm: 'The exact packaged product metadata must identify the current Analytix Project Owner.',
      missingAction: `Set the exact packaged product author metadata to ${PRODUCT_PACKAGE_AUTHOR}.`
    })
  }
  const runtimePackageEntry = runtimeMetadataEntry(reader, RUNTIME_PACKAGE_ENTRY)
  const runtimePackage = runtimePackageEntry ? readPackage(reader, runtimePackageEntry) : null
  if (!runtimePackage || licenseValue(runtimePackage) !== PRODUCT_LICENSE_ID) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_RUNTIME_LICENSE_METADATA_MISSING_OR_MISMATCHED',
      artifactEntry: runtimePackageEntry
        ? `/${runtimePackageEntry}:license`
        : `/<missing-${RUNTIME_PACKAGE_ENTRY}>`,
      governingTerm: 'The exact packaged public runtime metadata must declare the accepted Apache-2.0 license.',
      missingAction: `Retain readable ${RUNTIME_PACKAGE_ENTRY} metadata with license ${PRODUCT_LICENSE_ID}.`
    })
  }
  if (!runtimePackage || !hasExactPackageAuthor(runtimePackage.author)) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_RUNTIME_AUTHOR_IDENTITY_MISSING_OR_MISMATCHED',
      artifactEntry: runtimePackageEntry
        ? `/${runtimePackageEntry}:author`
        : `/<missing-${RUNTIME_PACKAGE_ENTRY}>`,
      governingTerm: 'The exact packaged public runtime metadata must identify the current Analytix Project Owner.',
      missingAction: `Retain readable ${RUNTIME_PACKAGE_ENTRY} metadata with author ${PRODUCT_PACKAGE_AUTHOR}.`
    })
  }
  const runtimePackageLockEntry = runtimeMetadataEntry(reader, RUNTIME_PACKAGE_LOCK_ENTRY)
  const runtimePackageLock = runtimePackageLockEntry ? readPackage(reader, runtimePackageLockEntry) : null
  const exactRuntimeLockPackage = runtimePackageLock?.packages?.['']
  if (exactRuntimeLockPackage?.license !== PRODUCT_LICENSE_ID) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_RUNTIME_LOCK_LICENSE_METADATA_MISSING_OR_MISMATCHED',
      artifactEntry: runtimePackageLockEntry
        ? `/${runtimePackageLockEntry}:packages[""]:license`
        : `/<missing-${RUNTIME_PACKAGE_LOCK_ENTRY}>`,
      governingTerm: 'The exact packaged runtime lockfile metadata must agree with the accepted Apache-2.0 runtime license.',
      missingAction: `Retain readable ${RUNTIME_PACKAGE_LOCK_ENTRY} metadata with root license ${PRODUCT_LICENSE_ID}.`
    })
  }
  if (!productLicenseEntry || sha256Text(exactProductLicenseText) !== REQUIRED_PRODUCT_LICENSE_SHA256) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_PRODUCT_LICENSE_MISSING_OR_DRIFTED',
      artifactEntry: productLicenseEntry ? `/${productLicenseEntry}` : `/<missing-${PRODUCT_LICENSE_ENTRY}>`,
      governingTerm: 'The exact packaged product must retain the declared Analytix license and required notice.',
      missingAction: 'Retain the exact reviewed LICENSE resource in the final packaged artifact.'
    })
  }
  if (!noticeEntry || sha256Text(exactNoticeText) !== REQUIRED_THIRD_PARTY_NOTICE_SHA256) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_THIRD_PARTY_NOTICE_MISSING_OR_DRIFTED',
      artifactEntry: noticeEntry ? `/${noticeEntry}` : `/<missing-${NOTICE_ENTRY}>`,
      governingTerm: 'The exact packaged product must retain the reviewed third-party notice material.',
      missingAction: 'Retain the exact reviewed THIRD_PARTY_NOTICES.md resource in the final packaged artifact.'
    })
  }

  const commercialLicenseDecisions = dependencyInstances.filter((entry) =>
    entry.obligationClass === ARTIFACT_OBLIGATION_CLASSES.commercialLicenseDecision
  )
  if (productPackage && COMMERCIAL_LICENSE_RE.test(licenseValue(productPackage))) {
    commercialLicenseDecisions.push(decisionRecord({
      artifactEntry: 'package.json',
      name: productPackage.name || 'analytix',
      version: productPackage.version || '',
      source: 'product',
      governingTerm: licenseValue(productPackage),
      missingAction: 'Make the explicit commercial license decision for product distribution; no commercial grant is inferred.',
      obligationClass: ARTIFACT_OBLIGATION_CLASSES.commercialLicenseDecision
    }))
  }
  const pluginEntry = packageEntryEndingWith(
    reader, 'plugins/analytix-fund-analysis/.codex-plugin/plugin.json'
  )
  const plugin = pluginEntry ? readPackage(reader, pluginEntry) : null
  if (plugin && (licenseValue(plugin) || String(plugin.license || '')) !== PRODUCT_LICENSE_ID) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_FUNDS_PLUGIN_LICENSE_METADATA_MISMATCH',
      artifactEntry: `/${pluginEntry}:license`,
      governingTerm: 'The bundled first-party Funds plugin must declare the accepted Apache-2.0 license.',
      missingAction: `Set the bundled Funds plugin license metadata to ${PRODUCT_LICENSE_ID}.`
    })
  }
  if (plugin && !hasExactPluginAuthor(plugin.author)) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_FUNDS_PLUGIN_AUTHOR_IDENTITY_MISSING_OR_MISMATCHED',
      artifactEntry: `/${pluginEntry}:author`,
      governingTerm: 'The bundled first-party Funds plugin must identify the current Analytix Project Owner.',
      missingAction: 'Retain the exact Guoqin He / Eysn0130 author identity in the bundled Funds plugin manifest.'
    })
  }
  const computerUsePackageEntry = packageEntryEndingWith(reader, PACKAGED_COMPUTER_USE_PACKAGE_ENTRY)
  const computerUsePackage = computerUsePackageEntry
    ? readPackage(reader, computerUsePackageEntry)
    : null
  if (!computerUsePackage || !hasExactPackageAuthor(computerUsePackage.author)) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_COMPUTER_USE_PACKAGE_AUTHOR_IDENTITY_MISSING_OR_MISMATCHED',
      artifactEntry: computerUsePackageEntry
        ? `/${computerUsePackageEntry}:author`
        : `/<missing-${PACKAGED_COMPUTER_USE_PACKAGE_ENTRY}>`,
      governingTerm: 'The exact Computer Use distribution package must identify the current Analytix Project Owner.',
      missingAction: `Retain ${PRODUCT_PACKAGE_AUTHOR} in the packaged Computer Use package metadata.`
    })
  }
  const computerUsePluginEntry = packageEntryEndingWith(reader, PACKAGED_COMPUTER_USE_PLUGIN_ENTRY)
  const computerUsePlugin = computerUsePluginEntry
    ? readPackage(reader, computerUsePluginEntry)
    : null
  if (!computerUsePlugin || !hasExactPluginAuthor(computerUsePlugin.author)) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_COMPUTER_USE_PLUGIN_AUTHOR_IDENTITY_MISSING_OR_MISMATCHED',
      artifactEntry: computerUsePluginEntry
        ? `/${computerUsePluginEntry}:author`
        : `/<missing-${PACKAGED_COMPUTER_USE_PLUGIN_ENTRY}>`,
      governingTerm: 'The packaged Computer Use plugin manifest must identify the current Analytix Project Owner.',
      missingAction: 'Retain the exact Guoqin He / Eysn0130 author identity in the packaged Computer Use plugin manifest.'
    })
  }
  const openClawPackageEntry = reader.has(PACKAGED_OPENCLAW_PACKAGE_ENTRY)
    ? PACKAGED_OPENCLAW_PACKAGE_ENTRY
    : null
  const openClawPackage = openClawPackageEntry
    ? readPackage(reader, openClawPackageEntry)
    : null
  if (!openClawPackage || openClawPackage.name !== 'openclaw' ||
    openClawPackage.version !== OPENCLAW_SHIM_VERSION) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_OPENCLAW_PACKAGE_MISSING_OR_MISMATCHED',
      artifactEntry: openClawPackageEntry
        ? `/${openClawPackageEntry}`
        : `/<missing-${PACKAGED_OPENCLAW_PACKAGE_ENTRY}>`,
      governingTerm: 'The exact artifact must contain the reviewed OpenClaw compatibility package instance.',
      missingAction: `Retain ${PACKAGED_OPENCLAW_PACKAGE_ENTRY} at version ${OPENCLAW_SHIM_VERSION}.`
    })
  }
  if (!openClawPackage || !hasExactOpenClawMaintainerMetadata(openClawPackage)) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_OPENCLAW_MAINTAINER_IDENTITY_MISSING_OR_MISMATCHED',
      artifactEntry: openClawPackageEntry
        ? `/${openClawPackageEntry}:author/maintainers`
        : `/<missing-${PACKAGED_OPENCLAW_PACKAGE_ENTRY}>`,
      governingTerm: 'The packaged compatibility shim must identify the exact current Analytix maintainer without replacing upstream ownership.',
      missingAction: `Retain exact author and maintainer metadata for ${PRODUCT_PACKAGE_AUTHOR}.`
    })
  }
  if (!openClawPackage || !hasExactOpenClawUpstreamMetadata(openClawPackage)) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_OPENCLAW_UPSTREAM_PROVENANCE_MISSING_OR_MISMATCHED',
      artifactEntry: openClawPackageEntry
        ? `/${openClawPackageEntry}:contributors/analytixProvenance`
        : `/<missing-${PACKAGED_OPENCLAW_PACKAGE_ENTRY}>`,
      governingTerm: 'The packaged compatibility shim must retain the exact OpenClaw contributor, annotated-tag, and peeled-commit provenance.',
      missingAction: 'Retain the exact OpenClaw v2026.5.18 upstream provenance metadata.'
    })
  }
  if (!openClawPackage || licenseValue(openClawPackage) !== 'MIT') {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_OPENCLAW_LICENSE_METADATA_MISSING_OR_MISMATCHED',
      artifactEntry: openClawPackageEntry
        ? `/${openClawPackageEntry}:license`
        : `/<missing-${PACKAGED_OPENCLAW_PACKAGE_ENTRY}>`,
      governingTerm: 'The packaged OpenClaw-derived shim must declare its retained upstream MIT term.',
      missingAction: 'Retain exact MIT license metadata in the packaged OpenClaw shim.'
    })
  }
  const openClawLicenseEntry = reader.has(PACKAGED_OPENCLAW_LICENSE_ENTRY)
    ? PACKAGED_OPENCLAW_LICENSE_ENTRY
    : null
  const exactOpenClawLicenseText = readTextEntry(reader, openClawLicenseEntry)
  if (!openClawLicenseEntry ||
    sha256Text(exactOpenClawLicenseText) !== OPENCLAW_UPSTREAM_LICENSE_SHA256) {
    artifactResourceBlockers.push({
      code: 'EXACT_ARTIFACT_OPENCLAW_LICENSE_MISSING_OR_DRIFTED',
      artifactEntry: openClawLicenseEntry
        ? `/${openClawLicenseEntry}`
        : `/<missing-${PACKAGED_OPENCLAW_LICENSE_ENTRY}>`,
      governingTerm: 'The exact artifact must retain the byte-exact OpenClaw v2026.5.18 MIT license and copyright notice.',
      missingAction: `Retain the exact reviewed license at ${PACKAGED_OPENCLAW_LICENSE_ENTRY}.`
    })
  }
  if (plugin && COMMERCIAL_LICENSE_RE.test(licenseValue(plugin) || String(plugin.license || ''))) {
    commercialLicenseDecisions.push(decisionRecord({
      artifactEntry: pluginEntry,
      name: plugin.name || 'analytix-fund-analysis',
      version: plugin.version || '',
      source: 'internal-plugin',
      governingTerm: licenseValue(plugin) || String(plugin.license),
      missingAction: 'Make the explicit commercial license decision for the Funds plugin; do not change or infer its manifest license.',
      obligationClass: ARTIFACT_OBLIGATION_CLASSES.commercialLicenseDecision
    }))
  }

  const releaseAuthorization = decisionRecord({
    artifactEntry: reader.label,
    name: 'release-authorization',
    source: 'release',
    governingTerm: 'Signing, notarization, publication, and commercial release authorization are external release decisions.',
    missingAction: 'Obtain explicit signing/notarization/release authority before publishing; this decision is not part of engineering admission.',
    obligationClass: ARTIFACT_OBLIGATION_CLASSES.releaseAuthorization
  })
  const internalMetadata = dependencyInstances.filter((entry) => entry.obligationClass === ARTIFACT_OBLIGATION_CLASSES.internalMetadata)
  const mandatoryExternalObligations = dependencyInstances.filter((entry) => entry.obligationClass === ARTIFACT_OBLIGATION_CLASSES.mandatoryExternal)
  const mandatoryBlockers = mandatoryExternalObligations.filter((entry) => entry.engineeringBlocking)
  const allObligations = [
    ...dependencyInstances,
    ...commercialLicenseDecisions,
    releaseAuthorization
  ]
  const inventory = {
    status: parseErrors.length > 0 || mandatoryBlockers.length > 0 || artifactResourceBlockers.length > 0
      ? 'blocked'
      : 'passed',
    exactArtifactReadable: true,
    artifactEntry: reader.label,
    artifact: {
      path: reader.label,
      sha256: reader.sha256 || null,
      byteLength: reader.byteLength ?? null,
      ...(Array.isArray(reader.components) ? { components: reader.components } : {})
    },
    dependencyInstances,
    packageInstanceCount: dependencyInstances.length,
    uniquePackageVersionCount: new Set(dependencyInstances.map((entry) => `${entry.name}@${entry.version}`)).size,
    obligations: allObligations,
    mandatoryExternalObligations,
    internalMetadata,
    commercialLicenseDecisions,
    releaseAuthorization: [releaseAuthorization],
    artifactResources: {
      productLicense: productLicenseEntry ? `/${productLicenseEntry}` : null,
      thirdPartyNotice: noticeEntry ? `/${noticeEntry}` : null
    },
    mandatoryBlockers: [
      ...parseErrors.map((entry) => ({
        code: 'EXACT_ARTIFACT_PACKAGE_JSON_UNREADABLE',
        artifactEntry: entry.artifactEntry,
        governingTerm: 'The exact artifact dependency package metadata must be readable to establish its governing redistribution term.',
        missingAction: 'Restore a readable package.json in the exact artifact and rerun the exact inventory.',
        detail: entry.reason
      })),
      ...mandatoryBlockers.map((entry) => ({
        code: 'EXACT_ARTIFACT_MANDATORY_LEGAL_OBLIGATION',
        artifactEntry: entry.artifactEntry,
        governingTerm: entry.governingTerm,
        missingAction: entry.missingAction
      })),
      ...artifactResourceBlockers
    ],
    parseErrors,
    commercialDecisionPending: commercialLicenseDecisions.length > 0,
    releaseAuthorizationPending: true,
    engineeringAdmission: parseErrors.length === 0 && mandatoryBlockers.length === 0 &&
      artifactResourceBlockers.length === 0
  }
  reader.assertStable?.()
  return inventory
}

function exactInputBlocked(detail, artifactEntry = '<exact-artifact>') {
  const blocker = {
    code: 'EXACT_ARTIFACT_INPUT_UNREADABLE',
    artifactEntry,
    governingTerm: 'Exact artifact bytes and dependency entries must be readable before legal admission can be decided.',
    missingAction: 'Provide one readable final app.asar or packaged app directory and rerun the exact inventory.',
    detail
  }
  return {
    status: 'blocked',
    provided: false,
    passed: false,
    exactArtifactReadable: false,
    reason: detail,
    artifactEntry,
    artifact: { path: artifactEntry, sha256: null, byteLength: null },
    dependencyInstances: [],
    packageInstanceCount: 0,
    uniquePackageVersionCount: 0,
    obligations: [],
    mandatoryExternalObligations: [],
    internalMetadata: [],
    commercialLicenseDecisions: [],
    releaseAuthorization: [],
    mandatoryBlockers: [blocker],
    engineeringAdmission: false
  }
}

function exactInputUnverified() {
  return {
    status: 'UNVERIFIED',
    provided: false,
    passed: false,
    exactArtifactReadable: false,
    reason: 'exact_artifact_not_provided',
    artifactEntry: '<exact-artifact>',
    artifact: { path: '<exact-artifact>', sha256: null, byteLength: null },
    dependencyInstances: [],
    packageInstanceCount: 0,
    uniquePackageVersionCount: 0,
    obligations: [],
    mandatoryExternalObligations: [],
    internalMetadata: [],
    commercialLicenseDecisions: [],
    releaseAuthorization: [],
    mandatoryBlockers: [],
    engineeringAdmission: false
  }
}

export function inspectExactArtifactLegalInventory({ artifact, artifactPath, appAsarPath, internalPackageNames, internalPackagePrefixes } = {}) {
  const input = artifact || artifactPath || appAsarPath
  if (!input) return exactInputBlocked('exact_artifact_input_missing')
  try {
    const reader = createExactArtifactReader(input)
    const inventory = exactInventory(reader, { internalPackageNames, internalPackagePrefixes })
    return {
      ...inventory,
      provided: true,
      passed: inventory.engineeringAdmission,
      status: inventory.engineeringAdmission ? 'passed' : 'blocked'
    }
  } catch (error) {
    return exactInputBlocked(error?.message || 'exact_artifact_input_unreadable', typeof input === 'string' ? input : undefined)
  }
}

export const artifactLegalObligationsTestInternals = Object.freeze({
  captureDirectoryInventory,
  createAsarReader,
  createDirectoryReader,
  createMemoryReader,
  createArtifactReaderFromPath,
  readStableSingleLinkRegularFile
})

function reportWithExactArtifact(plan, exactArtifactInput) {
  const requestedExactArtifact = exactArtifactInput || ''
  const exactFormalArtifact = requestedExactArtifact
    ? inspectExactArtifactLegalInventory({ artifact: requestedExactArtifact })
    : exactInputUnverified()
  const exactBlockers = exactFormalArtifact.mandatoryBlockers || []
  const artifactAdmissionGatePassed = exactFormalArtifact.engineeringAdmission === true
  const exactArtifactRequested = requestedExactArtifact !== ''
  const legalAuditPassed = plan.artifactLegalPlanPassed === true &&
    (!exactArtifactRequested || artifactAdmissionGatePassed)
  return {
    schemaVersion: 2,
    id: ARTIFACT_LEGAL_OBLIGATIONS_CONTRACT,
    status: legalAuditPassed ? 'passed' : 'blocked',
    passed: legalAuditPassed,
    claimCeiling: CLAIM_CEILING,
    artifactLegalPlanPassed: plan.artifactLegalPlanPassed,
    sourcePackagePlan: plan.sourcePackagePlan,
    exactFormalArtifact,
    exactArtifactLegalInventory: exactFormalArtifact,
    artifactAdmissionGatePassed,
    blockers: [...plan.sourcePackagePlan.blockers, ...exactBlockers],
    artifactAdmissionBlockers: exactBlockers,
    commercialLicenseDecisions: exactFormalArtifact.commercialLicenseDecisions || [],
    releaseAuthorization: exactFormalArtifact.releaseAuthorization || [],
    commercialReleaseAuthorized: false,
    formalArtifactNotChecked: !exactArtifactRequested
  }
}

export function evaluateArtifactLegalPlan(input = {}) {
  const plan = evaluatePlan(input)
  const exactArtifact = input.exactArtifact || input.artifact || input.exactArtifactPath || input.artifactPath
  return reportWithExactArtifact(plan, exactArtifact)
}

export function auditArtifactLegalObligations({ repoRoot = REPO_ROOT, exactArtifactPath, appAsarPath, artifact } = {}) {
  const plan = evaluatePlan(planInputFromRepository(resolve(repoRoot)))
  return reportWithExactArtifact(plan, artifact || exactArtifactPath || appAsarPath)
}

function selfTest() {
  const validBuilderConfig = `
    module.exports = {
      files: ['out/**/*', 'node_modules/openclaw/LICENSE'],
      extraResources: [
        { from: 'LICENSE', to: 'LICENSE' },
        { from: 'THIRD_PARTY_NOTICES.md', to: 'THIRD_PARTY_NOTICES.md' }
      ]
    }
  `
  const validPackage = {
    name: 'analytix',
    productName: 'analytix',
    author: PRODUCT_PACKAGE_AUTHOR,
    license: PRODUCT_LICENSE_ID,
    dependencies: { openclaw: 'file:vendor/openclaw-shim' }
  }
  const validRuntimePackage = {
    name: '@analytix/runtime',
    author: PRODUCT_PACKAGE_AUTHOR,
    license: PRODUCT_LICENSE_ID
  }
  const validPluginAuthor = { ...PRODUCT_PLUGIN_AUTHOR }
  const validOpenClawPackage = {
    name: 'openclaw',
    version: OPENCLAW_SHIM_VERSION,
    author: PRODUCT_PACKAGE_AUTHOR,
    maintainers: [PRODUCT_PACKAGE_AUTHOR],
    contributors: [OPENCLAW_UPSTREAM_CONTRIBUTOR],
    license: 'MIT',
    analytixProvenance: {
      upstreamRepository: OPENCLAW_UPSTREAM_REPOSITORY,
      upstreamTag: OPENCLAW_UPSTREAM_TAG,
      upstreamTagObject: OPENCLAW_UPSTREAM_TAG_OBJECT,
      upstreamCommit: OPENCLAW_UPSTREAM_COMMIT
    }
  }
  const validPackageLock = {
    packages: {
      '': { name: 'analytix', license: PRODUCT_LICENSE_ID },
      'node_modules/openclaw': { resolved: 'vendor/openclaw-shim', link: true },
      'vendor/openclaw-shim': {
        name: 'openclaw', version: OPENCLAW_SHIM_VERSION, license: 'MIT'
      }
    }
  }
  const validRuntimePackageLock = {
    packages: { '': { name: 'analytix-runtime', license: PRODUCT_LICENSE_ID } }
  }
  const validProductLicense = readText(REPO_ROOT, PRODUCT_LICENSE_ENTRY)
  const validNotice = readText(REPO_ROOT, NOTICE_ENTRY)
  const base = {
    packageJson: validPackage,
    packageLockJson: validPackageLock,
    runtimePackageJson: validRuntimePackage,
    runtimePackageLockJson: validRuntimePackageLock,
    fundsPluginManifest: {
      name: 'analytix-fund-analysis',
      author: validPluginAuthor,
      license: PRODUCT_LICENSE_ID
    },
    computerUsePluginManifest: {
      name: 'analytix-computer-use',
      author: validPluginAuthor,
      license: 'MIT'
    },
    computerUseVendorPackage: {
      name: 'analytix-computer-use',
      author: PRODUCT_PACKAGE_AUTHOR,
      license: 'MIT'
    },
    computerUseVendorPluginManifest: {
      name: 'analytix-computer-use',
      author: validPluginAuthor,
      license: 'MIT'
    },
    openClawShimPackage: validOpenClawPackage,
    openClawShimLicenseText: readText(REPO_ROOT, OPENCLAW_SHIM_LICENSE_ENTRY),
    codeReuseProvenanceText: readText(REPO_ROOT, CODE_REUSE_PROVENANCE_ENTRY),
    upstreamSourcesManifest: readJson(REPO_ROOT, UPSTREAM_SOURCES_ENTRY),
    builderConfigs: [{ artifactEntry: 'electron-builder.config.cjs', text: validBuilderConfig }],
    productLicenseText: validProductLicense,
    noticeText: validNotice
  }
  const artifactBaseEntries = {
    'package.json': JSON.stringify({
      name: 'analytix', version: '1.0.0', author: PRODUCT_PACKAGE_AUTHOR, license: PRODUCT_LICENSE_ID
    }),
    [RUNTIME_PACKAGE_ENTRY]: JSON.stringify({
      name: 'analytix-runtime', version: '1.0.0', author: PRODUCT_PACKAGE_AUTHOR,
      license: PRODUCT_LICENSE_ID
    }),
    [RUNTIME_PACKAGE_LOCK_ENTRY]: JSON.stringify(validRuntimePackageLock),
    LICENSE: validProductLicense,
    [NOTICE_ENTRY]: validNotice,
    'plugins/analytix-fund-analysis/.codex-plugin/plugin.json': JSON.stringify({
      name: 'analytix-fund-analysis', version: '1.0.0', author: validPluginAuthor,
      license: PRODUCT_LICENSE_ID
    }),
    [PACKAGED_COMPUTER_USE_PACKAGE_ENTRY]: JSON.stringify({
      name: 'analytix-computer-use', version: '1.0.0', author: PRODUCT_PACKAGE_AUTHOR,
      license: 'MIT'
    }),
    'node_modules/analytix-computer-use/LICENSE': 'MIT License',
    [PACKAGED_COMPUTER_USE_PLUGIN_ENTRY]: JSON.stringify({
      name: 'analytix-computer-use', version: '1.0.0', author: validPluginAuthor,
      license: 'MIT'
    }),
    [PACKAGED_OPENCLAW_PACKAGE_ENTRY]: JSON.stringify(validOpenClawPackage),
    [PACKAGED_OPENCLAW_LICENSE_ENTRY]: readText(REPO_ROOT, OPENCLAW_SHIM_LICENSE_ENTRY)
  }
  const positive = evaluateArtifactLegalPlan({ ...base, artifact: {
    label: 'self-test-artifact',
    entries: {
      ...artifactBaseEntries,
      'node_modules/mit-package/package.json': JSON.stringify({ name: 'mit-package', version: '1.0.0', license: 'MIT' }),
      'node_modules/mit-package/LICENSE': 'MIT License',
      'node_modules/mit-package/dist/esm/package.json': JSON.stringify({ type: 'module' }),
      'node_modules/choice-package/package.json': JSON.stringify({ name: 'choice-package', version: '2.0.0', license: '(MIT OR Apache-2.0)' }),
      'node_modules/choice-package/LICENSE': 'Choice license text',
      'node_modules/apache-package/package.json': JSON.stringify({ name: 'apache-package', version: '3.0.0', license: 'Apache-2.0' }),
      'node_modules/apache-package/LICENSE': 'Apache License 2.0',
      'node_modules/proprietary-package/package.json': JSON.stringify({ name: 'proprietary-package', version: '3.1.0', license: 'PROPRIETARY' }),
      'node_modules/internal-package/package.json': JSON.stringify({ name: '@analytix/internal-package', version: '4.0.0' }),
      'node_modules/duplicate/package.json': JSON.stringify({ name: 'duplicate', version: '1.0.0', license: 'MIT' }),
      'node_modules/duplicate/LICENSE': 'MIT License',
      'node_modules/nested/node_modules/duplicate/package.json': JSON.stringify({ name: 'duplicate', version: '1.0.0', license: 'MIT' }),
      'node_modules/nested/node_modules/duplicate/LICENSE': 'MIT License'
    }
  } })
  const inventory = positive.exactFormalArtifact
  const missingLicense = evaluateArtifactLegalPlan({ ...base, artifact: {
    label: 'self-test-missing-license',
    entries: {
      ...artifactBaseEntries,
      'node_modules/missing-license/package.json': JSON.stringify({ name: 'missing-license', version: '1.0.0', license: 'MIT' })
    }
  } }).exactFormalArtifact
  const missingExact = evaluateArtifactLegalPlan(base).exactFormalArtifact
  const productLicenseMismatch = evaluateArtifactLegalPlan({ ...base, artifact: {
    label: 'self-test-product-license-mismatch',
    entries: {
      ...artifactBaseEntries,
      'package.json': JSON.stringify({
        name: 'analytix', version: '1.0.0', author: PRODUCT_PACKAGE_AUTHOR, license: 'MIT'
      })
    }
  } }).exactFormalArtifact
  const runtimeLicenseMismatch = evaluateArtifactLegalPlan({ ...base, artifact: {
    label: 'self-test-runtime-license-mismatch',
    entries: {
      ...artifactBaseEntries,
      [RUNTIME_PACKAGE_ENTRY]: JSON.stringify({
        name: 'analytix-runtime', version: '1.0.0', author: PRODUCT_PACKAGE_AUTHOR,
        license: 'MIT'
      })
    }
  } }).exactFormalArtifact
  const runtimeLockLicenseMismatch = evaluateArtifactLegalPlan({ ...base, artifact: {
    label: 'self-test-runtime-lock-license-mismatch',
    entries: {
      ...artifactBaseEntries,
      [RUNTIME_PACKAGE_LOCK_ENTRY]: JSON.stringify({
        packages: { '': { name: 'analytix-runtime', license: 'MIT' } }
      })
    }
  } }).exactFormalArtifact
  const fundsLicenseMismatch = evaluateArtifactLegalPlan({ ...base, artifact: {
    label: 'self-test-funds-license-mismatch',
    entries: {
      ...artifactBaseEntries,
      'plugins/analytix-fund-analysis/.codex-plugin/plugin.json': JSON.stringify({
        name: 'analytix-fund-analysis', version: '1.0.0', author: validPluginAuthor,
        license: 'UNLICENSED'
      })
    }
  } }).exactFormalArtifact
  const productAuthorMismatch = evaluateArtifactLegalPlan({ ...base, artifact: {
    label: 'self-test-product-author-mismatch',
    entries: {
      ...artifactBaseEntries,
      'package.json': JSON.stringify({
        name: 'analytix', version: '1.0.0', author: 'Analytix Contributors',
        license: PRODUCT_LICENSE_ID
      })
    }
  } }).exactFormalArtifact
  const runtimeAuthorMismatch = evaluateArtifactLegalPlan({ ...base, artifact: {
    label: 'self-test-runtime-author-mismatch',
    entries: {
      ...artifactBaseEntries,
      [RUNTIME_PACKAGE_ENTRY]: JSON.stringify({
        name: 'analytix-runtime', version: '1.0.0', author: 'Analytix Contributors',
        license: PRODUCT_LICENSE_ID
      })
    }
  } }).exactFormalArtifact
  const fundsAuthorMismatch = evaluateArtifactLegalPlan({ ...base, artifact: {
    label: 'self-test-funds-author-mismatch',
    entries: {
      ...artifactBaseEntries,
      [FUNDS_PLUGIN_ENTRY]: JSON.stringify({
        name: 'analytix-fund-analysis', version: '1.0.0',
        author: { name: 'Analytix', url: 'https://analytix.top' },
        license: PRODUCT_LICENSE_ID
      })
    }
  } }).exactFormalArtifact
  const computerUsePackageAuthorMismatch = evaluateArtifactLegalPlan({ ...base, artifact: {
    label: 'self-test-computer-use-package-author-mismatch',
    entries: {
      ...artifactBaseEntries,
      [PACKAGED_COMPUTER_USE_PACKAGE_ENTRY]: JSON.stringify({
        name: 'analytix-computer-use', version: '1.0.0', author: 'Analytix Contributors',
        license: 'MIT'
      })
    }
  } }).exactFormalArtifact
  const computerUsePluginAuthorMismatch = evaluateArtifactLegalPlan({ ...base, artifact: {
    label: 'self-test-computer-use-plugin-author-mismatch',
    entries: {
      ...artifactBaseEntries,
      [PACKAGED_COMPUTER_USE_PLUGIN_ENTRY]: JSON.stringify({
        name: 'analytix-computer-use', version: '1.0.0',
        author: { name: 'Analytix', url: 'https://analytix.top' },
        license: 'MIT'
      })
    }
  } }).exactFormalArtifact
  const artifactWithoutOpenClaw = { ...artifactBaseEntries }
  delete artifactWithoutOpenClaw[PACKAGED_OPENCLAW_PACKAGE_ENTRY]
  delete artifactWithoutOpenClaw[PACKAGED_OPENCLAW_LICENSE_ENTRY]
  const missingOpenClawPackage = evaluateArtifactLegalPlan({ ...base, artifact: {
    label: 'self-test-openclaw-package-missing',
    entries: artifactWithoutOpenClaw
  } }).exactFormalArtifact
  const openClawMaintainerDrift = [
    { author: undefined, maintainers: undefined },
    { author: 'Analytix Contributors', maintainers: ['Analytix Contributors'] },
    { author: 'Guoqin He', maintainers: ['Guoqin He'] }
  ].map((metadata, index) => evaluateArtifactLegalPlan({ ...base, artifact: {
    label: `self-test-openclaw-maintainer-drift-${index + 1}`,
    entries: {
      ...artifactBaseEntries,
      [PACKAGED_OPENCLAW_PACKAGE_ENTRY]: JSON.stringify({
        ...validOpenClawPackage,
        ...metadata
      })
    }
  } }).exactFormalArtifact)
  const openClawUpstreamDrift = [
    {
      contributors: ['Analytix Contributors'],
      analytixProvenance: validOpenClawPackage.analytixProvenance
    },
    {
      contributors: validOpenClawPackage.contributors,
      analytixProvenance: {
        ...validOpenClawPackage.analytixProvenance,
        upstreamTagObject: '0'.repeat(40)
      }
    }
  ].map((metadata, index) => evaluateArtifactLegalPlan({ ...base, artifact: {
    label: `self-test-openclaw-upstream-drift-${index + 1}`,
    entries: {
      ...artifactBaseEntries,
      [PACKAGED_OPENCLAW_PACKAGE_ENTRY]: JSON.stringify({
        ...validOpenClawPackage,
        ...metadata
      })
    }
  } }).exactFormalArtifact)
  const openClawLicenseMetadataDrift = [undefined, 'Apache-2.0']
    .map((license, index) => evaluateArtifactLegalPlan({ ...base, artifact: {
      label: `self-test-openclaw-license-metadata-drift-${index + 1}`,
      entries: {
        ...artifactBaseEntries,
        [PACKAGED_OPENCLAW_PACKAGE_ENTRY]: JSON.stringify({
          ...validOpenClawPackage,
          license
        })
      }
    } }).exactFormalArtifact)
  const artifactWithoutOpenClawLicense = { ...artifactBaseEntries }
  delete artifactWithoutOpenClawLicense[PACKAGED_OPENCLAW_LICENSE_ENTRY]
  const openClawLicenseFileDrift = [
    evaluateArtifactLegalPlan({ ...base, artifact: {
      label: 'self-test-openclaw-license-file-missing',
      entries: artifactWithoutOpenClawLicense
    } }).exactFormalArtifact,
    evaluateArtifactLegalPlan({ ...base, artifact: {
      label: 'self-test-openclaw-license-file-variant',
      entries: {
        ...artifactBaseEntries,
        [PACKAGED_OPENCLAW_LICENSE_ENTRY]: 'MIT License\nCopyright (c) OpenClaw\n'
      }
    } }).exactFormalArtifact
  ]
  const unreadableExact = evaluateArtifactLegalPlan({
    ...base,
    exactArtifactPath: join(REPO_ROOT, 'this-exact-artifact-does-not-exist.asar')
  }).exactFormalArtifact
  const sourcePlanMissing = evaluateArtifactLegalPlan({ ...base, noticeText: '' })
  const sourcePlanLicenseDrift = evaluateArtifactLegalPlan({
    ...base,
    productLicenseText: `${validProductLicense}\ntrailing drift\n`
  })
  const legacyProductLicense = validProductLicense.replace(
    PRODUCT_COPYRIGHT_NOTICE,
    'Copyright 2026 xingyu'
  )
  const missingCopyrightProductLicense = validProductLicense.replace(
    `${PRODUCT_COPYRIGHT_NOTICE}\n`,
    ''
  )
  const duplicateCopyrightProductLicense = `${PRODUCT_COPYRIGHT_NOTICE}\n${validProductLicense}`
  const variantCopyrightProductLicenses = [
    validProductLicense.replace(PRODUCT_COPYRIGHT_NOTICE, 'Copyright 2026 Guoqin He'),
    validProductLicense.replace(PRODUCT_COPYRIGHT_NOTICE, 'Copyright 2026 Guoqin He (GitHub: @Eysn0130)'),
    validProductLicense.replace(PRODUCT_COPYRIGHT_NOTICE, 'Copyright 2026 Eysn0130')
  ]
  const sourceCopyrightIdentityDrift = [
    legacyProductLicense,
    missingCopyrightProductLicense,
    duplicateCopyrightProductLicense,
    ...variantCopyrightProductLicenses
  ].map((productLicenseText) => evaluateArtifactLegalPlan({ ...base, productLicenseText }))
  const exactCopyrightIdentityDrift = [
    legacyProductLicense,
    missingCopyrightProductLicense,
    duplicateCopyrightProductLicense,
    ...variantCopyrightProductLicenses
  ].map((licenseText, index) => evaluateArtifactLegalPlan({ ...base, artifact: {
    label: `self-test-product-copyright-drift-${index + 1}`,
    entries: { ...artifactBaseEntries, LICENSE: licenseText }
  } }).exactFormalArtifact)
  const sourceProductLicenseMismatch = evaluateArtifactLegalPlan({
    ...base,
    packageJson: { ...validPackage, license: 'MIT' }
  })
  const sourcePackageAuthorMismatch = evaluateArtifactLegalPlan({
    ...base,
    packageJson: { ...validPackage, author: 'Analytix Contributors' }
  })
  const sourcePackageLockLicenseMismatch = evaluateArtifactLegalPlan({
    ...base,
    packageLockJson: { packages: { '': { name: 'analytix', license: 'MIT' } } }
  })
  const sourceRuntimeLicenseMismatch = evaluateArtifactLegalPlan({
    ...base,
    runtimePackageJson: { ...validRuntimePackage, license: 'MIT' }
  })
  const sourceRuntimeAuthorMismatch = evaluateArtifactLegalPlan({
    ...base,
    runtimePackageJson: { ...validRuntimePackage, author: 'Analytix Contributors' }
  })
  const sourceRuntimeLockLicenseMismatch = evaluateArtifactLegalPlan({
    ...base,
    runtimePackageLockJson: {
      packages: { '': { name: 'analytix-runtime', license: 'MIT' } }
    }
  })
  const sourceFundsLicenseMismatch = evaluateArtifactLegalPlan({
    ...base,
    fundsPluginManifest: {
      name: 'analytix-fund-analysis',
      license: 'UNLICENSED'
    }
  })
  const sourceFundsAuthorMismatch = evaluateArtifactLegalPlan({
    ...base,
    fundsPluginManifest: {
      ...base.fundsPluginManifest,
      author: { name: 'Analytix', url: 'https://analytix.top' }
    }
  })
  const sourceComputerUseAuthorMismatch = evaluateArtifactLegalPlan({
    ...base,
    computerUsePluginManifest: {
      ...base.computerUsePluginManifest,
      author: { name: 'Analytix', url: 'https://analytix.top' }
    },
    computerUseVendorPackage: {
      ...base.computerUseVendorPackage,
      author: 'Analytix Contributors'
    },
    computerUseVendorPluginManifest: {
      ...base.computerUseVendorPluginManifest,
      author: { name: 'Analytix', url: 'https://analytix.top' }
    }
  })
  const sourceOpenClawMaintainerDrift = [
    { author: undefined, maintainers: undefined },
    { author: 'Analytix Contributors', maintainers: ['Analytix Contributors'] },
    { author: 'Guoqin He', maintainers: ['Guoqin He'] }
  ].map((metadata) => evaluateArtifactLegalPlan({
    ...base,
    openClawShimPackage: { ...validOpenClawPackage, ...metadata }
  }))
  const sourceOpenClawUpstreamDrift = evaluateArtifactLegalPlan({
    ...base,
    openClawShimPackage: {
      ...validOpenClawPackage,
      contributors: ['Analytix Contributors'],
      analytixProvenance: {
        ...validOpenClawPackage.analytixProvenance,
        upstreamCommit: '0'.repeat(40)
      }
    }
  })
  const sourceOpenClawLicenseDrift = evaluateArtifactLegalPlan({
    ...base,
    openClawShimPackage: { ...validOpenClawPackage, license: 'Apache-2.0' },
    openClawShimLicenseText: 'MIT License\nvariant\n'
  })
  const sourceOpenClawLockDrift = evaluateArtifactLegalPlan({
    ...base,
    packageLockJson: {
      packages: {
        ...validPackageLock.packages,
        'vendor/openclaw-shim': {
          ...validPackageLock.packages['vendor/openclaw-shim'],
          license: 'Apache-2.0'
        }
      }
    }
  })
  const sourceOpenClawProvenanceDrift = evaluateArtifactLegalPlan({
    ...base,
    codeReuseProvenanceText: '<!-- analytix-openclaw-shim-provenance-v1 {} -->\n'
  })
  const sourceOpenClawRegistryDrift = evaluateArtifactLegalPlan({
    ...base,
    upstreamSourcesManifest: {
      ...base.upstreamSourcesManifest,
      sources: base.upstreamSourcesManifest.sources.map((entry) => entry.id === 'openclaw'
        ? { ...entry, commit: '0'.repeat(40) }
        : entry)
    }
  })
  const validAsarLinks = flattenAsarFiles({
    node_modules: { files: {
      '.bin': { files: {
        tool: { link: 'node_modules/tool/bin/tool', unpacked: true }
      } }
    } }
  })
  const validAsarLinkAccepted = validAsarLinks.get('node_modules/.bin/tool')?.link ===
    'node_modules/tool/bin/tool'
  let escapingAsarLinkRejected = false
  try {
    flattenAsarFiles({ tool: { link: '../outside', unpacked: true } })
  } catch {
    escapingAsarLinkRejected = true
  }
  const physicalAsarFiles = filterPhysicalAsarFiles(new Map([
    ['packed/package.json', { offset: 0, size: 1, unpacked: false }],
    ['present/package.json', { offset: 0, size: 1, unpacked: true }],
    ['pruned/package.json', { offset: 0, size: 1, unpacked: true }],
    ['node_modules/.bin/tool', { link: 'node_modules/tool/bin/tool', offset: 0, size: 0, unpacked: true }]
  ]), (entry) => entry === 'present/package.json')
  const physicalAsarEntriesFiltered = physicalAsarFiles.has('packed/package.json') &&
    physicalAsarFiles.has('present/package.json') &&
    !physicalAsarFiles.has('pruned/package.json') &&
    physicalAsarFiles.has('node_modules/.bin/tool')
  const syntheticAppRoot = resolve(sep, 'tmp', 'analytix-self-test.app')
  const containedDirectorySymlinkAccepted = resolveContainedSymlinkTarget(
    syntheticAppRoot,
    join(syntheticAppRoot, 'Contents', 'Frameworks', 'Example.framework', 'Versions', 'Current'),
    'A'
  ) === join(syntheticAppRoot, 'Contents', 'Frameworks', 'Example.framework', 'Versions', 'A')
  let escapingDirectorySymlinkRejected = false
  try {
    resolveContainedSymlinkTarget(
      syntheticAppRoot,
      join(syntheticAppRoot, 'Contents', 'escape'),
      '../../../outside'
    )
  } catch {
    escapingDirectorySymlinkRejected = true
  }
  const recordsHaveRequiredFields = inventory.dependencyInstances.every((entry) => [
    'artifactEntry', 'name', 'version', 'source', 'governingTerm',
    'licenseFile', 'noticeFile', 'missingAction'
  ].every((key) => Object.hasOwn(entry, key)))
  const classes = new Set(inventory.obligations.map((entry) => entry.obligationClass))
  const passed = positive.artifactAdmissionGatePassed === true &&
    positive.sourcePackagePlan.passed === true &&
    inventory.packageInstanceCount === 9 &&
    inventory.dependencyInstances.filter((entry) => entry.name === 'duplicate').length === 2 &&
    inventory.dependencyInstances.some((entry) => entry.name === 'openclaw' &&
      entry.obligationClass === ARTIFACT_OBLIGATION_CLASSES.mandatoryExternal &&
      entry.licenseFile === `/${PACKAGED_OPENCLAW_LICENSE_ENTRY}` &&
      entry.status === 'passed') &&
    recordsHaveRequiredFields &&
    classes.has(ARTIFACT_OBLIGATION_CLASSES.mandatoryExternal) &&
    classes.has(ARTIFACT_OBLIGATION_CLASSES.internalMetadata) &&
    classes.has(ARTIFACT_OBLIGATION_CLASSES.commercialLicenseDecision) &&
    classes.has(ARTIFACT_OBLIGATION_CLASSES.releaseAuthorization) &&
    missingLicense.status === 'blocked' &&
    missingLicense.mandatoryBlockers.length === 1 &&
    missingExact.status === 'UNVERIFIED' &&
    missingExact.exactArtifactReadable === false &&
    missingExact.mandatoryBlockers.length === 0 &&
    productLicenseMismatch.status === 'blocked' &&
    productLicenseMismatch.mandatoryBlockers.some((entry) =>
      entry.code === 'EXACT_ARTIFACT_PRODUCT_LICENSE_METADATA_MISMATCH') &&
    runtimeLicenseMismatch.status === 'blocked' &&
    runtimeLicenseMismatch.mandatoryBlockers.some((entry) =>
      entry.code === 'EXACT_ARTIFACT_RUNTIME_LICENSE_METADATA_MISSING_OR_MISMATCHED') &&
    runtimeLockLicenseMismatch.status === 'blocked' &&
    runtimeLockLicenseMismatch.mandatoryBlockers.some((entry) =>
      entry.code === 'EXACT_ARTIFACT_RUNTIME_LOCK_LICENSE_METADATA_MISSING_OR_MISMATCHED') &&
    fundsLicenseMismatch.status === 'blocked' &&
    fundsLicenseMismatch.mandatoryBlockers.some((entry) =>
      entry.code === 'EXACT_ARTIFACT_FUNDS_PLUGIN_LICENSE_METADATA_MISMATCH') &&
    productAuthorMismatch.status === 'blocked' &&
    productAuthorMismatch.mandatoryBlockers.some((entry) =>
      entry.code === 'EXACT_ARTIFACT_PRODUCT_AUTHOR_IDENTITY_MISSING_OR_MISMATCHED') &&
    runtimeAuthorMismatch.status === 'blocked' &&
    runtimeAuthorMismatch.mandatoryBlockers.some((entry) =>
      entry.code === 'EXACT_ARTIFACT_RUNTIME_AUTHOR_IDENTITY_MISSING_OR_MISMATCHED') &&
    fundsAuthorMismatch.status === 'blocked' &&
    fundsAuthorMismatch.mandatoryBlockers.some((entry) =>
      entry.code === 'EXACT_ARTIFACT_FUNDS_PLUGIN_AUTHOR_IDENTITY_MISSING_OR_MISMATCHED') &&
    computerUsePackageAuthorMismatch.status === 'blocked' &&
    computerUsePackageAuthorMismatch.mandatoryBlockers.some((entry) =>
      entry.code === 'EXACT_ARTIFACT_COMPUTER_USE_PACKAGE_AUTHOR_IDENTITY_MISSING_OR_MISMATCHED') &&
    computerUsePluginAuthorMismatch.status === 'blocked' &&
    computerUsePluginAuthorMismatch.mandatoryBlockers.some((entry) =>
      entry.code === 'EXACT_ARTIFACT_COMPUTER_USE_PLUGIN_AUTHOR_IDENTITY_MISSING_OR_MISMATCHED') &&
    missingOpenClawPackage.engineeringAdmission === false &&
    missingOpenClawPackage.mandatoryBlockers.some((entry) =>
      entry.code === 'EXACT_ARTIFACT_OPENCLAW_PACKAGE_MISSING_OR_MISMATCHED') &&
    openClawMaintainerDrift.every((entry) => entry.engineeringAdmission === false &&
      entry.mandatoryBlockers.some((blocker) =>
        blocker.code === 'EXACT_ARTIFACT_OPENCLAW_MAINTAINER_IDENTITY_MISSING_OR_MISMATCHED')) &&
    openClawUpstreamDrift.every((entry) => entry.engineeringAdmission === false &&
      entry.mandatoryBlockers.some((blocker) =>
        blocker.code === 'EXACT_ARTIFACT_OPENCLAW_UPSTREAM_PROVENANCE_MISSING_OR_MISMATCHED')) &&
    openClawLicenseMetadataDrift.every((entry) => entry.engineeringAdmission === false &&
      entry.mandatoryBlockers.some((blocker) =>
        blocker.code === 'EXACT_ARTIFACT_OPENCLAW_LICENSE_METADATA_MISSING_OR_MISMATCHED')) &&
    openClawLicenseFileDrift.every((entry) => entry.engineeringAdmission === false &&
      entry.mandatoryBlockers.some((blocker) =>
        blocker.code === 'EXACT_ARTIFACT_OPENCLAW_LICENSE_MISSING_OR_DRIFTED')) &&
    unreadableExact.status === 'blocked' &&
    unreadableExact.mandatoryBlockers.length === 1 &&
    sourcePlanMissing.artifactAdmissionGatePassed === false &&
    sourcePlanMissing.passed === false &&
    sourcePlanMissing.sourcePackagePlan.passed === false &&
    sourcePlanLicenseDrift.sourcePackagePlan.passed === false &&
    sha256Text(legacyProductLicense) === '4a0f06cea251953ef0b0a5c91f914ad371fef6ae6efc9cb3f9096d3f09863adb' &&
    sourceCopyrightIdentityDrift.every((entry) => entry.sourcePackagePlan.passed === false) &&
    exactCopyrightIdentityDrift.every((entry) =>
      entry.status === 'blocked' && entry.mandatoryBlockers.some((blocker) =>
        blocker.code === 'EXACT_ARTIFACT_PRODUCT_LICENSE_MISSING_OR_DRIFTED')) &&
    sourceProductLicenseMismatch.sourcePackagePlan.passed === false &&
    sourcePackageAuthorMismatch.sourcePackagePlan.passed === false &&
    sourcePackageLockLicenseMismatch.sourcePackagePlan.passed === false &&
    sourceRuntimeLicenseMismatch.sourcePackagePlan.passed === false &&
    sourceRuntimeAuthorMismatch.sourcePackagePlan.passed === false &&
    sourceRuntimeLockLicenseMismatch.sourcePackagePlan.passed === false &&
    sourceFundsLicenseMismatch.sourcePackagePlan.passed === false &&
    sourceFundsAuthorMismatch.sourcePackagePlan.passed === false &&
    sourceComputerUseAuthorMismatch.sourcePackagePlan.passed === false &&
    sourceOpenClawMaintainerDrift.every((entry) => entry.sourcePackagePlan.passed === false) &&
    sourceOpenClawUpstreamDrift.sourcePackagePlan.passed === false &&
    sourceOpenClawLicenseDrift.sourcePackagePlan.passed === false &&
    sourceOpenClawLockDrift.sourcePackagePlan.passed === false &&
    sourceOpenClawProvenanceDrift.sourcePackagePlan.passed === false &&
    sourceOpenClawRegistryDrift.sourcePackagePlan.passed === false &&
    sourcePlanMissing.commercialReleaseAuthorized === false &&
    validAsarLinkAccepted &&
    escapingAsarLinkRejected &&
    physicalAsarEntriesFiltered &&
    containedDirectorySymlinkAccepted &&
    escapingDirectorySymlinkRejected
  return {
    schemaVersion: 2,
    id: 'analytix-artifact-legal-obligations-self-test',
    status: passed ? 'passed' : 'failed',
    passed,
    cases: {
      mit: inventory.dependencyInstances.some((entry) => entry.name === 'mit-package' && entry.licenseFile === '/node_modules/mit-package/LICENSE'),
      apache: inventory.dependencyInstances.some((entry) => entry.name === 'apache-package' && entry.governingTerm === 'Apache-2.0'),
      dualChoicePreserved: inventory.dependencyInstances.some((entry) => entry.name === 'choice-package' && entry.governingTerm === '(MIT OR Apache-2.0)'),
      missingLicenseBlocked: missingLicense.status === 'blocked',
      internalMetadataSeparated: inventory.internalMetadata.length === 2 &&
        inventory.internalMetadata.every((entry) => entry.engineeringBlocking === false),
      duplicateInstancesRetained: inventory.dependencyInstances.filter((entry) => entry.name === 'duplicate').length === 2,
      missingExactInputUnverified: missingExact.status === 'UNVERIFIED' && missingExact.mandatoryBlockers.length === 0,
      productLicenseMismatchBlocked: productLicenseMismatch.status === 'blocked',
      runtimeLicenseMismatchBlocked: runtimeLicenseMismatch.status === 'blocked',
      runtimeLockLicenseMismatchBlocked: runtimeLockLicenseMismatch.status === 'blocked',
      fundsLicenseMismatchBlocked: fundsLicenseMismatch.status === 'blocked',
      productAuthorMismatchBlocked: productAuthorMismatch.status === 'blocked',
      runtimeAuthorMismatchBlocked: runtimeAuthorMismatch.status === 'blocked',
      fundsAuthorMismatchBlocked: fundsAuthorMismatch.status === 'blocked',
      computerUsePackageAuthorMismatchBlocked: computerUsePackageAuthorMismatch.status === 'blocked',
      computerUsePluginAuthorMismatchBlocked: computerUsePluginAuthorMismatch.status === 'blocked',
      openClawMandatoryExternal: inventory.dependencyInstances.some((entry) =>
        entry.name === 'openclaw' &&
        entry.obligationClass === ARTIFACT_OBLIGATION_CLASSES.mandatoryExternal &&
        entry.licenseFile === `/${PACKAGED_OPENCLAW_LICENSE_ENTRY}`),
      openClawMissingPackageBlocked: missingOpenClawPackage.engineeringAdmission === false &&
        missingOpenClawPackage.mandatoryBlockers.some((entry) =>
          entry.code === 'EXACT_ARTIFACT_OPENCLAW_PACKAGE_MISSING_OR_MISMATCHED'),
      openClawMaintainerMissingOldAndVariantBlocked: openClawMaintainerDrift.every((entry) =>
        entry.engineeringAdmission === false),
      openClawUpstreamVariantBlocked: openClawUpstreamDrift.every((entry) =>
        entry.engineeringAdmission === false),
      openClawLicenseMetadataMissingAndVariantBlocked: openClawLicenseMetadataDrift
        .every((entry) => entry.engineeringAdmission === false),
      openClawLicenseFileMissingAndVariantBlocked: openClawLicenseFileDrift
        .every((entry) => entry.engineeringAdmission === false),
      unreadableExactInputBlocked: unreadableExact.status === 'blocked' && unreadableExact.mandatoryBlockers.length === 1,
      asarLinkEntryAccepted: validAsarLinkAccepted,
      escapingAsarLinkRejected,
      physicalAsarEntriesFiltered,
      containedDirectorySymlinkAccepted,
      escapingDirectorySymlinkRejected,
      commercialDecisionSeparated: inventory.commercialLicenseDecisions.length >= 1 && inventory.engineeringAdmission === true,
      sourcePlanDoesNotAuthorizeExact: sourcePlanMissing.artifactAdmissionGatePassed === false && sourcePlanMissing.sourcePackagePlan.passed === false,
      sourceLicenseDriftBlocked: sourcePlanLicenseDrift.sourcePackagePlan.passed === false,
      legacyOwnerAndHashBlocked: sha256Text(legacyProductLicense) ===
        '4a0f06cea251953ef0b0a5c91f914ad371fef6ae6efc9cb3f9096d3f09863adb' &&
        sourceCopyrightIdentityDrift[0].sourcePackagePlan.passed === false &&
        exactCopyrightIdentityDrift[0].status === 'blocked',
      missingDuplicateAndVariantCopyrightBlocked: sourceCopyrightIdentityDrift.slice(1)
        .every((entry) => entry.sourcePackagePlan.passed === false) &&
        exactCopyrightIdentityDrift.slice(1).every((entry) => entry.status === 'blocked'),
      sourceMetadataDriftBlocked: sourceProductLicenseMismatch.sourcePackagePlan.passed === false &&
        sourcePackageAuthorMismatch.sourcePackagePlan.passed === false &&
        sourcePackageLockLicenseMismatch.sourcePackagePlan.passed === false &&
        sourceRuntimeLicenseMismatch.sourcePackagePlan.passed === false &&
        sourceRuntimeAuthorMismatch.sourcePackagePlan.passed === false &&
        sourceRuntimeLockLicenseMismatch.sourcePackagePlan.passed === false &&
        sourceFundsLicenseMismatch.sourcePackagePlan.passed === false &&
        sourceFundsAuthorMismatch.sourcePackagePlan.passed === false &&
        sourceComputerUseAuthorMismatch.sourcePackagePlan.passed === false &&
        sourceOpenClawMaintainerDrift.every((entry) =>
          entry.sourcePackagePlan.passed === false) &&
        sourceOpenClawUpstreamDrift.sourcePackagePlan.passed === false &&
        sourceOpenClawLicenseDrift.sourcePackagePlan.passed === false &&
        sourceOpenClawLockDrift.sourcePackagePlan.passed === false &&
        sourceOpenClawProvenanceDrift.sourcePackagePlan.passed === false &&
        sourceOpenClawRegistryDrift.sourcePackagePlan.passed === false
    }
  }
}

function cliArtifactPath() {
  const args = process.argv.slice(2)
  for (const flag of ['--artifact', '--app-asar', '--exact-artifact']) {
    const index = args.indexOf(flag)
    if (index >= 0) return args[index + 1] || ''
  }
  return process.env[EXACT_ARTIFACT_ENV] || undefined
}

if (process.argv[1] && fileURLToPath(import.meta.url) === resolve(process.argv[1])) {
  const report = process.argv.includes('--self-test')
    ? selfTest()
    : auditArtifactLegalObligations({ repoRoot: REPO_ROOT, exactArtifactPath: cliArtifactPath() })
  process.stdout.write(`${JSON.stringify(report, null, 2)}\n`)
  process.exitCode = report.passed === true ? 0 : 1
}
