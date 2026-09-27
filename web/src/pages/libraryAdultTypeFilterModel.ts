import type { Media } from '../types'

export interface AdultTypeFacet {
  name: 'AV' | 'FC2'
  count: number
}

const adultTypeOrder: AdultTypeFacet['name'][] = ['AV', 'FC2']

export function buildAdultTypeFacets(items: Media[]): AdultTypeFacet[] {
  const counts = new Map<AdultTypeFacet['name'], number>()
  for (const media of items) {
    const name = normalizeAdultType(media.adult_type)
    if (!name) continue
    counts.set(name, (counts.get(name) ?? 0) + 1)
  }
  return adultTypeOrder
    .filter((name) => counts.has(name))
    .map((name) => ({ name, count: counts.get(name) ?? 0 }))
}

export function mediaHasAdultType(media: Media, adultType: string): boolean {
  const expected = normalizeAdultType(adultType)
  return !expected || normalizeAdultType(media.adult_type) === expected
}

function normalizeAdultType(value?: string): AdultTypeFacet['name'] | '' {
  const normalized = value?.trim().toUpperCase()
  return normalized === 'AV' || normalized === 'FC2' ? normalized : ''
}
