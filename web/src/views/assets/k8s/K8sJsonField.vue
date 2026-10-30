<script setup>
import { onBeforeUnmount, ref, watch } from 'vue'
import { tr } from '../../../utils/i18n'
const props = defineProps({ modelValue: [Object, Array], label: String, array: Boolean, disabled: Boolean, required:Boolean, validator:Function })
const emit = defineEmits(['update:modelValue', 'validity'])
const text = ref(''), error = ref('')
watch(() => props.modelValue, value => { text.value = value == null ? '' : JSON.stringify(value, null, 2) }, { immediate: true, deep: true })
function update(value) {
  text.value = value
  try {
    const parsed = value.trim() ? JSON.parse(value) : undefined
    if (props.required && parsed == null) throw Error(tr('此配置不能为空'))
    if (parsed != null && props.validator && !props.validator(parsed)) throw Error(tr('JSON 结构不正确，请保留必填字段'))
    if (parsed != null && (props.array ? !Array.isArray(parsed) : typeof parsed !== 'object' || Array.isArray(parsed))) throw Error(tr('JSON 类型不正确'))
    error.value = ''; emit('validity', true); emit('update:modelValue', parsed)
  } catch (e) { error.value = e.message; emit('validity', false) }
}
onBeforeUnmount(() => emit('validity',true))
</script>
<template><div class="kwe-json"><label>{{ label }}</label><el-input :model-value="text" type="textarea" :rows="4" :disabled="disabled" :placeholder="array ? '[]' : '{}'" @input="update" /><small v-if="error" role="alert">{{ error }}</small></div></template>
<style scoped>.kwe-json{margin:12px 0}.kwe-json>label{display:block;margin-bottom:8px;color:#64748b}.kwe-json small{color:#dc2626}.kwe-json :deep(textarea){font-family:Consolas,monospace;font-size:12px}</style>
