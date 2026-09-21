// 图表 hook：echarts 实例生命周期 + 容器尺寸自适应（Dashboard 复用）。
import { onBeforeUnmount, onMounted, ref } from 'vue'
import * as echarts from 'echarts/core'

export function useChart(getOption: () => echarts.EChartsCoreOption) {
  const el = ref<HTMLElement>()
  let chart: echarts.ECharts | null = null
  let observer: ResizeObserver | null = null

  function render() {
    if (!el.value) return
    chart = chart ?? echarts.init(el.value)
    chart.setOption(getOption())
  }

  function resize() {
    chart?.resize()
  }

  onMounted(() => {
    render()
    window.addEventListener('resize', resize)
    if (el.value) {
      observer = new ResizeObserver(() => chart?.resize())
      observer.observe(el.value)
    }
  })

  onBeforeUnmount(() => {
    window.removeEventListener('resize', resize)
    observer?.disconnect()
    observer = null
    chart?.dispose()
    chart = null
  })

  return { el, render, resize }
}
