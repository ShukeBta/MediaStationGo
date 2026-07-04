export type OrganizePreviewItem = {
  source: string
  target?: string
  action: string
  reason?: string
}

type OrganizeScanSummary = {
  name: string
  added: number
  updated: number
  visited: number
  error?: string
}

type OrganizeScrapeSummary = {
  name: string
  matched: number
  processed?: number
  skipped?: boolean
  reason?: string
  error?: string
}

type OrganizeDirectoryResult = {
  organized?: number
  replaced?: number
  reclassified?: number
  skipped?: number
  errors?: string[]
  items?: OrganizePreviewItem[]
  scans?: OrganizeScanSummary[]
  scrapes?: OrganizeScrapeSummary[]
}

export function isCloudLibraryPath(value: string): boolean {
  return value.trim().toLowerCase().startsWith('cloud://')
}

export function summarizeOrganizeResults(results: OrganizeDirectoryResult[]) {
  const preview = results.flatMap((result) => result.items ?? [])
  const organized = results.reduce((sum, result) => sum + (result.organized ?? 0), 0)
  const replaced = results.reduce((sum, result) => sum + (result.replaced ?? 0), 0)
  const reclassified = results.reduce((sum, result) => sum + (result.reclassified ?? 0), 0)
  const skipped = results.reduce((sum, result) => sum + (result.skipped ?? 0), 0)
  const errors = results.flatMap((result) => result.errors ?? [])
  const scans = results.flatMap((result) => result.scans ?? [])
  const scrapes = results.flatMap((result) => result.scrapes ?? [])
  const total = organized + replaced + reclassified + skipped + errors.length

  return {
    preview,
    organized,
    replaced,
    reclassified,
    skipped,
    errors,
    scans,
    scrapes,
    total,
  }
}

export function formatScanSummary(scans: OrganizeScanSummary[]): string {
  if (scans.length === 0) return ' · 未扫描：没有匹配的媒体库'
  const ok = scans.filter((scan) => !scan.error)
  const added = ok.reduce((sum, scan) => sum + (scan.added ?? 0), 0)
  const updated = ok.reduce((sum, scan) => sum + (scan.updated ?? 0), 0)
  const visited = ok.reduce((sum, scan) => sum + (scan.visited ?? 0), 0)
  return ` · 扫描 ${ok.length}/${scans.length} 个库 · 新入库 ${added} · 更新 ${updated} · 访问 ${visited}`
}

export function formatScrapeSummary(scrapes: OrganizeScrapeSummary[]): string {
  if (scrapes.length === 0) return ''
  const ok = scrapes.filter((scrape) => !scrape.error && !scrape.skipped)
  const skipped = scrapes.filter((scrape) => scrape.skipped).length
  const matched = ok.reduce((sum, scrape) => sum + (scrape.matched ?? 0), 0)
  const processed = ok.reduce((sum, scrape) => sum + (scrape.processed ?? 0), 0)
  if (ok.length === 0 && skipped > 0) return ` · 刮削跳过 ${skipped} 个库`
  return ` · 刮削 ${ok.length}/${scrapes.length} 个库 · 处理 ${processed} · 匹配 ${matched}${skipped ? ` · 跳过 ${skipped}` : ''}`
}
