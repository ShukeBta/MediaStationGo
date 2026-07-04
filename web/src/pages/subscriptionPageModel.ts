import type { Subscription } from '../types'

export function subscriptionRuleBadges(subscription: Subscription): string[] {
  const labels = [
    subscription.search_mode === 'imdb' ? `IMDB ${subscription.imdb_id || '未填'}` : '关键词搜索',
    subscription.resolution && subscription.resolution !== 'best' ? subscription.resolution : '分辨率不限',
    subscription.quality || '质量不限',
    subscription.effects || '',
    subscription.release_groups ? `发布组 ${subscription.release_groups}` : '',
    subscription.free_only ? '仅免费' : '',
    seedersLabel(subscription),
    sizeLabel(subscription),
    washStatusLabel(subscription),
  ]
  return labels.filter(Boolean)
}

function seedersLabel(subscription: Subscription): string {
  const min = subscription.min_seeders || 0
  const max = subscription.max_seeders || 0
  if (min > 0 && max > 0) return `做种 ${min}-${max}`
  if (min > 0) return `做种 >=${min}`
  if (max > 0) return `做种 <=${max}`
  return ''
}

function sizeLabel(subscription: Subscription): string {
  const min = subscription.min_size_gb || 0
  const max = subscription.max_size_gb || 0
  if (min > 0 && max > 0) return `体积 ${min}-${max}GB`
  if (min > 0) return `体积 >=${min}GB`
  if (max > 0) return `体积 <=${max}GB`
  return ''
}

export function subscriptionProgressLabel(subscription: Subscription): string {
  const isSeries = ['tv', 'anime', 'variety'].includes((subscription.media_type || '').toLowerCase())
  if (!isSeries) {
    if (subscription.in_library) return '本地已入库'
    return (subscription.downloaded_episodes || subscription.local_media_count || 0) > 0 ? '已下载未入库' : '本地未入库'
  }
  const downloaded = subscription.downloaded_episodes || 0
  const total = subscription.total_episodes || 0
  if (total > 0) {
    const missing = subscription.missing_episodes?.length || 0
    return missing > 0 ? `已下载 ${downloaded}/${total} 集，缺 ${missing} 集` : `已下载 ${downloaded}/${total} 集`
  }
  return `已下载 ${downloaded}/未知 集`
}

function washPriorityLabel(priority?: string): string {
  switch (priority) {
    case 'resolution':
      return '分辨率优先'
    case 'quality':
      return '片源质量优先'
    case 'effects':
      return '特效优先'
    case 'seeders':
      return '做种数优先'
    default:
      return '均衡'
  }
}

function washStatusLabel(subscription: Subscription): string {
  if (!subscription.wash_enabled) return '未启用洗版'
  if (!hasExplicitWashCriteria(subscription)) return '洗版待配置'
  return `洗版 ${washPriorityLabel(subscription.wash_priority)}`
}

function hasExplicitWashCriteria(subscription: Subscription): boolean {
  const resolution = (subscription.resolution || '').trim().toLowerCase()
  const quality = (subscription.quality || '').trim().toLowerCase()
  return Boolean(
    (resolution && resolution !== 'best') ||
      (quality && quality !== 'best') ||
      subscription.effects?.trim() ||
      subscription.release_groups?.trim(),
  )
}
