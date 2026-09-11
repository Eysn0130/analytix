import { mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync } from 'node:fs'
import { createHash, generateKeyPairSync, sign } from 'node:crypto'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import {
  bindBundledFundsMaterializationToCurrentRuntimeV1,
  clearBundledFundsMaterializationCurrentRuntimeV1,
  currentBundledFundsMaterializationBindingV1,
  materializeBundledFundsBeforeRuntimeV1,
  verifyBundledFundsMaterializationOutputV1,
  type BundledFundsMaterializationCommandResultV1
} from './bundled-funds-materialization'

const RECEIPT_SIGNATURE_DOMAIN = Buffer.from('analytix.bundled-plugin-materialization-receipt/signature/v1\0')

describe('bundled funds materialization desktop binding', () => {
  afterEach(() => clearBundledFundsMaterializationCurrentRuntimeV1())

  it('accepts only the current invocation and installation-authority signed receipt/index', () => {
    const root = testRuntimeHome()
    try {
      const invocationId = '1'.repeat(64)
      const stdout = readyFixture(invocationId, root)
      const ready = verifyBundledFundsMaterializationOutputV1(stdout, invocationId, join(root, 'data'))
      expect(ready.pluginVersion).toBe('0.16.16')
      expect(ready.factToolsEnabled).toBe(false)
      expect(ready.publishable).toBe(false)
      expect(() => verifyBundledFundsMaterializationOutputV1(stdout, '2'.repeat(64), join(root, 'data')))
        .toThrow(/current-run binding/)

      const forged = JSON.parse(stdout.toString('utf8').slice(
        'ANALYTIX_BUNDLED_FUNDS_MATERIALIZATION_READY_V1 '.length
      ))
      forged.receipt.sourceTreeSha256 = '9'.repeat(64)
      const forgedOutput = Buffer.from(`ANALYTIX_BUNDLED_FUNDS_MATERIALIZATION_READY_V1 ${JSON.stringify(forged)}\n`)
      expect(() => verifyBundledFundsMaterializationOutputV1(forgedOutput, invocationId, join(root, 'data')))
        .toThrow(/current-run binding/)
    } finally {
      rmSync(root, { recursive: true, force: true })
    }
  })

  it('consumes a newly admitted canonical package version from the signed ready binding', () => {
    const root = testRuntimeHome()
    try {
      const invocationId = '1'.repeat(64)
      const ready = verifyBundledFundsMaterializationOutputV1(
        readyFixture(invocationId, root, '0.16.17'),
        invocationId,
        join(root, 'data')
      )
      expect(ready.pluginVersion).toBe('0.16.17')
      expect(ready.receipt.pluginVersion).toBe('0.16.17')
      expect(ready.index.pluginVersion).toBe('0.16.17')
      expect(ready.activePluginRoot).toBe(join(
        root,
        'plugins/cache/analytix-hub/analytix-fund-analysis/0.16.17'
      ))
    } finally {
      rmSync(root, { recursive: true, force: true })
    }
  })

  it('rejects a signed ready binding whose package version is not canonical semver', () => {
    const root = testRuntimeHome()
    try {
      const invocationId = '1'.repeat(64)
      expect(() => verifyBundledFundsMaterializationOutputV1(
        readyFixture(invocationId, root, 'v0.16.17'),
        invocationId,
        join(root, 'data')
      )).toThrow(/ready schema is invalid/)
    } finally {
      rmSync(root, { recursive: true, force: true })
    }
  })

  it('rejects duplicate-key JSON before schema or signature validation', () => {
    const root = testRuntimeHome()
    try {
      const invocationId = '1'.repeat(64)
      const stdout = readyFixture(invocationId, root)
      const duplicate = Buffer.from(stdout.toString('utf8').replace(
        '{"schemaVersion":1,',
        '{"schemaVersion":1,"schemaVersion":1,'
      ))
      expect(() => verifyBundledFundsMaterializationOutputV1(
        duplicate,
        invocationId,
        join(root, 'data')
      )).toThrow(/ready JSON is invalid/)
    } finally {
      rmSync(root, { recursive: true, force: true })
    }
  })

  it('invokes the exact bundled runtime before startup and consumes one nonce-bound line', async () => {
    const runtimeHome = testRuntimeHome()
    try {
      const dataDir = join(runtimeHome, 'data')
      let resultBuffer: Buffer | null = null
      const ready = await materializeBundledFundsBeforeRuntimeV1({
        appIsPackaged: true,
        launchTarget: { command: '/Applications/Analytix.app/Contents/Resources/runtime-go/bin/runtime-server', argsPrefix: [], mode: 'bundled-binary' },
        dataDir,
        runner: async (command, args) => {
          expect(command).toContain('/Resources/runtime-go/bin/runtime-server')
          expect(args.slice(0, 4)).toEqual([
            'bundled-plugin', 'materialize-funds-v1', '--data-dir', dataDir
          ])
          const invocationId = args[5]
          resultBuffer = readyFixture(invocationId, runtimeHome)
          return commandResult(resultBuffer)
        }
      })
      expect(ready?.invocationId).toMatch(/^[a-f0-9]{64}$/u)
      expect(resultBuffer).not.toBeNull()
      expect(bufferContainsOnlyZeroes(resultBuffer)).toBe(true)
    } finally {
      rmSync(runtimeHome, { recursive: true, force: true })
    }
  })

  it('does not run packaged materialization for source development and fails closed on command error', async () => {
    let calls = 0
    const notPackaged = await materializeBundledFundsBeforeRuntimeV1({
      appIsPackaged: false,
      launchTarget: { command: '/tmp/runtime-server', argsPrefix: [], mode: 'go-run-source' },
      dataDir: '/tmp/analytix/data',
      runner: async () => {
        calls += 1
        return commandResult(Buffer.alloc(0))
      }
    })
    expect(notPackaged).toBeNull()
    expect(calls).toBe(0)

    const stdout = Buffer.from('unexpected')
    const stderr = Buffer.from('private command detail')
    await expect(materializeBundledFundsBeforeRuntimeV1({
      appIsPackaged: true,
      launchTarget: { command: '/tmp/runtime-server', argsPrefix: [], mode: 'bundled-binary' },
      dataDir: '/tmp/analytix/data',
      runner: async () => ({ exitCode: 1, signal: null, stdout, stderr, timedOut: false, overflow: false })
    })).rejects.toThrow(/command failed/)
    expect(stdout.every((value) => value === 0)).toBe(true)
    expect(stderr.every((value) => value === 0)).toBe(true)
  })

  it('keeps materialization and config binding ahead of the runtime-server spawn', () => {
    const source = readFileSync(new URL('./analytix-adapter.ts', import.meta.url), 'utf8')
    const start = source.indexOf('async function startGoConformanceSidecarOnce')
    const body = source.slice(start)
    const optionalSettlement = body.indexOf(
      "await settleOptionalRuntimeCapability('bundled_funds'"
    )
    const materialize = body.indexOf('await materializeBundledFundsBeforeRuntimeV1(')
    const optionalConfigSync = body.indexOf('await syncRuntimeConfig(materialization)')
    const generalConfigSync = body.indexOf(
      'if (!bundledFundsConfigSynced) await syncRuntimeConfig()'
    )
    const spawn = body.indexOf('const child = spawn(')
    expect(start).toBeGreaterThanOrEqual(0)
    expect(optionalSettlement).toBeGreaterThanOrEqual(0)
    expect(materialize).toBeGreaterThan(optionalSettlement)
    expect(optionalConfigSync).toBeGreaterThan(materialize)
    expect(generalConfigSync).toBeGreaterThan(optionalConfigSync)
    expect(spawn).toBeGreaterThan(generalConfigSync)
  })

  it('keeps the verified binding only for the current runtime lifecycle and freezes it', () => {
    const root = testRuntimeHome()
    try {
      const invocationId = '1'.repeat(64)
      const ready = verifyBundledFundsMaterializationOutputV1(
        readyFixture(invocationId, root),
        invocationId,
        join(root, 'data')
      )
      bindBundledFundsMaterializationToCurrentRuntimeV1(ready)
      const current = currentBundledFundsMaterializationBindingV1()
      expect(current).not.toBeNull()
      expect(Object.isFrozen(current)).toBe(true)
      expect(Object.isFrozen(current?.receipt)).toBe(true)
      expect(() => {
        if (current) current.pluginVersion = '0.16.15' as '0.16.16'
      }).toThrow()
      expect(currentBundledFundsMaterializationBindingV1()?.pluginVersion).toBe('0.16.16')
      clearBundledFundsMaterializationCurrentRuntimeV1()
      expect(currentBundledFundsMaterializationBindingV1()).toBeNull()
    } finally {
      rmSync(root, { recursive: true, force: true })
    }
  })
})

function commandResult(stdout: Buffer): BundledFundsMaterializationCommandResultV1 {
  return { exitCode: 0, signal: null, stdout, stderr: Buffer.alloc(0), timedOut: false, overflow: false }
}

function readyFixture(invocationId: string, runtimeHome: string, pluginVersion = '0.16.16'): Buffer {
  const activeRelativePath = `plugins/cache/analytix-hub/analytix-fund-analysis/${pluginVersion}`
  const activePluginRoot = join(runtimeHome, activeRelativePath)
  mkdirSync(activePluginRoot, { recursive: true })
  const { privateKey, publicKey } = generateKeyPairSync('ed25519')
  const publicDer = publicKey.export({ format: 'der', type: 'spki' })
  const rawPublicKey = publicDer.subarray(publicDer.length - 32)
  const authorityKeyId = hash(rawPublicKey)
  const receipt: Record<string, unknown> = {
    schemaVersion: 1,
    purpose: 'analytix.bundled-plugin-materialization-receipt/v1',
    receiptId: '',
    intentId: '3'.repeat(64),
    packageAuthoritySha256: '4'.repeat(64),
    target: { platform: 'darwin', arch: 'arm64' },
    pluginName: 'analytix-fund-analysis',
    pluginVersion,
    generationId: '5'.repeat(64),
    activeRelativePath,
    sourceTreeSha256: '6'.repeat(64),
    sourceTreeFileCount: 42,
    manifestSha256: '7'.repeat(64),
    entrypointSha256: '8'.repeat(64),
    factToolsEnabled: false,
    issuedAt: '2026-07-23T18:00:00Z',
    authorityAlgorithm: 'Ed25519',
    authorityKeyId,
    authorityPublicKey: rawPublicKey.toString('base64url'),
    authoritySignature: ''
  }
  receipt.receiptId = domainHash('analytix.bundled-plugin-materialization-receipt/id/v1\0', receipt)
  const signingDigest = createHash('sha256').update(JSON.stringify(receipt)).digest()
  receipt.authoritySignature = sign(null, Buffer.concat([RECEIPT_SIGNATURE_DOMAIN, signingDigest]), privateKey).toString('base64url')
  const index: Record<string, unknown> = {
    schemaVersion: 1,
    purpose: 'analytix.bundled-plugin-materialization-index/v1',
    indexDigest: '',
    pluginName: receipt.pluginName,
    pluginVersion: receipt.pluginVersion,
    generationId: receipt.generationId,
    activeRelativePath: receipt.activeRelativePath,
    receiptId: receipt.receiptId,
    receiptSha256: hash(Buffer.from(JSON.stringify(receipt))),
    intentId: receipt.intentId,
    sourceTreeSha256: receipt.sourceTreeSha256,
    sourceTreeFileCount: receipt.sourceTreeFileCount,
    discoverableGenerationCount: 1,
    factToolsEnabled: false,
    committedAt: '2026-07-23T18:00:00Z'
  }
  index.indexDigest = domainHash('analytix.bundled-plugin-materialization-index/digest/v1\0', index)
  const ready: Record<string, unknown> = {
    schemaVersion: 1,
    purpose: 'analytix.bundled-funds-materialization-ready/v1',
    invocationId,
    configurationBindingDigest: '',
    packageAuthority: {
      fileSha256: receipt.packageAuthoritySha256,
      authorityDigest: 'a'.repeat(64),
      classification: 'controlled_release_clean_candidate_non_publishable',
      dispositionKind: 'controlled_release_receipt',
      platformAnchor: 'macos_developer_id_resource_seal'
    },
    runtimeIdentity: { payloadSha256: 'b'.repeat(64), payloadBytes: 4096, format: 'mach-o', arch: 'arm64' },
    pluginName: receipt.pluginName,
    pluginVersion: receipt.pluginVersion,
    activePluginRoot,
    receipt,
    index,
    publishable: false,
    factToolsEnabled: false,
    completedAt: '2026-07-23T18:00:00Z'
  }
  ready.configurationBindingDigest = domainHash('analytix.bundled-funds-materialization-ready/binding/v1\0', ready)
  return Buffer.from(`ANALYTIX_BUNDLED_FUNDS_MATERIALIZATION_READY_V1 ${JSON.stringify(ready)}\n`)
}

function domainHash(domain: string, value: Record<string, unknown>): string {
  return hash(Buffer.concat([Buffer.from(domain), Buffer.from(JSON.stringify(value))]))
}

function hash(value: Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function testRuntimeHome(): string {
  return realpathSync(mkdtempSync(join(tmpdir(), 'analytix-bundled-funds-test-')))
}

function bufferContainsOnlyZeroes(value: Buffer | null): boolean {
  return value !== null && value.every((item) => item === 0)
}
