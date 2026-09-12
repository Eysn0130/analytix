import { spawnSync } from 'node:child_process'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const runtimePackage = 'analytix.local/runtime-go/internal/runtimeapp'
export const runtimeShardCount = 4

export function otherGoPackages(output) {
  const packages = output.trim().split(/\s+/)
  if (new Set(packages).size !== packages.length ||
    packages.some(name => !/^analytix\.local\/runtime-go(?:\/[a-zA-Z0-9_.-]+)*$/.test(name)) ||
    !packages.includes(runtimePackage) || packages.length < 2) {
    throw new Error('Go package inventory is incomplete or ambiguous.')
  }
  // This one package is fully covered by the required runtime-shards matrix.
  return packages.filter(name => name !== runtimePackage)
}

export function runtimeTestShards(output) {
  const names = []
  let summaries = 0
  for (const raw of output.split(/\r?\n/)) {
    const line = raw.trim()
    if (!line) continue
    if (/^(Test|Example|Fuzz)[\p{L}\p{N}_]*$/u.test(line)) names.push(line)
    else if (line.startsWith(`ok\t${runtimePackage}\t`) ||
      new RegExp(`^ok\\s+${runtimePackage.replaceAll('.', '\\.')}\\s+`).test(line)) summaries++
    else throw new Error('Unexpected output in Go runtime test inventory.')
  }
  if (summaries !== 1 || names.length < runtimeShardCount || new Set(names).size !== names.length) {
    throw new Error('Go runtime test inventory is incomplete or duplicated.')
  }
  const shards = Array.from({ length: runtimeShardCount }, () => [])
  names.sort().forEach((name, index) => shards[index % runtimeShardCount].push(name))
  return shards
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
    const [mode, shardValue] = process.argv.slice(2)
    const tags = process.env.TEST_TAGS || ''
    if (!['', 'analytix_prod'].includes(tags)) throw new Error('Unsupported Go CI build tags.')
    const args = ['test', '-count=1', '-p', '1', '-parallel', '2', '-timeout', '20m', '-tags', tags]
    if (mode === 'packages' && shardValue === undefined) {
      const packages = otherGoPackages(inventory(['list', '-tags', tags, './...']))
      console.log(`Go package partition: ${packages.length} packages; runtimeapp belongs to all ${runtimeShardCount} required shards.`)
      args.push(...packages)
    } else if (mode === 'runtime' && /^[0-3]$/.test(shardValue || '')) {
      const shards = runtimeTestShards(inventory(['test', '-count=1', '-tags', tags, '-list', '^(Test|Example|Fuzz)', runtimePackage]))
      const index = Number(shardValue)
      const names = shards[index]
      console.log(JSON.stringify({ package: runtimePackage, tags, shard: index, total: shards.flat().length, selected: names }))
      const pattern = `^(${names.map(name => name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})$`
      args.push('-run', pattern, runtimePackage)
    } else throw new Error('Expected packages or runtime <0..3>.')
    process.exitCode = goExitStatus(spawnSync('go', args, { stdio: 'inherit' }))
  } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}
