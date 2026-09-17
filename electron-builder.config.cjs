const { existsSync, readFileSync } = require('node:fs')
const { join } = require('node:path')
const { requireOfficialTeamIdentifier } = require('./scripts/macos-signing-policy.cjs')
const {
  loadProductionMcpEntryClosureContract
} = require('./plugins/analytix-fund-analysis/scripts/production-mcp-entry-closure-manifest.cjs')
const {
  assertNoPrepackagedCommandLine
} = require('./scripts/packaged-lifecycle-guard.cjs')._internals

assertNoPrepackagedCommandLine()

const productionFundsMcpEntryClosure = loadProductionMcpEntryClosureContract(
  join(__dirname, 'plugins', 'analytix-fund-analysis', 'scripts', 'production-mcp-entry-closure.json')
)
const productionFundsMcpEntryClosureFiles = productionFundsMcpEntryClosure.files

function loadLocalReleaseEnv() {
  if (process.env.ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE === 'isolated-local-v1') return
  const candidates = [
    process.env.ANALYTIX_RELEASE_ENV,
    join(__dirname, 'scripts', 'release.local.env'),
    join(__dirname, 'release.local.env')
  ].filter(Boolean)

  for (const candidate of candidates) {
    if (!existsSync(candidate)) continue
    for (const rawLine of readFileSync(candidate, 'utf8').split(/\r?\n/)) {
      const line = rawLine.trim()
      if (!line || line.startsWith('#')) continue
      const match = line.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$/)
      if (!match) continue
      let value = match[2].trim()
      if (
        (value.startsWith('"') && value.endsWith('"')) ||
        (value.startsWith("'") && value.endsWith("'"))
      ) {
        value = value.slice(1, -1)
      }
      if (!process.env[match[1]]) process.env[match[1]] = value
    }
    break
  }
}

loadLocalReleaseEnv()

const hasExplicitMacSigningIdentity = Boolean(
  process.env.CSC_LINK ||
    process.env.CSC_NAME ||
    process.env.CSC_KEY_PASSWORD ||
    process.env.MAC_SIGN === '1'
)

const hasNotaryToolCredentials = Boolean(
  process.env.APPLE_API_KEY_ID &&
    process.env.APPLE_API_ISSUER &&
    (process.env.APPLE_API_KEY || process.env.APPLE_API_KEY_BASE64)
)

if (hasExplicitMacSigningIdentity) {
  requireOfficialTeamIdentifier()
}
if (hasNotaryToolCredentials && !hasExplicitMacSigningIdentity) {
  throw new Error('Apple notarization credentials require an explicitly authorized Developer ID signing identity')
}

const defaultStableUpdateFeedUrl = 'https://analytix.top/desktop/releases/standard-win/'
const releaseBaseUrl = (process.env.ANALYTIX_RELEASE_BASE_URL || '')
  .trim()
  .replace(/\/+$/, '')
const updateChannel = normalizeUpdateChannel(
  process.env.ANALYTIX_UPDATE_CHANNEL || 'stable'
)
const genericUpdateUrl = (process.env.ANALYTIX_UPDATE_FEED_URL || process.env.ANALYTIX_DESKTOP_UPDATE_FEED_URL || '')
  .trim() ||
  (releaseBaseUrl ? `${releaseBaseUrl}/channels/${updateChannel}/latest/` : defaultStableUpdateFeedUrl)
const releaseAppVersion = (process.env.ANALYTIX_APP_VERSION || '').trim()
const artifactVersion = releaseAppVersion || '${version}'
const privatePwshRoot = join(__dirname, 'build', 'private-pwsh', 'pwsh')

function normalizeUpdateChannel(raw) {
  const value = String(raw || '').trim()
  if (value === 'stable' || value === 'beta') return value
  throw new Error(`ANALYTIX_UPDATE_CHANNEL must be "stable" or "beta", got: ${raw}`)
}

function privatePwshExtraResources() {
  if (!existsSync(join(privatePwshRoot, 'pwsh.exe'))) return []
  return [
    {
      from: privatePwshRoot,
      to: 'pwsh',
      filter: [
        '**/*',
        '!**/.DS_Store',
        '!**/._*'
      ]
    }
  ]
}

if (releaseAppVersion && !/^\d+\.\d+\.\d+$/.test(releaseAppVersion)) {
  throw new Error(
    `ANALYTIX_APP_VERSION must be a valid x.y.z semver for electron-updater, got: ${releaseAppVersion}`
  )
}

module.exports = {
  appId: 'com.analytix.desktop',
  productName: 'Analytix',
  executableName: 'analytix',
  protocols: [
    {
      name: 'Analytix OAuth Callback',
      schemes: ['com.analytix.desktop']
    }
  ],
  asar: true,
  asarUnpack: [
    'out/office-codec/**/*',
    '**/packages/runtime/dist/cli/**/*',
    '**/packages/runtime/dist/config/**/*',
    '**/packages/runtime/dist/contracts/**/*',
    '**/packages/runtime/dist/hooks/**/*',
    '**/packages/runtime/package*.json',
    '**/packages/runtime/node_modules/**/*',
    '**/node_modules/better-sqlite3/**/*',
    '**/node_modules/node-pty/**/*',
    '**/node_modules/bindings/**/*',
    '**/node_modules/file-uri-to-path/**/*',
    '**/node_modules/@computer-use/**/*',
    '**/node_modules/analytix-computer-use/**/*'
  ],
  npmRebuild: true,
  directories: {
    output: process.env.ANALYTIX_DIST_DIR || 'dist'
  },
  files: [
    'out/**/*',
    'package.json',
    'packages/runtime/dist/cli/**/*',
    'packages/runtime/dist/config/**/*',
    'packages/runtime/dist/contracts/**/*',
    'packages/runtime/dist/hooks/**/*',
    'packages/runtime/package.json',
    'packages/runtime/package-lock.json',
    'packages/runtime/node_modules/**/*',
    'node_modules/openclaw/LICENSE',
    '!**/*.map',
    '!**/*.d.ts',
    '!**/*.ts',
    '!**/tsconfig*.json',
    '!**/test/**',
    '!**/tests/**',
    '!**/__tests__/**',
    '!**/fixtures/**',
    '!**/__fixtures__/**',
    '!**/testsupport/**',
    '!**/conformance/**',
    '!**/upstreamaudit/**',
    '!**/readiness/**',
    '!**/benchmark/**',
    '!**/benchmarks/**',
    '!**/coverage/**',
    '!**/demo/**',
    '!**/demos/**',
    '!**/doc/**',
    '!**/docs/**',
    '!**/documentation/**',
    '!**/example/**',
    '!**/examples/**',
    '!**/README*',
    '!**/Readme*',
    '!**/readme*',
    '!**/CHANGELOG*',
    '!**/ChangeLog*',
    '!**/changelog*',
    '!**/.DS_Store',
    '!**/._*'
    // node_modules/openclaw (the vendor/openclaw-shim file: dep) must ship:
    // the WeChat bridge imports @tencent-weixin/openclaw-weixin/dist at
    // runtime to send media, and that chain resolves openclaw/plugin-sdk/*.
  ],
  extraResources: [
    {
      from: 'LICENSE',
      to: 'LICENSE'
    },
    {
      from: 'THIRD_PARTY_NOTICES.md',
      to: 'THIRD_PARTY_NOTICES.md'
    },
    ...privatePwshExtraResources(),
    {
      from: 'backend',
      to: 'backend',
      filter: [
        '**/*',
        '!**/.DS_Store',
        '!**/._*',
        '!**/.venv/**',
        '!**/.pytest_cache/**',
        '!**/tests/**',
        '!**/test/**',
        '!**/__pycache__/**',
        '!**/*.pyc'
      ]
    },
    {
      from: 'runtime',
      to: 'runtime',
      filter: [
        '7za.exe',
        'analytix-archive-extractor-manifest.json',
        '!**/.DS_Store',
        '!**/._*'
      ]
    },
    {
      from: 'plugins/analytix-fund-analysis',
      to: 'plugins/analytix-fund-analysis',
      filter: [
        '.analytix-plugin/package.json',
        '.codex-plugin/plugin.json',
        '.mcp.json',
        'agents/**/*',
        'assets/**/*',
        'references/**/*',
        'skills/**/*',
        ...productionFundsMcpEntryClosureFiles,
        '!**/.DS_Store',
        '!**/._*'
      ]
    },
    {
      from: 'node_modules/@fontsource/noto-serif-sc/files',
      to: 'fonts/noto-serif-sc',
      filter: [
        'noto-serif-sc-chinese-simplified-400-normal.woff2',
        'noto-serif-sc-chinese-simplified-700-normal.woff2'
      ]
    }
  ],
  artifactName: `analytix-${artifactVersion}-\${os}-\${arch}.\${ext}`,
  publish: [
    {
      provider: 'generic',
      url: genericUpdateUrl
    }
  ],
  artifactBuildStarted: './scripts/packaged-lifecycle-guard.cjs',
  artifactBuildCompleted: './scripts/packaged-lifecycle-guard.cjs',
  afterExtract: './scripts/after-extract.cjs',
  afterPack: './scripts/after-pack.cjs',
  afterSign: './scripts/mac-notarize.cjs',
  mac: {
    category: 'public.app-category.developer-tools',
    identity: hasExplicitMacSigningIdentity ? undefined : '-',
    // We notarize in scripts/mac-notarize.cjs so APPLE_API_KEY_BASE64 can be supported.
    notarize: false,
    hardenedRuntime: true,
    forceCodeSigning: true,
    timestamp: hasExplicitMacSigningIdentity ? 'http://timestamp.apple.com/ts01' : null,
    gatekeeperAssess: false,
    preAutoEntitlements: false,
    sign: './scripts/mac-sign.cjs',
    entitlements: 'build/entitlements.mac.plist',
    entitlementsInherit: 'build/entitlements.mac.inherit.plist',
    extendInfo: {
      // 语音输入：渲染进程通过 getUserMedia 录音做语音转文字。
      NSMicrophoneUsageDescription: 'analytix uses the microphone for voice-to-text input.'
    },
    // macOS app/dock/installer icon generated from the approved analytix app icon source.
    icon: './build/icon.icns',
    // arm64 (Apple Silicon) + x64 (Intel). On M 系列 Mac 本地打包会各出一组 dmg/zip。
    target: [
      { target: 'dmg', arch: ['arm64', 'x64'] },
      { target: 'zip', arch: ['arm64', 'x64'] }
    ]
  },
  dmg: {
    sign: hasExplicitMacSigningIdentity
  },
  win: {
    // Ship a multi-size .ico (16/24/32/48/64/72/96/128/256) so Explorer and
    // the desktop render crisp icons at small sizes.
    icon: './build/icon.ico',
    target: [{ target: 'nsis', arch: ['x64'] }]
  },
  nsis: {
    oneClick: false,
    allowToChangeInstallationDirectory: true,
    perMachine: false,
    allowElevation: true,
    selectPerMachineByDefault: false,
    include: 'build/installer.nsh',
    // 明确创建快捷方式；always 在覆盖安装时也会重建（即使用户曾删掉桌面图标）
    createDesktopShortcut: 'always',
    createStartMenuShortcut: true,
    shortcutName: 'Analytix',
    uninstallDisplayName: 'Analytix',
    deleteAppDataOnUninstall: false
  },
  linux: {
    category: 'Development',
    icon: './src/asset/brand/analytix-app-icon-512.png',
    target: [{ target: 'AppImage', arch: ['x64'] }]
  },
  extraMetadata: {
    ...(releaseAppVersion ? { version: releaseAppVersion } : {}),
    updateChannel,
    buildHints: {
      macSigningEnabled: hasExplicitMacSigningIdentity,
      notarizationEnabled: hasNotaryToolCredentials
    }
  }
}
