import { useCallback } from 'react'
import toast from 'react-hot-toast'

import { useWebSocket } from '../hooks/useWebSocket'
import { useAuthStore } from '../stores/auth'

// GlobalEvents subscribes to the WS hub and surfaces interesting events
// as toasts. Lives at the top of the component tree so every page sees
// the same stream without re-opening connections.
export function GlobalEvents() {
  const role = useAuthStore((state) => state.user?.role)
  const onEvent = useCallback((topic: string, payload: unknown) => {
    if (!payload || typeof payload !== 'object') return
    const p = payload as Record<string, unknown>
    if (topic === 'scan') {
      if (role !== 'admin') return
      const id = `scan-${String(p.library_id ?? 'global')}`
      if (p.error) {
        toast.error(`扫描失败：${String(p.error)}`, { id })
        return
      }
      if (p.finished) {
        const elapsed = Number(p.elapsed_seconds ?? p.elapsed ?? 0)
        const elapsedText = elapsed > 0 ? ` · 耗时 ${formatDuration(elapsed)}` : ''
        toast.success(`扫描完成：发现 ${p.discovered ?? p.visited ?? 0} · 新增 ${p.added ?? 0} · 更新 ${p.updated ?? 0} · 跳过 ${p.skipped ?? 0}${elapsedText}`, { id })
        return
      }
      if (p.queued) {
        toast.loading(String(p.message ?? '云盘扫描已加入后台队列，会自动入库'), { id })
        return
      }
      if (p.cloud && p.stage) {
        const stage = p.stage === 'importing' ? '正在入库' : '正在遍历目录'
        const speed = Number(p.files_per_second ?? 0)
        const speedText = speed > 0 ? ` · ${speed.toFixed(speed >= 10 ? 0 : 1)} 个/秒` : ''
        toast.loading(`${stage}：目录 ${p.dirs ?? 0} · 已发现 ${p.discovered ?? 0} · 已入库 ${p.visited ?? 0}${speedText}`, { id })
      }
    }
    if (topic === 'scrape' && p.finished) {
      const processed = Number(p.processed ?? 0)
      const matched = Number(p.matched ?? 0)
      const failed = Number(p.failed ?? 0)
      if (failed > 0) {
        toast.error(`刮削完成但有错误：处理 ${processed} 项 · 成功匹配 ${matched} 项 · 失败 ${failed} 项`)
        return
      }
      if (processed > 0) {
        toast.success(`刮削完成：处理 ${processed} 项 · 成功匹配 ${matched} 项`)
      } else {
        toast.success(`刮削完成：没有待刮削媒体 · 成功匹配 ${matched} 项`)
      }
    }
    if (topic === 'subscription') {
      const queued = (p.queued as number | undefined) ?? 0
      if (queued > 0) toast.success(`订阅「${p.name}」已加入 ${queued} 项下载`)
    }
  }, [role])

  useWebSocket(onEvent)
  return null
}

function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return ''
  if (seconds < 60) return `${Math.round(seconds)}秒`
  const minutes = Math.floor(seconds / 60)
  const rest = Math.round(seconds % 60)
  if (minutes < 60) return `${minutes}分${rest}秒`
  const hours = Math.floor(minutes / 60)
  return `${hours}小时${minutes % 60}分`
}
