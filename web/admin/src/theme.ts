import { useSyncExternalStore } from 'react'

const query = '(prefers-color-scheme: dark)'

function subscribe(fn: () => void) {
  const mq = window.matchMedia?.(query)
  mq?.addEventListener('change', fn)
  return () => mq?.removeEventListener('change', fn)
}

const snapshot = () => (window.matchMedia?.(query).matches ? 'dark' : 'light')

// Системная тема на лету, без перезагрузки.
export function useAppearance(): 'light' | 'dark' {
  return useSyncExternalStore(subscribe, snapshot)
}
