<script setup lang="ts">
import { NButton } from 'naive-ui'
import { DocumentOutline } from '@vicons/ionicons5'

withDefaults(
  defineProps<{
    title?: string
    description?: string
    action?: string
    compact?: boolean
  }>(),
  {
    title: '暂无数据',
    description: '这里还没有内容，先完成一次配置吧。',
    compact: false,
  },
)

defineEmits<{ action: [] }>()
</script>

<template>
  <div class="empty-state" :class="{ compact }">
    <div class="empty-icon">
      <slot name="icon">
        <DocumentOutline />
      </slot>
    </div>
    <h3>{{ title }}</h3>
    <p>{{ description }}</p>
    <n-button v-if="action" type="primary" secondary @click="$emit('action')">{{ action }}</n-button>
    <slot />
  </div>
</template>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 72px 24px;
  text-align: center;
}

.empty-state.compact {
  padding: 38px 20px;
}

.empty-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  margin-bottom: 16px;
  border-radius: 16px;
  background: var(--brand-soft);
  color: var(--brand);
  font-size: 24px;
}

.empty-icon :deep(svg) {
  width: 24px;
  height: 24px;
}

h3 {
  margin: 0 0 5px;
  font-size: 15px;
  font-weight: 600;
  color: var(--ink);
}

p {
  max-width: 380px;
  margin: 0 0 18px;
  color: var(--muted);
  font-size: 12.5px;
  line-height: 1.7;
}
</style>
