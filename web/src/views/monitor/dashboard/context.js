import { inject, provide } from 'vue'

// Each mounted dashboard owns its domains. Nothing mutable is shared across routes.
const domains = Object.fromEntries([
  'selection', 'time', 'queries', 'display', 'trends', 'bars', 'grid',
  'dashboardEditor', 'panelEditor', 'panelPreview', 'inspection', 'report', 'k8s',
  'loader', 'autoRefresh', 'fullscreen', 'filters', 'theme', 'view',
].map(name => [name, Symbol.for(`ops-admin-monitor:${name}`)]))

export function provideDashboardDomains(values) {
  for (const [name, value] of Object.entries(values)) provide(domains[name], value)
}

export function useDashboardDomain(name) {
  const value = inject(domains[name])
  if (!value) throw new Error('Dashboard domain is unavailable: ' + name)
  return value
}
