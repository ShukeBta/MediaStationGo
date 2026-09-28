import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { playbackStatsAPI, type PlaybackStats } from '../api/playbackStats'

export function PlaybackStatsPage() {
  const today = new Date().toISOString().slice(0, 10)
  const [from, setFrom] = useState(new Date(Date.now() - 29 * 86400000).toISOString().slice(0, 10))
  const [to, setTo] = useState(today)
  const [grain, setGrain] = useState('day')
  const [kind, setKind] = useState('')
  const [userID, setUserID] = useState('')
  const [libraryID, setLibraryID] = useState('')
  const [rankDate, setRankDate] = useState(today)
  const [rankGrain, setRankGrain] = useState('day')
  const [page, setPage] = useState(1)
  const [data, setData] = useState<PlaybackStats | null>(null)
  const [error, setError] = useState('')
  useEffect(() => { setPage(1) }, [from, to, grain, kind, userID, libraryID])
  useEffect(() => {
    const controller = new AbortController()
    setError('')
    playbackStatsAPI.stats({ from, to, grain, media_type: kind, user_id: userID, library_id: libraryID, page, rank_date: rankDate, rank_grain: rankGrain }, controller.signal).then(setData).catch((error) => { if (!controller.signal.aborted) setError(error?.response?.data?.error ?? '统计加载失败') })
    return () => controller.abort()
  }, [from, to, grain, kind, userID, libraryID, page, rankDate, rankGrain])
  const maximum = Math.max(1, ...(data?.buckets.map((item) => item.count) ?? []))
  return <div className="space-y-6"><h1 className="text-3xl font-bold">播放统计</h1><p className="text-sm text-gray-500">UTC 日期；同一播放会话只统计一次。播放进度达到 20 秒计数，超过 10 分钟的作品需达到 60 秒；手动标记已看不计数。</p>
    <div className="flex flex-wrap gap-3"><label>开始 <input aria-label="开始日期" type="date" value={from} onChange={(e) => { setFrom(e.target.value); if (rankDate < e.target.value) setRankDate(e.target.value) }} /></label><label>结束 <input aria-label="结束日期" type="date" value={to} onChange={(e) => { setTo(e.target.value); if (rankDate > e.target.value) setRankDate(e.target.value) }} /></label><select aria-label="统计粒度" value={grain} onChange={(e) => setGrain(e.target.value)}><option value="day">按日</option><option value="week">按周</option><option value="month">按月</option></select><select aria-label="媒体类型" value={kind} onChange={(e) => setKind(e.target.value)}><option value="">全部类型</option><option value="movie">电影</option><option value="tv">剧集</option></select><input aria-label="用户ID筛选" placeholder="用户 ID" value={userID} onChange={(e) => setUserID(e.target.value)} /><input aria-label="媒体库ID筛选" placeholder="媒体库 ID" value={libraryID} onChange={(e) => setLibraryID(e.target.value)} /></div>
    {error && <p role="alert" className="text-red-500">{error}</p>}
    {data && <><section className="glass-panel space-y-3"><h2 className="font-semibold">共 {data.total} 次播放</h2><div className="max-h-80 space-y-2 overflow-auto">{data.buckets.map((bucket) => <div key={bucket.period} className="flex items-center gap-3 text-sm"><span className="w-24 shrink-0">{bucket.period}</span><div className="h-4 rounded bg-brand-400" style={{ width: `${bucket.count / maximum * 70}%` }} /><span>{bucket.count}</span></div>)}</div></section>
      <section className="glass-panel space-y-3"><div className="flex flex-wrap gap-3"><h2 className="font-semibold">播放热榜 Top 10</h2><input aria-label="热榜日期" type="date" value={rankDate} min={from} max={to} onChange={(e) => setRankDate(e.target.value)} /><select aria-label="热榜粒度" value={rankGrain} onChange={(e) => setRankGrain(e.target.value)}><option value="day">当日</option><option value="week">当周</option></select></div><ol className="space-y-2">{data.ranking.map((item, index) => <li key={item.work_key} className="flex justify-between"><Link to={`/media/${item.media_id}`}>{index + 1}. {item.title}{item.season_num ? ` · 第 ${item.season_num} 季` : ''}</Link><span>{item.count} 次</span></li>)}</ol></section>
      <section className="glass-panel overflow-x-auto"><h2 className="mb-3 font-semibold">播放明细</h2><table className="w-full text-left text-sm"><thead><tr><th>时间</th><th>作品</th><th>用户</th><th>媒体库</th><th>客户端</th></tr></thead><tbody>{data.items.map((item) => <tr key={item.id} className="border-t border-gray-100"><td className="py-3">{new Date(item.played_at).toLocaleString()}</td><td><Link to={`/media/${item.media_id}`}>{item.title}{item.episode_num ? ` S${item.season_num}E${item.episode_num}` : ''}</Link></td><td>{item.user_name || '已删除用户'}</td><td>{item.library_name || '已删除媒体库'}</td><td>{item.client || '—'}</td></tr>)}</tbody></table><div className="mt-4 flex gap-3"><button className="neon-button" disabled={page <= 1} onClick={() => setPage(page - 1)}>上一页</button><span>第 {page} 页</span><button className="neon-button" disabled={page * 25 >= data.total} onClick={() => setPage(page + 1)}>下一页</button></div></section></>}
  </div>
}
