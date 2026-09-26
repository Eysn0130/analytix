// Runs only in Main's isolated world. No guest preload, IPC, eval or page-supplied
// function is used. Retained DOM objects and offsets are checked again at read.
export function browserSelectionScript(action: 'capture' | 'check' | 'release', id: string): string {
  return `(${isolatedSelection.toString()})(${JSON.stringify(action)},${JSON.stringify(id)})`
}

function isolatedSelection(action: string, id: string): string | boolean | { error: 'child-frame-unsupported' } {
  type Held = { range: Range; start: Node; end: Node; startOffset: number; endOffset: number; text: string; dirty: boolean; observer: MutationObserver }
  const world = globalThis as typeof globalThis & { __analytixSelections?: Map<string, Held> }
  const held = world.__analytixSelections ??= new Map()
  const remove = (key: string) => { held.get(key)?.observer.disconnect(); held.delete(key) }
  if (action === 'release') { remove(id); return true }
  if (action === 'capture') {
    // A focused child frame may leave an old selection in the parent document.
    // Never substitute that stale parent text for the user's child-frame intent.
    if (document.activeElement?.matches('iframe, frame')) return { error: 'child-frame-unsupported' }
    const selection = window.getSelection()
    if (!selection || selection.rangeCount !== 1 || selection.isCollapsed) return false
    const range = selection.getRangeAt(0).cloneRange(), text = range.toString()
    if (!text.trim() || text.length > 4096 || /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(text) ||
      !range.startContainer.isConnected || !range.endContainer.isConnected ||
      range.startContainer.ownerDocument !== document || range.endContainer.ownerDocument !== document) return false
    while (held.size >= 8) remove(held.keys().next().value!)
    // Conservative invalidation includes an ABA change restored before reading.
    // It supplements the held nodes/text/offset checks; it is not the authority.
    const observer = new MutationObserver(() => { value.dirty = true })
    const value: Held = { range, start: range.startContainer, end: range.endContainer,
      startOffset: range.startOffset, endOffset: range.endOffset, text, dirty: false, observer }
    observer.observe(document, { subtree: true, childList: true, characterData: true, attributes: true })
    held.set(id, value)
    return text
  }
  const value = held.get(id)
  if (!value) return false
  if (value.observer.takeRecords().length) value.dirty = true
  const current = !value.dirty && value.start.isConnected && value.end.isConnected &&
    value.start.ownerDocument === document && value.end.ownerDocument === document &&
    value.range.startContainer === value.start && value.range.endContainer === value.end &&
    value.range.startOffset === value.startOffset && value.range.endOffset === value.endOffset &&
    value.range.toString() === value.text
  if (!current) remove(id)
  return current
}
