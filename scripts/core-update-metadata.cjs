const { readFileSync, writeFileSync, lstatSync } = require('node:fs')
const { join, basename } = require('node:path')
const { createHash } = require('node:crypto')
const yaml = require('js-yaml')

function validateCoreUpdateMetadata(value, { version, channel }) {
  if (typeof version !== 'string' || !/^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/.test(version) || !value || !['stable', 'beta'].includes(channel) || value.version !== version ||
      value.analytixProfile !== 'core' || value.analytixTarget !== 'darwin-arm64' ||
      value.analytixChannel !== channel || value.analytixAppId !== 'com.analytix.desktop' ||
      value.analytixDataCompatibility !== 'core-v1' || !Array.isArray(value.files) || !value.files.length) throw Error('core_update_metadata_invalid')
  const names = new Set()
  for (const entry of value.files) {
    if (!entry || typeof entry.url !== 'string' || !['zip', 'dmg'].some(ext => entry.url === `analytix-core-${version}-mac-arm64.${ext}`) ||
        names.has(entry.url) || !/^[A-Za-z0-9+/]{86}==$/.test(entry.sha512 || '') || !Number.isSafeInteger(entry.size) || entry.size <= 0) throw Error('core_update_artifact_invalid')
    names.add(entry.url)
  }
  if (!names.has(`analytix-core-${version}-mac-arm64.zip`)) throw Error('core_update_zip_required')
}

function verifyCoreUpdateArtifacts(root, metadata, expected) {
  validateCoreUpdateMetadata(metadata, expected)
  const names = new Set([...metadata.files.map(file => file.url), `analytix-core-${expected.version}-mac-arm64.dmg`])
  for (const name of names) {
    if (basename(name) !== name) throw Error('core_update_artifact_invalid')
    const path = join(root, name)
    const stat = lstatSync(path)
    if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1) throw Error('core_update_artifact_untrusted')
    const bytes = readFileSync(path)
    const file = metadata.files.find(file => file.url === name)
    if (file && (bytes.length !== file.size || createHash('sha512').update(bytes).digest('base64') !== file.sha512)) throw Error('core_update_artifact_digest_mismatch')
  }
}

// Run only after electron-builder completes: its afterAllArtifactBuild hook
// precedes update YAML generation in pinned builder 26.15.3. This postprocessor
// grants no release authority; publication separately requires a signed receipt.
async function completeCoreUpdateMetadata(result) {
  if (result.configuration?.extraMetadata?.releaseProfile !== 'core') return []
  const channel = result.configuration.extraMetadata.releaseChannel
  const name = channel === 'stable' ? 'latest-mac.yml' : channel === 'beta' ? 'beta-mac.yml' : ''
  if (!name) throw Error('core_update_channel_invalid')
  const path = join(result.outDir, name)
  const stat = lstatSync(path)
  if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1 || stat.size > 1024 * 1024) throw Error('core_update_metadata_untrusted')
  const metadata = yaml.load(readFileSync(path, 'utf8'))
  Object.assign(metadata, { analytixProfile: 'core', analytixTarget: 'darwin-arm64', analytixChannel: channel, analytixAppId: 'com.analytix.desktop', analytixDataCompatibility: 'core-v1' })
  verifyCoreUpdateArtifacts(result.outDir, metadata, { version: metadata.version, channel })
  writeFileSync(path, yaml.dump(metadata))
  return []
}
module.exports = completeCoreUpdateMetadata
module.exports.validateCoreUpdateMetadata = validateCoreUpdateMetadata
module.exports.verifyCoreUpdateArtifacts = verifyCoreUpdateArtifacts

if (require.main === module) {
  completeCoreUpdateMetadata({
    outDir: process.env.ANALYTIX_DIST_DIR || 'dist',
    configuration: { extraMetadata: { releaseProfile: 'core', releaseChannel: process.env.ANALYTIX_UPDATE_CHANNEL || 'stable' } }
  }).catch(() => { console.error('core_update_metadata_completion_failed'); process.exitCode = 1 })
}
