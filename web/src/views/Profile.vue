<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { profile, updatePersonal, updatePersonalPassword } from '../api/system'
import { getUser, setUser } from '../utils/auth'
import { t, translateEntity } from '../utils/i18n'

const loading = ref(false)
const saving = ref(false)
const passwordSaving = ref(false)
const editVisible = ref(false)
const passwordVisible = ref(false)
const currentUser = ref(getUser())

const form = reactive({
  nickname: '',
  email: '',
  phone: '',
  note: ''
})

const passwordForm = reactive({
  password: '',
  newPassword: '',
  resetPassword: ''
})

const userName = computed(() => currentUser.value.nickname || currentUser.value.username || '管理员')

async function loadProfile() {
  loading.value = true
  try {
    const data = await profile()
    currentUser.value = data.user || {}
    setUser(data.user || {})
  } finally {
    loading.value = false
  }
}

function openEdit() {
  Object.assign(form, {
    nickname: currentUser.value.nickname || '',
    email: currentUser.value.email || '',
    phone: currentUser.value.phone || '',
    note: currentUser.value.note || ''
  })
  editVisible.value = true
}

function openPasswordDialog() {
  passwordForm.password = ''
  passwordForm.newPassword = ''
  passwordForm.resetPassword = ''
  passwordVisible.value = true
}

async function submitProfile() {
  saving.value = true
  try {
    await updatePersonal({
      username: currentUser.value.username,
      nickname: form.nickname,
      email: form.email,
      phone: form.phone,
      note: form.note
    })
    await loadProfile()
    editVisible.value = false
    ElMessage.success(t('profileUpdateSuccess'))
  } finally {
    saving.value = false
  }
}

async function submitPassword() {
  if (!passwordForm.password || !passwordForm.newPassword || !passwordForm.resetPassword) {
    ElMessage.warning('请完整填写密码信息')
    return
  }
  if (passwordForm.newPassword.length < 6) {
    ElMessage.warning('新密码至少 6 位')
    return
  }
  if (passwordForm.newPassword !== passwordForm.resetPassword) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }

  passwordSaving.value = true
  try {
    await updatePersonalPassword(passwordForm)
    passwordVisible.value = false
    ElMessage.success(t('passwordUpdateSuccess'))
  } finally {
    passwordSaving.value = false
  }
}

onMounted(loadProfile)
</script>

<template>
  <div class="profile-page" v-loading="loading">
    <div class="profile-hero page-card">
      <div class="profile-badge">{{ userName.slice(0, 1).toUpperCase() }}</div>
      <div class="profile-summary">
        <h1>{{ userName }}</h1>
        <p>@{{ currentUser.username || 'admin' }}</p>
        <div class="profile-tags">
          <span>{{ translateEntity(currentUser.roleName, t('superAdmin')) }}</span>
        </div>
      </div>
      <div class="profile-actions">
        <el-button @click="openEdit">{{ t('editProfile') }}</el-button>
        <el-button type="primary" @click="openPasswordDialog">{{ t('changePassword') }}</el-button>
      </div>
    </div>

    <div class="profile-grid">
      <div class="page-card">
        <h3 class="section-title">{{ t('accountInfo') }}</h3>
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="t('username')">{{ currentUser.username || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('nickname')">{{ currentUser.nickname || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('email')">{{ currentUser.email || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('phone')">{{ currentUser.phone || '-' }}</el-descriptions-item>
          <el-descriptions-item label="角色">{{ translateEntity(currentUser.roleName, '-') }}</el-descriptions-item>
          <el-descriptions-item label="状态">{{ currentUser.status === 1 ? '启用' : '停用' }}</el-descriptions-item>
        </el-descriptions>
      </div>

      <div class="page-card">
        <h3 class="section-title">{{ t('extraInfo') }}</h3>
        <div class="note-box">{{ currentUser.note || '暂未填写个人说明。' }}</div>
        <div class="meta-row">
          <span>创建时间</span>
          <strong>{{ currentUser.createTime || '-' }}</strong>
        </div>
        <div class="meta-row">
          <span>更新时间</span>
          <strong>{{ currentUser.updateTime || '-' }}</strong>
        </div>
      </div>
    </div>

    <el-dialog v-model="editVisible" :title="t('editProfile')" width="620px">
      <el-form label-width="88px">
        <el-form-item :label="t('nickname')">
          <el-input v-model="form.nickname" />
        </el-form-item>
        <el-form-item :label="t('email')">
          <el-input v-model="form.email" />
        </el-form-item>
        <el-form-item :label="t('phone')">
          <el-input v-model="form.phone" />
        </el-form-item>
        <el-form-item :label="t('note')">
          <el-input v-model="form.note" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">{{ t('cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="submitProfile">{{ t('save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="passwordVisible" :title="t('changePassword')" width="520px">
      <el-form label-width="96px">
        <el-form-item :label="t('currentPassword')">
          <el-input v-model="passwordForm.password" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('newPassword')">
          <el-input v-model="passwordForm.newPassword" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('confirmPassword')">
          <el-input v-model="passwordForm.resetPassword" type="password" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="passwordVisible = false">{{ t('cancel') }}</el-button>
        <el-button type="primary" :loading="passwordSaving" @click="submitPassword">{{ t('confirmChange') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.profile-page {
  display: grid;
  gap: 18px;
}

.profile-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}

.profile-badge {
  width: 88px;
  height: 88px;
  border-radius: 28px;
  display: grid;
  place-items: center;
  color: #fff;
  font-size: 34px;
  font-weight: 800;
  background: linear-gradient(135deg, var(--app-primary), #2bb9ff);
}

.profile-summary {
  flex: 1;
}

.profile-summary h1 {
  margin: 0;
}

.profile-summary p {
  margin: 8px 0 12px;
  color: #64748b;
}

.profile-tags {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.profile-tags span {
  padding: 6px 10px;
  border-radius: 999px;
  background: #eef3ff;
  color: #475569;
}

.profile-actions {
  display: flex;
  gap: 12px;
}

.profile-grid {
  display: grid;
  grid-template-columns: 1.3fr 0.9fr;
  gap: 18px;
}

.section-title {
  margin: 0 0 16px;
}

.note-box {
  min-height: 120px;
  padding: 16px;
  border-radius: 16px;
  background: #f8faff;
  color: #475569;
  margin-bottom: 16px;
}

.meta-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 0;
  border-top: 1px solid #edf1f7;
}

@media (max-width: 1100px) {
  .profile-hero,
  .profile-grid {
    grid-template-columns: 1fr;
    flex-direction: column;
    align-items: flex-start;
  }

  .profile-actions {
    width: 100%;
  }
}
</style>
