import type { Library, Subscription } from '../types'
import { seriesTitle, type SeriesCard } from '../utils/groupSeries.ts'

export function followedSeriesKeys(library: Library | null, seriesCards: SeriesCard[], subscriptions: Subscription[], selection?: { key: string; season: number | null }): Set<string> {
  if (!library) return new Set()
  const active = subscriptions.filter((subscription) => (
    subscription.enabled
    && (
      (subscription.delivery_mode === 'resource_import' && subscription.library_id === library.id)
      || isPTSubscription(subscription)
    )
  ))
  return new Set(seriesCards.filter((series) => active.some((subscription) => subscriptionMatchesSeries(subscription, series, selection?.key === series.key ? selection.season : null))).map((series) => series.key))
}

function subscriptionMatchesSeries(subscription: Subscription, series: SeriesCard, selectedSeason: number | null): boolean {
  const media = series.rep
  const pt = isPTSubscription(subscription)
  if (!pt && (!subscription.library_root_id || subscription.library_root_id !== media.library_root_id)) return false
  const season = selectedSeason && selectedSeason > 0 ? selectedSeason : media.season_num > 0 ? media.season_num : series.linkMedia.season_num > 0 ? series.linkMedia.season_num : 1
  if ((subscription.season_number || 1) !== season) return false
  if (pt && subscription.year && media.year && subscription.year !== media.year) return false
  const titles = [seriesTitle(media), media.title, media.display_title, media.original_name, series.linkMedia.title, series.linkMedia.original_name]
    .map(normalizeFollowText)
    .filter(Boolean)
  return subscriptionFollowTerms(subscription).some((term) => titles.some((title) => pt ? title === term : title.includes(term)))
}

function isPTSubscription(subscription: Subscription): boolean {
  return subscription.delivery_mode === 'download' && subscription.feed_url.startsWith('site-search://')
}

function subscriptionFollowTerms(subscription: Subscription): string[] {
  const aliases: string[] = []
  try {
    const url = new URL(subscription.feed_url)
    if (isPTSubscription(subscription)) aliases.push(url.searchParams.get('keyword') || '')
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
