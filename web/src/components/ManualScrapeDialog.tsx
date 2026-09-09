import { useRef, useState } from 'react'
import toast from 'react-hot-toast'
import type { Media } from '../types'
import { mediaAPI, type ManualScrapeCandidate, type ScrapePreviewRow, type TMDbScrapeSummary } from '../api/library'
import {
  ManualScrapeCandidateList,
  ManualScrapeDialogHeader,
  ManualScrapeSearchControls,
} from './ManualScrapeDialogSections'
import { ManualScrapePreviewPanel } from './ManualScrapePreviewPanel'
import { useManualScrapeDialogState } from './useManualScrapeDialogState'

interface ManualScrapeDialogProps {
  open: boolean
  media: Media | null
  mediaIds?: string[]
  defaultQuery?: string
  mediaType?: string
  scopeLabel?: string
  onClose: () => void
  onApplied?: () => void
}

export function ManualScrapeDialog({
  open,
  media,
  mediaIds,
  defaultQuery,
  mediaType,
  scopeLabel,
  onClose,
  onApplied,
}: ManualScrapeDialogProps) {
  const [preview, setPreview] = useState<{ mediaId: string; item: ManualScrapeCandidate; rows: ScrapePreviewRow[]; tmdb?: TMDbScrapeSummary } | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const previewGeneration = useRef(0)
  const closeDialog = () => {
    previewGeneration.current++
    setPreviewing(false)
    setPreview(null)
    onClose()
  }
  const dialog = useManualScrapeDialogState({
    open,
    media,
    mediaIds,
    defaultQuery,
    mediaType,
    onClose: closeDialog,
    onApplied,
  })

  if (!open || !media) return null

  const previewMatch = async (item: ManualScrapeCandidate) => {
    const generation = ++previewGeneration.current
    setPreview(null)
    setPreviewing(true)
    try {
      const result = await mediaAPI.previewManualScrape(dialog.targetIds, item)
      if (generation !== previewGeneration.current) return
      setPreview({ mediaId: media.id, item, rows: result.items, tmdb: result.tmdb })
    } catch (error) {
      toast.error((error as { response?: { data?: { error?: string } } }).response?.data?.error || '预览失败，未写入数据')
    } finally { if (generation === previewGeneration.current) setPreviewing(false) }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink-900/40 px-4 py-8 backdrop-blur-sm">
      <div className="flex max-h-[86vh] w-full max-w-4xl flex-col overflow-hidden rounded-2xl border border-sand-200 bg-white shadow-2xl">
        <ManualScrapeDialogHeader title={scopeLabel || media.title} targetCount={dialog.targetIds.length} onClose={closeDialog} />

        <ManualScrapeSearchControls
          query={dialog.query}
          selectedProviders={dialog.selectedProviders}
          searching={dialog.searching}
          onQueryChange={dialog.setQuery}
          onProviderChange={dialog.setSelectedProviders}
          onSearch={dialog.runSearch}
        />

        <div className="flex-1 overflow-y-auto p-5">
          <p className="mb-4 text-sm text-ink-600">
            手动选择以所选条目为准，即使剧名与路径不同也可应用。剧集季集号仍会结合路径与 TMDB 校验；冲突时请核对并修正季集信息后重试。
          </p>
          {previewing && <p role="status">正在按剧、季校验，不写入数据…</p>}
          {preview?.mediaId === media.id && (
            <ManualScrapePreviewPanel
              key={`${preview.mediaId}:${preview.item.source}:${preview.item.tmdb_id || preview.item.title}`}
              title={preview.item.title}
              rows={preview.rows}
              tmdb={preview.tmdb}
              applying={!!dialog.applyingKey}
              onCancel={() => setPreview(null)}
              onValidate={(episodeMappings) => void previewMatch({ ...preview.item, episode_mappings: episodeMappings })}
              onConfirm={() => void dialog.apply({
                ...preview.item,
                expected_revisions: Object.fromEntries(preview.rows.map((row) => [row.media_id, row.revision])),
              })}
            />
          )}
          <ManualScrapeCandidateList items={dialog.items} applyingKey={dialog.applyingKey} onApply={(item) => {
            if (previewing || dialog.applyingKey) return
            void previewMatch(item)
          }} />
        </div>
      </div>
    </div>
  )
}
