import { useMemo } from 'react'
import type { Station } from '../types/station'

interface Props {
  stations: Station[]
  onSelect: (id: string) => void
  onDelete: (id: string) => void
}

const STATE_LABEL: Record<string, string> = {
  connected: '연결됨',
  connecting: '연결 중',
  disconnected: '연결 끊김',
  not_running: '중지됨',
}

function StationCard({ station, onSelect, onDelete }: { station: Station; onSelect: (id: string) => void; onDelete: (id: string) => void }) {
  return (
    <div className={`station-card${station.deletedAt ? ' station-card-deleted' : ''}`} onClick={() => onSelect(station.id)}>
      <div className="station-card-header">
        <strong>{station.identity}</strong>
        {station.deletedAt ? (
          <span className="state-badge state-disconnected">삭제됨</span>
        ) : (
          <span className={`state-badge state-${station.state}`}>{STATE_LABEL[station.state] ?? station.state}</span>
        )}
      </div>
      <div className="muted small">
        OCPP {station.version} · 커넥터 {station.connectorCount}개 · 생성자 {station.createdBy}
      </div>
      {!station.deletedAt && (
        <button
          className="danger small-btn"
          onClick={(event) => {
            event.stopPropagation()
            if (window.confirm(`${station.identity} 을(를) 삭제할까요?`)) onDelete(station.id)
          }}
        >
          삭제
        </button>
      )}
    </div>
  )
}

// Grouped by CSMS URL so "what's pointed at which server" is visible at a
// glance instead of having to read each card's small-print URL line one by
// one — the whole reason each card used to print csmsUrl at all.
export function StationList({ stations, onSelect, onDelete }: Props) {
  const groups = useMemo(() => {
    const byServer = new Map<string, Station[]>()
    for (const station of stations) {
      const key = station.csmsUrl
      if (!byServer.has(key)) byServer.set(key, [])
      byServer.get(key)!.push(station)
    }
    return [...byServer.entries()].sort(([a], [b]) => a.localeCompare(b))
  }, [stations])

  if (stations.length === 0) {
    return <p className="muted">아직 생성된 가상 충전소가 없습니다. 우측 상단의 "충전소 생성" 버튼으로 시작하세요.</p>
  }

  return (
    <div className="server-groups">
      {groups.map(([csmsUrl, group]) => {
        const connected = group.filter((station) => station.state === 'connected').length
        return (
          <section key={csmsUrl} className="server-group">
            <div className="server-group-header">
              <code>{csmsUrl}</code>
              <span className="muted small">
                {group.length}개 {connected > 0 && `· 연결됨 ${connected}`}
              </span>
            </div>
            <div className="station-grid">
              {group.map((station) => (
                <StationCard key={station.id} station={station} onSelect={onSelect} onDelete={onDelete} />
              ))}
            </div>
          </section>
        )
      })}
    </div>
  )
}
