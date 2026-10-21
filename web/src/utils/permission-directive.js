export const PERMISSIONS_CHANGED_EVENT = 'ops-admin-permissions-changed'

export function createPermissionDirective(getPermissions, target = window) {
  const states = new WeakMap()
  function apply(el, binding) {
    let state = states.get(el)
    if (!state) {
      state = { display: el.style.display, binding }
      states.set(el, state)
    }
    state.binding = binding
    const required = Array.isArray(binding.value) ? binding.value : [binding.value]
    const permissions = getPermissions()
    // Hide instead of detaching: newly granted operations must be restorable
    // without remounting the view, and revoked operations must disappear too.
    el.style.display = required.some(key => permissions.includes(key)) ? state.display : 'none'
  }
  return {
    beforeMount: apply,
    mounted(el, binding) {
      apply(el, binding)
      const state = states.get(el)
      state.listener = () => apply(el, state.binding)
      target.addEventListener(PERMISSIONS_CHANGED_EVENT, state.listener)
    },
    updated: apply,
    unmounted(el) {
      const state = states.get(el)
      if (state?.listener) target.removeEventListener(PERMISSIONS_CHANGED_EVENT, state.listener)
      states.delete(el)
    }
  }
}
