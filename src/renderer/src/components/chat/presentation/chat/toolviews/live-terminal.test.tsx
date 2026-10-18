// @vitest-environment jsdom
import { createElement, act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { processDisplayBlockSchemaV1, type ProcessDisplayBlockV1 } from '../../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import { processTerminalCardModel, terminalFailed } from '../models/terminal-card-model'
import { ProcessToolCardBody } from './ToolCardBody'

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))

function shell(state: ProcessDisplayBlockV1['state'] = 'running', text = ''): ProcessDisplayBlockV1 {
  return { blockId: 'a'.repeat(32), kind: 'shell', state, attempt: 0,
    source: { kind: 'tool', unitId: 'b'.repeat(32), callId: 'call_host_' + 'c'.repeat(64),
      toolCallItemId: 'item_call_' + 'd'.repeat(64), startedEventSeq: '1',
      ...(state === 'running' ? {} : { toolResultItemId: 'item_result_' + 'e'.repeat(64) }) },
    title: 'actual command', durationMs: 1, truncated: false, classification: 'source_exact_local',
    ...(state === 'running' ? {} : { captureDigest: 'f'.repeat(64) }),
    ...(state === 'running' ? {} : { body: { text, before: '', after: '', error: '', addedLines: 0, removedLines: 0, approximate: false } }) }
}

describe('protected foreground terminal model and leaf', () => {
  let container: HTMLDivElement, root: Root
  beforeEach(() => { container = document.createElement('div'); document.body.append(container); root = createRoot(container) })
  afterEach(async () => { await act(async () => root.unmount()); container.remove() })
  const draw = async (block: ProcessDisplayBlockV1) => {
    const dto = processDisplayBlockSchemaV1.parse(block)
    await act(async () => root.render(createElement(ProcessToolCardBody, { block: dto })))
  }

  it('keeps a dispatched command banner output-free until real settlement', async () => {
    const first = shell()
    await draw(first)
    const card = container.querySelector('[data-terminal]')
    expect(card).not.toBeNull(); expect(card!.hasAttribute('data-running')).toBe(true)
    expect(card!.hasAttribute('data-body')).toBe(false)
    expect(container.textContent).toContain('actual command')
    expect(processTerminalCardModel(first)?.output).toBeUndefined()
    expect(processTerminalCardModel(first)?.exitCode).toBeUndefined()
    expect(processDisplayBlockSchemaV1.safeParse({ ...first, body: { ...shell('completed').body!, text: 'unsolicited output' } }).success).toBe(false)
    const output = Array.from({ length: 20 }, (_, index) => `actual line ${index + 1}`).join('\n') + '\n'
    await draw({ ...shell('completed', output), exitCode: 7 })
    expect(container.querySelector('[data-terminal]')).toBe(card)
    expect(card!.hasAttribute('data-running')).toBe(false)
    expect(container.textContent).toContain('actual line 10')
    expect(container.textContent).toContain('actual line 20')
    expect(card!.querySelector('[aria-label="protectedToolShowLines"]')).toBeNull()
    expect(terminalFailed(processTerminalCardModel(shell('completed'))!)).toBe(false)
    expect(terminalFailed(processTerminalCardModel({ ...shell('completed'), exitCode: 7 })!)).toBe(true)
  })

  it('keeps real tool errors generic and never infers a shell mode from output or truncation', async () => {
    for (const state of ['failed', 'timeout', 'canceled'] as const) {
      const block = shell(state, '[exit code: 0]\nsource output')
      block.body!.error = 'actual host error'
      expect(processTerminalCardModel(block)).toBeNull()
      await draw(block)
      expect(container.querySelector('[data-terminal]')).toBeNull()
      expect(container.querySelector('[data-tool-io]')?.textContent).toContain('INactual commandOUT')
      expect(container.textContent).toContain('actual host error')
    }
    expect(processTerminalCardModel({ ...shell('completed', '[spill footer]'), truncated: true })).not.toBeNull()
    expect(processTerminalCardModel({ ...shell('unknown'), classification: 'withheld', title: undefined, body: undefined })).toBeNull()
  })
})
