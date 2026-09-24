import { api, json, query } from './client'
import type { Page, ListQuery } from '../types/common'
import type { AnalysisState, DeviationAnalysis } from '../types/deviation-analysis'
import type { BatchTrend } from '../types/batch-trend'

export const listAnalyses = (params: ListQuery & { state?: string; deviation_level?: string } = {}) =>
  api<Page<DeviationAnalysis>>(`/deviation-analyses${query(params)}`)
export const getAnalysis = (id: number) =>
  api<DeviationAnalysis>(`/deviation-analyses/${id}`)
export const getBatchTrend = (id: number) =>
  api<BatchTrend>(`/deviation-analyses/${id}/trend`)
export const runAnalysis = (sensorSeriesId: number, idempotencyKey: string) =>
  api<DeviationAnalysis>('/deviation-analyses', json('POST', { sensor_series_id: sensorSeriesId }, { 'Idempotency-Key': idempotencyKey }))
export const transitionAnalysis = (id: number, toState: AnalysisState, comment = '') =>
  api<DeviationAnalysis>(`/deviation-analyses/${id}/transition`, json('POST', { to_state: toState, comment }))
export const replayAnalysis = (id: number) =>
  api<DeviationAnalysis>(`/deviation-analyses/${id}/replay`, json('POST'))
