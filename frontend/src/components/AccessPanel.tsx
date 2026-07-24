import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { StationAccessRow } from '../types/station'
import { useToast } from '../state/ToastContext'

interface Props {
  stationId: string
}

// Admin-only (see StationDetail's me.isAdmin check) — grants/revokes are the
// sole authority of admins, no delegated sharing (see plan). A user without
// any grant here gets a 404 for the station everywhere else in the app, not
// a 403, so this panel is the only place its access even becomes visible.
export function AccessPanel({ stationId }: Props) {
  const { showToast } = useToast()
  const [rows, setRows] = useState<StationAccessRow[]>([])
  const [username, setUsername] = useState('')
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')

  const refresh = () => api.listStationAccess(stationId).then(setRows).catch(() => {})

  useEffect(() => {
    refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stationId])

  const grant = async () => {
    setBusy('grant')
    setError('')
    try {
      await api.grantStationAccess(stationId, username)
      showToast(`접근 권한 부여됨 — ${username}`, 'success')
      setUsername('')
      await refresh()
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      showToast(`접근 권한 부여 실패 — ${message}`, 'error')
    } finally {
      setBusy('')
    }
  }

  const revoke = async (row: StationAccessRow) => {
    try {
      await api.revokeStationAccess(stationId, row.username)
      showToast(`접근 권한 회수됨 — ${row.username}`, 'success')
      await refresh()
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      showToast(`접근 권한 회수 실패 — ${message}`, 'error')
    }
  }

  return (
    <div className="connector-panel">
      <h3>접근 권한</h3>
      <p className="muted small">이 충전소를 조회/조작할 수 있는 사용자 목록 (관리자는 항상 모든 충전소에 접근 가능)</p>
      {rows.length === 0 && <p className="muted small">권한이 부여된 사용자가 없습니다.</p>}
      {rows.map((row) => (
        <div key={row.username} className="row small">
          <code>{row.username}</code>
          <span className="muted">부여자 {row.grantedBy}</span>
          <button className="danger small-btn" onClick={() => revoke(row)}>회수</button>
        </div>
      ))}
      <div className="row">
        <input placeholder="username" value={username} onChange={(event) => setUsername(event.target.value)} />
        <button disabled={!!busy || !username} onClick={grant}>권한 부여</button>
      </div>
      {error && <p className="error small">{error}</p>}
    </div>
  )
}
