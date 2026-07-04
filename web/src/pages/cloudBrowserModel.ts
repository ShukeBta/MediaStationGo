import toast from 'react-hot-toast'

export type CloudMountResult = {
  already_mounted?: boolean
  estimate_message?: string
  message?: string
  reason?: string
  skipped?: boolean
}

export function apiErrorMessage(err: unknown, fallback: string): string {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}

export function showCloudMountResult(res: unknown, label: string) {
  const out = res as CloudMountResult
  if (out.skipped) {
    toast(`已跳过「${label}」：和已挂载目录重叠`)
    return 'skipped'
  }
  if (out.already_mounted) {
    toast(`「${label}」已经挂载，后台会刷新扫描并自动入库`)
    return 'mounted'
  }
  toast.success(`已挂载「${label}」，${out.message ?? '后台会递归扫描并自动加入媒体库'}。${out.estimate_message ?? ''}`)
  return 'mounted'
}

export function normalizeCloudFolderInput(value: string | null): string {
  const name = (value ?? '').trim()
  if (!name) return ''
  if (name === '.' || name === '..' || /[\\/]/.test(name)) {
    toast.error('文件夹名称不能包含路径分隔符')
    return ''
  }
  return name
}
