import { router } from './router'

// Ошибка ответа с телом: для 400 сервер кладёт {error, message, problems?}.
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly body: { error?: string; message?: string; problems?: string[] },
  ) {
    super(`${status}`)
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  // FormData — multipart: Content-Type с boundary ставит сам браузер
  const form = body instanceof FormData
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: form ? undefined : { 'Content-Type': 'application/json' },
    body: form ? body : body === undefined ? undefined : JSON.stringify(body),
  })
  if (res.status === 401) {
    await router.push('/login-required')
    throw new Error('unauthorized')
  }
  if (!res.ok) throw new ApiError(res.status, await res.json().catch(() => ({})))
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const get = <T>(path: string) => request<T>('GET', path)
export const post = <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {})
export const put = <T>(path: string, body: unknown) => request<T>('PUT', path, body)
export const del = <T>(path: string) => request<T>('DELETE', path)

export function upload<T>(path: string, file: File) {
  const f = new FormData()
  f.append('file', file)
  return request<T>('POST', path, f)
}
