import { reactive } from 'vue'
import { hostAggregateTrendModes, lineColors } from '../config/templates.js'
import { numberValue, metricName, formatByUnit } from '../utils/format.js'

// A sample gap wider than this many typical intervals means the series stopped
// reporting; the line has to break instead of drawing a straight bridge across
// the missing window.
const TREND_GAP_FACTOR = 2.5
const TREND_MAX_SERIES = 8
const TREND_MAX_SERIES_K8S = 10

export function useTrendChart({ isHostDashboard, panelRows, isK8sDashboard, rangeEndText }) {
  const trendTooltips = reactive({})
  const hiddenSeries = reactive({})

  function seriesCap() {
    return isK8sDashboard.value ? TREND_MAX_SERIES_K8S : TREND_MAX_SERIES
  }

  function hiddenNames(panel) {
    return hiddenSeries[panel.id] || (hiddenSeries[panel.id] = {})
  }

  function isTrendSeriesHidden(panel, name) {
    return !!hiddenNames(panel)[name]
  }

  function toggleTrendSeries(panel, name) {
    const hidden = hiddenNames(panel)
    if (hidden[name]) delete hidden[name]
    else hidden[name] = true
    hideTrendTooltip(panel)
  }

  function hostTrendAggregateMode(panel) {
    return isHostDashboard.value ? hostAggregateTrendModes.get(panel?.title) : undefined
  }

  function aggregateHostTrend(panel, mode) {
    const samplesByTimestamp = new Map()
    panelRows(panel).forEach((row) => {
      ;(row.values || []).forEach((sample) => {
        const timestamp = Number(sample?.[0])
        const value = Number(sample?.[1])
        if (!Number.isFinite(timestamp) || !Number.isFinite(value)) return
        const values = samplesByTimestamp.get(timestamp) || []
        values.push(value)
        samplesByTimestamp.set(timestamp, values)
      })
    })
    return Array.from(samplesByTimestamp.entries())
      .sort(([left], [right]) => left - right)
      .map(([, values]) => mode === 'sum'
        ? values.reduce((sum, value) => sum + value, 0)
        : values.reduce((sum, value) => sum + value, 0) / values.length)
  }

  function panelTrend(panel) {
  	const aggregateMode = hostTrendAggregateMode(panel)
  	if (aggregateMode) {
  		const values = aggregateHostTrend(panel, aggregateMode)
  		if (values.length) return values
  	}
  	const firstSeries = panelRows(panel)[0]
  	if (firstSeries?.values?.length) return firstSeries.values.map((item) => Number(item?.[1])).filter(Number.isFinite)
  	return panelRows(panel).slice(0, 24).map((item) => numberValue(item))
  }

  function trendCurrentLabel(panel) {
    const aggregateMode = hostTrendAggregateMode(panel)
    if (aggregateMode === 'sum') return '当前总计'
    if (aggregateMode === 'average') return '当前均值'
    return '当前值'
  }

  function sparklinePoints(panel) {
    const values = panelTrend(panel)
    if (!values.length) return ''
    const max = Math.max(...values)
    const min = Math.min(...values)
    return trendPoints(values, min, max)
  }

  function trendPoints(values, min, max) {
    const range = max - min || 1
    return values.map((value, index) => {
      const x = values.length === 1 ? 100 : (index / (values.length - 1)) * 100
      const y = 48 - ((value - min) / range) * 40
      return `${x},${y}`
    }).join(' ')
  }

  /** Normalise Prometheus timestamps (seconds or milliseconds) to milliseconds. */
  function sampleMilliseconds(timestamp) {
    const numeric = Number(timestamp)
    if (!Number.isFinite(numeric)) return NaN
    return numeric > 100000000000 ? numeric : numeric * 1000
  }

  /**
   * Split a series into contiguous index ranges. Gaps are detected relative to
   * the median sampling interval, so a 60s step series with a 5 minute hole
   * breaks, while irregular-but-dense scrapes stay connected.
   */
  function trendSegments(samples) {
    const whole = [{ start: 0, end: Math.max(samples.length - 1, 0) }]
    if (samples.length < 3) return whole
    const steps = []
    for (let index = 1; index < samples.length; index++) {
      const delta = sampleMilliseconds(samples[index].timestamp) - sampleMilliseconds(samples[index - 1].timestamp)
      if (Number.isFinite(delta) && delta > 0) steps.push(delta)
    }
    if (!steps.length) return whole
    const sorted = [...steps].sort((left, right) => left - right)
    const limit = sorted[Math.floor(sorted.length / 2)] * TREND_GAP_FACTOR
    const segments = []
    let start = 0
    for (let index = 1; index < samples.length; index++) {
      const delta = sampleMilliseconds(samples[index].timestamp) - sampleMilliseconds(samples[index - 1].timestamp)
      if (delta > limit) {
        segments.push({ start, end: index - 1 })
        start = index
      }
    }
    segments.push({ start, end: samples.length - 1 })
    return segments
  }

  /** Candidate series in display order; colours stay stable when one is hidden. */
  function panelTrendSeries(panel) {
    return panelRows(panel)
      .map((row) => {
        const samples = (row.values || [])
          .map((item) => ({ timestamp: item?.[0], value: Number(item?.[1]) }))
          .filter((item) => Number.isFinite(item.value))
        return {
          name: metricName(row.metric) === 'metric' ? trendSeriesName(panel) : metricName(row.metric),
          samples,
          values: samples.map((item) => item.value),
          timestamps: samples.map((item) => item.timestamp)
        }
      })
      .filter((item) => item.values.length)
      .slice(0, seriesCap())
  }

  function panelLegendSeries(panel) {
    return panelTrendSeries(panel).map((item, index) => ({
      name: item.name,
      color: lineColors[index % lineColors.length],
      hidden: isTrendSeriesHidden(panel, item.name)
    }))
  }

  function panelLineSeries(panel) {
    const series = panelTrendSeries(panel)
      .map((item, index) => ({ ...item, color: lineColors[index % lineColors.length] }))
      .filter((item) => !isTrendSeriesHidden(panel, item.name))
    const allValues = series.flatMap((item) => item.values)
    if (!allValues.length) return []
    // Percentage trends must share an absolute 0-100 scale. Scaling each panel
    // to its local min/max makes stable request-utilization data look like a flat
    // line at the bottom of the chart instead of its real utilization level.
    const min = panel.unit === '%' ? 0 : Math.min(...allValues)
    const max = panel.unit === '%' ? 100 : Math.max(...allValues)
    const range = max - min || 1
    return series.map((item) => ({
      ...item,
      points: trendPoints(item.values, min, max),
      segmentPoints: trendSegments(item.samples).map(({ start, end }) => {
        const pairs = []
        const lastIndex = Math.max(item.values.length - 1, 1)
        for (let index = start; index <= end; index++) {
          const x = item.values.length === 1 ? 100 : (index / lastIndex) * 100
          pairs.push(`${x},${48 - ((item.values[index] - min) / range) * 40}`)
        }
        return pairs.join(' ')
      })
    }))
  }

  function trendSeriesName(panel) {
    if (panel?.title?.includes('CPU Request')) return 'CPU Request 使用率'
    if (panel?.title?.includes('内存 Request')) return '内存 Request 使用率'
    return panel?.title || 'metric'
  }

  function panelStats(panel) {
    const values = panelTrend(panel)
    if (!values.length) return { min: '-', max: '-', avg: '-' }
    const min = Math.min(...values)
    const max = Math.max(...values)
    const avg = values.reduce((sum, value) => sum + value, 0) / values.length
    return {
      min: formatByUnit(min, panel.unit),
      max: formatByUnit(max, panel.unit),
      avg: formatByUnit(avg, panel.unit)
    }
  }

  function trendSample(series, sampleIndex, sampleCount) {
    if (!series?.values?.length) return null
    const index = series.values.length === 1
      ? 0
      : Math.round((sampleIndex / Math.max(sampleCount - 1, 1)) * (series.values.length - 1))
    return {
      value: series.values[index],
      timestamp: series.timestamps?.[index]
    }
  }

  function trendTooltipTime(timestamp) {
    const numeric = Number(timestamp)
    if (!Number.isFinite(numeric)) return rangeEndText.value
    const milliseconds = numeric > 100000000000 ? numeric : numeric * 1000
    return new Date(milliseconds).toLocaleTimeString('zh-CN', { hour12: false })
  }

  function handleTrendPointer(panel, event) {
    const series = panelLineSeries(panel)
    const sampleCount = Math.max(...series.map((item) => item.values.length), 0)
    if (!sampleCount) return

    const container = event.currentTarget
    const bounds = container.getBoundingClientRect()
    // The SVG is inset by the chart padding, so the pointer has to be measured
    // against the plot area instead of the outer card, otherwise the crosshair
    // drifts away from the sampled point towards both edges.
    const plot = container.querySelector?.('svg.sparkline')
    const plotBounds = plot ? plot.getBoundingClientRect() : bounds
    const plotWidth = plotBounds.width || bounds.width || 1
    const ratio = Math.min(1, Math.max(0, (event.clientX - plotBounds.left) / plotWidth))
    const sampleIndex = Math.round(ratio * Math.max(sampleCount - 1, 0))
    const firstSample = trendSample(series[0], sampleIndex, sampleCount)
    const offset = plotBounds.left - bounds.left + ratio * plotWidth
    const half = Math.min(120, bounds.width / 2)

    trendTooltips[panel.id] = {
      x: `${offset}px`,
      left: `${Math.min(Math.max(offset, half), Math.max(bounds.width - half, half))}px`,
      ratio,
      time: trendTooltipTime(firstSample?.timestamp),
      items: series.map((item) => {
        const sample = trendSample(item, sampleIndex, sampleCount)
        return {
          name: item.name,
          color: item.color,
          value: Number.isFinite(sample?.value) ? formatByUnit(sample.value, panel.unit) : '-'
        }
      })
    }
  }

  function hideTrendTooltip(panel) {
    delete trendTooltips[panel.id]
  }

  return {
    trendTooltips,
    hiddenSeries,
    hostTrendAggregateMode,
    aggregateHostTrend,
    panelTrend,
    trendCurrentLabel,
    sparklinePoints,
    trendPoints,
    trendSegments,
    sampleMilliseconds,
    panelTrendSeries,
    panelLegendSeries,
    panelLineSeries,
    trendSeriesName,
    isTrendSeriesHidden,
    toggleTrendSeries,
    panelStats,
    trendSample,
    trendTooltipTime,
    handleTrendPointer,
    hideTrendTooltip,
  }
}
