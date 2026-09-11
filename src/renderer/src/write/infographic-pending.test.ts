import { describe, expect, it } from 'vitest'
import {
  beginPendingInfographic,
  buildPendingInfographicMarkdown,
  finishPendingInfographic,
  isPendingInfographicActive,
  lineEndAfter,
  parsePendingInfographicId,
  parsePendingInfographicImage,
  pendingInfographicKind,
  replacePendingInfographicInText
} from './infographic-pending'

describe('pending infographic tokens', () => {
  it('creates registered tokens and parses the id back from the src', () => {
    const pending = beginPendingInfographic()
    expect(isPendingInfographicActive(pending.id)).toBe(true)
    expect(parsePendingInfographicId(pending.src)).toBe(pending.id)
    finishPendingInfographic(pending.id)
    expect(isPendingInfographicActive(pending.id)).toBe(false)
  })

  it('tracks the image kind for active generations only', () => {
    const infographic = beginPendingInfographic()
    const design = beginPendingInfographic('design')
    const prototype = beginPendingInfographic('prototype')
    expect(pendingInfographicKind(infographic.id)).toBe('infographic')
    expect(pendingInfographicKind(design.id)).toBe('design')
    expect(pendingInfographicKind(prototype.id)).toBe('prototype')
    finishPendingInfographic(infographic.id)
    finishPendingInfographic(design.id)
    finishPendingInfographic(prototype.id)
    expect(pendingInfographicKind(design.id)).toBeNull()
  })

  it('finds the insertion point at the end of the selection line', () => {
    expect(lineEndAfter('第一行\n第二行', 1)).toBe(3)
    expect(lineEndAfter('单行无换行', 2)).toBe(5)
    expect(lineEndAfter('', 0)).toBe(0)
  })

  it('rejects non-pending sources', () => {
    expect(parsePendingInfographicId('img/photo.png')).toBeNull()
    expect(parsePendingInfographicId('https://example.com/a.png')).toBeNull()
    expect(parsePendingInfographicId('analytix-pending-infographic://')).toBeNull()
    expect(parsePendingInfographicId(undefined)).toBeNull()
  })

  it('parses pending image markdown and ignores regular images', () => {
    const parsed = parsePendingInfographicImage('![信息图](analytix-pending-infographic://abc-123)')
    expect(parsed).toEqual({
      alt: '信息图',
      id: 'abc-123',
      src: 'analytix-pending-infographic://abc-123'
    })
    expect(parsePendingInfographicImage('![信息图](img/a.png)')).toBeNull()
    expect(parsePendingInfographicImage('not markdown')).toBeNull()
  })

  it('strips square brackets from the alt text', () => {
    expect(buildPendingInfographicMarkdown('a[b]c', 'analytix-pending-infographic://x'))
      .toBe('![abc](analytix-pending-infographic://x)')
  })
})

describe('replacePendingInfographicInText', () => {
  const token = '![信息图](analytix-pending-infographic://abc)'

  it('swaps the placeholder for the generated image markdown', () => {
    const content = `段落一。\n\n${token}\n\n段落二。`
    expect(replacePendingInfographicInText(content, token, '![信息图](img/a.png)'))
      .toBe('段落一。\n\n![信息图](img/a.png)\n\n段落二。')
  })

  it('returns null when the placeholder was deleted by the user', () => {
    expect(replacePendingInfographicInText('段落一。\n\n段落二。', token, '![x](a.png)')).toBeNull()
  })

  it('removes the placeholder without leaving a blank gap', () => {
    const content = `段落一。\n\n${token}\n\n段落二。`
    expect(replacePendingInfographicInText(content, token, null)).toBe('段落一。\n\n段落二。')
  })

  it('keeps a single trailing newline when removing at the end of the file', () => {
    const content = `段落一。\n\n${token}\n`
    expect(replacePendingInfographicInText(content, token, null)).toBe('段落一。\n')
  })

  it('removes an inline placeholder without touching surrounding text', () => {
    const content = `前缀 ${token} 后缀`
    expect(replacePendingInfographicInText(content, token, null)).toBe('前缀  后缀')
  })
})
