import { useState } from 'react'
import { api } from '../api/client'
import {
  DIAGNOSTICS_STATUS_VALUES_16, DIAGNOSTICS_STATUS_VALUES_2X, FIRMWARE_STATUS_VALUES, type OcppVersion,
} from '../types/station'

interface Props {
  stationId: string
  version: OcppVersion
}

// UpdateFirmware/GetDiagnostics (1.6) or GetLog (2.0.1/2.1) requests from the
// CSMS are auto-accepted by the backend (see the simulator adapters) — this
// panel is how the operator reports progress afterward, same "operator
// picks a status, nothing auto-progresses" pattern as MeterValues.
export function MaintenancePanel({ stationId, version }: Props) {
  const diagnosticsValues = version === '1.6' ? DIAGNOSTICS_STATUS_VALUES_16 : DIAGNOSTICS_STATUS_VALUES_2X
  const [firmwareStatus, setFirmwareStatus] = useState(FIRMWARE_STATUS_VALUES[0])
  const [diagnosticsStatus, setDiagnosticsStatus] = useState(diagnosticsValues[0])
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')

  const run = async (label: string, action: () => Promise<void>) => {
    setBusy(label)
    setError('')
    try {
      await action()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy('')
    }
  }

  return (
    <div className="connector-panel">
      <h3>펌웨어 / 진단</h3>
      <div className="row">
        <select value={firmwareStatus} onChange={(event) => setFirmwareStatus(event.target.value)}>
          {FIRMWARE_STATUS_VALUES.map((value) => (
            <option key={value} value={value}>{value}</option>
          ))}
        </select>
        <button
          disabled={!!busy}
          onClick={() => run('firmware', () => api.firmwareStatusNotification(stationId, firmwareStatus))}
        >
          FirmwareStatusNotification 전송
        </button>
      </div>
      <div className="row">
        <select value={diagnosticsStatus} onChange={(event) => setDiagnosticsStatus(event.target.value)}>
          {diagnosticsValues.map((value) => (
            <option key={value} value={value}>{value}</option>
          ))}
        </select>
        <button
          disabled={!!busy}
          onClick={() => run('diagnostics', () => api.diagnosticsStatusNotification(stationId, diagnosticsStatus))}
        >
          {version === '1.6' ? 'DiagnosticsStatusNotification' : 'LogStatusNotification'} 전송
        </button>
      </div>
      {error && <p className="error small">{error}</p>}
    </div>
  )
}
