import { expect, it } from 'vitest'
import { cn } from '@/lib/utils'

it('cn: последний класс группы побеждает, токенные цвета не путаются с размером', () => {
  expect(cn('p-2', false, 'p-4')).toBe('p-4')
  expect(cn('text-text', 'admin-label', 'text-text-secondary')).toBe('admin-label text-text-secondary')
})
