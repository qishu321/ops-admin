<script setup>
import { computed, ref, watch } from 'vue'
import { Promotion } from '@element-plus/icons-vue'

const props = defineProps({
  modelValue: Boolean,
  targets: { type: Array, default: () => [] },
  sending: Boolean
})
const emit = defineEmits(['update:modelValue', 'confirm'])
const command = ref('')
const inputRef = ref()
const gutterRef = ref()
const canSend = computed(() => command.value.trim() && props.targets.length && !props.sending)
const lineCount = computed(() => command.value.split(/\r\n|\r|\n/).length)
watch(() => props.modelValue, (visible) => { if (visible) command.value = '' })
function syncEditorScroll(event) {
  if (gutterRef.value) gutterRef.value.scrollTop = event.target.scrollTop
}
</script>

<template>
  <el-dialog class="terminal-broadcast-dialog" :model-value="modelValue" title="发送命令到所有会话" width="min(680px, calc(100vw - 32px))" align-center append-to-body :close-on-click-modal="false" @update:model-value="emit('update:modelValue', $event)" @opened="inputRef?.focus()">
    <template #header="{ titleId }">
      <div class="broadcast-heading">
        <span class="broadcast-heading-icon" aria-hidden="true"><el-icon><Promotion /></el-icon></span>
        <div><h2 :id="titleId">发送命令到所有会话</h2><p>一次输入，同步发送至已连接的终端</p></div>
      </div>
    </template>
    <section class="broadcast-recipient-section" aria-label="接收命令的会话">
      <div class="broadcast-section-heading"><span>接收会话</span><span class="broadcast-connected"><i aria-hidden="true" />{{ targets.length }} 个已连接</span></div>
      <div class="broadcast-targets">
        <span v-for="target in targets" :key="target.id" class="broadcast-target" :title="target.label"><i aria-hidden="true" /><span>{{ target.label }}</span></span>
        <span v-if="!targets.length" class="broadcast-empty">暂无已连接的终端</span>
      </div>
      <p class="broadcast-scope-note">未连接的会话会自动跳过</p>
    </section>
    <div class="broadcast-editor">
      <div class="broadcast-editor-toolbar"><div class="broadcast-editor-title"><span aria-hidden="true">&gt;_</span><label for="terminal-broadcast-command">命令</label></div><span>支持多行输入</span></div>
      <div class="broadcast-editor-body" @scroll.capture="syncEditorScroll">
        <div ref="gutterRef" class="broadcast-gutter" aria-hidden="true"><div v-for="line in lineCount" :key="line">{{ line }}</div></div>
        <el-input id="terminal-broadcast-command" ref="inputRef" v-model="command" type="textarea" :rows="7" resize="none" placeholder="例如：hostname && uptime" :spellcheck="false" :disabled="sending" />
      </div>
    </div>
    <template #footer>
      <div class="broadcast-footer">
        <span class="broadcast-enter-note"><span aria-hidden="true">↵</span>发送后自动回车执行</span>
        <div class="broadcast-footer-actions"><el-button class="broadcast-cancel" :disabled="sending" @click="emit('update:modelValue', false)">取消</el-button><el-button class="broadcast-send" type="primary" :icon="Promotion" :loading="sending" :disabled="!canSend" @click="emit('confirm', command)">发送命令</el-button></div>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
:global(.terminal-broadcast-dialog.el-dialog) {
  --broadcast-surface: #142230; --broadcast-line: #2b3e4e; --broadcast-ink: #e1e9f0; --broadcast-muted: #93a6b6; --broadcast-accent: #21c791;
  --ops-line: var(--broadcast-line); --ops-ink: var(--broadcast-ink);
  --el-dialog-bg-color: var(--broadcast-surface); --el-text-color-primary: var(--broadcast-ink); --el-text-color-regular: var(--broadcast-muted);
  padding: 0; border: 1px solid var(--broadcast-line); border-color: var(--broadcast-line) !important; background: var(--broadcast-surface); color: var(--broadcast-ink); border-radius: 14px !important; box-shadow: 0 28px 90px #0007 !important;
}
:global(.terminal-broadcast-dialog .el-dialog__header) { height: auto; margin: 0; padding: 24px 26px 22px !important; border-bottom: 1px solid var(--broadcast-line); background: transparent; }
:global(.terminal-broadcast-dialog .el-dialog__body) { padding: 22px 26px 24px !important; }
:global(.terminal-broadcast-dialog .el-dialog__footer) { padding: 16px 26px !important; border-top: 1px solid var(--broadcast-line); background: #101c28; }
:global(.terminal-broadcast-dialog .el-dialog__headerbtn) { top: 17px; right: 15px; width: 32px; height: 32px; }
:global(.terminal-broadcast-dialog .el-dialog__close) { color: #93a6b6; }
.broadcast-heading { display: flex; align-items: center; gap: 14px; padding-right: 24px; }
.broadcast-heading-icon { display: grid; flex: 0 0 42px; height: 42px; place-items: center; border: 1px solid #285748; border-radius: 10px; background: #193b34; color: var(--broadcast-accent); font-size: 23px; }
.broadcast-heading h2 { margin: 0; color: var(--broadcast-ink); font: 600 19px/1.5 "Segoe UI", "Microsoft YaHei", sans-serif; }
.broadcast-heading p { margin: 3px 0 0; color: var(--broadcast-muted); font-size: 12px; line-height: 1.6; }
.broadcast-section-heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 11px; color: #c3d1dc; font-size: 12px; font-weight: 500; }
.broadcast-connected { display: inline-flex; align-items: center; gap: 6px; color: #8ed7b9; font-size: 12px; font-weight: 400; }
.broadcast-connected i, .broadcast-target i { width: 5px; height: 5px; flex: 0 0 5px; border-radius: 50%; background: var(--broadcast-accent); }
.broadcast-targets { display: flex; flex-wrap: wrap; gap: 7px; max-height: 98px; overflow-y: auto; }
.broadcast-target { display: inline-flex; align-items: center; gap: 8px; max-width: 100%; padding: 6px 9px; border: 1px solid #2b4252; border-radius: 6px; background: #1a2d3d; color: #cbdce8; font: 12px/1.4 Consolas, "Microsoft YaHei", monospace; }
.broadcast-target > span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.broadcast-scope-note { margin: 9px 0 20px; color: var(--broadcast-muted); font-size: 12px; }
.broadcast-empty { color: var(--broadcast-muted); font-size: 13px; }
.broadcast-editor { overflow: hidden; border: 1px solid #354c5d; border-radius: 9px; background: #0b1420; transition: border-color .15s, box-shadow .15s; }
.broadcast-editor:focus-within { border-color: var(--broadcast-accent); box-shadow: 0 0 0 3px #21c79112; }
.broadcast-editor-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 38px; padding: 0 14px; border-bottom: 1px solid #263747; background: #192b3b; }
.broadcast-editor-title { display: inline-flex; align-items: center; gap: 9px; }
.broadcast-editor-title label { color: #c8d8e5; font-size: 12px; font-weight: 500; }
.broadcast-editor-title > span { color: #8aa9bf; font: 600 14px Consolas, monospace; }
.broadcast-editor-toolbar > span { color: var(--broadcast-muted); font-size: 11px; }
.broadcast-editor-body { display: flex; }
.broadcast-gutter { flex: 0 0 38px; max-height: 200px; padding: 16px 10px 16px 0; overflow: hidden; border-right: 1px solid #1e3040; color: #627e93; text-align: right; font: 13px/24px Consolas, monospace; user-select: none; }
.broadcast-editor :deep(.el-textarea) { min-width: 0; }
.broadcast-editor :deep(.el-textarea__inner) { height: 200px; padding: 16px 15px; border: 0; border-radius: 0 !important; background: #0b1420; color: #dcf1e8; box-shadow: none !important; font: 14px/24px Consolas, "Microsoft YaHei", monospace; tab-size: 2; caret-color: var(--broadcast-accent); }
.broadcast-editor :deep(.el-textarea__inner::placeholder) { color: #6d879b; }
.broadcast-editor :deep(.el-textarea__inner::selection) { background: #245b4d; color: #fff; }
.broadcast-footer { display: flex; justify-content: space-between; align-items: center; gap: 16px; }
.broadcast-enter-note { display: inline-flex; align-items: center; gap: 8px; color: var(--broadcast-muted); font-size: 12px; }
.broadcast-enter-note > span { font: 18px Consolas, monospace; }
.broadcast-footer-actions { display: flex; flex: 0 0 auto; gap: 8px; }
.broadcast-footer-actions :deep(.el-button) { height: 34px; margin: 0; padding: 0 16px; font-size: 13px; }
.broadcast-cancel { --el-button-bg-color: #1c2d3b; --el-button-border-color: #354b5c; --el-button-text-color: #c4d2dd; --el-button-hover-bg-color: #263f50; --el-button-hover-border-color: #4b6578; --el-button-hover-text-color: #fff; }
.broadcast-send.el-button--primary { --el-button-bg-color: #21c791; --el-button-border-color: #21c791; --el-button-text-color: #062d22; --el-button-hover-bg-color: #47d7a9; --el-button-hover-border-color: #47d7a9; --el-button-hover-text-color: #062d22; --el-button-disabled-bg-color: #25483f; --el-button-disabled-border-color: #2e5649; --el-button-disabled-text-color: #83ac9c; font-weight: 600; }
@media (max-width: 600px) {
  :global(.terminal-broadcast-dialog .el-dialog__header) { padding: 20px 18px 18px !important; }
  :global(.terminal-broadcast-dialog .el-dialog__body) { padding: 18px !important; }
  :global(.terminal-broadcast-dialog .el-dialog__footer) { padding: 14px 18px !important; }
  .broadcast-heading { gap: 10px; }.broadcast-heading h2 { font-size: 16px; }.broadcast-heading-icon { flex-basis: 34px; height: 34px; font-size: 19px; }
  .broadcast-footer { flex-wrap: wrap; }.broadcast-footer-actions { margin-left: auto; }
}
@media (prefers-reduced-motion: reduce) { .broadcast-editor { transition: none; } }
</style>
