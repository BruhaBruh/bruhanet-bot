import { useEffect, useRef } from 'react'
import { useNavigate } from 'react-router'
import { Spinner } from '@/ui'

// Вход одним тапом: кнопка «🛠 Админка» в боте открывает эту страницу как Mini App. Telegram кладёт подписанные
// данные в хеш адреса (#tgWebAppData=…); SDK не нужен. Сервер проверяет подпись, свежесть и что это админ, и ставит
// сессионную cookie. Хеш сразу убираем из адреса и истории. Отказ — обычная страница входа (/login в боте).
export default function TgLogin() {
  const navigate = useNavigate()
  // StrictMode гоняет эффект дважды: подпись одноразовая, второй запрос сервер отклонил бы
  const started = useRef(false)
  useEffect(() => {
    if (started.current) return
    started.current = true
    const initData = new URLSearchParams(window.location.hash.slice(1)).get('tgWebAppData') ?? ''
    window.history.replaceState(null, '', window.location.pathname + window.location.search)
    if (!initData) {
      navigate('/login-required', { replace: true })
      return
    }
    fetch('/auth/tma', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ initData }),
    }).then(
      (res) => navigate(res.ok ? '/' : '/login-required', { replace: true }),
      () => navigate('/login-required', { replace: true }),
    )
  }, [navigate])
  return (
    <main className="flex min-h-dvh items-center justify-center bg-page-bg" aria-busy="true">
      <Spinner />
    </main>
  )
}
