// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toolResultLocalDisplaySchemaV1, type ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { normalizeAppSettings, type AppSettingsV1 } from '@shared/app-settings'
import i18n from '../../../../../i18n'
import { ToolResultInspector, type ToolResultInspectorPresentation } from './ToolResultInspector'
import { parse } from 'postcss'

const callId = 'call_host_' + 'a'.repeat(64)
const snapshot = (): ToolResultLocalDisplayV1 => ({ schemaVersion: 1, snapshotDigest: 'b'.repeat(64), resultItemId: 'item_result_' + 'c'.repeat(64),
  toolName: 'web_fetch', canOpenWeb: { request: true, result: true }, capture: { kind: 'web_fetch', status: 'completed', label: 'web_fetch',
    body: 'ACTUAL_RESPONSE\nsecond line', durationMs: 41, truncated: false, startLine: 0, endLine: 0, totalLines: 0, web: {
      requestUrl: 'https://request.invalid/path', finalUrl: 'https://response.invalid/path', inputJSON: '{"url":"https://request.invalid/path","nested":{"actual":true}}',
      schemaJSON: '{"name":"web_fetch","description":"Actual schema description","parameters":{"type":"object","properties":{"url":{"type":"string"}}}}',
      statusCode: 200, fetchTruncated: false, responseComplete: true, toolIsError: false, startedAt: '2026-10-08T00:00:00Z',
      inputTruncated: false, schemaTruncated: false, requestUrlTruncated: false, finalUrlTruncated: false
    } } })
let root: Root, container: HTMLDivElement
const close = vi.fn(), copy = vi.fn()
const draw = async (display = snapshot(), presentation?: ToolResultInspectorPresentation) => { await act(async () => root.render(createElement(ToolResultInspector, { display, callId, onClose: close, onCopy: copy, copied: false, pending: false, presentation }))) }
const click = async (label: string) => {
  const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent?.trim() === label || node.getAttribute('aria-label') === label)
  expect(button, label).toBeDefined(); await act(async () => button!.click())
}
beforeEach(async () => { vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en'); container = document.createElement('div'); document.body.append(container); root = createRoot(container); close.mockReset(); copy.mockReset() })
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('pinned DSH details leaf over a real authorized capture', () => {
  it('shows the typed file receipt with original request facts and no copy action', async () => {
    const display: ToolResultLocalDisplayV1 = { schemaVersion: 1, snapshotDigest: 'b'.repeat(64), resultItemId: 'item_result_' + 'c'.repeat(64), toolName: 'move_file',
      inspect: { inputJSON: '{"source_path":"ORIGINAL_REQUEST"}', inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null },
      capture: { kind: 'file_result', status: 'completed', body: '', label: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false,
        fileResult: { operation: 'move_file', classification: 'source_exact_local', move: {
          sourcePath: '/synthetic/ORIGINAL_FILE', destinationPath: '/synthetic/ORIGINAL_FILE', sourceRelativePath: 'ORIGINAL_FILE',
          destinationRelativePath: 'ORIGINAL_FILE', bytesMoved: '0', moved: false, message: 'ACTUAL_NOOP' } } } }
    await draw(display); await click('Result')
    for (let index = 0; index < 12; index++) {
      const expand = container.querySelector<HTMLButtonElement>('[data-json-node] button[aria-expanded="false"]')
      if (!expand) break
      await act(async () => expand.click())
    }
    const pane = container.querySelector('[role="tabpanel"]')!
    expect(pane.textContent).toContain('ACTUAL_NOOP'); expect(pane.textContent).toContain('false')
    expect(pane.hasAttribute('data-protected-tool-readonly')).toBe(true)
    expect(container.textContent).not.toContain('Copy'); expect(copy).not.toHaveBeenCalled()
    await click('Payload'); expect(container.textContent).toContain('ORIGINAL_REQUEST')
  })
  it('shows the original Schedule receipt beside common facts without reading a current catalog', async () => {
    const task = normalizeAppSettings({ schedule: { tasks: [{ id: 'task', title: 'HISTORICAL_TITLE', prompt: 'Synthetic prompt' }] } } as AppSettingsV1).schedule.tasks[0]
    const read = vi.fn(); Object.assign(window, { analytix: { schedule: { listTasks: read } } })
    const display: ToolResultLocalDisplayV1 = { schemaVersion: 1, snapshotDigest: 'b'.repeat(64), resultItemId: 'item_result_' + 'c'.repeat(64), toolName: 'mcp__gui_schedule__gui_schedule_create',
      inspect: { inputJSON: '{"title":"REQUESTED_TITLE"}', inputTruncated: false, schemaTruncated: true, callEventAt: null, resultEventAt: null, durationMs: null },
      capture: { kind: 'schedule', status: 'completed', label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false,
        schedule: { operation: 'create', status: 'success', message: '', task } } }
    await draw(display); await click('Payload'); expect(container.textContent).toContain('REQUESTED_TITLE')
    await click('Result')
    for (let count = 0; count < 12; count++) {
      const expand = container.querySelector<HTMLButtonElement>('[data-json-node] button[aria-expanded="false"]')
      if (!expand) break
      await act(async () => expand.click())
    }
    expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('HISTORICAL_TITLE')
    expect(container.textContent).not.toContain('Copy'); expect(copy).not.toHaveBeenCalled(); expect(read).not.toHaveBeenCalled()
  })
  it('focuses the exact call and shows real Payload/Result/Schema/Timing without fabricated data', async () => {
    await draw()
    expect(container.querySelector('[data-tool-result-inspector]')?.getAttribute('data-call-id')).toBe(callId)
    expect(document.activeElement?.textContent).toBe('Summary')
    await click('Payload'); expect(container.textContent).toContain('https://request.invalid/path')
    await click('Copy'); expect(copy).toHaveBeenLastCalledWith('payload')
    await click('Result'); expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('ACTUAL_RESPONSE')
    await click('Copy'); expect(copy).toHaveBeenLastCalledWith('result')
    await click('Schema'); expect(container.textContent).toContain('Actual schema description')
    await click('Copy'); expect(copy).toHaveBeenLastCalledWith('schema')
    await click('Timing'); expect(container.textContent).toContain('2026-10-08T00:00:00Z'); expect(container.textContent).toContain('41 ms')
    await click('Close details'); expect(close).toHaveBeenCalledOnce()
  })
  it('keeps wrap and JSON node expansion in the same leaf and supports tab keyboard navigation', async () => {
    await draw(); await click('Payload')
    const wrapper = container.querySelector<HTMLButtonElement>('[aria-pressed]')!
    expect(wrapper.getAttribute('aria-pressed')).toBe('true')
    await act(async () => wrapper.click()); expect(wrapper.getAttribute('aria-pressed')).toBe('false')
    expect(container.querySelectorAll('[data-json-node]')).toHaveLength(3)
    await click('Expand node'); expect(container.textContent).toContain('true')
    const payloadTab = container.querySelector<HTMLButtonElement>('[role="tab"][aria-selected="true"]')!
    await act(async () => payloadTab.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true })))
    expect(container.querySelector('[role="tab"][aria-selected="true"]')?.textContent).toBe('Result')
    expect(document.activeElement?.textContent).toBe('Result')
    await act(async () => container.querySelector('[data-tool-result-inspector]')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
    expect(close).toHaveBeenCalledOnce()
  })

  const lossyJson = [
    ['large integers', '{ "value": 9007199254740993, "nested": [-9007199254740993] }'],
    ['number spelling', '{ "value": 1.2300, "nested": [1e+03, -0, 1e400, 1e-400] }'],
    ['duplicate keys', '{ "value": 1, "value": 2, "nested": { "key": 3, "key": 4 } }'],
    ['integer key ordering', '{ "2": "second", "1": "first", "nested": { "4": 4, "3": 3 } }'],
    ['string escapes', String.raw`{ "value": "\u0041\/", "nested": ["\u003cscript\u003e"] }`]
  ] as const
  it.each(lossyJson.flatMap(([name, value]) => ['Payload', 'Result'].map(section => ({ name, value, section }))))(
    'preserves $name in $section as complete original text', async ({ name, value, section }) => {
      const display = snapshot()
      if (display.capture.kind !== 'web_fetch') throw new Error('fixture requires Web')
      if (section === 'Payload') display.capture.web.inputJSON = value
      else display.capture.body = value
      await draw(display); await click(section)
      if (name === 'large integers') expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('9007199254740993')
      expect(container.querySelector('[role="tabpanel"] pre')?.textContent).toBe(value)
      expect(container.querySelectorAll('[data-json-node]')).toHaveLength(0)
    })
  it.each(lossyJson)('preserves %s in the complete model schema without bypassing the text fallback', async (_name, parameters) => {
    const value = '{ "name": "bash", "description": "Actual schema", "parameters": ' + parameters + ' }'
    const display = snapshot()
    display.inspect = { schemaJSON: value, inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null }
    await draw(display); await click('Schema')
    expect(container.querySelector('[role="tabpanel"] pre')?.textContent).toBe(value)
    expect(container.querySelectorAll('[data-json-node]')).toHaveLength(0)
    expect(container.textContent).not.toContain('Copy')
  })
  it.each(['quote " with spaces', 'ends\\', '\\"', '\\\\"'])('keeps harmless formatting and escaped string %j in the small tree', async text => {
    const value = JSON.stringify({ text, numericText: '9007199254740993', whitespace: ' \t\r\n ', safeNumber: 0.5 }, null, 2)
    const display = snapshot(); display.capture.body = value
    await draw(display); await click('Result')
    expect(container.querySelector('[role="tabpanel"] pre')).toBeNull()
    expect(container.querySelectorAll('[data-json-node]')).toHaveLength(5)
    expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain(JSON.stringify(text))
    expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('"9007199254740993"')
    expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('" \\t\\r\\n "')
  })
  it.each([' 9007199254740993\n', ' actual non-JSON text\t', '{ "broken": }\n'])('keeps non-tree text %j unchanged', async value => {
    const display = snapshot(); display.capture.body = value
    await draw(display); await click('Result')
    expect(container.querySelector('[role="tabpanel"] pre')?.textContent).toBe(value)
  })
  it.each([false, true])('escapes HTML-looking strings in the %s text fallback', async fallback => {
    const text = '<img src=x onerror=alert(1)><script>alert(2)</script>'
    const value = fallback ? '{ "text": ' + JSON.stringify(text) + ', "value": 9007199254740993 }' : JSON.stringify({ text })
    const display = snapshot(); display.capture.body = value
    await draw(display); await click('Result')
    expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain(text)
    expect(container.querySelectorAll('img, script')).toHaveLength(0)
    expect(container.querySelector('[role="tabpanel"] pre')?.textContent ?? null).toBe(fallback ? value : null)
  })
  it('keeps a large flat JSON response complete as wrapped text instead of mounting hundreds of thousands of nodes', async () => {
    const display = snapshot()
    const body = '[' + '0,'.repeat(100_000) + '0]'
    display.capture.body = body
    await draw(display); await click('Result')
    expect(container.querySelectorAll('[data-json-node]')).toHaveLength(0)
    expect(container.querySelector('[role="tabpanel"] pre')?.textContent).toBe(body)
    expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('JSON shown as text')
    await click('Copy'); expect(copy).toHaveBeenLastCalledWith('result')
  })
  it('shows absent or omitted fields as unavailable/capped and offers no unsupported copy', async () => {
    const display = snapshot()
    if (display.capture.kind !== 'web_fetch') throw new Error('fixture requires Web')
    display.capture.web.schemaJSON = undefined; display.capture.web.inputJSON = undefined; display.capture.web.inputTruncated = true
    await draw(display); await click('Schema')
    expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('Unavailable')
    expect([...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Copy')?.disabled).toBe(true)
    await click('Payload'); expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('Local capture is incomplete')
    expect(copy).not.toHaveBeenCalled()
  })
  it.each(['goal', 'todo', 'skill'] as const)('limits the %s observation to its real Result and identity without fabricating request/schema/timing', async kind => {
    const timestamp = '2026-10-09T00:00:00Z'
    const base = { schemaVersion: 1 as const, resultItemId: 'item_result_' + 'c'.repeat(64), snapshotDigest: 'b'.repeat(64), canNavigate: false }
    const reserved = { status: 'completed', label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false } as const
    const display: ToolResultLocalDisplayV1 = kind === 'goal' ? { ...base, toolName: 'get_goal', capture: { ...reserved, kind, goal: {
      goal: { threadId: 'thread', objective: 'ACTUAL_GOAL', status: 'active', tokensUsed: 2, timeUsedSeconds: 4,
        createdAt: timestamp, updatedAt: timestamp }, remainingTokens: null } } } : kind === 'todo' ? { ...base, toolName: 'todo_write',
      capture: { ...reserved, kind, todo: { todos: { threadId: 'thread', items: [], updatedAt: timestamp }, baseline: { availability: 'unavailable' } } } } :
      { ...base, toolName: 'run_skill', capture: { ...reserved, kind, body: 'ACTUAL_INSTRUCTION', skill: { requestedName: 'Actual skill',
        resolved: { id: 'skill-a', name: 'Actual skill', runAs: 'inline', packageDigest: 'd'.repeat(64) }, child: null, error: null } } }
    await draw(display)
    expect(container.textContent).toContain(display.snapshotDigest)
    expect([...container.querySelectorAll('[role="tab"]')].map(node => node.textContent)).toEqual(['Summary', 'Result'])
    await click('Result')
    const resultTab = container.querySelector<HTMLButtonElement>('[role="tab"][aria-selected="true"]')!
    await act(async () => resultTab.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true })))
    expect(document.activeElement?.textContent).toBe('Summary')
    await click('Result')
    for (let count = 0; count < 20; count++) {
      const button = container.querySelector<HTMLButtonElement>('[data-json-node] button[aria-expanded="false"]')
      if (!button) break
      await act(async () => button.click())
    }
    expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain(kind === 'goal' ? 'ACTUAL_GOAL' : kind === 'todo' ? 'unavailable' : 'ACTUAL_INSTRUCTION')
    expect(container.textContent).not.toMatch(/Copy|Save|Payload|Schema|Timing|0 ms|Host execution/)
    expect(copy).not.toHaveBeenCalled()
  })
  it('uses the original common facts for an ordinary shell call and grants no input/schema export', async () => {
    const display: ToolResultLocalDisplayV1 = { schemaVersion: 1, snapshotDigest: 'b'.repeat(64), resultItemId: 'item_result_' + 'c'.repeat(64), toolName: 'bash',
      capture: { kind: 'shell', status: 'completed', body: 'ACTUAL_RESULT', label: 'execution label', durationMs: 999, truncated: false, startLine: 0, endLine: 0, totalLines: 0 },
      inspect: { inputJSON: '{"command":"MODEL_INPUT"}', schemaJSON: '{"name":"bash","description":"MODEL_TIME_SCHEMA","parameters":{"type":"object"}}', inputTruncated: false, schemaTruncated: false,
        callEventAt: '2026-10-10T00:00:00Z', resultEventAt: '2026-10-10T00:00:00.125Z', durationMs: 125 } }
    await draw(display)
    expect([...container.querySelectorAll('[role="tab"]')].map(node => node.textContent)).toEqual(['Summary', 'Payload', 'Result', 'Schema', 'Timing'])
    await click('Payload'); expect(container.textContent).toContain('MODEL_INPUT')
    expect(container.querySelector('[role="tabpanel"][data-protected-tool-readonly]')).not.toBeNull()
    expect(container.textContent).not.toContain('Copy')
    await click('Schema'); expect(container.textContent).toContain('Model tool schema'); expect(container.textContent).toContain('MODEL_TIME_SCHEMA')
    expect(container.textContent).not.toContain('Copy')
    await click('Timing'); expect(container.textContent).toContain('2026-10-10T00:00:00.125Z'); expect(container.textContent).toContain('125 ms')
    expect(container.textContent).not.toMatch(/999 ms|Execution duration/)
    await click('Result'); expect(container.textContent).toContain('ACTUAL_RESULT')
    await click('Copy'); expect(copy).toHaveBeenCalledExactlyOnceWith('result')
  })
  it('labels legacy Web schema and measurements separately from unavailable model/event facts', async () => {
    await draw(); await click('Schema')
    expect(container.textContent).toContain('Execution schema'); expect(container.textContent).not.toContain('Model tool schema')
    await click('Timing')
    const rows = [...container.querySelectorAll('[role="tabpanel"] dl > div')].map(node => node.textContent)
    expect(rows).toContain('Call eventUnavailable'); expect(rows).toContain('Result eventUnavailable'); expect(rows).toContain('DurationUnavailable')
    expect(rows).toContain('Execution duration41 ms'); expect(rows).toContain('Execution started2026-10-08T00:00:00Z')
  })
  it.each(['job', 'inspect'] as const)('keeps %s typed result and nullable common facts read-only', async kind => {
    const reserved = { status: 'completed', body: '', label: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false } as const
    const name = kind === 'job' ? 'bash_output' : 'session_event_read'
    const display: ToolResultLocalDisplayV1 = { schemaVersion: 1, resultItemId: 'item_result_' + 'c'.repeat(64), snapshotDigest: 'b'.repeat(64), toolName: name,
      inspect: { inputJSON: '{"original":true}', inputTruncated: false, schemaTruncated: true, callEventAt: null, resultEventAt: null, durationMs: null },
      capture: kind === 'job' ? { ...reserved, kind, job: { operation: 'output', jobs: [{ jobId: 'job-a', kind: 'bash', status: 'running', name: null, label: null,
        output: { availability: 'available', body: 'ACTUAL_JOB_OUTPUT', truncated: true } }], total: 1, truncated: false, receipt: null } } : { ...reserved, kind } }
    await draw(display); await click('Payload'); expect(container.textContent).toContain('true')
    await click('Schema'); expect(container.textContent).toContain('Local capture is incomplete')
    await click('Timing'); expect(container.textContent).toContain('Unavailable'); expect(container.textContent).not.toContain('0 ms')
    await click('Result')
    for (let count = 0; count < 30; count++) {
      const button = container.querySelector<HTMLButtonElement>('[data-json-node] button[aria-expanded="false"]')
      if (!button) break
      await act(async () => button.click())
    }
    expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain(kind === 'job' ? 'ACTUAL_JOB_OUTPUT' : 'Unavailable')
    expect(container.textContent).not.toContain('Copy'); expect(copy).not.toHaveBeenCalled()
  })
  it('does not replace omitted common facts with legacy Web inputs, schemas or durations', async () => {
    const display = snapshot()
    if (display.capture.kind !== 'web_fetch') throw new Error('fixture requires Web')
    delete display.capture.web.inputJSON; delete display.capture.web.schemaJSON
    display.inspect = { inputTruncated: true, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null }
    await draw(display); await click('Payload'); expect(container.textContent).toContain('Local capture is incomplete')
    await click('Schema'); expect(container.textContent).toContain('Unavailable')
    await click('Timing'); expect(container.textContent).not.toContain('41 ms'); expect(container.textContent).not.toContain('2026-10-08')
    expect(copy).not.toHaveBeenCalled()
  })
})


it('shows Present Result independently from original request and excludes private Host targets', async () => {
  const display: ToolResultLocalDisplayV1 = { schemaVersion: 1, snapshotDigest: 'b'.repeat(64), resultItemId: 'item_result_' + 'c'.repeat(64),
    toolName: 'present', inspect: { inputJSON: '{"files":[{"path":"REQUESTED_FILE.md"}]}', inputTruncated: false, schemaTruncated: true,
      callEventAt: null, resultEventAt: null, durationMs: null },
    capture: { kind: 'present', status: 'completed', label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0,
      truncated: false, present: { files: [{ index: 0, path: 'ACTUAL_FILE.md', description: 'Actual declaration' }] } } }
  await draw(display); await click('Payload')
    for (let count = 0; count < 12; count++) {
      const expand = container.querySelector<HTMLButtonElement>('[data-json-node] button[aria-expanded="false"]')
      if (!expand) break
      await act(async () => expand.click())
    }
    expect(container.textContent).toContain('REQUESTED_FILE.md')
  await click('Result')
  for (let count = 0; count < 12; count++) {
    const expand = container.querySelector<HTMLButtonElement>('[data-json-node] button[aria-expanded="false"]')
    if (!expand) break
    await act(async () => expand.click())
  }
  const panel = container.querySelector('[role="tabpanel"]')!
  expect(panel.textContent).toContain('ACTUAL_FILE.md')
  expect(panel.textContent).not.toContain('REQUESTED_FILE.md')
  expect(panel.hasAttribute('data-protected-tool-readonly')).toBe(true)
  expect(container.textContent).not.toContain('Copy'); expect(copy).not.toHaveBeenCalled()
})


it('formats generic success only as literal text on the selected Result tab with unchanged readonly effects', async () => {
  const body = '{ "n": 9007199254740993, "html": "<img onerror=unsafe()>" }\r\nsecond'
  const display: ToolResultLocalDisplayV1 = { schemaVersion: 1, snapshotDigest: 'b'.repeat(64), resultItemId: 'item_result_' + 'c'.repeat(64), toolName: 'mcp__docs__lookup',
    capture: { kind: 'inspect', status: 'completed', body: '', label: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false },
    inspect: { outputText: { availability: 'available', body, classification: 'source_exact_local', truncated: true }, inputJSON: '{}', inputTruncated: false, schemaTruncated: false,
      callEventAt: null, resultEventAt: null, durationMs: null } }
  await draw(display)
  expect(container.querySelector('[data-tool-text-output]')).toBeNull()
  await click('Result')
  const output = container.querySelector<HTMLPreElement>('[data-tool-text-output]')!
  expect(output.textContent).toBe(body)
  expect(output.style.whiteSpace).toBe('pre-wrap')
  await click('Wrap strings')
  expect(output.style.whiteSpace).toBe('pre'); expect(output.style.wordBreak).toBe('normal')
  expect(output.textContent).toBe(body)
  await click('Wrap strings')
  expect(output.style.whiteSpace).toBe('pre-wrap'); expect(output.style.wordBreak).toBe('break-word')
  expect(container.querySelector('[data-tool-text-limit]')?.textContent).toContain('32')
  expect(container.querySelectorAll('[data-json-node],img')).toHaveLength(0)
  expect(container.querySelector('[role="tabpanel"]')?.hasAttribute('data-protected-tool-readonly')).toBe(true)
  expect(container.textContent).not.toContain('Copy'); expect(copy).not.toHaveBeenCalled()
  await click('Summary'); expect(container.querySelector('[data-tool-text-output]')).toBeNull()
  await draw({ ...display, inspect: { ...display.inspect!, outputText: undefined } }); await click('Result')
  expect(container.querySelector('[data-tool-text-output]')).toBeNull()
})

it('shows the sealed error in embedded Result without a contradictory unavailable placeholder or export action', async () => {
  const message = 'Original failure\r\n9007199254740993 <img src=x onerror=unsafe()>'
  const display: ToolResultLocalDisplayV1 = { schemaVersion: 1, snapshotDigest: 'b'.repeat(64), resultItemId: 'item_result_' + 'c'.repeat(64), toolName: 'read',
    capture: { kind: 'inspect', status: 'failed', body: '', label: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false },
    inspect: { outputError: { availability: 'available', entries: [{ message, classification: 'source_exact_local' }] },
      inputJSON: '{}', inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null } }
  expect(toolResultLocalDisplaySchemaV1.safeParse(display).success).toBe(true)
  await draw(display, 'embedded')
  expect(container.querySelector('[data-tool-raw-error]')).toBeNull()
  await click('Result')
  const error = container.querySelector<HTMLPreElement>('[data-tool-raw-error]')!
  expect(error.textContent).toBe(message)
  expect(error.style.whiteSpace).toBe('pre-wrap')
  await click('Wrap strings')
  expect(error.style.whiteSpace).toBe('pre'); expect(error.style.wordBreak).toBe('normal')
  expect(error.textContent).toBe(message)
  await click('Wrap strings')
  expect(error.style.whiteSpace).toBe('pre-wrap'); expect(error.style.wordBreak).toBe('break-word')
  expect(error.closest('[data-protected-tool-readonly]')).not.toBeNull()
  expect(container.querySelector('[role="tabpanel"]')?.textContent).not.toContain('Unavailable')
  expect(container.querySelectorAll('img,[data-json-node]')).toHaveLength(0)
  expect(container.textContent).not.toContain('Copy'); expect(copy).not.toHaveBeenCalled()
  await click('Payload'); expect(container.querySelector('[data-tool-raw-error]')).toBeNull()
  await click('Result'); expect(container.querySelectorAll('[data-tool-raw-error]')).toHaveLength(1)
  await draw({ ...display, inspect: { ...display.inspect!, outputError: { availability: 'unavailable', reason: 'limit' } } }, 'embedded')
  expect(container.querySelector('[data-tool-raw-error]')).toBeNull()
  expect(container.querySelector('[data-tool-raw-error-unavailable]')?.textContent).toBe('Local capture is incomplete')
  expect(container.textContent).not.toContain(message)
})

it('keeps a typed Result beside its sealed error and distinguishes an actual empty result from missing data', async () => {
  const display = snapshot()
  if (display.capture.kind !== 'web_fetch') throw new Error('fixture requires Web')
  display.capture.web.statusCode = 500; display.capture.web.toolIsError = true
  delete display.capture.web.inputJSON; delete display.capture.web.schemaJSON
  display.inspect = { outputError: { availability: 'available', entries: [{ message: 'Original fetch failure', classification: 'credential_redacted' }] },
    inputJSON: '{}', inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null }
  expect(toolResultLocalDisplaySchemaV1.safeParse(display).success).toBe(true)
  await draw(display, 'embedded'); await click('Result')
  expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('ACTUAL_RESPONSE')
  expect(container.querySelector('[data-tool-raw-error]')?.textContent).toBe('Original fetch failure')
  const empty = snapshot(); empty.capture.body = ''
  await draw(empty, 'embedded')
  expect(container.querySelector('[role="tabpanel"]')?.textContent).toBe('No visible output')
  expect(container.querySelector('[data-tool-raw-error]')).toBeNull()
  expect(container.querySelector('[role="tabpanel"] pre')).toBeNull()
})

it('keeps the default overlay width/resize while embedding the same content in the containing pane', async () => {
  await draw()
  const pane = container.querySelector<HTMLElement>('[data-tool-result-inspector]')!
  expect(pane.getAttribute('data-presentation')).toBe('overlay')
  expect(pane.style.width).toBe('440px')
  expect(pane.querySelector('[aria-label="Resize tool details"]')).not.toBeNull()
  await draw(snapshot(), 'embedded')
  expect(container.querySelector('[data-tool-result-inspector]')).toBe(pane)
  expect(pane.getAttribute('data-presentation')).toBe('embedded')
  expect(pane.style.width).toBe('')
  expect(pane.querySelector('[aria-label="Resize tool details"]')).toBeNull()
  await click('Result')
  expect(pane.textContent).toContain('ACTUAL_RESPONSE')
  await click('Close details')
  expect(close).toHaveBeenCalledOnce()
})
it('keeps embedded layout in flow and protects detail typography from its containing public summary', async () => {
  const { readFileSync } = await vi.importActual<{ readFileSync: (path: string, encoding: 'utf8') => string }>('node:fs')
  const css = parse(readFileSync('src/renderer/src/components/chat/presentation/chat/toolviews/ToolResultInspector.module.css', 'utf8'))
  const values = (selector: string) => {
    const declarations: Record<string, string> = {}
    css.walkRules(selector, rule => { rule.walkDecls(declaration => { declarations[declaration.prop] = declaration.value }) })
    return declarations
  }
  expect(values('.details').position).toBe('fixed')
  expect(values('.details[data-presentation="embedded"]')).toMatchObject({ position: 'relative', inset: 'auto', width: '100%', 'max-width': '100%', 'z-index': 'auto' })
  expect(values('.details[data-presentation="embedded"] .overview').display).toBe('flex')
  expect(values('.details[data-presentation="embedded"] .overview dd').font).toBe('inherit')
})
