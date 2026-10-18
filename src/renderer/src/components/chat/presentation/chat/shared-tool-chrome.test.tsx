// @vitest-environment jsdom
import { act, type ReactElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../../../i18n'
import { DisclosureRow } from '../DisclosureRow'
import { LinkIconMedium } from '../LinkIcon'
import { ToolRow, type ToolRowState } from './ToolRow'

// This suite mounts the actual chrome consumers without a Provider, network,
// clipboard or protected-display transport. CSS pixels and native inert focus
// behavior require the separate renderer/native acceptance seam.
let root: Root
let container: HTMLDivElement

async function draw(node: ReactElement) {
  await act(async () => root.render(node))
}

function header() {
  const row = container.querySelector<HTMLElement>('[data-disclosure-row]')
  if (!row) throw new Error('fixture must mount the actual disclosure header')
  return row
}

async function key(target: HTMLElement, init: KeyboardEventInit) {
  const event = new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init })
  await act(async () => { target.dispatchEvent(event) })
  return event
}

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('en')
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})

afterEach(async () => {
  await act(async () => root.unmount())
  container.remove()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('shared pinned DSH tool chrome', () => {
  it.each<ToolRowState>(['running', 'preparing'])('shares one inert whole-header shimmer while %s', async state => {
    await draw(<ToolRow title="Search" summary="Synthetic business request" summarySuffix="Synthetic suffix"
      diffStats={{ added: 3, removed: 1 }} state={state} />)

    const row = header()
    const envelopes = row.querySelectorAll<HTMLElement>('[data-shimmer]')
    expect(envelopes).toHaveLength(1)
    const envelope = envelopes[0]
    expect(envelope.firstElementChild?.textContent).toContain('Search')
    expect(envelope.firstElementChild?.textContent).toContain('Synthetic business request')
    expect(envelope.firstElementChild?.textContent).toContain('Synthetic suffix')
    expect(envelope.firstElementChild?.textContent).toContain('+3')
    expect(envelope.firstElementChild?.textContent).toContain('-1')
    expect(envelope.firstElementChild?.querySelector('[data-shimmer-decoration]')).not.toBeNull()

    const decorations = envelope.querySelectorAll<HTMLElement>('[aria-hidden="true"][inert]')
    expect(decorations).toHaveLength(1)
    const decoration = decorations[0]
    const generated = [...decoration.querySelectorAll<HTMLElement>('[data-shimmer-text]')]
      .map(node => node.getAttribute('data-shimmer-text')).join(' ')
    expect(generated).toContain('Search')
    expect(generated).toContain('Synthetic business request')
    expect(generated).toContain('Synthetic suffix')
    expect(decoration.textContent).toContain('+3')
    expect(decoration.textContent).toContain('-1')
    expect(decoration.querySelector('[data-shimmer-decoration]')).not.toBeNull()
    expect(decoration.querySelector('[id]')).toBeNull()
  })

  it('keeps action controls outside the decorative copy and removes activity when settled', async () => {
    const action = vi.fn(), toggle = vi.fn()
    const node = (state: ToolRowState) => <ToolRow title="Search" summary="Synthetic request" state={state}
      expandable onToggle={toggle} actions={<button type="button" onClick={action}>Synthetic action</button>} />
    await draw(node('running'))
    const button = [...container.querySelectorAll<HTMLButtonElement>('button')]
      .find(current => current.textContent === 'Synthetic action')
    expect(button).toBeDefined()
    expect(container.querySelectorAll('button')).toHaveLength(1)
    expect(button!.closest('[aria-hidden="true"][inert]')).toBeNull()
    await act(async () => button!.click())
    expect(action).toHaveBeenCalledOnce()
    expect(toggle).not.toHaveBeenCalled()

    await draw(node('ok'))
    expect(header().querySelector('[data-shimmer]')).toBeNull()
    expect(header().querySelector('[aria-hidden="true"][inert]')).toBeNull()
    expect(header().textContent).toContain('Search')
    expect(header().textContent).toContain('Synthetic request')
  })

  it.each<ToolRowState>(['error', 'stopped'])('retains business identity and selects the correct %s summary', async state => {
    await draw(<ToolRow title="Fetch" icon={<span data-business-icon>Business icon</span>} state={state}
      summary="Synthetic business request" errorSummary={'Synthetic failure first line\nSynthetic failure detail'} />)
    expect(container.querySelector('[data-state]')?.getAttribute('data-state')).toBe(state)
    expect(header().querySelector('[data-business-icon]')).not.toBeNull()
    expect(header().textContent).toContain('Fetch')
    expect(header().textContent).toContain(state === 'error' ? 'Synthetic failure first line' : 'Synthetic business request')
    expect(header().textContent).not.toContain(state === 'error' ? 'Synthetic business request' : 'Synthetic failure first line')
    expect(header().textContent).not.toContain('Synthetic failure detail')
    expect(header().querySelector('[data-shimmer]')).toBeNull()
  })

  it('renders the exact 12px Inspect glyph and calls only its owner action', async () => {
    const inspect = vi.fn(), toggle = vi.fn()
    await draw(<ToolRow title="Fetch" state="ok" expanded expandable onToggle={toggle}
      body={{ kind: 'web', content: <span>Synthetic body</span> }} inspect={{ label: 'Inspect', run: inspect }} />)
    const button = [...container.querySelectorAll<HTMLButtonElement>('button')]
      .find(current => current.textContent?.trim() === 'Inspect')
    expect(button).toBeDefined()
    const glyph = button!.querySelector('svg')
    expect(glyph?.getAttribute('width')).toBe('12')
    expect(glyph?.getAttribute('height')).toBe('12')
    expect(glyph?.getAttribute('viewBox')).toBe('0 0 16 16')
    expect(glyph?.getAttribute('fill')).toBe('none')
    expect(glyph?.getAttribute('stroke-width')).toBe('1')
    expect(glyph?.getAttribute('aria-hidden')).toBe('true')
    expect([...button!.querySelectorAll('path')].map(path => path.getAttribute('d'))).toEqual([
      'M4.67398 4.25061L1.36094 7.86484C1.29085 7.9413 1.29085 8.05866 1.36094 8.13513L4.67398 11.7494',
      'M11.3262 4.25061L14.6392 7.86484C14.7093 7.9413 14.7093 8.05866 14.6392 8.13513L11.3262 11.7494',
      'M9.56222 3.62573L6.43774 12.3743'
    ])
    expect(button!.closest('[aria-hidden="true"][inert]')).toBeNull()
    await act(async () => { button!.focus(); button!.click() })
    expect(inspect).toHaveBeenCalledOnce()
    expect(toggle).not.toHaveBeenCalled()
  })

  it('renders the pinned generic globe and opens a URL through its owner callback without href', async () => {
    const openLink = vi.fn(), toggle = vi.fn()
    await draw(<ToolRow title="Fetch" state="ok" expandable onToggle={toggle} onOpenLink={openLink}
      summary={<><LinkIconMedium kind="url" />https://synthetic.invalid/request</>} />)
    const link = container.querySelector<HTMLButtonElement>('[data-tool-web-link]')
    expect(link?.getAttribute('role')).toBe('link')
    expect(container.querySelector('[href]')).toBeNull()
    const glyph = link!.querySelector('svg')
    expect(glyph?.getAttribute('viewBox')).toBe('0 0 16 16')
    expect(glyph?.getAttribute('fill')).toBe('none')
    expect(glyph?.getAttribute('stroke-width')).toBe('1.3')
    expect(glyph?.getAttribute('aria-hidden')).toBe('true')
    expect([...link!.querySelectorAll('path')].map(path => path.getAttribute('d'))).toEqual([
      'M7.99986 14.0887C11.3626 14.0887 14.0886 11.3627 14.0886 7.99998C14.0886 4.63727 11.3626 1.91125 7.99986 1.91125C4.63715 1.91125 1.91113 4.63727 1.91113 7.99998C1.91113 11.3627 4.63715 14.0887 7.99986 14.0887Z',
      'M2.34619 8H13.6538',
      'M7.99976 14.0889C9.23509 14.0889 10.1743 11.3629 10.1743 8.00006C10.1743 4.63739 9.23509 1.91138 7.99976 1.91138',
      'M7.99973 14.0889C6.76445 14.0889 5.8252 11.3629 5.8252 8.00006C5.8252 4.63739 6.76445 1.91138 7.99973 1.91138'
    ])
    expect(link!.querySelectorAll('path')[1].getAttribute('stroke-linecap')).toBe('square')
    await act(async () => link!.click())
    expect(openLink).toHaveBeenCalledExactlyOnceWith()
    expect(toggle).not.toHaveBeenCalled()
    await key(link!, { key: 'Enter' })
    expect(toggle).not.toHaveBeenCalled()
  })

  it('keeps disclosure identity and only accepts nonrepeated Enter or Space outside IME from the row itself', async () => {
    const toggle = vi.fn()
    await draw(<DisclosureRow title="Search" icon={<span />} open={false} expandable expandOnRowClick
      bodyId="synthetic-disclosure-body" onToggle={toggle} actions={<button type="button">Child control</button>} />)
    const row = header()
    expect(row.getAttribute('role')).toBe('button')
    expect(row.getAttribute('tabindex')).toBe('0')
    expect(row.getAttribute('aria-expanded')).toBe('false')
    expect(row.getAttribute('aria-controls')).toBe('synthetic-disclosure-body')
    const chevron = row.querySelector('svg')
    expect(chevron?.getAttribute('viewBox')).toBe('0 0 16 16')
    expect(chevron?.getAttribute('stroke-width')).toBe('1')
    expect(chevron?.querySelector('path')?.getAttribute('d')).toBe('M4 6L7.29289 9.29289C7.68342 9.68342 8.31658 9.68342 8.70711 9.29289L12 6')
    for (const init of [
      { key: 'ArrowDown' }, { key: 'Enter', repeat: true }, { key: 'Enter', keyCode: 229 }, { key: 'Enter', isComposing: true }
    ]) expect((await key(row, init)).defaultPrevented).toBe(false)
    await key(row.querySelector<HTMLButtonElement>('button')!, { key: 'Enter' })
    expect(toggle).not.toHaveBeenCalled()
    expect((await key(row, { key: 'Enter' })).defaultPrevented).toBe(true)
    expect((await key(row, { key: ' ' })).defaultPrevented).toBe(true)
    expect(toggle).toHaveBeenCalledTimes(2)

    await draw(<DisclosureRow title="Search" icon={<span />} open expandable expandOnRowClick
      bodyId="synthetic-disclosure-body" onToggle={toggle} />)
    expect(header().getAttribute('aria-expanded')).toBe('true')
    expect(header().querySelector('svg path')?.getAttribute('d')).toBe('M12 10L8.70711 6.70711C8.31658 6.31658 7.68342 6.31658 7.29289 6.70711L4 10')
  })

  it('does not add a disclosure target to a nonexpandable row', async () => {
    const toggle = vi.fn()
    await draw(<DisclosureRow title="Fetch" icon={<span />} open={false} expandable={false} expandOnRowClick onToggle={toggle} />)
    const row = header()
    expect(row.hasAttribute('role')).toBe(false)
    expect(row.hasAttribute('tabindex')).toBe(false)
    expect(row.hasAttribute('aria-expanded')).toBe(false)
    expect(row.hasAttribute('aria-controls')).toBe(false)
    await act(async () => row.click())
    await key(row, { key: 'Enter' })
    expect(toggle).not.toHaveBeenCalled()
  })
})

// Append to the existing shared-tool-chrome.test.tsx. Uses its actual ToolRow,
// draw/header helpers, act/vi imports and mounted jsdom container; no new imports.
// These are DOM/event regressions, not native Tab, default keyboard activation,
// screen-reader, protected lease or external-navigation acceptance.

function runningPresentationCopy() {
  const envelopes = header().querySelectorAll<HTMLElement>('[data-shimmer]')
  expect(envelopes).toHaveLength(1)
  const envelope = envelopes[0]
  const base = envelope.firstElementChild as HTMLElement
  const copies = envelope.querySelectorAll<HTMLElement>('[aria-hidden="true"][inert]')
  expect(copies).toHaveLength(1)
  const decoration = copies[0]
  expect(decoration.getAttribute('aria-hidden')).toBe('true')
  expect(decoration.hasAttribute('inert')).toBe(true)
  expect(decoration.querySelector('button,a,[href],[tabindex],[role],[onclick],[ondblclick],[onkeydown],[onmousedown]')).toBeNull()
  expect(decoration.querySelector('[data-tool-file-link],[data-tool-web-link],[data-synthetic-action]')).toBeNull()
  for (const element of [decoration, ...decoration.querySelectorAll<HTMLElement>('*')]) {
    expect(element.onclick).toBeNull()
    expect(element.ondblclick).toBeNull()
    expect(element.onkeydown).toBeNull()
    expect(element.onmousedown).toBeNull()
  }
  return { base, decoration }
}

describe('running tool summaries keep one real entry', () => {
  it('keeps one file button and its original single-click and double-click owners', async () => {
    const openFile = vi.fn(), openEditor = vi.fn(), toggle = vi.fn()
    const path = '/synthetic/running-file.ts'
    await draw(<ToolRow title="Read" state="running" summary={`Reading ${path}`}
      filePath={path} filePathLine={7} onOpenFile={openFile} onOpenFileInEditor={openEditor}
      expandable onToggle={toggle} />)

    const { base, decoration } = runningPresentationCopy()
    expect(container.querySelectorAll('button,a')).toHaveLength(1)
    expect(container.querySelectorAll('[data-tool-file-link]')).toHaveLength(1)
    const button = base.querySelector<HTMLButtonElement>('[data-tool-file-link]')!
    expect(button.textContent).toBe(`${path}:7`)
    const paintedText = `${decoration.textContent} ${[...decoration.querySelectorAll('[data-shimmer-text]')]
      .map(node => node.getAttribute('data-shimmer-text')).join(' ')}`
    expect(paintedText).toContain(`${path}:7`)
    await act(async () => button.click())
    expect(openFile).toHaveBeenCalledExactlyOnceWith(path, 7)
    expect(openEditor).not.toHaveBeenCalled()
    expect(toggle).not.toHaveBeenCalled()

    // A direct dblclick checks the existing handler. jsdom does not synthesize
    // the two preceding physical click events for this dispatch.
    const doubleClick = new MouseEvent('dblclick', { bubbles: true, cancelable: true })
    await act(async () => { button.dispatchEvent(doubleClick) })
    expect(doubleClick.defaultPrevented).toBe(true)
    expect(openEditor).toHaveBeenCalledExactlyOnceWith(path, 7)
    expect(openFile).toHaveBeenCalledExactlyOnceWith(path, 7)
    expect(toggle).not.toHaveBeenCalled()
  })

  it('keeps one Host Web button and both copies of its synthetic private label inside the protected owner', async () => {
    const openLink = vi.fn(), toggle = vi.fn()
    const label = 'SYNTHETIC_LOCAL_ONLY_WEB_LABEL'
    const ownerId = 'synthetic-protected-running-owner'
    await draw(<div data-protected-process-block={ownerId}>
      <ToolRow title="Fetch" state="running" expandable onToggle={toggle} onOpenLink={openLink}
        summary={<span data-protected-process-block="synthetic-readonly-label">{label}</span>}
        actions={<span data-synthetic-action>Outside summary action</span>} />
    </div>)

    const owner = container.querySelector<HTMLElement>(`[data-protected-process-block="${ownerId}"]`)!
    const { base, decoration } = runningPresentationCopy()
    expect(container.querySelectorAll('button,a')).toHaveLength(1)
    expect(container.querySelectorAll('[data-tool-web-link]')).toHaveLength(1)
    expect(container.querySelector('[href]')).toBeNull()
    const button = base.querySelector<HTMLButtonElement>('[data-tool-web-link]')!
    expect(button.getAttribute('role')).toBe('link')
    expect(button.textContent).toBe(label)
    expect(decoration.textContent).toContain(label)
    expect(button.closest(`[data-protected-process-block="${ownerId}"]`)).toBe(owner)
    expect(decoration.closest(`[data-protected-process-block="${ownerId}"]`)).toBe(owner)
    // Static readonly protection markers are allowed in the highlight. The
    // boundary is that no literal or generated private label escapes its owner.
    const labelHolders = [...document.body.querySelectorAll<HTMLElement>('*')]
      .filter(node => node.textContent === label || node.getAttribute('data-shimmer-text') === label)
    expect(labelHolders.length).toBeGreaterThan(1)
    for (const node of labelHolders) expect(owner.contains(node)).toBe(true)
    const action = container.querySelector('[data-synthetic-action]')!
    expect(container.querySelectorAll('[data-synthetic-action]')).toHaveLength(1)
    expect(action.closest('[data-shimmer]')).toBeNull()
    await act(async () => button.click())
    expect(openLink).toHaveBeenCalledExactlyOnceWith()
    expect(toggle).not.toHaveBeenCalled()
  })

  it('keeps one href anchor with its real target and stops its click from toggling the row', async () => {
    const toggle = vi.fn(), observedClick = vi.fn()
    const href = 'https://synthetic.invalid/running-source'
    await draw(<ToolRow title="Fetch" state="running" summary="Synthetic href summary"
      href={href} expandable onToggle={toggle} />)

    const { base, decoration } = runningPresentationCopy()
    expect(container.querySelectorAll('button,a')).toHaveLength(1)
    expect(container.querySelectorAll('a[href]')).toHaveLength(1)
    const link = base.querySelector<HTMLAnchorElement>('a[href]')!
    expect(link.getAttribute('href')).toBe(href)
    expect(link.getAttribute('target')).toBe('_blank')
    expect(link.getAttribute('rel')).toBe('noopener noreferrer')
    expect(link.textContent).toBe('Synthetic href summary')
    const paintedText = `${decoration.textContent} ${[...decoration.querySelectorAll('[data-shimmer-text]')]
      .map(node => node.getAttribute('data-shimmer-text')).join(' ')}`
    expect(paintedText).toContain('Synthetic href summary')
    // The observer suppresses navigation only in this fixture; no external URL
    // is opened and this does not assert native browser navigation behavior.
    link.addEventListener('click', event => { event.preventDefault(); observedClick() })
    await act(async () => link.click())
    expect(observedClick).toHaveBeenCalledExactlyOnceWith()
    expect(toggle).not.toHaveBeenCalled()
  })
})
