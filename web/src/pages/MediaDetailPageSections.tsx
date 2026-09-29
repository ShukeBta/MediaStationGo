import { ArrowLeft, CircleArrowUp, CirclePlus, Heart, LoaderCircle, Play, RefreshCw } from 'lucide-react'
import { Link } from 'react-router-dom'

import { ExternalPlayerButton } from '../components/ExternalPlayerButton'
import { ManualScrapeDialog } from '../components/ManualScrapeDialog'
import { MetadataEditDialog } from '../components/MetadataEditDialog'
import { MoveMediaLibraryDialog } from '../components/MoveMediaLibraryDialog'
import { OrganizeMediaDialog } from '../components/OrganizeMediaDialog'
import type { Media, MediaPart, MediaVersion } from '../types'
import { MediaDetailAdminPanel } from './MediaDetailAdminPanel'
import { MediaDetailPoster } from './MediaDetailArtwork'
import { MediaDetailMetadata } from './MediaDetailMetadata'
import { MediaDetailTracks } from './MediaDetailTracks'
import { MediaSTRMTargetPanel } from './MediaSTRMTargetPanel'
import { mediaDetailScrapeMediaType } from './MediaDetailPageModel'
import { MediaDetailVersions } from './MediaDetailVersions'
import { MediaDetailParts } from './MediaDetailParts'
import { MediaDetailSubtitles } from './MediaDetailSubtitles'
import { MediaDetailDanmaku } from './MediaDetailDanmaku'
import '../styles/media.css'

interface MediaDetailPlaybackActionsProps {
  media: Media
  favourite: boolean
  canFavorite: boolean
  canExternalPlayer: boolean
  onToggleFavourite: () => void
  onUpgrade: () => void
  upgradeOpening: boolean
  canReplenish: boolean
  replenishOpening: boolean
  onReplenish: () => void
}

interface MediaDetailMainContentProps extends MediaDetailPlaybackActionsProps {
  isAdmin: boolean
  onSmartScrape: () => void
  onManualScrape: () => void
  onMetadataEdit: () => void
  onOrganize: () => void
  onMoveLibrary: () => void
  onProbe: () => void
  onGenerateArtwork: () => void
  onExportNFO: () => void
  onSoftDelete: () => void
  versions: MediaVersion[]
  versionsLoading: boolean
  parts: MediaPart[]
  partsLoading: boolean
  versionDeletingID: string
  onDeleteVersion: (version: MediaVersion) => void
}

interface MediaDetailDialogsProps {
  media: Media
  manualScrapeOpen: boolean
  metadataEditOpen: boolean
  organizeOpen: boolean
  moveLibraryOpen: boolean
  onManualScrapeClose: () => void
  onMetadataEditClose: () => void
  onOrganizeClose: () => void
  onMoveLibraryClose: () => void
  onManualScrapeApplied: () => void
  onMetadataSaved: (media: Media) => void | Promise<void>
  onOrganized: () => void
  onMoved: () => void | Promise<void>
}

interface MediaDetailManualScrapeDialogProps {
  open: boolean
  media: Media
  onClose: () => void
  onApplied: () => void
}

export function MediaDetailLoading() {
  return (
    <div className="media-detail-loading" role="status">
      <LoaderCircle size={28} className="animate-spin" />
      <span>正在布置你的观影空间…</span>
    </div>
  )
}

export function MediaDetailMissing() {
  return (
    <div className="collection-empty-state">
      <p>媒体资源已被移除或不存在</p>
      <Link to="/libraries" className="btn-outline"><ArrowLeft size={15} />返回媒体库</Link>
    </div>
  )
}

export function MediaDetailBackButton({ onBack }: { onBack: () => void }) {
  return (
    <div className="media-detail-back">
      <button
        type="button"
        onClick={onBack}
        className="media-detail-back-button"
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
  canFavorite,
  canExternalPlayer,
  onToggleFavourite,
  onUpgrade,
  upgradeOpening,
  canReplenish,
  replenishOpening,
  onReplenish,
}: MediaDetailPlaybackActionsProps) {
  return (
    <div className="media-detail-playback-actions">
      <Link to={`/play/${media.id}`} className="btn-primary media-detail-primary-play">
        <Play size={16} fill="currentColor" />
        <span>立即播放</span>
      </Link>

      <Link
        to={`/play/${media.id}?mode=hls`}
        className="btn-outline"
      >
        <RefreshCw size={14} />
        <span>HLS 兼容转码播放</span>
      </Link>

      {canExternalPlayer && <ExternalPlayerButton mediaId={media.id} />}

      <button
        type="button"
        onClick={onUpgrade}
        disabled={upgradeOpening}
        className="btn-outline gap-2"
      >
        {upgradeOpening ? <LoaderCircle size={15} className="animate-spin" /> : <CircleArrowUp size={15} />}
        <span>{media.series_id || media.season_num > 0 || media.episode_num > 0 ? '整剧升级片源' : '升级片源'}</span>
      </button>

      {canReplenish && (
        <button
          type="button"
          onClick={onReplenish}
          disabled={replenishOpening}
          className="btn-outline gap-2"
        >
          {replenishOpening ? <LoaderCircle size={15} className="animate-spin" /> : <CirclePlus size={15} />}
          <span>补集</span>
        </button>
      )}

      {canFavorite && (
        <button
          type="button"
          onClick={onToggleFavourite}
          aria-pressed={favourite}
          className={
            'btn-outline gap-2 ' +
            (favourite
              ? 'media-detail-favourite-active'
              : '')
          }
        >
          <Heart size={14} fill={favourite ? 'currentColor' : 'none'} />
          <span>{favourite ? '取消收藏' : '加入收藏'}</span>
        </button>
      )}
    </div>
  )
}

export function MediaDetailMainContent({
  media,
  isAdmin,
  favourite,
  canFavorite,
  canExternalPlayer,
  onToggleFavourite,
  onUpgrade,
  upgradeOpening,
  canReplenish,
  replenishOpening,
  onReplenish,
  onSmartScrape,
  onManualScrape,
  onMetadataEdit,
  onOrganize,
  onMoveLibrary,
  onProbe,
  onGenerateArtwork,
  onExportNFO,
  onSoftDelete,
  versions,
  versionsLoading,
  parts,
  partsLoading,
  versionDeletingID,
  onDeleteVersion,
}: MediaDetailMainContentProps) {
  return (
    <div className="media-detail-main">
      <MediaDetailPoster media={media} />

      <div className="media-detail-content">
        <MediaDetailMetadata media={media}>
          <MediaDetailPlaybackActions
            media={media}
            favourite={favourite}
            canFavorite={canFavorite}
            canExternalPlayer={canExternalPlayer}
            onToggleFavourite={onToggleFavourite}
            onUpgrade={onUpgrade}
            upgradeOpening={upgradeOpening}
            canReplenish={canReplenish}
            replenishOpening={replenishOpening}
            onReplenish={onReplenish}
          />
        </MediaDetailMetadata>
        <MediaDetailTracks media={media} />
        {isAdmin && <MediaSTRMTargetPanel media={media} />}
        <div className="media-detail-secondary">
          <MediaDetailVersions
            versions={versions}
            loading={versionsLoading}
            isAdmin={isAdmin}
            deletingID={versionDeletingID}
            onDelete={onDeleteVersion}
          />
          {isAdmin && (
            <MediaDetailSubtitles
              mediaId={media.id}
              versions={versions}
              versionsLoading={versionsLoading}
            />
          )}
          <MediaDetailDanmaku
            mediaId={media.id}
            versions={versions}
            versionsLoading={versionsLoading}
          />
          <MediaDetailParts parts={parts} loading={partsLoading} />
          {isAdmin && (
            <MediaDetailAdminPanel
              media={media}
              onSmartScrape={onSmartScrape}
              onManualScrape={onManualScrape}
              onMetadataEdit={onMetadataEdit}
              onOrganize={onOrganize}
              onMoveLibrary={onMoveLibrary}
              onProbe={onProbe}
              onGenerateArtwork={onGenerateArtwork}
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
  moveLibraryOpen,
  onManualScrapeClose,
  onMetadataEditClose,
  onOrganizeClose,
  onMoveLibraryClose,
  onManualScrapeApplied,
  onMetadataSaved,
  onOrganized,
  onMoved,
}: MediaDetailDialogsProps) {
  return (
    <>
      <MediaDetailManualScrapeDialog
        open={manualScrapeOpen}
        media={media}
        onClose={onManualScrapeClose}
        onApplied={onManualScrapeApplied}
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
      <MoveMediaLibraryDialog
        open={moveLibraryOpen}
        media={media}
        onClose={onMoveLibraryClose}
        onMoved={onMoved}
      />
    </>
  )
}

function MediaDetailManualScrapeDialog({
  open,
  media,
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
      onClose={onClose}
      onApplied={onApplied}
    />
  )
}
