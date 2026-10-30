<script setup>
import { computed } from 'vue'
import { tr } from '../../../utils/i18n'
import JsonField from './K8sJsonField.vue'
import Rows from './K8sObjectRows.vue'
const props = defineProps({ modelValue:Object, disabled:Boolean })
const emit = defineEmits(['update:modelValue','validity'])
const affinity = computed(() => props.modelValue || {})
function add(kind, preferred) {
  const next = props.modelValue || {}
  next[kind] ||= {}
  const key = preferred ? 'preferredDuringSchedulingIgnoredDuringExecution' : 'requiredDuringSchedulingIgnoredDuringExecution'
  if (kind === 'nodeAffinity') {
    if (preferred) (next[kind][key] ||= []).push({weight:50,preference:{matchExpressions:[]}})
    else { next[kind][key] ||= {nodeSelectorTerms:[]}; next[kind][key].nodeSelectorTerms.push({matchExpressions:[]}) }
  } else {
    const term = {topologyKey:'kubernetes.io/hostname',labelSelector:{matchExpressions:[]}}
    ;(next[kind][key] ||= []).push(preferred ? {weight:50,podAffinityTerm:term} : term)
  }
  emit('update:modelValue',next)
}
function terms(kind, preferred) {
  const key = preferred ? 'preferredDuringSchedulingIgnoredDuringExecution' : 'requiredDuringSchedulingIgnoredDuringExecution'
  const value = affinity.value[kind]?.[key]
  return kind === 'nodeAffinity' && !preferred ? value?.nodeSelectorTerms || [] : value || []
}
function term(rule,kind,preferred) { return preferred ? (kind === 'nodeAffinity' ? rule.preference : rule.podAffinityTerm) : rule }
function expressions(rule,kind,preferred) { const t = term(rule,kind,preferred); return kind === 'nodeAffinity' ? t : (t.labelSelector || {}) }
function setExpressions(rule,kind,preferred,value) { const t=term(rule,kind,preferred); if(kind==='nodeAffinity') t.matchExpressions=value; else {t.labelSelector ||= {};t.labelSelector.matchExpressions=value} }
</script>
<template><div><el-collapse><el-collapse-item v-for="kind in ['nodeAffinity','podAffinity','podAntiAffinity']" :key="kind" :title="tr({nodeAffinity:'节点亲和性',podAffinity:'Pod 亲和性',podAntiAffinity:'Pod 反亲和性'}[kind])"><section v-for="preferred in [false,true]" :key="String(preferred)"><h4>{{ tr(preferred ? '优选规则' : '必需规则') }}</h4><div v-for="(rule,index) in terms(kind,preferred)" :key="index" class="kwe-affinity-rule"><div class="rule-head"><el-input-number v-if="preferred" v-model="rule.weight" :min="1" :max="100" /><el-input v-if="kind !== 'nodeAffinity'" v-model="term(rule,kind,preferred).topologyKey" placeholder="topologyKey" /><el-button link type="danger" :disabled="disabled" @click="terms(kind,preferred).splice(index,1)">{{ tr('删除') }}</el-button></div><Rows :model-value="expressions(rule,kind,preferred).matchExpressions" @update:model-value="setExpressions(rule,kind,preferred,$event)" :disabled="disabled" :columns="[{key:'key',label:'标签键'},{key:'operator',label:'运算符',options:['In','NotIn','Exists','DoesNotExist',...(kind === 'nodeAffinity' ? ['Gt','Lt'] : [])]}]" :create="() => ({key:'',operator:'In',values:[]})" /><div v-for="(expression,i) in expressions(rule,kind,preferred).matchExpressions" :key="i" class="expression-values"><span>{{ expression.key || `#${i+1}` }}</span><el-select v-if="!['Exists','DoesNotExist'].includes(expression.operator)" v-model="expression.values" multiple filterable allow-create default-first-option :placeholder="tr('标签值，输入后回车')" /></div><el-select v-if="kind !== 'nodeAffinity'" v-model="term(rule,kind,preferred).namespaces" multiple filterable allow-create default-first-option :placeholder="tr('命名空间，留空使用当前空间')" /></div><el-button size="small" :disabled="disabled" @click="add(kind,preferred)">+ {{ tr(preferred ? '优选规则' : '必需规则') }}</el-button></section></el-collapse-item></el-collapse><JsonField :model-value="modelValue" :disabled="disabled" :label="tr('完整亲和性配置（JSON）')" @update:model-value="emit('update:modelValue',$event)" @validity="emit('validity',$event)" /><p class="hint">{{ tr('节点必需规则之间为 OR，同一规则内的表达式为 AND；优选规则按权重评分') }}</p></div></template>
<style scoped>.kwe-affinity-rule{padding:14px;border:1px solid #e4eaf2;border-radius:8px;margin-bottom:12px}.rule-head{display:flex;gap:12px;justify-content:space-between;margin-bottom:12px}.expression-values{display:flex;align-items:center;gap:12px;margin:10px 0}.expression-values .el-select{flex:1}.hint{color:#8390a2;font-size:12px}section{padding:0 0 18px}</style>
