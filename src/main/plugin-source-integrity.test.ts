import { mkdtempSync, mkdirSync, realpathSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { computePluginSourceTreeIdentity } from './plugin-source-integrity'

const roots: string[] = []

afterEach(() => {
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true })
})

function fixtureRoot(): string {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'analytix-plugin-tree-')))
  roots.push(root)
  mkdirSync(join(root, 'nested'))
  writeFileSync(join(root, 'a.mjs'), 'export const a = 1\n')
  writeFileSync(join(root, 'nested', 'b.json'), '{"b":2}\n')
  return root
}

describe('plugin source integrity', () => {
  it('matches the portable cross-runtime tree digest', () => {
    const identity = computePluginSourceTreeIdentity(fixtureRoot())
    expect(identity.fileCount).toBe(2)
    expect(identity.treeSha256).toBe('b61f6361ff562bcc58ec5e40f258cb462857d2ae423b4639936df377e1e4ddf3')
  })

  it('changes when an imported source file changes', () => {
    const root = fixtureRoot()
    const before = computePluginSourceTreeIdentity(root)
    writeFileSync(join(root, 'nested', 'b.json'), '{"b":3}\n')
    expect(computePluginSourceTreeIdentity(root).treeSha256).not.toBe(before.treeSha256)
  })

  it('rejects symbolic links anywhere in the source tree', () => {
    const root = fixtureRoot()
    symlinkSync(join(root, 'a.mjs'), join(root, 'alias.mjs'))
    expect(() => computePluginSourceTreeIdentity(root)).toThrow(/symbolic link/u)
  })

  it('rejects a symbolic-link root and unsafe exclusion paths', () => {
    const root = fixtureRoot()
    const alias = `${root}-alias`
    roots.push(alias)
    symlinkSync(root, alias)
    expect(() => computePluginSourceTreeIdentity(alias)).toThrow(/root must not contain a symbolic link/u)
    expect(() => computePluginSourceTreeIdentity(root, { excludeRelativePaths: ['../a.mjs'] }))
      .toThrow(/exclusion path is invalid/u)
  })

  it('still rejects excluded symbolic links and directories', () => {
    const root = fixtureRoot()
    symlinkSync(join(root, 'a.mjs'), join(root, '.marker.json'))
    expect(() => computePluginSourceTreeIdentity(root, { excludeRelativePaths: ['.marker.json'] }))
      .toThrow(/symbolic link/u)
    rmSync(join(root, '.marker.json'))
    mkdirSync(join(root, '.marker.json'))
    expect(() => computePluginSourceTreeIdentity(root, { excludeRelativePaths: ['.marker.json'] }))
      .toThrow(/not a regular file/u)
  })
})
