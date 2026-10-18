// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, expect, it, vi } from 'vitest'
import { SearchBlock, type SearchBlockLabels } from './SearchBlock'
const labels: SearchBlockLabels = { pathsSummary: (shown, total) => `${shown}/${total ?? 'unknown'}`, matchesSummary: (shown, total, files) => `${shown}/${total ?? 'unknown'} in ${files}`,
  copy: 'Copy sealed', copied: 'Copied', noResults: 'None', collapseAria: 'Collapse', expandAria: hidden => `Expand ${hidden}`, collapse: 'Collapse', expand: hidden => `Expand ${hidden}` }
const container = document.createElement('div'), root = createRoot(container)
afterEach(async () => { await act(async () => root.render(null)); vi.unstubAllGlobals() })
it('holds eight rows including restored tail file header and copies independently of both disclosures', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const matches = (start: number) => Array.from({ length: 8 }, (_, i) => ({ lineNumber: start + i, line: `SOURCE_${start + i}`, column: 1, contextBefore: [], contextAfter: [] }))
  const search = { shape: 'matches' as const, files: [{ path: 'first.go', matches: matches(1) }, { path: 'tail.go', matches: matches(20) }], total: null, truncated: true,
    toolIsError: false, inputJSON: '{"pattern":"SOURCE"}', inputTruncated: false, startedAt: '2026-10-08T00:00:00Z' }
  const onCopy = vi.fn()
  await act(async () => root.render(createElement(SearchBlock, { search, labels, maxLines: 8, copied: false, onCopy })))
  expect(container.querySelectorAll('[data-search-row]')).toHaveLength(8)
  expect([...container.querySelectorAll('[data-search-row="file"]')].map(node => node.textContent)).toEqual(['first.go8', 'tail.go8'])
  await act(async () => (container.querySelector('[data-search-row="file"] button') as HTMLButtonElement).click())
  expect(container.textContent).toContain('16/unknown in 2')
  const copy = [...container.querySelectorAll('button')].find(button => button.textContent === 'Copy sealed')!
  await act(async () => copy.click()); expect(onCopy).toHaveBeenCalledTimes(1)
  const expand = container.querySelector('button[aria-label^="Expand"]') as HTMLButtonElement
  await act(async () => expand.click())
  expect(container.querySelectorAll('[data-search-row]')).toHaveLength(10)
})

it('keeps a one-row cap from accidentally appending the entire zero-length tail', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const search = { shape: 'paths' as const, paths: ['first', 'middle', 'last'], total: 3, truncated: false, toolIsError: false,
    inputJSON: '{"pattern":"*"}', inputTruncated: false, startedAt: '2026-10-08T00:00:00Z' }
  await act(async () => root.render(createElement(SearchBlock, { search, labels, maxLines: 1, copied: false, onCopy: vi.fn() })))
  expect(container.querySelectorAll('[data-search-row]')).toHaveLength(1)
  expect(container.querySelector('[data-search-row]')?.textContent).toBe('first')
})

it('reports sealed capture clipping independently of the producer total and retained rows', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const search = { shape: 'paths' as const, paths: ['sealed'], total: 2, truncated: false, toolIsError: false,
    inputJSON: '{"pattern":"*"}', inputTruncated: false, startedAt: '2026-10-08T00:00:00Z' }
  const pathsSummary = vi.fn((shown: number, total: number | null, truncated: boolean) => `${shown}/${total}:${truncated}`)
  await act(async () => root.render(createElement(SearchBlock, { search, labels: { ...labels, pathsSummary }, captureTruncated: true,
    className: 'owned-chat-root', copied: false, onCopy: vi.fn() })))
  expect(pathsSummary).toHaveBeenLastCalledWith(1, 2, true)
  expect(container.querySelector('[data-search]')?.classList.contains('owned-chat-root')).toBe(true)
  expect(container.querySelectorAll('[data-search-row]')).toHaveLength(1)
  expect(search.truncated).toBe(false)
})
