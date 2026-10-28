<script setup>
import { Plus, Clock, Connection, ArrowRight } from '@element-plus/icons-vue'
import { nextTick, ref, watch } from 'vue'
import { useNodeAdd } from './useNodeAdd'
import NodeAssetPicker from './NodeAssetPicker.vue'
import NodeAddProgress from './NodeAddProgress.vue'
import { nodeAddStatus, nodeAddTone } from '../../../../utils/k8s-node-add'
import './node-add.css'
const props = defineProps({ cluster: { type: Object, required: true } })
const emit = defineEmits(['completed'])
const { state: s, running, validCheck, open, pick, check, next, start, showTask, cancel, retryFailed, newDraft } = useNodeAdd(props.cluster.id, () => emit('completed'))
const steps = [{ title: '选择服务器', hint: '从 CMDB 关联资产' }, { title: '节点配置', hint: '继承集群配置' }, { title: '环境预检', hint: '逐台检查可用性' }, { title: '信息确认', hint: '确认并加入集群' }]
const body = ref(null)
watch(() => [s.step, s.view], async () => { await nextTick(); body.value?.parentElement?.scrollTo({ top: 0 }) })
function checkedOS(row) { return s.check?.nodes?.find(node => node.hostId === row.hostId)?.os || row.os || '待识别' }
function remove(id) { s.draft.nodes = s.draft.nodes.filter(n => n.hostId !== id) }
</script>
<template>
  <div class="node-add-actions"><el-button :icon="Clock" text @click="open('history')">添加任务</el-button><el-button type="primary" :icon="Plus" @click="open()">添加节点</el-button></div>
  <el-drawer v-model="s.visible" class="node-add-drawer" size="min(1080px, 100vw)" :close-on-click-modal="false" :close-on-press-escape="!s.busy" :show-close="!s.busy" :destroy-on-close="false">
    <template #header><div class="na-drawer-title"><div class="na-title-icon"><el-icon><Connection /></el-icon></div><div><h2>添加节点</h2><p>从已有服务器扩展集群容量</p></div><el-tag effect="plain">Worker 扩容</el-tag></div></template>
    <div ref="body" v-loading="s.busy" class="na-body">
      <div class="na-cluster-strip"><div><span>目标集群</span><strong>{{ s.context?.name || cluster.name }}</strong></div><div><span>Kubernetes</span><strong class="na-mono">{{ s.context?.version || cluster.version }}</strong></div><div><span>当前节点</span><strong>{{ s.context?.nodeCount ?? cluster.nodeCount }} <em>个</em></strong></div><div class="na-strip-end"><span>执行方式</span><strong>SSH · kubeadm</strong></div></div>
      <div v-if="s.error" class="na-error" role="alert">{{ s.error }}</div>
      <el-alert v-if="s.context && !s.context.canAdd" :title="s.context.blockedReason" type="warning" :closable="false" show-icon />
      <template v-if="s.view === 'wizard' && s.context?.canAdd">
        <nav class="na-stepper" aria-label="添加节点步骤"><div v-for="(item, index) in steps" :key="item.title" :class="{ active: s.step === index, done: s.step > index }" :aria-current="s.step === index ? 'step' : undefined"><i>{{ s.step > index ? '✓' : index + 1 }}</i><div><b>{{ item.title }}</b><small>{{ item.hint }}</small></div><el-icon v-if="index < 3"><ArrowRight /></el-icon></div></nav>
        <section v-if="s.step === 0" class="na-panel">
          <div class="na-section-head"><div><h3>选择要加入的服务器</h3><p>系统、配置与认证从资产自动带入，节点角色为 Worker。</p></div><el-tag effect="plain">{{ s.draft.nodes.length }} 台已选</el-tag></div>
          <NodeAssetPicker :cluster-id="cluster.id" :selected="s.draft.nodes" :environments="s.context.environments" :groups="s.context.groups" @pick="pick" @remove="remove" />
          <div v-if="s.draft.nodes.length" class="na-selection"><el-tag v-for="node in s.draft.nodes" :key="node.hostId" closable @close="remove(node.hostId)">{{ node.hostName }}</el-tag></div>
        </section>
        <section v-if="s.step === 1" class="na-panel">
          <div class="na-section-head"><div><h3>节点配置</h3><p>加入现有集群，版本、运行时和网络沿用集群配置。</p></div><el-tag effect="plain">Worker</el-tag></div>
          <div class="na-inherited"><div><span>容器运行时</span><b>{{ s.context.runtime }}</b></div><div><span>容器网络</span><b>{{ s.context.network }}</b></div><div><span>Pod CIDR</span><b class="na-mono">{{ s.context.podCIDR }}</b></div><div><span>Service CIDR</span><b class="na-mono">{{ s.context.serviceCIDR }}</b></div></div>
          <el-table :data="s.draft.nodes" class="na-assets"><el-table-column label="服务器 / 系统" min-width="250"><template #default="{ row }"><b>{{ row.hostName }}</b><small>{{ row.privateIp || row.sshIp || row.publicIp }} · {{ row.os || '待识别' }}</small></template></el-table-column><el-table-column label="节点名称" min-width="260"><template #default="{ row }"><el-input v-model="row.nodeName" :aria-label="`${row.hostName} 的节点名称`" maxlength="63" /></template></el-table-column><el-table-column width="80"><template #default="{ row }"><el-button text type="danger" @click="remove(row.hostId)">移除</el-button></template></el-table-column></el-table>
          <div class="na-config-grid"><label>节点标签 <span>可选，每行一条</span><el-input v-model="s.draft.labels" type="textarea" :rows="3" placeholder="例如：workload=batch" aria-label="节点标签" /></label><label>节点污点 <span>可选，每行一条</span><el-input v-model="s.draft.taints" type="textarea" :rows="3" placeholder="例如：dedicated=batch:NoSchedule" aria-label="节点污点" /></label></div>
          <p class="na-muted">节点网卡根据部署 IP 自动识别；系统兼容性会在预检时从服务器重新核验。</p>
          <div class="na-config-grid na-system-options">
            <div><label>安装时关闭 SELinux</label><el-switch v-model="s.draft.disableSELinux" aria-label="安装时关闭 SELinux" /><p>确认添加后，目标服务器已启用的 SELinux 将被关闭；预检阶段不调整。</p></div>
            <div><label>安装时关闭 Firewalld</label><el-switch v-model="s.draft.disableFirewalld" aria-label="安装时关闭 Firewalld" /><p>默认保留；CentOS 防火墙正在运行时，请核对网络要求后明确选择。</p></div>
          </div>
        </section>
        <section v-if="s.step === 2" class="na-panel">
          <div class="na-section-head"><div><h3>环境预检</h3><p>先自动补齐 Python、网络工具、证书和时间同步组件，再通过 SSH 检查服务器。</p></div><el-button type="primary" plain :loading="s.busy || s.checking" @click="check">{{ s.checking ? '检查中' : s.check ? '重新预检' : '开始预检' }}</el-button></div>
          <div v-if="!s.check" class="na-check-empty"><el-icon><Connection /></el-icon><h4>准备检查 {{ s.draft.nodes.length }} 台服务器</h4><p>基础依赖 · 内核与 cgroup · 资源 · 网络 · 集群可达性</p><small>系统与 Kubernetes 软件包源默认使用阿里云；此阶段不安装 Kubernetes 或执行 join</small><p>CentOS 7.9 需 x86_64，运行 5.10/5.15 或 6.x 内核；不会自动升级内核或重启。</p></div>
          <template v-else><el-alert :type="s.checking ? 'info' : validCheck ? 'success' : 'error'" :title="s.checking ? '正在检查服务器，请等待结果' : validCheck ? '预检通过，可以进入信息确认' : s.check.error || '预检未通过或已过期，请重新检查'" show-icon :closable="false" /><div v-for="node in s.check.nodes" :key="node.hostId" class="na-check-node"><div><div class="na-check-identity"><b>{{ node.nodeName }}</b><small v-if="node.os">{{ node.os }} · {{ node.arch }} · {{ node.interface }} · {{ node.kernel }} / {{ node.cgroup }}</small></div><el-tag :type="s.checking ? 'info' : node.passed ? 'success' : 'danger'" effect="plain">{{ s.checking ? '检查中' : node.passed ? '通过' : '未通过' }}</el-tag></div><ul><li v-for="item in node.checks" :key="item.title" :class="item.status"><i>{{ item.status === 'failed' ? '!' : item.status === 'warning' ? '·' : item.status === 'running' ? '…' : '✓' }}</i><span>{{ item.title }}</span><small>{{ item.message }}</small></li></ul></div><p class="na-muted">预检有效期 5 分钟；执行前会再次核对集群和主机身份。</p></template>
        </section>
        <section v-if="s.step === 3" class="na-panel">
          <div class="na-section-head"><div><h3>确认添加信息</h3><p>确认后将在所选服务器上安装组件，并加入当前集群。</p></div><el-tag type="success">预检通过</el-tag></div>
          <div class="na-capacity"><div><span>当前节点</span><b>{{ s.context.nodeCount }}</b></div><el-icon><Plus /></el-icon><div><span>新增 Worker</span><b>{{ s.draft.nodes.length }}</b></div><el-icon><ArrowRight /></el-icon><div><span>全部成功后</span><b>{{ s.context.nodeCount + s.draft.nodes.length }}</b></div></div>
          <template v-for="node in s.check?.nodes || []" :key="node.hostId"><el-alert v-for="warning in node.checks.filter(c => c.status === 'warning')" :key="warning.title" :title="`${node.nodeName}：${warning.message}`" type="warning" :closable="false" show-icon style="margin-bottom:12px" /></template>
          <el-table :data="s.draft.nodes" class="na-assets"><el-table-column label="节点名称" prop="nodeName" min-width="170" /><el-table-column label="IP" prop="privateIp" min-width="150" /><el-table-column label="系统" min-width="210"><template #default="{ row }">{{ checkedOS(row) }}</template></el-table-column><el-table-column label="角色" width="100"><template #default>Worker</template></el-table-column></el-table>
          <dl class="na-confirm-meta"><dt>集群配置</dt><dd>{{ s.context.version }} · {{ s.context.runtime }} · {{ s.context.network }}</dd><dt>标签</dt><dd class="na-mono">{{ s.draft.labels || '未设置' }}</dd><dt>污点</dt><dd class="na-mono">{{ s.draft.taints || '未设置' }}</dd><dt>关闭 SELinux</dt><dd>{{ s.draft.disableSELinux ? '正式安装时关闭' : '保留' }}</dd><dt>关闭 Firewalld</dt><dd>{{ s.draft.disableFirewalld ? '正式安装时关闭' : '保留' }}</dd><dt>软件包源</dt><dd>阿里云系统源及 Kubernetes 包源</dd><dt>API 入口</dt><dd class="na-mono">{{ s.context.endpoint || cluster.apiServer }}</dd></dl>
        </section>
      </template>
      <section v-if="s.view === 'history'" class="na-panel"><div class="na-section-head"><div><h3>添加任务</h3><p>查看节点添加记录、逐节点结果和执行日志。</p></div><el-button :disabled="!s.context?.canAdd" @click="newDraft">新建添加任务</el-button></div><el-empty v-if="!s.history.length" description="还没有添加任务" /><el-table v-else :data="s.history" class="na-assets"><el-table-column label="任务" width="100"><template #default="{ row }">#{{ row.id }}</template></el-table-column><el-table-column label="类型" width="85"><template #default="{ row }">{{ row.kind === 'precheck' ? '预检' : '添加' }}</template></el-table-column><el-table-column label="服务器" min-width="140"><template #default="{ row }">{{ row.nodes.length }} 台 Worker</template></el-table-column><el-table-column label="创建时间" min-width="180"><template #default="{ row }">{{ new Date(row.createdAt).toLocaleString('zh-CN', { hour12: false }) }}</template></el-table-column><el-table-column label="状态" width="120"><template #default="{ row }"><el-tag :type="nodeAddTone(row.status)">{{ nodeAddStatus[row.status] }}</el-tag></template></el-table-column><el-table-column width="100"><template #default="{ row }"><el-button text type="primary" @click="showTask(row)">查看</el-button></template></el-table-column></el-table></section>
      <NodeAddProgress v-if="s.view === 'progress' && s.task" :task="s.task" />
      <p class="na-isolation-note">沿用现有集群网络 · 不重新初始化控制平面 · 失败后保留现场供核对</p>
    </div>
    <template #footer><div class="na-footer"><span>{{ s.view === 'wizard' ? `步骤 ${s.step + 1} / 4` : '关闭后可从「添加任务」继续查看' }}</span><div><el-button :disabled="s.busy" @click="s.visible = false">{{ running ? '稍后查看' : '关闭' }}</el-button><template v-if="s.view === 'wizard' && s.context?.canAdd"><el-button v-if="s.step > 0" :disabled="s.busy || s.checking" @click="s.step--; s.error = ''">上一步</el-button><el-button v-if="s.step < 3" type="primary" :disabled="s.busy || s.checking || (s.step === 0 && !s.draft.nodes.length) || (s.step === 2 && !validCheck)" @click="next">下一步</el-button><el-button v-else type="primary" :loading="s.busy" @click="start">确认并添加</el-button></template><template v-if="s.view === 'progress'"><el-button v-if="running" type="danger" plain :loading="s.busy" :disabled="s.task?.stopRequested" @click="cancel">停止后续步骤</el-button><template v-else><el-button @click="s.view = 'history'">任务记录</el-button><el-button v-if="s.task?.nodes.some(n => n.retryable !== false && n.status !== 'success')" type="primary" @click="retryFailed">重试未成功节点</el-button><el-button v-else type="primary" @click="newDraft">继续添加</el-button></template></template></div></div></template>
  </el-drawer>
</template>
