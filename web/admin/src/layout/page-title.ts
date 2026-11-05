import { useEffect } from 'react'
import { useOutletContext } from 'react-router'

export interface ShellContext {
  setPageTitle: (title?: string) => void
}

/** Заголовок шапки вместо раздела из `nav.ts`: для экранов, чьё имя известно только из данных. */
export function usePageTitle(title?: string) {
  const { setPageTitle } = useOutletContext<ShellContext>()
  useEffect(() => {
    setPageTitle(title)
    return () => setPageTitle(undefined)
  }, [title, setPageTitle])
}
