import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router'
import { Avatar, Button, Icon, MessageBubble, Spinner, toast } from '@/ui'
import { buttonVariants } from '@/ui/primitives/button'
import { cn } from '@/lib/utils'
import { NO_BOT, parseBot, type BotInfo } from '../me'

const POLL_MS = 3000
const COPIED_MS = 2500
const linkClass = 'focus-ring text-link underline hover:text-accent-strong'
const SAMPLE_KEYBOARD = [[{ text: 'Войти в админ-панель', url: '#' }]]

function Step({ n, children }: { n: number; children: ReactNode }) {
  return (
    <li className="flex items-start gap-3">
      <span className="admin-table flex size-7 flex-none items-center justify-center rounded-full bg-accent-tint font-semibold text-accent-strong">
        {n}
      </span>
      {children}
    </li>
  )
}

export default function LoginRequired() {
  const navigate = useNavigate()
  const [copied, setCopied] = useState(false)
  const [bot, setBot] = useState<BotInfo>(NO_BOT)
  const botUrl = bot.username ? `https://t.me/${bot.username}` : ''
  const copiedTimer = useRef<ReturnType<typeof setTimeout>>(undefined)

  // Не через api.ts: его 401 — глобальный переход сюда же. Таймаут после ответа, чтобы запросы не копились.
  useEffect(() => {
    let stopped = false
    let timer: ReturnType<typeof setTimeout>
    const poll = async () => {
      const ok = await fetch('/api/me', { credentials: 'same-origin' }).then(
        (res) => res.ok,
        () => false,
      )
      if (stopped) return
      if (ok) navigate('/', { replace: true })
      else timer = setTimeout(poll, POLL_MS)
    }
    timer = setTimeout(poll, POLL_MS)
    return () => {
      stopped = true
      clearTimeout(timer)
    }
  }, [navigate])

  useEffect(() => () => clearTimeout(copiedTimer.current), [])

  // Публичный ответ без сессии: только ник и имя бота. Нет ответа — ссылки на бота не показываем.
  useEffect(() => {
    let stale = false
    fetch('/auth/bot', { credentials: 'same-origin' })
      .then((res) => (res.ok ? res.json() : null))
      .then((r) => !stale && setBot(parseBot(r)))
      .catch(() => {})
    return () => {
      stale = true
    }
  }, [])

  async function copy() {
    try {
      await navigator.clipboard.writeText('/login')
    } catch {
      toast('Не удалось скопировать')
      return
    }
    setCopied(true)
    toast('Команда /login скопирована', { icon: 'copy' })
    clearTimeout(copiedTimer.current)
    copiedTimer.current = setTimeout(() => setCopied(false), COPIED_MS)
  }

  return (
    <div className="admin-body flex min-h-screen items-center justify-center bg-page-bg px-5 py-10 font-sans tracking-[-0.2px] text-text antialiased">
      <div className="flex w-full max-w-220 flex-wrap items-stretch gap-6">
        <section className="flex min-w-0 grow basis-95 flex-col gap-6 rounded-lg bg-surface p-8 shadow-card">
          <div className="flex flex-col gap-4">
            <div aria-hidden="true" className="flex size-14 items-center justify-center rounded-full bg-brand text-white">
              <Icon name="send" size={28} strokeWidth={2} />
            </div>
            <div className="flex flex-col gap-1.5">
              <h1 className="admin-stat">Войдите через Telegram</h1>
              <p className="text-text-secondary">
                Пароль не нужен: вход подтверждается в нашем боте. Это займёт меньше минуты.
              </p>
            </div>
          </div>

          <ol className="flex flex-col gap-4">
            <Step n={1}>
              <div className="flex flex-col gap-0.5 pt-1">
                <span className="font-semibold">
                  Откройте бота
                  {botUrl && (
                    <>
                      {' '}
                      <a href={botUrl} target="_blank" rel="noreferrer" className={linkClass}>
                        @{bot.username}
                      </a>
                    </>
                  )}
                </span>
                {botUrl && <span className="footnote text-text-secondary">Кнопка ниже откроет чат с ботом в Telegram.</span>}
              </div>
            </Step>
            <Step n={2}>
              <div className="flex grow flex-col gap-2 pt-1">
                <span className="font-semibold">Отправьте команду</span>
                <div className="flex items-center gap-2 rounded-md bg-fill-tertiary py-1.5 pr-1.5 pl-3.5">
                  <code className="headline grow font-mono tracking-normal text-text">/login</code>
                  <Button size="sm" variant="tinted" icon={copied ? 'check' : 'copy'} onClick={copy}>
                    {copied ? 'Скопировано' : 'Скопировать'}
                  </Button>
                </div>
              </div>
            </Step>
            <Step n={3}>
              <div className="flex flex-col gap-0.5 pt-1">
                <span className="font-semibold">Нажмите «Войти в админ-панель»</span>
                <span className="footnote text-text-secondary">
                  Бот пришлёт кнопку — после нажатия эта страница откроется уже авторизованной.
                </span>
              </div>
            </Step>
          </ol>

          <div className="mt-auto flex flex-col gap-3">
            {botUrl && (
              <a
                href={`${botUrl}?start=login`}
                target="_blank"
                rel="noreferrer"
                className={cn(buttonVariants({ variant: 'filled', size: 'lg', block: true }), 'no-underline')}
              >
                Открыть бота в Telegram
              </a>
            )}
            <div role="status" className="footnote flex items-center justify-center gap-2 text-text-secondary">
              <Spinner size={16} />
              <span>Ждём подтверждения из Telegram…</span>
            </div>
          </div>
        </section>

        <aside
          aria-label="Пример ответа бота"
          className="flex min-w-0 grow basis-80 flex-col overflow-hidden rounded-lg bg-chat-bg"
        >
          <div className="flex items-center gap-2.5 bg-header-bg px-4 py-2.5 shadow-[inset_0_-0.5px_0_var(--separator)]">
            <Avatar name={bot.name || 'Бот'} colorIndex={5} size={36} />
            <div className="flex flex-col">
              <span className="font-semibold">{bot.name || 'Бот'}</span>
              <span className="admin-label font-normal text-text-secondary">бот</span>
            </div>
          </div>
          <div className="flex grow flex-col justify-end gap-2 p-4">
            <MessageBubble out time="14:30">
              /login
            </MessageBubble>
            <MessageBubble time="14:30" keyboard={SAMPLE_KEYBOARD}>
              Привет! Вы администратор{bot.name ? ` ${bot.name}` : ''}. Нажмите кнопку ниже, чтобы войти в админ-панель.
            </MessageBubble>
          </div>
          <p className="footnote px-4 pb-4 text-center text-text opacity-80">Так выглядит ответ бота</p>
        </aside>

        <p className="footnote grow basis-full text-center text-section-header">
          Бот не отвечает или пишет «нет доступа»? Обратитесь к владельцу бота, чтобы он добавил ваш аккаунт в администраторы.
        </p>
      </div>
    </div>
  )
}
