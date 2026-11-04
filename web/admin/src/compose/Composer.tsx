import { Button } from '@telegram-apps/telegram-ui'
import { useEffect, useRef, useState } from 'react'
import { ApiError, get, post } from '../api'
import { sanitize, visibleLen, visibleText } from '../editor/serialize'
import { TgEditor } from '../editor/TgEditor'
import type { Broadcast, DirectMessage } from '../types'
import { PhotoPicker } from './PhotoPicker'

// broadcast — всем из панели; direct — одному юзеру (userId)
export type ComposerProps =
  | { mode: 'broadcast'; onSent?(b: Broadcast): void }
  | { mode: 'direct'; userId: number; onSent?(m: DirectMessage): void }

export function Composer(props: ComposerProps) {
  const { mode } = props
  const [text, setText] = useState('')
  const [photo, setPhoto] = useState<string>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [testSent, setTestSent] = useState(false)
  const [confirming, setConfirming] = useState(false)
  // null — грузится, undefined — панель недоступна
  const [recipients, setRecipients] = useState<number | null | undefined>(null)
  const [uploading, setUploading] = useState(false)
  // ключ идемпотентности — на конкретное содержимое: повтор без правок после сбоя
  // не создаст вторую рассылку, а правка даёт новый ключ (иначе сервер вернёт старую)
  const key = useRef(crypto.randomUUID())
  // синхронный флаг: второй клик до перерисовки не шлёт второй запрос
  const inFlight = useRef(false)

  async function loadRecipients() {
    setRecipients(null)
    try {
      setRecipients((await get<{ count: number }>('/api/broadcasts/recipients')).count)
    } catch {
      setRecipients(undefined)
    }
  }
  useEffect(() => {
    if (mode === 'broadcast') void loadRecipients()
  }, [mode])

  // правка черновика (и сброс после успеха) отменяет начатое подтверждение и меняет ключ
  useEffect(() => {
    key.current = crypto.randomUUID()
    setConfirming(false)
    setTestSent(false)
  }, [text, photo])

  const len = visibleLen(text)
  const limit = photo ? 1024 : 4096
  const valid = !uploading && len <= limit && (!!photo || visibleText(text).trim() !== '')

  const body = () => ({ text, ...(photo ? { photo } : {}) })

  async function run(fn: () => Promise<void>) {
    if (inFlight.current) return
    inFlight.current = true
    setError('')
    setTestSent(false)
    setBusy(true)
    try {
      await fn()
    } catch (e) {
      setError(
        e instanceof ApiError && e.body.error === 'panel_unavailable'
          ? 'Панель недоступна, рассылка не создана'
          : e instanceof ApiError && e.body.message
            ? e.body.message
            : 'Не удалось отправить, попробуйте ещё раз',
      )
    } finally {
      inFlight.current = false
      setBusy(false)
    }
  }

  const sendTest = () =>
    run(async () => {
      await post('/api/broadcasts/preview-send', body())
      setTestSent(true)
    })

  function reset() {
    setText('')
    setPhoto(undefined)
  }

  const broadcast = () =>
    run(async () => {
      setConfirming(false)
      const { broadcast } = await post<{ broadcast: Broadcast }>('/api/broadcasts', { idempotencyKey: key.current, ...body() })
      reset()
      if (props.mode === 'broadcast') props.onSent?.(broadcast)
    })

  const direct = () =>
    run(async () => {
      if (props.mode !== 'direct') return
      const b = body()
      const { id } = await post<{ id: number }>(`/api/users/${props.userId}/messages`, b)
      reset()
      props.onSent?.({ id, telegramId: props.userId, ...b, status: 'pending', createdAt: new Date().toISOString() })
    })

  return (
    <div className="composer">
      <TgEditor value={text} onChange={setText} disabled={busy} />
      <div className="composer-row">
        <PhotoPicker value={photo} onChange={setPhoto} onUploading={setUploading} disabled={busy} />
        <span className={len > limit ? 'count over' : 'count'}>{`${len} / ${limit}`}</span>
      </div>

      <div className="composer-label">Превью</div>
      {/* только разрешённые теги после sanitize */}
      <div className="tg-msg">
        {photo && <div className="ph">🖼 фото</div>}
        <div dangerouslySetInnerHTML={{ __html: sanitize(text) }} />
      </div>

      {mode === 'broadcast' && recipients === undefined && (
        <div className="tg-err" role="alert">
          Панель недоступна, количество получателей неизвестно{' '}
          <Button mode="plain" size="s" onClick={loadRecipients}>
            Повторить
          </Button>
        </div>
      )}
      {error && (
        <p className="tg-err" role="alert">
          {error}
        </p>
      )}
      {testSent && (
        <p className="tg-ok" role="status">
          Отправлено в Telegram
        </p>
      )}

      <div className="composer-actions">
        {mode === 'broadcast' ? (
          confirming ? (
            <>
              <Button loading={busy} disabled={!valid} onClick={broadcast}>
                {`Точно разослать ${recipients} юзерам?`}
              </Button>
              <Button mode="plain" disabled={busy} onClick={() => setConfirming(false)}>
                Отмена
              </Button>
            </>
          ) : (
            <Button loading={busy} disabled={!valid || !recipients} onClick={() => setConfirming(true)}>
              {`Разослать всем${recipients != null ? ` (${recipients})` : ''}`}
            </Button>
          )
        ) : (
          <Button loading={busy} disabled={!valid} onClick={direct}>
            Отправить
          </Button>
        )}
        <Button mode="bezeled" disabled={busy || !valid} onClick={sendTest}>
          Отправить себе
        </Button>
      </div>
    </div>
  )
}
