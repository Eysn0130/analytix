import { mkdtemp, mkdir, writeFile, rm, symlink, link } from 'node:fs/promises'
import { writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import {
  captureLocalCredentialEntry,
  coordinateLocalCredentialEntry,
  disposeLocalCredentialScanSource,
  readLocalCredentialScanSource,
  scanLocalCredentialIsolation
// @ts-expect-error The production harness module is JavaScript.
} from '../../scripts/lib/local-provider-credential-scan.mjs'

const providerRegistry = {
  schemaVersion: 1, registryRevision: '2', registryIncarnation: 'inc_' + 'a'.repeat(43),
  selectedProviderId: 'synthetic-provider', providers: [{
    id: 'synthetic-provider', kind: 'deepseek', endpoint: 'https://api.example.invalid/v1',
    models: ['deepseek-v4-flash'], selectedModel: 'deepseek-v4-flash', mediaModels: [], selectedRoutes: [],
    credentialConfigured: true, credentialPurpose: 'provider-api-key', tombstone: false,
    revision: '1', generation: '1', incarnation: 'inc_' + 'b'.repeat(43)
  }]
}
const context = { runId: 'synthetic-run', entryAttemptId: 'synthetic-attempt', provider: {
  ok: true, id: 'synthetic-provider', model: 'deepseek-v4-flash',
  baseUrl: 'https://api.example.invalid/v1', endpointFormat: 'chat_completions',
  registryRevision: '2', registryIncarnation: providerRegistry.registryIncarnation,
  providerRevision: '1', providerGeneration: '1', providerIncarnation: providerRegistry.providers[0].incarnation
} }
const secret = 'synthetic-key-+/%"-中-end'
const roots: string[] = []
const sources: object[] = []
afterEach(async () => {
  for (const source of sources.splice(0)) disposeLocalCredentialScanSource(source)
  for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true })
})

async function capture(entryMethod = 'visible-computer-use', callback: (bytes: Buffer) => Promise<object> = async () => ({ completed: true, providerRegistry })) {
  const input = Buffer.from(secret)
  try {
    const source = await captureLocalCredentialEntry({ ...context, providerId: context.provider.id, entryMethod, credential: input }, callback)
    sources.push(source)
    return source
  } finally { input.fill(0) }
}

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), 'analytix-credential-scan-'))
  roots.push(root)
  const settingsPath = path.join(root, 'settings.json')
  await writeFile(settingsPath, '{"provider":{}}')
  return { root, settingsPath, reportSnapshot: { status: 'synthetic' }, ...context, source: await capture() }
}

describe('private run-bound credential scan source', () => {
  it('binds the bytes used by completed visible entry without serializing secrets or granting provider authority', async () => {
    let entered: Buffer | undefined
    const source = await capture('visible-computer-use', async (bytes) => {
      expect(bytes.toString()).toBe(secret)
      entered = bytes
      return { completed: true, providerRegistry }
    })
    expect(entered?.every((byte) => byte === 0)).toBe(true)
    expect(JSON.stringify(source)).toBe('{}')
    expect(Reflect.ownKeys(source)).toEqual([])
    expect(readLocalCredentialScanSource(source, context)).toMatchObject({
      ok: true, sourceBound: true, automatedCredentialEntryUsed: true,
      expectedSecretCount: 1, sourceSecretCount: 1, uniqueSecretCount: 1
    })
    expect(readLocalCredentialScanSource(source, { ...context, provider: { id: context.provider.id, ready: false } }).ok).toBe(false)
    for (const altered of [
      { ...context, runId: 'other-run' }, { ...context, entryAttemptId: 'other-entry' },
      { ...context, provider: { id: 'other-provider' } }, { ...context, provider: {} }
    ]) expect(readLocalCredentialScanSource(source, altered)).toMatchObject({
      ok: false, blocked: true, sourceBound: false, expectedSecretCount: 0, sourceSecretCount: 0, uniqueSecretCount: 0
    })
    expect(readLocalCredentialScanSource(JSON.parse(JSON.stringify(source)), context).ok).toBe(false)
    disposeLocalCredentialScanSource(source)
    expect(readLocalCredentialScanSource(source, context).sourceSecretCount).toBe(0)
  })

  it('records visible-human entry truthfully', async () => {
    expect(readLocalCredentialScanSource(await capture('visible-human'), context)).toMatchObject({ entryMethod: 'visible-human', automatedCredentialEntryUsed: false })
  })

  it('R130-F1 rejects K1 capture against same-id K2 generation before first ready and scan', async () => {
    const options = await fixture()
    const provider = { ...context.provider, registryRevision: '3', providerRevision: '2', providerGeneration: '2' }
    await writeFile(path.join(options.root, 'k2-only.log'), 'synthetic-replacement-K2-only')
    expect(readLocalCredentialScanSource(options.source, { ...context, provider })).toMatchObject({
      ok: false, sourceBound: false, sourceSecretCount: 0
    })
    expect(await scanLocalCredentialIsolation({ ...options, provider })).toMatchObject({
      ok: false, status: 'blocked', sourceBound: false, scannedFileCount: 0
    })
  })

  it.each(['id', 'model', 'baseUrl', 'endpointFormat', 'registryRevision', 'registryIncarnation',
    'providerRevision', 'providerGeneration', 'providerIncarnation'])('R130-F1 rejects changed or missing %s', async (key) => {
    const source = await capture()
    for (const value of [undefined, '', 'changed']) {
      expect(readLocalCredentialScanSource(source, { ...context,
        provider: { ...context.provider, [key]: value } }).sourceBound).toBe(false)
    }
  })

  it('R130-F1 copies the completion readback before later same-id credential replacement', async () => {
    const completion = structuredClone(providerRegistry)
    const source = await capture('visible-human', async () => ({ completed: true, providerRegistry: completion }))
    completion.registryRevision = '3'
    completion.providers[0].revision = '2'
    completion.providers[0].generation = '2'
    expect(readLocalCredentialScanSource(source, context).ok).toBe(true)
    expect(readLocalCredentialScanSource(source, { ...context, provider: {
      ...context.provider, registryRevision: '3', providerRevision: '2', providerGeneration: '2'
    } }).ok).toBe(false)
  })

  it.each([
    undefined, null, {}, { ...providerRegistry, registryRevision: '0' },
    { ...providerRegistry, registryRevision: '01' },
    { ...providerRegistry, registryRevision: '18446744073709551616' },
    { ...providerRegistry, registryIncarnation: '' },
    ...['revision', 'generation', 'incarnation', 'endpoint', 'selectedModel'].flatMap((key) =>
      [undefined, '', 'malformed'].map((value) => ({ ...providerRegistry,
        providers: [{ ...providerRegistry.providers[0], [key]: value }] }))),
    { ...providerRegistry, selectedProviderId: 'other', providers: [{ ...providerRegistry.providers[0], id: 'other' }] },
    { ...providerRegistry, providers: [{ ...providerRegistry.providers[0], credentialConfigured: false }] }
  ])('R130-F1 refuses a handle without a valid matching completion Registry readback %#', async (providerRegistry) => {
    let entered: Buffer | undefined
    await expect(capture('visible-human', async (bytes) => {
      entered = bytes
      return { completed: true, providerRegistry }
    })).rejects.toThrow('credential-entry-authority-invalid')
    expect(entered?.every((byte) => byte === 0)).toBe(true)
  })

  it.each([undefined, Buffer.alloc(0), Buffer.alloc(8193, 65), Buffer.from([0xff]), Buffer.from('  '), Buffer.from('bad\nkey')])('rejects missing, empty, oversized and malformed credential input', async (credential) => {
    let called = false
    await expect(captureLocalCredentialEntry({ ...context, providerId: context.provider.id, entryMethod: 'visible-computer-use', credential }, async () => {
      called = true; return { completed: true }
    })).rejects.toThrow('credential-entry-invalid')
    expect(called).toBe(false)
  })

  it.each(['cancel', 'throw', 'mutate'])('cleans controlled entry memory on %s and emits only closed errors', async (mode) => {
    let entered: Buffer | undefined
    await expect(capture('visible-computer-use', async (bytes) => {
      entered = bytes
      if (mode === 'throw') throw new Error(secret)
      if (mode === 'mutate') bytes[0] = 0
      return { completed: mode !== 'cancel' }
    })).rejects.toThrow(mode === 'mutate' ? 'credential-entry-changed' : 'credential-entry-not-completed')
    expect(entered?.every((byte) => byte === 0)).toBe(true)
  })

  it('blocks invalid and disposed sources without claiming supplied needles', async () => {
    const options = await fixture()
    disposeLocalCredentialScanSource(options.source)
    for (const source of [undefined, {}, options.source]) {
      expect(await scanLocalCredentialIsolation({ ...options, source })).toMatchObject({ status: 'blocked', sourceBound: false, sourceSecretCount: 0, scannedFileCount: 0 })
    }
  })

  it('cancels pending visible entry and disposes a late coordinator result after timeout', async () => {
    const controller = new AbortController()
    let entered: Buffer | undefined
    const input = Buffer.from(secret)
    const pending = captureLocalCredentialEntry({ ...context, providerId: context.provider.id,
      entryMethod: 'visible-computer-use', credential: input, signal: controller.signal }, async (bytes: Buffer) => {
      entered = bytes
      controller.abort()
      return new Promise(() => {})
    })
    await expect(pending).rejects.toThrow('credential-entry-not-completed')
    expect(entered?.every((byte) => byte === 0)).toBe(true)
    input.fill(0)
    const source = await capture()
    let finish: ((source: object) => void) | undefined
    await expect(coordinateLocalCredentialEntry(() => new Promise((resolve) => {
      finish = resolve
    }), context, 5)).rejects.toThrow('credential-entry-timeout')
    finish!(source)
    await new Promise((resolve) => setImmediate(resolve))
    expect(readLocalCredentialScanSource(source, context).sourceBound).toBe(false)
  })
})

describe('streaming leakage scan', () => {
  it('passes clean isolated settings, files, and report', async () => {
    const options = await fixture()
    await mkdir(path.join(options.root, 'nested'))
    await writeFile(path.join(options.root, 'nested', 'safe.log'), 'no credential values')
    expect(await scanLocalCredentialIsolation(options)).toMatchObject({ status: 'passed', ok: true, scannedFileCount: 2, findingCount: 0 })
  })

  const encodings = [
    ['utf8', secret], ['base64', Buffer.from(secret).toString('base64')],
    ['base64url', Buffer.from(secret).toString('base64url')], ['hex', Buffer.from(secret).toString('hex')],
    ['url', encodeURIComponent(secret)], ['json-escape', JSON.stringify(secret).slice(1, -1)],
    ['json-unicode', secret.split('').map((unit) => `\\u${unit.charCodeAt(0).toString(16).padStart(4, '0')}`).join('')]
  ]
  it.each(encodings)('finds %s across the read boundary without outputting value, path, or fingerprint', async (_encoding, encoded) => {
    const options = await fixture()
    const payload = Buffer.concat([Buffer.alloc(65536 - 3, 46), Buffer.from(encoded), Buffer.alloc(300, 46)])
    await writeFile(path.join(options.root, 'synthetic.log'), payload)
    const result = await scanLocalCredentialIsolation(options)
    expect(result).toMatchObject({ status: 'failed', findingCount: 1, scannedFileCount: 2, settingsFindingCount: 0, reportFindingCount: 0 })
    const serialized = JSON.stringify(result)
    expect(serialized).not.toContain(secret)
    expect(serialized).not.toContain(encoded)
    expect(serialized).not.toContain(options.root)
    expect(serialized).not.toMatch(/fingerprint|digest|sha256/iu)
  })

  it('counts settings/report leakage and does not exempt protected-store files', async () => {
    const options = await fixture()
    await writeFile(options.settingsPath, JSON.stringify({ apiKey: secret }))
    await writeFile(path.join(options.root, 'provider-secrets.json'), secret)
    const result = await scanLocalCredentialIsolation({ ...options, reportSnapshot: { unsafe: secret } })
    expect(result).toMatchObject({ status: 'failed', findingCount: 3, settingsFindingCount: 1, reportFindingCount: 1 })
  })

  it('scans every file when a disjoint root has no separately classified settings', async () => {
    const options = await fixture()
    await writeFile(options.settingsPath, secret)
    expect(await scanLocalCredentialIsolation({ ...options, settingsPath: '' })).toMatchObject({ status: 'failed', scannedFileCount: 1, findingCount: 1, settingsFindingCount: 0 })
  })

  it.each(['symlink', 'hardlink'])('fails closed on %s without following it', async (kind) => {
    const options = await fixture()
    if (kind === 'symlink') await symlink(options.settingsPath, path.join(options.root, 'unsafe'))
    else await link(options.settingsPath, path.join(options.root, 'unsafe'))
    expect(await scanLocalCredentialIsolation(options)).toMatchObject({ status: 'blocked', blocked: true, unsafeEntryCount: kind === 'symlink' ? 1 : 2, symlinkCount: kind === 'symlink' ? 1 : 0 })
  })

  it('fails closed for missing settings and unserializable reports', async () => {
    const options = await fixture()
    expect(await scanLocalCredentialIsolation({ ...options, settingsPath: path.join(options.root, 'missing') })).toMatchObject({ status: 'blocked', blocker: 'credential-scan-settings-missing' })
    const circular: Record<string, unknown> = {}; circular.self = circular
    expect(await scanLocalCredentialIsolation({ ...options, reportSnapshot: circular })).toMatchObject({ status: 'blocked', blocker: 'credential-scan-report-invalid' })
  })

  it('fails closed when file identity changes after its read and before completion', async () => {
    const options = await fixture()
    const reportSnapshot = { toJSON() {
      writeFileSync(options.settingsPath, 'changed-size-after-file-scan')
      return { synthetic: true }
    } }
    expect(await scanLocalCredentialIsolation({ ...options, reportSnapshot })).toMatchObject({ status: 'blocked', blocker: 'credential-scan-identity-changed' })
  })

  it('invalidates counts if disposed during scanning', async () => {
    const options = await fixture()
    const reportSnapshot = { toJSON() {
      disposeLocalCredentialScanSource(options.source)
      return { synthetic: true }
    } }
    expect(await scanLocalCredentialIsolation({ ...options, reportSnapshot })).toMatchObject({ status: 'blocked', sourceBound: false, sourceSecretCount: 0, uniqueSecretCount: 0 })
  })
})
