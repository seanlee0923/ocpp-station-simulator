import { useState } from 'react'
import { api } from '../api/client'
import { MEASURAND_PRESETS, STATUS_VALUES_16, STATUS_VALUES_2X, type OcppVersion } from '../types/station'
import { useToast } from '../state/ToastContext'

const currentLocalDateTime = () => {
  const now = new Date()
  now.setMinutes(now.getMinutes() - now.getTimezoneOffset())
  return now.toISOString().slice(0, 19)
}

const toTimestamp = (value: string) => value ? new Date(value).toISOString() : undefined

interface Props {
  stationId: string
  connectorNumber: number
  version: OcppVersion
}

// One connector's controls: Authorize -> Start -> MeterValues -> Stop, plus
// an independent StatusNotification control. connectorNumber is used as
// both OCPP 1.6's connectorId and 2.0.1/2.1's evseId — a 1:1 mapping that's
// a reasonable simplification for a simulator (see plan).
export function ConnectorPanel({ stationId, connectorNumber, version }: Props) {
  const { showToast } = useToast()
  const [idTag, setIdTag] = useState('DEMO-TAG-01')
  const [transactionId, setTransactionId] = useState<string | null>(null)
  const [status, setStatus] = useState((version === '1.6' ? STATUS_VALUES_16 : STATUS_VALUES_2X)[0])
  const [measurand, setMeasurand] = useState(MEASURAND_PRESETS[0])
  const [meterValue, setMeterValue] = useState('0')
  const [timestamp, setTimestamp] = useState('')
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')

  const statusValues = version === '1.6' ? STATUS_VALUES_16 : STATUS_VALUES_2X

  // Every action toasts on completion — the panel's only prior feedback was
  // the button briefly disabling, which is easy to miss entirely.
  const run = async (label: string, action: () => Promise<string | void>) => {
    setBusy(label)
    setError('')
    try {
      const detail = await action()
      showToast(`커넥터 ${connectorNumber}: ${label}${detail ? ` — ${detail}` : ' 완료'}`, 'success')
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      showToast(`커넥터 ${connectorNumber}: ${label} 실패 — ${message}`, 'error')
    } finally {
      setBusy('')
    }
  }

  const authorize = () => run('Authorize', async () => {
    const result = await api.authorize(stationId, idTag)
    return `상태: ${result.status}`
  })

  const start = () => run('Start Transaction', async () => {
    const result = await api.startTransaction(stationId, {
      connectorId: connectorNumber, evseId: connectorNumber, idTag, meterStart: 0,
      timestamp: toTimestamp(timestamp),
    })
    setTransactionId(result.transactionId)
    return `트랜잭션 ${result.transactionId}`
  })

  const sendMeterValues = () => run('MeterValues 전송', async () => {
    await api.meterValues(stationId, {
      transactionId: transactionId ?? undefined,
      connectorId: connectorNumber,
      evseId: connectorNumber,
      samples: [{ measurand, value: meterValue }],
      timestamp: toTimestamp(timestamp),
    })
    return `${measurand}=${meterValue}`
  })

  const stop = () => run('Stop Transaction', async () => {
    if (!transactionId) return
    await api.stopTransaction(stationId, transactionId, {
      meterStop: Number(meterValue) || 0, reason: 'Local', timestamp: toTimestamp(timestamp),
    })
    setTransactionId(null)
  })

  const sendStatus = () => run('StatusNotification 전송', async () => {
    await api.statusNotification(stationId, {
      connectorId: connectorNumber, evseId: connectorNumber, status, timestamp: toTimestamp(timestamp),
    })
    return status
  })

  return (
    <div className="connector-panel">
      <div className="connector-header">
        <h3>커넥터 {connectorNumber}</h3>
        {transactionId && <span className="tx-badge">진행 중인 트랜잭션: {transactionId}</span>}
      </div>

      <div className="row">
        <input
          type="datetime-local"
          step="1"
          value={timestamp}
          onChange={(event) => setTimestamp(event.target.value)}
          aria-label="OCPP 타임스탬프"
        />
        <button type="button" disabled={!!busy} onClick={() => setTimestamp(currentLocalDateTime())}>
          현재 시각
        </button>
        <span className="muted small">비워 두면 백엔드에서 전송 시각을 사용합니다.</span>
      </div>

      <div className="row">
        <input value={idTag} onChange={(event) => setIdTag(event.target.value)} placeholder="idTag" />
        <button disabled={!!busy} onClick={authorize}>
          Authorize
        </button>
        <button disabled={!!busy || !!transactionId} onClick={start}>
          Start Transaction
        </button>
        <button disabled={!!busy || !transactionId} onClick={stop}>
          Stop Transaction
        </button>
      </div>

      <div className="row">
        <select value={measurand} onChange={(event) => setMeasurand(event.target.value)}>
          {MEASURAND_PRESETS.map((value) => (
            <option key={value} value={value}>
              {value}
            </option>
          ))}
        </select>
        <input value={meterValue} onChange={(event) => setMeterValue(event.target.value)} placeholder="값" />
        <button disabled={!!busy} onClick={sendMeterValues}>
          MeterValues 전송
        </button>
      </div>

      <div className="row">
        <select value={status} onChange={(event) => setStatus(event.target.value)}>
          {statusValues.map((value) => (
            <option key={value} value={value}>
              {value}
            </option>
          ))}
        </select>
        <button disabled={!!busy} onClick={sendStatus}>
          StatusNotification 전송
        </button>
      </div>

      {error && <p className="error small">{error}</p>}
    </div>
  )
}
