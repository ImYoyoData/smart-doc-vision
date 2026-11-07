<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NButton } from 'naive-ui'
import { HelpCircleOutline, SchoolOutline } from '@vicons/ionicons5'
import { useTour } from '@/composables/useTour'

const props = defineProps<{
  title: string
  subtitle?: string
  eyebrow?: string
  /** Tour key from constants/tours.ts; renders the page guide button. */
  tourKey?: string
}>()

const tour = useTour()
const offering = ref(false)

function openTour() {
  if (props.tourKey) tour.start(props.tourKey, true)
}

function acceptOffer() {
  offering.value = false
  if (props.tourKey) tour.start(props.tourKey)
}

// First visit to this page (and never seen): show a small inline prompt instead
// of popping the tour open behind the user's back. The offer is remembered, so
// it never asks twice.
onMounted(() => {
  if (!props.tourKey) return
  if (tour.state.active) return
  if (!tour.shouldOffer(props.tourKey)) return
  tour.markOffered(props.tourKey)
  offering.value = true
})
</script>

<template>
  <header class="page-head">
    <div>
      <div v-if="eyebrow" class="eyebrow">
        <span class="eyebrow-dot"></span>
        {{ eyebrow }}
      </div>
      <h1>{{ title }}</h1>
      <p v-if="subtitle">{{ subtitle }}</p>
    </div>
    <div v-if="$slots.actions || tourKey" class="page-actions">
      <slot name="actions" />
      <button
        v-if="tourKey"
        class="page-tour-btn"
        type="button"
        title="查看本页引导"
        aria-label="查看本页引导"
        @click="openTour"
      >
        <HelpCircleOutline />
        <span>本页引导</span>
      </button>
    </div>

    <div v-if="offering" class="tour-offer">
      <span class="tour-offer-icon"><SchoolOutline /></span>
      <span class="tour-offer-copy">
        <strong>第一次来到「{{ title }}」</strong>
        <small>需要我用语音带你过一遍这个页面的主要功能吗？</small>
      </span>
      <n-button size="tiny" type="primary" @click="acceptOffer">开始引导</n-button>
      <n-button size="tiny" quaternary @click="offering = false">不用了</n-button>
    </div>
  </header>
</template>

<style scoped>
.page-head {
  align-items: flex-start;
  flex-wrap: wrap;
}

.tour-offer {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 10px;
  margin-top: 10px;
  padding: 9px 12px;
  border: 1px solid var(--brand-border);
  border-radius: 11px;
  background: var(--brand-softer);
}

.tour-offer-icon {
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

.tour-offer-icon :deep(svg) {
  width: 15px;
  height: 15px;
}

.tour-offer-copy {
  min-width: 0;
  flex: 1;
}

.tour-offer-copy strong {
  display: block;
  color: var(--ink);
  font-size: 11.5px;
  font-weight: 620;
}

.tour-offer-copy small {
  display: block;
  margin-top: 2px;
  color: var(--muted);
  font-size: 10px;
}

.eyebrow-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--brand);
  box-shadow: 0 0 0 4px rgba(43, 108, 246, 0.09);
}

.page-tour-btn {
  display: inline-flex;
  height: 32px;
  align-items: center;
  gap: 5px;
  padding: 0 11px;
  border: 1px solid var(--brand-border);
  border-radius: 9px;
  background: var(--brand-softer);
  color: var(--brand-strong);
  font-size: 11.5px;
  font-weight: 550;
  cursor: pointer;
  transition: background 0.16s ease, border-color 0.16s ease;
}

.page-tour-btn:hover {
  border-color: var(--brand);
  background: var(--brand-soft);
}

.page-tour-btn :deep(svg) {
  width: 15px;
  height: 15px;
}
</style>
