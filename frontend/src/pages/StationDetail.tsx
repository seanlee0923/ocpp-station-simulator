import { useEffect, useMemo, useState } from 'react'
import { api } from '../api/client'
import { useStationSocket } from '../api/useStationSocket'
import type { Station, StationEventRow, WsEvent } from '../types/station'
import { ConnectorPanel } from '../components/ConnectorPanel'
import { EventLog } from '../components/EventLog'

interface Props {
  stationId: string
  onBack: () => void
}

function historyToWsEvent(row: StationEventRow): WsEvent {
  return {
    stationId: '', type: row.eventType, action: row.action, direction: row.direction,
    actor: row.actor, payload: row.payload, timestamp: row.createdAt,
  }
}

export function StationDetail({ stationId, onBack }: Props) {
  const [station, setStation] = useState<Station | null>(null)
  const [history, setHistory] = useState<StationEventRow[]>([])
  const [vendorName, setVendorName] = useState('Acme')
  const [model, setModel] = useState('SimStation')
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const { events, connected } = useStationSocket(stationId)

  const refresh = async () => {
    setStation(await api.getStation(stationId))
  }

  useEffect(() => {
    refresh()
    api.events(stationId).then(setHistory).catch(() => {})
    const interval = setInterval(refresh, 5000)
    return () => clearInterval(interval)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stationId])

  const combinedLog = useMemo(() => [...events, ...history.map(historyToWsEvent)], [events, history])

  const run = async (label: string, action: () => Promise<void>) => {
    setBusy(label)
    setError('')
    try {
      await action()
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy('')
    }
  }

  if (!station) return <p className="muted">불러오는 중...</p>

  const isDeleted = !!station.deletedAt

  return (
    <div>
      <div className="page-header">
        <button onClick={onBack}>&larr; 목록으로</button>
        <h1>{station.identity}</h1>
        {isDeleted ? (
          <span className="state-badge state-disconnected">삭제됨</span>
        ) : (
          <span className={`state-badge state-${station.state}`}>{station.state}</span>
        )}
      </div>
      <p className="muted small">
        {station.csmsUrl} · OCPP {station.version} · 생성자 {station.createdBy}
        {station.insecureSkipTlsVerify && ' · TLS 검증 안 함'}
      </p>

      {isDeleted ? (
        <p className="muted">
          삭제된 충전소입니다 (읽기 전용) — 아래 이력만 조회할 수 있고 더 이상 조작할 수 없습니다.
        </p>
      ) : (
        <>
          <div className="row">
            <button disabled={!!busy} onClick={() => run('connect', () => api.connect(stationId))}>
              연결
            </button>
            <button disabled={!!busy} onClick={() => run('disconnect', () => api.disconnect(stationId))}>
              연결 해제
            </button>
            <input value={vendorName} onChange={(event) => setVendorName(event.target.value)} placeholder="Vendor" />
            <input value={model} onChange={(event) => setModel(event.target.value)} placeholder="Model" />
            <button
              disabled={!!busy}
              onClick={() => run('boot', async () => {
                await api.bootNotification(stationId, { vendorName, model })
              })}
            >
              BootNotification 전송
            </button>
          </div>
          {error && <p className="error">{error}</p>}

          <div className="connector-grid">
            {Array.from({ length: station.connectorCount }, (_, index) => index + 1).map((number) => (
              <ConnectorPanel key={number} stationId={stationId} connectorNumber={number} version={station.version} />
            ))}
          </div>
        </>
      )}

      <EventLog entries={combinedLog} connected={connected} />
    </div>
  )
}
