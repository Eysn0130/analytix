import { mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import { afterEach, describe, expect, it } from 'vitest'
import {
  DATA_ANALYSIS_SUBPROCESS_BASE_ENV_KEYS,
  DATA_NATIVE_ENV_KEYS,
  PACKAGED_DATA_ANALYSIS_ENV_KEYS,
  canonicalWindowsSystemExecutablePath,
  canonicalWindowsSystemRoot,
  nativeBinaryFileName,
  nativeTargetKey,
  resolveExplicitNativeBinary,
  resolvePackagedPythonExecutable,
  resolvePackagedPythonSitePackages,
  resolveRuntimeNativeBinary,
  resolveWindowsSystemExecutable,
  runtimeNativeBinaryPath,
  sanitizePackagedNativeEnvironment
} from './native-runtime-paths'

const roots: string[] = []

function temporaryRoot(): string {
  const root = mkdtempSync(join(tmpdir(), 'analytix-native-paths-'))
  roots.push(root)
  return root
}

function writeExecutable(path: string): void {
  mkdirSync(join(path, '..'), { recursive: true })
  writeFileSync(path, 'native', { mode: 0o755 })
}

afterEach(() => {
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true })
})

describe('data native runtime paths', () => {
  it('normalizes only the four supported package targets', () => {
    expect(nativeTargetKey('darwin', 'aarch64')).toBe('darwin-arm64')
    expect(nativeTargetKey('darwin', 'x86_64')).toBe('darwin-x64')
    expect(nativeTargetKey('windows', 'AMD64')).toBe('win32-x64')
    expect(nativeTargetKey('linux', 'x64')).toBe('linux-x64')
    expect(nativeTargetKey('linux', 'arm64')).toBeUndefined()
    expect(nativeTargetKey('freebsd', 'x64')).toBeUndefined()
    expect(nativeBinaryFileName('analytix-data-engine', 'win32')).toBe('analytix-data-engine.exe')
  })

  it('derives Windows system executables from the canonical system root and never from PATH', () => {
    expect(canonicalWindowsSystemExecutablePath('C:\\Windows', 'taskkill.exe')).toBe(
      'C:\\Windows\\System32\\taskkill.exe'
    )
    expect(canonicalWindowsSystemExecutablePath('d:/windows/', 'taskkill.exe')).toBe(
      'd:\\Windows\\System32\\taskkill.exe'
    )
    expect(canonicalWindowsSystemRoot('d:/windows/')).toBe('d:\\Windows')
    expect(canonicalWindowsSystemExecutablePath('C:\\Windows', '..\\taskkill.exe')).toBeUndefined()
    expect(canonicalWindowsSystemExecutablePath('C:\\attacker', 'taskkill.exe')).toBeUndefined()
    expect(canonicalWindowsSystemExecutablePath('relative\\Windows', 'taskkill.exe')).toBeUndefined()
    expect(resolveWindowsSystemExecutable({ PATH: 'C:\\attacker' }, 'taskkill.exe')).toBeUndefined()
    expect(resolveWindowsSystemExecutable({ SystemRoot: 'C:\\attacker' }, 'taskkill.exe')).toBeUndefined()
    expect(resolveWindowsSystemExecutable({
      SystemRoot: 'C:\\Windows',
      SYSTEMROOT: 'D:\\Windows'
    }, 'taskkill.exe')).toBeUndefined()
  })

  it('never selects a native helper outside the Go execution authority', () => {
    const root = temporaryRoot()
    writeExecutable(join(
      root,
      'runtime',
      'data-native',
      'darwin-arm64',
      'analytix-data-engine'
    ))
    expect(runtimeNativeBinaryPath({
      root,
      packaged: false,
      platform: 'darwin',
      arch: 'arm64',
      binaryName: 'analytix-data-engine'
    })).toBeUndefined()

    expect(resolveRuntimeNativeBinary({
      root,
      packaged: false,
      platform: 'darwin',
      arch: 'arm64',
      binaryName: 'analytix-data-engine'
    })).toBeUndefined()
  })

  it('rejects packaged helpers even when a regular file or symlink exists', () => {
    const root = temporaryRoot()
    const packaged = join(root, 'runtime', 'analytix-data-engine')
    const external = join(root, 'external-data-engine')
    writeExecutable(external)
    mkdirSync(join(root, 'runtime'), { recursive: true })
    symlinkSync(external, packaged)

    expect(resolveRuntimeNativeBinary({
      root,
      packaged: true,
      platform: 'darwin',
      arch: 'arm64',
      binaryName: 'analytix-data-engine'
    })).toBeUndefined()

    rmSync(packaged)
    writeExecutable(packaged)
    expect(resolveRuntimeNativeBinary({
      root,
      packaged: true,
      platform: 'darwin',
      arch: 'arm64',
      binaryName: 'analytix-data-engine'
    })).toBeUndefined()
  })

  it('allows regular explicit helper paths while child environments remain deny-by-default', () => {
    const root = temporaryRoot()
    const explicit = join(root, 'custom', 'analytix-data-engine')
    writeExecutable(explicit)
    expect(resolveExplicitNativeBinary('custom/analytix-data-engine', root)).toBeUndefined()

    const env: NodeJS.ProcessEnv = {
      PATH: '/usr/bin:/bin',
      TMPDIR: '/tmp/analytix',
      LANG: 'zh_CN.UTF-8',
      LC_ALL: 'C.UTF-8',
      __CF_USER_TEXT_ENCODING: '0x1F5:0x19:0x34',
      SystemRoot: 'C:\\Windows',
      ANALYTIX_DATA_ENGINE_BIN: explicit,
      ANALYTIX_IMPORT_ACCELERATOR_BIN: explicit,
      ANALYTIX_ARCHIVE_EXTRACTOR_BIN: explicit,
      ANALYTIX_DATA_ANALYSIS_BACKEND_DIR: explicit,
      ANALYTIX_DATA_ANALYSIS_PYTHON: explicit,
      ANALYTIX_DISABLE_DIRECT_DUCKDB_HELPERS: '0',
      ANALYTIX_DISABLE_DIRECT_PYTHON_DUCKDB: 'false',
      CONDA_PREFIX: explicit,
      PYTHON: explicit,
      PYTHONHOME: explicit,
      PYTHONPATH: explicit,
      PYTHONUSERBASE: explicit,
      VIRTUAL_ENV: explicit,
      KEEP_ME: 'no',
      OPENAI_API_KEY: 'secret',
      GITHUB_TOKEN: 'secret',
      AWS_SESSION_TOKEN: 'secret',
      NPM_CONFIG_USERCONFIG: '/tmp/host-npmrc',
      HTTPS_PROXY: 'http://credential@proxy.invalid',
      NO_PROXY: 'localhost',
      SSH_AUTH_SOCK: '/tmp/agent.sock',
      DBUS_SESSION_BUS_ADDRESS: 'unix:path=/tmp/dbus',
      SSL_CERT_FILE: '/tmp/host-ca.pem'
    }
    const development = sanitizePackagedNativeEnvironment(env, false, 'darwin')
    const packaged = sanitizePackagedNativeEnvironment(env, true, 'darwin')
    expect(development).toEqual(packaged)
    expect(packaged).toEqual({
      TMPDIR: '/tmp/analytix',
      LANG: 'zh_CN.UTF-8',
      LC_ALL: 'C.UTF-8',
      __CF_USER_TEXT_ENCODING: '0x1F5:0x19:0x34'
    })
    expect(DATA_ANALYSIS_SUBPROCESS_BASE_ENV_KEYS).not.toContain('PATH')
    expect(packaged.PATH).toBeUndefined()
    expect(DATA_NATIVE_ENV_KEYS.length).toBe(4)
    for (const key of PACKAGED_DATA_ANALYSIS_ENV_KEYS) expect(packaged[key]).toBeUndefined()
    for (const key of [
      'KEEP_ME',
      'OPENAI_API_KEY',
      'GITHUB_TOKEN',
      'AWS_SESSION_TOKEN',
      'NPM_CONFIG_USERCONFIG',
      'HTTPS_PROXY',
      'NO_PROXY',
      'SSH_AUTH_SOCK',
      'DBUS_SESSION_BUS_ADDRESS',
      'SSL_CERT_FILE',
      'SystemRoot'
    ]) {
      expect(packaged[key]).toBeUndefined()
    }

    const actualChildEnvironment = sanitizePackagedNativeEnvironment({
      ...process.env,
      OPENAI_API_KEY: 'must-not-survive',
      GITHUB_TOKEN: 'must-not-survive',
      HTTPS_PROXY: 'http://credential@proxy.invalid',
      SSH_AUTH_SOCK: '/tmp/agent.sock'
    }, false)
    const child = spawnSync(process.execPath, [
      '-e',
      `process.stdout.write(JSON.stringify({
        apiKey: process.env.OPENAI_API_KEY ?? null,
        github: process.env.GITHUB_TOKEN ?? null,
        proxy: process.env.HTTPS_PROXY ?? null,
        socket: process.env.SSH_AUTH_SOCK ?? null
      }))`
    ], {
      encoding: 'utf8',
      env: actualChildEnvironment
    })
    expect(child.status).toBe(0)
    expect(JSON.parse(child.stdout)).toEqual({
      apiKey: null,
      github: null,
      proxy: null,
      socket: null
    })

    const lowerCaseSanitized = sanitizePackagedNativeEnvironment({
      path: 'C:\\Windows\\System32',
      systemroot: 'C:\\Windows',
      windir: 'C:\\Windows',
      SystemDrive: 'C:',
      PATHEXT: '.COM;.EXE;.BAT;.CMD',
      ComSpec: 'C:\\Windows\\System32\\cmd.exe',
      pythonhome: 'C:\\evil',
      PythonPath: 'C:\\inject',
      analytix_data_engine_bin: 'C:\\evil.exe',
      keep_me: 'yes'
    }, true, 'win32')
    expect(lowerCaseSanitized.path).toBeUndefined()
    expect(lowerCaseSanitized.SystemRoot).toBe('C:\\Windows')
    expect(lowerCaseSanitized.systemroot).toBeUndefined()
    expect(lowerCaseSanitized.windir).toBeUndefined()
    expect(lowerCaseSanitized.SystemDrive).toBeUndefined()
    expect(lowerCaseSanitized.PATHEXT).toBeUndefined()
    expect(lowerCaseSanitized.ComSpec).toBeUndefined()
    expect(lowerCaseSanitized.pythonhome).toBeUndefined()
    expect(lowerCaseSanitized.PythonPath).toBeUndefined()
    expect(lowerCaseSanitized.analytix_data_engine_bin).toBeUndefined()
    expect(lowerCaseSanitized.keep_me).toBeUndefined()
    expect(() => sanitizePackagedNativeEnvironment({ SystemRoot: 'D:\\attacker' }, true, 'win32')).toThrow(
      /SystemRoot is not canonical/
    )
    expect(() => sanitizePackagedNativeEnvironment({ LANG: 'one', lang: 'two' }, true)).toThrow(
      /Duplicate case-insensitive subprocess environment key/
    )
  })

  it('uses only the bundled regular Windows Python executable in packaged mode', () => {
    const root = temporaryRoot()
    const bundled = join(root, '.python-runtime', 'current', 'python', 'python.exe')
    const external = join(root, 'external-python.exe')
    writeExecutable(external)

    expect(resolvePackagedPythonExecutable({ root, packaged: true, platform: 'win32' })).toBeUndefined()
    writeExecutable(bundled)
    expect(resolvePackagedPythonExecutable({ root, packaged: true, platform: 'win32' })).toBe(bundled)
    expect(resolvePackagedPythonExecutable({ root, packaged: true, platform: 'darwin' })).toBeUndefined()
    expect(resolvePackagedPythonExecutable({ root, packaged: false, platform: 'win32' })).toBeUndefined()

    rmSync(bundled)
    symlinkSync(external, bundled)
    expect(resolvePackagedPythonExecutable({ root, packaged: true, platform: 'win32' })).toBeUndefined()

    const sitePackages = join(root, 'python-site-packages')
    mkdirSync(sitePackages)
    expect(resolvePackagedPythonSitePackages({ root, packaged: true })).toBe(sitePackages)
    expect(resolvePackagedPythonSitePackages({ root, packaged: false })).toBeUndefined()
    rmSync(sitePackages, { recursive: true })
    symlinkSync(join(root, '.python-runtime'), sitePackages)
    expect(resolvePackagedPythonSitePackages({ root, packaged: true })).toBeUndefined()
  })

  it('rejects packaged native and Python paths that traverse a symlinked ancestor', () => {
    const root = temporaryRoot()
    const external = temporaryRoot()
    const externalDataEngine = join(external, 'runtime', 'analytix-data-engine.exe')
    const externalPython = join(external, '.python-runtime', 'current', 'python', 'python.exe')
    writeExecutable(externalDataEngine)
    writeExecutable(externalPython)
    symlinkSync(join(external, 'runtime'), join(root, 'runtime'))
    symlinkSync(join(external, '.python-runtime'), join(root, '.python-runtime'))

    expect(resolveRuntimeNativeBinary({
      root,
      packaged: true,
      platform: 'win32',
      arch: 'x64',
      binaryName: 'analytix-data-engine'
    })).toBeUndefined()
    expect(resolvePackagedPythonExecutable({ root, packaged: true, platform: 'win32' })).toBeUndefined()
  })
})
