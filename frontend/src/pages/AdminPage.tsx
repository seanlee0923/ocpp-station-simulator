import { useEffect, useState } from 'react'
import { adminApi, type AppUser } from '../api/auth'

interface Props {
  onBack: () => void
}

export function AdminPage({ onBack }: Props) {
  const [users, setUsers] = useState<AppUser[]>([])
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const refresh = () => adminApi.listUsers().then(setUsers).catch((err) => setError(String(err)))

  useEffect(() => {
    refresh()
  }, [])

  const create = async (event: React.FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await adminApi.createUser(username, password)
      setUsername('')
      setPassword('')
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const remove = async (user: AppUser) => {
    if (!window.confirm(`${user.username} 계정을 삭제할까요?`)) return
    try {
      await adminApi.deleteUser(user.id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div>
      <div className="page-header">
        <button onClick={onBack}>&larr; 목록으로</button>
        <h1>사용자 관리</h1>
      </div>

      <form className="form row" onSubmit={create}>
        <input placeholder="아이디" value={username} onChange={(event) => setUsername(event.target.value)} />
        <input
          type="password"
          placeholder="비밀번호 (8자 이상)"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
        <button type="submit" disabled={busy || !username || password.length < 8}>
          사용자 생성
        </button>
      </form>
      {error && <p className="error">{error}</p>}

      <table className="user-table">
        <thead>
          <tr>
            <th>아이디</th>
            <th>권한</th>
            <th>생성자</th>
            <th>생성일</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {users.map((user) => (
            <tr key={user.id}>
              <td>{user.username}</td>
              <td>{user.isAdmin ? '관리자' : '사용자'}</td>
              <td>{user.createdBy}</td>
              <td>{new Date(user.createdAt).toLocaleString()}</td>
              <td>
                <button className="danger small-btn" onClick={() => remove(user)}>
                  삭제
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
