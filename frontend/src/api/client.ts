import { apiFetch, ApiError } from './http'
import type {
  BootFields, BootResult, AuthorizeResult, CreateStationInput, DataTransferHandlerRow, MeterValuesInput,
  Station, StartTxInput, StartTxResult, StationEventRow, StatusInput, StopTxInput,
} from '../types/station'

export interface SendDataTransferInput {
  vendorId: string
  messageId?: string
  data?: string
}

export interface DataTransferResult {
  status: string
  data?: string
}

export interface DataTransferHandlerInput {
  vendorId: string
  messageId?: string
  status: string
  data?: string
}

const request = <T>(path: string, init?: RequestInit) => apiFetch<T>(`/api/stations${path}`, init)
const post = <T>(path: string, body?: unknown) =>
  request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined })

export const api = {
  listStations: (includeDeleted = false) => request<Station[]>(includeDeleted ? '?includeDeleted=true' : ''),
  getStation: (id: string) => request<Station>(`/${id}`),
  createStation: (input: CreateStationInput) => post<Station>('', input),
  deleteStation: (id: string) => request<void>(`/${id}`, { method: 'DELETE' }),
  connect: (id: string) => post<void>(`/${id}/connect`),
  disconnect: (id: string) => post<void>(`/${id}/disconnect`),
  bootNotification: (id: string, body: BootFields) => post<BootResult>(`/${id}/boot-notification`, body),
  authorize: (id: string, idTag: string) => post<AuthorizeResult>(`/${id}/authorize`, { idTag }),
  startTransaction: (id: string, body: StartTxInput) => post<StartTxResult>(`/${id}/transactions/start`, body),
  stopTransaction: (id: string, txId: string, body: StopTxInput) =>
    post<void>(`/${id}/transactions/${encodeURIComponent(txId)}/stop`, body),
  meterValues: (id: string, body: MeterValuesInput) => post<void>(`/${id}/meter-values`, body),
  statusNotification: (id: string, body: StatusInput) => post<void>(`/${id}/status-notification`, body),
  firmwareStatusNotification: (id: string, status: string) => post<void>(`/${id}/firmware-status-notification`, { status }),
  diagnosticsStatusNotification: (id: string, status: string) => post<void>(`/${id}/diagnostics-status-notification`, { status }),
  events: (id: string) => request<StationEventRow[]>(`/${id}/events`),
  sendDataTransfer: (id: string, body: SendDataTransferInput) => post<DataTransferResult>(`/${id}/data-transfer`, body),
  listDataTransferHandlers: (id: string) => request<DataTransferHandlerRow[]>(`/${id}/data-transfer-handlers`),
  createDataTransferHandler: (id: string, body: DataTransferHandlerInput) =>
    post<DataTransferHandlerRow>(`/${id}/data-transfer-handlers`, body),
  deleteDataTransferHandler: (id: string, handlerId: number) =>
    request<void>(`/${id}/data-transfer-handlers/${handlerId}`, { method: 'DELETE' }),
  getConfig: (id: string) => request<Record<string, string>>(`/${id}/config`),
}

export { ApiError }
