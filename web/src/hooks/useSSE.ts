import { useEffect, useRef, useCallback } from 'react'

import { useAuthStore } from '../stores/auth'
import type { SSEEvent } from '../types'

type SSEEventHandler = (event: SSEEvent) => void

/**
 * useSSE hook - 管理 Server-Sent Events 连接
 * 
 * @param onEvent - 事件处理函数
 * @param options - 配置选项
 * 
 * @example
 * ```tsx
 * function MyComponent() {
 *   const { connect, disconnect } = useSSE((event) => {
 *     if (event.type === 'scan') {
 *       updateScanProgress(event.payload)
 *     }
 *   })
 *   
 *   useEffect(() => {
 *     connect()
 *     return () => disconnect()
 *   }, [])
 *   
 *   return <div>SSE Demo</div>
 * }
 * ```
 */
export function useSSE(
  onEvent: SSEEventHandler,
  options: { autoConnect?: boolean } = {}
) {
  const { autoConnect = true } = options
  const eventSourceRef = useRef<EventSource | null>(null)
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const isConnectedRef = useRef(false)
  const reconnectAttemptsRef = useRef(0)
  const maxReconnectAttempts = 5

  const connect = useCallback(() => {
    // 如果已有连接，先断开
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
    }

    const token = useAuthStore.getState().token
    if (!token) {
      console.warn('Cannot connect to SSE: No auth token')
      return
    }

    const url = `/api/events?token=${encodeURIComponent(token)}`
    const eventSource = new EventSource(url)
    eventSourceRef.current = eventSource

    eventSource.onopen = () => {
      isConnectedRef.current = true
      reconnectAttemptsRef.current = 0
    }

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as SSEEvent
        onEvent(data)
      } catch (err) {
        console.error('Failed to parse SSE event:', err)
      }
    }

    eventSource.onerror = () => {
      isConnectedRef.current = false
      eventSource.close()

      // 尝试重连
      if (reconnectAttemptsRef.current < maxReconnectAttempts) {
        const delay = Math.min(1000 * Math.pow(2, reconnectAttemptsRef.current), 30000)
        reconnectAttemptsRef.current++
        reconnectTimeoutRef.current = setTimeout(connect, delay)
      } else {
        console.error('SSE connection failed after max attempts')
      }
    }

  }, [onEvent])

  const disconnect = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current)
      reconnectTimeoutRef.current = null
    }
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
      eventSourceRef.current = null
    }
    isConnectedRef.current = false
    reconnectAttemptsRef.current = 0
  }, [])

  const isConnected = useCallback(() => {
    return isConnectedRef.current
  }, [])

  useEffect(() => {
    if (autoConnect) {
      connect()
    }
    return () => {
      disconnect()
    }
  }, [autoConnect, connect, disconnect])

  return {
    connect,
    disconnect,
    isConnected,
  }
}
