import { useEffect, useState, useCallback } from 'react'
import { api } from '../api/client'
import type { CreateStationInput, Station } from '../types/station'
import { StationList } from '../components/StationList'
import { CreateStationModal } from '../components/CreateStationModal'

interface Props {
  onSelect: (id: string) => void
}

export function Dashboard({ onSelect }: Props) {
  const [stations, setStations] = useState<Station[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    try {
      setStations(await api.listStations())
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
    const interval = setInterval(refresh, 5000)
    return () => clearInterval(interval)
  }, [refresh])

  const handleCreate = async (input: CreateStationInput) => {
    await api.createStation(input)
    setShowCreate(false)
    await refresh()
  }

  const handleDelete = async (id: string) => {
    await api.deleteStation(id)
    await refresh()
  }

  return (
    <div>
      <div className="page-header">
        <h1>가상 충전소</h1>
        <button onClick={() => setShowCreate(true)}>충전소 생성</button>
      </div>
      {error && <p className="error">{error}</p>}
      {loading ? <p className="muted">불러오는 중...</p> : <StationList stations={stations} onSelect={onSelect} onDelete={handleDelete} />}
      {showCreate && <CreateStationModal onCreate={handleCreate} onClose={() => setShowCreate(false)} />}
    </div>
  )
}
