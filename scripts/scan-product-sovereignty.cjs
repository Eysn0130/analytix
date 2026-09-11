#!/usr/bin/env node

const { spawnSync } = require('node:child_process')
const { existsSync, readFileSync } = require('node:fs')
const {
  manifest: nativeComponentManifest,
  sourceSetDigest: nativeSourceSetDigest
} = require('./native-component-contract.cjs')

const rg = 'rg'

function run(command, args) {
  const result = spawnSync(command, args, {
    cwd: process.cwd(),
    encoding: 'utf8'
  })
  if (result.error) {
    throw result.error
  }
  return result
}

function fail(name, message) {
  console.error(`\n[scan:product-sovereignty] ${name} failed`)
  console.error(message.trimEnd())
  process.exitCode = 1
}

function expectNoRgMatches(name, pattern, paths, globs = []) {
  const args = ['-n', pattern, ...paths]
  for (const glob of globs) {
    args.push('--glob', glob)
  }
  const result = run(rg, args)
  if (result.status === 0) {
    fail(name, `${result.stdout}${result.stderr}`)
    return
  }
  if (result.status !== 1) {
    fail(name, `${result.stdout}${result.stderr}`)
  }
}

function expectNoRgMatchesExcept(name, pattern, paths, globs = [], allowedLinePatterns = []) {
  const args = ['-n', pattern, ...paths]
  for (const glob of globs) {
    args.push('--glob', glob)
  }
  const result = run(rg, args)
  if (result.status === 1) return
  if (result.status !== 0) {
    fail(name, `${result.stdout}${result.stderr}`)
    return
  }
  const allowed = allowedLinePatterns.map((item) => new RegExp(item))
  const violations = result.stdout
    .split(/\r?\n/)
    .filter((line) => line && !allowed.some((pattern) => pattern.test(line)))
  if (violations.length > 0) {
    fail(name, violations.join('\n'))
  }
}

function expectRgMatches(name, pattern, paths, globs = []) {
  const args = ['-n']
  for (const glob of globs) {
    args.push('--glob', glob)
  }
  args.push('--', pattern, ...paths)
  const result = run(rg, args)
  if (result.status === 1) {
    fail(name, `No matches for ${pattern}`)
    return
  }
  if (result.status !== 0) {
    fail(name, `${result.stdout}${result.stderr}`)
  }
}

function expectAllRgMatches(name, patterns, paths, globs = []) {
  for (const pattern of patterns) {
    expectRgMatches(`${name}: ${pattern}`, pattern, paths, globs)
  }
}

function expectWindowAnalytixApiAllowList(name, pattern, allowedMethods, paths, globs = []) {
  const args = ['-n', pattern, ...paths]
  for (const glob of globs) {
    args.push('--glob', glob)
  }
  const result = run(rg, args)
  if (result.status === 1) return
  if (result.status !== 0) {
    fail(name, `${result.stdout}${result.stderr}`)
    return
  }
  const allowed = new Set(allowedMethods)
  const violations = []
  for (const line of result.stdout.split(/\r?\n/)) {
    if (!line) continue
    const match = line.match(/window\.analytix(?:\?\.|\.)(?:runtime|settings)(?:\?\.|\.)([A-Za-z0-9_]+)/)
    if (match && !allowed.has(match[1])) {
      violations.push(line)
    }
  }
  if (violations.length > 0) {
    fail(name, violations.join('\n'))
  }
}

function expectNoFileMatches(name, pattern) {
  const result = run(rg, ['--files'])
  if (result.status !== 0) {
    fail(name, `${result.stdout}${result.stderr}`)
    return
  }
  const regex = new RegExp(pattern)
  const matches = result.stdout.split(/\r?\n/).filter((file) => file && regex.test(file))
  if (matches.length > 0) {
    fail(name, matches.join('\n'))
  }
}

function expectNativeComponentRegistry(name) {
  const violations = []
  try {
    nativeSourceSetDigest(process.cwd())
  } catch (error) {
    violations.push(error instanceof Error ? error.message : String(error))
  }
  const rootKeys = Object.keys(nativeComponentManifest).sort().join(',')
  if (
    rootKeys !== 'components,receiptSchemaVersion,schemaVersion' ||
    nativeComponentManifest.schemaVersion !== 4 ||
    nativeComponentManifest.receiptSchemaVersion !== 6
  ) {
    violations.push('scripts/native-components.json: invalid top-level schema')
  }
  const components = Array.isArray(nativeComponentManifest.components)
    ? nativeComponentManifest.components
    : []
  const expectedKeys = [
    'agentCore',
    'authorization',
    'binaryName',
    'cargoManifest',
    'consumers',
    'executionProbe',
    'id',
    'packagePath',
    'role',
    'sourceRoot',
    'supportedTargets'
  ].join(',')
  const roots = new Set()
  const binaries = new Set()
  const registeredCargoManifests = new Set()
  const frozenBoundaries = [
    ['import-accelerator', 'tools/import_accelerator', 'analytix-import-accelerator', 'existing-data-plane-boundary@33d6de1d3c40691b73033f698af8bb518b15300d'],
    ['cleaning-ops', 'tools/cleaning_ops', 'analytix-cleaning-ops', 'existing-data-plane-boundary@33d6de1d3c40691b73033f698af8bb518b15300d'],
    ['analysis-compute', 'tools/analysis_compute', 'analytix-analysis-compute', 'existing-data-plane-boundary@33d6de1d3c40691b73033f698af8bb518b15300d'],
    ['data-engine', 'tools/data_engine', 'analytix-data-engine', 'existing-data-plane-boundary@af0ea1967c76c7b771aaff1c0d5606ca4d5b832c']
  ]
  const probePolicies = [
    '0cc3872164fdadfb687a8642846913bf0f2394cd21c26799a5d483dee1ea8d2a',
    '59c4287d312b4972eb6890883865e3c37d226650caf78f65168363413ac85e35',
    'fd2227234c61de1505e50aa33e1e24c2f58c2545fe4d07e41b8064709c18f4d8',
    '97ce13396fb6f5ba7e9d851ab4abc2f23ac8abdd13acaa3e983a326d6bf4a1e0'
  ]
  if (components.length !== frozenBoundaries.length) {
    violations.push('scripts/native-components.json: component inventory is not the frozen four-boundary set')
  }
  for (let componentIndex = 0; componentIndex < components.length; componentIndex += 1) {
    const component = components[componentIndex]
    if (!component || typeof component !== 'object' || Array.isArray(component)) {
      violations.push('scripts/native-components.json: component must be an object')
      continue
    }
    if (Object.keys(component).sort().join(',') !== expectedKeys) {
      violations.push(`scripts/native-components.json:${component.id || '<unknown>'}: invalid component schema`)
      continue
    }
    const root = String(component.sourceRoot || '').replace(/\/$/, '')
    const binary = String(component.binaryName || '')
    const frozen = frozenBoundaries[componentIndex]
    if (
      !frozen ||
      component.id !== frozen[0] ||
      root !== frozen[1] ||
      binary !== frozen[2] ||
      component.authorization !== frozen[3]
    ) {
      violations.push(`scripts/native-components.json:${component.id}: boundary is not in the frozen authorized inventory`)
    }
    if (!/^tools\/[a-z0-9_]+$/.test(root) || roots.has(root)) {
      violations.push(`scripts/native-components.json:${component.id}: invalid or duplicate sourceRoot`)
    }
    if (!/^analytix-[a-z0-9-]+$/.test(binary) || binaries.has(binary)) {
      violations.push(`scripts/native-components.json:${component.id}: invalid or duplicate binaryName`)
    }
    roots.add(root)
    binaries.add(binary)
    const cargoManifest = `${root}/Cargo.toml`
    registeredCargoManifests.add(cargoManifest)
    if (component.cargoManifest !== cargoManifest || !existsSync(cargoManifest)) {
      violations.push(`scripts/native-components.json:${component.id}: cargoManifest must be ${cargoManifest}`)
    } else if (!new RegExp(`(?:^|\\n)name\\s*=\\s*["']${binary}["']`, 'm').test(readTextFile(cargoManifest))) {
      violations.push(`${cargoManifest}: package/bin identity does not match ${binary}`)
    }
    if (component.packagePath !== `runtime/${binary}`) {
      violations.push(`scripts/native-components.json:${component.id}: invalid packagePath`)
    }
    if (component.agentCore !== false) {
      violations.push(`scripts/native-components.json:${component.id}: agentCore must be false`)
    }
    if (!/^(?:immutable-data-import|case-data-cleaning|bounded-analysis-compute|single-owner-case-database)$/.test(String(component.role))) {
      violations.push(`scripts/native-components.json:${component.id}: unapproved data-plane role`)
    }
    if (!/^existing-data-plane-boundary@[0-9a-f]{40}$/.test(String(component.authorization))) {
      violations.push(`scripts/native-components.json:${component.id}: missing immutable authorization commit`)
    }
    const expectedTargets = ['darwin-arm64', 'darwin-x64', 'linux-x64', 'win32-x64']
    if (!Array.isArray(component.supportedTargets) || component.supportedTargets.join(',') !== expectedTargets.join(',')) {
      violations.push(`scripts/native-components.json:${component.id}: unsupported target declaration`)
    }
    const probe = component.executionProbe
    if (
      !probe ||
      typeof probe !== 'object' ||
      Array.isArray(probe) ||
      Object.keys(probe).sort().join(',') !== 'authorityProtocol,authorityTargets,componentProtocol,policySha256,schemaVersion' ||
      probe.schemaVersion !== 1 ||
      probe.authorityProtocol !== 'analytix-native-build-probe-v1' ||
      probe.componentProtocol !== 'analytix-native-v1' ||
      probe.policySha256 !== probePolicies[componentIndex] ||
      !Array.isArray(probe.authorityTargets) ||
      probe.authorityTargets.join(',') !== 'darwin-arm64,darwin-x64'
    ) {
      violations.push(`scripts/native-components.json:${component.id}: invalid execution probe`)
    }
    if (!Array.isArray(component.consumers) || component.consumers.length === 0) {
      violations.push(`scripts/native-components.json:${component.id}: consumers must be non-empty`)
    } else {
      let binaryConsumerFound = false
      for (const consumer of component.consumers) {
        if (!/^(?:backend\/|src\/main\/data-analysis\/|packages\/runtime-go\/internal\/adapters\/outbound\/(?:nativecomponentregistry|nativecomponentrunner)\/)/.test(consumer) || !existsSync(consumer)) {
          violations.push(`scripts/native-components.json:${component.id}: invalid data-plane consumer ${consumer}`)
        } else if (readTextFile(consumer).includes(binary)) {
          binaryConsumerFound = true
        }
      }
      if (!binaryConsumerFound) {
        violations.push(`scripts/native-components.json:${component.id}: no declared consumer resolves ${binary}`)
      }
    }
  }

  const fileResult = run(rg, ['--files'])
  if (fileResult.status !== 0) {
    violations.push(`${fileResult.stdout}${fileResult.stderr}`)
  } else {
    const files = fileResult.stdout.split(/\r?\n/).filter(Boolean)
    for (const file of files) {
      if (/(^|\/)(?:src-tauri\/|tauri\.conf\.(?:json|json5)$)/i.test(file)) {
        violations.push(`${file}: Tauri is forbidden in every source root`)
        continue
      }
      if (!/(^|\/)(?:Cargo\.toml|Cargo\.lock|.*\.rs)$/i.test(file)) continue
      const ownerRoot = [...roots].find((root) => file.startsWith(`${root}/`))
      if (!ownerRoot) {
        violations.push(`${file}: Rust/Cargo file is outside the native component registry`)
        continue
      }
      if (file.endsWith('/Cargo.toml') && !registeredCargoManifests.has(file)) {
        violations.push(`${file}: nested or additional Cargo manifest is not authorized`)
      }
    }
  }

  const runtimeReferences = run(rg, [
    '-n',
    [...binaries].join('|'),
    'packages/runtime-go',
    'packages/runtime',
    'src/main/runtime',
    '--glob',
    '!**/*test*'
  ])
  if (runtimeReferences.status === 0) {
    const allowedGoAuthorityReferences = new Set([
      'packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/manifest.go',
      'packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/registry.go',
      'packages/runtime-go/internal/adapters/outbound/nativecomponentrunner/protocol.go',
      'packages/runtime-go/internal/adapters/outbound/nativecomponentrunner/runner_darwin.go',
      'packages/runtime-go/internal/adapters/outbound/nativecomponentpublication/publisher_darwin.go',
      'packages/runtime-go/internal/domain/packagedbuildauthority/authority_v2.go'
    ])
    const undeclared = runtimeReferences.stdout.split(/\r?\n/).filter(Boolean).filter((line) => {
      const separator = line.indexOf(':')
      return separator < 0 || !allowedGoAuthorityReferences.has(line.slice(0, separator))
    })
    if (undeclared.length > 0) {
      violations.push(`Data-plane helpers may be referenced only by the frozen Go execution authority:\n${undeclared.join('\n')}`)
    }
    const publisherPath = 'packages/runtime-go/internal/adapters/outbound/nativecomponentpublication/publisher_darwin.go'
    const publisherFirstLine = readTextFile(publisherPath).split(/\r?\n/, 1)[0]
    if (publisherFirstLine !== '//go:build darwin && analytix_native_build_probe && !analytix_prod') {
      violations.push(`${publisherPath}: build-only publisher must be excluded from every production source set`)
    }
  } else if (runtimeReferences.status !== 1) {
    violations.push(`${runtimeReferences.stdout}${runtimeReferences.stderr}`)
  }

  if (violations.length > 0) fail(name, violations.join('\n'))
}

function expectAppRouteAllowList(name, path, allowedRoutes) {
  const text = readTextFile(path)
  const match = text.match(/export type AppRoute\s*=\s*([^\n]+)/)
  if (!match) {
    fail(name, `${path}: AppRoute declaration is missing`)
    return
  }
  const actual = [...match[1].matchAll(/['"]([^'"]+)['"]/g)].map((item) => item[1])
  if (actual.join(',') !== allowedRoutes.join(',')) {
    fail(name, `${path}: expected ${allowedRoutes.join(',')}; got ${actual.join(',')}`)
  }
}

function isLegacyProductSurfaceFileName(file) {
  const segments = String(file).replaceAll('\\', '/').split('/').filter(Boolean)
  const legacySegment = /^(?:d0(?:24|25)[a-z0-9_-]*|(?:proof|oracle|fake|mcpLocalProof)(?:[-_][a-z0-9_-]+)?)$/i
  const directorySegments = segments.slice(0, -1)
  const fileStem = String(segments.at(-1) || '').split('.')[0]
  return directorySegments.some((segment) => legacySegment.test(segment)) || legacySegment.test(fileStem)
}

function expectNoLegacyProductSurfaceFileNames(name, paths) {
  const matches = []
  for (const path of paths) {
    if (!existsSync(path)) continue
    const result = run(rg, ['--files', path])
    if (result.status !== 0) {
      fail(name, `${result.stdout}${result.stderr}`)
      return
    }
    matches.push(...result.stdout.split(/\r?\n/).filter((file) => file && isLegacyProductSurfaceFileName(file)))
  }
  if (matches.length > 0) {
    fail(name, [...new Set(matches)].sort().join('\n'))
  }
}

const formalEvidenceRuntimeBackendCountAllowedLine =
  '^scripts/runtime-go-formal-evidence\\.mjs:\\d+:\\s+seam\\.runtimeBackendProcessCount === 1 && seam\\.runtimeServerProcessCount === 1 &&$'

function legacyKunSettingsMigrationLineAllowed(line) {
  return legacyKunSettingsMigrationAllowedLinePatterns.some((pattern) => new RegExp(pattern).test(line))
}

function sourceClassificationSelfTest() {
  const backendCountAllowed = new RegExp(formalEvidenceRuntimeBackendCountAllowedLine)
  const cases = {
    productionBindingProofAllowed: !isLegacyProductSurfaceFileName(
      'packages/runtime-go/internal/domain/toolresult/case_source_binding_proof.go'
    ),
    standaloneProofRejected: isLegacyProductSurfaceFileName('scripts/proof.mjs'),
    standaloneOracleRejected: isLegacyProductSurfaceFileName('src/main/oracle-helper.ts'),
    standaloneFakeRejected: isLegacyProductSurfaceFileName('src/shared/fake.ts'),
    numberedLegacyStageRejected: isLegacyProductSurfaceFileName('scripts/d0251-live.mjs'),
    exactFormalEvaluatorAllowed: backendCountAllowed.test(
      'scripts/runtime-go-formal-evidence.mjs:524:    seam.runtimeBackendProcessCount === 1 && seam.runtimeServerProcessCount === 1 &&'
    ),
    selectorAssignmentStillRejected: !backendCountAllowed.test(
      'scripts/runtime-go-formal-evidence.mjs:524:    runtimeBackend = true'
    ),
    incompleteProcessCountStillRejected: !backendCountAllowed.test(
      'scripts/runtime-go-formal-evidence.mjs:524:    seam.runtimeBackendProcessCount === 1 &&'
    ),
    collectKunCredentialsAgentsReadAllowed: legacyKunSettingsMigrationLineAllowed(
      'src/main/settings-store.ts:1345:  const kun = isRecord(agents) ? agents.kun : undefined'
    ),
    collectKunCredentialsLocatorAllowed: legacyKunSettingsMigrationLineAllowed(
      'src/main/settings-store.ts:1355:      `${sourceLocator}:agents.kun.apiKey`,'
    ),
    inspectKunSettingsFileComparisonAllowed: legacyKunSettingsMigrationLineAllowed(
      'src/main/settings-store.ts:1868:      const isKunSource = basename(loaded.sourcePath) === LEGACY_KUN_SETTINGS_FILE_NAME'
    ),
    cleanupKunSettingsFileComparisonAllowed: legacyKunSettingsMigrationLineAllowed(
      'src/main/settings-store.ts:1927:        basename(loaded.sourcePath) === LEGACY_KUN_SETTINGS_FILE_NAME,'
    ),
    privateLegacyMigrationLocatorAllowed: legacyKunSettingsMigrationLineAllowed(
      "src/main/provider-registry-legacy-migration.ts:58:  'agents.kun.apiKey',"
    ),
    anotherPathStillRejected: !legacyKunSettingsMigrationLineAllowed(
      'src/main/ordinary-provider.ts:1345:  const kun = isRecord(agents) ? agents.kun : undefined'
    ),
    ordinaryAgentsKunUseStillRejected: !legacyKunSettingsMigrationLineAllowed(
      'src/main/settings-store.ts:1400:  ordinaryProvider = agents.kun'
    ),
    windowKunStillRejected: !legacyKunSettingsMigrationLineAllowed(
      'src/main/settings-store.ts:1401:  window.kun.start()'
    ),
    kunAgentProviderStillRejected: !legacyKunSettingsMigrationLineAllowed(
      "src/main/settings-store.ts:1402:  agentProvider = 'kun'"
    ),
    kunEnvironmentStillRejected: !legacyKunSettingsMigrationLineAllowed(
      'src/main/settings-store.ts:1403:  const KUN_TOKEN = value'
    )
  }
  const failedCaseIds = Object.entries(cases)
    .filter(([, passed]) => !passed)
    .map(([id]) => id)
  if (failedCaseIds.length > 0) {
    throw new Error(`product sovereignty source classification self-test failed: ${failedCaseIds.join(', ')}`)
  }
  return {
    id: 'analytix-product-sovereignty-source-classification-self-test',
    status: 'passed',
    passed: true,
    cases
  }
}

function expectFilesPresent(name, paths) {
  const missing = [...new Set(paths)].filter((file) => !existsSync(file))
  if (missing.length > 0) {
    fail(name, missing.join('\n'))
  }
}

function expectFilesAbsent(name, paths) {
  const present = [...new Set(paths)].filter((file) => existsSync(file))
  if (present.length > 0) {
    fail(name, present.join('\n'))
  }
}

function expectGoReplaySourcesExcludedFromProd(name, rootPath) {
  const result = run(rg, ['--files', rootPath])
  if (result.status !== 0) {
    fail(name, `${result.stdout}${result.stderr}`)
    return
  }
  const replayMarker = /\b(?:fixtureOnly|FixtureOnly|LiveLocal|liveLocal|fixtureBacked|\/v1\/conformance)\b/
  const violations = []
  for (const file of result.stdout.split(/\r?\n/)) {
    if (!file || !file.endsWith('.go') || file.endsWith('_test.go')) continue
    const text = readTextFile(file)
    if (!replayMarker.test(text)) continue
    const firstLines = text.split(/\r?\n/).slice(0, 12).join('\n')
    if (!firstLines.includes('!analytix_prod')) {
      violations.push(file)
    }
  }
  if (violations.length > 0) {
    fail(name, violations.join('\n'))
  }
}

function expectArchiveManifestCoversPaths(name, manifestPath, paths) {
  const text = readTextFile(manifestPath)
  const missing = []
  for (const file of [...new Set(paths)]) {
    const exact = `\`${file}\``
    const exactDir = `\`${file}/\``
    const segments = file.split('/')
    const coveredByDir = segments
      .map((_, index) => segments.slice(0, index + 1).join('/'))
      .filter((candidate) => candidate && file.startsWith(`${candidate}/`))
      .some((candidate) => text.includes(`\`${candidate}/\``))
    if (!text.includes(exact) && !text.includes(exactDir) && !coveredByDir) {
      missing.push(file)
    }
  }
  if (missing.length > 0) {
    fail(name, missing.join('\n'))
  }
}

function discoverLegacyStageEvidencePaths(name) {
  const result = run(rg, ['--files', 'docs/analytix/upstreams'])
  if (result.status !== 0) {
    fail(name, `${result.stdout}${result.stderr}`)
    return []
  }
  const legacyPathPattern = /(?:d0(?:24|25)|proof|oracle)/i
  return [...new Set(result.stdout
    .split(/\r?\n/)
    .filter((file) => file && legacyPathPattern.test(file)))]
    .sort()
}

function existingPaths(paths) {
  return [...new Set(paths)].filter((file) => existsSync(file))
}

function readTextFile(file) {
  return readFileSync(file, 'utf8')
}

function runInCwd(command, args, cwd) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: 'utf8'
  })
  if (result.error) {
    throw result.error
  }
  return result
}

function expectGoAnalytixProdSourceSetClean(name) {
  const result = runInCwd('go', [
    'list',
    '-tags',
    'analytix_prod',
    '-f',
    '{{.ImportPath}}\t{{.Dir}}\t{{range .GoFiles}}{{.}} {{end}}',
    './cmd/runtime-server',
    './internal/server',
    '.'
  ], 'packages/runtime-go')
  if (result.status !== 0) {
    fail(name, `${result.stdout}${result.stderr}`)
    return
  }

  const forbiddenFiles = [
    {
      importPath: 'analytix.local/runtime-go',
      file: 'runtime_server.go'
    },
    {
      importPath: 'analytix.local/runtime-go/internal/server',
      file: 'durable_store_candidate.go'
    },
    {
      importPath: 'analytix.local/runtime-go/internal/server',
      file: 'runtime_server_config.go'
    }
  ]
  const forbiddenToken = /\b(?:NewRuntimeServerContractHandler|RuntimeServerContractConfig|CandidateDurableRoot)\b|--candidate-durable-root|contract-sidecar/
  const violations = []

  for (const line of result.stdout.split(/\r?\n/)) {
    if (!line.trim()) continue
    const [importPath, dir, filesText = ''] = line.split('\t')
    const files = filesText.trim().split(/\s+/).filter(Boolean)
    for (const file of files) {
      if (forbiddenFiles.some((entry) => entry.importPath === importPath && entry.file === file)) {
        violations.push(`${importPath}: ${file} included in analytix_prod source set`)
      }
      const path = `${dir}/${file}`
      const text = readTextFile(path)
      if (forbiddenToken.test(text)) {
        violations.push(`${path}: contains retired production runtime token`)
      }
    }
  }

  if (violations.length > 0) {
    fail(name, violations.join('\n'))
  }
}

function expectPackageScriptsNoLegacyRuntimeMarkers(name) {
  const packagePaths = ['package.json', 'packages/runtime/package.json']
  const publicRuntimeScript = /^(build:runtime|qa:runtime:packaged|runtime:go:)/
  const legacyPublicRuntimeMarker = /\b(?:d0(?:24|25)[a-z0-9_-]*|proof|conformance|fixture|fake|mcpLocalProof)\b/i
  const legacyGlobalMarker = /\b(?:d0(?:24|25)[a-z0-9_-]*|proof|oracle|fake|mcpLocalProof)\b/i
  const violations = new Set()
  for (const packagePath of packagePaths) {
    if (!existsSync(packagePath)) continue
    const parsed = JSON.parse(readTextFile(packagePath))
    const scripts = parsed && typeof parsed === 'object' && parsed.scripts && typeof parsed.scripts === 'object'
      ? parsed.scripts
      : {}
    for (const [scriptName, command] of Object.entries(scripts)) {
      const commandText = String(command)
      const scriptSurface = `${scriptName}: ${commandText}`
      if (legacyGlobalMarker.test(scriptSurface)) {
        violations.add(`${packagePath}: ${scriptSurface}`)
      }
      if (publicRuntimeScript.test(scriptName) && legacyPublicRuntimeMarker.test(commandText)) {
        violations.add(`${packagePath}: ${scriptSurface}`)
      }
    }
  }
  if (violations.size > 0) {
    fail(name, [...violations].join('\n'))
  }
}

function expectRuntimeValidationRegistryNoLegacyMarkers(name, path) {
  const text = readTextFile(path)
  const commandEntryPattern = /^\s*['"]?([a-z0-9:-]+)['"]?\s*:\s*\{[^}]*script:\s*['"]([^'"]+)['"]/gim
  const legacyRegistryMarker = /\b(?:d0(?:24|25)[a-z0-9_-]*|proof|oracle|conformance|fixture|fake|mcpLocalProof)\b/i
  const violations = []
  let match
  while ((match = commandEntryPattern.exec(text)) !== null) {
    const [, commandName, scriptName] = match
    const registrySurface = `${commandName}: ${scriptName}`
    if (legacyRegistryMarker.test(registrySurface)) {
      violations.push(`${path}: ${registrySurface}`)
    }
  }
  if (violations.length > 0) {
    fail(name, violations.join('\n'))
  }
}

function expectGoRuntimeCurrentnessBoundaries(name, entries) {
  const missing = []
  for (const entry of entries) {
    const blocks = readTextFile(entry.path).split(/(?=^#{1,6}\s)/m)
    const hasCurrentnessBlock = blocks.some((block) => {
      const hasBoundary = /current[- ]state note|current[- ]state addendum|current status|current release status|current principle|status:|conclusion|historical|superseded|post-cutover/i.test(block)
      const hasCurrentDefault = /go-runtime-default|Go runtime default|TypeScript[^.\n]*retired-backend|`ANALYTIX_RUNTIME_BACKEND=typescript`[^.\n]*retired-backend|post-cutover|superseded/i.test(block)
      return hasBoundary && hasCurrentDefault
    })
    if (!hasCurrentnessBlock) {
      missing.push(`${entry.path}: missing a bounded current/superseded Go-default section`)
    }
  }
  if (missing.length > 0) {
    fail(name, missing.join('\n'))
  }
}

function expectNoUnboundedGoRuntimeCurrentnessRegressions(name, entries) {
  const regressionPattern = /\b(TypeScript remains (?:the )?default(?: runtime| backend)?|TypeScript is (?:the |current )default(?: runtime| backend)?|Go default (?:is )?blocked|Go remains shadow-only|Go is shadow-only|no default Go backend|default Go backend still not ready|Default Go backend remains rejected|Go default cutover is therefore not authorized|Go runtime may be treated as an internal candidate only|Go runtime is not a default backend|Go remains shadow-only and is not a default backend)\b/i
  const localBoundaryPattern = /historical|superseded|current[- ]state|current status|post-cutover|stage record|stage text|Rows that say|older .*wording|no longer evidence/i
  const violations = []
  for (const entry of entries) {
    const lines = readTextFile(entry.path).split(/\r?\n/)
    for (let index = 0; index < lines.length; index += 1) {
      if (!regressionPattern.test(lines[index])) continue
      if (entry.historicalRowsAllowed) continue
      const context = lines.slice(Math.max(0, index - 4), Math.min(lines.length, index + 2)).join('\n')
      if (!localBoundaryPattern.test(context)) {
        violations.push(`${entry.path}:${index + 1}:${lines[index]}`)
      }
    }
  }
  if (violations.length > 0) {
    fail(name, violations.join('\n'))
  }
}

function expectIntegrationTopologyPostCutoverCurrentness(name, path) {
  const text = readTextFile(path)
  const violations = []
  if (!/Current Go runtime default delivery is active through `go-runtime-default`/.test(text)) {
    violations.push(`${path}: missing current Go runtime default status`)
  }
  if (!/Reasonix integration topology is runtime-info evidence/.test(text)) {
    violations.push(`${path}: missing Reasonix integration topology evidence boundary`)
  }
  if (!/Live provider\/MCP\/packaged\/operator validation remains post-cutover evidence/.test(text)) {
    violations.push(`${path}: missing post-cutover live validation boundary`)
  }
  for (const forbidden of [
    'TypeScript remains the default',
    'Go default cutover is therefore not authorized',
    'Go runtime may be treated as an internal candidate only'
  ]) {
    if (text.includes(forbidden)) {
      violations.push(`${path}: forbidden stale currentness phrase: ${forbidden}`)
    }
  }
  if (violations.length > 0) {
    fail(name, violations.join('\n'))
  }
}

function expectCurrentGoEvidenceBlocksNoLegacyMarkers(name) {
  const runtimeIndexPath = 'docs/analytix/runtime/go-runtime-conformance.md'
  const runtimeIndexText = readTextFile(runtimeIndexPath)
  const evidenceMatch = runtimeIndexText.match(/Evidence:\n\n```text\n([\s\S]*?)\n```/)
  const upstreamPath = 'docs/analytix/upstreams/go-runtime-conformance.md'
  const upstreamText = readTextFile(upstreamPath)
  const upstreamCurrentIntro = upstreamText.split('## 2026-06-23 - D-0250A')[0] || ''
  const g6Row = upstreamText.split(/\r?\n/).find((line) => line.startsWith('| G6 |')) || ''
  const checks = [
    {
      path: runtimeIndexPath,
      label: 'current evidence block',
      text: evidenceMatch?.[1] || ''
    },
    {
      path: upstreamPath,
      label: 'G6 evidence row',
      text: g6Row
    },
    {
      path: upstreamPath,
      label: 'current principle and stage summary',
      text: upstreamCurrentIntro
    }
  ]
  const legacyMarker = /scripts\/d0(?:24|25)|docs\/analytix\/upstreams\/d0(?:24|25)|contract-sidecar|conformance-sidecar|conformance\/fixtures\/|live_local|(?:^|\b)(?:proof|oracle|fake|mcpLocalProof)(?:\b|$)/i
  const violations = []
  for (const check of checks) {
    if (!check.text.trim()) {
      violations.push(`${check.path}: missing ${check.label}`)
      continue
    }
    if (legacyMarker.test(check.text)) {
      violations.push(`${check.path}: ${check.label} contains archived/proof marker`)
    }
  }
  if (violations.length > 0) {
    fail(name, violations.join('\n'))
  }
}

const browserPreviewBridgePath = 'src/renderer/src/lib/browser-analytix-bridge.ts'
const directRuntimeRequestBridgePattern =
  'window\\.analytix(?:\\?\\.|\\.)runtime(?:\\?\\.|\\.)runtimeRequest'
const directSettingsGetBridgePattern =
  'window\\.analytix(?:\\?\\.|\\.)settings(?:\\?\\.|\\.)getSettings'
const windowAnalytixRuntimeApiPattern =
  'window\\.analytix(?:\\?\\.|\\.)runtime(?:\\?\\.|\\.)[A-Za-z0-9_]+'
const windowAnalytixSettingsApiPattern =
  'window\\.analytix(?:\\?\\.|\\.)settings(?:\\?\\.|\\.)[A-Za-z0-9_]+'
const legacyKunSettingsMigrationAllowedLinePatterns = [
  '^src/main/settings-store\\.ts:\\d+:const LEGACY_KUN_SETTINGS_FILE_NAME = [\'"]kun-settings\\.json[\'"]',
  '^src/main/settings-store\\.ts:\\d+:\\s*join\\(currentUserDataDir, LEGACY_KUN_SETTINGS_FILE_NAME\\)',
  '^src/main/settings-store\\.ts:\\d+:\\s*if \\(dirName === [\'"]Kun[\'"]\\) candidates\\.push\\(join\\(parentDir, dirName, LEGACY_KUN_SETTINGS_FILE_NAME\\)\\)',
  '^src/main/settings-store\\.ts:\\d+:\\s*if \\(basename\\(sourcePath\\) === LEGACY_KUN_SETTINGS_FILE_NAME\\) \\{',
  '^src/main/settings-store\\.ts:\\d+:  const kun = isRecord\\(agents\\) \\? agents\\.kun : undefined$',
  '^src/main/settings-store\\.ts:\\d+:      `\\$\\{sourceLocator\\}:agents\\.kun\\.apiKey`,$',
  '^src/main/settings-store\\.ts:\\d+:      const isKunSource = basename\\(loaded\\.sourcePath\\) === LEGACY_KUN_SETTINGS_FILE_NAME$',
  '^src/main/settings-store\\.ts:\\d+:        basename\\(loaded\\.sourcePath\\) === LEGACY_KUN_SETTINGS_FILE_NAME,$',
  "^src/main/provider-registry-legacy-migration\\.ts:\\d+:  'agents\\.kun\\.apiKey',$"
]

if (process.argv.includes('--self-test-source-classification')) {
  console.log(JSON.stringify(sourceClassificationSelfTest()))
  process.exit(0)
}

const rendererEntryPaths = [
  'src/renderer/src/App.tsx',
  'src/renderer/src/main.tsx',
  'src/renderer/src/components/Workbench.tsx',
  'src/renderer/src/components/SessionHeader.tsx',
  'src/renderer/src/components/PluginMarketplaceView.tsx',
  'src/renderer/src/components/write/WriteSidebar.tsx',
  'src/renderer/src/components/chat/WorkspaceModeTabs.tsx',
  'src/renderer/src/components/chat/SidebarProjectsSection.tsx',
  'src/renderer/src/components/chat',
  'src/renderer/src/store'
]

const workflowQuarantineEntryPaths = [
  ...rendererEntryPaths,
  'src/preload',
  browserPreviewBridgePath,
  'src/main/tray-session-menu.ts',
  'src/renderer/src/components/settings-section-shortcuts.tsx'
]

const productRuntimeSurfacePaths = [
  'src/main',
  'src/preload',
  'src/renderer/src',
  'src/shared',
  'packages/runtime/src/contracts',
  'packages/runtime/src/server-test-support/routes'
]

const formalRuntimeValidationPaths = [
  'scripts/runtime-go-validation-command.mjs',
  'scripts/runtime-go-packaged-qa.mjs',
  'scripts/runtime-go-preflight.mjs',
  'scripts/runtime-go-live-validation.mjs',
  'scripts/runtime-go-local-validation.mjs',
  'scripts/runtime-go-runtime-health-smoke.mjs',
  'scripts/runtime-go-performance-check.mjs',
  'scripts/runtime-go-speed-cache-gate.mjs',
  'scripts/runtime-go-product-regression.mjs',
  'scripts/runtime-go-release-gate.mjs',
  'scripts/runtime-go-packaged-session-soak.mjs',
  'scripts/runtime-go-packaged-gui-smoke.mjs',
  'scripts/runtime-go-default-readiness-report.mjs',
  'scripts/runtime-go-rollback-retirement-evidence.mjs',
  'scripts/runtime-go-rollback-retirement-report.mjs',
  'scripts/runtime-go-cutover-report.mjs',
  'scripts/runtime-go-engine-absorption-report.mjs'
]

const formalRuntimeCurrentEvidencePaths = [
  'docs/analytix/upstreams/go-runtime-retirement-checklist.md',
  'docs/analytix/upstreams/reasonix-integration-topology.md',
  'docs/analytix/upstreams/runtime-go-live-evidence',
  'docs/analytix/upstreams/runtime-go-default-readiness',
  'docs/analytix/upstreams/runtime-go-retirement'
]

const currentProductSurfaceFileNamePaths = [
  'packages/runtime/dist',
  'packages/runtime-go',
  'src/main',
  'src/preload',
  'src/renderer/src',
  'src/shared',
  'scripts',
  ...formalRuntimeCurrentEvidencePaths
]

const archivedLegacyRuntimeEvidencePaths = []

const archivedExistingRuntimeEvidencePaths = existingPaths(archivedLegacyRuntimeEvidencePaths)

const legacyStageEvidenceArchiveOnlyPaths = [
  'docs/analytix/upstreams/d0248-go-runtime-g6-preflight-report.json',
  'docs/analytix/upstreams/d0248-go-runtime-g6-preflight-summary.md',
  'docs/analytix/upstreams/d0248-go-runtime-readiness-gate-report.json',
  'docs/analytix/upstreams/d0248-go-runtime-readiness-gate-summary.md',
  'docs/analytix/upstreams/d0248-packaged-go-runtime-qa-report.json',
  'docs/analytix/upstreams/d0249-d0250-go-runtime-retirement-checklist.md',
  'docs/analytix/upstreams/d0249-reasonix-integration-topology.md',
  'docs/analytix/upstreams/d0250b-go-runtime-cutover-report.json',
  'docs/analytix/upstreams/d0250b-go-runtime-cutover-summary.md',
  'docs/analytix/upstreams/d0250c-go-runtime-code-stage-report.json',
  'docs/analytix/upstreams/d0250c-go-runtime-code-stage-summary.md',
  'docs/analytix/upstreams/d0250c-reasonix-superiority-matrix.json',
  'docs/analytix/upstreams/d0251-live',
  'docs/analytix/upstreams/d0251-live/d0251-live-evidence-report.json',
  'docs/analytix/upstreams/d0251-live/d0251-live-evidence-summary.md',
  'docs/analytix/upstreams/d0252-candidate',
  'docs/analytix/upstreams/d0252-candidate/d0252-go-default-candidate-report.json',
  'docs/analytix/upstreams/d0252-candidate/d0252-go-default-candidate-summary.md',
  'docs/analytix/upstreams/d0253-retirement',
  'docs/analytix/upstreams/d0253-retirement/d0253-fallback-retirement-report.json',
  'docs/analytix/upstreams/d0253-retirement/d0253-fallback-retirement-summary.md',
  'docs/analytix/upstreams/d0253-retirement/d0253-retirement-evidence-summary.md'
]

const legacyStageRedirectMarkdownPaths = [
  'docs/analytix/upstreams/d0249-d0250-go-runtime-retirement-checklist.md',
  'docs/analytix/upstreams/d0249-reasonix-integration-topology.md'
]

const discoveredLegacyStageEvidenceArchiveOnlyPaths = discoverLegacyStageEvidencePaths(
  'legacy stage evidence dynamic path discovery'
)

const staleRuntimeDistLegacyArtifactPaths = [
  'packages/runtime/dist/domain/checkpoint-oracle.js',
  'packages/runtime/dist/domain/checkpoint-oracle.js.map',
  'packages/runtime/dist/domain/checkpoint-oracle.d.ts',
  'packages/runtime/dist/domain/checkpoint-oracle.d.ts.map'
]

const settingsSovereigntyPaths = [
  'src/shared/app-settings.ts',
  'src/shared/app-settings-normalize.ts',
  'src/shared/app-settings-runtime.ts',
  'src/shared/app-settings-types.ts',
  'src/shared/app-settings.test.ts',
  'src/main/settings-store.ts',
  'src/main/settings-store.test.ts',
  'src/main/ipc/app-ipc-schemas.ts',
  'src/main/ipc/app-ipc-schemas.test.ts',
  'src/preload/index.ts',
  'src/preload/preload-sandbox.test.ts',
  browserPreviewBridgePath,
  'src/renderer/src/lib/browser-analytix-bridge.test.ts'
]

const runtimeContractFreshnessPaths = [
  'packages/runtime/src/conformance/runtime-parity-fixtures.ts',
  'packages/runtime/src/conformance/go-runtime-kernel-conformance.ts',
  'packages/runtime/src/conformance/fixtures/provider-cache-contract.json',
  'packages/runtime/src/conformance/fixtures/approval-user-input-route-contract.json',
  'packages/runtime/src/conformance/fixtures/task-job-orchestration-contract.json',
  'packages/runtime/src/conformance/fixtures/mcp-tool-lifecycle-contract.json',
  'packages/runtime/src/conformance/fixtures/go-g2-route-replay-contract.json',
  'packages/runtime/src/conformance/fixtures/go-g3-provider-streaming-usage-cache-contract.json',
  'packages/runtime/src/conformance/fixtures/go-g4-tools-approval-user-input-mcp-contract.json',
  'packages/runtime/src/conformance/fixtures/go-g5-full-loop-contract.json',
  'packages/runtime/src/conformance/fixtures/go-durable-sidecar-contract.json',
  'packages/runtime/src/conformance/fixtures/go-kernel-live-scaffold-contract.json',
  'packages/runtime/tests/provider-cache-contract.test.ts',
  'packages/runtime/tests/approval-user-input-route-contract.test.ts',
  'packages/runtime/tests/task-job-orchestration-contract.test.ts',
  'packages/runtime/tests/mcp-tool-lifecycle-contract.test.ts',
  'packages/runtime/tests/create-plan-tool.test.ts',
  'packages/runtime/src/contracts/capabilities.ts',
  'packages/runtime/tests/go-runtime-g3-g4-conformance.test.ts',
  'packages/runtime/tests/go-runtime-conformance.test.ts',
  'packages/runtime/tests/go-durable-sidecar-conformance.test.ts',
  'packages/runtime/tests/go-kernel-live-scaffold-conformance.test.ts',
  'packages/runtime/tests/go-production-candidate-conformance.test.ts',
  'packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix.go',
  'packages/runtime-go/internal/upstreamaudit/reasonix_integration_topology.go',
  'packages/runtime-go/reasonix_engine_matrix_test.go',
  'packages/runtime-go/internal/server/capabilities.go',
  'packages/runtime-go/internal/adapters/inbound/httpapi/response.go',
  'packages/runtime-go/internal/protocol/route_replay.go',
  'packages/runtime-go/internal/server/durable_store.go',
  'packages/runtime-go/internal/conformance/livelocal/production_candidate.go',
  'packages/runtime-go/internal/conformance/livelocal/g1_handler.go',
  'packages/runtime-go/internal/contracts/boundary.go',
  'packages/runtime-go/internal/contracts/product_boundary.go',
  'packages/runtime-go/internal/contracts/hash.go',
  'packages/runtime-go/internal/contracts/records.go',
  'packages/runtime-go/contracts_shim.go',
  'packages/runtime-go/runtime_domain_shims.go',
  'packages/runtime-go/runtime_durable_store_shim.go',
  'packages/runtime-go/internal/provider/provider.go',
  'packages/runtime-go/internal/provider/provider_contract_matrix.go',
  'packages/runtime-go/internal/provider/provider_contract_replay_handler.go',
  'packages/runtime-go/internal/goal/goal_evidence.go',
  'packages/runtime-go/goal_evidence_contract_test.go',
  'packages/runtime-go/internal/mcp/lifecycle.go',
  'packages/runtime-go/internal/mcp/manager.go',
  'packages/runtime-go/internal/mcp/g4_tools_contract.go',
  'packages/runtime-go/internal/mcp/lifecycle_contract.go',
  'packages/runtime-go/mcp_lifecycle_contract_test.go',
  'packages/runtime-go/internal/agent/gates.go',
  'packages/runtime-go/internal/agent/approval_user_input_contract.go',
  'packages/runtime-go/internal/jobs/lineage.go',
  'packages/runtime-go/internal/upstreamaudit/baseline_absorption.go',
  'packages/runtime-go/kun_analytix_baseline_absorption_test.go',
  'packages/runtime-go/internal/research/autoresearch_state.go',
  'packages/runtime-go/autoresearch_state_contract_test.go',
  'packages/runtime-go/reasonix_integration_topology_test.go',
  'packages/runtime-go/reasonix_superiority_matrix_test.go',
  'packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix.go',
  'packages/runtime-go/internal/readiness/readiness.go',
  'packages/runtime-go/g6_readiness.go',
  'packages/runtime-go/g6_readiness_test.go',
  'packages/runtime-go/runtime_server.go',
  'packages/runtime-go/internal/server/runtime_components.go',
  'packages/runtime-go/runtime_server_test.go',
  'packages/runtime-go/cmd/runtime-server/main.go',
  'packages/runtime/tests/http-server.test.ts',
 'packages/runtime-go/root_shims_test.go',
  'packages/runtime-go/contract_shims_test.go',
 'packages/runtime-go/internal/conformance/livelocal/harness.go',
 'packages/runtime-go/internal/conformance/livelocal/store.go',
 'packages/runtime-go/internal/conformance/livelocal/durable_handler.go',
  'packages/runtime-go/internal/server/durable_store.go',
  'packages/runtime-go/durable_replay_contract_test.go',
  'packages/runtime-go/live_production_candidate.go',
  'packages/runtime-go/internal/conformance/livelocal/production_candidate_handler.go',
  'packages/runtime-go/live_production_candidate_test.go',
  'packages/runtime-go/runtime_server.go',
  'packages/runtime-go/internal/server/runtime_components.go',
  'packages/runtime-go/runtime_server_test.go',
 'packages/runtime-go/internal/conformance/livelocal/loop_handler.go',
 'packages/runtime-go/internal/agent/minimal_loop_contract.go',
 'packages/runtime-go/internal/conformance/livelocal/kernel_handler.go',
 'packages/runtime-go/kernel_replay_contract_test.go',
 'packages/runtime-go/cmd/contract-sidecar/main.go',
 'packages/runtime-go/cmd/runtime-server/main.go',
 'packages/runtime-go/internal/mcp/mcp_contract_replay_handler.go',
 'packages/runtime-go/internal/conformance/g5_shadow.go',
 'packages/runtime-go/shadow_test.go',
  'src/main/runtime/analytix-adapter.ts'
]

const runtimeRouteClientSourcePaths = [
  'packages/runtime/src/server-test-support/routes',
  'src/shared/analytix-endpoints.ts',
  'src/main/ipc/app-ipc-schemas.ts',
  'src/renderer/src/agent/analytix-runtime.ts'
]

const post881RuntimeContractTokenPaths = [
  'packages/runtime/src/conformance/runtime-parity-fixtures.ts',
  'packages/runtime/src/conformance/fixtures/provider-cache-contract.json',
  'packages/runtime/src/conformance/fixtures/task-job-orchestration-contract.json',
  'packages/runtime/src/conformance/fixtures/mcp-tool-lifecycle-contract.json',
  'packages/runtime/src/conformance/fixtures/go-g2-route-replay-contract.json',
  'packages/runtime/src/conformance/fixtures/go-g5-full-loop-contract.json',
  'packages/runtime/src/conformance/fixtures/go-kernel-live-scaffold-contract.json',
  'packages/runtime/src/server-test-support/routes/index.ts',
  'packages/runtime/src/server-test-support/router.ts',
  'packages/runtime/src/server-test-support/http-server.ts',
  'packages/runtime/tests/http-server.test.ts',
  'src/shared/analytix-endpoints.ts',
  'src/shared/analytix-endpoints.test.ts',
  'packages/runtime/src/tool-test-support/tool/mcp-tool-provider.ts',
  'package.json',
  'packages/runtime/package.json',
  'electron-builder.config.cjs',
  'src/main/app-identity.ts',
  'src/main/index.ts',
  'src/main/resolve-analytix-binary.ts',
  'packages/runtime/src/cli/serve-entry.ts',
  'packages/runtime/src/cli/serve.ts',
  'scripts/after-pack.cjs',
  'src/main/packaging-config.test.ts',
  'src/preload/index.ts',
  'src/preload/preload-sandbox.test.ts',
  'src/preload/preload-runtime-request.test.ts',
  'src/preload/preload-sse-bridge.test.ts',
  'src/shared/analytix-api.ts',
  'src/renderer/src/agent/runtime-client.test.ts',
  'src/renderer/src/agent/analytix-runtime.ts',
  'src/renderer/src/agent/analytix-runtime.test.ts',
  'src/renderer/src/store/chat-store-side-actions.ts',
  'src/renderer/src/store/chat-store-side-actions.test.ts',
  'src/renderer/src/hooks/use-thread-usage.ts',
  'src/renderer/src/hooks/use-thread-usage.test.ts',
  'src/renderer/src/hooks/use-daily-usage.ts',
  'src/renderer/src/hooks/use-daily-usage.test.ts',
  'src/renderer/src/hooks/use-model-usage.ts',
  'src/renderer/src/hooks/use-model-usage.test.ts',
  'src/renderer/src/components/settings-section-agents.tsx',
  'src/renderer/src/components/settings-section-agents.test.ts',
  'src/renderer/src/components/settings-section-llm-debug.tsx',
  'src/renderer/src/lib/keyboard-shortcut-settings.ts',
  'src/renderer/src/components/chat/use-voice-dictation.ts',
  'src/renderer/src/components/chat/InitialSessionUsageHeatmap.tsx',
  'src/renderer/src/components/chat/InitialSessionUsageHeatmap.test.ts',
  'src/main/ipc/app-ipc-schemas.ts',
  'src/main/ipc/app-ipc-schemas.test.ts',
  'src/main/ipc/register-app-ipc-handlers.ts',
  'src/main/ipc/register-app-ipc-handlers.test.ts',
  'src/main/runtime/analytix-adapter.test.ts',
  'src/main/runtime-sse-ipc.ts',
  'src/main/runtime-sse-ipc.test.ts',
  'src/renderer/src/components/Workbench.tsx',
  'src/renderer/src/components/Workbench.route-surface.test.ts',
  'src/renderer/src/components/PluginMarketplaceView.tsx',
  'src/renderer/src/lib/browser-analytix-bridge.ts',
  'src/renderer/src/styles/base-shell.css',
  'packages/runtime/tests/provider-cache-contract.test.ts',
  'packages/runtime/tests/task-job-orchestration-contract.test.ts',
  'packages/runtime/tests/mcp-tool-lifecycle-contract.test.ts',
  'packages/runtime/tests/mcp-tool-provider.test.ts',
  'packages/runtime/src/contracts/capabilities.ts',
  'packages/runtime/src/shared/tool-result-image.ts',
  'packages/runtime/src/loop/tool-result-image.test.ts',
  'packages/runtime/tests/attachment-store.test.ts',
  'packages/runtime/src/adapters/file/file-session-store.ts',
  'packages/runtime/tests/loop.test.ts',
  'packages/runtime/tests/runtime-event-recorder.test.ts',
  'packages/runtime/tests/file-session-store.test.ts',
  'packages/runtime/tests/create-plan-tool.test.ts',
  'packages/runtime/tests/go-runtime-conformance.test.ts',
  'packages/runtime/tests/go-runtime-g3-g4-conformance.test.ts',
  'packages/runtime/tests/go-durable-sidecar-conformance.test.ts',
  'packages/runtime/tests/go-kernel-live-scaffold-conformance.test.ts',
  'packages/runtime/tests/go-production-candidate-conformance.test.ts',
 'packages/runtime-go/root_shims_test.go',
  'packages/runtime-go/contract_shims_test.go',
 'packages/runtime-go/internal/conformance/livelocal/harness.go',
 'packages/runtime-go/internal/conformance/livelocal/store.go',
 'packages/runtime-go/internal/provider/provider_contract_replay_handler.go',
 'packages/runtime-go/internal/mcp/mcp_contract_replay_handler.go',
 'packages/runtime-go/internal/conformance/livelocal/durable_handler.go',
 'packages/runtime-go/internal/conformance/livelocal/loop_handler.go',
  'packages/runtime-go/internal/agent/minimal_loop_contract.go',
  'packages/runtime-go/internal/server/durable_store.go',
  'packages/runtime-go/internal/conformance/livelocal/production_candidate.go',
  'packages/runtime-go/durable_replay_contract_test.go',
  'packages/runtime-go/live_production_candidate.go',
  'packages/runtime-go/internal/conformance/livelocal/production_candidate_handler.go',
  'packages/runtime-go/live_production_candidate_test.go',
  'packages/runtime-go/internal/readiness/readiness.go',
  'packages/runtime-go/internal/readiness/readiness_semantics.go',
  'packages/runtime-go/g6_readiness.go',
  'packages/runtime-go/g6_readiness_test.go',
  'packages/runtime-go/internal/upstreamaudit/single_baseline_checklist.go',
  'packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix.go',
  'packages/runtime-go/internal/upstreamaudit/reasonix_integration_topology.go',
  'packages/runtime-go/reasonix_engine_matrix_test.go',
  'packages/runtime-go/internal/adapters/inbound/httpapi/response.go',
  'packages/runtime-go/internal/contracts/boundary.go',
  'packages/runtime-go/internal/contracts/product_boundary.go',
  'packages/runtime-go/internal/contracts/hash.go',
  'packages/runtime-go/contracts_shim.go',
  'packages/runtime-go/runtime_domain_shims.go',
  'packages/runtime-go/runtime_durable_store_shim.go',
  'packages/runtime-go/internal/provider/provider.go',
  'packages/runtime-go/internal/provider/provider_contract_matrix.go',
  'packages/runtime-go/internal/goal/goal_evidence.go',
  'packages/runtime-go/goal_evidence_contract_test.go',
  'packages/runtime-go/internal/mcp/lifecycle.go',
  'packages/runtime-go/internal/mcp/manager.go',
  'packages/runtime-go/internal/mcp/g4_tools_contract.go',
  'packages/runtime-go/internal/mcp/lifecycle_contract.go',
  'packages/runtime-go/mcp_lifecycle_contract_test.go',
  'packages/runtime-go/internal/agent/gates.go',
  'packages/runtime-go/internal/agent/approval_user_input_contract.go',
  'packages/runtime-go/internal/jobs/lineage.go',
  'packages/runtime-go/internal/upstreamaudit/baseline_absorption.go',
  'packages/runtime-go/kun_analytix_baseline_absorption_test.go',
  'packages/runtime-go/internal/research/autoresearch_state.go',
  'packages/runtime-go/autoresearch_state_contract_test.go',
  'packages/runtime-go/reasonix_integration_topology_test.go',
  'packages/runtime/src/contracts/events.ts',
  'packages/runtime-go/live_production_candidate.go',
  'packages/runtime-go/internal/conformance/livelocal/production_candidate_handler.go',
 'packages/runtime-go/internal/conformance/livelocal/kernel_handler.go',
  'packages/runtime-go/kernel_replay_contract_test.go',
  'packages/runtime-go/cmd/contract-sidecar/main.go',
  'packages/runtime-go/runtime_server.go',
  'packages/runtime-go/internal/server/runtime_components.go',
  'packages/runtime-go/runtime_server_test.go',
  'packages/runtime-go/cmd/runtime-server/main.go',
  'packages/runtime-go/cmd/runtime-engine-absorption-report/main.go',
  'packages/runtime-go/shadow_test.go',
  'packages/runtime-go/README.md',
  'src/renderer/src/agent/analytix-mapper.test.ts',
 'packages/runtime-go/internal/conformance/g5_shadow.go',
  'src/main/runtime/analytix-adapter.ts'
]

const runtimeDesktopBridgeContractPaths = [
  'src/shared/analytix-api.ts',
  'src/shared/analytix-endpoints.ts',
  'src/renderer/src/agent/runtime-client.ts',
  'src/renderer/src/agent/runtime-client.test.ts',
  'src/renderer/src/agent/analytix-runtime.ts',
  'src/renderer/src/agent/analytix-runtime.test.ts',
  'src/renderer/src/lib/browser-analytix-bridge.ts',
  'src/renderer/src/lib/browser-analytix-bridge.test.ts',
  'src/preload/index.ts',
  'src/preload/index.d.ts',
  'src/preload/preload-sandbox.test.ts',
  'src/preload/preload-runtime-request.test.ts',
  'src/preload/preload-sse-bridge.test.ts',
  'src/main/ipc/app-ipc-schemas.ts',
  'src/main/ipc/app-ipc-schemas.test.ts',
  'src/main/ipc/register-app-ipc-handlers.ts',
  'src/main/ipc/register-app-ipc-handlers.test.ts',
  'src/main/runtime/analytix-adapter.ts',
  'src/main/runtime/analytix-adapter.test.ts',
  'src/main/runtime-sse-ipc.ts',
  'src/main/runtime-sse-ipc.test.ts'
]

const releaseEvidenceContractPaths = [
  'docs/analytix/qa/release-evidence-gate-2026-06-21.md'
]

const post881StageClosurePaths = [
  'docs/analytix/qa/post-881-stage-closure-2026-06-22.md'
]

const post881SubagentReviewPaths = [
  'docs/analytix/qa/post-881-subagent-review-2026-06-22.md'
]

const legacyStageEvidenceArchivePaths = [
  'docs/analytix/upstreams/legacy-stage-evidence-archive.md'
]

const goRuntimeCurrentnessDocEntries = [
  { path: 'docs/analytix/runtime/go-runtime-conformance.md', historicalRowsAllowed: false },
  { path: 'docs/analytix/upstreams/README.md', historicalRowsAllowed: false },
  { path: 'docs/analytix/upstreams/final-go-runtime-delivery-report.md', historicalRowsAllowed: false },
  { path: 'docs/analytix/upstreams/absorption-ledger.md', historicalRowsAllowed: true },
  { path: 'docs/analytix/upstreams/go-runtime-conformance.md', historicalRowsAllowed: true },
  { path: 'docs/analytix/upstreams/go-runtime-retirement-checklist.md', historicalRowsAllowed: false },
  { path: 'docs/analytix/upstreams/reasonix-integration-topology.md', historicalRowsAllowed: false },
  { path: 'docs/analytix/specs/08-upstream-absorption-and-go-runtime.md', historicalRowsAllowed: true },
  { path: 'docs/analytix/specs/09-agent-quality-product-benchmark.md', historicalRowsAllowed: true },
  { path: 'docs/analytix/benchmarks/upstream-scorecard.md', historicalRowsAllowed: true },
  { path: 'docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-summary.md', historicalRowsAllowed: false },
  { path: 'docs/analytix/upstreams/runtime-go-default-readiness/default-readiness-summary.md', historicalRowsAllowed: false },
  { path: 'docs/analytix/upstreams/runtime-go-retirement/fallback-retirement-summary.md', historicalRowsAllowed: false },
  { path: 'docs/analytix/upstreams/runtime-go-retirement/retirement-evidence-summary.md', historicalRowsAllowed: false }
]

expectFilesPresent('scan path freshness', [
  ...workflowQuarantineEntryPaths,
  ...settingsSovereigntyPaths,
  ...formalRuntimeValidationPaths,
  ...runtimeContractFreshnessPaths,
  ...runtimeRouteClientSourcePaths,
  ...runtimeDesktopBridgeContractPaths,
  ...releaseEvidenceContractPaths,
  ...post881StageClosurePaths,
  ...post881SubagentReviewPaths,
  ...legacyStageEvidenceArchivePaths,
  ...legacyStageEvidenceArchiveOnlyPaths,
  ...discoveredLegacyStageEvidenceArchiveOnlyPaths,
  'src/shared/analytix-api.ts',
  'src/preload/index.d.ts',
  'src/renderer/src/components/chat/Sidebar.tsx',
  'src/renderer/src/components/Workbench.route-surface.test.ts'
])

expectFilesPresent('settings sovereignty path freshness', settingsSovereigntyPaths)
expectFilesPresent('formal runtime validation path freshness', formalRuntimeValidationPaths)
expectFilesPresent('formal runtime current evidence path freshness', formalRuntimeCurrentEvidencePaths)
expectFilesPresent('runtime contract path freshness', runtimeContractFreshnessPaths)
expectFilesPresent('runtime route/client path freshness', runtimeRouteClientSourcePaths)
expectFilesPresent('runtime desktop bridge contract path freshness', runtimeDesktopBridgeContractPaths)
expectFilesPresent('post-881 runtime contract token path freshness', post881RuntimeContractTokenPaths)
expectFilesPresent('release evidence contract path freshness', releaseEvidenceContractPaths)
expectFilesPresent('post-881 stage closure path freshness', post881StageClosurePaths)
expectFilesPresent('post-881 sub-agent review path freshness', post881SubagentReviewPaths)
expectFilesPresent('legacy stage evidence archive path freshness', legacyStageEvidenceArchivePaths)
expectFilesPresent('legacy stage evidence archive-only path freshness', legacyStageEvidenceArchiveOnlyPaths)
expectFilesPresent('legacy stage evidence dynamic archive path freshness', discoveredLegacyStageEvidenceArchiveOnlyPaths)
expectFilesPresent(
  'Go runtime currentness guard path freshness',
  goRuntimeCurrentnessDocEntries.map((entry) => entry.path)
)
expectFilesAbsent(
  'runtime dist stale legacy artifact scan',
  staleRuntimeDistLegacyArtifactPaths
)

expectGoRuntimeCurrentnessBoundaries('Go runtime currentness boundary scan', goRuntimeCurrentnessDocEntries)
expectNoUnboundedGoRuntimeCurrentnessRegressions(
  'Go runtime currentness regression scan',
  goRuntimeCurrentnessDocEntries
)
expectIntegrationTopologyPostCutoverCurrentness(
  'Reasonix integration topology post-cutover currentness scan',
  'docs/analytix/upstreams/reasonix-integration-topology.md'
)

expectCurrentGoEvidenceBlocksNoLegacyMarkers(
  'current Go evidence block legacy marker scan'
)

expectAllRgMatches(
  'legacy stage evidence archive boundary scan',
  [
    'Status: archive-only',
    'Current formal replacements',
    'not production runtime entrypoints',
    'not product capabilities',
    'not current Go-default authorization gates',
    'runtime-go-preflight',
    'runtime-go-live-evidence',
    'Historical paths must not be used as current production gates'
  ],
  legacyStageEvidenceArchivePaths
)

expectArchiveManifestCoversPaths(
  'legacy stage evidence archive coverage scan',
  'docs/analytix/upstreams/legacy-stage-evidence-archive.md',
  legacyStageEvidenceArchiveOnlyPaths
)

expectArchiveManifestCoversPaths(
  'legacy stage evidence dynamic archive coverage scan',
  'docs/analytix/upstreams/legacy-stage-evidence-archive.md',
  discoveredLegacyStageEvidenceArchiveOnlyPaths
)

expectAllRgMatches(
  'legacy stage redirect markdown archive-only scan',
  [
    'Status: archive-only',
    'historical stage-numbered path',
    'not a current production gate',
    'Use the current formal'
  ],
  legacyStageRedirectMarkdownPaths
)

expectNoRgMatches(
  'formal runtime current evidence legacy naming scan',
  '\\b(?:proof|oracle|fake|mcpLocalProof)\\b',
  formalRuntimeCurrentEvidencePaths
)

expectNoLegacyProductSurfaceFileNames(
  'current product surface legacy filename scan',
  currentProductSurfaceFileNamePaths
)

expectNoRgMatches(
  'Go runtime public capability upstream-audit metadata scan',
  'upstreamAbsorption|upstreamaudit|Reasonix|reasonix',
  [
    'packages/runtime-go/internal/server/capabilities.go',
    'packages/runtime-go/internal/app/runtimeinfo/capabilities.go',
    'packages/runtime-go/internal/app/runtimeinfo/capability_assembly.go',
    'packages/runtime-go/internal/app/runtimeinfo/contract_capabilities.go',
    'packages/runtime-go/internal/app/runtimeinfo/info_response.go',
    'packages/runtime-go/internal/app/runtimeinfo/public_projection.go'
  ]
)

expectGoReplaySourcesExcludedFromProd(
  'Go fixture/replay source build-tag boundary scan',
  'packages/runtime-go'
)

expectRgMatches(
  'post-881 provider contract token scan',
  'providerCacheCoverageFloor|derivedUrlMatchCaseIds|customFullEndpointAppendedPathCount|customFullEndpointRequestShapeOnly|telemetrySupportedExcludesCustomFullEndpoints|customProviderCacheTelemetryClaimAllowed',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 provider raw accounting contract token scan',
  'rawProviderCacheAccounting|rawPayloadParsedCaseIds|rawTelemetrySupportedCaseIds|rawMatchesExpectedUsageCaseIds|rawUsageFromProviderPayload',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 planner/task contract token scan',
  'plannerToolsetInventory|parallelValidation|controlExecutableCases\\.planner|create_plan',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 combined step/cancel/cache contract token scan',
  'routeCacheReusedUntilStepLimit|sameTurnRouterCalls|nextTurnRouterCalls|combinedStepCancelCacheExpected|cancelResults|planStepCancelCache',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 plan step/cancel/cache G5 shadow token scan',
  'planStepCancelCache|followUpOnlyCreatePlan|cancelledStepDoesNotAdvanceCacheBaseline|nextPlanReusesOriginalCacheBaseline',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 plan cancel state reset G5 shadow token scan',
  'planCancelStateReset|cancelledPlanDoesNotLeakMode|autoTurnReroutesAfterCancel|autoRouterRequestIsolated',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 approval/user-input contract token scan',
  'approvalUserInputRouteReplay|approvalUserInputInventory|resolvedEventIncludesAnswers|resumePendingGates|answersCopiedToResume',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 goal persistence off-lock contract token scan',
  'goalPersistenceOffLock|noForbiddenControllerLock|goalWritesOutsideSharedStatusApprovalLock|persistenceFailureWarns',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 tool result file/image contract token scan',
  'toolResultFileImageBoundary|inlineImageKindsPreserved|attachmentLocalFilePathPreserved|generatedFileMetaLifted|FilePath: /tmp/picked/shot\\.png',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 event JSONL replay contract token scan',
  'eventJsonlReplayBoundary|appendNewlineTerminated|malformedJsonlLineSkipped|usageCompactionKeepsCarryover|compactionFailureKeepsAppendOnlyLog',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 MCP malformed schema contract token scan',
  'mcpMalformedSchemaBoundary|nonObjectSchemaDefaults|propertiesArrayDropped|requiredNonStringsDropped|modelCatalogSchemaSafe',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 desktop bridge IPC contract token scan',
  'desktopSovereignty|runtimeRequestUsesAnalytixIpc|runtimeSseUsesAnalytixIpc|publicApiTypesAnalytixOwned|forbiddenRuntimeIpcExposed',
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'post-881 renderer bridge allow-list G5 contract token scan',
  [
    'rendererBridgeAllowList',
    'rendererNamedRuntimeApisAllowListed',
    'rendererNamedSettingsApisAllowListed',
    'rendererGenericRuntimeBypassAbsent',
    'rendererSettingsReadBypassAbsent',
    'optionalChainBridgeAccessScanned'
  ],
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 desktop main IPC boundary contract token scan',
  'desktopMainIpcBoundary|runtimeRequestHandlerRejectsBeforeRuntimeCall|sseRejectsReasonixSessionPayload|sseUsesAnalytixThreadEventsRoute|sseBatchesEventsAt100ms',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 renderer route surface sovereignty contract token scan',
  'rendererRouteSurfaceSovereignty|appRouteUnionKunCompatible|dormantWorkflowCodeQuarantined|browserPreviewBridgeAnalytixOnly|pluginMarketplaceSafeAreaPropagates',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 package runtime identity contract token scan',
  'packageRuntimeIdentity|rootPackageNameAnalytix|runtimeBinAnalytixServeEntry|serveEntryAllowsOnlyAnalytixServe|afterPackRequiresServeEntry',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 runtime HTTP route sovereignty contract token scan',
  'runtimeHttpRouteSovereignty|routeCountExact|onlyHealthUnauthenticated|sseRouteAnalytixThreadEvents|singularUserInputCompatibilityOnly',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 runtime HTTP auth matrix contract token scan',
  'requires auth on every registered /v1 runtime route|authMatrix|allAuthenticatedRoutesRejectMissingAuth|unauthorizedBodyShapeStable|sensitiveRoutesProtected|POST /v1/runtime/task-jobs/wait|POST /v1/sessions/:id/resume-thread',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 runtime HTTP forbidden route dispatch contract token scan',
  'returns structured not_found for forbidden public route tokens|forbiddenDispatchMatrix|forbiddenDispatchReturnsStructuredNotFound|forbiddenProtocolTokensRejected|forbiddenHiddenSurfaceTokensRejected|/v1/runtime/go|/session-api',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 shared endpoint builder sovereignty contract token scan',
  'URL-encodes route ids|sharedEndpointBuilderMatrix|sharedEndpointBuildersEncodeRouteIds|sharedEndpointTemplatesAnalytixOwned|sharedEndpointCanonicalUserInputPlural|sharedEndpointBuilderUnitEvidencePresent|/v1/user-inputs/\\{id\\}|FORBIDDEN_PUBLIC_ENDPOINT',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 renderer runtime endpoint builder contract token scan',
  'URL-encodes dynamic runtime route ids before calling the bridge|ANALYTIX_HEALTH_PATH|ANALYTIX_THREADS_PATH|encodedThread|encodedSession|rendererProviderEndpointMatrix|rendererProviderEncodesDynamicRouteIds|rendererProviderRuntimePathsAnalytixOwned|rendererProviderUsesRuntimeClientFacade',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 renderer runtime provider alias guard contract token scan',
  'legacy runtime provider alias should not be read|keeps renderer runtime requests on analytix-owned HTTP routes|calls Analytix fork and user-input compatibility endpoints|URL-encodes dynamic runtime route ids before calling the bridge',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 renderer runtime provider alias guard G5 shadow token scan',
  'rendererProviderAliasGuardMatrix|rendererProviderAliasGuardInstallsThrowingAliases|rendererProviderAliasGuardCoversRuntimeRoutes|rendererProviderAliasGuardUnitEvidencePresent',
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'post-881 renderer runtime provider facade contract token scan',
  [
    'archiveThread\\(threadId: string, archived: boolean\\)',
    "updateThreadRelation\\(threadId: string, relation: NonNullable<NormalizedThread\\['relation'\\]>\\)",
    'archive thread failed',
    'update thread relation failed',
    'rendererRuntimeClient\\.runtimeRequest'
  ],
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 renderer runtime provider facade seal G5 shadow token scan',
  'rendererProviderFacadeSealMatrix|rendererProviderFacadeSealUsesRuntimeClient|rendererProviderFacadeSealRejectsDirectBridge|rendererProviderFacadeSealUnitEvidencePresent',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 side conversation provider relation contract token scan',
  "promoteSideConversation clears the relation through the provider and refreshes the thread list|updateThreadRelation\\(sideId, 'primary'\\)|relationMock|refreshThreadsMock",
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 side conversation relation G5 shadow token scan',
  'sideConversationRelationContractMatrix|sideConversationRelationPromotesThroughProvider|sideConversationRelationRejectsDirectBridge|sideConversationRelationUnitEvidencePresent',
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'post-881 renderer usage runtime client facade contract token scan',
  [
    'loadThreadUsage\\(threadId: string\\)',
    'loadDailyUsage\\(range: DailyUsageRange\\)',
    'loadModelUsage\\(range: DailyUsageRange\\)',
    'loadTokenEconomySavingsSummary\\(\\)',
    'rendererRuntimeClient\\.runtimeRequest'
  ],
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 renderer usage runtime client facade seal G5 shadow token scan',
  'rendererUsageRuntimeClientFacadeMatrix|rendererUsageRuntimeClientCoversThreadUsage|rendererUsageRuntimeClientCoversSettingsDiagnostics|rendererUsageRuntimeClientUnitEvidencePresent',
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'post-881 renderer settings read facade contract token scan',
  [
    'useKeyboardShortcutSettings\\(\\)',
    'useSpeechToTextEnabled\\(\\)',
    'setModelLabel\\(settings\\.runtime\\.model\\.trim\\(\\)\\)',
    'rendererRuntimeClient\\.getSettings'
  ],
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 renderer settings read facade seal G5 shadow token scan',
  'rendererSettingsReadFacadeMatrix|rendererSettingsReadFacadeCoversKeyboardShortcuts|rendererSettingsReadFacadeRejectsDirectBridge|rendererSettingsReadFacadeScanGuardPresent',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 AutoResearch direction tracking G5 shadow token scan',
  'directionTrackingFileWritten|iterationLogRecordsDirection|recordResearchDirectionToolPresent|recordDirectionRequiresActiveResearchGoal',
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'post-881 renderer named bridge allow-list contract token scan',
  [
    'renderer named runtime API allow-list scan',
    'renderer named settings API allow-list scan',
    'directRuntimeRequestBridgePattern',
    'directSettingsGetBridgePattern',
    'windowAnalytixRuntimeApiPattern',
    'windowAnalytixSettingsApiPattern',
    'getAnalytixConfigFile',
    'saveSettingsSilent'
  ],
  ['scripts/scan-product-sovereignty.cjs']
)

expectRgMatches(
  'post-881 main IPC endpoint builder allow-list contract token scan',
  'accepts shared endpoint builder output for encoded dynamic ids|rejects unencoded dynamic route ids and singular user-input compatibility paths|analytixSessionResumePath|/v1/user-input/input_raw|endpointBuilderAllowListMatrix|mainIpcEndpointBuilderAcceptsSharedPaths|mainIpcEndpointBuilderRejectsRawDynamicRoutes|mainIpcEndpointBuilderUsesSharedTemplates|mainIpcEndpointBuilderUnitEvidencePresent',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 main IPC runtime adapter handoff contract token scan',
  'passes encoded shared endpoint builder paths to the runtime adapter unchanged|rejects raw dynamic and singular compatibility paths before runtime adapter handoff|toHaveBeenNthCalledWith|analytixThreadCheckpointRewindPlanPath|runtimeAdapterHandoffMatrix|mainIpcRuntimeAdapterPreservesEncodedPaths|mainIpcRuntimeAdapterRejectsBeforeCall|mainIpcRuntimeAdapterUnitEvidencePresent',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 preload runtime request bridge contract token scan',
  'passes runtime request paths through the analytix runtime facade unchanged|keeps diagnostics runtime requests on the same analytix IPC channel|analytixThreadSteerPath|runtime:request|preloadRuntimeRequestBridgeMatrix|preloadRuntimeRequestPreservesPathMethodBody|preloadDiagnosticsRuntimeRequestUsesSameChannel|preloadRuntimeRequestUnitEvidencePresent',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 runtime host URL handoff contract token scan',
  'preserves encoded shared endpoint paths when forwarding to the runtime host|runtime-host-path|timezone=Asia%2FShanghai|thr%2Fwith%20space%3Fx%3D1%23frag|runtimeHostHandoffMatrix|mainRuntimeHostHandoffPreservesEncodedPathAndQuery|mainRuntimeHostHandoffPreservesMethodHeadersBody|mainRuntimeHostHandoffUnitEvidencePresent',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 preload SSE bridge contract token scan',
  'passes SSE start and stop requests through the analytix runtime facade unchanged|forwards SSE event, end, and error payloads without exposing Electron events|runtime:sse:start|runtime:sse-error|preloadSseBridgeMatrix|preloadSseStartStopPreservesArguments|preloadSsePayloadListenersOmitElectronEvent|preloadSseBridgeUnitEvidencePresent',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 main SSE host URL encoding contract token scan',
  'URL-encodes SSE thread ids before fetching the analytix runtime host|thr%2Fwith%20space%3Fx%3D1%23frag/events|Last-Event-ID|text/event-stream|mainSseHostEncodingMatrix|mainSseHostEncodesThreadIdAndCursor|mainSseHostPreservesHeadersAndStreamId|mainSseHostEncodingUnitEvidencePresent',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 renderer runtime client bridge contract token scan',
  'passes runtime requests through window\\.analytix without rewriting arguments|passes SSE control and listener handlers through window\\.analytix unchanged|legacy bridge alias should not be read|analytixThreadSteerPath|rendererRuntimeClientBridgeMatrix|rendererRuntimeClientRuntimeRequestPreservesArguments|rendererRuntimeClientSseControlsPreserveArguments|rendererRuntimeClientUnitEvidencePresent',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 renderer settings bridge contract token scan',
  'passes top-level runtime settings patches through window\\.analytix\\.settings without legacy aliases|deepseek-reasoner|approvalPolicy|legacy bridge alias should not be read|rendererSettingsBridgeMatrix|rendererSettingsBridgePreservesTopLevelRuntimePatch|rendererSettingsBridgeCachesReads|rendererSettingsBridgeUnitEvidencePresent',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 session route replay contract token scan',
  'sessionRouteReplay|archiveResponseHash|searchResponseHash|forkResponseHash|resumeResponseHash|replaySseHash|unauthorizedBodyHash',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 MCP contract token scan',
  'mcpCoreLifecycle|mcpSearchMetaTools|mcpSearchRefreshDrift|mcpLiveLocalIndexer|mcpApprovalAnnotations|mcpCallReconnect',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 MCP search refresh drift token scan',
  'mcpSearchRefreshDrift|mcp_refresh_catalog|catalogDrift|totalIndexed|topLevelRouteExposed',
  post881RuntimeContractTokenPaths
)

expectRgMatches(
  'post-881 MCP call-time reconnect G5 shadow token scan',
  'mcpCallReconnect|transportErrorRetried|protocolErrorRetried|staleCallSucceeded',
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'post-881 Go live-local sidecar prototype token scan',
  [
    'NewLiveLocalSidecarHandler',
    'LiveLocalSidecarProductBoundary',
    'TestConformanceOnly',
    'ReadOnlyRouteReplayOnly',
    'ProviderLiveCallsAllowed',
    'ApprovalExecutionAllowed',
    'MCPCredentialsAllowed',
    'FileMutationAllowed',
    'readOnlyG2Routes'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'post-881 Go isolated mutating G2 sidecar token scan',
  [
    'NewLiveLocalSidecarHarness',
    'LiveLocalSidecarSnapshot',
    'IsolatedMutableG2LifecyclePrototype',
    'IsolatedInMemoryStoreOnly',
    'mutatingG2Routes',
    'TestLiveLocalSidecarIsolatedMutatingG2LifecycleMatchesTypeScriptContract',
    'TestLiveLocalSidecarMutationsDoNotTouchFilesystem',
    'EventsJSONLWriteAttempts',
    'RealWorkspaceWriteAttempts'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'post-881 Go G3 provider/cache streaming live-local sidecar token scan',
  [
    'LiveLocalG3ProviderHandler',
    '/v1/conformance/g3/provider',
    'ProviderContract\\s+provider\\.G3ProviderConformanceContract',
    'FixtureBackedProviderG3Prototype',
    'FixtureBackedProviderOnly',
    'ExternalNetworkAllowed',
    'ProviderCredentialsAllowed',
    'APIKeyReadAllowed',
    'StubProviderUsageReplays',
    'StubProviderShapeReplays',
    'StubProviderStreamReplays',
    'StubProviderCacheReplays',
    'TestLiveLocalSidecarG3ProviderUsageAndCacheReplayMatchesTypeScriptContract',
    'TestLiveLocalSidecarG3ProviderRequestShapeReplayMatchesTypeScriptContract',
    'TestLiveLocalSidecarG3ProviderStreamingReplayMatchesTypeScriptContract',
    'ParsedProviderUsageFromRawPayload',
    'DerivedG3ProviderRequestURL',
    'BuildG3ProviderCacheAccounting',
    'BuildProviderDriftAttribution'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'post-881 Go G4 approval/user-input/MCP manager live-local sidecar token scan',
  [
    'LiveLocalG4ManagerHandler',
    '/v1/conformance/g4/manager',
    'G4Contract\\s+mcp\\.G4ToolsConformanceContract',
    'ApprovalUserInputContract\\s+agent\\.ApprovalUserInputRouteContract',
    'MCPToolLifecycleContract\\s+mcp\\.MCPToolLifecycleContract',
    'IsolatedG4ManagerPrototype',
    'FixtureBackedG4ManagerOnly',
    'ToolExecutionAttempts',
    'MCPConnectionAttempts',
    'CredentialReadAttempts',
    'StubG4ApprovalReplays',
    'StubG4UserInputReplays',
    'StubG4MCPReplays',
    'StubG4ValidationReplays',
    'TestLiveLocalSidecarG4ApprovalUserInputReplayMatchesTypeScriptContract',
    'TestLiveLocalSidecarG4MCPReplayMatchesTypeScriptContract',
    'handleApprovalDecision',
    'handleUserInputValidation',
    'handleMCPCallReconnect',
    'handleMCPDiagnostics'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'post-881 Go kernel live scaffold token scan',
  [
    'go-kernel-live-scaffold-contract',
    'GoKernelLiveScaffoldContract',
    'LiveLocalKernelHandler',
    '/v1/conformance/kernel',
    'KernelLiveScaffoldPrototype',
    'FixtureBackedKernelOnly',
    'InternalGateOnly',
    'StubKernelScaffoldReplays',
    'TestLiveLocalKernelScaffoldRoutesMatchContract',
    'Go kernel live scaffold sidecar conformance',
    'job-subagent-orchestration',
    'session-scoped Job Manager',
    'subagent continue/fork lineage guard',
    'mcp-catalog-recovery',
    'provider-aware cache accounting'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'post-881 Go G6 retirement cleanup token scan',
  [
    'g6-retirement-cleanup',
    'retirementCleanup',
    'GoKernelRetirementCleanup',
    'deleteAfterG6',
    'currentRetainedPaths',
    'forbiddenRedundancy',
    'presentForbiddenRedundancyCount',
    'tsRuntimeRetained',
    'noImmediateProductionDelete',
    'internal-conformance-env-gate',
    'fixture-conformance-routes',
    'shadow-only-g5-replay'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'Go production-candidate parity slice token scan',
  [
    'go-production-candidate',
    'runtimeGoContractParitySlice',
    'GoHTTPProviderClient',
    'providerscript.RunLocalProviderContract',
	'deepseekCacheFieldsConsistent',
    'durableReplayFromEventSink',
    'runApprovalUserInputContractExercise',
    'runMCPManagerContractExercise',
    'runJobLineageContractExercise',
    'SingleBaselineChecklist',
    'ANALYTIX_GO_RUNTIME_PRODUCTION_CANDIDATE',
    'go-production-candidate-canary'
  ],
  post881RuntimeContractTokenPaths
)

expectNoRgMatches(
  'Go production-candidate retired local/fake/fixture field scan',
  '\\b(?:live-provider-scripted-server|live-provider-fake-server|usesLocalScriptedProviderServer|usesLocalFakeProviderServer|usesLocalFakeMCPTransport|localScriptedProviderServer|fakeMCPTransportUsed|fixtureMCPTransportUsed|fake-mcp-manager|analytix-go-d0241)\\b',
  [
    'src/main/runtime/analytix-adapter.ts',
    'src/main/runtime/analytix-adapter.test.ts',
    'packages/runtime-go/internal/conformance/livelocal/production_candidate_handler.go',
    'packages/runtime-go/internal/mcp/manager_test_double.go',
    'packages/runtime-go/internal/testsupport/providerscript/provider_script.go',
    'packages/runtime-go/live_production_candidate_test.go',
    'packages/runtime-go/mcp_lifecycle_contract_test.go',
    'packages/runtime-go/internal/mcp/manager_test.go',
    'packages/runtime/tests/go-production-candidate-conformance.test.ts'
  ]
)

expectNoRgMatches(
  'Go upstream audit retired proof/fake/oracle/D token scan',
  '\\b(?:d0(?:24|25)[a-z0-9_-]*|D0(?:24|25)[A-Za-z0-9_-]*|proof|oracle|fake|mcpLocalProof)\\b',
  ['packages/runtime-go/internal/upstreamaudit'],
  ['*.go']
)

expectAllRgMatches(
  'Runtime Go server contract slice token scan',
  [
    'go-runtime-candidate',
    'ANALYTIX_GO_RUNTIME_CANDIDATE',
    'cmd/runtime-server',
    'NewRuntimeServerContractHandler',
    'RuntimeServerContractProductBoundary',
    'go-runtime-candidate-sse-replay',
    'go-production-internal-route-hidden'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'Runtime Go readiness hardening token scan',
  [
    'RuntimeReadinessStatusFromEnv',
    'RunProviderReadinessMatrix',
    'RunMCPReadinessMatrix',
    'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS',
    'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS',
    'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS',
    'runtime-durable-root',
    'go-runtime-candidate-g6-readiness',
    'runtime-go-packaged-qa',
    'packaged-app-startup',
    'typescript-retired-backend'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'Reasonix capability absorption matrix token scan',
  [
    'BuildReasonixCapabilityAuditMatrix',
    'EvaluateGoalEvidence',
    'RunMCPLifecycleAudit',
    'GoalEvidenceAuditEvent',
    'MCPLifecycleAuditEvent',
    'goal_evidence_audit',
    'mcp_lifecycle_audit',
    'mcp-lifecycle-contract',
    'runtime-mcp-lifecycle-sse',
    'goal-evidence-contract-sse',
    'reasonixCapabilityMatrix',
    'goal-evidence-kernel',
    'autoresearch-project-state',
    'reasonix-public-protocol',
    'capabilities\\.upstreamAbsorption'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'Kun baseline and multi-model absorption token scan',
  [
    'BuildKunAnalytixBaselineGuard',
    'BuildReasonixAbsorptionMatrix',
    'RunMultiModelNonRegressionMatrix',
    'ProviderTurnConfig',
    'github\\.com/KunAgent/Kun\\.git refs/tags/v0\\.2\\.13',
    'github\\.com/KunAgent/Kun\\.git refs/tags/v0\\.2\\.14',
    'https://github\\.com/esengine/DeepSeek-Reasonix',
    'fullFunctionBaseline',
    'multi-model-non-regression',
    'provider-model-multimodel',
    'deepSeekEnhancementScopedOnly',
    'deepseekOnlyRuntimeAllowed',
    'Reasonix DeepSeek capability absorption is provider-specific enhancement, not provider narrowing',
    'custom_endpoint',
    'openai-compatible',
    'anthropic-compatible'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'AutoResearch Go runtime project state token scan',
  [
    'AutoResearchProjectStore',
    'AutoResearchStateAuditEvent',
    'autoresearch_state_audit',
    '\\.analytix/autoresearch',
    'task_spec\\.md',
    'progress\\.json',
    'findings\\.jsonl',
    'directions_tried\\.json',
    'iteration_log\\.jsonl',
    'unknownRequirementAccepted',
    'stablePrefixContainsState',
    'toolSchemaContainsState',
    'topLevelAutoResearchRouteExposed',
    'writesReasonixFile',
    'writesAgentsFile',
    'autoresearch-state-contract-sse'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'Runtime Go readiness semantics token scan',
  [
    'BuildRuntimeReadinessSemantics',
    'readinessSemantics',
    'defaultBackendReadiness',
    'capabilityMatrixGreen',
    'absorptionMatrixGreen',
    'matrixGreenEnablesDefaultBackend',
    'fixtureMatrixCountsAsCredentialedPass',
    'runtime-go-preflight',
    'credentialedProviderMatrix',
    'credentialedMCPMatrix'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'Runtime Go strict preflight gate token scan',
  [
    'retiredGateIds',
    'runtime-go-product-regression',
    'runtime-go-speed-cache-gate',
    'runtime-go-preflight',
    'dryRunCannotAuthorizeCutover',
    'expectedBlocked',
    'provider-matrix-credentialed',
    'mcp-matrix-credentialed',
    'rendererVisibleGoSwitcher',
    'runtime:go:preflight'
  ],
	  [
	    ...post881RuntimeContractTokenPaths,
	    ...formalRuntimeValidationPaths,
	    ...archivedExistingRuntimeEvidencePaths,
	    'package.json'
	  ]
	)

expectAllRgMatches(
  'Runtime Go worktree final gate token scan',
  [
    'worktreeFinalGateBlockers',
    'worktree:non-goal-dirty-files',
    'worktree:generated-artifacts',
    'worktree:tracked-generated-artifacts',
    'runtime-go-worktree-audit.test.ts',
    'worktree:clean-or-classify',
    'trackedGeneratedArtifactDirtyFileCount',
    'untrackedGeneratedArtifactDirtyFileCount',
    'nonGoalScopeDirtyFiles',
    'trackedGeneratedArtifactDirtyFiles',
    'destructiveCleanupSuggested: false',
    'finalAcceptanceRequiresCleanOrClassifiedTree'
  ],
  [
    'scripts/runtime-go-worktree-audit.mjs',
    'src/main/runtime-go-worktree-audit.test.ts',
    'scripts/runtime-go-preflight.mjs',
    'src/main/runtime/analytix-adapter.test.ts',
    'docs/analytix/upstreams/kun-reasonix-feature-delta-audit.md'
  ]
)

expectAllRgMatches(
  'Runtime Go operator evidence clean-scope token scan',
  [
    'runtimeGoRelevantEvidencePaths',
    'relevantEvidenceFinalGateBlockers',
    'evidence:relevant-clean-scope-dirty',
    'relevantEvidenceDirtyFileCount',
    'operator evidence target has uncommitted runtime evidence changes',
    'hard gate scripts in operator relevant-clean evidence scope',
    'package.json',
    'packages/runtime/package.json',
    'scripts/cache-first-review-gate.mjs',
    'scripts/scan-product-sovereignty.cjs',
    'scripts/runtime-go-validation-delegate.mjs',
    'scripts/runtime-go-local-validation.mjs',
    'scripts/runtime-go-runtime-health-smoke.mjs',
    'scripts/runtime-go-performance-check.mjs',
    'scripts/runtime-go-speed-cache-gate.mjs',
    'scripts/runtime-go-product-regression.mjs',
    'src/main/runtime-go-worktree-audit.test.ts',
    'src/main/runtime/runtime-go-live-evidence-collector.test.ts'
  ],
  [
    'scripts/runtime-go-worktree-audit.mjs',
    'scripts/runtime-go-preflight.mjs',
    'scripts/runtime-go-cutover-report.mjs',
    'scripts/runtime-go-live-evidence-collector.mjs',
    'src/main/runtime-go-worktree-audit.test.ts',
    'src/main/runtime/runtime-go-live-evidence-collector.test.ts'
  ]
)

expectAllRgMatches(
  'Runtime Go live evidence dry-run no-write token scan',
  [
    'const noWrite = Boolean(options.noWrite) || dryRun',
    'does not overwrite persisted live evidence in dry-run mode',
    'dryRun',
    'noWrite',
    'persistentEvidenceWritten',
    'persistentEvidenceWritten: false',
    'outputPaths: \\{\\}',
    'evidencePaths: \\{\\}',
    'DEFAULT_PACKAGED_TIMEOUT_MS',
    'packagedTimeoutMs',
    '--packaged-timeout-ms',
    'requests actual packaged aggregate evidence',
    'requests actual packaged session soak evidence',
    'live-evidence-report.json',
    'live-evidence-summary.md'
  ],
  [
    'scripts/runtime-go-live-evidence-collector.mjs',
    'src/main/runtime/runtime-go-live-evidence-collector.test.ts'
  ]
)

expectAllRgMatches(
  'Runtime Go operator dependency summary token scan',
  [
    'dependencySummary',
    'operatorDependencySummary',
    'operatorCurrentEnvGate',
    'currentEnvGate',
    'evidenceEnvGate',
    'evidenceRefreshRequired',
    'evidenceRefreshCommand',
    'defaultReadinessCommand',
    'operatorGateDetail',
    'requiresOperatorEvidenceRefresh',
    'operatorBlockedByDependencies',
    'operatorDependencyBlockers',
    'envGate',
    'runtimeReady',
    'operatorApprovesDefault',
    'providerPassed',
    'mcpPassed',
    'packagedPassed',
    'packagedGuiRequired',
    'packagedSoakRequired',
    'packagedSoakActualFinalEvidencePresent',
    'credentialedEvidenceReviewed',
    'commitBinding',
    'relevantEvidenceClean',
    'blockers',
    'operator dependency summary'
  ],
  [
    'scripts/runtime-go-live-evidence-collector.mjs',
    'scripts/runtime-go-preflight.mjs',
    'scripts/runtime-go-default-readiness-report.mjs',
    'src/main/runtime/runtime-go-live-evidence-collector.test.ts',
    'src/main/runtime/analytix-adapter.test.ts',
    'src/main/runtime-go-packaged-contract-report.test.ts',
    'docs/analytix/upstreams/kun-reasonix-feature-delta-audit.md'
  ]
)

expectAllRgMatches(
  'Runtime Go candidate final gate token scan',
  [
    'runPreflightGate',
    'runtime:go:preflight',
    'runtime:go:preflight -- --json --gate',
    'preflightGate',
    'finalGateEnabled',
    'finalGateBlocked',
    'finalGateBlockers',
    'finalGateBlockerSummary',
    'requiresExternalInput',
    'requiresNonDeepSeekProvider',
    'providerGateDetail',
    'credentialedDeepSeekPassed',
    'credentialedNonDeepSeekProviderIds',
    'settingsProfileCoverage',
    'usableNonDeepSeekProviderIds',
    'requiresOperatorApproval',
    'requiresLocalHygiene',
    'requiresDeterministicFix',
    'deterministicCheckEnv',
    'localCheckEnv',
    'deterministicGateBlockers',
    'deterministic-default-readiness-not-passed',
    'deterministic-check:',
    'deterministicDefaultReady',
    'operator gate env',
    'does not leak operator gate env into preflight local checks',
    'goDefaultReady',
    'runtime-go-default-readiness-report'
  ],
  [
    'scripts/runtime-go-default-readiness-report.mjs',
    'scripts/runtime-go-preflight.mjs',
    'src/main/runtime-go-packaged-contract-report.test.ts'
  ]
)

expectAllRgMatches(
  'Runtime Go cutover final gate token scan',
  [
    'worktreeAudit',
    'worktreeFinalGateBlockers',
    'finalGateBlocked',
    'finalGateBlockers',
    'worktreeFinalAcceptanceRequiresCleanOrClassifiedTree',
    'deterministic checks passed, but preflight final blockers remain',
    'runtime-go-cutover-report'
  ],
  [
    'scripts/runtime-go-worktree-audit.mjs',
    'scripts/runtime-go-cutover-report.mjs',
    'src/main/runtime-go-packaged-contract-report.test.ts'
  ]
)

expectAllRgMatches(
  'Reasonix integration topology token scan',
  [
    'BuildReasonixIntegrationTopology',
    'reasonixIntegrationTopology',
    'IntegrationDecisionKunAnalytixBaselineRetained',
    'IntegrationDecisionReasonixEngineStrongerAbsorb',
    'IntegrationDecisionConflictProductBaselineEngineAbs',
    'baselineSurfaceCount',
    'reasonixStrongerAbsorbedCount',
    'kunAnalytixRetainedSurfaceCount',
    'deepseek-cache-prefix-provider-adapter',
    'agent-loop-job-subagent-lineage',
    'mcp-lifecycle-search-call-reconnect-redaction',
    'autoresearch-project-state-goal-research',
    'workflow-create-loop-internal-planner',
    'StrictG6DefaultCutoverReady',
    'GoDefaultCutoverCandidate',
    'TypeScriptFallbackRetained'
  ],
  post881RuntimeContractTokenPaths
)

expectAllRgMatches(
  'Runtime Go domain package organization token scan',
  [
    'Runtime-Go Domain Package Organization',
    'TestRootCompatibilityShimsDelegateToInternalDomains',
    'Compatibility shim',
    'internal/server',
    'internal/provider',
    'internal/mcp',
    'internal/upstreamaudit',
    'TypeScript fallback',
    'ANALYTIX_GO_RUNTIME_G6_READY=1',
    'runtime-go-cutover-report'
  ],
  [
    ...post881RuntimeContractTokenPaths,
    'packages/runtime-go/README.md',
    'docs/analytix/upstreams/go-runtime-conformance.md',
    'docs/analytix/runtime/go-runtime-conformance.md',
    'docs/analytix/upstreams/go-runtime-retirement-checklist.md'
  ]
)

expectAllRgMatches(
  'Runtime Go cutover evidence intake token scan',
  [
    'runtime-go-cutover-report',
    'runtime-go-operator-gate',
    'operatorGate',
    'credentialedEvidenceReviewed',
    'goDefaultApproved',
    'requiredCoverage',
    'reasonixEngineRuntimeOnly',
    'typeScriptFallbackRetained',
    'runtime:go:cutover-report'
  ],
	  [
	    ...post881RuntimeContractTokenPaths,
	    ...formalRuntimeValidationPaths,
	    'src/main/runtime/analytix-adapter.ts',
    'src/main/runtime/analytix-adapter.test.ts',
    'package.json'
  ]
)

expectAllRgMatches(
  'Reasonix superiority evidence matrix token scan',
  [
    'BuildReasonixSuperiorityMatrix',
    'reasonixSuperiorityMatrix',
    'runtime-go-engine-absorption-report',
    'legacyMatrixId',
    'legacyCodeStageReportId',
    'llmAnswerQualityEvidenceUsed',
    'deterministicEvidenceOnly',
    'kunAnalytixProductLayerPreserved',
    'reasonixEngineRuntimeOnly',
    'runtime:go:engine-absorption-report'
  ],
	  [
	    ...post881RuntimeContractTokenPaths,
	    ...formalRuntimeValidationPaths,
	    'packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix.go',
    'packages/runtime-go/reasonix_superiority_matrix_test.go',
    'packages/runtime-go/internal/server/capabilities.go',
    'packages/runtime/src/contracts/capabilities.ts',
    'packages/runtime/tests/contracts.test.ts',
    'packages/runtime/tests/go-runtime-conformance.test.ts',
	    'package.json'
	  ]
	)

expectAllRgMatches(
  'post-881 release evidence final gate scan',
  [
    'Final command gate for D-0172',
    'Final command gate for D-0173',
    'Final command gate for D-0174',
    'Final command gate for D-0175',
    'Final command gate for D-0176',
    'Final command gate for D-0177',
    'Final command gate for D-0178',
    'Final command gate for D-0179',
    'Final command gate for D-0180',
    'Final command gate for D-0181',
    'Final command gate for D-0182',
    'Final command gate for D-0183',
    'Final command gate for D-0184',
    'Final command gate for D-0185',
    'Final command gate for D-0186',
    'Final command gate for D-0187',
    'Final command gate for D-0188',
    'Final command gate for D-0189',
    'Final command gate for D-0190',
    'Final command gate for D-0191',
    'Final command gate for D-0192',
    'Final command gate for D-0193',
    'Final command gate for D-0194',
    'Final command gate for D-0195',
    'Final command gate for D-0196',
    'Final command gate for D-0197',
    'Final command gate for D-0198',
    'Final command gate for D-0199',
    'Final command gate for D-0200',
    'Final command gate for D-0201',
    'Final command gate for D-0202',
    'Final command gate for D-0203',
    'Final command gate for D-0204',
    'Final command gate for D-0205',
    'Final command gate for D-0206',
    'Final command gate for D-0207',
    'Final command gate for D-0208',
    'Final command gate for D-0209',
    'Final command gate for D-0210',
    'Final command gate for D-0211',
    'Final command gate for D-0212',
    'Final command gate for D-0213',
    'Final command gate for D-0214',
    'Final command gate for D-0215',
    'Final command gate for D-0216',
    'Final command gate for D-0217',
    'Final command gate for D-0218',
    'Final command gate for D-0219',
    'Final command gate for D-0220',
    'Final command gate for D-0221',
    'Final command gate for D-0222',
    'Final command gate for D-0223',
    'Final command gate for D-0224',
    'Final command gate for D-0225',
    'Final command gate for D-0226',
    'Final command gate for D-0227',
    'Final command gate for D-0228',
    'Final command gate for D-0229',
    'Final command gate for D-0230',
    'Final command gate for D-0231',
    'Final command gate for D-0232',
    'Final command gate for D-0233',
    'Final command gate for D-0234',
    'Final command gate for D-0235',
    'Runtime package tests.*npm --prefix packages/runtime test',
    'Workspace tests.*npm run test',
    'Workspace typecheck.*npm run typecheck',
    'Runtime package build.*npm run build:runtime',
    'Go shadow tests, non-cached.*go test -count=1 ./\\.\\.\\.',
    'Product sovereignty scan.*npm run scan:product-sovereignty'
  ],
  releaseEvidenceContractPaths
)

expectAllRgMatches(
  'post-881 stage closure capability scan',
  [
    'post881StageClosureCapabilityFloor',
    'reasonixAbsorbedDeltas',
    'kunBaselinePreserved',
    'analytixExceedsReasonixWhere',
    'remainingOpenGates',
    'Provider cache accounting',
    'Step/cancel/cache stability',
    'Approval/user-input gates',
    'Thread/session routes and SSE replay',
    'Go live-local sidecar prototype',
    'Go live-local isolated mutating G2 lifecycle prototype',
    'Go live-local G3 provider/cache streaming prototype',
    'No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry',
    'No live provider/cache superiority matrix',
    'No credentialed Go provider matrix',
    'No production Go thread/session router',
    'No Electron-connected or renderer-visible live Go HTTP server'
  ],
  post881StageClosurePaths
)

expectAllRgMatches(
  'post-881 sub-agent review matrix scan',
  [
    'post881SubagentReviewMatrix',
    'subagentAReasonixAgentKernel',
    'subagentBKunBaseline',
    'subagentCRuntimeCache',
    'subagentDMcpToolSubagent',
    'subagentEGoRuntime',
    'subagentFQaDocs',
    'customProviderRequestShapeOnly',
    'goRuntimeConformanceRangeCorrected',
    'subagentOpenGates'
  ],
  post881SubagentReviewPaths
)

expectNoRgMatchesExcept(
  'top-level forbidden entry scan',
  'Workflow|Create Loop|create-loop|createLoop|AutoResearch|auto-research|autoResearch|MCP-indexer|MCP Indexer|mcp-indexer|mcpIndexer|workflowCreateLoop',
  rendererEntryPaths,
  ['!**/*.test.ts'],
  []
)

expectAppRouteAllowList(
  'top-level renderer route allow-list',
  'src/renderer/src/store/chat-store-types.ts',
  ['chat', 'write', 'settings', 'plugins', 'claw', 'schedule']
)

expectNoRgMatches(
  'Workflow/Create Loop quarantine scan',
  'WorkflowCreateLoopView|runCreateLoopWorkflow|findPendingWorkflowGate|analytix-create-loop',
  workflowQuarantineEntryPaths,
  ['!**/*.test.ts']
)

expectNoRgMatches(
  'runtime public route/client forbidden surface scan',
  '/v1/reasonix|/v1/runtime/go|/v1/workflows?|/v1/create-loop|/v1/subagents?|/v1/autoresearch|/v1/mcp-indexer|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol',
  runtimeRouteClientSourcePaths,
  ['!**/*.test.ts']
)

expectNoRgMatches(
  'product MCP legacy diagnostic field scan',
  '\\b(?:fakeMCPTransportUsed|fixtureMCPTransportUsed|mcpLocalProof|contractProof)\\b',
  productRuntimeSurfacePaths,
  ['!**/*.test.ts', '!**/*.test.tsx']
)

expectPackageScriptsNoLegacyRuntimeMarkers(
  'runtime package script legacy marker scan'
)

expectRuntimeValidationRegistryNoLegacyMarkers(
  'runtime validation command registry legacy marker scan',
  'scripts/runtime-go-validation-command.mjs'
)

expectAllRgMatches(
  'Go runtime production handler composition root scan',
  [
    'runtimeapp.PrepareRuntimeServerStartupWithPersistenceLeaseContextE',
    'runtimeapp.Config',
    'internal/runtimeapp'
  ],
  ['packages/runtime-go/cmd/runtime-server/main.go']
)

expectAllRgMatches(
  'Go runtime production runtimeapp facade scan',
  [
    'type Config = server.RuntimeServerConfig',
    'func NewRuntimeServerHandler\\(config Config\\) http.Handler'
  ],
  ['packages/runtime-go/internal/runtimeapp/app.go']
)

expectNoRgMatches(
  'Go runtime production contract handler alias scan',
  'NewRuntimeServerContractHandler|RuntimeServerContractConfig',
  ['packages/runtime-go/cmd/runtime-server/main.go']
)

expectAllRgMatches(
  'Go runtime production internal handler naming scan',
  [
    'type RuntimeServerConfig struct',
    'func NewRuntimeServerHandlerFromComponents\\(',
    'type runtimeServerHandler struct'
  ],
  [
    'packages/runtime-go/internal/server/runtime_components.go',
    'packages/runtime-go/internal/server/runtime_handler.go',
    'packages/runtime-go/internal/server/runtime_server_config_prod.go'
  ]
)

expectNoRgMatches(
  'Go runtime production internal contract handler naming scan',
  'runtimeServerContractHandler|type RuntimeServerContractConfig struct',
  [
    'packages/runtime-go/internal/server/runtime_components.go',
    'packages/runtime-go/internal/server/runtime_server_config_prod.go'
  ]
)

expectGoAnalytixProdSourceSetClean(
  'Go runtime analytix_prod source-set retired token scan'
)

expectNoRgMatches(
  'renderer direct runtime bridge bypass scan',
  directRuntimeRequestBridgePattern,
  ['src/renderer/src'],
  ['!**/*.test.ts']
)

expectNoRgMatches(
  'renderer direct settings read bypass scan',
  directSettingsGetBridgePattern,
  ['src/renderer/src'],
  ['!**/*.test.ts']
)

expectWindowAnalytixApiAllowList(
  'renderer named runtime API allow-list scan',
  windowAnalytixRuntimeApiPattern,
  [
    'acceptedSlotDisplay',
    'cancelFundsCSVImport',
    'cleaningDiffPreview',
    'confirmFundsCSVSnapshot',
    'directSourcePreview',
    'fetchUpstreamModels',
    'getAnalytixConfigFile',
    'importMappingPreview',
    'onRuntimeStatus',
    'openAnalytixConfigDir',
    'probeModelCapabilities',
    'probeModelProvider',
    'restartRuntime',
    'revokeCleaningDiffPreview',
    'runDeterministicFundsCleaning',
    'stageFundsCSVSnapshot',
    'statusFundsCSVImport',
    'setAnalytixConfigFile'
  ],
  ['src/renderer/src'],
  ['!**/*.test.ts']
)

expectWindowAnalytixApiAllowList(
  'renderer named settings API allow-list scan',
  windowAnalytixSettingsApiPattern,
  [
    'saveSettingsSilent',
    'setSettings'
  ],
  ['src/renderer/src'],
  ['!**/*.test.ts']
)

expectNoRgMatches(
  'legacy native layout production scan',
  `runtime[\\\\/]data-native|['"]runtime['"]\\s*,\\s*['"]data-native['"]|['"]runtime['"]\\s*\\/\\s*['"]data-native['"]`,
  [
    'electron-builder.config.cjs',
    'electron-builder.standard-win.cjs',
    'package.json',
    '.github/workflows/release.yml',
    'scripts',
    'installer/winforms-bootstrapper',
    'src/main/data-analysis',
    'backend/app',
    'packages/runtime-go'
  ],
  [
    '!scripts/scan-product-sovereignty.cjs',
    '!**/*.test.*',
    '!**/*_test.go',
    '!**/node_modules/**'
  ]
)

expectNoRgMatchesExcept(
  'Kun/Reasonix identity scan',
  'window\\.kun|window\\.reasonix|kunGui|reasonixGui|KunAgent|Kun Desktop|kun serve|KUN_|agentProvider\\s*:\\s*[\'"]kun[\'"]|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol',
  ['src', 'packages'],
  ['!**/*.test.ts', '!packages/runtime/src/conformance/fixtures/*.json'],
  [
    ...legacyKunSettingsMigrationAllowedLinePatterns,
    '^packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix\\.go:.*Reasonix SessionAPI, config roots, CLI identity',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix\\.go:.*Reasonix SessionAPI, config roots, CLI identity',
    '^packages/runtime-go/internal/upstreamaudit/baseline_absorption\\.go:.*github\\.com/KunAgent/Kun',
    '^packages/runtime-go/internal/upstreamaudit/baseline_absorption\\.go:.*github\\.com/KunAgent/Kun',
    '^packages/runtime-go/internal/upstreamaudit/baseline_absorption\\.go:.*KunAgent/Kun v0\\.2\\.13',
    '^packages/runtime-go/internal/upstreamaudit/baseline_absorption\\.go:.*KunAgent/Kun v0\\.2\\.13',
    '^packages/runtime-go/internal/upstreamaudit/baseline_absorption\\.go:.*Reasonix SessionAPI, config roots, CLI identity',
    '^packages/runtime-go/internal/upstreamaudit/baseline_absorption\\.go:.*Reasonix SessionAPI, config roots, CLI identity',
    '^packages/runtime-go/kun_analytix_baseline_absorption_test\\.go:.*KunAgent/Kun',
    '^packages/runtime-go/reasonix_superiority_matrix_test\\.go:.*Reasonix public protocol must be rejected',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix\\.go:.*Reasonix SessionAPI/config roots/public job protocol/top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer UI',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix\\.go:.*Reasonix SessionAPI/config roots/public job protocol/top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer UI',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix\\.go:.*Reasonix public protocol or top-level product entries',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix\\.go:.*Reasonix public protocol or top-level product entries',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix\\.go:.*Reasonix public protocol and top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entries remain rejected',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix\\.go:.*Reasonix public protocol and top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entries remain rejected'
  ]
)

expectNoRgMatchesExcept(
  'release/package identity scan',
  'window\\.kun|window\\.reasonix|kunGui|reasonixGui|KunAgent|Kun Desktop|kun serve|KUN_|REASONIX_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol|com\\.kun|com\\.reasonix|kun-desktop|reasonix-desktop|Kun\\.app|Reasonix\\.app',
  ['package.json', 'packages/runtime/package.json', 'electron-builder.config.cjs', 'scripts', 'build'],
  ['!scripts/scan-product-sovereignty.cjs'],
  []
)

expectNoRgMatchesExcept(
  'deprecated bridge/settings fallback scan',
  'window\\.kun|window\\.reasonix|window\\.deepseek|kunGui|reasonixGui|agents\\.kun|agentProvider\\s*:\\s*[\'"]kun[\'"]|KUN_|deprecated bridge|window\\.analytixGui|window\\.deepseek',
  ['src/shared', 'src/main', 'src/preload', 'src/renderer/src', 'packages/runtime/src'],
  ['!**/*.test.ts'],
  legacyKunSettingsMigrationAllowedLinePatterns
)

expectNoRgMatchesExcept(
  'default Go/Rust/Tauri scan',
  'defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf',
  ['src', 'packages', 'electron-builder.config.cjs', 'package.json', 'scripts', 'build'],
  ['!**/*.test.ts', '!scripts/scan-product-sovereignty.cjs'],
  [
    '^src/main/runtime/analytix-adapter\\.ts:.*ANALYTIX_GO_RUNTIME_CONFORMANCE',
    '^src/main/runtime/analytix-adapter\\.ts:.*ANALYTIX_GO_RUNTIME_PRODUCTION_CANDIDATE',
    '^src/main/runtime/analytix-adapter\\.ts:.*ANALYTIX_GO_RUNTIME_CANDIDATE',
    '^src/main/runtime/analytix-adapter\\.ts:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^src/main/runtime/analytix-adapter\\.ts:.*ANALYTIX_GO_RUNTIME_CANDIDATE_DURABLE_ROOT',
    '^src/main/runtime/analytix-adapter\\.ts:.*ANALYTIX_GO_RUNTIME_DIR',
    '^src/main/runtime/analytix-adapter\\.ts:.*ANALYTIX_GO_RUNTIME_FIXTURES_DIR',
    '^src/main/runtime/analytix-adapter\\.ts:.*ANALYTIX_GO_RUNTIME_SERVER_BIN',
    '^src/main/runtime/analytix-adapter\\.ts:.*ANALYTIX_GO_RUNTIME_SERVER_CACHE_DIR',
    '^src/main/runtime/analytix-adapter\\.ts:.*ANALYTIX_GO_BIN',
    '^src/main/runtime/analytix-adapter\\.ts:.*defaultGoBackendEnabled: true',
    '^packages/runtime/tests/go-test-toolchain\\.ts:.*ANALYTIX_GO_BIN',
    '^packages/runtime-go/internal/readiness/readiness\\.go:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix\\.go:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix\\.go:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_integration_topology\\.go:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_integration_topology\\.go:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/internal/upstreamaudit/baseline_absorption\\.go:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/internal/upstreamaudit/baseline_absorption\\.go:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/internal/readiness/readiness_semantics\\.go:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/g6_readiness\\.go:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/README\\.md:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^scripts/after-pack\\.cjs:.*ANALYTIX_GO_BIN',
    '^scripts/runtime-go-packaged-milestone-a\\.mjs:.*runtimeBackend(?:TopologyEvidence|ProcessCount)',
    formalEvidenceRuntimeBackendCountAllowedLine,
    '^scripts/runtime-go-live-evidence-collector\\.mjs:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix\\.go:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix\\.go:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/g6_readiness_test\\.go:.*ANALYTIX_GO_RUNTIME_G6_READY',
    '^packages/runtime-go/runtime_server_test\\.go:.*ANALYTIX_GO_BIN'
  ]
)

expectNativeComponentRegistry('native component registry / Cargo / Tauri boundary')

expectNoRgMatches(
  'Connect Phone locale scan',
  '\\b(?:Claw|LobsterAI)\\b',
  ['src/renderer/src/locales'],
  ['!**/*.test.ts']
)

if (process.exitCode) {
  process.exit(process.exitCode)
}

console.log('[scan:product-sovereignty] pass')
