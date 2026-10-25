// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToolResultTurnItem } from '../../../../../packages/runtime/src/contracts/items'
import type { GeneratedArtifactMetadata } from '../../../../../packages/runtime/src/contracts/generated-artifact'
import { chatBlockFromItem } from '../../agent/analytix-mapper'
import type { CoreTurnItemJson } from '../../agent/analytix-contract'
import type { ToolBlock } from '../../agent/types'
import i18n from '../../i18n'
import { useChatStore } from '../../store/chat-store'
import { GeneratedFilesPanel } from './message-timeline-bubbles'
import { GeneratedOfficeArtifact } from './GeneratedOfficeArtifact'
import { ProtectedToolResultScope } from './presentation/ProtectedToolResult'

const { opener } = vi.hoisted(() => ({ opener: vi.fn<(...args: unknown[]) => Promise<boolean>>() }))
vi.mock('../../office/open-generated-artifact', () => ({ openGeneratedArtifact: opener }))
const timestamp = '2026-10-11T00:00:00Z'
function artifact(index: number, kind: GeneratedArtifactMetadata['kind'] = 'docx'): GeneratedArtifactMetadata {
  return { artifactId: index.toString(16).padStart(64, '0'), kind, contentHash: 'f'.repeat(64), byteSize: 2048, savedAt: timestamp }
}
function generated(index: number, kind: GeneratedArtifactMetadata['kind'] = 'docx'): ToolBlock {
  const item: CoreTurnItemJson = { id: 'item-' + index, threadId: 'thread', turnId: 'turn', kind: 'tool_result', role: 'tool',
    status: 'completed', toolName: 'generate_office_document', toolKind: 'file_change', callId: 'call-' + index,
    createdAt: timestamp, isError: false, output: { schemaVersion: 1, projectionKind: 'artifact_status', disclosure: 'metadata_only',
      status: 'completed', code: 'artifact_created', messageKey: 'artifact_created', privatePayloadWithheld: true,
      factAnswerAllowed: false, evidenceAuthority: false, artifact: artifact(index, kind) } }
  expect(ToolResultTurnItem.safeParse(item).success).toBe(true)
  const block = chatBlockFromItem(item)
  if (block?.kind !== 'tool') throw new Error('fixture must reach the real Office mapper')
  return block
}
let root: Root, container: HTMLDivElement, previous: ReturnType<typeof useChatStore.getState>
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('en')
  previous = useChatStore.getState()
  useChatStore.setState({ activeThreadId: 'thread', workspaceRoot: '/workspace' })
  opener.mockReset().mockResolvedValue(true)
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount())
  container.remove(); useChatStore.setState(previous)
  vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
const draw = async (blocks: ToolBlock[]) => {
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' },
    createElement(GeneratedFilesPanel, { blocks }))))
}
const cards = () => container.querySelectorAll('[data-generated-office-artifact]')
const buttons = () => [...container.querySelectorAll<HTMLButtonElement>('[data-generated-office-artifact] button')]
describe('real generated Office delivery tail', () => {
  it('deduplicates validated successful receipts before applying first-four disclosure', async () => {
    const valid = [1, 2, 3, 4, 5, 6].map(index => generated(index, index === 2 ? 'xlsx' : index === 3 ? 'pptx' : 'docx'))
    const rejected: ToolBlock = { ...generated(7), status: 'error' }
    const forged: ToolBlock = { ...generated(8), meta: { generatedArtifact: { ...artifact(8), path: '/PRIVATE/forged.docx' } } }
    await draw([...valid, valid[0], rejected, forged])
    expect(cards()).toHaveLength(4)
    expect(container.querySelector('button button')).toBeNull()
    expect(opener).not.toHaveBeenCalled()
    const toggle = container.querySelector<HTMLButtonElement>('button[aria-expanded]')!
    expect(toggle.getAttribute('aria-label')).toBe('Show all 6 generated files')
    await act(async () => toggle.click())
    expect(cards()).toHaveLength(6); expect(toggle.getAttribute('aria-expanded')).toBe('true')
    await act(async () => toggle.click())
    expect(cards()).toHaveLength(4); expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(container.textContent).not.toContain('/PRIVATE')
    expect(container.textContent).not.toContain(artifact(1).artifactId)
    expect(container.textContent).toContain('Generated workbook')
    expect(container.textContent).toContain('XLSX · 2 KB')
  })
  it('keeps a single receipt full-width and mounts no empty or invalid receipt group', async () => {
    await draw([generated(1)])
    expect(container.querySelector('[data-generated-office-files-row]')?.getAttribute('data-single')).toBe('true')
    expect(container.querySelector('button[aria-expanded]')).toBeNull()
    await draw([{ ...generated(1), status: 'error' }])
    expect(container.querySelector('[data-generated-office-deliverables]')).toBeNull()
  })
  it('uses the same guarded opener from separate card and action controls, then retries a real failure', async () => {
    let finish!: (value: boolean) => void
    opener.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await draw([generated(1)])
    const [preview, action] = buttons()
    expect(preview.getAttribute('aria-label')).toBe('Preview Generated document')
    expect(action.getAttribute('aria-label')).toBe('Open Generated document')
    await act(async () => { preview.click(); action.click() })
    expect(opener).toHaveBeenCalledExactlyOnceWith(artifact(1).artifactId, { threadId: 'thread', workspace: '/workspace' })
    expect(preview.disabled).toBe(true); expect(action.disabled).toBe(true)
    expect(container.querySelector('[data-generated-office-description]')?.getAttribute('role')).toBe('status')
    expect(container.textContent).toContain('Opening preview…')
    await act(async () => finish(false))
    expect(preview.disabled).toBe(false); expect(action.disabled).toBe(false)
    expect(container.textContent).toContain('Unable to open this document')
    expect(container.querySelector('[data-generated-office-description]')?.getAttribute('data-error')).toBe('true')
    await act(async () => action.click())
    expect(opener).toHaveBeenCalledTimes(2); expect(container.textContent).toContain('Preview opened')
  })
  it('reports success only after the actual opener returns true and clears its short-lived status', async () => {
    vi.useFakeTimers()
    await draw([generated(1)])
    expect(container.querySelector('[data-success]')).toBeNull()
    await act(async () => buttons()[0].click())
    expect(container.querySelector('[data-success]')).not.toBeNull()
    await act(async () => { vi.advanceTimersByTime(5000) })
    expect(container.textContent).toContain('Preview opened')
    await act(async () => { vi.advanceTimersByTime(200) })
    expect(container.querySelector('[data-success]')).toBeNull()
    expect(container.querySelector('[data-generated-office-description]')?.getAttribute('role')).toBeNull()
    expect(container.textContent).toContain('DOCX · 2 KB')
  })
  it('binds a displayed receipt to its existing thread owner instead of borrowing the active thread', async () => {
    await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'historical-owner' },
      createElement(GeneratedOfficeArtifact, { artifact: artifact(1) }))))
    await act(async () => buttons()[0].click())
    expect(opener).toHaveBeenCalledExactlyOnceWith(artifact(1).artifactId, { threadId: 'historical-owner', workspace: '/workspace' })
  })
  it('keeps the existing generated-media delivery when an Office receipt is also present', async () => {
    const media: ToolBlock = { ...generated(2), meta: { generatedFiles: [{ name: 'painting.png', mimeType: 'image/png',
      previewUrl: 'data:image/png;base64,paint', localFilePath: '/workspace/painting.png' }] } }
    await draw([generated(1), media])
    expect(cards()).toHaveLength(1)
    expect(container.querySelector('img')?.getAttribute('src')).toBe('data:image/png;base64,paint')
    expect(container.querySelector('img')?.getAttribute('alt')).toBe('painting.png')
    expect(container.querySelector('img')?.closest('figure')?.getAttribute('title')).toBe('painting.png')
  })
})
