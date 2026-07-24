import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { DATA_TRANSFER_STATUS_VALUES, type DataTransferHandlerRow } from '../types/station'

interface Props {
  stationId: string
}

// DataTransfer's payload is entirely vendor-defined, so there's no schema to
// drive a form from — the operator registers canned responses per
// vendorId/messageId for inbound requests, and can also send an outbound
// DataTransfer directly. See internal/simulator/datatransfer.go on the
// backend for the matching rules (exact match, then vendorId-only wildcard).
export function DataTransferPanel({ stationId }: Props) {
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
      setVendorID('')
      setMessageID('')
      setData('')
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy('')
    }
  }

  const remove = async (handler: DataTransferHandlerRow) => {
    try {
      await api.deleteDataTransferHandler(stationId, handler.id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
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
      setSendResult(`${result.status}${result.data ? ` — ${result.data}` : ''}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
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
      <div className="row">
        <input placeholder='응답 data (JSON 또는 텍스트, 선택)' value={data} onChange={(event) => setData(event.target.value)} style={{ flex: 1 }} />
        <button disabled={!!busy || !vendorID} onClick={register}>규칙 등록</button>
      </div>

      <p className="muted small">아웃바운드 전송</p>
      <div className="row">
        <input placeholder="vendorId" value={sendVendorID} onChange={(event) => setSendVendorID(event.target.value)} />
        <input placeholder="messageId (선택)" value={sendMessageID} onChange={(event) => setSendMessageID(event.target.value)} />
      </div>
      <div className="row">
        <input placeholder="data (선택)" value={sendData} onChange={(event) => setSendData(event.target.value)} style={{ flex: 1 }} />
        <button disabled={!!busy || !sendVendorID} onClick={send}>DataTransfer 전송</button>
      </div>
      {sendResult && <p className="muted small">응답: {sendResult}</p>}
      {error && <p className="error small">{error}</p>}
    </div>
  )
}
