import { useState } from 'react'
import { api } from '../api/client'

interface Props {
  stationId: string
}

// Read-only: every value here came from the CSMS via ChangeConfiguration
// (1.6) or SetVariables (2.0.1/2.1) — see internal/simulator/configstore.go
// on the backend. There is no operator-facing setter; this panel exists so
// the operator can see what the CSMS has configured, including confirming a
// AuthorizationKey/BasicAuthPassword rotation actually landed (the value
// itself is withheld, but the key showing up here means it was received).
export function ConfigPanel({ stationId }: Props) {
  const [values, setValues] = useState<Record<string, string> | null>(null)
  const [error, setError] = useState('')

  const refresh = async () => {
    setError('')
    try {
      setValues(await api.getConfig(stationId))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  const entries = values ? Object.entries(values) : []

  return (
    <div className="connector-panel">
      <div className="connector-header">
        <h3>CSMS 설정값 (ChangeConfiguration / SetVariables)</h3>
        <button className="small-btn" onClick={refresh}>새로고침</button>
      </div>
      {error && <p className="error small">{error}</p>}
      {values !== null && entries.length === 0 && <p className="muted small">CSMS가 아직 아무 값도 보내지 않았습니다.</p>}
      {entries.map(([key, value]) => (
        <div key={key} className="row small">
          <code>{key}</code>
          <span>{value}</span>
        </div>
      ))}
    </div>
  )
}
