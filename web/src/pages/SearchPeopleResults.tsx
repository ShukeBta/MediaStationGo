import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { peopleAPI, type PersonSummary } from '../api/people'
import { PeopleCards } from './PeopleCards'

export function SearchPeopleResults({ query }: { query: string }) {
  const [people, setPeople] = useState<PersonSummary[]>([])
  useEffect(() => {
    setPeople([])
    if (!query.trim()) return
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      peopleAPI.search(query, 0, controller.signal).then((result) => setPeople(result.Items.slice(0, 6))).catch(() => undefined)
    }, 250)
    return () => { window.clearTimeout(timer); controller.abort() }
  }, [query])
  if (!people.length) return null
  return <section className="space-y-3"><div className="flex items-center justify-between"><h2 className="font-semibold">人物</h2><Link className="text-sm text-brand-600" to={`/people?q=${encodeURIComponent(query)}`}>全部人物</Link></div><PeopleCards people={people} /></section>
}
