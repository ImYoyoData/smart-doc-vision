<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { NSpin } from 'naive-ui'
import { CheckmarkCircleOutline, ChevronForwardOutline, RefreshOutline } from '@vicons/ionicons5'
import { authApi } from '@/api'
import type { CaptchaChallenge } from '@/api/types'

const HANDLE_SIZE = 42

const props = withDefaults(defineProps<{ disabled?: boolean }>(), { disabled: false })
const model = defineModel<{ id: string; x: number } | null>({ default: null })
const challenge = ref<CaptchaChallenge | null>(null)
const loading = ref(false)
const errorText = ref('')
const handleLeft = ref(0)
const dragging = ref(false)
const stageWidth = ref(0)
const stageRef = ref<HTMLElement | null>(null)
let startX = 0
let startLeft = 0
let resizeObserver: ResizeObserver | null = null

const scale = computed(() => {
  if (!challenge.value || !stageWidth.value) return 0
  return stageWidth.value / challenge.value.width
})

const maxHandleLeft = computed(() => Math.max(0, stageWidth.value - HANDLE_SIZE))
const maxPieceLeft = computed(() => {
  if (!challenge.value) return 0
  return Math.max(0, challenge.value.width - challenge.value.piece_size)
})

const pieceStyle = computed(() => {
  if (!challenge.value || !scale.value) return {}
  const ratio = maxHandleLeft.value > 0 ? handleLeft.value / maxHandleLeft.value : 0
  return {
    width: `${challenge.value.piece_size * scale.value}px`,
    height: `${challenge.value.piece_size * scale.value}px`,
    left: `${ratio * maxPieceLeft.value * scale.value}px`,
    top: `${challenge.value.piece_y * scale.value}px`,
  }
})

const ready = computed(() => model.value !== null)

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

function measure() {
  if (stageRef.value) stageWidth.value = stageRef.value.clientWidth
}

function syncModel() {
  if (!challenge.value) {
    model.value = null
    return
  }
  if (handleLeft.value <= 2) {
    model.value = null
    return
  }
  const ratio = maxHandleLeft.value > 0 ? handleLeft.value / maxHandleLeft.value : 0
  model.value = {
    id: challenge.value.id,
    x: Math.round(ratio * maxPieceLeft.value),
  }
}

function onPointerDown(event: PointerEvent) {
  if (!challenge.value || loading.value || props.disabled) return
  dragging.value = true
  startX = event.clientX
  startLeft = handleLeft.value
  model.value = null
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}

function onPointerMove(event: PointerEvent) {
  if (!dragging.value || props.disabled) return
  handleLeft.value = clamp(startLeft + event.clientX - startX, 0, maxHandleLeft.value)
}

function onPointerUp(event: PointerEvent) {
  if (!dragging.value) return
  dragging.value = false
  if ((event.currentTarget as HTMLElement).hasPointerCapture(event.pointerId)) {
    ;(event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId)
  }
  syncModel()
}

function onKeydown(event: KeyboardEvent) {
  if (!challenge.value || loading.value || props.disabled) return
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    if (handleLeft.value > 2) syncModel()
    return
  }
  if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') return
  event.preventDefault()
  const delta = event.key === 'ArrowRight' ? 6 : -6
  handleLeft.value = clamp(handleLeft.value + delta, 0, maxHandleLeft.value)
  model.value = null
}

async function refresh() {
  loading.value = true
  errorText.value = ''
  model.value = null
  handleLeft.value = 0
  dragging.value = false
  challenge.value = null
  try {
    challenge.value = await authApi.captcha()
    await nextTick()
    measure()
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '验证码加载失败'
  } finally {
    loading.value = false
  }
}

defineExpose({ refresh, ready })

onMounted(() => {
  resizeObserver = new ResizeObserver(measure)
  if (stageRef.value) resizeObserver.observe(stageRef.value)
  refresh()
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
})
</script>

<template>
  <div class="slider-captcha" :class="{ ready, disabled: props.disabled }">
    <div ref="stageRef" class="captcha-stage">
      <img v-if="challenge" class="captcha-bg" :src="challenge.background" alt="滑块验证背景" draggable="false" />
      <img
        v-if="challenge"
        class="captcha-piece"
        :src="challenge.piece"
        :style="pieceStyle"
        alt=""
        draggable="false"
      />
      <div v-if="loading" class="captcha-layer">
        <n-spin size="small" />
        <span>正在加载安全验证...</span>
      </div>
      <div v-else-if="errorText" class="captcha-layer error">
        <span>{{ errorText }}</span>
        <button type="button" class="captcha-retry" @click="refresh">重新加载</button>
      </div>
      <button type="button" class="captcha-refresh" title="换一张" :disabled="loading || props.disabled" @click="refresh">
        <RefreshOutline />
      </button>
      <div v-if="ready && !dragging" class="captcha-done">
        <CheckmarkCircleOutline />
        已对齐，等待后端验证
      </div>
    </div>

    <div class="slider-track" :class="{ aligning: dragging, ready }">
      <div class="slider-fill" :style="{ width: `${handleLeft + HANDLE_SIZE / 2}px` }"></div>
      <span v-if="!dragging" class="slider-hint">{{ ready ? '已对齐，正在提交后端校验' : '向右拖动滑块完成拼图' }}</span>
      <span v-else class="slider-hint">松开滑块后由后端校验</span>
      <div
        class="slider-handle"
        :style="{ transform: `translateX(${handleLeft}px)` }"
        role="slider"
        tabindex="0"
        aria-label="拖动滑块完成验证"
        :aria-valuemin="0"
        :aria-valuemax="maxHandleLeft"
        :aria-valuenow="Math.round(handleLeft)"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="onPointerUp"
        @pointercancel="onPointerUp"
        @keydown="onKeydown"
      >
        <CheckmarkCircleOutline v-if="ready" />
        <ChevronForwardOutline v-else />
      </div>
    </div>

    <div class="captcha-meta">
      <span>服务端校验 · 拖动对齐缺口，120 秒内有效</span>
      <button type="button" :disabled="loading || props.disabled" @click="refresh">换一张</button>
    </div>
  </div>
</template>

<style scoped>
.slider-captcha {
  margin: 4px 0 2px;
}

.captcha-stage {
  position: relative;
  width: 100%;
  overflow: hidden;
  aspect-ratio: 2 / 1;
  border: 1px solid #e2e8f2;
  border-radius: 11px;
  background: #f1f5fb;
  user-select: none;
}

.captcha-bg,
.captcha-piece {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  user-select: none;
}

.captcha-bg {
  object-fit: fill;
}

.captcha-piece {
  inset: auto;
  z-index: 2;
  filter: drop-shadow(0 2px 3px rgba(15, 23, 42, 0.22));
  transition: left 0.06s linear;
}

.captcha-layer {
  position: absolute;
  inset: 0;
  z-index: 4;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 7px;
  background: rgba(245, 248, 253, 0.9);
  color: var(--muted);
  font-size: 10px;
}

.captcha-layer.error {
  color: #b45309;
  font-size: 10.5px;
}

.captcha-retry {
  padding: 3px 9px;
  border: 1px solid #e3d2ad;
  border-radius: 6px;
  background: #fffdf8;
  color: #a16207;
  font-size: 10px;
  cursor: pointer;
}

.captcha-refresh {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 5;
  display: flex;
  width: 26px;
  height: 26px;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(255, 255, 255, 0.32);
  border-radius: 7px;
  background: rgba(15, 23, 42, 0.28);
  color: #fff;
  cursor: pointer;
  backdrop-filter: blur(4px);
}

.captcha-refresh:hover {
  background: rgba(15, 23, 42, 0.45);
}

.captcha-refresh:disabled {
  cursor: wait;
  opacity: 0.6;
}

.captcha-refresh :deep(svg) {
  width: 13px;
  height: 13px;
}

.captcha-done {
  position: absolute;
  right: 9px;
  bottom: 8px;
  z-index: 4;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 7px;
  border-radius: 999px;
  background: rgba(230, 250, 239, 0.94);
  color: #15803d;
  font-size: 9px;
}

.captcha-done :deep(svg) {
  width: 11px;
  height: 11px;
}

.slider-track {
  position: relative;
  height: 42px;
  margin-top: 9px;
  overflow: hidden;
  border: 1px solid #e1e8f2;
  border-radius: 10px;
  background: #f7f9fc;
  touch-action: none;
  user-select: none;
}

.slider-track.ready {
  border-color: #b8e8cb;
  background: #f4fcf7;
}

.slider-fill {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  max-width: calc(100% - 21px);
  border-radius: 9px 0 0 9px;
  background: linear-gradient(90deg, #dce9ff, #eaf3ff);
  pointer-events: none;
  transition: width 0.06s linear;
}

.slider-track.ready .slider-fill {
  background: linear-gradient(90deg, #dff8e9, #edfdf4);
}

.slider-hint {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding-left: 28px;
  color: #8b97aa;
  font-size: 10px;
  pointer-events: none;
}

.slider-track.ready .slider-hint {
  color: #3f8a5e;
}

.slider-handle {
  position: absolute;
  top: 4px;
  left: 4px;
  z-index: 2;
  display: flex;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: #fff;
  color: var(--brand);
  box-shadow: 0 2px 8px -3px rgba(15, 23, 42, 0.22);
  cursor: grab;
  touch-action: none;
}

.slider-handle:active {
  cursor: grabbing;
}

.slider-handle:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: 2px;
}

.slider-handle :deep(svg) {
  width: 16px;
  height: 16px;
}

.slider-track.ready .slider-handle {
  background: #e8fbf0;
  color: #15803d;
}

.captcha-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 6px;
  color: #9aa5b5;
  font-size: 9px;
}

.captcha-meta button {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--brand);
  font-size: 9px;
  cursor: pointer;
}

.captcha-meta button:disabled {
  color: #aab4c2;
  cursor: wait;
}
</style>
