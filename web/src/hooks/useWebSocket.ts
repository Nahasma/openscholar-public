import { useCallback, useEffect, useRef, useState } from 'react'
import type { ServerMessage } from '../types'

interface UseWebSocketOptions {
  url: string
  onMessage?: (msg: ServerMessage) => void
}

export function useWebSocket({ url, onMessage }: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null)
  const onMessageRef = useRef(onMessage)
  onMessageRef.current = onMessage
  const [isConnected, setIsConnected] = useState(false)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>(undefined)

  const connect = useCallback(() => {
    try {
      const ws = new WebSocket(url)
      wsRef.current = ws

      ws.onopen = () => {
        console.log('WebSocket connected')
        setIsConnected(true)
      }

      ws.onclose = () => {
        console.log('WebSocket disconnected, reconnecting in 3s...')
        setIsConnected(false)
        reconnectTimer.current = setTimeout(connect, 3000)
      }

      ws.onerror = () => {
        setIsConnected(false)
      }

      ws.onmessage = (event) => {
        try {
          const msg: ServerMessage = JSON.parse(event.data)
          onMessageRef.current?.(msg)
        } catch {
          console.error('Failed to parse WebSocket message:', event.data)
        }
      }
    } catch {
      console.error('WebSocket connection failed, retrying in 3s...')
      reconnectTimer.current = setTimeout(connect, 3000)
    }
  }, [url])

  useEffect(() => {
    connect()
    return () => {
      clearTimeout(reconnectTimer.current)
      wsRef.current?.close()
    }
  }, [connect])

  const send = useCallback((data: object) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(data))
    } else {
      console.warn('WebSocket not connected, message dropped')
    }
  }, [])

  return { send, isConnected }
}
