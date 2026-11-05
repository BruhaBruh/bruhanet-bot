import { expect, it } from 'vitest'
import { cn } from '@/lib/utils'

it('cn: последний класс группы побеждает, токенные цвета не путаются с размером', () => {
  expect(cn('p-2', false, 'p-4')).toBe('p-4')
  expect(cn('text-text', 'admin-label', 'text-text-secondary')).toBe('admin-label text-text-secondary')
})

it('cn: знает токенные радиусы, тени и типографику', () => {
  expect(cn('rounded-md', 'rounded-sheet')).toBe('rounded-sheet')
  expect(cn('rounded-bubble', 'rounded-md')).toBe('rounded-md')
  expect(cn('shadow-card', 'shadow-popover')).toBe('shadow-popover')
  expect(cn('admin-label', 'subheadline', 'font-semibold')).toBe('subheadline font-semibold')
})
