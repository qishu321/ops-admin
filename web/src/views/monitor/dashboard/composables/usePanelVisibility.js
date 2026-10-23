import { ref, onMounted, onBeforeUnmount } from 'vue'

// Mount charts close to the viewport, then retain them (table filters/tooltips
// are not thrown away by scrolling). Data queries retain the existing queue.
export function usePanelVisibility(element) {
  const visible = ref(false)
  let observer
  onMounted(() => {
    if (!window.IntersectionObserver) { visible.value = true; return }
    observer = new IntersectionObserver(entries => {
      if (entries.some(entry => entry.isIntersecting)) {
        visible.value = true
        observer.disconnect()
      }
    }, { rootMargin: '300px' })
    if (element.value) observer.observe(element.value)
  })
  onBeforeUnmount(() => observer?.disconnect())
  return visible
}
