import { useCallback, useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { X } from 'lucide-react'
import toast from 'react-hot-toast'

import { playbackAPI, type PlaylistDetail } from '../api/playback'
import { MediaCard } from '../components/MediaCard'

export function PlaylistDetailPage() {
  const { id = '' } = useParams()
  const [detail, setDetail] = useState<PlaylistDetail | null>(null)

  const refresh = useCallback(() => {
    return playbackAPI.getPlaylist(id).then(setDetail).catch(() => setDetail(null))
  }, [id])

  useEffect(() => {
    refresh()
  }, [refresh])

  if (!detail) {
    return <p className="text-sand-500">加载中…</p>
  }

  return (
    <div className="space-y-6">
      <header>
        <h1 className="font-display text-3xl font-bold text-ink-600">{detail.playlist.name}</h1>
        <p className="text-sm text-ink-50">{detail.items.length} 部影片</p>
      </header>

      {detail.items.length === 0 && (
        <p className="text-ink-50">暂无内容,前往媒体详情页通过「加入播放列表」添加。</p>
      )}

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
        {detail.items.map((m) => (
          <div key={m.id} className="relative">
            <MediaCard media={m} />
            <button
              className="absolute right-2 top-2 rounded-full bg-black/70 p-1 text-ink-600 transition hover:bg-red-600"
              onClick={async () => {
                await playbackAPI.removeFromPlaylist(detail.playlist.id, m.id)
                toast.success('已移除')
                await refresh()
              }}
              title="从播放列表移除"
            >
              <X size={14} />
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}
