import axios, { AxiosError, type InternalAxiosRequestConfig } from 'axios'

import { useAuthStore } from '../stores/auth'
import { getActivePlayProfileId, getActivePlayProfilePinToken } from '../stores/playProfile'

// Single axios instance used by every API helper. Adds the JWT to outgoing
// requests and routes 401s back to the login page.
export const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
})

export const LONG_REQUEST_TIMEOUT = 120_000
export const BATCH_REQUEST_TIMEOUT = 300_000

let refreshPromise: Promise<string> | null = null

function isRefreshRequest(config?: InternalAxiosRequestConfig | null): boolean {
  return Boolean(config?.url?.includes('/auth/refresh'))
}

function tokenExpiresSoon(token: string, leewaySeconds = 30): boolean {
  try {
    const encoded = token.split('.')[1]
    if (!encoded) return false
    const normalized = encoded.replace(/-/g, '+').replace(/_/g, '/')
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
    const payload = JSON.parse(atob(padded)) as { exp?: number }
    return typeof payload.exp === 'number' && payload.exp * 1000 <= Date.now() + leewaySeconds * 1000
  } catch {
    return false
  }
}

function redirectToLogin() {
  useAuthStore.getState().logout()
  if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
    window.location.href = '/login'
  }
}

async function refreshAccessToken(): Promise<string> {
  if (!refreshPromise) {
    refreshPromise = (async () => {
      const refreshed = await useAuthStore.getState().tokenRefresh()
      const token = useAuthStore.getState().token
      if (!refreshed || !token) throw new Error('access token refresh failed')
      return token
    })()
      .catch((error) => {
        redirectToLogin()
        throw error
      })
      .finally(() => {
        refreshPromise = null
      })
  }
  return refreshPromise
}

export async function ensureAccessToken(): Promise<string | null> {
  const token = useAuthStore.getState().token
  if (!token || !tokenExpiresSoon(token)) return token
  return refreshAccessToken()
}

// Add auth token to requests
api.interceptors.request.use(async (config) => {
  const token = isRefreshRequest(config)
    ? useAuthStore.getState().token
    : await ensureAccessToken()
  if (token) {
    config.headers = config.headers ?? {}
    config.headers.Authorization = `Bearer ${token}`
  }
  const activeProfileId = getActivePlayProfileId()
  if (activeProfileId) {
    config.headers = config.headers ?? {}
    config.headers['X-Play-Profile-ID'] = activeProfileId
    const pinToken = getActivePlayProfilePinToken()
    if (pinToken) {
      config.headers['X-Play-Profile-PIN-Token'] = pinToken
    }
  }
  return config
})

// Handle 401 errors with token refresh
api.interceptors.response.use(
  (resp) => resp,
  async (err: AxiosError) => {
    const originalRequest = err.config as InternalAxiosRequestConfig & { _retry?: boolean }

    // If 401 and not already retried
    if (
      err.response?.status === 401 &&
      originalRequest &&
      !originalRequest._retry &&
      !isRefreshRequest(originalRequest)
    ) {
      originalRequest._retry = true
      try {
        const newToken = await refreshAccessToken()
        if (originalRequest.headers) {
          originalRequest.headers.Authorization = `Bearer ${newToken}`
        }
        return api(originalRequest)
      } catch (refreshError) {
        return Promise.reject(refreshError)
      }
    }

    // For other errors, just reject
    return Promise.reject(err)
  },
)

const tokenQuery = () => {
  const t = useAuthStore.getState().token ?? ''
  return `token=${encodeURIComponent(t)}`
}

const profileQuery = () => {
  const id = getActivePlayProfileId()
  if (!id) return ''
  const pinToken = getActivePlayProfilePinToken()
  return `&profile_id=${encodeURIComponent(id)}${
    pinToken ? `&profile_pin_token=${encodeURIComponent(pinToken)}` : ''
  }`
}

// streamURL returns a direct-play URL for <video src>. The JWT is added as
// a query parameter because <video> elements cannot send Authorization
// headers.
export function streamURL(mediaId: string): string {
  return `/api/stream/${encodeURIComponent(mediaId)}?${tokenQuery()}${profileQuery()}`
}

// hlsURL returns the m3u8 playlist URL fed into hls.js.
export function hlsURL(mediaId: string): string {
  return `/api/hls/${encodeURIComponent(mediaId)}/index.m3u8?${tokenQuery()}${profileQuery()}`
}

// imageURL converts a remote poster URL into a same-origin proxy URL so it
// can never be blocked by CORS / GFW. Empty strings pass through unchanged.
export type ImageURLOptions =
  | boolean
  | {
      refreshCache?: boolean
      retryFailed?: boolean
      maxWidth?: number
      maxHeight?: number
      quality?: number
    }

type ImageURLLegacyParams = Record<string, string | number | boolean | undefined | null>

function paramsQuery(params?: ImageURLLegacyParams): string {
  if (!params) return ''
  return Object.entries(params)
    .filter(([, value]) => value !== undefined && value !== null && value !== false && value !== '')
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
    .join('&')
}

export function imageURL(remote?: string, version?: string | ImageURLLegacyParams, options: ImageURLOptions = false): string {
  if (!remote) return ''
  const legacyQuery = typeof version === 'object' ? paramsQuery(version) : ''
  const retryFailed = typeof options === 'boolean' ? options : Boolean(options.retryFailed)
  const refreshCache = typeof options === 'boolean' ? false : Boolean(options.refreshCache)
  // A remote image URL already identifies the artwork. Media updated_at also
  // changes for playback/probe/metadata updates, so attaching it to remote
  // artwork creates needless browser/CDN cache misses for unchanged images.
  // Local and cloud sidecar paths still need the explicit version because the
  // bytes can change while their path remains stable. An explicit refresh also
  // retains its revision so the freshly fetched remote bytes are visible now.
  const versionQuery = typeof version === 'string' && version && (!isRemoteProxyImage(remote) || refreshCache)
    ? `v=${encodeURIComponent(version)}`
    : ''
  const maxWidth = typeof options === 'boolean' ? 0 : positiveImageOption(options.maxWidth)
  const maxHeight = typeof options === 'boolean' ? 0 : positiveImageOption(options.maxHeight)
  const quality = typeof options === 'boolean' ? 0 : positiveImageOption(options.quality)
  const retryQuery = retryFailed ? 'retry=1' : ''
  const refreshQuery = refreshCache ? 'refresh=1' : ''
  const widthQuery = maxWidth ? `maxWidth=${maxWidth}` : ''
  const heightQuery = maxHeight ? `maxHeight=${maxHeight}` : ''
  const qualityQuery = quality ? `quality=${quality}` : ''
  const imageQuery = [legacyQuery, versionQuery, retryQuery, refreshQuery, widthQuery, heightQuery, qualityQuery]
    .filter(Boolean)
    .join('&')
  if (remote.startsWith('/api/img')) return withQuery(withoutAuthQuery(remote), imageQuery)
  if (remote.startsWith('/api/cloud/play/')) return withQuery(withoutAuthQuery(remote), imageQuery)
  if (remote.startsWith('/api/')) return withQuery(withQuery(remote, tokenQuery()), imageQuery)
  return withQuery(`/api/img?url=${encodeURIComponent(remote)}`, imageQuery)
}

function isRemoteProxyImage(remote: string): boolean {
  if (/^https?:\/\//i.test(remote)) return true
  if (!remote.startsWith('/api/img')) return false
  const queryIndex = remote.indexOf('?')
  if (queryIndex < 0) return false
  const raw = new URLSearchParams(remote.slice(queryIndex + 1)).get('url') ?? ''
  return /^https?:\/\//i.test(raw)
}

function positiveImageOption(value?: number): number {
  if (!Number.isFinite(value) || !value || value <= 0) return 0
  return Math.round(value)
}

function withQuery(url: string, query: string): string {
  if (!query) return url
  return `${url}${url.includes('?') ? '&' : '?'}${query}`
}

function withoutAuthQuery(url: string): string {
  const hashIndex = url.indexOf('#')
  const beforeHash = hashIndex >= 0 ? url.slice(0, hashIndex) : url
  const hash = hashIndex >= 0 ? url.slice(hashIndex) : ''
  const queryIndex = beforeHash.indexOf('?')
  if (queryIndex < 0) return url

  const path = beforeHash.slice(0, queryIndex)
  const params = new URLSearchParams(beforeHash.slice(queryIndex + 1))
  ;['token', 'api_key', 'apiKey', 'ApiKey'].forEach((key) => params.delete(key))
  const query = params.toString()
  return `${path}${query ? `?${query}` : ''}${hash}`
}

// getToken returns the current auth token
export function getToken(): string | null {
  return useAuthStore.getState().token
}

// getRefreshToken returns the current refresh token
export function getRefreshToken(): string | null {
  return useAuthStore.getState().refreshToken
}
