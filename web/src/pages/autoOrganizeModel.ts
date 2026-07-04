import type { Setting } from '../types'

export type AutoOrganizeConfig = {
  enabled: string
  afterDownload: string
  scrapeAfter: string
  downloadSmartClassify: string
  smartClassify: string
  autoAddLibrary: string
  sourceDir: string
  targetDir: string
  transferMode: string
  intervalSeconds: string
  keepSeeding: string
  movieFormat: string
  tvFormat: string
  animeFormat: string
  scrapeAutoOnScan: string
  scrapeProviders: string
  scrapeLanguage: string
  scrapeDelayMinMs: string
  scrapeDelayMaxMs: string
}

export const AUTO_ORGANIZE_DEFAULTS: AutoOrganizeConfig = {
  enabled: 'false',
  afterDownload: 'false',
  scrapeAfter: 'true',
  downloadSmartClassify: 'true',
  smartClassify: 'true',
  autoAddLibrary: 'true',
  sourceDir: '',
  targetDir: '',
  transferMode: 'hardlink',
  intervalSeconds: '300',
  keepSeeding: 'true',
  movieFormat: '{title} ({year})/{title} ({year})',
  tvFormat: '{title} ({year})/Season {season:02}/{title} S{season:02}E{episode:02}',
  animeFormat: '{title}/Season {season:02}/{title} S{season:02}E{episode:02}',
  scrapeAutoOnScan: 'false',
  scrapeProviders: 'tmdb,douban,bangumi,thetvdb,fanart',
  scrapeLanguage: 'zh-CN',
  scrapeDelayMinMs: '250',
  scrapeDelayMaxMs: '500',
}

export const AUTO_ORGANIZE_KEYS: Record<keyof AutoOrganizeConfig, string> = {
  enabled: 'organize.auto',
  afterDownload: 'organizer.auto_after_download',
  scrapeAfter: 'organize.scrape_after',
  downloadSmartClassify: 'downloads.smart_classify',
  smartClassify: 'organizer.smart_classify',
  autoAddLibrary: 'organize.auto_add_library',
  sourceDir: 'organize.source_dir',
  targetDir: 'organize.target_dir',
  transferMode: 'organize.transfer_mode',
  intervalSeconds: 'organize.interval_seconds',
  keepSeeding: 'organize.keep_seeding',
  movieFormat: 'organize.movie_format',
  tvFormat: 'organize.tv_format',
  animeFormat: 'organize.anime_format',
  scrapeAutoOnScan: 'scrape.auto_on_scan',
  scrapeProviders: 'scrape.providers',
  scrapeLanguage: 'scrape.language',
  scrapeDelayMinMs: 'scrape.delay_min_ms',
  scrapeDelayMaxMs: 'scrape.delay_max_ms',
}

export type AutoOrganizeTab = 'basic' | 'naming' | 'scrape'

export function mergeAutoOrganizeSettings(rows: Setting[]): AutoOrganizeConfig {
  const idx = settingIndex(rows)
  return {
    enabled: idx[AUTO_ORGANIZE_KEYS.enabled] ?? AUTO_ORGANIZE_DEFAULTS.enabled,
    afterDownload: idx[AUTO_ORGANIZE_KEYS.afterDownload] ?? AUTO_ORGANIZE_DEFAULTS.afterDownload,
    scrapeAfter: idx[AUTO_ORGANIZE_KEYS.scrapeAfter] ?? AUTO_ORGANIZE_DEFAULTS.scrapeAfter,
    downloadSmartClassify: idx[AUTO_ORGANIZE_KEYS.downloadSmartClassify] ?? AUTO_ORGANIZE_DEFAULTS.downloadSmartClassify,
    smartClassify: idx[AUTO_ORGANIZE_KEYS.smartClassify] ?? AUTO_ORGANIZE_DEFAULTS.smartClassify,
    autoAddLibrary: idx[AUTO_ORGANIZE_KEYS.autoAddLibrary] ?? AUTO_ORGANIZE_DEFAULTS.autoAddLibrary,
    sourceDir: idx[AUTO_ORGANIZE_KEYS.sourceDir] ?? AUTO_ORGANIZE_DEFAULTS.sourceDir,
    targetDir: idx[AUTO_ORGANIZE_KEYS.targetDir] ?? AUTO_ORGANIZE_DEFAULTS.targetDir,
    transferMode: idx[AUTO_ORGANIZE_KEYS.transferMode] ?? AUTO_ORGANIZE_DEFAULTS.transferMode,
    intervalSeconds: idx[AUTO_ORGANIZE_KEYS.intervalSeconds] ?? AUTO_ORGANIZE_DEFAULTS.intervalSeconds,
    keepSeeding: idx[AUTO_ORGANIZE_KEYS.keepSeeding] ?? AUTO_ORGANIZE_DEFAULTS.keepSeeding,
    movieFormat: idx[AUTO_ORGANIZE_KEYS.movieFormat] ?? AUTO_ORGANIZE_DEFAULTS.movieFormat,
    tvFormat: idx[AUTO_ORGANIZE_KEYS.tvFormat] ?? AUTO_ORGANIZE_DEFAULTS.tvFormat,
    animeFormat: idx[AUTO_ORGANIZE_KEYS.animeFormat] ?? AUTO_ORGANIZE_DEFAULTS.animeFormat,
    scrapeAutoOnScan: idx[AUTO_ORGANIZE_KEYS.scrapeAutoOnScan] ?? AUTO_ORGANIZE_DEFAULTS.scrapeAutoOnScan,
    scrapeProviders: idx[AUTO_ORGANIZE_KEYS.scrapeProviders] ?? AUTO_ORGANIZE_DEFAULTS.scrapeProviders,
    scrapeLanguage: idx[AUTO_ORGANIZE_KEYS.scrapeLanguage] ?? AUTO_ORGANIZE_DEFAULTS.scrapeLanguage,
    scrapeDelayMinMs: idx[AUTO_ORGANIZE_KEYS.scrapeDelayMinMs] ?? AUTO_ORGANIZE_DEFAULTS.scrapeDelayMinMs,
    scrapeDelayMaxMs: idx[AUTO_ORGANIZE_KEYS.scrapeDelayMaxMs] ?? AUTO_ORGANIZE_DEFAULTS.scrapeDelayMaxMs,
  }
}

export function settingOn(value: string): boolean {
  return ['1', 'true', 'yes', 'on', 'enabled', '启用', '开启'].includes(value.trim().toLowerCase())
}

function settingIndex(rows: Setting[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const row of rows) out[row.key] = row.value
  return out
}
