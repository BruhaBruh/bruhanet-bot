const date = new Intl.DateTimeFormat('ru-RU', { timeZone: 'Europe/Moscow', day: 'numeric', month: 'long', year: 'numeric' })
const dateTime = new Intl.DateTimeFormat('ru-RU', {
  timeZone: 'Europe/Moscow',
  day: 'numeric',
  month: 'long',
  hour: '2-digit',
  minute: '2-digit',
})

// Время показываем по Москве, как везде в проекте.
export const formatDate = (iso: string) => date.format(new Date(iso))
export const formatDateTime = (iso: string) => `${dateTime.format(new Date(iso))} МСК`

export function formatBytes(n: number): string {
  const gb = n / 1024 ** 3
  if (gb >= 100) return `${Math.round(gb)} ГБ`
  if (gb >= 1) return `${gb.toFixed(1).replace('.0', '')} ГБ`
  return `${Math.round(n / 1024 ** 2)} МБ`
}

// plural(1, ['день','дня','дней']) → «день»
export function plural(n: number, forms: [string, string, string]): string {
  const a = Math.abs(n) % 100
  const b = a % 10
  if (a > 10 && a < 20) return forms[2]
  if (b === 1) return forms[0]
  if (b >= 2 && b <= 4) return forms[1]
  return forms[2]
}

export const days = (n: number) => `${n} ${plural(n, ['день', 'дня', 'дней'])}`
