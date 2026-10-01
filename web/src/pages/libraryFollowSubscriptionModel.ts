import type { Media } from '../types'
import { defaultSubscriptionFormValues, type SubscriptionFormValues } from './subscriptionFormModel.ts'

export function libraryFollowSubscriptionDraft(input: {
  title: string
  feed: string
  mediaType: string
  selectedSeason: number | null
  representativeSeason: number
  episodes: Pick<Media, 'season_num'>[]
}): SubscriptionFormValues {
  const season = [input.selectedSeason, input.representativeSeason, ...input.episodes.map((media) => media.season_num)]
    .find((value) => Number.isInteger(value) && Number(value) > 0) ?? 1
  return {
    ...defaultSubscriptionFormValues,
    deliveryMode: 'download',
    sourceMode: 'pt',
    name: input.title,
    searchKeyword: input.title,
    feed: input.feed,
    seasonNumber: String(season),
    mediaType: input.mediaType,
  }
}
