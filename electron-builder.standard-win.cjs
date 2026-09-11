const baseConfig = require('./electron-builder.config.cjs')
const { existsSync } = require('node:fs')
const { join } = require('node:path')

const DEFAULT_STANDARD_WIN_UPDATE_FEED_URL = 'https://analytix.top/desktop/releases/standard-win/'
const WINDOWS_BACKEND_RUNTIME_ROOT = join(__dirname, 'build', 'windows-backend-runtime')

function clean(value) {
  return String(value || '').trim()
}

function standardWindowsUpdateFeedUrl(env = process.env) {
  const updateFeedUrl =
    clean(env.ANALYTIX_STANDARD_WIN_UPDATE_FEED_URL || env.ANALYTIX_DESKTOP_UPDATE_FEED_URL || env.ANALYTIX_UPDATE_FEED_URL) ||
    DEFAULT_STANDARD_WIN_UPDATE_FEED_URL
  let parsed
  try {
    parsed = new URL(updateFeedUrl)
  } catch {
    throw new Error(`Standard Windows update feed URL is invalid: ${updateFeedUrl}`)
  }
  if (parsed.protocol !== 'https:') {
    throw new Error(`Standard Windows update feed URL must use https: ${updateFeedUrl}`)
  }
  return updateFeedUrl
}

function windowsBackendRuntimeExtraResources() {
  const pythonRuntimeRoot = join(WINDOWS_BACKEND_RUNTIME_ROOT, '.python-runtime')
  const sitePackagesRoot = join(WINDOWS_BACKEND_RUNTIME_ROOT, 'python-site-packages')
  if (!existsSync(pythonRuntimeRoot) || !existsSync(sitePackagesRoot)) {
    return []
  }
  const commonFilters = [
    '**/*',
    '!**/.DS_Store',
    '!**/._*',
    '!**/__pycache__/**',
    '!**/*.pyc',
    '!**/tests/**',
    '!**/test/**'
  ]
  return [
    {
      from: pythonRuntimeRoot,
      to: '.python-runtime',
      filter: commonFilters
    },
    {
      from: sitePackagesRoot,
      to: 'python-site-packages',
      filter: commonFilters
    }
  ]
}

function managedChromeWindowsExtraResources() {
  const managedChromeRoot = join(__dirname, 'managed-chrome')
  if (!existsSync(managedChromeRoot)) return []
  return [
    {
      from: managedChromeRoot,
      to: 'managed-chrome',
      filter: [
        'codex-extension/**/*',
        'chrome/extension-host/windows/x64/extension-host.exe',
        'chrome/scripts/browser-client.mjs',
        '!**/.DS_Store',
        '!**/._*'
      ]
    }
  ]
}

module.exports = {
  ...baseConfig,
  productName: 'Analytix灵鉴',
  executableName: 'analytix',
  extraResources: [
    ...(baseConfig.extraResources || []),
    ...managedChromeWindowsExtraResources(),
    ...windowsBackendRuntimeExtraResources()
  ],
  directories: {
    ...baseConfig.directories,
    output: process.env.ANALYTIX_STANDARD_WIN_DIST_DIR || 'dist-standard-win'
  },
  artifactName: 'analytix-standard-${version}-${arch}.${ext}',
  publish: [
    {
      provider: 'generic',
      url: standardWindowsUpdateFeedUrl()
    }
  ],
  win: {
    ...baseConfig.win,
    forceCodeSigning: true,
    signtoolOptions: {
      ...(baseConfig.win?.signtoolOptions || {}),
      signingHashAlgorithms: ['sha256'],
      rfc3161TimeStampServer: 'http://timestamp.digicert.com'
    },
    target: [{ target: 'nsis', arch: ['x64'] }]
  },
  nsis: {
    ...(baseConfig.nsis || {}),
    oneClick: true,
    perMachine: true,
    allowElevation: true,
    packElevateHelper: true,
    allowToChangeInstallationDirectory: false,
    runAfterFinish: false,
    createDesktopShortcut: true,
    createStartMenuShortcut: true,
    shortcutName: 'Analytix灵鉴',
    menuCategory: 'Analytix灵鉴',
    uninstallDisplayName: 'Analytix灵鉴',
    include: 'build/standard-win-installer.nsh'
  }
}
