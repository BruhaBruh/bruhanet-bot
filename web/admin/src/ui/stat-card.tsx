import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { IconTile, type IconName, type IconTileColor } from './icon'
import { StatusTag, type Tone } from './status-tag'

export interface StatCardProps {
  label: ReactNode
  value: ReactNode
  delta?: string
  deltaTone?: Tone
  hint?: ReactNode
  icon?: IconName
  iconColor?: IconTileColor
  className?: string
}

export function StatCard({ label, value, delta, deltaTone, hint, icon, iconColor, className }: StatCardProps) {
  const tone = deltaTone ?? (delta?.startsWith('-') ? 'destructive' : 'success')
  return (
    <div className={cn('flex min-w-45 flex-col gap-2 rounded-lg bg-surface p-4 text-text shadow-card', className)}>
      <div className="flex items-center gap-2">
        {icon && <IconTile icon={icon} color={iconColor} />}
        <span className="admin-label text-text-secondary">{label}</span>
      </div>
      <div className="admin-stat">{value}</div>
      {(delta || hint) && (
        <div className="flex items-center gap-2">
          {delta && <StatusTag tone={tone}>{delta}</StatusTag>}
          {hint && <span className="admin-label font-normal text-text-secondary">{hint}</span>}
        </div>
      )}
    </div>
  )
}
