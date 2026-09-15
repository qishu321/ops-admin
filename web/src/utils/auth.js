const TOKEN_KEY = 'ops-admin-token'
const USER_KEY = 'ops-admin-user'
const MENU_KEY = 'ops-admin-menus'
const PERMISSION_KEY = 'ops-admin-permissions'
const SIDEBAR_COLLAPSED_KEY = 'ops-admin-sidebar-collapsed'
const TOKEN_EXPIRES_AT_KEY = 'ops-admin-token-expires-at'
const SESSION_EXPIRES_AT_KEY = 'ops-admin-session-expires-at'
const LAST_ACTIVITY_AT_KEY = 'ops-admin-last-activity-at'
const REMEMBER_LOGIN_KEY = 'ops-admin-remember-login'

const AUTH_KEYS = [
  TOKEN_KEY,
  USER_KEY,
  MENU_KEY,
  PERMISSION_KEY,
  TOKEN_EXPIRES_AT_KEY,
  SESSION_EXPIRES_AT_KEY,
  LAST_ACTIVITY_AT_KEY
]

function storageForCurrentSession() {
  return localStorage.getItem(REMEMBER_LOGIN_KEY) === '1' ? localStorage : sessionStorage
}

function readSessionItem(key) {
  const primary = storageForCurrentSession()
  return primary.getItem(key) ?? (primary === localStorage ? sessionStorage : localStorage).getItem(key)
}

function writeSessionItem(key, value) {
  storageForCurrentSession().setItem(key, value)
}

function removeSessionItem(key) {
  localStorage.removeItem(key)
  sessionStorage.removeItem(key)
}

function clearAuthData() {
  for (const key of AUTH_KEYS) removeSessionItem(key)
}

function notifySessionChanged() {
  if (typeof window !== 'undefined') window.dispatchEvent(new Event('ops-admin-session-changed'))
}

// Call this after login succeeds and before storing the returned identity.
// Ordinary logins are tab sessions; remembered logins survive browser restarts.
export function setAuthPersistence(rememberLogin) {
  clearAuthData()
  localStorage.removeItem(REMEMBER_LOGIN_KEY)
  if (rememberLogin) localStorage.setItem(REMEMBER_LOGIN_KEY, '1')
  notifySessionChanged()
}

export function isRememberLogin() {
  return localStorage.getItem(REMEMBER_LOGIN_KEY) === '1'
}

export function getToken() {
  if (isSessionExpired()) {
    logout()
    return ''
  }
  return readSessionItem(TOKEN_KEY) || ''
}

export function setToken(token, expiresAt = 0, sessionExpiresAt = 0) {
  writeSessionItem(TOKEN_KEY, token)
  if (expiresAt) writeSessionItem(TOKEN_EXPIRES_AT_KEY, String(expiresAt))
  if (sessionExpiresAt) writeSessionItem(SESSION_EXPIRES_AT_KEY, String(sessionExpiresAt))
  if (!getLastActivityAt()) setLastActivityAt(Date.now())
  notifySessionChanged()
}

export function clearToken() {
  removeSessionItem(TOKEN_KEY)
  removeSessionItem(TOKEN_EXPIRES_AT_KEY)
  removeSessionItem(SESSION_EXPIRES_AT_KEY)
}

export function getTokenExpiresAt() {
  const stored = Number(readSessionItem(TOKEN_EXPIRES_AT_KEY) || 0)
  if (stored) return stored
  const token = readSessionItem(TOKEN_KEY) || ''
  if (!token) return 0
  try {
    const payload = JSON.parse(atob(token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')))
    return Number(payload.exp || 0) * 1000
  } catch {
    return 0
  }
}

export function getSessionExpiresAt() {
  return Number(readSessionItem(SESSION_EXPIRES_AT_KEY) || 0)
}

export function isSessionExpired(now = Date.now()) {
  const expiresAt = getSessionExpiresAt()
  return Boolean(expiresAt && now >= expiresAt)
}

export function getLastActivityAt() {
  return Number(readSessionItem(LAST_ACTIVITY_AT_KEY) || 0)
}

export function setLastActivityAt(timestamp) {
  writeSessionItem(LAST_ACTIVITY_AT_KEY, String(timestamp))
}

export function setUser(user) {
  writeSessionItem(USER_KEY, JSON.stringify(user || {}))
}

export function getUser() {
  const raw = readSessionItem(USER_KEY)
  return raw ? JSON.parse(raw) : {}
}

export function clearUser() {
  removeSessionItem(USER_KEY)
}

export function setMenus(menus) {
  writeSessionItem(MENU_KEY, JSON.stringify(menus || []))
}

export function getMenus() {
  const raw = readSessionItem(MENU_KEY)
  return raw ? JSON.parse(raw) : []
}

export function clearMenus() {
  removeSessionItem(MENU_KEY)
}

export function setPermissions(permissions) {
  writeSessionItem(PERMISSION_KEY, JSON.stringify(permissions || []))
}

export function getPermissions() {
  const raw = readSessionItem(PERMISSION_KEY)
  return raw ? JSON.parse(raw) : []
}

export function clearPermissions() {
  removeSessionItem(PERMISSION_KEY)
}

export function setSidebarCollapsed(collapsed) {
  localStorage.setItem(SIDEBAR_COLLAPSED_KEY, collapsed ? '1' : '0')
}

export function getSidebarCollapsed() {
  return localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === '1'
}

export function logout() {
  clearAuthData()
  localStorage.removeItem(REMEMBER_LOGIN_KEY)
  notifySessionChanged()
}
