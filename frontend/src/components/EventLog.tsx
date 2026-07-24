import type { WsEvent } from '../types/station'

interface Props {
  entries: WsEvent[]
  connected: boolean
}

const TYPE_LABEL: Record<string, string> = {
  created: '생성됨',
  deleted: '삭제됨',
  connect_requested: '연결 요청',
  disconnect_requested: '연결 해제 요청',
  connected: '연결됨',
  disconnected: '연결 끊김',
  message_sent: '송신',
  message_received: '수신',
  remote_command_received: 'CSMS 원격 명령',
}

// A single chronological feed covering connection lifecycle, every OCPP
// frame sent/received, and CSMS-initiated remote commands — all of it comes
// from the same db.StationEvent shape on the backend (see plan), so one
// component renders all of it rather than three near-duplicate lists.
export function EventLog({ entries, connected }: Props) {
  return (
    <div className="event-log">
      <div className="event-log-header">
        <h3>메시지 로그 / 이력</h3>
        <span className={connected ? 'ws-badge ws-connected' : 'ws-badge'}>{connected ? '실시간 연결됨' : '실시간 연결 끊김'}</span>
      </div>
      <div className="event-log-list">
        {entries.length === 0 && <p className="muted">아직 이벤트가 없습니다.</p>}
        {entries.map((entry, index) => (
          <div key={index} className={`event-row event-${entry.type}`}>
            <span className="event-time">{new Date(entry.timestamp).toLocaleTimeString()}</span>
            <span className="event-type">{TYPE_LABEL[entry.type] ?? entry.type}</span>
            {entry.action && <span className="event-action">{entry.action}</span>}
            {entry.actor && <span className="event-actor">{entry.actor}</span>}
            {entry.payload !== undefined && entry.payload !== null && (
              <pre className="event-payload">{JSON.stringify(entry.payload)}</pre>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
