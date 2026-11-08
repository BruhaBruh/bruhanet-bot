import { useCallback, useEffect, useState, type ComponentType } from 'react'
import { TabBar, type NavItem } from '@/ui/tab-bar'
import { ApiError, get } from './lib/api'
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

type Boot =
  | { status: 'loading' }
  | { status: 'ready'; me: Me; referralOn: boolean }
  | { status: 'expired' | 'notRegistered' | 'error' }

function failure(e: unknown): Boot {
  if (e instanceof ApiError && e.status === 401) return { status: 'expired' }
  if (e instanceof ApiError && e.status === 403 && e.code === 'not_registered') return { status: 'notRegistered' }
  return { status: 'error' }
}

function tabFromUrl(allowed: Tab[]): Tab {
  const t = new URLSearchParams(window.location.search).get('tab') as Tab | null
  return t && allowed.includes(t) ? t : 'subscription'
}

export function App() {
  const [boot, setBoot] = useState<Boot>({ status: 'loading' })
  const [tab, setTab] = useState<Tab>('subscription')

  const load = useCallback(() => {
    Promise.all([get<Me>('/m/api/me'), get<{ enabled: boolean }>('/m/api/referral')]).then(
      ([me, ref]) => {
        setBoot({ status: 'ready', me, referralOn: ref.enabled })
        setTab(tabFromUrl(ref.enabled ? TABS.map((t) => t.key) : TABS.map((t) => t.key).filter((k) => k !== 'friends')))
      },
      (e: unknown) => setBoot(failure(e)),
    )
  }, [])

  useEffect(() => {
    ready()
    load()
  }, [load])

  const retry = () => {
    setBoot({ status: 'loading' })
    load()
  }

  const choose = (key: string) => {
    setTab(key as Tab)
    window.history.replaceState(null, '', `?tab=${key}`)
  }

  switch (boot.status) {
    case 'loading':
      return <Loading />
    case 'expired':
      return (
        <Notice title="Сессия закончилась" action={{ label: 'Закрыть', onClick: closeApp }}>
          Открой кабинет заново из бота.
        </Notice>
      )
    case 'notRegistered':
      return (
        <Notice title="Кабинет ещё недоступен" action={{ label: 'Закрыть', onClick: closeApp }}>
          Сначала напиши боту /start, а потом открой кабинет снова.
        </Notice>
      )
    case 'error':
      return (
        <Notice title="Не получилось загрузить" action={{ label: 'Повторить', onClick: retry }}>
          Проверь соединение и попробуй ещё раз.
        </Notice>
      )
  }

  const items = TABS.filter((t) => t.key !== 'friends' || boot.referralOn)
  const Screen = SCREENS[tab]
  return (
    <div className="min-h-dvh pb-[calc(var(--size-tab-bar)+env(safe-area-inset-bottom))]">
      {boot.me.isAdmin && <AdminButton />}
      <Screen me={boot.me} />
      <div className="fixed inset-x-0 bottom-0 bg-bottom-bar-bg pb-[env(safe-area-inset-bottom)]">
        <TabBar items={items} value={tab} onChange={choose} />
      </div>
    </div>
  )
}
