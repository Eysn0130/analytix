import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

describe('K10 task-owned explicit Darwin Keychain harness', () => {
  it('keeps the password off argv/env/files and never mutates default/search-list state', async () => {
    const source = await readFile(
      join(process.cwd(), 'scripts/k10-darwin-explicit-keychain.mjs'),
      'utf8'
    )
    expect(source).toContain("operation !== 'create-keychain' && operation !== 'unlock-keychain'")
    expect(source).toContain("stdio: ['pipe', 'ignore', 'ignore']")
    expect(source).toContain('child.stdin.write(password)')
    expect(source).toContain('requestedPath, databasePath: requestedPath')
    expect(source).toContain(
      "runTaskKeychainOperation('unlock-keychain', paths.databasePath, isolationRoot, password)"
    )
    expect(source).toContain("const KEYCHAIN_REQUEST_NAME = 'analytix-task.keychain-db'")
    expect(source).not.toContain('`${requestedPath}-db`')
    expect(source).not.toContain('default-keychain')
    expect(source).not.toContain('list-keychains')
    expect(source).not.toContain('login-keychain')
    expect(source).not.toMatch(/\bHOME\s*:/u)
    expect(source).not.toMatch(/\bUSERPROFILE\s*:/u)
    expect(source).not.toContain("'-p'")
    expect(source).not.toContain("'-s'")
  })
})
