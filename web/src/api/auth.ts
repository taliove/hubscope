// Admin auth API: account/password login, logout, and session status.
import { http } from './client'

// Role mirrors the backend users.role column (spec 0005).
export type Role = 'super_admin' | 'admin' | 'operator' | 'viewer'

// AuthUser is the identity returned by GET /api/auth/me when authenticated.
// hub_id is null for super_admin (global, unbound to any Hub); hub_name
// mirrors hub_id and is null when unbound or when the Hub row is missing.
export interface AuthUser {
  id: number
  username: string
  role: Role
  hub_id: number | null
  hub_name: string | null
}

// AuthStatus is the shape of GET /api/auth/me. When unauthenticated, user is
// null; when authenticated, user carries the full identity. The `user`
// field is kept optional for callers that only read `.authenticated`
// (router guard, EndpointDetailView) so the type stays backward-compatible.
export interface AuthStatus {
  authenticated: boolean
  user?: AuthUser | null
}

// login posts the account/password pair; on success the response carries
// `{authenticated: true}` and the server sets the session cookie.
export function login(input: { username: string; password: string }): Promise<AuthStatus> {
  return http.post<AuthStatus>('/auth/login', input)
}

export function logout(): Promise<void> {
  return http.post<void>('/auth/logout')
}

export function fetchAuthStatus(): Promise<AuthStatus> {
  return http.get<AuthStatus>('/auth/me')
}
