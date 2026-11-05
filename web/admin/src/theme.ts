import { useLayoutEffect, useSyncExternalStore } from 'react'

export type Theme = 'light' | 'dark'

interface TelegramWebApp {
  colorScheme?: Theme
  onEvent?: (event: 'themeChanged', fn: () => void) => void
  offEvent?: (event: 'themeChanged', fn: () => void) => void
}

declare global {
  interface Window {
    Telegram?: { WebApp?: TelegramWebApp }
  }
}

const KEY = 'bn_theme'
const QUERY = '(prefers-color-scheme: dark)'
const isTheme = (v: unknown): v is Theme => v === 'light' || v === 'dark'
const webApp = () => window.Telegram?.WebApp

// Источник истины один — читаем его заново, поэтому все пользователи хука видят одно и то же.
function read(): Theme {
  const scheme = webApp()?.colorScheme
  if (isTheme(scheme)) return scheme
  const saved = localStorage.getItem(KEY)
  if (isTheme(saved)) return saved
  return window.matchMedia?.(QUERY).matches ? 'dark' : 'light'
}

const listeners = new Set<() => void>()

function subscribe(fn: () => void) {
  const mq = window.matchMedia?.(QUERY)
  const app = webApp()
  listeners.add(fn)
  mq?.addEventListener('change', fn)
  app?.onEvent?.('themeChanged', fn)
  return () => {
    listeners.delete(fn)
    mq?.removeEventListener('change', fn)
    app?.offEvent?.('themeChanged', fn)
  }
}

function toggle() {
  localStorage.setItem(KEY, read() === 'dark' ? 'light' : 'dark')
  listeners.forEach((fn) => fn())
}

export function useTheme() {
  const theme = useSyncExternalStore(subscribe, read)
  useLayoutEffect(() => {
    document.documentElement.dataset.theme = theme
    document.documentElement.style.colorScheme = theme
  }, [theme])
  return { theme, toggle }
}
