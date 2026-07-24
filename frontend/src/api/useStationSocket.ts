import { useEffect, useState } from 'react'
import type { WsEvent } from '../types/station'

const RECONNECT_DELAY_MS = 2000

/**
 * Subscribes to a station's live event feed. Events accumulate in a
 * capped ring buffer (newest first) since this is a debugging log, not a
 * source of truth — the REST /events endpoint is that, for history predating
 * this hook's mount.
 */
export function useStationSocket(stationId: string | null, maxEvents = 500) {
  const [events, setEvents] = useState<WsEvent[]>([])
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    if (!stationId) return
    let socket: WebSocket | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let stopped = false

    const connect = () => {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      socket = new WebSocket(`${protocol}//${window.location.host}/api/stations/${stationId}/ws`)
      socket.onopen = () => setConnected(true)
      socket.onclose = () => {
        setConnected(false)
        if (!stopped) reconnectTimer = setTimeout(connect, RECONNECT_DELAY_MS)
      }
      socket.onerror = () => socket?.close()
      socket.onmessage = (message) => {
        try {
          const event = JSON.parse(message.data) as WsEvent
          setEvents((previous) => [event, ...previous].slice(0, maxEvents))
        } catch {
          // ignore malformed frames
        }
      }
    }
    connect()

    return () => {
      stopped = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      socket?.close()
    }
  }, [stationId, maxEvents])

  return { events, connected }
}
