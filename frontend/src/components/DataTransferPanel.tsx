import { useEffect, useMemo, useState } from 'react'
import { api } from '../api/client'
import { DATA_TRANSFER_STATUS_VALUES, type DataTransferHandlerRow } from '../types/station'
import { useToast } from '../state/ToastContext'

interface Props {
  stationId: string
}

interface JsonFieldProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
}

// A plain textarea plus a JSON on/off toggle — not a real code editor (no
// syntax highlighting/autocomplete), but covers the actual pain point of
// hand-typing DataTransfer's vendor-defined JSON in a single-line input,
// without pulling in a CodeMirror/Monaco dependency for what's a small
// payload field in an internal test tool. The toggle exists because
// DataTransfer.Data is just a string on the wire — some vendors send plain
// text, not JSON — so JSON validation/formatting is opt-in, not assumed.
// The raw text is sent as-is either way; the toggle only controls whether
// this UI assists with it.
function JsonField({ value, onChange, placeholder }: JsonFieldProps) {
  const [jsonMode, setJsonMode] = useState(true)

  const validity = useMemo(() => {
    if (!jsonMode || !value.trim()) return null
    try {
      JSON.parse(value)
      return 'valid' as const
    } catch (err) {
      return err instanceof Error ? err.message : 'invalid JSON'
    }
  }, [jsonMode, value])

  const format = () => {
    try {
      onChange(JSON.stringify(JSON.parse(value), null, 2))
    } catch {
      // leave the text as-is; the validity message below already explains why
    }
  }

  return (
    <div className="json-field">
      <textarea
        className="json-textarea"
        rows={4}
        placeholder={placeholder}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
      <div className="json-field-footer">
        <label className="toggle-switch">
          <input type="checkbox" checked={jsonMode} onChange={(event) => setJsonMode(event.target.checked)} />
          <span className="toggle-slider" />
        </label>
        <span className="small muted">JSON</span>
        {jsonMode && validity === 'valid' && <span className="json-valid small">유효한 JSON</span>}
        {jsonMode && validity && validity !== 'valid' && <span className="error small">{validity}</span>}
        {jsonMode && (
          <button type="button" className="small-btn" onClick={format} disabled={validity !== 'valid'}>
            포맷
          </button>
        )}
      </div>
    </div>
  )
}

// DataTransfer's payload is entirely vendor-defined, so there's no schema to
// drive a form from — the operator registers canned responses per
// vendorId/messageId for inbound requests, and can also send an outbound
// DataTransfer directly. See internal/simulator/datatransfer.go on the
// backend for the matching rules (exact match, then vendorId-only wildcard).
export function DataTransferPanel({ stationId }: Props) {
  const { showToast } = useToast()
  const [handlers, setHandlers] = useState<DataTransferHandlerRow[]>([])
  const [vendorID, setVendorID] = useState('')
  const [messageID, setMessageID] = useState('')
  const [status, setStatus] = useState(DATA_TRANSFER_STATUS_VALUES[0])
  const [data, setData] = useState('')
  const [sendVendorID, setSendVendorID] = useState('')
  const [sendMessageID, setSendMessageID] = useState('')
  const [sendData, setSendData] = useState('')
  const [sendResult, setSendResult] = useState('')
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')

  const refresh = () => api.listDataTransferHandlers(stationId).then(setHandlers).catch(() => {})

  useEffect(() => {
    refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stationId])

  const register = async () => {
    setBusy('register')
    setError('')
    try {
      await api.createDataTransferHandler(stationId, { vendorId: vendorID, messageId: messageID || undefined, status, data })
      showToast(`응답 규칙 등록됨 — ${vendorID}${messageID ? ` / ${messageID}` : ''} → ${status}`, 'success')
      setVendorID('')
      setMessageID('')
      setData('')
      await refresh()
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      showToast(`규칙 등록 실패 — ${message}`, 'error')
    } finally {
      setBusy('')
    }
  }

  const remove = async (handler: DataTransferHandlerRow) => {
    try {
      await api.deleteDataTransferHandler(stationId, handler.id)
      showToast(`규칙 삭제됨 — ${handler.vendorId}`, 'success')
      await refresh()
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      showToast(`규칙 삭제 실패 — ${message}`, 'error')
    }
  }

  const send = async () => {
    setBusy('send')
    setError('')
    setSendResult('')
    try {
      const result = await api.sendDataTransfer(stationId, {
        vendorId: sendVendorID, messageId: sendMessageID || undefined, data: sendData || undefined,
      })
      const resultText = `${result.status}${result.data ? ` — ${result.data}` : ''}`
      setSendResult(resultText)
      showToast(`DataTransfer 전송 — ${resultText}`, 'success')
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      showToast(`DataTransfer 전송 실패 — ${message}`, 'error')
    } finally {
      setBusy('')
    }
  }

  return (
    <div className="connector-panel">
      <h3>DataTransfer</h3>

      <p className="muted small">인바운드 응답 규칙 (CSMS가 이 vendorId/messageId로 보내면 아래대로 응답)</p>
      {handlers.length === 0 && <p className="muted small">등록된 규칙이 없습니다. CSMS가 보낸 미등록 요청은 UnknownVendorId로 응답됩니다.</p>}
      {handlers.map((handler) => (
        <div key={handler.id} className="row small">
          <code>{handler.vendorId}{handler.messageId ? ` / ${handler.messageId}` : ' (모든 messageId)'}</code>
          <span>→ {handler.status}</span>
          <button className="danger small-btn" onClick={() => remove(handler)}>삭제</button>
        </div>
      ))}
      <div className="row">
        <input placeholder="vendorId" value={vendorID} onChange={(event) => setVendorID(event.target.value)} />
        <input placeholder="messageId (선택)" value={messageID} onChange={(event) => setMessageID(event.target.value)} />
        <select value={status} onChange={(event) => setStatus(event.target.value)}>
          {DATA_TRANSFER_STATUS_VALUES.map((value) => (
            <option key={value} value={value}>{value}</option>
          ))}
        </select>
      </div>
      <JsonField value={data} onChange={setData} placeholder="응답 data (JSON 또는 텍스트, 선택)" />
      <div className="row">
        <button disabled={!!busy || !vendorID} onClick={register}>규칙 등록</button>
      </div>

      <p className="muted small">아웃바운드 전송</p>
      <div className="row">
        <input placeholder="vendorId" value={sendVendorID} onChange={(event) => setSendVendorID(event.target.value)} />
        <input placeholder="messageId (선택)" value={sendMessageID} onChange={(event) => setSendMessageID(event.target.value)} />
      </div>
      <JsonField value={sendData} onChange={setSendData} placeholder="data (JSON 또는 텍스트, 선택)" />
      <div className="row">
        <button disabled={!!busy || !sendVendorID} onClick={send}>DataTransfer 전송</button>
      </div>
      {sendResult && <p className="muted small">응답: {sendResult}</p>}
      {error && <p className="error small">{error}</p>}
    </div>
  )
}
