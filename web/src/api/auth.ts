// Admin auth API: password login, logout, and session status.
import { http } from './client'

export interface AuthStatus {
  authenticated: boolean
}

export function login(password: string): Promise<AuthStatus> {
  return http.post<AuthStatus>('/auth/login', { password })
}

export function logout(): Promise<void> {
  return http.post<void>('/auth/logout')
}

export function fetchAuthStatus(): Promise<AuthStatus> {
  return http.get<AuthStatus>('/auth/me')
}
