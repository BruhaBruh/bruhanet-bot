import { useCallback, useEffect, useRef, useSyncExternalStore } from 'react'
import { get } from './api'

// Мини-кэш «stale-while-revalidate»: экран при повторном заходе сразу рисуется из кэша, а свежие данные подтягиваются
// в фоне. Кэш живёт только в памяти страницы (ни в хранилище, ни на диске): ссылок подписки и ключей здесь нет,
// они берутся отдельными запросами по нажатию.
interface Entry<T = unknown> {
  data?: T
  error?: unknown // только пока данных нет: сбой обновления не затирает показанное
  fetching: boolean
}

const EMPTY: Entry = { fetching: false }
const cache = new Map<string, Entry>()
const subs = new Map<string, Set<() => void>>()
const inflight = new Map<string, Promise<void>>()

function put(key: string, e: Entry) {
  cache.set(key, e)
  subs.get(key)?.forEach((f) => f())
}

function subscribe(key: string, cb: () => void) {
  let set = subs.get(key)
  if (!set) subs.set(key, (set = new Set()))
  set.add(cb)
  return () => {
    set.delete(cb)
  }
}

// revalidate тянет свежие данные; параллельные вызовы по одному ключу делят один запрос.
function revalidate<T>(key: string, fetcher: () => Promise<T>): Promise<void> {
  const running = inflight.get(key)
  if (running) return running
  const prev = cache.get(key)
  put(key, { data: prev?.data, fetching: true }) // ошибку прошлой попытки сбрасываем: повтор снова показывает загрузку
  const p = fetcher()
    .then(
      (data) => put(key, { data, fetching: false }),
      (error: unknown) => put(key, prev?.data === undefined ? { error, fetching: false } : { data: prev.data, fetching: false }),
    )
    .finally(() => inflight.delete(key))
  inflight.set(key, p)
  return p
}

export function clearQueryCache() {
  cache.clear()
  inflight.clear()
}

export function useQuery<T>(key: string, fetcher?: () => Promise<T>) {
  const fetchRef = useRef<() => Promise<T>>(fetcher ?? (() => get<T>(key)))
  fetchRef.current = fetcher ?? (() => get<T>(key))
  const entry = useSyncExternalStore(
    (cb) => subscribe(key, cb),
    () => cache.get(key) ?? EMPTY,
  ) as Entry<T>

  useEffect(() => {
    void revalidate(key, () => fetchRef.current())
    // вернулись в приложение (свернули и открыли снова) — тоже обновляем
    const onVisible = () => document.visibilityState === 'visible' && void revalidate(key, () => fetchRef.current())
    document.addEventListener('visibilitychange', onVisible)
    return () => document.removeEventListener('visibilitychange', onVisible)
  }, [key])

  return {
    data: entry.data,
    error: entry.error,
    isLoading: entry.data === undefined && entry.error === undefined,
    isRefreshing: entry.fetching && entry.data !== undefined,
    refetch: useCallback(() => revalidate(key, () => fetchRef.current()), [key]),
    // оптимистичная правка кэша (удалили устройство — список обновился сразу, без запроса)
    mutate: useCallback((update: (old: T | undefined) => T) => {
      const cur = cache.get(key) as Entry<T> | undefined
      put(key, { data: update(cur?.data), fetching: cur?.fetching ?? false })
    }, [key]),
  }
}
