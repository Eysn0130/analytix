/** Frame identity alone survives a same-process reload. Every native request
 * and subscription retains an irreversible lease on the initiating document. */
export function captureNativeRendererDocument(
  contents: Electron.WebContents,
  current: () => boolean,
  onRevoke: () => void = () => undefined
) {
  let active = true
  const release = () => {
    active = false
    contents.removeListener('did-start-navigation', navigating)
    contents.removeListener('render-process-gone', revoke)
    contents.removeListener('destroyed', revoke)
  }
  const revoke = () => {
    if (!active) return
    release()
    onRevoke()
  }
  const navigating = (_event: Electron.Event, _url: string, _inPlace: boolean, mainFrame: boolean) => {
    if (mainFrame) revoke()
  }
  contents.on('did-start-navigation', navigating)
  contents.on('render-process-gone', revoke)
  contents.once('destroyed', revoke)
  return { isCurrent: () => {
    try { if (active && !current()) revoke() } catch { revoke() }
    return active
  }, release }
}
