import { spawnSync } from 'node:child_process'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { runGoTestPartition } from './go-test-partition.mjs'

export const runtimePackage = 'analytix.local/runtime-go/internal/runtimeapp'
export const rootPackage = 'analytix.local/runtime-go'
// These actual protected-process positives supplement the Linux root suite.
// Only the stdio positive is Darwin-only; the other root tests retain their
// Linux ordinary-shell/fail-closed coverage in the packages partition.
export const rootPlatformTests = [
  'TestRuntimeServerConfiguredStdioMCPToolLoopRejectsMutationWithoutHostSemanticIdentityAndContinues',
  'TestRuntimeServerCheckpointApplyBlocksStagedGitChanges',
  'TestRuntimeServerInterruptStopsLaterToolsInSameProviderStep',
  'TestRuntimeServerBashRunInBackgroundWithholdsOutputAndSupportsKill',
  'TestRuntimeServerGoalTodoCompleteStepAndFinalReadiness',
  'TestRuntimeServerCompleteStepRejectsMismatchedTodoIndexBeforeEvidence',
  'TestRuntimeServerBashApprovalDenyAndAllowControlExecution',
  'TestRuntimeServerBashReportsExitCodeAndDiagnostics',
  'TestRuntimeServerBashTimeoutKillsProcessGroupGrandchild',
  'TestRuntimeServerSideEffectIntentBlocksSecondEquivalentBashWrite'
]
// At four partitions, CI exhausted the 20-minute package deadline after
// only 36/91 and 44/92 top-level tests. Smaller partitions retain that deadline
// and the complete required inventory; JSON execution proves each selection.
export const runtimeShardCount = 16
export const runtimePlatformTests = [
  'TestRuntimeOptionalPluginOrdinaryLifecycle',
  'TestRuntimeHTTPHostScheduleListUsesExactContainedLoopback'
]
export const runtimePlatformSubtests = ['missing', 'disabled', 'incompatible', 'unauthorized', 'domain-semantic']
  .map(fault => `TestRuntimeOptionalPluginOrdinaryLifecycle/${fault}`)
export const runtimeExternalDiagnostics = [
  { test: 'TestRuntimeOptionalPublicFixturePathV1', reason: 'Explicit Owner task directory and fixture activation required.' },
  { test: 'TestRuntimeOptionalPluginPublicConsumerV1', reason: 'Explicit Owner public-consumer acceptance and isolated corpus/profile required.' },
  { test: 'TestRuntimeOptionalPluginUnauthorizedFirstMCPTerminalDiagnosticV1', reason: 'Explicit Owner terminal diagnostic and evidence output authority required.' },
  { test: 'TestLocalNonPublishablePackageInspectionWhenExplicitlyProvided', reason: 'Explicit exact packaged executable and local Funds data required.' }
]

export function runtimeShardIndex(value) {
  if (!/^(0|[1-9][0-9]*)$/.test(value || '') || Number(value) >= runtimeShardCount) {
    throw new Error(`Expected runtime shard 0..${runtimeShardCount - 1}.`)
  }
  return Number(value)
}

export function otherGoPackages(output) {
  const packages = output.trim().split(/\s+/)
  if (new Set(packages).size !== packages.length ||
    packages.some(name => !/^analytix\.local\/runtime-go(?:\/[a-zA-Z0-9_.-]+)*$/.test(name)) ||
    !packages.includes(runtimePackage) || packages.length < 2) {
    throw new Error('Go package inventory is incomplete or ambiguous.')
  }
  // Runtime tests have their own required shards and explicit platform transfer.
  return packages.filter(name => name !== runtimePackage)
}

function goTestInventory(output, packageName, minimumCount = 1) {
  const names = []
  let summaries = 0
  for (const raw of output.split(/\r?\n/)) {
    const line = raw.trim()
    if (!line) continue
    if (/^(Test|Example|Fuzz)[\p{L}\p{N}_]*$/u.test(line)) names.push(line)
    else if (line.startsWith(`ok\t${packageName}\t`) ||
      new RegExp(`^ok\\s+${packageName.replaceAll('.', '\\.')}\\s+`).test(line)) summaries++
    else throw new Error('Unexpected output in Go runtime test inventory.')
  }
  if (summaries !== 1 || names.length < minimumCount || new Set(names).size !== names.length) {
    throw new Error('Go runtime test inventory is incomplete or duplicated.')
  }
  return names.sort()
}

export function runtimeTestInventory(output) {
  return goTestInventory(output, runtimePackage, runtimeShardCount)
}

export function rootPlatformSelection(output, tags) {
  if (tags !== '') throw new Error('Root platform contracts require ordinary build tags.')
  const discovered = goTestInventory(output, rootPackage)
  if (rootPlatformTests.some(name => !discovered.includes(name))) {
    throw new Error('Go root platform inventory lost a required test.')
  }
  return [...rootPlatformTests]
}

export function runtimeTestPartition(output) {
  const discovered = runtimeTestInventory(output)
  const transferred = runtimePlatformTests.slice(0, 1)
  const external = runtimeExternalDiagnostics.map(entry => entry.test)
  const assigned = [...transferred, ...external]
  if (new Set(assigned).size !== assigned.length || assigned.some(name => !discovered.includes(name))) {
    throw new Error('Go runtime partition inventory lost a declared platform or external entry.')
  }
  const required = discovered.filter(name => !assigned.includes(name))
  if (required.length < runtimeShardCount) throw new Error('Go runtime partition would contain an empty shard.')
  const shards = Array.from({ length: runtimeShardCount }, () => [])
  required.forEach((name, index) => shards[index % runtimeShardCount].push(name))
  return {
    discovered, shards,
    platformRequired: transferred.map(test => ({ test, lane: 'runtime-platform', platform: 'darwin' })),
    externalDiagnostics: runtimeExternalDiagnostics.map(entry => ({ ...entry, status: 'not_executed' }))
  }
}

export function runtimeTestShards(output) {
  return runtimeTestPartition(output).shards
}

export function runtimePlatformSelection(output) {
  const discovered = runtimeTestInventory(output)
  if (runtimePlatformTests.some(name => !discovered.includes(name))) {
    throw new Error('Go runtime platform inventory lost a required test.')
  }
  return [...runtimePlatformTests]
}

export function goExitStatus(result) {
  return !result.error && !result.signal && Number.isInteger(result.status) ? result.status : 1
}

function inventory(args) {
  const result = spawnSync('go', args, { encoding: 'utf8', maxBuffer: 4 << 20 })
  process.stderr.write(result.stderr || '')
  if (goExitStatus(result) !== 0) {
    process.stdout.write(result.stdout || '')
    throw new Error('Go inventory command failed; tests were not admitted.')
  }
  return result.stdout
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [mode, shardValue, ...extra] = process.argv.slice(2)
    if (extra.length) throw new Error('Unexpected Go CI arguments.')
    const tags = process.env.TEST_TAGS || ''
    if (!['', 'analytix_prod'].includes(tags)) throw new Error('Unsupported Go CI build tags.')
    const args = ['test', '-count=1', '-p', '1', '-parallel', '2', '-timeout', '20m', '-tags', tags]
    if (mode === 'packages' && shardValue === undefined) {
      const packages = otherGoPackages(inventory(['list', '-tags', tags, './...']))
      console.log(`Go package partition: ${packages.length} packages; runtimeapp has ${runtimeShardCount} required shards, a required platform lane and explicit external diagnostics.`)
      args.push(...packages)
    } else if (mode === 'platform-root' && shardValue === undefined) {
      if (process.platform !== 'darwin' || tags !== '') {
        throw new Error('Root platform contracts must execute on Darwin with ordinary build tags.')
      }
      const listing = inventory(['test', '-count=1', '-tags', tags, '-list', '^(Test|Example|Fuzz)', rootPackage])
      const names = rootPlatformSelection(listing, tags)
      console.log(JSON.stringify({ package: rootPackage, tags, lane: 'runtime-platform', selected: names }))
      const pattern = `^(${names.join('|')})$`
      args.push('-json', '-run', pattern, rootPackage)
      await runGoTestPartition(args, rootPackage, names)
    } else if (mode === 'runtime' || (mode === 'platform' && shardValue === undefined)) {
      if ((mode === 'runtime' && process.platform !== 'linux') || (mode === 'platform' && process.platform !== 'darwin')) {
        throw new Error('Go runtime partition must execute on its declared platform.')
      }
      const listing = inventory(['test', '-count=1', '-tags', tags, '-list', '^(Test|Example|Fuzz)', runtimePackage])
      let names
      let requiredSubtests = []
      if (mode === 'runtime') {
        const index = runtimeShardIndex(shardValue)
        const partition = runtimeTestPartition(listing)
        names = partition.shards[index]
        console.log(JSON.stringify({ package: runtimePackage, tags, shard: index, total: partition.discovered.length, ...partition, selected: names }))
      } else {
        names = runtimePlatformSelection(listing)
        requiredSubtests = runtimePlatformSubtests
        console.log(JSON.stringify({ package: runtimePackage, tags, lane: 'runtime-platform', selected: names, requiredSubtests }))
      }
      const pattern = `^(${names.map(name => name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})$`
      args.push('-json', '-run', pattern, runtimePackage)
      await runGoTestPartition(args, runtimePackage, names, requiredSubtests)
    } else throw new Error(`Expected packages, platform, platform-root or runtime <0..${runtimeShardCount - 1}>.`)
    if (mode === 'packages') process.exitCode = goExitStatus(spawnSync('go', args, { stdio: 'inherit' }))
  } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}
