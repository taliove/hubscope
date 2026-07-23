// User management API calls (ticket 67).
import { http } from './client'
import type { Role } from './auth'

// User mirrors server.userDTO. password_hash is never present (W6).
export interface User {
  id: number
  username: string
  role: Role
  hub_id: number | null
  hub_name: string | null
  enabled: boolean
  created_at: string
}

export interface CreateUserPayload {
  username: string
  password: string
  role: Role
  // hub_id is required for hub-scoped roles and must be omitted for
  // super_admin. admin callers omit it (the backend pins it to the session
  // hub) so the client never needs to know the session hub.
  hub_id?: number
}

export interface UpdateUserPayload {
  // Every field is optional; an omitted field leaves the column unchanged.
  role?: Role
  hub_id?: number
  enabled?: boolean
}

export async function listUsers(): Promise<User[]> {
  return http.get<User[]>('/users')
}

export async function createUser(payload: CreateUserPayload): Promise<User> {
  return http.post<User>('/users', payload)
}

export async function updateUser(id: number, payload: UpdateUserPayload): Promise<User> {
  return http.patch<User>(`/users/${id}`, payload)
}

// resetPassword is a forced reset (the caller is already authorized); the
// old password is not required. Resolves to 204 (no body).
export async function resetPassword(id: number, password: string): Promise<void> {
  await http.put<void>(`/users/${id}/password`, { password })
}

export async function deleteUser(id: number): Promise<void> {
  await http.del<void>(`/users/${id}`)
}
