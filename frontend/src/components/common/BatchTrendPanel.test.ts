import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import BatchTrendPanel from './BatchTrendPanel.vue'
import type { BatchTrend, BatchTrendPoint } from '../../types/batch-trend'
import type { DeviationLevel } from '../../types/enums/deviation-level'

const stubs = {
  'el-skeleton': { template: '<div class="skeleton-stub" />' },
  'el-alert': { template: '<div class="alert-stub"><slot name="title" /><slot /></div>' },
  BatchTrendChart: { props: ['trend'], template: '<div class="chart-stub" />' },
  DeviationBadge: { props: ['level'], template: '<span class="badge-stub">{{ level }}</span>' },
}

function buildTrend(count: number): BatchTrend {
  return {
    context: {
      vessel_id: 3, vessel_code: 'FV-201', recipe_id: 7, recipe_code: 'YEAST-FEDBATCH-A',
      recipe_version: 1, channel: 'multichannel',
    },
    anchor_analysis_id: 100 + count - 1,
    comparable: count >= 2,
    points: Array.from({ length: count }, (_, index) => {
      const level: DeviationLevel = index === 0 ? 'normal' : index === 1 ? 'watch' : 'major'
      const point: BatchTrendPoint = {
        analysis_id: 100 + index,
        sensor_series_id: 200 + index,
        run_code: `RUN-2026-000${index}`,
        analyzed_at: new Date(Date.UTC(2026, 8, 1 + index)).toISOString(),
        overall_deviation: 0.1 + index * 0.15,
        deviation_level: level,
        analysis_state: 'completed',
        phase_deviations: { lag: 0.05, growth: 0.1, production: 0.2, harvest: 0.15 },
      }
      return point
    }),
  }
}

describe('BatchTrendPanel', () => {
  it('explains that a single comparable batch is insufficient for trend judgement', () => {
    const wrapper = mount(BatchTrendPanel, { props: { trend: buildTrend(1), loading: false, error: '' }, global: { stubs } })
    expect(wrapper.text()).toContain('还不足以判断趋势')
    expect(wrapper.text()).toContain('至少需要 2 批')
    expect(wrapper.text()).toContain('FV-201')
    expect(wrapper.text()).toContain('YEAST-FEDBATCH-A · v1')
    expect(wrapper.text()).toContain('multichannel')
    expect(wrapper.find('.chart-stub').exists()).toBe(false)
  })

  it('renders chronological batches and emits the selected analysis for evidence switching', async () => {
    const wrapper = mount(BatchTrendPanel, { props: { trend: buildTrend(3), loading: false, error: '' }, global: { stubs } })
    expect(wrapper.find('.chart-stub').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('还不足以判断趋势')
    const rows = wrapper.findAll('.trend-row')
    expect(rows).toHaveLength(3)
    expect(rows[0].text()).toContain('RUN-2026-0000')
    expect(rows[2].classes()).toContain('selected')
    await rows[1].trigger('click')
    const emitted = wrapper.emitted('select')
    expect(emitted?.[0]).toEqual([101])
  })
})
