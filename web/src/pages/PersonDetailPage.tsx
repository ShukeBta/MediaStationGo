import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import toast from 'react-hot-toast'
import { imageURL } from '../api/client'
import { peopleAPI, peopleErrorMessage, type PersonSummary, type PersonWork } from '../api/people'
import { useAuthStore } from '../stores/auth'

export function PersonDetailPage() {
  const { id = '' } = useParams()
  const [person, setPerson] = useState<PersonSummary | null>(null)
  const [works, setWorks] = useState<PersonWork[]>([])
  const [total, setTotal] = useState(0)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const isAdmin = useAuthStore((s) => s.user?.role === 'admin')
  const load = useCallback(async () => {
    const [profile, result] = await Promise.all([peopleAPI.get(id), peopleAPI.works(id)])
    setPerson(profile); setWorks(result.items); setTotal(result.total); setError('')
  }, [id])
  useEffect(() => { setPerson(null); void load().catch((error) => setError(peopleErrorMessage(error))) }, [load])
  async function update(translate: boolean) {
    setBusy(true)
    try { if (translate) await peopleAPI.translate(id); else await peopleAPI.refresh(id); await load(); toast.success('人物资料已更新') }
    catch (error) { toast.error(peopleErrorMessage(error)) }
    finally { setBusy(false) }
  }
  async function more() {
    setBusy(true)
    try { const result = await peopleAPI.works(id, works.length); setWorks((previous) => [...previous, ...result.items]); setTotal(result.total) }
    catch (error) { toast.error(peopleErrorMessage(error)) }
    finally { setBusy(false) }
  }
  if (error) return <p role="alert" className="text-red-500">{error}</p>
  if (!person) return <p className="text-gray-500">加载人物资料…</p>
  const tmdbID = person.ProviderIds?.tmdb
  return <div className="space-y-8">
    <Link to="/people" className="text-sm text-brand-600">← 人物资料</Link>
    <section className="flex flex-col gap-6 sm:flex-row">
      {person.ImageURL && <img className="w-44 self-start rounded-2xl object-cover" src={imageURL(person.ImageURL)} alt={person.Name} />}
      <div className="min-w-0 flex-1 space-y-3"><h1 className="text-3xl font-bold">{person.Name}</h1>
        {person.OriginalTitle && person.OriginalTitle !== person.Name && <p className="text-gray-500">{person.OriginalTitle}</p>}
        <p className="text-sm text-gray-600">{[person.Department, person.PremiereDate && `出生：${person.PremiereDate}`, person.EndDate && `逝世：${person.EndDate}`, ...(person.ProductionLocations ?? [])].filter(Boolean).join(' · ')}</p>
        {!!person.Aliases?.length && <p className="text-sm text-gray-500">别名：{person.Aliases.join('、')}</p>}
        <p className="whitespace-pre-line text-sm leading-7 text-gray-700">{person.Overview || '暂无人物简介'}</p>
        {tmdbID && <a className="text-sm text-brand-600" href={`https://www.themoviedb.org/person/${encodeURIComponent(tmdbID)}`} target="_blank" rel="noreferrer">TMDb 人物资料 ↗</a>}
        {isAdmin && <div className="flex gap-3">{tmdbID && <button className="neon-button !text-xs" disabled={busy} onClick={() => void update(false)}>刷新 TMDb 资料</button>}<button className="neon-button !text-xs" disabled={busy} onClick={() => void update(true)}>{busy ? '处理中…' : 'AI 翻译资料'}</button></div>}
      </div>
    </section>
    <section className="space-y-4"><h2 className="text-lg font-semibold">媒体库中的作品 · {total}</h2><div className="grid grid-cols-2 gap-4 sm:grid-cols-4 lg:grid-cols-6">{works.map((work) => <Link key={work.id} to={`/media/${work.id}`} className="overflow-hidden rounded-xl border border-gray-200 bg-white">{work.poster_url && <img className="aspect-[2/3] w-full object-cover" src={imageURL(work.poster_url)} alt={work.title} loading="lazy" />}<div className="space-y-1 p-3"><p className="text-sm font-semibold">{work.title}</p><p className="text-xs text-gray-500">{work.episode_title || work.year || ''}</p></div></Link>)}</div>{works.length < total && <button className="neon-button" disabled={busy} onClick={() => void more()}>加载更多</button>}</section>
  </div>
}
