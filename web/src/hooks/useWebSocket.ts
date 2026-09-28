import { useEffect, useRef } from 'react'

import { ensureAccessToken } from '../api/client'
import { useAuthStore } from '../stores/auth'

const MAX_RECONNECT_ATTEMPTS = 5

// useWebSocket opens a single connection to /api/ws and dispatches every
// message to the supplied handler. Auto-reconnects with back-off while the
// auth token is present, but stops after repeated failures so an expired token
// cannot create an endless /api/ws 401 loop.
export function useWebSocket(onEvent: (topic: string, payload: unknown) => void) {
  const ref = useRef<WebSocket | null>(null)
  const token = useAuthStore((s) => s.token)

  useEffect(() => {
    if (!token) return
    let closed = false
    let timer: number | undefined
    let reconnectAttempts = 0

    const open = async () => {
      if (closed) return
      if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) return
      const activeToken = await ensureAccessToken().catch(() => null)
      if (closed || !activeToken) return
      const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const url = `${proto}//${window.location.host}/api/ws?token=${encodeURIComponent(activeToken)}`
      const ws = new WebSocket(url)
      ref.current = ws
      ws.onopen = () => {
        reconnectAttempts = 0
      }
      ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data)
          if (msg && typeof msg.topic === 'string') {
            onEvent(msg.topic, msg.payload)
          }
        } catch {
          // ignore malformed frames
        }
      }
      ws.onclose = () => {
        if (closed) return
        reconnectAttempts += 1
        const delay = Math.min(3_000 * reconnectAttempts, 30_000)
        timer = window.setTimeout(() => { void open() }, delay)
      }
    }

    void open()
    return () => {
      closed = true
      if (timer) window.clearTimeout(timer)
      ref.current?.close()
    }
  }, [token, onEvent])
}
