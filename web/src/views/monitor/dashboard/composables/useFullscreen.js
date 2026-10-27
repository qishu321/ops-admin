import { ref, onMounted, onBeforeUnmount } from 'vue'

export function useFullscreen() {
  const isFullscreen = ref(false)
  let fallback = false
  let previousOverflow = ''
  let previousFocus = null

  function exitPresentation() {
    if (!fallback) return
    fallback = false
    isFullscreen.value = false
    document.body.style.overflow = previousOverflow
    previousFocus?.focus?.()
  }

  async function toggleFullscreen() {
    const el = document.querySelector('.dashboard-main')
    if (!el) return
    if (fallback) return exitPresentation()
    if (document.fullscreenElement && document.exitFullscreen) {
      await document.exitFullscreen()
      return
    }
    if (el.requestFullscreen) {
      try { await el.requestFullscreen(); return } catch { /* Embedded browsers can reject native fullscreen. */ }
    }
    previousFocus = document.activeElement
    previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    fallback = true
    isFullscreen.value = true
  }

  function onFullscreenChange() {
    isFullscreen.value = fallback || Boolean(document.fullscreenElement)
  }

  function onKeydown(event) {
    // Let an open dialog/drawer consume Escape before leaving presentation mode.
    if (event.key === 'Escape' && !event.defaultPrevented && !event.target?.closest?.('[role="dialog"]')) exitPresentation()
  }

  onMounted(() => {
    document.addEventListener('fullscreenchange', onFullscreenChange)
    document.addEventListener('keydown', onKeydown)
  })
  onBeforeUnmount(() => {
    exitPresentation()
    document.removeEventListener('fullscreenchange', onFullscreenChange)
    document.removeEventListener('keydown', onKeydown)
  })

  return {
    isFullscreen,
    toggleFullscreen,
    onFullscreenChange,
  }
}
