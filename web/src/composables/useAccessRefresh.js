import { onActivated, onBeforeUnmount, onDeactivated, onMounted } from 'vue'
import { ACCESS_CHANGED_EVENT } from '../utils/access-sync'

// Refresh data in place. Remounting the whole router view would destroy SSH/Pod
// sessions and unsaved forms whenever an administrator changes a policy.
export function useAccessRefresh(refresh) {
  let active = true, dirty = false, disposed = false, pending = false
  async function run() {
    dirty = true
    if (!active || disposed || pending) return
    pending = true
    try {
      do {
        dirty = false
        try { await refresh() } catch { /* The HTTP layer displays actionable errors. */ }
      } while (dirty && active && !disposed)
    } finally { pending = false }
  }
  onMounted(() => window.addEventListener(ACCESS_CHANGED_EVENT, run))
  onDeactivated(() => { active = false })
  onActivated(() => { active = true; if (dirty) run() })
  onBeforeUnmount(() => { disposed = true; window.removeEventListener(ACCESS_CHANGED_EVENT, run) })
}
