import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

const mocks = vi.hoisted(() => {
  const setOption = vi.fn()
  const on = vi.fn()
  const init = vi.fn(() => ({ setOption, on, resize: vi.fn(), clear: vi.fn(), dispose: vi.fn() }))
  return { setOption, on, init }
})

vi.mock('echarts', () => ({ init: mocks.init }))

import BatchTrendPanel from './BatchTrendPanel.vue'
import type { DeviationTrend, DeviationTrendPoint } from '../../types/deviation-analysis'

function point(id: number, runCode: string, overall: number, analyzedAt: string): DeviationTrendPoint {
  return {
    id, sensor_series_id: id + 10, run_code: runCode, vessel_id: 1, vessel_code: 'FV-201',
    recipe_id: 1, recipe_code: 'YEAST-FEDBATCH-A', recipe_version: 1, channel: 'multichannel',
    overall_score: overall, deviation_level: 'watch', analysis_state: 'completed',
    phase_scores_json: [
      {
        phase: 'lag', duration_deviation: 0, slope_deviation: 0, peak_time_deviation: 0,
        curve_distance: 0, weighted_deviation: overall * 0.8, channel_scores: {}, observed_points: 0,
      },
      {
        phase: 'growth', duration_deviation: 0, slope_deviation: 0, peak_time_deviation: 0,
        curve_distance: 0, weighted_deviation: overall, channel_scores: {}, observed_points: 0,
      },
      {
        phase: 'production', duration_deviation: 0, slope_deviation: 0, peak_time_deviation: 0,
        curve_distance: 0, weighted_deviation: overall * 1.1, channel_scores: {}, observed_points: 0,
      },
      {
        phase: 'harvest', duration_deviation: 0, slope_deviation: 0, peak_time_deviation: 0,
        curve_distance: 0, weighted_deviation: overall * 0.9, channel_scores: {}, observed_points: 0,
      },
    ],
    analyzed_at: analyzedAt,
  }
}

describe('BatchTrendPanel', () => {
  beforeEach(() => { vi.clearAllMocks() })
  it('explains that fewer than two comparable batches cannot show a trend', () => {
    const trend: DeviationTrend = {
      anchor_id: 5, vessel_id: 1, vessel_code: 'FV-201', recipe_id: 1,
      recipe_code: 'YEAST-FEDBATCH-A', recipe_version: 1, channel: 'multichannel',
      comparable: false, items: [point(5, 'RUN-ONLY', 0.2, '2026-09-20T00:00:00Z')],
    }
    const wrapper = mount(BatchTrendPanel, { props: { trend } })
    expect(wrapper.text()).toContain('可比结果不足两批，尚不足以判断批间趋势')
    expect(wrapper.findAll('.trend-batch')).toHaveLength(1)
    expect(mocks.init).not.toHaveBeenCalled()
  })

  it('renders comparable batches, marks the anchor and emits select for other batches', async () => {
    const trend: DeviationTrend = {
      anchor_id: 7, vessel_id: 1, vessel_code: 'FV-201', recipe_id: 1,
      recipe_code: 'YEAST-FEDBATCH-A', recipe_version: 1, channel: 'multichannel',
      comparable: true,
      items: [
        point(6, 'RUN-0815', 0.205, '2026-09-15T00:00:00Z'),
        point(7, 'RUN-0819', 0.298, '2026-09-19T00:00:00Z'),
        point(8, 'RUN-0821', 0.388, '2026-09-21T00:00:00Z'),
      ],
      current_point: undefined,
    }
    trend.current_point = trend.items[1]
    const wrapper = mount(BatchTrendPanel, { props: { trend } })
    await Promise.resolve()
    expect(wrapper.findAll('.trend-batch')).toHaveLength(3)
    const rows = wrapper.findAll('.trend-batch')
    expect(rows[1].classes()).toContain('current')
    expect(wrapper.text()).toContain('FV-201')
    expect(wrapper.text()).toContain('YEAST-FEDBATCH-A v1')
    expect(wrapper.text()).toContain('multichannel')
    await rows[0].trigger('click')
    const emitted = wrapper.emitted('select')
    expect(emitted).toBeTruthy()
    expect(emitted![0][0]).toMatchObject({ id: 6, run_code: 'RUN-0815' })
  })
})
