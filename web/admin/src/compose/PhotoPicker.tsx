import { Button, VisuallyHidden } from '@telegram-apps/telegram-ui'
import { useEffect, useRef, useState } from 'react'
import { ApiError, upload } from '../api'
import { checkPhoto } from './photo'

// value — file_id загруженного фото
export interface PhotoPickerProps {
  value?: string
  onChange(photo: string | undefined): void
  onUploading?(busy: boolean): void
  disabled?: boolean
}

export function PhotoPicker({ value, onChange, onUploading, disabled }: PhotoPickerProps) {
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const input = useRef<HTMLInputElement>(null)
  // gen меняется при сбросе/удалении: результат загрузки, начатой до этого, игнорируется
  const gen = useRef(0)

  useEffect(() => () => void (url && URL.revokeObjectURL(url)), [url])
  // родитель сбросил фото (после отправки) — убрать превью
  useEffect(() => {
    if (value) return
    gen.current++
    setUrl('')
  }, [value])

  function setUploading(v: boolean) {
    setBusy(v)
    onUploading?.(v)
  }

  async function pick(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0]
    e.target.value = ''
    if (!f) return
    const bad = checkPhoto(f) ?? ''
    setError(bad)
    if (bad) return
    setUploading(true)
    const g = gen.current
    try {
      const { photo } = await upload<{ photo: string }>('/api/photos', f)
      if (g !== gen.current) return
      setUrl(URL.createObjectURL(f))
      onChange(photo)
    } catch (err) {
      if (g !== gen.current) return
      setError(err instanceof ApiError && err.body.message ? err.body.message : 'Не удалось загрузить фото')
    } finally {
      setUploading(false)
    }
  }

  function remove() {
    gen.current++
    setUrl('')
    onChange(undefined)
  }

  return (
    <div className="photo">
      <input ref={input} type="file" accept="image/jpeg,image/png,image/webp" hidden onChange={pick} />
      {value ? (
        <>
          {url && <img src={url} alt="Фото" />}
          <Button mode="plain" size="s" disabled={disabled} onClick={remove}>
            Удалить фото
          </Button>
        </>
      ) : (
        <Button mode="bezeled" size="s" loading={busy} disabled={disabled} onClick={() => input.current?.click()}>
          🖼 Добавить фото
        </Button>
      )}
      {busy && (
        <VisuallyHidden role="status">Загрузка…</VisuallyHidden>
      )}
      {error && (
        <p className="tg-err" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}
