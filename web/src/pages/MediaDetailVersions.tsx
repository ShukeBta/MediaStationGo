import { Film, LoaderCircle, Trash2 } from 'lucide-react'

import type { MediaVersion } from '../types'
import { formatSize } from './libraryPageModel'

type MediaDetailVersionsProps = {
  versions: MediaVersion[]
  loading: boolean
  isAdmin: boolean
  deletingID: string
  onDelete: (version: MediaVersion) => void
}

export function MediaDetailVersions({ versions, loading, isAdmin, deletingID, onDelete }: MediaDetailVersionsProps) {
  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-sand-500">
        <LoaderCircle size={15} className="animate-spin" />
        正在加载片源版本
      </div>
    )
  }
  if (versions.length === 0) return null

  return (
    <section className="space-y-3" aria-label="片源版本">
      <div className="flex items-center gap-2">
        <Film size={15} className="text-[#c9954a]" />
        <h2 className="text-sm font-semibold text-ink-600">片源版本</h2>
        <span className="text-xs text-sand-500">{versions.length} 个</span>
      </div>
      <div className="divide-y divide-gray-200 border-y border-gray-200">
        {versions.map((version) => (
          <div key={version.id} className="grid gap-2 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
            <div className="min-w-0 space-y-1">
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm font-medium text-ink-600">
                <span>{versionLabel(version)}</span>
                {version.is_current && <span className="text-xs font-normal text-[#c9954a]">当前播放版本</span>}
              </div>
              {version.path && <p className="truncate text-xs text-sand-500" title={version.path}>{version.path}</p>}
              <p className="text-xs text-ink-50">{versionTechnicalText(version, isAdmin)}</p>
            </div>
            {version.can_manage && (
              <button
                type="button"
                title="将这个版本移入回收站"
                aria-label={`删除片源版本 ${versionLabel(version)}`}
                onClick={() => onDelete(version)}
                disabled={Boolean(deletingID)}
                className="btn-outline h-9 w-9 justify-center p-0 !border-red-100 !text-red-500 hover:!border-red-200 hover:!bg-red-50"
              >
                {deletingID === version.id ? <LoaderCircle size={14} className="animate-spin" /> : <Trash2 size={14} />}
              </button>
            )}
          </div>
        ))}
      </div>
    </section>
  )
}

function versionLabel(version: MediaVersion): string {
  const resolution = version.width > 0 && version.height > 0 ? `${version.width}x${version.height}` : ''
  return [resolution, version.container?.toUpperCase(), version.video_codec?.toUpperCase()].filter(Boolean).join(' · ') || '未探测版本'
}

function versionTechnicalText(version: MediaVersion, isAdmin: boolean): string {
  return [
    version.size_bytes > 0 ? formatSize(version.size_bytes) : '',
    version.audio_codec?.toUpperCase(),
    isAdmin && version.created_at ? `入库于 ${new Date(version.created_at).toLocaleString()}` : '',
  ].filter(Boolean).join(' · ')
}
