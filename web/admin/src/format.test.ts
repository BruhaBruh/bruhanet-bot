import { expect, test } from 'vitest'
import { formatDate, formatRub } from './format'

test('formatRub uses ru-RU grouping', () => {
  expect(formatRub(2350)).toBe('2\u00a0350\u00a0₽')
})

test('formatDate renders Moscow time', () => {
  expect(formatDate('2026-09-12T09:30:00Z')).toBe('12.09.2026 12:30')
})
