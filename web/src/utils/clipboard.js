// Clipboard API is unavailable on many HTTP intranet origins. Keep the legacy
// copy path inside the click handler's user gesture, and report its actual result.
export async function copyTextToClipboard(value) {
  const text = String(value ?? '')
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // Permissions or browser policy may block the modern API as well.
    }
  }
  if (typeof document === 'undefined' || !document.body) return false

  const activeElement = document.activeElement
  let textarea
  let selection
  const ranges = []
  try {
    selection = document.getSelection?.()
    for (let i = 0; i < (selection?.rangeCount || 0); i++) ranges.push(selection.getRangeAt(i).cloneRange())
    textarea = document.createElement('textarea')
    textarea.value = text
    textarea.readOnly = true
    textarea.setAttribute('aria-label', '待复制内容')
    Object.assign(textarea.style, { position: 'fixed', left: '0', top: '0', width: '1px', height: '1px', opacity: '0', pointerEvents: 'none' })
    document.body.appendChild(textarea)
    textarea.focus({ preventScroll: true })
    textarea.select()
    textarea.setSelectionRange(0, text.length)
    return typeof document.execCommand === 'function' && document.execCommand('copy') === true
  } catch {
    return false
  } finally {
    textarea?.remove()
    try {
      if (activeElement?.isConnected) activeElement.focus({ preventScroll: true })
      if (selection && ranges.length) {
        selection.removeAllRanges()
        ranges.forEach(range => selection.addRange(range))
      }
    } catch {
      // A concurrently removed element/range must not change the copy result.
    }
  }
}
