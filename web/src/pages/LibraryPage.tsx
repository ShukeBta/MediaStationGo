import { useEffect, useMemo, useState } from 'react'
import { useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { motion } from 'framer-motion'
import toast from 'react-hot-toast'

import type { Media, Subscription } from '../types'
import { resourceImportsAPI, type EpisodeReplenishmentContext } from '../api/resourceImports'
import { buildResourceImportFeedURL, buildSubscriptionAliases, subscriptionsAPI } from '../api/subscriptions'
import { useAuthStore } from '../stores/auth'
import { seriesTitle, type SeriesCard } from '../utils/groupSeries'
import { LibraryPageDialogs } from './LibraryPageDialogs'
import { LibraryPageHeader } from './LibraryPageHeader'
import { LibraryMediaSections } from './LibraryMediaSections'
import { LibrarySeriesDetailSection } from './LibrarySeriesDetailSection'
import { useLibraryData } from './useLibraryData'
import { useLibraryScanStatus } from './useLibraryScanStatus'
import { useLibrarySeriesSelection } from './useLibrarySeriesSelection'
import { useLibraryAdminActions } from './useLibraryAdminActions'
import { useLibraryResourceImports } from './useLibraryResourceImports'
import { LibraryResourceImportStatus } from './LibraryResourceImportStatus'
import { ResourceSearchDrawer } from './ResourceSearchDrawer'
import { resourceSearchAlternateQuery, resourceSearchPrimaryQuery } from './resourceImportModel'
import { LibraryFilterBar } from './LibraryActorFilter'
import { buildActorFacets, librarySupportsActorFilter, mediaHasActor } from './libraryActorFilterModel'
import { buildCategoryFacets, mediaHasCategory } from './libraryCategoryFilterModel'
import { buildAdultTypeFacets, mediaHasAdultType } from './libraryAdultTypeFilterModel'
import { AITitleCleanupDialog } from '../components/AITitleCleanupDialog'
import { ManualMediaAggregationDialog } from '../components/ManualMediaAggregationDialog'
import { defaultSubscriptionFormValues } from './subscriptionFormModel'
import { followedSeriesKeys } from './subscriptionFollowModel'
import { seriesReplenishmentTargets, type SeriesReplenishmentTarget } from './libraryPageModel'

export function LibraryPage() {
  const { id = '' } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const location = useLocation()
  const navigate = useNavigate()
  const role = useAuthStore((s) => s.user?.role)
  const userID = useAuthStore((s) => s.user?.id ?? '')

  const [manualSeriesScrapeOpen, setManualSeriesScrapeOpen] = useState(false)
  const [seriesMetadataEditOpen, setSeriesMetadataEditOpen] = useState(false)
  const [manualMovie, setManualMovie] = useState<Media | null>(null)
  const [resourceDrawerOpen, setResourceDrawerOpen] = useState(false)
  const [resourceReplenishment, setResourceReplenishment] = useState<EpisodeReplenishmentContext | null>(null)
  const [resourceInitialQuery, setResourceInitialQuery] = useState('')
  const [resourceTaskID, setResourceTaskID] = useState('')
  const [resourceUpgradeMediaID, setResourceUpgradeMediaID] = useState('')
  const [resourceUpgradeScope, setResourceUpgradeScope] = useState<'media' | 'work' | undefined>()
  const [resourceFixedRootID, setResourceFixedRootID] = useState('')
  const [replenishmentTargets, setReplenishmentTargets] = useState<SeriesReplenishmentTarget[] | null>(null)
  const [replenishmentSeason, setReplenishmentSeason] = useState(0)
  const [replenishmentOpening, setReplenishmentOpening] = useState(false)
  const [titleCleanupOpen, setTitleCleanupOpen] = useState(false)
  const [aggregationOpen, setAggregationOpen] = useState(false)
  const [activeSubscriptions, setActiveSubscriptions] = useState<Subscription[]>([])

  // 剧集模式：选中某个剧集后展开详情
  const [selectedSeries, setSelectedSeries] = useState<SeriesCard | null>(null)
  const [selectedSeason, setSelectedSeason] = useState<number | null>(null)

  const {
    library,
    items,
    seriesEpisodeItems,
    total,
    loading,
    loadingSeriesEpisodes,
    isSeriesLibrary,
    isSeries,
    seriesCards,
    loadingAllText,
    reloadCurrentLibrary,
  } = useLibraryData(id, selectedSeries)

  const {
    scanning,
    scanProgress,
    handleScan,
  } = useLibraryScanStatus({
    libraryID: id,
    isAdmin: role === 'admin',
    onLibraryChanged: reloadCurrentLibrary,
  })

  const {
    selectedEpisodes,
    visibleEpisodes,
    selectedSeriesEpisodes,
    selectedSeriesMediaIDs,
    handleSeriesClick,
    clearSelectedSeries,
  } = useLibrarySeriesSelection({
    items,
    seriesEpisodeItems,
    isSeriesLibrary,
    isSeries,
    loading,
    seriesCards,
    searchParams,
    setSearchParams,
    selectedSeries,
    setSelectedSeries,
    selectedSeason,
    setSelectedSeason,
    onClearSeriesState: () => setSeriesMetadataEditOpen(false),
  })

  const {
    scraping,
    repairing,
    seriesToolBusy,
    handleScrape,
    handleRepairRescrape,
    handleSeriesSmartScrape,
    handleSeriesProbe,
    handleSeriesNFO,
    handleSeriesOrganize,
    handleSeriesSoftDelete,
    movieActions,
  } = useLibraryAdminActions({
    libraryID: id,
    role,
    library,
    selectedSeries,
    selectedSeriesEpisodes,
    reloadCurrentLibrary,
    clearSelectedSeries,
    setManualMovie,
  })

  const resourceImports = useLibraryResourceImports(id, userID, reloadCurrentLibrary)
  const supportsActorFilter = librarySupportsActorFilter(library?.type)
  const actorFacets = useMemo(
    () => supportsActorFilter && !isSeries ? buildActorFacets(items) : [],
    [isSeries, items, supportsActorFilter],
  )
  const requestedActor = searchParams.get('actor')?.trim() ?? ''
  const selectedActor = supportsActorFilter ? requestedActor : ''
  const selectedCategory = searchParams.get('category')?.trim() ?? ''
  const supportsAdultTypeFilter = library?.type === 'adult'
  const requestedAdultType = searchParams.get('adult_type')?.trim() ?? ''
  const selectedAdultType = supportsAdultTypeFilter ? requestedAdultType : ''
  const adultTypeFacets = useMemo(
    () => supportsAdultTypeFilter && !isSeries ? buildAdultTypeFacets(items) : [],
    [isSeries, items, supportsAdultTypeFilter],
  )
  const categoryFacets = useMemo(
    () => buildCategoryFacets(isSeries ? seriesCards.map((series) => series.rep) : items),
    [isSeries, items, seriesCards],
  )
  const requestedResourceQuery = searchParams.get('resource_query')?.trim() ?? ''
  const filteredItems = useMemo(
    () => items.filter((media) => (
      mediaHasAdultType(media, selectedAdultType)
      && mediaHasActor(media, selectedActor)
      && mediaHasCategory(media, selectedCategory)
    )),
    [items, selectedActor, selectedAdultType, selectedCategory],
  )
  const filteredSeriesCards = useMemo(
    () => seriesCards.filter((series) => mediaHasCategory(series.rep, selectedCategory)),
    [selectedCategory, seriesCards],
  )
  const autoFollowedSeries = useMemo(
    () => followedSeriesKeys(library, seriesCards, activeSubscriptions),
    [activeSubscriptions, library, seriesCards],
  )

  useEffect(() => {
    if (role !== 'admin') {
      setActiveSubscriptions([])
      return
    }
    let cancelled = false
    subscriptionsAPI.list().then((items) => {
      if (!cancelled) setActiveSubscriptions(items)
    }).catch(() => {
      if (!cancelled) toast.error('自动追更标识加载失败')
    })
    return () => {
      cancelled = true
    }
  }, [role])

  useEffect(() => {
    if (loading || !library || !requestedActor) return
    if (supportsActorFilter && (loadingAllText || actorFacets.some((actor) => actor.name === requestedActor))) return
    const next = new URLSearchParams(searchParams)
    next.delete('actor')
    setSearchParams(next, { replace: true })
  }, [actorFacets, library, loading, loadingAllText, requestedActor, searchParams, setSearchParams, supportsActorFilter])

  useEffect(() => {
    if (loading || loadingAllText || !selectedCategory || categoryFacets.some((category) => category.name === selectedCategory)) return
    const next = new URLSearchParams(searchParams)
    next.delete('category')
    setSearchParams(next, { replace: true })
  }, [categoryFacets, loading, loadingAllText, searchParams, selectedCategory, setSearchParams])

  useEffect(() => {
    if (loading || loadingAllText || !requestedAdultType) return
    if (supportsAdultTypeFilter && adultTypeFacets.some((adultType) => adultType.name === requestedAdultType.toUpperCase())) return
    const next = new URLSearchParams(searchParams)
    next.delete('adult_type')
    setSearchParams(next, { replace: true })
  }, [adultTypeFacets, loading, loadingAllText, requestedAdultType, searchParams, setSearchParams, supportsAdultTypeFilter])

  useEffect(() => {
    if (loading || !library || !requestedResourceQuery) return
    setResourceInitialQuery(requestedResourceQuery)
    setResourceTaskID('')
    setResourceUpgradeMediaID('')
    setResourceUpgradeScope(undefined)
    setResourceFixedRootID('')
    setResourceReplenishment(null)
    setResourceDrawerOpen(true)
    const next = new URLSearchParams(searchParams)
    next.delete('resource_query')
    setSearchParams(next, { replace: true })
  }, [library, loading, requestedResourceQuery, searchParams, setSearchParams])

  const selectActor = (actor: string) => {
    const next = new URLSearchParams(searchParams)
    if (actor) next.set('actor', actor)
    else next.delete('actor')
    setSearchParams(next)
  }

  const selectCategory = (category: string) => {
    const next = new URLSearchParams(searchParams)
    if (category) next.set('category', category)
    else next.delete('category')
    setSearchParams(next)
  }

  const selectAdultType = (adultType: string) => {
    const next = new URLSearchParams(searchParams)
    if (adultType) next.set('adult_type', adultType)
    else next.delete('adult_type')
    setSearchParams(next)
  }

  const openSeriesUpgrade = () => {
    if (!library || !selectedSeries) return
    const media = selectedSeries.rep
    const enabledRoots = (library.roots ?? []).filter((root) => root.enabled)
    const rootID = enabledRoots.some((root) => root.id === media.library_root_id)
      ? media.library_root_id ?? ''
      : enabledRoots.length === 1 ? enabledRoots[0].id : ''
    if (!rootID) {
      toast.error('当前剧集缺少明确的媒体库目录，无法升级片源')
      return
    }
    setResourceInitialQuery(resourceSearchPrimaryQuery({ ...media, title: seriesTitle(media) }))
    setResourceUpgradeMediaID(media.id)
    setResourceUpgradeScope('work')
    setResourceFixedRootID(rootID)
    setResourceReplenishment(null)
    setResourceTaskID('')
    setResourceDrawerOpen(true)
  }

  const openReplenishmentTarget = async (target: SeriesReplenishmentTarget) => {
    if (replenishmentOpening) return
    setReplenishmentOpening(true)
    try {
      const context = await resourceImportsAPI.replenishmentContext(target.media.id)
      setResourceInitialQuery(context.title)
      setResourceTaskID('')
      setResourceUpgradeMediaID('')
      setResourceUpgradeScope(undefined)
      setResourceFixedRootID(context.root_id)
      setResourceReplenishment(context)
      setReplenishmentTargets(null)
      setResourceDrawerOpen(true)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '加载补集入口失败')
    } finally {
      setReplenishmentOpening(false)
    }
  }

  const openSeriesReplenish = () => {
    if (!library || !selectedSeries || replenishmentOpening) return
    const season = selectedSeason ?? selectedEpisodes[0]?.season ?? 0
    const targets = seriesReplenishmentTargets(selectedSeriesEpisodes, season)
    if (targets.length === 0) {
      toast.error('当前剧集缺少明确的季集信息，无法补集')
      return
    }
    if (targets.length === 1) {
      void openReplenishmentTarget(targets[0])
      return
    }
    setReplenishmentSeason(season)
    setReplenishmentTargets(targets)
  }

  const configureSeriesFollow = () => {
    if (!library || !selectedSeries) return
    const roots = [...new Set(selectedSeriesEpisodes.map((media) => media.library_root_id).filter(Boolean))]
    const rootID = roots.length === 1 ? roots[0] : ''
    if (!rootID) {
      toast.error('当前剧集没有唯一的媒体库目录，无法创建自动追更')
      return
    }
    const target = selectedSeriesEpisodes.find((media) => media.season_num > 0 && media.episode_num > 0) ?? selectedSeries.rep
    if (target.season_num <= 0 || target.episode_num <= 0) {
      toast.error('当前剧集缺少明确的季集信息，无法创建自动追更')
      return
    }
    const title = seriesTitle(selectedSeries.rep)
    navigate('/subscriptions', {
      state: {
        subscriptionDraft: {
          ...defaultSubscriptionFormValues,
          name: title,
          feed: buildResourceImportFeedURL(buildSubscriptionAliases({
            title,
            original_name: selectedSeries.rep.original_name,
            year: selectedSeries.rep.year,
          })),
          libraryID: library.id,
          libraryRootID: rootID,
          seasonNumber: String(target.season_num || 1),
          filter: title,
          mediaType: library.type,
        },
      },
    })
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
      <LibraryPageHeader
        library={library}
        itemCount={isSeries
          ? selectedCategory ? filteredSeriesCards.length : seriesCards.length
          : selectedActor || selectedAdultType || selectedCategory ? filteredItems.length : total}
        loadingAllText={loadingAllText}
        scanProgress={scanProgress}
        isAdmin={role === 'admin'}
        scanning={scanning}
        scraping={scraping}
        repairing={repairing}
        canCleanTitles={role === 'admin' && library?.title_mode === 'filename'}
        canManageAggregation={role === 'admin' && !isSeriesLibrary}
        onScan={handleScan}
        onScrape={handleScrape}
        onRepairRescrape={handleRepairRescrape}
        onCleanTitles={() => setTitleCleanupOpen(true)}
        onManageAggregation={() => setAggregationOpen(true)}
        onResourceSearch={() => {
          setResourceInitialQuery('')
          setResourceTaskID('')
          setResourceUpgradeMediaID('')
          setResourceUpgradeScope(undefined)
          setResourceFixedRootID('')
          setResourceReplenishment(null)
          setResourceDrawerOpen(true)
        }}
      />

      <LibraryResourceImportStatus
        activeTasks={resourceImports.activeTasks}
        latestCompletedTask={resourceImports.latestCompletedTask}
        loading={resourceImports.loading}
        error={resourceImports.error}
        onOpenTask={(task) => {
          setResourceTaskID(task.id)
          setResourceUpgradeMediaID(task.upgrade_media_id ?? '')
          setResourceUpgradeScope(task.upgrade_scope)
          setResourceFixedRootID(task.root_id ?? '')
          setResourceDrawerOpen(true)
        }}
        onDismissCompleted={resourceImports.dismissCompletedTask}
        onRetryLoad={() => void resourceImports.refresh()}
      />

      <LibraryFilterBar
        categories={categoryFacets}
        selectedCategory={selectedCategory}
        onCategoryChange={selectCategory}
        adultTypes={adultTypeFacets}
        selectedAdultType={selectedAdultType}
        onAdultTypeChange={selectAdultType}
        actors={supportsActorFilter ? actorFacets : []}
        selectedActor={selectedActor}
        onActorChange={selectActor}
      />

      <LibraryMediaSections
        isSeries={isSeries}
        items={filteredItems}
        seriesCards={filteredSeriesCards}
        selectedSeries={selectedSeries}
        loading={loading}
        movieActions={movieActions}
        onSeriesClick={handleSeriesClick}
        highlightedMediaID={resourceImports.highlightedMediaID}
        followedSeriesKeys={autoFollowedSeries}
      />

      <LibrarySeriesDetailSection
        selectedSeries={selectedSeries}
        selectedEpisodes={selectedEpisodes}
        selectedSeason={selectedSeason}
        visibleEpisodes={visibleEpisodes}
        allEpisodes={selectedSeriesEpisodes}
        loadingEpisodes={loadingSeriesEpisodes}
        playbackFrom={`${location.pathname}${location.search}`}
        isAdmin={role === 'admin'}
        seriesToolBusy={seriesToolBusy}
        onBack={() => {
          setReplenishmentTargets(null)
          clearSelectedSeries()
        }}
        onSmartScrape={handleSeriesSmartScrape}
        onManualScrape={() => setManualSeriesScrapeOpen(true)}
        onMetadataEdit={() => setSeriesMetadataEditOpen(true)}
        onProbe={handleSeriesProbe}
        onNFO={handleSeriesNFO}
        onOrganize={handleSeriesOrganize}
        onSoftDelete={handleSeriesSoftDelete}
        onUpgrade={openSeriesUpgrade}
        canReplenish={Boolean(selectedSeries && selectedSeriesEpisodes.some((media) => media.season_num > 0 && media.episode_num > 0))}
        onReplenish={openSeriesReplenish}
        canFollow={Boolean(selectedSeries && library && ['tv', 'anime', 'variety'].includes(library.type.toLowerCase()))}
        onFollow={configureSeriesFollow}
        autoFollow={Boolean(selectedSeries && autoFollowedSeries.has(selectedSeries.key))}
        onSeasonChange={setSelectedSeason}
      />

      <LibraryPageDialogs
        manualSeriesScrapeOpen={manualSeriesScrapeOpen}
        seriesMetadataEditOpen={seriesMetadataEditOpen}
        manualMovie={manualMovie}
        selectedSeries={selectedSeries}
        selectedSeriesMediaIDs={selectedSeriesMediaIDs}
        libraryType={library?.type}
        onCloseManualSeriesScrape={() => setManualSeriesScrapeOpen(false)}
        onCloseSeriesMetadataEdit={() => setSeriesMetadataEditOpen(false)}
        onCloseManualMovie={() => setManualMovie(null)}
        onApplied={reloadCurrentLibrary}
      />

      <ResourceSearchDrawer
        open={resourceDrawerOpen}
        autoSearch={Boolean(resourceReplenishment)}
        initialQuery={resourceInitialQuery}
        alternateQuery={resourceUpgradeScope === 'work' && selectedSeries
          ? resourceSearchAlternateQuery({ ...selectedSeries.rep, title: seriesTitle(selectedSeries.rep) })
          : undefined}
        upgradeMediaID={resourceUpgradeMediaID || undefined}
        upgradeScope={resourceUpgradeScope}
        replenishment={resourceReplenishment ?? undefined}
        fixedRootID={resourceFixedRootID || undefined}
        canRemoveOldVersion={role === 'admin'}
        libraryID={id}
        libraryName={library?.name ?? '媒体库'}
        libraryRoots={library?.roots ?? []}
        tasks={resourceImports.tasks}
        taskID={resourceTaskID}
        onTaskIDChange={setResourceTaskID}
        onTaskChanged={resourceImports.acceptTask}
        onClose={() => {
          setResourceDrawerOpen(false)
          setResourceInitialQuery('')
          setResourceUpgradeMediaID('')
          setResourceUpgradeScope(undefined)
          setResourceFixedRootID('')
          setResourceReplenishment(null)
        }}
      />

      {replenishmentTargets && (
        <EpisodeReplenishmentTargetDialog
          season={replenishmentSeason}
          targets={replenishmentTargets}
          opening={replenishmentOpening}
          onClose={() => setReplenishmentTargets(null)}
          onSelect={(target) => void openReplenishmentTarget(target)}
        />
      )}

      <AITitleCleanupDialog
        open={titleCleanupOpen}
        libraryID={id}
        libraryName={library?.name ?? '媒体库'}
        onClose={() => setTitleCleanupOpen(false)}
        onApplied={reloadCurrentLibrary}
      />

      <ManualMediaAggregationDialog
        open={aggregationOpen}
        libraryID={id}
        libraryName={library?.name ?? '媒体库'}
        items={items}
        onClose={() => setAggregationOpen(false)}
        onApplied={reloadCurrentLibrary}
      />
    </div>
  )
}

function EpisodeReplenishmentTargetDialog({
  season,
  targets,
  opening,
  onClose,
  onSelect,
}: {
  season: number
  targets: SeriesReplenishmentTarget[]
  opening: boolean
  onClose: () => void
  onSelect: (target: SeriesReplenishmentTarget) => void
}) {
  return (
    <div
      className="fixed inset-0 z-[90] flex items-center justify-center bg-black/45 p-4 backdrop-blur-sm"
      onClick={() => !opening && onClose()}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="选择补集目录"
        className="w-full max-w-xl overflow-hidden rounded-lg border border-white/70 bg-[var(--app-panel)] shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <header className="flex items-center justify-between gap-4 border-b border-gray-200 px-5 py-4">
          <div>
            <h2 className="font-display text-lg font-bold text-ink-600">选择补集目录</h2>
            <p className="mt-1 text-xs text-sand-500">第 {season} 季存在多个实际入库目录，请选择要补入的目录。</p>
          </div>
          <button type="button" className="btn-outline px-3 py-1.5 text-xs" disabled={opening} onClick={onClose}>取消</button>
        </header>
        <div className="space-y-2 p-5">
          {targets.map((target) => (
            <button
              key={`${target.sourceLabel}\u0000${target.media.id}`}
              type="button"
              disabled={opening}
              onClick={() => onSelect(target)}
              className="flex w-full items-center justify-between gap-4 rounded-xl border border-sand-200 bg-white px-4 py-3 text-left transition hover:border-brand-300 hover:bg-brand-50 disabled:cursor-wait disabled:opacity-60"
            >
              <span className="min-w-0">
                <span className="block truncate text-sm font-semibold text-ink-600">{target.sourceLabel}</span>
                <span className="mt-1 block text-xs text-sand-500">当前目录已入库 {target.episodeCount} 集</span>
              </span>
              <span className="shrink-0 text-xs font-semibold text-brand-600">{opening ? '加载中…' : '选择'}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}
