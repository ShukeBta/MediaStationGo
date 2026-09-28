import { useEffect, useState } from 'react'
import { X } from 'lucide-react'

import { discoverAPI } from '../api/discover'
import type { DiscoverItem } from '../api/discover'
import { imageURL } from '../api/client'
import { discoverItemSource, discoverSourceLabel } from './discoverPageModel'
import {
  DiscoverArtworkPanel,
  DiscoverBackdropPanel,
  DiscoverMetadataPanel,
  DiscoverModalHeader,
  DiscoverOverviewPanel,
  DiscoverPreviewGallery,
} from './DiscoverDetailModalSections'
import {
  catalogItemProviderID,
  mergeDiscoverDetail,
  supportsAdultMovieDetail,
  supportsCatalogItemDetail,
} from './discoverDetailModalModel'
import { DiscoverResourceAction } from './DiscoverResourceAction'

export function DiscoverDetailModal({
  item,
  onClose,
  onSelectPerformer,
}: {
  item: DiscoverItem
  onClose: () => void
  onSelectPerformer?: (item: DiscoverItem) => void
}) {
  const [resolvedItem, setResolvedItem] = useState(item)
  const [detailLoading, setDetailLoading] = useState(() => supportsAdultMovieDetail(item) || supportsCatalogItemDetail(item))
  const [detailError, setDetailError] = useState('')
  const [artworkPreviewURL, setArtworkPreviewURL] = useState('')
  const [resourceSidecarOpen, setResourceSidecarOpen] = useState(false)
  const [resourceSidecarRoot, setResourceSidecarRoot] = useState<HTMLDivElement | null>(null)
  const [titleActionRoot, setTitleActionRoot] = useState<HTMLDivElement | null>(null)
  const source = discoverSourceLabel(discoverItemSource(resolvedItem))

  useEffect(() => {
    setResolvedItem(item)
    setDetailError('')
    setArtworkPreviewURL('')
    setResourceSidecarOpen(false)
    const loadAdultDetail = supportsAdultMovieDetail(item)
    const loadCatalogDetail = supportsCatalogItemDetail(item)
    if (!loadAdultDetail && !loadCatalogDetail) {
      setDetailLoading(false)
      return
    }
    let cancelled = false
    setDetailLoading(true)
    const detailRequest = loadAdultDetail
      ? discoverAPI.adultMovieDetail(item.source!, item.provider_id!, item.original_name!)
      : discoverAPI.itemDetail(item.source!, catalogItemProviderID(item), item.media_type!)
    detailRequest
      .then((detail) => {
        if (!cancelled) setResolvedItem(mergeDiscoverDetail(item, detail))
      })
      .catch((error) => {
        if (cancelled) return
        const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error
        setDetailError(message || '作品详细资料加载失败')
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [item])

  useEffect(() => {
    if (!artworkPreviewURL) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setArtworkPreviewURL('')
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [artworkPreviewURL])

  const metadataPanel = (
    <DiscoverMetadataPanel
      item={resolvedItem}
      loading={detailLoading}
      error={detailError}
      onSelectPerformer={onSelectPerformer}
    />
  )
  const overviewPanel = <DiscoverOverviewPanel overview={resolvedItem.overview} />

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-2 backdrop-blur-sm sm:p-4">
      <div
        className={`flex w-full items-start justify-center gap-4 ${
          resourceSidecarOpen ? 'max-w-[1680px]' : 'max-w-5xl'
        }`}
      >
        <div className="flex max-h-[calc(100dvh-1rem)] min-w-0 flex-[3_1_0%] flex-col overflow-hidden rounded-3xl border border-white/60 bg-white shadow-2xl sm:max-h-[92vh] lg:max-w-5xl">
          <div className="shrink-0 px-4 pt-4 sm:px-5 sm:pt-5">
            <DiscoverModalHeader
              item={resolvedItem}
              source={source}
              onClose={onClose}
              setTitleActionRoot={setTitleActionRoot}
            />
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4 sm:px-5 sm:pb-5">
            {resolvedItem.media_type === 'adult' ? (
              <>
                <div className="grid items-start gap-5 lg:grid-cols-[minmax(340px,42%)_1fr]">
                  <DiscoverArtworkPanel
                    item={resolvedItem}
                    deferred={detailLoading}
                    onPreview={() => setArtworkPreviewURL(resolvedItem.poster_url || '')}
                  />
                  <div className="space-y-5">
                    {metadataPanel}
                    {overviewPanel}
                  </div>
                </div>
                <DiscoverPreviewGallery
                  images={resolvedItem.preview_images}
                  title={resolvedItem.title}
                  onPreview={setArtworkPreviewURL}
                />
                <div className="mt-5 border-t border-gray-200 pt-5">
                  <DiscoverResourceAction
                    item={resolvedItem}
                    sidecarRoot={resourceSidecarRoot}
                    onSidecarOpenChange={setResourceSidecarOpen}
                  />
                </div>
              </>
            ) : (
              <div className="grid items-start gap-5 lg:grid-cols-[minmax(240px,300px)_minmax(0,1fr)]">
                <div className="min-w-0 space-y-4">
                  <DiscoverArtworkPanel item={resolvedItem} onPreview={() => setArtworkPreviewURL(resolvedItem.poster_url || '')} />
                  {overviewPanel}
                  <DiscoverResourceAction
                    item={resolvedItem}
                    sidecarRoot={resourceSidecarRoot}
                    subscriptionActionRoot={titleActionRoot}
                    onSidecarOpenChange={setResourceSidecarOpen}
                  />
                </div>
                <div className="min-w-0 space-y-4">
                  <DiscoverBackdropPanel item={resolvedItem} />
                  {metadataPanel}
                </div>
              </div>
            )}
          </div>
        </div>
        {resourceSidecarOpen && (
          <div
            ref={setResourceSidecarRoot}
            className="hidden min-w-[26rem] flex-[2_1_0%] xl:block xl:max-w-3xl"
          />
        )}
      </div>
      {artworkPreviewURL && (
        <div
          className="fixed inset-0 z-[90] flex items-center justify-center bg-black/90 p-4 sm:p-8"
          role="dialog"
          aria-modal="true"
          aria-label={`${resolvedItem.title} 大图预览`}
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setArtworkPreviewURL('')
          }}
        >
          <img
            src={imageURL(artworkPreviewURL, undefined, { maxWidth: 1920, quality: 92 })}
            alt={resolvedItem.title}
            decoding="async"
            className="max-h-full max-w-full object-contain shadow-2xl"
          />
          <button
            type="button"
            className="absolute right-4 top-4 rounded-full border border-white/30 bg-black/55 p-2.5 text-white transition hover:bg-black/80 sm:right-6 sm:top-6"
            aria-label="关闭大图预览"
            title="关闭"
            onClick={() => setArtworkPreviewURL('')}
          >
            <X size={22} />
          </button>
        </div>
      )}
    </div>
  )
}
