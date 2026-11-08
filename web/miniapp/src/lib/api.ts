import { initData } from './telegram'

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    readonly serverMessage: string,
  ) {
    super(`${status} ${code}`)
  }
}

interface Init {
  method?: 'GET' | 'POST' | 'DELETE'
  body?: unknown
}

// Авторизация — заголовок tma <initData> на каждом запросе: ни cookie, ни токенов в хранилище.
export async function api<T>(path: string, { method = 'GET', body }: Init = {}): Promise<T> {
  const headers: Record<string, string> = { Authorization: `tma ${initData()}` }
  if (method !== 'GET') headers['Content-Type'] = 'application/json'
  const res = await fetch(path, {
    method,
    headers,
    credentials: 'omit',
    cache: 'no-store',
    body: body === undefined ? (method === 'GET' ? undefined : '{}') : JSON.stringify(body),
  })
  if (!res.ok) {
    const e = (await res.json().catch(() => ({}))) as { error?: string; message?: string }
    throw new ApiError(res.status, e.error ?? 'error', e.message ?? '')
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const get = <T>(path: string) => api<T>(path)
export const post = <T>(path: string, body?: unknown) => api<T>(path, { method: 'POST', body })
export const del = <T>(path: string) => api<T>(path, { method: 'DELETE' })
