<script setup>
import { computed, watch } from 'vue'
import en from 'element-plus/es/locale/lang/en'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import { locale } from './utils/i18n'

const componentLocale = computed(() => locale.value === 'en-US' ? en : zhCn)
watch(locale, value => { document.documentElement.lang = value }, { immediate: true })
</script>

<template>
  <el-config-provider :locale="componentLocale">
    <router-view />
  </el-config-provider>
</template>
