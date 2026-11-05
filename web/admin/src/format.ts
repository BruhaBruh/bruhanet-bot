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
  return `${new Intl.NumberFormat('ru-RU').format(n)}\u00a0₽`
}

// 3 214 (неразрывный пробел)
export function formatCount(n: number): string {
  return new Intl.NumberFormat('ru-RU').format(n)
}

// Как в макете: Intl даёт «мар.», «июн.», «июл.»
const SHORT_MONTHS = ['янв.', 'февр.', 'марта', 'апр.', 'мая', 'июня', 'июля', 'авг.', 'сент.', 'окт.', 'нояб.', 'дек.']

const timeFmt = new Intl.DateTimeFormat('ru-RU', { timeZone: 'Europe/Moscow', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })

// «сегодня, 14:32», «вчера, 21:47», «4 нояб., 14:32», другой год — «4 нояб. 2025, 14:32» (МСК)
export function formatShortDate(iso: string, now = new Date()): string {
  const d = new Date(iso)
  const day = mskDate(d)
  const time = timeFmt.format(d)
  if (day === mskDate(now)) return `сегодня, ${time}`
  if (day === mskDate(now, -1)) return `вчера, ${time}`
  return `${formatDay(day, now)}, ${time}`
}

// ГГГГ-ММ-ДД → «30 нояб. 2026»; с now год только если он не текущий
export function formatDay(ymd: string, now?: Date): string {
  const [year, month, date] = ymd.split('-')
  const label = `${Number(date)} ${SHORT_MONTHS[Number(month) - 1]}`
  return now && year === mskDate(now).slice(0, 4) ? label : `${label} ${year}`
}

// Изменение к прошлому периоду: «+12%», «-4%»; без базы сравнивать не с чем
export function formatDelta(current: number, previous: number): string | undefined {
  if (previous === 0) return undefined
  const percent = Math.round(((current - previous) / previous) * 100)
  return `${percent > 0 ? '+' : ''}${percent}%`
}

export const statusLabels: Record<string, string> = {
  pending: 'ожидает',
  confirmed: 'подтверждён',
  declined: 'отклонён',
  cancelled: 'отменён',
}

// статус юзера в Remnawave (+ not_found от api)
export const panelStatusLabels: Record<string, string> = {
  ACTIVE: 'активна',
  DISABLED: 'отключена',
  LIMITED: 'ограничена',
  EXPIRED: 'исчерпана',
  not_found: 'нет в панели',
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
// Месяц в родительном падеже («с ноября 2026»): с day Intl отдаёт «ноября», без day — «ноябрь».
export function formatMonthYear(iso: string): string {
  const p = new Intl.DateTimeFormat('ru-RU', { timeZone: 'Europe/Moscow', day: 'numeric', month: 'long', year: 'numeric' }).formatToParts(new Date(iso))
  return `${p.find((x) => x.type === 'month')?.value} ${p.find((x) => x.type === 'year')?.value}`
}
