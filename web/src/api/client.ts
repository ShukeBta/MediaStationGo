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

// Flag to prevent multiple simultaneous refresh attempts
let isRefreshing = false
let refreshSubscribers: Array<{
  resolve: (token: string) => void
  reject: (error: unknown) => void
}> = []

// Subscribe to token refresh
function subscribeTokenRefresh(resolve: (token: string) => void, reject: (error: unknown) => void) {
  refreshSubscribers.push({ resolve, reject })
}

// Notify all subscribers about new token
function onTokenRefreshed(newToken: string) {
  refreshSubscribers.forEach((subscriber) => subscriber.resolve(newToken))
  refreshSubscribers = []
}

function onTokenRefreshFailed(error: unknown) {
  refreshSubscribers.forEach((subscriber) => subscriber.reject(error))
  refreshSubscribers = []
}

function isRefreshRequest(config?: InternalAxiosRequestConfig | null): boolean {
  return Boolean(config?.url?.includes('/auth/refresh'))
}

// Add auth token to requests
api.interceptors.request.use((config) => {
  const token = useAuthStore.getState().token
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
      if (isRefreshing) {
        // Wait for token refresh to complete
        return new Promise((resolve, reject) => {
          subscribeTokenRefresh((token: string) => {
            if (originalRequest.headers) {
              originalRequest.headers.Authorization = `Bearer ${token}`
            }
            resolve(api(originalRequest))
          }, reject)
        })
      }

      originalRequest._retry = true
      isRefreshing = true

      try {
        const refreshed = await useAuthStore.getState().tokenRefresh()
        if (refreshed) {
          const newToken = useAuthStore.getState().token
          if (newToken && originalRequest.headers) {
            originalRequest.headers.Authorization = `Bearer ${newToken}`
          }
          onTokenRefreshed(newToken || '')
          isRefreshing = false
          return api(originalRequest)
        }
      } catch (refreshError) {
        isRefreshing = false
        onTokenRefreshFailed(refreshError)
        useAuthStore.getState().logout()
        if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
          window.location.href = '/login'
        }
        return Promise.reject(refreshError)
      }

      // Refresh failed, logout
      isRefreshing = false
      onTokenRefreshFailed(err)
      useAuthStore.getState().logout()
      if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
        window.location.href = '/login'
      }
      return Promise.reject(err)
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
    }

export function imageURL(remote?: string, version?: string, options: ImageURLOptions = false): string {
  if (!remote) return ''
  const versionQuery = version ? `v=${encodeURIComponent(version)}` : ''
  const retryFailed = typeof options === 'boolean' ? options : Boolean(options.retryFailed)
  const refreshCache = typeof options === 'boolean' ? false : Boolean(options.refreshCache)
  const retryQuery = retryFailed ? 'retry=1' : ''
  const refreshQuery = refreshCache ? 'refresh=1' : ''
  const imageQuery = [versionQuery, retryQuery, refreshQuery].filter(Boolean).join('&')
  if (remote.startsWith('/api/img')) return withQuery(withoutAuthQuery(remote), imageQuery)
  if (remote.startsWith('/api/cloud/play/')) return withQuery(withoutAuthQuery(remote), imageQuery)
  if (remote.startsWith('/api/')) return withQuery(withQuery(remote, tokenQuery()), imageQuery)
  return withQuery(`/api/img?url=${encodeURIComponent(remote)}`, imageQuery)
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
