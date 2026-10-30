<script setup>
import { ref, watch } from 'vue'
import { tr } from '../../../utils/i18n'
const props=defineProps({modelValue:Object,disabled:Boolean})
const emit=defineEmits(['update:modelValue','validity'])
const rows=ref([])
let ownUpdate=false
watch(() => props.modelValue, value => {if(ownUpdate){ownUpdate=false;return} rows.value=Object.entries(value || {}).map(([key,value]) => ({key,value}))},{immediate:true,deep:true})
function update() {
  const keys=rows.value.map(row => row.key)
  const valid=keys.every(Boolean) && new Set(keys).size===keys.length
  emit('validity',valid)
  if(valid){ownUpdate=true;emit('update:modelValue',Object.fromEntries(rows.value.map(row => [row.key,String(row.value)])))}
}
</script>
<template><div><div v-for="(row,i) in rows" :key="i" class="map-row"><el-input v-model="row.key" :disabled="disabled" :placeholder="tr('键')" @input="update" /><el-input v-model="row.value" :disabled="disabled" :placeholder="tr('值')" @input="update" /><el-button :disabled="disabled" link type="danger" @click="rows.splice(i,1);update()">{{ tr('删除') }}</el-button></div><el-button :disabled="disabled" size="small" @click="rows.push({key:'',value:''});emit('validity',false)">+ {{ tr('添加') }}</el-button></div></template>
<style scoped>.map-row{display:flex;gap:10px;margin:8px 0}.map-row .el-input{flex:1}</style>
