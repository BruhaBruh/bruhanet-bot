import { useRef, useState } from 'react'
import { Button } from '@/ui/primitives/button'
import { ApiError, post } from '~/lib/api'
import { openAdmin } from '~/lib/telegram'

// Только админу (me.isAdmin): сервер выдаёт одноразовую ссылку входа, админка открывается уже залогиненной.
export function AdminButton() {
  const [error, setError] = useState('')
  const busy = useRef(false)

  const open = async () => {
    if (busy.current) return
    busy.current = true
    setError('')
    try {
      openAdmin((await post<{ url: string }>('/m/api/admin/login-link')).url)
    } catch (e) {
      setError(e instanceof ApiError && e.status === 401 ? 'Открой кабинет заново из бота' : 'Не получилось открыть админку')
    } finally {
      busy.current = false
    }
  }

  return (
    <div className="flex flex-col items-end gap-1 px-4 pt-3">
      <Button size="sm" variant="tinted" onClick={() => void open()}>
        Админка
      </Button>
      {error && <p className="footnote text-destructive-strong">{error}</p>}
    </div>
  )
}
