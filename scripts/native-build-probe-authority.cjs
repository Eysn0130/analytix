'use strict'

const crypto = require('node:crypto')
const { spawnSync } = require('node:child_process')
const {
  chmodSync,
  closeSync,
  constants,
  fchmodSync,
  fstatSync,
  fsyncSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  readdirSync,
  readSync,
  rmSync
} = require('node:fs')
const { tmpdir } = require('node:os')
const { isAbsolute, join, relative, resolve } = require('node:path')
const {
  hermeticGoBuildEnvironment,
  resolvePinnedGoToolchain
} = require('./go-runtime-build-contract.cjs')
const { parseStrictJsonObject } = require('./lib/strict-json.cjs')
const { projectGoDevelopmentCacheEnvironment } = require('./lib/development-cache-environment.cjs')
const {
  darwinAuthorityLaunchGateIdentity,
  invokeDarwinAuthorityPinnedV1
} = require('./darwin-authority-launch-gate.cjs')

const BUILD_PROBE_PROTOCOL = 'analytix-native-build-probe-v1'
const BUILD_PROBE_REQUEST_LIMIT = 4 * 1024
const BUILD_PROBE_RESPONSE_LIMIT = 8 * 1024
const BUILD_COORDINATOR_RESPONSE_LIMIT = 16 * 1024
const GENERATION_PUBLISHER_RESPONSE_LIMIT = 16 * 1024
const SOURCE_SNAPSHOT_RESPONSE_LIMIT = 64 * 1024
const SOURCE_SNAPSHOT_CONTAINER = '/private/tmp'
const FROZEN_NATIVE_MANIFEST_SHA256 = '7265c3508f16731b5f8dbb1e08f7d645a2e8ab6d7bf4a35cf64c887e0c129aa4'
const MAX_EXECUTABLE_BYTES = 2 * 1024 * 1024 * 1024
const SHA256 = /^[0-9a-f]{64}$/u
const RECEIPT_FIELDS = Object.freeze([
  'kind',
  'schema_version',
  'status',
  'component_id',
  'request_nonce',
  'executable_sha256',
  'executable_size',
  'manifest_sha256',
  'policy_sha256',
  'authority_sha256',
  'host_platform',
  'host_arch',
  'loaded_image_bound',
  'working_directory_bound',
  'guardian_authenticated',
  'process_tree_empty'
])
const BUILD_COORDINATOR_RESPONSE_FIELDS = Object.freeze([
  'kind',
  'schema_version',
  'status',
  'request_nonce',
  'blocker',
  'generation_id',
  'inventory_sha256',
  'generation_receipt_sha256',
  'component_receipt_sha256',
  'publication_binding_sha256'
])
const GENERATION_RESPONSE_FIELDS = Object.freeze([
  'kind',
  'schema_version',
  'status',
  'request_nonce',
  'publication_binding_sha256',
  'target_key',
  'generation_id',
  'inventory_sha256',
  'generation_receipt_sha256',
  'component_receipt_sha256',
  'manifest_sha256',
  'authority_sha256',
  'previous_generation_id',
  'cleanup_recovered',
  'cleanup_pending'
])
const GENERATION_REJECTION_FIELDS = Object.freeze([
  'kind',
  'schema_version',
  'status',
  'request_nonce',
  'blocker'
])
const GENERATION_REJECTION_BLOCKERS = new Set([
  'request_frame_invalid',
  'request_invalid',
  'descriptor_input_invalid',
  'authority_identity_unavailable',
  'authority_target_invalid',
  'cargo_execution_ineligible',
  'publication_request_invalid',
  'publication_input_invalid',
  'publication_probe_failed',
  'publication_commit_failed',
  'publication_deadline_exceeded',
  'publication_failed',
  'response_encoding_failed'
])
const SOURCE_SNAPSHOT_RESPONSE_FIELDS = Object.freeze([
  'kind',
  'schema_version',
  'status',
  'operation',
  'request_nonce',
  'session_name',
  'manifest_sha256',
  'generation_id',
  'inventory_sha256',
  'generation_receipt_sha256',
  'file_count',
  'total_bytes',
  'components'
])
const SOURCE_SNAPSHOT_DISCARD_RESPONSE_FIELDS = Object.freeze([
  'kind',
  'schema_version',
  'status',
  'operation',
  'request_nonce',
  'session_name',
  'disposition',
  'generation_id',
  'inventory_sha256',
  'generation_receipt_sha256',
  'session_removed'
])
const SOURCE_SNAPSHOT_REJECTION_FIELDS = Object.freeze([
  'kind',
  'schema_version',
  'status',
  'operation',
  'request_nonce',
  'blocker'
])
const SOURCE_SNAPSHOT_COMPONENT_FIELDS = Object.freeze([
  'id',
  'source_root',
  'source_digest',
  'cargo_lock_sha256',
  'file_count',
  'total_bytes'
])
const SOURCE_SNAPSHOT_REJECTION_BLOCKERS = new Set([
  'request_frame_invalid',
  'request_invalid',
  'descriptor_input_invalid',
  'snapshot_generation_mismatch',
  'snapshot_source_changed',
  'snapshot_publication_failed',
  'snapshot_destroy_failed',
  'snapshot_input_invalid',
  'snapshot_deadline_exceeded',
  'snapshot_failed'
])
const SOURCE_SNAPSHOT_COMPONENTS = Object.freeze([
  Object.freeze({ id: 'import-accelerator', sourceRoot: 'tools/import_accelerator' }),
  Object.freeze({ id: 'cleaning-ops', sourceRoot: 'tools/cleaning_ops' }),
  Object.freeze({ id: 'analysis-compute', sourceRoot: 'tools/analysis_compute' }),
  Object.freeze({ id: 'data-engine', sourceRoot: 'tools/data_engine' })
])

let cachedAuthority = null
let cleanupRegistered = false

function runNativeBuildProbe(executablePath, component, manifestSHA256) {
  if (process.platform !== 'darwin') {
    throw new Error('[data-native] Native build probe authority is unavailable on this host')
  }
  if (
    typeof executablePath !== 'string' || !isAbsolute(executablePath) || executablePath.includes('\0') ||
    !component || typeof component !== 'object' || Array.isArray(component) ||
    !validComponentID(component.id) || !validProbePolicy(component.executionProbe) ||
    !SHA256.test(String(manifestSHA256 || ''))
  ) {
    throw new Error('[data-native] Native build probe request is invalid')
  }

  const authority = nativeBuildProbeAuthority()
  assertAuthorityUnchanged(authority)
  const target = openExecutable(executablePath)
  try {
    const before = descriptorIdentity(target)
    const executableSHA256 = sha256Descriptor(target, before.size)
    const request = {
      kind: 'analytix_native_build_probe_request',
      schema_version: 1,
      component_id: component.id,
      manifest_sha256: manifestSHA256,
      expected_executable_sha256: executableSHA256,
      expected_executable_size: before.size,
      request_nonce: crypto.randomBytes(32).toString('hex')
    }
    const input = Buffer.from(`${JSON.stringify(request)}\n`, 'utf8')
    if (input.length === 0 || input.length > BUILD_PROBE_REQUEST_LIMIT) {
      throw new Error('[data-native] Native build probe request is invalid')
    }
    const result = invokeDarwinAuthorityPinnedV1(
      'build_probe',
      authority.descriptor,
      authority.sha256,
      authority.identity.size,
      input,
      [target]
    )
    if (
      result.exitCode !== 0 || result.signal !== 0 ||
      !Buffer.isBuffer(result.stdout) || !Buffer.isBuffer(result.stderr) ||
      result.stderr.length !== 0
    ) {
      throw new Error(`[data-native] Native build probe failed for ${component.id}`)
    }
    const receipt = decodeReceiptFrame(result.stdout)
    validateReceipt(receipt, request, component.executionProbe.policySha256, authority.sha256)

    const after = descriptorIdentity(target)
    if (!sameDescriptorIdentity(before, after) || sha256Descriptor(target, after.size) !== executableSHA256) {
      throw new Error(`[data-native] Native build probe target changed for ${component.id}`)
    }
    assertAuthorityUnchanged(authority)
    return receipt
  } finally {
    closeSync(target)
  }
}

function nativeBuildProbeAuthorityIdentity() {
  const authority = nativeBuildProbeAuthority()
  assertAuthorityUnchanged(authority)
  return Object.freeze({ ...authority.receipt })
}

// Runs the whole formal native build lifecycle inside one pinned Go process.
// The request deliberately contains routing only: no Cargo receipt, release
// eligibility assertion, publication permit, signer identity, or component
// output can cross this boundary from Node.
function runNativeBuildCoordinator({ repositoryRoot, publicationRoot, targetKey }) {
  if (
    process.platform !== 'darwin' ||
    typeof repositoryRoot !== 'string' || !isAbsolute(repositoryRoot) || repositoryRoot.includes('\0') ||
    typeof publicationRoot !== 'string' || !isAbsolute(publicationRoot) || publicationRoot.includes('\0') ||
    !['darwin-arm64', 'darwin-x64'].includes(targetKey)
  ) {
    throw new Error('[data-native] Native build coordinator request is invalid')
  }
  const request = {
    kind: 'analytix_native_build_coordinator_request',
    schema_version: 1,
    request_nonce: crypto.randomBytes(32).toString('hex'),
    repository_root: repositoryRoot,
    publication_root: publicationRoot,
    target_key: targetKey
  }
  const input = Buffer.from(`${JSON.stringify(request)}\n`, 'utf8')
  if (input.length <= 1 || input.length > 8 * 1024) {
    throw new Error('[data-native] Native build coordinator request is invalid')
  }
  const authority = nativeBuildProbeAuthority()
  assertAuthorityUnchanged(authority)
  const result = invokeDarwinAuthorityPinnedV1(
    'build_coordinator',
    authority.descriptor,
    authority.sha256,
    authority.identity.size,
    input,
    []
  )
  if (
    result.signal !== 0 || !Buffer.isBuffer(result.stdout) || !Buffer.isBuffer(result.stderr) ||
    result.stderr.length !== 0 || result.stdout.length <= 1 ||
    result.stdout.length > BUILD_COORDINATOR_RESPONSE_LIMIT ||
    result.stdout[result.stdout.length - 1] !== 0x0a
  ) {
    throw new Error('[data-native] Native build coordinator execution failed')
  }
  const body = result.stdout.subarray(0, result.stdout.length - 1)
  const response = parseStrictJsonObject(body, {
    maxBytes: BUILD_COORDINATOR_RESPONSE_LIMIT,
    maxDepth: 2,
    maxTokens: 64,
    maxStringBytes: 4096,
    maxNumberBytes: 8,
    integerOnly: true
  })
  if (
    `${JSON.stringify(response)}\n` !== result.stdout.toString('utf8') ||
    !validBuildCoordinatorResponse(response, request.request_nonce)
  ) {
    throw new Error('[data-native] Native build coordinator response is invalid')
  }
  if (result.exitCode !== 0 || response.status !== 'published') {
    if (
      result.exitCode === 1 && ['blocked', 'failed'].includes(response.status) &&
      response.blocker !== '' && response.generation_id === '' && response.inventory_sha256 === '' &&
      response.generation_receipt_sha256 === '' && response.component_receipt_sha256 === '' &&
      response.publication_binding_sha256 === ''
    ) {
      throw new Error(`[data-native] Native release build blocked: ${response.blocker}`)
    }
    throw new Error('[data-native] Native build coordinator execution failed')
  }
  if (
    response.blocker !== '' || ![
      'generation_id',
      'inventory_sha256',
      'generation_receipt_sha256',
      'component_receipt_sha256',
      'publication_binding_sha256'
    ].every((field) => SHA256.test(String(response[field])))
  ) {
    throw new Error('[data-native] Native build coordinator publication receipt is invalid')
  }
  assertAuthorityUnchanged(authority)
  return Object.freeze({ ...response })
}

function validBuildCoordinatorResponse(response, requestNonce) {
  return exactKeys(response, BUILD_COORDINATOR_RESPONSE_FIELDS) &&
    response.kind === 'analytix_native_build_coordinator_response' && response.schema_version === 1 &&
    ['blocked', 'failed', 'published'].includes(response.status) &&
    response.request_nonce === requestNonce &&
    typeof response.blocker === 'string' && response.blocker.length <= 128 &&
    [
      'generation_id',
      'inventory_sha256',
      'generation_receipt_sha256',
      'component_receipt_sha256',
      'publication_binding_sha256'
    ].every((field) => response[field] === '' || SHA256.test(String(response[field])))
}

// Opens one private source-snapshot session. Source-reading descriptors and
// cleanup descriptors remain pinned for the session lifetime. The returned
// path is only a Cargo convenience; the receipts and descriptor bindings are
// authoritative.
function openNativeSourceSnapshotSession(manifestInputPath, repositoryPath) {
  if (
    process.platform !== 'darwin' || typeof manifestInputPath !== 'string' ||
    !isAbsolute(manifestInputPath) || manifestInputPath.includes('\0') ||
    typeof repositoryPath !== 'string' || !isAbsolute(repositoryPath) || repositoryPath.includes('\0')
  ) {
    throw new Error('[data-native] Native source snapshot session request is invalid')
  }
  const authority = nativeBuildProbeAuthority()
  assertAuthorityUnchanged(authority)
  const descriptors = { manifest: -1, repository: -1, parent: -1, container: -1 }
  let parentPath = ''
  try {
    descriptors.manifest = openRegularInput(manifestInputPath)
    descriptors.repository = openDirectoryInput(repositoryPath, false)
    descriptors.container = openSessionContainerInput(SOURCE_SNAPSHOT_CONTAINER)
    parentPath = mkdtempSync(join(SOURCE_SNAPSHOT_CONTAINER, 'analytix-native-source-session-'))
    chmodSync(parentPath, 0o700)
    descriptors.parent = openDirectoryInput(parentPath, true)
  } catch (error) {
    const parentPinned = descriptors.parent >= 0
    let closeError = null
    for (const [name, descriptor] of Object.entries(descriptors).reverse()) {
      if (descriptor < 0) continue
      descriptors[name] = -1
      try {
        closeSync(descriptor)
      } catch (descriptorError) {
        closeError ||= descriptorError
      }
    }
    if (closeError || parentPath !== '' && !parentPinned) {
      throw new Error('[data-native] Native source snapshot session initialization is indeterminate')
    }
    throw error
  }
  const sessionName = parentPath.slice(SOURCE_SNAPSHOT_CONTAINER.length + 1)
  return createSourceSnapshotSessionControllerV1({
    sessionName,
    currentRoot: join(parentPath, 'source', 'current'),
    sourceDescriptors: [
      descriptors.manifest,
      descriptors.repository,
      descriptors.parent,
      descriptors.container
    ],
    cleanupDescriptors: [descriptors.parent, descriptors.container],
    invoke(profile, input, inheritedFds) {
      assertAuthorityUnchanged(authority)
      const result = invokeDarwinAuthorityPinnedV1(
        profile,
        authority.descriptor,
        authority.sha256,
        authority.identity.size,
        input,
        inheritedFds
      )
      assertAuthorityUnchanged(authority)
      return result
    },
    closeDescriptors() {
      let closeError = null
      for (const [name, descriptor] of Object.entries(descriptors).reverse()) {
        if (descriptor < 0) continue
        descriptors[name] = -1
        try {
          closeSync(descriptor)
        } catch (error) {
          closeError ||= error
        }
      }
      if (closeError) throw new Error('[data-native] Native source snapshot descriptor cleanup failed')
    },
    nonce: () => crypto.randomBytes(32).toString('hex')
  })
}

function createSourceSnapshotSessionControllerV1(options) {
  if (
    !options || typeof options !== 'object' || Array.isArray(options) ||
    typeof options.sessionName !== 'string' ||
    !/^analytix-native-source-session-[A-Za-z0-9]{6}$/u.test(options.sessionName) ||
    typeof options.currentRoot !== 'string' || !isAbsolute(options.currentRoot) ||
    !Array.isArray(options.sourceDescriptors) || options.sourceDescriptors.length !== 4 ||
    !Array.isArray(options.cleanupDescriptors) || options.cleanupDescriptors.length !== 2 ||
    options.sourceDescriptors.some((value, index) =>
      !Number.isInteger(value) || value < 0 || options.sourceDescriptors.indexOf(value) !== index) ||
    options.cleanupDescriptors[0] !== options.sourceDescriptors[2] ||
    options.cleanupDescriptors[1] !== options.sourceDescriptors[3] ||
    typeof options.invoke !== 'function' || typeof options.closeDescriptors !== 'function' ||
    typeof options.nonce !== 'function'
  ) {
    throw new Error('[data-native] Native source snapshot controller is invalid')
  }
  let evidenceState = 'new'
  let cleanupState = 'required_unknown'
  let created = null
  let closed = false

  const requestFor = (operation) => {
    const known = ['verify', 'discard'].includes(operation)
    return {
      kind: 'analytix_native_source_snapshot_request',
      schema_version: 1,
      operation,
      request_nonce: options.nonce(),
      expected_generation_id: known ? created?.generation_id || '' : '',
      expected_inventory_sha256: known ? created?.inventory_sha256 || '' : '',
      expected_generation_receipt_sha256: known ? created?.generation_receipt_sha256 || '' : '',
      session_name: options.sessionName
    }
  }

  const invokeOperation = (operation) => {
    if (closed || !['create', 'verify', 'discard', 'reconcile_discard'].includes(operation)) {
      throw new Error('[data-native] Native source snapshot session state is invalid')
    }
    if (
      operation === 'create' && (evidenceState !== 'new' || created !== null) ||
      operation === 'verify' && (evidenceState !== 'created' || created === null) ||
      operation === 'discard' && (!['created', 'verified'].includes(evidenceState) || created === null) ||
      operation === 'reconcile_discard' && cleanupState === 'clean'
    ) {
      throw new Error('[data-native] Native source snapshot session state is invalid')
    }
    if (operation === 'create') {
      evidenceState = 'create_pending'
      cleanupState = 'required_unknown'
    } else if (operation === 'verify') {
      evidenceState = 'verify_pending'
    } else if (operation === 'discard') {
      evidenceState = 'discard_pending'
      cleanupState = 'required_unknown'
    }
    const request = requestFor(operation)
    const input = Buffer.from(`${JSON.stringify(request)}\n`, 'utf8')
    if (input.length <= 1 || input.length > BUILD_PROBE_REQUEST_LIMIT) {
      evidenceState = 'invalid'
      throw new Error('[data-native] Native source snapshot request is invalid')
    }
    const profile = ['create', 'verify'].includes(operation)
      ? 'source_snapshot'
      : 'source_snapshot_discard'
    const inheritedFds = profile === 'source_snapshot'
      ? options.sourceDescriptors
      : options.cleanupDescriptors
    let transportCompleted = false
    try {
      const result = options.invoke(profile, input, inheritedFds)
      transportCompleted = true
      if (
        !result || typeof result !== 'object' || result.signal !== 0 ||
        !Buffer.isBuffer(result.stdout) || !Buffer.isBuffer(result.stderr) || result.stderr.length !== 0
      ) {
        throw snapshotSessionError('[data-native] Native source snapshot authority failed', true)
      }
      const response = decodeSourceSnapshotResponse(result.stdout)
      if (result.exitCode !== 0) {
        if (
          exactKeys(response, SOURCE_SNAPSHOT_REJECTION_FIELDS) &&
          response.kind === 'analytix_native_source_snapshot_response' && response.schema_version === 1 &&
          response.status === 'rejected' && response.operation === operation &&
          response.request_nonce === request.request_nonce && SOURCE_SNAPSHOT_REJECTION_BLOCKERS.has(response.blocker)
        ) {
          throw snapshotSessionError(`[data-native] Native source snapshot rejected: ${response.blocker}`, false)
        }
        throw snapshotSessionError('[data-native] Native source snapshot authority failed', false)
      }
      if (operation === 'create' || operation === 'verify') {
        validateSourceSnapshotResponse(response, request)
        if (operation === 'create') {
          created = freezeSourceSnapshotResponse(response)
          evidenceState = 'created'
          cleanupState = 'required_known'
        } else if (!sameSourceSnapshotReceipt(created, response)) {
          throw snapshotSessionError('[data-native] Native source snapshot verification receipt mismatch', true)
        } else {
          evidenceState = 'verified'
          cleanupState = 'required_known'
        }
        return freezeSourceSnapshotResponse(response)
      }
      validateSourceSnapshotDiscardResponse(response, request)
      cleanupState = 'clean'
      evidenceState = operation === 'discard' ? 'discarded' : 'invalid'
      return Object.freeze({ ...response })
    } catch (error) {
      if (operation !== 'reconcile_discard') evidenceState = 'invalid'
      if (operation === 'verify' && created !== null) cleanupState = 'required_known'
      if (operation === 'create' || operation === 'discard') cleanupState = 'required_unknown'
      if (
        transportCompleted && error instanceof Error &&
        error.snapshotReconcileRetrySafe === undefined
      ) {
        Object.defineProperty(error, 'snapshotReconcileRetrySafe', { value: true })
      }
      throw error
    }
  }

  const cleanup = () => {
    if (closed) throw new Error('[data-native] Native source snapshot session state is invalid')
    if (cleanupState === 'clean') return null
    let priorError = null
    if (cleanupState === 'required_known' && created !== null) {
      try {
        return invokeOperation('discard')
      } catch (error) {
        priorError = error
      }
    }
    for (let attempt = 0; attempt < 2; attempt += 1) {
      try {
        return invokeOperation('reconcile_discard')
      } catch (error) {
        priorError = error
        if (error?.snapshotReconcileRetrySafe !== true) break
      }
    }
    throw priorError || new Error('[data-native] Native source snapshot cleanup failed')
  }

  return Object.freeze({
    currentRoot: options.currentRoot,
    create: () => invokeOperation('create'),
    verify: () => invokeOperation('verify'),
    discard: () => invokeOperation('discard'),
    cleanup,
    close: () => {
      if (closed) return
      if (cleanupState !== 'clean') {
        throw new Error('[data-native] Native source snapshot cleanup is required before close')
      }
      options.closeDescriptors()
      closed = true
    }
  })
}

function snapshotSessionError(message, retrySafe) {
  const error = new Error(message)
  Object.defineProperty(error, 'snapshotReconcileRetrySafe', { value: retrySafe === true })
  return error
}

function publishNativeComponentGeneration(request, manifestInputPath, componentInputPaths) {
  if (
    process.platform !== 'darwin' || !request || typeof request !== 'object' || Array.isArray(request) ||
    typeof manifestInputPath !== 'string' || !isAbsolute(manifestInputPath) || manifestInputPath.includes('\0') ||
    !Array.isArray(componentInputPaths) || componentInputPaths.length !== 4 ||
    componentInputPaths.some((value) => typeof value !== 'string' || !isAbsolute(value) || value.includes('\0'))
  ) {
    throw new Error('[data-native] Native generation publication request is invalid')
  }
  const input = Buffer.from(`${JSON.stringify(request)}\n`, 'utf8')
  if (input.length <= 1 || input.length > 256 * 1024) {
    throw new Error('[data-native] Native generation publication request is invalid')
  }
  const authority = nativeBuildProbeAuthority()
  assertAuthorityUnchanged(authority)
  const descriptors = []
  try {
    descriptors.push(openRegularInput(manifestInputPath))
    for (const path of componentInputPaths) descriptors.push(openExecutable(path))
    const result = invokeDarwinAuthorityPinnedV1(
      'generation_publish',
      authority.descriptor,
      authority.sha256,
      authority.identity.size,
      input,
      descriptors
    )
    if (
      result.signal !== 0 || !Buffer.isBuffer(result.stdout) ||
      !Buffer.isBuffer(result.stderr) || result.stderr.length !== 0
    ) {
      throw new Error('[data-native] Native generation publication failed')
    }
    const response = decodeGenerationResponse(result.stdout)
    if (result.exitCode !== 0) {
      if (
        exactKeys(response, GENERATION_REJECTION_FIELDS) &&
        response.kind === 'analytix_native_generation_publish_response' &&
        response.schema_version === 1 && response.status === 'rejected' &&
        (response.request_nonce === '' || response.request_nonce === request.request_nonce) &&
        GENERATION_REJECTION_BLOCKERS.has(response.blocker)
      ) {
        throw new Error(`[data-native] Native generation publication rejected: ${response.blocker}`)
      }
      throw new Error('[data-native] Native generation publication failed')
    }
    if (
      response.kind !== 'analytix_native_generation_publish_response' || response.schema_version !== 1 ||
      response.status !== 'committed' || response.request_nonce !== request.request_nonce ||
      response.target_key !== request.target_key || response.manifest_sha256 !== FROZEN_NATIVE_MANIFEST_SHA256 ||
      response.authority_sha256 !== authority.sha256 ||
      !['publication_binding_sha256', 'generation_id', 'inventory_sha256', 'generation_receipt_sha256', 'component_receipt_sha256', 'manifest_sha256']
        .every((field) => SHA256.test(String(response[field] || ''))) ||
      (response.previous_generation_id !== '' && !SHA256.test(String(response.previous_generation_id))) ||
      typeof response.cleanup_recovered !== 'boolean' || typeof response.cleanup_pending !== 'boolean'
    ) {
      throw new Error('[data-native] Native generation publication receipt mismatch')
    }
    assertAuthorityUnchanged(authority)
    return Object.freeze({ ...response })
  } finally {
    for (const descriptor of descriptors) closeSync(descriptor)
  }
}

function nativeBuildProbeAuthority() {
  if (cachedAuthority !== null) return cachedAuthority
  if (process.platform !== 'darwin' || !['arm64', 'x64'].includes(process.arch)) {
    throw new Error('[data-native] Native build probe authority is unavailable on this host')
  }
  const projectedGoCache = projectGoDevelopmentCacheEnvironment(process.env)
  const toolchain = resolvePinnedGoToolchain({
    env: projectedGoCache.environment,
    authorizedModuleCache: projectedGoCache.authorizedModuleCache
  })
  const launchGate = darwinAuthorityLaunchGateIdentity()
  const moduleRoot = join(__dirname, '..', 'packages', 'runtime-go')
  const sourceSetSHA256 = authoritySourceSetDigest(moduleRoot)
  const root = mkdtempSync(join(tmpdir(), 'analytix-native-build-probe-authority-'))
  chmodSync(root, 0o700)
  const home = join(root, 'home')
  const cache = join(root, 'cache')
  const temporary = join(root, 'tmp')
  for (const directory of [home, cache, temporary]) mkdirSync(directory, { mode: 0o700 })
  const output = join(root, 'native-component-build-probe')
  const goarch = process.arch === 'x64' ? 'amd64' : 'arm64'
  const environment = hermeticGoBuildEnvironment(
    toolchain,
    { goos: 'darwin', goarch },
    { path: home, cache, temp: temporary }
  )
  const buildEnvironmentSHA256 = canonicalAuthorityBuildEnvironmentDigest(
    environment,
    { home, cache, temporary },
    launchGate
  )
  let descriptor
  try {
    const verify = spawnSync(
      toolchain.executable,
      ['mod', 'verify'],
      {
        cwd: moduleRoot,
        env: environment,
        stdio: ['ignore', 'pipe', 'pipe'],
        timeout: 120_000,
        maxBuffer: 1024 * 1024,
        killSignal: 'SIGKILL',
        windowsHide: true
      }
    )
    if (verify.error || verify.status !== 0 || verify.signal !== null) {
      throw new Error('[data-native] Native build probe authority dependency verification failed')
    }
    descriptor = openSync(
      output,
      constants.O_CREAT | constants.O_EXCL | constants.O_RDWR | constants.O_NOFOLLOW,
      0o600
    )
    const result = spawnSync(
      toolchain.executable,
      [
        'build',
        '-mod=readonly',
        '-buildvcs=false',
        '-trimpath',
        '-tags',
        'analytix_native_build_probe',
        '-o',
        '/dev/fd/3',
        './cmd/native-component-build-probe'
      ],
      {
        cwd: moduleRoot,
        env: environment,
        stdio: ['ignore', 'pipe', 'pipe', descriptor],
        timeout: 120_000,
        maxBuffer: 1024 * 1024,
        killSignal: 'SIGKILL',
        windowsHide: true
      }
    )
    if (result.error || result.status !== 0 || result.signal !== null) {
      throw new Error('[data-native] Native build probe authority build failed')
    }
    fsyncSync(descriptor)
    fchmodSync(descriptor, 0o700)
    fsyncSync(descriptor)
    if (authoritySourceSetDigest(moduleRoot) !== sourceSetSHA256) {
      throw new Error('[data-native] Native build probe authority source changed during build')
    }
    const identity = descriptorIdentity(descriptor)
    if ((identity.mode & 0o7777) !== 0o700) {
      throw new Error('[data-native] Native build probe authority mode is invalid')
    }
    const sha256 = sha256Descriptor(descriptor, identity.size)
    assertPathIdentity(output, identity)
    const receipt = Object.freeze({
      schemaVersion: 1,
      trustClass: 'local_provisional',
      protocol: BUILD_PROBE_PROTOCOL,
      targetKey: `darwin-${normalizeArch(process.arch)}`,
      binarySha256: sha256,
      binarySize: identity.size,
      sourceSetSha256: sourceSetSHA256,
      buildEnvironmentSha256: buildEnvironmentSHA256,
      goToolchainKey: toolchain.key,
      goExecutableSha256: toolchain.lock.goExecutableSha256
    })
    cachedAuthority = Object.freeze({ path: output, root, descriptor, identity, sha256, receipt })
    descriptor = undefined
    registerAuthorityCleanup()
    return cachedAuthority
  } catch (error) {
    if (descriptor !== undefined) {
      try {
        closeSync(descriptor)
      } catch {
        // Preserve the primary probe failure during best-effort cleanup.
      }
    }
    rmSync(root, { recursive: true, force: true })
    throw error
  }
}

function registerAuthorityCleanup() {
  if (cleanupRegistered) return
  cleanupRegistered = true
  process.once('exit', () => {
    const authority = cachedAuthority
    cachedAuthority = null
    if (authority !== null) {
      try {
        closeSync(authority.descriptor)
      } catch {
        // Process-exit cleanup cannot recover from a close failure.
      }
      rmSync(authority.root, { recursive: true, force: true })
    }
  })
}

function assertAuthorityUnchanged(authority) {
  if (!authority || typeof authority !== 'object') {
    throw new Error('[data-native] Native build probe authority is invalid')
  }
  const identity = descriptorIdentity(authority.descriptor)
  if (
    !sameDescriptorIdentity(identity, authority.identity) ||
    (identity.mode & 0o7777) !== 0o700 ||
    sha256Descriptor(authority.descriptor, identity.size) !== authority.sha256
  ) {
    throw new Error('[data-native] Native build probe authority identity changed')
  }
  assertPathIdentity(authority.path, identity)
}

function openExecutable(path) {
  let descriptor
  try {
    descriptor = openSync(path, constants.O_RDONLY | constants.O_CLOEXEC | constants.O_NOFOLLOW)
    descriptorIdentity(descriptor)
    return descriptor
  } catch (error) {
    if (descriptor !== undefined) closeSync(descriptor)
    throw new Error('[data-native] Native build probe executable is unsafe')
  }
}

function openRegularInput(path) {
  let descriptor
  try {
    descriptor = openSync(path, constants.O_RDONLY | constants.O_CLOEXEC | constants.O_NOFOLLOW)
    const stat = fstatSync(descriptor, { bigint: true })
    if (
      Number(stat.mode & BigInt(constants.S_IFMT)) !== constants.S_IFREG || stat.nlink !== 1n ||
      stat.size <= 0n || stat.size > BigInt(256 * 1024) ||
      (Number(stat.mode) & 0o7022) !== 0 ||
      (stat.uid !== 0n && stat.uid !== BigInt(process.geteuid()))
    ) {
      throw new Error('[data-native] Native generation manifest input is unsafe')
    }
    return descriptor
  } catch (error) {
    if (descriptor !== undefined) closeSync(descriptor)
    throw new Error('[data-native] Native generation manifest input is unsafe')
  }
}

function openDirectoryInput(path, privateExact) {
  let descriptor
  try {
    descriptor = openSync(
      path,
      constants.O_RDONLY | constants.O_CLOEXEC | constants.O_NOFOLLOW | constants.O_DIRECTORY
    )
    const stat = fstatSync(descriptor, { bigint: true })
    const mode = Number(stat.mode)
    if (
      Number(stat.mode & BigInt(constants.S_IFMT)) !== constants.S_IFDIR ||
      (privateExact
        ? stat.uid !== BigInt(process.geteuid()) || (mode & 0o7777) !== 0o700
        : (stat.uid !== 0n && stat.uid !== BigInt(process.geteuid())) || (mode & 0o7022) !== 0)
    ) {
      throw new Error('[data-native] Native source snapshot directory input is unsafe')
    }
    return descriptor
  } catch (error) {
    if (descriptor !== undefined) closeSync(descriptor)
    throw new Error('[data-native] Native source snapshot directory input is unsafe')
  }
}

function openSessionContainerInput(path) {
  let descriptor
  try {
    descriptor = openSync(
      path,
      constants.O_RDONLY | constants.O_CLOEXEC | constants.O_NOFOLLOW | constants.O_DIRECTORY
    )
    const stat = fstatSync(descriptor, { bigint: true })
    if (
      Number(stat.mode & BigInt(constants.S_IFMT)) !== constants.S_IFDIR || stat.uid !== 0n ||
      (Number(stat.mode) & 0o7777) !== 0o1777
    ) {
      throw new Error('[data-native] Native source snapshot container is unsafe')
    }
    return descriptor
  } catch (error) {
    if (descriptor !== undefined) closeSync(descriptor)
    throw new Error('[data-native] Native source snapshot container is unsafe')
  }
}

function descriptorIdentity(descriptor) {
  const stat = fstatSync(descriptor, { bigint: true })
  const identity = executableIdentityFromStat(stat)
  const type = Number(stat.mode & BigInt(constants.S_IFMT))
  if (
    type !== constants.S_IFREG || stat.nlink !== 1n ||
    !Number.isSafeInteger(identity.size) || identity.size <= 0 || identity.size > MAX_EXECUTABLE_BYTES ||
    (identity.mode & 0o022) !== 0 || (identity.mode & 0o111) === 0 ||
    (stat.uid !== 0n && stat.uid !== BigInt(process.geteuid()))
  ) {
    throw new Error('[data-native] Native build probe executable is unsafe')
  }
  return Object.freeze(identity)
}

function executableIdentityFromStat(stat) {
  return {
    dev: stat.dev,
    ino: stat.ino,
    mode: Number(stat.mode),
    uid: stat.uid,
    gid: stat.gid,
    nlink: stat.nlink,
    size: Number(stat.size),
    mtimeNs: stat.mtimeNs,
    ctimeNs: stat.ctimeNs
  }
}

function assertPathIdentity(path, expected) {
  const stat = lstatSync(path, { bigint: true })
  if (stat.isSymbolicLink() || !sameDescriptorIdentity(executableIdentityFromStat(stat), expected)) {
    throw new Error('[data-native] Native build probe authority path identity changed')
  }
}

function sameDescriptorIdentity(left, right) {
  return Boolean(left && right) &&
    left.dev === right.dev && left.ino === right.ino && left.mode === right.mode &&
    left.uid === right.uid && left.gid === right.gid && left.nlink === right.nlink &&
    left.size === right.size && left.mtimeNs === right.mtimeNs && left.ctimeNs === right.ctimeNs
}

function sha256Descriptor(descriptor, size) {
  if (!Number.isSafeInteger(size) || size <= 0 || size > MAX_EXECUTABLE_BYTES) {
    throw new Error('[data-native] Native build probe executable size is invalid')
  }
  const hash = crypto.createHash('sha256')
  const buffer = Buffer.allocUnsafe(1024 * 1024)
  let offset = 0
  while (offset < size) {
    const count = Math.min(buffer.length, size - offset)
    const read = readSync(descriptor, buffer, 0, count, offset)
    if (read <= 0 || read > count) {
      throw new Error('[data-native] Native build probe executable read failed')
    }
    hash.update(buffer.subarray(0, read))
    offset += read
  }
  const extra = readSync(descriptor, buffer, 0, 1, offset)
  if (extra !== 0) throw new Error('[data-native] Native build probe executable size changed')
  return hash.digest('hex')
}

function authoritySourceSetDigest(moduleRoot) {
  const canonicalRoot = resolve(moduleRoot)
  const rootStat = lstatSync(canonicalRoot)
  if (rootStat.isSymbolicLink() || !rootStat.isDirectory()) {
    throw new Error('[data-native] Native build probe authority source root is unsafe')
  }
  const files = []
  const visit = (directory) => {
    const entries = readdirSync(directory, { withFileTypes: true }).sort((left, right) =>
      Buffer.compare(Buffer.from(left.name, 'utf8'), Buffer.from(right.name, 'utf8'))
    )
    for (const entry of entries) {
      const path = join(directory, entry.name)
      if (entry.isSymbolicLink()) {
        throw new Error('[data-native] Native build probe authority source contains a symbolic link')
      }
      if (entry.isDirectory()) {
        // Directory names are not a build-input boundary. Go embed patterns
        // may consume files under target, vendor, dot-directories, or any
        // other ordinary directory, so the local module closure must include
        // every regular file rather than applying repository-cleanup rules.
        visit(path)
        continue
      }
      if (!entry.isFile()) {
        throw new Error('[data-native] Native build probe authority source contains a special file')
      }
      // Hash the complete local module tree, not an extension allowlist. Go
      // builds may consume assembly, cgo, syso, or arbitrarily named embed
      // inputs; omitting any regular file would let the same provenance digest
      // identify a different authority executable.
      files.push(path)
    }
  }
  visit(canonicalRoot)
  if (files.length === 0 || files.length > 20_000) {
    throw new Error('[data-native] Native build probe authority source inventory is invalid')
  }
  const hash = crypto.createHash('sha256')
  let total = 0
  for (const path of files) {
    const stat = lstatSync(path, { bigint: true })
    const size = Number(stat.size)
    if (
      Number(stat.mode & BigInt(constants.S_IFMT)) !== constants.S_IFREG || stat.nlink !== 1n ||
      !Number.isSafeInteger(size) || size < 0 || size > 64 * 1024 * 1024 ||
      (Number(stat.mode) & 0o022) !== 0 ||
      (stat.uid !== 0n && stat.uid !== BigInt(process.geteuid()))
    ) {
      throw new Error('[data-native] Native build probe authority source file is unsafe')
    }
    total += size
    if (total > 256 * 1024 * 1024) {
      throw new Error('[data-native] Native build probe authority source inventory is too large')
    }
    const name = relative(canonicalRoot, path).split('\\').join('/')
    const bodySHA256 = sha256AuthoritySourceFile(path, stat, size)
    hash.update(name)
    hash.update('\0')
    hash.update(String(size))
    hash.update('\0')
    hash.update(bodySHA256)
    hash.update('\0')
  }
  return hash.digest('hex')
}

function sha256AuthoritySourceFile(path, expected, size) {
  let descriptor
  try {
    descriptor = openSync(path, constants.O_RDONLY | constants.O_CLOEXEC | constants.O_NOFOLLOW)
    const before = fstatSync(descriptor, { bigint: true })
    if (!sameAuthoritySourceIdentity(before, expected) || Number(before.size) !== size) {
      throw new Error('[data-native] Native build probe authority source changed before hashing')
    }
    const hash = crypto.createHash('sha256')
    const buffer = Buffer.allocUnsafe(1024 * 1024)
    let offset = 0
    while (offset < size) {
      const length = Math.min(buffer.length, size - offset)
      const count = readSync(descriptor, buffer, 0, length, offset)
      if (count <= 0 || count > length) {
        throw new Error('[data-native] Native build probe authority source read failed')
      }
      hash.update(buffer.subarray(0, count))
      offset += count
    }
    const after = fstatSync(descriptor, { bigint: true })
    if (!sameAuthoritySourceIdentity(before, after)) {
      throw new Error('[data-native] Native build probe authority source changed while hashing')
    }
    return hash.digest('hex')
  } finally {
    if (descriptor !== undefined) closeSync(descriptor)
  }
}

function sameAuthoritySourceIdentity(left, right) {
  return left.dev === right.dev && left.ino === right.ino && left.mode === right.mode &&
    left.uid === right.uid && left.gid === right.gid && left.nlink === right.nlink &&
    left.size === right.size && left.mtimeNs === right.mtimeNs && left.ctimeNs === right.ctimeNs
}

function canonicalEnvironmentDigest(environment) {
  const keys = Object.keys(environment).sort((left, right) =>
    Buffer.compare(Buffer.from(left, 'utf8'), Buffer.from(right, 'utf8'))
  )
  const canonical = {}
  for (const key of keys) canonical[key] = String(environment[key])
  return crypto.createHash('sha256').update(JSON.stringify(canonical)).digest('hex')
}

function canonicalAuthorityBuildEnvironmentDigest(environment, directories, launchGate) {
  const expectedPaths = {
    HOME: directories.home,
    GOPATH: directories.home,
    GOCACHE: directories.cache,
    GOTMPDIR: directories.temporary
  }
  const canonical = { ...environment }
  for (const [key, expected] of Object.entries(expectedPaths)) {
    if (!isAbsolute(expected) || canonical[key] !== expected) {
      throw new Error('[data-native] Native build probe authority environment is not isolated')
    }
    canonical[key] = `$ANALYTIX_AUTHORITY_${key}`
  }
  if (launchGate !== undefined) {
    if (
      !launchGate || typeof launchGate !== 'object' || Array.isArray(launchGate) ||
      launchGate.schemaVersion !== 1 ||
      launchGate.protocol !== 'analytix-darwin-authority-launch-gate-v1' ||
      launchGate.trustClass !== 'local_provisional' ||
      !SHA256.test(String(launchGate.sourceSha256 || '')) ||
      !SHA256.test(String(launchGate.binarySha256 || '')) ||
      !SHA256.test(String(launchGate.toolchainSha256 || '')) ||
      !Number.isSafeInteger(launchGate.binarySize) || launchGate.binarySize <= 0 ||
      launchGate.napiVersion !== 1 || launchGate.descriptorLoaded !== true
    ) {
      throw new Error('[data-native] Native build probe launch gate identity is invalid')
    }
    canonical.ANALYTIX_DARWIN_AUTHORITY_LAUNCH_GATE = canonicalEnvironmentDigest({
      schemaVersion: launchGate.schemaVersion,
      protocol: launchGate.protocol,
      trustClass: launchGate.trustClass,
      hostKey: launchGate.hostKey,
      sourceSha256: launchGate.sourceSha256,
      binarySha256: launchGate.binarySha256,
      binarySize: launchGate.binarySize,
      toolchainSha256: launchGate.toolchainSha256,
      napiVersion: launchGate.napiVersion,
      descriptorLoaded: launchGate.descriptorLoaded
    })
  }
  return canonicalEnvironmentDigest(canonical)
}

function decodeReceiptFrame(stdout) {
  if (
    stdout.length <= 1 || stdout.length > BUILD_PROBE_RESPONSE_LIMIT || stdout.at(-1) !== 0x0a ||
    stdout.subarray(0, stdout.length - 1).includes(0x0a) || stdout.includes(0x0d) || stdout.includes(0)
  ) {
    throw new Error('[data-native] Native build probe receipt channel is invalid')
  }
  let receipt
  try {
    receipt = parseStrictJsonObject(stdout.subarray(0, stdout.length - 1), {
      maxBytes: BUILD_PROBE_RESPONSE_LIMIT - 1,
      maxDepth: 2,
      maxTokens: 64,
      maxStringBytes: 256,
      maxNumberBytes: 32,
      integerOnly: true
    })
  } catch {
    throw new Error('[data-native] Native build probe receipt is invalid')
  }
  if (!exactKeys(receipt, RECEIPT_FIELDS)) {
    throw new Error('[data-native] Native build probe receipt schema is invalid')
  }
  return receipt
}

function decodeGenerationResponse(stdout) {
  if (
    stdout.length <= 1 || stdout.length > GENERATION_PUBLISHER_RESPONSE_LIMIT || stdout.at(-1) !== 0x0a ||
    stdout.subarray(0, stdout.length - 1).includes(0x0a) || stdout.includes(0x0d) || stdout.includes(0)
  ) {
    throw new Error('[data-native] Native generation publication response channel is invalid')
  }
  let response
  try {
    response = parseStrictJsonObject(stdout.subarray(0, stdout.length - 1), {
      maxBytes: GENERATION_PUBLISHER_RESPONSE_LIMIT - 1,
      maxDepth: 2,
      maxTokens: 64,
      maxStringBytes: 256,
      maxNumberBytes: 32,
      integerOnly: true
    })
  } catch {
    throw new Error('[data-native] Native generation publication response is invalid')
  }
  if (
    !exactKeys(response, GENERATION_RESPONSE_FIELDS) &&
    !exactKeys(response, GENERATION_REJECTION_FIELDS)
  ) {
    throw new Error('[data-native] Native generation publication response schema is invalid')
  }
  return response
}

function decodeSourceSnapshotResponse(stdout) {
  if (
    stdout.length <= 1 || stdout.length > SOURCE_SNAPSHOT_RESPONSE_LIMIT || stdout.at(-1) !== 0x0a ||
    stdout.subarray(0, stdout.length - 1).includes(0x0a) || stdout.includes(0x0d) || stdout.includes(0)
  ) {
    throw new Error('[data-native] Native source snapshot response channel is invalid')
  }
  let response
  try {
    response = parseStrictJsonObject(stdout.subarray(0, stdout.length - 1), {
      maxBytes: SOURCE_SNAPSHOT_RESPONSE_LIMIT - 1,
      maxDepth: 4,
      maxTokens: 256,
      maxStringBytes: 256,
      maxNumberBytes: 32,
      integerOnly: true
    })
  } catch {
    throw new Error('[data-native] Native source snapshot response is invalid')
  }
  if (
    !exactKeys(response, SOURCE_SNAPSHOT_RESPONSE_FIELDS) &&
    !exactKeys(response, SOURCE_SNAPSHOT_DISCARD_RESPONSE_FIELDS) &&
    !exactKeys(response, SOURCE_SNAPSHOT_REJECTION_FIELDS)
  ) {
    throw new Error('[data-native] Native source snapshot response schema is invalid')
  }
  return response
}

function validateSourceSnapshotResponse(response, request) {
  const expectedStatus = { create: 'committed', verify: 'verified' }[request.operation]
  if (
    !exactKeys(response, SOURCE_SNAPSHOT_RESPONSE_FIELDS) ||
    response.kind !== 'analytix_native_source_snapshot_response' || response.schema_version !== 1 ||
    response.status !== expectedStatus || response.operation !== request.operation ||
    response.request_nonce !== request.request_nonce ||
    response.session_name !== request.session_name ||
    response.manifest_sha256 !== FROZEN_NATIVE_MANIFEST_SHA256 ||
    !['generation_id', 'inventory_sha256', 'generation_receipt_sha256']
      .every((field) => SHA256.test(String(response[field] || ''))) ||
    request.operation === 'verify' && (
      response.generation_id !== request.expected_generation_id ||
      response.inventory_sha256 !== request.expected_inventory_sha256 ||
      response.generation_receipt_sha256 !== request.expected_generation_receipt_sha256
    ) ||
    !Number.isSafeInteger(response.file_count) || response.file_count <= 0 || response.file_count > 4096 ||
    !Number.isSafeInteger(response.total_bytes) || response.total_bytes <= 0 || response.total_bytes > 128 * 1024 * 1024 ||
    !Array.isArray(response.components) || response.components.length !== SOURCE_SNAPSHOT_COMPONENTS.length
  ) {
    throw new Error('[data-native] Native source snapshot receipt mismatch')
  }
  let componentFiles = 0
  let componentBytes = 0
  for (let index = 0; index < SOURCE_SNAPSHOT_COMPONENTS.length; index += 1) {
    const expected = SOURCE_SNAPSHOT_COMPONENTS[index]
    const component = response.components[index]
    if (
      !exactKeys(component, SOURCE_SNAPSHOT_COMPONENT_FIELDS) || component.id !== expected.id ||
      component.source_root !== expected.sourceRoot || !SHA256.test(String(component.source_digest || '')) ||
      !SHA256.test(String(component.cargo_lock_sha256 || '')) ||
      !Number.isSafeInteger(component.file_count) || component.file_count <= 0 || component.file_count > 4094 ||
      !Number.isSafeInteger(component.total_bytes) || component.total_bytes <= 0 ||
      component.total_bytes > 128 * 1024 * 1024
    ) {
      throw new Error('[data-native] Native source snapshot component receipt mismatch')
    }
    componentFiles += component.file_count
    componentBytes += component.total_bytes
  }
  if (
    response.file_count < componentFiles + 2 || response.file_count > componentFiles + 4 ||
    response.total_bytes <= componentBytes
  ) {
    throw new Error('[data-native] Native source snapshot aggregate receipt mismatch')
  }
}

function validateSourceSnapshotDiscardResponse(response, request) {
  const generationDisposition = response?.disposition === 'generation_discarded'
  const emptyDisposition = ['empty_session_discarded', 'already_discarded'].includes(response?.disposition)
  if (
    !exactKeys(response, SOURCE_SNAPSHOT_DISCARD_RESPONSE_FIELDS) ||
    response.kind !== 'analytix_native_source_snapshot_response' || response.schema_version !== 1 ||
    response.status !== 'discarded' || response.operation !== request.operation ||
    response.request_nonce !== request.request_nonce || response.session_name !== request.session_name ||
    response.session_removed !== true ||
    !['discard', 'reconcile_discard'].includes(request.operation) ||
    request.operation === 'discard' && !generationDisposition ||
    request.operation === 'reconcile_discard' && !generationDisposition && !emptyDisposition ||
    generationDisposition && ![
      response.generation_id,
      response.inventory_sha256,
      response.generation_receipt_sha256
    ].every((value) => SHA256.test(String(value || ''))) ||
    emptyDisposition && [
      response.generation_id,
      response.inventory_sha256,
      response.generation_receipt_sha256
    ].some((value) => value !== '') ||
    request.operation === 'discard' && (
      response.generation_id !== request.expected_generation_id ||
      response.inventory_sha256 !== request.expected_inventory_sha256 ||
      response.generation_receipt_sha256 !== request.expected_generation_receipt_sha256
    )
  ) {
    throw new Error('[data-native] Native source snapshot discard receipt mismatch')
  }
}

function sameSourceSnapshotReceipt(created, verified) {
  if (!created || !verified) return false
  const comparable = (value) => ({
    manifest_sha256: value.manifest_sha256,
    session_name: value.session_name,
    generation_id: value.generation_id,
    inventory_sha256: value.inventory_sha256,
    generation_receipt_sha256: value.generation_receipt_sha256,
    file_count: value.file_count,
    total_bytes: value.total_bytes,
    components: value.components
  })
  return JSON.stringify(comparable(created)) === JSON.stringify(comparable(verified))
}

function freezeSourceSnapshotResponse(response) {
  return Object.freeze({
    ...response,
    components: Object.freeze(response.components.map((component) => Object.freeze({ ...component })))
  })
}

function validateReceipt(receipt, request, policySHA256, authoritySHA256) {
  if (
    receipt.kind !== 'analytix_native_build_probe_receipt' || receipt.schema_version !== 1 ||
    receipt.status !== 'passed' || receipt.component_id !== request.component_id ||
    receipt.request_nonce !== request.request_nonce ||
    receipt.executable_sha256 !== request.expected_executable_sha256 ||
    receipt.executable_size !== request.expected_executable_size ||
    receipt.manifest_sha256 !== request.manifest_sha256 ||
    receipt.policy_sha256 !== policySHA256 || receipt.authority_sha256 !== authoritySHA256 ||
    receipt.host_platform !== 'darwin' || receipt.host_arch !== normalizeArch(process.arch) ||
    receipt.loaded_image_bound !== true || receipt.working_directory_bound !== true ||
    receipt.guardian_authenticated !== true || receipt.process_tree_empty !== true
  ) {
    throw new Error(`[data-native] Native build probe receipt mismatch for ${request.component_id}`)
  }
}

function validProbePolicy(value) {
  return exactKeys(value, [
    'schemaVersion',
    'authorityProtocol',
    'componentProtocol',
    'policySha256',
    'authorityTargets'
  ]) && value.schemaVersion === 1 && value.authorityProtocol === BUILD_PROBE_PROTOCOL &&
    value.componentProtocol === 'analytix-native-v1' && SHA256.test(String(value.policySha256 || '')) &&
    Array.isArray(value.authorityTargets) && value.authorityTargets.length === 2 &&
    value.authorityTargets[0] === 'darwin-arm64' && value.authorityTargets[1] === 'darwin-x64'
}

function validComponentID(value) {
  return [
    'import-accelerator',
    'cleaning-ops',
    'analysis-compute',
    'data-engine'
  ].includes(value)
}

function normalizeArch(value) {
  if (value === 'arm64') return 'arm64'
  if (value === 'x64' || value === 'amd64') return 'x64'
  return ''
}

function exactKeys(value, expected) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const actual = Object.keys(value)
  if (actual.length !== expected.length) return false
  const keys = new Set(expected)
  return actual.every((key) => keys.has(key))
}

module.exports = {
  BUILD_PROBE_PROTOCOL,
  BUILD_PROBE_RESPONSE_LIMIT,
  RECEIPT_FIELDS,
  nativeBuildProbeAuthorityIdentity,
  openNativeSourceSnapshotSession,
  publishNativeComponentGeneration,
  runNativeBuildCoordinator,
  runNativeBuildProbe,
  _internals: {
    authoritySourceSetDigest,
    canonicalAuthorityBuildEnvironmentDigest,
    createSourceSnapshotSessionControllerV1,
    decodeSourceSnapshotResponse,
    sameSourceSnapshotReceipt,
    validBuildCoordinatorResponse,
    validateSourceSnapshotDiscardResponse,
    validateSourceSnapshotResponse
  }
}
