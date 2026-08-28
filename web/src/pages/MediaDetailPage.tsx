import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import toast from 'react-hot-toast'

import { libraryAPI, mediaAPI } from '../api/library'
import { resourceImportsAPI, type EpisodeReplenishmentContext, type ResourceImportTask } from '../api/resourceImports'
import { confirmAction } from '../components/confirmAction'
import { GeneratedArtworkDialog } from '../components/GeneratedArtworkDialog'
import { usePermission } from '../hooks/usePermission'
import { useAuthStore } from '../stores/auth'
import type { Library, MediaPart, MediaVersion } from '../types'
import { MediaDetailBackdrop } from './MediaDetailArtwork'
import {
  MediaDetailBackButton,
  MediaDetailDialogs,
  MediaDetailLoading,
  MediaDetailMainContent,
  MediaDetailMissing,
} from './MediaDetailPageSections'
import { ResourceSearchDrawer } from './ResourceSearchDrawer'
import { mergeResourceImportTasks, resourceSearchAlternateQuery, resourceSearchPrimaryQuery } from './resourceImportModel'
import { useMediaDetailPageState } from './useMediaDetailPageState'

export function MediaDetailPage() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const role = useAuthStore((s) => s.user?.role)
  const canFavorite = usePermission('can_favorite')
  const canExternalPlayer = usePermission('can_external_player')
  const detail = useMediaDetailPageState({ id, navigate, canFavorite })
  const refreshMedia = detail.refresh
  const [upgradeOpening, setUpgradeOpening] = useState(false)
  const [upgradeOpen, setUpgradeOpen] = useState(false)
  const [upgradeLibrary, setUpgradeLibrary] = useState<Library | null>(null)
  const [upgradeRootID, setUpgradeRootID] = useState('')
  const [upgradeTargetID, setUpgradeTargetID] = useState('')
  const [upgradeTasks, setUpgradeTasks] = useState<ResourceImportTask[]>([])
  const [upgradeTaskID, setUpgradeTaskID] = useState('')
  const [versions, setVersions] = useState<MediaVersion[]>([])
  const [versionsLoading, setVersionsLoading] = useState(true)
  const [versionDeletingID, setVersionDeletingID] = useState('')
  const [parts, setParts] = useState<MediaPart[]>([])
  const [partsLoading, setPartsLoading] = useState(true)
  const [moveLibraryOpen, setMoveLibraryOpen] = useState(false)
  const [replenishOpening, setReplenishOpening] = useState(false)
  const [replenishOpen, setReplenishOpen] = useState(false)
  const [replenishLibrary, setReplenishLibrary] = useState<Library | null>(null)
  const [replenishmentContext, setReplenishmentContext] = useState<EpisodeReplenishmentContext | null>(null)
  const [replenishTasks, setReplenishTasks] = useState<ResourceImportTask[]>([])
  const [replenishTaskID, setReplenishTaskID] = useState('')
  const [generatedArtworkOpen, setGeneratedArtworkOpen] = useState(false)

  const loadVersions = useCallback(async () => {
    if (!id) return
    setVersionsLoading(true)
    try {
      const result = await mediaAPI.listVersions(id)
      setVersions(result.items ?? [])
    } finally {
      setVersionsLoading(false)
    }
  }, [id])

  useEffect(() => {
    loadVersions().catch((error: unknown) => {
      setVersions([])
      const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error
      toast.error(message || '片源版本加载失败')
    })
  }, [loadVersions])

  const loadParts = useCallback(async () => {
    if (!id) return
    setPartsLoading(true)
    try {
      const result = await mediaAPI.listParts(id)
      setParts(result.items ?? [])
    } catch {
      setParts([])
    } finally {
      setPartsLoading(false)
    }
  }, [id])

  useEffect(() => {
    loadParts().catch(() => undefined)
  }, [loadParts])

  const refreshAfterMove = useCallback(async () => {
    await Promise.all([refreshMedia(), loadVersions(), loadParts()])
  }, [loadParts, loadVersions, refreshMedia])

  const openUpgrade = useCallback(async () => {
    if (!detail.media || upgradeOpening) return
    const currentMedia = detail.media
    setUpgradeOpening(true)
    try {
      const loadedVersions = (await mediaAPI.listVersions(currentMedia.id)).items ?? []
      setVersions(loadedVersions)
      const primaryVersion = loadedVersions[0]
      const upgradeTarget = primaryVersion ?? currentMedia
      const library = await libraryAPI.get(upgradeTarget.library_id)
      const enabledRoots = (library.roots ?? []).filter((root) => root.enabled)
      const rootID = enabledRoots.some((root) => root.id === upgradeTarget.library_root_id)
        ? upgradeTarget.library_root_id ?? ''
        : enabledRoots.length === 1 ? enabledRoots[0].id : ''
      if (!rootID) throw new Error('当前作品缺少明确的媒体库目录，无法升级片源')
      setUpgradeLibrary(library)
      setUpgradeRootID(rootID)
      setUpgradeTargetID(upgradeTarget.id)
      setUpgradeTaskID('')
      setUpgradeOpen(true)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '加载升级片源入口失败')
    } finally {
      setUpgradeOpening(false)
    }
  }, [detail.media, upgradeOpening])

  const openReplenish = useCallback(async () => {
    if (!detail.media || role !== 'admin' || replenishOpening) return
    setReplenishOpening(true)
    try {
      const [library, context] = await Promise.all([
        libraryAPI.get(detail.media.library_id),
        resourceImportsAPI.replenishmentContext(detail.media.id),
      ])
      if (library.type !== 'tv' && library.type !== 'anime') {
        throw new Error('补集只支持电视剧或动漫媒体库')
      }
      setReplenishLibrary(library)
      setReplenishmentContext(context)
      setReplenishTasks([])
      setReplenishTaskID('')
      setReplenishOpen(true)
    } catch (error) {
      const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error
      toast.error(message || (error instanceof Error ? error.message : '加载补集入口失败'))
    } finally {
      setReplenishOpening(false)
    }
  }, [detail.media, replenishOpening, role])

  const acceptUpgradeTask = useCallback((task: ResourceImportTask) => {
    setUpgradeTasks((current) => mergeResourceImportTasks(current, [task]))
    if (
      task.keep_old_version === false &&
      task.media_id &&
      (task.status === 'completed' || task.status === 'completed_with_warning')
    ) {
      setUpgradeOpen(false)
      navigate(`/media/${task.media_id}`, { replace: true })
    }
  }, [navigate])

  const acceptReplenishTask = useCallback((task: ResourceImportTask) => {
    setReplenishTasks((current) => mergeResourceImportTasks(current, [task]))
  }, [])

  const deleteVersion = useCallback(async (version: MediaVersion) => {
    if (!detail.media || versionDeletingID) return
    const versionName = version.path?.trim() || [
      version.width > 0 && version.height > 0 ? `${version.width}x${version.height}` : '',
      version.container?.toUpperCase(),
      version.video_codec?.toUpperCase(),
    ].filter(Boolean).join(' · ') || '此片源版本'
    const confirmed = await confirmAction({
      title: '删除这个片源版本',
      message: `将「${versionName}」移入回收站？其他版本不受影响，云盘文件会在回收站彻底删除时才移除。`,
      confirmText: '移入回收站',
    })
    if (!confirmed) return
    setVersionDeletingID(version.id)
    try {
      const result = await mediaAPI.deleteVersion(detail.media.id, version.id)
      toast.success('该片源版本已移入回收站')
      if (version.id === detail.media.id) {
        if (result.next_media_id) navigate(`/media/${result.next_media_id}`, { replace: true })
        else detail.goBack()
        return
      }
      await loadVersions()
    } catch (error) {
      const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error
      toast.error(message || '删除片源版本失败')
    } finally {
      setVersionDeletingID('')
    }
  }, [detail, loadVersions, navigate, versionDeletingID])

  if (detail.loading) return <MediaDetailLoading />
  if (!detail.media) return <MediaDetailMissing />
  const media = detail.media
  const upgradeScope = upgradeLibrary?.type === 'tv' || upgradeLibrary?.type === 'anime' ? 'work' : 'media'
  const upgradeSearchMedia = upgradeScope === 'work'
    ? media
    : versions.find((version) => version.id === upgradeTargetID) ?? media

  return (
    <div className="relative overflow-hidden rounded-3xl bg-white border border-gray-200/90 shadow-[0_1px_3px_rgba(0,0,0,0.01),0_1px_2px_rgba(0,0,0,0.015)]">
      <MediaDetailBackdrop media={media} />

      <MediaDetailBackButton onBack={detail.goBack} />

      <MediaDetailMainContent
        media={media}
        isAdmin={role === 'admin'}
        favourite={detail.favourite}
        canFavorite={canFavorite}
        canExternalPlayer={canExternalPlayer}
        onToggleFavourite={detail.toggleFavourite}
        onUpgrade={() => void openUpgrade()}
        upgradeOpening={upgradeOpening}
        canReplenish={role === 'admin' && Boolean(media.series_id && media.season_num > 0 && media.episode_num > 0)}
        replenishOpening={replenishOpening}
        onReplenish={() => void openReplenish()}
        onSmartScrape={detail.rescrape}
        onManualScrape={() => detail.setManualScrapeOpen(true)}
        onMetadataEdit={() => detail.setMetadataEditOpen(true)}
        onOrganize={() => detail.setOrganizeOpen(true)}
        onMoveLibrary={() => setMoveLibraryOpen(true)}
        onProbe={detail.reprobe}
        onGenerateArtwork={() => setGeneratedArtworkOpen(true)}
        onExportNFO={detail.exportNFO}
        onSoftDelete={detail.softDelete}
        versions={versions}
        versionsLoading={versionsLoading}
        parts={parts}
        partsLoading={partsLoading}
        versionDeletingID={versionDeletingID}
        onDeleteVersion={(version) => void deleteVersion(version)}
      />
      <MediaDetailDialogs
        media={media}
        manualScrapeOpen={detail.manualScrapeOpen}
        metadataEditOpen={detail.metadataEditOpen}
        organizeOpen={detail.organizeOpen}
        moveLibraryOpen={moveLibraryOpen}
        onManualScrapeClose={() => detail.setManualScrapeOpen(false)}
        onMetadataEditClose={() => detail.setMetadataEditOpen(false)}
        onOrganizeClose={() => detail.setOrganizeOpen(false)}
        onMoveLibraryClose={() => setMoveLibraryOpen(false)}
        onManualScrapeApplied={detail.refresh}
        onMetadataSaved={detail.handleMetadataSaved}
        onOrganized={detail.refresh}
        onMoved={refreshAfterMove}
      />
      <GeneratedArtworkDialog
        open={generatedArtworkOpen}
        media={media}
        onClose={() => setGeneratedArtworkOpen(false)}
        onGenerated={detail.handleMetadataSaved}
      />
      {replenishOpen && replenishLibrary && replenishmentContext && (
        <ResourceSearchDrawer
          open
          autoSearch
          initialQuery={replenishmentContext.title}
          replenishment={replenishmentContext}
          fixedRootID={replenishmentContext.root_id}
          libraryID={replenishLibrary.id}
          libraryName={replenishLibrary.name}
          libraryRoots={replenishLibrary.roots ?? []}
          tasks={replenishTasks}
          taskID={replenishTaskID}
          onTaskIDChange={setReplenishTaskID}
          onTaskChanged={acceptReplenishTask}
          onClose={() => {
            setReplenishOpen(false)
            setReplenishmentContext(null)
          }}
        />
      )}
      {upgradeLibrary && (
        <ResourceSearchDrawer
          open={upgradeOpen}
          initialQuery={resourceSearchPrimaryQuery(upgradeSearchMedia)}
          alternateQuery={resourceSearchAlternateQuery(upgradeSearchMedia)}
          upgradeMediaID={upgradeTargetID || media.id}
          upgradeScope={upgradeScope}
          fixedRootID={upgradeRootID}
          canRemoveOldVersion={upgradeScope === 'work'
            ? role === 'admin'
            : role === 'admin' || Boolean(versions.find((version) => version.id === upgradeTargetID)?.can_manage)}
          libraryID={upgradeLibrary.id}
          libraryName={upgradeLibrary.name}
          libraryRoots={upgradeLibrary.roots ?? []}
          tasks={upgradeTasks}
          taskID={upgradeTaskID}
          onTaskIDChange={setUpgradeTaskID}
          onTaskChanged={acceptUpgradeTask}
          onClose={() => setUpgradeOpen(false)}
        />
      )}
    </div>
  )
}
