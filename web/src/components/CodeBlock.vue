<script setup lang="ts">
import CopyButton from './CopyButton.vue'

withDefaults(
  defineProps<{
    code: string
    title?: string
    language?: string
    maxHeight?: string
  }>(),
  { title: '', language: '', maxHeight: '320px' },
)
</script>

<template>
  <div class="code-shell">
    <div class="code-head">
      <span class="code-title">{{ title || language || 'code' }}</span>
      <copy-button :text="code" label="复制" size="tiny" />
    </div>
    <pre class="code-body" :style="{ maxHeight }"><code>{{ code }}</code></pre>
  </div>
</template>

<style scoped>
.code-shell {
  overflow: hidden;
  border: 1px solid #e5eaf2;
  border-radius: 12px;
  background: #f8fafc;
}

.code-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 6px 9px 6px 13px;
  border-bottom: 1px solid #e9eef5;
  background: #fbfcfe;
}

.code-title {
  color: #94a3b8;
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.07em;
  text-transform: uppercase;
}

.code-body {
  margin: 0;
  padding: 13px 15px;
  overflow: auto;
  color: #334155;
  font-family: var(--mono);
  font-size: 11px;
  line-height: 1.75;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
