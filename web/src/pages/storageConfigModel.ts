import type { CloudEntry, StorageType } from '../api/storage_config'

export interface StorageFieldDef {
  key: string
  label: string
  secret?: boolean
  placeholder?: string
}

export const STORAGE_TABS: StorageType[] = ['openlist', 'alist', 'webdav', 'clouddrive2', 'cloud115']

const CLOUD_TYPES: StorageType[] = ['cloud115', 'clouddrive2', 'openlist']
const PATH_BASED_CLOUD = new Set<StorageType>(['openlist', 'clouddrive2'])

export const TYPE_LABEL: Record<string, string> = {
  alist: 'ALIST',
  openlist: 'OpenList',
  webdav: 'WEBDAV',
  cloud115: '115网盘',
  clouddrive2: 'CloudDrive2',
}

export const TRANSFER_SUPPORTED_TYPES = new Set<StorageType>(['alist', 'openlist', 'webdav', 'clouddrive2'])

export const FIELD_DEFS: Record<StorageType, StorageFieldDef[]> = {
  alist: [
    { key: 'server', label: 'Server URL', placeholder: 'https://alist.example.com' },
    { key: 'token', label: 'Token', secret: true },
  ],
  openlist: [
    { key: 'server', label: 'OpenList 服务地址(API / 浏览 / 挂载 / 转存)', placeholder: 'http://NAS-IP:5244' },
    { key: 'username', label: 'OpenList 用户名' },
    { key: 'password', label: 'OpenList 密码', secret: true },
    { key: 'token', label: 'OpenList Token(可选,优先于用户名密码)', secret: true },
    { key: 'url', label: 'WebDAV URL(可选兼容备用)', placeholder: '通常不用填；需要备用时填 http://NAS-IP:5244/dav/' },
    { key: 'timeout_seconds', label: '请求超时秒数', placeholder: '120' },
  ],
  webdav: [
    { key: 'url', label: 'URL', placeholder: 'https://example.com/dav/' },
    { key: 'username', label: '用户名' },
    { key: 'password', label: '密码', secret: true },
    { key: 'timeout_seconds', label: '请求超时秒数', placeholder: '120' },
  ],
  cloud115: [
    { key: 'cookie', label: 'Cookie(UID / CID / SEID,或扫码登录自动填充)', secret: true, placeholder: 'UID=...; CID=...; SEID=...' },
  ],
  clouddrive2: [
    { key: 'url', label: 'CloudDrive2 WebDAV URL', placeholder: 'http://host.docker.internal:19798/dav 或 http://NAS-IP:19798/dav' },
    { key: 'username', label: '用户名' },
    { key: 'password', label: '密码 / Token', secret: true },
    { key: 'token', label: 'Authorization Token(可选)', secret: true, placeholder: 'Bearer ... 或 Basic ...' },
    { key: 'timeout_seconds', label: '请求超时秒数', placeholder: '120' },
  ],
}

export function isCloud(type: StorageType) {
  return CLOUD_TYPES.includes(type)
}

export function normalizeCloudDisplayPath(value: string) {
  let text = value.trim()
  try {
    text = decodeURIComponent(text)
  } catch {
    // Keep the original value if it is not URI-encoded.
  }
  return text.replace(/\\/g, '/').replace(/^\/+|\/+$/g, '')
}

export function cloudLibraryProvider(path: string) {
  if (!path.toLowerCase().startsWith('cloud://')) return ''
  try {
    return new URL(path).host
  } catch {
    return ''
  }
}

export function cloudLibraryLabel(path: string) {
  try {
    const parsed = new URL(path)
    const display = normalizeCloudDisplayPath(parsed.pathname)
    const scanDir = normalizeCloudDisplayPath(parsed.searchParams.get('dir') ?? '')
    return display || scanDir || '根目录'
  } catch {
    return path
  }
}

export function cloudMountDisplayPath(type: StorageType, stack: { id: string; name: string }[], child?: CloudEntry) {
  if (PATH_BASED_CLOUD.has(type)) {
    return normalizeCloudDisplayPath(child?.id ?? stack[stack.length - 1]?.id ?? '')
  }
  const parts = stack.slice(1).map((item) => item.name).filter(Boolean)
  if (child?.name) parts.push(child.name)
  return parts.map(normalizeCloudDisplayPath).filter(Boolean).join('/')
}

export function fmtBytes(value: number): string {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let idx = 0
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024
    idx++
  }
  return `${size.toFixed(size >= 10 || idx === 0 ? 0 : 1)} ${units[idx]}`
}
