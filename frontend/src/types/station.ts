export type OcppVersion = '1.6' | '2.0.1' | '2.1'

export interface Station {
  id: string
  identity: string
  csmsUrl: string
  version: OcppVersion
  connectorCount: number
  basicAuthUser?: string
  insecureSkipTlsVerify: boolean
  createdBy: string
  createdAt: string
  lastKnownStatus: string
  state: string
  deletedAt?: string
}

export interface CreateStationInput {
  identity: string
  csmsUrl: string
  version: OcppVersion
  connectorCount: number
  basicAuthUser?: string
  basicAuthPass?: string
  insecureSkipTlsVerify: boolean
}

export interface BootFields {
  vendorName: string
  model: string
  serialNumber?: string
  firmwareVersion?: string
}

export interface BootResult {
  status: string
  currentTime: string
  interval: number
}

export interface AuthorizeResult {
  status: string
}

export interface StartTxInput {
  connectorId: number
  evseId: number
  idTag: string
  meterStart: number
}

export interface StartTxResult {
  transactionId: string
}

export interface StopTxInput {
  meterStop: number
  reason: string
}

export interface MeterSample {
  measurand: string
  value: string
  unit?: string
}

export interface MeterValuesInput {
  transactionId?: string
  connectorId: number
  evseId: number
  samples: MeterSample[]
}

export interface StatusInput {
  connectorId: number
  evseId: number
  status: string
  errorCode?: string
  info?: string
}

export interface StationEventRow {
  id: number
  actor: string
  eventType: string
  action?: string
  direction?: string
  payload?: unknown
  createdAt: string
}

export interface WsEvent {
  stationId: string
  type: string
  action?: string
  direction?: string
  actor?: string
  payload?: unknown
  timestamp: string
}

// OCPP status enum values per version, so ActionPanel can offer the right
// choices instead of a free-text field that's easy to typo into an invalid
// CALLERROR.
export const STATUS_VALUES_16 = [
  'Available', 'Preparing', 'Charging', 'SuspendedEVSE', 'SuspendedEV',
  'Finishing', 'Reserved', 'Unavailable', 'Faulted',
]
export const STATUS_VALUES_2X = ['Available', 'Occupied', 'Reserved', 'Unavailable', 'Faulted']

export const MEASURAND_PRESETS = [
  'Energy.Active.Import.Register',
  'Power.Active.Import',
  'Current.Import',
  'Voltage',
  'SoC',
  'Temperature',
]
