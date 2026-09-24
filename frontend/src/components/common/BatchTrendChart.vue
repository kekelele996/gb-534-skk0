<script setup lang="ts">
import * as echarts from 'echarts'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { BatchTrend } from '../../types/batch-trend'
import { fermentationPhases } from '../../types/enums/fermentation-phase'

const props = withDefaults(defineProps<{ trend: BatchTrend; height?: number }>(), { height: 300 })
const emit = defineEmits<{ select: [analysisId: number] }>()
const chartEl = ref<HTMLDivElement>()
let chart: echarts.ECharts | undefined
let observer: ResizeObserver | undefined
const phaseLabels: Record<string, string> = { lag: '延滞期', growth: '生长期', production: '产物期', harvest: '收获期' }
const phaseColors: Record<string, string> = {
  lag: '#526a5d', growth: '#1b533b', production: '#1e6663', harvest: '#315d88',
}
const levelColors: Record<string, string> = {
  normal: '#25714f', watch: '#a9680c', major: '#8b4c0d', critical: '#963d68',
}
const points = computed(() => props.trend.points)
const percent = (value: number) => `${(value * 100).toFixed(1)}%`

function option(): echarts.EChartsOption {
  const series: echarts.SeriesOption[] = [{
    name: '总体偏差', type: 'line', symbolSize: 9, z: 5,
    lineStyle: { width: 3, color: '#17221d' }, itemStyle: { color: '#17221d' },
    data: points.value.map((point) => ({
      value: point.overall_deviation,
      itemStyle: {
        color: levelColors[point.deviation_level] ?? '#17221d',
        borderColor: point.analysis_id === props.trend.anchor_analysis_id ? '#17221d' : 'transparent',
        borderWidth: 2,
      },
    })),
  }]
  fermentationPhases.forEach((phase) => {
    series.push({
      name: phaseLabels[phase] ?? phase, type: 'line', symbol: 'circle', symbolSize: 5,
      lineStyle: { width: 1.6, color: phaseColors[phase], type: 'dashed' },
      itemStyle: { color: phaseColors[phase] },
      data: points.value.map((point) => point.phase_deviations[phase] ?? null),
      connectNulls: false,
    })
  })
  return {
    animationDuration: 240,
    grid: { left: 48, right: 24, top: 50, bottom: 64 },
    legend: { top: 4, left: 6, type: 'scroll', textStyle: { color: '#52615a', fontSize: 11 } },
    tooltip: {
      trigger: 'axis',
      formatter: (raw) => {
        const rows = Array.isArray(raw) ? raw : [raw]
        const index = rows[0]?.dataIndex ?? 0
        const point = points.value[index]
        if (!point) return ''
        const lines = [
          `<strong>${point.run_code}</strong>`,
          new Date(point.analyzed_at).toLocaleString(),
          ...rows
            .filter((row) => typeof row.value === 'number')
            .map((row) => `${row.marker}${row.seriesName}：${percent(row.value as number)}`),
        ]
        return lines.join('<br/>')
      },
    },
    xAxis: {
      type: 'category',
      data: points.value.map((point) => point.run_code),
      axisLabel: {
        color: '#52615a', fontSize: 10, interval: 0,
        formatter: (value: string) => (value.length > 14 ? `${value.slice(0, 13)}…` : value),
      },
      axisLine: { lineStyle: { color: '#9ca9a2' } },
      axisTick: { alignWithLabel: true },
    },
    yAxis: {
      type: 'value', min: 0, max: 1,
      axisLabel: { color: '#52615a', formatter: (value: number) => `${Math.round(value * 100)}%` },
      splitLine: { lineStyle: { color: '#e0e7e2' } },
    },
    series,
  }
}
async function render() {
  await nextTick()
  if (!chartEl.value || points.value.length === 0) return
  chart ??= echarts.init(chartEl.value)
  chart.setOption(option(), true)
}
watch(() => props.trend, render, { deep: true })
onMounted(() => {
  render()
  chart?.on('click', (event) => {
    const index = (event as { dataIndex?: number }).dataIndex
    const point = points.value[Number(index)]
    if (point) emit('select', point.analysis_id)
  })
  observer = new ResizeObserver(() => chart?.resize())
  if (chartEl.value) observer.observe(chartEl.value)
})
onBeforeUnmount(() => { observer?.disconnect(); chart?.dispose() })
</script>

<template>
  <div class="chart-frame" :style="{ height: `${height}px` }">
    <div ref="chartEl" class="chart-canvas" />
  </div>
</template>
