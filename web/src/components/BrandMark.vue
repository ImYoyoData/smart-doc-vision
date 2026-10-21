<script setup lang="ts">
withDefaults(defineProps<{ size?: number; variant?: 'gradient' | 'plain' }>(), {
  size: 38,
  variant: 'gradient',
})
</script>

<template>
  <span class="brand-mark" :class="variant" :style="{ width: `${size}px`, height: `${size}px` }">
    <svg viewBox="0 0 48 48" fill="none" aria-hidden="true">
      <path
        d="M13 9.5h16.5L37 17v21.5H13z"
        stroke="currentColor"
        stroke-width="2.6"
        stroke-linejoin="round"
      />
      <path d="M29.5 9.5V17H37" stroke="currentColor" stroke-width="2.6" stroke-linejoin="round" />
      <path d="M18.5 24h13M18.5 30h9" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" />
      <circle cx="34.5" cy="34.5" r="6.2" fill="var(--brand, #2b6cf6)" stroke="white" stroke-width="2" />
      <path d="M32.2 34.5l1.6 1.7 3-3.5" stroke="white" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
  </span>
</template>

<style scoped>
.brand-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  flex-shrink: 0;
}

.brand-mark svg {
  width: 76%;
  height: 76%;
}

.brand-mark.gradient {
  color: #fff;
  background: linear-gradient(140deg, #4f8bff 0%, #2358df 58%, #1b3fae 100%);
  box-shadow: 0 6px 16px -4px rgba(43, 108, 246, 0.5), inset 0 1px 0 rgba(255, 255, 255, 0.25);
}

.brand-mark.gradient svg circle {
  fill: #0b1e3f;
}

.brand-mark.plain {
  color: var(--brand);
  background: var(--brand-soft);
}
</style>
