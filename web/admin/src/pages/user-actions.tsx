import { useRef, useState } from 'react'
import { Button, Dialog, Input, toast } from '@/ui'
import { ApiError, post } from '../api'
import { formatFullDay, plural } from '../format'
import { grantedExpire, MAX_DAYS, parseDays } from '../grant'
import { userLabel } from '../promo'
import type { UserSummary } from '../types'

const CHIPS = [7, 14, 30]

const daysLabel = (n: number) => `${n} ${plural(n, ['день', 'дня', 'дней'])}`

const failure = (e: unknown, fallback: string) => (e instanceof ApiError && e.body.message) || fallback

/** «Выдать дни»: число дней, быстрые значения, дата результата; один POST на двойной клик. */
export function GrantDialog({ user, expireAt, onClose, onDone }: { user: UserSummary; expireAt: string | null; onClose: () => void; onDone: () => void }) {
  const [text, setText] = useState('7')
  const [touched, setTouched] = useState(false)
  const sending = useRef(false)
  const days = parseDays(text)

  async function submit() {
    if (days === undefined) {
      setTouched(true)
      return
    }
    if (sending.current) return
    sending.current = true
    try {
      await post(`/api/users/${user.telegramId}/grant`, { days })
      toast(`Добавлено ${daysLabel(days)}`, { icon: 'check' })
      onClose()
      onDone()
    } catch (e) {
      toast(failure(e, 'Не удалось выдать дни'))
      sending.current = false
    }
  }

  return (
    <Dialog
      title="Выдать дни"
      message={`Пользователь ${userLabel(user)} получит уведомление.`}
      actions={[
        { label: 'Отмена', style: 'cancel', onClick: onClose },
        { label: 'Выдать', onClick: () => void submit() },
      ]}
    >
      <div className="mt-3 flex flex-col items-center gap-3">
        <Input
          label="Сколько дней"
          type="number"
          fit
          min={1}
          max={MAX_DAYS}
          value={text}
          onChange={(e) => setText(e.target.value)}
          error={touched && days === undefined ? `От 1 до ${MAX_DAYS}` : undefined}
          after="дн."
        />
        <div className="flex gap-2">
          {CHIPS.map((n) => (
            <Button key={n} size="sm" variant={days === n ? 'tinted' : 'gray'} onClick={() => setText(String(n))}>
              {n}
            </Button>
          ))}
        </div>
        <p className="footnote text-text-secondary">
          {days === undefined ? 'Укажите число дней' : `Подписка продлится до ${formatFullDay(grantedExpire(expireAt, days))}`}
        </p>
      </div>
    </Dialog>
  )
}

/** Блокировка и снятие: последствия в тексте, тост в прошедшем времени. */
export function BlockDialog({ user, onClose, onDone }: { user: UserSummary; onClose: () => void; onDone: () => void }) {
  const blocked = Boolean(user.blockedAt)
  const sending = useRef(false)
  const who = userLabel(user)

  async function submit() {
    if (sending.current) return
    sending.current = true
    try {
      await post(`/api/users/${user.telegramId}/${blocked ? 'unblock' : 'block'}`)
      toast(blocked ? 'Блокировка снята' : 'Пользователь заблокирован', { icon: 'check' })
      onClose()
      onDone()
    } catch (e) {
      toast(failure(e, blocked ? 'Не удалось снять блокировку' : 'Не удалось заблокировать'))
      sending.current = false
    }
  }

  return (
    <Dialog
      title={blocked ? 'Снять блокировку?' : 'Заблокировать?'}
      message={
        blocked
          ? `${who} снова сможет получать авансы и промокоды.`
          : `Заблокировать ${who}? Он не сможет получать авансы и промокоды. Обычная оплата останется доступна.`
      }
      actions={[
        { label: 'Отмена', style: 'cancel', onClick: onClose },
        { label: blocked ? 'Снять блокировку' : 'Заблокировать', style: blocked ? 'default' : 'destructive', onClick: () => void submit() },
      ]}
    />
  )
}
