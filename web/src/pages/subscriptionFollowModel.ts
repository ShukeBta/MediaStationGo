import type { Library, Subscription } from '../types'
import { seriesTitle, type SeriesCard } from '../utils/groupSeries'

export function followedSeriesKeys(library: Library | null, seriesCards: SeriesCard[], subscriptions: Subscription[]): Set<string> {
  if (!library) return new Set()
  const active = subscriptions.filter((subscription) => (
    subscription.enabled
    && subscription.delivery_mode === 'resource_import'
    && subscription.library_id === library.id
  ))
  return new Set(seriesCards.filter((series) => active.some((subscription) => subscriptionMatchesSeries(subscription, series))).map((series) => series.key))
}

function subscriptionMatchesSeries(subscription: Subscription, series: SeriesCard): boolean {
  const media = series.rep
  if (!subscription.library_root_id || subscription.library_root_id !== media.library_root_id) return false
  const season = media.season_num > 0 ? media.season_num : series.linkMedia.season_num > 0 ? series.linkMedia.season_num : 1
  if ((subscription.season_number || 1) !== season) return false
  const titles = [seriesTitle(media), media.title, media.display_title, media.original_name, series.linkMedia.title, series.linkMedia.original_name]
    .map(normalizeFollowText)
    .filter(Boolean)
  return subscriptionFollowTerms(subscription).some((term) => titles.some((title) => title.includes(term)))
}

function subscriptionFollowTerms(subscription: Subscription): string[] {
  const aliases: string[] = []
  try {
    const url = new URL(subscription.feed_url)
    aliases.push(...url.searchParams.getAll('alias'))
    for (const raw of url.searchParams.getAll('aliases')) aliases.push(...raw.split(/[|\r\n\t]/))
  } catch {
    // Invalid URLs are rejected server-side; an old malformed rule simply has no URL aliases here.
  }
  return [...new Set([subscription.name, subscription.filter, subscription.original_name, ...aliases]
    .map(normalizeFollowText)
    .filter(Boolean))]
}

function normalizeFollowText(value?: string): string {
  return String(value || '').toLocaleLowerCase().replace(/[^\p{L}\p{N}]/gu, '')
}
