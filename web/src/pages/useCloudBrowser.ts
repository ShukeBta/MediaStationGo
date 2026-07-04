import { useCallback, useEffect, useState } from 'react'

import { libraryAPI } from '../api/library'
import {
  cloudAPI,
  type CloudEntry,
  type StorageType,
} from '../api/storage_config'
import type { Library } from '../types'
import { apiErrorMessage } from './cloudBrowserModel'
import { cloudLibraryProvider } from './storageConfigModel'
import { useCloudBrowserFileActions } from './useCloudBrowserFileActions'
import { useCloudBrowserMounts } from './useCloudBrowserMounts'
import { useCloudBrowserScan } from './useCloudBrowserScan'

export function useCloudBrowser(type: StorageType, enabled = true) {
  const [stack, setStack] = useState<{ id: string; name: string }[]>([{ id: '', name: '根目录' }])
  const [items, setItems] = useState<CloudEntry[]>([])
  const [mounts, setMounts] = useState<Library[]>([])
  const [loading, setLoading] = useState(false)
  const [mountMediaType, setMountMediaType] = useState('auto')
  const [error, setError] = useState('')

  const cur = stack[stack.length - 1]
  const load = useCallback(async (dir: string) => {
    if (!enabled) {
      setItems([])
      setError('该外部存储未启用，勾选启用并保存后再刷新资源')
      setLoading(false)
      return
    }
    setLoading(true)
    setError('')
    try {
      const r = await cloudAPI.list(type, dir)
      setItems(r.items ?? [])
      if (r.error) setError(r.error)
    } catch (err: unknown) {
      setError(apiErrorMessage(err, '加载失败'))
      setItems([])
    } finally {
      setLoading(false)
    }
  }, [enabled, type])

  const loadMounts = useCallback(async () => {
    const libs = await libraryAPI.list({ includeHidden: true })
    setMounts(libs.filter((lib) => cloudLibraryProvider(lib.path) === type))
  }, [type])

  const scan = useCloudBrowserScan(type)
  const mountActions = useCloudBrowserMounts({
    items,
    loadMounts,
    mountMediaType,
    stack,
    type,
  })
  const fileActions = useCloudBrowserFileActions({
    load,
    loadMounts,
    setLoading,
    stack,
    type,
  })

  useEffect(() => {
    load(cur.id).catch(() => undefined)
  }, [cur.id, load])

  useEffect(() => {
    loadMounts().catch(() => undefined)
  }, [loadMounts])

  const enter = (entry: CloudEntry) => setStack((current) => [...current, { id: entry.id, name: entry.name }])
  const goTo = (index: number) => setStack((current) => current.slice(0, index + 1))
  const goUp = () => setStack((current) => (current.length > 1 ? current.slice(0, -1) : current))

  return {
    batchMounting: mountActions.batchMounting,
    cancelBusy: scan.cancelBusy,
    cancelCloudScans: scan.cancelCloudScans,
    createFolder: fileActions.createFolder,
    enter,
    error,
    goTo,
    goUp,
    hasDirectories: items.some((item) => item.is_dir),
    items,
    loading,
    mountCurrent: mountActions.mountCurrent,
    mountMediaType,
    mounting: mountActions.mounting,
    mounts,
    mountVisibleDirectories: mountActions.mountVisibleDirectories,
    removeMount: mountActions.removeMount,
    renameFolder: fileActions.renameFolder,
    scanAllCloudLibraries: scan.scanAllCloudLibraries,
    scanBusy: scan.scanBusy,
    scanStatuses: scan.scanStatuses,
    setMountMediaType,
    stack,
    doImport: fileActions.doImport,
  }
}
