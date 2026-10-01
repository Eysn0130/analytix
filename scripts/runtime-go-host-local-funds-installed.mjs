#!/usr/bin/env node

// Bounded private Full acceptance. Native file selection remains visible;
// a supervising Computer Use session selects only the printed task CSV.
import { spawn } from 'node:child_process'
import { createHash } from 'node:crypto'
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs'
import http from 'node:http'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { packagedSessionAcceptance as installed } from './runtime-go-packaged-session-soak.mjs'
import { cdpComposerSubmit, cdpNormalQuit, dispatchCdpCommands, goJSON } from './runtime-go-packaged-milestone-b.mjs'

const providerId = 'host-local-installed-synthetic'
const model = 'mimo-v2.5-pro'
const syntheticKey = 'host-local-installed-synthetic-key'
const account = '6222021234567890'
const flowTool = 'mcp__analytix_funds__analyze_account_flows'
const purpose = 'analytix.account-flow-provider-semantics/v3'
const timeoutMs = 180_000
const sleep = (ms) => new Promise((done) => setTimeout(done, ms))
const hash = (value) => createHash('sha256').update(value).digest('hex')
const requireCondition = (condition, code) => { if (!condition) throw new Error(code) }

// Decode JSON text instead of embedding an object literal, which would treat
// an own __proto__ key as prototype syntax. goJSON remains the JSON/hash owner.
export function cdpValueExpression(value) {
  if (value === undefined) throw new TypeError('CDP value must not be undefined.')
  return `JSON.parse(${goJSON(goJSON(value))})`
}

export function ownedPackagedRenderer(pid, port, processInfo, targets) {
  const browsers = processInfo?.filter((process) => process.type === 'browser') || []
  requireCondition(browsers.length === 1 && browsers[0].id === pid, 'owned_browser_pid_mismatch')
  const pages = targets.filter((target) => target.type === 'page' && target.webSocketDebuggerUrl)
  requireCondition(pages.length > 0, 'owned_packaged_renderer_not_ready')
  requireCondition(pages.length === 1, 'owned_packaged_renderer_mismatch')
  const rendererURL = new URL(pages[0].url)
  const pageURL = new URL(pages[0].webSocketDebuggerUrl)
  requireCondition(rendererURL.protocol === 'analytix-app:' && rendererURL.hostname === 'renderer' &&
    rendererURL.pathname === '/index.html', 'owned_packaged_renderer_mismatch')
  requireCondition(pageURL.protocol === 'ws:' && ['127.0.0.1', 'localhost'].includes(pageURL.hostname) &&
    Number(pageURL.port) === port, 'owned_renderer_endpoint_mismatch')
  return pages[0]
}

export function syntheticCleaningCSV() {
  const columns = [
    '交易卡号', '交易账号', '账户开户名称', '开户人证件号码', '交易时间', '交易金额', '交易余额', '收付标志',
    '交易对手账卡号', '现金标志', '对手户名', '对手身份证号', '对手开户银行', '摘要说明', '交易币种', '交易网点名称',
    '交易网点代码', '交易发生地', '交易是否成功', '传票号', '终端号', 'IP地址', 'MAC地址', '对手交易余额',
    '交易流水号', '日志号', '凭证种类', '凭证号', '交易柜员号', '商户名称', '商户号', '备注', '交易类型', '查询反馈结果原因'
  ]
  const rows = [
    ['10:00:00', '12.5', '进', '9000-001 23'],
    ['11:00:00', '2.5', '出', '9000-002 34'],
    ['12:00:00', '7', '进', '900000345']
  ].map(([time, amount, direction, counterparty]) => {
    const row = Array(34).fill('')
    row[1] = account
    row[4] = `2026-08-27 ${time}`
    row[5] = amount
    row[7] = direction
    row[8] = counterparty
    row[12] = '合成银行'
    row[14] = 'CNY'
    return row
  })
  return `${[columns, ...rows].map((row) => row.join(',')).join('\r\n')}\r\n`
}

function semanticEnvelope(value) {
  if (typeof value === 'string') {
    try { return semanticEnvelope(JSON.parse(value)) } catch { return null }
  }
  if (!value || typeof value !== 'object') return null
  if (value.purpose === purpose) return value
  for (const child of Object.values(value)) {
    const found = semanticEnvelope(child)
    if (found) return found
  }
  return null
}

export function exactAccountFlow(value) {
  return value?.schemaVersion === 3 && value.purpose === purpose && value.semanticStatus === 'success' &&
    value.data?.subjectAlias === 'acct:1' && value.data?.currency === 'CNY' && value.data?.minorUnitScale === 2 &&
    value.data?.currentness === 'current' && value.data?.coverage?.state === 'complete' &&
    value.data?.inflowMinor === '1950' && value.data?.outflowMinor === '250' && value.data?.netMinor === '1700' &&
    value.data?.transactionCount === 3 && value.data?.evidenceTransactionCount === 3 &&
    value.data?.aggregateComplete === true && value.data?.evidenceRowsComplete === true &&
    Array.isArray(value.data?.coverage?.gaps) && value.data.coverage.gaps.length === 0
}

export function currentAccountFlow(messages, toolCallId = '') {
  if (!Array.isArray(messages)) return null
  const lastUser = messages.findLastIndex((message) => message?.role === 'user')
  const current = messages.slice(lastUser + 1)
  if (toolCallId && !current.some((message) => message?.role === 'assistant' &&
      message.tool_calls?.some((call) => call.id === toolCallId && call.function?.name === flowTool))) return null
  const result = current.findLast((message) => message?.role === 'tool' && (!toolCallId || message.tool_call_id === toolCallId))
  return semanticEnvelope(result?.content)
}

export async function syntheticProvider() {
  const requests = []
  const issued = new Map()
  let closing
  const server = http.createServer((request, response) => {
    const receipt = { round: 'unclassified', safe: null,
      authorized: request.headers.authorization === `Bearer ${syntheticKey}`,
      advertised: false, semantic: false, exact: null, accepted: false }
    requests.push(receipt)
    if (request.method !== 'POST' || request.url !== '/v1/chat/completions') {
      response.writeHead(404).end()
      return
    }
    let raw = ''
    request.setEncoding('utf8')
    request.on('data', (chunk) => {
      raw += chunk
      if (Buffer.byteLength(raw) > 4 * 1024 * 1024) request.destroy()
    })
    request.on('end', () => {
      receipt.safe = ![account, '9000-001 23', '9000-002 34', '900000123', '900000234', '900000345'].some((value) => raw.includes(value))
      try {
        const body = JSON.parse(raw)
        const user = (body.messages || []).findLast((message) => message?.role === 'user')
        const text = JSON.stringify(user?.content || '')
        const round = ['BEFORE_CLEANING', 'AFTER_CLEANING'].find((marker) => text.includes(marker))
        requireCondition(round, 'synthetic_provider_round_missing')
        const expectedCall = issued.get(round)
        const semantic = expectedCall ? currentAccountFlow(body.messages, expectedCall) : null
        const tools = (body.tools || []).map((tool) => tool.function?.name)
        const advertised = tools.includes(flowTool) && !tools.includes('mcp__analytix_funds__count_case_rows')
        const valid = expectedCall ? !!semantic && exactAccountFlow(semantic) : advertised
        Object.assign(receipt, { round, advertised, semantic: !!semantic, exact: semantic ? valid : null })
        requireCondition(receipt.safe && receipt.authorized && valid, 'synthetic_provider_request_refused')
        receipt.accepted = true
        const callId = `call_host_local_${requests.length}`
        if (!semantic) issued.set(round, callId)
        const delta = semantic ? { content: 'HOST_LOCAL_INSTALLED_SYNTHETIC_TERMINAL' } : {
          role: 'assistant', tool_calls: [{ index: 0, id: callId, type: 'function', function: {
            name: flowTool, arguments: JSON.stringify({ subject_alias: 'acct:1',
              start_inclusive: '2026-08-27T00:00:00.000000Z', end_inclusive: '2026-08-27T23:59:59.999999Z', evidence_row_limit: 3 })
          } }]
        }
        response.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
        response.end(`data: ${JSON.stringify({ choices: [{ delta, finish_reason: semantic ? 'stop' : 'tool_calls' }] })}\n\ndata: [DONE]\n\n`)
      } catch {
        if (!response.headersSent) response.writeHead(503, { 'content-type': 'application/json' })
        response.end('{"error":{"message":"synthetic request refused"}}')
      }
    })
  })
  await new Promise((done) => server.listen(0, '127.0.0.1', done))
  return { url: `http://127.0.0.1:${server.address().port}/v1`, requests,
    close: () => closing ||= new Promise((done) => server.close(done)) }
}

export async function runHostLocalInstalled(appPath, cacheRoot) {
  requireCondition(process.platform === 'darwin' && process.arch === 'arm64', 'darwin_arm64_required')
  requireCondition(cacheRoot.startsWith('/Volumes/AnalytixCache/'), 'task_cache_root_required')
  const target = { platform: 'darwin', arch: 'arm64', key: 'darwin-arm64' }
  const artifact = installed.packagedAppArtifactEvidence(appPath, target)
  requireCondition(artifact.ok, 'packaged_artifact_authority_failed')
  const root = mkdtempSync(join(cacheRoot, 'host-local-installed-'))
  const paths = Object.fromEntries(['home', 'state', 'profile', 'chromium', 'runtime', 'workspace'].map((name) => {
    const path = join(root, name)
    mkdirSync(path, { mode: 0o700 })
    return [name, path]
  }))
  mkdirSync(join(paths.workspace, '.analytix'), { mode: 0o700 })
  writeFileSync(join(paths.workspace, '.analytix/case-project.json'), JSON.stringify({ version: 1,
    workspaceRoot: paths.workspace, caseId: 'case_host_local_installed_synthetic', source: 'analytix-data-analysis',
    updatedAt: '2026-09-30T00:00:00Z' }), { mode: 0o600 })
  const csvPath = join(paths.workspace, 'synthetic-cleaning.csv')
  writeFileSync(csvPath, syntheticCleaningCSV(), { mode: 0o600 })
  const runtimePort = await installed.getFreePort()
  const provider = await syntheticProvider()
  const env = installed.smokeChildEnv({ tempHome: paths.home, stateRoot: paths.state,
    userDataDir: paths.profile, chromiumTempDir: paths.chromium })
  const report = { schemaVersion: 1, id: 'host-local-funds-installed-synthetic/v1', generatedAt: new Date().toISOString(),
    source: artifact, root, synthetic: true, liveProvider: false, passed: false, checks: [] }
  let child, debugPort, quitAttempted = false, phase = 'launch'
  const progress = (stage, extra = {}) => process.stdout.write(`${JSON.stringify({ stage, ...extra })}\n`)
  const check = (id, condition) => {
    report.checks.push({ id, passed: !!condition })
    requireCondition(condition, id)
    progress(id)
  }
  const launch = async () => {
    debugPort = await installed.getFreePort()
    child = spawn(installed.resolveExecutablePath(appPath, target), [`--remote-debugging-port=${debugPort}`],
      { env, stdio: ['ignore', 'ignore', 'ignore'] })
    child.exited = new Promise((done) => child.once('exit', (code, signal) => done({ code, signal })))
    child.once('error', () => { if (!child.pid) child.exitCode = -1 })
    quitAttempted = false
    progress('owned-main-launched', { pid: child.pid, appPath, debugPort })
    const deadline = Date.now() + timeoutMs
    while (true) {
      try { await ownedTarget(); break } catch (error) {
        if (Date.now() >= deadline || !['ECONNREFUSED', 'owned_packaged_renderer_not_ready'].includes(error?.cause?.code || error?.message)) throw error
        await sleep(250)
      }
    }
    await poll(`!!window.analytix`, Boolean)
  }
  const ownedTarget = async () => {
    requireCondition(Number.isInteger(child?.pid) && child.exitCode === null && !child.signalCode, 'owned_main_unavailable')
    const version = await fetch(`http://127.0.0.1:${debugPort}/json/version`, { signal: AbortSignal.timeout(2000) }).then((r) => r.json())
    const browserURL = new URL(version.webSocketDebuggerUrl)
    requireCondition(browserURL.protocol === 'ws:' && ['127.0.0.1', 'localhost'].includes(browserURL.hostname) &&
      Number(browserURL.port) === debugPort, 'owned_browser_endpoint_mismatch')
    // CDP reports this browser's OS PID without auxiliary process inventory.
    const [info] = await dispatchCdpCommands(browserURL.href, [{ method: 'SystemInfo.getProcessInfo' }], 5000)
    const targets = await fetch(`http://127.0.0.1:${debugPort}/json/list`, { signal: AbortSignal.timeout(2000) }).then((r) => r.json())
    requireCondition(child.exitCode === null && !child.signalCode, 'owned_main_unavailable')
    return ownedPackagedRenderer(child.pid, debugPort, info.processInfo, targets)
  }
  const evaluate = async (expression, budget = 30_000) => {
    const target = await ownedTarget()
    return installed.evaluateCdp(target.webSocketDebuggerUrl, expression, budget)
  }
  const poll = async (expression, predicate, budget = timeoutMs) => {
    const deadline = Date.now() + budget
    while (Date.now() < deadline) {
      try {
        const value = await evaluate(expression)
        if (predicate(value)) return value
      } catch (error) {
        // Only this read-only observer may wait through document replacement.
        if (error?.cdpProtocolCode !== -32000 && error?.message !== 'cdp_connection_closed') throw error
      }
      await sleep(500)
    }
    throw new Error('readonly_observation_timeout')
  }
  const normalQuit = async () => {
    requireCondition(!quitAttempted, 'normal_quit_already_attempted')
    quitAttempted = true
    const quit = await cdpNormalQuit(debugPort, ownedTarget)
    report.quitOutcomes = [...(report.quitOutcomes || []), { pid: child.pid, ok: quit.ok, outcome: quit.cdpOutcome || 'acknowledged' }]
    const exit = await Promise.race([child.exited, sleep(30_000).then(() => ({ timeout: true }))])
    requireCondition((quit.ok || quit.cdpOutcome === 'unknown') && exit.code === 0 && !exit.signal, 'normal_owned_main_exit_failed')
    // This is a listener-shutdown observation, not an OS process-tree claim.
    for (let attempt = 0; attempt < 20; attempt++) {
      try { await fetch(`http://127.0.0.1:${runtimePort}/health`, { signal: AbortSignal.timeout(500) }) }
      catch { return }
      await sleep(250)
    }
    throw new Error('owned_runtime_listener_did_not_close')
  }
  const sourceExpression = `window.analytix.runtime.directSourcePreview({kind:'direct_source_preview',view:'transactions',
    fields:['account','amountText','counterpartyAccount'],rowOffset:0,rowLimit:3,displayMode:'full'})`
  let threadId
  const title = 'Host-local installed journey'
  const selectThread = async () => {
    // The normal packaged deep link loads even a newly created empty thread;
    // its sidebar registration may not precede the first message.
    const target = await ownedTarget()
    await dispatchCdpCommands(target.webSocketDebuggerUrl, [{ method: 'Page.navigate', params: {
      url: `analytix-app://renderer/index.html?threadId=${encodeURIComponent(threadId)}`
    } }], 10_000)
    await poll(`!!window.analytix&&new URL(window.location.href).searchParams.get('threadId')===${cdpValueExpression(threadId)}`, Boolean)
    await poll(`(()=>{const h=document.querySelector('.ds-session-title-compact');return h?.title===${cdpValueExpression(title)}||h?.textContent.trim()===${cdpValueExpression(title)};})()`, Boolean)
  }
  const readTurn = () => `(async()=>{const r=await window.analytix.runtime.runtimeRequest('/v1/threads/'+${cdpValueExpression(threadId)},'GET');
    if(r.status!==200)return null;const t=JSON.parse(r.body);return {id:t.id,title:t.title,turns:t.turns};})()`
  const display = (turn) => evaluate(`window.analytix.runtime.acceptedSlotDisplay({kind:'accepted_slot_display',
    threadId:${cdpValueExpression(threadId)},turnId:${cdpValueExpression(turn.id)},acceptedFinalDigest:${cdpValueExpression(turn.acceptedFinalView?.acceptedFinalDigest)},displayMode:'full'})`)
  const observeDisplay = async (turn, id) => {
    const user = turn.items?.find((item) => item.kind === 'user_message')
    requireCondition(!!user?.id, 'task_turn_user_item_missing')
    const observed = await poll(`(()=>{const row=[...document.querySelectorAll('[data-turn-key]')].find(e=>e.getAttribute('data-turn-key')===${cdpValueExpression(user.id)});
      const section=row?.querySelector('[data-analytix-local-display="accepted_slot_display"]');
      const values=section?[...section.querySelectorAll('dd')].map(e=>e.textContent):[];
      return {full:section?.getAttribute('data-analytix-local-display-mode')==='full',exactAccount:values.includes(${cdpValueExpression(account)}),slotCount:values.length};})()`,
      (value) => value.full && value.exactAccount && value.slotCount > 0)
    check(id, observed.full && observed.exactAccount)
  }
  const query = async (marker) => {
    const before = await evaluate(readTurn())
    const count = before?.turns?.length || 0
    const submitted = await cdpComposerSubmit(debugPort,
      `查询当前案件账户在2026年8月27日的流入、流出、净额和交易笔数。银行账号为 ${account}。 ${marker}`,
      ['Send message', 'Send', '发送消息', '发送'], ownedTarget)
    requireCondition(submitted.ok, 'composer_submission_failed')
    const observation = await poll(readTurn(), (value) => value?.turns?.length === count + 1 &&
      ['completed', 'failed', 'aborted'].includes(value.turns.at(-1)?.status))
    const turn = observation.turns.at(-1)
    check(`final-${marker}`, turn.status === 'completed' && turn.acceptedFinalView?.schemaVersion === 3 &&
      turn.acceptedFinalView.variant === 'EvidenceBackedAnswer' && turn.acceptedFinalView.terminalReason === 'success' &&
      /^[a-f0-9]{64}$/.test(turn.acceptedFinalView.acceptedFinalDigest || ''))
    return turn
  }
  try {
    await launch()
    phase = 'configure-synthetic-provider'
    const configured = await evaluate(`(async()=>{const a=window.analytix;await a.settings.setSettings({workspaceRoot:${cdpValueExpression(paths.workspace)},
      runtime:{port:${runtimePort},dataDir:${cdpValueExpression(paths.runtime)},providerId:${cdpValueExpression(providerId)},model:${cdpValueExpression(model)},autoStart:true},
      provider:{activeProviderId:${cdpValueExpression(providerId)},providers:[{id:${cdpValueExpression(providerId)},
        name:'Installed synthetic',baseUrl:${cdpValueExpression(provider.url)},endpointFormat:'chat_completions',models:[${cdpValueExpression(model)}],
        modelProfiles:{[${cdpValueExpression(model)}]:{contextWindowTokens:1000000,inputModalities:['text'],outputModalities:['text'],supportsToolCalling:true,
          messageParts:['text'],reasoning:{supportedEfforts:['off','low','medium','high','max'],defaultEffort:'high',requestProtocol:'mimo-chat-completions'}}}}]}});
      await a.runtime.restartRuntime();const p=await a.providerRegistry.request({schemaVersion:1,operation:'list'});
      const c=await a.providerRegistry.request({schemaVersion:1,operation:'connect',expected:{registryRevision:p.registryRevision,registryIncarnation:p.registryIncarnation,
        providerRevision:'0',providerGeneration:'0',providerIncarnation:'',providerCredentialPurpose:''},provider:{id:${cdpValueExpression(providerId)},kind:'openai-compatible',
        endpoint:${cdpValueExpression(provider.url)},proxy:'',models:[${cdpValueExpression(model)}],mediaModels:[],selectedModel:${cdpValueExpression(model)},selectedMediaModel:'',selectedRoutes:[]},
        credential:{kind:'set',purpose:'provider-api-key',valueBase64:btoa(${cdpValueExpression(syntheticKey)})}});return !c.error;})()`, timeoutMs)
    check('isolated-synthetic-registry-connect', configured)
    phase = 'native-import'
    await evaluate(`(()=>{window.__hostLocalInstalledStage={pending:true};window.analytix.runtime.stageFundsCSVSnapshot().then(
      value=>{window.__hostLocalInstalledStage=value;},()=>{window.__hostLocalInstalledStage={ok:false};});return true;})()`)
    progress('native-file-selection-required', { pid: child.pid, appPath, csvPath })
    const staged = await poll(`window.__hostLocalInstalledStage`, (value) => value && !value.pending, 10 * 60_000)
    check('native-csv-stage', staged.ok && staged.totalRowCount === 3 && staged.items?.length === 1)
    const confirmed = await evaluate(`window.analytix.runtime.confirmFundsCSVSnapshot(${cdpValueExpression(staged.items[0].selector)})`, timeoutMs)
    check('native-csv-confirm', confirmed.ok && confirmed.rowCount === 3)
    const firstSource = await evaluate(sourceExpression, timeoutMs)
    check('current-import-source', firstSource.kind === 'direct_source_preview' && firstSource.rows?.length === 3)
    const created = await evaluate(`(async()=>{const r=await window.analytix.runtime.runtimeRequest('/v1/threads','POST',JSON.stringify({title:'Host-local installed journey',
      workspace:${cdpValueExpression(paths.workspace)},providerId:${cdpValueExpression(providerId)},model:${cdpValueExpression(model)},mode:'agent'}));return r.status===201?JSON.parse(r.body):null;})()`)
    threadId = created?.id
    check('durable-thread-created', !!threadId)
    await selectThread()
    phase = 'final-before-cleaning'
    const firstFinal = await query('BEFORE_CLEANING')
    const firstDisplay = await display(firstFinal)
    check('accepted-display-before-cleaning', firstDisplay.kind === 'accepted_slot_display' && firstDisplay.datasetSnapshotId === firstSource.datasetSnapshotId &&
      firstDisplay.slots?.some((slot) => slot.field === 'account' && slot.displayValue === account))
    await observeDisplay(firstFinal, 'renderer-display-before-cleaning')
    phase = 'deterministic-cleaning'
    const cleaning = await evaluate(`window.analytix.runtime.runDeterministicFundsCleaning()`, timeoutMs)
    check('actual-cleaning-committed', cleaning.ok && cleaning.status === 'committed' && cleaning.rowCount === 3 && cleaning.changedRowCount === 2 && cleaning.inputSnapshot !== cleaning.outputSnapshot)
    const diff = await evaluate(`window.analytix.runtime.cleaningDiffPreview({kind:'cleaning_diff_preview',selector:${cdpValueExpression(cleaning.selector)},
      fields:['counterpartyAccount'],rowOffset:0,rowLimit:3,displayMode:'full'})`, timeoutMs)
    check('actual-cleaning-diff', diff.kind === 'cleaning_diff_preview' && diff.rows?.length === 3 &&
      diff.rows[0].cells[0].beforeDisplayValue === '9000-001 23' && diff.rows[0].cells[0].afterDisplayValue === '900000123' &&
      diff.rows[1].cells[0].beforeDisplayValue === '9000-002 34' && diff.rows[1].cells[0].afterDisplayValue === '900000234')
    const secondSource = await evaluate(sourceExpression, timeoutMs)
    check('cleaning-successor-current', secondSource.kind === 'direct_source_preview' && secondSource.caseId === firstSource.caseId && secondSource.datasetSnapshotId !== firstSource.datasetSnapshotId)
    phase = 'final-after-cleaning'
    const secondFinal = await query('AFTER_CLEANING')
    const secondDisplay = await display(secondFinal)
    check('accepted-display-successor', secondDisplay.kind === 'accepted_slot_display' && secondDisplay.datasetSnapshotId === secondSource.datasetSnapshotId &&
      secondDisplay.contextEpoch > firstDisplay.contextEpoch && secondDisplay.slots?.some((slot) => slot.displayValue === account))
    await observeDisplay(secondFinal, 'renderer-display-successor')
    const retainedFirst = await display(firstFinal)
    check('original-display-retained', retainedFirst.datasetSnapshotId === firstSource.datasetSnapshotId && retainedFirst.contextEpoch === firstDisplay.contextEpoch)
    phase = 'normal-exit-and-relaunch'
    await normalQuit()
    check('normal-first-exit-and-listener-close', true)
    await launch()
    await selectThread()
    const recovered = await evaluate(readTurn())
    check('original-finals-restored', recovered.turns?.length === 2 && recovered.turns[0].acceptedFinalView?.acceptedFinalDigest === firstFinal.acceptedFinalView.acceptedFinalDigest &&
      recovered.turns[1].acceptedFinalView?.acceptedFinalDigest === secondFinal.acceptedFinalView.acceptedFinalDigest &&
      recovered.turns[0].status === 'completed' && recovered.turns[1].status === 'completed')
    const recoveredDisplay = await display(recovered.turns[1])
    check('successor-display-restored', recoveredDisplay.datasetSnapshotId === secondDisplay.datasetSnapshotId && recoveredDisplay.contextEpoch === secondDisplay.contextEpoch &&
      recoveredDisplay.slots?.some((slot) => slot.displayValue === account))
    await observeDisplay(recovered.turns[1], 'renderer-display-restored')
    const recoveredSource = await evaluate(sourceExpression, timeoutMs)
    check('successor-current-after-relaunch', recoveredSource.datasetSnapshotId === secondSource.datasetSnapshotId)
    await normalQuit()
    check('normal-second-exit-and-listener-close', true)
    await provider.close()
    check('synthetic-provider-exact-safe-results', provider.requests.length >= 4 && provider.requests.every((request) => request.safe && request.authorized && request.accepted) &&
      provider.requests.filter((request) => request.semantic && request.exact).length === 2)
    report.identities = { importedSnapshotSHA256: hash(firstSource.datasetSnapshotId), successorSnapshotSHA256: hash(secondSource.datasetSnapshotId),
      firstPublication: firstFinal.acceptedFinalView.acceptedFinalDigest, secondPublication: secondFinal.acceptedFinalView.acceptedFinalDigest }
    report.rounds = [{ round: 'BEFORE_CLEANING', contextEpoch: firstDisplay.contextEpoch, datasetSHA256: hash(firstDisplay.datasetSnapshotId) },
      { round: 'AFTER_CLEANING', contextEpoch: secondDisplay.contextEpoch, datasetSHA256: hash(secondDisplay.datasetSnapshotId) }]
      .map((round) => ({ ...round, freshToolResults: provider.requests.filter((request) => request.round === round.round && request.semantic && request.exact).length }))
    report.passed = true
  } catch (error) {
    report.failure = { phase, code: /^[a-z0-9_-]+$/.test(error?.message || '') ? error.message : 'bounded_installed_check_failed',
      mutatingCdpOutcome: error?.cdpOutcome || null }
    if (child && child.exitCode === null && !quitAttempted) {
      try { await normalQuit() } catch { report.ownedExitUnverified = true }
    } else if (child && child.exitCode === null) {
      const exit = await Promise.race([child.exited, sleep(30_000).then(() => ({ timeout: true }))])
      report.ownedExitUnverified = exit.code !== 0 || !!exit.signal
    }
  } finally {
    await provider.close()
    writeFileSync(join(root, 'report.json'), `${JSON.stringify(report, null, 2)}\n`, { mode: 0o600 })
    progress('installed-report', { passed: report.passed, path: join(root, 'report.json'), failure: report.failure })
  }
  return report
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const report = await runHostLocalInstalled(resolve(process.argv[2] || ''), resolve(process.argv[3] || ''))
  if (!report.passed) process.exitCode = 1
}
