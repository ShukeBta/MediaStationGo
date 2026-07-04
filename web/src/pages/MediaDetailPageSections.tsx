import { ArrowLeft, Heart, Play, RefreshCw } from 'lucide-react'
import { Link } from 'react-router-dom'

import { ExternalPlayerButton } from '../components/ExternalPlayerButton'
import { ManualScrapeDialog } from '../components/ManualScrapeDialog'
import { MetadataEditDialog } from '../components/MetadataEditDialog'
import { OrganizeMediaDialog } from '../components/OrganizeMediaDialog'
import type { Media } from '../types'
import { MediaDetailAdminPanel } from './MediaDetailAdminPanel'
import { MediaDetailPoster } from './MediaDetailArtwork'
import { MediaDetailMetadata } from './MediaDetailMetadata'
import { mediaDetailScrapeMediaType } from './MediaDetailPageModel'

interface MediaDetailPlaybackActionsProps {
  media: Media
  favourite: boolean
  onToggleFavourite: () => void
}

interface MediaDetailMainContentProps extends MediaDetailPlaybackActionsProps {
  isAdmin: boolean
  scrapeEpisodeArtwork: boolean
  onScrapeEpisodeArtworkChange: (checked: boolean) => void
  onSmartScrape: () => void
  onManualScrape: () => void
  onMetadataEdit: () => void
  onOrganize: () => void
  onProbe: () => void
  onExportNFO: () => void
  onSoftDelete: () => void
}

interface MediaDetailDialogsProps {
  media: Media
  manualScrapeOpen: boolean
  metadataEditOpen: boolean
  organizeOpen: boolean
  scrapeEpisodeArtwork: boolean
  onManualScrapeClose: () => void
  onMetadataEditClose: () => void
  onOrganizeClose: () => void
  onManualScrapeApplied: () => void
  onMetadataSaved: (media: Media) => void | Promise<void>
  onOrganized: () => void
}

interface MediaDetailManualScrapeDialogProps {
  open: boolean
  media: Media
  episodeArtwork: boolean
  onClose: () => void
  onApplied: () => void
}

export function MediaDetailLoading() {
  return (
    <div className="flex items-center justify-center py-48">
      <div className="h-8 w-8 animate-spin rounded-full border-4 border-gray-100 border-t-gray-900" />
    </div>
  )
}

export function MediaDetailMissing() {
  return (
    <div className="text-center py-24 bg-white rounded-2xl border border-gray-200">
      <p className="text-gray-500">媒体资源已被移除或不存在</p>
    </div>
  )
}

export function MediaDetailBackButton({ onBack }: { onBack: () => void }) {
  return (
    <div className="relative z-20 px-6 pt-6 sm:px-10 sm:pt-8">
      <button
        type="button"
        onClick={onBack}
        className="btn-ghost gap-2 bg-white/80 shadow-sm backdrop-blur hover:bg-white"
      >
        <ArrowLeft size={16} />
        <span>返回媒体库</span>
      </button>
    </div>
  )
}

export function MediaDetailPlaybackActions({
  media,
  favourite,
  onToggleFavourite,
}: MediaDetailPlaybackActionsProps) {
  return (
    <div className="flex flex-wrap gap-3">
      <Link to={`/play/${media.id}`} className="btn-primary px-6 py-3.5 shadow-sm">
        <Play size={16} fill="currentColor" />
        <span>立即播放</span>
      </Link>

      <Link
        to={`/play/${media.id}?mode=hls`}
        className="btn-outline border-brand-500/30 hover:border-brand-500 text-[#c9954a] hover:bg-brand-50 px-5"
      >
        <RefreshCw size={14} className="animate-spin-slow" />
        <span>HLS 兼容转码播放</span>
      </Link>

      <ExternalPlayerButton mediaId={media.id} />

      <button
        onClick={onToggleFavourite}
        className={
          'btn-outline gap-2 ' +
          (favourite
            ? '!border-red-200 !bg-red-50 !text-red-600 hover:!bg-red-100/50'
            : 'hover:border-red-200 hover:text-red-600 hover:bg-red-50/50')
        }
      >
        <Heart size={14} fill={favourite ? 'currentColor' : 'none'} />
        <span>{favourite ? '取消收藏' : '加入收藏'}</span>
      </button>
    </div>
  )
}

export function MediaDetailMainContent({
  media,
  isAdmin,
  favourite,
  scrapeEpisodeArtwork,
  onToggleFavourite,
  onScrapeEpisodeArtworkChange,
  onSmartScrape,
  onManualScrape,
  onMetadataEdit,
  onOrganize,
  onProbe,
  onExportNFO,
  onSoftDelete,
}: MediaDetailMainContentProps) {
  return (
    <div className="relative z-10 p-6 sm:p-10 flex flex-col md:flex-row gap-8 lg:gap-12">
      <MediaDetailPoster media={media} />

      <div className="flex-1 space-y-6">
        <MediaDetailMetadata media={media} />
        <div className="divider border-gray-200/60" />
        <div className="flex flex-col gap-5">
          <MediaDetailPlaybackActions media={media} favourite={favourite} onToggleFavourite={onToggleFavourite} />
          {isAdmin && (
            <MediaDetailAdminPanel
              media={media}
              scrapeEpisodeArtwork={scrapeEpisodeArtwork}
              onScrapeEpisodeArtworkChange={onScrapeEpisodeArtworkChange}
              onSmartScrape={onSmartScrape}
              onManualScrape={onManualScrape}
              onMetadataEdit={onMetadataEdit}
              onOrganize={onOrganize}
              onProbe={onProbe}
              onExportNFO={onExportNFO}
              onSoftDelete={onSoftDelete}
            />
          )}
        </div>
      </div>
    </div>
  )
}

export function MediaDetailDialogs({
  media,
  manualScrapeOpen,
  metadataEditOpen,
  organizeOpen,
  scrapeEpisodeArtwork,
  onManualScrapeClose,
  onMetadataEditClose,
  onOrganizeClose,
  onManualScrapeApplied,
  onMetadataSaved,
  onOrganized,
}: MediaDetailDialogsProps) {
  return (
    <>
      <MediaDetailManualScrapeDialog
        open={manualScrapeOpen}
        media={media}
        onClose={onManualScrapeClose}
        onApplied={onManualScrapeApplied}
        episodeArtwork={scrapeEpisodeArtwork}
      />
      <MetadataEditDialog
        open={metadataEditOpen}
        media={media}
        onClose={onMetadataEditClose}
        onSaved={onMetadataSaved}
      />
      <OrganizeMediaDialog
        open={organizeOpen}
        media={media}
        onClose={onOrganizeClose}
        onOrganized={onOrganized}
      />
    </>
  )
}

function MediaDetailManualScrapeDialog({
  open,
  media,
  episodeArtwork,
  onClose,
  onApplied,
}: MediaDetailManualScrapeDialogProps) {
  return (
    <ManualScrapeDialog
      open={open}
      media={media}
      defaultQuery={media.title}
      mediaType={mediaDetailScrapeMediaType(media)}
      scopeLabel={media.title}
      episodeArtwork={episodeArtwork}
      onClose={onClose}
      onApplied={onApplied}
    />
  )
}
