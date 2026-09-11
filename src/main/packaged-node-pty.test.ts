import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'

// eslint-disable-next-line @typescript-eslint/no-require-imports
const afterPack = require('../../scripts/after-pack.cjs')

const temporaryRoots: string[] = []

afterEach(() => {
  for (const root of temporaryRoots.splice(0)) {
    rmSync(root, { recursive: true, force: true })
  }
})

function fixture() {
  const root = mkdtempSync(join(tmpdir(), 'analytix-node-pty-package-'))
  temporaryRoots.push(root)
  const context = {
    appOutDir: join(root, 'mac-arm64'),
    electronPlatformName: 'darwin',
    arch: 'arm64',
    packager: { appInfo: { productFilename: 'analytix' } }
  }
  const source = join(
    afterPack._internals.unpackedAppRoot(context),
    'node_modules',
    'node-pty'
  )
  const prebuild = join(source, 'prebuilds', 'darwin-arm64')
  mkdirSync(join(source, 'lib'), { recursive: true })
  mkdirSync(join(source, 'node-addon-api'), { recursive: true })
  mkdirSync(prebuild, { recursive: true })
  mkdirSync(join(
    context.appOutDir,
    'analytix.app',
    'Contents',
    'MacOS'
  ), { recursive: true })
  writeFileSync(join(source, 'LICENSE'), 'node-pty license\n')
  writeFileSync(join(source, 'package.json'), '{"name":"node-pty"}\n')
  for (const name of [
    'eventEmitter2.js',
    'index.js',
    'terminal.js',
    'unixTerminal.js',
    'utils.js'
  ]) {
    writeFileSync(join(source, 'lib', name), 'module.exports = {}\n')
  }
  mkdirSync(join(source, 'src'))
  writeFileSync(join(source, 'node-addon-api', 'node_addon_api.Makefile'), 'build source')
  writeFileSync(join(source, 'src', 'unneeded.cc'), 'build source')
  writeFileSync(join(source, 'lib', 'terminal.test.js'), 'test source')
  writeFileSync(join(prebuild, 'pty.node'), 'native-module')
  writeFileSync(join(prebuild, 'spawn-helper'), 'helper')
  chmodSync(join(prebuild, 'spawn-helper'), 0o644)
  return { context, source }
}

describe('packaged Darwin node-pty relocation', () => {
  it('moves the exact target module into Contents/MacOS and fixes helper execution', () => {
    const { context, source } = fixture()
    const result = afterPack._internals.relocatePackagedDarwinNodePty(context)
    const destination = afterPack._internals.packagedDarwinNodePtyRoot(context)

    expect(result).toEqual(expect.objectContaining({
      root: destination,
      prebuildFolder: 'darwin-arm64'
    }))
    expect(existsSync(source)).toBe(false)
    expect(existsSync(join(destination, 'lib', 'index.js'))).toBe(true)
    expect(existsSync(join(destination, 'src'))).toBe(false)
    expect(existsSync(join(destination, 'node-addon-api'))).toBe(false)
    expect(existsSync(join(destination, 'lib', 'terminal.test.js'))).toBe(false)
    expect(existsSync(join(destination, 'node_modules'))).toBe(false)
    expect(result.helper).toBe(join(
      destination,
      'prebuilds',
      'darwin-arm64',
      'spawn-helper'
    ))
  })

  it('keeps generic stable reads ctime-sensitive and rejects relocated identity substitution', () => {
    const identity = {
      dev: 1,
      ino: 2,
      mode: 0o40755,
      uid: 501,
      gid: 20,
      size: 160,
      mtimeMs: 3,
      ctimeMs: 4,
      birthtimeMs: 5,
      nlink: 1
    }

    expect(afterPack._internals.sameFileIdentity(identity, { ...identity, ctimeMs: 6 })).toBe(false)
    expect(afterPack._internals.sameRelocatedDirectoryIdentity(identity, {
      ...identity,
      ctimeMs: 6
    })).toBe(true)
    for (const [field, value] of [
      ['dev', 9],
      ['ino', 9],
      ['mode', 0o40700],
      ['uid', 0],
      ['gid', 0],
      ['size', 161],
      ['mtimeMs', 9],
      ['birthtimeMs', 9],
      ['nlink', 2]
    ] as const) {
      expect(afterPack._internals.sameRelocatedDirectoryIdentity(identity, {
        ...identity,
        [field]: value,
        ctimeMs: 6
      })).toBe(false)
    }
  })

  it('rejects a symlinked target payload before relocation', () => {
    const { context, source } = fixture()
    const externalRoot = mkdtempSync(join(tmpdir(), 'analytix-node-pty-payload-external-'))
    temporaryRoots.push(externalRoot)
    const externalPtyNode = join(externalRoot, 'pty.node')
    writeFileSync(externalPtyNode, 'external-native-module')
    rmSync(join(source, 'prebuilds', 'darwin-arm64', 'pty.node'))
    symlinkSync(externalPtyNode, join(source, 'prebuilds', 'darwin-arm64', 'pty.node'))

    expect(() => afterPack._internals.relocatePackagedDarwinNodePty(context)).toThrow(
      /invalid generation link/
    )
    expect(existsSync(source)).toBe(true)
    expect(existsSync(afterPack._internals.packagedDarwinNodePtyRoot(context))).toBe(false)
    expect(readFileSync(externalPtyNode, 'utf8')).toBe('external-native-module')
  })

  it('fails closed when the signed-bundle destination is already occupied', () => {
    const { context } = fixture()
    mkdirSync(afterPack._internals.packagedDarwinNodePtyRoot(context))

    expect(() => afterPack._internals.relocatePackagedDarwinNodePty(context)).toThrow(
      /destination already exists/
    )
  })

  it('rejects an incomplete Darwin runtime closure', () => {
    const { context, source } = fixture()
    rmSync(join(source, 'lib', 'utils.js'))

    expect(() => afterPack._internals.relocatePackagedDarwinNodePty(context)).toThrow(
      /inventory/
    )
  })

  it('fails before pruning through a symlinked node-pty root', () => {
    const { context, source } = fixture()
    const externalRoot = mkdtempSync(join(tmpdir(), 'analytix-node-pty-external-'))
    temporaryRoots.push(externalRoot)
    const sentinel = join(externalRoot, 'build', 'sentinel.txt')
    rmSync(source, { recursive: true })
    mkdirSync(join(externalRoot, 'build'), { recursive: true })
    writeFileSync(sentinel, 'must survive')
    symlinkSync(externalRoot, source, 'dir')

    expect(() => afterPack._internals.prunePackedProductionArtifacts(context)).toThrow(
      /node-pty root directory is not canonical/
    )
    expect(existsSync(sentinel)).toBe(true)
  })

  it('fails before pruning through a symlinked runtime library directory', () => {
    const { context, source } = fixture()
    const externalLib = mkdtempSync(join(tmpdir(), 'analytix-node-pty-lib-external-'))
    temporaryRoots.push(externalLib)
    const sentinel = join(externalLib, 'terminal.test.js')
    writeFileSync(sentinel, 'must survive')
    rmSync(join(source, 'lib'), { recursive: true })
    symlinkSync(externalLib, join(source, 'lib'), 'dir')

    expect(() => afterPack._internals.relocatePackagedDarwinNodePty(context)).toThrow(
      /runtime library directory is not canonical/
    )
    expect(existsSync(sentinel)).toBe(true)
  })

  it('does not relocate node-pty for non-Darwin packages', () => {
    const { context, source } = fixture()
    expect(afterPack._internals.relocatePackagedDarwinNodePty({
      ...context,
      electronPlatformName: 'win32'
    })).toBeNull()
    expect(existsSync(source)).toBe(true)
  })
})
