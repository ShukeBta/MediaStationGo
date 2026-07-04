import type { Dispatch, SetStateAction } from 'react'
import toast from 'react-hot-toast'

import { cloudAPI, type CloudEntry, type StorageType } from '../api/storage_config'
import { apiErrorMessage, normalizeCloudFolderInput } from './cloudBrowserModel'

type CloudStackItem = { id: string; name: string }

type UseCloudBrowserFileActionsOptions = {
  load: (dir: string) => Promise<void>
  loadMounts: () => Promise<void>
  setLoading: Dispatch<SetStateAction<boolean>>
  stack: CloudStackItem[]
  type: StorageType
}

export function useCloudBrowserFileActions({
  load,
  loadMounts,
  setLoading,
  stack,
  type,
}: UseCloudBrowserFileActionsOptions) {
  const currentDir = () => stack[stack.length - 1]?.id ?? ''

  const doImport = async (entry: CloudEntry) => {
    const ref = type === 'cloud115' ? entry.pick_code || entry.id : entry.id
    try {
      await cloudAPI.import(type, ref, entry.name, entry.size)
      toast.success(`已导入「${entry.name}」,可在媒体库中 302 播放`)
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '导入失败'))
    }
  }

  const createFolder = async () => {
    const name = normalizeCloudFolderInput(window.prompt('新建文件夹名称') ?? '')
    if (!name) return
    setLoading(true)
    try {
      await cloudAPI.mkdir(type, currentDir(), name)
      toast.success(`已新建文件夹「${name}」`)
      await load(currentDir())
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '新建文件夹失败'))
    } finally {
      setLoading(false)
    }
  }

  const renameFolder = async (entry: CloudEntry) => {
    if (!entry.is_dir) return
    const name = normalizeCloudFolderInput(window.prompt('重命名文件夹', entry.name) ?? '')
    if (!name || name === entry.name) return
    setLoading(true)
    try {
      await cloudAPI.rename(type, entry.id, name)
      toast.success(`已重命名为「${name}」`)
      await load(currentDir())
      await loadMounts()
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '重命名失败'))
    } finally {
      setLoading(false)
    }
  }

  return {
    createFolder,
    doImport,
    renameFolder,
  }
}
