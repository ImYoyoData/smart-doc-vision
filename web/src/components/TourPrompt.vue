<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NButton } from 'naive-ui'
import { SchoolOutline } from '@vicons/ionicons5'
import { useTour } from '@/composables/useTour'

const props = defineProps<{
  tourKey: string
  /** What the prompt calls the thing being configured, e.g. 「模型接入」. */
  label?: string
  /** Extra delay so a modal animation finishes before the prompt slides in. */
  delay?: number
}>()

const tour = useTour()
const offering = ref(false)

function accept() {
  offering.value = false
  tour.start(props.tourKey)
}

// Once per browser per tour: never ask the same question twice.
onMounted(() => {
  window.setTimeout(() => {
    if (tour.state.active) return
    if (!tour.shouldOffer(props.tourKey)) return
    tour.markOffered(props.tourKey)
    offering.value = true
  }, props.delay ?? 700)
})

defineExpose({ dismiss: () => (offering.value = false) })
</script>

<template>
  <div v-if="offering" class="tour-prompt">
    <span class="tour-prompt-icon"><SchoolOutline /></span>
    <span class="tour-prompt-copy">
      <strong>需要我带你填一遍{{ label || '这个表单' }}吗？</strong>
      <small>约 30 秒，带语音讲解；随时可以跳过。</small>
    </span>
    <n-button size="tiny" type="primary" @click="accept">开始引导</n-button>
    <n-button size="tiny" quaternary @click="offering = false">不用了</n-button>
  </div>
</template>

<style scoped>
.tour-prompt {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
  padding: 9px 12px;
  border: 1px solid var(--brand-border);
  border-radius: 11px;
  background: var(--brand-softer);
}

.tour-prompt-icon {
  display: flex;
  width: 26px;
  height: 26px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--brand-soft);
  color: var(--brand);
  flex: none;
}

.tour-prompt-icon :deep(svg) {
  width: 15px;
  height: 15px;
}

.tour-prompt-copy {
  min-width: 0;
  flex: 1;
}

.tour-prompt-copy strong {
  display: block;
  color: var(--ink);
  font-size: 11.5px;
  font-weight: 620;
}

.tour-prompt-copy small {
  display: block;
  margin-top: 2px;
  color: var(--muted);
  font-size: 10px;
}
</style>
