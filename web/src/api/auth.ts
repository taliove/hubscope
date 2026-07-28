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

// CaptchaChallenge is the shape of GET /api/auth/captcha (spec 0012
// decision 2): a one-shot challenge id plus the PNG image as a data URI.
export interface CaptchaChallenge {
  captcha_id: string
  image: string
}

// login posts the account/password pair; on success the response carries
// `{authenticated: true}` and the server sets the session cookie. The
// optional captcha fields (spec 0012 decision 3) are sent only while the
// LoginView captcha section is expanded; the server ignores them when no
// captcha is required.
export function login(input: {
  username: string
  password: string
  captcha_id?: string
  captcha_answer?: string
}): Promise<AuthStatus> {
  return http.post<AuthStatus>('/auth/login', input)
}

// fetchCaptcha issues a new one-shot captcha challenge (spec 0012 decision
// 2). Every login submit destroys the used id server-side, so a fresh
// challenge must be fetched after each failed submit while the captcha
// section is expanded.
export function fetchCaptcha(): Promise<CaptchaChallenge> {
  return http.get<CaptchaChallenge>('/auth/captcha')
}

export function logout(): Promise<void> {
  return http.post<void>('/auth/logout')
}

export function fetchAuthStatus(): Promise<AuthStatus> {
  return http.get<AuthStatus>('/auth/me')
}
