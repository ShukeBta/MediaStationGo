import type { DiscoverItem, DiscoverPerson } from '../api/discover'

export function supportsAdultMovieDetail(item: DiscoverItem): boolean {
  const source = item.source?.toLowerCase()
  return item.media_type === 'adult'
    && (source === 'javdb' || source === 'fd2ppv')
    && Boolean(item.provider_id?.trim())
    && Boolean(item.original_name?.trim())
}

export function supportsCatalogItemDetail(item: DiscoverItem): boolean {
  const mediaType = item.media_type?.trim().toLowerCase()
  if (mediaType !== 'movie' && mediaType !== 'tv') return false
  const source = item.source?.trim().toLowerCase()
  if (source === 'tmdb') return Boolean(item.tmdb_id && item.tmdb_id > 0)
  if (source === 'douban') return Boolean(item.douban_id?.trim())
  return false
}

export function catalogItemProviderID(item: DiscoverItem): string | number {
  return item.source?.trim().toLowerCase() === 'douban' ? item.douban_id || '' : item.tmdb_id || 0
}

export function mergeDiscoverDetail(item: DiscoverItem, detail: DiscoverItem): DiscoverItem {
  return {
    ...item,
    ...detail,
    title: detail.title?.trim() || item.title,
    original_name: detail.original_name?.trim() || item.original_name,
    overview: detail.overview?.trim() || item.overview,
    poster_url: detail.poster_url?.trim() || item.poster_url,
    backdrop_url: detail.backdrop_url?.trim() || item.backdrop_url,
    preview_images: detail.preview_images?.length ? detail.preview_images : item.preview_images,
    release_date: detail.release_date?.trim() || item.release_date,
    year: detail.year && detail.year > 0 ? detail.year : item.year,
    rating: detail.rating && detail.rating > 0 ? detail.rating : item.rating,
    duration_minutes: detail.duration_minutes && detail.duration_minutes > 0
      ? detail.duration_minutes
      : item.duration_minutes,
    maker: detail.maker?.trim() || item.maker,
    people: detail.people?.length ? detail.people : item.people,
    genres: discoverItemValues(detail.genres).length > 0 ? detail.genres : item.genres,
    actors: discoverItemValues(detail.actors).length > 0 ? detail.actors : item.actors,
    directors: discoverItemValues(detail.directors).length > 0 ? detail.directors : item.directors,
    writers: discoverItemValues(detail.writers).length > 0 ? detail.writers : item.writers,
    aliases: discoverItemValues(detail.aliases).length > 0 ? detail.aliases : item.aliases,
    countries: discoverItemValues(detail.countries).length > 0 ? detail.countries : item.countries,
    languages: discoverItemValues(detail.languages).length > 0 ? detail.languages : item.languages,
    in_library: detail.in_library || item.in_library,
    media_id: detail.media_id || item.media_id,
    library_id: detail.library_id || item.library_id,
  }
}

export function discoverItemValues(value: unknown): string[] {
  const values = Array.isArray(value) ? value : typeof value === 'string' ? value.split(',') : []
  return values
    .filter((item): item is string => typeof item === 'string')
    .map((item) => item.trim())
    .filter((item, index, items) => Boolean(item) && items.indexOf(item) === index)
}

export function discoverItemPeople(item: DiscoverItem): DiscoverPerson[] {
  if (item.people?.length) return item.people.filter((person) => Boolean(person.name?.trim()))
  return discoverItemValues(item.actors).map((name) => ({ name }))
}

export function discoverPerformerItem(person: DiscoverPerson): DiscoverItem {
  return {
    source: person.source || 'javdb',
    media_type: 'person',
    title: person.name,
    poster_url: person.image_url,
    provider_url: person.profile_url,
    provider_id: person.source_id,
    people: [person],
    nsfw: true,
  }
}

export function discoverReleaseStatus(releaseDate?: string, now = new Date()): 'upcoming' | 'released' | '' {
  const normalized = releaseDate?.trim() ?? ''
  if (!/^\d{4}-\d{2}-\d{2}$/.test(normalized)) return ''
  const today = [
    now.getFullYear(),
    String(now.getMonth() + 1).padStart(2, '0'),
    String(now.getDate()).padStart(2, '0'),
  ].join('-')
  return normalized > today ? 'upcoming' : 'released'
}
