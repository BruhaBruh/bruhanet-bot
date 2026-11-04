import type { ReactNode } from 'react'
import { Link, useNavigate } from 'react-router'

// Кликабельная строка-ячейка: ссылка (Enter) + Space, как у кнопки.
export function RowLink({ to, children, current }: { to: string; children: ReactNode; current?: boolean }) {
  const navigate = useNavigate()
  return (
    <Link
      to={to}
      className="row-link"
      aria-current={current ? 'page' : undefined}
      onKeyDown={(e) => {
        if (e.key === ' ') {
          e.preventDefault()
          navigate(to)
        }
      }}
    >
      {children}
    </Link>
  )
}
