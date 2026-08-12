import type { DiscoverItem } from '../api/discover'
import { buildSubscribeKeyword, discoverMediaTypeLabel } from './discoverPageModel'
import { resourceSearchAlternateQuery, resourceSearchPrimaryQuery } from './resourceImportModel'

export {
  catalogItemProviderID,
  discoverItemPeople,
  discoverItemValues,
  discoverPerformerItem,
  discoverReleaseStatus,
  mergeDiscoverDetail,
  supportsAdultMovieDetail,
  supportsCatalogItemDetail,
} from './discoverDetailDataModel'

export function discoverSubscriptionKeyword(item: DiscoverItem): string {
  return item.subscribe_keyword || buildSubscribeKeyword(item)
}

export function discoverResourceSearchKeyword(item: DiscoverItem): string {
  return resourceSearchPrimaryQuery(item)
}

export function discoverResourceSearchAlternateKeyword(item: DiscoverItem): string {
  return resourceSearchAlternateQuery(item)
}

export function discoverItemMetaText(item: DiscoverItem): string {
  return [
    discoverMediaTypeLabel(item.media_type),
    item.release_date?.trim() || (item.year && item.year > 0 ? item.year : ''),
    item.rating ? `★ ${item.rating.toFixed(1)}` : '',
  ]
    .filter(Boolean)
    .join(' · ')
}
