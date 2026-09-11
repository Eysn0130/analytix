import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { lstatSync, mkdtempSync, readFileSync, readdirSync, rmdirSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export function candidateImage(directory) {
  const candidates = readdirSync(directory).filter(name => /^analytix-\d+\.\d+\.\d+-mac-arm64\.dmg$/.test(name))
  if (candidates.length !== 1) throw new Error('Require exactly one versioned macOS ARM64 DMG in a fresh output directory')
  const image = join(directory, candidates[0])
  const stat = lstatSync(image)
  if (!stat.isFile() || !stat.size) throw new Error('Installer must be a nonempty regular file, not a symlink')
  return image
}

export function checkCandidateLayout(mount) {
  const app = join(mount, 'Analytix.app')
  for (const name of ['Contents/Info.plist', 'Contents/MacOS/analytix', 'Contents/Resources/app.asar', 'Contents/Resources/runtime-go/bin/runtime-server']) {
    const stat = lstatSync(join(app, name))
    if (!stat.isFile() || !stat.size) throw new Error(`Missing or invalid packaged product file: ${name}`)
  }
  return app
}

function run(command, args) {
  const result = spawnSync(command, args, { encoding: 'utf8', timeout: 60000, maxBuffer: 1024 * 1024, stdio: ['ignore', 'pipe', 'pipe'] })
  if (result.status !== 0) throw new Error(`${command} failed during installer verification; no artifact is accepted`)
  return result.stdout.trim()
}

export function smokeCandidate(directory) {
  if (process.platform !== 'darwin') throw new Error('macOS is required to verify this installer')
  const image = candidateImage(directory)
  const hash = () => createHash('sha256').update(readFileSync(image)).digest('hex')
  const digest = hash()
  run('/usr/bin/hdiutil', ['verify', image])
  const mount = mkdtempSync(join(tmpdir(), 'analytix-dmg-smoke-'))
  let attached = false
  try {
    run('/usr/bin/hdiutil', ['attach', '-readonly', '-nobrowse', '-mountpoint', mount, image])
    attached = true
    const app = checkCandidateLayout(mount)
    if (run('/usr/bin/plutil', ['-extract', 'CFBundleIdentifier', 'raw', '-o', '-', join(app, 'Contents/Info.plist')]) !== 'com.analytix.desktop') {
      throw new Error('Packaged app identity does not match Analytix')
    }
    run('/usr/bin/codesign', ['--verify', '--deep', '--strict', app])
  } finally {
    // Never recursively remove a mount point, especially after failed detach.
    if (attached) run('/usr/bin/hdiutil', ['detach', mount])
    rmdirSync(mount)
  }
  if (hash() !== digest) throw new Error('Installer changed during verification')
  writeFileSync(join(directory, 'SHA256SUMS'), `${digest}  ${image.slice(directory.length + 1)}\n`, { flag: 'wx', mode: 0o600 })
  return { passed: true, scope: 'DMG container, resource layout, bundle identity and code signature; not GUI, upgrade, notarization or release acceptance' }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    if (process.argv.length !== 3) throw new Error('Usage: node scripts/package-candidate-smoke.mjs <fresh-output-directory>')
    console.log(JSON.stringify(smokeCandidate(resolve(process.argv[2])), null, 2))
  } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}
