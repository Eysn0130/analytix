import { createHash } from 'node:crypto'
import { chmodSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'

// eslint-disable-next-line @typescript-eslint/no-require-imports
const contract = require('../../scripts/go-runtime-build-contract.cjs')

const temporaryRoots: string[] = []

afterEach(() => {
  for (const root of temporaryRoots.splice(0)) {
    rmSync(root, { recursive: true, force: true })
  }
})

describe('Go runtime build authority', () => {
  it('rejects ambient compiler, workspace, network, loader, and executable overrides', () => {
    for (const name of [
      'ANALYTIX_GO_BIN',
      'GOFLAGS',
      'goenv',
      'GOWORK',
      'GOROOT',
      'GOTOOLDIR',
      'GOTOOLCHAIN',
      'GOCACHEPROG',
      'GOPROXY',
      'GOAUTH',
      'CGO_CFLAGS',
      'DYLD_INSERT_LIBRARIES',
      'LD_PRELOAD',
      'CC',
      'CXX',
      'AR'
    ]) {
      expect(() => contract.assertNoAmbientGoOverrides({ [name]: 'attacker-controlled' })).toThrow(
        /Ambient build override is forbidden/
      )
    }
    expect(() => contract.assertNoAmbientGoOverrides({ Path: '/safe', PATH: '/other' })).toThrow(
      /Duplicate environment key is forbidden/
    )
  })

  it('resolves only the exact pinned Go executable and core tool identities', () => {
    const root = temporaryRoot()
    const goRoot = join(root, 'go-root')
    const bin = join(goRoot, 'bin')
    const toolDir = join(goRoot, 'pkg', 'tool', 'darwin_arm64')
    const moduleCache = join(root, 'module-cache')
    mkdirSync(bin, { recursive: true })
    mkdirSync(toolDir, { recursive: true })
    mkdirSync(moduleCache, { recursive: true })
    const goExecutable = writeExecutable(join(bin, 'go'), 'pinned-go')
    const tools = Object.fromEntries(
      ['asm', 'compile', 'link'].map((name) => [name, digestFile(writeExecutable(join(toolDir, name), `pinned-${name}`))])
    )
    const lock = {
      hosts: {
        'darwin-arm64': {
          goVersion: 'go1.26.4',
          goExecutableSha256: digestFile(goExecutable),
          tools
        }
      }
    }
    const execCalls: Array<{ file: string; args: string[]; env: Record<string, string> }> = []
    const resolved = contract.resolvePinnedGoToolchain({
      env: { PATH: '/attacker/path', CSC_KEY_PASSWORD: 'must-not-flow' },
      platform: 'darwin',
      arch: 'arm64',
      lock,
      candidates: [join(root, 'missing-go'), goExecutable],
      authorizedModuleCache: moduleCache,
      execFileSync: (file: string, args: string[], options: { env: Record<string, string> }) => {
        execCalls.push({ file, args, env: options.env })
        if (args[0] === 'version') return 'go version go1.26.4 darwin/arm64\n'
        return `${goRoot}\n${toolDir}\n${moduleCache}\n`
      }
    })
    expect(resolved).toEqual(expect.objectContaining({
      key: 'darwin-arm64',
      executable: realpathSync(goExecutable),
      goRoot: realpathSync(goRoot),
      toolDir: realpathSync(toolDir),
      moduleCache: realpathSync(moduleCache)
    }))
    expect(execCalls).toHaveLength(2)
    for (const call of execCalls) {
      expect(Object.keys(call.env).sort()).toEqual([
        'GOENV', 'GOMODCACHE', 'GOTOOLCHAIN', 'GOWORK', 'HOME', 'LANG', 'LC_ALL', 'PATH'
      ])
      expect(call.env.GOMODCACHE).toBe(realpathSync(moduleCache))
      expect(JSON.stringify(call.env)).not.toContain('must-not-flow')
      expect(call.file).toBe(realpathSync(goExecutable))
    }
  })

  it('uses only an explicit trusted module cache while ambient overrides remain forbidden', () => {
    const root = temporaryRoot()
    const moduleCache = join(root, 'module-cache')
    mkdirSync(moduleCache, { recursive: true })

    expect(contract.trustedModuleCachePath(moduleCache)).toBe(realpathSync(moduleCache))
    expect(() => contract.trustedModuleCachePath('relative/module-cache')).toThrow(/path is invalid/)
    expect(() => contract.assertNoAmbientGoOverrides({
      GOMODCACHE: moduleCache
    })).toThrow(/Ambient build override is forbidden/)

    const link = join(root, 'module-cache-link')
    symlinkSync(moduleCache, link)
    expect(() => contract.trustedModuleCachePath(link)).toThrow(/cache is untrusted/)

    chmodSync(moduleCache, 0o777)
    expect(() => contract.trustedModuleCachePath(moduleCache)).toThrow(/cache is untrusted/)
  })

  it('constructs an exact credential-free offline build environment', () => {
    const environment = contract.hermeticGoBuildEnvironment(
      { executable: '/trusted/go/bin/go', moduleCache: '/trusted/module-cache' },
      { goos: 'darwin', goarch: 'arm64' },
      { cache: '/isolated/cache', path: '/isolated/path', temp: '/isolated/temp' }
    )
    expect(environment).toEqual({
      PATH: '/trusted/go/bin:/usr/bin:/bin',
      HOME: '/isolated/path',
      LANG: 'C',
      LC_ALL: 'C',
      GOENV: 'off',
      GOWORK: 'off',
      GOTOOLCHAIN: 'local',
      GOOS: 'darwin',
      GOARCH: 'arm64',
      CGO_ENABLED: '0',
      GOCACHE: '/isolated/cache',
      GOMODCACHE: '/trusted/module-cache',
      GOPATH: '/isolated/path',
      GOTMPDIR: '/isolated/temp',
      GOPROXY: 'off',
      GOSUMDB: 'sum.golang.org',
      GONOSUMDB: '',
      GOPRIVATE: '',
      GONOPROXY: '',
      GOINSECURE: '',
      GOVCS: 'public:git|hg,private:off',
      TZ: 'UTC'
    })
    for (const forbidden of ['CSC_', 'APPLE_', 'ANALYTIX_', 'API_KEY', 'TOKEN', 'PASSWORD', 'DYLD_', 'LD_PRELOAD']) {
      expect(JSON.stringify(environment)).not.toContain(forbidden)
    }
  })
})

function temporaryRoot(): string {
  const root = mkdtempSync(join(tmpdir(), 'analytix-go-runtime-build-'))
  temporaryRoots.push(root)
  return root
}

function writeExecutable(path: string, value: string): string {
  writeFileSync(path, value, 'utf8')
  chmodSync(path, 0o755)
  return path
}

function digestFile(path: string): string {
  // Test fixtures are tiny and deliberately local.
  return createHash('sha256').update(readFileSync(path)).digest('hex')
}
