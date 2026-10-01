// @vitest-environment jsdom
import html from './surface/office-surface.html?raw'
import source from './surface/office-surface.js?raw'
import { expect, test, vi } from 'vitest'

test('preview toolbar preserves keyboard focus, exposes fit, and disables terminal paging', async () => {
  document.body.innerHTML = new DOMParser().parseFromString(html, 'text/html').body.innerHTML
  vi.useFakeTimers()
  try {
    let receive!: (request: unknown) => Promise<void>
    vi.stubGlobal('analytixOfficeSurface', { connect: (handler: typeof receive) => { receive = handler }, send() {} })
    const Engine = new Function(source.replaceAll('export ', '') + '\nreturn OfficeEngineSurface;')()
    Engine.prototype.start = async function () {
      this.state = { kind: 'pptx', documentId: 'document', version: 'version' }
      this.openOperationId = 'opened'
    }
    const localView = vi.fn(async function (this: any) {
      this.view = { zoom: 39, fit: true, page: 3, pages: 3 }
      this.onView(this.view)
    })
    Engine.prototype.localView = localView
    await receive({ type: 'bind', channel: 'channel' })
    const button = document.querySelector<HTMLButtonElement>('[data-view="fit"]')!
    button.focus()
    button.click()
    await Promise.resolve()
    expect(localView).toHaveBeenCalledWith('fit')
    expect(document.activeElement).toBe(button)
    expect(button.getAttribute('aria-pressed')).toBe('true')
    expect(button.getAttribute('aria-label')).toBe('适合页面')
    expect(document.querySelector<HTMLButtonElement>('[data-view="next-page"]')!.disabled).toBe(true)
    expect(document.querySelector<HTMLButtonElement>('[data-view="previous-page"]')!.disabled).toBe(false)
    expect(document.getElementById('pageValue')!.textContent).toBe('3 / 3')
  } finally { vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals(); document.body.innerHTML = '' }
})
