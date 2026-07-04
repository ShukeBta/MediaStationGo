import { useEffect, useState } from 'react'
import toast from 'react-hot-toast'

import { filesAPI, type FileEntry } from '../api/files'
import { confirmAction } from '../components/confirmAction'

type UseFileOperationsOptions = {
  currentDir: string
  path: string
  refresh: () => void
}

export function useFileOperations({ currentDir, path, refresh }: UseFileOperationsOptions) {
  const [selected, setSelected] = useState<FileEntry | null>(null)
  const [selectedPaths, setSelectedPaths] = useState<string[]>([])
  const [folderName, setFolderName] = useState('')
  const [renameTo, setRenameTo] = useState('')
  const [destPath, setDestPath] = useState('')
  const [transferMode, setTransferMode] = useState('copy')
  const [busy, setBusy] = useState('')

  useEffect(() => {
    setSelected(null)
    setSelectedPaths([])
    setRenameTo('')
  }, [path])

  const choose = (entry: FileEntry) => {
    setSelected(entry)
    setRenameTo(entry.name)
    if (entry.is_dir) setDestPath(entry.path)
  }

  const toggleSelectedPath = (entry: FileEntry, checked: boolean) => {
    setSelectedPaths((current) => {
      if (checked) {
        return current.includes(entry.path) ? current : [...current, entry.path]
      }
      return current.filter((item) => item !== entry.path)
    })
  }

  const createFolder = async () => {
    if (!currentDir || !folderName.trim()) return
    setBusy('mkdir')
    try {
      const res = await filesAPI.createFolder(currentDir, folderName.trim())
      toast.success(`已创建目录：${res.path}`)
      setFolderName('')
      refresh()
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '创建失败')
    } finally {
      setBusy('')
    }
  }

  const renameSelected = async () => {
    if (!selected || !renameTo.trim()) return
    setBusy('rename')
    try {
      const res = await filesAPI.rename(selected.path, renameTo.trim())
      toast.success(`已重命名：${res.path}`)
      setSelected(null)
      refresh()
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '重命名失败')
    } finally {
      setBusy('')
    }
  }

  const deleteSelected = async () => {
    if (!selected) return
    const ok = await confirmAction({
      title: '确认删除文件',
      message: `将删除：${selected.path}。此操作不可恢复，请确认不是媒体库根目录。`,
      confirmText: '确认删除',
      danger: true,
    })
    if (!ok) return
    setBusy('delete')
    try {
      await filesAPI.remove(selected.path)
      toast.success('已删除')
      setSelected(null)
      refresh()
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '删除失败')
    } finally {
      setBusy('')
    }
  }

  const transferSelected = async () => {
    if (!selected || selected.is_dir || !destPath.trim()) return
    setBusy('transfer')
    try {
      const res = await filesAPI.transfer(selected.path, destPath.trim(), transferMode)
      toast.success(`已完成转移：${res.path}`)
      if (transferMode === 'move') setSelected(null)
      refresh()
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '转移失败')
    } finally {
      setBusy('')
    }
  }

  return {
    selected,
    selectedPaths,
    folderName,
    renameTo,
    destPath,
    transferMode,
    busy,
    setSelectedPaths,
    setFolderName,
    setRenameTo,
    setDestPath,
    setTransferMode,
    choose,
    toggleSelectedPath,
    createFolder,
    renameSelected,
    deleteSelected,
    transferSelected,
  }
}
