import { useCallback, useEffect, useState } from 'react'
import toast from 'react-hot-toast'

import { libraryAPI } from '../api/library'
import { storageAPI } from '../api/storage_config'
import { useWebSocket } from '../hooks/useWebSocket'
import { formatCloudScanStatus, formatDuration } from './libraryPageModel'

type UseLibraryScanStatusInput = {
  libraryID: string
  isAdmin: boolean
  onLibraryChanged: () => void
}

export function useLibraryScanStatus({
  libraryID,
  isAdmin,
  onLibraryChanged,
}: UseLibraryScanStatusInput) {
  const [scanning, setScanning] = useState(false)
  const [scanProgress, setScanProgress] = useState('')

  const onRealtimeEvent = useCallback((topic: string, payload: unknown) => {
    if (!isAdmin) return
    if (topic !== 'scan' || !payload || typeof payload !== 'object') return
    const event = payload as Record<string, unknown>
    if (event.library_id !== libraryID) return
    if (event.error) {
      setScanning(false)
      setScanProgress(`扫描失败：${String(event.error)}`)
      return
    }
    if (event.finished) {
      setScanning(false)
      setScanProgress(finishedScanMessage(event))
      onLibraryChanged()
      return
    }
    if (event.queued) {
      setScanning(true)
      setScanProgress(String(event.message ?? '扫描已排队，后台会自动入库'))
      return
    }
    if (event.cloud && event.stage) {
      setScanning(true)
      setScanProgress(runningCloudScanMessage(event))
    }
  }, [isAdmin, libraryID, onLibraryChanged])

  useWebSocket(onRealtimeEvent)

  useEffect(() => {
    if (!isAdmin || !libraryID) return
    let cancelled = false
    let terminal = false
    let timer: number | undefined
    const restoreCloudScanStatus = async () => {
      const response = await storageAPI.cloudScanStatus()
      if (cancelled) return
      const status = (response.items ?? []).find((item) => item.library_id === libraryID)
      if (!status) return
      if (status.state === 'running' || status.state === 'queued' || status.state === 'canceling') {
        setScanning(true)
        setScanProgress(formatCloudScanStatus(status))
        return
      }
      if (status.state === 'finished') {
        setScanning(false)
        setScanProgress(formatCloudScanStatus(status))
        onLibraryChanged()
        terminal = true
        if (timer) window.clearInterval(timer)
        return
      }
      if (status.state === 'error' && status.error) {
        setScanning(false)
        setScanProgress(`扫描失败：${status.error}`)
        terminal = true
        if (timer) window.clearInterval(timer)
      }
    }
    restoreCloudScanStatus()
      .catch(() => undefined)
      .finally(() => {
        if (!cancelled && !terminal) {
          timer = window.setInterval(() => {
            restoreCloudScanStatus().catch(() => undefined)
          }, 5000)
        }
      })
    return () => {
      cancelled = true
      if (timer) window.clearInterval(timer)
    }
  }, [isAdmin, libraryID, onLibraryChanged])

  const handleScan = useCallback(async () => {
    setScanning(true)
    setScanProgress('正在提交扫描任务…')
    let keepScanning = false
    try {
      const result = await libraryAPI.scan(libraryID)
      if (result.queued) {
        keepScanning = true
        setScanProgress(`${result.message ?? '云盘扫描已在后台运行，发现的媒体会自动加入当前媒体库'}；${result.estimate_message ?? '大目录耗时取决于网盘接口速度'}`)
        toast.success('云盘扫描已加入后台队列')
      } else {
        toast.success(`扫描完成:新增 ${result.added} 项，更新 ${result.updated ?? 0} 项`)
        setScanProgress(`扫描完成：新增 ${result.added} · 更新 ${result.updated ?? 0}`)
        onLibraryChanged()
        setScanning(false)
      }
    } catch {
      toast.error('扫描失败')
      setScanProgress('扫描失败，请查看日志或稍后重试')
      setScanning(false)
    } finally {
      if (!keepScanning) setScanning(false)
    }
  }, [libraryID, onLibraryChanged])

  return {
    scanning,
    scanProgress,
    handleScan,
  }
}

function finishedScanMessage(event: Record<string, unknown>) {
  const elapsed = Number(event.elapsed_seconds ?? event.elapsed ?? 0)
  const elapsedText = elapsed > 0 ? ` · 耗时 ${formatDuration(elapsed)}` : ''
  return `扫描完成：发现 ${event.discovered ?? event.visited ?? 0} · 新增 ${event.added ?? 0} · 更新 ${event.updated ?? 0} · 跳过 ${event.skipped ?? 0}${elapsedText}`
}

function runningCloudScanMessage(event: Record<string, unknown>) {
  const stage = event.stage === 'importing' ? '正在入库' : '正在遍历目录'
  const speed = Number(event.files_per_second ?? 0)
  const speedText = speed > 0 ? ` · ${speed.toFixed(speed >= 10 ? 0 : 1)} 个/秒` : ''
  return `${stage}：目录 ${event.dirs ?? 0} · 已发现 ${event.discovered ?? 0} · 已入库 ${event.visited ?? 0}${speedText}`
}
