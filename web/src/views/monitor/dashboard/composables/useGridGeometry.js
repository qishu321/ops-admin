// Geometry keeps the persisted 24-column coordinate system independent of input.
export function useGridGeometry(visualPanels) {
  const GRID_COLUMNS = 24, GRID_ROW_HEIGHT = 32, GRID_GAP = 9, GRID_MIN_WIDTH = 6, GRID_MIN_HEIGHT = 7
  function normalizeGridWidth(value) {
    return Math.max(GRID_MIN_WIDTH, Math.min(GRID_COLUMNS, (Number.isFinite(Number(value)) && Number(value) > 0 ? Math.round(Number(value)) : 12)))
  }

  function normalizeGridHeight(value, panel) {
    const fallback = panel?.chartType === 'table' ? 14 : panel?.chartType === 'stat' ? 7 : 9
    return Math.max(GRID_MIN_HEIGHT, Math.min(40, (Number.isFinite(Number(value)) && Number(value) > 0 ? Math.round(Number(value)) : fallback)))
  }

  function panelGridRect(panel) {
    return {
      x: Math.max(0, Math.min(GRID_COLUMNS - normalizeGridWidth(panel.gridW || panel.span), (Number.isFinite(Number(panel.gridX)) ? Math.round(Number(panel.gridX)) : 0))),
      y: Math.max(0, (Number.isFinite(Number(panel.gridY)) ? Math.min(1000, Math.round(Number(panel.gridY))) : 0)),
      w: normalizeGridWidth(panel.gridW || panel.span),
      h: normalizeGridHeight(panel.gridH, panel)
    }
  }

  function gridRectsOverlap(left, right) {
    return left.x < right.x + right.w && left.x + left.w > right.x && left.y < right.y + right.h && left.y + left.h > right.y
  }

  function initializePanelGrid(items = visualPanels.value) {
    const placed = []
    const changed = []
    ;[...items].sort((left, right) => Number(left.sort || 0) - Number(right.sort || 0)).forEach((panel) => {
      const hadGridPosition = Number(panel.gridW || 0) > 0 && Number(panel.gridH || 0) > 0
      const rect = panelGridRect(panel)
      if (!hadGridPosition || placed.some((item) => gridRectsOverlap(rect, item))) {
        rect.x = 0
        rect.y = 0
        let found = false
        for (let y = 0; y < 1000 && !found; y += 1) {
          for (let x = 0; x <= GRID_COLUMNS - rect.w; x += 1) {
            const candidate = { ...rect, x, y }
            if (!placed.some((item) => gridRectsOverlap(candidate, item))) {
              Object.assign(rect, candidate)
              found = true
              break
            }
          }
        }
      }
      if (panel.gridX !== rect.x || panel.gridY !== rect.y || panel.gridW !== rect.w || panel.gridH !== rect.h) changed.push(panel)
      Object.assign(panel, { gridX: rect.x, gridY: rect.y, gridW: rect.w, gridH: rect.h, span: rect.w })
      placed.push(rect)
    })
    return changed
  }

  function captureCurrentPanelGrid(grid) {
    if (!grid) return initializePanelGrid()
    const gridRect = grid.getBoundingClientRect()
    if (!gridRect.width) return initializePanelGrid()

    const panelsById = new Map(visualPanels.value.map((panel) => [String(panel.id), panel]))
    const captured = []
    grid.querySelectorAll(':scope > .chart-panel').forEach((element, index) => {
      const panel = panelsById.get(String(element.dataset.panelId || ''))
      if (!panel) return
      const rect = element.getBoundingClientRect()
      const width = normalizeGridWidth(Math.round((rect.width / gridRect.width) * GRID_COLUMNS))
      const x = Math.max(0, Math.min(GRID_COLUMNS - width, Math.round(((rect.left - gridRect.left) / gridRect.width) * GRID_COLUMNS)))
      const rowStep = GRID_ROW_HEIGHT + GRID_GAP
      const y = Math.max(0, Math.round((rect.top - gridRect.top) / rowStep))
      const height = normalizeGridHeight(Math.round((rect.height + GRID_GAP) / rowStep), panel)
      Object.assign(panel, {
        gridX: x,
        gridY: y,
        gridW: width,
        gridH: height,
        span: width,
        sort: index + 1
      })
      captured.push(panel)
    })

    if (captured.length !== visualPanels.value.length) return initializePanelGrid()
    return captured
  }


  return { normalizeGridWidth, normalizeGridHeight, panelGridRect, gridRectsOverlap, initializePanelGrid, captureCurrentPanelGrid }
}
