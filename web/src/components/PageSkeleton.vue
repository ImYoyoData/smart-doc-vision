<script setup lang="ts">
import { NSkeleton } from 'naive-ui'

withDefaults(
  defineProps<{
    variant?: 'dashboard' | 'table' | 'cards' | 'form' | 'review'
    rows?: number
    cards?: number
  }>(),
  { variant: 'cards', rows: 8, cards: 6 },
)
</script>

<template>
  <div class="page-skeleton" :class="`page-skeleton--${variant}`" aria-busy="true" aria-label="内容加载中">
    <template v-if="variant === 'dashboard'">
      <div class="sk-metrics">
        <div v-for="i in cards" :key="i" class="sk-card">
          <n-skeleton text style="width: 42%" />
          <n-skeleton :height="'30px'" :sharp="false" style="width: 62%" />
          <n-skeleton text :repeat="2" />
        </div>
      </div>
      <div class="sk-charts">
        <div v-for="i in 2" :key="i" class="sk-panel">
          <n-skeleton text style="width: 28%" />
          <n-skeleton :height="'180px'" :sharp="false" />
        </div>
      </div>
    </template>

    <template v-else-if="variant === 'table'">
      <div class="sk-panel">
        <div class="sk-toolbar">
          <n-skeleton text style="width: 180px" />
          <n-skeleton text style="width: 96px" />
          <n-skeleton text style="width: 96px" />
        </div>
        <div class="sk-table-head">
          <n-skeleton v-for="c in 6" :key="c" text />
        </div>
        <div v-for="r in rows" :key="r" class="sk-table-row">
          <n-skeleton v-for="c in 6" :key="c" text />
        </div>
      </div>
    </template>

    <template v-else-if="variant === 'review'">
      <div class="sk-review">
        <div class="sk-panel">
          <n-skeleton text style="width: 32%" />
          <n-skeleton v-for="i in 6" :key="i" :height="'64px'" :sharp="false" />
        </div>
        <div class="sk-panel">
          <n-skeleton text style="width: 40%" />
          <n-skeleton :height="'220px'" :sharp="false" />
          <n-skeleton text :repeat="4" />
        </div>
      </div>
    </template>

    <template v-else-if="variant === 'form'">
      <div v-for="i in 3" :key="i" class="sk-panel">
        <n-skeleton text style="width: 26%" />
        <n-skeleton text :repeat="3" />
        <n-skeleton :height="'36px'" :sharp="false" style="width: 42%" />
      </div>
    </template>

    <template v-else>
      <div class="sk-cards">
        <div v-for="i in cards" :key="i" class="sk-card">
          <n-skeleton text style="width: 38%" />
          <n-skeleton text :repeat="3" />
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.page-skeleton {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.sk-metrics,
.sk-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}

.sk-charts,
.sk-review {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.sk-card,
.sk-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 18px;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: var(--card);
}

.sk-panel {
  padding: 20px;
}

.sk-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
}

.sk-table-head,
.sk-table-row {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 14px;
  padding: 10px 0;
  border-bottom: 1px solid var(--border);
}

.sk-table-head {
  padding-top: 0;
}

.sk-table-row:last-child {
  border-bottom: 0;
}

@media (max-width: 900px) {
  .sk-charts,
  .sk-review {
    grid-template-columns: 1fr;
  }
}
</style>
