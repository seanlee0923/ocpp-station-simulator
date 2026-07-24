import { apiFetch } from './http'

export interface Me {
  username: string
  isAdmin: boolean
}

export interface AppUser {
  id: number
  username: string
  isAdmin: boolean
  createdBy: string
  createdAt: string
}

export const authApi = {
  login: (username: string, password: string) =>
    apiFetch<Me>('/api/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
  logout: () => apiFetch<void>('/api/auth/logout', { method: 'POST' }),
  me: () => apiFetch<Me>('/api/auth/me'),
}

export const adminApi = {
  listUsers: () => apiFetch<AppUser[]>('/api/admin/users'),
  createUser: (username: string, password: string) =>
    apiFetch<AppUser>('/api/admin/users', { method: 'POST', body: JSON.stringify({ username, password }) }),
  deleteUser: (id: number) => apiFetch<void>(`/api/admin/users/${id}`, { method: 'DELETE' }),
}
