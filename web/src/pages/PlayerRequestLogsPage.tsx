import { useEffect, useState } from 'react'
import { playbackStatsAPI, type PlayerRequestLog } from '../api/playbackStats'

export function PlayerRequestLogsPage() {
  const [date, setDate] = useState(new Date().toISOString().slice(0, 10))
  const [route, setRoute] = useState('')
  const [method, setMethod] = useState('')
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [live, setLive] = useState(true)
  const [items, setItems] = useState<PlayerRequestLog[]>([])
  const [total, setTotal] = useState(0)
  const [dropped, setDropped] = useState(0)
  const [error, setError] = useState('')
  useEffect(() => { setPage(1) }, [date, route, method, status])
  useEffect(() => {
    const controller = new AbortController()
    const load = () => playbackStatsAPI.requests({ from: date, to: date, route, method, status, page }, controller.signal).then((result) => { setItems(result.items); setTotal(result.total); setDropped(result.dropped); setError('') }).catch((error) => { if (!controller.signal.aborted) setError(error?.response?.data?.error ?? '请求日志加载失败') })
    void load()
    const timer = live && page === 1 ? window.setInterval(() => void load(), 5000) : undefined
    return () => { controller.abort(); if (timer) window.clearInterval(timer) }
  }, [date, route, method, status, page, live])
  return <div className="space-y-6"><h1 className="text-3xl font-bold">播放器请求日志</h1><p className="text-sm text-gray-500">最近 30 天的 Emby 兼容接口诊断。仅记录路由模板和允许的播放参数；不保存凭据、请求正文或响应正文。</p><div className="flex flex-wrap gap-3"><input aria-label="日志日期UTC" type="date" value={date} onChange={(e) => setDate(e.target.value)} /><input aria-label="路由筛选" placeholder="路由包含…" value={route} onChange={(e) => setRoute(e.target.value)} /><select aria-label="HTTP方法" value={method} onChange={(e) => setMethod(e.target.value)}><option value="">全部方法</option>{['GET', 'POST', 'HEAD', 'DELETE'].map((value) => <option key={value}>{value}</option>)}</select><input aria-label="HTTP状态码" placeholder="状态码" value={status} onChange={(e) => setStatus(e.target.value)} /><label className="flex gap-2"><input type="checkbox" checked={live} onChange={(e) => setLive(e.target.checked)} />实时刷新首页</label></div>{error && <p role="alert" className="text-red-500">{error}</p>}{dropped > 0 && <p className="text-amber-600">繁忙期间有 {dropped} 条诊断日志未写入。</p>}
    <section className="glass-panel overflow-x-auto"><p className="mb-3 text-sm text-gray-500">共 {total} 条 · UTC 日期范围</p><table className="w-full text-left text-sm"><thead><tr><th>时间</th><th>请求</th><th>状态</th><th>耗时</th><th>来源</th><th>参数</th></tr></thead><tbody>{items.map((item) => <tr key={item.id} className="border-t border-gray-100"><td className="py-3">{new Date(item.requested_at).toLocaleTimeString()}</td><td className="font-mono text-xs">{item.method} {item.route}</td><td className={item.status >= 400 ? 'text-red-500' : ''}>{item.status}</td><td>{item.duration_ms} ms</td><td>{item.ip}</td><td><details><summary className="cursor-pointer">查看</summary><pre className="max-w-sm whitespace-pre-wrap break-all">{item.query}</pre></details></td></tr>)}</tbody></table></section><div className="flex gap-3"><button className="neon-button" disabled={page === 1} onClick={() => setPage(page - 1)}>上一页</button><span>第 {page} 页</span><button className="neon-button" disabled={page * 25 >= total} onClick={() => setPage(page + 1)}>下一页</button></div>
  </div>
}
