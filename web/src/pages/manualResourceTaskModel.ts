import type { ResourceSearchCandidate, ResourceSearchResponse } from '../api/resourceImports'

export function manualResourcePreviewSelection(preview: ResourceSearchResponse) {
  const candidates = Array.isArray(preview.results) ? preview.results : []
  if (candidates.length !== 1) throw new Error('手动链接解析结果无效')
  const roots = (preview.roots ?? []).filter((root) => root.enabled !== false)
  if (roots.length !== 1) throw new Error('媒体库必须且只能有一个可用入库目录')
  return { candidate: candidates[0], root: roots[0] }
}

export function manualResourceTypeLabel(candidate: ResourceSearchCandidate) {
  if (candidate.resource_type === '115_share') return '115 分享'
  if (candidate.resource_type === 'magnet') return '磁链'
  return candidate.source || '手动链接'
}
