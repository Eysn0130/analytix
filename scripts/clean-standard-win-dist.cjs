const fs = require('node:fs')
const path = require('node:path')

const repo = path.resolve(__dirname, '..')
const distDir = path.join(repo, 'dist-standard-win')
const outDir = path.join(repo, 'out')

function removeIfPresent(targetPath) {
  if (!fs.existsSync(targetPath)) return
  fs.rmSync(targetPath, { recursive: true, force: true })
}

fs.mkdirSync(distDir, { recursive: true })
removeIfPresent(outDir)

for (const entry of fs.readdirSync(distDir, { withFileTypes: true })) {
  const name = entry.name
  if (
    name === 'win-unpacked' ||
    name === 'builder-debug.yml' ||
    name === 'builder-effective-config.yaml' ||
    name.startsWith('._') ||
    /^latest.*\.ya?ml$/i.test(name) ||
    /^analytix-standard-.+/i.test(name) ||
    /\.blockmap$/i.test(name)
  ) {
    removeIfPresent(path.join(distDir, name))
  }
}

console.log(`[clean-standard-win-dist] cleaned ${distDir}`)
