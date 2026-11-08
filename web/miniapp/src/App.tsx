import { useEffect, useState, type ComponentType } from 'react'
import { TabBar, type NavItem } from '@/ui/tab-bar'
import { ApiError } from './lib/api'
import { useQuery } from './lib/query'
import { closeApp, ready } from './lib/telegram'
import { AdminButton } from './screens/AdminButton'
import Devices from './screens/Devices'
import Friends from './screens/Friends'
import Promos from './screens/Promos'
import Subscription from './screens/Subscription'
import { Loading, Notice } from './screens/states'

export interface Me {
  name: string
  username: string
  botUsername: string
  blocked: boolean
  isAdmin: boolean
}

type Tab = 'subscription' | 'devices' | 'friends' | 'promos'

const TABS: (NavItem & { key: Tab })[] = [
  { key: 'subscription', label: 'Подписка', icon: 'card' },
  { key: 'devices', label: 'Устройства', icon: 'phone' },
  { key: 'friends', label: 'Друзья', icon: 'users' },
  { key: 'promos', label: 'Промокоды', icon: 'ticket' },
]

// экраны без нужды в me принимают меньше параметров — это допустимо
const SCREENS: Record<Tab, ComponentType<{ me: Me }>> = { subscription: Subscription, devices: Devices, friends: Friends, promos: Promos }

function failure(e: unknown): 'expired' | 'notRegistered' | 'error' {
  if (e instanceof ApiError && e.status === 401) return 'expired'
  if (e instanceof ApiError && e.status === 403 && e.code === 'not_registered') return 'notRegistered'
  return 'error'
}

export function App() {
  // me и referral кэшируются: вкладки открываются сразу, данные обновляются в фоне
  const meQ = useQuery<Me>('/m/api/me')
  const refQ = useQuery<{ enabled: boolean }>('/m/api/referral')
  const [rawTab, setRawTab] = useState<Tab>(() => (new URLSearchParams(window.location.search).get('tab') as Tab | null) ?? 'subscription')

  useEffect(() => {
    ready()
  }, [])

  const choose = (key: string) => {
    setRawTab(key as Tab)
    window.history.replaceState(null, '', `?tab=${key}`)
  }
  const retry = () => {
    void meQ.refetch()
    void refQ.refetch()
  }

  // рефералка не блокирует кабинет: сбой её запроса = вкладки «Друзья» нет
  const me = meQ.data
  const referralOn = refQ.data?.enabled ?? false
  const error = meQ.error ?? (me ? undefined : refQ.error)
  if (!me && !error) return <Loading />
  if (!me || refQ.isLoading) {
    if (error) {
      const kind = failure(error)
      if (kind === 'expired') {
        return (
          <Notice title="Сессия закончилась" action={{ label: 'Закрыть', onClick: closeApp }}>
            Открой кабинет заново из бота.
          </Notice>
        )
      }
      if (kind === 'notRegistered') {
        return (
          <Notice title="Кабинет ещё недоступен" action={{ label: 'Закрыть', onClick: closeApp }}>
            Сначала напиши боту /start, а потом открой кабинет снова.
          </Notice>
        )
      }
      return (
        <Notice title="Не получилось загрузить" action={{ label: 'Повторить', onClick: retry }}>
          Проверь соединение и попробуй ещё раз.
        </Notice>
      )
    }
    return <Loading />
  }

  const items = TABS.filter((t) => t.key !== 'friends' || referralOn)
  const tab: Tab = items.some((t) => t.key === rawTab) ? rawTab : 'subscription'
  const Screen = SCREENS[tab]
  return (
    <div className="min-h-dvh pb-[calc(var(--size-tab-bar)+env(safe-area-inset-bottom))]">
      {me.isAdmin && <AdminButton />}
      <Screen me={me} />
      <div className="fixed inset-x-0 bottom-0 bg-bottom-bar-bg pb-[env(safe-area-inset-bottom)]">
        <TabBar items={items} value={tab} onChange={choose} />
      </div>
    </div>
  )
}
