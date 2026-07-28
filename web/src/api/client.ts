// Lightweight fetch wrapper that unwraps the {"data"} / {"error"} envelope.

const BASE = '/api'

// Error carrying the server-provided message so the UI can surface it.
// captchaRequired mirrors the frozen login contract (spec 0012 decision 3):
// the `captcha_required` key exists only on the two captcha 401 responses,
// so its presence — not the message wording — is the unfold signal for the
// LoginView captcha section.
export class ApiError extends Error {
  readonly status: number
  readonly captchaRequired: boolean
  constructor(message: string, status: number, captchaRequired = false) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.captchaRequired = captchaRequired
  }
}

interface EnvelopeError {
  error?: { message?: string; captcha_required?: boolean }
}

interface EnvelopeData<T> {
  data: T
}

async function parseBody(res: Response): Promise<unknown> {
  const text = await res.text()
  if (!text) return undefined
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

function extractErrorMessage(body: unknown, status: number): string {
  const envelope = body as EnvelopeError | null
  const message = envelope?.error?.message
  if (message) return message
  return `请求失败(HTTP ${status})`
}

function extractCaptchaRequired(body: unknown): boolean {
  const envelope = body as EnvelopeError | null
  return envelope?.error?.captcha_required === true
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  const body = await parseBody(res)
  if (!res.ok) {
    // Session expired or missing: send the user to the login page, preserving
    // where they were headed. Skip when already on /login (or the login call
    // itself failed) to avoid a redirect loop.
    if (res.status === 401 && path !== '/auth/login' && window.location.pathname !== '/login') {
      const redirect = encodeURIComponent(window.location.pathname + window.location.search)
      window.location.href = `/login?redirect=${redirect}`
    }
    throw new ApiError(extractErrorMessage(body, res.status), res.status, extractCaptchaRequired(body))
  }
  // 204 and other empty successful responses carry no envelope.
  if (body === undefined) return undefined as T
  return (body as EnvelopeData<T>).data
}

export const http = {
  get: <T>(path: string) => request<T>(path, { method: 'GET' }),
  post: <T>(path: string, payload?: unknown) =>
    request<T>(path, { method: 'POST', body: JSON.stringify(payload ?? {}) }),
  put: <T>(path: string, payload?: unknown) =>
    request<T>(path, { method: 'PUT', body: JSON.stringify(payload ?? {}) }),
  patch: <T>(path: string, payload?: unknown) =>
    request<T>(path, { method: 'PATCH', body: JSON.stringify(payload ?? {}) }),
  del: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}
