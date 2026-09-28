import { useCallback, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { peopleAPI, peopleErrorMessage, type PersonSummary } from '../api/people'
import { PeopleCards } from './PeopleCards'

export function MediaPeoplePanel({ mediaID, revision, isAdmin }: { mediaID: string; revision?: string; isAdmin: boolean }) {
  const [people, setPeople] = useState<PersonSummary[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const load = useCallback(async () => {
    const result = await peopleAPI.media(mediaID)
    setPeople(result.items)
    setError('')
  }, [mediaID])
  useEffect(() => { void load().catch((error) => setError(peopleErrorMessage(error))) }, [load, revision])
  async function translate() {
    setBusy(true)
    try { const result = await peopleAPI.translateMedia(mediaID); await load(); toast.success(`已更新 ${result.translated} 项角色译文`) }
    catch (error) { toast.error(peopleErrorMessage(error)) }
    finally { setBusy(false) }
  }
  async function refreshPeople() {
    setBusy(true)
    try { await peopleAPI.refreshMedia(mediaID); await load(); toast.success('演职员资料已补全') }
    catch (error) { toast.error(peopleErrorMessage(error)) }
    finally { setBusy(false) }
  }
  if (!people.length && !error && !isAdmin) return null
  return <section className="relative space-y-4 border-t border-gray-100 p-6">
    <div className="flex items-center justify-between"><h2 className="font-semibold text-gray-900">演职员</h2>{isAdmin && <div className="flex gap-2"><button type="button" className="neon-button !text-xs" disabled={busy} onClick={() => void refreshPeople()}>补全演职员</button><button type="button" className="neon-button !text-xs" disabled={busy || !people.length} onClick={() => void translate()}>{busy ? '处理中…' : 'AI 翻译角色'}</button></div>}</div>
    {error ? <p className="text-sm text-red-500">{error}</p> : <PeopleCards people={people} />}
  </section>
}
