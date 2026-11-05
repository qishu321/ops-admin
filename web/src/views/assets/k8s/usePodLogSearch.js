import { computed, ref } from 'vue'

export function usePodLogSearch(readLogs) {
  const keyword = ref('')
  const searching = computed(() => Boolean(keyword.value.trim()))
  const matchingLines = computed(() => {
    const term = keyword.value.trim().toLowerCase()
    const lines = String(readLogs() || '').split('\n')
    if (lines.at(-1) === '') lines.pop()

    return lines
      .map((text, index) => ({ text, number: index + 1 }))
      .filter(({ text }) => !term || text.toLowerCase().includes(term))
  })

  return { keyword, searching, matchingLines }
}
