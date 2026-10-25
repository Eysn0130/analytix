// @vitest-environment jsdom
import { act, createElement, useCallback, useState, type RefObject } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { hasPublicFocus, useSearchableHidden } from './searchable-hidden'

let root: Root, container: HTMLDivElement, mounted: boolean
type ProbeProps = { hidden: boolean; reveal: () => void; existingRef?: RefObject<HTMLDivElement | null> }
function Probe({ hidden, reveal, existingRef }: ProbeProps) {
  const ref = useSearchableHidden(hidden, reveal, existingRef)
  return <div ref={ref} data-search-root><button data-public-control>Public control</button></div>
}
async function render(props: ProbeProps) {
  await act(async () => root.render(createElement(Probe, props)))
  return container.querySelector<HTMLDivElement>('[data-search-root]')!
}
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  container = document.createElement('div'); document.body.append(container); root = createRoot(container); mounted = true
})
afterEach(async () => {
  if (mounted) await act(async () => root.unmount())
  container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('public searchable hidden subtree', () => {
  it('retains the same DOM and external ref while applying and removing until-found', async () => {
    const existingRef = { current: null } as RefObject<HTMLDivElement | null>, reveal = vi.fn()
    const element = await render({ hidden: false, reveal, existingRef })
    const control = element.querySelector('button')!
    expect(existingRef.current).toBe(element)
    await render({ hidden: true, reveal, existingRef })
    expect(element.getAttribute('hidden')).toBe('until-found')
    expect(element.querySelector('button')).toBe(control)
    expect(reveal).not.toHaveBeenCalled()
    await render({ hidden: false, reveal, existingRef })
    expect(element.hasAttribute('hidden')).toBe(false)
    expect(existingRef.current).toBe(element)
  })

  it('reveals nested beforematch events idempotently through the public setter', async () => {
    function Owner() {
      const [hidden, setHidden] = useState(true)
      const reveal = useCallback(() => setHidden(false), [])
      return <Probe hidden={hidden} reveal={reveal} />
    }
    await act(async () => root.render(createElement(Owner)))
    const element = container.querySelector('[data-search-root]')!, control = element.querySelector('button')!
    expect(element.getAttribute('hidden')).toBe('until-found')
    await act(async () => {
      control.dispatchEvent(new Event('beforematch', { bubbles: true }))
      control.dispatchEvent(new Event('beforematch', { bubbles: true }))
    })
    expect(element.hasAttribute('hidden')).toBe(false)
    expect(container.querySelector('[data-search-root]')).toBe(element)
    expect(element.querySelector('button')).toBe(control)
  })

  it('protects public focus during a fold without first hiding the focused control', async () => {
    const reveal = vi.fn(), element = await render({ hidden: false, reveal })
    const control = element.querySelector<HTMLButtonElement>('button')!
    control.focus()
    expect(hasPublicFocus(element)).toBe(true)
    await render({ hidden: true, reveal })
    expect(reveal).toHaveBeenCalledOnce()
    expect(element.hasAttribute('hidden')).toBe(false)
    expect(document.activeElement).toBe(control)
  })

  it.each(['protected-process-block', 'protected-process-body', 'protected-assistant-draft', 'protected-tool-result',
    'protected-tool-body', 'protected-tool-summary', 'protected-user-input'])('does not preserve private focus under data-%s', async marker => {
    const reveal = vi.fn(), element = await render({ hidden: false, reveal })
    const privateRoot = document.createElement('div'), control = document.createElement('button')
    privateRoot.setAttribute('data-' + marker, ''); privateRoot.append(control); element.append(privateRoot); control.focus()
    expect(hasPublicFocus(element)).toBe(false)
    await render({ hidden: true, reveal })
    expect(element.getAttribute('hidden')).toBe('until-found')
    expect(reveal).not.toHaveBeenCalled()
  })

  it('preserves the admitted public Job Inspector focus while keeping its readonly body marker', async () => {
    const reveal = vi.fn(), element = await render({ hidden: false, reveal })
    const owner = document.createElement('div'), inspector = document.createElement('aside'), control = document.createElement('button')
    owner.setAttribute('data-public-job-inspector', ''); inspector.setAttribute('data-tool-result-inspector', '')
    inspector.setAttribute('data-protected-tool-body', ''); inspector.setAttribute('data-protected-tool-readonly', '')
    inspector.append(control); owner.append(inspector); element.append(owner); control.focus()
    expect(hasPublicFocus(element)).toBe(true)
    await render({ hidden: true, reveal })
    expect(reveal).toHaveBeenCalledOnce(); expect(element.hasAttribute('hidden')).toBe(false)
    expect(inspector.hasAttribute('data-protected-tool-readonly')).toBe(true); expect(document.activeElement).toBe(control)
  })

  it.each(['protected-process-block', 'protected-process-body', 'protected-assistant-draft', 'protected-tool-result',
    'protected-tool-body', 'protected-tool-summary', 'protected-user-input'])('does not let a public Inspector marker override data-%s ownership', async marker => {
    const element = await render({ hidden: false, reveal: vi.fn() }), privateOwner = document.createElement('div')
    const owner = document.createElement('div'), inspector = document.createElement('aside'), control = document.createElement('button')
    privateOwner.setAttribute('data-' + marker, ''); owner.setAttribute('data-public-job-inspector', '')
    inspector.setAttribute('data-tool-result-inspector', ''); inspector.setAttribute('data-protected-tool-body', '')
    inspector.setAttribute('data-protected-tool-readonly', '')
    inspector.append(control); owner.append(inspector); privateOwner.append(owner); element.append(privateOwner); control.focus()
    expect(hasPublicFocus(element)).toBe(false)
  })

  it('does not reclassify another protected body inside a public Inspector', async () => {
    const element = await render({ hidden: false, reveal: vi.fn() }), owner = document.createElement('div')
    const inspector = document.createElement('aside'), privateBody = document.createElement('div'), control = document.createElement('button')
    owner.setAttribute('data-public-job-inspector', ''); inspector.setAttribute('data-tool-result-inspector', '')
    inspector.setAttribute('data-protected-tool-body', ''); inspector.setAttribute('data-protected-tool-readonly', '')
    privateBody.setAttribute('data-protected-tool-body', '')
    privateBody.append(control); inspector.append(privateBody); owner.append(inspector); element.append(owner); control.focus()
    expect(hasPublicFocus(element)).toBe(false)
  })

  it('ignores focus outside the owner and replaces and cleans up the native listener', async () => {
    const first = vi.fn(), second = vi.fn(), element = await render({ hidden: true, reveal: first })
    const outside = document.createElement('button'); container.append(outside); outside.focus()
    expect(hasPublicFocus(element)).toBe(false); expect(hasPublicFocus(null)).toBe(false)
    await render({ hidden: true, reveal: second })
    await act(async () => element.dispatchEvent(new Event('beforematch')))
    expect(first).not.toHaveBeenCalled(); expect(second).toHaveBeenCalledOnce()
    await act(async () => root.unmount()); mounted = false
    element.dispatchEvent(new Event('beforematch'))
    expect(second).toHaveBeenCalledOnce(); outside.remove()
  })
})
