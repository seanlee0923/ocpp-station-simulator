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

export function StationList({ stations, onSelect, onDelete }: Props) {
  if (stations.length === 0) {
    return <p className="muted">아직 생성된 가상 충전소가 없습니다. 우측 상단의 "충전소 생성" 버튼으로 시작하세요.</p>
  }

  return (
    <div className="station-grid">
      {stations.map((station) => (
        <div key={station.id} className={`station-card${station.deletedAt ? ' station-card-deleted' : ''}`} onClick={() => onSelect(station.id)}>
          <div className="station-card-header">
            <strong>{station.identity}</strong>
            {station.deletedAt ? (
              <span className="state-badge state-disconnected">삭제됨</span>
            ) : (
              <span className={`state-badge state-${station.state}`}>{STATE_LABEL[station.state] ?? station.state}</span>
            )}
          </div>
          <div className="muted small">{station.csmsUrl}</div>
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
      ))}
    </div>
  )
}
