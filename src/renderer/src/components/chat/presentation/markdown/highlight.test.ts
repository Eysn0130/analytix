import { afterEach, expect, it, vi } from 'vitest'
afterEach(() => { vi.useRealTimers(); vi.resetModules(); vi.doUnmock('shiki/core'); vi.doUnmock('shiki/engine/javascript'); vi.doUnmock('@shikijs/langs/python') })
async function harness() {
  vi.resetModules(); vi.useFakeTimers()
  const instance = {
    getLoadedLanguages: vi.fn(() => ['typescript', 'shellscript', 'json']),
    loadLanguageSync: vi.fn(), codeToHtml: vi.fn(() => '<pre>safe</pre>'),
    codeToTokens: vi.fn(() => ({ tokens: [[{ content: 'safe', color: 'black' }]] })),
    codeToTokensBase: vi.fn((text: string) => text.split('\n').map(line => [{ content: line, color: 'black' }])),
    getLastGrammarState: vi.fn(() => ({}))
  }
  const create = vi.fn(() => instance)
  vi.doMock('shiki/core', () => ({ createHighlighterCoreSync: create, createCssVariablesTheme: () => ({}) }))
  vi.doMock('shiki/engine/javascript', () => ({ createJavaScriptRegexEngine: () => ({}), defaultJavaScriptRegexConstructor: () => ({}) }))
  const module = await import('./highlight')
  return { module, instance, create }
}
it('keeps all source paths plain after initialization failure, without retrying each render', async () => {
  const h = await harness(); h.create.mockImplementation(() => { throw new Error('Synthetic init failure') })
  expect(() => vi.runAllTimers()).not.toThrow()
  expect(h.module.highlightToHtml('safe', 'typescript')).toBeUndefined()
  expect(h.module.highlightLines('safe', 'typescript')).toBeUndefined()
  expect(new h.module.StreamingHighlightSession().update('safe', 'typescript')).toBeUndefined()
  expect(h.create).toHaveBeenCalledOnce()
})
it('keeps one failed lazy import plain without unhandled rejection or repeated requests', async () => {
  const h = await harness(), load = vi.fn(() => { throw new Error('Synthetic chunk unavailable') })
  vi.doMock('@shikijs/langs/python', load)
  const notified = vi.fn(); h.module.subscribeGrammarLoaded(notified)
  expect(h.module.highlightToHtml('safe', 'python')).toBeUndefined()
  await vi.dynamicImportSettled()
  expect(h.module.highlightToHtml('safe', 'python')).toBeUndefined()
  expect(load).toHaveBeenCalledOnce(); expect(notified).not.toHaveBeenCalled()
  expect(h.module.highlightToHtml('safe', 'typescript')).toBe('<pre>safe</pre>')
})
it('deduplicates lazy requests, isolates subscriber errors and permits successful grammar use', async () => {
  const h = await harness(), load = vi.fn(() => ({ default: [] }))
  vi.doMock('@shikijs/langs/python', load)
  const notified = vi.fn(); const unsubscribe = h.module.subscribeGrammarLoaded(notified)
  h.module.subscribeGrammarLoaded(() => { throw new Error('Synthetic subscriber error') })
  h.module.highlightToHtml('safe', 'python'); h.module.highlightToHtml('safe', 'python')
  await vi.dynamicImportSettled()
  h.instance.getLoadedLanguages.mockReturnValue(['typescript', 'python'])
  expect(load).toHaveBeenCalledOnce(); expect(notified).toHaveBeenCalledOnce(); unsubscribe()
  expect(h.module.highlightToHtml('safe', 'python')).toBe('<pre>safe</pre>')
})
it('keeps registration failure isolated to its lazy grammar', async () => {
  const h = await harness(); vi.doMock('@shikijs/langs/python', () => ({ default: [] }))
  h.instance.loadLanguageSync.mockImplementation(() => { throw new Error('Synthetic invalid grammar') })
  h.module.highlightToHtml('safe', 'python'); await vi.dynamicImportSettled()
  expect(h.module.highlightToHtml('safe', 'python')).toBeUndefined()
  expect(h.module.highlightToHtml('safe', 'typescript')).toBe('<pre>safe</pre>')
})
it('recovers static source rendering after one bad tokenization input', async () => {
  const h = await harness(); h.instance.codeToHtml.mockImplementationOnce(() => { throw new Error('Synthetic token failure') })
  expect(h.module.highlightToHtml('bad', 'typescript')).toBeUndefined()
  expect(h.module.highlightToHtml('safe', 'typescript')).toBe('<pre>safe</pre>')
})
it.each(['tail', 'state'])('rolls back a partially tokenized prefix after %s failure and rebuilds full source', async failure => {
  const h = await harness(); const session = new h.module.StreamingHighlightSession()
  const first = session.updateFrame('one\n', 'typescript')!
  if (failure === 'tail') h.instance.codeToTokensBase.mockImplementationOnce(text => text.split('\n').map(content => [{content,color:'black'}])).mockImplementationOnce(() => { throw new Error('Synthetic tail failure') })
  else h.instance.getLastGrammarState.mockImplementationOnce(() => { throw new Error('Synthetic state failure') })
  expect(session.updateFrame('one\ntwo\nthree', 'typescript')).toBeUndefined()
  const recovered = session.updateFrame('one\ntwo\nthree', 'typescript')!
  expect(recovered.generation).toBeGreaterThan(first.generation)
  expect([...recovered.appended, ...recovered.tail].map(line => line.map(span => span.text).join('')).join('\n')).toBe('one\ntwo\nthree')
})
