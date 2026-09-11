import { createHash } from 'node:crypto'
import { constants, copyFileSync, existsSync, lstatSync, mkdirSync, readFileSync, realpathSync, renameSync, chmodSync } from 'node:fs'
import { dirname, isAbsolute, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

export const assetManifest = JSON.parse(readFileSync(new URL('./runtime-assets.json', import.meta.url), 'utf8'))
const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..')

function confined(root, name) {
  if (isAbsolute(name) || name.split(/[\\/]/).includes('..') || name.includes('\\')) throw new Error('Invalid asset path')
  const base = realpathSync(root)
  const target = resolve(base, name)
  if (!relative(base, target) || relative(base, target).startsWith(`..${sep}`)) throw new Error('Asset escaped its root')
  let current = target
  while (current !== base) {
    if (lstatSync(current, { throwIfNoEntry: false })?.isSymbolicLink()) throw new Error('Asset paths must not contain symlinks')
    current = dirname(current)
  }
  return target
}

function matches(target, entry) {
  if (!existsSync(target)) return false
  const stat = lstatSync(target)
  if (!stat.isFile() || stat.size !== entry.bytes) return false
  return createHash('sha256').update(readFileSync(target)).digest('hex') === entry.sha256
}

export function verifyAssets(root = repo, target = `${process.platform}-${process.arch}`, manifest = assetManifest) {
  const selected = manifest.targets[target]
  if (!selected) return { target, passed: false, configured: false, missing: [], invalid: [], reason: 'No verified runtime asset set for this target' }
  const missing = [], invalid = []
  for (const entry of selected.files) {
    const path = confined(root, entry.path)
    if (!existsSync(path)) missing.push(entry.path)
    else if (!matches(path, entry)) invalid.push(entry.path)
  }
  return { target, configured: true, passed: missing.length === 0 && invalid.length === 0, missing, invalid }
}

export function prepareAssets(source, destination = repo, target = `${process.platform}-${process.arch}`, manifest = assetManifest) {
  const check = verifyAssets(source, target, manifest)
  if (!check.passed) throw new Error('Source archive is incomplete or does not match the pinned asset manifest')
  const files = manifest.targets[target].files
  // Validate the entire destination before any copy; never overwrite unknown data.
  for (const entry of files) {
    const path = confined(destination, entry.path)
    if (existsSync(path) && !matches(path, entry)) throw new Error(`Existing asset differs; preserve and review it first: ${entry.path}`)
  }
  let copied = 0
  for (const entry of files) {
    const from = confined(source, entry.path), to = confined(destination, entry.path)
    if (existsSync(to)) continue
    mkdirSync(dirname(to), { recursive: true, mode: 0o700 })
    const temporary = `${to}.prepare-${process.pid}`
    if (existsSync(temporary)) throw new Error('An unfinished asset preparation must be reviewed first')
    copyFileSync(from, temporary, constants.COPYFILE_EXCL)
    chmodSync(temporary, entry.executable ? 0o700 : 0o600)
    if (!matches(temporary, entry)) throw new Error('Asset changed during preparation')
    renameSync(temporary, to)
    copied++
  }
  return { ...verifyAssets(destination, target, manifest), copied }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const args = process.argv.slice(2)
    const command = args.shift() || 'verify'
    let target = `${process.platform}-${process.arch}`, source
    while (args.length) {
      const name = args.shift(), value = args.shift()
      if (!value || !['--target', '--from'].includes(name)) throw new Error('Usage: runtime-assets.mjs verify|prepare [--target os-arch] [--from archive-root]')
      if (name === '--target') target = value
      else source = value
    }
    if (!['verify', 'prepare'].includes(command) || (command === 'prepare' && !source)) throw new Error('Preparation requires an explicit --from archive-root')
    const result = command === 'prepare' ? prepareAssets(resolve(source), repo, target) : verifyAssets(repo, target)
    console.log(JSON.stringify(result, null, 2))
    if (!result.passed) process.exitCode = 1
  } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}
