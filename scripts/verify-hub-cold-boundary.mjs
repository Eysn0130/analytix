import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { dirname, extname, relative, resolve } from 'node:path'
import ts from 'typescript'

const repoRoot = resolve(process.cwd())
const graphDir = resolve(process.env.ANALYTIX_HUB_COLD_GRAPH_DIR || 'out/hub-cold-build-graph')
const sourceOnly = process.argv.includes('--source-only')
const negativeFixturePath = resolve(repoRoot, 'scripts/fixtures/hub-cold-boundary-negative.json')

const mainRoots = ['src/main/index.ts']
const rendererRoots = [
  'src/renderer/src/main.tsx',
  'src/renderer/src/account/StartupAuthGate.tsx',
  'src/renderer/src/components/SettingsView.tsx',
  'src/renderer/src/components/chat/Sidebar.tsx',
  'src/renderer/src/components/chat/AcceptedSlotDisplay.tsx',
  'src/renderer/src/components/chat/FloatingComposerModelPicker.tsx'
]
const mainForbidden = new Set([
  'src/main/services/hub-account-service.ts',
  'src/main/services/hub-agent-marketplace-service.ts',
  'src/main/services/hub-gateway-runtime-secret.ts',
  'src/shared/hub-account.ts'
])
const rendererForbidden = new Set([
  'src/renderer/src/components/settings-section-account.tsx',
  'src/renderer/src/account/hub-account-store.ts',
  'src/renderer/src/account/AnimatedHubLoginPage.tsx',
  'src/renderer/src/components/chat/SidebarAccountMenu.tsx',
  'src/shared/hub-account.ts'
])
const allowedDynamicEdges = new Set([
  'src/main/index.ts->src/main/services/hub-account-service.ts',
  'src/main/ipc/register-app-ipc-handlers.ts->src/main/services/hub-agent-marketplace-service.ts',
  'src/renderer/src/components/SettingsView.tsx->src/renderer/src/components/settings-section-account.tsx'
])
const excludedNonElectronDynamicEdges = new Set([
  'src/renderer/src/main.tsx->src/renderer/src/lib/browser-analytix-bridge.ts'
])

const hubModuleClassifications = new Map(Object.entries({
  'src/main/analytix-process.ts': 'reference-only',
  'src/main/hub-activity-observation.ts': 'main-private-instrumentation',
  'src/main/index.ts': 'explicit-deprecated-account-boundary',
  'src/main/ipc/register-app-ipc-handlers.ts': 'explicit-marketplace-action-boundary',
  'src/main/plugin-install-marker.ts': 'reference-only',
  'src/main/plugin-install-verifier.ts': 'reference-only',
  'src/main/runtime/analytix-adapter.ts': 'reference-only',
  'src/main/runtime/bundled-funds-materialization.ts': 'reference-only',
  'src/main/services/hub-account-service.ts': 'deprecated-account-network-observed',
  'src/main/services/hub-agent-marketplace-service.ts': 'marketplace-network-observed',
  'src/main/services/hub-gateway-runtime-secret.ts': 'deprecated-account-token-dependency-observed',
  'src/main/services/skill-service.ts': 'reference-only',
  'src/preload/index.ts': 'public-action-bridge-only',
  'src/renderer/src/account/AnimatedHubLoginPage.tsx': 'deprecated-account-ui',
  'src/renderer/src/account/hub-account-store.ts': 'deprecated-account-ui',
  'src/renderer/src/account/hub-avatar.ts': 'deprecated-account-ui',
  'src/renderer/src/components/PluginMarketplaceView.tsx': 'explicit-marketplace-ui',
  'src/renderer/src/components/chat/FloatingComposer.tsx': 'reference-only',
  'src/renderer/src/components/chat/SidebarAccountMenu.tsx': 'deprecated-account-ui',
  'src/renderer/src/components/chat/message-timeline-bubbles.tsx': 'reference-only',
  'src/renderer/src/components/chat/sidebar-account-usage.ts': 'deprecated-account-ui',
  'src/renderer/src/components/settings-section-account.tsx': 'deprecated-account-ui-boundary',
  'src/renderer/src/lib/browser-analytix-bridge.ts': 'non-electron-preview-bridge',
  'src/shared/analytix-api.ts': 'public-action-contract-only',
  'src/shared/analytix-runtime-status.ts': 'reference-only',
  'src/shared/hub-account.ts': 'deprecated-account-contract'
}))

const hubBearingIdentifierMarker = /(?:analytix[- ]hub|Analytix Hub|Hub(?:Account|Activity|Agent|Gateway)|hub-(?:account|activity|agent|gateway))/u
const hubAuthoritativeConfigOrChannelMarker = /(?:ANALYTIX_(?:HUB|AGENT_PLUGIN_MARKETPLACE)_[A-Z0-9_]+|hub-agent-marketplace:)/u
const hubAuthoritativeEndpointMarker = /(?:https?:\/\/(?:[^/'"\s]+\.)?analytix\.top\/(?:api\/(?:auth|agent\/plugins)|v1)(?:[/?#'"\s]|$)|\/api\/(?:auth|agent\/plugins)(?:[/?#'"`]|$))/iu
const directNetworkPrimitiveMarker = /(?:\bfetch\s*\(|\b(?:http|https|client)\.request\s*\(|\brequestBuffer\s*\()/u
const hubActivityObservationRequirements = new Map([
  ['src/main/services/hub-account-service.ts', ['moduleLoad', 'serviceInstance', 'tokenRead', 'request', 'fallback']],
  ['src/main/services/hub-agent-marketplace-service.ts', ['moduleLoad', 'request', 'fallback']],
  ['src/main/services/hub-gateway-runtime-secret.ts', ['moduleLoad', 'tokenRead']]
])
const observedHubActivityClassifications = new Set([
  'deprecated-account-network-observed',
  'marketplace-network-observed',
  'deprecated-account-token-dependency-observed'
])
const ordinaryForbiddenClassifications = new Set([
  ...observedHubActivityClassifications,
  'deprecated-account-contract',
  'deprecated-account-ui',
  'deprecated-account-ui-boundary'
])

function posixPath(value) {
  return value.replaceAll('\\', '/')
}

function repoPath(value) {
  const absolute = resolve(value)
  const rel = posixPath(relative(repoRoot, absolute))
  return rel.startsWith('../') || rel === '..' ? null : rel
}

function sourceFileKind(filePath) {
  switch (extname(filePath)) {
    case '.tsx': return ts.ScriptKind.TSX
    case '.jsx': return ts.ScriptKind.JSX
    case '.js': return ts.ScriptKind.JS
    default: return ts.ScriptKind.TS
  }
}

function resolveSourceImport(importer, specifier) {
  let base
  if (specifier.startsWith('.')) {
    base = resolve(dirname(resolve(repoRoot, importer)), specifier)
  } else if (specifier.startsWith('@renderer/')) {
    base = resolve(repoRoot, 'src/renderer/src', specifier.slice('@renderer/'.length))
  } else if (specifier.startsWith('@shared/')) {
    base = resolve(repoRoot, 'src/shared', specifier.slice('@shared/'.length))
  } else {
    return null
  }
  const candidates = [
    base,
    base + '.ts',
    base + '.tsx',
    base + '.js',
    base + '.jsx',
    resolve(base, 'index.ts'),
    resolve(base, 'index.tsx'),
    resolve(base, 'index.js'),
    resolve(base, 'index.jsx')
  ]
  const found = candidates.find((candidate) => existsSync(candidate) && statSync(candidate).isFile())
  return found ? repoPath(found) : null
}

function importClauseIsTypeOnly(node) {
  if (!node.importClause) return false
  if (node.importClause.isTypeOnly) return true
  const bindings = node.importClause.namedBindings
  return Boolean(
    bindings &&
    ts.isNamedImports(bindings) &&
    bindings.elements.length > 0 &&
    bindings.elements.every((element) => element.isTypeOnly)
  )
}

function sourceImports(filePath) {
  const raw = readFileSync(resolve(repoRoot, filePath), 'utf8')
  const parsed = ts.createSourceFile(filePath, raw, ts.ScriptTarget.Latest, true, sourceFileKind(filePath))
  const staticImports = []
  const dynamicImports = []

  function visit(node) {
    if (ts.isImportDeclaration(node) && ts.isStringLiteral(node.moduleSpecifier)) {
      if (!importClauseIsTypeOnly(node)) staticImports.push(node.moduleSpecifier.text)
    } else if (ts.isExportDeclaration(node) && node.moduleSpecifier && ts.isStringLiteral(node.moduleSpecifier)) {
      if (!node.isTypeOnly) staticImports.push(node.moduleSpecifier.text)
    } else if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments.length === 1 &&
      ts.isStringLiteral(node.arguments[0])
    ) {
      dynamicImports.push(node.arguments[0].text)
    }
    ts.forEachChild(node, visit)
  }
  visit(parsed)

  return {
    staticImports: staticImports.map((specifier) => resolveSourceImport(filePath, specifier)).filter(Boolean),
    dynamicImports: dynamicImports.map((specifier) => resolveSourceImport(filePath, specifier)).filter(Boolean)
  }
}

function listProductionSourceFiles() {
  const queue = ['src/main', 'src/preload', 'src/renderer/src', 'src/shared']
  const files = []
  while (queue.length > 0) {
    const current = queue.shift()
    if (!current) continue
    for (const entry of readdirSync(resolve(repoRoot, current), { withFileTypes: true })) {
      const child = posixPath(current + '/' + entry.name)
      if (entry.isDirectory()) {
        queue.push(child)
      } else if (
        /\.(?:ts|tsx)$/u.test(entry.name) &&
        !/\.(?:test|spec)\.(?:ts|tsx)$/u.test(entry.name)
      ) {
        files.push(child)
      }
    }
  }
  return files.sort()
}

function isHubBearingProductionModule(filePath, source) {
  const normalizedPath = posixPath(filePath)
  const baseName = normalizedPath.slice(normalizedPath.lastIndexOf('/') + 1)
  const hubPathIdentity = /hub/iu.test(baseName)
  return hubPathIdentity ||
    hubBearingIdentifierMarker.test(source) ||
    hubAuthoritativeConfigOrChannelMarker.test(source) ||
    hubAuthoritativeEndpointMarker.test(source)
}

function directlyConstructsHubNetwork(source) {
  return directNetworkPrimitiveMarker.test(source) && hubAuthoritativeEndpointMarker.test(source)
}

function assertInventoryClassification(discovered, classified) {
  const discoveredSet = new Set(discovered)
  const classifiedSet = new Set(classified)
  for (const filePath of [...discoveredSet].sort()) {
    if (!classifiedSet.has(filePath)) throw new Error('hub_cold_inventory_unclassified:' + filePath)
  }
  for (const filePath of [...classifiedSet].sort()) {
    if (!discoveredSet.has(filePath)) throw new Error('hub_cold_inventory_stale_classification:' + filePath)
  }
}

function inspectHubModuleInventory() {
  const discovered = listProductionSourceFiles().filter((filePath) =>
    isHubBearingProductionModule(filePath, readFileSync(resolve(repoRoot, filePath), 'utf8'))
  )
  assertInventoryClassification(discovered, hubModuleClassifications.keys())
  return {
    moduleCount: discovered.length,
    classificationCount: new Set(hubModuleClassifications.values()).size,
    instrumentedModuleCount: hubActivityObservationRequirements.size
  }
}

function assertHubClassificationCoherence() {
  for (const [filePath, classification] of hubModuleClassifications) {
    const requirements = hubActivityObservationRequirements.get(filePath)
    const observedClass = observedHubActivityClassifications.has(classification)
    if (observedClass !== Boolean(requirements)) {
      throw new Error('hub_activity_observation_classification_mismatch:' + filePath)
    }
    if (observedClass && !mainForbidden.has(filePath)) {
      throw new Error('hub_activity_observation_module_not_cold:' + filePath)
    }
    if (!filePath.startsWith('src/main/')) continue
    const source = readFileSync(resolve(repoRoot, filePath), 'utf8')
    if (directlyConstructsHubNetwork(source) && !observedClass) {
      throw new Error('hub_activity_observation_network_class_missing:' + filePath)
    }
    const directlyAccessesToken = /(?:readGatewayToken|readStoredAuth|RuntimeTokenForProvider|hasHubGatewayRuntimeToken)/u.test(source)
    if (directlyAccessesToken && (!observedClass || !requirements?.includes('tokenRead'))) {
      throw new Error('hub_activity_observation_token_class_missing:' + filePath)
    }
  }

  for (const filePath of [...mainForbidden, ...rendererForbidden]) {
    const classification = hubModuleClassifications.get(filePath)
    if (!classification) throw new Error('hub_cold_forbidden_module_unclassified:' + filePath)
    if (!ordinaryForbiddenClassifications.has(classification)) {
      throw new Error('hub_cold_forbidden_classification_invalid:' + filePath)
    }
  }

  for (const edge of allowedDynamicEdges) {
    const target = edge.slice(edge.indexOf('->') + 2)
    if (!hubModuleClassifications.has(target)) {
      throw new Error('hub_cold_allowed_target_unclassified:' + target)
    }
    if (!mainForbidden.has(target) && !rendererForbidden.has(target)) {
      throw new Error('hub_cold_allowed_target_not_forbidden:' + target)
    }
  }

  for (const [filePath, dimensions] of hubActivityObservationRequirements) {
    const source = readFileSync(resolve(repoRoot, filePath), 'utf8')
    for (const dimension of dimensions) {
      if (!source.includes(`recordHubActivity('${dimension}')`)) {
        throw new Error('hub_activity_observation_dimension_missing:' + filePath + ':' + dimension)
      }
    }
    const schedulesTimer = /(?:^|[^\w.])(?:globalThis\.)?set(?:Interval|Timeout)\(/mu.test(source)
    if (schedulesTimer && !source.includes("recordHubActivity('refreshTimer')")) {
      throw new Error('hub_activity_observation_refresh_timer_missing:' + filePath)
    }
  }
}

function assertSyntheticGraphCold(roots, modules, forbidden, errorPrefix) {
  const reachable = traverse(roots, (moduleId) => modules[moduleId]?.staticImports || [])
  for (const moduleId of reachable) {
    if (forbidden.has(moduleId)) throw new Error(errorPrefix + ':' + moduleId)
  }
}

function runNegativeFixtures() {
  const fixture = JSON.parse(readFileSync(negativeFixturePath, 'utf8'))
  if (fixture?.schemaVersion !== 1 || !Array.isArray(fixture?.cases)) {
    throw new Error('hub_cold_negative_fixture_invalid')
  }
  for (const testCase of fixture.cases) {
    let observedError = ''
    try {
      if (testCase.kind === 'discovery-inventory') {
        const filePath = String(testCase.filePath || '')
        const source = String(testCase.source || '')
        const discovered = isHubBearingProductionModule(filePath, source) ? [filePath] : []
        assertInventoryClassification(discovered, [])
      } else if (testCase.kind === 'reachability') {
        assertSyntheticGraphCold(
          testCase.roots || [],
          testCase.modules || {},
          new Set(testCase.forbidden || []),
          'hub_cold_fixture_static_reachability'
        )
      } else {
        throw new Error('hub_cold_negative_fixture_unknown_kind:' + String(testCase.kind))
      }
    } catch (error) {
      observedError = error instanceof Error ? error.message : String(error)
    }
    if (observedError !== testCase.expectedError) {
      throw new Error('hub_cold_negative_fixture_not_rejected:' + String(testCase.name))
    }
  }
  return fixture.cases.length
}

function assertObservationRemainsMainPrivate() {
  const forbiddenMarkers = [
    'hub-activity-observation',
    'ANALYTIX_HUB_ACTIVITY_OBSERVATION',
    'observeHubActivity',
    'HubActivityObservation'
  ]
  const queue = ['src/preload', 'src/renderer/src', 'src/shared']
  let scannedFileCount = 0
  while (queue.length > 0) {
    const current = queue.shift()
    if (!current) continue
    const absolute = resolve(repoRoot, current)
    for (const entry of readdirSync(absolute, { withFileTypes: true })) {
      const child = posixPath(current + '/' + entry.name)
      if (entry.isDirectory()) {
        queue.push(child)
        continue
      }
      if (!/\.(?:ts|tsx)$/u.test(entry.name)) continue
      scannedFileCount += 1
      const source = readFileSync(resolve(repoRoot, child), 'utf8')
      if (forbiddenMarkers.some((marker) => source.includes(marker))) {
        throw new Error('hub_activity_observation_public_leak:' + child)
      }
    }
  }
  return scannedFileCount
}

function inspectSourceGraph(roots, forbidden) {
  const visited = new Set()
  const dynamicEdges = []
  const queue = [...roots]
  while (queue.length > 0) {
    const current = queue.shift()
    if (!current || visited.has(current)) continue
    visited.add(current)
    if (forbidden.has(current)) throw new Error('hub_cold_source_static_reachability:' + current)
    const imports = sourceImports(current)
    for (const target of imports.staticImports) queue.push(target)
    for (const target of imports.dynamicImports) {
      const edge = current + '->' + target
      dynamicEdges.push(edge)
      if (!allowedDynamicEdges.has(edge) && !excludedNonElectronDynamicEdges.has(edge)) queue.push(target)
    }
  }
  for (const edge of dynamicEdges) {
    const target = edge.slice(edge.indexOf('->') + 2)
    if ((mainForbidden.has(target) || rendererForbidden.has(target)) && !allowedDynamicEdges.has(edge)) {
      throw new Error('hub_cold_source_unapproved_dynamic_edge:' + edge)
    }
  }
  return { visited, dynamicEdges }
}

function requireAllowedSourceEdges(edges) {
  for (const allowed of allowedDynamicEdges) {
    if (!edges.has(allowed)) throw new Error('hub_cold_source_required_lazy_edge_missing:' + allowed)
  }
}

function loadBuildGraph(target) {
  const filePath = resolve(graphDir, target + '.json')
  if (!existsSync(filePath)) throw new Error('hub_cold_build_graph_missing:' + target)
  const graph = JSON.parse(readFileSync(filePath, 'utf8'))
  if (graph?.schemaVersion !== 1 || graph?.target !== target) {
    throw new Error('hub_cold_build_graph_invalid:' + target)
  }
  return graph
}

function traverse(start, next) {
  const seen = new Set()
  const queue = [...start]
  while (queue.length > 0) {
    const current = queue.shift()
    if (!current || seen.has(current)) continue
    seen.add(current)
    for (const target of next(current)) queue.push(target)
  }
  return seen
}

function inspectBuildGraph(target, graph, roots, forbidden, requiredEdges) {
  const modules = graph.modules || {}
  const chunks = graph.chunks || {}
  const sourceReachable = traverse(roots, (moduleId) => [
    ...(modules[moduleId]?.staticImports || []),
    ...(modules[moduleId]?.dynamicImports || []).filter((nextModuleId) => {
      const edge = moduleId + '->' + nextModuleId
      return !allowedDynamicEdges.has(edge) && !excludedNonElectronDynamicEdges.has(edge)
    })
  ])
  for (const moduleId of sourceReachable) {
    if (forbidden.has(moduleId)) throw new Error('hub_cold_build_static_reachability:' + target + ':' + moduleId)
  }

  const requiredTargets = []
  for (const edge of requiredEdges) {
    const split = edge.indexOf('->')
    const importer = edge.slice(0, split)
    const requiredTarget = edge.slice(split + 2)
    if (!(modules[importer]?.dynamicImports || []).includes(requiredTarget)) {
      throw new Error('hub_cold_build_required_lazy_edge_missing:' + edge)
    }
    requiredTargets.push(requiredTarget)
  }

  const rootChunks = Object.entries(chunks)
    .filter(([, value]) => roots.some((root) => value.modules.includes(root)))
    .map(([fileName]) => fileName)
  const compatibilityChunks = Object.entries(chunks)
    .filter(([, value]) => requiredTargets.some((requiredTarget) => value.modules.includes(requiredTarget)))
    .map(([fileName]) => fileName)
  if (compatibilityChunks.length !== requiredTargets.length) {
    throw new Error('hub_cold_build_compatibility_chunk_count:' + target)
  }
  const stoppedModules = target === 'renderer'
    ? [...requiredTargets, 'src/renderer/src/lib/browser-analytix-bridge.ts']
    : requiredTargets
  const stoppedChunks = new Set(Object.entries(chunks)
    .filter(([, value]) => stoppedModules.some((moduleId) => value.modules.includes(moduleId)))
    .map(([fileName]) => fileName))
  const ordinaryChunks = traverse(rootChunks, (fileName) => [
    ...(chunks[fileName]?.imports || []),
    ...(chunks[fileName]?.dynamicImports || [])
  ].filter((nextFileName) => Boolean(chunks[nextFileName]) && !stoppedChunks.has(nextFileName)))
  if (compatibilityChunks.some((fileName) => ordinaryChunks.has(fileName))) {
    throw new Error('hub_cold_build_compatibility_in_ordinary_chunk:' + target)
  }
  for (const fileName of ordinaryChunks) {
    for (const moduleId of chunks[fileName]?.modules || []) {
      if (forbidden.has(moduleId)) throw new Error('hub_cold_build_forbidden_module_in_ordinary_chunk:' + target)
    }
  }

  return {
    moduleCount: Object.keys(modules).length,
    chunkCount: Object.keys(chunks).length,
    ordinaryChunkCount: ordinaryChunks.size,
    compatibilityChunkCount: compatibilityChunks.length
  }
}

const hubInventory = inspectHubModuleInventory()
assertHubClassificationCoherence()
const negativeFixtureCount = runNegativeFixtures()
const mainSource = inspectSourceGraph(mainRoots, mainForbidden)
const rendererSource = inspectSourceGraph(rendererRoots, rendererForbidden)
requireAllowedSourceEdges(new Set([...mainSource.dynamicEdges, ...rendererSource.dynamicEdges]))
const publicObservationFileCount = assertObservationRemainsMainPrivate()

const report = {
  schemaVersion: 1,
  source: {
    mainModuleCount: mainSource.visited.size,
    rendererModuleCount: rendererSource.visited.size,
    requiredLazyEdges: allowedDynamicEdges.size,
    hubInventory,
    negativeFixtureCount,
    publicObservationFileCount,
    observationMainPrivate: true,
    passed: true
  },
  build: {
    executed: !sourceOnly,
    passed: false
  }
}

if (!sourceOnly) {
  const mainBuild = inspectBuildGraph(
    'main', loadBuildGraph('main'), mainRoots, mainForbidden,
    [
      'src/main/index.ts->src/main/services/hub-account-service.ts',
      'src/main/ipc/register-app-ipc-handlers.ts->src/main/services/hub-agent-marketplace-service.ts'
    ]
  )
  const rendererBuild = inspectBuildGraph(
    'renderer', loadBuildGraph('renderer'), rendererRoots, rendererForbidden,
    ['src/renderer/src/components/SettingsView.tsx->src/renderer/src/components/settings-section-account.tsx']
  )
  report.build = { executed: true, passed: true, main: mainBuild, renderer: rendererBuild }
}

process.stdout.write(JSON.stringify(report) + '\n')
