// @vitest-environment jsdom
import { afterEach, expect, test } from 'vitest'
import { browserSelectionScript } from './selection-script'

const execute = (action: 'capture' | 'check' | 'release', id = 'selection') => window.eval(browserSelectionScript(action, id))
function select() {
  document.body.innerHTML = '<p>first 中文 selected text</p><p>untouched</p>'
  const node = document.querySelector('p')!.firstChild!, range = document.createRange()
  range.setStart(node, 6); range.setEnd(node, 11)
  window.getSelection()!.removeAllRanges(); window.getSelection()!.addRange(range)
  return node
}
afterEach(() => { execute('release'); document.body.innerHTML = '' })
test('retains actual Range nodes and offsets after selection focus changes', () => {
  select(); expect(execute('capture')).toBe('中文 se')
  window.getSelection()!.removeAllRanges()
  expect(execute('check')).toBe(true)
})
test.each(['text', 'aba', 'replace', 'detach'])('rejects %s changes including same-text replacement and ABA', mode => {
  const node = select(); expect(execute('capture')).toBe('中文 se')
  if (mode === 'text') node.textContent = 'different'
  if (mode === 'aba') { const old = node.textContent; node.textContent = 'different'; node.textContent = old }
  if (mode === 'replace') node.parentNode!.replaceChild(document.createTextNode(node.textContent!), node)
  if (mode === 'detach') node.parentNode!.removeChild(node)
  expect(execute('check')).toBe(false); expect(execute('check')).toBe(false)
})
test('rejects absent/collapsed/oversized selections and cannot resurrect released handles', () => {
  select(); execute('capture'); execute('release'); expect(execute('check')).toBe(false)
  window.getSelection()!.removeAllRanges(); expect(execute('capture')).toBe(false)
  document.body.textContent = 'x'.repeat(4097); const r = document.createRange(); r.selectNodeContents(document.body)
  window.getSelection()!.addRange(r); expect(execute('capture')).toBe(false)
})
