import type { FormEvent } from 'react'

import type { GenerateSTRMResult, STRMOutputPreset } from '../api/strm'
import type { Library } from '../types'
import type { CloudPlaybackMode } from './strmPageModel'
import {
  PlaybackPreferencePanel,
  PlaybackTogglePanel,
  StrmGenerateForm,
  StrmGenerateHeader,
  StrmGenerateHint,
  StrmGenerateResultPanel,
} from './StrmGenerateSectionParts'

export type StrmGenerateSectionProps = {
  libraries: Library[]
  generateLibraryID: string
  baseURL: string
  outputDir: string
  outputPresets: STRMOutputPreset[]
  cloudPlaybackMode: CloudPlaybackMode
  strmPlaybackEnabled: boolean
  redirectProxyEnabled: boolean
  autoGenerate: boolean
  savingSettings: boolean
  overwrite: boolean
  includeLocal: boolean
  preserveTree: boolean
  refreshLibrary: boolean
  scrapeAfter: boolean
  generating: boolean
  generateResult: GenerateSTRMResult | null
  playbackStatus: string
  onGenerate: (event: FormEvent) => void
  saveSTRMSettings: () => void
  setGenerateLibraryID: (value: string) => void
  setBaseURL: (value: string) => void
  setOutputDir: (value: string) => void
  setCloudPlaybackMode: (value: CloudPlaybackMode) => void
  setStrmPlaybackEnabled: (value: boolean) => void
  setRedirectProxyEnabled: (value: boolean) => void
  setAutoGenerate: (value: boolean) => void
  setOverwrite: (value: boolean) => void
  setIncludeLocal: (value: boolean) => void
  setPreserveTree: (value: boolean) => void
  setRefreshLibrary: (value: boolean) => void
  setScrapeAfter: (value: boolean) => void
}

export function StrmGenerateSection(props: StrmGenerateSectionProps) {
  return (
    <section className="glass-panel space-y-4">
      <StrmGenerateHeader
        playbackStatus={props.playbackStatus}
        strmPlaybackEnabled={props.strmPlaybackEnabled}
        redirectProxyEnabled={props.redirectProxyEnabled}
      />
      <PlaybackTogglePanel {...props} />
      <PlaybackPreferencePanel {...props} />
      <StrmGenerateForm {...props} />
      <StrmGenerateHint />
      <StrmGenerateResultPanel result={props.generateResult} />
    </section>
  )
}
