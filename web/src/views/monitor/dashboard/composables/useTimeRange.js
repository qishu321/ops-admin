import { ref, computed } from 'vue'

export function useTimeRange({ refreshAllPanels, isListLayout }) {
  const timeRangeSeconds = ref(3600)

  const timeRangeOption = ref(3600)

  const customTimeRange = ref([])

  const rangeStartText = computed(() => new Date(currentTimeBounds().startAt * 1000).toLocaleTimeString('zh-CN', { hour12: false }))

  const rangeEndText = computed(() => new Date(currentTimeBounds().endAt * 1000).toLocaleTimeString('zh-CN', { hour12: false }))

  function selectTimeRange(seconds) {
    timeRangeOption.value = seconds
    timeRangeSeconds.value = seconds
    refreshAllPanels()
  }

  function currentTimeBounds() {
    if (isListLayout.value && timeRangeOption.value === 'custom' && customTimeRange.value?.length === 2) {
      const startAt = Math.floor(new Date(customTimeRange.value[0]).getTime() / 1000)
      const endAt = Math.floor(new Date(customTimeRange.value[1]).getTime() / 1000)
      if (Number.isFinite(startAt) && Number.isFinite(endAt) && endAt > startAt) return { startAt, endAt }
    }
    const endAt = Math.floor(Date.now() / 1000)
    return { startAt: endAt - timeRangeSeconds.value, endAt }
  }

  function handleTimeRangeChange(value) {
    if (value === 'custom') {
      if (!customTimeRange.value?.length) {
        const endAt = new Date()
        customTimeRange.value = [new Date(endAt.getTime() - 3600 * 1000), endAt]
      }
      return
    }
    timeRangeSeconds.value = Number(value)
    if (!isListLayout.value) refreshAllPanels()
  }

  function handleCustomTimeRangeChange(value) {
    if (!value?.length) customTimeRange.value = []
  }

  function disableFutureDate(date) {
    return date.getTime() > Date.now()
  }

  function inspectionRangeLabel() {
    const labels = {
      3600: '最近 1 小时',
      21600: '最近 6 小时',
      86400: '最近 24 小时',
      259200: '最近三天',
      604800: '最近七天'
    }
    if (timeRangeOption.value !== 'custom') return labels[timeRangeOption.value] || '所选时间范围'
    const { startAt, endAt } = currentTimeBounds()
    const format = (timestamp) => new Date(timestamp * 1000).toLocaleString('zh-CN', { hour12: false })
    return `${format(startAt)} 至 ${format(endAt)}`
  }

  return {
    timeRangeSeconds,
    timeRangeOption,
    customTimeRange,
    rangeStartText,
    rangeEndText,
    selectTimeRange,
    currentTimeBounds,
    handleTimeRangeChange,
    handleCustomTimeRangeChange,
    disableFutureDate,
    inspectionRangeLabel,
  }
}
