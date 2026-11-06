import { toast } from '@/ui'
import { formatCount, plural } from '../format'
import type { Broadcast } from '../types'
import { formatSchedule, isoToMskLocal } from './schedule'

// Тост после отправки: запущена сейчас или запланирована.
export function announceSent(b: Broadcast) {
  if (b.status === 'scheduled' && b.scheduledAt) {
    toast(`Рассылка запланирована на ${formatSchedule(isoToMskLocal(b.scheduledAt))} (МСК)`, { icon: 'check' })
    return
  }
  toast(`Рассылка запущена — ${formatCount(b.total)} ${plural(b.total, ['получатель', 'получателя', 'получателей'])}`, { icon: 'check' })
}
