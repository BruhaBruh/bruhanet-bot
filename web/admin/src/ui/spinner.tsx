import { cn } from '@/lib/utils'

export interface SpinnerProps {
  size?: number
  color?: string
  label?: string
  className?: string
}

export function Spinner({ size = 22, color, label = 'Загрузка', className }: SpinnerProps) {
  return (
    <span
      className={cn('inline-flex text-text-hint', className)}
      role="status"
      aria-label={label}
      style={{ width: size, height: size, color }}
    >
      <svg
        className="animate-[tg-spin_.8s_linear_infinite] motion-reduce:animate-[tg-spin_2.4s_linear_infinite]"
        viewBox="0 0 24 24"
        width={size}
        height={size}
      >
        <circle
          cx={12}
          cy={12}
          r={9.5}
          fill="none"
          stroke="currentColor"
          strokeWidth={2.4}
          strokeLinecap="round"
          strokeDasharray="44 60"
        />
      </svg>
    </span>
  )
}
