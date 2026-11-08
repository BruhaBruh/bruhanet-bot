// Тонкая обёртка над Telegram Web App SDK (грузится с telegram.org в index.html). Вне Telegram всё безопасно пусто.
interface TgWebApp {
  initData: string
  colorScheme: 'light' | 'dark'
  ready(): void
  expand(): void
  close(): void
  openLink(url: string): void
  openTelegramLink(url: string): void
}

declare global {
  interface Window {
    Telegram?: { WebApp?: TgWebApp }
  }
}

export const tg = () => window.Telegram?.WebApp

// Подписанная строка запуска: сервер по ней узнаёт юзера. Нигде не сохраняется.
export const initData = () => tg()?.initData ?? ''

export function ready() {
  tg()?.ready()
  tg()?.expand()
}

export function closeApp() {
  const app = tg()
  if (app) app.close()
  else window.close()
}

export function openBot(username: string) {
  const url = `https://t.me/${username}`
  const app = tg()
  if (app) app.openTelegramLink(url)
  else window.open(url, '_blank', 'noopener,noreferrer')
}

// https-ссылки открывает Telegram (внешний браузер), схемы приложений (happ://…) — сама система.
export function openExternal(url: string) {
  const app = tg()
  if (url.startsWith('https://') && app) app.openLink(url)
  else window.location.href = url
}

// «Поделиться» ссылкой через выбор чата Telegram.
export function shareLink(url: string, text: string) {
  const share = `https://t.me/share/url?url=${encodeURIComponent(url)}&text=${encodeURIComponent(text)}`
  const app = tg()
  if (app) app.openTelegramLink(share)
  else window.open(share, '_blank', 'noopener,noreferrer')
}
