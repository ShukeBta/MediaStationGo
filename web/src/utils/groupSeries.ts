import type { Media } from '../types'

/**
 * 把若干 Episode/Movie 行折叠成"剧集卡片"。
 *
 * 后端的 /api/media 等接口默认返回 episode 级行——同一部剧的每一集都
 * 是一行。在「最近添加」「海报墙」「收藏墙」这种以剧集为单位展示的页面
 * 我们要把它们合成一张代表卡片，避免同一海报刷屏。
 *
 * 折叠键优先级（命中第一个就分组）：
 *
 * 剧集（有季集号 / 路径像剧集）:
 *   1. 库标识 + 路径剧名       ← 最稳定:同一部剧的各集都在同一剧目录下
 *   2. tmdb_id / bangumi_id / douban / thetvdb （无路径剧名时兜底）
 *   3. series_id              （后端预聚合,目前仅 Emby 虚拟分组用,DB 一般为空）
 *   4. 库标识 + title         （最终兜底）
 * 电影:
 *   1. tmdb_id / bangumi_id
 *   2. 库标识 + 路径剧名
 *   3. 库标识 + title
 *
 * 为什么剧集要把「路径剧名」放在 tmdb_id 之前:
 * 本地/网盘按 MoviePilot 整理的剧集每集旁带 episode NFO, 其 <uniqueid type="tmdb">
 * 是【单集 episode id】(每集都不同)。若按 tmdb_id 分组, 同一部剧 N 集会被拆成 N
 * 张卡(实测「遮天」90 集 = 89 个不同 tmdb_id)。而整剧目录名对全剧一致, 是最可靠的
 * 分组依据, 且对「部分集已刮削、部分未刮削」也能稳定合并。
 *
 * key 必须是「单条 media 的纯函数」且与子集无关。最终暴露给 URL/API 的
 * key 是短 hash，避免把库标识和标题这类内部分类依据塞进地址栏。
 *
 * 同一组内取最早 created_at 的那条作为代表卡片，并带 count 表示集数。
 */
export type SeriesCard = { key: string; rep: Media; linkMedia: Media; count: number }

export function getSeriesKey(media: Media): string {
  return compactSeriesKey(getSeriesRawKey(media))
}

function getSeriesRawKey(media: Media): string {
  const fromPath = seriesTitleFromPath(media.path)
  if (isEpisodeLike(media) || pathLooksEpisodic(media)) {
    // 路径剧名优先:对全剧一致, 不受单集 tmdb 污染影响。
    if (fromPath) return seriesFingerprint('library-path', targetLibraryID(media), fromPath)
    const pathID = seriesExternalIDFromPath(media.path)
    if (pathID) return seriesFingerprint('library-path-id', targetLibraryID(media), pathID)
    // 无路径剧名(扁平目录)时才退而用外部 id;此时各集若共享整剧 id 仍能合并。
    if (media.tmdb_id && media.tmdb_id > 0) return `tmdb:${media.tmdb_id}`
    if (media.bangumi_id && media.bangumi_id > 0) return `bgm:${media.bangumi_id}`
    if (media.douban_id) return `douban:${media.douban_id}`
    if (media.thetvdb_id) return `thetvdb:${media.thetvdb_id}`
    if (media.series_id) return `series:${media.series_id}`
    return seriesFingerprint('library-title', targetLibraryID(media), normalizeTitle(seriesTitle(media)))
  }
  if (media.series_id) return `series:${media.series_id}`
  if (media.tmdb_id && media.tmdb_id > 0) return `tmdb:${media.tmdb_id}`
  if (media.bangumi_id && media.bangumi_id > 0) return `bgm:${media.bangumi_id}`
  if (fromPath) return seriesFingerprint('library-path', media.library_id, fromPath)
  return seriesFingerprint('library-title', media.library_id, normalizeTitle(media.title))
}

function seriesFingerprint(...parts: string[]): string {
  return parts.join('\x1f')
}

function compactSeriesKey(raw: string): string {
  const normalized = raw.trim()
  if (!normalized) return ''
  let hash = 0x811c9dc5
  for (const byte of new TextEncoder().encode(normalized)) {
    hash ^= byte
    hash = Math.imul(hash, 0x01000193) >>> 0
  }
  return `series:${hash.toString(16).padStart(8, '0')}`
}

export function isEpisodeLike(media: Media): boolean {
  if (!media) return false
  return (media.season_num ?? 0) > 0 || (media.episode_num ?? 0) > 0
}

// 剧集类目录名(电视剧/动漫及其二级分类)。媒体路径落在这些目录下时, 即便
// 季集号未识别出来, 也应按剧集对待, 跳转到 /library 分类视图而非 /media 单页。
const EPISODIC_PATH_RE =
  /[\\/](?:电视剧|剧集|连续剧|短剧|国产剧|国剧|大陆剧|华语剧|国产电视剧|大陆电视剧|华语电视剧|欧美剧|欧美电视剧|美剧|英剧|日韩剧|日韩电视剧|日剧|韩剧|港剧|台剧|港台剧|泰剧|综艺|纪录片|儿童|动漫|番剧|国漫|日番|韩漫|美漫|欧美动漫|欧美动画|其他动漫|tv|series|shows?|season[\s._-]*\d|s\d{1,2}(?:[\s._-]|[\\/])|special[\s._-]*episodes?|specials?|sp|ovas?|oads?|extras?|bonus(?:es)?|omake|特别篇|特別篇|番外篇?|特典|外传|外傳|总集篇|總集篇)[\\/]/i

const SEASON_FOLDER_RE =
  /^(?:s\d{1,2}|season[\s._-]*\d{1,2}|第\s*[0-9一二三四五六七八九十百零两]+\s*季|special[\s._-]*episodes?|specials?|sp|ovas?|oads?|extras?|bonus(?:es)?|omake|特别篇|特別篇|番外篇?|特典|外传|外傳|总集篇|總集篇)$/i

function pathLooksEpisodic(media: Media): boolean {
  const path = (media.path || media.display_library_path || media.library_path || '')
  return EPISODIC_PATH_RE.test(path)
}

export function isSeriesCard(card: SeriesCard): boolean {
  return (
    card.count > 1 ||
    isEpisodeLike(card.rep) ||
    isEpisodeLike(card.linkMedia) ||
    pathLooksEpisodic(card.rep) ||
    pathLooksEpisodic(card.linkMedia)
  )
}

export function seriesTitle(media: Media): string {
  const title = media.title?.trim()
  const fromPath = seriesTitleFromPath(media.path)
  return (title && !unsafeEpisodeTitle(title) ? title : '') || fromPath || media.original_name || title || '未命名节目'
}

function normalizeTitle(value?: string): string {
  return (value ?? '')
    .toLowerCase()
    .replace(/\s*\((?:19|20)\d{2}\)\s*/g, ' ')
    .replace(/\s*\[(?:tmdb|tmdbid)[=-]\d+\]\s*/g, ' ')
    .replace(/\s*\{(?:tmdb|tmdbid|douban|bangumi|bgm|thetvdb|tvdb)[\s:=#-]*[a-z0-9_-]+\}\s*/g, ' ')
    .replace(/[\s._-]+/g, ' ')
    .trim()
}

const SERIES_SPECIAL_CODE_RE =
  /\s*[[(（【]?\s*(?:s0+\s*e?\s*\d+|season\s*0+(?:\s*episode)?\s*\d*|special(?:\s*episode)?s?\s*\d*|sp\s*\d*|ovas?\s*\d*|oads?\s*\d*|extras?\s*\d*|bonus(?:es)?\s*\d*|omake\s*\d*)\s*[\])）】]?$/i

const SERIES_SPECIAL_CJK_RE =
  /\s*[[(（【]?\s*(?:特别篇|特別篇|番外篇?|特典|外传|外傳|总集篇|總集篇)(?:\s*第?\s*[0-9一二三四五六七八九十百零两]+(?:[集话話期])?)?\s*[\])）】]?$/i

function normalizePathSeriesTitle(value?: string): string {
  const title = normalizeTitle(value)
  const stripped = stripSeriesSpecialSuffix(title)
  const normalized = cleanSeriesReleaseNoise(stripped || title)
  return unsafeEpisodeTitle(normalized) ? '' : normalized
}

function stripSeriesSpecialSuffix(title: string): string {
  for (const pattern of [SERIES_SPECIAL_CODE_RE, SERIES_SPECIAL_CJK_RE]) {
    const stripped = title.replace(pattern, '').trim()
    if (stripped && stripped !== title) return stripped
  }
  return title
}

const SERIES_RELEASE_NOISE = new Set([
  '1080p', '2160p', '4k', '720p', '480p', 'uhd', 'ds4k', 'fhd',
  'bd', 'bdrip', 'brrip', 'dvd', 'dvdrip', 'hdtv', 'pdtv', 'webdl',
  'hdrip', 'bluray', 'blu ray', 'webrip', 'web', 'web dl',
  'x264', 'x265', 'h264', 'h265', 'hevc', 'avc', '10bit', '8bit', 'hi10p', 'hi10',
  'hdr', 'hdr10', 'sdr', 'dts', 'ddp', 'ddp5', 'dd5', 'dd2', 'eac3', 'truehd',
  'dovi', 'atmos', 'aac', 'aac2', 'aac5', 'ac3', 'flac', 'fps', 'hlg', 'dv',
  'remux', 'extended', 'uncut', 'remastered', 'repack', 'proper', 'internal',
  'limited', 'imax',
  'netflix', 'nf', 'amzn', 'hulu', 'disney', 'max', 'hbo', 'linetv', 'ourtv',
  'iqiyi', 'youku', 'bilibili', 'qiyi', 'krj', 'atvp', 'appletv', 'tx', 'txweb',
  'crunchyroll', 'funimation', 'anidb', 'horriblesubs', 'subsplease',
  'erai', 'raws', 'judas', 'asw', 'smcat', 'leopard', 'ohys', 'colortv',
  'mweb', 'ubweb', 'hhweb', 'adweb', 'chdweb', 'kurosawa', 'qhstudio',
  'zm', 'zw', 'ch', 'chs', 'cht', 'cn', 'tc', 'sc',
  '中字', '繁字', '简中', '繁中', '国语', '粤语', '日语',
  'season', '264', '265',
])

const SERIES_RELEASE_BOUNDARY = new Set([
  '1080p', '2160p', '4k', '720p', '480p', 'uhd', 'fhd',
  'bd', 'bdrip', 'brrip', 'dvd', 'dvdrip', 'hdtv', 'pdtv',
  'webdl', 'hdrip', 'bluray', 'webrip', 'web', 'remux',
  'x264', 'x265', 'h264', 'h265', 'hevc', 'avc',
])

const SERIES_DANGLING_SE_RE = /^s\d{1,2}e$/i
const SERIES_MULTI_WORD_NOISE = [
  /\bweb\s+dl\b/g,
  /\bblu\s+ray\b/g,
  /\bdirectors\s+cut\b/g,
  /\berai\s+raws\b/g,
  /\bohys\s+raws\b/g,
  /\bleopard\s+raws\b/g,
]

function cleanSeriesReleaseNoise(title: string): string {
  for (const pattern of SERIES_MULTI_WORD_NOISE) {
    title = title.replace(pattern, ' ')
  }
  const fields = title.split(/\s+/).filter(Boolean)
  const out: string[] = []
  let seenReleaseBoundary = false
  for (const field of fields) {
    if (SERIES_DANGLING_SE_RE.test(field)) continue
    if (SERIES_RELEASE_NOISE.has(field)) {
      if (SERIES_RELEASE_BOUNDARY.has(field)) seenReleaseBoundary = true
      continue
    }
    if (seenReleaseBoundary && /^[a-z0-9]+$/.test(field)) continue
    if (field.length <= 1 && /^[a-z0-9]$/.test(field)) continue
    out.push(field)
  }
  return out.join(' ').trim()
}

const EPISODE_ONLY_TITLE_RE =
  /^(?:e(?:p(?:isode)?)?\s*\d{1,3}|episode\s*\d{1,3}|第\s*[0-9一二三四五六七八九十百零两]+\s*[集期话話](?:\s*[上下])?|第\s*[集期话話])$/i

const EPISODE_TITLE_RE =
  /^第\s*[0-9一二三四五六七八九十百零两]+\s*[集期话話](?:\s*[上下])?\s*[:：].+/

const EPISODIC_RELEASE_TITLE_RE =
  /(?:^|\s)(?:s\d{1,2}\s*e\d{1,3}|season\s*\d{1,2}\s*(?:episode|ep)\s*\d{1,3}|\d{1,2}x\d{1,3}|e(?:p(?:isode)?)?\s*\d{1,3})(?:\s|$)/i

function unsafeEpisodeTitle(title: string): boolean {
  const value = title.trim()
  return EPISODE_ONLY_TITLE_RE.test(value) || EPISODE_TITLE_RE.test(value) || EPISODIC_RELEASE_TITLE_RE.test(normalizeTitle(value))
}

export function seriesTitleFromPath(path?: string): string {
  const part = seriesDirectoryNameFromPath(path)
  return part ? normalizePathSeriesTitle(part) : ''
}

function seriesDirectoryNameFromPath(path?: string): string {
  if (!path) return ''
  const parts = path.split(/[\\/]+/).filter(Boolean)
  if (parts.length < 2) return ''
  let dirIndex = parts.length - 2
  const lastPart = parts[parts.length - 1]
  if (!seriesPathPartLooksLikeFile(lastPart) && !SEASON_FOLDER_RE.test(lastPart)) {
    dirIndex = parts.length - 1
  }
  while (dirIndex >= 0 && SEASON_FOLDER_RE.test(parts[dirIndex])) {
    dirIndex -= 1
  }
  if (dirIndex < 0) return ''
  return parts[dirIndex]
}

function seriesExternalIDFromPath(path?: string): string {
  const part = seriesDirectoryNameFromPath(path)
  if (!part) return ''
  for (const [source, pattern] of [
    ['tmdb', /(?:^|[^a-z0-9])(?:tmdbid|tmdb)[\s_:=#-]*(\d{2,})/i],
    ['bgm', /(?:^|[^a-z0-9])(?:bangumi|bgm)[\s_:=#-]*(\d{2,})/i],
    ['douban', /(?:^|[^a-z0-9])(?:douban|db)[\s_:=#-]*(\d{2,})/i],
    ['thetvdb', /(?:^|[^a-z0-9])(?:thetvdb|tvdb)[\s_:=#-]*(\d{2,})/i],
  ] as const) {
    const match = pattern.exec(part)
    if (match?.[1]) return `${source}:${match[1]}`
  }
  return ''
}

function seriesPathPartLooksLikeFile(part: string): boolean {
  return /\.(?:mkv|mp4|m4v|avi|mov|webm|flv|wmv|ts|m2ts|mts|vob|rmvb|rm|3gp|mpg|mpeg|iso|strm|nfo|srt|ass|ssa|vtt|sub|idx|jpe?g|png|webp)$/i.test(part)
}

export function groupSeries(items: Media[] = []): SeriesCard[] {
  const safeItems = Array.isArray(items) ? items : []
  const groups = new Map<string, SeriesCard>()
  for (const m of safeItems) {
    if (!m) continue
    const key = getSeriesKey(m)

    const g = groups.get(key)
    if (!g) {
      groups.set(key, { key, rep: m, linkMedia: m, count: 1 })
    } else {
      g.count += 1
      if (betterSeriesLinkMedia(m, g.linkMedia)) {
        g.linkMedia = m
      }
      const currentArtwork = artworkScore(m)
      const representativeArtwork = artworkScore(g.rep)
      if (currentArtwork > representativeArtwork) {
        g.rep = m
      } else if (currentArtwork === representativeArtwork) {
        const cur = (m.season_num ?? 0) * 10000 + (m.episode_num ?? 0)
        const rep = (g.rep.season_num ?? 0) * 10000 + (g.rep.episode_num ?? 0)
        if (cur > 0 && (rep === 0 || cur < rep)) g.rep = m
      }
    }
  }
  return Array.from(groups.values())
}

export function seriesCardLink(card: SeriesCard): string {
  if (isSeriesCard(card)) {
    return `/library/${targetLibraryID(card.linkMedia)}?series=${encodeURIComponent(card.key)}`
  }
  return `/media/${card.rep.id}`
}

function betterSeriesLinkMedia(candidate: Media, current: Media): boolean {
  const candidateScore = librarySpecificityScore(candidate)
  const currentScore = librarySpecificityScore(current)
  if (candidateScore !== currentScore) return candidateScore > currentScore
  return artworkScore(candidate) > artworkScore(current)
}

function librarySpecificityScore(media: Media): number {
  const rawPath = (media.display_library_path || media.library_path || '').trim()
  if (!rawPath) return 0
  const normalized = rawPath.replace(/\\/g, '/').replace(/\/+$/, '')
  const lower = normalized.toLowerCase()
  if (lower.startsWith('cloud://')) {
    const rest = normalized.slice('cloud://'.length)
    const slash = rest.indexOf('/')
    if (slash < 0 || slash === rest.length - 1) return 0
    return 100 + rest.slice(slash + 1).split('/').filter(Boolean).length
  }
  return 200 + normalized.split('/').filter(Boolean).length
}

function targetLibraryID(media: Media): string {
  return media.display_library_id || media.library_id
}

export function artworkScore(media: Media): number {
  const poster = (media.poster_url ?? '').toLowerCase()
  const backdrop = (media.backdrop_url ?? '').toLowerCase()
  if (poster) {
    if (/(poster|folder|cover|movie|show|pl)(?:[._-]|\.[a-z0-9]+$|$)/.test(poster)) return 40
    if (/(actor|actress|cast|avatar|sample|screenshot|screen|still|scene|fanart|backdrop|background|landscape|banner|logo|disc)/.test(poster)) return 10
    if (/(thumb)/.test(poster)) return 20
    return 30
  }
  return backdrop ? 5 : 0
}
