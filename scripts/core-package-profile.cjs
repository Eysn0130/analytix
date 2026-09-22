const { lstatSync } = require('node:fs')
const { join } = require('node:path')

const CORE_DISPOSITION = 'core_no_professional_components'
const ABSENT_FUNDS = Object.freeze({ treeSha256: '', fileCount: 0, manifestSha256: '', entrypointSha256: '', totalBytes: 0 })
const EXCLUDED_RESOURCES = Object.freeze([
  'backend', 'plugins/analytix-fund-analysis', 'office-private',
  'runtime/document-runtime', 'runtime/native-components',
  'runtime/analytix-native-development-build.json',
  'runtime/analytix-native-components-receipt.json',
  ...['import-accelerator', 'cleaning-ops', 'analysis-compute', 'data-engine'].flatMap(
    name => [`runtime/analytix-${name}`, `runtime/analytix-${name}.exe`]
  )
])

function releaseProfile(value = 'full') {
  if (value !== 'full' && value !== 'core') throw Error('release_profile_invalid')
  return value
}

function isCoreContext(context) {
  return releaseProfile(context?.packager?.config?.extraMetadata?.releaseProfile) === 'core'
}

function assertCoreResourcesAbsent(resources) {
  for (const name of EXCLUDED_RESOURCES) {
    try { lstatSync(join(resources, name)) } catch (error) {
      if (error.code === 'ENOENT') continue
      throw error
    }
    throw Error('core_profile_professional_resource_present')
  }
}

function applyReleaseProfile(config, profile) {
  profile = releaseProfile(profile)
  config.extraMetadata = { ...config.extraMetadata, releaseProfile: profile }
  if (profile === 'core') {
    config.extraResources = config.extraResources.filter(resource =>
      !['backend', 'plugins/analytix-fund-analysis'].includes(resource.to))
    config.artifactName = config.artifactName.replace('analytix-', 'analytix-core-')
  }
  return config
}

module.exports = { CORE_DISPOSITION, ABSENT_FUNDS, EXCLUDED_RESOURCES, releaseProfile, isCoreContext, assertCoreResourcesAbsent, applyReleaseProfile }
