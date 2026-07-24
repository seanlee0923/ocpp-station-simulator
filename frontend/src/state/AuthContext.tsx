import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { authApi, type Me } from '../api/auth'
import { ApiError } from '../api/http'

interface AuthContextValue {
  me: Me | null
  loading: boolean
  error: string
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    authApi
      .me()
      .then(setMe)
      .catch((err) => {
        // 401 just means "not logged in yet" — not a real error to surface.
        if (!(err instanceof ApiError && err.status === 401)) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
      .finally(() => setLoading(false))
  }, [])

  const login = async (username: string, password: string) => {
    setError('')
    const result = await authApi.login(username, password)
    setMe(result)
  }

  const logout = async () => {
    await authApi.logout()
    setMe(null)
  }

  return <AuthContext.Provider value={{ me, loading, error, login, logout }}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) throw new Error('useAuth must be used within AuthProvider')
  return context
}
