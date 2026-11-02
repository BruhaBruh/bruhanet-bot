const dateFmt = new Intl.DateTimeFormat('ru-RU', {
  timeZone: 'Europe/Moscow',
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
})

// 02.01.2006 15:04 (МСК)
export function formatDate(iso: string): string {
  return dateFmt.format(new Date(iso)).replace(',', '')
}

// 2 350 ₽ (неразрывные пробелы)
export function formatRub(n: number): string {
  return `${new Intl.NumberFormat('ru-RU').format(n)} ₽`
}

export const statusLabels: Record<string, string> = {
  pending: 'ожидает',
  confirmed: 'подтверждён',
  declined: 'отклонён',
  cancelled: 'отменён',
}

// YYYY-MM-DD в МСК, сдвинутая на days (для default-периода фильтра)
export function mskDate(now: Date, days = 0): string {
  const d = new Date(now.getTime() + days * 864e5)
  return new Intl.DateTimeFormat('sv-SE', { timeZone: 'Europe/Moscow' }).format(d)
}

// русская плюрализация: plural(1, ['оплата','оплаты','оплат']) → «оплата»
export function plural(n: number, forms: [string, string, string]): string {
  const a = Math.abs(n) % 100
  const b = a % 10
  if (a > 10 && a < 20) return forms[2]
  if (b === 1) return forms[0]
  if (b >= 2 && b <= 4) return forms[1]
  return forms[2]
}

// «сентябрь 2026» (МСК)
export function formatMonthYear(iso: string): string {
  const p = new Intl.DateTimeFormat('ru-RU', { timeZone: 'Europe/Moscow', month: 'long', year: 'numeric' }).formatToParts(new Date(iso))
  return `${p.find((x) => x.type === 'month')?.value} ${p.find((x) => x.type === 'year')?.value}`
}
