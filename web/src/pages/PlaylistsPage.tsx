import { FormEvent, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ListPlus, Trash2 } from 'lucide-react'
import toast from 'react-hot-toast'

import { playbackAPI } from '../api/playback'
import { confirmAction } from '../components/confirmAction'
import type { Playlist } from '../types'

// Landing page for playlists. Lists every playlist owned by the current
// user and lets them create / delete one.
export function PlaylistsPage() {
  const [items, setItems] = useState<Playlist[]>([])
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(true)

  const refresh = () =>
    playbackAPI
      .listPlaylists()
      .then(setItems)
      .finally(() => setLoading(false))

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    try {
      await playbackAPI.createPlaylist(name)
      toast.success('已创建')
      setName('')
      await refresh()
    } catch {
      toast.error('创建失败')
    }
  }

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl font-bold text-ink-600">播放列表</h1>

      <form onSubmit={onCreate} className="glass-panel grid gap-3 md:grid-cols-[1fr_auto]">
        <input
          required
          className="input-base"
          placeholder="新播放列表名称"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <button type="submit" className="neon-button">
          <ListPlus size={16} /> 新建
        </button>
      </form>

      {loading && <p className="text-sand-500">加载中…</p>}
      {!loading && items.length === 0 && <p className="text-ink-50">还没有任何播放列表。</p>}

      <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
        {items.map((p) => (
          <div
            key={p.id}
            className="glass-panel flex items-center justify-between !p-4"
          >
            <Link
              to={`/playlist/${p.id}`}
              className="font-medium text-ink-600 transition hover:text-brand-500"
            >
              {p.name}
              {p.is_public && (
                <span className="ml-2 rounded-lg border border-primary-400/40 px-1.5 py-0.5 text-xs text-brand-500">
                  public
                </span>
              )}
            </Link>
            <button
              onClick={async () => {
                if (!(await confirmAction({ title: '删除播放列表', message: `删除「${p.name}」?`, confirmText: '删除' }))) return
                await playbackAPI.deletePlaylist(p.id)
                toast.success('已删除')
                await refresh()
              }}
              className="rounded-lg border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10"
            >
              <Trash2 size={12} />
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}
