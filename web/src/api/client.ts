// Lightweight fetch wrapper that unwraps the {"data"} / {"error"} envelope.

const BASE = '/api'

// Error carrying the server-provided message so the UI can surface it.
export class ApiError extends Error {
  readonly status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

interface EnvelopeError {
  error?: { message?: string }
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

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  const body = await parseBody(res)
  if (!res.ok) {
    throw new ApiError(extractErrorMessage(body, res.status), res.status)
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
  del: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}
