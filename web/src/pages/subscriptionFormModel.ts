export interface SubscriptionFormValues {
  deliveryMode: 'resource_import' | 'download'
  name: string
  feed: string
  libraryID: string
  libraryRootID: string
  maxImportsPerRun: string
  pollIntervalMinutes: string
  seasonNumber: string
  totalEpisodes: string
  filter: string
  mediaType: string
  mediaCategory: string
  savePath: string
  searchMode: string
  imdbID: string
  resolution: string
  quality: string
  effects: string
  releaseGroups: string
  excludeWords: string
  minSeeders: string
  maxSeeders: string
  minSizeGB: string
  maxSizeGB: string
  freeOnly: boolean
  washEnabled: boolean
  washPriority: string
}

export const defaultSubscriptionExcludeWords =
  'cam,ts,tc,telesync,telecine,hdcam,hdts,枪版,抢先,抢鲜,预告,trailer,sample,hr,h&r,hit and run,hit&run,hit-and-run,禁转,禁止转载,禁下,禁止下载,dovi,dv,dolby vision,dolby,杜比视界,杜比,h265,h.265,h-265,h_265,h 265,hevc,x265,10bit,10-bit,10 bit,hi10p,atmos,truehd,ddp,dd+,eac3'

export const defaultSubscriptionFormValues: SubscriptionFormValues = {
  deliveryMode: 'resource_import',
  name: '',
  feed: '',
  libraryID: '',
  libraryRootID: '',
  maxImportsPerRun: '1',
  pollIntervalMinutes: '15',
  seasonNumber: '1',
  totalEpisodes: '',
  filter: '',
  mediaType: '',
  mediaCategory: '',
  savePath: '',
  searchMode: 'keyword',
  imdbID: '',
  resolution: 'best',
  quality: '',
  effects: '',
  releaseGroups: '',
  excludeWords: '',
  minSeeders: '',
  maxSeeders: '',
  minSizeGB: '',
  maxSizeGB: '',
  freeOnly: false,
  washEnabled: false,
  washPriority: 'balanced',
}
