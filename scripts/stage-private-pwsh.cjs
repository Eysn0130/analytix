const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const { execFileSync } = require('node:child_process')

const repo = path.resolve(__dirname, '..')
const source = String(process.env.ANALYTIX_PRIVATE_PWSH_SOURCE || '').trim()
const expectedSha256 = String(process.env.ANALYTIX_PRIVATE_PWSH_SHA256 || '').trim().toLowerCase()
const stageRoot = path.join(repo, 'build', 'private-pwsh')
const outputRoot = path.join(stageRoot, 'pwsh')
const extractRoot = path.join(stageRoot, 'extract')

function sha256File(filePath) {
  return crypto.createHash('sha256').update(fs.readFileSync(filePath)).digest('hex')
}

function walk(root, acc = []) {
  if (!fs.existsSync(root)) return acc
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const fullPath = path.join(root, entry.name)
    if (entry.isDirectory()) walk(fullPath, acc)
    else if (entry.isFile()) acc.push(fullPath)
  }
  return acc
}

function sha256Tree(root) {
  const hash = crypto.createHash('sha256')
  for (const filePath of walk(root).sort()) {
    const relative = path.relative(root, filePath).split(path.sep).join('/')
    hash.update(relative)
    hash.update('\0')
    hash.update(fs.readFileSync(filePath))
    hash.update('\0')
  }
  return hash.digest('hex')
}

function findPwshRoot(root) {
  for (const filePath of walk(root)) {
    if (path.basename(filePath).toLowerCase() === 'pwsh.exe') {
      return path.dirname(filePath)
    }
  }
  return ''
}

function copyPwshRoot(root) {
  fs.rmSync(outputRoot, { recursive: true, force: true })
  fs.mkdirSync(path.dirname(outputRoot), { recursive: true })
  fs.cpSync(root, outputRoot, { recursive: true, dereference: true })
}

function extractZip(sourcePath) {
  let path7za
  try {
    path7za = require('7zip-bin').path7za
  } catch (error) {
    throw new Error(`7zip-bin is required to extract private pwsh ZIP archives: ${error.message}`)
  }
  fs.rmSync(extractRoot, { recursive: true, force: true })
  fs.mkdirSync(extractRoot, { recursive: true })
  execFileSync(path7za, ['x', '-y', `-o${extractRoot}`, sourcePath], { stdio: 'inherit' })
  const root = findPwshRoot(extractRoot)
  if (!root) throw new Error(`Private pwsh ZIP does not contain pwsh.exe: ${sourcePath}`)
  return root
}

function main() {
  fs.rmSync(stageRoot, { recursive: true, force: true })
  if (!source) {
    console.log('[private-pwsh] disabled; set ANALYTIX_PRIVATE_PWSH_SOURCE to stage resources/pwsh.')
    return
  }
  if (!fs.existsSync(source)) {
    throw new Error(`ANALYTIX_PRIVATE_PWSH_SOURCE does not exist: ${source}`)
  }

  const stat = fs.statSync(source)
  let sourceKind
  let actualSha256
  let pwshRoot
  if (stat.isDirectory()) {
    sourceKind = 'directory'
    actualSha256 = sha256Tree(source)
    pwshRoot = findPwshRoot(source)
    if (!pwshRoot) throw new Error(`ANALYTIX_PRIVATE_PWSH_SOURCE directory does not contain pwsh.exe: ${source}`)
  } else {
    sourceKind = 'zip'
    actualSha256 = sha256File(source)
    pwshRoot = extractZip(source)
  }
  if (expectedSha256 && expectedSha256 !== actualSha256.toLowerCase()) {
    throw new Error(`Private pwsh SHA256 mismatch. expected=${expectedSha256} actual=${actualSha256}`)
  }
  copyPwshRoot(pwshRoot)
  const manifest = {
    schemaVersion: 1,
    sourceKind,
    source: sourceKind === 'zip' ? path.basename(source) : source,
    sha256: actualSha256,
    expectedSha256: expectedSha256 || '',
    executable: 'pwsh.exe',
    generatedAt: new Date().toISOString()
  }
  fs.writeFileSync(
    path.join(outputRoot, 'analytix-private-pwsh-manifest.json'),
    `${JSON.stringify(manifest, null, 2)}\n`,
    'utf8'
  )
  console.log(`[private-pwsh] staged source=${sourceKind} sha256=${actualSha256} output=${outputRoot}`)
}

main()
