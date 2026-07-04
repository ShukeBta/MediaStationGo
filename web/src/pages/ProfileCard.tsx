import { useMemo } from 'react'
import { Pencil, Trash2 } from 'lucide-react'

import type { Library, PlayProfile } from '../types'

type ProfileCardProps = {
  profile: PlayProfile
  libraries: Library[]
  active: boolean
  onSelect: () => void
  onEdit: () => void
  onDelete: () => void
}

export function ProfileCard({
  profile,
  libraries,
  active,
  onSelect,
  onEdit,
  onDelete,
}: ProfileCardProps) {
  const libNames = useMemo(() => {
    if (!profile.allowed_library_ids?.length) return '全部'
    const idx = new Map(libraries.map((library) => [library.id, library.name]))
    return profile.allowed_library_ids.map((id) => idx.get(id) ?? id).join(', ')
  }, [profile, libraries])

  return (
    <div className="glass-panel flex items-start justify-between gap-4">
      <div className="flex min-w-0 items-start gap-3">
        <div
          className="flex h-12 w-12 shrink-0 items-center justify-center rounded-full text-lg font-bold text-ink-600"
          style={{
            background: `hsl(${(profile.name.charCodeAt(0) * 47) % 360}, 60%, 35%)`,
          }}
        >
          {profile.name[0]?.toUpperCase()}
        </div>
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-semibold text-ink-600">{profile.name}</span>
            {profile.is_default && (
              <span className="rounded-lg bg-primary-400/20 px-2 py-0.5 text-xs text-brand-500">
                默认
              </span>
            )}
            {active && (
              <span className="rounded-lg bg-gray-950 px-2 py-0.5 text-xs text-white">
                当前使用
              </span>
            )}
            {profile.allow_adult && (
              <span className="rounded-lg bg-red-400/20 px-2 py-0.5 text-xs text-red-400">
                成人内容
              </span>
            )}
            {profile.require_pin && (
              <span className="rounded-lg bg-amber-400/20 px-2 py-0.5 text-xs text-amber-400">
                🔒 PIN
              </span>
            )}
          </div>
          <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-50">
            {profile.content_rating_limit && <span>分级: {profile.content_rating_limit}</span>}
            <span>媒体库: {libNames}</span>
          </div>
          <div className="text-xs text-sand-500">
            观看时长 {Math.round(profile.total_watch_time / 3600)} 小时
            {profile.last_active_at && ` · 最近活跃 ${new Date(profile.last_active_at).toLocaleDateString()}`}
          </div>
        </div>
      </div>
      <div className="flex shrink-0 gap-2">
        <button
          onClick={onSelect}
          className="rounded-lg border border-primary-400/40 px-2 py-1 text-xs text-brand-500 hover:bg-primary-400/10"
        >
          设为当前
        </button>
        <button
          onClick={onEdit}
          className="rounded-lg border border-gray-200 px-2 py-1 text-xs text-ink-100 hover:border-primary-400/40 hover:text-brand-500"
        >
          <Pencil size={12} className="inline" /> 编辑
        </button>
        <button
          onClick={onDelete}
          className="rounded-lg border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10"
        >
          <Trash2 size={12} className="inline" /> 删除
        </button>
      </div>
    </div>
  )
}
