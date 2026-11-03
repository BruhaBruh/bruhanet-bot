// Тексты ошибок — как у сервера (POST /api/photos).
const types = ['image/jpeg', 'image/png', 'image/webp']
const maxSize = 10 << 20 // лимит Telegram на фото

export function checkPhoto(f: File): string | undefined {
  if (!types.includes(f.type)) return 'можно загрузить только JPEG, PNG или WebP'
  if (f.size > maxSize) return 'файл больше 10 МБ'
  return undefined
}
