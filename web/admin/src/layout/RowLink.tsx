import type { ReactNode } from 'react'
import { Link, useNavigate } from 'react-router'

// Кликабельная строка-ячейка: ссылка (Enter) + Space, как у кнопки.
export function RowLink({ to, children }: { to: string; children: ReactNode }) {
  const navigate = useNavigate()
  return (
    <Link
      to={to}
      className="row-link"
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
