<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getSystemConfig, updateSystemConfig, uploadSystemAsset } from '../../api/system'
import { applySystemTheme, resolveSystemAsset, setSystemConfig } from '../../utils/system-config'
import { t } from '../../utils/i18n'

const loading = ref(false)
const saving = ref(false)
const logoUploading = ref(false)
const backgroundUploading = ref(false)

const form = reactive({
  siteName: '',
  siteSlogan: '',
  logoType: 'text',
  logoValue: 'OA',
  loginTitle: '',
  loginSubtitle: '',
  useLoginBackground: false,
  loginBackground: '',
  primaryColor: '#5b6cf9',
  sidebarTheme: 'dark',
  rememberLoginEnabled: true
})

const logoPreviewUrl = computed(() => {
  if (form.logoType === 'upload' || form.logoType === 'url') {
    return resolveSystemAsset(form.logoValue)
  }
  return ''
})

const loginBackgroundPreview = computed(() => resolveSystemAsset(form.loginBackground))

async function loadData() {
  loading.value = true
  try {
    const data = await getSystemConfig()
    Object.assign(form, data || {})
  } finally {
    loading.value = false
  }
}

async function submit() {
  saving.value = true
  try {
    const data = await updateSystemConfig(form)
    Object.assign(form, data || {})
    setSystemConfig(data || {})
    applySystemTheme(data || {})
    ElMessage.success(t('saveSuccess'))
  } finally {
    saving.value = false
  }
}

async function handleLogoUpload(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) {
    return
  }
  logoUploading.value = true
  try {
    const data = await uploadSystemAsset(file)
    form.logoType = 'upload'
    form.logoValue = data.url || ''
    ElMessage.success(t('saveSuccess'))
  } finally {
    logoUploading.value = false
  }
}

async function handleBackgroundUpload(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) {
    return
  }
  backgroundUploading.value = true
  try {
    const data = await uploadSystemAsset(file)
    form.useLoginBackground = true
    form.loginBackground = data.url || ''
    ElMessage.success(t('saveSuccess'))
  } finally {
    backgroundUploading.value = false
  }
}

onMounted(loadData)
</script>

<template>
  <div class="page-card console-card-page" v-loading="loading">
    <div class="page-section-head">
      <div>
        <h2 class="card-title">{{ t('configTitle') }}</h2>
        <p class="card-desc">{{ t('configDesc') }}</p>
      </div>
      <el-button type="primary" :loading="saving" v-permission="'system:config:save'" @click="submit">{{ t('saveConfig') }}</el-button>
    </div>

    <el-form label-width="120px" class="config-form">
      <div class="config-grid">
        <div class="config-column">
          <el-form-item :label="t('siteName')">
            <el-input v-model="form.siteName" :placeholder="t('siteName')" />
          </el-form-item>
          <el-form-item :label="t('siteSlogan')">
            <el-input v-model="form.siteSlogan" :placeholder="t('siteSlogan')" />
          </el-form-item>
          <el-form-item :label="t('logoType')">
            <el-radio-group v-model="form.logoType">
              <el-radio value="text">{{ t('textLogo') }}</el-radio>
              <el-radio value="upload">{{ t('uploadLogo') }}</el-radio>
              <el-radio value="url">{{ t('imageUrl') }}</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item v-if="form.logoType === 'text'" :label="t('logoValue')">
            <el-input v-model="form.logoValue" maxlength="8" placeholder="如 OA / OPS" />
          </el-form-item>
          <el-form-item v-else-if="form.logoType === 'upload'" :label="t('uploadLogo')">
            <div class="native-upload-row" :class="{ uploading: logoUploading }">
              <input type="file" accept="image/*" @change="handleLogoUpload" />
            </div>
          </el-form-item>
          <el-form-item v-else :label="t('imageUrl')">
            <el-input v-model="form.logoValue" :placeholder="t('imageUrl')" />
          </el-form-item>
          <el-form-item :label="t('primaryColor')">
            <el-color-picker v-model="form.primaryColor" />
          </el-form-item>
          <el-form-item :label="t('rememberLoginEnabled')">
            <div class="remember-login-setting">
              <el-switch v-model="form.rememberLoginEnabled" />
              <span>{{ t('rememberLoginSettingHint') }}</span>
            </div>
          </el-form-item>
        </div>

        <div class="config-column">
          <el-form-item :label="t('loginTitle')">
            <el-input v-model="form.loginTitle" :placeholder="t('loginTitle')" />
          </el-form-item>
          <el-form-item :label="t('loginSubtitle')">
            <el-input v-model="form.loginSubtitle" :placeholder="t('loginSubtitle')" />
          </el-form-item>
          <el-form-item :label="t('useLoginBackground')">
            <el-switch v-model="form.useLoginBackground" />
          </el-form-item>
          <el-form-item :label="t('loginBackground')">
            <el-input v-model="form.loginBackground" :placeholder="t('loginBackground')" />
          </el-form-item>
          <el-form-item :label="t('uploadBackground')">
            <div class="native-upload-row" :class="{ uploading: backgroundUploading }">
              <input type="file" accept="image/*" @change="handleBackgroundUpload" />
            </div>
          </el-form-item>
          <el-form-item :label="t('sidebarTheme')">
            <el-radio-group v-model="form.sidebarTheme">
              <el-radio value="dark">{{ t('darkTheme') }}</el-radio>
              <el-radio value="light">{{ t('lightTheme') }}</el-radio>
            </el-radio-group>
          </el-form-item>
        </div>
      </div>
    </el-form>

    <div class="preview-grid">
      <div class="preview-card">
        <span class="preview-label">{{ t('logoPreview') }}</span>
        <div v-if="form.logoType === 'text'" class="logo-preview" :style="{ background: `linear-gradient(135deg, ${form.primaryColor || '#5b6cf9'}, #23b2ff)` }">
          {{ (form.logoValue || 'OA').slice(0, 2).toUpperCase() }}
        </div>
        <div v-else class="image-preview-shell logo-shell">
          <img v-if="logoPreviewUrl" :src="logoPreviewUrl" alt="logo-preview" class="image-preview logo-image" />
          <span v-else>暂无图片</span>
        </div>
      </div>

      <div class="preview-card">
        <span class="preview-label">{{ t('loginPreview') }}</span>
        <div class="login-preview" :style="{ backgroundImage: form.useLoginBackground && loginBackgroundPreview ? `linear-gradient(135deg, rgba(18,27,71,.7), rgba(41,64,149,.68)), url(${loginBackgroundPreview})` : '' }">
          <div class="login-preview-panel">
            <strong>{{ form.loginTitle || form.siteName || 'Ops Admin' }}</strong>
            <span>{{ form.loginSubtitle || form.siteSlogan || '运维管理平台' }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page-section-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
}

.card-title {
  margin: 0;
  font-size: 28px;
}

.card-desc {
  margin: 8px 0 0;
  color: #7b879c;
}

.config-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 24px;
}

.preview-grid {
  display: grid;
  grid-template-columns: 260px 1fr;
  gap: 18px;
  margin-top: 10px;
}

.preview-card {
  padding: 18px;
  border: 1px solid #e6ebf5;
  border-radius: 18px;
  background: #f8faff;
}

.preview-label {
  display: inline-block;
  margin-bottom: 14px;
  color: #64748b;
}

.logo-preview {
  width: 72px;
  height: 72px;
  border-radius: 18px;
  display: grid;
  place-items: center;
  color: #fff;
  font-size: 24px;
  font-weight: 800;
}

.image-preview-shell {
  border-radius: 18px;
  border: 1px dashed #cdd8ec;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  background: #fff;
  color: #94a3b8;
}

.logo-shell {
  width: 72px;
  height: 72px;
}

.image-preview {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.logo-image {
  object-fit: cover;
}

.login-preview {
  min-height: 220px;
  border-radius: 18px;
  background: linear-gradient(135deg, #192558, #2f4fb2);
  background-size: cover;
  background-position: center;
  display: flex;
  align-items: center;
  justify-content: center;
}

.login-preview-panel {
  min-width: 280px;
  padding: 24px;
  border-radius: 20px;
  color: #fff;
  background: rgba(10, 18, 48, 0.68);
  backdrop-filter: blur(8px);
}

.login-preview-panel strong {
  display: block;
  font-size: 26px;
}

.login-preview-panel span {
  display: block;
  margin-top: 8px;
  color: rgba(255, 255, 255, 0.82);
}

.native-upload-row {
  display: inline-flex;
  align-items: center;
}

.native-upload-row.uploading {
  opacity: 0.65;
  pointer-events: none;
}

.remember-login-setting {
  display: flex;
  align-items: center;
  gap: 12px;
  color: #7b879c;
  line-height: 1.5;
}

@media (max-width: 1100px) {
  .config-grid,
  .preview-grid {
    grid-template-columns: 1fr;
  }
}
</style>
