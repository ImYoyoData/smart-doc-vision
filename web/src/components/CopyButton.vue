<script setup lang="ts">
import { ref } from 'vue'
import { NButton, useMessage } from 'naive-ui'
import { CheckmarkOutline, CopyOutline } from '@vicons/ionicons5'
import { copyText } from '@/utils/clipboard'

const props = withDefaults(
  defineProps<{
    text: string
    label?: string
    size?: 'tiny' | 'small' | 'medium'
    quaternary?: boolean
  }>(),
  { label: '复制', size: 'small', quaternary: true },
)

const message = useMessage()
const copied = ref(false)

async function copy() {
  const ok = await copyText(props.text)
  if (!ok) {
    message.error('复制失败，请手动选择文本')
    return
  }
  copied.value = true
  message.success('已复制到剪贴板')
  window.setTimeout(() => (copied.value = false), 1600)
}
</script>

<template>
  <n-button :size="size" :quaternary="quaternary" @click="copy">
    <template #icon>
      <CheckmarkOutline v-if="copied" />
      <CopyOutline v-else />
    </template>
    {{ copied ? '已复制' : label }}
  </n-button>
</template>
