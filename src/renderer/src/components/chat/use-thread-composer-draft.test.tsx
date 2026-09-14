// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { useThreadComposerDraft } from './use-thread-composer-draft'
import { useChatStore } from '../../store/chat-store'
import { composerDraftKey } from '../../store/composer-drafts'

let root: Root
let container: HTMLDivElement
let current: ReturnType<typeof useThreadComposerDraft>
function Composer({ workspace, thread }: { workspace: string; thread: string | null }) {
  current = useThreadComposerDraft(workspace, thread)
  return createElement('textarea', { value: current.draft.input, readOnly: true })
}
async function render(workspace = '/synthetic/a', thread: string | null = 'a') {
  await act(async () => root.render(createElement(Composer, { workspace, thread })))
}
beforeEach(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  useChatStore.setState({ composerDrafts: {} })
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount())
  container.remove()
  useChatStore.setState({ composerDrafts: {} })
})

describe('conversation composer drafts', () => {
  it('retains text and explicit references through settings-style unmount/remount', async () => {
    await render()
    await act(async () => {
      current.setInput('未发送草稿 😀')
      current.setFileReferences([{ path: '/synthetic/a/draft.md', name: 'draft.md', relativePath: 'draft.md' }])
    })
    await act(async () => root.render(createElement('div', null, 'settings')))
    await render()
    expect(current.draft.input).toBe('未发送草稿 😀')
    expect(current.draft.fileReferences).toEqual([{ path: '/synthetic/a/draft.md', name: 'draft.md', relativePath: 'draft.md' }])
  })

  it('isolates threads and workspace drafts before a thread exists', async () => {
    await render('/synthetic/a', null)
    await act(async () => current.setInput('new A'))
    await render('/synthetic/b', null)
    expect(current.draft.input).toBe('')
    await act(async () => current.setInput('new B'))
    await render('/synthetic/a', 'a')
    expect(current.draft.input).toBe('')
    await act(async () => current.setInput('thread A'))
    await render('/synthetic/a', 'b')
    expect(current.draft.input).toBe('')
    await render('/synthetic/a', 'a')
    expect(current.draft.input).toBe('thread A')
    await render('/synthetic/a', null)
    expect(current.draft.input).toBe('new A')
  })

  it('binds a late successful send to the original draft and preserves later identical input', async () => {
    await render()
    await act(async () => current.setInput('same'))
    const receipt = current.clearSubmitted
    await act(async () => current.setInput('changed'))
    await act(async () => current.setInput('same'))
    await render('/synthetic/a', 'b')
    await act(async () => current.setInput('same'))
    await act(async () => receipt({ includeInput: true, attachments: [], fileReferences: [] }))
    expect(current.draft.input).toBe('same')
    await render()
    expect(current.draft.input).toBe('same')
  })

  it('clears only submitted references after success, retaining additions while sending', async () => {
    await render()
    const first = { path: '/synthetic/a/first.md', name: 'first.md', relativePath: 'first.md' }
    const next = { path: '/synthetic/a/next.md', name: 'next.md', relativePath: 'next.md' }
    await act(async () => {
      current.setInput('send')
      current.setFileReferences([first])
    })
    const receipt = current.clearSubmitted
    await act(async () => current.setFileReferences((items) => [...items, next]))
    await render('/synthetic/a', 'b')
    await act(async () => receipt({ includeInput: true, attachments: [], fileReferences: [first] }))
    const original = useChatStore.getState().composerDrafts[composerDraftKey('/synthetic/a', 'a')]
    expect(original.input).toBe('')
    expect(original.fileReferences).toEqual([next])
  })
})
