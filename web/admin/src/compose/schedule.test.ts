import { describe, expect, it } from 'vitest'
import { formatSchedule, isoToMskLocal, mskLocalNow, scheduleInFuture, scheduleShapeOk } from './schedule'

describe('schedule (МСК, UTC+3, без перехода на летнее время)', () => {
  it('converts an instant to Moscow wall time, across midnight and in winter and summer alike', () => {
    expect(isoToMskLocal('2026-11-02T12:00:00Z')).toBe('2026-11-02T15:00')
    expect(isoToMskLocal('2026-11-02T21:30:00Z')).toBe('2026-11-03T00:30')
    expect(isoToMskLocal('2026-07-01T00:00:00Z')).toBe('2026-07-01T03:00')
    expect(isoToMskLocal('2026-12-31T20:59:00Z')).toBe('2026-12-31T23:59')
  })

  it('«now» is not the future, the next minute is', () => {
    const now = new Date('2026-11-02T12:00:30Z') // 15:00 МСК
    expect(mskLocalNow(now)).toBe('2026-11-02T15:00')
    expect(scheduleInFuture('2026-11-02T15:00', now)).toBe(false)
    expect(scheduleInFuture('2026-11-02T14:59', now)).toBe(false)
    expect(scheduleInFuture('2026-11-02T15:01', now)).toBe(true)
    expect(scheduleInFuture('', now)).toBe(false)
    expect(scheduleInFuture('завтра', now)).toBe(false)
  })

  it('shape check and label', () => {
    expect(scheduleShapeOk('2026-11-07T18:30')).toBe(true)
    expect(scheduleShapeOk('2026-11-07 18:30')).toBe(false)
    expect(scheduleShapeOk('2026-11-07T18:30:00')).toBe(false)
    expect(formatSchedule('2026-11-07T18:30')).toBe('7 нояб. 2026 в 18:30')
  })
})
