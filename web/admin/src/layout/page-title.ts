import { useEffect } from 'react'
import { useOutletContext } from 'react-router'

export interface ShellContext {
  setPageTitle: (title?: string) => void
  syncKey: number
  refreshPending: () => void
}

/** Растёт после синхронизации с панелью: экраны с данными юзеров кладут его в зависимости загрузки. */
export const useSyncKey = () => useOutletContext<ShellContext | null>()?.syncKey ?? 0

/** Перечитать бейдж платежей на проверке: после решения по платежу. */
export const useRefreshPending = () => useOutletContext<ShellContext | null>()?.refreshPending ?? (() => {})

/** Заголовок шапки вместо раздела из `nav.ts`: для экранов, чьё имя известно только из данных. */
export function usePageTitle(title?: string) {
  const { setPageTitle } = useOutletContext<ShellContext>()
  useEffect(() => {
    setPageTitle(title)
    return () => setPageTitle(undefined)
  }, [title, setPageTitle])
}
