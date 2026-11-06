import { formatDay } from '../format'

// Время рассылки — московское, без пояса (datetime-local): МСК не знает перехода на летнее время.
const parts = new Intl.DateTimeFormat('en-GB', {
  timeZone: 'Europe/Moscow',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
})

// ISO → «ГГГГ-ММ-ДДTЧЧ:ММ» по Москве
export function isoToMskLocal(iso: string | Date): string {
  const p = Object.fromEntries(parts.formatToParts(new Date(iso)).map((x) => [x.type, x.value]))
  return `${p.year}-${p.month}-${p.day}T${p.hour}:${p.minute}`
}

export const mskLocalNow = (now = new Date()) => isoToMskLocal(now)

const SHAPE = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/

export const scheduleShapeOk = (v: string) => SHAPE.test(v)

// Строго позже текущей минуты: «сейчас» уже не будущее
export const scheduleInFuture = (v: string, now = new Date()) => SHAPE.test(v) && v > mskLocalNow(now)

// «7 нояб. 2026 в 18:30» (МСК)
export function formatSchedule(v: string): string {
  const [day, time] = v.split('T')
  return `${formatDay(day)} в ${time}`
}
