import type { CSSProperties } from 'react'
import { cn } from '@/lib/utils'

const PATHS = {
  'chevron-right': ['M9 6l6 6-6 6'],
  'chevron-left': ['M15 5l-7 7 7 7'],
  'chevron-down': ['M6 9l6 6 6-6'],
  check: ['M5 12.5l4.5 4.5L19 7'],
  search: ['M10.5 4a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13z', 'M15.5 15.5L20 20'],
  plus: ['M12 5v14', 'M5 12h14'],
  close: ['M6 6l12 12', 'M18 6L6 18'],
  more: ['M6 12h.01', 'M12 12h.01', 'M18 12h.01'],
  users: [
    'M9 4.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7z',
    'M2.5 19.5c0-3.3 2.9-5.5 6.5-5.5s6.5 2.2 6.5 5.5',
    'M16 4.6a3.5 3.5 0 0 1 0 6.8',
    'M18 14.2c2 .7 3.5 2.4 3.5 5.3',
  ],
  megaphone: ['M3.5 10v4a1 1 0 0 0 1 1H7l8 4.5v-15L7 9H4.5a1 1 0 0 0-1 1z', 'M18.5 9a4 4 0 0 1 0 6'],
  ticket: ['M3 7h18v3a2 2 0 0 0 0 4v3H3v-3a2 2 0 0 0 0-4z', 'M14.5 7.5v1.5', 'M14.5 11.25v1.5', 'M14.5 15v1.5'],
  card: ['M4 5.5h16a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-11a1 1 0 0 1 1-1z', 'M3 10h18', 'M7 15h4'],
  gear: [
    'M19.07 9.81 L21.41 10.09 L21.41 13.91 L19.07 14.19 L18.55 15.45 L20.00 17.30 L17.30 20.00 L15.45 18.55 L14.19 19.07 L13.91 21.41 L10.09 21.41 L9.81 19.07 L8.55 18.55 L6.70 20.00 L4.00 17.30 L5.45 15.45 L4.93 14.19 L2.59 13.91 L2.59 10.09 L4.93 9.81 L5.45 8.55 L4.00 6.70 L6.70 4.00 L8.55 5.45 L9.81 4.93 L10.09 2.59 L13.91 2.59 L14.19 4.93 L15.45 5.45 L17.30 4.00 L20.00 6.70 L18.55 8.55z',
    'M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z',
  ],
  message: ['M4 5h16v11H10l-6 4.5z'],
  chart: ['M5 20v-8', 'M12 20V5', 'M19 20v-5'],
  home: ['M4 11l8-7 8 7v9h-5.5v-6h-5v6H4z'],
  bell: ['M6 16V11a6 6 0 0 1 12 0v5l1.5 2h-15z', 'M10 20.5a2 2 0 0 0 4 0'],
  link: ['M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1', 'M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1'],
  trash: ['M5 7h14', 'M10 4h4', 'M7 7l1 13h8l1-13'],
  edit: ['M4 20h4l10.5-10.5a2.8 2.8 0 0 0-4-4L4 16z'],
  copy: ['M9 9h11v11H9z', 'M5 15H4V4h11v1'],
  send: ['M4 12l16-8-6 16-2.5-6.5z'],
  calendar: ['M4 6h16v14H4z', 'M4 10h16', 'M8 3v4', 'M16 3v4'],
  filter: ['M4 5h16l-6 7.5V19l-4-2v-4.5z'],
  moon: ['M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z'],
  sun: [
    'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z',
    'M12 2.5v2',
    'M12 19.5v2',
    'M2.5 12h2',
    'M19.5 12h2',
    'M5.3 5.3l1.4 1.4',
    'M17.3 17.3l1.4 1.4',
    'M5.3 18.7l1.4-1.4',
    'M17.3 6.7l1.4-1.4',
  ],
  user: ['M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8z', 'M4.5 20c0-3.6 3.4-6 7.5-6s7.5 2.4 7.5 6'],
  phone: ['M8 3h8a1 1 0 0 1 1 1v16a1 1 0 0 1-1 1H8a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z', 'M11 18h2'],
  gift: ['M4 10h16v10H4z', 'M3 7h18v3H3z', 'M12 7v13', 'M12 7C10 3 6 4 7.5 6.5', 'M12 7c2-4 6-3 4.5-0.5'],
  percent: ['M19 5L5 19', 'M7 5a2 2 0 1 0 0 4 2 2 0 0 0 0-4z', 'M17 15a2 2 0 1 0 0 4 2 2 0 0 0 0-4z'],
  ruble: ['M9 20V4h5a4 4 0 0 1 0 8H7', 'M7 16h7'],
  code: ['M8 7l-5 5 5 5', 'M16 7l5 5-5 5'],
  braces: [
    'M8 4C6 4 6 5 6 7v2c0 1.2-.8 2.2-2 3 1.2.8 2 1.8 2 3v2c0 2 0 3 2 3',
    'M16 4c2 0 2 1 2 3v2c0 1.2.8 2.2 2 3-1.2.8-2 1.8-2 3v2c0 2 0 3-2 3',
  ],
  block: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z', 'M5.6 5.6l12.8 12.8'],
  clock: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z', 'M12 7v5l3 2'],
  refresh: ['M20 11a8 8 0 0 0-14.5-4.5L4 8', 'M4 4v4h4', 'M4 13a8 8 0 0 0 14.5 4.5L20 16', 'M20 20v-4h-4'],
  sidebar: ['M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1z', 'M9 5v14'],
  // В наборе DS нет: нужна для «Выйти» в свёрнутом сайдбаре и на телефоне.
  logout: ['M10 4H6a1 1 0 0 0-1 1v14a1 1 0 0 0 1 1h4', 'M14 8l4 4-4 4', 'M18 12h-8'],
  'arrow-up': ['M12 19V5', 'M6 11l6-6 6 6'],
  'arrow-down': ['M12 5v14', 'M6 13l6 6 6-6'],
  lock: ['M6 11h12a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1z', 'M8 11V8a4 4 0 0 1 8 0v3'],
  support: ['M4 13v-1a8 8 0 0 1 16 0v1', 'M4 13h3v6H5a1 1 0 0 1-1-1z', 'M20 13h-3v6h2a1 1 0 0 0 1-1z'],
} as const

export type IconName = keyof typeof PATHS
export type IconTileColor = 'blue' | 'green' | 'orange' | 'red' | 'purple' | 'gray'

export interface IconProps {
  name: IconName
  size?: number
  strokeWidth?: number
  label?: string
  className?: string
  style?: CSSProperties
}

export function Icon({ name, size = 24, strokeWidth, label, className, style }: IconProps) {
  return (
    <svg
      className={cn('inline-block flex-none align-middle', className)}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth ?? (name === 'more' ? 3 : 1.8)}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden={label ? undefined : true}
      role={label ? 'img' : undefined}
      aria-label={label}
      style={style}
    >
      {PATHS[name].map((d) => (
        <path key={d} d={d} />
      ))}
    </svg>
  )
}

export interface IconTileProps {
  icon: IconName
  color?: IconTileColor
}

export function IconTile({ icon, color = 'blue' }: IconTileProps) {
  return (
    <span
      className="inline-flex size-(--size-icon-tile) flex-none items-center justify-center rounded-[7px] text-white"
      style={{ background: `var(--icon-${color})` }}
    >
      <Icon name={icon} size={19} strokeWidth={2} />
    </span>
  )
}
