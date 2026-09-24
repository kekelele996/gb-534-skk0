import { ref } from 'vue'
import { defineStore } from 'pinia'
import {
  getAnalysis,
  getAnalysisTrend,
  listAnalyses,
  replayAnalysis,
  runAnalysis,
  transitionAnalysis,
} from '../api/deviation-analysis'
import { errorMessage } from '../api/client'
import type { AnalysisState, DeviationAnalysis, DeviationTrend } from '../types/deviation-analysis'

export const useAnalysisStore = defineStore('deviation-analyses', () => {
  const items = ref<DeviationAnalysis[]>([])
  const selected = ref<DeviationAnalysis | null>(null)
  const loading = ref(false)
  const running = ref(false)
  const error = ref('')
  const trend = ref<DeviationTrend | null>(null)
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
  async function selectById(id: number) {
    try {
      selected.value = await getAnalysis(id)
      error.value = ''
    } catch (cause) { error.value = errorMessage(cause) }
  }
  async function loadTrend() {
    if (!selected.value) { trend.value = null; return }
    trendLoading.value = true; trendError.value = ''
    try { trend.value = await getAnalysisTrend(selected.value.id) }
    catch (cause) { trend.value = null; trendError.value = errorMessage(cause) }
    finally { trendLoading.value = false }
  }
  async function run(seriesId: number, key: string) {
    running.value = true
    try { selected.value = await runAnalysis(seriesId, key); await load(); await loadTrend() }
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
    load, selectById, loadTrend, run, transition, replay,
  }
})
