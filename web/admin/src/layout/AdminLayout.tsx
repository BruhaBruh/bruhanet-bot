import { Avatar, Button, Cell, List } from '@telegram-apps/telegram-ui'
import { useEffect, useRef } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router'
import { post } from '../api'

export const sections = [
  { to: '/broadcasts', emoji: '📣', label: 'Рассылки', sub: 'отправка всем', color: 1 },
  { to: '/promos', emoji: '🎟', label: 'Промокоды', sub: 'скидки и бонусы', color: 2 },
  { to: '/users', emoji: '👤', label: 'Юзеры', sub: 'поиск и карточки', color: 3 },
  { to: '/payments', emoji: '💳', label: 'Платежи', sub: 'история и выручка', color: 4 },
  { to: '/texts', emoji: '✏️', label: 'Тексты', sub: 'сообщения бота', color: 5 },
  { to: '/settings', emoji: '⚙️', label: 'Настройки', sub: 'тарифы и реквизиты', color: 6 },
] as const

export function AdminLayout() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const nav = useRef<HTMLElement>(null)

  // На телефоне меню — горизонтальная лента: подкручиваем активный пункт в зону видимости.
  useEffect(() => {
    nav.current?.querySelector('[aria-current="page"]')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
  }, [pathname])

  async function logout() {
    await post('/api/logout')
    navigate('/login-required')
  }

  return (
    <div className="layout">
      <nav ref={nav} className="nav" aria-label="Разделы">
        <div className="brand">BruhaNet</div>
        <List className="nav-list">
          {sections.map((s) => (
            // Cell не знает про `to` — ссылка снаружи, aria-current ставит NavLink
            <NavLink key={s.to} to={s.to}>
              <Cell
                subtitle={<span className="nav-sub">{s.sub}</span>}
                before={<Avatar size={40} acronym={s.emoji} className={`av av-${s.color}`} aria-hidden="true" />}
              >
                {s.label}
              </Cell>
            </NavLink>
          ))}
        </List>
        <Button className="logout" mode="plain" onClick={logout}>
          Выйти
        </Button>
      </nav>
      <main className="main">
        <div className="col">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
