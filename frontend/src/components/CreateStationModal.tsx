import { useState } from 'react'
import type { CreateStationInput, OcppVersion } from '../types/station'

interface Props {
  onCreate: (input: CreateStationInput) => Promise<void>
  onClose: () => void
}

export function CreateStationModal({ onCreate, onClose }: Props) {
  const [identity, setIdentity] = useState('')
  const [csmsUrl, setCsmsUrl] = useState('ws://localhost:8080')
  const [version, setVersion] = useState<OcppVersion>('1.6')
  const [connectorCount, setConnectorCount] = useState(1)
  const [heartbeatInterval, setHeartbeatInterval] = useState(60)
  const [pingInterval, setPingInterval] = useState(0)
  const [basicAuthUser, setBasicAuthUser] = useState('')
  const [basicAuthUserTouched, setBasicAuthUserTouched] = useState(false)
  const [basicAuthPass, setBasicAuthPass] = useState('')
  const [insecureSkipTlsVerify, setInsecureSkipTlsVerify] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const isWss = csmsUrl.trim().toLowerCase().startsWith('wss://')

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    setSubmitting(true)
    setError('')
    try {
      await onCreate({
        identity: identity.trim(),
        csmsUrl: csmsUrl.trim(),
        version,
        connectorCount,
        heartbeatInterval,
        pingInterval,
        basicAuthUser: basicAuthUser.trim() || undefined,
        basicAuthPass: basicAuthPass || undefined,
        insecureSkipTlsVerify,
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setSubmitting(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(event) => event.stopPropagation()}>
        <h2>가상 충전소 생성</h2>
        <form onSubmit={submit} className="form">
          <label>
            Identity (충전소 식별자)
            <input
              required
              value={identity}
              onChange={(event) => {
                const value = event.target.value
                setIdentity(value)
                // Most CSMS (including this app's own, by default) require
                // the Basic Auth username to equal the charge point identity
                // — see csms/security.go's AllowBasicUsernameMismatch check.
                // Keep them in sync until the operator explicitly edits the
                // username field themselves.
                if (!basicAuthUserTouched) setBasicAuthUser(value)
              }}
              placeholder="CP-001"
            />
          </label>
          <label>
            CSMS URL
            <input
              required
              value={csmsUrl}
              onChange={(event) => setCsmsUrl(event.target.value)}
              placeholder="ws://host:port 또는 wss://host:port"
            />
          </label>
          <label>
            OCPP 버전
            <select value={version} onChange={(event) => setVersion(event.target.value as OcppVersion)}>
              <option value="1.6">OCPP 1.6</option>
              <option value="2.0.1">OCPP 2.0.1</option>
              <option value="2.1">OCPP 2.1</option>
            </select>
          </label>
          <label>
            커넥터 수
            <input
              type="number"
              min={1}
              max={16}
              value={connectorCount}
              onChange={(event) => setConnectorCount(Math.max(1, Number(event.target.value) || 1))}
            />
          </label>
          <label>
            Heartbeat 주기 (초, 0이면 비활성화)
            <input
              type="number"
              min={0}
              value={heartbeatInterval}
              onChange={(event) => setHeartbeatInterval(Math.max(0, Number(event.target.value) || 0))}
            />
          </label>
          <label>
            WebSocket Ping 주기 (초, 0이면 비활성화)
            <input
              type="number"
              min={0}
              value={pingInterval}
              onChange={(event) => setPingInterval(Math.max(0, Number(event.target.value) || 0))}
            />
            <span className="muted small">
              OCPP Heartbeat와는 다른 계층입니다. 유휴 연결을 끊는 CSMS는 ping 프레임을 봅니다.
              CSMS가 WebSocketPingInterval을 지정하면 그 값이 저장되어 다음 연결부터 적용됩니다.
            </span>
          </label>
          <fieldset>
            <legend>Basic Auth (선택)</legend>
            <label>
              Username (기본값: Identity와 동일 — 필요 시 직접 수정)
              <input
                value={basicAuthUser}
                onChange={(event) => {
                  setBasicAuthUser(event.target.value)
                  setBasicAuthUserTouched(true)
                }}
              />
            </label>
            <label>
              Password
              <input type="password" value={basicAuthPass} onChange={(event) => setBasicAuthPass(event.target.value)} />
            </label>
          </fieldset>
          {isWss && (
            <label className="checkbox-row">
              <input
                type="checkbox"
                checked={insecureSkipTlsVerify}
                onChange={(event) => setInsecureSkipTlsVerify(event.target.checked)}
              />
              TLS 인증서 검증 건너뛰기 (자체 서명/내부 CA 테스트 CSMS 전용 — 운영 환경에서 사용 금지)
            </label>
          )}
          {error && <p className="error">{error}</p>}
          <div className="modal-actions">
            <button type="button" onClick={onClose} disabled={submitting}>
              취소
            </button>
            <button type="submit" disabled={submitting}>
              {submitting ? '생성 중...' : '생성'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
