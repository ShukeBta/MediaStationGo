import { useCallback, useEffect, useState, type Dispatch, type SetStateAction } from 'react'
import type { NavigateFunction } from 'react-router-dom'
import toast from 'react-hot-toast'

import { api } from '../api/client'
import { mediaAPI } from '../api/library'
import { playbackAPI } from '../api/playback'
import { recycleAPI } from '../api/recycle'
import { confirmAction } from '../components/confirmAction'
import type { Media } from '../types'
import { mediaLibraryBackTarget } from './MediaDetailPageModel'

interface MediaDetailPageStateParams {
  id: string
  navigate: NavigateFunction
}

interface MediaDetailRefreshParams {
  id: string
  setMedia: Dispatch<SetStateAction<Media | null>>
  setFavourite: Dispatch<SetStateAction<boolean>>
  setLoading: Dispatch<SetStateAction<boolean>>
}

interface MediaDetailActionsParams {
  media: Media | null
  scrapeEpisodeArtwork: boolean
  navigate: NavigateFunction
  refresh: () => Promise<void>
  setFavourite: Dispatch<SetStateAction<boolean>>
}

export function useMediaDetailPageState({ id, navigate }: MediaDetailPageStateParams) {
  const [media, setMedia] = useState<Media | null>(null)
  const [favourite, setFavourite] = useState(false)
  const [loading, setLoading] = useState(true)
  const [manualScrapeOpen, setManualScrapeOpen] = useState(false)
  const [metadataEditOpen, setMetadataEditOpen] = useState(false)
  const [organizeOpen, setOrganizeOpen] = useState(false)
  const [scrapeEpisodeArtwork, setScrapeEpisodeArtwork] = useState(false)

  const refresh = useMediaDetailRefresh({ id, setMedia, setFavourite, setLoading })
  const actions = useMediaDetailActions({
    media,
    scrapeEpisodeArtwork,
    navigate,
    refresh,
    setFavourite,
  })

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [refresh])

  const handleMetadataSaved = useCallback(async (next: Media) => {
    setMedia(next)
    await refresh()
  }, [refresh])

  return {
    media,
    favourite,
    loading,
    manualScrapeOpen,
    metadataEditOpen,
    organizeOpen,
    scrapeEpisodeArtwork,
    refresh,
    handleMetadataSaved,
    setManualScrapeOpen,
    setMetadataEditOpen,
    setOrganizeOpen,
    setScrapeEpisodeArtwork,
    ...actions,
  }
}

function useMediaDetailRefresh({
  id,
  setMedia,
  setFavourite,
  setLoading,
}: MediaDetailRefreshParams): () => Promise<void> {
  return useCallback(async () => {
    if (!id) return
    setLoading(true)
    try {
      const nextMedia = await mediaAPI.get(id)
      setMedia(nextMedia)
      const favourites = await playbackAPI.listFavourites().catch(() => [])
      setFavourite(favourites.some((item) => item.id === nextMedia.id))
    } finally {
      setLoading(false)
    }
  }, [id, setFavourite, setLoading, setMedia])
}

function useMediaDetailActions({
  media,
  scrapeEpisodeArtwork,
  navigate,
  refresh,
  setFavourite,
}: MediaDetailActionsParams) {
  const goBack = useCallback(() => goBackFromMediaDetail(media, navigate), [media, navigate])
  const toggleFavourite = useCallback(
    () => toggleMediaFavourite(media, setFavourite),
    [media, setFavourite],
  )
  const rescrape = useCallback(
    () => rescrapeMedia(media, scrapeEpisodeArtwork, refresh),
    [media, refresh, scrapeEpisodeArtwork],
  )
  const reprobe = useCallback(() => reprobeMedia(media, refresh), [media, refresh])
  const exportNFO = useCallback(() => exportMediaNFO(media), [media])
  const softDelete = useCallback(
    () => softDeleteMedia(media, navigate),
    [media, navigate],
  )
  return { goBack, toggleFavourite, rescrape, reprobe, exportNFO, softDelete }
}

function goBackFromMediaDetail(media: Media | null, navigate: NavigateFunction, replace = false): void {
  if (!media) return
  const backTarget = mediaLibraryBackTarget(media)
  if (backTarget) navigate(backTarget, replace ? { replace: true } : undefined)
  else navigate(-1)
}

async function toggleMediaFavourite(
  media: Media | null,
  setFavourite: Dispatch<SetStateAction<boolean>>,
): Promise<void> {
  if (!media) return
  const state = await playbackAPI.toggleFavourite(media.id)
  setFavourite(state)
  toast.success(state ? '已加入我的收藏' : '已取消收藏')
}

async function rescrapeMedia(
  media: Media | null,
  scrapeEpisodeArtwork: boolean,
  refresh: () => Promise<void>,
): Promise<void> {
  if (!media) return
  await api.post(`/media/${media.id}/scrape`, {
    episode_images: scrapeEpisodeArtwork,
    refresh_matched: true,
    include_matched: true,
  })
  toast.success('已触发重新刮削')
  await refresh()
}

async function reprobeMedia(media: Media | null, refresh: () => Promise<void>): Promise<void> {
  if (!media) return
  try {
    const result = await api.post(`/media/${media.id}/probe`)
    if (result.data?.code === 0) toast.success('重新探测成功')
    else toast.error(result.data?.error || '探测失败')
    await refresh()
  } catch (err: unknown) {
    toast.error(apiErrorMessage(err, '探测失败，请检查 ffprobe 是否已安装'))
  }
}

async function exportMediaNFO(media: Media | null): Promise<void> {
  if (!media) return
  try {
    const result = await recycleAPI.exportNFO(media.id)
    toast.success(`NFO 已成功写入 ${result.path}`)
  } catch (err: unknown) {
    toast.error(apiErrorMessage(err, '导出失败'))
  }
}

async function softDeleteMedia(media: Media | null, navigate: NavigateFunction): Promise<void> {
  if (!media) return
  const confirmed = await confirmAction({
    title: '移入回收站',
    message: `将「${media.title}」移至回收站? (磁盘文件保留)`,
    confirmText: '移入回收站',
  })
  if (!confirmed) return
  await recycleAPI.softDelete(media.id)
  toast.success('已移至回收站')
  goBackFromMediaDetail(media, navigate, true)
}

function apiErrorMessage(err: unknown, fallback: string): string {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}
