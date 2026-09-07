import { useState } from 'react'
import toast from 'react-hot-toast'
import type { Media } from '../types'
import {
  ManualScrapeCandidateList,
  ManualScrapeDialogHeader,
  ManualScrapeSearchControls,
} from './ManualScrapeDialogSections'
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
  const [episodeOverride, setEpisodeOverride] = useState({ mediaId: '', season: '', episode: '' })
  const closeDialog = () => {
    setEpisodeOverride({ mediaId: '', season: '', episode: '' })
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
          {dialog.targetIds.length === 1 && (
            <div className="mb-4 flex flex-wrap items-center gap-3 text-sm">
              <span>季集冲突时可手动指定（留空按路径校验）：</span>
              <input aria-label="指定季号" type="number" min="0" step="1" placeholder="季号"
                className="w-24 rounded border border-sand-200 px-2 py-1"
                value={episodeOverride.mediaId === media.id ? episodeOverride.season : ''}
                onChange={(e) => setEpisodeOverride({ mediaId: media.id, season: e.target.value, episode: episodeOverride.mediaId === media.id ? episodeOverride.episode : '' })} />
              <input aria-label="指定集号" type="number" min="1" step="1" placeholder="集号"
                className="w-24 rounded border border-sand-200 px-2 py-1"
                value={episodeOverride.mediaId === media.id ? episodeOverride.episode : ''}
                onChange={(e) => setEpisodeOverride({ mediaId: media.id, episode: e.target.value, season: episodeOverride.mediaId === media.id ? episodeOverride.season : '' })} />
            </div>
          )}
          <ManualScrapeCandidateList items={dialog.items} applyingKey={dialog.applyingKey} onApply={(item) => {
            if (dialog.targetIds.length === 1 && episodeOverride.mediaId === media.id && (episodeOverride.season || episodeOverride.episode)) {
              const season = Number(episodeOverride.season)
              const episode = Number(episodeOverride.episode)
              if (!episodeOverride.season || !episodeOverride.episode || !Number.isInteger(season) || !Number.isInteger(episode) || season < 0 || episode < 1) {
                toast.error('请同时填写有效的季号和集号')
                return
              }
              void dialog.apply({ ...item, season_num: season, episode_num: episode })
              return
            }
            void dialog.apply(item)
          }} />
        </div>
      </div>
    </div>
  )
}
