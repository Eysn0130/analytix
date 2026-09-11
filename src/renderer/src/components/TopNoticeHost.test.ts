import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { TopNoticeCard, TopNoticeViewport } from './TopNoticeHost'

describe('TopNoticeHost', () => {
  it('uses a Codex-style fixed top viewport', () => {
    const html = renderToStaticMarkup(
      createElement(TopNoticeViewport, {
        children: createElement('span', null, 'toast')
      })
    )

    expect(html).toContain('pointer-events-none')
    expect(html).toContain('fixed inset-0 z-[60] mx-auto my-2')
    expect(html).toContain('max-w-[560px]')
    expect(html).toContain('items-center justify-start')
  })

  it('renders error notices as top alerts', () => {
    const html = renderToStaticMarkup(
      createElement(TopNoticeCard, {
        confirmLabel: '确定',
        onDismiss: () => undefined,
        notice: {
          id: 'notice-error',
          tone: 'error',
          message: '保存失败'
        }
      })
    )

    expect(html).toContain('data-testid="top-notice"')
    expect(html).toContain('role="alert"')
    expect(html).toContain('aria-live="assertive"')
    expect(html).toContain('保存失败')
    expect(html).toContain('确定')
    expect(html).toContain('data-tone="error"')
    expect(html).toContain('border-ds-border')
    expect(html).toContain('ring-ds-border')
    expect(html).toContain('bg-red-500')
  })

  it('renders success notices with the success tone', () => {
    const html = renderToStaticMarkup(
      createElement(TopNoticeCard, {
        confirmLabel: '确定',
        onDismiss: () => undefined,
        notice: {
          id: 'notice-success',
          tone: 'success',
          message: '已完成'
        }
      })
    )

    expect(html).toContain('role="status"')
    expect(html).toContain('aria-live="polite"')
    expect(html).toContain('已完成')
    expect(html).toContain('data-tone="success"')
    expect(html).toContain('border-ds-border')
    expect(html).toContain('ring-ds-border')
    expect(html).toContain('bg-emerald-500')
  })
})
