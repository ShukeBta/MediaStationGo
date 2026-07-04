import { useState, type ReactNode } from 'react'
import toast from 'react-hot-toast'

import { api } from '../api/client'
import { libraryAPI } from '../api/library'
import { recycleAPI } from '../api/recycle'
import { toolsAPI } from '../api/tools'
import { confirmAction } from '../components/confirmAction'
import type { Library, Media } from '../types'
import { seriesTitle, type SeriesCard } from '../utils/groupSeries'
import { LibraryMovieActions } from './LibraryMovieActions'
import { seriesSourceRoot } from './libraryPageModel'

type UseLibraryAdminActionsOptions = {
  libraryID: string
  role?: string
  library: Library | null
  selectedSeries: SeriesCard | null
  selectedSeriesEpisodes: Media[]
  reloadCurrentLibrary: () => void
  clearSelectedSeries: () => void
  setManualMovie: (media: Media | null) => void
}

export function useLibraryAdminActions({
  libraryID,
  role,
  library,
  selectedSeries,
  selectedSeriesEpisodes,
  reloadCurrentLibrary,
  clearSelectedSeries,
  setManualMovie,
}: UseLibraryAdminActionsOptions) {
  const [scraping, setScraping] = useState(false)
  const [scrapeEpisodeArtwork, setScrapeEpisodeArtwork] = useState(false)
  const [repairing, setRepairing] = useState(false)
  const [seriesToolBusy, setSeriesToolBusy] = useState('')
  const [movieToolBusy, setMovieToolBusy] = useState('')

  const handleScrape = async () => {
    setScraping(true)
    try {
      await libraryAPI.scrape(libraryID, { episode_images: scrapeEpisodeArtwork, refresh_matched: true })
      toast.success('刮削已加入后台队列')
    } catch {
      toast.error('刮削失败')
    } finally {
      setScraping(false)
    }
  }

  const handleRepairRescrape = async () => {
    if (repairing) return
    setRepairing(true)
    try {
      await toolsAPI.repairAndRescrapeLibrary(libraryID, { episode_images: scrapeEpisodeArtwork, refresh_matched: true })
      toast.success('本库修复+重刮已加入后台队列，进度可在任务中查看')
    } catch {
      toast.error('修复+重刮启动失败')
    } finally {
      setRepairing(false)
    }
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
    runSeriesTool('scrape', '整剧智能刮削', (media) =>
      api.post(`/media/${media.id}/scrape`, smartScrapeOptions(scrapeEpisodeArtwork)),
    )
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
      message: `来源：${source}\n目标：自动按元数据选择正确分类库，当前库仅作为就近解析范围。`,
      confirmText: '开始整理',
    }))) return

    setSeriesToolBusy('organize')
    try {
      const result = await toolsAPI.organizeDirectory({
        source_path: source,
        dest_path: library.path,
        scan_after: true,
        scrape_after: true,
      })
      const replaced = result.replaced ?? 0
      const reclassified = result.reclassified ?? 0
      toast.success(`合集整理完成：新增 ${result.organized ?? 0} · 替换 ${replaced} · 纠偏 ${reclassified} · 跳过 ${result.skipped ?? 0}`)
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
    runMovieTool(media, 'scrape', '智能刮削', (item) =>
      api.post(`/media/${item.id}/scrape`, smartScrapeOptions(scrapeEpisodeArtwork)),
    )
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

  const movieActions = (media: Media): ReactNode => {
    if (role !== 'admin') return undefined
    return (
      <LibraryMovieActions
        media={media}
        busy={movieToolBusy.endsWith(`:${media.id}`)}
        onSmartScrape={handleMovieSmartScrape}
        onManualScrape={setManualMovie}
        onProbe={handleMovieProbe}
        onNFO={handleMovieNFO}
        onSoftDelete={handleMovieSoftDelete}
      />
    )
  }

  return {
    scraping,
    scrapeEpisodeArtwork,
    repairing,
    seriesToolBusy,
    setScrapeEpisodeArtwork,
    handleScrape,
    handleRepairRescrape,
    handleSeriesSmartScrape,
    handleSeriesProbe,
    handleSeriesNFO,
    handleSeriesOrganize,
    handleSeriesSoftDelete,
    movieActions,
  }
}

function smartScrapeOptions(episodeImages: boolean) {
  return {
    episode_images: episodeImages,
    refresh_matched: true,
    include_matched: true,
  }
}
