import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useLocation, useParams, useSearchParams } from 'react-router-dom'
import { motion, AnimatePresence } from 'framer-motion'
import toast from 'react-hot-toast'
import { ArrowLeft, Play, Film, Database, FileText, Search, Sparkles, Trash2, Pencil, FolderInput } from 'lucide-react'

import { libraryAPI } from '../api/library'
import { toolsAPI } from '../api/tools'
import { storageAPI, type CloudScanStatus } from '../api/storage_config'
import { api } from '../api/client'
import { recycleAPI } from '../api/recycle'
import type { Library, Media } from '../types'
import { MediaCard } from '../components/MediaCard'
import { ExternalPlayerButton } from '../components/ExternalPlayerButton'
import { imageURL } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { getSeriesKey, groupSeries, isEpisodeLike, seriesTitle, type SeriesCard } from '../utils/groupSeries'
import { useWebSocket } from '../hooks/useWebSocket'
import { confirmAction } from '../components/ConfirmDialog'
import { ManualScrapeDialog } from '../components/ManualScrapeDialog'
import { MetadataEditDialog } from '../components/MetadataEditDialog'

export function LibraryPage() {
  const { id = '' } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const location = useLocation()
  const role = useAuthStore((s) => s.user?.role)

  const [library, setLibrary] = useState<Library | null>(null)
  const [items, setItems] = useState<Media[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingAll, setLoadingAll] = useState(false)
  const [scanning, setScanning] = useState(false)
  const [scanProgress, setScanProgress] = useState('')
  const [scraping, setScraping] = useState(false)
  const [repairing, setRepairing] = useState(false)
  const [seriesToolBusy, setSeriesToolBusy] = useState('')
  const [manualSeriesScrapeOpen, setManualSeriesScrapeOpen] = useState(false)
  const [seriesMetadataEditOpen, setSeriesMetadataEditOpen] = useState(false)
  const [manualMovie, setManualMovie] = useState<Media | null>(null)
  const [movieToolBusy, setMovieToolBusy] = useState('')

  // 剧集模式：选中某个剧集后展开详情
  const [selectedSeries, setSelectedSeries] = useState<SeriesCard | null>(null)
  const [selectedSeason, setSelectedSeason] = useState<number | null>(null)

  const hasEpisodicItems = useMemo(() => items.some(isEpisodeLike), [items])
  const isSeries = library?.type === 'tv' || library?.type === 'anime' || library?.type === 'variety' || hasEpisodicItems

  // 折叠后的剧集卡片
  const seriesCards = useMemo(() => {
    if (!isSeries || items.length === 0) return []
    return groupSeries(items)
  }, [isSeries, items])

  // 选中的剧集：所有集按季分组
  const selectedEpisodes = useMemo(() => {
    if (!selectedSeries || items.length === 0) return []
    const eps = items.filter((m) => getSeriesKey(m) === selectedSeries.key)
    // 按季分组，按集排序
    const seasons = new Map<number, Media[]>()
    for (const ep of eps) {
      const s = ep.episode_num > 0 ? (ep.season_num ?? 0) : (ep.season_num || 1)
      if (!seasons.has(s)) seasons.set(s, [])
      seasons.get(s)!.push(ep)
    }
    for (const [, list] of seasons) {
      list.sort((a, b) => (a.episode_num || 0) - (b.episode_num || 0))
    }
    return Array.from(seasons.entries())
      .sort(([a], [b]) => a - b)
      .map(([season, episodes]) => ({ season, episodes }))
  }, [selectedSeries, items])

  const visibleEpisodes = useMemo(() => {
    if (selectedSeason == null) return selectedEpisodes[0]?.episodes ?? []
    return selectedEpisodes.find((s) => s.season === selectedSeason)?.episodes ?? []
  }, [selectedEpisodes, selectedSeason])

  const selectedSeriesEpisodes = useMemo(
    () => selectedEpisodes.flatMap((season) => season.episodes),
    [selectedEpisodes],
  )

  const selectedSeriesMediaIDs = useMemo(
    () => selectedSeriesEpisodes.map((ep) => ep.id),
    [selectedSeriesEpisodes],
  )

  useEffect(() => {
    if (!id) return
    libraryAPI.list().then((all) => {
      const lib = all.find((l) => l.id === id) ?? null
      setLibrary(lib)
    })
  }, [id])

  useEffect(() => {
    if (!id || !library) return
    let cancelled = false
    setLoading(true)
    setLoadingAll(true)
    setItems([])
    const loadAll = async () => {
      const pageSize = 2000
      let page = 1
      let collected: Media[] = []
      try {
        for (;;) {
          const d = await libraryAPI.listMedia(id, page, pageSize, { groupVersions: false })
          if (cancelled) return
          collected = collected.concat(d.items)
          setItems(collected)
          setTotal(d.total)
          if (page === 1) setLoading(false)
          if (collected.length >= d.total || d.items.length < pageSize) break
          page += 1
        }
      } finally {
        if (!cancelled) {
          setLoading(false)
          setLoadingAll(false)
        }
      }
    }
    loadAll().catch(() => {
      if (!cancelled) {
        toast.error('媒体库加载失败')
        setLoading(false)
      }
    })
    return () => { cancelled = true }
  }, [id, library])

  const reloadCurrentLibrary = useCallback(() => {
    setLibrary((l) => (l ? { ...l } : l))
  }, [])

  const onRealtimeEvent = useCallback((topic: string, payload: unknown) => {
    if (role !== 'admin') return
    if (topic !== 'scan' || !payload || typeof payload !== 'object') return
    const p = payload as Record<string, unknown>
    if (p.library_id !== id) return
    if (p.error) {
      setScanning(false)
      setScanProgress(`扫描失败：${String(p.error)}`)
      return
    }
    if (p.finished) {
      setScanning(false)
      const elapsed = Number(p.elapsed_seconds ?? p.elapsed ?? 0)
      const elapsedText = elapsed > 0 ? ` · 耗时 ${formatDuration(elapsed)}` : ''
      setScanProgress(`扫描完成：发现 ${p.discovered ?? p.visited ?? 0} · 新增 ${p.added ?? 0} · 更新 ${p.updated ?? 0} · 跳过 ${p.skipped ?? 0}${elapsedText}`)
      reloadCurrentLibrary()
      return
    }
    if (p.queued) {
      setScanning(true)
      setScanProgress(String(p.message ?? '扫描已排队，后台会自动入库'))
      return
    }
    if (p.cloud && p.stage) {
      const stage = p.stage === 'importing' ? '正在入库' : '正在遍历目录'
      const speed = Number(p.files_per_second ?? 0)
      const speedText = speed > 0 ? ` · ${speed.toFixed(speed >= 10 ? 0 : 1)} 个/秒` : ''
      setScanning(true)
      setScanProgress(`${stage}：目录 ${p.dirs ?? 0} · 已发现 ${p.discovered ?? 0} · 已入库 ${p.visited ?? 0}${speedText}`)
    }
  }, [id, reloadCurrentLibrary, role])

  useWebSocket(onRealtimeEvent)

  useEffect(() => {
    if (role !== 'admin' || !id) return
    let cancelled = false
    let terminal = false
    let timer: number | undefined
    const restoreCloudScanStatus = async () => {
      const r = await storageAPI.cloudScanStatus()
      if (cancelled) return
      const status = (r.items ?? []).find((item) => item.library_id === id)
      if (!status) return
      if (status.state === 'running' || status.state === 'queued' || status.state === 'canceling') {
        setScanning(true)
        setScanProgress(formatCloudScanStatus(status))
        return
      }
      if (status.state === 'finished') {
        setScanning(false)
        setScanProgress(formatCloudScanStatus(status))
        reloadCurrentLibrary()
        terminal = true
        if (timer) window.clearInterval(timer)
        return
      }
      if (status.state === 'error' && status.error) {
        setScanning(false)
        setScanProgress(`扫描失败：${status.error}`)
        terminal = true
        if (timer) window.clearInterval(timer)
      }
    }
    restoreCloudScanStatus()
      .catch(() => undefined)
      .finally(() => {
        if (!cancelled && !terminal) {
          timer = window.setInterval(() => {
            restoreCloudScanStatus().catch(() => undefined)
          }, 5000)
        }
      })
    return () => {
      cancelled = true
      if (timer) window.clearInterval(timer)
    }
  }, [id, reloadCurrentLibrary, role])

  useEffect(() => {
    if (loading) return
    if (!isSeries) {
      setSelectedSeries(null)
      setSelectedSeason(null)
      return
    }

    const key = searchParams.get('series')
    if (!key) {
      setSelectedSeries(null)
      return
    }

    const next = seriesCards.find((card) => card.key === key)
    if (next) {
      setSelectedSeries(next)
    } else {
      setSelectedSeries(null)
    }
  }, [isSeries, loading, searchParams, seriesCards])

  useEffect(() => {
    if (!selectedSeries || selectedEpisodes.length === 0) {
      setSelectedSeason(null)
      return
    }
    if (selectedSeason == null || !selectedEpisodes.some((s) => s.season === selectedSeason)) {
      setSelectedSeason(selectedEpisodes[0].season)
    }
  }, [selectedSeries, selectedEpisodes, selectedSeason])

  const handleScan = async () => {
    setScanning(true)
    setScanProgress('正在提交扫描任务…')
    let keepScanning = false
    try {
      const r = await libraryAPI.scan(id)
      if (r.queued) {
        keepScanning = true
        setScanProgress(`${r.message ?? '云盘扫描已在后台运行，发现的媒体会自动加入当前媒体库'}；${r.estimate_message ?? '大目录耗时取决于网盘接口速度'}`)
        toast.success('云盘扫描已加入后台队列')
      } else {
        toast.success(`扫描完成:新增 ${r.added} 项，更新 ${r.updated ?? 0} 项`)
        setScanProgress(`扫描完成：新增 ${r.added} · 更新 ${r.updated ?? 0}`)
        reloadCurrentLibrary()
        setScanning(false)
      }
    } catch {
      toast.error('扫描失败')
      setScanProgress('扫描失败，请查看日志或稍后重试')
      setScanning(false)
    } finally {
      if (!keepScanning) setScanning(false)
    }
  }

  const handleScrape = async () => {
    setScraping(true)
    try {
      await libraryAPI.scrape(id)
      toast.success('刮削已加入后台队列')
    } catch { toast.error('刮削失败') }
    finally { setScraping(false) }
  }

  const handleRepairRescrape = async () => {
    if (repairing) return
    setRepairing(true)
    try {
      await toolsAPI.repairAndRescrapeLibrary(id)
      toast.success('本库修复+重刮已加入后台队列，进度可在任务中查看')
    } catch { toast.error('修复+重刮启动失败') }
    finally { setRepairing(false) }
  }

  const runSeriesTool = async (key: string, label: string, action: (media: Media) => Promise<unknown>) => {
    if (selectedSeriesEpisodes.length === 0) return
    setSeriesToolBusy(key)
    try {
      for (const ep of selectedSeriesEpisodes) {
        await action(ep)
      }
      toast.success(`${label}完成：${selectedSeriesEpisodes.length} 个媒体`)
      reloadCurrentLibrary()
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error || `${label}失败`
      toast.error(msg)
    } finally {
      setSeriesToolBusy('')
    }
  }

  const handleSeriesSmartScrape = () => {
    runSeriesTool('scrape', '整剧智能刮削', (media) => api.post(`/media/${media.id}/scrape`))
  }

  const handleSeriesProbe = () => {
    runSeriesTool('probe', '整剧媒体轨探测', (media) => api.post(`/media/${media.id}/probe`))
  }

  const handleSeriesNFO = () => {
    runSeriesTool('nfo', '整剧 NFO 写出', (media) => recycleAPI.exportNFO(media.id))
  }

  const handleSeriesOrganize = async () => {
    if (!selectedSeries || selectedSeriesEpisodes.length === 0 || !library) return
    const source = seriesSourceRoot(selectedSeriesEpisodes)
    if (!source || source.toLowerCase().startsWith('cloud://')) {
      toast.error('当前合集不是本地文件夹，无法使用本地整理入库')
      return
    }
    if (!(await confirmAction({
      title: '整理当前合集',
      message: `来源：${source}\n目标：${library.path}\n将按当前媒体库类型整理整个文件夹。`,
      confirmText: '开始整理',
    }))) return
    setSeriesToolBusy('organize')
    try {
      const result = await toolsAPI.organizeDirectory({
        source_path: source,
        dest_path: library.path,
        media_type: library.type || 'auto',
        library_id: library.id,
        scan_after: true,
        scrape_after: true,
      })
      const replaced = result.replaced ?? 0
      toast.success(`合集整理完成：新增 ${result.organized ?? 0} · 替换 ${replaced} · 跳过 ${result.skipped ?? 0}`)
      reloadCurrentLibrary()
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error || '合集整理失败'
      toast.error(msg)
    } finally {
      setSeriesToolBusy('')
    }
  }

  const handleSeriesSoftDelete = async () => {
    if (!selectedSeries || selectedSeriesEpisodes.length === 0) return
    if (!(await confirmAction({
      title: '移入回收站',
      message: `将「${seriesTitle(selectedSeries.rep)}」的 ${selectedSeriesEpisodes.length} 个媒体移至回收站? (磁盘文件保留)`,
      confirmText: '移入回收站',
    }))) return
    await runSeriesTool('delete', '整剧移入回收站', (media) => recycleAPI.softDelete(media.id))
    clearSelectedSeries()
  }

  const runMovieTool = async (media: Media, key: string, label: string, action: (media: Media) => Promise<unknown>) => {
    const busyKey = `${key}:${media.id}`
    setMovieToolBusy(busyKey)
    try {
      await action(media)
      toast.success(`${label}完成：${media.title}`)
      reloadCurrentLibrary()
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error || `${label}失败`
      toast.error(msg)
    } finally {
      setMovieToolBusy('')
    }
  }

  const handleMovieSmartScrape = (media: Media) => {
    runMovieTool(media, 'scrape', '智能刮削', (item) => api.post(`/media/${item.id}/scrape`))
  }

  const handleMovieProbe = (media: Media) => {
    runMovieTool(media, 'probe', '媒体轨探测', (item) => api.post(`/media/${item.id}/probe`))
  }

  const handleMovieNFO = (media: Media) => {
    runMovieTool(media, 'nfo', 'NFO 写出', (item) => recycleAPI.exportNFO(item.id))
  }

  const handleMovieSoftDelete = async (media: Media) => {
    if (!(await confirmAction({
      title: '移入回收站',
      message: `将「${media.title}」移至回收站? (磁盘文件保留)`,
      confirmText: '移入回收站',
    }))) return
    await runMovieTool(media, 'delete', '移入回收站', (item) => recycleAPI.softDelete(item.id))
  }

  const movieActions = (media: Media) => {
    if (role !== 'admin') return undefined
    const busy = movieToolBusy.endsWith(`:${media.id}`)
    const buttonClass = 'flex h-8 w-8 items-center justify-center rounded-lg border border-white/70 bg-white/90 text-gray-700 shadow-sm backdrop-blur transition hover:bg-brand-50 hover:text-brand-600 disabled:opacity-50'
    return (
      <>
        <button title="智能刮削" disabled={busy} onClick={() => handleMovieSmartScrape(media)} className={buttonClass}>
          <Sparkles size={13} />
        </button>
        <button title="手动匹配刮削" disabled={busy} onClick={() => setManualMovie(media)} className={buttonClass}>
          <Search size={13} />
        </button>
        <button title="探测媒体轨" disabled={busy} onClick={() => handleMovieProbe(media)} className={buttonClass}>
          <Database size={13} />
        </button>
        <button title="写出本地 NFO" disabled={busy} onClick={() => handleMovieNFO(media)} className={buttonClass}>
          <FileText size={13} />
        </button>
        <button title="移入回收站" disabled={busy} onClick={() => handleMovieSoftDelete(media)} className={`${buttonClass} hover:!bg-red-50 hover:!text-red-500`}>
          <Trash2 size={13} />
        </button>
      </>
    )
  }

  const handleSeriesClick = (card: SeriesCard) => {
    setSelectedSeries(card)
    const next = new URLSearchParams(searchParams)
    next.set('series', card.key)
    setSearchParams(next)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const clearSelectedSeries = () => {
    setSelectedSeries(null)
    setSelectedSeason(null)
    setSeriesMetadataEditOpen(false)
    const next = new URLSearchParams(searchParams)
    next.delete('series')
    setSearchParams(next)
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-32">
        <motion.div animate={{ opacity: [0.4, 1, 0.4] }} transition={{ repeat: Infinity, duration: 2 }} className="flex items-center gap-3">
          <div className="h-2 w-2 rounded-full bg-brand-500" />
          <span className="text-sm text-sand-500">加载中…</span>
        </motion.div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">
            {library?.name ?? '媒体库'}
            <span className="text-sand-500"> ({isSeries ? seriesCards.length : total})</span>
          </h1>
          {library && <p className="text-sm text-ink-50">{library.type} · {library.path}</p>}
          {loadingAll && !loading && total > items.length && (
            <p className="mt-1 text-xs text-sand-500">正在继续加载全部条目：{items.length} / {total}</p>
          )}
          {scanProgress && <p className="mt-1 text-xs text-brand-500">{scanProgress}</p>}
        </div>
        {role === 'admin' && (
          <div className="flex flex-wrap gap-2">
            <button onClick={handleScan} disabled={scanning} className="btn-outline">{scanning ? '扫描中…' : '立即扫描'}</button>
            <button onClick={handleScrape} disabled={scraping} className="btn-outline">{scraping ? '刮削中…' : '刮削元数据'}</button>
            <button onClick={handleRepairRescrape} disabled={repairing} className="btn-outline" title="回填本库占位符外部 ID 并重刮，修正空 ID / 拆集问题">{repairing ? '修复中…' : '修复+重刮本库'}</button>
          </div>
        )}
      </div>

      {/* 非剧集：直接展示海报网格 */}
      {!isSeries && items.length > 0 && (
        <div className="grid grid-cols-3 gap-4 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 xl:grid-cols-7 2xl:grid-cols-8">
          {items.map((m) => (
            <MediaCard key={m.id} media={m} actions={movieActions(m)} />
          ))}
        </div>
      )}

      {!isSeries && items.length === 0 && (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <Film className="h-12 w-12 text-gray-500 mb-4" />
          <p className="text-ink-50">该媒体库暂无内容，触发一次扫描后再来看看</p>
        </div>
      )}

      {/* 剧集模式：折叠卡片网格 */}
      {isSeries && seriesCards.length > 0 && !selectedSeries && (
        <div className="grid grid-cols-3 gap-4 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 xl:grid-cols-7 2xl:grid-cols-8">
          {seriesCards.map((s) => (
            <MediaCard key={s.key} media={s.rep} count={s.count} onClick={() => handleSeriesClick(s)} />
          ))}
        </div>
      )}

      {/* 剧集详情：季/集选择器 */}
      <AnimatePresence mode="wait">
        {selectedSeries && (
          <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0 }}
            className="space-y-6"
          >
            {/* 返回按钮 + 标题 */}
            <div className="flex items-center gap-4">
              <button
                onClick={clearSelectedSeries}
                className="btn-ghost gap-2"
              >
                <ArrowLeft size={16} />
                返回列表
              </button>
              <h2 className="font-display text-2xl font-bold text-ink-600 truncate">
                {seriesTitle(selectedSeries.rep)}
              </h2>
              <span className="text-sm text-sand-500">共 {selectedSeries.count} 集</span>
            </div>

            {/* 海报 + 从第一集开始播放 */}
            <div className="flex flex-col sm:flex-row gap-6">
              <div className="w-40 shrink-0 overflow-hidden rounded-xl bg-sand-200 shadow-card">
                {selectedSeries.rep.poster_url ? (
                  <img src={imageURL(selectedSeries.rep.poster_url)} alt={selectedSeries.rep.title} className="w-full aspect-[2/3] object-cover" referrerPolicy="no-referrer" />
                ) : (
                  <div className="flex items-center justify-center aspect-[2/3] text-gray-500"><Film size={40} /></div>
                )}
              </div>
              <div className="flex-1 space-y-3">
                <p className="text-sm text-ink-50 leading-relaxed">
                  {selectedSeries.rep.overview || '暂无简介'}
                </p>

                {/* 从第一集开始 */}
                {(() => {
                  const firstEps = [...(visibleEpisodes.length > 0 ? visibleEpisodes : selectedEpisodes.flatMap((s) => s.episodes))]
                  firstEps.sort((a, b) =>
                    (a.season_num || 0) - (b.season_num || 0)
                    || (a.episode_num || 0) - (b.episode_num || 0),
                  )
                  const first = firstEps.length > 0 ? firstEps[0] : null
                  return first ? (
                    <div className="flex flex-wrap gap-2">
                      <Link to={`/play/${first.id}`} state={{ from: `${location.pathname}${location.search}` }} className="btn-primary inline-flex">
                        <Play size={16} fill="currentColor" />
                        从第一集开始播放
                      </Link>
                      <ExternalPlayerButton mediaId={first.id} label="外部播放器播放" />
                    </div>
                  ) : null
                })()}

                {role === 'admin' && selectedSeriesEpisodes.length > 0 && (
                  <div className="rounded-2xl border border-sand-200 bg-white/80 p-4 shadow-sm">
                    <p className="mb-3 text-[10px] font-bold uppercase tracking-[0.2em] text-[#c9954a]">系统后台高级控制面板</p>
                    <div className="flex flex-wrap gap-2">
                      <button onClick={handleSeriesSmartScrape} disabled={!!seriesToolBusy} className="btn-outline py-2 px-3.5 text-xs gap-1.5">
                        <Sparkles size={13} className="text-[#c9954a]" />
                        <span>{seriesToolBusy === 'scrape' ? '刮削中…' : '整剧智能刮削'}</span>
                      </button>
                      <button onClick={() => setManualSeriesScrapeOpen(true)} disabled={!!seriesToolBusy} className="btn-outline py-2 px-3.5 text-xs gap-1.5">
                        <Search size={13} className="text-[#c9954a]" />
                        <span>手动匹配整剧</span>
                      </button>
                      <button onClick={() => setSeriesMetadataEditOpen(true)} disabled={!!seriesToolBusy} className="btn-outline py-2 px-3.5 text-xs gap-1.5">
                        <Pencil size={13} />
                        <span>编辑元数据</span>
                      </button>
                      <button onClick={handleSeriesProbe} disabled={!!seriesToolBusy} className="btn-outline py-2 px-3.5 text-xs gap-1.5">
                        <Database size={13} />
                        <span>{seriesToolBusy === 'probe' ? '探测中…' : '探测媒体轨'}</span>
                      </button>
                      <button onClick={handleSeriesNFO} disabled={!!seriesToolBusy} className="btn-outline py-2 px-3.5 text-xs gap-1.5">
                        <FileText size={13} />
                        <span>{seriesToolBusy === 'nfo' ? '写出中…' : '写出本地 NFO'}</span>
                      </button>
                      <button onClick={handleSeriesOrganize} disabled={!!seriesToolBusy} className="btn-outline py-2 px-3.5 text-xs gap-1.5">
                        <FolderInput size={13} />
                        <span>{seriesToolBusy === 'organize' ? '整理中…' : '整理当前合集'}</span>
                      </button>
                      <button onClick={handleSeriesSoftDelete} disabled={!!seriesToolBusy} className="btn-outline py-2 px-3.5 text-xs gap-1.5 !border-red-100 !text-red-500 hover:!bg-red-50 hover:!border-red-200">
                        <Trash2 size={13} />
                        <span>{seriesToolBusy === 'delete' ? '处理中…' : '移入回收站'}</span>
                      </button>
                    </div>
                  </div>
                )}
              </div>
            </div>

            {/* 季 / 集列表 */}
            <div className="space-y-6">
              <div className="flex flex-wrap items-center gap-2">
                {selectedEpisodes.map(({ season, episodes }) => (
                  <button
                    key={season}
                    onClick={() => setSelectedSeason(season)}
                    className={
                      'rounded-xl border px-4 py-2 text-sm font-semibold transition ' +
                      (selectedSeason === season
                        ? 'border-brand-300 bg-brand-50 text-brand-700'
                        : 'border-sand-200 bg-white text-ink-100 hover:border-brand-200 hover:text-brand-600')
                    }
                  >
                    {season === 0 ? '特别篇' : `第 ${season} 季`} · {episodes.length} 集
                  </button>
                ))}
              </div>

              <div>
                <h3 className="mb-3 font-display text-lg font-semibold text-ink-600">
                  {(selectedSeason ?? selectedEpisodes[0]?.season ?? 1) === 0
                    ? '特别篇'
                    : `第 ${selectedSeason ?? selectedEpisodes[0]?.season ?? 1} 季`}
                </h3>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
                  {visibleEpisodes.map((ep) => (
                    <div
                      key={ep.id}
                      className="group flex items-center gap-3 rounded-xl border border-sand-200 bg-white p-3 shadow-card transition-all hover:border-brand-300 hover:shadow-card-hover"
                    >
                      <Link to={`/play/${ep.id}`} state={{ from: `${location.pathname}${location.search}` }} className="flex min-w-0 flex-1 items-center gap-3">
                        <div className="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-brand-50 text-brand-600 font-semibold text-sm">
                          {ep.backdrop_url || ep.poster_url ? (
                            <img
                              src={imageURL(ep.backdrop_url || ep.poster_url || '')}
                              alt=""
                              className="h-full w-full object-cover"
                              referrerPolicy="no-referrer"
                            />
                          ) : (
                            ep.episode_num || '—'
                          )}
                        </div>
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium text-ink-600">
                            {ep.original_name || (ep.episode_num > 0 ? `第 ${ep.episode_num} 集` : ep.title)}
                          </p>
                          <p className="text-xs text-sand-500">
                            {ep.duration_sec > 0
                              ? `${Math.floor(ep.duration_sec / 60)} 分钟`
                              : formatSize(ep.size_bytes)}
                          </p>
                        </div>
                        <Play size={14} className="shrink-0 text-gray-500 opacity-0 transition-opacity group-hover:opacity-100 group-hover:text-brand-500" />
                      </Link>
                      <ExternalPlayerButton mediaId={ep.id} label="外部" compact />
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* 剧集为空 */}
      {isSeries && seriesCards.length === 0 && !loading && (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <Film className="h-12 w-12 text-gray-500 mb-4" />
          <p className="text-ink-50">该库尚未发现任何剧集，触发一次扫描后再来看看</p>
        </div>
      )}

      <ManualScrapeDialog
        open={manualSeriesScrapeOpen}
        media={selectedSeries?.rep ?? null}
        mediaIds={selectedSeriesMediaIDs}
        defaultQuery={selectedSeries ? seriesTitle(selectedSeries.rep) : ''}
        mediaType={library?.type || 'tv'}
        scopeLabel={selectedSeries ? seriesTitle(selectedSeries.rep) : '当前剧集'}
        onClose={() => setManualSeriesScrapeOpen(false)}
        onApplied={reloadCurrentLibrary}
      />
      <MetadataEditDialog
        open={seriesMetadataEditOpen}
        media={selectedSeries?.rep ?? null}
        mediaIds={selectedSeriesMediaIDs}
        mode="series"
        scopeLabel={selectedSeries ? seriesTitle(selectedSeries.rep) : '当前剧集'}
        onClose={() => setSeriesMetadataEditOpen(false)}
        onSaved={reloadCurrentLibrary}
      />
      <ManualScrapeDialog
        open={!!manualMovie}
        media={manualMovie}
        defaultQuery={manualMovie?.title ?? ''}
        mediaType={library?.type || 'movie'}
        scopeLabel={manualMovie?.title ?? '当前电影'}
        onClose={() => setManualMovie(null)}
        onApplied={reloadCurrentLibrary}
      />
    </div>
  )
}

function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return ''
  if (seconds < 60) return `${Math.round(seconds)}秒`
  const minutes = Math.floor(seconds / 60)
  const rest = Math.round(seconds % 60)
  if (minutes < 60) return `${minutes}分${rest}秒`
  const hours = Math.floor(minutes / 60)
  return `${hours}小时${minutes % 60}分`
}

function formatCloudScanStatus(status: CloudScanStatus): string {
  const stage =
    status.state === 'queued' ? '扫描已排队'
      : status.state === 'canceling' ? '正在中断扫描'
        : status.state === 'finished' ? '扫描完成'
          : status.stage === 'importing' ? '正在入库'
            : '正在遍历目录'
  const speed = Number(status.files_per_second ?? 0)
  const speedText = speed > 0 && status.state !== 'finished'
    ? ` · ${speed.toFixed(speed >= 10 ? 0 : 1)} 个/秒`
    : ''
  return `${stage}：目录 ${status.dirs ?? 0} · 已发现 ${status.discovered ?? 0} · 已入库 ${status.visited ?? 0} · 新增 ${status.added ?? 0} · 更新 ${status.updated ?? 0}${speedText}`
}

function seriesSourceRoot(episodes: Media[]): string {
  const firstPath = episodes.find((item) => item.path)?.path ?? ''
  if (!firstPath) return ''
  const dir = dirname(firstPath)
  const base = basename(dir)
  if (/^(?:s\d{1,2}|season[\s._-]*\d{1,2}|第\s*\d{1,2}\s*季|specials?|sp|ova|oad|特别篇|特別篇)$/i.test(base)) {
    return dirname(dir)
  }
  return dir
}

function dirname(value: string): string {
  const index = Math.max(value.lastIndexOf('/'), value.lastIndexOf('\\'))
  return index > 0 ? value.slice(0, index) : ''
}

function basename(value: string): string {
  const index = Math.max(value.lastIndexOf('/'), value.lastIndexOf('\\'))
  return index >= 0 ? value.slice(index + 1) : value
}

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = bytes, i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(1)} ${units[i]}`
}
