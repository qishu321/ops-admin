import { ref, onBeforeUnmount } from 'vue'

export function useAutoRefresh({ isListLayout, activePanels, refreshAllPanels, isRefreshing }) {
  const autoRefreshSeconds = ref(0)

  let refreshTimer = null

  function restartAutoRefresh() {
    if (refreshTimer) window.clearInterval(refreshTimer)
    refreshTimer = null
    if (isListLayout.value) return
    if (!autoRefreshSeconds.value) return
    refreshTimer = window.setInterval(() => {
      if (activePanels.value.length && !isRefreshing.value) refreshAllPanels()
    }, autoRefreshSeconds.value * 1000)
  }

  onBeforeUnmount(() => { if (refreshTimer) window.clearInterval(refreshTimer) })

  return {
    autoRefreshSeconds,
    restartAutoRefresh,
  }
}
