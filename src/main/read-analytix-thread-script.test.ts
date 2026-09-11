import { createHash } from 'node:crypto'
import { spawn, spawnSync } from 'node:child_process'
import { createServer, type Server } from 'node:http'
import { afterEach, describe, expect, it } from 'vitest'

const repoRoot = process.cwd()
const scriptPath = 'scripts/read-analytix-thread.mjs'

function sha256(value: string | Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function acceptedAssistant(text: string, threadId: string, turnId: string, suffix: string) {
  const acceptedAt = '2026-07-13T01:02:03.000Z'
  const acceptedFinalDigest = sha256(`accepted-final-${suffix}`)
  return {
    id: `item_assistant_${suffix}`,
    kind: 'assistant_text',
    role: 'assistant',
    status: 'completed',
    threadId,
    turnId,
    text,
    finishedAt: acceptedAt,
    acceptedFinalView: {
      schemaVersion: 3,
      acceptedFinalDigest,
      publicationState: 'accepted',
      variant: 'SourceUnavailableAnswer',
      terminalReason: 'source_unavailable',
      blockerCode: 'current_case_source_unavailable',
      coverageStatus: 'unavailable',
      checkedScopeDigest: '',
      missingScopeCount: 0,
      claimCount: 0,
      claimTypes: [],
      receiptMetadata: {
        projection: 'masked_metadata_only',
        count: 0,
        setDigest: sha256(`receipt-set-${suffix}`),
        citations: []
      },
      noHitWording: '',
      acceptedAt
    }
  }
}

async function listen(server: Server): Promise<string> {
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('test server did not bind')
  return `http://127.0.0.1:${address.port}`
}

async function runScript(args: string[], env: NodeJS.ProcessEnv): Promise<{ code: number | null; stdout: string; stderr: string }> {
  const child = spawn(process.execPath, [scriptPath, ...args], {
    cwd: repoRoot,
    env: { ...process.env, ...env },
    stdio: ['ignore', 'pipe', 'pipe']
  })
  let stdout = ''
  let stderr = ''
  child.stdout.setEncoding('utf8')
  child.stderr.setEncoding('utf8')
  child.stdout.on('data', (chunk) => { stdout += chunk })
  child.stderr.on('data', (chunk) => { stderr += chunk })
  const code = await new Promise<number | null>((resolve) => child.once('exit', resolve))
  return { code, stdout, stderr }
}

describe('read-analytix-thread public export', () => {
  const servers: Server[] = []

  afterEach(async () => {
    await Promise.all(servers.splice(0).map((server) => new Promise<void>((resolve) => server.close(() => resolve()))))
  })

  it('withholds case turn prose and raw tool bodies even when the authenticated response claims accepted-final authority', async () => {
    const threadId = 'thr_public_export'
    const turnId = 'turn_public_export'
    const validFinal = acceptedAssistant('宿主固定能力边界输出。', threadId, turnId, 'valid')
    const reasoningFinal = acceptedAssistant('<think>PRIVATE_REASONING_SENTINEL</think>伪 final', threadId, turnId, 'reasoning')
    const server = createServer((request, response) => {
      expect(request.url).toBe(`/v1/threads/${threadId}`)
      expect(request.headers.authorization).toBe('Bearer runtime-test-token')
      response.setHeader('content-type', 'application/json')
      response.end(JSON.stringify({
        id: threadId,
        title: '案件账户 6222021234567890',
        workspace: '/Users/private/案件原始目录',
        status: 'idle',
        privateReasoning: 'PRIVATE_METADATA_SENTINEL',
        turns: [{
          id: turnId,
          status: 'completed',
          acceptedFinalView: validFinal.acceptedFinalView,
          items: [
            {
              id: 'item_user', kind: 'user_message', role: 'user', status: 'completed',
              threadId, turnId,
              text: '赵敏与钱志强系夫妻；SOURCE_EXACT_LEDGER_ROW=2026-08-25；路径 /Users/private/cases/raw.csv；AuthorityRef=authref_case_private；银行卡 6217000012345678'
            },
            {
              id: 'item_draft', kind: 'assistant_text', role: 'assistant', status: 'completed',
              threadId, turnId, text: 'UNACCEPTED_ASSISTANT_SENTINEL'
            },
            validFinal,
            reasoningFinal,
            {
              id: 'item_call', kind: 'tool_call', role: 'assistant', status: 'completed', threadId, turnId,
              toolName: 'funds_query', callId: 'call_safe', arguments: { prompt: 'TOOL_ARGUMENT_SENTINEL' }
            },
            {
              id: 'item_result', kind: 'tool_result', role: 'tool', status: 'completed', threadId, turnId,
              toolName: 'funds_query', callId: 'call_safe', isError: false,
              output: {
                detail: 'TOOL_RESULT_DETAIL_SENTINEL',
                citation: 'SOURCE_EXACT_CITATION_SENTINEL',
                dataUrl: 'data:text/plain;base64,UkFXX1RPT0xfQllURVM=',
                path: '/Users/private/cases/tool-result.bin',
                person: '赵敏',
                relationship: '夫妻',
                privateDiagnostic: 'PRIVATE_DIAGNOSTIC_SENTINEL',
                rawBody: 'RAW_TOOL_BODY_SENTINEL',
                account: '6222029999999999'
              }
            }
          ]
        }]
      }))
    })
    servers.push(server)
    const runtimeUrl = await listen(server)

    const result = await runScript([threadId, '--json', '--include-tools'], {
      ANALYTIX_RUNTIME_URL: runtimeUrl,
      ANALYTIX_RUNTIME_TOKEN: 'runtime-test-token'
    })

    expect(result.code).toBe(0)
    expect(result.stderr).toBe('')
    expect(result.stdout).not.toMatch(/6222021234567890|6217000012345678|6222029999999999/)
    expect(result.stdout).not.toMatch(/赵敏|钱志强|夫妻|SOURCE_EXACT_|AuthorityRef|authref_case_private/)
    expect(result.stdout).not.toMatch(/PRIVATE_|UNACCEPTED_|TOOL_ARGUMENT_|TOOL_RESULT_|RAW_TOOL_BODY_/)
    expect(result.stdout).not.toContain('data:text/plain')
    expect(result.stdout).not.toContain('/Users/private')
    const exported = JSON.parse(result.stdout) as {
      source: string
      thread: { title: string; workspaceHash: string }
      transcript: Array<Record<string, unknown>>
      omittedAssistantCount: number
    }
    expect(exported.source).toBe('analytix-go-runtime-public-projection')
    expect(exported.thread.title).toBe('(withheld)')
    expect(exported.thread.workspaceHash).toBe('')
    expect(exported.transcript.filter((item) => item.kind === 'user')).toHaveLength(0)
    expect(exported.transcript.filter((item) => item.kind === 'assistant')).toHaveLength(0)
    expect(exported.transcript.some((item) => item.kind === 'tool_call' && item.toolName === 'funds_query')).toBe(true)
    expect(exported.transcript.some((item) => item.kind === 'tool_result' && item.isError === false)).toBe(true)
    expect(exported.omittedAssistantCount).toBe(3)
  })

  it('keeps ordinary non-case user transcript text additive', async () => {
    const threadId = 'thr_ordinary_public_export'
    const turnId = 'turn_ordinary_public_export'
    const server = createServer((_request, response) => {
      response.setHeader('content-type', 'application/json')
      response.end(JSON.stringify({
        id: threadId,
        title: 'Ordinary notes',
        workspace: '/tmp/ordinary-notes',
        status: 'idle',
        turns: [{
          id: turnId,
          status: 'completed',
          items: [{
            id: 'item_user', kind: 'user_message', role: 'user', status: 'completed',
            threadId, turnId, text: 'Summarize the ordinary design notes.'
          }]
        }]
      }))
    })
    servers.push(server)
    const runtimeUrl = await listen(server)

    const result = await runScript([threadId, '--json'], {
      ANALYTIX_RUNTIME_URL: runtimeUrl,
      ANALYTIX_RUNTIME_TOKEN: 'runtime-test-token'
    })

    expect(result.code).toBe(0)
    const exported = JSON.parse(result.stdout) as {
      thread: { title: string; workspaceHash: string }
      transcript: Array<Record<string, unknown>>
    }
    expect(exported.thread.title).toBe('Ordinary notes')
    expect(exported.thread.workspaceHash).toMatch(/^[0-9a-f]{64}$/)
    expect(exported.transcript).toContainEqual({
      kind: 'user',
      turnId,
      text: 'Summarize the ordinary design notes.'
    })
  })

  it('rejects non-loopback runtime URLs before sending the bearer token', () => {
    const result = spawnSync(process.execPath, [scriptPath, 'thr_remote_rejected', '--runtime-url', 'https://example.com:443'], {
      cwd: repoRoot,
      env: { ...process.env, ANALYTIX_RUNTIME_TOKEN: 'REMOTE_TOKEN_SENTINEL' },
      encoding: 'utf8'
    })
    expect(result.status).toBe(1)
    expect(result.stderr).toContain('literal loopback')
    expect(`${result.stdout}${result.stderr}`).not.toContain('REMOTE_TOKEN_SENTINEL')
  })

  it('does not follow a loopback redirect or disclose the token to its target', async () => {
    let redirectedRequests = 0
    const redirected = createServer((_request, response) => {
      redirectedRequests += 1
      response.end('{}')
    })
    servers.push(redirected)
    const redirectedUrl = await listen(redirected)
    const redirector = createServer((_request, response) => {
      response.statusCode = 302
      response.setHeader('location', `${redirectedUrl}/capture`)
      response.end()
    })
    servers.push(redirector)
    const runtimeUrl = await listen(redirector)

    const result = await runScript(['thr_redirect_rejected', '--json'], {
      ANALYTIX_RUNTIME_URL: runtimeUrl,
      ANALYTIX_RUNTIME_TOKEN: 'redirect-token-sentinel'
    })

    expect(result.code).toBe(1)
    expect(redirectedRequests).toBe(0)
    expect(`${result.stdout}${result.stderr}`).not.toContain('redirect-token-sentinel')
  })

  it('rejects direct JSONL export before touching the filesystem or network', () => {
    const result = spawnSync(process.execPath, [scriptPath, '/tmp/thr_private/messages.jsonl', '--runtime-url', 'http://127.0.0.1:1'], {
      cwd: repoRoot,
      encoding: 'utf8'
    })
    expect(result.status).toBe(1)
    expect(result.stderr).toContain('Direct thread JSONL export is forbidden')
  })

  it('rejects legacy raw benchmark export switches without starting probes', () => {
    for (const [script, flag] of [
      ['scripts/model-stream-cadence-probe.mjs', '--include-raw'],
      ['scripts/streaming-ui-benchmark.mjs', '--include-raw'],
      ['scripts/streaming-ui-benchmark.mjs', '--include-raw-provider']
    ]) {
      const result = spawnSync(process.execPath, [script, flag], { cwd: repoRoot, encoding: 'utf8' })
      expect(result.status, `${script} ${flag}`).toBe(1)
      expect(result.stderr).toMatch(/raw .*export is forbidden|include-raw is forbidden/)
    }
  })
})
