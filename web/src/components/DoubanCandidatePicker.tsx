import { useEffect, useRef, useState } from 'react'
import toast from 'react-hot-toast'
import { mediaAPI, type ManualScrapeCandidate } from '../api/library'
import { imageURL } from '../api/client'

export function DoubanCandidatePicker({ mediaID, title, onSelect }: {
 mediaID: string; title: string; onSelect: (id: string) => void
}) {
 const [query, setQuery] = useState(title)
 const [items, setItems] = useState<ManualScrapeCandidate[]>([])
 const [loading, setLoading] = useState(false)
 const [searched, setSearched] = useState(false)
 const revision = useRef(0)
 useEffect(() => { revision.current++; setQuery(title); setItems([]); setSearched(false); setLoading(false) }, [mediaID])
 useEffect(() => () => { revision.current++ }, [])
 const search = async () => {
  if (!query.trim()) return
  const current = ++revision.current
  setLoading(true)
  try {
   const candidates = await mediaAPI.manualScrapeSearch(mediaID, { query: query.trim(), provider: 'douban' })
   if (current !== revision.current) return
   setItems(candidates.filter(item => item.douban_id)); setSearched(true)
  } catch { if (current === revision.current) toast.error('豆瓣搜索失败，请稍后重试') }
  finally { if (current === revision.current) setLoading(false) }
 }
 return <div className="space-y-2 rounded-xl border border-gray-200 p-3 md:col-span-2">
  <p className="text-xs font-bold text-gray-600">搜索豆瓣候选</p>
  <div className="flex gap-2">
   <input aria-label="豆瓣标题或 ID" className="input-base flex-1" value={query} onChange={e => setQuery(e.target.value)} placeholder="标题或豆瓣 ID" onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); void search() } }} />
   <button type="button" className="btn-outline" disabled={loading || !query.trim()} onClick={() => void search()}>{loading ? '搜索中…' : '搜索'}</button>
  </div>
  {searched && items.length === 0 && <p className="text-xs text-gray-500">没有找到候选，可调整名称后重试。</p>}
  <div className="max-h-52 space-y-1 overflow-y-auto">
   {items.map(item => <button type="button" key={item.douban_id} className="flex w-full items-center gap-3 rounded-lg p-2 text-left hover:bg-gray-50" onClick={() => { onSelect(item.douban_id!); setItems([]); setSearched(false) }}>
    {item.poster_url && <img src={imageURL(item.poster_url)} alt="" className="h-14 w-10 rounded object-cover" />}
    <span className="text-sm"><strong>{item.title}</strong><span className="ml-2 text-xs text-gray-500">{item.year || '年份未知'} · ID {item.douban_id}</span></span>
   </button>)}
  </div>
  <p className="text-xs text-gray-500">选择候选后填入豆瓣 ID；保存并补齐会刷新豆瓣评分，并补全缺少的详情。</p>
 </div>
}
