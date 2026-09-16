import type { Subscription, SubscriptionImportJob } from '../types'

export function subscriptionSeriesDetailHref(subscription: Subscription): string {
  const libraryID = subscription.media?.display_library_id || subscription.media?.library_id || subscription.library_id || ''
  const seriesKey = subscription.series_key?.trim() || ''
  return libraryID && seriesKey
    ? `/library/${encodeURIComponent(libraryID)}?series=${encodeURIComponent(seriesKey)}`
    : ''
}

export function subscriptionRuleBadges(subscription: Subscription): string[] {
  if (subscription.delivery_mode === 'resource_import') {
    return [
      `第 ${subscription.season_number || 1} 季`,
      `每轮最多 ${subscription.max_imports_per_run || 2} 集`,
      resolutionLabel(subscription.resolution),
      subscription.quality || '质量不限',
      subscription.effects || '',
      subscription.release_groups ? `发布组 ${subscription.release_groups}` : '',
    ].filter(Boolean)
  }
  const labels = [
    subscription.search_mode === 'imdb' ? `IMDB ${subscription.imdb_id || '未填'}` : '关键词搜索',
    resolutionLabel(subscription.resolution),
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

function resolutionLabel(resolution?: string): string {
  const value = (resolution || '').trim().toLowerCase()
  return value && value !== 'best' ? value : '分辨率自动择优'
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
    const prefix = subscription.delivery_mode === 'resource_import' ? '已入库' : '已下载'
    return missing > 0 ? `${prefix} ${downloaded}/${total} 集，待补 ${missing} 集` : `${prefix} ${downloaded}/${total} 集`
  }
  return `${subscription.delivery_mode === 'resource_import' ? '已入库' : '已下载'} ${downloaded}/未知 集`
}

export function subscriptionImportResultLabel(outcome = '', status = ''): string {
  const outcomeLabels: Record<string, string> = {
    imported: '已入库',
    no_new_episodes: '无新增集',
    rejected: '已拒绝',
    failed: '失败',
    superseded: '已替代',
    canceled: '已取消',
  }
  const normalizedOutcome = outcome.trim().toLowerCase()
  if (outcomeLabels[normalizedOutcome]) return outcomeLabels[normalizedOutcome]

  const statusLabels: Record<string, string> = {
    pending: '排队中',
    queued: '排队中',
    running: '进行中',
    retrying: '重试中',
    canceling: '取消中',
    completed: '已完成',
    completed_with_warning: '已完成（有告警）',
    failed: '失败',
    canceled: '已取消',
    cancelled: '已取消',
  }
  return statusLabels[status.trim().toLowerCase()] || '状态未知'
}

export function subscriptionImportTimeLabel(job: SubscriptionImportJob): string {
  const isImported = (job.outcome || '').trim().toLowerCase() === 'imported'
  const prefix = isImported ? '入库时间' : '结束时间'
  const raw = job.finished_at?.trim()
  if (!raw) {
    const finalStatuses = ['completed', 'completed_with_warning', 'failed', 'canceled', 'cancelled']
    return (job.outcome || finalStatuses.includes(job.status.trim().toLowerCase())) ? `${prefix}：记录缺失` : ''
  }
  const parsed = Date.parse(raw)
  return `${prefix}：${Number.isFinite(parsed) ? new Date(parsed).toLocaleString() : '记录异常'}`
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
