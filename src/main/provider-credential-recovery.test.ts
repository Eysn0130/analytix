import { createHash } from 'node:crypto'
import { describe, expect, it, vi } from 'vitest'
import {
  createMainProviderCredentialRecoveryAuthority,
  type MainProviderCredentialRecoveryAuthority,
  type ProtectedRecoveryInvokeEvent
} from './provider-credential-recovery'
import type {
  ProtectedRecoveryRuntimeCall,
  ProtectedRecoveryRuntimeResult
} from './ipc/provider-registry-ipc'

const destinationProfileBinding = 'synthetic-destination-profile'
const destinationDataDirectory = '/isolated/synthetic-destination-data'
const sourceProfileBinding = 'synthetic-source-profile'
const sourceDataDirectory = '/isolated/synthetic-source-data'
const destinationWindowId = 41
const sourceWindowId = 42
const destinationFrameId = 43
const sourceFrameId = 44
const requestPath = '/isolated/synthetic/request.json'
const bundlePath = '/isolated/synthetic/bundle.json'
const receiptPath = '/isolated/synthetic/receipt.json'
const sessionNonce = 'synthetic-session-nonce'
const expiresAt = '2026-09-01T00:00:00.000Z'
const syntheticSecretMarker = 'synthetic-protected-secret-must-not-echo'

const portableManifestJson = JSON.stringify({
  schema: 'analytix.provider-portable-manifest/v1',
  providers: [{
    correlation: 'provider-0',
    kind: 'openai-compatible',
    endpoint: 'https://portable-provider.invalid',
    models: ['model-alpha'],
    mediaModels: [],
    selectedModel: 'model-alpha',
    routes: [],
    intent: 'reentry_required'
  }],
  accounts: []
})
const manifestDigest = createHash('sha256').update(portableManifestJson).digest('hex')
const portableImportReceipt = {
  manifestJson: portableManifestJson,
  entries: [{
    correlation: 'provider-0',
    destinationProviderId: 'provider-destination-0',
    status: 'reentry_required',
    fence: { revision: '1', generation: '1', incarnation: `inc_${'a'.repeat(43)}` }
  }]
}

const requestEntries = [{
  correlation: 'provider-0',
  destinationProviderId: 'provider-destination-0',
  destinationProviderRevision: 1,
  destinationProviderGeneration: 1,
  destinationProviderIncarnation: `inc_${'a'.repeat(43)}`
}]
const requestWithoutFingerprint = {
  schema: 'analytix.provider-protected-recovery-request/v1',
  protocolVersion: 1,
  manifestDigest,
  operationId: 'synthetic-protected-recovery-operation',
  sessionNonce,
  expiresAt,
  destinationEphemeralPublicKey: 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=',
  entries: requestEntries
}
// The request deliberately omits requestDigest. Main/Go compute it over the
// canonical request payload with verificationFingerprint omitted; the bundle
// and receipt carry the resulting digest.
const requestDigest = createHash('sha256').update(JSON.stringify(requestWithoutFingerprint)).digest('hex')
const requestFingerprint = createHash('sha256').update(requestDigest).digest('hex').slice(0, 16)
const itemSetDigest = createHash('sha256').update(JSON.stringify(requestEntries)).digest('hex')
const requestBytes = Buffer.from(JSON.stringify({
  ...requestWithoutFingerprint,
  verificationFingerprint: requestFingerprint
}))
const bundleWithoutDigest = {
  schema: 'analytix.provider-protected-recovery-bundle/v1',
  protocolVersion: 1,
  manifestDigest,
  requestDigest,
  operationId: 'synthetic-protected-recovery-operation',
  sessionNonce,
  expiresAt,
  sourceEphemeralPublicKey: 'AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=',
  nonce: 'AgICAgICAgICAgIC',
  ciphertext: 'AwMDAwMDAwMDAwMDAwMDAw==',
  entries: [{ correlation: 'provider-0', destinationProviderId: 'provider-destination-0', purpose: 'provider-api-key' }]
}
const bundleBytes = Buffer.from(JSON.stringify({ ...bundleWithoutDigest, requestDigest }))
const bundleDigest = createHash('sha256').update(bundleBytes).digest('hex')
const receiptBytes = Buffer.from(JSON.stringify({
  schema: 'analytix.provider-protected-recovery-receipt/v1',
  protocolVersion: 1,
  manifestDigest,
  requestDigest,
  bundleDigest,
  operationId: 'synthetic-protected-recovery-operation',
  sessionNonce,
  expiresAt,
  entries: [{ correlation: 'provider-0', destinationProviderId: 'provider-destination-0', status: 'applied' }],
  authenticationTag: 'BwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwc='
}))

function mainFrame(id: number, url = `file:///isolated/synthetic/${id}.html`) {
  return { routingId: id, url }
}

function mainWindow(id: number, frame: ReturnType<typeof mainFrame>) {
  return {
    id,
    isDestroyed: () => false,
    webContents: { id, isDestroyed: () => false, mainFrame: frame }
  }
}

function invokeEvent(
  window: ReturnType<typeof mainWindow>,
  frame: ReturnType<typeof mainFrame>
): ProtectedRecoveryInvokeEvent {
  return {
    sender: window.webContents as never,
    senderFrame: frame as never
  }
}

function authorityFixture() {
  const destinationFrame = mainFrame(destinationFrameId)
  const sourceFrame = mainFrame(sourceFrameId)
  const destinationWindow = mainWindow(destinationWindowId, destinationFrame)
  const sourceWindow = mainWindow(sourceWindowId, sourceFrame)
  const events: string[] = []

  const sourceOwnerBindingInventory = [
    {
      owner: 'mcp', provider: 'synthetic-source-mcp-server', accountId: 'synthetic-source-mcp-account',
      channelId: 'synthetic-source-mcp-channel', purpose: 'mcp-oauth-access-token',
      fingerprint: 'synthetic-source-mcp-current-binding'
    },
    {
      owner: 'extension', provider: 'synthetic-source-extension-package', accountId: 'synthetic-source-extension-account',
      channelId: 'synthetic-source-extension-channel', purpose: 'extension-provider-account-token',
      fingerprint: 'synthetic-source-extension-current-binding'
    }
  ]
  const destinationOwnerBindingInventory = [
    {
      owner: 'mcp', provider: 'synthetic-destination-mcp-server', accountId: 'synthetic-destination-mcp-account',
      channelId: 'synthetic-destination-mcp-channel', purpose: 'mcp-oauth-access-token',
      fingerprint: 'synthetic-destination-mcp-current-binding'
    },
    {
      owner: 'extension', provider: 'synthetic-destination-extension-package', accountId: 'synthetic-destination-extension-account',
      channelId: 'synthetic-destination-extension-channel', purpose: 'extension-provider-account-token',
      fingerprint: 'synthetic-destination-extension-current-binding'
    }
  ]
  let active = {
    side: 'destination' as 'destination' | 'source',
    window: destinationWindow,
    frame: destinationFrame,
    profileBinding: destinationProfileBinding,
    dataDirectory: destinationDataDirectory
  }

  const showOpenDialog = vi.fn()
    .mockImplementationOnce(async () => {
      events.push('native:open')
      return { canceled: false, filePaths: [requestPath] }
    })
    .mockImplementationOnce(async () => {
      events.push('native:open')
      return { canceled: false, filePaths: [bundlePath] }
    })
    .mockImplementationOnce(async () => {
      events.push('native:open')
      return { canceled: false, filePaths: [receiptPath] }
    })
  const showSaveDialog = vi.fn()
    .mockImplementationOnce(async () => {
      events.push('native:save')
      return { canceled: false, filePath: requestPath }
    })
    .mockImplementationOnce(async () => {
      events.push('native:save')
      return { canceled: false, filePath: bundlePath }
    })
    .mockImplementationOnce(async () => {
      events.push('native:save')
      return { canceled: false, filePath: receiptPath }
    })
  const confirmNative = vi.fn().mockImplementation(async (confirmation: { side: string }) => {
    events.push(`confirm:${confirmation.side}`)
    return true
  })
  const lstat = vi.fn().mockResolvedValue({
    isFile: () => true,
    isSymbolicLink: () => false,
    isDirectory: () => false
  })
  lstat.mockImplementation(async (path: string) => path === '/isolated/synthetic'
    ? { isFile: () => false, isSymbolicLink: () => false, isDirectory: () => true }
    : { isFile: () => true, isSymbolicLink: () => false, isDirectory: () => false })
  const readInputs = [Buffer.from(requestBytes), Buffer.from(bundleBytes), Buffer.from(receiptBytes)]
  const readBuffers = [...readInputs]
  const readFile = vi.fn(async () => {
    events.push('native:read')
    return readBuffers.shift()!
  })
  const writeInputs: Uint8Array[] = []
  const writeFile = vi.fn(async (_path: string, bytes: Uint8Array) => {
    events.push('native:write')
    writeInputs.push(bytes)
  })
  const runtimeRequest = vi.fn()
  const privateArtifactBuffers = [Buffer.from(requestBytes), Buffer.from(bundleBytes), Buffer.from(receiptBytes)]
  const privateArtifacts = [...privateArtifactBuffers]
  const protectedRuntimeRequest = vi.fn()
    .mockImplementation(async (call: { operation: string; [key: string]: unknown }) => {
      events.push(`private:${call.operation}`)
      switch (call.operation) {
        case 'prepare-protected-recovery-request':
          return {
            ok: true,
            status: 'request-prepared',
            artifactBytes: privateArtifacts.shift(),
            requestDigest,
            requestFingerprint,
            manifestDigest,
            itemSetDigest,
            operationId: 'synthetic-protected-recovery-operation',
            sessionNonce,
            expiresAt
          }
        case 'confirm-protected-recovery-destination':
          return { ok: true, status: 'destination-confirmed' }
        case 'create-protected-recovery-bundle':
          return { ok: true, status: 'bundle-created', artifactBytes: privateArtifacts.shift() }
        case 'apply-protected-recovery-bundle':
          return { ok: true, status: 'receipt-created', entryCount: 1, artifactBytes: privateArtifacts.shift() }
        case 'finalize-protected-recovery-receipt':
          return { ok: true, status: 'finalized', entryCount: 1 }
        case 'rollback-protected-recovery':
          return { ok: true, status: 'rolled_back' }
        default:
          throw new Error(`unexpected protected operation ${call.operation}`)
      }
    })
  const getCurrentWindow = vi.fn(() => active.window)
  const getCurrentMainFrame = vi.fn(() => active.frame)
  const getCurrentProfile = vi.fn(() => ({
    profileBinding: active.profileBinding,
    dataDirectory: active.dataDirectory
  }))
  const getPortableImportReceipt = vi.fn(() => portableImportReceipt)
  const getCurrentPortableManifest = vi.fn(() => portableManifestJson)
  const getCurrentOwnerBindingInventory = vi.fn(() => (
    active.side === 'destination' ? destinationOwnerBindingInventory : sourceOwnerBindingInventory
  ))
  const resolveMcpBindingFingerprint = vi.fn(async () => (
    active.side === 'destination'
      ? 'synthetic-destination-mcp-current-binding'
      : 'synthetic-source-mcp-current-binding'
  ))
  const resolveExtensionBindingFingerprint = vi.fn(async () => (
    active.side === 'destination'
      ? 'synthetic-destination-extension-current-binding'
      : 'synthetic-source-extension-current-binding'
  ))
  const authority = createMainProviderCredentialRecoveryAuthority({
    showOpenDialog,
    showSaveDialog,
    confirmNative,
    lstat,
    readFile,
    writeFile,
    runtimeRequest,
    protectedRuntimeRequest,
    getCurrentWindow,
    getCurrentMainFrame,
    getCurrentProfile,
    getPortableImportReceipt,
    getCurrentPortableManifest,
    getCurrentOwnerBindingInventory,
    resolveMcpBindingFingerprint,
    resolveExtensionBindingFingerprint,
    getSaveTarget: (role: 'request' | 'bundle' | 'receipt') => (
      role === 'request' ? requestPath : role === 'bundle' ? bundlePath : receiptPath
    )
  } as never) as MainProviderCredentialRecoveryAuthority
  return {
    authority,
    destinationWindow,
    destinationFrame,
    sourceWindow,
    sourceFrame,
    setActive: (
      side: 'destination' | 'source',
      profileBinding: string,
      dataDirectory: string,
      window: typeof destinationWindow,
      frame: typeof destinationFrame
    ) => {
      active = { side, window, frame, profileBinding, dataDirectory }
    },
    showOpenDialog,
    showSaveDialog,
    confirmNative,
    lstat,
    readFile,
    readBuffers,
    readInputs,
    writeFile,
    writeInputs,
    runtimeRequest,
    protectedRuntimeRequest,
    privateArtifacts,
    privateArtifactBuffers,
    getCurrentWindow,
    getCurrentMainFrame,
    getCurrentProfile,
    getPortableImportReceipt,
    getCurrentPortableManifest,
    getCurrentOwnerBindingInventory,
    resolveMcpBindingFingerprint,
    resolveExtensionBindingFingerprint,
    sourceOwnerBindingInventory,
    destinationOwnerBindingInventory,
    events
  }
}

function expectEventOrder(events: string[], ...wanted: string[]) {
  let cursor = -1
  for (const event of wanted) {
    const index = events.indexOf(event, cursor + 1)
    expect(index, `missing ordered event ${event} in ${JSON.stringify(events)}`).toBeGreaterThan(cursor)
    cursor = index
  }
}

describe('Main protected provider credential recovery authority', () => {
  it('uses the exact import receipt, five private calls, two confirmations, and redacted results', async () => {
    const fixture = authorityFixture()
    const destinationEvent = invokeEvent(fixture.destinationWindow, fixture.destinationFrame)
    // The first destination target is a regular nonexistent path; Main's one
    // hardened artifact-writer seam owns this case and still performs the
    // private prepare -> confirm -> save/write sequence.
    fixture.lstat.mockResolvedValueOnce({
      isFile: () => false,
      isSymbolicLink: () => false,
      isDirectory: () => false
    })

    const requestResult = await fixture.authority.createDestinationRequest(destinationEvent)

    fixture.setActive(
      'source', sourceProfileBinding, sourceDataDirectory, fixture.sourceWindow, fixture.sourceFrame
    )
    const sourceEvent = invokeEvent(fixture.sourceWindow, fixture.sourceFrame)
    const bundleResult = await fixture.authority.createSourceBundle(sourceEvent)

    fixture.setActive(
      'destination', destinationProfileBinding, destinationDataDirectory,
      fixture.destinationWindow, fixture.destinationFrame
    )
    const applyResult = await fixture.authority.applyDestinationBundle(destinationEvent)

    fixture.setActive(
      'source', sourceProfileBinding, sourceDataDirectory, fixture.sourceWindow, fixture.sourceFrame
    )
    const finalizeResult = await fixture.authority.finalizeSourceReceipt(sourceEvent)

    expect(requestResult).toEqual({ ok: true, status: 'request-created' })
    expect(bundleResult).toEqual({ ok: true, status: 'bundle-created' })
    expect(applyResult).toEqual({ ok: true, status: 'receipt-created', entryCount: 1 })
    expect(finalizeResult).toEqual({ ok: true, status: 'finalized', entryCount: 1 })
    expect(fixture.confirmNative).toHaveBeenCalledTimes(2)
    expect(fixture.confirmNative).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({
        side: 'destination', requestFingerprint, requestDigest, manifestDigest, itemSetDigest, expiresAt,
        operationId: 'synthetic-protected-recovery-operation', sessionNonce
      })
    )
    expect(fixture.confirmNative).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        side: 'source', requestFingerprint, requestDigest, manifestDigest, itemSetDigest, expiresAt,
        operationId: 'synthetic-protected-recovery-operation', sessionNonce
      })
    )
    expect(fixture.confirmNative.mock.calls[0][0]).not.toHaveProperty('profileBinding')
    expect(fixture.confirmNative.mock.calls[0][0]).not.toHaveProperty('dataDirectory')
    expect(fixture.confirmNative.mock.calls[0][0]).not.toHaveProperty('windowId')
    expect(fixture.confirmNative.mock.calls[0][0]).not.toHaveProperty('frameId')
    expect(fixture.getPortableImportReceipt).toHaveBeenCalledWith()
    expect(fixture.getCurrentOwnerBindingInventory).toHaveBeenCalled()
    expect(fixture.resolveMcpBindingFingerprint).toHaveBeenCalled()
    expect(fixture.resolveExtensionBindingFingerprint).toHaveBeenCalled()
    expect(fixture.runtimeRequest).not.toHaveBeenCalled()
    expect(fixture.protectedRuntimeRequest).toHaveBeenCalledTimes(5)
    expect(fixture.protectedRuntimeRequest.mock.calls.map(([call]) => call.operation)).toEqual([
      'prepare-protected-recovery-request',
      'confirm-protected-recovery-destination',
      'create-protected-recovery-bundle',
      'apply-protected-recovery-bundle',
      'finalize-protected-recovery-receipt'
    ])

    const prepareCall = fixture.protectedRuntimeRequest.mock.calls[0][0] as Record<string, unknown>
    const importReceipt = prepareCall.importReceipt ?? prepareCall.portableImportReceipt ?? {
      manifestJson: prepareCall.manifestJson,
      entries: prepareCall.entries
    }
    expect(importReceipt).toEqual(portableImportReceipt)
    expect(prepareCall).toEqual(expect.objectContaining({
      ownerBindingInventory: []
    }))
    expect(prepareCall.manifestDigest).toBeUndefined()
    const confirmationCallJSON = JSON.stringify(fixture.protectedRuntimeRequest.mock.calls[1][0])
    for (const generated of [requestDigest, requestFingerprint, itemSetDigest, expiresAt, sessionNonce]) {
      expect(confirmationCallJSON).toContain(generated)
    }
    expect(fixture.protectedRuntimeRequest.mock.calls[1][0]).toEqual(expect.objectContaining({
      ownerBindingInventory: []
    }))
    expect(fixture.protectedRuntimeRequest.mock.calls[2][0]).toEqual(expect.objectContaining({
      ownerBindingInventory: []
    }))
    expect(fixture.protectedRuntimeRequest.mock.calls[3][0]).toEqual(expect.objectContaining({
      ownerBindingInventory: []
    }))
    expect(JSON.stringify(fixture.protectedRuntimeRequest.mock.calls)).not.toMatch(
      new RegExp(`${requestPath}|${bundlePath}|${receiptPath}|fileHandle|${syntheticSecretMarker}|credentialRef`, 'i')
    )
    expect(requestBytes.toString()).not.toContain('"requestDigest"')
    expect(fixture.showOpenDialog).toHaveBeenCalledTimes(3)
    expect(fixture.showSaveDialog).toHaveBeenCalledTimes(3)
    // request is saved then opened by source, bundle is saved then opened by
    // destination, and receipt is saved then opened by source: all six paths
    // pass the same hardened artifact seam.
    // Each selected save target is checked before the private operation and
    // again immediately before the hardened write; each open input is checked
    // once. This closes the target replacement window without exposing a path
    // to the renderer.
    expect(fixture.lstat).toHaveBeenCalledTimes(15)
    const validatedPaths = fixture.lstat.mock.calls.map(([path]) => path)
    expect(validatedPaths.filter(path => path === requestPath)).toHaveLength(3)
    expect(validatedPaths.filter(path => path === bundlePath)).toHaveLength(3)
    expect(validatedPaths.filter(path => path === receiptPath)).toHaveLength(3)
    expect(fixture.writeFile).toHaveBeenCalledTimes(3)
    expect(fixture.writeInputs).toHaveLength(3)
    expect(fixture.writeInputs.every(bytes => bytes.every(value => value === 0))).toBe(true)
    expect(fixture.readBuffers).toHaveLength(0)
    expect(fixture.privateArtifacts).toHaveLength(0)
    expect(fixture.readInputs.every(bytes => bytes.every(value => value === 0))).toBe(true)
    expect(fixture.privateArtifactBuffers.every(bytes => bytes.every(value => value === 0))).toBe(true)
    expectEventOrder(
      fixture.events,
      'private:prepare-protected-recovery-request', 'confirm:destination',
      'private:confirm-protected-recovery-destination', 'native:save', 'native:write',
      'native:open', 'native:read', 'confirm:source', 'private:create-protected-recovery-bundle',
      'native:save', 'native:write',
      'native:open', 'native:read', 'private:apply-protected-recovery-bundle', 'native:save', 'native:write',
      'native:open', 'native:read', 'private:finalize-protected-recovery-receipt'
    )
    for (const externalRecord of [requestBytes, bundleBytes, receiptBytes]) {
      const externalJSON = externalRecord.toString()
      for (const localAuthority of [destinationProfileBinding, destinationDataDirectory, sourceProfileBinding, sourceDataDirectory]) {
        expect(externalJSON).not.toContain(localAuthority)
      }
      const externalValue = JSON.parse(externalJSON) as Record<string, unknown>
      for (const localAuthorityField of ['windowId', 'frameId', 'profileBinding', 'dataDirectory']) {
        expect(externalValue).not.toHaveProperty(localAuthorityField)
      }
    }
    const publicResultJSON = JSON.stringify({ requestResult, bundleResult, applyResult, finalizeResult })
    expect(publicResultJSON).not.toMatch(
      new RegExp(`${syntheticSecretMarker}|ciphertext|privateKey|fileHandle|artifactBytes|${requestPath}`, 'i')
    )
  })

  it('rejects stale invoke context and symlink input before private runtime calls', async () => {
    const fixture = authorityFixture()
    const staleFrame = mainFrame(destinationFrameId, 'file:///isolated/synthetic/stale.html')
    const staleEvent = invokeEvent(fixture.destinationWindow, staleFrame)

    await expect(fixture.authority.createDestinationRequest(staleEvent))
      .resolves.toEqual({ ok: false, code: 'invalid_context' })
    expect(fixture.protectedRuntimeRequest).not.toHaveBeenCalled()

    fixture.setActive(
      'source', sourceProfileBinding, sourceDataDirectory, fixture.sourceWindow, fixture.sourceFrame
    )
    fixture.lstat.mockResolvedValueOnce({
      isFile: () => true,
      isSymbolicLink: () => true,
      isDirectory: () => false
    })
    await expect(fixture.authority.createSourceBundle(
      invokeEvent(fixture.sourceWindow, fixture.sourceFrame)
    )).resolves.toEqual({ ok: false, code: 'invalid_path' })
    expect(fixture.protectedRuntimeRequest).not.toHaveBeenCalled()
    expect(JSON.stringify(fixture.protectedRuntimeRequest.mock.calls)).not.toMatch(
      new RegExp(`${syntheticSecretMarker}|${requestPath}|${bundlePath}`, 'i')
    )
  })

  it('rejects symlink and non-regular save targets before private mutation or write', async () => {
    for (const stat of [
      { isFile: () => true, isSymbolicLink: () => true, isDirectory: () => false },
      { isFile: () => false, isSymbolicLink: () => false, isDirectory: () => true },
      {
        isFile: () => false, isSymbolicLink: () => false, isDirectory: () => false,
        isFIFO: () => true
      }
    ]) {
      const fixture = authorityFixture()
      fixture.lstat.mockResolvedValueOnce(stat)

      await expect(fixture.authority.createDestinationRequest(
        invokeEvent(fixture.destinationWindow, fixture.destinationFrame)
      )).resolves.toEqual({ ok: false, code: 'invalid_path' })
      expect(fixture.protectedRuntimeRequest).not.toHaveBeenCalled()
      expect(fixture.writeFile).not.toHaveBeenCalled()
      expect(JSON.stringify(fixture.protectedRuntimeRequest.mock.calls)).not.toMatch(
        new RegExp(`${requestPath}|${syntheticSecretMarker}`, 'i')
      )
    }
  })

  it('rejects malformed private artifacts, stale import evidence, and owner inventory drift before publication', async () => {
    for (const malformed of [
      '{not-json',
      requestBytes.toString('utf8').replace(
        `"verificationFingerprint":"${requestFingerprint}"`,
        `"verificationFingerprint":"${requestFingerprint}","verificationFingerprint":"${requestFingerprint}"`
      ),
      requestBytes.toString('utf8').replace('"entries":[', '"unknown":1,"entries":['),
      requestBytes.toString('utf8').replace('{"schema"', '{ "schema"')
    ]) {
      const fixture = authorityFixture()
      fixture.lstat.mockResolvedValueOnce({
        isFile: () => false,
        isSymbolicLink: () => false,
        isDirectory: () => false
      })
      fixture.privateArtifacts[0] = Buffer.from(malformed)
      await expect(fixture.authority.createDestinationRequest(
        invokeEvent(fixture.destinationWindow, fixture.destinationFrame)
      )).resolves.toEqual({ ok: false, code: 'invalid_response' })
      expect(fixture.confirmNative).not.toHaveBeenCalled()
      expect(fixture.writeFile).not.toHaveBeenCalled()
    }

    const staleReceipt = authorityFixture()
    staleReceipt.getPortableImportReceipt.mockReturnValue(null as never)
    await expect(staleReceipt.authority.createDestinationRequest(
      invokeEvent(staleReceipt.destinationWindow, staleReceipt.destinationFrame)
    )).resolves.toEqual({ ok: false, code: 'invalid_request' })
    expect(staleReceipt.protectedRuntimeRequest).not.toHaveBeenCalled()

    const driftedInventory = authorityFixture()
    driftedInventory.lstat.mockResolvedValueOnce({
      isFile: () => false,
      isSymbolicLink: () => false,
      isDirectory: () => false
    })
    driftedInventory.getCurrentOwnerBindingInventory
      .mockImplementationOnce(() => driftedInventory.destinationOwnerBindingInventory)
      .mockImplementationOnce(() => [{
        owner: 'hub', provider: 'drift', accountId: 'drift', channelId: 'drift',
        purpose: 'hub-credential', fingerprint: 'drift'
      }] as never)
    await expect(driftedInventory.authority.createDestinationRequest(
      invokeEvent(driftedInventory.destinationWindow, driftedInventory.destinationFrame)
    )).resolves.toEqual({ ok: false, code: 'invalid_request' })
    expect(driftedInventory.protectedRuntimeRequest.mock.calls.map(([call]) => call.operation)).toEqual([
      'prepare-protected-recovery-request', 'rollback-protected-recovery'
    ])
    expect(driftedInventory.writeFile).not.toHaveBeenCalled()
  })

  it.each([
    ['wrong provider', 'provider', 'synthetic-current-wrong-provider'],
    ['wrong accountId', 'accountId', 'synthetic-current-wrong-account'],
    ['wrong channelId', 'channelId', 'synthetic-current-wrong-channel'],
    ['wrong fingerprint', 'fingerprint', 'synthetic-current-wrong-fingerprint'],
    ['wrong correlation', 'correlation', 'account-99']
  ] as const)('rejects an admitted destination binding with %s before private prepare and native confirmation', async (
    _name,
    field,
    driftedValue
  ) => {
    const fixture = authorityFixture()
    const admittedBinding = {
      correlation: 'account-0',
      owner: 'mcp',
      provider: 'synthetic-admitted-destination-provider',
      accountId: 'synthetic-admitted-destination-account',
      channelId: 'synthetic-admitted-destination-channel',
      purpose: 'mcp-oauth-access-token',
      fingerprint: 'synthetic-admitted-destination-fingerprint'
    }
    const currentBinding = { ...admittedBinding, [field]: driftedValue }
    const accountManifestJson = JSON.stringify({
      schema: 'analytix.provider-portable-manifest/v1',
      providers: [],
      accounts: [{
        correlation: 'account-0',
        owner: 'mcp',
        // The source descriptor is deliberately aligned with the drifted
        // current inventory. Only the destination-local binding admitted by
        // the ordinary import receipt distinguishes the two authorities.
        provider: currentBinding.provider,
        endpoint: 'https://portable-account.invalid/v1',
        purpose: 'mcp-oauth-access-token',
        intent: 'reentry_required'
      }]
    })
    fixture.getPortableImportReceipt.mockReturnValue({
      manifestJson: accountManifestJson,
      entries: [{
        correlation: 'account-0',
        destinationProviderId: 'provider-private-destination',
        status: 'reentry_required',
        fence: { revision: '1', generation: '1', incarnation: `inc_${'d'.repeat(43)}` },
        destinationOwnerBinding: admittedBinding
      }]
    } as never)
    fixture.getCurrentOwnerBindingInventory.mockReturnValue([currentBinding] as never)
    fixture.resolveMcpBindingFingerprint.mockResolvedValue(currentBinding.fingerprint as never)

    const result = await fixture.authority.createDestinationRequest(
      invokeEvent(fixture.destinationWindow, fixture.destinationFrame)
    )
    expect.soft(result).toEqual({ ok: false, code: 'invalid_request' })
    expect.soft(fixture.protectedRuntimeRequest).toHaveBeenCalledTimes(0)
    expect.soft(fixture.confirmNative).toHaveBeenCalledTimes(0)
    expect.soft(fixture.showSaveDialog).toHaveBeenCalledTimes(0)
    expect.soft(fixture.writeFile).toHaveBeenCalledTimes(0)
  })

  it('rechecks the exact admitted receipt before the native prompt and after bundle open', async () => {
    const driftedReceipt = {
      ...portableImportReceipt,
      entries: portableImportReceipt.entries.map((entry) => ({
        ...entry,
        fence: { ...entry.fence, revision: '2' }
      }))
    }

    const promptDrift = authorityFixture()
    promptDrift.getPortableImportReceipt
      .mockImplementationOnce(() => portableImportReceipt)
      .mockReturnValue(driftedReceipt as never)
    const promptResult = await promptDrift.authority.createDestinationRequest(
      invokeEvent(promptDrift.destinationWindow, promptDrift.destinationFrame)
    )
    expect(promptResult).toEqual({ ok: false, code: 'invalid_request' })
    expect(promptDrift.protectedRuntimeRequest.mock.calls.map(([call]) => call.operation)).toEqual([
      'prepare-protected-recovery-request', 'rollback-protected-recovery'
    ])
    expect(promptDrift.confirmNative).not.toHaveBeenCalled()
    expect(promptDrift.showSaveDialog).not.toHaveBeenCalled()
    expect(promptDrift.writeFile).not.toHaveBeenCalled()

    const postOpenDrift = authorityFixture()
    const destinationEvent = invokeEvent(postOpenDrift.destinationWindow, postOpenDrift.destinationFrame)
    await expect(postOpenDrift.authority.createDestinationRequest(destinationEvent))
      .resolves.toEqual({ ok: true, status: 'request-created' })
    postOpenDrift.privateArtifacts.splice(0, postOpenDrift.privateArtifacts.length, Buffer.from(receiptBytes))
    postOpenDrift.readBuffers.splice(0, postOpenDrift.readBuffers.length, Buffer.from(bundleBytes))
    postOpenDrift.showSaveDialog.mockReset()
    postOpenDrift.showSaveDialog.mockResolvedValue({ canceled: false, filePath: receiptPath })
    let bundleOpened = false
    postOpenDrift.readFile.mockImplementation(async () => {
      postOpenDrift.events.push('native:read')
      bundleOpened = true
      return postOpenDrift.readBuffers.shift()!
    })
    postOpenDrift.getPortableImportReceipt.mockImplementation(() => (
      bundleOpened ? driftedReceipt : portableImportReceipt
    ) as never)

    const applyResult = await postOpenDrift.authority.applyDestinationBundle(destinationEvent)
    expect(applyResult).toEqual({ ok: false, code: 'invalid_request' })
    expect(postOpenDrift.protectedRuntimeRequest.mock.calls.map(([call]) => call.operation)).toEqual([
      'prepare-protected-recovery-request', 'confirm-protected-recovery-destination'
    ])
    expect(postOpenDrift.readFile).toHaveBeenCalledTimes(1)
    expect(postOpenDrift.writeFile).toHaveBeenCalledTimes(1)
  })

  it('preflights targeted durable recovery before showing a native confirmation', async () => {
    const fixture = authorityFixture()
    fixture.getPortableImportReceipt.mockReturnValue(null as never)
    fixture.readBuffers.splice(0, fixture.readBuffers.length, Buffer.from(requestBytes))
    fixture.protectedRuntimeRequest.mockImplementation(async (call: ProtectedRecoveryRuntimeCall) => {
      if (call.operation === 'recover-protected-recovery') {
        return { ok: false, code: 'invalid_request' }
      }
      throw new Error(`unexpected protected operation ${call.operation}`)
    })

    await expect(fixture.authority.applyDestinationBundle(
      invokeEvent(fixture.destinationWindow, fixture.destinationFrame)
    )).resolves.toEqual({ ok: false, code: 'invalid_request' })
    expect(fixture.protectedRuntimeRequest.mock.calls.map(([call]) => call.operation)).toEqual([
      'recover-protected-recovery'
    ])
    expect(fixture.confirmNative).not.toHaveBeenCalled()
  })

  it('keeps destination apply unreachable when context or owner inventory changes while opening the bundle', async () => {
    const cases: Array<{
      name: string
      mutate: (fixture: ReturnType<typeof authorityFixture>) => void
      expectedCode: 'invalid_context' | 'invalid_request'
    }> = [
      {
        name: 'window and frame',
        mutate: (fixture) => {
          fixture.setActive(
            'destination', destinationProfileBinding, destinationDataDirectory,
            mainWindow(destinationWindowId + 1, mainFrame(destinationFrameId + 1)),
            mainFrame(destinationFrameId + 1)
          )
        },
        expectedCode: 'invalid_context'
      },
      {
        name: 'profile binding',
        mutate: (fixture) => {
          fixture.setActive(
            'destination', 'synthetic-destination-profile-drifted', destinationDataDirectory,
            fixture.destinationWindow, fixture.destinationFrame
          )
        },
        expectedCode: 'invalid_request'
      },
      {
        name: 'data directory',
        mutate: (fixture) => {
          fixture.setActive(
            'destination', destinationProfileBinding, '/isolated/synthetic-destination-data-drifted',
            fixture.destinationWindow, fixture.destinationFrame
          )
        },
        expectedCode: 'invalid_request'
      },
      {
        name: 'owner fingerprint',
        mutate: (fixture) => {
          fixture.destinationOwnerBindingInventory[0]!.fingerprint =
            'synthetic-destination-mcp-drifted-binding'
        },
        expectedCode: 'invalid_request'
      }
    ]

    for (const testCase of cases) {
      const fixture = authorityFixture()
      fixture.getPortableImportReceipt.mockReturnValue(null as never)
      fixture.readBuffers.splice(0, fixture.readBuffers.length, Buffer.from(requestBytes), Buffer.from(bundleBytes))
      fixture.showSaveDialog.mockReset()
      fixture.showSaveDialog.mockResolvedValue({ canceled: false, filePath: receiptPath })
      let readCount = 0
      fixture.readFile.mockImplementation(async () => {
        fixture.events.push('native:read')
        const bytes = fixture.readBuffers.shift()!
        if (readCount++ === 1) testCase.mutate(fixture)
        return bytes
      })
      let applyCalled = false
      fixture.protectedRuntimeRequest.mockImplementation(async (call: ProtectedRecoveryRuntimeCall) => {
        if (call.operation === 'recover-protected-recovery') {
          return { ok: true, status: 'reconfirmation_required', entryCount: 1 }
        }
        if (call.operation === 'confirm-protected-recovery-destination') {
          return { ok: true, status: 'destination-confirmed' }
        }
        if (call.operation === 'apply-protected-recovery-bundle') {
          applyCalled = true
          return {
            ok: true,
            status: 'receipt-created',
            entryCount: 1,
            artifactBytes: Buffer.from(receiptBytes)
          }
        }
        throw new Error(`unexpected protected operation ${call.operation}`)
      })

      await expect(fixture.authority.applyDestinationBundle(
        invokeEvent(fixture.destinationWindow, fixture.destinationFrame)
      ), testCase.name).resolves.toEqual({ ok: false, code: testCase.expectedCode })
      expect(applyCalled, testCase.name).toBe(false)
      expect(fixture.protectedRuntimeRequest.mock.calls.map(([call]) => call.operation), testCase.name)
        .not.toContain('apply-protected-recovery-bundle')
      expect(fixture.writeFile.mock.calls, testCase.name).toHaveLength(0)
    }
  })

  it('keeps a cancelled native confirmation non-publishing and does not call the confirm seam', async () => {
    const fixture = authorityFixture()
    fixture.lstat.mockResolvedValueOnce({
      isFile: () => false,
      isSymbolicLink: () => false,
      isDirectory: () => false
    })
    fixture.confirmNative.mockResolvedValueOnce(false)
    await expect(fixture.authority.createDestinationRequest(
      invokeEvent(fixture.destinationWindow, fixture.destinationFrame)
    )).resolves.toEqual({ ok: false, code: 'cancelled' })
    expect(fixture.protectedRuntimeRequest.mock.calls.map(([call]) => call.operation)).toEqual([
      'prepare-protected-recovery-request', 'rollback-protected-recovery'
    ])
    expect(fixture.writeFile).not.toHaveBeenCalled()
  })

  it('reopens exact saved request and bundle through durable recovery after a new Main authority instance', async () => {
    let prepared = false
    let sourceBundleCreated = false
    let destinationReconfirmed = false
    const durableRuntime = vi.fn(async (call: ProtectedRecoveryRuntimeCall): Promise<ProtectedRecoveryRuntimeResult> => {
      switch (call.operation) {
        case 'prepare-protected-recovery-request':
          prepared = true
          return {
            ok: true,
            status: 'request-prepared',
            artifactBytes: Buffer.from(requestBytes),
            requestDigest,
            requestFingerprint,
            manifestDigest,
            itemSetDigest,
            operationId: 'synthetic-protected-recovery-operation',
            sessionNonce,
            expiresAt
          }
        case 'confirm-protected-recovery-destination':
          destinationReconfirmed = true
          return { ok: true, status: 'destination-confirmed' }
        case 'create-protected-recovery-bundle':
          sourceBundleCreated = true
          return { ok: true, status: 'bundle-created', artifactBytes: Buffer.from(bundleBytes) }
        case 'recover-protected-recovery':
          return { ok: true, status: 'reconfirmation_required', entryCount: 1 }
        case 'apply-protected-recovery-bundle':
          if (!prepared || !sourceBundleCreated || !destinationReconfirmed) {
            return { ok: false, code: 'invalid_request' }
          }
          return {
            ok: true,
            status: 'receipt-created',
            entryCount: 1,
            artifactBytes: Buffer.from(receiptBytes)
          }
        case 'rollback-protected-recovery':
          return { ok: true, status: 'rolled_back' }
        default:
          return { ok: false, code: 'invalid_request' }
      }
    })

    const original = authorityFixture()
    original.protectedRuntimeRequest.mockImplementation(durableRuntime)
    const savedArtifacts = new Map<string, Buffer>()
    original.writeFile.mockImplementation(async (path: string, bytes: Uint8Array) => {
      savedArtifacts.set(path, Buffer.from(bytes))
    })

    const destinationEvent = invokeEvent(original.destinationWindow, original.destinationFrame)
    expect(await original.authority.createDestinationRequest(destinationEvent)).toEqual({
      ok: true,
      status: 'request-created'
    })
    original.setActive(
      'source', sourceProfileBinding, sourceDataDirectory, original.sourceWindow, original.sourceFrame
    )
    expect(await original.authority.createSourceBundle(
      invokeEvent(original.sourceWindow, original.sourceFrame)
    )).toEqual({ ok: true, status: 'bundle-created' })
    expect(savedArtifacts.get(requestPath)).toEqual(requestBytes)
    expect(savedArtifacts.get(bundlePath)).toEqual(bundleBytes)

    const restarted = authorityFixture()
    restarted.protectedRuntimeRequest.mockImplementation(durableRuntime)
    restarted.getPortableImportReceipt.mockReturnValue(null as never)
    restarted.destinationOwnerBindingInventory.unshift({
      owner: 'extension', provider: 'aaa-unrelated-provider', accountId: 'unrelated-account',
      channelId: 'unrelated-channel', purpose: 'extension-provider-account-token',
      fingerprint: 'unrelated-binding'
    })
    restarted.readBuffers.splice(
      0,
      restarted.readBuffers.length,
      Buffer.from(savedArtifacts.get(requestPath)!),
      Buffer.from(savedArtifacts.get(bundlePath)!)
    )
    restarted.showSaveDialog.mockReset()
    restarted.showSaveDialog.mockResolvedValue({ canceled: false, filePath: receiptPath })

    const result = await restarted.authority.applyDestinationBundle(
      invokeEvent(restarted.destinationWindow, restarted.destinationFrame)
    )

    expect(result).toEqual({ ok: true, status: 'receipt-created', entryCount: 1 })
    expect(restarted.getPortableImportReceipt).toHaveBeenCalled()
    expect(restarted.confirmNative).toHaveBeenCalledTimes(1)
    expect(restarted.protectedRuntimeRequest.mock.calls.map(([call]) => call.operation)).toEqual([
      'recover-protected-recovery',
      'confirm-protected-recovery-destination',
      'apply-protected-recovery-bundle'
    ])
    expect(restarted.protectedRuntimeRequest.mock.calls[2][0]).toEqual(expect.objectContaining({
      bundleBytes: expect.any(Buffer),
      requestBytes: expect.any(Buffer)
    }))
    for (const [call] of restarted.protectedRuntimeRequest.mock.calls) {
      if (call.operation === 'recover-protected-recovery' ||
        call.operation === 'confirm-protected-recovery-destination' ||
        call.operation === 'apply-protected-recovery-bundle') {
        expect(call.ownerBindingInventory).toEqual(restarted.destinationOwnerBindingInventory)
        expect(call.ownerBindingInventory?.every((entry: { correlation?: string }) => entry.correlation === undefined)).toBe(true)
      }
    }
  })

  it('does not auto-apply or accept a durable owner/fence drift from a new Main authority instance', async () => {
    const savedRequest = Buffer.from(requestBytes)
    const savedBundle = Buffer.from(bundleBytes)

    const cancelled = authorityFixture()
    cancelled.getPortableImportReceipt.mockReturnValue(null as never)
    cancelled.readBuffers.splice(0, cancelled.readBuffers.length, savedRequest, savedBundle)
    cancelled.confirmNative.mockResolvedValueOnce(false)
    cancelled.protectedRuntimeRequest.mockImplementation(async (call: ProtectedRecoveryRuntimeCall) => {
      if (call.operation === 'recover-protected-recovery') {
        return { ok: true, status: 'reconfirmation_required', entryCount: 1 }
      }
      if (call.operation === 'rollback-protected-recovery') return { ok: true, status: 'rolled_back' }
      if (call.operation === 'apply-protected-recovery-bundle') {
        throw new Error('apply must remain unreachable after cancelled confirmation')
      }
      return { ok: true, status: 'destination-confirmed' }
    })
    await expect(cancelled.authority.applyDestinationBundle(
      invokeEvent(cancelled.destinationWindow, cancelled.destinationFrame)
    )).resolves.toEqual({ ok: false, code: 'cancelled' })
    expect(cancelled.protectedRuntimeRequest.mock.calls.map(([call]) => call.operation)).toEqual([
      'recover-protected-recovery'
    ])
    expect(cancelled.confirmNative).toHaveBeenCalledTimes(1)

    const drifted = authorityFixture()
    drifted.getPortableImportReceipt.mockReturnValue(null as never)
    drifted.readBuffers.splice(0, drifted.readBuffers.length, Buffer.from(requestBytes), Buffer.from(bundleBytes))
    drifted.protectedRuntimeRequest.mockImplementation(async (call: ProtectedRecoveryRuntimeCall) => {
      if (call.operation === 'recover-protected-recovery') {
        return { ok: false, code: 'invalid_request' }
      }
      if (call.operation === 'apply-protected-recovery-bundle') {
        throw new Error('owner/fence drift must fail before destination apply')
      }
      return { ok: true, status: 'destination-confirmed' }
    })
    await expect(drifted.authority.applyDestinationBundle(
      invokeEvent(drifted.destinationWindow, drifted.destinationFrame)
    )).resolves.toEqual({ ok: false, code: 'invalid_request' })
    expect(drifted.protectedRuntimeRequest.mock.calls.map(([call]) => call.operation)).toEqual([
      'recover-protected-recovery'
    ])
  })
})
