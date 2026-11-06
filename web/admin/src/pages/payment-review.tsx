import { useRef, useState, type ReactNode } from 'react'
import { Button, Dialog, toast } from '@/ui'
import { ApiError, post } from '../api'
import { formatFullDay } from '../format'
import { useRefreshPending } from '../layout/page-title'
import { userLabel } from '../promo'
import type { Payment } from '../types'
import { PaymentReceipt } from './PaymentReceipt'
import { payActions } from './payment-columns'

const MONTH_MS = 30 * 864e5

// Для диалога, дата может отстать от сервера на время до клика. Формула — payflow.ExtendedExpire.
export const extendsTo = (p: Payment, now = Date.now()) =>
  new Date(Math.max(p.prevExpireAt ? Date.parse(p.prevExpireAt) : 0, now) + p.month * MONTH_MS).toISOString()

function consequence(p: Payment, approve: boolean) {
  const who = userLabel(p)
  if (approve) return `Подписка ${who} продлится до ${formatFullDay(extendsTo(p))}. Пользователь получит уведомление.`
  const back = p.prevExpireAt ? `Срок подписки ${who} вернётся к ${formatFullDay(p.prevExpireAt)}. ` : ''
  return `${back}Пользователь получит уведомление.`
}

/** Действия с платежом: кнопки решения, чек, диалоги и раскрытие строки на телефоне (там в строке нет кнопок). */
export function usePaymentReview(onChanged: () => void, openUser?: (p: Payment) => void) {
  const refreshPending = useRefreshPending()
  const [deciding, setDeciding] = useState<{ payment: Payment; approve: boolean }>()
  const [receipt, setReceipt] = useState<Payment>()
  const [openId, setOpenId] = useState<Payment['id']>()
  const sending = useRef(false)

  async function resolve(p: Payment, approve: boolean) {
    setDeciding(undefined)
    if (sending.current) return
    sending.current = true
    try {
      await post(`/api/payments/${p.id}/${approve ? 'confirm' : 'decline'}`)
      toast(approve ? 'Платёж подтверждён' : 'Платёж отклонён', { icon: 'check' })
    } catch (e) {
      toast((e instanceof ApiError && e.body.message) || 'Не удалось обновить платёж')
    } finally {
      sending.current = false
      // и после отказа: «Уже обработано» значит, что строка на экране устарела
      onChanged()
      refreshPending()
    }
  }

  const buttons = (p: Payment): ReactNode => (
    <>
      {p.hasPhoto ? (
        <Button variant="gray" size="sm" onClick={() => setReceipt(p)}>
          Чек
        </Button>
      ) : (
        p.submittedAt && <span className="footnote text-text-secondary">Чек в боте</span>
      )}
      {p.status === 'pending' && (
        <>
          <Button size="sm" onClick={() => setDeciding({ payment: p, approve: true })}>
            Подтвердить
          </Button>
          <Button variant="destructive" size="sm" onClick={() => setDeciding({ payment: p, approve: false })}>
            Отклонить
          </Button>
        </>
      )}
    </>
  )

  return {
    rowActions: (p: Payment) => (
      // opacity-100!: кнопки решения не приглушаются, как значки в ячейке действий
      <span className="inline-flex items-center justify-end gap-1.5 opacity-100!">
        {buttons(p)}
        {payActions(p)}
      </span>
    ),
    expandedRow: (p: Payment) =>
      openId !== p.id ? null : (
        <div className="flex flex-wrap items-center gap-2 bg-[color-mix(in_srgb,var(--page-bg)_55%,var(--surface))] px-5 py-3 whitespace-normal">
          {buttons(p)}
          {openUser && (
            <Button variant="plain" size="sm" onClick={() => openUser(p)}>
              Открыть пользователя
            </Button>
          )}
        </div>
      ),
    toggle: (p: Payment) => setOpenId(openId === p.id ? undefined : p.id),
    dialogs: (
      <>
        {deciding && (
          <Dialog
            title={deciding.approve ? 'Подтвердить платёж?' : 'Отклонить платёж?'}
            message={consequence(deciding.payment, deciding.approve)}
            actions={[
              { label: 'Отмена', style: 'cancel', onClick: () => setDeciding(undefined) },
              {
                label: deciding.approve ? 'Подтвердить' : 'Отклонить',
                style: deciding.approve ? 'default' : 'destructive',
                onClick: () => void resolve(deciding.payment, deciding.approve),
              },
            ]}
          />
        )}
        {receipt && <PaymentReceipt payment={receipt} onClose={() => setReceipt(undefined)} />}
      </>
    ),
  }
}
