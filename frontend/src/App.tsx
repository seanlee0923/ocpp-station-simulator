import { useState } from 'react'
import { AuthProvider, useAuth } from './state/AuthContext'
import { ToastProvider } from './state/ToastContext'
import { ToastContainer } from './components/ToastContainer'
import { LoginPage } from './components/LoginPage'
import { Dashboard } from './pages/Dashboard'
import { StationDetail } from './pages/StationDetail'
import { AdminPage } from './pages/AdminPage'

type View = { name: 'dashboard' } | { name: 'station'; id: string } | { name: 'admin' }

function Shell() {
  const { me, loading, logout } = useAuth()
  const [view, setView] = useState<View>({ name: 'dashboard' })

  if (loading) return <p className="muted center-pad">불러오는 중...</p>
  if (!me) return <LoginPage />

  return (
    <main className="app-shell">
      <nav className="top-nav">
        <span className="brand">OCPP Station Simulator</span>
        <span className="muted small">{me.username}{me.isAdmin && ' (관리자)'}</span>
        {me.isAdmin && (
          <button className="small-btn" onClick={() => setView({ name: 'admin' })}>
            사용자 관리
          </button>
        )}
        <button className="small-btn" onClick={() => logout()}>
          로그아웃
        </button>
      </nav>

      {view.name === 'dashboard' && <Dashboard onSelect={(id) => setView({ name: 'station', id })} />}
      {view.name === 'station' && (
        <StationDetail stationId={view.id} onBack={() => setView({ name: 'dashboard' })} />
      )}
      {view.name === 'admin' && <AdminPage onBack={() => setView({ name: 'dashboard' })} />}
    </main>
  )
}

function App() {
  return (
    <AuthProvider>
      <ToastProvider>
        <Shell />
        <ToastContainer />
      </ToastProvider>
    </AuthProvider>
  )
}

export default App
