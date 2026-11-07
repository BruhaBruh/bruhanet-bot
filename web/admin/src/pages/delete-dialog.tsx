import { useRef } from 'react'
import { Dialog, toast } from '@/ui'
import { ApiError, del } from '../api'

interface DeleteDialogProps {
  title: string
  message: string
  /** Адрес DELETE. */
  url: string
  /** Тост после успеха (прошедшее время). */
  done: string
  /** Тост на 409 (строка стала историей, пока диалог был открыт). */
  conflict: string
  /** Закрыть диалог; зовётся до запроса, так что второй клик по «Удалить» невозможен. */
  onClose: () => void
  /** Строка исчезла (удалена сейчас или уже была): обновить список или уйти с экрана. */
  onDeleted: () => void
  /** Строка устарела (409): обновить, не уходя. */
  onStale?: () => void
}

/** Подтверждение удаления: один DELETE на нажатие, 404 считается успехом (уже удалено). */
export function DeleteDialog({ title, message, url, done, conflict, onClose, onDeleted, onStale }: DeleteDialogProps) {
  const busy = useRef(false)
  async function run() {
    onClose()
    if (busy.current) return
    busy.current = true
    try {
      await del(url)
      toast(done, { icon: 'check' })
      onDeleted()
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) onDeleted()
      else if (e instanceof ApiError && e.status === 409) {
        toast(conflict)
        onStale?.()
      } else toast('Не удалось удалить, попробуйте ещё раз')
    } finally {
      busy.current = false
    }
  }
  return (
    <Dialog
      title={title}
      message={message}
      actions={[
        { label: 'Назад', style: 'cancel', onClick: onClose },
        { label: 'Удалить', style: 'destructive', onClick: () => void run() },
      ]}
    />
  )
}
