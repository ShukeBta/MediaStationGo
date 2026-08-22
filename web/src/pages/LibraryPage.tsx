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

  const openSeriesReplenish = async () => {
    if (!library || !selectedSeries) return
    const target = selectedSeriesEpisodes.find((media) => media.season_num > 0 && media.episode_num > 0) ?? selectedSeries.rep
    if (target.season_num <= 0 || target.episode_num <= 0) {
      toast.error('当前剧集缺少明确的季集信息，无法补集')
      return
    }
    try {
      const context = await resourceImportsAPI.replenishmentContext(target.id)
      setResourceInitialQuery(context.title)
      setResourceTaskID('')
      setResourceUpgradeMediaID('')
      setResourceUpgradeScope(undefined)
      setResourceFixedRootID(context.root_id)
      setResourceReplenishment(context)
      setResourceDrawerOpen(true)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '加载补集入口失败')
    }
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
        scrapeEpisodeArtwork={scrapeEpisodeArtwork}
        scanning={scanning}
        scraping={scraping}
        repairing={repairing}
        canCleanTitles={role === 'admin' && library?.title_mode === 'filename'}
        canManageAggregation={role === 'admin' && !isSeriesLibrary}
        onScrapeEpisodeArtworkChange={setScrapeEpisodeArtwork}
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
        onBack={clearSelectedSeries}
        onSmartScrape={handleSeriesSmartScrape}
        onManualScrape={() => setManualSeriesScrapeOpen(true)}
        onMetadataEdit={() => setSeriesMetadataEditOpen(true)}
        onProbe={handleSeriesProbe}
        onNFO={handleSeriesNFO}
        onOrganize={handleSeriesOrganize}
        onSoftDelete={handleSeriesSoftDelete}
        onUpgrade={openSeriesUpgrade}
        canReplenish={Boolean(selectedSeries && selectedSeriesEpisodes.some((media) => media.season_num > 0 && media.episode_num > 0))}
        onReplenish={() => void openSeriesReplenish()}
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
        scrapeEpisodeArtwork={scrapeEpisodeArtwork}
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
