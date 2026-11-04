import { Cell, Navigation, Placeholder, Section } from '@telegram-apps/telegram-ui'
import { formatDate, formatRub, statusLabels } from '../format'
import { RowLink } from '../layout/RowLink'
import type { Payment } from '../types'

export function PaymentsTable({ items, showUser, header }: { items: Payment[]; showUser?: boolean; header?: string }) {
  return (
    <Section header={header}>
      {!items.length ? (
        <Placeholder header="Платежей нет">
          <span className="ph-emoji">💳</span>
        </Placeholder>
      ) : (
        items.map((p) => {
          const cell = (
            <Cell
              key={p.id}
              className="keep-after"
              subtitle={`${formatDate(p.createdAt)} · ${statusLabels[p.status] ?? p.status}`}
              after={showUser ? <Navigation>{formatRub(p.price)}</Navigation> : formatRub(p.price)}
            >
              {(showUser ? `${p.username ? '@' + p.username : p.telegramId} · ` : '') + `${p.month} мес`}
            </Cell>
          )
          return showUser ? (
            <RowLink key={p.id} to={`/users/${p.telegramId}`}>
              {cell}
            </RowLink>
          ) : (
            cell
          )
        })
      )}
    </Section>
  )
}
