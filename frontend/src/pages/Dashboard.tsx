import { useEffect, useState, useCallback, useMemo } from 'react'
import { api } from '../api/client'
import type { CreateStationInput, Station } from '../types/station'
import { StationList } from '../components/StationList'
import { CreateStationModal } from '../components/CreateStationModal'

interface Props {
  onSelect: (id: string) => void
}

type SortKey = 'created_desc' | 'created_asc' | 'identity' | 'state'

const SORT_LABEL: Record<SortKey, string> = {
  created_desc: '생성일 (최신순)',
  created_asc: '생성일 (오래된순)',
  identity: '이름순',
  state: '상태순 (연결됨 우선)',
}

const STATE_RANK: Record<string, number> = { connected: 0, connecting: 1, disconnected: 2, not_running: 3 }

function matches(station: Station, query: string) {
  const needle = query.trim().toLowerCase()
  if (!needle) return true
  return [station.identity, station.csmsUrl, station.createdBy, station.version].some((field) =>
    field.toLowerCase().includes(needle),
  )
}

function compare(a: Station, b: Station, sortBy: SortKey) {
  switch (sortBy) {
    case 'created_asc':
      return a.createdAt.localeCompare(b.createdAt)
    case 'identity':
      return a.identity.localeCompare(b.identity)
    case 'state':
      return (STATE_RANK[a.state] ?? 99) - (STATE_RANK[b.state] ?? 99)
    case 'created_desc':
    default:
      return b.createdAt.localeCompare(a.createdAt)
  }
}

export function Dashboard({ onSelect }: Props) {
  const [stations, setStations] = useState<Station[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [showDeleted, setShowDeleted] = useState(false)
  const [search, setSearch] = useState('')
  const [sortBy, setSortBy] = useState<SortKey>('created_desc')
  const [error, setError] = useState('')

  const refresh = useCallback(async (includeDeleted: boolean) => {
    try {
      setStations(await api.listStations(includeDeleted))
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh(showDeleted)
    const interval = setInterval(() => refresh(showDeleted), 5000)
    return () => clearInterval(interval)
  }, [refresh, showDeleted])

  const handleCreate = async (input: CreateStationInput) => {
    await api.createStation(input)
    setShowCreate(false)
    await refresh(showDeleted)
  }

  const handleDelete = async (id: string) => {
    await api.deleteStation(id)
    await refresh(showDeleted)
  }

  const visibleStations = useMemo(
    () => stations.filter((station) => matches(station, search)).sort((a, b) => compare(a, b, sortBy)),
    [stations, search, sortBy],
  )

  return (
    <div>
      <div className="page-header">
        <h1>가상 충전소</h1>
        <label className="checkbox-row small">
          <input type="checkbox" checked={showDeleted} onChange={(event) => setShowDeleted(event.target.checked)} />
          삭제된 충전소 포함
        </label>
        <button onClick={() => setShowCreate(true)}>충전소 생성</button>
      </div>
      <div className="row">
        <input
          placeholder="검색: identity, CSMS URL, 생성자, 버전"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          style={{ flex: 1 }}
        />
        <select value={sortBy} onChange={(event) => setSortBy(event.target.value as SortKey)}>
          {Object.entries(SORT_LABEL).map(([key, label]) => (
            <option key={key} value={key}>{label}</option>
          ))}
        </select>
      </div>
      {error && <p className="error">{error}</p>}
      {!loading && stations.length > 0 && (
        <p className="muted small">{visibleStations.length} / {stations.length}개 표시 중</p>
      )}
      {loading ? (
        <p className="muted">불러오는 중...</p>
      ) : stations.length > 0 && visibleStations.length === 0 ? (
        <p className="muted">검색 조건에 맞는 충전소가 없습니다.</p>
      ) : (
        <StationList stations={visibleStations} onSelect={onSelect} onDelete={handleDelete} />
      )}
      {showCreate && <CreateStationModal onCreate={handleCreate} onClose={() => setShowCreate(false)} />}
    </div>
  )
}
