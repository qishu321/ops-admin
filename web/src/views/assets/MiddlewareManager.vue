<script setup>
import { tr } from '../../utils/i18n'
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Refresh, Search, ArrowLeft } from '@element-plus/icons-vue'
import { getUser, getPermissions } from '../../utils/auth'
import { listMiddleware, saveMiddleware, deleteMiddleware, testMiddleware, operateMiddleware } from '../../api/middleware'
import { queryAssetCredentialOptions } from '../../api/asset'
import EtcdExplorer from './EtcdExplorer.vue'
import { middlewareTypes, middlewareRows, middlewareValue } from './middleware-schema'
const props = defineProps({ kind: { type: String, required: true } })
const router = useRouter(), route = useRoute()
const spec = computed(() => middlewareTypes[props.kind])
const sources = ref([]), credentials = ref([]), loading = ref(false), saving = ref(false), testing = ref(false), busy = ref(false)
const search = ref(''), selected = ref(null), tab = ref(''), result = ref(null), error = ref(''), sourceDialog = ref(false), actionDialog = ref(false), detailDialog = ref(false), detail = ref(null), activeOperation = ref(null)
const revision = ref(0)
const isAdmin = computed(() => { revision.value; return getUser().accountType === 'administrator' })
const can = action => { revision.value; return isAdmin.value || getPermissions().includes(`assets:middleware:${action === 'read' ? 'list' : action}`) }
const form = reactive({}), params = reactive({}), operationForm = reactive({})
const endpointText = computed({ get: () => form.endpointText || '', set: value => { form.endpointText = value } }), sourceFormRef = ref(null), credentialLoading = ref(false), credentialError = ref(''), listError = ref(''), testingId = ref(null), resourceSearch = ref(''), actionPending = ref(false)
const sourceRules = computed(() => ({
 name: [{required:true, whitespace:true, message:tr("请输入数据源名称"), trigger:'blur'}],
 endpointText: [{required:true, whitespace:true, message:tr("请输入连接地址"), trigger:'blur'}],
 credentialId: [{required:true, message:tr("请选择认证凭据"), validator:(_rule,value,done) => form.authType === 'none' || Number(value) > 0 ? done() : done(new Error(tr("请选择认证凭据"))), trigger:'change'}]
}))
let sourceGeneration = 0
const tlsHint = computed(() => form.tls ? tr("原生 Kafka 协议使用 TLS 加密") : tr("可启用 TLS 加密连接"))
const visibleRows = computed(() => rows.value.filter(row => Object.values(row).some(value => String(typeof value === 'object' ? JSON.stringify(value) : value).toLowerCase().includes(resourceSearch.value.trim().toLowerCase()))))

const filtered = computed(() => sources.value.filter(x => `${x.name} ${x.description}`.toLowerCase().includes(search.value.toLowerCase())))
const rows = computed(() => middlewareRows(props.kind, tab.value, result.value, params))
const columns = computed(() => (spec.value.columns[tab.value] || []).filter(([key]) => rows.value.length === 0 || rows.value.some(r => middlewareValue(r, key) !== '—')))
const contextFields = computed(() => spec.value.fields.filter(f => ['vhost','namespaceId','groupName'].includes(f.key)))
const actionFields = computed(() => [...contextFields.value.filter(f => !activeOperation.value?.fields.some(x => x.key === f.key)), ...(activeOperation.value?.fields || [])])
const operations = computed(() => spec.value.operations.filter(o => can(o.action)))
const etcdDetail = computed(() => detail.value?.kvs?.length === 1 ? detail.value.kvs[0] : detail.value)
const detailFields = computed(() => {
 if(props.kind !== 'etcd' || !detail.value || typeof detail.value !== 'object')return []
 const labels={key:'Key',version:tr("版本"),create_revision:tr("创建修订"),mod_revision:tr("修改修订"),lease:tr("租约"),ID:tr("成员 ID"),name:tr("名称"),clientURLs:tr("客户端地址"),peerURLs:tr("Peer 地址"),isLearner:'Learner'}
 return Object.entries(etcdDetail.value).filter(([key]) => key !== 'value').map(([key,value]) => ({key,label:labels[key] || key,value:middlewareValue({value},'value')}))
})
const pretty = v => typeof v === 'string' ? v : JSON.stringify(v, null, 2)
let generation = 0, queryGeneration = 0
function permissionsChanged() { revision.value++;queryGeneration++;result.value=null;busy.value=false;sourceDialog.value=false;actionDialog.value=false;detailDialog.value=false;sourceGeneration++;load(true) }
onMounted(() => { window.addEventListener('ops-admin-permissions-changed', permissionsChanged); reset() })
onUnmounted(() => { generation++; queryGeneration++;sourceGeneration++; window.removeEventListener('ops-admin-permissions-changed', permissionsChanged) })
watch(() => props.kind, reset)
function reset() { sourceGeneration++;credentialLoading.value=false;credentialError.value='';listError.value='';resourceSearch.value='';busy.value = false; selected.value = null; queryGeneration++; result.value = null; error.value = ''; tab.value = spec.value.tabs[0][0]; search.value = ''; Object.keys(params).forEach(k => delete params[k]); spec.value.fields.forEach(f => { params[f.key] = f.value }); sourceDialog.value = false; actionDialog.value = false; load() }
async function load(refreshSelection = false) {
 const g = ++generation; loading.value = true; listError.value = ''
 try {
  const data = await listMiddleware(props.kind)
  if (g !== generation) return
  sources.value = data || []
  if (!selected.value && props.kind === 'kafka' && route.query.sourceId) { const source = sources.value.find(x => x.id === Number(route.query.sourceId)); if (source) enter(source) }
  if (selected.value) { selected.value = sources.value.find(x => x.id === selected.value.id) || null; if (refreshSelection && selected.value && can('query')) query() }
 } catch(e) { if(g === generation){sources.value=[];selected.value=null;listError.value=e.message || tr("数据源加载失败")} }
 finally { if(g === generation) loading.value = false }
}
async function loadCredentials(g = sourceGeneration) {
 credentialLoading.value = true; credentialError.value = ''
 try { const data = await queryAssetCredentialOptions(props.kind === 'etcd' ? 'password,tls' : ''); if(g === sourceGeneration) credentials.value=(data || []).filter(x => x.authType==='password' || (props.kind==='etcd' && x.authType==='tls')) }
 catch(e) { if(g === sourceGeneration) credentialError.value=e.message || tr("凭据加载失败") }
 finally { if(g === sourceGeneration) credentialLoading.value=false }
}
async function openSource(row) {
 const g = ++sourceGeneration; credentials.value=[];credentialError.value=''
 Object.keys(form).forEach(k => delete form[k])
 Object.assign(form,row || {id:0,name:'',type:props.kind,version:'2',credentialId:undefined,authType:props.kind==='kafka'?'plain':props.kind==='zookeeper'?'digest':'basic',tls:false,description:''})
 endpointText.value=(row?.endpoints || []).join('\n');sourceDialog.value=true
 await nextTick();sourceFormRef.value?.clearValidate();await loadCredentials(g)
}
async function validateSource() {try {await sourceFormRef.value?.validate();return true}catch{return false}}

const credentialChoices = computed(() => credentials.value.filter(c => form.authType === 'tls' ? c.authType === 'tls' : c.authType === 'password'))
function onAuthChange() { if(props.kind==='etcd')form.tls=form.authType==='tls';form.credentialId=undefined;nextTick(()=>sourceFormRef.value?.clearValidate('credentialId')) }
function payload() {
 const { endpointText: _input, ...data } = form
 const endpoints = endpointText.value.split(/[,，\n]/).map(s => s.trim()).filter(Boolean)
 if(props.kind==='etcd') {
  data.tls=form.authType==='tls' || endpoints.some(endpoint => /^https:\/\//i.test(endpoint))
  if(form.authType==='tls') endpoints.forEach((endpoint,i)=>{endpoints[i]=endpoint.replace(/^http:\/\//i,'https://')})
 }
 return {...data,endpoints,credentialId:form.authType==='none'?0:Number(form.credentialId)||0}
}
async function saveSource() { if(saving.value || !await validateSource())return;const g=sourceGeneration;saving.value=true;try {await saveMiddleware(payload());ElMessage.success(tr("数据源已保存"));if(g===sourceGeneration){sourceDialog.value=false;await load()}}finally{saving.value=false} }
async function testSource(row) { if(testing.value || (!row && !await validateSource()))return;testing.value=true;testingId.value=row?.id || 0;try {await testMiddleware(row || payload());ElMessage.success(tr("连接成功"))}finally{testing.value=false;testingId.value=null} }

async function removeSource(row) { try { await ElMessageBox.confirm(tr("删除数据源“{p0}”？远端数据不会删除。", { p0: (row.name) }), tr("删除数据源"), { type: 'warning', confirmButtonText: tr("删除") }) } catch { return }; await deleteMiddleware(row.id); ElMessage.success(tr("数据源已删除")); if (selected.value?.id === row.id) selected.value = null; await load() }
function enter(row) { if (props.kind === 'zookeeper') { router.push({ path: '/assets/middleware/zookeeper/explorer', query: { sourceId: row.id } }); return } if (props.kind === 'rabbitmq' || props.kind === 'nacos') { router.push({ path: `/assets/middleware/${props.kind}/explorer`, query: { sourceId: row.id } }); return } queryGeneration++;resourceSearch.value='';selected.value = row; result.value = null; error.value = ''; if (props.kind !== 'etcd' && can('query')) query() }
async function query() { if (!selected.value || !can('query')) return; const g = ++queryGeneration; busy.value = true; error.value = ''; try { const data = await operateMiddleware(selected.value.id, tab.value, { ...params }); if (g === queryGeneration) result.value = data } catch(e) { if (g === queryGeneration) { result.value = null; error.value = e.message || tr("查询失败，请检查连接与权限") } } finally { if (g === queryGeneration) busy.value = false } }
function changeTab() { result.value = null; queryGeneration++;busy.value=false;resourceSearch.value=''; if (props.kind === 'nacos' && tab.value === 'instances' && !params.serviceName) return; query() }
function inspect(row) { if (props.kind === 'kafka' && tab.value === 'topics') { router.push({path:'/assets/middleware/kafka/topic',query:{sourceId:selected.value.id,name:row.Name}}); return };  if (props.kind === 'zookeeper') { params.path = row.path; query(); return }; if (props.kind === 'nacos' && tab.value === 'services') { params.serviceName = row.name || row.serviceName; tab.value = 'instances'; query(); return }; detail.value = row; detailDialog.value = true }
function openOperation(op) { activeOperation.value = op; Object.keys(operationForm).forEach(k => delete operationForm[k]); actionFields.value.forEach(f => { operationForm[f.key] = params[f.key] || f.value }); actionDialog.value = true }
async function executeOperation() {
 if(actionPending.value || saving.value || !selected.value || !activeOperation.value)return
 const op = activeOperation.value, id = selected.value.id, name = selected.value.name, g = queryGeneration
 const values = { ...params, ...operationForm }
 if(op.fields.some(f => f.required && !String(values[f.key] ?? '').trim())) {ElMessage.warning(tr("请填写所有必填参数"));return}
 actionPending.value=true
 try {
  if(op.action !== 'query') {try {await ElMessageBox.confirm(tr("将在“{p0}”执行{p1}，请确认参数无误。", { p0: (name), p1: (tr(op.label)) }),tr(op.label),{type:op.action==='delete'?'warning':'info',confirmButtonText:tr("确认执行")})}catch{return}}
  if(g!==queryGeneration || selected.value?.id!==id || !can(op.action))return
  saving.value=true
  const data = await operateMiddleware(id,op.key,values,op.action,op.action!=='query')
  if(g!==queryGeneration || selected.value?.id!==id)return
  actionDialog.value=false
  if(op.action==='query'){detail.value=data;detailDialog.value=true}else{ElMessage.success(tr("操作成功"));await query()}
 }finally{actionPending.value=false;saving.value=false}
}

function up() { params.path = params.path.replace(/\/[^/]+\/?$/, '') || '/'; query() }
</script>
<template>
 <div class="middleware-page" :style="{ '--middleware-color': spec.color }">
  <header v-if="!selected || kind !== 'etcd'" class="middleware-hero"><div class="product-mark">{{ spec.mark }}</div><div><span class="eyebrow">MIDDLEWARE MANAGEMENT</span><h1>{{ spec.name }} <span>{{ tr("中间件管理") }}</span></h1><p>{{ tr(spec.description) }}</p></div><el-button v-if="isAdmin" type="primary" :icon="Plus" @click="openSource()">{{ tr("新增数据源") }}</el-button></header>
  <section v-if="!selected" class="source-section" v-loading="loading"><div class="section-toolbar"><div class="list-title"><h2>{{ tr("数据源") }}</h2><span>{{ tr("{p0} 个", { p0: (filtered.length) }) }}</span></div><el-input v-model="search" :prefix-icon="Search" :placeholder="tr('搜索名称或说明')" clearable :aria-label="tr('搜索中间件数据源')"/><el-button :icon="Refresh" @click="load">{{ tr("刷新") }}</el-button></div>
   <el-alert v-if="listError" :title="listError" type="error" :closable="false" show-icon/>
   <el-table :data="filtered" row-key="id" :empty-text="search ? tr('没有匹配的数据源') : tr('暂无可访问的数据源')" class="source-table">
    <el-table-column :label="tr('数据源名称')" min-width="180" show-overflow-tooltip><template #default="{ row }"><el-button link type="primary" @click="enter(row)">{{ row.name }}</el-button></template></el-table-column>
    <el-table-column :label="tr('类型')" width="125"><template #default>{{ spec.name }}</template></el-table-column>
    <el-table-column v-if="kind === 'nacos'" :label="tr('版本')" width="90"><template #default="{ row }">{{ row.version }}.x</template></el-table-column>
    <el-table-column :label="tr('连接地址')" min-width="230" show-overflow-tooltip><template #default="{ row }"><span class="endpoint">{{ row.endpoints?.join(' · ') || tr("连接信息由管理员维护") }}</span></template></el-table-column>
    <el-table-column v-if="kind === 'etcd'" :label="tr('认证方式')" width="150"><template #default="{ row }"><span class="auth-badge" :class="{ secured: row.tls || row.authType === 'tls' }">{{ row.authType === 'tls' ? tr("TLS 证书") : row.authType === 'basic' ? tr("用户名 / 密码") : tr("无认证") }}<em v-if="row.tls">HTTPS</em></span></template></el-table-column>
    <el-table-column :label="tr('说明')" min-width="180" show-overflow-tooltip><template #default="{ row }">{{ row.description || '—' }}</template></el-table-column>
    <el-table-column :label="tr('操作')" :width="isAdmin ? 260 : 110" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="enter(row)">{{ tr("工作台") }}</el-button><template v-if="isAdmin"><el-button link type="primary" @click="openSource(row)">{{ tr("编辑") }}</el-button><el-button link type="primary" :loading="testingId === row.id" :disabled="testing && testingId !== row.id" @click="testSource(row)">{{ tr("测试连接") }}</el-button><el-button link type="danger" @click="removeSource(row)">{{ tr("删除") }}</el-button></template></template></el-table-column>
   </el-table>
   <p class="result-note">{{ search ? tr("匹配 {p0} / {p1} 个数据源", { p0: (filtered.length), p1: (sources.length) }) : tr("共 {p0} 个数据源", { p0: (sources.length) }) }}<span v-if="!sources.length && !loading && !listError && isAdmin">{{ tr("· 点击右上方新增数据源开始接入") }}</span></p>
  </section>
  <EtcdExplorer v-else-if="kind === 'etcd'" :key="`${selected.id}-${revision}`" :source="selected" :can="can" @back="selected = null; queryGeneration++"/>
  <section v-else class="workbench"><div class="section-toolbar"><el-button :icon="ArrowLeft" @click="selected = null; queryGeneration++">{{ tr("数据源") }}</el-button><div class="workbench-title"><h2>{{ selected.name }}</h2><p>{{ spec.name }}{{ kind === 'nacos' ? ` ${selected.version}.x` : '' }}</p></div><el-dropdown v-if="operations.length" @command="openOperation"><el-button type="primary">{{ tr("管理操作 ▾") }}</el-button><template #dropdown><el-dropdown-menu><el-dropdown-item v-for="op in operations" :key="op.key" :command="op">{{ tr(op.label) }}</el-dropdown-item></el-dropdown-menu></template></el-dropdown><el-button :icon="Refresh" :disabled="!can('query')" :loading="busy" @click="query">{{ tr("刷新") }}</el-button></div>
   <el-alert v-if="!can('query')" :title="tr('当前仅有查看数据源权限，请联系管理员授权查询操作。')" type="info" :closable="false"/>
   <el-tabs v-model="tab" @tab-change="changeTab"><el-tab-pane v-for="[key,label] in spec.tabs" :key="key" :label="tr(label)" :name="key"/></el-tabs>
   <div v-if="spec.fields.length && !['overview','namespaces','members'].includes(tab)" class="query-toolbar"><el-button v-if="kind === 'zookeeper'" :disabled="params.path === '/' || busy" @click="up">{{ tr("上一级") }}</el-button><label v-for="f in spec.fields" :key="f.key">{{ tr(f.label) }}<el-input v-model="params[f.key]" :aria-label="tr(f.label)" :placeholder="f.key === 'namespaceId' ? tr('默认命名空间') : tr(f.label)" clearable @keyup.enter="query"/></label><el-button type="primary" :disabled="!can('query')" :loading="busy" @click="query">{{ tr("查询") }}</el-button></div>
   <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon/>
   <div v-loading="busy" class="results"><div v-if="tab === 'overview' && result" class="overview-grid"><article v-for="key in ['rabbitmq_version','cluster_name','node','object_totals','queue_totals','message_stats']" :key="key"><span>{{ ({rabbitmq_version:tr("RabbitMQ 版本"),cluster_name:tr("集群名称"),node:tr("节点"),object_totals:tr("资源统计"),queue_totals:tr("队列消息"),message_stats:tr("消息统计")})[key] }}</span><pre>{{ pretty(result[key] ?? '—') }}</pre></article></div>
    <template v-else><div class="resource-toolbar"><el-input v-model="resourceSearch" :prefix-icon="Search" clearable :placeholder="tr('筛选当前结果')" :aria-label="tr('筛选中间件资源')"/><span>{{ tr("{p0} 条结果", { p0: (visibleRows.length) }) }}</span></div><el-table :data="visibleRows" :empty-text="tr('暂无数据，请检查查询范围或刷新')" @row-dblclick="inspect"><el-table-column v-for="[key,label] in columns" :key="key" :label="tr(label)" min-width="145" show-overflow-tooltip><template #default="{ row }"><el-button v-if="kind === 'kafka' && tab === 'topics' && key === 'Name'" link type="primary" @click="inspect(row)">{{ row.Name }}</el-button><span v-else :class="{ 'resource-name': key === columns[0]?.[0] }">{{ (typeof row[key] === 'boolean' ? tr(row[key] ? '是' : '否') : middlewareValue(row,key)) }}</span></template></el-table-column><el-table-column :label="tr('操作')" width="100" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="inspect(row)">{{ kind === 'zookeeper' ? tr("展开") : kind === 'nacos' && tab === 'services' ? tr("查看实例") : tr("详情") }}</el-button></template></el-table-column></el-table><p class="result-note">{{ tr("{p0} 条结果", { p0: (visibleRows.length) }) }}<span v-if="kind === 'etcd'">{{ tr("· 每次最多返回 200 个 Key，请使用前缀缩小范围{p0}", { p0: (result?.more ? tr("（当前仍有更多数据）") : '') }) }}</span><span v-if="kind === 'nacos'">{{ tr("· 每页 100 条") }}</span></p><el-pagination v-if="kind === 'nacos' && ['configs','services'].includes(tab)" :current-page="Number(params.pageNo || 1)" :page-size="100" :total="Number(result?.totalCount || result?.count || 0)" layout="prev, pager, next" @current-change="p => { params.pageNo = String(p); query() }"/></template>
   </div><p v-if="kind === 'rabbitmq' && tab === 'consumers'" class="workbench-note">{{ tr("消费者随通道生命周期管理；可在管理操作中关闭对应连接（该连接上的消费者将一并断开）。") }}</p>
  </section>
  <el-dialog v-model="sourceDialog" :title="form.id ? tr('编辑数据源') : tr('新增 {p0} 数据源', { p0: (spec.name) })" width="min(640px, 94vw)" top="6vh" class="middleware-dialog" :close-on-click-modal="false"><el-form ref="sourceFormRef" :model="form" :rules="sourceRules" label-position="top" :disabled="saving || testing"><el-form-item :label="tr('数据源名称')" prop="name" required><el-input v-model="form.name" :placeholder="tr('例如：生产消息集群')" maxlength="128"/></el-form-item><el-form-item v-if="kind === 'nacos'" :label="tr('Nacos 版本')"><el-radio-group v-model="form.version"><el-radio-button value="2">2.x</el-radio-button><el-radio-button value="3">3.x</el-radio-button></el-radio-group></el-form-item><el-form-item :label="kind === 'nacos' && form.version === '3' ? tr('控制台 API 地址') : tr('连接地址')" prop="endpointText" required><el-input v-model="endpointText" type="textarea" :rows="2" :placeholder="spec.endpoint"/><p class="field-help">{{ kind === 'nacos' ? form.version === '3' ? tr("3.x 填控制台地址，默认 http://主机:8080；自定义根路径请一并填写。") : tr("2.x 默认 http://主机:8848/nacos，需包含 /nacos 根路径。") : ['kafka','zookeeper'].includes(kind) ? tr("每行一个 主机:端口，支持集群多个节点。") : kind === 'etcd' && form.authType === 'tls' ? tr("TLS 证书认证自动使用 HTTPS，CA 与客户端证书来自所选凭据。") : tr("填写 HTTP API 地址及自定义根路径，HTTPS 默认校验证书。") }}</p></el-form-item><div class="form-columns" :class="{ 'single-column': kind !== 'kafka' }"><el-form-item :label="tr('认证方式')"><el-select v-model="form.authType" @change="onAuthChange"><el-option :label="tr('无认证')" value="none"/><template v-if="kind === 'kafka'"><el-option label="SASL / PLAIN" value="plain"/><el-option label="SCRAM-SHA-256" value="scram256"/><el-option label="SCRAM-SHA-512" value="scram512"/></template><template v-else-if="kind === 'etcd'"><el-option :label="tr('用户名 / 密码')" value="basic"/><el-option :label="tr('TLS 证书（mTLS）')" value="tls"/></template><el-option v-else :label="kind === 'zookeeper' ? 'Digest' : tr('用户名 / 密码')" :value="kind === 'zookeeper' ? 'digest' : 'basic'"/></el-select></el-form-item><el-form-item v-if="kind === 'kafka'" :label="tr('传输加密')"><div class="tls-control"><el-switch v-model="form.tls" active-text="TLS"/><span class="field-help">{{ tlsHint }}</span></div></el-form-item></div><el-form-item v-if="form.authType !== 'none'" :label="tr('认证凭据')" prop="credentialId" required><el-select v-model="form.credentialId" :loading="credentialLoading" filterable :placeholder="form.authType === 'tls' ? tr('选择凭据管理中的 TLS 证书凭据') : tr('选择凭据管理中的用户名密码凭据')"><el-option v-for="c in credentialChoices" :key="c.id" :label="`${c.name}${c.authType === 'tls' ? ' · TLS' : c.username ? ' · ' + c.username : ''}`" :value="c.id"/></el-select><p class="field-help">{{ form.authType === 'tls' ? tr("证书与私钥在凭据管理中维护，工作台不会读取或展示内容。") : tr("凭据由凭据管理维护，工作台不展示密码。") }}</p><p v-if="credentialError" class="credential-error">{{credentialError}} <el-button link type="primary" @click="loadCredentials()">{{ tr("重新加载") }}</el-button></p><p v-else-if="!credentialLoading && !credentialChoices.length" class="field-help">{{ tr("暂无可用凭据，请先在凭据管理中添加。") }}</p></el-form-item><el-form-item :label="tr('说明')"><el-input v-model="form.description" type="textarea" :rows="2" :placeholder="tr('用途、环境或维护说明')" maxlength="500"/></el-form-item></el-form><template #footer><el-button :loading="testing" :disabled="saving || (credentialLoading && form.authType !== 'none')" @click="testSource()">{{ tr("测试连接") }}</el-button><el-button :disabled="saving || testing" @click="sourceDialog = false">{{ tr("取消") }}</el-button><el-button type="primary" :loading="saving" :disabled="testing || (credentialLoading && form.authType !== 'none')" @click="saveSource">{{ tr("保存数据源") }}</el-button></template></el-dialog>
  <el-dialog v-model="actionDialog" :title="tr(activeOperation?.label)" width="min(640px,94vw)" :close-on-click-modal="false" class="middleware-dialog"><p class="field-help">{{ tr("目标数据源：{p0}", { p0: (selected?.name) }) }}</p><el-form label-position="top" :disabled="saving"><el-form-item v-for="f in actionFields" :key="f.key" :label="tr(f.label)" :required="f.required"><el-input v-model="operationForm[f.key]" :type="f.multiline ? 'textarea' : 'text'" :rows="f.multiline ? 8 : undefined" :placeholder="tr(f.label)"/></el-form-item></el-form><template #footer><el-button :disabled="saving" @click="actionDialog = false">{{ tr("取消") }}</el-button><el-button type="primary" :loading="saving" @click="executeOperation">{{ activeOperation?.action === 'query' ? tr("读取") : tr("确认操作") }}</el-button></template></el-dialog>
  <el-dialog v-model="detailDialog" :title="kind === 'etcd' ? tr('etcd 资源详情') : tr('资源详情')" width="min(840px,94vw)" class="middleware-dialog"><template v-if="detailFields.length"><el-descriptions :column="1" border><el-descriptions-item v-for="field in detailFields" :key="field.key" :label="field.label">{{field.value}}</el-descriptions-item></el-descriptions><template v-if="Object.hasOwn(etcdDetail || {}, 'value')"><h3 class="detail-label">Value</h3><pre class="detail-content">{{ etcdDetail.value }}</pre></template><el-collapse class="raw-detail"><el-collapse-item :title="tr('原始数据')" name="raw"><pre class="detail-content">{{pretty(detail)}}</pre></el-collapse-item></el-collapse></template><pre v-else class="detail-content">{{ pretty(detail) }}</pre></el-dialog>
 </div>
</template>
<style scoped>
.middleware-page{min-width:0;display:grid;gap:16px;color:var(--el-text-color-primary,#243451)}.middleware-hero,.source-section,.workbench{background:var(--el-bg-color,#fff);border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:10px}.middleware-hero{display:flex;align-items:center;gap:16px;padding:20px 24px;border-top:2px solid var(--middleware-color)}.product-mark{display:grid;place-items:center;width:48px;height:48px;border-radius:10px;background:color-mix(in srgb,var(--middleware-color) 12%,white);color:var(--middleware-color);font-size:24px;font-weight:750}.eyebrow{font-size:11px;letter-spacing:1.5px;color:var(--middleware-color);font-weight:750}h1{font-size:23px;margin:5px 0}h1 span{font-size:14px;font-weight:400;color:#8896ad;margin-left:8px}.middleware-hero p,.section-toolbar p{margin:6px 0 0;color:#8795aa;font-size:13px}.middleware-hero>.el-button{margin-left:auto}.source-section,.workbench{padding:24px;min-width:0}.section-toolbar{display:flex;align-items:center;gap:12px;margin-bottom:18px}.section-toolbar h2{margin:0;font-size:17px}.section-toolbar>.el-input{max-width:280px;margin-left:auto}.list-title{display:flex;align-items:center;gap:10px}.list-title>span{font-size:12px;color:var(--el-text-color-secondary);background:var(--el-fill-color-light);padding:3px 8px;border-radius:4px}.resource-toolbar{display:flex;align-items:center;gap:16px;margin:0 0 14px}.resource-toolbar>.el-input{max-width:320px}.resource-toolbar>span{margin-left:auto;color:var(--el-text-color-secondary);font-size:12px}.credential-error{font-size:12px;color:var(--el-color-danger);margin:8px 0 0}.source-table .endpoint{font-family:monospace;font-size:12px;color:#7889a0}.auth-badge{display:inline-flex;align-items:center;gap:6px;font-size:12px;color:#68788f}.auth-badge.secured{color:#1f7a63}.auth-badge em{font-style:normal;font-size:10px;padding:2px 6px;border-radius:4px;background:#eaf7f1;color:#1f7a63}.workbench-title{margin-right:auto}.query-toolbar{display:flex;align-items:flex-end;gap:12px;flex-wrap:wrap;margin:6px 0 20px}.query-toolbar label{display:grid;gap:8px;font-size:12px;color:#76859c;min-width:150px;flex:1;max-width:240px}.results{min-height:260px}.resource-name{font-weight:550;color:#3b68b0}.result-note,.workbench-note,.field-help{font-size:12px;color:#8795aa;line-height:1.7}.field-help{margin:8px 0 0}.tls-control{display:grid;gap:2px}.tls-control .field-help{margin:0}.workbench-note{padding:12px 16px;background:#f6f8fc;border-radius:7px}.overview-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:16px}.overview-grid article{border:1px solid #e6ebf3;border-radius:10px;padding:18px}.overview-grid span{font-size:13px;color:#8795aa}.overview-grid pre{font-size:13px;white-space:pre-wrap;overflow-wrap:anywhere}.detail-label{font-size:14px;margin:20px 0 10px}.raw-detail{margin-top:16px}.detail-content{padding:18px;background:#f6f8fc;border-radius:8px;max-height:60vh;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;line-height:1.65}.form-columns{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:20px}.form-columns.single-column{grid-template-columns:1fr}.form-columns>*{flex:1}:deep(.el-form-item .el-select){width:100%}:global(.middleware-dialog){max-height:90vh;display:flex;flex-direction:column}:global(.middleware-dialog .el-dialog__body){overflow:auto;min-height:0}:global(.middleware-dialog .el-dialog__footer){border-top:1px solid #edf1f7;padding-top:18px}@media(max-width:900px){.middleware-hero{padding:20px;flex-wrap:wrap}.middleware-hero>.el-button{margin-left:0}.source-section,.workbench{padding:16px}.section-toolbar{flex-wrap:wrap}.section-toolbar>.el-input{margin-left:0;max-width:none;flex:1;min-width:180px}.form-columns{grid-template-columns:1fr;gap:0}}
</style>
