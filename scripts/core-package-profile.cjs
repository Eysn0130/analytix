const { lstatSync } = require('node:fs')
const { join } = require('node:path')

const CORE_CONTROLLED_DISPOSITION = 'core_controlled_release'
const isCoreDisposition = value => [CORE_DISPOSITION, CORE_CONTROLLED_DISPOSITION].includes(value?.kind)

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
const CORE_OPTIONAL_ASSET_EXCLUSIONS = Object.freeze([
  // PDF.js uses browser canvas for its renderer and needs only text extraction in Main.
  '!node_modules/@napi-rs/canvas/**',
  '!node_modules/@napi-rs/canvas-*/**',
  // This is a KaTeX font-build input; runtime styles reference dist/fonts instead.
  '!node_modules/katex/src/fonts/lib/Extra.otf'
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

function assertCoreOptionalAssetsAbsent(reader) {
  if (reader.entries().some(entry =>
    /(?:^|\/)node_modules\/@napi-rs\/canvas(?:-[^/]+)?\//.test(entry) ||
    /(?:^|\/)node_modules\/katex\/src\/fonts\/lib\/Extra\.otf$/.test(entry)
  )) throw Error('core_optional_asset_present')
}

function applyReleaseProfile(config, profile) {
  profile = releaseProfile(profile)
  config.extraMetadata = { ...config.extraMetadata, releaseProfile: profile }
  if (profile === 'core') {
    if (!Array.isArray(config.files)) throw Error('core_profile_files_invalid')
    config.files = [...config.files, ...CORE_OPTIONAL_ASSET_EXCLUSIONS]
    config.extraResources = config.extraResources.filter(resource =>
      !['backend', 'plugins/analytix-fund-analysis'].includes(resource.to))
    config.artifactName = config.artifactName.replace('analytix-', 'analytix-core-')
    const channel = process.env.ANALYTIX_UPDATE_CHANNEL || 'stable'
    if (!['stable', 'beta'].includes(channel)) throw Error('core_update_channel_invalid')
    config.extraMetadata.releaseChannel = channel
    const base = (process.env.ANALYTIX_RELEASE_BASE_URL || 'https://analytix.top/desktop/releases').trim().replace(/\/+$/, '')
    if (Array.isArray(config.publish)) config.publish = config.publish.map(publisher => ({ ...publisher, url: `${base}/core/darwin-arm64/channels/${channel}/latest/`, channel: channel === 'stable' ? 'latest' : 'beta' }))
  }
  return config
}

module.exports = { CORE_CONTROLLED_DISPOSITION, isCoreDisposition, CORE_DISPOSITION, ABSENT_FUNDS, EXCLUDED_RESOURCES, CORE_OPTIONAL_ASSET_EXCLUSIONS, releaseProfile, isCoreContext, assertCoreResourcesAbsent, assertCoreOptionalAssetsAbsent, applyReleaseProfile }
