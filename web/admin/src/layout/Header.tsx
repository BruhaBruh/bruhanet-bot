import type { FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { OWNER_NAME, OWNER_USERNAME } from '@/brand'
import { Avatar, Button, SearchField } from '@/ui'
import { useTheme } from '../theme'
import { SyncButton } from './SyncButton'

export function Header({ title, onLogout, onSynced }: { title: string; onLogout: () => void; onSynced: () => void }) {
  const { theme, toggle } = useTheme()
  const navigate = useNavigate()
  const themeLabel = theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'

  function search(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const q = String(new FormData(e.currentTarget).get('q') ?? '').trim()
    navigate(q ? `/users?q=${encodeURIComponent(q)}` : '/users')
  }

  return (
    <header className="flex flex-wrap items-center gap-3 bg-header-bg px-4 py-3 shadow-[inset_0_-0.5px_0_var(--separator)] md:px-8 md:py-4">
      <h1 className="title-3 md:admin-page-title m-0 min-w-0 flex-1 truncate font-bold">{title}</h1>
      {/* Поиск — своей строкой, пока не влезает рядом с заголовком. */}
      <form role="search" className="order-last w-full lg:order-none lg:w-70" onSubmit={search}>
        <SearchField name="q" placeholder="Поиск по пользователям и платежам" />
      </form>
      <Button variant="gray" icon={theme === 'dark' ? 'sun' : 'moon'} aria-label={themeLabel} title={themeLabel} onClick={toggle} />
      <SyncButton onSynced={onSynced} />
      {/* На телефоне аватар уступает место заголовку. */}
      <div className="hidden items-center gap-2.5 pl-1 md:flex">
        <Avatar name={OWNER_NAME} colorIndex={2} size={32} />
        <div className="flex flex-col">
          <span className="admin-table font-semibold">@{OWNER_USERNAME}</span>
          <span className="caption-1 text-text-secondary">Владелец</span>
        </div>
      </div>
      {/* На телефоне сайдбара нет, а выйти надо. */}
      <Button variant="gray" icon="logout" aria-label="Выйти" title="Выйти" className="md:hidden" onClick={onLogout} />
    </header>
  )
}
