import { StatusTag, type Tone } from '@/ui'
import { visibleText } from '../editor/serialize'
import { formatCount, formatShortDate } from '../format'
import type { Broadcast } from '../types'

export const titleOf = (b: Broadcast) =>
  visibleText(b.text)
    .split('\n')
    .find((line) => line.trim())
    ?.trim() ?? 'Без названия'

export const stats = (b: Broadcast) => {
  switch (b.status) {
    case 'draft':
      return 'черновик, не отправлялась'
    case 'scheduled':
      return b.scheduledAt ? `уйдёт ${formatShortDate(b.scheduledAt)} (МСК)` : 'запланирована'
    case 'cancelled':
      return 'отменена, не отправлялась'
    default:
      return `доставлено ${formatCount(b.sent)} из ${formatCount(b.total)}, не доставлено ${formatCount(b.failed)}`
  }
}

const TAGS: Record<Broadcast['status'], { tone: Tone; label: string }> = {
  draft: { tone: 'neutral', label: 'Черновик' },
  scheduled: { tone: 'warning', label: 'Запланирована' },
  sending: { tone: 'accent', label: 'Отправляется' },
  sent: { tone: 'success', label: 'Отправлена' },
  cancelled: { tone: 'neutral', label: 'Отменена' },
}
export const statusTag = (b: Broadcast) => <StatusTag tone={TAGS[b.status].tone}>{TAGS[b.status].label}</StatusTag>

// Когда ушла или уйдёт; у черновика — когда менялся
export const whenOf = (b: Broadcast) => formatShortDate(b.status === 'scheduled' && b.scheduledAt ? b.scheduledAt : b.status === 'draft' ? b.updatedAt : b.createdAt)

export const delivered = (b: Broadcast) => b.status === 'sending' || b.status === 'sent'
