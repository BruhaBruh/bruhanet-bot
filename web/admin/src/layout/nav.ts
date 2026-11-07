import type { NavItem } from '@/ui'

export type Section = NavItem & { short: string }

const section = (key: string, label: string, short: string, icon: NavItem['icon']): Section => ({ key, label, short, icon })

// Порядок и группы — из макета; key — путь раздела.
export const nav: Array<Section | { section: string }> = [
  section('/', 'Обзор', 'Обзор', 'chart'),
  { section: 'Маркетинг' },
  section('/broadcasts', 'Рассылки', 'Рассылки', 'megaphone'),
  section('/promos', 'Промокоды', 'Промо', 'ticket'),
  section('/gifts', 'Подарки', 'Подарки', 'gift'),
  { section: 'Бот' },
  section('/texts', 'Редактор сообщений', 'Тексты', 'message'),
  section('/map', 'Схема бота', 'Схема', 'map'),
  { section: 'Данные' },
  section('/users', 'Пользователи', 'Юзеры', 'users'),
  section('/payments', 'Платежи', 'Платежи', 'card'),
  { section: 'Система' },
  section('/settings', 'Настройки', 'Настройки', 'gear'),
]

export const sections = nav.filter((n) => !('section' in n)) as Section[]

export const sectionOf = (pathname: string) =>
  sections.find((s) => s.key !== '/' && (pathname === s.key || pathname.startsWith(`${s.key}/`))) ?? sections[0]

const pageTitles: Record<string, string> = {
  '/broadcasts/new': 'Новая рассылка',
  '/promos/new': 'Новый промокод',
}

export const pageTitle = (pathname: string) =>
  pageTitles[pathname] ?? (/^\/broadcasts\/[^/]+\/edit$/.test(pathname) ? 'Редактирование рассылки' : sectionOf(pathname).label)
