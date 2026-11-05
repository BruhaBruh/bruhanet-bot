import { useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'
import { Button, Icon, Spinner } from '@/ui'
import { ApiError, upload } from '../api'
import { checkPhoto } from './photo'

// value — file_id загруженного фото; url — локальное превью, у дублированной рассылки его нет
export interface PhotoPickerProps {
  value?: string
  onChange(photo: string | undefined, url?: string): void
  onUploading?(busy: boolean): void
  disabled?: boolean
}

export function PhotoPicker({ value, onChange, onUploading, disabled }: PhotoPickerProps) {
  const [file, setFile] = useState({ url: '', name: '' })
  const [busy, setBusy] = useState(false)
  const [dragging, setDragging] = useState(false)
  const [error, setError] = useState('')
  const input = useRef<HTMLInputElement>(null)
  // gen меняется при сбросе/удалении: результат загрузки, начатой до этого, игнорируется
  const gen = useRef(0)

  useEffect(() => () => void (file.url && URL.revokeObjectURL(file.url)), [file.url])
  // родитель сбросил фото (после отправки)
  useEffect(() => {
    if (!value) gen.current++
  }, [value])

  function setUploading(v: boolean) {
    setBusy(v)
    onUploading?.(v)
  }

  async function pick(f: File) {
    const bad = checkPhoto(f) ?? ''
    setError(bad)
    if (bad) return
    setUploading(true)
    const g = gen.current
    try {
      const { photo } = await upload<{ photo: string }>('/api/photos', f)
      if (g !== gen.current) return
      const url = URL.createObjectURL(f)
      setFile({ url, name: f.name })
      onChange(photo, url)
    } catch (err) {
      if (g !== gen.current) return
      setError(err instanceof ApiError && err.body.message ? err.body.message : 'Не удалось загрузить фото')
    } finally {
      setUploading(false)
    }
  }

  function remove() {
    gen.current++
    setFile({ url: '', name: '' })
    setError('')
    onChange(undefined)
  }

  const locked = disabled || busy
  const open = () => input.current?.click()

  return (
    <div
      className="flex flex-col gap-3"
      onDragOver={(e) => {
        e.preventDefault()
        if (!locked) setDragging(true)
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault()
        setDragging(false)
        const f = e.dataTransfer.files[0]
        if (f && !locked) void pick(f)
      }}
    >
      <input
        ref={input}
        type="file"
        accept="image/jpeg,image/png,image/webp"
        hidden
        onChange={(e) => {
          const f = e.target.files?.[0]
          e.target.value = ''
          if (f) void pick(f)
        }}
      />
      {value ? (
        <div className={cn('flex flex-wrap items-center gap-4 rounded-md', dragging && 'outline-2 outline-accent')}>
          {file.url ? (
            <img src={file.url} alt="Прикреплённое фото" className="block h-22.5 w-40 rounded-md bg-fill-tertiary object-cover" />
          ) : (
            <div className="flex h-22.5 w-40 items-center justify-center rounded-md bg-fill-tertiary text-text-secondary">
              <Icon name="card" size={24} />
            </div>
          )}
          <div className="flex min-w-0 flex-[1_1_--spacing(40)] flex-col gap-2">
            <span className="truncate">{file.name || 'Фото из рассылки'}</span>
            <div className="flex gap-2">
              <Button size="sm" variant="gray" loading={busy} disabled={disabled} onClick={open}>
                Заменить
              </Button>
              <Button size="sm" variant="destructive" icon="trash" disabled={disabled} onClick={remove}>
                Удалить
              </Button>
            </div>
          </div>
        </div>
      ) : (
        <button
          type="button"
          disabled={locked}
          onClick={open}
          className={cn(
            'focus-ring flex min-h-18 w-full cursor-pointer flex-wrap items-center justify-center gap-2.5 rounded-md border-[1.5px] border-dashed border-separator bg-page-bg px-4 py-3 text-accent-text disabled:cursor-default disabled:opacity-60',
            dragging && 'border-accent bg-accent-tint',
          )}
        >
          {busy ? <Spinner size={20} color="currentColor" /> : <Icon name="plus" />}
          <span className="font-semibold">Добавить фото</span>
          <span className="footnote text-text-secondary">или перетащите файл</span>
        </button>
      )}
      {busy && (
        <span role="status" className="sr-only">
          Загрузка…
        </span>
      )}
      {error && (
        <p role="alert" className="footnote text-destructive-strong">
          {error}
        </p>
      )}
    </div>
  )
}
