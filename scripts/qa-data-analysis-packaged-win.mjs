import { spawn } from 'node:child_process'
import fs from 'node:fs'
import net from 'node:net'
import os from 'node:os'
import path from 'node:path'

const appPath = process.env.ANALYTIX_APP_EXE || 'C:\\Program Files\\Analytix\\analytix\\analytix.exe'
const sampleDir = process.argv[2] || process.env.ANALYTIX_QA_SAMPLE_DIR || 'C:\\Users\\Public\\AnalytixTestDataLarge'
const progressPath = process.env.ANALYTIX_QA_PROGRESS_PATH || path.join(os.tmpdir(), 'analytix-data-analysis-packaged-win-progress.json')
const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

function hrMs(startedAt) {
  return Math.round(Number(process.hrtime.bigint() - startedAt) / 1_000_000)
}

async function getFreePort() {
  return await new Promise((resolve, reject) => {
    const server = net.createServer()
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      const port = address.port
      server.close(() => resolve(port))
    })
    server.on('error', reject)
  })
}

async function waitForTarget(debugPort, timeoutMs = 60_000) {
  const deadline = Date.now() + timeoutMs
  let last = ''
  while (Date.now() < deadline) {
    try {
      const targets = await fetch(`http://127.0.0.1:${debugPort}/json/list`).then((res) => res.json())
      const page = targets.find((target) => target.type === 'page' && target.webSocketDebuggerUrl)
      if (page) return page
    } catch (error) {
      last = error?.message || String(error)
    }
    await wait(500)
  }
  throw new Error(`debug target timeout: ${last}`)
}

async function evaluate(wsUrl, expression) {
  const ws = new WebSocket(wsUrl)
  await new Promise((resolve, reject) => {
    ws.addEventListener('open', resolve, { once: true })
    ws.addEventListener('error', reject, { once: true })
  })
  const result = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('Runtime.evaluate timeout')), 120_000)
    ws.addEventListener('message', (event) => {
      const message = JSON.parse(String(event.data))
      if (message.id !== 1) return
      clearTimeout(timer)
      resolve(message)
    })
    ws.send(JSON.stringify({
      id: 1,
      method: 'Runtime.evaluate',
      params: { expression, awaitPromise: true, returnByValue: true }
    }))
  })
  ws.close()
  if (result.result?.exceptionDetails) throw new Error(JSON.stringify(result.result.exceptionDetails))
  return result.result?.result?.value
}

function writeProgress(payload) {
  if (!progressPath) return
  try {
    fs.writeFileSync(progressPath, JSON.stringify(payload, null, 2), 'utf8')
  } catch {
    // Progress snapshots are best-effort and must not change QA behavior.
  }
}

async function requestJson(apiBase, method, route, body, steps, label) {
  const started = process.hrtime.bigint()
  let response
  try {
    response = await fetch(`${apiBase}${route}`, {
      method,
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json'
      },
      body: body === undefined ? undefined : JSON.stringify(body)
    })
  } catch (error) {
    const elapsedMs = hrMs(started)
    steps.push({
      label,
      method,
      route,
      status: 0,
      elapsedMs,
      error: error?.message || String(error)
    })
    writeProgress({ ok: false, phase: label, steps })
    throw error
  }
  const text = await response.text()
  let parsed = null
  try {
    parsed = text ? JSON.parse(text) : null
  } catch {
    parsed = { raw: text }
  }
  const elapsedMs = hrMs(started)
  steps.push({
    label,
    method,
    route,
    status: response.status,
    elapsedMs,
    serverTiming: response.headers.get('server-timing') || ''
  })
  writeProgress({ ok: true, phase: label, steps })
  if (!response.ok) {
    throw new Error(`${label} failed: HTTP ${response.status} ${text.slice(0, 1200)}`)
  }
  return parsed?.data ?? parsed
}

async function pollJob(apiBase, route, steps, label, timeoutMs = 20 * 60_000) {
  const started = process.hrtime.bigint()
  const deadline = Date.now() + timeoutMs
  let latest = null
  while (Date.now() < deadline) {
    latest = await requestJson(apiBase, 'GET', route, undefined, steps, `${label}:poll`)
    if (['succeeded', 'failed', 'canceled'].includes(latest?.status)) {
      steps.push({ label, status: latest.status, elapsedMs: hrMs(started), progress: latest.progress ?? null })
      if (latest.status !== 'succeeded') {
        throw new Error(`${label} ended with ${latest.status}: ${latest.error || ''}`)
      }
      return latest
    }
    await wait(1000)
  }
  throw new Error(`${label} timed out after ${timeoutMs}ms; latest=${JSON.stringify(latest)}`)
}

async function pollTask(apiBase, taskId, caseId, steps, label, timeoutMs = 30 * 60_000) {
  const started = process.hrtime.bigint()
  const deadline = Date.now() + timeoutMs
  let latest = null
  while (Date.now() < deadline) {
    latest = await requestJson(
      apiBase,
      'GET',
      `/api/v1/system/tasks/${encodeURIComponent(taskId)}?case_id=${encodeURIComponent(caseId)}`,
      undefined,
      steps,
      `${label}:poll`
    )
    if (['succeeded', 'failed', 'canceled'].includes(latest?.status)) {
      steps.push({ label, status: latest.status, elapsedMs: hrMs(started), progress: latest.progress ?? null })
      writeProgress({ ok: latest.status === 'succeeded', phase: label, steps, task: latest })
      if (latest.status !== 'succeeded') {
        throw new Error(`${label} ended with ${latest.status}: ${latest.error || ''}`)
      }
      return latest
    }
    await wait(1000)
  }
  throw new Error(`${label} timed out after ${timeoutMs}ms; latest=${JSON.stringify(latest)}`)
}

function listSampleFiles(root) {
  const files = fs.readdirSync(root, { withFileTypes: true })
    .filter((entry) => entry.isFile())
    .map((entry) => path.join(root, entry.name))
    .filter((filePath) => /\.(zip|rar|7z|csv|xlsx?)$/i.test(filePath))
    .sort((a, b) => a.localeCompare(b))
  if (files.length === 0) throw new Error(`no importable sample files found in ${root}`)
  return files
}

function importFileSpec(filePath) {
  return {
    file_name: path.basename(filePath),
    source_path: filePath
  }
}

function firstGroupKey(tree) {
  for (const group of tree?.groups ?? []) {
    for (const key of ['key', 'account_key', 'accountKey', 'id', 'name']) {
      const value = group?.[key]
      if (typeof value === 'string' && value.trim()) return value.trim()
    }
    const children = Array.isArray(group?.children) ? group.children : []
    for (const child of children) {
      for (const key of ['key', 'account_key', 'accountKey', 'id', 'name']) {
        const value = child?.[key]
        if (typeof value === 'string' && value.trim()) return value.trim()
      }
    }
  }
  return ''
}

const debugPort = await getFreePort()
const child = spawn(appPath, [`--remote-debugging-port=${debugPort}`], {
  env: {
    ...process.env,
    ANALYTIX_DATA_ANALYSIS_PORT: process.env.ANALYTIX_DATA_ANALYSIS_PORT || ''
  },
  stdio: ['ignore', 'pipe', 'pipe']
})
let stdout = ''
let stderr = ''
child.stdout.on('data', (chunk) => { stdout += String(chunk) })
child.stderr.on('data', (chunk) => { stderr += String(chunk) })

const steps = []
const qaContext = {
  backendState: null,
  apiBase: '',
  caseId: '',
  importJobId: '',
  cleaningJobId: '',
  analysisRefreshJobId: ''
}
let finalResult = null
try {
  const target = await waitForTarget(debugPort)
  const backendState = await evaluate(target.webSocketDebuggerUrl, `(async () => {
    const deadline = Date.now() + 30000
    while (Date.now() < deadline) {
      if (window.analytix?.dataAnalysis?.ensureBackend) {
        return await window.analytix.dataAnalysis.ensureBackend()
      }
      await new Promise((resolve) => setTimeout(resolve, 100))
    }
    throw new Error('window.analytix.dataAnalysis.ensureBackend unavailable')
  })()`)
  qaContext.backendState = backendState
  const apiBase = backendState?.apiBase
  if (!apiBase) throw new Error(`data analysis backend did not return apiBase: ${JSON.stringify(backendState)}`)
  qaContext.apiBase = apiBase

  const files = listSampleFiles(sampleDir)
  const caseNumber = `QA-${new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14)}`
  const createdCase = await requestJson(apiBase, 'POST', '/api/v1/cases', {
    case_name: `Windows QA ${caseNumber}`,
    case_number: caseNumber,
    owner: 'qa',
    note: 'packaged Windows data-analysis E2E',
    case_type: 'qa',
    tags: ['windows', 'packaged']
  }, steps, 'create-case')
  const caseId = createdCase.case_id
  qaContext.caseId = caseId

  const importFiles = files.map(importFileSpec)
  const preview = await requestJson(apiBase, 'POST', '/api/v1/import/files/preview', {
    case_id: caseId,
    files: importFiles
  }, steps, 'import-preview')

  const importJob = await requestJson(apiBase, 'POST', '/api/v1/import/jobs', {
    case_id: caseId,
    files: importFiles,
    auto_cleaning: false
  }, steps, 'import-create')
  qaContext.importJobId = importJob?.job_id || ''
  const importDone = await pollJob(apiBase, `/api/v1/import/jobs/${encodeURIComponent(importJob.job_id)}`, steps, 'import-job')

  const cleaningJob = await requestJson(apiBase, 'POST', '/api/v1/cleaning/jobs', {
    case_id: caseId,
    steps: [],
    force_rebuild: true
  }, steps, 'cleaning-create')
  qaContext.cleaningJobId = cleaningJob?.job_id || ''
  const cleaningDone = await pollJob(apiBase, `/api/v1/cleaning/jobs/${encodeURIComponent(cleaningJob.job_id)}`, steps, 'cleaning-job')

  const refreshQueued = await requestJson(apiBase, 'POST', '/api/v1/analysis/refresh', {
    case_id: caseId,
    force_refresh: true,
    mode: 'async'
  }, steps, 'analysis-refresh-queue')
  qaContext.analysisRefreshJobId = refreshQueued?.job_id || ''
  const refreshTask = refreshQueued?.job_id
    ? await pollTask(apiBase, refreshQueued.job_id, caseId, steps, 'analysis-refresh-job')
    : null
  const refresh = {
    ...refreshQueued,
    task_status: refreshTask?.status || null,
    task_progress: refreshTask?.progress ?? null
  }
  const cleaningStatus = cleaningDone?.status || cleaningDone?.job?.status || cleaningDone?.summary?.status || 'succeeded'

  const meta = await requestJson(apiBase, 'POST', '/api/v1/analysis/stats/v2/meta', {
    case_id: caseId
  }, steps, 'stats-meta')
  const overview = await requestJson(apiBase, 'POST', '/api/v1/analysis/stats/v2/case-overview', {
    case_id: caseId
  }, steps, 'stats-overview')
  const tree = await requestJson(apiBase, 'POST', '/api/v1/analysis/stats/v2/tree', {
    case_id: caseId,
    tab: 'byName'
  }, steps, 'stats-tree')
  const rows = await requestJson(apiBase, 'POST', '/api/v1/analysis/stats/v2/query/rows/direct', {
    case_id: caseId,
    mode: 'inAccount',
    row_limit: 50,
    row_format: 'object'
  }, steps, 'stats-rows-direct')
  const txnRows = await requestJson(apiBase, 'POST', '/api/v1/analysis/stats/v2/query/txn-rows/direct', {
    case_id: caseId,
    limit: 50,
    row_format: 'object'
  }, steps, 'stats-txn-rows-direct')
  const chart = await requestJson(apiBase, 'POST', '/api/v1/analysis/stats/v2/chart-dashboard', {
    case_id: caseId,
    selected: [],
    date_start: '',
    date_end: '',
    metric_mode: 'amount',
    direction_mode: 'all',
    granularity: 'day',
    success_filter: 'all',
    cash_filter: 'all',
    chart_filters: [],
    panel_views: []
  }, steps, 'stats-chart-dashboard')

  const seed = firstGroupKey(tree) || firstGroupKey({ groups: rows?.rows ?? [] })
  let flow = null
  if (seed) {
    flow = await requestJson(apiBase, 'POST', '/api/v1/analysis/flow/graph', {
      case_id: caseId,
      seeds: [seed],
      depth: 2,
      direction: 'both',
      min_amount: 0
    }, steps, 'flow-graph')
  }

  finalResult = {
    ok: true,
    appPath,
    sampleDir,
    progressPath,
    apiBase,
    backendState,
    caseId,
    files: files.map((filePath) => ({ filePath, size: fs.statSync(filePath).size })),
    preview: {
      items: preview?.items?.length ?? 0,
      archiveChildren: (preview?.items ?? []).reduce((sum, item) => sum + (item.archive_children?.length ?? 0), 0),
      statuses: (preview?.items ?? []).map((item) => ({ file: item.file_name, status: item.status, issue: item.issue }))
    },
    importSummary: importDone.summary,
    importFiles: importDone.files,
    cleaningStatus,
    cleaningSummary: cleaningDone.summary,
    cleanedRows: cleaningDone.cleaned_rows,
    refresh,
    meta,
    overview,
    statsRows: {
      total: rows?.total ?? rows?.row_summary?.total ?? 0,
      rows: rows?.rows?.length ?? 0,
      fields: rows?.row_fields ?? []
    },
    txnRows: {
      rows: txnRows?.rows?.length ?? 0,
      done: txnRows?.done ?? null,
      fields: txnRows?.row_fields ?? []
    },
    chartSummaryKeys: Object.keys(chart?.summary ?? {}),
    flow: flow
      ? {
          seed,
          nodes: flow.nodes?.length ?? 0,
          edges: flow.edges?.length ?? 0,
          stats: flow.stats ?? {}
        }
      : { skipped: true, reason: 'no seed key found from stats tree/rows' },
    steps,
    p50StepMs: percentile(steps.map((step) => step.elapsedMs).filter(Number.isFinite), 0.5),
    p95StepMs: percentile(steps.map((step) => step.elapsedMs).filter(Number.isFinite), 0.95)
  }
  console.log(JSON.stringify(finalResult, null, 2))
  const transactionCandidates = [
    overview.transaction_count,
    overview.summary?.transaction_count,
    overview.stats?.transaction_count,
    txnRows?.total,
    txnRows?.row_summary?.total,
    importDone.summary?.persistence?.norm_rows_by_kind?.fc_transaction,
    cleaningDone.cleaned_rows,
  ].map((value) => Number(value || 0))
  const transactionCount = transactionCandidates.find((value) => value > 0) || 0
  const importedFileCount = Number(
    importDone.imported_files
      ?? importDone.files?.length
      ?? importDone.summary?.total_files
      ?? importDone.summary?.file_count
      ?? 0
  )
  writeProgress(finalResult)
  if (transactionCount <= 0 || importedFileCount <= 0 || cleaningStatus !== 'succeeded') {
    process.exitCode = 2
  }
} catch (error) {
  finalResult = {
    ok: false,
    appPath,
    sampleDir,
    progressPath,
    context: qaContext,
    error: {
      name: error?.name || '',
      message: error?.message || String(error),
      stack: error?.stack || ''
    },
    steps,
    p50StepMs: percentile(steps.map((step) => step.elapsedMs).filter(Number.isFinite), 0.5),
    p95StepMs: percentile(steps.map((step) => step.elapsedMs).filter(Number.isFinite), 0.95)
  }
  writeProgress(finalResult)
  console.log(JSON.stringify(finalResult, null, 2))
  process.exitCode = process.exitCode || 1
} finally {
  child.kill('SIGTERM')
  setTimeout(() => child.kill('SIGKILL'), 3000).unref()
  if (process.exitCode && (stdout || stderr)) {
    console.error(JSON.stringify({ stdoutTail: stdout.slice(-4000), stderrTail: stderr.slice(-4000) }, null, 2))
  }
}

function percentile(values, p) {
  if (values.length === 0) return 0
  const sorted = [...values].sort((a, b) => a - b)
  const index = Math.min(sorted.length - 1, Math.max(0, Math.ceil(sorted.length * p) - 1))
  return sorted[index]
}
