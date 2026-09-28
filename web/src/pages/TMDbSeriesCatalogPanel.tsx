import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import { tmdbCatalogAPI, type TMDbSeriesCatalog } from '../api/tmdbCatalog'
import { tasksAPI } from '../api/tasks'

export function TMDbSeriesCatalogPanel({ mediaID, isAdmin }: { mediaID: string; isAdmin: boolean }) {
  const [catalog, setCatalog] = useState<TMDbSeriesCatalog | null>(null)
  const [error, setError] = useState('')
  const [taskID, setTaskID] = useState('')
  const [starting, setStarting] = useState(false)
  const load = useCallback(async () => {
    try {
      setCatalog(await tmdbCatalogAPI.get(mediaID))
      setError('')
    } catch { setError('季集目录加载失败') }
  }, [mediaID])

  useEffect(() => { setCatalog(null); setTaskID(''); void load() }, [load])
  useEffect(() => {
    if (!taskID) return
    let active = true
    const poll = async () => {
      try {
        const snapshot = await tasksAPI.snapshot()
        if (!active) return
        const task = snapshot.background_tasks?.recent.find((entry) => entry.id === taskID)
        if (task || !snapshot.background_tasks?.active.some((entry) => entry.id === taskID)) {
          setTaskID('')
          if (task?.status === 'failed') toast.error(task.error || '目录更新失败')
          else toast.success('季集目录已更新')
          await load()
        }
      } catch { /* The next interval retries transient connection errors. */ }
    }
    const timer = window.setInterval(() => void poll(), 3000)
    return () => { active = false; window.clearInterval(timer) }
  }, [taskID, load])

  const refresh = async () => {
    setStarting(true)
    try {
      const result = await tmdbCatalogAPI.refresh(mediaID)
      setTaskID(result.task_id)
      toast.success('已开始更新，可在实时任务中查看进度')
    } catch (failure) {
      toast.error((failure as { response?: { data?: { error?: string } } })?.response?.data?.error || '更新失败')
    } finally { setStarting(false) }
  }

  return <section className="relative m-6 space-y-4 rounded-2xl border border-gray-200 bg-white/90 p-5">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 className="font-semibold text-ink-600">完整季集目录</h2>
        <p className="text-sm text-sand-500">TMDb 目录包含尚未收录的剧集，已收录条目可打开详情。</p>
      </div>
      {isAdmin && <button className="rounded-lg border border-gray-200 px-3 py-2 text-sm disabled:opacity-50" disabled={starting || Boolean(taskID)} onClick={() => void refresh()}>
        {starting || taskID ? '正在更新…' : '更新 TMDb 目录'}
      </button>}
    </div>
    {error && <p role="alert" className="text-sm text-red-500">{error} <button onClick={() => void load()}>重试</button></p>}
    {!catalog && !error && <p className="text-sm text-sand-500">加载中…</p>}
    {catalog && !catalog.series && <p className="text-sm text-sand-500">尚未获取完整 TMDb 目录，当前显示本地剧集。</p>}
    {catalog?.seasons.map((season) => <details key={season.season_num} className="rounded-xl border border-gray-200 p-3" open={catalog.seasons.length === 1}>
      <summary className="cursor-pointer font-medium text-ink-600">
        {season.title || (season.season_num === 0 ? '特别篇' : `第 ${season.season_num} 季`)} · {season.episodes.filter((episode) => episode.available).length}/{Math.max(season.episode_count, season.episodes.length)} 集已收录
      </summary>
      {season.recheck?.checked_at && <p className="mt-2 text-xs text-sand-500">上次复查：{new Date(season.recheck.checked_at).toLocaleString()} · 下次：{new Date(season.recheck.due_at).toLocaleDateString()}</p>}
      {season.recheck?.status === 'not_found' && <p className="mt-2 text-xs text-amber-600">TMDb 暂未找到该季，稍后会再次复核。</p>}
      <div className="mt-3 grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
        {season.episodes.map((episode) => {
          const body = <>
            <span className="font-medium">E{episode.episode_num} · {episode.title || '暂无标题'}</span>
            <span className="text-xs text-sand-500">{episode.release_date || '播出日期待定'} · {episode.available ? '已收录' : '未收录'}</span>
          </>
          return episode.available && episode.media_id
            ? <Link key={episode.episode_num} className="flex flex-col gap-1 rounded-lg border border-gray-200 p-3 hover:border-primary-400" to={`/media/${episode.media_id}`}>{body}</Link>
            : <div key={episode.episode_num} className="flex flex-col gap-1 rounded-lg border border-dashed border-gray-200 bg-gray-50 p-3 text-sand-500">{body}</div>
        })}
      </div>
    </details>)}
  </section>
}
