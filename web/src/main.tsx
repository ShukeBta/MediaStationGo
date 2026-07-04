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
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/artwork-cache-sw.js').catch(() => undefined)
  })
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
