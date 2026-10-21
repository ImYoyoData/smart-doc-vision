<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts'
import type { EChartsOption } from 'echarts'

const props = withDefaults(
  defineProps<{
    option: EChartsOption
    height?: string
    loading?: boolean
  }>(),
  { height: '280px', loading: false },
)

const container = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null
let observer: ResizeObserver | null = null

function render() {
  if (!chart) return
  chart.setOption(props.option, true)
}

onMounted(() => {
  if (!container.value) return
  chart = echarts.init(container.value, undefined, { renderer: 'canvas' })
  render()
  if (props.loading) chart.showLoading('default', { color: '#2b6cf6', maskColor: 'rgba(255,255,255,.65)' })
  observer = new ResizeObserver(() => chart?.resize())
  observer.observe(container.value)
})

watch(
  () => props.option,
  () => render(),
  { deep: true },
)

watch(
  () => props.loading,
  (value) => {
    if (!chart) return
    if (value) chart.showLoading('default', { color: '#2b6cf6', maskColor: 'rgba(255,255,255,.65)' })
    else chart.hideLoading()
  },
)

onBeforeUnmount(() => {
  observer?.disconnect()
  chart?.dispose()
  chart = null
})
</script>

<template>
  <div ref="container" class="echart" :style="{ height }"></div>
</template>

<style scoped>
.echart {
  width: 100%;
  min-height: 180px;
}
</style>
