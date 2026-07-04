import type { CloudScanStatus } from '../api/storage_config'
import type { Media } from '../types'

export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return ''
  if (seconds < 60) return `${Math.round(seconds)}秒`
  const minutes = Math.floor(seconds / 60)
  const rest = Math.round(seconds % 60)
  if (minutes < 60) return `${minutes}分${rest}秒`
  const hours = Math.floor(minutes / 60)
  return `${hours}小时${minutes % 60}分`
}

export function formatCloudScanStatus(status: CloudScanStatus): string {
  const stage =
    status.state === 'queued' ? '扫描已排队'
      : status.state === 'canceling' ? '正在中断扫描'
        : status.state === 'finished' ? '扫描完成'
          : status.stage === 'importing' ? '正在入库'
            : '正在遍历目录'
  const speed = Number(status.files_per_second ?? 0)
  const speedText = speed > 0 && status.state !== 'finished'
    ? ` · ${speed.toFixed(speed >= 10 ? 0 : 1)} 个/秒`
    : ''
  return `${stage}：目录 ${status.dirs ?? 0} · 已发现 ${status.discovered ?? 0} · 已入库 ${status.visited ?? 0} · 新增 ${status.added ?? 0} · 更新 ${status.updated ?? 0}${speedText}`
}

export function seriesSourceRoot(episodes: Media[]): string {
  const firstPath = episodes.find((item) => item.path)?.path ?? ''
  if (!firstPath) return ''
  const dir = dirname(firstPath)
  const base = basename(dir)
  if (/^(?:s\d{1,2}|season[\s._-]*\d{1,2}|第\s*\d{1,2}\s*季|specials?|sp|ova|oad|extra|extras|特别篇|特別篇|番外|特典)$/i.test(base)) {
    return dirname(dir)
  }
  return dir
}

export function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value.toFixed(1)} ${units[index]}`
}

function dirname(value: string): string {
  const index = Math.max(value.lastIndexOf('/'), value.lastIndexOf('\\'))
  return index > 0 ? value.slice(0, index) : ''
}

function basename(value: string): string {
  const index = Math.max(value.lastIndexOf('/'), value.lastIndexOf('\\'))
  return index >= 0 ? value.slice(index + 1) : value
}
