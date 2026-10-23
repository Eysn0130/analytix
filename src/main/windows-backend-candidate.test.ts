import { cpSync, existsSync, linkSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { spawnSync } from 'node:child_process'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { runInThisContext } from 'node:vm'
import { afterEach, describe, expect, it, vi } from 'vitest'

const require = createRequire(import.meta.url)
const pythonRuntimeBuild = require('../../scripts/build-windows-backend-runtime-assets.cjs')
const pythonRuntimeContract = require('../../scripts/windows-python-runtime-contract.cjs')

// Load the unchanged CommonJS implementation with file-local dependencies.
// No global require cache or production environment switch is modified.
function fixtureCommonJS(relativePath: string, dependencies: Record<string, unknown>) {
  const filename = resolve(__dirname, '../..', relativePath)
  const localRequire = createRequire(filename)
  const module = { exports: {} as any }
  const execute = runInThisContext(`(function(exports, require, module, __filename, __dirname) {\n${readFileSync(filename, 'utf8')}\n})`, { filename })
  execute(module.exports, (id: string) => Object.hasOwn(dependencies, id) ? dependencies[id] : localRequire(id),
    module, filename, dirname(filename))
  return module.exports
}

const tempRoots: string[] = []

function tempRoot(): string {
  const root = mkdtempSync(join(tmpdir(), 'analytix-windows-candidate-'))
  tempRoots.push(root)
  return root
}

afterEach(() => {
  while (tempRoots.length > 0) {
    const root = tempRoots.pop()
    if (root) rmSync(root, { recursive: true, force: true })
  }
})

describe('Windows backend frozen candidate', () => {
  it('keeps frozen candidate AnyIO synchronization and permits only an explicit Pillow pin', () => {
    const accepted = readFileSync(pythonRuntimeContract.REQUIREMENTS_LOCK_PATH, 'utf8')
    const backend = readFileSync(join(process.cwd(), 'backend/uv.lock'), 'utf8')
    const hash = createHash('sha256').update('synthetic candidate wheel').digest('hex')
    const stale = accepted.replace(/^anyio==[^\n]+/mu, `anyio==0.0.0 --hash=sha256:${hash}`)
    expect(pythonRuntimeBuild.candidateRequirementsText(stale, backend)).toBe(accepted)
    const explicit = accepted.replace(/^pillow==[^\n]+/mu, `pillow==99.0.0 --hash=sha256:${hash}`)
    expect(pythonRuntimeBuild.candidateRequirementsText(stale, backend, explicit)).toBe(explicit)
    expect(pythonRuntimeBuild.candidateRequirementsText(accepted, backend, accepted)).toBe(accepted)
  })

  it('rejects frozen candidate dependency drift and requirements syntax extensions', () => {
    const accepted = readFileSync(pythonRuntimeContract.REQUIREMENTS_LOCK_PATH, 'utf8')
    const backend = readFileSync(join(process.cwd(), 'backend/uv.lock'), 'utf8')
    const hash = createHash('sha256').update('synthetic rejected pin').digest('hex')
    const pillow = `pillow==99.0.0 --hash=sha256:${hash}`
    const replacePillow = (line: string) => accepted.replace(/^pillow==[^\n]+/mu, line)
    const invalid: Record<string, string> = {
      'another dependency': accepted.replace(/^certifi==[^\n]+/mu, `certifi==0.0.0 --hash=sha256:${hash}`),
      'deleted dependency': accepted.replace(/^certifi==[^\n]+\n/mu, ''),
      'duplicate Pillow': `${accepted}${pillow}\n`,
      'duplicate AnyIO': `${accepted}${pythonRuntimeContract.backendAnyioRequirement(backend)}\n`,
      'reordered dependencies': accepted.split('\n').reverse().join('\n'),
      'CRLF conversion': accepted.replaceAll('\n', '\r\n'),
      'missing final LF': accepted.trimEnd(),
      'extra blank line': `${accepted}\n`,
      'wrong AnyIO': accepted.replace(/^anyio==[^\n]+/mu, `anyio==0.0.0 --hash=sha256:${hash}`),
      'direct URL': replacePillow(`pillow @ https://untrusted.invalid/pillow.whl --hash=sha256:${hash}`),
      'version range': replacePillow(`pillow>=99.0.0 --hash=sha256:${hash}`),
      'missing hash': replacePillow('pillow==99.0.0'),
      'invalid hash': replacePillow('pillow==99.0.0 --hash=sha256:invalid'),
      'extra hash': replacePillow(`${pillow} --hash=sha256:${hash}`),
      'environment marker': replacePillow(`${pillow}; python_version >= "3.11"`),
      'pip option': replacePillow(`${pillow} --index-url https://untrusted.invalid`),
      'line continuation': replacePillow(`${pillow} \\` + `\n  --hash=sha256:${hash}`)
    }
    for (const [label, text] of Object.entries(invalid)) {
      expect(() => pythonRuntimeBuild.candidateRequirementsText(accepted, backend, text), label).toThrow()
    }
  })

  it('reads frozen candidate requirements only from an isolated regular temporary file', () => {
    const input = join(tempRoot(), 'requirements.txt')
    writeFileSync(input, 'synthetic input\n')
    expect(pythonRuntimeBuild.readCandidateRequirements(input)).toBe('synthetic input\n')
    const source = tempRoot()
    const sourceInput = join(source, 'requirements.txt')
    writeFileSync(sourceInput, 'source input\n')
    expect(() => pythonRuntimeBuild.readCandidateRequirements(sourceInput, source)).toThrow()
    expect(() => pythonRuntimeBuild.readCandidateRequirements(pythonRuntimeContract.REQUIREMENTS_LOCK_PATH, source)).toThrow()
    expect(() => pythonRuntimeBuild.readCandidateRequirements(tempRoot())).toThrow()
    const link = join(tempRoot(), 'requirements.txt')
    symlinkSync(input, link)
    expect(() => pythonRuntimeBuild.readCandidateRequirements(link)).toThrow()
    const ancestor = join(tempRoot(), 'linked-parent')
    symlinkSync(dirname(input), ancestor, 'dir')
    expect(() => pythonRuntimeBuild.readCandidateRequirements(join(ancestor, 'requirements.txt'))).toThrow()
    const hardLink = join(tempRoot(), 'requirements.txt')
    linkSync(input, hardLink)
    expect(() => pythonRuntimeBuild.readCandidateRequirements(hardLink)).toThrow()
  })

  it('rejects frozen candidate arguments and invalid input before writing or downloading', () => {
    const output = tempRoot()
    const input = join(tempRoot(), 'requirements.txt')
    const accepted = readFileSync(pythonRuntimeContract.REQUIREMENTS_LOCK_PATH, 'utf8')
    writeFileSync(input, accepted.replace(/^certifi==[^\n]+\n/mu, ''))
    const execute = vi.fn(() => { throw new Error('invalid input reached a subprocess') })
    const build = fixtureCommonJS('scripts/build-windows-backend-runtime-assets.cjs', {
      'node:child_process': { spawnSync: execute }
    })
    expect(() => build.authorFrozenCandidate(output, input)).toThrow()
    expect(execute).not.toHaveBeenCalled()
    expect(readdirSync(output)).toEqual([])
    for (const args of [
      ['--candidate-requirements', input],
      ['--freeze-candidate', output, '--unknown', input],
      ['--freeze-candidate', output, '--candidate-requirements'],
      ['--candidate-requirements', input, '--freeze-candidate', output]
    ]) {
      const result = spawnSync(process.execPath, [join(process.cwd(), 'scripts/build-windows-backend-runtime-assets.cjs'), ...args], { encoding: 'utf8' })
      expect(result.status, args.join(' ')).toBe(1)
      expect(result.stderr).toContain('Usage:')
      expect(readdirSync(output)).toEqual([])
    }
  })

  it('verifies a generated frozen candidate with unchanged production guards and rejects tampering', async () => {
    const acceptedPath = pythonRuntimeContract.REQUIREMENTS_LOCK_PATH
    const lockPath = pythonRuntimeContract.LOCK_PATH
    const accepted = readFileSync(acceptedPath, 'utf8')
    const acceptedLock = readFileSync(lockPath)
    const productionStage = join(process.cwd(), 'build/windows-backend-runtime')
    const productionStageExisted = existsSync(productionStage)
    const productionIdentity = productionStageExisted ? pythonRuntimeContract.rawTreeIdentity(productionStage) : null
    const JSZip = require('jszip')
    // Only pip's network boundary is synthetic. Extraction, pruning, identities,
    // manifests and the separately loaded production validators execute normally.
    const wheels: { fileName: string; bytes: Buffer }[] = []
    for (const [index, line] of accepted.trimEnd().split('\n').entries()) {
      const [name, pin] = line.split('==')
      const version = name === 'pillow' ? '99.0.0' : pin.split(' ')[0]
      const archive = new JSZip()
      archive.file(`synthetic_${index}/__init__.py`, `value = ${index}\n`)
      archive.file(`synthetic_${index}/tests/build_only.py`, 'removed = True\n')
      archive.file(`${name}-${version}.dist-info/METADATA`, `Name: ${name}\nVersion: ${version}\n`)
      archive.file(`${name}-${version}.dist-info/licenses/LICENSE`, 'synthetic license\n')
      wheels.push({ fileName: `${name.replaceAll('-', '_')}-${version}-py3-none-any.whl`, bytes: await archive.generateAsync({ type: 'nodebuffer' }) })
    }
    const pillow = wheels.find((wheel) => wheel.fileName.startsWith('pillow-'))!
    const requirements = accepted.replace(/^pillow==[^\n]+/mu,
      `pillow==99.0.0 --hash=sha256:${createHash('sha256').update(pillow.bytes).digest('hex')}`)
    const input = join(tempRoot(), 'requirements.txt')
    writeFileSync(input, requirements)
    let downloads = 0
    let extractions = 0
    const build = fixtureCommonJS('scripts/build-windows-backend-runtime-assets.cjs', {
      'node:child_process': { spawnSync: (command: string, args: string[], options: any) => {
        if (args.includes('pip')) {
          downloads++
          expect(args).toEqual(expect.arrayContaining(['--isolated', '--index-url', 'https://pypi.org/simple', '--no-cache-dir', '--only-binary=:all:', '--require-hashes', '--platform', 'win_amd64', '--python-version', '311', '--implementation', 'cp', '--abi', 'cp311']))
          expect(readFileSync(args[args.indexOf('--requirement') + 1], 'utf8')).toBe(requirements)
          const destination = args[args.indexOf('--dest') + 1]
          for (const wheel of wheels) writeFileSync(join(destination, wheel.fileName), wheel.bytes)
          return { status: 0, stdout: '', stderr: '' }
        }
        if (!args.includes('-c') || !args[args.indexOf('-c') + 1].includes('zipfile')) throw new Error('unexpected candidate subprocess')
        extractions++
        return spawnSync(command, args, options)
      } }
    })
    const output = tempRoot()
    const candidate = build.authorFrozenCandidate(output, input)
    expect(downloads).toBe(1)
    expect(extractions).toBe(1)
    expect(candidate.pythonRuntime).toEqual(pythonRuntimeContract.lock.pythonRuntime)
    expect(readFileSync(acceptedPath, 'utf8')).toBe(accepted)
    expect(readFileSync(lockPath)).toEqual(acceptedLock)
    expect(existsSync(productionStage)).toBe(productionStageExisted)
    if (productionStageExisted) expect(pythonRuntimeContract.rawTreeIdentity(productionStage)).toEqual(productionIdentity)
    const site = join(output, 'python-site-packages')
    expect(existsSync(join(site, 'synthetic_0/tests'))).toBe(false)
    expect(readFileSync(join(site, 'annotated-doc-0.0.4.dist-info/licenses/LICENSE'), 'utf8')).toBe('synthetic license\n')
    const verifierScripts = join(tempRoot(), 'scripts')
    cpSync(join(process.cwd(), 'scripts'), verifierScripts, { recursive: true })
    for (const name of ['windows-backend-requirements.lock.txt', 'windows-python-runtime-lock.json']) cpSync(join(output, name), join(verifierScripts, name))
    const contract = join(verifierScripts, 'windows-python-runtime-contract.cjs')
    expect(readFileSync(contract)).toEqual(readFileSync(join(process.cwd(), 'scripts/windows-python-runtime-contract.cjs')))
    const verify = () => spawnSync(process.execPath, ['-e',
      'const c = require(process.argv[1]); c.verifyWheelhouse(process.argv[2]); c.verifySitePackages(process.argv[3])',
      contract, join(output, 'backend-wheelhouse'), site], { encoding: 'utf8' })
    const clean = verify()
    expect(clean.status, clean.stderr).toBe(0)
    const generatedRequirements = join(verifierScripts, 'windows-backend-requirements.lock.txt')
    writeFileSync(generatedRequirements, `${requirements}\n`)
    expect(verify().stderr).toContain('Frozen requirements or inventory authority is stale')
    writeFileSync(generatedRequirements, requirements)
    const wheelPath = join(output, 'backend-wheelhouse', wheels[0].fileName)
    writeFileSync(wheelPath, 'tampered wheel')
    expect(verify().stderr).toContain('Wheelhouse does not match the frozen hash lock')
    writeFileSync(wheelPath, wheels[0].bytes)
    writeFileSync(join(site, 'synthetic_0/__init__.py'), 'tampered = True\n')
    expect(verify().stderr).toContain('Site-packages authority is stale or mismatched')
    writeFileSync(join(site, 'synthetic_0/__init__.py'), 'value = 0\n')
    const manifestPath = join(site, pythonRuntimeContract.SITE_PACKAGES_MANIFEST_NAME)
    const originalManifest = readFileSync(manifestPath, 'utf8')
    const manifest = JSON.parse(originalManifest)
    manifest.lockSha256 = createHash('sha256').update('tampered candidate manifest authority').digest('hex')
    writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`)
    const rejectedManifest = verify()
    expect(rejectedManifest.status).not.toBe(0)
    expect(rejectedManifest.stderr).toContain('Site-packages authority is stale or mismatched')
    writeFileSync(manifestPath, originalManifest)
    const restored = verify()
    expect(restored.status, restored.stderr).toBe(0)
  })

})
