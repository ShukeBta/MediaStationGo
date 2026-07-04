import { useCallback, useEffect, useState } from 'react'
import toast from 'react-hot-toast'

import { storageAPI, type CloudScanStatus, type StorageType } from '../api/storage_config'
import { apiErrorMessage } from './cloudBrowserModel'

export function useCloudBrowserScan(type: StorageType) {
  const [scanBusy, setScanBusy] = useState(false)
  const [cancelBusy, setCancelBusy] = useState(false)
  const [scanStatuses, setScanStatuses] = useState<CloudScanStatus[]>([])

  const loadScanStatus = useCallback(async () => {
    const r = await storageAPI.cloudScanStatus()
    setScanStatuses((r.items ?? []).filter((item) => !type || item.provider === type))
  }, [type])

  useEffect(() => {
    loadScanStatus().catch(() => undefined)
  }, [loadScanStatus])

  useEffect(() => {
    const timer = window.setInterval(() => {
      loadScanStatus().catch(() => undefined)
    }, 3000)
    return () => window.clearInterval(timer)
  }, [loadScanStatus])

  const scanAllCloudLibraries = async () => {
    setScanBusy(true)
    try {
      const r = await storageAPI.scanAllCloud()
      setScanStatuses(r.items ?? [])
      toast.success(r.message ?? '已开始扫描所有启用的网盘媒体库')
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '启动扫描失败'))
    } finally {
      setScanBusy(false)
    }
  }

  const cancelCloudScans = async () => {
    setCancelBusy(true)
    try {
      const r = await storageAPI.cancelCloudScan('', type)
      toast.success(r.message ?? `已中断 ${r.cancelled} 个扫描任务`)
      await loadScanStatus()
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '中断扫描失败'))
    } finally {
      setCancelBusy(false)
    }
  }

  return {
    cancelBusy,
    cancelCloudScans,
    scanAllCloudLibraries,
    scanBusy,
    scanStatuses,
  }
}
