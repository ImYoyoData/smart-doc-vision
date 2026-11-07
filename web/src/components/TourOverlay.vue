<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NSwitch } from 'naive-ui'
import {
  ChevronBackOutline,
  ChevronForwardOutline,
  CloseOutline,
  VolumeHighOutline,
  VolumeMuteOutline,
} from '@vicons/ionicons5'
import { useTour } from '@/composables/useTour'

const tour = useTour()
const tooltipRef = ref<HTMLElement | null>(null)
const tooltipSize = ref({ width: 360, height: 190 })
const viewport = ref({ width: window.innerWidth, height: window.innerHeight })

const step = computed(() => tour.currentStep.value)
const rect = computed(() => tour.state.rect)
const progress = computed(() => (tour.totalSteps.value ? `${tour.state.index + 1} / ${tour.totalSteps.value}` : ''))

const placement = computed<'top' | 'bottom' | 'left' | 'right'>(() => {
  const preferred = step.value?.placement
  if (!rect.value) return 'bottom'
  const space = {
    top: rect.value.top,
    bottom: viewport.value.height - (rect.value.top + rect.value.height),
    left: rect.value.left,
    right: viewport.value.width - (rect.value.left + rect.value.width),
  }
  const need = { vertical: tooltipSize.value.height + 24, horizontal: tooltipSize.value.width + 24 }
  if (preferred && space[preferred] > (preferred === 'top' || preferred === 'bottom' ? need.vertical : need.horizontal)) {
    return preferred
  }
  if (space.bottom > need.vertical) return 'bottom'
  if (space.top > need.vertical) return 'top'
  if (space.right > need.horizontal) return 'right'
  if (space.left > need.horizontal) return 'left'
  return 'bottom'
})

// Tooltip position is clamped to the viewport so a spotlight near an edge never
// pushes the card off screen.
const tooltipStyle = computed(() => {
  const gap = 14
  const margin = 12
  const width = tooltipSize.value.width
  const height = tooltipSize.value.height
  if (!rect.value) {
    return {
      left: `${Math.max(margin, (viewport.value.width - width) / 2)}px`,
      top: `${Math.max(margin, (viewport.value.height - height) / 2)}px`,
      width: `${width}px`,
    }
  }
  let left = 0
  let top = 0
  switch (placement.value) {
    case 'top':
      left = rect.value.left + rect.value.width / 2 - width / 2
      top = rect.value.top - height - gap
      break
    case 'left':
      left = rect.value.left - width - gap
      top = rect.value.top + rect.value.height / 2 - height / 2
      break
    case 'right':
      left = rect.value.left + rect.value.width + gap
      top = rect.value.top + rect.value.height / 2 - height / 2
      break
    default:
      left = rect.value.left + rect.value.width / 2 - width / 2
      top = rect.value.top + rect.value.height + gap
  }
  left = Math.min(Math.max(margin, left), viewport.value.width - width - margin)
  top = Math.min(Math.max(margin, top), viewport.value.height - height - margin)
  return { left: `${left}px`, top: `${top}px`, width: `${width}px` }
})

const spotlightStyle = computed(() => {
  if (!rect.value) return {}
  return {
    left: `${rect.value.left}px`,
    top: `${rect.value.top}px`,
    width: `${rect.value.width}px`,
    height: `${rect.value.height}px`,
  }
})

const isLast = computed(() => tour.state.index >= tour.totalSteps.value - 1)

watch(
  () => [tour.state.active, tour.state.index],
  () => {
    if (!tour.state.active) return
    requestAnimationFrame(() => {
      const element = tooltipRef.value
      if (!element) return
      tooltipSize.value = {
        width: Math.min(400, Math.max(300, element.offsetWidth || 360)),
        height: Math.max(150, element.offsetHeight || 190),
      }
    })
  },
)

function onViewportChange() {
  viewport.value = { width: window.innerWidth, height: window.innerHeight }
}

function onKeydown(event: KeyboardEvent) {
  if (!tour.state.active) return
  if (event.key === 'Escape') {
    tour.dismiss()
  } else if (event.key === 'ArrowRight' || event.key === 'Enter') {
    event.preventDefault()
    tour.next()
  } else if (event.key === 'ArrowLeft') {
    event.preventDefault()
    tour.prev()
  }
}

onMounted(() => {
  window.addEventListener('resize', onViewportChange)
  window.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', onViewportChange)
  window.removeEventListener('keydown', onKeydown)
  tour.cancelSpeech()
})
</script>

<template>
  <Teleport to="body">
    <div v-if="tour.state.active && step" class="tour-root" role="dialog" aria-modal="true" :aria-label="step.title">
      <div class="tour-scrim" :class="{ dim: !rect }" @click.self="tour.dismiss()"></div>
      <div v-if="rect" class="tour-spotlight" :style="spotlightStyle"></div>

      <div ref="tooltipRef" class="tour-card" :style="tooltipStyle" @click.stop>
        <div class="tour-card-head">
          <span class="tour-badge">{{ tour.tour.value?.title }}</span>
          <span class="tour-progress">{{ progress }}</span>
          <button class="tour-close" type="button" title="关闭引导（Esc）" aria-label="关闭引导" @click="tour.dismiss()">
            <CloseOutline />
          </button>
        </div>

        <h3 class="tour-title">{{ step.title }}</h3>
        <p class="tour-body">{{ step.body }}</p>

        <div class="tour-voice">
          <button
            type="button"
            class="tour-voice-btn"
            :class="{ off: !tour.state.voiceEnabled }"
            :title="tour.state.voiceEnabled ? '关闭语音讲解' : '开启语音讲解'"
            @click="tour.setVoice(!tour.state.voiceEnabled)"
          >
            <VolumeHighOutline v-if="tour.state.voiceEnabled" />
            <VolumeMuteOutline v-else />
          </button>
          <span>{{ tour.state.voiceEnabled ? '语音讲解已开启' : '语音讲解已关闭' }}</span>
          <n-switch
            size="small"
            :value="tour.state.voiceEnabled"
            @update:value="(value: boolean) => tour.setVoice(value)"
          />
        </div>

        <div class="tour-dots" v-if="tour.totalSteps.value > 1">
          <button
            v-for="index in tour.totalSteps.value"
            :key="index"
            type="button"
            class="tour-dot"
            :class="{ active: index - 1 === tour.state.index }"
            :title="`第 ${index} 步`"
            @click="tour.state.index = index - 1"
          ></button>
        </div>

        <div class="tour-actions">
          <n-button text type="primary" @click="tour.dismiss()">跳过引导</n-button>
          <div class="tour-actions-right">
            <n-button secondary :disabled="tour.state.index === 0" @click="tour.prev()">
              <template #icon><ChevronBackOutline /></template>
              上一步
            </n-button>
            <n-button type="primary" @click="tour.next()">
              {{ isLast ? '完成' : '下一步' }}
              <template v-if="!isLast" #icon><ChevronForwardOutline /></template>
            </n-button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.tour-root {
  position: fixed;
  inset: 0;
  z-index: 4000;
  pointer-events: none;
}

.tour-scrim {
  position: absolute;
  inset: 0;
  pointer-events: auto;
}

.tour-scrim.dim {
  background: rgba(9, 16, 32, 0.62);
  backdrop-filter: blur(1.5px);
}

.tour-spotlight {
  position: absolute;
  border: 2px solid var(--brand, #2b6cf6);
  border-radius: 12px;
  box-shadow: 0 0 0 9999px rgba(9, 16, 32, 0.62);
  transition: all 0.22s ease;
  pointer-events: none;
}

.tour-card {
  position: absolute;
  max-width: calc(100vw - 24px);
  padding: 16px 18px 14px;
  border: 1px solid var(--border, #e6ebf3);
  border-radius: 16px;
  background: var(--card, #fff);
  box-shadow: 0 24px 64px -20px rgba(15, 23, 42, 0.42);
  pointer-events: auto;
  animation: tour-in 0.2s ease;
}

@keyframes tour-in {
  from { opacity: 0; transform: translateY(6px); }
  to { opacity: 1; transform: translateY(0); }
}

.tour-card-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tour-badge {
  padding: 1px 7px;
  border-radius: 99px;
  background: var(--brand-soft, rgba(43, 108, 246, 0.1));
  color: var(--brand-strong, #1d4ed8);
  font-size: 9.5px;
  font-weight: 620;
}

.tour-progress {
  color: var(--muted-light, #94a3b8);
  font-family: var(--mono, ui-monospace, monospace);
  font-size: 9.5px;
}

.tour-close {
  display: flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  margin-left: auto;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--muted-light, #94a3b8);
  cursor: pointer;
}

.tour-close:hover {
  background: var(--surface-hover, #f1f4f9);
  color: var(--ink, #0f172a);
}

.tour-close :deep(svg) {
  width: 14px;
  height: 14px;
}

.tour-title {
  margin: 10px 0 6px;
  color: var(--ink, #0f172a);
  font-size: 15px;
  font-weight: 660;
}

.tour-body {
  margin: 0;
  color: var(--muted, #5b6b82);
  font-size: 12px;
  line-height: 1.75;
}

.tour-voice {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  padding: 7px 10px;
  border: 1px solid var(--border, #e6ebf3);
  border-radius: 9px;
  background: var(--surface-soft, #f7f9fc);
  color: var(--muted, #5b6b82);
  font-size: 10.5px;
}

.tour-voice :deep(.n-switch) {
  margin-left: auto;
}

.tour-voice-btn {
  display: flex;
  width: 22px;
  height: 22px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: var(--brand-soft, rgba(43, 108, 246, 0.1));
  color: var(--brand, #2b6cf6);
  cursor: pointer;
  flex: none;
}

.tour-voice-btn.off {
  background: var(--surface-strong, #eef2f8);
  color: var(--muted-light, #94a3b8);
}

.tour-voice-btn :deep(svg) {
  width: 13px;
  height: 13px;
}

.tour-dots {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-top: 12px;
}

.tour-dot {
  width: 6px;
  height: 6px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: var(--border-strong, #d5dde8);
  cursor: pointer;
}

.tour-dot.active {
  width: 16px;
  border-radius: 99px;
  background: var(--brand, #2b6cf6);
}

.tour-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 14px;
}

.tour-actions-right {
  display: flex;
  gap: 8px;
}
</style>
