import { useEffect, useMemo, useState } from 'react'
import {
  ChevronDown,
  ChevronUp,
  CornerDownRight,
  GitMerge,
  LoaderCircle,
  Search,
  Unlink,
  X,
} from 'lucide-react'
import toast from 'react-hot-toast'

import { libraryAPI } from '../api/library'
import type { Media } from '../types'
import { compareMediaFilename, mediaFilename } from '../utils/mediaFilename'

type ManualMediaAggregationDialogProps = {
  open: boolean
  libraryID: string
  libraryName: string
  onClose: () => void
  onApplied: () => void | Promise<void>
}

type AggregationTree = {
  key: string
  kind: 'single' | 'version' | 'part'
  title: string
  members: Media[]
  versionCount: number
}

export function ManualMediaAggregationDialog({
  open,
  libraryID,
  libraryName,
  onClose,
  onApplied,
}: ManualMediaAggregationDialogProps) {
  const [query, setQuery] = useState('')
  const [sourceKey, setSourceKey] = useState('')
  const [busyKey, setBusyKey] = useState('')
  const [items, setItems] = useState<Media[]>([])
  const [loadState, setLoadState] = useState('')
  const [loadError, setLoadError] = useState('')
  const [revision, setRevision] = useState(0)
  // Cross-work aggregation needs the complete inventory, but only after the
  // administrator explicitly opens this management dialog, never on browsing.
  useEffect(() => {
    if (!open) return
    const controller = new AbortController()
    setItems([])
    setSourceKey('')
    setLoadError('')
    setLoadState('正在加载聚合作品…')
    void (async () => {
      const collected: Media[] = []
      for (let page = 1; ; page++) {
        const result = await libraryAPI.browse(libraryID, { page }, controller.signal)
        if (controller.signal.aborted) return
        if (result.is_series) throw new Error('剧集库不支持手动聚合')
        if (result.page !== page || (result.items.length === 0 && collected.length < result.total)) {
          throw new Error('媒体库内容发生变化，请重新加载')
        }
        collected.push(...result.items)
        if (collected.length >= result.total) break
        setLoadState(`正在加载聚合作品：${collected.length} / ${result.total}`)
      }
      if (new Set(collected.map((item) => item.id)).size !== collected.length) throw new Error('媒体库内容发生变化，请重新加载')
      setItems(collected)
      setLoadState('')
    })().catch((error: unknown) => {
      if (controller.signal.aborted) return
      setLoadState('')
      setLoadError(aggregationError(error, error instanceof Error ? error.message : '加载聚合作品失败'))
    })
    return () => controller.abort()
  }, [open, libraryID, revision])
  const applied = async () => {
    setItems([])
    setLoadState('正在刷新聚合作品…')
    setRevision((value) => value + 1)
    await onApplied()
  }
  const trees = useMemo(() => buildAggregationTrees(items), [items])
  const source = trees.find((tree) => tree.key === sourceKey) ?? null
  const visibleTrees = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    if (!normalized) return trees
    return trees.filter((tree) => aggregationSearchText(tree).includes(normalized))
  }, [query, trees])

  if (!open) return null

  const applyGroup = async (target: AggregationTree) => {
    if (!source || source.key === target.key) return
    const mediaIDs = [...target.members, ...source.members].map((media) => media.id)
    setBusyKey(`attach:${target.key}`)
    try {
      await libraryAPI.updateAggregation(libraryID, {
        action: 'group',
        media_ids: mediaIDs,
        title: target.title,
      })
      toast.success(`已将「${source.title}」挂到「${target.title}」下`)
      setSourceKey('')
      await applied()
    } catch (error) {
      toast.error(aggregationError(error, '聚合失败'))
    } finally {
      setBusyKey('')
    }
  }

  const convertToParts = async (tree: AggregationTree) => {
    if (tree.kind !== 'version') return
    setBusyKey(`convert:${tree.key}`)
    try {
      await libraryAPI.updateAggregation(libraryID, {
        action: 'group',
        media_ids: tree.members.map((media) => media.id),
        title: tree.title,
      })
      toast.success(`已将「${tree.title}」的 ${tree.members.length} 个版本转为多片段`)
      if (sourceKey === tree.key) setSourceKey('')
      await applied()
    } catch (error) {
      toast.error(aggregationError(error, '转换为多片段失败'))
    } finally {
      setBusyKey('')
    }
  }

  const detach = async (tree: AggregationTree, media: Media) => {
    setBusyKey(`detach:${media.id}`)
    try {
      await libraryAPI.updateAggregation(libraryID, { action: 'detach', media_ids: [media.id] })
      toast.success(`已解除「${mediaTitle(media)}」的聚合关系`)
      if (sourceKey === tree.key) setSourceKey('')
      await applied()
    } catch (error) {
      toast.error(aggregationError(error, '解除聚合失败'))
    } finally {
      setBusyKey('')
    }
  }

  const move = async (tree: AggregationTree, index: number, direction: -1 | 1) => {
    const nextIndex = index + direction
    if (nextIndex < 0 || nextIndex >= tree.members.length) return
    const next = [...tree.members]
    ;[next[index], next[nextIndex]] = [next[nextIndex], next[index]]
    setBusyKey(`move:${tree.key}`)
    try {
      await libraryAPI.updateAggregation(libraryID, {
        action: 'group',
        media_ids: next.map((media) => media.id),
        title: tree.title,
      })
      await applied()
    } catch (error) {
      toast.error(aggregationError(error, '调整顺序失败'))
    } finally {
      setBusyKey('')
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink-900/45 px-3 py-5 backdrop-blur-sm" role="dialog" aria-modal="true">
      <div className="flex max-h-[92vh] w-full max-w-5xl flex-col overflow-hidden rounded-xl border border-gray-200 bg-white shadow-2xl">
        <header className="flex items-start justify-between gap-4 border-b border-gray-200 px-5 py-4">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <GitMerge size={19} className="text-brand-500" />
              <h2 className="font-display text-xl font-bold text-gray-900">手动聚合作品</h2>
            </div>
            <p className="mt-1 text-xs text-gray-500">
              {libraryName} · 将顶层作品归为同一分段作品；多版本作品会完整转换为片段
            </p>
          </div>
          <button type="button" className="btn-ghost h-9 w-9 shrink-0 p-0" onClick={onClose} aria-label="关闭">
            <X size={17} />
          </button>
        </header>

        <div className="border-b border-gray-200 px-5 py-3">
          <div className="flex flex-wrap items-center gap-3">
            <label className="relative min-w-0 flex-1">
              <Search size={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                className="input-base h-10 w-full pl-9"
                placeholder="按标题或文件名查找作品"
              />
            </label>
            <span className="shrink-0 text-xs text-gray-500">{trees.length} 个顶层作品</span>
          </div>
          {source && (
            <div className="mt-3 flex items-center justify-between gap-3 rounded-lg border border-brand-200 bg-brand-50 px-3 py-2 text-sm">
              <span className="min-w-0 truncate text-brand-700">待挂载：{source.title}</span>
              <button type="button" className="text-xs font-semibold text-brand-700" onClick={() => setSourceKey('')}>取消选择</button>
            </div>
          )}
        </div>

        <div className="flex-1 overflow-y-auto p-5">
          {loadState && <div role="status" className="py-16 text-center text-sm text-gray-500">{loadState}</div>}
          {loadError && <div role="alert" className="py-16 text-center text-sm text-red-500">
            {loadError} <button type="button" className="btn-outline ml-3" onClick={() => setRevision((value) => value + 1)}>重新加载</button>
          </div>}
          <div className="space-y-2" role="tree" aria-label="当前作品聚合关系">
            {visibleTrees.map((tree) => {
              const selected = sourceKey === tree.key
              const treeBusy = busyKey.includes(tree.key) || tree.members.some((media) => busyKey.includes(media.id))
              return (
                <div key={tree.key} className={`rounded-lg border ${selected ? 'border-brand-300 bg-brand-50/40' : 'border-gray-200 bg-white'}`}>
                  <div className="flex min-w-0 items-center gap-3 px-3 py-3" role="treeitem" aria-expanded={tree.members.length > 1}>
                    <GitMerge size={16} className={tree.members.length > 1 ? 'shrink-0 text-brand-500' : 'shrink-0 text-gray-300'} />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-semibold text-gray-900">{tree.title}</p>
                      <p className="truncate text-xs text-gray-500">{mediaFilename(tree.members[0])}</p>
                    </div>
                    {tree.members.length > 1 && <span className="shrink-0 text-xs text-gray-500">{tree.members.length} 项</span>}
                    {tree.kind === 'version' && (
                      <span className="shrink-0 rounded bg-amber-50 px-2 py-1 text-xs text-amber-700">
                        {tree.versionCount} 版本 · 可转为片段
                      </span>
                    )}
                    {!source && tree.kind === 'version' && (
                      <button
                        type="button"
                        className="btn-primary h-8 shrink-0 px-2 text-xs"
                        disabled={Boolean(busyKey)}
                        onClick={() => void convertToParts(tree)}
                      >
                        {busyKey === `convert:${tree.key}` && <LoaderCircle size={14} className="animate-spin" />}
                        转为多片段
                      </button>
                    )}
                    {!source && (
                      <button
                        type="button"
                        className="btn-outline h-8 shrink-0 px-2 text-xs"
                        disabled={Boolean(busyKey)}
                        onClick={() => setSourceKey(tree.key)}
                      >
                        选择作品
                      </button>
                    )}
                    {source && source.key !== tree.key && (
                      <button
                        type="button"
                        className="btn-primary h-8 shrink-0 px-2 text-xs"
                        disabled={Boolean(busyKey)}
                        onClick={() => void applyGroup(tree)}
                      >
                        {busyKey === `attach:${tree.key}` ? <LoaderCircle size={14} className="animate-spin" /> : <CornerDownRight size={14} />}
                        挂到这里
                      </button>
                    )}
                    {selected && <span className="shrink-0 text-xs font-semibold text-brand-600">已选择</span>}
                  </div>

                  {tree.members.length > 1 && (
                    <div className="border-t border-gray-100 bg-gray-50/60 px-3 py-2" role="group">
                      {tree.members.map((media, index) => (
                        <div key={media.id} className="flex min-w-0 items-center gap-2 border-b border-gray-100 py-2 pl-6 last:border-b-0">
                          <CornerDownRight size={14} className="shrink-0 text-gray-400" />
                          <span className="w-6 shrink-0 text-center text-xs tabular-nums text-gray-400">{index + 1}</span>
                          <div className="min-w-0 flex-1">
                            <p className="truncate text-sm text-gray-700">{mediaTitle(media)}</p>
                            <p className="truncate text-xs text-gray-400">{mediaFilename(media)}</p>
                          </div>
                          {tree.kind === 'part' && (
                            <>
                              <button
                                type="button"
                                className="icon-button h-8 w-8"
                                title="上移"
                                aria-label={`上移 ${mediaTitle(media)}`}
                                disabled={treeBusy || index === 0}
                                onClick={() => void move(tree, index, -1)}
                              >
                                <ChevronUp size={14} />
                              </button>
                              <button
                                type="button"
                                className="icon-button h-8 w-8"
                                title="下移"
                                aria-label={`下移 ${mediaTitle(media)}`}
                                disabled={treeBusy || index === tree.members.length - 1}
                                onClick={() => void move(tree, index, 1)}
                              >
                                <ChevronDown size={14} />
                              </button>
                              <button
                                type="button"
                                className="icon-button h-8 w-8 text-red-500"
                                title="从当前作品解除"
                                aria-label={`解除 ${mediaTitle(media)}`}
                                disabled={treeBusy}
                                onClick={() => void detach(tree, media)}
                              >
                                {busyKey === `detach:${media.id}` ? <LoaderCircle size={14} className="animate-spin" /> : <Unlink size={14} />}
                              </button>
                            </>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
            {visibleTrees.length === 0 && !loadState && !loadError && (
              <div className="py-16 text-center text-sm text-gray-500">没有匹配的作品</div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function buildAggregationTrees(items: Media[]): AggregationTree[] {
  return items.map((item) => {
    const kind: AggregationTree['kind'] = item.parts?.length ? 'part' : item.versions?.length ? 'version' : 'single'
    const members = kind === 'part'
      ? [...(item.parts ?? [])].sort((left, right) => (left.part_index ?? 0) - (right.part_index ?? 0) || compareMediaFilename(left, right))
      : kind === 'version'
        ? [...(item.versions ?? [])].sort(compareMediaFilename)
        : [item]
    return {
      key: item.part_group_key ? `part:${item.part_group_key}` : `media:${item.id}`,
      kind,
      title: item.part_group_title || item.display_title || item.title,
      members,
      versionCount: item.versions?.length ?? 0,
    }
  })
}

function aggregationSearchText(tree: AggregationTree): string {
  return [tree.title, ...tree.members.flatMap((media) => [mediaTitle(media), mediaFilename(media), media.path])]
    .join('\n')
    .toLowerCase()
}

function mediaTitle(media: Media): string {
  return media.display_title || media.title || mediaFilename(media)
}

function aggregationError(error: unknown, fallback: string): string {
  const payload = (error as { response?: { data?: { error?: string } } })?.response?.data?.error
  return typeof payload === 'string' && payload.trim() ? payload : fallback
}
