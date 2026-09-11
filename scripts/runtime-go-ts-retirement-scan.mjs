#!/usr/bin/env node

import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, normalize, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import process from 'node:process'

const forbiddenRuntimeRoots = [
  'packages/runtime/src/server',
  'packages/runtime/src/loop',
  'packages/runtime/src/adapters/model',
  'packages/runtime/src/adapters/tool',
  'packages/runtime/src/delegation',
  'packages/runtime/src/services',
  'packages/runtime/src/review'
]

const forbiddenDistRoots = [
  'packages/runtime/dist/server',
  'packages/runtime/dist/loop',
  'packages/runtime/dist/adapters/model',
  'packages/runtime/dist/adapters/tool',
  'packages/runtime/dist/delegation',
  'packages/runtime/dist/services',
  'packages/runtime/dist/review'
]

const productionScanRoots = [
  'packages/runtime/src/index.ts',
  'packages/runtime/src/cli',
  'packages/runtime/src/contracts',
  'packages/runtime/src/config',
  'packages/runtime/src/telemetry',
  'packages/runtime/src/hooks/hook-config.ts',
  'packages/runtime/src/hooks/hook-phases.ts',
  'src/main',
  'src/preload',
  'src/renderer',
  'src/shared'
]

const forbiddenSpecPatterns = [
  /runtime-factory/i,
  /agent-loop/i,
  /compat-model-client/i,
  /multi-provider-model-client/i,
  /mcp-tool-provider/i,
  /local-tool-host/i,
  /delegation-runtime/i,
  /child-agent-executor/i,
  /turn-service/i,
  /thread-service/i
]

const forbiddenNamedImports = [
  'AgentLoop',
  'CompatModelClient',
  'MultiProviderModelClient',
  'DelegationRuntime',
  'TurnService',
  'ThreadService'
]

const allowedRuntimePackageExports = new Set([
  '.',
  './contracts',
  './config',
  './cli',
  './telemetry'
])

function posixPath(path) {
  return path.split('\\').join('/')
}

function repoPath(root, absolutePath) {
  return posixPath(relative(root, absolutePath))
}

function safeRead(path) {
  try {
    return readFileSync(path, 'utf8')
  } catch {
    return ''
  }
}

function walkFiles(rootPath, predicate = () => true) {
  if (!existsSync(rootPath)) return []
  const stat = statSync(rootPath)
  if (stat.isFile()) return predicate(rootPath) ? [rootPath] : []
  const files = []
  for (const entry of readdirSync(rootPath, { withFileTypes: true })) {
    if (entry.name === 'node_modules' || entry.name === 'dist' || entry.name === '.git') continue
    const child = join(rootPath, entry.name)
    if (entry.isDirectory()) {
      files.push(...walkFiles(child, predicate))
      continue
    }
    if (entry.isFile() && predicate(child)) files.push(child)
  }
  return files
}

function isSourceFile(path) {
  return /\.(?:mjs|cjs|js|jsx|ts|tsx)$/.test(path) &&
    !/\.test\.(?:js|jsx|ts|tsx)$/.test(path) &&
    !/\.spec\.(?:js|jsx|ts|tsx)$/.test(path) &&
    !/(^|\/)__tests__(?:\/|$)/.test(posixPath(path))
}

function importStatements(source) {
  const matches = []
  const importExportPattern = /\b(?:import|export)\s+(?:[^'"]*?\s+from\s*)?['"]([^'"]+)['"]/g
  const requirePattern = /\brequire\(\s*['"]([^'"]+)['"]\s*\)/g
  const dynamicImportPattern = /\bimport\(\s*['"]([^'"]+)['"]\s*\)/g
  for (const pattern of [importExportPattern, requirePattern, dynamicImportPattern]) {
    for (const match of source.matchAll(pattern)) {
      matches.push({
        statement: match[0],
        specifier: match[1]
      })
    }
  }
  return matches
}

function resolvesIntoForbiddenRuntimeRoot(root, filePath, specifier) {
  if (!specifier.startsWith('.')) return false
  const resolved = posixPath(normalize(resolve(dirname(filePath), specifier)))
    .replace(/\.(?:js|jsx|ts|tsx|mjs|cjs)$/, '')
  return forbiddenRuntimeRoots.some((item) => {
    const absoluteForbiddenRoot = posixPath(resolve(root, item))
    return resolved === absoluteForbiddenRoot || resolved.startsWith(`${absoluteForbiddenRoot}/`)
  })
}

function forbiddenImportReason(root, filePath, item) {
  if (resolvesIntoForbiddenRuntimeRoot(root, filePath, item.specifier)) {
    return 'production import resolves into retired TypeScript runtime source'
  }
  if (forbiddenSpecPatterns.some((pattern) => pattern.test(item.specifier))) {
    return 'production import names a retired TypeScript runtime module'
  }
  if (
    /(?:^|\/)(?:analytix-runtime|@analytix\/runtime|packages\/runtime)(?:\/|$)?/.test(item.specifier) &&
    forbiddenNamedImports.some((name) => item.statement.includes(name))
  ) {
    return 'production import requests a retired TypeScript runtime symbol'
  }
  return ''
}

function scanProductionImports(root) {
  const files = productionScanRoots.flatMap((item) => walkFiles(resolve(root, item), isSourceFile))
  const violations = []
  for (const file of files) {
    const source = safeRead(file)
    for (const statement of importStatements(source)) {
      const reason = forbiddenImportReason(root, file, statement)
      if (!reason) continue
      violations.push({
        path: repoPath(root, file),
        specifier: statement.specifier,
        reason
      })
    }
  }
  return {
    scannedFileCount: files.length,
    violations
  }
}

function scanDist(root) {
  const violations = []
  for (const item of forbiddenDistRoots) {
    const absolute = resolve(root, item)
    if (!existsSync(absolute)) continue
    const stat = statSync(absolute)
    violations.push({
      path: item,
      reason: stat.isDirectory()
        ? 'retired TypeScript runtime directory exists in packaged runtime dist'
        : 'retired TypeScript runtime module exists in packaged runtime dist'
    })
  }
  return violations
}

function scanPackageExports(root) {
  const packagePath = resolve(root, 'packages/runtime/package.json')
  const raw = safeRead(packagePath)
  if (!raw) {
    return [{ path: 'packages/runtime/package.json', reason: 'package manifest is missing' }]
  }
  let parsed
  try {
    parsed = JSON.parse(raw)
  } catch {
    return [{ path: 'packages/runtime/package.json', reason: 'package manifest is not valid JSON' }]
  }
  const violations = []
  const exported = parsed.exports && typeof parsed.exports === 'object' ? parsed.exports : {}
  const forbiddenExportPattern = /(?:^|\/)(?:server|loop|adapters|delegation|services|review)(?:\/|$)|runtime-factory|agent-loop|compat-model-client|multi-provider-model-client/i
  for (const [key, value] of Object.entries(exported)) {
    const serialized = `${key} ${JSON.stringify(value)}`
    if (!allowedRuntimePackageExports.has(key)) {
      violations.push({
        path: 'packages/runtime/package.json',
        export: key,
        reason: 'package exports expose an unapproved runtime package surface after TypeScript runtime retirement'
      })
      continue
    }
    if (forbiddenExportPattern.test(serialized)) {
      violations.push({
        path: 'packages/runtime/package.json',
        export: key,
        reason: 'package exports expose retired TypeScript runtime surface'
      })
    }
  }
  return violations
}

function scanBuildConfig(root) {
  const tsconfigPath = resolve(root, 'packages/runtime/tsconfig.build.json')
  const raw = safeRead(tsconfigPath)
  if (!raw) {
    return [{ path: 'packages/runtime/tsconfig.build.json', reason: 'runtime build tsconfig is missing' }]
  }
  let parsed
  try {
    parsed = JSON.parse(raw)
  } catch {
    return [{ path: 'packages/runtime/tsconfig.build.json', reason: 'runtime build tsconfig is not valid JSON' }]
  }
  const includes = Array.isArray(parsed.include) ? parsed.include : []
  const forbiddenIncludePattern = /(?:^|\/)src\/(?:server|loop|adapters\/model|adapters\/tool|delegation|services|review)(?:\/|\*|$)|runtime-factory|agent-loop|compat-model-client|multi-provider-model-client/i
  return includes
    .filter((item) => forbiddenIncludePattern.test(String(item)))
    .map((item) => ({
      path: 'packages/runtime/tsconfig.build.json',
      include: item,
      reason: 'runtime build includes retired TypeScript runtime source'
    }))
}

function scanSourceResidual(root) {
  const files = forbiddenRuntimeRoots.flatMap((item) => walkFiles(resolve(root, item), (path) =>
    /\.(?:ts|tsx)$/.test(path) && !/\.test\.(?:ts|tsx)$/.test(path)
  ))
  return files.map((file) => repoPath(root, file))
}

export function runTypeScriptRetirementScan(root = process.cwd()) {
  const repoRoot = resolve(root)
  const productionImports = scanProductionImports(repoRoot)
  const distViolations = scanDist(repoRoot)
  const packageExportViolations = scanPackageExports(repoRoot)
  const buildConfigViolations = scanBuildConfig(repoRoot)
  const sourceResiduals = scanSourceResidual(repoRoot)
  const productionViolationCount = productionImports.violations.length +
    distViolations.length +
    packageExportViolations.length +
    buildConfigViolations.length
  const typeScriptCodePathDeleted = productionViolationCount === 0
  const report = {
    schemaVersion: 1,
    id: 'runtime-go-ts-retirement-scan',
    status: typeScriptCodePathDeleted ? 'passed' : 'failed',
    passed: typeScriptCodePathDeleted,
    typeScriptCodePathDeleted,
    typeScriptRuntimeSourceDeleted: sourceResiduals.length === 0,
    productionForbiddenImportCount: productionImports.violations.length,
    packagedDistRuntimeModuleCount: distViolations.length,
    packageExportViolationCount: packageExportViolations.length,
    allowedRuntimePackageExports: [...allowedRuntimePackageExports],
    buildConfigViolationCount: buildConfigViolations.length,
    sourceRuntimeImplementationCount: sourceResiduals.length,
    scannedProductionFileCount: productionImports.scannedFileCount,
    productionForbiddenImports: productionImports.violations.slice(0, 40),
    packagedDistRuntimeModules: distViolations.slice(0, 40),
    packageExportViolations: packageExportViolations.slice(0, 40),
    buildConfigViolations: buildConfigViolations.slice(0, 40),
    sourceRuntimeImplementationSamples: sourceResiduals.slice(0, 40)
  }
  report.summary = typeScriptCodePathDeleted
    ? 'production exports, build output, and production imports do not expose retired TypeScript agent runtime paths'
    : 'production code still exposes retired TypeScript agent runtime paths'
  return report
}

function shouldRunCli() {
  const currentFile = fileURLToPath(import.meta.url)
  return process.argv[1] && resolve(process.argv[1]) === currentFile
}

if (shouldRunCli()) {
  const report = runTypeScriptRetirementScan(process.cwd())
  if (process.argv.includes('--json')) {
    console.log(JSON.stringify(report, null, 2))
  } else {
    console.log(`${report.status.toUpperCase()} ${report.id}: ${report.summary}`)
    for (const item of report.productionForbiddenImports) {
      console.log(`FAILED ${item.path}: ${item.specifier} (${item.reason})`)
    }
    for (const item of report.packagedDistRuntimeModules) {
      console.log(`FAILED ${item.path}: ${item.reason}`)
    }
    for (const item of report.packageExportViolations) {
      console.log(`FAILED ${item.path}: ${item.reason}`)
    }
    for (const item of report.buildConfigViolations) {
      console.log(`FAILED ${item.path}: ${item.reason}`)
    }
  }
  if (!report.passed) process.exitCode = 1
}
