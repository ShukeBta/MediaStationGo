const ARTWORK_CACHE_PREFIX = 'mediastationgo-artwork-'
const ARTWORK_CACHE = `${ARTWORK_CACHE_PREFIX}v2`
const MIN_CACHEABLE_ARTWORK_BYTES = 128
const STRIP_QUERY_KEYS = ['token', 'profile_id', 'profile_pin_token']

function isArtworkRequest(url) {
  if (url.origin !== self.location.origin) return false
  if (url.pathname === '/api/img') return true
  return url.pathname.startsWith('/api/cloud/play/')
}

function normalizedArtworkRequest(request) {
  const url = new URL(request.url)
  for (const key of STRIP_QUERY_KEYS) {
    url.searchParams.delete(key)
  }
  return new Request(url.toString(), {
    method: 'GET',
    headers: {
      Accept: request.headers.get('Accept') || 'image/avif,image/webp,image/apng,image/*,*/*;q=0.8',
    },
    credentials: 'same-origin',
    mode: 'same-origin',
    redirect: 'follow',
  })
}

self.addEventListener('fetch', (event) => {
  const request = event.request
  if (request.method !== 'GET') return

  const url = new URL(request.url)
  if (!isArtworkRequest(url)) return

  event.respondWith(cacheArtwork(request))
})

self.addEventListener('install', (event) => {
  event.waitUntil(self.skipWaiting())
})

self.addEventListener('activate', (event) => {
  event.waitUntil(deleteOldArtworkCaches().then(() => self.clients.claim()))
})

async function cacheArtwork(request) {
  const cache = await caches.open(ARTWORK_CACHE)
  const cacheKey = normalizedArtworkRequest(request)
  const cached = await cache.match(cacheKey)
  if (cached) return cached

  const response = await fetch(request)
  const cacheResponse = await cloneCacheableArtworkResponse(response)
  if (cacheResponse) {
    await cache.put(cacheKey, cacheResponse)
    await deleteOldArtworkVariants(cache, cacheKey)
  }
  return response
}

async function cloneCacheableArtworkResponse(response) {
  if (!response.ok) return null
  const contentType = response.headers.get('Content-Type') || ''
  if (!contentType.toLowerCase().startsWith('image/')) return null
  const cacheControl = response.headers.get('Cache-Control') || ''
  if (/\bno-store\b/i.test(cacheControl)) return null

  const contentLength = Number(response.headers.get('Content-Length') || '0')
  if (Number.isFinite(contentLength) && contentLength > 0 && contentLength <= MIN_CACHEABLE_ARTWORK_BYTES) {
    return null
  }

  const buffer = await response.clone().arrayBuffer()
  if (buffer.byteLength <= MIN_CACHEABLE_ARTWORK_BYTES) return null
  return new Response(buffer, {
    status: response.status,
    statusText: response.statusText,
    headers: new Headers(response.headers),
  })
}

async function deleteOldArtworkCaches() {
  const names = await caches.keys()
  await Promise.all(names.map((name) => {
    if (!name.startsWith(ARTWORK_CACHE_PREFIX) || name === ARTWORK_CACHE) return undefined
    return caches.delete(name)
  }))
}

async function deleteOldArtworkVariants(cache, currentRequest) {
  const currentURL = new URL(currentRequest.url)
  const currentIdentity = artworkIdentity(currentURL)
  if (!currentIdentity) return

  const keys = await cache.keys()
  await Promise.all(keys.map(async (key) => {
    if (key.url === currentRequest.url) return
    const keyURL = new URL(key.url)
    if (artworkIdentity(keyURL) !== currentIdentity) return
    await cache.delete(key)
  }))
}

function artworkIdentity(url) {
  if (url.pathname === '/api/img') {
    return `${url.origin}${url.pathname}?url=${url.searchParams.get('url') || ''}`
  }
  if (url.pathname.startsWith('/api/cloud/play/')) {
    return `${url.origin}${url.pathname}?ref=${url.searchParams.get('ref') || ''}`
  }
  return ''
}
