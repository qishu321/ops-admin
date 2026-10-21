export const ACCESS_CHANGED_EVENT = 'ops-admin-access-changed'

// Kept independent of Vue and HTTP so concurrent refreshes and session changes
// can be verified without a browser or a real privileged account.
export function createAccessSynchronizer({ getIdentity, loadProfile, applyProfile, onChanged }) {
  let pending = null
  let generation = 0
  let fingerprint = null
  let stopped = false

  function sync() {
    const identity = getIdentity()
    if (stopped || !identity) return Promise.resolve()
    if (pending?.identity === identity) return pending.promise
    const version = ++generation
    const promise = Promise.resolve().then(loadProfile).then(data => {
      if (stopped || version !== generation || getIdentity() !== identity) return
      const next = JSON.stringify([
        data?.user?.id, data?.user?.accountType, data?.accessRevision || '',
        [...(data?.permissionList || [])].sort(), data?.leftMenuList || []
      ])
      applyProfile(data)
      if (fingerprint !== next) {
        const initial = fingerprint === null
        fingerprint = next
        onChanged({ initial })
      }
      return data
    }).finally(() => {
      if (pending?.promise === promise) pending = null
    })
    pending = { identity, promise }
    return promise
  }

  return { sync, dispose() { stopped = true; generation++; pending = null } }
}
