import { describe, expect, it } from 'vitest'
import { days, formatBytes, formatDate, plural } from './format'

describe('format', () => {
  it('pluralizes russian words', () => {
    expect([1, 2, 5, 11, 12, 13, 14, 21, 22, 25, 111].map(days)).toEqual([
      '1 день', '2 дня', '5 дней', '11 дней', '12 дней', '13 дней', '14 дней', '21 день', '22 дня', '25 дней', '111 дней',
    ])
    expect(plural(0, ['а', 'б', 'в'])).toBe('в')
  })

  it('formats bytes', () => {
    expect(formatBytes(10 * 1024 ** 3)).toBe('10 ГБ')
    expect(formatBytes(1.5 * 1024 ** 3)).toBe('1.5 ГБ')
    expect(formatBytes(300 * 1024 ** 2)).toBe('300 МБ')
    expect(formatBytes(250 * 1024 ** 3)).toBe('250 ГБ')
  })

  it('shows dates in Moscow time', () => {
    // 21:30 UTC уже следующий день в Москве
    expect(formatDate('2026-11-08T21:30:00Z')).toContain('9 ноября')
  })
})
