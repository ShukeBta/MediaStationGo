import { useState } from 'react'
import toast from 'react-hot-toast'

import { cloudAPI, type CloudEntry, type StorageType } from '../api/storage_config'
import { libraryAPI } from '../api/library'
import { confirmAction } from '../components/confirmAction'
import type { Library } from '../types'
import {
  TYPE_LABEL,
  cloudMountDisplayPath,
} from './storageConfigModel'
import { apiErrorMessage, showCloudMountResult } from './cloudBrowserModel'

type CloudStackItem = { id: string; name: string }

type UseCloudBrowserMountsOptions = {
  items: CloudEntry[]
  loadMounts: () => Promise<void>
  mountMediaType: string
  stack: CloudStackItem[]
  type: StorageType
}

export function useCloudBrowserMounts({
  items,
  loadMounts,
  mountMediaType,
  stack,
  type,
}: UseCloudBrowserMountsOptions) {
  const [mounting, setMounting] = useState(false)
  const [batchMounting, setBatchMounting] = useState(false)

  const cur = stack[stack.length - 1]
  const currentMountPath = () => cloudMountDisplayPath(type, stack)
  const childMountPath = (child: CloudEntry) => cloudMountDisplayPath(type, stack, child)

  const mountCurrent = async () => {
    setMounting(true)
    try {
      const label = TYPE_LABEL[type] ?? type
      const name = cur.id ? cur.name : label
      const res = await cloudAPI.mount(type, cur.id, name, mountMediaType, currentMountPath())
      showCloudMountResult(res, cur.name)
      await loadMounts()
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '挂载失败'))
    } finally {
      setMounting(false)
    }
  }

  const mountVisibleDirectories = async () => {
    const dirs = items.filter((item) => item.is_dir)
    if (dirs.length === 0) {
      toast.error('当前目录下没有可挂载的子目录')
      return
    }
    setBatchMounting(true)
    let ok = 0
    let skipped = 0
    let failed = 0
    for (const dir of dirs) {
      try {
        const result = await cloudAPI.mount(type, dir.id, dir.name, 'auto', childMountPath(dir))
        const state = showCloudMountResult(result, dir.name)
        if (state === 'skipped') skipped += 1
        else ok += 1
      } catch {
        failed += 1
      }
    }
    if (failed > 0) {
      toast.error(`已挂载 ${ok} 个目录，跳过 ${skipped} 个重叠目录，失败 ${failed} 个`)
    } else {
      toast.success(`已挂载 ${ok} 个目录，跳过 ${skipped} 个重叠目录，后台会自动生成 302/STRM 播放入口`)
    }
    await loadMounts()
    setBatchMounting(false)
  }

  const removeMount = async (lib: Library) => {
    const ok = await confirmAction({
      title: '移除网盘挂载',
      message: `仅移除「${lib.name}」在本项目中的媒体库和媒体记录，不会删除网盘文件。`,
      confirmText: '移除',
    })
    if (!ok) return
    await libraryAPI.remove(lib.id)
    toast.success('已移除挂载')
    await loadMounts()
  }

  return {
    batchMounting,
    mountCurrent,
    mounting,
    mountVisibleDirectories,
    removeMount,
  }
}
