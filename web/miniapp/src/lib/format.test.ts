import { describe, expect, it } from 'vitest'
import { days, formatBytes, formatDate, plural } from './format'

describe('format', () => {
  it('pluralizes russian words', () => {
    const word = (n: number) => plural(n, ['день', 'дня', 'дней'])
    expect([1, 2, 5, 11, 12, 13, 14, 21, 22, 25, 111].map(word)).toEqual(['день', 'дня', 'дней', 'дней', 'дней', 'дней', 'дней', 'день', 'дня', 'дней', 'дней'])
    expect(plural(0, ['а', 'б', 'в'])).toBe('в')
  })

  it('writes days in the short form', () => {
    expect([0, 1, 2, 5, 11, 26744].map(days)).toEqual(['0 д.', '1 д.', '2 д.', '5 д.', '11 д.', '26744 д.'])
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
