import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { Toaster } from 'react-hot-toast'

import App from './App'
import { GlobalEvents } from './components/GlobalEvents'
import { initializeThemeMode } from './components/useThemeMode'
import './index.css'

initializeThemeMode()

if ('serviceWorker' in navigator && import.meta.env.PROD) {
  void retireArtworkServiceWorker().catch((error) => {
    console.error('Failed to retire artwork service worker', error)
  })
}

const ARTWORK_SERVICE_WORKER_PATH = '/artwork-cache-sw.js'
const ARTWORK_CACHE_PREFIX = 'mediastationgo-artwork-'
const ARTWORK_RETIRE_RELOAD_KEY = 'mediastationgo-artwork-worker-retired'

async function retireArtworkServiceWorker() {
  const registrations = await navigator.serviceWorker.getRegistrations()
  const artworkRegistrations = registrations.filter((registration) => {
    const worker = registration.active ?? registration.waiting ?? registration.installing
    return isArtworkServiceWorkerScript(worker?.scriptURL)
  })
  const controlledByArtworkWorker = isArtworkServiceWorkerScript(
    navigator.serviceWorker.controller?.scriptURL,
  )

  const results = await Promise.all(artworkRegistrations.map((registration) => registration.unregister()))
  if (results.some((unregistered) => !unregistered)) {
    throw new Error('artwork service worker unregister returned false')
  }

  const cacheNames = await caches.keys()
  await Promise.all(cacheNames
    .filter((name) => name.startsWith(ARTWORK_CACHE_PREFIX))
    .map((name) => caches.delete(name)))

  if (controlledByArtworkWorker) {
    if (sessionStorage.getItem(ARTWORK_RETIRE_RELOAD_KEY) !== '1') {
      sessionStorage.setItem(ARTWORK_RETIRE_RELOAD_KEY, '1')
      window.location.reload()
      return
    }
    console.error('Artwork service worker still controls the page after retirement reload')
    return
  }
  sessionStorage.removeItem(ARTWORK_RETIRE_RELOAD_KEY)
}

function isArtworkServiceWorkerScript(scriptURL?: string) {
  if (!scriptURL) return false
  try {
    return new URL(scriptURL, window.location.origin).pathname === ARTWORK_SERVICE_WORKER_PATH
  } catch {
    return false
  }
}

// Application root: BrowserRouter + global toast container.
ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <BrowserRouter>
      <GlobalEvents />
      <App />
      <Toaster
        position="top-right"
        toastOptions={{
          className: '!bg-surface-800 !text-white !border !border-white/10',
        }}
      />
    </BrowserRouter>
  </React.StrictMode>,
)
