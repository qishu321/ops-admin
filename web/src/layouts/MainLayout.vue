<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowDown, Check, Expand, Fold, Grid, House, Search } from '@element-plus/icons-vue'
import { logoutSession, profile } from '../api/system'
import {
  getMenus,
  getSidebarCollapsed,
  getUser,
  logout,
  setMenus,
  setPermissions,
  setSidebarCollapsed,
  setUser
} from '../utils/auth'
import { appDefinitions, getAppByRoute, setCurrentApp } from '../utils/apps'
import { locale, setLocale, t, translateRoute } from '../utils/i18n'
import { applySystemTheme, getSystemConfig, resolveSystemAsset, setSystemConfig } from '../utils/system-config'

const route = useRoute()
const router = useRouter()
const TAGS_KEY = 'ops-admin-tags-view'
const MAX_TAG_VIEWS = 10

const user = ref(getUser())
const rawMenus = ref(getMenus())
const collapsed = ref(getSidebarCollapsed())
const layoutConfig = ref(getSystemConfig())
const tagViews = ref(loadTags())
const appDrawerVisible = ref(false)
const appNavigationVisible = ref(false)
const commandPaletteVisible = ref(false)
const commandKeyword = ref('')
const tagContextMenu = ref({
  visible: false,
  x: 0,
  y: 0,
  tag: null
})

const currentLocale = computed(() => locale.value)
const activePath = computed(() => route.path)
const siteName = computed(() => layoutConfig.value.siteName || 'Ops Admin')
const sidebarTheme = computed(() => layoutConfig.value.sidebarTheme || 'dark')
const currentApp = computed(() => getAppByRoute(route.path))
const currentAppLabel = computed(() => t(currentApp.value.labelKey || 'appConsole'))
const appNavigationItems = computed(() => appDefinitions.map((app) => ({
  ...app,
  menuCount: countAppMenus(app),
  accent: appNavigationAccent[app.key] || '#5b6cf9',
  softAccent: appNavigationSoftAccent[app.key] || 'rgba(91, 108, 249, 0.12)'
})))
const currentTitle = computed(() => translateRoute(route.path, route.meta.title || findMenuTitle(sidebarMenus.value, route.path) || siteName.value))
const logoImage = computed(() => {
  if (layoutConfig.value.logoType === 'upload' || layoutConfig.value.logoType === 'url') {
    return resolveSystemAsset(layoutConfig.value.logoValue)
  }
  return ''
})

const sidebarMenus = computed(() => {
  if (currentApp.value.menuSource === 'backend') {
    return normalizeBackendMenus(rawMenus.value)
  }
  return normalizeStaticMenus(currentApp.value.menus || [])
})

const breadcrumbs = computed(() => buildBreadcrumbs(sidebarMenus.value, route.path, currentTitle.value))
const allCommandItems = computed(() => {
  const seenPaths = new Set()

  return appDefinitions.flatMap((app) => {
    const menus = app.menuSource === 'backend'
      ? normalizeBackendMenus(rawMenus.value)
      : normalizeStaticMenus(app.menus || [])
    const appLabel = t(app.labelKey || 'appConsole')

    return flattenMenus(menus).map((item) => ({
      ...item,
      appKey: app.key,
      appLabel
    }))
  }).filter((item) => {
    if (!item.path || seenPaths.has(item.path)) {
      return false
    }
    seenPaths.add(item.path)
    return true
  })
})

const commandItems = computed(() => {
  const keyword = commandKeyword.value.trim().toLowerCase()
  return allCommandItems.value
    .filter((item) => `${item.appLabel} ${item.parentTitle} ${item.title} ${item.path}`.toLowerCase().includes(keyword))
    .slice(0, 40)
})

const appNavigationAccent = {
  console: '#5b6cf9',
  assets: '#11a765',
  containers: '#0ea5e9',
  ops: '#f59e0b',
  applications: '#3b82f6',
  notify: '#ec4899',
  integration: '#14b8a6',
  monitor: '#6366f1',
  domains: '#8b5cf6'
}

const appNavigationSoftAccent = {
  console: 'rgba(91, 108, 249, 0.13)',
  assets: 'rgba(17, 167, 101, 0.13)',
  containers: 'rgba(14, 165, 233, 0.13)',
  ops: 'rgba(245, 158, 11, 0.14)',
  applications: 'rgba(59, 130, 246, 0.13)',
  notify: 'rgba(236, 72, 153, 0.13)',
  integration: 'rgba(20, 184, 166, 0.13)',
  monitor: 'rgba(99, 102, 241, 0.13)',
  domains: 'rgba(139, 92, 246, 0.13)'
}

function countAppMenus(app) {
  const countTree = (items = []) => items.reduce((total, item) => total + 1 + countTree(item.children), 0)
  return app.menuSource === 'backend' ? countTree(rawMenus.value) : countTree(app.menus)
}

function loadTags() {
  const defaults = [{ title: t('dashboard'), path: '/dashboard', affix: true }]
  const raw = localStorage.getItem(TAGS_KEY)
  if (!raw) {
    return defaults
  }
  try {
    const parsed = JSON.parse(raw)
    const source = Array.isArray(parsed) && parsed.length ? parsed : defaults
    return trimTagViews(source.map((item) => ({
      ...item,
      fullPath: item.fullPath || item.path,
      title: translateRoute(item.path, item.title),
      affix: isPinnedTag(item)
    })))
  } catch {
    return defaults
  }
}

function saveTags() {
  localStorage.setItem(
    TAGS_KEY,
    JSON.stringify(tagViews.value.map((item) => ({ ...item, affix: isPinnedTag(item) })))
  )
}

function isPinnedTag(tag) {
  return tag?.path === '/dashboard'
}

function trimTagViews(tags, protectedPath = '') {
  const result = [...tags]
  while (result.length > MAX_TAG_VIEWS) {
    let removeIndex = result.findIndex((item) => !isPinnedTag(item) && item.path !== protectedPath)
    if (removeIndex === -1) {
      removeIndex = result.findIndex((item) => !isPinnedTag(item))
    }
    if (removeIndex === -1) {
      break
    }
    result.splice(removeIndex, 1)
  }
  return result
}

function releaseTagResources(tag) {
  window.dispatchEvent(new CustomEvent('ops-admin:tab-closed', {
    detail: {
      path: tag.path,
      fullPath: tag.fullPath || tag.path
    }
  }))
}

function normalizeBackendMenus(raw) {
  const menuList = (raw || [])
    // 控制台只应使用属于控制台的后端菜单。后端菜单中也包含各应用的
    // 容器/资产等分支；只排除应用根路径会遗漏 /containers/k8s 这类子路径。
    .filter((item) => !isAppEntryMenu(item) && isConsoleMenuPath(item.url || item.path))
    .map((item) => ({
      title: item.menuName,
      path: item.url || '',
      icon: item.icon || 'Menu',
      children: (item.menuSvoList || [])
        .filter((child) => child.menuType !== 3 && !isAppEntryMenu(child) && isConsoleMenuPath(child.url || child.path))
        .map((child) => ({
          title: child.menuName,
          path: child.url || '',
          icon: child.icon || 'Document',
          children: []
        }))
    }))
    // 没有控制台路由、也没有保留下级菜单的应用分组不应显示为空菜单项。
    .filter((item) => item.path || item.children.length)

  if (!menuList.some((item) => item.path === '/dashboard')) {
    menuList.unshift({
      title: t('dashboard'),
      path: '/dashboard',
      icon: 'House',
      children: []
    })
  }

  return menuList
}

function isAppEntryMenu(item) {
  const name = item.menuName || item.title || ''
  const path = item.url || item.path || ''
  return name === '控制台' || name === 'Console' || path === '/console'
}

function normalizeStaticMenus(raw) {
  return (raw || []).map((item) => ({
    ...item,
    children: normalizeStaticMenus(item.children || [])
  }))
}

function findMenuTitle(menuList, path) {
  for (const item of menuList) {
    if (item.path === path) {
      return item.title
    }
    if (item.children?.length) {
      const match = item.children.find((child) => child.path === path)
      if (match) {
        return match.title
      }
    }
  }
  return ''
}

function displayTitle(item) {
  return item.titleKey ? t(item.titleKey) : translateRoute(item.path, item.title)
}

function flattenMenus(items = [], parentTitle = '') {
  return items.flatMap((item) => {
    const title = displayTitle(item)
    const current = item.path ? [{ title, parentTitle, path: item.path, icon: item.icon || 'Document' }] : []
    return current.concat(flattenMenus(item.children || [], title))
  })
}

function openCommandPalette() {
  commandKeyword.value = ''
  commandPaletteVisible.value = true
}

function selectCommand(item) {
  commandPaletteVisible.value = false
  router.push(item.path)
}

function onGlobalKeydown(event) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    openCommandPalette()
  }
}

function isConsoleMenuPath(path) {
  const normalizedPath = String(path || '').trim()
  return !normalizedPath || getAppByRoute(normalizedPath).key === 'console'
}

function buildBreadcrumbs(menuList, path, fallbackTitle) {
  if (path === '/dashboard') {
    return [
      { title: currentAppLabel.value, path: '/dashboard' },
      { title: t('dashboard'), path }
    ]
  }

  const trail = findMenuTrail(menuList, path)
  if (trail.length) {
    return [
      { title: currentAppLabel.value, path: currentApp.value.defaultRoute },
      ...trail.map((item) => ({ title: displayTitle(item), path: item.path || path }))
    ]
  }

  return [
    { title: currentAppLabel.value, path: currentApp.value.defaultRoute },
    { title: fallbackTitle, path }
  ]
}

function findMenuTrail(menuList, path, parents = []) {
  for (const item of menuList) {
    const trail = [...parents, item]
    if (item.path === path) {
      return trail
    }
    if (item.children?.length) {
      const childTrail = findMenuTrail(item.children, path, trail)
      if (childTrail.length) {
        return childTrail
      }
    }
  }
  return []
}

function currentLogoText() {
  if (layoutConfig.value.logoType === 'text' && layoutConfig.value.logoValue) {
    return String(layoutConfig.value.logoValue).slice(0, 2).toUpperCase()
  }
  return 'OA'
}

function ensureTag(path, title, fullPath = path) {
  if (!path || path === '/login') {
    return
  }
  const translated = translateRoute(path, title || '未命名页面')
  const found = tagViews.value.find((item) => item.path === path)
  if (found) {
    found.title = translated
    // 标签按路由路径去重，但必须保留最后一次打开时的查询参数。
    // 否则像服务资源拓扑这类依赖 serviceId 的页面在切换标签后会丢失上下文。
    found.fullPath = fullPath || path
    saveTags()
    return
  }
  tagViews.value.push({
    title: translated,
    path,
    fullPath: fullPath || path,
    affix: path === '/dashboard'
  })
  const nextTags = trimTagViews(tagViews.value, path)
  tagViews.value
    .filter((item) => !nextTags.some((nextItem) => nextItem.path === item.path))
    .forEach(releaseTagResources)
  tagViews.value = nextTags
  saveTags()
}

function handleTagClick(tag) {
  const target = tag?.fullPath || tag?.path
  if (target && target !== route.fullPath) {
    router.push(target)
  }
}

function closeTag(tag) {
  if (isPinnedTag(tag)) {
    return
  }
  const index = tagViews.value.findIndex((item) => item.path === tag.path)
  if (index === -1) {
    return
  }
  const wasActive = route.path === tag.path
  releaseTagResources(tag)
  tagViews.value.splice(index, 1)
  saveTags()
  if (wasActive) {
    const nextTag = tagViews.value[index - 1] || tagViews.value[index] || tagViews.value[0]
    router.push(nextTag?.fullPath || nextTag?.path || '/dashboard')
  }
}

// Some pages own long-lived resources (such as Pod terminal WebSockets). A
// page can explicitly request that its own tag be closed for a deliberate
// return action, while ordinary navigation continues to preserve the tag.
function closeRequestedTag(event) {
  const path = event.detail?.path
  const nextPath = event.detail?.nextPath || '/dashboard'
  const index = tagViews.value.findIndex((item) => item.path === path)
  if (index !== -1) {
    releaseTagResources(tagViews.value[index])
    tagViews.value.splice(index, 1)
    saveTags()
  }
  router.push(nextPath)
}

function openTagContextMenu(event, tag) {
  event.preventDefault()
  tagContextMenu.value = {
    visible: true,
    x: event.clientX,
    y: event.clientY,
    tag
  }
}

function closeTagContextMenu() {
  tagContextMenu.value.visible = false
}

function closeLeftTags() {
  const target = tagContextMenu.value.tag
  if (!target) {
    return
  }
  const targetIndex = tagViews.value.findIndex((item) => item.path === target.path)
  if (targetIndex <= 0) {
    closeTagContextMenu()
    return
  }
  const removedTags = tagViews.value.filter((item, index) => index < targetIndex && !isPinnedTag(item))
  const removedActive = removedTags.some((item) => item.path === route.path)
  removedTags.forEach(releaseTagResources)
  tagViews.value = tagViews.value.filter((item, index) => index >= targetIndex || isPinnedTag(item))
  saveTags()
  closeTagContextMenu()
  if (removedActive) {
    router.push(target.fullPath || target.path)
  }
}

function closeRightTags() {
  const target = tagContextMenu.value.tag
  if (!target) {
    return
  }
  const targetIndex = tagViews.value.findIndex((item) => item.path === target.path)
  if (targetIndex === -1) {
    closeTagContextMenu()
    return
  }
  const removedTags = tagViews.value.filter((item, index) => index > targetIndex && !isPinnedTag(item))
  const removedActive = removedTags.some((item) => item.path === route.path)
  removedTags.forEach(releaseTagResources)
  tagViews.value = tagViews.value.filter((item, index) => index <= targetIndex || isPinnedTag(item))
  saveTags()
  closeTagContextMenu()
  if (removedActive) {
    router.push(target.fullPath || target.path)
  }
}

function closeOtherTags() {
  const target = tagContextMenu.value.tag
  if (!target) {
    return
  }
  const keepPath = target.path
  tagViews.value
    .filter((item) => !isPinnedTag(item) && item.path !== keepPath)
    .forEach(releaseTagResources)
  tagViews.value = tagViews.value.filter((item) => isPinnedTag(item) || item.path === keepPath)
  saveTags()
  closeTagContextMenu()
  if (route.path !== keepPath && !tagViews.value.some((item) => item.path === route.path)) {
    router.push(target.fullPath || keepPath)
  }
}

function toggleCollapsed() {
  collapsed.value = !collapsed.value
  setSidebarCollapsed(collapsed.value)
}

function handleLogout() {
  logoutSession().catch(() => {})
  logout()
  router.push('/login')
}

function openProfile() {
  router.push('/profile')
}

function switchLocale(value) {
  setLocale(value)
  tagViews.value = tagViews.value.map((item) => ({
    ...item,
    title: translateRoute(item.path, item.title)
  }))
  saveTags()
}

function switchApp(key) {
  const targetApp = appDefinitions.find((item) => item.key === key) || appDefinitions[0]
  setCurrentApp(targetApp.key)
  appNavigationVisible.value = false
  if (route.path !== targetApp.defaultRoute) {
    router.push(targetApp.defaultRoute)
  }
}

async function syncProfile() {
  try {
    const data = await profile()
    if (data?.user) {
      setUser(data.user)
      user.value = data.user
    }
    if (data?.leftMenuList) {
      setMenus(data.leftMenuList)
      rawMenus.value = data.leftMenuList
    }
    if (data?.permissionList) {
      setPermissions(data.permissionList)
    }
    if (data?.systemConfig) {
      setSystemConfig(data.systemConfig)
      layoutConfig.value = data.systemConfig
      applySystemTheme(data.systemConfig)
    }
  } catch {
    rawMenus.value = getMenus()
    user.value = getUser()
    layoutConfig.value = getSystemConfig()
    applySystemTheme(layoutConfig.value)
  }
}

watch(
  () => route.fullPath,
  () => {
    closeTagContextMenu()
    setCurrentApp(currentApp.value.key)
    ensureTag(route.path, currentTitle.value, route.fullPath)
  },
  { immediate: true }
)

watch(
  () => locale.value,
  () => {
    tagViews.value = tagViews.value.map((item) => ({
      ...item,
      title: translateRoute(item.path, item.title)
    }))
    saveTags()
  }
)

onMounted(() => {
  applySystemTheme(layoutConfig.value)
  syncProfile()
  window.addEventListener('keydown', onGlobalKeydown)
  window.addEventListener('ops-admin:close-tab-request', closeRequestedTag)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onGlobalKeydown)
  window.removeEventListener('ops-admin:close-tab-request', closeRequestedTag)
})
</script>

<template>
  <el-container class="layout-shell" :class="[`theme-${sidebarTheme}`]">
    <el-aside :width="collapsed ? '78px' : '200px'" class="layout-aside" :class="{ 'is-collapsed': collapsed }">
      <div class="brand-box">
        <div class="brand-row">
          <img v-if="logoImage" :src="logoImage" alt="logo" class="brand-image" />
          <div v-else class="brand-mark">{{ currentLogoText() }}</div>
          <div v-if="!collapsed" class="brand-text">
            <strong>{{ siteName }}</strong>
            <span>{{ layoutConfig.siteSlogan || '个人运维管理平台' }}</span>
          </div>
          <el-popover v-model:visible="appNavigationVisible" placement="bottom-start" :width="820" trigger="click" popper-class="platform-nav-popper">
            <template #reference>
              <button type="button" class="platform-nav-trigger" :aria-label="t('appSwitch')" :title="t('appSwitch')">
                <el-icon><Grid /></el-icon>
                <span>平台导航</span>
                <el-icon class="platform-nav-arrow"><ArrowDown /></el-icon>
              </button>
            </template>
            <div class="platform-navigation">
              <div class="platform-navigation-head">
                <div>
                  <span>PLATFORM NAVIGATION</span>
                  <strong>应用平台导航</strong>
                </div>
                <small>选择一个应用，切换对应的工作台与菜单</small>
              </div>
              <div class="platform-navigation-grid">
                <button
                  v-for="app in appNavigationItems"
                  :key="app.key"
                  type="button"
                  class="platform-app-card"
                  :class="{ active: app.key === currentApp.key }"
                  :style="{ '--app-nav-accent': app.accent, '--app-nav-soft': app.softAccent }"
                  @click="switchApp(app.key)"
                >
                  <span class="platform-app-icon"><el-icon><component :is="app.icon || House" /></el-icon></span>
                  <span class="platform-app-copy">
                    <strong>{{ t(app.labelKey || 'appConsole') }}</strong>
                    <small>{{ app.menuCount }} 个菜单入口</small>
                  </span>
                  <el-icon v-if="app.key === currentApp.key" class="platform-app-check"><Check /></el-icon>
                </button>
              </div>
            </div>
          </el-popover>
        </div>
      </div>

      <el-scrollbar class="sidebar-scroll">
        <el-menu
          :default-active="activePath"
          :collapse="collapsed"
          :collapse-transition="false"
          class="sidebar-menu"
          router
        >
          <template v-for="menu in sidebarMenus" :key="menu.path || menu.title">
            <el-menu-item v-if="!menu.children?.length" :index="menu.path">
              <el-icon><component :is="menu.icon || House" /></el-icon>
              <template #title>{{ displayTitle(menu) }}</template>
            </el-menu-item>

            <el-sub-menu v-else :index="menu.path || menu.title">
              <template #title>
                <el-icon><component :is="menu.icon || 'Menu'" /></el-icon>
                <span>{{ displayTitle(menu) }}</span>
              </template>
              <template v-for="child in menu.children" :key="child.path || child.title">
                <el-menu-item v-if="!child.children?.length" :index="child.path">
                  <el-icon><component :is="child.icon || 'Document'" /></el-icon>
                  <template #title>{{ displayTitle(child) }}</template>
                </el-menu-item>
                <el-sub-menu v-else :index="child.path || child.title">
                  <template #title>
                    <el-icon><component :is="child.icon || 'Document'" /></el-icon>
                    <span>{{ displayTitle(child) }}</span>
                  </template>
                  <el-menu-item v-for="grandChild in child.children" :key="grandChild.path" :index="grandChild.path">
                    <el-icon><component :is="grandChild.icon || 'Document'" /></el-icon>
                    <template #title>{{ displayTitle(grandChild) }}</template>
                  </el-menu-item>
                </el-sub-menu>
              </template>
            </el-sub-menu>
          </template>
        </el-menu>
      </el-scrollbar>
    </el-aside>

    <el-container class="main-shell">
      <el-header class="layout-header">
        <div class="header-top">
          <div class="header-left">
            <el-button class="collapse-button" circle @click="toggleCollapsed">
              <el-icon><component :is="collapsed ? Expand : Fold" /></el-icon>
            </el-button>

            <el-breadcrumb separator="/">
              <el-breadcrumb-item v-for="item in breadcrumbs" :key="item.path">
                {{ item.title }}
              </el-breadcrumb-item>
            </el-breadcrumb>
          </div>

          <div class="header-right">
            <el-button class="command-trigger" @click="openCommandPalette">
              <el-icon><Search /></el-icon>
              <span>全局搜索</span>
              <kbd>⌘K</kbd>
            </el-button>
            <el-dropdown trigger="click" @command="switchLocale">
              <div class="locale-box">
                <span>{{ currentLocale === 'zh-CN' ? 'CN' : 'EN' }}</span>
                <el-icon><ArrowDown /></el-icon>
              </div>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="zh-CN">
                    <div class="locale-item">
                      <span>CN</span>
                      <el-icon v-if="currentLocale === 'zh-CN'"><Check /></el-icon>
                    </div>
                  </el-dropdown-item>
                  <el-dropdown-item command="en-US">
                    <div class="locale-item">
                      <span>EN</span>
                      <el-icon v-if="currentLocale === 'en-US'"><Check /></el-icon>
                    </div>
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>

            <el-dropdown>
              <div class="user-box">
                <div class="user-avatar">{{ (user.nickname || user.username || 'A').slice(0, 1).toUpperCase() }}</div>
                <span>{{ user.nickname || user.username || 'admin' }}</span>
                <el-icon><ArrowDown /></el-icon>
              </div>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item @click="openProfile">{{ t('profile') }}</el-dropdown-item>
                  <el-dropdown-item divided @click="handleLogout">{{ t('logout') }}</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>

        <div class="tabs-bar">
          <div
            v-for="tag in tagViews"
            :key="tag.path"
            class="tab-chip"
            :class="{ active: tag.path === route.path }"
            @click="handleTagClick(tag)"
            @contextmenu="openTagContextMenu($event, tag)"
          >
            <span>{{ tag.title }}</span>
            <button v-if="!isPinnedTag(tag)" type="button" class="tab-close" @click.stop="closeTag(tag)">×</button>
          </div>
        </div>
      </el-header>

      <el-main class="layout-main">
        <router-view v-slot="{ Component, route: currentRoute }">
          <keep-alive>
            <component :is="Component" v-if="currentRoute.meta.keepAlive" :key="currentRoute.fullPath" />
          </keep-alive>
          <component :is="Component" v-if="!currentRoute.meta.keepAlive" />
        </router-view>
      </el-main>
    </el-container>

    <el-drawer v-if="false" v-model="appDrawerVisible" :size="320" direction="ltr" class="app-drawer" :with-header="false">
      <div class="app-drawer-header">
        <span>{{ t('appSwitch') }}</span>
        <strong>{{ currentAppLabel }}</strong>
      </div>

      <div class="app-drawer-list">
        <button
          v-for="app in appDefinitions"
          :key="app.key"
          type="button"
          class="app-drawer-item"
          :class="{ active: app.key === currentApp.key }"
          @click="switchApp(app.key)"
        >
          <span class="app-drawer-icon">
            <el-icon><component :is="app.icon || House" /></el-icon>
          </span>
          <span class="app-drawer-text">
            <strong>{{ t(app.labelKey || 'appConsole') }}</strong>
            <small>{{ app.key === currentApp.key ? '当前应用' : '切换进入' }}</small>
          </span>
          <el-icon v-if="app.key === currentApp.key" class="app-drawer-check"><Check /></el-icon>
        </button>
      </div>
    </el-drawer>

    <div v-if="tagContextMenu.visible" class="tag-menu-mask" @click="closeTagContextMenu">
      <div
        class="tag-context-menu"
        :style="{ left: `${tagContextMenu.x}px`, top: `${tagContextMenu.y}px` }"
        @click.stop
      >
        <button type="button" @click="closeLeftTags">关闭左侧标签页</button>
        <button type="button" @click="closeRightTags">关闭右侧标签页</button>
        <button type="button" @click="closeOtherTags">关闭其他标签页</button>
      </div>
    </div>

    <el-dialog v-model="commandPaletteVisible" class="command-palette" width="640px" :show-close="false" :append-to-body="true">
      <template #header>
        <div class="command-palette-head">
          <div>
            <span>GLOBAL COMMAND</span>
            <strong>快速跳转与检索</strong>
          </div>
          <kbd>ESC</kbd>
        </div>
      </template>
      <el-input v-model="commandKeyword" autofocus placeholder="搜索全部应用的页面、模块或路径" clearable>
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <div class="command-result-list">
        <button v-for="item in commandItems" :key="`${item.appKey}:${item.path}`" type="button" class="command-result" @click="selectCommand(item)">
          <el-icon><component :is="item.icon" /></el-icon>
          <span><b>{{ item.title }}</b><small>{{ [item.appLabel, item.parentTitle].filter(Boolean).join(' / ') }} · {{ item.path }}</small></span>
          <em>Enter</em>
        </button>
        <el-empty v-if="!commandItems.length" description="未找到匹配的页面或操作" :image-size="48" />
      </div>
    </el-dialog>
  </el-container>
</template>

<style scoped>
.layout-shell {
  min-height: 100vh;
  background: #eef1f8;
}

.layout-aside {
  background: linear-gradient(180deg, #20295b 0%, #1f2552 100%);
  color: #d8defb;
  border-right: 1px solid rgba(255, 255, 255, 0.08);
  transition: width 0.2s ease;
}

.theme-light .layout-aside {
  background: linear-gradient(180deg, #30406b 0%, #2d3a61 100%);
}

.brand-box {
  padding: 16px 9px 12px;
  min-height: 68px;
  overflow: hidden;
}

.brand-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.layout-aside.is-collapsed .brand-box {
  min-height: 104px;
}
.layout-aside.is-collapsed .brand-row {
  flex-wrap: wrap;
  justify-content: center;
  gap: 9px;
}
.layout-aside.is-collapsed .platform-nav-trigger {
  margin-left: 0;
}

.brand-mark,
.brand-image {
  width: 38px;
  height: 38px;
  min-width: 38px;
  max-width: 38px;
  min-height: 38px;
  max-height: 38px;
  border-radius: 11px;
  box-shadow: 0 10px 22px rgba(35, 178, 255, 0.28);
}

.brand-mark {
  display: grid;
  place-items: center;
  font-weight: 800;
  color: #fff;
  background: linear-gradient(135deg, var(--app-primary), #23b2ff);
}

.brand-image {
  object-fit: cover;
  display: block;
}

.brand-text {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.brand-text strong {
  color: #fff;
  font-size: 16px;
  white-space: nowrap;
}

.brand-text span {
  color: rgba(216, 222, 251, 0.72);
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.brand-switch-button {
  width: 30px;
  height: 30px;
  min-width: 30px;
  display: grid;
  place-items: center;
  border: none;
  border-radius: 8px;
  color: #d8defb;
  background: rgba(255, 255, 255, 0.08);
  cursor: pointer;
  transition: background 0.18s ease, color 0.18s ease;
}

.brand-switch-button:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.14);
}

.platform-nav-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  margin-left: auto;
  padding: 0;
  border: 1px solid rgba(177, 197, 255, 0.58);
  border-radius: 12px;
  color: #fff;
  background: linear-gradient(135deg, rgba(112, 134, 255, 0.78), rgba(91, 108, 249, 0.48));
  box-shadow: 0 0 0 3px rgba(130, 151, 255, 0.14), 0 8px 18px rgba(6, 16, 66, 0.28);
  cursor: pointer;
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
  transition: transform 0.18s ease, background 0.18s ease, box-shadow 0.18s ease;
}
.platform-nav-trigger .el-icon { font-size: 19px; }
.platform-nav-trigger > span,
.platform-nav-trigger .platform-nav-arrow { display: none; }
.platform-nav-trigger:hover {
  color: #fff;
  border-color: #fff;
  background: linear-gradient(135deg, #7c8eff, #596cf4);
  box-shadow: 0 0 0 4px rgba(141, 160, 255, 0.22), 0 12px 24px rgba(6, 16, 66, 0.34);
  transform: translateY(-1px);
}
.platform-nav-arrow { font-size: 11px; }
.platform-navigation { padding: 16px; }
.platform-navigation-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 18px;
  margin: 2px 2px 14px;
}
.platform-navigation-head span,
.platform-navigation-head strong { display: block; }
.platform-navigation-head span {
  color: #8493ab;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.12em;
}
.platform-navigation-head strong {
  margin-top: 4px;
  color: #172b4d;
  font-size: 18px;
}
.platform-navigation-head small {
  color: #7a8aa3;
  font-size: 12px;
}
.platform-navigation-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  padding: 12px;
  border: 1px solid #e3eaf5;
  border-radius: 16px;
  background: linear-gradient(145deg, #fbfdff, #f4f8fd);
}
.platform-app-card {
  position: relative;
  display: flex;
  align-items: center;
  min-height: 78px;
  gap: 11px;
  padding: 12px;
  border: 1px solid transparent;
  border-radius: 12px;
  background: transparent;
  color: #172b4d;
  cursor: pointer;
  text-align: left;
  transition: transform 0.16s ease, border-color 0.16s ease, background 0.16s ease, box-shadow 0.16s ease;
}
.platform-app-card:hover {
  transform: translateY(-2px);
  border-color: color-mix(in srgb, var(--app-nav-accent) 34%, #dce6f3);
  background: #fff;
  box-shadow: 0 8px 18px rgba(52, 78, 117, 0.1);
}
.platform-app-card.active {
  border-color: color-mix(in srgb, var(--app-nav-accent) 45%, #dce6f3);
  background: linear-gradient(110deg, var(--app-nav-soft), rgba(255, 255, 255, 0.94));
  box-shadow: inset 0 0 0 1px var(--app-nav-soft);
}
.platform-app-icon {
  display: grid;
  width: 42px;
  height: 42px;
  min-width: 42px;
  place-items: center;
  border-radius: 12px;
  color: #fff;
  background: var(--app-nav-accent);
  box-shadow: 0 8px 16px color-mix(in srgb, var(--app-nav-accent) 28%, transparent);
  font-size: 21px;
}
.platform-app-copy { min-width: 0; }
.platform-app-copy strong,
.platform-app-copy small { display: block; }
.platform-app-copy strong {
  overflow: hidden;
  color: #1e3559;
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.platform-app-copy small {
  margin-top: 5px;
  color: #8190a9;
  font-size: 12px;
}
.platform-app-check {
  position: absolute;
  top: 10px;
  right: 10px;
  color: var(--app-nav-accent);
  font-size: 15px;
}
:global(.platform-nav-popper.el-popper) {
  padding: 0;
  overflow: hidden;
  border: 1px solid #dce6f3;
  border-radius: 18px;
  box-shadow: 0 18px 44px rgba(24, 44, 78, 0.2);
}

.app-switcher-wrap {
  width: 100%;
  margin-bottom: 20px;
}

.app-switcher,
:deep(.app-switcher .el-tooltip__trigger) {
  width: 100%;
  display: block;
}

.app-switcher-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  min-height: 48px;
  width: 100%;
  box-sizing: border-box;
  padding: 0 14px;
  border-radius: 11px;
  background: rgba(255, 255, 255, 0.11);
  border: 1px solid rgba(255, 255, 255, 0.08);
  color: #fff;
  cursor: pointer;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
  transition: background 0.2s ease, border-color 0.2s ease;
}

.app-switcher-trigger:hover {
  background: rgba(255, 255, 255, 0.16);
  border-color: rgba(255, 255, 255, 0.14);
}

.app-switcher-current {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.app-switcher-current span {
  font-size: 14px;
  font-weight: 600;
  line-height: 1;
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.app-switcher-icon {
  width: 28px;
  height: 28px;
  min-width: 28px;
  display: grid;
  place-items: center;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.08);
  font-size: 16px;
}

.app-switcher-arrow {
  color: rgba(255, 255, 255, 0.76);
}

.sidebar-scroll {
  height: calc(100vh - 74px);
}

:deep(.sidebar-menu) {
  border-right: none;
  background: transparent;
}

:deep(.sidebar-menu .el-menu) {
  background: transparent;
}

:deep(.sidebar-menu .el-menu-item),
:deep(.sidebar-menu .el-sub-menu__title) {
  margin: 5px 8px;
  border-radius: 10px;
  color: #d8defb;
}

:deep(.sidebar-menu .el-menu-item .el-icon),
:deep(.sidebar-menu .el-sub-menu__title .el-icon) {
  margin-right: 8px;
}

:deep(.sidebar-menu .el-menu-item:hover),
:deep(.sidebar-menu .el-sub-menu__title:hover) {
  background: rgba(255, 255, 255, 0.08);
}

:deep(.sidebar-menu .el-menu-item.is-active) {
  color: #fff;
  background: linear-gradient(90deg, rgba(91, 108, 249, 0.95), rgba(122, 84, 255, 0.9));
  box-shadow: 0 10px 24px rgba(91, 108, 249, 0.28);
}

:deep(.sidebar-menu > .el-menu-item:first-child),
:deep(.sidebar-menu > .el-sub-menu:first-child .el-sub-menu__title) {
  margin-top: 0;
}

.main-shell {
  min-width: 0;
}

.layout-header {
  height: auto;
  padding: 14px 20px 10px;
  background: rgba(255, 255, 255, 0.94);
  border-bottom: 1px solid #e5eaf4;
  backdrop-filter: blur(10px);
}

.header-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.header-left,
.header-right {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.command-trigger { height: 34px; border-color: #d8e1f0; background: #fff; color: #64748b; }
.command-trigger:hover { border-color: #b9c8e9; background: #f3f6fd; color: #334155; }
.command-trigger kbd, .command-palette-head kbd, .command-result em { margin-left: 6px; padding: 1px 5px; border: 1px solid #d8e1f0; border-radius: 4px; color: #8391a7; font-family: inherit; font-size: 10px; font-style: normal; }

.collapse-button {
  border-color: #d9e0ef;
  color: #334155;
}

.locale-box {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 58px;
  height: 32px;
  justify-content: center;
  padding: 0 8px;
  border-radius: 999px;
  border: 1px solid #d8e1f0;
  background: #fff;
  color: #0f172a;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
}

.locale-box:hover {
  background: #f3f6fd;
}

.locale-item {
  min-width: 72px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.user-box {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border-radius: 999px;
  cursor: pointer;
  color: #111827;
}

.user-box:hover {
  background: #f3f6fd;
}

.user-avatar {
  width: 30px;
  height: 30px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  color: #fff;
  background: linear-gradient(135deg, #1d8cf8, #5b6cf9);
  font-size: 13px;
  font-weight: 700;
}

.tabs-bar {
  display: flex;
  gap: 8px;
  overflow: auto;
  padding-top: 12px;
}

.tab-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 7px 12px;
  border-radius: 10px;
  border: 1px solid #d8e1f0;
  background: #fff;
  color: #52607a;
  cursor: pointer;
  white-space: nowrap;
}

.tab-chip.active {
  color: #fff;
  border-color: transparent;
  background: linear-gradient(90deg, var(--app-primary), #7c5cff);
}

.tab-close {
  border: none;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  padding: 0;
}

.tag-menu-mask {
  position: fixed;
  inset: 0;
  z-index: 3000;
  background: transparent;
}

.tag-context-menu {
  position: fixed;
  min-width: 156px;
  padding: 6px;
  border: 1px solid #e5eaf4;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 14px 34px rgba(15, 23, 42, 0.14);
}

.tag-context-menu button {
  width: 100%;
  height: 34px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: #334155;
  cursor: pointer;
  font-size: 13px;
  text-align: left;
}

.tag-context-menu button:hover {
  color: var(--app-primary);
  background: #f4f7ff;
}

.layout-main {
  padding: 20px;
}

:deep(.app-switcher-menu .el-dropdown-menu__item) {
  min-width: 224px;
  padding: 0;
  line-height: normal;
}

:deep(.app-switcher-menu .el-dropdown-menu__item.is-current-app) {
  background: linear-gradient(90deg, rgba(91, 108, 249, 0.12), rgba(122, 84, 255, 0.08));
}

:deep(.app-switcher-menu.el-dropdown-menu) {
  padding: 10px;
  border: 1px solid #e7ebf4;
  border-radius: 14px;
  box-shadow: 0 18px 48px rgba(15, 23, 42, 0.16);
}

.app-option {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 12px;
}

.app-option-main {
  display: flex;
  align-items: center;
  gap: 12px;
}

.app-option-icon {
  width: 30px;
  height: 30px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  color: var(--app-primary);
  background: rgba(91, 108, 249, 0.08);
}

:deep(.app-switcher-menu .el-dropdown-menu__item:not(.is-disabled):focus) .app-option,
:deep(.app-switcher-menu .el-dropdown-menu__item:not(.is-disabled):hover) .app-option {
  background: #f7f9ff;
}

:deep(.app-drawer .el-drawer__body) {
  padding: 0;
  background: #f7f9fc;
}

.app-drawer-header {
  padding: 22px 22px 18px;
  border-bottom: 1px solid #e6ebf3;
  background: #fff;
}

.app-drawer-header span {
  display: block;
  margin-bottom: 8px;
  font-size: 13px;
  color: #64748b;
}

.app-drawer-header strong {
  font-size: 22px;
  color: #0f172a;
}

.app-drawer-list {
  display: grid;
  gap: 10px;
  padding: 16px;
}

.app-drawer-item {
  width: 100%;
  min-height: 64px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  background: #fff;
  color: #0f172a;
  cursor: pointer;
  text-align: left;
  transition: border-color 0.18s ease, box-shadow 0.18s ease, background 0.18s ease;
}

.app-drawer-item:hover {
  border-color: #cbd5e1;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.08);
}

.app-drawer-item.active {
  border-color: rgba(91, 108, 249, 0.42);
  background: linear-gradient(90deg, rgba(91, 108, 249, 0.12), rgba(91, 108, 249, 0.04));
}

.app-drawer-icon {
  width: 38px;
  height: 38px;
  min-width: 38px;
  display: grid;
  place-items: center;
  border-radius: 10px;
  color: var(--app-primary);
  background: rgba(91, 108, 249, 0.1);
  font-size: 18px;
}

.app-drawer-text {
  flex: 1;
  display: grid;
  gap: 4px;
  min-width: 0;
}

.app-drawer-text strong {
  font-size: 15px;
  color: #0f172a;
}

.app-drawer-text small {
  font-size: 12px;
  color: #64748b;
}

.app-drawer-check {
  color: var(--app-primary);
}

@media (max-width: 960px) {
  .layout-header {
    padding: 12px;
  }

  .layout-main {
    padding: 14px;
  }
}
:global(.command-palette.el-dialog) { overflow: hidden; border: 1px solid #dce6f3; border-top: 2px solid var(--app-primary); border-radius: 12px; background: #fff; box-shadow: 0 26px 70px rgba(24, 44, 78, .22); }
:global(.command-palette .el-dialog__header) { margin: 0; padding: 16px 18px 12px; }
:global(.command-palette .el-dialog__body) { padding: 0 18px 18px; }
.command-palette-head { display: flex; align-items: center; justify-content: space-between; }.command-palette-head span, .command-palette-head strong { display: block; }.command-palette-head span { color: var(--app-primary); font-size: 10px; font-weight: 800; letter-spacing: .11em; }.command-palette-head strong { margin-top: 4px; color: #172b4d; font-size: 17px; }
.command-result-list { display: grid; gap: 6px; max-height: 380px; margin-top: 12px; overflow: auto; }.command-result { display: flex; align-items: center; gap: 11px; width: 100%; padding: 11px 12px; border: 1px solid transparent; border-radius: 8px; background: #f7f9fd; color: #40516d; text-align: left; cursor: pointer; }.command-result:hover { border-color: #c6d9ee; background: #eef4fc; }.command-result > span { display: grid; gap: 2px; min-width: 0; }.command-result b { color: #172b4d; font-size: 13px; }.command-result small { overflow: hidden; color: #8190a9; font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }.command-result em { margin-left: auto; }
</style>
