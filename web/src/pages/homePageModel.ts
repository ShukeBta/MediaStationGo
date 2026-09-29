import type { Library } from '../types'
import type { SeriesCard } from '../utils/groupSeries'

export type RecentFilter = 'all' | 'movie' | 'series' | 'anime'

type HeroPage = { items: SeriesCard[]; total: number }
export type HeroSelection = { item: SeriesCard | null; page: number }

// Page size one lets every visible work participate without downloading the library.
export async function loadRandomHero(
  loadPage: (page: number) => Promise<HeroPage>,
  previousPage = 0,
  random: () => number = Math.random,
): Promise<HeroSelection> {
  const first = await loadPage(1)
  const total = Math.max(0, Math.floor(first.total))
  if (!total) return { item: null, page: 0 }
  const excludePrevious = total > 1 && previousPage >= 1 && previousPage <= total
  const choices = total - (excludePrevious ? 1 : 0)
  let page = Math.min(choices, Math.floor(Math.max(0, random()) * choices) + 1)
  if (excludePrevious && page >= previousPage) page += 1
  const selected = page === 1 ? first : await loadPage(page)
  // A concurrent deletion can invalidate the sampled page; keep a valid card.
  return selected.items?.[0]
    ? { item: selected.items[0], page }
    : { item: first.items?.[0] ?? null, page: first.items?.[0] ? 1 : 0 }
}

export function filterRecentCards(cards: SeriesCard[], libraries: Library[], filter: RecentFilter): SeriesCard[] {
  if (filter === 'all') return cards
  const types = new Map(libraries.map((library) => [library.id, library.type]))
  return cards.filter((card) => {
    const type = types.get(card.rep.display_library_id || card.rep.library_id) || types.get(card.rep.library_id)
    return filter === 'series' ? type === 'tv' || type === 'variety' : type === filter
  })
}

export function playbackProgress(position: number, duration: number): number {
  if (!Number.isFinite(position) || !Number.isFinite(duration) || duration <= 0) return 0
  return Math.round(Math.min(1, Math.max(0, position / duration)) * 100)
}
