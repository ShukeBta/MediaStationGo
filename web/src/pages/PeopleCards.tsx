import { Link } from 'react-router-dom'
import { UserRound } from 'lucide-react'
import { imageURL } from '../api/client'
import type { PersonSummary } from '../api/people'

export function PeopleCards({ people }: { people: PersonSummary[] }) {
  return <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 lg:grid-cols-6">
    {people.map((person, index) => <Link key={`${person.Id}-${person.Type}-${index}`} to={`/people/${encodeURIComponent(person.Id)}`} className="min-w-0 rounded-xl border border-gray-200 bg-white p-3 transition hover:border-brand-400">
      <div className="mb-2 flex aspect-[3/4] items-center justify-center overflow-hidden rounded-lg bg-gray-100">
        {person.ImageURL ? <img src={imageURL(person.ImageURL)} alt={person.Name} loading="lazy" className="h-full w-full object-cover" /> : <UserRound className="text-gray-400" size={40} />}
      </div>
      <p className="truncate text-sm font-semibold text-gray-900">{person.Name}</p>
      {person.Role && <p className="mt-1 text-xs text-gray-500">{person.Role}</p>}
      {!person.Role && person.RecursiveItemCount !== undefined && <p className="mt-1 text-xs text-gray-500">{person.RecursiveItemCount} 部作品</p>}
      {person.Type === 'Director' && <p className="mt-1 text-xs text-gray-500">导演</p>}
      {person.Type === 'Writer' && <p className="mt-1 text-xs text-gray-500">编剧</p>}
    </Link>)}
  </div>
}
