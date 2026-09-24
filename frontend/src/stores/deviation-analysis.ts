import { ref } from 'vue'
import { defineStore } from 'pinia'
import { getAnalysis, getBatchTrend, listAnalyses, replayAnalysis, runAnalysis, transitionAnalysis } from '../api/deviation-analysis'
import { errorMessage } from '../api/client'
import type { BatchTrend } from '../types/batch-trend'
import type { AnalysisState, DeviationAnalysis } from '../types/deviation-analysis'

export const useAnalysisStore = defineStore('deviation-analyses', () => {
  const items = ref<DeviationAnalysis[]>([])
  const selected = ref<DeviationAnalysis | null>(null)
  const loading = ref(false)
  const running = ref(false)
  const error = ref('')
  const trend = ref<BatchTrend | null>(null)
  const trendLoading = ref(false)
  const trendError = ref('')
  async function load() {
    loading.value = true; error.value = ''
    try {
      items.value = (await listAnalyses({ page_size: 100 })).items
      if (!selected.value || !items.value.some((item) => item.id === selected.value?.id)) selected.value = items.value[0] ?? null
      else selected.value = items.value.find((item) => item.id === selected.value?.id) ?? null
    } catch (cause) { error.value = errorMessage(cause) }
    finally { loading.value = false }
  }
  async function select(id: number) {
    selected.value = await getAnalysis(id)
    if (!items.value.some((item) => item.id === id)) items.value.unshift(selected.value)
    await loadTrend()
  }
  async function loadTrend() {
    if (!selected.value) { trend.value = null; return }
    trendLoading.value = true; trendError.value = ''
    try { trend.value = await getBatchTrend(selected.value.id) }
    catch (cause) { trend.value = null; trendError.value = errorMessage(cause) }
    finally { trendLoading.value = false }
  }
  async function run(seriesId: number, key: string) {
    running.value = true
    try { selected.value = await runAnalysis(seriesId, key); await load() }
    finally { running.value = false }
  }
  async function transition(state: AnalysisState, comment = '') {
    if (!selected.value) return
    selected.value = await transitionAnalysis(selected.value.id, state, comment)
    await load()
    await loadTrend()
  }
  async function replay() {
    if (!selected.value) return
    selected.value = await replayAnalysis(selected.value.id)
    await load()
  }
  return {
    items, selected, loading, running, error, trend, trendLoading, trendError,
    load, select, loadTrend, run, transition, replay,
  }
})
