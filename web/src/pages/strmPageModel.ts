import type { Library } from '../types'

export type CloudPlaybackMode = 'strm' | 'redirect_proxy'

export function currentOrigin() {
  if (typeof window === 'undefined') return ''
  return window.location.origin.replace(/\/+$/, '')
}

export function isLocalPlaybackBase(raw: string) {
  try {
    const u = new URL(raw)
    const host = u.hostname.toLowerCase()
    return host === 'localhost' || host === '127.0.0.1' || host === '::1'
  } catch {
    return false
  }
}

export function preferredSTRMBaseURL(saved: string) {
  const current = currentOrigin()
  const trimmed = saved.trim().replace(/\/+$/, '')
  if (!trimmed) return current
  if (current && isLocalPlaybackBase(trimmed) && trimmed !== current) return current
  return trimmed
}

export function playbackStatusText(
  strmPlaybackEnabled: boolean,
  redirectProxyEnabled: boolean,
  cloudPlaybackMode: CloudPlaybackMode,
) {
  if (strmPlaybackEnabled && redirectProxyEnabled) {
    return `两者开启 · 优先${cloudPlaybackMode === 'strm' ? 'STRMURL' : '302/反代'}`
  }
  if (strmPlaybackEnabled) return '仅 STRMURL'
  if (redirectProxyEnabled) return '仅 302/反代'
  return '云盘第三方播放关闭'
}

export function suggestedSTRMOutputDir(root: string, library: Library) {
  const base = trimPath(root)
  const subdir = strmLibraryOutputSubdir(library)
  if (!subdir) return base
  if (pathEndsWith(base, subdir) || pathEndsWith(base, pathBase(subdir))) return base
  const tail = missingCategoryTail(base, subdir)
  if (tail) return joinPath(base, tail)
  return joinPath(base || 'data/strm', subdir)
}

export function inferSTRMOutputRoot(saved: string, libraries: Library[]) {
  const normalized = trimPath(saved)
  if (!normalized) return ''
  const slashSaved = toSlash(normalized).toLowerCase()
  for (const library of libraries) {
    const subdir = strmLibraryOutputSubdir(library)
    if (!subdir) continue
    const suffix = `/${toSlash(subdir).toLowerCase()}`
    if (slashSaved.endsWith(suffix)) {
      return trimPath(normalized.slice(0, normalized.length - suffix.length))
    }
    if (slashSaved === toSlash(subdir).toLowerCase()) return ''
  }
  const inferredBySTRMRoot = stripLibraryCategoryAfterSTRMRoot(normalized)
  if (inferredBySTRMRoot) return inferredBySTRMRoot
  return normalized
}

export function strmLibraryOutputSubdir(library: Library) {
  const parts = categoryPartsFromPath(libraryPathParts(library.path))
    ?? categoryPartsFromPath(nameParts(library.name))
    ?? mediaTypeRootParts(library.type)
    ?? []
  return joinPath('', ...parts.map(sanitizePathPart).filter(Boolean))
}

function categoryPartsFromPath(parts: string[]) {
  for (let index = 0; index < parts.length; index += 1) {
    const part = parts[index]
    const root = canonicalRoot(part)
    if (root) return [root, ...parts.slice(index + 1)]
    const categoryRoot = categoryRootFor(part)
    if (categoryRoot) return [categoryRoot, canonicalCategory(part)]
  }
  return null
}

function libraryPathParts(raw: string) {
  const cloudParts = cloudDisplayPathParts(raw)
  if (cloudParts.length > 0) return cloudParts
  const cleaned = toSlash(raw).replace(/^[A-Za-z]:/, '').replace(/^\/+|\/+$/g, '')
  return splitPath(cleaned)
}

function cloudDisplayPathParts(raw: string) {
  if (!/^cloud:\/\//i.test(raw.trim())) return []
  try {
    const url = new URL(raw)
    return splitPath(decodeURIComponent(url.pathname.replace(/^\/+|\/+$/g, '')))
  } catch {
    const rest = raw.trim().replace(/^cloud:\/\/[^/]+\/?/i, '').split('?')[0] ?? ''
    return splitPath(decodeURIComponent(rest.replace(/^\/+|\/+$/g, '')))
  }
}

function nameParts(name: string) {
  return splitPath(name.replace(/[·>｜|\\]/g, '/'))
}

function mediaTypeRootParts(type: string) {
  const normalized = type.trim().toLowerCase()
  if (normalized === 'movie') return ['电影']
  if (normalized === 'tv' || normalized === 'variety') return ['电视剧']
  if (normalized === 'anime') return ['动漫']
  if (normalized === 'adult') return ['成人']
  return null
}

function canonicalRoot(part: string) {
  const key = part.trim().toLowerCase()
  if (['电影', 'movie', 'movies', 'film', 'films'].includes(key)) return '电影'
  if (['电视剧', '剧集', 'tv', 'tvs', 'series', 'show', 'shows'].includes(key)) return '电视剧'
  if (['动漫', '动画', 'anime', 'bangumi'].includes(key)) return '动漫'
  if (['成人', 'adult', 'adults', 'jav', 'nsfw', '9kg'].includes(key)) return '成人'
  return ''
}

function categoryRootFor(part: string) {
  const key = part.trim().toLowerCase()
  if (['演唱会', '音乐会', '纪录片', '纪录', '动画电影', '动漫电影', '华语电影', '国产电影', '外语电影', '外国电影', '欧美电影', '日韩电影', '日本电影', '韩国电影'].includes(key)) return '电影'
  if (['国产剧', '欧美剧', '日韩剧', '日剧', '韩剧', '综艺', '真人秀', '儿童', '少儿', '未分类'].includes(key)) return '电视剧'
  if (['国漫', '国产动漫', '日番', '番剧', '日漫', '日本动漫', '日本动画', '韩漫', '韩国动漫', '韩国动画', '美漫', '欧美动漫', '欧美动画', '西方动画', '其他', '其他动漫', '其它动漫'].includes(key)) return '动漫'
  if (key === '番号') return '成人'
  return ''
}

function canonicalCategory(part: string) {
  const key = part.trim().toLowerCase()
  if (key === '音乐会') return '演唱会'
  if (key === '纪录') return '纪录片'
  if (key === '动漫电影') return '动画电影'
  if (key === '国产电影') return '华语电影'
  if (key === '外语电影' || key === '外国电影') return '欧美电影'
  if (key === '日本电影' || key === '韩国电影') return '日韩电影'
  if (key === '日剧' || key === '韩剧') return '日韩剧'
  if (key === '真人秀') return '综艺'
  if (key === '少儿') return '儿童'
  if (key === '国产动漫') return '国漫'
  if (['番剧', '日漫', '日本动漫', '日本动画'].includes(key)) return '日番'
  if (key === '韩国动漫' || key === '韩国动画') return '韩漫'
  if (['欧美动漫', '欧美动画', '西方动画'].includes(key)) return '美漫'
  if (key === '其他动漫' || key === '其它动漫') return '其他'
  if (key === '番号') return '成人'
  return part.trim()
}

function splitPath(raw: string) {
  return raw.split('/').map((part) => part.trim()).filter((part) => part && part !== '.')
}

function stripLibraryCategoryAfterSTRMRoot(raw: string) {
  const normalized = toSlash(raw)
  const parts = normalized.split('/')
  let strmIndex = -1
  for (let index = 0; index < parts.length; index += 1) {
    if (parts[index].trim().toLowerCase() === 'strm') strmIndex = index
  }
  if (strmIndex < 0 || strmIndex >= parts.length - 1) return ''
  const tail = parts.slice(strmIndex + 1).filter((part) => part.trim())
  const categoryParts = categoryPartsFromPath(tail)
  if (!categoryParts || categoryParts.length < 2) return ''
  return trimPath(parts.slice(0, strmIndex + 1).join('/'))
}

function joinPath(root: string, ...parts: string[]) {
  return [trimPath(root), ...parts.map(trimPath)].filter(Boolean).join('/')
}

function missingCategoryTail(base: string, subdir: string) {
  const parts = splitPath(subdir)
  if (parts.length < 2 || !pathEndsWith(base, parts[0])) return ''
  return joinPath('', ...parts.slice(1))
}

function pathEndsWith(pathValue: string, suffix: string) {
  const path = trimPath(pathValue).toLowerCase()
  const tail = trimPath(suffix).toLowerCase()
  return !!path && !!tail && (path === tail || path.endsWith(`/${tail}`))
}

function pathBase(pathValue: string) {
  const parts = splitPath(pathValue)
  return parts[parts.length - 1] || ''
}

function trimPath(raw: string) {
  return toSlash(raw).replace(/\/+$/g, '').trim()
}

function toSlash(raw: string) {
  return raw.trim().replace(/\\/g, '/')
}

function sanitizePathPart(part: string) {
  return part.replace(/[/:*?"<>|\\]/g, ' ').trim()
}
