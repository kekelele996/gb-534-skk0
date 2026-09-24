<script setup lang="ts">
import { computed } from 'vue'
import { Info, TrendingUp } from 'lucide-vue-next'
import type { BatchTrend } from '../../types/batch-trend'
import { fermentationPhases } from '../../types/enums/fermentation-phase'
import BatchTrendChart from './BatchTrendChart.vue'
import DeviationBadge from './DeviationBadge.vue'

const props = defineProps<{ trend: BatchTrend | null; loading: boolean; error: string }>()
const emit = defineEmits<{ select: [analysisId: number] }>()
const phaseLabels: Record<string, string> = { lag: '延滞期', growth: '生长期', production: '产物期', harvest: '收获期' }
const levelColors: Record<string, string> = {
  normal: '#1b533b', watch: '#a9680c', major: '#8b4c0d', critical: '#963d68',
}
const points = computed(() => props.trend?.points ?? [])
const percent = (value?: number) => `${((value ?? 0) * 100).toFixed(1)}%`
</script>

<template>
  <section class="trend-panel">
    <div class="section-heading">
      <div>
        <h2><TrendingUp :size="16" />批间趋势</h2>
        <p>同发酵罐 · 同配方版本 · 同通道，已形成分析结果且未作废的批次</p>
      </div>
    </div>
    <el-skeleton v-if="loading" :rows="5" animated />
    <el-alert v-else-if="error" :title="error" type="error" :closable="false" show-icon />
    <template v-else-if="trend">
      <div class="token-list trend-context">
        <span>{{ trend.context.vessel_code }}</span>
        <span>{{ trend.context.recipe_code }} · v{{ trend.context.recipe_version }}</span>
        <span>通道 {{ trend.context.channel }}</span>
        <span>{{ trend.points.length }} 批可比较</span>
      </div>
      <el-alert v-if="!trend.comparable" class="trend-insufficient" type="info" :closable="false" show-icon>
        <template #icon><Info :size="16" /></template>
        <template #title>当前条件下仅有 {{ trend.points.length }} 批有效结果，还不足以判断趋势（至少需要 2 批）。</template>
        在同一发酵罐、同一配方版本和同一通道下再产生一批已完成且未作废的分析后，总体偏差与各阶段加权偏差将按时间展示；可点击批次切换查看它的原始证据。
      </el-alert>
      <template v-else>
        <p class="trend-hint muted">深色连线为总体偏差，虚线为各阶段加权偏差；点击图上批次或下方行可切换到该批的原始证据。当前结果以加粗描边标记。</p>
        <BatchTrendChart :trend="trend" @select="emit('select', $event)" />
        <div class="trend-rows">
          <button v-for="point in points" :key="point.analysis_id" class="trend-row"
            :class="{ selected: point.analysis_id === trend.anchor_analysis_id }"
            @click="emit('select', point.analysis_id)">
            <span class="primary-cell">
              <strong>{{ point.run_code }}</strong>
              <small>{{ new Date(point.analyzed_at).toLocaleString() }}</small>
            </span>
            <span class="trend-overall numeric" :style="{ color: levelColors[point.deviation_level] ?? '#17221d' }">
              {{ percent(point.overall_deviation) }}
            </span>
            <DeviationBadge :level="point.deviation_level" />
            <span class="trend-phases">
              <span v-for="phase in fermentationPhases" :key="phase">
                <i>{{ phaseLabels[phase] }}</i>{{ percent(point.phase_deviations[phase]) }}
              </span>
            </span>
          </button>
        </div>
      </template>
    </template>
  </section>
</template>
