const COLORS = ['red', 'orange', 'violet', 'green', 'cyan', 'blue', 'pink'] as const

export interface AvatarProps {
  name?: string
  id?: string | number
  src?: string
  size?: number
  color?: (typeof COLORS)[number]
  colorIndex?: number
  /** Картинка не загрузилась (404, сеть): вызывающий возвращает буквенный аватар. */
  onImgError?: () => void
}

function initials(name?: string) {
  const parts = String(name || '?')
    .replace(/^@/, '')
    .trim()
    .split(/\s+/)
  const first = (part: string) => Array.from(part)[0] ?? ''
  return (first(parts[0] || '?') + (parts[1] ? first(parts[1]) : '')).toUpperCase()
}

function hash(value: string) {
  let x = 0
  for (let i = 0; i < value.length; i++) x = (x * 31 + value.charCodeAt(i)) | 0
  return Math.abs(x)
}

const base = 'inline-flex flex-none items-center justify-center rounded-full font-semibold select-none'

export function Avatar({ name, id, src, size = 40, color, colorIndex, onImgError }: AvatarProps) {
  const style = { width: size, height: size, fontSize: Math.round(size * 0.4) }
  const tone = color ?? COLORS[(colorIndex ?? hash(String(id || name || ''))) % COLORS.length]
  const background = `linear-gradient(180deg, var(--avatar-${tone}-top), var(--avatar-${tone}))`
  // имя есть в тексте рядом: картинка декоративна; градиент держит место, пока она грузится
  if (src)
    return (
      <img
        className={`${base} object-cover`}
        src={src}
        alt=""
        loading="lazy"
        decoding="async"
        style={{ ...style, background }}
        onError={onImgError}
      />
    )
  return (
    <span
      className={`${base} text-white`}
      role="img"
      aria-label={name}
      style={{ ...style, background }}
    >
      {initials(name)}
    </span>
  )
}
