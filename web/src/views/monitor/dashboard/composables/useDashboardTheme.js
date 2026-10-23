import { ref, watch } from 'vue'

const STORAGE_KEY = 'ops-admin-monitor-theme'

export function useDashboardTheme() {
  const isDark = ref(false)
  try { isDark.value = localStorage.getItem(STORAGE_KEY) === 'dark' } catch { /* Storage may be disabled. */ }
  watch(isDark, value => {
    try { localStorage.setItem(STORAGE_KEY, value ? 'dark' : 'light') } catch { /* Keep the in-memory preference. */ }
  })
  const toggleTheme = () => { isDark.value = !isDark.value }
  return { isDark, toggleTheme }
}
