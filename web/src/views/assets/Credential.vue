<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search, Lock, Key, Connection, ArrowRight, Edit, Delete, CircleCheck, Files } from '@element-plus/icons-vue'
import { addAssetCredential, assetCredentialInfo, deleteAssetCredential, queryAssetCredentialList, updateAssetCredential } from '../../api/asset'

const router = useRouter()
const loading = ref(false), saving = ref(false), dialogVisible = ref(false), isEdit = ref(false)
const formRef = ref(null), fileInput = ref(null), fileTarget = ref(''), originalType = ref('password'), tableData = ref([]), total = ref(0)
const query = reactive({ pageNum: 1, pageSize: 10, keyword: '', authType: '' })
const types = [
  { value: 'password', title: '密码认证', label: 'SSH 密码', icon: Lock, tone: 'blue', use: '主机登录', description: '使用用户名与密码连接主机', hint: '适用于通过 SSH 密码认证的主机、网关。' },
  { value: 'key', title: '密钥认证', label: 'SSH 密钥', icon: Key, tone: 'teal', use: '主机登录', description: '使用 SSH 私钥连接主机', hint: '保存登录私钥；私钥已加密时，请同时填写密钥口令。' },
  { value: 'tls', title: 'TLS 证书', label: 'TLS / mTLS', icon: Files, tone: 'amber', use: '加密连接', description: 'CA 与客户端证书认证', hint: '用于 etcd 等需要 TLS 的中间件：单向加密只填 CA 证书；要求客户端认证时，再一并提供客户端证书与私钥。' },
  { value: 'jumpserver_key', title: 'JumpServer API Key', label: 'API Key', icon: Connection, tone: 'violet', use: '资产同步', description: '授权读取 JumpServer 资产与节点', hint: '用于 JumpServer API 签名访问。主机登录请另外选择 SSH 凭据。' }
]
const typeInfo = value => types.find(type => type.value === value) || types[0]
const selectedType = computed(() => typeInfo(form.authType))
const keepExistingSecret = computed(() => isEdit.value && originalType.value === form.authType)
const form = reactive({ id: undefined, name: '', authType: 'password', username: '', password: '', privateKey: '', passphrase: '', caCert: '', clientCert: '', status: 1, description: '' })
const storedCert = reactive({ ca: false, cert: false, key: false })
let requestGeneration = 0

const identityLabel = row => row.authType === 'jumpserver_key' ? 'ACCESS KEY ID' : row.authType === 'tls' ? '证书内容' : 'SSH USER'
const identityValue = row => {
  if (row.authType !== 'tls') return row.username || '—'
  const parts = [row.caCert ? 'CA 证书' : '', row.clientCert ? '客户端证书' : ''].filter(Boolean)
  return parts.length ? parts.join(' + ') : '—'
}

function resetForm() {
  Object.assign(form, { id: undefined, name: '', authType: 'password', username: '', password: '', privateKey: '', passphrase: '', caCert: '', clientCert: '', status: 1, description: '' })
  Object.assign(storedCert, { ca: false, cert: false, key: false })
  originalType.value = 'password'
  formRef.value?.clearValidate()
}
const rules = computed(() => ({
  name: [{ required: true, whitespace: true, message: '请输入凭据名称', trigger: 'blur' }],
  username: [{ required: form.authType !== 'tls', validator: (_, value, callback) => {
    if (form.authType === 'tls') return callback()
    if (!value?.trim()) return callback(new Error(form.authType === 'jumpserver_key' ? '请输入 Access Key ID' : '请输入登录用户名'))
    if (form.authType === 'jumpserver_key' && !/^[a-f\d]{8}-[a-f\d]{4}-[a-f\d]{4}-[a-f\d]{4}-[a-f\d]{12}$/i.test(value.trim())) return callback(new Error('Access Key ID 应为 UUID 格式'))
    callback()
  }, trigger: 'blur' }],
  password: [{ required: form.authType !== 'key' && form.authType !== 'tls' && !keepExistingSecret.value, validator: (_, value, callback) => {
    if (form.authType === 'key' || form.authType === 'tls') return callback()
    if (!keepExistingSecret.value && !value) return callback(new Error(form.authType === 'jumpserver_key' ? '请输入 Key Secret' : '请输入登录密码'))
    callback()
  }, trigger: 'blur' }],
  privateKey: [{ required: form.authType === 'key' && !keepExistingSecret.value, validator: (_, value, callback) => {
    if (form.authType === 'key') {
      if (!keepExistingSecret.value && !value?.trim()) return callback(new Error('请输入 SSH 私钥'))
      return callback()
    }
    if (form.authType !== 'tls') return callback()
    const cert = (form.clientCert || '').trim(), key = (value || '').trim()
    if (cert && !key && !keepExistingSecret.value) return callback(new Error('填写客户端证书后需要同时提供客户端私钥'))
    if (key && !cert) return callback(new Error('填写客户端私钥后需要同时提供客户端证书'))
    callback()
  }, trigger: 'blur' }],
  caCert: [{ validator: (_, value, callback) => {
    if (form.authType !== 'tls') return callback()
    const ca = (value || '').trim(), cert = (form.clientCert || '').trim(), key = (form.privateKey || '').trim()
    if (!ca && !cert && !key && !keepExistingSecret.value) return callback(new Error('请至少填写 CA 证书，或同时填写客户端证书与私钥'))
    callback()
  }, trigger: 'blur' }]
}))

async function loadData() {
  const generation = ++requestGeneration
  loading.value = true
  try {
    const data = await queryAssetCredentialList({ ...query })
    if (generation !== requestGeneration) return
    tableData.value = data.list || []; total.value = data.total || 0
  } finally { if (generation === requestGeneration) loading.value = false }
}
function search() { query.pageNum = 1; loadData() }
function chooseFilter(type) { query.authType = type; search() }
function resetSearch() { query.keyword = ''; query.authType = ''; search() }
function openCreate() { resetForm(); isEdit.value = false; form.authType = query.authType || 'password'; dialogVisible.value = true }
async function openEdit(row) {
  const data = await assetCredentialInfo(row.id)
  resetForm(); isEdit.value = true; originalType.value = data.authType
  Object.assign(storedCert, { ca: Boolean(data.caCert), cert: Boolean(data.clientCert), key: Boolean(data.privateKey) })
  Object.assign(form, data, { password: '', privateKey: '', passphrase: '', caCert: '', clientCert: '' }); dialogVisible.value = true
}
function chooseType(type) {
  if (form.authType === type) return
  if (type === 'jumpserver_key' || form.authType === 'jumpserver_key') form.username = ''
  form.authType = type; form.password = ''; form.privateKey = ''; form.passphrase = ''; form.caCert = ''; form.clientCert = ''
  formRef.value?.clearValidate()
}
function pickFile(target) { fileTarget.value = target; fileInput.value?.click() }
async function readFile(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  if (file.size > 256 * 1024) { ElMessage.warning('证书文件过大，请确认是否为 PEM 文件'); return }
  try {
    const text = await file.text()
    if (!text.includes('-----BEGIN')) { ElMessage.warning(`${file.name} 不是 PEM 格式，请上传 .pem / .crt / .key 文件`); return }
    form[fileTarget.value === 'ca' ? 'caCert' : fileTarget.value === 'cert' ? 'clientCert' : 'privateKey'] = text.trim()
    formRef.value?.clearValidate()
    ElMessage.success(`已读取 ${file.name}`)
  } catch { ElMessage.error('读取文件失败，请重试或直接粘贴内容') }
}
async function submit() {
  if (saving.value) return
  if (!await formRef.value.validate().catch(() => false)) return
  saving.value = true
  try {
    if (isEdit.value) await updateAssetCredential({ ...form })
    else await addAssetCredential({ ...form })
    ElMessage.success(isEdit.value ? '凭据已更新' : '凭据已创建')
    dialogVisible.value = false; await loadData()
  } finally { saving.value = false }
}
async function handleDelete(row) {
  try { await ElMessageBox.confirm(`确认删除凭据“${row.name}”？正在被主机或同步源使用的凭据无法删除。`, '删除凭据', { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }) } catch { return }
  await deleteAssetCredential(row.id)
  if (tableData.value.length === 1 && query.pageNum > 1) query.pageNum--
  ElMessage.success('凭据已删除'); await loadData()
}
onMounted(loadData)
</script>

<template>
  <div class="credential-page">
    <header class="credential-heading">
      <div><div class="credential-eyebrow">资产管理 / 访问凭据</div><h1>凭据管理</h1><p>集中维护主机登录、资产同步与加密连接凭据，在需要时统一引用。</p></div>
      <el-button type="primary" :icon="Plus" @click="openCreate">新增凭据</el-button>
    </header>

    <div class="credential-types" aria-label="按认证类型筛选">
      <button v-for="type in types" :key="type.value" type="button" class="credential-type" :class="[type.tone, { selected: query.authType === type.value }]" :aria-pressed="query.authType === type.value" @click="chooseFilter(query.authType === type.value ? '' : type.value)">
        <span class="credential-icon"><component :is="type.icon" /></span>
        <span class="type-copy"><strong>{{ type.title }}</strong><small>{{ type.description }}</small></span>
        <span class="type-use">{{ type.use }}</span><el-icon v-if="query.authType === type.value" class="selected-check"><CircleCheck /></el-icon>
      </button>
    </div>

    <section class="credential-panel">
      <div class="credential-toolbar">
        <div class="credential-search"><el-input v-model="query.keyword" :prefix-icon="Search" clearable placeholder="搜索凭据名称、用户名 / Key ID" @keyup.enter="search" @clear="search" /><el-button type="primary" plain @click="search">查询</el-button><el-button @click="resetSearch">重置</el-button></div>
        <div class="credential-list-label"><strong>{{ query.authType ? typeInfo(query.authType).title : '全部凭据' }}</strong><span>{{ total }} 项</span><el-button v-if="query.authType" link type="primary" @click="chooseFilter('')">查看全部</el-button></div>
      </div>

      <el-table v-loading="loading" :data="tableData" class="credential-table" row-key="id">
        <el-table-column label="凭据" min-width="230"><template #default="{ row }"><div class="credential-name"><span class="credential-icon" :class="typeInfo(row.authType).tone"><component :is="typeInfo(row.authType).icon" /></span><div><strong>{{ row.name }}</strong><small>{{ typeInfo(row.authType).use }} · #{{ row.id }}</small></div></div></template></el-table-column>
        <el-table-column label="认证类型" min-width="180"><template #default="{ row }"><span class="credential-kind" :class="typeInfo(row.authType).tone">{{ typeInfo(row.authType).title }}</span></template></el-table-column>
        <el-table-column label="用户名 / 证书" min-width="240"><template #default="{ row }"><div class="credential-identity"><small>{{ identityLabel(row) }}</small><code>{{ identityValue(row) }}</code></div></template></el-table-column>
        <el-table-column label="状态" width="105"><template #default="{ row }"><span class="credential-status" :class="{ disabled: row.status !== 1 }"><i />{{ row.status === 1 ? '可用' : '已停用' }}</span></template></el-table-column>
        <el-table-column label="备注" min-width="180" show-overflow-tooltip><template #default="{ row }"><span class="credential-description">{{ row.description || '—' }}</span></template></el-table-column>
        <el-table-column label="操作" width="142" fixed="right"><template #default="{ row }"><el-button link type="primary" :icon="Edit" @click="openEdit(row)">编辑</el-button><el-button link type="danger" :icon="Delete" @click="handleDelete(row)">删除</el-button></template></el-table-column>
        <template #empty><el-empty :description="query.keyword || query.authType ? '没有找到符合条件的凭据' : '添加第一条凭据，供主机或同步源使用'" :image-size="80"><el-button v-if="!query.keyword" type="primary" plain :icon="Plus" @click="openCreate">新增凭据</el-button><el-button v-else @click="resetSearch">清除筛选</el-button></el-empty></template>
      </el-table>

      <footer class="credential-pager"><span><el-icon><Lock /></el-icon> 密码、私钥、证书与 Secret 不在列表中展示</span><el-pagination v-model:current-page="query.pageNum" v-model:page-size="query.pageSize" :page-sizes="[10, 20, 50]" layout="total, sizes, prev, pager, next" :total="total" @current-change="loadData" @size-change="search" /></footer>
    </section>

    <aside class="credential-guidance"><span class="credential-icon violet"><Connection /></span><div><strong>接入 JumpServer？</strong><p>创建 API Key 凭据读取资产，再用 SSH 凭据配置主机登录。</p></div><el-button link type="primary" @click="router.push('/assets/server/sync')">前往资产同步 <el-icon><ArrowRight /></el-icon></el-button></aside>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑凭据' : '新增凭据'" width="min(760px, calc(100vw - 28px))" top="5vh" class="credential-dialog" :close-on-click-modal="false" :close-on-press-escape="!saving" :show-close="!saving" @closed="resetForm">
      <input ref="fileInput" type="file" accept=".pem,.crt,.cer,.key,.txt" class="cert-file-input" @change="readFile">
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" :disabled="saving" :validate-on-rule-change="false" @submit.prevent="submit">
        <div class="form-section-heading"><span>认证方式</span><small>选择凭据的使用场景</small></div>
        <div class="form-type-options" role="group" aria-label="认证方式">
          <button v-for="type in types" :key="type.value" type="button" :disabled="saving" :aria-pressed="form.authType === type.value" :class="[type.tone, { selected: form.authType === type.value }]" @click="chooseType(type.value)"><component :is="type.icon" /><strong>{{ type.value === 'jumpserver_key' ? 'JumpServer API' : type.title }}</strong><small>{{ type.use }}</small><CircleCheck v-if="form.authType === type.value" class="type-check" /></button>
        </div>
        <div class="credential-form-hint" :class="selectedType.tone"><el-icon><component :is="selectedType.icon" /></el-icon>{{ selectedType.hint }}</div>

        <div class="form-section-heading"><span>基本信息</span></div>
        <div class="credential-form-grid"><el-form-item label="凭据名称" prop="name" :class="{ 'full-row': form.authType === 'tls' }"><el-input v-model="form.name" maxlength="128" placeholder="例如：生产运维密钥" /></el-form-item><el-form-item v-if="form.authType !== 'tls'" :label="form.authType === 'jumpserver_key' ? 'Access Key ID' : '登录用户名'" prop="username"><el-input v-model="form.username" :placeholder="form.authType === 'jumpserver_key' ? 'JumpServer Key ID（UUID）' : '例如：ops / root'" autocomplete="off" spellcheck="false" /></el-form-item></div>

        <div class="form-section-heading"><span>{{ form.authType === 'jumpserver_key' ? 'API 访问密钥' : form.authType === 'tls' ? '证书内容' : '认证信息' }}</span><small v-if="keepExistingSecret">留空保留原值</small></div>
        <template v-if="form.authType === 'tls'">
          <el-form-item prop="caCert" class="cert-field">
            <template #label><span class="cert-label">CA 证书 <small>可选，留空使用系统信任的根证书</small><el-button link type="primary" :icon="Files" :disabled="saving" @click="pickFile('ca')">选择文件</el-button></span></template>
            <el-input v-model="form.caCert" class="private-key-input" type="textarea" :rows="3" autocomplete="off" spellcheck="false" :placeholder="storedCert.ca ? '已配置 CA 证书，留空则不修改' : '粘贴 CA 证书（PEM），或从本地选择 .crt / .pem 文件'" />
          </el-form-item>
          <div class="credential-form-grid">
            <el-form-item prop="clientCert" class="cert-field">
              <template #label><span class="cert-label">客户端证书 <small>双向认证时填写</small><el-button link type="primary" :icon="Files" :disabled="saving" @click="pickFile('cert')">选择文件</el-button></span></template>
              <el-input v-model="form.clientCert" class="private-key-input" type="textarea" :rows="3" autocomplete="off" spellcheck="false" :placeholder="storedCert.cert ? '已配置客户端证书，留空则不修改' : '粘贴客户端证书（PEM）'" />
            </el-form-item>
            <el-form-item prop="privateKey" class="cert-field">
              <template #label><span class="cert-label">客户端私钥 <small>与证书成对</small><el-button link type="primary" :icon="Files" :disabled="saving" @click="pickFile('key')">选择文件</el-button></span></template>
              <el-input v-model="form.privateKey" class="private-key-input" type="textarea" :rows="3" autocomplete="off" spellcheck="false" :placeholder="storedCert.key ? '已配置客户端私钥，留空则不修改' : '粘贴客户端私钥（PEM）'" />
            </el-form-item>
          </div>
        </template>
        <template v-else>
        <el-form-item v-if="form.authType !== 'key'" :label="form.authType === 'jumpserver_key' ? 'Key Secret' : '登录密码'" prop="password"><el-input v-model="form.password" type="password" show-password autocomplete="new-password" :placeholder="keepExistingSecret ? '已配置，留空则不修改' : form.authType === 'jumpserver_key' ? '填写 JumpServer Access Key Secret' : '填写主机登录密码'" /></el-form-item>
        <template v-else><el-form-item label="SSH 私钥" prop="privateKey"><el-input v-model="form.privateKey" class="private-key-input" type="textarea" :rows="5" autocomplete="off" spellcheck="false" :placeholder="keepExistingSecret ? '已配置，留空则不修改' : '粘贴完整私钥内容，包含 BEGIN / END 标记'" /></el-form-item><el-form-item label="密钥口令（可选）"><el-input v-model="form.passphrase" type="password" show-password autocomplete="new-password" :placeholder="keepExistingSecret ? '留空则保持原值' : '仅在私钥已加密时填写'" /></el-form-item></template>
        </template>

        <div class="credential-form-grid final-row"><el-form-item label="凭据状态"><div class="status-control"><el-switch v-model="form.status" :active-value="1" :inactive-value="2" /><span>{{ form.status === 1 ? '可用，可被新资产引用' : '已停用，暂停供新资产选择' }}</span></div></el-form-item><el-form-item label="备注（可选）"><el-input v-model="form.description" maxlength="255" placeholder="说明用途、环境或负责人" /></el-form-item></div>
      </el-form>
      <template #footer><div class="credential-dialog-footer"><span><el-icon><Lock /></el-icon> 认证信息不会回显</span><el-button :disabled="saving" @click="dialogVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="submit">{{ isEdit ? '保存更改' : '创建凭据' }}</el-button></div></template>
    </el-dialog>
  </div>
</template>

<style scoped src="./credential.css"></style>
