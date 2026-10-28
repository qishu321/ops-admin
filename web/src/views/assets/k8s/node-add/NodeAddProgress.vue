<script setup>
import { nodeAddStages, nodeAddStatus, nodeAddTone } from '../../../../utils/k8s-node-add'
defineProps({ task: { type: Object, required: true } })
const summary = task => `${task.nodes.filter(n => n.status === 'success').length} 成功 / ${task.nodes.filter(n => ['failed', 'needs_review'].includes(n.status)).length} 失败或待核对 / ${task.nodes.filter(n => n.status === 'cancelled').length} 取消`
</script>
<template>
  <div class="na-task-heading"><div><span class="na-eyebrow">添加任务 #{{ task.id }}</span><h3>{{ nodeAddStatus[task.status] }}</h3><p>{{ summary(task) }} · 目标集群 {{ task.clusterName }}</p></div><el-tag :type="nodeAddTone(task.status)" effect="light" size="large">{{ nodeAddStatus[task.status] }}</el-tag></div>
  <el-alert v-if="task.error || task.stopRequested" :title="task.error || '已请求停止；当前命令结束后停止后续步骤，已完成的系统变更保留。'" type="warning" :closable="false" show-icon />
  <div class="na-stage-track"><div v-for="(title, i) in nodeAddStages" :key="title" :class="{ done: task.stage > i || task.status === 'success', active: task.stage === i && task.status === 'running' }"><i>{{ task.stage > i || task.status === 'success' ? '✓' : i + 1 }}</i><span>{{ title }}</span></div></div>
  <el-table :data="task.nodes" class="na-assets"><el-table-column label="节点" min-width="185"><template #default="{ row }"><b>{{ row.nodeName }}</b><small>{{ row.host?.privateIp || row.host?.sshIp }}</small></template></el-table-column><el-table-column label="当前阶段" min-width="130"><template #default="{ row }">{{ nodeAddStages[row.stage] }}</template></el-table-column><el-table-column label="结果" width="110"><template #default="{ row }"><el-tag :type="nodeAddTone(row.status)" effect="plain">{{ nodeAddStatus[row.status] }}</el-tag></template></el-table-column><el-table-column label="说明" prop="message" min-width="230" /></el-table>
  <div class="na-log-head"><h4>执行日志</h4><span>敏感加入凭据不会写入日志</span></div>
  <div class="na-terminal" role="log" aria-label="添加节点日志" tabindex="0"><div v-for="(line, i) in task.logs" :key="i"><span>{{ String(i + 1).padStart(2, '0') }}</span><code>{{ line }}</code></div><p v-if="!task.logs.length">等待执行…</p></div>
</template>
