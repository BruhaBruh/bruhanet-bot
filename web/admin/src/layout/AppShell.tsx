import { Suspense, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router'
import { BOT_NAME, BOT_USERNAME } from '@/brand'
import { cn } from '@/lib/utils'
import { Avatar, Button, Sidebar, Spinner, TabBar, toast } from '@/ui'
import { get, post } from '../api'
import { Header } from './Header'
import { nav, pageTitle, sectionOf, sections } from './nav'
import type { ShellContext } from './page-title'

const COLLAPSED_KEY = 'bn_sidebar_collapsed'
// Без from сервер считает только последние 30 дней, а зависший платёж старше не должен пропасть из счётчика.
const PENDING_URL = '/api/payments?status=pending&from=2000-01-01'

export function AppShell() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem(COLLAPSED_KEY) === '1')
  const [pending, setPending] = useState(0)
  const [customTitle, setPageTitle] = useState<string>()
  const [syncKey, setSyncKey] = useState(0)
  const context: ShellContext = { setPageTitle, syncKey }
  const tabs = useRef<HTMLDivElement>(null)
  const active = sectionOf(pathname).key

  useEffect(() => {
    let stale = false
    get<{ total: number }>(PENDING_URL).then(
      (r) => !stale && setPending(r.total),
      () => !stale && setPending(0),
    )
    return () => {
      stale = true
    }
  }, [pathname])

  // Вкладок больше, чем влезает в 390px: активную подкручиваем в зону видимости.
  useEffect(() => {
    tabs.current?.querySelector('[aria-current="page"]')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
  }, [active])

  function toggleSidebar() {
    localStorage.setItem(COLLAPSED_KEY, collapsed ? '0' : '1')
    setCollapsed(!collapsed)
  }

  function logout() {
    post('/api/logout').then(
      () => navigate('/login-required'),
      () => toast('Не удалось выйти'),
    )
  }

  const withBadge = <T extends object>(item: T) =>
    'key' in item && item.key === '/payments' ? { ...item, badge: pending } : item

  return (
    <div className="flex min-h-screen items-stretch">
      <div
        className={cn(
          'sticky top-0 hidden h-screen flex-none flex-col overflow-y-auto bg-surface shadow-[inset_-0.5px_0_0_var(--separator)] transition-[width] duration-200 motion-reduce:transition-none md:flex',
          collapsed ? 'w-18' : 'w-(--size-sidebar)',
        )}
      >
        <div className={cn('flex items-center gap-2.5 pt-5 pb-2', collapsed ? 'justify-center' : 'px-5')}>
          <Avatar name={BOT_NAME} colorIndex={5} size={36} />
          {!collapsed && (
            <div className="flex min-w-0 flex-col">
              <span className="admin-body truncate font-semibold">{BOT_NAME}</span>
              <span className="admin-label truncate font-normal text-text-secondary">@{BOT_USERNAME}</span>
            </div>
          )}
        </div>
        <Sidebar
          className="w-full grow shadow-none"
          items={nav.map(withBadge)}
          value={active}
          onChange={(key) => navigate(key)}
          collapsed={collapsed}
          onToggle={toggleSidebar}
          footer={
            <Button
              variant="plain"
              block
              icon="logout"
              aria-label="Выйти"
              className={cn('px-0 text-text-secondary', !collapsed && 'justify-start')}
              onClick={logout}
            >
              {collapsed ? undefined : 'Выйти'}
            </Button>
          }
        />
      </div>

      <div className="flex min-w-0 flex-1 flex-col">
        <Header title={customTitle ?? pageTitle(pathname)} onLogout={logout} onSynced={() => setSyncKey((k) => k + 1)} />
        <main className="flex w-full max-w-320 grow flex-col gap-6 px-4 pt-4 pb-[calc(var(--size-tab-bar)+var(--space-6))] md:px-8 md:pt-6 md:pb-10">
          <Suspense fallback={<Spinner className="self-center p-8" />}>
            <Outlet context={context} />
          </Suspense>
        </main>
      </div>

      <div ref={tabs} className="fixed inset-x-0 bottom-0 z-40 md:hidden">
        <TabBar
          items={sections.map((s) => ({ ...withBadge(s), label: s.short }))}
          value={active}
          onChange={(key) => navigate(key)}
        />
      </div>
    </div>
  )
}
