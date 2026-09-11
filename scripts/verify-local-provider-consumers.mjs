#!/usr/bin/env node

import { readFileSync, readdirSync, statSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const SCRIPT_PATH = fileURLToPath(import.meta.url)
const REPO_ROOT = resolve(dirname(SCRIPT_PATH), '..')

const ALLOWED_PROVIDER_NETWORK_OWNERS = new Set([
  // Readiness/runtime-host traffic. These owners may probe Provider metadata or
  // call the loopback Go runtime, but they are not ordinary model execution.
  'src/main/analytix-process.ts',
  'src/main/provider-connection.ts',
  'src/main/runtime/analytix-adapter.ts',
  // K8 transport/account lifecycle owners. Their tokens cannot authorize model egress.
  'src/main/telegram-runtime.ts',
  'src/main/weixin-bridge-runtime.ts',
  'src/main/services/hub-account-service.ts',
])

const ORDINARY_TS_CONSUMERS = [
  'src/main/claw-scheduled-task-detector.ts',
  'src/main/services/write-inline-completion-service.ts'
]

const MEDIA_TS_CONSUMERS = [
  'src/main/services/image-generation-client.ts',
  'src/main/services/write-infographic-service.ts',
  'src/main/services/speech-to-text-service.ts'
]

const FORBIDDEN_ORDINARY_CAPABILITIES = [
  ['direct fetch', /\b(?:fetch|net\.fetch)\s*\(/u],
  ['credential field', /\b(?:apiKey|credentialRef)\b/u],
  ['provider authorization header', /\b(?:Authorization|x-api-key)\b/u],
  ['copied provider route', /\b(?:baseUrl|endpointFormat|proxyUrl)\b/u],
  ['provider SDK', /(?:from\s+['"](?:openai|@anthropic-ai\/sdk)['"]|new\s+(?:OpenAI|Anthropic)\s*\()/u]
]

const FORBIDDEN_MEDIA_CAPABILITIES = [
  ['direct network', /\b(?:fetch|net\.fetch|https?\.request|undici\.request)\s*\(/u],
  ['provider credential', /\b(?:apiKey|credentialRef|Authorization|x-api-key)\b/u],
  ['provider route', /\b(?:baseUrl|endpointFormat|proxyUrl)\b/u],
  ['provider protocol', /\bprotocol\b/u],
  ['provider SDK', /(?:from\s+['"](?:openai|@anthropic-ai\/sdk)['"]|new\s+(?:OpenAI|Anthropic)\s*\()/u]
]

const NETWORK_CALL = /\b(?:fetch|net\.fetch|https?\.request|undici\.request)\s*\(/u
const PROVIDER_SDK_CALL = /(?:from\s+['"](?:openai|@anthropic-ai\/sdk)['"]|new\s+(?:OpenAI|Anthropic)\s*\()/u
const PROVIDER_EFFECT_MARKER = /(?:\bapiKey\b|x-api-key|chat\/completions|images\/(?:generations|edits)|audio\/transcriptions|image_generation|input_audio|max_(?:output_)?tokens|buildProviderHeaders|upstreamOpenAiModelEndpointUrl|new\s+(?:OpenAI|Anthropic)\s*\()/u

function productionTypeScriptFiles(root) {
  const start = join(root, 'src/main')
  const files = []
  const visit = (directory) => {
    for (const entry of readdirSync(directory)) {
      const path = join(directory, entry)
      const stat = statSync(path)
      if (stat.isDirectory()) {
        visit(path)
      } else if (entry.endsWith('.ts') && !entry.endsWith('.test.ts')) {
        files.push(path)
      }
    }
  }
  visit(start)
  return files
}

export function forbiddenProviderEgressCapabilities(source) {
  return FORBIDDEN_ORDINARY_CAPABILITIES
    .filter(([, pattern]) => pattern.test(source))
    .map(([label]) => label)
}

export function forbiddenMediaProviderCapabilities(source) {
  return FORBIDDEN_MEDIA_CAPABILITIES
    .filter(([, pattern]) => pattern.test(source))
    .map(([label]) => label)
}

export function forbiddenRendererSpeechAuthorityCapabilities(source) {
  return [
    ['resolved Provider config', /resolveAnalytixSpeechToTextSettings|speechToText\?:\s*AnalytixSpeechToTextSettings/u],
    ['Provider field gate', /speechToTextSettings\.(?:baseUrl|model)/u],
    ['copied config forwarding', /speechToText:\s*speechToTextSettings/u]
  ].filter(([, pattern]) => pattern.test(source)).map(([label]) => label)
}

export function isProviderLikeNetworkEffect(source) {
  return (NETWORK_CALL.test(source) && PROVIDER_EFFECT_MARKER.test(source)) || PROVIDER_SDK_CALL.test(source)
}

export function forbiddenCLIProviderAuthorityArgs(source) {
  return [
    '--provider-id', '--base-url', '--model-proxy-url', '--endpoint-format', '--model-providers-json'
  ].filter((flag) => source.includes(flag))
}

function requireSource(source, pattern, message, failures) {
  if (!pattern.test(source)) failures.push(message)
}

function requireOrderedSource(source, first, second, message, failures) {
  const firstIndex = source.indexOf(first)
  const secondIndex = source.indexOf(second)
  if (firstIndex < 0 || secondIndex < 0 || firstIndex >= secondIndex) failures.push(message)
}

export function verifyLocalProviderConsumers(root = REPO_ROOT) {
  const failures = []

  for (const path of productionTypeScriptFiles(root)) {
    const id = relative(root, path)
    const source = readFileSync(path, 'utf8')
    if (isProviderLikeNetworkEffect(source) && !ALLOWED_PROVIDER_NETWORK_OWNERS.has(id)) {
      failures.push(`${id}: production Provider-like network effect is not an allowed readiness, K8, or Slice B owner`)
    }
  }

  for (const id of ORDINARY_TS_CONSUMERS) {
    const source = readFileSync(join(root, id), 'utf8')
    for (const capability of forbiddenProviderEgressCapabilities(source)) {
      failures.push(`${id}: forbidden ordinary consumer capability: ${capability}`)
    }
  }


  for (const id of MEDIA_TS_CONSUMERS) {
    const source = readFileSync(join(root, id), 'utf8')
    for (const capability of forbiddenMediaProviderCapabilities(source)) {
      failures.push(`${id}: forbidden media consumer capability: ${capability}`)
    }
  }

  const adapter = readFileSync(join(root, 'src/main/runtime/analytix-adapter.ts'), 'utf8')
  const argvOwner = adapter.slice(
    adapter.indexOf('export function buildGoRuntimeProviderArgs'),
    adapter.indexOf('export function buildGoRuntimeSidecarEnv')
  )
  for (const flag of [
    '--provider-id', '--base-url', '--endpoint-format', '--model', '--model-providers-json', '--model-proxy-url'
  ]) {
    if (argvOwner.includes(flag)) failures.push(`src/main/runtime/analytix-adapter.ts: copied Provider argv flag ${flag}`)
  }
  requireSource(
    adapter,
    /for \(const providerGroup of PROVIDER_CREDENTIAL_GROUPS\)[\s\S]*delete childEnvironment\[name\]/u,
    'src/main/runtime/analytix-adapter.ts: sidecar environment does not strip ambient Provider aliases',
    failures
  )

  const serveEntry = readFileSync(join(root, 'packages/runtime/src/cli/serve-entry.ts'), 'utf8')
  const serveArgsStart = serveEntry.indexOf('function goRuntimeServeArgs(')
  const serveArgsEnd = serveEntry.indexOf('\nexport function resolveGoRuntimeLaunchTarget', serveArgsStart)
  const serveArgsOwner = serveArgsStart < 0 || serveArgsEnd < 0 ? '' : serveEntry.slice(serveArgsStart, serveArgsEnd)
  for (const flag of forbiddenCLIProviderAuthorityArgs(serveArgsOwner)) {
    failures.push(`packages/runtime/src/cli/serve-entry.ts: copied CLI Provider authority ${flag}`)
  }

  const runtimeCLI = readFileSync(join(root, 'packages/runtime-go/cmd/runtime-server/main.go'), 'utf8')
  const runtimeConfigStart = runtimeCLI.indexOf('func runtimeConfigFromCLI(')
  const runtimeConfigEnd = runtimeCLI.indexOf('\nfunc clearRuntimeServerSecretEnvironment(', runtimeConfigStart)
  const runtimeConfigOwner = runtimeConfigStart < 0 || runtimeConfigEnd < 0
    ? ''
    : runtimeCLI.slice(runtimeConfigStart, runtimeConfigEnd)
  for (const field of ['ProviderID', 'BaseURL', 'Model', 'EndpointFormat', 'ModelProvidersJSON', 'ModelProxyURL']) {
    requireSource(
      runtimeConfigOwner,
      new RegExp(`${field}:\\s+""`, 'u'),
      `runtime-server CLI: legacy ${field} can still become Provider authority`,
      failures
    )
  }

  const observationOwner = readFileSync(join(root, 'packages/runtime-go/internal/runtimeapp/provider_registry_operations.go'), 'utf8')
  const observeStart = observationOwner.indexOf('func (service *providerRegistryOperationsV1) ObserveAccount(')
  const observeEnd = observationOwner.indexOf('\ntype providerRegistryAccountObservationOutcomeV1', observeStart)
  const observeMethod = observeStart < 0 || observeEnd < 0 ? '' : observationOwner.slice(observeStart, observeEnd)
  const physicalObservation = observeMethod.indexOf('executeProviderRegistryAccountObservationV1')
  const firstObservationFence = observeMethod.lastIndexOf('ValidateProviderOperationCurrent', physicalObservation)
  const secondObservationFence = observeMethod.indexOf('ValidateProviderOperationCurrent', physicalObservation)
  if (firstObservationFence < 0 || physicalObservation < 0 || secondObservationFence < 0 ||
      firstObservationFence >= physicalObservation || physicalObservation >= secondObservationFence) {
    failures.push('Provider account observation: exact currentness does not bracket the bounded Provider effect')
  }
  requireSource(
    observationOwner,
    /CheckRedirect:\s*func\([\s\S]*http\.ErrUseLastResponse/u,
    'Provider account observation: redirects are not fail-closed',
    failures
  )

  const mainIndex = readFileSync(join(root, 'src/main/index.ts'), 'utf8')
  const trayObservationStart = mainIndex.indexOf('loadTrayProviderObservation = async () => {')
  const trayObservationEnd = mainIndex.indexOf('\n  const managedMcpAccountBindings', trayObservationStart + 1)
  const trayObservationOwner = trayObservationStart < 0 || trayObservationEnd < 0
    ? ''
    : mainIndex.slice(trayObservationStart, trayObservationEnd)
  requireSource(
    trayObservationOwner,
    /operation:\s*'observe-account'[\s\S]*operation:\s*'list'[\s\S]*currentTrayProviderObservation/u,
    'tray Provider observation lacks a post-effect Registry readback before publication',
    failures
  )
  if (/Hub|setInterval|credentialRef|token/u.test(trayObservationOwner)) {
    failures.push('tray Provider observation retained Hub, timer, or secret authority')
  }
  const trayOwner = readFileSync(join(root, 'src/main/tray-session-menu.ts'), 'utf8')
  requireSource(
    trayOwner,
    /currentSnapshot\.registryRevision\s*!==\s*observation\.registryRevision[\s\S]*currentProvider\.generation\s*!==\s*observation\.providerGeneration[\s\S]*JSON\.stringify\(currentProvider\.accountObservation\s*\?\?\s*null\)[\s\S]*expiresAtMs\s*<=\s*nowMs/u,
    'tray Provider observation publication does not verify the full fence, binding, and expiry',
    failures
  )
  requireSource(
    trayOwner,
    /observation\.providerCredentialPurpose\s*!==\s*expectedProvider\.credentialPurpose[\s\S]*currentProvider\.credentialPurpose\s*!==\s*expectedProvider\.credentialPurpose/u,
    'tray Provider observation publication omits the credential-purpose fence',
    failures
  )

  const providerSettings = readFileSync(join(root, 'src/renderer/src/components/settings-section-providers.tsx'), 'utf8')
  const rendererObservationStart = providerSettings.indexOf('const runAccountObservation = async')
  const rendererObservationEnd = providerSettings.indexOf('\n  const configureAccountObservation', rendererObservationStart)
  const rendererObservationOwner = rendererObservationStart < 0 || rendererObservationEnd < 0
    ? ''
    : providerSettings.slice(rendererObservationStart, rendererObservationEnd)
  requireSource(
    rendererObservationOwner,
    /operation:\s*'observe-account'[\s\S]*operation:\s*'list'[\s\S]*providerAccountObservationStateFromResult/u,
    'renderer Provider observation lacks a post-effect Registry readback before publication',
    failures
  )
  requireSource(
    providerSettings,
    /snapshot\.registryRevision\s*!==\s*state\.registryRevision[\s\S]*provider\.generation\s*!==\s*state\.providerGeneration[\s\S]*JSON\.stringify\(provider\.accountObservation\s*\?\?\s*null\)[\s\S]*expiresAtMs\s*>\s*nowMs/u,
    'renderer Provider observation state does not retain and verify the full fence, binding, and expiry',
    failures
  )
  requireSource(
    providerSettings,
    /\(provider\.credentialPurpose\s*\?\?\s*''\)\s*!==\s*state\.providerCredentialPurpose[\s\S]*observation\.providerCredentialPurpose\s*!==\s*expectedProvider\.credentialPurpose/u,
    'renderer Provider observation publication omits the credential-purpose fence',
    failures
  )
  requireSource(
    providerSettings,
    /persistProviderAccountObservationBinding\(\{[\s\S]*accountObservation:\s*remove[\s\S]*method:\s*'GET'[\s\S]*projection:\s*'normalized-quota-v1'/u,
    'custom Provider observation binding has no deliberate Registry-owned configure/remove producer',
    failures
  )

  const loop = readFileSync(join(root, 'packages/runtime-go/internal/server/agent_loop.go'), 'utf8')
  requireSource(loop, /resolveRuntimeTurnIntent\(/u, 'agent_loop.go: model steps do not refresh key-free Registry intent', failures)
  requireSource(loop, /resolveRuntimeTurnExecution\(/u, 'agent_loop.go: provider effects do not resolve current Registry execution', failures)
  requireSource(loop, /validateRuntimeTurnExecutionCurrent\(/u, 'agent_loop.go: provider responses lack a post-effect currentness fence', failures)
  requireSource(
    loop,
    /request\.PrivateProviderCurrentnessBeforeSend\s*=\s*func\(physicalAttempt int\) error[\s\S]*validateRuntimeTurnExecutionCurrent\(attemptCtx, authority\)/u,
    'agent_loop.go: prepared Provider authority lacks an exact pre-send currentness fence',
    failures
  )
  requireSource(
    loop,
    /request\.ProxyURL\s*=\s*execution\.Config\.ProxyURL/u,
    'agent_loop.go: physical Provider request does not bind the current Registry proxy',
    failures
  )

  const providerClient = readFileSync(join(root, 'packages/runtime-go/internal/adapters/outbound/provider/client/client.go'), 'utf8')
  requireSource(
    providerClient,
    /httpClientForAttempt\([\s\S]*request\.ProxyURL,[\s\S]*request\.PrivateProviderProxyAuthority/u,
    'provider client: physical attempt does not construct transport from current Registry proxy authority',
    failures
  )
  const streamOnceStart = providerClient.indexOf('func (c *HTTPProviderClient) streamOnce(')
  const streamOnceEnd = streamOnceStart < 0
    ? -1
    : providerClient.indexOf('\nfunc ', streamOnceStart + 1)
  const streamOnce = streamOnceStart < 0 || streamOnceEnd < 0
    ? ''
    : providerClient.slice(streamOnceStart, streamOnceEnd)
  requireOrderedSource(
    streamOnce,
    'request.PrivateProviderCurrentnessBeforeSend(attempt)',
    'httpClient.Do(httpReq)',
    'provider client: exact currentness is not revalidated immediately before every physical send',
    failures
  )
  requireSource(
    loop,
    /request\.PrivateProviderProxyAuthority\s*=\s*requireIntentMatch/u,
    'agent_loop.go: an empty current Registry proxy can fall back to ambient startup transport',
    failures
  )

  const admission = readFileSync(join(root, 'packages/runtime-go/internal/server/turn_start.go'), 'utf8')
  requireSource(admission, /resolveRuntimeTurnIntent\(/u, 'turn_start.go: admission does not resolve key-free Registry intent', failures)
  if (/resolveRuntimeTurnExecution\(/u.test(admission)) {
    failures.push('turn_start.go: admission retained effect credential authority')
  }

  const pending = readFileSync(join(root, 'packages/runtime-go/internal/app/model/pending_tool.go'), 'utf8')
  requireSource(pending, /providerConfig\.APIKey\s*=\s*""/u, 'pending_tool.go: durable pending work may retain credential bytes', failures)
  requireSource(pending, /providerConfig\.ProxyURL\s*=\s*""/u, 'pending_tool.go: durable pending work may retain Provider proxy authority', failures)

  const registryComposition = readFileSync(join(root, 'packages/runtime-go/internal/runtimeapp/provider_registry_execution.go'), 'utf8')
  requireSource(
    registryComposition,
    /input\.RequestEndpointFormat\s*=\s*""/u,
    'provider_registry_execution.go: caller endpoint override may replace Registry route authority',
    failures
  )
  requireSource(
    registryComposition,
    /ResolveVisionExecution\([\s\S]*ResolveSelectedMediaForExecution\([\s\S]*ValidateMediaExecutionCurrent\(/u,
    'provider_registry_execution.go: Vision Bridge lacks JIT Registry media resolution and currentness',
    failures
  )

  const mediaExecutor = readFileSync(join(root, 'packages/runtime-go/internal/app/mediaexecution/service.go'), 'utf8')
  requireSource(
    mediaExecutor,
    /ResolveSelectedMediaForExecution\(ctx\)/u,
    'media executor: physical effect does not resolve current Registry media authority',
    failures
  )
  const mediaSendStart = mediaExecutor.indexOf('func (executor *Executor) send(')
  const mediaSendEnd = mediaSendStart < 0 ? -1 : mediaExecutor.indexOf('\nfunc ', mediaSendStart + 1)
  const mediaSend = mediaSendStart < 0 || mediaSendEnd < 0 ? '' : mediaExecutor.slice(mediaSendStart, mediaSendEnd)
  const firstMediaValidation = mediaSend.indexOf('ValidateMediaExecutionCurrent')
  const physicalMediaSend = mediaSend.indexOf('client.Do(request)')
  const secondMediaValidation = mediaSend.indexOf('ValidateMediaExecutionCurrent', firstMediaValidation + 1)
  if (firstMediaValidation < 0 || physicalMediaSend < 0 || secondMediaValidation < 0 ||
      firstMediaValidation >= physicalMediaSend || physicalMediaSend >= secondMediaValidation) {
    failures.push('media executor: Registry media currentness does not bracket the physical send')
  }
  for (const [functionName, resultMarker] of [
    ['executeImage', 'return Result{Image: image, MIMEType: mimeType}, nil'],
    ['downloadImage', 'return Result{Image: image, MIMEType: mimeType}, nil'],
    ['executeSpeech', 'return Result{Transcript: transcript}, nil']
  ]) {
    const start = mediaExecutor.indexOf(`func (executor *Executor) ${functionName}(`)
    const end = start < 0 ? -1 : mediaExecutor.indexOf('\nfunc ', start + 1)
    const owner = start < 0 || end < 0 ? '' : mediaExecutor.slice(start, end)
    requireOrderedSource(
      owner,
      'readBounded(response.Body',
      'ValidateMediaExecutionCurrent(ctx',
      `media executor: ${functionName} does not revalidate authority after its complete bounded body`,
      failures
    )
    requireOrderedSource(
      owner,
      'ValidateMediaExecutionCurrent(ctx',
      resultMarker,
      `media executor: ${functionName} can return a result before its final authority fence`,
      failures
    )
  }
  requireSource(
    mediaExecutor,
    /CheckRedirect\s*=\s*func\([\s\S]*http\.ErrUseLastResponse/u,
    'media executor: Provider redirects are not fail-closed',
    failures
  )

  const voiceDictation = readFileSync(join(root, 'src/renderer/src/components/chat/use-voice-dictation.ts'), 'utf8')
  const floatingComposer = readFileSync(join(root, 'src/renderer/src/components/chat/FloatingComposer.tsx'), 'utf8')
  requireSource(
    voiceDictation,
    /return settings\.runtime\.speechToText\.enabled === true/u,
    'voice dictation: explicit key-free enabled preference is not the sole renderer start preference',
    failures
  )
  requireSource(
    floatingComposer,
    /const showVoiceDictation = speechToTextEnabled/u,
    'FloatingComposer: voice attempt is not gated solely by the explicit key-free enabled preference',
    failures
  )
  for (const capability of forbiddenRendererSpeechAuthorityCapabilities(`${voiceDictation}\n${floatingComposer}`)) {
    failures.push(`renderer speech: forbidden copied authority capability: ${capability}`)
  }
  requireSource(
    mediaExecutor,
    /ResolveSelectedMediaForExecution\(ctx\)[\s\S]*original\.Authority\(\)[\s\S]*http\.MethodGet[\s\S]*false\)/u,
    'media executor: remote image download lacks fresh authority or may forward the Provider credential',
    failures
  )

  const mediaHTTP = readFileSync(join(root, 'packages/runtime-go/internal/adapters/inbound/httpapi/media_execution.go'), 'utf8')
  requireSource(
    mediaHTTP,
    /MediaExecutionPathV1\s*=\s*"\/v1\/runtime\/_private\/media-execution"/u,
    'media HTTP: fixed private Main-to-Go route is missing',
    failures
  )
  requireSource(
    mediaHTTP,
    /decoder\.DisallowUnknownFields\(\)/u,
    'media HTTP: renderer-supplied Provider authority fields are not rejected',
    failures
  )

  const visionBridge = readFileSync(join(root, 'packages/runtime-go/internal/server/vision_bridge.go'), 'utf8')
  if (/ConfigResolver:\s*h\.providerConfig/u.test(visionBridge)) {
    failures.push('vision bridge: startup Provider config remains an execution authority')
  }
  requireSource(
    visionBridge,
    /h\.providerExecution\.\(visionbridgeapp\.ExecutionResolver\)/u,
    'vision bridge: current Registry media execution resolver is not composed',
    failures
  )
  const runtimeComposition = readFileSync(join(root, 'packages/runtime-go/internal/runtimeapp/app.go'), 'utf8')
  for (const field of ['ProviderID', 'BaseURL', 'APIKey', 'Model', 'EndpointFormat']) {
    requireSource(
      runtimeComposition,
      new RegExp(`visionBridgeConfig\\.${field}\\s*=\\s*""`, 'u'),
      `runtimeapp: legacy Vision Bridge ${field} remains retained Provider authority`,
      failures
    )
  }
  requireSource(
    registryComposition,
    /providerMetadata\.ModelProxyURL\s*=\s*resolvedProvider\.Proxy/u,
    'provider_registry_execution.go: current Registry proxy is not propagated to internal execution config',
    failures
  )
  const runtimeHandler = readFileSync(join(root, 'packages/runtime-go/internal/server/runtime_handler.go'), 'utf8')
  const currentnessGuardStart = runtimeHandler.indexOf('func (h *runtimeServerHandler) requireRuntimeTurnExecutionCurrentness() error {')
  const currentnessGuardEnd = currentnessGuardStart < 0
    ? -1
    : runtimeHandler.indexOf('\n}', currentnessGuardStart)
  const currentnessGuard = currentnessGuardStart < 0 || currentnessGuardEnd < 0
    ? ''
    : runtimeHandler.slice(currentnessGuardStart, currentnessGuardEnd + 2)
  requireSource(
    currentnessGuard,
    /ProviderExecutionIntentResolver\);\s*!lateBound\s*\{\s*return nil\s*\}/u,
    'runtime_handler.go: legacy neither-interface compatibility is not explicitly bounded',
    failures
  )
  requireSource(
    currentnessGuard,
    /ProviderExecutionCurrentnessValidator\);\s*!ok \|\| validator == nil\s*\{\s*return errors\.New\("provider execution currentness authority is unavailable"\)\s*\}/u,
    'runtime_handler.go: late-bound Provider composition does not fail closed without the post-response validator',
    failures
  )
  const childRun = readFileSync(join(root, 'packages/runtime-go/internal/app/subagent/child_run.go'), 'utf8')
  requireSource(
    childRun,
    /ExecutionRef\(execution\.ProviderID, execution\.Model, execution\.Variant, "", "", modelSource\)/u,
    'subagent child run persists Registry route metadata',
    failures
  )
  const childTurn = readFileSync(join(root, 'packages/runtime-go/internal/app/subagent/turn_completion.go'), 'utf8')
  requireSource(childTurn, /EndpointFormat:\s+""/u, 'subagent child turn reuses persisted Registry route', failures)

  if (failures.length > 0) {
    throw new Error(`local Provider consumer guard failed:\n- ${failures.join('\n- ')}`)
  }
  return {
    checkedProductionTypeScriptFiles: productionTypeScriptFiles(root).length,
    checkedOrdinaryConsumers: ORDINARY_TS_CONSUMERS.length,
    checkedMediaConsumers: MEDIA_TS_CONSUMERS.length,
    checkedGoAuthorityOwners: 12
  }
}

if (process.argv[1] && resolve(process.argv[1]) === SCRIPT_PATH) {
  try {
    const result = verifyLocalProviderConsumers(process.argv[2] ? resolve(process.argv[2]) : REPO_ROOT)
    process.stdout.write(`local-provider-consumer-guard: pass ${JSON.stringify(result)}\n`)
  } catch (error) {
    process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`)
    process.exitCode = 1
  }
}
