import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import toast from 'react-hot-toast'
import { peopleAPI, peopleErrorMessage, type PersonSummary } from '../api/people'
import { useAuthStore } from '../stores/auth'
import { PeopleCards } from './PeopleCards'

export function PeoplePage() {
  const [params, setParams] = useSearchParams()
  const query = params.get('q') ?? ''
  const offset = Math.max(0, Number(params.get('offset')) || 0)
  const [people, setPeople] = useState<PersonSummary[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [automatic, setAutomatic] = useState(false)
  const [saving, setSaving] = useState(false)
  const isAdmin = useAuthStore((s) => s.user?.role === 'admin')
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    peopleAPI.search(query, offset, controller.signal).then((result) => { setPeople(result.Items); setTotal(result.TotalRecordCount) }).catch((error) => { if (!controller.signal.aborted) setError(peopleErrorMessage(error)) }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [query, offset])
  useEffect(() => {
    if (isAdmin) void peopleAPI.settings().then((settings) => setAutomatic(settings.enabled)).catch((error) => toast.error(peopleErrorMessage(error)))
  }, [isAdmin])
  async function toggleAutomatic() {
    setSaving(true)
    try { const settings = await peopleAPI.saveSettings(!automatic); setAutomatic(settings.enabled) }
    catch (error) { toast.error(peopleErrorMessage(error)) }
    finally { setSaving(false) }
  }
  return <div className="space-y-6">
    <h1 className="text-3xl font-bold">人物资料</h1>
    <input type="search" aria-label="搜索人物" placeholder="搜索姓名、原名或别名" value={query} onChange={(event) => setParams({ q: event.target.value })} className="w-full rounded-xl border border-gray-200 bg-white px-4 py-3" />
    {isAdmin && <label className="flex items-center gap-2 text-sm text-gray-600"><input type="checkbox" checked={automatic} disabled={saving} onChange={() => void toggleAutomatic()} />自动补全 TMDb 人物资料并通过已配置 AI 翻译姓名、简介和角色</label>}
    {error && <p role="alert" className="text-red-500">{error}</p>}
    {loading ? <p className="text-gray-500">加载中…</p> : <><p className="text-sm text-gray-500">共 {total} 位人物</p><PeopleCards people={people} />{total === 0 && <p className="text-gray-500">当前可见媒体中暂无匹配人物。刮削作品后会自动建立人物资料。</p>}</>}
    <div className="flex gap-3"><button disabled={offset === 0 || loading} className="neon-button" onClick={() => setParams({ q: query, offset: String(Math.max(0, offset - 24)) })}>上一页</button><button disabled={offset + 24 >= total || loading} className="neon-button" onClick={() => setParams({ q: query, offset: String(offset + 24) })}>下一页</button></div>
  </div>
}
