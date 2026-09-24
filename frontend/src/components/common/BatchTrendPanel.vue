<script setup lang="ts">
import * as echarts from 'echarts'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Info } from 'lucide-vue-next'
import DeviationBadge from './DeviationBadge.vue'
import type { DeviationTrend, DeviationTrendPoint } from '../../types/deviation-analysis'

const props = defineProps<{ trend: DeviationTrend | null; loading?: boolean }>()
const emit = defineEmits<{ select: [point: DeviationTrendPoint] }>()

const chartEl = ref<HTMLDivElement>()
let chart: echarts.ECharts | undefined
let observer: ResizeObserver | undefined

const phaseOrder = ['lag', 'growth', 'production', 'harvest']
const phaseLabels: Record<string, string> = { lag: '延滞期', growth: '生长期', production: '产物期', harvest: '收获期' }
const phaseColors: Record<string, string> = {
  lag: '#8a978f', growth: '#25714f', production: '#237a78', harvest: '#35689a',
}
const levelColors: Record<string, string> = {
  normal: '#25714f', watch: '#a9680c', major: '#8b4c0d', critical: '#963d68',
}
const items = computed(() => props.trend?.items ?? [])
const currentID = computed(() => props.trend?.current_point?.id ?? props.trend?.anchor_id)

function scoreFor(point: DeviationTrendPoint, phase: string): number | null {
  const score = point.phase_scores_json?.find((entry) => entry.phase === phase)
  return score ? score.weighted_deviation : null
}
function percent(value: number | null | undefined): string {
  return value === null || value === undefined ? '—' : `${(value * 100).toFixed(1)}%`
}
function axisLabel(point: DeviationTrendPoint): string {
  return `${point.run_code}\n${new Date(point.analyzed_at).toLocaleDateString()}`
}
function option(): echarts.EChartsOption {
  const overallData = items.value.map((point) => ({
    value: point.overall_score,
    itemStyle: { color: levelColors[point.deviation_level] ?? '#17221d' },
    symbolSize: point.id === currentID.value ? 12 : 7,
    lineStyle: { width: 2.6, color: '#17221d' },
  }))
  const phaseSeries: echarts.SeriesOption[] = phaseOrder.map((phase) => ({
    name: phaseLabels[phase], type: 'line', showSymbol: false, connectNulls: true,
    data: items.value.map((point) => scoreFor(point, phase)),
    lineStyle: { width: 1.4, type: 'dashed', color: phaseColors[phase], opacity: 0.85 },
    itemStyle: { color: phaseColors[phase] },
  }))
  return {
    animationDuration: 200,
    grid: { left: 46, right: 26, top: 58, bottom: 56 },
    legend: { top: 6, left: 6, itemWidth: 16, itemHeight: 8, textStyle: { color: '#52615a', fontSize: 11 } },
    tooltip: {
      trigger: 'axis',
      formatter: (raw: unknown) => {
        const rows = raw as Array<{ dataIndex: number; marker: string; seriesName: string; value: number | null }>
        if (!rows.length) return ''
        const point = items.value[rows[0].dataIndex]
        const lines = [
          `<strong>${point.run_code}</strong>`,
          `<span style="color:#617069">${new Date(point.analyzed_at).toLocaleString()}</span>`,
          ...rows.map((row) => `${row.marker}${row.seriesName}：${percent(row.value)}`),
        ]
        return lines.join('<br/>')
      },
    },
    xAxis: {
      type: 'category', data: items.value.map(axisLabel), boundaryGap: true,
      axisLine: { lineStyle: { color: '#9ca9a2' } },
      axisLabel: { color: '#52615a', fontSize: 10, interval: 0, lineHeight: 14 },
    },
    yAxis: {
      type: 'value', min: 0, max: 1, interval: 0.2,
      axisLabel: { color: '#617069', formatter: (value: number) => `${value * 100}%` },
      splitLine: { lineStyle: { color: '#e0e7e2' } },
    },
    series: [
      {
        name: '总体加权偏差', type: 'line', data: overallData, z: 5,
        label: {
          show: true, position: 'top', fontSize: 10, fontWeight: 700, color: '#17221d',
          formatter: (params: { value?: unknown }) => percent(typeof params.value === 'number' ? params.value : null),
        },
        markLine: {
          silent: true, symbol: 'none',
          data: [
            { yAxis: 0.4, lineStyle: { color: '#d6b77e', type: 'dashed', width: 1 }, label: { formatter: '重大', color: '#a9680c', fontSize: 9, position: 'insideEndTop' } },
            { yAxis: 0.65, lineStyle: { color: '#c691ab', type: 'dashed', width: 1 }, label: { formatter: '严重', color: '#963d68', fontSize: 9, position: 'insideEndTop' } },
          ],
        },
      } satisfies echarts.SeriesOption,
      ...phaseSeries,
    ],
  }
}
async function render() {
  await nextTick()
  if (!chartEl.value || !items.value.length) { chart?.clear(); return }
  if (!chart) {
    chart = echarts.init(chartEl.value)
    chart.on('click', handleChartClick)
    observer = new ResizeObserver(() => chart?.resize())
    observer.observe(chartEl.value)
  }
  chart.setOption(option(), true)
}
function handleChartClick(raw: unknown) {
  const params = raw as { componentType?: string; dataIndex?: number }
  if (params.componentType !== 'series' || params.dataIndex === undefined) return
  const point = items.value[params.dataIndex]
  if (point && point.id !== currentID.value) emit('select', point)
}
watch(() => props.trend, render, { deep: true })
onMounted(render)
onBeforeUnmount(() => { observer?.disconnect(); chart?.dispose() })
</script>

<template>
  <section class="trend-panel">
    <div class="trend-heading">
      <div>
        <p class="eyebrow">BATCH-TO-BATCH TREND</p>
        <h3>批间趋势 · 同罐 / 同配方版本 / 同通道</h3>
      </div>
      <div v-if="trend" class="trend-context">
        <span>{{ trend.vessel_code }}</span>
        <span>{{ trend.recipe_code }} v{{ trend.recipe_version }}</span>
        <span>{{ trend.channel }}</span>
      </div>
    </div>
    <el-skeleton v-if="loading" :rows="4" animated />
    <template v-else-if="trend">
      <div v-if="!trend.comparable" class="trend-insufficient" role="status">
        <Info :size="16" />
        <div>
          <strong>可比结果不足两批，尚不足以判断批间趋势</strong>
          <p>当前发酵罐、配方版本与通道下，仅汇总到已形成分析结果且未作废的记录不足两批；待更多批次完成分析后，将在此按时间展示总体偏差与各阶段加权偏差。</p>
        </div>
      </div>
      <div v-if="items.length" class="trend-body">
        <div ref="chartEl" class="trend-chart" />
        <div class="trend-batches">
          <button
            v-for="point in items" :key="point.id" type="button" class="trend-batch"
            :class="{ current: point.id === currentID }" @click="point.id !== currentID && emit('select', point)"
          >
            <span class="trend-batch-main">
              <strong>{{ point.run_code }}</strong>
              <small>{{ new Date(point.analyzed_at).toLocaleString() }}</small>
            </span>
            <span class="numeric">{{ percent(point.overall_score) }}</span>
            <DeviationBadge :level="point.deviation_level" />
          </button>
          <p class="trend-hint"><Info :size="13" />点击图上数据点或批次可切换到该批原始证据</p>
        </div>
      </div>
    </template>
    <div v-else class="trend-insufficient error" role="alert">
      <Info :size="16" />
      <div><strong>批间趋势暂不可用</strong><p>趋势数据加载失败，请稍后重试。</p></div>
    </div>
  </section>
</template>
