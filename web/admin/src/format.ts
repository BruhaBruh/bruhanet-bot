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
