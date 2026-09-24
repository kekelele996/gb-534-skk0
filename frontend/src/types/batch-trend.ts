import type { DeviationLevel } from './enums/deviation-level'
import type { AnalysisState } from './deviation-analysis'

// Shared vessel + recipe version + channel cohort used to compare batches.
export interface BatchTrendContext {
  vessel_id: number
  vessel_code: string
  recipe_id: number
  recipe_code: string
  recipe_version: number
  channel: string
}

export interface BatchTrendPoint {
  analysis_id: number
  sensor_series_id: number
  run_code: string
  analyzed_at: string
  overall_deviation: number
  deviation_level: DeviationLevel
  analysis_state: AnalysisState
  phase_deviations: Record<string, number>
}

export interface BatchTrend {
  context: BatchTrendContext
  anchor_analysis_id: number
  points: BatchTrendPoint[]
  comparable: boolean
}
