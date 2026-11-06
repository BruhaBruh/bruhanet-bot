import { useState } from 'react'
import { Sheet } from '@/ui'
import { userLabel } from '../promo'
import type { Payment } from '../types'

export function PaymentReceipt({ payment, onClose }: { payment: Payment; onClose: () => void }) {
  const [failed, setFailed] = useState(false)
  return (
    <Sheet title={`Чек платежа ${payment.id}`} onClose={onClose}>
      <div className="px-4">
        {failed ? (
          <p role="alert" className="admin-body text-center text-destructive-strong">
            Не удалось загрузить чек, он остался в боте
          </p>
        ) : (
          <img
            src={`/api/payments/${payment.id}/photo`}
            alt={`Чек платежа ${payment.id} от ${userLabel(payment)}`}
            className="max-h-[70dvh] w-full rounded-md object-contain"
            onError={() => setFailed(true)}
          />
        )}
      </div>
    </Sheet>
  )
}
