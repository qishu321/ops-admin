import { ref, computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useGridGeometry } from './useGridGeometry'
import { useLayoutPersistence, snapshotLayout, restoreLayout } from './useLayoutPersistence'

export function useGridLayout({ isCustomGridLayout, isFullscreen, visualPanels, activeDashboard }) {
  const layoutEditing = ref(false)
  const layoutInteraction = ref(null)
  const layoutNeedsMigration = ref(false)
  const gridElement = ref(null)
  const geometry = useGridGeometry(visualPanels)
  const persistence = useLayoutPersistence()
  const { panelGridRect, gridRectsOverlap, initializePanelGrid, captureCurrentPanelGrid } = geometry
  const { layoutSaving, layoutRecovery, persist } = persistence
  const layoutLocked = computed(() => layoutSaving.value || !!layoutRecovery.value)
  let frame = 0, pendingEvent = null

  const rectStyle = rect => ({ gridColumn: (rect.x + 1) + ' / span ' + rect.w, gridRow: (rect.y + 1) + ' / span ' + rect.h })
  const layoutPreviewStyle = computed(() => layoutInteraction.value ? rectStyle(layoutInteraction.value.target) : {})
  const layoutPositionText = computed(() => {
    const rect = layoutInteraction.value?.target
    return rect ? '第 ' + (rect.x + 1) + ' 列 · 第 ' + (rect.y + 1) + ' 行 · ' + rect.w + ' × ' + rect.h : ''
  })
  function panelGridStyle(panel) {
    if (!isCustomGridLayout.value || isFullscreen.value) return {}
    const style = rectStyle(panelGridRect(panel))
    const drag = layoutInteraction.value
    if (drag?.panel.id !== panel.id) return style
    if (drag.type === 'move') style.transform = 'translate3d(' + drag.offsetX + 'px,' + drag.offsetY + 'px,0)'
    else {
      style.width = drag.target.w * drag.columnStep - 9 + 'px'
      style.height = drag.target.h * 41 - 9 + 'px'
    }
    return style
  }
  function clearInteraction() {
    const drag = layoutInteraction.value
    layoutInteraction.value = null
    if (frame) window.cancelAnimationFrame(frame)
    frame = 0
    pendingEvent = null
    if (drag?.handle?.hasPointerCapture?.(drag.pointerId)) drag.handle.releasePointerCapture(drag.pointerId)
  }
  function cancelPanelInteraction(event) {
    if (event?.pointerId !== undefined && event.pointerId !== layoutInteraction.value?.pointerId) return
    clearInteraction()
  }
  function exitLayoutEditing() { clearInteraction(); layoutEditing.value = false }
  async function enableLayoutEditing() {
    if (!activeDashboard.value || layoutLocked.value) return
    if (layoutEditing.value) return exitLayoutEditing()
    const before = snapshotLayout(visualPanels.value)
    if (!isCustomGridLayout.value) {
      captureCurrentPanelGrid(gridElement.value)
      const ok = await persist({ panels: visualPanels.value, before, dashboard: activeDashboard.value, nextMode: 'grid-custom' })
      if (!ok) return
    } else {
      const changed = initializePanelGrid()
      if (changed.length || layoutNeedsMigration.value) {
        // Loader may already have normalized the visible geometry.
        const original = migrationSnapshot || before
        if (!await persist({ panels: visualPanels.value, before: original, dashboard: activeDashboard.value })) return
      }
    }
    layoutNeedsMigration.value = false
    layoutEditing.value = true
  }
  let migrationSnapshot = null
  function preparePanelGrid() {
    migrationSnapshot = snapshotLayout(visualPanels.value)
    layoutNeedsMigration.value = initializePanelGrid().length > 0
  }
  function startPanelInteraction(event, panel, type) {
    if (!layoutEditing.value || layoutLocked.value || layoutInteraction.value || event.button !== 0) return
    const grid = event.currentTarget.closest('.panel-grid')
    if (!grid) return
    event.preventDefault()
    const rect = panelGridRect(panel)
    const bounds = grid.getBoundingClientRect()
    event.currentTarget.setPointerCapture?.(event.pointerId)
    layoutInteraction.value = {
      pointerId: event.pointerId, handle: event.currentTarget, grid, panel, type,
      startX: event.clientX, startY: event.clientY, startLeft: bounds.left, startTop: bounds.top,
      origin: rect, target: { ...rect }, before: snapshotLayout(visualPanels.value),
      columnStep: (bounds.width + 9) / 24, offsetX: 0, offsetY: 0,
    }
  }
  function applyPointer(event) {
    const drag = layoutInteraction.value
    if (!drag || drag.pointerId !== event.pointerId) return
    const bounds = drag.grid.getBoundingClientRect()
    const px = event.clientX - drag.startX + drag.startLeft - bounds.left
    const py = event.clientY - drag.startY + drag.startTop - bounds.top
    const dx = Math.round(px / drag.columnStep), dy = Math.round(py / 41)
    const { origin } = drag
    if (drag.type === 'move') {
      drag.target = { ...origin, x: Math.max(0, Math.min(24 - origin.w, origin.x + dx)), y: Math.max(0, Math.min(1000, origin.y + dy)) }
      drag.offsetX = Math.max(-origin.x * drag.columnStep, Math.min((24 - origin.w - origin.x) * drag.columnStep, px))
      drag.offsetY = Math.max(-origin.y * 41, Math.min((1000 - origin.y) * 41, py))
    } else {
      drag.target = { ...origin, w: Math.max(6, Math.min(24 - origin.x, origin.w + dx)), h: Math.max(7, Math.min(40, origin.h + dy)) }
    }
  }
  function updatePanelInteraction(event) {
    if (layoutInteraction.value?.pointerId !== event.pointerId) return
    pendingEvent = { pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY }
    if (!frame) frame = window.requestAnimationFrame(() => { frame = 0; if (pendingEvent) applyPointer(pendingEvent) })
  }
  function pushGridCollisions(activePanel) {
    const placed = [panelGridRect(activePanel)]
    const others = visualPanels.value.filter(panel => panel.id !== activePanel.id)
      .sort((a, b) => a.gridY - b.gridY || a.gridX - b.gridX || a.id - b.id)
    for (const panel of others) {
      const rect = panelGridRect(panel)
      let collisions = placed.filter(other => gridRectsOverlap(rect, other))
      while (collisions.length) {
        rect.y = Math.max(...collisions.map(other => other.y + other.h))
        collisions = placed.filter(other => gridRectsOverlap(rect, other))
      }
      panel.gridY = rect.y
      placed.push(rect)
    }
  }
  async function persistGridLayout(before) {
    const panels = visualPanels.value
    const ordered = [...panels].sort((a, b) => a.gridY - b.gridY || a.gridX - b.gridX)
    ordered.forEach((panel, index) => { panel.sort = index + 1 })
    return persist({ panels, before, dashboard: activeDashboard.value })
  }
  async function finishPanelInteraction(event) {
    const drag = layoutInteraction.value
    if (!drag || drag.pointerId !== event.pointerId) return
    applyPointer(event)
    const { target, origin, panel, before } = drag
    clearInteraction()
    if (['x', 'y', 'w', 'h'].every(key => target[key] === origin[key])) return
    Object.assign(panel, { gridX: target.x, gridY: target.y, gridW: target.w, gridH: target.h, span: target.w })
    pushGridCollisions(panel)
    await persistGridLayout(before)
  }
  async function keyboardInteraction(event, panel, resize) {
    if (event.key === 'Escape') { event.preventDefault(); cancelPanelInteraction(); return }
    const delta = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }[event.key]
    if (!delta || !layoutEditing.value) return
    event.preventDefault()
    if (layoutLocked.value || layoutInteraction.value) return
    const before = snapshotLayout(visualPanels.value), rect = panelGridRect(panel), step = event.shiftKey ? 4 : 1
    if (resize) {
      panel.gridW = Math.max(6, Math.min(24 - rect.x, rect.w + delta[0] * step))
      panel.gridH = Math.max(7, Math.min(40, rect.h + delta[1] * step))
      panel.span = panel.gridW
    } else {
      panel.gridX = Math.max(0, Math.min(24 - rect.w, rect.x + delta[0] * step))
      panel.gridY = Math.max(0, Math.min(1000, rect.y + delta[1] * step))
    }
    if (['x', 'y', 'w', 'h'].every(key => panelGridRect(panel)[key] === rect[key])) return
    pushGridCollisions(panel)
    await persistGridLayout(before)
  }
  async function restoreDefaultLayout() {
    if (!activeDashboard.value || layoutLocked.value) return
    clearInteraction()
    const panels = visualPanels.value
    const before = snapshotLayout(panels)
    if (await persist({ panels, before, dashboard: activeDashboard.value, nextMode: 'grid' })) exitLayoutEditing()
  }
  function handleEscape(event) { if (event.key === 'Escape') cancelPanelInteraction() }
  onMounted(() => {
    window.addEventListener('keydown', handleEscape)
    window.addEventListener('blur', cancelPanelInteraction)
    window.addEventListener('resize', cancelPanelInteraction)
  })
  watch(() => activeDashboard.value?.id, exitLayoutEditing, { flush: 'sync' })
  watch(isFullscreen, exitLayoutEditing)
  onBeforeUnmount(() => {
    exitLayoutEditing()
    window.removeEventListener('keydown', handleEscape)
    window.removeEventListener('blur', cancelPanelInteraction)
    window.removeEventListener('resize', cancelPanelInteraction)
  })
  return { ...geometry, ...persistence, gridElement, layoutEditing, layoutInteraction, layoutNeedsMigration,
    layoutLocked, layoutPreviewStyle, layoutPositionText, panelGridStyle, preparePanelGrid,
    enableLayoutEditing, exitLayoutEditing, startPanelInteraction, updatePanelInteraction,
    cancelPanelInteraction, finishPanelInteraction, pushGridCollisions, persistGridLayout, restoreDefaultLayout,
    movePanelWithKeyboard: (event, panel) => keyboardInteraction(event, panel, false),
    resizePanelWithKeyboard: (event, panel) => keyboardInteraction(event, panel, true),
  }
}
