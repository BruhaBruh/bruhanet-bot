import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { Avatar, Button, Card, Dialog, Icon, MessageBubble, Spinner, toast } from '@/ui'
import { BOT_NAME } from '@/brand'
import { ApiError, get, post } from '../api'
import { audienceLabel } from '../compose/audience'
import { formatSchedule, isoToMskLocal } from '../compose/schedule'
import { previewHtml, renderPreview } from '../editor/serialize'
import { formatCount, formatShortDate } from '../format'
import type { Broadcast, TextVar } from '../types'
import { delivered, statusTag, titleOf } from './broadcast-format'

const row = 'flex flex-wrap items-baseline justify-between gap-3 py-2 shadow-[inset_0_-0.5px_0_var(--separator)] last:shadow-none'

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className={row}>
      <dt className="text-text-secondary">{label}</dt>
      <dd className="text-right">{children}</dd>
    </div>
  )
}

// Экран рассылки: что и кому ушло, расписание, счётчики; действия по статусу.
export default function BroadcastView() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [data, setData] = useState<{ id?: string; broadcast?: Broadcast; failed?: boolean }>({})
  const [globals, setGlobals] = useState<TextVar[]>([])
  const [cancelling, setCancelling] = useState(false)
  const [busy, setBusy] = useState(false)

  const load = useCallback(
    () =>
      get<{ broadcast: Broadcast }>(`/api/broadcasts/${id}`).then(
        (r) => setData({ id, broadcast: r.broadcast }),
        () => setData((d) => (d.id === id && d.broadcast ? d : { id, failed: true })),
      ),
    [id],
  )

  useEffect(() => {
    void load()
  }, [load])
  useEffect(() => {
    get<{ globals: TextVar[] }>('/api/texts').then(
      (r) => setGlobals(r.globals),
      () => {},
    )
  }, [])

  const b = data.id === id ? data.broadcast : undefined
  const sending = b?.status === 'sending'
  // счётчики растут, пока идёт доставка
  useEffect(() => {
    if (!sending) return
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible') void load()
    }, 5000)
    return () => clearInterval(timer)
  }, [sending, load])

  const back = (
    <div className="-my-2 -ml-2">
      <Button variant="plain" icon="chevron-left" onClick={() => navigate('/broadcasts')}>
        Все рассылки
      </Button>
    </div>
  )
  if (data.id !== id) return <Spinner className="self-center p-8" />
  if (!b) {
    return (
      <>
        {back}
        <p role="alert">Не удалось загрузить рассылку</p>
      </>
    )
  }

  async function cancel() {
    if (busy) return
    setBusy(true)
    setCancelling(false)
    try {
      await post(`/api/broadcasts/${id}/cancel`)
      toast('Рассылка отменена', { icon: 'check' })
    } catch (e) {
      toast(e instanceof ApiError && e.status === 409 ? 'Рассылка уже запущена, отменить нельзя' : 'Не удалось отменить, попробуйте ещё раз')
    } finally {
      setBusy(false)
      void load()
    }
  }

  const duplicate = () => {
    navigate('/broadcasts/new', {
      state: { text: b.text, photo: b.photo, audience: b.audience, audienceDays: b.audienceDays, silent: b.silent, buttons: b.buttons },
    })
    toast(`Копия «${titleOf(b)}» открыта в редакторе`, { icon: 'check' })
  }

  const samples = Object.fromEntries(globals.map((v) => [v.name, v.sample]))
  const editable = b.status === 'draft' || b.status === 'scheduled'
  const done = b.sent + b.failed
  const percent = b.total > 0 ? Math.min(100, Math.round((done / b.total) * 100)) : 0

  return (
    <>
      {back}
      <div className="admin-body flex flex-wrap items-start gap-6 text-text">
        <div className="flex min-w-0 grow-999 basis-130 flex-col gap-4">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="title-2 min-w-0 grow truncate">{titleOf(b)}</h1>
            {statusTag(b)}
          </div>

          <Card title="Параметры">
            <dl className="flex flex-col">
              <Row label="Получатели">{audienceLabel(b.audience, String(b.audienceDays ?? ''))}</Row>
              <Row label="Звук">{b.silent ? 'Без звука' : 'Со звуком'}</Row>
              {b.status === 'scheduled' && b.scheduledAt && <Row label="Уйдёт">{formatSchedule(isoToMskLocal(b.scheduledAt))} (МСК)</Row>}
              {b.status !== 'draft' && <Row label="Создана">{formatShortDate(b.createdAt)}</Row>}
              {b.status === 'draft' && <Row label="Изменена">{formatShortDate(b.updatedAt)}</Row>}
              {b.finishedAt && <Row label="Завершена">{formatShortDate(b.finishedAt)}</Row>}
              <Row label="Кнопки">{b.buttons.length ? `${b.buttons.length}: ${b.buttons.map((x) => x.text).join(', ')}` : 'нет'}</Row>
            </dl>
          </Card>

          <Card title="Доставка">
            {delivered(b) ? (
              <>
                <div
                  role="progressbar"
                  aria-label="Доставлено"
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={percent}
                  className="h-2 overflow-hidden rounded-full bg-fill-tertiary"
                >
                  <div className="h-full bg-accent" style={{ width: `${percent}%` }} />
                </div>
                <dl className="flex flex-col">
                  <Row label="Получателей">{formatCount(b.total)}</Row>
                  <Row label="Доставлено">{formatCount(b.sent)}</Row>
                  <Row label="Не доставлено">{formatCount(b.failed)}</Row>
                </dl>
                <p className="footnote text-text-secondary">
                  {sending ? `Идёт отправка: обработано ${formatCount(done)} из ${formatCount(b.total)} (${percent}%).` : 'Недоставленные — те, кто заблокировал бота или удалил чат.'}
                </p>
              </>
            ) : (
              <p className="text-text-secondary">
                {b.status === 'cancelled' ? 'Рассылка отменена и не отправлялась.' : 'Получатели определятся в момент отправки: счётчики появятся после старта.'}
              </p>
            )}
          </Card>

          <div className="flex flex-wrap justify-end gap-3">
            <Button variant="gray" icon="copy" onClick={duplicate}>
              Дублировать
            </Button>
            {editable && (
              <Button variant="gray" icon="edit" onClick={() => navigate(`/broadcasts/${b.id}/edit`)}>
                Изменить
              </Button>
            )}
            {b.status === 'scheduled' && (
              <Button variant="gray" icon="close" loading={busy} onClick={() => setCancelling(true)}>
                Отменить рассылку
              </Button>
            )}
          </div>
        </div>

        <aside aria-label="Сообщение" className="flex max-w-100 min-w-0 grow basis-85 flex-col gap-2 md:sticky md:top-6">
          <h2 className="admin-label px-1 text-text-secondary">Сообщение с примером данных</h2>
          <div className="flex flex-col overflow-hidden rounded-lg shadow-card">
            <div className="flex items-center gap-2.5 bg-header-bg px-3.5 py-2.5 shadow-[inset_0_-0.5px_0_var(--separator)]">
              <Avatar name={BOT_NAME} colorIndex={5} size={32} />
              <div className="flex flex-col">
                <span className="font-semibold">{BOT_NAME}</span>
                <span className="caption-1 text-text-secondary">бот</span>
              </div>
            </div>
            <div className="flex min-h-60 flex-col justify-end bg-chat-bg px-3 py-4">
              <MessageBubble
                time={b.createdAt ? new Intl.DateTimeFormat('ru-RU', { timeZone: 'Europe/Moscow', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(new Date(b.createdAt)) : ''}
                keyboard={b.buttons.map((x) => [{ text: x.text, url: x.url, style: x.style || 'default' }])}
                media={
                  b.photo && (
                    <div className="flex aspect-video w-full min-w-60 items-center justify-center bg-fill-tertiary text-text-secondary">
                      <Icon name="card" size={32} label="Фото" />
                    </div>
                  )
                }
              >
                {b.text.trim() === '' ? (
                  <span className="text-text-hint">Текста нет</span>
                ) : (
                  // только разрешённые теги после sanitize
                  <span className="whitespace-pre-wrap" dangerouslySetInnerHTML={{ __html: previewHtml(renderPreview(b.text, samples)) }} />
                )}
              </MessageBubble>
            </div>
          </div>
          {b.photo && <p className="caption-1 px-1 text-text-secondary">Фото отправлено как есть; Telegram хранит его по file_id, картинку здесь не показываем.</p>}
        </aside>
      </div>

      {cancelling && (
        <Dialog
          title="Отменить рассылку?"
          message="Она не уйдёт получателям. Вернуть отменённую нельзя — можно создать копию."
          actions={[
            { label: 'Не отменять', style: 'cancel', onClick: () => setCancelling(false) },
            { label: 'Отменить рассылку', style: 'destructive', onClick: () => void cancel() },
          ]}
        />
      )}
    </>
  )
}
