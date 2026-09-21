<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getPublicSystemConfig, login } from '../api/system'
import { setAuthPersistence, setMenus, setPermissions, setToken, setUser } from '../utils/auth'
import { applySystemTheme, getSystemConfig, resolveSystemAsset, setSystemConfig } from '../utils/system-config'
import { t } from '../utils/i18n'

const router = useRouter()
const loading = ref(false)
const configLoaded = ref(false)
const config = ref(getSystemConfig())
const form = reactive({
  username: '',
  password: '',
  rememberLogin: false
})

const canSubmit = computed(() => Boolean(form.username.trim() && form.password))

const logoText = computed(() => {
  if (config.value.logoType === 'text' && config.value.logoValue) {
    return String(config.value.logoValue).slice(0, 2).toUpperCase()
  }
  return 'OA'
})

const logoImage = computed(() => {
  if (config.value.logoType === 'upload' || config.value.logoType === 'url') {
    return resolveSystemAsset(config.value.logoValue)
  }
  return ''
})

const loginBackgroundStyle = computed(() => {
  const bg = resolveSystemAsset(config.value.loginBackground)
  if (config.value.useLoginBackground && bg) {
    return {
      backgroundImage: `linear-gradient(135deg, rgba(18, 27, 71, 0.82), rgba(41, 64, 149, 0.72)), url(${bg})`
    }
  }
  return {}
})

async function loadSystemConfig() {
  try {
    const data = await getPublicSystemConfig()
    config.value = data || {}
    if (!config.value.rememberLoginEnabled) form.rememberLogin = false
    setSystemConfig(config.value)
    applySystemTheme(config.value)
  } catch {
    applySystemTheme(config.value)
  } finally {
    configLoaded.value = true
  }
}

async function submit() {
  if (!form.username.trim() || !form.password) {
    ElMessage.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    const data = await login({ ...form, username: form.username.trim() })
    setAuthPersistence(data.rememberLogin === true)
    setToken(data.token, data.accessTokenExpiresAt, data.sessionExpiresAt)
    setUser(data.sysAdmin)
    setMenus(data.leftMenuList || [])
    setPermissions(data.permissionList || [])
    if (data.systemConfig) {
      setSystemConfig(data.systemConfig)
      applySystemTheme(data.systemConfig)
    }
    ElMessage.success('登录成功')
    await router.push('/dashboard')
  } catch {
    // 请求层负责展示服务端错误；组件在此收口异常，避免 Vue 未捕获事件告警。
  } finally {
    loading.value = false
  }
}

onMounted(loadSystemConfig)
</script>

<template>
  <div class="login-shell">
    <div class="login-backdrop" :style="loginBackgroundStyle"></div>
    <div class="login-panel">
      <section class="login-hero">
        <div class="hero-badge">
          <img v-if="logoImage" :src="logoImage" alt="logo" class="hero-logo-image" />
          <template v-else>{{ logoText }}</template>
        </div>
        <h1>{{ config.loginTitle || config.siteName || 'Ops Admin' }}</h1>
        <p>{{ config.loginSubtitle || config.siteSlogan || '运维系统管理后台' }}</p>
        <div class="hero-meta">
          <span>System</span>
          <span>RBAC</span>
          <span>Audit</span>
        </div>
      </section>

      <section class="login-form">
        <div class="form-head">
          <h2>{{ t('loginWelcome') }}</h2>
          <p>{{ t('loginHint') }}</p>
        </div>
        <el-form label-position="top" @submit.prevent="submit">
          <el-form-item :label="t('username')">
            <el-input v-model="form.username" :placeholder="t('username')" />
          </el-form-item>
          <el-form-item :label="t('password')">
            <el-input v-model="form.password" type="password" show-password :placeholder="t('password')" />
          </el-form-item>
          <el-button native-type="submit" type="primary" class="submit-btn" :loading="loading" :disabled="!canSubmit">{{ t('loginSystem') }}</el-button>
          <div v-if="configLoaded && config.rememberLoginEnabled" class="remember-login-row">
            <el-checkbox v-model="form.rememberLogin">{{ t('rememberLoginSevenDays') }}</el-checkbox>
            <span>{{ t('rememberLoginFixedHint') }}</span>
          </div>
        </el-form>
      </section>
    </div>
  </div>
</template>
