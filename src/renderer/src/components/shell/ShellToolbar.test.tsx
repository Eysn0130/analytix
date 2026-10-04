// @vitest-environment jsdom
import { readFileSync } from 'node:fs'
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ToolbarTooltip } from './ShellToolbar'

describe('ToolbarTooltip', () => {
  it('marks the wrapper as no-drag for frameless Windows hit testing', () => {
    const html = renderToStaticMarkup(
      createElement(ToolbarTooltip, { label: 'Back' }, createElement('button', { type: 'button' }, 'Back'))
    )

    expect(html).toContain('ds-toolbar-tooltip-anchor ds-no-drag')
  })
})


afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

function measuredTooltip(zoom: number, placement: 'center' | 'rail' = 'center') {
  let anchorLeft = 400, viewport = 1000, bubbleWidth = 180 * zoom
  const original = HTMLElement.prototype.getBoundingClientRect
  const rect = (left: number, width: number): DOMRect => ({x:left,y:10,left,right:left+width,top:10,bottom:42,width,height:32,toJSON:()=>({})})
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    if (this.classList.contains('ds-toolbar-tooltip-anchor')) return rect(anchorLeft, 28 * zoom)
    if (this.classList.contains('ds-toolbar-tooltip-bubble')) {
      const shift = Number.parseFloat(this.parentElement!.style.getPropertyValue('--ds-tooltip-shift')) || 0
      const baseLeft = placement === 'rail' ? anchorLeft + 40 * zoom : anchorLeft + 14 * zoom - bubbleWidth / 2
      return rect(baseLeft + shift * zoom, bubbleWidth)
    }
    return original.call(this)
  })
  vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(28)
  const viewportDescriptor = Object.getOwnPropertyDescriptor(window, 'innerWidth')!
  Object.defineProperty(window, 'innerWidth', {configurable:true,get:()=>viewport})
  return {
    move: (left: number) => { anchorLeft = left },
    resize: (width: number) => { viewport = width },
    grow: () => { bubbleWidth += 50 * zoom },
    restore: () => { Object.defineProperty(window, 'innerWidth', viewportDescriptor) }
  }
}

describe('ToolbarTooltip measured layout lifecycle', () => {
  it.each([0.82,0.88,1].flatMap(zoom => (['center','rail'] as const).map(placement => [zoom,placement] as const)))('reclamps the measured %s zoom bubble after a same-label %s layout move', async (zoom, placement) => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    if (placement === 'rail') expect(readFileSync('src/renderer/src/components/shell/navigation-rail.css','utf8')).toContain('left: calc(100% + 12px + var(--ds-tooltip-shift, 0px))')
    const geometry = measuredTooltip(zoom, placement), host = document.createElement('div'); document.body.append(host)
    const root = createRoot(host)
    const render = () => act(async () => root.render(createElement(ToolbarTooltip, {label:'Expand workspace (Ctrl/⌘+Shift+B)'},createElement('button',{type:'button'},'Expand'))))
    const bubble = () => host.querySelector<HTMLElement>('[role=tooltip]')!
    const within = (width: number) => { expect(bubble().getBoundingClientRect().left).toBeGreaterThanOrEqual(11.9); expect(bubble().getBoundingClientRect().right).toBeLessThanOrEqual(width-11.9) }
    try {
      await render(); within(1000)
      // The owner rerenders as a sibling panel closes; label and focus stay unchanged.
      geometry.move(960); await render(); within(1000)
      geometry.grow(); await render(); within(1000)
      geometry.resize(700); geometry.move(660)
      await act(async () => window.dispatchEvent(new Event('resize'))); within(700)
      geometry.resize(1000); geometry.move(400); await render(); within(1000)
      expect(Number.parseFloat(host.querySelector<HTMLElement>('.ds-toolbar-tooltip-anchor')!.style.getPropertyValue('--ds-tooltip-shift'))).toBeCloseTo(0)
    } finally { await act(async () => root.unmount()); host.remove(); geometry.restore() }
  })

  it('dismisses keyboard tooltip on Escape without consuming the original handler or losing focus; composing Escape stays open', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    const geometry=measuredTooltip(0.82), host=document.createElement('div');document.body.append(host);const root=createRoot(host), action=vi.fn(),keydown=vi.fn()
    const outside=document.createElement('button');document.body.append(outside)
    try {
      await act(async () => root.render(createElement(ToolbarTooltip,{label:'Expand'},createElement('button',{type:'button',onClick:action,onKeyDown:keydown},'Expand'))))
      const button=host.querySelector('button')!,anchor=host.querySelector<HTMLElement>('.ds-toolbar-tooltip-anchor')!
      await act(async()=>button.focus())
      await act(async()=>button.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',isComposing:true,bubbles:true,cancelable:true})))
      expect(anchor.dataset.tooltipHidden).toBeUndefined()
      const escape=new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true})
      await act(async()=>button.dispatchEvent(escape))
      expect(anchor.dataset.tooltipHidden).toBe('true');expect(document.activeElement).toBe(button);expect(escape.defaultPrevented).toBe(false);expect(action).not.toHaveBeenCalled();expect(keydown).toHaveBeenCalledTimes(2)
      await act(async()=>outside.focus());expect(anchor.dataset.tooltipHidden).toBeUndefined()
      await act(async()=>button.click());expect(action).toHaveBeenCalledTimes(1);expect(anchor.dataset.tooltipHidden).toBe('true')
      await act(async()=>anchor.dispatchEvent(new MouseEvent('pointerout',{bubbles:true,relatedTarget:outside})));expect(anchor.dataset.tooltipHidden).toBeUndefined()
    } finally { await act(async()=>root.unmount());host.remove();outside.remove();geometry.restore() }
  })

  it('dismisses a hovered tooltip on Escape while focus belongs to another control', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    const geometry=measuredTooltip(0.88), host=document.createElement('div');document.body.append(host);const root=createRoot(host),outside=document.createElement('button');document.body.append(outside)
    let hovering=true;const matches=HTMLElement.prototype.matches
    vi.spyOn(HTMLElement.prototype,'matches').mockImplementation(function(this:HTMLElement,selector:string){if(selector===':hover'&&this.classList.contains('ds-toolbar-tooltip-anchor'))return hovering;return matches.call(this,selector)})
    try {
      await act(async()=>root.render(createElement(ToolbarTooltip,{label:'Expand'},createElement('button',{type:'button'},'Expand'))));outside.focus()
      const anchor=host.querySelector<HTMLElement>('.ds-toolbar-tooltip-anchor')!
      await act(async()=>outside.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true})))
      expect(anchor.dataset.tooltipHidden).toBe('true');expect(document.activeElement).toBe(outside)
      hovering=false;await act(async()=>anchor.dispatchEvent(new MouseEvent('pointerout',{bubbles:true,relatedTarget:outside})))
      expect(anchor.dataset.tooltipHidden).toBeUndefined()
    } finally { await act(async()=>root.unmount());host.remove();outside.remove();geometry.restore() }
  })
})
