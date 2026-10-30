<script setup>
import { computed } from 'vue'
import { tr } from '../../../utils/i18n'
import Rows from './K8sObjectRows.vue'
import JsonField from './K8sJsonField.vue'
const props = defineProps({ container: Object, volumes: Array, configMaps: Array, secrets: Array, disabled: Boolean, init: Boolean })
const emit = defineEmits(['validity'])
const c = computed(() => props.container)
const references = type => (type === 'secretKeyRef' || type === 'secretRef' ? props.secrets : props.configMaps) || []
const refType = env => Object.keys(env.valueFrom || {})[0] || 'value'
function setEnvType(env, type) { delete env.value; delete env.valueFrom; if (type === 'value') env.value = ''; else env.valueFrom = {[type]:type === 'fieldRef' ? {fieldPath:'metadata.name'} : type === 'resourceFieldRef' ? {resource:'limits.cpu'} : {name:'',key:''}} }
const probeTypes = ['httpGet','tcpSocket','exec','grpc']
function setProbe(key, type) {
  if (!type) { delete c.value[key]; return }
  const previous = c.value[key] || {}
  const next = {...previous}
  probeTypes.forEach(k => delete next[k])
  next[type] = type === 'httpGet' ? {path:'/',port:8080,scheme:'HTTP'} : type === 'tcpSocket' ? {port:8080} : type === 'grpc' ? {port:8080} : {command:['/bin/sh','-c','true']}
  c.value[key] = {...next,periodSeconds:previous.periodSeconds ?? 10, timeoutSeconds:previous.timeoutSeconds ?? 1, failureThreshold:previous.failureThreshold ?? 3}
}
function hook(name, text) { c.value.lifecycle ||= {}; if (!text.trim()) delete c.value.lifecycle[name]; else c.value.lifecycle[name] = {exec:{command:text.split('\n')}} }
function setCommand(field, text) { if (!text.trim()) delete c.value[field]; else c.value[field] = text.split('\n') }
function setRef(env, type, name) { env[type] = {name} }
</script>
<template>
  <div class="kwe-container">
    <el-form label-position="top" :disabled="disabled">
      <div class="kwe-grid"><el-form-item :label="tr('容器名称')" required><el-input v-model="c.name" /></el-form-item><el-form-item :label="tr('镜像')" required><el-input v-model="c.image" placeholder="registry.example.com/app:1.0" /></el-form-item><el-form-item :label="tr('拉取策略')"><el-select v-model="c.imagePullPolicy"><el-option v-for="policy in ['Always','IfNotPresent','Never']" :key="policy" :value="policy" /></el-select></el-form-item><el-form-item :label="tr('工作目录')"><el-input v-model="c.workingDir" placeholder="/app" /></el-form-item></div>
      <div class="kwe-grid"><el-form-item :label="tr('启动命令（每行一个参数）')"><el-input :model-value="c.command?.join('\n')" type="textarea" :rows="3" @input="setCommand('command', $event)" /></el-form-item><el-form-item :label="tr('启动参数（每行一个参数）')"><el-input :model-value="c.args?.join('\n')" type="textarea" :rows="3" @input="setCommand('args', $event)" /></el-form-item></div>
      <div class="kwe-grid four"><el-form-item v-for="quantity in [{group:'requests',key:'cpu',label:'CPU request',hint:'100m'},{group:'limits',key:'cpu',label:'CPU limit',hint:'500m'},{group:'requests',key:'memory',label:'Memory request',hint:'128Mi'},{group:'limits',key:'memory',label:'Memory limit',hint:'512Mi'}]" :key="quantity.label" :label="quantity.label"><el-input v-model="c.resources[quantity.group][quantity.key]" :placeholder="quantity.hint" /></el-form-item></div>
      <el-collapse>
        <el-collapse-item :title="tr('容器端口')" name="ports"><Rows v-model="c.ports" :columns="[{key:'name',label:'名称'},{key:'containerPort',label:'容器端口',type:'number',min:1,max:65535},{key:'protocol',label:'协议',options:['TCP','UDP','SCTP']}]" :create="() => ({name:'',containerPort:8080,protocol:'TCP'})" :disabled="disabled" /></el-collapse-item>
        <el-collapse-item :title="tr('环境变量')" name="env">
          <div v-for="(env, i) in c.env" :key="i" class="kwe-reference"><el-input v-model="env.name" :placeholder="tr('变量名')" /><el-select :model-value="refType(env)" @change="setEnvType(env,$event)"><el-option v-for="type in ['value','configMapKeyRef','secretKeyRef','fieldRef','resourceFieldRef']" :key="type" :value="type" :label="type === 'value' ? tr('普通变量') : type" /></el-select><template v-if="refType(env) === 'value'"><el-input v-model="env.value" :placeholder="tr('变量值')" /></template><template v-else-if="['configMapKeyRef','secretKeyRef'].includes(refType(env))"><el-select v-model="env.valueFrom[refType(env)].name" filterable><el-option v-for="ref in references(refType(env))" :key="ref.name" :value="ref.name" /></el-select><el-input v-model="env.valueFrom[refType(env)].key" placeholder="key" /></template><el-input v-else-if="refType(env) === 'fieldRef'" v-model="env.valueFrom.fieldRef.fieldPath" placeholder="metadata.name" /><template v-else><el-input v-model="env.valueFrom.resourceFieldRef.resource" placeholder="limits.cpu" /><el-input v-model="env.valueFrom.resourceFieldRef.divisor" placeholder="1m" /></template><el-button link type="danger" @click="c.env.splice(i,1)">{{ tr('删除') }}</el-button></div>
          <el-button plain size="small" @click="(c.env ||= []).push({name:'',value:''})">+ {{ tr('环境变量') }}</el-button>
          <h4>envFrom · {{ tr('整组导入') }}</h4><div v-for="(env,i) in c.envFrom" :key="i" class="kwe-reference"><el-input v-model="env.prefix" :placeholder="tr('前缀（可选）')" /><el-select :model-value="env.secretRef ? 'secretRef' : 'configMapRef'" @change="value => { delete env.secretRef; delete env.configMapRef; setRef(env,value,'') }"><el-option value="configMapRef" label="ConfigMap" /><el-option value="secretRef" label="Secret" /></el-select><el-select v-model="env[env.secretRef ? 'secretRef' : 'configMapRef'].name" filterable><el-option v-for="ref in references(env.secretRef ? 'secretRef' : 'configMapRef')" :key="ref.name" :value="ref.name" /></el-select><el-button link type="danger" @click="c.envFrom.splice(i,1)">{{ tr('删除') }}</el-button></div><el-button plain size="small" @click="(c.envFrom ||= []).push({configMapRef:{name:''}})">+ envFrom</el-button>
          <p class="kwe-hint">{{ tr('Secret 仅引用名称与键，不读取或展示明文') }}</p>
        </el-collapse-item>
        <el-collapse-item v-if="!init" :title="tr('健康检查')" name="probes">
          <div v-for="probe in [{key:'startupProbe',label:'启动检查'},{key:'readinessProbe',label:'就绪检查'},{key:'livenessProbe',label:'存活检查'}]" :key="probe.key" class="kwe-probe"><div class="kwe-probe-title"><strong>{{ tr(probe.label) }}</strong><el-select :model-value="probeTypes.find(type => c[probe.key]?.[type]) || ''" :placeholder="tr('不启用')" @change="setProbe(probe.key,$event)"><el-option value="" :label="tr('不启用')" /><el-option v-for="type in probeTypes" :key="type" :value="type" /></el-select></div><template v-if="c[probe.key]">
            <div v-if="c[probe.key].httpGet" class="kwe-grid"><el-input v-model="c[probe.key].httpGet.path" :placeholder="tr('路径')" /><el-input v-model="c[probe.key].httpGet.port" :placeholder="tr('端口或端口名称')" /><el-select v-model="c[probe.key].httpGet.scheme"><el-option value="HTTP" /><el-option value="HTTPS" /></el-select></div>
            <el-input v-if="c[probe.key].tcpSocket" v-model="c[probe.key].tcpSocket.port" :placeholder="tr('端口或端口名称')" />
            <template v-if="c[probe.key].grpc"><el-input-number v-model="c[probe.key].grpc.port" :min="1" :max="65535" /><el-input v-model="c[probe.key].grpc.service" placeholder="gRPC service" /></template>
            <el-input v-if="c[probe.key].exec" :model-value="c[probe.key].exec.command?.join('\n')" type="textarea" :placeholder="tr('每行一个命令参数')" @input="c[probe.key].exec.command = $event.split('\n')" />
            <div class="kwe-grid four"><el-form-item v-for="field in [{key:'initialDelaySeconds',label:'初始延迟（秒）',min:0},{key:'periodSeconds',label:'检查间隔（秒）',min:1},{key:'timeoutSeconds',label:'超时（秒）',min:1},{key:'failureThreshold',label:'失败阈值',min:1},{key:'successThreshold',label:'成功阈值',min:1}]" :key="field.key" :label="tr(field.label)"><el-input-number v-model="c[probe.key][field.key]" :min="field.min" :max="field.key === 'successThreshold' && probe.key !== 'readinessProbe' ? 1 : undefined" /></el-form-item></div>
            <JsonField v-if="c[probe.key].httpGet" v-model="c[probe.key].httpGet.httpHeaders" array :label="tr('HTTP 请求头（JSON）')" @validity="emit('validity', `${probe.key}-headers`, $event)" />
          </template></div><p class="kwe-hint">{{ tr('启动检查保护启动过程；就绪检查决定是否接收流量；存活检查失败会重启容器') }}</p>
        </el-collapse-item>
        <el-collapse-item :title="tr('存储挂载')" name="mounts"><Rows v-model="c.volumeMounts" :columns="[{key:'name',label:'存储卷',options:(volumes || []).map(v => v.name)},{key:'mountPath',label:'挂载路径'},{key:'subPath',label:'子路径'},{key:'readOnly',label:'只读',type:'boolean'}]" :create="() => ({name:'',mountPath:'',readOnly:false})" :disabled="disabled" /></el-collapse-item>
        <el-collapse-item v-if="!init" :title="tr('生命周期')" name="lifecycle"><el-form-item v-for="name in ['postStart','preStop']" :key="name" :label="`${name} · Exec`"><el-input :model-value="c.lifecycle?.[name]?.exec?.command?.join('\n')" type="textarea" :placeholder="tr('每行一个命令参数')" @input="hook(name,$event)" /></el-form-item><JsonField v-model="c.lifecycle" :label="tr('完整生命周期配置（JSON）')" @validity="emit('validity', 'lifecycle', $event)" /></el-collapse-item>
        <el-collapse-item :title="tr('安全配置')" name="security"><el-button v-if="!c.securityContext" size="small" @click="c.securityContext = {}">{{ tr('配置') }}</el-button><template v-else><div class="kwe-grid"><el-form-item v-for="key in ['runAsUser','runAsGroup']" :key="key" :label="key"><el-input-number v-model="c.securityContext[key]" :min="0" /></el-form-item><el-form-item v-for="key in ['runAsNonRoot','readOnlyRootFilesystem','allowPrivilegeEscalation','privileged']" :key="key" :label="key"><el-switch v-model="c.securityContext[key]" /></el-form-item></div><JsonField v-model="c.securityContext" :label="tr('完整安全配置（JSON）')" @validity="emit('validity', 'security', $event)" /></template></el-collapse-item>
      </el-collapse>
    </el-form>
  </div>
</template>
<style scoped>.kwe-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:0 18px}.kwe-grid.four{grid-template-columns:repeat(4,minmax(0,1fr))}.kwe-grid :deep(.el-select),.kwe-grid :deep(.el-input-number){width:100%}.kwe-reference{display:flex;gap:8px;margin:8px 0;align-items:center;flex-wrap:wrap}.kwe-reference>.el-input,.kwe-reference>.el-select{flex:1;min-width:120px}.kwe-probe{padding:16px;border:1px solid #e5eaf1;border-radius:10px;margin-bottom:12px}.kwe-probe-title{display:flex;align-items:center;gap:20px;margin-bottom:14px}.kwe-probe-title .el-select{width:180px}.kwe-hint{color:#8190a6;font-size:12px;line-height:1.6}.kwe-container :deep(.el-collapse-item__header){font-weight:600;color:#334a66}.kwe-container h4{margin:22px 0 10px}@media(max-width:1100px){.kwe-grid.four{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:650px){.kwe-grid{grid-template-columns:1fr}}</style>
