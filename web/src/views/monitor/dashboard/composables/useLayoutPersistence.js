import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { saveMonitorDashboardPanel, saveMonitorDashboard } from '../../../../api/monitor'

const fields = ['gridX', 'gridY', 'gridW', 'gridH', 'span', 'sort']
export const snapshotLayout = panels => panels.map(panel => ({ ...panel }))
export function restoreLayout(panels, snapshot) {
  const byId = new Map(snapshot.map(panel => [panel.id, panel]))
  for (const panel of panels) {
    const old = byId.get(panel.id)
    if (old) for (const key of fields) panel[key] = old[key]
  }
}

// The API saves one panel at a time. Wait for ALL requests before compensating;
// otherwise a late successful write could overwrite the restored layout.
export function useLayoutPersistence() {
  const layoutSaving = ref(false)
  const layoutSaveState = ref('idle')
  const layoutMessage = ref('松开后自动保存')
  const layoutRecovery = ref(null)
  // Rapid layout edits (drag then nudge, or a mode switch right after a move)
  // arrive while a write is still running. Only the newest snapshot matters, so
  // pending calls are merged into one follow-up write that every caller shares.
  let merged = null

  async function savePanelLayout(panel) {
    const { id, dashboardId, datasourceId, title, promql, unit, chartType,
      span, gridX, gridY, gridW, gridH, inspectionKind, reducer, operator,
      warningValue, criticalValue, noDataState, category, sort, status, description } = panel
    return saveMonitorDashboardPanel({ id, dashboardId, datasourceId, title, promql, unit, chartType,
      span, gridX, gridY, gridW, gridH, inspectionKind, reducer, operator,
      warningValue, criticalValue, noDataState, category, sort, status, description })
  }
  async function writePanels(items) {
    const results = await Promise.allSettled(items.map(savePanelLayout))
    if (results.some(result => result.status === 'rejected')) throw new Error('layout write failed')
  }
  async function recover(operation) {
    await writePanels(operation.before)
    if (operation.nextMode !== undefined) await saveMonitorDashboard(operation.dashboardBefore)
  }
  async function persist({ panels, before, dashboard, nextMode }) {
    if (layoutRecovery.value) return false
    if (layoutSaving.value) {
      return new Promise((resolve) => {
        if (merged) {
          merged.params = { panels, before, dashboard, nextMode }
          merged.waiters.push(resolve)
        } else {
          merged = { params: { panels, before, dashboard, nextMode }, waiters: [resolve] }
        }
      })
    }
    return runPersist({ panels, before, dashboard, nextMode })
  }
  async function runPersist({ panels, before, dashboard, nextMode }) {
    const operation = { before, dashboardBefore: { ...dashboard }, nextMode }
    const after = snapshotLayout(panels)
    const changed = after.filter(panel => {
      const old = before.find(item => item.id === panel.id)
      return !old || fields.some(key => old[key] !== panel[key])
    })
    if (!changed.length && nextMode === undefined) return true
    layoutSaving.value = true
    layoutSaveState.value = 'saving'
    layoutMessage.value = '正在保存布局…'
    try {
      await writePanels(changed)
      if (nextMode !== undefined) await saveMonitorDashboard({ ...dashboard, layout: nextMode })
      if (nextMode !== undefined) dashboard.layout = nextMode
      layoutSaveState.value = 'saved'
      layoutMessage.value = '布局已保存'
      return true
    } catch {
      restoreLayout(panels, before)
      try {
        await recover(operation)
        layoutSaveState.value = 'error'
        layoutMessage.value = '保存失败，已恢复上次布局；可重新调整'
      } catch {
        layoutRecovery.value = operation
        layoutSaveState.value = 'recovery'
        layoutMessage.value = '保存失败，本地已回滚；服务端恢复未完成，请重试恢复'
      }
      ElMessage.error(layoutMessage.value)
      return false
    } finally {
      layoutSaving.value = false
      const next = merged
      merged = null
      if (next) {
        // The follow-up runs synchronously up to its first await, so the lock
        // never appears released in between.
        const ok = await runPersist(next.params)
        next.waiters.forEach(resolve => resolve(ok))
      }
    }
  }
  async function retryLayoutRecovery() {
    if (!layoutRecovery.value || layoutSaving.value) return
    layoutSaving.value = true
    try {
      await recover(layoutRecovery.value)
      layoutRecovery.value = null
      layoutSaveState.value = 'saved'
      layoutMessage.value = '上次布局已恢复，可以继续编辑'
    } catch { ElMessage.error('恢复失败，请检查连接后重试') }
    finally { layoutSaving.value = false }
  }
  return { layoutSaving, layoutSaveState, layoutMessage, layoutRecovery, savePanelLayout, persist, retryLayoutRecovery }
}
