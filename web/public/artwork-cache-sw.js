const ARTWORK_CACHE_PREFIX = 'mediastationgo-artwork-'
self.addEventListener('install', (event) => {
  event.waitUntil(self.skipWaiting())
})

self.addEventListener('activate', (event) => {
  event.waitUntil(retireArtworkWorker())
})

async function retireArtworkWorker() {
  const names = await caches.keys()
  await Promise.all(names
    .filter((name) => name.startsWith(ARTWORK_CACHE_PREFIX))
    .map((name) => caches.delete(name)))
  await self.clients.claim()
  await self.registration.unregister()
}
